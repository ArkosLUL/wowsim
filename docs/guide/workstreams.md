# Workstreams

The registry of efforts: what each one owns, and how they depend on each other. Status lives in each
effort's PLAN.

The efforts marked **loop** run as work items in the [wave loop](../wave-loop/wave-loop.PLAN.md). Each item
gets its own `<effort>-<id>` branch and worktree off `integration` ([int]).

| Effort | Scope | Branch, worktree | Docs | Owns |
|---|---|---|---|---|
| wave-loop | Orchestrates the loop efforts in waves | `integration`; [int] | [docs/wave-loop/](../wave-loop/) | `docs/wave-loop/`; git and integration for the loop efforts |
| docker-build | Build and serve the sim from Docker | `master` | README "Docker" section | `Dockerfile`, `.dockerignore` |
| azerothcore-item-diff (loop) | Diff the sim's item data and Go effects against the server; `gen_db` AzerothCore mode | loop | [docs/azerothcore-item-diff/](../azerothcore-item-diff/) | `tools/database/azerothcore/` (shared readers), `tools/database/acdiff/`, `tools/database/gen_db/` AzerothCore mode |
| azerothcore-raid-import (loop) | Export the raid group from the server and import it into the sim: reforges, racial traits, all professions | loop | [docs/azerothcore-raid-import/](../azerothcore-raid-import/) | `tools/database/acraid/`, the roster files in `tools/database/azerothcore/`, `ui/raid/acore_*.ts` |
| azerothcore-parity (loop) | Server-exact mechanics, generated constants, server settings, validation | loop | [docs/azerothcore-parity/](../azerothcore-parity/) | `tools/acore/`, `tools/simval/`, `sim/core` combat mechanics (tables, outcomes, base stats), `sim/core/serverdata/`, `assets/db_inputs/acore/`, `[ac]/modules/mod-sim-validation` (own repo) |
| bis-optimizer (loop) | Best-in-slot items, gems, enchants, reforges and racial traits per phase, for a character or a raid | loop | [docs/bis-optimizer/](../bis-optimizer/) | `sim/optimizer/`, `proto/optimizer.proto`, `cmd/wowsimcli/cmd/optimize.go`, `tools/database/accatalog/`, `tools/database/azerothcore/catalog*.go`, `assets/database/server_catalog.json`, `ui/core/optimizer/`, `optimizer_tab.ts`, `ui/raid/optimizer_*.ts`, `sim/core/sheet.go` |
| bis-tooltip-addon | Serve the BiS optimizer's results to an in-game tooltip addon through a server module | `bis-tooltip-addon`; [sim] | [docs/bis-tooltip-addon/](../bis-tooltip-addon/) | `tools/database/acbis/`, `tools/database/azerothcore/bisdata*.go`, `[ac]/modules/mod-bis-tooltip` (own repo), the BisTooltipAC addon (own repo) |
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
- **Shared files:** bis-optimizer edits `proto/api.proto`, `sim/core/database.go`, `sim/web/main.go`,
  `sim/wasm/main.go`, `ui/worker/*`, `ui/core/worker_pool.ts`, `ui/core/sim.ts`, `gear_picker.tsx`,
  `player.ts`, `individual_sim_ui.ts`, `gear_tab.ts` and `saved_data_manager.ts`.
  Parity P6-1 also edits `database.go` and `gear_picker.tsx`.
- **Presets:** BiS presets take over parity P6's re-pointing of presets.
- **Re-baseline:** BiS results follow the sim, so they're re-baselined after every parity wave from D on.
