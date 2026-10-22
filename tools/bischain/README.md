# bischain

Runs the BiS chain in one command: sim servers, the BiS presets ([presetgen](../presetgen/README.md)), the raid
roster, the BiS tooltip dataset ([acbis driver](../database/acbis/driver/README.md)), then
[stat weights](../statweights/README.md) and the [DPS rankings](../rankings/README.md). It drives those tools as
they are; their READMEs own how each works.

## Before running

- The user's go: the chain loads every core for most of a day.
- `ac-database` up (SELECT only: the roster and the live group). The worldserver can stay off; with it on, ask
  the user to run `saveall` first.
- A clean checkout at the commit to stamp. Ports other sessions use go in `RESERVED` in `chain.py`; the chain
  also skips any port something listens on.
- The switches file for raiders whose build changes by phase, outside the repo:
  `G:\DevStuff\GitHub\.wave-loop\bis-chain-switches.json` (Deathsong: Morbidity dual-wield through phase 2,
  Scourge Strike 2H from phase 3, the user's call 2026-10-17). Each entry names the raider, phases, talents,
  glyph swaps, gear by AC slot, slots to empty and raid item filters. The page's filters are raid-wide, so each
  switch runs alone as its own batch on its own server, and its entries replace the main batch's.

## Running

From Git Bash at the repo root, with a fresh `--work` per pass:

```sh
python tools/bischain/chain.py all --work G:/DevStuff/GitHub/.wave-loop/bis-chain-<date> \
  --times 'G:/DevStuff/GitHub/.wave-loop/bis-chain-*/presets/*/summary.jsonl' \
  --times 'G:/DevStuff/GitHub/.wave-loop/bis-chain-1021/presets-*/summary.jsonl' \
  --switches G:/DevStuff/GitHub/.wave-loop/bis-chain-switches.json
```

`all` starts the servers (named `wotlk-chain-<work name>-<role>`) and exports the roster, which must match
Deathsong's live parties (`--allow-party-mismatch` overrides). Then two branches run side by side:
- **Presets**, then stat weights, rankings and the stat values ([publish.py](../statweights/README.md#running)),
  last since the rankings merge would read a new file under `assets/` as a dirty sim. Each ui folder goes
  whole to the next free server, slowest first by `--times`, with its own `presets/<folder>/` output and a
  1200 s Normal timeout. **Stop 1:** once written, the presets wait for the orchestrator to check and commit them (`ui/` clean), so stat weights and
  rankings stamp a clean commit.
- **Dataset**: one batch per `--dataset-split` group (default `1,2,3:4,5`) on its own server, both stages,
  plus the switches; a pass missing any job's entry reruns its failed jobs once, then fails. Then the merge,
  `check_sources.py` and acbis. **Stop 2:** importing replaces the live dataset and needs the user's OK; the
  log prints the command.

A rerun with the same `--work` resumes: it reuses the first run's options (`args.json`), the roster and
anything settled, and skips a finished `assemble` or `weights` (delete `assemble.done` or `weights.done` to redo
one). Single steps: `servers`, `presets`, `roster` (exports a new roster), `dataset`, `assemble`, `weights`,
`stop` (removes this run's containers and ends its Chromes). Errors land in `logs/errors.log`.

Rerun only what a change touches: `--builds` and `--phases` narrow presets, stat weights and rankings;
`--dataset-raiders` the dataset. A narrowed run needs `--from-work` naming an earlier full run, since
`weights.json`, `rankings.json` and the live dataset cover everyone: it starts from that run's captures,
results and merged batch. Without it, stat weights cover the chosen builds alone, the rankings and stat values
are skipped and `assemble` refuses. The 2026-10-21 run is laid out for it as `bis-chain-1021`.
Both dataset stages stay at Normal (the user's call, 2026-10-21).
`--no-write`, `--dataset-effort Quick` and `--rankings-dest` are for test runs.

## Output

- In `--work`: `chain.log`, `logs/`, `args.json`, `servers.json`, `raid.json` and each switch's copy,
  `presets/<ui folder>/`, `bis/<batch>/`, `switch/<raider>/`.
- In the checkout's `tmp/`: `<work name>/bis_dataset.sql` (and `.lua`, `batch.json`),
  `statweights-<work name>/weights.json` (for playerbots' `generate_weights.py`), `rankings-<work name>/`.
- `ui/*/gear_sets/`, `assets/rankings/rankings.json` and `assets/statweights/stat_values.json`, to commit.
  With `--rankings-dest`, the stat values go to `tmp/statweights-<work name>/stat_values.json` instead.

## Timing

The 2026-10-21 run on the 9950X3D (16 cores, 32 threads), all builds and phases, by hand with three to four
servers at once: presets 4 h, dataset stage 1 4.1 h alongside, stage 2 3.1 h in one batch (Fel's raid-DPS job,
28-41 min a phase, on about 31 threads), stat weights and rankings 2 h alongside stage 2. The CPU stays near
full throughout, so more servers trade speed per run for runs at once; the defaults start five (two presets,
two dataset batches, one switch).
