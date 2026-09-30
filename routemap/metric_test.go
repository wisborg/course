package routemap

import (
	"math"
	"testing"
	"time"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// eastward is a course along 10°N from 20°E, a point every 10 m, taking
// step(i) to reach point i from the one before.
func eastward(n int, step func(i int) time.Duration) *course.Course {
	c := &course.Course{Timed: true}
	var t time.Duration
	for i := 0; i < n; i++ {
		if i > 0 {
			t += step(i)
		}
		c.Points = append(c.Points, course.Point{Lat: 10, Lon: 20 + float64(i)*10/109_600, Elapsed: t})
	}
	return c
}

func every(d time.Duration) func(int) time.Duration { return func(int) time.Duration { return d } }

// sameValue is a within 1% of b -- the courses here are laid out with an
// approximate metre of longitude -- or both not known.
func sameValue(a, b float64) bool {
	return a == b || math.IsNaN(a) && math.IsNaN(b) || math.Abs(a-b) < 0.01*math.Max(1, math.Abs(b))
}

// A course is split where its recording has a gap -- a long pause and a
// jump of more than a few hundred metres -- and nowhere else.
func TestStretches(t *testing.T) {
	c := eastward(100, every(2*time.Second))
	if got := Stretches(c); len(got) != 1 || got[0] != [2]int{0, 99} {
		t.Errorf("an unbroken course is %v", got)
	}
	// A tunnel: ten minutes and a kilometre between points 49 and 50.
	for i := 50; i < len(c.Points); i++ {
		c.Points[i].Lon += 1000.0 / 109_600
		c.Points[i].Elapsed += 10 * time.Minute
	}
	if got := Stretches(c); len(got) != 2 || got[0] != [2]int{0, 49} || got[1] != [2]int{50, 99} {
		t.Errorf("a course with a gap is %v", got)
	}
	if got := Stretches(&course.Course{}); got != nil {
		t.Errorf("an empty course is %v", got)
	}
}

// Speed is distance over time around each point: steady where the course
// was steady, the faster half faster, never across a gap, and not known on a
// course with no clock.
func TestSpeed(t *testing.T) {
	c := eastward(100, func(i int) time.Duration {
		if i > 50 {
			return time.Second // 10 m/s
		}
		return 2 * time.Second // 5 m/s
	})
	s := Speed(c, 30)
	if !sameValue(s[10], 5) || !sameValue(s[90], 10) || !sameValue(s[0], 5) || !sameValue(s[99], 10) {
		t.Errorf("speeds %v %v %v %v, want 5 at the start and 10 at the end", s[0], s[10], s[90], s[99])
	}
	if s[50] <= 5 || s[50] >= 10 {
		t.Errorf("where the pace changes, speed %v, want between", s[50])
	}

	// Across a gap nothing is averaged: the points either side of it are
	// as fast as their own stretch.
	g := eastward(100, every(2*time.Second))
	for i := 50; i < len(g.Points); i++ {
		g.Points[i].Lon += 1000.0 / 109_600
		g.Points[i].Elapsed += 10 * time.Minute
	}
	gs := Speed(g, 30)
	if !sameValue(gs[49], 5) || !sameValue(gs[50], 5) {
		t.Errorf("either side of a gap, speeds %v and %v, want 5", gs[49], gs[50])
	}

	// Fixes further apart than the window still have a speed, from their
	// neighbours.
	sparse := eastward(10, every(20*time.Second))
	for i := range sparse.Points {
		sparse.Points[i].Lon = 20 + float64(i)*100/109_600
	}
	if v := Speed(sparse, 30)[5]; !sameValue(v, 5) {
		t.Errorf("fixes 100 m apart, speed %v, want 5", v)
	}

	still := eastward(5, every(0))
	untimed := eastward(5, every(time.Second))
	untimed.Timed = false
	for name, c := range map[string]*course.Course{"a clock that stood still": still, "no clock": untimed} {
		for i, v := range Speed(c, 30) {
			if !math.IsNaN(v) {
				t.Errorf("with %s, speed at %d is %v, want not known", name, i, v)
			}
		}
	}
}

func TestElevation(t *testing.T) {
	c := &course.Course{Points: []course.Point{{HasElevation: true, Elevation: 12}, {}, {HasElevation: true, Elevation: 0}}}
	got := Elevation(c)
	for i, want := range []float64{12, math.NaN(), 0} {
		if !sameValue(got[i], want) {
			t.Errorf("elevation %d is %v, want %v: no elevation is not zero", i, got[i], want)
		}
	}
}

// The spread leaves out a share at each end, ignores unknown values, and
// says when there are none.
func TestSpread(t *testing.T) {
	var v []float64
	for i := 0; i <= 100; i++ {
		v = append(v, float64(i), math.NaN())
	}
	if lo, hi, ok := Spread(v, 0.05); !ok || lo != 5 || hi != 95 {
		t.Errorf("spread %v-%v %v, want 5-95", lo, hi, ok)
	}
	if lo, hi, ok := Spread(v, 0); !ok || lo != 0 || hi != 100 {
		t.Errorf("whole spread %v-%v %v, want 0-100", lo, hi, ok)
	}
	if _, _, ok := Spread([]float64{math.NaN()}, 0.05); ok {
		t.Error("a spread of nothing known")
	}
	if lo, hi := Widen(9, 11, 20); lo != 0 || hi != 20 {
		t.Errorf("widened to %v-%v, want 0-20", lo, hi)
	}
	if lo, hi := Widen(0, 50, 20); lo != 0 || hi != 50 {
		t.Errorf("a wide range widened to %v-%v", lo, hi)
	}
}

// A coloured course is a gradient a recorded stretch, each with its own
// values, and none across a gap.
func TestGradients(t *testing.T) {
	c := eastward(100, every(2*time.Second))
	for i := 50; i < len(c.Points); i++ {
		c.Points[i].Lon += 1000.0 / 109_600
		c.Points[i].Elapsed += 10 * time.Minute
	}
	values := make([]float64, 100)
	for i := range values {
		values[i] = float64(i)
	}
	// And a last fix alone after a second gap, which is no line at all.
	c.Points[99].Lon += 1000.0 / 109_600
	c.Points[99].Elapsed += 10 * time.Minute
	gs := Gradients(c, values, render.Scale{Min: 0, Max: 99}, 2)
	if len(gs) != 2 {
		t.Fatalf("%d gradients, want one each side of the gap", len(gs))
	}
	if g := gs[1]; len(g.Points) != 49 || len(g.Values) != 49 || g.Values[0] != 50 || g.Points[0].Lon != c.Points[50].Lon || g.Width != 6 || g.Halo <= 0 {
		t.Errorf("the second gradient is %d points, values from %v, width %v", len(g.Points), g.Values[0], g.Width)
	}
}
