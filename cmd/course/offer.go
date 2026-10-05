package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wisborg/course/mapstyle"
	"github.com/wisborg/osmbase/acquire"
	"github.com/wisborg/osmbase/fetch"
	"github.com/wisborg/osmbase/render"
	"github.com/wisborg/osmbase/slice"
	"github.com/wisborg/osmbase/terrain"
)

// defaultArchive is where a store with no map yet is filled from: the public
// OpenStreetMap build osmbase and fitdash both default to, named here and in
// the help rather than being a library default nobody chose.
const defaultArchive = "https://data.source.coop/protomaps/openstreetmap/v4.pmtiles"

// stdinAnswerable is whether a person can answer; see fetch.Consent. A
// variable so a test can answer through a pipe.
var stdinAnswerable = fetch.StdinIsTerminal

// need is what a command lacks from the store: an area at a zoom.
type need struct {
	root    string
	archive string // what the store was filled from; empty for defaultArchive
	areas   []slice.Bounds
	zoom    uint8
	held    int
	wanted  int
	why     string // what it is for, in the command's words
}

// offerToFill asks whether to fetch what a command lacks, and fetches it on a
// yes. It reports whether anything was fetched.
//
// A fetch tells the archive's host which part of the map was asked about --
// here, where a course went -- so it is never done without a yes, typed or
// given with --yes, and asked only where somebody can answer. The question
// comes before anything is read, measured from the disk alone. A no, nobody
// there, or a failed download is not an error: the command carries on with
// what the store holds and says what that cost.
func offerToFill(ctx context.Context, w io.Writer, n need, yes bool) bool {
	source := n.archive
	if source == "" {
		source = defaultArchive
	}
	if n.wanted == 0 {
		fmt.Fprintf(w, "course: %s holds no map yet; %s.\n", n.root, n.why)
	} else {
		fmt.Fprintf(w, "course: %s holds %d of the %d tiles at zoom %d %s.\n", n.root, n.held, n.wanted, n.zoom, n.why)
	}
	fmt.Fprintf(w, "course: fetching them contacts %s, which learns which part of the map\n", hostOf(source))
	fmt.Fprintf(w, "course:   -- where this course went -- you asked about. Afterwards, nothing does.\n")
	consent := fetch.Consent{Yes: yes, Answerable: stdinAnswerable}
	switch consent.Ask(w, "Fetch it now? [y/N] ") {
	case fetch.Accepted:
	case fetch.Unattended:
		fmt.Fprintf(w, "course: nothing is attached to answer, so nothing was fetched; pass --yes to fetch without asking\n")
		return false
	case fetch.NoAnswer:
		fmt.Fprintf(w, "\ncourse: no answer, so nothing was fetched\n")
		return false
	default:
		fmt.Fprintf(w, "course: not fetched\n")
		return false
	}
	if err := fill(ctx, w, n, source); err != nil {
		fmt.Fprintf(w, "course: %v\ncourse: carrying on with what the store holds\n", err)
		return false
	}
	return true
}

func fill(ctx context.Context, w io.Writer, n need, source string) error {
	fmt.Fprintf(w, "course: reading %s\n", source)
	a, err := fetch.Open(source, fetch.Options{RequireVectorTiles: true})
	if err != nil {
		return err
	}
	defer a.Close()
	credit, err := a.Attribution()
	if err != nil {
		return err
	}
	for _, b := range n.areas {
		res, err := fetch.Fill(ctx, n.root, a, credit, acquire.Request{Bounds: b, MaxZoom: int(n.zoom)},
			func(p *acquire.Plan) {
				if !p.Empty() {
					fmt.Fprintf(w, "course: %d tiles, %s to download\n", p.Tiles, humanBytes(p.Transfer))
				}
			}, nil)
		if err != nil {
			return err
		}
		if res.Written > 0 {
			fmt.Fprintf(w, "course: fetched %d tiles, %s\n", res.Written, humanBytes(res.Transfer))
		}
	}
	return nil
}

func hostOf(url string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		return url
	}
	return s
}

// humanBytes is a size to the digit osmbase and fitdash report it at, so the
// three say the same number the same way.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d bytes", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTP"[exp])
}

// defaultTerrainSource is where terrain is fetched from when --terrain-source
// does not say: Mapterhorn's download host, which osmbase's own command
// defaults to as well, so the terrain store shared by the two is filled from
// one source. Named here rather than in the library, as defaultArchive is.
const defaultTerrainSource = "https://download.mapterhorn.com"

// terrainFor opens the terrain a map is shaded with, when the style asks for
// it: from the store beside the map's, or --terrain-store, after offering
// to fetch what the view lacks. It is nil when the style does not ask, and
// when there is still none after the offer -- the map is then drawn
// unshaded and the report says so, as a map with no basemap is drawn on a
// blank ground rather than refused.
func terrainFor(cmd *cobra.Command, st mapstyle.Style, mapRoot string, v render.View) (*terrain.Store, error) {
	if !st.Map.Terrain {
		if cmd.Flags().Changed("terrain-source") || cmd.Flags().Changed("terrain-store") {
			return nil, fmt.Errorf("--terrain-source and --terrain-store say where terrain is, and --terrain is what draws it; add --terrain")
		}
		return nil, nil
	}
	root := mapOpts.terrainStore
	if root == "" {
		root = terrain.Root(mapRoot)
	}
	z, _, err := v.Zoom()
	if err != nil {
		return nil, err
	}
	if s, short := terrain.Measure(root, boundsOf(v), z); short {
		offerTerrain(cmd.Context(), cmd.ErrOrStderr(), s, mapOpts.terrainSource, mapOpts.yes)
	}
	ts, err := terrain.Open(root)
	if errors.Is(err, terrain.ErrNoTerrain) {
		return nil, nil
	}
	return ts, err
}

// offerTerrain asks whether to fetch the terrain a map lacks, and fetches it
// on a yes, on offerToFill's terms: measured from the disk, asked before
// anything is requested -- the list of archives included -- and a no or a
// failure only costs the shading.
func offerTerrain(ctx context.Context, w io.Writer, s terrain.Shortfall, source string, yes bool) {
	if s.Empty {
		fmt.Fprintf(w, "course: %s holds no terrain yet, to shade the map with.\n", s.Root)
	} else {
		fmt.Fprintf(w, "course: %s holds %d of the %d terrain tiles at zoom %d this map needs.\n", s.Root, s.Held, s.Wanted, s.Zoom)
	}
	if strings.Contains(source, "://") {
		fmt.Fprintf(w, "course: fetching them contacts %s, which learns which part of the map\n", hostOf(source))
		fmt.Fprintf(w, "course:   -- where this course went -- you asked about. Afterwards, nothing does.\n")
	} else {
		fmt.Fprintf(w, "course: they would be copied from %s, contacting nobody.\n", source)
	}
	switch (fetch.Consent{Yes: yes, Answerable: stdinAnswerable}).Ask(w, "Fetch the terrain now? [y/N] ") {
	case fetch.Accepted:
	case fetch.Unattended:
		fmt.Fprintf(w, "course: nothing is attached to answer, so no terrain was fetched; pass --yes to fetch without asking\n")
		return
	case fetch.NoAnswer:
		fmt.Fprintf(w, "\ncourse: no answer, so no terrain was fetched\n")
		return
	default:
		fmt.Fprintf(w, "course: no terrain fetched\n")
		return
	}
	l, err := terrain.Locate(ctx, source, func(index string) {
		fmt.Fprintf(w, "course: reading the list of terrain archives at %s\n", index)
	})
	if err == nil {
		var res terrain.Result
		res, err = terrain.Fill(ctx, l, s.Root, s.Bounds, s.MapZoom(),
			func(src string) (*fetch.Archive, error) { return fetch.Open(src, fetch.Options{}) },
			func(p *terrain.Plan) {
				if t := p.Totals(); !p.Empty() {
					fmt.Fprintf(w, "course: %d terrain tiles, %s to download\n", t.Tiles, humanBytes(t.Transfer))
				}
			}, nil)
		if err == nil && res.Written > 0 {
			fmt.Fprintf(w, "course: fetched %d terrain tiles, %s\n", res.Written, humanBytes(res.Transfer))
		}
	}
	if err != nil {
		fmt.Fprintf(w, "course: %v\ncourse: carrying on with what the terrain store holds\n", err)
	}
}
