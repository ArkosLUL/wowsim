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
  batch below takes about 8.3 h.

The BiS Batch's own runs on the live 25-raider roster, whose grid holds 21 non-healers (19 DPS, 2 tanks):

| Run | Time |
|---|---|
| Raid contribution, one raider and phase, two Combat Rogues, P1, Quick | 340–415 s |
| Batch stage 1, one raider and phase, Quick | 4–32 s for DPS (median 10 s), 17–94 s for the tanks |
| Batch stage 2, one raider and phase, Quick | 277–388 s (median 287 s) |
| Batch stage 1, one raider and phase, Normal | 139 s (Prot Paladin), 347 s (Unholy DK) |
| Batch, both stages, 21 non-healers × 5 phases, Quick | ~8.3 h: stage 1 27 min, stage 2 96 min a phase |

The Quick batch rows come from wave J's batch through `tools/database/acbis/driver/` at sim commit
`5d5ff1d23`, with the worldserver and database stopped. Stage 1 ran all 5 phases (105 jobs) and stage 2
phase 1 only (19 jobs): the user stopped it there once its cost was clear, so the total extrapolates
stage 2 over 5 phases. The stored batch filled 41% of the page's localStorage after stage 1 and 51%
after phase 1's stage 2, so about 91% for a whole batch. The Normal rows and the Combat Rogue pair are
earlier partial runs at sim `b9fc6c703039`, their load unrecorded.

## Speed audit (after wave J)

What a whole-roster batch spends its ~8.3 h on, and what would cut it: a read-only audit at `5d5ff1d23`,
from the code, the timing run's reports and the batch's rows above.

- **Stage 2 is ~95% of the batch.** Raid mode (`Optimize` in `api.go` swaps in `NewRaidEvaluator` for the
  whole run) re-measures the objective, racial screen, curves and effects with 25-player sims, rebuilding
  the surrogate stage 1 already built. A raid iteration costs 25.7 ms of CPU, about 1.2× the sum of its
  members' solo ones (Fury P1 0.62 ms). A Quick raid-mode run spends 53% on effects, 18% on curves, 14%
  on alternatives and 7% on the racial screen.
- **Raid mode is noisy.** `IsTest` rolls draw from streams keyed by label alone (`Simulation.labelRand`),
  shared by every unit, so a target whose gear changes its number of rolls shifts every other raider's.
  Stage 2's deltas carry ±35–62 raid DPS where stage 1's carry ±7–13 at the same iterations.
- **Few channels carry one raider's gear to the others.** `Derive` reads another raider's gear only for
  Demonic Pact (`demonicPactSP`), the roster's one strong channel (Fel). Focus Magic needs a target the
  roster doesn't set. Debuff uptimes and Replenishment's receivers barely move with the provider's stats
  (estimated). Heroic Presence is one choice per party.
- **Bugs that cost picks:**
  - Each stage 2 job wears the others' stage 1 picks (`wearStage1Picks`), so every raider counts Heroic
    Presence as its own: 10 of phase 1's 19 stage 2 picks went Draenei, parties 1 and 2 whole.
  - Quick never adopts a runner-up: over budget, `neighborhood.go` halves a round's iterations, and a
    half round doesn't adopt. 64 of the batch's 105 stage 1 runs list a one-slot alternative more than
    4 SE over the pick (median +0.8% J).
  - At Quick the raid racial screen keeps 6 finalists in 11 of 13 stage 2 runs, and `withRacialTraits`
    pairs each gear set with each finalist, so verify covers about 3 gear sets.
- **Stage 2 loses to stage 1.** A paired raid A/B of phase 1's 19 DPS raiders (4,000 iterations, everyone
  else in their stage 1 pick; `G:\DevStuff\GitHub\.wave-loop\ab-p1\`) put the stage 2 gear under the
  stage 1 pick for 15, by up to 1,004 raid DPS (Angry), and over it for 3 (Hellflame, Druidica,
  Malediction, +129 to +380). Their own DPS moved the same way. The stage 1 pick was each run's warm
  start and stage 2 scores its pick as the A/B does, so the loss is its Quick search's: likely the noisy
  raid-mode surrogate, with acceptance measured against the raider's live gear, not the stage 1 pick.
  Draenei's Heroic Presence adds 150–250 raid DPS, which the gear around it gives back.

Options, estimated, none measured:

| Option | Effect | Effort |
|---|---|---|
| Stage 2 as a raid-scored refinement: reuse stage 1's normalizer, curves and residuals; raid sims race ~5 finalists and confirm the neighborhood's top moves | stage 2 ~8 h to ~48 min | L |
| Stage 2 only where a channel exists: Demonic Pact's provider and recipients, one Draenei per party, a short raid check for the rest | stage 2 to ~10–25 min | S–M; changes a settled Decision |
| Per-unit random streams behind an optimizer-only `SimOptions` flag, goldens unchanged while it's off | raid comparisons need 16–50× fewer iterations for equal precision | M |
| Score raid mode on the target and the raiders it couples to (a control variate) | ~4× fewer iterations | S |
| Sim only the units a channel touches | raid iterations ~7–20× cheaper | M |
| Curves and effects solo, raid sims for the rest (the timing run's proposal) | stage 2 to ~2.1 h | M |
| Racing, with the budget a ceiling: a comparison stops once decided; a short round adopts after topping up | stage 1 −6–10%; fixes the adoption bug | S–M |
| Two jobs at once, on one process-wide worker semaphore | stage 1 −8–15% | M |
| Roll streams resolved once, not a map lookup per roll | 1.05–1.15× both stages | S–M |
| The rotation hot paths PERF-HOT left (`getFirstReadyMCD` every pass, empty APL passes) | 1.1–1.3× both stages | M–L |

Together: about 65–75 min for a whole batch with every raider's stage 2 kept, 30–50 min with stage 2
narrowed, stage 1's ~20 min then the floor. Rejected: shorter encounters (the same noise per simmed second,
plus bias) and a learned surrogate (retrained on every sim commit).
