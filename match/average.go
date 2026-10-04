package match

import (
	"errors"
	"slices"
	"time"

	"github.com/wisborg/course"
)

// Averaged is the result of Average: the averaged course, and how each run
// followed it.
type Averaged struct {
	Course *course.Course
	// Runs are how each run followed the average, in the order given; a
	// run that did not follow the first has no Match and was left out.
	Runs []AveragedRun
}

// AveragedRun is one run of an average: whether it was in it, and how
// closely it followed the averaged line.
type AveragedRun struct {
	Used  bool
	Match Match
}

// averagePasses is how many times the runs are aligned: against the first
// run, then against the average of that alignment, so the average is where
// the runs were and not where the first one happened to be.
const averagePasses = 2

// Average is the line many runs of one course share, point for point: the
// first run is the course -- cropped already, if it carried a warm-up --
// and every other run is aligned to it as matching aligns an activity to a
// reference, so its own warm-up and cool-down are left out by the
// alignment, not by hand. At every Options.Step metres of the course the
// average is the median of where the runs were, latitude and longitude
// each, and of how long each had taken from the course's start, if any had
// times.
//
// The median rather than the mean, so a run that took a detour the others
// did not -- a closed bridge, a wrong turn -- leaves the line where the
// rest of them ran, rather than bending it a fraction of the way towards
// the detour. It needs most runs to agree: of two, the median is their mean.
//
// The alignment is done twice, the second time against the first average,
// so the result does not lean on the first run's own GPS. A run that does
// not follow the first at all is left out and reported, rather than
// averaged in somewhere it never was; fewer than two runs that follow it is
// an error, since one run is no average.
func Average(runs []*course.Course, o Options) (Averaged, error) {
	if len(runs) < 2 {
		return Averaged{}, errors.New("an average takes two runs or more")
	}
	o = o.WithDefaults()
	base := runs[0]
	if len(base.Points) < 2 {
		return Averaged{}, errors.New("the first run has no line to average along")
	}
	var out Averaged
	ref := base
	for pass := 0; pass < averagePasses; pass++ {
		r := Resample(ref, o.Step)
		out.Runs = make([]AveragedRun, len(runs))
		var used []Match
		var timed []bool
		for i, run := range runs {
			ms := Find(run, []Reference{{Name: "average", Course: ref}}, o)
			if len(ms) == 0 || len(ms[0].Arrivals) != len(r) {
				continue
			}
			out.Runs[i] = AveragedRun{Used: true, Match: ms[0]}
			used = append(used, ms[0])
			timed = append(timed, run.Timed)
		}
		if len(used) < 2 {
			return Averaged{}, errors.New("fewer than two of the runs follow the first; there is nothing to average")
		}
		avg := &course.Course{Sources: base.Sources, Sport: base.Sport, Start: base.Start}
		lats := make([]float64, 0, len(used))
		lons := make([]float64, 0, len(used))
		times := make([]float64, 0, len(used))
		for j := range r {
			lats, lons, times = lats[:0], lons[:0], times[:0]
			for k, m := range used {
				lats = append(lats, m.Arrivals[j].Lat)
				lons = append(lons, m.Arrivals[j].Lon)
				if timed[k] {
					times = append(times, float64(m.Arrivals[j].Elapsed-m.Arrivals[0].Elapsed))
				}
			}
			p := course.Point{Lat: median(lats), Lon: median(lons)}
			if len(times) > 0 {
				p.Elapsed = time.Duration(median(times))
			}
			avg.Points = append(avg.Points, p)
		}
		avg.Timed = slices.Contains(timed, true)
		out.Course = avg
		ref = avg
	}
	return out, nil
}

// median is the middle of vs, or the mean of the middle two; vs is sorted
// in place.
func median(vs []float64) float64 {
	slices.Sort(vs)
	n := len(vs)
	if n%2 == 1 {
		return vs[n/2]
	}
	return (vs[n/2-1] + vs[n/2]) / 2
}
