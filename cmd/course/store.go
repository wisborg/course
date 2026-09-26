package main

import (
	"errors"
	"fmt"
	iofs "io/fs"
	"strings"

	"github.com/wisborg/osmbase/boundary"
	"github.com/wisborg/osmbase/locate"
	"github.com/wisborg/osmbase/render"
	"github.com/wisborg/osmbase/slice"

	"github.com/wisborg/course/summary"
)

// store is what a command reads places and maps from.
type store struct {
	root       string
	tiles      *slice.Source // nil when the store holds no map
	manifest   slice.Manifest
	boundaries *boundary.Source // nil when it holds no outlines
}

// openStore opens the store at root, the default when empty, and the one
// archive in it -- or the one archive names, when it holds several.
//
// A store with boundaries and no map opens; what it can answer is then the
// levels the boundaries cover. One with neither is an error naming the
// commands that fill it.
func openStore(root, archive string) (*store, error) {
	if root == "" {
		var err error
		if root, err = slice.DefaultRoot(); err != nil {
			return nil, fmt.Errorf("finding the default store: %w; pass --store", err)
		}
	}
	s := &store{root: root}
	if boundary.Available(root, boundary.DefaultDetail) {
		s.boundaries = boundary.Open(root, boundary.DefaultDetail)
	}
	st, err := slice.Open(root)
	switch {
	case errors.Is(err, iofs.ErrNotExist) && s.boundaries != nil:
		return s, nil
	case errors.Is(err, iofs.ErrNotExist):
		return nil, fmt.Errorf("there is no map data at %s; fill it with \"osmbase fetch\" and \"osmbase boundaries\"", root)
	case err != nil:
		return nil, fmt.Errorf("opening the store at %s: %w", root, err)
	}
	sources, err := st.Sources()
	if err != nil {
		return nil, err
	}
	if s.manifest, err = chooseSource(root, sources, archive); err != nil {
		return nil, err
	}
	if s.tiles, err = st.Source(s.manifest.ID); err != nil {
		return nil, err
	}
	return s, nil
}

// tileSource is the tiles as locate takes them: nil, not a nil pointer in an
// interface, when there are none.
func (s *store) tileSource() locate.TileSource {
	if s.tiles == nil {
		return nil
	}
	return s.tiles
}

func (s *store) holdings() summary.Holdings {
	if s.tiles == nil {
		return nil
	}
	return s.tiles
}

func (s *store) boundarySource() locate.BoundarySource {
	if s.boundaries == nil {
		return nil
	}
	return s.boundaries
}

// credits are what the places given owe: the tile archive's credit when any
// answer came from the tiles, and each boundary file's own for the answers
// taken from it. Plain text, for a terminal and for JSON alike.
func (s *store) credits(matches []locate.Match) []string {
	var out []string
	seen := map[string]bool{}
	add := func(c string) {
		if c = render.PlainCredit(c); c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for _, m := range matches {
		switch {
		case m.Attribution != "":
			add(m.Attribution)
		case m.Source == locate.Near || m.Source == locate.Within:
			// Both are read from the tiles, and owe the archive's credit.
			add(s.manifest.Attribution)
		}
	}
	return out
}

// chooseSource is osmbase's rule: the one archive a store holds, or the one
// --archive names by ID or part of its source.
func chooseSource(root string, sources []slice.Manifest, want string) (slice.Manifest, error) {
	if len(sources) == 0 {
		return slice.Manifest{}, fmt.Errorf("the store at %s is empty; fill it with \"osmbase fetch --store %s\"", root, root)
	}
	if want == "" {
		if len(sources) == 1 {
			return sources[0], nil
		}
		return slice.Manifest{}, fmt.Errorf("the store at %s holds %d archives; name one with --archive:\n%s", root, len(sources), describe(sources))
	}
	var hits []slice.Manifest
	for _, m := range sources {
		if m.ID == want {
			return m, nil
		}
		if strings.HasPrefix(m.ID, want) || strings.Contains(strings.ToLower(m.Source), strings.ToLower(want)) {
			hits = append(hits, m)
		}
	}
	if len(hits) == 1 {
		return hits[0], nil
	}
	return slice.Manifest{}, fmt.Errorf("--archive %q matches %d of the archives at %s:\n%s", want, len(hits), root, describe(sources))
}

func describe(sources []slice.Manifest) string {
	var b strings.Builder
	for _, m := range sources {
		fmt.Fprintf(&b, "  %s  %s\n", m.ID, m.Source)
	}
	return strings.TrimRight(b.String(), "\n")
}
