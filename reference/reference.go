// Package reference keeps courses to compare others against: the parkrun
// run eighty times, a standard loop from home, an organiser's official
// course. Each is stored as the file it was made from, byte for byte, with a
// manifest beside it saying what it is called and which part of the file is
// the course.
//
// The original rather than a converted track, because a comparison of two
// runs along a course needs everything the recording has, and every
// conversion drops something; and because a reference is a claim about where
// a course goes, and the file is the evidence for it.
//
// A reference averaged from several runs is the one exception, being made
// rather than recorded: it is stored as a GPX of the averaged line and its
// times, and the runs it was averaged from are stored beside it, byte for
// byte, as its evidence.
package reference

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/wisborg/course"
	"github.com/wisborg/course/match"
)

// Manifest is what a stored reference is: its reference.json.
type Manifest struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	// Source is the stored file's name in the reference's directory.
	Source string `json:"source"`
	// Crop is which part of the file is the course; nil is all of it.
	Crop  *Crop  `json:"crop,omitempty"`
	Added string `json:"added"`
	Note  string `json:"note,omitempty"`
	// Summary is derived from the file and the crop, and recomputed when
	// missing. It is there so a reference can be ruled out -- by where it is
	// and how long -- without reading its file.
	Summary *Summary `json:"summary,omitempty"`
	// Runs are, for a reference averaged from several, the runs it was
	// averaged from, the first being the course the others were aligned
	// to; nil for one stored from a file.
	Runs []Run `json:"runs,omitempty"`

	dir string
}

// Run is one of the runs an averaged reference was made from.
type Run struct {
	// File is the stored copy's name in the reference's directory, and
	// From the name of the file it was copied from.
	File string `json:"file"`
	From string `json:"from"`
	// Crop is which part of the first run is the course.
	Crop *Crop `json:"crop,omitempty"`
	// Used says the run followed the course and is in the average; a run
	// that did not is kept, and left out of it.
	Used bool `json:"used"`
}

// Crop is part of a course, by distance along it or by time since its start;
// one or the other. A To of zero is the end.
type Crop struct {
	FromM float64 `json:"from_m,omitempty"`
	ToM   float64 `json:"to_m,omitempty"`
	FromS float64 `json:"from_s,omitempty"`
	ToS   float64 `json:"to_s,omitempty"`
}

func (c *Crop) byTime() bool { return c.FromS != 0 || c.ToS != 0 }

// Summary is a reference's length, where it is, and whether it ends where it
// starts.
type Summary struct {
	LengthM float64 `json:"length_m"`
	Loop    bool    `json:"loop"`
	West    float64 `json:"west"`
	South   float64 `json:"south"`
	East    float64 `json:"east"`
	North   float64 `json:"north"`
}

// Store is a directory of references.
type Store struct {
	Dir string
}

// DefaultDir is where references are kept when a command names no other
// place: under the user's configuration directory, since they are the user's
// own and not a cache the system may empty.
func DefaultDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "course", "references"), nil
}

// manifestFile is each reference's manifest.
const manifestFile = "reference.json"

// Slug is the directory a name is stored under: lower case, words joined by
// hyphens, nothing but letters, digits and hyphens -- in any script, so
// "Fælledparken" is stored as "fælledparken".
func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// Add stores the course in the file at path as a reference called name.
//
// The file is copied in unchanged; the crop, if any, is recorded, and the
// course it leaves is read now, so a file that is not a course, or a crop
// that leaves nothing, is refused before anything is written. A name, an
// alias or a slug another reference already answers to is refused too: a
// name that finds two references finds neither.
func (s Store) Add(name, path string, aliases []string, crop *Crop, note string) (Manifest, error) {
	slug, err := s.claim(name, aliases)
	if err != nil {
		return Manifest{}, err
	}
	m := Manifest{
		Name: name, Aliases: aliases, Source: "source" + strings.ToLower(filepath.Ext(path)),
		Crop: crop, Added: time.Now().Format("2006-01-02"), Note: note,
	}
	c, err := read(path, crop)
	if err != nil {
		return Manifest{}, err
	}
	m.Summary = summarise(c)

	dir := filepath.Join(s.Dir, slug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Manifest{}, err
	}
	if err := copyFile(path, filepath.Join(dir, m.Source)); err != nil {
		os.RemoveAll(dir)
		return Manifest{}, err
	}
	m.dir = dir
	if err := m.write(); err != nil {
		os.RemoveAll(dir)
		return Manifest{}, err
	}
	return m, nil
}

// claim is the slug a new reference called name, with aliases, is stored
// under, or why it cannot be: nothing to make one of, or a name, alias or
// slug another reference already answers to.
func (s Store) claim(name string, aliases []string) (string, error) {
	slug := Slug(name)
	if slug == "" {
		return "", fmt.Errorf("%q has nothing a reference can be stored under; use letters or digits", name)
	}
	existing, err := s.List()
	if err != nil {
		return "", err
	}
	for _, want := range append([]string{name, slug}, aliases...) {
		for _, m := range existing {
			if m.answers(want) {
				return "", fmt.Errorf("%q is already taken by the reference %q", want, m.Name)
			}
		}
	}
	return slug, nil
}

// AddAverage stores the average of the runs in the files at paths as a
// reference called name: the first file, cropped as crop says, is the
// course, and the rest are aligned to it and averaged with it, as
// match.Average does with o. The averaged line is stored as a GPX, the runs
// beside it as they were, and the manifest says which runs are in the
// average. As with Add, nothing is written until every file has been read
// and the average made.
func (s Store) AddAverage(name string, paths []string, aliases []string, crop *Crop, note string, o match.Options) (Manifest, match.Averaged, error) {
	slug, err := s.claim(name, aliases)
	if err != nil {
		return Manifest{}, match.Averaged{}, err
	}
	runs := make([]*course.Course, len(paths))
	for i, path := range paths {
		c := crop
		if i > 0 {
			c = nil // matching finds the course in the others
		}
		if runs[i], err = read(path, c); err != nil {
			return Manifest{}, match.Averaged{}, err
		}
	}
	avg, err := match.Average(runs, o)
	if err != nil {
		return Manifest{}, match.Averaged{}, err
	}
	m := Manifest{
		Name: name, Aliases: aliases, Source: "average.gpx",
		Added: time.Now().Format("2006-01-02"), Note: note, Summary: summarise(avg.Course),
	}
	for i, path := range paths {
		run := Run{File: fmt.Sprintf("run-%d%s", i+1, strings.ToLower(filepath.Ext(path))), From: filepath.Base(path), Used: avg.Runs[i].Used}
		if i == 0 {
			run.Crop = crop
		}
		m.Runs = append(m.Runs, run)
	}

	dir := filepath.Join(s.Dir, slug)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Manifest{}, match.Averaged{}, err
	}
	fail := func(err error) (Manifest, match.Averaged, error) {
		os.RemoveAll(dir)
		return Manifest{}, match.Averaged{}, err
	}
	if err := writeGPX(filepath.Join(dir, m.Source), name, avg.Course); err != nil {
		return fail(err)
	}
	for i, path := range paths {
		if err := copyFile(path, filepath.Join(dir, m.Runs[i].File)); err != nil {
			return fail(err)
		}
	}
	m.dir = dir
	if err := m.write(); err != nil {
		return fail(err)
	}
	return m, avg, nil
}

// List is every reference in the store, by name. A store that does not exist
// yet holds none; a directory without a manifest is not a reference and is
// passed over.
func (s Store) List() ([]Manifest, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Manifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(s.Dir, e.Name())
		data, err := os.ReadFile(filepath.Join(dir, manifestFile))
		if err != nil {
			continue
		}
		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("the reference in %s: %w", dir, err)
		}
		m.dir = dir
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// Find is the reference called name -- its name, one of its aliases, or its
// slug, in any case.
func (s Store) Find(name string) (Manifest, error) {
	all, err := s.List()
	if err != nil {
		return Manifest{}, err
	}
	for _, m := range all {
		if m.answers(name) {
			return m, nil
		}
	}
	return Manifest{}, fmt.Errorf("no reference called %q in %s", name, s.Dir)
}

func (m Manifest) answers(name string) bool {
	if strings.EqualFold(m.Name, name) || filepath.Base(m.dir) == Slug(name) && Slug(name) != "" {
		return true
	}
	for _, a := range m.Aliases {
		if strings.EqualFold(a, name) {
			return true
		}
	}
	return false
}

// Remove deletes the reference called name, its file with it.
func (s Store) Remove(name string) (Manifest, error) {
	m, err := s.Find(name)
	if err != nil {
		return m, err
	}
	return m, os.RemoveAll(m.dir)
}

// Course is the reference's course: its file, cropped.
func (m Manifest) Course() (*course.Course, error) {
	return read(filepath.Join(m.dir, m.Source), m.Crop)
}

// File is where the reference's original file is stored.
func (m Manifest) File() string { return filepath.Join(m.dir, m.Source) }

func read(path string, crop *Crop) (*course.Course, error) {
	c, err := course.Read(path)
	if err != nil {
		return nil, err
	}
	if crop != nil {
		if crop.byTime() {
			c, err = c.CropTime(time.Duration(crop.FromS*float64(time.Second)), time.Duration(crop.ToS*float64(time.Second)))
		} else {
			c, err = c.CropDistance(crop.FromM, crop.ToM)
		}
		if err != nil {
			return nil, err
		}
	}
	if len(c.Points) < 2 {
		return nil, fmt.Errorf("%s has no line to be a reference", path)
	}
	return c, nil
}

// loopWithin is how near its start a course must end to be a loop: a
// parkrun's finish funnel is rarely on its start line.
const loopWithin = 150.0

func summarise(c *course.Course) *Summary {
	s := &Summary{LengthM: c.Length(), West: 180, South: 90, East: -180, North: -90}
	for _, p := range c.Points {
		s.West, s.East = min(s.West, p.Lon), max(s.East, p.Lon)
		s.South, s.North = min(s.South, p.Lat), max(s.North, p.Lat)
	}
	a, b := c.Points[0], c.Points[len(c.Points)-1]
	s.Loop = course.Metres(a.Lat, a.Lon, b.Lat, b.Lon) <= loopWithin
	return s
}

func (m Manifest) write() error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(m.dir, manifestFile+".tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(m.dir, manifestFile))
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
