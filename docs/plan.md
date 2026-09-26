# course — summarise and draw a recorded course

`course` takes a recording of a course — a run, a ride, a flight — as FIT, GPX, KML or KMZ,
and does two things with it:

```
course summary ACTIVITY [--detailed] [--depth auto|street|neighbourhood|locality|region|country]
course map     ACTIVITY [--out course.png] [--width N] [--height N]
```

- **summary** says where the course went. The one-liner is a chain of places —
  `Richmond → Kew → Putney → Fulham`; `--detailed` is the change log behind it,
  one row per change of place with elapsed time and distance.
- **map** draws the course over the map: the route, its start and finish, and distance markers.

Both work from data already on disk. The places come from an osmbase store's boundaries and
tiles, and the map from its tiles. Nothing about the course is sent to anybody. The Python
original this replaces sent a reverse-geocoding request per sampled point, and the whole route
as a polyline to Mapbox's static-map API; here the route never leaves the machine.

## Where each piece lives, and why

| piece | lives in | because |
|---|---|---|
| drawing lines and markers over a map | **osmbase** `render` | geometry over the map it already projects, clips and draws; no new dependency |
| reading GPX, KML and KMZ | **fitactivity** | beside FIT, into the same `Track`, so fitdash and videofx read them too instead of a second copy of every reader |
| the course type, the summary, the map's decisions, the CLI | **course** (this module) | policy built on osmbase's data, and dependencies osmbase refuses to carry |

The split between osmbase and course follows `osmbase/docs/locate.md`, "What does not live
here", which decided it before this plan existed: route summarising needs `fitactivity` and a
rich output module, four modules for a library that has two, and osmbase is about the data,
not about what a consumer does with it.

The split on drawing is by kind of decision. **osmbase** strokes a polyline with a halo and
clips it, draws a marker with an optional label, and fits a view to a rectangle — all of which
it already does for its own roads and names. **course** decides which points form the route,
which fixes are rogue, where the start, finish and kilometre markers go, and in what colours.

## The summary

### It is a change log, not a sample

Walk the course, ask what it is in, and emit a row only when a name at some reported level
differs from the previous row. A four-hour activity becomes the handful of rows naming the
places it passed through. `locate.AtEach` answers thousands of points from a handful of tile
reads and now builds each point's containment stack once, so lookups are cheap: sample
generously and let the change detection do the work. The Python original's thinning — a lookup
every 120 s or 200 m, or on leaving a locality's bounding box — existed because Mapbox is
metered, and goes. So does its polyline-length iteration, which existed for a URL limit.

### Two controls, not one

**Shape** — the one-liner or `--detailed` — and **depth** — the finest level reported — are
separate, because they answer different questions. A local run and an intercontinental flight
both want a one-liner; they want it at very different depths.

osmbase's levels, widest first: country, region, water, locality, macrohood, neighbourhood,
street. No postcode, which Mapbox had. `water` is new, and gives a flight something the
original could not say: over the Tasman Sea.

### Depth defaults to automatic, by a row budget

`--depth auto` computes the change log at each depth and takes the FINEST whose log fits a
budget of rows. A local run's streets fit and are reported; a long-haul flight's street changes
number in the thousands, so it falls back to regions and countries by itself — with no speed or
distance thresholds to tune, and no guess about what kind of activity it was. The budget is the
one number to settle, measured on real courses of each kind; about 25 rows is where to start.

Any depth can be asked for explicitly, and the one-liner follows the same depth: at `street` a
short run is a chain of streets, at `locality` a chain of suburbs.

Flights get one more consideration: street and neighbourhood lookups at cruising altitude are
true answers to a question nobody asked, which `osmbase locate --levels` already argues. The row
budget handles the output; the lookups themselves can skip the fine levels once the depth is
chosen, since they are not reported.

## The map

`course map` fits the view to the course, as `render --place` fits one to a place, draws the
basemap from the store, and the course over it:

- the route, thinned to what the image can show, with rogue fixes and 0,0 points left out;
- start and finish markers;
- distance markers at an interval chosen from the course's length — the Python original's
  table is the starting point: none under 1.2 km, every kilometre up to 15 km, then 2, 3, 4
  and 5 km as the course grows to 50 km, and every 10 km beyond.

When the store lacks the tiles the view needs, it offers to fetch them the way `osmbase render
--store` does. That offer lives in osmbase's command, not its library; whether to move it into
the library or repeat it here is part 8's to decide.

## What the readers must not invent

Every `fitactivity.Sample` carries a `Time`, and fitdash and videofx both rely on it. A GPX or
KML file is often a PLANNED route with no times at all. Such a file is a valid course to
summarise and to map, and has no elapsed time to report. It must not be given invented
timestamps — that is the "absence is not zero" rule this family of projects is built on — so
part 3 has to decide how it is represented: a `Track` flag saying the times are absent, or a
separate geometry-only result. Whichever it is has to be checked against fitdash and videofx,
which would otherwise animate a planned route at a speed nobody moved at.

GPX and KML rarely carry distance, where FIT always does. The course type computes distance
along the track when the file does not supply it, so distance markers and the change log's
distances work for every format.

## Parts

Each is reviewable on its own and lands in the repository it belongs to.

| # | part | where | done when |
|---|---|---|---|
| 1 | Overlay | osmbase | `render.Options` takes lines and markers in coordinates, drawn over the basemap with a halo, clipped, in inks checked against the palette's; the command gains nothing yet |
| 2 | GPX reader | fitactivity | tracks, segments and route points into a `Track`; elevation, time, and Garmin's heart rate and cadence extensions; its own branch, checked against fitdash and videofx |
| 3 | Time-less courses | fitactivity | the representation decided above, and a planned route read without invented times |
| 4 | KML and KMZ reader | fitactivity | `gx:Track` with times, `LineString` without, KMZ through `archive/zip` |
| 5 | One entry point | fitactivity | a decode that tells FIT, GPX, KML and KMZ apart by content, not by extension |
| 6 | The course type | course | a course from any of them: thinned, rogue fixes out, distance computed where absent |
| 7 | Summary | course | the change log, the one-liner, `--depth` with the row-budget `auto`, text and JSON |
| 8 | Map | course | `course map` over osmbase's render and part 1's overlay, the offer to fetch |

Part 1 is first because it is small, sits in a library that already has the pieces, and is
useful to fitdash too, which draws its own route over osmbase's basemap today. Parts 2 to 5
follow the rule fitactivity's consumers set: each on its own branch, verified against both
fitdash and videofx before it is done.

## Open questions

- **The row budget.** 25 is a guess; measure it on a run, a ride, a drive and a flight.
- **Part 3's representation.** A flag on `Track`, or a separate type. The flag is less code; the
  separate type cannot be animated by accident.
- **The fetch offer** — moved into osmbase's library for course and fitdash to share, or
  repeated here. fitdash already has a third copy of the idea.
- **Output formats.** Text and JSON at least; `github.com/wisborg/output` would give the rest,
  at the cost of its dependencies, which this module can afford where osmbase could not.
- **Test data.** Synthetic courses for every test, as elsewhere. Real recordings — FIT files,
  `~/Source/gtrack`'s KML and KMZ — are for checking by hand and never enter a fixture.
