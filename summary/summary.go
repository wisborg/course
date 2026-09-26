// Package summary says where a course went: a change log of the places it
// passed through, and a one-line chain of them.
//
// The places come from osmbase's locate, answered from data already on this
// machine. Nothing about the course is sent anywhere.
package summary

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/wisborg/osmbase/locate"
	"github.com/wisborg/osmbase/mercator"
	"github.com/wisborg/osmbase/slice"

	"github.com/wisborg/course"
)

// Depths are the levels a summary can be reported at, widest first. Water is
// not one: it is reported at every depth, because a course over the sea is
// over the sea whatever depth the land beside it is named to.
var Depths = []locate.Level{locate.Country, locate.Region, locate.City, locate.Locality, locate.Macrohood, locate.Neighbourhood, locate.Area, locate.Street}

// StreetReachM is how far a course may be from a street, in metres, to be on
// it. osmbase's default reach is for a coordinate asked about on its own,
// where the nearest street two hundred metres off is worth knowing; a course
// on a path through a park is not on that street, and naming it said the
// course went somewhere it did not. See also locate.Options.OnWay, which
// decides between a street and the unnamed path beside it.
const StreetReachM = 30.0

// DefaultMaxRows is how many rows an automatic depth may produce.
const DefaultMaxRows = 25

// DefaultSpacing is how far apart, in metres, the points looked up are.
//
// Close enough that a street a course runs along for a block is seen, and far
// enough that a marathon is a couple of thousand lookups rather than sixteen
// thousand. The cost of a lookup is the tile, not the point, so this could be
// much denser for little more; it is set by what a place name can resolve.
const DefaultSpacing = 20.0

// Options configure Summarise. The zero value summarises automatically.
type Options struct {
	// Depth is the finest level reported; zero value with Auto set, or Auto
	// alone, chooses it.
	Depth locate.Level
	// Auto chooses the finest depth whose change log has no more than
	// MaxRows rows.
	Auto bool
	// MaxRows is the most rows Auto may produce; 0 is DefaultMaxRows.
	MaxRows int
	// Spacing is the distance between points looked up; 0 is DefaultSpacing.
	Spacing float64
	// Language prefers names in a language; see locate.Options.Language.
	Language string
	// Boundaries answers levels by containment; see locate.Options.
	Boundaries locate.BoundarySource
	// Holdings says which tiles the store holds; nil takes it that it holds
	// every one. A store's slice.Source is one.
	Holdings Holdings
	// Prefix is where the one-liner's leading "Sydney:" comes from.
	Prefix Prefix
}

// Prefix is where a summary finer than a locality takes the place it names
// ahead of the chain: "Sydney: Woolloomooloo → Darlinghurst".
type Prefix int

const (
	// PrefixCity is the city the course was in: the city level's answer,
	// from a city's mapped extent, where the boundaries have one -- and where
	// they do not, the place whose reach the course is most within
	// (locate.Options.Prominent). A run at Western Sydney's airport is in
	// Sydney either way, though Penrith's label is nearer.
	PrefixCity Prefix = iota
	// PrefixLocality is the locality level's own answer. Where suburb
	// outlines answer it, that is the local council.
	PrefixLocality
	// PrefixNone names nothing ahead of the chain.
	PrefixNone
)

// Prefixes are the prefix modes by name, for a command line.
var Prefixes = map[string]Prefix{"city": PrefixCity, "locality": PrefixLocality, "none": PrefixNone}

// Holdings is what a store can say about the tiles it holds, without reading
// any of them.
type Holdings interface {
	HeldAt(b slice.Bounds, zoom uint8) (held, wanted int, err error)
}

// DefaultHeld is the share of the tiles along a course a store must hold for
// a level read from them to be one an automatic depth can choose.
const DefaultHeld = 0.9

// Shortfall is a level the store does not hold the map for along the
// course: Held of the Wanted tiles at Zoom it is read at.
type Shortfall struct {
	Level  locate.Level `json:"level"`
	Zoom   uint8        `json:"zoom"`
	Held   int          `json:"held"`
	Wanted int          `json:"wanted"`
}

// Summary is where a course went, at one depth.
type Summary struct {
	// Depth is the finest level reported, and Auto whether it was chosen.
	Depth locate.Level `json:"depth"`
	Auto  bool         `json:"depth_auto"`
	// Timed and Measured say whether the rows carry an elapsed time and a
	// distance: whether the course recorded them.
	Timed    bool `json:"timed"`
	Measured bool `json:"measured"`
	// Rows are the changes, the first being the start.
	Rows []Row `json:"rows"`
	// Finish is where the course ended: its elapsed time and distance.
	Finish Mark `json:"finish"`
	// Prefix is the one place the whole course was in, named ahead of the
	// chain when the depth is finer than a locality; empty when there is no
	// one place, or the depth is a locality or wider.
	Prefix string `json:"prefix,omitempty"`
	// Departure and Arrival are the airports a course starts and ends in,
	// when it does and the depth is too wide to name them otherwise: a
	// flight summarised by countries still says where it took off.
	Departure *locate.Match `json:"departure,omitempty"`
	Arrival   *locate.Match `json:"arrival,omitempty"`
	// Short are the levels the store lacks the map for along the course,
	// among those this summary asked about. An automatic depth is never one
	// of them or finer; a chosen one may be, and its rows then name only
	// what the store held.
	Short []Shortfall `json:"short,omitempty"`
}

// Mark is how far into the course something happened, in whichever terms the
// course recorded.
type Mark struct {
	HasElapsed  bool          `json:"-"`
	Elapsed     time.Duration `json:"-"`
	HasDistance bool          `json:"-"`
	Distance    float64       `json:"-"`
}

// Row is one change: where the course was from this point on.
type Row struct {
	Mark
	Lat float64 `json:"latitude"`
	Lon float64 `json:"longitude"`
	// Places are the answers at the levels reported, widest first; a level
	// with no answer is absent.
	Places []locate.Match `json:"places"`
}

// Summarise looks the course up and returns its change log.
//
// Every depth's change log is built from one lookup at the finest depth any
// of them needs, so choosing a depth automatically costs nothing more than
// asking for the finest.
func Summarise(ctx context.Context, c *course.Course, src locate.TileSource, o Options) (*Summary, error) {
	if len(c.Points) == 0 {
		return nil, fmt.Errorf("the course has no positions to look up")
	}
	maxRows := o.MaxRows
	if maxRows <= 0 {
		maxRows = DefaultMaxRows
	}
	spacing := o.Spacing
	if spacing <= 0 {
		spacing = DefaultSpacing
	}
	finest := o.Depth
	if o.Auto {
		finest = Depths[len(Depths)-1]
	} else if !isDepth(finest) {
		return nil, fmt.Errorf("%s is not a depth a summary is reported at", finest)
	}

	idx := c.Thin(spacing)
	pts := make([]locate.Coord, len(idx))
	for i, j := range idx {
		pts[i] = locate.Coord{Lat: c.Points[j].Lat, Lon: c.Points[j].Lon}
	}
	places, err := locate.AtEach(ctx, src, pts, locate.Options{
		Language:     o.Language,
		Boundaries:   o.Boundaries,
		Levels:       levelsTo(finest),
		OnWay:        true,
		MaxDistanceM: map[locate.Level]float64{locate.Street: StreetReachM},
	})
	if err != nil {
		return nil, err
	}

	s := &Summary{Depth: finest, Auto: o.Auto, Timed: c.Timed, Measured: measured(c)}
	if s.Short, err = shortfalls(pts, levelsTo(finest), o); err != nil {
		return nil, err
	}
	walk := track{c: c, idx: idx, places: places}
	walk.dropPuddles()
	walk.bridge()
	if o.Auto {
		for i := len(Depths) - 1; i >= 0; i-- {
			s.Depth = Depths[i]
			if i > 0 && (lacks(s.Short, s.Depth) || !walk.answers(s.Depth)) {
				continue
			}
			if s.Rows = walk.rows(s.Depth, 0); len(s.Rows) <= maxRows {
				break
			}
		}
		// Even the widest depth has more than maxRows rows -- an archipelago is
		// island, sea, island, and each is a country or a sea -- so the
		// shortest stretches go first until it fits, and what is left is the
		// longest part of the journey rather than its first twenty-five.
		if len(s.Rows) > maxRows {
			s.Rows = walk.rows(s.Depth, maxRows)
		}
	} else {
		s.Rows = walk.rows(finest, 0)
	}
	s.Finish = mark(c, c.Points[len(c.Points)-1])
	if s.Prefix, err = prefix(ctx, src, pts, walk, s.Depth, o); err != nil {
		return nil, err
	}
	if s.Departure, s.Arrival, err = airports(ctx, src, pts, s.Depth, o); err != nil {
		return nil, err
	}
	return s, nil
}

// prefix is the one place the whole course was in, by o.Prefix, when the
// depth is finer than a locality. A point with no answer does not count
// against it; two different answers do, and then there is no prefix.
func prefix(ctx context.Context, src locate.TileSource, pts []locate.Coord, t track, depth locate.Level, o Options) (string, error) {
	if depth <= locate.Locality || o.Prefix == PrefixNone {
		return "", nil
	}
	places, level := t.places, locate.Locality
	if o.Prefix == PrefixCity {
		// The outlines first. A course through two cities has no one city
		// to name, and a label's guess is not asked to pick between them.
		switch name, ok := one(t.places, locate.City); {
		case !ok:
			return "", nil
		case name != "":
			return name, nil
		}
		if src == nil {
			return "", nil
		}
		// The tiles only: a boundary answer is an outline holding the point,
		// which is the council where suburbs are mapped, not the city.
		var err error
		places, err = locate.AtEach(ctx, src, pts, locate.Options{
			Language: o.Language, Levels: []locate.Level{locate.Locality}, Prominent: true,
		})
		if err != nil {
			return "", err
		}
	}
	name, _ := one(places, level)
	return name, nil
}

// one is the one name places give at a level, ignoring the places with no
// answer there; false when they give two.
func one(places []locate.Place, l locate.Level) (string, bool) {
	name := ""
	for _, p := range places {
		m, ok := p.Match(l)
		if !ok {
			continue
		}
		if name != "" && m.Name != name {
			return "", false
		}
		name = m.Name
	}
	return name, true
}

// airports are the airports a course starts and ends inside, when the depth
// is wider than an area -- at area depth or finer they are rows already.
func airports(ctx context.Context, src locate.TileSource, pts []locate.Coord, depth locate.Level, o Options) (*locate.Match, *locate.Match, error) {
	if depth >= locate.Area || src == nil {
		return nil, nil, nil
	}
	ends := []locate.Coord{pts[0], pts[len(pts)-1]}
	places, err := locate.AtEach(ctx, src, ends, locate.Options{Language: o.Language, Levels: []locate.Level{locate.Area}})
	if err != nil {
		return nil, nil, err
	}
	at := func(p locate.Place) *locate.Match {
		if m, ok := p.Match(locate.Area); ok && m.Kind == "aerodrome" {
			return &m
		}
		return nil
	}
	return at(places[0]), at(places[1]), nil
}

// shortfalls measures, for each level asked about that the tiles answer, how
// many of the tiles along the course the store holds at that level's zoom.
//
// Measured rather than learned from the answers, because an answer that is
// not there cannot say why: a run through a store with no map of it has no
// street names, exactly as a run across open country does, and without this
// "street" would look like a depth that fit -- a change log of one row, the
// region, at street depth. A level the boundaries answer needs no tiles and
// is not measured.
func shortfalls(pts []locate.Coord, levels []locate.Level, o Options) ([]Shortfall, error) {
	if o.Holdings == nil {
		return nil, nil
	}
	var out []Shortfall
	for _, l := range levels {
		z, ok := l.Zoom()
		if !ok || (o.Boundaries != nil && o.Boundaries.Covers(l)) {
			continue
		}
		seen := map[[2]uint32]bool{}
		held := 0
		for _, p := range pts {
			x, y, err := mercator.TileAt(z, p.Lon, p.Lat)
			if err != nil {
				return nil, err
			}
			if seen[[2]uint32{x, y}] {
				continue
			}
			seen[[2]uint32{x, y}] = true
			w, sth, e, n, err := mercator.TileBounds(z, x, y)
			if err != nil {
				return nil, err
			}
			// The middle of the tile, so the rectangle is in this tile alone.
			cx, cy := (w+e)/2, (sth+n)/2
			h, _, err := o.Holdings.HeldAt(slice.Bounds{West: cx, South: cy, East: cx, North: cy}, z)
			if err != nil {
				return nil, err
			}
			held += min(h, 1)
		}
		if float64(held) < DefaultHeld*float64(len(seen)) {
			out = append(out, Shortfall{Level: l, Zoom: z, Held: held, Wanted: len(seen)})
		}
	}
	return out, nil
}

// lacks reports whether the store lacks the map for d or any level wider.
func lacks(short []Shortfall, d locate.Level) bool {
	for _, s := range short {
		if s.Level <= d {
			return true
		}
	}
	return false
}

func isDepth(l locate.Level) bool {
	for _, d := range Depths {
		if d == l {
			return true
		}
	}
	return false
}

// levelsTo are the levels reported at depth d, widest first: every level no
// finer than d, and water.
func levelsTo(d locate.Level) []locate.Level {
	var out []locate.Level
	for _, l := range locate.Levels {
		if l <= d || l == locate.Water {
			out = append(out, l)
		}
	}
	return out
}

func measured(c *course.Course) bool {
	for _, p := range c.Points {
		if !p.HasDistance {
			return false
		}
	}
	return true
}

func mark(c *course.Course, p course.Point) Mark {
	return Mark{HasElapsed: c.Timed, Elapsed: p.Elapsed, HasDistance: p.HasDistance, Distance: p.Distance}
}

// answers reports whether anything along the course was answered at level
// l. A depth that names nothing at its own level is the next wider depth's
// change log with a finer name on it -- "area" for a course that passed no
// park -- and auto does not choose it.
func (t track) answers(l locate.Level) bool {
	for _, p := range t.places {
		if _, ok := p.Match(l); ok {
			return true
		}
	}
	return false
}

// track is the looked-up points of a course.
type track struct {
	c      *course.Course
	idx    []int
	places []locate.Place
}

// maxBridge is the longest stretch, in metres, over which a country or a
// region is carried across a gap in the answers.
const maxBridge = 50_000

// bridge fills a gap in the country and region answers where the ground is
// the same on both sides of it and nothing -- not even a sea -- is named in
// between.
//
// The gap it is for is the coast. Country and region outlines are Natural
// Earth's, generalised by up to a kilometre, and a course along a beachfront
// runs in and out of them: a marathon along the shore was in Queensland, then
// nowhere, then Queensland again, for twenty-five kilometres at a time. The
// sea beyond has a name of its own when the course is really over it, so a
// stretch with neither a region nor a sea is a stretch the outlines missed,
// not one the course left the region for. At the start or the end of a
// course, a gap is filled from the one side it has.
//
// A filled answer is Near, not Contained -- it is an inference from the
// ground either side, not a boundary holding the point -- at the straight-line
// distance to the answer it was taken from, and keeps that answer's credit.
func (t track) bridge() {
	for _, l := range []locate.Level{locate.Country, locate.Region} {
		n := len(t.places)
		for i := 0; i < n; {
			if t.answered(i, l) {
				i++
				continue
			}
			j := i
			for j < n && !t.answered(j, l) {
				j++
			}
			// [i, j) has no answer at l.
			before, after := i-1, j
			var from []int
			switch {
			case before < 0 && after < n:
				from = []int{after}
			case before >= 0 && after >= n:
				from = []int{before}
			case before >= 0 && after < n:
				a, _ := t.places[before].Match(l)
				b, _ := t.places[after].Match(l)
				if a.Name == b.Name {
					from = []int{before, after}
				}
			}
			if len(from) > 0 && !t.watery(i, j) && t.span(i, j) <= maxBridge {
				for k := i; k < j; k++ {
					t.fill(k, l, from)
				}
			}
			i = j
		}
	}
}

func (t track) answered(i int, l locate.Level) bool {
	_, ok := t.places[i].Match(l)
	return ok
}

// watery reports whether any point in [i, j) is over named water.
func (t track) watery(i, j int) bool {
	for k := i; k < j; k++ {
		if t.answered(k, locate.Water) {
			return true
		}
	}
	return false
}

// span is how far the course goes from the point before i to the one at j.
func (t track) span(i, j int) float64 {
	var d float64
	for k := max(i, 1); k <= min(j, len(t.idx)-1); k++ {
		a, b := t.c.Points[t.idx[k-1]], t.c.Points[t.idx[k]]
		d += course.Metres(a.Lat, a.Lon, b.Lat, b.Lon)
	}
	return d
}

// fill gives point k level l's answer from the nearest of the points in from.
func (t track) fill(k int, l locate.Level, from []int) {
	p := t.c.Points[t.idx[k]]
	best, bestD := -1, 0.0
	for _, f := range from {
		q := t.c.Points[t.idx[f]]
		if d := course.Metres(p.Lat, p.Lon, q.Lat, q.Lon); best < 0 || d < bestD {
			best, bestD = f, d
		}
	}
	m, _ := t.places[best].Match(l)
	m.Source = locate.Near
	m.DistanceM = math.Round(bestD*10) / 10
	pl := &t.places[k]
	pl.Matches = append(pl.Matches, m)
	sort.SliceStable(pl.Matches, func(a, b int) bool { return pl.Matches[a].Level < pl.Matches[b].Level })
}

// minWater is the shortest stretch, in metres, over which a course is said
// to be over named water.
const minWater = 1_000

// dropPuddles takes out named water the course is over for less than
// minWater at a time.
//
// Natural Earth's seas are drawn to its generalised coastline, and reach
// onto the land by as much as the land falls short of it: a run round a
// headland was "over the Tasman Sea" for two hundred metres of footpath. A
// crossing a summary should name -- a ferry, a flight -- is kilometres, so a
// shorter one is the outline's error and not the course's. Runs of water are
// taken out before bridge, so the land either side is then carried across.
func (t track) dropPuddles() {
	n := len(t.places)
	for i := 0; i < n; {
		if !t.answered(i, locate.Water) {
			i++
			continue
		}
		j := i
		for j < n && t.answered(j, locate.Water) {
			j++
		}
		if t.span(i, j) < minWater {
			for k := i; k < j; k++ {
				pl := &t.places[k]
				kept := pl.Matches[:0]
				for _, m := range pl.Matches {
					if m.Level != locate.Water {
						kept = append(kept, m)
					}
				}
				pl.Matches = kept
			}
		}
		i = j
	}
}

// run is a stretch of looked-up points with the same names.
type run struct {
	first, last int // into track.idx
	key         string
	length      float64 // metres along the course
	// contained is a run whose finest answer is an outline or an area
	// holding the course, rather than the nearest name to it.
	contained bool
}

// minRun is how far a course must go in a place for it to be a row, and
// minDetour how far when it comes back to where it was just before.
//
// A course running along the edge of two places -- down the street that is
// the boundary between two suburbs, along a footpath beside a road -- flickers
// between them from one point to the next, and each flicker would be a row
// saying the course went somewhere it only brushed. A stretch shorter than
// minRun is folded into the one before it. One that returns to the place
// before it -- A, B, A -- is folded up to minDetour: the footpath beside the
// road is nearest for a hundred and fifty metres, then the road, then the
// path, all the way round a lake, and a lap of it is one street's worth of
// course, not six.
//
// Both grow with the depth, because the places do and so does the flicker: a
// flight up the west coast of Scotland is over an island, then a sound, then
// an island, thirty seconds and seven kilometres each, and at country depth
// that is the United Kingdom and the Inner Seas eight times in five minutes.
// The detour stops short of a crossing worth naming, though: the Great Belt is
// eighteen kilometres of sea from Denmark to Denmark, and a ferry over it
// went somewhere.
func minRun(d locate.Level) (least, detour float64) {
	switch d {
	case locate.Street:
		return 60, 400
	case locate.Neighbourhood, locate.Macrohood, locate.Area:
		return 250, 1_000
	case locate.Locality:
		return 500, 2_000
	}
	return 5_000, 15_000
}

// rows is the change log at depth d, of no more than limit rows when limit is
// not 0.
func (t track) rows(d locate.Level, limit int) []Row {
	levels := levelsTo(d)
	runs := t.runs(levels)
	least, detour := minRun(d)
	runs = fold(runs, least, detour, limit)

	rows := make([]Row, len(runs))
	for i, r := range runs {
		p := t.c.Points[t.idx[r.first]]
		rows[i] = Row{Mark: mark(t.c, p), Lat: p.Lat, Lon: p.Lon, Places: t.names(r, levels)}
	}
	return rows
}

func (t track) runs(levels []locate.Level) []run {
	var runs []run
	for i := range t.idx {
		k := key(t.places[i], levels)
		var step float64
		if i > 0 {
			a, b := t.c.Points[t.idx[i-1]], t.c.Points[t.idx[i]]
			step = course.Metres(a.Lat, a.Lon, b.Lat, b.Lon)
		}
		if n := len(runs); n > 0 && runs[n-1].key == k {
			runs[n-1].last = i
			runs[n-1].length += step
			continue
		}
		if n := len(runs); n > 0 {
			// The step into a new place is counted as half each side's.
			runs[n-1].length += step / 2
			step /= 2
		}
		runs = append(runs, run{first: i, last: i, key: k, length: step, contained: finestHeld(t.places[i], levels)})
	}
	return runs
}

// minHeld is how far a course must go in a place that holds it -- an outline,
// a park -- to be a row, returning or not.
//
// The limits above are for NEAREST answers, whose flicker is an artefact of
// asking which label is closest. An outline holding the course is a fact, and
// the course crossing it is too: a run along Powells Creek, which is the
// boundary between Sydney Olympic Park and Liberty Grove, is in each for a
// hundred and fifty metres at a time, and that is where it went. Only a
// crossing short enough to be the GPS's wobble across the line is folded.
const minHeld = 100.0

// finestHeld reports whether a place's finest land answer at these levels is
// one that holds it.
func finestHeld(p locate.Place, levels []locate.Level) bool {
	for i := len(levels) - 1; i >= 0; i-- {
		if levels[i] == locate.Water {
			continue
		}
		if m, ok := p.Match(levels[i]); ok {
			return m.Source == locate.Contained || m.Source == locate.Within
		}
	}
	return false
}

// fold folds every run shorter than min -- or than detour, when the runs
// either side of it are the same place -- into its predecessor, and joins
// neighbours that then have the same names, shortest first, until none is
// left to fold. With a limit, it then goes on folding the shortest whatever
// its length until there are no more than limit runs. The first run is the
// start and is never folded, and to the limit the last is not either: it is
// where the course ended, and a flight landing in Denmark that is summarised
// as ending in Sweden has lost the one row everybody reads.
func fold(runs []run, min, detour float64, limit int) []run {
	for {
		shortest := -1
		over := limit > 0 && len(runs) > limit
		for i := 1; i < len(runs); i++ {
			least := min
			if i+1 < len(runs) && runs[i-1].key == runs[i+1].key {
				least = detour
			}
			if runs[i].contained {
				least = minHeld
			}
			foldable := runs[i].length < least || (over && i < len(runs)-1)
			if foldable && (shortest < 0 || runs[i].length < runs[shortest].length) {
				shortest = i
			}
		}
		if shortest < 0 {
			return runs
		}
		prev := &runs[shortest-1]
		prev.last = runs[shortest].last
		prev.length += runs[shortest].length
		runs = append(runs[:shortest], runs[shortest+1:]...)
		if shortest < len(runs) && runs[shortest].key == prev.key {
			prev.last = runs[shortest].last
			prev.length += runs[shortest].length
			runs = append(runs[:shortest], runs[shortest+1:]...)
		}
	}
}

func key(p locate.Place, levels []locate.Level) string {
	var b strings.Builder
	for _, l := range levels {
		if m, ok := p.Match(l); ok {
			b.WriteString(m.Name)
		}
		b.WriteByte(0)
	}
	return b.String()
}

// names are a run's places: at each level, the best evidence among the run's
// points for the one name it has there -- a boundary holding a point over the
// nearest name to one, and the nearest of those. Every point in a run has the
// same names, but not the same evidence for them: the first point of a run
// along the coast may have its region only by bridge while the next is inside
// the region's outline, and the row should say what is known, not what the
// first point happened to know.
func (t track) names(r run, levels []locate.Level) []locate.Match {
	var out []locate.Match
	for _, l := range levels {
		var best locate.Match
		found := false
		for i := r.first; i <= r.last; i++ {
			// Only the points that are this run's place: a run that had a
			// short stretch folded into it still holds that stretch's points,
			// and their names are the ones the fold took away.
			if key(t.places[i], levels) != r.key {
				continue
			}
			m, ok := t.places[i].Match(l)
			if !ok {
				continue
			}
			if !found || better(m, best) {
				best, found = m, true
			}
		}
		if found {
			out = append(out, best)
		}
	}
	return out
}

func better(a, b locate.Match) bool {
	if a.Source != b.Source {
		return a.Source == locate.Contained
	}
	return a.DistanceM < b.DistanceM
}

// Label is the most specific name a row has for the ground: its deepest
// place on land, or the water it is over when it is over nothing else.
//
// Land first, because a course on the coast is in both -- Nice and the Golfe
// du Lion, Sydney and the Tasman Sea -- and a flight that "departed the Golfe
// du Lion" has been described by the wrong half of the truth.
func (r Row) Label() string {
	label := ""
	for _, m := range r.Places {
		if m.Level != locate.Water {
			label = m.Name
		}
	}
	if label == "" {
		for _, m := range r.Places {
			if m.Level == locate.Water {
				label = m.Name
			}
		}
	}
	return label
}

// OneLiner is the chain of places the course passed through, each row's most
// specific name with repeats left out -- "Sydney: Richmond → Kew → Putney" --
// with a loop run more than once written once, "2x (Lowe Road → …)", and the
// airports at the ends of a flight beside the first and last names.
func (s *Summary) OneLiner() string {
	var parts []string
	for _, r := range s.Rows {
		l := r.Label()
		if l == "" || (len(parts) > 0 && parts[len(parts)-1] == l) {
			continue
		}
		parts = append(parts, l)
	}
	if len(parts) == 1 && s.Arrival != nil {
		// One place throughout, and an airport at the end: the start and the
		// end are both that place, and the airport belongs to the end.
		parts = append(parts, parts[0])
	}
	if len(parts) > 0 {
		if s.Departure != nil {
			parts[0] = s.Departure.Name + ", " + parts[0]
		}
		if s.Arrival != nil {
			parts[len(parts)-1] = s.Arrival.Name + ", " + parts[len(parts)-1]
		}
	}
	line := strings.Join(loops(parts), " → ")
	if s.Prefix != "" && line != "" {
		line = s.Prefix + ": " + line
	}
	return line
}

// loops writes a stretch of the chain repeated back to back once, with how
// many times: a course run twice round the same block is the block, twice.
//
// Read left to right, taking at each place the repeat that covers the most of
// the chain, and of those the shortest: a loop of ten streets run twice is one
// group rather than the shorter coincidences inside it, and a lap of two
// places run four times is "4x" of the two rather than "2x" of the four. A
// stretch of one name cannot repeat, since repeats are already left out.
func loops(parts []string) []string {
	var out []string
	for i := 0; i < len(parts); {
		bestK, bestN := 0, 1
		for k := 2; k <= (len(parts)-i)/2; k++ {
			n := 1
			for i+(n+1)*k <= len(parts) && equal(parts[i:i+k], parts[i+n*k:i+(n+1)*k]) {
				n++
			}
			if n > 1 && n*k > bestN*bestK {
				bestK, bestN = k, n
			}
		}
		if bestK == 0 {
			out = append(out, parts[i])
			i++
			continue
		}
		out = append(out, fmt.Sprintf("%dx (%s)", bestN, strings.Join(parts[i:i+bestK], " → ")))
		i += bestN * bestK
	}
	return out
}

func equal(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
