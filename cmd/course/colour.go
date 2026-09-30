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
	case "elevation", "grade":
		if _, _, ok := routemap.Spread(routemap.Elevation(c), 0); !ok {
			return fmt.Errorf("--colour %s: the course records no elevation", metric)
		}
	default:
		return fmt.Errorf("--colour %q: colour by %s", metric, colourMetrics)
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
	case "grade":
		grade := routemap.Grade(c, gradeWindow)
		if _, _, ok := routemap.Spread(grade, 0); !ok {
			return nil, errors.New("--colour grade: the course has too little elevation to take a slope from")
		}
		r := ramp{
			scale: render.Scale{Min: -gradeReach, Max: gradeReach},
			low:   "≤" + gradeText(-gradeReach),
			high:  "≥" + gradeText(gradeReach),
		}
		gs := routemap.Gradients(c, grade, r.scale, scale)
		for i := range gs {
			// Each piece of the line by its steepest grade, not its
			// average: at a whole course's zoom a piece is tens of
			// metres, and a staircase averaged with the level ground
			// either side is drawn as a gentle slope.
			gs[i].Peaks = true
		}
		return &colouring{
			gradients: gs,
			legend:    name + ", grade",
			ramp:      r,
			report:    fmt.Sprintf("%-10s by grade, %s (blue) to %s (red)", "coloured", r.low, r.high),
		}, nil
	}
	return nil, fmt.Errorf("--colour %q: colour by %s", metric, colourMetrics)
}

// colourMetrics are what --colour takes, in words.
const colourMetrics = "pace, elevation or grade"

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

// gradeReach is how steep a slope the grade scale tells apart, either way:
// level ground in the middle, and 15% or steeper at each end. The same on
// every map, because a grade means the same on any course -- unlike pace or
// height, which have no common range -- and so one colour is one slope from
// a map to the next. Reaching to each course's own steepest did not work: a
// staircase at -26% stretched the scale until a 14% bridge ramp was yellow
// and the rest of a half marathon one green. At 15%, the ramp is red, the
// stairs are blue past the end, and a 5% hill is plainly tinted.
const gradeReach = 0.15

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
