package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/osmbase/locate"
	"github.com/wisborg/output"

	"github.com/wisborg/course"
	"github.com/wisborg/course/summary"
)

// A time or a distance the course did not record is left out of the JSON,
// never written as zero: a plan's rows have no elapsed_s, and a GPX's no
// distance_m.
func TestJSONLeavesOutWhatWasNotRecorded(t *testing.T) {
	c := &course.Course{Sources: []string{"plan.gpx"}, Points: []course.Point{{Lat: 10, Lon: 20}}}
	s := &summary.Summary{Depth: locate.Country, Rows: []summary.Row{{Lat: 10, Lon: 20,
		Places: []locate.Match{{Level: locate.Country, Name: "Land"}}}}}
	row := func() (map[string]any, map[string]any) {
		var b bytes.Buffer
		if err := (output.Document{Data: jsonSummary(c, s, nil)}).Write(&b, output.JSON); err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		finish, _ := doc["finish"].(map[string]any)
		return doc["rows"].([]any)[0].(map[string]any), finish
	}
	r, finish := row()
	if _, ok := r["elapsed_s"]; ok {
		t.Errorf("a plan's row has elapsed_s: %v", r)
	}
	if _, ok := r["distance_m"]; ok {
		t.Errorf("a plan's row has distance_m: %v", r)
	}
	if finish != nil {
		t.Errorf("a plan with neither has a finish: %v", finish)
	}

	c.Timed = true
	s.Rows[0].Mark = summary.Mark{HasElapsed: true, Elapsed: 90 * time.Second}
	r, _ = row()
	if r["elapsed_s"] != 90.0 {
		t.Errorf("elapsed_s is %v, want 90", r["elapsed_s"])
	}
	if _, ok := r["distance_m"]; ok {
		t.Errorf("a course with no distance has distance_m: %v", r)
	}
}

// --depth takes auto and the depths, and not water, which is named at every
// depth rather than being one.
func TestParseDepth(t *testing.T) {
	if _, auto, err := parseDepth("auto"); !auto || err != nil {
		t.Errorf("auto: %v %v", auto, err)
	}
	if d, auto, err := parseDepth("street"); d != locate.Street || auto || err != nil {
		t.Errorf("street: %v %v %v", d, auto, err)
	}
	for _, bad := range []string{"water", "suburb", ""} {
		if _, _, err := parseDepth(bad); err == nil {
			t.Errorf("--depth %q accepted", bad)
		}
	}
}

// The table has a column for each level something was named at, and none
// for the rest: a flight is not a table of empty street columns.
func TestSummaryTableHasOnlyNamedLevels(t *testing.T) {
	s := &summary.Summary{Depth: locate.Street, Timed: true, Rows: []summary.Row{
		{Places: []locate.Match{{Level: locate.Country, Name: "Land"}}},
		{Places: []locate.Match{{Level: locate.Water, Name: "Sea"}}},
	}}
	metric, _ := units.Of(units.Metric)
	out := summaryTable(s, metric).String()
	header := strings.Fields(strings.SplitN(out, "\n", 2)[0])
	if strings.Join(header, " ") != "elapsed country water" {
		t.Errorf("header %v", header)
	}
}

// What a summary asks the store for: the whole of a local course, only the
// ends of a wide one, and nothing a level's boundaries answer.
func TestSummaryNeed(t *testing.T) {
	line := func(n int, step float64) *course.Course {
		c := &course.Course{}
		for i := 0; i < n; i++ {
			c.Points = append(c.Points, course.Point{Lat: 10, Lon: 20 + float64(i)*step})
		}
		return c
	}
	st := &store{root: "nowhere"}

	n, short := summaryNeed(st, line(100, 0.0001), 0, true)
	if !short || len(n.areas) != 1 || n.zoom != 14 {
		t.Errorf("a 1 km run: %+v, short %v; want its one area at zoom 14", n, short)
	}
	if a := n.areas[0]; a.West > 20 || a.East < 20.0099 {
		t.Errorf("the area %+v does not hold the course", a)
	}

	n, short = summaryNeed(st, line(100, 0.1), 0, true) // 1100 km
	if !short || len(n.areas) != 2 {
		t.Fatalf("a flight: %+v; want the two ends", n)
	}
	if a, b := n.areas[0], n.areas[1]; a.East > 20.1 || b.West < 29.8 {
		t.Errorf("the ends %+v %+v are not round the start and the finish", a, b)
	}

	st.boundaries = nil
	if _, short := summaryNeed(st, line(100, 0.0001), locate.Water, false); short {
		t.Error("water, which only boundaries answer, asked for tiles")
	}
}
