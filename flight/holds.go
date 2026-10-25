package flight

import (
	"math"
	"time"

	"github.com/wisborg/course"
)

// Hold is a hold flown: the aircraft going round, Laps times, turning right
// or left, from Start to End.
type Hold struct {
	Start, End Event
	Laps       int
	Right      bool

	// From and To are the elevations at the start and the end, when
	// HasLevels: a hold is often flown descending, level by level.
	HasLevels bool
	From, To  float64

	// Lat and Lon are the middle of the hold: the mean of its fixes.
	Lat, Lon float64
}

// Duration is how long the hold lasted.
func (h Hold) Duration() time.Duration { return h.End.Elapsed - h.Start.Elapsed }

const (
	// lapTurn is how far, in degrees, a course must turn one way to have
	// gone round once: a little under a full turn, for a lap closed on a
	// tracker's report a few degrees short of it.
	lapTurn = 340.0

	// maxLap is the longest one lap may take: a hold's two turns and two
	// legs of a few minutes each. A course that turns all the way round
	// more slowly than this is wandering, not holding.
	maxLap = 20 * time.Minute

	// trimSlack is how far short of whole laps the trimmed hold may turn:
	// its first and last reports are seldom exactly where the circuit
	// closes.
	trimSlack = 10.0
)

// holds are the holds flown between fixes a and b: every stretch in which
// the course turned all the way round one way within maxLap, joined where
// they overlap, and trimmed to whole laps -- so the turn onto the hold from
// the way it came, and off it toward the airport, are left out.
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
		total := turned[e] - turned[s]
		laps := max(1, int(math.Round(math.Abs(total)/360)))
		whole := float64(laps)*360 - trimSlack
		for s+1 < e && math.Abs(turned[e]-turned[s+1]) >= whole {
			s++
		}
		for e-1 > s && math.Abs(turned[e-1]-turned[s]) >= whole {
			e--
		}
		h := Hold{Start: event(pts, a+s), End: event(pts, a+e), Laps: laps, Right: total > 0}
		first, last := pts[a+s], pts[a+e]
		h.HasLevels = first.HasElevation && last.HasElevation
		h.From, h.To = first.Elevation, last.Elevation
		for k := s; k <= e; k++ {
			h.Lat += pts[a+k].Lat / float64(e-s+1)
			h.Lon += pts[a+k].Lon / float64(e-s+1)
		}
		out = append(out, h)
	}
	return out
}
