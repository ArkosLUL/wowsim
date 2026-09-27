# BiS optimizer: investigation

What existed before the optimizer and why the design in [the PLAN](bis-optimizer.PLAN.md) looks the way it
does. Verified on 2026-09-18.

## Existing machinery and its gaps

**Bulk sim** (`sim/core/bulksim.go`):
- Single player only. It ranks by DPS alone (`Score()`), keeps the top 30, and panics above 1M combos.
- Auto-gem puts one gem per socket color, never checks meta requirements, and assumes a belt buckle.
- Auto-enchant copies the replaced item's enchant, whether or not it fits.
- `isValidEquipment` misses a unique one-hander in both hands, a 2H with a 1H in the off hand, and Titan's
  Grip.
- `NewEquipmentSet` and `EquipItem` re-slot weapons by hand type.

**Meta gems:** Go never checks meta activation. The UI strips inactive metas in `ui/core/sim.ts` before it
sends a request.

**Stat weights** (`CalcStatWeight` in `statweight.go`):
- ±20-point steps, with `IsTest` and `SaveAllValues`.
- It panics inside workers and can't be cancelled.
- It weighs DPS, HPS, TPS, DTPS, TMI and PDeath. DTPS, TMI and PDeath use Armor as the reference stat.

**Tanks:**
- Metrics come from `UnitMetrics`; TMI from `calculateTMI` in `metrics_aggregator.go`.
- Incoming healing is a `HealingModel`; its presim sets HPS to 1.5 × DTPS.

**Raid buffs:**
- They come from the players (`AddRaidBuffs`/`AddPartyBuffs` in `raid.go`).
- `GetRaidBuffs` mutates its input.
- Nothing provides Demonic Pact SP.
- Targeted buffs go through spec-option `UnitReference`s.

**Async plumbing:**
- The path is `sim/core/api.go`, then the `sim/web` routes and wasm exports, then `net_worker.js` and
  `worker_pool.ts`.
- `sim/web` serves `net_worker.js` unless it's started with `--usefs --wasm`.
- An async result is dropped after 10 minutes without progress.

**Equip rules** live only in the UI:
- `canEquipItem`, `enchantAppliesToItem` and `canEquipEnchant` (`ui/core/proto_utils/utils.ts`)
- `MetaGemCondition` (`gems.ts`), which is declarative
- uniqueness by item id (`gear.ts`)
- a warning, not a rule, above 3 JC gems

**Item DB:**
- `UIItem.phase` is Classic's, and source filters work by exclusion.
- Enchant phases are unusable: 212 of 225 are 0.
- 268 items carry only Titan Rune sources.
- Item `requiredProfession` is never set.
- `LoadItems` doesn't read `maxcount`, `ItemLimitCategory` or `AllowableRace`.

**Presets:**
- Most specs have P1–P4; only 5 have P5.
- 22 Go test files load gear through `GetGearSet`.

**Encounters** (all still Classic numbers):

| Raid | Boss AIs |
|---|---|
| Naxx | Patchwerk 10/25, Loatheb, Thaddius, Kel'Thuzad |
| Ulduar | Algalon 25, Hodir |
| ToC | Gormok 25H, Anub'arak 25H |
| ICC | Sindragosa 25H, Lich King 25H |
| RS | none |

## Server facts behind the catalog

These durable facts live in [azerothcore-server](../guide/azerothcore-server.md#progression-tiers) and
[azerothcore-data](../guide/azerothcore-data.md#items-acore_world):
- progression tiers per map
- vendor gates
- emblem tiers
- epic gems from prospecting
- difficulty entries and ExtendedCost

## Performance

Measured by `BenchmarkOptimizerEval` (`sim/optimizer/bench_test.go`) on a Ryzen 7 7800X3D, 8 cores and 16
threads, GOMAXPROCS 16. Each sim is 180 s ± 5 s against one target, with full buffs and `IsTest`:

`tools/acore/dock.sh exec go test --tags=with_db -run '^$' -bench BenchmarkOptimizerEval ./sim/optimizer/`

| Cost | Fury P1 | Arcane P3 |
|---|---|---|
| Iteration, one thread | 0.35 ms | 0.12 ms |
| Iteration, all 16 threads busy (wall) | 45 µs | 18 µs |
| Building a sim | 0.3 ms | 0.33 ms |

- 16 threads do about 8× one: SMT adds little.
- Pairing cuts a delta's standard error 2.4× (+100 AP, Fury, 400 iterations), worth about 6× the
  iterations.

The budgets `EffortBudget` pins, and the whole-run targets:

| Run | Evaluations × iterations | Target |
|---|---|---|
| Quick | 250 × 500 | 3–11 s |
| Normal | 500 × 4000 | 1–2 min |
| Thorough | 1000 × 10000 | 5–10 min |

A tank run's target is up to 1.6× a DPS run's at the same effort. Thorough is untimed: its budget alone,
every thread busy, comes to 7.5 min on Fury P1 and 2.9 on Arcane P3.

Whole runs, timed by the [perf harness](../../tools/perf/README.md#scenario-harness) at the web server's
shape (`bench -procs 16 -workers 15`, median of 3 runs) at sim commit `06a2d3e7c`, on the 7800X3D with the
worldserver and database stopped and only the idle prod sim container up. `raid25` is raidctx's synthetic
25-player raid:

| Run | Quick | Normal |
|---|---|---|
| Fury P1 | 6.6 s | 75 s |
| Combat Rogue P3 | 10.6 s | |
| Fire Mage P3 | 3.1 s | |
| Ret P4 | 5.9 s | |
| Prot Paladin P3, tank | 7.9 s | 66 s |
| Feral Tank P2, tank | 7.5 s | 86 s |
| Prot Warrior P1 in `raid25`, tank | 9.4 s | 119 s |
| Fury P1 in `raid25`, raid mode (a batch's stage 2) | 255 s | 56 min, one run |

- Every Quick run sims 134k–146k iterations, up to 17% past the budget (the curves' extra knots, the pair
  sims), and every Normal one here 1.9M–2.1M. So the spread is the spec's cost per iteration: Combat
  Rogue's takes 3.2× Fire Mage's CPU. Sequential steps like bisection and the search leave threads idle.
- Tanks take 1.1–1.4× Fury P1 at Quick and 0.9–1.6× at Normal. Prot Warrior's 1.6× is its sim: 6% more
  iterations, each 1.5× Fury's CPU.
- Raid mode sims the whole raid every time, 41–45× Fury P1's CPU per iteration. Effects take 53% of its
  Quick run and Stat curves 18%.
- The search stage never sims; on these pools of 1.9k–4.6k candidates it took at most 3.1 s at Quick and
  5.5 s at Normal, both Prot Paladin P3.
- A whole roster's batch at these times, 21 non-healers' stage 1 and 19 DPS raiders' stage 2 over 5
  phases: about 7 h at Quick, stage 2 all but 20 min of it, and about 4 days at Normal. The real roster's
  stage 2 below ran 1.3–2.2× as long, its load unrecorded.

The BiS Batch's own runs on a real 25-player roster, the machine's load unrecorded unless stated:

| Run | Time |
|---|---|
| Raid contribution, one raider and phase, two Combat Rogues, P1, Quick | 340–415 s |
| Batch stage 1, one raider and phase, Quick | 7–54 s, the slowest all with another container up |
| Batch stage 2, one raider and phase, Quick | 486–570 s |
| Batch stage 1, one raider and phase, Normal | 139 s (Prot Paladin), 347 s (Unholy DK) |
| Batch, both stages, 21 non-healers × 5 phases, Quick | ~15 h, extrapolated |

The batch rows come from partial runs of the BiS tooltip effort's batch driver
(`tools/database/acbis/driver/`) at sim commit `b9fc6c703039`, over the live 25-raider roster whose grid
holds 21 non-healers (19 DPS, 2 tanks). None completed a batch: the widest covered 9 of one phase's 21
stage 1 jobs, and the rest sample three phases. The total extrapolates the per-job rows over 21
non-healers × 5 phases for stage 1 (~1–1.5 h) and 19 DPS × 5 for stage 2 (~14 h). Stage 2 ran above the
340–415 s the Combat Rogue pair measured. BIS-e2e-perf still owes a measured full-roster batch; Normal
has only the two stage 1 samples.
