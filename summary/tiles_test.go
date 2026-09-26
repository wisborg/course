package summary

import (
	"context"
	"strings"
	"testing"

	"github.com/wisborg/osmbase/locate"
	"github.com/wisborg/osmbase/mercator"
	"github.com/wisborg/osmbase/mvt"
	"github.com/wisborg/osmbase/osmbasetest"
)

// Tests that need the map tiles themselves -- areas, streets, airports --
// build them, with features placed in degrees along the same line of
// latitude the courses run down.

type tileSet map[[3]uint32][]byte

func (t tileSet) Tile(z uint8, x, y uint32) ([]byte, bool, error) {
	b, ok := t[[3]uint32{uint32(z), x, y}]
	return b, ok, nil
}

type feature struct {
	layer string
	typ   mvt.GeomType
	id    uint64
	tags  map[string]string
	pts   [][2]float64 // lon, lat
}

// tilesAt builds the zoom-14 tile holding (lon, lat) with the features, in
// that tile's coordinates.
func tilesAt(t *testing.T, lon, lat float64, feats ...feature) tileSet {
	t.Helper()
	const z = 14
	x, y, err := mercator.TileAt(z, lon, lat)
	if err != nil {
		t.Fatal(err)
	}
	n := float64(uint32(1) << z)
	layers := map[string][]osmbasetest.FeatureSpec{}
	for _, f := range feats {
		var pts []mvt.Point
		for _, p := range f.pts {
			px, py := mercator.Project(p[0], p[1])
			pts = append(pts, mvt.Point{X: int32((px*n - float64(x)) * mvt.DefaultExtent), Y: int32((py*n - float64(y)) * mvt.DefaultExtent)})
		}
		spec := osmbasetest.FeatureSpec{Type: f.typ, ID: f.id, HasID: f.id != 0}
		for k, v := range f.tags {
			spec.Tags = append(spec.Tags, osmbasetest.Tag{Key: k, Value: mvt.StringValue(v)})
		}
		switch f.typ {
		case mvt.GeomPolygon:
			spec.Geometry.Polygons = []mvt.Polygon{{Exterior: pts}}
		case mvt.GeomLineString:
			spec.Geometry.Lines = [][]mvt.Point{pts}
		default:
			spec.Geometry.Points = pts
		}
		layers[f.layer] = append(layers[f.layer], spec)
	}
	var specs []osmbasetest.LayerSpec
	for name, fs := range layers {
		specs = append(specs, osmbasetest.LayerSpec{Name: name, Features: fs})
	}
	data, err := osmbasetest.BuildTile(osmbasetest.TileSpec{Layers: specs})
	if err != nil {
		t.Fatal(err)
	}
	return tileSet{{z, x, y}: data}
}

// box is a polygon from lon0 to lon1 either side of the course's line.
func box(lon0, lon1 float64) [][2]float64 {
	return [][2]float64{{lon0, 9.999}, {lon1, 9.999}, {lon1, 10.001}, {lon0, 10.001}, {lon0, 9.999}}
}

func area(id uint64, kind, name string, lon0, lon1 float64) []feature {
	return []feature{
		{layer: "landuse", typ: mvt.GeomPolygon, id: id, tags: map[string]string{"kind": kind}, pts: box(lon0, lon1)},
		{layer: "pois", typ: mvt.GeomPoint, id: id, tags: map[string]string{"kind": kind, "name": name}, pts: [][2]float64{{(lon0 + lon1) / 2, 10}}},
	}
}

func summariseOn(t *testing.T, src tileSet, o Options) *Summary {
	t.Helper()
	// Boundaries for the country and the sea only: a level the boundaries
	// cover is not asked of the tiles.
	o.Boundaries = world{locate.Country: always("Land"), locate.Water: always("")}
	s, err := Summarise(context.Background(), line(300, 0.0001), src, o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A flight that starts inside an airport names it -- and a course that
// starts in a park does not, since a park is not where a flight takes off.
// At area depth or finer the airport is a row already, and is not named
// twice.
func TestAirportsAtTheEnds(t *testing.T) {
	src := tilesAt(t, 20.001, 10, area(1, "aerodrome", "Home Airport", 19.999, 20.005)...)
	s := summariseOn(t, src, Options{Depth: locate.Country})
	if s.Departure == nil || s.Departure.Name != "Home Airport" || s.Arrival != nil {
		t.Errorf("departure %+v, arrival %+v", s.Departure, s.Arrival)
	}
	if s = summariseOn(t, src, Options{Depth: locate.Area}); s.Departure != nil {
		t.Errorf("at area depth the airport is named again as a departure: %+v", s.Departure)
	}
	park := tilesAt(t, 20.001, 10, area(1, "park", "Home Park", 19.999, 20.005)...)
	if s = summariseOn(t, park, Options{Depth: locate.Country}); s.Departure != nil {
		t.Errorf("a park was taken for an airport: %+v", s.Departure)
	}
}

// An area holds the course, so a stretch through one is a row once it is
// longer than minHeld -- a hundred and fifty metres through a small park
// between two stretches of the same neighbourhood is where the course went.
func TestAShortStretchThroughAnAreaIsARow(t *testing.T) {
	src := tilesAt(t, 20.01, 10, area(1, "park", "Pocket Park", 20.01, 20.0114)...)
	s := summariseOn(t, src, Options{Depth: locate.Area})
	if len(s.Rows) != 3 || s.Rows[1].Label() != "Pocket Park" || s.Rows[0].Label() != "Land" {
		t.Errorf("one line %q; want the park as the middle of three rows", s.OneLiner())
	}
}

// On the street means on it: a course along an unnamed path 25 m from a road
// was not on the road.
func TestACoursePastAStreetIsNotOnIt(t *testing.T) {
	road := func(name string, lat float64) feature {
		tags := map[string]string{"kind": "minor_road"}
		if name != "" {
			tags["name"] = name
		}
		return feature{layer: "roads", typ: mvt.GeomLineString, tags: tags, pts: [][2]float64{{19.99, lat}, {20.05, lat}}}
	}
	src := tilesAt(t, 20.01, 10, road("", 10), road("Main Road", 10.000225))
	s := summariseOn(t, src, Options{Depth: locate.Street})
	for _, r := range s.Rows {
		for _, m := range r.Places {
			if m.Level == locate.Street {
				t.Fatalf("the course is on %q, 25 m off", m.Name)
			}
		}
	}
}

// The repeat covering most of the chain wins, and of those the shortest: a
// loop that contains a shorter repeat of its own is the loop, twice, and a
// lap run four times is four laps.
func TestLoopsTakeTheLongestRepeat(t *testing.T) {
	got := strings.Join(loops(strings.Fields("A B A B C A B A B C")), " → ")
	if got != "2x (A → B → A → B → C)" {
		t.Errorf("%q", got)
	}
	// Four laps of two places are four laps, not two of four.
	if got = strings.Join(loops(strings.Fields("A B A B A B A B")), " → "); got != "4x (A → B)" {
		t.Errorf("%q", got)
	}
}

// A store holding part of the map along a course names streets on that part
// only; a street depth chosen from it would describe half the course in
// streets and the rest as nothing. Auto skips a depth the store holds too
// little of, even when something along the course was named at it.
func TestAutoSkipsADepthTheStoreHoldsOnlyPartOf(t *testing.T) {
	road := feature{layer: "roads", typ: mvt.GeomLineString, tags: map[string]string{"kind": "minor_road", "name": "First Street"},
		pts: [][2]float64{{19.999, 10}, {20.005, 10}}}
	src := tilesAt(t, 20.001, 10, road)
	o := Options{Auto: true, Holdings: holdings{14: true}}
	s := summariseOn(t, src, o)
	if s.Depth >= locate.Neighbourhood {
		t.Errorf("auto chose %s from a store short of zoom 14", s.Depth)
	}
	o.Holdings = nil
	if s = summariseOn(t, src, o); s.Depth != locate.Street {
		t.Errorf("with the holdings unknown, auto chose %s; the fixture must name a street to test anything", s.Depth)
	}
}
