package course

import (
	"fmt"
	"time"
)

// Along is each point's distance from the course's start, in metres: the
// recorded distance where every point has one, measured along the line
// otherwise. A crop, a length or a comparison by distance needs a distance
// for every point, and a GPX has none recorded.
func (c *Course) Along() []float64 {
	out := make([]float64, len(c.Points))
	if recordedThroughout(c) {
		for i, p := range c.Points {
			out[i] = p.Distance - c.Points[0].Distance
		}
		return out
	}
	for i := 1; i < len(c.Points); i++ {
		a, b := c.Points[i-1], c.Points[i]
		out[i] = out[i-1] + Metres(a.Lat, a.Lon, b.Lat, b.Lon)
	}
	return out
}

func recordedThroughout(c *Course) bool {
	for _, p := range c.Points {
		if !p.HasDistance {
			return false
		}
	}
	return len(c.Points) > 0
}

// Length is how far the course goes, by Along.
func (c *Course) Length() float64 {
	a := c.Along()
	if len(a) == 0 {
		return 0
	}
	return a[len(a)-1]
}

// CropDistance is the part of the course from from to to metres along it,
// by Along -- the parkrun out of a run that also had its warm-up and
// cool-down. to of 0 is the end. The points are the course's own, not
// interpolated: a crop starts at the first point at or past from and ends at
// the last at or before to.
func (c *Course) CropDistance(from, to float64) (*Course, error) {
	along := c.Along()
	if to == 0 && len(along) > 0 {
		to = along[len(along)-1]
	}
	if from < 0 || to <= from {
		return nil, fmt.Errorf("a crop from %.0f m to %.0f m is empty", from, to)
	}
	return c.keep(func(i int) bool { return along[i] >= from && along[i] <= to })
}

// CropTime is the part of a timed course from from to to after its start.
// to of 0 is the end. A plan has no clock to crop by.
func (c *Course) CropTime(from, to time.Duration) (*Course, error) {
	if !c.Timed {
		return nil, fmt.Errorf("the course has no times to crop by; crop it by distance")
	}
	if to == 0 && len(c.Points) > 0 {
		to = c.Points[len(c.Points)-1].Elapsed
	}
	if from < 0 || to <= from {
		return nil, fmt.Errorf("a crop from %v to %v is empty", from, to)
	}
	return c.keep(func(i int) bool { return c.Points[i].Elapsed >= from && c.Points[i].Elapsed <= to })
}

// keep is the course with only the points in, and at least two of them.
func (c *Course) keep(in func(int) bool) (*Course, error) {
	out := *c
	out.Points = nil
	for i, p := range c.Points {
		if in(i) {
			out.Points = append(out.Points, p)
		}
	}
	if len(out.Points) < 2 {
		return nil, fmt.Errorf("the crop leaves %d points of the course", len(out.Points))
	}
	return &out, nil
}
