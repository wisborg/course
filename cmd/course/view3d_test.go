package main

import (
	"math"
	"strconv"
	"testing"

	"github.com/wisborg/osmbase/render"

	"github.com/wisborg/course/mapstyle"
)

// An auto heading is the bearing, of every headingStep, from which the camera
// stands nearest to take the course in -- for a long straight course in a
// wide picture that lays it corner to corner, the longest way across, and
// never looks along it. A heading given is kept, turned into 0 to 360.
func TestCameraForChoosesTheHeadingThatShowsTheCourseLargest(t *testing.T) {
	line := func(dLat, dLon float64) []render.Coord {
		var pts []render.Coord
		for i := 0; i <= 50; i++ {
			pts = append(pts, render.Coord{Lat: -33.7 + dLat*float64(i), Lon: 151.1 + dLon*float64(i)})
		}
		return pts
	}
	const aspect = 16.0 / 10
	v := mapstyle.Default().View
	for _, c := range []struct {
		name  string
		pts   []render.Coord
		along []float64
	}{
		{"east to west", line(0, 0.001), []float64{90, 270}},
		{"north to south", line(0.001, 0), []float64{0, 180}},
	} {
		cam, err := cameraFor(v, aspect, c.pts)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range c.along {
			if cam.Heading == h {
				t.Errorf("%s: heading %g looks along the course", c.name, h)
			}
		}
		for h := 0.0; h < 360; h += headingStep {
			v.Heading = strconv.FormatFloat(h, 'g', -1, 64)
			other, err := cameraFor(v, aspect, c.pts)
			if err != nil {
				t.Fatal(err)
			}
			if other.Distance < cam.Distance*0.99 {
				t.Errorf("%s: heading %g stands %.0f m off, nearer than the %.0f m of the heading chosen, %g",
					c.name, h, other.Distance, cam.Distance, cam.Heading)
			}
		}
		v.Heading = "auto"
	}
	v.Heading = "-90"
	if cam, err := cameraFor(v, aspect, line(0, 0.001)); err != nil || math.Abs(cam.Heading-270) > 1e-9 {
		t.Errorf("heading -90 given: %g, %v", cam.Heading, err)
	}
}
