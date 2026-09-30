package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/wisborg/osmbase/render"
)

// entry is one line of a map's legend: what a line on the map is, or a
// colour bar saying what a coloured line's colours mean.
type entry struct {
	name   string
	ink    color.RGBA
	dashed bool
	// ramp, when set, makes the entry a colour bar, with name above it.
	ramp *ramp
}

// ramp is a coloured line's scale and the words for its two ends.
type ramp struct {
	scale     render.Scale
	low, high string
}

// legendMetrics are the legend's sizes at scale: its padding, the length of
// a line's sample and a colour bar, and a line's thickness and row height.
type legendMetrics struct{ pad, sample, thick, lineH, bar int }

func metricsFor(face font.Face, scale float64) legendMetrics {
	pad := int(math.Round(6 * scale))
	sample := int(math.Round(30 * scale))
	return legendMetrics{
		pad: pad, sample: sample, thick: max(2, int(math.Round(3*scale))),
		lineH: face.Metrics().Height.Ceil() + pad/2, bar: 4 * sample,
	}
}

// legendSize is the size of the legend's plate.
func legendSize(entries []entry, face font.Face, scale float64) image.Point {
	m := metricsFor(face, scale)
	width, rows := 0, 0
	for _, e := range entries {
		if e.ramp != nil {
			ends := font.MeasureString(face, e.ramp.low).Ceil() + font.MeasureString(face, e.ramp.high).Ceil()
			width = max(width, font.MeasureString(face, e.name).Ceil()-m.sample-m.pad, m.bar+ends+2*m.pad-m.sample-m.pad)
			rows += 2
			continue
		}
		width = max(width, font.MeasureString(face, render.Visual(e.name)).Ceil())
		rows++
	}
	return image.Pt(2*m.pad+m.sample+m.pad+width, 2*m.pad+m.lineH*rows)
}

// legendCorners are where --legend may put the legend, in the order auto
// prefers them when two are as clear as each other: top left first, where a
// legend is looked for.
var legendCorners = []string{"top-left", "top-right", "bottom-left", "bottom-right"}

// legendPlate is where a legend of size goes in bounds, in corner, margin in
// from the edges. In the bottom right it stands above the map's credit, which
// is drawn there, credit pixels tall; a legend is never drawn over the credit
// the licence asks for.
func legendPlate(bounds image.Rectangle, size image.Point, corner string, margin, credit int) image.Rectangle {
	x := bounds.Min.X + margin
	if corner == "top-right" || corner == "bottom-right" {
		x = bounds.Max.X - margin - size.X
	}
	y := bounds.Min.Y + margin
	switch corner {
	case "bottom-left":
		y = bounds.Max.Y - margin - size.Y
	case "bottom-right":
		y = bounds.Max.Y - margin - credit - size.Y
	}
	return image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x+size.X, y+size.Y)}.Intersect(bounds)
}

// creditSpace is how far up from the bottom edge a legend in the bottom right
// has to stand to clear the map's credit, with margin between them: the
// credit's plate is its text's height and 8 pixels, as render.DrawCredit
// draws it. A map with no credit to show leaves the corner free.
func creditSpace(credit string, face font.Face, margin int) int {
	if render.PlainCredit(credit) == "" {
		return 0
	}
	return face.Metrics().Height.Ceil() + 8 + margin
}

// quietestCorner is the corner where a legend of size would cover least of
// what is drawn in overlay -- a picture of the course, its markers, labels
// and references alone, on nothing: the pixels there are what a legend would
// hide. Ties go to the earlier corner in legendCorners.
func quietestCorner(overlay *image.RGBA, size image.Point, margin, credit int) string {
	best, least := legendCorners[0], -1
	for _, corner := range legendCorners {
		r := legendPlate(overlay.Bounds(), size, corner, margin, credit)
		n := 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if overlay.RGBAAt(x, y).A > 0 {
					n++
				}
			}
		}
		if least < 0 || n < least {
			best, least = corner, n
		}
	}
	return best
}

// drawLegend writes a legend into the picture at plate, on a plate like the
// credit's, so it reads over any map: a short sample of each line, solid or
// dashed in its ink, and its name beside it.
//
// It is there because a dashed line on a map with nothing saying what it is
// invites the wrong reading -- a detour, a gap in the recording -- and with
// several references there is no other way to tell them apart.
func drawLegend(img *image.RGBA, entries []entry, face font.Face, scale float64, plate image.Rectangle) {
	if face == nil || len(entries) == 0 {
		return
	}
	m := metricsFor(face, scale)
	pad, sample, thick, lineH, bar := m.pad, m.sample, m.thick, m.lineH, m.bar
	draw.Draw(img, plate, image.NewUniform(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xdd}), image.Point{}, draw.Over)

	ink := image.NewUniform(color.RGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xff})
	text := func(s string, x, mid int) {
		d := font.Drawer{Dst: img, Src: ink, Face: face,
			Dot: fixed.P(x, mid+(face.Metrics().Ascent.Ceil()-face.Metrics().Descent.Ceil())/2)}
		d.DrawString(render.Visual(s))
	}
	row := 0
	for _, e := range entries {
		top := plate.Min.Y + pad + row*lineH
		mid := top + lineH/2
		x0 := plate.Min.X + pad
		if e.ramp != nil {
			text(e.name, x0, mid)
			mid += lineH
			lowW := font.MeasureString(face, e.ramp.low).Ceil()
			text(e.ramp.low, x0, mid)
			bx := x0 + lowW + pad
			render.DrawScaleBar(img, image.Rect(bx, mid-thick, bx+bar, mid+thick), e.ramp.scale)
			text(e.ramp.high, bx+bar+pad, mid)
			row += 2
			continue
		}
		row++
		dash, gapLen := sample, 0
		if e.dashed {
			dash, gapLen = max(3, sample/4), max(2, sample/7)
		}
		for x := x0; x < x0+sample; x += dash + gapLen {
			end := min(x+dash, x0+sample)
			draw.Draw(img, image.Rect(x, mid-thick/2, end, mid-thick/2+thick), image.NewUniform(e.ink), image.Point{}, draw.Over)
		}
		text(e.name, x0+sample+pad, mid)
	}
}

// activityNames are what the legend calls each file's activity: its title
// if --title gave one, and its file's name otherwise. Merged into one, the
// files are one course with one name, the first file's; drawn separately,
// each has its own. Titles are given once each, in the files' order: fewer
// or more would leave which title is whose to a guess.
func activityNames(files, titles []string, separate bool) ([]string, error) {
	if !separate {
		if len(titles) > 1 {
			return nil, fmt.Errorf("%d titles for one merged course; give one, or --separate to title each file", len(titles))
		}
		if len(titles) == 1 {
			return titles, nil
		}
		return []string{nameOf(files[0])}, nil
	}
	if len(titles) > 0 && len(titles) != len(files) {
		return nil, fmt.Errorf("%d titles for %d activities; give one --title for each, in the order of the files", len(titles), len(files))
	}
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = nameOf(f)
		if len(titles) > 0 {
			names[i] = titles[i]
		}
	}
	return names, nil
}
