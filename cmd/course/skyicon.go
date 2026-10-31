package main

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"

	fw "github.com/wisborg/fitactivity/weather"
)

// The icons are drawn rather than read from a set, so there is no
// licence to carry with them: a sun, a moon, a cloud, and what falls from
// it, in shapes made of circles and lines on a square of side size.

// iconInks are the colours an icon is drawn in, for a light header and a
// dark one.
type iconInks struct {
	sun, moon, cloud, darkCloud, rain, snow, bolt, fog color.RGBA
}

func inksForIcon(dark bool) iconInks {
	i := iconInks{
		sun:       color.RGBA{0xf5, 0xa6, 0x23, 0xff},
		moon:      color.RGBA{0xc9, 0xb8, 0x5a, 0xff},
		cloud:     color.RGBA{0xa7, 0xb1, 0xbc, 0xff},
		darkCloud: color.RGBA{0x6e, 0x78, 0x84, 0xff},
		rain:      color.RGBA{0x2f, 0x7f, 0xc8, 0xff},
		snow:      color.RGBA{0x7a, 0xb6, 0xe8, 0xff},
		bolt:      color.RGBA{0xf2, 0xc2, 0x0b, 0xff},
		fog:       color.RGBA{0x99, 0xa2, 0xab, 0xff},
	}
	if dark {
		i.moon = color.RGBA{0xe8, 0xdc, 0x9a, 0xff}
		i.cloud = color.RGBA{0xc4, 0xcc, 0xd4, 0xff}
		i.darkCloud = color.RGBA{0x8a, 0x94, 0x9f, 0xff}
	}
	return i
}

// drawSkyIcon draws sky's icon into img in the square at, by day or by night,
// over a background of bg, which a moon's shadow is cut from.
func drawSkyIcon(img draw.Image, at image.Rectangle, sky fw.Sky, day, dark bool, bg color.RGBA) {
	in := inksForIcon(dark)
	p := &iconPainter{img: img, at: at, size: float64(min(at.Dx(), at.Dy()))}
	light := func(x, y, r float64) {
		if day {
			p.sun(x, y, r, in.sun)
		} else {
			p.moon(x, y, r, in.moon, bg)
		}
	}
	switch sky {
	case fw.Clear:
		light(0.5, 0.5, 0.3)
	case fw.PartlyCloudy:
		light(0.38, 0.36, 0.22)
		p.cloud(0.08, in.cloud)
	case fw.Overcast:
		p.cloud(0, in.cloud)
	case fw.Fog:
		p.cloud(-0.1, in.cloud)
		for i, y := range []float64{0.74, 0.84} {
			p.bar(0.2+0.06*float64(i), y, 0.8-0.06*float64(1-i), 0.035, in.fog)
		}
	case fw.Drizzle:
		p.cloud(-0.12, in.cloud)
		for _, x := range []float64{0.35, 0.5, 0.65} {
			p.disc(x, 0.8, 0.03, in.rain)
		}
	case fw.Rain:
		p.cloud(-0.12, in.cloud)
		for _, x := range []float64{0.36, 0.52, 0.68} {
			p.slant(x, 0.72, 0.16, 0.035, in.rain)
		}
	case fw.Snow:
		p.cloud(-0.12, in.cloud)
		for i, x := range []float64{0.34, 0.5, 0.66} {
			p.flake(x, 0.78+0.05*float64(i%2), 0.06, in.snow)
		}
	case fw.Thunder:
		p.cloud(-0.12, in.darkCloud)
		p.boltShape(in.bolt)
	}
}

// iconPainter draws shapes in the icon's own coordinates: 0 to 1 across
// and down its square.
type iconPainter struct {
	img  draw.Image
	at   image.Rectangle
	size float64
}

func (p *iconPainter) fill(ink color.RGBA, path func(r *vector.Rasterizer, s float64)) {
	w, h := p.at.Dx(), p.at.Dy()
	r := vector.NewRasterizer(w, h)
	path(r, p.size)
	r.Draw(p.img, p.at, image.NewUniform(ink), image.Point{})
}

// circle adds a circle at x, y of radius rad, as four cubic arcs.
func circle(r *vector.Rasterizer, s, x, y, rad float64) {
	const k = 0.5522847498
	cx, cy, R := float32(x*s), float32(y*s), float32(rad*s)
	K := float32(k) * R
	r.MoveTo(cx+R, cy)
	r.CubeTo(cx+R, cy+K, cx+K, cy+R, cx, cy+R)
	r.CubeTo(cx-K, cy+R, cx-R, cy+K, cx-R, cy)
	r.CubeTo(cx-R, cy-K, cx-K, cy-R, cx, cy-R)
	r.CubeTo(cx+K, cy-R, cx+R, cy-K, cx+R, cy)
	r.ClosePath()
}

// quad adds a four-cornered shape.
func quad(r *vector.Rasterizer, s float64, pts [4][2]float64) {
	r.MoveTo(float32(pts[0][0]*s), float32(pts[0][1]*s))
	for _, q := range pts[1:] {
		r.LineTo(float32(q[0]*s), float32(q[1]*s))
	}
	r.ClosePath()
}

// stroke adds a straight stroke of width w from one point to another.
func stroke(r *vector.Rasterizer, s, x0, y0, x1, y1, w float64) {
	dx, dy := x1-x0, y1-y0
	l := math.Hypot(dx, dy)
	nx, ny := -dy/l*w/2, dx/l*w/2
	quad(r, s, [4][2]float64{{x0 + nx, y0 + ny}, {x1 + nx, y1 + ny}, {x1 - nx, y1 - ny}, {x0 - nx, y0 - ny}})
}

func (p *iconPainter) disc(x, y, rad float64, ink color.RGBA) {
	p.fill(ink, func(r *vector.Rasterizer, s float64) { circle(r, s, x, y, rad) })
}

// sun is a disc with eight rays round it.
func (p *iconPainter) sun(x, y, rad float64, ink color.RGBA) {
	p.fill(ink, func(r *vector.Rasterizer, s float64) {
		circle(r, s, x, y, rad*0.62)
		for i := 0; i < 8; i++ {
			a := float64(i) * math.Pi / 4
			c, sn := math.Cos(a), math.Sin(a)
			stroke(r, s, x+c*rad*0.82, y+sn*rad*0.82, x+c*rad*1.12, y+sn*rad*1.12, rad*0.16)
		}
	})
}

// moon is a crescent: a disc with a disc of the background over it.
func (p *iconPainter) moon(x, y, rad float64, ink, bg color.RGBA) {
	p.disc(x, y, rad*0.8, ink)
	p.disc(x+rad*0.38, y-rad*0.3, rad*0.68, bg)
}

// cloud is three puffs on a flat base, lifted by dy.
func (p *iconPainter) cloud(dy float64, ink color.RGBA) {
	p.fill(ink, func(r *vector.Rasterizer, s float64) {
		circle(r, s, 0.34, 0.6+dy, 0.15)
		circle(r, s, 0.52, 0.5+dy, 0.2)
		circle(r, s, 0.7, 0.6+dy, 0.14)
		quad(r, s, [4][2]float64{{0.34, 0.6 + dy}, {0.7, 0.6 + dy}, {0.7, 0.74 + dy}, {0.34, 0.74 + dy}})
	})
}

// bar is a level stroke, as fog.
func (p *iconPainter) bar(x0, y, x1, w float64, ink color.RGBA) {
	p.fill(ink, func(r *vector.Rasterizer, s float64) { stroke(r, s, x0, y, x1, y, w) })
}

// slant is a falling drop's streak, leaning with the wind.
func (p *iconPainter) slant(x, y, l, w float64, ink color.RGBA) {
	p.fill(ink, func(r *vector.Rasterizer, s float64) { stroke(r, s, x, y, x-l*0.35, y+l, w) })
}

// flake is a six-armed star.
func (p *iconPainter) flake(x, y, rad float64, ink color.RGBA) {
	p.fill(ink, func(r *vector.Rasterizer, s float64) {
		for i := 0; i < 3; i++ {
			a := float64(i) * math.Pi / 3
			c, sn := math.Cos(a)*rad, math.Sin(a)*rad
			stroke(r, s, x-c, y-sn, x+c, y+sn, rad*0.35)
		}
	})
}

// boltShape is a lightning bolt under the cloud.
func (p *iconPainter) boltShape(ink color.RGBA) {
	p.fill(ink, func(r *vector.Rasterizer, s float64) {
		pts := [][2]float64{{0.55, 0.58}, {0.42, 0.78}, {0.51, 0.78}, {0.45, 0.95}, {0.62, 0.72}, {0.53, 0.72}, {0.6, 0.58}}
		r.MoveTo(float32(pts[0][0]*s), float32(pts[0][1]*s))
		for _, q := range pts[1:] {
			r.LineTo(float32(q[0]*s), float32(q[1]*s))
		}
		r.ClosePath()
	})
}
