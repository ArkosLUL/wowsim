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
| Z7 | EC-CORE-FIX4 (G, FS) · EC-PET-FIX3 (G, live) · EC-CLASS-FIX3 (G, live) · EC-TOOLS | Z5's follow-ups (Z6 is parity's and raid-import's) |
| Z8 | EC-CORE-FIX5 (G, FS) · EC-PET-FIX4 (G, live) · EC-CLASS-FIX4 (G, live) | Z7's follow-ups; the fourth place is bis-optimizer's BIS-TANK-MAGIC |
| Z9 | EC-CORE-FIX6 (G, FS, live) · EC-PET-FIX5 (G, live) · EC-CLASS-FIX5 (G, live) | Z8's follow-ups; the fourth place is bis-optimizer's BIS-TANK-PICKS |
| Z10 | EC-CORE-FIX7 (G, FS, live) · EC-CLASS-FIX6 (G, C++, live) · EC-TOOLS2 | Z9's follow-ups, and EC-TOOLS2 moved from Z8, then Z9; the fourth place is bis-optimizer's BIS-TANK-SWAP (the user's call, 2026-10-13) |
| Z11 | EC-CORE-FIX8 (G, FS, live) · EC-PET-FIX6 (G, live) · EC-CLASS-FIX7 (G, live) · EC-TOOLS3 (C++, live) | Z10's follow-ups; EC-PET-FIX6 moved from Z10 |
| Z12 | EC-CORE-FIX9 (G, FS, live) · EC-PET-FIX7 (G, live) · EC-CLASS-FIX8 (G) | Z11's follow-ups and the tank fixes; the fourth place is bis-optimizer's BIS-TANK-HEALTH (the user's call, 2026-10-15) |
| Z13 | EC-TOOLS4 (live) | moved from Z12 |

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

From Z5, not in Z7: Mana Burn, which Silent Resolve's spell mod covers and the sim lacks; `.simval threat`
doesn't record `POWER_DRAIN`'s gain (Dark Pact), and the replay can't compare the tick threat of 71883 and
57669; Heart Strike takes no Rage of Rivendare or Tundra Stalker (no level 80 build holds both);
`spell_dru_rip` adding Rip's AP share after the done percents (a lead); the raid's Replenishment buff never
restarts the tick timer.

From Z9: PaladinRetribution's Replenishment check passes at the top of the sim's range (log 1128, sim 1068 to
1128), an exact replay needing the cast-against-tick order off the log; no golden summons a Val'kyr
(`NibelungAverageCasts` 0); a dummy that takes snares (module SQL), to show Hamstring, Slow and rank 5 Hurricane
giving Torment the Weak; the other e2e tests' `.cast <id> triggered` uses, unaudited for one that needs an effect
run; `TestParfu9PinAndSavageRend`'s bat control still at the entrance, with no line of sight; an APL `castSpell`
of Bauble of True Blood with no target heals the enemy; Forethought Talisman's heal over time under its item id
(60530 on the server); Improved Hamstring's and Glyph of Hamstring's root, and Curse of Tongues and Mind-numbing
Poison in Slow's cast time group 1096, unmodelled; the druid's heals, which will need `SpellFlagNaturesGrace`
and `SpellFlagOmenTrigger`.

From Z10: the miss cost of the ghoul's Claw, Cleave's main target and Primal Precision's Ferocious Bite and Rip,
from the source only, with no deterministic miss to probe; Piercing Howl's full cost on a miss rests on the
dummy's result being an immune miss, unread; `.simval trace` logs summons only; a creature's missile landing
against an aura running out on it, unfixed, which matters only once a landing reads its pet caster's aura; Ret's
run time over Z6 beyond the grid, spread over `Step`, `OnSpellHitDealt`, `dealDamageInternal` and the APL's
`IsReady` and `getFirstReadyMCD` with no hot spot; `fightSight`'s `LINE_OF_SIGHT` branch (both spots are in
sight) and `spellModRun`'s weekly-reset message (needs a reset day), unproven live.

From Z11: Mirror Image's chase from 19 to 19.5 yd, where the generator's 400 ms recheck (`PositionOkay` against 19)
can stop the image short of 17.5 on a run over more than one non-casting update; `petfixCheckImageHits` taking
each image's distance at 25 s for every Frostbolt; a shapeshift's `SetDisplayId` resetting the scale under Bloodlust.

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
  own capture ([server validation](../guide/server-validation.md)): `simval.jsonl` (the default run), `damage.jsonl`,
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

### EC-CORE-FIX3 (Z5, done; changes goldens, full suite; stages: code, live)

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

Cross-review, 2026-10-10, on `5692dd49d` with the three items merged:
- Healing taken (fixed, the class item's contract request): `Unit::SpellHealingBonusTaken` counts one
  `SPELL_AURA_MOD_HEALING_PCT`, the highest positive, times the lowest negative. `Unit.AddHealingTakenPct` and
  `RemoveHealingTakenPct` (`sim/core/health.go`) keep the list and fold the result into `HealingTakenMultiplier`,
  off whole percents. Through it: Improved Devotion Aura, Guardian Spirit, Spirit Bond, Blood of the Rhino, Vampiric
  Blood, Glyph of Frenzied Regeneration, Mind Trauma. Strength of Wrynn stays a plain multiplier: its third effect is
  healing done (aura 136), stood in on the one healed.
- `spendRuneCost` and `spendOnResult` no longer build the spent-slot slice nobody reads (an allocation a rune cast).
- e2e: `TestSimvalFight` failed HunterMM (Kill Shot never goes off on a full dummy) and saved a log the replay can't
  use; it now fights a `pull` or `dummyHealthPct` cycle through `ecCore3Fight`. `run.sh`'s default run skips
  `TestSimvalEcCore3LowHealthFight` and `TestSimvalEcClass2Fight` by name. Compiled and vetted, not run live.
- `gen_serverdata` rerun on the merged tree: no change. The fights README's empty drop table says so.
- Tests: `TestHealingTakenPctsDontStack`, `TestImprovedDevotionAuraDoesntStackWithGuardianSpirit`,
  `TestHealingTakenTalentsDontStack` (hunter).
- Goldens: TestFeralTank, one row's hps −1.4% (Glyph of Frenzied Regeneration's 20% no longer over the aura's 6%).
- Follow-ups: heal Penance rolls its base inline (`sim.Roll`), not in whole steps, as Ferocious Bite; Blood Presence's
  50475 takes Strength of Wrynn's stand-in, where the server's class-none heal takes no healing done percent.

Run 2, 2026-10-10, on `ef598ad48` against [ac] `3d89793c7`, no rebuild (notes in `tmp/ec-z5-live-sweep.md`):
- `spellmod_spells.json` regenerated: 279 talents, 359 spells, 1217 group-spell pairs. In: Seal of Righteousness
  25742, Penance's 52985 and 53000, Windfury Attack 25504 and 33750, Sunder Armor 7386 and 58567, Scourge Strike
  70890, Endurance (no spell). Out: Vampiric Touch 34917.
- Sweep re-recorded: `spellmods.jsonl` is 2274 lines, 29 paths of one run and PriestHoly of a rerun. The weekly raid
  reset (Saturday 17:00 local, `instance_reset.resettime`) ported its first bot out of Naxxramas at step 18
  (`InstanceMap::Reset`'s `RepopAtGraveyard`). The 2240 probes the old file shares are unchanged.
- `TestSpellModRecords`: 540 spell steps agree, 378 move nothing, 262 aren't compared, 2 disagree, the two standing
  drops (Berserk's Mangle (Bear) cooldown, Dispersion's miss). No new mismatch, no drop removed. No golden moves.
- Left: the 17 new step-spell probes aren't compared. The sim names those ids as a `ThreatSpellID` or a tick spell
  and deals them under another id.

### EC-PET-FIX2 (Z5, done; changes goldens; stages: code, live)

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

Cross-review, 2026-10-10, on `5692dd49d` with the three items merged:
- Fire Shield (fixed, the core item's contract request): 13376 is magic class with no `ALWAYS_HIT`, so
  `WorldObject::MagicSpellHitResult` rolls each tick's hit per target; it had `OutcomeMagicCrit`.
  `TestFireShieldRollsSpellHit`.
- Goldens (Average-Convection): Elemental −0.06% (Fire Shield hits 40 → 38.8, the rest the extra roll's reshuffle),
  rows −1.8 to +1.1%; Enhancement rows −1.2 to +0.7%.
- Not changed: the three images' hits truncate as one sum, each on the server (about a point a cast over, with a
  talent multiplier).
- Follow-up: a `spell 13376` record of the elemental (`petfixPets`, `petsProbes`).

Run 2, 2026-10-10, against [ac] `3d89793c7`, no rebuild (notes in `tmp/ec-z5-live-fiend.md`):
- `TestPetfixRecords` gains a Shadowfiend (`melee`) and the elemental's `spell 13376`; `pets.jsonl` is that run's 20
  records, 170 checks pass. No golden moves.
- Shadowfiend's swing, matches: at 1978 AP and 120–280 a 1.5 s swing, 18034 plain swings average 510.7 before the
  resist; the sim rolls 391–631, the sheet's range would give 412. Table: block 5.6%, crit 4.4%, nothing else. Probed
  out of combat, Shadowcrawl down.
- Fire Shield, matches: 13376's table isn't auto-hit, miss threshold 1700, 3397 of 20000 rolls missed; crit 5%.
- Replay: `rolled miss` holds a pet spell's rolled misses to the sim's chance. `TestPetsReplayCatchesAWrongRecord`
  fails a fiend on the sheet's range and a Fire Shield that never misses.
- Left: a swing under Shadowcrawl (63619), which the idle fiend never casts; the fixture README and
  `tools/simval/README.md` don't name the Shadowfiend or `rolled miss`.

### EC-CLASS-FIX2 (Z5, done; changes goldens; stages: melee, casters, live)

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

Cross-review, 2026-10-10, on `5692dd49d` with the three items merged:
- 63514 went through core's new `AddHealingTakenPct` (EC-CORE-FIX3's Cross-review), in `sim/core/buffs.go` and
  `improveDevotionAura`, with the other healing taken auras of these classes; no golden of theirs moves but
  TestFeralTank's row.
- Checked, no change: every damage and heal line the item added truncates once, where the server does; the cast
  reorder leaves Blood Tap and Death Coil as tested; nothing reads a pet's stats at a fixed time.
- Follow-up: Improved Blood Presence's 5/10% is now `AddHealingTakenPct` on Blood Presence's aura.

Rotations (EC-CLASS-FIX2-APL, on `5692dd49d`; `ui/deathknight/apls`): the changed lines tap only where the server
rule returns a rune. Average-Default, old model on the wave base → before → after:
- `frost_uh_pesti` 13938 → 13163 → 13826 (Obliterate 74.6 → 68.5 → 73.8 casts, Blood Tap's aura 11 → 87 → 14 s).
  Unbreakable Armor's sequence is Obliterate on the two death runes, Blood Tap, Unbreakable Armor, cancel: both
  blood runes are then plain and down, so the tap readies one, which pays the armor. It needs both death runes
  ready, no frost or unholy rune, and Frost Fever over 10 s (the tapped rune loses its grace, so both return a
  full 10 s later). Empower Rune Weapon now returns plain blood runes: the opener spends them on Pestilence and
  Blood Strike at once, one Obliterate fewer than the old model. The hidden alternative is Pestilence, Blood Tap,
  Unbreakable Armor.
- `frost_bl_pesti` 11908 → 11702 → 11850: the opener starts Pestilence, Blood Tap (the old order tapped nothing,
  then stopped before Empower Rune Weapon: 0.01 → 1 cast), and its death rune Obliterate is Pestilence, Blood
  Strike.
- `blood_dps` 12374 → 12290 → 12382: taps with both blood runes down, the sooner 5 s or more from ready.
- `uh_2h_ss` 12031 → 11989 → 12017: taps with no blood rune ready; Ghoul Frenzy takes a death rune in either
  blood slot (6 → 4.9 → 6 casts).
- `blood_pesti_aoe` (no suite): taps with no blood rune ready; nothing measured moves. `unholy_dw_ss` and
  `uh_dnd_aoe` are unchanged: their Blood Tap, Ghoul Frenzy sequence still lands 6 casts, though `uh_dnd_aoe`'s
  tap returns a rune one time in five on one target.

TestFrost's `frost_uh_pesti` rows fall 1.0–2.6%: its talents lack Epidemic, and 15 s diseases never meet that
rotation's disease rules. FrostUH's short rows move −1.5 to 0%. `db.json`'s APL presets move with these files.

Follow-ups: `frost_uh_pesti` stalls with both death runes ready and Frost Fever at 3–9 s (a Pestilence line would
fix it); TestFrost pairs `frost_uh_pesti` with talents it wasn't written for; `blood_pesti_aoe` needs a suite
before tuning (an 8 s threshold gains 3.9% full-buff multi-target, loses 2.5% unbuffed); `frost_bl_pesti` taps
only 1.4 times a fight; on `uh_dnd_aoe` a both-runes-down condition gains 0.7–1.1% multi-target and costs 0.2% on
one long target, under the old model too, so it's no Blood Tap break; the scratch harness (`tmp/apldbg`) belongs
in `tools/`.

### EC-THREAT-FIX4 (Z5, done; changes goldens, full suite; C++; stages: threat, C++ and e2e, live)

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

#### As built

Threat stage (offline; each lead re-measured against [ac] 3d89793c7 and the spelldump; `threat.jsonl` has no record
of any of them; notes and the live plan in `tmp/ec-threat-fix4-threat.md`):
- Replenishment: confirmed and fixed (`sim/core/replenishment.go`). 57669 is a periodic energize, 15 ticks a second
  apart, each `int32(float32(max mana) × 0.002f)` read as the aura goes on or is refreshed
  (`spell_gen_replenishment_aura`), 1 when it went on at no mana (the script's `Load`).
  - A refresh restarts the tick timer (`AuraEffect::CalculatePeriodic`: only periodic damage keeps it). The cast
    lands ahead of the unit's update (`Unit::Update` runs events, then auras), so a tick due on that world tick or
    map update is lost and the tick's event runs last at its time; with exact timing it goes first. A player's
    timer counts from the world tick before the cast.
  - Mana must be the unit's power: the cast drops other targets (`PowerCheck`), and a player's tick skips them (57669
    lacks the attribute 0x10000 of Ex7), so a druid in cat or bear form gets none.
  - Each tick gives its caster half the mana really gained, unmodified, and each cast that takes a target its level
    1, modified (`Unit.AddEnergizeTickThreat`, `CastThreat`). `NewReplenishmentSource` takes the caster.
  - The raid's buff (individual buffs) has no caster: a tick a second for good, reading the max each time, no
    threat. A proc over it owns its 15 ticks.
  - `fightEnergizeLeftOut`'s 57669 went: the five fights with it (HunterSV, MageFrost, PaladinRetribution,
    PriestShadow, WarlockDestruction) now compare its `amount` and `gain sizes` and pass, each logged tick size met
    exactly (22, 46, 12, 36, 56).
- Aura casts: confirmed and fixed with `CastThreat` (`Unit.NewCastThreat(spellID).Add`), a positive server spell's
  bonus, modified, booked under a `ResourceMetrics`' action: Bloodrage's 29131 (10; 2687 itself is
  `NO_INITIAL_THREAT`), Heartpierce's Invigoration of the wearer's power (80; 71881–71883, heroic 71886–71888),
  Lifeblood's 55503 (55). No periodic aura sets `NO_INITIAL_THREAT` on this server.
- Silent Resolve: confirmed in part and fixed. Its `MOD_THREAT` is on holy and arcane (misc 66), not shadow, at
  7/14/20% (base points −8/−15/−21 plus die side 1); its `SPELLMOD_THREAT` mask 0x10 is Mana Burn alone, which the
  sim lacks. Now one `AddThreatSchoolMultiplier`, the 15 per-spell multipliers back to 1.
- `Unit.GainThreat(actionID)` reads an action's gain threat. serverdata regenerated: the six Invigorations.
- Tests: `sim/core/replenishment_test.go`, `threat_test.go`, `sim/threat_casts_test.go`.
- Open: a mana pet under the raid's buff queues a tick event every second while it isn't out (Mirror Images,
  Water Elemental, Fire Elemental); stopping that needs a hook in `Pet.Enable` and `Disable`.

Goldens (`dock.sh delta`): 27 suites move; DK, rogue, resto druid and holy paladin don't. Average-Default:
- Replenishment's own ticks, TPS: Frost mage +0.33%, Destruction +0.26%, Ret +0.13%, SV +0.11%; Shadow sits at full
  mana, so none.
- Mana (truncated ticks, restarts): Elemental DPS −0.53% (one Lightning Bolt in 163), Balance −0.04 to −0.08%, Holy
  priest HPS −0.16%, Smite TPS −0.34% (two Shadow Word: Pains become Smites), the rest within ±0.02%. NoBuffs and
  small-pool rows move most, to −1.4%.
- Warriors TPS +0.002 to +0.006% (29131); Heartpierce rows up to +0.09%.
- Ferals: DPS within ±0.003%, 10 to 67 rows: no mana in form, and the ticks' events take expired auras off sooner.

C++ and e2e stage (compile-checked with `-fsyntax-only -Werror`, e2e vetted; not rebuilt, no live run; golden-neutral;
the live plan in `tmp/ec-threat-fix4-code.md`):
- `.simval threat` writes `energize` (format in the module README): the direct energizes on the caster (`cast`,
  `EffectEnergize` and `EffectEnergizePct` copied) and one tick of its periodic ones (`tick`, the amount read off
  the caster's aura, put on for a moment), each half the power, unmodified; live through the real
  `ForwardThreatForAssistingMe` (`energizeAssist`, `energizeTickAssist`). No command or SQL change.
- `TestSimvalThreat` gains a HolyPriest (Silent Resolve: Smite, Holy Fire, Mind Blast) and the probes 2687, 29131,
  55503 and 71883 (ProtWarrior; the first two on FuryWarrior too), 44578 (ProtWarrior, ProtPaladin), 22845 and 5229
  (BearDruid), 57669 (ShadowPriest). `threatCheckEnergize` holds the arithmetic and the live split.
- Replay (`tools/simval/threat.go`; 546 checks over the committed 161 records, was 385): `school multipliers` on
  every record, all seven schools; `energize threat`, `energize shares` and `energize tick threat` for records
  with the field; a `CastThreat`'s bonus counted (`Unit.CastThreatOf`, `Unit.EnergizeThreatOf` in
  `sim/core/threat.go`). Shadowform is a `threatForms` form, so the two priests' keys differ.
- Each new probe also replays on a hand-built record of what the source predicts
  (`TestThreatReplayOnHandCheckedRecords`).

Live stage (the user's rebuild: [ac] 3d89793c7 plus the module's working tree; golden-neutral, `dock.sh delta`
byte-identical to the code stages'; notes in `tmp/ec-threat-fix4-live.md`):
- One `run.sh TestSimvalThreat` (15 bots, 322 s, passed, no orphans) re-recorded `threat.jsonl`: 175 records, was
  161. The 14 `threatNewProbe` drops and the six `recapture` rows went. Replay: 611 checks, all pass;
  `knownThreatMismatches` stays empty.
- Every verdict left to the run holds, so nothing more was fixed:
  - Bloodrage 2687 and Enrage 5229: no bonus; 200 tenths of rage make 100, unmodified, a third per creature. Their
    ticks (29131, 5229) make 5 of 10 tenths.
  - 29131's level 10, Lifeblood's 55, Invigoration's 80 and Replenishment's 1: modified, in thirds (×2.0735 in
    Defensive Stance, ×0.7 in Berserker Stance with the talent and in Shadowform).
  - Silent Resolve: schools [1 0.8 1 1 1 1 0.8]; Smite and Holy Fire ×0.8, Mind Blast ×1.
  - Lifeward and Frenzied Regeneration's 22845: no bonus, heal threat half (a quarter for the paladin), modified.
  - Evocation's and Divine Plea's ticks: half the mana, unmodified.
- Not compared, for want of a sim spell to cast: the tick threat of Invigoration and Replenishment (server 10 of 20
  tenths, 6 of 12 mana).

Cross-review, 2026-10-10, on `8be984c35` with the four items merged:
- The threat stage's open item (fixed): `Pet.reset` and `Disable` pause a pet's Replenishment timer, `enable`
  resumes it (`replenishment.pauseTicks`, `resumeTicks`). The timer keeps its place, so the ticks land on the
  updates they did; a pet that isn't out queues nothing. A pet's permanent auras outlive `Disable` (`expireAll`
  only walks timed ones), so the raid's buff ticked for nobody after every dismissal too.
- Not golden-neutral, though the hook is: with a no-op event left at each paused tick time, TestArcane and
  TestEnhancement are byte-identical to the goldens. Without, they reshuffle: the sim expires auras only when an
  event moves the clock, tracker by tracker, so on seed 162 Arcane Power (out before 268.95 s) still raises
  the Arcane Missiles tick at 269.00 that the target's tracker fires first.
- Regen order (fixed): on a shared map update a pet's regen ran before its Replenishment tick, the server's
  after (`Unit::Update`, then `Creature::Update`'s `Regenerate`); near full mana the caster lost the tick's
  threat. Regen is now `petRegenPriority`, below the aura ticks; the tick is queued after the scaling timer
  (lower spell ids tick first).
- Uncontrollable guardians (fixed): Mirror Image (SummonProperties 1021) and the Greater Fire Elemental (61, from
  32982) have neither a pet's type nor category (`Guardian::Guardian`), so `TARGET_CHECK_RAID` skips them. Both
  are `Uncontrollable` now, and that drops the raid's Replenishment, a raid cast, as well.
- Checked, no change: heal threat is half the health gained after the healing taken percent, as the server's
  `gain`; Silent Resolve reaches Penance's ticks once, its damage tick's bonus unmodified; Dirge's threat hook
  already had Obliterate, Blood Tap is `NO_THREAT`, 50475 adds its level 40 a cast; the `CastThreat`s sit in the
  effects, past the cast checks; drop lists and READMEs agree; `gen_serverdata` rerun: no change.
- Tests: `TestReplenishmentTimerKeepsItsPlaceWhileAPetIsAway`, `…WaitsForThePetAgainNextFight`,
  `…ProcOnAPetThatIsAwayQueuesNothing`, `…TicksAheadOfThePetsRegen`, `TestMirrorImageTakesNoRaidCast`,
  `TestFireElementalTakesNoRaidCast`; `TestPetDropsBuffsItCannotTake` holds Replenishment.
- Goldens, 9 suites: Enhancement 160 rows within ±0.26% (Average-Default +0.005%) and Arcane 7 rows up to
  +0.003%, both reshuffles; Destruction TPS +0.001% (the Imp's tick threat); Fire's Firestarter row +0.00003%.
  A pet's `aura|57669` row goes from Blood (Bloodworm), the four mage suites and the three shaman ones.
- Follow-ups: expire auras in time order, so no unrelated event moves a golden; a proc's cast skips every
  guardian, the Water Elemental included (controllable, 1561), and still lands on a pet that's away (the Imp
  under Inferno), taking one of the ten places; by the same rule the gargoyle (209), army ghouls (687) and Raise
  Dead's guardian (829) are uncontrollable; the raid's buff puts its aura on units without mana; direct
  `GainHealth` heals (Lifeward, Frenzied Regeneration's integer steps) aren't whole.

### EC-CORE-FIX4 (Z7, done; changes goldens, full suite)

- Aura expiry: an aura expires only when some event advances the clock, one unit's tracker after another, so an
  unrelated event decides whether a tick still sees a buff (Arcane Power on the last Arcane Missiles tick, Z5's
  second cross-review). Expire auras in time order.
- `Aura.countdownStart` doesn't take the cast's own world tick off a player's aura, so it expires one world tick
  late; Replenishment's `startTicks` applies that shift itself and loses it once core does.
- Weapon strikes: `Spell::EffectWeaponDmg`'s own `int32` steps (56 of 148 recorded strike amounts are 1 to 3 over).
- `core.PowerBonus` casts a float64 product to float32 where the server multiplies two float32s (0.13 × 1700 is
  220 there, 221 here), and the sim's spell power can be fractional where the server's is an `int32`: pass the
  coefficient and the power apart (about 180 lines).
- Whole steps left: direct `GainHealth` heals (potions, the DK's and druid's self heals, Lifeward, Frenzied
  Regeneration), the inline base rolls (Ferocious Bite, Eviscerate, heal Penance), the three Mirror Images
  truncating as one sum, Chimera Shot - Serpent truncating after its 2.0 or 2.8.
- Meta gems: an item swap moves the meta's stats only, not its effect or crit damage.
- `StartTargetAtHealth` moves the execute phases only at the first advance.
- Dot ticks (PAR-FU-10 as built): `AuraEffect::Update` stops at `GetTotalTicks`, `sim/core/dot.go` doesn't, so a
  dot whose haste adds ticks can tick once more on its expiry's update (12 s at 2401 ms: 5 ticks, the server 4);
  `Dot.Rollover` must zero `TickCount` first, as `RefreshTimersWithMods` does. One unit's same-update ticks run in
  spell id order (`Unit::_UpdateSpells`): `pyroblast.go` orders Ignite by hand, Living Bomb's ticks still read
  Ignite ahead (about 0.8% of Fire).
- The bleed damage-taken bonus (Mangle, Trauma, Stampede) goes by the spell's mechanic mask
  (`Unit::SpellDamageBonusTaken`; `Spell.TargetDamageMultiplier`): Savage Rend's first hit takes it, Pin's ticks
  don't.
- **Owns:** `sim/core/` but EC-PET-FIX3's pet paths; those inline lines in class code; their drops and rows.
- **Goldens:** all 37 (aura expiry); report each.

#### As built

Timing stage, on `fd12d68cc` against [ac] `3d89793c7`, offline (notes in `tmp/ec-core-fix4-timing.md`). Verdicts:
- Aura expiry (fixed): `Simulation.Step` stops at the earliest expiry ahead of every queued event; on one update
  players' auras go before the map's (`Map::Update`), and a unit's due ticks before its expiries
  (`Unit::_UpdateSpells`). A Fury warrior's DPS beside an Arcane mage is now bit-identical to its DPS alone.
- `countdownStart` (fixed): a player's aura counts from the world tick before its cast or landing; not one its own
  swing, tick or expiry puts on, nor a creature's or pet's update. Replenishment lost its own shift. Arcane Blast's
  buff measured 5992 ms live.
- Dot ticks (fixed): the fifth tick of four came from the late expiry, not the missing cap, and far more often than
  one in 50 (Arms' Rend: 0.17 an application). `Dot.outOfTicks` stops at `TotalTicks`, the tick owed until a refresh;
  `Rollover`, `Dot.Refresh` and `Dot.Activate` restart the count (the last two shadow the aura's for Holy Vengeance,
  Deadly Poison and Lacerate). `Dot.UpdateExpires` raises the max duration, `Aura.UpdateExpires` the expiry alone
  (stock Glyph of Backstab). One unit's due ticks run lowest spell id first (`Unit.tickingDots`); `pyroblast.go`'s
  hand ordering and `rupture.go`'s own cap went.
- `StartTargetAtHealth` (fixed): runs the phases the target is in.
- Meta gems (fixed, but TBC effects): `metaGemSwap`, only where the full swap changes the gem's condition, moves
  every crit multiplier and runs the gem's `OnMetaGemSwap` toggles (`NewMetaGemEffect`; the eight in
  `sim/common/wotlk/metagems.go`). `sim/common/tbc/metagems.go` isn't owned: those effects stay the starting gear's.
- No drop or README row belonged to these bullets.
- Fallout: an aura with a `StartPeriodicAction` beside it now ends before the action a whole duration after the cast.
  `Unit.DurationThroughLastTick` replaces the `+1` ns in Killing Spree and Divine Plea.

Tests: `sim/core/update_order_test.go`, `TestItemSwapMovesTheMetaGemsCritDamageAndEffect`. `simval.jsonl` 1114/1114.
Fails by design: `sim/optimizer`'s `TestCalibrationGapComparesTheDerivedContextToTheRealRaid` asserts a non-zero gap
(+0.08% at base, all of it the mage's events expiring the warrior's auras).

Goldens (`dock.sh delta`), all 37 move, Average-Default within ±0.13% but:
- TestDisc hps −1.02%: Weakened Soul's 15 s is ten GCDs and is now gone at the tenth (Power Word: Shield 20 →
  20.32 casts, Prayer of Mending hits −3.6%).
- TestFire and TestFrostFire −0.45%: Ignite −1.7% a tick, its due tick ahead of the tick crit that rolls it over.
- TestArms −0.39%: Rend 153.6 → 151.2 ticks, Overpower 43.7 → 43.1 casts.
- TestUnholy tps +0.37%: Summon Gargoyle's 30 s is thirty GCDs, so Blood Presence comes 18 s sooner.

Follow-ups: the optimizer's gap test needs a pair that really couples; periodic actions aren't on their aura's
timeline; execute phase callbacks fire with the first clock stop past their time; a channel's tick still follows a
creature aura's expiry on a shared update; `spell_sha_t10_elemental_4p_bonus` only raises Flame Shock's max
duration past its own, so `GetTotalTicks` would cap the added 6 s (a lead); TBC meta gem effects across a swap;
the guide's expiry, tick order, dot cap and meta gem lines are stale.

Amounts stage, on the timing stage's tree, offline (notes in `tmp/ec-core-fix4-amounts.md`). Verdicts:
- Weapon strikes (fixed): `Spell.StrikeDamage` rolls whole between the truncated ends, the hand's TOTAL_PCT inside
  (`sim/core/weapon_strike.go`), `StrikeFixed` scales the fixed bonus, `core.ApplyPct` is each percent step; a
  rolled spell is `wholeStrike`, and `doneDamage` leaves that percent out. White swings roll the same way. Off
  the physical school there's no TOTAL_PCT, off hand's half included. Mutilate's and Shiv's off hand and
  Whirlwind's gained `TotalPctMultiplier`. All 164 recorded strike amounts are exact; `strikeTolerance` went.
- `core.PowerBonus(coefficient, power)` (fixed): two float32s; `Spell.SpellPower()` is whole.
- Whole steps (fixed): `GainHealing` (potions, Healthstone, Lifeblood, Judgement of Light, Lifeward, Blood Reserve,
  Death Pact, Death Strike, Rune Tap, Frenzied Regeneration, Improved Leader of the Pack, which now takes the
  healing taken percent); `Simulation.RollWhole` (Ferocious Bite, Eviscerate, heal Penance, the potion, Lifeward);
  a Mirror Image hit is one image's, times the images after the calc (`petHitsPerCast` went); Chimera Shot -
  Serpent truncates the sting's tick before its 200 or 280% (`CalcDamageDone`).
- Bleed bonus (fixed): `serverdata.FlagBleed` (`gen_serverdata` sets it; ten rows, regenerated identical), or
  Shred and Maul on a weapon hit. Savage Rend's and the pet Rake's first hit take it, Pin's ticks don't; the
  hand-applied 30% left Rake, Lacerate, Shred and Maul, so Stampede gives them its 25%.
- No drop or README row belonged to these bullets.

Tests: `sim/core/weapon_strike_test.go`. `simval.jsonl` 1114/1114; the optimizer's gap test still fails.

Goldens: all 37 move again, each within ±0.04% of the timing stage's number (melee and hunters −0.01 to −0.04%,
casters under 0.02%); TestFeralTank dtps +0.16% (the whole heals).

Follow-ups: the rune weapon's strikes and swing, Sweeping Strikes' copy, `pet.go`'s swing and Manifest Anger still
roll as floats; Steady Shot's weapon part is an `irand` of truncated ends; Eviscerate's and Envenom's AP share is
a float64 floor; the AoE cap goes into Fan of Knives' and Swipe's base; Multi-Shot rolls its weapon once;
`HealingPower` adds the target's flat healing taken before the coefficient; the guide's "Damage rounding" line is
stale.

Review, round 1 (the stages judged Average-Default only; every case was compared here). Fixed:
- Clearcasting told "just procced" by time left == duration, never true once a player's aura counts from the tick
  before: the proccing cast spent it (Balance's uptime 47 → 15 s, NoBuffs and set cases −13 to −20%).
  `Aura.RefreshedNow` replaces it; `sim/druid/feral/apl_values.go` keeps the old test, true for swing procs.
- The same shift cost Frenzied Regeneration and Enrage (druid) their tenth tick, and Heartpierce's Invigoration its
  last: `DurationThroughLastTick`, as Killing Spree.
- `ApplyFixedUptimeAura` at full uptime (Sniper Training, Focus Magic) dropped up to 10 ms a period: it activates as
  the unit's own update.
- A cast from the main hand's last-moment APL check is the session's, so its aura counts from the tick before
  (`WeaponAttack.swing`).
- A tick `RescheduleNextTick` puts in the past ran at once and was now skipped (`Dot.tickOwed`).
- A dot out of ticks on a unit nothing updates (a hot on a dismissed pet) polled to the fight's end: it ends itself
  past its expiry.
- The meta gem's crit swap moved heals with a damage gem and damage with Revitalizing, where the multipliers
  coincide: by `SpellFlagHelpful` now.

Tests: `update_order_test.go` (+5), `sim/druid/balance/clearcasting_test.go`, the heal in the meta gem swap test.

Goldens after the review: Average-Default as above but TestFeralTank dtps +0.11% and TestArms −0.39%. Cases over
1.5%, each explained:
- TestFeralTank multi-target −3.6%: Maul's second target has no Mangle, and the bleed percent is each victim's.
- TestHoly Glowing Twilight Scale −6%: its hot on the dismissed Shadowfiend ticked for the whole fight at base;
  the cap stops it at six.
- TestShadow NoBuffs −1.6%, TestBloodTank Unholy Presence −0.4%: hasted dots without the tick past their total.
- TestDisc −1.3% median: Weakened Soul, as the Average.
- TestUnholy hps to +5.9%: Blood Presence sooner. TestSV aoe +1 to +1.5%: one more Volley a fight, each channel
  ending a world tick sooner.
- TestBlood Sigil of the Hanged Man +3.4%, TestFire fire_aoe +1.6%, TestProtection dtps +1.7% (4.64): 20-iteration
  or small-number reshuffles.

Follow-ups: a periodic action has no owner, so an aura a talent's tick puts on (Prey on the Weak) counts a world
tick early; heals land on dismissed pets, whose auras never expire; a crit multiplier class code changes mid-fight
(Combustion) misses a meta gem swap; `pet.go`'s physical swings do roll whole (core's default auto attack).

Repair, round 1: no code change; the re-run matches the review's (delta identical, `simval.jsonl` 1114/1114). The
one failure left is `sim/optimizer`'s gap test, not this item's to edit. Pairs measured through a build overlay:
- `arcane_p3` as target beside `demonologyWarlock`, `DemonicPactSp` 0: gap −2.25%, the replacement to ask for.
- `arcane_p3` beside `fury_p1`: −0.018%, cause untraced.
- `fury_p1` beside `arcane_p3`: exactly 0.

Cases over 1.5% the review's list left out, none an Average case, not traced: TestArms Wrynn's Battlegear −1.6%
(119 item cases, median −0.43%), TestElemental Undead Slayer's Blessed Armor tps +1.7%, TestBalance
NoBuffs-LongMultiTarget −1.8%, TestSV aoe NoBuffs-ShortSingleTarget tps +2.0%, TestFeralTank
SwitchInFrontOfTarget dtps −4.1% (40.8 → 39.2).

Cross-review (Z7):
- `Simulation.Step` (fixed): an aura overdue on a unit nothing updated (a pet summoned again with one put on while
  it was away) took the clock back to its expiry. It goes at the current time (`TestOverdueAuraLeavesTheClockAlone`).
- Replenishment's refresh (fixed): it dropped the tick due on that world tick even when the cast came out of a
  creature's update or the unit's own swing, after the unit's auras ticked (Hunting Party off Explosive Shot's tick):
  `replenishment.updatedAheadOf`; `endsAt` went. TestSV tps +0.004% (145 rows, −0.12 to +0.30%).
- `sim/druid/feral/apl_values.go` reads `RefreshedNow`: on a world tick that doesn't divide 15 s (7 ms) a swing's
  proc never had its duration left (`TestSwingProcDelaysTheNextMoveAtAnyWorldTick`). Golden: the Gnomish Lightning
  Generator row of TestFeral (+0.06%) and TestFeralApl (+0.13%), whose hit's proc reads as fresh too.
- The five untraced cases, `18a41a330^1` against `18a41a330` over 5000 batches of the golden's 20 iterations
  (`.wave-loop/z7-xreview-ab`), none a bug:
  - Reshuffles: Elemental Undead Slayer's −0.02% (se 0.02), Balance NoBuffs-LongMultiTarget +0.14% (se 0.06),
    FeralTank SwitchInFrontOfTarget dtps −0.04% (se 0.14).
  - Arms Wrynn's Battlegear −0.60% (se 0.01): the Average's Rend tick and Overpower loss, larger where the lost
    tenth tick procced Taste for Blood 0.64 times (default gear 0.28). The rest is reshuffle.
  - SV aoe NoBuffs-ShortSingleTarget tps +1.61% (se 0.02): one more Auto Shot a fight (2 → 3), not a Volley. The
    channel's aura ends on time, so the ranged timer runs the 50 ms clip delay between two Volleys, here with
    0.19 ms to spare.
  - TestFeralTank's Average dtps is +0.01% (se 0.13), not the +0.11% above.
- A cast on its aura's last tick (confirmed, not fixed): `Unit::Update` runs `m_Events` (a hardcast's landing, a
  missile's hit) ahead of `_UpdateSpells`, which fires Auto Shot, ticks the auras, then removes the expired ones;
  melee follows in `Player::Update`. The sim expires first. A tie needs the landing on the aura's last 10 ms world
  tick: about 10 ms over the cast time of each expiry's last cast. The fix reorders landings, Auto Shot and held
  swings around the tracker pass (`Simulation.advance`, `releaseCastHold`) and reshuffles every caster's goldens.
- Meta gem swap (not fixed): Combustion and Hodir's Storm Power multiply a crit multiplier in place, so a swap
  meanwhile misses the spell; Prey on the Weak's and Predatory Instincts' values are the starting gem's. No preset
  swaps a meta gem's condition.

### EC-PET-FIX3 (Z7, done; changes goldens; stages: code, live)

- Guardian AIs: `npc_pet_shaman_fire_elemental` paces Fire Nova (5–20 s in, then 8–15 s) and Fire Blast (5–20 s,
  then 4–8 s) on event timers with no GCD, about 10 Novas and 19 Blasts in 2 min; the sim's rotation caps both at
  15 under its own GCDs, so its multi-target rows are overstated. Fire Blast is 57984, registered as 13339. The
  Mirror Image's 59638 and 59637 have no GCD either. The six serverdata allowances go.
- Timers: focus and energy regen on the map update (24 focus every 4 s, 20 energy every 2 s; `sim/core/focus.go`
  ticks each second from the summon); a pet's MP5 inside the truncated regen tick; Improved Demonic Tactics every
  10 s in whole percent, as a spell mod; a pet out before the pull ticks through the prepull.
- Guardians: by the SummonProperties rule (neither a pet's type nor category) the gargoyle (209), army ghouls
  (687) and Raise Dead's guardian (829) are uncontrollable too: confirm, and drop the party and raid casts they
  then can't take.
- Replenishment's targets: a proc's cast skips every guardian, though the Water Elemental (1561) is controllable;
  it lands on a pet that's away, which can take one of the ten places; the raid's buff goes on units without mana;
  `ProcReplenishment` keeps the sim's own target lists; the mana potion and gem heuristics read
  `ManaRegenPerSecondWhileCasting`, which no longer holds it.
- Imp and Water Elemental health pools (server 4421 and 7688).
- A guardian whose `Reset` calls `Pet.Disable` and is still out at a fight's end (Spirit Wolves in fights ending
  0–45 or 180–225 s in; check the Shadowfiend) starts the next iteration with the last one's inherited stats and
  haste taken off again: `Unit.reset` sets enabled before `agent.Reset` runs.
- Live: Shadowcrawl (63619) on the Shadowfiend's swing (`.aura 63619` before a second `melee` probe); the Fire
  Elemental's cast counts over its life.
- **Owns:** `sim/core/pet.go`, `focus.go` and the pets' timers; `sim/core/replenishment.go`'s target rules; the
  pets' files in class code; `pets.jsonl` and the module's `e2e/petfix_test.go`; their drops and rows.
- **Goldens:** the pet suites, and the mana users under Replenishment; report each.

#### As built

Stage code, re-measured on `fd12d68cc` against [ac] `3d89793c7` (notes and the live plan in
`tmp/ec-pet-fix3-code.md`). Verdicts:
- Fire Elemental AI (fixed): two event timers on its updates (`FireElemental.runEvents`), counted from the update
  before its first one (`Pet.AIClockStart`); whole ms draws. Neither spell has a GCD, and a script cast starts no
  creature cooldown (`Creature::AddSpellCooldown` is PetAI's and the pet bar's). Found on re-measure: the AI casts
  with `CastSpell`, not `DoCast`, so a Fire Blast during a Fire Nova's 2 s cast breaks it
  (`Unit::SetCurrentCastedSpell`; `Pet.InterruptCast`). A life gives 9.8 Novas started, 6.6 landed, 18.3 Blasts.
  Fire Blast is 57984.
- Mirror Image AI (fixed): `MirrorImage.summon`, `engage`, `runEvent`. 1.2 s wait, then `CasterAI`'s opener (either
  spell, evenly), one event an update, Frostbolt put back 2.5 s and Fire Blast 6.5 s, dead at 29.5 s: 9 Frostbolts
  of 3 s and 3 or 4 Fire Blasts. No GCD, no cooldown.
- Serverdata allowances: the four GCD ones and 13339's cooldown went. 12470's cooldown stays and 59637 gains one,
  both sim 0 with the reason corrected: the DBC recovery time never starts on a script cast.
- Focus and energy (fixed): `sim/core/focus.go` ticks 24 focus (whole, times Bestial Discipline) every 4 s ahead of
  the pet's AI and 20 energy every 2 s after it (`EnableCreatureEnergyBar`), on the pet's timers. The ghoul's AI
  runs on its updates. The strikes replay compares tick and interval (`Unit.FocusRegenTick`), the ghoul's too.
- MP5 (fixed): two fifths of the whole MP5 in `Pet.CreatureManaRegen`, five second rule or not; core's mana tick
  skips those pets; a full pool takes no tick.
- Improved Demonic Tactics (fixed): `sim/warlock/pet.go`. int32(int32(sheet crit %) × 10 a point / 100), spell for
  spell and melee for melee (the sim gave the spell share to both), reworked every 10 s on the warlock's world
  ticks from a drawn phase, on the demon at once when it comes.
- Prepull (fixed): a pet out since before the pull starts its timers at the prepull's start.
- Guardians (confirmed from SummonProperties.dbc: 209, 687 and 829 are category 1 with types 7, 3 and 2): gargoyle,
  army and Raise Dead's guardian are `Uncontrollable`. Found on re-measure: Bloodlust is a raid cast
  (`TARGET_UNIT_CASTER_AREA_RAID`), and the sim gave it to Mirror Images, the Fire Elemental and Bloodworms.
- Replenishment (fixed): a cast lands on every unit that's out and on mana, pets and guardians with a pet bar
  included; over ten, the ten lowest (`PowerPctOrderPred`, float32, stable), the rest keeping their aura. No
  per-source lists. A Spirit Wolf's power is mana with a pool of 0, so it sorts first (`Pet.EmptyManaPool`). The
  raid's buff only goes on a unit with mana. The potion, rune and gem heuristics add
  `Unit.ReplenishmentManaPerSecond`.
- Health (fixed): `Pet.SetCreateHealth`, `pet_levelstats` health and `Guardian::UpdateMaxHealth`'s per-stamina
  (Imp 8.4, Water Elemental 7.5, Succubus 9.1, Felhunter 9.5, Felguard 11). `petsWithoutHealth` went; the replay
  takes the sim's Blood Pact off an Imp whose record has none.
- Reset (fixed): `Pet.forgetSummon` ahead of the reset. The Shadowfiend had it as the Spirit Wolves did.
- `petCadenceAllowed` went: the pets' `timing` records compare the GCD.

Outside the owned paths, in place: `mana.go` (the tick's pet list), `raid.go` (Replenishment's fields and reset
call), `buffs.go` (the raid buff's mana check, Bloodlust's guardian check), `tools/simval/damage.go` (`petRegen`).

Tests: `sim/core/pet_regen_test.go`, five in `replenishment_test.go`, `TestFireElementalCastsOffItsEventTimers`,
`TestMirrorImageCastsOffItsEventTimers`, `TestImprovedDemonicTacticsTicksEveryTenSeconds`, `TestDemonHealth`,
`TestDeathKnightGuardiansHaveNoPetBar`. Replays all pass (`simval.jsonl` 1114/1114).

e2e (run in stage live): `TestPetfixRecords` runs the Shadowfiend's `melee` again under `.aura 63619` (`petsKey.under`
keeps the two apart; `petsProbes` gains it with the capture); `TestPetfixFireElementalCasts` reads an elemental's
casts and swings off the packets and fails a life that breaks the timers.

Goldens (`dock.sh delta`), 32 of 37 move, Average-Default:
- Elemental +0.56% (Convection): swings 39.3 → 50.0, Novas 15 → 6.6, Blasts 15 → 18.3; single-target rows −0.06
  to +3.7%, multi-target −10 to −17.8%. Enhancement +0.56%, same rows.
- Mages: Arcane −0.14%, Fire +0.04%, Frost −0.22%, FrostFire −0.05% (the images' timers and no Bloodlust; Frost's
  short rows to −2.6%).
- BM +0.21% (focus in 24s; Kill Command up 34.7 s for 31.3), MM and SV −0.002%.
- Demonology +0.002%, its Improved Demonic Tactics row −0.21%; Affliction and Destruction +0.001%.
- Death knights +0.003 to +0.007%, Shadow −0.002%.
- Protection paladin rows within ±0.09% (Dark Rune 0.34 → 0.28 casts), Retribution TPS, one Disc and one
  Restoration shaman row: the mana heuristics.
- Only `aura|57669` rows gone: Balance, Holy, Smite, the three rogue and three warrior suites.

Follow-ups: summons with a full mana pool the sim gives none (treants, Shadowfiend, Infernal) can take one of a
cast's ten places; the image's cast landing on the update it dies on is lost; the elemental's Fire Nova ignores
the haste auras that reach it; `gen_serverdata` and the audit CSVs still name 13339.

Stage live, on the server at [ac] `3d89793c7` (notes: `tmp/ec-pet-fix3-live.md`):
- `pets.jsonl` recut from `TestPetfixRecords`, 21 records; `petsProbes` gains the fiend's `melee` under 63619, and
  `TestPetsReplayCatchesAWrongRecord` a Shadowcrawl that adds nothing and one that counts twice.
- Shadowcrawl (confirmed, sim unchanged): the fiend's plain swing is 510.7 bare and 586.8 under it, 1.149 times, so
  the 15% applies once.
- Creature roll (fixed, found on the record): `Unit::CalculateDamage` rolls a whole number between the two truncated
  ends; the sim rolled a float and truncated after the percent, 0.4 a swing high bare and 1.1 under Shadowcrawl
  (`rollCreatureRangeOffPhysical`, `TestPetSwingOffPhysicalRollsAWholeNumber`). Fiend and Fire Elemental only.
- Fire Elemental (confirmed, sim unchanged), 6 lives of `TestPetfixFireElementalCasts`, a life: 9.67 Novas started
  (sim 9.82), 6.17 landed (6.63), 3.33 broken (3.06; 34% of the starts for 31%), 17.5 Blasts (18.29), 39.67 swings
  (41.09, now held by `TestFireElementalCastsOffItsEventTimers`). All 99 Blast gaps are 4.05 to 8.05 s and all 52
  Nova gaps 8.1 to 15.02 s, 21% of them under the DBC's 9.5 s cooldown; a Nova breaks when the Blast comes on its
  own update too. All 114 swings come 1.94 to 2.13 s after the swing, Blast or landed Nova before them.
- e2e fixes: `.learn` fails with the dummy selected, and a GM's elemental never attacks (`.gm off`); the test now
  logs and checks each swing's wait, and holds the swings a life to the sim's.
- Goldens on top of stage code (`dock.sh delta`, still 32 of 37): the roll moves only Shadow (−0.002% to −0.003%),
  Elemental (+0.562% to +0.561%) and Enhancement (+0.561% to +0.559%), the fiend's and the elemental's swings.

Follow-up: the live map update runs about 102.7 ms, so an unhasted 2 s creature swing takes 20 updates, 2055 ms (63
swing to swing gaps, 1994 to 2125), where the sim's exact 100 ms grid gives 2000: about one swing a life more in
the sim. Players' and physical pets' white swings roll whole numbers on the server too (`attack.go`).

Review and repair, on the same server build (logs: `tmp/e2e_images2.log`, `e2e_images3.log`, `e2e_casts5.log`):
- Fire Elemental swing order (fixed): `UpdateAI` runs its events ahead of `DoMeleeAttackIfReady`, so a cast on the
  update a swing is due takes it, a Blast by restarting the timer, a Nova by holding it. The swing goes through
  `ReplaceMHSwing`, and `ExecuteCustomRotation` runs the due events first. 40.1 swings a life for 41.1; live 40.2
  over 9 lives, with no swing on a cast's update. This replaces the 41.09 above.
- Mirror Image opener (fixed): images summoned into a fight are engaged in `InitializeAI`, as the target's threat
  list takes them in, with no victim for `CasterAI`'s opener: a Frostbolt an update after the wait and 3 Fire
  Blasts, never 4 (`engagedAtSummon`, a summon after time 0). A prepull summon keeps the opener.
- e2e: `TestPetfixMirrorImageCasts` watches the images' casts, three summons into a fight and three 3 s ahead of the
  pull. Two runs: into a fight 0 of 14 images opened on a Fire Blast, each starting a Frostbolt 1.26 to 1.32 s in,
  with 9 landed and 3 Fire Blasts; ahead of the pull 7 of 11 opened on a Fire Blast, at the pull, with a Frostbolt
  an update later, and a Frostbolt opener's first Fire Blast 9 s on. One image in three often casts nothing at the
  test spot. `TestPetfixFireElementalCasts` fails a run with a swing on a cast's update.
- Goldens on top of stage live (`dock.sh delta`, still 32 of 37; the other 26 unchanged): Elemental +0.405%
  (swings 39.3 → 48.8), Enhancement +0.385%; Arcane −0.155%, Fire −0.012%, Frost −0.246%, FrostFire −0.079% (the
  images' 59637 casts 6.98 → 6.50: the second summon of a fight has no opener).

Follow-ups: a 4th Fire Blast after a prepull opener lands 0.1 s before the images die in the sim, and wouldn't on
the live 102.7 ms update; the Bloodworm's health takes 1 a point of stamina (`Guardian::UpdateMaxHealth`), the sim
10, and nothing reads it.

Review round 2 (no live run, goldens unchanged):
- Prepull Fire Elemental (fixed): its event timers counted from the summon, so a totem dropped in the prepull
  cast before the pull. `FireElemental.enable` engages at the pull (`TestFireElementalOutBeforeThePullWaitsForIt`).

Follow-ups:
- A creature's swing after its hardcast lands comes one update early on the server: the cast lands in the event
  pass, ahead of `Unit::Update`'s timer decrement. 22 recorded swings after a landed Nova waited 1968 ms on
  average, 2057 after a swing or Blast; core's `resetSwingTimers` gives the full 2 s (about 0.3 swings a life).
- Mirror Images summoned at exactly 0 s count as out before the fight (`engagedAtSummon`).
- Whether and when an elemental summoned with nothing selected joins the fight is unmeasured.

Cross-review (Z7): the pets' timers, the Fire Elemental's and Mirror Images' event clocks and Improved Demonic
Tactics' tick are queued actions on their units' updates, not auras, so EC-CORE-FIX4's expiry order left them as
they were, and no check tells a fresh aura by its time left. Replenishment's refresh is in EC-CORE-FIX4's
cross-review. `e2e/run.sh` passes `SIMVAL_PETFIX_LIVES`; `spells_audit.csv` is regenerated, 13339 gone. Follow-ups:
Improved Demonic Tactics' tick follows an expiry on its world tick (the server ticks first); `Pet.Disable` expires
the raid's Replenishment, and a second summon doesn't get it back.

### EC-CLASS-FIX3 (Z7, done; changes goldens; stages: melee, casters, live)

- Item damage spells carry no server spell id (`sim/common/wotlk/damage_procs.go` and the other item damage
  effects), so no class aura can read their damage class or cast time. With it: Omen of Clarity rolls on any landed
  magic-class damage or heal at 3.5 PPM over the spell's cast time entry, floored at 1.5 s (Extract of Necromantic
  Power 12.83%, 4 of 29 live; ferals too: DMC: Death, Bandit's Insignia). Nature's Grace's 16886 rolls it by the
  same rule and Survival Instincts' 61336 shouldn't (neither measured).
- Death knight: Blood Presence's heal fires on a `NO_XP` target (`PROC_ATTR_REQ_EXP_OR_HONOR`), skips an on-use
  item's damage, and its 50475 takes Strength of Wrynn's stand-in though the server's class-none heal takes no
  healing done percent; Improved Blood Presence's 5/10% healing taken (`AddHealingTakenPct` on the presence's
  aura).
- Death knight rotations: `frost_uh_pesti` stalls with both death runes ready and Frost Fever at 3–9 s (a
  Pestilence line); TestFrost pairs it with talents without Epidemic; `blood_pesti_aoe` has no suite;
  `uh_dnd_aoe` taps with a blood rune ready; `frost_bl_pesti` taps 1.4 times a fight.
- Druid: Illustration of the Dragon Soul's trigger (Hurricane's ticks gave no stack live, its cast one).
- Mage: other units' snares (Hamstring, Frost Shock, Slow) count for Torment the Weak; snare immunity becomes a
  `Target` proto field from `creature_template.mechanic_immune_mask` (the snares key on `IsWorldBoss`). The same
  mask refuses a spell whose own mechanic the target is immune to (`Unit::IsImmunedToSpell`;
  `creature_immunities.MechanicsMask` is `1 << mechanic`): on the raid bosses Pin and Venom Web Spray (root),
  Ravage (stun), Tendon Rip (snare) and Snatch (disarm) deal damage only in the sim.
- Warlock: Demonology's rotation casts Curse of Doom only with `remainingTime > 60s`, so it skips a hasted curse
  that would land with 40 to 60 s left; it needs an APL value for the curse's length (proto and core), right
  with `SpellTweaks.CurseOfDoomScaling` off too.
- Hunter: Hunting Party procs off Explosive Shot's ticks; Serpent Sting keeps its first application's multiplier
  across Chimera Shot's refresh; the server applies the target's damage-taken mods to 53353 twice.
- Spell-mod sweep: 17 step-spell probes aren't compared (25742, 52985, 53000, 25504, 33750, 7386, 58567, 70890):
  the sim deals them under another id, so map each to the sim spell dealing it, cast time, cost and cooldown left
  out (Twin Disciplines and Searing Light on Penance's damage first).
- Fight means that pass far out: DeathknightBlood's Bloodworm melee +12% (z 3.4), Protection warrior melee −5%
  (z −2.8), Destruction's Immolate tick +15% (z 2.7).
- Live: the death rune take-over (a Frost cycle with no Icy Touch miss), Nature's Grace's Omen (a Wrath-only
  fight of 900 s), a WarriorArms talent path with a mace; Ignite's total on a hasted Fire mage (Pyroblast under
  Bloodlust for minutes: 40% of the fire crits feeding it, about 1.6% of Fire). Once the server runs
  mod-sim-validation `8a6f13f` (a rebuild by the user), rerun MageFire's spell-mod sweep and drop the fixture
  README's note on Pyroblast; exempting core's `spell_warl_curse_of_doom` (it acts on removal only) would compare
  Curse of Doom's damage too.
- **Owns:** `sim/{deathknight,druid,mage,hunter,priest,warrior,warlock}/` but the other Z7 items' hunks; the item damage
  effects in `sim/common/`; `ui/deathknight/apls/`, `ui/warlock/apls/`; `tools/simval/spellmods*.go`; `proto/common.proto`'s target
  field and its plumbing; their cycles, fights, drops and rows.
- **Goldens:** those classes' suites, and every suite with an item damage proc; report each.

#### As built

Melee stage, on `fd12d68cc`, each lead re-measured against [ac] `3d89793c7` (notes and the live checks in
`tmp/ec-class-fix3-melee.md`). Verdicts:
- Blood Presence's XP rule (partly fixed, `givesXPOrHonor`): `Player::isHonorOrXPTarget` wants a victim above the
  gray level (71 for an 80). The `NO_XP` flag stays unread: `proto.Target` has no field for it (contract request).
- On-use item damage (fixed, `onUseItemDamage`): `Player::CastItemUseSpell` casts an item's first spell
  untriggered, so the bombs, the Pyro Rocket and the Gnomish Lightning Generator heal, the sapper's hit on the DK
  too (by the source). The list keys on their sim action ids.
- Strength of Wrynn (fixed, `healingTakenPct`): 50475 takes the healing taken percents only; the DK reads the raid
  buff in `AddRaidBuffs` and divides the stand-in out.
- Improved Blood Presence (fixed): a flat `SPELLMOD_EFFECT2` on 48266's healing taken aura, 5% a point in Blood
  Presence only.
- Rotations, Average-Default base → after:
  - `frost_uh_pesti` 13826 → 13831: Pestilence with both death runes ready, no frost or unholy rune, Frost Fever
    at 9 s or less.
  - `frost_bl_pesti` 11850 → 11903 (Blood Tap 1.4 → 5 casts, Blood Strike 23.6 → 26.9): taps a blood-slot death
    rune 5 s or more from ready, after 19 s, Unbreakable Armor over 20 s away, on one target (on several the tap
    costs 0.7%: the armor's sequence needs it).
  - `uh_dnd_aoe` 12317 → 12310: taps with no blood rune ready; multi-target +0.7 to +1.1%, one long target −0.2%.
  - TestFrost drops `frost_uh_pesti` (TestFrostUH runs it); TestBlood runs `blood_pesti_aoe`, untuned.
- Hunting Party (fixed): spell_proc -53290's mask holds 53352, not the cast, with `TRIGGERED_CAN_PROC`: each crit
  tick of Explosive Shot rolls it. The ranks' chances are 33/66/100.
- Chimera Shot - Serpent (fixed): the sting's stored tick keeps the done percents it went on with
  (`AuraEffect::CalculateAmount` reuses `m_pctMods`), and 53353 adds none. The target's damage taken applied
  twice here; the live stage undid that. No truncation added (EC-CORE-FIX4's).
- Bloodworm melee (no gap): both logged summons came with all four strength procs up; the sim's worm at that AP
  hits 56–79, mean 67.8, against the log's 58–79, mean 69.3 (z 1.2). The replay averages summons at any AP.
- Protection warrior melee (no gap): the log's second Devastate was parried and Sunder Armor ran out at 44 s; sim
  fights that lose it the same way (7%) average 398 (z −1.7), glances agree.

Tests: `presence_heal_test.go` (four new), `sim/hunter/chimera_shot_test.go`, `hunting_party_test.go`.

Goldens (`dock.sh delta`), Average-Default: TestFrost +0.45% (133 rows −0.5 to +1.2%, 24 gone), TestFrostUH
+0.03% (133 rows), TestMM +0.69% (all rows; 53353 4036 → 4550 a hit), TestSV 0 (145 rows −0.1 to +1.6%,
Replenishment). TestUnholy: the 12 `uh_dnd_aoe` rows (dps to +1.6%, hps to +18.8%). TestBlood: 12 new rows. The
Gnomish Lightning Generator rows' tps +0.004 and +0.007%. `db.json`'s APL presets move.

Follow-ups: a `no_xp` target field, set on the simval dummies; a core accessor for the healing taken percents
apart from the healing done stand-in, which Death Strike's 45470 and Death Pact's 48743 (class none) may need
too; `blood_pesti_aoe`'s tuning; the fight replay summoning proc guardians when the log does.

Casters stage, same base (notes and the live plan in `tmp/ec-class-fix3-code.md`). Verdicts:
- Item damage (fixed, `itemDamageSpell` in `sim/common/wotlk`): the six proc trinkets, the capacitors and the
  Gnomish Lightning Generator deal their damage as the server spell (60307, 60483, 55748, 60443, 60488, 60203,
  54843, 67714, 67760, 55039), with a declared range, whole-step rolls and the bolt's travel (20 yd/s for 60488
  and the capacitors). Left: the bombs and the sapper (`sim/core`), Bryntroll, the items' heals.
- Omen of Clarity off item damage (fixed, `rollsOmenOffFamily`): any landed magic-class spell outside the druid
  family that deals direct damage, at the PPM over its cast time entry (8.75%, 60488 12.83%).
- Nature's Grace's 16886 (refuted): the talent has `SPELL_ATTR3_INSTANT_TARGET_PROCS`, so
  `Unit::TriggerAurasProcOnEvent` blocks procs while it casts 16886 on the druid; nothing rolls. Not changed.
- Survival Instincts (no gap): its `SpellFlagOmenTrigger` never rolled, as the spell deals no outcome; removed.
- Illustration of the Dragon Soul (fixed): Hurricane's cast deals a hit with no damage on the druid's side
  (`OnSpellHitDealt` only), one stack; the ticks already gave none. The hit names the cast's target, not the
  druid: a proc that puts a dot on the hit's target (Shard of the Gods) panicked on a friendly one (review).
- Mechanic immunities (fixed): `proto.Target.mechanic_immune_mask` (field 21), unset meaning row -361
  (`core.RaidBossMechanicImmunities`) on a world boss and nothing elsewhere; `Unit.ImmuneToMechanic`. The
  Naxxramas presets carry row -286. Pin, Venom Web Spray, Ravage, Tendon Rip and Snatch cast and miss on an
  immune target. The mage's snares read the mask, and Torment the Weak counts any aura tagged
  `core.SnareAuraTag` (another mage's chill, Mind Flay). Hamstring and Slow have no sim spell.
- Curse of Doom (fixed): `APLValueWarlockCurseOfDoomLength` (apl.proto field 66), 60 s without the tweak;
  `demo.apl.json` compares the time left to it. Demonology Average-Default 15057.9 → 15059.2, Curse of Doom
  7.00 → 7.02 casts; the short single-target rows +2.3 and +1.8%.
- Destruction's Immolate tick (no gap): the log's five snapshots (1315, 2109, 1329, 1329, 1983) are the sim's to
  the point in the same aura states; six of ten non-crit ticks came from the two under procs.
- Spell-mod sweep (fixed, `spellModSimSpells`): the 17 probes compare as the sim spell whose `ThreatSpellID` they
  are. 6 agree (53000 under Twin Disciplines ×1.0479 and Searing Light ×1.0987, through a declared tick; Inner
  Focus on both ticks; 7386's cost under Puncture and Focused Rage), 11 move nothing. Replay: 544 agree, 391 move
  nothing, 245 not compared.

Tests: `omen_items_test.go`, `mechanic_immunity_test.go`, `mind_flay_snare_test.go`,
`TestTormentTheWeakReadsImmunitiesAndOtherUnitsSnares`, `TestPetSpecialsNeverLandOnAnImmuneTarget`,
`TestCurseOfDoomLengthValue`, `TestSpellModSimSpellsResolve`. e2e: `TestSimvalEcClass3Fight`
(`e2e/ecclass3_test.go`: auras on the bot or target, items used as steps), vetted only.

Goldens on top: 35 of 37 suites, most by one to seven item rows within 0.02%. The mage suites and
BalancePhase3 wear Reign of the Unliving, so every row moves (Average-Default −0.006 to −0.018%, key
`{ItemID: 47188}` → `{SpellID: 67760}`); feral rows with a damage trinket −0.7 to +1.0% (Omen). TestBalance's
six `p3_alliance` rows wear the trinket too (one iteration each, to −1.34%). Subtlety's Reign of the Dead rows
−0.37% on 20 iterations; over 10000 it's +0.15%: at the suite's 30 yd the bolt's crit lands 1.5 s on, past Honor
Among Thieves' 1 s cooldown, and rolls it again (51701 has `CAN_PROC_FROM_PROCS`, finish phase; 90.2 → 93.1
points a fight). `db.json` moves
(the Naxxramas presets, `demo.apl.json`); `spells_auto_gen.go` regenerated.

Follow-ups: Frost Shock's snare tag (shaman); the bombs and sapper under their spells; the simval dummies' own
masks; Barkskin, Innervate and Rebirth never roll Omen; Typhoon's and Hurricane's snares; exempting
`spell_warl_curse_of_doom` in the module's shaped scripts.

Live stage, 2026-10-11 against `3d89793c7` with module `8a6f13f`, no rebuild (runs, logs and counts in
`tmp/ec-class-fix3-live.md`). `TestSimvalEcClass3Fight` ran after three e2e fixes: the dummy selected again after
the item commands, the equip waited for in the saved inventory, and item steps counted from the Chronicle log
(an item's SMSG_SPELL_GO names the item as caster).
- Blood Presence (confirmed): on the Heroic Training Dummy with Improved Blood Presence 2/2, 220 heals, each
  int(int(damage × 0.04) × 1.10); 228 more under Hellscream's Warsong, no ×1.30. One heal per Saronite Bomb and
  Gnomish Lightning Generator hit; the sapper heals for its hit on the dummy and for its hit on the DK. None for a
  critter or the `NO_XP` simval dummy.
- Death rune take-over (confirmed): twice with no miss, Blood Tap, two Blood Strikes, then four Icy Touches, the
  last two 22.0 and 23.5 s after the tap, all went off. With one Blood Strike dodged the tapped rune was never
  taken over and the fourth Icy Touch was refused.
- Chimera Shot - Serpent (the melee fix undone): under Curse of the Elements a 603 sting gave 1907 =
  uint32(int32(603 × 2.8) × 1.13), not 2154. 53353 has `SPELL_ATTR0_CU_NO_POSITIVE_TAKEN_BONUS`
  (`spell_custom_attr`), so its own hit takes a damage taken multiplier only below 1: it's now
  `SpellFlagIgnoreModifiers`. In Aspect of the Viper it kept the Dragonhawk amount, and the sting's refresh there
  lost only the 300 AP (confirmed).
- Hunting Party (confirmed): 19 Replenishment procs, each on a crit of 53352, none at a cast; 19 of 53 crits at
  1/3.
- Omen of Clarity (confirmed): 69 Clearcasting procs on 518 landed Wraths, each on its hit: 13.3% against 11.67%
  (Wrath's cast time entry is 2 s, not the plan's 8.75%). 16886 rolls nothing: none at the launch of 171 crit
  Wraths, where it goes on.
- Nature's Grace (new, fixed, `rollNaturesGrace`): spell_proc -16880 has the cast and the hit phase, so a crit
  Wrath gives 16886 at its launch and again at its hit (171 of 171 each); the sim gave it at the hit only. A
  cast spell without a missile rolls both phases at its hit, a triggered one (Starfall's stars) the hit phase
  only (`Spell::_cast`; review); it matters under 3 points.
- Ignite (found, not fixed): all 76 ticks are the amount the server's rule gives. Over the fight Ignite paid 1.19
  of 40% of the crits feeding it, the sim's bot in the same fight 1.56 (sd 0.38 over 60 seeds), 110 ticks to 76.
  After an application the sim's first tick comes exactly 1.9 s later, ahead of an application due 2.0 s on; live
  it comes 1.85 to 2.05 s later and that application won 14 times in 91. Core timing.
- Mace Specialization (confirmed): WarriorArms with a two-handed mace (40406) moves armor penetration +15 on
  both sides. The committed axe path's drop stays.
- Spell-mod sweep: MageFire's 192 lines are fresh, Pyroblast unshaped: 546 agree, 389 move nothing, 245 not
  compared. The fixture README's Pyroblast note is gone.
- Fights: DeathknightBlood and WarriorProtectionWarrior re-recorded. Bloodworm melee mean 61.51 against 61.71
  (z −0.1), warrior melee 402.2 against 402.3 (z 0.0).

Tests: `TestChimeraShotSerpentTakesTheTargetsBonusOnce`, `TestChimeraShotSerpentTakesAReductionTwice`,
`TestNaturesGraceComesAtWrathsLaunch`; from the review, golden-neutral: `TestNaturesGraceRollsOncePerPhase`,
`TestHurricaneCastProcsGoOnItsTarget`.

Goldens on top: TestBalance +1.32% and BalancePhase3 +1.09% Average-Default (all rows, −4.0 to +3.8%; Nature's
Grace uptime 247 → 263 s, Wrath 124.6 → 126.7 casts); TestMM back to −0.02% against the base (53353 4036 → 4022 a
hit), where the melee stage had +0.69%.

Follow-ups: Ignite's first tick after a delayed application (`sim/core/dot.go`, `delayed_periodic.go`); the
WarriorArms sweep path with a mace (`sweepClasses` in `talentpaths.go`, then its talent and spell-mod lines), which
would retire its drop; a hit phase of its own for other missile spells in a cast-and-hit proc's mask; the Shard
of the Gods sets by the server's scripts (69739 casts only the heal, 69755 only Searing Flames 69729; the sim
gives both sets both).

Cross-review (Z7):
- Explosives (fixed): Saronite Bomb, Cobalt Frag Bomb and the sapper go by 56350, 67769 and 56488
  (`item_template.spellid_1`), with declared ranges, the bombs' flight (14 and 25 yd/s) and their own 60 s. They had
  hit cap and 2× crits; by the server's class rules the two magic ones miss and crit as spells, Saronite Bomb (no
  damage class) always lands and never crits (`WorldObject::SpellHitResult`, `Unit::SpellDoneCritChance`), and the
  sapper never misses its caster. `legacyAPLItemIDs` keeps a rotation naming the item, the generator's 41121 too;
  the four preset APLs name the spells. Cobalt's 0.5 s cast is an allowance. Goldens: TestFeralTank Average-Default
  dps −0.34%, tps −0.36%, dtps −0.78% (133 rows, dps −2.7 to +1.8%, dtps to −10.0%; Saronite 7786 → 6163 and the
  sapper 5494 → 4800 a fight); TestRetribution dps −0.04% (175 rows, −0.23 to +0.05%), dtps −11.6% (the sapper's
  hit on the paladin crits for 1.5×).
- Frost Shock (fixed): an 8 s `SnareAuraTag` aura where the target takes snares. Hamstring and Slow have no sim
  spell.
- `Unit.HealingTakenPctMultiplier` replaces the death knight's division by 1.30.
- Chimera Shot - Serpent agrees with `spell_hun_chimera_shot` and `SpellDamageBonusTaken` step by step, but for the
  flat taken part (Gift of Arthas), which the sim leaves out.
- Mechanics agree: `FlagBleed` (Savage Rend, the pet's Rake) and the abilities' `Mechanic` (Pin and Venom Web Spray
  7, Ravage 12, Tendon Rip 11, Snatch 3) match the dump, and -361 has no bleed bit.
- An unset `mechanic_immune_mask` is right on every preset: Hodir, Algalon, Anub'arak, Gormok 25H, Sindragosa and
  the Lich King carry -361 (`CreatureImmunitiesId`, difficulty entries included), as do 144 level 83 bosses, the
  generic boss's stand-in. Gormok's 10 and 25 normal carry -355, without disarm.
- Ignite (cause found, not fixed): the sim agrees with `spell_mage_ignite`, `CastDelayedSpellWithPeriodicAmount` and
  `AuraEffect::Update`. An application due 2.0 or 4.0 s after the last races the old Ignite's tick. On the sim's
  exact 100 ms map update the tick is always first (1.9 to 2.0 s on); on the live 102.7 ms it comes 1.95 to 2.05 s
  on and the application wins about half. The live fight is phase-locked (4 s is ten 400 ms windows, Pyroblast's dot
  ticks every 1.96 s), so its 1.19 is one draw of what the sim's seeds spread from 1.00 to 2.57. On the committed
  MageFire fight a 102.7 ms update (build overlay) takes Ignite from 23.5 to 22.8 ticks for the log's 22, −1.3% of
  its damage. It needs the real update period in the server settings.
- Module: `NO_AMOUNT_SCRIPTS` (was `TICK_PERIOD_SCRIPTS`) lists `spell_warl_curse_of_doom`, which only acts in
  `AfterEffectRemove`. Syntax-checked, not built: after a rebuild, rerun WarlockAffliction's spell-mod sweep and
  drop the fixture README's note.

Follow-ups: Death Strike's 45470 and Death Pact's 48743 have no damage class either; the class-none and own-caster
hit rules belong in core's outcomes; `db.json` has no icon rows for the three explosives' spells.

### EC-TOOLS (Z7, done; golden-neutral; stages: code, live)

- The rotation debug harness from Z5's retune (`G:\DevStuff\GitHub\.wave-loop\ec-class-fix2-apl-tmp\apldbg`:
  per-rotation rows, cast counts and aura uptimes at chosen iterations, against an overlaid base) moves under
  `tools/` with a README.
- A threat fixture cutter in `tools/simval`: a recorded stream to `threat.jsonl`, by `threatRecord.key` and
  `droppedThreatRecords` (each wave wrote a scratch script).
- e2e: a fight spot with line of sight to the dummy's centre (18 yd north hides it from Starfall); a path fails
  early, naming the cause, when its bot leaves map 533 (the weekly raid reset).
- proto: a `BonusDefenseSkill` pseudo stat, so the sheet stops reading enchant 3847 off the gear, with a UI test
  of the sheet's defense skill and block chance.
- **Owns:** the new tool's directory; `tools/simval/threat*.go`'s cutter; the module's `e2e/fight_test.go` and
  `spellmod_sweep_test.go`; `proto/common.proto`'s pseudo stat and its plumbing;
  `ui/core/components/character_stats.tsx` and its test.
- **Goldens:** none.

#### As built

Stage code (offline; live plan in `tmp/ec-tools-code.md`). No golden moves.
- `tools/apldbg` ([README](../../tools/apldbg/README.md)): a preset is its suite's `CharacterSuiteConfig`, so its
  rows are the suite's own requests (`FullCharacterTestSuiteGenerator`) and match the golden to the digit, held by
  `TestPresetsRunTheirSuitesRows`; the four `sim/deathknight/dps` suites, or any request file. `-apl`, `-gear`,
  `-row`, `-iters`, `-seed`, `-ids`, `-log`, `-timeline`; `base.sh` builds it over a commit's sim code too.
- `simval threat` (`tools/simval/threat_fixture.go`, the simval README's Threat probe): cutting the committed
  `threat.jsonl` gives it back byte for byte. `threatKey`, `key()` and `droppedThreatRecords` moved there from
  `threat_test.go`, with their own class constants (the test file's aren't in the binary); `main.go` got the dispatch.
- Fight spot: the entrance, where the dummies stand, is the foot of stairs rising 4.5 yards to the south, 2.7 yards
  from the portal's wall (the hub's navmesh, `mmaps/5332638.mmtile`). A bot south of the dummy at the dummy's height
  is inside the stairs, and 18 yards north is behind the wall, inside the hollow central block: in sight of the
  dummy's hit sphere, not its centre. `fightBuild` now moves the bot to the hub's outer floor (3046 -3458 298.2,
  level 18 yards north and south, hostile spawns 68 yards off) before the build spawns the dummy, so every fight
  through it stands there; ranged stays `standOff`'s 18 yards north. `fightSight` holds a ranged spot to both lines
  of sight with two casts that are always out of range (module README).
- Spell-mod sweep: `spellModRun` checks the bot's map before each command and on a failure, and names the reset.
- `PseudoStatBonusDefenseSkill` (6): `GetPseudoStatsProto` sends `PseudoStats.BonusDefenseSkill`; the sheet adds
  each row's part to the defense skill and the total to block chance, and no longer knows enchant 3847. It now shows
  what the sim applies: the rune on both weapons or on an item-swap weapon counts each time. UI tests in
  `tests/gear/character_stats.spec.ts`: the rune on a death knight tank, and a bonus written into a protection
  warrior's `computeStats` answer for block chance (no blocking class has one).
- Pre-checked for the live stage: DruidBalance's cycle with `"before": [Starfall 53201]` passes `TestFightCycles`,
  the bot's own fight and the committed log's replay (`tmp/cycles-starfall.json`).

Review: `apldbg` refuses a broken `-timeline`, `-match` or `-apl` file before any sim runs; the cutter gives the
last scratch script's output on a real X2 stream (950 lines, 119 kept); the sight probes are real ones (Kick 1766
and Stormwind Banner 62736: 5 yards, neither ignores line of sight, and `Spell::CheckCast` tests it before range).

Stage live (2026-10-11, one run, `tmp/ec-tools-live.md`): `TestSimvalFight` for DruidBalance from the new floor,
its cycle now `"before": [Starfall 53201]`.
- `FIGHTSPOT`: both casts answered `SPELL_FAILED_OUT_OF_RANGE`, so `fightSight`'s commands reply to a GM-off bot; its
  `LINE_OF_SIGHT` branch stays unproven.
- Starfall went off once and 10 stars (53195) hit the dummy (sim 10); two more killed a Maggot and a Spider in the
  first two seconds; no splash (53190) landed.
- Replay: 67 of 70 checks, the three failures all where the aura sits: the server's 53201 is on the druid, the sim's
  a dot on the target beside a second dot timing the splash. `fightSimAurasOn[53201] = "self"` and
  `fightSimAurasLeftOut[{53190}]` (`tools/simval/fight.go`) give 67 of 67, the aura's uptime compared (9.989 s
  against 9.951 s). The trimmed log replaced `DruidBalance.log.gz` (old copy: `tmp/DruidBalance.committed.log.gz`);
  DruidBalance had no left-out entry or drop row to remove.

Follow-ups: with three or more units in sight Starfall's two stars a second go to two random ones, so a critter can
take the dummy's star (a re-record fails the count: clear the critters first); the sim's multi-target Starfall is 20
one-second ticks on the main target, the server's 10 seconds of two stars each; Nature's Grace applications log 22,
sim 12 (4 to 21), passing only on the widened range;
the other tests' dummies still stand at the entrance (`newBot`, `spawnInFront`), where a short race has
no line of sight from 4 yards south (the threat and Parfu10 notes on Undead and gnomes) and `strikes_test.go` moves
past the dummy; `testing.md`'s e2e gotchas and the module README's `TestSimvalEcClass2Fight` remarks on line of
sight describe that spot; critters within 30 yards of the fight floor can take a star or an area spell.

Cross-review (Z7): the four suites' configs are `sim/deathknight/dps/suites.go`'s `BloodSuite(root)` and its three
siblings, which the tests and `tools/apldbg/presets.go` share. The copies and `TestPresetsRunTheirSuitesRows` went
(`TestEveryPresetRunsItsDefaultRow` reads no golden), `base.sh` keeps `suites.go` in a base build, and
`tools/simval/goldchar.go` follows a config a package function returns. No golden moves. The module README's
`TestSimvalEcClass2Fight` paragraph describes the outer floor.

### EC-CORE-FIX5 (Z8, done; changes goldens, full suite)

- Update order: a hardcast landing, a missile hit or Auto Shot on the world tick a player's aura expires finds it
  gone. `Unit::Update` runs `m_Events` (landings, hits), then `_UpdateSpells` (Auto Shot, ticks, then expiries),
  and melee follows in `Player::Update`. Reorder landings, Auto Shot and held swings around the tracker pass
  (`Simulation.advance`, `releaseCastHold`); every caster's goldens reshuffle.
- The live map update runs about 102.7 ms (the host's loop time), the sim's 100: make it a server setting. It
  decides Ignite's race (an application due 2.0 s after the last beat the old tick in 14 of 21 such races live,
  one due 4.0 s after in 5 of 9, never in the sim; −1.3% of Ignite on MageFire's replay, another recording with
  3 races), a creature's 2 s swing (20 updates, 2055 ms) and a prepull opener's 4th Fire Blast. A creature's
  swing after its hardcast lands comes one update early (the cast lands in the event pass, ahead of the attack
  timer: 1968 ms for 2057).
- A periodic action has no owner unit: an aura its tick puts on (Prey on the Weak) counts a world tick early, and
  an aura paired with one needs `DurationThroughLastTick`. Execute phase callbacks fire with the first clock stop
  past their time. A channel's tick still follows a creature aura's expiry on a shared update.
- Nothing expires auras on a pet that's away, and heals land on dismissed pets (Circle of Healing, Prayer of
  Healing).
- Whole steps left: white swings (`urand(uint32(min), uint32(max))`; `attack.go` rolls a float, about half a
  point high), the rune weapon's strikes and swing, Sweeping Strikes' copy, Manifest Anger, Steady Shot's weapon
  part, Eviscerate's and Envenom's AP share, the AoE cap inside Fan of Knives' and Swipe's base, Multi-Shot
  rolling its weapon once, `HealingPower`'s flat healing taken ahead of the coefficient, the flat taken part
  (Gift of Arthas) in Chimera Shot - Serpent.
- Meta gems: a crit multiplier class code changes mid-fight (Combustion, Hodir's Storm Power, Prey on the Weak's
  table, Predatory Instincts) misses a swap; `sim/common/tbc/metagems.go`'s effects stay the starting gear's.
- Leads: `spell_sha_t10_elemental_4p_bonus` only raises Flame Shock's max duration, so `GetTotalTicks` would cap
  the added 6 s; `arcane_p3` as optimizer target beside `fury_p1` shows a −0.018% calibration gap.
- `tools/simval/fight.go`'s per-update event is no longer needed.
- **Owns:** `sim/core/` but EC-PET-FIX4's pet paths; `proto/common.proto`'s server setting and its plumbing; those
  inline lines in class code; their drops and rows.
- **Goldens:** all 37; report each.

#### As built

Timing stage, on `c244f8ee9` against [ac] `3d89793c7`, offline (notes `tmp/ec-core-fix5-timing.md`). Verdicts:
- Update order (fixed): a player's cast landing and missile hit (`ActionPriorityEvent`, `PendingAction.playerEvent`)
  go ahead of its aura pass on a tick they tie, then its Auto Shot; a melee swing a cast held or `resumeMelee` frees
  goes after the pass (`WeaponAttack.releasing`). A player's hardcast always queues its own landing now, apart from
  the GCD action. Creatures' landings keep their place.
- Map update period (fixed): `ServerSettings.world_loop_ms` (field 6), the host's real tick, never under
  `world_update_interval_ms`. Default 10.27 ms, a constant in `gen_server_defaults`: ten ticks are the 102.7 ms of
  the creature swings (20 updates, 2055 ms) and Ignite's ticks, and it comes from the host's sleep granularity, not
  a config key. A map update stays ten world ticks, so it still falls on a tick. UI: "World loop time".
- Periodic action owner (fixed): `PeriodicActionOptions.Owner` and `FirstTickAt` make it one of the unit's periodic
  ticks (own update, remainder carried, ahead of the player's expiries, auras it puts on count from it). Prey on the
  Weak uses it and loses `upUntil`.
- Execute phases (fixed for time-based ones): `Step` stops at their time. Damage-based ones still start at the first
  clock stop after the damage.
- Channel tick (fixed): a player's channel or self hot ticks ahead of the creatures' expiries on a shared update.
- Pet that's away (heals fixed, `pet.go` not touched): Circle of Healing and Prayer of Healing skip disabled units.
- Found: with a non-10 ms tick, `Dot.UpdateExpires` callers that add a whole 2 or 3 s to a snapped expiry left it
  off the grid and ahead of the last tick (Rip lost about a tick a cast); Rip, Moonfire, Scourge Strike and the
  Pestilence refresh snap with `NextServerTick`, as Rupture does.
- `tools/simval/fight.go`'s per-update event stays: it samples refreshes, metrics-only damage and silent misses once
  an update (taking it out fails checks). Its comment no longer says auras run out on it.
- Drop: PaladinRetribution `energize 57669 mana amount` (log 1128, sim 1087; 1123 on 10 ms ticks). The log stamps an
  application about 47 ms after its first tick's clock and the bot refreshes 8.976 s after the last stamp, so the
  sim's refresh lands before the ninth tick; 9 s is a whole number of 10 ms ticks, which hid it. Needs the casts read
  off the log's tick stamps.

Tests: `update_order_test.go` (+4), Arcane Blast buff, Mirror Image, Fire Elemental (swings 40.1 to 38.65 a life),
Frost Shock snare and Nature's Grace moved with the period. `simval.jsonl` 1114/1114.

Goldens (`dock.sh delta`), all 37 move. The order change alone moves 24, Average-Default within ±0.075% but TestHoly
hps −5.3% (heals on pets that are away: Circle of Healing hits 75.8 to 50.5). With the period:
- TestFire −3.7% and TestFrostFire −3.3%: Ignite's damage −13.8% and −11.9% (ticks −22%, each 10% larger), 90 and
  95% of the drop. An application due 2.0 s after the last beats the old tick 52% of the time (never before), which
  drops the double count of the tick-first order (`procIgnite` reads the old tick as owed, then it pays; the server
  does the same). Cross-review, on the live fight the count came from (Pyroblast under Bloodlust, not the committed
  `MageFire.log.gz`, which holds 3 races): the application won 14 of 21 such races live and 54.7% in the sim's replay
  of it (z +0.95). One due 4.0 s after wins 5 of 9 live and 4% in the sim: on a fixed 102.7 ms update the old
  Ignite's last tick always comes first, and the live update drifts (103.1 ms in that fight). Under one such race a
  fight in the goldens. The fire goldens move 0.15% for each 0.01 ms of `world_loop_ms`.
- Boss swings −2.7%: BloodTank dtps −2.6%, FeralTank −5.3%, Protection (paladin) −6.2%, ProtectionWarrior −2.8%.
  Cross-review, at 400,000 to 1,000,000 iterations against `world_loop_ms` 0 (the reorder moves none of the four):
  swings −2.5 to −2.6% everywhere. FeralTank is −5.6%: the rest is Savage Defense, whose one charge the druid's
  crits refill at their own rate, so a longer gap leaves fewer swings without one (13.65 to 13.11%). The paladin is
  −2.4%: it only takes 1.1 hits a fight, and the golden's 2000 iterations scatter by 2.8%. The warrior is −2.75%
  (Shield Block's windows −0.26%).
- TestUnholy −0.48%, TestFury −0.69%, TestBM −0.92%, TestDisc −0.77%: reshuffles and pet swings, not traced further.
- The other suites within ±0.4%. TestArcane's Ephemeral Snowflake row (−31.7%) and TestDisc's Regalia of Faith row
  (−29.6%) were a stalled rotation, no golden move: on the 10.27 ms tick a GCD could end between a hasted channel's
  last tick and its aura's end, and nothing woke the unit again. The channel dot's `OnExpire` hands off now
  (`32ca3596c`). After that fix the rows 5% or more off the base are TestHoly's (above), the fire mages'
  Bloodmage's rows and one short FrostFire row (Ignite), a Balance multi-target row and the Frost Witch row (below),
  and the tanks' Average and 20-iteration SwitchInFrontOfTarget rows.

Amounts stage (notes `tmp/ec-core-fix5-amounts.md`), on top of the timing stage. Verdicts:
- White swings: whole already (`Spell.StrikeDamage`). The rune weapon's strikes and swing roll whole now
  (`Weapon.WholeDamage`; its multipliers still follow the roll).
- Manifest Anger (fixed): 50% of a whole weapon roll (`ApplyPct` of `MHStrikeDamage`/`OHStrikeDamage`), not a float
  roll under a halved `DamageMultiplier`.
- Steady Shot (fixed): `irand(int32(min), int32(max))` of the ranged weapon plus `int32(ammo dps × speed ms ×
  0.001f)`.
- Eviscerate, Envenom (fixed): the AP share is `int32(ap × combo × 0.07f)` in float32, Envenom's
  `int32(ap × 0.09f × combo)` (that order since the cross-review: the other can be a point off).
- AoE cap (fixed for Swipe and Fan of Knives): `Spell.AoECapUnits`; past 10 units the hit is cut to `damage × 10 /
  units` in ints, after the done and taken percents, before armor and crit. No effect up to 10 targets.
- Multi-Shot: already one weapon roll for all hits. Not changed.
- Sweeping Strikes (fixed): the Execute copy is a whole normalized roll; every other copy is the hit's damage before
  armor (`SpellResult.ArmorReduced` is the server's cleanDamage); Whirlwind copies only its first hit (0.5 s lockout)
  and copies that hit, not a fresh roll.
- Chimera Shot - Serpent (fixed): flat damage taken (Gift of Arthas) at the sting's dot coefficient joins the tick
  before the taken percents (`Spell.PeriodicFlatDamageTaken`).
- `HealingPower` (confirmed, not fixed): the server adds flat healing taken as its own int after the caster's
  multiplier; the sim folds it into spell power ahead of the coefficient. Only `BonusHealingTaken` gear (Talisman of
  Troll Divinity) is off, by the caster's done percent on that share; the fix is a hook after the done percent
  (11 priest lines, `shaman/talents.go`).
- Meta gems (fixed): `Character.ScaleCrit` keeps the factor of an effect that scales a multiplier in place (Combustion,
  Storm Power) across a swap; `MetaCritFollow`/`MetaCritBuilt` carry values worked out ahead of time (Predatory
  Instincts, Prey on the Weak). `sim/common/tbc/metagems.go`'s six effects go on and off with the swap. No preset
  swaps a gem, so no golden moves from it.
- `spell_sha_t10_elemental_4p_bonus` (fixed): 6 s on the time left, the max duration (which caps the ticks) raised only
  when the new time passes it (`Dot.AddRemaining`). The old extra ticks and the haste reschedule went. Flame Shock's
  max duration is hasted like its interval (`SPELL_ATTR5_SPELL_HASTE_AFFECTS_PERIODIC`), so the raised one counts in
  hasted intervals: 7 to 8 ticks an application, not 6 (cross-review). A Frost Witch row of TestElemental −4.1%,
  nothing else.
- A creature's swing after its hardcast lands (fixed in cross-review): `Unit::Update` runs the attack timers down
  after its event pass, so the restarted timer loses that update. Live the Fire Elemental swung 1957 ms after a landed
  Nova (34 swings) and 2058 ms after a swing or Blast: 19 and 20 updates in the sim now. A player's restart loses
  its 10 ms world tick by the same lines: unmeasured, not changed.
- `arcane_p3` beside `fury_p1`: not fixed. The gap is `sim/optimizer`'s derived context against the real raid, no
  server mechanic, and that package is BIS-TANK-MAGIC's.
- `tools/simval/fight.go` per-update event: kept, as the timing stage found.

Goldens move little on top of the timing stage (still 37 of 37, Average-Default within 0.01%): TestBlood,
TestBloodTank, TestBM, TestMM, TestSV rows ±0.01% (rune weapon, Steady Shot), TestCombat rows to 0.3% (Fan of Knives
past 10 targets), TestArms rows to 0.75% (Sweeping Strikes on Whirlwind), TestElemental's Frost Witch row −5.0%
(−4.1% since the cross-review's tick count).

### EC-PET-FIX4 (Z8, done; changes goldens; stages: code, live)

- Replenishment: summons with a full mana pool the sim gives none (treants, Shadowfiend, Infernal) can take one
  of a cast's ten places.
- Mirror Images: summoned at exactly 0 s they count as out before the fight (`engagedAtSummon`); the server rolls
  each image's opener, the sim once for all three; a cast landing on the update the image dies on is lost.
- Fire Elemental: Fire Nova keeps `IgnoreHaste`; whether and when one summoned with nothing selected joins the
  fight.
- A permanent pet dismissed and summoned again in one fight gets its inherited stats twice. The Bloodworm's
  health takes 1 a point of stamina (sim 10).
- Disc's Greatness row and Restoration shaman's Healing Grace row are put down to the mana heuristics by pattern
  only: A/B them.
- Live: more Fire Elemental lives (`SIMVAL_PETFIX_LIVES`; Blasts 17.5 a life for the sim's 18.29, z −1.7); why one
  Mirror Image in three casts nothing at the e2e spot.
- **Owns:** `sim/core/pet.go`, `focus.go`, the pets' timers and `replenishment.go`'s target rules; the pets'
  files in class code; `pets.jsonl` and the module's `e2e/petfix_test.go`; their drops and rows.
- **Goldens:** the pet suites, and the mana users under Replenishment; report each.

#### As built

Code stage, re-measured on `c244f8ee9` against [ac] `3d89793c7`. Verdicts:
- Replenishment (fixed): treants, Shadowfiend and Infernal have a pool that stays full (`pet_levelstats` mana, the
  scaling int, `Creature::Regenerate` refilling a controlled creature), so a cast ranks them last at 100%
  (`Pet.FullManaPool`, `Raid.replenishmentFullPool`). They displace only units tied at 100% further down the raid.
  No golden moves.
- Fire Nova (fixed): magic class, so `Unit::ModSpellCastTime` scales it by cast speed; `IgnoreHaste` went. Wrath of
  Air is a raid area aura (effect 65), which reaches a guardian, so the Greater Fire Elemental takes it
  (`Pet.TakesWrathOfAir`) and casts Nova in 1904 ms. Other casting pets (Imp, Water Elemental, Mirror Image) would
  take it too; the sim still drops it for them.
- Nothing selected (not a gap): `_initAttack` looks for the owner's selection on the first update only. After that a
  guardian joins through `OwnerAttacked`, called by `Spell::_cast` for each harmful unit-target spell of the owner as
  it goes off. The sim always has a target selected.
- Mirror Images, three leads, none changed:
  - Opener per image: each image's marginal is the sim's, so the mean is the same; only the correlation of the three
    differs. Splitting them means three casters and 3x the cast counts.
  - A summon at exactly 0 s: images engage when the owner is in combat (his first harmful cast going off), which the
    sim has no notion of. Every preset summons at -2 s.
  - A cast landing on the image's death update (a gap, not fixed): a cast's `SpellEvent` is queued for the last
    update's time plus 1 ms, ahead of `DeathEvent`'s 29.5 s (`EventProcessor::Update`), so it goes off, and the AI
    doesn't run on that update: the live 2 s lead shows it (9 Frostbolts, 2 Fire Blasts). The sim's timeout runs first
    and loses the cast. Landing it (the death at `ActionPriorityLow`) moves Frost +0.4%: there a Fire Blast opener's last
    Frostbolt at the presets' lead is such a tie. Whether its missile hits before the corpse goes at 30 s is unmeasured,
    so it stays. Cross-review, on `integration`'s `world_loop_ms`: the same change moves no mage golden and no line of
    the replay, so no cast is due on that update at the presets' lead any more. The 9th Frostbolt the server has after
    a Frostbolt opener would end 0.43 s after the sim's image dies: nine casts of 3081 ms against the live 3048.
- Permanent pet summoned again (fixed): `Pet.Disable` takes the inherited stats off for every pet, not just guardians
  and timed ones.
- Bloodworm health (fixed): 437 create health, 1 a point of stamina (`SetCreateHealth(437, 25, 1)`). Nothing reads it.
- A/B, 100,000 iterations (400,000 for the shaman), same seed:
  - Disc Greatness row, `205e071b3^` against `205e071b3`: TPS 890.78 to 889.89 (se 0.05), HPS unchanged. A real move of
    -0.1%, matching the golden's; the heuristics only add a tick's mana. Against this base TPS is 874.25, from later work.
  - Restoration shaman Healing Grace row, `205e071b3^` against this base: TPS 2862.78 to 2863.34, HPS 8866.67 to 8868.43
    (se 0.07, 0.17). A real +0.02%, the golden's direction.
- Lead: the Rune Weapon (SummonProperties 208) and the Valkyr (2909) have neither a pet's type nor category, so they are
  `Uncontrollable` on the server and take no Bloodlust; the sim gives it to both.

Goldens: only Elemental (Convection Average +0.02%, Nova 6.65 to 6.80 a fight) and Enhancement (Average +0.10%) move,
with the Fire Elemental's Nova landing 96 ms sooner. The rest of the 37 are unchanged; `simval.jsonl` 1114 of 1114.

e2e: `TestPetfixFireElementalJoinsOnTheOwnersCast`, `TestPetfixFireElementalUnderWrathOfAir`; the packet watch reads
a spell start's cast time.

Stage live, on [ac] `3d89793c7` (notes: `tmp/ec-pet-fix4-live.md`). The sim is unchanged and no golden moves past
stage code's.
- Wrath of Air (confirmed): with 2895 on the shaman (`.aura`, the area aura's owner as his totem would be), his
  elemental's Fire Nova started with a 1904 ms cast; so did both Novas with the aura put on the elemental itself.
- Nothing selected (confirmed): the elemental did nothing for 12 s, then swung 13 ms after the shaman's bolt went
  off. Rank 1 Lightning Bolt is interrupted as a level 80's cast bar fills (cause not found); the test casts rank 12.
- Fire Elemental, 12 lives (confirmed): 10.0 Novas started, 6.25 landed, 3.75 broken, 18.17 Blasts, 39.25 swings a
  life, for the sim's 9.8, 6.58, 3.06, 18.27 and 40.16: z -0.7, -0.3 and -1.4. With Z7's 6 lives Blasts are 17.94.
  All 206 Blast gaps are 4.06 to 8.08 s. On `integration` the replay swings 38.87 times a life (cross-review:
  `world_loop_ms`, and the swing an update sooner after a landed Nova).
- Silent image (a spot artifact): at the entrance the mage's spot is inside the stairs, and one or two of the three
  images drop to the ground at z 150, 144 yd below; a Frostbolt at their victim gets `SPELL_FAILED_BAD_TARGETS`. On
  the hub's outer floor (`fightGround`) all 45 images of 15 summons cast. The test moved there and fails a summon with
  fewer than three casters, logging each image's `.gps` and the refusal (`PETFIX_IMAGE_PROBE`).
- Mirror Images (confirmed, but for the last casts): into a fight 18 of 18 images land 9 Frostbolts and 3 Fire Blasts,
  3 s ahead of the pull 8 and 2, or 3 after a Fire Blast opener, as the sim. 2 s ahead, the presets' lead, the server
  has 8 and 3 after a Fire Blast opener (6 images) and 9 and 2 after a Frostbolt (3, the 9th landing on the update
  the image dies on); this base's 100 ms grid gives 9 and 4, and 9 and 3. Those casts ride on the update period:
  EC-CORE-FIX5's `world_loop_ms`, already on `integration`, gives 8 and 3, and 8 and 2 (cross-review).
- Opener (confirmed, unchanged): 8 of 9 summons ahead of the pull mix both openers, so the server rolls per image;
  16 of 27 opened on a Fire Blast.
- Fixture: `pets_casts.jsonl`, the 12 lives and 15 summons, replayed by `TestPetsCastsRecords`
  (`tools/simval/pets_casts.go`, 100 checks); a cast within 0.5 s of an image's death may be missing on either side.
  Its drift allowance is 1.5% since the cross-review, 3% before: on `world_loop_ms` only the sim's fixed update
  against the live jitter is left (0.5% fails 6 checks).
- e2e: `TestPetfixMirrorImageCasts` gains three summons 2 s ahead of the pull (9 summons, about 6 min).

Follow-ups:
- A 3 s cast takes 3048 ms live on average (3.00 to 3.10: the first update past 3 s of a loop that jitters), 3081 on
  a fixed 102.7 ms grid, which loses the Frostbolt opener's 9th Frostbolt at a 2 s lead.
- An image's corpse is unsummoned at 30 s (`TempSummon::Update`), which cancels a Frostbolt still in flight: one
  cast as it dies only hits within 12 yd, which decides the tie above. The sim flies it 5 yd (the pet's
  `DistanceFromTarget` is 0); `CasterAI` keeps an image within 20 yd. Unmeasured: the e2e reads `SMSG_SPELL_GO`, not
  the damage log.
- `TEMPSUMMON_TIMED_DESPAWN` runs the creature's whole update before it unsummons, so another timed guardian's cast
  or AI event due on its last update may still happen; `EnableWithTimeout` ends it first. Unchecked per guardian.
- Wrath of Air for the Imp, Water Elemental and Mirror Image, and the Rune Weapon's and Valkyr's Bloodlust (above).
- The Fire Elemental tests and `TestPetfixRecords` still stand at the entrance.
- Why a level 80's rank 1 Lightning Bolt is interrupted.

### EC-CLASS-FIX4 (Z8, done; changes goldens; stages: code, live)

- Druid: multi-target Starfall is 20 one-second ticks on the main target, the server's 10 s of two stars a tick
  on random units in sight; the Balance fight logs 22 Nature's Grace applications for the sim's 12 and 2
  Clearcastings for 5.4; Barkskin, Innervate and Rebirth roll Omen; other missile spells in a cast-and-hit proc's
  mask may need Nature's Grace's launch roll.
- Items: heals and Bryntroll under their server spells; Shard of the Gods' sets by the server's script table (the
  Purified set's 69755 casts only Cauterizing Heal 69733, the Shiny set's 69739 only Searing Flames 69729:
  ItemSet.dbc rows 881 and 882; the cpp comments swap the two; the sim gives both sets both procs).
- Death knight: `blood_pesti_aoe` tuning; a `no_xp` Target field, set on the simval dummies, for Blood Presence's
  heal; Death Strike's 45470 and Death Pact's 48743 may need `HealingTakenPctMultiplier` too.
- Snares for Torment the Weak: Hamstring and Slow have no sim spell; Typhoon's and Hurricane's.
- Hunter pets: Ravage, Tendon Rip, Snatch and Venom Web Spray on an immune boss are from the source only (a live
  cast of each); the simval dummies' own `creature_immunities` masks in `tools/simval/damage.go` and `fight.go`.
- The WarriorArms sweep path with a mace (`tools/simval/talentpaths.go`), retiring its drop; its live records are
  in `G:\DevStuff\GitHub\.wave-loop\ec-class-fix3-tmp`.
- Live, once the server runs mod-sim-validation `bcc58cf` (a rebuild by the user): the spell-mod sweep's
  WarlockAffliction path again (`SIMVAL_SPELLMOD_ONLY=WarlockAffliction`), dropping the fixture README's note on
  Curse of Doom.
- **Owns:** `sim/{deathknight,druid,mage,hunter,warrior}/` but the other Z8 items' hunks; the item effects in
  `sim/common/`; `ui/deathknight/apls/`; `tools/simval/spellmods*.go` and `talentpaths.go`;
  `proto/common.proto`'s target field and its plumbing; their cycles, fights, drops and rows.
- **Goldens:** those classes' suites, and every suite wearing a changed item; report each.

#### As built

Druid and items stage (notes in `tmp/ec-class-fix4-druid.md`). Verdicts:
- Starfall (fixed): 10 ticks, each casting 2 stars on distinct random units (the one unit when alone), every star
  casting its splash 53190 at launch; the splash dot is gone. The Balance fight's Nature's Grace (log 22, sim 15.8, range
  4 to 28) and Clearcasting (2 vs 5.4, range 0 to 13) are chance, not changed.
- Omen on Barkskin, Innervate, Rebirth (fixed): `CheckProc` passes them, so each hit rolls.
- Nature's Grace launch roll: only Wrath and Typhoon are missiles in the mask; Typhoon's triggered 53227 rolls at its hit
  only (added).
- Shard of the Gods (fixed): Purified (69755) casts only Cauterizing Heal 69733, Shiny (69739) only Searing Flames
  69729 (spell_script_names; the cpp comments swap the two; ItemSet.dbc rows 881 and 882 carry 69755 and 69739), 25%,
  45 s, off any landed spell damage or heal; no heroic ids. A damage hit uses up the Purified proc and heals nothing,
  and a heal the Shiny one (cross-review): the spell fails on the wrong kind of target (`SPELL_FAILED_BAD_TARGETS`).
- Bryntroll (fixed): Drain Life 71838/71839, crit capable; the leech heal to the player isn't modeled.
- Item heals under server spells (not changed): nothing reads them, as Omen rolls on `OnSpellHitDealt` only and no
  restoration heal rolls it.
- Goldens: TestBlood, TestBloodTank, TestArms (Bryntroll rows +0.2 to +0.5%), TestBalance, TestBalancePhase3
  (multi-target rows -4.3 to +1.3%), TestFeralTank (Barkskin's Omen roll shifts the random stream, Average dps +0.07%).

Rest stage (notes in `tmp/ec-class-fix4-code.md`). Verdicts:
- Death knight `NO_XP` (fixed): `proto.Target.no_xp` (field 22) reaches `Unit.NoXP`; `givesXPOrHonor` reads it, and the
  simval dummies set it, so Blood Presence's heal never fires on them (`Player::isHonorOrXPTarget`).
- Death Strike's 45470 and Death Pact's 48743 (fixed, `gainHealingTakenOnly`): no damage class and no
  `spell_bonus_data` row, so `SpellHealingBonusDone` returns the amount; only the healing taken percents apply, not
  Strength of Wrynn's stand-in.
- `blood_pesti_aoe` (tuned): Blood Tap needs the next blood rune 6 s or more from ready. Average-Default 9231 -> 9338
  (+1.15%, se 4); full-buff multi-target +3.7%, unbuffed rows -0.1 to -2.7%.
- Torment the Weak's snares (partly fixed): Typhoon's 53227 snares a landed target for 6 s (`SnareAuraTag`), Hurricane
  tags every target for the channel (`SlowAttackAuraTag`, counts on a boss too); the tag slows no swing
  (cross-review). Hamstring and Slow stay unmodelled: no sim spell casts them.
- Dummy immunities (fixed): the simval sims give 999000 row -361 and 999001 to 999008 row -286 (`dummyMechanicImmunities`);
  the boss dummy's matched the default already.
- WarriorArms with a mace (partly): the setup takes 40406, which moved the axe talents between the warrior paths
  (WarriorFury2 is new), so the 146 live mace records fit no path. WarriorArms, Fury, Fury2 and Protection leave both
  sweeps' replay (`sweepStaleSteps`, `spellModRerecord`) until the live stage reran them (below).
- Pet specials on an immune target: `TestEcClass4PetSpecialsOnImmuneTarget` (live stage, below).
- Goldens: TestBlood (12 `blood_pesti_aoe` rows), no other moves from this stage.

Live stage (notes in `tmp/ec-class-fix4-live.md`; server 3d89793c7, module bc1d35e). Verdicts:
- Pet specials on an immune target (confirmed, no sim change): Ravage, Tendon Rip, Snatch and Venom Web Spray at the
  boss dummy go off and report immune with no hit or tick, as the sim has them. The bat control was refused
  (`DONT_REPORT`) and is only logged.
- Curse of Doom (fixed by the rebuild): WarlockAffliction rerun, `shaped` is empty, the dot compares and agrees. Its
  fixture README note is gone.
- WarriorArms mace path (fixed): talent and spell-mod sweeps rerun for Arms, Fury, Fury2 and Protection. Mace
  Specialization +15 armor penetration on both sides; `sweepStaleSteps`, `spellModRerecord`, the Arms
  `maceSpecialization` drop and its README row are gone.
- Balance fight (re-recorded, no sim change): Nature's Grace 11 applications (sim 15.8, range 4 to 28), Clearcasting 5
  (sim 5.4), Starfall 9 stars; 66 checks, none dropped. The old 22 vs 12 and 2 vs 5.4 were chance.
- Not live-checked: Omen on Barkskin, Innervate and Rebirth, Typhoon's Nature's Grace, Shard of the Gods, Bryntroll's
  crit and heal, a multi-target Starfall; the source stands.
- Goldens: none from this stage.

### EC-CORE-FIX6 (Z9, done; changes goldens, full suite; stages: code, live)

- The map update's period: the sim's is a fixed ten world ticks of `world_loop_ms`, 102.7 ms; live it jitters from
  update to update and drifts (103.14 ms over the logged Fire fight, a read that took every slime cast for 20
  updates: counted, 103.97; 102.2 to 104.1 over 21 s stretches). Model it: a jitter per update (a period per
  recording gives a 2 s timer one length, and a quarter of the slimes' fire an update early). The sim already
  rolls an update phase per iteration. Three things ride on it:
  - Ignite's races, a new application against the old Ignite's tick on one update. Due 4.0 s after the last, the
    application wins 5 of 9 live and 4.1% in the sim (z +9.2): a fixed update always puts the last tick on the
    39th, 4005 ms, just ahead. Under one such race a fight, about 0.15% of Fire. Due 2.0 s after, it wins 14 of 21
    live (67%, 95% interval 45 to 83%), 54.7% in the sim's replay of that fight (z +0.95) and 52% of the goldens'
    27 races a fight. `spell_mage_ignite`, `AuraMunchingQueue` and `AuraEffect::Update` agree with the sim step
    for step, the double count included, so only the period is open.
  - The fire goldens' level: 0.15% for each 0.01 ms of `world_loop_ms` (TestFire 12047 at 10.20, 11693 at 10.40),
    −0.47% and −0.64% at that log's 10.314, about −1.2% at its two rates.
  - A creature's 3 s cast: 3048 ms live on average (3.00 to 3.10 s over 584 Frostbolts in `pets_casts.jsonl`, the
    first update past 3 s), always 3081 in the sim. That costs a Mirror Image its 9th Frostbolt after a Frostbolt
    opener at the presets' 2 s lead (server 9 and 2, the 9th at 29.49 to 29.53 s; sim 8 and 2; Z9's records have
    8 and 2 on both sides, the ninth riding on the recording's period) and keeps `petsCastsDrift` at 1.5% (0.5%
    fails 6 checks).

  The log is `G:\DevStuff\GitHub\.wave-loop\ec-class-fix3-tmp\live\fights\MageFire_Svfleadizmkp_1791726042.log`
  (71 Pyroblasts under Bloodlust; the committed `MageFire.log.gz` is another recording, with 3 races). The counting
  is in `G:\DevStuff\GitHub\wowsimwotlk-int\tmp\xr-ignite`: `py/races.py` defines both races, `py/live_period.py`
  reads the period off the Embalming Slimes' casts (20 updates apart), `src/` holds the replay and golden tests.
- Ignite off an instant cast (a Hot Streak Pyroblast's hit, Living Bomb's explosion) takes
  `DelayedPeriodicApplier.ApplyFromInstantCast`'s delay, drawn off the clock of the tick before; on the server
  both see the current clock, as a hardcast's landing does (`Apply`); Fire Blast, an instant with no missile,
  does keep the stale one. From the source only; 1.1 applications a fight in TestFire.
- Order on a shared update, each from the source only:
  - A player's swing timer, restarted as its hardcast lands, loses that world tick: `Unit::Update` runs the
    attack timers down after its event pass. `cast.go` does it for creatures, a map update back. Measurable
    only from a recording with a hardcast at the target between swings.
  - A creature swings ahead of the ticks of the auras on it: `Step` runs a weapon attack before a pending action
    due with it, the server ticks the creature's auras before its AI swings (`Creature::Update`). The boss beats
    a Lacerate tick 0.83 times a fight, Savage Defense's uncovered swings −0.04%.
  - A creature's own landing against an aura expiring on it, and a player's swing against an aura expiring on a
    creature, still follow the whole aura pass: EC-CORE-FIX5 moved a player's landings, Auto Shot and held swings
    only. By `Unit::Update` the landing comes first, and players update ahead of creatures.
- Damage-based execute phases (health-based fights, `StartTargetAtHealth`'s replays) start at the first clock
  stop after the damage that crosses them (`runExecutePhases`); time-based ones are exact since EC-CORE-FIX5.
  Not in the damage path: the server sets the under 35% and under 20% states in the target's `Unit::Update`, so
  those start on its first map update from the crossing damage on; only 25% (Drain Soul) starts with its damage.
- Expiries off the update grid: on a world tick that isn't 10 ms, an `UpdateExpires` caller adding whole seconds
  to a snapped expiry leaves it ahead of the last tick (Rip lost about a tick a cast). Rip, Moonfire, Scourge
  Strike, Pestilence and Rupture snap with `NextServerTick`, which ends them an update late half the time (the
  server adds to what the duration has left); Bone Shield's `now + 5 min` and every other `Aura.UpdateExpires`
  caller are unchecked.
- Whole steps left:
  - `Spell.HealingPower` adds `BonusHealingTaken` to spell power ahead of the coefficient; `SpellHealingBonusTaken`
    adds `int32(TakenAdvertisedBenefit × coeff × factorMod)` as its own term after the caster's multiplier. Off by
    the done percent on that share, about 0.2% of a heal under Talisman of Troll Divinity: a hook after the done
    percent, 11 priest call sites and `shaman/talents.go`.
  - The AoE cap: `Spell::DoAllEffectOnLaunchTarget` cuts a player's area hit to `damage × 10 / targets` in ints
    past 10 targets, after the done and taken percents, before armor and crit. Swipe and Fan of Knives do
    (`Spell.AoECapUnits`); every other `AOECapMultiplier` caller still scales its base as a float.
- The fight replay's PaladinRetribution `energize 57669 mana amount` drop (log 1128, sim 1087; 1123 on 10 ms
  ticks): the log stamps an application about 47 ms after its first tick's clock and the bot refreshes 8.976 s
  after the last stamp, so the sim's refresh lands before the ninth tick. Replay the casts at the log's tick
  stamps; the drop (`tools/simval/fight.go`) and its fights README row go.
- EC-CORE-FIX5's Average moves nobody traced: TestBM −0.92% (wolf swings −2.5%), TestDisc hps −0.77% (aura
  52800's uptime −18.8%), TestFury −0.69% (Slam −5.2%, Whirlwind −3.8%: both noise), TestUnholy −0.48% (aura
  48265's uptime +24.7%, 48266's −7.3%). A/B each against `world_loop_ms` 0 at depth, as the cross-review did the tanks.
- Live: a Fire fight several times the logged one's length (its cycle is `cycles-ecclass3.json`, beside the
  scripts), with its period read off the recording: 21 and 9 races leave the 2 s rate at 45 to 83% and the fire
  goldens' level at ±0.75%. Note the host's load, which moves the period.
- **Owns:** `sim/core/` but EC-PET-FIX5's pet paths and BIS-TANK-PICKS' hunks (the enemy's dodge multiplier, a
  unit's per-iteration death values); `proto/common.proto`'s server setting and its plumbing; those
  inline lines in class code (`mage/ignite.go`, the AoE and heal call sites); `tools/simval/fight.go`'s cast
  replay and `pets_casts.go`'s drift; their drops and rows.
- **Goldens:** all 37 (the period); report each.

#### As built

Period stage, on `b4e8f6d52` against [ac] `3d89793c7`, offline (notes `tmp/ec-core-fix6-period.md`). Verdicts:
- Map update period (fixed, a jitter per update): `ServerSettings.map_update_jitter_ms` (field 7), the standard
  deviation of one update's length; an update is its configured 100 ms plus a lognormal excess, ten even world
  ticks, drawn per iteration (`sim/core/update_grid.go`). Defaults 10.46 ms a tick and 6 ms, fitted to 10,264
  spacings of the Naxxramas slimes' 2 s cast timer in 124 raw fight logs: 2064 ms on average, a quarter on the 19th
  update. A fixed period gives that timer one length, and so does a period per recording; the 103.14 read off the
  Fire log assumed 20 updates every time. Sim: a 2 s timer 2064.3 ms, a 3 s cast 3044.3 ms (3.00 to 3.14 s).
  Outside the fight the grid keeps the mean; `world_loop_ms` 0 or jitter 0 gives a fixed grid.
- Ignite's races (fixed): the logged Fire fight replayed over 2000 seeds has the application win 62.7% at 2 s (live
  14 of 21) and 54.8% at 4 s (live 5 of 9, z +0.03; was 4.1%).
- A creature's 3 s cast (fixed); `petsCastsDrift` 0.5%. A Mirror Image's 9th Frostbolt now ends within an update
  of its death (19 of 978 summons land it): EC-PET-FIX5's order on the death update decides the rest.
- Ignite off an instant cast (fixed): `SpellResult.DealtWithItsCast`. A missile's hit and Living Bomb's explosion
  read the current clock; Fire Blast keeps the stale one.
- PaladinRetribution energize (fixed, drop and README row gone): `castClocks` moves a cast that put a ticking aura
  on the player to the aura's clock, the earliest tick stamp less its periods. Log 1128, sim 1110 (1068 to 1128).
  The log's rule: the 9th tick lands when the next cast's clock is 9000 ms on.
- EC-CORE-FIX5's untraced moves (all hold at 200,000 iterations against `world_loop_ms` 0): TestBM −0.89% (wolf
  swings −2.4%), TestDisc −0.79% (Power Word: Shield 20.33 to 20.00 casts, Borrowed Time 103.5 to 84.1 s), TestFury
  −0.74% (Deep Wounds −4% a tick, Overpower −5.4%; Slam and Whirlwind were noise), TestUnholy −0.46% (Gargoyle casts
  −3.0%). On the jittered grid: −1.09%, +0.29%, −0.93%, −0.15%.

Built besides: `Simulation.ServerTickBefore` and `WorldTickBefore` (with jitter the update before isn't one
interval back; `serverTickInterval` then holds the shortest update), `core.OnRegularGrid` for tests that count
updates, a depth rerun of a suite's Average row (`SUITE_DEPTH_*`, `sim/core/test_suite.go`).

Goldens (`dock.sh delta`), all 37 move, period and Ignite fix only:
- TestFire −0.58% and TestFrostFire −0.69%: Ignite −2.8% and −3.0%. The period alone is −0.65% and −0.79%, the
  instant-cast fix +0.07% and +0.10%.
- Tank dtps +0.6 to +0.9% (paladin +2.0%, 1.1 hits a fight): the boss's 2.4 s swing took 24 fixed updates,
  2464.8 ms, and now the 23rd or 24th, about 2449 ms.
- TestDisc hps +1.08% (above); every other Average row within ±0.31%. 20-iteration rows reshuffle by up to 2.8%.

Open: `pet.go` and `optimizer_tank_boss.go` still step by `serverTickInterval` (other items' files); the jittered
grid costs Elemental about 14% of its run time (measured loaded); the world tick's own length isn't measured.

Order-amounts stage, on the period stage's tree, offline (notes `tmp/ec-core-fix6-order.md`). Verdicts:
- A player's swing after its hardcast lands (fixed): the restart counts from the world tick before
  (`sim.updateBefore`), as an instant the session casts does; one cast from the unit's own update keeps its tick.
- A creature's swing against the ticks on it (fixed): `WeaponAttack.swing` runs its due ticks first.
- A creature's landing against an aura running out on it (fixed): `auraTracker.advance` lands its due hardcast
  first (`Unit.landDueCast`). Its missiles in flight keep their queue order.
- A player's swing against a creature's aura running out (fixed): `Step` runs the players' auras, their melee, then
  the creatures' auras, and the players' attacks lead `sim.weaponAttacks`. In 88 raw fight logs the swing came
  first in 13 of 13 shared ticks.
- Damage-based execute phases (fixed on the target's update): the server sets the under 35% and under 20% aura
  states only in `Unit::Update`, so those phases start on the target's first map update from the crossing damage
  on, as a queued action. 25% starts with its damage: only Drain Soul reads it, off the health.
- Expiries off the grid (fixed): Bone Shield's `UpdateExpires` is gone. `Aura.AddDuration` and `Dot.AddDuration`
  add to what the duration has left (`Aura.expiresNominal`), where the snapped callers ended an update late half
  the time; Rip, Moonfire, Scourge Strike, Pestilence, Rupture and `Dot.AddRemaining` use them.
- `Spell.HealingPower` (fixed): the caster's alone. The heal path adds `int32(taken × coefficient)` after the
  caster's percent, per target and tick: the declared coefficient where the sim's spell is the server's heal, else
  `SpellConfig.HealingTakenCoefficient` (Prayer of Mending, Penance, Earth Shield, the two priest glyph heals).
  Power Word: Shield's absorb takes none.
- The AoE cap (fixed): every caller sets `Spell.AoECapUnits`; a tick the server casts (Volley) is cut too.

Goldens, this stage alone: Average rows within 0.03% but TestFeral +0.19% and TestFeralApl +0.16% (Rip back up an
update sooner), TestBalancePhase3 +0.08%, TestFeralTank dtps −0.16%, TestProtection dtps +1.0% (noise). The
20-target rows move under 0.11% with the cap; 20-iteration rows reshuffle by up to 1.7%.

Open: the taken coefficient takes the target's spell mods on the server, the sim the caster's; Empowered Renew's
heal (63544) gets no taken term; `Encounter.AOECapMultiplier` has no caller left.

Live stage, 2026-10-12 against [ac] `3d89793c7` (notes `tmp/ec-core-fix6-live.md`). No sim change, no golden move:
- Five more 300 s Fire fights, six with last wave's (1813 s). The application wins 108 of 174 two-second races
  (62.1%, sim 62.7%) and 41 of 84 four-second ones (48.8%, sim 54.8%, z −1.1). Ignite ticks 82.3 a fight (sim
  81.4) but pays 1.256 of 40% of the crits against 1.175 (z +2.2): open, about +0.6% of TestFire if it holds.
- The period is the recording's: counted off the creatures' cast stamps it's 103.75 to 104.53 ms a fight, and
  104.36 on average over the 125 earlier logs (sd 0.75; a test run on the host adds about a ms). The four-second
  race reads it, (39 p − 4000) / p: 2 of 16 in one fight, 11 of 14 in another.
- The grid strays 4.66 ms an update: the share of the slimes' timers on the 19th update follows each log's own
  period (2.9% at 103.0 ms, 24.7% at 104.5, 74.0% at 106.0). The period stage's fit, floored at 100 ms, reads high.
- Defaults kept at 104.6 and 6 ms: at 104.4 and 4.7 the sim's race rates leave the live ones (66.5%, 56.9%). The
  6 covers the scatter of the players' ticks, which the even-tick model lacks. Over the recordings' range the fire
  goldens sit within −0.4% to +0.45% of the default's (50,000 iterations).
- New, not fixed: world ticks aren't even. `TestEcCore6WorldTick` (`.server info`): median 10 ms, mean 13 to 14,
  one tick in eight 31 to 60 ms. `MapMgr::Update` updates the open world, battlegrounds and instances on three
  ticks in a row and the first runs long under the bots, so an instance updates one tick after the long one. 1321
  recorded player swings land 10.8 ms past the attack time (sim 5.2), 21% over 12 ms. Needs a setting for the long
  tick; with it the period and jitter go to 104.4 and 4.7.
- Not measured: the swing after a hardcast (the recording's Flash of Light at itself was refused), a boss swing
  against a tick, the execute phase (the dummies zero every hit).

Review, round 1: no code change; the delta is the live stage's, byte for byte.
- Each new order, execute-phase and `AddDuration` test fails with its fix switched off.
- Tank dtps at 100,000 iterations, the old fixed 102.7 ms grid against the default: TestProtection +0.69% (4.474 to
  4.505, ±0.013 each), so the rest of its +3.0% golden row is noise; TestFeralTank +1.36% (42.19 to 42.76, ±0.04).

**Cross-review (Z9).** `pet.go`'s clocks follow the varying grid: `updateTimersStart` and `AIClockStart` are
`ServerTickBefore(NextServerTick(x))`, `UpdateAfter` is `NextServerTick(NextServerTick(t) + 1)` (`TestPetClocksFollowUpdatesOfVaryingLength`).
Treants' timed unsummon moves 16 ms earlier in TestBalance's uptime rows. Dead code gone: `Encounter.AOECapMultiplier` with
`AOECapMultiplierFor`, `SpellResult.FromInstantCast`, `SlowAttackAuraTag`. `arcane_blast_buff_test.go` allows the longest
world tick (17.9 ms). The grid's cost stays open: lookups are about 10% of an Elemental run (`stretch` 8%, drawing 2.4%), so
drawing the updates in one loop recovers little.

### EC-PET-FIX5 (Z9, done; changes goldens; stages: code, live)

- Wrath of Air: the totem's raid area aura reaches pets and guardians (live, the Greater Fire Elemental's Fire
  Nova in 1904 ms with 2895 on the shaman), all but a haste carrier, immune by its script. `applyPetBuffEffects`
  drops it for every pet without `Pet.TakesWrathOfAir`, which only the elemental sets; the Imp, Water Elemental
  and Mirror Images cast hasted spells too.
- Bloodlust: the Rune Weapon (SummonProperties 208) and Nibelung's Valkyr (2909) have neither a pet's type nor
  category, so by EC-PET-FIX3's rule they are uncontrollable and no party or raid cast reaches them. The sim
  gives both Bloodlust: neither sets `Pet.Uncontrollable`. A lead from the two rows; confirm it first. (It holds,
  but the Rune Weapon's is no gap: it swings only as a copy of each of the DK's white hits, hasted with him.)
- Nothing expires auras on a pet that's away: TestHoly's Shadowfiend, never summoned, carried Inspiration for
  298 s until EC-CORE-FIX5 kept Circle of Healing and Prayer of Healing off such a pet. Other casts still land.
- A timed guardian's last update:
  - A Mirror Image's cast due on the update it dies on goes off on the server: a cast's `SpellEvent` is queued
    for the last update's time plus 1 ms, ahead of `DeathEvent`'s 29.5 s (`EventProcessor::Update`); live, the 9th
    Frostbolt of 3 of 3 images after a Frostbolt opener at a 2 s lead (at Z8's 3055 ms a cast; of none of Z9's
    18, at 3081: the recording's period decides). The sim's timeout runs first. The death
    at `ActionPriorityLow` fixes the order and moves no golden or replay line today, since the sim's 9th Frostbolt
    would end 0.43 s late (EC-CORE-FIX6's period): do it if the damage lands (Live).
  - The corpse is unsummoned at 30 s (`TempSummon::Update`), which cancels a Frostbolt in flight: one cast on the
    death update only hits within 12 yd. The sim flies it 5 yd (the pet's `DistanceFromTarget` is 0); an image
    casts from where it's summoned, within 4 yd of the mage (`CasterAI` moves it only between casts, toward 15 yd).
  - `TEMPSUMMON_TIMED_DESPAWN` runs the creature's whole update before it unsummons, so a cast or AI event due on
    any timed guardian's last update may still happen; `Pet.EnableWithTimeout` ends it first. Check each.
- Mirror Image leads EC-PET-FIX4 left as they were:
  - One opener roll for all three; the server rolls per image (8 of 9 summons mixed, 16 of 27 images on Fire
    Blast). Each image's mean is the sim's, so only the spread differs; splitting them takes three casters.
  - Summoned at exactly 0 s they count as out before the fight (`engagedAtSummon`); the server engages them at
    the owner's first harmful cast. Every preset summons at −2 s. (Right as it is: a raid boss puts the mage in
    combat on its first update, after his cast at the pull; only the e2e's dummy waits for his cast.)
- Live:
  - Whether that Frostbolt's damage lands: `SMSG_SPELLNONMELEEDAMAGELOG` in `TestPetfixMirrorImageCasts`, which
    reads `SMSG_SPELL_GO` only, 2 s ahead of the pull after a Frostbolt opener, with the image's distance.
  - Cast times under 2895 on the owner for the Imp, the Water Elemental and a Mirror Image, as
    `TestPetfixFireElementalUnderWrathOfAir` reads Fire Nova's; the Rune Weapon's swings and the Valkyr's casts
    under Bloodlust.
  - The Fire Elemental tests and `TestPetfixRecords` still stand at the entrance, inside the stairs (the spot
    that silenced the images): move them to `fightGround` and re-record with more lives. At 12 the replay's mean
    checks are weak (5 standard errors is 3.2 swings; 39.25 swings a life for the sim's 38.87). 12 lives take
    24 min: run them in the foreground, since a background run ends with the agent's turn.
  - Why a level 80's rank 1 Lightning Bolt (403) is interrupted as its cast bar fills: `CheckCast` passes and no
    `SMSG_SPELL_GO` follows; rank 12 goes off.
- **Owns:** `sim/core/pet.go`, `buffs.go`'s pet rules and the pets' timers; the pets' files in class code and
  `sim/common/wotlk/nibelung.go`; `pets.jsonl`, `pets_casts.jsonl` but its drift, and the module's
  `e2e/petfix_test.go`; their drops and rows.
- **Goldens:** the mage and warlock suites (Wrath of Air), the death knight suites and Nibelung's rows
  (Bloodlust; no golden summons a Val'kyr, so none of those moves); report each.

#### As built

Stage code, re-measured on `b4e8f6d52` against [ac] `3d89793c7` and the live DBCs (notes and the live plan:
`tmp/ec-pet-fix5-code.md`). Verdicts:
- Wrath of Air (fixed): every pet and guardian takes it but a haste carrier, whose script makes it immune to
  positive `MOD_CASTING_SPEED_NOT_STACK` (`spell_dk_pet_scaling`, mod-spell-tweaks' 425790 and 425792).
  `Pet.TakesWrathOfAir` went. Firebolt and Waterbolt 2500 to 2380 ms, an image's Frostbolt 3000 to 2857.
- Bloodlust:
  - Val'kyr (fixed): SummonProperties 2909 is category 1, type 2, so `Uncontrollable`. Found on re-measure: its AI
    casts Smite on the creature's 2 s attack timer (`DoSpellAttackIfReady`), not a 1.95 s GCD: one on its first
    update, then every 20 updates, 15 in 30 s for the sim's 16. A life is `NibelungAverageCasts` × 2 s.
  - Rune Weapon (not a gap, unchanged): 208 is category 1, type 6, so the aura never lands. But
    `npc_pet_dk_dancing_rune_weapon` is a `NullCreatureAI` that never swings: `spell_dk_dancing_rune_weapon` copies
    each white swing the DK lands, so the copies come as fast as he swings, Bloodlust included. The sim's pet
    Bloodlust stands in for that.
- A pet that's away (fixed in part): a summon starts without what was cast on it while away (`Pet.enable`).
  Casts still land on an away pet: over every `./sim/...` test, a warrior's Commanding Shout, a priest's Prayer of
  Mending, Inspiration, Divine Aegis and Forethought Talisman, the hunter's Bestial Wrath, Cobra Strikes and
  Windrunner proc, the warlock's Devious Minds. Refusing them takes a gate in `Aura.Activate`, EC-CORE-FIX6's file.
- A timed guardian's last update (fixed): `EnableWithTimeout` unsummons on the pet's own update, the first one a
  duration past the update before its first, behind everything the pet does on it (`petUnsummonPriority`). Before,
  15% of 12 s summons had one update fewer. It covers every timed guardian; the gargoyle's 32 s is its AI's timer,
  which runs out the same way.
- Mirror Image's last casts (fixed, from the source):
  - Its death is its own action at `ActionPriorityLow`: behind a cast landing on that update, ahead of its AI.
  - The corpse goes at the first update 30 s in, and a Frostbolt whose flight ends later is lost
    (`Pet.MissileLost`; a timed Water Elemental's Waterbolt too).
  - It flies the mage's distance: an image casts from where it's summoned, and `CasterAI` only moves it between
    casts, toward 15 yd (three quarters of Fire Blast's 20), not 20.
- Mirror Image leads, both unchanged:
  - Per-image opener: `CasterAI::JustEngagedWith` rolls per creature. The mean is the sim's.
  - A summon at exactly 0 s: on a raid boss the mage is in combat from the boss's first update
    (`BossAI::_JustEngagedWith` calls `DoZoneInCombat`), after his cast at the pull, so the images do come ahead of
    the fight. Only the e2e's dummy waits for his own cast.

Tests: `sim/core/pet_summon_test.go`, `TestMageGuardiansCastFasterUnderWrathOfAir`,
`TestMirrorImageFrostboltInTheAirAtTheUnsummonIsLost`, `TestMirrorImageCastDueOnItsDeathUpdateGoesOff`,
`TestImpFireboltUnderWrathOfAir`, `TestValkyrSmitesOnItsAttackTimer`; each fails on the old code. Replays pass
(`simval.jsonl` 1114 of 1114); no drop or allowance entry was this item's.

e2e (written and vetted, run in stage live): `TestPetfixMirrorImageCasts` reads the damage logs, miss lists and
each image's distance; `TestPetfixCastTimesUnderWrathOfAir`, `TestPetfixRuneWeaponUnderBloodlust`,
`TestPetfixValkyrSmites`.

Goldens (`dock.sh delta`), 16 of 37 move, Average-Default:
- Mages: Frost +0.39% (Waterbolt casts 125.3 to 131.0; image Frostbolt hits 20.58 to 19.60 of 21.5 casts, the
  goldens' mage standing 30 yd out), Fire +0.10%, Arcane −0.02%, FrostFire −0.06%; rows to +1.6%.
- Destruction +0.19% (Firebolt casts 128.1 to 130.0, the Imp's mana holding it back); Affliction's Infernal
  uptime only; Demonology unchanged.
- The last update: Blood +0.05% (Rune Weapon swings 36.26 to 36.72), BloodTank +0.004%, Frost, FrostUH and
  Unholy within ±0.02% (army, ghoul, bloodworm), Balance and BalancePhase3 +0.02% (treants), Enhancement −0.005%
  (rows to −1.0%), 17 Elemental rows to +0.7%, Shadow's fiend uptime only.
- No Nibelung row: the goldens never summon a Val'kyr.

Follow-ups:
- The Rune Weapon's white hits as copies: each landed DK swing, either hand, rolled again on the DK's own table
  and halved (`HandleProc`), with no procs. The sim swings a 3.5 s weapon on its own timer, and buffs on the pet
  count twice (Battle Shout, the 10% attack power auras).
- `Aura.Activate` refusing a temporary aura on a pet that's away.
- An image more than 20 yd plus reach out can't Fire Blast (`CheckRange`), and never closes in while it chains
  Frostbolts. The goldens' mage stands at 30.
- The tooltip of `nibelungAverageCasts` says max 16; it's 15.

Stage live, on [ac] `3d89793c7` (logs and notes: `tmp/live/`, `tmp/ec-pet-fix5-live.md`). No sim change came of it
but `Pet.UnsummonAt` for the replay; the records back the code stage's model:
- Wrath of Air: Firebolt and Waterbolt 2500 to 2380 ms, an image's Frostbolt 3000 to 2857, Fire Nova 1904.
- Val'kyr: 15 Smites in its 30 s, the first as it comes, 2055 ms apart, the same under the priest's Bloodlust, no
  2825 on it.
- Rune Weapon: no 2825. It swung with 10 of the knight's 10 landed swings, on the same update, and never
  otherwise, plain and under Bloodlust: the copies follow-up stands, now with a record.
- Mirror Image, 24 summons:
  - Images stay where they come up, within 4 yd of the mage (2 to 8 yd for a mage at 4, 11 to 19 at 16), and a
    Frostbolt flies the image's own distance less about 1.5 yd.
  - The lost missile: from 16 yd, the ninth Frostbolts whose flight ended by 29975 ms after the summon hit (12)
    and those from 30006 ms on never did (5). The sim's corpse goes at 29988.
  - A ninth Frostbolt 2 s ahead of the pull needs the live updates to run fast: none of 18 images got one at
    3081 ms a cast, where Z8's three did at 3055. EC-CORE-FIX6's period decides it.
  - Past 20 yd is unmeasured: from 28 yd the mage's own Fire Blast doesn't reach the dummy on that floor.
- Fire Elemental, 12 lives on `fightGround`: 6.83 Novas landed, 17.83 Blasts and 38.83 swings a life for the sim's
  6.51, 18.30 and 38.89 (z +0.8, −1.4, −0.1).
- Rank 1 Lightning Bolt: mod-individual-progression cancels the vanilla and TBC ranks a level 70 player past TBC
  casts, as the cast time is up (`OnPlayerSpellCast`; `IndividualProgression.EnableAllSpellRanks = 0`).

Fixtures re-cut: `pets.jsonl` (21 records, 180 checks) and `pets_casts.jsonl` (12 lives, 24 summons). The replay
gained `image N Frostbolt hits` (`checkPetsCastsHits`): each Frostbolt against the lost-missile rule at the
image's own distance, either outcome within one update of the corpse going; three mutants fail it.

e2e: every petfix test stands on `fightGround`. New: `TestPetfixFarMirrorImageCasts`,
`TestPetfixMirrorImageLastFrostbolt`, `TestPetfixLightningBoltRanks`. Found on the way: `.cast <id> triggered`
skips a spell's effects (`TRIGGERED_IGNORE_EFFECTS`), so it summons nothing; a fresh Imp's Firebolt is off
autocast; Dancing Rune Weapon needs a weapon in hand.

Review, both in `Pet.EnableWithTimeout`:
- A summon that comes again while still out cancels the old timer. Bloodworms' 20 s proc cooldown is their life,
  and the unsummon can now come up to 26 ms past it: a proc in between lost its worms to the stale timer
  (TestBlood's Bloodworm swings went down; now 274.02 to 274.05).
- On exact timing (`MapUpdateInterval` 0) the unsummon keeps its old priority, ahead of a cast due at that very
  moment: a Val'kyr on 11 casts got 12.
- Tests: `TestSummonedAgainWhileStillOutLastsItsNewTime`, `TestTimedSummonOnExactTimingEndsAheadOfACastDueThen`,
  `TestValkyrSmitesOnExactTiming`.
- Lead, from the source only: the Val'kyr's attack timer is hasted (`resetAttackTimer`), so under Windfury Totem
  it would Smite every 17 updates, 18 in 30 s. The sim's 2 s is fixed; it only matters past 15 casts.

**Cross-review (Z9).** `Aura.Activate` refuses a temporary aura on a pet that isn't out, and the summon's `expireAll`
is gone. Callers that assumed the aura went on guard it: Cobra Strikes, and Prayer of Mending's bounce, which skips units
not in the world (it lost the chain on an away Shadowfiend). `TestSummonStartsWithoutWhatWasCastOnItAway` checks the
refusal; `TestValkyrSmitesOnUpdatesOfVaryingLength` covers the Val'kyr on the live grid (15 Smites, each on an update).
The three `OnRegularGrid` count tests still test their counts; the Mirror Image's casts on the varying grid are covered
by its replay. Replay: a Frostbolt opener at a 2 s lead is 8 and 2 in sim and server alike; at 1.8 s the ninth cast goes
either way at 29.3 to 29.5 s. The Val'kyr tooltip says 15.

### EC-CLASS-FIX5 (Z9, done; changes goldens; stages: code, live)

- Druid:
  - Omen of Clarity and Nature's Grace on heals. Omen rolls on any landed magic-class damage or heal
    (EC-CLASS-FIX3: 3.5 PPM over the cast time entry, floored at 1.5 s); the sim rolls on `OnSpellHitDealt` only,
    so no restoration heal rolls it and no item heal (Bauble of True Blood 71607, Althor's Abacus 71610 and
    71641, Glowing Twilight Scale 75493 and 75494, which then go under those spells; the Scale's two only tick
    and have no damage class, so they never roll). Healing Touch, Regrowth, Nourish and Swiftmend are in
    Nature's Grace's mask without `SpellFlagNaturesGrace` (the sim registers none of the four).
  - Insect Swarm and Faerie Fire carry `SpellFlagOmenTrigger`; whether a hit with no damage rolls Omen on the
    server is unconfirmed (the source wasn't read through for them; it does).
  - Hurricane's `SlowAttackAuraTag` aura only tags its targets for Torment the Weak. 16914's second effect slows
    the attacks of every creature in the storm (`MECHANIC_SLOW_ATTACK`, which no immunity row blocks), a boss's
    swings too. Ranks 1 to 4 only: rank 5's (48467) slow has no mechanic, so it gives Torment the Weak only
    where its snare lands, and on a snare-immune target the slow is on half the time.
- Bryntroll's Drain Life (71838, 71839) heals its wielder; the sim deals the damage only.
- Torment the Weak: Hamstring and Slow have no sim spell, so neither tags a snare (`SnareAuraTag`, as Frost
  Shock's does); each is a new spell.
- `blood_pesti_aoe`: Blood Tap waiting until the next blood rune is 6 s or more from ready gains 1.15% on
  Average-Default and 3.7% on the full-buff multi-target rows, and costs the unbuffed rows 0.1 to 2.7% (on Z9's
  base the gains are 0.05% and 0.1 to 0.3%, the cost 0.7 to 3.0%). Find a condition that keeps both
  (`tools/apldbg`).
- Live, each from the source only so far:
  - Omen rolling on a Barkskin, Innervate or Rebirth cast (8.75% a cast).
  - A Typhoon crit giving Nature's Grace 16886 at the hit, once per target.
  - Shard of the Gods: the Purified set casting only 69733 and the Shiny only 69729, 45 s apart (50: the
    server's SQL updates both rows), off damage and heals, a hit of the wrong kind using the proc up
    (`SPELL_FAILED_BAD_TARGETS`).
  - Bryntroll's 71838 critting (it can't: `SPELL_ATTR2_CANT_CRIT`), and the size of its heal.
  - A multi-target Starfall: 20 stars over 10 s on two distinct units a tick, each with its splash 53190 (the
    fight dummy stands alone).
  - Typhoon's 6 s snare (53227) and Hurricane counting for a mage's Torment the Weak (a mage and a druid; on
    the snare-immune dummy only rank 4 does).
  - Death Strike's 45470 and Death Pact's 48743 taking nothing from Strength of Wrynn.
  - The bat control of `TestEcClass4PetSpecialsOnImmuneTarget`: refused (`DONT_REPORT`) for all four pets, so
    nothing shows a pet landing its special on a target that isn't immune. Cause: no line of sight from the
    pet to the bat at the entrance.
- **Owns:** `sim/{deathknight,druid,mage,warrior}/` but the other Z9 items' hunks; the item effects in
  `sim/common/` but the Valkyr's; `ui/deathknight/apls/`; the module's `e2e/ecclass4_test.go` and this item's
  own e2e file; their cycles, fights, drops and rows.
- **Goldens:** those classes' suites, the restoration druid's (Omen on its item heals), and every suite wearing
  a changed item; report each.

#### As built

Druid stage (notes in `tmp/ec-class-fix5-druid.md`). Verdicts:
- Omen on heals (fixed): the aura rolls on `OnHealDealt` too, for a flagged spell or one `rollsOmenOffFamily`
  passes. A magic-class heal carries `DONE_SPELL_MAGIC_DMG_CLASS_POS` and a HealInfo, so the PPM applies (8.75%).
- Item heals (fixed): Bauble of True Blood's use casts 71607 (both items: the heroic's `spellid_1` is 71607 here),
  Althor's Abacus heals as 71610 / 71641, Glowing Twilight Scale ticks as 75493 / 75494, each rolling a whole step
  (`itemHealSpell`). 75493 / 75494 have no damage class and only tick: no roll. Cauterizing Heal 69733 rolls.
- Nature's Grace on heals (nothing to flag): the sim registers no Healing Touch, Regrowth, Nourish or Swiftmend.
  The aura reads heals now, so one only needs `SpellFlagNaturesGrace`.
- Insect Swarm and Faerie Fire (confirmed, no change): a landed hit without damage reaches `ProcSkillsAndAuras`
  with a `NODAMAGE` DamageInfo, `PROC_HIT_NORMAL` and `DONE_SPELL_MAGIC_DMG_CLASS_NEG`; `CheckProc` passes a
  druid-family spell, and the DamageInfo makes the PPM apply.
- Hurricane (fixed; the bullet's mechanic holds for ranks 1 to 4 only): 48467's attack slow has mechanic 0. Every
  target's swings slow by 20% in the `AtkSpdReduction` category (spell_group 1059), a boss's half the time (live
  stage). Torment the Weak counts Hurricane only where its snare effect lands, never on a snare-immune target
  (`attackSlowed` in `sim/mage/talents.go`).
- Shard of the Gods (fixed, found on the way): 50 s cooldown, not 45 (`2026_02_18_01.sql` updates both rows).
- `spells_auto_gen.go` regenerated: the five heal ids, and 71838 / 71839, which the base lacked.
- Goldens: 35 of 37 suites, item rows only. Bauble rows hps up to +0.008% (whole-step roll); Abacus rows of
  TestUnholy, TestDisc, priest TestHoly up to +0.002%; Bauble rows of TestFeral dps +0.66%, TestFeralApl +0.23%,
  TestFeralTank +0.13% (Omen on the heal); Bryntroll rows of TestBlood, TestBloodTank, TestArms +0.03 to +0.26%
  (71838 / 71839 now binary from their server rows).
- Live stage: rank 5 Hurricane gives a mage no Torment the Weak on the dummy and rank 4 (27012) does; the dummy's
  attack time x1.2 under Hurricane; Clearcasting off Bauble uses; Shard procs 50 s apart.

Rest stage (notes and the live plan in `tmp/ec-class-fix5-code.md`). Verdicts:
- Bryntroll's heal (fixed): the wielder gains the damage dealt through `GainHealing`
  (`Spell::DoAllEffectOnTarget`: a bare `HealBySpell`, so no crit, threat, proc or spell power; healing done percent
  applies). Found on the way: the item gives 71838 / 71839 a 500 ms cooldown, which `Player::CastItemCombatSpell`
  checks, so a roll won inside it is lost.
- Hamstring (fixed): 1715, 10 rage less Focused Rage, Battle or Berserker Stance, a 15 s `SnareAuraTag` aura. Its
  one effect is the snare, so a snare-immune target is immune to the whole spell: a miss with the refund.
- Slow (fixed): 31589 behind the talent, a 15 s aura on one target at a time: cast time x1.3 and ranged attack time
  x1.6 on any target (category `CastTimeDebuff`, spell_group 1096), the snare tag only where snares land. No
  `SpellFlagMage`: Arcane Concentration takes damage only, and Clearcasting's and Arcane Power's masks leave Slow
  out, so it pays in full under Clearcasting.
  - `slow` is now an implemented talent: `implemented_talents.json`, `talent_paths.json` and `costaura_spells.json`
    are rewritten, and the three mage paths wait in `sweepRecut` for a rerun of the three sweeps.
  - Every dummy is snare immune (rows -361 and -286), so live can show only the immune side.
- `blood_pesti_aoe` (fixed; the premise is gone on this base): against no gate the 6 s gate gains 0.05% on the
  default row and 0.1 to 0.3% on full-buff multi-target, and costs unbuffed and short rows 0.7 to 3.0%. Blood Tap
  now also needs no death rune ready, and in the last 60 s takes neither condition: full-buff long rows and the
  default row keep the gate's numbers, unbuffed and short rows gain 0.6 to 2.8% (1500 iterations a row).
- Goldens: the six Bryntroll rows and 8 of TestBlood's 12 `blood_pesti_aoe` rows. `db.json` moves (the rotation, and an
  icon for Hamstring).
- e2e, vetted only: `ecclass5_test.go`, five tests (Slow, Hamstring, Drain Life's leech, Hurricane's ranks with
  Torment the Weak, Omen on Barkskin and Innervate).

Live stage (notes in `tmp/ec-class-fix5-live.md`; server [ac] 3d89793c7, no orphan bots). Fixed in the sim:
- Bryntroll's Drain Life never crits: 71838 and 71839 carry `SPELL_ATTR2_CANT_CRIT` (0 crits in 146 hits at 28%
  crit). The spelldump's `critCapable` doesn't read that attribute. Its six golden rows now sit 0.16 to 0.20% under
  the base.
- Hurricane's slow is on a snare-immune target half the time. `Aura::UpdateTargetMap` compares an application's
  effects with the storm's, immune ones included, and a persistent area aura isn't `IsArea()`, so the mismatch is
  dropped and added back on alternate passes, 500 ms apart on the storm's map updates (`nextStormPass`). Live: on
  the dummy in 9 and 11 of 21 probes. No golden moves.

Live checks, all agreeing with the sim after those two:
- Omen: Barkskin 19 of 200, Innervate 10 of 100, Insect Swarm 7 of 80 landed, Bauble of True Blood 18 of 150 heals.
  Rebirth not run (needs a dead player).
- Typhoon: a crit of 53227 gives 16886 within 10 ms of the hit, 0.5 s after the launch at 16 yards (5 of 5); a
  plain hit gives none; a crit on a critter in the cone gives it too. Its snare sat 5.93 s on a Drakkari Bat, with
  Torment the Weak's 12% for as long.
- Shard of the Gods: the Purified set cast only 69733 and the Shiny only 69729, procs 52.2 and 51.5 s apart, and no
  proc of the right kind in the 25 s after 25 s of the wrong kind.
- Bryntroll: each heal equals its hit, x1.3 under Strength of Wrynn (73828).
- Starfall on two dummies: 10 stars each, one a tick, each star's splash on the other dummy only.
- Hurricane: with the aura on, swings 20% slower for both ranks and the 12% only in rank 4; rank 5 never gives it.
- Death Strike 5% of maximum health a disease and Death Pact 40%, nothing from Strength of Wrynn.
- The bat control: refused at the entrance because the pet has no line of sight to the bat there. On the fight's
  floor all four specials go off at the bat (three land, one dodged); the test stands there now and requires it.
- Hamstring immune (8) on the dummy; Slow's aura on it without the 12%, with it under Thunder Clap.
- The three mage sweeps rerun: `talents.jsonl` 727 records, 31 of 31 paths; `spellmods.jsonl` 404 lines for 399;
  `costauras.jsonl` 108 for 102. `sweepRecut` is empty. Slow's cost: -12 under Arcane Focus and Precision, -40 under
  Frost Channeling, both sides.
- The code stage's e2e had three faults, fixed: `.cast triggered` skips effects, `Effects[0]` isn't Frostbolt's
  damage, and a `.aura` read before the next update.

Review: no sim change. `ecclass5_test.go`: its chat helper carries the item's prefix (`ecClass5Answers`), and the
Shard test's message names its 130 s; vetted, not rerun live.

**Cross-review (Z9).** `.cast 71838` in `TestEcClass5DrainLifeLeech` is untriggered, as its README row says; a triggered
`.cast` runs no effect (`TRIGGERED_FULL_DEBUG_MASK` holds `TRIGGERED_IGNORE_EFFECTS`), and the test comment now says so.
`SlowAttackAuraTag` is gone.

### EC-CORE-FIX7 (Z10, done; changes goldens, full suite; stages: tick, rest, speed, live)

Scratch paths below are under `G:\DevStuff\GitHub\.wave-loop\ec-core-fix6-tmp\`.

- The long world tick. The sim cuts a map update into ten even world ticks (`sim/core/update_grid.go`); live,
  one tick in eight takes 31 to 60 ms (40 on average) and the rest 10 or 11 (`TestEcCore6WorldTick` reading
  `.server info`; `live/worldtick1.log`, `worldtick2.log`, `worldtick.py`). `MapMgr::Update` updates the open
  world, battlegrounds and instances on three ticks in a row and the first runs long under the bots, so an
  instance updates one tick after the long one. The records agree: 1321 player swings land 10.8 ms past the
  attack time, 21% over 12 ms (`live/swings.py`, `swings_all.txt`), for the sim's 5.2 ms and none over 10.5. So
  every player timer runs about 5 ms a step long, 0.2% of a 2.5 s swing: an estimated −0.1 to −0.4% on all 37
  suites. The design is a lead:
  - A setting `world_long_tick_ms` (`ServerSettings` field 8; 40 live, as 34 waits 8.9 ms for live's 10.8) and
    its picker field. An update's ticks
    are the update, short ticks of `world_update_interval_ms`, the long tick taking what the drawn length
    leaves, one short tick, the next update. Even ticks when the setting or the jitter is 0, so the fixed-grid
    tests stand.
  - An update's length then spreads around its mean, not above a 100 ms floor: the floor is the cycle's, and an
    instance's update comes under 100 ms after the last when the long tick shrinks. `period/fit3.py`'s floored
    lognormal reads 0.3 to 1 ms high on a log.
  - Refit period and jitter together. The count (`live/updates.py`) and the slimes' share on the 19th update
    (`live/nineteen.py`) give 104.4 and 4.7 ms; on even ticks those put the Ignite races at 66.5% and 56.9% for
    live's 62.1% and 48.8%, so the defaults stayed 104.6 and 6, effective values. Check against the six Fire
    replays (`live/all_fights.sh`), the swings' overshoot and the slimes' share by period.
- Ignite pays 1.256 of 40% of the feeding crits live and 1.175 in the sim's replays of the same six fights (sd
  0.09 a fight, z +2.2), at matching race rates and tick counts (82.3 a fight, sim 81.4). Not a gap: the
  comparison added the tick's resisted part back on the live side only. Each fight's comparison is `live/ignite/*.cmp`; the recordings are
  `fights-fire/` and `period/MageFire_Svfleadizmkp_1791726042.log`.
- The grid's run time: its lookups are about 10% of a TestElemental run (`stretch` 7.8%, `nextWorldTick` 5.7%,
  `nextUpdate` 2.4%, `worldTickBefore` 2.2%; the drawing, `grow`, 2.4%). A flat table drawn to the horizon at
  reset and indexed by time over the shortest update, in place of the cursor walk. No golden may move; A/B on an
  idle host.
- Z9's idle throughput row is 10 to 24% over Z6's at 100 iterations, Ret and the raid most (wave-loop PLAN, Sim
  throughput), unattributed across Z7 to Z9. Profile `BenchmarkSimulate` for Ret and the raid against
  `e5a289570` (Z6), name where the time went, and win back what the grid's table doesn't; an interleaved A/B
  stands in for an idle host.
- A heal's taken term (`healTakenCoefficient`, `sim/core/spell_result.go`), each from the source only:
  - `Unit::SpellHealingBonusTaken` puts the target's own `SPELLMOD_BONUS_MULTIPLIER` mods on the coefficient
    (its `GetSpellModOwner`); the sim uses the caster's modded coefficient. Up to 1.9% of a Greater Heal with
    Empowered Healing on another target under five Talisman of Troll Divinity stacks.
  - Empowered Renew is dealt as 63543, the talent, so it takes no term; its heal 63544 is in the spell dump
    (magic class, no bonus row).
  - Paladin, death knight and item heals take the term only where they declare `Direct.SP` on the server's
    heal: unchecked spell by spell.
- A missile flies the distance less the caster's object size: `Spell::AddUnitTarget` reads
  `GetDistance(x, y, z)`, floored at 5 yd, and `Spell.TravelTime` takes `DistanceFromTarget` whole. Live:
  Typhoon hits 0.48 to 0.50 s after its launch at 16 yd (0.53 at 30 yd a second), and an image's Frostbolt flies
  its distance less about 1.5 yd. The pets' replay takes that off itself (`petsCastsImageReach`), which then
  goes. Size unknown.
- Order on a shared update, leads from the source: a creature's missile landing against an aura running out on
  it, and a boss's instant cast against a tick on it, still follow queue order.
- No Playwright test covers the "Map update jitter (ms)" picker field (`server_settings_picker.ts`; the
  pattern is in `tools/uitest/tests/settings/settings_tab.spec.ts`).
- Live:
  - Settle the period: Fire fights (`period/cycles-fire.json` through `TestSimvalEcClass3Fight`) on an idle
    host over several days, 1000 or more races of each kind for today's 174 and 84, each log with its counted
    period and its slimes' share. A 10 to 13 core test run on the host adds about 1 ms within a minute, and four
    of the six fights ran under one or a worldserver past 130%.
  - The swing after a hardcast: a hardcast at the target between swings (a paladin's Exorcism without Art of
    War, Enhancement's Lightning Bolt without Maelstrom Weapon), about 100 casts. A Flash of Light at the bot
    itself was refused 70 of 70 (`live/swing1.log`, `cycles-swing.json`). `live/swing_after_cast.py` reads it:
    landing to the next swing about 18 ms under swing to swing on the real ticks, 10.5 on the sim's.
  - A boss's swing against a Lacerate tick on it (the tick first every time) and the first Execute or Kill
    Shot no sooner than the target's first map update after the hit that crosses 20%: they need a target that
    swings and one that loses health. The dummies do neither, and no e2e cycle fights a mob with GM off.
- **Owns:** `sim/core/` but EC-PET-FIX6's pet paths and the other Z10 items' hunks; `proto/common.proto`'s server settings and their
  plumbing, the picker and its Playwright test; `mage/ignite.go` and the heal call sites, in inline hunks;
  `tools/simval/pets_casts.go`'s reach; the module's `e2e/eccore6_test.go`; their drops and rows.
- **Goldens:** all 37 (the long tick); report each.

#### As built

Tick stage (notes in `tmp/ec-core-fix7-tick.md`; base `346249292`, offline). Built:
- `ServerSettings.world_long_tick_ms` (field 8; 40 live, picker "World long tick (ms)" with a Playwright test of
  all five timing fields). An update is short ticks of `world_update_interval_ms`, the long tick, one short tick,
  the next update (`sim/core/update_grid.go`). The update's length is normal around its mean, no 100 ms floor; the
  long tick takes what the length leaves after as many short ticks as bring it nearest the setting. At 0 the
  grid is the old one, ten even ticks.
- Refit with it: `world_loop_ms` 10.44 (mean update 104.4), `map_update_jitter_ms` 5 (was 10.46 and 6).

Verdicts:
- Long tick (modelled; the spec's 34 became 40). `MapMgr::Update` runs three steps on three ticks and ends the cycle
  on the first tick `i_timer[3]` passes 100 ms by, the open world's step being the long tick: a cycle is the least
  `L + 10 m` of 100 or over, 104.5 for L of 31 to 60, which is the counted 104.4, and an instance's gap
  (`C_n + L_(n+1) - L_n`) can be under 100. A player's event waits 10.8 ms for the next tick at 40 (live swings
  10.8, `.server info` 10.8 to 11.8), 8.9 at 34, 5.2 on even ticks. Live long ticks average 38.5 to 40.8.
- Fit: the slimes' 2 s timer 2064.6 ms and 22.6% on the 19th update (live 2064.4, 25%), a 3 s timer 3043 ms;
  Ignite's 2 s race 62.7% (live 62.1%), 4 s 56.4% (48.8% of 84, z -1.4), 81.1 ticks a fight (82.3); 2400 fights,
  six recordings. The old 10.46 and 6 gave 57.7% and 53.4% on the new grid.
- Ignite's +7% (not a gap): the server's amount is what `procIgnite` computes, off the crit's post-resist damage; the
  comparison added the tick's resisted part back for the live side only, and the sim's tick is resisted. Without
  it live pays 1.218 against the sim's 1.18. One difference left: `CalcValue` adds 1 to a die-of-1 effect, 0.04%
  of a tick, unmodelled.
- Goldens: all 37 suites run, 34 move (the healers' three only in uptimes). Average-Default: -0.54 to +0.10%, the
  rogues -0.39 to -0.54 and the rest -0.34 to +0.10 (every player timer waits 5.6 ms longer a step); the loop and
  jitter refit alone moves nothing over 0.3% but Fire's -0.29.
- `DurationThroughLastTick` adds 45 ms now (the longest tick). Tests that counted on 10 ms ticks changed:
  `dot_test.go`, `natures_grace_test.go`, `arcane_blast_buff_test.go`. `TestTankBossFollowUpTicksOnceAndHastesTheBoss`
  (optimizer) wants its 0.02 s bound at 0.05.

Live stage must measure: the world tick (one in 7.4 over 15 ms, 35 to 45 long in the sim against live 31 to 60, mean wait
10.8 ms), swings 10.8 ms past the attack time, and the period against the new defaults (details in the notes).

Rest stage (notes `tmp/ec-core-fix7-rest.md`, on the tick stage's tree, offline). Verdicts:
- Heal's taken term (fixed): `Unit::SpellHealingBonusTaken` runs the target's own `SPELLMOD_BONUS_MULTIPLIER` mods on
  the coefficient. `healTakenCoefficient(target)` (`spell_result.go`) takes the caster's modded one only on a self
  heal, else the target's own copy of the spell, else the unmodded declaration (`Spell.unmoddedSP`,
  `Dot.unmoddedTickSP`). Pets are left to the unmodded one.
- Missing terms (fixed). The server's heal 63544 is in the dump (magic class, no bonus row), so Empowered Renew takes
  the default 0.8057. Item heals 69733, 71607, 71610, 71641 and Ancestral Awakening 52752 take it too. Earthliving's
  hot is 52000 with row 0.164 (the sim's spell is 51994, which has no hot aura). No term, checked in the dump: 66235,
  55341, 60530, 75493/4 and 70770 (class none, or an aura that ignores heal mods), Ghoul Frenzy (aura 20).
  Open: Empowered Renew's base is the Renew tick after `SpellHealingBonusTaken` (flat and percent), then 63544 takes it
  again; the sim takes it once.
- Missile reach (fixed): `GetDistance` takes the caster's object size off, which is 1.5 for a player
  (`Player::SetObjectScale`) and an image (live: Typhoon 0.48 to 0.50 s at 16 yd of 30 yd/s). `Spell.TravelTime` is
  `max(distance - 1.5, 5)`; `petsCastsImageReach` is gone and the replay passes `yards` whole. Other guardians'
  model reach is unread (Gargoyle, Val'kyr take 1.5).
- Order leads. A creature's missile landing against an aura running out: unfixed. `Unit::Update` runs the caster's
  events ahead of its own auras, but no sim hit reads its creature caster's aura (damage is fixed at launch), and the
  target's side is a player (updates first, already modelled) or a creature (grid order, not in the source). A boss's
  instant cast against a tick on it (fixed): `Creature::Update` ticks the creature's auras in `Unit::Update` ahead of
  `UpdateAI`; the target's gcd action now runs its due ticks first (`target_ai.go`).
- Playwright test: `server_timing.spec.ts` (tick stage) already covers the jitter field with the other four.

Goldens, this stage on top of the tick's: the heal terms and the boss order move nothing (all 37 suites bit-identical
with the reach off). The reach moves 28 suites, Average-Default within +-0.13%: Fire -0.125% (Fire Blast -2.2%,
Scorch +1.1%, Ignite -0.6%), MM +0.05%, Frost +0.03%, Demonology +0.03%, FrostFire +0.03%; Unholy Blight -2.5%;
TestProtection dtps +0.5% (1.1 hits a fight). Nine suites are unchanged. Tests changed to the 1.5 yd reach:
`missile_pool_test.go`, `pet_summon_test.go`, `serverdata_test.go`, `mirror_image_test.go` (30 yd flight 1187 ms).

Live stage must also measure: a missile's flight from a known distance (a Frostbolt at 30 yd lands 1187 ms after its
cast, not 1250), and a heal under Talisman of Troll Divinity on a target other than the caster.

Speed stage (notes `tmp/ec-core-fix7-speed.md`, offline). Golden-neutral: `dock.sh delta` before and after is identical.
- Grid lookups (`sim/core/update_grid.go`): a bucket table (`tab`, buckets the biggest power of two within the shortest
  update) replaces the cursor walk, `growTo` draws in one loop, and an update's tick fields are one packed `updateStep`.
  `TestUpdateTableFindsTheUpdateAtATime` checks the lookup against a binary search. Interleaved old/new, 100 iterations:
  Ret and Elemental -3 to -5%, Hunter, Rogue and the raid about -1%. The draw itself (4% of a run) stays.
- Where Z6 to this base went (interleaved A/B per wave, Z6 = 1.00; Ret / Elemental / raid): Z7 1.07 / 0.96 / 1.15 (dot
  ticks on the server's timers, +4% on the raid), Z8 1.05 / 1.03 / 1.12, EC-CORE-FIX6's jittered grid 1.28 / 1.12 / 1.22,
  Z9 end 1.21 / 1.16 / 1.21, this base 1.26 / 1.20 / 1.24. The grid is about 11% of a Ret or Elemental run (0.9 to 1.0 s of
  8 to 9 s, against 0.05 s before), half of their gap to Z6; the rest is spread over events and APL work. This wave's long
  tick adds 2 to 4%.
- Left: Z7's dot ticking and the spread costs, unattributed; the PLAN's Sim throughput row not retaken (host not idle).

Live stage (2026-10-13, server `3d89793c7`, no rebuild; notes `tmp/ec-core-fix7-live.md`). No sim change, no golden move;
the new `TestEcCore6MissileReach` (module `e2e/eccore6_test.go`, README paragraph) logs the bot's and the dummy's distance and
the bot's combat reach.
- World tick (two 600-sample runs): the long tick follows the host's load. Over 15 ms: 13.3% at worldserver 102%
  (long ticks 29.6 ms, wait 8.3 ms), 19.0% at 144% (40.4, 13.3), last wave 38.5 to 40.8 and 10.8 to 11.8; sim 13.4%, 40, 10.8.
  Settings stand.
- Swing after a hardcast (ShamanElemental's Lightning Bolt every 6 s, 104 casts): landing to swing 20.4 +- 4.0 ms shorter
  than swing to swing; the sim's size-biased tick is 21.6. A bolt every 3.6 s equals the swing period, so no swing
  falls between casts: a spacing must not equal the attack time plus the cast.
- Period and races, nine Fire recordings: update 103.75 to 104.80, mean 104.29 (sim 104.4); slimes' 19th-update spread
  5.14 ms (sim 5); Ignite 2 s races 171 of 284 (60.2%, sim 62.8%), 4 s 68 of 129 (52.7%, sim 56.0%), ticks 81.3 (79.7);
  the paid ratio stays about 1.28 against 1.18 (the resist add-back). Defaults 104.4, 5 and 40 stand; 129 four-second
  races give +-9%, and the host was never idle, so the spec's days of idle fights are still open.
- Missile reach: bot 18.000 yd from the dummy, reach 1.500, scale 1.000. Fireball, Frostbolt, Frostfire Bolt, Lightning
  Bolt, Shadow Bolt, Chaos Bolt and Starfire at 18 yd imply 1.5 to 1.7 by the fastest flight, so the 1.5 yd reach stands
  (30 yd: 1187 ms). Open: Pyroblast lands 20 ms earlier than that (878 casts, median 676, minimum 657).
- Not run: the heal term under healing taken (needs a priest, a second bot and `.simval damage` of a heal with the target
  selected), a boss's swing against a Lacerate tick and the Execute window (the dummies neither swing nor lose health;
  the e2e lacks a mob fight with GM off).

### EC-PET-FIX6 (Z11, done; changes goldens; stages: code, live)

Scratch paths below are under `G:\DevStuff\GitHub\.wave-loop\ec-pet-fix5-tmp\`.

- The Rune Weapon's white hits are copies, confirmed from the source and live. `npc_pet_dk_dancing_rune_weapon`
  is a `NullCreatureAI` that never swings; for each white swing the DK lands, either hand,
  `spell_dk_dancing_rune_weapon::HandleProc` deals a fresh `CalculateMeleeDamage` roll on the DK's own table
  for that hand, halved, with no procs. Live it swung with 10 of the knight's 10 landed swings, on the same
  update, and never otherwise, plain and under Bloodlust (`PETFIX_DRW` in `live/live3-drw.log`). The sim
  (`sim/deathknight/dancing_rune_weapon.go`) swings a 3.5 s weapon on its own timer, 36.7 times a fight in
  TestBlood, with the pet's Bloodlust as a stand-in, and buffs on the pet count twice over the inherited attack
  power (Battle Shout, the 10% attack power auras). Drop the auto swing, cast a copy without procs on each
  landed DK white hit, set `Uncontrollable`. Moves TestBlood and TestBloodTank; size unknown.
- Heals still land on a pet that's away: Althor's Abacus picks from `Env.Raid.AllUnits`, absent pets among
  them (`sim/common/wotlk/other_effects.go`), and `Aura.Activate`'s refusal covers auras only. Check the other
  heals that pick a raid unit; Prayer of Mending's bounce already skips units not in the world.
- Mirror Image past 20 yd, a lead: further than 20 yd plus both reaches from its target (24 from the boss dummy)
  an image can't Fire Blast (`Spell::CheckRange`); it does close in between its Frostbolts, against the lead. The
  sim's Fire Blasts anyway, and the goldens' mage stands at 30 yd (`sim/mage/mage_test.go`), where the
  lost-missile rule is an extrapolation too: its records are from 2 to 19 yd.
- The Val'kyr under melee haste, a lead: `Unit::resetAttackTimer` sets the hasted attack time, so if Windfury
  Totem's (8515) or Improved Icy Talons' area aura reaches the guardian it Smites every 17 updates, 18 in 30 s.
  `valkyrAttackTime` is a fixed 2 s (`sim/common/wotlk/nibelung.go`). It matters only past 15 casts, and moves
  the tooltip's max with it.
- `checkPetsCastsHits` (`tools/simval/pets_casts.go`) takes an image whose distance wasn't read (`yards` 0) at
  the 5 yd floor; no committed record has one. Fail such a record by name.
- A guardian's missile reach, a lead from Z10: `Spell.TravelTime` (`sim/core/spell.go`) takes
  `defaultCombatReach`, 1.5 yd, off every caster's distance; the server takes the caster's `GetObjectSize()`,
  for a creature its model's `CombatReach` (`creature_model_info`, SELECT) by its scale. Confirmed 1.5 for a
  player and an image only; unread for the Gargoyle, the Val'kyr and the greater fire elemental. No gap: the
  Gargoyle's is 1.5, and the other two's casts have no missile.
- Live:
  - After the user's rebuild (module `86f62a3`; apply `data/sql/db-world/base/simval_commands.sql` first):
    `./e2e/run.sh TestEcClass6SummonTrace` (`e2e/ecclass6_test.go`) turns `.simval trace on`, which appends each
    summon's `spellGo` and `unsummon` with its world update (`gameTimeMs`) to `<LogsDir>/simval/summontrace.jsonl`.
    Expect 3 `unsummon` rows of entry 31216 on one `gameTimeMs`, 29.5 to 30.5 s after the cast, and `spellGo` rows
    (Frostbolt 59638, Fire Blast 59637) before it. Then read the corpse's update against the last Frostbolt's
    flight end off the `ECCLASS6_TRACE` rows. Until then it's known to one update from packet times, good to
    about 40 ms: the 12 ninth Frostbolts whose flight ended by 29975 ms after the summon hit, the 5 from 30006
    on never did, and the sim's corpse goes at 29988. From the source, the `unsummon` row carries the update
    `UnSummon` ran in (`Map::DelayedUpdate` removes it within the same `MapMgr::Update`), a pet's too
    (`Pet::Remove`). The test has never run.
  - An image's Fire Blast and last Frostbolt from 30 yd: needs a spot with 30 yd of sight. `fightGround` is
    level for 18 yd, and from 28 the mage's own Fire Blast didn't engage the dummy
    (`live/live8-images-distant.log`); a Frostbolt pull from 30 yd did, in sight.
  - `TestPetfixValkyrSmites` with 8515 on the priest.
- **Owns:** `sim/core/pet.go`, `buffs.go`'s pet rules and the pets' timers; the pets' files in class code and
  `sim/common/wotlk/nibelung.go`; a heal's pick of an away pet and a guardian's reach in `Spell.TravelTime`, in
  inline hunks; `pets.jsonl`, `pets_casts.jsonl` and their replays; the module's `e2e/petfix_test.go` and
  `TestEcClass6SummonTrace`; their drops and rows.
- **Goldens:** TestBlood and TestBloodTank (the copies), Abacus rows (TestUnholy, TestDisc, the priest's
  TestHoly), the mage suites if the image's range changes, the suites of a guardian whose reach changes
  (TestUnholy's Gargoyle, the fire elemental's rows); report each.

#### As built

Stage code, re-measured on `434442b93` against [ac] `3d89793c7` and the live DB (SELECT); notes and the live plan in
`tmp/ec-pet-fix6-code.md`. Verdicts:
- Rune Weapon's white hits (fixed; cross-review: its attack power and crit follow the DK while it's out, through
  `Pet.EnableDynamicStats`, as the server asks the owner each time; the damage mods are still the summon's
  snapshot): no auto swing; each landed DK white swing (either hand, extra attacks too, not
  Rune Strike) casts `RuneWeaponPet.swingCopies[hand]`: the DK's own swing rolled again on a never-dealt DK spell
  (`registerDrwOwnerSwing`: his table, weapon, AP and mods, `OtherActionAttack` tags 3 and 4), floored half, dealt by
  the weapon with no procs. `Uncontrollable` (SummonProperties 208). New `Pet.OwnerBuffsOnly`: the weapon's hits
  read only the owner's AP, crit and damage mods (`Unit::MeleeDamageBonusDone`, `SpellDamageBonusDone`, Spell.cpp's
  crit), so it gets no buffs; that ends the double count on its strike copies too.
- Heals that pick a raid unit (fixed): Althor's Abacus picks from units in the world (71610 searches the area
  around the healed unit); Chain Heal's bounces skip an away pet. Prayer of Healing, Circle of Healing and Prayer
  of Mending already did; nothing else picks one.
- Mirror Image past 20 yd (fixed, from the source and live): from 20 yd plus both reaches (24: the image's 1.5,
  the boss dummy's 2.5, model 27510) `Spell::CheckRange` fails a Fire Blast at prepare (`<`), and the event is put
  back anyway. The image does close in, against the lead: past 15 + reaches (19) on an update it starts with no cast
  going, `ChaseMovementGenerator` sets it running: from 19.5 on (`ChaseRange`'s `MaxRange` + reaches, `IsInDist`) at
  the target, the path cut at 19 (`ShortenPathUntilDist`), so it parks on 19; from nearer toward 15 + the target's
  reach (17.5, `GetNearPoint`). The update after a cast starts still moves it (`UpdateSplineMovement` before the
  generator stops it), at speed_run 1.14286 × 7 yd/s: one update's run a Frostbolt, two after a Fire Blast. From 30 yd
  into a fight it casts Frostbolts from 29.2 down to 21 yd and about one Fire Blast, its last; Frostbolts fly
  from where it stands (`MirrorImage.closeIn`).
- Val'kyr under melee haste (fixed): Windfury Totem's 8515 and Improved Icy Talons' 55610 are raid area auras
  (effect 65, aura 138), Swift Retribution and Improved Moonkin Form aura 193, and none is blocked on the guardian;
  `resetAttackTimer` takes `GetAttackTime × m_modAttackSpeedPct`. `ValkyrPet.attackTime` is 2 s over the melee
  speed multiplier: 1666 ms, 17 updates, 18 in 30 s; with Swift Retribution 1618 ms, 16, 19. `NibelungAverageCasts`
  × that is the life.
- `checkPetsCastsHits` (fixed): a record with `yards` 0 fails by name.
- A guardian's missile reach (not a gap at the sim's distances, unchanged): `creature_model_info` × scale gives the
  Gargoyle 1.5 (models 7533, 7534, 7854), the Val'kyr 1.6 and the Greater Fire Elemental 3, but Smite and Fire
  Blast 57984 have no missile; the timed Water Elemental (70907, SummonProperties 1561: a `Guardian`, not
  `IsPet`) has model 525's 0.85 and the Rune Weapon 0.01 (53160), both at the sim's distance 0, under the 5 yd floor.

Tests: `rune_weapon_test.go` (copies on landed swings only, half the DK's roll; no buffs), `runes_test.go`'s two
creature-type tests read the DK's roll; `abacus_test.go`, `chain_heal_test.go`; `TestValkyrSmitesFasterUnderMeleeHaste`;
`TestMirrorImageClosesInBetweenCasts`, and the lost-missile test takes each Frostbolt's own flight. Replays: simval
1114 of 1114, `TestPetsCastsReplayCatchesAWrongRecord` gains the unread distance. No drop or allowance entry is
this item's.

e2e (vetted, run in stage live): `TestPetfixValkyrSmites` adds a pass under `.aura 8515`;
`TestPetfixMirrorImagesFromThirtyYards` (new); `petfixWatchImages` pulls with Frostbolt past 20 yd and logs each
image's GUID and distance every 5 s; `TestEcClass6SummonTrace` runs the far-at-the-death summons and holds each last
Frostbolt to the trace's updates.

Goldens (`dock.sh delta`), 8 suites move, Average-Default:
- TestBlood −2.03%: copies 35.8 to 28.8 a fight, crit 51.7 to 46.8%, and every weapon spell 15 to 20% a hit lower
  (no buffs counted twice). TestBloodTank's BloodAggro −3.1% dps.
- Mages, the image's Fire Blasts 6.5 to 2.1 casts at 30 yd: Arcane −0.16%, Fire −0.08%, FrostFire −0.07%; Frost
  +0.06% (Fire Blasts 6.5 to 5.5, image Frostbolt hits 19.7 to 20.8 on shorter flights).
- TestDisc and TestHoly: only the Abacus rows, hps +0.03 to +0.15%, tps +1.0 to +1.9%.
- TestUnholy and TestRestoration don't move.

Stage live (machine loaded by another session's batch; notes in `tmp/ec-pet-fix6-live.md`):
- `TestPetfixValkyrSmites` passes: plain and Bloodlust 15 Smites (mean 2052, 2062 ms), under `.aura 8515` (16%) 17,
  mean 1789 ms; the sim on its jittered updates gives 2064 ms and 17 (1776 ms), 18 only at 20%. Its precondition now
  waits for the area aura, which reaches its owner on the owner's next aura update.
- `TestPetfixMirrorImagesFromThirtyYards`: sight is fine at 30 yd. Every image closed in by the sim's step count;
  Fire Blasts landed from up to about 24 yd and failed from 24.6 on, one image parked at 19.0 and one stepped to
  18.44 (from 19 to 19.5), so the dummy's reach (2.5, not the assumed 1.5) went into the sim (range 24, chase 19,
  stop 19 or 17.5) and the test's bounds. The recorded run passes them.
- `TestEcClass6SummonTrace` not concluded: one summon's 3 unsummons share one update 30 s in, no cast after, but on
  slow updates each image got 8 Frostbolts, the last 3.2 s clear of the corpse, so no bolt hit the edge; the other
  summons and two retries hit server stalls (auth timeouts, a 159 s world update).

Review round 1: the chase's stop now depends on where the run set off (19 from 19.5 on, else 17.5; live's image
parked at 19.00 from 19.78), and Fire Blast's range is strict. `TestMirrorImageClosesInBetweenCasts` gains 19.7 and
19.3 yd cases. Goldens: only TestFrost moves further, +0.047% (was +0.056%), image Frostbolt hits 19.7 to 20.7 as
its images park on 19.

### EC-CLASS-FIX6 (Z10, done; changes goldens, C++; stages: melee, casters and C++, live)

Scratch paths below are under `G:\DevStuff\GitHub\.wave-loop\ec-class-fix5-tmp\`.

- C++, the wave's one change to the module (`SimValidationCommands.cpp`); the rebuild is the user's:
  - The spelldump's `critCapable` is `spell->IsCritCapable()`, which doesn't read `SPELL_ATTR2_CANT_CRIT`: AND
    it with `!HasAttribute(SPELL_ATTR2_CANT_CRIT)`. Until then `tools/simval/critcap.go` reads
    `Attributes[2] & 0x20000000` itself.
  - For EC-PET-FIX6's records: the creature's update time on `SMSG_SPELL_GO` and on `TempSummon::UnSummon`.
- Spells with `SPELL_ATTR2_CANT_CRIT` that may still roll a crit in the sim. 26 of the sim's spells carry it
  (`Attributes[2]` in `sim/core/serverdata/spells_auto_gen.go`); none in death knight, druid, mage or warrior
  code rolls one, and the hunter's pet abilities carry `CantCrit`. Nor does any of the rest: Shiv 5938 and
  5940, Seal of Righteousness 25742, Lightning Shield 49279, Fire Breath 55485, Healing Stream 52042, Improved
  Stormstrike 63375 and the others.
- A miss costs `irand(0, cost / 4)` of a rage or energy cost (`Spell::TakePower`: any result but a hit, block,
  absorb or reflect on the main target, after `SPELLMOD_SPELL_COST_REFUND_ON_FAIL`). Every warrior spell
  refunds a flat 80% (`Refund: 0.8`); the rogue's do too (`sim/rogue/backstab.go`), unchecked. Runic power
  already pays this way (`spendOnResult`, `sim/core/runic_power.go`). From the source; size unknown.
- Persistent area auras, a lead: `Aura::UpdateTargetMap` drops an application whose effects differ from the
  aura's by an immune one and adds it back on the next pass, 500 ms apart. Hurricane does it (`nextStormPass`,
  `sim/druid/hurricane.go`). Refuted for the rest: Blizzard with Improved Blizzard and Death and Decay's glyph
  have no effect a boss is immune to, and a snare-immune target is immune to Frost Trap's whole aura.
- Shard of the Gods' heal 69733 procs on a heal over time's application on the server (live it was
  Rejuvenation's; proc type mask 7). The sim's trigger reads `OnHealDealt` and `OnSpellHitDealt`
  (`sim/common/wotlk/item_sets.go`); whether a Renew's application fires it is unchecked.
- `blood_pesti_aoe`: Blood Tap's gate still loses 0.2 to 0.3% to no gate in the unbuffed long rows, for 0.1 to
  0.3% gained on full-buff multi-target. A 2 s change of the threshold swings a row by up to 4.7%, so tune at
  depth (`tools/apldbg`).
- `spellmod_spells.json` is stale (nothing tests it): a rerun adds Slow 31589 under Arcane Focus, Hamstring
  1715 under Impale and Focused Rage, and Bloodrage 29131 under two talents. Each then needs spell-mod probes
  on its paths.
- Live:
  - Not run in Z9: Omen off a Rebirth (needs a dead player), Bryntroll's proc rate and its 500 ms item
    cooldown, Nature's Grace once per target at the talent's ranks 1 and 2 (rank 3 can't tell per target from
    per cast).
  - The spell-mod sweep for the three new ids.
  - `ecclass5_test.go`: rerun the review's edits, vetted only. `TestEcClass5HurricaneAndTorment` accepts the
    dummy's swing multiplier at 1.2 or 1 / 1.2: keep the one the field shows.
    `TestEcClass5HealsUnderStrengthOfWrynn` failed once with no Death Strike heal in three tries and passed
    unchanged on the next run (`live/wrynn.log`): unexplained.
- **Owns:** the class directories its fixes touch but the other Z10 items' hunks; the item effects in
  `sim/common/` but EC-PET-FIX6's; `ui/deathknight/apls/`; the rage and energy costs' refund in `sim/core/`, in a small hunk;
  `spellmod_spells.json` and its probes; the module's
  `SimValidationCommands.cpp` and `e2e/ecclass5_test.go`; their cycles, fights, drops and rows.
- **Goldens:** the suites of each class whose spell changes (warrior and rogue for the miss cost), TestBlood's
  `blood_pesti_aoe` rows, and every suite wearing a changed item; report each.

#### As built

Melee stage (notes in `tmp/ec-class-fix6-melee.md`). Verdicts:
- `SPELL_ATTR2_CANT_CRIT` (confirmed, no change): all 26 spells checked. The 24 the sim casts use `OutcomeAlwaysHit`,
  `OutcomeMagicHit`, `OutcomeMeleeSpecialHit`, `OutcomeHealing`, `OutcomeMeleeSpecialNoBlockDodgeParryNoCrit` (Shiv),
  `TicksCanCrit: false` or the hunter pet's `CantCrit`; none rolls a crit. Water Shield 57961 and Improved Stormstrike
  63375 are mana gains, which the sim has no spell for.
- Miss cost (fixed): `RageCost` and `EnergyCost` `IssueRefund` give back all but `irand(0, cost / 4)` (rage in tenths),
  as `Spell::TakePower` takes. `Refund` is now `SPELLMOD_SPELL_COST_REFUND_ON_FAIL`'s percent mod only (Primal
  Precision on Ferocious Bite and Rip; the cost is cut before the `/ 4`). The flat `Refund: 0.8` of every warrior,
  rogue builder and druid spell is gone. Quick Recovery isn't that mod but a proc (`spell_rog_quick_recovery`): a
  separate energize of the listed `ManaCost` at 40 / 80% on top, so `EnergyCostOptions.ProcRefund`.
- Missing refunds (fixed): Cleave pays by its main target's result like Heroic Strike; Shiv, which never refunded.
- Ghoul's Claw (fixed): `TakePower` pays full for a non-player, so the refund is gone.
- Area spells (unchecked): Whirlwind, Thunder Clap, Shockwave, Swipe, the shouts and Fan of Knives pay the full cost on
  a miss. Whether the bot's unit target makes its miss count is for the live stage.
- Blood Tap gate (`blood_pesti_aoe`, fixed): 6 s, 10 s and no gate swing the long rows by -3 to +2%. Any threshold of
  10 s or more never fires (rune cooldown is under 10 s), so the gate is dropped: Blood Tap only in the last 60 s.
  Full-buff long rows +1.1 to +3.0%, Average-Default (the pesti APL) +1.1%, unbuffed long rows -0.3 to -1.5%
  (1500 iterations a row, seeds 101 and 5001 agree).
- Goldens (`dock.sh delta`): TestBlood 8 `blood_pesti_aoe` rows (above); TestUnholy -0.17% Average-Default (Ghoul's Claw
  81.3 to 76.2 casts); TestFeral, TestFeralApl, TestFeralTank (Maul, Swipe casts), TestCombat, TestAssassination,
  TestSubtlety, TestArms, TestFury and TestProtectionWarrior move within +-0.3% on their default rows, up to +-2% on
  single rows, from the new miss draw. `tools/simval` unchanged and all-PASS; `db.json` moves with the APL.
- `TestHamstringSnaresWhatTakesSnares` now wants the immune boss's miss to take 0 to a quarter of the cost.

Casters and C++ stage (notes in `tmp/ec-class-fix6-code.md`). No sim Go code changed. Verdicts:
- C++ (`SimValidationCommands.cpp`, compile-checked, not built): the spelldump's `critCapable` also needs no
  `SPELL_ATTR2_CANT_CRIT`; `.simval trace on|off` appends each summon's spell go and unsummon, with the world update
  (`gameTimeMs`), to `summontrace.jsonl` (`simval_commands.sql` and the module README have the row). `critcap.go` keeps
  reading `Attributes[2]`.
- Persistent area auras (no gap): `Aura::UpdateTargetMap` flickers an application only when the aura mixes an effect the
  target is immune to with one it isn't, and a `DYNOBJ_AURA` isn't `IsArea()`. Of the 105 spells with a persistent area
  effect, only Hurricane (done) and the Frost Trap auras mix them, and Frost Trap's spell-level snare mechanic makes a
  snare-immune target immune to the whole aura. Blizzard, Death and Decay, Rain of Fire, Volley, Consecration and the
  traps have no immune effect (`creature_immunities` -361 and -286: mechanics and knock back / pull effects).
- Shard of the Gods' heal (gap, not fixed): the proc flags are `DONE_SPELL_MAGIC_DMG_CLASS_POS | _NEG`, type mask 7, so any
  landed magic-class cast procs, a Renew or Power Word: Shield with no result too, ticks never. The sim's trigger reads
  `OnHealDealt` and `OnSpellHitDealt`, which `Hits++`-only casts (Renew, Earth Shield, Riptide's hot) don't fire. The fix
  is a core callback on a landed cast with its target. No suite wears the sets.
- `spellmod_spells.json` regenerated: +Slow (Arcane Focus), Hamstring (Impale, Focused Rage), Bloodrage 29131 (Improved
  Bloodrage, Intensify Rage). Six `droppedSpellMods` entries and four fixture README rows hold them until the live sweep.
- e2e `ecclass6_test.go` (vetted, not run): the rage and energy miss cost (Hamstring and Kidney Shot at the boss dummy,
  Quick Recovery's refund) and, after a rebuild, the summon trace and `critCapable`.

Live stage (notes in `tmp/ec-class-fix6-live.md`; server lock 15:01 to 15:35 on 2026-10-13, no rebuild or restart):
- Rage miss cost (confirmed): 30 immune Hamstrings took 1 to 25 tenths, mean 12.43 (sim 12.5).
- Energy miss cost and Quick Recovery (confirmed): immune Kidney Shot changed energy by -2 to +5 (draw 0 to 6, regeneration
  only adds); with rank 2, +20 to +25; means 20.07 apart (sim 20). The test refills to 100 and spends 70 on Eviscerates
  first, since `.modify energy N` sets the cap to 10N and the cast's change was lost to it.
- Area spells (no change): they pay the full cost on a miss. `Spell::InitExplicitTargets` drops a unit target the spell's
  explicit mask doesn't name, so a playerbot's `SetUnitTarget` doesn't survive for Whirlwind, Thunder Clap, Shockwave,
  the bear's Swipe, the shouts or Fan of Knives (Swipe (Cat) 62078 names its unit target and still pays in full:
  EC-CLASS-FIX7). Live: Piercing Howl on the dummy (unit target sent), 20 casts, full 100 tenths each
  (`TestEcClass6AreaMissCost`; that the dummy's result was an immune miss rests on Hamstring's snare immunity).
- Shattering Throw (fixed): its target is an explicit unit, so a miss now refunds (`sim/warrior/shattering_throw.go`). No
  suite moves beyond the melee stage's.
- Ghoul's Claw, Cleave's main target, Primal Precision's Bite and Rip: source only, no deterministic miss to probe.
- Spell-mod sweep for MageArcane, WarriorArms, WarriorFury and WarriorProtection: Slow, Hamstring and Bloodrage compare equal
  to the sim. Their 6 `droppedSpellMods` entries and 4 fixture README rows are gone; `spellmods.jsonl` has the four paths'
  new lines (all bot names change).
- Hurricane's attack time reads x1.2 on the server (`m_modAttackSpeedPct`), the sim's speed / 1.2; the test wants that only.
  `TestEcClass5HealsUnderStrengthOfWrynn` is flaky (about half the Death Strikes that go off show no heal packet, cause
  unknown); it now takes 8 tries per phase.
- Not run (need new tests): Omen off a Rebirth, Bryntroll's proc rate and 500 ms cooldown, Nature's Grace once per target at
  ranks 1 and 2.
- After the rebuild (apply `simval_commands.sql` first): `TestEcClass6CritCapable`, `TestSpelldumpCapture` (only the
  CANT_CRIT spells' `critCapable` flips), `TestEcClass6SummonTrace` and the Mirror Image tie read off its rows.

### EC-CORE-FIX8 (Z11, done; changes goldens, full suite; stages: code, speed, live)

Scratch paths below are under `G:\DevStuff\GitHub\.wave-loop\ec-core-fix7-tmp\`; the stages' notes are
`ec-core-fix7-<stage>.md` there.

- A heal's taken term (`healTakenCoefficient`, `sim/core/spell_result.go`), each from `Unit::SpellHealingBonusTaken`
  only and unmeasured; none moves a result unless the target has healing taken:
  - Empowered Renew: the server's base for 63544 is the Renew tick after the taken terms (flat and percent),
    and 63544 then takes them again; the sim takes them once (`sim/priest/renew.go`).
  - A spell with `SPELL_ATTR6_IGNORE_HEALTH_MODIFIERS` or `SPELL_ATTR0_CU_NO_POSITIVE_TAKEN_BONUS` loses the flat
    term and the percent when the taken percent is over 1 (the function's last branch). No sim heal models it;
    which of them carry either attribute is unread (Ardent Defender has the first, and class none).
  - A pet as the target: its `GetSpellModOwner` is the owner, so the coefficient takes the owner's
    `SPELLMOD_BONUS_MULTIPLIER` mods; the sim gives a pet the unmodded one.
- Ignite's tick lacks 1: `CalcValue` adds 1 to an effect with a die of 1, so the first tick of an application
  with no Ignite up is `trunc(trunc(0.4 × crit) / 2) + 1` (`procIgnite`, `sim/mage/ignite.go`). From the source;
  0.04% of a tick. Refuted: `Spell::SetSpellValue` stores the custom base less the 1, so no tick gains it.
- The long tick's spread, a lead: the sim draws it within 35 to 45 ms (`sim/core/update_grid.go`); live it ran 16
  to 46 ms at worldserver 102% and 15 to 104 at 144% (`live/worldtick1.log`, `worldtick2.log`), sd about 7. The
  mean wait, 10.8 ms, is already right, so only timers that read one tick's length move; size unknown.
- Pyroblast (42891) lands 20 ms early, unexplained: 878 casts at 18 yd, minimum 657 ms and median 676 from cast
  go to damage for the reach model's 687 plus a tick, while Fireball, Frostbolt, Frostfire Bolt, Lightning Bolt,
  Shadow Bolt, Chaos Bolt and Starfire fit 1.5 yd (`live/reach_all.py`, `fights-fire/`). The cause was
  Bloodlust's 30% scale on the caster's reach; the casts are 598 hardcasts (878 counted copies), so not Hot Streak.
- Dot ticking's run time: Z7's ticks on the server's timers (`Dot.runTick`, `tickIsDue`, `runDueTicks`,
  `TotalTicks`) cost about 4% of the raid benchmark (interleaved A/B per wave, `ab_waves_summary.txt`, `ab.sh`).
  A rewrite that moves no golden; A/B interleaved, the host is never idle. Those four functions cost under 1%: the
  cost is each tick's arming (`startTickAction`'s allocations).
- Live:
  - The taken term on a target other than the caster, never run and needing a new test: a priest and a second
    bot, five stacks of Talisman of Troll Divinity's aura (Touched by a Troll 60518, not the item's 37734; 290
    healing taken) on the target by `.aura`, then
    `.simval damage` of Greater Heal 48063, Renew 48068 and Empowered Renew's 63544 with the target selected,
    against `sim/core/healing_taken_test.go` (63544 gains `int32(290 × 0.8057)` = 233 of its own).
  - Settle the period, carried from Z10: nine Fire recordings give 284 two-second and 129 four-second races
    (60.2% and 52.7%, sim 62.8% and 56.0%; ±9% on the second) for the 1000 of each wanted, and the host was
    never idle. Each log with its counted period, slimes' share and worldserver load (`live/updates.py`,
    `nineteen.py`, `run_fight.sh`; cycles `live/cycles-fire3.json` through `TestSimvalEcClass3Fight`).
  - Ignite's first tick with its amount and resisted part, on applications with no Ignite up for 4.5 s, for the 1
    (none: 39 of 39 exact).
- **Owns:** `sim/core/` but EC-PET-FIX6's pet paths and the other Z11 items' hunks; `mage/ignite.go` and the heal
  call sites, in inline hunks; the module's `e2e/eccore6_test.go`; their drops and rows.
- **Goldens:** TestFire and TestFrostFire (Ignite's 1, refuted); all 37 if the long tick's spread changes; none for
  the dot rewrite; report each.

#### As built

Code stage (notes `tmp/ec-core-fix8-code.md`; base `434442b93`, offline). Built:
- Heal taken terms (`sim/core/spell_result.go`): `healTakenCoefficient` reads the target's spell-mod owner
  (`Unit.spellModOwner`, a pet's or guardian's owner); a heal whose server spell heals and has
  `SPELL_ATTR6_IGNORE_HEALTH_MODIFIERS` or `SPELL_ATTR0_CU_NO_POSITIVE_TAKEN_BONUS` drops the flat term and the
  taken percent when that percent is over 1 (`noPositiveHealingTaken`; a no damage class heal without a bonus row
  returns earlier on the server); `Dot.TickHealingTaken`.
- Empowered Renew (`sim/priest/renew.go`): 63544's base is `5 × rank × ticks × int32(tick) / 100`, the tick being
  Renew's aura amount (its SPELLMOD_DOT mods, `renewDotMultiplier`) as the target takes it; 63544's multiplier is the
  aura 136 talents (`renewHealingDoneMultiplier`) × Zabra's 4 piece, then its own taken terms.
- `ServerSettings.world_long_tick_jitter_ms` (field 9, 6.5 live; picker "World long tick jitter (ms)", Playwright row,
  6 of 6 pass). Each of the 1024 steps aims its long tick at `long + score × spread`, the score out of order
  (`longTickPairing`, r 0.025 with the update's length), spread `sqrt(jitter² − interval²/12)`. Update lengths and
  draws are unchanged; jitter 0 is the old table. `world_long_tick_ms` 40 to 39.5 keeps the wait at 10.8 ms (40 gives
  10.95); the longest tick goes 45 to 59 ms.

Verdicts:
- Empowered Renew (fixed): `spell_pri_renew` takes the target's `SpellHealingBonusTaken(DOT)` on the aura's amount
  (aura 136 left out of it), and `EffectHeal` runs 63544's done (aura 136, 67202) and taken (0.8057) again.
- The two attributes (fixed in core): only Ancestral Awakening (52752, CU) among the sim's heals. Beacon of Light
  53652-4 and 70691 (ATTR6) aren't modelled; Ardent Defender 66235 (ATTR6, class none) returns before the branch;
  Earth Shield's 49284 has CU but heals as 379, which doesn't.
- Pet as the target (fixed): `Unit::GetSpellModOwner` is the owner.
- Ignite's 1 (not a gap): `Spell::SetSpellValue` stores a custom base point as `CalcBaseValue` (value − 1 for a die),
  and `CalcValue` adds it back. 29 of 29 first ticks with no Ignite up for 4.5 s equal `trunc(trunc(0.4 × crit) / 2)`
  plus their resisted part (nine Fire recordings, three older MageFire fights).
- Long tick's spread (modelled): no source length, it's the open world maps' run; all four `.server info` samples
  spread it by 5.3, 6.5, 6.6 and 12.2 ms against the sim's 2.9. At 200k iterations TestAssassination −0.013%,
  TestFire −0.000%.
- Pyroblast's 20 ms (not a sim gap): the nine recordings' 598 Pyroblasts are all hardcasts (the spec's 878 counted
  copies of the same logs). There Pillar of Flame (67760, cast off the landing, delay from the next update) lands
  early too, min 809 to 815 for 825 plus a tick, while the older MageFire fights at 18 yd give Pillar 832 and Fireball
  685 (flight 687). Both floors imply 17.55 yd: the cycle fight's bot (`ecClass2Stand`, no `.simval face`) stood
  about 0.45 yd closer. Cause unread.

Goldens (never promoted): all 37 move, none 0.5% on Average-Default.
- Heal changes alone: only priest TestHoly, +0.055 to +0.065% every row; Empowered Renew +3.1% a hit, as Improved
  Devotion Aura's 6% now counts twice on the priest's self-cast ones.
- Spread alone: a reshuffle of player ticks, Average-Default −0.064% (TestBalancePhase3) to +0.043% (TestFrostFire),
  no sign pattern; healers' uptimes only.
- Final: −0.064% to +0.053% (TestHoly hps); per suite in the notes.

Speed stage (notes `tmp/ec-core-fix8-speed.md`; golden-neutral: `delta` and all 37 `.results.tmp` byte-identical):
- The four named functions cost under 1% of the raid; the dot's cost was arming: each `startTickAction` allocated
  a `PendingAction` and two closures, 60% of the raid's allocations an op (Ret 51%, Hunter 40%).
- `sim/core/dot.go`: a dot re-arms its last tick action (`spareTick`) once nothing queued can run it (consumed,
  not queued, or cancelled and detached, the GCD action's pattern), else makes one; `tickArmings` replaces the
  `tickAction == pa` test, which re-arming under a running tick defeats; `tickIsDue` reads `nextTickAt`; the second
  `tickOwed` runs only after `runDueTicks` ran a tick. `TotalTicks` kept: a multiply differs past int32. Tests in
  `dot_rearm_test.go`.
- Won: allocations an op raid −60%, Ret −51%, Hunter −40%, Elemental −20%, Rogue −24%. Five interleaved rounds
  under load 30 to 50: Ret 0.925 (all five faster); the rest within the noise (0.98 to 1.01).

Live stage must measure:
- A new `e2e/eccore8_test.go`, real casts read off heal packets (`.simval damage` heals land on the caster): Renew from
  a priest with Empowered Renew 3/3 on a second bot with and without five Talisman of Troll Divinity stacks (37734):
  each tick +109, 63544 `int32(int32(75 × (tick0 + 109) / 100) × R136) + 233`; on the priest +125 a tick; on its
  Shadowfiend +125 (the owner's mod). Ancestral Awakening on a target under Guardian Spirit: 52752 is
  `int32(0.3 × the crit heal)` exactly.
- `TestEcCore6WorldTick` on an idle host: long ticks by tick 39.5 ± 6.5, 13.4% of ticks, wait 10.8 ms.
- `.distance` at the Fire cycle's spot (17.55 if the records hold, 18 if not), and after `.simval face`.
- Follow-ups: Ancestral Awakening takes Purification and an untruncated 30% in the sim (52752 ignores caster mods,
  `CalculatePct(int32(heal), 30)` on the server); a hot's aura 136 comes after its whole amount on the server.

Live stage (notes `tmp/ec-core-fix8-live.md`; lock 16:01 to 16:46 and 16:55 to 16:56, 2026-10-14; host loaded throughout,
another session's job at 1300 to 1400% CPU, worldserver 140 to 147%). Built:
- `e2e/eccore8_test.go`: `TestEcCore8RenewTaken` (GM Island, two bots, Touched by a Troll 60518 ×5, read off heal
  packets), `TestEcCore8AncestralAwakening`, `TestEcCore8FireSpot` (`.distance` and cast go to hit by cast kind and
  Bloodlust). Module README rows.
- Bloodlust's scale (`sim/core/spell.go`, `buffs.go`, `stats.go`): `TravelTime` takes the caster's
  `combatReach()` = 1.5 × (1 + `PseudoStats.ScaleBonus`); `BloodlustAura` adds 0.3 (Heroism shares it).
- Guardian heal coefficient (`spell_result.go`): a heal declared `DefaultHealingTakenCoefficient(0)` on a pet unit
  that isn't `SummonedAsPet` takes `DefaultHealingTakenCoefficient(1)`. Cross-review: it keyed on `IsGuardian()`,
  which a glyphed Water Elemental has (built as a guardian, summoned by 70908's SUMMON_PET, so a server pet).
- Ancestral Awakening (`sim/shaman/heals.go`, inline): `SpellFlagIgnoreAttackerModifiers`, no Purification, amount
  `int32(float32(crit) × 30 / 100)`. Cross-review: that flag also keeps Strength of Wrynn's stand-in off it
  (`SpellPctHealingModsDone` returns 1 for `SPELL_ATTR3_IGNORE_CASTER_MODIFIERS`, and the buff is a healing done
  percent), so such a heal takes `HealingTakenPctMultiplier()` alone.

Verdicts:
- Taken terms (as built, live): another player +109 a tick, 63544 75% of the taken tick + 233; the priest +125; its
  Shadowfiend +125 (owner's mod). New gap, fixed: 63544 on the Shadowfiend + 545, as `Unit::GetCastingTimeForBonus`
  gives a creature that isn't `IsPet()` 3500 ms for an instant spell (coefficient 1 × 1.88).
- Ancestral Awakening (fixed): 6 of 6 exactly `CalculatePct(int32(crit), 30)` under Purification 5/5, Guardian Spirit
  and five stacks.
- Pyroblast's 20 ms (fixed; the code stage's 17.55 yd refuted): the spot is 18.000 yd, but Bloodlust and Heroism carry
  SPELL_AURA_MOD_SCALE 30%, so `Player::SetObjectScale` makes the caster's reach 1.95 (`.distance` 14 to 13.55) and
  flights 0.45 yd shorter: 668 ms at 18 yd for speed 24. Live minima under Bloodlust: Pyroblast 665 to 669, Fireball
  670; without: 686, 684. The Fire cycle fights cast under Bloodlust all fight.
- Ignite's 1 (not a gap, confirmed): 10 of 10 new first ticks exact, 39 of 39 over twelve recordings.
- Period (not settled): new fights 105.0, 106.3, 108.1 ms an update (last wave 103.7 to 104.8), slimes on the 19th 43
  to 83%; the period follows the host's load. Ignite races over twelve recordings, sim at the defaults: 2 s 202 of 349
  (57.9%, sim 62.9%; the three loaded fights 47.7%), 4 s 83 of 155 (53.5%, sim 56.4%), about 850 more 4 s races
  wanted. Defaults stand.
- Long tick: one sample at worldserver 145%, long ticks 52.5% of ticks at 63.8 ± 43.8 ms, a load artefact; the 39.5 ±
  6.5 fit stands until an idle sample.

Goldens (never promoted): this stage moves 22 suites, Average-Default within 0.03% (missiles under Bloodlust; resto
shaman's HealingGrace row from Ancestral Awakening, 52752 -9.1% a hit); 20-iteration rows to ±0.9%. Final against the
goldens: all 37 move, Average-Default −0.060% (BalancePhase3) to +0.065% (FrostFire).

### EC-CLASS-FIX7 (Z11, done; changes goldens; stages: code, live, carried)

Scratch paths below are under `G:\DevStuff\GitHub\.wave-loop\ec-class-fix6-tmp\`; the stages' notes are
`ec-class-fix6-<stage>.md` there.

- Swipe (Cat) 62078 pays the whole cost on a miss of its main target. It names a unit target (`targetA` 6 in
  `assets/db_inputs/acore/spelldump.jsonl`), so `Spell::InitExplicitTargets` keeps it and `Spell::TakePower` takes
  only `irand(0, cost / 4)`; `sim/druid/swipe.go` never calls `IssueRefund`. Confirmed in the dump and the source,
  unprobed live; size unknown. Swipe (Bear) 48562 (`targetA` 22) and Shockwave 46968 (a cone, 104) name none and
  stay as they are, which settles the cross-review's lead about them.
- Shard of the Gods (69755 Purified to 69733, 69739 Shiny to 69729) misses casts that land with no result. The
  server procs on any landed magic-class cast (proc flags `DONE_SPELL_MAGIC_DMG_CLASS_POS | _NEG`, type mask 7, 25%,
  50 s; `Spell::DoAllEffectOnTarget`'s branch for a hit with no damage or heal), ticks never. The sim's triggers
  read `OnHealDealt` and `OnSpellHitDealt` (`sim/common/wotlk/item_sets.go`), which Renew, Earth Shield, Riptide's
  hot, Prayer of Mending's bounce and Power Word: Shield don't fire. Confirmed from the source and live
  (Rejuvenation's application). The fix: a core callback raised once per target a cast lands on (the server
  rolls each target, triggered casts too), whatever the cast did, read by the trigger in place of the two so no
  landing rolls the 25% twice. The casts that
  only count `Hits++` are in `sim/core/shield.go`, `priest/renew.go`, `deathknight/ghoul_frenzy.go`,
  `mage/ignite.go`, `shaman/weapon_imbues.go` and `druid/items.go`; `Spell.OnCastComplete` carries no target. No
  suite wears the sets (items 49310, 49463, 49464, 49488).
- A static enchant effect on an item-swap weapon applies in neither direction: not in the bag, not after the swap
  in, and the main set's stays on after its weapon goes out (`sim/core/item_swaps.go`, `swapFollowingEnchants`).
  The server's `Player::SwapItem` removes the old weapon's enchants and `_ApplyItemMods` applies the new one's.
  Stat enchants already move (item stats). Make the effects with a static part reversible on
  `RegisterOnItemSwap`, each an inverse stat or pseudo-stat change: the Death Knight runes 3883, 3847, 3594, 3365,
  3595, 3367, 3369 (3595 and 3367 have no effect to move, 3369 is a proc), and weapon enchants like 3247 and 963,
  ten or so. No preset or suite has an `itemSwap`, so add
  a test that swaps to one.
- `TestEcClass5HealsUnderStrengthOfWrynn` is flaky: 6 of 11 Death Strikes that went off with diseases on the
  dummy showed no heal packet; when one comes it is 5% of max health a disease. It takes 8 tries a phase. Find
  why; unknown whether the server skips the heal or the harness misses the packet. Neither: those strikes didn't
  land (a creature dodges from behind), a test fault.
- Live:
  - After the user's rebuild (module `86f62a3`; apply `data/sql/db-world/base/simval_commands.sql` first):
    `./e2e/run.sh TestEcClass6CritCapable` (of its 13 spells every one with `Attributes[2] & 0x20000000` has
    `critCapable` false, Frostbolt true), then `./e2e/run.sh TestSpelldumpCapture`: the `spelldump.jsonl` diff is
    `critCapable` false for exactly the `SPELL_ATTR2_CANT_CRIT` spells that had true, and `dock.sh test
    ./tools/simval` stays green. `tools/simval/critcap.go` can then stop reading `Attributes[2]` itself. Neither
    test has run.
  - Carried from Z9, each needing a new test: Omen off a Rebirth (needs a dead player), Bryntroll's proc rate and
    its 500 ms item cooldown, Nature's Grace once per target at the talent's ranks 1 and 2.
- **Owns:** the class directories its fixes touch but the other Z11 items' hunks; the item effects in
  `sim/common/` but EC-PET-FIX6's; the landed-cast callback and the item swap's enchants in `sim/core/`, in small
  hunks; `spelldump.jsonl` and `tools/simval/critcap.go`; the module's `e2e/ecclass5_test.go` and
  `ecclass6_test.go` but `TestEcClass6SummonTrace`; their drops and rows.
- **Goldens:** the feral suites' rows that cast Swipe (Cat); none expected for the Shard or the swap enchants (no
  suite has them); report each.

#### As built

Code stage (notes and both live plans in `tmp/ec-class-fix7-code.md`). Verdicts:
- Swipe (Cat) (fixed): a miss on the cast's own target calls `IssueRefund` (`sim/druid/swipe.go`); another target's miss
  pays in full. `TestSwipeCatPaysTheMissCostOnItsTarget`. No golden casts 62078, so none moves.
- Shard of the Gods (fixed, wider than the bullet): per the source, the roll is once per target a cast lands on
  (`DoAllEffectOnTarget` procs each target), not once per cast, and both bonuses carry
  `SPELL_ATTR3_CAN_PROC_FROM_PROCS`, so triggered magic-class landings roll too (Earth Shield's 379, Prayer of Mending's
  41635 and 33110, Divine Aegis, Earthliving, a `ticksAsCasts` tick).
  - Core: `Unit.RegisterOnCastLanded` and `Spell.LandOn` (`sim/core/cast_landed.go`), raised by every landed direct hit
    or heal, by a `ticksAsCasts` tick, and by `LandOn` at the sites that land without a result: Renew, Power Word:
    Shield, Divine Aegis, Prayer of Mending's cast and jumps, the Prayer of Healing glyph's hot, Earth Shield's cast and
    charge heal, Earthliving, Ghoul Frenzy (on the ghoul), Ignite, Languish. Once per spell, target and instant.
  - The trigger reads only that: a magic-class spell without `SPELL_ATTR3_SUPPRESS_CASTER_PROCS`; a spell with no damage
    class by its proc mask (stand-ins: Prayer of Mending 48113, Empowered Renew 63543, Divine Aegis 47515, Arcane
    Missiles 42846). Melee and ranged never. `TestShardOfTheGodsProcsOffAHotAlone`, `TestCastLandsOnceATarget`.
  - Not covered: other aura-only casts with no `LandOn` (buffs, curses without an outcome, Beacon of Light, Sacred
    Shield 53601, Power Infusion); NONE-class spells with a spell proc mask roll wrongly (Persistent Shield 26470,
    Invigorating Earthsiege Health Regen 55341, Ardent Defender, Explosive Trap 49067).
- Item-swap enchants (fixed): `NewEnchantSwapEffect` (`sim/core/item_swaps.go`) moves a static effect with the swap,
  counted per worn weapon: the runes 3883, 3847 (defense, stamina via `NewEnchantSwapMultiplyStat`), 3594, 3365
  (parry), Scourgebane 3247, scopes 2523, 2724, 3608, 3607, Sunfire 2671, Soulfrost 2672. Weapon-damage enchants
  (963, 1897, 2723, 3843) go on the new weapon in `swapWeapon`. Cinderglacier 3369 now toggles like the other procs
  (`swapFollowingEnchants`). 3595 and 3367 have no effect to move.
  - Found on the way: an iteration ending swapped started the next one off by the swap's stat change, with a 1.5 s
    GCD (`ItemSwap.reset` re-ran `SwapItems` after `Unit.reset`). Right after `Unit.reset` it now restores the gear
    only, then runs the swap callbacks; the APL's mid-fight swap to the main set (also `reset`) stays a full
    `SwapItems`. `TestItemSwapMovesStaticEnchants` (death knight: runes, weapon enchants, to a bare weapon),
    `TestItemSwapBackToTheMainSet`.
  - Not covered: racial weapon expertise (human swords) doesn't follow a swap.
- Death Strike's missing heals (not found): `spell_dk_death_strike`'s EFFECT_2 dummy casts 45470 untriggered on a hit;
  45470 has no GCD category, cooldown, cost, reagent or form, and the executing strike can't be interrupted by it,
  so nothing in `Spell::prepare` or `CheckCast` refuses it. Logs from the last live runs show the strike from behind
  (no dodge or parry; 7.6% miss) failing to heal about half the time, which a miss doesn't explain.
  `TestEcClass5HealsUnderStrengthOfWrynn` now logs `ECCLASS5_STRIKE`: the strike's result on the dummy and 45470's
  `SMSG_SPELL_GO` or `SMSG_CAST_FAILED`.
- e2e (vetted, not run), `ecclass7_test.go`: `TestEcClass7SwipeMissCost`, `TestEcClass7RebirthOmen` (11.67%),
  `TestEcClass7BryntrollProcs` (2 PPM, 500 ms; `item_template` agrees), `TestEcClass7NaturesGracePerTarget`
  (ranks 1, 2). README rows.
- Goldens: none moved (37 suites); no suite casts Swipe (Cat), wears the Shard sets or swaps items.

Live stage (notes in `tmp/ec-class-fix7-live.md`; server lock 14:24 to 14:40 on 2026-10-14, no rebuild or restart):
- `TestEcClass6CritCapable` (pass): the 12 `SPELL_ATTR2_CANT_CRIT` spells dump `critCapable` false, Frostbolt true.
- Spelldump recaptured (`TestSpelldumpCapture`, `spellids -all`, the ten classes and pets, merged over the committed
  capture): `critCapable` true to false for exactly the 109 CANT_CRIT spells that had true. Also moved, read by
  neither `gen_serverdata` nor `tools/simval`: `scripts` on 19 spells (mod-spell-tweaks 9bf5c75's Pyroblast, Burnout,
  Curse of Doom and Glyph of Backstab scripts, which the sim already models), and 31930 Judgements of the Wise is new
  (AC `67b510271` sets its family to paladin). Six ids the id list no longer asks for keep their committed lines.
- `gen_serverdata` rerun: no table moves, so no golden. `critcap.go` now reads the spelldump's `critCapable` (no
  `Attributes[2]` or effect list); `attr2CantCrit` stays for `damage.go`'s ticks. `TestCritCapableReadsCantCrit`.
- Death Strike's missing heals (test fault, fixed): in 7 runs, 13 of 17 strikes hit and each healed once; 3 dodges and
  1 miss cast no 45470. A creature dodges from behind (`Unit::MeleeSpellHitResult` turns dodge off for a player
  victim only), which the test had ruled out. It now retries a strike that doesn't land and fails one that hits
  diseases and heals nothing. No sim gap: the sim's dodge doesn't depend on the side.
- Swipe (Cat) (confirmed): 63 hits took 49 to 50 energy, 17 misses (9 parries, 5 dodges, 3 misses) 0 to 12, mean 5.59
  (sim `irand(0, 12)`).

Carried stage (notes in `tmp/ec-class-fix7-carried.md`; server lock 14:58 to 15:30 on 2026-10-14, no rebuild or
restart). All three match the sim; no sim change, no golden.
- Omen of Clarity off Rebirth (pass): 12 Clearcastings in 100 landed Rebirths, 12.0% (sim 11.67%: 3.5 PPM over 48477's
  2 s cast time entry; a no-damage hit passes a `DamageInfo`, so `Aura::CalcProcChance` takes the PPM path).
- Bryntroll (pass): white swings 8 Drain Lifes off 89 landed, 8.99% (sim 11.33%, z -0.70). Whirlwind, two runs: 42 off
  385 landed hits in 140 Whirlwinds (about 40 expected at one a Whirlwind), never two inside 500 ms (about 5 expected
  pairs without the cooldown). No white swings ran; 44949's off-hand hits can't roll a main-hand two-hander.
- Nature's Grace per target (pass): rank 1 by dummies crit 0 to 3, casts 31/33/14/2 gave it 0/13/7/1; rank 2 26/41/12/1
  gave 0/22/11/1. Per target beats once a cast about 26 to 1; nothing on 57 casts without a crit (61384 isn't crit
  capable, so its cast phase never carries `PROC_HIT_CRITICAL`; 53227 is triggered, so it has none).
- Test faults fixed (`ecclass7_test.go`): `.additem 44614 200` read 44614 as a character guid (an offline character,
  so nothing was added anywhere), now named; the warrior is stripped and learns Two-Handed Axes (197) before
  Bryntroll; `/Whirlwind` also checks the Drain Life count against `1 - (1 - p)^n` a Whirlwind plus white swings.
  Cross-review: every other `.additem <entry> <count>` (ecclass3, ecclass5, fight, strikes, the hunter's recorded run)
  had the same fault and goes through `runner.addItem`, which names the bot.

### EC-TOOLS3 (Z11, done; golden-neutral, no C++; stages: code, live, spawn spots)

Scratch paths below are under `G:\DevStuff\GitHub\.wave-loop\`.

- A fight against a target that swings and loses health. The dummies do neither and no fight cycle runs with GM
  off, so two of EC-CORE-FIX7's orders stay unmeasured (below). Wanted in the fight helpers (`e2e/fight_test.go`):
  a creature that swings at the bot and takes a bleed, fought with `.gm off`.
- A health read per update, to find the hit that crosses 20%. C++, the wave's one change to the module
  (`SimValidationCommands.cpp`; the rebuild is the user's), unless the health updates' packet times already place
  it (they do: no C++): `.simval trace` (Z10, `summontrace.jsonl`) stamps rows with `gameTimeMs` and logs summons
  only, so a watched unit's health by update is its extension.
- The other e2e tests' dummies still stand at the entrance (`newBot`, `spawnInFront`): move them to the fight
  floor, as `fightBuild` does.
- `DruidBalance.log.gz` keeps two critter `UNIT_DIED` lines: re-trim it (`simval fight -trim`); the replay doesn't
  read them.
- Live, as the helpers' proof; record the results and fix nothing in the sim (a mismatch is EC-CORE's follow-up):
  - A boss's swing against a Lacerate tick on it, on a shared update: the tick first every time, from the source.
  - The first Execute or Kill Shot comes no sooner than the target's first map update after the hit that crosses
    20%.
- **Owns:** the module's `e2e/fight_test.go`, its new `e2e/ectools3_test.go`, `SimValidationCommands.cpp` and
  `simval_commands.sql`; the other e2e tests' spawn spots, in inline hunks; `tools/simval/fight.go`;
  `DruidBalance.log.gz`; their rows.
- **Goldens:** none.

#### As built

Code stage (offline; notes and live plan in `tmp/ec-tools3-code.md`):
- No C++: the packets place the crossing. `Map::Update` sends a creature's changed fields only in
  `SendObjectUpdates`, on the map's own update and after `UpdateNonPlayerObjects`, so the first values block with
  the health under 20% is the update whose `Unit::Update` turns `AURA_STATE_HEALTHLESS_20_PERCENT` on
  (`UNIT_FIELD_AURASTATE` comes with it). Each hit's damage packet (`SendAttackStateUpdate`,
  `SendSpellNonMeleeDamageLog`, `SendPeriodicAuraLog`) goes out at the hit, in the same stream, so the last block's
  health less each hit names the crossing hit. The committed logs agree (Chronicle stamps at those same sends): the
  dummy's ticks on one update are 0 to 3 ms apart, on two at least 60, and the player's hits land between
  (WarriorArms: Deep Wounds and Rend at ...936098, Overpower at ...936307).
  A `.simval trace` health row would give the same health per update, on a clock Chronicle's doesn't share.
- e2e `fight_test.go`: `fightMobs` (XT-005 Debugger 34515, a level 83 boss without script, spells or immunities, under
  GM mode 10 yd south of the dummy, the bot 3 yd past) and `fightMobsEngage` (`.cheat god on`, `.gm off`, `FIGHTMOB`).
  `ectools3_test.go`: `TestEcTools3LacerateBeforeSwing` and `TestEcTools3ExecuteWindow` (module README), off a packet
  recorder that parses the values blocks itself: the client applies a packet after its hooks, and takes white hits off
  its own copy of the health. Vetted; the analysis was checked on synthetic packets in a scratch copy.
- Spawn spots: `tmp/spawn-spots.patch` (not applied) has `newBot` teleport to `fightGround`, so every
  `spawnInFront` dummy stands on the floor, with the README and `fightGround` comment; `git apply --check` passes.
- `DruidBalance.log.gz` re-trimmed with the existing `-trim`: only its two critter `UNIT_DIED` lines went. `fight.go`
  needed no change.

Live stage (2026-10-14, server lock held 14:08 to 14:23; running image, no rebuild; notes in `tmp/ec-tools3-live.md`):
- `TestEcTools3LacerateBeforeSwing` PASS twice. XT-005 comes up with its template's 6,000,004 health (no
  mod-dungeon-scale), targets the bot and swings every 2 s. First run: 70 of 160 Lacerates refused
  `UNIT_NOT_INFRONT`, since engaged mobs spread around the bot. `fightFace` now turns the bot to each mob before its
  cast: 155 of 160 went off, 72 to 76 ticks a mob. Tick before swing on every shared update, 14 of 14 and 28 of 28.
- `TestEcTools3ExecuteWindow` PASS: 8 of 8 rounds with the crossing white hit, the update and an Execute, none
  before the update. The aura state came in the first block under 20% and the health added up every round. Executes
  sent between the hit and the update were refused (`TARGET_AURASTATE`) in 4 rounds. Loaded timings: hit to update
  9 to 117 ms, update to Execute 3 to 25 ms.
- Both match the sim (`runDueTicks` before a creature's swing; phase 20 at `NextServerTick` from the hit): nothing
  for EC-CORE.
- `ecTools3Kill` kills the tests' live mobs at the end: a temp spawn outlasts the bot. Orphan bots: none. Nothing
  waits for a rebuild.

Spawn spots (2026-10-14, lock 17:54 to 18:54; notes in `tmp/ec-tools3-spawn.md`): `newBot` and `TestSimvalBuffs`'
caster stand on `fightGround`. The default `run.sh` passes (two P7 tank tests on their known relog and talent
races, then on a rerun), its capture replays 1114 of 1114, and every by-name test passes but
`TestParfu10ArcaneMana`, which stays at the entrance: it fails there too, casting before its `.aura` takes effect.
`TestEcTools3ExecuteWindow`'s final form passes, 8 of 8 rounds. Cross-review: `parfu10SelfAuras` now waits 700 ms
after its `.aura`s, `TestParfu10ArcaneMana` kills the floor's critters (`fightKillCritters`) and stands on the floor,
and passes there (166 s).

### EC-CORE-FIX9 (Z12; changes goldens, full suite; stages: code, live)

Scratch paths below are under `G:\DevStuff\GitHub\.wave-loop\ec-core-fix8-tmp\`; the stages' notes are
`ec-core-fix8-<stage>.md` there.

- A hot's healing done percent (aura 136: Blessed Resilience, Focused Power) comes after its whole amount, off the
  caster's auras at each tick: `CalculateAmount` truncates the amount without it (`SpellPctHealingModsDone(...,
  false)`), then `AuraEffect::HandlePeriodicHealAurasTick` multiplies by
  `GetTotalAuraMultiplier(SPELL_AURA_MOD_HEALING_DONE_PERCENT)` and truncates again. The sim snapshots both in one
  multiplier (`dot.SnapshotAttackerMultiplier = CasterHealingMultiplier()`, `sim/core/spell_result.go`; set in
  `priest/renew.go`, `prayer_of_healing.go`, `shaman/heals.go`, `common/wotlk/other_effects.go` and others) and
  truncates once. From the source: off by 1 a tick, plus a percent that comes or goes mid-hot; size unknown.
- No target combat reach: Mirror Image hardcodes the boss dummy's 2.5 (`mirrorImageTargetReach`, model 27510,
  `sim/mage/mirror_image.go`) in its Fire Blast range and chase stop, which a raid boss's model reach
  (`creature_model_info.CombatReach` × scale) moves. Add a target reach to the target's settings, the dummy's 2.5
  by default, and have the image read it (`mirrorImageReach` beside it is EC-PET-FIX7's). Confirmed from the source
  and live in EC-PET-FIX6; no golden at the default.
- An instant's missile lands about 10 ms sooner than a hardcast's, a lead: at 18 yd, Pyroblast under the cast time
  cheat took 658 ms from cast go to hit against 665 to 669 hardcast under Bloodlust (flight 668), 675 against 686
  without (flight 687), minima of 15 casts (`TestEcCore8FireSpot`, `live8/firespot1.log` to `firespot3.log`). It
  fits an instant going off in the session pass; the sim gives both the same flight (`Spell.TravelTime`). Cause
  unread; Hot Streak's Pyroblasts (TestFire, TestFrostFire), size unknown.
- Live, each on an idle host:
  - Settle the period, carried from Z10: twelve Fire recordings give 2 s races 202 of 349 (57.9%, sim 62.9%; Z11's
    three loaded fights 47.7%) and 4 s 83 of 155 (53.5%, sim 56.4%), about 850 more 4 s races wanted. The period
    follows the host's load: Z11's fights ran 105.0 to 108.1 ms an update at worldserver 140 to 145%, Z10's 103.7
    to 104.8. Each log with its period, slimes' share and load (`live8/updates.py`, `nineteen.py`, `replay8.sh`;
    cycles `live8/cycles-fire3.json` through `TestSimvalEcClass3Fight`; recordings `fights-fire/`).
  - `TestEcCore6WorldTick` (`e2e/eccore6_test.go`): the long tick's 39.5 ± 6.5 ms rests on four lighter samples;
    Z11's at worldserver 145% (52.5% long ticks at 63.8 ± 43.8 ms, `live8/worldtick1.log`) was a load artefact.
  - The dot re-arm's wall time: under load 30 to 50 only Ret's 0.925 cleared the noise. `speed/build.sh base old
    new` (old overlays `old_src/`), then `speed/ab.sh 5 <out> old new`.
- **Owns:** `sim/core/` but EC-PET-FIX7's pet paths and the other Z12 items' hunks; the hot call sites and the
  image's target reach, in inline hunks; the module's `e2e/eccore6_test.go` and `eccore8_test.go`; their drops and
  rows.
- **Goldens:** the healer suites with a hot under aura 136 (TestHoly, TestDisc, TestRestoration); TestFire and
  TestFrostFire if instant missiles land sooner; none for the target reach or the wall time; report each.

#### As built

Code stage (notes `tmp/ec-core-fix9-code.md`; base `9a5d8fb6a`, offline). Built:
- Hot ticks (`sim/core/healing_done.go`): `Unit.AddHealingDonePct`/`RemoveHealingDonePct` hold the aura 136 percents;
  `Dot.SnapshotHotMultiplier(m)` snapshots the amount without them, and each tick is `int32(float32(trunc(base × m)) ×
  mult)`, mult `GetTotalAuraMultiplier`'s float32 AddPct chain off the caster at that tick. Call sites (inline): Renew
  (`DamageMultiplier` now `renewDotMultiplier`), Glyph of Prayer of Healing's 56161 (keeps PoH's multiplier, as its
  bp comes off the PoH heal), T10 2-piece, Riptide (Purification divided out), Earthliving, Forethought Talisman,
  Glowing Twilight Scale; not Ghoul Frenzy (OBS_MOD_HEALTH, no caster 136) or Earth Shield (a charge, no tick; see
  the review).
  Registered: Focused Power, Blessed Resilience (`priest/talents.go`), Purification, aura 136 too
  (`shaman/talents.go`), Avenging Wrath's 20% (`paladin/avenging_wrath.go`).
- Target reach: `Target.combat_reach` (field 24) → `Unit.EnemyCombatReach`, `core.BossDummyCombatReach` 2.5 when
  unset; Mirror Image moves its Fire Blast range and chase by `targetReachOver()`; UI "Combat Reach" in the advanced
  target picker (0 means 2.5). Tests `healing_done_test.go`, `target_reach_test.go`, `priest/healing/renew_test.go`,
  `mage/mirror_image_reach_test.go`.

Verdicts:
- Hot aura 136 (fixed, from the source; no record or drop existed): the tick multiplies for every unit-aura hot,
  whatever its attributes or damage class, so item hots take it too.
- Divinity (left out): reading `Talents.Divinity` makes it implemented, staling `implemented_talents.json` and the
  talent sweep's paths for a live re-record, with no golden.
- Target reach (built): bit-identical at the default.
- Instant missile lead (not a gap, unchanged): an instant from the session pass starts its flight in the same tick's
  `m_Events` (`Map::Update` runs the session, then `Player::Update`; `SpellEvent::Execute` sets DelayStart), as a
  hardcast does in the Execute that casts it. Committed fights' Chronicle stamps, excess over the flight (min /
  median ms): instants (Arcane, Aimed, Chimera, Kill Shot, Ice Lance) -3..3 / 1..7, hardcasts -2..5 / 5..14; the
  twelve Fire recordings' 835 Pyroblasts are all hardcasts (-15 / 9 under Bloodlust). Firespot's 10 ms was client
  receipt on a loaded host with fixed waits.
- New, not fixed: the sim lands a missile at go + flight, the server on the caster's first update after (median 5
  to 9 ms); Auto Shot (15 to 17) and a cast off a landing (Pillar of Flame 67760, 21) start one update later.

Goldens (never promoted), the hot change alone; the reach and the lead move none:
- TestDisc +0.097% hps: Forethought Talisman +3.9% a hit (Focused Power at the tick), rows +0.09 to +0.14%.
- TestHoly (priest) BlessedResilience row -0.012%: Renew -0.038% a hit (truncation order); Default takes neither.
- TestRestoration (shaman) HealingGrace row +0.338% hps: Earthliving +10% a hit (Purification).
- 34 unchanged.

Live stage must measure (idle host; plan with commands in the notes):
- Fire recordings for the period: `cycles-fire3.json`, 300 s each, about 9 four-second races a fight; 12 fights add
  about 105. Sim: period 104.4 ms, 2 s races 62.9%, 4 s 56.4%.
- `TestEcCore6WorldTick`: long ticks 39.5 ± 6.5 ms, 13.4% of ticks, wait 10.8 ms.
- Dot re-arm A/B: `tmp/speed/build.sh old new` (`tmp/old_src/dot.go`: Z11's pre-rearm file plus this item's field),
  `ab.sh 10 <out> old new`; Z11 under load: Ret 0.925, others 0.98 to 1.01.

Live stage (notes `tmp/ec-core-fix9-live.md`; lock 2026-10-16 15:34 to 21:12; no orphan bots). Host idle (7 to 30% CPU)
until 20:59, but the random bots were refilling (106 online at 15:35, 982 by 16:04, 1180 from 18:43), and the
worldserver's load and timing followed them; the user raided from 20:50 and loaded the host from 21:00 (last fight,
last two tick samples). Recorded 51 Fire fights (`tmp/fights-fire/`) and ten tick samples; no fixture, no sim change.
Verdicts:
- Period (defaults stand): 104.35 over the 51, 104.36 over the 48 idle; by population 103.18 at 982 online, 104.24 at
  1056, 105.08 at 1180 (the last 13 at 105.35 to 105.55); 104.35 and 104.54 with the user's raid on. Fights range
  102.7 to 106.1; within one, 19 updates stray 16.8 ms (`nineteen.py`).
- Ignite races: live 2 s 854 of 1590 (53.7%), 4 s 375 of 746 (50.3%); sim at the defaults 62.8% (z -7.5) and 56.2%
  (z -3.2), but replayed at each log's own counted period 55.2% (z -0.7) and 49.9% (z +0.0), the twelve older logs
  z -0.7 and +0.1: the model's races are right, and the gap is the fights' period spread against one mean (the rate
  peaks near 104.3). Jitter 3.85 at the own period fits worse (z -1.8, older -2.5), so 5 stays. 4 s races: 901 with
  the twelve, about 100 (7 or 8 fights) short of the spec's 1000.
- Long tick (defaults stand): grows with the population: 106 online 2.6% of ticks at 18.9 ms (wait 5.3); 982 to 1056
  12.9% at 36.2 ± 5.1 (9.7); 1180 15.3% at 37.7 ± 8.7 (11.0), the model's 13.4% at 39.5 ± 6.5 (10.8).
- Dot re-arm A/B, 16 interleaved pairs, loaded by the user's game (host 18 to 48%): new/old Ret 0.972 (14 of 16
  faster), Hunter 0.994, Elemental 0.992, Rogue 1.002, raid 0.999.

Goldens: this stage moves none; the final moves are the code stage's.

Review, round 1: Earth Shield's charge (fixed, from the source; golden-neutral, no suite casts it).
`spell_sha_earth_shield::CalculateAmount` takes 49284's `SpellHealingBonusDone(HEAL)`, aura 136 (Purification)
included, before the glyph and Improved Shields; 379 has `SPELL_ATTR3_IGNORE_CASTER_MODIFIERS`. The sim had none:
`purification` in `shaman/heals.go`, test `shaman/restoration/earth_shield_test.go`.

### EC-PET-FIX7 (Z12; changes goldens; stages: code, live)

Scratch paths below are under `G:\DevStuff\GitHub\.wave-loop\ec-pet-fix6-tmp\`.

- The Rune Weapon's threat goes to the knight: `spell_dk_dancing_rune_weapon::HandleApply` (49028) calls
  `RegisterRedirectThreat` for 100% of the weapon's threat. The sim books it on the weapon
  (`sim/deathknight/dancing_rune_weapon.go`), and `AddFinalPetMetrics` passes only dps to the owner. A redirect
  moves the threat the weapon made, with its own modifiers, not the knight's presence: read those first.
  Confirmed from the source; TestBlood about +1.1k tps on 5868 (+9 to +18% by the weapon's threat per damage), and
  TestBloodTank's BloodAggro row, whose APL casts 49028, unsized.
- The Rune Weapon's snapshot leftovers: its damage multipliers stay the summon's (`drwDmgSnapshot`, no event to
  hook), and its strikes roll a 3.5 s stand-in weapon where `Spell::EffectWeaponDmg` goes through
  `owner->CalculateDamage`, the knight's own weapon (normalized 3.3 or his speed: a normalized flag at the
  `DrwWeaponDamage` call sites). A lead too: `HandleProc` leaves the knight's `UNIT_FIELD_COMBATREACH` at 0.01
  after each spell copy, which lengthens his missiles' flight (Death Coil) once he stands past the 5 yd floor. Size
  unknown (TestBlood, BloodAggro).
- Pets' distance and reach, a lead: the Imp and the Water Elemental (`PetAI`, combat range their spell's max) and
  the Gargoyle (`AttackStartCaster` 40) cast from about where they stand, but their `DistanceFromTarget` is 0, so
  Firebolt and Waterbolt fly the 5 yd floor (312 ms), Gargoyle Strike 250. With a distance the reach counts: a
  creature's is its model's `CombatReach` × scale (`Creature::SetObjectScale`; 1.5 × scale for `IsPet()`), the
  timed Water Elemental's 0.85 (model 525). Scale auras: Bestial Wrath 19574 gives a hunter pet +50%, and the haste
  carriers (hunter pet, ghouls, Spirit Wolves) take Bloodlust's 30% (their immunity is per effect, auras 193 and
  65), which `BloodlustAura` doesn't give them (`PseudoStats.ScaleBonus`, `Unit.combatReach`); a hunter pet's
  native scale is its family's, for its missiles in `sim/hunter/pet_abilities.go`. Size unknown.
- Mirror Image's own reach is its cloned model's (`CloneCaster`'s `SetDisplayId` resets the scale, and it never
  gets Bloodlust): in the base `creature_model_info`, 1.5 for the human, night elf and blood elf models and the
  male dwarf, undead and troll ones; none, so 0.389, for the gnome models, the male draenei and the female dwarf,
  undead and troll ones. The sim takes 1.5 (`mirrorImageReach`), 1.1 yd off on the range, chase and flights for those. Read
  the live table first; gender isn't a sim input, so 1.5 stays the default and no golden moves (Troll mages).
- Live:
  - `TestEcClass6SummonTrace` (`e2e/ecclass6_test.go`) on an idle host, carried from Z11 with EC-PET-FIX6's
    expectations: it needs a ninth Frostbolt going off about 29.2 to 29.5 s into the summon, which updates slower
    than about 3.1 s a Frostbolt never allow. So far one summon's 3 unsummons on one update 30 s in, no cast after,
    its last bolt 3.2 s clear of the corpse (`live/trace.log`; `trace2.log`, `trace3.log` stalled).
  - The knight's threat with the Rune Weapon out, against the redirect; needs a new test.
- **Owns:** `sim/core/pet.go`, `buffs.go`'s pet rules and the pets' timers; the pets' files in class code; the
  weapon's threat and the pets' reach in `sim/core`, in inline hunks; `pets.jsonl`, `pets_casts.jsonl` and their
  replays; the module's `e2e/petfix_test.go` and `TestEcClass6SummonTrace`; their drops and rows.
- **Goldens:** TestBlood and TestBloodTank (threat, multipliers, weapon); the suites of a pet that gets a distance
  (warlock, mage, TestUnholy's Gargoyle, the hunter suites); report each.

### EC-CLASS-FIX8 (Z12; changes goldens, full suite if Judgement of Light moves every suite; stages: code)

- Chain Heal's jumps: the sim takes the first units of `Raid.AllUnits` in order, the main target included
  (`sim/shaman/heals.go`). `Spell::SearchChainTargets` drops the main target and each unit already hit, and a chain
  heal jumps only to the unit with the largest health deficit, over 0, in jump range and sight, so it ends when no
  one is hurt. Confirmed from the source; turns on what the sim knows of the raid's health; size unknown
  (TestRestoration's Chain Heal rows).
- Echoes of Light (Althor's Abacus, 71610 and 71641): `spell_item_echoes_of_light::FilterTargets` keeps the ally
  with the lowest health percent (`HealthPctOrderPred`) around the healed unit; the sim picks a random unit in the
  world (`sim/common/wotlk/other_effects.go`). Confirmed from the source; size unknown (the Abacus rows in TestDisc
  and TestHoly, which Z11's away-pet fix moved +0.03 to +0.15% hps).
- Shard of the Gods' remaining gaps: landings with no `LandOn` never roll: aura-only casts (buffs, curses without an
  outcome, Beacon of Light, Sacred Shield 53601, Power Infusion) and item hots and shields (Scarab Brooch 21625 in
  `sim/common/tbc/caster_trinkets.go`; Forethought Talisman, Glowing Twilight Scale and 40382's shield in
  `sim/common/wotlk/other_effects.go`). Spells with no damage class but a spell proc mask roll where the server
  wouldn't (Persistent Shield 26470, Invigorating Earthsiege Health Regen 55341, Ardent Defender; Explosive Trap
  49067 a lead). From the source and the code; no suite wears the sets (items 49310, 49463, 49464, 49488).
- A racial weapon expertise (a human's sword or mace) doesn't follow an item swap (`sim/core/item_swaps.go`). From
  the code; no suite swaps items.
- **Tank fixes** (the user's call, 2026-10-15; BIS-TANK-HEALTH's health margin measures through them). From the
  code, against [ac]'s source:
  - Ardent Defender (`applyArdentDefender`, `sim/paladin/talents.go`, against
    `spell_pal_ardent_defender::Absorb`): lethal on the raw hit (`cur - dmg <= 0`, 66233 down), where the sim
    decides on the hit after its own cut; a save absorbs the whole hit and heals to min(cur + heal, Max), where
    the sim ends at the heal; under 35% the whole hit loses the absorb percent, crossing 35% only (35% Max -
    remaining), where the sim over-cuts; integer amounts as `CalculatePct`; the heal percent at the proc,
    `uint32(30 x min(1, def/540))` off `DefenseSkill()` then, where the sim fixes it at talent time. Check 66235
    against healing-done percents and crits, and the absorb order against Sacred Shield and Divine Aegis.
  - Bonus health on expiry (Vampiric Blood 55233, Last Stand 12976, Survival Instincts 50322): as
    `HandleAuraModIncreaseHealth`, removal takes the bonus off current health (to 1 if health <= bonus; 0 stays
    0), then lowers max; the sim never takes it off, a lasting heal. A `healthBar` method in `health.go`: no TMI
    entry or damage-taken event, which `RemoveHealth` would add.
  - Improved Death Strike's heal +100% at 2/2 against the server's +50% (`death_strike.go`), and Last Stand's
    cooldown inverted with its glyph (`warrior/talents.go`, 58376 against 12975's 180 s): confirm from the spell
    data first.
  - Feral Tank `TypeAuto` (`autoRotation`, `ui/feral_tank_druid/sim.ts`) sends the simple preset with no
    priority list: a bear only swings. Send a real APL; a captured request shows Survival Instincts, Frenzied
    Regeneration and Barkskin cast.
  - Judgement of Light's heal (`sim/core/debuffs.go`, an empty hook): 2% of the attacker's max health, rate and
    conditions from the spell data and proc table.
  - Lay on Hands, missing: heals for max health, 20 min (glyph), Forbearance with Divine Protection. Cast by the
    Prot Paladin default APL behind `currentHealthPercent` at mod-playerbots' Lay on Hands threshold; say which.
- **Hunter shots without a ranged weapon** (the user's call, the same day). Since 044fee9d6 `StrikeDamage` returns
  NaN for a weapon with no `AttackPowerPerDPS`, which `EnableAutoAttacks` sets for the hands only: the chain's P1
  hunters, whose Ulduar weapons the P1 pool trims, simmed NaN and the optimizer returned junk. `Spell::CheckItems`
  fails a spell whose `EquippedItemClass` isn't worn: Auto, Aimed, Multi, Kill, Chimera, Silencing, Steady and
  Arcane Shot can't be cast without one (confirm each from Spell.dbc). `no_ranged_weapon_test.go` passes on NaN
  (`Dps.Avg <= 0`): make it fail on NaN and empty the main hand too.
- **Verify, the tank fixes:** an AD save at 10,000 of 50,000 health, a raw 11,000 hit, 535 Defense ends at 24,500
  with the debuff up, and a second lethal hit inside 120 s kills; at 30% a 5,000 hit costs 4,000; from 20,000 a
  5,000 hit costs 4,500; a Defense buff applied after talents raises the heal. VB, Last Stand and SI on expiry:
  from full the tank ends at max; at 30% at H - bonus; H <= bonus ends at 1; 0 stays 0; no TMI entry.
- **Owns:** the class directories its fixes touch but the other Z12 items' hunks; the item effects in
  `sim/common/`; `sim/core/cast_landed.go` and the swap's racial expertise in `sim/core/item_swaps.go`, in small
  hunks; the bonus-health helper in `health.go` and Judgement of Light in `debuffs.go`, in small hunks; the
  Feral Tank `sim.ts` and the Prot Paladin APL; their drops and rows; `class-mechanics.md` and
  `caster-mechanics.md` for the tank fixes.
- **Goldens:** TestRestoration (Chain Heal), TestDisc and TestHoly (the Abacus rows); none for the Shard or the swap;
  the tank suites have no healing model, so AD, the expiry and Lay on Hands move none; Judgement of Light's heal
  moves the suites whose debuffs hold it, the hunter fix none (every suite wears a ranged weapon); report each.

### EC-TOOLS4 (Z13, moved from Z12; golden-neutral; stages: code, live)

- `docs/wave-loop/int-check.sh` takes gofmt's and eslint's files from `git diff BASE HEAD` only, so an uncommitted
  fix (a review's, the cross-review's) is never checked: add the changed files from `git status`. Confirmed; the
  Z11 cross-review ran both by hand.
- `.additem <entry> <count>` read the entry as a character guid where one exists, giving the item to that
  character; module `3d0c771` names the bot (`runner.addItem`), but which runs went without their item is unread, a
  lead. SELECT the `characters` guids among the entries those sites gave: Corpse Dust 37201 (`ecclass5_test.go`),
  Soul Shard 6265 and Infernal Stone 5565 (`strikes_test.go`), the golden setups' items (`ecclass3_test.go`), the
  fights' ammo (`fightAmmo`), the hunter recorded run's ranged fallback, quiver and ammo. A record or recording
  taken without its item is re-taken.
- Live: the tests `3d0c771` touched, none run since: `TestSimvalStrikes`, the ecclass3 and ecclass5 tests that
  call `addItem`, a hunter `TestSimvalFight` cycle, the hunter recorded run.
- **Owns:** `docs/wave-loop/int-check.sh`; the module's `addItem` call sites and the records or recordings they
  re-take; their rows.
- **Goldens:** none.

### EC-TOOLS2 (Z10, done; golden-neutral; stages: code, live)

- `db.json` has no icon rows for the explosives' spells (56350, 67769, 56488) or 55039: the UI fetches them.
- e2e: `fightBuild` kills the critters near the floor first (a star of Starfall can hit one and fail the count);
  `fightSight`'s `LINE_OF_SIGHT` branch and `spellModRun`'s weekly-reset message are unproven live; `simval fight
  -trim` keeps `UNIT_DIED` lines for critters; DruidBalancePhase3's cycle has no Starfall and its log is from the
  old spot; the other tests' dummies still stand at the entrance.
- The sim applies enchant effects of unequipped item-swap weapons, which the sheet now shows.
- **Owns:** `tools/database/gen_db`'s spell icons and `assets/database/`; the module's `e2e/fight_test.go` and
  `spellmod_sweep_test.go`; `tools/simval/fight.go`'s trim; DruidBalancePhase3's cycle and log; the item-swap
  enchants in `sim/core/`, in a small hunk.
- **Goldens:** none.

#### As built

Code stage (offline; the live stage re-records DruidBalancePhase3):
- `db.json`: `gen_db` adds `explosiveSpellIcons` (56350, 67769, 56488, 55039) after `SharedSpellsIcons`. Regenerated
  with `make items`' command through `dock.sh run` and `DBC_DIR` the loop's live DBCs: the diff is those four icon
  rows plus `db.bin`; `leftover_db.*` and `server_catalog.json` come out unchanged.
- e2e `fightBuild`: after `fightGround` and before the dummy spawns, `fightKillCritters` kills every creature within
  `fightClearYards` (40) under GM mode, logging `FIGHTCLEAR`. Vetted only (`go vet -tags=e2e`). `fightSight`'s
  `LINE_OF_SIGHT` branch and `spellModRun`'s weekly-reset message stay unproven live and unchanged.
- `simval fight -trim` drops a `UNIT_DIED` unless the victim is the player's, a minion or the dummy
  (`TestTrimFightDropsCritterDeaths`). `DruidBalance.log.gz` still holds its two critter deaths until re-recorded.
- `cycles.json`: DruidBalancePhase3 gets DruidBalance's `before` Starfall (53201). The committed log has no Starfall;
  `TestFightReplays` passes on it, so the replay only needs the live re-record.
- Item swap (`sim/core/item_swaps.go`, `character.go`): the weapon in the bag applies only the enchants in
  `swapFollowingEnchants`, those that register through the two `RegisterOnSwapItemFor...Enchant...` helpers (which
  panic on an id missing from it). Static effects (stats, threat, defense skill, weapon damage) used to count for
  the bag weapon beside the worn one's; a static enchant on the weapon swapped in now isn't applied at all.
  `TestItemSwapLeavesAStaticEnchantOfTheBagWeaponUnapplied`. Cross-review: the list missed 3273 (Deathfrost) and
  3370 (Razorice), which register their own `RegisterOnItemSwap` callback instead of a helper, so a bag weapon
  with either lost it; both are in now. The server's swap (`Player::SwapItem`, `_ApplyItemMods`) removes the old
  weapon's enchants and applies the new one's, static ones included; the sim moves stat enchants (they are item
  stats) but not an enchant effect with a static part (the Death Knight runes 3883, 3847, 3594, 3365, 3595,
  3367, 3369 and weapon enchants like 3247 or 963). No preset or suite swaps to one, so it's a follow-up: make
  those effects reversible on `RegisterOnItemSwap`, ten or so, each an inverse stat or pseudo-stat change.
- Not touched: the other tests' dummies still stand at the entrance (`newBot`, `spawnInFront`): they are other
  items' files.

Live stage (2026-10-13, server lock held 13:31 to 13:40; `ac-worldserver` untouched):
- DruidBalancePhase3 from the outer floor, `TestSimvalFight` with `SIMVAL_FIGHT_ONLY`, 171 s, PASS. `FIGHTCLEAR`
  killed 3 creatures (a Spider, two Maggots); `FIGHTSPOT` answered `SPELL_FAILED_OUT_OF_RANGE` from both spots
  (in sight, so `fightSight`'s `LINE_OF_SIGHT` branch wasn't reached); `FIGHTSTEP 53201 Starfall: sent 1, went off 1`.
  The log has 10 stars (53195), all on the dummy.
- First run failed: `fightKillCritters` left the last dead critter selected, and the build's `.simval hp 100`
  answers "Attacker and target must both be alive" on a dead selection. It now selects the bot again after the loop.
- Re-trimmed into `DruidBalancePhase3.log.gz` (no critter deaths; 2936 bytes, was 2546). `simval fight` replays it
  73 of 73, 0 dropped (Starfall aura 1 application, 9.95 s against the sim's 10 s), and `TestFightReplays` passes.
- Not reachable live: `spellModRun`'s weekly-reset message (needs the sweep to run on a reset day). `DruidBalance.log.gz`
  keeps its two critter deaths (no re-record; the replay doesn't read them). Orphan bots: none.

## Orchestrator onboarding (wave S start)

- Add waves S–X to the registry and an `EC-` row to "Where the specs are".
- The RUNBOOK's "files workstreams hands test-enrichment take master's side" now names e2e-coverage.
- Apply items' `contractChangeRequests`.
