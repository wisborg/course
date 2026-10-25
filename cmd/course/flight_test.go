package main

import (
	"strings"
	"testing"
	"time"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/osmbase/locate"

	"github.com/wisborg/course"
	"github.com/wisborg/course/flight"
)

func TestWriteFlights(t *testing.T) {
	u, err := units.Of(units.Imperial)
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Use(units.Distance, "nm"); err != nil {
		t.Fatal(err)
	}
	if err := u.Use(units.Speed, "kn"); err != nil {
		t.Fatal(err)
	}
	f := fitactivity.Flight{Number: "XX1", Callsign: "XXX1", From: "AAA", To: "BBB", Aircraft: "Test 100", AircraftType: "T100", Registration: "ZZ-ZZZ"}
	c := &course.Course{Start: time.Date(2026, 10, 23, 7, 0, 0, 0, time.UTC), Flights: []fitactivity.Flight{f}}
	min := func(m int) time.Duration { return time.Duration(m) * time.Minute }
	l := legPlace{
		Leg: flight.Leg{
			Flight: &c.Flights[0],
			Out:    flight.Event{Elapsed: min(10)}, In: flight.Event{Elapsed: min(100)}, InLast: true,
			Off: flight.Event{Elapsed: min(20)}, On: flight.Event{Elapsed: min(90), Lat: -33.94, Lon: 151.18}, HasOff: true, HasOn: true,
			Holds: []flight.Hold{{Start: flight.Event{Elapsed: min(70)}, End: flight.Event{Elapsed: min(78)},
				Laps:      []flight.Lap{{Start: flight.Event{Elapsed: min(70)}, End: flight.Event{Elapsed: min(78)}}},
				HasLevels: true, From: 7003, To: 6393, Lat: -32.69, Lon: 150.956}},
			TakeOff: &flight.Use{Designator: "34R"},
			Cruise:  []flight.Level{{Altitude: 34000 * 0.3048}}, HasHighest: true, Highest: 38000 * 0.3048,
			HasTopSpeed: true, TopSpeed: 232,
			Flown: 1852 * 400, Direct: 1852 * 380,
		},
		From: &locate.Match{Name: "Invented Airport"},
	}
	var b strings.Builder
	writeFlights(&b, c, []legPlace{l}, u)
	want := `
XX1 (XXX1) · Test 100 (T100) · ZZ-ZZZ
  from      AAA Invented Airport, runway 34R
  to        BBB
  times     out 07:10:00, off 07:20:00, on 08:30:00, in 08:40:00 UTC (still moving at the last report)
            block at least 1:30:00, air 1:10:00
  cruise    FL340 (highest FL380)
  holding   1 lap left, 08:10:00–08:18:00 UTC (8 min), FL230 to FL210, 76 nm N of BBB
  speed     top 451 kn over the ground
  distance  400.00 nm flown, 380.00 nm direct
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}

func TestLevel_AFlightLevelOnlyForATrackerAboveEveryTransition(t *testing.T) {
	u, _ := units.Of(units.Imperial)
	for _, c := range []struct {
		m       float64
		tracker bool
		want    string
	}{
		{35000 * 0.3048, true, "FL350"},
		{35000 * 0.3048, false, "35000 ft"},
		{9000 * 0.3048, true, "9000 ft"},
	} {
		if got := level(c.m, c.tracker, u); got != c.want {
			t.Errorf("level(%v, %v) = %q, want %q", c.m, c.tracker, got, c.want)
		}
	}
}

func TestLegTitle_WhatTheFileSays(t *testing.T) {
	for _, c := range []struct {
		f    *fitactivity.Flight
		want string
	}{
		{nil, "flight"},
		{&fitactivity.Flight{}, "flight"},
		{&fitactivity.Flight{Callsign: "THA951"}, "THA951"},
		{&fitactivity.Flight{Number: "TG951", Callsign: "TG951"}, "TG951"},
		{&fitactivity.Flight{Number: "TG951", AircraftType: "B77W"}, "TG951 · B77W"},
	} {
		if got := legTitle(legPlace{Leg: flight.Leg{Flight: c.f}}); got != c.want {
			t.Errorf("legTitle(%+v) = %q, want %q", c.f, got, c.want)
		}
	}
}
