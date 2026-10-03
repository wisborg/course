package main

import (
	"fmt"
	"io"
	"math"
	"time"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/output"
	"github.com/wisborg/output/table"

	"github.com/wisborg/course"
	"github.com/wisborg/course/match"
)

var matchOpts struct {
	references []string
	near       float64
	format     formatFlag
}

var matchCmd = &cobra.Command{
	Use:   "match ACTIVITY [ACTIVITY ...]",
	Short: "Find where an activity followed reference courses",
	Long: `match finds every stretch of an activity that followed a reference course --
a parkrun inside a morning's run, both parkruns of a morning with the warm-up,
the commute between them and the cool-down around them, an official race
course -- and says how closely: how much of the course it covered, how far
off it was, where it missed the course or left it, and where it stopped for a
minute or more -- a toilet, a drink station -- which is reported and never
held against the match.

It checks every stored reference ("course reference list"), or those named
with --reference, stored or as files. The reference is aligned with the
activity in order, so the way back of an out-and-back course is not taken for
the way out, and a course run backwards is not a match.

--near is how far from the course is still on it, in metres whatever the
units shown. GPS among tall
buildings wanders up to about 20 m from a course followed exactly; the
default, 25, is just above that.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMatch,
}

func init() {
	f := matchCmd.Flags()
	f.StringArrayVar(&matchOpts.references, "reference", nil, "a reference to match against, stored or a file; repeat for several (default: every stored reference)")
	f.StringVar(&referencesDir, "references", "", "the directory stored references are kept in (default: course/references in your configuration directory)")
	f.Float64Var(&matchOpts.near, "near", 0, "how far from a course, in metres, is still on it (default 25)")
	matchOpts.format = formatFlag{Format: output.Text}
	f.Var(&matchOpts.format, "format", "output format: text, csv, json or yaml")
	root.AddCommand(matchCmd)
}

func runMatch(cmd *cobra.Command, args []string) error {
	u, err := unitsShown(cmd, matchOpts.format.Format)
	if err != nil {
		return err
	}
	c, err := course.Read(args...)
	if err != nil {
		return err
	}
	refs, err := matchReferences(matchOpts.references)
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return fmt.Errorf("no references to match against; store one with \"course reference add\", or name a file with --reference")
	}
	ms := match.Find(c, refs, match.Options{Near: matchOpts.near})
	if matchOpts.format.Format == output.Text {
		writeMatches(cmd.OutOrStdout(), ms, c.Timed, u)
		return nil
	}
	return output.Document{Data: jsonMatches(ms, c.Timed), Table: matchTable(ms, c.Timed, u)}.Write(cmd.OutOrStdout(), matchOpts.format.Format)
}

type jsonStretch struct {
	FromM     float64 `json:"from_m"`
	ToM       float64 `json:"to_m"`
	FarthestM float64 `json:"farthest_m"`
}

type jsonMatch struct {
	Reference  string        `json:"reference"`
	FromM      float64       `json:"from_m"`
	ToM        float64       `json:"to_m"`
	FromS      *float64      `json:"from_s,omitempty"`
	ToS        *float64      `json:"to_s,omitempty"`
	Coverage   float64       `json:"coverage"`
	MeanM      float64       `json:"mean_m"`
	MedianM    float64       `json:"median_m"`
	WorstM     float64       `json:"worst_m"`
	Missed     []jsonStretch `json:"missed"`
	Excursions []jsonStretch `json:"excursions"`
	Stops      []jsonStop    `json:"stops"`
}

type jsonStop struct {
	AtM       float64 `json:"at_m"`
	FromS     float64 `json:"from_s"`
	DurationS float64 `json:"duration_s"`
	OffM      float64 `json:"off_m"`
}

// jsonMatches are the matches as the other commands' JSON is written: names
// in snake case, distances in metres and times in seconds -- and no time for
// an activity that has none.
func jsonMatches(ms []match.Match, timed bool) []jsonMatch {
	stretches := func(ss []match.Stretch) []jsonStretch {
		out := []jsonStretch{}
		for _, s := range ss {
			out = append(out, jsonStretch{FromM: round1(s.From), ToM: round1(s.To), FarthestM: round1(s.Farthest)})
		}
		return out
	}
	out := []jsonMatch{}
	for _, m := range ms {
		j := jsonMatch{
			Reference: m.Reference, FromM: round1(m.From), ToM: round1(m.To), Coverage: m.Coverage,
			MeanM: round1(m.Mean), MedianM: round1(m.Median), WorstM: round1(m.Worst),
			Missed: stretches(m.Missed), Excursions: stretches(m.Excursions), Stops: []jsonStop{},
		}
		for _, st := range m.Stops {
			j.Stops = append(j.Stops, jsonStop{AtM: round1(st.At), FromS: math.Round(st.From.Seconds()), DurationS: math.Round(st.Duration.Seconds()), OffM: round1(st.Off)})
		}
		if timed {
			from, to := m.FromTime.Seconds(), m.ToTime.Seconds()
			j.FromS, j.ToS = &from, &to
		}
		out = append(out, j)
	}
	return out
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// matchReferences are the references named, stored or files, or every stored
// reference when none is named.
func matchReferences(names []string) ([]match.Reference, error) {
	if len(names) == 0 {
		s, err := openReferences()
		if err != nil {
			return nil, err
		}
		all, err := s.List()
		if err != nil {
			return nil, err
		}
		var out []match.Reference
		for _, m := range all {
			c, err := m.Course()
			if err != nil {
				return nil, fmt.Errorf("the reference %q: %w", m.Name, err)
			}
			out = append(out, match.Reference{Name: m.Name, Course: c})
		}
		return out, nil
	}
	var out []match.Reference
	for _, n := range names {
		name, c, err := resolveCourse(n)
		if err != nil {
			return nil, fmt.Errorf("--reference %w", err)
		}
		out = append(out, match.Reference{Name: name, Course: c})
	}
	return out, nil
}

func matchTable(ms []match.Match, timed bool, u units.Set) *table.Table {
	cols := []table.Column{
		{Header: "reference"},
		{Header: "from " + u.Distance.Name, Align: table.Right, Format: "%.2f"},
		{Header: "to " + u.Distance.Name, Align: table.Right, Format: "%.2f"},
	}
	if timed {
		cols = append(cols, table.Column{Header: "from", Align: table.Right}, table.Column{Header: "to", Align: table.Right})
	}
	cols = append(cols,
		table.Column{Header: "covered", Align: table.Right, Format: "%.0f%%"},
		table.Column{Header: "median " + u.Elevation.Name, Align: table.Right, Format: "%.0f"},
		table.Column{Header: "worst " + u.Elevation.Name, Align: table.Right, Format: "%.0f"},
	)
	t := table.New(cols...)
	for _, m := range ms {
		row := []any{m.Reference, u.Distance.FromSI(m.From), u.Distance.FromSI(m.To)}
		if timed {
			row = append(row, clock(m.FromTime), clock(m.ToTime))
		}
		row = append(row, 100*m.Coverage, u.Elevation.FromSI(m.Median), u.Elevation.FromSI(m.Worst))
		t.MustAppend(row...)
	}
	return t
}

// writeMatches is the matches for a person: the table, then under it each
// match's detours, which a table has no room for.
func writeMatches(w io.Writer, ms []match.Match, timed bool, u units.Set) {
	if len(ms) == 0 {
		fmt.Fprintln(w, "no reference matched")
		return
	}
	fmt.Fprint(w, matchTable(ms, timed, u).String())
	for _, m := range ms {
		if len(m.Missed) == 0 && len(m.Excursions) == 0 && len(m.Stops) == 0 {
			continue
		}
		d := u.Distance.FromSI
		fmt.Fprintf(w, "\n%s, %.2f-%s:\n", m.Reference, d(m.From), distance(m.To, u))
		for _, s := range m.Missed {
			fmt.Fprintf(w, "  missed the course from %.2f to %s along it, up to %s off\n", d(s.From), distance(s.To, u), short(s.Farthest, u))
		}
		for _, s := range m.Excursions {
			fmt.Fprintf(w, "  left it from %.2f to %s into the stretch, up to %s off\n", d(s.From), distance(s.To, u), short(s.Farthest, u))
		}
		for _, s := range m.Stops {
			fmt.Fprintf(w, "  stopped at %s for %s, %s along the course, %s from it\n", clock(s.From), s.Duration.Round(time.Second), distance(s.At, u), short(s.Off, u))
		}
	}
}
