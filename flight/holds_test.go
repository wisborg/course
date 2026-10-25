package flight

import (
	"math"
	"testing"
	"time"
)

// turn is a turn at speed from bearing by degrees, right positive, over d,
// reported every five seconds at elevation e.
func turn(from, by, speed, e float64, d time.Duration) []stretch {
	var out []stretch
	steps := int(d / (5 * time.Second))
	for k := range steps {
		b := from + by*(float64(k)+0.5)/float64(steps)
		out = append(out, stretch{d: 5 * time.Second, speed: speed, bearing: math.Mod(b+360, 360), e0: e, e1: e, every: 5 * time.Second})
	}
	return out
}

// racetrack is a hold of laps right-hand laps at elevation e, entered and
// left heading south: a minute's turn, a minute's leg, and the same back.
func racetrack(laps int, e float64) []stretch {
	var out []stretch
	for range laps {
		out = append(out, turn(180, 180, 110, e, time.Minute)...)
		out = append(out, stretch{d: time.Minute, speed: 110, e0: e, e1: e, every: 10 * time.Second})
		out = append(out, turn(0, 180, 110, e, time.Minute)...)
		out = append(out, stretch{d: time.Minute, speed: 110, bearing: 180, e0: e, e1: e, every: 10 * time.Second})
	}
	return out
}

func TestHolds_TwoLapsRight(t *testing.T) {
	var ss []stretch
	ss = append(ss, stretch{d: 10 * time.Minute, speed: 150, bearing: 180, e0: 7000, e1: 7000, every: 10 * time.Second})
	ss = append(ss, racetrack(2, 7000)...)
	// Off the hold toward the airport: a quarter turn and a long leg, and
	// later a turn onto final -- turns, but nowhere near all the way round.
	ss = append(ss, turn(180, -90, 110, 5000, time.Minute)...)
	ss = append(ss, stretch{d: 10 * time.Minute, speed: 110, bearing: 90, e0: 5000, e1: 5000, every: 10 * time.Second})
	ss = append(ss, turn(90, 90, 80, 1000, 30*time.Second)...)
	c := fly(10, 20, ss...)
	hs := holds(c.Points, 0, len(c.Points)-1)
	if len(hs) != 1 {
		t.Fatalf("got %d holds, want 1: %+v", len(hs), hs)
	}
	h := hs[0]
	if h.Laps != 2 || !h.Right {
		t.Errorf("hold of %d laps, right %v; want 2 right", h.Laps, h.Right)
	}
	// The hold's first turn begins at ten minutes and its last ends at
	// seventeen: trimmed to its laps, it starts and ends within a report or
	// two of them. The last leg inbound is the way off it, as far as the
	// track can tell.
	if d := h.Start.Elapsed - 10*time.Minute; d < 0 || d > 15*time.Second {
		t.Errorf("hold starts at %v, want just after 10m", h.Start.Elapsed)
	}
	if d := 17*time.Minute - h.End.Elapsed; d < 0 || d > 15*time.Second {
		t.Errorf("hold ends at %v, want just before 17m", h.End.Elapsed)
	}
	if !h.HasLevels || h.From != 7000 || h.To != 7000 {
		t.Errorf("hold levels %v to %v (%v), want 7000 throughout", h.From, h.To, h.HasLevels)
	}
}

func TestHolds_AFlightThatOnlyTurnsHasNone(t *testing.T) {
	legs, _ := Legs(oneFlight(), nil)
	if len(legs) != 1 || len(legs[0].Holds) != 0 {
		t.Errorf("holds %+v in a flight that never went round", legs[0].Holds)
	}
	// A U-turn from downwind onto final turns half way round, not all the
	// way.
	var ss []stretch
	ss = append(ss, stretch{d: 5 * time.Minute, speed: 120, bearing: 0, e0: 1500, e1: 1500, every: 10 * time.Second})
	ss = append(ss, turn(0, 170, 100, 1000, time.Minute)...)
	ss = append(ss, stretch{d: 3 * time.Minute, speed: 90, bearing: 170, e0: 1000, e1: 300, every: 10 * time.Second})
	c := fly(10, 20, ss...)
	if hs := holds(c.Points, 0, len(c.Points)-1); len(hs) != 0 {
		t.Errorf("holds %+v in a turn onto final", hs)
	}
}
