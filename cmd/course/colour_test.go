package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course/routemap"
)

// --colour pace colours a run slow in its first half and fast in its second
// at both ends of the scale, says the ends' paces, and draws the legend's
// bar; --colour elevation does the same by height. A metric the course does
// not have, one that does not exist, and --colour with --compare are
// refused before anything is drawn.
func TestMapColour(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run1, plan, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "plan.gpx"), filepath.Join(dir, "run.png")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	sec := 0
	for i := 0; i < 100; i++ {
		// 22 m a point: 6 s a point for the first half, 4 s after.
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%d</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), i, stamp(sec))
		if i < 50 {
			sec += 6
		} else {
			sec += 4
		}
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, run1, b.String())
	writeFile(t, plan, `<gpx><trk><trkseg><trkpt lat="10" lon="20"></trkpt><trkpt lat="10" lon="20.02"></trkpt></trkseg></trk></gpx>`)
	oneHeight := filepath.Join(dir, "one.gpx")
	writeFile(t, oneHeight, `<gpx><trk><trkseg><trkpt lat="10" lon="20"><ele>5</ele><time>`+stamp(0)+`</time></trkpt><trkpt lat="10" lon="20.02"><time>`+stamp(600)+`</time></trkpt></trkseg></trk></gpx>`)
	store := filepath.Join(dir, "store")

	for _, c := range []struct {
		metric, report string
	}{{"pace", "by pace, 4:3"}, {"elevation", "by elevation, 5 m (blue) to 94 m (red)"}} {
		resetNow(mapCmd)
		o, err := run(t, "map", "--store", store, "--out", out, "--width", "600", "--height", "400", "--colour", c.metric, run1)
		if err != nil {
			t.Fatalf("--colour %s: %v\n%s", c.metric, err, o)
		}
		if !strings.Contains(o, c.report) {
			t.Errorf("--colour %s reports:\n%s\nwant %q", c.metric, o, c.report)
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
		// The slow or low end in the west half of the course, the fast or
		// high end in the east, and both in the legend's bar at the top.
		found := func(c [4]uint8, x0, x1, y0, y1 int) bool {
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, bl, _ := img.At(x, y).RGBA()
					if near(r>>8, uint32(c[0])) && near(g>>8, uint32(c[1])) && near(bl>>8, uint32(c[2])) {
						return true
					}
				}
			}
			return false
		}
		lo, hi := render.DefaultColours[0], render.DefaultColours[len(render.DefaultColours)-1]
		blue, red := [4]uint8{lo.R, lo.G, lo.B}, [4]uint8{hi.R, hi.G, hi.B}
		if !found(blue, 0, 300, 100, 400) || found(blue, 320, 600, 100, 400) {
			t.Errorf("--colour %s: the blue end is not only in the west half", c.metric)
		}
		if !found(red, 300, 600, 100, 400) || found(red, 0, 280, 100, 400) {
			t.Errorf("--colour %s: the red end is not only in the east half", c.metric)
		}
		if !found(blue, 0, 600, 0, 60) || !found(red, 0, 600, 0, 60) {
			t.Errorf("--colour %s: no colour bar in the legend", c.metric)
		}
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--colour", "pace", plan}, "no times"},
		{[]string{"--colour", "elevation", plan}, "no elevation"},
		{[]string{"--colour", "grade", plan}, "no elevation"},
		{[]string{"--colour", "grade", oneHeight}, "too little elevation"},
		{[]string{"--colour", "power", run1}, "pace, elevation or grade"},
		{[]string{"--colour", "pace", "--compare", run1, run1}, "use one"},
	} {
		resetNow(mapCmd)
		_, err := run(t, append([]string{"map", "--store", store, "--out", out}, c.args...)...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("map %v: %v, want an error saying %q", c.args, err, c.want)
		}
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Error("a store was made for a map that was refused")
	}
}

func TestPaceText(t *testing.T) {
	for speed, want := range map[float64]string{1000.0 / 300: "5:00/km", 1000.0 / 359.6: "6:00/km", 0: "-"} {
		if got := paceText(speed); got != want {
			t.Errorf("paceText(%v) = %q, want %q", speed, got, want)
		}
	}
}

// Where a coloured course has nothing to colour it by -- here the cool-down
// after the stretch compared -- it is drawn in the palette's grey, not in the
// course's own ink, which would read as a colour on the scale.
func TestMapCompareGreysWhatIsNotCompared(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run1, ref, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "ref.gpx"), filepath.Join(dir, "run.png")
	writeLine(t, ref, 10, 20, 0.0002, 50)
	writeLine(t, run1, 10, 20, 0.0002, 100) // the reference, then as far again
	if o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", out, "--width", "1200", "--height", "900", "--compare", ref, run1); err != nil {
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
	palette, overlay, err := paletteNamed("light")
	if err != nil {
		t.Fatal(err)
	}
	inks := routemap.InksFor(palette, overlay)
	var grey, own bool
	for y := 300; y < 700; y++ {
		for x := 800; x < 1050; x++ { // the cool-down, clear of the finish
			r, g, b, _ := img.At(x, y).RGBA()
			is := func(c [3]uint8) bool {
				return near(r>>8, uint32(c[0])) && near(g>>8, uint32(c[1])) && near(b>>8, uint32(c[2]))
			}
			grey = grey || is([3]uint8{inks.Gap.R, inks.Gap.G, inks.Gap.B})
			own = own || is([3]uint8{inks.Route.R, inks.Route.G, inks.Route.B})
		}
	}
	if !grey || own {
		t.Errorf("the cool-down: grey %v, the course's own ink %v; want grey only", grey, own)
	}
}

// A flat course is not coloured over its barometer's drift, nor an even run
// over a few seconds a kilometre: each is shown over a least range about its
// middle.
func TestMapColourOfAnEvenFlatRun(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run1, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "run.png")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%d</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), 10+i%2, stamp(5*i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, run1, b.String())
	for metric, want := range map[string]string{
		"elevation": "0 m (blue) to 20 m (red)",
		// 21.9 m in 5 s, 3:48/km: 5% of the speed either side is 4:00
		// and 3:37.
		"pace": "4:00/km (blue) to 3:37/km (red)",
		// Level ground: the grade scale still reaches 3% either way.
		"grade": "-3.0% (blue) to +3.0% (red)",
	} {
		resetNow(mapCmd)
		o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", out, "--width", "300", "--height", "200", "--colour", metric, run1)
		if err != nil || !strings.Contains(o, want) {
			t.Errorf("--colour %s of an even, flat run: %v\n%s\nwant %q", metric, err, o, want)
		}
	}
}

// --colour grade colours a climb red and a descent blue, level ground in the
// middle of the scale; a course steeper than 15% has the scale stop there,
// and the legend and report say the ends are "this steep or steeper", as
// videofx writes a grade. A gentler course's scale reaches only its own
// steepest, with no such marks.
func TestMapColourGrade(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run1, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "run.png")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 100; i++ {
		// 22 m a point: up 4.4 m a point for the first half, 20%, then
		// down as steeply -- past the scale's end both ways.
		ele := 4.4 * float64(min(i, 99-i))
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%.1f</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), ele, stamp(10*i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, run1, b.String())

	o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", out, "--width", "600", "--height", "400", "--colour", "grade", run1)
	if err != nil {
		t.Fatalf("--colour grade: %v\n%s", err, o)
	}
	if !strings.Contains(o, "by grade, ≤-15.0% (blue) to ≥+15.0% (red)") {
		t.Errorf("--colour grade reports:\n%s", o)
	}
	img := decodePNG(t, out)

	gentle := filepath.Join(dir, "gentle.gpx")
	var g strings.Builder
	g.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 100; i++ {
		// Up at 4% for two thirds, down at 8%: the scale reaches the
		// steeper of the two either way.
		ele := 0.88 * float64(min(i, 132-2*i))
		fmt.Fprintf(&g, `<trkpt lat="10" lon="%s"><ele>%.2f</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), ele, stamp(10*i))
	}
	g.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, gentle, g.String())
	resetNow(mapCmd)
	o, err = run(t, "map", "--store", filepath.Join(dir, "store"), "--out", filepath.Join(dir, "gentle.png"), "--width", "600", "--height", "400", "--colour", "grade", gentle)
	var reach float64
	if i := strings.Index(o, "by grade, -"); err != nil || i < 0 || strings.Contains(o, "≤") {
		t.Errorf("--colour grade of a gentle hill: %v\n%s", err, o)
	} else if fmt.Sscanf(o[i+len("by grade, -"):], "%f%%", &reach); reach < 7 || reach > 8.5 {
		t.Errorf("--colour grade of a hill up at 4%% and down at 8%% reaches %v%%, want about 8:\n%s", reach, o)
	}

	lo, hi := render.DefaultColours[0], render.DefaultColours[len(render.DefaultColours)-1]
	blue, red := [3]uint8{lo.R, lo.G, lo.B}, [3]uint8{hi.R, hi.G, hi.B}
	if !hasColour(img, red, 50, 250, 100, 400) || hasColour(img, blue, 50, 250, 100, 400) {
		t.Error("the climb, in the west, is not red")
	}
	if !hasColour(img, blue, 350, 550, 100, 400) || hasColour(img, red, 350, 550, 100, 400) {
		t.Error("the descent, in the east, is not blue")
	}
}

// A staircase on a long level course -- 22 m at -35%, walked at 1.8 m a
// second -- is still blue at the whole course's zoom, and reads its
// steepness: taken over 30 m either side rather than 10 it would read about
// half as steep, and averaged into a piece of the line with the level ground
// either side it would be drawn a gentle green. A fix every second, as a
// watch records.
func TestMapColourGradeShowsAStaircase(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run1, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "run.png")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	x, ele := 0.0, 20.0
	for i := 0; i < 3000; i++ {
		step := 3.0
		if i >= 1500 && i < 1512 {
			step, ele = 1.8, ele-0.63 // down the stairs
		}
		x += step
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%.2f</ele><time>%s</time></trkpt>`, ftoa6(20+x/109_600), ele, stamp(i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, run1, b.String())

	o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", out, "--width", "300", "--height", "200", "--colour", "grade", run1)
	if err != nil {
		t.Fatalf("--colour grade: %v\n%s", err, o)
	}
	// Past the scale's 15% cap, which the report marks; over 30 m either
	// side it reads about 11%.
	if !strings.Contains(o, "by grade, ≤-15.0% (blue)") {
		t.Errorf("the staircase does not read 15%% or steeper:\n%s", o)
	}
	img := decodePNG(t, out)
	// Around the stairs, halfway along.
	bluest := 0
	for y := 60; y < 200; y++ {
		for x := 110; x < 190; x++ {
			r, _, b, _ := img.At(x, y).RGBA()
			bluest = max(bluest, int(b>>8)-int(r>>8))
		}
	}
	if bluest < 150 {
		t.Errorf("the staircase is nowhere drawn blue: at most %d more blue than red", bluest)
	}
}

func decodePNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// hasColour reports whether any pixel in x0-x1, y0-y1 is c, within a little
// for antialiasing.
func hasColour(img image.Image, c [3]uint8, x0, x1, y0, y1 int) bool {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if near(r>>8, uint32(c[0])) && near(g>>8, uint32(c[1])) && near(b>>8, uint32(c[2])) {
				return true
			}
		}
	}
	return false
}
