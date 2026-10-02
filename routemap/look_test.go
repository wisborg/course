package routemap

import (
	"image/color"
	"testing"
	"time"

	"github.com/wisborg/osmbase/render"
)

// A look draws a line as wide as it says at the scale, its ink at its
// opacity -- premultiplied, every channel scaled -- with its halo, and
// broken as its pattern says: dashes five widths on and three off, dots a
// width across and two apart.
func TestLookLine(t *testing.T) {
	ink, halo := color.RGBA{R: 200, G: 100, B: 50, A: 255}, color.RGBA{R: 1, G: 2, B: 3, A: 255}
	pts := []render.Coord{{}, {Lat: 1}}
	l := Look{Width: 3, Opacity: 0.7, Halo: 1, Pattern: Solid}.line(pts, ink, halo, 2)
	if l.Width != 6 || l.Halo != 2 || l.HaloInk != halo || l.Ink != (color.RGBA{R: 140, G: 70, B: 35, A: 179}) || l.Dash != nil || len(l.Points) != 2 {
		t.Errorf("solid: %+v", l)
	}
	if l := (Look{Width: 2, Opacity: 1, Pattern: Dashed}).line(pts, ink, halo, 1); l.Ink != ink || len(l.Dash) != 2 || l.Dash[0] != 10 || l.Dash[1] != 6 {
		t.Errorf("dashed: %+v", l)
	}
	if l := (Look{Width: 2, Opacity: 1, Pattern: Dotted}).line(pts, ink, halo, 1); len(l.Dash) != 2 || l.Dash[0] >= 0.1 || l.Dash[1] != 4 {
		t.Errorf("dotted: %+v", l)
	}
	if l := (Look{Width: 2, Opacity: 0}).line(pts, ink, halo, 1); l.Ink != (color.RGBA{}) {
		t.Errorf("opacity 0: %+v", l.Ink)
	}
}

// A gap in the recording is drawn in the look's width and opacity, but in
// the gap's ink and always dashed, whatever the look's pattern.
func TestDrawingGapsKeepTheirDashes(t *testing.T) {
	// TestAGapIsDashed's course: a kilometre on, ten minutes later, halfway.
	c := line(40, 0.0005, time.Second, false)
	for i := 20; i < len(c.Points); i++ {
		c.Points[i].Elapsed += 10 * time.Minute
		c.Points[i].Lon += 0.01
	}
	inks := Inks{Route: color.RGBA{R: 9, A: 255}, Gap: color.RGBA{G: 9, A: 255}}
	d := Drawing(c, view(c), inks, Look{Width: 4, Opacity: 1, Pattern: Dotted}, 1)
	var gap, route bool
	for _, l := range d.Lines {
		switch l.Ink {
		case inks.Gap:
			gap = true
			if l.Width != 4 || l.Dash[0] != 12 {
				t.Errorf("the gap: %+v; want 4 wide, dashed 12 on", l)
			}
		case inks.Route:
			route = true
			if l.Dash[0] >= 1 {
				t.Errorf("the course: %+v; want dotted", l)
			}
		}
	}
	if !gap || !route {
		t.Errorf("gap drawn %v, course drawn %v", gap, route)
	}
}
