package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/wisborg/osmbase/acquire"
	"github.com/wisborg/osmbase/fetch"
	"github.com/wisborg/osmbase/slice"
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
