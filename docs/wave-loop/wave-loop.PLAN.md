# Wave loop: BiS optimizer, parity, item DB and raid import in one orchestrated program

## Context

The user wants a Best-in-Slot generator for the server ([bis-optimizer](../bis-optimizer/bis-optimizer.PLAN.md)).
BiS is only as good as the sim's parity with the server, so four efforts run together, in parallel work items
(WIs), from one orchestrator session:
- the remaining parity phases
- item-diff's AzerothCore item-DB mode
- raid-import Phase 3
- BiS

Each wave:
- An implementer agent and then a separate reviewer agent handle every WI, each WI on its own branch and
  worktree.
- The orchestrator merges the WIs into `integration`.
- `integration` fast-forwards `master`.

The procedure is in the [RUNBOOK](wave-loop.RUNBOOK.md).

## Decisions (settled with the user)

**Coordination**
- One loop and one integration branch for all four efforts. Parity is loop-driven.
- The orchestrator stops after every wave with a report and waits for "continue".

**Goldens and load**
- Golden-changing WIs are developed in parallel and promoted one at a time at integration.
- At most 4 WIs per wave, and at most 2 full-suite golden changers. P7 class WIs run only their own
  class's suites.
- BiS WIs change no goldens. BiS presets are new files, because the tests load
  `ui/<spec>/gear_sets/*.gear.json`.

**Specific WIs**
- P4 was committed unreviewed, so wave A reviews it (PAR-P4R).
- Recorded runs: playerbots fight a dummy in an instance, logged by Chronicle. A mismatch with the sim is a
  finding for that class's WI, not a blocker.

## Wave registry

G = changes goldens. FS = runs all 37 suites.

| Wave | WIs | BiS re-baseline |
|---|---|---|
| A | PAR-P4R (review-only; G, FS) · BIS-contract · PAR-P5-1 · AC-1 | |
| B | AC-2 (G, FS, 34 suites) · PAR-P3-1 · BIS-catalog · BIS-eval | |
| C | PAR-P6-2 (G, FS, 14 suites) · PAR-P5-23 · BIS-rules (with catalog changes) · BIS-raidctx | |
| D | PAR-P3-2 (G, FS) · PAR-P3-3 (G, FS) · BIS-search · BIS-ui-tab | ✔ |
| E | PAR-P3-4 (G, FS) · PAR-P3-5 (G, FS) · PAR-P6-1 · RI-3 | ✔ |
| F | PAR-P7-0a (G, FS) · PAR-P7-0b (G, FS) · BIS-tanks-racials · PAR-TOOLS-RR | ✔ |
| F2 | PAR-P7-0c (G, FS) · BIS-picker-switch | ✔ |
| G | PAR-P7-DK (G) · PAR-P7-HUN (G) · BIS-batch-ui | ✔ |
| H | PAR-P7-ROG · PAR-P7-WAR · PAR-P7-RET (G) · BIS-raid-contrib | ✔ |
| I | PAR-P7-SHA · PAR-P7-DRU · PAR-P7-MAG · PAR-P7-WLK (G) | ✔ |
| J | PAR-P7-PRI (G) · PAR-P7-TANK (G) · BIS-e2e-perf · AC-3 | ✔ |
| K | BIS-presets · PAR-P8 (G, FS) · BIS-tank-boss | ✔ |

F2 is an inserted wave, not a fifth item in F: the core swing and cast fixes of PAR-P7-0c have to land
before the class items in G, a class item can't build on a core fix merging in its own wave, and F already
carries the 2 full-suite golden changers a wave is allowed. BIS-picker-switch moved in from G to give it a
second, light item.

**Where the specs are:**

| WI prefix | Spec |
|---|---|
| `PAR-` | [parity PLAN, "Loop work items"](../azerothcore-parity/azerothcore-parity.PLAN.md) |
| `AC-` | [item-diff PLAN, "Loop work items"](../azerothcore-item-diff/azerothcore-item-diff.PLAN.md) |
| `RI-3` | [raid-import PLAN, Phase 3](../azerothcore-raid-import/azerothcore-raid-import.PLAN.md) |
| `BIS-` | [bis-optimizer PLAN, "Work items"](../bis-optimizer/bis-optimizer.PLAN.md) |

## Current wave

- Wave: E, running. Wave D (base `30137e6d1`) landed on `master`.
- Base SHA: `328553aa9`.
- Workflow runId: `wf_f577e7e6-b9d`; transcript dir (its `journal.jsonl`)
  `C:\Users\boss2\.claude\projects\g--DevStuff-GitHub-wowsimwotlk\2b733101-1b9b-4106-be24-1e2f5864000d\subagents\workflows\wf_f577e7e6-b9d`.
- `sim/core` splits between the two timing items: PAR-P3-4 takes `rage.go`, `aura_helpers.go`, a new
  `ppm.go`, `debuffs.go`, the generated `serverdata/` files and `sim/common/**`; PAR-P3-5 takes
  `dot.go`, `periodic_action.go`, `spell_outcome.go`, and `cast.go` and `aura.go` only to drop P3-3's
  channel exemption.
- RI-3 reads the roster export at `G:\DevStuff\GitHub\.wave-loop\raid.json`, taken live at wave start,
  so it needs no server. Its click-through checks are the user's: nothing here drives a browser.

## BiS baseline

The `optimizer_slow` suite after each wave ([how to run](../guide/testing.md#go)), as J over the spec's
preset gear. A `J_preset` drop the goldens don't explain means a parity change hit that spec's gear or
rotation harder than the golden suites cover, which is how wave D caught Glyph of Reckoning.

| Wave | Fury P1 | Combat Rogue P3 | Fire Mage P3 | Ret P4 |
|---|---|---|---|---|
| D | 8607.6, +273 / +244 | 10547.1, +823 / +838 | 5044.3, +14 / +20 | 13140.6, +102 |

Quick / Normal. Ret P4 ran at Quick only, after the glyph fix; its wave C numbers (12821.9, +54 / +73)
came from a seed that wore the glyph.

## Status

| WI | Status | Merge commit | Notes |
|---|---|---|---|
| PAR-P4R | merged | `21f3881b3` | TestBlood golden promoted in `e43b0540f` |
| BIS-contract | merged | `11012001c` | |
| PAR-P5-1 | merged | `35f5544be` | |
| AC-1 | merged | `63553f9ef` | |
| AC-2 | merged | `80e9cd8c7` | 34 goldens promoted in `9db9bd34b` |
| PAR-P3-1 | merged | `d9597704b` | capture driver in mod-sim-validation `a77f6f3` |
| BIS-catalog | merged | `aeded1153` | re-run on AC-2's `db.json`: unchanged |
| BIS-eval | merged | `42291df9a` | |
| PAR-P6-2 | merged | `d17c2838c` | 30 goldens promoted in `7e1567b9f` |
| PAR-P5-23 | merged | `ac367aa97` | |
| BIS-rules | merged | `92027abb1` | catalog regenerated: 186 items moved |
| BIS-raidctx | merged | `61b8c303c` | |
| PAR-P3-2 | merged | `763d0056e` | 21 goldens promoted in `2df9b319b` |
| PAR-P3-3 | merged | `ef34b0059` | 34 goldens promoted in `936c96e19` |
| BIS-search | merged | `67b8ea2d3` | |
| BIS-ui-tab | merged | `5683f6892` | icons and tooltips fixed after the user's check, `302a6e29a` |
| PAR-P3-4 | merged | `d9de023d3` | 23 goldens promoted in `e9a444be8`; e2e `7368cac` |
| PAR-P3-5 | running | | |
| PAR-P6-1 | running | | |
| RI-3 | running | | |

Later WIs are added as their wave starts.

## User actions

- None until wave E's report.
