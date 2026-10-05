package main

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course/mapstyle"
	"github.com/wisborg/course/routemap"
)

// styleOpts are the flags that make a map's style: a file of settings, and
// settings one at a time. They are on map and on map style alike, so that
// map style prints exactly the style a map with the same flags is drawn in.
var styleOpts struct {
	file string
	sets []string
}

var mapStyleCmd = &cobra.Command{
	Use:   "style",
	Short: "Print the style a map would be drawn in, every setting with a comment",
	Long: `style prints, as YAML, the style course map draws in with the same --style,
--set and the flags that are settings of it -- --palette, --width, --height,
--units, --unit, --legend, --colour, --grade-cap, --power-source and --temperature-source: the built-in defaults, then the file's settings over
them, then the command line's. Every setting is there, with a comment saying
what it takes, so

  course map style > mytheme.yaml

starts a theme from the defaults, and

  course map style --style mytheme.yaml > mytheme.yaml.new

brings an old one up to the settings there are now. Use the result with
course map --style mytheme.yaml, and change one setting for one map with
--set, such as --set course.colour=#d32f2f.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := buildStyle(cmd)
		if err != nil {
			return err
		}
		return st.Write(cmd.OutOrStdout(), autoNotes(st))
	},
}

func init() {
	f := mapCmd.PersistentFlags()
	f.StringVar(&styleOpts.file, "style", "", "a style file, YAML or JSON, whose settings are drawn in over the defaults; course map style prints one to start from")
	f.StringArrayVar(&styleOpts.sets, "set", nil, "one style setting, path=value, over the defaults and --style: course.colour=#d32f2f, reference.style=dotted, activities.2.width=5, 'references.Rhodes parkrun.colour=#0077aa'; repeat for more, the last of one setting winning")
	mapCmd.AddCommand(mapStyleCmd)
}

// buildStyle is the style the flags make: the defaults, the --style file
// over them, then the command line -- --palette, --width, --height,
// --units, --unit, --legend, --colour, --grade-cap and --power-source, which
// are the same as --set palette=... and so on, and then every --set in turn
// -- and checked.
func buildStyle(cmd *cobra.Command) (mapstyle.Style, error) {
	st := mapstyle.Default()
	if styleOpts.file != "" {
		if err := st.Load(styleOpts.file); err != nil {
			return st, fmt.Errorf("--style %w", err)
		}
	}
	if cmd.Flags().Changed("palette") {
		st.Palette = mapOpts.palette
	}
	if cmd.Flags().Changed("width") {
		st.Width = mapOpts.width
	}
	if cmd.Flags().Changed("height") {
		st.Height = mapOpts.height
	}
	if cmd.Flags().Changed("units") {
		st.Units.System = unitOpts.system
	}
	for _, e := range unitOpts.each {
		q, name, err := splitUnit(e)
		if err != nil {
			return st, err
		}
		switch q {
		case units.Distance:
			st.Units.Distance = name
		case units.Elevation:
			st.Units.Elevation = name
		case units.Speed:
			st.Units.Speed = name
		case units.Pace:
			st.Units.Pace = name
		case units.Temperature:
			st.Units.Temperature = name
		default:
			return st, fmt.Errorf("--unit %s: %q is not a quantity with units; use distance, elevation, speed, pace or temperature", e, q)
		}
	}
	if cmd.Flags().Changed("legend") {
		st.Legend.Position = mapOpts.legend
	}
	if cmd.Flags().Changed("terrain") {
		st.Map.Terrain = mapOpts.terrain
	}
	if cmd.Flags().Changed("contours") {
		st.Map.Contours = mapOpts.contours
	}
	if cmd.Flags().Changed("colour") {
		st.Colouring.By = mapOpts.colour
	}
	if cmd.Flags().Changed("grade-cap") {
		st.Colouring.GradeCap = mapOpts.gradeCap
	}
	if cmd.Flags().Changed("power-source") {
		st.Colouring.PowerSource = mapOpts.power
	}
	if cmd.Flags().Changed("temperature-source") {
		st.Colouring.TemperatureSource = mapOpts.temperature
	}
	for _, s := range styleOpts.sets {
		if err := st.Set(s); err != nil {
			return st, fmt.Errorf("--set %w", err)
		}
	}
	if err := st.Validate(); err != nil {
		if styleOpts.file != "" {
			return st, fmt.Errorf("style, with --style %s: %w", styleOpts.file, err)
		}
		return st, fmt.Errorf("style: %w", err)
	}
	return st, nil
}

// drawn is a style made concrete for one map: the ink and look of each
// activity, in order, and of each reference.
type drawn struct {
	actInks  []color.RGBA
	actLooks []routemap.Look
	refInks  []color.RGBA
	refLooks []routemap.Look
}

// resolve makes st concrete for a map of n activities -- one, unless they
// are --separate -- and the references refs, on palette and overlay:
// the palette's own inks where st says auto, and an auto opacity resolved.
//
// An auto opacity is 0.7, so the map shows through the lines, but 1 when a
// reference is drawn whole beside the course: two translucent lines over one
// another mix into a third colour, and which is which is lost. An auto halo
// is a slim one on a line drawn opaque, auto or not, to hold it off the map,
// as an opaque line always had, and none on a translucent one, so as to hide
// no more of the map than its own width.
func resolve(st mapstyle.Style, n int, refs []routemap.Reference, p render.Palette, o render.Overlay) drawn {
	whole := false
	for _, r := range refs {
		whole = whole || !r.Follows
	}
	var d drawn
	ownActs := routemap.ActivityInks(p, o)
	for i := 0; i < n; i++ {
		line := st.Course
		if i < len(st.Activities) {
			line = over(line, st.Activities[i])
		}
		ink := ownActs[i%len(ownActs)]
		switch {
		case i < len(st.Activities) && st.Activities[i].Colour != "":
			ink = hex(st.Activities[i].Colour)
		case i == 0 && st.Course.Colour != "auto":
			ink = hex(st.Course.Colour)
		case i > 0 && len(st.Course.Colours) > 0:
			ink = hex(st.Course.Colours[(i-1)%len(st.Course.Colours)])
		}
		d.actInks = append(d.actInks, ink)
		d.actLooks = append(d.actLooks, look(line, whole))
	}
	ownRefs := routemap.ReferenceInks(p)
	for i, r := range refs {
		line := st.Reference
		named, ok := st.References[r.Name]
		if ok {
			line = over(line, named)
		}
		ink := ownRefs[i%len(ownRefs)]
		switch {
		case ok && named.Colour != "":
			ink = hex(named.Colour)
		case st.Reference.Colour != "auto":
			ink = hex(st.Reference.Colour)
		case len(st.Reference.Colours) > 0:
			ink = hex(st.Reference.Colours[i%len(st.Reference.Colours)])
		}
		d.refInks = append(d.refInks, ink)
		d.refLooks = append(d.refLooks, look(line, whole))
	}
	return d
}

// mapLabelSize is the size of the map's own names on a picture drawn at
// scale, in pixels: 13 at any size of picture, as they have always been,
// unless st gives a size, which is scaled with the picture as everything
// course draws is.
func mapLabelSize(st mapstyle.Style, scale float64) float64 {
	if st.Map.LabelSize == "auto" {
		return baseTextSize
	}
	n, _ := strconv.ParseFloat(st.Map.LabelSize, 64) // checked by Validate
	return n * scale
}

// markerSpacing is where st's distance markers go, as
// routemap.DistanceMarkersEvery takes it: every so many of the distance unit
// apart, in metres -- 0 for auto, below 0 for none -- and numbered in it.
func markerSpacing(st mapstyle.Style, u units.Set) routemap.Spacing {
	sp := routemap.Spacing{Unit: u.Distance.ToSI(1)}
	switch st.Markers.Every {
	case "auto":
	case "none":
		sp.Every = -1
	default:
		n, _ := strconv.ParseFloat(st.Markers.Every, 64) // checked by Validate
		sp.Every = u.Distance.ToSI(n)
	}
	return sp
}

// over is base with every setting o makes made.
func over(base, o mapstyle.Line) mapstyle.Line {
	if o.Width != nil {
		base.Width = o.Width
	}
	if o.Opacity != "" {
		base.Opacity = o.Opacity
	}
	if o.Style != "" {
		base.Style = o.Style
	}
	if o.Halo != "" {
		base.Halo = o.Halo
	}
	if o.Beside != nil {
		base.Beside = o.Beside
	}
	return base
}

// look is l as routemap draws it, with whole whether a reference is drawn
// whole on the map.
func look(l mapstyle.Line, whole bool) routemap.Look {
	opacity := 0.7
	if whole {
		opacity = 1
	}
	if l.Opacity != "auto" {
		opacity, _ = strconv.ParseFloat(l.Opacity, 64) // checked by Validate
	}
	halo := 0.0
	switch {
	case l.Halo != "auto":
		halo, _ = strconv.ParseFloat(l.Halo, 64) // checked by Validate
	case opacity >= 1:
		halo = 1
	}
	pattern := map[string]routemap.Pattern{"solid": routemap.Solid, "dashed": routemap.Dashed, "dotted": routemap.Dotted}[l.Style]
	return routemap.Look{Width: *l.Width, Opacity: opacity, Halo: halo, Pattern: pattern, Beside: l.Beside != nil && *l.Beside}
}

// hex is the colour s, #rrggbb or #rrggbbaa, which Validate has checked.
func hex(s string) color.RGBA {
	v, _ := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if len(s) == 7 {
		return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
	}
	// #rrggbbaa: color.RGBA is premultiplied.
	a := uint8(v)
	c := color.NRGBA{R: uint8(v >> 24), G: uint8(v >> 16), B: uint8(v >> 8), A: a}
	r, g, b, al := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(al >> 8)}
}

// autoNotes say what st's auto settings come to on its palette, for map
// style to write beside them.
func autoNotes(st mapstyle.Style) map[string]string {
	p, o, err := paletteNamed(st.Palette)
	if err != nil {
		return nil
	}
	hexOf := func(c color.RGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
	list := func(cs []color.RGBA) string {
		var out []string
		for _, c := range cs {
			out = append(out, hexOf(c))
		}
		return strings.Join(out, ", ")
	}
	notes := map[string]string{}
	if st.Course.Colour == "auto" {
		notes["course.colour"] = "on the " + st.Palette + " palette, " + hexOf(routemap.InksFor(p, o).Route)
	}
	if len(st.Course.Colours) == 0 {
		notes["course.colours"] = "on the " + st.Palette + " palette, " + list(routemap.ActivityInks(p, o)[1:])
	}
	if len(st.Reference.Colours) == 0 {
		notes["reference.colours"] = "on the " + st.Palette + " palette, " + list(routemap.ReferenceInks(p))
	}
	return notes
}
