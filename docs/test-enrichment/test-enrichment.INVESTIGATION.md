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
