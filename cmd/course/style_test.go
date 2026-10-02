package main

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/osmbase/render"

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
	writeFile(t, theme, "palette: dark\ncourse:\n  colour: '#d32f2f'\n  width: 5\nreference:\n  style: dotted\n")

	out, err := run(t, "map", "style", "--style", theme, "--palette", "light", "--set", "course.width=7", "--set", "references.Loop.colour=#0077aa")
	if err != nil {
		t.Fatalf("map style: %v\n%s", err, out)
	}
	st := mapstyle.Default()
	if err := st.Read(strings.NewReader(out)); err != nil {
		t.Fatalf("what map style printed does not read back: %v\n%s", err, out)
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
	if l := d.refLooks[0]; l.Width != 2.25 || l.Pattern != routemap.Dashed {
		t.Errorf("default reference look %+v", l)
	}

	for _, set := range []string{
		"course.colour=#010203", "course.colours=[#0a0b0c, #0d0e0f]", "activities.3.colour=#111111", "activities.2.width=9",
		"reference.colours=[#202020, #303030]", "references.B.colour=#404040", "references.C.style=dotted", "references.C.opacity=0.5",
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
