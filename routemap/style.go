package routemap

import (
	"image/color"

	"github.com/wisborg/osmbase/render"
)

// LineStyle is how heavily a map's lines are drawn over it.
type LineStyle int

const (
	// Light is thin and translucent, with no halo: the streets, paths and
	// names under the course show through it, which an opaque line with a
	// halo either side hid for its whole length.
	Light LineStyle = iota
	// Solid is thin and opaque, with a slim halo. It is for a map where a
	// reference is drawn whole beside the course: two translucent lines
	// over one another mix into a third colour, and which is which is
	// lost, where two opaque ones stay two.
	Solid
)

// lightAlpha is how opaque a Light line is: enough that the course reads as
// a line at a glance, little enough that a road and its name read through.
const lightAlpha = 0.7

// Styled is d with its lines drawn in style s: narrower than Drawing,
// ReferenceLines and WithReferences make them -- three quarters for Light,
// five eighths for Solid, dashes scaled with them -- with Light's inks
// translucent and its halos gone, and Solid's halos halved. Markers and
// gradients are left as they are: a dot is small, and a coloured line is
// already thin with a dark edge.
func Styled(d render.Drawing, s LineStyle) render.Drawing {
	width, halo, alpha := 0.75, 0.0, lightAlpha
	if s == Solid {
		width, halo, alpha = 0.625, 0.5, 1
	}
	lines := make([]render.Line, len(d.Lines))
	for i, l := range d.Lines {
		l.Width *= width
		l.Halo *= halo
		l.Ink = translucent(l.Ink, alpha)
		if len(l.Dash) > 0 {
			dash := make([]float32, len(l.Dash))
			for k, v := range l.Dash {
				dash[k] = v * float32(width)
			}
			l.Dash = dash
		}
		lines[i] = l
	}
	d.Lines = lines
	return d
}

// translucent is c at a share a of its opacity, premultiplied as color.RGBA
// is: every channel scaled, not only alpha, or the colour is not a valid
// premultiplied one and composites as something else.
func translucent(c color.RGBA, a float64) color.RGBA {
	if a >= 1 {
		return c
	}
	scale := func(v uint8) uint8 { return uint8(float64(v)*a + 0.5) }
	return color.RGBA{R: scale(c.R), G: scale(c.G), B: scale(c.B), A: scale(c.A)}
}
