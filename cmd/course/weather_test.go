package main

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	cw "github.com/wisborg/cobrakit/weather"
	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/units"
	fw "github.com/wisborg/fitactivity/weather"

	"github.com/wisborg/course"
)

// Every value here is invented: a run near 10°N 20°E.

// A planned route has no hours to ask the weather for, and says so rather
// than asking.
func TestResolveWeather_PlannedRoute(t *testing.T) {
	defer func(o cw.Options) { summaryOpts.weather = o }(summaryOpts.weather)
	summaryOpts.weather = cw.Options{Weather: true, Exposure: cw.ExposureSuburban}
	p, err := resolveWeather(&cobra.Command{}, &course.Course{})
	if err != nil || !p.Asked || p.Report != nil || !strings.Contains(p.Reason, "planned route") {
		t.Errorf("plan %+v, %v", p, err)
	}
	summaryOpts.weather.Exposure = "windy"
	if _, err := resolveWeather(&cobra.Command{}, &course.Course{}); err == nil {
		t.Error("--wind-exposure windy accepted")
	}
}

// --detailed gives the weather in the line every program says it in, with
// the credit, and on a run what it cost.
func TestWriteWeather(t *testing.T) {
	t0 := time.Date(2026, 4, 2, 6, 0, 0, 0, time.UTC)
	tr := &fitactivity.Track{Sport: "running"}
	for i := 0; i <= 1200; i++ {
		tr.Samples = append(tr.Samples, fitactivity.Sample{Time: t0.Add(time.Duration(i) * time.Second),
			HasGPS: true, Lat: 10 + float64(i)*3/111195, Lon: 20, HasSpeed: true, Speed: 3})
	}
	hours := []fw.Conditions{
		{Time: t0, Temperature: 22, DewPoint: 12, Humidity: 53, WindSpeed: 5, WindFrom: 0, Pressure: 1013},
		{Time: t0.Add(time.Hour), Temperature: 22, DewPoint: 12, Humidity: 53, WindSpeed: 5, WindFrom: 0, Pressure: 1013},
	}
	p := cw.Plan{Asked: true, Report: &fw.Report{Places: []fw.Place{{Lat: 10, Lon: 20, Hours: hours}}}}
	if p.Exposure, _ = cw.ParseExposure(cw.ExposureSuburban); p.Exposure == 0 {
		t.Fatal("no exposure")
	}
	c := &course.Course{Sport: "running", Track: tr}
	m, _ := units.Of(units.Metric)
	costs, ok := weatherCosts(p, c, m)
	var b strings.Builder
	writeWeather(&b, p, c, costs, ok, m)
	for _, want := range []string{"weather   22°C, dew point 12°C, wind 18 km/h from N (Weather data by Open-Meteo.com)",
		"the wind cost about", "into it, never behind", "the heat cost about"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("lacks %q:\n%s", want, b.String())
		}
	}
	if j := cw.JSONOf(p, tr, costs, ok); j == nil || j.WindCostS == nil || *j.WindCostS <= 0 {
		t.Errorf("json %+v", j)
	}
}
