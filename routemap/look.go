package routemap

import (
	"image/color"

	"github.com/wisborg/osmbase/render"
)

// Pattern is how a line is broken along its length.
type Pattern int

const (
	Solid Pattern = iota
	Dashed
	Dotted
)

// Look is how a line is drawn, sized for a picture about a thousand pixels
// across and scaled with the picture.
type Look struct {
	// Width is the line's full width.
	Width float64
	// Opacity is from 0, invisible, to 1, opaque. Below 1 the map shows
	// through the line.
	Opacity float64
	// Halo is how far a band of the map's background reaches either side
	// of the line; 0 draws none.
	Halo    float64
	Pattern Pattern
}

// line is points drawn in look l and ink at scale: as wide as l says, its
// ink at l's opacity, broken as l's pattern says, with a halo in haloInk if
// l has one.
func (l Look) line(points []render.Coord, ink, haloInk color.RGBA, scale float64) render.Line {
	w := l.Width * scale
	out := render.Line{Points: points, Ink: Translucent(ink, l.Opacity), Width: w, Halo: l.Halo * scale, HaloInk: haloInk}
	switch l.Pattern {
	case Dashed:
		out.Dash = []float32{float32(5 * w), float32(3 * w)}
	case Dotted:
		// A dash of next to no length is a dot: its round caps are a disc
		// the line's width across.
		out.Dash = []float32{float32(0.01 * w), float32(2 * w)}
	}
	return out
}

// Translucent is c at a share a of its opacity, premultiplied as color.RGBA
// is: every channel scaled, not only alpha, or the colour is not a valid
// premultiplied one and composites as something else.
func Translucent(c color.RGBA, a float64) color.RGBA {
	if a >= 1 {
		return c
	}
	if a <= 0 {
		return color.RGBA{}
	}
	scale := func(v uint8) uint8 { return uint8(float64(v)*a + 0.5) }
	return color.RGBA{R: scale(c.R), G: scale(c.G), B: scale(c.B), A: scale(c.A)}
}
