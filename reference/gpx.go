package reference

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"os"
	"time"

	"github.com/wisborg/course"
)

// writeGPX writes c to path as a GPX track called name: every point's
// position and, for a course with times, its time -- c.Start, or the Unix
// epoch for an average that has no one day, plus its Elapsed -- so that
// course.Read gives back the same line and the same times.
func writeGPX(path, name string, c *course.Course) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintln(w, `<gpx version="1.1" creator="course" xmlns="http://www.topografix.com/GPX/1/1">`)
	fmt.Fprint(w, "<trk><name>")
	xml.EscapeText(w, []byte(name))
	fmt.Fprintln(w, "</name><trkseg>")
	start := c.Start
	if start.IsZero() {
		start = time.Unix(0, 0)
	}
	for _, p := range c.Points {
		fmt.Fprintf(w, `<trkpt lat="%.7f" lon="%.7f">`, p.Lat, p.Lon)
		if c.Timed {
			fmt.Fprintf(w, "<time>%s</time>", start.Add(p.Elapsed).UTC().Format("2006-01-02T15:04:05.000Z"))
		}
		fmt.Fprintln(w, "</trkpt>")
	}
	fmt.Fprintln(w, "</trkseg></trk></gpx>")
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
