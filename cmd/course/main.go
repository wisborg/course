// Command course summarises and maps a recorded or planned course.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/wisborg/output"
)

// formatFlag adapts output.Format to pflag, which wants Type() beside the
// standard library's String and Set.
type formatFlag struct{ output.Format }

func (formatFlag) Type() string { return "format" }

var root = &cobra.Command{
	Use:   "course",
	Short: "Say where a course went, and draw it on a map",
	Long: `course reads a course -- a run, a ride, a flight, or a route somebody planned --
and says where it went, or draws it on a map.

A course is a FIT, GPX, TCX, KML or KMZ file. What it carries is what is
reported: a GPX has no distance, a planned route has no times, and neither is
estimated.

The places come from an osmbase store on this machine, filled by "osmbase
fetch" and, for answers by containment, "osmbase boundaries". What the store
lacks for a course is offered before it is fetched, and fetching it tells the
host the area; otherwise nothing about the course is sent anywhere.`,
	Version:       version(),
	SilenceUsage:  true,
	SilenceErrors: true,
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "course: %v\n", err)
		os.Exit(1)
	}
}
