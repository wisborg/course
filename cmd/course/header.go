package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	cw "github.com/wisborg/cobrakit/weather"
	"github.com/wisborg/fitactivity/units"
	fw "github.com/wisborg/fitactivity/weather"
	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// header is what a map's header band says, each part only when asked
// for: a title and a subtitle for the whole map, the stats, and the
// weather.
type header struct {
	title, subtitle string
	stats           []statLine
	weather         *headerWeather
	dark            bool
}

// headerWeather is the weather as the header shows it: an icon of the most
// significant weather met, by day or night; the sky in words; the
// temperatures; and the wind and dew point, with the data's credit.
type headerWeather struct {
	hasSky      bool
	sky         fw.Sky
	day         bool
	caption     string
	temperature string
	detail      string
}

func (h header) empty() bool {
	return h.title == "" && h.subtitle == "" && len(h.stats) == 0 && h.weather == nil
}

// mapHeader is the header the flags ask for over c, named name: the
// weather is fetched here, and what was sent, and its credit, said on
// errw, as summary does.
func mapHeader(cmd *cobra.Command, c *course.Course, name string, u units.Set, src statSources, dark bool, errw io.Writer) (header, error) {
	h := header{title: mapOpts.mapTitle, subtitle: mapOpts.mapSubtitle, dark: dark}
	if h.title == "auto" {
		h.title = name
	}
	if len(mapOpts.stats) > 0 {
		var err error
		if h.stats, err = statsOf(mapOpts.stats, c, u, src); err != nil {
			return header{}, err
		}
	}
	if !mapOpts.weather {
		return h, nil
	}
	var p cw.Plan
	if c.Track == nil {
		p = cw.Plan{Asked: true, Reason: "a planned route has no times to ask the weather for"}
	} else {
		var err error
		p, err = cw.Resolve(cmd, cw.Options{Weather: true, Exposure: cw.ExposureSuburban}, c.Track, len(c.Flights) > 0 || c.Sport == "flying")
		if err != nil {
			return header{}, err
		}
	}
	cw.WriteNote(errw, p, c.Track, u, "the map")
	if p.Report == nil {
		return h, nil
	}
	s, ok := fw.Summarise(p.Report, c.Track)
	if !ok {
		return h, nil
	}
	w := cw.WordsOf(s, u)
	hw := &headerWeather{hasSky: s.HasSky, sky: fw.Kind(s.WeatherCode), day: s.Day || !s.HasSky,
		caption: w.Sky, temperature: w.Temperature}
	if hw.caption == "" {
		hw.caption = "weather"
	}
	hw.detail = strings.Join([]string{w.Wind, w.DewPoint, "Open-Meteo.com"}, " · ")
	h.weather = hw
	return h, nil
}

// headerInks are the band's colours, light or dark to suit the map's
// palette.
type headerInks struct{ bg, text, caption, rule color.RGBA }

func inksForHeader(dark bool) headerInks {
	if dark {
		return headerInks{bg: color.RGBA{0x0f, 0x15, 0x1b, 0xff}, text: color.RGBA{0xee, 0xf2, 0xf5, 0xff},
			caption: color.RGBA{0x93, 0xa0, 0xad, 0xff}, rule: color.RGBA{0x24, 0x30, 0x3a, 0xff}}
	}
	return headerInks{bg: color.RGBA{0xff, 0xff, 0xff, 0xff}, text: color.RGBA{0x1a, 0x1a, 0x1a, 0xff},
		caption: color.RGBA{0x6b, 0x72, 0x80, 0xff}, rule: color.RGBA{0xd9, 0xdd, 0xe2, 0xff}}
}

// band is a header laid out to a width: how tall it is, and how to draw
// it at the top of a picture.
type band struct {
	height int
	draw   func(img *image.RGBA)
}

// headerFaces are the band's type sizes, from the map's text size: a
// title twice it, the stats' values half again, and the captions a little
// under it.
type headerFaces struct{ title, subtitle, value, caption font.Face }

func lineHeight(f font.Face) int { return f.Metrics().Height.Ceil() }

func textWidth(f font.Face, s string) int { return font.MeasureString(f, render.Visual(s)).Ceil() }

// tile is a block of the band: its size, and how to draw it at a corner.
type tile struct {
	size image.Point
	draw func(img *image.RGBA, at image.Point)
}

// layout lays h out across width, its text at textSize scaled by scale,
// as a map's text is: the title and subtitle on the left, and the weather
// and the stats on the right, beside them where they fit and in rows
// under them where they do not.
func (h header) layout(width int, textSize, scale float64) band {
	if h.empty() {
		return band{}
	}
	base := textSize * scale
	f := headerFaces{title: faceAt(base * 2.1), subtitle: faceAt(base * 1.2), value: faceAt(base * 1.6), caption: faceAt(base * 0.95)}
	in := inksForHeader(h.dark)
	pad := int(math.Round(base * 1.1))
	gap := int(math.Round(base * 1.8))
	inner := width - 2*pad

	text := func(img *image.RGBA, face font.Face, ink color.RGBA, s string, x, top int) {
		d := font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: face, Dot: fixed.P(x, top+face.Metrics().Ascent.Ceil())}
		d.DrawString(render.Visual(s))
	}

	var left *tile
	if h.title != "" || h.subtitle != "" {
		type textLine struct {
			face font.Face
			ink  color.RGBA
			s    string
		}
		var lines []textLine
		if h.title != "" {
			lines = append(lines, textLine{f.title, in.text, h.title})
		}
		if h.subtitle != "" {
			lines = append(lines, textLine{f.subtitle, in.caption, h.subtitle})
		}
		var size image.Point
		for _, l := range lines {
			size.X = max(size.X, textWidth(l.face, l.s))
			size.Y += lineHeight(l.face)
		}
		left = &tile{size: size, draw: func(img *image.RGBA, at image.Point) {
			y := at.Y
			for _, l := range lines {
				text(img, l.face, l.ink, l.s, at.X, y)
				y += lineHeight(l.face)
			}
		}}
	}

	var weather *tile
	if w := h.weather; w != nil {
		capH, valH := lineHeight(f.caption), lineHeight(f.value)
		tall := capH + valH + capH
		icon := 0
		if w.hasSky {
			icon = tall + int(math.Round(base*0.6))
		}
		textW := max(textWidth(f.caption, w.caption), textWidth(f.value, w.temperature), textWidth(f.caption, w.detail))
		weather = &tile{size: image.Pt(icon+textW, tall), draw: func(img *image.RGBA, at image.Point) {
			if w.hasSky {
				drawSkyIcon(img, image.Rect(at.X, at.Y, at.X+tall, at.Y+tall), w.sky, w.day, h.dark, in.bg)
			}
			x := at.X + icon
			text(img, f.caption, in.caption, w.caption, x, at.Y)
			text(img, f.value, in.text, w.temperature, x, at.Y+capH)
			text(img, f.caption, in.caption, w.detail, x, at.Y+capH+valH)
		}}
	}
	var stats []tile
	for _, s := range h.stats {
		s := s
		capH := lineHeight(f.caption)
		stats = append(stats, tile{size: image.Pt(max(textWidth(f.caption, s.caption), textWidth(f.value, s.value)), capH+lineHeight(f.value)),
			draw: func(img *image.RGBA, at image.Point) {
				text(img, f.caption, in.caption, s.caption, at.X, at.Y)
				text(img, f.value, in.text, s.value, at.X, at.Y+capH)
			}})
	}

	rowOf := func(ts []tile) (w, h int) {
		for i, t := range ts {
			if i > 0 {
				w += gap
			}
			w += t.size.X
			h = max(h, t.size.Y)
		}
		return w, h
	}
	type placed struct {
		t  tile
		at image.Point
	}
	var all []placed
	y := pad
	rowGap := int(math.Round(base * 0.8))
	// row lays ts in a row from x, their middles level, and moves y past
	// it.
	row := func(ts []tile, x int) {
		_, rh := rowOf(ts)
		for _, t := range ts {
			all = append(all, placed{t, image.Pt(x, y+(rh-t.size.Y)/2)})
			x += t.size.X + gap
		}
		y += rh + rowGap
	}
	// beside lays the title on the left and ts on the right, if they fit
	// on one row.
	beside := func(ts []tile) bool {
		w, _ := rowOf(ts)
		if left == nil || len(ts) == 0 || left.size.X+gap+w > inner {
			return false
		}
		row(append([]tile{*left}, ts...), pad)
		// The title stays at the left edge and the rest goes to the right.
		at := width - pad - w
		for i := len(all) - len(ts); i < len(all); i++ {
			all[i].at.X = at
			at += all[i].t.size.X + gap
		}
		return true
	}
	var rest []tile
	if weather != nil {
		rest = append(rest, *weather)
	}
	rest = append(rest, stats...)
	switch {
	case left == nil && func() bool { w, _ := rowOf(rest); return w <= inner }():
		row(rest, pad)
	case beside(rest):
	default:
		// The title and the weather on a row where they fit, and the
		// stats in rows under them, as many to a row as fit across.
		if weather == nil || !beside([]tile{*weather}) {
			if left != nil {
				row([]tile{*left}, pad)
			}
			if weather != nil {
				row([]tile{*weather}, pad)
			}
		}
		var r []tile
		for _, t := range stats {
			if w, _ := rowOf(append(append([]tile{}, r...), t)); w > inner && len(r) > 0 {
				row(r, pad)
				r = nil
			}
			r = append(r, t)
		}
		if len(r) > 0 {
			row(r, pad)
		}
	}
	y -= rowGap
	height := y + pad
	return band{height: height, draw: func(img *image.RGBA) {
		r := image.Rect(img.Bounds().Min.X, img.Bounds().Min.Y, img.Bounds().Max.X, img.Bounds().Min.Y+height)
		draw.Draw(img, r, image.NewUniform(in.bg), image.Point{}, draw.Src)
		rule := max(1, int(math.Round(scale)))
		draw.Draw(img, image.Rect(r.Min.X, r.Max.Y-rule, r.Max.X, r.Max.Y), image.NewUniform(in.rule), image.Point{}, draw.Src)
		for _, p := range all {
			p.t.draw(img, r.Min.Add(p.at))
		}
	}}
}

// withHeader is the map img under the band b, as one picture.
func withHeader(img *image.RGBA, b band) *image.RGBA {
	if b.height == 0 {
		return img
	}
	size := img.Bounds().Size()
	out := image.NewRGBA(image.Rect(0, 0, size.X, size.Y+b.height))
	b.draw(out)
	draw.Draw(out, image.Rect(0, b.height, size.X, b.height+size.Y), img, img.Bounds().Min, draw.Src)
	return out
}

// headerRoom checks that a band of height leaves the map below it room:
// at least half the picture.
func headerRoom(height, picture int) error {
	if height > picture/2 {
		return fmt.Errorf("the header is %d pixels of a %d-pixel picture, leaving too little for the map; "+
			"ask for fewer --stats, or a larger --height", height, picture)
	}
	return nil
}
