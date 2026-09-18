# Workstreams

The registry of efforts: what each one owns, and how they depend on each other. Status lives in each
effort's PLAN.

| Effort | Scope | Branch, worktree | Docs | Owns |
|---|---|---|---|---|
| docker-build | Build and serve the sim from Docker | `master` | README "Docker" section | `Dockerfile`, `.dockerignore` |
| azerothcore-item-diff | Diff the sim's item data and Go effects against the server; next, a `gen_db` AzerothCore mode | `azerothcore-item-diff` | [docs/azerothcore-item-diff/](../azerothcore-item-diff/) | `tools/database/azerothcore/` (shared readers), `tools/database/acdiff/` |
| azerothcore-raid-import | Export the raid group from the server and import it into the sim: reforges, racial traits, all professions | `azerothcore-raid-import`, `azerothcore-roster-export`; [sim] | [docs/azerothcore-raid-import/](../azerothcore-raid-import/) | `tools/database/acraid/`, the roster files in `tools/database/azerothcore/`, `ui/raid/acore_*.ts` |
| azerothcore-parity | Server-exact mechanics, generated constants, server settings, validation | `azerothcore-parity`; [par] | `docs/azerothcore-parity/` (on its branch) | `tools/acore/`, `tools/simval/`, `sim/core` combat mechanics (tables, outcomes, base stats), `[ac]/modules/mod-sim-validation` (own repo) |
| dev-guide | This guide's structure | `master` | `docs/dev-guide/` | `CLAUDE.md`, `CONTEXT.md`, `docs/guide/`, `docs/adr/`. Every effort adds content |

## Dependencies

- azerothcore-parity branched from `176e93f3b`, so it lacks master's reforge, racial-traits and
  professions commits. Merge `master` into it before P5 and P6, which touch the same models.
  - Master's models win. P5/P6 extend them (e.g. reforge % and stat list as server settings) rather
    than rebuild them.
  - The parity PLAN still describes building them, with the racial picker in `other_inputs.ts`. Update
    it at the merge.
- Parity P6 (server items) needs item-diff's `gen_db` AzerothCore mode
  ([ADR 0002](../adr/0002-item-data-from-live-db.md)).
- Raid-import Phase 3 (the UI importers) reads acraid's roster JSON v1.
- [par] only sees this guide once `master` is merged into `azerothcore-parity`.
