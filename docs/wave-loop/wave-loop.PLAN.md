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
| H3 | PAR-P7-0e (G, FS) · PAR-P7-RET-RR · BIS-alt5 | ✔ |
| I | PAR-P7-MAG (G, FS) · PAR-P7-SHA · PAR-P7-DRU · PAR-P7-WLK (G) | ✔ |
| U | UI-RESULTS · UI-GEAR · UI-RAID · UI-SETTINGS | |
| I2 | PERF-TOOLS · PERF-CONC · BIS-seed · UI-FIX | ✔ |
| I3 | PERF-OPT · PERF-HOT · PAR-P7-0f · UI-FIX2 | ✔ |
| I4 | PAR-DECL (G, FS) · RI-4 · BIS-hunter-ranged | ✔ |
| I5 | PAR-DECL-1 (G) · PAR-DECL-2 (G) · PAR-DECL-3 (G) | ✔ |
| I6 | PERF-MISSILE (FS) · PAR-DECL-4 (G, FS) | ✔ |
| J | PAR-P7-PRI (G) · PAR-P7-TANK (G) · BIS-e2e-perf · AC-3 | ✔ |
| J2 | BIS-stage2 · BIS-adopt · PERF-RNG (FS) · BIS-stage2b | ✔ |
| K | BIS-presets · PAR-P8 (G, FS) · BIS-tank-boss | ✔ |

F2 is an inserted wave, not a fifth item in F: the core swing and cast fixes of PAR-P7-0c have to land
before the class items in G, a class item can't build on a core fix merging in its own wave, and F already
carries the 2 full-suite golden changers a wave is allowed. BIS-picker-switch moved in from G to give it a
second, light item. PAR-PERF sits there because F2 ends the core combat work: after it, G through J pile
eleven class items on top and a profile can no longer say what cost what.

H2 is inserted the same way: PAR-P7-0d's core cast, crit and pet fixes have to land before the caster
wave I, and H is full. The user ran it before H, so H's melee items build on its swing fixes. H3 is the
user's call too (2026-09-22): wave I's Mage item needs PAR-P7-0e's delay helper for Ignite. PAR-P7-RET-RR
rides along, as it only needs the live server. BIS-alt5 joined mid-wave, also the user's call.

In I, PAR-P7-MAG also fixes the delay helper's timing gap (the user's call) and tick-rounds the APL's
`spell.cast_time`, both in core, so it runs all 37 suites and merges first. I2 and I3, the performance pass,
are the user's call too: they come before J, whose BIS-e2e-perf uses their tools. So are I4 and I5
(2026-09-23), the [effect declarations](../azerothcore-parity/effect-declarations.PLAN.md): PAR-DECL is I4's only parity item,
since a class item can't build on a core change merging in its own wave, and both come before J so that
PRI, TANK and K's PAR-P8 build on them. I6 too (2026-09-26): PERF-MISSILE wins back what I5's travel waits
cost before J's BIS-e2e-perf times the optimizer, and PAR-DECL-4 adds the ids I5 found `serverdata` lacking.

U, the Playwright suite and the UI bugs it finds, comes before I2 (the user's call, 2026-09-23). Its items
touch only UI and test paths, so goldens, BiS and throughput can't move, and it skips the re-baseline.
UI-FIX joins I2 (the user's call): four fixes wave U's tests left open. PAR-P7-0f joins I3 (the user's call,
2026-09-24): two core fixes wave I2 found. So do UI-FIX2 in I3, and RI-4 and BIS-hunter-ranged in I4: two UI
bugs, the importer's missing pets, ammo and consumables, and a batch hung on a hunter, all found by the user.
J2 is the user's call too (2026-09-27): wave J's batch took ~8 h, and its stage 2 made most picks worse
([speed audit](../bis-optimizer/bis-optimizer.INVESTIGATION.md#speed-audit-after-wave-j)), so the optimizer
gets fixed before K builds presets with it.

**Where the specs are:**

| WI prefix | Spec |
|---|---|
| `PAR-` | [parity PLAN, "Loop work items"](../azerothcore-parity/azerothcore-parity.PLAN.md) |
| `AC-` | [item-diff PLAN, "Loop work items"](../azerothcore-item-diff/azerothcore-item-diff.PLAN.md) |
| `RI-` | [raid-import PLAN, Phase 3 and RI-4](../azerothcore-raid-import/azerothcore-raid-import.PLAN.md) |
| `BIS-` | [bis-optimizer PLAN, "Work items"](../bis-optimizer/bis-optimizer.PLAN.md) |
| `PERF-` | [sim-performance PLAN, "Work items"](../sim-performance/sim-performance.PLAN.md) |
| `UI-` | [ui-tests PLAN, "Work items"](../ui-tests/ui-tests.PLAN.md) |

## Current wave

- Wave: J2, running. Base SHA `b24799131`. Wave J landed on `master`, and prod runs it.
- Workflow runId: `wf_b5e75270-6c6`, args `waveJ2-args.json` in `G:\DevStuff\GitHub\.wave-loop`. Transcript dir:
  `C:\Users\boss2\.claude\projects\g--DevStuff-GitHub-wowsimwotlk\2b733101-1b9b-4106-be24-1e2f5864000d\subagents\workflows\wf_b5e75270-6c6`.
- All four items merged and cross-reviewed. Next, alone on the machine: the re-baselines, then the full-roster
  batch at Quick (`--out G:\DevStuff\GitHub\.wave-loop\bis-j2`, `wotlk-bisdata` on 3346 over [int]).

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
| H3 | 8865.0, +282 / +278 | 10686.4, +616 / +942 | 5059.8, +31 / +29 | 12956.4, +231 / +227 | -91848.5, +5451 / +5891 | -1326.7, +1408 / +1460 |
| I | 8842.8, +253 / +304 | 10686.4, +616 / +942 | 5064.5, +43 / +40 | 12956.7, +224 / +256 | -91848.5, +5451 / +5891 | -1324.8, +1405 / +1456 |
| I2 | 8842.8, +253 / +304 | 10686.4, +616 / +942 | 5064.5, +43 / +40 | 12956.7, +224 / +256 | -89694.0, +3349 / +3736 | -668.4, +750 / +799 |
| I3 | 8842.8, +272 / +304 | 10686.4, +608 / +942 | 5064.5, +43 / +40 | 12962.2, +224 / +228 | -89694.0, +3349 / +3736 | -668.4, +750 / +799 |
| I4 | 8842.8, +272 / +304 | 10686.4, +608 / +942 | 5064.5, +43 / +40 | 12962.2, +224 / +228 | -89694.0, +3349 / +3736 | -668.4, +750 / +799 |
| I5 | 8860.8, +284 / +293 | 10728.5, +653 / +813 | 5064.6, +43 / +43 | 12963.5, +224 / +182 | -89267.2, +3186 / +3703 | -668.4, +750 / +799 |
| I6 | 8860.8, +284 / +293 | 10728.5, +653 / +813 | 5064.6, +43 / +43 | 12963.5, +224 / +182 | -89267.2, +3186 / +3703 | -668.4, +750 / +799 |
| J | 8860.8, +282 / +286 | 10728.5, +624 / +926 | 5064.6, +43 / +63 | 12963.5, +229 / +228 | -97067.0, +3376 / +3759 | -1644.5, +727 / +821 |
| J2 | 8725.6, +253 / +297 | 10728.7, +674 / +799 | 5064.5, +40 / +47 | 12964.3, +213 / +231 | -96051.1, +3590 / +3707 | -1642.4, +740 / +804 |

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
`J_preset` move against the slow line's `dps_preset`. From I: Fury 8210.4, Combat Rogue 9945.9, Fire
Mage 11280.3, Ret 16140.2, Prot Pal 294.9, Feral Tank 4140.9. Normal's pick is path-dependent: H2 moved
Ret's golden by 0.007% and its Normal gain fell from +143 to +97. In H the DPS presets rose with their
goldens (Fury +9.6%, Combat Rogue +9.1%, Ret +7.0%); Combat Rogue's Quick gain fell from +720 to +574 DPS
while Normal held. Prot Pal's preset lost 11% DPS against TestProtection's -0.6%, not traced (PAR-P7-TANK).
In H3 Ret's preset lost 1.8% with its goldens (-1.4%) and its gains rose to +231 / +227, as its items were
revalued; Combat Rogue's Normal gain rose to +942 from a 4th or 5th runner-up (BIS-alt5). In I the Fire Mage
preset lost 4.0% with TestFire (-3.7%) while its J rose, since the normalizer fell with it; Combat Rogue and
Prot Paladin, whose classes the wave left alone, didn't move at all. In I2 the DPS rows held. BIS-seed scores
the slow line's `J_preset`, gains and `dps_preset` against the preset as equipped, which the pool's trim
had cut into for both tanks: their `J_preset` rose and their gains fell by as much, while their Normal
`J_opt` held within 0.4. Their `dps_preset` now reads Prot Pal 257.1, Feral Tank 4282.9. In I3 PERF-OPT
moved two Quick gains within earlier rows' range: Fury +272, Combat Rogue +608. Ret's `J_preset` and
`dps_preset` (16147.1) rose 0.04% with PAR-P7-0f's +20.61 Spell Power, and its Normal gain fell from +256
to +228, the path dependence H2 showed. In I4 nothing moved: the wave's one golden move, Frost mage, isn't
in the suite. In I5 Prot Pal's `dps_preset` fell 16% (215.6) against TestProtection's -1.1%, likely Holy
Shield's proc (-15%) weighing more against a boss that keeps hitting (not traced). Combat Rogue's fell 0.2%
(9927.0) with its goldens (-0.7%); its Normal gain fell to +813 and Ret's to +182, the path dependence again.
In I6 nothing moved: PERF-MISSILE keeps every golden, and PAR-DECL-4's (hunter, Frost mage) aren't in the suite.
In J PAR-P7-TANK moved both tanks' `J_preset`: Prot Pal's `dps_preset` rose 21% (260.8), back near I4's, as
Holy Shield no longer misses, and Feral Tank's DTPS rose with its goldens (+5.9%, Faerie Fire no longer procs
Savage Defense). Four Normal gains rose, Combat Rogue's to +926, likely BIS-e2e-perf's neighborhood adopting a
clear win on its last round.
In J2 PERF-RNG's per-unit streams gave the optimizer new random numbers: Fury's `J_preset` fell 1.5%
(8725.6), inside its normalizer's noise, and the DPS specs' `dps_preset` moved under 0.1%. Where BIS-adopt
took a better swap, Quick gained (Combat Rogue +674, Prot Pal +3590); Normal fell for Combat Rogue (+799)
and Fire Mage (+47), the path dependence again. Racing cut Normal's sims about 23% and its wall 18–24% on
the DPS specs.

## Sim throughput

`BenchmarkSimulate` after each wave ([how to run](../guide/testing.md#go)), ms per op, median of three runs
on an idle machine. No golden measures time, so parity work that costs the hot paths shows up only here.

From G the benches run their suites' default players with rotations, at 1 and 100 iterations:

| Wave | Combat Rogue | Ret Paladin | Hunter | Elemental | Raid |
|---|---|---|---|---|---|
| G | 1.538 / 130.8 | 0.491 / 33.6 | 0.610 / 41.7 | 0.327 / 15.1 | 4.604 / 311.4 |
| H2 | 1.561 / 129.4 | 0.515 / 35.0 | 0.613 / 43.2 | 0.343 / 16.1 | 4.755 / 313.9 |
| H | 1.451 / 130.8 | 0.477 / 32.4 | 0.585 / 40.8 | 0.327 / 15.1 | 4.568 / 300.2 |
| H3 | 1.490 / 132.8 | 0.493 / 33.6 | 0.602 / 42.9 | 0.332 / 15.4 | 4.586 / 304.2 |
| I | 1.588 / 134.8 | 0.573 / 34.0 | 0.784 / 46.1 | 0.495 / 16.8 | 6.026 / 302.5 |
| I2 | 1.685 / 128.3 | 0.639 / 33.2 | 0.774 / 43.1 | 0.444 / 15.6 | 5.382 / 294.8 |
| I3 | 1.101 / 74.8 | 0.553 / 27.9 | 0.757 / 41.4 | 0.386 / 12.2 | 5.355 / 250.0 |
| I4 | 1.088 / 75.2 | 0.510 / 27.8 | 0.743 / 40.3 | 0.414 / 12.5 | 5.029 / 250.6 |
| I5 | 1.059 / 78.6 | 0.508 / 28.5 | 0.727 / 45.7 | 0.488 / 18.7 | 5.463 / 263.4 |
| I6 | 1.104 / 76.5 | 0.530 / 28.6 | 0.823 / 44.1 | 0.503 / 17.0 | 5.296 / 263.6 |
| J | 1.087 / 77.1 | 0.527 / 28.7 | 0.755 / 43.2 | 0.480 / 17.2 | 5.213 / 262.6 |
| J2 | 1.043 / 77.5 | 0.561 / 28.7 | 0.759 / 44.3 | 0.493 / 16.9 | 5.300 / 272.8 |

H2 ran with the live worldserver using half a core, which moved whole runs by up to 2×
and cost a few percent here (Ret, which H2 barely touched, +4%). An interleaved A/B against the base
puts H2's own cost at 5-7% for Hunter and Elemental at 100 iterations, the raid flat. H ran with the
worldserver at a sixth of a core, and every case came in at or under G's. H3, same load, came in 0.4-5% over
H: Combat Rogue, whose hot path H3 barely touched, +1.5-2.7%, so most of it is noise; Hunter at 100
iterations +5%, likely the pending action each Piercing Shots proc now queues.

I's row sits 20-30% over H3's at one iteration, but the base re-measured beside it that day did too (Ret
0.584, Hunter 0.865, Elemental 0.428, raid 5.915), so the machine moved, not the sim. Its interleaved A/B
puts the wave's own cost at Elemental +15.7% at one iteration and +6.6% at 100, from the spells the id
splits add at setup; every other case lands within ±5%, some negative. Read the A/B, not the row.

I2's row ran idle, the worldserver stopped: at 100 iterations every case sits 2-7% under I's, at one
iteration between 11% under (the raid) and 12% over (Ret). An interleaved A/B against master, the wave's
base, taken under load, put every case within ±10%, 8 of 10 faster. The bench drives `RunRaidSim`, which
PERF-CONC left one stream, so the Simulate button's speedup shows in the
[harness baseline](../sim-performance/sim-performance.INVESTIGATION.md#baseline-integration-idle-machine),
not here.

I3's row ran idle, with the worldserver, database and Chronicle stopped. Against I2's, PERF-HOT's cuts at 1 /
100 iterations: Rogue -35% / -42%, Elemental -13% / -22%, Retribution -13% / -16%, the raid -1% / -15%, Hunter
-2% / -4%. The harness's re-timing is in the
[INVESTIGATION](../sim-performance/sim-performance.INVESTIGATION.md#re-timing-after-wave-i3-integration-idle-machine).

I4's row ran idle (the worldserver stopped, the database up). Elemental is 7% over I3's at one iteration and
2% at 100: PAR-DECL's cost, +2.5% / +3.5% in its A/B against the wave base, with no new code in the profile,
so the cause is open. The other cases are within 3% at 100 iterations and 1-8% faster at one.

I5's row ran idle (the worldserver, authserver and database stopped). At 100 iterations Elemental is 50%
over I4's, Hunter 13%, the raid 5%, Rogue 5%, Ret 3%. An interleaved A/B of Elemental against `sim/shaman`
at the wave base puts PAR-DECL-2's cost at +39% / +47%: a 100-iteration op allocates 139k objects, not
34k (7.2 MB, not 1.2), from `WaitTravelTime`'s delayed action per missile and `NewResult` allocating
while an earlier missile holds the spell's cached result (Lightning Bolt, Searing Totem). Clearing that
cache in `Spell.reset` alone saves 1.6%. Hunter's +13% fits the same cause, not measured.

I6's row ran idle too. At 100 iterations PERF-MISSILE brought Elemental 9% under I5's and Hunter 4%; the rest
moved under 3%. Elemental stays 36% over I4: an op queues 112k actions, not 64k, and makes 12.5k more APL
passes that cast nothing, the rotation since I5 rather than allocation. One-iteration Hunter reads 13% over I5
here, but an interleaved A/B of the whole wave puts it at -1.6% (-4.0% at 100 iterations): read the A/B.

J's row ran idle (the worldserver and database stopped): every case within 2% of I6's at 100 iterations, and
at one iteration Hunter 8% under, the rest within 5%. Elemental's is a re-run, one of the first three having
read 74% over the others at 100 iterations; another session's container used under a core during it.

J2's row ran idle too (the worldserver stopped, the database up). At 100 iterations every case is within 3%
of J's but the raid, +3.9%. PERF-RNG's own interleaved A/B found no difference outside noise: these benches
run with `IsTest` off, where rolls take the old path.

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
| PAR-P7-0e | merged | `c056bfd24` | 6 goldens promoted in `051d8e6b9` |
| PAR-P7-RET-RR | merged | `30caa2716` | 2 paladin goldens promoted in `3390bacf0`; e2e `f4060b1`; a repair round redid the first run's per-ability table, which compared melee with glances on one side only and mixed crit rate into hit size |
| BIS-alt5 | merged | `826e952c4` | goldens unchanged; its reviewer fixed a pick lost on a tight budget |
| wave H3 cross-review | | `d2bf9e5e9`, `050ec2fac` | 1 bug: a delayed refresh landed after a same-tick swing or dot tick, paying the tick twice (Fury -1.5%, Arms -1.3%, Ret -0.8%); retail deviations for munching and spell mods dropped or moved to unsettled; module `1bd7607` |
| PAR-P7-MAG | merged | `e01becb6d` | 13 goldens promoted in `de48cfabf`; a live Fire capture closed PAR-P7-0e's delay gap; module `4f2cb34` |
| PAR-P7-WLK | merged | `727738cf6` | 3 goldens promoted in `eea8954f8`; serverdata regenerated in `733088729`; Affliction recaptured; module `7af08bd`, which also fixed a ranged recorded run losing line of sight |
| PAR-P7-SHA | merged | `3b624ddf6` | 2 goldens promoted in `18c0e3429`; its live round found the totem-dot split panicking any APL that named the summon's dot; module `987b74e` |
| PAR-P7-DRU | merged | `b4fff9609` | 5 goldens promoted in `667779e0f`; serverdata regenerated in `8fad1a579`; module `b5d8135` |
| wave I cross-review | | `38c7b5243`, `4c9fde2d3` | 4 bugs: the spirit wolves took Windfury Totem and Improved Icy Talons twice, the totems' own hits fed the shaman's procs, Fire Nova's crits stopped granting Clearcasting, and Ignite read a cast time a missile had outlived; Elemental and Enhancement re-promoted |
| UI-RESULTS | merged | `05329a523` | 15 bugs fixed, among them the whole-raid timeline, the target filter and the damage-row pie; the log tab gained search and raider filtering; 3 fixtures |
| UI-GEAR | merged | `febe74f8b` | 13 bugs fixed, mostly swap and batch enchant lists, the batch setup on reload, and leaked listeners; 1 fixme |
| UI-RAID | merged | `cafc5b44d` | 11 bugs fixed: tanks and buff targets follow their raider through edits, imports and reloads; 1 fixme; `1851b79d8` settled its clash with UI-SETTINGS's exporter header |
| UI-SETTINGS | merged | `70247564c` | 22 bugs fixed across the encounter, talents, importers and exporters; 1 fixme |
| wave U cross-review | | `305acaf57` | 3 bugs: the results page built the whole log with the Log tab closed, slowing every tab; `activateTab` never switched tabs; closed modals kept their window listeners. 3 timing-dependent tests fixed. The gate now lists every eslint file (`68720687a`) and no longer breaks tsc while it lints (`9d73e93b4`) |
| PERF-TOOLS | merged | `5751dd72b` | profiling flags on the CLI, pprof moved off the sim's port behind `--pprof`, `tools/perf` trace reader and bench harness, benchstat in the toolchain image; baseline taken at `93e62aecd` |
| PERF-CONC | merged | `891421898` | the Simulate button shards its iterations over GOMAXPROCS-1 goroutines: 7.1 to 8.8× at 16 threads, idle; bulk sim at GOMAXPROCS goroutines |
| BIS-seed | merged | `d31d32c9e` | equipped items stay in the pool, gains are against the gear as equipped, the score names its stat; 1 bug fixed in review (a locked second ring that moved up left its pool entry behind) |
| UI-FIX | merged | `b332259d9` | all four fixes, plus Block Value's multiplier; the warrior row is "Stance & Shout"; Might counted twice in the tooltip's snapshots found and left (moves goldens) |
| wave I2 cross-review | | `93e62aecd` | 2 bugs: a 40-player raid crashed when a raider in groups 6 to 8 had a healing model (presim sized 25), and the batch sim's progress reporter could send on a closed channel; the harness's optimizer requests refreshed for BIS-seed's `equipped`; a test keeps pprof off the sim's port |
| PERF-OPT | merged | `8c1ab4802` | Quick 9 to 40% faster idle, evaluator busy 56 to 89% at 16 threads (was 53 to 78%); raid mode screens sets on the raider's own DPS, 4 simmed where 11 were |
| PERF-HOT | merged | `bb1e35ab4` | goldens byte-identical; sims up to 41% faster idle (Rogue), stat weights 39%, bulk 48%; Protection Warrior +3% CPU at 16 threads in its A/B |
| PAR-P7-0f | merged | `3abef3a1f` | Enhancement and Retribution goldens promoted in `48fd72cab` (+20.61 Spell Power; the spec expected none) |
| UI-FIX2 | merged | `49344a96c` | |
| wave I3 cross-review | | | no findings |
| PAR-DECL | merged | `accba95f6` | Frost goldens promoted in `812eb7771` (-0.02%, Frostbolt); missile speeds move nothing until I5 adds the travel waits; Elemental bench +2.5% / +3.5%, cause open |
| RI-4 | merged | `fc052a747` | acbis rejected version 1 rosters, fixed at merge |
| BIS-hunter-ranged | merged | `4a04a6c4d` | |
| wave I4 cross-review | | `d1814fae3` | 2 bugs: RI-4's real pets exposed Spore Cloud going to a Bat, not a Spore Bat, in the raid stats and the BiS batch's raid context; a Thori'dal hunter's export cleared their ammo, so every other bow simmed without any. A mistyped `FromSpellID` now panics |
| PAR-DECL-1 | merged | `26663ada7` | 7 goldens promoted in `8f4752244`: Frost mage +0.38% (Mirror Image takes the mage's Frostbolt talents), Destruction -0.54% (Conflagrate, Imp Firebolt, Chaos Bolt's travel) |
| PAR-DECL-2 | merged | `facac0b37` | 7 goldens promoted in `b0c156289`: Survival +3.3% (Explosive Shot's 0.16 AP), Enhancement -2.9% (fire elemental), Elemental -1.5% (travel, Clearcasting kept for the next cast, fire elemental; Intellect weight 0.37 to 3.03); review fixed Savage Fury declared flat, not a percent |
| PAR-DECL-3 | merged | `90a8ea16f` | 13 goldens promoted in `b15974648`: DK Frost +1.9% (off-hand Frost Strike's flat bonus unhalved, Obliterate's halved), Blood Tank -2.7% (Rune Strike), Assassination +1.6% (poisons), Protection Paladin -1.1% (Holy Shield); review fixed Death Coil and Unholy Blight dealing 1 over, and Death Coil rolling at landing |
| wave I5 cross-review | | | no findings; Improved Earth Shield's percent mod on Earth Shield, which it couldn't confirm from source, checked in the live Spell.dbc |
| PERF-MISSILE | merged | `ffe8b7ea1` | goldens byte-identical; its A/B at 100 iterations: Elemental -12% (allocations -59%), Hunter -4%, the raid -2%. Elemental stays ~29% over I4, from more queued actions, not allocation |
| PAR-DECL-4 | merged | `97b207b14` | 4 goldens promoted in `57b62de5b`: hunter BM, MM and SV +0.006% to +0.007% (Explosive Trap's burst scales to level 80), Frost mage +0.0004% (Deep Freeze's SP) |
| wave I6 cross-review | | `148cd9eaf` | 1 bug: Languish read its triggering hit's result after a delay, which PERF-MISSILE's reuse lets another cast overwrite (multi-target only, no golden moves) |
| PAR-P7-PRI | merged | `3812fd681` | 4 goldens promoted in `6679ad909`: Smite +2.3% (Holy Fire's dot), Holy +0.56% (Empowered Renew multiplies), Shadow -0.14%; module `dcecf85`. The orchestrator sent back Mind Flay's tick dropping its 196 base, which the channel hands it (Shadow had read -7.5%) |
| PAR-P7-TANK | merged | `b9e77cded` | 7 goldens promoted in `32d09f2be`: a GenericBoss case in each tank suite, Protection +0.41% (Holy Shield never misses), Feral Tank DTPS +5.9% (Faerie Fire no longer procs Savage Defense), Blood -0.15% (rune weapon); BM and Combat lose a phantom dodge rating; module `926df4c`. The orchestrator sent back a permanent Savage Defense and Feral Swiftness dropped from bear dodge, and gave the generic boss Patchwerk 25's damage (`fb7dbda45`); `db.json` in `63209f728` |
| BIS-e2e-perf | merged | `6dc2141b5`, timing run `cc0a1e03a` | goldens unchanged. The timing run moved Quick's target to 3-11 s and the tanks' to 1.6x a DPS run; a roster at Normal takes about 4 days, so the batch's "Normal overnight" waits on the user. The batch took ~8.3 h at Quick, stage 2 for phase 1 only (the user's call) |
| AC-3 | merged | `b634b26cc` | goldens unchanged; `db.json` regenerated in the merge |
| wave J cross-review | | `eaafe11bd` | no bugs: Mind Sear under-counted its casts in multi-target fights, and the INVESTIGATION had Leeching Swarm's two ids swapped; spell audit refreshed |
| PERF-RNG | merged | `faeb8c649` | goldens unchanged. With it on, a raid-mode delta's SE fell from ±27.1 to ±4.2 raid DPS (raid25, +100 hit); "Damage Roll" and the PPM procs still share streams |
| BIS-adopt | merged | `e3ae9d86c` | goldens unchanged. Quick adopts a clearly better swap: 2 of the slow suite's 6 Quick picks gained, none lost |
| BIS-stage2 | merged | `1686e5c63` | goldens unchanged. Phase 1 on the live roster took 16.5 min against wave J's ~123 (only Fel re-ran, +248 raid DPS). Sent back: its party pass only removes extra Draenei (BIS-stage2b) |
| BIS-stage2b | merged | `7e057b86c` | goldens unchanged; proto comment `6ded31a1e`. Phase 1: the party checks took 47 s and gave Angry, Assasin and Smartface Draenei (+234 to +254 raid DPS each); every final pick together beat all stage 1 picks by +822 ± 12 |
| wave J2 cross-review | | `1493175df` | 1 bug: the driver gave up on a pass 2 with nothing left to run. Also: old stage 2 picks still counted for raiders without stage 2, stage 2 texts said "as equipped" for the stage 1 pick, a cancelled party check read as an error |

Later WIs are added as their wave starts.

## User actions

- Worth a click-through when convenient: the optimizer tab's tank controls on a tank spec (the
  survival/threat slider, the crit-immunity box, the racial select). No agent can judge those, and
  BIS-ui-tab's own click-through found three real bugs.
