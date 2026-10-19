# statweights

Measures stat slopes for every [presetgen](../presetgen/README.md) build at each phase's BiS gear,
the source of the playerbots' per-spec stat weights. It uses wide steps, not the sim's Stat Weights
tool: that tool's ±20 lands on rotation breakpoints and misreads haste in most builds.

## Before running

- Host Python with `websockets`, and Chrome: `capture.py` drives pages through presetgen's `gen.py`,
  imported read-only.
- A sim server whose individual sim pages come from this checkout's UI. Capture only loads pages, so
  prod at `http://localhost:3333` does when its image matches.
- The toolchain image, for the runner ([dev-environment](../../docs/guide/dev-environment.md)).

## Running

From Git Bash at the repo root. The work directory goes under the gitignored `tmp/`, which the
container also sees:

```sh
python tools/statweights/capture.py --out tmp/statweights
tools/acore/dock.sh run --tags=with_db ./tools/statweights --out tmp/statweights
python tools/statweights/merge.py --out tmp/statweights
```

- All three take `--out` (required), `--builds` (comma-separated keys of `BUILDS` in presetgen's
  `gen.py`, default all 30, tanks included) and `--phases` (default `1-5`, or e.g. `1,5`).
- `capture.py` also takes `--base` (the sim server, default `http://localhost:3333`) and `--port`
  (Chrome's debugging port, default 9361). A Chrome already on the port is reused, so pick one no
  other run holds.
- The runner also takes `--iterations` (default 10000) and `--seed` (default 424242).
- 28 builds' 5 phases: capture about 45 min, the runner 40 to 75 min on 16 cores, depending on
  what else is running. Run both in the background and `tail` `capture.log` and `run.log`.
- Another seed without recapturing: copy `settings/` into a new work directory and run there. Seed
  independent runs at least `--iterations` apart ([testing](../../docs/guide/testing.md#go)).

An interrupted step just runs again with the same `--out`:
- Capture skips an existing `settings/<build>_p<N>.bin`. Delete one to recapture it, e.g. after
  presetgen rewrites its gear.
- The runner reruns a result whole when its settings file, seed or iterations changed, else only the
  slopes whose step changed.
- Merge fails on a missing result, one that no longer matches its settings file or gear preset, or
  results that mix seeds or iteration counts.

## Output

In `--out`:
- `settings/<build>_p<N>.bin`: the `ComputeStatsRequest` the build's page sent with that phase's
  gear imported.
- `results/<build>_p<N>.json`: the runner's result for one build and phase.
- `weights.json`:
  - `sim_commit`: HEAD at merge time, with `-dirty` when `sim/`, `proto/` or `assets/` has
    uncommitted changes. Merge right after the run.
  - `generated_at`, `iterations`, `seed`.
  - `builds[]`: `key`, `class` and `spec` (proto enum names), and `phases[]`:
    - `phase`;
    - `items`: 17 item ids in `ItemSlot` order (Head … Ranged), 0 for an empty slot;
    - base `dps`, `tps` and `tmi`;
    - `slopes`, keyed by `Stat` or `PseudoStat` enum name: `{delta, shift, dps, tps, tmi}`, per point.
- `capture.log`, `run.log`, `chrome-profile/`, `settings.tmp.json`.

## Method

- **Settings** come from the page, because an exported settings JSON keeps the rotation as
  `TypeAuto` without the APL the page resolves (it sims Fire Mage at 0 DPS). UI defaults live only
  in TS, so there is no browser-free path.
  - Each build's page opens as presetgen opens it: cleared storage, the tree's talent preset, an
    Alliance race, all professions, party Heroic Presence, the build's pet talent preset, the Playerbot
    switch on ([playerbot rules](../../docs/guide/playerbot-rules.md)). Other buffs,
    debuffs, consumes, encounter and rotation stay the page's defaults.
  - Per phase, capture imports the gear from `ui/<dir>/gear_sets/p<N>_bis[_<tree>].gear.json` and
    the racial traits from that directory's `bis_presets.json`. It saves the last `computeStats`
    request the page posts to its worker, once none has followed for 2 s.
- **Slopes:** `(sim(base+shift+δ) − sim(base+shift−δ)) / 2δ` through `Player.BonusStats`, both
  ends at the same seed with `IsTest`. The stats per role (melee, hunter, caster, tank, from the
  player's spec) are in `steps.go`.

  | Stat | δ | Shift |
  |---|---|---|
  | Haste | 300 | 0 |
  | AP, RAP, SP | 200 | 0 |
  | Expertise | 30 | −60 |
  | Melee hit | 100 | −200 |
  | Spell hit | 100 | −250 |
  | ArP | 100 | `max(−100, 100 − base ArP)` |
  | Armor | 1000 | 0 |
  | Weapon DPS (pseudo stats) | 10 | 0 |
  | Any other stat | 100 | 0 |

  - Hit and expertise are measured below the cap. ArP clamps at 0%, so its range starts at or
    above 0 rating; below that, low-ArP gear reads 0.
  - Haste steps ±300 because ±100 zig-zags between phases on real haste thresholds. The wider step
    averages across the ones a bot's non-BiS gear lands around.
