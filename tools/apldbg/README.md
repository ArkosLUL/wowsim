# apldbg

Runs one character on a rotation and prints what it did, for tuning an APL: a golden suite's rows, the casts,
damage and aura uptimes behind one row, and one iteration's log or timeline.

## Run

From Git Bash at the checkout's root:

```sh
tools/acore/dock.sh run --tags=with_db ./tools/apldbg -spec frostuh -rows
tools/acore/dock.sh run --tags=with_db ./tools/apldbg -spec frostuh -apl ui/deathknight/apls/frost_uh_pesti.apl.json \
  -row orc,nobuffs,multi -iters 500 -ids 45529,51271 -timeline 0:30 -match 'rune from'
tools/acore/dock.sh run --tags=with_db ./tools/apldbg -request tools/perf/harness/scenarios/sim_rogue.json.gz -log tmp/rogue.log
```

| Flag | Meaning |
|---|---|
| `-spec` | a golden suite's character: `blood`, `frost`, `frostuh`, `unholy` (the `sim/deathknight/dps` suites) |
| `-request` | in place of `-spec`: a `RaidSimRequest` or the UI's exported settings as JSON, gzipped or not; its first player, one row |
| `-apl` | an `.apl.json` run in place of the character's own rotations |
| `-gear` | with `-spec`: another gear set of the spec's `gear_sets`, without `.gear.json` |
| `-rows` | print every row's DPS and stop |
| `-row` | the row to look into, as comma-separated parts of its label, any case (default `Average`) |
| `-iters`, `-seed` | in place of the row's own: 20 a Settings row, 2000 Average, 1000 a request without options; seed 101 |
| `-ids` | print only actions and auras whose id holds one of these parts, e.g. `45529,OtherID: 3` |
| `-log` | write one debug iteration's log, same seed |
| `-timeline from:to` | print that iteration's casts (no swings), idle decisions and the character's own `-ids` auras going on and off between two times in seconds; `:` for the whole fight |
| `-match` | with `-timeline`: also the character's log lines matching this regexp |

A `-spec` row is the suite's own request (`core.FullCharacterTestSuiteGenerator`), so without `-apl`, `-gear`,
`-iters` or `-seed` its DPS is the golden's to the digit. The report gives casts, landed hits, crits and damage per
iteration, aura uptime in seconds, and `WARN` lines for spells the rotation names but the character doesn't know.

## Against a base

To tell a rotation change from a model change, build the tool on a commit's sim code too. On the host:

```sh
tools/apldbg/base.sh <commit> [path ...]       # default path: sim
tools/acore/dock.sh exec tmp/apldbg/base -spec frostuh -rows
tools/acore/dock.sh exec tmp/apldbg/new -spec frostuh -rows
```

`base` overlays that commit's copy of every non-test Go file under the paths that differs from the working tree
(`go build -overlay`); `new` is the working tree. A commit whose protos differ won't build: the generated code isn't
tracked.

## Presets

A suite's config lives in its `_test.go`, which nothing can import, so `presets.go` copies it.
`TestPresetsRunTheirSuitesRows` holds each copy to its `.results`, so it also fails while that suite's golden
is unpromoted. When it fails alone, copy the suite's gear, talents, glyphs, options, races and rotations again. A
new spec is a new `presets` entry naming its golden.
