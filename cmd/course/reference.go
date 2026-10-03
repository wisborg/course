package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/output"
	"github.com/wisborg/output/table"

	"github.com/wisborg/course/reference"
)

var referencesDir string

var referenceCmd = &cobra.Command{
	Use:   "reference",
	Short: "Keep courses to compare others against",
	Long: `reference keeps courses to draw beside others on a map: a parkrun you run
every week, a standard loop, an organiser's official course.

Each is stored by name as the file it came from, unchanged, and optionally
only part of it -- the parkrun out of a run that also had a warm-up and a
cool-down -- with --from and --to (times into it, as 7m30s) or --from-km and
--to-km. "course map --reference NAME" draws it.

They are kept in your configuration directory, and nowhere else.`,
}

var addOpts struct {
	aliases      []string
	from, to     time.Duration
	fromKM, toKM float64
	note         string
}

var referenceAdd = &cobra.Command{
	Use:   "add NAME FILE",
	Short: "Store a course as a reference",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		crop, err := cropFlags()
		if err != nil {
			return err
		}
		s, err := openReferences()
		if err != nil {
			return err
		}
		u, err := unitsFor(cmd)
		if err != nil {
			return err
		}
		m, err := s.Add(args[0], args[1], addOpts.aliases, crop, addOpts.note)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "stored %q, %s%s, in %s\n", m.Name, distance(m.Summary.LengthM, u), loopText(m.Summary), s.Dir)
		return nil
	},
}

// cropFlags is the crop --from/--to or --from-km/--to-km ask for, or nil.
func cropFlags() (*reference.Crop, error) {
	byTime := addOpts.from != 0 || addOpts.to != 0
	byDistance := addOpts.fromKM != 0 || addOpts.toKM != 0
	switch {
	case byTime && byDistance:
		return nil, errors.New("crop by time (--from, --to) or by distance (--from-km, --to-km), not both")
	case byTime:
		return &reference.Crop{FromS: addOpts.from.Seconds(), ToS: addOpts.to.Seconds()}, nil
	case byDistance:
		return &reference.Crop{FromM: addOpts.fromKM * 1000, ToM: addOpts.toKM * 1000}, nil
	}
	return nil, nil
}

var listFormat = formatFlag{Format: output.Text}

var referenceList = &cobra.Command{
	Use:   "list",
	Short: "List the stored references",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openReferences()
		if err != nil {
			return err
		}
		all, err := s.List()
		if err != nil {
			return err
		}
		u, err := unitsShown(cmd, listFormat.Format)
		if err != nil {
			return err
		}
		if len(all) == 0 && listFormat.Format == output.Text {
			fmt.Fprintf(cmd.OutOrStdout(), "no references in %s; add one with \"course reference add NAME FILE\"\n", s.Dir)
			return nil
		}
		t := table.New(
			table.Column{Header: "name"}, table.Column{Header: "also"},
			table.Column{Header: u.Distance.Name, Align: table.Right, Format: "%.2f"},
			table.Column{Header: "loop"}, table.Column{Header: "added"}, table.Column{Header: "note"},
		)
		for _, m := range all {
			length, loop := 0.0, ""
			if m.Summary != nil {
				length = u.Distance.FromSI(m.Summary.LengthM)
				if m.Summary.Loop {
					loop = "loop"
				}
			}
			t.MustAppend(m.Name, strings.Join(m.Aliases, ", "), length, loop, m.Added, m.Note)
		}
		if all == nil {
			all = []reference.Manifest{}
		}
		return output.Document{Data: all, Table: t}.Write(cmd.OutOrStdout(), listFormat.Format)
	},
}

var referenceShow = &cobra.Command{
	Use:   "show NAME",
	Short: "Say what a stored reference is",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openReferences()
		if err != nil {
			return err
		}
		m, err := s.Find(args[0])
		if err != nil {
			return err
		}
		if listFormat.Format != output.Text {
			return output.Document{Data: m}.Write(cmd.OutOrStdout(), listFormat.Format)
		}
		u, err := unitsFor(cmd)
		if err != nil {
			return err
		}
		writeManifest(cmd.OutOrStdout(), m, u)
		return nil
	},
}

func writeManifest(w io.Writer, m reference.Manifest, u units.Set) {
	fmt.Fprintf(w, "%-8s %s\n", "name", m.Name)
	if len(m.Aliases) > 0 {
		fmt.Fprintf(w, "%-8s %s\n", "also", strings.Join(m.Aliases, ", "))
	}
	if m.Summary != nil {
		fmt.Fprintf(w, "%-8s %s%s\n", "course", distance(m.Summary.LengthM, u), loopText(m.Summary))
	}
	if c := m.Crop; c != nil {
		if c.FromS != 0 || c.ToS != 0 {
			fmt.Fprintf(w, "%-8s from %v to %v into the file\n", "crop", secs(c.FromS), endOr(secs(c.ToS), c.ToS))
		} else {
			fmt.Fprintf(w, "%-8s from %s to %s into the file\n", "crop", distance(c.FromM, u), endOr(distance(c.ToM, u), c.ToM))
		}
	}
	fmt.Fprintf(w, "%-8s %s\n", "file", m.File())
	fmt.Fprintf(w, "%-8s %s\n", "added", m.Added)
	if m.Note != "" {
		fmt.Fprintf(w, "%-8s %s\n", "note", m.Note)
	}
}

func secs(s float64) string { return (time.Duration(s * float64(time.Second))).String() }

func endOr(s string, v float64) string {
	if v == 0 {
		return "the end"
	}
	return s
}

func loopText(s *reference.Summary) string {
	if s != nil && s.Loop {
		return ", a loop"
	}
	return ""
}

var referenceRemove = &cobra.Command{
	Use:   "remove NAME",
	Short: "Delete a stored reference and its file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openReferences()
		if err != nil {
			return err
		}
		m, err := s.Remove(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %q\n", m.Name)
		return nil
	},
}

// openReferences is the store --references names, or the default one.
func openReferences() (reference.Store, error) {
	dir := referencesDir
	if dir == "" {
		var err error
		if dir, err = reference.DefaultDir(); err != nil {
			return reference.Store{}, fmt.Errorf("finding where references are kept: %w; pass --references", err)
		}
	}
	return reference.Store{Dir: dir}, nil
}

func init() {
	referenceCmd.PersistentFlags().StringVar(&referencesDir, "references", "", "the directory references are kept in (default: course/references in your configuration directory)")
	f := referenceAdd.Flags()
	f.StringArrayVar(&addOpts.aliases, "alias", nil, "another name to find it by; repeat for several")
	f.DurationVar(&addOpts.from, "from", 0, "only the part of the file from this long into it, e.g. 7m30s")
	f.DurationVar(&addOpts.to, "to", 0, "only the part of the file up to this long into it")
	f.Float64Var(&addOpts.fromKM, "from-km", 0, "only the part of the file from this many kilometres into it")
	f.Float64Var(&addOpts.toKM, "to-km", 0, "only the part of the file up to this many kilometres into it")
	f.StringVar(&addOpts.note, "note", "", "a note to keep with it")
	for _, c := range []*cobra.Command{referenceList, referenceShow} {
		c.Flags().Var(&listFormat, "format", "output format: text, csv, json or yaml")
	}
	referenceCmd.AddCommand(referenceAdd, referenceList, referenceShow, referenceRemove)
	root.AddCommand(referenceCmd)
}
