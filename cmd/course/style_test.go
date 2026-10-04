package main

import (
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/fittest"
	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
	"github.com/wisborg/course/mapstyle"
	"github.com/wisborg/course/routemap"
)

// The style is made in layers, each changing only what it says: the
// defaults, the --style file, --palette, and every --set in turn.
func TestMapStyleLayers(t *testing.T) {
	resetFlags(t, mapStyleCmd)
	t.Cleanup(func() { styleOpts.file, styleOpts.sets = "", nil })
	dir := t.TempDir()
	theme := filepath.Join(dir, "theme.yaml")

	writeFile(t, theme, "palette: dark\nwidth: 2000\nheight: 2000\ncourse:\n  colour: '#d32f2f'\n  width: 5\nreference:\n  style: dotted\n")
	out, err := run(t, "map", "style", "--style", theme, "--palette", "light", "--width", "800", "--set", "course.width=7", "--set", "references.Loop.colour=#0077aa")
	if err != nil {
		t.Fatalf("map style: %v\n%s", err, out)
	}
	st := mapstyle.Default()
	if err := st.Read(strings.NewReader(out)); err != nil {
		t.Fatalf("what map style printed does not read back: %v\n%s", err, out)
	}
	// --width over the file's, the file's height kept.
	if st.Width != 800 || st.Height != 2000 {
		t.Errorf("size %d by %d, want 800 by 2000", st.Width, st.Height)
	}
	if st.Palette != "light" || st.Course.Colour != "#d32f2f" || *st.Course.Width != 7 || st.Reference.Style != "dotted" || st.References["Loop"].Colour != "#0077aa" || *st.Reference.Width != 2.25 {
		t.Errorf("layered style %+v", st)
	}
	for _, want := range []string{"# The map's colours", "opacity: auto", "colours: [] # on the light palette"} {
		if !strings.Contains(out, want) {
			t.Errorf("map style printed no %q:\n%s", want, out)
		}
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"map", "style", "--set", "course.colur=red"}, `no such setting "colur"`},
		{[]string{"map", "style", "--set", "course.colour=red"}, "course.colour"},
		{[]string{"map", "style", "--style", filepath.Join(dir, "missing.yaml")}, "missing.yaml"},
		{[]string{"map", "style", "--palette", "sepia"}, "palette"},
		{[]string{"map", "style", "--width", "20"}, "width: 20 pixels is too small"},
	} {
		resetNow(mapStyleCmd)
		resetNow(mapCmd)
		styleOpts.file, styleOpts.sets = "", nil
		if _, err := run(t, c.args...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want an error saying %q", c.args[1:], err, c.want)
		}
	}
}

// A style is made concrete for a map: each activity's ink -- its own, the
// course's, the course's list in turn, or the palette's -- and each
// reference's -- by name, one for all, a list in turn, or the palette's --
// with an auto opacity 0.7 unless a reference is drawn whole, and a halo on
// an opaque line only.
func TestResolve(t *testing.T) {
	p, o := render.LightPalette(), render.LightOverlay()
	own := routemap.ActivityInks(p, o)
	refs := []routemap.Reference{{Name: "A", Follows: true}, {Name: "B", Follows: true}, {Name: "C", Follows: true}}

	st := mapstyle.Default()
	d := resolve(st, 3, refs, p, o)
	if d.actInks[0] != own[0] || d.actInks[2] != own[2] || d.refInks[1] != routemap.ReferenceInks(p)[1] {
		t.Errorf("auto inks %v %v", d.actInks, d.refInks)
	}
	if l := d.actLooks[0]; l.Width != 3 || l.Opacity != 0.7 || l.Halo != 0 || l.Pattern != routemap.Solid {
		t.Errorf("default course look %+v", l)
	}
	if l := d.refLooks[0]; l.Width != 2.25 || l.Pattern != routemap.Dashed || !l.Beside || d.actLooks[0].Beside {
		t.Errorf("default reference look %+v", l)
	}

	for _, set := range []string{
		"course.colour=#010203", "course.colours=[#0a0b0c, #0d0e0f]", "activities.3.colour=#111111", "activities.2.width=9",
		"reference.colours=[#202020, #303030]", "references.B.colour=#404040", "references.C.style=dotted", "references.C.opacity=0.5",
		"reference.beside=false", "references.B.beside=true",
	} {
		if err := st.Set(set); err != nil {
			t.Fatal(err)
		}
	}
	whole := append(refs, routemap.Reference{Name: "D"}) // D is drawn whole
	d = resolve(st, 4, whole, p, o)
	wantActs := []color.RGBA{hex("#010203"), hex("#0a0b0c"), hex("#111111"), hex("#0a0b0c")}
	for i, w := range wantActs {
		if d.actInks[i] != w {
			t.Errorf("activity %d ink %v, want %v", i+1, d.actInks[i], w)
		}
	}
	if d.actLooks[1].Width != 9 || d.actLooks[0].Width != 3 {
		t.Errorf("activity widths %v and %v, want 3 and 9", d.actLooks[0].Width, d.actLooks[1].Width)
	}
	wantRefs := []color.RGBA{hex("#202020"), hex("#404040"), hex("#202020"), hex("#303030")}
	for i, w := range wantRefs {
		if d.refInks[i] != w {
			t.Errorf("reference %d ink %v, want %v", i, d.refInks[i], w)
		}
	}
	if l := d.actLooks[0]; l.Opacity != 1 || l.Halo != 1 {
		t.Errorf("with a reference drawn whole, the course %+v; want opaque with a halo", l)
	}
	if l := d.refLooks[2]; l.Opacity != 0.5 || l.Halo != 0 || l.Pattern != routemap.Dotted {
		t.Errorf("reference C %+v; want its own 0.5, no halo, dotted", l)
	}
	if d.refLooks[0].Beside || !d.refLooks[1].Beside {
		t.Errorf("beside: A %v, B %v; want false for all, true for B by name", d.refLooks[0].Beside, d.refLooks[1].Beside)
	}
	st.Reference.Colour = "#505050"
	if d := resolve(st, 1, refs, p, o); d.refInks[0] != hex("#505050") || d.refInks[1] != hex("#404040") {
		t.Errorf("one colour for every reference, B by name: %v", d.refInks)
	}
}

// A colour is #rrggbb, or #rrggbbaa premultiplied as color.RGBA is.
func TestHex(t *testing.T) {
	if got := hex("#d32f2f"); got != (color.RGBA{R: 0xd3, G: 0x2f, B: 0x2f, A: 0xff}) {
		t.Errorf("#d32f2f is %v", got)
	}
	if got := hex("#ff000080"); got != (color.RGBA{R: 0x80, A: 0x80}) {
		t.Errorf("#ff000080 is %v, want red at half, premultiplied", got)
	}
}

// A map is drawn in the style the flags make: --set course.colour and an
// opaque course draws the course in exactly that colour.
func TestMapDrawsInItsStyle(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)
	t.Cleanup(func() { styleOpts.file, styleOpts.sets = "", nil })

	dir := t.TempDir()
	run1, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "run.png")
	writeLine(t, run1, 10, 20, 0.0002, 50)
	if o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", out, "--width", "800", "--height", "600",
		"--set", "course.colour=#00c853", "--set", "course.opacity=1", run1); err != nil {
		t.Fatalf("map: %v\n%s", err, o)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !hasColour(img, [3]uint8{0x00, 0xc8, 0x53}, 100, 700, 100, 600) {
		t.Error("the course is not in the colour set")
	}
}

// The picture is the size the style says: --set width and height make it,
// as --width and --height do.
func TestMapIsTheSizeOfItsStyle(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)
	t.Cleanup(func() { styleOpts.file, styleOpts.sets = "", nil })

	dir := t.TempDir()
	run1, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "run.png")
	writeLine(t, run1, 10, 20, 0.0002, 50)
	if o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", out, "--set", "width=320", "--set", "height=200", run1); err != nil {
		t.Fatalf("map: %v\n%s", err, o)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 320 || cfg.Height != 200 {
		t.Errorf("the picture is %d by %d, %v; want 320 by 200", cfg.Width, cfg.Height, err)
	}
}

// A style may colour every map: a theme colouring by grade colours a map
// with no --colour; --colour none, or --set colouring.by=none, undoes it;
// --grade-cap may go with a theme's grade. --compare with a style's
// colouring is refused, saying how to undo the style's.
func TestMapColouringFromItsStyle(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)
	t.Cleanup(func() { styleOpts.file, styleOpts.sets = "", nil })

	dir := t.TempDir()
	run1, out, theme := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "run.png"), filepath.Join(dir, "theme.yaml")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%d</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), min(i, 99-i), stamp(10*i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, run1, b.String())
	writeFile(t, theme, "colouring:\n  by: grade\n  grade-cap: 3\n")
	base := []string{"map", "--store", filepath.Join(dir, "store"), "--out", out, "--width", "300", "--height", "200", "--style", theme}

	for _, c := range []struct {
		args []string
		want string
		not  string
	}{
		{nil, "by grade, ≤-3.0% (blue) to ≥+3.0% (red)", ""},
		{[]string{"--grade-cap", "20"}, "by grade, -4.", "≥"},
		{[]string{"--colour", "none"}, "", "coloured"},
		{[]string{"--set", "colouring.by=none"}, "", "coloured"},
		{[]string{"--colour", "elevation"}, "by elevation", ""},
	} {
		resetNow(mapCmd)
		styleOpts.file, styleOpts.sets = "", nil
		o, err := run(t, append(append(base, c.args...), run1)...)
		if err != nil || !strings.Contains(o, c.want) || c.not != "" && strings.Contains(o, c.not) {
			t.Errorf("%v: %v\n%s\nwant %q and not %q", c.args, err, o, c.want, c.not)
		}
	}
	resetNow(mapCmd)
	styleOpts.file, styleOpts.sets = "", nil
	if _, err := run(t, append(base, "--compare", run1, run1)...); err == nil || !strings.Contains(err.Error(), "--set colouring.by=none") {
		t.Errorf("--compare over a style colouring by grade: %v", err)
	}
}

// A style's power source is the one a map is coloured by, with no
// --power-source on the command line.
func TestMapPowerSourceFromItsStyle(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)
	t.Cleanup(func() { styleOpts.file, styleOpts.sets = "", nil })

	dir := t.TempDir()
	both, theme := filepath.Join(dir, "both.fit"), filepath.Join(dir, "theme.yaml")
	opts := fittest.DefaultOptions()
	opts.Count, opts.PowerWatts, opts.DeveloperField = 600, 250, fitactivity.StrydPowerField
	if err := fittest.WriteFile(both, opts); err != nil {
		t.Fatal(err)
	}
	writeFile(t, theme, "colouring:\n  by: power\n  power-source: native\n")
	o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", filepath.Join(dir, "m.png"), "--width", "300", "--height", "200", "--style", theme, both)
	if err != nil || !strings.Contains(o, "by native power") {
		t.Errorf("a style colouring by native power: %v\n%s", err, o)
	}
}

// A halo is auto -- slim on an opaque line, none on a translucent one -- or
// the size given, translucent line or not; an activity or a reference may
// have its own.
func TestResolveHalo(t *testing.T) {
	p, o := render.LightPalette(), render.LightOverlay()
	refs := []routemap.Reference{{Name: "A", Follows: true}, {Name: "B", Follows: true}}
	st := mapstyle.Default()
	for _, set := range []string{"course.halo=2.5", "references.B.halo=0.5", "reference.opacity=1"} {
		if err := st.Set(set); err != nil {
			t.Fatal(err)
		}
	}
	d := resolve(st, 1, refs, p, o)
	if d.actLooks[0].Halo != 2.5 || d.actLooks[0].Opacity != 0.7 {
		t.Errorf("course %+v; want its own 2.5 halo, translucent", d.actLooks[0])
	}
	if d.refLooks[0].Halo != 1 || d.refLooks[1].Halo != 0.5 {
		t.Errorf("references' halos %v and %v; want auto's 1 on an opaque line, and B's own 0.5", d.refLooks[0].Halo, d.refLooks[1].Halo)
	}
}

// The map's own names are 13 pixels at any size unless a size is given,
// which is scaled with the picture; markers are every so many of the
// distance unit, auto, or none, and numbered in it.
func TestMapLabelSizeAndMarkerEvery(t *testing.T) {
	st := mapstyle.Default()
	if got := mapLabelSize(st, 2.4); got != 13 {
		t.Errorf("auto at scale 2.4: %v, want 13", got)
	}
	st.Map.LabelSize = "9"
	if got := mapLabelSize(st, 2.4); math.Abs(got-21.6) > 1e-9 {
		t.Errorf("9 at scale 2.4: %v, want %v", got, 9*2.4)
	}
	metric, _ := units.Of(units.Metric)
	imperial, _ := units.Of(units.Imperial)
	for _, c := range []struct {
		every string
		u     units.Set
		want  routemap.Spacing
	}{
		{"auto", metric, routemap.Spacing{Every: 0, Unit: 1000}},
		{"none", metric, routemap.Spacing{Every: -1, Unit: 1000}},
		{"2.5", metric, routemap.Spacing{Every: 2500, Unit: 1000}},
		{"2", imperial, routemap.Spacing{Every: 2 * 1609.344, Unit: 1609.344}},
	} {
		st.Markers.Every = c.every
		if got := markerSpacing(st, c.u); math.Abs(got.Every-c.want.Every) > 1e-9 || got.Unit != c.want.Unit {
			t.Errorf("markers every %s in %s: %+v, want %+v", c.every, c.u.Distance.Name, got, c.want)
		}
	}
}

// The new settings reach the map: markers as often as asked, or none; the
// coloured line as wide as asked; the text, and so the legend, as large.
func TestMapDrawsTheNewSettings(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)
	t.Cleanup(func() { styleOpts.file, styleOpts.sets = "", nil })

	dir := t.TempDir()
	fit := filepath.Join(dir, "run.fit")
	opts := fittest.DefaultOptions()
	opts.Count = 2000 // 6 km at 3 m/s
	if err := fittest.WriteFile(fit, opts); err != nil {
		t.Fatal(err)
	}
	draw := func(sets ...string) string {
		t.Helper()
		resetNow(mapCmd)
		styleOpts.file, styleOpts.sets = "", nil
		args := []string{"map", "--store", filepath.Join(dir, "store"), "--out", filepath.Join(dir, "m.png"), "--width", "600", "--height", "400"}
		for _, s := range sets {
			args = append(args, "--set", s)
		}
		o, err := run(t, append(args, fit)...)
		if err != nil {
			t.Fatalf("map %v: %v\n%s", sets, err, o)
		}
		return o
	}
	for _, c := range []struct {
		set  []string
		want string
	}{
		{nil, "5 distance markers"},
		{[]string{"markers.every=2"}, "2 distance markers"},
		{[]string{"markers.every=none"}, "markers    none, as the style asks"},
	} {
		if o := draw(c.set...); !strings.Contains(o, c.want) {
			t.Errorf("%v: the report says\n%s\nwant %q", c.set, o, c.want)
		}
	}

	// The coloured line's width, in the gradient handed to the drawing.
	c, err := course.Read(fit)
	if err != nil {
		t.Fatal(err)
	}
	col, err := colourBy([]*course.Course{c}, "run", colourOptions{metric: "elevation", gradeCap: 15, power: "auto", width: 6}, 2)
	if err != nil || len(col.gradients) == 0 || col.gradients[0].Width != 12 {
		t.Errorf("a coloured line 6 wide at scale 2: %v, %+v", err, col)
	}

	// A larger text makes a larger legend.
	face1, face2 := faceAt(13), faceAt(26)
	e := []entry{{name: "A course with a long name", ink: color.RGBA{A: 0xff}}}
	if a, b := legendSize(e, face1, 1), legendSize(e, face2, 1); b.X <= a.X || b.Y <= a.Y {
		t.Errorf("legends at text 13 and 26: %v and %v", a, b)
	}
}
