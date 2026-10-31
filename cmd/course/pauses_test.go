package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/units"

	"github.com/wisborg/course"
)

// Every value here is invented: a run north from 10°N 20°E.

// timedRun is 20 minutes at 3 m/s, the timer stopped for two minutes
// while walking 60 m on, and 10 more minutes; a fix a second, each with
// the distance the watch counted, which stood still over the pause.
func timedRun() *course.Course {
	t0 := time.Date(2026, 4, 2, 6, 0, 0, 0, time.UTC)
	tr := &fitactivity.Track{Timing: fitactivity.ActivityTiming{Start: t0, Events: []fitactivity.TimerEvent{
		{Time: t0, Start: true}, {Time: t0.Add(20 * time.Minute)}, {Time: t0.Add(22 * time.Minute), Start: true},
		{Time: t0.Add(32 * time.Minute)}}}}
	c := &course.Course{Sport: "running", Timed: true, Start: t0, Track: tr}
	north, dist := 0.0, 0.0
	for i := 0; i <= 32*60; i++ {
		switch {
		case i > 22*60:
			north, dist = north+3, dist+3
		case i > 20*60:
			// Walking on with the watch stopped: no fixes until it starts
			// again.
			if north += 0.5; i < 22*60 {
				continue
			}
		case i > 0:
			north, dist = north+3, dist+3
		}
		at := t0.Add(time.Duration(i) * time.Second)
		tr.Samples = append(tr.Samples, fitactivity.Sample{Time: at, HasGPS: true, Lat: 10 + north/111195, Lon: 20})
		c.Points = append(c.Points, course.Point{Lat: 10 + north/111195, Lon: 20, Elapsed: time.Duration(i) * time.Second,
			HasDistance: true, Distance: dist})
	}
	return c
}

// A pause is placed where the timer stopped -- by elapsed time, by the
// timer's own, and along the course -- and where it started again; the
// file's closing stop is where it ends, not a pause.
func TestPauses(t *testing.T) {
	rows, located := pauses(timedRun())
	if !located || len(rows) != 1 {
		t.Fatalf("%v, %+v", located, rows)
	}
	r := rows[0]
	if r.elapsed != 20*time.Minute || r.resumed != 22*time.Minute || r.active != 20*time.Minute {
		t.Errorf("elapsed %s, resumed %s, active %s", r.elapsed, r.resumed, r.active)
	}
	if r.distance != 3600 || r.resumedAt != 3600 {
		t.Errorf("distance %.0f, resumed at %.0f", r.distance, r.resumedAt)
	}
	if r.moved < 59 || r.moved > 61 {
		t.Errorf("moved %.1f m, want 60", r.moved)
	}

	var b bytes.Buffer
	m, _ := units.Of(units.Metric)
	writePauses(&b, timedRun(), rows, located, m)
	for _, want := range []string{"pauses: 1, 0:02:00 in all", "0:20:00", "3.60", "06:20:00", "06:22:00", "60 m"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("no %q in\n%s", want, b.String())
		}
	}
	j := pausesJSON(rows, located)
	if j.TotalS != 120 || len(j.Intervals) != 1 || j.Intervals[0].DistanceM != 3600 || j.Intervals[0].ActiveS != 1200 {
		t.Errorf("%+v", j)
	}
}

// With no timer events there is nothing to locate a pause from, which is
// not the same as having none; a timer that ran throughout has none; and a
// planned route has no timer.
func TestPauses_None(t *testing.T) {
	m, _ := units.Of(units.Metric)
	say := func(c *course.Course) string {
		rows, located := pauses(c)
		var b bytes.Buffer
		writePauses(&b, c, rows, located, m)
		return b.String()
	}
	c := timedRun()
	c.Track.Timing.Events = nil
	if got := say(c); !strings.Contains(got, "carries no timer events") {
		t.Errorf("no events: %q", got)
	}
	c.Track.Timing.Events = []fitactivity.TimerEvent{{Time: c.Start, Start: true}, {Time: c.Start.Add(32 * time.Minute)}}
	if got := say(c); !strings.Contains(got, "the timer ran throughout") {
		t.Errorf("never paused: %q", got)
	}
	c.Track, c.Timed = nil, false
	if got := say(c); !strings.Contains(got, "planned route") {
		t.Errorf("a plan: %q", got)
	}
	if j := pausesJSON(nil, false); j.TimerEvents || j.Intervals == nil {
		t.Errorf("%+v", j)
	}
}
