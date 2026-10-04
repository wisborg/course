package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/fittest"
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
		{[]string{"--colour", "stride", run1}, `colouring.by: "stride" is not one; colour by pace, speed, grade-adjusted-pace, elevation, grade, heart-rate, power, air-power, cadence, temperature, humidity, or none`},
		{[]string{"--colour", "pace", "--grade-cap", "25", run1}, "--grade-cap is for --colour grade"},
		{[]string{"--colour", "grade", "--grade-cap", "0", run1}, "more than 0"},
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
			// The lines are drawn light, translucent over the ground.
			grey = grey || is(onGround(inks.Gap))
			own = own || is(onGround(inks.Route))
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

	// With the cap raised past the hill's 20%, the scale reaches the
	// hill's own steepest and the ends are no longer "or steeper".
	resetNow(mapCmd)
	o, err = run(t, "map", "--store", filepath.Join(dir, "store"), "--out", filepath.Join(dir, "capped.png"), "--width", "600", "--height", "400", "--colour", "grade", "--grade-cap", "30", run1)
	if err != nil || strings.Contains(o, "≥") || !strings.Contains(o, "by grade, -") {
		t.Errorf("--grade-cap 30 on a 20%% hill: %v\n%s", err, o)
	} else if i := strings.Index(o, "(blue) to +"); i >= 0 {
		var reach float64
		if fmt.Sscanf(o[i+len("(blue) to +"):], "%f%%", &reach); reach < 17 || reach > 21 {
			t.Errorf("--grade-cap 30 on a 20%% hill reaches %v%%, want about 20:\n%s", reach, o)
		}
	}
	// And lowered under it, the scale stops at the lower cap.
	resetNow(mapCmd)
	o, err = run(t, "map", "--store", filepath.Join(dir, "store"), "--out", filepath.Join(dir, "capped.png"), "--width", "600", "--height", "400", "--colour", "grade", "--grade-cap", "10", run1)
	if err != nil || !strings.Contains(o, "by grade, ≤-10.0% (blue) to ≥+10.0% (red)") {
		t.Errorf("--grade-cap 10 on a 20%% hill: %v\n%s", err, o)
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

// --colour heart-rate and --colour power colour a recording by its own
// readings; power by the sensor --power-source picks, the same three ways as
// videofx and fitdash, and says which in the report. Asking for a sensor or
// a metric the file does not have, a --power-source that is not one, or one
// without --colour power, is refused before anything is drawn.
func TestMapColourHeartRateAndPower(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	both, native, bare := filepath.Join(dir, "both.fit"), filepath.Join(dir, "native.fit"), filepath.Join(dir, "bare.gpx")
	opts := fittest.DefaultOptions()
	opts.Count, opts.PowerWatts, opts.DeveloperField = 600, 250, fitactivity.StrydPowerField
	if err := fittest.WriteFile(both, opts); err != nil {
		t.Fatal(err)
	}
	opts.DeveloperField = ""
	if err := fittest.WriteFile(native, opts); err != nil {
		t.Fatal(err)
	}
	writeLine(t, bare, 10, 20, 0.0002, 50)
	jumpy := filepath.Join(dir, "jumpy.gpx")
	var b strings.Builder
	b.WriteString(`<gpx xmlns:gpxtpx="http://www.garmin.com/xmlschemas/TrackPointExtension/v1"><trk><trkseg>`)
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><time>%s</time><extensions><gpxtpx:TrackPointExtension><gpxtpx:hr>%d</gpxtpx:hr></gpxtpx:TrackPointExtension></extensions></trkpt>`,
			ftoa6(20+float64(i)*0.00003), stamp(i), 130+20*(i%2))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, jumpy, b.String())
	store, out := filepath.Join(dir, "store"), filepath.Join(dir, "map.png")

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--colour", "heart-rate", both}, "by heart rate, 139 bpm (blue) to 149 bpm (red)"},
		{[]string{"--colour", "power", both}, "by Stryd power, "},
		{[]string{"--colour", "power", "--power-source", "stryd", both}, "by Stryd power, "},
		// 250 W throughout, shown over 5% either side.
		{[]string{"--colour", "power", "--power-source", "native", both}, "by native power, 238 W (blue) to 262 W (red)"},
		{[]string{"--colour", "power", native}, "by native power, 238 W"},
		// An optical sensor's second-to-second jumps between 130 and 150
		// are averaged out: the run was at 140 throughout.
		{[]string{"--colour", "heart-rate", jumpy}, "by heart rate, 135 bpm (blue) to 145 bpm (red)"},
	} {
		resetNow(mapCmd)
		o, err := run(t, append([]string{"map", "--store", store, "--out", out, "--width", "300", "--height", "200"}, c.args...)...)
		if err != nil || !strings.Contains(o, c.want) {
			t.Errorf("map %v: %v\n%s\nwant %q", c.args, err, o, c.want)
		}
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--colour", "heart-rate", bare}, "no heart rate"},
		{[]string{"--colour", "power", bare}, "no power"},
		{[]string{"--colour", "power", "--power-source", "stryd", native}, "no stryd power; --power-source auto"},
		{[]string{"--colour", "power", "--power-source", "strid", both}, `colouring.power-source: "strid" is invalid; use auto, stryd, or native`},
		{[]string{"--colour", "heart-rate", "--power-source", "native", both}, "--power-source is for --colour power"},
	} {
		resetNow(mapCmd)
		_, err := run(t, append([]string{"map", "--store", store, "--out", out}, c.args...)...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("map %v: %v, want an error saying %q", c.args, err, c.want)
		}
	}
}

// --colour cadence, air-power and grade-adjusted-pace colour a recording by
// its steps, by the wind a footpod felt, and by its pace as on level ground;
// each refused on a file without what it needs.
func TestMapColourCadenceAirPowerAndGradeAdjustedPace(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	stryd, plain, bare := filepath.Join(dir, "stryd.fit"), filepath.Join(dir, "plain.fit"), filepath.Join(dir, "bare.gpx")
	opts := fittest.DefaultOptions()
	opts.Count, opts.DeveloperField = 600, fitactivity.StrydPowerField
	opts.DeveloperFields = []string{fitactivity.StrydAirPowerField}
	if err := fittest.WriteFile(stryd, opts); err != nil {
		t.Fatal(err)
	}
	opts.DeveloperField, opts.DeveloperFields = "", nil
	if err := fittest.WriteFile(plain, opts); err != nil {
		t.Fatal(err)
	}
	writeLine(t, bare, 10, 20, 0.0002, 50)
	// A still day: air power of 1-5 W, which the scale shows over at
	// least 10 W rather than calling the difference wind.
	still := filepath.Join(dir, "still.fit")
	opts.DeveloperField, opts.DeveloperFields, opts.DeveloperFieldScale = fitactivity.StrydAirPowerField, nil, 100
	if err := fittest.WriteFile(still, opts); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(dir, "plan.gpx")
	writeFile(t, plan, `<gpx><rte><rtept lat="10" lon="20"><ele>5</ele></rtept><rtept lat="10" lon="20.01"><ele>9</ele></rtept></rte></gpx>`)
	// A metronome's 85 rpm, with no sport: shown over at least 10 either way.
	even := filepath.Join(dir, "even.gpx")
	var b strings.Builder
	b.WriteString(`<gpx xmlns:gpxtpx="http://www.garmin.com/xmlschemas/TrackPointExtension/v1"><trk><trkseg>`)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><time>%s</time><extensions><gpxtpx:TrackPointExtension><gpxtpx:cad>85</gpxtpx:cad></gpxtpx:TrackPointExtension></extensions></trkpt>`,
			ftoa6(20+float64(i)*0.0001), stamp(4*i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, even, b.String())
	// Ten minutes stood still halfway, a fix a second: by points a sixth
	// of the run, by ground none of it, so the slow end is the running.
	stop := filepath.Join(dir, "stop.gpx")
	b.Reset()
	b.WriteString(`<gpx><trk><trkseg>`)
	sec := 0
	for i := 0; i < 400; i++ {
		fixes := 1
		if i == 200 {
			fixes = 600
		}
		for range fixes {
			fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%.1f</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.00003), 10+float64(i%50)/10, stamp(sec))
			sec++
		}
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, stop, b.String())
	store, out := filepath.Join(dir, "store"), filepath.Join(dir, "map.png")

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--colour", "air-power", still}, "by air power, -2 W (blue) to 8 W (red)"},
		{[]string{"--colour", "cadence", even}, "by cadence, 80 rpm (blue) to 90 rpm (red)"},
		{[]string{"--colour", "grade-adjusted-pace", stop}, "by grade-adjusted pace, 6:02/km (blue)"},
		{[]string{"--colour", "pace", stop}, "by pace, 5:20/km (blue)"},
		// The fixture runs at 82 rpm ±3, counted one leg: steps a minute
		// are twice it.
		{[]string{"--colour", "cadence", plain}, "by cadence, 16"},
		{[]string{"--colour", "cadence", plain}, " spm (red)"},
		{[]string{"--colour", "air-power", stryd}, "by air power, "},
		// 3 m/s up and down the fixture's gentle hills.
		{[]string{"--colour", "grade-adjusted-pace", plain}, "by grade-adjusted pace, "},
	} {
		resetNow(mapCmd)
		o, err := run(t, append([]string{"map", "--store", store, "--out", out, "--width", "300", "--height", "200"}, c.args...)...)
		if err != nil || !strings.Contains(o, c.want) {
			t.Errorf("map %v: %v\n%s\nwant %q", c.args, err, o, c.want)
		}
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--colour", "cadence", bare}, "no cadence"},
		{[]string{"--colour", "air-power", plain}, "no air power"},
		{[]string{"--colour", "grade-adjusted-pace", bare}, "no elevation"},
		{[]string{"--colour", "grade-adjusted-pace", plan}, "no times"},
	} {
		resetNow(mapCmd)
		_, err := run(t, append([]string{"map", "--store", store, "--out", out}, c.args...)...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("map %v: %v, want an error saying %q", c.args, err, c.want)
		}
	}
}

// --colour temperature and humidity colour a recording by the air a footpod
// felt, or the watch's own temperature, in the units asked for; each refused
// on a file without what it needs.
func TestMapColourTemperatureAndHumidity(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetUnits(t)
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	stryd, bare := filepath.Join(dir, "stryd.fit"), filepath.Join(dir, "bare.gpx")
	opts := fittest.DefaultOptions()
	opts.Count = 300
	opts.DeveloperField, opts.DeveloperFieldScale = fitactivity.StrydTemperatureField, 10 // 10.0 to 39.9 °C
	opts.DeveloperFields = []string{fitactivity.StrydHumidityField}
	if err := fittest.WriteFile(stryd, opts); err != nil {
		t.Fatal(err)
	}
	writeLine(t, bare, 10, 20, 0.0002, 50)
	// A watch at 20 °C throughout, which the scale shows over 4 °C.
	watch := filepath.Join(dir, "watch.gpx")
	var b strings.Builder
	b.WriteString(`<gpx xmlns:gpxtpx="http://www.garmin.com/xmlschemas/TrackPointExtension/v1"><trk><trkseg>`)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><time>%s</time><extensions><gpxtpx:TrackPointExtension><gpxtpx:atemp>20</gpxtpx:atemp></gpxtpx:TrackPointExtension></extensions></trkpt>`,
			ftoa6(20+float64(i)*0.0001), stamp(4*i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, watch, b.String())
	// A humidity of 1 to 4% -- a footpod's raw values scaled by a hundred --
	// is too little to spend the ramp on, and is shown over 10%: -3 to 7.
	damp := filepath.Join(dir, "damp.fit")
	opts.DeveloperField, opts.DeveloperFieldScale, opts.DeveloperFields = fitactivity.StrydHumidityField, 100, nil
	if err := fittest.WriteFile(damp, opts); err != nil {
		t.Fatal(err)
	}
	store, out := filepath.Join(dir, "store"), filepath.Join(dir, "map.png")

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--colour", "temperature", stryd}, "by air (Stryd) temperature, "},
		{[]string{"--colour", "temperature", "--temperature-source", "stryd", stryd}, "by air (Stryd) temperature, "},
		{[]string{"--colour", "temperature", watch}, "by watch temperature, 18 °C (blue) to 22 °C (red)"},
		{[]string{"--colour", "temperature", "--units", "imperial", watch}, "by watch temperature, 64 °F (blue) to 72 °F (red)"},
		{[]string{"--colour", "temperature", "--unit", "temperature=F", watch}, "64 °F (blue)"},
		{[]string{"--colour", "humidity", stryd}, "by humidity, "},
		{[]string{"--colour", "humidity", damp}, "by humidity, -3% (blue) to 7% (red)"},
	} {
		resetNow(mapCmd)
		unitOpts.system, unitOpts.each = "metric", nil
		o, err := run(t, append([]string{"map", "--store", store, "--out", out, "--width", "300", "--height", "200"}, c.args...)...)
		if err != nil || !strings.Contains(o, c.want) {
			t.Errorf("map %v: %v\n%s\nwant %q", c.args, err, o, c.want)
		}
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--colour", "temperature", bare}, "no temperature"},
		{[]string{"--colour", "humidity", watch}, "no humidity, which a Stryd footpod measures"},
		{[]string{"--colour", "temperature", "--temperature-source", "native", stryd}, "no native temperature; --temperature-source auto"},
		{[]string{"--colour", "temperature", "--temperature-source", "wrist", stryd}, `colouring.temperature-source: "wrist" is invalid; use auto, stryd, or native`},
		{[]string{"--colour", "pace", "--temperature-source", "native", stryd}, "--temperature-source is for --colour temperature"},
	} {
		resetNow(mapCmd)
		unitOpts.system, unitOpts.each = "metric", nil
		_, err := run(t, append([]string{"map", "--store", store, "--out", out}, c.args...)...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("map %v: %v, want an error saying %q", c.args, err, c.want)
		}
	}
}
