package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errw bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errw)
	root.SetArgs(args)
	defer root.SetArgs(nil)
	err := root.Execute()
	return out.String() + errw.String(), err
}

// A course is stored, listed, shown, drawn on a map by its alias, and
// removed -- all in the store --references names, and nowhere else.
func TestReferenceCommands(t *testing.T) {
	defer func(f func() bool) { stdinAnswerable = f }(stdinAnswerable)
	stdinAnswerable = func() bool { return false }
	for _, c := range []*cobra.Command{referenceAdd, referenceList, referenceShow, mapCmd} {
		resetFlags(t, c)
	}
	t.Cleanup(func() { referencesDir = "" })

	dir := t.TempDir()
	refs := filepath.Join(dir, "refs")
	course := filepath.Join(dir, "usual.gpx")
	writeLine(t, course, 10, 20, 0.0002, 60)

	out, err := run(t, "reference", "add", "--references", refs, "Usual Loop", course, "--alias", "usual", "--from-km", "0.2", "--note", "round the block")
	if err != nil || !strings.Contains(out, `stored "Usual Loop"`) {
		t.Fatalf("add: %v\n%s", err, out)
	}
	out, err = run(t, "reference", "list", "--references", refs, "--format", "json")
	var listed []map[string]any
	if err != nil || json.Unmarshal([]byte(out), &listed) != nil || len(listed) != 1 || listed[0]["name"] != "Usual Loop" {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if crop, _ := listed[0]["crop"].(map[string]any); crop["from_m"] != 200.0 {
		t.Errorf("the crop is not recorded: %v", listed[0]["crop"])
	}
	out, err = run(t, "reference", "show", "--references", refs, "--format", "text", "usual")
	if err != nil || !strings.Contains(out, "round the block") || !strings.Contains(out, "from 0.20 km to the end") {
		t.Errorf("show: %v\n%s", err, out)
	}

	png := filepath.Join(dir, "map.png")
	out, err = run(t, "map", "--store", filepath.Join(dir, "store"), "--references", refs, "--reference", "usual",
		"--out", png, "--width", "300", "--height", "200", course)
	if err != nil {
		t.Fatalf("map by the reference's alias: %v\n%s", err, out)
	}
	if _, err := run(t, "map", "--store", filepath.Join(dir, "store"), "--references", refs, "--reference", "nothing-by-this-name", "--out", png, course); err == nil ||
		!strings.Contains(err.Error(), "no such file") || !strings.Contains(err.Error(), "no reference called") {
		t.Errorf("a reference that is neither a file nor stored: %v", err)
	}

	resetNow(referenceAdd)
	if _, err := run(t, "reference", "add", "--references", refs, "Both", course, "--from", "10s", "--to-km", "1"); err == nil ||
		!strings.Contains(err.Error(), "not both") {
		t.Errorf("a crop by both time and distance: %v", err)
	}
	if out, err := run(t, "reference", "remove", "--references", refs, "Usual Loop"); err != nil || !strings.Contains(out, "removed") {
		t.Errorf("remove: %v\n%s", err, out)
	}
	if out, _ := run(t, "reference", "list", "--references", refs, "--format", "text"); !strings.Contains(out, "no references") {
		t.Errorf("an empty store lists as:\n%s", out)
	}
}

// Several files make an averaged reference: stored, said to be the average
// of the runs that followed the first, each with how closely, a run
// elsewhere kept and left out; show lists them.
func TestReferenceAddAverage(t *testing.T) {
	resetUnits(t)
	for _, c := range []*cobra.Command{referenceAdd, referenceShow} {
		resetFlags(t, c)
	}
	t.Cleanup(func() { referencesDir = "" })
	dir := t.TempDir()
	refs := filepath.Join(dir, "refs")
	a, b, far := filepath.Join(dir, "a.gpx"), filepath.Join(dir, "b.gpx"), filepath.Join(dir, "far.gpx")
	writePaced(t, a, 200, func(i int) int { return 10 * i })
	writePaced(t, b, 200, func(i int) int { return 11 * i })
	writeFile(t, far, strings.ReplaceAll(readFile(t, a), `lat="10"`, `lat="10.05"`))
	writePaced(t, filepath.Join(dir, "c.gpx"), 200, func(i int) int { return 12 * i })
	out, err := run(t, "reference", "add", "--references", refs, "Loop", a, b, filepath.Join(dir, "c.gpx"), far)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"the average of 3 runs", "a.gpx    100% of it", "far.gpx  does not follow the first; kept, and left out"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
	out, err = run(t, "reference", "show", "--references", refs, "Loop")
	if err != nil || !strings.Contains(out, "averaged a.gpx, the course the others were matched to (run-1.gpx)") || !strings.Contains(out, "far.gpx, left out: it does not follow the first (run-4.gpx)") {
		t.Errorf("show: %v\n%s", err, out)
	}
}
