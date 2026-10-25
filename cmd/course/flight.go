package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/osmbase/locate"
	"github.com/wisborg/output"

	"github.com/wisborg/course"
	"github.com/wisborg/course/flight"
)

// legPlace is a leg with the airports it left and reached, where the store
// names them.
type legPlace struct {
	flight.Leg
	From, To *locate.Match
}

// flightLegs are the flights in c, with their runways and airports from the
// store where it holds them.
func flightLegs(ctx context.Context, c *course.Course, st *store, lang string) ([]legPlace, error) {
	src := st.tileSource()
	var find flight.Finder
	if src != nil {
		find = func(lat, lon float64) ([]flight.Runway, error) { return flight.Runways(src, lat, lon) }
	}
	legs, err := flight.Legs(c, find)
	if err != nil || len(legs) == 0 {
		return nil, err
	}
	out := make([]legPlace, len(legs))
	var ends []locate.Coord
	for i, l := range legs {
		out[i].Leg = l
		ends = append(ends, locate.Coord{Lat: l.Out.Lat, Lon: l.Out.Lon}, locate.Coord{Lat: l.In.Lat, Lon: l.In.Lon})
	}
	if src == nil {
		return out, nil
	}
	places, err := locate.AtEach(ctx, src, ends, locate.Options{Language: lang, Levels: []locate.Level{locate.Area}})
	if err != nil {
		return nil, err
	}
	at := func(p locate.Place) *locate.Match {
		if m, ok := p.Match(locate.Area); ok && m.Kind == "aerodrome" {
			return &m
		}
		return nil
	}
	for i := range out {
		out[i].From, out[i].To = at(places[2*i]), at(places[2*i+1])
	}
	return out, nil
}

// flightUnits is the units a flight is written in: nautical miles, feet and
// knots, as fitdash writes one, unless --units or --unit say otherwise.
func flightUnits(cmd *cobra.Command, f output.Format) (units.Set, error) {
	if f != output.Text || cmd.Flags().Changed("units") || len(unitOpts.each) > 0 {
		return unitsShown(cmd, f)
	}
	u, err := units.Of(units.Imperial)
	if err != nil {
		return u, err
	}
	for q, name := range map[units.Quantity]string{units.Distance: "nm", units.Speed: "kn"} {
		if err := u.Use(q, name); err != nil {
			return u, err
		}
	}
	return u, nil
}

// highestMargin is how far above the highest cruise level, in metres, the
// highest altitude must be to be worth saying: a thousand feet, a level's
// spacing, and not a tracker's overshoot of the level it was cleared to.
const highestMargin = 1000 * 0.3048

// transitionCeiling is the highest transition altitude anywhere, in metres:
// above it a pressure altitude is a flight level everywhere.
const transitionCeiling = 18_000 * 0.3048

// writeFlights writes each leg as a block: what flew, between where, when,
// how high, how fast and how far. A flight tracker's altitudes are pressure
// altitudes, so its cruise is written as flight levels.
func writeFlights(w io.Writer, c *course.Course, legs []legPlace, u units.Set) {
	tracker := len(c.Flights) > 0
	for _, l := range legs {
		fmt.Fprintln(w)
		fmt.Fprintln(w, legTitle(l))
		row := func(label, format string, a ...any) {
			fmt.Fprintf(w, "  %-9s %s\n", label, fmt.Sprintf(format, a...))
		}
		var code [2]string
		if l.Flight != nil {
			code = [2]string{l.Flight.From, l.Flight.To}
		}
		row("from", "%s", airportLine(code[0], l.From, l.TakeOff))
		row("to", "%s", airportLine(code[1], l.To, l.Landing))

		times := []string{"out " + utc(c, l.Out.Elapsed)}
		if l.HasOff {
			times = append(times, "off "+utc(c, l.Off.Elapsed))
		}
		if l.HasOn {
			times = append(times, "on "+utc(c, l.On.Elapsed))
		}
		times = append(times, "in "+utc(c, l.In.Elapsed))
		line := strings.Join(times, ", ") + " UTC"
		var notes []string
		if l.OutFirst {
			notes = append(notes, "already moving at the first report")
		}
		if l.InLast {
			notes = append(notes, "still moving at the last report")
		}
		if len(notes) > 0 {
			line += " (" + strings.Join(notes, "; ") + ")"
		}
		row("times", "%s", line)
		block := "block " + clock(l.Block())
		if l.OutFirst || l.InLast {
			block = "block at least " + clock(l.Block())
		}
		if air, ok := l.Air(); ok {
			block += ", air " + clock(air)
		}
		row("", "%s", block)

		var levels []string
		top := 0.0
		for _, lv := range l.Cruise {
			levels = append(levels, level(lv.Altitude, tracker, u))
			top = max(top, lv.Altitude)
		}
		switch {
		case len(levels) > 0:
			line := strings.Join(levels, ", ")
			// A level reached but not held -- a climb too high, given back
			// -- is not cruise, but it is the flight's highest.
			if l.HasHighest && l.Highest >= top+highestMargin {
				line += " (highest " + level(math.Round(l.Highest/flight.LevelStep)*flight.LevelStep, tracker, u) + ")"
			}
			row("cruise", "%s", line)
		case l.HasHighest:
			row("highest", "%s", level(l.Highest, tracker, u))
		}
		if l.HasTopSpeed {
			row("speed", "top %s over the ground", units.FormatSpeed(l.TopSpeed, u.Speed))
		}
		row("distance", "%s flown, %s direct", distance(l.Flown, u), distance(l.Direct, u))
	}
}

// legTitle is what flew: "QF483 (QFA483) · Boeing 737-838 (B738) · VH-VXL",
// or "flight" where the file does not say.
func legTitle(l legPlace) string {
	f := l.Flight
	if f == nil {
		return "flight"
	}
	var parts []string
	add := func(s string) {
		if s != "" {
			parts = append(parts, s)
		}
	}
	name := f.Number
	if f.Callsign != "" && f.Callsign != f.Number {
		if name == "" {
			name = f.Callsign
		} else {
			name += " (" + f.Callsign + ")"
		}
	}
	add(name)
	aircraft := f.Aircraft
	if f.AircraftType != "" {
		if aircraft == "" {
			aircraft = f.AircraftType
		} else {
			aircraft += " (" + f.AircraftType + ")"
		}
	}
	add(aircraft)
	add(f.Registration)
	if len(parts) == 0 {
		return "flight"
	}
	return strings.Join(parts, " · ")
}

// airportLine is one end of a leg: "SYD Sydney (Kingsford Smith) Airport,
// runway 34L", with whatever of it is known.
func airportLine(code string, m *locate.Match, rw *flight.Use) string {
	var parts []string
	if code != "" {
		parts = append(parts, code)
	}
	if m != nil {
		parts = append(parts, m.Name)
	}
	s := strings.Join(parts, " ")
	if s == "" {
		s = "(not named in the store)"
	}
	if rw != nil {
		s += ", runway " + rw.Designator
	}
	return s
}

// level is an altitude as a pilot says it: a flight level for a tracker's
// pressure altitude above every transition altitude, and otherwise in the
// elevation unit.
func level(m float64, tracker bool, u units.Set) string {
	if tracker && m >= transitionCeiling {
		return fmt.Sprintf("FL%03.0f", m/0.3048/100)
	}
	return short(m, u)
}

// utc is a moment of the course as a time of day in UTC.
func utc(c *course.Course, elapsed time.Duration) string {
	return c.Start.Add(elapsed).UTC().Format("15:04:05")
}

type jsonEvent struct {
	Time      time.Time `json:"time" yaml:"time"`
	ElapsedS  float64   `json:"elapsed_s" yaml:"elapsed_s"`
	Latitude  float64   `json:"latitude" yaml:"latitude"`
	Longitude float64   `json:"longitude" yaml:"longitude"`
}

type jsonLevel struct {
	AltitudeM float64 `json:"altitude_m" yaml:"altitude_m"`
	HeldS     float64 `json:"held_s" yaml:"held_s"`
}

type jsonLeg struct {
	Number        string        `json:"number,omitempty" yaml:"number,omitempty"`
	Callsign      string        `json:"callsign,omitempty" yaml:"callsign,omitempty"`
	FromCode      string        `json:"from_code,omitempty" yaml:"from_code,omitempty"`
	ToCode        string        `json:"to_code,omitempty" yaml:"to_code,omitempty"`
	Aircraft      string        `json:"aircraft,omitempty" yaml:"aircraft,omitempty"`
	AircraftType  string        `json:"aircraft_type,omitempty" yaml:"aircraft_type,omitempty"`
	Registration  string        `json:"registration,omitempty" yaml:"registration,omitempty"`
	From          *locate.Match `json:"from,omitempty" yaml:"from,omitempty"`
	To            *locate.Match `json:"to,omitempty" yaml:"to,omitempty"`
	TakeOffRunway string        `json:"take_off_runway,omitempty" yaml:"take_off_runway,omitempty"`
	LandingRunway string        `json:"landing_runway,omitempty" yaml:"landing_runway,omitempty"`
	Out           jsonEvent     `json:"out" yaml:"out"`
	Off           *jsonEvent    `json:"off,omitempty" yaml:"off,omitempty"`
	On            *jsonEvent    `json:"on,omitempty" yaml:"on,omitempty"`
	In            jsonEvent     `json:"in" yaml:"in"`
	OutIsFirstFix bool          `json:"out_is_first_fix" yaml:"out_is_first_fix"`
	InIsLastFix   bool          `json:"in_is_last_fix" yaml:"in_is_last_fix"`
	Cruise        []jsonLevel   `json:"cruise" yaml:"cruise"`
	HighestM      *float64      `json:"highest_m,omitempty" yaml:"highest_m,omitempty"`
	TopSpeedMS    *float64      `json:"top_speed_m_s,omitempty" yaml:"top_speed_m_s,omitempty"`
	FlownM        float64       `json:"flown_m" yaml:"flown_m"`
	DirectM       float64       `json:"direct_m" yaml:"direct_m"`
}

// jsonFlights is the legs for JSON and YAML, in metres and seconds; a time
// the course could not tell is left out, never written as zero.
func jsonFlights(c *course.Course, legs []legPlace) []jsonLeg {
	ev := func(e flight.Event) jsonEvent {
		return jsonEvent{Time: c.Start.Add(e.Elapsed).UTC(), ElapsedS: e.Elapsed.Seconds(), Latitude: e.Lat, Longitude: e.Lon}
	}
	out := []jsonLeg{}
	for _, l := range legs {
		j := jsonLeg{From: l.From, To: l.To, Out: ev(l.Out), In: ev(l.In),
			OutIsFirstFix: l.OutFirst, InIsLastFix: l.InLast,
			Cruise: []jsonLevel{}, FlownM: l.Flown, DirectM: l.Direct}
		if f := l.Flight; f != nil {
			j.Number, j.Callsign, j.FromCode, j.ToCode = f.Number, f.Callsign, f.From, f.To
			j.Aircraft, j.AircraftType, j.Registration = f.Aircraft, f.AircraftType, f.Registration
		}
		if l.TakeOff != nil {
			j.TakeOffRunway = l.TakeOff.Designator
		}
		if l.Landing != nil {
			j.LandingRunway = l.Landing.Designator
		}
		if l.HasOff {
			e := ev(l.Off)
			j.Off = &e
		}
		if l.HasOn {
			e := ev(l.On)
			j.On = &e
		}
		for _, lv := range l.Cruise {
			j.Cruise = append(j.Cruise, jsonLevel{AltitudeM: lv.Altitude, HeldS: lv.Held.Seconds()})
		}
		if l.HasHighest {
			v := l.Highest
			j.HighestM = &v
		}
		if l.HasTopSpeed {
			v := l.TopSpeed
			j.TopSpeedMS = &v
		}
		out = append(out, j)
	}
	return out
}
