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

// headingStep is how finely an auto heading is chosen, in degrees: finer
// shows no difference anybody would see, and each costs a framing.
const headingStep = 15

// cameraFor is the 3d camera that shows pts whole, in a picture of the given
// aspect, at the style's pitch and lens: looking along the style's heading,
// or with heading auto, along whichever shows them largest -- the camera
// that can stand nearest -- which lays a long course across the picture
// rather than away from the camera. Of two bearings that tie, the first
// from north is taken, so the same course is always seen the same way.
func cameraFor(v mapstyle.View, aspect float64, pts []render.Coord) (perspective.Camera, error) {
	base := perspective.Camera{Pitch: v.Pitch, FOV: v.FOV}
	if v.Heading != "auto" {
		h, err := strconv.ParseFloat(v.Heading, 64)
		if err != nil {
			return base, fmt.Errorf("view.heading: %w", err) // checked by the style
		}
		base.Heading = math.Mod(math.Mod(h, 360)+360, 360)
		return base.Frame(pts, aspect)
	}
	// Chosen on a few hundred of the points, which frame as the whole
	// does to well within the margin, and quickly.
	sample := pts
	if n := len(pts); n > 400 {
		sample = make([]render.Coord, 0, 401)
		for i := 0; i < n; i += n / 400 {
			sample = append(sample, pts[i])
		}
		sample = append(sample, pts[n-1])
	}
	best, bestHeading := math.Inf(1), 0.0
	for h := 0.0; h < 360; h += headingStep {
		base.Heading = h
		cam, err := base.Frame(sample, aspect)
		if err != nil {
			return cam, err
		}
		// A thousandth nearer to count: the bearings opposite each other
		// frame a course alike, and rounding should not decide between them.
		if cam.Distance < best*0.999 {
			best, bestHeading = cam.Distance, h
		}
	}
	base.Heading = bestHeading
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
