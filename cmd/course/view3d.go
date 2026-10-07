package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/wisborg/fitactivity/units"
	"github.com/wisborg/osmbase/perspective"
	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course"
	"github.com/wisborg/course/mapstyle"
	"github.com/wisborg/course/routemap"
)

// framed is every point a 3d camera has to take in: the course's, and those
// of the references drawn beside it -- a reference that goes where the
// course did not is on the picture too, as it is on a flat map.
func framed(c *course.Course, refs []routemap.Reference) []render.Coord {
	pts := make([]render.Coord, 0, len(c.Points))
	for _, p := range c.Points {
		pts = append(pts, render.Coord{Lat: p.Lat, Lon: p.Lon})
	}
	for _, r := range refs {
		for _, piece := range r.Drawn() {
			for _, p := range piece {
				pts = append(pts, render.Coord{Lat: p.Lat, Lon: p.Lon})
			}
		}
	}
	return pts
}

// cameraFor is the 3d camera that shows pts whole, in a picture of the given
// aspect, at the style's pitch and lens: looking along the style's heading,
// or with heading auto, along whichever shows them largest -- osmbase's
// perspective.Camera.BestHeading, the rule fitdash's 3d route uses too, so
// the same course is seen the same way by both.
func cameraFor(v mapstyle.View, aspect float64, pts []render.Coord) (perspective.Camera, error) {
	base := perspective.Camera{Pitch: v.Pitch, FOV: v.FOV}
	if v.Heading == "auto" {
		return base.BestHeading(pts, aspect)
	}
	h, err := strconv.ParseFloat(v.Heading, 64)
	if err != nil {
		return base, fmt.Errorf("view.heading: %w", err) // checked by the style
	}
	base.Heading = math.Mod(math.Mod(h, 360)+360, 360)
	return base.Frame(pts, aspect)
}

// seenOn marks on overlay where pts are seen in the picture, a square r
// pixels either side of each: what a legend placed over them would hide.
func seenOn(overlay *image.RGBA, pic *perspective.Picture, pts []render.Coord, r int) {
	ink := color.RGBA{A: 0xff}
	for _, p := range pts {
		x, y, ok := pic.Locate(p)
		if !ok {
			continue
		}
		for dy := -r; dy <= r; dy++ {
			for dx := -r; dx <= r; dx++ {
				overlay.SetRGBA(int(x)+dx, int(y)+dy, ink)
			}
		}
	}
}

// compass is a bearing as the nearest of eight compass points.
func compass(deg float64) string {
	points := []string{"north", "north-east", "east", "south-east", "south", "south-west", "west", "north-west"}
	return points[int(math.Mod(math.Round(deg/45), 8)+8)%8]
}

// cameraLine says where a 3d map was seen from, the camera's distance in
// the style's distance unit.
func cameraLine(cam perspective.Camera, v mapstyle.View, set units.Set) string {
	u := set.Unit(units.Distance)
	line := fmt.Sprintf("3d, looking %s (%.0f°) from %.1f %s away, %g° down",
		compass(cam.Heading), cam.Heading, u.FromSI(cam.Distance), u.Name, cam.Pitch)
	if v.Exaggeration != 1 {
		line += fmt.Sprintf(", hills %g times their height", v.Exaggeration)
	}
	return line
}
