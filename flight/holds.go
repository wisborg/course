package flight

import (
	"math"
	"time"

	"github.com/wisborg/course"
)

// Hold is a hold flown: the aircraft going round, lap after lap, turning
// right or left, from Start to End.
type Hold struct {
	Start, End Event
	Laps       []Lap
	Right      bool

	// From and To are the elevations at the start and the end, when
	// HasLevels: a hold is often flown descending, level by level.
	HasLevels bool
	From, To  float64

	// Lat and Lon are the middle of the hold: the mean of its fixes.
	Lat, Lon float64
}

// Lap is one time round a hold. Laps differ: the controller shortens the
// last to fit the aircraft into the stream, or the first is the entry.
type Lap struct {
	Start, End Event
}

// Duration is how long the hold lasted.
func (h Hold) Duration() time.Duration { return h.End.Elapsed - h.Start.Elapsed }

// Duration is how long the lap took.
func (l Lap) Duration() time.Duration { return l.End.Elapsed - l.Start.Elapsed }

const (
	// lapTurn is how far, in degrees, a course must turn one way to have
	// gone round once: a little under a full turn, for a lap closed on a
	// tracker's report a few degrees short of it.
	lapTurn = 340.0

	// maxLap is the longest one lap may take: a hold's two turns and two
	// legs of a few minutes each. A course that turns all the way round
	// more slowly than this is wandering, not holding.
	maxLap = 20 * time.Minute

	// lapSlack is how far short of a full turn a lap may close: its
	// reports are seldom exactly where the circuit closes.
	lapSlack = 10.0

	// entryTurn is how far into a hold's turning its first turn is looked
	// for, and turnRate how fast, in degrees a second, a course must be
	// turning to be in it: a tenth of a rate-one turn, well clear of a
	// tracker's wobble on a straight leg.
	entryTurn = 90.0
	turnRate  = 0.3

	// closingTurn is how far round a lap must have turned before passing
	// back by where it began closes it: three quarters, so the far end of
	// the circuit is not mistaken for its close.
	closingTurn = 270.0

	// closingShare is how near, as a share of the furthest the lap went
	// from where it began, it must pass back by there to close.
	closingShare = 0.5

	// overTurn is how far past a full turn a lap is followed in search of
	// where it passes back by its start: a quarter turn more.
	overTurn = 90.0
)

// holds are the holds flown between fixes a and b: every stretch in which
// the course turned all the way round one way within maxLap, joined where
// they overlap, begun where its first turn began -- so the way it came is
// left out -- and cut into laps for as long as it goes on closing them, so
// the vectors after it are left out too.
//
// The turn is the course's direction from one fix to the next, unwound
// into a running total, so a tracker's wobble on a straight leg adds
// nothing: its turns one way and back cancel.
func holds(pts []course.Point, a, b int) []Hold {
	n := b - a + 1
	if n < 3 {
		return nil
	}
	turned := make([]float64, n)
	prev := math.NaN()
	for k := 1; k < n; k++ {
		turned[k] = turned[k-1]
		p, q := pts[a+k-1], pts[a+k]
		x, y := local(p.Lat, p.Lon, q.Lat, q.Lon)
		if x == 0 && y == 0 {
			continue
		}
		bearing := math.Atan2(x, y) * 180 / math.Pi
		if !math.IsNaN(prev) {
			turned[k] += math.Mod(bearing-prev+540, 360) - 180
		}
		prev = bearing
	}
	elapsed := func(k int) time.Duration { return pts[a+k].Elapsed }

	var spans [][2]int
	for i := range n {
		for j := i + 1; j < n && elapsed(j)-elapsed(i) <= maxLap; j++ {
			if math.Abs(turned[j]-turned[i]) < lapTurn {
				continue
			}
			if m := len(spans); m > 0 && i <= spans[m-1][1] {
				spans[m-1][1] = max(spans[m-1][1], j)
			} else {
				spans = append(spans, [2]int{i, j})
			}
			break
		}
	}

	var out []Hold
	for _, sp := range spans {
		s, e := sp[0], sp[1]
		sign := math.Copysign(1, turned[e]-turned[s])
		// Back from a quarter of the way round to where the turning began.
		m := s
		for m < e && sign*(turned[m]-turned[s]) < entryTurn {
			m++
		}
		for m > s+1 && sign*(turned[m]-turned[m-1]) >= turnRate*(elapsed(m)-elapsed(m-1)).Seconds() {
			m--
		}
		s = m
		h := Hold{Right: sign > 0}
		for start := s; ; {
			end, ok := closeLap(pts, a, turned, start, sign)
			if !ok {
				break
			}
			h.Laps = append(h.Laps, Lap{Start: event(pts, a+start), End: event(pts, a+end)})
			start = end
		}
		if len(h.Laps) == 0 {
			continue
		}
		h.Start, h.End = h.Laps[0].Start, h.Laps[len(h.Laps)-1].End
		first, last := pts[h.Start.Index], pts[h.End.Index]
		h.HasLevels = first.HasElevation && last.HasElevation
		h.From, h.To = first.Elevation, last.Elevation
		count := float64(h.End.Index - h.Start.Index + 1)
		for k := h.Start.Index; k <= h.End.Index; k++ {
			h.Lat += pts[k].Lat / count
			h.Lon += pts[k].Lon / count
		}
		out = append(out, h)
	}
	return out
}

// closeLap is where the lap begun at fix s closes, turning sign-wards: once
// it has turned three quarters of the way round, where it passes nearest
// back by where it began. That takes in the leg back to the start, which a
// lap ends with, and closes a lap the controller cut short and sent on its
// way, which passes back by its start before it has turned all the way
// round -- what comes after it is not the hold. A loop that never comes
// back near where it began closes where it has turned all the way round.
func closeLap(pts []course.Point, a int, turned []float64, s int, sign float64) (int, bool) {
	at := func(k int) float64 {
		return course.Metres(pts[a+s].Lat, pts[a+s].Lon, pts[a+k].Lat, pts[a+k].Lon)
	}
	furthest, full := 0.0, -1
	last := len(turned) - 1
	for j := s + 1; j <= last && pts[a+j].Elapsed-pts[a+s].Elapsed <= maxLap; j++ {
		furthest = max(furthest, at(j))
		round := sign * (turned[j] - turned[s])
		if round >= 360-lapSlack && full < 0 {
			full = j
		}
		if round > 360+overTurn {
			break
		}
		if round >= closingTurn && j < last && at(j) <= at(j-1) && at(j) < at(j+1) && at(j) <= closingShare*furthest {
			return j, true
		}
	}
	return full, full >= 0
}
