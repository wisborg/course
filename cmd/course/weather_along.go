package main

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	cw "github.com/wisborg/cobrakit/weather"
	"github.com/wisborg/fitactivity/units"
	fw "github.com/wisborg/fitactivity/weather"

	"github.com/wisborg/course"
)

// every is how often --weather-every asks for a row of the weather along
// a course: each so many metres, or each so long; neither is the default,
// which depends on the sport.
type every struct {
	metres float64
	period time.Duration
}

// mile is a mile in metres.
const mile = 1609.344

// parseEvery reads --weather-every: a distance in km or mi -- "2km",
// "0.5mi" -- or a duration as Go writes one -- "30m", "1h", "1h30m".
// Since "m" is minutes there, a number of minutes too large to have meant
// them -- "500m" -- is refused with the way to write metres.
func parseEvery(s string) (every, error) {
	if s == "" {
		return every{}, nil
	}
	for suffix, metres := range map[string]float64{"km": 1000, "mi": mile} {
		if n, ok := strings.CutSuffix(s, suffix); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
			if err != nil || v*metres < 100 {
				return every{}, fmt.Errorf("--weather-every %q: give a distance of at least 0.1km, such as 2km or 1mi", s)
			}
			return every{metres: v * metres}, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return every{}, fmt.Errorf("--weather-every %q is not a distance (2km, 1mi) or a duration (30m, 1h)", s)
	}
	if n, ok := strings.CutSuffix(s, "m"); ok && !strings.ContainsAny(n, "hms") && d > 2*time.Hour {
		return every{}, fmt.Errorf("--weather-every %q is %s, since m is minutes; for metres give km: %gkm", s, d, d.Minutes()/1000)
	}
	if d < time.Minute {
		return every{}, fmt.Errorf("--weather-every %q: give a duration of at least a minute", s)
	}
	return every{period: d}, nil
}

// defaultEvery is the spacing without --weather-every: a distance unit of
// u's -- a kilometre, or a mile -- on foot, and five on anything faster,
// so a marathon has its 42 rows and a 200 km ride its 40.
func defaultEvery(sport string, u units.Set) every {
	n := 5.0
	if cw.OnFoot(sport) {
		n = 1
	}
	return every{metres: u.Distance.ToSI(n)}
}

// weatherRow is the weather where the course was when it got there.
type weatherRow struct {
	elapsed  time.Duration
	distance float64
	lat, lon float64
	at       fw.Conditions
	// head and cross are the wind as the athlete met it, at 10 m in the
	// open: head against them, negative behind; cross from their right,
	// negative from the left. hasHeading is false where they were standing
	// still, with no direction to meet it from.
	head, cross float64
	hasHeading  bool
}

// weatherAlong is the weather along c each e, from the report: a row at
// the start, at the first point at or past each step, and at the finish.
func weatherAlong(r *fw.Report, c *course.Course, e every) []weatherRow {
	if r == nil || !c.Timed || len(c.Points) == 0 {
		return nil
	}
	step := e.metres
	if e.period > 0 {
		step = float64(e.period)
	}
	if step <= 0 {
		return nil
	}
	along := c.Along()
	var rows []weatherRow
	add := func(i int) {
		p := c.Points[i]
		cond, ok := r.At(p.Lat, p.Lon, true, c.Start.Add(p.Elapsed))
		if !ok {
			return
		}
		row := weatherRow{elapsed: p.Elapsed, distance: along[i], lat: p.Lat, lon: p.Lon, at: cond}
		if h, ok := headingAt(c, along, i); ok {
			row.head, row.cross = fw.Relative(cond, h)
			row.hasHeading = true
		}
		rows = append(rows, row)
	}
	next := 0.0
	for i, p := range c.Points {
		pos := along[i]
		if e.period > 0 {
			pos = float64(p.Elapsed)
		}
		if pos >= next || i == len(c.Points)-1 {
			add(i)
			for next <= pos {
				next += step
			}
		}
	}
	return rows
}

// headingSpan is how far either side of a point its direction is measured
// over, in metres: past a GPS fix's wander, within a street's turn.
const headingSpan = 25.0

// headingAt is the direction c was going at point i, from the point
// headingSpan behind it to the one headingSpan ahead; none where those
// are too close together to say.
func headingAt(c *course.Course, along []float64, i int) (float64, bool) {
	j, k := i, i
	for j > 0 && along[i]-along[j] < headingSpan {
		j--
	}
	for k < len(along)-1 && along[k]-along[i] < headingSpan {
		k++
	}
	a, b := c.Points[j], c.Points[k]
	if course.Metres(a.Lat, a.Lon, b.Lat, b.Lon) < headingSpan/2 {
		return 0, false
	}
	const rad = math.Pi / 180
	y := math.Sin((b.Lon-a.Lon)*rad) * math.Cos(b.Lat*rad)
	x := math.Cos(a.Lat*rad)*math.Sin(b.Lat*rad) - math.Sin(a.Lat*rad)*math.Cos(b.Lat*rad)*math.Cos((b.Lon-a.Lon)*rad)
	return math.Mod(math.Atan2(y, x)/rad+360, 360), true
}

// writeWeatherAlong writes the rows as a table in u: where and when, the
// temperature, dew point and humidity, the wind and where from, the gusts,
// the wind as met -- head, and cross from the left or right -- the
// sunlight, and the sky.
func writeWeatherAlong(w io.Writer, rows []weatherRow, u units.Set) {
	if len(rows) == 0 {
		return
	}
	t := func(v float64) string { return fmt.Sprintf("%.0f", u.Temperature.FromSI(v)) }
	v := func(ms float64) string { return fmt.Sprintf("%.0f", u.Speed.FromSI(ms)) }
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintf(tw, "elapsed\t%s\t%s\tdew\thumidity\twind %s\tfrom\tgusts\thead\tcross\tsun W/m²\tsky\t\n",
		u.Distance.Name, u.Temperature.Name, u.Speed.Name)
	for _, r := range rows {
		head, cross := "", ""
		if r.hasHeading {
			if head = fmt.Sprintf("%+.0f", u.Speed.FromSI(r.head)); head == "+0" || head == "-0" {
				head = "0"
			}
			side := "R"
			if r.cross < 0 {
				side = "L"
			}
			cross = fmt.Sprintf("%.0f %s", u.Speed.FromSI(math.Abs(r.cross)), side)
		}
		sky := ""
		if r.at.HasSky {
			sky = fw.Describe(r.at.WeatherCode)
		}
		fmt.Fprintf(tw, "%s\t%.2f\t%s\t%s\t%.0f%%\t%s\t%s\t%s\t%s\t%s\t%.0f\t%s\t\n",
			clock(r.elapsed), u.Distance.FromSI(r.distance), t(r.at.Temperature), t(r.at.DewPoint), r.at.Humidity,
			v(r.at.WindSpeed), fw.Compass(r.at.WindFrom), v(r.at.Gusts), head, cross, r.at.Radiation, sky)
	}
	tw.Flush()
	fmt.Fprintln(w, "  the wind at 10 m in the open: head is against you, negative behind; cross is from your left or right")
}

// jsonConditions are the weather at one place and moment, in SI units.
type jsonConditions struct {
	Time         time.Time `json:"time" yaml:"time"`
	TemperatureC float64   `json:"temperature_c" yaml:"temperature_c"`
	DewPointC    float64   `json:"dew_point_c" yaml:"dew_point_c"`
	HumidityPct  float64   `json:"humidity_pct" yaml:"humidity_pct"`
	WindSpeedMS  float64   `json:"wind_speed_ms" yaml:"wind_speed_ms"`
	WindFromDeg  float64   `json:"wind_from_deg" yaml:"wind_from_deg"`
	GustsMS      float64   `json:"gusts_ms" yaml:"gusts_ms"`
	PressureHPa  float64   `json:"pressure_hpa" yaml:"pressure_hpa"`
	RadiationWM2 float64   `json:"radiation_w_m2" yaml:"radiation_w_m2"`
	// The sky, where the hour had one: its WMO code and in words, what
	// fell over the hour before, in mm, the cloud cover, and whether it was
	// day.
	WeatherCode     *int     `json:"weather_code,omitempty" yaml:"weather_code,omitempty"`
	Sky             string   `json:"sky,omitempty" yaml:"sky,omitempty"`
	PrecipitationMM *float64 `json:"precipitation_mm,omitempty" yaml:"precipitation_mm,omitempty"`
	CloudCoverPct   *float64 `json:"cloud_cover_pct,omitempty" yaml:"cloud_cover_pct,omitempty"`
	Day             *bool    `json:"day,omitempty" yaml:"day,omitempty"`
}

func conditionsJSON(c fw.Conditions) jsonConditions {
	j := jsonConditions{Time: c.Time.UTC(), TemperatureC: c.Temperature, DewPointC: c.DewPoint, HumidityPct: c.Humidity,
		WindSpeedMS: c.WindSpeed, WindFromDeg: c.WindFrom, GustsMS: c.Gusts, PressureHPa: c.Pressure, RadiationWM2: c.Radiation}
	if c.HasSky {
		code, rain, cloud, day := c.WeatherCode, c.Precipitation, c.CloudCover, c.IsDay
		j.WeatherCode, j.Sky, j.PrecipitationMM, j.CloudCoverPct, j.Day = &code, fw.Describe(code), &rain, &cloud, &day
	}
	return j
}

// jsonWeatherRow is a weatherRow for JSON and YAML; the wind as met is
// left out where there was no direction to meet it from.
type jsonWeatherRow struct {
	ElapsedS       float64 `json:"elapsed_s" yaml:"elapsed_s"`
	DistanceM      float64 `json:"distance_m" yaml:"distance_m"`
	Latitude       float64 `json:"latitude" yaml:"latitude"`
	Longitude      float64 `json:"longitude" yaml:"longitude"`
	jsonConditions `yaml:",inline"`
	HeadwindMS     *float64 `json:"headwind_ms,omitempty" yaml:"headwind_ms,omitempty"`
	CrosswindMS    *float64 `json:"crosswind_ms,omitempty" yaml:"crosswind_ms,omitempty"`
}

// jsonWeatherPlace is one place's weather as fetched, hour by hour.
type jsonWeatherPlace struct {
	Latitude   float64          `json:"latitude" yaml:"latitude"`
	Longitude  float64          `json:"longitude" yaml:"longitude"`
	ElevationM float64          `json:"elevation_m" yaml:"elevation_m"`
	Hours      []jsonConditions `json:"hours" yaml:"hours"`
}

// jsonWeather is cobrakit's weather over the course, with the rows along
// it, how far or long apart they are, and with --weather-raw the places
// and hours fetched.
type jsonWeather struct {
	cw.JSON `yaml:",inline"`
	EveryM  *float64           `json:"along_every_m,omitempty" yaml:"along_every_m,omitempty"`
	EveryS  *float64           `json:"along_every_s,omitempty" yaml:"along_every_s,omitempty"`
	Along   []jsonWeatherRow   `json:"along,omitempty" yaml:"along,omitempty"`
	Places  []jsonWeatherPlace `json:"places,omitempty" yaml:"places,omitempty"`
}

// weatherJSON is the weather for JSON and YAML: j, the rows, and the
// report's places where raw; nil without j.
func weatherJSON(j *cw.JSON, rows []weatherRow, e every, r *fw.Report, raw bool) *jsonWeather {
	if j == nil {
		return nil
	}
	out := &jsonWeather{JSON: *j}
	if e.period > 0 {
		s := e.period.Seconds()
		out.EveryS = &s
	} else {
		out.EveryM = &e.metres
	}
	for _, r := range rows {
		jr := jsonWeatherRow{ElapsedS: r.elapsed.Seconds(), DistanceM: r.distance, Latitude: r.lat, Longitude: r.lon,
			jsonConditions: conditionsJSON(r.at)}
		if r.hasHeading {
			head, cross := r.head, r.cross
			jr.HeadwindMS, jr.CrosswindMS = &head, &cross
		}
		out.Along = append(out.Along, jr)
	}
	if raw && r != nil {
		for _, p := range r.Places {
			jp := jsonWeatherPlace{Latitude: p.Lat, Longitude: p.Lon, ElevationM: p.Elevation}
			for _, h := range p.Hours {
				jp.Hours = append(jp.Hours, conditionsJSON(h))
			}
			out.Places = append(out.Places, jp)
		}
	}
	return out
}
