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

## Performance estimates

Estimates for 16 threads. BIS-eval replaces them with measurements.

| Run | Time |
|---|---|
| Normal (about 500 sims) | 1–2 min |
| Quick | 3–6 s |
| Thorough | 5–10 min |
| Tanks | 1.5× the above |
| Raid contribution, one raider and phase | 4–6 min |
| Batch, Quick | 30–45 min |
| Batch, Normal | 6–8 h |
