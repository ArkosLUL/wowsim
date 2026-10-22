# rankings

Sims every DPS build at each content phase's BiS gear under one standard scenario, for the DPS
rankings page. Builds and presets come from [presetgen](../presetgen/README.md); settings capture and
`stale` checks are [statweights](../statweights/README.md)'s, imported read-only.

## Running

From Git Bash at the repo root, after statweights' "Before running". The work directory goes under the
gitignored `tmp/`:

```sh
python tools/statweights/capture.py --out tmp/rankings
tools/acore/dock.sh run --tags=with_db ./tools/rankings --out tmp/rankings
python tools/rankings/merge.py --out tmp/rankings
```

- Capture also covers the tank builds (the runner skips them); a `settings/` copied from a statweights work
  directory works too. Capturing all 30 builds takes about 50 min, the runner about 5.
- The runner takes `--out` (required), `--builds`, `--phases` (as statweights), `--fights` (seconds, default
  `60,180,300`), `--iterations` (10000) and `--seed` (424242).
- It reruns a result only when its settings file, the scenario, the sim build (its binary's hash), the seed,
  the iterations or the fights changed.
- Merge fails on a missing or stale result, or on results that mix seeds, iterations, fights or sim builds.
  It writes `assets/rankings/rankings.json` (`--dest` for elsewhere), so run it right after the runner.
- **Rerun** after every BiS preset re-baseline and after any sim change that moves DPS, then commit the JSON.
  After a re-baseline, capture into a fresh work directory: capture skips existing files, and the stale
  check compares item ids only, missing a changed gem, enchant or reforge.

## Standard scenario

Applied to each captured request (`scenario.go`), so builds compare as playerbots play them:
- **Kept from the page:** talents, glyphs, spec options and rotation.
- **Playerbot switch** on: the bots' potion, flask and food, and their pre-pull
  ([playerbot rules](../../docs/guide/playerbot-rules.md)).
- **Buffs:** explicit full raid buffs, boss debuffs, blessings, Mana Tide and Heroic Presence in every
  party. No single-target gifts (Focus Magic, Tricks, Power Infusion, Innervate, Hysteria). The lists
  are copies of `core.Full*` in `sim/core/test_utils.go`, which the goldens own.
- **Tanks:** none.
- **Boss:** one level 83 Giant, armor 10643, AP 805, as the optimizer's `plainBossTarget`; execute
  20/25/35%; length 60, 180 or 300 s ±5 s. The captured raid difficulty (unset on every page, so
  25-man normal) and server settings stay.

## Output

In `--out`:
- `settings/<build>_p<N>.bin`: from capture.
- `results/<build>_p<N>.json`: `build`, `phase`, `class`, `spec`, `talents`, `items` (ItemSlot order),
  `settings_sha256`, `scenario_sha256`, `sim_sha256`, `seed`, `iterations`, and:
  - `fights`: by length, the player's DPS `{dps, stdev, min, max}`. Pets are in it, stdev included.
  - `link`: base64 of the zlib-deflated `IndividualSimSettings` at 180 s, without the item database, the
    format of an individual sim's share link. Opened in the spec's sim page it loads the simmed setup.
    Its `settings` (phase, iterations, damage metrics shown) apply only when the URL's `?i=` lists the UI
    settings category.
- `run.log`, `capture.log`, `chrome-profile/`, `settings.tmp.json`.

`assets/rankings/rankings.json`:
- `sim_commit` (HEAD, `-dirty` when `sim/`, `proto/` or `assets/` outside `assets/rankings/` changed),
  `generated_at`, `catalog_date` (one value across the BiS presets), `iterations`, `seed`, `fights`.
- `builds[]`: `key`, `name`, `class`, `spec`, `dir`, `talents`, and `phases[]` of `{phase, fights, link}`.

## Balance check

`balance_check.py` checks a `rankings.json` against [spec-balance](../../docs/spec-balance/spec-balance.PLAN.md)'s
targets and prints every build's % of top and tier per phase and fight, as the page shows them.

```sh
python tools/rankings/balance_check.py [--rankings FILE] [--base FILE] [--buffed key,key]
python -B -m unittest tools/rankings/balance_check_test.py
```

`--rankings` defaults to the committed JSON; `--base` is a run from before the buffs; `--buffed` lists every
build a server buff changed, spillover included. It checks the counted cells (P1–P5 at 180 and 300 s) and
exits 1 on any of:
- a missing result, or an in-scope build below 85% of the cell's top (the excluded builds are `EXCLUDED`);
- a `--buffed` build at 95% or more, unless `--base` had it there already and it rose 0.3 points at most; a
  build that held the cell's top counts its rise against its own old DPS;
- a `--buffed` build taking a cell's top from another build (needs `--base`);
- an excluded build at 95% or more.

Size a buff against presets re-optimized with it in: one that changes a stat's worth (armor ignore) makes the
optimizer regem, and two Normal optimizer runs can move a cell by up to 3 points.
