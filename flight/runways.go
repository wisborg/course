package flight

import (
	"fmt"

	"github.com/wisborg/osmbase/locate"
	"github.com/wisborg/osmbase/mercator"
	"github.com/wisborg/osmbase/mvt"
)

const (
	// RunwayZoom is the zoom runways are read at: the zoom a summary's
	// finest levels are read at, so the map fetched round an airport to
	// name it holds its runways too.
	RunwayZoom = 14

	// RunwayReach is how far round a position, in degrees, runways are
	// looked for: an airport's worth, as far as a summary fetches round
	// the ends of a flight.
	RunwayReach = 0.03
)

// Runways reads the runways the map has within RunwayReach of a position,
// each the whole of its centre line however many tiles it crosses. A tile
// the store does not hold is skipped, so an airport the store lacks has
// no runways rather than an error.
func Runways(src locate.TileSource, lat, lon float64) ([]Runway, error) {
	x0, y0, err := mercator.TileAt(RunwayZoom, lon-RunwayReach, lat+RunwayReach)
	if err != nil {
		return nil, err
	}
	x1, y1, err := mercator.TileAt(RunwayZoom, lon+RunwayReach, lat-RunwayReach)
	if err != nil {
		return nil, err
	}
	// A runway is cut at every tile edge it crosses, and each piece carries
	// its name: the pieces of one name are put back together as the two
	// points of them furthest apart, a runway being straight.
	points := map[string][][2]float64{}
	var order []string
	for x := x0; x <= x1; x++ {
		for y := y0; y <= y1; y++ {
			raw, ok, err := src.Tile(RunwayZoom, x, y)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			tile, err := mvt.Decode(raw)
			if err != nil {
				return nil, fmt.Errorf("tile %d/%d/%d: %w", RunwayZoom, x, y, err)
			}
			layer, ok := tile.Layer("roads")
			if !ok {
				continue
			}
			tf, err := mercator.NewTileTransform(RunwayZoom, x, y, layer.Extent)
			if err != nil {
				return nil, err
			}
			for _, f := range layer.Features {
				ref, ok := runwayRef(f)
				if !ok {
					continue
				}
				if _, seen := points[ref]; !seen {
					order = append(order, ref)
				}
				for _, line := range f.Geometry.Lines {
					for _, p := range line {
						lo, la := tf.LonLat(p.X, p.Y)
						points[ref] = append(points[ref], [2]float64{la, lo})
					}
				}
			}
		}
	}
	var out []Runway
	for _, ref := range order {
		ps := points[ref]
		if len(ps) < 2 {
			continue
		}
		r, far := Runway{Ref: ref}, -1.0
		for i := range ps {
			for j := i + 1; j < len(ps); j++ {
				if d := (Runway{Lat1: ps[i][0], Lon1: ps[i][1], Lat2: ps[j][0], Lon2: ps[j][1]}).length(); d > far {
					far = d
					r.Lat1, r.Lon1, r.Lat2, r.Lon2 = ps[i][0], ps[i][1], ps[j][0], ps[j][1]
				}
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func (r Runway) length() float64 {
	x, y := local(r.Lat1, r.Lon1, r.Lat2, r.Lon2)
	return x*x + y*y
}

// runwayRef is a runway line's name, when the feature is one and has one:
// OpenStreetMap's ref, "16R/34L", which Protomaps keeps.
func runwayRef(f mvt.Feature) (string, bool) {
	if f.Type != mvt.GeomLineString {
		return "", false
	}
	kind, _ := f.Tag("kind")
	detail, _ := f.Tag("kind_detail")
	ref, _ := f.Tag("ref")
	k, _ := kind.Text()
	d, _ := detail.Text()
	r, ok := ref.Text()
	return r, k == "aeroway" && d == "runway" && ok && r != ""
}
