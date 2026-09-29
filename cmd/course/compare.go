package main

import (
	"fmt"
	"io"
	"math"
	"time"

	"github.com/spf13/cobra"

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
-- a warm-up or a cool-down around it is left out -- and where the run stopped
on the way is listed under the splits, since a stop is usually why a split was
slow.

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
	f.Float64Var(&compareOpts.split, "split", 1, "how long each split is, in kilometres along the reference")
	compareOpts.format = formatFlag{Format: output.Text}
	f.Var(&compareOpts.format, "format", "output format: text, csv, json or yaml")
	compareCmd.MarkFlagRequired("reference")
	root.AddCommand(compareCmd)
}

func runCompare(cmd *cobra.Command, args []string) error {
	if !(compareOpts.split > 0) {
		return fmt.Errorf("--split must be more than 0 km, not %v", compareOpts.split)
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
	splits := p.Splits(1000 * compareOpts.split)
	if compareOpts.format.Format == output.Text {
		writeComparison(cmd.OutOrStdout(), name, p, splits)
		return nil
	}
	return output.Document{Data: jsonComparison(name, p, splits), Table: splitTable(splits)}.Write(cmd.OutOrStdout(), compareOpts.format.Format)
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
func splitTable(splits []compare.Split) *table.Table {
	t := table.New(
		table.Column{Header: "km", Align: table.Right, Format: "%.2f"},
		table.Column{Header: "run", Align: table.Right},
		table.Column{Header: "reference", Align: table.Right},
		table.Column{Header: "split", Align: table.Right},
		table.Column{Header: "gap", Align: table.Right},
	)
	for _, s := range splits {
		t.MustAppend(s.To/1000, clock(s.Run), clock(s.Ref), splitWords(s.Run, s.Ref), gapWords(s.Gap))
	}
	return t
}

// writeComparison is the comparison for a person: which stretch of the run
// was compared, the splits, and the stops under them.
func writeComparison(w io.Writer, name string, p *compare.Profile, splits []compare.Split) {
	m := p.Match
	last := len(p.Run) - 1
	fmt.Fprintf(w, "against %s, %.2f km of it, from %s to %s into the run: %s\n\n",
		name, float64(last)*p.Step/1000, clock(m.FromTime), clock(m.ToTime), gapText(p.Gap(last)))
	fmt.Fprint(w, splitTable(splits).String())
	if len(m.Stops) > 0 {
		fmt.Fprintln(w)
	}
	for _, s := range m.Stops {
		fmt.Fprintf(w, "stopped at %s for %s, %.2f km along the course\n", clock(s.From), s.Duration.Round(time.Second), s.At/1000)
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
	LengthM   float64     `json:"length_m"`
	GapS      float64     `json:"gap_s"`
	Splits    []jsonSplit `json:"splits"`
	Stops     []jsonStop  `json:"stops"`
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
		LengthM: round1(float64(last) * p.Step), GapS: secs(p.Gap(last)),
		Splits: []jsonSplit{}, Stops: []jsonStop{},
	}
	for _, s := range splits {
		j.Splits = append(j.Splits, jsonSplit{FromM: round1(s.From), ToM: round1(s.To), RunS: secs(s.Run), RefS: secs(s.Ref), GapS: secs(s.Gap)})
	}
	for _, st := range m.Stops {
		j.Stops = append(j.Stops, jsonStop{AtM: round1(st.At), FromS: secs(st.From), DurationS: secs(st.Duration), OffM: round1(st.Off)})
	}
	return j
}
