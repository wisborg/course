package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// With no map on the machine and nobody to answer, map fetches nothing,
// says how to say yes, and still writes the picture: the course over a
// blank ground, reported as such. Nothing is created in the store.
func TestMapWithNoStoreAndNobodyToAsk(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }

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
