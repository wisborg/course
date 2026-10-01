package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wisborg/fitactivity/fittest"

	"github.com/wisborg/course/routemap"
)

// A legend goes margin in from the corner asked for, and in the bottom right
// above the credit, never over it.
func TestLegendPlate(t *testing.T) {
	b := image.Rect(0, 0, 400, 300)
	size := image.Pt(100, 50)
	for corner, want := range map[string]image.Rectangle{
		"top-left":     image.Rect(10, 10, 110, 60),
		"top-right":    image.Rect(290, 10, 390, 60),
		"bottom-left":  image.Rect(10, 240, 110, 290),
		"bottom-right": image.Rect(290, 210, 390, 260), // 30 px of credit below
	} {
		if got := legendPlate(b, size, corner, 10, 30); got != want {
			t.Errorf("%s: %v, want %v", corner, got, want)
		}
	}
	// A legend larger than the picture is cut to it, not drawn off it.
	if got := legendPlate(b, image.Pt(500, 50), "top-left", 10, 0); got.Max.X != 400 {
		t.Errorf("a legend wider than the picture: %v", got)
	}
}

// Auto takes the corner that covers least of what is drawn; when two cover
// as little, the earlier of top left, top right, bottom left, bottom right.
func TestQuietestCorner(t *testing.T) {
	overlay := image.NewRGBA(image.Rect(0, 0, 400, 300))
	size := image.Pt(100, 50)
	if got := quietestCorner(overlay, size, 10, 0); got != "top-left" {
		t.Errorf("on nothing: %s, want top-left", got)
	}
	fill := func(r image.Rectangle) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				overlay.SetRGBA(x, y, color.RGBA{A: 0xff})
			}
		}
	}
	fill(image.Rect(0, 0, 150, 80))      // the start, top left
	fill(image.Rect(300, 0, 400, 20))    // a little top right
	fill(image.Rect(0, 250, 50, 300))    // a little more bottom left
	fill(image.Rect(290, 180, 400, 300)) // the finish, bottom right
	if got := quietestCorner(overlay, size, 10, 0); got != "top-right" {
		t.Errorf("got %s, want top-right, which covers least", got)
	}
}

// --legend puts the legend where it is told, or leaves it out; --title alone
// asks for one; and a place that is not one is refused.
func TestMapLegendFlags(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run1, ref, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "ref.gpx"), filepath.Join(dir, "run.png")
	writeLine(t, run1, 10, 20, 0.0002, 50)
	writeLine(t, ref, 10.001, 20, 0.0002, 50)
	store := filepath.Join(dir, "store")

	// lighter reports whether the pixel near corner is lighter than the
	// ground mid-picture: the legend's near-white plate is there.
	plated := func(img image.Image, x, y int) bool {
		lum := func(c color.Color) uint32 { r, g, b, _ := c.RGBA(); return r + g + b }
		return lum(img.At(x, y)) > lum(img.At(600, 850))+3000 // the ground, clear of everything
	}
	draw := func(args ...string) image.Image {
		t.Helper()
		resetNow(mapCmd)
		// Large enough that a legend has the size a real map's has.
		all := append([]string{"map", "--store", store, "--out", out, "--width", "1200", "--height", "900"}, args...)
		if o, err := run(t, append(all, run1)...); err != nil {
			t.Fatalf("map %v: %v\n%s", args, err, o)
		}
		f, err := os.Open(out)
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

	img := draw("--reference", ref, "--legend", "bottom-left")
	if !plated(img, 12, 880) || plated(img, 12, 12) {
		t.Error("--legend bottom-left: the legend is not in the bottom left, or is still in the top left")
	}
	if img := draw("--reference", ref, "--legend", "none"); plated(img, 12, 12) || plated(img, 12, 880) || plated(img, 1188, 12) {
		t.Error("--legend none: a legend was drawn")
	}
	if img := draw(); plated(img, 12, 12) {
		t.Error("a lone course with nothing asked for has a legend")
	}
	if img := draw("--title", "Morning run"); !plated(img, 12, 12) {
		t.Error("--title alone draws no legend to show it in")
	}

	// The title is what the legend says: a long one is a wide legend.
	if img := draw("--reference", ref, "--legend", "top-left"); plated(img, 250, 20) {
		t.Fatal("the legend of a course called run already reaches 250 px across; the test needs it narrow")
	}
	if img := draw("--reference", ref, "--legend", "top-left", "--title", "A morning run along the tenth parallel"); !plated(img, 250, 20) {
		t.Error("--title: the legend is no wider for a long title")
	}

	// A course from the top left to the bottom right leaves the other two
	// corners clear; auto takes the first of them. Three references the
	// course followed all the way draw nothing but make the legend tall
	// enough to reach the start.
	diagonal := filepath.Join(dir, "diagonal.gpx")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, `<trkpt lat="%s" lon="%s"><time>%s</time></trkpt>`, ftoa6(10.01-float64(i)*0.0002), ftoa6(20+float64(i)*0.0003), stamp(i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, diagonal, b.String())
	resetNow(mapCmd)
	if o, err := run(t, "map", "--store", store, "--out", out, "--width", "1200", "--height", "900", "--title", "Diagonal",
		"--reference", diagonal, "--reference", diagonal, "--reference", diagonal, diagonal); err != nil {
		t.Fatalf("map: %v\n%s", err, o)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	img, err = png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !plated(img, 1188, 12) || plated(img, 12, 12) {
		t.Error("auto: the legend is not in the clear top right, or is over the course in the top left")
	}

	resetNow(mapCmd)
	if _, err := run(t, "map", "--store", store, "--out", out, "--legend", "middle", run1); err == nil || !strings.Contains(err.Error(), "top-left, top-right, bottom-left, bottom-right, auto or none") {
		t.Errorf("--legend middle: %v", err)
	}
}

// A bottom-right legend clears the credit by its plate and a margin, and
// has the corner to itself when there is no credit.
func TestCreditSpace(t *testing.T) {
	face := faceAt(baseTextSize)
	if got, want := creditSpace("© OpenStreetMap", face, 6), face.Metrics().Height.Ceil()+8+6; got != want {
		t.Errorf("with a credit, %d, want %d", got, want)
	}
	if got := creditSpace("", face, 6); got != 0 {
		t.Errorf("with none, %d, want 0", got)
	}
}

// Merged, the files are one course with one name; separate, each has its
// own, its title if every one was given one.
func TestActivityNames(t *testing.T) {
	files := []string{"dir/a.fit", "dir/b.gpx"}
	for _, c := range []struct {
		titles   []string
		separate bool
		want     []string
		err      string
	}{
		{nil, false, []string{"a"}, ""},
		{[]string{"Morning"}, false, []string{"Morning"}, ""},
		{[]string{"x", "y"}, false, nil, "2 titles for one merged course"},
		{nil, true, []string{"a", "b"}, ""},
		{[]string{"Warm-up", "Parkrun"}, true, []string{"Warm-up", "Parkrun"}, ""},
		{[]string{"Warm-up"}, true, nil, "1 titles for 2 activities"},
	} {
		got, err := activityNames(files, c.titles, c.separate)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%v %v: %v, want an error saying %q", c.titles, c.separate, err, c.err)
			}
			continue
		}
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%v %v: %v %v, want %v", c.titles, c.separate, got, err, c.want)
		}
	}
}

// --separate draws each file as its own activity, the second in the second
// activity ink; merged, the same files are one course in one ink. --compare
// and --separate together are refused, as are activities that count cadence
// in different units coloured on one scale.
func TestMapSeparate(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	one, two, out := filepath.Join(dir, "one.gpx"), filepath.Join(dir, "two.gpx"), filepath.Join(dir, "map.png")
	writeLine(t, one, 10, 20, 0.0002, 50)
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 50; i++ { // a kilometre north, an hour later
		fmt.Fprintf(&b, `<trkpt lat="10.01" lon="%s"><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), stamp(3600+i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, two, b.String())
	store := filepath.Join(dir, "store")
	palette, overlay, err := paletteNamed("light")
	if err != nil {
		t.Fatal(err)
	}
	second := routemap.ActivityInks(palette, overlay)[1]

	draw := func(args ...string) image.Image {
		t.Helper()
		resetNow(mapCmd)
		all := append([]string{"map", "--store", store, "--out", out, "--width", "1200", "--height", "900"}, args...)
		if o, err := run(t, append(all, one, two)...); err != nil {
			t.Fatalf("map %v: %v\n%s", args, err, o)
		}
		f, err := os.Open(out)
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
	inSecond := func(img image.Image) bool {
		// Right of the legend, which has a sample of every ink.
		return hasColour(img, onGround(second), 200, 1200, 0, 900)
	}
	if img := draw("--separate", "--title", "One", "--title", "Two"); !inSecond(img) {
		t.Error("--separate: the second activity is not in the second activity ink")
	}
	if img := draw(); inSecond(img) {
		t.Error("merged: part of the course is in the second activity ink")
	}

	// Merged, the two files are still two legs, each with its great
	// circle: the second is in the second reference ink, which a single
	// great circle over the whole course would not be.
	second2 := routemap.ReferenceInks(palette)[1]
	if img := draw("--great-circle=each"); !hasColour(img, [3]uint8{second2.R, second2.G, second2.B}, 0, 1200, 0, 900) {
		t.Error("--great-circle=each over two merged files: no second great circle")
	}
	if img := draw("--great-circle"); hasColour(img, [3]uint8{second2.R, second2.G, second2.B}, 0, 1200, 0, 900) {
		t.Error("plain --great-circle over two merged files: more than the one great circle")
	}

	fit := filepath.Join(dir, "run.fit")
	opts := fittest.DefaultOptions()
	opts.Count = 300
	if err := fittest.WriteFile(fit, opts); err != nil {
		t.Fatal(err)
	}
	cad := filepath.Join(dir, "cad.gpx")
	b.Reset()
	b.WriteString(`<gpx xmlns:gpxtpx="http://www.garmin.com/xmlschemas/TrackPointExtension/v1"><trk><trkseg>`)
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><time>%s</time><extensions><gpxtpx:TrackPointExtension><gpxtpx:cad>85</gpxtpx:cad></gpxtpx:TrackPointExtension></extensions></trkpt>`, ftoa6(20+float64(i)*0.0002), stamp(i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, cad, b.String())
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--separate", "--compare", one, one, two}, "--compare compares one run"},
		{[]string{"--separate", "--colour", "cadence", fit, cad}, "different units"},
		{[]string{"--separate", "--title", "Only one", one, two}, "1 titles for 2 activities"},
	} {
		resetNow(mapCmd)
		_, err := run(t, append([]string{"map", "--store", store, "--out", out}, c.args...)...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("map %v: %v, want an error saying %q", c.args, err, c.want)
		}
	}
}
