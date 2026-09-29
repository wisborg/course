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
