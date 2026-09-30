package main

import (
	"errors"
	"fmt"
	"math"

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

// minPaceSpread is the least range of speed a course is coloured over, as a
// share of its middle speed either side: an even run is one colour, not
// every shade of the ramp spent on a few seconds a kilometre.
const minPaceSpread = 0.05

// checkColour refuses a --colour the course cannot be coloured by, before
// any map is fetched or drawn: an unknown metric, one the file does not
// record at all, or --colour with --compare, which colours by something else.
func checkColour(c *course.Course, metric, compareWith string) error {
	switch metric {
	case "":
		return nil
	case "pace":
		if !c.Timed {
			return errors.New("--colour pace: the course has no times, so no pace")
		}
	case "elevation":
		if _, _, ok := routemap.Spread(routemap.Elevation(c), 0); !ok {
			return errors.New("--colour elevation: the course records no elevation")
		}
	default:
		return fmt.Errorf("--colour %q: colour by pace or elevation", metric)
	}
	if compareWith != "" {
		return errors.New("--colour and --compare both colour the course; use one")
	}
	return nil
}

// colourBy colours c by metric along it, on a scale from the slowest or
// lowest of it to the fastest or highest, with the ends' values in the
// legend.
func colourBy(c *course.Course, name, metric string, scale float64) (*colouring, error) {
	switch metric {
	case "pace":
		speed := routemap.Speed(c, paceAround)
		lo, hi, ok := routemap.Spread(speed, spreadTail)
		if !ok {
			return nil, errors.New("--colour pace: the course never moves in its clock")
		}
		mid := (lo + hi) / 2
		lo, hi = routemap.Widen(lo, hi, 2*minPaceSpread*mid)
		r := ramp{scale: render.Scale{Min: lo, Max: hi}, low: paceText(lo), high: paceText(hi)}
		return &colouring{
			gradients: routemap.Gradients(c, speed, r.scale, scale),
			legend:    name + ", pace",
			ramp:      r,
			report:    fmt.Sprintf("%-10s by pace, %s (blue) to %s (red)", "coloured", r.low, r.high),
		}, nil
	case "elevation":
		height := routemap.Elevation(c)
		lo, hi, _ := routemap.Spread(height, spreadTail)
		lo, hi = routemap.Widen(lo, hi, minElevationSpan)
		r := ramp{scale: render.Scale{Min: lo, Max: hi}, low: fmt.Sprintf("%.0f m", lo), high: fmt.Sprintf("%.0f m", hi)}
		return &colouring{
			gradients: routemap.Gradients(c, height, r.scale, scale),
			legend:    name + ", elevation",
			ramp:      r,
			report:    fmt.Sprintf("%-10s by elevation, %s (blue) to %s (red)", "coloured", r.low, r.high),
		}, nil
	}
	return nil, fmt.Errorf("--colour %q: colour by pace or elevation", metric)
}

// paceText is a speed as a pace, the way videofx writes one: minutes and
// seconds a kilometre.
func paceText(speed float64) string {
	if !(speed > 0) {
		return "-"
	}
	s := int(math.Round(1000 / speed))
	return fmt.Sprintf("%d:%02d/km", s/60, s%60)
}
