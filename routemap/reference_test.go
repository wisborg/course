package routemap

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
	"github.com/wisborg/course/match"
)

func between(a, b course.Point) *course.Course {
	return &course.Course{Timed: true, Points: []course.Point{a, {Lat: (a.Lat + b.Lat) / 2, Lon: (a.Lon + b.Lon) / 2, Elapsed: time.Hour}, b}}
}

// The great circle is the shortest way over the globe, not a straight line
// on the map: along the equator it is the equator, and between two points
// on the 45th parallel half the world apart it runs over the pole.
func TestGreatCircle(t *testing.T) {
	gc, ok := GreatCircle(between(course.Point{Lat: 0, Lon: 0}, course.Point{Lat: 0, Lon: 90}))
	if !ok || gc.Name != "Great circle" {
		t.Fatal("no great circle between two points a quarter of the world apart")
	}
	for _, p := range gc.Points {
		if math.Abs(p.Lat) > 1e-9 {
			t.Fatalf("the great circle along the equator leaves it at %v", p)
		}
	}
	if n := len(gc.Points); n < 400 || n > 600 {
		t.Errorf("%d points for 10,000 km; want one every 20 km", n)
	}

	gc, _ = GreatCircle(between(course.Point{Lat: 45, Lon: 0}, course.Point{Lat: 45, Lon: 180}))
	top := -90.0
	for _, p := range gc.Points {
		top = math.Max(top, p.Lat)
	}
	if top < 89.9 {
		t.Errorf("half the world apart on the 45th parallel, the great circle reaches only %.2f°N; want the pole", top)
	}

	if _, ok := GreatCircle(between(course.Point{Lat: 10, Lon: 20}, course.Point{Lat: 10, Lon: 20.001})); ok {
		t.Error("a course ending where it started has a great circle")
	}
}

// References are dashed, each in the next ink, and thinner than the course.
func TestReferenceLines(t *testing.T) {
	refs := []Reference{
		{Name: "a", Points: []render.Coord{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}}},
		{Name: "no line", Points: []render.Coord{{Lat: 0, Lon: 0}}},
		{Name: "b", Points: []render.Coord{{Lat: 1, Lon: 0}, {Lat: 1, Lon: 1}}},
	}
	inks := ReferenceInks(render.LightPalette())
	lines := ReferenceLines(refs, inks, render.LightPalette().Background, 1)
	if len(lines) != 2 {
		t.Fatalf("%d lines; a reference with one point is not a line", len(lines))
	}
	course, _, _, _ := sizes(1)
	for i, l := range lines {
		if len(l.Dash) == 0 || l.Width >= course {
			t.Errorf("reference %d is dashed %v, %v wide; want dashed and thinner than the course's %v", i, l.Dash, l.Width, course)
		}
	}
	if lines[0].Ink == lines[1].Ink {
		t.Error("two references share an ink")
	}
}

// Every reference ink reads against the map it is drawn on: a light map's
// land, water and background, and a dark map's. And none is one of a
// course's own inks.
func TestReferenceInksReadOnTheirMap(t *testing.T) {
	for name, pal := range map[string]struct {
		p render.Palette
		o render.Overlay
	}{"light": {render.LightPalette(), render.LightOverlay()}, "dark": {render.DarkPalette(), render.DarkOverlay()}} {
		own := InksFor(pal.p, pal.o)
		for i, ink := range ReferenceInks(pal.p) {
			for what, under := range map[string]any{"land": pal.p.Land, "water": pal.p.Water, "background": pal.p.Background} {
				c := under.(interface{ RGBA() (r, g, b, a uint32) })
				if got := render.ContrastRatio(ink, c); got < 2.5 {
					t.Errorf("%s palette: reference ink %d is %.2f against %s", name, i, got, what)
				}
			}
			for _, o := range []any{own.Route, own.Start, own.Finish, own.Gap} {
				if o == ink {
					t.Errorf("%s palette: reference ink %d is one of the course's own", name, i)
				}
			}
		}
	}
}

// With references, they go under the course and the course is narrowed so a
// reference that follows it shows along its edges; with none, nothing
// changes.
func TestWithReferences(t *testing.T) {
	course := render.Drawing{Lines: []render.Line{{Points: []render.Coord{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}}, Width: 4, Halo: 2}}}
	if got := WithReferences(course, nil, nil, render.LightPalette().Background, 1); len(got.Lines) != 1 || got.Lines[0].Width != 4 {
		t.Errorf("with no references the drawing changed: %+v", got.Lines)
	}
	refs := []Reference{{Name: "r", Points: []render.Coord{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}}}}
	got := WithReferences(course, refs, ReferenceInks(render.LightPalette()), render.LightPalette().Background, 1)
	if len(got.Lines) != 2 || len(got.Lines[0].Dash) == 0 || len(got.Lines[1].Dash) != 0 {
		t.Fatalf("lines %+v; want the reference first, then the course", got.Lines)
	}
	if c, r := got.Lines[1], got.Lines[0]; c.Width+2*c.Halo >= r.Width+2*r.Halo || c.Width >= 4 {
		t.Errorf("the course is %v wide with a %v halo over a %v reference with a %v halo; want it narrower, edges showing", c.Width, c.Halo, r.Width, r.Halo)
	}
	if course.Lines[0].Width != 4 {
		t.Error("the course's own drawing was changed in place")
	}
}

// line1km is a course 1 km east along 10°N from 20°E, a point every 10 m.
func line1km() *course.Course {
	c := &course.Course{}
	for x := 0.0; x <= 1000; x += 10 {
		c.Points = append(c.Points, course.Point{Lat: 10, Lon: 20 + x/109_600})
	}
	return c
}

// A followed reference is drawn where the course missed it, reaching a
// little past each end so it is seen leaving the course and coming back;
// two such stretches close enough to touch are one; and one followed all the
// way draws nothing.
func TestFollowed(t *testing.T) {
	ref := line1km()
	m := match.Match{Missed: []match.Stretch{{From: 200, To: 300}, {From: 340, To: 400}, {From: 800, To: 850}}}
	r := Followed("Loop", ref, m)
	if !r.Follows || r.Name != "Loop" || len(r.Points) != len(ref.Points) {
		t.Fatalf("Followed = %+v", r)
	}
	if len(r.Apart) != 2 {
		t.Fatalf("drawn in %d pieces, want 2: 200-400 m, the two stretches joined, and 800-850 m", len(r.Apart))
	}
	along := func(c render.Coord) float64 { return (c.Lon - 20) * 109_600 }
	for i, want := range [][2]float64{{170, 430}, {770, 880}} {
		p := r.Apart[i]
		if a, b := along(p[0]), along(p[len(p)-1]); math.Abs(a-want[0]) > 11 || math.Abs(b-want[1]) > 11 {
			t.Errorf("piece %d runs %.0f-%.0f m, want %.0f-%.0f", i, a, b, want[0], want[1])
		}
	}
	if d := r.Drawn(); len(d) != 2 {
		t.Errorf("drawn %d pieces, want the 2 apart", len(d))
	}
	if all := Followed("Loop", ref, match.Match{}); !all.Follows || len(all.Apart) != 0 || len(all.Drawn()) != 0 {
		t.Errorf("a reference followed all the way is drawn: %+v", all.Drawn())
	}
	if whole := FromCourse("Loop", ref); len(whole.Drawn()) != 1 || len(whole.Drawn()[0]) != len(ref.Points) {
		t.Errorf("a reference not followed is not drawn whole")
	}
}

// A reference drawn only where it parts needs no room beside the course,
// which keeps its width; one drawn whole still narrows it.
func TestWithReferencesKeepsTheCourseWideForDepartures(t *testing.T) {
	d := render.Drawing{Lines: []render.Line{{Points: []render.Coord{{}, {Lat: 1}}, Width: 10, Halo: 2}}}
	inks := []color.RGBA{{A: 0xff}}
	apart := Followed("Loop", line1km(), match.Match{Missed: []match.Stretch{{From: 200, To: 300}}})
	got := WithReferences(d, []Reference{apart}, inks, color.RGBA{}, 1)
	if n := len(got.Lines); n != 2 || got.Lines[1].Width != 10 || got.Lines[0].Dash == nil {
		t.Errorf("with a departure drawn: %+v; want the departure under the course at its full width", got.Lines)
	}
	got = WithReferences(d, []Reference{apart, FromCourse("Other", line1km())}, inks, color.RGBA{}, 1)
	if last := got.Lines[len(got.Lines)-1]; last.Width != 10*thinned {
		t.Errorf("with a reference drawn whole, the course is %v wide, want narrowed", last.Width)
	}
	if got := WithReferences(d, []Reference{Followed("Loop", line1km(), match.Match{})}, inks, color.RGBA{}, 1); len(got.Lines) != 1 || got.Lines[0].Width != 10 {
		t.Errorf("with nothing of the reference to draw, the drawing changed: %+v", got.Lines)
	}
}

// Several activities' inks read on their map, differ from each other, and
// are none of the references' or of a course's own start, finish or gap.
func TestActivityInksReadOnTheirMap(t *testing.T) {
	for name, pal := range map[string]struct {
		p render.Palette
		o render.Overlay
	}{"light": {render.LightPalette(), render.LightOverlay()}, "dark": {render.DarkPalette(), render.DarkOverlay()}} {
		own := InksFor(pal.p, pal.o)
		inks := ActivityInks(pal.p, pal.o)
		if inks[0] != own.Route {
			t.Errorf("%s palette: the first activity is not in the course's own ink", name)
		}
		seen := map[color.RGBA]bool{}
		for i, ink := range inks {
			if seen[ink] {
				t.Errorf("%s palette: activity ink %d repeats one before it", name, i)
			}
			seen[ink] = true
			for what, under := range map[string]color.RGBA{"land": pal.p.Land, "water": pal.p.Water, "background": pal.p.Background} {
				if got := render.ContrastRatio(ink, under); got < 2.5 {
					t.Errorf("%s palette: activity ink %d is %.2f against %s", name, i, got, what)
				}
			}
			for _, o := range append([]color.RGBA{own.Start, own.Finish, own.Gap}, ReferenceInks(pal.p)...) {
				if o == ink {
					t.Errorf("%s palette: activity ink %d is a reference's or a course's own", name, i)
				}
			}
		}
	}
}
