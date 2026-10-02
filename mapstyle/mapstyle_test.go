package mapstyle

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The built-in style is a valid one, with every setting of the course and
// the references made.
func TestDefault(t *testing.T) {
	s := Default()
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if s.Colouring != (Colouring{By: "none", GradeCap: 15, PowerSource: "auto", Width: 3}) {
		t.Errorf("default colouring %+v", s.Colouring)
	}
	if s.Text.Size != 13 || s.Map.LabelSize != "auto" || s.Legend.Position != "auto" || s.Markers.Every != "auto" || s.Course.Halo != "auto" || s.Reference.Halo != "auto" {
		t.Errorf("default text, map, legend, markers, halos: %+v", s)
	}
	if s.Width != 1600 || s.Height != 1000 {
		t.Errorf("default size %d by %d", s.Width, s.Height)
	}
	if s.Palette != "light" || *s.Course.Width != 3 || s.Course.Opacity != "auto" || s.Reference.Style != "dashed" || *s.Reference.Width != 2.25 {
		t.Errorf("default %+v", s)
	}
}

// A file changes what it says and nothing else, JSON as well as YAML; an
// empty one changes nothing; a key the style does not have is refused by
// name.
func TestRead(t *testing.T) {
	s := Default()
	if err := s.Read(strings.NewReader("palette: dark\ncourse:\n  colour: '#d32f2f'\n")); err != nil {
		t.Fatal(err)
	}
	if s.Palette != "dark" || s.Course.Colour != "#d32f2f" || *s.Course.Width != 3 || s.Reference.Style != "dashed" {
		t.Errorf("after a YAML file: %+v", s)
	}
	if err := s.Read(strings.NewReader(`{"reference": {"style": "dotted", "colours": ["#112233", "#445566"]}}`)); err != nil {
		t.Fatal(err)
	}
	if s.Reference.Style != "dotted" || len(s.Reference.Colours) != 2 || s.Course.Colour != "#d32f2f" {
		t.Errorf("after a JSON file: %+v", s)
	}
	before := s
	if err := s.Read(strings.NewReader("")); err != nil || s.Palette != before.Palette {
		t.Errorf("an empty file: %v, %+v", err, s)
	}
	err := s.Read(strings.NewReader("course:\n  colur: '#d32f2f'\n"))
	if err == nil || !strings.Contains(err.Error(), "colur") {
		t.Errorf("a misspelt key: %v", err)
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "theme.yaml")
	if err := os.WriteFile(path, []byte("course:\n  width: 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Default()
	if err := s.Load(path); err != nil || *s.Course.Width != 5 {
		t.Errorf("load: %v, width %v", err, *s.Course.Width)
	}
	if err := s.Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("a missing file loaded")
	}
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	os.WriteFile(bad, []byte("course: [\n"), 0o644)
	if err := s.Load(bad); err == nil || !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("a broken file: %v, want its name in the error", err)
	}
}

// One setting at a time, by its path: a colour that YAML would read as a
// comment is taken as written; a number, a list; an activity numbered from
// 1; a reference by a name with spaces and dots in it; later settings over
// earlier ones, and nothing else changed.
func TestSet(t *testing.T) {
	s := Default()
	for _, set := range []string{
		"course.colour=#d32f2f",
		"course.width=4.5",
		"reference.colours=[#112233, '#445566']",
		"activities.2.width=6",
		"references.Rhodes parkrun.colour=#0077aa",
		"references.run.v2.style=dotted",
		"palette=dark",
		"palette=light",
		"width=4800",
		"colouring.by=grade",
		"text.size=20",
		"map.label-size=9",
		"legend.position=none",
		"markers.every=none",
		"markers.every=2.5",
		"colouring.width=5",
		"course.halo=2",
		"colouring.grade-cap=30",
		"height=3000",
	} {
		if err := s.Set(set); err != nil {
			t.Fatalf("--set %s: %v", set, err)
		}
	}
	if s.Course.Colour != "#d32f2f" || *s.Course.Width != 4.5 || s.Palette != "light" || s.Course.Style != "solid" || s.Width != 4800 || s.Height != 3000 || s.Colouring.By != "grade" || s.Colouring.GradeCap != 30 ||
		s.Text.Size != 20 || s.Map.LabelSize != "9" || s.Legend.Position != "none" || s.Markers.Every != "2.5" || s.Colouring.Width != 5 || s.Course.Halo != "2" {
		t.Errorf("course and palette: %+v", s)
	}
	if len(s.Reference.Colours) != 2 || s.Reference.Colours[1] != "#445566" {
		t.Errorf("reference colours %q", s.Reference.Colours)
	}
	if len(s.Activities) != 2 || s.Activities[0].Width != nil || *s.Activities[1].Width != 6 {
		t.Errorf("activities %+v", s.Activities)
	}
	if s.References["Rhodes parkrun"].Colour != "#0077aa" || s.References["run.v2"].Style != "dotted" {
		t.Errorf("references %+v", s.References)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("valid settings refused: %v", err)
	}
	for _, c := range []struct{ set, want string }{
		{"course.colur=#d32f2f", "colur"},
		{"course", "path=value"},
		{"=3", "path=value"},
		{"activities.0.width=3", "numbered from 1"},
		{"activities.two.width=3", "numbered from 1"},
		{"palette.dark=1", "palette"},
		{"width.x=1", "width is one setting"},
		{"colouring.metric=pace", `no such setting "metric"; colouring has by, grade-cap, power-source, width`},
		{"text.colour=#000000", `no such setting "colour"; text has size`},
		{"markers.size=3", `no such setting "size"; markers has every`},
	} {
		s := Default()
		if err := s.Set(c.set); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("--set %s: %v, want an error saying %q", c.set, err, c.want)
		}
	}
}

// Validate names the setting that is not one.
func TestValidate(t *testing.T) {
	for _, c := range []struct{ set, want string }{
		{"palette=sepia", "palette"},
		{"width=63", "width: 63 pixels is too small"},
		{"colouring.by=stride", "colouring.by: \"stride\" is not one"},
		{"text.size=0", "text.size"},
		{"map.label-size=big", "map.label-size"},
		{"map.label-size=-3", "map.label-size"},
		{"legend.position=middle", "legend.position"},
		{"markers.every=0", "markers.every"},
		{"markers.every=often", "markers.every"},
		{"colouring.width=0", "colouring.width"},
		{"course.halo=-1", "course.halo"},
		{"references.Loop.halo=wide", "references.Loop.halo"},
		{"colouring.grade-cap=0", "colouring.grade-cap"},
		{"colouring.power-source=strid", "colouring.power-source: \"strid\" is invalid; use auto, stryd, or native"},
		{"height=0", "height: 0 pixels is too small"},
		{"course.colour=red", "course.colour"},
		{"course.colour=#12345", "course.colour"},
		{"reference.colours=[#123456, blue]", "reference.colours: \"blue\", the 2nd"},
		{"course.width=0", "course.width"},
		{"course.opacity=1.5", "course.opacity"},
		{"course.opacity=half", "course.opacity"},
		{"reference.style=wavy", "reference.style"},
		{"activities.1.colours=[#123456]", "activities.1.colours"},
		{"references.Loop.opacity=2", "references.Loop.opacity"},
	} {
		s := Default()
		if err := s.Set(c.set); err != nil {
			t.Fatalf("--set %s: %v", c.set, err)
		}
		if err := s.Validate(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an error naming %q", c.set, err, c.want)
		}
	}
	s := Default()
	s.Course.Width = nil
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "must all be set") {
		t.Errorf("a course without a width: %v", err)
	}
}

// What Write writes reads back as the same style -- the way a template is
// made -- with every setting in it, an empty list of colours included, a
// comment on each, and what an auto setting comes to.
func TestWrite(t *testing.T) {
	s := Default()
	s.Set("activities.2.width=6")
	s.Set("references.Loop.colour=#0077aa")
	var b bytes.Buffer
	if err := s.Write(&b, map[string]string{"course.colour": "light palette: #1b1b1b"}); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"palette: light", "size: 13", "label-size: auto", "position: auto", "every: auto", "halo: auto", "by: none", "grade-cap: 15", "power-source: auto", "width: 1600", "height: 1000", "# The picture's width in pixels", "colours: []", "width: 2.25", "style: dashed",
		"# A hex colour", "# solid, dashed or dotted.", "colour: auto # light palette: #1b1b1b",
		"Loop:", "width: 6",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the written style has no %q:\n%s", want, out)
		}
	}
	back := Default()
	back.Palette = "dark"
	if err := back.Read(strings.NewReader(out)); err != nil {
		t.Fatalf("reading it back: %v\n%s", err, out)
	}
	if back.Palette != "light" || *back.Activities[1].Width != 6 || back.References["Loop"].Colour != "#0077aa" {
		t.Errorf("read back: %+v", back)
	}
}
