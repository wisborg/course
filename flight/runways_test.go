package flight

import (
	"math"
	"testing"

	"github.com/wisborg/osmbase/mercator"
	"github.com/wisborg/osmbase/mvt"
	"github.com/wisborg/osmbase/osmbasetest"
)

// tiles is a store of tiles at RunwayZoom, by x and y.
type tiles map[[2]uint32][]byte

func (t tiles) Tile(z uint8, x, y uint32) ([]byte, bool, error) {
	if z != RunwayZoom {
		return nil, false, nil
	}
	d, ok := t[[2]uint32{x, y}]
	return d, ok, nil
}

func roadsTile(t *testing.T, fs ...osmbasetest.FeatureSpec) []byte {
	t.Helper()
	b, err := osmbasetest.BuildTile(osmbasetest.TileSpec{Layers: []osmbasetest.LayerSpec{{Name: "roads", Extent: 4096, Features: fs}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func line(kind, detail, ref string, pts ...mvt.Point) osmbasetest.FeatureSpec {
	tags := []osmbasetest.Tag{
		{Key: "kind", Value: mvt.StringValue(kind)},
		{Key: "kind_detail", Value: mvt.StringValue(detail)},
	}
	if ref != "" {
		tags = append(tags, osmbasetest.Tag{Key: "ref", Value: mvt.StringValue(ref)})
	}
	return osmbasetest.FeatureSpec{Type: mvt.GeomLineString, Tags: tags, Geometry: mvt.Geometry{Lines: [][]mvt.Point{pts}}}
}

func TestRunways_ARunwayCutAtATileEdgeIsWholeAgain(t *testing.T) {
	x, y, err := mercator.TileAt(RunwayZoom, 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	// One runway across the edge between two tiles, each piece carrying a
	// buffer past the edge as a real tile's does; a taxiway beside it, and
	// a runway with no name, which cannot be told from any other.
	src := tiles{
		{x, y}: roadsTile(t,
			line("aeroway", "runway", "09/27", mvt.Point{X: 2000, Y: 2048}, mvt.Point{X: 4160, Y: 2048}),
			line("aeroway", "taxiway", "A", mvt.Point{X: 2000, Y: 2100}, mvt.Point{X: 4000, Y: 2100}),
			line("aeroway", "runway", "", mvt.Point{X: 100, Y: 100}, mvt.Point{X: 100, Y: 900}),
		),
		{x + 1, y}: roadsTile(t,
			line("aeroway", "runway", "09/27", mvt.Point{X: -64, Y: 2048}, mvt.Point{X: 1000, Y: 2048}),
		),
	}
	rs, err := Runways(src, 10, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Ref != "09/27" {
		t.Fatalf("runways %+v, want 09/27 alone", rs)
	}
	t0, _ := mercator.NewTileTransform(RunwayZoom, x, y, 4096)
	t1, _ := mercator.NewTileTransform(RunwayZoom, x+1, y, 4096)
	wLon, wLat := t0.LonLat(2000, 2048)
	eLon, eLat := t1.LonLat(1000, 2048)
	r := rs[0]
	west, east := math.Min(r.Lon1, r.Lon2), math.Max(r.Lon1, r.Lon2)
	if math.Abs(west-wLon) > 1e-9 || math.Abs(east-eLon) > 1e-9 || math.Abs(r.Lat1-wLat) > 1e-9 || math.Abs(r.Lat2-eLat) > 1e-9 {
		t.Errorf("runway %+v, want from %v,%v to %v,%v", r, wLat, wLon, eLat, eLon)
	}
}

func TestRunways_AnAirportTheStoreLacksHasNone(t *testing.T) {
	rs, err := Runways(tiles{}, 10, 20)
	if err != nil || len(rs) != 0 {
		t.Errorf("runways %+v, %v; want none and no error", rs, err)
	}
}
