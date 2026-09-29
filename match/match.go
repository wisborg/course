// Package match finds where an activity followed a reference course, and how
// closely: the parkrun inside a morning's run, the official course of a
// race and where the runner left it.
//
// Both are resampled to a point every few metres and the reference is
// aligned against the activity in order -- subsequence dynamic time warping
// -- so the alignment can start and end anywhere in the activity, and must
// move forward along the reference. The order is what tells the way out of
// an out-and-back course from the way back along the same path, which a
// nearest-point comparison cannot.
package match

import (
	"math"
	"sort"
	"time"

	"github.com/wisborg/course"
)

// Sample is a course resampled: a point every Options.Step metres along it.
type Sample struct {
	Lat, Lon float64
	// Along is metres from the course's start, measured along its line.
	Along float64
	// Elapsed is the time since the course's start, when it has times.
	Elapsed time.Duration
}

// Resample puts a point every step metres along the course's line, each
// between the two fixes either side of it in proportion.
func Resample(c *course.Course, step float64) []Sample {
	if len(c.Points) == 0 {
		return nil
	}
	var out []Sample
	next, along := 0.0, 0.0
	for i := 1; i < len(c.Points); i++ {
		a, b := c.Points[i-1], c.Points[i]
		seg := course.Metres(a.Lat, a.Lon, b.Lat, b.Lon)
		for next <= along+seg && seg > 0 {
			f := (next - along) / seg
			out = append(out, Sample{
				Lat: a.Lat + f*(b.Lat-a.Lat), Lon: a.Lon + f*(b.Lon-a.Lon), Along: next,
				Elapsed: a.Elapsed + time.Duration(f*float64(b.Elapsed-a.Elapsed)),
			})
			next += step
		}
		along += seg
	}
	last := c.Points[len(c.Points)-1]
	if len(out) == 0 || out[len(out)-1].Along < along {
		out = append(out, Sample{Lat: last.Lat, Lon: last.Lon, Along: along, Elapsed: last.Elapsed})
	}
	return out
}

// Options tune the matching. The zero value is the defaults.
type Options struct {
	// Step is the resampling distance, in metres; 0 is 10.
	Step float64
	// Cap is the most a single point's distance counts for in the
	// alignment, in metres; 0 is 150. A detour of a kilometre costs no
	// more per point than one of 150 m, so one detour does not outweigh a
	// course otherwise followed exactly.
	Cap float64
	// Near is how far from the reference a point may be and still be on it,
	// in metres; 0 is 25. Measured on real courses: GPS among a city's
	// buildings wanders up to about 20 m from a course followed exactly, and
	// a detour worth reporting -- a closed bridge, road works -- is 25 m or
	// more.
	Near float64
	// MinCoverage is the share of the reference an activity must pass Near
	// to match; 0 is 0.8.
	MinCoverage float64
}

func (o Options) withDefaults() Options {
	if o.Step <= 0 {
		o.Step = 10
	}
	if o.Cap <= 0 {
		o.Cap = 150
	}
	if o.Near <= 0 {
		o.Near = 25
	}
	if o.MinCoverage <= 0 {
		o.MinCoverage = 0.8
	}
	return o
}

// Reference is a course to match against, by name.
type Reference struct {
	Name   string
	Course *course.Course
}

// Match is a stretch of an activity that followed a reference.
type Match struct {
	Reference string
	// From and To are where in the activity the stretch is, in metres along
	// it, and FromTime and ToTime when, for an activity with times.
	From, To         float64
	FromTime, ToTime time.Duration
	// Coverage is the share of the reference the stretch passed within
	// Near of.
	Coverage float64
	// Mean is the alignment's average distance, point for point in order,
	// in metres, each point's counted up to Options.Cap.
	Mean float64
	// Median and Worst are how far the activity was from the reference,
	// over the reference's points, in metres.
	Median, Worst float64
	// Missed are the stretches of the reference the activity did not pass
	// Near -- a detour, a changed course; Excursions the stretches of the
	// activity that left the reference -- a detour of its own, a stop.
	Missed, Excursions []Stretch
}

// Stretch is part of a course, in metres along it, and how far at most it
// was from the other course.
type Stretch struct {
	From, To, Farthest float64
}

// Find is every stretch of the activity that followed one of the
// references, best first, no two overlapping.
//
// Each reference is aligned against the activity, and the best-aligned
// stretch taken if it covers enough of the reference; that stretch is then
// set aside and the reference aligned again, for a course run twice or two
// parkruns of one course in a morning. References whose bounds miss the
// activity's are not aligned at all.
func Find(activity *course.Course, refs []Reference, o Options) []Match {
	o = o.withDefaults()
	a := Resample(activity, o.Step)
	if len(a) < 2 {
		return nil
	}
	used := make([]bool, len(a))
	var out []Match
	for _, r := range refs {
		rs := Resample(r.Course, o.Step)
		if len(rs) < 2 || !overlaps(a, rs, o.Near) {
			continue
		}
		for len(out) < 50 {
			m, ok := best(a, rs, used, o, activity.Timed, r.Course.Timed)
			if !ok {
				break
			}
			m.Reference = r.Name
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].From < out[j].From })
	return out
}

// best aligns the reference against the activity's unused points and
// returns the best stretch, if it covers enough of the reference, marking
// its points used either way it is found, so a failing search is not
// repeated on the same stretch.
func best(a, r []Sample, used []bool, o Options, aTimed, rTimed bool) (Match, bool) {
	from, to, mean, ok := align(a, r, used, o)
	if !ok {
		return Match{}, false
	}
	m := assess(a[from:to+1], r, o, aTimed, rTimed)
	m.Mean = mean
	for i := from; i <= to; i++ {
		used[i] = true
	}
	// Coverage is measured along the in-order alignment, so a course run
	// backwards, whose every point is near the course but not in order, is
	// not covered: it needs no test of its own.
	if m.Coverage < o.MinCoverage {
		return Match{}, false
	}
	m.From, m.To = a[from].Along, a[to].Along
	m.FromTime, m.ToTime = a[from].Elapsed, a[to].Elapsed
	return m, true
}

// align is subsequence dynamic time warping: the stretch of the activity,
// among its unused points, whose in-order alignment with the whole
// reference costs least. A used point costs Cap wherever it is, so an
// alignment through it is never better than one around it.
func align(a, r []Sample, used []bool, o Options) (from, to int, mean float64, ok bool) {
	m := len(r)
	prev, cur := make([]float64, m), make([]float64, m)
	prevS, curS := make([]int, m), make([]int, m)
	prevL, curL := make([]int, m), make([]int, m)
	cosLat := math.Cos(r[0].Lat * math.Pi / 180)
	dist := func(i, j int) float64 {
		if used[i] {
			return o.Cap
		}
		return math.Min(planar(a[i], r[j], cosLat), o.Cap)
	}
	bestCost := math.Inf(1)
	for i := range a {
		for j := 0; j < m; j++ {
			d := dist(i, j)
			if j == 0 {
				cur[0], curS[0], curL[0] = d, i, 1
				continue
			}
			c, s, l := cur[j-1], curS[j-1], curL[j-1] // the reference moves on, the activity stays
			if i > 0 {
				if prev[j-1] < c {
					c, s, l = prev[j-1], prevS[j-1], prevL[j-1] // both move on
				}
				if prev[j] < c {
					c, s, l = prev[j], prevS[j], prevL[j] // the activity moves on, the reference stays
				}
			}
			cur[j], curS[j], curL[j] = c+d, s, l+1
		}
		if cur[m-1] < bestCost {
			bestCost, from, to, ok = cur[m-1], curS[m-1], i, true
			mean = cur[m-1] / float64(curL[m-1])
		}
		prev, cur = cur, prev
		prevS, curS = curS, prevS
		prevL, curL = curL, prevL
	}
	// A stretch entirely of used points is not a match.
	for i := from; i <= to && ok; i++ {
		if !used[i] {
			return from, to, mean, true
		}
	}
	return 0, 0, 0, false
}

// assess measures how closely a stretch of the activity followed the
// reference, along the in-order alignment of the two: each point of the
// reference against the points of the stretch it is aligned with, and each
// point of the stretch against the reference's.
//
// Along the alignment, not against the nearest point anywhere: on an
// out-and-back course the way back runs beside the way out, and a detour
// off the way out is a few metres from the way back. Measured against the
// nearest point, the detour vanished.
func assess(a, r []Sample, o Options, aTimed, rTimed bool) Match {
	cosLat := math.Cos(r[0].Lat * math.Pi / 180)
	pairs := alignment(a, r, o, cosLat)
	rd := make([]float64, len(r))
	ad := make([]float64, len(a))
	for j := range rd {
		rd[j] = math.Inf(1)
	}
	for i := range ad {
		ad[i] = math.Inf(1)
	}
	for _, p := range pairs {
		d := planar(a[p[0]], r[p[1]], cosLat)
		rd[p[1]] = math.Min(rd[p[1]], d)
		ad[p[0]] = math.Min(ad[p[0]], d)
	}
	near := 0
	for _, d := range rd {
		if d <= o.Near {
			near++
		}
	}
	sorted := append([]float64(nil), rd...)
	sort.Float64s(sorted)
	return Match{
		Coverage:   float64(near) / float64(len(r)),
		Median:     sorted[len(sorted)/2],
		Worst:      sorted[len(sorted)-1],
		Missed:     stretches(r, rd, o.Near, rTimed),
		Excursions: stretches(a, ad, o.Near, aTimed),
	}
}

// alignment is the dynamic-time-warping alignment of a whole stretch with the
// whole reference, as pairs of their indices in order: the one align found,
// rebuilt with the steps kept so it can be walked back.
func alignment(a, r []Sample, o Options, cosLat float64) [][2]int {
	n, m := len(a), len(r)
	// Two rows of cost, rolled, and every step kept: a byte a cell, where a
	// marathon against its course is some eighteen million cells.
	prev, cur := make([]float64, m), make([]float64, m)
	step := make([]byte, n*m) // 0 both moved on, 1 the activity did, 2 the reference did
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			d := math.Min(planar(a[i], r[j], cosLat), o.Cap)
			k := i*m + j
			switch {
			case i == 0 && j == 0:
				cur[j] = d
			case i == 0:
				cur[j], step[k] = cur[j-1]+d, 2
			case j == 0:
				cur[j], step[k] = prev[j]+d, 1
			default:
				c, s := prev[j-1], byte(0)
				if prev[j] < c {
					c, s = prev[j], 1
				}
				if cur[j-1] < c {
					c, s = cur[j-1], 2
				}
				cur[j], step[k] = c+d, s
			}
		}
		prev, cur = cur, prev
	}
	var pairs [][2]int
	for i, j := n-1, m-1; ; {
		pairs = append(pairs, [2]int{i, j})
		if i == 0 && j == 0 {
			break
		}
		switch step[i*m+j] {
		case 0:
			i, j = i-1, j-1
		case 1:
			i--
		default:
			j--
		}
	}
	return pairs
}

// stretches are the runs of samples farther than near that are a detour and
// not a stray fix: lasting at least minDetour where the samples have times,
// and at least minStretch long where they have none.
//
// Time first, because length cannot tell the two apart: one fix 40 m off
// the line is a spike 80 m long, longer than a short detour, and it lasts a
// second or two where a detour or a stop lasts tens.
func stretches(s []Sample, d []float64, near float64, timed bool) []Stretch {
	var out []Stretch
	for i := 0; i < len(s); {
		if d[i] <= near {
			i++
			continue
		}
		j, far := i, 0.0
		for j < len(s) && d[j] > near {
			far = math.Max(far, d[j])
			j++
		}
		long := s[j-1].Along-s[i].Along >= minStretch
		if timed {
			long = s[j-1].Elapsed-s[i].Elapsed >= minDetour
		}
		if long {
			out = append(out, Stretch{From: s[i].Along - s[0].Along, To: s[j-1].Along - s[0].Along, Farthest: far})
		}
		i = j
	}
	return out
}

// minStretch is the shortest detour reported on a course with no times, in
// metres, and minDetour the shortest on one with them.
const (
	minStretch = 20.0
	minDetour  = 10 * time.Second
)

// overlaps reports whether two courses' bounds, grown by near, meet.
func overlaps(a, b []Sample, near float64) bool {
	ab, bb := bounds(a), bounds(b)
	pad := near / 111_000
	return ab[0]-pad <= bb[2] && bb[0]-pad <= ab[2] && ab[1]-pad <= bb[3] && bb[1]-pad <= ab[3]
}

func bounds(s []Sample) [4]float64 {
	b := [4]float64{90, 180, -90, -180}
	for _, p := range s {
		b[0], b[1] = math.Min(b[0], p.Lat), math.Min(b[1], p.Lon)
		b[2], b[3] = math.Max(b[2], p.Lat), math.Max(b[3], p.Lon)
	}
	return b
}

// planar is the distance between two nearby points on a plane tangent to
// the reference, in metres: accurate to well under a metre across a city.
func planar(p, q Sample, cosLat float64) float64 {
	const m = 111_320.0
	return math.Hypot((p.Lat-q.Lat)*m, (p.Lon-q.Lon)*m*cosLat)
}
