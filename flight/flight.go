// Package flight says what a flight was, from the course alone: when the
// aircraft left the stand, left the ground, landed and stopped again, which
// runways it used, the levels it cruised at, and how fast and how far it went.
//
// A leg is found by speed, not by what the file claims: a stretch of the
// course faster than anything taxis is a take-off, a flight and a landing
// roll. So a flight tracker's file, a phone's GPX taken on board and several
// flights merged into one recording are all read the same way, and a file's
// word on which flights they were is taken only when it names as many as
// were flown.
//
// What the course did not record is absent, not estimated. With no
// elevation there is no telling when the wheels left the ground, so a leg
// has no off and on times and no levels; with no recorded speed there is no
// top speed. Each time is the first REPORT of the event, so it is up to one
// report interval late -- a few seconds for a flight tracker near the ground.
package flight

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/wisborg/fitactivity"

	"github.com/wisborg/course"
)

const (
	// FastSpeed is the speed, in metres a second, above which a course is
	// taking off, flying or landing: about 50 knots, faster than any
	// aircraft taxis and slower than nearly any leaves the ground.
	FastSpeed = 25.0

	// mergeGap is how long a course may report slower than FastSpeed --
	// from its first slow report to its next fast one --
	// mid-flight and still be one leg: a tracker's glitch, not a landing.
	mergeGap = time.Minute

	// minLegTime and minLegDistance are how long and how far a fast stretch
	// must last to be a flight rather than a rejected take-off or a fast
	// taxi.
	minLegTime     = 2 * time.Minute
	minLegDistance = 5000.0

	// StandReach is how far, in metres, an aircraft may move and still be
	// standing in one place: a tracker's wobble at the stand.
	StandReach = 50.0

	// moving is the recorded speed, in metres a second, above which the
	// aircraft was moving at the first or last fix: past a stand's wobble.
	moving = 2.0

	// airborne is how far above the ground, in metres, a fix must be to be
	// in the air: a little more than a tracker's ground reports scatter.
	airborne = 10.0

	// LevelStep is what levels are rounded to: a hundred feet, a flight
	// level's own step.
	LevelStep = 100 * 0.3048

	// minCruise is how long a level must be held to be a cruise level, and
	// cruiseShare how high it must be, as a share of the highest held.
	minCruise   = 5 * time.Minute
	cruiseShare = 0.75

	// rollWindow is how much of the start and end of a fast stretch is
	// taken as its runway roll when there is no elevation to tell.
	rollWindow = time.Minute
)

// Event is one moment of a leg: a fix of the course.
type Event struct {
	// Index is the fix's index in the course's points.
	Index    int
	Elapsed  time.Duration
	Lat, Lon float64
}

// Level is an altitude the aircraft held, in metres rounded to LevelStep,
// and for how long.
type Level struct {
	Altitude float64
	Held     time.Duration
}

// Leg is one flight: stand to stand.
type Leg struct {
	// Flight is the file's own word on which flight this was, when it names
	// as many flights as legs were flown; nil otherwise.
	Flight *fitactivity.Flight

	// Out is when the aircraft left the stand and In when it reached the
	// next. OutFirst reports that the course began with it already moving,
	// so Out is only the first fix, and InLast that it ended still moving.
	Out, In          Event
	OutFirst, InLast bool

	// Off and On are the first fixes in the air and back on the ground,
	// when HasOff and HasOn: only a course with elevations tells.
	Off, On       Event
	HasOff, HasOn bool

	// TakeOff and Landing are the runways used, where the map had them.
	TakeOff, Landing *Use

	// Cruise are the levels held long enough to be cruise, in order, and
	// Highest the highest elevation reported in the air, when HasHighest.
	Cruise     []Level
	HasHighest bool
	Highest    float64

	// TopSpeed is the fastest recorded speed over the ground in the air
	// held over three reports running, in metres a second, when HasTopSpeed.
	HasTopSpeed bool
	TopSpeed    float64

	// Flown is how far the course went in the air, in metres, and Direct
	// the great circle from where it left the ground to where it landed.
	Flown, Direct float64
}

// Block is the time from the stand to the stand.
func (l Leg) Block() time.Duration { return l.In.Elapsed - l.Out.Elapsed }

// Air is the time in the air, when HasOff and HasOn.
func (l Leg) Air() (time.Duration, bool) {
	return l.On.Elapsed - l.Off.Elapsed, l.HasOff && l.HasOn
}

// Finder is the runways the map has round a position, as Runways reads
// them.
type Finder func(lat, lon float64) ([]Runway, error)

// Legs finds the flights in a timed course, in order, with the runways from
// find when it is not nil. A course with no stretch fast enough for long
// enough has none, and so has a plan, which has no clock.
func Legs(c *course.Course, find Finder) ([]Leg, error) {
	pts := c.Points
	if !c.Timed || len(pts) < 2 {
		return nil, nil
	}
	speeds := speedsOf(pts)
	runs := fastRuns(pts, speeds)
	legs := make([]Leg, len(runs))
	for k, r := range runs {
		l := &legs[k]
		l.air(pts, r)
		l.fly(pts, r)
	}
	if len(runs) == 0 {
		return nil, nil
	}
	legs[0].Out, legs[0].OutFirst = leave(pts, 0, runs[0][0])
	for k := 1; k < len(runs); k++ {
		// The ground between two legs is shared: the longest stand on it is
		// where one ended and the next began.
		in, out := longestStand(pts, runs[k-1][1]+1, runs[k][0]-1)
		legs[k-1].In, legs[k].Out = event(pts, in), event(pts, out)
	}
	n := len(legs)
	legs[n-1].In, legs[n-1].InLast = arrive(pts, runs[n-1][1]+1)
	if len(c.Flights) == len(legs) {
		for k := range legs {
			legs[k].Flight = &c.Flights[k]
		}
	}
	if find == nil {
		return legs, nil
	}
	for k, r := range runs {
		var err error
		if legs[k].TakeOff, err = runwayUsed(pts, takeOffFixes(pts, legs[k], r), find); err != nil {
			return nil, err
		}
		if legs[k].Landing, err = runwayUsed(pts, landingFixes(pts, legs[k], r), find); err != nil {
			return nil, err
		}
	}
	return legs, nil
}

func event(pts []course.Point, i int) Event {
	return Event{Index: i, Elapsed: pts[i].Elapsed, Lat: pts[i].Lat, Lon: pts[i].Lon}
}

// speedsOf is each fix's speed: the faster of the recorded one and the
// distance between its neighbours over the time between them. A tracker
// now and then reports 20 knots at 35,000 feet, and the positions either
// side say otherwise.
func speedsOf(pts []course.Point) []float64 {
	out := make([]float64, len(pts))
	for i, p := range pts {
		a, b := max(i-1, 0), min(i+1, len(pts)-1)
		if dt := (pts[b].Elapsed - pts[a].Elapsed).Seconds(); dt > 0 {
			out[i] = course.Metres(pts[a].Lat, pts[a].Lon, pts[b].Lat, pts[b].Lon) / dt
		}
		if p.HasSpeed {
			out[i] = max(out[i], p.Speed)
		}
	}
	return out
}

// fastRuns are the stretches of the course, first and last fix, at
// FastSpeed or more for long enough and far enough to be a flight.
func fastRuns(pts []course.Point, speeds []float64) [][2]int {
	// A flight out of a tracker's reach for an hour over the ocean is one
	// flight: only slow reports end one, never no reports.
	var runs [][2]int
	slow := -1
	for i := range pts {
		if speeds[i] < FastSpeed {
			if slow < 0 {
				slow = i
			}
			continue
		}
		n := len(runs)
		if n > 0 && (slow < 0 || pts[i].Elapsed-pts[slow].Elapsed <= mergeGap) {
			runs[n-1][1] = i
		} else {
			runs = append(runs, [2]int{i, i})
		}
		slow = -1
	}
	var out [][2]int
	for _, r := range runs {
		if pts[r[1]].Elapsed-pts[r[0]].Elapsed >= minLegTime && along(pts, r[0], r[1]) >= minLegDistance {
			out = append(out, r)
		}
	}
	return out
}

// along is how far the course went from fix a to fix b.
func along(pts []course.Point, a, b int) float64 {
	d := 0.0
	for i := a + 1; i <= b; i++ {
		d += course.Metres(pts[i-1].Lat, pts[i-1].Lon, pts[i].Lat, pts[i].Lon)
	}
	return d
}

// air finds when the leg of fast stretch r left the ground and landed: the
// first fix airborne above the ground it rolled from, and the first fix
// after the last one airborne above the ground it rolled onto. The ground
// is the elevation reported there, whatever it is measured from -- a flight
// tracker's ground reports are zero, a phone's are the field's height.
func (l *Leg) air(pts []course.Point, r [2]int) {
	ground := func(i int) (float64, bool) {
		i = max(0, min(i, len(pts)-1))
		return pts[i].Elevation, pts[i].HasElevation
	}
	if g, ok := ground(r[0] - 1); ok {
		for i := r[0]; i <= r[1]; i++ {
			if pts[i].HasElevation && pts[i].Elevation >= g+airborne {
				l.Off, l.HasOff = event(pts, i), true
				break
			}
		}
	}
	if g, ok := ground(r[1] + 1); ok && l.HasOff {
		for i := r[1]; i > l.Off.Index; i-- {
			if pts[i].HasElevation && pts[i].Elevation >= g+airborne {
				l.On, l.HasOn = event(pts, min(i+1, len(pts)-1)), true
				break
			}
		}
	}
}

// fly is what the leg of fast stretch r did in the air: its levels, its top
// speed and how far it went. Between off and on where they are known, and
// over the fast stretch otherwise.
func (l *Leg) fly(pts []course.Point, r [2]int) {
	a, b := r[0], r[1]
	if l.HasOff && l.HasOn {
		a, b = l.Off.Index, l.On.Index
	}
	l.Flown = along(pts, a, b)
	l.Direct = course.Metres(pts[a].Lat, pts[a].Lon, pts[b].Lat, pts[b].Lon)
	// The top speed is one held over three reports running: a tracker's
	// single report of 800 knots is not one.
	for i := a + 1; i < b; i++ {
		if !pts[i-1].HasSpeed || !pts[i].HasSpeed || !pts[i+1].HasSpeed {
			continue
		}
		if v := min(pts[i-1].Speed, pts[i].Speed, pts[i+1].Speed); !l.HasTopSpeed || v > l.TopSpeed {
			l.HasTopSpeed, l.TopSpeed = true, v
		}
	}
	if !l.HasOff || !l.HasOn {
		return
	}
	for i := a; i < b; i++ {
		if pts[i].HasElevation && (!l.HasHighest || pts[i].Elevation > l.Highest) {
			l.HasHighest, l.Highest = true, pts[i].Elevation
		}
	}
	l.Cruise = cruise(pts[a:b])
}

// cruise are the levels held for minCruise or longer and high enough to be
// cruise rather than a level-off on the way up or down, in the order flown;
// a level left briefly and come back to is one. A level is held from its
// first fix until the first fix at another.
func cruise(pts []course.Point) []Level {
	var held []Level
	for i := 0; i < len(pts); {
		if !pts[i].HasElevation {
			i++
			continue
		}
		lv := math.Round(pts[i].Elevation/LevelStep) * LevelStep
		j := i + 1
		for j < len(pts) && (!pts[j].HasElevation || math.Round(pts[j].Elevation/LevelStep)*LevelStep == lv) {
			j++
		}
		end := pts[len(pts)-1].Elapsed
		if j < len(pts) {
			end = pts[j].Elapsed
		}
		if d := end - pts[i].Elapsed; d >= minCruise {
			held = append(held, Level{Altitude: lv, Held: d})
		}
		i = j
	}
	top := 0.0
	for _, h := range held {
		top = max(top, h.Altitude)
	}
	var out []Level
	for _, h := range held {
		if h.Altitude < cruiseShare*top {
			continue
		}
		if n := len(out); n > 0 && out[n-1].Altitude == h.Altitude {
			out[n-1].Held += h.Held
			continue
		}
		out = append(out, h)
	}
	return out
}

// leave is when the aircraft left the stand the course began at, before
// fix end: the first report of the move that took it further than
// StandReach from the first fix -- a pushback creeps, and is the move. A
// course that began moving began after it left, and its first fix is all
// there is.
func leave(pts []course.Point, from, end int) (Event, bool) {
	if pts[from].HasSpeed && pts[from].Speed >= moving {
		return event(pts, from), true
	}
	i := from + 1
	for i < end && course.Metres(pts[from].Lat, pts[from].Lon, pts[i].Lat, pts[i].Lon) <= StandReach {
		i++
	}
	for i > from+1 && creeping(pts, i-1, i) {
		i--
	}
	// The move began after fix i-1; i is its first report, unless the
	// aircraft had not yet moved from the stand there and only set off
	// from it.
	if i < end && course.Metres(pts[from].Lat, pts[from].Lon, pts[i].Lat, pts[i].Lon) <= wobble {
		i++
	}
	return event(pts, i), false
}

// wobble is how far, in metres, a tracker's position of an aircraft that has
// not moved can wander.
const wobble = 10.0

// arrive is when the aircraft reached the stand the course ended at, from
// fix from on: the first fix after which it stayed within StandReach of the
// last and did no more than wobble. A course that ended moving ended before
// it arrived, and its last fix is all there is.
func arrive(pts []course.Point, from int) (Event, bool) {
	last := len(pts) - 1
	from = min(from, last)
	i := last
	for i > from && course.Metres(pts[last].Lat, pts[last].Lon, pts[i-1].Lat, pts[i-1].Lon) <= StandReach && !creeping(pts, i-1, i) {
		i--
	}
	return event(pts, i), i == last && pts[last].HasSpeed && pts[last].Speed >= moving
}

// creep is the speed, in metres a second, between two fixes above which the
// aircraft moved between them rather than a tracker wobbling: slower than
// any pushback.
const creep = 0.5

// creeping reports whether the aircraft moved from fix a to fix b, by the
// distance and time between them rather than either's own speed, which a
// tracker drops to zero now and then while it moves.
func creeping(pts []course.Point, a, b int) bool {
	dt := (pts[b].Elapsed - pts[a].Elapsed).Seconds()
	return dt > 0 && course.Metres(pts[a].Lat, pts[a].Lon, pts[b].Lat, pts[b].Lon)/dt >= creep
}

// longestStand is the longest the aircraft stood in one place between fixes
// a and b, as the fix it arrived at and the first fix after it left: the
// stand between two flights, where a stop at a holding point is shorter.
func longestStand(pts []course.Point, a, b int) (int, int) {
	if a > b {
		return b, a
	}
	best, bestIn, bestOut := time.Duration(-1), a, b
	for i := a; i <= b; i++ {
		j := i
		for j+1 <= b && course.Metres(pts[i].Lat, pts[i].Lon, pts[j+1].Lat, pts[j+1].Lon) <= StandReach {
			j++
		}
		if d := pts[j].Elapsed - pts[i].Elapsed; d > best {
			best, bestIn, bestOut = d, i, min(j+1, b)
		}
	}
	return bestIn, bestOut
}

// takeOffFixes are the fixes of the leg's take-off roll: the fast stretch
// up to the first fix in the air, which is still over the runway's end, or
// its first rollWindow where the course has no elevations.
func takeOffFixes(pts []course.Point, l Leg, r [2]int) []int {
	var out []int
	for i := r[0]; i <= r[1]; i++ {
		if l.HasOff && i > l.Off.Index || !l.HasOff && pts[i].Elapsed-pts[r[0]].Elapsed > rollWindow {
			break
		}
		out = append(out, i)
	}
	return out
}

// landingFixes are the fixes of the leg's landing: from the last fix in the
// air, over the runway in the flare, to the end of the fast stretch, or its
// last rollWindow where the course has no elevations.
func landingFixes(pts []course.Point, l Leg, r [2]int) []int {
	var out []int
	for i := r[0]; i <= r[1]; i++ {
		if l.HasOn && i >= l.On.Index-1 || !l.HasOn && pts[r[1]].Elapsed-pts[i].Elapsed <= rollWindow {
			out = append(out, i)
		}
	}
	return out
}

// Runway is a runway as the map draws it: its name, "16R/34L", and the ends
// of its centre line.
type Runway struct {
	Ref        string
	Lat1, Lon1 float64
	Lat2, Lon2 float64
}

// Use is a runway used, and the direction it was used in: "34L".
type Use struct {
	Runway
	Designator string
}

const (
	// runwayReach is how far, in metres, a fix may be from a runway's centre
	// line and be on it: half a wide runway and a tracker's error.
	runwayReach = 60.0
	// runwayOverrun is how far past either end a fix may be and still be
	// taken as on the runway: the lift-off and the flare.
	runwayOverrun = 300.0
)

// runwayUsed is the runway the fixes ran along, and in which direction: the
// runway near them that the most of them were on, with the designator of
// the end the course ran toward. Nil when the map has none there, or fewer
// than two fixes were on it.
func runwayUsed(pts []course.Point, idx []int, find Finder) (*Use, error) {
	if len(idx) == 0 {
		return nil, nil
	}
	p := pts[idx[0]]
	rs, err := find(p.Lat, p.Lon)
	if err != nil {
		return nil, err
	}
	var best *Use
	bestN, bestCross := 0, 0.0
	for _, r := range rs {
		n, cross, first, last := 0, 0.0, 0.0, 0.0
		length := course.Metres(r.Lat1, r.Lon1, r.Lat2, r.Lon2)
		for _, i := range idx {
			a, c := r.project(pts[i].Lat, pts[i].Lon)
			if math.Abs(c) > runwayReach || a < -runwayOverrun || a > length+runwayOverrun {
				continue
			}
			if n == 0 {
				first = a
			}
			last = a
			n++
			cross += math.Abs(c)
		}
		if n < 2 || n < bestN || n == bestN && cross/float64(n) >= bestCross {
			continue
		}
		bearing := r.bearing()
		if last < first {
			bearing = math.Mod(bearing+180, 360)
		}
		bestN, bestCross = n, cross/float64(n)
		best = &Use{Runway: r, Designator: designator(r.Ref, bearing)}
	}
	return best, nil
}

// project is how far along the runway from its first end a position is,
// and how far off its centre line, in metres, on a plane through the
// runway: an airfield is small enough for one.
func (r Runway) project(lat, lon float64) (alongM, crossM float64) {
	ex, ey := local(r.Lat1, r.Lon1, r.Lat2, r.Lon2)
	px, py := local(r.Lat1, r.Lon1, lat, lon)
	l := math.Hypot(ex, ey)
	if l == 0 {
		return 0, math.Hypot(px, py)
	}
	ux, uy := ex/l, ey/l
	return px*ux + py*uy, -px*uy + py*ux
}

// bearing is the runway's direction from its first end to its second, in
// degrees clockwise from true north.
func (r Runway) bearing() float64 {
	x, y := local(r.Lat1, r.Lon1, r.Lat2, r.Lon2)
	return math.Mod(math.Atan2(x, y)*180/math.Pi+360, 360)
}

// local is position 2 in metres east and north of position 1.
func local(lat1, lon1, lat2, lon2 float64) (x, y float64) {
	const r = 6_371_008.8
	rad := math.Pi / 180
	return (lon2 - lon1) * rad * r * math.Cos((lat1+lat2)/2*rad), (lat2 - lat1) * rad * r
}

// designator is the end of runway ref that a course travelling at bearing
// (true) runs toward: the one whose number, its magnetic heading in tens of
// degrees, is the nearer. The two ends are 180° apart, so no magnetic
// variation on Earth is enough to confuse them. A ref that names no
// numbered end is given whole.
func designator(ref string, bearing float64) string {
	best, bestOff := ref, 91.0
	for part := range strings.SplitSeq(ref, "/") {
		part = strings.TrimSpace(part)
		n, err := strconv.Atoi(strings.TrimRight(part, "LRCSGWlrcsgw"))
		if err != nil || n < 1 || n > 36 {
			continue
		}
		off := math.Abs(math.Mod(float64(n*10)-bearing+540, 360) - 180)
		if off < bestOff {
			best, bestOff = part, off
		}
	}
	return best
}
