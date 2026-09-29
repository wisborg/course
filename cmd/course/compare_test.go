package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePaced writes a GPX course of n points east along 10°N from 20°E,
// 0.0002° apart, seconds(i) seconds after the start at each.
func writePaced(t *testing.T, path string, n int, seconds func(i int) int) {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), stamp(seconds(i)))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, path, b.String())
}

// A run that lost time over the second half of the course says so split by
// split, in text and in JSON, with --split choosing how long a split is; a
// reference with no times, no reference and a split of nothing are refused.
func TestCompareCommand(t *testing.T) {
	resetFlags(t, compareCmd)
	t.Cleanup(func() { referencesDir = "" })

	dir := t.TempDir()
	ref, run1, plan := filepath.Join(dir, "ref.gpx"), filepath.Join(dir, "run.gpx"), filepath.Join(dir, "plan.gpx")
	// About 2.2 km at 22 m a point: the reference 10 s a point throughout,
	// the run the same to halfway and 12 s a point after.
	writePaced(t, ref, 100, func(i int) int { return 10 * i })
	writePaced(t, run1, 100, func(i int) int {
		if i <= 50 {
			return 10 * i
		}
		return 500 + 12*(i-50)
	})
	writeFile(t, plan, `<gpx><trk><trkseg><trkpt lat="10" lon="20"></trkpt><trkpt lat="10" lon="20.02"></trkpt></trkseg></trk></gpx>`)

	out, err := run(t, "compare", "--reference", ref, run1)
	if err != nil {
		t.Fatalf("compare: %v\n%s", err, out)
	}
	for _, want := range []string{"against ref, 2.1", "1m38s behind at the end", "2.00", "slower"} {
		if !strings.Contains(out, want) {
			t.Errorf("the comparison does not say %q:\n%s", want, out)
		}
	}
	// The first kilometre was run at the reference's pace: level over the
	// split and level at its end.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "1.00") && strings.Count(line, "level") != 2 {
			t.Errorf("the first kilometre is not level both ways: %q", line)
		}
	}
	if strings.Contains(out, "stopped") {
		t.Errorf("a run without a stop has one:\n%s", out)
	}

	// Two minutes stood still halfway are listed under the splits, when
	// and where they were as the 10 m samples have them.
	stopped := filepath.Join(dir, "stopped.gpx")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i, sec := 0, 0; i < 100; i++ {
		fixes := 1
		if i == 50 {
			fixes = 13 // a fix every 10 s while standing, as a watch records
		}
		for range fixes {
			fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), stamp(sec))
			sec += 10
		}
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, stopped, b.String())
	resetNow(compareCmd)
	if out, err := run(t, "compare", "--reference", ref, stopped); err != nil || !strings.Contains(out, "stopped at 0:08:") || !strings.Contains(out, "1.09 km along") {
		t.Errorf("a run with a stop: %v\n%s", err, out)
	}

	// 40 m north of the reference is off it, unless --near says otherwise.
	wide := filepath.Join(dir, "wide.gpx")
	writeFile(t, wide, strings.ReplaceAll(readFile(t, ref), `lat="10"`, `lat="10.00036"`))
	resetNow(compareCmd)
	if _, err := run(t, "compare", "--reference", ref, wide); err == nil {
		t.Error("a run 40 m off the reference was compared")
	}
	resetNow(compareCmd)
	if out, err := run(t, "compare", "--reference", ref, "--near", "60", wide); err != nil {
		t.Errorf("a run 40 m off the reference with --near 60: %v\n%s", err, out)
	}

	resetNow(compareCmd)
	out, err = run(t, "compare", "--reference", ref, "--split", "0.5", "--format", "json", run1)
	var doc struct {
		Reference string  `json:"reference"`
		GapS      float64 `json:"gap_s"`
		Splits    []struct {
			ToM  float64 `json:"to_m"`
			RunS float64 `json:"run_s"`
			RefS float64 `json:"reference_s"`
			GapS float64 `json:"gap_s"`
		} `json:"splits"`
		Stops []any `json:"stops"`
	}
	if err != nil || json.Unmarshal([]byte(out), &doc) != nil {
		t.Fatalf("compare as JSON: %v\n%s", err, out)
	}
	if doc.Reference != "ref" || doc.GapS < 95 || doc.GapS > 100 || len(doc.Splits) != 5 || doc.Stops == nil {
		t.Errorf("compare as JSON: %+v", doc)
	}
	if s := doc.Splits[0]; s.ToM != 500 || s.GapS != 0 || s.RunS != s.RefS {
		t.Errorf("the first half-kilometre split, run level: %+v", s)
	}
	if s := doc.Splits[3]; s.RunS <= s.RefS || s.GapS <= doc.Splits[2].GapS {
		t.Errorf("a split in the slower half is not slower: %+v", s)
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"compare", "--reference", plan, run1}, "no times"},
		{[]string{"compare", run1}, `"reference" not set`},
		{[]string{"compare", "--reference", ref, "--split", "0", run1}, "--split"},
		{[]string{"compare", "--reference", filepath.Join(dir, "missing.gpx"), run1}, "missing.gpx"},
	} {
		resetNow(compareCmd)
		if _, err := run(t, c.args...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want an error saying %q", c.args[1:], err, c.want)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
