package main

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
	"github.com/wisborg/course/routemap"
)

// paceAround is how far either side of a point its pace is taken over, in
// metres: a few GPS fixes, so one fix out of place does not colour the
// course on its own -- the same as a comparison's.
const paceAround = compareAround

// spreadTail is the share of a metric's values at each end left out of the
// scale's range: a sprint finish, a standstill at a crossing, a GPS spike in
// the elevation are each at an end of the scale, and do not squeeze every
// other value into its middle.
const spreadTail = 0.05

// minElevationSpan is the least range of height a course is coloured over, in
// metres: on a flat course the whole ramp over two metres would colour the
// barometer's drift as hills.
const minElevationSpan = 20.0

// minHeartRateSpan is the least range of heart rate a course is coloured
// over, in beats a minute: a steady easy run is one or two colours, not the
// whole ramp spent on a few beats of drift.
const minHeartRateSpan = 10.0

// minCadenceSpan is the least range of cadence a course is coloured over, in
// its unit: an even run's few steps a minute of wobble are one colour.
const minCadenceSpan = 10.0

// minAirPowerSpan is the least range of air power a course is coloured over,
// in watts: on a still day air power is a few watts that differ by one or
// two, and the whole ramp over them would colour noise as wind.
const minAirPowerSpan = 10.0

// anyKnown reports whether any of values is known.
func anyKnown(values []float64) bool {
	_, _, ok := routemap.Spread(values, 0)
	return ok
}

// minPowerSpread is the least range of power a course is coloured over, as a
// share of its middle either side, as minPaceSpread is for speed.
const minPowerSpread = 0.05

// minPaceSpread is the least range of speed a course is coloured over, as a
// share of its middle speed either side: an even run is one colour, not
// every shade of the ramp spent on a few seconds a kilometre.
const minPaceSpread = 0.05

// colourOptions are the flags that say what to colour a course by.
type colourOptions struct {
	metric  string // --colour
	compare string // --compare
	// gradeCap is --grade-cap, in per cent; gradeCapGiven whether it was
	// typed rather than left at its default.
	gradeCap      float64
	gradeCapGiven bool
	// power is --power-source; powerGiven whether it was typed.
	power      string
	powerGiven bool
}

// powerSources are --power-source's values: the same flag, the same three
// values meaning the same things, as videofx and fitdash take, so what a user
// has learned in one of the three works in the others.
var powerSources = map[string]fitactivity.PowerSource{
	"auto":   fitactivity.PowerAuto,
	"stryd":  fitactivity.PowerStryd,
	"native": fitactivity.PowerNative,
}

// checkColour refuses a --colour the course cannot be coloured by, before
// any map is fetched or drawn: an unknown metric, one the file does not
// record at all, or --colour with --compare, which colours by something else.
// It refuses a --grade-cap or --power-source that is not one, and either one
// given without the --colour it is for, which would otherwise be silently
// ignored.
func checkColour(c *course.Course, o colourOptions) error {
	if o.gradeCapGiven && o.metric != "grade" {
		return errors.New("--grade-cap is for --colour grade")
	}
	if !(o.gradeCap > 0) {
		return fmt.Errorf("--grade-cap %v: the cap is a grade in per cent, more than 0", o.gradeCap)
	}
	if o.powerGiven && o.metric != "power" {
		return errors.New("--power-source is for --colour power")
	}
	src, ok := powerSources[o.power]
	if !ok {
		// Refused rather than taken as auto: a typo would colour the
		// course by the other sensor, which reads a quarter lower or
		// higher -- enough to be taken for a different run.
		return fmt.Errorf("--power-source %q is invalid; use auto, stryd, or native", o.power)
	}
	switch o.metric {
	case "":
		return nil
	case "pace":
		if !c.Timed {
			return errors.New("--colour pace: the course has no times, so no pace")
		}
	case "elevation", "grade":
		if _, _, ok := routemap.Spread(routemap.Elevation(c), 0); !ok {
			return fmt.Errorf("--colour %s: the course records no elevation", o.metric)
		}
	case "cadence":
		if v, _ := routemap.Cadence(c); !anyKnown(v) {
			return errors.New("--colour cadence: the course records no cadence")
		}
	case "air-power":
		if !anyKnown(routemap.AirPower(c)) {
			return errors.New("--colour air-power: the course records no air power, which is a Stryd footpod's estimate")
		}
	case "grade-adjusted-pace":
		if !c.Timed {
			return errors.New("--colour grade-adjusted-pace: the course has no times, so no pace")
		}
		if !anyKnown(routemap.Elevation(c)) {
			return errors.New("--colour grade-adjusted-pace: the course records no elevation, so no grade to adjust for")
		}
	case "heart-rate":
		if _, _, ok := routemap.Spread(routemap.HeartRate(c), 0); !ok {
			return errors.New("--colour heart-rate: the course records no heart rate")
		}
	case "power":
		if _, _, ok := routemap.Spread(routemap.Power(c, src), 0); !ok {
			if src == fitactivity.PowerAuto {
				return errors.New("--colour power: the course records no power")
			}
			return fmt.Errorf("--colour power: the course records no %s power; --power-source auto takes whichever it has", src)
		}
	default:
		return fmt.Errorf("--colour %q: colour by %s", o.metric, colourMetrics)
	}
	if o.compare != "" {
		return errors.New("--colour and --compare both colour the course; use one")
	}
	return nil
}

// colourBy colours c by metric along it, on a scale from the slowest or
// lowest of it to the fastest or highest, with the ends' values in the
// legend.
//
// gradeCap is the most a grade scale reaches either way, as a fraction:
// --grade-cap, 15% unless told otherwise. Below it the scale reaches the
// course's own steepest grade, so a gentle course is not one green; at it,
// steeper ground takes the end colour and the legend says so. Without a cap
// a staircase at -26% stretched the scale until a 14% bridge ramp was yellow
// and the rest of a half marathon one green. A mountain hike with long
// stretches steeper than 15% wants a higher one, or they are all one colour.
func colourBy(cs []*course.Course, name string, o colourOptions, scale float64) (*colouring, error) {
	// Each metric's values, course by course, and one scale over them all:
	// on a map of several activities the same colour is the same pace, or
	// grade, on each.
	all := func(of func(c *course.Course) []float64) [][]float64 {
		vs := make([][]float64, len(cs))
		for k, c := range cs {
			vs[k] = of(c)
		}
		return vs
	}
	spread := func(vs [][]float64) (float64, float64, bool) { return routemap.SpreadAlongAll(cs, vs, spreadTail) }
	byShare := func(lo, hi, share float64) (float64, float64) {
		return routemap.Widen(lo, hi, 2*share*(lo+hi)/2)
	}
	number := func(unit string) func(float64) string {
		return func(v float64) string { return fmt.Sprintf("%.0f %s", v, unit) }
	}
	plain := func(lo, hi float64, text func(float64) string) ramp {
		return ramp{scale: render.Scale{Min: lo, Max: hi}, low: text(lo), high: text(hi)}
	}

	var label string
	var vs [][]float64
	var r ramp
	peaks := false
	switch o.metric {
	case "pace", "grade-adjusted-pace":
		label = "pace"
		of := func(c *course.Course) []float64 { return routemap.Speed(c, paceAround) }
		if o.metric == "grade-adjusted-pace" {
			label = "grade-adjusted pace"
			of = func(c *course.Course) []float64 { return routemap.GradeAdjustedSpeed(c, paceAround) }
		}
		vs = all(of)
		lo, hi, ok := spread(vs)
		if !ok {
			return nil, fmt.Errorf("--colour %s: the course never moves in its clock, or has too little elevation", o.metric)
		}
		lo, hi = byShare(lo, hi, minPaceSpread)
		r = plain(lo, hi, paceText)
	case "elevation":
		label, vs = "elevation", all(routemap.Elevation)
		lo, hi, _ := spread(vs)
		lo, hi = routemap.Widen(lo, hi, minElevationSpan)
		r = plain(lo, hi, number("m"))
	case "grade":
		label = "grade"
		vs = all(func(c *course.Course) []float64 { return routemap.Grade(c, gradeWindow) })
		lo, hi, ok := routemap.Spread(slices.Concat(vs...), 0)
		if !ok {
			return nil, errors.New("--colour grade: the course has too little elevation to take a slope from")
		}
		// As far as the course's steepest grade either way, level in the
		// middle; but no less than minGrade, and no more than the cap,
		// past which the ends' colours say "this steep or steeper".
		steepest := max(math.Abs(lo), math.Abs(hi))
		reach := min(max(steepest, minGrade), o.gradeCap/100)
		r = ramp{scale: render.Scale{Min: -reach, Max: reach}, low: gradeText(-reach), high: gradeText(reach)}
		if steepest > o.gradeCap/100 {
			r.low, r.high = "≤"+r.low, "≥"+r.high
		}
		// Each piece of the line by its steepest grade, not its average:
		// at a whole course's zoom a piece is tens of metres, and a
		// staircase averaged with the level ground either side is drawn
		// as a gentle slope.
		peaks = true
	case "cadence":
		label = "cadence"
		var unit string
		for _, c := range cs {
			_, u := routemap.Cadence(c)
			if unit != "" && u != unit {
				return nil, errors.New("--colour cadence: the activities count cadence in different units -- steps a minute and revolutions a minute -- and cannot share a scale")
			}
			unit = u
		}
		vs = all(func(c *course.Course) []float64 {
			raw, _ := routemap.Cadence(c)
			return routemap.Around(c, raw, paceAround)
		})
		lo, hi, _ := spread(vs)
		lo, hi = routemap.Widen(lo, hi, minCadenceSpan)
		r = plain(lo, hi, number(unit))
	case "air-power":
		label = "air power"
		vs = all(func(c *course.Course) []float64 { return routemap.Around(c, routemap.AirPower(c), paceAround) })
		lo, hi, _ := spread(vs)
		lo, hi = routemap.Widen(lo, hi, minAirPowerSpan)
		r = plain(lo, hi, number("W"))
	case "heart-rate":
		label = "heart rate"
		vs = all(func(c *course.Course) []float64 { return routemap.Around(c, routemap.HeartRate(c), paceAround) })
		lo, hi, _ := spread(vs)
		lo, hi = routemap.Widen(lo, hi, minHeartRateSpan)
		r = plain(lo, hi, number("bpm"))
	case "power":
		src := powerSources[o.power]
		label = powerSensor(cs, src) + " power"
		vs = all(func(c *course.Course) []float64 { return routemap.Around(c, routemap.Power(c, src), paceAround) })
		lo, hi, _ := spread(vs)
		lo, hi = byShare(lo, hi, minPowerSpread)
		r = plain(lo, hi, number("W"))
	default:
		return nil, fmt.Errorf("--colour %q: colour by %s", o.metric, colourMetrics)
	}

	col := &colouring{
		legend: name + ", " + label,
		ramp:   r,
		report: fmt.Sprintf("%-10s by %s, %s (blue) to %s (red)", "coloured", label, r.low, r.high),
	}
	for k, c := range cs {
		for _, g := range routemap.Gradients(c, vs[k], r.scale, scale) {
			g.Peaks = peaks
			col.gradients = append(col.gradients, g)
		}
	}
	return col, nil
}

// powerSensor is which sensor's power src takes on c, in words for the
// legend and the report: the two read a quarter apart on the same run, so a
// power map that does not say which it is cannot be read against another.
// auto takes the footpod's wherever it has one, so a course with any footpod
// power at all is coloured by it.
func powerSensor(cs []*course.Course, src fitactivity.PowerSource) string {
	if src == fitactivity.PowerNative {
		return "native"
	}
	if src == fitactivity.PowerStryd {
		return "Stryd"
	}
	for _, c := range cs {
		for _, p := range c.Points {
			if p.HasStrydPower {
				return "Stryd"
			}
		}
	}
	return "native"
}

// colourMetrics are what --colour takes, in words.
const colourMetrics = "pace, grade-adjusted-pace, elevation, grade, heart-rate, power, air-power or cadence"

// gradeWindow is how far either side of a point its grade is taken over, in
// metres. Narrower than the 30 m videofx and fitdash read a grade over:
// theirs is a number on screen that must not jump about from frame to
// frame, this is a map meant to show where the steep pitches are, and they
// are short. A staircase dropping 7.8 m in 22 m read -14% over 30 m either
// side and -26% over 10; a bridge ramp of 75 m at 14% read the same either
// way, and a half marathon had no more pitches over 10% than before. The
// elevation is smoothed first all the same, tuned as the siblings tune it,
// so the altimeter's own noise is gone before the window is taken.
const gradeWindow = 10.0

// minGrade is the least slope the grade scale reaches either way: on a
// course with nothing steeper than 1%, the ends of the ramp are not spent on
// what is, to a runner, level ground.
const minGrade = 0.03

// gradeText is a grade the way videofx writes one: a signed percentage.
func gradeText(g float64) string { return fmt.Sprintf("%+.1f%%", 100*g) }

// paceText is a speed as a pace, the way videofx writes one: minutes and
// seconds a kilometre.
func paceText(speed float64) string {
	if !(speed > 0) {
		return "-"
	}
	s := int(math.Round(1000 / speed))
	return fmt.Sprintf("%d:%02d/km", s/60, s%60)
}
