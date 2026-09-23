# Sim and optimizer performance: investigation

Findings behind [sim-performance.PLAN.md](sim-performance.PLAN.md), taken with [tools/perf](../../tools/perf/README.md) on
the Ryzen 7 7800X3D (16 threads, GOMAXPROCS 16). Wave timings are skewed by the items running alongside; targets
come from the integration baseline (PERF-TOOLS item 5).

## Baseline (integration, idle machine)

Method: the harness's [full baseline](../../tools/perf/README.md#full-baseline), with nothing else running; its
[columns](../../tools/perf/README.md#scenario-harness) are the ones below. Its `report.json` becomes
`tools/perf/baseline.json`.

- Commit: _
- Load: worldserver _ cores (`docker stats`); load average _ at the start, _ at the end
- Wall time: _

**Utilization** at GOMAXPROCS 16, medians. Busy is the optimizer evaluator's.

| Scenario | Wall | CPU | Util | GC | Alloc | Peak heap | Busy |
|---|---|---|---|---|---|---|---|
| sim/druid_feral | | | | | | | |
| sim/druid_tank | | | | | | | |
| sim/hunter | | | | | | | |
| sim/paladin_holy | | | | | | | |
| sim/paladin_protection | | | | | | | |
| sim/paladin_retribution | | | | | | | |
| sim/raid | | | | | | | |
| sim/rogue | | | | | | | |
| sim/shaman_elemental | | | | | | | |
| sim/shaman_enhancement | | | | | | | |
| sim/shaman_restoration | | | | | | | |
| sim/warrior_dps | | | | | | | |
| sim/warrior_protection | | | | | | | |
| statweights/rogue | | | | | | | |
| bulk/rogue | | | | | | | |
| optimizer/quick/combat_rogue_p3 | | | | | | | |
| optimizer/quick/feral_tank_p2 | | | | | | | |
| optimizer/quick/fire_mage_p3 | | | | | | | |
| optimizer/quick/fury_p1 | | | | | | | |
| optimizer/quick/prot_paladin_p3 | | | | | | | |
| optimizer/quick/retribution_p4 | | | | | | | |
| optimizer/normal/combat_rogue_p3 | | | | | | | |
| optimizer/normal/feral_tank_p2 | | | | | | | |
| optimizer/normal/fire_mage_p3 | | | | | | | |
| optimizer/normal/fury_p1 | | | | | | | |
| optimizer/normal/prot_paladin_p3 | | | | | | | |
| optimizer/normal/retribution_p4 | | | | | | | |

**Scaling:** speedup over the scenario's GOMAXPROCS 1 wall. Normal runs at 16 only.

| Scenario | 1 | 2 | 4 | 8 | 12 | 16 |
|---|---|---|---|---|---|---|
| sim (all 13; list any that differ) | 1.00 | | | | | |
| statweights/rogue | 1.00 | | | | | |
| bulk/rogue | 1.00 | | | | | |
| optimizer/quick/combat_rogue_p3 | 1.00 | | | | | |
| optimizer/quick/feral_tank_p2 | 1.00 | | | | | |
| optimizer/quick/fire_mage_p3 | 1.00 | | | | | |
| optimizer/quick/fury_p1 | 1.00 | | | | | |
| optimizer/quick/prot_paladin_p3 | 1.00 | | | | | |
| optimizer/quick/retribution_p4 | 1.00 | | | | | |

**Optimizer stages** at GOMAXPROCS 16: wall and evaluator busy per stage.

| Scenario | Setup | Objective | Racial screen | Stat curves | Effects | Search | Verify | Alternatives |
|---|---|---|---|---|---|---|---|---|
| quick/combat_rogue_p3 | | | | | | | | |
| quick/feral_tank_p2 | | | | | | | | |
| quick/fire_mage_p3 | | | | | | | | |
| quick/fury_p1 | | | | | | | | |
| quick/prot_paladin_p3 | | | | | | | | |
| quick/retribution_p4 | | | | | | | | |
| normal/combat_rogue_p3 | | | | | | | | |
| normal/feral_tank_p2 | | | | | | | | |
| normal/fire_mage_p3 | | | | | | | | |
| normal/fury_p1 | | | | | | | | |
| normal/prot_paladin_p3 | | | | | | | | |
| normal/retribution_p4 | | | | | | | | |

**Top CPU entries** at GOMAXPROCS 16, from each scenario's profile (`go tool pprof -top`): flat share, and the
cumulative entries that explain it.

| Scenario | Flat | Cumulative |
|---|---|---|
| sim (per class where they differ) | | |
| statweights/rogue | | |
| bulk/rogue | | |
| optimizer/quick (per request where they differ) | | |
| optimizer/normal (per request where they differ) | | |

## First readings (wave I2, loaded machine)

Request: `sim/optimizer/testdata/search/fury_p1.json`. The optimizer ran it at Quick through `wowsimcli optimize`,
whose default is one worker per GOMAXPROCS (the web server caps it at 15), while the other I2 items set up.

**The Simulate path uses one thread.** 3000 Fury iterations through `wowsimcli sim`, and a sim started in the UI and
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
