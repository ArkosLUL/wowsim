# AzerothCore parity: investigation

Sim-vs-server differences behind `azerothcore-parity.PLAN.md`, verified in code. **[sim]** is this repo, **[ac]** is
`G:\DevStuff\GitHub\azerothcore-wotlk-pb` (branch `Custom`). Line numbers are from the [ac] checkout of 2026-09-17.
"Retail" means the behavior the sim models today (WotLK Classic research), which the fork replaces with the server's.

## Findings (sim → server)

**Attack table.**
- Sim: `sim/core/target.go:225-234` (`NewAttackTable`), `spell_outcome.go`, `spell_result.go`.
- Server: `Unit.cpp` `RollMeleeOutcomeAgainst` 2970-3169, `MeleeSpellHitResult` 3338-3509, `isSpellBlocked` 3284-3308,
  `MagicSpellHitResult` 3511-3643, `GetUnitCriticalChance` 3904-3960, dodge/parry/block bases 3816-3900.

| Mechanic vs boss-flagged lvl-83 target | Sim (Classic) | Server |
|---|---|---|
| Melee/ranged crit suppression | 4.8% | **0.6%** = (400 − 415)·0.04 (`Unit.cpp:3952`); same for melee/ranged-class spells (`Unit.cpp:9443`) |
| Spell crit suppression | 2.1% | **0** |
| Glancing | 24%, ×0.75 | **25%** = min(40, 10 + def − min(skill,max)). **×0.70** = 1 − 0.1·min(lvlDiff,3) (`Unit.cpp:1904-1920`). +1: 15% ×0.9. +2: 20% ×0.8 |
| Dodge | 6.5% | **6.45%** (world boss base 5.85 + 0.6). Non-boss lvl 83: 5.6 |
| Parry | 14% | 14% (13.4 + 0.6). Non-boss: 5.6 humanoid, else 0 |
| White dodge/parry vs expertise | linear | The +0.6 skill bonus is only added if (base − exp%) > 0, so white dodge hits 0 at 23.4 expertise and white parry at 53.6. Yellow caps: 25.8 / 56 |
| White table order | miss→dodge→parry→glance→block→crit | miss→dodge→parry→**block→glance**→crit; basis points, `roll = urand(0,10000)`, outcome when roll < running sum |
| Block | 5% inline; a block prevents crit | White 5.6%. Yellow physical: **separate `isSpellBlocked` roll, 4.4%**, front only; a hit can crit and be blocked. Block value = lvl/2 + STR/20 (41) |
| Miss / DW / spell miss | 8 / 27 / 17% | Same. Melee miss clamped to [0,60]. Spell hit = 96−d (d<3), else 94−(d−2)·11 |
| Boss level | fixed 83 | Boss flag → attacker level + 3 for skill math; DB level for glancing, level resist and the ArP cap |

Roll details the tables above don't show:
- Yellow table (`MeleeSpellHitResult`): `roll = urand(0,10000)` against cumulative miss, mechanic resist, deflect
  (ranged), dodge, parry, block (block only for `ATTR3_COMPLETELY_BLOCKED` without `CU_DIRECT_DAMAGE`). Dodge applies the
  attacker's `MOD_ENEMY_DODGE` multiplier *before* subtracting expertise; the white table does it after.
- `isSpellBlocked` passes no target to either skill lookup: attacker skill 400 vs the boss's own level 83 → 415, so
  block = 5.0 − 0.6. It's rolled in `CalculateSpellDamageTaken` only for physical melee/ranged-class damage.
- Magic hit (`MagicSpellHitResult`): `rand = irand(1,10000)`, miss when `rand < 10000 − hitBp`, so P(miss) =
  (threshold − 1)/10000, e.g. 16.99% instead of 17%. Binary non-physical, non-holy spells add
  `GetEffectiveResistChance(..., spellInfo)`·10000 to the resist threshold.
- Spell crit: `SpellDoneCritChance` then `SpellTakenCritChance`, rolled with `roll_chance_f(max(0, chance))`
  (`Spell.cpp:8476-8478`), independent of the hit roll.

**Resists.** Sim: `sim/core/spell_resistances.go:87-106`. Server: `Unit.cpp:2304-2412`, `SpellMgr.cpp:3405-3466,3597-3633`.
- Sim: avg resist = R/(C+R) + 2% per level, i.e. 6% at +3.
- Server:
  - R = max(res − spellpen, 0) + 5·levelDiff. Spell pen can't remove the level part.
  - K = 150 + (L−60)(L−67.5), with caster L = 80 → 400.
  - avg = min(R/(R+K), 0.75) → **3.614%** (×0.9639).
  - Partial resists apply when the school has no physical bit, holy only vs creatures, and the spell is neither
    `CU_BINARY_SPELL` nor `ATTR4_NO_CAST_LOG`. Buckets: p[i] = max(0, 0.5 − 2.5·|0.1i − avg|); for avg ≤ 0.1,
    p0 = 1 − 7.5·avg, p1 = 5·avg, p2 = 2.5·avg. Resisted = floor(damage·i/10).
- Binary spells:
  - Which spells count: decided in `SpellMgr.cpp:3405-3466` from the spell's effects, with exceptions (Frostbolt,
    Frost Fever, Haunt, Drain Soul). The simple reading "any non-damage effect with a value" is wrong: Mind Flay (48156,
    snare effect) is **not** binary on this server, Steady Shot (49052) and Expose Armor (8647) are. P3 takes the flag
    per spell from the spelldump (`binary`), not from a rule.
  - They get no level resist and no partial resists; their resist chance folds into the hit roll.
  - The sim flags only Ret Aura and Thorns as binary.
- Holy gets the level resist vs creatures. Sim bug: `ResistanceStat()` returns index 0 (= Strength) for holy (`flags.go:238`).

**Armor, ratings, base stats**
- Armor mitigation and the ArP cap use the same expression, `467.5·L - 22167.5`, but at different levels: mitigation
  takes the attacker's, the cap the victim's (`Unit.cpp:2218-2302`). The sim used the attacker's for both, capping at
  15232.5 instead of 16635 against a level 83 target; fixed in P2. Damage after armor is `ceil`-rounded.
- Boss block value is `Creature::GetShieldBlockValue` = level/2 + STR/20, and `creature_classlevelstats` gives level 83
  creatures 0 strength, so 41. The sim hardcoded 76.
- ArP rating = 15.3953 / class scalar 1.1 = 13.9957; the sim hardcoded 13.99. Percent-ArP auras (Battle Stance, Mace
  Specialization) add to the rating's percentage before the cap, so the sim can't fold them into rating per class.
  Blood Gorged's class mask (`SpellInfoCorrections.cpp`) limits it to white swings and Plague, Blood, Heart, Death
  and Rune Strike.
- Clean gt DBCs match the sim's rating constants: hit 32.79, spell hit 26.232, crit 45.906, hybrid melee haste 25.223, expertise 8.1975, spirit regen 0.003345.
  Class scalars: melee haste 1.3 for Paladin, DK, Shaman and Druid, ArP 1.1 for everyone, every other rating 1.
- Level-80 `player_class_stats`: Shaman HP 6939 (sim 6960), Warlock 7136 (sim 7164); DK base mana 0 (sim 1000).
- Expertise and hit are floats on the server; the sim is already continuous.
- Primary stats are truncated to integers (`Player::UpdateStats`), and everything derived reads the integer. The sim
  keeps them continuous.
- Avoidance diminishes per class (`StatSystem.cpp:711-838`): k 0.956 (Warrior, Paladin, DK), 0.988 (Hunter, Rogue,
  Shaman), 0.983 (Priest, Mage, Warlock), 0.972 (Druid); dodge caps 88.13/145.56/150.38/116.89, parry caps
  47.00/145.56 and none for casters and druids, miss cap 16. The sim had one druid and one non-druid set.
- Dodge per agility is crit per agility × `crit_to_dodge` (`Player.cpp:5285-5336`), and the base agility's share
  doesn't diminish; the sim diminished all of it. Defense rating's bonus is truncated to whole skill points before it
  reaches dodge, parry, miss or block.

**Swing and cast timing**
- Each 100 ms map update throws away swing-timer overshoot, about 50 ms lost per swing.
- A landed swing pushes the other hand to at least 200 ms (`PlayerUpdates.cpp:159-231`).
- A melee swing resets the ranged timer.
- Instant specials reset the swing timer only when the spell has interrupt flags and no `ATTR2_DO_NOT_RESET_COMBAT_TIMERS` (and similar) (`Spell.cpp:8188-8202`).
- Ranged-slot non-autorepeat spells get **+500 ms** cast time before haste (`SpellInfo.cpp:2768`).
- Ranged-class cast time scales with ranged attack speed.
- GCD is clamped to [1000,1500] ms and hasted only for category-133 magic spells (`Spell.cpp:8955-9015`).
- Missile delay = max(dist, 5)/speed.

**Rage and procs**
- `Unit::RewardRage` (`Unit.cpp:16128-16158`): hit factor = `uint32(unhastedSpeed·3.5 | 1.75)` (`Unit.h:921-924`),
  which a crit doubles after the truncation; conversion at level 80 is 453.32217 (Classic hardcodes 453.3); each
  gain is floored to a tenth of rage (`uint32(addRage·10)`), which the sim doesn't model.
- PPM chance = unhasted weapon ms·PPM/600. For spell-triggered aura procs (`Aura::CalcProcChance`,
  `SpellAuras.cpp:2272-2305`) the server uses max(base cast, 1500 ms) unless the spell is melee-class or a ranged
  weapon spell. The weapon it reads belongs to the **aura's caster**, so a raid's Judgement of Wisdom measures
  against the judging paladin; the sim uses the attacker's. Item and enchant procs always use the weapon's attack
  time (`Player.cpp:7460-7575`).
- Judgement of Wisdom procs off its target debuff 20186 (15 PPM, no flat chance), and a taken proc's default hit
  mask is normal + critical, so a miss, dodge, parry or full block never reaches the roll (`SpellMgr.cpp:923-942`).
  Its `SpellTypeMask` is damage, and a hit that dealt none counts as `PROC_SPELL_TYPE_NO_DMG_HEAL`
  (`Unit.cpp:6856-6864`), so applying a dot doesn't proc it either. Its mana only goes to an attacker whose
  **current** power type is mana (`spell_paladin.cpp:1372-1375`), so a shapeshifted druid gets none.
- `spell_proc`'s `SpellPhaseMask` decides when an equip aura fires. On the cast (`Spell.cpp:3975-4004`) there is no
  damage info, so misses count and PPM never applies: Elemental Focus Stone (65005), Black Magic (59630). On the hit:
  Flare of the Heavens (64714), Show of Faith (64738), Sif's Remembrance (65002), Lightweave (55640), Darkglow (55768).
- `PROC_ATTR_REDUCE_PROC_60` multiplies proc chance by 1/3 at level 80.
- Enchant PPMs (`spell_enchant_proc_data`):

  | Enchant | Server PPM | Sim PPM |
  |---|---|---|
  | Mongoose | 1 | 0.73 |
  | Icebreaker | 3 | 4 |
  | Deathfrost | 3 | 2.15 |
  | Berserking, Crusader, Executioner | 1 | — |

- Item proc chance and ICDs come from live `spell_proc`.

**DoTs and periodic ticks**
- Ticks keep the lattice they started on: `AuraEffect::Update` (`SpellAuraEffects.cpp:926-954`) counts the periodic
  timer down by each map update and carries the leftover, so tick k lands on the first server tick at or after
  start + k·amplitude. The aura's expiry rounds the same way, so a channel ends with its last tick.
- Amplitude and duration are whole ms, and the tick count is maxDuration/amplitude truncated (`GetTotalTicks:7074`).
  `AuraEffect::Update` stops there, and an extension raises maxDuration, so it buys ticks (Glyph of Starfire).
- Haste on a channel scales the amplitude (`Unit::ModSpellCastTime`) and the channel's duration
  (`Spell::handle_immediate`) alike, so its tick count holds. On any other dot,
  `ATTR5_SPELL_HASTE_AFFECTS_PERIODIC` or a `SPELL_AURA_PERIODIC_HASTE` aura shortens the amplitude alone
  (`CalculatePeriodic:650`), and the unchanged duration then fits more ticks; only the rolling refresh
  `Aura::RefreshTimersWithMods` (`SpellAuras.cpp:880`) hastes the duration too.
- Live, `DoEffectCalcPeriodic` scripts in mod-spell-tweaks (list below) carry that haste, not the DBC flag, and
  they clamp the multiplier at 1: a slow never stretches the ticks, as the core's own path would.
  `Dot.TickHaste` stays `SpellHasteScalesBoth` until a class item says otherwise.
- A refresh resets the tick timer only when StackAmount < 2 and the cast isn't triggered
  (`Spell::DoSpellHitOnUnit`, `Spell.cpp:3146`); a periodic aura that isn't `PERIODIC_DAMAGE`/`_PERCENT` resets
  either way. `TRIGGERED_NO_PERIODIC_RESET` sits outside `TRIGGERED_FULL_MASK` (`SpellDefines.h:150-152`), so
  nothing but a GM's `.cast triggered` sets it and StackAmount decides alone. Every stacking dot the sim registers
  (Holy Vengeance 31803, Lacerate 48568, Deadly Poison 57970, Impale 66331, Chilled to the Bone 70106) already
  refreshed in place, so the rule moved no results.
- Ticks crit only with aura 286 or on Rupture. `Dot.TicksCanCrit` records that per dot, and
  `periodicCritsNeedDeclaration` (`spell_outcome.go`) enforces it once every class has declared its crit-capable
  dots (P8).
- Ignite munching: `MunchingBlizzlike = 1`.

**Pets**
- Scaling scripts:
  - hunter pet: 22% RAP→AP, 12.87% RAP→SP (`spell_hunter.cpp:215-247`)
  - ghoul: 70% Str / 30% Sta + owner melee haste (`spell_dk.cpp:792-845`)
  - gargoyle: 75% AP→SP
  - Feral Spirit: 30% AP (`spell_shaman.cpp:234-254`)
  - warlock pets: 57% SP→AP, 15% SP (`spell_warlock.cpp:462-480`)
  - Shadowfiend: `spell_priest.cpp:102-137`
  - Treants: `spell_druid.cpp:380-419`
  - Water Elemental: `spell_mage.cpp:266-296`
- Owner hit, spell hit and expertise come from a scaling aura `Guardian::InitStatsForLevel` hands out
  (`spell_generic.cpp` `spell_pet_hit_expertise_scalling` ~520-595,
  `spell_pet_spellhit_expertise_spellpen_scaling` ~5540-5587). Each amount is the owner's hit chance over its
  cap times its own scale, truncated to a whole point, and nothing caps the result, so 10% ranged hit is 21%
  spell hit and 32 expertise. Which aura a pet carries decides the rest:
  - 61017/61013, everything but the DK's summons — hunter and warlock pets, both water elementals, the fire
    and earth elementals, treants, the shadowfiend, Feral Spirits, mirror images, bloodworms, the infernal
    and the doomguard: hit 8, spell hit 17, expertise 26, off ranged hit over a cap of 8 for a hunter owner,
    spell hit over 17 for one whose power type is mana (shamans and paladins included), else melee hit.
  - 67561, every risen ghoul plus the gargoyle and the army: spell hit 17 and expertise 26 off the owner's
    melee hit over a cap of 8, and **no melee hit at all**. The dancing rune weapon carries neither aura, so
    it inherits none of the three.

  Only `IsPet()` recalculates, every 3 s; a guardian keeps what it got at summon.
- Armor penetration (`Unit::CalcArmorReducedDamage`): a hunter pet and any risen ghoul use the owner's rating
  and percent-ArP auras, each aura tested against the pet's own spell. A class mask no pet ability matches
  still reaches the pet's white swings, which carry no spell for it to fail against, so Blood Gorged applies
  to them.
- The haste carrier auras (`spell_dk_pet_scaling`, mod-spell-tweaks' 425790 and 425792) hold the owner's
  attack speed as whole percent, recalculated every 2 s: ranged for a hunter pet, melee for the ghoul and
  Feral Spirits. They make the pet immune to positive `MOD_CASTING_SPEED_NOT_STACK`, `MOD_MELEE_RANGED_HASTE`
  and `MELEE_SLOW`, so Bloodlust is blocked and reaches the pet through the owner instead. 425790 and
  `spell_dk_pet_scaling` leave `MOD_MELEE_HASTE` alone, so Frenzy and Ghoul Frenzy stack on top; 425792 blocks
  it too, since Windfury Totem and Improved Icy Talons reach the wolves as party auras and are already in the
  shaman's melee haste.
- Left open in P7-0b, all of it needing a class-side marker core doesn't have:
  - Core hands every pet the 61017 amounts, so the DK's ghouls, gargoyle, army and rune weapon carry melee
    hit the server denies them.
  - Only the permanent ghoul inherits ArP, since core can't tell the temporary Raise Dead ghoul from an army
    ghoul. That also keeps Blood Gorged off every pet swing, because the talent needs a Blood build while
    only an Unholy DK's ghoul is permanent: core hands the owner's white-swing bonus to the pet's swings,
    but no preset can reach both halves.
  - The hunter pet's inherited haste is continuous, not resnapshotted every 2 s, and the sim blocks Bloodlust
    on an inheriting pet by ignoring `MultiplyAttackSpeed`, its closest match for `MOD_MELEE_RANGED_HASTE`.
- The sim's pet "+1.8% crit" hacks (`hunter/pet.go:140`, `shaman/fire_elemental_pet.go:151`, `shaman/spirit_wolves.go:45`) are Classic-only.

**Hunter haste** (measured, `TestSimvalHunter`)
- The sim gives hunters ×1.15 ranged speed (`sim/hunter/hunter.go:165`). The server core has no such bonus.
- mod-individual-progression recasts spell 89507 on login (aura 141 `MOD_RANGED_AMMO_HASTE`, −15%, bows and crossbows)
  and gives quivers and ammo pouches their equip spells back
  (`mod-individual-progression/data/sql/world/base/vanilla_item_changes.sql`).
- Measured ranged speed multipliers of a night elf hunter after relogging with each gear set:

  | Gear | `m_modAttackSpeedPct[RANGED]` |
  |---|---|
  | Worn Shortbow + Light Quiver (2101, spell 29418 +10%) | 1/1.10 |
  | Zod's Repeating Longbow + Light Quiver | 1/1.10 |
  | Zod's Repeating Longbow + Nerubian Reinforced Quiver (44448, spell 29414 +15%) | 1/1.15 |
  | Dwarf, Old Blunderbuss + starting ammo pouch (spell 14824 +10%, guns) | 1/1.10 |

- The 89507 aura is always present but never changes the speed. Ranged haste is therefore just the quiver or pouch:
  the sim's ×1.15 is right only with a +15% quiver (level 75+), and a hunter without one has none.

**Server customizations (live values)**
- mod-spell-tweaks (`[ac]/modules/mod-spell-tweaks`, the user's repo; every toggle on, exotic pet damage 0):
  - **Haste adds ticks** (interval scaled, duration fixed):
    - Frost Fever/Blood Plague with Epidemic
    - Deadly Poison with Murder
    - Rupture with Weapon Expertise
    - Rend with Trauma
    - Moonfire/Insect Swarm with Eclipse (spell haste; the others use melee haste)
  - **DoT ticks can crit:**
    - Moonfire/IS (Earth and Moon)
    - Blood Plague (Crypt Fever)
    - Frost Fever (Runic Power Mastery; FF becomes magic damage class)
    - Holy Vengeance/Blood Corruption (2H Weapon Spec)
    - Righteous Vengeance
    - Deadly Poison (Murder)
    - Rend (Trauma, crit snapshotted)
  - **Ability changes:**
    - Crusader Strike and Divine Storm apply a seal stack.
    - Glyph of Reckoning is unimplemented, so Hand of Reckoning never deals damage (67485).
    - Titan's Grip has no damage penalty.
    - Faerie Fire (Feral) on NPCs always grants Clearcasting with Omen.
    - Explosive Trap works via Trap Launcher.
  - **Pets:**
    - Hunter pets inherit owner ranged haste and ArP.
    - Feral Spirits inherit melee haste and ignore other melee haste auras.
    - The ghoul inherits ArP (core edit in `Unit::CalcArmorReducedDamage`, gated by `CONFIG_DK_GHOUL_ARMOR_PEN`).
  - **Talent values:**
    - Virulence 2/4/6% spell hit
    - Nerves of Cold Steel 2/4/6% DW hit
    - Rage of Rivendare 2–10 expertise
    - Hunter's Mark off GCD
    - Beast Mastery +6 pet talent points
    - Ghoul/Pet Avoidance reduce all damage
- mod-reforging (`modules/mod-reforging/src/item_reforge.cpp:254-373`, `npc_reforger.cpp:103,141-146`):
  - Moves 40% (floored, at least 1) of one base template stat to another.
  - Both stats must be in {6 Spirit, 13 Dodge, 14 Parry, 31 Hit, 32 Crit, 36 Haste, 37 Expertise}, and the target can't already be on the item.
  - One reforge per item, fewer than 10 item stats, and not on gems, enchants or random suffixes.
- mod-dungeon-scale:
  - Instance creatures get Health, Armor and Damage multipliers from config (`DungeonScale.cpp:4957-4980, 5095-5096`), with separate Global and per-stat values for raid/raid heroic and for bosses.
  - Live (`configurationOverrides/DungeonScale.env`): boss Health 1.2, Mana 1.3, Damage 1.0; Armor and Global unset (1.0). These values are expected to change.
  - `DungeonScale.DisabledID` (entries skipped entirely) includes the sim dummies 999000-999003.
  - Player damage is unscaled.
- mod-racial-trait-swap swaps racial abilities only; base stats stay.
- Module hooks that run inside `CalculateMeleeDamage` (`ModifyMeleeDamage` in mod-dungeon-scale, mod-dungeon-master,
  mod-individual-progression) only scale damage; no module implements `OnBeforeRollMeleeOutcomeAgainst`.

**Items** (`docs/azerothcore-item-diff/data/summary.md`)
- 1509 of 8043 sim items differ.
- Classic raised Ulduar/emblem item levels, e.g. 226→232 on 329 items and 239→252 on 92.
- Trinket values differ: Mjolnir Runestone 45931 is 665 ArP on the server vs 751 in the sim; Flare of the Heavens 45518 is 850 vs 959.

## Verified on the live server

`[ac]/modules/mod-sim-validation/e2e` (`TestSimvalWarrior`, `TestSimvalHunter`) ran every probe inside Naxxramas and
checked each record: rolled rates within 5 standard errors of the server's own thresholds (70 checks), plus these fixed
values. Human warrior, level 80, maxed skills, Worn Shortsword (Sword Specialization: 3 expertise = 75 bp).

- Boss dummy, in front: miss 800, dodge 645 − 75, parry 1400 − 75, block 560, glancing 2500 bp; crit = sheet − 0.6.
- Boss dummy, behind: parry and block 0, dodge unchanged.
- Level-83 elite humanoid: dodge = parry = 560 − 75, block 560, glancing 2500. Beast: parry 0. Level-80 dummy: miss 500,
  glancing 0, crit = sheet (no suppression).
- Heroic Strike: partial block 4.40% in front, 0 behind; yellow dodge/parry/miss as the white table without the DW term.
- Creature vs player (`.simval taken`, boss vs naked warrior, defense 400): miss 469 bp (5% − 15·0.02, the penalty per
  skill point is 0.02 when the attacker's skill is higher), dodge and block = player value − 60 bp, crit 560 bp, no crushing.
- Frostbolt vs boss: miss threshold 1700 → 16.99%; resist buckets match avg 15/415 = 3.6145% (0%: 72.89, 10%: 18.07,
  20%: 9.04); mean resisted 3.614%. Vs level 80: threshold 400, no resists.
- Armor: every scenario matches the formula with Battle Stance's 10% ArP capped by the victim's level. Sunder Armor ×5
  (debuff 58567; the ability 47467 triggers it) and Expose Armor (8647) don't stack, Faerie Fire (770) multiplies:
  10643 → 8514 → 8088.
- Boss dummy health inside the instance is the template's 24,009,944, so `DungeonScale.DisabledID` keeps it unscaled.
- Proc chances (`TestSimvalProcPPM`, `.simval procs` with the auras applied by GM command, 1.9 s main hand):
  Judgement of Wisdom (20186, 15 PPM) reads 47.500% for a white swing and for Heroic Strike, 75.000% for Frostbolt's
  3 s base cast and 50.000% for Steady Shot's 2 s ranged slot. Hand of Justice (15600, 1 PPM with
  `REDUCE_PROC_60`) reads a third of the same basis: 1.056% and 1.667%.
- Base stats (`TestSimvalBaseStats`, one naked level-80 character per class over all ten races): primary stats, max
  health, armor, AP, ranged AP, melee and spell crit, real dodge and miss taken match the sim's generated base stats
  exactly, allowing only for the server truncating stats. With 400 defense rating (81 skill) and 512 dodge rating the
  diminished dodge and miss match too; ArP rating converts at 13.9957 per 1% for every class, and 1498 caps.
  Characters leveled by GM command never learn Parry (3127), so their sheet shows none until taught it.

## Retail deviations

Places where AzerothCore's core (not the user's modules) behaves differently from the retail model the sim uses today.
The fork copies the server. Patching any of these in [ac] means updating the matching sim function and its test.

| # | Mechanic | Server | Retail | Server code |
|---|---|---|---|---|
| 1 | Melee/ranged crit suppression vs +3 boss | 0.6% | 4.8% | `Unit.cpp:3952`, `9443` |
| 2 | Spell crit suppression vs +3 boss | none | 2.1% | `Unit.cpp:9172-9260` |
| 3 | Glancing vs +3 | 25% chance, ×0.70 | 24%, ×0.75 | `Unit.cpp:3121`, `1908-1911` |
| 4 | Boss dodge | 6.45% | 6.5% | `Unit.cpp:3825` |
| 5 | White dodge/parry vs expertise | skill bonus dropped once expertise exceeds the base, caps 23.4 / 53.6 expertise | linear, caps 26 / 56 | `Unit.cpp:3051-3053`, `3083-3085` |
| 6 | White table order | block before glancing | glancing before block | `Unit.cpp:3092-3128` |
| 7 | Yellow block | separate `isSpellBlocked` roll that doesn't stop crits, skill term sign flipped (4.4% vs boss) | in the attack table, a block can't crit | `Unit.cpp:3298` |
| 8 | Level-based spell resistance vs +3 | +15 resistance → 3.61% average | 6% average | `Unit.cpp:2325`, `2339` |
| 9 | Magic miss roll | P(miss) = (threshold − 1)/10000 | threshold/10000 | `Unit.cpp:3596-3599` |
| 10 | Rage weapon speed factor | truncated to an integer | continuous | `Unit.h:921-924` |
| 11 | Swing timer | 100 ms map update drops overshoot (~50 ms per swing) | overshoot carried over | `PlayerUpdates.cpp:159-231` |
| 12 | Pet hit, spell hit, expertise | owner values floored to whole percent | exact | `spell_generic.cpp` ~520-595, ~5540-5587 |
| 13 | Hunter base ranged haste | not in core (mod-individual-progression substitutes spell 89507 plus quivers) | ×1.15 | `IndividualProgression.cpp:106-118` |
| 14 | Bulwark of Azzinoth armor proc (40407) | 6 PPM on melee and ranged hits taken, at the wearer's weapon speed (~26% at 2.6 s) | 2% per hit taken (Spell.dbc) | `spell_proc.sql:513` |
| 15 | Ashen Band of Courage armor proc (72415) | 3%, no ICD: no `spell_proc` row | 3%, 1 min ICD (Classic's spell data) | `SpellMgr.cpp:2284` |
| 16 | Cast completion | on the first 100 ms map update after the cast time, which also starts the CD (~50 ms per hardcast) | exact | `Spell.cpp:4422-4435` |
| 17 | Aura expiry | on the first map update after the duration | exact | `SpellAuras.cpp:747-753` |
| 18 | GCD haste | only for 1.5 s category-133 spells that aren't melee, ranged, ranged-slot or `ATTR0_IS_ABILITY`: Feral Spirit hasted; Dispersion, Volley, Fire Elemental Totem and the imp's Firebolt not | per spell, the other way round for those five | `Spell.cpp:8991-9008` |
| 19 | Heroic Throw and Shattering Throw swing reset | every hand restarts in full, then the 200 ms push; glyphed (instant) Shattering Throw doesn't reset | off hand half a swing later when both weapons have the same speed; glyphed Shattering Throw resets too | `Spell.cpp:3955-3966`, `4039-4056`, `8188-8202` |
| 20 | Judgement of Wisdom PPM basis | max(base cast, 1500 ms) for a spell, the weapon's attack time for a melee-class or ranged weapon spell, read on the judging paladin | the attacker's hasted cast time, 0.75 s when instant, and its own weapon | `SpellAuras.cpp:2272-2305` |
| 21 | Judgement of Wisdom on a miss | no proc | white and ranged hits proc on a miss | `SpellMgr.cpp:923-942` |
| 22 | Rage per hit | conversion 453.32217 at level 80, and each gain floored to a tenth of rage | 453.3, unrounded | `Unit.cpp:16128-16158` |
| 23 | Equip proc phase | `spell_proc`: Elemental Focus Stone (65005) and Black Magic (59630) fire on the cast, misses counted; Flare of the Heavens (64714), Show of Faith (64738), Sif's Remembrance (65002), Lightweave (55640) and Darkglow (55768) on a landed hit | the other way round, except Lightweave, which is on the hit there too | `spell_proc` |
| 24 | DoT tick times | on the first map update at or after the nominal time, with the leftover carried into the next tick | exact | `SpellAuraEffects.cpp:926-954` |
| 25 | Tick interval and count | whole ms, count = maxDuration/amplitude | fractional interval, the declared tick count | `SpellAuraEffects.cpp:650`, `:7074` |
| 26 | Haste on a dot that isn't channeled | shortens the interval only, so the fixed duration gains ticks | interval and duration scale together | `SpellAuraEffects.cpp:650`, `SpellAuras.cpp:880` |
| 27 | DoT tick crits | only with `SPELL_AURA_ABILITY_PERIODIC_CRIT` (286) or on Rupture | any dot can crit | `AuraEffect::CanPeriodicTickCrit` |

Not yet settled against retail, check before patching: the 200 ms other-hand push (`PlayerUpdates.cpp`), the DoT
refresh tick-timer rule, the max(cast, 1500 ms) PPM basis for spell-triggered aura procs, the rule-based binary
spell list (`SpellMgr.cpp:3405-3466`), and the 5 yard missile floor, which `Spell::AddUnitTarget` calls a hack.
