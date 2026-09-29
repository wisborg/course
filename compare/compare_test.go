package compare

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/wisborg/course"
	"github.com/wisborg/course/match"
)

// Every course here is invented, along 10°N east from 20°E, where a metre
// east is about 1/109,600 of a degree.
const east = 1 / 109_600.0

// straight is a course from x0 to x1 metres east, a fix every 2 m, taking
// pace(x) seconds for each 2 m, starting at start.
func straight(x0, x1 float64, start time.Duration, pace func(x float64) time.Duration) *course.Course {
	c := &course.Course{Timed: true}
	t := start
	for x := x0; x <= x1; x += 2 {
		c.Points = append(c.Points, course.Point{Lat: 10, Lon: 20 + x*east, Elapsed: t})
		t += pace(x)
	}
	return c
}

func steady(d time.Duration) func(float64) time.Duration {
	return func(float64) time.Duration { return d }
}

// A run slower over the second half of a course than the reference is level
// over the first half and slower over the second, by the ratio of their
// paces, and behind at the end by the time lost.
func TestAgainstPutsTheSlowerHalfWhereItWas(t *testing.T) {
	ref := straight(0, 1000, 0, steady(time.Second))
	run := straight(0, 1000, 0, func(x float64) time.Duration {
		if x >= 500 {
			return 1250 * time.Millisecond
		}
		return time.Second
	})
	p, err := Against(run, ref, match.Options{})
	if err != nil {
		t.Fatal(err)
	}
	faster := p.Faster(30)
	n := len(faster)
	if n < 95 {
		t.Fatalf("%d points compared along a kilometre at 10 m", n)
	}
	if v := faster[n/4]; math.Abs(v) > 0.02 {
		t.Errorf("a quarter of the way, the run is %v against the reference, want level", v)
	}
	if v, want := faster[3*n/4], math.Log(1/1.25); math.Abs(v-want) > 0.02 {
		t.Errorf("three quarters of the way, the run is %v against the reference, want %v", v, want)
	}
	// 250 steps of 2 m, a quarter of a second slower each.
	if gap := p.Gap(n - 1); gap < 60*time.Second || gap > 65*time.Second {
		t.Errorf("the run is %v behind at the end, want about 62.5s", gap)
	}
}

// The comparison is of the stretch that followed the reference: a warm-up
// before it does not count, and the times start where the course did.
func TestAgainstLeavesOutTheWarmUp(t *testing.T) {
	ref := straight(0, 1000, 3*time.Minute, steady(time.Second))
	warmUp := straight(-600, -100, 0, steady(time.Second))
	onCourse := straight(0, 1000, 10*time.Minute, steady(time.Second))
	run := &course.Course{Timed: true, Points: append(warmUp.Points, onCourse.Points...)}
	p, err := Against(run, ref, match.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Run[0] != 0 || p.Ref[0] != 0 {
		t.Errorf("the times start at %v and %v, want both at 0", p.Run[0], p.Ref[0])
	}
	if gap := p.Gap(len(p.Run) - 1); gap.Abs() > 5*time.Second {
		t.Errorf("an even run of the course is %v behind at the end, want level", gap)
	}
	if lon := p.Where[0].Lon; math.Abs(lon-20) > 15*east {
		t.Errorf("the comparison starts %v m east of the course's start", (lon-20)/east)
	}
}

func TestAgainstRefusesWhatItCannotCompare(t *testing.T) {
	ref := straight(0, 1000, 0, steady(time.Second))
	untimed := &course.Course{Points: ref.Points}
	elsewhere := straight(5000, 6000, 0, steady(time.Second))
	if _, err := Against(ref, untimed, match.Options{}); !errors.Is(err, ErrUntimed) {
		t.Errorf("against an untimed reference: %v, want ErrUntimed", err)
	}
	if _, err := Against(untimed, ref, match.Options{}); err == nil {
		t.Error("an untimed run was compared")
	}
	if _, err := Against(elsewhere, ref, match.Options{}); err == nil {
		t.Error("a run elsewhere was compared")
	}
}

// A stretch over which either took no time, which only a broken recording
// does, is not known rather than level or infinitely fast; and the window is
// cut at the ends of the course rather than running off them.
func TestFasterWhereTheTimesDoNotMove(t *testing.T) {
	s := time.Second
	p := &Profile{
		Step: 10,
		Run:  []time.Duration{0, 2 * s, 4 * s, 4 * s, 4 * s, 4 * s, 6 * s},
		Ref:  []time.Duration{0, 1 * s, 2 * s, 3 * s, 4 * s, 5 * s, 6 * s},
	}
	got := p.Faster(10)
	for j, want := range []float64{math.Log(0.5), math.Log(0.5), 0, math.NaN(), math.NaN(), 0, math.Log(0.5)} {
		if !(got[j] == want || math.IsNaN(got[j]) && math.IsNaN(want) || math.Abs(got[j]-want) < 1e-9) {
			t.Errorf("point %d is %v, want %v", j, got[j], want)
		}
	}
	// Twice as far either side, point 1 is compared over points 0 to 3.
	if got, want := p.Faster(20)[1], math.Log(0.75); math.Abs(got-want) > 1e-9 {
		t.Errorf("over 20 m either side, point 1 is %v, want %v", got, want)
	}
}

// A run that went round the course twice is compared over the time it
// followed it better: here the second, the first having a detour in it.
func TestAgainstTakesTheBetterOfTwoRuns(t *testing.T) {
	ref := straight(0, 1000, 0, steady(time.Second))
	first := straight(0, 1000, 0, steady(time.Second))
	for i := range first.Points {
		if x := float64(2 * i); x > 400 && x < 520 {
			first.Points[i].Lat += 100 / 110_600.0
		}
	}
	back := straight(0, 1000, 0, steady(time.Second))
	for i := range back.Points {
		back.Points[i].Lon = 40 - back.Points[i].Lon // 1000 m back west, to the start
		back.Points[i].Lat += 30 / 110_600.0
		back.Points[i].Elapsed += 10 * time.Minute
	}
	second := straight(0, 1000, 20*time.Minute, steady(time.Second))
	run := &course.Course{Timed: true}
	for _, c := range []*course.Course{first, back, second} {
		run.Points = append(run.Points, c.Points...)
	}
	p, err := Against(run, ref, match.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Match.FromTime < 15*time.Minute {
		t.Errorf("compared from %v into the run, want the second time round, from 20m", p.Match.FromTime)
	}
}

// Splits are every so many metres of the reference, the last what is left,
// each with both runs' times over it and the gap at its end.
func TestSplits(t *testing.T) {
	s := time.Second
	p := &Profile{
		Step: 10,
		Run:  []time.Duration{0, 10 * s, 20 * s, 35 * s, 50 * s, 60 * s},
		Ref:  []time.Duration{0, 10 * s, 20 * s, 30 * s, 40 * s, 50 * s},
	}
	got := p.Splits(20)
	want := []Split{
		{From: 0, To: 20, Run: 20 * s, Ref: 20 * s, Gap: 0},
		{From: 20, To: 40, Run: 30 * s, Ref: 20 * s, Gap: 10 * s},
		{From: 40, To: 50, Run: 10 * s, Ref: 10 * s, Gap: 10 * s},
	}
	if len(got) != len(want) {
		t.Fatalf("splits = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("split %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := (&Profile{Step: 10, Run: []time.Duration{0}, Ref: []time.Duration{0}}).Splits(20); len(got) != 0 {
		t.Errorf("splits of one point = %+v, want none", got)
	}
}
