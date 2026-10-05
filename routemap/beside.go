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
	return besideLanes(points, []lane{{along: along, offset: offset}}, v)
}

// lane is a line a reference may be drawn beside, and how far from it: the
// course, at the reference's own lane distance, or a reference already
// placed, at the distance between the two references' lanes, measured from
// where that reference is drawn.
type lane struct {
	along  []render.Coord
	offset float64
	// way is which way along the line a reference has to be heading to be
	// drawn beside it: 0 either, 1 the same way only, -1 the opposite way
	// only. See besideLanes.
	way int
}

// besideLanes is beside measured from the course and, away from it, from
// the references drawn before this one.
//
// That is what keeps two references that leave the course together apart.
// Measured from the course alone, both were left where they were the moment
// they left it, and drawn through each other along whatever road they took
// instead. Now, where this reference runs along an earlier one and not the
// course, it is drawn beside that one, at the distance between the two
// lanes and from where the earlier one is DRAWN -- which is where its course
// lane would have put it, for as long as the earlier one is in its own.
//
// Along the course it keeps its course lane, even where it runs along an
// earlier reference too. Choosing per point between the two -- whichever
// matched first -- drew the later line as a run of hooks wherever both
// matched: the earlier reference's drawn line carries its own GPS jitter, so
// the two positions differ by a pixel or two, and the line flicked between
// them. Instead the course lane holds along the course, and where the two
// leave it together the position eases from the course lane to beside the
// earlier reference over the same distance the lanes ease in and out by,
// while the earlier one eases back to its road. Neither jumps.
//
// Beside an earlier reference heading the same way first, then the other
// way. An out-and-back reference is drawn in lanes on both sides of the
// course, its way out on one and its way back on the other, and a later one
// near both must follow the lane heading its own way, or it is drawn as a
// zigzag between them.
func besideLanes(points []render.Coord, lanes []lane, v render.View) []render.Coord {
	if len(points) < 2 || len(lanes) == 0 || lanes[0].offset <= 0 {
		return points
	}
	px := make([][2]float64, len(points))
	for i, p := range points {
		px[i] = pixel(v, p)
	}
	lpx := make([][][2]float64, len(lanes))
	for k, l := range lanes {
		lpx[k] = make([][2]float64, len(l.along))
		for i, p := range l.along {
			lpx[k][i] = pixel(v, p)
		}
	}
	cosMin := math.Cos(parallelAngle * math.Pi / 180)

	// alongLane reports whether point i runs along lane k.
	alongLane := func(i, k int) bool {
		l := lanes[k]
		if len(lpx[k]) == 0 || l.offset <= 0 {
			return false
		}
		n := normal(px, i)
		q, dir, ok := nearestOnWay(lpx[k], px[i], n, l.way, cosMin)
		if !ok {
			return false
		}
		dpx := math.Hypot(px[i][0]-q[0], px[i][1]-q[1])
		at := unpixel(v, q)
		close := dpx <= l.offset || course.Metres(points[i].Lat, points[i].Lon, at.Lat, at.Lon) <= near
		return close && math.Abs(n[1]*dir[0]-n[0]*dir[1]) >= cosMin
	}

	// On the course, lane 0; else beside the first other lane it runs
	// along, if any.
	onCourse := make([]float64, len(points))
	shared := make([]float64, len(points))
	other := make([]float64, len(points))
	laneOf := make([]int, len(points))
	for i := range px {
		if alongLane(i, 0) {
			onCourse[i], shared[i] = 1, 1
			continue
		}
		for k := 1; k < len(lanes); k++ {
			if alongLane(i, k) {
				other[i], shared[i], laneOf[i] = 1, 1, k
				break
			}
		}
	}

	// How far along the reference each point is, in pixels, to ease over.
	s := make([]float64, len(px))
	for i := 1; i < len(px); i++ {
		s[i] = s[i-1] + math.Hypot(px[i][0]-px[i-1][0], px[i][1]-px[i-1][1])
	}
	ramp := math.Max(rampPixels, 3*lanes[0].offset)
	weight := eased(shared, s, ramp)
	byCourse := eased(onCourse, s, ramp)
	anyOther := false
	for _, o := range other {
		anyOther = anyOther || o == 1
	}

	out := make([]render.Coord, len(points))
	for i := range px {
		if weight[i] == 0 {
			out[i] = points[i]
			continue
		}
		n := normal(px, i)
		q, _ := nearestOn(lpx[0], px[i])
		target := [2]float64{q[0] + lanes[0].offset*n[0], q[1] + lanes[0].offset*n[1]}
		if c := byCourse[i]; anyOther && c < 1 {
			// Beside the earlier reference this stretch runs along, or
			// the nearest stretch that does, eased against the course
			// lane where the two meet.
			k := laneOf[i]
			if other[i] == 0 {
				k = laneOf[nearestShared(other, s, i)]
			}
			if k > 0 {
				r, _, ok := nearestOnWay(lpx[k], px[i], n, lanes[k].way, cosMin)
				if !ok || math.Hypot(r[0]-px[i][0], r[1]-px[i][1]) > ramp {
					r, _ = nearestOn(lpx[k], px[i])
				}
				beside := [2]float64{r[0] + lanes[k].offset*n[0], r[1] + lanes[k].offset*n[1]}
				target = [2]float64{c*target[0] + (1-c)*beside[0], c*target[1] + (1-c)*beside[1]}
			}
		}
		w := weight[i]
		out[i] = unpixel(v, [2]float64{px[i][0] + w*(target[0]-px[i][0]), px[i][1] + w*(target[1]-px[i][1])})
	}
	return out
}

// nearestOnWay is nearestOn over only the parts of the line heading the way
// asked, relative to a reference whose normal is n there: any part for way
// 0, parts heading the reference's way within the parallel angle for 1, the
// opposite way for -1. ok is false when no part does.
func nearestOnWay(pts [][2]float64, p, n [2]float64, way int, cosMin float64) (nearest, dir [2]float64, ok bool) {
	if way == 0 {
		q, d := nearestOn(pts, p)
		return q, d, true
	}
	heading := [2]float64{n[1], -n[0]} // the reference's direction: n turned back
	bestD := math.Inf(1)
	for i := 0; i+1 < len(pts); i++ {
		a, b := pts[i], pts[i+1]
		dx, dy := b[0]-a[0], b[1]-a[1]
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		u := [2]float64{dx / l, dy / l}
		if float64(way)*(u[0]*heading[0]+u[1]*heading[1]) < cosMin {
			continue
		}
		t := math.Max(0, math.Min(1, ((p[0]-a[0])*dx+(p[1]-a[1])*dy)/(l*l)))
		q := [2]float64{a[0] + t*dx, a[1] + t*dy}
		if d := (p[0]-q[0])*(p[0]-q[0]) + (p[1]-q[1])*(p[1]-q[1]); d < bestD {
			nearest, dir, bestD, ok = q, u, d, true
		}
	}
	return nearest, dir, ok
}

// nearestShared is the point in a lane nearest point i along the reference.
// There is one wherever the easing gives i any weight.
func nearestShared(shared, s []float64, i int) int {
	best, bestD := i, math.Inf(1)
	for j, sh := range shared {
		if sh == 1 {
			if d := math.Abs(s[j] - s[i]); d < bestD {
				best, bestD = j, d
			}
		}
	}
	return best
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
