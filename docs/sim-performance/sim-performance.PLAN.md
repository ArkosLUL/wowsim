# Sim and optimizer performance pass

## Context

The user asked (2026-09-22) for a thorough performance pass over the sim and the BiS optimizer: check that
they use the machine, find the bottlenecks, and add tooling that keeps finding them.

- **Machine:** Ryzen 7 7800X3D, 8 cores and 16 threads; Docker Desktop's VM has all 16 and 32 GB. The prod web
  server (container `wowsims-wotlk`, `/wowsimwotlk --launch=false --host=:3333`) has no CPU limit, so Go runs
  with GOMAXPROCS 16. The worldserver shares the machine: its playerbots took up to 1.5 threads in earlier
  measurements.
- **Earlier passes:** PAR-PERF and PAR-PERF-2 ([parity PLAN](../azerothcore-parity/azerothcore-parity.PLAN.md),
  findings and leftovers in the [parity INVESTIGATION](../azerothcore-parity/azerothcore-parity.INVESTIGATION.md)'s
  Performance section); optimizer costs in the [BiS INVESTIGATION](../bis-optimizer/bis-optimizer.INVESTIGATION.md)'s
  Performance section; throughput per wave in the [wave-loop PLAN](../wave-loop/wave-loop.PLAN.md#sim-throughput).
- These are loop work items: the [RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md) agent rules apply. Findings go in
  `sim-performance.INVESTIGATION.md` (new).

## Thread use today (from the code)

| Path | Threads busy | Why |
|---|---|---|
| Simulate button (`/raidSimAsync`) | up to 15 of 16 | `core.RunRaidSimAsync` shards the iterations over GOMAXPROCS-1 goroutines (PERF-CONC) |
| Stat weights | 30 goroutines | `statweight.go`: `(GOMAXPROCS-1)*2`; 8 and 16 were no faster |
| Bulk sim | 16 goroutines | `bulksim.go`: GOMAXPROCS, which beat 8 and 17 |
| Optimizer | up to 15 | `sim/web/main.go` caps `Settings.Workers` at GOMAXPROCS-1. `SimEvaluator.Evaluate` runs 250-iteration shards, which Quick splits into five 50-iteration sims, and a prefetch sims the screens and early curve knots during Objective (PERF-OPT). Idle at 16 threads, Quick keeps the evaluator 56 to 89% busy and Normal 88 to 98%; Search, which sims nothing, is the least busy stage |
| Wasm (static hosting) | 1 | `ui/core/sim.ts`: `WorkerPool(1)`; GOMAXPROCS 1 gives one shard |

16 threads do about 8× one on the optimizer's sims (SMT adds little), so 8 is the real ceiling to scale against.

## Work items

All golden-neutral: a fix that changes results becomes its own item, as PAR-PERF-2 was.

### PERF-TOOLS: bottleneck tooling and a baseline (wave I2, done)

1. **Profile a live run.** Put pprof behind a `--pprof <addr>` flag, off by default, with mutex and block
   profiling. Add `--cpuprofile`, `--memprofile` and `--trace` to `wowsimcli sim` and `optimize`. Script a live
   capture (CPU profile plus `runtime/trace` over N seconds) for a run started from the UI.
2. **Utilization from a trace.** A `tools/perf` command reads a `runtime/trace` (`golang.org/x/exp/trace`) and
   prints, per 100 ms, busy threads over GOMAXPROCS and the GC's share. The optimizer's stages open trace regions,
   so its lines say which stage idled.
3. **Scenario harness** in `tools/perf`, run through `dock.sh`:
   - Scenarios: each `BenchmarkSimulate` spec's request at the UI's 3000 iterations, the raid bench's raid, stat
     weights and a bulk sim on one spec, and the optimizer at Quick and Normal on `slow_test.go`'s six requests.
   - Per scenario: wall time, CPU time, utilization (CPU / wall / GOMAXPROCS), GC CPU share, bytes allocated,
     peak heap. Per optimizer stage: wall time, sims, and the evaluator's busy fraction (a busy-time counter in
     `SimEvaluator`'s worker loop).
   - A GOMAXPROCS and workers sweep (1, 2, 4, 8, 12, 16) gives each scenario's scaling curve.
   - Writes a JSON report and compares it with a committed baseline, flagging moves past the run-to-run noise.
4. `benchstat` in the toolchain image (the root `Dockerfile`'s `toolchain` stage), and
   [testing.md](../guide/testing.md)'s `BenchmarkSimulate` comparison through it.
5. **Baseline.** The orchestrator runs the harness at integration, with nothing else running and the
   worldserver's load read from `docker stats`: a wave's parallel agents skew any timing taken inside it. It
   records utilization, scaling and each scenario's top pprof entries in the INVESTIGATION, which sets
   PERF-OPT's and PERF-HOT's targets.

Owns: `tools/perf/**`, `cmd/wowsimcli/cmd/`, `sim/web/main.go` (the pprof flag), `sim/optimizer/{evaluator,api}.go`
(the busy counter and trace regions only), the `Dockerfile` toolchain stage, testing.md's benchmark lines, the new
INVESTIGATION.

**As built:** everything runs from [tools/perf](../../tools/perf/README.md). pprof now answers only with
`--pprof <addr>`, on its own listener; the sim's port 404s it (`sim/web/pprof_test.go`). The harness's
requests are pinned in `tools/perf/harness/scenarios/` and its baseline is `tools/perf/baseline.json`,
read in the [INVESTIGATION](sim-performance.INVESTIGATION.md).

### PERF-CONC: the Simulate button on every thread (wave I2, done)

- Split a raid sim's iterations into shards on up to GOMAXPROCS-1 goroutines, seeded `RandomSeed + k·shard` as the
  optimizer's evaluator does, and merge the results: every distribution (mean, stdev, min, max, histogram), action,
  aura and resource metrics, and shard 0's first-iteration log and timeline. Progress sums the shards. Upstream's
  newer sims (wowsims/cata) split and combine raid sim requests in Go for their multi-worker wasm sims: read it
  before designing the merge.
- `core.RunRaidSimAsync` uses it. `core.RunRaidSim` stays one stream for the goldens and tools; wasm, at
  GOMAXPROCS 1, gets one shard.
- One seed and worker count gives one result. Results move from today's only through their random streams.
- Tests: one shard equals `RunRaidSim` byte for byte; N merged shards equal a hand merge; the means agree with one
  stream within noise.
- Stat weights and bulk sim: time them at 8, 16 and today's goroutine counts, and keep the fastest.
- Verify: 3000 iterations on each bench spec and the raid, before and after (about 8× expected), and a
  `tools/uicheck` pass that the results page renders (histogram, metrics tables, timeline).

Owns: `sim/core/api.go`, a new `sim/core` file for split and merge plus tests, the concurrency lines of
`statweight.go` and `bulksim.go`.

**As built:** `sim/core/sim_shards.go`. A shard seeds from `RandomSeed` plus its first iteration, so each
iteration, and the max and min seeds, match one stream's. Idle, 3000 iterations run 7.1 to 8.8× faster at 16
threads than at one ([baseline](sim-performance.INVESTIGATION.md)). Left open:
- Every shard runs its own presim: CPU, not wall time.
- A failed shard leaves the others running unread: `sim.run` has no cancel hook.
- Bulk sim, before I2: after a failed combo the remaining sims can block on the 10-slot `results` buffer,
  its error message dereferences a nil `Result`, and both `Run` and `BulkSim` send a `FinalBulkResult`.

### PERF-OPT: keep the optimizer's threads busy (wave I3, done)

- From PERF-TOOLS' per-stage busy fractions: batch what now runs one evaluation at a time (bisection brackets,
  the racial screen, verify and neighborhood rounds), and cut shards smaller when a batch has fewer jobs than
  workers. Keep the pairing (compared points share shard seeds) and the evaluation cache.
- Set the default worker count from the scaling curve.
- In raid mode (the batch's stage 2) J is the raid's DPS, so `setScreen` (5% of the seed's J, `surrogate.go`)
  admits every set with two pieces in the pool and sims each with whole-raid sims: a DK's stage 2 simmed
  Tidefury Raiment, a shaman caster set. Screen against the raider's own share of J, and report stage 2's set
  sims before and after.
- Takes over BIS-e2e-perf's "time whole runs" bullet ([BiS PLAN](../bis-optimizer/bis-optimizer.PLAN.md)).
- Smaller shards change the random streams: the slow suite's picks and gains must hold within noise.
- Verify: the optimizer tests, the slow suite, and the harness per stage before and after.

Owns: `sim/optimizer/**`, `sim/web/main.go`'s worker cap.

**As built, stage 1 (busy threads)** ([findings](sim-performance.INVESTIGATION.md#perf-opt-wave-i3-loaded-machine)):
- Quick sims every shard as 5 sims of 50 iterations (`shardSpans`, `evaluator.go`), whatever the batch: the layout
  follows the effort, so a cached point extends shard by shard and results don't depend on the batch or the worker
  count. Normal and Thorough, 16 and 40 shards an evaluation, don't split.
- `prefetch` (`api.go`) sims into the evaluator's cache ahead of need: the seed and both screens during the
  Objective stage, and `addKnots`' knots for stats that aren't cap-prone during bisection. It changes no result.
- The web server's cap stays GOMAXPROCS-1: 15 and 16 workers tie.
- Left open: Search, which sims nothing, is now the least busy stage, at 62 to 75% util.

**As built, stage 2 (raid screen)** ([findings](sim-performance.INVESTIGATION.md#perf-opt-wave-i3-loaded-machine)):
- `setScreen` sizes against `ownJ` (`api.go`): the seed's J, but in raid mode only the target's own DPS in J's
  points, which the raid evaluator keeps beside the raid's (`Evaluation.ownDPS`). Raid25's combat rogue simmed 11
  sets in 21,000 whole-raid iterations; now its 4 tier sets in 5,000 to 6,000.
- Left open: raid mode's acceptance bar (`acceptFraction`, `api.go`) still takes 0.05% of the whole raid's J, 15
  times the rogue's own share.

### PERF-HOT: single-thread hot paths (wave I3, done)

- Profile again: the class waves added cost (H3 ran 0.4-5% over H). Candidates from the parity INVESTIGATION:
  APL interpretation (`getNextAction` 71% of Rogue at 100 iterations, mostly `APLValueCompare` and
  `APLValueAnd`), `AddPendingAction` 4.5-6.4% (scattered pointers, cancelled actions queued until popped),
  metrics `ToProto` 3-5% at one iteration, `NewEnvironment` 13-37% of one-iteration cases, `NewPet` by value.
- Verify: all 37 goldens byte-identical, the simval replay, benchstat before and after, a throughput table row.

Owns: `sim/core` hot paths, and a class file only where its profile names a hot spot; not `applyAllEffects` or
the item-set and item-effect code, which PAR-P7-0f owns in I3.

**As built** ([INVESTIGATION](sim-performance.INVESTIGATION.md#perf-hot-wave-i3): each fix, why no result moves, the
numbers, what's left):
- APL: `APLAction.IsReady` skips a pure condition when its spell can't be cast anyway, and past that gate finishes
  `CanCast` without repeating it (`sim/core/apl_action.go`). Every `ExtraCastCondition` must only read the sim.
- Core: the pending queue holds pointer-free entries over a slot table, so inserts pay no GC write barriers;
  `SetGCDTimer` requeues a still-queued GCD action instead of allocating one (`sim/core/sim.go`). The partial resist
  table is cached, spells skip zero metrics, and progress reports stop building the whole metrics proto.
- Idle, `BenchmarkSimulate` at 100 iterations: Rogue -42%, Elemental -20%, Retribution -16%, the raid -13%, Hunter
  -5%. The harness's CPU at GOMAXPROCS 16: Rogue 0.62, the raid 0.85, the other rotation specs 0.80 to 0.97, but Bear
  1.02 and Protection Warrior 1.03.

### PERF-MISSILE: missiles without allocations (wave I6, done)

Wave I5's travel waits made every missile allocate. Idle, `BenchmarkSimulate` at 100 iterations went up 50%
for Elemental and 13% for Hunter over I4; a 100-iteration Elemental op allocates 139k objects, not 34k (7.2 MB,
not 1.2) ([throughput](../wave-loop/wave-loop.PLAN.md#sim-throughput)). Per missile:
- `Spell.WaitTravelTime` (`sim/core/spell_result.go`) allocates a `PendingAction` and a `pendingSlots` entry
  through `StartDelayedAction`, plus the caller's closure.
- `NewResult` allocates while an earlier missile holds the spell's `resultCache` (Lightning Bolt, Searing
  Totem). A missile in flight at an iteration's end leaves the cache held for every later iteration, since
  `Spell.reset` doesn't free it; freeing it there alone saves 1.6%.

Fix it in core: reuse landing actions and results, reset each iteration. A result is reused only after
`DisposeResult`, so check that no caller reads one after that. Change `WaitTravelTime`'s call sites only if
the A/B shows their closures cost enough. Profile Elemental at one iteration too (+18% over I4).

- Verify: all 37 goldens byte-identical, the simval replay, and an interleaved benchstat A/B with allocs/op
  of Elemental, Hunter and the raid against the wave base ([testing](../guide/testing.md#go)).
- Owns: `sim/core/**` except `sim/core/serverdata/`; the `WaitTravelTime` call lines in `sim/**`, which may
  sit in PAR-DECL-4's files (settled at merge).

**As built** ([INVESTIGATION](sim-performance.INVESTIGATION.md#perf-missile-wave-i6): the profile, each fix, why no
result moves, the numbers, what's left):
- `WaitTravelTime` queues a pooled landing (`Simulation.startLanding`, `sim/core/sim.go`), and
  `Spell.DealDamageAfterTravel` does it without a closure for the 20 call sites whose landing only deals the result.
  `NewResult` hands out per-spell spares from a free list, `DisposeResult` returns them, and `Spell.reset` frees the
  cache and every spare (`sim/core/spell_result.go`): a disposed result is only safe to read until its spell's next
  `NewResult`.
- `BenchmarkSimulate` at 100 iterations, loaded machine: Elemental -13% (allocations -59%), Hunter -5% (-38%), the
  raid flat on the wall (-40%) but -2.4% at `-test.cpu 1`. Elemental still runs 29% over I4, mostly the extra
  queued actions and rotation passes I5 brought.

### PERF-RNG: a random stream per unit for the optimizer (wave J2, done)

`IsTest` rolls draw from `Simulation.testRands`, a map keyed by label alone (`labelRand`, `reseedRands`,
`makeTestRandSeed` in `sim/core/sim.go`) that every unit shares. In a raid, a target whose gear changes its
number of rolls shifts every other unit's later ones, so raid-mode paired deltas carry ±35–62 raid DPS where
solo ones carry ±7–13 ([speed audit](../bis-optimizer/bis-optimizer.INVESTIGATION.md#speed-audit-after-wave-j)).
Every roll also looks up the map, and reseeding hashes each label again.
- An additive `SimOptions` field (`proto/api.proto`) keys `IsTest` streams by unit and label, pets and
  guardians included. Only the optimizer's evaluator sets it; off, every stream and draw stays as today.
- Resolve each label's stream once, so a roll looks up nothing and reseeding allocates nothing. Same seeds,
  same draw order.

- Verify: all 37 goldens byte-identical, the simval replay, and the optimizer's tests. The SE of a paired
  raid-mode delta on `raidctx`'s raid25 (+100 hit rating on the target, 1,000 iterations) with the field on
  and off. An interleaved benchstat A/B of `BenchmarkSimulate` for the lookups
  ([testing](../guide/testing.md#go)). The slow suite runs; report what moved.
- Owns: `sim/core/**` except `sim/core/serverdata/`, `SimOptions` in `proto/api.proto`, and the request the
  evaluator builds in `sim/optimizer/evaluator.go`.

**As built** ([INVESTIGATION](sim-performance.INVESTIGATION.md#perf-rng-wave-j2): the profile, why a field on
`SpellEffect` didn't fly, the numbers, what's left):
- `SimOptions.PerUnitRandomSeeds` (`proto/api.proto`, additive); only `SimEvaluator.request` (`evaluator.go`)
  sets it. `Simulation.UnitRandomFloat(unit, label)` resolves a stream from `unit.testRands` when it's on,
  else falls through to the shared `sim.testRands` stream, same as `RandomFloat`. Both maps hold a
  `*testRandStream` caching the label's (and, per unit, the unit's index's) FNV-1a hash, so a reseed mixes in
  the new seed without rebuilding or rehashing the label.
- Converted every `sim/core` roll with a unit or spell already in scope: crit, hit and resist checks
  (`spell_outcome.go`, `spell_result.go`, `spell_resistances.go`), `aura_helpers.go`'s two proc-chance rolls,
  the named procs in `buffs.go`, `consumes.go`, `debuffs.go`, `energy.go`, `health.go`, and `Weapon`'s two
  damage rolls (`attack.go`; `Weapon` now carries a `unit`, stamped by `EnableAutoAttacks`/`setWeapon`).
- Left shared even with the option on: `SpellEffect.Roll`'s "Damage Roll" (a `unit` field there made
  `sim/serverdata_test.go`'s `SpellEffect{...}` equality checks fail outside this WI's owned paths; reverted),
  `PPMManager.Proc`/`AutoAttacks.PPMProc` (14 files outside `sim/core` call them directly), and
  `Simulation.Roll`/`RollWithLabel`/`Proc`/`RandomExpFloat`, the generic passthroughs 35+ class and encounter
  files call with their own labels.
- Paired raid-mode SE (`raidctx`'s raid25, +100 hit/spell hit on one raider, 1,000 iterations, loaded
  machine): 27.1 off, 4.2 on, about 6.5× down (~42× fewer iterations for equal precision, inside the
  16-50× the option table estimated). 37 goldens byte-identical, simval 508/508, `go vet` and `gofmt` clean.
  `BenchmarkSimulate` (`IsTest` off, so untouched by the change) held within noise, interleaved.

### PERF-FU: the racial search and the harness's length (wave N, done)

Two follow-ups from wave L, file-disjoint.

**Per-race search.** BIS-caps-race runs one gear search per racial finalist (`searchRaces`, `racial.go`;
[as built](../bis-optimizer/bis-optimizer.PLAN.md#as-built-racial-traits-under-the-caps)). Idle P2 Normal jobs
went from 28 to 48 s for Stormweaver (elemental, 6 finalists, 34% more sims) and from 76 to 120 s for Mighty
(arms), whose base may have read fast (119 s under load on `6c79a6a62`). Profile it and cut what doesn't change
picks: race-independent work each search repeats, and `racialBounds`, which widens every weapon's prune bounds
by the most a specialization can win or lose, so pruning under an Orc, Human or Dwarf expertise floor is loose.
Picks stay identical; where tighter pruning moves the search's path, every floor is still met and J holds within
noise, reported per slow-suite line.

**Full baseline.** The harness's full run takes over 2 h since the raid25 scenarios joined (raid DPS Quick 39 min
at one proc; [README](../../tools/perf/README.md#full-baseline)). Bring it to about an hour, keeping the widest
points the comparison reads, e.g. fewer procs for the raid25 scenarios. Update the README's defaults and estimate.

- Verify: the optimizer's tests and the slow suite. Both jobs before and after, on your own sim server through
  the [acbis driver](../../tools/database/acbis/driver/README.md): `--stage 1 --phases 2 --effort Normal
  --raiders Mighty,Stormweaver --sources raid25,raid25-heroic,vendor,crafted,quest,reputation,achievement,prospecting,world-drop`,
  roster `C:\Users\boss2\AppData\Local\Temp\claude\g--DevStuff-GitHub-wowsimwotlk\bis-caps-evidence\raid.json`,
  interleaved if the machine is busy. The harness's new full-run time.
- Owns: `sim/optimizer/**` except `types.go`, `tools/perf/**`, its INVESTIGATION section.

**As built** ([INVESTIGATION](sim-performance.INVESTIGATION.md#perf-fu-wave-n)): static pruning never fired under a
floor (`slopeBox` gives the floor's stat ub +Inf), so `racialBounds` cost nothing and stays. Three cuts, every
`OptimizerResult` bit-identical: the surrogate kernel skips zero stats and reads each curve once per pass; a
state-aware `moveBound` lets `bestItem` and `bestPartner` skip moves that can't win (arms search CPU 2014 → 26 s);
gem kinds group once per pool. Captured P2 Normal jobs: Mighty 136 → 56 s, Stormweaver 52 → 40 s. The harness
runs raid25 Quick at `-raid-procs 8,16` and skips raid DPS at Normal (55 min) by default, kept by the user (2026-10-02): 46 min idle at O.

### PERF-COST: the cost check off the hot path (wave V, done)

EC-COST-FIX (wave T2) runs the server's cost arithmetic (`costFor`, `sim/core/cost.go`: three truncations and a
school lookup) on every `ApplyCostModifiers` call, and the APL calls that for each energy action on every
readiness check (`aplCostBlocked`, `sim/core/apl_action.go`). Idle, `BenchmarkSimulate` runs Combat Rogue 25% over
Q's at 100 iterations, 32% at one; a CPU profile puts 42% of the rogue's time there. The other cases held
([throughput](../wave-loop/wave-loop.PLAN.md#sim-throughput)).

Fix it in core, e.g. resolve each spell's cost school once and cache its computed cost until an input changes
(the base, the spell's and unit's percents and flats, the school's two auras). Check the other hot callers too:
`StaticCost` in the energy and rage rotations, `MeetsRequirement`.

- Verify: all 37 goldens byte-identical, every replay and the simval replay; an interleaved benchstat A/B of
  Combat Rogue, Hunter, Elemental and the raid against the wave base ([testing](../guide/testing.md#go)); a
  profile of Feral (energy) too.
- Owns: `sim/core/cost.go`, `cost_test.go`, `apl_action.go`'s cost checks, `spell.go`'s cost fields.

#### As built

- `Spell.costCache` (`sim/core/cost.go`) keeps the last `ApplyCostModifiers` result with every input it read: the
  base, the spell's and unit's `CostMultiplier`/`CostFlat`, the school's flat and multiplier, and the `SpellSchool`
  its school was resolved from. Class code writes those fields directly, so no hook can bump a version; each read
  compares them instead, and a hit runs no call. Spark of Hope and the per-iteration `PseudoStats` copy are covered
  the same way. `setStaticCost` clears it. A miss resolves the school (server data for schoolless spells) only when
  `SpellSchool` changed, then reruns `costFor`. `costFor` treats -0 and +0 alike, so the memo is exact.
- No call site changed: `MeetsRequirement`, `aplCostBlocked` and `CurrentCost` all go through `ApplyCostModifiers`.
  `StaticCost`/`StaticCostWithPercent` stay uncached: 0.9% of Feral's profile.
- Tests (`cost_test.go`): each input changed after a cached read, and a 20,000-step random walk (signed zeros,
  school and static changes) bit-equal to the uncached cost; dropping any compare fails them.
- 37 goldens byte-identical, simval 1114/1114, `go vet` and `gofmt` clean.
- Interleaved A/B against `16bdd941c`, 12 rounds, `BenchmarkSimulate` (benchstat medians, loaded machine): Rogue
  -10.1% at 1 iteration, -11.0% at 100; the raid -2.8% at 100, flat at 1; Hunter and Elemental flat. Allocations
  unchanged.
- Profiles, `tools/perf bench` at 3000 iterations, one proc, base → new: Rogue's cost path (`aplCostBlocked`
  cumulative) 34.3% → 9.2%, `costFor` 21.0% → 0; Feral's `ApplyCostModifiers` 2.1% → under the 0.45% sample floor,
  `costFor` left at 1.35%, two thirds from `StaticCost` callers.

### PERF-THREAT: per-cast threat bookkeeping off the hot path (wave Y, done)

EC-THREAT-FIX (wave V) keeps a threat record per cast (`openThreatCast`, `closeThreatCast`, `addAssistThreat`,
`sim/core/threat.go`). Idle at 100 iterations, Elemental runs 7.1%, Ret 6.2% and Hunter 3.7% over V's base; the three
take about 5% of Elemental's profile ([throughput](../wave-loop/wave-loop.PLAN.md#sim-throughput)).

- Verify: all 37 goldens byte-identical, every replay and the simval replay; an interleaved benchstat A/B of
  Elemental, Ret, Hunter and the raid against the wave base; a profile of Elemental.
- Owns: `sim/core/threat.go`'s bookkeeping and its tests.

#### As built

- `openThreatCast`, `closeThreatCast` and `threatBonus` inline (costs 79, 73, 71 of 80): open calls
  `addAssistThreat` only for a positive spell with a bonus; close calls `shareThreatBonus` only when the cast
  recorded hits, which splits the bonus with plain loops, no closures or `defer`. `threatHit` keeps the
  enemy's `UnitIndex`, not a `*Unit`, so a recorded hit skips the GC write barrier; the log finds the unit
  through `sim.AllUnits`. No call site changed.
- `TestThreatBookkeepingWalk` (`threat_test.go`): 20,000 random steps (nested casts, hits and landings, the
  threat multiplier, pull time, attack power, logging) bit-equal to a copy of the old bookkeeping, log lines
  included; dropping or moving the truncation, or misreading the hits, fails it.
- 37 goldens byte-identical, simval 1114/1114, `go vet` and `gofmt` clean.
- Interleaved A/B against `bb4ffd124`, 12 rounds, `BenchmarkSimulate` (benchstat medians, machine loaded to
  ±70–120%): Elemental, Ret, Hunter and the raid all flat (no p < 0.05); allocations unchanged.
- Profile, `tools/perf bench` Elemental at 60,000 iterations, one proc, base (via `-overlay`) → new in one
  window: the bookkeeping (open, close, share, assist, `threatBonus`) 2.71% → 1.49%; close and share 1.98% →
  1.01%. `ThreatFromDamage` (about 0.3%) is untouched: caching it would reorder float multiplications.

### PERF-Y: win back wave Y's slowdown (wave Z, done)

Idle at 100 iterations, wave Y runs Elemental 16% over V's, Hunter 13%, Combat Rogue and Ret 8%, the raid 4%
([throughput](../wave-loop/wave-loop.PLAN.md#sim-throughput)). 5–7% of Elemental's and Hunter's profiles is
`Spell.flatDamageTaken` (`sim/core/damage_taken.go`): Gift of Arthas' bonus resolves `dealsWeaponDamage` and the
spell's coefficient on every hit, though both are fixed per spell. The rest isn't attributed.

- First, an interleaved A/B of each Y merge against its first parent (EC-CORE-FIX `2c609daa4`, EC-PET-FIX
  `b1bf58d68`, EC-TICK-FIX `36ed3bdb3`, the cross-review `91bc1a112`) for the four spec benches at 100 iterations;
  profile what costs.
- Then take that cost off the hot path without changing any result, e.g. resolve per spell what never changes.
- Verify: all 37 goldens byte-identical, every replay and the simval replay; an interleaved benchstat A/B of the five
  benches against the wave base; Elemental and Hunter profiles.
- Owns: `sim/core/damage_taken.go` and the hot paths the A/B names, in small hunks; Z's class items own `sim/<class>/`
  and EC-THREAT-FIX2 `threat.go`.

#### As built

- Attribution: interleaved A/B of each Y step against its first parent, 12 rounds, `BenchmarkSimulate` at 100
  iterations, test binaries built from `git archive` trees (benchstat medians, loaded machine):
  - EC-CORE-FIX: Hunter +9.8%, Ret +9.2%, Rogue +9.1%, Elemental +17.6% (base ±18%), the raid flat. Diff profiles
    put about 4-6% of it in `flatDamageTaken` and 1% in `ticksAsCasts` (`aura_helpers.go`): both loop over
    `Effects` by value (`duffcopy`), on every hit or periodic proc check, and `flatDamageTaken` binary-searches the
    bonus rows too.
  - EC-TICK-FIX: the raid +5.1%, Ret +5.7%, Elemental +4.1%, allocations +0.6-2.2%. A diffuse diff profile: players
    land casts on the 10 ms world tick instead of the 100 ms map update, so a fight fits ~3% more hardcasts
    (Elemental's per-cast `OnComplete` closures +3.4%). That is the behaviour the goldens moved for, not overhead.
  - EC-PET-FIX, PERF-THREAT and the cross-review: no significant move. EC-PET-FIX's hunter pet `refreshScaling`
    costs 3.4% of Hunter's profile against the 0.9% of the `addOwnerStats` it replaced: pet code, left to a pet item.
- `Spell.serverDerived` (`sim/core/damage_taken.go`) caches the weapon and normalized flags, `ticksAsCasts`, and
  the direct and periodic taken coefficients (whether each takes the dot's stacks). It keeps every input it
  read: `serverSpell`, the `NoServerData` and `IncludeTargetBonusDamage` flags, whether `ProcMask` is a white hit,
  the unit's `Type` and `SummonedAsPet`; each read compares them, so runtime `Flags` writes stay exact. The float32
  products run in the same order, so the result is bit-equal.
- `TestServerDerivedWalk` (`damage_taken_test.go`): 20,000 random steps over 16 server data spells and a white
  swing (flags, proc mask, server data, unit kind and dot stacks changed under the cache) bit-equal to the uncached
  resolvers and a copy of the old `flatDamageTaken`; dropping any key compare, either flag, the stacks multiply or
  the periodic index fails it. Server data rows are keyed by pointer, so never edited in place.
- 37 goldens byte-identical, simval 1114/1114, `go vet` and `gofmt` clean.
- Profiles, `tools/perf bench` at 30,000 iterations, one proc, base → new in one window: Elemental's
  `flatDamageTaken` 7.1% → 1.4%, `ticksAsCasts` 0.6% → 0; Hunter's `flatDamageTaken` 4.8% → 1.0%,
  `dealsWeaponDamage` 1.2% and `ticksAsCasts` 0.7% → under the sample floor.
- Interleaved A/B against `d253e096c`, 14 rounds, both iteration cases: the machine ran at ±35-100%, so no case
  moved at p < 0.05 (Elemental at 100 −8%, p = 0.11); allocations unchanged. The idle re-baseline is the measure.
- Cross-review: `Pet.refreshScaling` skips a tick whose owner stats match the last one's and works in pet-held
  scratch (`TestPetScalingTickWalk`); Hunter's profile 3.4% → 2.1%, `refreshOwnerHitScaling` 1.0% → 0.5%.
- Left: `makeCastFunc`'s per-hardcast closure (about half of Elemental's allocations), `nextTick`'s 64-bit
  modulo (about 0.9% of the raid).

## Order

The user put the pass before wave J, whose BIS-e2e-perf times the optimizer with PERF-TOOLS' harness
(2026-09-22):
- **I2 (done):** PERF-TOOLS and PERF-CONC, which share no files.
- **I3 (done):** PERF-OPT and PERF-HOT, file-disjoint, once I2's baseline sets their targets.
- **I6 (done):** PERF-MISSILE, the user's call (2026-09-26), before J times the optimizer.
- **J2 (done):** PERF-RNG, the user's call (2026-09-27), with the optimizer fixes wave J's speed audit found.
- **N (done):** PERF-FU, the user's call (2026-10-01), with wave L's and M's other follow-ups.
- **V (done):** PERF-COST, the user's call (2026-10-03), after wave T2's cost arithmetic slowed Combat Rogue.
- **Y (done):** PERF-THREAT, the user's call (2026-10-05), after wave V's threat bookkeeping slowed the casters.
- **Z (done):** PERF-Y, the user's call (2026-10-06), after wave Y slowed every bench.
