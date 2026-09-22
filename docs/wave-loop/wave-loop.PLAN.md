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
| G | PAR-P7-DK (G) · PAR-P7-HUN (G) · BIS-batch-ui · PAR-PERF-2 (G) | ✔ |
| H2 | PAR-P7-0d (G, FS) | ✔ |
| H | PAR-P7-ROG · PAR-P7-WAR · PAR-P7-RET (G) · BIS-raid-contrib | ✔ |
| I | PAR-P7-SHA · PAR-P7-DRU · PAR-P7-MAG · PAR-P7-WLK (G) | ✔ |
| J | PAR-P7-PRI (G) · PAR-P7-TANK (G) · BIS-e2e-perf · AC-3 | ✔ |
| K | BIS-presets · PAR-P8 (G, FS) · BIS-tank-boss | ✔ |

F2 is an inserted wave, not a fifth item in F: the core swing and cast fixes of PAR-P7-0c have to land
before the class items in G, a class item can't build on a core fix merging in its own wave, and F already
carries the 2 full-suite golden changers a wave is allowed. BIS-picker-switch moved in from G to give it a
second, light item. PAR-PERF sits there because F2 ends the core combat work: after it, G through J pile
eleven class items on top and a profile can no longer say what cost what.

H2 is inserted the same way: PAR-P7-0d's core cast, crit and pet fixes have to land before the caster
wave I, and H is full. The user ran it before H, so H's melee items build on its swing fixes.

**Where the specs are:**

| WI prefix | Spec |
|---|---|
| `PAR-` | [parity PLAN, "Loop work items"](../azerothcore-parity/azerothcore-parity.PLAN.md) |
| `AC-` | [item-diff PLAN, "Loop work items"](../azerothcore-item-diff/azerothcore-item-diff.PLAN.md) |
| `RI-3` | [raid-import PLAN, Phase 3](../azerothcore-raid-import/azerothcore-raid-import.PLAN.md) |
| `BIS-` | [bis-optimizer PLAN, "Work items"](../bis-optimizer/bis-optimizer.PLAN.md) |

## Current wave

- Wave: I, not started. Wave H (base `695c55b78`) landed on `master`.
- Base SHA: set at wave start.
- Workflow runId: none. Wave H ran as `wf_cc41a703-bd9`.
- Before I, the user decides whether PAR-P7-0e runs first as its own wave, as P7-0d did in H2: wave I's
  Mage item needs its delay helper for Ignite.

## BiS baseline

The `optimizer_slow` suite after each wave ([how to run](../guide/testing.md#go)), as J over the spec's
preset gear. A `J_preset` move the goldens don't explain is worth tracing (wave D caught Glyph of
Reckoning that way), but J isn't DPS: see below.

| Wave | Fury P1 | Combat Rogue P3 | Fire Mage P3 | Ret P4 | Prot Pal P3 | Feral Tank P2 |
|---|---|---|---|---|---|---|
| D | 8607.6, +273 / +244 | 10547.1, +823 / +838 | 5044.3, +14 / +20 | 13140.6, +102 | | |
| E | 8963.3, +267 / +261 | 10546.7, +838 / +839 | 5042.8, +14 / +16 | 13141.4, +140 / +109 | | |
| F | 8961.6, +253 / +276 | 10546.7, +840 / +870 | 5042.7, +15 / +9 | 13141.4, +117 / +143 | -91924.9, +5493 / +5984 | -1336.3, +1421 / +1469 |
| F2 | 8808.8, +258 / +258 | 10546.6, +833 / +865 | 5042.7, +15 / +9 | 13141.4, +117 / +143 | -91924.9, +5493 / +5984 | -1326.7, +1408 / +1460 |
| G | 8808.8, +258 / +258 | 10546.6, +833 / +865 | 5042.7, +15 / +9 | 13141.4, +117 / +143 | -91924.9, +5493 / +5984 | -1326.7, +1408 / +1460 |
| H2 | 8808.8, +258 / +258 | 10546.6, +833 / +865 | 5059.8, +31 / +29 | 13141.4, +117 / +97 | -92086.3, +5511 / +5999 | -1326.7, +1408 / +1460 |
| H | 8744.8, +259 / +326 | 10686.4, +616 / +748 | 5059.8, +31 / +29 | 12929.9, +103 / +184 | -91847.8, +5451 / +5891 | -1326.7, +1408 / +1460 |

Quick / Normal. Ret P4 ran at Quick only in wave D, after the glyph fix; its wave C numbers
(12821.9, +54 / +73) came from a seed that wore the glyph. The two tanks arrive with
BIS-tanks-racials in F; their J is negative because DTPS and TMI carry negative normalizers, so
only the deltas say anything.

**Reading `J_preset`.** J is DPS over an AP normalizer (dDPS/dAP) each run measures from paired ±100 AP
sims. So `J_preset` moves when AP's marginal worth moves, not only when DPS does, and it carries the
normalizer's noise, which the slow line's ± leaves out: about 1.2% for Fury, whose rage feedback
decorrelates the paired sims, against 0.04% for Combat Rogue. F2's Fury drop (-1.7%) was that noise:
the preset's DPS rose on all 7 seeds tried, and at 100k iterations the two builds agree. Feral Tank's
+0.7% is real, from P7-0c's off-hand start on Algalon. Wave E's open gaps, Fury +4.1% and Ret flat
against goldens +0.1% and +8.9%, fit the same reading but weren't traced. Judge the deltas, and a
`J_preset` move against the slow line's `dps_preset`. From H: Fury 8142.4, Combat Rogue 9945.9, Fire
Mage 11746.7, Ret 16434.7, Prot Pal 295.3, Feral Tank 4159.7. Normal's pick is path-dependent: H2 moved
Ret's golden by 0.007% and its Normal gain fell from +143 to +97. In H the DPS presets rose with their
goldens (Fury +9.6%, Combat Rogue +9.1%, Ret +7.0%); Combat Rogue's Quick gain fell from +720 to +574 DPS
while Normal held. Prot Pal's preset lost 11% DPS against TestProtection's -0.6%, not traced (PAR-P7-TANK).

## Sim throughput

`BenchmarkSimulate` after each wave ([how to run](../guide/testing.md#go)), ms per op, median of three runs
on an idle machine. No golden measures time, so parity work that costs the hot paths shows up only here.

From G the benches run their suites' default players with rotations, at 1 and 100 iterations:

| Wave | Combat Rogue | Ret Paladin | Hunter | Elemental | Raid |
|---|---|---|---|---|---|
| G | 1.538 / 130.8 | 0.491 / 33.6 | 0.610 / 41.7 | 0.327 / 15.1 | 4.604 / 311.4 |
| H2 | 1.561 / 129.4 | 0.515 / 35.0 | 0.613 / 43.2 | 0.343 / 16.1 | 4.755 / 313.9 |
| H | 1.451 / 130.8 | 0.477 / 32.4 | 0.585 / 40.8 | 0.327 / 15.1 | 4.568 / 300.2 |

H2 ran with the live worldserver using half a core, which moved whole runs by up to 2×
and cost a few percent here (Ret, which H2 barely touched, +4%). An interleaved A/B against the base
puts H2's own cost at 5-7% for Hunter and Elemental at 100 iterations, the raid flat. H ran with the
worldserver at a sixth of a core, and every case came in at or under G's.

E to F2 ran the old requests: one iteration, and no rotation for Ret, Hunter and Elemental.

| Wave | Combat Rogue | Ret Paladin | Hunter | Elemental | Raid |
|---|---|---|---|---|---|
| E | 1.388 | 0.387 | 0.474 | 0.470 | |
| F | 1.005 | 0.290 | 0.380 | 0.369 | |
| F2 | 0.882 | 0.212 | 0.311 | 0.283 | 6.883 |

Wave F came in 18-26% under wave E on all four at once, including the two specs with no pet, which
nothing in the wave explains. F's row was taken on an idle machine and reproduces within 3%, so the
gap is what else was running during E, not the sim. Compare F onward; treat E as a loose ceiling.

F2 is PAR-PERF's fixes: 12-27% under F on the same requests, median of three runs. The raid bench
crashed before F2, so its column starts there.

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
| PAR-P7-0a | merged | `593158291` | 37 goldens promoted in `8c56ed770`; needed no serverdata regeneration |
| PAR-P7-0b | merged | `eea3298fc` | 20 pet goldens in `82f9e3a22`, re-promoted in `f3b2d9467` after the cross-review |
| BIS-tanks-racials | merged | `f688a9246` | `db.json` regenerated for Titanguard in `4c69521ed` |
| PAR-TOOLS-RR | merged | `e20ac0f21` | e2e `0339692`; 2 of the 4 recorded runs captured |
| wave F cross-review | | `3e0c922c8` | 3 bugs, each inside one item; the only cross-item finding was a tank run's cost, sent to BIS-batch-ui |
| PAR-P7-0c | merged | `922ffc7ab` | 14 goldens promoted in `e52af57b0` |
| PAR-PERF | merged | `8a963ff77` | goldens unchanged on top of PAR-P7-0c's; what changes results went to PAR-PERF-2 |
| BIS-picker-switch | merged | `f1c9d4f4d` | the orchestrator kept Save for an equipped improved pick |
| wave F2 cross-review | | `0fe9b481a` | no code bugs; a held-swing edge case to PAR-P7-WAR; Fury's `J_preset` drop traced to normalizer noise |
| PAR-P7-DK | merged | `cc1f42afa` | 5 DK goldens promoted in `8b87ec76a`; e2e `1c56fe2`; `db.json` regenerated for its spell ids in `d4fa9b3f7` |
| PAR-P7-HUN | merged | `694a92eb1` | 3 hunter goldens promoted in `68a2a4bfb`; e2e `e5aa956`; the INVESTIGATION's deviation rows renumbered at merge |
| BIS-batch-ui | merged | `4887ef580` | |
| PAR-PERF-2 | merged | `956ba4988` | goldens unchanged |
| wave G cross-review | | `a2b24100d`, `3ac9069fc` | split in two (parity; optimizer and UI); one batch bug (Apply and Save on a roster changed mid-run); the old DK ids now alias in the APL lookup; tooling moved out of scratch in `73e159351` |
| PAR-P7-0d | merged | `18b64143b` | 26 goldens promoted in `85c870717`; four stages and a review, 1.86M tokens across the five agents |
| wave H2 cross-review | | `3b9a3c6e6`, `ec0164745` | fixed the user's DK reforge report: the item cache kept items a stale page sent without server stats, and the DK registered its runeforges and sigils late (mispriced first optimize, a possible crash); its claim that the permanent ghoul lacks 51996 was wrong (a Ghoul family passive) and was reverted |
| PAR-P7-ROG | merged | `bfe3ec2f3` | 3 rogue goldens promoted in `1e1cbdc6a`; e2e `55764db` |
| PAR-P7-WAR | merged | `3b51edc5b` | 3 warrior goldens promoted in `3f29eb79a`; e2e `3f59b6b`; serverdata regenerated for Slam's damage spell |
| PAR-P7-RET | merged | `74a9a8c60` | 2 paladin goldens promoted in `be8b078a0`; e2e and the recorded run's talent and tier steps `da7028e`; its two deviation rows were module behaviour, dropped by the cross-review |
| BIS-raid-contrib | merged | `f6da0097a` | goldens unchanged |
| wave H cross-review | | `2ae1e1906` | 5 bugs: Slam's split left Recklessness and the T8 2pc on the cast, Deadly Poison kept its first haste, Exorcism was limited to undead and demons, the Ret capture's talent reset failed; Assassination +2.2% (goldens `6ca712f26`); module `39e086e`; spell audit refreshed in `ba4252567` |

Later WIs are added as their wave starts.

## User actions

- Rebuild the prod container for H2's reforge fix and reload open sim tabs. Until then a restart clears
  items cached without server stats.
- Worth a click-through when convenient: the optimizer tab's tank controls on a tank spec (the
  survival/threat slider, the crit-immunity box, the racial select). No agent can judge those, and
  BIS-ui-tab's own click-through found three real bugs.
