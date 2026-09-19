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

Whole runs take longer, since sequential steps like bisection leave threads idle; BIS-e2e-perf measures
them. Still estimates, for 16 threads:

| Run | Time |
|---|---|
| Tanks | 1.5× the above |
| Raid contribution, one raider and phase | 4–6 min |
| Batch, Quick | 30–45 min |
| Batch, Normal | 6–8 h |
