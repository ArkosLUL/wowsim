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
| F2 | PAR-P7-0c (G, FS) · PAR-PERF · BIS-picker-switch | ✔ |
| G | PAR-P7-DK (G) · PAR-P7-HUN (G) · BIS-batch-ui | ✔ |
| H | PAR-P7-ROG · PAR-P7-WAR · PAR-P7-RET (G) · BIS-raid-contrib | ✔ |
| I | PAR-P7-SHA · PAR-P7-DRU · PAR-P7-MAG · PAR-P7-WLK (G) | ✔ |
| J | PAR-P7-PRI (G) · PAR-P7-TANK (G) · BIS-e2e-perf · AC-3 | ✔ |
| K | BIS-presets · PAR-P8 (G, FS) · BIS-tank-boss | ✔ |

F2 is an inserted wave, not a fifth item in F: the core swing and cast fixes of PAR-P7-0c have to land
before the class items in G, a class item can't build on a core fix merging in its own wave, and F already
carries the 2 full-suite golden changers a wave is allowed. BIS-picker-switch moved in from G to give it a
second, light item. PAR-PERF sits there because F2 ends the core combat work: after it, G through J pile
eleven class items on top and a profile can no longer say what cost what.

**Where the specs are:**

| WI prefix | Spec |
|---|---|
| `PAR-` | [parity PLAN, "Loop work items"](../azerothcore-parity/azerothcore-parity.PLAN.md) |
| `AC-` | [item-diff PLAN, "Loop work items"](../azerothcore-item-diff/azerothcore-item-diff.PLAN.md) |
| `RI-3` | [raid-import PLAN, Phase 3](../azerothcore-raid-import/azerothcore-raid-import.PLAN.md) |
| `BIS-` | [bis-optimizer PLAN, "Work items"](../bis-optimizer/bis-optimizer.PLAN.md) |

## Current wave

- Wave: F, not started. Wave E (base `328553aa9`) landed on `master`.
- Base SHA: set at wave start.
- Workflow runId: none.

## BiS baseline

The `optimizer_slow` suite after each wave ([how to run](../guide/testing.md#go)), as J over the spec's
preset gear. A `J_preset` drop the goldens don't explain means a parity change hit that spec's gear or
rotation harder than the golden suites cover, which is how wave D caught Glyph of Reckoning.

| Wave | Fury P1 | Combat Rogue P3 | Fire Mage P3 | Ret P4 |
|---|---|---|---|---|
| D | 8607.6, +273 / +244 | 10547.1, +823 / +838 | 5044.3, +14 / +20 | 13140.6, +102 |
| E | 8963.3, +267 / +261 | 10546.7, +838 / +839 | 5042.8, +14 / +16 | 13141.4, +140 / +109 |

Quick / Normal. Ret P4 ran at Quick only in wave D, after the glyph fix; its wave C numbers
(12821.9, +54 / +73) came from a seed that wore the glyph.

**Open from wave E:** Fury's `J_preset` rose 4.1% while its golden presets moved about 0.1%, and Ret's
sat still while its goldens gained 8.9%. Neither direction follows the goldens, so something differs
between the two contexts rather than in the spec. Ret has a candidate: the golden suite's paladin judges
for itself, which is the path P3-4 fixed, while the optimizer takes Judgement of Wisdom from full
debuffs, where the sim reads the attacker's weapon instead of the judging paladin's (deviation 20).
Fury has none yet. Until one spec's gap is traced, read the deltas rather than `J_preset`: all four
improved, and every delta matches wave D.

## Sim throughput

`BenchmarkSimulate` after each wave ([how to run](../guide/testing.md#go)), ms per sim. No golden
measures time, so parity work that costs the hot paths shows up only here. PAR-PERF (F2) profiles
whatever these say.

| Wave | Combat Rogue | Ret Paladin | Hunter | Elemental |
|---|---|---|---|---|
| E | 1.388 | 0.387 | 0.474 | 0.470 |

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
| PAR-P3-5 | merged | `edae7e9c6` | 34 goldens promoted in `0f1ad932b` |
| PAR-P6-1 | merged | `9e4030986` | `db.json` regenerated in `70711a632`, replay fixtures in `33a7a63d7`; e2e `d8d7310` |
| RI-3 | merged | `ec483fa1a` | offline checks in `ui/raid/acore_harness` (`5b0e5703c`); the user's click-through found a truncated import alert and squashed checkboxes, both fixed in `059eb8004` |

Later WIs are added as their wave starts.

## User actions

- "continue" for wave F.
