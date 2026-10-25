package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/output"

	"github.com/spf13/cobra"

	"github.com/wisborg/cobrakit/strava"
	"github.com/wisborg/fitactivity"

	"github.com/wisborg/course"
)

func init() {
	strava.Add(root)
}

// loader reads a command's courses through cobrakit's strava.Loader, which
// fitdash reads its activities through too: a file as fitactivity reads it,
// and strava:ID from the athlete's own activities on Strava, with what more
// f asks for. Strava is asked only for a strava: source or with f.Attach, so
// a user with no Strava application set up never meets one.
func loader(cmd *cobra.Command, f strava.Fetch) course.Loader {
	return strava.Loader(cmd, fitactivity.Read, f)
}

// writeTitle writes the activity's title above its summary, where it has
// one.
func writeTitle(w io.Writer, c *course.Course) {
	if c.Title != "" {
		fmt.Fprintf(w, "%s\n\n", c.Title)
	}
}

// writeStrava writes, for --detailed, what Strava says about the activity
// beyond its title: its page; what it was marked as, how hard it was and
// what it was done in; what was written about it; and the segments it went
// through, each with when it entered and how long it took.
func writeStrava(w io.Writer, c *course.Course, u units.Set) {
	if c.Strava == nil && c.Description == "" {
		return
	}
	fmt.Fprintln(w)
	if a := c.Strava; a != nil {
		fmt.Fprintf(w, "strava    %s\n", a.URL)
		if kind := marked(a); kind != "" {
			fmt.Fprintf(w, "marked    %s\n", kind)
		}
		var effort []string
		if a.HasRelativeEffort {
			effort = append(effort, fmt.Sprintf("Relative Effort %.0f", a.RelativeEffort))
		}
		if a.HasPerceivedExertion {
			effort = append(effort, fmt.Sprintf("perceived exertion %g of 10", a.PerceivedExertion))
		}
		if len(effort) > 0 {
			fmt.Fprintf(w, "effort    %s\n", strings.Join(effort, ", "))
		}
		var energy []string
		if a.HasCalories {
			energy = append(energy, fmt.Sprintf("%.0f kcal", a.Calories))
		}
		if a.HasKilojoules {
			energy = append(energy, fmt.Sprintf("%.0f kJ of work", a.Kilojoules))
		}
		if len(energy) > 0 {
			fmt.Fprintf(w, "energy    %s, by Strava's reckoning\n", strings.Join(energy, ", "))
		}
		if g := a.Gear; g != nil {
			fmt.Fprintf(w, "gear      %s\n", gearLine(g, u))
		}
	}
	if c.Description != "" {
		fmt.Fprintln(w)
		for _, line := range strings.Split(c.Description, "\n") {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
	if c.Strava == nil || len(c.Strava.Segments) == 0 {
		return
	}
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "segment\tentered\ttook")
	for _, sg := range c.Strava.Segments {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", sg.Name, clock(sg.Start.Sub(c.Start)), clock(sg.End.Sub(sg.Start)))
	}
	tw.Flush()
}

// marked is what the activity was marked as: "race, commute".
func marked(a *fitactivity.StravaActivity) string {
	var kind []string
	for _, k := range []struct {
		name string
		on   bool
	}{{a.WorkoutType, a.WorkoutType != ""}, {"commute", a.Commute}, {"on a trainer", a.Trainer}, {"entered by hand", a.Manual}} {
		if k.on {
			kind = append(kind, k.name)
		}
	}
	return strings.Join(kind, ", ")
}

// gearLine is a piece of gear as far as it is known: "Daily Trainers
// (Brand Model 2), 812.40 km, retired".
func gearLine(g *fitactivity.StravaGear, u units.Set) string {
	name := g.Name
	if name == "" {
		name = g.ID
	}
	if bm := strings.TrimSpace(g.Brand + " " + g.Model); bm != "" && bm != name {
		name += " (" + bm + ")"
	}
	parts := []string{name}
	if g.Distance > 0 {
		parts = append(parts, distance(g.Distance, u)+" in all")
	}
	if g.Retired {
		parts = append(parts, "retired")
	}
	if g.Description != "" {
		parts = append(parts, g.Description)
	}
	return strings.Join(parts, ", ")
}

type jsonSegment struct {
	ID       int64   `json:"id" yaml:"id"`
	Name     string  `json:"name" yaml:"name"`
	StartS   float64 `json:"start_s" yaml:"start_s"`
	ElapsedS float64 `json:"elapsed_s" yaml:"elapsed_s"`
}

type jsonGear struct {
	ID          string  `json:"id" yaml:"id"`
	Name        string  `json:"name,omitempty" yaml:"name,omitempty"`
	DistanceM   float64 `json:"distance_m,omitempty" yaml:"distance_m,omitempty"`
	Brand       string  `json:"brand,omitempty" yaml:"brand,omitempty"`
	Model       string  `json:"model,omitempty" yaml:"model,omitempty"`
	Description string  `json:"description,omitempty" yaml:"description,omitempty"`
	Retired     bool    `json:"retired" yaml:"retired"`
}

type jsonStrava struct {
	ID                int64         `json:"id" yaml:"id"`
	URL               string        `json:"url" yaml:"url"`
	PhotoURL          string        `json:"photo_url,omitempty" yaml:"photo_url,omitempty"`
	Kudos             int           `json:"kudos" yaml:"kudos"`
	Comments          int           `json:"comments" yaml:"comments"`
	RelativeEffort    *float64      `json:"relative_effort,omitempty" yaml:"relative_effort,omitempty"`
	WorkoutType       string        `json:"workout_type,omitempty" yaml:"workout_type,omitempty"`
	Commute           bool          `json:"commute" yaml:"commute"`
	Trainer           bool          `json:"trainer" yaml:"trainer"`
	Manual            bool          `json:"manual" yaml:"manual"`
	PerceivedExertion *float64      `json:"perceived_exertion,omitempty" yaml:"perceived_exertion,omitempty"`
	CaloriesKcal      *float64      `json:"calories_kcal,omitempty" yaml:"calories_kcal,omitempty"`
	WorkKJ            *float64      `json:"work_kj,omitempty" yaml:"work_kj,omitempty"`
	Gear              *jsonGear     `json:"gear,omitempty" yaml:"gear,omitempty"`
	Segments          []jsonSegment `json:"segments" yaml:"segments"`
}

// jsonStravaOf is the activity on Strava for JSON and YAML, each segment's
// start in seconds into the course, as every row's is.
func jsonStravaOf(c *course.Course) *jsonStrava {
	a := c.Strava
	if a == nil {
		return nil
	}
	j := &jsonStrava{ID: a.ID, URL: a.URL, PhotoURL: a.PhotoURL, Kudos: a.Kudos, Comments: a.Comments, Segments: []jsonSegment{},
		WorkoutType: a.WorkoutType, Commute: a.Commute, Trainer: a.Trainer, Manual: a.Manual}
	if a.HasRelativeEffort {
		v := a.RelativeEffort
		j.RelativeEffort = &v
	}
	if a.HasPerceivedExertion {
		v := a.PerceivedExertion
		j.PerceivedExertion = &v
	}
	if a.HasCalories {
		v := a.Calories
		j.CaloriesKcal = &v
	}
	if a.HasKilojoules {
		v := a.Kilojoules
		j.WorkKJ = &v
	}
	if g := a.Gear; g != nil {
		j.Gear = &jsonGear{ID: g.ID, Name: g.Name, DistanceM: g.Distance, Brand: g.Brand, Model: g.Model, Description: g.Description, Retired: g.Retired}
	}
	for _, sg := range a.Segments {
		j.Segments = append(j.Segments, jsonSegment{ID: sg.ID, Name: sg.Name,
			StartS: sg.Start.Sub(c.Start).Seconds(), ElapsedS: sg.End.Sub(sg.Start).Seconds()})
	}
	return j
}

// writeUnplaced is the summary of an activity with no positions -- a ride on
// a trainer, a treadmill run -- that Strava knows: no places, as there is
// nowhere to look up, but what Strava says about it, as it would be beside
// the places of one outdoors.
func writeUnplaced(cmd *cobra.Command, c *course.Course) error {
	f := summaryOpts.format.Format
	u, err := unitsShown(cmd, f)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if f != output.Text {
		doc := jsonUnplacedDoc{Sources: c.Sources, Sport: c.Sport, Title: c.Title, Description: c.Description,
			Strava: jsonStravaOf(c), Rows: []jsonRow{}, Dropped: c.Dropped, Credits: []string{}}
		return output.Document{Data: doc}.Write(out, f)
	}
	writeTitle(out, c)
	fmt.Fprintln(out, "(no positions recorded, so no places)")
	if summaryOpts.detailed {
		writeStrava(out, c, u)
	}
	return nil
}

// jsonUnplacedDoc is an activity with no positions for JSON and YAML: a
// summary's fields where they mean anything -- no rows, no line -- and what
// Strava says about it.
type jsonUnplacedDoc struct {
	Sources     []string    `json:"sources" yaml:"sources"`
	Sport       string      `json:"sport,omitempty" yaml:"sport,omitempty"`
	OneLiner    string      `json:"one_liner" yaml:"one_liner"`
	Rows        []jsonRow   `json:"rows" yaml:"rows"`
	Title       string      `json:"title,omitempty" yaml:"title,omitempty"`
	Description string      `json:"description,omitempty" yaml:"description,omitempty"`
	Strava      *jsonStrava `json:"strava,omitempty" yaml:"strava,omitempty"`
	Dropped     int         `json:"rogue_fixes_left_out" yaml:"rogue_fixes_left_out"`
	Credits     []string    `json:"credits" yaml:"credits"`
}
