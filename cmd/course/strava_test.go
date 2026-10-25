package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/wisborg/fitactivity"
	fs "github.com/wisborg/fitactivity/strava"
	"github.com/wisborg/fitactivity/units"

	"github.com/wisborg/course"
)

func TestStravaID(t *testing.T) {
	for _, c := range []struct {
		in string
		id int64
		ok bool
	}{
		{"strava:1234", 1234, true},
		{"https://www.strava.com/activities/1234", 1234, true},
		{"https://www.strava.com/activities/1234/overview", 1234, true},
		{"strava:", 0, false},
		{"strava:abc", 0, false},
		{"run.fit", 0, false},
		{"/tmp/strava:1234.fit", 0, false},
	} {
		if id, ok := stravaID(c.in); id != c.id || ok != c.ok {
			t.Errorf("stravaID(%q) = %d, %v; want %d, %v", c.in, id, ok, c.id, c.ok)
		}
	}
	if got := nameOf("strava:1234"); got != "strava-1234" {
		t.Errorf("nameOf a Strava source is %q", got)
	}
}

// loggedInAt is a login and a cached answer for an invented activity, in a
// configuration and cache directory of the test's own, so a strava: source
// is read without asking Strava anything.
func loggedInAt(t *testing.T) {
	t.Helper()
	config, cache := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv(fs.EnvConfig, "")
	t.Setenv(fs.EnvClientID, "4242")
	t.Setenv(fs.EnvClientSecret, "invented")
	if err := fs.SaveToken(filepath.Join(config, "strava-token.json"), fs.Token{
		ClientID: "4242", AthleteID: 77, AccessToken: "a", RefreshToken: "r",
		ExpiresAt: time.Now().Add(6 * time.Hour), Scope: "read,activity:read_all",
	}); err != nil {
		t.Fatal(err)
	}
	answer := `{"id":1001,"name":"Invented Run","description":"Words.","sport_type":"Run","start_date":"2026-04-02T06:00:00Z",
		"segment_efforts":[{"name":"Invented Hill","start_date":"2026-04-02T06:00:01Z","elapsed_time":1,"segment":{"id":5}}],
		"streams":[{"type":"time","data":[0,1,2]},{"type":"latlng","data":[[10,20],[10.0001,20],[10.0002,20]]}]}`
	dir := filepath.Join(cache, "fitactivity", "strava", "77")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "1001.streams.json"), []byte(answer), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoader_AStravaSourceFromTheAthletesOwnActivities(t *testing.T) {
	loggedInAt(t)
	cmd := &cobra.Command{Use: "summary"}
	cmd.Flags().String("strava-config", "", "")
	c, err := course.ReadWith(loader(cmd, fromStrava{}), "strava:1001")
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Invented Run" || len(c.Points) != 3 || c.Strava == nil || len(c.Strava.Segments) != 1 {
		t.Errorf("course %q, %d points, strava %+v", c.Title, len(c.Points), c.Strava)
	}
}

func TestLoader_AFileAloneNeverAsksStrava(t *testing.T) {
	// No application set up anywhere: a file is read as ever.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(fs.EnvConfig, "")
	t.Setenv(fs.EnvClientID, "")
	t.Setenv(fs.EnvClientSecret, "")
	gpx := filepath.Join(t.TempDir(), "run.gpx")
	writeLine(t, gpx, 10, 20, 0.0002, 10)
	cmd := &cobra.Command{Use: "summary"}
	cmd.Flags().String("strava-config", "", "")
	if _, err := course.ReadWith(loader(cmd, fromStrava{}), gpx); err != nil {
		t.Fatalf("a file without --strava: %v", err)
	}
	_, err := course.ReadWith(loader(cmd, fromStrava{attach: true}), gpx)
	if err == nil || !strings.Contains(err.Error(), "no Strava API application") {
		t.Errorf("--strava without an application: %v", err)
	}
}

func TestWriteStrava(t *testing.T) {
	start := time.Date(2026, 4, 2, 6, 0, 0, 0, time.UTC)
	c := &course.Course{Start: start, Title: "Invented Run", Description: "Line one.\nLine two.",
		Strava: &fitactivity.StravaActivity{ID: 1001, URL: "https://www.strava.com/activities/1001", Kudos: 2,
			HasRelativeEffort: true, RelativeEffort: 8, WorkoutType: "race", Commute: true,
			Gear:     &fitactivity.StravaGear{ID: "g9", Name: "Daily Trainers", Brand: "Brand", Model: "Model 2", Distance: 812400},
			Segments: []fitactivity.StravaSegment{{ID: 5, Name: "Invented Hill", Start: start.Add(90 * time.Second), End: start.Add(150 * time.Second)}}}}
	var b strings.Builder
	writeTitle(&b, c)
	u, err := units.Of(units.Metric)
	if err != nil {
		t.Fatal(err)
	}
	writeStrava(&b, c, u)
	want := `Invented Run


strava    https://www.strava.com/activities/1001
marked    race, commute
effort    Relative Effort 8
gear      Daily Trainers (Brand Model 2), 812.40 km in all

  Line one.
  Line two.

segment        entered  took
Invented Hill  0:01:30  0:01:00
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
	j, err := json.Marshal(jsonStravaOf(c))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(j), `"start_s":90`) || !strings.Contains(string(j), `"elapsed_s":60`) || !strings.Contains(string(j), `"kudos":2`) ||
		!strings.Contains(string(j), `"relative_effort":8`) || !strings.Contains(string(j), `"workout_type":"race"`) ||
		!strings.Contains(string(j), `"commute":true`) || !strings.Contains(string(j), `"brand":"Brand"`) || strings.Contains(string(j), "perceived_exertion") {
		t.Errorf("json %s", j)
	}
}

func TestReferenceAdd_RefusesAStravaActivity(t *testing.T) {
	resetFlags(t, referenceAdd)
	_, err := run(t, "reference", "add", "--references", t.TempDir(), "Loop", "strava:1001")
	if err == nil || !strings.Contains(err.Error(), "API Policy") {
		t.Errorf("err %v", err)
	}
}
