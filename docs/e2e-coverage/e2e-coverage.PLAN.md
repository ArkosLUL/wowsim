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

## Waves

| Wave | Items | Why |
|---|---|---|
| S | EC-SWEEP2 (live) · EC-STACK · EC-TIMING (C++, live) | Cheap checks first; the timing probe EC-SPELLMOD needs |
| T | EC-SHEET · EC-SPELLMOD (live) · EC-THREAT (C++, live) · EC-AOE | SHEET builds on SWEEP2's sheet code; SPELLMOD needs `.simval timing` |
| T2 | fixes (G), specced once T lands | S's and T's mismatches, each fixed once with the full set known |
| V | EC-THREAT-FIX (G, FS) · EC-STRIKES (C++, live) · EC-BUFFS (live) | The fix needs T's probe |
| W | EC-GEAR (C++, live) · EC-MOBS (module SQL and config, live) | One rebuild for both |
| X | EC-FIGHT (live) | Fights wear GEAR's full gear, enchant procs included |

U is taken (the UI wave). Versus the INVESTIGATION's order, B7, A3 and A4 moved forward (cheap, independent) and
C1 moved last (after B6).

**Backlog:** golden rows against other mob types (test-enrichment's G4), once EC-MOBS lands; execute-range damage;
haste stacking beyond EC-BUFFS' pairs.

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

### EC-THREAT-FIX (V; changes goldens, full suite)

- Fix what EC-THREAT dropped, nothing more: likely the flat bonus outside `PseudoStats.ThreatMultiplier`, the
  `spell_threat` values, `SpellLevel` threat, the split across targets. Restore the dropped records.
- **Goldens:** every suite whose TPS moves (tanks, warriors, ferals at least); report each.
- **Owns:** `sim/core/spell_result.go`'s threat, the class files EC-THREAT names, `tools/simval/threat_test.go`'s
  drops.

### EC-STRIKES (V; golden-neutral; C++; stages: C++ and e2e, sim, live)

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

### EC-BUFFS (V; golden-neutral; stages: code, live)

- e2e `e2e/buffs_test.go`: on a golden setup's character, each `FullBuffs` buff alone, then pairs in each exclusive
  category, with `info` after each; on the boss dummy, each `FullDebuffs` debuff with `armor`, a `damage` probe and
  the derived crit of `yellow`/`spell`. Talent-improved buffs come from a second bot with the talent casting them.
- Sim (`tools/simval/buffs.go`, `TestBuffRecords`, `buffs.jsonl`): the sheet change per buff and the armor, taken and
  crit change per debuff against the sim's.
- **Owns:** `e2e/buffs_test.go`, `tools/simval/buffs.go`, its test and fixture, its fixture README rows, the simval
  README's Buffs.

### EC-GEAR (W; golden-neutral; C++; stages: C++ and e2e, sim, live)

- `.simval enchant <slot> <enchantId>` (`ApplyEnchantment` off, `SetEnchantment`, on) and
  `.simval gem <slot> <socket> <gemItemId>` (`HandleSocketOpcode`'s steps: socket enchant, socket bonus, meta toggle;
  reference: playerbots' `PlayerbotFactory::ApplyEnchantAndGemsNew`).
- `goldchar_setups.json` gains the gear set's gems and enchants; `TestSimvalGoldenCharacters` applies them;
  `goldchar.jsonl` is recaptured, with the regen fields, and `TestGoldenCharacters` builds the full gear and
  compares regen. EC-SHEET's checks must still pass.
- **Owns:** module `src/SimValidationCommands.cpp`, `simval_commands.sql`, `e2e/goldchar_test.go`, its module README
  rows; `goldchar_setups.json`, `goldchar.jsonl`, `goldchar.go`'s setup building, `tools/simval/goldchar_test.go`,
  its fixture README rows, the simval README's Golden characters.

### EC-MOBS (W; golden-neutral; module SQL and config; stages: SQL and e2e, sim, live)

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

### EC-FIGHT (X; golden-neutral; stages: sim tooling, e2e, live)

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

## Orchestrator onboarding (wave S start)

- Add waves S–X to the registry and an `EC-` row to "Where the specs are".
- The RUNBOOK's "files workstreams hands test-enrichment take master's side" now names e2e-coverage.
- Apply items' `contractChangeRequests`.
