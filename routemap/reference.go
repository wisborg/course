package routemap

import (
	"image/color"
	"math"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
	"github.com/wisborg/course/match"
)

// Reference is a line to compare a course against: another course, or the
// great circle between its ends.
type Reference struct {
	Name   string
	Points []render.Coord
	// Follows says the course followed the reference, and then only Apart
	// is drawn: the stretches of it where the two part. Where they
	// coincide a second line says nothing and hides the map beneath; where
	// they part, the difference stands alone. A reference the course
	// followed all the way has no Apart, and nothing of it is drawn.
	Follows bool
	Apart   [][]render.Coord
}

// Drawn is what of the reference is drawn: all of it, or where it and the
// course part.
func (r Reference) Drawn() [][]render.Coord {
	if r.Follows {
		return r.Apart
	}
	return [][]render.Coord{r.Points}
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

// ReferenceLines are the references as lines, each in its ink and look
// from inks and looks in turn, to go under the course: first in a Drawing's
// lines, so the course is drawn over them.
func ReferenceLines(refs []Reference, inks []color.RGBA, looks []Look, halo color.RGBA, scale float64) []render.Line {
	if scale <= 0 {
		scale = 1
	}
	var out []render.Line
	for i, r := range refs {
		for _, piece := range r.Drawn() {
			if len(piece) < 2 {
				continue
			}
			out = append(out, looks[i%len(looks)].line(piece, inks[i%len(inks)], halo, scale))
		}
	}
	return out
}

// thinned is how much of its usual width the course's line is drawn at when
// references are drawn whole under it: enough narrower that a reference
// following it shows along both its edges, rather than lying hidden beneath
// it.
const thinned = 0.55

// laneGap is the room between two lines drawn side by side, in pixels on a
// map 1000 across: enough of the map between them to tell them apart.
const laneGap = 1.0

// WithReferences is a course's drawing with references under it, the
// references first, so the course is drawn over them. A reference drawn
// only where it parts from the course needs no room beside it. One drawn
// whole is drawn beside the course where the two share a road, if its look
// says Beside: each such reference in a lane of its own to the right of its
// direction, the second outside the first, as a transit map draws lines
// that share a street -- along, the course's own points, is what they are
// measured from, on view v.
//
// All to the right, each at its own distance, rather than alternating
// sides, for an out-and-back: a reference's way back is on the other side
// of the road from its way out, and with two references on alternate sides
// one's way back fell in the same lane as the other's way out, the two
// dashed lines drawn through each other. At distances of their own no two
// lanes coincide, whichever way each runs. A
// reference drawn whole and not beside narrows the course's lines instead,
// with their halos, so it shows along the course's edges. With nothing to
// draw the drawing is as it was.
func WithReferences(d render.Drawing, refs []Reference, inks []color.RGBA, looks []Look, halo color.RGBA, scale float64, v render.View, along []render.Coord) render.Drawing {
	if scale <= 0 {
		scale = 1
	}
	half := 0.0 // the course's half width, halo and all
	for _, l := range d.Lines {
		half = math.Max(half, l.Width/2+l.Halo)
	}
	under := false // a reference drawn whole, and not beside
	reach := half
	moved := make([]Reference, len(refs))
	for i, r := range refs {
		moved[i] = r
		if r.Follows {
			continue
		}
		look := looks[i%len(looks)]
		if !look.Beside {
			under = true
			continue
		}
		w := look.Width*scale/2 + look.Halo*scale
		offset := reach + laneGap*scale + w
		reach = offset + w
		moved[i].Points = beside(r.Points, along, v, offset)
	}
	lines := ReferenceLines(moved, inks, looks, halo, scale)
	if len(lines) == 0 {
		return d
	}
	if !under {
		d.Lines = append(lines, d.Lines...)
		return d
	}
	narrow := make([]render.Line, len(d.Lines))
	for i, l := range d.Lines {
		l.Width *= thinned
		l.Halo *= thinned
		narrow[i] = l
	}
	d.Lines = append(lines, narrow...)
	return d
}

// Followed is a reference the course was matched to, as m found it, drawn
// only where the two part: the stretches of it the course missed -- a
// closed bridge, a road-works detour, the part of the course a watch
// started late never saw -- each reaching departurePad further at both
// ends, so the dashed line is seen leaving the course and coming back to
// it rather than hanging beside it unconnected.
func Followed(name string, ref *course.Course, m match.Match) Reference {
	r := FromCourse(name, ref)
	r.Follows = true
	s := match.Resample(ref, departureStep)
	var from, to []float64
	for _, st := range m.Missed {
		f, t := st.From-departurePad, st.To+departurePad
		if n := len(to); n > 0 && f <= to[n-1] {
			to[n-1] = t // padded, the two touch: one stretch
			continue
		}
		from, to = append(from, f), append(to, t)
	}
	for i := range from {
		var piece []render.Coord
		for _, p := range s {
			if p.Along >= from[i] && p.Along <= to[i] {
				piece = append(piece, render.Coord{Lat: p.Lat, Lon: p.Lon})
			}
		}
		r.Apart = append(r.Apart, piece)
	}
	return r
}

// departureStep is how finely a departure is drawn, in metres: the spacing
// matching measures at.
const departureStep = 10.0

// departurePad is how far past where the course and a reference part a
// departure is drawn, in metres: a little more than matching's tolerance, so
// the line starts on the course.
const departurePad = 30.0
