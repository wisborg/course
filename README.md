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

Names are written in the built-in Go font, and letters it lacks -- Thai, Indian scripts,
Chinese, Arabic -- in the first of the fonts given with `--font`, then of the common system
fonts this machine has (Arial Unicode and Apple's script fonts on macOS, Noto on Linux),
that has them. Arabic and Hebrew are drawn right to left, with Arabic's letters joined.
Scripts that reorder letters within a syllable, such as Devanagari and Myanmar, need a text
shaper for their exact forms and are drawn letter by letter. `--lang en` writes names in
English wherever the map has them, and `map` says when some letters had no font at all.
The system fonts are read on this machine and never copied anywhere.

### Colouring by pace or elevation

```
./course map run.fit --colour pace
./course map ride.fit --colour elevation
```

`--colour` colours the course by a metric along it, from blue at its slowest or lowest to red
at its fastest or highest, with a colour bar in the legend giving the pace or height at each
end. The ends are set by the course itself, with the few most extreme values at each end left
out, so a sprint finish or a standstill does not squeeze the rest into one colour; an even
run or a flat course is shown over at least 5% of its speed either side, or 20 m of height,
rather than spreading the whole ramp over noise. Pace is taken over 30 m either side of each
point. Where the file has no value -- no elevation for a stretch, a gap in the recording --
the course is not coloured, and a course without times or without any elevation is refused
rather than drawn in one colour.

### References

```
./course map run.fit --reference other-run.fit --reference official-course.gpx
./course map flight.kml --great-circle
```

`--reference` draws another course beside this one for comparison -- any file `course` reads,
repeatable -- and `--great-circle` the shortest way over the globe between the course's start
and finish. Each is dashed, under the course, in a colour of its own, and a legend in the top
left says which line is which. The view holds the course and all of them.

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
