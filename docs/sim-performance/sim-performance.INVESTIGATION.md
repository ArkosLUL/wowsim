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

## PERF-HOT (wave I3)

### APL stage

Base `2f1b8eb7d`. PERF-OPT's optimizer runs kept 8 to 15 of 16 threads busy throughout, so every timing here is an
interleaved A/B with that load beside it.

**Before** (harness `-run '^sim/' -procs 1,16`, GOMAXPROCS 16 profiles): `getNextAction` is 67% cumulative of Rogue
(`APLValueCompare.GetBool` 11 to 18% flat, `APLValueAnd.GetBool` 5 to 8%, `Spell.CanCast` 5%), 60% of Protection
Warrior and 20% of the raid, whose top entry is `AddPendingAction` (16% cumulative).

**Why:** most `APLAction.IsReady` calls evaluate a condition tree for a spell that can't be cast anyway (on cooldown,
on the GCD, mid-cast, short on energy). Per 100 `BenchmarkSimulate` iterations, with the fix below:

| Bench | `getNextAction` passes | `IsReady` calls | Blocked by the gate |
|---|---|---|---|
| sim/rogue | 370k | 7.32M | 5.66M (77%): 3.66M cast blocked, 1.55M energy, 0.45M sequences |
| sim/paladin/retribution | 128k | 838k | 605k (72%) |
| sim/shaman/elemental | 44k | 336k | 209k (62%) |
| sim/warrior/dps | 265k | 3.46M | 1.85M (54%) |
| sim/shaman/enhancement | 979k | 2.59M | 1.34M (52%) |
| sim/hunter | 224k | 758k | 372k (49%) |
| sim (raid) | 2.75M | 7.48M | 3.38M (45%) |
| sim/druid/feral | 828k | 2.48M | 825k (33%) |
| sim/druid/tank | 422k | 2.37M | 529k (22%) |
| sim/warrior/protection | 530k | 4.68M | 870k (19%) |

Feral's rotation is one class action (`catOptimalRotationAction`), outside the gate: 67% of its calls.

**Fix: a gate in `APLAction.IsReady`** (`sim/core/apl_action.go`). When the condition only reads the sim, check first
whether the action can be ready at all:
- Cast and channel actions: `Spell.castBlocked` (hardcast, GCD, cooldowns; `CanCast` calls it too), then
  `aplCostBlocked` (energy short). Past the gate, `isReadyPastGate` finishes `CanCast` without repeating them: the
  extra cast condition, then any cost but energy.
- Sequences: the current subaction's spell. Strict sequences: the GCD, then the first subaction's spell.
- Pure conditions: `aplValueIsPure` (`apl_value.go`), a whitelist of core value types. Class values, Spell Can Cast,
  sequence readiness and Math division (its int and duration panics) stay out.
- Logs on: the long way, so `ExtraCastCondition` log lines stay.

**Why no result moves:**
- Skipping a pure condition skips only reads.
- A blocked spell's `ExtraCastCondition` is skipped too. All 70 only read (`SpellConfig.ExtraCastCondition`'s comment
  now requires it), except: Druid's form check logs (hence logs off); the hunter's trap weave calls Explosive Trap's
  `CanCast`, which fails on the same hardcast first; the owner's Furious Howl calls the pet's `CanCast`, whose only
  write is a focus `CurCast.Cost`.
- The energy check skips `CanCast`'s write to `CurCast.Cost`, which only the spell's own cast reads, after resetting
  it (refunds, the VanCleef 2-piece). Rage, mana (whose check starts OOM events) and the rest stay in `CanCast`.

**Tried and dropped:**
- `APLValueCompare` caching its operand types and a constant right-hand side: Compare's share fell (Rogue 20 to 14%),
  `getNextAction`'s didn't. The interpreter waits on loads (spells, auras), not on dispatch.
- The gate with a rage check and no pass-through (a full `CanCast` after the gate): Protection Warrior ran 11% slower,
  since it spends most passes GCD-ready with nothing to cast. Rage in the gate alone still cost it 8%.

**Checks:** 37 `.results` byte-identical; simval 508/508; `sim/core/apl_gate_test.go`; an assertion build (the long
way on every call, panicking when the gate blocked a ready action or the pass-through disagreed) passed every sim test
and simval with byte-identical goldens, over 6.7M checked calls in Rogue's bench alone.

**`BenchmarkSimulate`**, cpu-sec/op, 12 interleaved rounds, benchstat:

| Package | 1 iteration | 100 iterations |
|---|---|---|
| sim/rogue | -27.0% | -34.6% |
| sim/paladin/retribution | -4.3% | -5.8% |
| sim/hunter | ~ (p=0.06) | -2.6% |
| sim/shaman/elemental | ~ | -8.3% |
| sim (raid) | -3.8% | -7.7% |

At 100 iterations, 10 to 12 rounds, median of the paired ratios (the load swung too much for benchstat): Enhancement
-16%, Warrior DPS -13%, Feral Tank -6%, Protection Warrior -1%, Feral +0.5 to +2% (won 1 to 4 of 10 to 12). Feral's
only gated action is a Berserk cast behind a constant `false`: the gate's `castBlocked` reads the spell's cold fields
where the constant was one call. Leaving constant conditions ungated didn't measure better (+1%, won 1 of 12).

**Harness** (`bench -run '^sim/' -procs 1,16`, base and new back to back for 4 rounds, median paired CPU ratio): at
GOMAXPROCS 1 Rogue -33%, Retribution, Enhancement, Warrior DPS, the raid and Hunter -8 to -9%, Feral Tank and
Elemental -3%; Protection Warrior +2% and Feral +3%, both inside their rounds' spread (0.95 to 1.09). At 16 the same
within noise, Feral +4% (1.00 to 1.07).

**After:** Rogue's `getNextAction` 49% cumulative (And 11%, Compare 7%); the gate itself 14% (`castBlocked` 8%,
`aplCostBlocked` 6%, mostly loads of `Unit.PseudoStats` and `Spell.CostMultiplier`). The raid: `getNextAction` 15%,
`AddPendingAction` 19%.

**Left:**
- Autocast Other Cooldowns runs `getFirstReadyMCD` every pass before its GCD check: 20% of Feral, 9% of Rogue. Checking
  the GCD first is exact only if every MCD's `ShouldActivate` and cost check only reads; mana MCDs' don't.
- With the GCD ready and nothing castable, `DoNextAction` re-polls every 50 ms (`apl.go`), and energy thresholds add
  passes. Fewer passes would move results.

### Core stage

On the APL stage's tree, with PERF-OPT's load beside it most of the time.

**Before** (single-stream `BenchmarkSimulate` profiles unless noted):
- **Pending queue:** `AddPendingAction` 9.5% of the raid at 100 iterations, 18% of the harness's raid at GOMAXPROCS
  16, where the GC runs often: its `copy` of `*PendingAction`s paid bulk write barriers (`bulkBarrierPreWrite` 9%,
  `wbBufFlush1` 7%). The raid queues 17.8k actions an iteration, 39 deep on average, 79% of them `SetGCDTimer`'s;
  single-player queues run 2 to 9 deep.
- **`SetGCDTimer`** allocated a new action whenever the GCD action was still queued: 78% of Rogue's bytes per
  iteration, 45% of Enhancement's, 10% of Warrior DPS's, 6% of the raid's.
- **`partialResistRollThresholds`** rebuilt the same table on every magic hit: 3.3% of the raid.
- **`Spell.doneIteration`** added every spell's metrics for every unit each iteration, nearly all zero (the raid: 334
  spell metrics × 23 units): 3% of the raid.
- **Progress reports** (every 100 ms per shard, the Simulate button's path) built all of `Raid.GetMetrics` for two
  averages, about 1 MB per report in the raid.
- **One iteration:** `NewEnvironment` 15% (Rogue) to 38% (Elemental); `GetMetrics` 2 to 5%, with an allocation per
  action and per target list.

**Fixes**, none moving a result:

| Fix | Why the results can't move |
|---|---|
| The queue holds `pendingEntry{at, prio, slot}`, no pointers, so an insert shifts plain memory and the binary search reads contiguous keys. The actions sit in `Simulation.pendingSlots`, each keeping its slot for the iteration, so requeueing writes no pointer | Same order: an entry's keys are its action's, which nothing changes while queued (the existing rule). `Cleanup` walks the same entries in the same order, and the entries grow at the same lengths the pointers did |
| `SetGCDTimer` requeues a still-queued GCD action instead of replacing it: `detachPendingAction` points its slot at a shared cancelled stand-in | What `Cancel` did to the old action: its entries only get popped. Only for an action without `CleanUp` that holds its slot and isn't the one `Step` popped and is advancing time for: a `Cancel` then stops it running (`Simulation.advancingSlot`) |
| `ResistanceMultiplier` keeps the last average resist's thresholds on the `Simulation` | A pure function of the resist |
| `addSpellMetrics` skips all-zero entries; `Spell.reset` zeroes with `clear` | Every sum starts at +0 and can't reach -0, so adding zero changes nothing |
| Progress reports read the raid's dps and hps means directly | The values `Raid.GetMetrics` returned |
| `UnitMetrics.ToProto` and `auraTracker.GetMetricsProto` allocate a unit's messages in a few slices | The same messages |

**Checks:** 37 `.results` byte-identical; simval 508/508; `sim/core/pending_queue_test.go` (order against a plain list
under random adds, cancels and requeues; `Cleanup` order; the GCD requeue, also from inside `Step`'s advance; slot
ownership), mutation-checked. An assertion build ran the old pointer queue beside the new one on every add, pop and
`Cleanup`, panicking on another action or order, a key unlike its action's or an add during the `Cleanup` walk. It
caught the first requeue in the optimizer's raid evaluator tests: an aura expiring in `Step`'s advance moved the GCD
whose action `Step` had just popped, so that action ran then and again at the new time; the goldens never hit it.
With the guard, every sim test, simval and ten `BenchmarkSimulate` packages pass under it, goldens byte-identical.

**Each fix alone**, cpu-sec/op, 8 to 12 interleaved rounds, median paired ratio:
- Queue, first with a free list of slots: single stream 0.998 to 1.005 on the raid, Retribution and Hunter; harness at
  GOMAXPROCS 16, idle, the raid 0.946, Rogue 0.946, Hunter 0.963. But it cost the shallow queues: Warrior DPS 1.074
  and Bear 1.115 (0 of 10 won, idle). Sticky slots against the free list: Warrior DPS 0.957, Protection Warrior
  0.984, Rogue and the raid 0.985; against the APL stage's queue: Bear 1.005 (4/12), Protection Warrior 0.996,
  Warrior DPS 0.985.
- Resist cache, zero skip, `clear` (idle, 8/8 won): at 100 iterations Elemental 0.881, Retribution 0.921, raid 0.938,
  Hunter 0.975, Rogue 0.984.
- GCD requeue: Rogue 0.950 (6/8), Enhancement 0.969 (7/8), the rest neutral. Bytes per 100 iterations: Rogue
  2.96 → 0.68 MB, Enhancement 2.19 → 1.17 MB.

**Tried and dropped:** walking the last 8 entries before the binary search: 0.99 to 1.02 at 100 iterations, 1 to 2%
slower at 1.

**Left:**
- Dot ticks: each application allocates an action and two closures (21% of the raid's bytes per iteration, 35% of
  Warrior DPS's). Requeueing one breaks the tick's `dot.tickAction != pa` check when a tick refreshes its own dot.
- `DelayedPeriodicApplier.queue` and Deep Wounds (`sim/warrior`) allocate per proc: 18% of Warrior DPS's bytes each.
- `Spell.NewResult` allocates when the spell's cached result is in use: 15% of the raid's bytes. A pool needs every
  caller to dispose of its results.
- `NewSim` builds raid stats and metadata that `NewEnvironment` returns and it drops: `GetMetadata` 2.6% of Elemental
  at 1 iteration. Skipping only the metadata takes a flag into `Character.FillPlayerStats` (`character.go`, PAR-P7-0f's
  in I3); its build-phase aura round trip has to stay.
- `NewPet` returns the 35 KB `Pet` by value, 19 KB of it the `Equipment` pets never use: a pointer means 15
  constructors in 14 class files, for ≤1% of a 1-iteration sim.
- The rest of a 1-iteration `NewEnvironment` is class construction and `applyAllEffects` (7.5% of Elemental).
- Under PERF-OPT's load, `runtime.asyncPreempt` took 15 to 35% of a harness profile's samples: read shares from an idle
  machine.

### Before and after

Base `2f1b8eb7d` against both stages; in parentheses, the APL stage against both (the core stage alone). Measured
before the `advancingSlot` guard, which costs nothing measurable: 0.995 to 1.002 against it (6 rounds, idle).

**`BenchmarkSimulate`**, cpu-sec/op, 12 interleaved rounds as PERF-OPT's load went from 15 cores to none, so
benchstat's spreads run 50 to 90%: median paired ratio (all won 10 to 12 of 12, except the core stage's Hunter at 1
iteration, 9/12), and benchstat's change (p ≤ 0.02):

| Package | 1 iteration | 100 iterations |
|---|---|---|
| sim/rogue | 0.669, -33% (0.972) | 0.589, -41% (0.957) |
| sim/paladin/retribution | 0.908, -9% (0.948) | 0.842, -16% (0.901) |
| sim/hunter | 0.979, -2% (0.990) | 0.949, -6% (0.976) |
| sim/shaman/elemental | 0.929, -7% (0.953) | 0.805, -20% (0.888) |
| sim (raid) | 0.935, -7% (0.984) | 0.862, -14% (0.944) |

Bytes per op at 100 iterations: Rogue -76%, the raid -6%, Hunter -4%. At 1 iteration up to 3% more: the slot table
and 16-byte entries, once per sim.

**Harness** (`bench -run '^sim/' -procs 1,16`, 3 rounds back to back, idle machine), median paired CPU ratio at
GOMAXPROCS 1 / 16:

| Scenario | Both stages | Core stage |
|---|---|---|
| sim/rogue | 0.583 / 0.632 | 0.937 / 0.953 |
| sim/shaman_elemental | 0.778 / 0.817 | 0.847 / 0.893 |
| sim/shaman_enhancement | 0.837 / 0.827 | 0.934 / 0.915 |
| sim/paladin_retribution | 0.837 / 0.868 | 0.906 / 0.920 |
| sim/raid | 0.851 / 0.837 | 0.920 / 0.925 |
| sim/paladin_protection | 0.858 / 0.915 | 0.854 / 0.931 |
| sim/warrior_dps | 0.865 / 0.883 | 1.004 / 0.991 |
| sim/hunter | 0.946 / 0.923 | 0.958 / 0.934 |
| sim/druid_tank | 0.950 / 0.933 | 1.005 / 0.996 |
| sim/druid_feral | 0.952 / 0.919 | 0.940 / 0.919 |
| sim/warrior_protection | 1.000 / 1.031 | 1.027 / 1.024 |

Every ratio won 3 of 3, except Warrior DPS and Bear in the core column (1 or 2 of 3) and Protection Warrior (0 or 1 of
3): likely the slot lookup, paid on its 4.9k adds an iteration to a queue 2 deep, where the old one cost nothing to
shift. Holy Paladin and Restoration Shaman, under 0.05 s, stay within noise. Allocated bytes: the raid 783 → 582 MB at
16, Rogue 88 → 15 MB.

**Review re-run**, the final tree (guard included) against base, idle:
- `BenchmarkSimulate` cpu-sec/op, 12 interleaved rounds, benchstat's spreads ±1%, every change p=0.000 and won 12/12,
  at 1 / 100 iterations: Rogue -33 / -42%, Retribution -9 / -16%, Hunter -2 / -5%, Elemental -7 / -20%, the raid
  -7 / -13%.
- Harness, 4 rounds, CPU ratio at GOMAXPROCS 1 / 16: Rogue 0.59 / 0.62, Elemental 0.80 / 0.80, Retribution
  0.84 / 0.88, Enhancement 0.84 / 0.84, the raid 0.85 / 0.85, Protection Paladin 0.85 / 0.89, Warrior DPS
  0.86 / 0.92, Hunter 0.94 / 0.95, Feral 0.96 / 0.97, Bear 0.97 / 1.02 (1/4 won at 16), Protection Warrior
  0.98 / 1.03 (0/4 won at 16).

**Top entries after**, cumulative, harness at GOMAXPROCS 16, idle, base → both stages: the raid's `getNextAction`
21.6 → 20.3%, `AddPendingAction` 12.9 → 8.8% (`bulkBarrierPreWrite` 2.3% → gone), `SetGCDTimer` 9.9 → 7.0%,
`ResistanceMultiplier` 7.4 → 3.5%; Rogue's `getNextAction` 68 → 55%; `ResistanceMultiplier` 24 → 8% of Elemental,
12 → 4% of Retribution.
