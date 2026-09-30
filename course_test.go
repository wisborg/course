package course

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wisborg/fitactivity"
	"github.com/wisborg/fitactivity/fittest"
)

// Every course here is invented, near 10°N 20°E.

var base = time.Date(2026, 4, 2, 6, 0, 0, 0, time.UTC)

func timed(lat, lon float64, s int) fix {
	return fix{Point: Point{Lat: lat, Lon: lon}, at: base.Add(time.Duration(s) * time.Second), timed: true}
}

func lats(ps []Point) []float64 {
	var out []float64
	for _, p := range ps {
		out = append(out, p.Lat)
	}
	return out
}

// A fix at exactly 0,0 is "no fix", and a spike -- a jump the course comes
// straight back from -- is a receiver's error. Both go, and nothing else.
func TestKeepPlausibleDropsNullIslandAndSpikes(t *testing.T) {
	fixes := []fix{
		timed(10, 20, 0),
		timed(0, 0, 1),
		timed(10.0001, 20, 2),
		timed(11, 20, 3), // 110 km in a second, and back
		timed(10.0002, 20, 4),
	}
	kept, dropped := keepPlausible(fixes)
	if got := lats(kept); dropped != 2 || len(got) != 3 || got[2] != 10.0002 {
		t.Errorf("kept %v, dropped %d; want the 0,0 and the spike gone", got, dropped)
	}
}

// A jump the course does not come back from is the course moving -- a
// recording resumed somewhere else -- and everything after it is kept.
func TestKeepPlausibleKeepsAJumpThatStays(t *testing.T) {
	fixes := []fix{timed(10, 20, 0), timed(10.0001, 20, 1)}
	for s := 2; s < 20; s++ {
		fixes = append(fixes, timed(12+float64(s)*0.0001, 20, s))
	}
	kept, dropped := keepPlausible(fixes)
	if dropped != 0 || len(kept) != len(fixes) {
		t.Errorf("kept %d of %d, dropped %d; a jump the course stayed at was taken for a spike", len(kept), len(fixes), dropped)
	}
}

// A plan has no clock to judge a jump by: only 0,0 is taken out of it.
func TestKeepPlausibleLeavesAPlansJumps(t *testing.T) {
	fixes := []fix{{Point: Point{Lat: 10, Lon: 20}}, {Point: Point{Lat: 11, Lon: 20}}, {Point: Point{Lat: 0, Lon: 0}}, {Point: Point{Lat: 10, Lon: 20}}}
	kept, dropped := keepPlausible(fixes)
	if dropped != 1 || len(kept) != 3 {
		t.Errorf("kept %v, dropped %d", lats(kept), dropped)
	}
}

// Thinning keeps the first and last, and points spacing apart from the last
// kept; a standstill is one point.
func TestThin(t *testing.T) {
	c := &Course{}
	for i := 0; i < 10; i++ {
		c.Points = append(c.Points, Point{Lat: 10, Lon: 20}) // standing still
	}
	for i := 1; i <= 100; i++ {
		c.Points = append(c.Points, Point{Lat: 10 + float64(i)*0.00001, Lon: 20}) // ~1.1 m apart
	}
	idx := c.Thin(20)
	if idx[0] != 0 || idx[len(idx)-1] != len(c.Points)-1 {
		t.Fatalf("thinned to %v; the ends must stay", idx)
	}
	if len(idx) < 5 || len(idx) > 8 {
		t.Errorf("110 m thinned at 20 m to %d points", len(idx))
	}
	for i := 1; i < len(idx)-1; i++ {
		a, b := c.Points[idx[i-1]], c.Points[idx[i]]
		if d := Metres(a.Lat, a.Lon, b.Lat, b.Lon); d < 20 {
			t.Errorf("points %d and %d kept %.1f m apart", idx[i-1], idx[i], d)
		}
	}
}

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// A recording is timed, with its elapsed time; a plan is not, and has none.
// Distance is the device's where it recorded one, and absent where not.
func TestRead(t *testing.T) {
	fit := filepath.Join(t.TempDir(), "run.fit")
	opts := fittest.DefaultOptions()
	opts.Count = 60
	if err := fittest.WriteFile(fit, opts); err != nil {
		t.Fatal(err)
	}
	c, err := Read(fit)
	if err != nil {
		t.Fatal(err)
	}
	last := c.Points[len(c.Points)-1]
	if !c.Timed || last.Elapsed != 59*time.Second || !last.HasDistance || last.Distance <= 0 {
		t.Errorf("FIT course: timed %v, last point %+v", c.Timed, last)
	}
	if !c.HasElevationTotals || c.TotalAscent != float64(opts.TotalAscent) || c.TotalDescent != float64(opts.TotalDescent) {
		t.Errorf("FIT course: device totals %v %v/%v, want %v/%v", c.HasElevationTotals, c.TotalAscent, c.TotalDescent, opts.TotalAscent, opts.TotalDescent)
	}

	gpx := write(t, "run.gpx", `<gpx><trk><trkseg>
		<trkpt lat="10" lon="20"><time>2026-04-02T06:00:00Z</time></trkpt>
		<trkpt lat="10.001" lon="20"><time>2026-04-02T06:00:30Z</time></trkpt>
		</trkseg></trk></gpx>`)
	if c, err = Read(gpx); err != nil {
		t.Fatal(err)
	}
	if !c.Timed || c.Points[1].Elapsed != 30*time.Second || c.Points[1].HasDistance || c.HasElevationTotals {
		t.Errorf("GPX course: timed %v, %+v; want 30 s in and no distance", c.Timed, c.Points[1])
	}

	plan := write(t, "plan.gpx", `<gpx><rte><rtept lat="10" lon="20"/><rtept lat="10.001" lon="20"/></rte></gpx>`)
	if c, err = Read(plan); err != nil {
		t.Fatal(err)
	}
	if c.Timed || len(c.Points) != 2 {
		t.Errorf("plan: timed %v with %d points", c.Timed, len(c.Points))
	}
	if _, err := Read(plan, gpx); err == nil {
		t.Error("a plan was merged with a recording")
	}
}

// A recording's heart rate and both its power readings are kept, and Power
// chooses between them by fitactivity's rule: auto takes the footpod's
// where there is one and the standard field otherwise; the forced sources
// never substitute the other; and no reading is no power, not 0 W.
func TestPointPower(t *testing.T) {
	native := Point{HasNativePower: true, NativePower: 250}
	stryd := Point{HasStrydPower: true, StrydPower: 200}
	both := Point{HasNativePower: true, NativePower: 250, HasStrydPower: true, StrydPower: 200}
	standing := Point{HasNativePower: true, NativePower: 0}
	for _, c := range []struct {
		p    Point
		src  fitactivity.PowerSource
		want float64
		ok   bool
	}{
		{both, fitactivity.PowerAuto, 200, true},
		{both, fitactivity.PowerStryd, 200, true},
		{both, fitactivity.PowerNative, 250, true},
		{native, fitactivity.PowerAuto, 250, true},
		{native, fitactivity.PowerStryd, 0, false},
		{stryd, fitactivity.PowerNative, 0, false},
		{stryd, fitactivity.PowerAuto, 200, true},
		{Point{}, fitactivity.PowerAuto, 0, false},
		{standing, fitactivity.PowerAuto, 0, true},
	} {
		if got, ok := c.p.Power(c.src); got != c.want || ok != c.ok {
			t.Errorf("%+v.Power(%v) = %v, %v; want %v, %v", c.p, c.src, got, ok, c.want, c.ok)
		}
	}
}

func TestReadHeartRateAndPower(t *testing.T) {
	fit := filepath.Join(t.TempDir(), "run.fit")
	opts := fittest.DefaultOptions()
	opts.Count, opts.PowerWatts, opts.DeveloperField = 60, 250, fitactivity.StrydPowerField
	if err := fittest.WriteFile(fit, opts); err != nil {
		t.Fatal(err)
	}
	c, err := Read(fit)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Points[10]
	if !p.HasHeartRate || p.HeartRate < 130 || p.HeartRate > 150 {
		t.Errorf("heart rate %v %v, want the fixture's 130-150", p.HasHeartRate, p.HeartRate)
	}
	if !p.HasNativePower || p.NativePower != 250 || !p.HasStrydPower || p.StrydPower != float64(fittest.DeveloperFieldRaw(10)) {
		t.Errorf("power: native %v %v, Stryd %v %v; want 250 and %v", p.HasNativePower, p.NativePower, p.HasStrydPower, p.StrydPower, fittest.DeveloperFieldRaw(10))
	}

	gpx := write(t, "run.gpx", `<gpx><trk><trkseg>
		<trkpt lat="10" lon="20"><time>2026-04-02T06:00:00Z</time></trkpt>
		<trkpt lat="10.001" lon="20"><time>2026-04-02T06:00:30Z</time></trkpt>
		</trkseg></trk></gpx>`)
	if c, err = Read(gpx); err != nil {
		t.Fatal(err)
	}
	if p := c.Points[1]; p.HasHeartRate || p.HasNativePower || p.HasStrydPower {
		t.Errorf("a GPX with no sensors has %+v", p)
	}
}
