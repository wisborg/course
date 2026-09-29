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
// reference, a step apart from its start, when each got there.
type Profile struct {
	// Step is the distance between points, in metres.
	Step float64
	// Run and Ref are when each got to each point. Ref is the reference's
	// own times: it is a run of the course too.
	Run, Ref []time.Duration
	// Where is where the run was at each point, to draw it.
	Where []match.Arrival
	// Match is the stretch of the run that followed the reference.
	Match match.Match
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
	step := o.Step
	if step <= 0 {
		step = 10
	}
	rs := match.Resample(ref, step)
	n := min(len(rs), len(best.Arrivals))
	p := &Profile{Step: step, Match: best, Where: best.Arrivals[:n]}
	for j := 0; j < n; j++ {
		p.Run = append(p.Run, best.Arrivals[j].Elapsed-best.Arrivals[0].Elapsed)
		p.Ref = append(p.Ref, rs[j].Elapsed-rs[0].Elapsed)
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
