package routemap

import (
	"image/color"
	"testing"

	"github.com/wisborg/osmbase/render"
)

// Light lines are three quarters as wide, without a halo, in their ink at
// 70% -- premultiplied, every channel scaled -- with their dashes scaled to
// match; Solid ones five eighths as wide, opaque, with half the halo.
// Markers and gradients are left as they are, and the drawing given is not
// changed.
func TestStyled(t *testing.T) {
	ink := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	d := render.Drawing{
		Lines:     []render.Line{{Ink: ink, Width: 8, Halo: 4, Dash: []float32{40, 24}}},
		Markers:   []render.Marker{{Radius: 6, Ink: ink}},
		Gradients: []render.Gradient{{Width: 3}},
	}
	light := Styled(d, Light)
	if l := light.Lines[0]; l.Width != 6 || l.Halo != 0 || l.Ink != (color.RGBA{R: 140, G: 70, B: 35, A: 179}) || l.Dash[0] != 30 || l.Dash[1] != 18 {
		t.Errorf("light: %+v", l)
	}
	solid := Styled(d, Solid)
	if l := solid.Lines[0]; l.Width != 5 || l.Halo != 2 || l.Ink != ink || l.Dash[0] != 25 {
		t.Errorf("solid: %+v", l)
	}
	if light.Markers[0].Radius != 6 || light.Markers[0].Ink != ink || light.Gradients[0].Width != 3 {
		t.Errorf("markers or gradients restyled: %+v %+v", light.Markers[0], light.Gradients[0])
	}
	if l := d.Lines[0]; l.Width != 8 || l.Dash[0] != 40 || l.Ink != ink {
		t.Errorf("the drawing given was changed: %+v", l)
	}
}
