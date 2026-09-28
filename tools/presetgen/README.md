# presetgen

Generates the phase 1-5 BiS gear presets in `bis-optimizer.PLAN.md`'s BIS-presets section: drives the
individual sim's BiS Optimizer tab in headless Chrome, one talent tree "build" at a time, like the
[acbis driver](../database/acbis/driver/README.md) drives the raid sim's batch tab.

## Before running

A sim server of its own, from the toolchain image over this checkout (not prod's port, not another
WI's dev server). Pass `SIM_COMMIT`, or results say sim `unknown`. Host Python with `websockets` and
Chrome at its default install path (same prerequisites as [uicheck](../uicheck/README.md)).

## Running

From Git Bash, with `SCRATCH` outside the repo:

```sh
python tools/presetgen/gen.py run --out "$SCRATCH/presets" --effort Normal
python tools/presetgen/gen.py run --out "$SCRATCH/check" --effort Normal --builds warrior_fury,tank_dk_blood --phases 1,5 --no-write
```

- `--builds`: comma-separated build keys from `BUILDS` in `gen.py` (`dk_blood`, `hunter_mm`, ...), or
  `all` (default). One build is one talent tree: a tree with several named gear presets today (Frost
  DK, Fire Mage, ...) gets its own key; a spec with one tree today is the spec's own key.
- `--phases`: e.g. `1-5` or `1,5` (default `1-5`). Phase 1 seeds from the tree's Classic P1 gear file
  (`BUILDS`' seed, so it stays in the repo); a later phase from the previous phase's pick at the same
  effort in `summary.jsonl`, else from that phase's gear file.
- `--effort`: `Quick` or `Normal` (default `Quick`). Either writes the files below.
- `--no-write`: a spot check that writes only the summary.
- `--base`, `--port`: the sim server and Chrome's debugging port (defaults `http://localhost:3337`,
  `9348`). A Chrome already on the port is reused; one the run started is stopped at the end.
- A rerun skips a `(build, phase, effort)` already in `--out/summary.jsonl`, except that a writing run
  redoes one a `--no-write` run settled: an interrupted run just runs again with the same `--out`.

Each build starts from a fresh page: cleared `localStorage`, a reload, the tree's talent preset (so
talents and glyphs come from the app's own registered preset, never hand-typed), an Alliance race
(the spec's default race if that's already Alliance, else the first Alliance race the class allows),
and all 11 professions (the pool builder gates profession-locked items, enchants and gems on the
player's own professions, so a real 2-profession character would hide options a raider with any
profession could get gear for). Buffs, debuffs, consumes, sources and the racial search stay the
spec's own defaults; a tank's boss and survival/threat slider are the tab's own per-phase defaults.

## Output

In `--out`:
- `summary.jsonl`: one line per settled `(build, phase, effort)`: score Δ ± se over the seed, the
  racial traits picked, sim commit, catalog date, sims, wall time and the pick's items. This is also
  the resume log.
- `driver.log`: everything the driver prints.
- `chrome-profile/`: the headless Chrome profile (safe to delete between runs; nothing persists a
  build across it since every build clears its own storage).

Without `--no-write`, each phase also writes into `ui/<spec>/gear_sets/`:
- `p<phase>_bis[_<tree>].gear.json`: the pick, named in its summary row's `gearFile`.
- that file's entry in `bis_presets.json`: the chip label, build, phase, effort, racial traits, sim
  commit and catalog date. `presets.ts` builds each preset's label and tooltip from it
  (`PresetUtils.makeBisPresetGear`), so a rerun restamps them with no hand edits. Other builds' entries
  stay.

`presets.ts` registers each file with its talent-tree gating, and `sim.ts` lists them in `presets.gear`,
with the default tree's P5 as `defaults.gear`.

`make`'s bundle rule doesn't watch `.json` files, so a dev server only shows a run's files after
`tools/acore/dock.sh exec sh -c 'rm -f dist/wotlk/bundle/.dirstamp && make dist/wotlk/.dirstamp'`.

## Mechanism

`page.js`, run through `tools/uicheck/cdp.py eval` on a spec's page (e.g. `/wotlk/deathknight/`),
installs `window.__pg`:

| Helper | Does |
|---|---|
| `clickPreset(tabId, name)` | clicks a named preset chip on the Talents or Gear tab |
| `exportSettings()` / `importSettings(json)` | the header's JSON export/import modal, for the full `IndividualSimSettings` in one round trip |
| `setupOptimizer(phase, effort)` | sets the optimizer tab's content phase and effort selects |
| `runOptimizer(timeoutMs)` | clicks Optimize and waits for it to settle |
| `exportResult(phase)` | clicks the result's Export JSON button and returns the download |

`gen.py` exports the page's default settings once per build (after clicking the tree's talent
preset), then for each phase sets `player.race`, `player.professions` and `player.equipment.items`
(the phase's seed) in that JSON and reimports it, so buffs, debuffs, consumes and encounter settings
always come from the page's own defaults, untouched. It reads the optimizer's result the same way the
tab's own "Export JSON" button would.

Both modals' Bootstrap-animated close sometimes never finishes on a scripted click (`hide.bs.modal`
not settling), so `page.js` drops the modal DOM directly instead of waiting on it.

A rerun on the same optimizer tab leaves the previous result, and its Export JSON button, on screen
until the new one settles, so `runOptimizer` reads completion off the Cancel button's visibility
(shown while running), not off that button's presence.

## Timing

At Quick, one phase runs 20-50 s wall time for most specs against a lightly loaded dev server, up to
~200 s for a tank (rarely longer: one Quick tank phase took 74 s at a 144 s budget and needed a
resumed run at a larger `--quick-timeout`). A full pass over all 28 builds' 5 phases took about
80 min. A `Normal` phase runs 45-115 s for a DPS spec and 90-335 s for a tank (Blood DK the slowest),
so a full `Normal` pass should take about 4-5 h.

## Rerun on the merged tree

Once PAR-P8 and BIS-tank-boss merge (both change what the optimizer picks), the orchestrator reruns
every build at Normal on `integration`, rebuilds the bundle as above, and commits what it wrote:

```sh
python tools/presetgen/gen.py run --out "$SCRATCH/presets-final" --effort Normal
```
