package reference

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/course/match"
)

// writeRun writes a GPX run east along 10°N from 20°E like writeCourse's,
// lat degrees further north and a fix every seconds.
func writeRun(t *testing.T, dir, name string, n int, lat float64, seconds int) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < n; i++ {
		s := i * seconds
		fmt.Fprintf(&b, `<trkpt lat="%.6f" lon="%.6f"><time>2026-04-02T06:%02d:%02dZ</time></trkpt>`, 10+lat, 20+float64(i)*0.0001, s/60, s%60)
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// An average is stored as a GPX of the line and its times, the runs beside
// it as they were, and the manifest saying which runs are in it; the stored
// reference reads back as the average, with times, and the first run's crop
// is kept with it.
func TestAddAverage(t *testing.T) {
	src := t.TempDir()
	runs := []string{
		writeRun(t, src, "a.gpx", 101, 0, 1),
		writeRun(t, src, "b.gpx", 101, 0.00004, 2),  // 4 m north, twice as slow
		writeRun(t, src, "c.gpx", 101, -0.00004, 3), // 4 m south, three times
	}
	s := Store{Dir: filepath.Join(t.TempDir(), "references")}
	crop := &Crop{FromM: 0, ToM: 1100}
	m, avg, err := s.AddAverage("Loop", runs, []string{"loop"}, crop, "three runs", match.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if m.Source != "average.gpx" || len(m.Runs) != 3 || m.Runs[1].From != "b.gpx" || m.Runs[0].Crop == nil || m.Runs[1].Crop != nil || m.Crop != nil {
		t.Errorf("manifest %+v", m)
	}
	for i, r := range m.Runs {
		orig, _ := os.ReadFile(runs[i])
		stored, err := os.ReadFile(filepath.Join(filepath.Dir(m.File()), r.File))
		if err != nil || string(stored) != string(orig) || !r.Used || !avg.Runs[i].Used {
			t.Errorf("run %d: stored %v, used %v: %v", i+1, string(stored) == string(orig), r.Used, err)
		}
	}
	found, err := s.Find("loop")
	if err != nil || len(found.Runs) != 3 {
		t.Fatalf("found %+v, %v", found, err)
	}
	c, err := found.Course()
	if err != nil || !c.Timed || len(c.Points) < 100 {
		t.Fatalf("the stored average reads as %d points, timed %v: %v", len(c.Points), c != nil && c.Timed, err)
	}
	if lat := c.Points[50].Lat - 10; math.Abs(lat) > 0.00001 {
		t.Errorf("the average is %.6f° off the middle run", lat)
	}
	// Half way, 550 m: the middle of 50, 100 and 150 s.
	if got := (c.Points[55].Elapsed - c.Points[0].Elapsed).Seconds(); math.Abs(got-100) > 3 {
		t.Errorf("half way in %.0f s, want the median run's 100", got)
	}
	if m.Summary == nil || math.Abs(m.Summary.LengthM-1096) > 15 {
		t.Errorf("summary %+v", m.Summary)
	}
}

// A name already taken, or runs with no average between them, are refused,
// and nothing is written.
func TestAddAverageRefusals(t *testing.T) {
	src := t.TempDir()
	a := writeRun(t, src, "a.gpx", 101, 0, 1)
	far := writeRun(t, src, "far.gpx", 101, 0.01, 1) // a kilometre north
	s := Store{Dir: filepath.Join(t.TempDir(), "references")}
	if _, _, err := s.AddAverage("Loop", []string{a, far}, nil, nil, "", match.Options{}); err == nil {
		t.Error("an average of a run and one a kilometre away")
	}
	if all, _ := s.List(); len(all) != 0 {
		t.Errorf("%d references written by a refused average", len(all))
	}
	if _, err := s.Add("Loop", a, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AddAverage("loop", []string{a, a}, nil, nil, "", match.Options{}); err == nil || !strings.Contains(err.Error(), "already taken") {
		t.Errorf("a taken name: %v", err)
	}
}

// A crop is the first run's alone: the others are matched to the course it
// leaves, not cropped by it. Cropped to its first 50 s, the first run is
// 550 m of road; the others, slower, would be cut short of that by the same
// crop, and would follow none of it.
func TestAddAverageCropsTheFirstRunOnly(t *testing.T) {
	src := t.TempDir()
	runs := []string{
		writeRun(t, src, "a.gpx", 101, 0, 1),
		writeRun(t, src, "b.gpx", 101, 0.00002, 2),
		writeRun(t, src, "c.gpx", 101, -0.00002, 3),
	}
	s := Store{Dir: filepath.Join(t.TempDir(), "references")}
	m, _, err := s.AddAverage("Half", runs, nil, &Crop{ToS: 50}, "", match.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if m.Summary == nil || math.Abs(m.Summary.LengthM-550) > 15 {
		t.Errorf("summary %+v; want the first 550 m", m.Summary)
	}
	for i, r := range m.Runs {
		if !r.Used {
			t.Errorf("run %d left out", i+1)
		}
	}
}
