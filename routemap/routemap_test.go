package routemap

import (
	"testing"
	"time"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// Courses here are invented, east along 10°N from 20°E.

func line(n int, step float64, every time.Duration, measured bool) *course.Course {
	c := &course.Course{Timed: every > 0}
	for i := 0; i < n; i++ {
		p := course.Point{Lat: 10, Lon: 20 + float64(i)*step, Elapsed: time.Duration(i) * every}
		if measured {
			p.HasDistance, p.Distance = true, float64(i)*step*109_600
		}
		c.Points = append(c.Points, p)
	}
	return c
}

func view(c *course.Course) render.View {
	b := render.Bounds{West: 180, South: 90, East: -180, North: -90}
	for _, p := range c.Points {
		b.West, b.East = min(b.West, p.Lon), max(b.East, p.Lon)
		b.South, b.North = min(b.South, p.Lat), max(b.North, p.Lat)
	}
	b.South, b.North = b.South-0.01, b.North+0.01
	v, _ := render.Fit(b, 1000, 600, 18)
	return v
}

func TestMarkerInterval(t *testing.T) {
	for _, tc := range []struct{ km, want float64 }{
		{1, 0}, {5, 1000}, {14.9, 1000}, {15, 2000}, {21.1, 3000}, {30, 4000}, {42.2, 5000}, {100, 10000},
	} {
		if got := MarkerInterval(tc.km * 1000); got != tc.want {
			t.Errorf("%v km: every %v m, want %v", tc.km, got, tc.want)
		}
	}
}

// Markers go at each whole interval of recorded distance, placed between
// the fixes either side in proportion, and none past the finish. A course
// with no recorded distance has none: they would be made up from the line.
func TestDistanceMarkers(t *testing.T) {
	c := line(56, 0.001, time.Second, true) // about 6 km
	ms := DistanceMarkers(c)
	if len(ms) != 6 || ms[0].Label != "1" || ms[5].Label != "6" {
		t.Fatalf("markers %+v; want 1 to 6", ms)
	}
	// 1000 m is 0.009124 degrees along, between the 9th and 10th fix.
	if got := ms[0].At.Lon - 20; got < 0.00912 || got > 0.00913 {
		t.Errorf("the first marker is %.6f° along; want 1000 m, placed between fixes", got)
	}
	if got := DistanceMarkers(line(56, 0.001, time.Second, false)); got != nil {
		t.Errorf("markers on a course with no recorded distance: %+v", got)
	}
	part := line(56, 0.001, time.Second, true)
	part.Points[30].HasDistance = false
	if got := DistanceMarkers(part); got != nil {
		t.Errorf("markers on a course with distance missing in places: %+v", got)
	}
}

// A gap in the recording is its own dashed stretch between the fixes either
// side; the rest is solid. A plan has no clock and no gaps.
func TestAGapIsDashed(t *testing.T) {
	c := line(40, 0.0005, time.Second, false)
	for i := 20; i < len(c.Points); i++ {
		c.Points[i].Elapsed += 10 * time.Minute
		c.Points[i].Lon += 0.01 // a kilometre on, ten minutes later
	}
	d := Drawing(c, view(c), Inks{}, testLook, 0, 1)
	var solid, dashed int
	for _, l := range d.Lines {
		if len(l.Dash) > 0 {
			dashed++
			if len(l.Points) != 2 || l.Points[0].Lon != c.Points[19].Lon || l.Points[1].Lon != c.Points[20].Lon {
				t.Errorf("the gap is drawn %+v; want the two fixes either side", l.Points)
			}
		} else {
			solid++
		}
	}
	if solid != 2 || dashed != 1 {
		t.Errorf("%d solid and %d dashed lines; want 2 and 1", solid, dashed)
	}

	c.Timed = false
	d = Drawing(c, view(c), Inks{}, testLook, 0, 1)
	if len(d.Lines) != 1 || len(d.Lines[0].Dash) != 0 {
		t.Errorf("a plan's line is %d lines, dashed %v", len(d.Lines), len(d.Lines) > 0 && len(d.Lines[0].Dash) > 0)
	}
}

// A pause where the course stood still is not a gap: nothing was missed.
func TestAPauseInPlaceIsNotAGap(t *testing.T) {
	c := line(40, 0.0005, time.Second, false)
	for i := 20; i < len(c.Points); i++ {
		c.Points[i].Elapsed += 10 * time.Minute
	}
	for _, l := range Drawing(c, view(c), Inks{}, testLook, 0, 1).Lines {
		if len(l.Dash) > 0 {
			t.Error("a pause in place was dashed as a gap")
		}
	}
}

// The line keeps a point only where it has moved a pixel, and always the
// ends: sixteen thousand fixes are not sixteen thousand points to stroke.
func TestTheLineIsThinnedToThePicture(t *testing.T) {
	c := line(5000, 0.000002, time.Second, false) // 1 m apart, 5 km
	v := view(c)
	d := Drawing(c, v, Inks{}, testLook, 0, 1)
	if len(d.Lines) != 1 {
		t.Fatalf("%d lines", len(d.Lines))
	}
	pts := d.Lines[0].Points
	if len(pts) > v.Width+2 || pts[0] != (render.Coord{Lat: 10, Lon: 20}) || pts[len(pts)-1].Lon != c.Points[len(c.Points)-1].Lon {
		t.Errorf("%d points kept for a line %d pixels across, ends %v %v", len(pts), v.Width, pts[0], pts[len(pts)-1])
	}
}

// A course that ends where it started has one label, not two written over
// each other.
func TestALoopHasOneLabelAtItsEnds(t *testing.T) {
	c := line(100, 0.0001, time.Second, false)
	c.Points = append(c.Points, c.Points[0])
	var labels []string
	for _, m := range Drawing(c, view(c), Inks{}, testLook, 0, 1).Markers {
		if m.Label != "" {
			labels = append(labels, m.Label)
		}
	}
	if len(labels) != 1 || labels[0] != "Start and finish" {
		t.Errorf("labels %v", labels)
	}
	c = line(100, 0.0001, time.Second, false)
	labels = nil
	for _, m := range Drawing(c, view(c), Inks{}, testLook, 0, 1).Markers {
		labels = append(labels, m.Label)
	}
	if len(labels) != 2 || labels[1] != "Start" || labels[0] != "Finish" {
		t.Errorf("labels %v; want the finish, then the start on top", labels)
	}
}

// A gap is measured against the recording's own rhythm: a flight tracker
// that reports every thirty seconds has not lost anything in ninety.
func TestAGapIsLongForItsRecording(t *testing.T) {
	c := line(40, 0.05, 30*time.Second, false) // 5 km every 30 s
	for i := 20; i < len(c.Points); i++ {
		c.Points[i].Elapsed += time.Minute // one report 90 s after the last
	}
	for _, l := range Drawing(c, view(c), Inks{}, testLook, 0, 1).Lines {
		if len(l.Dash) > 0 {
			t.Error("ninety seconds in a thirty-second recording was drawn as a gap")
		}
	}
}

// No marker is drawn at the finish itself: the finish has its own.
func TestNoMarkerAtTheFinish(t *testing.T) {
	c := line(50, 5000/(49*109_600.0), time.Second, true) // exactly 5 km
	ms := DistanceMarkers(c)
	if len(ms) != 4 || ms[3].Label != "4" {
		t.Errorf("markers %+v; want 1 to 4", ms)
	}
}

// testLook is a course's look as drawn by default: three pixels, a little
// translucent, solid.
var testLook = Look{Width: 3, Opacity: 0.7, Pattern: Solid}
