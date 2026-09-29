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

### References

```
./course map run.fit --reference other-run.fit --reference official-course.gpx
./course map flight.kml --great-circle
```

`--reference` draws another course beside this one for comparison -- any file `course` reads,
repeatable -- and `--great-circle` the shortest way over the globe between the course's start
and finish. Each is dashed, under the course, in a colour of its own, and a legend in the top
left says which line is which. The view holds the course and all of them. Storing references
by name, and matching activities against them, are planned in
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
