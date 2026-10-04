package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/output"

	"github.com/wisborg/course"
	"github.com/wisborg/course/mapstyle"
	"github.com/wisborg/course/summary"
)

// resetUnits puts --units and --unit back after a test, as resetFlags does
// for a command's own flags: they are the root's.
func resetUnits(t *testing.T) {
	t.Cleanup(func() { unitOpts.system, unitOpts.each = "metric", nil })
}

// --units and --unit make the units in use; a system, quantity or unit
// that is not one is refused saying what there is.
func TestUnitsFor(t *testing.T) {
	resetUnits(t)
	unitOpts.system, unitOpts.each = "imperial", []string{"distance=nmi", "speed=kn"}
	u, err := unitsFor(&cobra.Command{})
	if err != nil || u != (units.Set{Distance: units.NauticalMile, Elevation: units.Foot, Speed: units.Knot, Pace: units.MinutesPerMile, Temperature: units.Fahrenheit}) {
		t.Errorf("a flight's units: %+v %v", u, err)
	}
	for _, c := range []struct {
		system string
		each   []string
		want   string
	}{
		{"nautical", nil, "--units: \"nautical\" is not a system of units; use metric or imperial"},
		{"metric", []string{"distance=ft"}, "--unit distance=ft: \"ft\" is not a unit of distance; use km, mi, nmi"},
		{"metric", []string{"time=s"}, "not a quantity with units"},
		{"metric", []string{"distance"}, "give a quantity and its unit"},
	} {
		unitOpts.system, unitOpts.each = c.system, c.each
		if _, err := unitsFor(&cobra.Command{}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("--units %s --unit %v: %v, want %q", c.system, c.each, err, c.want)
		}
	}
	m, _ := units.Of(units.Metric)
	if got := distance(5090, m); got != "5.09 km" {
		t.Errorf("distance %q", got)
	}
	i, _ := units.Of(units.Imperial)
	if got := short(30, i); got != "98 ft" {
		t.Errorf("short %q", got)
	}
}

// The text commands write in the units asked for -- compare in mile splits,
// --split taken in miles, match with its deviations in feet -- while their
// JSON keeps metres for the programs that read it.
func TestTextCommandsInOtherUnits(t *testing.T) {
	resetUnits(t)
	resetFlags(t, compareCmd)
	resetFlags(t, matchCmd)
	dir := t.TempDir()
	ref, run1 := filepath.Join(dir, "ref.gpx"), filepath.Join(dir, "run.gpx")
	// About 4.3 km, 2.7 mi, at 22 m a point.
	writePaced(t, ref, 200, func(i int) int { return 10 * i })
	writePaced(t, run1, 200, func(i int) int { return 10 * i })

	out, err := run(t, "compare", "--units", "imperial", "--reference", ref, run1)
	if err != nil {
		t.Fatalf("compare: %v\n%s", err, out)
	}
	for _, want := range []string{"2.7", " mi of it", "  mi ", "1.00 ", "2.00 "} {
		if !strings.Contains(out, want) {
			t.Errorf("compare in imperial has no %q:\n%s", want, out)
		}
	}
	resetNow(compareCmd)
	unitOpts.system = "metric"
	out, err = run(t, "compare", "--units", "imperial", "--split", "0.5", "--format", "json", "--reference", ref, run1)
	var doc struct {
		Splits []struct {
			ToM float64 `json:"to_m"`
		} `json:"splits"`
	}
	if err != nil || json.Unmarshal([]byte(out), &doc) != nil || len(doc.Splits) < 2 || int(doc.Splits[0].ToM+0.5) != 800 && int(doc.Splits[0].ToM+0.5) != 810 {
		t.Errorf("half-mile splits as JSON: %v, %+v\n%s", err, doc.Splits, out)
	}
	resetNow(compareCmd)
	unitOpts.system = "metric"
	out, err = run(t, "compare", "--units", "imperial", "--format", "csv", "--reference", ref, run1)
	if err != nil || !strings.HasPrefix(out, "km,") || !strings.Contains(out, "\n1.61,") {
		t.Errorf("mile splits as CSV, in kilometres: %v\n%s", err, out)
	}

	unitOpts.system = "metric"
	out, err = run(t, "match", "--units", "imperial", "--reference", ref, run1)
	if err != nil || !strings.Contains(out, "from mi") || !strings.Contains(out, "median ft") {
		t.Errorf("match in imperial: %v\n%s", err, out)
	}
	resetNow(matchCmd)
	unitOpts.system = "metric"
	out, err = run(t, "match", "--units", "imperial", "--format", "csv", "--reference", ref, run1)
	if err != nil || !strings.Contains(out, "from km") {
		t.Errorf("match as CSV keeps its kilometres: %v\n%s", err, out)
	}
}

// A map's units are in its style: --units and --unit are the same as
// setting them, and map style prints them; the legend's ends are written in
// them -- a speed in knots, an elevation in feet.
func TestMapUnits(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetUnits(t)
	resetFlags(t, mapCmd)
	resetFlags(t, mapStyleCmd)
	t.Cleanup(func() { styleOpts.file, styleOpts.sets = "", nil })

	out, err := run(t, "map", "style", "--units", "imperial", "--unit", "distance=nmi", "--set", "units.speed=kn")
	st := mapstyle.Default()
	if err != nil || st.Read(strings.NewReader(out)) != nil || st.Units.System != "imperial" || st.Units.Distance != "nmi" || st.Units.Speed != "kn" || st.Units.Elevation != "auto" {
		t.Errorf("map style with units: %v, %+v\n%s", err, st.Units, out)
	}
	resetNow(mapStyleCmd)
	resetNow(mapCmd)
	unitOpts.system, unitOpts.each = "metric", nil
	styleOpts.file, styleOpts.sets = "", nil
	if _, err := run(t, "map", "style", "--set", "units.distance=ft"); err == nil || !strings.Contains(err.Error(), `units.distance: "ft" is not a unit of distance`) {
		t.Errorf("a distance in feet: %v", err)
	}

	dir := t.TempDir()
	run1 := filepath.Join(dir, "run.gpx")
	var b strings.Builder
	b.WriteString(`<gpx><trk><trkseg>`)
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, `<trkpt lat="10" lon="%s"><ele>%d</ele><time>%s</time></trkpt>`, ftoa6(20+float64(i)*0.0002), i, stamp(i))
	}
	b.WriteString(`</trkseg></trk></gpx>`)
	writeFile(t, run1, b.String())
	for _, c := range []struct {
		args []string
		want string
	}{
		// 22 m a second is 43 knots.
		{[]string{"--colour", "speed", "--unit", "speed=kn"}, " kn (red)"},
		{[]string{"--colour", "speed"}, " km/h (red)"},
		{[]string{"--colour", "pace", "--units", "imperial"}, "/mi (red)"},
		{[]string{"--colour", "elevation", "--units", "imperial"}, " ft (red)"},
	} {
		resetNow(mapCmd)
		unitOpts.system, unitOpts.each = "metric", nil
		styleOpts.file, styleOpts.sets = "", nil
		args := append([]string{"map", "--store", filepath.Join(dir, "store"), "--out", filepath.Join(dir, "m.png"), "--width", "300", "--height", "200"}, c.args...)
		o, err := run(t, append(args, run1)...)
		if err != nil || !strings.Contains(o, c.want) {
			t.Errorf("map %v: %v\n%s\nwant %q", c.args, err, o, c.want)
		}
	}
}

// A person reading text gets the units asked for; CSV, JSON and YAML, read by
// other programs, keep metric.
func TestUnitsShown(t *testing.T) {
	resetUnits(t)
	unitOpts.system = "imperial"
	if u, err := unitsShown(&cobra.Command{}, output.Text); err != nil || u.Distance != units.Mile {
		t.Errorf("text: %+v %v", u, err)
	}
	for _, f := range []output.Format{output.CSV, output.JSON, output.YAML} {
		if u, err := unitsShown(&cobra.Command{}, f); err != nil || u.Distance != units.Kilometre {
			t.Errorf("%v: %+v %v", f, u, err)
		}
	}
	unitOpts.system = "nautical"
	if _, err := unitsShown(&cobra.Command{}, output.CSV); err == nil {
		t.Error("a system that is not one is let through for CSV")
	}
}

// summary's table and footer are written in the units given.
func TestSummaryInOtherUnits(t *testing.T) {
	i, _ := units.Of(units.Imperial)
	s := &summary.Summary{Measured: true, Rows: []summary.Row{{Mark: summary.Mark{Distance: 1609.344, HasDistance: true}}}}
	s.Finish.HasDistance, s.Finish.Distance = true, 3218.688
	if out := summaryTable(s, i).String(); !strings.Contains(out, "mi") || !strings.Contains(out, "1.00") {
		t.Errorf("the summary's table in miles:\n%s", out)
	}
	var b strings.Builder
	writeFooter(&b, &course.Course{}, s, nil, i)
	if !strings.Contains(b.String(), "finish 2.00 mi") {
		t.Errorf("the summary's footer in miles:\n%s", b.String())
	}
}

// match's deviations from a course are short distances, in feet under
// imperial: a run 10 m off its course is 33 ft off.
func TestMatchDeviationsInFeet(t *testing.T) {
	resetUnits(t)
	resetFlags(t, matchCmd)
	dir := t.TempDir()
	ref, off := filepath.Join(dir, "ref.gpx"), filepath.Join(dir, "off.gpx")
	writePaced(t, ref, 200, func(i int) int { return 10 * i })
	writeFile(t, off, strings.ReplaceAll(readFile(t, ref), `lat="10"`, `lat="10.00009"`)) // 10 m north
	out, err := run(t, "match", "--units", "imperial", "--reference", ref, off)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	f := strings.Fields(lines[len(lines)-1])
	if err != nil || !strings.Contains(lines[0], "median ft") || len(f) < 2 || f[len(f)-2] != "33" || f[len(f)-1] != "33" {
		t.Errorf("10 m off in feet: %v\n%s", err, out)
	}
}

// A stored reference's length is in the units asked for, when stored, listed
// and shown; listed as CSV, in kilometres still.
func TestReferenceInOtherUnits(t *testing.T) {
	resetUnits(t)
	for _, c := range []*cobra.Command{referenceAdd, referenceList, referenceShow} {
		resetFlags(t, c)
	}
	t.Cleanup(func() { referencesDir = "" })
	dir := t.TempDir()
	refs, file := filepath.Join(dir, "refs"), filepath.Join(dir, "loop.gpx")
	writePaced(t, file, 200, func(i int) int { return 10 * i }) // about 4.3 km, 2.7 mi
	out, err := run(t, "reference", "add", "--references", refs, "--units", "imperial", "Loop", file)
	if err != nil || !strings.Contains(out, "2.7") || !strings.Contains(out, " mi") {
		t.Errorf("stored in miles: %v\n%s", err, out)
	}
	unitOpts.system = "metric"
	if out, err := run(t, "reference", "list", "--references", refs, "--units", "imperial"); err != nil || !strings.Contains(out, "mi") || !strings.Contains(out, "2.7") {
		t.Errorf("listed in miles: %v\n%s", err, out)
	}
	unitOpts.system = "metric"
	resetNow(referenceList)
	if out, err := run(t, "reference", "list", "--references", refs, "--units", "imperial", "--format", "csv"); err != nil || !strings.Contains(out, "km") || !strings.Contains(out, "4.3") {
		t.Errorf("listed as CSV in kilometres: %v\n%s", err, out)
	}
	unitOpts.system = "metric"
	resetNow(referenceList)
	if out, err := run(t, "reference", "show", "--references", refs, "--units", "imperial", "Loop"); err != nil || !strings.Contains(out, "2.7") || !strings.Contains(out, " mi") {
		t.Errorf("shown in miles: %v\n%s", err, out)
	}
}

// A map's --compare report gives the length compared in the distance unit.
func TestMapCompareReportInOtherUnits(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	resetUnits(t)
	resetFlags(t, mapCmd)
	dir := t.TempDir()
	ref, run1 := filepath.Join(dir, "ref.gpx"), filepath.Join(dir, "run.gpx")
	writePaced(t, ref, 200, func(i int) int { return 10 * i })
	writePaced(t, run1, 200, func(i int) int { return 11 * i })
	out, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--out", filepath.Join(dir, "m.png"), "--width", "300", "--height", "200", "--units", "imperial", "--compare", ref, run1)
	if err != nil || !strings.Contains(out, " mi of it") {
		t.Errorf("--compare in miles: %v\n%s", err, out)
	}
}
