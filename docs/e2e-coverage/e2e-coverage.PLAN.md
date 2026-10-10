# E2e coverage: more live checks of the sim

The live e2e and the replays leave timing, costs, threat, pets, gear and buffs unchecked
([INVESTIGATION](e2e-coverage.INVESTIGATION.md)). This effort closes those gaps in wave-loop work items; the
[RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md) agent rules apply to each.

## Decisions (settled with the user, 2026-10-02)

- Every option: A1–A4, B1–B7, C1–C3, as waves S–X after wave R lands.
- Threat: the probe first (EC-THREAT); EC-THREAT-FIX then fixes only what the probe confirms.
- The new creature-type dummies stand in Naxxramas like the others: the user OKs adding them to mod-dungeon-scale's
  disabled list.
- Carried from test-enrichment: a real sim/server mismatch becomes a follow-up item; the item that finds it drops
  the record (with a fixture README row) and never fixes the sim. EC-THREAT-FIX fixes threat; wave T2 (the
  user's call, 2026-10-03) fixes what S and T dropped or allowlisted, specced once T lands.

## Rules for every EC item

- Re-measure your finding on your own base first: the INVESTIGATION measured `master` at `c27854a8d`.
- Every replay stays all-PASS.
- Each item's fixture is its own file in `sim/core/testdata/simval/`, replayed by its own `tools/simval` test
  with its own record struct. `records.go`, `check.go` and `simval.jsonl` stay as they are.
- e2e: each item's tests go in its own files, and its new identifiers carry its prefix (the e2e is one Go package).
  Shared helpers (`simval_test.go`, `records_test.go`, other items' files) are read-only unless your Owns lists
  them; ask for a change through `contractChangeRequests`, as for `e2e/run.sh` and `docs/wave-loop/*`.
- The module README, `tools/simval/README.md`, the fixture README and `testing.md` "Against the server": edit only
  your item's section or rows.
- Never edit `ui/<spec>/gear_sets/*`.
- C++: compile-check with `acore/ac-wotlk-build:master -fsyntax-only`, then rebuild only while Deathsong is offline,
  else the stage returns `waiting-server`, and a later wave's live stage that needs the command carries forward
  too. `simval_commands.sql` starts with a `DELETE … IN (…)` and an `INSERT`: a new command extends both.
- Not needed by any item: the harness's cooldown and power-update parsing, combat-log packet parsing.
- Fix items (T2, Y, Z) fix the sim: re-measure each mismatch at your base against the server source first. A sim gap is fixed
  and its drop or allowance entry and fixture README row go, so the replay compares it; one that never reaches
  combat on the server (a sheet or probe artifact) keeps its entry with the reason corrected. Class files, drop
  lists, allowances and the fixture README are shared across a fix wave: edit only your own mismatches' code and entries.

## Waves

| Wave | Items | Why |
|---|---|---|
| S | EC-SWEEP2 (live) · EC-STACK · EC-TIMING (C++, live) | Cheap checks first; the timing probe EC-SPELLMOD needs |
| T | EC-SHEET · EC-SPELLMOD (live) · EC-THREAT (C++, live) · EC-AOE | SHEET builds on SWEEP2's sheet code; SPELLMOD needs `.simval timing` |
| T2 | EC-COST-FIX (G, FS) · EC-TIMING-FIX (G, FS) · EC-SHEET-FIX (G) · EC-STACK-FIX (G) | S's and T's mismatches, each fixed once with the full set known |
| V | EC-THREAT-FIX (G, FS) · EC-STRIKES (C++, live) · EC-BUFFS (live) | The fix needs T's probe |
| W | EC-GEAR (C++, live) · EC-MOBS (module SQL and config, live) | One rebuild for both |
| X | EC-FIGHT (live) | Fights wear GEAR's full gear, enchant procs included |
| X2 | EC-RECHECK (live) · EC-COVER (live) · EC-PROBES (C++, live) | Fresh records and fewer holes before the fixes |
| Y | EC-TICK-FIX (G, FS) · EC-CORE-FIX (G, FS) · EC-PET-FIX (G) · PERF-THREAT | The sim gaps the loop found, core first |
| Z | EC-THREAT-FIX2 (G, FS, live) · EC-MELEE-FIX (G) · EC-NATURE-FIX (G, live) · EC-ARCANE-FIX (G, live) · PERF-Y | The class gaps, on Y's core |
| Z2 | EC-THREAT-FIX3 (G, FS, C++, live) · EC-CORE-FIX2 (G, FS) · EC-CLASS-FIX (G, live) · PERF-Z · BIS-ALTS | Z's follow-ups |
| Z5 | EC-CORE-FIX3 (G, FS, live) · EC-PET-FIX2 (G, live) · EC-CLASS-FIX2 (G, live) · EC-THREAT-FIX4 (G, FS, C++, live) | Z2's follow-ups and Y's pets, before the BiS chain |

U is taken (the UI wave). Versus the INVESTIGATION's order, B7, A3 and A4 moved forward (cheap, independent) and
C1 moved last (after B6).

**Backlog:** golden rows against other mob types (test-enrichment's G4), once EC-MOBS lands; execute-range damage;
haste stacking beyond EC-BUFFS' pairs. From Y:
- Timing: the live map update's measured ~104 ms (the fights' `map update length` drop); a hardcast started in the
  session pass lands up to a world tick sooner, and a player aura put on there or by a hardcast runs up to one short;
  Drain Soul and Mind Sear tick from a target aura, so on the map update, not the caster's grid; a pet's aura on the
  dummy is assumed early (`_updatableObjectList`'s order unconfirmed).

From Z, not in Z2: the 72-point Fury and Balance strings in the gzipped perf scenarios and UI fixtures, which
pin their requests (the BiS chain's `opt_*` regeneration takes the perf ones).

## Work items

### EC-SWEEP2 (S, done; golden-neutral; stages: code, live)

- `rawTalentInfo.project` and `talentRecord` keep ArP% (`armorPenetrationAuraPct`) and each attack's
  `missChanceTaken` (Frigid Dreadplate, Arctic Winds); `sheet`, `sweepSheet` and `compareSweepSheets` compare them
  and each school's spell crit (Holy Specialization, Critical Mass). A field the record lacks is skipped, not failed.
- Live: rerun only the paths holding Mace Specialization, Serrated Blades, Blood-Gorged, Frigid Dreadplate and
  Arctic Winds (`SIMVAL_TALENT_ONLY`); the trim keeps the other paths' records. Per-school crit needs no rerun.
- **Owns:** `tools/simval/talents.go`, `talents_test.go`, `talents.jsonl`, the simval README's Talent sweep, its
  fixture README rows.

#### As built

Code stage:
- `talentRecord` gained `armorPenetrationAuraPct` (pointer) and `missChanceTaken` (mainhand, offhand, ranged); a
  record without them skips those fields. The record's per-school `spellCrit` is now compared per school.
- New `sweepFields`, all with slack 0 (both sides are whole percents or the same ratings; intellect truncation comes in
  through `sweepSources`): `armor penetration %`, `<attack> miss taken %`, `<school> spell crit %`.
- Sim side: ArP% is `PseudoStats.BonusArmorPenPct`; miss taken is `5 + (DefenseMissChance +
  ReducedPhysicalHitTakenChance) * 100`, one value for all three attacks. School crit has no sim sheet: it is the stat
  plus the lowest `BonusCritRating` among the character's crit-capable damage or healing spells of only that school.
  A school with no such spell is skipped.
- School crit limits: talents that add crit to named spells (Improved Scorch, Devastation) or a new spell with a lower
  bonus (Living Bomb) move that minimum while the server sheet doesn't, so those three steps are dropped (fixture
  README). PriestDiscipline's sim spec is `shadow_priest` and has no holy spell, so Holy Specialization's +5 holy crit
  on the server is skipped, not compared.
- The committed `talents.jsonl` agrees: Critical Mass compares (+6 fire both sides); its records lack ArP% and miss
  taken, so those are skipped until the rerun. Server sheets built from the sim agree on every path.
- Expected from the live rerun (paths WarriorArms, RogueCombat, RogueSubtlety, DeathknightBlood, DeathknightFrost,
  MageFrost): Blood-Gorged (sim keeps it as a health-gated aura, not in `BonusArmorPenPct`) and Arctic Winds (the sim
  only models its frost damage, no hit taken) will likely disagree; Frigid Dreadplate on the ranged attack may too
  (the server aura is melee only, the sim lowers every physical hit).

Live stage (2026-10-03; server lock 10:18 to 10:25; sweep of the six paths 10:18 to 10:23, 312 s; simval.jsonl
streamed into `tmp/`, tail stopped by PID; no restart, config or DB change):
- The six paths' 146 records replaced theirs in `talents.jsonl`; the other 24 paths kept theirs (715 records). The
  replay now compares ArP%, miss taken and per-school crit on every path.
- Agree: Mace Specialization on WarriorArms (+15 ArP), Serrated Blades (+9 ArP), Frigid Dreadplate's melee miss taken
  (+3).
- Dropped (fixture README rows) after ruling out a sweep bug (server side is the raw aura sums, sim side the path's own
  setup):
  - RogueCombat Mace Specialization: server +15 ArP, sim +0. The path carries two daggers; the server sums the aura
    without a mace, the sim adds it only with one.
  - Blood-Gorged: server +10 ArP, sim +0 (health-gated aura in the sim).
  - Frigid Dreadplate ranged miss taken: server +0, sim +3 (the server aura is melee only).
  - Arctic Winds miss taken, all three attacks: server +5, sim +0 (the sim models only the frost damage).

### EC-STACK (S, done; golden-neutral; offline)

- Export `acore_world.spell_group` and `spell_group_stack_rules` (SELECT) to
  `sim/core/testdata/simval/spell_groups.json`; the query goes in the test's doc comment.
- `sim/spell_groups_test.go` builds a raid with `FullBuffs` and a target with `FullDebuffs` and reads each aura's
  `ExclusiveEffects` (`GetAuras()`). Sim categories are effect types (`PercentDamageModifier`, `SpellPowerBonus`,
  …), so it maps auras by spell id: `ActionID.SpellID` only, ranks folded through spelldump's `firstRankId`; an
  item, tag or `OtherID` aura is skipped with a reason.
- It fails when two auras share a sim category but no server rule makes them exclusive, or a server rule makes two
  registered auras exclusive and the sim keeps them apart. Rule types: `EXCLUSIVE` and `EXCLUSIVE_HIGHEST` (one
  stays) ↔ `SingleAura`; `EXCLUSIVE_SAME_EFFECT` (both stay, the strongest amount counts) ↔ the highest `Priority`
  counting; `EXCLUSIVE_FROM_SAME_CASTER` doesn't apply across raid members. A pair differing only in that form is
  logged, not failed.
- Spell-specific exclusivity lives in code (`SpellInfo::IsAuraExclusiveBySpecificWith`): allowlist it, and every
  mismatch, as id pair → reason; a mismatch is a follow-up.
- **Owns:** the test, the export, a `testing.md` "Against the server" line.

#### As built

- `TestSimSpellGroups` builds one raid per `serverDataPresets` entry with every raid, party, player and debuff
  switch on, plus a one-preset-per-class sweep of every item (as the
  conflicts test does) and consumes variants n = 1.. on three casters. It checks every spell-id pair of each
  unit's registered auras, not only those with an exclusive effect (4.6M pairs, about 4 s).
- Rule lookup copies `SpellMgr::CheckSpellGroupStackRules`: nested groups expand, and a shared group is skipped
  when a nested group holds both spells. A same-effect group counts only its guessed aura type
  (`LoadSpellGroupStackRules`' most-common-type rule, ties to the lowest id); two spells lacking it don't
  interact, so group 1058 (poisons, with crit-taken debuffs) only ties spells that carry aura 197.
- `simAuraSpells` maps the sim's registered id to the server's aura where they differ (talent or cast spell vs the
  triggered aura): Vindication, Earth and Moon, Ebon Plaguebringer, Infected Wounds, Judgements of the Just,
  Improved Scorch, Winter's Chill, Savage Combat, Blood Frenzy, Trauma, Heart of the Crusader, Totem of Wrath,
  Flametongue, Demonic Pact, Sunder Armor.
- Skipped: item and `OtherID` auras, spell id 0, and any tag but 0 and -1 (-1 marks an external cooldown, one
  aura per spell). Form-only differences (sim single aura vs same-effect, or the reverse) are logged with `-v`.
- Allowlisted, with reasons in `spellGroupAllowances`:
  - Not in the tables, so the sim matches the server: stances, aspects, presences.
  - Real sim/server mismatches, each a follow-up: Power Infusion with Arcane Power (server keeps the stronger);
    Bestial Wrath and Enrage with Hysteria and Tricks of the Trade (the sim shares `PercentDamageModifier`, the
    server's group 1107 holds neither); poisons with Heart of the Crusader and Totem of Wrath's debuff (the
    sim's Master Poisoner stacks with them).
- Negative check: removing the Power Infusion and Arcane Power entry fails the test naming that pair. No live
  run.

### EC-TIMING (S, done; golden-neutral; C++; stages: C++ and e2e, sim, live)

- `.simval timing <spellId> [pet]` records:
  - cast time: `SpellInfo::CalcCastTime(caster, &probe)`, with a stack `Spell probe(player, info, TRIGGERED_NONE)`
    so percent mods count and no charge is used; check on the first live run that its destructor's checks hold
    (`m_appliedMods`, `m_spellModTakingSpell`). Which haste applied;
  - GCD: `Spell::TriggerGlobalCooldown`'s arithmetic, copied (it writes state), with its haste rule (category 133,
    1500 ms, dmgClass neither melee nor ranged, no ranged-slot or ability attribute) and `CHEAT_COOLDOWN` exit;
  - cooldown: `rec` and `catrec` per `Player::AddSpellAndCategoryCooldowns`' arithmetic, copied (it sends packets);
  - duration, amplitude, tick count: `player->AddAura` (the player is the caster, so `Aura::CalcMaxDuration` applies
    combo points and `SPELLMOD_DURATION`), read `GetMaxDuration`, `GetAmplitude`, `GetTotalTicks`, remove it. Copy
    `Unit::ModSpellDuration` (diminishing returns, mechanic and dispel mods, the periodic haste rule), which only
    `Spell::DoSpellHitOnUnit` calls. Beneficial auras go on the player; an immune dummy (`AddAura` returns null)
    records `immune`;
  - cost: `SpellInfo::CalcPowerCost`; rune cost per type after `SPELLMOD_COST`; runic power gain.
- The unit snapshot gains the regen fields: each power's `UNIT_FIELD_POWER_REGEN_FLAT_MODIFIER` and
  `_INTERRUPTED_FLAT_MODIFIER`, and `PLAYER_RUNE_REGEN_1..4`.
- e2e `e2e/timing_test.go` `TestSimvalTiming`: per class, a bot like `TestSimvalDamage`'s, 4–6 spells covering a cast,
  a channel, a cooldown, a dot, a cost and (DK) runes; bare, with the cast-time, cooldown, cost and duration talents
  as `.aura`, and under Bloodlust (2825); plus one `info` per bot for regen.
- Sim (`tools/simval/timing.go`, `TestTimingRecords`, `timing.jsonl`): the character as `damage.go` builds it
  (`newDamageSim`); compare cast time, GCD, cooldown, shared cooldown, duration, ticks and cost exactly after the
  server's integer truncation, and mana, energy and rune regen within ±0.1%. Regen is compared on these bots only;
  EC-GEAR's recapture carries it for the golden characters.
- **Owns:** module `src/SimValidationCommands.cpp`, `src/SimValidationSnapshot.cpp`, `simval_commands.sql`,
  `e2e/timing_test.go`, the module README's Commands row and timing paragraph; `tools/simval/timing.go`,
  `timing_test.go`, `timing.jsonl`, its fixture README rows, the simval README's Timing probe.

#### As built

- C++ and e2e (compile-checked with `-fsyntax-only -Werror`, e2e vetted):
  - `.simval timing <spellId> [pet]`, record format in the module README. Where the spec says copy, the arithmetic is
    copied (GCD, cooldown); `Unit::ModSpellDuration` and `ApplyDiminishingToDuration` are public, so they're called.
    `cast` also gives the haste kind and the cast time before haste. `channelMs` and the death knight's
    `baseCooldownMs` are extras.
  - The probe skips persistent area auras (Consecration): `aura.applies` is false for it. Hurricane has its own.
  - Player snapshots gain `regen` (flat and interrupted per power, rune regen, mana and energy rates).
  - `TestSimvalTiming`: 5 or 6 spells and 2 to 5 talent auras per class, three passes (bare, talents, Bloodlust on
    top), a bare and a Bloodlust `info` per bot.
  - `run.sh` without a name skips `TestSimvalTiming`, so a `simval.jsonl` recapture holds none of its records
    (the bots' `info` lines can't be told from the base stats bots'); the replays ignore fields they don't read.
- Sim (`tools/simval/timing.go`, `TestTimingRecords`): replays `timing.jsonl` and the bots' `info` regen against the
  sim's spell, dot, aura and cost; checks in the simval README's Timing probe. Mana regen is read at the server's
  intellect and spirit, since the sim character carries its class's raid buff. Hand-built records test the comparison.
- Probes the sim has no spell for were swapped in the e2e: Death Coil is 49895 (47541 is rank 1), Holy Light, Healing
  Touch, Smite, Frost Nova, Immolation Trap, Cloak of Shadows and Sprint became Divine Storm, Wrath, Devouring Plague,
  Arcane Blast, Explosive Trap and Eviscerate (the rogue has 5 spells).
- Live (rebuilt with `docker compose build ac-db-import ac-worldserver && up -d` at 10:41 to 10:46 with Deathsong
  offline; `TestSimvalTiming` ran 10:46 to 10:55, three times, the last one clean): no assert or crash in the
  worldserver log, no aura left behind, mana regen above 0 on every mana class. `timing.jsonl` is that last run's 174
  `timing` and 20 bot `info` lines.
  - Fixed on the way: the e2e expected Hurricane to have no aura, but its effect 2 is a periodic aura; the replay took
    the server's rune costs as blood, frost, unholy (they are blood, unholy, frost) and compared a negative cast time
    (hunter spells, -999500) before the server clamps it.
  - 1513 checks over 192 compared records pass; the differences left are in `knownTimingMismatches` and the fixture
    README: Avenging Wrath and Healing Wave costs, cost off by 1 with
    talents, tick period truncation under Bloodlust, finishers probed without combo points, and 10% ranged haste on
    the bare server hunter. The sim wasn't changed.

### EC-SHEET (T, done; golden-neutral; offline)

- `checkGoldcharInfo` (`tools/simval/goldchar.go`) also compares each `info` record's stats, max health, max power,
  armor, resistances, AP, RAP, dodge, parry, block, block value, ArP%, and each school's spell power and crit, not
  only the lowest school's.
- Reuse the talent sweep's `(*damageSim).sweepSheet` and `sweepSim`'s sim-derived bounds (after EC-SWEEP2); refactor
  `sweepSim` to take a `goldcharSetup`. Extra record fields parse in `goldcharRecord.UnmarshalJSON`.
- On the committed `goldchar.jsonl`; no live run. EC-GEAR's recapture (W) must keep these checks passing.
- **Owns:** `goldchar.go`'s info checks, `talents.go`'s `sweepSim`, `tools/simval/goldchar_test.go`
  (`droppedGoldchar`), its fixture README rows, the simval README's Golden characters.

#### As built

- `newSweepSim(rec, goldcharSetup)` holds `sweepSim`'s body; `talentPath.sweepSim(n)` wraps it, so the sweep's call
  sites are unchanged. `goldcharRecord.UnmarshalJSON` also parses the record as `rawTalentInfo`; its `sheet()` reuses
  `project` and `talentRecord.sheet`, without max power when the record lacks `maxPower`.
- `checkGoldcharSheet` compares the spec's fields on every info record: server in [sim − sweepSim bound, sim + 0.001]
  (no passing check had the server above). Missing sim field = 0, except block value (skipped without a shield).
  Per-school spell power and crit replace the old lowest-school checks (and `checkSpellPower`); a school the sim casts
  nothing of gets the stat's crit. Max power is compared on the 7 records with `maxPower`; 25 predate it (logged).
- Result: 1565 checks over 131 records pass; 38 info checks dropped (fixture README), after ruling out the comparison
  (each checked against the server source or DB):
  - Real sim mismatches (follow-ups): Envoy of Mortality's `ArmorDamageModifier` armor (7 setups), Rogue Endurance's
    stamina, shaman block value from strength, Moonkin Form armor and feral AP, Blood-Gorged ArP (as in EC-SWEEP2).
  - e2e capture faults (EC-GEAR's recapture): Parry (3127) never learned (9 setups), `.modify hp` pinning a stale max
    health (7 setups), HunterMM's `auras` casting Trueshot Aura its talents lack.
  - Comparison limit, as EC-SWEEP2's: spell-specific crit on every spell of a school (DruidBalance and
    DruidBalancePhase3 arcane, MageFire and MageFrostFire arcane, WarlockDestruction fire).
- Golden-neutral; no live run.

### EC-SPELLMOD (T, done; golden-neutral; stages: spells, live)

- Spells: for each implemented talent with spell mods (auras 107/108 in its rank spell; 276 at `c27854a8d`), the
  spells it changes by the server's rule, `SpellInfo::IsAffectedBySpellMod`: same `family`, `familyFlags` against
  the effect's `classMask`, the spell affected by spell mods at all, and `Player::IsAffectedBySpellmod`'s skip of
  duration mods on infinite-duration spells. Script hooks (`OnIsAffectedBySpellModCheck`) are listed exceptions.
  Keep spells the sim names (`tools/acore/spellids`); write `sim/core/testdata/simval/spellmod_spells.json` from
  `implemented_talents.json` and `assets/db_inputs/acore/spelldump.jsonl`.
- e2e `e2e/spellmod_sweep_test.go`: walk `talent_paths.json` as `TestSimvalTalentSweep` does (a golden character
  can't learn more: its 71 points are spent); before and after each spell-mod talent, `.simval timing`,
  `.simval damage` and the derived crit of `.simval yellow`/`spell` (`n` 1: the derived table is deterministic)
  for each of its spells.
- Sim (`tools/simval/spellmods.go`, `TestSpellModRecords`, `spellmods.jsonl`): each talent's server change per spell
  and field against the sim's, with and without the talent; a talent moving nothing on either side is logged.
- **Owns:** the spell list and its generator, `e2e/spellmod_sweep_test.go`, `e2e/run.sh` (the list's mount),
  `tools/simval/spellmods.go`, its test and fixture, the simval README's Spell-mod sweep.

#### As built

Spells stage:
- `tools/acore/spellmods` writes `spellmod_spells.json` (rule and format in its README). 277 talents: the
  INVESTIGATION's 276 (op counts match) plus Improved Demonic Tactics, whose mods a script builds on dummy effects.
- Groups: ranks sharing mods, values aside; 11 talents' ranks differ in the DBC (Convection, Genesis, Twin
  Disciplines, Unrelenting Assault, …), so a path step is looked up by its rank spell.
- 352 spells, 1203 group-spell pairs; the paths hold 294 such steps, 1171 step-spell probes. Excluded by the server:
  3 (`SPELL_ATTR3_IGNORE_CASTER_MODIFIERS`). Left out: 4 spells another class's code names (Felguard Cleave, Fire
  Elemental Fire Blast, Waterbolt).
- No server code hooks `OnIsAffectedBySpellModCheck`; of the `DoEffectCalcSpellMod` scripts only Improved Demonic
  Tactics sits on a rank spell (Killing Machine's is on its proc aura).
- For the live stage: 31 talents change no spell the sim names (pet passives, totems, raid buffs); 18 change only
  ops no probe shows (threat, range, pushback, dispel resist, proc chance, charges); 11 are casts (`passive:
  false`) whose mods need the aura up; Blade Twisting needs a weapon; 36 listed spells are pet casts (32 hunter pet
  abilities via Longevity, imp, succubus and felhunter spells, Demonic Frenzy), probed with `pet`.

E2e and sim stage (vetted and replayed offline; no live run):
- `TestSimvalSpellModSweep` (module README): the talent sweep's setup per path; per state the auras recast and an
  `info` baseline; per spell `timing`, `damage`, `yellow <id> 1`, a drifted state put right and the damage probe
  redone. Cast talents are cast for the after probes. One `SPELLMOD` line per spell and state; a talent's before
  reuses the last one's after. `run.sh` mounts `SIMVAL_SPELLMOD_SPELLS`, passes `SIMVAL_SPELLMOD_ONLY`, and without a
  name skips `TestSimvalTiming|TestSimvalThreat`.
- Pets aren't summoned: pet spells are probed as the player and the replay doesn't compare them.
- `tools/simval/spellmods.go` (`TestSpellModRecords`, simval README's Spell-mod sweep) compares each number's change
  across the step with the sim's. On lines made from the sim: 638 spell steps agree, 288 move nothing, 245 aren't
  compared (no sim spell on the character: pets, Mirror Image, off-hand strikes, spells some option registers).
- Live run: 506 states, 2251 spell probes (6753 probe commands), 685 talents; estimate 45 to 75 min.

Live stage, unfinished (lock 12:42 to 12:54; trials on DruidRestoration, MageArcane, RogueAssassination, DeathknightFrost,
then the full sweep stopped after 13 of 30 paths in about 7 min, so about 20 min for all; no restart, config or DB change):
`spellmods.jsonl` isn't written. The trials' replay fixed three comparison bugs: crit is read off the spell that hits,
both sides cap crit at 100, and a cast talent without a sim aura (Presence of Mind) isn't compared.

Review (replaying the trial and partial lines offline):
- Fixed: a line failed to parse on a health cost's unsigned power type (Bloodrage); the timing of a sim spell the sim
  only procs or triggers (Slam's hit 50783, Seal of Righteousness' 20154) was compared with the server's cast; a
  finisher's duration (Improved Slice and Dice) was compared at the server's 0 combo points against the sim's 5.
- Disagreements left for the live stage to drop: Annihilation crit on Scourge Strike, Vicious Strikes crit on Death
  Strike, Inner Focus crit on Circle of Healing (sim only); Focused Rage on Bloodthirst and Mortal Strike, Benediction
  on Avenging Wrath and Holy Shield, Efficiency on Volley (server only); Divine Protection costs nothing in the sim;
  Scorpid Sting's base cost; % cost mods 1 mana above the server (it truncates the base cost first); Intensify Rage
  cooldowns 1 ms above (the server truncates `60000 * 0.67f`).

Live rerun (lock 13:21 to 13:40; full sweep 13:22 to 13:40, 18 min, all 30 paths pass; no restart, config or DB
change): `spellmods.jsonl` is 2251 lines (3.6 MB). `TestSpellModRecords`: 445 spell steps agree, 380 move nothing, 93
disagree, 253 aren't compared. 102 failing checks are dropped (fixture README rows), each a real sim gap but Berserk's
Mangle (Bear) cooldown, which the sim resets per cast:
- Cost: the sim 1 above (base cost not truncated first) on 51 spells; Benediction, Efficiency, Focused Rage, Mental
  Quickness and Shadow Focus missing on 14; Scorpid Sting's base 9% vs 11%, and bases already apart (Healing Wave,
  Earth Shield, Mind Sear); percent cost mods multiplied, not added onto the base (Natural Shapeshifter with King of the Jungle, Berserk's energy cut).
- Crit: Annihilation, Vicious Strikes, Tidal Mastery (Thunderstorm) missing; Improved Searing Pain 0.1% not 10%; Inner
  Focus on Circle of Healing in the sim only.
- Timing: Nature's Swiftness misses heals and Rebirth; Permafrost's +3 s on Frostfire Bolt missing; Intensify Rage 1 ms;
  hasted ticks truncated (Celestial Focus, Icy Veins); Improved Earth Shield's charges stretch the sim aura.
- Shadow Focus hit doesn't reach Dispersion.

"Moves nothing" includes steps whose only server change sits in a skipped part, mostly a shaped damage record (Impale's
crit damage).

Live review: the replay read `critDone` 0 as a spell the server never crits, so Deathchill's +100 and Rime's +15 on Icy
Touch and Howling Blast (a death knight's 0 base) went unchecked; 0 on one side is now a real 0, and all three agree
(448 agree, 377 move nothing, 93 disagree, 253 not compared). A number the server moved without a sim counterpart is now
noted on its line, not dropped silently: category cooldowns the sim keeps as its own, Booming Voice on Demoralizing
Shout (the sim's aura is 47437, not the cast's 25203), Permafrost on Frostbolt's slow, dot crit multipliers one side
never crits. Two drops aren't sim gaps (Berserk's Mangle (Bear) cooldown, Dispersion's miss); Healing Wave's base cost
is 15% of base mana in the sim, 25% on the server.

### EC-THREAT (T, done; golden-neutral; C++; stages: C++ and e2e, sim, live)

- `.simval threat <spellId> [amount]` records:
  - the `spell_threat` row, `SPELLMOD_THREAT`, the school multiplier
    (`GetTotalAuraMultiplierByMiscMask(SPELL_AURA_MOD_THREAT, …)`);
  - threat for `amount` damage: `ThreatManager::CalculateModifiedThreat`'s arithmetic, copied (static and
    private), `pctMod` included;
  - the bonus as `Spell::HandleThreatSpells` adds it: nothing under `SPELL_ATTR1_NO_THREAT` or
    `SPELL_ATTR3_SUPPRESS_TARGET_PROCS`; flat + AP part, unmodified; `SpellLevel` with no entry unless
    `SPELL_ATTR0_CU_NO_INITIAL_THREAT`; split across targets; zero on a miss.
  - Live: `AddThreat`/`GetThreat` on the dummy, then `ResetThreat`, after `.gm off`; check it lands before
    relying on it (`CanHaveThreatList`, redirects).
- e2e `e2e/threat_test.go`: Defensive Stance warrior, Righteous Fury paladin, Bear Form druid, Frost Presence DK, and
  a DPS of each with a threat multiplier; their abilities with `FlatThreatBonus` in the sim, plus a few without.
- Sim (`tools/simval/threat.go`, `TestThreatRecords`, `threat.jsonl`): `ThreatFromDamage` and the flat bonus per
  spell against the record. Each mismatch is dropped and listed for EC-THREAT-FIX.
- **Owns:** module `src/SimValidationCommands.cpp`, `simval_commands.sql`, `e2e/threat_test.go`, its module README
  rows; `tools/simval/threat.go`, its test and fixture, its fixture README rows, the simval README's Threat probe.

#### As built

- C++ and e2e (compile-checked with `-fsyntax-only -Werror`, e2e vetted):
  - `.simval threat <spellId> [amount] [targets:<n>] [pet]`, record format in the module README. Needs the sim
    dummy. `CalculateModifiedThreat` is copied; `damage.threat` is 0 under `SPELL_ATTR1_NO_THREAT`.
  - The bonus split counts the units a cast at the dummy hits, from the server's own `SelectSpellTargets` on a
    never-cast `Spell` (no spell scripts): the caster counts when an effect lands on it (Eviscerate, Envenom,
    Arcane Blast, Divine Storm). `targets:<n>` overrides. Positive spells' bonus is modified and reaches the dummy
    through the caster.
  - The dummy's AI never engages, so live `AddThreat` drops a `SPELL_ATTR3_SUPPRESS_TARGET_PROCS` spell's damage
    threat (Slam); `live.engaged` says so and the e2e expects 0 there.
  - `TestSimvalThreat`: 13 bots, 119 probes: Defensive Stance warrior, Righteous Fury paladin, Dire Bear druid,
    Frost Presence DK; Berserker warrior (Improved Berserker Stance), Fanaticism paladin, Cat druid, Unholy
    Presence DK (Subversion), rogue, mage (Arcane Subtlety, Burning Soul, Frost Channeling), Shadowform priest
    (Shadow Affinity), warlock (Improved Drain Soul, Destructive Reach), shaman (Elemental Precision). Talents go
    on as `.aura` before `.cast <id> triggered` (Improved Berserker Stance after it: the server strips a passive
    stance-bound aura on any stance change, and the stance lands on the bot's next update), and each record must
    carry the form's passives (7376, 57339, 57340, 61261, 21178, 3025, 7381, 49868). It checks each record's
    arithmetic, the live threat list and `acore_world.spell_threat`, and that tanks raise and DPS lower some probe's
    threat (not the rogue). 50783 (Slam's damage, both warriors) and 61654 (Fire Nova's damage) are probed beside
    their casts, since the sim splits them off.
- Sim (`tools/simval/threat.go`, `TestThreatRecords`; checks in the simval README's Threat probe): the record's
  character in its form casts the spell at the dummy with a forced hit, and the threat beyond its damage is the
  bonus, so Lacerate's in-cast flat and Sunder Armor's attack power part count. Hand-built records test the
  comparison; `TestThreatProbesReplay` builds and casts every probe. Five probes can't be replayed and are dropped
  (fixture README): Chains of Ice, Death Grip (not in the sim), Seal of Righteousness, Dark Pact, Lava Lash (the sim
  character has no weapon or pet).
- Live (worldserver rebuilt by the user at 16:18; `TestSimvalThreat` 16:55 to 16:57, the Rogue
  rerun at 17:05): `AddThreat` lands on the dummy and matches every record; no assert. `threat.jsonl` holds 114
  records. ac-db-import wasn't rerun, so `acore_world.command` lacks the `simval threat` row: only its help text
  is missing until the next ac-db-import (the command registers at `SEC_GAMEMASTER`).
  - Fixed in the e2e on the way: the priest is Human (from an Undead bot at the e2e's Naxxramas spot the dummy is
    out of VMAP line of sight, so nothing is selected); Execute and Hammer of Wrath are probed at `.simval hp 19`
    (`TargetAuraState` 20%, strictly below); Envenom needs the rogue's own Deadly Poison (57970) on the dummy, whose
    aura shows seconds late and can drop off for a moment, so the probe repeats until its record has it.
  - 248 checks; the 135 that differ are in `knownThreatMismatches` and the fixture README, as predicted from source:
    the server's `SpellLevel` bonus, its unmodified and split flats, the caster in the split, `spell_threat` rows and
    percents the sim lacks or moves, Dire Bear Form, Unholy Presence, Shadowform, Fanaticism (never applied in the
    sim), Burning Soul and Frost Channeling. The sim wasn't changed.
  - A worldserver crash at 17:02 during a run: `ServerMailReward::OnPlayerLogin`'s query callback ran after a bot's
    relog and read a null player (core's `server_mail` script, not the probe); Docker restarted it. It left the bot
    Svtleadhrneb (guid 540540) in `acore_characters`; its cleanup (`run.sh TestCleanupOrphans`) writes to the DB, so
    it ran with the user's OK (18:06).

### EC-AOE (T, done; golden-neutral; offline)

- `TestAOECapMultiplier` already covers the cap's formula; this checks which spells get it, and chain values.
  Class code hand-codes chain jumps (`sim/shaman/chain_lightning.go`), so the test keeps a list: spell id → jumps,
  per-jump multiplier, max targets, capped or not. It checks each against spelldump's `chainTarget`,
  `damageMultiplier`, `maxAffectedTargets` and `serverdata`'s `ChainTargets`, and the cap rule (× 10 / targets past
  10, a player caster's area or non-magic chain damage).
- It fails on a spell the sim names (`tools/acore/spellids`) with `chainTarget` > 1 or `maxAffectedTargets` > 0 that
  the list lacks; allowlist id → reason.
- **Owns:** the test, a `testing.md` "Against the server" line.

#### As built

- `TestSimAOESpells` (`sim/aoe_test.go`) lists 62 entries keyed by spell id and server caster (player, pet,
  totem, trap, boss): chain targets (the first included), per-jump multiplier, max targets, capped, and per
  modifier spell id the values with it on. The 3 explosives also hold the item the sim names.
- Sim-named spells come from `spellset.ScanSim` (about 7 s). The list must also hold every one a jump,
  per-jump or max-target modifier moves (Maul, Revenge, Mangle, Healing Wave) or whose launch damage the cap
  rule covers (area or non-magic chain): stricter than the spec, so the cap check reaches the spells it bites.
- Server side, read from [ac]: cap in `Spell::DoAllEffectOnLaunchTarget`, effective only past 10 reachable
  targets (every effect's, the caster included) and only for a player caster; chain (only from an effect on
  the explicit or a nearby target) and per jump from `SelectImplicitChainTargets` and
  `CalcDamageMultiplier`; max targets from `SelectImplicitAreaTargets`; modifiers matched as
  `SpellInfo::IsAffectedBySpellMod`. Also checks serverdata's chain, multiplier and max targets against the
  capture, and fails stale entries, mods and allowances.
- Allowlisted (`aoeAllowances`, keyed by spell, caster, modifier and field), all follow-ups but the Felguard's:
  - cap: the sim caps the Infernal's Immolation and the rune weapon's Blood Boil (guardian casters, never
    capped), and doesn't cap Holy Wrath, Mind Sear's tick or Chaos Bane;
  - chain: the hunter pet's Swipe hits one target, the server every target in front (ChainTarget 1000);
  - Berserk: Mangle (Bear) hits 3 on the server, 1 in the sim;
  - Improved Revenge rank 1: server always hits a second target at 0.5, the sim at full damage half the time;
  - Tidefury 2 pieces: 0.83 per jump in the sim, 0.833 on the server;
  - unmodeled pre-WotLK set bonuses: Gift of the Gathering Storm, The Ten Storms, The Earthfury;
  - not a mismatch: Glyph of Cleaving matches the Felguard's Cleave by family mask, but a warlock can't have it.
- Follow-ups the test can't see: Mind Sear's tick and Starfall's splash hit the target the server's script
  drops (`spell_pri_mind_sear`, `spell_dru_starfall_aoe`); Thunderstorm and the Global Thermal Sapper Charge
  also hit the caster, so the server caps them from 10 enemies; Thunderfury's debuff only lands on the
  first target; the sim's Explosive Decoy hits at once and capped, but on the server its creature casts
  53273 (uncaptured) on death, uncapped.
- Negative checks: removing Arcane Explosion's entry fails naming it as missing; removing Holy Wrath's
  allowance fails naming it. No live run.

### EC-COST-FIX (T2, done; changes goldens, full suite; stages: core, mods)

- The server's cost (`SpellInfo::CalcPowerCost`, `Player::ApplySpellMod`): the base truncated (`ManaCostPercentage` of
  create mana) plus the school's flat aura (`UNIT_FIELD_POWER_COST_MODIFIER`, Spark of Hope); then × (1 + the sum of
  every `SPELLMOD_COST` percent) + the flat spell mods, truncated; then × (1 + the first school's
  `UNIT_FIELD_POWER_COST_MULTIPLIER`), truncated, floored at 0. The sim multiplies its percents
  (`ManaCostOptions.Multiplier`, `Spell.CostMultiplier`, `PseudoStats.CostMultiplier`) onto an untruncated base.
- Core: that arithmetic for every resource; every class's cost talents and cost auras (Arcane Power, Clearcasting, …)
  move onto it, each mod by its server kind.
- Mods, from the fixture README's spell-mod and timing cost rows: Benediction, Efficiency, Focused Rage, Mental
  Quickness and Shadow Focus missing; Scorpid Sting, Healing Wave, Earth Shield, Mind Sear, Divine Protection and
  Avenging Wrath bases or discounts; Natural Shapeshifter with King of the Jungle, Berserk.
- **Owns:** the cost code in `sim/core/{mana,cast,spell,energy,rage,focus,runic_power}.go` and its tests; each class's
  cost options and cost talents.
- **Goldens:** every suite whose resource spend moves; report each.

#### As built

Stage 1, core (`sim/core/cost.go`, `cost_test.go`):

- `ApplyCostModifiers` runs the server's arithmetic in server units (rage in tenths): truncated base plus the first
  school's flat aura, then `1 + Σ percent` and `+ Σ flat`, truncated, then the school multiplier, truncated, at least
  0.
  The first school is the server's lowest bit, not the sim's `schoolIndex` order.
- Kinds: `Spell.CostMultiplier` / `PseudoStats.CostMultiplier` (1 + Σ spell mod percents), the options' `Multiplier`
  for
  static talents (a sum, never a product), `Spell.CostFlat` / `PseudoStats.CostFlat` / options' `FlatMod` (flats,
  replacing `CostReduction`), `Unit.AddSchoolCostMultiplier` / `AddSchoolCostFlat` (auras 72 and 73).
- `DefaultCast.Cost` is the server base. `StaticCost()` and `StaticCostWithPercent(pct)` are what rotations, energy
  thresholds, rage refunds and mana sums read.
- Moved: Power Infusion, Beast Within, Owl's Focus, Precision, Frost Channeling (4/7/10% on arcane, fire and frost,
  the
  DBC's mask 84) and Arcane Focus to school auras; Spark of Hope to a flat school aura set in `applySparkOfHope`;
  rogue
  and feral flat talents to `FlatMod`; the rogue T7 4 piece to a 5% percent; every percent product (Natural
  Shapeshifter with King of the Jungle, glyphs, Greater Heal, Mind Blast, Penance, Prayer of Mending, Tidal Focus) to
  a sum.
- Verdicts: fixed and removed from the drop lists, rows and `knownTimingMismatches`: the 51 "1 above" costs, Natural
  Shapeshifter with King of the Jungle and Berserk's energy cut. Open for stage 2: the missing talents above, Scorpid
  Sting, Healing Wave, Earth Shield, Divine Protection, Avenging Wrath, Volley; Mind Blast's `focusedMind` drop is the
  Shadow Focus gap.
- Goldens: 25 of 37 suites move, all by truncation, sums or the new school auras (feral +1.9%, elemental +1.4%, arcane
  -0.8%, rest under 0.7%). DK, warrior, Assassination, resto druid, feral tank and holy paladin are unchanged.

Stage 2, mods:

- Added: Benediction on Avenging Wrath, Holy Shield and Divine Protection (which had no cost; 3% of base mana);
  Efficiency on Volley (summed with its glyph); Focused Rage on Bloodthirst and Mortal Strike; Mental Quickness on
  Stormstrike, Lava Lash, Fire Nova, Feral Spirit, Riptide, Earth Shield and Fire Elemental Totem (with Totemic
  Focus); Shadow Focus on Mind Blast and Mind Sear; Earth Shield's own cost (15% of base mana, with Tidal Focus and
  Mental Quickness).
- Bases: Scorpid Sting 11% (was 9%), Healing Wave 25% (was 15%).
- Verdicts: all fixed. Removed the cost entries from `droppedSpellMods` and `knownTimingMismatches` (Avenging Wrath,
  Healing Wave), with their README rows; both replays pass with them compared.
- Goldens: on top of stage 1, paladin Holy gains mana-gain threat (Divine Protection now spends mana, so regen has
  room: tps 0 to 0.22), Enhancement dps +0.151% and tps -0.277%, Feral Spirit/Fire Nova/Lava Lash/Stormstrike cost
  less; Restoration, Hunter and the rest move only by fractions of a cast. Warrior suites don't move (no Focused
  Rage in their talents).

### EC-TIMING-FIX (T2, done; changes goldens, full suite)

- Hasted dot and channel durations: the server truncates the duration and the tick period each on its own
  (`int32(x * UNIT_MOD_CAST_SPEED)` in `Spell.cpp` and `AuraEffect::CalculatePeriodic`); the sim's aura is the
  truncated period × ticks, 1 to 4 ms short (the Bloodlust rows, Celestial Focus, Icy Veins).
- Class: Intensify Rage's cooldown (the server's float `60000 * 0.67f`, 1 ms lower); Nature's Swiftness on heals and
  Rebirth; Permafrost's +3 s on Frostfire Bolt (its dot ticks more); Improved Earth Shield adds charges, not ticks.
- Re-measure the bare server hunter's 10% ranged haste: likely the bot's quiver, which the replay's character lacks
  (`quiverHaste`, `sim/hunter/hunter.go`), so a replay fix.
- **Owns:** `sim/core/dot.go`, `periodic_action.go` and their tests; the class code above; `tools/simval/timing.go`'s
  character setup.
- **Goldens:** every suite with a hasted dot or channel; report each.

#### As built

Each mismatch fixed except Healing Wave's base cast time (below); the drop and allowance entries (`droppedSpellMods`,
`knownTimingMismatches`) and
fixture README rows of the fixed ones are gone, so the replays compare them.

- Hasted channels (`sim/core/dot.go`): a `SpellHasteScalesBoth` aura's duration is base × cast speed truncated to
  ms, no longer truncated period × ticks (Bloodlust rows, Celestial Focus, Icy Veins).
- Intensify Rage (`sim/warrior`): float32 mod math (`1 + -33/100f`), 1 ms under the exact percent.
- Nature's Swiftness (shaman and druid): the aura makes instant every spell with a cast time that the server
  aura's class mask covers (`Spell.AffectedByAuraEffect`, `Unit.NaturesSwiftnessSpells`, new
  `sim/core/spell_class_mask.go`), consumed by any of them. Heals and Rebirth now qualify; the druid's
  Starfire no longer does (arcane, outside the mask).
- Improved Healing Wave (found by the NS probe): the sim lacks its 0.1 s a point off Healing Wave. Left a drop (49273,
  NS cast time checks): the path takes the talent as a filler, so implementing it needs a new path step and a live
  re-record.
- Permafrost: Frostfire Bolt's dot has `(9 + points) / 3` ticks (12 s, 4 ticks at 3 points; at 1 or 2 points the
  server's aura lasts 10 or 11 s with 3 ticks, which the sim's ticks × length can't show).
- Improved Earth Shield: new `DotConfig.AuraDuration` pins the aura to 10 min while the extra charges stay ticks.
- Hunter ranged haste: not a sim gap. The bare bot carries a Light Quiver-class bag (aura 29418, +10% ranged
  speed, in the record's auras), which the replay character lacks; `timingSim.start` applies it from the record.
- Goldens move in 12 suites: balance (Starfire no longer instant under NS: the APL's NS now lingers, aura uptime
  1.5 s to 247 s, -0.44% DPS), restoration shaman (NS consumed by heals: +0.7% HPS), Disc
  (Penance's duration, one AllItems row -4.9%), shadow priest, mage Arcane and Frost (Arcane Missiles, Blizzard),
  elemental and enhancement shaman (Flame Shock), warlock (Drain Soul, Metamorphosis), Fury (cooldown 1 ms) and
  Fire; all other rows within ±0.7%.
- Follow-up: the druid APL still casts NS before Starfire, which now wastes it; only Wrath uses it.

### EC-SHEET-FIX (T2, done; changes goldens)

- Talents: Annihilation and Vicious Strikes crit (Scourge Strike, Death Strike), Blood-Gorged's ArP outside its health
  gate, Frigid Dreadplate on melee only, Arctic Winds' 5% hit taken, Rogue Endurance's stamina, Tidal Mastery on
  Thunderstorm, Improved Searing Pain's 10% (the sim adds 0.1%), Inner Focus off Circle of Healing.
- Re-measure Mace Specialization without a mace: the server's damage checks the weapon
  (`Unit::CalcArmorReducedDamage`,
  `HasItemFitToSpellRequirements`), so likely a sheet artifact.
- Moonkin Form's armor from cloth and leather (24905) and feral AP (`StatSystem.cpp`); block value from strength for
  every class (`Player::GetShieldBlockValue`).
- Items: `ArmorDamageModifier` as armor (Envoy of Mortality) in the item converter; regenerate `db.json` (no other T2
  item touches it).
- Holy Specialization: build the PriestDiscipline path as the healing priest if its other checks still pass, so holy
  crit compares; else keep the skip.
- **Owns:** the class code above, `sim/core`'s block value, `tools/database/azerothcore/convert.go`,
  `assets/database/*`, `tools/simval/talents.go`'s path specs.
- **Goldens:** the suites its classes and items move; report each.

#### As built

Fixed (entry and README row gone, replay compares it):
- Annihilation on Scourge Strike and Vicious Strikes on Death Strike (`sim/deathknight`); Tidal Mastery on
  Thunderstorm; Improved Searing Pain 4/7/10 (was 0.04/0.07/0.10); Inner Focus's +25 crit off Circle of Healing.
- Frigid Dreadplate melee only: new `PseudoStats.ReducedMeleeHitTakenChance` (`ReducedPhysicalHitTakenChance` stays
  melee and ranged: Night Elf, Arctic Winds). The enemy table, `basestats.go` and the sweep's sheet add both; Arctic
  Winds now adds its 1% per rank.
- Rogue Endurance: 2% stamina per rank.
- Moonkin Form (`applyMoonkinForm`): items' armor ×4.7 (passive 24905) and the main hand's feral AP (`weaponFeralAP`,
  shared with cat and bear).
- Block value: Strength × 0.5 for every class, in `addUniversalStatDependencies`; the warrior's and paladin's own
  dependencies are gone.
- Converter `splitArmor` follows `Player::_ApplyItemBonuses`: base armor minus the modifier's whole part, the modifier
  as bonus armor (also without armor: Envoy of Mortality). `BonusArmor` → `Armor` moved from five class files into
  `addUniversalStatDependencies`: hunters and rogues lacked it.
- `db.json` regenerated (`make items`, live DBCs): 58 `db.json` and 5 `leftover_db.json` items changed, each only in
  `BonusArmor` (ranged weapons, one pre-Wrath mace). `spellIcons` also differs (Ancestral Fortitude out, Ancestral
  Healing and Anticipation in): drift of the inputs, the old converter regenerates it too.
- `talent_paths.json` rebuilt (PriestDiscipline as `healing_priest`; RogueCombat learns Endurance last),
  `implemented_talents.json` (Endurance). The live capture lacks that step, so it is dropped with a README row until
  `SIMVAL_TALENT_ONLY=RogueCombat` reruns.

Kept dropped, reason corrected (sheet artifacts; the sim matches the server's combat):
- Blood-Gorged: the sheet is the aura's raw sum, the sim applies it per spell by the server's class mask, ungated.
- Mace Specialization: the server's damage needs a mace too.
- Holy Specialization: PriestDiscipline now builds as the healing priest and every other check compares; the sheet's
  holy crit stays dropped, since Penance is holy and outside the aura's mask, so the sim's lowest holy spell stays 0.

Goldens (`dock.sh delta`; no DPS moves above the print threshold elsewhere):
- Every suite whose `CharacterStats` shows block value, armor or stamina fails on stats alone.
- TestBlood +0.1 to +0.7% (Death Strike crit 53 to 59% from Vicious Strikes), TestBloodTank +0.3%.
- TestDestruction Shadowburn +0.15% (Searing Pain crit 67 to 75%).
- TestCombat one gem row hps +1.3% (heal scales with Endurance's stamina).
- TestArms and TestFury ±0.01% on the default rows, one Fury multi-target row −1.1%: less damage taken from armor
  changes rage.

### EC-STACK-FIX (T2, done; changes goldens; stages: stacking, AoE)

- Stacking (`spellGroupAllowances`): Power Infusion exclusive with Arcane Power; Bestial Wrath and Enrage stack with
  Hysteria and Tricks of the Trade; Master Poisoner exclusive with Heart of the Crusader and Totem of Wrath's debuff.
- AoE (`aoeAllowances` and EC-AOE's follow-ups):
  - cap: never for guardian casters (Infernal's Immolation, rune weapon's Blood Boil); Holy Wrath, Mind Sear's tick
    and
    Chaos Bane capped; the count includes a caster an effect hits (Thunderstorm, Global Thermal Sapper Charge);
  - targets: the hunter pet's Swipe hits every target in front; Mangle (Bear) hits 3 under Berserk; Improved Revenge
    rank 1's second target always, at 0.5; Mind Sear's tick and Starfall's splash skip the target the server's scripts
    drop; Thunderfury's debuff lands on the first target only; Explosive Decoy casts 53273 on its death, uncapped;
  - Tidefury 2 pieces: 0.833 per jump.
- Stay allowlisted: the pre-WotLK sets (Gift of the Gathering Storm, The Ten Storms, The Earthfury), not worth
  modeling; Glyph of Cleaving.
- **Owns:** `sim/core`'s exclusive categories and AoE cap (`target.go`), the class and `sim/common` code above,
  `sim/spell_groups_test.go`, `sim/aoe_test.go`.
- **Goldens:** the AoE suites and those carrying these buffs; report each.

#### As built

Stacking stage:
- Power Infusion with Arcane Power: fixed. The server's group 1123 (EXCLUSIVE_HIGHEST) compares only a shared aura
  type, and they share none, so the one cast last removes the other (`Unit::_AddAura`).
  `core.RegisterPowerInfusionArcanePowerEffect` makes a category whose OnGain deactivates the other aura.
- Bestial Wrath with Hysteria and Tricks of the Trade: fixed. The hunter's self aura is The Beast Within (34471, group
  1107), so the sim aura now carries that id and label; the pet's aura keeps 19574, in no group.
- Enrage with Hysteria and Tricks of the Trade: fixed. Wrecking Crew's Enrage (57518) is only in group 1104, so it
  left `PercentDamageModifier`.
- Master Poisoner with Heart of the Crusader and Totem of Wrath: not a sim gap, entries kept with the reason
  corrected. Every poison rank's aura-197 effect is 0 and Master Poisoner's crit is the caster aura 45176 (type 308,
  outside group 1058), so the tie never changes a total.
- `spellGroupAllowances` lost its Power Infusion, Bestial Wrath and Enrage entries; `TestSimSpellGroups` compares
  them.
- Goldens: only `sim/hunter/TestBM.results` moves, the aura key 19574 to 34471 on Average-Default and
  Talents-SpiritBond-Average, same uptime. Mage and warrior suites don't change.
- All replays, `TestSimSpellGroups`, `TestSimAOESpells`, simval records (1114 of 1114) and `go vet` pass.

AoE stage:
- Cap: `core.AOECapMultiplierFor(units)` counts the units a hit reaches. The Infernal's Immolation and the rune
  weapon's Blood Boil lost the cap (guardian casters); Holy Wrath and Chaos Bane gained it; Mind Sear's tick and
  Starfall's splash use enemies minus 1 (the dropped target); Thunderstorm and the Thermal Sapper use enemies plus 1
  (the caster); Explosive Decoy is uncapped (its creature casts the blast).
- Targets: the hunter pet's Swipe (`AllTargets` on `PetSpecialAbilityConfig`) hits every target; Mangle (Bear) under
  Berserk hits 2 more, refreshing Mangle's debuff on each; Improved Revenge's second target is always hit, at 0.5 a
  rank; Mind Sear's tick and Starfall's splash skip the channeled or star target (`spell_pri_mind_sear`,
  `spell_dru_starfall_aoe`); Thunderfury's debuff goes on each target its jump lands on, not only the first (effect 0
  chains, the damage effect doesn't); Tidefury 2 pieces jump at 0.7 x 1.19.
- `aoeAllowances` keeps only Glyph of Cleaving and the three pre-WotLK sets. `TestSimAOESpells` compares the rest; its
  table gained the fixed values.
- Goldens (`dock.sh delta`, 32 of 37 suites unchanged): `TestBalance` and `TestBalancePhase3` lose about 1.4 to 1.8%
  DPS on single-target rows (Starfall's splash 53190 now hits nothing there; multi-target rows move by -0.4% to +5.2%,
  rotation reshuffled); `TestFeralTank` multi-target rows gain about 4% (Mangle under Berserk); `TestRetribution`
  multi-target rows lose 0.03 to 0.04% (Holy Wrath capped). `TestBM`'s aura key rename is the stacking stage's.
- All replays, `TestSimSpellGroups`, `TestSimAOESpells`, simval records (1114 of 1114) and `go vet` pass. The only
  failing tests are the four golden suites above.

### EC-THREAT-FIX (V, done; changes goldens, full suite)

- Fix what EC-THREAT dropped, nothing more: likely the flat bonus outside `PseudoStats.ThreatMultiplier`, the
  `spell_threat` values, `SpellLevel` threat, the split across targets. Restore the dropped records.
- **Goldens:** every suite whose TPS moves (tanks, warriors, ferals at least); report each.
- **Owns:** `sim/core/spell_result.go`'s threat, the class files EC-THREAT names, `tools/simval/threat_test.go`'s
  drops.

#### As built

Core stage:
- `sim/core/threat.go` holds the server's arithmetic. Hit threat is amount × `spell_threat` pctMod ×
  `Spell.ThreatMultiplier` (`SPELLMOD_THREAT`) × the school multiplier: `PseudoStats.ThreatMultiplier` times each
  `AddThreatSchoolMultiplier` aura covering the spell, once per aura (`GetTotalAuraMultiplierByMiscMask`).
- The cast bonus is `Spell::HandleThreatSpells`: the row's flat + apPctMod × AP, else `SpellLevel` (none under
  `NO_INITIAL_THREAT`, `ATTR1_NO_THREAT`, `ATTR3_SUPPRESS_TARGET_PROCS`), unmodified, split over the distinct enemies
  the cast reached plus the caster when an effect lands on it; a hit outside a cast splits by the server data.
  A positive spell's bonus is modified and shared as assist threat. Nothing before the pull; `ThreatMultiplier` 0
  means no threat at all.
- `SpellConfig.ThreatSpellID` reads another server spell's data (Death and Decay 52212, Demoralizing Shout 47437,
  Seal of Righteousness' damage 25742); `Spell.LandThreatBonus` counts a cast landing without a hit (Consecration).
  `FlatThreatBonus` panics if set.
- The rows are one generated table, `serverdata/threat_auto_gen.go` (gen_serverdata). Class flats and percents
  moved onto it: warrior, druid, DK, warlock, shaman, priest, paladin, Gift of Arthas.
- Thunderfury: one cast of 21992, damage threat ×1 on the first target, its level 63 split over the targets reached.
  27648, the slow, is `SPELL_ATTR1_NO_THREAT`, so it adds nothing (`TestThunderfuryThreat`).
- The replay casts through `SkipCastAndApplyEffects`; 106 of 135 mismatches went.

Classes stage (all re-measured against the record's `modifiers` and spelldump):
- Dire Bear Form ×2.0735 (21178's +45%, 57339's +43%), was ×2.1021.
- Blood and Unholy Presence: −20% plus Subversion's flat −8/−16/−25 on that effect, ×0.55 at 3 points.
- Shadowform's 49868 ×0.7 on every school; Shadow Affinity 8/16/25% on its class mask, Improved Devouring Plague
  outside it.
- Fanaticism −10% a point, always on: the server has no Righteous Fury exception.
- Arcane Subtlety, Burning Soul (10% a point) and Frost Channeling (4/7/10%) are school auras, so Frostfire Bolt
  multiplies them (×0.72).
- `knownThreatMismatches` is empty; `TestThreatRecords` compares all 248 checks over 114 records.
- Dropped records: the server log no longer holds them. `threatLoadouts` give Seal of Righteousness a main hand,
  Lava Lash two weapons (enhancement spec), Dark Pact a pet, so they replay; they stay dropped as `recapture`
  until the next `TestSimvalThreat` run. Chains of Ice and Death Grip stay dropped.

Review: sim spells standing for another server spell took that spell's bonus, so:
- `SpellConfig.NoThreatBonus` marks damage the server deals through an aura, casting nothing: Thorns and
  Retribution Aura (damage shields), Holy Shield's proc.
- `ThreatSpellID` names the spell the server casts: Windfury Attack 25504/33750, the DK off-hand strikes
  (66217 also drops Rune Strike's ×1.75 row), Scourge Strike's shadow part 70890, Deep Wounds 12721, Damage Shield
  59653, Fists of Fury 41990, Explosive Trap's explosion 49065, Immolation Aura's ticks 50590. The five new ids'
  rows are hand-copied from a gen_serverdata run.

Goldens (`dock.sh delta`, all of it): TPS moves in 35 of 37 suites, DPS and abilities in none (no suite wears
Thunderfury); `paladin/holy` and `druid/restoration` don't move. Average-Default: Retribution −28% (Fanaticism),
Shadow −30% (Shadowform), Protection Warrior −10.8% and Fury −2% (unmodified flats, the rows' percents gone), Blood
DK −7.2% and Frost DK −7.7% (Blood Presence ×0.55; off-hand strikes add no bonus), Feral Tank −5.6% (Dire Bear Form,
unmodified Maul and Lacerate), Unholy −1.3%, FrostUH −1.1%; Protection paladin +2.7%, Blood Tank +0.4%, rogues +2.7
to +3.7%, hunters, warlocks, shamans, Balance, Arcane and Frost mage +0.3 to +1.7% (each cast's level bonus); Fire
and FrostFire −0.2%; healers' few TPS +53 to +83% (assist threat). All other checks pass: replays,
`TestSimSpellGroups`, `TestSimAOESpells`, simval records (1114 of 1114), `go vet`.

### EC-STRIKES (V, done; golden-neutral; C++; stages: C++ and e2e, sim, live)

- Weapon strikes in `.simval damage`: `Spell::EffectWeaponDmg`'s fixed bonus, percent, spell bonus and total
  percent, at both ends of `CalculateMinMaxDamage` (normalized via `GetAPMultiplier`), then
  `MeleeDamageBonusDone`/`Taken`. A `WeaponFamilyCode` marks its per-class branches shaped (Devastate,
  dagger bonuses, Mutilate, Divine Storm, Lava Lash, Kill Shot, disease-scaled DK strikes, Rune Strike); the melee
  state overrides join `StateBonuses`.
- Pets and guardians: `pet <n>` reaches every unit in `m_Controlled`, not only `GetGuardianPet()`; `info pet`
  records each scaling aura's amount (`PetDefines.h` ids) and the owner's inputs.
- e2e `e2e/strikes_test.go`: each melee class's strikes, bare and talented; one pet or guardian per pet class
  (hunter pet, demons, ghoul, gargoyle, wolves, Water Elemental, treants, Shadowfiend), `info` and `damage`.
- Sim: `damage.go`'s replay accepts weapon strikes and pets. Leads: hunter pet focus (24 every 4 s), Water
  Elemental base stats, the Infernal's raid buffs, Ghoul Frenzy's cadence.
- **Owns:** module `src/SimValidationCommands.cpp`, `simval_commands.sql`, `e2e/strikes_test.go`, its module README
  rows; `tools/simval/damage.go`, `damage_test.go`, `strikes.jsonl`, its fixture README rows, the simval README's
  Damage probe.

#### As built

- C++ and e2e (compile-checked with `-fsyntax-only -Werror`, e2e vetted; not rebuilt):
  - `.simval damage` adds `weapon` for a spell with weapon effects: `EffectWeaponDmg`'s generic steps at both
    ends, then `MeleeDamageBonusDone`/`Taken`; record format in the module README. Weapon effects keep their
    `unprobed` entry (plus `base`) and the record's `shaped` is unchanged, so existing replays skip strikes as
    before; `weapon.shaped` is the strike's own (family branches gated like the server, melee state bonuses
    in `StateBonuses`, scripts that never touch the damage exempt).
  - `pet:<n>` picks the nth `m_Controlled` unit by GUID (plain `pet` unchanged: `pet <n>` would clash with
    `threat`'s and `armor`'s numbers). A pet's record adds `owner`; `info` for one adds `pet` (scaling aura
    amounts, base stats, white damage, `Creature::Regenerate`'s regen).
  - `TestSimvalStrikes` (`run.sh` without a name skips it and EC-BUFFS' `TestSimvalBuffs`): 8 melee bots on
    golden-setup weapons, bare and talented; 7 pet bots (tamed wolf, Imp, Felguard, Infernal, ghoul, Gargoyle,
    Spirit Wolves, Water Elemental, Treants, Shadowfiend), `info` and `damage`, and Ghoul Frenzy's `timing`.
    Bots, strikes and leads: `tmp/ec-strikes-cpp.md`.
- Sim (`damage.go`; no live capture yet, so `TestStrikesRecords` skips): strikes compare on the sim's strike
  with each weapon pinned to a range end and fixed bonus, percent and hand attack power pinned to the record's;
  pets on the sim pet with the owner pinned to its snapshot (`simPets`); Ghoul Frenzy through the timing replay
  with its talent. Method in the simval README's Damage probe. Unit tests run every e2e strike, pet and spell
  through it, with hand-checked server arithmetic for Mortal Strike, Seal of Command, off-hand Mutilate and
  Aimed Shot. Leads seen offline, for the live run: the sim's Ghoul Frenzy heals every 6 s ×5, the server's
  aura every 3 s ×10; the sim's Imp Firebolt crits ×2, the server's magic spells ×1.5; the sim's cat and bear
  weapons (43–66, 109–165) aren't the server's form damage (`min(level, 60)` × 0.85–1.25 × the form's speed:
  51–75, 127.5–187.5). Live notes: `tmp/ec-strikes-sim.md`.
- Live (W, 8 runs; `strikes.jsonl` is the last, all 15 subtests passing, 102 records, 87 compared). Probe fixes
  the runs forced, all in `strikes_test.go`:
  - Inferno and Force of Nature failed `SPELL_FAILED_LINE_OF_SIGHT` even 3 yards beyond the bot (the east wing's
    entry): their owner now moves 16 yards past the dummy into the hall and casts at its feet (`away`), clear of
    Inferno's 10-yard stun and Immolation's 8 yards plus the dummy's 2.5 reach. A failed summon now names its
    cast error.
  - Summon Gargoyle leaves its own periodic dummy aura on the dummy; the pet precondition allows it.
  - `.pet create` on the level 79 wolf kept level 79 create stats under a level 80 label (`Player::CreatePet`):
    the hunter tames Alterac Valley's level 80 Frostwolf (37294); `simPets` follows.
  - `.pet create` casts Hunter Pet Scaling 01–03 before the pet has an owner, so they read 0 until their first
    2 s refresh: the probe waits 2.5 s and requires an attack power share. The wolf's attack and spell power
    (22% and 12.87% of ranged AP) then agree.
  - The cat's form recast sometimes stripped `.aura`'d Predatory Instincts and Feral Instinct (passives needing
    the form; the recast restores only learned ones): the talent pass puts its auras on again after a recast.

  Verdicts: every warrior, paladin, rogue, death knight, shaman and hunter strike agrees, bare and talented, and
  Bite, Firebolt's amounts, Cleave's bonus and crit, Immolation, Claw, Waterbolt and hunter pet focus (24 per 4 s).
  Known mismatches (`knownStrikesMismatches`, fixture README rows; nothing dropped): cat and bear form weapons;
  the Felguard's weapon (88.8–133.3 vs `pet_levelstats`' 329–495); Imp Firebolt crit (×2 vs ×1.5); Ghoul Frenzy
  (6 s ×5 vs 3 s ×10); pet `info` base and total stats, armor, attack and spell power (not the wolf's), and Imp, Felguard and
  Water Elemental mana regen (the sim's pet constants and owner coefficients vs `pet_levelstats`, guardian
  defaults and the scaling auras). Not checkable: the Infernal's raid buffs (the e2e gives no party buffs). Force
  of Nature: the talent's 33831 summons Treant 1964, the sim casts 65861 (Treant 36070).

### EC-BUFFS (V, done; golden-neutral; stages: code, live)

- e2e `e2e/buffs_test.go`: on a golden setup's character, each `FullBuffs` buff alone, then pairs in each exclusive
  category, with `info` after each; on the boss dummy, each `FullDebuffs` debuff with `armor`, a `damage` probe and
  the derived crit of `yellow`/`spell`. Talent-improved buffs come from a second bot with the talent casting them.
- Sim (`tools/simval/buffs.go`, `TestBuffRecords`, `buffs.jsonl`): the sheet change per buff and the armor, taken and
  crit change per debuff against the sim's.
- **Owns:** `e2e/buffs_test.go`, `tools/simval/buffs.go`, its test and fixture, its fixture README rows, the simval
  README's Buffs.

#### As built

Stage code (no live run yet):
- Character: RogueAssassination (magic and bleed probes, no own raid buffs; mp5 still shows in its regen while casting).
  One warrior caster bot in its party applies everything: talent rank spells and area auras by `.aura` on itself, so the
  character only gets the raid-member part (Unleashed Rage's expertise stays on the caster).
- States: 46 single switches (9 in `unprobedBuffs`: Demonic Pact, Thorns, Mana Tide, both Replenishments, both
  Judgements, Shadow Embrace, Expose Armor) and 21 pairs, between two bare states. Run about 5 min.
- Sim: `newBuffSim` puts the keys' full values in the request; `buffHeldAuras` keep Bloodlust and Sunder up.
  Lines made from the sim agree on all but Gift of Arthas, which moves nothing (Garrote lacks
  `SpellFlagIncludeTargetBonusDamage`).

Stage live (6 runs; `buffs.jsonl` is the last, passing, 69 lines). Probe fixes the runs forced:
- The caster joins from outside Naxxramas: the server refuses an invite across two instances of one map
  (`ERR_TARGET_NOT_IN_INSTANCE_S`), and a member without a permanent bind then enters its leader's instance.
- `.unaura all` on the caster clears Totem of Wrath's 30708, an area aura on enemies the caster owns but never has;
  per-id `.unaura` left it on the dummy for every later state.
- Sunder's stacks by repeated `.aura` (`.aura stack` added none); Blessing of Sanctuary also wants its marker 68066;
  Kings by `.aura` on the character, since Blessings (spell_group 1010) don't stack from one caster.
- `buffsSelect` waits for `.guid` to report the selection (map thread) before the next GM command (world thread);
  after a talent `.aura`, a wait for the next update, which applies its spell mods (Strength of Earth and Devotion Aura
  came in unimproved without it).
- Expose Armor moved to `unprobedBuffs`, with the Sunder+Expose pair: its duration is its caster's combo points, and
  neither the caster's cast after Combo Point (34071) nor `.aura` after the dummy's own kept it on.
- Sim side: the dummy's resistances stop at 0 like the server's field, so Curse of the Elements' -165 shows on neither;
  the resist roll clamps the same on both.

Verdicts: 65 states agree, the closing bare line moves nothing, 2 pairs disagree, dropped with fixture README rows:
- Curse of Weakness + Vindication: exclusive-highest AP Debuffs (1062) removes the whole Curse, its 5% armor too.
- Insect Swarm + Scorpid Sting: -6% hit on the server, since a creature's `m_modMeleeHitChance` sums both and the
  same-effect rule (1060) never reaches it.

Leads: Improved Curse of Weakness's armor agrees at 5%; Moonkin Aura and Leader of the Pack land without a form. Gift of
Arthas (+8 taken, all schools) adds +3 per poison hit and +1 per Garrote tick on the server and nothing in the sim,
inside the 1-point truncation tolerance, so it agrees.

Review: `buffsSelect` (was unprefixed `selectUnit`); a pair's `off` takes only its own wanted auras, so a missing
baseline aura stays `missing` (the captured run's lines are unchanged by it).

### EC-GEAR (W, done; golden-neutral; C++; stages: C++ and e2e, sim, live)

- `.simval enchant <slot> <enchantId>` (`ApplyEnchantment` off, `SetEnchantment`, on) and
  `.simval gem <slot> <socket> <gemItemId>` (`HandleSocketOpcode`'s steps: socket enchant, socket bonus, meta toggle;
  reference: playerbots' `PlayerbotFactory::ApplyEnchantAndGemsNew`).
- `goldchar_setups.json` gains the gear set's gems and enchants; `TestSimvalGoldenCharacters` applies them;
  `goldchar.jsonl` is recaptured, with the regen fields, and `TestGoldenCharacters` builds the full gear and
  compares regen. EC-SHEET's checks must still pass.
- **Owns:** module `src/SimValidationCommands.cpp`, `simval_commands.sql`, `e2e/goldchar_test.go`, its module README
  rows; `goldchar_setups.json`, `goldchar.jsonl`, `goldchar.go`'s setup building, `tools/simval/goldchar_test.go`,
  its fixture README rows, the simval README's Golden characters.

#### As built

Stage C++ and e2e (compile-checked with `-fsyntax-only -Werror`, e2e vetted; not rebuilt, no live run):
- `.simval enchant` takes the enchant only if some spell enchanting with it passes `Spell::CheckItems`' item checks;
  a socket-adding enchant (belt buckle 3729, socket bracer 3717, gloves 3723) goes in the prismatic slot. `.simval
  gem` is `HandleSocketOpcode` for one socket. Both refuse an enchant the character's skill or level keeps inactive
  (`needs skill <id> at <value>`), which `ApplyEnchantment` would skip silently. `.simval hp` with yourself selected
  sets your health without `.modify hp`'s max health pin. `info`'s equipment adds each item's enchantments, gems and
  `inactive` ones; record format in the module README.
- `goldchar_setups.json` gains `enchants` and `gems` from the gear set (`goldchar.go`'s setup building; only enchants
  `db.json` knows, as the sim drops the rest), a socket-adding enchant per gem past an item's sockets; HunterMM's auras
  lose Trueshot Aura. `goldcharPlayer` stays gemless, so buffs and the sweeps build what they did.
- `TestSimvalGoldenCharacters` learns Parry, applies enchants then gems (`.setskill <id> 450 450` and a retry on a
  `needs skill` refusal), sets full health with `.simval hp 100` 2 s after the auras, and holds the record's gear
  to the setup. Plan for the sim and live stages: `tmp/ec-gear-cpp.md`.
- Offline from the live DBCs: every enchant has a spell that takes its item, every meta gem's colors are met.
  DruidFeralTank's gear set names 5 enchants by spell or item id, which the sim drops (follow-up).

Stage sim (offline; live stage plan in `tmp/ec-gear-sim.md`):
- `TestGoldenCharacters` replays a record wearing exactly the setup's enchants and gems geared (`wornBy`,
  `gearItems`), one from before those fields bare; buffs.go and the sweeps stay bare.
- Geared info adds `gearChecks` (socket bonus, meta gem, skipped enchantments); a record with `regen` gets the timing
  replay's `checkRegen`. Tested on hand-built records in the new format, bare and geared for every setup.
- The committed capture still passes unchanged: 1565 checks, all 32 bots bare, no regen. The Parry, max health and
  HunterMM AP/RAP drops stay until the recapture.

Review: `.simval hp` acts on you only with yourself selected (nothing selected refuses again, as other tests rely on);
`.simval enchant` refuses an enchant no spell puts on.

Stage live (server build of 2026-10-04 13:50):
- First full run: PriestShadow's info record carried Lightweave's proc (55637, +295 spell power) from the self buff
  casts. The e2e now `.unaura`s it before recording and fails if it's still on. The rerun's 131 records (32 setups,
  1080 s; the module README says about 18 minutes) are `goldchar.jsonl`.
- `TestGoldenCharacters`: 1996 checks, every bot geared and with regen. The capture faults' 18 drops (Parry, max
  health, HunterMM AP/RAP) pass and are gone.
- Real sim mismatches, dropped with fixture README rows (follow-ups):
  - Lightweave Embroidery's +1 spirit, missing from the sim's enchant (10 setups).
  - Weapon feral AP rounding: 1 AP short in DruidBalancePhase3 and DruidFeralApl.
  - Rune of the Stoneskin Gargoyle's defense: the server takes it as skill, undiminished (DK tank dodge, parry).
  - Crit damage auras: the server adds the meta gem's 3% to Predatory Instincts, Prey on the Weak and Poleaxe
    Specialization; the sim multiplies (Rake, poisons, Thunder Clap, Heroic Throw).
  - DK Icy Touch and Death Coil are magic class on the server: 1.5 base with the meta, doubled by Runic Focus (2.09);
    the sim uses the melee 2.0 base (2.06).

### EC-MOBS (W, done; golden-neutral; module SQL and config; stages: SQL and e2e, sim, live)

- Undead, demon, elemental, dragonkin and mechanical dummies 999004–999008: extend
  `data/sql/db-world/base/simval_dummies.sql`'s `DELETE` list and rows, same script. Add them to
  `AC_DUNGEON_SCALE_DISABLED_ID` in `[ac]/configurationOverrides/DungeonScale.env` (the user's OK, 2026-10-02); the
  rebuild's `docker compose up -d` applies both.
- e2e `e2e/mobs_test.go`, in Naxxramas like the others: melee and spell `damage` and crit against each type, with the
  creature-type auras the sim implements (Exorcism, Improved Tracking, slayer items, …).
- Sim (`tools/simval/mobs.go`, its test and fixture): the type modifiers against the sim's.
- **Owns:** module `data/sql/db-world/base/simval_dummies.sql`, `e2e/mobs_test.go`, the module README's dummy table
  and Requirements line; `[ac]/configurationOverrides/DungeonScale.env`; `docs/guide/azerothcore-server.md`'s dummy
  ids; `tools/simval/mobs.go`, its test and fixture, its fixture README rows.

#### As built

- SQL and e2e (e2e vetted; no rebuild yet, so no live run):
  - Dummies 999004–999008 are 999001's rows with only type, name and subname changed; all in
    `AC_DUNGEON_SCALE_DISABLED_ID`. Giant is probed on 999003 (level 80): the boss adds world boss rules.
  - `TestSimvalMobs` (`run.sh` without a name skips it): human warrior, human paladin and troll hunter, armed and
    stripped, through per-effect states on each of 8 dummies; a `MOBS` line per state and dummy. Asserts dummy
    type and health, parry by type, Exorcism's crit, Holy Wrath's and Tendon Rip's target selection, and each
    state's check probe moving most on its types. Bots, states, line shape and live command:
    `tmp/ec-mobs-sql.md`.
  - Leads from server source, for the live run: Elixir of Demonslaying is +105 AP (sim 265); the Blessed Undead
    Slaying set's three auras multiply (3pc ×1.0302, 4pc ×1.0611; sim 1.02, 1.03); versus attack power reaches
    only weapon damage (`MeleeDamageBonusDone`), never a spell's AP coefficient, which the sim's
    `MeleeAttackPower()` gives it; Predatory Strikes ignores weapon enchants, so Scourgebane adds no feral AP
    (not probeable before `.simval enchant`). Holy Wrath, Giant Slayer, Exorcism, Crusade, Improved Tracking
    (with a tracking up), Sense Undead's glyph (with Sense Undead up) and Beast Slaying agree with the sim.
- Sim (`tools/simval/mobs.go`, `TestMobRecords`; no capture yet, so it skips): each line against the same state on the
  mechanical dummy, the server's change against the sim's between those two types (Holy Wrath's undead against its
  demon); probe amounts as ratios, crit taken, attack power, white parry and plain swing mean, bare target selections.
  Method in the simval README's Mob types. `gen_server_defaults` knows 999004–999008. The e2e adds a night elf druid
  in cat form with Predatory Strikes, bare and with Scourgebane's enchant by `.simval enchant`, for the last lead.
  Leads the hand-checked tests confirm in the sim's own code, for the live run: Crusader Strike gets 75% of a versus
  bonus the server adds after the weapon percent (by the code, Mangle and Shred too, at 200% and 225%); Holy Wrath's AP
  part takes Scourgebane's; the druid's form gains 28 AP from Scourgebane on undead. Live notes: `tmp/ec-mobs-sim.md`.
- Live (2 runs; `mobs.jsonl` is the second, passing, 128 lines; replay: 49 agree, 58 move nothing (the same lines as a
  sim-made run), 16 references, 5 disagree, their failing checks dropped with fixture README rows). Probe fixes the
  runs forced:
  - Armor can't go on or off in combat (`EQUIP_ERR_NOT_IN_COMBAT`, equip error 60), and bare's `threat` engages the
    dummy: `.combatstop` before the set items.
  - In a druid form `info` shows no main hand, so the sim druid had no weapon or enchant: `mobBots` gives each bot's
    main hand for that case.

  Verdicts, all four leads confirmed: Elixir of Demonslaying +105 AP on the server (sim 265); the Blessed set's 3pc
  and 4pc multiply (×1.0302, ×1.0611; sim 1.02, 1.03; 2pc agrees); Scourgebane leaves Exorcism and Holy Wrath
  unchanged on the server and adds +33 to Crusader Strike after its 75% (sim 24.75); the druid's form AP ignores the
  enchant (sim +28). Mangle and Shred with Scourgebane get +10 on the server and about +20 and +22.5 in the sim
  (scaled by 200% and 225%), but that passes inside `strikeTolerance`. Everything else agrees: Scourgebane and Demon
  Slaying on 100% strikes and white, Crusade, Sense Undead's glyph, Improved Tracking, Beast Slaying, Exorcism's crit,
  Holy Wrath's and Tendon Rip's selections, parry only on humanoids.

  Review: a white mean moving inside its sampling bounds no longer counts as a move (49 lines had agreed on noise
  alone); generic test names got the `mob` prefix.

### EC-FIGHT (X, done; golden-neutral; stages: sim tooling, e2e, live)

- e2e `e2e/fight_test.go`: per golden setup, a 1–2 min fight on the boss dummy in Naxxramas under Chronicle; the bot
  runs a fixed cycle that never reacts to procs. Save the log and setup name like `TestRecordedRun`.
- Sim replay, no APL (`waitUntil` re-polls only every 50 ms and not during channels): build the setup's character
  with an empty rotation and schedule each logged cast with `sim.AddPendingAction` at its start, the logged
  `SPELL_CAST_SUCCESS` minus the sim's own hasted cast time (a channel's success is its start); `spell.Cast`. A
  cast the sim refuses counts as a miss, never retried. Auto attack starts at the log's first swing.
- Compare per ability (with `chronicle`'s per-outcome stats): casts the sim lands within a map tick (100 ms) of the
  log, crit %, mean non-crit and crit, ticks per application and spacing, proc counts, aura uptime, energize totals.
- Committed fights in `sim/core/testdata/fights/`, replayed under `go test` (no DBCs: the setup comes from the sim).
- Limits: the dummy's health never drops and it never attacks; swings carry no hand flag; a refresh counts as an
  application.
- **Owns:** `e2e/fight_test.go`, `e2e/run.sh` (Chronicle and output mounts), `tools/simval/fight.go` and its test,
  `sim/core/testdata/fights/`, the simval README's Fights; `chronicle.go` read-only.

#### As built

Stage sim (offline; e2e and live plan in `tmp/ec-fight-sim.md`):
- `sim/core/testdata/fights/cycles.json`: a proc-blind cycle per golden setup (90 s, distance, facing, hunter
  ammo, Auto Shot as a hunter's `before`), held by `TestFightCycles`. Left out: on-next-swing spells, spells
  whose cast time a proc changes (Fury Slam, Ret Exorcism, Fire Pyroblast, Enhancement Lightning Bolt), repeats
  inside their cooldown. Slack before hardcasts, slower rogue, cat and Protection warrior steps.
- `tools/simval/fight.go`, `simval fight` (replay, `-trim`): method in the simval README's Fights. Departures from
  the spec, each forced by a bot-driven self-test: per-fight range checks over 1000 iterations instead of pooled
  z (crits, means and auras correlate within a fight; rare events have long tails); a cast blocked by the sim's
  own cast, GCD or cooldown up to 2 s waits and lands late (still a miss) so the rest of the fight stays aligned;
  `casts` allows half the logged casts, or 2, to miss (proc and power divergence); applications count expiry
  extensions (rollovers and in-place refreshes aren't Activate calls); an event every map update (the sim only
  expires auras on events); the empty APL's 50 ms GCD hold is cleared before a cast.
- `TestFightReplaysTheBotsOwnFight`: every cycle run by a sim bot, written as a Chronicle log and replayed, all
  agree; 512 such fights over 16 seeds had no false failure. `TestFightCatchesMismatches` catches five kinds of
  disagreement. No capture fits a golden setup; a scratch replay of the Affliction, Ret and Prot captures with
  their own characters ran end to end (failures: ids the sim books differently, the captures' own gear) and seeded `fightSimActions` (Life Tap, paladin judgements and talents).

Stage e2e (vetted, no live run; run command in `tmp/ec-fight-e2e.md`):
- `TestSimvalFight` (module `e2e/fight_test.go`, method in the module README): golden build without probes, `.learn`
  of the cycle's spells, hunter ammo, mana filled by `.modify mana <maxPower>`, the cycle's spot (a ranged bot in
  front turns the dummy, as north is behind it), no pet on either side, so pet abilities are left out; guardians a
  proc summons (Bloodworms) are compared as they come. A spell that never went off fails the setup.
- The build's own casts land in the log (Chronicle logs every spell go, triggered too), and DKs cast Horn of Winter,
  a cycle spell, as an aura. The e2e names the log after the fight's first second on the log's clock (its clock
  drifts from the container's), and `fightStartFloor` makes the replay and trim ignore earlier casts and swings.
- `run.sh`: `SIMVAL_FIGHT_CYCLES`, `SIMVAL_CHRONICLE_LOGS`, `SIMVAL_FIGHT_OUT` mounts, `SIMVAL_FIGHT_ONLY`, and
  `TestSimvalFight` in the default `-skip`. About 3 min per setup.

Stage live (2026-10-04; the 32 committed fights are from the worldserver build of 20:33, rev 15282384e, which
someone else rebuilt and restarted mid-stage; 106 KB):
- e2e fixes the runs forced: 4 yards north of the dummy has no line of sight to it (Bloodthirst refused
  `LINE_OF_SIGHT`, no swing landed), so melee stays south and a `behind` cycle turns the dummy away; equipping starts
  item proc cooldowns (`Player::ApplyEquipCooldown`, trinkets first proccing 30 s in), so the gear goes on under
  `.cheat cooldown`; a `learn` cycle field teaches passives the sim assumes (Savage Defense 62600, a trainer spell).
- Cycle fixes: Sunder Armor 7386, as rank 47467 is a flat-armor Sunder on the server (Fury ran 32% over the sim);
  spells that never went off (rune-starved Howling Blast and Death Strike, Multi-Shot under Aimed Shot's cooldown,
  Earth Shock under Flame Shock's) dropped or re-timed, and the bot self-test now fails such a spell.
- Replay fixes, each a false failure: only damage on the dummy counts (a Naxxramas critter took a Thunder Clap,
  Shadow Word: Death's backlash); metrics-only damage (Necrosis, Unholy Blight) and silent misses (Corruption);
  aura applications count stack and charge updates, the player's per 10 ms world tick; permanent triggers sharing a
  proc's id skipped; pre-fight procs start the sim's internal cooldown; a gain gets one tick of slack at the edges;
  72 server-to-sim id pairs and 30 representation gaps (`fightAurasLeftOut` and kin) with printed reasons.
- Verdicts (after both reviews): 2047 checks over 32 fights; 45 fail on real mismatches, dropped with fights README rows by
  cause (follow-ups): world tick swings, early dummy ticks, Bloodworms without party buffs (the server never buffs an uncontrollable
  guardian), Master of Elements' gain sizes (Burnout netted in, no refund on no-cost crits), Frost Vulnerability's misses,
  Vindication's proc rate, Insect Swarm's Vengeance crits, Eclipse on any hit, dot-tick procs
  (Dragon Soul, Greatness, Dying Curse), Rake's armor, Torment the Weak, Enduring Winter's cooldown, Crusader Strike
  refreshing judgements, Prey on the Weak's 10 s lapses, Hemorrhage's debuff, Lava Lash's off-hand halving, Rend's
  ticks 3 to 11% high.

Review: the sim's target health percent was the time left, so its Rend lost the +35% above 75% health after the
first quarter while the server's barely scratched dummy kept it; the replay now puts the dummy's max health on the
encounter. That hid a real Rend gap (Arms passed only as the two errors cancelled), now dropped for Arms and
Protection. A persistent area aura's missed tick, left out on purpose, came back through `silentMisses` as a direct
miss (Consecration: about 3 a fight against the log's 0, passing only as 0 was in range). `TestFightReplays` now
wants one fight per setup. 256 bot fights (seeds 2 to 9) had no false check failure; twice a finisher cast once a
cycle never landed (Feral APL's Savage Roar, Subtlety's Eviscerate), which would fail the e2e (follow-up).

Cross-review: power gains are also checked gain by gain (`gain sizes`), since totals hid Master of Elements'
wrong sizes; Hot Streak's sim-only crit counter no longer counts as Hot Streak (`fightSimAurasLeftOut`); uptime
slack grows with gains from off, not every refresh, exposing 60486 (Shadow, Smite) and Ret's 60314; auras logged
only as removed make no checks. Confirmed in source: players update every world tick (`Map::Update`), creatures
on the map update.

### EC-RECHECK (X2, done; golden-neutral; stages: code, live)

- Every fixture predates the server's `15282384e` merge (2026-10-04). Re-record on the current build, each with its
  own capture ([testing](../guide/testing.md#against-the-server)): `simval.jsonl` (the default run), `damage.jsonl`,
  `mobs.jsonl`, `goldchar.jsonl`, `buffs.jsonl` and the fights.
- Expose Armor joins the buffs sweep (`buffsEntries`, `buffsPairs`), its pair list kept in one place for the e2e and
  the replay; the golden build becomes one e2e helper `goldchar_test.go`, `buffs_test.go` and `fight_test.go` share.
- A record that moved: name the server change behind it, keep the new record, and drop what the sim now gets wrong
  (a README row; report it for Y or Z by area).
- **Owns:** those fixtures, their replays' drop lists and README rows; e2e `goldchar_test.go`, `buffs_test.go`,
  `fight_test.go`, `damage_test.go`, `mobs_test.go`; `tools/simval/buffs.go`.

#### As built

Stage code (e2e vetted; no live run; live plan in `tmp/ec-recheck-code.md`):
- Expose Armor: `buffsEntries` gains it (`comboPoints: 5`: `.simval combopoints 5` on the dummy, then `.cast 8647
  triggered`); `.cast triggered` skips the points check (`TRIGGERED_IGNORE_COMBO_POINTS`) but the duration still reads
  the caster's points (`WorldObject::CalcSpellDuration`, 30 s at 5), and none of the caster's other casts has a duration
  range. It leaves `unprobedBuffs`; `buffHeldAuras` holds 8647, which the sim schedules 3 s in. Pair: Sunder Armor +
  Expose Armor (major armor, one wins in the sim).
- Pairs live in `sim/core/testdata/simval/buffs_pairs.json`: `loadBuffPairs` (sim) and `SIMVAL_BUFFS_PAIRS` (e2e, which
  now skips without it; `run.sh` mounts it).
- `goldcharBuild` (`goldchar_test.go`) is the one golden build: `goldcharOptions` `bare` (buffs: no trainer spells,
  enchants, gems, health top-up), `learn`, `noEquipCooldowns` and `geared` (fight: cycle spells, `.cheat cooldown`,
  ammo). Each caller sends the same commands in the same order as before.

Stage live-records (notes in `tmp/ec-recheck-live.md`): `simval.jsonl`, `damage.jsonl`, `goldchar.jsonl`,
`buffs.jsonl` and `mobs.jsonl` re-recorded on the X2 build; no deterministic field moved, no new drop, every old drop
still fails as its row says.
- Default run: two tests reran alone (a tank warrior's `.cast` raced its homebind transfer; Mind Flay hit its 100%
  resist bucket, reachable only when `float(rand_norm())` rounds to 1.0f).
- Buffs: the e2e puts a state on again up to 3 times (`buffsRetries`), since a cast at the dummy rolls hit. Expose
  Armor is -20%, and with Sunder Armor still -20% (spell_group 1015): exclusive, as in the sim.
- Spelldump recapture (report only, `tmp/ec-recheck-spelldump.md`): 32 spells moved; one sim gap, Mage T8 4P at 20%
  (e9517efd2), for EC-ARCANE-FIX.

Stage live-fights (notes in `tmp/ec-recheck-fights.md`): all 32 fights re-recorded on the 16:52 build (rev 3d89793c7);
no combat change since the old build, so what moved is roll spread.
- Reruns: DeathknightFrost (Howling Blast never had its runes), ShamanElemental (login timeouts).
- New drops, each a known mechanism: DruidFeral's Rake crit mean; MageFrost's Brain Freeze applications (the sim's
  Fireball! never expires, the server's 57761 lasts 15 s); PaladinProtection's Vindication misses (26017 rolls spell
  hit).
- Six drops whose checks passed this capture went with their rows, the `early ticks` cause with them.

### EC-COVER (X2, done; golden-neutral; stages: code, live)

- The talent sweep's never-seen talents (test-enrichment INVESTIGATION, TE-SWEEP as built): armor talents (DK
  Toughness, both Thick Hides, Survival of the Fittest's armor) on a path character wearing armor; buff talents with
  their buff cast (Swift Retribution, Improved Devotion Aura, Commanding Presence, Improved Inner Fire, Improved Imp,
  Enhancing Totems). Then re-record `talents.jsonl` and `spellmods.jsonl`, which runs `sweepPrepare` live.
- The Infernal's raid buffs in the strikes probe; re-record `strikes.jsonl`.
- New mismatches as in EC-RECHECK.
- **Owns:** `talent_paths.json`, `talents.jsonl`, `spellmods.jsonl`, `strikes.jsonl`, their replays' drops and README
  rows; e2e `talent_sweep_test.go`, `spellmod_sweep_test.go`, `strikes_test.go`.

#### As built

Code stage (golden-neutral, no live run; notes and live plan in `tmp/ec-cover-code.md`):
- Paths (`talentpaths.go`, steps unchanged): DeathknightFrost, HunterBeastMastery and DruidFeralBear wear a stamina
  and frost resistance chest (43586, 43593, 43590; crit or hit moved old spell-mod lines); WarriorFury, both paladins,
  PriestDiscipline, WarlockAffliction and ShamanEnhancement cast the buff their talent improves themselves. The sim
  gets each through a spec option (`sweepBuffOptions`, `newPathSim`).
- Server (source): effect spell mods come from the caster's mod owner (`SpellEffectInfo::CalcValue`), so a self-cast
  buff equals the pet's or totem's; Swift Retribution is 63531's EFFECT3, kept only under the paladin's own aura.
  Every listed talent has a route.
- Committed fixtures keep passing: `sweepRerecord` replays the 9 changed paths as their records stood; the replay now
  also holds each record to the path's auras. Old spell-mod lines pass unchanged.
- Infernal: a controllable guardian (SummonProperties 711), so area auras and raid casts target it, but its scaling
  auras make it immune to their aura types (stats, armor, AP, spell damage; `spell_warl_infernal_scaling`). The e2e
  records a second `info` under Strength of Earth, Devotion Aura, Flametongue Totem and Battle Shout; the replay
  compares what they added with the sim pet's raid buffs (`checkPetBuffs`), pending in `strikesUnrecorded`.
- Predicted live mismatches, from source: Improved Devotion Aura (sim needs 5 of 3 ranks), Improved Imp (sim
  improves Blood Pact at 2/3 only), Enhancing Totems on Flametongue (sim fixed 144), the Infernal's raid buffs (sim
  gives them; also the treant, Water Elemental, Shadowfiend and Spirit Wolf scaling scripts' immunity).

Live stage (server `56ee3ed2b`; notes in `tmp/ec-cover-live.md`): `talents.jsonl` (716 records, 30 paths),
`spellmods.jsonl` (2251 lines) and `strikes.jsonl` (103) re-recorded; `sweepRerecord` and `strikesUnrecorded` emptied.
- All 10 never-seen talents now move a field. Agree: DK Toughness, both Thick Hides, Survival of the
  Fittest's armor, Swift Retribution, Commanding Presence, Improved Inner Fire, Enhancing Totems' Strength of Earth.
- Confirmed and dropped (fixture README rows): Improved Devotion Aura armor (+602, sim 0), Improved Imp max health
  (+398, sim 0), Enhancing Totems' Flametongue spell power (+21, sim 0); the Infernal's buffed `info` adds nothing on
  the server, the sim +155 str/agi, +860 AP, +144 SP, +1515 armor (`knownStrikesMismatches`).
- RogueCombat's Endurance step is recorded and agrees: its drop is gone.
- Server change: `27e096204` (SPELLMOD_COST as (base + flat) × percent) moved Berserk's Mangle (Cat) and Shred costs,
  dropped; it also clamps a cost at 0, so Endless Winter's Mind Freeze frost rune cost went -199 to 0 (not compared:
  the sim has no Mind Freeze). No talent record moved on an unchanged path.
- Not a server change: the Water Elemental's melee damage (86–101 to 85–100). Its template levels are 60–70, so
  `Creature::SelectLevel` rolls its base damage from a random level's `creature_classlevelstats` row (64, then 63),
  which its summon keeps.
- Probe fix: the recast waits 1.5 s, since 63514 and 63531 need the paladin's aura and drop off for a target map
  update after each recast (the talent sweep's Swift Retribution haste landed two steps late).

### EC-PROBES (X2, done; golden-neutral; C++; stages: C++ and e2e, live)

- `.simval timing` with combo points (Rupture, Slice and Dice; the timing README row goes); `.simval threat`'s assist
  threat with other creatures in combat with the target. Re-record `timing.jsonl` and `threat.jsonl`.
- The rebuild is the user's, once, after X2's code stages; every live stage runs on that build.
- **Owns:** the module's C++ and `simval_commands.sql`, e2e `timing_test.go`, `threat_test.go`, `tools/simval/timing*.go`
  and `threat*.go`, both fixtures and their README rows.

#### As built

C++ and e2e (`-Werror -fsyntax-only` clean, e2e vetted; notes in `tmp/ec-probes-cpp.md`):
- `.simval combopoints <1-5>` sets exactly that many on the selected unit (`Unit::AddComboPoints`). `.simval engage`
  puts the selected dummy in combat with you, restarting its combat timeout.
- `.simval timing … combo:<1-5>` gives the caster those points for the probe, its own back after;
  `aura.comboPoints` records them (`WorldObject::CalcSpellDuration` reads them on any target).
- `.simval threat`: an assist bonus goes through the real `ForwardThreatForAssistingMe` on the caster;
  `live.assist` lists each creature fighting it with its share, `live.bonus` is the dummy's.
- `TestSimvalTiming` probes Rupture and Slice and Dice at 1 to 5 points per pass. `TestSimvalThreat` probes Seal of
  Command (ProtPaladin) and Fire Nova with two more dummies engaged: each of three gets a third.

Sim: a finisher record is replayed by casting it with the record's points (none: not compared, `Spell::CheckCast`
refuses it); the timing README row and `comboless` entries went. The threat replay builds a target per sharing
creature, checks the dummy's share as `bonus` and the rest as `assist shares`. New probes waited as recapture
entries (`timingRecapture`, `droppedThreatRecords`) until the live stage recorded them. Holy Shield was dropped
as an assist probe: the sim's cast makes no threat at all (EC-THREAT-FIX2's self buffs).

Live (2026-10-05, build of 16:25): `timing.jsonl` (198 timing, 20 info) and `threat.jsonl` (119) re-recorded; all
replays pass, so `timingRecapture` and the five threat `recapture` drops went, with their rows. Rupture is
6000 + 2000 ms per point, Slice and Dice 6000 + 3000 (+50% talented); Seal of Command and Fire Nova split a third
to each creature, as the sim does. No compared value moved since the old capture; only new fields (EC-GEAR's
equipment, `live.assist`) and pre-cast state (cat energy, the rogue's poison stacks) differ. No new mismatch.

### EC-TICK-FIX (Y, done; changes goldens, full suite)

- The server updates players every world tick (`MinWorldUpdateTime`, 10 ms live) and creatures, pets and guardians
  only on the map update (`Map::Update`, `MapMgr::Update`). An aura updates with the unit it sits on
  (`Unit::_UpdateSpells`), and a creature aura's first update counts a whole map update (`AuraEffect::Update`). The
  sim puts everything on one grid (`Simulation.NextServerTick`).
- Fix: a world-tick grid (a new `ServerSettings` world update interval, 10 ms live, from the config) for a player's
  swings, Auto Shot, hardcast landings and the auras on it; target auras and dots, pets and guardians stay on the map
  update, with the early first tick.
- Done when the fights README's `world tick` drops go. X2's capture left no `early ticks` drop (its ticks fell inside
  the sim's spread): check the first tick against `AuraEffect::Update` instead.
- **Owns:** `sim/core/{sim,attack,cast,aura,dot,periodic_action,server_settings}.go` and their tests,
  `proto/common.proto`'s `ServerSettings`, `tools/acore/gen_server_defaults/`, the server settings UI.
- **Goldens:** all 37; report each.

#### As built

- `ServerSettings.world_update_interval_ms` (`MinWorldUpdateTime`, code default 1, live 10), read by
  `gen_server_defaults` from `Main.cpp`'s key; a UI picker beside the map update's.
- Grids (`ServerSettings.updateGrids`, `sim.go`): players on the world tick (`NextWorldTick`), everything else on the
  map update (`NextServerTick`), which falls on the first world tick its interval is up by, at least 3 ticks apart
  (`MapMgr::Update`'s four steps). One phase draw per iteration as before; the world phase is the map phase modulo the
  tick. Map interval 0 makes every unit exact.
- Per unit (`nextUpdate`): swings, Auto Shot and hardcast landings (also `APLValueSpellCastTime`) on the caster's grid;
  auras and dots on the grid of what counts them down: the unit they sit on, a channel's caster (`Spell::update`; most
  channels tick from a caster aura), or the map update for an area dot from a dynamic object, totem or trap (effects
  27, 28, 105).
- Early first tick, confirmed in source (`Map::Update` hands `Unit::_UpdateSpells` the whole map update's diff) and in
  the fights: 127 first ticks of the dummy's dots land on average 10 ms before cast + amplitude (median -14), against
  +52 if counted from the cast. A creature aura put on or refreshed outside its own update counts down from the map
  update before (`Aura.countdownStart`); one it puts on itself in its own swing, AI instant cast (a GCD or cost),
  dot tick or expiry (`sim.updating`) counts from that update, as `_UpdateSpells` has listed its auras by then. Its
  hardcast lands in the event pass before that, so counts early. An area dot's dynamic object made in a trap's or
  pet's own map update joins the update list only on the next one, so counts from its own (`Aura.madeInMapUpdate`).
- Verdicts: all 7 `world tick` drops fixed and removed (melee counts within range, Fury of the Five Flights' uptime).
  New drop `HunterMM 49001 ticks` (cause `map update length`): the live map update measures 104.4 ms (world ticks
  ~10.4 ms), so a whole-second dot's tick on the sim's exact grid always beats the next Serpent Sting's landing, on the
  server about half the time. Removed `DruidFeral 48574 crit mean` (Rake): it passes on the new random stream (one
  logged crit); the hit-mean drop stays.
- `DelayFromInstantCast` reads the world tick: `Map::Update`'s session pass casts a `CMSG_CAST_SPELL` just before
  that tick's `Player::Update`, so the caster's clock is a world tick stale. In the fights it barely moves Ignite's and
  Righteous Vengeance's totals over their logs (base, world tick, this fix): MageFire +22%, +33%, +34%; MageFrostFire
  +1%, +6%, +6%; Ret +19%, +20%, +20%. Not the tick model: Ignite pays out 0.96 (Fire) and 0.95 (FrostFire) of 40% of
  the fire crits feeding it, the logs 1.04 and 0.97, base 0.88 and 0.89. The excess is those crits: Fire's crit damage
  is 45% over its log (21 crits to 16, Fireball's crit mean +16%).
- Goldens: all 37 move, mostly up: fast and dual-wield melee keep the swing overshoot (Feral +3.4%, Subtlety +4.1%,
  Combat +2.7%, Blood +2.9%), hardcasts land up to 90 ms sooner, dots tick a map update earlier (Fire +7.5%,
  FrostFire +6.6%, both through Ignite's +28 to 33%, Ret +1.3%: the instant-cast delay is longer); Unholy Blight -15%;
  healers move only aura uptimes.
- Tests: `TestFightReplaysTheBotsOwnFight`'s bot log split a real gain over earlier gains of nothing (Thrill of the Hunt
  off a free Lock and Load shot, 48 logged as four 12s), not a capped one; it now uses them up, and all 32 setups pass
  seeds 1 to 4. `TestFightCatchesMismatches` injects Shadow Bolt at 2x: 1.4x fell inside the hit mean's range, whose
  top is one iteration's tail. `TestRetP1QuickPricesBrewfestTrinkets` checks the screening order on Ret's pool with set
  slopes (each Coren trinket ranks at or above every Naxx-or-older trinket with no better stats), and Quick still
  prices the two crit trinkets; the Ancient Pickled Egg ties The Egg of Mortal Essence at Quick's cut.
- `TestCasterCapFloorReachesTheCap` (`sim/optimizer/caps_test.go`): Fire's unconstrained pick lands within a gem
  of its 368 floor either side as the sim moves, so the precondition now asks only that some of the
  unconstrained top loadouts fall short.

### EC-CORE-FIX (Y, done; changes goldens, full suite; stages: crit and procs, gear and debuffs)

- First EC-RECHECK's spelldump recapture (`G:\DevStuff\GitHub\.wave-loop\ec-recheck-tmp\capture\spelldump.jsonl`, 32
  spells moved: 64869's chance, two Heroic Strike levels, Backstab rank 1's behind flag, the Shadow scripts) replaces
  the committed one; then a full `gen_serverdata` run: The Beast Within, Turn the Tables, Totem of Wrath and three
  threat rows the hunter and rogue code reference.
- Crit: the server sums crit damage bonuses (aura 163: meta gem, Predatory Instincts, Prey on the Weak, Poleaxe) for
  spells and specials, which the sim multiplies; DK magic spells crit from the spell base 1.5 (the fixture README's
  goldchar rows).
- Procs on dot ticks (`DONE_PERIODIC`): Illustration of the Dragon Soul, Dying Curse, DMC: Greatness, and Serpent
  Sting's `ProcMaskEmpty` (the fights README's `dot procs`).
- Gear and consumables: Lightweave's +1 spirit; Elixir of Demonslaying's +105 AP; the Undead Slaying set's auras
  multiply; versus-creature attack power only in melee damage, after the weapon percent, never spells (the mobs rows);
  Gift of Arthas on every school; meta gem color conditions.
- Debuffs (the buffs rows): an exclusive AP debuff removes a weaker one whole (Curse of Weakness + Vindication);
  Insect Swarm's and Scorpid Sting's hit debuffs stack.
- Rune and runic power costs on T2's cost arithmetic (`sim/core/cost.go`); cost mods as the server applies them since
  `27e096204`: (base + flat) × percent, clamped at 0 (Berserk's Mangle (Cat) and Shred, the spell-mod rows).
- **Owns:** `sim/core/` but EC-TICK-FIX's and PERF-THREAT's files and `pet.go`; `sim/common/`; `sim/core/serverdata/`;
  the DK crit base, rune costs and Serpent Sting's proc mask; the drop entries and README rows it fixes.
- **Goldens:** every suite that moves; report each.

#### As built

Stage crit and procs (notes in `tmp/ec-core-crit.md`):
- EC-RECHECK's spelldump is committed; `gen_serverdata` found no DB/capture disagreement and moved nothing.
- Crit damage: spells and specials sum the aura 163 bonuses (meta gem, Predatory Instincts, Prey on the Weak,
  Poleaxe) before `SPELLMOD_CRIT_DAMAGE_BONUS` scales the part over 1; white swings multiply them
  (`Character.WhiteCritMultiplier`). DK magic spells (Icy Touch, Death Coil, Blood Boil, Howling Blast, Death and
  Decay) crit from 1.5 with Runic Focus' +100% and Might of Mograine / Guile of Gorefiend; the rune weapon's spell
  and strike copies get the DK's mods, not the meta (its white hits, the DK's own halved, keep it).
- `ProcTrigger.AnyPeriodic` is `DONE_PERIODIC`: any dot or hot tick procs whatever its mask. A tick the server deals
  as a triggered cast (aura 23, 226, 227) only procs a `CanProcFromProcs` aura (`SPELL_ATTR3_CAN_PROC_FROM_PROCS`:
  Greatness, Dying Curse), as a hit of its damage class. Illustration of the Dragon Soul and Dying Curse proc on landed
  hits, heals and ticks, Dying Curse on triggered magic hits too; Greatness on ticks too.
- Verdicts, all fixed, entries and rows gone: goldchar Icy Touch, Death Coil (2.09), Rake (2.26), Instant and Wound
  Poison (1.845), Thunder Clap, Heroic Throw (2.392); the fights' 6 `dot procs`. Serpent Sting keeps
  `ProcMaskEmpty`: a ranged mask would fire the sim's landed-only ranged procs on its no-damage hit, which the
  server's damage-only entries (spellTypeMask 1) skip. WarriorArms' Rend `tick crit mean` drop went stale under the
  new RNG and failed again once merged, so it stays (ticks still ~13% high, EC-MELEE-FIX's).

Stage gear and debuffs:
- Costs (`cost.go`): `(base + flats) × (1 + Σ percents)`, 0 once the flats reach it (`spellModCost`). A DK's runic
  power cost (now tenths in `RuneCost`) takes the whole arithmetic, each rune count the spell mods only
  (`runeCountFor`), replacing `CurCast.Cost *= CostMultiplier`. A rune spell's runic power gain no longer needs a
  cost left to pay (`Spell::TakeRunePower`): Freezing Fog's Howling Blast gains its 15.
- Debuffs: Curse of Weakness' armor is on only while its AP (−573, truncated) is the AP debuff that counts; Insect
  Swarm and Scorpid Sting stack (`spellGroupAllowances` and `stackingBuffPairs` carry the server's reason).
- Gear: the three tailoring embroideries' +1 spirit; Elixir of Demonslaying +105; the Blessed undead sets ×1.01,
  ×1.02, ×1.03 multiplied. Creature-type AP leaves `MeleeAttackPower`/`RangedAttackPower`; a white swing or a spell
  with a weapon effect adds `int32(AP / 14 × speed)` after its weapon percent (`versusAttackPowerBonus`).
- Gift of Arthas is `PseudoStats.BonusDamageTaken`, every school: whole on a weapon hit, else
  `int32(8 × coefficient × stacks)` (`Unit::SpellDamageBonusTaken`: the `spell_bonus_data` value, else
  `CalculateDefaultCoefficient`, which counts a dot's stacks twice and halves for an area target; new serverdata
  `FlagTargetsArea`). Instant Poison +3, Garrote +1, Deadly Poison +38 a tick at 5 stacks.
- Meta gems activate only with their `SpellItemEnchantmentCondition` met (`meta_gems.go`, the live DBC's 47 rows);
  an inactive one gives no stats, no effect and fails `HasMetaGemEquipped`.
- Verdicts, all fixed, entries and rows gone: spell-mod Berserk Mangle (Cat) and Shred cost; buffs CoW + Vindication
  armor, IS + Scorpid hit; goldchar spirit (10 setups); mobs Demonslaying, Blessed 3pc and 4pc, Scourgebane on
  Exorcism, Holy Wrath and Crusader Strike (`TestMobReplayCatchesMismatches` now holds them as agreements). Kept:
  the NightElfDruid row (EC-NATURE-FIX's Predatory Strikes).
- Goldens, both stages (`dock.sh delta`): 34 of 37 suites move; resto druid and shaman, holy paladin don't.
  Average-Default: Feral −1.9%, FeralApl −2.1% (Berserk costs); Frost DK +0.7%, FrostUH +0.65% (Rime's runic
  power); Assassination +0.55%, Subtlety +0.29%, Combat +0.10% (Deadly Poison +2%/hit); Smite +0.66%, Elemental
  +0.46%, Balance +0.45%, Shadow +0.43% (stage 1's procs); casters +0.07 to +0.12% (Gift of Arthas); hunters −0.05 to
  0% (pet Bite takes the spell path); tank dtps −6.8% to −13% (Insect Swarm + Scorpid). AllItems rows with an unmet
  meta fall to the no-meta value (Chaotic Skyflare on Frost DK, Relentless on Balance), as do set rows that pull gemmed
  pieces.

### EC-PET-FIX (Y, done; changes goldens)

- The strikes README's pet rows: create stats from `pet_levelstats` or `Guardian::InitStatsForLevel`, armor, attack
  and spell power from the scaling auras, a controlled creature's regen (`Creature::Regenerate`), the Imp's Firebolt
  crit ×1.5, the Felguard's weapon damage, Ghoul Frenzy every 3 s ×10, Force of Nature's treant.
- Bloodworms never get a party or raid buff (the fights README's `Bloodworms`). Guardians with a scaling script (the
  Infernal, treants, Water Elemental, Shadowfiend, Spirit Wolves, Fire Elemental) are immune to their scaling aura
  types, so raid buffs add nothing (the strikes row); the Water Elemental's base damage comes from a random level 60–70
  `creature_classlevelstats` row per summon.
- Improved Imp improves Blood Pact at every rank (the talents row; `warlock.go` checks 2 of 3).
- **Owns:** `sim/core/pet.go` and each class's pet and guardian files, `warlock.go`'s Blood Pact; their drop entries
  and README rows.
- **Goldens:** the pet suites; report each.

#### As built

Stage 1 (stats), re-measured on `bb4ffd124`:
- `sim/core/pet.go`: `PetCreateStats` (pet_levelstats row, armor less 2 × create agility, as `Guardian::UpdateArmor`
  counts only agility over create) and `GuardianCreateStats` (22/22/25/28/27, 50 armor a level); `PetScalingPct`
  (`CalculatePct` on the truncated owner value, float32, truncated); `ScalingPeriod` (real pets rework their scaling
  every 2 s, Spirit Wolves every 1 s, others snapshot at the summon; `refreshScaling` is the tick);
  `EnableCreatureManaBar` (create mana plus 4.95 or 11.5 a point of intellect over create, percent costs off create
  mana, `Creature::Regenerate`'s trunc(spirit/5 + 17) every 2 s, under the five second rule spirit/8 + 17 for the Imp
  and Water Elemental, else 0; MP5 through core's tick).
- `BuffImmunity` and `Uncontrollable`, applied by `dropBuffsItCannotTake` from a one-line hook in `applyPetBuffEffects`
  (`sim/core/buffs.go`, EC-CORE-FIX's: contract request). Full scaling immunity: Infernal, treants, Shadowfiend,
  Spirit Wolves, Fire Elemental, guardian Water Elemental, Mirror Image; stats and AP only (`spell_dk_pet_scaling`,
  non-pets): Raise Dead's ghoul, gargoyle, army. Bloodworms lose the party and raid casts, keep area auras. A buff
  mixing types (Gift of the Wild's stats and armor) goes whole. Stat-immune summons (Infernal, Spirit Wolves,
  Shadowfiend) skip `ApplyPetConsumeEffects`: pet food and scrolls are stat auras.
- Per pet: create rows for hunter pet (+5% base armor, 17223), Imp, Felguard, Succubus, Felhunter, Infernal, ghoul,
  Water Elemental, Shadowfiend; fallback for gargoyle, Spirit Wolf, treant, army, Bloodworm. AP: Imp Str − 10, ghoul
  589 + Str (+ Agi for the pet only: `UpdateAllStats` sets strength before agility, and nothing moves a guardian's
  strength after), else 2 × Str − 20. Scaling per script, truncated; the Infernal gets no intellect (36186's
  immunities block 36188's, its auras aren't passive); Shadowfiend AP 300% of shadow SP, Twisted Faith's included;
  Spirit Wolf armor 0 (35676 unscripted). Spirit Wolves and the Infernal are guardians now. Treant and Spirit Wolf
  agility crit gone (creature crit). Cleave 10% and Shadow Bite 3% of create mana.
- Verdicts: every pet `info` row (create and total stats, armor, AP, SP, regen, the Infernal's buffed info) fixed,
  its `knownStrikesMismatches` entries and README rows gone; fights `Bloodworms` fixed, drops and rows gone.
- Goldens: the pet suites drop where guardians lose raid buffs (DK −1.8 to −5.9%, Enhancement −2.6%, Balance −1.0 to
  −1.3%, mages −0.4 to −0.6%, Shadow −0.6%, Affliction and Destruction −0.3%); hunters, Demonology ±0.1%; healer
  suites and Restoration lose only metrics entries.

Stage 2 (pets), re-measured on `bb4ffd124`:
- Replay: pet `info` now also compares melee and spell hit, expertise (`auraExpertise`), and the main hand's weapon
  range and attack time; `pinOwner` pins the owner's hit too, and a real pet already up catches up through
  `ResyncScaling` (hit and stat scaling). `petsThatNeverSwing` (Imp, Water Elemental,
  gargoyle) leave the weapon `not compared`.
- Imp Firebolt crit 1.5 + 0.1 × Ruin (`SpellCriticalDamageBonus`: magic class, Ruin's mask matches); fixed.
- Weapons from `InitStatsForLevel`/`pet_levelstats`: Felguard 329–495 (fixed, Cleave compared), Succubus 350–524,
  Felhunter 246–379, Infernal 1263–1768 (its ×3.2 gone), Spirit Wolf 240–400, Shadowfiend 120–280, treant 120–280
  at 1574 ms with a plain ×2 white crit.
- Hit: every pet's 61013/61017 makes it immune to Heroic Presence; the DK's 67561 lets its melee hit through
  (`dropBuffsItCannotTake`; the Fire Elemental's own counter-hack went). Treant's extra 5% hit and 120 expertise
  gone; Shadowfiend gets Mana Leech's (28305) 9% hit and 56 expertise.
- Ghoul Frenzy heals 6% every 3 s, 10 times; fixed.
- Force of Nature: the sim keeps 65861 (33831's cost, cooldown, flags; changing it touches APLs, serverdata, db.json);
  the treants model 33831's 1964.
- Water Elemental's level roll: never reaches combat (`PetAI::_canMeleeAttack` keeps it at Waterbolt range, and it
  doesn't melee out of mana); a `petsThatNeverSwing` reason.
- Improved Imp: the raid buff is Improved at 3 points; at 1–2, `improveBloodPact` raises every raid unit's Blood Pact
  by 10% a point (`TestImprovedImpBloodPactEveryRank`). The sweep row agrees; drop and README row gone.
- Goldens (Average-Default, both stages against `bb4ffd124`): Demonology +2.0% (Felguard 2136 → 2483 a swing),
  Destruction −0.9% and Affliction −0.3% (Infernal 2825 → 1602 a swing without the ×3.2; Felhunter swings +24%),
  Balance −1.5% (treant 931 → 479 a swing, more swings), Shadow −0.4% (Shadowfiend's swings up from stage 1's),
  Enhancement −2.5%, Unholy HPS ×2 (Ghoul Frenzy); other pet suites as stage 1.
- Left: the Fire Elemental's stats, AP and weapon (200–360 on the server) are still estimates no record covers;
  Mirror Image's crit and mana; pets' spell crit from intellect (`EnableManaBarWithModifier`; a creature's is a flat
  5% plus auras), no record checks it.

### EC-THREAT-FIX2 (Z, done; changes goldens, full suite; stages: threat, live)

- V's leftovers, each re-measured with `.simval threat` first: Heart Strike's second target adds the bonus twice (the
  server splits one); per-tick bonuses on aura ticks the server casts (Bladestorm 50622, the Infernal's 20153, Volley);
  `SPELL_ATTR1_NO_THREAT` spells making threat (Healing Stream Totem); Auto Shot's +1; Devastate's Sunder Armor 58567;
  Feint; positive self buffs' assist threat; Elemental Precision and Righteous Fury as school auras; heal threat ×0.5.
- Live: the fixed self buffs join EC-PROBES' `threatAssistProbes`; `TestSimvalThreat` re-records `threat.jsonl`.
- **Owns:** `sim/core/threat.go` and the class threat code above; `tools/simval/threat*.go`'s drops; `threat.jsonl`.
- **Goldens:** every suite whose TPS moves; report each.

#### As built

Threat stage (offline; each lead re-measured against [ac] 3d89793c7 and the spelldump):
- `sim/core/threat.go`:
  - `SPELL_ATTR1_NO_THREAT` spells make no threat at all (Healing Stream Totem's 52042).
  - `SPELL_EFFECT_THREAT`: a flat, unmodified amount on each enemy hit; a cut never takes the unit's threat there
    below 0 (Feint's −2164). Faerie Fire (Feral) 60089 adds 638 on top of its `spell_threat` row.
  - `SetThreatCastOf` puts a second spell's hits in the main spell's cast: Heart Strike's chain hit and the rune
    weapon's copy split one bonus.
  - `ThreatTick`: the bonus of a spell an aura casts each tick, Bladestorm's 50622 (44949 with an off hand) and
    Volley's 58433.
  - Heal threat is half the health gained (a quarter for paladins), modified like a hit and shared over the enemies
    as assist threat, no longer kept on the ally (`spell_result.go`'s heal deal measures the gain).
- Classes: Righteous Fury (holy ×1.8) and Elemental Precision (fire, frost, nature) are school auras. Devastate's
  Sunder reads 58567 (+10, also when the aura can't apply); Auto Shot reads 75 (+1). 41 positive self buffs get
  `ThreatMultiplier` 1, and a school and `ProcMaskEmpty` where missing; spells with no unit target (totems, traps,
  Water Elemental) and externally cast buffs stay at 0. gen_serverdata emits 75 and 58567 (`ThreatSpellID` literals).
- Not changed: the Infernal's 20153 already matches, and pet threat never reaches the owner's TPS.
- Replay: the bonus check leaves out `SPELL_EFFECT_THREAT`, which `.simval threat` doesn't record.

Live stage (`TestSimvalThreat` 13:13 to 13:17, all PASS, no orphan bots):
- `threatAssistProbes` gained 28 self buffs, Shield Block to Bloodlust, each with three creatures fighting.
  `threat.jsonl` re-recorded: 147 records, 345 checks, all pass; the self buffs' recapture drops and README row went.
  Each record's bonus is its `SpellLevel`, modified by the caster's school multiplier, a third to each creature, as
  in the sim.
- Not probed: Shield Wall and Recklessness (the replay's specs don't register them), Berserker Rage and Tiger's
  Fury (the replay's cast hits a nil APL on a resource gain), Power Infusion and Demonic Empowerment (cast on
  another unit). `SPELL_EFFECT_THREAT`, heal threat, Volley and Auto Shot (no hunter bot), Heart Strike's split,
  58567 and Bladestorm's ticks rest on the source only.

Review:
- A summon starts at full health, its owner's stamina in (`Guardian::InitStatsForLevel`; `sim/core/pet.go`).
  It used to start short of that, so Ghoul Frenzy's heals gained health and made heal threat the server's
  undamaged ghoul doesn't.
- Ardent Defender's heal reads 66235, the server's heal spell, not `SPELL_ATTR1_NO_THREAT` 66233, and drops its
  ×0.25 stand-in for the paladin factor. It never procs in the suites.

Goldens (`dock.sh delta`): TPS moves in 34 of 37 suites; `druid/restoration`, `paladin/holy` and `priest/shadow`
don't. Average-Default: Disc +1659% and Holy +2180% (heal threat), Feral Tank +1.4% (heal threat, Faerie Fire's
638), Protection paladin +0.23%, Arms +0.06%, Protection warrior +0.05%, the rest under +0.04%,
Unholy +0.03% (self-buff assist threat). Extremes: SV aoe +6.6% (Volley's ticks), Elemental −1.5% with Reign of the Dead
(Elemental Precision on fire procs), Blood −0.3% multi-target (Heart Strike's split), Restoration shaman 0 →
0.01 (its default APL overheals). DPS moves only with Insightful Earthsiege Diamond (Arcane +0.017%, Demonology
+0.082%, Protection paladin −0.164%, Fire and FrostFire ≤0.002%) and Holy's HPS by ±0.1%: the diamond's
on-cast proc now counts the self buffs' casts (`ProcMaskEmpty`), as 55381's `DONE_SPELL_NONE_DMG_CLASS_POS` does.

### EC-MELEE-FIX (Z, done; changes goldens)

- Death knight: Rune of the Stoneskin Gargoyle's defense as skill (goldchar row); Frost Vulnerability (51714) rolls
  spell hit; Howling Blast with death runes (a fights lead: DeathknightFrost's cycle can starve it of runes for a
  whole fight, failing the e2e at random); the rune weapon's strikes take creature-type AP at half through the DK
  (`MeleeDamageBonusDone` for 27893; a lead); refundable spells gain runic power on a miss (a lead).
- Warrior: Rend's ticks and tick crits 3–13% high (cause not found); Flurry spends no charge on a miss (spell_proc
  -12966's HitMask), the shaman's too.
- Paladin: Vindication at 3 PPM (spell_proc -9452), its 26017 rolling spell hit (no `SPELL_ATTR3_ALWAYS_HIT`);
  Improved Devotion Aura at its 3 ranks (the sim checks 5; the talents row); Crusader Strike refreshes no judgement.
- Rogue: Prey on the Weak's 10 s aura, re-added by its 2 s check only once gone; Hemorrhage's debuff solo; Endurance's
  Sprint and Evasion cooldowns; Honor Among Thieves' combo points from the rogue's own crits only (a lead).
- The Subtlety fight cycle's Eviscerate gets a second step; Fury's 72-point strings in the optimizer's test inputs and
  `gen_db`.
- **Owns:** `sim/{deathknight,warrior,paladin,rogue}/` and the shaman's Flurry, but pet, threat and EC-CORE-FIX's code;
  their drop entries and README rows.

#### As built

Re-measured on `d253e096c`. Verdicts:
- Rend (fixed, 3 drops gone): `spell_warr_rend` adds the weapon fifth after `SpellDamageBonusDone`, so only the base
  takes the done percents; ticks now 278/278 (Protection) and 933/932 (Arms).
- Flurry, warrior and shaman (fixed): charges go on landed white hits only; the buffs last 15 s; a shaman white crit
  spends a charge (and starts the 500 ms cooldown) before the talent refreshes, unless it was the last (rank 2+).
- Vindication (fixed, 3 drops gone): spell_proc -9452 at 3 PPM (`AuraPPMProcChance`) on landed damaging swings, casts
  and `SPELL_ATTR3_NOT_A_PROC` triggered spells (judgements, Seal of Command); the talent casts 67/26017, which rolls
  spell hit. The fight map now names the sim's 26016 aura as 26017 (`fightLogIDs`).
- Crusader Strike (fixed, drop gone): refreshes no judgement. Improved Devotion Aura (fixed, sweep drop gone): improved
  at 3 points; 1–2 points can't ride the tristate (follow-up).
- Prey on the Weak (fixed, 2 drops gone): a 2 s check on the world tick, random phase, puts the 10 s 58670 on while the
  rogue's health % beats the target's (remaining duration stands in) and only once gone; the fifth check still sees it,
  so it's down one check in six. The buff swaps every spell's crit multiplier (aura 163 on all schools; white swings
  multiply). Goldchar mirrors 58670 when the record had it.
- Hemorrhage (fixed, 2 drops gone): the debuff goes on solo; its 10 charges go on any landed hit on the target
  (swing, cast, NOT_A_PROC trigger), not poisons. Fight: 145.8 applications to the log's 146.
- Honor Among Thieves (lead, fixed in part): spell_proc 51701 procs at the finish phase, so crit ticks (Rupture) give
  no combo point; party members' crits do on the server (area aura), so the crit-rate option stays.
- Endurance's Sprint and Evasion cooldowns: no sim gap, the sim has neither spell.
- Frost Vulnerability (fixed, 3 drops gone): enchant 3370's second combat spell, 51714, rolls spell hit.
- Howling Blast (lead, fixed in core): the sim's rune grace was 250 ms too generous (`Player::GetRuneBaseCooldown`
  cuts grace − 250, at most 2250), and death runes went in the wrong order (server: those whose base type is owed,
  then any in slot order blood, unholy, frost). The bot now lands Howling Blast 1.3 times a fight (server 1.2, was
  4.8). The committed fight's 3 casts were all Freezing Fog procs, so the cycle starves it on both sides: new drop
  `DeathknightFrost 51411 casts` (cause Rime) until its cycle gets runes for it and is re-recorded.
- Refundable runic power (lead, fixed in core): `Spell::TakePower` on the main target's miss, dodge or parry gives no
  runic power, puts the runes on `RUNE_MISS_COOLDOWN` (1.5 s) and keeps death runes death (`spendOnResult`); a
  runic power cost (Frost Strike) takes only `irand(0, cost/4)`. Blood Boil has no unit target, so it always pays.
- Blade Barrier (found by the replay once runes moved): `spell_dk_blade_barrier` refreshes on any DK cast leaving both
  blood slots on cooldown, not on every GCD (31.25 applications to the log's 28). The check runs in the cast phase,
  after the cost, before the effects: Blood Tap, Empower Rune Weapon and the casts that skip `OnCastComplete` (Rune
  Tap, Vampiric Blood, Mark of Blood, Bone Shield, ...) call `bladeBarrierCastPhase` first in `ApplyEffects`.
- Stoneskin Gargoyle's defense as skill: confirmed (`Player::UpdateDodgePercentage` takes skill undiminished), needs
  core avoidance (contract request); drop kept.
- Rune weapon's creature-type AP: confirmed (`MeleeDamageBonusDone` goes through the owner at half), a pet-file
  fix (contract request).
- Subtlety cycle: a second Eviscerate step; the committed fight still replays. Fury's 72-point strings: Improved Cleave
  3 → 2 in the optimizer inputs and `gen_db` (db.json moves at integration).

Core hunks (DK-only): `sim/core/runic_power.go`, `runic_power_helper.go` (grace, death order, miss). Shared tooling
hunks: `tools/simval/fight.go` (Vindication's id), `goldchar.go` (58670 mirror).

Goldens (Average-Default): Blood −3.6%, Frost −1.3%, FrostUH −0.6%, Unholy −0.2% (runes); BloodTank dps −1.8%, dtps
+0.5% (Blade Barrier); Combat −1.9% (Prey on the Weak); Subtlety −1.5% (no Rupture-tick combo points); Arms −1.7%,
Fury −0.1% (Rend); Enhancement +0.2% (Flurry); Protection paladin ±0.0% (Vindication's misses). `raid25.derived.json`
moves too (Devotion Aura improved).

### EC-NATURE-FIX (Z, done; changes goldens; stages: fixes, live)

- Druid: Eclipse (Solar and Lunar) on crits only (spell_proc -48516 HitMask 2); Insect Swarm's crit without Vengeance;
  Rake's first hit takes armor; Predatory Strikes ignores the weapon's enchant; cat and bear form
  weapon damage (the strikes row); the server's weapon feral AP truncation (goldchar row); the Balance APL's Nature's
  Swiftness on Starfire; Starfall's stars (53195, `SPELL_ATTR3_NOT_A_PROC`) proc Illustration of the Dragon Soul on
  the server, never in the sim; Force of Nature casts 65861, not the talent's 33831 (Omen of Clarity); normalized
  form attacks' creature-type AP at 2.4 speed (a lead).
- Shaman: Enhancing Totems on Flametongue Totem's spell power (the sim's is a fixed 144; the talents row); Lava
  Lash's off-hand percent (`EffectWeaponDmg` skips it for a non-physical spell); Elemental's Lightning
  Bolt 13–18% over the sim's (a lead).
- DruidFeralTank's gear set names 5 enchants by spell or item id, which the sim drops; the Feral APL fight cycle's
  Savage Roar gets a second step (once a cycle, it can miss a fight); Balance's 72-point strings in the optimizer's
  test inputs and `gen_db`.
- Live: Improved Healing Wave (a re-record of the ShamanRestoration spell-mod path); Glyph of Totem of Wrath's 84
  spell power.
- **Owns:** `sim/{druid,shaman}/` but pet, threat and Flurry code; `ui/balance_druid/apls/`; their drop entries and
  README rows.

#### As built

Stage fixes (offline, on `d253e096c`; notes and live plan in `tmp/ec-nature-fix-code.md`). Each mismatch re-measured
against [ac] `3d89793c7` and the committed fixtures:
- Eclipse procs on a Wrath or Starfire crit only (spell_proc -48516 HitMask 2), at the rank's chance squared (script
  and proc system each roll 33/66/100%, Wrath at 60% of it), never while either eclipse is up. Fixed.
- Vengeance reaches only its class mask (Wrath, Moonfire, Starfire, Starfall): Insect Swarm, Hurricane, Typhoon and
  Faerie Fire (Feral)'s 60089 crit at the plain 1.5. Fixed.
- Rake's first hit takes armor (`Unit::IsDamageReducedByArmor` exempts only the bleed tick). Fixed.
- Form weapons `min(level, 60)` × 0.85–1.25 × form speed (cat 51–75, bear 127.5–187.5); weapon feral AP
  `int32(dps × 14) − 767`, float32 `getDPS`; Predatory Strikes 7/14/20% of feral AP plus the weapon's template AP,
  never its enchant (Scourgebane's dynamic part gone). Fixed: strikes known mismatch, goldchar AP rows, mobs row.
- Balance: the Dreamstate test APL spends Nature's Swiftness on Wrath (17116's mask covers Wrath, not Starfire).
- Starfall's stars (53195, `NOT_A_PROC`) take `ProcMaskSpellDamage | ProcMaskNotInSpellbook`, as Arcane Missiles'
  missiles: they proc Illustration of the Dragon Soul; the splash stays suppressed. No record covers it.
- Force of Nature casts 33831 (serverdata hunk regenerated, 65861 gone; APLs and raid25.json's Balance rotation
  moved); Clearcasting's mask covers it, so it spends Omen of Clarity. No record covers it.
- Lava Lash: the whole off hand (`EffectWeaponDmg` skips TOTAL_PCT, the off hand's 50%, for a fire spell) and a melee
  crit (2.0 plus meta; Elemental Fury's mask misses it). Fixed.
- Enhancing Totems: Flametongue Totem's aura takes `int32(144 × (1 + 5% a point))`, 165 at 3, for the whole raid and
  its pets (58655 is `APPLY_AREA_AURA_RAID`; `improveFlametongueTotem`, via the aura's exclusive priority; no proto
  change; `TestEnhancingTotemsReachTheRaid`). Fixed.
- Improved Healing Wave: Healing Wave 3000 − 100 ms a point. `implemented_talents.json`, `talent_paths.json`
  (ShamanRestoration step 1 now rank 5, not a 4-point filler) and `spellmod_spells.json` (its hunk only) follow. Until
  the live rerun: `sweepRerecord` lists ShamanRestoration (`asRecorded` now also restores learned talent ranks), and
  its spell-mod drops say rerun pending.
- Feral tank gear sets: 3878, 3852, 3860, 3822, 3606 for the spell and item ids; `goldchar_setups.json` follows.
  `goldcharRerecord` (new) replays DruidFeralTank with its record's enchants until the rerun.
- DruidFeralApl's cycle gets a second Savage Roar after Mangle; raid25.json's Balance string drops Owlkin Frenzy (72 to
  71 points, Phase 2's preset).
- Leads not confirmed: no druid spell carries `NORMALIZED_WEAPON_DMG`, so the 2.4 fist speed never reaches a form
  attack; Elemental's Lightning Bolt agrees in every probe and its fight (hit mean +4%, inside the range).
- Drops gone: fights Vengeance ×2, Eclipse, Rake, Lava Lash ×2; goldchar AP ×2; sweep enhancingTotems; mobs
  NightElfDruid; strikes form weapons. All replays pass, the fights with 0 druid or shaman drops.
- Goldens (`dock.sh delta`, Average-Default): Balance −3.6% (Eclipse, Insect Swarm crits), BalancePhase3 −2.2%,
  Dreamstate −0.01% (NS on Wrath, a Starfire fewer), Feral +0.14% and FeralApl +0.33% (form weapon, Rake −4%),
  FeralTank DPS +1.7% and DTPS −6.4% (enchants), Enhancement +2.4% and FrozenPower +2.7% (Lava Lash ×2); 31 of 37
  unchanged.

Stage live (2026-10-06, server lock, [ac] `3d89793c7`, no rebuild; notes in `tmp/ec-nature-fix-live.md`):
- ShamanRestoration re-recorded: talent sweep (18 records, trimmed into `talents.jsonl`) and spell-mod sweep (94
  lines replace the path's 92 in `spellmods.jsonl`). Improved Healing Wave: Healing Wave −500 ms on both sides; Nature's
  Swiftness −2500. `sweepRerecord` empty; the three rerun-pending drops and their README rows gone.
- DruidFeralTank re-recorded with the five enchants: golden character (4 records in `goldchar.jsonl`;
  `goldcharRerecord` empty) and fight (70/70, log 961 DPS, sim 952; the old log 935).
- Glyph of Totem of Wrath: new `TestNatfixTotemOfWrathGlyph` (mod-sim-validation, run by name) casts the totem for real:
  +280 from 57663 and +84 from 63283 in every magic school, the sim's 84. No sim change.
- DruidFeralApl's fight not re-recorded: its log already lands 3 Savage Roars and replays green.

Review: Enhancing Totems first reached only the shaman's party; widened to the raid as above. No golden moves (no
suite pairs Flametongue Totem with Enhancing Totems; raid25's Totem of Wrath outranks it).

### EC-ARCANE-FIX (Z, done; changes goldens; stages: fixes, live)

- Mage: Torment the Weak only on a snared or slowed target; T8 4-piece's 20% since `e9517efd2` (64869, the sim's 10%);
  Brain Freeze's Fireball! (57761) lasts 15 s; Enduring Winter's 6 s proc cooldown; Master of Elements
  refunds 30%, on no-cost and clearcast crits too, with Burnout burned apart; Frost's crit over the sim's (a lead);
  MageFire's fire crits deal 45% over its log (21 to 16, Fireball's crit mean +16%), the fight 23% (Ignite pays its
  share); Blizzard's 42938 procs Illustration in the sim, not on the server.
- Hunter: Ranged Weapon Specialization on Explosive Trap and magic procs; Thrill of the Hunt on the base cost;
  Explosive Decoy's blast on its creature's death; Thrill of the Hunt's mana off a free Lock and Load shot; Hunter's
  Mark reaching a weapon hit only as flat AP after the weapon percent (a lead).
- Live: Frost Channeling's mask 84.
- **Owns:** `sim/{mage,hunter}/` but pet, threat and EC-CORE-FIX's code; their drop entries and README rows.

#### As built

Stage fixes (offline; notes and live plan in `tmp/ec-arcane-fix-code.md`), re-measured on `d253e096c`:
- Torment the Weak: fixed. The server adds it only against a snared or attack-slowed target
  (`Unit::SpellPctDamageModsDone`); raid bosses and the boss dummy are immune to snares, not to `MECHANIC_SLOW_ATTACK`,
  which every sim attack speed debuff carries. `Mage.tormentTheWeak` reads the target's `AtkSpdReduction` category, on
  Fireball, Frostfire Bolt, Pyroblast (dots at their snapshot), Arcane Blast, Missiles, Barrage, and Frostbolt
  (family flag 0x20, which the sim lacked).
- T8 4-piece: fixed, 20% (`T84PcProcChance`), keeping Missile Barrage, Hot Streak and Fireball! (`spell_mage.cpp`).
- Brain Freeze: fixed. Fireball! lasts 15 s and is used up at the cast (spell_proc 57761's cast phase), not the landing.
  T10 2-piece only on that use, never on the timeout (`Aura::HandleAuraSpecificMods`' 57761 case).
- Enduring Winter: fixed. A landed Frostbolt at 33/66/100%, at most every 6 s (spell_proc -44557).
- Master of Elements: fixed. 10% a point of the truncated base cost on any fire or frost mage spell crit, whatever the
  cast paid; Living Bomb's explosion reads the bomb's cost, Deep Freeze (71757's damage) none. Burnout burns 1% a
  point of the crit spell's cost apart, as unlogged spent mana (44472). Each at most once per 8 ms (spell_proc -29074
  and -44449), so a spell critting several targets at once refunds and burns once.
- Blizzard's tick (42938): fixed, `ProcMaskProc`, so only `SPELL_ATTR3_CAN_PROC_FROM_PROCS` procs take it (no
  Illustration of the Dragon Soul).
- MageFire's fire crits: Fireball's hit mean was 12% over the log and its crit mean 16%: Torment the Weak (3 points).
  After the fix the hit mean matches (5434 to 5429) and the crit mean is +4% on 5 logged crits, inside the range; the
  crit multiplier matches the server's 1 + 0.545 × 1.5. The rest is crit count, 20.1 expected to 15 logged (|z| about
  1.5) on source-confirmed chances, and Ignite pays 40% of those crits. Fight DPS: +22.7% to +14%.
- Frost's crit: not confirmed. The MageFrost fight's extra Ice Lance and Fire Blast crits follow 6 Fingers of Frost
  procs in 20 Frostbolts (spell_proc 44545: 15% at the cast; the sim expects 3, inside its range).
- Hunter, Thrill of the Hunt: fixed. 40% of the shot's truncated base cost, a free Lock and Load shot too, a third on
  an Explosive Shot tick (`spell_hun_thrill_of_the_hunt`), only on spell_proc -34497's shots (no Multi-Shot), at
  33/66/100%.
- Ranged Weapon Specialization: fixed. With a bow, gun or crossbow it covers every non-physical school (Explosive
  Trap, Black Arrow, Volley, magic procs) and, of physical, only ranged attacks (`Unit::UpdateDamagePctDoneMods`).
- Hunter's Mark: confirmed and fixed. A weapon hit's weapon part reads the hunter's own ranged attack power; the mark
  adds `int32(bonus / 14 × speed)` after the weapon percent (`Unit::MeleeDamageBonusDone`, 2.8 s normalized): Auto
  Shot, Aimed, Multi, Chimera, Kill, Silencing Shot, Wild Quiver, Zod's Quick Shot (now 50% of the weapon part).
  Kill Shot's own 0.4 AP reads no mark (`Spell::EffectWeaponDmg`: `GetTotalAttackPowerValue` without a victim).
- Explosive Decoy: fixed (`sim/core/consumes.go`, its entry). Creature 29134 casts 53273 only from its SmartAI's death
  event, and a boss fighting the raid never attacks it, so the sim's decoy deals nothing and only takes the
  explosives' shared cooldown.
- Drops gone with their fights README rows and causes: MageFire and MageFrostFire `gain sizes` (Master of Elements),
  MageFrost 44549 and 57669 `applications`, MageFrostFire 47610 crit and tick hit means. The tick's 33 against the
  sim's 33.99 is the server truncating a dot's amount, inside the check's floor. Kept, sheet artifacts: goldchar
  MageFire and MageFrostFire arcane crit, talents MageFire improvedScorch and livingBomb.
- Goldens (`dock.sh delta`, 7 suites move): Frost Average-Default +7.7% (Frostbolt +11.8% a hit under Thunder Clap,
  fewer Frostfire Bolts); Fire +0.07%, FrostFire 0%, Arcane 0% (Thunder Clap keeps Torment the Weak); NoBuffs rows -3
  to -10%; Kirin Tor Garb rows +0.4 to +8.9%; MM +0.12% (Explosive Trap +5%, Silencing Shot +5.8%, Chimera -1.2%, Kill
  Shot down), multi-target up to +2.7%; BM -0.08%, SV -0.13% (Kill Shot).

Stage live (2026-10-06, server 3d89793c7, mod-sim-validation f72dbf7):
- Frost Channeling's mask 84: confirmed, no change. The talent sweep's records carry no cost, so
  `TestSimvalSpellModSweep` ran on MageFrost with a scratch list naming only 12519 and nine fire, arcane and frost
  spells: each cost drops 10%, truncated (Fire Blast 686 to 617, Arcane Blast 228 to 205), and all nine agree with
  the sim's exact cost. Nothing committed: `spellmod_spells.json` (tools/acore/spellmods) lists SPELLMOD talents only.
- No fixture re-recorded; no goldens moved.

### EC-THREAT-FIX3 (Z2, done; changes goldens, full suite; C++; stages: threat, C++ and e2e, live)

- Sunder Armor: the server's warriors know 7386 (`character_spell`), whose `spell_threat` row gives 345 + 5% AP, plus
  58567's level 10 and its `SPELL_EFFECT_THREAT`; the sim and the e2e's ProtWarrior probe use 47467 (its level 77).
- Energize threat: `Unit::EnergizeBySpell` and the periodic energize tick hand half the gain, unmodified, to the
  enemies as assist threat (rage and runic power in tenths): Horn of Winter, Divine Plea, Anger Management, Unbridled
  Wrath, Shield Specialization, mana returns. The sim makes none.
- Heals through `GainHealth` make no heal threat in the sim: Death Strike, Rune Tap, Death Pact, Frenzied Regeneration,
  Blood Presence (Improved Leader of the Pack's 34299 is `ATTR1_NO_THREAT`). Penance's 53007 ticks make none (a lead).
- Probe: `.simval threat` records `SPELL_EFFECT_THREAT` and heal threat (C++); `threatCharacters` gains a hunter bot
  (Volley, Auto Shot), Heart Strike's split (55262, 2 targets), 58567, Bladestorm's 50622 and 44949, and Sunder 7386;
  the self buffs the replay can't cast (Shield Wall, Recklessness, Berserker Rage, Tiger's Fury, Power Infusion,
  Demonic Empowerment) get a replay path or a drop with its reason.
- Live, after the user's rebuild: `TestSimvalThreat` re-records `threat.jsonl`.
- **Owns:** `sim/core/threat.go` and the energize and `GainHealth` threat hooks in core; the threat lines of its
  mismatches in class code; `tools/simval/threat*.go`; `threat.jsonl`; the module's `.simval threat` C++ and
  `e2e/threat_test.go`.
- **Goldens:** TPS in nearly every suite (energize threat); report each.

#### As built

Threat stage (offline; each lead re-measured against [ac] 3d89793c7, the spelldump and the live DBCs):
- Gain threat (`sim/core/threat.go`, hooked into the `Add*` power functions and `GainHealth`): a `ResourceMetrics`
  can carry the server's threat for its gains, added at each gain (none before the pull) and put under the gain's
  action, shared evenly over the enemies, at iteration end. Feint's floor counts it.
  - `EnergizeThreat(kind)`: half, unmodified, rage and runic power in tenths. `EnergizeBySpell` takes the amount
    asked for (the cap's cut too), `EnergizeByTick` (periodic energize, `OBS_MOD_POWER`) the gain, both only for
    the unit's current power; `EnergizeByPotion` (`SPELLFAMILY_POTION`) for any power it has.
    `AddEnergizeThreat` books an energize the sim gains inside another gain: Chill of the Grave and Dirge, per
    landed strike in their class masks.
  - `HealThreat(metrics, spellID)`: half the health gained (a quarter for paladins), plus the cast's bonus when
    each gain is a cast, modified like a hit of that spell.
  - The upstream mana (×0.5) and rage (×5) gain threat and its `OtherActionManaGain`/`RageGain` holders went: it
    took every non-regen gain, actual amount only, whole on each target.
- Each energize site is marked after checking its server spell for `ATTR1_NO_THREAT` and its caster. None for
  Anger Management and Butchery's tick (`MOD_POWER_REGEN`), Horn of Winter, Empower Rune Weapon and Blood Tap (rune
  cost runic power), Sudden Death (`SetPower`), refunds and cost mods, `NO_THREAT` energizes (Life Tap, Soul Leech,
  Furor, Spiritual Attunement, Viper, Judgement of Wisdom, Sanctuary, Go for the Throat), other units' energizes.
  Heals: Death Strike 45470, (Improved) Blood Presence 50475 (+40 each), Rune Tap 48982, healing potions,
  healthstone, Lifeblood's ticks, Blood Reserve.
- Sunder Armor reads 7386 (`ThreatSpellID`: 345 + 5% AP) and adds 58567's 10 on landing; its ActionID stays 47467.
- Penance's ticks are casts of 52985 and 53000 (level 80): `ThreatSpellID`, `NoThreatBonus` and a `ThreatTick` per
  tick. `ThreatTick` takes positive tick spells, as assist threat.
- serverdata regenerated: 7386's row and nine spells (34917 dropped).
- Replay: the 47467 Sunder record's bonus is a known mismatch (a probe artifact, fixture README row);
  `threatSimSpells` maps 7386 to the sim's Sunder.

Verdicts: Sunder, energize threat, the heals and Penance confirmed and fixed, except: Anger Management and Horn of
Winter aren't energizes; Death Pact (48743) is `ATTR1_NO_THREAT`; Frenzied Regeneration's 22845 and Lifeward's
44578 heal but aren't in the spelldump, so the live stage captures them.

Goldens (`dock.sh delta`): TPS moves in all 37 suites, nothing else; the OtherID 10/11 holder abilities are gone.
Average-Default: Disc +192% (Penance), Holy paladin 0.22 → 8.6 (energizes at full mana count), Blood Tank +0.14%,
Frost DK +0.09%, FrostUH and Unholy +0.07%, rogues +0.04 to +0.06%, ferals up to +0.03%, Blood +0.001%; Fury
−0.007%, Arms −0.03%, casters and healers −0.08 to −0.93%, Enhancement −1.0%, hunters −0.5 to −1.4% (Viper,
Judgement of Wisdom), warlocks −0.7 to −1.1% (Life Tap), Ret −0.35%, Prot paladin −0.55%; Prot warrior, Resto druid
and Resto shaman unchanged. LongMultiTarget settings fall most, to −31%: the old threat was whole on each target.

C++ and e2e stage (`-Werror -fsyntax-only` clean, e2e vetted; not rebuilt, no live run; notes and live plan in
`tmp/ec-threat-fix3-code.md`):
- `.simval threat` records `threatEffect` (each `SPELL_EFFECT_THREAT` effect's value, its sum on the dummy) and,
  for a healing spell, `heal` (threat for `amount` healed: half, a quarter for a paladin, modified); live adds
  both, heal threat through `ForwardThreatForAssistingMe` (`healAssist`). An ally-only spell (Power Infusion) is
  probed cast on the caster, a channel's tick spell (Volley's 58433) as if channeled at the dummy. Command syntax
  and `simval_commands.sql` unchanged.
- `TestSimvalThreat`: Sunder Armor probed as 7386 (was 47467), plus 58567, Bladestorm's 50622 and 44949, Heart
  Strike on 2 targets, heals 45470, 48982 and 50475, Demonic Empowerment, a Night Elf hunter (Auto Shot 75, Volley
  58433); `threatAssistProbes` gain Shield Wall, Recklessness, Berserker Rage, Tiger's Fury, Power Infusion. Each
  record is held to its `threatEffect` and `heal` arithmetic and their live shares.
- Replay (`tools/simval/threat.go`): `effect threat` and `heal threat` checks when the record has the fields (the
  bonus no longer leaves the effect out); a heal with no sim spell but a `Unit.HealThreat` (45470, 50475) replays
  without a cast; a mapped probe measures only the spells reading its server spell (`Spell.ThreatServerSpellID`,
  a hit's or tick's `FromSpellID`): 7386 without 58567's 10, 58567 alone, each Bladestorm Whirlwind; a tick probe
  casts the spell carrying the dot; `threatSimActions` maps Auto Shot to the sim's Shoot. Core gained the read-only
  `Spell.ThreatServerSpellID`, `Spell.ThreatFromHealing` and `Unit.HealThreatOf`.
- Self buffs: Shield Wall (a shield), Recklessness (`UseRecklessness`), Berserker Rage and Tiger's Fury (an empty
  rotation for the gain to ask), Power Infusion (the talent, `PowerInfusionTarget` self) replay; Demonic
  Empowerment is dropped: cast on the pet, its bonus goes to the creatures fighting the pet, and the probe's
  warlock has none. Every new probe is a `recapture` drop until the live run; the committed 147 records still
  replay all-PASS (345 checks).

Goldens: this stage adds no golden move; `dock.sh delta` is stage 1's.

Live stage (server on [ac] 3d89793c7 plus the module tree, 2026-10-06):
- Spelldump: 22845 (Frenzied Regeneration's heal: physical, level 0, not `NO_THREAT`) captured; 44578 (Lifeward) was
  already in. gen_serverdata regenerated; Frenzied Regeneration and Lifeward take `HealThreat` (22845, 44578). No
  golden moves: no suite casts either.
- `TestSimvalThreat` re-recorded `threat.jsonl` (161 records; Envenom's last of 7 kept; 45524, 49576 and 47193 are
  dropped probes, so their records stay out). Every `recapture` drop and the 47467 known mismatch with their README
  rows went; `knownThreatMismatches` is empty. The whole run passed except ProtPaladin's cleanup (a character
  delete that lagged: guid 540381 was gone by the next SELECT), its records all written.
- Every new check passes against the server: Sunder 7386 (373.40 = 345 + 5% AP) and 58567 (10, effect threat 0);
  Faerie Fire (Feral) 60089's 638 `effect threat`; heal threat of 45470, 48982 and 50475 (half the heal times Frost
  Presence, bonus included, 0 for Rune Tap); Volley's 58433 ticks (80), Auto Shot (1), each Bladestorm Whirlwind
  (36), Heart Strike on 2 targets (40 each); Shield Wall, Recklessness, Berserker Rage, Tiger's Fury and Power
  Infusion bonuses and per-creature shares (Power Infusion selected the dummy: the `CheckExplicitTarget` fallback
  took); Mind Flay 58381 selects the dummy, bonus 0. Evocation 12051 and Devouring Plague 48300 carry a `heal`,
  "not compared" (the sim doesn't heal with them).
- Replay fix: `checkHealThreat` restarted a sim a cast had left running, which panicked on Evocation's channel;
  it now starts only a heal-only probe.
- Still unsettled by a record: energize threat (the probe has no energize field), and 22845 and 44578's heal
  threat (no probe casts them).

Goldens: the live stage adds no golden move; `dock.sh delta` is stage 1's.

### EC-CORE-FIX2 (Z2, done; changes goldens, full suite)

- Damage truncation: the server truncates `SpellDamageBonusDone`'s result, a dot's amount at application and a
  direct hit (Frostfire Bolt's tick: 33 on the server, 33.99 in the sim); re-measure which paths truncate
  (`MeleeDamageBonusDone` too) and where in the order.
- Hunter's Mark and creature-type AP: the server adds them as one `int32(bonus / 14 × speed)`, outside the physical
  percent mods (`MeleeDamageBonusDone` skips aura 79 for school-normal damage); the sim truncates each apart and
  scales both (The Beast Within, Ranged Weapon Specialization). Kill Shot's weapon part (effect 17) isn't normalized:
  RAP / 14 × the weapon's own speed, not 2.8.
- Rune of the Stoneskin Gargoyle (enchant 3847): its +25 defense is skill, not rating, so dodge, parry and miss take it
  undiminished (`Player::UpdateDodgePercentage`), and so do the attack table, crit and block as the defender's skill:
  a core defense-skill bonus; DeathknightBloodTank's dodge and parry goldchar drops go.
- Tooling: `docs/azerothcore-parity/audit/spells_audit.csv` regenerated (`tools/acore/spellaudit`; it still lists
  65861); `tools/simval/check.go`'s `rangedWeaponSpell` comment (serverdata has Auto Shot).
- **Owns:** `sim/core/` but `threat.go`, EC-THREAT-FIX3's hooks and PERF-Z's hot paths; the hunter's weapon damage
  (`huntersMarkBonus`, Kill Shot); `sim/deathknight/items.go`'s 3847; the audit CSV; their drops and rows.
- **Goldens:** every suite whose damage truncates; report each.

#### As built

Re-measured on `0ccf8aa60` against [ac] `3d89793c7` and the committed records. Verdicts:
- Damage truncation (fixed, core): every step after the base is whole on the server. `SpellDamageBonusDone`,
  `MeleeDamageBonusDone` (done, a dot's amount at application), `SpellDamageBonusTaken`, `MeleeDamageBonusTaken`
  truncate; `CalcArmorReducedDamage` rounds up; a partial resist takes `damage × i / 10` in integers; crit, glancing
  and absorbs stay whole. The sim truncates after the attacker multiplier (`doneDamage`) and the target's, rounds up
  after armor or resist, and truncates after the outcome and post-outcome mods
  (`ApplyPostOutcomeDamageModifiers`). MageFrostFire's
  47610 tick: sim 33.99 → 33, the log's 33. Left: each SP/AP bonus's own `int32` (class code computes it inline), a
  stacked dot's per-stack truncation, magic crit before resist (≤1 point), heals (`SpellHealingBonusDone` truncates
  too).
- Hunter's Mark and creature-type AP (fixed, core): `weaponAttackPowerBonus` sums the mark (ranged hits) and
  `MobTypeAttackPower`, then one `int32(sum / 14 × speed)`; a physical hit's bonus skips the aura 79 share
  (`weaponBonusMultiplier`: `DamageDealtMultiplier`, the physical school's, `Spell.TotalPctMultiplier`, which
  Ranged Weapon Specialization sets on ranged physical spells). `huntersMarkBonus` is gone. No record covers it
  (the fights carry no mark); source-confirmed. DONE_VERSUS (aura 168) scales the bonus too: Troll Beast Slaying
  (20557) moved from `DamageDealtMultiplier` to the per-target attack table multiplier (golden-neutral, the goldens
  fight a demon); the undead sets (57943, 29068) still sit in `DamageDealtMultiplier`. Cross-review: the off hand's
  `MOD_OFFHAND_DAMAGE_PCT` (Dual Wield Specialization, Nerves of Cold Steel) is `TotalPctMultiplier` too
  (`SpellConfig.TotalPctMultiplier`), and Crusade's aura 79 half went from the attack tables to
  `DamageDealtMultiplier` (the Thermal Sapper's hit on the paladin takes it now).
- Kill Shot (fixed): effect 17's weapon part reads the weapon's own speed (`RangedWeaponDamage`). No record (no
  fight reaches 20%).
- Rune of the Stoneskin Gargoyle (fixed): 62157's +25 is `PseudoStats.BonusDefenseSkill`; dodge, parry and miss
  take it undiminished, `Unit.DefenseSkill()` gives the table, crit taken and block the full skill. Goldchar
  BloodTank dodge 20.4424 → 20.6844 (server 20.6844), parry 18.9400 → 19.4142 (19.4128); both drops and the README
  row gone. Cross-review: Icebound Fortitude and Ardent Defender read the full skill too.
- Tooling: `spells_audit.csv` regenerated (33831, not 65861; Auto Shot listed); `rangedWeaponSpell`'s comment no
  longer names Auto Shot. `TestPetStrikeReplay` (`tools/simval/damage_test.go`) expects the truncated amount.
- Replays all pass (`simval.jsonl` 1114/1114).
- Goldens (`dock.sh delta`), 35 of 37 move (not the priest healers), Average-Default: BM −0.20%, MM −0.21%
  (Auto Shot −0.7%/−0.6%, Aimed −0.6%, Silencing −1.2%: the mark without TBW, RWS, raid aura 79s; Kill Shot
  +1.1%), SV −0.04%; BloodTank dps +0.1%, dtps −2.9% (Stoneskin); every other suite −0.01 to −0.1% (truncation:
  dot ticks, Razorice −1.2%, Thorns −0.8%); the other healers move only the Gnomish Lightning Generator row. Twin Blades of Azzinoth rows −0.14% (Combat) to
  −0.99% (Fury) for melee (its +200 AP on the demon dummy loses the aura 79s).

### EC-CLASS-FIX (Z2, done; changes goldens; stages: melee, casters, live)

- Death knight: death runes from Blood of the North, Reaping and Death Rune Mastery revert 30 s after the talent last
  procced (`spell_dk_death_rune`'s periodic); Improved Unholy Presence's rune regen scales the cooldown after the grace
  cut (`Player::GetRuneBaseCooldown`); Death Coil and Rune Strike, not refundable, take irand(0, cost / 4) on a miss; the
  rune weapon takes the DK's creature-type AP at half (`MeleeDamageBonusDone` for 27893, `dancing_rune_weapon.go`);
  DeathknightFrost's cycle gives Howling Blast runes, so the Rime drop goes once it's re-recorded.
- Paladin: Improved Devotion Aura at 1–2 points (+17% or +34% armor), which the raid buff's tristate can't carry: a
  per-raider hook (as `improveBloodPact`).
- Rogue: Honor Among Thieves below 3/3 rolls once a cast, at the finish phase, not per hit.
- Druid: Omen of Clarity (16864: `CAN_PROC_FROM_PROCS` and a CheckProc script) may proc from triggered damage such as
  Starfall's stars and splash (a lead); Starfall's 53201 is a self aura that can't miss, the sim rolls spell hit.
- Mage: T10's 2-piece only when Hot Streak or Missile Barrage is used up, never on its timeout (`Aura::
  HandleAuraSpecificMods`, `spell_mage_gen_extra_effects`); Fingers of Frost rolls at the cast (spell_proc 44545), and
  its Shatter crit reaches only spells in Shatter's class mask its ignore-aura-state aura covers, not item procs;
  Torment the Weak against a non-boss add a chill slows; the Frost crit lead (6 Fingers of Frost procs in 20 Frostbolts
  logged, 3 expected): a recapture says whether it repeats; school cost auras (72, 73: Frost Channeling) get a
  permanent replay, as the spell-mod sweep covers SPELLMOD talents only.
- Live: DeathknightFrost's, RogueSubtlety's (two Eviscerate steps) and MageFrost's fights re-recorded; the Omen lead;
  the cost auras' records; 58670 (Prey on the Weak) captured into the spelldump.
- **Owns:** `sim/{deathknight,paladin,rogue,druid,mage}/` but EC-THREAT-FIX3's threat lines and 3847; the DK-only
  hunks of `sim/core/runic_power*.go`; those three setups' cycles and fights; the cost-aura replay's files; their
  drops and rows.

#### As built

Melee stage, on `0ccf8aa60`. Verdicts:
- Death rune talents (fixed): every `spell_dk_death_rune` proc restarts the talent's 30 s periodic, which then reverts
  all runes that talent turned, ready or not (`procDeathRuneTalent`, `RuneMeta.talent`); a proc turning none still
  restarts it. Blood of the North and Reaping share one revert, as the sim rolls them as one. Proc chances are
  spell_proc's (30/60/100%, 33/66/100%), not points / 3.
- Improved Unholy Presence (fixed): 63622's regen percent scales the cooldown after the grace cut, in whole ms
  (`runeCooldown`; `SetRuneCd` became `SetRuneRegenPercent`).
- Death Coil and Rune Strike (fixed): refundable, so a miss takes irand(0, cost / 4); Sudden Doom's triggered Death
  Coil pays nothing.
- Rune weapon creature-type AP (fixed): its hits add the DK's int32(AP / 14 × the DK's main-hand speed, normalized for
  effect 121), float32 like the server, halved with the hit; white swings through its `MHConfig`. Cross-review: it skips
  the DK's aura 79s the rune weapon's multipliers hold, and the rune weapon copies the DK's per-target multipliers
  (Troll Beast Slaying).
- DeathknightFrost cycle (fixed): IT, PS, BS, FS, Ob, BS, FS, HB, HoW at 2000 ms; a model of `Spell::TakeRunePower`
  lands every rune spell from 1700 to 2200 ms, and the sim's bot lands Howling Blast 4 times a fight.
- Improved Devotion Aura at 1–2 points (fixed): `improveDevotionAura` gives every raider and pet under the best paladin's
  aura +204 / +409 armor while the raid buff is Regular; RaidStats stays unimproved, as with Blood Pact.
- Honor Among Thieves (fixed): one roll a cast at the finish phase; Fan of Knives rolls once per hand (`dealCastHits`).
  Mutilate's hands, poisons and Killing Spree are triggered casts of their own, which 51698's `CAN_PROC_FROM_PROCS` lets
  roll each.

Core hunks (DK-only): `sim/core/runic_power.go`, `runic_power_helper.go`. Tests: `sim/deathknight/dps/runes_test.go`,
`sim/paladin/retribution/devotion_aura_test.go`, `sim/rogue/honor_among_thieves_test.go`.

Goldens: Death Coil and Rune Strike misses now cost less runic power, so more casts: Blood −0.2 to +0.4% (AllItems, NoBuffs
only), Unholy −0.2 to +0.4% (same), BloodTank −0.4 to +1.3% (Average-Default +0.4%: Death Coil 12.6 → 14.0 hits). No
golden has the revert, Improved Unholy Presence or partial ranks; creature-type AP on a DK only in the AllItems Twin
Blades of Azzinoth rows; Frost, rogue and paladin suites don't move.

Follow-ups: Blood Tap's rune pick and its "convert the other blood rune" interplay with the talents; Improved Devotion
Aura's 63514 healing taken; a Threat of Thassarian off-hand strike rerolling partial Death Rune Mastery; the DK's own
strikes add creature-type AP after Rage of Rivendare and Tundra Stalker.

Casters stage, same base (notes in `tmp/ec-class-fix-code.md`). Verdicts:
- Starfall (fixed): 53201 is an aura on the druid, never misses.
- Omen of Clarity lead (not fixed, refuted live): 16864's `CAN_PROC_FROM_PROCS` suggested each star (53195) and splash hit
  (53190) rolls it, but 14 live Starfalls gave no Clearcasting from them (about 13 expected) and 2 from the casts, the
  8.75% the sim already rolled at the cast. Stars and splash roll nothing; the live stage reverted the flags.
- T10 2-piece (fixed): only when Hot Streak or Missile Barrage is used up (`pushingTheLimit`). Missile Barrage goes at
  Arcane Missiles' finish phase, so T10 at the cast and again at the channel's end (`spell_mage_arcane_missiles`); one
  timing out mid-channel gives it at the end, otherwise never.
- Fingers of Frost (fixed): spell_proc 44545 rolls at the cast, hit or miss; Improved Blizzard's script rolls its own on
  each landed tick (was dead in the sim). Only spells in spell_proc 74396's mask take charges
  (`spell_mage_fingers_of_frost`): one a cast, none a channel's cast, one a landed hit of a triggered spell (Blizzard's
  ticks, Arcane Missiles' missiles, Living Bomb's explosion), each before any proc it triggers, so a cast that procs
  keeps both. Shatter's crit goes to the spells in both Shatter's and 44544's masks (`Unit::SpellTakenCritChance`), not
  the mage's crit: not item procs, not Blizzard.
- Torment the Weak (fixed): a landed Frostbolt (not under Glyph of Frostbolt, which zeroes its slow) or Frostfire Bolt
  (9 s) or Chilled (1.5 s), plus Permafrost, slows a target that isn't a world boss; raid bosses and every simval dummy
  are snare-immune.
- Frost crit lead (refuted): 13 Fingers of Frost procs in 80 Frostbolts over four logs (12 expected at 15%), all at a
  cast; the committed log's 6 in 20 was chance.
- Cost auras (built, recorded in the live stage): `tools/simval/costauras.go` writes `costaura_spells.json` (Arcane Focus,
  Precision, Frost Channeling, Power Infusion); `TestCostAuraRecords` replays `costauras.jsonl` as the spell-mod replay
  does; the module's `TestSimvalCostAuraSweep` records it.

Tests: `sim/mage/procs_test.go`, `sim/druid/balance/starfall_test.go`, `tools/simval/costauras_test.go`.

Follow-ups: Frostbite's and Mirror Image's chills for Torment the Weak; Brain Freeze from Blizzard; `spellmod_spells.json`
is stale on the base.

Live stage, against the server build of 2026-10-06 19:37 (`3d89793c7` plus the module's working tree); notes in
`tmp/ec-class-fix-live.md`:
- Fights re-recorded (19:52 to 20:06, `TestSimvalFight`): DeathknightFrost, RogueSubtlety, MageFrost (three runs, the
  last one committed). The replay passes on them and Howling Blast went off 5 of 5 (runes left it), so the Rime drop and
  its README row are gone; the fights README's date line is updated.
- Omen lead: refuted, see above (a 300 s and a 900 s fight of Starfall alone, `tmp/omen-cycles*.json`, not committed).
  `starfall.go` keeps only the never-misses fix; its test checks the cast's 8.75%.
- Cost auras recorded: 146 lines, 63 spell steps compared. Two were sim gaps, fixed: Precision counted twice on
  Frostfire Bolt (the server takes it once; the old comment was retail's), and Arcane Focus's hit reached the Arcane
  Missiles channel cast (its class mask covers the missiles, 42845, not 42846). Two Penance rows are dropped with README
  rows: the sim's Mental Agility cut (the server's mask misses Penance) and a channel the server's dummy 53007 lacks.
- 58670 (Prey on the Weak) captured into the spelldump, `gen_serverdata` regenerated.
- Goldens: Frost Average-Default +0.39% (Frostfire Bolt misses once Precision counts once; Fingers of Frost uptime
  48 → 77 s), rows −0.7 to +0.8%; Arcane Average-Default −0.35% (the channel cast misses more), rows −0.9 to +0.1%;
  Balance Average-Default 0, rows −3.0 to +3.2% (Starfall's hit roll); BalancePhase3 +0.03%, rows −1.6 to +2.6%; Blood,
  Unholy and BloodTank as the melee stage.
- Follow-up: priest Mental Agility on Penance (`sim/priest/penance.go`; mask `0x173` in index 1, Penance is
  `0x800000`).

### EC-CORE-FIX3 (Z5; changes goldens, full suite; stages: code, live)

- Heals: `SpellHealingBonusDone` and `SpellHealingBonusTaken` return `uint32` on the server, as damage does; the sim
  doesn't truncate heals (heal threat follows the amount).
- Damage left from EC-CORE-FIX2: each SP or AP bonus's own `int32` (class code computes it inline), a stacked dot's
  per-stack amount (Deadly Poison, Lacerate), magic crit before resist.
- Creature-type bonuses: the undead sets' DONE_VERSUS (57943, 29068; `sim/common/wotlk/item_sets.go`) leave
  `DamageDealtMultiplier` for the per-target attack table multiplier, as Troll Beast Slaying did; Rage of Rivendare
  and Tundra Stalker are DoneTotalMod, so they scale the DK's flat creature-type bonus; Improved Tracking
  (52783–52788) is a spell mod (aura 107), not an attack-table multiplier: check what it scales on a weapon hit.
- Casting: `makeCastFunc` (`sim/core/cast.go`) starts the cooldown and shared cooldown before the GCD and hardcast
  checks, so a direct `Cast` failing them still starts the cooldown.
- Meta gems: an item swap doesn't re-check meta activation; goldchar could check it (`HasMetaGemEquipped`).
- UI and tooling: the sheet's defense skill (`ui/core/components/character_stats.tsx`) leaves out
  `BonusDefenseSkill`; `spellmod_spells.json` stale; `procs_audit.csv` and `spells_missing.csv` regenerated after the
  fixes (`tools/acore/spellaudit`); a fight's mean check ranges to one iteration's tail (a 1.4× Shadow Bolt passed).
- Live: a hunter fight with Hunter's Mark up into the execute range, so a record covers the mark and Kill Shot;
  13376, 13377 and 48332 captured into the spelldump.
- **Owns:** `sim/core/` but `threat.go`, EC-PET-FIX2's pet paths and EC-THREAT-FIX4's hooks; the undead sets; the
  DK's Rage of Rivendare and Tundra Stalker hunks; the hunter's Improved Tracking; `character_stats.tsx`; the audit
  CSVs, `spellmod_spells.json` and the fight check; the hunter fight's cycle and log; their drops and rows.
- **Goldens:** every suite whose damage or heals truncate; report each.

#### As built

Code stage, on `a1bb8d403` against [ac] `3d89793c7` (notes in `tmp/ec-core-fix3-code.md`). Verdicts:
- Heals (fixed): `calcHealingInternal` is whole after the caster multiplier, the target's and the crit. Left: direct
  `GainHealth` heals (potions, DK and druid self heals).
- SP and AP bonuses (fixed): `core.PowerBonus`, int32 of the float32 product, around each inline `coefficient × power`
  (180 lines), script bonuses included (Rune Strike, Kill Shot, Rip, Ferocious Bite, Power Word: Shield).
- Base roll (fixed, found on re-measure): `SpellEffect.Roll` is a whole step, as `CalcValue`'s irand.
- Stacked dots (fixed): `Dot.SnapshotStacks`; one stack's done damage truncates, then multiplies (Deadly Poison,
  Lacerate, Holy Vengeance).
- Magic crit before resist (fixed): the resist is rolled where it was and comes off after the outcome.
- Measured on the damage, goldchar and strikes replays: 114 of 296 non-strike amounts were 1 to 3 over the server,
  none now; `amountTolerance` is 0.1% or 1 point either way. 56 of 148 weapon strike amounts stay 1 to 3 over
  (`Spell::EffectWeaponDmg`'s own int32 steps).
- Undead sets (fixed): per-target attack table multiplier.
- Rage of Rivendare, Tundra Stalker (fixed): the DK's seven weapon strikes add `Spell.WeaponBonusForDonePct`, with
  Glacier Rot and Merciless Combat on Frost Strike and Obliterate (DoneTotalMod too).
- Improved Tracking (no change): a flat SPELLMOD_EFFECT2 on the tracking spells' aura 168 (type mask 127), so
  DONE_VERSUS, where the sim has it.
- Casting (fixed): cooldowns start after the GCD and hardcast checks.
- Meta gems (partly): an item swap adds or takes the meta's stats with its condition (`metaGemStatChange`); its
  effect and crit damage stay the starting gear's. Goldchar's `meta gem` check reads `HasMetaGemEquipped`.
- UI: the sheet's defense skill and block chance count enchant 3847's 25 skill, read off the weapons.
- Tooling: audit CSVs regenerated. `spellmod_spells.json` not: the new list names spells `spellmods.jsonl` never
  probed (8 paths), so it needs the sweep re-recorded. The fight's means go through `checkMean` (4 deviations of a
  fight's mean; a 1.4× Shadow Bolt fails).
- Live stage tooling: cycle fields `dummyHealthPct` and `pull`, `Simulation.StartTargetAtHealth`, HunterMM's cycle
  (Hunter's Mark pulled, dummy at 20.5%, Kill Shot), the module's `TestSimvalEcCore3LowHealthFight`.

Tests: `sim/core/truncation_test.go`, `TestFightMeanCheck`. Replays all pass (`simval.jsonl` 1114/1114).

Goldens (`dock.sh delta`), all 37 move, Average-Default: −0.00 to −0.15% (Protection paladin −0.15%, Retribution
−0.09%, the priest healers' hps −0.03 and −0.05%), but Enhancement +0.27% (the Fire Elemental swings 35.9 → 38.1
times; Elemental's fire elemental rows −1.7 to +2.0%), which is the cast fix (Review). Frost DKs' Twin Blades of
Azzinoth rows +0.19 to +0.28% (Tundra Stalker on the creature-type AP).

Follow-ups: weapon strikes' int32 steps; a meta gem's effect across an item swap; Heart Strike takes no Rage of
Rivendare or Tundra Stalker though their class mask covers 55262; `spell_dru_rip` adds Rip's AP share and idol
after `SpellDamageBonusDone`, so they'd take no done percents; class code's direct heals; the spell-mod sweep's
re-record.

Live stage, 2026-10-10, against [ac] `3d89793c7` with module `408f7b5` (notes in `tmp/ec-core-fix3-live.md`):
- Spelldump: 13376, 13377, 48332 and 48333, which 48332 triggers; `gen_serverdata` and the audit CSVs rerun (0 not
  captured). `TestSimAOESpells` takes Fire Shield 13376 as the pet's, uncapped. The sim deals it with no hit roll,
  and the data has no can't-miss attribute: EC-PET-FIX2's file.
- Hunter fight re-recorded (`HunterMM.log.gz`): Hunter's Mark pulled, the dummy at 19%, Kill Shot 4 times. The
  dummy's AI zeroes every hit, `.damage`'s too, so the e2e sets its health with `.simval hp` and it stays there all
  fight; `dummyHealthPct` is that percent. 57 checks pass, none dropped: the `HunterMM 49001 ticks` drop and its
  README row went. Without the mark's aura line five means fail (Auto Shot z 9.7).
- Chimera Shot - Serpent (fixed, found on the record at z −3.57): 53353 ignores caster modifiers
  (`SPELL_ATTR3_IGNORE_CASTER_MODIFIERS`), so no Mortal Shots on its crits; Marked for Death's class mask leaves it
  out; `spell_hun_chimera_shot` reads the sting's amount before recalculating it. Both logs' 11 hits are
  int(the sting's stored tick × 2.8), crits × 2.06.
- Goldens: only TestMM moved again, Average-Default −0.92% (53353 −14.4% a hit).
- Not done: the spell-mod sweep's re-record, since 7 of its 8 changed paths are EC-CLASS-FIX2's classes this wave.

Review, 2026-10-10:
- The cast fix is Enhancement's +0.27%, measured: with the base `cast.go` overlaid (`go test -overlay`) the suite
  sits at −0.016%, and Elemental's fire elemental rows at −0.01 to −0.06%. The elemental reaches its 15 Fire Novas
  (14.94 before), then only swings.
- `makeCastFuncSimple` set the cooldown before the shared cooldown's check; both now start after both checks. No
  golden moves.
- `damage_test.go`: a hand-checked Frostbolt 2 over the server fails, as 2 under does.
- Follow-ups: `PowerBonus` casts a float64 product to float32, where the server multiplies two float32s (0.13 ×
  1700 is 220 there, 221 here; 11 of the whole SP values under 6000 for that coefficient), and the sim's spell power can be
  fractional where the server's is an int32; Ferocious Bite and Eviscerate roll their base inline, not in whole
  steps; 53353 truncates after its 2.0 or 2.8, the server before; no UI test holds the sheet's defense skill.

### EC-PET-FIX2 (Z5; changes goldens; stages: code, live)

- Timers: pets' 2 s scaling and mana regen, and the 3 s hit and 2 s haste refreshes (`sim/core/pet.go`), count from
  the summon in the sim; the server runs them on the map update.
- Spell crit from intellect (`EnableManaBarWithModifier`); a creature's is a flat 5% plus auras.
- Estimates no record covers: the Fire Elemental's stats, AP and weapon (200–360 on the server); Mirror Image's
  crit and mana. Live: measure them, and the intellect crit, then model what the records show.
- **Owns:** `sim/core/pet.go` and the pets' scaling and regen; the Fire Elemental's and Mirror Image's stats; their
  drops and rows.
- **Goldens:** the pet suites; report each.

#### As built

Stage code, re-measured on `a1bb8d403` against [ac] `3d89793c7`:
- Timers (`sim/core/pet.go`, `startUpdateTimer`): hit, haste, scaling and mana regen count from the map update
  before the pet's first one and tick on the update each period is up by, the wait not carried over
  (`AuraEffect::Update`, `Creature::Update`: `timer += period`). `ActionPriorityLow`, regen last. A pet out since
  before the pull draws its place in the cycles (`"Pet Timers"`, one start for all four). Map interval 0 keeps the
  exact times. Fixed.
- Intellect crit: `EnableCreatureManaBar` wires its own mana bar, without the intellect dependency
  (`Unit::SpellDoneCritChance`: a non-player's is `m_baseSpellCritChance`). The strikes replay now holds a mana
  pet's spell crit to the record's `baseSpellCrit`: on base the Imp (7.50 vs 5), Felguard (6.18) and Water
  Elemental (7.54) fail it. Fixed.
- Fire Elemental, from source and DB, no record yet: `pet_levelstats` 15438 (283/160/125/405/460, 8000 health,
  2280 mana, 1100 armor), 2 × Str − 20 AP, weapon 200–360 at 2 s; `spell_sha_fire_elemental_scaling`: intellect
  30%, armor 35%, AP 300% and SP 100% of the owner's spell power, no stamina (35665's first effect is a dummy);
  15 mana a point of intellect, creature regen; Fire Blast costs 40 (57984), Fire Nova 30.
- Mirror Image, from source and DB, no record yet: fallback create stats, crit a flat 5% (the 8% went),
  `spell_mage_pet_scaling` as the Water Elemental's; pool 8814 (`creature_classlevelstats`; the `NPC_MIRROR_IMAGE`
  case only resets the create mana, 2428) plus 15 a point; Frostbolt costs 90, Fire Blast 120.
- No drop or README row was theirs.
- Goldens (Average-Default): Destruction −1.2% (Firebolt crit 25.8 → 17.9%, so Empowered Imp 25.9 → 18.0 s),
  Frost −0.4% (Waterbolt crit 24.2 → 18.0%), Affliction −0.2% (Shadow Bite crit), Arcane, Fire and FrostFire
  −0.03 to −0.05% (Mirror Image crit), BM +0.15%, MM, SV, Unholy and Demonology within ±0.05% (timers),
  Enhancement +0.16%, Elemental +0.07%: the elemental swings for 2611 (was 3296) and Fire Nova hits for 2622
  (was 1423), so its multi-target tests gain 12 to 21% (Enhancement's up to 4.9%).
- Left: focus and energy regen (24 every 4 s, 20 every 2 s on the map update; `sim/core/focus.go` ticks each
  second from the summon); a pet's MP5 inside the truncated regen tick; Improved Demonic Tactics (every 10 s,
  whole percent, a spell mod); the Fire Elemental's AI timers and its 13339 id.

Stage live, 2026-10-10 against [ac] `3d89793c7`, no rebuild:
- e2e `TestPetfixRecords` (`e2e/petfix_test.go`, module README): Greater Fire Elemental, Mirror Image, Imp and Water
  Elemental through `pet:<n>`; 18 records, the new `pets.jsonl`, replayed by `TestPetsRecords` (`tools/simval/pets.go`;
  `simPets` gains 15438 and 31216, `petsThatNeverSwing` 31216). 152 checks pass.
- Fire Elemental: every stat, pool (8000 health, 2280 + 15 a point mana), AP, SP, armor, weapon range, regen, cost and
  spell amount is as stage code modelled it. One gap, fixed: its swing is fire, so `Unit::CalculateMeleeDamage` skips
  the sheet's range for `Creature::CalculateMinMaxDamage`'s, (weapon + AP/14) × attack time: mean 860 a plain swing at
  2106 AP where the sim rolled 580. Physical pets keep the sheet's range. No glancing (a guardian isn't `IsPet`); a
  swing takes the level resist.
- Mirror Image: crit 5% (rolled 5.0% of 20000), mana 8814 + 15 a point, health, armor, SP, amounts and costs as
  modelled; it carries none of the mage's auras.
- Intellect crit: Imp (416 intellect) and Water Elemental (423) show `critTaken` 5 and roll 4.86% and 4.85% of
  20000; the elemental (442) 5.2%. Confirmed.
- Goldens against the base, shaman suites only moved from stage code: Elemental +0.6% (Convection-Average; swing
  3296 → 2966 a hit, was 2611 after stage code), multi-target up to +21%; Enhancement +0.7% (swing 2688 → 2536),
  multi-target up to +5.1%.
- Left: the sim's GCDs on the four guardian spells stand in for the AI's event timers (the server's GCD is 0;
  `petCadenceAllowed`, the classes' `serverdata_allowlist.go`); the Imp's and Water Elemental's health pools
  (`petsWithoutHealth`: 4421 and 7688 on the server, 8.4 and 7.5 a point of stamina; nothing in the sim reads them).

Review, on `a1bb8d403`:
- The swing fix moved to core (`Pet.rollCreatureRangeOffPhysical`, from `Pet.Finalize`): any pet swing off the
  physical school rolls (weapon + AP/14) × attack time. The Shadowfiend's is shadow (`creature_template` 19668:
  `dmgschool` 5, `DamageModifier` and `BaseVariance` 1, as 15438's), so it had the elemental's gap. From source and
  DB: no `melee` record of it yet. `TestPetSwingOffPhysicalRollsTheCreatureRange`.
- Goldens: Shadow +0.16% (Average-Default; swing 1838 → 1980 a hit, tests +0.13 to +0.46%); the other 13 suites as
  the stages left them.
- Left: a `melee` record of the Shadowfiend; a pet out since before the pull ticks nothing during the prepull, its
  first tick lands up to a period after 0 (as before the item); a tick sharing a map update with the pet's own
  swing or cast runs after it, where the server's aura tick comes first.

### EC-CLASS-FIX2 (Z5; changes goldens; stages: melee, casters, live)

- Death knight: Blood Tap's rune pick against `EffectActivateRune` and `spell_dk_death_rune` (the talent takes over
  the tapped slot); a Threat of Thassarian off-hand strike rerolls partial Death Rune Mastery (`CAN_PROC_FROM_PROCS`,
  no cooldown); Chill of the Grave procs per Howling Blast target hit; Dirge's class mask covers Obliterate's runic
  power; Blood Presence's 50475 heal (63611) procs for every DK in it, and with Improved Blood Presence in the
  others, on direct and periodic damage from non-triggered spells and auto attacks (the sim: tank DKs, direct hits).
- Paladin: Improved Devotion Aura's 63514 (+2/4/6% healing taken under the paladin's auras).
- Warrior: Mace Specialization (`sim/warrior/talents.go`) compares its mask to `ProcMaskEmpty`, which
  `GetProcMaskForTypes` never returns, so it applies whatever the weapons; once fixed, add it to the optimizer's
  `weaponTalentsOf` (`sim/optimizer/weapon_talents.go`) and flip `TestWarriorMaceSpecializationIgnoresTheWeapons`.
- Druid: Omen of Clarity from triggered item damage (16864: `CAN_PROC_FROM_PROCS` plus its CheckProc; Starfall's stars
  don't roll it live, so recheck); `innervate.go` adds a `ResourceMetrics` every iteration.
- Mage: Frostbite's and Mirror Image's chills count for Torment the Weak; Frostbite's root rolls Fingers of Frost
  (`spell_mage.cpp`'s 12494 sync); Brain Freeze's Blizzard case never fires (the tick is 42938); snare immunity is
  `IsWorldBoss` (a target field would be exact; a proto change goes in contractChangeRequests).
- Priest: Mental Agility's mask leaves out Penance (server cost 618, sim 556); 53000's damage coefficient is 0.1905,
  the sim's 0.2290; heal Penance's 53007 hit gives an Illustration of the Dragon Soul stack the sim's doesn't.
- UI: RaidStats shows Devotion Aura and Blood Pact unimproved at 1–2 points of their talents (`ui/raid/raid_stats.ts`).
- Live: what each verdict needs, at least Omen of Clarity's recheck, Blood Presence's heal on a DPS DK, and
  Penance's cost and damage.
- **Owns:** `sim/{deathknight,paladin,warrior,druid,mage,priest}/` but the other Z5 items' hunks; `weaponTalentsOf`
  and its test; `raid_stats.ts`'s two rows; their cycles, fights, drops and rows.
- **Goldens:** those classes' suites; report each.

#### As built

Melee stage, on `a1bb8d403`, each lead re-measured against [ac] `3d89793c7` and the spelldump (notes and the live
checks in `tmp/ec-class-fix2-melee.md`). Verdicts:
- Blood Tap's rune pick (fixed, `BloodTapConversion`): `EffectActivateRune` runs first, at launch. It readies a blood
  slot's death rune that's on cooldown, else, only with both blood runes plain and down, the one due back sooner (the
  second on a tie). Then the aura turns the first plain blood rune that's ready, so sometimes none. The sim turned a
  rune whatever its state and always readied one.
- The talent takes over the tapped slot (fixed, `procDeathRuneTalent`): a proc puts its own aura on the rune it turns
  (`AddRuneByAuraEffect`), so a tapped rune Blood of the North or Reaping turns again reverts with the talent's 30 s,
  not with Blood Tap. The sim turned the other blood rune instead.
- Off-hand rerolls (fixed): the three death rune talents are `CAN_PROC_FROM_PROCS`, so Threat of Thassarian's
  off-hand Blood Strike, Death Strike and Obliterate roll them again for the runes the main hand just spent
  (`RollBloodDeathRunes`, `RollFrostUnholyDeathRunes`; `RuneMeta.spentAt` is the script's "just went on cooldown").
  Blood of the North and Reaping too: their 100 ms cooldown starts only on a proc, so a failed main-hand roll leaves
  the off hand free. After a main-hand miss the off hand's proc turns nothing but restarts the periodic.
- Chill of the Grave per Howling Blast target (no gap): spell_proc 49149 is phase HIT, and `howling_blast.go` and
  the threat hook already give it per landed target.
- Dirge on Obliterate (fixed): spell_proc 49223's mask has Obliterate, so +2.5 runic power a point.
- Blood Presence's heal (fixed, `registerBloodPresenceHeal`): 63611 is on every DK in Blood Presence (4%) and, with
  Improved Blood Presence, in the other two (2% a point). It procs on auto attacks, the DK's own casts, every
  periodic damage tick, and triggered spells only with `NOT_A_PROC` (Death Coil's 47632, Necrosis): not the off-hand
  strikes, Scourge Strike's 70890, Blood-Caked Blade, Razorice (all three `SUPPRESS_CASTER_PROCS`), Wandering Plague
  or Death and Decay's 52212. The heal is int32(damage × percent), then the healing taken percent, truncated.
- Improved Devotion Aura's 63514 (fixed): aura 118, +2% healing taken a point on whoever carries that paladin's
  Devotion Aura: ×1.06 with the raid buff Improved (`sim/core/buffs.go`), 1–2 points through `improveDevotionAura`.
- Mace Specialization (fixed): needs a mace in either hand (`HasItemFitToSpellRequirements`); it's in
  `weaponTalentsOf`, and `TestWarriorMaceSpecializationNeedsAMace` replaces the old test. The WarriorArms sweep step
  (two-handed axe) is now a sheet artifact like the rogue's: dropped, with its README row.
- RaidStats (no gap): the rows key on `Boolean(talent)`, so a paladin or warlock with 1–2 points already counts under
  the Improved row, never the plain one. `raid_stats.ts` is unchanged.

Core hunks: `sim/core/runic_power.go` (DK only), `sim/core/buffs.go` (the 63514 line). Tests:
`sim/deathknight/dps/{blood_tap,presence_heal,runes}_test.go`, `sim/paladin/retribution/devotion_aura_test.go`,
`sim/optimizer/weapon_talents_test.go`.

Goldens (`dock.sh delta`), Average-Default: FrostUH −5.5% dps (Obliterate 74.6 → 68.5 casts), Frost −1.7%, Blood
−0.7%, Unholy −0.3% (Ghoul Frenzy 6 → 4.9 casts, hps −13.8%): the rotations cast Blood Tap where the old one always
gave a rune. TPS −4.1 to +1.3%: it follows the damage, plus the heal's level 40 a proc on a DPS DK. BloodTank dps ±0.06%, tps up to
+1.3%. Every other suite moves only in hps, +6% (63514 under the Improved raid buff): the Bauble of True Blood rows,
Disc +0.9%, Holy priest +1.9%, Resto shaman.

Follow-ups: retune the DK APLs' Blood Tap conditions (`ui/deathknight/apls`); `SPELL_AURA_MOD_HEALING_PCT` counts
only the highest positive aura (`SpellHealingBonusTaken`), the sim multiplies them all (Vampiric Blood, Guardian
Spirit, Spirit Bond, 63514); Improved Blood Presence's +5/10% healing taken in Blood Presence; Unholy Blight's ticks
skip the damage callbacks, so `DONE_PERIODIC` procs miss them; an on-use item's damage is a non-triggered cast and
procs 63611; `class-mechanics.md` says RaidStats shows 1–2 points unimproved.

Casters stage, same base (notes and the live plan in `tmp/ec-class-fix2-code.md`). Verdicts:
- Omen of Clarity from triggered item damage (not changed, live decides): by the source a magic-class direct damage
  or heal spell rolls 8.75% triggered or not, but so would Starfall's stars, which Z2 saw never roll, and nothing in
  the proc code explains that. The sim rolls it on Hurricane's tick (48466), which the recheck covers too.
- Innervate (fixed): the aura is made once, post-finalize, not a new `ResourceMetrics` each reset.
- Torment the Weak (fixed): a Mirror Image's Frostbolt (59638, 4 s plus Permafrost), Blast Wave (6 s) and Dragon's
  Breath (5 s, less under Review) snare a target that isn't a world boss. Frostbite doesn't count: 12494 is a root, and only snares
  and attack slows do.
- Frostbite's Fingers of Frost (refuted): the 12494 sync needs the root's triggering aura, which the
  `ADD_TARGET_TRIGGER` cast never passes (`Spell::DoTriggersOnSpellHit`), so it never rolls.
- Brain Freeze (fixed): spell_proc -44546's mask has Blizzard's tick, so each landed tick hit rolls, with or
  without Improved Blizzard; Chilled is outside it. Misses no longer roll (`brainFreezeMask`).
- Penance (fixed): Mental Agility's mask leaves it out (cost 618); a tick's 53000 takes 0.1905 of spell power
  (`spell_bonus_data`).
- Penance and Illustration of the Dragon Soul (refuted as worded, fixed the other way): 53007 and its channel are
  `SPELL_DAMAGE_CLASS_NONE`, outside 60485's flags, so only the three ticks give stacks. The sim's damage Penance
  gave a fourth from an outcome at the cast, which also rolled a hit the server doesn't (class NONE with a family
  never misses): the cast now rolls and deals nothing.

Drops: Penance's `cost` entry in `droppedCostAuras` and its README row are gone; `channel` stays, a probe artifact.
Tests: `sim/mage/procs_test.go`, `sim/priest/smite/penance_test.go`.

Goldens on top of the melee stage: mage Frost Average-Default −0.04% dps (rows −0.7 to +0.9%; Frostfire Bolt 29.2 →
28.8 casts, as a missed Frostbolt no longer rolls Brain Freeze); one Disc row's tps +0.004%. Nothing else moves.

Follow-ups: a snare immunity field on the proto target (the mage reads `IsWorldBoss`); item damage spells carry
no server spell id (`sim/common/wotlk/damage_procs.go`), which the Omen fix needs if live confirms it; heal
Penance's hits count one short (`Hits--`); `class-mechanics.md` says Starfall's stars proc nothing, they proc
Illustration.

Live stage, 2026-10-10 against `3d89793c7` with module `408f7b5`, no rebuild (runs, logs and counts in
`tmp/ec-class-fix2-live.md`). The module's new `TestSimvalEcClass2Fight` (`e2e/ecclass2_test.go`, module README)
fights one-off cycles: a chosen spot, no swing, casts at the bot or the ground, an XP-giving mob.
- Omen of Clarity (fixed for druid spells): Z2's refutal was an artifact. From 18 yards north no star falls, as
  `spell_dru_starfall_dummy` wants line of sight to the dummy's centre. From 4 yards south Clearcasting came at 6 of
  82 star hits, 29 of 288 Hurricane ticks, 3 of 30 Hurricane casts and 4 of 29 Extract of Necromantic Power bolts,
  each on the hit's own millisecond. Stars and splash hits now roll it (`SpellFlagOmenTrigger`), and Hurricane's
  cast (`rollOmenOfClarity`). Item damage stays unfixed: its spells carry no server id to read the class and cast
  time from (60488's 2.2 s entry makes 12.83%).
- Blood Presence's heal (confirmed): none on a simval dummy, which carries `NO_XP` (63611 needs an XP or honor
  target). On the Heroic Training Dummy 192 heals in 90 s: every swing, own cast and disease tick, no off-hand
  strike or Razorice hit, each exactly int(damage × 0.04).
- Blood Tap (confirmed): one blood rune down, the tap brings none back (a third Heart Strike is refused); both
  down, one comes back; a Blood of the North death rune on cooldown comes back and the ready rune turns (three
  Blood Strikes). The take-over fits its log but a missed Icy Touch leaves it unproven.
- Penance (confirmed, one more fix): cost 618 and 494 in a fresh sweep; 53000 probes at 0.1905; three Illustration
  stacks a Penance, damage or heal, none at the cast. Searing Light and Twin Disciplines multiply on 53000 (974 at
  2290 spell power), the sim added them: fixed.

Drops: `costauras.jsonl`'s PriestDiscipline lines are the fresh sweep's; 53007 still has no channel, so the
`channel` drop stays. Tests: `sim/druid/balance/starfall_test.go`.

Goldens on top: Balance Average-Default 0 (Clearcasting 44.6 → 47.4 s), 17 rows −1.6 to +8.6%, the mana-starved
NoBuffs rows most.

Follow-ups: item damage spells need their server spell id (`sim/common/wotlk`), then any landed magic-class
damage or heal spell rolls Omen at its own cast time entry (60203, 60307, 60443, 60483 at 8.75%, Sapper Charge
too; not Saronite Bomb, class none); by the same source rule Nature's Grace's 16886 (magic class, on each crit of a
cast spell) rolls it and Survival Instincts' 61336 (class none) doesn't, the sim has it the other way; Hurricane's
ticks give no Illustration stack live and its cast gives one; the sim's Blood Presence heal ignores the XP rule;
the fight e2e's north spot hides the dummy's centre, so area scripts that check it (Starfall) can't be recorded
there; `class-mechanics.md`'s Starfall line is wrong on Omen.

Review, on the same base:
- Heal Penance (fixed): its multiplier was 1 + the talents' product + Twin Disciplines, about twice the server's.
  Spiritual Healing and Twin Disciplines are percent mods on 52985, Blessed Resilience and Focused Power healing
  done auras, so all four multiply, as the live 2960–3170 shows. Disc hps −21 to −23% (Penance 12450 → 6505 a
  hit), tps −30%.
- Dragon's Breath's snare (corrected): 42950's aura has one proc charge (spell_proc -31661), so the first direct
  hit that damages the target takes it off; dot ticks and Living Bomb's explosion don't. It has its own aura
  (`dragonsBreathSnares`), not the casters stage's flat 5 s. No golden moves.
- Tests: `sim/priest/healing/penance_test.go`, `TestDamagePenanceTalentsMultiply`, the Dragon's Breath part of
  `TestTormentTheWeakCountsImagesAndFireSnares`.
- Contract request: 63514 multiplies with a larger healing taken aura the server would count alone (Spirit Bond,
  Vampiric Blood, Guardian Spirit, Strength of Wrynn); core needs the highest positive aura only.

### EC-THREAT-FIX4 (Z5; changes goldens, full suite; C++; stages: threat, C++ and e2e, live)

- Probe: `.simval threat` gains an energize field, so energize threat gets a record (Bloodrage's 20 rage should give
  100, split); 22845 (Frenzied Regeneration) and 44578 (Lifeward) heal threat probed.
- Replenishment from the caster's own spells (Vampiric Touch, Hunting Party, Judgements of the Wise, Enduring
  Winter, Improved Soul Leech) is 57669 `PERIODIC_ENERGIZE`, whose ticks make the caster's threat; the sim's is MP5,
  with none.
- Auras the sim applies without a cast skip their spell's level bonus as assist threat: Bloodrage's 29131 (10),
  Heartpierce's Invigoration (80), Lifeblood's 55503 (55).
- Silent Resolve is a holy and shadow school `SPELL_AURA_MOD_THREAT` (8/15/21%) plus a `SPELLMOD_THREAT` on one class
  mask (14523, 14784, 14785); the sim applies 7/14/20% per spell.
- Live, after the user rebuilds with this item's C++: `threat.jsonl` re-recorded with the new field and probes.
- **Owns:** `sim/core/threat.go` and the threat hooks in core; Replenishment's mana and threat; Silent Resolve's and
  the three auras' threat lines; `tools/simval/threat*.go`; `threat.jsonl`; in the module: `src/`, its SQL if a
  command changes, `e2e/threat_test.go`, the README's Commands rows and threat section.
- **Goldens:** every suite whose TPS or mana moves; report each.

## Orchestrator onboarding (wave S start)

- Add waves S–X to the registry and an `EC-` row to "Where the specs are".
- The RUNBOOK's "files workstreams hands test-enrichment take master's side" now names e2e-coverage.
- Apply items' `contractChangeRequests`.
