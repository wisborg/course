package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/output"
)

// unitOpts are the units every command shows its numbers in: a system, and
// a unit for any quantity that is to differ from it.
var unitOpts struct {
	system string
	each   []string
}

func init() {
	f := root.PersistentFlags()
	f.StringVar(&unitOpts.system, "units", "metric", "the units numbers are shown in: metric or imperial; for a map the same as --set units.system=...")
	f.StringArrayVar(&unitOpts.each, "unit", nil, "one quantity's unit, over --units: distance=km|mi|nmi, elevation=m|ft (also short distances, such as how far off a course), speed=km/h|mph|kn|m/s, pace=min/km|min/mi; repeat for several, as a flight's --units imperial --unit distance=nmi --unit speed=kn; for a map the same as --set units.QUANTITY=...")
}

// unitsFor is the units the flags ask for: --units, then every --unit over it.
// A map takes them through its style instead, so that a theme can keep
// them; see buildStyle.
func unitsFor(cmd *cobra.Command) (units.Set, error) {
	set, err := units.Of(units.System(unitOpts.system))
	if err != nil {
		return set, fmt.Errorf("--units: %w", err)
	}
	for _, e := range unitOpts.each {
		q, name, err := splitUnit(e)
		if err != nil {
			return set, err
		}
		if err := set.Use(q, name); err != nil {
			return set, fmt.Errorf("--unit %s: %w", e, err)
		}
	}
	return set, nil
}

// unitsShown is the units a command's output in format f is written in:
// the ones asked for, for a person reading text; metric for CSV, JSON and
// YAML, which other programs read, and which keep the units they always had.
func unitsShown(cmd *cobra.Command, f output.Format) (units.Set, error) {
	u, err := unitsFor(cmd)
	if err != nil || f == output.Text {
		return u, err
	}
	return units.Of(units.Metric)
}

// splitUnit is one --unit, quantity=unit, split.
func splitUnit(e string) (units.Quantity, string, error) {
	q, name, ok := strings.Cut(e, "=")
	if !ok || q == "" || name == "" {
		return "", "", fmt.Errorf("--unit %q: give a quantity and its unit, such as distance=mi", e)
	}
	return units.Quantity(q), name, nil
}

// distance is d metres in u's distance unit, with the unit: "5.09 km".
func distance(d float64, u units.Set) string {
	return fmt.Sprintf("%.2f %s", u.Distance.FromSI(d), u.Distance.Name)
}

// short is d metres in u's elevation unit, for short distances such as how
// far off a course: "12 m", "39 ft".
func short(d float64, u units.Set) string {
	return fmt.Sprintf("%.0f %s", u.Elevation.FromSI(d), u.Elevation.Name)
}
