# Reference courses

A map of a course can show a **reference** beside it: another course to compare the line
against. The Rhodes parkrun, run eighty times, drawn over the eighty-first; a standard loop
from home; for a flight, the great circle between where it took off and where it landed.
The reference is drawn dashed, in an ink of its own, under the course.

This is the plan. Parts 0 to 3 are built; comparison is next.

## Where references come from

### Your own, first

The references that matter most are the ones only you have: the loops you run, the parkrun
you go to. So the store is built for adding your own from the start -- any file `course`
reads (FIT, GPX, TCX, KML, KMZ), by name, optionally cut from a longer activity: the 5 km
of a parkrun out of a run that also had the warm-up and the cool-down.

### Public databases: nothing openly licensed that fits

- **parkrun** publishes each event's course map on its own site. There is no open dataset
  and no API, and parkrun states that it does not condone scraping its data
  ([parkrun: Scraping](https://www.parkrun.com/scraping/)). Its course maps are not
  something this repository can bundle, fetch on a user's behalf, or build a feature on.
- **Marathons and other events** publish their routes, when they do, as downloads under
  each organiser's own terms, and the routes change from year to year. A user may download
  one for their own use and add it as a reference; `course` will not fetch or ship them.
- **OpenStreetMap** has running routes as route relations (`route=running`,
  `route=fitness_trail`, sometimes a parkrun as `route=foot`), under the ODbL. Coverage is
  thin and uneven, measured on the extracts on this machine: Denmark has 6 running routes,
  8 fitness trails and one parkrun ("Fælledparken Parkrun"); the Sydney extract has none at
  all. Whether parkruns belong in OpenStreetMap at all is disputed there
  ([OSM forum: How to map parkrun?](https://community.openstreetmap.org/t/how-to-map-parkrun/106162)).
  **Decided: no import.** At this coverage it is not worth the code and the licence
  handling; an official course downloaded by hand is added like any other reference.

## How a reference is stored

### The original file, and a manifest beside it

Each reference is a directory holding the file it was made from, byte for byte, and a
small `reference.json`:

```
references/
  rhodes-parkrun/
    reference.json
    source.fit
```

```json
{
  "name": "Rhodes parkrun",
  "aliases": ["rhodes"],
  "source": "source.fit",
  "from_m": 1210, "to_m": 6230,
  "added": "2026-09-29",
  "note": "the course since the 2025 re-route",
  "summary": {"length_m": 5020, "loop": true,
              "bounds": {"west": 151.07, "south": -33.84, "east": 151.09, "north": -33.82}}
}
```

Why the original rather than a converted track:

- **Nothing is lost.** Comparing two runs of a course -- pace along it, heart rate at the
  same hill -- needs everything the recording has, and every conversion so far (FIT to GPX,
  for one) drops fields. `course` already reads every format; converting would only throw
  information away to save a reader it has.
- **Provenance.** A reference is a claim about where a course goes, and the file it came
  from is the evidence.
- **A crop is a note, not an edit.** `from_m`/`to_m` (or `from_s`/`to_s` for a file with
  times and no distance) say which part of the file is the course, so the same recording can
  be re-cut without re-adding it.
- A reference with no file to copy -- an OpenStreetMap route, a great circle -- is written
  as GPX, the one format every tool reads for geometry alone, untimed.

The manifest's `summary` is derived, recomputed whenever it is missing, and exists for one
reason: to rule a reference out without reading its file. With a few hundred references
the matcher reads bounds and lengths from the manifests and opens only the ones that could
match.

### Where the store lives

**Decided:** references are yours, not a cache: they go under the user configuration
directory (`os.UserConfigDir`, so `~/Library/Application Support/course/references` on
macOS), not beside osmbase's store under `Caches`, which the system may empty. `--references DIR`
overrides it. The store is outside every repository, and nothing in it is ever a fixture or
an example; tests build synthetic courses as they do now.

### What matching and comparison need, and why no special format

Both work on one shape: **the reference as a line with distance along it**, resampled to
a point every few metres. Every question then becomes "where along the reference is this
point, and how far off it":

- **Drawing** needs only the line.
- **Comparison** projects each sample of an activity onto the reference, which gives it a
  *reference distance*; two runs compared at the same reference distance give the time
  between them, the pace difference and the deviation, stretch by stretch.
- **Matching** asks what share of the reference the activity passes within some tolerance
  of, in order, and where.

That line is quick to build from the original file -- a 5 km course is five hundred points
-- so it is built when needed, not stored. No geometry database, spatial index or special
file format is needed at this scale; the manifests' bounds are the index. If the store
grows to thousands, a cache of resampled lines can be added then without changing what is
stored.

## Drawing a reference

```
course map run.fit --reference "Rhodes parkrun"
course map run.fit --reference other-run.gpx
course map flight.kml --great-circle
```

- `--reference` takes a stored name (or alias) or a file. **Decided:** more than one can be
  given from the start, each in its own ink and named in the legend.
- The reference is drawn **dashed, under the course**, in an ink distinct from the course's
  and from the dashed grey of a recording gap, and checked for contrast against the palette
  with osmbase's contrast check, as the overlay inks already are.
- A small **legend** in a corner names what the dashed line is ("--- Rhodes parkrun"): a
  dashed line on a map with nothing saying what it is invites the wrong reading.
- The view is fitted to the course **and** its references together, so a reference that
  goes somewhere the course did not is on the picture.
- **The great circle** is the shortest path over the globe between start and finish, drawn
  as a reference named "Great circle", with points every few tens of kilometres along it.

### Seeing what is underneath

As built, the course is drawn narrower when there are references, so one that follows it
closely shows along its edges. Drawing the references over the course instead, translucent,
was tried and buried the course under them. Neither is good enough: the lines hide the map
beneath them -- the paths, the names -- and a reference that follows the course is only a
coloured edge. To try later:

- **Lighter lines altogether**: thinner and translucent strokes, lighter halos, so the map
  shows through.
- **A reference drawn only where it leaves the course**, by more than a few metres: where
  the two coincide there is nothing to see, and where they part the difference stands alone.
  It needs each point's distance from the reference, which matching (part 3) computes, so it
  follows that.
- **Parallel lines**: the reference offset beside the course, as a transit map draws lines
  that share a road.

### First, the 180° meridian

A great circle across the Pacific crosses 180°, and so does the Australia-to-USA flight in
`example_files` -- whose map today draws its Pacific crossing as a dashed line **straight
across the whole world**, from the eastern Pacific west past Africa to Australia, because a
line is drawn from one fix to the next and nothing knows that 179° and -179° are neighbours.
Antimeridian handling was set aside in osmbase earlier. It has to come first here, at least
for lines: split a line where it crosses 180° into pieces that run off each edge of the
map. A view centred on the Pacific -- so the whole crossing is on one picture -- is the
fuller fix and larger, and can follow.

## Matching, later

Once references are stored, a map or a summary can find them unasked:

```
course match run.fit                 # which references, and where
course map run.fit --reference auto
```

- **Rule out** by bounds and start: a reference whose bounds miss the activity's, or whose
  start the activity never comes near, is not read.
- **Whole course**: resample both; the share of the reference within ~25 m of the activity,
  the share of the activity within ~25 m of the reference, and order -- an alignment that
  must move forward along both (dynamic time warping, or the discrete Fréchet distance, with
  a band), so a loop run backwards or a different lap order does not score as the course.
- **Part of an activity**: the parkrun inside a run with a warm-up and a cool-down. For
  each place the activity passes near the reference's start, take the stretch of the
  activity about as long as the reference (within ±10%) and score it the same way; the best
  stretch above the threshold is the match, with where it starts and ends in the activity.
- **Several references in one activity**: two parkruns in one morning, with a warm-up, the
  commute between them and a cool-down. Matching finds every stretch that matches some
  reference, not only the best one, and the stretches may not overlap; the rest of the
  activity is the part that matched nothing. `--reference auto` then draws each reference
  it found.
- **Result**: the reference, the stretch of the activity, and how well -- coverage, median
  and worst deviation -- so a map can say "Rhodes parkrun, 0.4 km in to 5.4 km, within 6 m".

Averaging many runs of one course into a reference -- eighty parkruns make a better line
than any one of them -- falls out of the same alignment, and is a later refinement.

## Comparison

`course map run.fit --compare "Rhodes parkrun"` -- a stored reference or a file -- aligns the
run with the reference by the matching above, notes when each reached every 10 m of the
reference, and colours the course by the run's pace against the reference's there: the log
of their speeds, over 30 m either side, on a scale from 15% slower (blue) to 15% faster
(red). The warm-up and cool-down outside the matched stretch stay thin and grey. The report
says how far ahead or behind the run finished. It needs the reference to keep its times,
which the original file does; a course file without them is refused. The organisers'
`Three Bridges.gpx` turned out to have times, at a steady 12 min/km from whatever planned it,
and is compared with as if somebody had walked it: nothing in a file tells planned times
from recorded ones.

How finely the colours change is the drawing's to decide, not the comparison's: osmbase's
`render.Gradient` takes a value at every point and colours pieces a few pixels long, each
the length-weighted average of its values, so a whole marathon and one corner of a parkrun
are both coloured as finely as the picture can show and no finer. The same drawing is meant
for pace, height or power along a single run later.

`course compare run.fit --reference "Rhodes parkrun"` is the same comparison in words: splits
of `--split` kilometres (1 by default) measured along the reference -- the same ground for
both, however far either ran over it -- with each run's time over the split, which was
faster, and the running gap at its end; then the run's stops, which are usually why a split
was slow. Text, CSV, JSON or YAML, like `match`.

Not yet: the reference's own stops. A reference run that stopped makes the run look fast
over that split, and nothing says why.

## Parts

| # | part | where | done when |
|---|---|---|---|
| 0 | Lines across 180° | osmbase render | a line crossing the antimeridian is split into pieces running off each edge; the trans-Pacific flight's map shows no line across the world |
| 1 | Draw a reference from a file, and the great circle | course | `--reference FILE` and `--great-circle` draw dashed, under the course, with a legend; the view holds both |
| 2 | The reference store | course | `course reference add/list/show/remove`, crops by distance or time, names and aliases in `--reference` |
| 3 | Matching | course | `course match`, whole and partial, `--reference auto` |
| 4 | Comparison | osmbase render, course | `map --compare` colours the course by pace against a reference; `course compare` gives splits and the running gap |

## Courses to test against

Real recordings, for checking by hand; never fixtures.

- **Rhodes parkrun**, four runs (`rhodes_parkrun_*`), and 2026-08-22's among them. The one
  on 2026-06-13 took a slightly different course with a bridge closed: it should match, less
  well, and the difference should show where the bridge was.
- **Two parkruns in one morning**: the 2025-06-28 files in fitdash's example_files, which
  merge into one run with a warm-up, both parkruns, the commute between them and a cool-down.
- **An official course against the run**: `Three Bridges.gpx`, the organisers' course for the
  2026-09-27 race -- with two road-work detours the file may not show and one to a toilet --
  and `TCS_Sydney_Marathon_2026_Course.gpx` for the 2025-08-31 marathon. Planned courses with
  no times: they test matching a plan against a recording, and deviations that are real.

## What matching measured

Built as subsequence dynamic time warping on points every 10 m, with each match's detours
measured along the in-order alignment. On the courses above:

- **The tolerance is 25 m.** The marathon, among the city's buildings, wanders up to about
  20 m from its course followed exactly; at 15 m it reported 25 "detours", at 20 m seven, at
  25 m one. The Rhodes runs other than the bridge closure are within 4-6 m.
- **The bridge closure is found**: 92% of the course covered, a missed stretch from 0.65 to
  1.03 km up to 87 m off, and the turnaround that day further north.
- **Both parkruns of the two-parkrun morning are found where they were**, from 2.37 km and
  from 11.20 km, and a Rhodes reference given alongside matches nothing.
- **The official courses match their races completely.** Three Bridges shows nothing 25 m
  off for more than a moment -- the road-work detours were near the course or are in the
  file -- and the marathon one stretch 32 m off at 9.2 km, which may be the course changing
  between the 2025 run and the 2026 file.
- **Measured nearest-anywhere, a detour vanished** on an out-and-back course -- the way
  back runs beside the way out. So coverage and detours are measured along the alignment,
  in order; which also makes a loop run backwards fail to match, without a check of its
  own.
- **A stray fix is not a detour.** One fix 40 m off makes a spike 80 m long, longer than a
  short detour, so an excursion in an activity with times must last 10 s; a stop or a
  detour lasts tens.
- A marathon against its course takes under half a second.

## Open questions

- **The rendering**: a reference that follows the course is hidden under it; see "Seeing
  what is underneath".
