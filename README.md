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

`--depth` is the finest level named: `country`, `region`, `locality`, `macrohood`,
`neighbourhood`, `area` (a park, a campus, an airport) or `street`. Named water — a sea, a
strait, a bay — is named at every depth. The default, `auto`, takes the finest depth whose
change log has no more than `--max-rows` rows (25): a local run is named street by street,
and a flight falls back to countries and seas by itself. Auto only considers depths that
name something along the course and that the store holds the map for, and says on stderr
when it lacks some.

A summary finer than a locality names the place the whole course was in first —
`Sydney: Woolloomooloo → Darlinghurst` — chosen by `--prefix`: `city` (the most prominent
place in reach), `locality` (the locality level's own answer, which is the council where
suburb outlines answer it) or `none`. A loop run more than once is written once,
`4x (James Park → Hornsby)`, and a flight names the airports it left and reached where the
store holds them.

A course reports what its file recorded and nothing else. A GPX has no distance, so its
rows have none; a planned route has no times, so its rows have no elapsed time. Neither
is estimated.

## Where the places come from

An [osmbase](https://github.com/wisborg/osmbase) store: `osmbase boundaries` for country,
region and sea outlines, and `osmbase fetch` for the map the finer levels are read from.
A name read from OpenStreetMap data owes its credit, which `summary` prints with every
answer and writes into its JSON; see [NOTICE](NOTICE).

`course map`, to draw the course over the map, is next. The plan, and why the work is
split between this module, `osmbase` and `fitactivity`, is in
[docs/plan.md](docs/plan.md).

Licensed under the Apache License 2.0; see [LICENSE](LICENSE).
