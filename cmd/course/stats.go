package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/units"

	cw "github.com/wisborg/cobrakit/weather"

	"github.com/wisborg/course"
	"github.com/wisborg/course/routemap"
)

// stat is one number --stats can put over a map: its name on the command
// line, its caption on the map, and its value over a course in u, with
// false where the course has nothing to work it out from.
type stat struct {
	name, caption string
	value         func(c *course.Course, u units.Set, src statSources) (string, bool)
}

// statSources are which sensor's power and temperature the stats read,
// where a file has two: the map's --power-source and --temperature-source.
type statSources struct {
	power       fitactivity.PowerSource
	temperature fitactivity.TemperatureSource
}

// stats are every stat --stats knows, in the order they are drawn when
// named in another.
var stats = []stat{
	{"distance", "distance", func(c *course.Course, u units.Set, _ statSources) (string, bool) {
		d := c.Length()
		return distance(d, u), d > 0
	}},
	{"time", "time", func(c *course.Course, _ units.Set, _ statSources) (string, bool) {
		if !c.Timed || len(c.Points) < 2 {
			return "", false
		}
		return clock(c.Points[len(c.Points)-1].Elapsed), true
	}},
	{"moving-time", "moving time", func(c *course.Course, _ units.Set, _ statSources) (string, bool) {
		t, _, ok := moving(c)
		return clock(t), ok
	}},
	{"pace", "pace", func(c *course.Course, u units.Set, _ statSources) (string, bool) {
		t, d, ok := moving(c)
		if !ok || d <= 0 {
			return "", false
		}
		return units.FormatPace(d/t.Seconds(), u.Pace), true
	}},
	{"speed", "speed", func(c *course.Course, u units.Set, _ statSources) (string, bool) {
		t, d, ok := moving(c)
		if !ok || d <= 0 {
			return "", false
		}
		return units.FormatSpeed(d/t.Seconds(), u.Speed), true
	}},
	{"ascent", "ascent", func(c *course.Course, u units.Set, _ statSources) (string, bool) {
		up, _, ok := climbed(c)
		return short(up, u), ok
	}},
	{"descent", "descent", func(c *course.Course, u units.Set, _ statSources) (string, bool) {
		_, down, ok := climbed(c)
		return short(down, u), ok
	}},
	{"heart-rate", "heart rate", func(c *course.Course, _ units.Set, _ statSources) (string, bool) {
		v, ok := mean(routemap.HeartRate(c))
		return fmt.Sprintf("%.0f bpm", v), ok
	}},
	{"max-heart-rate", "max heart rate", func(c *course.Course, _ units.Set, _ statSources) (string, bool) {
		v, ok := highest(routemap.HeartRate(c))
		return fmt.Sprintf("%.0f bpm", v), ok
	}},
	{"power", "power", func(c *course.Course, _ units.Set, src statSources) (string, bool) {
		v, ok := mean(routemap.Power(c, src.power))
		return fmt.Sprintf("%.0f W", v), ok
	}},
	{"cadence", "cadence", func(c *course.Course, _ units.Set, _ statSources) (string, bool) {
		values, unit := routemap.Cadence(c)
		// Standing still is not a cadence of 0 to average in.
		for i, v := range values {
			if v == 0 {
				values[i] = math.NaN()
			}
		}
		v, ok := mean(values)
		return fmt.Sprintf("%.0f %s", v, unit), ok
	}},
	{"temperature", "temperature", func(c *course.Course, u units.Set, src statSources) (string, bool) {
		v, ok := mean(routemap.Temperature(c, src.temperature))
		return fmt.Sprintf("%.0f%s", u.Temperature.FromSI(v), u.Temperature.Name), ok
	}},
}

// autoStats are what --stats alone shows, those of them the course has:
// how far and how long, the pace on foot and the speed on anything else,
// the climb, and the heart rate.
func autoStats(sport string) []string {
	pace := "speed"
	if cw.OnFoot(sport) {
		pace = "pace"
	}
	return []string{"distance", "time", pace, "ascent", "heart-rate"}
}

// statNames are every stat's name, for help and errors.
func statNames() string {
	var names []string
	for _, s := range stats {
		names = append(names, s.name)
	}
	return strings.Join(names, ", ")
}

// statLine is one stat as drawn: its caption and its value.
type statLine struct{ caption, value string }

// statsOf are the stats names asks for over c, in the order given. "auto"
// is autoStats, of which those the course lacks are left out; a stat named
// that the course lacks is an error, as --colour's are.
func statsOf(names []string, c *course.Course, u units.Set, src statSources) ([]statLine, error) {
	var out []statLine
	for _, name := range names {
		auto := name == "auto"
		want := []string{name}
		if auto {
			want = autoStats(c.Sport)
		}
		for _, w := range want {
			s, ok := statNamed(w)
			if !ok {
				return nil, fmt.Errorf("--stats %q is not one of auto, %s", w, statNames())
			}
			v, has := s.value(c, u, src)
			if !has {
				if auto {
					continue
				}
				return nil, fmt.Errorf("--stats %s: the course records nothing to work it out from", w)
			}
			out = append(out, statLine{caption: s.caption, value: v})
		}
	}
	return out, nil
}

func statNamed(name string) (stat, bool) {
	for _, s := range stats {
		if s.name == name {
			return s, true
		}
	}
	return stat{}, false
}

// movingSpeed is the slowest a stretch between two points may be gone over
// and still count as moving, in metres a second: under a slow walk, over
// a watch's drift while standing.
const movingSpeed = 0.5

// moving is the time c spent moving and the distance covered in it: each
// stretch between two points gone over at movingSpeed or faster, so a
// pause -- at a crossing, or the watch's own -- is left out.
func moving(c *course.Course) (time.Duration, float64, bool) {
	if !c.Timed || len(c.Points) < 2 {
		return 0, 0, false
	}
	along := c.Along()
	var t time.Duration
	var d float64
	for i := 1; i < len(c.Points); i++ {
		dt := c.Points[i].Elapsed - c.Points[i-1].Elapsed
		dd := along[i] - along[i-1]
		if dt > 0 && dd/dt.Seconds() >= movingSpeed {
			t += dt
			d += dd
		}
	}
	return t, d, t > 0
}

// climbed is how far c climbed and descended: the device's own totals
// where it kept them, and otherwise fitactivity's smoothed elevation's.
func climbed(c *course.Course) (up, down float64, ok bool) {
	if c.HasElevationTotals {
		return c.TotalAscent, c.TotalDescent, true
	}
	if c.Track == nil {
		return 0, 0, false
	}
	m := fitactivity.BuildElevationModel(c.Track, fitactivity.ElevationOptions{})
	if m == nil || m.Empty() {
		return 0, 0, false
	}
	return m.TotalGain(), m.TotalLoss(), true
}

// mean is the average of values, NaN left out.
func mean(values []float64) (float64, bool) {
	var sum float64
	var n int
	for _, v := range values {
		if !math.IsNaN(v) {
			sum += v
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// highest is the largest of values, NaN left out.
func highest(values []float64) (float64, bool) {
	best, ok := 0.0, false
	for _, v := range values {
		if !math.IsNaN(v) && (!ok || v > best) {
			best, ok = v, true
		}
	}
	return best, ok
}
