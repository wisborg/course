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
so many kilometres where the file recorded distance. A stretch the recording has no fixes
for -- a tunnel, a flight over an ocean -- is dashed, because the straight line across it is
not where the course went. The picture carries the map data's credit in its corner.

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
./course map ride.fit --colour elevation
./course map walk.fit --colour grade
./course map hike.fit --colour grade --grade-cap 30
./course map run.fit --colour heart-rate
./course map run.fit --colour power --power-source native
./course map run.fit --colour air-power
./course map run.fit --colour cadence
./course map hilly.fit --colour grade-adjusted-pace
```

The metrics are `pace`, `grade-adjusted-pace`, `elevation`, `grade`, `heart-rate`, `power`,
`air-power` and `cadence`.

`--colour` colours the course by a metric along it, from blue at its slowest or lowest to red
at its fastest or highest, with a colour bar in the legend giving the pace or height at each
end. The ends are set by the course itself, with the few most extreme values at each end left
out, so a sprint finish or a standstill does not squeeze the rest into one colour; an even
run or a flat course is shown over at least 5% of its speed either side, or 20 m of height,
rather than spreading the whole ramp over noise. Pace is taken over 30 m either side of each
point.

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

References you use often can be stored by name:

```
./course reference add "Rhodes parkrun" run.fit --alias rhodes --from 6m --to 34m
./course reference list
./course map today.fit --reference rhodes
```

A stored reference is the original file, unchanged, with a note of which part of it is the
course -- `--from`/`--to` in time, or `--from-km`/`--to-km` -- kept under `course/references`
in your configuration directory (`--references` for elsewhere).

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

The design and what was measured on real courses are in
[docs/references.md](docs/references.md).

## Fetching what the store lacks

Both commands check the store before they read it, and offer to fetch what is missing --
which tells the archive's host where the course went, so it is never done without a yes,
typed or given with `--yes`, and never asked where nobody can answer. The map fetches its
view. The summary fetches only what its depth needs: the whole of a local course at street
detail, and for a flight or a long drive just a few kilometres round each end, where the
airports are; the rest is named from country and sea outlines. Declined, both carry on
with what the store holds, and say what that cost.

The plan, and why the work is split between this module, `osmbase` and `fitactivity`, is
in [docs/plan.md](docs/plan.md).

Licensed under the Apache License 2.0; see [LICENSE](LICENSE).
