// Package mapstyle is how a course's map looks: its palette, and the colour,
// width, opacity and style of its lines -- the course's, each activity's,
// and each reference's.
//
// A style is built in three layers, each changing only what it says: the
// built-in defaults, then a file (Load), then settings given one at a time
// on the command line (Set). The file is YAML; JSON, being YAML, will do as
// well. A key the style does not have is refused with its name, so that a
// misspelt setting is an error rather than a setting silently not made.
//
// The style is data. What "auto" resolves to -- the palette's own inks, an
// opacity that depends on whether a reference is drawn whole -- is decided by
// whoever draws the map.
package mapstyle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/wisborg/fitactivity/units"
)

// Style is everything about a map's look that can be set.
type Style struct {
	// Palette is the map's: light, dark or outdoors.
	Palette string `yaml:"palette"`
	// Width and Height are the picture's, in pixels. Everything drawn on
	// it is scaled with it, so a style's line widths look the same at any
	// size.
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
	// Units are the units the map's numbers are shown in: its distance
	// markers, its legend, its report.
	Units Units `yaml:"units"`
	// Text is the text course writes over the map.
	Text Text `yaml:"text"`
	// Map is the map under the course.
	Map Map `yaml:"map"`
	// View is where the map is seen from.
	View View `yaml:"view"`
	// Legend is the legend saying what each line or colour is.
	Legend Legend `yaml:"legend"`
	// Markers are the course's distance markers.
	Markers Markers `yaml:"markers"`
	// Colouring is what the course's line is coloured by, if anything.
	Colouring Colouring `yaml:"colouring"`
	// Course is how the course is drawn, and with --separate every
	// activity, unless Activities says otherwise for one.
	Course Line `yaml:"course"`
	// Activities are settings for single activities of a --separate map,
	// by position: the first entry is activity 1, as numbered on the map.
	// Each says only what differs from Course.
	Activities []Line `yaml:"activities"`
	// Reference is how every reference is drawn, unless References says
	// otherwise for one.
	Reference Line `yaml:"reference"`
	// References are settings for single references, by name -- a stored
	// reference's, a file's without its extension, "Great circle 1".
	// Each says only what differs from Reference.
	References map[string]Line `yaml:"references"`
}

// Units are the units numbers are shown in: a system, and the unit for each
// quantity, auto for the system's -- so a flight is imperial with its
// distance in nautical miles and its speed in knots.
type Units struct {
	System      string `yaml:"system"`
	Distance    string `yaml:"distance"`
	Elevation   string `yaml:"elevation"`
	Speed       string `yaml:"speed"`
	Pace        string `yaml:"pace"`
	Temperature string `yaml:"temperature"`
}

// Set is u as the units in use.
func (u Units) Set() (units.Set, error) {
	set, err := units.Of(units.System(u.System))
	if err != nil {
		return set, fmt.Errorf("units.system: %w", err)
	}
	for _, q := range []struct {
		q    units.Quantity
		name string
	}{{units.Distance, u.Distance}, {units.Elevation, u.Elevation}, {units.Speed, u.Speed}, {units.Pace, u.Pace}, {units.Temperature, u.Temperature}} {
		if q.name == "auto" {
			continue
		}
		if err := set.Use(q.q, q.name); err != nil {
			return set, fmt.Errorf("units.%s: %w", q.q, err)
		}
	}
	return set, nil
}

// Text is the text course writes over the map: start and finish, the
// distance markers' numbers, the legend, the map's credit.
type Text struct {
	// Size is in pixels on a map 1000 pixels across, scaled with the map.
	Size float64 `yaml:"size"`
}

// Map is the map under the course.
type Map struct {
	// LabelSize is the size of the map's own names -- streets, places --
	// in pixels on a map 1000 pixels across and scaled with it; or auto,
	// for the 13 pixels they have always been at any size of picture.
	LabelSize string `yaml:"label-size"`
	// Terrain shades the shape of the ground under the map, from elevation
	// kept beside the map's store; what the store lacks is offered before
	// it is fetched, as the map is. Off unless asked for, because fetching
	// it tells another host where the course is.
	Terrain bool `yaml:"terrain"`
	// Contours draws contour lines with the terrain, every fifth one
	// labelled with its height. Nothing without Terrain.
	Contours bool `yaml:"contours"`
}

// View is where the map is seen from: straight above, as a map is, or in
// perspective from a camera in the sky, the ground shaped by its heights.
type View struct {
	// Mode is one of ViewModes: flat, or 3d.
	Mode string `yaml:"mode"`
	// Heading is the compass bearing a 3d camera looks along, in degrees,
	// 0 looking north; or auto, for the bearing that shows the course
	// largest.
	Heading string `yaml:"heading"`
	// Pitch is how far below the horizontal a 3d camera looks, in degrees:
	// 90 is straight down.
	Pitch float64 `yaml:"pitch"`
	// FOV is a 3d camera's vertical field of view, in degrees.
	FOV float64 `yaml:"fov"`
	// Exaggeration is how many times their height hills are drawn in 3d.
	Exaggeration float64 `yaml:"exaggeration"`
}

// ViewModes are the values View.Mode takes.
var ViewModes = []string{"flat", "3d"}

// Legend is the legend saying what each line or colour is.
type Legend struct {
	// Position is one of LegendPositions: a corner; auto for whichever
	// corner covers least of the course; none for no legend.
	Position string `yaml:"position"`
}

// Markers are the course's distance markers, on a course whose file
// recorded distance.
type Markers struct {
	// Every is the distance between them in the distance unit; auto for a
	// distance that suits the course's length; none for no markers.
	Every string `yaml:"every"`
}

// LegendPositions are where a legend may go.
var LegendPositions = []string{"auto", "top-left", "top-right", "bottom-left", "bottom-right", "none"}

// Colouring is what a course's line is coloured by, along it, and how.
type Colouring struct {
	// By is one of Metrics, or none to draw the line in its own colour.
	By string `yaml:"by"`
	// GradeCap is, with By grade, the steepest grade the colours tell
	// apart, in per cent either way; steeper takes the end colour.
	GradeCap float64 `yaml:"grade-cap"`
	// PowerSource is, with By power, which of a recording's two power
	// readings: one of PowerSources.
	PowerSource string `yaml:"power-source"`
	// TemperatureSource is, with By temperature, which of a recording's
	// two temperatures: one of PowerSources, which it shares.
	TemperatureSource string `yaml:"temperature-source"`
	// Width is the coloured line's, in pixels on a map 1000 pixels across.
	Width float64 `yaml:"width"`
}

// Metrics are what a course can be coloured by, and PowerSources the power
// readings it can be coloured by -- the same three, meaning the same, as
// videofx's and fitdash's --power-source.
var (
	Metrics      = []string{"pace", "speed", "grade-adjusted-pace", "elevation", "grade", "heart-rate", "power", "air-power", "cadence", "temperature", "humidity"}
	PowerSources = []string{"auto", "stryd", "native"}
)

// Line is how one kind of line is drawn. In Course and Reference every
// setting has a value; in an entry of Activities or References, a setting
// left out is taken from Course or Reference.
type Line struct {
	// Colour is a hex colour, #rrggbb or #rrggbbaa, or auto for the
	// palette's own. For Reference, one colour for every reference, or
	// auto to take Colours in turn.
	Colour string `yaml:"colour,omitempty"`
	// Colours are hex colours taken in turn: the second, third, ...
	// activity's for Course, every reference's for Reference. Empty is the
	// palette's own. Only Course and Reference have them.
	Colours []string `yaml:"colours,omitempty"`
	// Width is in pixels on a map 1000 pixels across, and scaled with the
	// map.
	Width *float64 `yaml:"width,omitempty"`
	// Opacity is from 0 to 1, or auto.
	Opacity string `yaml:"opacity,omitempty"`
	// Style is solid, dashed or dotted.
	Style string `yaml:"style,omitempty"`
	// Halo is how far a band of the map's background reaches either side
	// of the line, in pixels on a map 1000 pixels across; or auto, for a
	// slim one on an opaque line and none on a translucent one.
	Halo string `yaml:"halo,omitempty"`
	// Beside is whether a reference drawn whole is drawn beside the course
	// where the two share a road, in a lane of its own, or under it, the
	// course narrowed so it shows along the edges: true or false. A
	// reference's setting only.
	Beside *bool `yaml:"beside,omitempty"`
}

func ptr(v float64) *float64 { return &v }

func yes(v bool) *bool { return &v }

// Default is the built-in style: the map course has always drawn.
func Default() Style {
	return Style{
		Palette:    "light",
		Width:      1600,
		Height:     1000,
		Units:      Units{System: "metric", Distance: "auto", Elevation: "auto", Speed: "auto", Pace: "auto", Temperature: "auto"},
		Text:       Text{Size: 13},
		Map:        Map{LabelSize: "auto", Contours: true},
		View:       View{Mode: "flat", Heading: "auto", Pitch: 35, FOV: 40, Exaggeration: 1},
		Legend:     Legend{Position: "auto"},
		Markers:    Markers{Every: "auto"},
		Colouring:  Colouring{By: "none", GradeCap: 15, PowerSource: "auto", TemperatureSource: "auto", Width: 3},
		Course:     Line{Colour: "auto", Colours: []string{}, Width: ptr(3), Opacity: "auto", Style: "solid", Halo: "auto"},
		Activities: []Line{},
		Reference:  Line{Colour: "auto", Colours: []string{}, Width: ptr(2.25), Opacity: "auto", Style: "dashed", Halo: "auto", Beside: yes(true)},
		References: map[string]Line{},
	}
}

// Palettes, LineStyles are the values Palette and a line's Style take.
var (
	Palettes   = []string{"light", "dark", "outdoors"}
	LineStyles = []string{"solid", "dashed", "dotted"}
)

// Load reads the style file at path over s: every setting the file has
// replaces s's, and every one it leaves out stays as it was.
func (s *Style) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := s.Read(f); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Read is Load from r.
func (s *Style) Read(r io.Reader) error {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	err := dec.Decode(s)
	if errors.Is(err, io.EOF) {
		return nil // an empty file changes nothing
	}
	return plain(err)
}

// plain is err with the decoder's Go type names put as what they are in a
// style file.
func plain(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.NewReplacer(
		" in type mapstyle.Line", "; a line has "+strings.Join(lineKeys, ", "),
		" in type mapstyle.Style", "; a style has "+strings.Join(topKeys, ", "),
		" in type mapstyle.Colouring", "; colouring has "+strings.Join(groupKeys["colouring"], ", "),
		" in type mapstyle.Text", "; text has "+strings.Join(groupKeys["text"], ", "),
		" in type mapstyle.Units", "; units has "+strings.Join(groupKeys["units"], ", "),
		" in type mapstyle.Map", "; map has "+strings.Join(groupKeys["map"], ", "),
		" in type mapstyle.Legend", "; legend has "+strings.Join(groupKeys["legend"], ", "),
		" in type mapstyle.Markers", "; markers has "+strings.Join(groupKeys["markers"], ", "),
	).Replace(err.Error())
	return errors.New(msg)
}

// Set makes one setting, path=value, over s: path is the setting's keys
// joined by dots -- course.colour, reference.style, references.Rhodes
// parkrun.colour -- with an activity numbered from 1, as on the map:
// activities.2.width. value is read as YAML, so a list is [a, b]; a value
// YAML reads as nothing, such as an unquoted #d32f2f, which it takes for a
// comment, is the text as given.
func (s *Style) Set(setting string) error {
	path, value, ok := strings.Cut(setting, "=")
	if !ok || path == "" {
		return fmt.Errorf("%q: give a setting as path=value, such as course.colour=#d32f2f", setting)
	}
	// YAML reads # as the start of a comment, so a colour typed bare --
	// #d32f2f, or [#112233, #445566] -- would be read as nothing. A colour
	// not already in quotes is quoted first.
	var v yaml.Node
	if err := yaml.Unmarshal([]byte(bareColour.ReplaceAllString(value, "$1'$2'")), &v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if v.Kind == yaml.DocumentNode && len(v.Content) == 1 {
		val = v.Content[0]
	}

	var root yaml.Node
	if err := root.Encode(s); err != nil {
		return err
	}
	keys := strings.Split(path, ".")
	// A reference's name may have dots in it; after "references" every key
	// up to the last is the name.
	if keys[0] == "references" && len(keys) > 3 {
		keys = []string{keys[0], strings.Join(keys[1:len(keys)-1], "."), keys[len(keys)-1]}
	}
	if err := checkPath(keys); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	at := &root
	for i, k := range keys {
		last := i == len(keys)-1
		switch at.Kind {
		case yaml.MappingNode:
			child := mappingValue(at, k)
			if child == nil {
				child = &yaml.Node{Kind: yaml.MappingNode}
				at.Content = append(at.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, child)
			}
			if last {
				*child = *val
			}
			at = child
		case yaml.SequenceNode:
			n, err := strconv.Atoi(k)
			if err != nil || n < 1 {
				return fmt.Errorf("%s: %q is not a position; activities are numbered from 1", path, k)
			}
			for len(at.Content) < n {
				at.Content = append(at.Content, &yaml.Node{Kind: yaml.MappingNode})
			}
			if last {
				*at.Content[n-1] = *val
			}
			at = at.Content[n-1]
		default:
			return fmt.Errorf("%s: %s has no setting %q", path, strings.Join(keys[:i], "."), k)
		}
	}
	var out Style
	if err := decodeKnown(&root, &out); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	*s = out
	return nil
}

// bareColour is a hex colour not in quotes: at the start, or after a
// bracket, a comma or a space.
var bareColour = regexp.MustCompile(`(^|[\[,\s])(#[0-9a-fA-F]{6}(?:[0-9a-fA-F]{2})?)\b`)

var (
	topKeys  = []string{"palette", "width", "height", "units", "text", "map", "view", "legend", "markers", "colouring", "course", "activities", "reference", "references"}
	lineKeys = []string{"colour", "colours", "width", "opacity", "style", "halo", "beside"}
	// groupKeys are the settings of each group that is not a line.
	groupKeys = map[string][]string{
		"units":     {"system", "distance", "elevation", "speed", "pace", "temperature"},
		"text":      {"size"},
		"map":       {"label-size", "terrain", "contours"},
		"view":      {"mode", "heading", "pitch", "fov", "exaggeration"},
		"legend":    {"position"},
		"markers":   {"every"},
		"colouring": {"by", "grade-cap", "power-source", "temperature-source", "width"},
	}
)

// checkPath says what is wrong with keys as the path of a setting, in the
// style's terms: which keys there are where one is not.
func checkPath(keys []string) error {
	settings := func(of, bad string, ks []string) error {
		return fmt.Errorf("no such setting %q; %s has %s", bad, of, strings.Join(ks, ", "))
	}
	if !slices.Contains(topKeys, keys[0]) {
		return fmt.Errorf("no such setting %q; a style has %s", keys[0], strings.Join(topKeys, ", "))
	}
	var line []string // the keys that name a setting of a line
	switch keys[0] {
	case "palette", "width", "height":
		if len(keys) > 1 {
			return fmt.Errorf("%s is one setting, not a group of them", keys[0])
		}
		return nil
	case "units", "text", "map", "view", "legend", "markers", "colouring":
		group := groupKeys[keys[0]]
		if len(keys) > 2 || len(keys) == 2 && !slices.Contains(group, keys[1]) {
			return fmt.Errorf("no such setting %q; %s has %s", strings.Join(keys[1:], "."), keys[0], strings.Join(group, ", "))
		}
		return nil
	case "course", "reference":
		line = keys[1:]
	case "activities":
		if len(keys) < 2 {
			return errors.New("give an activity's number and its setting, such as activities.2.colour")
		}
		if n, err := strconv.Atoi(keys[1]); err != nil || n < 1 {
			return fmt.Errorf("%q is not a position; activities are numbered from 1", keys[1])
		}
		line = keys[2:]
	case "references":
		if len(keys) < 2 {
			return errors.New("give a reference's name and its setting, such as references.Loop.colour")
		}
		line = keys[2:]
	}
	switch {
	case len(line) == 0:
		return nil // the whole of a line, given as a mapping
	case len(line) > 1 || !slices.Contains(lineKeys, line[0]):
		return settings(strings.Join(keys[:len(keys)-len(line)], "."), strings.Join(line, "."), lineKeys)
	}
	return nil
}

// mappingValue is the value of key k in mapping m, or nil.
func mappingValue(m *yaml.Node, k string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == k {
			return m.Content[i+1]
		}
	}
	return nil
}

// decodeKnown decodes n into v, refusing a key v does not have. Decoding a
// node makes no such check, so it goes through YAML text and a decoder that
// does.
func decodeKnown(n *yaml.Node, v any) error {
	b, err := yaml.Marshal(n)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	return dec.Decode(v)
}

var hexColour = regexp.MustCompile(`^#([0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// Validate reports the first setting in s that is not one, by its path.
func (s Style) Validate() error {
	if !slices.Contains(Palettes, s.Palette) {
		return fmt.Errorf("palette: %q is not one; use %s", s.Palette, strings.Join(Palettes, " or "))
	}
	// A picture smaller than this has no room for a course and its legend,
	// let alone a map under them.
	const smallest = 64
	if s.Width < smallest {
		return fmt.Errorf("width: %d pixels is too small to draw a course on; give at least %d", s.Width, smallest)
	}
	if s.Height < smallest {
		return fmt.Errorf("height: %d pixels is too small to draw a course on; give at least %d", s.Height, smallest)
	}
	if _, err := s.Units.Set(); err != nil {
		return err
	}
	if !(s.Text.Size > 0) {
		return fmt.Errorf("text.size: %v is not a size; give pixels on a map 1000 across, more than 0", s.Text.Size)
	}
	if err := autoOrPositive("map.label-size", s.Map.LabelSize, "pixels on a map 1000 across"); err != nil {
		return err
	}
	if !slices.Contains(ViewModes, s.View.Mode) {
		return fmt.Errorf("view.mode: %q is not one; use %s", s.View.Mode, strings.Join(ViewModes, " or "))
	}
	if s.View.Heading != "auto" {
		if h, err := strconv.ParseFloat(s.View.Heading, 64); err != nil || math.IsInf(h, 0) || math.IsNaN(h) {
			return fmt.Errorf("view.heading: %q is not one; give a compass bearing in degrees, or auto", s.View.Heading)
		}
	}
	// Level with the ground, the camera sees the course edge on; past
	// straight down, it is looking back over its own shoulder.
	if !(s.View.Pitch >= 10 && s.View.Pitch <= 90) {
		return fmt.Errorf("view.pitch: %v is not one; give degrees below the horizontal, from 10 to 90", s.View.Pitch)
	}
	if !(s.View.FOV >= 10 && s.View.FOV <= 100) {
		return fmt.Errorf("view.fov: %v is not one; give a field of view in degrees, from 10 to 100", s.View.FOV)
	}
	if !(s.View.Exaggeration > 0 && s.View.Exaggeration <= 10) {
		return fmt.Errorf("view.exaggeration: %v is not one; give how many times their height hills are drawn, more than 0 and at most 10", s.View.Exaggeration)
	}
	if !slices.Contains(LegendPositions, s.Legend.Position) {
		return fmt.Errorf("legend.position: %q is not one; use %s", s.Legend.Position, strings.Join(LegendPositions, ", "))
	}
	if s.Markers.Every != "none" {
		if err := autoOrPositive("markers.every", s.Markers.Every, "a distance in the distance unit, or none"); err != nil {
			return err
		}
	}
	if !(s.Colouring.Width > 0) {
		return fmt.Errorf("colouring.width: %v is not a width; give pixels on a map 1000 across, more than 0", s.Colouring.Width)
	}
	if s.Colouring.By != "none" && !slices.Contains(Metrics, s.Colouring.By) {
		return fmt.Errorf("colouring.by: %q is not one; colour by %s, or none", s.Colouring.By, strings.Join(Metrics, ", "))
	}
	if !(s.Colouring.GradeCap > 0) {
		return fmt.Errorf("colouring.grade-cap: %v is not one; the cap is a grade in per cent, more than 0", s.Colouring.GradeCap)
	}
	if !slices.Contains(PowerSources, s.Colouring.PowerSource) {
		// The words videofx and fitdash refuse it in.
		return fmt.Errorf("colouring.power-source: %q is invalid; use auto, stryd, or native", s.Colouring.PowerSource)
	}
	if !slices.Contains(PowerSources, s.Colouring.TemperatureSource) {
		return fmt.Errorf("colouring.temperature-source: %q is invalid; use auto, stryd, or native", s.Colouring.TemperatureSource)
	}
	if err := s.Course.check("course", true, true); err != nil {
		return err
	}
	if s.Course.Beside != nil {
		return errors.New("course.beside: only a reference is drawn beside the course; set reference.beside")
	}
	if err := s.Reference.check("reference", true, true); err != nil {
		return err
	}
	for i, a := range s.Activities {
		if err := a.check(fmt.Sprintf("activities.%d", i+1), false, false); err != nil {
			return err
		}
		if a.Beside != nil {
			return fmt.Errorf("activities.%d.beside: only a reference is drawn beside the course; set reference.beside", i+1)
		}
	}
	names := make([]string, 0, len(s.References))
	for n := range s.References {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		if err := s.References[n].check("references."+n, false, false); err != nil {
			return err
		}
	}
	return nil
}

// check reports a setting of l, at path, that is not one: whole is whether
// l is a default, which must have every setting; colours whether it may
// have a Colours list.
func (l Line) check(path string, whole, colours bool) error {
	if whole && (l.Colour == "" || l.Width == nil || l.Opacity == "" || l.Style == "" || l.Halo == "") {
		return fmt.Errorf("%s: colour, width, opacity, style and halo must all be set", path)
	}
	if l.Colour != "" && l.Colour != "auto" && !hexColour.MatchString(l.Colour) {
		return fmt.Errorf("%s.colour: %q is not a colour; use #rrggbb, #rrggbbaa or auto", path, l.Colour)
	}
	if len(l.Colours) > 0 && !colours {
		return fmt.Errorf("%s.colours: only course and reference take a list of colours; give one colour", path)
	}
	for i, c := range l.Colours {
		if !hexColour.MatchString(c) {
			return fmt.Errorf("%s.colours: %q, the %s, is not a colour; use #rrggbb or #rrggbbaa", path, c, ordinal(i+1))
		}
	}
	if l.Width != nil && !(*l.Width > 0) {
		return fmt.Errorf("%s.width: %v is not a width; give pixels on a map 1000 across, more than 0", path, *l.Width)
	}
	if l.Opacity != "" && l.Opacity != "auto" {
		o, err := strconv.ParseFloat(l.Opacity, 64)
		if err != nil || o < 0 || o > 1 {
			return fmt.Errorf("%s.opacity: %q is not one; use a number from 0 to 1, or auto", path, l.Opacity)
		}
	}
	if l.Halo != "" && l.Halo != "auto" {
		h, err := strconv.ParseFloat(l.Halo, 64)
		if err != nil || h < 0 {
			return fmt.Errorf("%s.halo: %q is not one; give pixels on a map 1000 across, 0 or more, or auto", path, l.Halo)
		}
	}
	if l.Style != "" && !slices.Contains(LineStyles, l.Style) {
		return fmt.Errorf("%s.style: %q is not one; use %s", path, l.Style, strings.Join(LineStyles, ", "))
	}
	return nil
}

// autoOrPositive checks that the setting at path, v, is auto or a number
// above 0, in the unit given.
func autoOrPositive(path, v, unit string) error {
	if v == "auto" {
		return nil
	}
	if n, err := strconv.ParseFloat(v, 64); err != nil || !(n > 0) {
		return fmt.Errorf("%s: %q is not one; give %s, more than 0, or auto", path, v, unit)
	}
	return nil
}

func ordinal(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 13:
		return fmt.Sprintf("%dth", n)
	case n%10 == 1:
		return fmt.Sprintf("%dst", n)
	case n%10 == 2:
		return fmt.Sprintf("%dnd", n)
	case n%10 == 3:
		return fmt.Sprintf("%drd", n)
	}
	return fmt.Sprintf("%dth", n)
}

// Write writes s as YAML to w, every setting with a comment saying what it
// is and what it takes -- a style to start a theme from, or to bring an old
// one up to the settings there are now. autos says, for a setting whose
// value is auto or empty, what that comes to, keyed by its path.
func (s Style) Write(w io.Writer, autos map[string]string) error {
	var doc yaml.Node
	if err := doc.Encode(s); err != nil {
		return err
	}
	// Every setting is shown, an empty list of colours included: a
	// template that leaves one out does not say it is there to set.
	top := &doc
	if top.Kind == yaml.DocumentNode {
		top = top.Content[0]
	}
	for _, k := range []string{"course", "reference"} {
		if m := mappingValue(top, k); m != nil && mappingValue(m, "colours") == nil {
			at := 2 // after colour
			list := []*yaml.Node{{Kind: yaml.ScalarNode, Value: "colours"}, {Kind: yaml.SequenceNode, Style: yaml.FlowStyle}}
			m.Content = append(m.Content[:at], append(list, m.Content[at:]...)...)
		}
	}
	comment(&doc, "", autos)
	doc.HeadComment = "A course map style. Give it to course map with --style FILE; settings\n" +
		"left out keep their defaults, and --set path=value changes one more."
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	return enc.Close()
}

// comment attaches each key's description to it, walking the mapping n at
// path.
func comment(n *yaml.Node, path string, autos map[string]string) {
	if n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		p := k.Value
		if path != "" {
			p = path + "." + k.Value
		}
		general := p
		if strings.HasPrefix(path, "references.") || strings.HasPrefix(path, "activities.") {
			general = "override." + k.Value
		}
		if d, ok := descriptions[general]; ok {
			k.HeadComment = d
		}
		if a, ok := autos[p]; ok {
			// On the value: on an empty list's key, the encoder puts it at
			// the end of the next line.
			v.LineComment = a
		}
		if v.Kind == yaml.MappingNode {
			comment(v, p, autos)
		}
		if v.Kind == yaml.SequenceNode && k.Value == "activities" {
			for j, e := range v.Content {
				comment(e, fmt.Sprintf("activities.%d", j+1), autos)
			}
		}
	}
}

var descriptions = map[string]string{
	"palette":                      "The map's colours: light, dark, or outdoors -- greener woods and parks,\nfor terrain and 3d.",
	"width":                        "The picture's width in pixels. Everything on it is scaled with it, so\nthe line widths below look the same at any size.",
	"height":                       "The picture's height in pixels.",
	"units":                        "The units numbers are shown in. --units and --unit are the same as\nsetting these, for this and every other command.",
	"units.system":                 "metric or imperial.",
	"units.distance":               "auto for the system's, or km, mi, nmi.",
	"units.elevation":              "auto for the system's, or m, ft. Also short distances, such as how far\na run strayed from a course.",
	"units.speed":                  "auto for the system's, or km/h, mph, kn, m/s.",
	"units.pace":                   "auto for the system's, or min/km, min/mi.",
	"units.temperature":            "auto for the system's, or C, F.",
	"text":                         "The text course writes over the map: start and finish, the distance\nmarkers' numbers, the legend, the map's credit.",
	"text.size":                    "In pixels on a map 1000 pixels across; scaled with the map.",
	"map":                          "The map under the course.",
	"map.label-size":               "The size of the map's own names, streets and places, in pixels on a map\n1000 pixels across and scaled with it; or auto, for 13 pixels at any size.",
	"map.terrain":                  "true: shade the shape of the ground under the map. The elevation is kept\nbeside osmbase's store, and what it lacks is offered before it is fetched,\nwhich tells Mapterhorn's host the area. --terrain is the same as setting it.",
	"map.contours":                 "With terrain, draw contour lines, every fifth one labelled with its height;\nfalse for the shading alone. --contours is the same as setting it.",
	"view":                         "Where the map is seen from.",
	"view.mode":                    "flat, seen from straight above; or 3d, in perspective from a camera in the\nsky, the ground shaped by its heights and the course lying over it. 3d\nneeds the terrain's heights, and offers them as --terrain does, shading\nthe map too. --3d is the same as setting it.",
	"view.heading":                 "With 3d, the compass bearing the camera looks along, in degrees, 0 looking\nnorth; auto for the one that shows the course largest. --heading is the\nsame as setting it.",
	"view.pitch":                   "With 3d, how far below the horizontal the camera looks, in degrees, from\n10 to 90 (straight down).",
	"view.fov":                     "With 3d, the camera's vertical field of view, in degrees; wider takes in\nmore and draws the ground near the camera larger than the distance.",
	"view.exaggeration":            "With 3d, how many times their height hills are drawn; 1 is true to life.",
	"legend":                       "The legend saying what each line or colour is.",
	"legend.position":              "top-left, top-right, bottom-left or bottom-right; auto for whichever covers\nleast of the course; none for no legend. --legend is the same as setting it.",
	"markers":                      "The distance markers, on a course whose file recorded distance.",
	"markers.every":                "The distance between them, in the distance unit; auto for one that suits\nthe course's length; none for no markers.",
	"colouring":                    "What the course's line is coloured by along it; --colour, --grade-cap,\n--power-source and --temperature-source are the same as setting these.",
	"colouring.by":                 "none, or one of pace, speed, grade-adjusted-pace, elevation, grade,\nheart-rate, power, air-power, cadence, temperature, humidity.",
	"colouring.grade-cap":          "With by: grade, the steepest grade the colours tell apart, in per cent\neither way; steeper takes the end colour.",
	"colouring.power-source":       "With by: power, which reading when a file has both: auto (a footpod's,\nsuch as Stryd's, if there is one), stryd, or native (the watch's).",
	"colouring.temperature-source": "With by: temperature, which reading when a file has both: auto (a\nfootpod's of the air, such as Stryd's, if there is one), stryd, or native\n(the watch's, which reads warm on a wrist).",
	"colouring.width":              "The coloured line's width, in pixels on a map 1000 pixels across.",
	"course.halo":                  "A band of the map's background either side of the line, in pixels on a\nmap 1000 across; auto: a slim one on an opaque line, none on a translucent one.",
	"reference.halo":               "As for course.",
	"reference.beside":             "true: a reference drawn whole is drawn beside the course where the two\nshare a road, in a lane of its own, as a transit map draws lines; false:\nunder it, the course narrowed so it shows along the edges.",
	"override.beside":              "true or false; references only.",
	"override.halo":                "In pixels on a map 1000 pixels across, or auto.",
	"course":                       "The course, and with --separate every activity unless activities says otherwise.",
	"course.colour":                "A hex colour, #rrggbb or #rrggbbaa, or auto for the palette's own.\nWith --separate, activity 1's.",
	"course.colours":               "With --separate, the 2nd, 3rd, ... activities' colours, taken in turn.\nEmpty is the palette's own.",
	"course.width":                 "In pixels on a map 1000 pixels across; scaled with the map.",
	"course.opacity":               "From 0 to 1, or auto: 0.7, so the map shows through, but 1 -- with a\nslim halo -- when a reference is drawn whole over the course.",
	"course.style":                 "solid, dashed or dotted.",
	"activities":                   "With --separate, settings for single activities, by position: the first\nentry is activity 1, as numbered on the map. Each needs only what differs\nfrom course: colour, width, opacity, style. On the command line:\n--set activities.2.colour=#1565c0",
	"reference":                    "Every reference, unless references says otherwise.",
	"reference.colour":             "One hex colour for every reference, or auto to take colours in turn.",
	"reference.colours":            "Hex colours taken in turn, one for each reference. Empty is the\npalette's own.",
	"reference.width":              "In pixels on a map 1000 pixels across; scaled with the map.",
	"reference.opacity":            "From 0 to 1, or auto, as for course.",
	"reference.style":              "solid, dashed or dotted.",
	"references":                   "Settings for single references, by name: a stored reference's name, a\nfile's without its extension, or Great circle, Great circle 1, ...\nEach needs only what differs from reference: colour, width, opacity,\nstyle, halo, beside. On the command line: --set 'references.Rhodes parkrun.colour=#0077aa'",
	"override.colour":              "A hex colour, #rrggbb or #rrggbbaa.",
	"override.width":               "In pixels on a map 1000 pixels across.",
	"override.opacity":             "From 0 to 1, or auto.",
	"override.style":               "solid, dashed or dotted.",
}
