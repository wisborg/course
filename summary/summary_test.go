package summary

import (
	"context"
	"strings"
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

// A local course's streets are its change log when they fit in MaxRows, and
// it falls back to the next depth out when they do not.
func TestAutoTakesTheFinestDepthWithinMaxRows(t *testing.T) {
	// 5.5 km; a street every 165 m is thirty-three streets, a suburb every
	// 1.1 km is five.
	c := line(500, 0.0001)
	w := onlyLevels(world{
		locate.Country:       always("Land"),
		locate.Neighbourhood: every("Suburb", 0.01),
		locate.Street:        every("Street", 0.0015),
	})
	s := summarise(t, c, w, Options{Auto: true})
	if s.Depth != locate.Neighbourhood || len(s.Rows) != 5 {
		t.Errorf("auto chose %s with %d rows; want neighbourhood with 5", s.Depth, len(s.Rows))
	}
	s = summarise(t, c, w, Options{Auto: true, MaxRows: 60})
	if s.Depth != locate.Street {
		t.Errorf("with room for every street, auto chose %s", s.Depth)
	}
	s = summarise(t, c, w, Options{Depth: locate.Street})
	if s.Auto || len(s.Rows) < 30 {
		t.Errorf("street asked for gave %d rows at %s", len(s.Rows), s.Depth)
	}
	if got := s.Rows[1].Elapsed; got < 13*time.Second || got > 17*time.Second {
		t.Errorf("the second street starts %v in; want about 15 s", got)
	}
}

// A place that holds the course is a row once the course is in it for
// minHeld; a shorter crossing is the GPS wobbling over the line. A detour
// into it and back counts like any other stretch: the course went there.
func TestCrossingsOfAnOutlineAreKept(t *testing.T) {
	c := line(300, 0.0001) // 3.3 km
	cases := []struct {
		name  string
		b     func(float64) string
		wantN int
	}{
		{"40 m of B", between("B", 0.01, 0.0104), 1},
		{"a 150 m detour into B and back", between("B", 0.01, 0.0114), 3},
		{"600 m in B and back", between("B", 0.01, 0.0155), 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := onlyLevels(world{locate.Neighbourhood: func(lon float64) string {
				if n := tc.b(lon); n != "" {
					return n
				}
				return "A"
			}})
			s := summarise(t, c, w, Options{Depth: locate.Neighbourhood})
			if len(s.Rows) != tc.wantN {
				t.Errorf("%d rows, want %d: %s", len(s.Rows), tc.wantN, s.OneLiner())
			}
		})
	}
}

// A NEAREST answer's flicker is folded harder: a stretch under the depth's
// minimum, and a detour back to where it came from under the larger one.
func TestNearestFlickerIsFolded(t *testing.T) {
	near := func(lengths ...float64) []run {
		keys := []string{"A", "B", "A", "C"}
		var rs []run
		for i, l := range lengths {
			rs = append(rs, run{first: i, last: i, key: keys[i%len(keys)], length: l})
		}
		return rs
	}
	least, detour := minRun(locate.Street)
	for _, tc := range []struct {
		name    string
		lengths []float64
		want    int
	}{
		{"a 300 m detour into B and back to A", []float64{1000, 300, 1000}, 1},
		{"a 30 m brush of B on the way to... A", []float64{1000, 30, 1000}, 1},
		{"a 500 m detour, over the limit", []float64{1000, 500, 1000}, 3},
		{"long stretches all stay", []float64{1000, 1000, 1000, 1000}, 4},
	} {
		if got := fold(near(tc.lengths...), least, detour, 0); len(got) != tc.want {
			t.Errorf("%s: %d runs, want %d", tc.name, len(got), tc.want)
		}
	}
	held := near(1000, 300, 1000)
	held[1].contained = true
	if got := fold(held, least, detour, 0); len(got) != 3 {
		t.Errorf("a 300 m crossing into a place that holds the course was folded")
	}
}

// Auto does not choose a depth that names nothing at its own level: with no
// region answered anywhere, "region" is the country log under another name.
func TestAutoSkipsADepthThatNamesNothing(t *testing.T) {
	c := line(200, 0.0001)
	w := onlyLevels(world{locate.Country: always("Land"), locate.Locality: always("Town")})
	s := summarise(t, c, w, Options{Auto: true})
	if s.Depth != locate.Locality {
		t.Errorf("auto chose %s; want locality, the finest that names anything", s.Depth)
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

// When even the widest depth is over MaxRows, the shortest stretches go
// until it fits -- but never the start or the finish.
func TestAutoFoldsToMaxRowsKeepingTheEnds(t *testing.T) {
	c := line(2000, 0.001) // 220 km
	// An archipelago: 22 km islands, each its own, between 22 km of sea --
	// too long to fold as detours -- then a last country.
	land := func(lon float64) string {
		switch x := lon - 20; {
		case x < 0.2:
			return "Start"
		case x >= 1.8:
			return "Finish"
		case int(x*5)%2 == 0:
			return "Island " + string(rune('A'+int(x*5)))
		}
		return ""
	}
	w := onlyLevels(world{locate.Country: land, locate.Water: func(lon float64) string {
		if land(lon) == "" {
			return "Sea"
		}
		return ""
	}})
	if s := summarise(t, c, w, Options{Auto: true, MaxRows: 50}); len(s.Rows) <= 5 {
		t.Fatalf("the fixture is %d rows unfolded; it must be over 5 to test anything", len(s.Rows))
	}
	s := summarise(t, c, w, Options{Auto: true, MaxRows: 5})
	if len(s.Rows) > 5 || s.Depth != locate.Country {
		t.Fatalf("%d rows at %s; want at most 5 at country", len(s.Rows), s.Depth)
	}
	if first, last := s.Rows[0].Label(), s.Rows[len(s.Rows)-1].Label(); first != "Start" || last != "Finish" {
		t.Errorf("folded to %q; the start and finish must stay", s.OneLiner())
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

// A loop run more than once is written once, with the count; the longest
// repeat at each place is taken, and the rest of the chain is left alone.
func TestLoopsAreWrittenOnce(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"A B C A B C D", "2x (A → B → C) → D"},
		{"S A B C A B C A B C", "S → 3x (A → B → C)"},
		{"A B A C", "A → B → A → C"},
		{"A B A B", "2x (A → B)"},
		{"A", "A"},
	} {
		if got := strings.Join(loops(strings.Fields(tc.in)), " → "); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The one-liner puts the prefix ahead of the chain, and the airports beside
// the first and last places -- the arrival at the end even when the course
// was in one place throughout.
func TestOneLinerDressing(t *testing.T) {
	row := func(name string) Row { return Row{Places: []locate.Match{{Level: locate.Region, Name: name}}} }
	s := &Summary{Prefix: "Sydney", Rows: []Row{row("A"), row("B")}}
	if got := s.OneLiner(); got != "Sydney: A → B" {
		t.Errorf("prefix: %q", got)
	}
	s = &Summary{Rows: []Row{row("Shire")}, Arrival: &locate.Match{Name: "Big Airport"}}
	if got := s.OneLiner(); got != "Shire → Big Airport, Shire" {
		t.Errorf("arrival in one place: %q", got)
	}
	s = &Summary{Rows: []Row{row("A"), row("Sea"), row("B")},
		Departure: &locate.Match{Name: "Home Airport"}, Arrival: &locate.Match{Name: "Away Airport"}}
	if got := s.OneLiner(); got != "Home Airport, A → Sea → Away Airport, B" {
		t.Errorf("flight: %q", got)
	}
}

// The prefix is the one locality the whole course was in, and only when the
// summary is finer than a locality; two localities along it, none.
func TestPrefixIsTheOnePlaceTheCourseWasIn(t *testing.T) {
	c := line(200, 0.0001)
	w := onlyLevels(world{locate.Country: always("Land"), locate.Locality: always("Town"), locate.Neighbourhood: every("Suburb", 0.01)})
	s := summarise(t, c, w, Options{Depth: locate.Neighbourhood, Prefix: PrefixLocality})
	if s.Prefix != "Town" {
		t.Errorf("prefix %q, want Town", s.Prefix)
	}
	if s = summarise(t, c, w, Options{Depth: locate.Locality, Prefix: PrefixLocality}); s.Prefix != "" {
		t.Errorf("a locality-depth summary has prefix %q", s.Prefix)
	}
	if s = summarise(t, c, w, Options{Depth: locate.Neighbourhood, Prefix: PrefixNone}); s.Prefix != "" {
		t.Errorf("--prefix none gave %q", s.Prefix)
	}
	w[locate.Locality] = every("Town", 0.01)
	if s = summarise(t, c, w, Options{Depth: locate.Neighbourhood, Prefix: PrefixLocality}); s.Prefix != "" {
		t.Errorf("a course through two towns has prefix %q", s.Prefix)
	}

	// A city's mapped extent is the city prefix, ahead of any label.
	w[locate.City] = always("Metropolis")
	if s = summarise(t, c, w, Options{Depth: locate.Neighbourhood, Prefix: PrefixCity}); s.Prefix != "Metropolis" {
		t.Errorf("city prefix %q, want the city holding the course", s.Prefix)
	}
	w[locate.City] = every("Metropolis", 0.01)
	if s = summarise(t, c, w, Options{Depth: locate.Neighbourhood, Prefix: PrefixCity}); s.Prefix != "" {
		t.Errorf("a course through two cities has city prefix %q", s.Prefix)
	}

	// A course that is nine tenths in one town and a tenth on top of a
	// district's label is in the town.
	w[locate.Locality] = func(lon float64) string {
		if lon-20 < 0.0018 {
			return "District"
		}
		return "Town"
	}
	if s = summarise(t, c, w, Options{Depth: locate.Neighbourhood, Prefix: PrefixLocality}); s.Prefix != "Town" {
		t.Errorf("prefix %q for a course nine tenths in one town", s.Prefix)
	}
}

// At street depth, a stretch on no named way and in no area is named by its
// suburbs when it is long enough, or in a suburb other than the one the
// chain was last in, and left out otherwise; at area depth or wider, or when
// no row names a street or an area at all, every row is named as before.
func TestOneLinerNamesAStretchOnNoNamedWayOnlyWhenItSaysSomething(t *testing.T) {
	sub := func(n string) locate.Match { return locate.Match{Level: locate.Neighbourhood, Name: n} }
	street := func(n string) locate.Match { return locate.Match{Level: locate.Street, Name: n} }
	row := func(length float64, ms ...locate.Match) Row { return Row{Places: ms, Length: length} }
	for _, tc := range []struct {
		name string
		rows []Row
		want string
	}{
		{"a short stretch in the same suburb is a gap",
			[]Row{row(100, sub("Home"), street("First Street")), row(261, sub("Home")), row(100, sub("Home"), street("Second Street"))},
			"First Street → Second Street"},
		{"a long stretch in the same suburb is named",
			[]Row{row(100, sub("Home"), street("First Street")), row(1012, sub("Home")), row(100, sub("Home"), street("Second Street"))},
			"First Street → Home → Second Street"},
		{"a short stretch in another suburb is named",
			[]Row{row(100, sub("Home"), street("First Street")), row(130, sub("Away")), row(100, sub("Home"), street("Second Street"))},
			"First Street → Away → Second Street"},
		{"a stretch across several suburbs is measured whole and named in order",
			[]Row{row(100, sub("Home"), street("First Street")), row(150, sub("Home")), row(200, sub("Home")),
				row(100, sub("Home"), street("Second Street"))},
			"First Street → Home → Second Street"},
		{"a stretch at the start compares with the street after it",
			[]Row{row(100, sub("Home")), row(100, sub("Home"), street("First Street"))},
			"First Street"},
	} {
		s := &Summary{Depth: locate.Street, Rows: tc.rows}
		if got := s.OneLiner(); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}

	s := &Summary{Depth: locate.Street, SuburbGap: 2000, Rows: []Row{
		row(100, sub("Home"), street("First Street")), row(1012, sub("Home")), row(100, sub("Home"), street("Second Street"))}}
	if got := s.OneLiner(); got != "First Street → Second Street" {
		t.Errorf("with a 2 km gap, a kilometre in the same suburb is named: %q", got)
	}
	s = &Summary{Depth: locate.Street, Rows: []Row{row(100, sub("Home")), row(100, sub("Home"))}}
	if got := s.OneLiner(); got != "Home" {
		t.Errorf("street depth with no street at all: %q", got)
	}
	s = &Summary{Depth: locate.Area, Rows: []Row{row(100, sub("Home")), row(100, sub("Home"), locate.Match{Level: locate.Area, Name: "The Park"}), row(100, sub("Away"))}}
	if got := s.OneLiner(); got != "Home → The Park → Away" {
		t.Errorf("area depth, the suburbs either side of a park: %q", got)
	}
}

// Each row carries how far the course went in its place.
func TestRowsCarryTheirLength(t *testing.T) {
	c := line(300, 0.0001) // 3.3 km; a suburb every 1.1 km
	s := summarise(t, c, onlyLevels(world{locate.Neighbourhood: every("Suburb", 0.01)}), Options{Depth: locate.Neighbourhood})
	if len(s.Rows) != 3 {
		t.Fatalf("%d rows", len(s.Rows))
	}
	total := 0.0
	for _, r := range s.Rows {
		total += r.Length
	}
	if got := s.Rows[1].Length; got < 1000 || got > 1200 {
		t.Errorf("the middle suburb is %.0f m long; want about 1100", got)
	}
	if total < 3200 || total > 3400 {
		t.Errorf("the rows add up to %.0f m of a 3.3 km course", total)
	}
}
