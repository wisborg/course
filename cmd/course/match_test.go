package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/wisborg/course/match"
)

// A run through two stored references, with a stretch between them, is
// matched to both, in order, in text and in JSON; with --reference, only the
// one named is looked for; and --reference auto on a map draws the ones
// matched.
func TestMatchCommand(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	for _, c := range []*cobra.Command{referenceAdd, matchCmd, mapCmd} {
		resetFlags(t, c)
	}
	t.Cleanup(func() { referencesDir = "" })

	dir := t.TempDir()
	refs := filepath.Join(dir, "refs")
	first, second := filepath.Join(dir, "first.gpx"), filepath.Join(dir, "second.gpx")
	writeLine(t, first, 10, 20, 0.0001, 100)     // ~1.1 km east
	writeLine(t, second, 10.02, 20, 0.0001, 100) // the same, 2.2 km north
	run1 := filepath.Join(dir, "morning.gpx")
	writeMorning(t, run1)

	for name, file := range map[string]string{"First": first, "Second": second} {
		resetNow(referenceAdd)
		if out, err := run(t, "reference", "add", "--references", refs, name, file); err != nil {
			t.Fatalf("add %s: %v\n%s", name, err, out)
		}
	}
	out, err := run(t, "match", "--references", refs, run1)
	if err != nil || strings.Index(out, "First") < 0 || strings.Index(out, "Second") < strings.Index(out, "First") {
		t.Fatalf("match: %v\n%s", err, out)
	}
	resetNow(matchCmd)
	out, err = run(t, "match", "--references", refs, "--format", "json", run1)
	var ms []map[string]any
	if err != nil || json.Unmarshal([]byte(out), &ms) != nil || len(ms) != 2 || ms[0]["coverage"] != 1.0 || ms[0]["reference"] != "First" || ms[0]["from_s"] == nil || ms[0]["stops"] == nil {
		t.Fatalf("match as JSON: %v\n%s", err, out)
	}
	resetNow(matchCmd)
	out, err = run(t, "match", "--references", refs, "--reference", "Second", run1)
	if err != nil || strings.Contains(out, "First") || !strings.Contains(out, "Second") {
		t.Errorf("match with one reference named: %v\n%s", err, out)
	}
	resetNow(matchCmd)
	if out, err := run(t, "match", "--references", refs, first); err != nil || strings.Contains(out, "Second") {
		t.Errorf("a run of the first course alone: %v\n%s", err, out)
	}

	png := filepath.Join(dir, "morning.png")
	if out, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--references", refs, "--reference", "auto",
		"--out", png, "--width", "300", "--height", "300", run1); err != nil {
		t.Errorf("map --reference auto: %v\n%s", err, out)
	}
	refsFound, err := matchedReferences(mustRead(t, run1), true)
	if err != nil || len(refsFound) != 2 || refsFound[0].Name != "First" || refsFound[1].Name != "Second" || !refsFound[0].Follows {
		t.Errorf("auto found %+v, %v; want First then Second, in the order run, drawn where they part", refsFound, err)
	}
	if whole, err := matchedReferences(mustRead(t, run1), false); err != nil || len(whole) != 2 || whole[0].Follows || whole[1].Follows {
		t.Errorf("auto with whole references found %+v, %v; want both, drawn whole", whole, err)
	}
}

// writeMorning is a run of the first course, a stretch north, and the second
// course, as one GPX.
func writeMorning(t *testing.T, path string) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	s := 0
	pt := func(lat, lon float64) {
		b.WriteString(`<trkpt lat="` + ftoa6(lat) + `" lon="` + ftoa6(lon) + `"><time>` + stamp(s) + `</time></trkpt>`)
		s++
	}
	for i := 0; i < 100; i++ {
		pt(10, 20+float64(i)*0.0001)
	}
	for i := 1; i < 200; i++ {
		pt(10+float64(i)*0.0001, 20.0099-float64(i)*0.0000495)
	}
	for i := 0; i < 100; i++ {
		pt(10.02, 20+float64(i)*0.0001)
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, path, b.String())
}

// Of two matches with one reference, the better covered is taken; and a
// reference with none has none.
func TestBestMatch(t *testing.T) {
	ms := []match.Match{{Reference: "A", Coverage: 0.85, From: 1}, {Reference: "B", Coverage: 1}, {Reference: "A", Coverage: 0.95, From: 2}}
	if m, ok := bestMatch(ms, "A"); !ok || m.From != 2 {
		t.Errorf("best of A = %+v, %v; want the 0.95", m, ok)
	}
	if _, ok := bestMatch(ms, "C"); ok {
		t.Error("a match found for a reference never matched")
	}
}
