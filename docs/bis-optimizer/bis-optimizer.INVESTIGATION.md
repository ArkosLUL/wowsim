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

The budgets `EffortBudget` pins, every thread busy:

| Run | Evaluations × iterations | Fury P1 | Arcane P3 | Target |
|---|---|---|---|---|
| Quick | 250 × 500 | 5.7 s | 2.3 s | 3–6 s |
| Normal | 500 × 4000 | 91 s | 36 s | 1–2 min |
| Thorough | 1000 × 10000 | 7.5 min | 2.9 min | 5–10 min |

Whole runs take longer: the curves' extra knots and the pair sims go up to 10% past the budget, and
sequential steps like bisection and the search leave threads idle. The slow suite's DPS runs, with the
worldserver busy on about 1.5 of the 16 threads, and J gained over the preset (paired, 10000 iterations):

| Run | Candidates | Quick | Normal |
|---|---|---|---|
| Fury P1 | 3062 | 9 s, +219 J | 105 s, +226 J |
| Combat Rogue P3 | 2749 | 21 s, +550 J | 247 s, +553 J |
| Fire Mage P3 | 1935 | 5 s, +33 J | 39 s, +39 J |
| Ret P4 | 4569 | 11 s, +54 J | 125 s, +73 J |

The search stage itself takes at most 3 s at Quick and 8 s at Normal with 4.6k candidates, the top of the
UI's pools (its Ret replay fixtures hold 2.6k–4.6k). Raid contribution is measured two ways: `wowsimcli
optimize` on a 2-player smoke raid (a floor, since a full roster's raid sims cost more per iteration), and
the BiS Batch's own stage 2 on a real 25-player roster (two Combat Rogues, Quick), 16–20× the smoke raid's
cost. BIS-e2e-perf still owes the rest:

| Run | Time |
|---|---|
| Tanks | 1.5× a DPS run |
| Raid contribution, one raider and phase, 2-player smoke raid, Quick | 23 s |
| Raid contribution, one raider and phase, 2-player smoke raid, Normal | 3 min 22 s |
| Raid contribution, one raider and phase, real 25-player roster, Quick | 340–415 s |
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
