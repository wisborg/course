package routemap

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// east is a course of n points 10 m apart east along 10°N from lon, with
// distance recorded so it has distance markers.
func east(lon float64, n int) *course.Course {
	c := &course.Course{Timed: true}
	for i := 0; i < n; i++ {
		c.Points = append(c.Points, course.Point{
			Lat: 10, Lon: lon + float64(i)*10/109_600,
			Elapsed:     time.Duration(i) * 3 * time.Second,
			HasDistance: true, Distance: float64(i) * 10,
		})
	}
	return c
}

func viewOf(cs ...*course.Course) render.View {
	b := render.Bounds{West: 180, South: 90, East: -180, North: -90}
	for _, c := range cs {
		for _, p := range c.Points {
			b.West, b.East = min(b.West, p.Lon), max(b.East, p.Lon)
			b.South, b.North = min(b.South, p.Lat-0.001), max(b.North, p.Lat+0.001)
		}
	}
	v, _ := render.Fit(b, 800, 600, 18)
	return v
}

// Two activities, the second starting where the first finished, and a
// third, a loop, elsewhere: each line is in its own ink, each keeps its
// distance markers, the handover is one dot labelled with both ends in
// order and in the start's ink, and the loop's start and finish are one.
func TestActivitiesDrawing(t *testing.T) {
	first := east(20, 201)                            // 2 km
	second := east(first.Points[200].Lon, 101)        // 1 km on from its finish
	loop := east(20.05, 101)                          // elsewhere
	loop.Points = append(loop.Points, loop.Points[0]) // and back to its start
	loop.Points[len(loop.Points)-1].Elapsed = 10 * time.Minute
	acts := []*course.Course{first, second, loop}
	inks := InksFor(render.LightPalette(), render.LightOverlay())
	actInks := []color.RGBA{{R: 1, A: 0xff}, {G: 1, A: 0xff}}
	d := ActivitiesDrawing(acts, viewOf(acts...), inks, actInks, 1)

	seen := map[color.RGBA]bool{}
	for _, l := range d.Lines {
		if l.Ink != inks.Gap { // the loop's way back, a gap in its recording
			seen[l.Ink] = true
		}
	}
	if len(seen) != 2 || !seen[actInks[0]] || !seen[actInks[1]] {
		t.Errorf("line inks %v; want the two activity inks, the third taking the first again", seen)
	}
	var labels []string
	var handover *render.Marker
	for i, m := range d.Markers {
		if strings.Contains(m.Label, "Start") || strings.Contains(m.Label, "Finish") {
			labels = append(labels, m.Label)
			if m.Label == "Finish 1 · Start 2" {
				handover = &d.Markers[i]
			}
		}
	}
	want := map[string]bool{"Start 1": true, "Finish 1 · Start 2": true, "Finish 2": true, "Start and finish 3": true}
	if len(labels) != len(want) {
		t.Errorf("end labels %q; want %v", labels, want)
	}
	for _, l := range labels {
		if !want[l] {
			t.Errorf("end label %q; want one of %v", l, want)
		}
	}
	if handover == nil || handover.Ink != inks.Start {
		t.Errorf("the handover is %+v; want it in the start's ink", handover)
	}
	want2 := 0
	for _, a := range acts {
		want2 += len(DistanceMarkers(a))
	}
	if got := len(d.Markers) - len(labels); want2 == 0 || got != want2 {
		t.Errorf("%d distance markers, want each activity's own, %d", got, want2)
	}
	// Starts go on top, drawn after every finish.
	last := d.Markers[len(d.Markers)-1]
	if last.Ink != inks.Start {
		t.Errorf("the last marker drawn is %q, not a start", last.Label)
	}
}

// Across several activities each value counts for the ground around it in
// its own activity: the way from one's finish to the next one's start is
// not ground anybody covered.
func TestSpreadAlongAll(t *testing.T) {
	a, b := east(20, 11), east(21, 11) // 100 m each, 110 km apart
	va, vb := make([]float64, 11), make([]float64, 11)
	for i := range va {
		va[i], vb[i] = 1, 2
	}
	// The ends' values would get 55 km of weight each if the jump counted.
	va[10], vb[0] = 100, -100
	lo, hi, ok := SpreadAlongAll([]*course.Course{a, b}, [][]float64{va, vb}, 0.1)
	if !ok || lo != 1 || hi != 2 {
		t.Errorf("spread %v-%v %v, want 1-2: the ends are a sliver of the ground", lo, hi, ok)
	}
	if got, _, _ := SpreadAlongAll([]*course.Course{a}, [][]float64{va}, 0); got != 1 {
		t.Errorf("one course's whole spread starts at %v, want 1", got)
	}
}
