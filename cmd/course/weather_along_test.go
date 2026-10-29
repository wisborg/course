package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	cw "github.com/wisborg/cobrakit/weather"
	"github.com/wisborg/fitactivity/units"
	fw "github.com/wisborg/fitactivity/weather"

	"github.com/wisborg/course"
)

// Every value here is invented: a course due north from 10°N 20°E.

// --weather-every takes a distance in km or mi, or a duration; "m" is
// minutes, so a number of them that must have meant metres is refused.
func TestParseEvery(t *testing.T) {
	for s, want := range map[string]every{
		"":      {},
		"2km":   {metres: 2000},
		"2 km":  {metres: 2000},
		"0.5mi": {metres: mile / 2},
		"30m":   {period: 30 * time.Minute},
		"1h30m": {period: 90 * time.Minute},
		"120m":  {period: 2 * time.Hour},
	} {
		if got, err := parseEvery(s); err != nil || got != want {
			t.Errorf("%q: %+v, %v; want %+v", s, got, err, want)
		}
	}
	for s, want := range map[string]string{"500m": "0.5km", "0.05km": "at least 0.1km", "10s": "at least a minute", "2 ft": "not a distance", "fast": "not a distance"} {
		if _, err := parseEvery(s); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", s, err, want)
		}
	}
}

// line is a course north at 3 m/s for d, a point a second, with a steady
// northerly that warms by a degree each 10 minutes.
func line(d time.Duration) (*course.Course, *fw.Report) {
	t0 := time.Date(2026, 4, 2, 6, 0, 0, 0, time.UTC)
	c := &course.Course{Sport: "running", Timed: true, Start: t0}
	for i := 0; i <= int(d.Seconds()); i++ {
		c.Points = append(c.Points, course.Point{Lat: 10 + float64(i)*3/111195, Lon: 20, Elapsed: time.Duration(i) * time.Second})
	}
	var hours []fw.Conditions
	for h := 0; h <= int(d.Hours())+1; h++ {
		hours = append(hours, fw.Conditions{Time: t0.Add(time.Duration(h) * time.Hour), Temperature: 10 + 6*float64(h),
			DewPoint: 5, Humidity: 70, WindSpeed: 5, WindFrom: 0, Gusts: 9, Pressure: 1013, Radiation: 300})
	}
	return c, &fw.Report{Places: []fw.Place{{Lat: 10, Lon: 20, Elevation: 40, Hours: hours}}}
}

// Rows fall at the start, each step along -- by distance or by time -- and
// at the finish, with the weather of the moment the course got there and
// the wind as met.
func TestWeatherAlong(t *testing.T) {
	c, r := line(40 * time.Minute) // 7.2 km
	rows := weatherAlong(r, c, every{metres: 2000})
	if len(rows) != 5 {
		t.Fatalf("%d rows by 2 km over 7.2 km, want 5", len(rows))
	}
	for i, want := range []float64{0, 2000, 4000, 6000, 7200} {
		if d := rows[i].distance; d < want-3 || d > want+3 {
			t.Errorf("row %d at %.0f m, want %.0f", i, d, want)
		}
	}
	if last := rows[4]; last.at.Temperature != 14 || !last.hasHeading || last.head < 4.99 || last.cross > 0.01 {
		t.Errorf("finish %+v", last)
	}

	rows = weatherAlong(r, c, every{period: 15 * time.Minute})
	if len(rows) != 4 || rows[1].elapsed != 15*time.Minute || rows[3].elapsed != 40*time.Minute {
		t.Errorf("by 15 min: %+v", rows)
	}
	if weatherAlong(nil, c, every{metres: 1000}) != nil {
		t.Error("rows without a report")
	}
}

// The table is in the units asked for, with the wind as met; a marathon's
// default is every km, a ride's every 5.
func TestWriteWeatherAlong(t *testing.T) {
	c, r := line(40 * time.Minute)
	m, _ := units.Of(units.Metric)
	if e := defaultEvery("running", m); e.metres != 1000 {
		t.Errorf("running by %v", e)
	}
	if e := defaultEvery("cycling", m); e.metres != 5000 {
		t.Errorf("cycling by %v", e)
	}
	var b strings.Builder
	writeWeatherAlong(&b, weatherAlong(r, c, every{metres: 2000}), m)
	for _, want := range []string{"km/h", "0:40:00", "7.20", "14", "70%", "N", "+18", "0 R", "300"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("lacks %q:\n%s", want, b.String())
		}
	}
}

// In JSON and YAML the rows sit beside cobrakit's summary, in one object;
// the places as fetched only with --weather-raw.
func TestWeatherJSON(t *testing.T) {
	c, r := line(40 * time.Minute)
	rows := weatherAlong(r, c, every{period: 15 * time.Minute})
	j := weatherJSON(&cw.JSON{TemperatureMinC: 10}, rows, every{period: 15 * time.Minute}, r, false)
	data, err := json.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"temperature_min_c":10`, `"along_every_s":900`, `"along":[`, `"headwind_ms":5`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("json lacks %s: %s", want, data)
		}
	}
	if strings.Contains(string(data), `"places"`) {
		t.Errorf("places without --weather-raw: %s", data)
	}

	j = weatherJSON(&cw.JSON{TemperatureMinC: 10}, rows, every{metres: 1000}, r, true)
	y, err := yaml.Marshal(j)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"\ntemperature_min_c: 10", "along_every_m: 1000", "places:", "elevation_m: 40", "hours:"} {
		if !strings.Contains("\n"+string(y), want) {
			t.Errorf("yaml lacks %q:\n%s", want, y)
		}
	}
	if weatherJSON(nil, rows, every{metres: 1000}, r, true) != nil {
		t.Error("weather without a summary")
	}
}
