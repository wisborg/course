package routemap

import (
	"math"
	"testing"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// coords is c's points as a line to draw.
func coords(c *course.Course) []render.Coord { return FromCourse("", c).Points }

// A reference along the course is moved into its lane: offset pixels to its
// right -- south, for one running east -- along the whole of the shared
// stretch but its ends, and run the other way it is moved to the north.
func TestBesideMovesASharedReferenceIntoItsLane(t *testing.T) {
	c := line1km()
	v := view(c)
	along := coords(c)
	moved := beside(along, along, v, 6)
	for i := 20; i < len(moved)-20; i++ {
		a, b := pixel(v, along[i]), pixel(v, moved[i])
		if math.Abs(b[1]-a[1]-6) > 0.01 || math.Abs(b[0]-a[0]) > 0.01 {
			t.Fatalf("point %d moved by (%.2f, %.2f) px, want (0, 6): south of an eastward course", i, b[0]-a[0], b[1]-a[1])
		}
	}
	// The same road recorded 15 m to the north, as GPS on another day puts
	// it: about 4 px on this map, further than a 2 px lane, but within
	// matching's tolerance, so it is the same road and moved into the lane.
	north := make([]render.Coord, len(along))
	for i, p := range along {
		north[i] = render.Coord{Lat: p.Lat + 15.0/111_000, Lon: p.Lon}
	}
	if d := pixelDistance(v, north[50], along[50]); d <= 2 {
		t.Fatalf("15 m is %.2f px on this map; the test needs it further than the lane", d)
	}
	moved = beside(north, along, v, 2)
	if a, b := pixel(v, along[50]), pixel(v, moved[50]); math.Abs(b[1]-a[1]-2) > 0.01 {
		t.Errorf("a reference 15 m off the course is %.2f px from it, want 2: in the lane", b[1]-a[1])
	}

	back := make([]render.Coord, len(along))
	for i := range along {
		back[i] = along[len(along)-1-i]
	}
	moved = beside(back, along, v, 6)
	if a, b := pixel(v, back[50]), pixel(v, moved[50]); math.Abs(b[1]-a[1]+6) > 0.01 {
		t.Errorf("the way back moved by %.2f px, want -6: north, its own right", b[1]-a[1])
	}
}

// A reference nowhere near the course is left where it is, and so is one
// that only crosses it; a stretch that joins the course is eased into the
// lane, not jumped.
func TestBesideLeavesTheRestAlone(t *testing.T) {
	c := line1km()
	v := view(c)
	along := coords(c)

	far := make([]render.Coord, len(along))
	for i, p := range along {
		far[i] = render.Coord{Lat: p.Lat + 0.002, Lon: p.Lon} // 220 m north
	}
	for i, p := range beside(far, along, v, 6) {
		if p != far[i] {
			t.Fatalf("point %d of a reference 220 m away moved", i)
		}
	}

	// North to south across the course, through its middle.
	var across []render.Coord
	for y := -0.002; y <= 0.002; y += 0.0001 {
		across = append(across, render.Coord{Lat: 10 + y, Lon: 20 + 500.0/109_600})
	}
	moved := beside(across, along, v, 6)
	for i := range across {
		if d := pixelDistance(v, across[i], moved[i]); d > 3 {
			t.Errorf("a reference crossing the course moved %.1f px at point %d; want it barely touched", d, i)
		}
	}

	// Far to the north, then down onto the course and along it: the lane
	// is reached by steps no bigger than the ramp allows.
	var joining []render.Coord
	for y := 0.002; y > 0; y -= 0.0001 {
		joining = append(joining, render.Coord{Lat: 10 + y, Lon: 20})
	}
	joining = append(joining, along...)
	moved = beside(joining, along, v, 6)
	worst := 0.0
	for i := 1; i < len(moved); i++ {
		shift := func(j int) float64 { return pixelDistance(v, joining[j], moved[j]) }
		worst = math.Max(worst, math.Abs(shift(i)-shift(i-1)))
	}
	if worst > 3 {
		t.Errorf("joining the course the lane jumps by %.1f px between neighbours; want it eased", worst)
	}
}

// References drawn whole and beside are each given a lane of their own, the
// second outside the first, and the course keeps its width; one drawn whole
// but not beside narrows it as before; a followed one needs no lane.
func TestWithReferencesBeside(t *testing.T) {
	c := line1km()
	v := view(c)
	along := coords(c)
	d := render.Drawing{Lines: []render.Line{{Points: along, Width: 4}}}
	besideLook := Look{Width: 2, Opacity: 1, Beside: true}
	refs := []Reference{{Name: "a", Points: along}, {Name: "b", Points: along}}
	got := WithReferences(d, refs, ReferenceInks(render.LightPalette()), []Look{besideLook}, render.LightPalette().Background, 1, v, along)
	if len(got.Lines) != 3 || got.Lines[2].Width != 4 {
		t.Fatalf("lines %+v; want two references and the course, full width", got.Lines)
	}
	lane := func(l render.Line) float64 { return pixel(v, l.Points[50])[1] - pixel(v, along[50])[1] }
	// The course's half width 2, a gap of 1, the reference's half width 1:
	// 4 px out; the next another 2 + 1 further.
	if a, b := lane(got.Lines[0]), lane(got.Lines[1]); math.Abs(a-4) > 0.01 || math.Abs(b-7) > 0.01 {
		t.Errorf("lanes %.2f and %.2f px from the course, want 4 and 7", a, b)
	}
	under := WithReferences(d, refs[:1], ReferenceInks(render.LightPalette()), []Look{{Width: 2, Opacity: 1}}, render.LightPalette().Background, 1, v, along)
	if under.Lines[1].Width >= 4 || lane(under.Lines[0]) != 0 {
		t.Errorf("not beside: course %v wide, reference %.2f px off; want it narrowed and the reference on it", under.Lines[1].Width, lane(under.Lines[0]))
	}
	followed := Reference{Name: "f", Points: along, Follows: true, Apart: [][]render.Coord{along[:10]}}
	got = WithReferences(d, []Reference{followed, refs[0]}, ReferenceInks(render.LightPalette()), []Look{besideLook}, render.LightPalette().Background, 1, v, along)
	if math.Abs(lane(render.Line{Points: got.Lines[1].Points})-4) > 0.01 {
		t.Errorf("a followed reference took the first lane: the whole one is %.2f px off, want 4", lane(render.Line{Points: got.Lines[1].Points}))
	}
}
