package main

import (
	"fmt"
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

// --colour grade colours a descent blue and the gentler climb before it on
// the warm side, on a scale with level ground in its middle reaching as far
// as the steeper of the two either way, and writes the ends as videofx
// writes a grade.
func TestMapColourGrade(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run1, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "run.png")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 100; i++ {
		// 22 m a point: up 1 m a point for two thirds, a 4.5% climb, then
		// down 2 m a point, 9%.
		ele := min(i, 66-2*(i-66))
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%d</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), ele, stamp(5*i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, run1, b.String())

	o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", out, "--width", "600", "--height", "400", "--colour", "grade", run1)
	if err != nil {
		t.Fatalf("--colour grade: %v\n%s", err, o)
	}
	// The descent's 9%, a little less once smoothed.
	if !strings.Contains(o, "by grade, -8.") || !strings.Contains(o, "(blue) to +8.") {
		t.Errorf("--colour grade reports:\n%s\nwant about -8.5%% to +8.5%%", o)
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
	has := func(c [3]uint8, x0, x1 int) bool {
		for y := 100; y < 400; y++ {
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
	blue, red := [3]uint8{lo.R, lo.G, lo.B}, [3]uint8{hi.R, hi.G, hi.B}
	if has(blue, 50, 350) || has(red, 50, 350) {
		t.Error("the climb, in the west, is at an end of the scale; want it halfway up the warm side")
	}
	if !has(blue, 440, 550) || has(red, 440, 550) {
		t.Error("the descent, in the east, is not blue")
	}
}

// A short steep pitch on a long level course sets the grade scale's reach:
// a 75 m ramp at 14% is under 2% of the course, and a scale leaving out the
// most extreme few per cent would draw it the same red as the gentlest
// slope. Recorded as a watch records, a fix every 3 m.
func TestMapColourGradeReachesTheSteepestPitch(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run1, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "run.png")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	ele := 10.0
	for i := 0; i < 1500; i++ {
		if i >= 700 && i < 725 {
			ele += 0.42 // 14% over 3 m
		}
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%.2f</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.000027), ele, stamp(i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, run1, b.String())

	o, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", out, "--width", "300", "--height", "200", "--colour", "grade", run1)
	if err != nil {
		t.Fatalf("--colour grade: %v\n%s", err, o)
	}
	i := strings.Index(o, "(blue) to +")
	var reach float64
	if i < 0 {
		t.Fatalf("no grade range reported:\n%s", o)
	}
	if _, err := fmt.Sscanf(o[i+len("(blue) to +"):], "%f%%", &reach); err != nil || reach < 10 || reach > 15 {
		t.Errorf("the grade scale reaches %v%%, want the ramp's 14%% less a little smoothing:\n%s", reach, o)
	}
}
