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

## What was learned building the summary

Measured on real courses -- runs from 5 km to a marathon, domestic and long-haul flights --
which are kept out of the repository:

- **Natural Earth's coastline is a kilometre out.** A marathon along a beachfront was in
  Queensland, then nowhere, then Queensland, for twenty-five kilometres at a time. A gap in
  country or region with no named water in it, and the same place either side, is bridged
  -- as Near, never as Contained -- up to 50 km.
- **Its seas reach onto the land by the same error**, so named water under a course for
  less than a kilometre is not named: a run round a headland is not over the Tasman Sea.
- **Flicker needs folding, scaled by depth.** A footpath beside a road alternates with the
  road as the nearest street; a flight up the Scottish coast alternates between islands and
  sounds every thirty seconds. A stretch shorter than a depth's minimum is folded into the
  one before it, and a detour that returns to the same place is folded up to a larger one --
  but a sea crossing between two parts of one country (the Great Belt, 18 km) is kept.
- **A row is labelled by the land when it has any.** A coastal airport is in France and over
  the Golfe du Lion; the flight departed France.
- **An empty lookup cannot say why.** A run through a city the store holds no map of has no
  street names, exactly like a run across open country, and "street" then looked like a
  depth that fit. Auto now measures the tiles held along the course at each level's zoom
  (osmbase's `Level.Zoom`) and chooses only among depths the store can answer.
- **The budget holds at the widest depth too.** An archipelago is island, sea, island at
  country depth; when even that is over the budget, the shortest stretches are folded until
  it fits, never the start or the finish.
- **Without suburb outlines, a neighbourhood is the nearest label** -- "Koreatown", "Ben
  Buckler". osmbase now prefers a suburb label in reach, and with outlines built from an
  extract (`osmbase boundaries --osm`) neighbourhoods are answered by containment.
- **An outline crossing is a fact, not flicker.** A run along Powells Creek, the boundary
  between two suburbs, crossed it every 150 m; answers that hold the course fold only below
  100 m, where the limits above are for nearest-label answers.
- **Parks, campuses and airports** are a level of their own, `area`, answered by the polygon
  holding the course (osmbase's `Within`); a run inside Bygholm Park was otherwise the streets
  around it.
- **"On a street" means on it**: osmbase's `OnWay`, and 30 m rather than 250.
- **The prefix is the prominent city**, not the nearest locality (Parramatta for Sydney Olympic
  Park) and not the council; `--prefix` keeps the alternatives to compare.
- **Distance is not computed.** A course reports what its file recorded; a computed
  distance along the track may come later, as a deliberate addition.

## The map, as built

`course map` fits the view with osmbase's `render.Fit`, up to zoom 18 -- a lap of a park fitted
at the public builds' 15 was a small loop in four kilometres of town -- and draws with
`render.Draw` and `render.DrawCredit`, both moved into osmbase's library for it. What is drawn
is `routemap`'s: the line thinned to a pixel, dashed across a gap five times the recording's
own rhythm and a few hundred metres wide, and distance markers only where the file recorded
distance, at the Python original's intervals. The fetch offer is shared with the summary,
which asks only for what its depth needs -- a local course's area, a wide course's ends.

## Open questions

- **The row limit** (`--max-rows`). 25 holds for runs of 5 km to a marathon and for long-haul
  flights; a city run through thirty suburbs is over it, and a ride and a drive have not been
  tried. Fixed for now, by decision.
- **The prefix's source** -- `city`, `locality` or `none` -- to be decided by comparing.
- ~~**Part 3's representation.**~~ A separate type: `fitactivity.Route`, from `ReadRoute`;
  `Read` refuses a file without times with `ErrNoTimes` (fitactivity v0.7.0).
- ~~**The fetch offer.**~~ Moved into osmbase's library as `fetch.Consent` and `fetch.Fill`
  (v0.11.0), and fitdash is on it. `summary` names what the store lacks; offering to fetch it
  is part 8's, shared with the map.
- ~~**Output formats.**~~ `github.com/wisborg/output`: text, CSV, JSON and YAML.
- **Test data.** Synthetic courses for every test, as elsewhere. Real recordings — FIT files,
  `~/Source/gtrack`'s KML and KMZ — are for checking by hand and never enter a fixture.
