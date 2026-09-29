package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/wisborg/course"
)

// resetFlags puts a command's flags back to their defaults after a test ran
// it: cobra keeps them between runs in one process, and array flags go on
// appending.
func resetFlags(t *testing.T, c *cobra.Command) {
	t.Cleanup(func() { resetNow(c) })
}

// resetNow puts a command's flags back to their defaults at once, between
// two runs of it in one test.
func resetNow(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			sv.Replace(nil)
		} else {
			f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}

// writeLine writes a GPX course of n points east along lat from lon, step
// degrees apart, a second each.
func writeLine(t *testing.T, path string, lat, lon, step float64, n int) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<trkpt lat="%.6f" lon="%.6f"><time>2026-04-02T06:%02d:%02dZ</time></trkpt>`, lat, lon+float64(i)*step, i/60, i%60)
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// With no map on the machine and nobody to answer, map fetches nothing,
// says how to say yes, and still writes the picture: the course over a
// blank ground, reported as such. Nothing is created in the store.
func TestMapWithNoStoreAndNobodyToAsk(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	gpx := filepath.Join(dir, "run.gpx")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%.6f"><time>2026-04-02T06:00:%02dZ</time></trkpt>`, 20+float64(i)*0.0002, i)
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	if err := os.WriteFile(gpx, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	store, out := filepath.Join(dir, "store"), filepath.Join(dir, "run.png")

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"map", "--store", store, "--out", out, "--width", "400", "--height", "300", gpx})
	defer root.SetArgs(nil)
	if err := root.Execute(); err != nil {
		t.Fatalf("map: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Errorf("the refusal does not say how to answer in advance:\n%s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "blank ground") {
		t.Errorf("the report does not say there was no map:\n%s", stdout.String())
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() != 400 || img.Bounds().Dy() != 300 {
		t.Errorf("the picture is %v, %v", img.Bounds(), err)
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Errorf("a store was created with nobody's consent: %v", err)
	}
}

// A reference from a file and the great circle are drawn, with a legend
// naming them in the top left; the view holds a reference that goes where the
// course did not; and a reference that cannot be read is an error naming it.
func TestMapWithReferences(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetFlags(t, mapCmd)

	dir := t.TempDir()
	run, ref, out := filepath.Join(dir, "run.gpx"), filepath.Join(dir, "other.gpx"), filepath.Join(dir, "run.png")
	writeLine(t, run, 10, 20, 0.0002, 50)
	writeLine(t, ref, 10.01, 20, 0.0002, 50) // a kilometre north of the course
	store := filepath.Join(dir, "store")

	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	defer root.SetArgs(nil)
	root.SetArgs([]string{"map", "--store", store, "--out", out, "--width", "400", "--height", "300",
		"--reference", ref, "--great-circle", run})
	if err := root.Execute(); err != nil {
		t.Fatalf("map: %v\n%s", err, stderr.String())
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
	// The legend's plate is near-white in the top left, where the blank
	// ground is the palette's background.
	plate := img.At(12, 12)
	ground := img.At(390, 150)
	if plate == ground {
		t.Error("nothing drawn in the top left, where the legend goes")
	}
	// The reference, a kilometre north, is on the picture: some pixel in the
	// top half is the first reference ink.
	if !anyInk(img, image.Rect(60, 0, 400, 150)) {
		t.Error("the reference north of the course is not on the picture")
	}

	root.SetArgs([]string{"map", "--store", store, "--out", out, "--reference", filepath.Join(dir, "missing.gpx"), run})
	err = root.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing.gpx") {
		t.Errorf("a reference that is not there: %v", err)
	}
}

// anyInk reports whether any pixel in r is the light palette's first
// reference ink, within a little for antialiasing.
func anyInk(img image.Image, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			if near(cr>>8, 0xC2) && near(cg>>8, 0x18) && near(cb>>8, 0x5B) {
				return true
			}
		}
	}
	return false
}

func near(a uint32, b uint32) bool { return a+12 >= b && a <= b+12 }

func ftoa6(f float64) string { return fmt.Sprintf("%.6f", f) }

func stamp(s int) string { return fmt.Sprintf("2026-04-02T%02d:%02d:%02dZ", 6+s/3600, s/60%60, s%60) }

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) *course.Course {
	t.Helper()
	c, err := course.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
