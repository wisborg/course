package routemap

import (
	"image/color"
	"math"
	"sort"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// Stretches are the recorded stretches of a course, as index ranges into its
// points, first and last inclusive: the course split at its gaps, as its
// line is drawn. Nothing measured along a course should reach across a gap,
// where the course is not known to have gone the straight way between.
func Stretches(c *course.Course) [][2]int {
	if len(c.Points) == 0 {
		return nil
	}
	gapAfter := gapThreshold(c)
	var out [][2]int
	from := 0
	for i := 1; i < len(c.Points); i++ {
		p, prev := c.Points[i], c.Points[i-1]
		if gapAfter > 0 && p.Elapsed-prev.Elapsed > gapAfter && course.Metres(prev.Lat, prev.Lon, p.Lat, p.Lon) > 300 {
			out = append(out, [2]int{from, i - 1})
			from = i
		}
	}
	return append(out, [2]int{from, len(c.Points) - 1})
}

// Speed is how fast the course was going at each point, in metres a second:
// the distance over the stretch around metres either side of it, by the
// time it took, within the recorded stretch the point is in. A course with
// no clock has no speed, and a point whose window took no time -- a
// recording that stood still in its clock -- has none either: both NaN, not
// known, rather than zero or infinitely fast.
func Speed(c *course.Course, around float64) []float64 {
	out := make([]float64, len(c.Points))
	for i := range out {
		out[i] = math.NaN()
	}
	if !c.Timed {
		return out
	}
	along := c.Along()
	for _, s := range Stretches(c) {
		lo, hi := s[0], s[0]
		for i := s[0]; i <= s[1]; i++ {
			for along[i]-along[lo] > around {
				lo++
			}
			for hi < s[1] && along[hi+1]-along[i] <= around {
				hi++
			}
			// At least the points either side, where fixes are further
			// apart than the window.
			a, b := min(lo, max(i-1, s[0])), max(hi, min(i+1, s[1]))
			d, t := along[b]-along[a], (c.Points[b].Elapsed - c.Points[a].Elapsed).Seconds()
			if d > 0 && t > 0 {
				out[i] = d / t
			}
		}
	}
	return out
}

// Elevation is each point's elevation, in metres, and NaN where the file
// recorded none.
func Elevation(c *course.Course) []float64 {
	out := make([]float64, len(c.Points))
	for i, p := range c.Points {
		out[i] = math.NaN()
		if p.HasElevation {
			out[i] = p.Elevation
		}
	}
	return out
}

// Grade is the slope at each point, as a fraction -- 0.061 climbing 6.1%,
// negative descending -- over window metres either side, from fitactivity's
// elevation model: the elevation smoothed first, tuned to the device's own
// ascent and descent where the file has them, as videofx and fitdash tune
// it. A barometer or a GPS altitude wanders by metres, and a slope taken
// from it raw is a saw-tooth of climbs and drops that were never there.
//
// A point with no elevation of its own has no grade -- NaN, not flat -- even
// though the model would interpolate one across it; so does every point of a
// course with too little elevation to model.
func Grade(c *course.Course, window float64) []float64 {
	out := make([]float64, len(c.Points))
	for i := range out {
		out[i] = math.NaN()
	}
	along := c.Along()
	t := &fitactivity.Track{}
	for i, p := range c.Points {
		t.Samples = append(t.Samples, fitactivity.Sample{
			HasDistance: true, Distance: along[i],
			HasElevation: p.HasElevation, Elevation: p.Elevation,
		})
	}
	var opts fitactivity.ElevationOptions
	if c.HasElevationTotals {
		opts.TargetGain, opts.TargetLoss = c.TotalAscent, c.TotalDescent
	}
	m := fitactivity.BuildElevationModel(t, opts)
	if m.Empty() {
		return out
	}
	for i, p := range c.Points {
		if p.HasElevation {
			out[i] = m.GradeAtDistance(along[i], window)
		}
	}
	return out
}

// HeartRate is each point's heart rate, in beats a minute, and NaN where
// the recording had none.
func HeartRate(c *course.Course) []float64 {
	out := make([]float64, len(c.Points))
	for i, p := range c.Points {
		out[i] = math.NaN()
		if p.HasHeartRate {
			out[i] = p.HeartRate
		}
	}
	return out
}

// Power is each point's power from src, in watts, by fitactivity's rule for
// choosing between the two sensors a recording can carry, and NaN where src
// has none. A recorded 0 -- standing at a crossing -- is 0, not absent.
func Power(c *course.Course, src fitactivity.PowerSource) []float64 {
	out := make([]float64, len(c.Points))
	for i, p := range c.Points {
		out[i] = math.NaN()
		if w, ok := p.Power(src); ok {
			out[i] = w
		}
	}
	return out
}

// Around is each point's value averaged over the stretch around metres
// either side of it, within the recorded stretch it is in: a power meter
// reads a new number every second, and a map coloured by each is a line of
// flecks. The average is by time, as a watch's own 10-second power is --
// each value counts for as long as it was held, so a standstill counts for
// how long it lasted, not for the one fix it took. Unknown values are left
// out of it, and a point with none known around it is NaN.
func Around(c *course.Course, values []float64, around float64) []float64 {
	out := make([]float64, len(values))
	for i := range out {
		out[i] = math.NaN()
	}
	along := c.Along()
	for _, s := range Stretches(c) {
		lo, hi := s[0], s[0]
		for i := s[0]; i <= s[1]; i++ {
			for along[i]-along[lo] > around {
				lo++
			}
			hi = max(hi, i)
			for hi < s[1] && along[hi+1]-along[i] <= around {
				hi++
			}
			var sum, weight float64
			for j := lo; j <= hi; j++ {
				if math.IsNaN(values[j]) {
					continue
				}
				w := 1.0
				switch {
				case !c.Timed:
				case j < s[1]:
					w = (c.Points[j+1].Elapsed - c.Points[j].Elapsed).Seconds()
				case j > s[0]: // the stretch's last point, held as long as the one before
					w = (c.Points[j].Elapsed - c.Points[j-1].Elapsed).Seconds()
				}
				sum, weight = sum+w*values[j], weight+w
			}
			out[i] = sum / weight // NaN, 0/0, when nothing around is known
		}
	}
	return out
}

// Spread is the range of the known values worth telling apart: from the
// share tail up from the lowest to the same down from the highest, so a GPS
// spike or a standstill does not push every other value into the middle of
// the scale. ok is false when no value is known.
func Spread(values []float64, tail float64) (lo, hi float64, ok bool) {
	var known []float64
	for _, v := range values {
		if !math.IsNaN(v) {
			known = append(known, v)
		}
	}
	if len(known) == 0 {
		return 0, 0, false
	}
	sort.Float64s(known)
	at := func(q float64) float64 { return known[int(math.Round(q*float64(len(known)-1)))] }
	return at(tail), at(1 - tail), true
}

// Widen is lo to hi made at least span wide about its middle: on a flat
// course the whole ramp over two metres of height would colour GPS noise as
// hills.
func Widen(lo, hi, span float64) (float64, float64) {
	if hi-lo >= span {
		return lo, hi
	}
	mid := (lo + hi) / 2
	return mid - span/2, mid + span/2
}

// gradientEdge is the thin dark edge round a coloured course: enough to hold
// it off a map of any colour, little enough that the map shows beside it.
var gradientEdge = color.RGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}

// Gradient is a line through points coloured by the values at them, on the
// scale s: thinner than a course's own line, with a thin dark edge, so the
// map beside it still reads.
func Gradient(points []render.Coord, values []float64, s render.Scale, scale float64) render.Gradient {
	if scale <= 0 {
		scale = 1
	}
	return render.Gradient{Points: points, Values: values, Scale: s, Width: 3 * scale, Halo: 0.8 * scale, HaloInk: gradientEdge}
}

// Gradients are a course coloured by a value at each of its points: a
// Gradient for each recorded stretch, so none is drawn across a gap.
func Gradients(c *course.Course, values []float64, s render.Scale, scale float64) []render.Gradient {
	var out []render.Gradient
	for _, st := range Stretches(c) {
		if st[1] == st[0] {
			continue
		}
		var pts []render.Coord
		for _, p := range c.Points[st[0] : st[1]+1] {
			pts = append(pts, render.Coord{Lat: p.Lat, Lon: p.Lon})
		}
		out = append(out, Gradient(pts, values[st[0]:st[1]+1], s, scale))
	}
	return out
}
