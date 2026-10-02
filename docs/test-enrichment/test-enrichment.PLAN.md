# Test enrichment: richer goldens and live e2e

The goldens and the live e2e cover less than they seem to ([INVESTIGATION](test-enrichment.INVESTIGATION.md)).
This effort fixes that in wave-loop work items; the [RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md) agent rules
apply to each.

## Decisions (settled with the user, 2026-10-01)

- Wave-loop items, waves M–P, after L lands.
- The three empty healer suites are marked, not given APLs. Healers stay out of scope (parity PLAN).
- Ability results only in `Average` rows. They hold actions, player and pet auras, and target debuffs.
- Unholy and BM get one-point talent moves, not the UI presets.
- Elemental's "Adaptive" option allowlists Fire Elemental. Subtlety allowlists its Combat cooldowns and is
  marked; no Subtlety APL.
- Both guards (rotation, talent) fail, with reasoned allowlists.
- Damage probe: a server command first. Talent, glyph and set mods are applied as auras. First scope is
  direct damage, heals and DoT ticks; weapon strikes later.
- Golden characters go without gems and enchants.
- A real sim/server mismatch becomes a follow-up item; the item that finds it drops the record (with a
  fixture README row) and never fixes the sim.
- No `tmi` in goldens: it's 0 without a `HealingModel`, and a real one moves every tank row.

## Rules for every TE item

- Re-measure your finding on your own base first: the INVESTIGATION measured `master` at wave K.
- The simval replay stays all-PASS.
- Never edit `ui/<spec>/gear_sets/*`. Edit a UI APL only for a bug the UI shares; extra test rotations go
  inline in the test file.
- A suite whose talents or rotations change also updates `serverDataPresets` (`sim/serverdata_test.go`).
- Each item's fixture is its own file in `sim/core/testdata/simval/`, replayed by its own `tools/simval` test
  with its own record struct. `simval.jsonl` stays `TestAgainstServerRecords`' `CheckRecord` replay, which
  `int-check.sh` also runs.
- Changes to `docs/wave-loop/*` go in `contractChangeRequests`.

## Waves

| Wave | Items | Why |
|---|---|---|
| M | TE-ABILITY (G, FS) · TE-REPLAY (live) | The ability block lands first, so later golden changes show which ability moved |
| N | TE-GUARD (G) · TE-DAMAGE (C++, live) | The guard's fixes read through the block |
| O | TE-TALENTS (G) · TE-GOLDCHAR (C++, live) | Need the guard's allowlist type and the probe |
| P | TE-SWEEP (live) | Needs TE-TALENTS' list and `.simval talent` |

Only one item per wave edits the e2e package (a single Go package).

**Backlog:** other mob types (every golden target is `MobTypeDemon`); recorded runs as a regression test
(rrsim needs DBCs).

## Work items

### TE-ABILITY (M, done; goldens, full suite)

- `proto/test.proto`: replace `CastsTestResult`/`casts_results` with
  `map<string, AbilitiesTestResult> ability_results = 5` keyed by test name; reserve 4.
  `AbilitiesTestResult {map<string, AbilityTestResult> entries}`;
  `AbilityTestResult {casts, hits, crit_pct, amount_per_hit, uptime_s, amount}`, per iteration, summed over `Targets`.
- Keys: `<unit>|action|<id>` and `<unit>|aura|<id>`. `<id>` is `core.ProtoToActionID(m.Id).String()`;
  `<unit>` is `player`, `pet:<name>` or `target<i>` (`EncounterMetrics.Targets[i].Auras`). Same-named pets and
  repeated aura ids are summed. Only actions with casts or hits, auras with uptime.
- `sim/core/test_suite.go`: `TestDPS` fills the block for any test whose name contains `Average`, from the
  same result. Init in `newTestSuiteResult`; compare both directions. Delete `TestCasts`, its routing and
  `RotationCastsTestGenerator`.
- `tools/acore/dock.sh`: `delta` prints moved abilities, largest damage change first, on lines indented two
  or more spaces (`int-check.sh` filters them from its summary). Fix the usage text.
- Contract requests: optional `abilityDelta` in `workflow.js`'s `goldens.suites[]` and `brief()`; the
  RUNBOOK's contract and step 4.4 say "stats, abilities and weights".
- Docs: `testing.md` Goldens (owned by this item in M).
- Goldens: every suite gains the block; no DPS moves.
- After merge, every worktree and [int] runs `dock.sh proto`: `dock.sh test` won't regenerate an existing
  `api.pb.go`.
- **Owns:** `proto/test.proto`, `test_suite.go`, `test_generators.go`, `dock.sh` golden commands, `testing.md`
  Goldens.

### TE-REPLAY (M, done; golden-neutral; stages: code, live)

Code:
- `check.go`:
  - crit suppression only for crit-capable spells (serverdata effects, `Attributes[2]` `CANT_CRIT`); an id
    missing from serverdata gets a README row;
  - attack type from `spell.dmgClass` (Judgement of Wisdom 53408 is ranged);
  - no miss check for positive spells on the faction-7 dummies 999000–999003;
  - partial block gated on `NoActiveDefense`, without the physical-school condition.
  `records.go` parses `dmgClass`, `positive`, `derived.yellow.attackType`.
- `naked()` also requires no ratings and no non-racial auras (reforge and DK `info` records).
- White damage: compare naked player `melee` records' `meanDamage` per outcome with the sim's white damage
  (`nakedCharacter`, target armor from the snapshot), allowing for the server's integer rolls and `ceil`
  armor. Skip `taken`. Add `damageMin/Max` and `mean*` to `records.go`.
- Housekeeping: rename `TestP7Tank*` → `TestSimvalP7Tank*`; `isTestAccount` becomes a prefix registry filled
  from each test file's `init()`; remove the stray `e2e;C` dir; measure `run.sh` and fix "about 2 min";
  update the `tools/simval/README.md` Replay caveats; capture recipe in `testing.md` "Against the server".

Live (server lock): one full `run.sh`, streamed with `tail -F -n +1`. Keep lines with `unixMs` at or after the
run's start, dedupe by line, and replace `simval.jsonl` (a recapture never appends). Key expectations on
(command, `spell.id`, class, target entry): bot names don't identify tests.

**Owns:** `tools/simval/**` except rrsim and chronicle, `simval.jsonl` and the fixture README; module
`e2e/p7_tank_test.go`, `e2e/simval_test.go`, `README.md`; `testing.md` "Against the server".

### TE-GUARD (N, done; goldens)

- `GetAPLSpell`/`GetTargetAPLSpell` (`apl_helpers.go`) record unknown `ActionID`s on `APLRotation` (accessor);
  no warning-string parsing.
- After the test loop, `RunTestSuite` checks each unique player (deduped on the marshalled player), with
  `NewEnvironment` on a `googleProto.Clone` of the raid (`NewShaman` mutates options). It fails on an
  unallowlisted unknown `SpellID` whose `serverdata.SpellByID` is nil or whose `Family` is the class's. Move
  `classFamilies` from `sim/serverdata_test.go` into `serverdata`.
- Allowlist: `DroppedSpells map[int32]string` (id → reason) on `SpecOptionsCombo` and `RotationCombo`,
  exposed by `FullCharacterTestSuiteGenerator` through an optional interface that `CombinedTestGenerator` and
  `joinedTestGenerators` forward.
- Fixes (report each suite's full talent change):

  | Suite | Fix |
  |---|---|
  | Unholy | `-320033500002-2300303050032152000150013133151` (Black Ice 4→3, Scourge Strike +1). Run the DK parity tests, which read `UnholyTalents`; if one breaks, give the suite its own variable |
  | BM | `51200201515012233110531351-005305001-4` (Improved Tracking 5→4, Aimed Shot +1) |
  | Blood Tank | `blood_aggro` leaves `OtherRotations` for a `SingleDpsTestGenerator{Name: "BloodAggro"}` in the suite's `JoinTestGenerators`, with the UI "Blood Aggro" talents `0355220530303303201020131301--0052003050032` and glyphs Dancing Rune Weapon, Rune Strike, Dark Command |
  | Enhancement | `enhShamWFWF` gets `Shield: LightningShield` and the FT totems with `UseFireElemental` |
  | Elemental | allowlist 2894 on "Adaptive" (no Fire Elemental in that totem set) |
  | Frost Mage | `ui/mage/apls/frost_aoe.apl.json`: 42939 → 42940 (the UI shares the bug; db.json regenerates at integration) |
  | Subtlety | allowlist Adrenaline Rush, Blade Flurry, Killing Spree on `combat_expose` (no Subtlety APL) |
  | Assassination | allowlist Blade Flurry (conditional cast) |
  | Combat | allowlist Cold Blood on `fan_aoe` (shared AoE APL) |
- One-line comments on the three healer suites (stats and item heals only) and Subtlety (runs a Combat
  APL); the same notes in `testing.md` Goldens.
- Goldens: Unholy, BM, Blood Tank, Enhancement, Frost Mage.
- **Owns:** `test_suite.go`, `test_generators.go`, the unknown-spell list (`apl_helpers.go`, `apl.go`), the
  serverdata family map, the fixed and healer suites' `*_test.go`, `sim/deathknight/dps/parity_test.go` if
  needed, `frost_aoe.apl.json`, `sim/serverdata_test.go`, `testing.md` Goldens.

### TE-DAMAGE (N, done; golden-neutral; C++; stages: C++ and e2e, sim, live)

- `.simval damage <spellId> [pet]` in `SimValidationCommands.cpp`, wired like `HandleYellowCommand`, with a
  help row in `simval_commands.sql`. Deterministic; per effect it records type, base min and max, then
  `SpellDamageBonusDone` and `SpellDamageBonusTaken` at both bases (heals: `SpellHealingBonus*`; DoTs: done at
  apply, taken at tick), the crit bonus on a fixed amount, and the target's health % and auras.
- A spell shaped by a script, a dummy or trigger effect, per-family code in `EffectSchoolDMG` or
  `AuraEffect::CalculateAmount` (Rend, Rupture, Garrote), or a target-state override in
  `SpellDamageBonusDone` is recorded `shaped: <reason>` and not compared.
- Compile-check with `acore/ac-wotlk-build:master -fsyntax-only` before the rebuild.
- `e2e/damage_test.go` `TestSimvalDamage`, per class: a level-80 bot with known spell power and attack power
  the sim can mirror; mods as `.aura <rank spell>` (the `dk_test.go` pattern); 2–4 spells, each with and
  without the mods that touch it.
- Sim (`tools/simval/damage.go`, `TestDamageRecords`, `damage.jsonl`): build the character with the spell's
  spec (fix `basestats.go`'s class→spec map for feral and enhancement) and the same auras; weapon from the
  snapshot's `attacks[].weapon` (no `with_db`); target from the snapshot's level, armor and creature type;
  ids via `Direct.FromSpellID`. Compute `spell.CalcDamage(sim, target, base + coef·SP,
  spell.OutcomeAlwaysHit)` at both bases; compare within ±0.1% or 1 point, and the crit bonus too.
- **Owns:** module `src/SimValidationCommands.cpp`, `simval_commands.sql`, `e2e/damage_test.go`, `README.md`;
  `tools/simval/damage.go`, `basestats.go`'s spec map, `damage.jsonl`, `tools/simval/README.md`.

### TE-TALENTS (O, done; goldens)

- Guard test in package `sim`: "implemented" is a `Talents` field read in `sim/<class>` non-test code (via
  `go/ast`, following variables bound from `.Talents`, excluding pet talents), mapped to the tree's
  `fieldName` through protoreflect `JSONName`. "Taken" is a rank in any suite build (order from
  `ui/core/talents/trees/<class>.json`). It fails on implemented-but-untaken, unless allowlisted with a
  reason (raid-buff talents FullBuffs supplies, tank-only, PvP). Export the implemented list as committed
  JSON.
- Extra builds, one or two per class, cover the rest: `SingleDpsTestGenerator{Name:
  "Talents-<label>-Average"}` in the suite's `JoinTestGenerators`, with Average's options and encounter and
  an inline APL casting the build's active talents. Existing rows don't move.
- **Owns:** the suites' `*_test.go`, `sim/serverdata_test.go`, the guard test, the exported JSON.

### TE-GOLDCHAR (O, done; golden-neutral; C++; stages: C++ and e2e, sim, live)

- C++: `.simval glyph <slot> <glyphPropertiesId>`, and `.simval talent <spellId>` via `GetTalentSpellPos` and
  `Player::LearnTalent` (fills `m_talents`, unlike `.learn`).
- `e2e/goldchar_test.go`: each non-healer suite's default player — class, race, the gear set's items via
  `EquipEntry` without gems or enchants, talents and glyphs through the new commands, `.ip set <name> 18` —
  records `info` and TE-DAMAGE's probes.
- Sim (`tools/simval/goldchar.go`, `TestGoldenCharacters`, `goldchar.jsonl`): the same character without gems,
  enchants, buffs or consumes; compare geared `info` fields (spell power, attack speeds, weapon damage, crit,
  hit, expertise) and damage. Stay out of `checkInfo`; keep `nakedCharacter`'s signature.
- **Owns:** module `src/SimValidationCommands.cpp`, `simval_commands.sql`, `e2e/goldchar_test.go`, `README.md`;
  `tools/simval/goldchar.go`, `records.go`, `goldchar.jsonl`, `tools/simval/README.md`.

### TE-SWEEP (P; golden-neutral; live)

- Per class, several learn paths over TE-TALENTS' list (`implemented_talents.json`, its `rankSpells` one per rank) (71 points can't hold them all), reset between paths,
  filler points per tier, via `.simval talent`. Stance, form and presence match on both sides.
- `.simval info` after each implemented talent; fixture `talents.jsonl` is a trimmed attacker-sheet
  projection decoded by `talents.go`'s own struct.
- Compare each step's sheet delta with `ComputeStats` with and without the talent: stats, AP, RAP, spell
  power, crit, hit, expertise, haste, dodge, parry, block, armor, health, mana. A talent moving no field on
  either side is a proc or spell-mod talent, TE-DAMAGE's ground.
- **Owns:** `e2e/talent_sweep_test.go`, `tools/simval/talents.go`, the path generator, the fixture.

## Orchestrator onboarding (wave M start)

- Merge `master` into `integration`; add waves M–P to the registry and a `TE-` row to "Where the specs are".
- Extend the RUNBOOK's "parity-owned files take master's side" to the files this effort took over.
- Apply items' `contractChangeRequests`; run `dock.sh proto` in [int] after TE-ABILITY merges.
- Per-wave re-baselines as usual; the BiS slow suite and `BenchmarkSimulate` shouldn't move.
