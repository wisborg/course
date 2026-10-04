package match

import (
	"math"
	"testing"
	"time"

	"github.com/wisborg/course"
)

// shifted is c moved north by metres and slowed by a factor: every fix
// that much later from its own start.
func shifted(c *course.Course, metres, slower float64) *course.Course {
	out := &course.Course{Timed: c.Timed}
	for _, p := range c.Points {
		p.Lat += metres * north
		p.Elapsed = time.Duration(float64(p.Elapsed) * slower)
		out.Points = append(out.Points, p)
	}
	return out
}

// northOf is how far north of the out-and-back's out leg a point is, in
// metres.
func northOf(p course.Point) float64 { return (p.Lat - 10) / north }

// Three runs of one road, 4 m north, on it, and 4 m south, average to the
// road; their times to the middle one's; a warm-up before one of them is
// left out by the alignment; and each run is reported as following the
// average closely.
func TestAverageIsWhereTheRunsWere(t *testing.T) {
	road := path(0, outAndBack...)
	warm := join(path(0, [2]float64{0, -300}, [2]float64{0, -1}), shifted(road, -4, 1.1))
	avg, err := Average([]*course.Course{shifted(road, 4, 0.9), road, warm}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := avg.Course
	if !c.Timed || len(c.Points) < 200 {
		t.Fatalf("an average of %d points, timed %v", len(c.Points), c.Timed)
	}
	for _, i := range []int{10, 50, 90} { // along the out leg, 100..900 m
		if d := northOf(c.Points[i]); math.Abs(d) > 1 {
			t.Errorf("point %d is %.1f m north of the road, want on it", i, d)
		}
	}
	// 500 m out at 2 m a second is 250 s for the middle run: the median of
	// 225, 250 and 275.
	if got := c.Points[50].Elapsed - c.Points[0].Elapsed; math.Abs(got.Seconds()-250) > 3 {
		t.Errorf("500 m in %v, want the median run's 250 s", got)
	}
	for i, r := range avg.Runs {
		if !r.Used || r.Match.Coverage < 0.99 || r.Match.Median > 6 {
			t.Errorf("run %d: used %v, %.0f%% of it, median %.1f m off", i+1, r.Used, 100*r.Match.Coverage, r.Match.Median)
		}
	}
}

// A detour one run of three took does not bend the line: the median is
// where the other two were.
func TestAnAverageIgnoresOneRunsDetour(t *testing.T) {
	road := path(0, outAndBack...)
	detour := path(0, [2]float64{0, 0}, [2]float64{400, 0}, [2]float64{400, -100}, [2]float64{600, -100}, [2]float64{600, 0}, [2]float64{1000, 0}, [2]float64{1000, 8}, [2]float64{0, 8})
	avg, err := Average([]*course.Course{road, shifted(road, 2, 1), detour}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	worst := 0.0
	for _, p := range avg.Course.Points[:100] { // the out leg
		worst = math.Max(worst, math.Abs(northOf(p)))
	}
	if worst > 3 {
		t.Errorf("the average strays %.0f m from the road; want it where two runs of three were", worst)
	}
}

// A run elsewhere is left out and said so; with fewer than two runs that
// follow the first there is no average; one run is no average either; and
// runs without times make an average without them.
func TestAverageRefusals(t *testing.T) {
	road := path(0, outAndBack...)
	elsewhere := shifted(road, 500, 1)
	avg, err := Average([]*course.Course{road, shifted(road, 3, 1), elsewhere}, Options{})
	if err != nil || avg.Runs[2].Used || !avg.Runs[1].Used {
		t.Errorf("a run 500 m away: %v, used %v %v %v", err, avg.Runs[0].Used, avg.Runs[1].Used, avg.Runs[2].Used)
	}
	if _, err := Average([]*course.Course{road, elsewhere}, Options{}); err == nil {
		t.Error("an average of one run and one elsewhere")
	}
	if _, err := Average([]*course.Course{road}, Options{}); err == nil {
		t.Error("an average of one run")
	}
	untimed := func(c *course.Course) *course.Course {
		c = shifted(c, 0, 1)
		c.Timed = false
		for i := range c.Points {
			c.Points[i].Elapsed = 0
		}
		return c
	}
	avg, err = Average([]*course.Course{untimed(road), untimed(shifted(road, 2, 1))}, Options{})
	if err != nil || avg.Course.Timed {
		t.Errorf("untimed runs: %v, timed %v", err, avg.Course != nil && avg.Course.Timed)
	}
	if math.Abs(northOf(avg.Course.Points[50])-1) > 0.5 {
		t.Errorf("of two runs, the average is %.1f m north, want their mean, 1", northOf(avg.Course.Points[50]))
	}
}

// Times are from each run's own arrival at the course's start, so a run
// with a warm-up before it is not two and a half minutes slower: of two
// runs the median is their mean, 262.5 s to 500 m, whatever came first.
func TestAverageTimesAreFromTheCoursesStart(t *testing.T) {
	road := path(0, outAndBack...)
	warm := join(path(0, [2]float64{0, -300}, [2]float64{0, -1}), shifted(road, 0, 1.1))
	avg, err := Average([]*course.Course{road, warm}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := avg.Course.Points[50].Elapsed - avg.Course.Points[0].Elapsed; math.Abs(got.Seconds()-262.5) > 3 {
		t.Errorf("500 m in %v, want 262.5 s", got)
	}
	if got := avg.Course.Points[0].Elapsed; got != 0 {
		t.Errorf("the average starts %v in, want at 0", got)
	}
}

// A run without times is in the line and not in the times: of a run at
// 250 s to 500 m, one at 275 s, and one with no times, the average takes
// 262.5 s, not a median dragged towards the third run's zeros.
func TestAnUntimedRunIsNotInTheTimes(t *testing.T) {
	road := path(0, outAndBack...)
	untimed := shifted(road, 1, 1)
	untimed.Timed = false
	for i := range untimed.Points {
		untimed.Points[i].Elapsed = 0
	}
	avg, err := Average([]*course.Course{road, shifted(road, -1, 1.1), untimed}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := avg.Course.Points[50].Elapsed - avg.Course.Points[0].Elapsed; !avg.Course.Timed || math.Abs(got.Seconds()-262.5) > 3 {
		t.Errorf("500 m in %v, timed %v; want 262.5 s from the two timed runs", got, avg.Course.Timed)
	}
}
