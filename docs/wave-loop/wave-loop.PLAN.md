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
| B | AC-2 (G, FS, 12 suites) · PAR-P3-1 · BIS-catalog · BIS-eval | |
| C | PAR-P6-2 (G, FS, 14 suites) · PAR-P5-23 · BIS-rules · BIS-raidctx | |
| D | PAR-P3-2 (G, FS) · PAR-P3-3 (G, FS) · BIS-search · BIS-ui-tab | ✔ |
| E | PAR-P3-4 (G, FS) · PAR-P3-5 (G, FS) · PAR-P6-1 · RI-3 | ✔ |
| F | PAR-P7-0a (G, FS) · PAR-P7-0b (G, FS) · BIS-tanks-racials · PAR-TOOLS-RR | ✔ |
| G | PAR-P7-DK (G) · PAR-P7-HUN (G) · BIS-batch-ui · BIS-picker-switch | ✔ |
| H | PAR-P7-ROG · PAR-P7-WAR · PAR-P7-RET (G) · BIS-raid-contrib | ✔ |
| I | PAR-P7-SHA · PAR-P7-DRU · PAR-P7-MAG · PAR-P7-WLK (G) | ✔ |
| J | PAR-P7-PRI (G) · PAR-P7-TANK (G) · BIS-e2e-perf · AC-3 | ✔ |
| K | BIS-presets · PAR-P8 (G, FS) · BIS-tank-boss | ✔ |

**Where the specs are:**

| WI prefix | Spec |
|---|---|
| `PAR-` | [parity PLAN, "Loop work items"](../azerothcore-parity/azerothcore-parity.PLAN.md) |
| `AC-` | [item-diff PLAN, "Loop work items"](../azerothcore-item-diff/azerothcore-item-diff.PLAN.md) |
| `RI-3` | [raid-import PLAN, Phase 3](../azerothcore-raid-import/azerothcore-raid-import.PLAN.md) |
| `BIS-` | [bis-optimizer PLAN, "Work items"](../bis-optimizer/bis-optimizer.PLAN.md) |

## Current wave

- Wave: A, all four WIs merged and their worktrees removed; next is step 5, the cross-review.
- Base SHA: `e704d104c`.
- Workflow runId: `wf_a636aa2a-e61`. Script passed inline; the persisted copy for `resumeFromRunId` is
  `C:\Users\boss2\.claude\projects\g--DevStuff-GitHub-wowsimwotlk\2b733101-1b9b-4106-be24-1e2f5864000d\workflows\scripts\wave-loop-wf_a636aa2a-e61.js`.

## Status

| WI | Status | Merge commit | Notes |
|---|---|---|---|
| PAR-P4R | merged | `21f3881b3` | TestBlood golden promoted in `e43b0540f` |
| BIS-contract | merged | `11012001c` | |
| PAR-P5-1 | merged | `35f5544be` | |
| AC-1 | merged | `63553f9ef` | |

Later WIs are added as their wave starts.

## User actions

None pending. The pre-wave-A commits (mod-sim-validation's P4 e2e work, the raid-import PLAN status edit) are
done. The user's only action is "continue" after each wave report.
