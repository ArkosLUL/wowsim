# Test enrichment: investigation

What the golden suites and the live e2e tests miss today, and ways to make them catch more. **[sim]** is this
repo, **[ac]** is `G:\DevStuff\GitHub\azerothcore-wotlk-pb`. Measured on `master` at `e7d3abf9c` (2026-09-30,
wave K), so each work item re-measures on its base. The work is planned in
[test-enrichment.PLAN.md](test-enrichment.PLAN.md).

## How the tests work today

**Goldens.** 37 suites, one per spec (`sim/**/*.results`). `core.FullCharacterTestSuiteGenerator`
(`sim/core/test_generators.go`) takes one talent build, one gear set and one or more APLs, and generates:
- `CharacterStats`: the final stat sheet;
- `Settings`: races × rotations × NoBuffs/FullBuffs × (60 s one target, 300 s one target, 300 s 20 targets);
- `AllItems`: each item with an effect swapped into the default setup;
- `SwitchInFrontOfTarget`;
- `StatWeights`: only Disc, Elemental and Prot Warrior;
- `Average`: 2000 iterations. Every other sim test runs 20 iterations, all with seed 101.

`RunTestSuite` (`sim/core/test_suite.go`) stores only dps, tps, dtps and hps, with tolerance 1e-5, so any
change fails (exact regression). `tmi` is in `DpsTestResult` but never set. The whole 37-suite run takes about
3 min in the container (2 to 25 s per package). Every suite's target is `MobTypeDemon`.

**Casts support is half built.**
- `RunTestSuite` routes any test whose name contains `Casts` to `TestCasts`, which stores casts per action.
- Its comparator only walks the actual actions, so an action that stops being cast never fails.
- The generator for such tests, `RotationCastsTestGenerator` (`test_generators.go`), is referenced nowhere.
- Upstream used it for one rogue suite and dropped it in `069579620` (2023-01), with no reason given.

**Golden tooling.**
- `dock.sh delta` compares only `dps_results` (dps, hps, tps, dtps). A suite whose stats or weights moved
  still counts as "unchanged".
- The wave loop's report schema (`docs/wave-loop/wave-loop.workflow.js`: `goldens.suites[].dpsDelta`) and
  RUNBOOK step 4.4 are built on `delta`.
- `sim/serverdata_test.go`'s `serverDataPresets` copies every golden suite's character, so a new suite or
  build must be added there too.

**Live e2e.** `[ac]/modules/mod-sim-validation/e2e`: 27 Go tests, run by `run.sh` (needs the live stack and
the user's OK).
- Headless GM bots send `.simval` commands. Each command calls the server's own combat functions without dealing
  damage and appends a record to `simval.jsonl`. The records hold:
  - hit and outcome tables (`melee`, `taken`, `yellow`, `spell`);
  - armor multipliers and proc chances;
  - a stat snapshot (`info`);
  - static SpellInfo (`spelldump`).
- The tests assert server facts only: they never run sim code.
- `[sim]/tools/simval` compares records with the sim's formulas (`check.go`, `procs.go`, `basestats.go`).
  `TestAgainstServerRecords` replays the committed `sim/core/testdata/simval/simval.jsonl` in every `go test`.
- Recorded runs (`TestRecordedRun*`, opt-in) fight the dummy under mod-chronicle. `tools/simval chronicle` and
  `rrsim` then compare DPS (±2%) and per-ability averages, by hand. rrsim has rotations only for Ret, Prot
  Paladin and Affliction, plus the hunter's own harness. The captures are in `sim/core/testdata/chronicle/`.
- Replaying the bot's fixed 1.5 s cycle in rrsim was rejected: the APL engine can't do it. So recorded runs are
  judged per ability, not on DPS (`tools/simval/README.md`, rrsim).

**Static spell data is already covered offline.** `TestServerDataConflicts` (`sim/serverdata_test.go`) checks
each registered spell against the captured spelldump:
- cast time, GCD and cooldowns;
- the flags that decide its hit table;
- damage range, SP/AP coefficients and weapon percent.

What no test covers is behaviour the code computes: talent, glyph and set-bonus modifiers; how they stack; what a
rotation actually does.

## Findings: goldens

Measured by a probe, a `go test -overlay` copy of `sim/core/test_suite.go`. For every DPS test it dumps the
per-action metrics (casts, hits, damage) and aura uptimes, and it runs `ComputeStats` on each test's raid to
collect the APL's validation warnings. All 37 suites still passed under it. Every registered spell appears in
the action metrics, even with zero casts (`Spell.doneIteration`). Every aura with an action id appears in the
aura metrics.

**1. Three suites test nothing of their spec.**
- Resto Shaman, Holy Paladin and Resto Druid use an APL that only autocasts cooldowns (`DefaultRotation` in
  their `*_test.go`). The UI's `autoRotation` for these specs is empty too.
- They never cast a heal: 131 to 137 of each suite's ~140 entries are `value: {}`.
- Healers are out of scope for parity and for the BiS optimizer (both PLANs' Decisions).

**2. Rotations name class spells their build can't cast, and the APL drops them silently.** An action whose
spell the character doesn't know becomes a validation warning (`sim/core/apl_helpers.go`, shown through
`ComputeStats` → `RotationStats`), and the sim runs on without it.

| Suite | Dropped | In rotations |
|---|---|---|
| Unholy | Scourge Strike: the build, same as upstream's, lacks the talent | all three, default `uh_2h_ss` included |
| Subtlety | Adrenaline Rush, Blade Flurry, Killing Spree: a Combat APL on a Subtlety build | default `combat_expose` |
| BM | Aimed Shot | default `bm` |
| Blood Tank | Dancing Rune Weapon, Heart Strike | `blood_aggro` |
| Assassination | Blade Flurry | all |
| Combat | Cold Blood | `fan_aoe` |
| Elemental | Fire Elemental Totem (2894) | all |
| Enhancement | Lightning Shield (49281) | all |
| Frost Mage | Blizzard 42939; the sim registers 42940 | `frost_aoe` |

The remaining warnings are expected: other races' racials, other classes' buffs (Bloodlust, Power Infusion,
Tricks of the Trade), and `OtherID`/`ItemID` actions.

**3. Talent coverage.** Of the 615 talents the sim's code reads (`Talents.<Name>` in `sim/<class>`), no golden
build takes 69. Nothing checks those 69.

| Class | Untaken, of implemented | Untaken |
|---|---|---|
| DK | 8 of 76 | Scourge Strike, Reaping, Outbreak, Deathchill, Mark of Blood, Improved Blood/Frost/Unholy Presence |
| Druid | 4 of 62 | Dreamstate, Feral Aggression, Infected Wounds, Natural Perfection |
| Hunter | 11 of 66 | Deflection, Efficiency, Imp. Aspect of the Monkey, Imp. Steady Shot, Invigoration, Rapid Killing, Rapid Recuperation, Savage Strikes, Spirit Bond, Thick Hide, Trueshot Aura |
| Mage | 6 of 63 | Arcane Barrage, Cold as Ice, Firestarter, Imp. Blizzard, Imp. Fire Blast, Magic Absorption |
| Paladin | 0 of 37 | |
| Priest | 2 of 58 | Blessed Resilience, Shadow Affinity |
| Rogue | 9 of 61 | Deflection, Ghostly Strike, Hack and Slash, Imp. Expose Armor, Mace Spec., Quick Recovery, Shadow Dance, Turn the Tables, Vigor |
| Shaman | 9 of 60 | Anticipation, Convection, Frozen Power, Guardian Totems, Healing Grace, Imp. Stormstrike, Reverberation, Totemic Focus, Toughness |
| Warlock | 9 of 63 | Amplify Curse, Dark Pact, Imp. Curse of Weakness, Imp. Demonic Tactics, Imp. Drain Soul, Imp. Life Tap, Imp. Searing Pain, Molten Skin, Shadowburn |
| Warrior | 11 of 69 | Anger Management, Booming Voice, Commanding Presence, Focused Rage, Imp. Berserker Rage, Imp. Demoralizing Shout, Imp. Disciplines, Mace Spec., Puncture, Rampage, Sword Spec. |

Some of these are raid-buff talents whose effect `FullBuffs` already supplies (Rampage, Trueshot Aura,
Commanding Presence, Booming Voice). Method: a talent string's digits follow the talent order in
`ui/core/talents/trees/<class>.json`.

**4. Registered spells no golden ever casts.** These are spell ids with zero casts and zero hits across every
test of every suite of the class, and no uptime on an aura of the same id; tag variants are merged. The total
is 73 of 457. Seals, forms, stances and shouts are applied at the start without a cast, so the aura test keeps
them out.

| Class | Never cast |
|---|---|
| DK | Anti-Magic Shell, Death Pact, Icebound Fortitude, Mind Freeze, Vampiric Blood |
| Druid | Faerie Fire, Hurricane, Innervate, Rebirth, Swipe (Cat), Typhoon |
| Hunter | Arcane Shot, Raptor Strike, Scorpid Sting |
| Mage | Blizzard, Ice Lance |
| Paladin | Avenger's Shield, Holy Wrath, Judgement of Light, Ardent Defender |
| Priest | Mind Blast, Mind Sear, Shadow Word: Death, Flash Heal, Binding Heal, Prayer of Healing |
| Rogue | Ambush, Backstab, Garrote, Hemorrhage, Shiv, Shadowstep, Premeditation, Feint, Wound Poison VII |
| Shaman | Chain Heal, Healing Wave, Lesser Healing Wave, Riptide, Earth Shield, Earthliving Weapon, Healing Stream Totem |
| Warlock | Curse of the Elements, Curse of Tongues, Curse of Weakness, Searing Pain, Seed of Corruption |
| Warrior | Concussion Blow, Shockwave |

Spells that aren't registered at all (a talent spell the build lacks) are invisible here: finding 3 covers
them.

**5. Only totals are stored.**
- When DPS moves, the golden can't say which ability moved.
- A rotation losing an ability shows only as a total.
- `eaafe11bd` (Mind Sear casts miscounted in multi-target) had no golden to catch it.
- Short rows move −1.2% to +0.9% whenever the order of random draws changes. Reviews therefore read
  `Average-Default` (parity PLAN P4, PAR-FU-2).

## Findings: live e2e

**6. Damage amounts are unchecked.**
- The module never calls `SpellDamageBonusDone`, `SpellHealingBonusDone` or `MeleeDamageBonusDone` for a spell.
- The only damage-amount assertion is the Mind Flay tick in `p7_pri_test.go` (196 + 0.257 × SP, ±1). It reads
  the combat log through `collectSpellHits`.
- `melee` records a mean damage for each white-swing outcome, but nothing reads it.
- Talent, glyph and set-bonus damage mods are code, so the spelldump can't show them. Checking them is listed
  as "Not scheduled" in `docs/azerothcore-parity/effect-declarations.PLAN.md`.

**7. Per-spell coverage is thin.** Of the 870 spell ids the sim names (`docs/azerothcore-parity/audit`), 43
have a hit-table or spelldump check. Uncovered offensive spells: warrior 16, paladin 15, hunter 43, rogue 14,
priest 6, DK 18, shaman 13, mage 16, warlock 24, druid 15. Most hit tables follow flags that
`TestServerDataConflicts` already checks, so a live check pays off mainly for spells whose outcome a script or
an aura changes.

**8. Most e2e records never reach the offline replay.**
- The committed fixture (82 records) holds only `TestSimvalWarrior`, `TestSimvalHunter` and
  `TestSimvalBaseStats` output.
- The P7, P8, DK, tank and proc tests only log their records. So `go test` doesn't replay them.
- Traced by hand, about eight of those records would fail `check.go` today:
  - crit suppression is expected even for spells that can't crit (periodic-only and dummy spells);
  - Judgement of Wisdom is replayed as melee, while the server rolls it as ranged from its `dmgClass`;
  - a positive spell never misses the faction-7 dummy (Death Coil);
  - `naked()` only checks weapons, so reforge and DK `info` records would get base-stat checks.

**9. The replay ignores recorded fields.**
- `melee`: every `mean*`.
- `info` on geared characters: spell power, attack speeds, weapon damage, most ratings, resistances.
- `yellow`/`spell`: the rolled crit and some thresholds.

**10. Recorded runs aren't regression-tested.**
- `TestChronicleCaptures` only parses the committed captures, and the ±2% gate runs only by hand.
- rrsim needs the live DBCs, which keeps it out of `go test`.
- No tool compares aura uptimes on the sim side, although the parity PLAN asks "tick counts and proc uptimes
  match within noise".

**11. Housekeeping.**
- `run.sh`'s default `-run TestSimval` skips the four `TestP7Tank*` tests.
- The module README says `run.sh` takes "about 2 min". With 19 matching tests now, that is likely stale
  (not measured).
- `tools/simval/README.md` still says `spell` doesn't model ALWAYS_HIT or binary spells, but `check.go` handles
  both.

## Options

### Goldens

- **G1. Rotation guard, then fix the builds.**
  - A test that runs `ComputeStats` for every suite's rotations. It fails on "does not know spell" for a spell of
    the class's own spell family, with an allowlist for the rest.
  - Then fix the nine suites in finding 2, through their talent build or their APL. Each fix changes that
    suite's goldens.
  - Cheap. It stops a rotation from silently drifting away from its build.
- **G2. Per-ability block on `Average-Default`.**
  - Per action: casts, hits, crit rate and damage per hit. Per aura with uptime: uptime.
  - About 20 to 60 entries per suite, stored either in a new `test.proto` message or by reviving
    `CastsResults`, with a comparator that checks both directions.
  - `dock.sh delta` and the wave-loop schema report moved abilities.
  - Churn is the same as today, since those rows already move exactly when DPS does. The gain is that a review
    sees where the change came from.
  - `sim/core/sim_shards_raid_test.go` (`handMerge`) already compares action metrics field by field.
- **G3. Talent coverage.**
  - Extra builds so that every implemented talent sits in at least one golden, raid-buff talents excepted.
  - Add them as light `Talents-<label>` tests (the default setup and the per-ability block), not the full
    Settings matrix. `SettingsCombos` leaves the talent label out of test names, so add them through
    `JoinTestGenerators`.
  - A guard test that fails when an implemented talent is in no build.
  - Add each build to `serverDataPresets`.
- **G4. Mob types.** One default test per suite against Undead (and Beast or Humanoid where the class cares), so
  creature-type effects run at all.
- **G5. `tmi` (dropped).** Every golden's TMI is 0: `calculateTMI` needs a `HealingModel`, which no suite sets.
  A real one would move every tank row.
- **G6. Healer suites.** They are out of scope, so either give them minimal APLs (heal the target dummy) or
  mark them as not covering their spec. Today they look like coverage but aren't.

### Live e2e

- **E1. Replay every e2e record offline.** Capture a full run's records (stream them out of the worldserver,
  per `testing.md`) into `sim/core/testdata/simval/`, so `TestAgainstServerRecords` covers P7, P8, DK, tanks and
  procs. One live run, no code.
- **E2. Read the ignored fields.** Compare white `meanDamage` per outcome with the sim's white damage for the
  recorded snapshot, and compare the geared `info` fields. Sim side only.
- **E3. Damage probe.**
  - Add `.simval damage <spellId> <n> [pet]`. It runs the server's damage or heal pipeline for a spell cast by
    the current character (done and taken bonuses per effect) and records mean non-crit and crit amounts.
  - The replay rebuilds the character (snapshot plus a setup: gear, talents, glyphs, auras), computes the sim's
    amounts, and allows ±0.1%.
  - This checks the talent, glyph and set-bonus mods that no test reaches.
  - It is C++ plus a sim-side character builder, so it needs a server rebuild.
  - A first step without C++: cast real spells and read `collectSpellHits`, as the Mind Flay check does.
- **E4. The golden characters on the server.**
  - Give a bot a golden suite's default setup: `EquipEntry` per item, talents through `CMSG_LEARN_TALENT`, and
    glyphs.
  - `.learn` never fills `m_talents`, so talent-gated server effects would be missing
    (`docs/guide/azerothcore-server.md`). `learnFeralTalents` in the e2e sends the real packet for any class.
    A talent-string→talents helper is missing; `BuildTalentString` in
    `tools/database/azerothcore/roster.go` does the reverse.
  - Then record `info` and E3 for each of the spec's offensive spells. The `CharacterStats` golden and the
    per-ability numbers then get a server answer.
- **E5. Talent sweep.** Learn each implemented passive talent on a naked bot and diff `.simval info` before and
  after. The sim computes the same diff through `ComputeStats`. This generalizes `checkAvoidanceTalent`
  (`p7_tank_test.go`).
- **E6. Recorded runs as a regression test.**
  - For each committed capture, run rrsim offline and compare per-ability non-crit and crit averages and crit
    rate within their standard errors. Add sim-side aura uptimes against chronicle's.
  - This first needs rrsim's DBC dependency removed or its data committed.
- **E7. Housekeeping** (finding 11): rename `TestP7Tank*` to `TestSimvalP7Tank*`, and fix both READMEs.

A live hit-table sweep over all 870 spells is not recommended: the static checks already cover the flags that
decide those tables.

The decisions, the item split and the waves are in the [PLAN](test-enrichment.PLAN.md).

## Constraints

- **Effort.** Parity is closed ("Sequencing: done"), so this effort took over `tools/simval`,
  `sim/core/testdata/simval` and `[ac]/modules/mod-sim-validation` ([workstreams](../guide/workstreams.md)).
- **Goldens.** Promote suite by suite after reading the delta, and never `make update-tests`. Sessions sharing a
  worktree overwrite each other's `.tmp` files.
- **Server.** Live e2e runs and module rebuilds need the user's OK, or in the wave loop the server lock. At most
  one work item per wave changes the module's C++, and new e2e tests go in per-item files.
- **Tooling.** Probes that prove useful move out of scratch, next to the code they test, with a run command in a
  README. G1 and G3's guards make this investigation's probe permanent.

## TE-ABILITY as built

- `Average` tests store `ability_results` (`proto/test.proto`, field 5; `casts_results` removed, 4 reserved).
  `abilityResults` in `sim/core/test_suite.go` fills it from the run `TestDPS` already did.
- `hits` is landed hits, as the UI's `landedHits`: the metric's `Hits` + `Crits` + `Blocks` + `Glances` +
  `Crushes`. The sim's `Hits` excludes crits on most outcome paths, so a raw `crits / hits` went past 100 % (a
  hunter pet's Claw showed 205 %). `crit_pct = crits / landed`, `amount_per_hit = amount / landed`.
- `amount` (damage + healing + shielding per iteration) is stored too: some dots' ticks count no hit (Smite's
  Shadow Word: Pain, 14 % of its damage), and one Disc trinket heals with no cast or hit at all. With it, a
  suite's action amounts add up to its DPS + HPS within 0.3 %.
- Every `Average` test of the 37 suites gains a block, 38 to 105 entries each. The three healer suites
  with empty APLs hold only cooldown casts. No DPS, HPS, TPS or DTPS moved.
- Until promoted, every Average test fails on "Unexpected abilities": the old goldens have no block.
- `dock.sh delta` has to register a test from its ability rows too: a healer test with all-zero metrics has
  no `dps_results` rows, so it would otherwise count as unchanged.
- `int-check.sh` needed no change: its `grep -v '^  '` already drops the four-space ability lines.

## TE-REPLAY as built

Code stage measured on `33da5ca34`, then the live recapture.

**Findings re-measured.**
- Finding 8: the fixture still holds 82 records (warrior, hunter, base stats), replaying 508 checks. The
  server code confirms three of the four traced failures; the fourth is untested:
  - `MeleeSpellHitResult` and `DeriveYellowTable` pick the attack type from `dmgClass` (ranged), and a ranged
    class spell without a ranged weapon rolls with level*5 skill.
  - `SpellDoneCritChance` and `SpellTakenCritChance` return 0 for `CANT_CRIT` and for a spell without a damage or
    heal effect (`ComputeIsCritCapable`), so the sim expected suppression the server never reports.
  - `SpellHitResult` returns no miss for a positive spell on a non-hostile target; `DeriveMagicTable` ignores that,
    so only the rolled rate disagrees.
  - Reforge and DK `info` records couldn't be traced offline, since both tests start armed.
- Finding 11: `run.sh`'s default `-run TestSimval` matches 24 tests now (19 before; 4 renamed tank tests and
  `TestSimvalWhiteDamage`).

**Replay.**
- Attack type, crit suppression, partial block and the positive-spell miss follow the server (see the tools/simval
  README). `derived.yellow.attackType` is cross-checked against `dmgClass`.
- `naked()` also needs no rating beyond defense, dodge, parry and armor penetration, and no timed aura beyond the base
  stats test's. The fixture's 508 checks are unchanged.
- White damage (`whitedamage.go`) models the server's integer arithmetic over the sim's attack power, armor
  multiplier, glancing multiplier, crit multiplier and creature block value. A hand-checked unit test pins it. It had
  no real naked `melee` record to match until the recapture: on the committed armed warrior (weapon 1-3, 1.9 s, 568
  AP) the server rolls about 1.7 more per normal hit than the model, equal to the roll range shifted up by 3 before
  armor, and the level 80 dummy's armor multiplier reads slightly high. Cause not found; the armed warrior isn't
  compared.
- `TestSimvalWhiteDamage` records four stripped characters for it. `fixture_test.go` holds the records a full run must
  produce, enforced now that the fixture is a full capture.

**Housekeeping.** `TestSimvalP7Tank*` renamed, the account prefix registry, the stray `e2e;C` directory removed.

**Live recapture.** One `run.sh` (24 tests, all passed): 286-288 s wall, so about 5 min. The stream captured 142 records
after the start filter and dedupe (the fixture held 82), replaying 1060 checks (508 before). No check the code stage
newly failed on the old fixture; on the new capture one record failed.
- Records per command, before to after: armor 21 to 21, info 45 to 51, melee 5 to 8, procs 2 to 7, spell 3 to 27,
  taken 1 to 10, yellow 5 to 18.
- Judgement of Wisdom (53408) and Death Coil cast (49895) pass. The reforge, DK and tank `info` records pass the
  `naked()` rules; the paladin, hunter and rogue unarmed melee records pass the white damage model.
- Dropped: the unarmed warrior's `melee` record. The server rolls 51 normal, 102 crit, 35 glancing, 10 blocked
  against the model's 49, 98, 34, 8, with the same attack power (568) as the armed warrior. It's the same +2 as the
  armed one, so it's warrior-specific. A fixture README row records it and it's a follow-up, not a sim change.
  PAR-FU-3 (N) restored it: the +2 is Battle Stance's 10% armor penetration, which the model left out.

## TE-GUARD as built

Measured on `7f4174efb`.

**Finding 2 re-measured.** The guard, run before any fix, failed the same nine suites on the same spells, plus one
the table filed under "other classes' buffs": shaman rotations name the raid buff's Bloodlust (2825, tag -1),
which a shaman's NoBuffs tests don't have, and 2825 is the shaman's own family.

**Guard.**
- `GetAPLSpell` and `GetTargetAPLSpell` append each miss to `APLRotation.UnknownSpells()`.
- `RunTestSuite` collects every test's raid (stat weights requests become one), dedupes players on their
  deterministic marshal, then a `RotationGuard` subtest builds each on a clone with `NewEnvironment`. It
  fails on an unknown spell id missing from serverdata or of `serverdata.ClassFamily(class)`
  (`sim/core/serverdata/families.go`, moved from `sim/serverdata_test.go`), unless allowlisted. Tag -1 ids
  are skipped: buffs.go registers them only when the raid buffs bring them, so they say nothing about the
  build. Each id reports once per suite, with the first test that drops it.
- An allowlist entry without a reason, or that no player drops, fails as stale.
- About 150 players per suite, mostly AllItems: 0.1 to 0.5 s per suite.
- Allowlists: `DroppedSpells` on `SpecOptionsCombo` and `RotationCombo`. `DroppedSpellsGenerator` is the
  optional interface: `SettingsCombos` decodes a test's spec options and rotation; the other subgenerators
  of `FullCharacterTestSuiteGenerator` get the default player's; `CombinedTestGenerator` and
  `JoinTestGenerators` forward it.
- Negative check: Chimera Shot (53209) added to TestBM's rotation failed the guard, and passed once
  allowlisted; reverted.

**Fixes.**

| Suite | Change | Effect on `Average-Default` |
|---|---|---|
| Unholy | `UnholyTalents` `-320033500002-2300303050032152000150013133151`: Black Ice 4→3, Scourge Strike 0→1. The DK parity tests pass on it, so no own variable | +14.9% DPS: Scourge Strike new (55271 tags 1 and 2), more Death Coil, Icy Touch and Ghoul Frenzy. Every row moves: `uh_2h_ss` +11 to +19%, `uh_dnd_aoe` −0.2 to +7%, `unholy_dw_ss` +2.7 to +5.7% |
| BM | `BMTalents` `51200201515012233110531351-005305001-4`: Improved Tracking 5→4, Aimed Shot 0→1 | −0.32% DPS: Aimed Shot (49050) replaces Multi-Shot (49048) at about the same damage, Improved Tracking's 1% less lowers every hit; −4.7 to −7.1% on the 20-target rows |
| Blood Tank | `blood_aggro` leaves `OtherRotations` (12 `Settings-…-blood_aggro-…` rows gone) for `TestBloodTank-BloodAggro`: the default player's setup on talents `0355220530303303201020131301--0052003050032`, glyphs Dancing Rune Weapon, Rune Strike, Dark Command, one 300 s target, 20 iterations | new row, 4001.9 DPS; no other row moves |
| Enhancement | `enhShamWFWF` takes Lightning Shield and the FT options' totems (`standardTotems`, Fire Elemental on) | only the 36 `WF` Settings rows: single target +6 to +28%, 20 targets −0.3 to −9% |
| Frost Mage | `frost_aoe.apl.json` casts Blizzard 42940, the rank the sim registers, not 42939 | only the 6 `frost_aoe` rows: Blizzard now casts, +176% to +2330% |

**Allowlists.**

| Suite | Combo | Ids | Reason |
|---|---|---|---|
| Elemental | spec options `Adaptive` | 2894 | BasicTotems leave Fire Elemental off; EleFireElemental covers it |
| Subtlety | rotation `combat_expose` | 13750, 13877, 51690 | Combat talents: there's no Subtlety APL |
| Assassination | its four rotations | 13877 | the mutilate APLs cast Blade Flurry, a Combat talent, under Slice and Dice |
| Combat | rotation `fan_aoe` | 14177 | Cold Blood, an Assassination talent, in the AoE APL both specs share |

`serverDataPresets` takes the Unholy and BM talents and a `BloodAggro` preset; `TestServerDataConflicts`
stays green. The healer suites and Subtlety carry a one-line note (also in testing.md Goldens).

**Bloodlust is intended.** Every shaman APL casts the raid buff's Bloodlust (2825 tag -1) before its own (2825,
tagged with the shaman's raid index: 0 here), so NoBuffs tests dropping the first is expected, not a bug.

## TE-DAMAGE as built

**C++ and e2e stage** (golden-neutral).
- `.simval damage <spellId> [pet]` (`SimValidationCommands.cpp`, help row in `simval_commands.sql`) writes one
  `damage` record; the module README has its fields and shaped rules. Its keys fit `tools/simval`'s `record`, so a
  full `simval.jsonl` recapture holding them still parses; `CheckRecord` skips the command.
- Re-measured on `7f4174efb`: the module still calls no bonus function, and the spec's shaped list needed two fixes.
  - Rend and Rupture are shaped by scripts (`spell_warr_rend`, `spell_rog_rupture`); `AuraEffect::CalculateAmount`
    has no per-family code left. Garrote has neither, so it's compared.
  - mod-spell-tweaks binds tick-period-only scripts to the DK diseases, Deadly Poison, Rupture, Rend and the balance
    dots. They don't shape amounts and are allowlisted; any other script shapes.
- `TestSimvalDamage` (`e2e/damage_test.go`): one bot per class, 3 or 4 spells, a bare pass and a pass with the
  talents and glyphs as `.aura`; six expected-shaped spells cover the script, `EffectSchoolDMG` and target state rules.

**Sim stage** (golden-neutral; see the tools/simval README's Damage probe).
- The spec's `CalcDamage(base + coef·SP)` alone would miss terms the sim keeps in handler code (Impurity's AP
  coefficient, Moonfire's and Immolate's dot-only multipliers, Black Arrow dividing out its own debuff). So the
  replay runs the sim's own `ApplyEffects` (direct) or `Dot.Apply` plus one snapshot tick, with the record's base
  pinned, the outcome forced to a plain hit and resists off.
- The ±0.1%-or-1-point tolerance is widened upward only, by one multiplier per SP or AP bonus: the server truncates
  each before the done multipliers, which matters on small ticks.
- Death Coil: the probe reads 47632's own 600 base; a real cast hands it the 443 the sim declares from 49895. Only
  the base check is skipped.
- `basestats.go` gains `otherSpecs` (healing priest for Flash Heal, enhancement, feral for Rake); the base stats
  checks still build `spec`.
- In the sim, every compared spell rises with its class's second-pass auras, by each talent's tooltip amount.

**Live stage.** One `run.sh TestSimvalDamage` on the rebuilt server: all 10 classes passed, about 105 s wall on a
loaded machine. `damage.jsonl` holds its 78 records, bare and talented for each of the 39 probes, the six shaped
spells included.
- `TestDamageRecords`: 346 checks over 66 compared records, all passing; nothing dropped. The two over the first
  run's 344 are Shadow Word: Pain's and Devouring Plague's tick crits in Shadowform: a tick crits unless `CANT_CRIT`,
  crit capable or not.
- All 148 amounts sit at or above the server's, as its truncation predicts. The closest to tolerance is Lava
  Burst's max, 2.01 over against +2.11.

## TE-TALENTS as built

Measured on `b069c688b`.

**Finding 3 re-measured.** Still 615 implemented talents and 69 untaken. Scourge Strike is taken now (TE-GUARD's
Unholy); the grep's Divine Guardian is only a comment, and Permafrost is read through `talents := mage.Talents`.

**Guard** (`TestTalentCoverage`, `sim/talent_coverage_test.go`; one subtest per class, so each passes alone).
- Implemented: go/ast over non-test `sim/<class>/**`: `x.Talents.Field`, its getter, or `Field` on a variable
  bound from `x.Talents` in the same declaration. The hunter pet's `Talents()` is a call, so it never counts.
  Go name to the tree's `fieldName` through the struct tag and `JSONName`.
- Talent string position n is proto field n (`core.FillTalentsProto`); a tree `fieldName` differing from that
  field fails only when implemented, else is logged (the cross-review fixed shaman.json's `ancestralUealing`).
- Taken: a rank in any suite build. Builds come from go/ast, not `serverDataPresets`: from each `Test*` calling
  `RunTestSuite`, through the package-level funcs, vars and consts it names, every `TalentsString` and every
  `Talents` of a `CharacterSuiteConfig` or `TalentsCombo`, resolved to a string constant. 37 suites, 57
  builds (36 before this item). The `ServerDataPresets` subtest fails on a build no preset runs.
- Allowlist `untakenTalents` (class → fieldName → reason): an empty reason, an entry a build takes, or one the
  class no longer reads fails.
- `sim/core/testdata/simval/implemented_talents.json`: every implemented talent with class, tree, `fieldName`,
  row, max points and `rankSpells` (one per rank; the cross-review filled the trees' compressed lists), one per line. Stale fails; rewrite with
  `go test ./sim -run TestTalentCoverage -update`.
- Negative check: Judgements of the Just zeroed in the Prot Paladin build failed naming it; allowlisted it
  passed; a blank reason and the restored build with the entry kept both failed; reverted.

**Allowlist.** Targets only attack `raid.Tanks`, which only tank suites set.

| Class | Talents | Reason |
|---|---|---|
| Hunter | improvedAspectOfTheMonkey, thickHide, deflection | tank-only (dodge, armor, parry) |
| Mage | magicAbsorption | tank-only (resistances) |
| Rogue | deflection | tank-only (parry) |
| Shaman | anticipation, toughness, guardianTotems | tank-only (dodge, stamina, Stoneskin armor) |
| Warlock | moltenSkin | tank-only (damage taken) |
| Warlock | improvedCurseOfWeakness | `FullDebuffs` applies improved Curse of Weakness |
| Warrior | rampage, commandingPresence | `FullBuffs` gives the crit and improved Battle Shout; Commanding Shout only adds health |

**Builds.** Each a `SingleDpsTestGenerator` with Average-Default's setup
(`MakeSingleTargetEncounter(5)`, `AverageDefaultSimTestOptions`, the suite's default gear, consumes and spec),
a `serverDataPresets` entry, and the suite's UI APL where it already exercises the talents, else an inline
one. Existing rows don't move.

| Row | Takes | Ability block shows |
|---|---|---|
| TestBloodTank-Talents-FrostPresence-Average | Mark of Blood, Deathchill, Imp. Frost Presence | MoB casts and target aura; Icy Touch crit 35% (default 18%); DTPS 276 (default 284) |
| TestBloodTank-Talents-UnholyPresence-Average | Imp. Blood Presence, Outbreak, Reaping, Imp. Unholy Presence | Unholy Presence; Plague Strike 1897 per hit (default 1228) |
| TestBalance-Talents-Dreamstate-Average | Dreamstate, Natural Perfection | 33 Resto points leave no Eclipse or Starfall: Starfire filler |
| TestFeral-Talents-FeralAggression-Average | Feral Aggression, Infected Wounds | Ferocious Bite +13.8%; Infected Wounds aura |
| TestBM-Talents-SpiritBond-Average | Spirit Bond, Invigoration, Savage Strikes | Raptor Strike, 65% crit |
| TestMM-Talents-TrueshotAura-Average | Rapid Killing, Efficiency, Trueshot Aura + glyph, Rapid Recuperation, Imp. Steady Shot | Rapid Fire 4 casts (default 3); Imp. Steady Shot aura; no Aspect of the Viper |
| TestArcane-Talents-ArcaneBarrage-Average | Arcane Barrage | Arcane Barrage casts |
| TestFire-Talents-Firestarter-Average | Firestarter, Imp. Fire Blast, Permafrost, Imp. Blizzard, Cold as Ice | Firestarter Flamestrikes; Fire Blast 35 casts; Imp. Blizzard target aura; Mirror Image damage |
| TestHoly-Talents-BlessedResilience-Average | Blessed Resilience | HPS +3.0% |
| TestShadow-Talents-ShadowAffinity-Average | Shadow Affinity (Shadow Reach's 2 points) | same DPS, TPS 6283 (default 7362) |
| TestCombat-Talents-HackAndSlash-Average | Hack and Slash, Mace Spec. (1H sword MH, 1H mace OH) | Hack and Slash 17.5 procs; DPS 6421 (default 6939: weapons, no Close Quarters Combat) |
| TestAssassination-Talents-TurnTheTables-Average | Turn the Tables, Quick Recovery | Mutilate crit 69.8% (default 66.9%: TTT +6, CQC −3) |
| TestSubtlety-Talents-ShadowDance-Average | Ghostly Strike, Shadow Dance, Vigor, Imp. Expose Armor | Shadow Dance 5 casts, 30 s; Premeditation 5; Ghostly Strike 6.3 |
| TestElemental-Talents-Convection-Average | Convection | LB 158.8 casts (default 155.9); DPS +6.7%, mostly Fire Elemental |
| TestEnhancement-Talents-FrozenPower-Average | Frozen Power, Imp. Stormstrike, Reverberation, Convection (Frostbrand MH) | Earth Shock 32.9 casts (default 23.3); Frostbrand target aura 288 s |
| TestRestoration-Talents-HealingGrace-Average | Totemic Focus, Healing Grace | HPS 8860 (default none: its APL never heals) |
| TestAffliction-Talents-DarkPact-Average | Imp. Drain Soul, Imp. Life Tap, Amplify Curse, Dark Pact | Dark Pact 4.8, Life Tap 15.5 (default 21.6); TPS −14.6% |
| TestDemonology-Talents-ImprovedDemonicTactics-Average | Imp. Demonic Tactics | Felguard crit 38.4% (default 23.1%) |
| TestDestruction-Talents-Shadowburn-Average | Shadowburn, Imp. Searing Pain | Shadowburn 19 casts; Searing Pain crit 67%; Pyroclasm 270 s (default 240) |
| TestFury-Talents-SwordSpecialization-Average | Anger Management, Sword and Mace Spec., Imp. Berserker Rage (31/40, 1H sword and mace, no Titan's Grip) | Sword Spec. 6.7 procs; Berserker Rage 15 casts (default 1); DPS 4427 (default 8008) |
| TestProtectionWarrior-Talents-FocusedRage-Average | Puncture, Imp. Disciplines, Focused Rage, Booming Voice, Imp. Demoralizing Shout | Shield Wall 2 casts (default 1.4); Heroic Strike 109 (default 96); Demo Shout 7.4 casts (default 0) |

Points mostly come from talents the sim doesn't read or `FullBuffs` already supplies; each talent string's
comment names what it swaps. Rows with no UI APL that fits get an inline one or, via the package's
`withActionsBefore`, extra actions ahead of a UI APL's first cast of a spell (Affliction's Dark Pact before
Life Tap, Destruction's Shadowburn and Searing Pain before Incinerate, Fury's Berserker Rage before
Bloodrage). Weapon swaps edit the suite's gear set in the test; their `serverDataPresets` entries keep the
suite's gear, so Hack and Slash (13964) and Sword Specialization (12281) don't register there.

Imp. Demoralizing Shout only raises the shout's AP reduction past `FullDebuffs`' Vindication (575 vs 574), so
the APL's `auraShouldRefresh` starts casting it; DTPS doesn't drop.

No golden field records: Imp. Blood Presence's heal and Spirit Bond (`GainHealth`), Dreamstate and
Invigoration (mana), Cold as Ice (Cold Snap's cooldown stays longer than the fight), Vigor, Imp. Expose
Armor and Quick Recovery (energy), Convection, Imp. Stormstrike, Totemic Focus and Imp. Life Tap (mana),
Healing Grace (healing threat isn't in TPS), Anger Management (rage).

`FuryTalents` (`sim/warrior/dps`) and Balance's `StandardTalents` (`sim/druid/balance`) spend 72 points.

**Deathchill without Frost Strike panics** (`talents_frost.go`: the aura dereferences a nil
`FrostStrikeMhHit`), so the Frost Presence build keeps Frost Strike. A follow-up, not fixed here.

Negative checks: Shadow Affinity, Dark Pact and Hack and Slash, each zeroed in its only build, failed their
class naming it; allowlisted, each passed; reverted.

## TE-GOLDCHAR as built

**C++ and e2e stage** (golden-neutral; no rebuild yet).
- `SimValidationCommands.cpp`, each with a help row in `simval_commands.sql`:
  - `.simval talent <spellId>`: `GetTalentSpellPos` then `Player::LearnTalent(id, rank, true)`; it names why
    `LearnTalent` refused (missing prerequisite, higher rank).
  - `.simval glyph <slot> <glyphPropertiesId>`: `EffectApplyGlyph`'s slot level and type checks, old aura removal and
    glyph spell cast, then `SetGlyph`.
  - `.simval equip <slot> <itemId>`: not in the spec. Eight setups (both Orc frost DKs, Tauren BalancePhase3 and
    FeralApl, all four Troll mages) wear Alliance-only ToC items, which `CanUseItem` refuses a Horde race; the command runs
    `CanEquipNewItem`, and for those items only its slot, class, race, proficiency and level checks. Login unequips such items, so the e2e equips after its relog.
  - `info` records gain `character` (talent rank spells, glyph per slot, equipped items), so the e2e verifies the
    build and the replay can match a record to its setup; `damage` records match through the attacker's name.
  - `spell_tweaks_shadow_dot_haste` (bound to SW:P, VT and DP after TE-DAMAGE) joins the tick-period allowlist;
    without it TE-DAMAGE's shadow probes come back shaped.
- The list: `sim/core/testdata/simval/goldchar_setups.json`, 32 setups (no two suites share a default player);
  format and in-step test in the tools/simval README's Golden characters. Feral's Glyph of Omen of Clarity
  (206580) isn't on the server and the sim doesn't implement it: glyph 0, left out.
- `e2e/goldchar_test.go` `TestSimvalGoldenCharacters` (module README); `run.sh` mounts `SIMVAL_GOLDCHAR_SETUPS` and
  passes `SIMVAL_GOLDCHAR_ONLY` and `E2E_TIMEOUT`. Expected `shaped` reasons are predictions from the builds'
  talents (Molten Fury shapes every fire mage spell, Torment the Weak the arcane and frost mages' bolts, Glacier Rot
  frost DKs' Icy Touch and Howling Blast, Improved Insect Swarm balance's Wrath and Starfire); the live run settles
  them.

**Sim stage** (golden-neutral; see the tools/simval README's Golden characters).
- `TestGoldenCharacters` replays `goldchar.jsonl`: bots match their setup through the info record's `character`, the
  sim builds the setup's character (`goldcharPlayer`), and the checks are the spec's info fields plus the damage replay.
  `damage.go` now takes a ready player (`newDamageSim`, `replayDamage`); `TestDamageRecords` is unchanged.
- The list's `forms` became `auras` (e2e too): the sim starts specs in states the bot lacked, found by
  `TestGoldcharAurasCoverTheSimsStart`. Added: Seal of Vengeance for both paladins (Glyph of Seal of Vengeance's 10
  expertise), Righteous Fury, Unholy's Bone Shield (2% damage), hunters' Aspect of the Dragonhawk, and the raid buffs
  `AddRaidBuffs` gives every DK, druid, mage, priest and MM hunter whatever the options say (Horn of Winter, Mark of the
  Wild, Arcane Intellect, Fortitude, Divine Spirit, Shadow Protection, Trueshot Aura), as the base stats test does.
- The sheet's spell hit leaves out `SPELL_AURA_MOD_INCREASES_SPELL_PCT_TO_HIT`: the replay adds what every magic school
  gets (Balance of Power). Suppression covers only fire and shadow, where the sim gives every school its 3%, so
  WarlockAffliction's spell hit is expected to fail live: harmless for a warlock, drop it as a real mismatch.
- Checked offline on the recorded-run captures' server snapshots (gear, gems, talents): hunter and Protection Paladin
  pass every check; Ret's hasted speed and expertise differ only through rrsim's Retribution Aura and seal, which the
  snapshot predates.
- Lacerate's dot is replayed for one stack, as the probe works it out.

**Live stage** (golden-neutral). Module rebuilt; `goldchar.jsonl` is one full run (all 32 setups pass the e2e,
124 records after drops), and `TestGoldenCharacters` passes 628 checks; 613 over 122 after the cross-review dropped
WarriorArms (Sudden Death learned at 2/3).
- Fixed in the e2e: profession skill and reputation for gated items (`.setskill`, `.modify reputation`); hunters'
  starting 10% quiver swapped for a 15% one; full health before recording (bots sat at 35–50%, which turned off
  Blood Gorged); Elemental's Flametongue Weapon cast on the main hand through a raw CMSG_CAST_SPELL.
- Fixed in the list: talents learned prerequisite first (`talentStringSpells`; Dual Wield before its
  specialization); Blood Presence for every DK (the server's default, the sim has none until its rotation); Track
  Giants for hunters (the sim always counts Improved Tracking); `imbue`; `shaped` reasons as the server gave them
  (Rage of Rivendare for all Unholy probes, Merciless Combat or Tundra Stalker for frost DKs' Death Coil and
  diseases, Dirty Deeds for Subtlety's Garrote, spell_tweaks_omen_faerie_fire, Mind Flay has no probeable effect;
  Balance's Starfire compares).
- Dropped as real mismatches (fixture README): BM's Arcane Shot and Serpent Sting (The Beast Within's 10% needs a
  pet in the sim), MM's Serpent Sting (Ranged Weapon Specialization), SV's Serpent Sting (Noxious Stings) and Black
  Arrow, Elemental's spell power (+84), Affliction's spell hit (Suppression), Fury's and Arms' Thunder Clap
  (Two-Handed Weapon Specialization; Arms' crit multiplier).
- Priest Shadow's 48125 and 48300 compared live, so the shadow dot tick-period allowlist holds.
