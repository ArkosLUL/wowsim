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

## Order

The user put the pass before wave J, whose BIS-e2e-perf times the optimizer with PERF-TOOLS' harness
(2026-09-22):
- **I2 (done):** PERF-TOOLS and PERF-CONC, which share no files.
- **I3 (done):** PERF-OPT and PERF-HOT, file-disjoint, once I2's baseline sets their targets.
- **I6 (done):** PERF-MISSILE, the user's call (2026-09-26), before J times the optimizer.
- **J2 (done):** PERF-RNG, the user's call (2026-09-27), with the optimizer fixes wave J's speed audit found.
