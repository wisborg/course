package reference

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/course"
)

// writeCourse writes a GPX course east along 10°N from 20°E, n points
// 0.0001° (about 11 m) and a second apart, returning its path.
func writeCourse(t *testing.T, dir, name string, n int) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%.6f"><time>2026-04-02T06:%02d:%02dZ</time></trkpt>`, 20+float64(i)*0.0001, i/60, i%60)
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Rhodes parkrun":         "rhodes-parkrun",
		"  Three Bridges (2026)": "three-bridges-2026",
		"Fælledparken Parkrun":   "fælledparken-parkrun",
		"!!!":                    "",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

// A reference is stored as its original file, byte for byte, with a manifest
// summarising the course; it is found by name, alias or slug in any case,
// and removed with its file.
func TestAddFindRemove(t *testing.T) {
	src := t.TempDir()
	path := writeCourse(t, src, "run.gpx", 101)
	s := Store{Dir: filepath.Join(t.TempDir(), "references")}

	m, err := s.Add("Rhodes parkrun", path, []string{"rhodes"}, nil, "the usual course")
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := os.ReadFile(path)
	stored, err := os.ReadFile(m.File())
	if err != nil || string(stored) != string(orig) {
		t.Fatalf("the stored file is not the original: %v", err)
	}
	if m.Summary == nil || m.Summary.LengthM < 1090 || m.Summary.LengthM > 1100 || m.Summary.Loop {
		t.Errorf("summary %+v; want about 1096 m and not a loop", m.Summary)
	}
	for _, name := range []string{"Rhodes parkrun", "rhodes PARKRUN", "rhodes", "Rhodes", "rhodes-parkrun"} {
		if got, err := s.Find(name); err != nil || got.Name != "Rhodes parkrun" {
			t.Errorf("Find(%q): %v %v", name, got.Name, err)
		}
	}
	if _, err := s.Find("Olympic"); err == nil {
		t.Error("found a reference that is not there")
	}
	c, err := m.Course()
	if err != nil || len(c.Points) != 101 {
		t.Errorf("the stored course reads as %v points, %v", len(c.Points), err)
	}

	if _, err := s.Add("Rhodes", path, nil, nil, ""); err == nil {
		t.Error("a second reference answering to an alias of the first was stored")
	}
	if _, err := s.Add("rhodes parkrun", path, nil, nil, ""); err == nil {
		t.Error("a second reference with the first's name in another case was stored")
	}

	if _, err := s.Remove("rhodes"); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.List(); len(all) != 0 {
		t.Errorf("%d references left after removing the only one", len(all))
	}
	if _, err := os.Stat(m.File()); !os.IsNotExist(err) {
		t.Error("the removed reference's file is still there")
	}
}

// A crop is recorded, not applied to the file, and the reference's course is
// the cropped part; a crop that leaves nothing, or a file that is not a
// course, is refused before anything is written.
func TestCropsAndRefusals(t *testing.T) {
	src := t.TempDir()
	path := writeCourse(t, src, "run.gpx", 101) // ~1.1 km, 100 s
	s := Store{Dir: filepath.Join(t.TempDir(), "references")}

	m, err := s.Add("Middle", path, nil, &Crop{FromM: 200, ToM: 500}, "")
	if err != nil {
		t.Fatal(err)
	}
	// The crop keeps the course's own points, 11 m apart, not interpolated
	// ends: 300 m less up to a step at each end.
	if m.Summary.LengthM < 278 || m.Summary.LengthM > 300 {
		t.Errorf("a 200-500 m crop is %.0f m long", m.Summary.LengthM)
	}
	c, _ := m.Course()
	if c.Points[0].Lon <= 20.0015 {
		t.Errorf("the cropped course starts at %v; want about 200 m in", c.Points[0].Lon)
	}
	if m, err := s.Add("By time", path, nil, &Crop{FromS: 10, ToS: 20}, ""); err != nil || len(mustCourse(t, m).Points) != 11 {
		t.Errorf("a 10-20 s crop: %v", err)
	}

	if _, err := s.Add("Nothing", path, nil, &Crop{FromM: 5000}, ""); err == nil {
		t.Error("a crop past the end was stored")
	}
	junk := filepath.Join(src, "junk.gpx")
	os.WriteFile(junk, []byte("not a course"), 0o644)
	if _, err := s.Add("Junk", junk, nil, nil, ""); err == nil {
		t.Error("a file that is not a course was stored")
	}
	all, _ := s.List()
	if len(all) != 2 {
		t.Errorf("%d references stored; the refused ones left something behind", len(all))
	}
}

// A loop ends where it started.
func TestLoop(t *testing.T) {
	src := t.TempDir()
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i, lon := range []float64{20, 20.005, 20.005, 20.0001} {
		lat := 10.0
		if i == 1 || i == 2 {
			lat = 10.003
		}
		fmt.Fprintf(&b, `<trkpt lat="%v" lon="%v"><time>2026-04-02T06:00:%02dZ</time></trkpt>`, lat, lon, i)
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	path := filepath.Join(src, "loop.gpx")
	os.WriteFile(path, []byte(b.String()), 0o644)
	m, err := Store{Dir: t.TempDir()}.Add("Loop", path, nil, nil, "")
	if err != nil || !m.Summary.Loop {
		t.Errorf("a course ending 11 m from its start is not a loop: %+v %v", m.Summary, err)
	}
}

func mustCourse(t *testing.T, m Manifest) *course.Course {
	t.Helper()
	c, err := m.Course()
	if err != nil {
		t.Fatal(err)
	}
	return c
}
