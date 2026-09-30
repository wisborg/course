package routemap

import (
	"math"
	"testing"
	"time"

	"github.com/wisborg/fitactivity"
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

// hill is a course east along 10°N, a point every 10 m: up 5% for 500 m,
// then down 5%, with wander(i) metres of altimeter error at each point.
func hill(wander func(i int) float64) *course.Course {
	c := eastward(101, every(3*time.Second))
	for i := range c.Points {
		x := float64(10 * i)
		h := 0.05 * x
		if x > 500 {
			h = 0.05 * (1000 - x)
		}
		c.Points[i].HasElevation, c.Points[i].Elevation = true, 100+h+wander(i)
	}
	return c
}

// The grade is the slope, climbing positive and descending negative,
// through altimeter noise that taken raw would read as ±20% from one point
// to the next.
func TestGrade(t *testing.T) {
	c := hill(func(i int) float64 { return float64(i%2)*2 - 1 })
	g := Grade(c, 30)
	if math.Abs(g[25]-0.05) > 0.01 || math.Abs(g[75]+0.05) > 0.01 {
		t.Errorf("grade a quarter of the way %v and three quarters %v, want +0.05 and -0.05", g[25], g[75])
	}

	// Over 200 m either side, the grade 400 m up the climb takes in the
	// top of the hill and the start of the descent, and reads gentler.
	if wide := Grade(c, 200); wide[40] > g[40]-0.01 {
		t.Errorf("400 m along, %v over 200 m either side and %v over 30; want the wider window gentler", wide[40], g[40])
	}

	c.Points[40].HasElevation = false
	if g := Grade(c, 30); !math.IsNaN(g[40]) || math.IsNaN(g[41]) {
		t.Errorf("grade %v at a point with no elevation and %v beside it; want not known, then known", g[40], g[41])
	}
	for i := range c.Points {
		c.Points[i].HasElevation = false
	}
	for i, v := range Grade(c, 30) {
		if !math.IsNaN(v) {
			t.Fatalf("with no elevation at all, grade %v at %d, want not known", v, i)
		}
	}
	// One point with an elevation is too little to take a slope from: not
	// known there either, rather than level.
	c.Points[10].HasElevation = true
	if v := Grade(c, 30)[10]; !math.IsNaN(v) {
		t.Errorf("with one elevation in the course, grade %v there, want not known", v)
	}
}

// The smoothing is tuned to the device's own ascent and descent where the
// file has them: a flat course whose altimeter drifts in long swells reads
// as rolling ground on the default smoothing, and as the flat ground the
// device's totals say it was when they are given.
func TestGradeIsSmoothedToTheDevicesTotals(t *testing.T) {
	c := eastward(201, every(3*time.Second))
	for i := range c.Points {
		c.Points[i].HasElevation, c.Points[i].Elevation = true, 50+3*math.Sin(2*math.Pi*float64(i)/40)
	}
	steepest := func(g []float64) float64 {
		m := 0.0
		for _, v := range g {
			m = math.Max(m, math.Abs(v))
		}
		return m
	}
	if m := steepest(Grade(c, 30)); m < 0.02 {
		t.Fatalf("the swells read %v at their steepest untuned; the test needs them visible", m)
	}
	c.HasElevationTotals, c.TotalAscent, c.TotalDescent = true, 1, 1
	if m := steepest(Grade(c, 30)); m > 0.01 {
		t.Errorf("tuned to a device that climbed 1 m, the steepest grade is %v, want under 1%%", m)
	}
}

// Heart rate and power are the recording's, NaN where it had none; power
// by the source asked for; and a recorded 0 W is 0, not missing.
func TestHeartRateAndPower(t *testing.T) {
	c := &course.Course{Timed: true, Points: []course.Point{
		{HasHeartRate: true, HeartRate: 140, HasNativePower: true, NativePower: 300, HasStrydPower: true, StrydPower: 240},
		{HasNativePower: true, NativePower: 0},
		{},
	}}
	hr := HeartRate(c)
	for i, want := range []float64{140, math.NaN(), math.NaN()} {
		if !sameValue(hr[i], want) {
			t.Errorf("heart rate %d is %v, want %v", i, hr[i], want)
		}
	}
	for src, want := range map[fitactivity.PowerSource][]float64{
		fitactivity.PowerAuto:   {240, 0, math.NaN()},
		fitactivity.PowerNative: {300, 0, math.NaN()},
		fitactivity.PowerStryd:  {240, math.NaN(), math.NaN()},
	} {
		got := Power(c, src)
		for i := range want {
			if !sameValue(got[i], want[i]) {
				t.Errorf("%v power %d is %v, want %v", src, i, got[i], want[i])
			}
		}
	}
}

// A value averaged around a point counts each reading for as long as it was
// held, leaves out unknown ones, and does not reach across a gap.
func TestAround(t *testing.T) {
	// 10 m and a second a point, but 9 seconds held at point 5.
	c := eastward(11, func(i int) time.Duration {
		if i == 6 {
			return 9 * time.Second
		}
		return time.Second
	})
	v := make([]float64, 11)
	for i := range v {
		v[i] = 100
	}
	v[5] = 200
	v[7] = math.NaN()
	got := Around(c, v, 15) // points 4-6 around 5
	// 100 for 1 s, 200 for 9 s, 100 for 1 s.
	if want := (100 + 9*200 + 100) / 11.0; !sameValue(got[5], want) {
		t.Errorf("around point 5, %v, want %v: the reading held 9 s counts 9 times", got[5], want)
	}
	if !sameValue(got[8], 100) {
		t.Errorf("around point 8, %v, want 100 with the unknown one left out", got[8])
	}
	all := make([]float64, 11)
	for i := range all {
		all[i] = math.NaN()
	}
	if v := Around(c, all, 15)[5]; !math.IsNaN(v) {
		t.Errorf("with nothing known, %v, want not known", v)
	}

	// Across a gap, the other side's values are not taken in.
	g := eastward(20, every(time.Second))
	for i := 10; i < len(g.Points); i++ {
		g.Points[i].Lon += 1000.0 / 109_600
		g.Points[i].Elapsed += 10 * time.Minute
	}
	w := make([]float64, 20)
	for i := range w {
		w[i] = 100
		if i >= 10 {
			w[i] = 300
		}
	}
	if a := Around(g, w, 50); !sameValue(a[9], 100) || !sameValue(a[10], 300) {
		t.Errorf("either side of a gap, %v and %v, want 100 and 300", a[9], a[10])
	}
}

// Cadence is doubled into steps a minute for sports where a FIT file counts
// one leg, and left in revolutions a minute otherwise, an unknown sport
// included; a recorded 0 is 0 and no reading is not known.
func TestCadence(t *testing.T) {
	pts := []course.Point{{HasCadence: true, Cadence: 85}, {HasCadence: true, Cadence: 0}, {}}
	for sport, want := range map[string]struct {
		first float64
		unit  string
	}{
		"running": {170, "spm"}, "Walking": {170, "spm"}, "hiking": {170, "spm"},
		"cycling": {85, "rpm"}, "": {85, "rpm"},
	} {
		v, unit := Cadence(&course.Course{Sport: sport, Points: pts})
		if v[0] != want.first || unit != want.unit || v[1] != 0 || !math.IsNaN(v[2]) {
			t.Errorf("sport %q: cadence %v %s, want %v %s, then 0, then not known", sport, v, unit, want.first, want.unit)
		}
	}
	a := AirPower(&course.Course{Points: []course.Point{{HasAirPower: true, AirPower: -2}, {}}})
	if a[0] != -2 || !math.IsNaN(a[1]) {
		t.Errorf("air power %v, want -2 -- a tailwind -- then not known", a)
	}
}

// The energy cost of running on a slope is Minetti's: 3.6 J/kg/m on the
// flat, about 1.7 times that at +10%, least about -20%, and taken as the
// end of what was measured past ±45%.
func TestMinetti(t *testing.T) {
	if c := minetti(0); c != 3.6 {
		t.Errorf("on the flat, %v, want 3.6", c)
	}
	if r := minetti(0.10) / minetti(0); r < 1.6 || r > 1.8 {
		t.Errorf("at +10%%, %v times the flat, want about 1.7", r)
	}
	if !(minetti(-0.20) < minetti(-0.10)) || !(minetti(-0.20) < minetti(-0.30)) {
		t.Errorf("descending costs least about -20%%: %v at -10%%, %v at -20%%, %v at -30%%", minetti(-0.10), minetti(-0.20), minetti(-0.30))
	}
	if minetti(0.9) != minetti(0.45) || minetti(-0.9) != minetti(-0.45) {
		t.Error("past ±45% the polynomial is used where nobody measured")
	}
}

// Grade-adjusted speed is the speed on level ground, faster up a climb by
// what the climb costs, and not known where the grade is not.
func TestGradeAdjustedSpeed(t *testing.T) {
	flat := eastward(101, every(2*time.Second))
	for i := range flat.Points {
		flat.Points[i].HasElevation, flat.Points[i].Elevation = true, 10
	}
	if v := GradeAdjustedSpeed(flat, 30)[50]; !sameValue(v, 5) {
		t.Errorf("on the flat at 5 m/s, %v, want 5", v)
	}
	up := hill(func(int) float64 { return 0 }) // 5% up, 3 s a point: 3.33 m/s
	g := GradeAdjustedSpeed(up, 30)
	want := 10.0 / 3 * minetti(0.05) / minetti(0)
	if math.Abs(g[25]-want) > 0.03*want {
		t.Errorf("up 5%% at 3.33 m/s, %v, want %v", g[25], want)
	}
	// Near the top, where the grade changes within the window, the grade
	// is taken over the same stretch as the speed.
	if w := Speed(up, 30)[48] * minetti(Grade(up, 30)[48]) / minetti(0); !sameValue(g[48], w) {
		t.Errorf("near the top, %v, want %v: speed and grade over the same 30 m", g[48], w)
	}
	if down := g[75]; down >= 10.0/3 {
		t.Errorf("down 5%% at 3.33 m/s, %v, want slower: a gentle descent is easier", down)
	}
	for i := range up.Points {
		up.Points[i].HasElevation = false
	}
	if v := GradeAdjustedSpeed(up, 30)[25]; !math.IsNaN(v) {
		t.Errorf("with no elevation, %v, want not known", v)
	}
}

// A range counted by ground leaves out a standstill however long it lasted;
// counted by points, a long enough one sets the slow end.
func TestSpreadAlong(t *testing.T) {
	// 100 points 10 m apart at 5 m/s, then 60 more standing still.
	c := eastward(160, every(2*time.Second))
	v := make([]float64, 160)
	for i := range c.Points {
		v[i] = 5
		if i >= 100 {
			c.Points[i].Lon = c.Points[99].Lon
			v[i] = 0
		}
	}
	if lo, _, _ := Spread(v, 0.05); lo != 0 {
		t.Fatalf("by points the slow end is %v; the test needs it to be the standstill", lo)
	}
	if lo, hi, ok := SpreadAlong(c, v, 0.05); !ok || lo != 5 || hi != 5 {
		t.Errorf("by ground, %v-%v %v, want 5-5: the standstill covers none", lo, hi, ok)
	}
	still := eastward(5, every(time.Second))
	for i := range still.Points {
		still.Points[i].Lon = 20
	}
	if lo, hi, ok := SpreadAlong(still, []float64{1, 2, 3, 4, 5}, 0); !ok || lo != 1 || hi != 5 {
		t.Errorf("on a course that never moves, %v-%v %v, want the points' own 1-5", lo, hi, ok)
	}
	if _, _, ok := SpreadAlong(still, []float64{math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()}, 0); ok {
		t.Error("a range of nothing known")
	}
}
