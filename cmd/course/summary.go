package main

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/osmbase/fetch"
	"github.com/wisborg/osmbase/locate"
	"github.com/wisborg/osmbase/slice"
	"github.com/wisborg/output"
	"github.com/wisborg/output/table"

	"github.com/wisborg/course"
	"github.com/wisborg/course/summary"
)

var summaryOpts struct {
	detailed bool
	depth    string
	maxRows  int
	store    string
	archive  string
	language string
	prefix   string
	gap      float64
	yes      bool
	format   formatFlag
}

var summaryCmd = &cobra.Command{
	Use:   "summary COURSE [COURSE ...]",
	Short: "Say where a course went",
	Long: `summary says where a course went: by default as one line, a chain of the
places it passed through, and with --detailed as the change log behind it --
one row each time the course entered a different place, with how far in it was.

At street depth, a stretch on no named way -- a path, a station concourse --
is named by its suburb when it is at least --suburb-gap metres long or in a
suburb other than the streets either side; otherwise it is left out of the
line, and shown in --detailed.

--depth is the finest level named: country, region, city, locality, macrohood,
neighbourhood, area (a park, a campus, an airport) or street. Named water -- a sea, a strait, a bay -- is named at
every depth. The default, auto, takes the finest depth whose change log fits
--max-rows rows: a local run is named street by street, and a flight falls back
to regions and countries by itself.

Several files are one course, merged in the order they were recorded.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSummary,
}

func init() {
	f := summaryCmd.Flags()
	f.BoolVar(&summaryOpts.detailed, "detailed", false, "the change log rather than one line")
	f.StringVar(&summaryOpts.depth, "depth", "auto", "the finest level named: auto, "+depthNames())
	f.IntVar(&summaryOpts.maxRows, "max-rows", summary.DefaultMaxRows, "the most rows --depth auto may produce")
	f.StringVar(&summaryOpts.store, "store", "", "the osmbase store to read places from (default: osmbase's own)")
	f.StringVar(&summaryOpts.archive, "archive", "", "which archive in the store, when it holds several")
	f.StringVar(&summaryOpts.language, "lang", "", "prefer names in this language, e.g. en; default is the local spelling")
	f.StringVar(&summaryOpts.prefix, "prefix", "city", "what the one line names ahead of its chain: city (the city's mapped extent, or the place whose reach the course is most within), locality (the locality level's answer), or none")
	f.BoolVar(&summaryOpts.yes, "yes", false, "fetch the map the summary needs without asking")
	f.Float64Var(&summaryOpts.gap, "suburb-gap", summary.DefaultSuburbGap, "at street depth, how long in metres a stretch on no named way must be to be named by its suburb when the streets either side are in the same one")
	summaryOpts.format = formatFlag{Format: output.Text}
	f.Var(&summaryOpts.format, "format", "output format: text, csv, json or yaml")
	root.AddCommand(summaryCmd)
}

func depthNames() string {
	var names []string
	for _, d := range summary.Depths {
		names = append(names, d.String())
	}
	return strings.Join(names, ", ")
}

func parseDepth(s string) (locate.Level, bool, error) {
	if s == "auto" {
		return 0, true, nil
	}
	if l, ok := locate.ParseLevel(s); ok {
		for _, d := range summary.Depths {
			if d == l {
				return l, false, nil
			}
		}
	}
	return 0, false, fmt.Errorf("--depth %q is not auto or one of %s", s, depthNames())
}

func runSummary(cmd *cobra.Command, args []string) error {
	depth, auto, err := parseDepth(summaryOpts.depth)
	if err != nil {
		return err
	}
	pre, ok := summary.Prefixes[summaryOpts.prefix]
	if !ok {
		return fmt.Errorf("--prefix %q is not city, locality or none", summaryOpts.prefix)
	}
	c, err := course.Read(args...)
	if err != nil {
		return err
	}
	st, err := openStore(summaryOpts.store, summaryOpts.archive)
	if err != nil {
		return err
	}
	n, offered := summaryNeed(st, c, depth, auto)
	if offered && offerToFill(cmd.Context(), cmd.ErrOrStderr(), n, summaryOpts.yes) {
		if st, err = openStore(summaryOpts.store, summaryOpts.archive); err != nil {
			return err
		}
	}
	if err := st.empty(); err != nil {
		return err
	}
	s, err := summary.Summarise(cmd.Context(), c, st.tileSource(), summary.Options{
		Depth: depth, Auto: auto, MaxRows: summaryOpts.maxRows,
		Language: summaryOpts.language, Boundaries: st.boundarySource(),
		Holdings: st.holdings(), Prefix: pre, SuburbGap: summaryOpts.gap,
	})
	if err != nil {
		return err
	}
	var all []locate.Match
	for _, r := range s.Rows {
		all = append(all, r.Places...)
	}
	for _, m := range []*locate.Match{s.Departure, s.Arrival} {
		if m != nil {
			all = append(all, *m)
		}
	}
	credits := st.credits(all)

	// The offer has already said what the store lacks, and a wide course
	// needs only its ends: the note is for a local course nobody was asked
	// about -- one whose store holds most of it, but not all.
	if !offered && len(n.areas) <= 1 {
		writeShortfall(cmd.ErrOrStderr(), st, s)
	}

	doc := output.Document{Data: jsonSummary(c, s, credits)}
	out := cmd.OutOrStdout()
	if summaryOpts.format.Format == output.Text && !summaryOpts.detailed {
		writeOneLiner(out, s, credits)
		return nil
	}
	u, err := unitsShown(cmd, summaryOpts.format.Format)
	if err != nil {
		return err
	}
	doc.Table = summaryTable(s, u)
	if err := doc.Write(out, summaryOpts.format.Format); err != nil {
		return err
	}
	if summaryOpts.format.Format == output.Text {
		writeFooter(out, c, s, credits, u)
	}
	return nil
}

// writeShortfall says, on stderr so it reaches a person whatever the format,
// which levels the store could not name along this course for want of the
// map, and how to fill it. Without it a run through an unfetched city reads
// as a run through a city with no streets.
func writeShortfall(w io.Writer, st *store, s *summary.Summary) {
	if len(s.Short) == 0 {
		return
	}
	var parts []string
	for _, sh := range s.Short {
		parts = append(parts, fmt.Sprintf("%s (%d of %d tiles at zoom %d)", sh.Level, sh.Held, sh.Wanted, sh.Zoom))
	}
	fmt.Fprintf(w, "course: %s holds too little of the map along this course to name %s.\n", st.root, strings.Join(parts, ", "))
	fmt.Fprintf(w, "        Fill it with \"osmbase fetch --store %s\" over the course's area.\n", st.root)
}

// localSpan is how wide a course may be, in metres corner to corner, for its
// whole area to be worth fetching at street detail: a run, a ride, a day's
// walk. Wider is a drive or a flight, whose middle is named by country and
// sea outlines and needs no map, and whose ends -- where it started and
// finished, an airport -- are all that is fetched.
const localSpan = 100_000.0

// endReach is how far round each end of a wide course is fetched, in
// degrees: an airport's worth.
const endReach = 0.03

// summaryNeed is the map a summary at depth needs, and whether the store
// lacks it:
// along the whole of a local course, at the ends of a wide one, at the zoom
// the finest level asked for is read at. Levels the boundaries answer need
// none.
func summaryNeed(st *store, c *course.Course, depth locate.Level, auto bool) (need, bool) {
	finest := depth
	if auto {
		finest = summary.Depths[len(summary.Depths)-1]
	}
	z, ok := finest.Zoom()
	if !ok || len(c.Points) == 0 || (st.boundaries != nil && st.boundaries.Covers(finest)) {
		return need{}, false
	}
	b := slice.Bounds{West: 180, South: 90, East: -180, North: -90}
	for _, p := range c.Points {
		b.West, b.East = min(b.West, p.Lon), max(b.East, p.Lon)
		b.South, b.North = min(b.South, p.Lat), max(b.North, p.Lat)
	}
	n := need{root: st.root, archive: st.manifest.Source, zoom: z}
	if course.Metres(b.South, b.West, b.North, b.East) <= localSpan {
		const pad = 0.003
		n.areas = []slice.Bounds{{West: b.West - pad, South: b.South - pad, East: b.East + pad, North: b.North + pad}}
		n.why = "along this course, which a summary to " + finest.String() + " depth needs"
	} else {
		for _, p := range []course.Point{c.Points[0], c.Points[len(c.Points)-1]} {
			n.areas = append(n.areas, slice.Bounds{West: p.Lon - endReach, South: p.Lat - endReach, East: p.Lon + endReach, North: p.Lat + endReach})
		}
		n.why = "round where this course started and finished, which a summary needs to name them"
	}
	if st.tiles == nil {
		n.why = "a summary to " + finest.String() + " depth needs one"
		return n, true
	}
	n.zoom = fetch.DrawnZoom(st.tiles, z)
	for _, a := range n.areas {
		held, wanted, err := st.tiles.HeldAt(a, n.zoom)
		if err != nil {
			return n, false
		}
		n.held, n.wanted = n.held+held, n.wanted+wanted
	}
	// The summary's own threshold for a depth it can choose: see
	// summary.DefaultHeld.
	if float64(n.held) >= summary.DefaultHeld*float64(n.wanted) {
		return n, false
	}
	return n, true
}

func writeOneLiner(w io.Writer, s *summary.Summary, credits []string) {
	line := s.OneLiner()
	if line == "" {
		line = "(no named place along this course in the store)"
	}
	fmt.Fprintln(w, line)
	for _, c := range credits {
		fmt.Fprintf(w, "  %s\n", c)
	}
}

// summaryTable is the change log for a terminal or CSV: one column per level
// that named anything, so a flight is not a table of empty street columns.
func summaryTable(s *summary.Summary, u units.Set) *table.Table {
	var levels []locate.Level
	for _, l := range locate.Levels {
		for _, r := range s.Rows {
			if _, ok := placeAt(r, l); ok {
				levels = append(levels, l)
				break
			}
		}
	}
	var cols []table.Column
	if s.Timed {
		cols = append(cols, table.Column{Header: "elapsed", Align: table.Right})
	}
	if s.Measured {
		cols = append(cols, table.Column{Header: u.Distance.Name, Align: table.Right, Format: "%.2f"})
	}
	for _, l := range levels {
		cols = append(cols, table.Column{Header: l.String()})
	}
	t := table.New(cols...)
	for _, r := range s.Rows {
		var cells []any
		if s.Timed {
			cells = append(cells, clock(r.Elapsed))
		}
		if s.Measured {
			cells = append(cells, u.Distance.FromSI(r.Distance))
		}
		for _, l := range levels {
			name := ""
			if m, ok := placeAt(r, l); ok {
				name = m.Name
				if m.Source == locate.Near {
					name = "~" + name
				}
			}
			cells = append(cells, name)
		}
		t.MustAppend(cells...)
	}
	return t
}

func placeAt(r summary.Row, l locate.Level) (locate.Match, bool) {
	for _, m := range r.Places {
		if m.Level == l {
			return m, true
		}
	}
	return locate.Match{}, false
}

func writeFooter(w io.Writer, c *course.Course, s *summary.Summary, credits []string, u units.Set) {
	how := "chosen"
	if s.Auto {
		how = fmt.Sprintf("auto, the finest within %d rows", summaryOpts.maxRows)
	}
	fmt.Fprintf(w, "\ndepth %s (%s); ~ is the nearest named, not a boundary or area holding the course\n", s.Depth, how)
	finish := []string{}
	if s.Finish.HasElapsed {
		finish = append(finish, clock(s.Finish.Elapsed))
	}
	if s.Finish.HasDistance {
		finish = append(finish, distance(s.Finish.Distance, u))
	}
	if len(finish) > 0 {
		fmt.Fprintf(w, "finish %s\n", strings.Join(finish, ", "))
	}
	if !s.Timed {
		fmt.Fprintf(w, "no times: this course is a plan, not a recording\n")
	}
	if c.Dropped > 0 {
		fmt.Fprintf(w, "%d rogue fixes left out\n", c.Dropped)
	}
	for _, cr := range credits {
		fmt.Fprintf(w, "%s\n", cr)
	}
}

// clock is an elapsed time as a stopwatch shows it, truncated.
func clock(d time.Duration) string {
	d = d.Truncate(time.Second)
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

type jsonRow struct {
	ElapsedS  *float64       `json:"elapsed_s,omitempty"`
	DistanceM *float64       `json:"distance_m,omitempty"`
	LengthM   *float64       `json:"length_m,omitempty"`
	Latitude  float64        `json:"latitude"`
	Longitude float64        `json:"longitude"`
	Places    []locate.Match `json:"places"`
}

type jsonDoc struct {
	Sources   []string      `json:"sources"`
	Sport     string        `json:"sport,omitempty"`
	Depth     locate.Level  `json:"depth"`
	Auto      bool          `json:"depth_auto"`
	Prefix    string        `json:"prefix,omitempty"`
	Departure *locate.Match `json:"departure,omitempty"`
	Arrival   *locate.Match `json:"arrival,omitempty"`
	OneLiner  string        `json:"one_liner"`
	Rows      []jsonRow     `json:"rows"`
	Finish    *jsonRow      `json:"finish,omitempty"`
	Dropped   int           `json:"rogue_fixes_left_out"`
	Credits   []string      `json:"credits"`
}

// jsonSummary is the summary for JSON and YAML. A time or a distance the
// course did not record is left out of each row, never written as zero.
func jsonSummary(c *course.Course, s *summary.Summary, credits []string) jsonDoc {
	conv := func(m summary.Mark, lat, lon float64, places []locate.Match) jsonRow {
		r := jsonRow{Latitude: lat, Longitude: lon, Places: places}
		if m.HasElapsed {
			v := m.Elapsed.Seconds()
			r.ElapsedS = &v
		}
		if m.HasDistance {
			v := m.Distance
			r.DistanceM = &v
		}
		return r
	}
	d := jsonDoc{Sources: c.Sources, Sport: c.Sport, Depth: s.Depth, Auto: s.Auto, OneLiner: s.OneLiner(),
		Prefix: s.Prefix, Departure: s.Departure, Arrival: s.Arrival,
		Dropped: c.Dropped, Credits: credits, Rows: []jsonRow{}}
	for _, r := range s.Rows {
		row := conv(r.Mark, r.Lat, r.Lon, r.Places)
		length := math.Round(r.Length)
		row.LengthM = &length
		d.Rows = append(d.Rows, row)
	}
	last := c.Points[len(c.Points)-1]
	if f := conv(s.Finish, last.Lat, last.Lon, nil); f.ElapsedS != nil || f.DistanceM != nil {
		f.Places = nil
		d.Finish = &f
	}
	if d.Credits == nil {
		d.Credits = []string{}
	}
	return d
}
