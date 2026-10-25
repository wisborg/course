package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/wisborg/cobrakit/strava"
	"github.com/wisborg/fitactivity"
	fs "github.com/wisborg/fitactivity/strava"

	"github.com/wisborg/course"
)

func init() {
	strava.Add(root)
}

// stravaID is the activity a source names on Strava: strava:ID, or the
// activity's page, https://www.strava.com/activities/ID.
func stravaID(source string) (int64, bool) {
	rest, ok := strings.CutPrefix(source, "strava:")
	if !ok {
		for _, page := range []string{"https://www.strava.com/activities/", "https://strava.com/activities/"} {
			if rest, ok = strings.CutPrefix(source, page); ok {
				rest, _, _ = strings.Cut(rest, "/")
				break
			}
		}
	}
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	return id, err == nil && id > 0
}

// loader reads a command's courses: a file as fitactivity reads it, and
// strava:ID from the athlete's own activities on Strava. With attach, a
// file is also given Strava's details -- title, description, segments --
// from the activity that started when it did.
//
// Strava is asked only for a strava: source or with attach, and a client
// opened only then, so a user with no Strava application set up never meets
// one.
func loader(cmd *cobra.Command, attach bool) course.Loader {
	var client *fs.Client
	open := func() (*fs.Client, error) {
		if client != nil {
			return client, nil
		}
		c, err := strava.Open(cmd, cmd.ErrOrStderr())
		if err != nil {
			return nil, err
		}
		client = c
		return c, nil
	}
	return func(source string) (*fitactivity.Track, error) {
		if id, ok := stravaID(source); ok {
			c, err := open()
			if err != nil {
				return nil, err
			}
			t, err := c.Track(cmd.Context(), id)
			return t, strava.Explain(cmd, err)
		}
		t, err := fitactivity.Read(source)
		if err != nil || !attach {
			return t, err
		}
		c, err := open()
		if err != nil {
			return nil, err
		}
		s, err := c.Attach(cmd.Context(), t, filepath.Base(source))
		if err != nil {
			return nil, strava.Explain(cmd, err)
		}
		if s == nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "course: no Strava activity of yours started when %s did\n", source)
		}
		return t, nil
	}
}

// writeTitle writes the activity's title above its summary, where it has
// one.
func writeTitle(w io.Writer, c *course.Course) {
	if c.Title != "" {
		fmt.Fprintf(w, "%s\n\n", c.Title)
	}
}

// writeStrava writes, for --detailed, what Strava says about the activity
// beyond its title: its page, what was written about it, and the segments
// it went through, each with when it entered and how long it took.
func writeStrava(w io.Writer, c *course.Course) {
	if c.Strava == nil && c.Description == "" {
		return
	}
	fmt.Fprintln(w)
	if c.Strava != nil {
		fmt.Fprintf(w, "strava    %s\n", c.Strava.URL)
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

type jsonSegment struct {
	ID       int64   `json:"id" yaml:"id"`
	Name     string  `json:"name" yaml:"name"`
	StartS   float64 `json:"start_s" yaml:"start_s"`
	ElapsedS float64 `json:"elapsed_s" yaml:"elapsed_s"`
}

type jsonStrava struct {
	ID       int64         `json:"id" yaml:"id"`
	URL      string        `json:"url" yaml:"url"`
	PhotoURL string        `json:"photo_url,omitempty" yaml:"photo_url,omitempty"`
	Kudos    int           `json:"kudos" yaml:"kudos"`
	Comments int           `json:"comments" yaml:"comments"`
	Segments []jsonSegment `json:"segments" yaml:"segments"`
}

// jsonStravaOf is the activity on Strava for JSON and YAML, each segment's
// start in seconds into the course, as every row's is.
func jsonStravaOf(c *course.Course) *jsonStrava {
	a := c.Strava
	if a == nil {
		return nil
	}
	j := &jsonStrava{ID: a.ID, URL: a.URL, PhotoURL: a.PhotoURL, Kudos: a.Kudos, Comments: a.Comments, Segments: []jsonSegment{}}
	for _, sg := range a.Segments {
		j.Segments = append(j.Segments, jsonSegment{ID: sg.ID, Name: sg.Name,
			StartS: sg.Start.Sub(c.Start).Seconds(), ElapsedS: sg.End.Sub(sg.Start).Seconds()})
	}
	return j
}
