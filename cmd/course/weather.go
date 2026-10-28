package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	cw "github.com/wisborg/cobrakit/weather"
	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/units"
	fw "github.com/wisborg/fitactivity/weather"

	"github.com/wisborg/course"
)

// resolveWeather fetches the weather c was done in, when --weather asks,
// through cobrakit's weather, as fitdash does: a planned route has no
// hours to ask about, and a flight's are above the ground's weather.
func resolveWeather(cmd *cobra.Command, c *course.Course) (cw.Plan, error) {
	o := summaryOpts.weather
	if c.Track == nil && o.Weather {
		x, err := cw.ParseExposure(o.Exposure)
		if err != nil {
			return cw.Plan{}, err
		}
		a, err := cw.ParseAcclimatised(o.Acclimatised)
		if err != nil {
			return cw.Plan{}, err
		}
		return cw.Plan{Asked: true, Reason: "a planned route has no times to ask the weather for", Exposure: x, Acclim: a}, nil
	}
	return cw.Resolve(cmd, o, c.Track, len(c.Flights) > 0 || c.Sport == "flying")
}

// weatherCosts is what the conditions cost c, on the elevation model
// tuned to the device's own totals where it has them, as fitdash's
// profile is.
func weatherCosts(p cw.Plan, c *course.Course, u units.Set) (cw.Costs, bool) {
	if c.Track == nil {
		return cw.Costs{}, false
	}
	var opts fitactivity.ElevationOptions
	if c.HasElevationTotals {
		opts.TargetGain, opts.TargetLoss = c.TotalAscent, c.TotalDescent
	}
	return cw.CostsOf(p, c.Track, u, cw.Grade(c.Track, fitactivity.BuildElevationModel(c.Track, opts)))
}

// writeWeather writes, for --detailed, the weather the course was done in
// and what it cost: the line every program says it in, its credit, and
// each cost with what it rests on.
func writeWeather(w io.Writer, p cw.Plan, c *course.Course, costs cw.Costs, hasCosts bool, u units.Set) {
	if p.Report == nil || c.Track == nil {
		return
	}
	s, ok := fw.Summarise(p.Report, c.Track)
	if !ok {
		return
	}
	fmt.Fprintf(w, "\nweather   %s (%s)\n", cw.Line(s, u), fw.Credit)
	cw.WriteCosts(w, costs, hasCosts)
}
