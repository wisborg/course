# course

Say where a recorded course went — a run, a ride, a flight, or a route somebody planned —
from FIT, GPX, TCX, KML or KMZ, using map data already on your own disk. Nothing about the
course is sent to anybody.

```
make build
./course summary run.fit
./course summary --detailed run.fit
./course summary --depth locality --format json ride.gpx
```

`summary` prints one line — a chain of the places the course passed through,
`Richmond → Kew → Putney → Fulham` — and `--detailed` the change log behind it: a row each
time the course entered a different place, with how far in it was.

`--depth` is the finest level named: `country`, `region`, `city`, `locality`, `macrohood`,
`neighbourhood`, `area` (a park, a campus, an airport) or `street`. Named water — a sea, a
strait, a bay — is named at every depth. The default, `auto`, takes the finest depth whose
change log has no more than `--max-rows` rows (25): a local run is named street by street,
and a flight falls back to countries and seas by itself. Auto only considers depths that
name something along the course and that the store holds the map for, and says on stderr
when it lacks some.

A summary finer than a locality names the place the whole course was in first —
`Sydney: Woolloomooloo → Darlinghurst` — chosen by `--prefix`: `city` (the city's mapped
extent where the store has city outlines, and otherwise the place whose reach the course
is most within — a big city reaches further than a town), `locality` (the locality level's own answer, which is the council where
suburb outlines answer it) or `none`. A loop run more than once is written once,
`4x (James Park → Hornsby)`, and a flight names the airports it left and reached where the
store holds them.

At street depth, a stretch on no named way -- a path along a creek, a station concourse -- is
named by its suburb when it is at least `--suburb-gap` metres long (300) or in a different
suburb from the streets either side, and otherwise left out of the line as a gap between
two streets. `--detailed` shows every stretch, with its length.

A course reports what its file recorded and nothing else. A GPX has no distance, so its
rows have none; a planned route has no times, so its rows have no elapsed time. Neither
is estimated.

## Where the places come from

An [osmbase](https://github.com/wisborg/osmbase) store: `osmbase boundaries` for country,
region and sea outlines, and `osmbase fetch` for the map the finer levels are read from.
A name read from OpenStreetMap data owes its credit, which `summary` prints with every
answer and writes into its JSON; see [NOTICE](NOTICE).

## The map

```
./course map run.fit                 # -> run.png
./course map --palette dark --width 1920 --height 1080 flight.kml
```

`map` draws the course over the map: the route, its start and finish, and a marker every
so many kilometres -- or miles, see [Units](#units) -- where the file recorded distance. A stretch the recording has no fixes
for -- a tunnel, a flight over an ocean -- is dashed, because the straight line across it is
not where the course went. The picture carries the map data's credit in its corner.

```
./course map --terrain hike.fit      # the hills shaded, with contour lines
```

`--terrain` (or `map.terrain: true` in a style) shades the shape of the ground under the map
and draws contour lines, every fifth one labelled with its height; `--contours=false` keeps
the shading and leaves the lines out. The elevation comes from
[Mapterhorn](https://mapterhorn.com) and is kept beside the map's store, where `osmbase`
finds it too; what the view lacks is offered before it is fetched, like the map. The
picture's credit then names the elevation briefly, and the report prints its full notice --
for Copernicus GLO-30, a sentence its licence dictates -- which **whoever publishes the
picture must give with it**, in a caption or a description. See [NOTICE](NOTICE).

```
./course map --3d --palette outdoors hike.fit                 # the course over the hills, in perspective
./course map --3d --heading 200 --set view.pitch=50 hike.fit  # looking south-south-west, more steeply
```

`--3d` (or `view.mode: 3d`) draws the map in perspective instead, from a camera in the sky,
the ground shaped by its heights: the course, its colouring and its references lie on the
ground and follow it over hills, and the start, finish, distance markers and the names of
places stand upright where they are seen -- left out where a hill hides them. It needs the
terrain, and offers it as `--terrain` does; without it the ground is drawn level, and the
report says so. The camera takes in the whole course. `--heading` says which way it looks,
0 north; `auto`, the default, takes whichever bearing shows the course largest, which lays a
long course corner to corner. `view.pitch` (35° below the horizontal), `view.fov` (40°) and
`view.exaggeration` (1, true to life) are the rest of the camera. The `outdoors` palette's
greener woods and darker lines suit it best.

The course is drawn thin and translucent, without a halo, so the streets, paths and names
under it show through. Where a reference is drawn whole beside it (see below), the lines are
drawn opaque instead, with a slim halo: two translucent lines over one another mix into a
third colour, and which is which is lost.

Names are written in the built-in Go font, and letters it lacks -- Thai, Indian scripts,
Chinese, Arabic -- in the first of the fonts given with `--font`, then of the common system
fonts this machine has (Arial Unicode and Apple's script fonts on macOS, Noto on Linux),
that has them. Arabic and Hebrew are drawn right to left, with Arabic's letters joined.
Scripts that reorder letters within a syllable, such as Devanagari and Myanmar, need a text
shaper for their exact forms and are drawn letter by letter. `--lang en` writes names in
English wherever the map has them, and `map` says when some letters had no font at all.
The system fonts are read on this machine and never copied anywhere.

### Map styles

How a map looks -- its palette and size, the size of its text and of the map's own names,
its legend and distance markers, the units it is labelled in, what its course is coloured by, and the colour, width,
opacity, style and halo of its lines -- is a style, made in three layers, each changing only what it says: the built-in defaults, then a
style file given with `--style`, then single settings given with `--set`. The file is YAML,
and so may be JSON.

```
./course map style > mytheme.yaml                 # every setting, with what it takes
./course map run.fit --style mytheme.yaml
./course map run.fit --style mytheme.yaml --set course.colour=#d32f2f --set reference.style=dotted
```

`course map style` prints the style the same flags would draw in, every setting with a
comment, so it starts a theme from the defaults and, given `--style` with an old theme,
brings it up to the settings there are now.

```yaml
palette: dark            # light, dark or outdoors; --palette is the same as --set palette=...
width: 1920              # the picture, in pixels; --width and --height are the same
height: 1080             #   as --set width=... and --set height=...
text:
  size: 13               # start, finish, distance numbers, legend, credit; scaled
map:
  label-size: auto       # the map's own names: auto is 13 pixels at any size, or a
                         #   size on a map 1000 across, scaled with the picture
view:
  mode: flat             # flat, or 3d; --3d is the same
  heading: auto          # with 3d, degrees from north, or auto; --heading is the same
  pitch: 35              # with 3d, degrees below the horizontal, 10 to 90
legend:
  position: auto         # a corner, auto, or none; --legend is the same
markers:
  every: auto            # distance units between distance markers, auto, or none
units:                   # --units and --unit are the same; see Units
  system: metric         # metric or imperial
  distance: auto         # km, mi or nmi, or auto for the system's
  elevation: auto        # m or ft
  speed: auto            # km/h, mph, kn or m/s
  pace: auto             # min/km or min/mi
  temperature: auto      # C or F
colouring:               # --colour, --grade-cap and --power-source are the same
  by: grade              # none, or pace, speed, grade-adjusted-pace, elevation, ...
  grade-cap: 15
  power-source: auto
  temperature-source: auto
  width: 3               # the coloured line's width
course:                  # the course, and with --separate every activity
  colour: "#ff5252"      # #rrggbb or #rrggbbaa, or auto for the palette's own
  colours: []            # with --separate, the 2nd, 3rd, ... activities' colours
  width: 4               # pixels on a map 1000 across, scaled with the map
  opacity: auto          # 0 to 1, or auto: 0.7, but 1 when a reference is drawn whole
  style: solid           # solid, dashed or dotted
  halo: auto             # pixels either side, or auto: slim when opaque, else none
activities:              # with --separate: settings for activity 1, 2, ... in turn
  - {}
  - colour: "#40c4ff"
reference:               # every reference
  colour: auto           # one colour for all, or auto to take colours in turn
  colours: []
  width: 2.25
  opacity: auto
  style: dotted
  beside: true           # where it shares a road with the course, beside it, not under
references:              # one reference, by its name
  Rhodes parkrun:
    colour: "#0077aa"
```

A theme that colours its maps colours every map drawn with it; `--colour none` undoes that for
one map. `--compare` colours the course too, so with a style's colouring it is refused until
`--set colouring.by=none` is given as well.

A setting for one activity or one reference says only what differs from `course` or
`reference`. On the command line a setting's path is its keys joined by dots, an activity
numbered from 1 as on the map: `--set activities.2.width=5`,
`--set 'references.Rhodes parkrun.colour=#0077aa'`, `--set 'reference.colours=[#c2185b, #00695c]'`.
A colour may be typed without quotes. A setting the style does not have is refused by name,
in a file as on the command line, so a misspelt one is never silently ignored.

### Several activities

```
./course map warm-up.fit parkrun.fit cool-down.fit                # merged into one
./course map --separate warm-up.fit parkrun.fit cool-down.fit     # each on its own
```

Several files are merged into one activity, ordered by when each started -- a run recorded in
pieces, as fitdash merges them -- and drawn as one course, with a dashed gap wherever the
recording has one. `--separate` draws each as its own activity instead: in its own colour,
with its own distance markers and its own start and finish, numbered in the order the files
were given, and a legend line for each, its file's name or one `--title` for each file in
turn. Where one activity finishes and the next starts, the two are one dot labelled with both,
"Finish 1 · Start 2". With `--colour`, the activities share one scale, so the same colour is
the same pace, or grade, on each.

### The legend

A map with references or colours has a legend saying what each line or colour is. By default
it goes in whichever corner covers least of the course, its markers and their labels;
`--legend top-left` (or `top-right`, `bottom-left`, `bottom-right`) puts it there, above the
map's credit in the bottom right, and `--legend none` leaves it out. The legend calls the
course by its file's name unless `--title` names it; a title, or a corner, asks for a legend
even on a map with nothing else in it.

```
./course map hike.fit --colour grade --title "Oia to Fira" --legend bottom-left
```

### Colouring by a metric along the course

```
./course map run.fit --colour pace
./course map ride.fit --colour speed
./course map ride.fit --colour elevation
./course map walk.fit --colour grade
./course map hike.fit --colour grade --grade-cap 30
./course map run.fit --colour heart-rate
./course map run.fit --colour power --power-source native
./course map run.fit --colour air-power
./course map run.fit --colour cadence
./course map hilly.fit --colour grade-adjusted-pace
./course map hike.fit --colour temperature
./course map run.fit --colour humidity
```

The metrics are `pace`, `speed`, `grade-adjusted-pace`, `elevation`, `grade`, `heart-rate`, `power`,
`air-power`, `cadence`, `temperature` and `humidity`.

`--colour` colours the course by a metric along it, from blue at its slowest or lowest to red
at its fastest or highest, with a colour bar in the legend giving the pace, speed or height at each
end, in the units asked for. The ends are set by the course itself, with the few most extreme values at each end left
out, so a sprint finish or a standstill does not squeeze the rest into one colour; an even
run or a flat course is shown over at least 5% of its speed either side, or 20 m of height,
rather than spreading the whole ramp over noise. Pace and speed are taken over 30 m either
side of each point; `speed` is the same colouring as `pace`, read the way a cyclist or a
pilot reads it.

`grade` is the slope: red climbing, blue descending, green level in the middle of a scale
that reaches the course's own steepest grade either way -- at least 3%, so level ground is not
painted in the ramp's strongest colours, and at most `--grade-cap` (15% unless told), past
which the ends mean "this steep or steeper" and the legend says so. A mountain hike can be
steeper than 15% for a third of its length; `--grade-cap 30` tells those stretches apart
instead of drawing them all one colour. It is
taken from the elevation smoothed by [fitactivity](https://github.com/wisborg/fitactivity),
tuned to the device's own total ascent and descent where the file has them as videofx and
fitdash tune it, then over 10 m either side of each point: narrower than the 30 m those two
read a grade over, because a map is looked at for the steep pitches and they are short -- a
staircase that drops 8 m in 22 m reads -26% here and -14% over 30 m. Each few pixels of the
line are coloured by the steepest grade in them rather than their average, so a pitch much
shorter than the map can show still stands out at a whole course's zoom. A raw altimeter or
GPS altitude wanders by metres, and a slope taken from it unsmoothed is a saw-tooth of climbs
that were never there.

`heart-rate` and `power` are the recording's own readings, averaged over 30 m either side of
each point by how long each was held, so a power meter's second-to-second swings do not fleck
the line; the scale runs from the course's lowest to its highest, as pace does, and at least
10 beats a minute or 5% of the power either way. Many recordings carry power from two
sensors that disagree by a quarter or more -- a footpod such as Stryd, and the watch's own.
`--power-source` picks which, exactly as it does in videofx and fitdash: `auto` (the default)
the footpod's where there is one and the watch's otherwise, `stryd` or `native` only that
one. The legend and the report say which sensor the colours are.

`air-power` is a Stryd footpod's estimate of the power spent against the air -- mostly
headwind, below zero with the wind behind -- so unlike the rest of a run it depends on which
way the course faces and what shelters it: on a loop run on a windy day one side is red and
the other blue. `cadence` is steps a minute for running, walking and hiking, where a FIT file
counts one leg, and revolutions a minute otherwise, as fitdash shows it. Both are averaged
over 30 m either side, as heart rate and power are.

`grade-adjusted-pace` is the pace as it would be on level ground for the same effort: the pace
times how much more, or less, running at that grade costs than running on the flat, by the
energy cost Minetti and others measured on a treadmill (J Appl Physiol, 2002). A climb at 10%
costs about 1.7 times the flat; a gentle descent costs less, and past about -20% more again.
Pace and grade are taken over the same 30 m either side.

The ends of every scale but grade's leave out the most extreme 5% of the course's ground --
by distance, not by points, so a toilet stop of ten minutes recorded a point a second does
not become the slow end of the pace scale.

`temperature` is the air's where a Stryd footpod measured it, and the watch's own otherwise.
The two are not the same reading: a watch on a wrist reads warm, a few degrees above the
footpod out in the wind, so `--temperature-source` chooses as `--power-source` does --
`auto` (the footpod's if there is one), `stryd`, or `native` -- and the legend says which,
in `°C` or, under `--units imperial` or `--unit temperature=F`, `°F`. `humidity` is a
footpod's alone, FIT having no field of its own for it. Both are taken over 30 m either
side, and shown over at least 4 °C and 10%, so a degree of drift is not drawn as
weather.

Where the file has no value -- no elevation for a stretch, a gap in the recording --
the course is not coloured, and a course without times or without any elevation is refused
rather than drawn in one colour.

### References

```
./course map run.fit --reference other-run.fit --reference official-course.gpx
./course map flight.kml --great-circle
./course map --separate leg1.kml leg2.kml --great-circle=both
```

`--reference` draws another course beside this one for comparison -- any file `course` reads,
repeatable -- and `--great-circle` the shortest way over the globe between the course's start
and finish. Each is dashed, under the course, in a colour of its own, and a legend says which
line is which. The view holds the course and all of them.

With several files, `--great-circle` (or `--great-circle=overall`) is the great circle from the
first start to the last finish, `--great-circle=each` one for each file -- each flight of a
journey against its own shortest way -- and `--great-circle=both` all of them; whether the
files are merged or `--separate`. The value is joined with `=`: after a space it would be
taken for another file.

A reference the course followed is drawn only where the two part -- a closed bridge, a
detour, the stretch a late-started watch never saw -- so the map is not buried under a
second line that says nothing new; the legend says "where the course left it", or
"followed all the way" when there is nothing to draw. `--whole-references` draws them whole
again. A reference the course did not follow, and the great circle, are always drawn whole.

A reference drawn whole is drawn beside the course where the two share a road, as a transit
map draws lines that share a street: in a lane of its own to the right of its direction,
each further reference in a lane outside the last, easing out of its lane where it leaves the
course and back in where it rejoins. A reference that crosses the course is left alone, and
an out-and-back is drawn on both sides of the road, as a bus route is. `--set
reference.beside=false` draws it under the course instead, the course narrowed so the
reference shows along its edges, as before.

References you use often can be stored by name:

```
./course reference add "Rhodes parkrun" run.fit --alias rhodes --from 6m --to 34m
./course reference list
./course map today.fit --reference rhodes
```

A stored reference is the original file, unchanged, with a note of which part of it is the
course -- `--from`/`--to` in time, or `--from-km`/`--to-km` -- kept under `course/references`
in your configuration directory (`--references` for elsewhere).

Given several runs of one course, `reference add` stores their average, which is a better
line than any one of them and, with times, the pace of a typical run to `--compare` against:

```
./course reference add "Rhodes parkrun" usual.fit parkrun-*.fit --from 6m --to 34m
```

The first file is the course, cropped as `--from`/`--to` say; every other one is matched to
it, so its warm-up and cool-down are left out without cropping. At every 10 m of the course
the reference is the median of where the runs were, and of how long each had taken from the
start -- the median, so a detour one run took, a closed bridge, does not bend the line. Each
run is reported with how closely it followed the average, and one that does not follow the
course is kept with the reference but left out of it. The average is stored as a GPX, and
the runs beside it as they were.

`match` finds where an activity followed stored references -- a parkrun inside a longer run,
both parkruns of a morning, an official race course -- and how closely, with where it
missed the course or left it; `map --reference auto` draws every reference it matched.

```
./course match morning.fit
./course map morning.fit --reference auto
```

`--compare` colours the course by how much faster or slower it was than another run of it --
a stored reference or a file -- place by place, from blue (15% slower) to red (15% faster),
and says how far ahead or behind it finished. The two are lined up by where on the course
each was, so a wide corner or a stop does not put them out of step. It needs the other run's
times, so a course file without them will not do.

`compare` says the same in words: split by split along the course, how long each took,
and how far ahead or behind the run was at the end of each, with any stops listed under.

```
./course map today.fit --compare rhodes
./course compare today.fit --reference rhodes --split 0.5
```

`--split` is in the distance unit: half a kilometre here, half a mile with `--units imperial`.

The design and what was measured on real courses are in
[docs/references.md](docs/references.md).

## Units

```
./course compare today.fit --reference rhodes --units imperial
./course map flight.kml --units imperial --unit distance=nmi --unit speed=kn
./course map ride.fit --colour speed --unit speed=m/s
```

Every command reads and writes distance, elevation, speed, pace and temperature in metric
unless told otherwise. `--units imperial` changes them all -- miles, feet, mph, min/mi, °F -- and
`--unit QUANTITY=UNIT` changes one, after the system, for the mixtures some activities are
read in: a flight has its altitude in feet, its distance in nautical miles and its speed in
knots. The units are the common ones for each:

| quantity    | units                   |
|-------------|-------------------------|
| `distance`  | `km`, `mi`, `nmi`       |
| `elevation` | `m`, `ft`               |
| `speed`     | `km/h`, `mph`, `kn`, `m/s` |
| `pace`      | `min/km`, `min/mi`      |
| `temperature` | `C`, `F`              |

A short distance -- how far a run strayed from its reference in `match` -- is in the
elevation unit, metres or feet. A number given in a distance follows the distance unit:
`--split` for `compare` and `markers.every` for `map` are miles under `--units imperial`.
`--from-km`/`--to-km` stay in kilometres, as their name says, and `--near` in metres.

For a map the units are part of its style, the `units:` group, so a theme can carry them;
`--units` and `--unit` set the same settings. Only what is written for a person changes:
`--format json`, `csv` and `yaml` stay in metres, seconds and kilometres whatever is asked,
so a program reading them reads one thing. The conversions are
[fitactivity's](https://github.com/wisborg/fitactivity/tree/main/units), shared with the tools
that draw the same activities.

## Fetching what the store lacks

Both commands check the store before they read it, and offer to fetch what is missing --
which tells the archive's host where the course went, so it is never done without a yes,
typed or given with `--yes`, and never asked where nobody can answer. The map fetches its
view, and with `--terrain` the view's elevation from Mapterhorn's host, asked about
separately since it is a second host. The summary fetches only what its depth needs: the whole of a local course at street
detail, and for a flight or a long drive just a few kilometres round each end, where the
airports are; the rest is named from country and sea outlines. Declined, both carry on
with what the store holds, and say what that cost.

The plan, and why the work is split between this module, `osmbase` and `fitactivity`, is
in [docs/plan.md](docs/plan.md).

Licensed under the Apache License 2.0; see [LICENSE](LICENSE).
