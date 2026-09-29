// Package compare compares two runs of one course: where along it one was
// faster than the other, and by how much, point by point -- aligned by
// where on the course each was, so running wide of a corner, a detour or a
// stop does not put the two out of step.
package compare

import (
	"errors"
	"math"
	"time"

	"github.com/wisborg/course"
	"github.com/wisborg/course/match"
)

// Profile is two runs of a course side by side: at each point of the
// reference, a step apart, when each got there.
type Profile struct {
	// Step is the distance between points, in metres, and From how far
	// along the reference the first of them is: its start, unless the run
	// joined it later.
	Step, From float64
	// Run and Ref are when each got to each point. Ref is the reference's
	// own times: it is a run of the course too.
	Run, Ref []time.Duration
	// Where is where the run was at each point, to draw it.
	Where []match.Arrival
	// Match is the stretch of the run that followed the reference. Its
	// Stops are the run's.
	Match match.Match
	// RefStops are where the reference run stood still: over a split with
	// one in it the run looks fast, and this is why.
	RefStops []match.Stop
}

// ErrUntimed is a comparison with a course that has no times: a plan, an
// official course file. There is nothing to be faster or slower than.
var ErrUntimed = errors.New("the reference has no times to compare against; compare with a run of the course")

// Against aligns a run with a reference run of the same course -- the
// stretch of the run that followed it best, if any -- and puts their times
// side by side.
func Against(run, ref *course.Course, o match.Options) (*Profile, error) {
	if !ref.Timed {
		return nil, ErrUntimed
	}
	if !run.Timed {
		return nil, errors.New("the run has no times to compare")
	}
	ms := match.Find(run, []match.Reference{{Name: "reference", Course: ref}}, o)
	if len(ms) == 0 {
		return nil, errors.New("the run does not follow the reference")
	}
	best := ms[0]
	for _, m := range ms[1:] {
		if m.Coverage > best.Coverage {
			best = m
		}
	}
	o = o.WithDefaults()
	step := o.Step
	rs := match.Resample(ref, step)
	n := min(len(rs), len(best.Arrivals))
	// The alignment lines up the whole reference, so where the run
	// covered only part of it -- a watch started late, stopped early --
	// the rest is aligned with wherever the run began or ended, and would
	// be compared as run in no time. Compared is only from the first point
	// of the reference the run was on to the last.
	on := func(j int) bool {
		a := best.Arrivals[j]
		return course.Metres(a.Lat, a.Lon, rs[j].Lat, rs[j].Lon) <= o.Near
	}
	lo, hi := 0, n-1
	for lo < hi && !on(lo) {
		lo++
	}
	for hi > lo && !on(hi) {
		hi--
	}
	p := &Profile{Step: step, From: float64(lo) * step, Match: best, Where: best.Arrivals[lo : hi+1]}
	for _, st := range match.Stops(ref, step) {
		if st.At >= p.From && st.At <= float64(hi)*step { // on the stretch compared
			p.RefStops = append(p.RefStops, st)
		}
	}
	for j := lo; j <= hi; j++ {
		p.Run = append(p.Run, best.Arrivals[j].Elapsed-best.Arrivals[lo].Elapsed)
		p.Ref = append(p.Ref, rs[j].Elapsed-rs[lo].Elapsed)
	}
	return p, nil
}

// Gap is how far the run was behind the reference at point j: positive
// behind, negative ahead.
func (p *Profile) Gap(j int) time.Duration { return p.Run[j] - p.Ref[j] }

// Faster is the run's pace against the reference's at each point, as a
// natural log of their speeds: 0 the same, log(1.1) a tenth faster,
// negative slower. A log so that a tenth faster and a tenth slower are the
// same distance from level either way.
//
// Each point is compared over the stretch around metres either side of it
// -- smooth enough that a GPS fix a few metres out does not colour a point
// on its own, fine enough that a hill or a stop stands out. Where either
// run took no time over the stretch, which only a broken recording does,
// the point is NaN: not known, rather than level.
func (p *Profile) Faster(around float64) []float64 {
	k := max(int(math.Round(around/p.Step)), 1)
	out := make([]float64, len(p.Run))
	last := len(p.Run) - 1
	for j := range out {
		lo, hi := max(j-k, 0), min(j+k, last)
		run, ref := p.Run[hi]-p.Run[lo], p.Ref[hi]-p.Ref[lo]
		out[j] = math.NaN()
		if run > 0 && ref > 0 {
			out[j] = math.Log(float64(ref) / float64(run))
		}
	}
	return out
}

// Split is a stretch of the reference and how long each took over it.
type Split struct {
	// From and To are where it starts and ends, in metres along the
	// reference.
	From, To float64
	// Run and Ref are how long each took over it.
	Run, Ref time.Duration
	// Gap is how far the run was behind the reference at its end: positive
	// behind, negative ahead.
	Gap time.Duration
}

// Splits are the compared stretch in pieces every metres long along the
// reference, from where the comparison starts, the last taking what is left. Measured along the reference, not
// along the run: a split is the same piece of ground for both, however far
// either ran over it.
func (p *Profile) Splits(every float64) []Split {
	last := len(p.Run) - 1
	k := max(int(math.Round(every/p.Step)), 1)
	var out []Split
	for from := 0; from < last; from += k {
		to := min(from+k, last)
		out = append(out, Split{
			From: p.From + float64(from)*p.Step, To: p.From + float64(to)*p.Step,
			Run: p.Run[to] - p.Run[from], Ref: p.Ref[to] - p.Ref[from],
			Gap: p.Gap(to),
		})
	}
	return out
}
