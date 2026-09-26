package main

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/image/font"

	"github.com/wisborg/osmbase/fetch"
	"github.com/wisborg/osmbase/render"
	"github.com/wisborg/osmbase/slice"

	"github.com/wisborg/course"
	"github.com/wisborg/course/routemap"
)

var mapOpts struct {
	out           string
	width, height int
	palette       string
	store         string
	archive       string
	yes           bool
}

var mapCmd = &cobra.Command{
	Use:   "map COURSE [COURSE ...]",
	Short: "Draw a course over the map",
	Long: `map draws a course over the map, from an osmbase store on this machine: the
route, its start and finish, and a marker every so many kilometres where the
file recorded distance. A stretch the recording has no fixes for -- a tunnel,
a flight over an ocean -- is dashed, because the straight line across it is
not where the course went.

The view fits the course. When the store lacks the map for it, map offers to
fetch it, which tells the archive's host the area; --yes answers in advance.
Declined, the map is drawn from what the store holds, from shallower tiles or
hatched where it holds nothing.

The picture carries the map data's credit in its corner, which is what the
data's licence asks of anything drawn from it.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMap,
}

func init() {
	f := mapCmd.Flags()
	f.StringVar(&mapOpts.out, "out", "", "the PNG to write (default: the course's name with .png, in this directory)")
	f.IntVar(&mapOpts.width, "width", 1600, "the picture's width in pixels")
	f.IntVar(&mapOpts.height, "height", 1000, "the picture's height in pixels")
	f.StringVar(&mapOpts.palette, "palette", "light", "the map's colours: light or dark")
	f.StringVar(&mapOpts.store, "store", "", "the osmbase store to draw from (default: osmbase's own)")
	f.StringVar(&mapOpts.archive, "archive", "", "which archive in the store, when it holds several")
	f.BoolVar(&mapOpts.yes, "yes", false, "fetch what the store lacks without asking")
	root.AddCommand(mapCmd)
}

func runMap(cmd *cobra.Command, args []string) error {
	palette, overlay, err := paletteNamed(mapOpts.palette)
	if err != nil {
		return err
	}
	if mapOpts.width < 64 || mapOpts.height < 64 {
		return fmt.Errorf("a map of %d by %d pixels is too small to draw a course on", mapOpts.width, mapOpts.height)
	}
	c, err := course.Read(args...)
	if err != nil {
		return err
	}
	if len(c.Points) == 0 {
		return errors.New("the course has no positions to draw")
	}
	out := mapOpts.out
	if out == "" {
		base := filepath.Base(args[0])
		out = strings.TrimSuffix(base, filepath.Ext(base)) + ".png"
	}

	view, cropped := render.Fit(extent(c), mapOpts.width, mapOpts.height, maxMapZoom)
	errw := cmd.ErrOrStderr()
	if cropped {
		fmt.Fprintf(errw, "course: the course is wider than the map at this size; it is drawn cropped\n")
	}
	root := mapOpts.store
	if root == "" {
		if root, err = slice.DefaultRoot(); err != nil {
			return fmt.Errorf("finding the default store: %w; pass --store", err)
		}
	}

	manifest, src, err := tilesIn(root, mapOpts.archive)
	if err != nil {
		return err
	}
	if n, short := mapNeed(root, manifest, src, view); short && offerToFill(cmd.Context(), errw, n, mapOpts.yes) {
		if manifest, src, err = tilesIn(root, mapOpts.archive); err != nil {
			return err
		}
	}

	img, res, err := basemap(cmd, src, manifest, view, palette)
	if err != nil {
		return err
	}
	scale := float64(max(view.Width, view.Height)) / 1000
	face := faceAt(baseTextSize * scale)
	drawing := routemap.Drawing(c, view, routemap.InksFor(palette, overlay), scale)
	if err := render.Draw(img, view, drawing, face); err != nil {
		return err
	}
	render.DrawCredit(img, manifest.Attribution, face)
	if err := writePNG(out, img); err != nil {
		return err
	}
	writeMapReport(cmd.OutOrStdout(), out, c, view, res, len(routemap.DistanceMarkers(c)))
	return nil
}

// maxMapZoom is the deepest a course's map is fitted at. The public tile
// builds stop at 15; a lap of a park fitted there is a small loop in four
// kilometres of town, and vector tiles drawn deeper are sharp, only emptier.
// The fetch offer still asks for no deeper than the archive has.
const maxMapZoom = 18

// room is the ground left round a course beyond render.Fit's own margin, as a
// fraction of its size each side, so a start or finish at the edge of the
// course has space for its label.
const room = 0.05

func paletteNamed(name string) (render.Palette, render.Overlay, error) {
	switch name {
	case "light":
		return render.LightPalette(), render.LightOverlay(), nil
	case "dark":
		return render.DarkPalette(), render.DarkOverlay(), nil
	}
	return render.Palette{}, render.Overlay{}, fmt.Errorf("--palette %q is not light or dark", name)
}

// extent is the rectangle the course covers, with a few hundred metres round
// a course that stands still -- one fix, or a treadmill's worth of the same
// one -- so it is a place on a map and not a point at zoom 15.
func extent(c *course.Course) render.Bounds {
	b := render.Bounds{West: 180, South: 90, East: -180, North: -90}
	for _, p := range c.Points {
		b.West, b.East = min(b.West, p.Lon), max(b.East, p.Lon)
		b.South, b.North = min(b.South, p.Lat), max(b.North, p.Lat)
	}
	if course.Metres(b.South, b.West, b.North, b.East) < 500 {
		const pad = 0.003
		b.West, b.East, b.South, b.North = b.West-pad, b.East+pad, b.South-pad, b.North+pad
	}
	dx, dy := (b.East-b.West)*room, (b.North-b.South)*room
	b.West, b.East = max(b.West-dx, -180), min(b.East+dx, 180)
	b.South, b.North = max(b.South-dy, -85), min(b.North+dy, 85)
	return b
}

// tilesIn is the store's archive, when it has one: a nil source is a store
// with no map, which the offer can fill.
func tilesIn(root, archive string) (slice.Manifest, *slice.Source, error) {
	st, err := slice.Open(root)
	if errors.Is(err, iofs.ErrNotExist) {
		return slice.Manifest{}, nil, nil
	}
	if err != nil {
		return slice.Manifest{}, nil, fmt.Errorf("opening the store at %s: %w", root, err)
	}
	sources, err := st.Sources()
	if err != nil {
		return slice.Manifest{}, nil, err
	}
	if len(sources) == 0 {
		return slice.Manifest{}, nil, nil
	}
	m, err := chooseSource(root, sources, archive)
	if err != nil {
		return m, nil, err
	}
	src, err := st.Source(m.ID)
	return m, src, err
}

// mapNeed is what the store lacks for the view, at the zoom it will be drawn
// at -- no deeper than the archive the store was filled from goes.
func mapNeed(root string, m slice.Manifest, src *slice.Source, v render.View) (need, bool) {
	z, _, err := v.Zoom()
	if err != nil {
		return need{}, false
	}
	b := slice.Bounds{West: v.Bounds.West, South: v.Bounds.South, East: v.Bounds.East, North: v.Bounds.North}
	n := need{root: root, archive: m.Source, areas: []slice.Bounds{b}, zoom: z, why: "to draw this map"}
	if src == nil {
		return n, true
	}
	n.zoom = fetch.DrawnZoom(src, z)
	held, wanted, err := src.HeldAt(b, n.zoom)
	if err != nil || held >= wanted {
		return need{}, false
	}
	n.held, n.wanted, n.why = held, wanted, "this map needs"
	return n, true
}

// basemap is the map under the course. With no map to draw from, it is the
// palette's background: a course over nothing is still a picture of the
// course, and the report says there was no map.
func basemap(cmd *cobra.Command, src *slice.Source, m slice.Manifest, v render.View, p render.Palette) (*image.RGBA, *render.Result, error) {
	blank := func() *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, v.Width, v.Height))
		draw.Draw(img, img.Bounds(), image.NewUniform(p.Background), image.Point{}, draw.Src)
		return img
	}
	if src == nil {
		return blank(), nil, nil
	}
	r, err := render.New(src, render.Options{
		Style: render.BasemapStyle(), Palette: p, Attribution: m.Attribution,
		LabelFace: faceAt(baseTextSize), LabelFaceFor: func(s float64) font.Face { return faceAt(baseTextSize * s) },
	})
	if err != nil {
		return nil, nil, err
	}
	res, err := r.Render(cmd.Context(), v)
	if errors.Is(err, render.ErrNoCoverage) {
		return blank(), nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return res.Image, res, nil
}

// writePNG writes to a temporary name and renames, so an interrupted write
// never leaves half a picture under the name asked for.
func writePNG(path string, img image.Image) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := png.Encode(tmp, img); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func writeMapReport(w io.Writer, out string, c *course.Course, v render.View, res *render.Result, markers int) {
	fmt.Fprintf(w, "%-10s %s, %d by %d\n", "map", out, v.Width, v.Height)
	if res == nil {
		fmt.Fprintf(w, "%-10s none: the store holds nothing of this view, so the course is drawn on a blank ground\n", "basemap")
	} else {
		fmt.Fprintf(w, "%-10s zoom %.1f, %.0f%% covered", "basemap", res.ContinuousZoom, 100*res.Covered)
		if res.Overzoomed > 0 {
			fmt.Fprintf(w, ", %.0f%% from shallower tiles", 100*res.Overzoomed)
		}
		fmt.Fprintln(w)
	}
	switch {
	case markers > 0:
		fmt.Fprintf(w, "%-10s %d distance markers\n", "markers", markers)
	case !measured(c):
		fmt.Fprintf(w, "%-10s none: the file records no distance\n", "markers")
	}
}

func measured(c *course.Course) bool {
	for _, p := range c.Points {
		if !p.HasDistance {
			return false
		}
	}
	return len(c.Points) > 0
}
