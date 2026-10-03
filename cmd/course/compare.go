package main

import (
	"fmt"
	"io"
	"math"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/output"
	"github.com/wisborg/output/table"

	"github.com/wisborg/course"
	"github.com/wisborg/course/compare"
	"github.com/wisborg/course/match"
)

var compareOpts struct {
	reference string
	near      float64
	split     float64
	format    formatFlag
}

var compareCmd = &cobra.Command{
	Use:   "compare ACTIVITY --reference REFERENCE",
	Short: "Compare a run with another run of the same course",
	Long: `compare puts a run beside a reference run of the same course -- a stored
reference or a file -- and says where it gained and lost time: split by split
along the course, and how far ahead or behind it was at the end of each.

The two are lined up by where on the course each was, not by time or by the
distance each recorded, so a wide corner or a GPS fix out of place does not
put them out of step; the splits are pieces of the reference, the same ground
for both. Only the stretch of the run that followed the reference is compared
-- a warm-up or a cool-down around it is left out -- and where either run
stopped on the way is listed under the splits, since a stop is usually why a
split was slow, or, in the reference, why one looks fast.

The reference must be a run: it needs its times. A course file with none is
refused. Some route planners write times into a course at a pace of their
own choosing; those are compared with as if somebody had run them.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runCompare,
}

func init() {
	f := compareCmd.Flags()
	f.StringVar(&compareOpts.reference, "reference", "", "the run to compare with: a stored reference's name or a file (required)")
	f.StringVar(&referencesDir, "references", "", "the directory stored references are kept in (default: course/references in your configuration directory)")
	f.Float64Var(&compareOpts.near, "near", 0, "how far from the course, in metres, is still on it (default 25)")
	f.Float64Var(&compareOpts.split, "split", 1, "how long each split is along the reference, in the distance unit: kilometres, or miles with --units imperial")
	compareOpts.format = formatFlag{Format: output.Text}
	f.Var(&compareOpts.format, "format", "output format: text, csv, json or yaml")
	compareCmd.MarkFlagRequired("reference")
	root.AddCommand(compareCmd)
}

func runCompare(cmd *cobra.Command, args []string) error {
	u, err := unitsFor(cmd)
	if err != nil {
		return err
	}
	shown, err := unitsShown(cmd, compareOpts.format.Format)
	if err != nil {
		return err
	}
	if !(compareOpts.split > 0) {
		return fmt.Errorf("--split must be more than 0 %s, not %v", u.Distance.Name, compareOpts.split)
	}
	c, err := course.Read(args...)
	if err != nil {
		return err
	}
	name, ref, err := resolveCourse(compareOpts.reference)
	if err != nil {
		return fmt.Errorf("--reference %w", err)
	}
	p, err := compare.Against(c, ref, match.Options{Near: compareOpts.near})
	if err != nil {
		return fmt.Errorf("against %s: %w", name, err)
	}
	// --split is in the distance unit: a mile a split under imperial.
	splits := p.Splits(u.Distance.ToSI(compareOpts.split))
	if compareOpts.format.Format == output.Text {
		writeComparison(cmd.OutOrStdout(), name, p, splits, u)
		return nil
	}
	return output.Document{Data: jsonComparison(name, p, splits), Table: splitTable(splits, shown)}.Write(cmd.OutOrStdout(), compareOpts.format.Format)
}

// gapWords is a gap in words: behind, ahead, or level.
func gapWords(gap time.Duration) string {
	gap = gap.Round(time.Second)
	switch {
	case gap > 0:
		return fmt.Sprintf("%v behind", gap)
	case gap < 0:
		return fmt.Sprintf("%v ahead", -gap)
	}
	return "level"
}

// splitWords is how a split compared, in words: slower, faster, or level.
func splitWords(run, ref time.Duration) string {
	d := (run - ref).Round(time.Second)
	switch {
	case d > 0:
		return fmt.Sprintf("%v slower", d)
	case d < 0:
		return fmt.Sprintf("%v faster", -d)
	}
	return "level"
}

// splitTable has a row a split: where it ends, both times over it, and how
// they compare. Words rather than signed numbers, so nobody has to remember
// which way plus means.
func splitTable(splits []compare.Split, u units.Set) *table.Table {
	t := table.New(
		table.Column{Header: u.Distance.Name, Align: table.Right, Format: "%.2f"},
		table.Column{Header: "run", Align: table.Right},
		table.Column{Header: "reference", Align: table.Right},
		table.Column{Header: "split", Align: table.Right},
		table.Column{Header: "gap", Align: table.Right},
	)
	for _, s := range splits {
		t.MustAppend(u.Distance.FromSI(s.To), clock(s.Run), clock(s.Ref), splitWords(s.Run, s.Ref), gapWords(s.Gap))
	}
	return t
}

// writeComparison is the comparison for a person: which stretch of the run
// was compared, the splits, and the stops under them.
func writeComparison(w io.Writer, name string, p *compare.Profile, splits []compare.Split, u units.Set) {
	m := p.Match
	last := len(p.Run) - 1
	length := float64(last) * p.Step
	of := distance(length, u) + " of it"
	if p.From > 0 {
		// The run joined the reference late; say where, or the splits'
		// distances would seem to start from nowhere.
		of = fmt.Sprintf("%s of it, from %.2f to %s along", distance(length, u), u.Distance.FromSI(p.From), distance(p.From+length, u))
	}
	fmt.Fprintf(w, "against %s, %s, from %s to %s into the run: %s\n\n",
		name, of, clock(m.FromTime), clock(m.ToTime), gapText(p.Gap(last)))
	fmt.Fprint(w, splitTable(splits, u).String())
	// Both runs' stops, in the order they come along the course, so a stop
	// is read beside the split it is in.
	type stop struct {
		at   float64
		line string
	}
	var stops []stop
	for _, s := range m.Stops {
		stops = append(stops, stop{s.At, fmt.Sprintf("stopped at %s for %s, %s along the course", clock(s.From), s.Duration.Round(time.Second), distance(s.At, u))})
	}
	for _, s := range p.RefStops {
		stops = append(stops, stop{s.At, fmt.Sprintf("the reference stopped for %s, %s along the course", s.Duration.Round(time.Second), distance(s.At, u))})
	}
	sort.SliceStable(stops, func(i, j int) bool { return stops[i].at < stops[j].at })
	if len(stops) > 0 {
		fmt.Fprintln(w)
	}
	for _, s := range stops {
		fmt.Fprintln(w, s.line)
	}
}

type jsonSplit struct {
	FromM float64 `json:"from_m"`
	ToM   float64 `json:"to_m"`
	RunS  float64 `json:"run_s"`
	RefS  float64 `json:"reference_s"`
	GapS  float64 `json:"gap_s"`
}

type jsonComparisonDoc struct {
	Reference string      `json:"reference"`
	FromM     float64     `json:"from_m"`
	ToM       float64     `json:"to_m"`
	FromS     float64     `json:"from_s"`
	ToS       float64     `json:"to_s"`
	AlongM    float64     `json:"along_m"`
	LengthM   float64     `json:"length_m"`
	GapS      float64     `json:"gap_s"`
	Splits    []jsonSplit `json:"splits"`
	Stops     []jsonStop  `json:"stops"`
	RefStops  []jsonStop  `json:"reference_stops"`
}

// jsonComparison is the comparison as the other commands' JSON is written:
// distances in metres, times in seconds, and a gap positive when the run was
// behind.
func jsonComparison(name string, p *compare.Profile, splits []compare.Split) jsonComparisonDoc {
	m := p.Match
	last := len(p.Run) - 1
	secs := func(d time.Duration) float64 { return math.Round(d.Seconds()) }
	j := jsonComparisonDoc{
		Reference: name, FromM: round1(m.From), ToM: round1(m.To),
		FromS: secs(m.FromTime), ToS: secs(m.ToTime),
		AlongM: round1(p.From), LengthM: round1(float64(last) * p.Step), GapS: secs(p.Gap(last)),
		Splits: []jsonSplit{}, Stops: []jsonStop{}, RefStops: []jsonStop{},
	}
	for _, s := range splits {
		j.Splits = append(j.Splits, jsonSplit{FromM: round1(s.From), ToM: round1(s.To), RunS: secs(s.Run), RefS: secs(s.Ref), GapS: secs(s.Gap)})
	}
	for _, st := range m.Stops {
		j.Stops = append(j.Stops, jsonStop{AtM: round1(st.At), FromS: secs(st.From), DurationS: secs(st.Duration), OffM: round1(st.Off)})
	}
	for _, st := range p.RefStops {
		j.RefStops = append(j.RefStops, jsonStop{AtM: round1(st.At), FromS: secs(st.From), DurationS: secs(st.Duration)})
	}
	return j
}
