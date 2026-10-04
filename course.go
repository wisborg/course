// Package course reads a recorded or planned course and says where it went.
//
// A Course is the part of an activity that is about ground: the positions it
// passed through, and -- where the file recorded them -- when, and how far
// along. It is read through fitactivity, so every format fitactivity takes is
// a course here, and a planned route with no times is one too: it has
// positions, and positions are all a summary of places or a map needs.
//
// What a file did not record is absent, not estimated. A GPX has no distance,
// so a course read from one reports none; a planned route has no times, so it
// reports no elapsed time. The caller shows what is there.
package course

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/wisborg/fitactivity"
)

// Course is a course's positions in order, with what the file said about each.
type Course struct {
	// Sources are the files it was read from.
	Sources []string
	// Sport is in the FIT profile's vocabulary, or empty; see
	// fitactivity.Track.Sport.
	Sport string
	// Timed reports whether every point carries the time it was reached. A
	// planned route does not.
	Timed bool
	// Start is when the activity started, when Timed.
	Start time.Time
	// Points are the positions, in time order for a recording and in the
	// file's order for a plan. Rogue fixes are already left out; see Read.
	Points []Point
	// Dropped is how many fixes were left out as rogue.
	Dropped int
	// TotalAscent and TotalDescent are the recording device's own totals
	// of climb and descent, in metres, when HasElevationTotals: a far
	// better guide to how much a noisy elevation trace should be smoothed
	// than anything worked out from the trace itself.
	HasElevationTotals        bool
	TotalAscent, TotalDescent float64
}

// Point is one position on a course.
type Point struct {
	Lat, Lon float64

	// Elapsed is the wall-clock time since Start, pauses included, and is
	// meaningful only on a Timed course.
	Elapsed time.Duration

	// HasDistance reports whether Distance is one the device recorded:
	// metres from the start, along the course.
	HasDistance bool
	Distance    float64

	HasElevation bool
	Elevation    float64

	// HasHeartRate reports whether the recording had a heart rate here, in
	// beats a minute.
	HasHeartRate bool
	HeartRate    float64

	// A recording can carry two power readings from two sensors, which
	// disagree: the standard FIT power field (Native), and a footpod's
	// developer field (Stryd). Both are kept; which one is meant is the
	// caller's to say, through Power.
	HasNativePower bool
	NativePower    float64
	HasStrydPower  bool
	StrydPower     float64

	// HasCadence reports whether the recording had a cadence here, as the
	// file has it: revolutions a minute, which for running is one leg's
	// steps -- half the steps a minute a runner knows.
	HasCadence bool
	Cadence    float64

	// HasAirPower reports whether a footpod estimated the power spent
	// against the air here, in watts: part of StrydPower, mostly headwind.
	HasAirPower bool
	AirPower    float64

	// A recording can carry two temperatures too, in °C: the watch's own
	// (Native), taken on a wrist and reading warm, and a footpod's of the
	// air (Stryd). Which one is meant is the caller's to say, through
	// Temperature.
	HasNativeTemperature bool
	NativeTemperature    float64
	HasStrydTemperature  bool
	StrydTemperature     float64

	// HasHumidity reports whether a footpod measured the relative humidity
	// here, in per cent; FIT's own record has no field for it.
	HasHumidity bool
	Humidity    float64
}

// Power is the power at the point from src, in watts, and whether there is
// one: fitactivity's own rule for choosing between the two sensors -- auto
// prefers the footpod and falls back to the standard field, and the forced
// sources never substitute the other -- so course, videofx and fitdash pick
// the same reading from the same file.
func (p Point) Power(src fitactivity.PowerSource) (float64, bool) {
	s := fitactivity.Sample{HasPower: p.HasNativePower, Power: uint16(p.NativePower)}
	if p.HasStrydPower {
		s.DevFields = map[string]float64{fitactivity.StrydPowerField: p.StrydPower}
	}
	return s.ResolvedPower(src)
}

// Temperature is the temperature at the point from src, in °C, and whether
// there is one, by fitactivity's rule for choosing between the two sensors,
// as Power is.
func (p Point) Temperature(src fitactivity.TemperatureSource) (float64, bool) {
	s := fitactivity.Sample{HasTemperature: p.HasNativeTemperature, Temperature: int8(math.Round(p.NativeTemperature))}
	if p.HasStrydTemperature {
		s.DevFields = map[string]float64{fitactivity.StrydTemperatureField: p.StrydTemperature}
	}
	return s.ResolvedTemperature(src)
}

// Read reads the course in the files at paths, several of which are merged as
// fitactivity.DecodeAll merges them.
//
// A file without times is read as a plan: its positions, in its own order,
// with no elapsed time. Several files must then be one of them only, since
// there is no clock to order a merge by.
func Read(paths ...string) (*Course, error) {
	if len(paths) == 0 {
		return nil, errors.New("no course given")
	}
	track, err := fitactivity.DecodeAll(paths...)
	if err == nil {
		return fromTrack(track), nil
	}
	if !errors.Is(err, fitactivity.ErrNoTimes) {
		return nil, err
	}
	if len(paths) > 1 {
		return nil, fmt.Errorf("%w; a planned route without times cannot be merged with other files, as there is nothing to order them by", err)
	}
	route, err := fitactivity.ReadRoute(paths[0])
	if err != nil {
		return nil, err
	}
	return fromRoute(route), nil
}

func fromTrack(t *fitactivity.Track) *Course {
	c := &Course{Sources: t.Sources, Sport: t.Sport, Timed: true,
		HasElevationTotals: t.HasElevationTotals, TotalAscent: t.TotalAscent, TotalDescent: t.TotalDescent}
	timer := fitactivity.BuildTimerModel(t)
	c.Start, _ = timer.Window()
	var fixes []fix
	for _, s := range t.Samples {
		if !s.HasGPS {
			continue
		}
		fixes = append(fixes, fix{Point: Point{
			Lat: s.Lat, Lon: s.Lon,
			Elapsed:     timer.Elapsed(s.Time),
			HasDistance: s.HasDistance, Distance: s.Distance,
			HasElevation: s.HasElevation, Elevation: s.Elevation,
			HasHeartRate: s.HasHeartRate, HeartRate: float64(s.HeartRate),
			HasNativePower: s.HasPower, NativePower: float64(s.Power),
			HasCadence: s.HasCadence, Cadence: float64(s.Cadence),
			HasNativeTemperature: s.HasTemperature, NativeTemperature: float64(s.Temperature),
		}, at: s.Time, timed: true})
		p := &fixes[len(fixes)-1].Point
		if w, ok := s.DevFields[fitactivity.StrydPowerField]; ok {
			p.HasStrydPower, p.StrydPower = true, w
		}
		if w, ok := s.DevFields[fitactivity.StrydAirPowerField]; ok {
			p.HasAirPower, p.AirPower = true, w
		}
		if v, ok := s.DevFields[fitactivity.StrydTemperatureField]; ok {
			p.HasStrydTemperature, p.StrydTemperature = true, v
		}
		if v, ok := s.Humidity(); ok {
			p.HasHumidity, p.Humidity = true, v
		}
	}
	c.Points, c.Dropped = keepPlausible(fixes)
	return c
}

func fromRoute(r *fitactivity.Route) *Course {
	c := &Course{Sources: []string{r.SourcePath}, Sport: r.Sport}
	fixes := make([]fix, len(r.Points))
	for i, p := range r.Points {
		fixes[i] = fix{Point: Point{Lat: p.Lat, Lon: p.Lon, HasElevation: p.HasElevation, Elevation: p.Elevation}}
	}
	c.Points, c.Dropped = keepPlausible(fixes)
	return c
}

type fix struct {
	Point
	at    time.Time
	timed bool
}

// MaxSpeed is the fastest a course is taken to move between two fixes, in
// metres a second: a little over an airliner's ground speed in a strong
// jet stream. A fix that could only be reached faster is a receiver's error.
const MaxSpeed = 350.0

// keepPlausible leaves out the fixes no course could have passed through.
//
// Exactly 0,0 is one: nobody records there, and it is what a device writes
// for "no fix" when it does not write FIT's own sentinel.
//
// On a timed course a spike is another: a fix that could only be reached
// from the last good one faster than MaxSpeed, when a fix shortly after it
// can be reached from there at a believable speed. That is a receiver jumping
// to the wrong place and back -- drawn on a map, a spike; in a summary, a
// country the course never entered. A jump the course does NOT come back
// from within spikeWindow fixes is kept: that is the course moving, however
// fast, and throwing away everything after it would be worse than any spike.
//
// A plan has no clock to judge a jump by, so only 0,0 is taken out of one; a
// plan is drawn by somebody, and its jumps are theirs. Wobble of a few metres
// is not touched at all -- it is below anything a place name or a map at a
// course's scale can show.
func keepPlausible(fixes []fix) ([]Point, int) {
	var kept []Point
	var last *fix
	dropped := 0
	for i := range fixes {
		f := &fixes[i]
		if f.Lat == 0 && f.Lon == 0 {
			dropped++
			continue
		}
		if last != nil && !reachable(last, f) && returns(last, fixes[i+1:]) {
			dropped++
			continue
		}
		kept = append(kept, f.Point)
		last = f
	}
	return kept, dropped
}

// spikeWindow is how many fixes a course has to come back within for a jump
// to be a spike.
const spikeWindow = 10

func reachable(from, to *fix) bool {
	if !from.timed || !to.timed {
		return true
	}
	dt := math.Max(to.at.Sub(from.at).Seconds(), 1)
	return Metres(from.Lat, from.Lon, to.Lat, to.Lon) <= MaxSpeed*dt
}

func returns(from *fix, next []fix) bool {
	for i := 0; i < len(next) && i < spikeWindow; i++ {
		if !(next[i].Lat == 0 && next[i].Lon == 0) && reachable(from, &next[i]) {
			return true
		}
	}
	return false
}

// Metres is the great-circle distance between two positions.
func Metres(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6_371_008.8
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	dφ, dλ := φ2-φ1, (lon2-lon1)*math.Pi/180
	a := math.Sin(dφ/2)*math.Sin(dφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(a)))
}

// Thin returns the indices of the points at least spacing metres apart along
// the course, always with the first and last.
//
// Measured from the last point kept, so a course that stands still -- a
// runner at a crossing, a plane at the gate -- collapses to one point, and a
// fast one keeps every fix further apart than spacing.
func (c *Course) Thin(spacing float64) []int {
	if len(c.Points) == 0 {
		return nil
	}
	idx := []int{0}
	last := c.Points[0]
	for i := 1; i < len(c.Points); i++ {
		p := c.Points[i]
		if Metres(last.Lat, last.Lon, p.Lat, p.Lon) >= spacing {
			idx = append(idx, i)
			last = p
		}
	}
	if end := len(c.Points) - 1; idx[len(idx)-1] != end {
		idx = append(idx, end)
	}
	return idx
}
