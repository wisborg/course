package routemap

import (
	"math"

	"github.com/wisborg/osmbase/mercator"
	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
	"github.com/wisborg/course/match"
)

// beside is points moved, where they run along the course, into a lane
// offset pixels to one side of it, as a transit map draws two lines that
// share a road: side by side, each in its own colour, rather than one over
// the other where only the upper shows.
//
// A point runs along the course when it is within matching's tolerance of
// it, near, or within offset pixels of it on a map zoomed out so far that
// the two would touch anyway -- and is heading along it, one way or the
// other, within parallelAngle: a reference crossing the course is within
// tolerance of it for a few metres too, and moving it sideways there would
// put a jog in a line that only cut across. Such a point is put offset pixels from
// the nearest point of the course, square to the reference's own direction
// there, to its right. Taking the side from the reference rather than the
// course keeps it steady where the course doubles back over itself, as an
// out-and-back does, and a reference that itself goes out and back is drawn
// on both sides of the road, as a bus route is.
//
// Into and out of a shared stretch the lane is eased over rampPixels of the
// reference, so the line is seen leaving the course's side, not jumping.
// Everywhere else the reference is where it was.
func beside(points []render.Coord, along []render.Coord, v render.View, offset float64) []render.Coord {
	if len(points) < 2 || len(along) == 0 || offset <= 0 {
		return points
	}
	px := make([][2]float64, len(points))
	for i, p := range points {
		px[i] = pixel(v, p)
	}
	cpx := make([][2]float64, len(along))
	for i, p := range along {
		cpx[i] = pixel(v, p)
	}

	shared := make([]float64, len(points))
	snap := make([][2]float64, len(points))
	cosMin := math.Cos(parallelAngle * math.Pi / 180)
	for i, p := range px {
		q, dir := nearestOn(cpx, p)
		snap[i] = q
		dpx := math.Hypot(p[0]-q[0], p[1]-q[1])
		at := unpixel(v, q)
		close := dpx <= offset || course.Metres(points[i].Lat, points[i].Lon, at.Lat, at.Lon) <= near
		n := normal(px, i) // square to the reference: its direction turned
		if close && math.Abs(n[1]*dir[0]-n[0]*dir[1]) >= cosMin {
			shared[i] = 1
		}
	}

	// How far along the reference each point is, in pixels, to ease over.
	s := make([]float64, len(px))
	for i := 1; i < len(px); i++ {
		s[i] = s[i-1] + math.Hypot(px[i][0]-px[i-1][0], px[i][1]-px[i-1][1])
	}
	ramp := math.Max(rampPixels, 3*offset)
	weight := eased(shared, s, ramp)

	out := make([]render.Coord, len(points))
	for i := range px {
		if weight[i] == 0 {
			out[i] = points[i]
			continue
		}
		n := normal(px, i)
		lane := [2]float64{snap[i][0] + offset*n[0], snap[i][1] + offset*n[1]}
		w := weight[i]
		out[i] = unpixel(v, [2]float64{px[i][0] + w*(lane[0]-px[i][0]), px[i][1] + w*(lane[1]-px[i][1])})
	}
	return out
}

// near is how close to the course a reference runs along it: matching's
// own tolerance, so a stretch drawn beside the course is one matching would
// call the same road.
var near = match.Options{}.WithDefaults().Near

// parallelAngle is how far, in degrees, a reference's direction may be from
// the course's for it to be running along it rather than across.
const parallelAngle = 35.0

// rampPixels is the shortest the way into or out of a lane is eased over.
const rampPixels = 12.0

// eased is shared, 0 or 1 at each point, smoothed along s: the share of the
// reference within ramp/2 either side of each point that is shared, so a
// lane fades in over ramp pixels where a shared stretch begins and out where
// it ends, and a point shared alone in a long stretch that is not -- a
// crossing, where the reference cuts over the course -- barely moves.
func eased(shared, s []float64, ramp float64) []float64 {
	out := make([]float64, len(shared))
	lo, hi := 0, 0
	var sum, length float64
	// Each point stands for the reference from halfway to its neighbour on
	// one side to halfway on the other.
	share := func(i int) float64 {
		a, b := s[i], s[i]
		if i > 0 {
			a = (s[i-1] + s[i]) / 2
		}
		if i < len(s)-1 {
			b = (s[i] + s[i+1]) / 2
		}
		return b - a
	}
	for i := range shared {
		for hi < len(shared) && s[hi] <= s[i]+ramp/2 {
			sum += shared[hi] * share(hi)
			length += share(hi)
			hi++
		}
		for lo < hi && s[lo] < s[i]-ramp/2 {
			sum -= shared[lo] * share(lo)
			length -= share(lo)
			lo++
		}
		if length > 0 {
			out[i] = math.Max(0, math.Min(1, sum/length))
		}
	}
	// Smoothstep, so the line leaves and joins the lane along a curve.
	for i, w := range out {
		out[i] = w * w * (3 - 2*w)
	}
	return out
}

// normal is the unit vector square to the line px at point i, to its right
// on the screen (y grows down), from its neighbours either side.
func normal(px [][2]float64, i int) [2]float64 {
	a, b := px[max(0, i-1)], px[min(len(px)-1, i+1)]
	dx, dy := b[0]-a[0], b[1]-a[1]
	l := math.Hypot(dx, dy)
	if l == 0 {
		return [2]float64{}
	}
	return [2]float64{-dy / l, dx / l}
}

// nearestOn is the point of the line through pts nearest p, and the unit
// direction of the line there -- zero for a line of one point.
func nearestOn(pts [][2]float64, p [2]float64) (nearest, dir [2]float64) {
	best, bestD := pts[0], math.Inf(1)
	for i := 0; i+1 < len(pts) || i == 0 && len(pts) == 1; i++ {
		a := pts[i]
		b := a
		if i+1 < len(pts) {
			b = pts[i+1]
		}
		q, u := a, [2]float64{}
		if dx, dy := b[0]-a[0], b[1]-a[1]; dx != 0 || dy != 0 {
			t := ((p[0]-a[0])*dx + (p[1]-a[1])*dy) / (dx*dx + dy*dy)
			t = math.Max(0, math.Min(1, t))
			q = [2]float64{a[0] + t*dx, a[1] + t*dy}
			l := math.Hypot(dx, dy)
			u = [2]float64{dx / l, dy / l}
		}
		if d := (p[0]-q[0])*(p[0]-q[0]) + (p[1]-q[1])*(p[1]-q[1]); d < bestD {
			best, dir, bestD = q, u, d
		}
	}
	return best, dir
}

// unpixel is pixel undone: where on the globe the pixel p of view v is.
func unpixel(v render.View, p [2]float64) render.Coord {
	x0, y0 := mercator.Project(v.Bounds.West, v.Bounds.North)
	x1, y1 := mercator.Project(v.Bounds.East, v.Bounds.South)
	lon, lat := mercator.Unproject(x0+p[0]/float64(v.Width)*(x1-x0), y0+p[1]/float64(v.Height)*(y1-y0))
	return render.Coord{Lat: lat, Lon: lon}
}
