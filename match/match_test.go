package match

import (
	"math"
	"testing"
	"time"

	"github.com/wisborg/course"
)

// Every course here is invented, near 10°N 20°E. A metre east is about
// 1/109,600 of a degree at 10°N and a metre north 1/110,600.
const east, north = 1 / 109_600.0, 1 / 110_600.0

// path is a course through the corners given, in metres east and north of
// the origin, a fix every 2 m and every second, starting after offset
// seconds.
func path(offset int, corners ...[2]float64) *course.Course {
	c := &course.Course{Timed: true}
	t := offset
	add := func(x, y float64) {
		c.Points = append(c.Points, course.Point{Lat: 10 + y*north, Lon: 20 + x*east, Elapsed: time.Duration(t) * time.Second})
		t++
	}
	add(corners[0][0], corners[0][1])
	for i := 1; i < len(corners); i++ {
		a, b := corners[i-1], corners[i]
		n := int(math.Hypot(b[0]-a[0], b[1]-a[1]) / 2)
		for k := 1; k <= n; k++ {
			f := float64(k) / float64(n)
			add(a[0]+f*(b[0]-a[0]), a[1]+f*(b[1]-a[1]))
		}
	}
	return c
}

// join is several courses one after another, as one.
func join(cs ...*course.Course) *course.Course {
	out := &course.Course{Timed: true}
	var t time.Duration
	for _, c := range cs {
		for _, p := range c.Points {
			p.Elapsed += t
			out.Points = append(out.Points, p)
		}
		t = out.Points[len(out.Points)-1].Elapsed + time.Second
	}
	return out
}

// An out-and-back course, 1 km out along a road and back on its other side,
// 8 m over -- the case a nearest-point comparison gets wrong.
var outAndBack = [][2]float64{{0, 0}, {1000, 0}, {1000, 8}, {0, 8}}

func reference() []Reference {
	return []Reference{{Name: "Loop", Course: path(0, outAndBack...)}}
}

func find(t *testing.T, activity *course.Course) []Match {
	t.Helper()
	return Find(activity, reference(), Options{})
}

// The course itself matches, whole and closely.
func TestACourseMatchesItself(t *testing.T) {
	ms := find(t, path(0, outAndBack...))
	if len(ms) != 1 || ms[0].Coverage != 1 || ms[0].Worst > 1 || ms[0].From > 5 || ms[0].To < 2000 {
		t.Errorf("matches %+v", ms)
	}
}

// Inside a longer run -- a warm-up before, a cool-down after -- the course
// is found where it was, in distance and in time.
func TestTheCourseIsFoundInsideARun(t *testing.T) {
	warmUp := path(0, [2]float64{-600, -300}, [2]float64{0, 0})
	coolDown := path(0, [2]float64{0, 8}, [2]float64{-500, 500})
	ms := find(t, join(warmUp, path(0, outAndBack...), coolDown))
	if len(ms) != 1 {
		t.Fatalf("%d matches", len(ms))
	}
	m := ms[0]
	if math.Abs(m.From-670) > 20 || math.Abs(m.To-2686) > 20 {
		t.Errorf("found from %.0f m to %.0f m; want about 670 to 2686", m.From, m.To)
	}
	if m.FromTime < 300*time.Second || m.FromTime > 350*time.Second {
		t.Errorf("found from %v into the run; want about 5m35s", m.FromTime)
	}
}

// Run twice in one activity, the course is found twice, not overlapping.
func TestACourseRunTwiceIsFoundTwice(t *testing.T) {
	between := path(0, [2]float64{0, 8}, [2]float64{-300, 200}, [2]float64{0, 0})
	ms := find(t, join(path(0, outAndBack...), between, path(0, outAndBack...)))
	if len(ms) != 2 || ms[0].To > ms[1].From {
		t.Fatalf("matches %+v", ms)
	}
}

// Backwards round a loop, the course is not matched: every point of it is
// on the course, but not in order, and coverage is measured in order. (An
// out-and-back run backwards is the same course within GPS's reach, and
// would match.)
func TestALoopRunBackwardsIsNotMatched(t *testing.T) {
	loop := [][2]float64{{0, 0}, {300, 0}, {300, 300}, {0, 300}, {0, 0}}
	back := make([][2]float64, len(loop))
	for i, c := range loop {
		back[len(back)-1-i] = c
	}
	refs := []Reference{{Name: "Square", Course: path(0, loop...)}}
	if ms := Find(path(0, loop...), refs, Options{}); len(ms) != 1 {
		t.Fatalf("the loop does not match itself: %+v", ms)
	}
	if ms := Find(path(0, back...), refs, Options{}); len(ms) != 0 {
		t.Errorf("the loop run backwards matched: %+v", ms)
	}
}

// A detour of 100 m around a closed stretch still matches, and says where
// the course was missed and where the run left it.
func TestADetourIsReported(t *testing.T) {
	detour := [][2]float64{{0, 0}, {400, 0}, {400, -100}, {600, -100}, {600, 0}, {1000, 0}, {1000, 8}, {0, 8}}
	ms := find(t, path(0, detour...))
	if len(ms) != 1 {
		t.Fatalf("%d matches", len(ms))
	}
	m := ms[0]
	if len(m.Missed) != 1 || math.Abs(m.Missed[0].From-430) > 20 || math.Abs(m.Missed[0].To-570) > 20 || m.Missed[0].Farthest < 25 {
		t.Errorf("missed %+v; want about 430 m to 570 m of the course", m.Missed)
	}
	if len(m.Excursions) != 1 || m.Excursions[0].Farthest < 95 {
		t.Errorf("excursions %+v; want the one 100 m off", m.Excursions)
	}
	if m.Coverage > 0.95 || m.Coverage < 0.9 {
		t.Errorf("coverage %.2f", m.Coverage)
	}
}

// A different course nearby is not the course: parallel 200 m north, or
// half of it.
func TestADifferentCourseIsNotMatched(t *testing.T) {
	for name, c := range map[string]*course.Course{
		"200 m north":      path(0, [2]float64{0, 200}, [2]float64{1000, 200}, [2]float64{1000, 208}, [2]float64{0, 208}),
		"the way out only": path(0, [2]float64{0, 0}, [2]float64{1000, 0}),
		"far away":         path(0, [2]float64{50_000, 0}, [2]float64{51_000, 0}),
	} {
		if ms := find(t, c); len(ms) != 0 {
			t.Errorf("%s matched: %+v", name, ms)
		}
	}
}

// Resampling puts a point every step along the line, interpolated between
// the fixes either side, in place and in time: 25 m along a line with a fix
// every 2 m is halfway between the fixes at 24 m and 26 m.
func TestResample(t *testing.T) {
	s := Resample(path(0, [2]float64{0, 0}, [2]float64{100, 0}), 5)
	if len(s) != 21 || math.Abs(s[5].Along-25) > 1e-6 || math.Abs(s[20].Along-100) > 0.5 {
		t.Fatalf("%d samples, the sixth %.2f m along", len(s), s[5].Along)
	}
	if got := (s[5].Lon - 20) / east; math.Abs(got-25) > 0.05 {
		t.Errorf("25 m along is placed at %.2f m", got)
	}
	if d := s[5].Elapsed - 12500*time.Millisecond; d < -20*time.Millisecond || d > 20*time.Millisecond {
		t.Errorf("25 m along a 2 m-a-second line at %v; want 12.5 s", s[5].Elapsed)
	}
}

// A third of the course followed 50 m off is not the course, however close
// the rest.
func TestACourseFollowedPartlyIsNotMatched(t *testing.T) {
	off := [][2]float64{{0, 0}, {600, 0}, {600, 50}, {1000, 50}, {1000, 58}, {0, 8}}
	if ms := find(t, path(0, off...)); len(ms) != 0 {
		t.Errorf("matched with a third of it 50 m off: %+v", ms)
	}
}

// A fix or two off the line is GPS, not a detour: nothing is reported. A
// detour that lasts is, as TestADetourIsReported shows.
func TestAStrayFixIsNotADetour(t *testing.T) {
	c := path(0, outAndBack...)
	c.Points[100].Lat += 40 * north
	ms := find(t, c)
	if len(ms) != 1 || len(ms[0].Missed) != 0 || len(ms[0].Excursions) != 0 {
		t.Errorf("one fix 40 m off: %+v", ms)
	}
}

// Matches come in the order they happened, whatever order the references
// were given in.
func TestMatchesAreInTheOrderTheyHappened(t *testing.T) {
	a := path(0, [2]float64{0, 0}, [2]float64{1000, 0})
	b := path(0, [2]float64{0, 2000}, [2]float64{1000, 2000})
	run := join(a, path(0, [2]float64{1000, 0}, [2]float64{0, 2000}), b)
	ms := Find(run, []Reference{{Name: "B", Course: b}, {Name: "A", Course: a}}, Options{})
	if len(ms) != 2 || ms[0].Reference != "A" || ms[1].Reference != "B" {
		t.Errorf("matches %+v; want A then B", ms)
	}
}

// Standing still for five minutes 20 m off the course -- within the
// tolerance, so no detour -- is a stop, found by time, where it was along
// the course -- the time between two samples 10 m apart, so a few seconds of
// moving with it; a pause of a few seconds is not.
func TestAStopIsFoundByTime(t *testing.T) {
	out := path(0, [2]float64{0, 0}, [2]float64{400, 0}, [2]float64{400, -20})
	back := path(0, [2]float64{400, -20}, [2]float64{400, 0}, [2]float64{1000, 0}, [2]float64{1000, 8}, [2]float64{0, 8})
	run := join(out, back)
	// Five minutes at the toilet, 20 m off the course.
	for i := len(out.Points); i < len(run.Points); i++ {
		run.Points[i].Elapsed += 5 * time.Minute
	}
	ms := find(t, run)
	if len(ms) != 1 || len(ms[0].Excursions) != 0 {
		t.Fatalf("matches %+v; want one, with no detour", ms)
	}
	stops := ms[0].Stops
	if len(stops) != 1 || stops[0].Duration < 5*time.Minute || stops[0].Duration > 5*time.Minute+10*time.Second ||
		math.Abs(stops[0].At-400) > 20 || stops[0].Off < 15 || stops[0].Off > 25 {
		t.Errorf("stops %+v; want five minutes, 400 m along, about 20 m off", stops)
	}

	brief := join(out, back)
	for i := len(out.Points); i < len(brief.Points); i++ {
		brief.Points[i].Elapsed += 20 * time.Second
	}
	if ms := find(t, brief); len(ms) != 1 || len(ms[0].Stops) != 0 {
		t.Errorf("a 20-second pause is a stop: %+v", ms)
	}
}

// Two stops at one place a minute apart, with a few steps between, are one.
func TestAStopWithStepsInItIsOneStop(t *testing.T) {
	out := path(0, [2]float64{0, 0}, [2]float64{400, 0}, [2]float64{400, -20})
	shuffle := path(0, [2]float64{400, -20}, [2]float64{400, -40}, [2]float64{400, -20})
	back := path(0, [2]float64{400, -20}, [2]float64{400, 0}, [2]float64{1000, 0}, [2]float64{1000, 8}, [2]float64{0, 8})
	run := join(out, shuffle, back)
	for i := len(out.Points); i < len(run.Points); i++ {
		run.Points[i].Elapsed += 2 * time.Minute
	}
	for i := len(out.Points) + len(shuffle.Points); i < len(run.Points); i++ {
		run.Points[i].Elapsed += 2 * time.Minute
	}
	ms := find(t, run)
	if len(ms) != 1 || len(ms[0].Stops) != 1 || ms[0].Stops[0].Duration < 4*time.Minute {
		t.Errorf("stops %+v; want one of over four minutes", ms[0].Stops)
	}
}

// Each point of the reference has when and where the activity reached it,
// never earlier than the point before: at 2 m a second, the point 500 m in
// is reached 250 s after the start of the stretch, and a stop before it
// adds its length.
func TestArrivals(t *testing.T) {
	ms := find(t, path(0, outAndBack...))
	if len(ms) != 1 {
		t.Fatalf("%d matches", len(ms))
	}
	arr := ms[0].Arrivals
	if len(arr) < 200 {
		t.Fatalf("%d arrivals for a 2 km course sampled every 10 m", len(arr))
	}
	if got := arr[50].Elapsed; got < 245*time.Second || got > 255*time.Second {
		t.Errorf("500 m in at %v; want about 250 s", got)
	}
	for j := 1; j < len(arr); j++ {
		if arr[j].Elapsed < arr[j-1].Elapsed {
			t.Fatalf("time runs back at %d", j)
		}
	}
	if math.Abs((arr[50].Lon-20)/east-500) > 15 {
		t.Errorf("500 m in is placed %.0f m east", (arr[50].Lon-20)/east)
	}
}

// A course's own stops are found along itself, the way an activity's are
// along a reference; a course with no times has none.
func TestStops(t *testing.T) {
	c := path(0, [2]float64{0, 0}, [2]float64{1000, 0})
	for i := 300; i < len(c.Points); i++ { // 600 m along
		c.Points[i].Elapsed += 3 * time.Minute
	}
	stops := Stops(c, 10)
	if len(stops) != 1 || math.Abs(stops[0].At-600) > 10 || stops[0].Duration < 3*time.Minute || stops[0].Duration > 3*time.Minute+10*time.Second || stops[0].Off != 0 {
		t.Errorf("stops %+v; want three minutes, 600 m along", stops)
	}
	c.Timed = false
	if stops := Stops(c, 10); len(stops) != 0 {
		t.Errorf("an untimed course has stops %+v", stops)
	}
}
