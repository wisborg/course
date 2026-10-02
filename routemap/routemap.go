// Package routemap decides what of a course is drawn over a map, and how:
// which points make the line, where it is dashed for want of data, where the
// start, the finish and the distance markers go. osmbase's render.Draw puts
// the result on the picture; nothing here draws a pixel.
package routemap

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"time"

	"github.com/wisborg/osmbase/mercator"
	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// Inks are the colours a course is drawn in. Halo is the band round every
// line and marker, and is usually the map's own background, so the course
// reads over the map's texture rather than merging into it.
type Inks struct {
	Route, Gap, Start, Finish, Marker, Halo color.RGBA
}

// InksFor are the inks for a palette: its overlay's, which osmbase checks for
// contrast against that palette.
func InksFor(p render.Palette, o render.Overlay) Inks {
	return Inks{Route: o.Foreground, Gap: o.Dim, Start: o.Accent, Finish: o.Highlight, Marker: o.Dim, Halo: p.Background}
}

// sizes are how big the markers are drawn and the halo round them: 1 is
// sized for a picture about a thousand pixels across. A line's size is its
// Look's. line is the width lines were drawn at before they had a Look, and
// sizes the departure padding still measured against.
func sizes(scale float64) (line, halo, dot, small float64) {
	return 4 * scale, 2 * scale, 6 * scale, 3.5 * scale
}

// Drawing is the course as lines and markers over a picture of v, its line
// drawn in look and its distance markers every metres apart, as
// DistanceMarkersEvery takes it. A gap in the recording is drawn as wide and as opaque, but
// always dashed and in the gap's ink, so it is never taken for where the
// course went.
func Drawing(c *course.Course, v render.View, inks Inks, look Look, every, scale float64) render.Drawing {
	if scale <= 0 {
		scale = 1
	}
	_, halo, dot, small := sizes(scale)
	var d render.Drawing
	if len(c.Points) == 0 {
		return d
	}
	for _, run := range runs(c, v) {
		if run.gap {
			l := look.line(run.points, inks.Gap, inks.Halo, scale)
			l.Dash = []float32{float32(3 * l.Width), float32(2 * l.Width)}
			d.Lines = append(d.Lines, l)
			continue
		}
		d.Lines = append(d.Lines, look.line(run.points, inks.Route, inks.Halo, scale))
	}

	for _, m := range DistanceMarkersEvery(c, every) {
		d.Markers = append(d.Markers, render.Marker{At: m.At, Ink: inks.Marker, Radius: small, Halo: halo, HaloInk: inks.Halo, Label: m.Label})
	}
	first, last := c.Points[0], c.Points[len(c.Points)-1]
	start := render.Marker{At: render.Coord{Lat: first.Lat, Lon: first.Lon}, Ink: inks.Start, Radius: dot, Halo: halo, HaloInk: inks.Halo, Label: "Start"}
	finish := render.Marker{At: render.Coord{Lat: last.Lat, Lon: last.Lon}, Ink: inks.Finish, Radius: dot, Halo: halo, HaloInk: inks.Halo, Label: "Finish"}
	// A loop ends where it started, and two labels on one dot are one label
	// written over the other.
	// The finish dot goes down first and without a label, so the start's
	// dot is on top and carries both.
	if pixelDistance(v, start.At, finish.At) < 3*dot {
		finish.Label, start.Label = "", "Start and finish"
	}
	d.Markers = append(d.Markers, finish, start)
	return d
}

// run is a stretch of the line drawn one way: recorded, or a gap across
// which nothing was.
type run struct {
	points []render.Coord
	gap    bool
}

// minPixel is how far apart, in pixels, the points kept for the line are. A
// marathon is sixteen thousand fixes; at the scale of a picture most of them
// are the same pixel, and a line through them is the same line.
const minPixel = 1.0

// runs are the course's line in stretches: a recording is broken where it has
// a gap, and the gap is its own stretch, dashed.
//
// A gap is two fixes further apart in time than the recording's own rhythm
// -- five times its usual interval, and at least a minute -- and in space
// than a few hundred metres. A tunnel, a watch that lost the satellites, a
// flight over an ocean no receiver hears: the straight line between the two
// fixes is not where the course went, only that it got from one to the
// other, and drawing it solid would say more than the file does. A plan has
// no clock and no gaps.
func runs(c *course.Course, v render.View) []run {
	gapAfter := gapThreshold(c)
	var out []run
	cur := run{}
	var lastPx [2]float64
	flush := func() {
		if len(cur.points) >= 2 {
			out = append(out, cur)
		}
	}
	for i, p := range c.Points {
		at := render.Coord{Lat: p.Lat, Lon: p.Lon}
		px := pixel(v, at)
		if i > 0 && gapAfter > 0 {
			prev := c.Points[i-1]
			if p.Elapsed-prev.Elapsed > gapAfter && course.Metres(prev.Lat, prev.Lon, p.Lat, p.Lon) > 300 {
				prevAt := render.Coord{Lat: prev.Lat, Lon: prev.Lon}
				if n := len(cur.points); n == 0 || cur.points[n-1] != prevAt {
					cur.points = append(cur.points, prevAt)
				}
				flush()
				out = append(out, run{points: []render.Coord{prevAt, at}, gap: true})
				cur = run{points: []render.Coord{at}}
				lastPx = px
				continue
			}
		}
		last := i == len(c.Points)-1
		if len(cur.points) == 0 || last || math.Hypot(px[0]-lastPx[0], px[1]-lastPx[1]) >= minPixel {
			cur.points = append(cur.points, at)
			lastPx = px
		}
	}
	flush()
	return out
}

// gapThreshold is the pause between two fixes that is a gap in the
// recording, or 0 for a course with no clock.
func gapThreshold(c *course.Course) time.Duration {
	if !c.Timed || len(c.Points) < 3 {
		return 0
	}
	steps := make([]time.Duration, 0, len(c.Points)-1)
	for i := 1; i < len(c.Points); i++ {
		steps = append(steps, c.Points[i].Elapsed-c.Points[i-1].Elapsed)
	}
	sort.Slice(steps, func(a, b int) bool { return steps[a] < steps[b] })
	return max(5*steps[len(steps)/2], time.Minute)
}

func pixel(v render.View, at render.Coord) [2]float64 {
	x0, y0 := mercator.Project(v.Bounds.West, v.Bounds.North)
	x1, y1 := mercator.Project(v.Bounds.East, v.Bounds.South)
	x, y := mercator.Project(at.Lon, at.Lat)
	return [2]float64{(x - x0) / (x1 - x0) * float64(v.Width), (y - y0) / (y1 - y0) * float64(v.Height)}
}

func pixelDistance(v render.View, a, b render.Coord) float64 {
	pa, pb := pixel(v, a), pixel(v, b)
	return math.Hypot(pa[0]-pb[0], pa[1]-pb[1])
}

// Marker is a distance marker: where it goes and what it says.
type Marker struct {
	At    render.Coord
	Label string
}

// MarkerInterval is how far apart distance markers go on a course of total
// metres, or 0 for none.
//
// The table the Python original settled on, which reads well on a map a
// thousand pixels across: none on a course too short for one, every
// kilometre to a half marathon's worth less a little, then wider so a
// marathon is ten markers rather than forty.
func MarkerInterval(total float64) float64 {
	km := total / 1000
	switch {
	case km < 1.2:
		return 0
	case km < 15:
		return 1000
	case km < 21:
		return 2000
	case km < 25:
		return 3000
	case km < 41:
		return 4000
	case km < 50:
		return 5000
	}
	return 10000
}

// DistanceMarkers are the markers along a course whose every point has a
// recorded distance, at MarkerInterval apart, each placed between the two
// fixes either side of its distance in proportion. A course without recorded
// distance has none: they would be computed from the line, and the file did
// not say how far anybody went.
func DistanceMarkers(c *course.Course) []Marker { return DistanceMarkersEvery(c, 0) }

// DistanceMarkersEvery is DistanceMarkers every metres apart: 0 for
// MarkerInterval's, and less than 0 for none.
func DistanceMarkersEvery(c *course.Course, every float64) []Marker {
	if len(c.Points) < 2 || every < 0 {
		return nil
	}
	for _, p := range c.Points {
		if !p.HasDistance {
			return nil
		}
	}
	first, last := c.Points[0], c.Points[len(c.Points)-1]
	if every == 0 {
		every = MarkerInterval(last.Distance - first.Distance)
	}
	if every == 0 {
		return nil
	}
	var out []Marker
	next := first.Distance + every
	for i := 1; i < len(c.Points); i++ {
		a, b := c.Points[i-1], c.Points[i]
		for next <= b.Distance && next < last.Distance {
			if next <= a.Distance || b.Distance == a.Distance {
				next += every
				continue
			}
			f := (next - a.Distance) / (b.Distance - a.Distance)
			out = append(out, Marker{
				At:    render.Coord{Lat: a.Lat + f*(b.Lat-a.Lat), Lon: a.Lon + f*(b.Lon-a.Lon)},
				Label: fmt.Sprintf("%.0f", (next-first.Distance)/1000),
			})
			next += every
		}
	}
	return out
}
