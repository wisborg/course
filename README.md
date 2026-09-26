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
`neighbourhood` or `street`. Named water — a sea, a strait, a bay — is named at every depth.
The default, `auto`, takes the finest depth whose change log fits `--budget` rows (25): a
local run is named street by street, and a flight falls back to countries and seas by
itself. Auto only considers depths the store holds the map for along the course, and says
on stderr when it lacks some.

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
