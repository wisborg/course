package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

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
	out := summaryTable(s).String()
	header := strings.Fields(strings.SplitN(out, "\n", 2)[0])
	if strings.Join(header, " ") != "elapsed country water" {
		t.Errorf("header %v", header)
	}
}
