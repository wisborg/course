package flight

import (
	"math"
	"testing"
	"time"

	"github.com/wisborg/fitactivity"

	"github.com/wisborg/course"
)

// stretch is part of an invented flight: for d, at speed metres a second on
// bearing degrees true, from elevation e0 to e1, reported every every.
type stretch struct {
	d            time.Duration
	speed        float64
	bearing      float64
	e0, e1       float64
	every        time.Duration
	unrecorded   bool // no recorded speed, as a phone's GPX has none
	noElevations bool
}

// fly is a timed course from lat, lon through the stretches in turn, with a
// report at the end of every interval, so a stretch's first report is
// already one interval into it.
func fly(lat, lon float64, ss ...stretch) *course.Course {
	c := &course.Course{Timed: true, Start: time.Date(2026, 10, 23, 7, 0, 0, 0, time.UTC)}
	c.Points = append(c.Points, course.Point{Lat: lat, Lon: lon, HasElevation: true, HasSpeed: true})
	var t time.Duration
	for _, s := range ss {
		for k := s.every; k <= s.d; k += s.every {
			step := s.speed * s.every.Seconds()
			lat += step * math.Cos(s.bearing*math.Pi/180) / 111_195
			lon += step * math.Sin(s.bearing*math.Pi/180) / (111_195 * math.Cos(lat*math.Pi/180))
			p := course.Point{Lat: lat, Lon: lon, Elapsed: t + k,
				HasElevation: !s.noElevations, Elevation: s.e0 + (s.e1-s.e0)*float64(k)/float64(s.d),
				HasSpeed: !s.unrecorded, Speed: s.speed}
			c.Points = append(c.Points, p)
		}
		t += s.d
	}
	return c
}

// oneFlight is a flight east along 10°N: half an hour at the stand, a
// pushback and taxi south, a wait at the holding point, a roll down the
// runway, a climb to FL350, cruise, a descent, a landing roll, a taxi north
// and another half hour at the stand. Every place here is invented.
func oneFlight(opts ...func(*stretch)) *course.Course {
	ss := []stretch{
		{d: 30 * time.Minute, every: 30 * time.Second},
		{d: time.Minute, speed: 5, bearing: 180, every: 5 * time.Second},
		{d: time.Minute, every: 10 * time.Second},
		{d: 40 * time.Second, speed: 40, bearing: 90, every: 5 * time.Second},
		{d: 10 * time.Minute, speed: 120, bearing: 90, e0: 0, e1: 10668, every: 5 * time.Second},
		{d: 30 * time.Minute, speed: 230, bearing: 90, e0: 10668, e1: 10668, every: time.Minute},
		{d: 15 * time.Minute, speed: 150, bearing: 90, e0: 10668, e1: 100, every: 10 * time.Second},
		{d: 30 * time.Second, speed: 50, bearing: 90, every: 5 * time.Second},
		{d: 2 * time.Minute, speed: 8, bearing: 0, every: 10 * time.Second},
		{d: 30 * time.Minute, every: 30 * time.Second},
	}
	for i := range ss {
		for _, o := range opts {
			o(&ss[i])
		}
	}
	return fly(10.0027, 20, ss...)
}

func at(c *course.Course, d time.Duration) int {
	for i, p := range c.Points {
		if p.Elapsed == d {
			return i
		}
	}
	return -1
}

func TestLegs_OneFlightStandToStand(t *testing.T) {
	c := oneFlight()
	legs, err := Legs(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(legs) != 1 {
		t.Fatalf("got %d legs, want 1", len(legs))
	}
	l := legs[0]
	min := func(m float64) time.Duration { return time.Duration(m * float64(time.Minute)) }
	// Out is the first report of the pushback, not the last at the stand.
	if want := min(30) + 5*time.Second; l.Out.Elapsed != want || l.OutFirst {
		t.Errorf("out %v (first %v), want %v", l.Out.Elapsed, l.OutFirst, want)
	}
	// Off is the first report of the climb; On the first of the roll.
	if want := min(32) + 40*time.Second + 5*time.Second; !l.HasOff || l.Off.Elapsed != want {
		t.Errorf("off %v (%v), want %v", l.Off.Elapsed, l.HasOff, want)
	}
	landed := min(32) + 40*time.Second + min(55)
	if want := landed + 5*time.Second; !l.HasOn || l.On.Elapsed != want {
		t.Errorf("on %v (%v), want %v", l.On.Elapsed, l.HasOn, want)
	}
	// In is the last report of the taxi, where it reached the stand.
	if want := landed + 30*time.Second + min(2); l.In.Elapsed != want || l.InLast {
		t.Errorf("in %v (last %v), want %v", l.In.Elapsed, l.InLast, want)
	}
	if len(l.Cruise) != 1 || math.Abs(l.Cruise[0].Altitude-35000*0.3048) > 1 {
		t.Errorf("cruise %+v, want FL350 alone", l.Cruise)
	}
	if !l.HasTopSpeed || l.TopSpeed != 230 {
		t.Errorf("top speed %v (%v), want 230", l.TopSpeed, l.HasTopSpeed)
	}
	if air, ok := l.Air(); !ok || air != l.On.Elapsed-l.Off.Elapsed {
		t.Errorf("air %v (%v)", air, ok)
	}
}

func TestLegs_ATrackerGlitchIsNotALanding(t *testing.T) {
	c := oneFlight()
	// One report of 10 m/s at cruise with more than a minute to the next,
	// one report of 400, and a gap of an hour out of reach: none of it
	// ends the flight or is its top speed.
	i := at(c, 52*time.Minute+40*time.Second)
	c.Points[i].Speed = 10
	for j := i + 1; j < len(c.Points); j++ {
		c.Points[j].Elapsed += 5 * time.Second
	}
	c.Points[i+2].Speed = 400
	for j := i + 4; j < len(c.Points); j++ {
		c.Points[j].Elapsed += time.Hour
	}
	legs, _ := Legs(c, nil)
	if len(legs) != 1 {
		t.Fatalf("got %d legs, want 1", len(legs))
	}
	if legs[0].TopSpeed != 230 {
		t.Errorf("top speed %v, want 230: one report is not a speed held", legs[0].TopSpeed)
	}
}

func TestLegs_TwoFlightsShareTheStandBetween(t *testing.T) {
	a := oneFlight()
	b := oneFlight()
	end := a.Points[len(a.Points)-1]
	for _, p := range b.Points[1:] {
		p.Elapsed += end.Elapsed
		p.Lat += end.Lat - b.Points[0].Lat
		p.Lon += end.Lon - b.Points[0].Lon
		a.Points = append(a.Points, p)
	}
	a.Flights = []fitactivity.Flight{{Number: "XX1"}, {Number: "XX2"}}
	legs, _ := Legs(a, nil)
	if len(legs) != 2 {
		t.Fatalf("got %d legs, want 2", len(legs))
	}
	if legs[0].Flight.Number != "XX1" || legs[1].Flight.Number != "XX2" {
		t.Errorf("flights %v, %v", legs[0].Flight, legs[1].Flight)
	}
	if !(legs[0].In.Elapsed < legs[1].Out.Elapsed) || legs[1].Out.Elapsed-legs[0].In.Elapsed < 50*time.Minute {
		t.Errorf("in %v then out %v: the hour at the stand between is the turnaround", legs[0].In.Elapsed, legs[1].Out.Elapsed)
	}
}

func TestLegs_FlightsNotMatchingTheLegsAreNotGuessedAt(t *testing.T) {
	c := oneFlight()
	c.Flights = []fitactivity.Flight{{Number: "XX1"}, {Number: "XX2"}}
	legs, _ := Legs(c, nil)
	if len(legs) != 1 || legs[0].Flight != nil {
		t.Errorf("a leg was given one of two flights the file names")
	}
}

func TestLegs_WithoutElevationsThereIsNoOffOrOn(t *testing.T) {
	c := oneFlight(func(s *stretch) { s.noElevations, s.unrecorded = true, true })
	legs, _ := Legs(c, nil)
	if len(legs) != 1 {
		t.Fatalf("got %d legs, want 1", len(legs))
	}
	l := legs[0]
	if l.HasOff || l.HasOn || len(l.Cruise) > 0 || l.HasHighest || l.HasTopSpeed {
		t.Errorf("leg %+v claims what the course did not record", l)
	}
}

func TestLegs_AWalkIsNoFlight(t *testing.T) {
	c := fly(10, 20, stretch{d: time.Hour, speed: 1.5, bearing: 90, every: 5 * time.Second})
	if legs, _ := Legs(c, nil); len(legs) != 0 {
		t.Errorf("a walk has %d flights", len(legs))
	}
}

func TestLegs_Runways(t *testing.T) {
	c := oneFlight()
	legs, _ := Legs(c, nil)
	roll := c.Points[at(c, 32*time.Minute+5*time.Second)]
	land := c.Points[legs[0].On.Index]
	east := func(lat, lon, m float64) (float64, float64) {
		return lat, lon + m/(111_195*math.Cos(lat*math.Pi/180))
	}
	// The departure runway is drawn east to west and the arrival runway
	// named west end last, so neither the order of the ends nor of the
	// name says which way it was used. A parallel runway 300 m north is a
	// decoy at each.
	e1, e2 := east(roll.Lat, roll.Lon, 3000)
	w1, w2 := east(roll.Lat, roll.Lon, -200)
	l1, l2 := east(land.Lat, land.Lon, -500)
	r1, r2 := east(land.Lat, land.Lon, 2500)
	find := func(lat, lon float64) ([]Runway, error) {
		if math.Abs(lon-roll.Lon) < 0.1 {
			return []Runway{
				{Ref: "09L/27R", Lat1: e1 + 0.0027, Lon1: e2, Lat2: w1 + 0.0027, Lon2: w2},
				{Ref: "09R/27L", Lat1: e1, Lon1: e2, Lat2: w1, Lon2: w2},
			}, nil
		}
		return []Runway{
			{Ref: "26/08", Lat1: l1, Lon1: l2, Lat2: r1, Lon2: r2},
			{Ref: "35/17", Lat1: l1 - 0.01, Lon1: l2, Lat2: l1 + 0.01, Lon2: l2},
		}, nil
	}
	legs, err := Legs(c, find)
	if err != nil {
		t.Fatal(err)
	}
	if u := legs[0].TakeOff; u == nil || u.Designator != "09R" {
		t.Errorf("took off from %+v, want 09R", u)
	}
	if u := legs[0].Landing; u == nil || u.Designator != "08" {
		t.Errorf("landed on %+v, want 08", u)
	}
}

func TestDesignator(t *testing.T) {
	for _, c := range []struct {
		ref     string
		bearing float64
		want    string
	}{
		{"16R/34L", 345, "34L"},
		{"16R/34L", 165, "16R"},
		{"09/27", 274, "27"},
		{"27/09", 85, "09"},
		{"18", 175, "18"},
		{"H1", 90, "H1"},
		// The variation is large here, and the nearer end still the one.
		{"01/19", 30, "01"},
	} {
		if got := designator(c.ref, c.bearing); got != c.want {
			t.Errorf("designator(%q, %v) = %q, want %q", c.ref, c.bearing, got, c.want)
		}
	}
}

func TestCruise_ALevelOffOnTheWayDownIsNotCruise(t *testing.T) {
	c := fly(10, 20,
		stretch{d: 20 * time.Minute, speed: 230, bearing: 90, e0: 10668, e1: 10668, every: time.Minute},
		stretch{d: 5 * time.Minute, speed: 200, bearing: 90, e0: 10668, e1: 6400, every: 30 * time.Second},
		stretch{d: 8 * time.Minute, speed: 180, bearing: 90, e0: 6400, e1: 6400, every: 30 * time.Second},
	)
	got := cruise(c.Points[1:])
	if len(got) != 1 || math.Abs(got[0].Altitude-10668) > 1 {
		t.Errorf("cruise %+v, want FL350 alone", got)
	}
}

func TestLegs_TheApproachNamesTheRunwayWhenTheRollIsLost(t *testing.T) {
	c := oneFlight()
	legs, _ := Legs(c, nil)
	on := c.Points[legs[0].On.Index]
	// The tracker loses the aircraft over the threshold and finds it again
	// on the taxiway: the roll's reports are gone.
	var kept []course.Point
	for i, p := range c.Points {
		if i < legs[0].On.Index || p.Speed < FastSpeed {
			kept = append(kept, p)
		}
	}
	c.Points = kept
	l1, l2 := on.Lat, on.Lon-300/(111_195*math.Cos(on.Lat*math.Pi/180))
	r1, r2 := on.Lat, on.Lon+2500/(111_195*math.Cos(on.Lat*math.Pi/180))
	find := func(lat, lon float64) ([]Runway, error) {
		return []Runway{
			{Ref: "08/26", Lat1: r1, Lon1: r2, Lat2: l1, Lon2: l2},
			{Ref: "17/35", Lat1: l1 - 0.01, Lon1: l2, Lat2: l1 + 0.01, Lon2: l2},
		}, nil
	}
	legs, err := Legs(c, find)
	if err != nil {
		t.Fatal(err)
	}
	if len(legs) != 1 {
		t.Fatalf("got %d legs", len(legs))
	}
	if u := legs[0].Landing; u == nil || u.Designator != "08" {
		t.Errorf("landed on %+v, want 08, named by the approach", u)
	}
}
