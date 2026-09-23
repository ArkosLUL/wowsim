# Sim and optimizer performance: investigation

Findings behind [sim-performance.PLAN.md](sim-performance.PLAN.md), taken with [tools/perf](../../tools/perf/README.md) on
the Ryzen 7 7800X3D (16 threads, GOMAXPROCS 16). Wave timings are skewed by the items running alongside; targets
come from the integration baseline (PERF-TOOLS item 5).

## Baseline (integration, idle machine)

Method: the harness's [full baseline](../../tools/perf/README.md#full-baseline), with nothing else running; its
[columns](../../tools/perf/README.md#scenario-harness) are the ones below. Its `report.json` becomes
`tools/perf/baseline.json`.

- Commit: `93e62aecd`, wave I2's integration; go1.23.12, 3000 iterations.
- Load: none. The user stopped the worldserver for it, and a per-minute `docker stats` saw nothing else over 6%.
  A first run with the worldserver at 1.4 to 2.0 cores was slower at every point: against it, this one reads
  244 points faster past the noise.
- Wall time: 30 min.

**Utilization** at GOMAXPROCS 16, medians of 3 (Normal ran once). Busy is the optimizer evaluator's.

| Scenario | Wall | CPU | Util | GC | Alloc | Peak heap | Busy |
|---|---|---|---|---|---|---|---|
| sim/druid_feral | 0.25 s | 3.4 s | 87% | 4% | 280 MB | 63 MB |  |
| sim/druid_tank | 0.17 s | 2.3 s | 88% | 1% | 22 MB | 43 MB |  |
| sim/hunter | 0.16 s | 2.2 s | 87% | 2% | 110 MB | 59 MB |  |
| sim/paladin_holy | 0.00 s | 0.0 s | 72% | 0% | 5 MB | 30 MB |  |
| sim/paladin_protection | 0.03 s | 0.3 s | 81% | 0% | 5 MB | 30 MB |  |
| sim/paladin_retribution | 0.13 s | 1.7 s | 89% | 2% | 55 MB | 55 MB |  |
| sim/raid | 1.04 s | 15.3 s | 90% | 2% | 782 MB | 146 MB |  |
| sim/rogue | 0.46 s | 6.7 s | 91% | 1% | 88 MB | 56 MB |  |
| sim/shaman_elemental | 0.06 s | 0.8 s | 87% | 2% | 31 MB | 44 MB |  |
| sim/shaman_enhancement | 0.26 s | 3.7 s | 91% | 1% | 60 MB | 59 MB |  |
| sim/shaman_restoration | 0.01 s | 0.1 s | 75% | 0% | 8 MB | 33 MB |  |
| sim/warrior_dps | 0.22 s | 3.1 s | 88% | 3% | 188 MB | 60 MB |  |
| sim/warrior_protection | 0.19 s | 2.6 s | 88% | 1% | 58 MB | 57 MB |  |
| statweights/rogue | 7.37 s | 86.3 s | 73% | 1% | 1.06 GB | 81 MB |  |
| bulk/rogue | 3.94 s | 56.3 s | 90% | 1% | 688 MB | 75 MB |  |
| optimizer/quick/combat_rogue_p3 | 16.85 s | 220.2 s | 82% | 1% | 5.67 GB | 186 MB | 76% |
| optimizer/quick/feral_tank_p2 | 9.98 s | 124.6 s | 78% | 1% | 5.50 GB | 135 MB | 59% |
| optimizer/quick/fire_mage_p3 | 3.47 s | 43.1 s | 78% | 4% | 6.22 GB | 179 MB | 78% |
| optimizer/quick/fury_p1 | 7.53 s | 97.5 s | 81% | 2% | 7.00 GB | 162 MB | 77% |
| optimizer/quick/prot_paladin_p3 | 8.29 s | 107.5 s | 81% | 1% | 5.21 GB | 200 MB | 53% |
| optimizer/quick/retribution_p4 | 6.37 s | 80.8 s | 79% | 1% | 4.67 GB | 224 MB | 63% |
| optimizer/normal/combat_rogue_p3 | 190.62 s | 2994.7 s | 98% | 0% | 40.18 GB | 250 MB | 98% |
| optimizer/normal/feral_tank_p2 | 89.05 s | 1356.9 s | 95% | 1% | 36.61 GB | 226 MB | 98% |
| optimizer/normal/fire_mage_p3 | 30.89 s | 411.8 s | 83% | 3% | 51.42 GB | 186 MB | 96% |
| optimizer/normal/fury_p1 | 82.12 s | 1235.7 s | 94% | 1% | 72.83 GB | 244 MB | 97% |
| optimizer/normal/prot_paladin_p3 | 73.65 s | 1129.4 s | 96% | 1% | 45.01 GB | 311 MB | 89% |
| optimizer/normal/retribution_p4 | 56.32 s | 869.9 s | 97% | 1% | 31.18 GB | 312 MB | 94% |

**Scaling:** speedup over the scenario's GOMAXPROCS 1 wall. Normal runs at 16 only. A sim runs GOMAXPROCS-1
shards, so at 2 it runs one.

| Scenario | 1 | 2 | 4 | 8 | 12 | 16 |
|---|---|---|---|---|---|---|
| sim, the 11 with rotations | 1.00 | 0.99 to 1.07 | 2.77 to 3.10 | 5.07 to 6.36 | 6.45 to 7.82 | 7.11 to 8.82 |
| sim/paladin_holy, sim/shaman_restoration (under 0.05 s) | 1.00 | 0.87 to 1.07 | 3.04 to 3.15 | 5.13 to 6.11 | 4.84 to 5.25 | 5.62 to 6.27 |
| statweights/rogue | 1.00 | 1.84 | 3.37 | 5.31 | 5.26 | 6.74 |
| bulk/rogue | 1.00 | 1.94 | 3.72 | 6.32 | 7.67 | 7.13 |
| optimizer/quick/combat_rogue_p3 | 1.00 | 1.90 | 3.60 | 6.12 | 7.23 | 7.65 |
| optimizer/quick/feral_tank_p2 | 1.00 | 1.90 | 3.58 | 6.02 | 6.90 | 7.46 |
| optimizer/quick/fire_mage_p3 | 1.00 | 1.86 | 3.61 | 5.49 | 6.16 | 6.67 |
| optimizer/quick/fury_p1 | 1.00 | 1.89 | 3.65 | 6.27 | 7.19 | 7.67 |
| optimizer/quick/prot_paladin_p3 | 1.00 | 1.88 | 3.51 | 5.98 | 7.11 | 7.47 |
| optimizer/quick/retribution_p4 | 1.00 | 1.91 | 3.59 | 6.15 | 7.10 | 7.49 |

**Optimizer stages** at GOMAXPROCS 16: wall and evaluator busy per stage. Search sims nothing; these requests keep
their racial traits, so the screen is skipped.

| Scenario | Setup | Objective | Racial screen | Stat curves | Effects | Search | Verify | Alternatives |
|---|---|---|---|---|---|---|---|---|
| quick/combat_rogue_p3 | 0.02 s | 0.44 s, 19% | skipped | 4.19 s, 72% | 7.25 s, 94% | 1.43 s | 1.08 s, 87% | 2.45 s, 84% |
| quick/feral_tank_p2 | 0.02 s | 0.25 s, 36% | skipped | 2.68 s, 81% | 2.49 s, 91% | 2.99 s | 0.52 s, 84% | 0.99 s, 95% |
| quick/fire_mage_p3 | 0.01 s | 0.08 s, 18% | skipped | 0.69 s, 60% | 1.57 s, 95% | 0.19 s | 0.38 s, 84% | 0.51 s, 85% |
| quick/fury_p1 | 0.03 s | 0.19 s, 19% | skipped | 1.81 s, 70% | 3.56 s, 95% | 0.63 s | 0.30 s, 86% | 1.00 s, 82% |
| quick/prot_paladin_p3 | 0.03 s | 0.16 s, 36% | skipped | 2.00 s, 81% | 2.19 s, 91% | 3.02 s | 0.10 s, 43% | 0.75 s, 86% |
| quick/retribution_p4 | 0.03 s | 0.15 s, 18% | skipped | 1.51 s, 72% | 2.20 s, 92% | 1.46 s | 0.34 s, 87% | 0.66 s, 88% |
| normal/combat_rogue_p3 | 0.02 s | 1.08 s, 99% | skipped | 31.46 s, 99% | 93.66 s, 100% | 1.88 s | 7.82 s, 99% | 54.69 s, 99% |
| normal/feral_tank_p2 | 0.02 s | 1.36 s, 97% | skipped | 26.39 s, 99% | 37.11 s, 99% | 1.26 s | 4.80 s, 99% | 18.10 s, 99% |
| normal/fire_mage_p3 | 0.02 s | 0.31 s, 94% | skipped | 5.11 s, 97% | 13.15 s, 99% | 0.59 s | 2.25 s, 98% | 9.47 s, 99% |
| normal/fury_p1 | 0.03 s | 0.65 s, 92% | skipped | 18.07 s, 97% | 45.07 s, 100% | 1.35 s | 3.31 s, 99% | 13.64 s, 99% |
| normal/prot_paladin_p3 | 0.03 s | 0.77 s, 98% | skipped | 19.58 s, 99% | 33.07 s, 99% | 7.58 s | 2.68 s, 98% | 9.93 s, 98% |
| normal/retribution_p4 | 0.04 s | 0.41 s, 96% | skipped | 12.20 s, 98% | 28.69 s, 99% | 2.88 s | 2.62 s, 99% | 9.49 s, 99% |

**Top CPU entries** at GOMAXPROCS 16, from each scenario's profile (`go tool pprof -top`): flat share, and the
cumulative entries that explain it.

| Scenario | Flat | Cumulative |
|---|---|---|
| sim/rogue | `APLValueCompare.GetBool` 15.9%, `APLValueAnd.GetBool` 9.5% | `APLRotation.getNextAction` 70%, `Spell.Cast` 15% |
| sim/raid | `Simulation.AddPendingAction` 7.1%, `APLAction.IsReady` 4.2% | the rotation 46%, `Spell.Cast` 29%, `AddPendingAction` 12% |
| statweights/rogue | `APLValueCompare.GetBool` 14.7%, `APLValueAnd.GetBool` 8.2% | `getNextAction` 65%, `Spell.Cast` 21% |
| bulk/rogue | `APLValueCompare.GetBool` 15.4%, `APLValueAnd.GetBool` 9.1% | `getNextAction` 67%, `Spell.Cast` 18% |
| optimizer/quick/fury_p1 | `APLValueCompare.GetBool` 6.9%, `APLAction.IsReady` 6.7% | sims 91%, `Spell.Cast` 44%, `getNextAction` 41% |
| optimizer/quick/prot_paladin_p3 | `runtime.duffcopy` 9.9%, `optimizer.dot` 4.7% | sims 64%; most of the rest is the search's own work |
| optimizer/normal/combat_rogue_p3 | `APLValueCompare.GetBool` 14.2%, `APLValueAnd.GetBool` 8.0% | sims 99%, `getNextAction` 63% |

**Reading:**
- The Simulate button scales 7.1 to 8.8 times at 16 on the rotation specs, at 87 to 91% util, and still gains
  from 12 to 16.
- Stat weights reaches 6.7 at 16 but stalls from 8 to 12 (5.3 at both), at 73% util; its baseline sim runs
  alone before the others start. The bulk sim peaks at 12 (7.7) and does 7.1 at 16.
- Quick optimizer runs reach 6.7 to 7.7 times at 16, at 78 to 82% util and 53 to 78% evaluator busy:
  PERF-OPT's target. The tanks' Search, which sims nothing, is 30 to 36% of their wall (prot paladin 3.0 of
  8.3 s, feral tank 3.0 of 10.0 s), Objective runs 18 to 36% busy, and Stat curves 60 to 81%.
- Normal keeps the evaluator 97 to 100% busy in its long stages (Objective 92 to 99%, util 83 to 98%): its
  ceiling is the machine.
- APL evaluation is 63 to 70% of a rogue's CPU in every path, and `AddPendingAction` 12% of the raid's:
  PERF-HOT's target.

## First readings (wave I2, loaded machine)

Request: `sim/optimizer/testdata/search/fury_p1.json`. The optimizer ran it at Quick through `wowsimcli optimize`,
whose default is one worker per GOMAXPROCS (the web server caps it at 15), while the other I2 items set up.

**The Simulate path used one thread** before PERF-CONC. 3000 Fury iterations through `wowsimcli sim`, and a sim started in the UI and
captured live, both keep 1.0 of 16 threads busy.

**Quick optimizer run:** 9.4 s traced, 12.5 of 16 goroutines busy (78%), 11.7 cores by the CPU profile. Per stage:

| Stage | Wall | Busy of 16 | Util |
|---|---|---|---|
| Setup | 0.02 s | 1.2 | 7% |
| Objective | 0.26 s | 3.1 | 19% |
| Racial screen | skipped (keep current) | | |
| Stat curves | 2.85 s | 11.3 | 70% |
| Effects | 3.68 s | 14.2 | 89% |
| Search | 0.59 s | 10.8 | 67% |
| Verify | 0.86 s | 12.7 | 79% |
| Alternatives | 1.10 s | 13.6 | 85% |

- Stat curves dips to about 6 of 16 for 0.7 s in its middle.
- GC takes 2% of busy time in every stage.

**CPU:** APL evaluation takes 38% cumulative (`APLRotation.getNextAction`; `APLValueCompare`, `APLValueAnd` and
`Spell.CanCast` lead the flat list), `auraTracker.OnSpellHitDealt` 21% cumulative, `mallocgc` 8%.

**Allocation:** the run allocates 6.3 GB. `Dot.startTickAction` 30%, Deep Wounds (`applyDeepWounds.func4`) 17% flat
and 33% cumulative, `DelayedPeriodicApplier.queue` 16%, `Unit.SetGCDTimer` 6.5%, `Pool.gemOptions` 6.4%,
`NewDelayedAction` 4%.

**Load inflates a trace's busy count.** Rerun while another container used 9.2 cores and the worldserver 0.6: 20.2 s,
the trace still showed 10.8 of 16 busy, the CPU profile measured 5.6 cores, and `runtime.asyncPreempt` rose from 3%
to 13% of CPU samples.

**Harness runs** (GOMAXPROCS 16 unless noted, 3 to 8 cores of other load):
- Sims at 3000 iterations run on one thread (util 6 to 7%): the raid 15.3 s, rogue 5.9 s, most specs 2 to 3 s. Holy
  paladin and resto shaman finish in under 0.1 s, having nothing to cast.
- Stat weights 13.2 s at 60% util, the bulk sim 6.5 s at 69%.
- Quick: 12 to 38 s per request, util 34 to 69%, evaluator busy 54 to 81%. Normal combat rogue took 446 s, 14 times
  its Quick run.
- The Search stage sims nothing, so its evaluator busy reads 0: its time is the search itself, 5.2 s of prot
  paladin's 16 s Quick run and 2.6 s of combat rogue's 31 s. Effects keeps the evaluator 91 to 95% busy, Stat curves
  60 to 78%, Objective under 35%.
- Quick fury scales 5.5 times from GOMAXPROCS 1 to 16 (74 s to 13 s, loaded); its Search stage 7 times.
- Comparison check: an unchanged rerun while other load came and went flagged the rogue sim 40% slower. Back to back
  on a quiet machine (only the worldserver, 0.5 cores) it moved 1% and nothing was flagged, while a planted 1.5 s
  spin in `core.RunRaidSimAsync` read 41% slower and left the optimizer's point alone.
