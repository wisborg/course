package summary

import (
	"context"
	"testing"
	"time"

	"github.com/wisborg/osmbase/locate"
	"github.com/wisborg/osmbase/slice"

	"github.com/wisborg/course"
)

// world answers each level it has from the longitude alone, so a test lays
// places out along a line of latitude and runs a course down it. An empty
// name is outside everything at that level. Every place here is invented.
type world map[locate.Level]func(lon float64) string

func (w world) Covers(l locate.Level) bool { _, ok := w[l]; return ok }

func (w world) Contains(l locate.Level, lat, lon float64) (string, string, string, locate.Containment) {
	if name := w[l](lon); name != "" {
		return name, "", "test", locate.Inside
	}
	return "", "", "", locate.Outside
}

// every names a place per width degrees of longitude: prefix0, prefix1, ...
func every(prefix string, width float64) func(float64) string {
	return func(lon float64) string {
		return prefix + string(rune('A'+int((lon-20)/width)%26)) + string(rune('a'+int((lon-20)/width)/26))
	}
}

func always(name string) func(float64) string { return func(float64) string { return name } }

// between names a place inside [from, to) degrees east of 20, and nothing
// outside it.
func between(name string, from, to float64) func(float64) string {
	return func(lon float64) string {
		if lon-20 >= from && lon-20 < to {
			return name
		}
		return ""
	}
}

// except is f with nothing named inside [from, to).
func except(f func(float64) string, from, to float64) func(float64) string {
	return func(lon float64) string {
		if lon-20 >= from && lon-20 < to {
			return ""
		}
		return f(lon)
	}
}

// line is a timed course east along 10°N from 20°E, n points step degrees
// apart and a second each.
func line(n int, step float64) *course.Course {
	c := &course.Course{Timed: true}
	for i := 0; i < n; i++ {
		c.Points = append(c.Points, course.Point{Lat: 10, Lon: 20 + float64(i)*step, Elapsed: time.Duration(i) * time.Second})
	}
	return c
}

// A degree of longitude at 10°N, roughly, in metres.
const degree = 109_600.0

func summarise(t *testing.T, c *course.Course, w world, o Options) *Summary {
	t.Helper()
	o.Boundaries = w
	s, err := Summarise(context.Background(), c, nil, o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func onlyLevels(w world) world {
	for _, l := range locate.Levels {
		if _, ok := w[l]; !ok {
			w[l] = always("")
		}
	}
	return w
}

// A local course's streets are its change log when they fit the budget, and
// it falls back to the next depth out when they do not.
func TestAutoTakesTheFinestDepthWithinTheBudget(t *testing.T) {
	// 5.5 km; a street every 110 m is fifty streets, a suburb every 1.1 km
	// is five.
	c := line(500, 0.0001)
	w := onlyLevels(world{
		locate.Country:       always("Land"),
		locate.Neighbourhood: every("Suburb", 0.01),
		locate.Street:        every("Street", 0.001),
	})
	s := summarise(t, c, w, Options{Auto: true})
	if s.Depth != locate.Neighbourhood || len(s.Rows) != 5 {
		t.Errorf("auto chose %s with %d rows; want neighbourhood with 5", s.Depth, len(s.Rows))
	}
	s = summarise(t, c, w, Options{Auto: true, Budget: 60})
	if s.Depth != locate.Street {
		t.Errorf("with room for every street, auto chose %s", s.Depth)
	}
	s = summarise(t, c, w, Options{Depth: locate.Street})
	if s.Auto || len(s.Rows) < 45 {
		t.Errorf("street asked for gave %d rows at %s", len(s.Rows), s.Depth)
	}
	if got := s.Rows[1].Elapsed; got < 9*time.Second || got > 12*time.Second {
		t.Errorf("the second street starts %v in; want about 10 s", got)
	}
}

// A place the course only brushes is folded away; one it comes back from
// is folded further; one it stays in is a row.
func TestFlickerIsFolded(t *testing.T) {
	c := line(300, 0.0001) // 3.3 km
	cases := []struct {
		name  string
		b     func(float64) string
		wantN int
	}{
		{"40 m of B", between("B", 0.01, 0.0104), 1},
		{"a 300 m detour into B and back", between("B", 0.01, 0.0127), 1},
		{"600 m in B and back", between("B", 0.01, 0.0155), 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := onlyLevels(world{locate.Street: func(lon float64) string {
				if n := tc.b(lon); n != "" {
					return n
				}
				return "A"
			}})
			s := summarise(t, c, w, Options{Depth: locate.Street})
			if len(s.Rows) != tc.wantN {
				t.Errorf("%d rows, want %d: %s", len(s.Rows), tc.wantN, s.OneLiner())
			}
		})
	}
}

// A gap in the country with nothing named in it -- a coastline the outlines
// missed -- is carried across, as Near. One with a sea in it is not: that is
// the course leaving the land.
func TestBridgeCarriesTheLandAcrossAGapWithNoWater(t *testing.T) {
	c := line(1000, 0.001) // 110 km
	land := except(always("Land"), 0.3, 0.5)
	s := summarise(t, c, onlyLevels(world{locate.Country: land}), Options{Depth: locate.Country})
	if len(s.Rows) != 1 {
		t.Fatalf("%d rows across a gap with nothing in it; want the one country", len(s.Rows))
	}

	sea := between("Sea", 0.3, 0.5)
	s = summarise(t, c, onlyLevels(world{locate.Country: land, locate.Water: sea}), Options{Depth: locate.Country})
	if got := s.OneLiner(); got != "Land → Sea → Land" {
		t.Errorf("a crossing of 22 km of sea reads %q", got)
	}
}

// The bridged answer says what it is: near, not inside.
func TestABridgedAnswerIsNear(t *testing.T) {
	c := line(1000, 0.001)
	// The course starts in the gap, so its first row's country is bridged
	// from the land after it, and that land's evidence is all there is.
	land := except(always("Land"), 0, 0.2)
	s := summarise(t, c, onlyLevels(world{locate.Country: land}), Options{Depth: locate.Country})
	if len(s.Rows) != 1 {
		t.Fatalf("%d rows", len(s.Rows))
	}
	m := s.Rows[0].Places[0]
	if m.Name != "Land" || m.Source != locate.Contained {
		t.Errorf("the row's country is %+v; a row is named by its best evidence, which is contained", m)
	}

	walk := track{c: c, idx: c.Thin(DefaultSpacing)}
	pts := make([]locate.Coord, len(walk.idx))
	for i, j := range walk.idx {
		pts[i] = locate.Coord{Lat: c.Points[j].Lat, Lon: c.Points[j].Lon}
	}
	w := onlyLevels(world{locate.Country: land})
	walk.places, _ = locate.AtEach(context.Background(), nil, pts, locate.Options{Boundaries: w, Levels: []locate.Level{locate.Country}})
	walk.bridge()
	first, ok := walk.places[0].Match(locate.Country)
	if !ok || first.Source != locate.Near || first.DistanceM < 20_000 {
		t.Errorf("the first point's bridged country is %+v; want Near, about 22 km from its evidence", first)
	}
}

// Named water under a course for less than minWater is the outline reaching
// onto the land, and is not named.
func TestPuddlesAreNotNamed(t *testing.T) {
	c := line(1000, 0.0001) // 11 km
	w := onlyLevels(world{locate.Country: always("Land"), locate.Water: between("Sea", 0.05, 0.055)})
	s := summarise(t, c, w, Options{Depth: locate.Country})
	for _, r := range s.Rows {
		for _, m := range r.Places {
			if m.Level == locate.Water {
				t.Errorf("550 m of water named: %+v", s.Rows)
			}
		}
	}
}

// A row on the coast is labelled by the land, and one at sea by the sea.
func TestLabelPrefersLand(t *testing.T) {
	r := Row{Places: []locate.Match{{Level: locate.Country, Name: "Land"}, {Level: locate.Water, Name: "Sea"}}}
	if r.Label() != "Land" {
		t.Errorf("coast labelled %q", r.Label())
	}
	r = Row{Places: []locate.Match{{Level: locate.Water, Name: "Sea"}}}
	if r.Label() != "Sea" {
		t.Errorf("sea labelled %q", r.Label())
	}
}

// holdings holds every tile except at the zooms listed.
type holdings map[uint8]bool

func (h holdings) HeldAt(b slice.Bounds, zoom uint8) (int, int, error) {
	if h[zoom] {
		return 0, 1, nil
	}
	return 1, 1, nil
}

// empty is a store's tiles with nothing in them.
type empty struct{}

func (empty) Tile(uint8, uint32, uint32) ([]byte, bool, error) { return nil, false, nil }

// A depth whose tiles the store does not hold along the course is not one
// auto may choose -- with no streets on disk, every street lookup is empty,
// and a change log of one row would pass for a street-level summary -- and
// it is reported as short whether chosen or not.
func TestAutoSkipsADepthTheStoreHasNoMapFor(t *testing.T) {
	c := line(200, 0.0001)
	w := world{locate.Country: always("Land"), locate.Region: always("Shire"), locate.Water: always("")}
	// Street and neighbourhood are read at 14, and the store has none of it.
	o := Options{Auto: true, Boundaries: w, Holdings: holdings{14: true}}
	s, err := Summarise(context.Background(), c, empty{}, o)
	if err != nil {
		t.Fatal(err)
	}
	if s.Depth >= locate.Neighbourhood {
		t.Errorf("auto chose %s over a store with no tiles at zoom 14", s.Depth)
	}
	short := map[locate.Level]bool{}
	for _, sh := range s.Short {
		short[sh.Level] = true
		if sh.Held != 0 || sh.Wanted == 0 || sh.Zoom != 14 {
			t.Errorf("shortfall %+v", sh)
		}
	}
	if !short[locate.Street] || !short[locate.Neighbourhood] || short[locate.Locality] {
		t.Errorf("short %v; want street and neighbourhood, and not locality, which the store holds", s.Short)
	}

	o.Auto, o.Depth = false, locate.Street
	if s, err = Summarise(context.Background(), c, empty{}, o); err != nil {
		t.Fatal(err)
	}
	if s.Depth != locate.Street || len(s.Short) == 0 {
		t.Errorf("street asked for was %s, short %v; want street, reported short", s.Depth, s.Short)
	}
}

// A plan's rows carry no elapsed time, and a course with no recorded
// distance no distance.
func TestAnUntimedCourseHasNoElapsedTime(t *testing.T) {
	c := line(100, 0.0001)
	c.Timed = false
	s := summarise(t, c, onlyLevels(world{locate.Country: always("Land")}), Options{Depth: locate.Country})
	if s.Timed || s.Rows[0].HasElapsed || s.Finish.HasElapsed || s.Measured || s.Finish.HasDistance {
		t.Errorf("summary %+v", s)
	}
}

// When even the widest depth is over the budget, the shortest stretches go
// until it fits -- but never the start or the finish.
func TestAutoFoldsToTheBudgetKeepingTheEnds(t *testing.T) {
	c := line(2000, 0.001) // 220 km
	// An archipelago: a 10 km island every 20 km, then a long last country.
	land := func(lon float64) string {
		switch x := lon - 20; {
		case x < 0.2:
			return "Start"
		case x >= 1.8:
			return "Finish"
		case int(x*10)%2 == 0:
			return "Island"
		}
		return ""
	}
	w := onlyLevels(world{locate.Country: land, locate.Water: func(lon float64) string {
		if land(lon) == "" {
			return "Sea"
		}
		return ""
	}})
	s := summarise(t, c, w, Options{Auto: true, Budget: 5})
	if len(s.Rows) > 5 || s.Depth != locate.Country {
		t.Fatalf("%d rows at %s; want at most 5 at country", len(s.Rows), s.Depth)
	}
	line := s.OneLiner()
	if first, last := s.Rows[0].Label(), s.Rows[len(s.Rows)-1].Label(); first != "Start" || last != "Finish" {
		t.Errorf("folded to %q; the start and finish must stay", line)
	}
}

// A gap longer than maxBridge is not carried across, however alike its two
// sides: sixty kilometres with no country is not a coastline's error.
func TestBridgeStopsAtMaxBridge(t *testing.T) {
	c := line(2000, 0.001) // 220 km
	land := except(always("Land"), 0.5, 1.1)
	s := summarise(t, c, onlyLevels(world{locate.Country: land}), Options{Depth: locate.Country})
	if len(s.Rows) != 3 {
		t.Errorf("%d rows across a 66 km gap; want Land, nothing, Land", len(s.Rows))
	}
}

// A level the boundaries answer needs no tiles, and is not short for want
// of them.
func TestALevelTheBoundariesAnswerIsNeverShort(t *testing.T) {
	c := line(200, 0.0001)
	w := world{locate.Country: always("Land"), locate.Locality: always("Town"), locate.Water: always("")}
	s, err := Summarise(context.Background(), c, empty{}, Options{Depth: locate.Locality, Boundaries: w, Holdings: holdings{10: true}})
	if err != nil {
		t.Fatal(err)
	}
	for _, sh := range s.Short {
		if sh.Level == locate.Locality {
			t.Errorf("locality reported short %+v, though the boundaries answer it", sh)
		}
	}
}

// A gap between two different countries is not bridged: which of them the
// course was in there is not known, and neither is carried into it.
func TestBridgeNeedsTheSameGroundOnBothSides(t *testing.T) {
	c := line(1000, 0.001)
	country := func(lon float64) string {
		switch x := lon - 20; {
		case x < 0.3:
			return "West"
		case x >= 0.5:
			return "East"
		}
		return ""
	}
	s := summarise(t, c, onlyLevels(world{locate.Country: country}), Options{Depth: locate.Country})
	if len(s.Rows) != 3 || len(s.Rows[1].Places) != 0 {
		t.Errorf("rows %+v; want West, a row naming nothing, East", s.Rows)
	}
}
