package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	iofs "io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/image/font"

	"github.com/wisborg/osmbase/fetch"
	"github.com/wisborg/osmbase/render"
	"github.com/wisborg/osmbase/slice"

	"github.com/wisborg/course"
	"github.com/wisborg/course/compare"
	"github.com/wisborg/course/match"
	"github.com/wisborg/course/routemap"
)

var mapOpts struct {
	out           string
	width, height int
	palette       string
	store         string
	archive       string
	yes           bool
	language      string
	fonts         []string
	references    []string
	greatCircle   bool
	whole         bool
	compare       string
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

Names are written in the built-in Go font, and letters it lacks -- Thai,
Indian scripts, Chinese -- in the first of the fonts given with --font, then
of a few common system fonts, that has them. Scripts whose letters join, like
Arabic and Devanagari, come out in real letters but not always joined
properly; --lang en writes names in English wherever the map has them.

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
	f.StringVar(&mapOpts.language, "lang", "", "write the map's names in this language where the map has them, e.g. en; default is each place's own")
	f.StringArrayVar(&mapOpts.references, "reference", nil, "a course to draw beside this one for comparison, dashed: a stored reference's name, a FIT, GPX, TCX, KML or KMZ file, or auto for every stored reference the course matched; repeat for several")
	f.StringVar(&referencesDir, "references", "", "the directory stored references are kept in (default: course/references in your configuration directory)")
	f.StringVar(&mapOpts.compare, "compare", "", "colour the course by how much faster or slower it was than another run of it, place by place: a stored reference's name or a file")
	f.BoolVar(&mapOpts.whole, "whole-references", false, "draw every reference whole, even where the course followed it (default: a reference the course followed is drawn only where the two part)")
	f.BoolVar(&mapOpts.greatCircle, "great-circle", false, "draw the great circle between the course's start and finish, dashed: the shortest way over the globe")
	f.StringArrayVar(&mapOpts.fonts, "font", nil, "a TrueType or OpenType font to write names in when the built-in font lacks their letters; repeat for several")
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
	if err := loadFonts(mapOpts.fonts); err != nil {
		return err
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
		out = nameOf(args[0]) + ".png"
	}
	refs, err := loadReferences(c, mapOpts.references, mapOpts.greatCircle, !mapOpts.whole)
	if err != nil {
		return err
	}

	view, cropped := render.Fit(extent(c, refs), mapOpts.width, mapOpts.height, maxMapZoom)
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
	inks := routemap.InksFor(palette, overlay)
	drawing := routemap.Drawing(c, view, inks, scale)
	var cmp *comparison
	if mapOpts.compare != "" {
		if cmp, err = compareWith(c, mapOpts.compare, scale); err != nil {
			return err
		}
		// The whole course thin and grey, for the stretches outside the
		// one compared -- a warm-up, a cool-down -- and the compared
		// stretch coloured over it.
		for i := range drawing.Lines {
			drawing.Lines[i].Ink, drawing.Lines[i].Dash = inks.Gap, nil
			drawing.Lines[i].Width *= 0.5
			drawing.Lines[i].Halo *= 0.5
		}
		drawing.Gradients = append(drawing.Gradients, cmp.gradient)
	}
	refInks := routemap.ReferenceInks(palette)
	drawing = routemap.WithReferences(drawing, refs, refInks, inks.Halo, scale)
	if err := render.Draw(img, view, drawing, face); err != nil {
		return err
	}
	if len(refs) > 0 || cmp != nil {
		var legend []entry
		if cmp == nil {
			legend = append(legend, entry{name: nameOf(args[0]), ink: inks.Route})
		} else {
			legend = append(legend, entry{name: nameOf(args[0]) + " against " + cmp.name, ramp: &cmp.ramp})
		}
		for i, r := range refs {
			name := r.Name
			switch {
			case r.Follows && len(r.Apart) == 0:
				name += ", followed all the way"
			case r.Follows:
				name += ", where the course left it"
			}
			legend = append(legend, entry{name: name, ink: refInks[i%len(refInks)], dashed: true})
		}
		drawLegend(img, legend, face, scale)
	}
	render.DrawCredit(img, manifest.Attribution, face)
	if err := writePNG(out, img); err != nil {
		return err
	}
	writeMapReport(cmd.OutOrStdout(), out, c, view, res, len(routemap.DistanceMarkers(c)))
	if cmp != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "%-10s against %s, %.2f km of it: %s\n", "compared", cmp.name,
			float64(len(cmp.profile.Run)-1)*cmp.profile.Step/1000, gapText(cmp.profile.Gap(len(cmp.profile.Run)-1)))
	}
	writeMissing(errw)
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

// nameOf is a file's name without its directory or extension: what a course
// read from it is called on a map.
func nameOf(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// loadReferences reads the references to draw beside c: each one given, a
// file if there is one by that name and a stored reference otherwise, and
// the great circle between c's ends when asked for.
//
// A file first, because a path is unambiguous and a name is only a name: a
// reference stored as "run.gpx" should not stop the file run.gpx being
// drawn.
//
// When apart is set, a reference the course followed is drawn only where the
// two part; one it did not follow is drawn whole, since all of it is
// different.
func loadReferences(c *course.Course, names []string, greatCircle, apart bool) ([]routemap.Reference, error) {
	var refs []routemap.Reference
	for _, n := range names {
		if n == "auto" {
			found, err := matchedReferences(c, apart)
			if err != nil {
				return nil, err
			}
			refs = append(refs, found...)
			continue
		}
		name, rc, err := resolveCourse(n)
		if err != nil {
			return nil, fmt.Errorf("--reference %w", err)
		}
		r := routemap.FromCourse(name, rc)
		if apart {
			if m, ok := bestMatch(match.Find(c, []match.Reference{{Name: name, Course: rc}}, match.Options{}), name); ok {
				r = routemap.Followed(name, rc, m)
			}
		}
		refs = append(refs, r)
	}
	if greatCircle {
		gc, ok := routemap.GreatCircle(c)
		if !ok {
			return nil, errors.New("--great-circle: the course ends where it started, and has no great circle")
		}
		refs = append(refs, gc)
	}
	return refs, nil
}

// comparison is a course coloured against another run of it.
type comparison struct {
	name     string
	profile  *compare.Profile
	gradient render.Gradient
	ramp     ramp
}

// compareRamp is how far the colours reach: 15% faster is red, 15% slower
// blue. Enough for a parkrun run hard against one jogged; a hill or a stop
// goes past it and is simply at the end of the scale.
var compareRamp = ramp{
	scale: render.Scale{Min: math.Log(1 / 1.15), Max: math.Log(1.15)},
	low:   "15% slower",
	high:  "15% faster",
}

// compareAround is how far either side of a point its pace is compared
// over, in metres: a few GPS fixes, so one fix out of place does not
// colour the course on its own.
const compareAround = 30

// compareEdge is the thin dark edge round a coloured course: enough to hold
// it off a map of any colour, little enough that the map shows beside it.
var compareEdge = color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}

// compareWith colours c against the run name names. How finely is the
// drawing's to decide, from the view.
func compareWith(c *course.Course, name string, scale float64) (*comparison, error) {
	refName, ref, err := resolveCourse(name)
	if err != nil {
		return nil, fmt.Errorf("--compare %w", err)
	}
	p, err := compare.Against(c, ref, match.Options{})
	if err != nil {
		return nil, fmt.Errorf("--compare %s: %w", refName, err)
	}
	g := render.Gradient{
		Values: p.Faster(compareAround),
		Scale:  compareRamp.scale,
		Width:  3 * scale, Halo: 0.8 * scale, HaloInk: compareEdge,
	}
	for _, w := range p.Where {
		g.Points = append(g.Points, render.Coord{Lat: w.Lat, Lon: w.Lon})
	}
	return &comparison{name: refName, profile: p, gradient: g, ramp: compareRamp}, nil
}

// gapText is the gap at the end of a comparison, in words.
func gapText(gap time.Duration) string { return gapWords(gap) + " at the end" }

// resolveCourse is the course a reference names: a file if there is one by
// that name -- a path is unambiguous, and a reference stored as "run.gpx"
// should not stop the file run.gpx being read -- and a stored reference
// otherwise. The name returned is the one to show it by.
func resolveCourse(n string) (string, *course.Course, error) {
	if info, err := os.Stat(n); err == nil && !info.IsDir() {
		c, err := course.Read(n)
		if err != nil {
			return "", nil, fmt.Errorf("%s: %w", n, err)
		}
		if len(c.Points) < 2 {
			return "", nil, fmt.Errorf("%s has no line", n)
		}
		return nameOf(n), c, nil
	}
	s, err := openReferences()
	if err != nil {
		return "", nil, err
	}
	m, err := s.Find(n)
	if err != nil {
		return "", nil, fmt.Errorf("%s: no such file, and %w", n, err)
	}
	c, err := m.Course()
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", n, err)
	}
	return m.Name, c, nil
}

// matchedReferences are the stored references the course matched, each once
// however often it was matched, for --reference auto: drawn where the course
// parted from them when apart is set, and whole otherwise.
func matchedReferences(c *course.Course, apart bool) ([]routemap.Reference, error) {
	stored, err := matchReferences(nil)
	if err != nil {
		return nil, err
	}
	ms := match.Find(c, stored, match.Options{})
	var out []routemap.Reference
	for _, r := range stored {
		m, ok := bestMatch(ms, r.Name)
		switch {
		case !ok:
		case apart:
			out = append(out, routemap.Followed(r.Name, r.Course, m))
		default:
			out = append(out, routemap.FromCourse(r.Name, r.Course))
		}
	}
	// In the order the course reached them, as before: a legend reads
	// down the morning.
	sort.SliceStable(out, func(i, j int) bool { return firstMatch(ms, out[i].Name) < firstMatch(ms, out[j].Name) })
	return out, nil
}

// bestMatch is the match with name that covered most of it: when a course
// followed a reference twice, the pass that parted from it least.
func bestMatch(ms []match.Match, name string) (match.Match, bool) {
	var best match.Match
	found := false
	for _, m := range ms {
		if m.Reference == name && (!found || m.Coverage > best.Coverage) {
			best, found = m, true
		}
	}
	return best, found
}

// firstMatch is where along the course the first match with name starts.
func firstMatch(ms []match.Match, name string) float64 {
	for _, m := range ms {
		if m.Reference == name {
			return m.From
		}
	}
	return math.Inf(1)
}

// extent is the rectangle the course and its references cover -- a reference
// that goes where the course did not is on the picture too -- with a few
// hundred metres round a course that stands still, one fix or a treadmill's
// worth of the same one, so it is a place on a map and not a point at zoom 15.
func extent(c *course.Course, refs []routemap.Reference) render.Bounds {
	b := render.Bounds{West: 180, South: 90, East: -180, North: -90}
	for _, p := range c.Points {
		b.West, b.East = min(b.West, p.Lon), max(b.East, p.Lon)
		b.South, b.North = min(b.South, p.Lat), max(b.North, p.Lat)
	}
	for _, r := range refs {
		for _, piece := range r.Drawn() {
			for _, p := range piece {
				b.West, b.East = min(b.West, p.Lon), max(b.East, p.Lon)
				b.South, b.North = min(b.South, p.Lat), max(b.North, p.Lat)
			}
		}
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
		Style: render.BasemapStyle(), Palette: p, Attribution: m.Attribution, Language: mapOpts.language,
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
