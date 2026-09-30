package routemap

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
)

// ActivityInks are the inks several activities drawn on one map are told
// apart by, in turn: the course's own route ink first, so one activity looks
// as a course always has, then a blue, a purple, a green, a petrol and a
// plum; a seventh activity takes the first ink again, told apart by its
// number. None is any of
// the references' inks -- pink, orange, teal, brown -- or a course's start,
// finish or gap ink, so an activity is never taken for a reference or for a
// gap in another; each is checked against the palette's land, water and
// background.
func ActivityInks(p render.Palette, o render.Overlay) []color.RGBA {
	route := InksFor(p, o).Route
	if luminance(p.Land) < 0.5 {
		return []color.RGBA{
			route,
			{R: 0x90, G: 0xCA, B: 0xF9, A: 0xff}, // light blue
			{R: 0xCE, G: 0x93, B: 0xD8, A: 0xff}, // light purple
			{R: 0xA5, G: 0xD6, B: 0xA7, A: 0xff}, // light green
			{R: 0x80, G: 0xDE, B: 0xEA, A: 0xff}, // light cyan
			{R: 0xFF, G: 0xE0, B: 0x82, A: 0xff}, // light amber
		}
	}
	return []color.RGBA{
		route,
		{R: 0x15, G: 0x65, B: 0xC0, A: 0xff}, // blue
		{R: 0x6A, G: 0x1B, B: 0x9A, A: 0xff}, // purple
		{R: 0x2E, G: 0x6B, B: 0x1E, A: 0xff}, // green
		{R: 0x00, G: 0x60, B: 0x80, A: 0xff}, // petrol
		{R: 0x88, G: 0x0E, B: 0x4F, A: 0xff}, // plum
	}
}

// ActivitiesDrawing is several activities on one map: each its own line in
// its own ink from actInks, its own distance markers, and its start and
// finish, numbered from 1 in the order given.
//
// Where one activity ends and the next begins -- the run to a parkrun, the
// parkrun, the run home -- their ends are one place, and a dot and a label
// for each would be drawn over one another into something nobody can read.
// So ends within a few pixels of each other are one dot, labelled with all
// of them in order: "Finish 1 · Start and finish 2 · Start 3". It is the
// start's ink if anything starts there, and the finish's otherwise.
func ActivitiesDrawing(acts []*course.Course, v render.View, inks Inks, actInks []color.RGBA, scale float64) render.Drawing {
	if scale <= 0 {
		scale = 1
	}
	_, halo, dot, _ := sizes(scale)
	var d render.Drawing
	type end struct {
		at            render.Coord
		n             int
		start, finish bool
	}
	var ends []end
	for i, a := range acts {
		if len(a.Points) == 0 {
			continue
		}
		own := inks
		own.Route = actInks[i%len(actInks)]
		one := Drawing(a, v, own, scale)
		d.Lines = append(d.Lines, one.Lines...)
		// Drawing's last two markers are the finish and the start; the
		// ends are drawn here instead, together.
		d.Markers = append(d.Markers, one.Markers[:len(one.Markers)-2]...)
		first, last := a.Points[0], a.Points[len(a.Points)-1]
		ends = append(ends,
			end{at: render.Coord{Lat: first.Lat, Lon: first.Lon}, n: i + 1, start: true},
			end{at: render.Coord{Lat: last.Lat, Lon: last.Lon}, n: i + 1, finish: true})
	}
	used := make([]bool, len(ends))
	var finishes, starts []render.Marker
	for i := range ends {
		if used[i] {
			continue
		}
		group := []end{ends[i]}
		used[i] = true
		for j := i + 1; j < len(ends); j++ {
			if !used[j] && pixelDistance(v, ends[i].at, ends[j].at) < 3*dot {
				group = append(group, ends[j])
				used[j] = true
			}
		}
		var parts []string
		anyStart := false
		for k := 0; k < len(group); k++ {
			e := group[k]
			anyStart = anyStart || e.start
			// An activity's own start and finish together are one part.
			if k+1 < len(group) && group[k+1].n == e.n {
				parts = append(parts, fmt.Sprintf("Start and finish %d", e.n))
				k++
				continue
			}
			word := "Finish"
			if e.start {
				word = "Start"
			}
			parts = append(parts, fmt.Sprintf("%s %d", word, e.n))
		}
		m := render.Marker{At: ends[i].at, Ink: inks.Finish, Radius: dot, Halo: halo, HaloInk: inks.Halo, Label: strings.Join(parts, " · ")}
		if anyStart {
			m.Ink = inks.Start
			starts = append(starts, m)
		} else {
			finishes = append(finishes, m)
		}
	}
	// Starts on top, as a single course's is.
	d.Markers = append(append(d.Markers, finishes...), starts...)
	return d
}
