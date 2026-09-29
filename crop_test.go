package course

import (
	"testing"
	"time"
)

// A straight course east along 10°N, a point every ~11 m and every second.
func straight(n int, recorded bool) *Course {
	c := &Course{Timed: true}
	for i := 0; i < n; i++ {
		p := Point{Lat: 10, Lon: 20 + float64(i)*0.0001, Elapsed: time.Duration(i) * time.Second}
		if recorded {
			p.HasDistance, p.Distance = true, 1000+float64(i)*10 // a recording that began earlier
		}
		c.Points = append(c.Points, p)
	}
	return c
}

// Along is the recorded distance from the course's own start where there is
// one, and measured along the line where there is not.
func TestAlong(t *testing.T) {
	if a := straight(11, true).Along(); a[0] != 0 || a[10] != 100 {
		t.Errorf("recorded: %v; want from 0, 10 m a point", a)
	}
	a := straight(11, false).Along()
	if a[0] != 0 || a[10] < 108 || a[10] > 111 {
		t.Errorf("measured: %.1f m for ten steps of 0.0001° at 10°N; want about 109.6", a[10])
	}
	if l := straight(11, true).Length(); l != 100 {
		t.Errorf("length %v", l)
	}
}

// A crop keeps the course's own points between its ends, by distance or by
// time, and refuses one that keeps nothing.
func TestCrop(t *testing.T) {
	c := straight(101, true) // 1 km, 100 s
	got, err := c.CropDistance(200, 500)
	if err != nil || len(got.Points) != 31 || got.Points[0].Distance != 1200 || got.Points[30].Distance != 1500 {
		t.Fatalf("200 m to 500 m: %v points, %v", len(got.Points), err)
	}
	if got, _ := c.CropDistance(900, 0); len(got.Points) != 11 {
		t.Errorf("900 m to the end: %d points", len(got.Points))
	}
	if got, err := c.CropTime(10*time.Second, 20*time.Second); err != nil || len(got.Points) != 11 || got.Points[0].Elapsed != 10*time.Second {
		t.Errorf("10 s to 20 s: %v", err)
	}
	if len(c.Points) != 101 {
		t.Error("cropping changed the course cropped")
	}
	for _, bad := range [][2]float64{{500, 200}, {-1, 100}, {2000, 3000}} {
		if _, err := c.CropDistance(bad[0], bad[1]); err == nil {
			t.Errorf("a crop from %v to %v was accepted", bad[0], bad[1])
		}
	}
	plan := straight(10, false)
	plan.Timed = false
	if _, err := plan.CropTime(time.Second, 5*time.Second); err == nil {
		t.Error("a plan was cropped by time")
	}
}
