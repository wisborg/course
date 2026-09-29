package routemap

import (
	"image/color"
	"math"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// Reference is a line to compare a course against: another course, or the
// great circle between its ends.
type Reference struct {
	Name   string
	Points []render.Coord
}

// FromCourse is a course's line as a reference, named name.
func FromCourse(name string, c *course.Course) Reference {
	r := Reference{Name: name}
	for _, p := range c.Points {
		r.Points = append(r.Points, render.Coord{Lat: p.Lat, Lon: p.Lon})
	}
	return r
}

// GreatCircle is the shortest way over the globe between a course's start and
// its finish, as a reference: what a flight would have flown with nothing in
// the way.
//
// A point every greatCircleStep metres along it, so it curves on the map as
// the great circle does -- a straight line in the projection is not the
// shortest way anywhere but along the equator and the meridians. A course
// that ends where it started has no great circle, and none is returned.
func GreatCircle(c *course.Course) (Reference, bool) {
	if len(c.Points) < 2 {
		return Reference{}, false
	}
	a, b := c.Points[0], c.Points[len(c.Points)-1]
	d := course.Metres(a.Lat, a.Lon, b.Lat, b.Lon)
	if d < 1000 {
		return Reference{}, false
	}
	n := max(2, min(1000, int(d/greatCircleStep)+1))
	r := Reference{Name: "Great circle"}
	for i := 0; i < n; i++ {
		lat, lon := slerp(a.Lat, a.Lon, b.Lat, b.Lon, float64(i)/float64(n-1))
		r.Points = append(r.Points, render.Coord{Lat: lat, Lon: lon})
	}
	return r, true
}

// greatCircleStep is the spacing of a great circle's points, in metres.
const greatCircleStep = 20_000.0

// slerp is the point a fraction t of the way along the great circle from one
// position to another: the two as unit vectors, turned between.
func slerp(lat1, lon1, lat2, lon2, t float64) (float64, float64) {
	const rad = math.Pi / 180
	vec := func(lat, lon float64) [3]float64 {
		return [3]float64{math.Cos(lat*rad) * math.Cos(lon*rad), math.Cos(lat*rad) * math.Sin(lon*rad), math.Sin(lat * rad)}
	}
	p, q := vec(lat1, lon1), vec(lat2, lon2)
	dot := p[0]*q[0] + p[1]*q[1] + p[2]*q[2]
	omega := math.Acos(math.Max(-1, math.Min(1, dot)))
	if omega < 1e-12 {
		return lat1, lon1
	}
	s := math.Sin(omega)
	wa, wb := math.Sin((1-t)*omega)/s, math.Sin(t*omega)/s
	v := [3]float64{wa*p[0] + wb*q[0], wa*p[1] + wb*q[1], wa*p[2] + wb*q[2]}
	return math.Atan2(v[2], math.Hypot(v[0], v[1])) / rad, math.Atan2(v[1], v[0]) / rad
}

// ReferenceInks are the inks references are drawn in, one per reference in
// turn, for a palette: dark for a light map and light for a dark one. None is
// any of a course's own inks -- near-black route, red start, indigo finish,
// grey gaps -- so a reference is never taken for part of the course, and each
// is checked against the palette's land, water and background.
func ReferenceInks(p render.Palette) []color.RGBA {
	light := []color.RGBA{
		{R: 0xC2, G: 0x18, B: 0x5B, A: 0xff}, // deep pink
		{R: 0xB3, G: 0x42, B: 0x00, A: 0xff}, // burnt orange
		{R: 0x00, G: 0x69, B: 0x5C, A: 0xff}, // teal
		{R: 0x5D, G: 0x40, B: 0x37, A: 0xff}, // brown
	}
	dark := []color.RGBA{
		{R: 0xF4, G: 0x8F, B: 0xB1, A: 0xff},
		{R: 0xFF, G: 0xB7, B: 0x4D, A: 0xff},
		{R: 0x4D, G: 0xD0, B: 0xC0, A: 0xff},
		{R: 0xD7, G: 0xCC, B: 0xC8, A: 0xff},
	}
	if luminance(p.Land) < 0.5 {
		return dark
	}
	return light
}

func luminance(c color.RGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// ReferenceLines are the references as dashed lines, each in its ink, to go
// under the course: first in a Drawing's lines, so the course is drawn over
// them. Dashes longer than a gap's, and a little thinner than the course, so
// neither is taken for the other.
func ReferenceLines(refs []Reference, inks []color.RGBA, halo color.RGBA, scale float64) []render.Line {
	if scale <= 0 {
		scale = 1
	}
	line, h, _, _ := sizes(scale)
	width := 0.75 * line
	var out []render.Line
	for i, r := range refs {
		if len(r.Points) < 2 {
			continue
		}
		out = append(out, render.Line{
			Points: r.Points, Ink: inks[i%len(inks)], Width: width, Halo: h, HaloInk: halo,
			Dash: []float32{float32(5 * width), float32(3 * width)},
		})
	}
	return out
}

// thinned is how much of its usual width the course's line is drawn at when
// references are drawn under it: enough narrower that a reference following
// it shows along both its edges, rather than lying hidden beneath it.
const thinned = 0.55

// WithReferences is a course's drawing with references under it: the
// references first, so the course is drawn over them, and the course's lines
// narrowed, with their halos, so a reference that follows the course closely
// shows along its edges. With no references the drawing is as it was.
func WithReferences(d render.Drawing, refs []Reference, inks []color.RGBA, halo color.RGBA, scale float64) render.Drawing {
	lines := ReferenceLines(refs, inks, halo, scale)
	if len(lines) == 0 {
		return d
	}
	course := make([]render.Line, len(d.Lines))
	for i, l := range d.Lines {
		l.Width *= thinned
		l.Halo *= thinned
		course[i] = l
	}
	d.Lines = append(lines, course...)
	return d
}
