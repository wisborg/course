package main

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/units"

	"github.com/wisborg/course"
)

// pauseRow is one stretch the recording was paused: when the timer
// stopped and started again, where the course was either side, and how far
// apart those are.
type pauseRow struct {
	from, to time.Time
	// elapsed and resumed are the wall-clock times since the start at
	// either end, pauses included; active is the time the timer had run
	// when it stopped, as the watch showed it.
	elapsed, resumed, active time.Duration
	// distance and resumedAt are how far along the course it was at
	// either end, by course.Along; they differ where the device counted a
	// distance across the pause.
	distance, resumedAt float64
	lat, lon            float64
	resumedLat          float64
	resumedLon          float64
	// moved is how far apart, in a straight line, the fixes either side
	// of the pause are: a walk to a café with the watch stopped, a ferry.
	moved float64
}

// pauses are c's pauses and whether its recording carries the timer
// events they are read from: a planned route, and an activity with no
// timer events -- one read from Strava -- have none to locate.
func pauses(c *course.Course) ([]pauseRow, bool) {
	if c.Track == nil || !c.Timed || len(c.Points) == 0 {
		return nil, false
	}
	timer := fitactivity.BuildTimerModel(c.Track)
	if !timer.HasTimerEvents() {
		return nil, false
	}
	along := c.Along()
	var rows []pauseRow
	for _, p := range timer.Pauses() {
		from, to := p.Start.Sub(c.Start), p.End.Sub(c.Start)
		// The last fix at or before the stop, and the first at or after
		// the start again.
		i := sort.Search(len(c.Points), func(k int) bool { return c.Points[k].Elapsed > from }) - 1
		j := sort.Search(len(c.Points), func(k int) bool { return c.Points[k].Elapsed >= to })
		i = max(i, 0)
		if j >= len(c.Points) {
			j = len(c.Points) - 1
		}
		j = max(i, j)
		a, b := c.Points[i], c.Points[j]
		rows = append(rows, pauseRow{from: p.Start, to: p.End, elapsed: from, resumed: to, active: timer.Active(p.Start),
			distance: along[i], resumedAt: along[j], lat: a.Lat, lon: a.Lon, resumedLat: b.Lat, resumedLon: b.Lon,
			moved: course.Metres(a.Lat, a.Lon, b.Lat, b.Lon)})
	}
	return rows, true
}

// writePauses writes the pauses as a table in u: when each began, by the
// elapsed and the timer's clock and as a time of day, how far along, how
// long it lasted and when it ended, and how far the course moved over it;
// or says there are none, or that none can be located.
func writePauses(w io.Writer, c *course.Course, rows []pauseRow, located bool, u units.Set) {
	fmt.Fprintln(w)
	switch {
	case c.Track == nil:
		fmt.Fprintln(w, "pauses: none, as a planned route has no timer to pause")
		return
	case !located:
		fmt.Fprintln(w, "pauses: none can be located, as the recording carries no timer events (a GPX or TCX carries none, nor does an activity read from Strava)")
		return
	case len(rows) == 0:
		fmt.Fprintln(w, "pauses: none; the timer ran throughout")
		return
	}
	var total time.Duration
	for _, r := range rows {
		total += r.to.Sub(r.from)
	}
	s := "s"
	if len(rows) == 1 {
		s = ""
	}
	fmt.Fprintf(w, "pauses: %d, %s in all\n", len(rows), clock(total))
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(tw, "\telapsed\tactive\t%s\tpaused UTC\tresumed UTC\tfor\tmoved\t\n", u.Distance.Name)
	for i, r := range rows {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%.2f\t%s\t%s\t%s\t%s\t\n", i+1, clock(r.elapsed), clock(r.active),
			u.Distance.FromSI(r.distance), r.from.UTC().Format("15:04:05"), r.to.UTC().Format("15:04:05"),
			clock(r.to.Sub(r.from)), short(r.moved, u))
	}
	tw.Flush()
	fmt.Fprintf(w, "  elapsed is the time since the start, pause%s included, and active the timer's own; "+
		"moved is how far apart the fixes either side are\n", s)
}

// jsonPause is a pauseRow for JSON and YAML, in SI units.
type jsonPause struct {
	PausedAt         time.Time `json:"paused_at" yaml:"paused_at"`
	ResumedAt        time.Time `json:"resumed_at" yaml:"resumed_at"`
	DurationS        float64   `json:"duration_s" yaml:"duration_s"`
	ElapsedS         float64   `json:"elapsed_s" yaml:"elapsed_s"`
	ResumedElapsedS  float64   `json:"resumed_elapsed_s" yaml:"resumed_elapsed_s"`
	ActiveS          float64   `json:"active_s" yaml:"active_s"`
	DistanceM        float64   `json:"distance_m" yaml:"distance_m"`
	ResumedDistanceM float64   `json:"resumed_distance_m" yaml:"resumed_distance_m"`
	Latitude         float64   `json:"latitude" yaml:"latitude"`
	Longitude        float64   `json:"longitude" yaml:"longitude"`
	ResumedLatitude  float64   `json:"resumed_latitude" yaml:"resumed_latitude"`
	ResumedLongitude float64   `json:"resumed_longitude" yaml:"resumed_longitude"`
	MovedM           float64   `json:"moved_m" yaml:"moved_m"`
}

// jsonPauses are the pauses for JSON and YAML: whether the recording
// carries the timer events they are read from, their total, and each.
type jsonPauses struct {
	TimerEvents bool        `json:"timer_events" yaml:"timer_events"`
	TotalS      float64     `json:"total_s" yaml:"total_s"`
	Intervals   []jsonPause `json:"intervals" yaml:"intervals"`
}

func pausesJSON(rows []pauseRow, located bool) *jsonPauses {
	j := &jsonPauses{TimerEvents: located, Intervals: []jsonPause{}}
	for _, r := range rows {
		d := r.to.Sub(r.from).Seconds()
		j.TotalS += d
		j.Intervals = append(j.Intervals, jsonPause{PausedAt: r.from.UTC(), ResumedAt: r.to.UTC(), DurationS: d,
			ElapsedS: r.elapsed.Seconds(), ResumedElapsedS: r.resumed.Seconds(), ActiveS: r.active.Seconds(),
			DistanceM: r.distance, ResumedDistanceM: r.resumedAt, Latitude: r.lat, Longitude: r.lon,
			ResumedLatitude: r.resumedLat, ResumedLongitude: r.resumedLon, MovedM: r.moved})
	}
	return j
}
