package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/wisborg/osmbase/render"
)

// entry is one line of a map's legend: what a line on the map is.
type entry struct {
	name   string
	ink    color.RGBA
	dashed bool
}

// drawLegend writes a legend in the top left of the picture, on a plate like
// the credit's, so it reads over any map: a short sample of each line, solid
// or dashed in its ink, and its name beside it.
//
// It is there because a dashed line on a map with nothing saying what it is
// invites the wrong reading -- a detour, a gap in the recording -- and with
// several references there is no other way to tell them apart.
func drawLegend(img *image.RGBA, entries []entry, face font.Face, scale float64) {
	if face == nil || len(entries) == 0 {
		return
	}
	pad := int(math.Round(6 * scale))
	sample := int(math.Round(30 * scale))
	thick := max(2, int(math.Round(3*scale)))
	lineH := face.Metrics().Height.Ceil() + pad/2
	width := 0
	for _, e := range entries {
		width = max(width, font.MeasureString(face, render.Visual(e.name)).Ceil())
	}
	plate := image.Rect(pad, pad, pad+2*pad+sample+pad+width, pad+2*pad+lineH*len(entries)).Intersect(img.Bounds())
	draw.Draw(img, plate, image.NewUniform(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xdd}), image.Point{}, draw.Over)

	for i, e := range entries {
		top := plate.Min.Y + pad + i*lineH
		mid := top + lineH/2
		x0 := plate.Min.X + pad
		dash, gapLen := sample, 0
		if e.dashed {
			dash, gapLen = max(3, sample/4), max(2, sample/7)
		}
		for x := x0; x < x0+sample; x += dash + gapLen {
			end := min(x+dash, x0+sample)
			draw.Draw(img, image.Rect(x, mid-thick/2, end, mid-thick/2+thick), image.NewUniform(e.ink), image.Point{}, draw.Over)
		}
		d := font.Drawer{
			Dst: img, Src: image.NewUniform(color.RGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xff}), Face: face,
			Dot: fixed.P(x0+sample+pad, mid+(face.Metrics().Ascent.Ceil()-face.Metrics().Descent.Ceil())/2),
		}
		d.DrawString(render.Visual(e.name))
	}
}
