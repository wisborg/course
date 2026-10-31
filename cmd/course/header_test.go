package main

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	fw "github.com/wisborg/fitactivity/weather"

	"github.com/wisborg/course"
)

// Every value here is invented: a run north from 10°N 20°E.

// paused is 20 minutes at 3 m/s, a two-minute stop, and 10 more minutes,
// with a heart rate on the first half.
func paused() *course.Course {
	c := &course.Course{Sport: "running", Timed: true, Start: time.Date(2026, 4, 2, 6, 0, 0, 0, time.UTC),
		HasElevationTotals: true, TotalAscent: 42, TotalDescent: 40}
	north := 0.0
	for i := 0; i <= 32*60; i++ {
		if i > 0 && (i <= 20*60 || i > 22*60) {
			north += 3
		}
		p := course.Point{Lat: 10 + north/111195, Lon: 20, Elapsed: time.Duration(i) * time.Second}
		if i < 16*60 {
			p.HasHeartRate, p.HeartRate = true, 140+float64(i%20)
		}
		c.Points = append(c.Points, p)
	}
	return c
}

// The stats are worked out over the time spent moving, a stop left out;
// auto gives those the course has, and a stat named that it lacks is
// refused, as --colour refuses one.
func TestStatsOf(t *testing.T) {
	c := paused()
	m, _ := units.Of(units.Metric)
	got, err := statsOf([]string{"auto", "moving-time", "max-heart-rate"}, c, m, statSources{})
	if err != nil {
		t.Fatal(err)
	}
	want := []statLine{{"distance", "5.40 km"}, {"time", "0:32:00"}, {"pace", "5:33/km"}, {"ascent", "42 m"},
		{"heart rate", "150 bpm"}, {"moving time", "0:30:00"}, {"max heart rate", "159 bpm"}}
	if len(got) != len(want) {
		t.Fatalf("%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: %v, want %v", i, got[i], want[i])
		}
	}
	if _, err := statsOf([]string{"power"}, c, m, statSources{}); err == nil || !strings.Contains(err.Error(), "nothing to work it out from") {
		t.Errorf("power without power: %v", err)
	}
	if _, err := statsOf([]string{"vo2max"}, c, m, statSources{}); err == nil || !strings.Contains(err.Error(), "is not one of auto") {
		t.Errorf("an unknown stat: %v", err)
	}
	c.Sport = "cycling"
	if got, _ := statsOf([]string{"auto"}, c, m, statSources{}); got[2].caption != "speed" || got[2].value != "11 km/h" {
		t.Errorf("a ride's auto: %v", got)
	}
}

// The header is nothing when nothing is asked for; laid out, the title
// and the rest share a row where they fit and take rows of their own where
// they do not, and the map is drawn under it in the height left.
func TestHeaderLayout(t *testing.T) {
	if b := (header{}).layout(1600, 13, 1.6); b.height != 0 {
		t.Errorf("an empty header %d tall", b.height)
	}
	stats := []statLine{{"distance", "42.20 km"}, {"time", "3:58:12"}, {"pace", "5:39/km"}}
	wide := header{title: "Marathon", stats: stats}.layout(1600, 13, 1.6)
	narrow := header{title: "Marathon", stats: stats}.layout(500, 13, 0.5*1.6)
	tall := header{title: "Marathon", subtitle: "20 April", stats: append(stats, stats...),
		weather: &headerWeather{hasSky: true, sky: fw.Rain, day: true, caption: "light rain", temperature: "10°C rising to 22°C",
			detail: "wind 14 km/h from WSW · dew point 6°C · Open-Meteo.com"}}.layout(1000, 13, 1)
	if wide.height == 0 || tall.height <= wide.height {
		t.Errorf("heights: wide %d, tall %d", wide.height, tall.height)
	}
	if narrow.height == 0 {
		t.Error("narrow: no band")
	}

	m := image.NewRGBA(image.Rect(0, 0, 1000, 600))
	pic := withHeader(m, tall)
	if pic.Bounds().Dy() != 600+tall.height {
		t.Errorf("picture %v under a band of %d", pic.Bounds(), tall.height)
	}
	if got := pic.RGBAAt(500, 2); got != (color.RGBA{0xff, 0xff, 0xff, 0xff}) {
		t.Errorf("band background %v", got)
	}
	if err := headerRoom(tall.height, 2*tall.height-2); err == nil {
		t.Error("a header over half the picture accepted")
	}
}

// Asked for on a planned route, the weather is said to be impossible, not
// fetched.
func TestMapHeader_PlannedRoute(t *testing.T) {
	defer func(w bool, s []string) { mapOpts.weather, mapOpts.stats = w, s }(mapOpts.weather, mapOpts.stats)
	mapOpts.weather, mapOpts.stats = true, nil
	m, _ := units.Of(units.Metric)
	var errs bytes.Buffer
	c := &course.Course{Points: []course.Point{{Lat: 10, Lon: 20}, {Lat: 10.01, Lon: 20}}}
	h, err := mapHeader(&cobra.Command{}, c, "route", m, statSources{}, false, &errs)
	if err != nil || h.weather != nil || !strings.Contains(errs.String(), "a planned route has no times") {
		t.Errorf("header %+v, %v, %q", h, err, errs.String())
	}
}
