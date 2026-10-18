# Workstreams

The registry of efforts: what each one owns, and how they depend on each other. Status lives in each
effort's PLAN.

The efforts marked **loop** run as work items in the [wave loop](../wave-loop/wave-loop.PLAN.md). Each item
gets its own `<effort>-<id>` branch and worktree off `integration` ([int]).

| Effort | Scope | Branch, worktree | Docs | Owns |
|---|---|---|---|---|
| wave-loop | Orchestrates the loop efforts in waves | `integration`; [int] | [docs/wave-loop/](../wave-loop/) | `docs/wave-loop/`, `tools/uicheck/`; git and integration for the loop efforts |
| docker-build | Build and serve the sim from Docker | `master` | README "Docker" section | `Dockerfile`, `.dockerignore` |
| azerothcore-item-diff (loop) | Diff the sim's item data and Go effects against the server; `gen_db` AzerothCore mode | loop | [docs/azerothcore-item-diff/](../azerothcore-item-diff/) | `tools/database/azerothcore/` (shared readers), `tools/database/acdiff/`, `tools/database/gen_db/` AzerothCore mode |
| azerothcore-raid-import (loop) | Export the raid group from the server and import it into the sim: reforges, racial traits, all professions | loop | [docs/azerothcore-raid-import/](../azerothcore-raid-import/) | `tools/database/acraid/`, the roster files in `tools/database/azerothcore/`, `ui/raid/acore_*.ts`, `sim/web/acroster/`, `ui/core/talents/trees/embed.go`, `tools/uitest/tests/raid/acore_*.spec.ts` |
| azerothcore-parity (loop) | Server-exact mechanics, generated constants, server settings, validation | loop | [docs/azerothcore-parity/](../azerothcore-parity/) | `tools/acore/` (except `dock.sh`'s golden commands), `sim/core` combat mechanics (tables, outcomes, base stats), `sim/core/serverdata/`, `assets/db_inputs/acore/` |
| test-enrichment (loop) | Richer goldens and live e2e: ability results, rotation and talent guards, damage probe, replay fixtures | loop | [docs/test-enrichment/](../test-enrichment/) | `sim/core/test_suite.go`, `sim/core/test_generators.go`, `proto/test.proto`, `dock.sh`'s golden commands (`dps`, `delta`, `promote`) |
| e2e-coverage (loop) | More live checks: timing and costs, spell-mod and buff sweeps, threat, weapon strikes, pets, gems and enchants, fights, creature types | loop | [docs/e2e-coverage/](../e2e-coverage/) | `tools/simval/`, `tools/apldbg/`, `sim/core/testdata/simval/`, `sim/core/testdata/fights/`, `[ac]/modules/mod-sim-validation` (own repo); its items' carve-outs (the PLAN) |
| bis-optimizer (loop) | Best-in-slot items, gems, enchants, reforges and racial traits per phase, for a character or a raid | loop | [docs/bis-optimizer/](../bis-optimizer/) | `sim/optimizer/`, `proto/optimizer.proto`, `cmd/wowsimcli/cmd/optimize.go`, `tools/database/accatalog/`, `tools/database/azerothcore/catalog*.go`, `assets/database/server_catalog.json`, `ui/core/optimizer/`, `optimizer_tab.ts`, `ui/raid/optimizer_*.ts`, `sim/core/sheet.go`, `sim/encounters/optimizer_tank_boss.go`, `tools/presetgen/` and the `p*_bis*` gear presets it writes |
| sim-performance (loop) | Use the whole machine, find the sim's and the optimizer's bottlenecks, and keep tooling that finds them | loop | [docs/sim-performance/](../sim-performance/) | `tools/perf/`; its items' carve-outs in `sim/core`, `sim/optimizer`, `sim/web` and `cmd/wowsimcli` (the PLAN) |
| ui-tests (loop) | Playwright regression tests for the sim UI, and the bugs they find | loop | [docs/ui-tests/](../ui-tests/) | `tools/uitest/`; per wave, the UI paths its PLAN assigns |
| bis-tooltip-addon | Serve the BiS optimizer's results to an in-game tooltip addon through a server module | `bis-tooltip-addon`; [sim] | [docs/bis-tooltip-addon/](../bis-tooltip-addon/) | `tools/database/acbis/`, `tools/database/azerothcore/bisdata*.go`, `[ac]/modules/mod-bis-tooltip` (own repo), the BisTooltipAC addon (own repo) |
| stat-weights | Wide-step stat slopes per build and phase at the BiS presets, for the playerbots' stat weights | `stat-weights`; [sim] | [statweights README](../../tools/statweights/README.md) | `tools/statweights/` |
| dps-rankings | DPS tier list page: every DPS build's DPS per content phase in one standard scenario | `dps-rankings`; `wowsimwotlk-rankings` | [docs/dps-rankings/](../dps-rankings/) | `tools/rankings/`, `ui/rankings/`, `ui/scss/sims/rankings/`, `assets/rankings/`, `tools/uitest/tests/rankings/` |
| dev-guide | This guide's structure | `master` | `docs/dev-guide/` | `CLAUDE.md`, `CONTEXT.md`, `docs/guide/`, `docs/adr/`. Every effort adds content |

## Dependencies

Work-item order is set in the [wave registry](../wave-loop/wave-loop.PLAN.md#wave-registry). These are
the cross-effort ties:

- **Models:** master's reforge, racial-traits and professions models win. Parity P5/P6 extend them (e.g.
  reforge % and stat list as server settings) rather than rebuild them.
- **Parity P6** (server items) needs item-diff's AC-1 ([ADR 0002](../adr/0002-item-data-from-live-db.md)).
- **Raid-import Phase 3** (RI-3) reads acraid's roster JSON v1. The BiS raid batch uses its importer.
- **BiS tooltip:** `acbis` reads acraid's roster JSON v1 and the optimizer's `OptimizerResult` and
  `OptimizerBatchExport`, so a change to either format needs `acbis` updated.
- **Stat weights:** `tools/statweights` imports presetgen's `gen.py` and reads the `p*_bis*` presets and
  `bis_presets.json`. Playerbots' `apps/simweights/generate_weights.py` reads its `weights.json` and
  maps or skips every `BUILDS` key (an unknown one fails the run), so a change to the `gen.py` API, the
  `BUILDS` keys or that layout needs the other side updated.
- **Shared files:** bis-optimizer edits `proto/api.proto`, `sim/core/database.go`, `sim/web/main.go`,
  `sim/wasm/main.go`, `ui/worker/*`, `ui/core/worker_pool.ts`, `ui/core/sim.ts`, `gear_picker.tsx`,
  `player.ts`, `individual_sim_ui.ts`, `gear_tab.ts` and `saved_data_manager.ts`.
  Parity P6-1 also edits `database.go` and `gear_picker.tsx`.
- **Presets:** BiS presets take over parity P6's re-pointing of presets.
- **Golden report:** `dock.sh delta`'s output feeds the wave-loop report schema (`wave-loop.workflow.js`), so a
  change to either needs the other updated.
- **Re-baseline:** BiS results follow the sim, so they're re-baselined after every parity wave from D on.
