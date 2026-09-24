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
- ALWAYS_HIT returns before the yellow table rolls at all, not just before active defense, and
  `isSpellBlocked` exempts it and NO_ACTIVE_DEFENSE from the partial block roll. `rollYellow` and
  `OutcomeMeleeSpecialCritOnly` read the flag now (PAR-P7-0e). **Fixed (PAR-P7-SHA):** Stormstrike's two
  hits now deal damage under their own ids, 32175 (MH) and 32176 (OH), which carry both flags; 17364
  still rolls the yellow table once, for the debuff. Before, both hits rolled under 17364 too, which
  carries neither flag, so they still took a partial block from the front.

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
- **Fixed (PAR-P7-0d):** `pauseMelee` shifted a Slam-frozen mh/oh timer forward by a fixed offset, so a
  haste change mid-freeze rescaled that offset along with the real time left on the timer.
  `suspendMelee`/`resumeMelee` freeze it the way `SuspendRangedTimer` already does for Volley
  (`suspendedLeft`, rescaled by `UpdateSwingTimers`), and `cast.go` calls them for any
  `ATTR2_DO_NOT_RESET_COMBAT_TIMERS` hardcast instead.
- **Fixed (PAR-P7-0d):** the main hand's last-moment APL check (`swing()`'s `replaceSwing` branch, for
  Heroic Strike's queue) can cast something that cancels auto attacks outright (a shapeshift, Vanish,
  Army of the Dead), and the in-flight swing still landed. `swing()` now bails out once
  `AutoAttacks.enabled` goes false there.
- **Fixed (PAR-P7-0d):** a channel not driven by `Spell.Cast`'s own `CastTime` — a periodic aura, like
  Army of the Dead — held no melee at all; only its own class workaround did.
  `Spell.HoldMeleeUntil`/`ReleaseMeleeHold` give it the same `Hardcast`-driven hold a hardcast gets
  (frozen if the spell's own ATTR2 says so, held otherwise), and Army of the Dead's
  `CancelAutoSwing`/`EnableAutoSwing` pair is gone. The signal is the server's own `FlagChanneled` bit
  (`spell.timing`), not the sim's `SpellFlagChanneled`: Bladestorm keeps that flag only to gate the
  GCD (`serverdata_allowlist.go`'s allowance says so, "modeled as a channel so nothing else is cast
  during it"), since the server runs it as a plain periodic aura with no `UNIT_STATE_CASTING`, so it
  stays outside this hold. Volley needed no change: the sim gives hunters no melee auto-attack
  (`AutoSwingMelee` unset), so its existing `SuspendRangedTimer`/`ResumeRangedTimer` pair is all it
  ever exercised.

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
- `Unit::CastDelayedSpellWithPeriodicAmount`'s refresh (`MunchingBlizzlike.Enabled`, live on) queues on
  the caster's own 400 ms event-clock boundary (`EventProcessor::CalculateQueueTime`) whenever caster
  and target differ, and the old dot keeps ticking meanwhile; two procs inside one window read the
  same stale dot, so the later application overwrites the earlier one's (munching).
  `core.DelayedPeriodicApplier` (PAR-P7-0e) draws that boundary's phase once per caster per iteration,
  since the sim has no equivalent of the caster's own creation-time clock. On it: Deep Wounds, Unholy
  Blight, Piercing Shots, Righteous Vengeance, Ignite (PAR-P7-MAG), Languish (PAR-P7-DRU), Electrified and
  Lava Burst's bonus dot (PAR-P7-SHA): every proc the server delays.
  - The proc counts the old dot's outstanding damage, so an old tick landing before the refresh is paid
    twice. A tick or swing due on the refresh's own server tick comes after it: `Player::Update` runs the
    caster's events before its swings, and `Map::Update` updates players before the creatures whose auras
    tick. The sim lands the refresh 1 ns early for that; ticking and swinging first had put Righteous
    Vengeance 8% and Fury's Deep Wounds 11% high (H3 cross-review).
  - PAR-P7-MAG (I): the Ret capture's 78 Righteous Vengeance refreshes sit on one 400 ms lattice,
    averaging 189 ms after the crit that fed them. Isolating the two unambiguous feeders (Crusader
    Strike and Divine Storm, 33 crits total) gives a weighted mean of 176 ms; the third feeder the sim
    credits, the Seal of Vengeance proc (42463), paired inconsistently with nearby crits and wasn't
    used for the mean. `Delay()` was landing a mean of 250 ms (100-400 ms): its own 400 ms boundary was
    right, but it then rounded the landing up to the sim's general server tick a second time. That
    second rounding has no server counterpart here: `Unit::CastDelayedSpellWithPeriodicAmount` queues
    onto the *caster's own* `m_Events`, which `WorldObject::Update` advances on every `Map::Update` pass
    for that player, not gated to the slower ~100 ms `MapUpdateInterval` that throttles
    `UpdateNonPlayerObjects` (where a dot tick on the target actually runs). Dropping it moves `Delay()`'s
    mean to 200 ms, matching the capture within normal sampling noise for 33 crits off a uniform
    (0, 400 ms] draw.
  - The capture's casts were a bot's, which matters for a second reason: mod-playerbots casts through
    `PlayerbotAI::CastSpell` -> `Spell::prepare`, called synchronously from `PlayerbotAI::UpdateAI`,
    hooked to `OnPlayerAfterUpdate` at the very end of `Player::Update`, after `Unit::Update`'s
    `WorldObject::Update` has already advanced `m_Events` for that pass. A bot's own cast sees the same
    up-to-date clock as a swing or a hardcast landing in `_UpdateSpells`, so the fixed `Delay()` already
    covers it. A *human* player's instant cast differs: `CMSG_CAST_SPELL` is `PROCESS_THREADSAFE`
    (`Opcodes.cpp`), handled by `WorldSession::Update` in `Map::Update`'s session pass, one pass before
    that same player's own `Player::Update` runs. Its boundary is drawn off the clock as it stood one
    pass ago, so it can already be behind "now" by the time the pass catches up, landing sooner, in the
    same update at the earliest. `DelayFromInstantCast()` models that. Righteous Vengeance's Crusader
    Strike/Divine Storm feeders and Ignite's instant-cast feeders (Living Bomb's direct hit, and
    Pyroblast/Frostfire Bolt made instant by Hot Streak/Brain Freeze, read off `spell.CurCast.CastTime
    == 0`) use it; everything else, including Righteous Vengeance's Seal of Vengeance feeder (a weapon
    swing proc, not a cast), stays on `Delay()`.
  - A live Fire mage capture (Findings, Mage) checks both `Delay()` and `procIgnite`'s tick-damage math against
    ~80 real Ignite applications; both hold.

**Pets**
- Scaling scripts:
  - hunter pet: 22% RAP→AP, 12.87% RAP→SP (`spell_hunter.cpp:215-247`)
  - ghoul: 70% Str / 30% Sta + owner melee haste (`spell_dk.cpp:792-845`)
  - gargoyle: 75% AP→SP
  - Feral Spirit: 30% AP (`spell_shaman.cpp:234-254`)
  - warlock pets: 57% SP→AP, 15% SP, both floored to a whole point (`CalculatePct` on an `int32`,
    `spell_warl_generic_scaling`/`spell_warl_infernal_scaling` in `spell_warlock.cpp:335-360,460-486`)
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
    melee hit over a cap of 8, and **no melee hit at all**. The dancing rune weapon gets 61017 from its AI
    (`npc_pet_dk_dancing_rune_weapon`). The owner's hit chance is `m_modMeleeHitChance`, so Nerves of Cold
    Steel and Heroic Presence count.

  Only `IsPet()` recalculates, every 3 s; a guardian keeps what it got at summon.
- Armor penetration (`Unit::CalcArmorReducedDamage`): a hunter pet and any risen ghoul use the owner's rating
  and percent-ArP auras, each aura tested against the pet's own spell. A class mask no pet ability matches
  still reaches the pet's white swings, which carry no spell for it to fail against, so Blood Gorged applies
  to them.
- The haste carrier auras (`spell_dk_pet_scaling`, mod-spell-tweaks' 425790 and 425792) hold the owner's
  attack speed as whole percent, recalculated every 2 s: ranged for a hunter pet, melee for the ghoul and
  Feral Spirits. They make the pet immune to `MOD_CASTING_SPEED_NOT_STACK`, `MOD_MELEE_RANGED_HASTE`
  and `MELEE_SLOW` of either sign (`Unit::ApplySpellImmune` drops the positive-only block type), except from
  spells with `ATTR0_NO_IMMUNITIES`, `ATTR3_ALWAYS_HIT` or `ATTR4_OWNER_POWER_SCALING`. So Bloodlust, both
  halves, is blocked and reaches the pet through the owner instead. 425790 and
  `spell_dk_pet_scaling` leave `MOD_MELEE_HASTE` alone, so Frenzy and Ghoul Frenzy stack on top; 425792 blocks
  it too, since Windfury Totem and Improved Icy Talons reach the wolves as party auras and are already in the
  shaman's melee haste. **Fixed (PAR-P7-SHA review):** Spirit Wolves now carry `Unit.HasteCarrier` too
  (`spirit_wolves.go`). **Fixed (wave I cross-review):** `Pet.OwnerHasteCoversMeleeHasteAuras` marks the
  carriers that block `MOD_MELEE_HASTE`, and `applyPetBuffEffects` (`sim/core/buffs.go`) strips
  `WindfuryTotem`/`IcyTalons` for those; the hunter pet and the ghoul still take both directly.
- A pet's melee crit is a flat 5% plus crit auras (`Unit::GetUnitCriticalChance`), nothing from agility,
  and a creature's spell crit `m_baseSpellCritChance`, 5%. The DK's summons, the hunter pet and now the
  warlock's demons and infernal (PAR-P7-WLK) follow it; treants and spirit wolves still take agility crit
  in the sim, pending their own class items.
- DK summons (`Guardian::InitStatsForLevel`, `pet_dk.cpp`, `spell_dk.cpp`); core tells 67561 and risen
  ghouls apart by `Pet.HitScaling` and `Pet.RisenGhoul`, which the DK sets:
  - Risen ghoul (26125, pet or guardian): pet_levelstats' 4665 health (+10 a point of Sta over 361), 331
    Str, 247 Agi, 361 Sta, and `IsPetGhoul`'s AP of 589 + Str + Agi. It swings every 2.0 s for AP/14 with no
    weapon damage: the pet's pet_levelstats 0-0, the guardian's the 0.13-0.20 `Creature::SelectLevel` rolled
    at its template level 1. It inherits 70% of Str and 30% of Sta (Ravenous Dead +20% a rank, Glyph of the
    Ghoul +40), whole percent of whole stats. Risen Ghoul Self Stun (47466) holds it 4.5 s after the summon.
    The guardian runs `npc_pet_dk_ghoul`'s CombatAI, casting Claw every 5000 + rand() % 5000 ms from its
    first attack; the pet's PetAI casts it from 75 energy.
  - Army ghoul (24207): no pet_levelstats row, so 22 Str and Agi and 2 × Str − 20 AP, plus Army of the Dead
    Passive's 6.5% of the owner's AP; a 60-100 weapon at 2.0 s; AggressorAI, so it never Claws.
  - Bloodworm (28017): the same fallback stats and AP, a 2.66 s weapon of 30-70 plus 0.6% of the owner's AP
    at the summon, and no DK pet scaling. 49543 summons 2 to 4, evenly.
  - Gargoyle: Gargoyle Strike is 51-69 + 3 a level over 60, plus 0.453 of its spell power, which is 75% of
    the owner's AP (Impurity +4% a rank on the 75), both truncated. `npc_pet_dk_ebon_gargoyle` decides every
    400 ms from the summon: from 2 s in and once it has landed, it starts a cast at 80% when idle. 32 s in it
    flies off, cutting its cast.
  - A guardian's `spell_dk_pet_scaling` haste is fixed at the summon, and `MELEE_SLOW` hastes the gargoyle's
    cast time too. The army is immune to Bloodlust by id. Of the DK's summons only risen ghouls and the
    gargoyle get the orc's Command (65221).
- **Correction (PAR-P7-0d):** the line above said the hunter pet's inherited haste is continuous.
  It's the carrier aura itself that ticks every 2 s (`CalcPeriodic`'s amplitude on 425790/425792/51996
  alike), so `Pet.OwnerHasteSource` now resnapshots on that cadence too, real pet only, same as the
  owner-hit scaling's 3 s (`petOwnerHasteRefreshInterval`, `pet.go`). A guardian still only takes the
  one snapshot at the summon, since `CalcPeriodic` gates the reschedule on `IsPet()`.
  The sim blocks Bloodlust on an inheriting pet by ignoring `MultiplyAttackSpeed`, its closest match for
  `MOD_MELEE_RANGED_HASTE`. The carrier blocks positive `MELEE_SLOW` too, and the sim's two buffs of that
  type are build-phase multipliers rather than `MultiplyAttackSpeed` calls, so that guard missed them:
  Improved Moonkin Form (50170-50172) and Swift Retribution (Retribution Aura 54043's third effect)
  reached an inheriting pet both directly and through the owner's ranged speed, +3% twice.
  `applyPetBuffEffects` now strips them the way it strips Bloodlust, keeping plain Moonkin Aura for the
  spell crit, which isn't blocked. Both places now ask `Pet.inheritsOwnerAttackSpeed`, since it was two
  copies of that test drifting apart that let it through.
- **Guardians and raid auras (PAR-P7-0d):** the sim gave every guardian none of them, on the premise
  they aren't around to receive raid buffs. `Spell::SelectImplicitAreaTargets`'s
  `AnyGroupedUnitInObjectRangeCheck` (`GridNotifiers.h`) has no pet/guardian branch at all: it checks
  vehicle, same raid or party, hostility and range, so a guardian is as valid a target as a real pet.
  `applyPetBuffEffects` no longer skips guardians. A haste-carrier guardian (the DK's temp ghoul,
  gargoyle, army) is still immune to Bloodlust and its like, the same way a haste-carrier real pet is
  (`inheritsOwnerAttackSpeed`); a bloodworm carries no DK pet scaling at all, so it now takes Bloodlust,
  Windfury Totem, Improved Icy Talons and the flat AP auras (Blessing of Might, Trueshot Aura,
  Abomination's Might, Unleashed Rage, ...) like any other party member. The army's immunity is the same
  carrier mechanism as the ghoul and gargoyle, not a separate id check in Bloodlust's own script: `Pet.cpp`'s
  `NPC_ARMY_OF_THE_DEAD` case in `Guardian::InitStatsForLevel` adds `SPELL_DK_PET_SCALING_02` (51996)
  unconditionally, the same aura that carries the MELEE_SLOW immunities. This moves every suite with a
  guardian pet, DK's bloodworms most of all: a full raid-buff kit against a creature with barely any AP of
  its own is a large relative jump (see the delta report).
- `BloodlustAura` used to haste the permanent DK ghoul directly on top of the owner's own melee speed
  reaching it through `DynamicMeleeSpeedPets` (`character.MultiplyAttackSpeed(sim, 1.3)` on the owner
  mirrored the 1.3 to the ghoul, and the Bloodlust cascade activated a second `BloodlustAura` on the
  ghoul itself, since `inheritsOwnerAttackSpeed` was hunter-only): 1.3 × 1.3 instead of 1.3. 51996's
  MELEE_SLOW immunity blocks that on the server, so the ghoul only gets Bloodlust through the owner,
  like the hunter pet. `Unit.HasteCarrier` and `Pet.OwnerHasteSource` now cover the whole DK ghoul
  family (temp ghoul, permanent ghoul, gargoyle, army), replacing the ghoul-only
  `EnableDynamicMeleeSpeed`/`DynamicMeleeSpeedPets` mirroring and the separate `MeleeHaste` rating
  pass-through in `ghoulStatInheritance`'s Master of Ghouls branch, both made redundant by the one 2 s
  carrier snapshot. The permanent ghoul learns 51996 as a Ghoul family passive (SkillLineAbility 782,
  CreatureFamily 40, `Pet::LearnPetPassives`), which is why `Guardian::InitStatsForLevel` adds it only
  `if (!IsPet())`.
- **Fixed (PAR-P7-SHA):** the crit hacks were Classic-only bases (Spirit Wolves 1.1515%, the fire
  elemental 2.61% plus an Intellect dependency). `Unit::GetUnitCriticalChance` gives every non-player
  unit a flat 5% base, matching `m_baseSpellCritChance`'s 5% for spell crit; a scripted aura adds agility
  on top for the pets above still taking it (Spirit Wolves included, so its `Agility → MeleeCrit`
  dependency stays), and the fire elemental isn't one of them, so its Intellect dependency is gone.
- **Fixed (PAR-P7-SHA):** Feral Spirit and the fire elemental (Fire Blast, Fire Nova) each called
  `AutoAttacks.StopMeleeUntil` by hand on cast. All three already carry `FlagResetsAutoAttack`, which
  `RegisterSpell`'s serverdata sync now applies on its own (P3-2/P3-3); the manual calls only duplicated it.
- **Fixed (PAR-P7-SHA review):** Spirit Wolves never wired the haste carrier the Pets section already
  described (425792), so they took whatever buffs reached them directly and never echoed the shaman's
  own gear Haste Rating at all. `spirit_wolves.go` now sets `HasteCarrier`/`OwnerHasteSource` to
  `shaman.SwingSpeed()`, gated on `SpellTweaks.FeralSpiritHaste` (on live), matching the hunter pet and
  DK ghoul pattern. Also fixed their AP inheritance, 31%/61% glyphed to the server's 30%/60%
  (`spell_shaman.cpp:222-243`, `CalculateAPAmount`; Glyph of Feral Spirit 63271's own effect is +30, not
  +31). Bisected on `WF-default_wf-NoBuffs-LongSingleTarget` (Orc p1): the pre-review code (both
  fixes reverted) gives 3258.20 DPS; the AP fix alone drops it to 3256.68 (-0.05%, less AP from the
  wolves); both fixes together give 3283.45 (+0.77% over the pre-review number). The net move varies by
  suite and sign, since the AP cut and the new haste gain partly offset and Spirit Wolves are a minority
  of Enhancement's total damage; `LongMultiTarget` suites move by roughly -0.5% to +1% beyond what
  stages 1-2 already reported, `AllItems` rows that swap in a haste or haste-proc piece move up to +9%
  (e.g. `BlessedGarboftheUndeadSlayer`), since the wolves now actually benefit from it.
  **Fixed (wave I cross-review):** those `AllItems` gains were the double count 425792's own
  `MOD_MELEE_HASTE` block exists to stop, since Windfury Totem and Improved Icy Talons are already in the
  shaman's `SwingSpeed()`. `applyPetBuffEffects` now strips both for a carrier that blocks them (Pets,
  above), which takes most of it back: Enhancement 189 rows move, median -1.08%, -3.86% to +0.52%.

**Hunter** (`TestSimvalHunter`, the hunter recorded runs, code)
- Ranged haste: the server core has no base bonus. mod-individual-progression recasts spell 89507 on login (aura 141
  `MOD_RANGED_AMMO_HASTE`, −15%, bows and crossbows) and gives quivers and ammo pouches their equip spells back
  (`mod-individual-progression/data/sql/world/base/vanilla_item_changes.sql`).
- Measured ranged speed multipliers of a night elf hunter after relogging with each gear set:

  | Gear | `m_modAttackSpeedPct[RANGED]` |
  |---|---|
  | Worn Shortbow + Light Quiver (2101, spell 29418 +10%) | 1/1.10 |
  | Zod's Repeating Longbow + Light Quiver | 1/1.10 |
  | Zod's Repeating Longbow + Nerubian Reinforced Quiver (44448, spell 29414 +15%) | 1/1.15 |
  | Dwarf, Old Blunderbuss + starting ammo pouch (spell 14824 +10%, guns) | 1/1.10 |

- The 89507 aura is always present but never changes the speed, so ranged haste is just the quiver or pouch.
  `Hunter.Options.quiver` picks it: ×1.15 for a bow, crossbow or gun with a 15% quiver or pouch (equip spell
  29414/14829, the default), else ×1.
- Auto Shot: `_UpdateAutoRepeatSpell` fires it one update after its timer runs out, but restarts the timer before that
  update's decrement, so shots land on `NextServerTick(timerAt)` like melee; the PLAN's "one update late" premise nets
  to nothing, and the recorded runs' shot intervals agree. A haste change rescales the running timer
  (`Unit::ApplyAttackTimePercentMod`, rating included through `Player::ApplyRatingMod`).
- Volley (58434) channels with `ATTR2_DO_NOT_RESET_COMBAT_TIMERS`: `Unit::Update` stands the ranged timer still
  (`suspendRangedAttackTimer`) and resumes it where it stopped; a shot already due still goes. The sim cancelled Auto
  Shot and restarted it 500 ms after the channel.
- Moving fails Auto Shot (`SPELL_FAILED_MOVING`, `Spell::CheckCast`) without the 500 ms restart delay. Explosive
  Trap's object (189322) arms on the first update after its 1 s `startDelay`. With mod-spell-tweaks on, Trap Launcher
  (425777-425782) lays it from range, so there's no walk-in. Its damage is 49065's: magic class with
  `ATTR3_ALWAYS_HIT`, so it never misses and crits (glyphed ticks too) off the hunter's spell crit for +50%
  (`Spell::AddUnitTarget` rolls for `m_originalCaster`). The trap's trigger creature casts it
  (`GameObject::CastSpell`), so there's no ten-target cap.
- Steady Shot (`Spell::EffectSchoolDMG`): 252 + a plain weapon roll + ammo DPS × weapon speed, neither normalized, +
  0.1 RAP (`spell_bonus_data`). It and Multi-Shot take the ranged-slot cast rule, with no cast-time code of their own.
- Weapon damage (`Player::CalculateMinMaxDamage`) includes ammo DPS × the weapon's speed, × 2.8 when normalized
  (Aimed Shot, Multi-Shot, Chimera Shot).
- Crit damage (`Unit::SpellCriticalDamageBonus`): ranged +100%, then Mortal Shots and Marked for Death once each,
  Serpent Sting included (the sim doubled Mortal Shots there). Serpent Sting ticks crit only with the T9 2pc (67150,
  aura 286), Explosive Trap's only with its glyph (63068); Volley's and Explosive Shot's ticks are triggered hits
  (58433, 53352) and crit, and 53352 never misses (`ATTR3_ALWAYS_HIT`).
- Black Arrow: `ap_dot_bonus` 0.02 a tick on ranged AP, the target's Hunter's Mark included (the sim had 0.023
  without it).
- Improved Arcane Shot (19454-19456) is +5% damage a rank, no cooldown cut. Nether Shock (53589) has a 40 s cooldown,
  less Longevity. Wild Quiver deals its damage as 53254, the shot the talent triggers: 80% weapon damage, ammo
  included, and Marked for Death covers it like Auto Shot.
- Pets:
  - `spell_hun_generic_scaling` (34902-34904), recalculated every 2 s: 45% of the owner's stamina; 22% of the owner's
    RAP plus Hunter vs. Wild's stamina share as AP; 12.87% of RAP as spell damage for the magic schools (misc 126);
    35% of armor. Wild Hunt's `AddPct` truncates the int32 stamina and AP percents (54/63, 25/28), not the float
    spell one.
  - Strength 192 and agility 158 at 80 (`pet_levelstats`), AP 2·Str − 20, weapon 60-100 at 2 s
    (`Pet::InitStatsForLevel`). Crit is 5% plus auras for melee and magic alike, with none from agility
    (`Unit::GetUnitCriticalChance`, `SpellDoneCritChance`): the captured BM pet reads 14.4% = 5 + Ferocity 10 − 0.6.
    Magic crit takes only spell crit auras (`m_baseSpellCritChance`), not Spider's Bite's weapon crit, and is 0 for a
    physical-school magic ability (Demoralizing Screech) or an `ATTR2_CANT_CRIT` one (Fire Breath).
  - `Guardian::UpdateDamagePhysical` reads the unhasted 2 s (`Unit::GetAttackTime`), so Cobra Reflexes (61682/61683,
    aura 9 alone) is pure haste at full damage per hit. Happiness's +25% goes on weapon damage only.
  - Focus: `Creature::Regenerate` gives 24 every 4 s, +50% a Bestial Discipline rank.
  - `PetAI::UpdateAI` runs on the pet's update and picks at random among the autocast spells it can cast then. The
    GCD is the spell's own 1.5 s; Pin and Nether Shock are off it.
  - Magic-class abilities roll the magic hit table, crit for +50% and scale with pet spell damage (`spell_bonus_data`
    0.333 direct; ticks 0.067, 0.167, 0.333). Physical ones take 0.07 AP only where the data gives it; Pin's and
    Savage Rend's ticks take none. Spore Cloud ticks once on each target (the sim hit its main target twice).
  - Exotic families (`CREATURE_TYPE_FLAG_TAMEABLE_EXOTIC`, Rhino included) take mod-spell-tweaks' exotic multiplier
    on everything they deal (`Unit::DealDamage`).

**Raid buffs, debuffs and racials** (`sim/core/{buffs,debuffs,consumes,racials}.go` against the capture)
- Reading an amount off the capture (`SpellEffectInfo::CalcValue`): `basePoints`, plus
  `int32(realPointsPerLevel * (80 - max(baseLevel, spellLevel)))` when `realPointsPerLevel` isn't 0, plus 1
  at `dieSides` 1. Skipping the level term reads Battle Shout as 548, Demoralizing Shout as 410 and
  Demoralizing Roar as 408; all three are really 550, 411 and 411 at level 80, which is what the sim had.
  Blood Fury's 322 AP / 163 SP and Vindication's 574 come from the same term.
- Values the sim carried over from Classic data: Blessing of Wisdom 92 while Mana Spring stays 91 (48936,
  58777); Demoralizing Screech 574, not 576, and 10 s, not 4 s (55487; the panel makes it permanent, so
  only the amount moved a golden).
- A talent's percentage on an aura amount is truncated, since `CalcValue` returns int32: Improved Power
  Word: Fortitude is 214, not 214.5, Improved Devotion Aura 1807, Improved Demoralizing Shout 575.
  `addPct` in `buffs.go` does the same.
- Curse of Weakness at tristate "regular" was paying Improved Curse of Weakness rank 1's +10%. The plain
  curse is 478 (50511); only "improved" takes the +20% (18180).
- Booming Voice is +25% duration a rank on all three shouts: 12835's class mask covers Demoralizing
  Shout (0x20000) as well as Battle and Commanding Shout. The sim gave Demoralizing Shout +10%.
- Master Poisoner's crit debuff (45176) is `MOD_CRIT_CHANCE_FOR_CASTER`, so it helps only the rogue who
  applied it (`Unit.cpp:3940`, `9455`), while the sim hands it to the whole raid. Fixing it needs the
  caster on `core.MasterPoisonerDebuff`, which `sim/rogue/poisons.go` also calls. Heart of the Crusader
  (54499) and the Totem of Wrath debuff (30708) are `MOD_ATTACKER_SPELL_AND_WEAPON_CRIT_CHANCE`: raid-wide.
- Thorns (53307) isn't binary, so it resists partially. Its `spell_bonus_data` Direct 0.033 comes off the
  druid who cast it, not off the buffed player, so the sim leaves the shield unscaled.
- The 10-target AoE cap stays: `Spell::DoAllEffectOnLaunchTarget` (`Spell.cpp:8442-8454`) does
  `damage * 10 / count` past 10 targets, but only for a player caster and only on the launch damage. A
  pet's, totem's or guardian's area hit and a periodic tick aren't capped, which class items still owe.
  `Unit::CalculateAOEDamageReduction` is aura avoidance, no cap.
- Confirmed against the capture, so a class item needn't re-check: Gift of the Wild 37/750/54 and the
  improved 51/1050/75, Fel Intelligence 48/64, Strength of Earth 155 (178 with Enhancing Totems),
  Flametongue Totem 144, Totem of Wrath 280 and its 3%, Windfury Totem 16/20%, Icy Talons 20%,
  Retribution Aura 112 (+50% with Sanctified Retribution), Blood Fury 322 AP / 163 SP, Berserking 20%
  (`HandleModCombatSpeedPct` covers cast, melee and ranged), Arcane Torrent 15 energy / 15 runic power /
  6% mana, racial resistances 2% spell miss, Curse of the Elements 13% and −165, Earth and Moon 13%,
  Blood Frenzy and Savage Combat 4%, Mangle and Trauma 30%, Stampede 25%, Shadow Mastery and Improved
  Scorch 5%, Winter's Chill 1% a stack, Misery 3%, Hunter's Mark 500, Thunder Clap 10/20%, Infected
  Wounds 20%, Judgements of the Just 20%, Frost Fever 14%, Insect Swarm and Scorpid Sting 3%,
  Vindication 574 (26017 scales −8.8 a level from 20), and the explosives' 1150-1500, 750-1000 and
  2188-2812 rolls.
- Still open: Stoneform's +10% armor, since 20594 lasts 100 ms and only grants immunities, and the real
  buff 65116 is outside the capture; Enhancing Totems raising Flametongue Totem to 165, which the sim's
  plain bool can't express; and Thorns' Brambles bonus, which isn't a spell modifier at all but a dummy
  (16836) the server folds into `SpellDamageBonusDone` as a float and truncates only after spell power,
  so `addPct` on the sim's 73 base would be the wrong shape of fix.

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
    - Frost Fever (Runic Power Mastery; FF becomes melee damage class like Blood Plague: `DefenseType` 2,
      which mod-spell-tweaks' docs mislabel magic)
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

**Rogue** (`TestCombat`, `TestAssassination`, `TestSubtlety`, code)
- Deadly Poison (57970, magic dmg class) and Rupture (48672, melee dmg class) both go through
  `sim/rogue/{poisons,rupture}.go`'s `AffectedByCastSpeed`/`TickHaste: core.MeleeHasteAddsTicks`, gated
  on `Talents.Murder`/`Talents.WeaponExpertise` and `Server().SpellTweaks.{DeadlyPoisonMurder,
  RuptureWeaponExpertise}` (P7-0a's audit already lists both scripts; this item wires them). Duration
  stays fixed and the tick interval shrinks with melee haste, the same shape as the DK disease tweak.
  Every Deadly Poison refresh and stack recomputes the interval off current haste (`Aura::RefreshTimers`
  reruns the script's `CalcPeriodic` hook), not just its first application: the missing recompute, found
  in cross-review, froze the interval at the pull's haste and cost Assassination 2-5%.
- Deadly Poison's tick crit is Murder-gated too: rank 1 and 2 both carry a second effect, aura 286
  (`SPELL_AURA_ABILITY_PERIODIC_CRIT`) on class mask `[0, 0x80000, 0]` (spelldump ids 14158/14159),
  which covers Deadly and Wound Poison's family flags. Since Deadly Poison is magic dmg class, its
  snapshot crit chance reads `Spell.SpellCritChance` (crit rating, no agility), not the physical
  formula the diseases use. Rupture already crits unconditionally
  (`AuraEffect::CalcPeriodicCritChance`'s `SPELLFAMILY_ROGUE` case, family flag `0x100000`), so its
  `OutcomeSnapshotCrit` call predates this item; `TicksCanCrit` on both is now declared explicitly
  (`true` for Rupture, Murder-gated for Deadly Poison, `false` on Garrote, which neither aura covers).
- Off-hand start after Vanish or a stealth break: `AutoAttacks.EnableAutoSwing` (`rogue.BreakStealth`'s
  only caller) put both hands live at the same instant, where `startPull` already staggered a ready off
  hand `max(own timer, main hand timer + half the main hand's hasted swing)` (retail deviation #30,
  `Unit::Attack`). `EnableAutoSwing` now applies the same stagger. `IsDualWielding` gates it, so pets,
  druid forms and the ghoul (the other callers) are untouched.
- Master Poisoner (45176) is `SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER` (confirmed in the spelldump, aura
  308, and `Unit.cpp:3940,9455`): `victim->GetTotalAuraModifier` filters to auras whose caster GUID
  matches the attacker, so it raises crit chance only for the rogue whose Deadly/Wound Poison carries it
  (58410's `triggerSpell`), never the raid. `core.MasterPoisonerAura` now takes that caster and adds
  `stats.MeleeCrit`/`stats.SpellCrit` to it (`AddStatsDynamic`) instead of `PseudoStats.BonusCritRatingTaken`
  on the target; duration corrected to 15 s (spelldump, was the shared 20 s `HeartOfTheCrusaderDebuff` used).
  `sim/rogue/poisons.go` reference-counts targets carrying the rogue's own poison debuff
  (`gainMasterPoisoner`/`loseMasterPoisoner`) so the bonus survives one target's poison dropping while
  another's is still up; it's exact for a single target and an approximation once a rogue keeps poison
  on two targets at once (no suite exercises that). The raid-wide `Debuffs.master_poisoner` option had no
  target to apply the per-caster bonus to (`applyDebuffEffects` only ever sees the enemy), so it can't
  represent this mechanic at all, and nothing reads it any more: the `debuffs.go` branch, the raidctx
  provider row, `ui/raid/raid_stats.ts`'s entry, the `buffs_debuffs.ts` checkbox, `player.ts`'s crit-cap
  term, `character_stats.tsx`'s debuff stats and the feral tank default are gone. The proto field stays so
  saved settings still load.
- Poison proc chance (Instant/Wound/Deadly's application, not their damage): `Player::CastItemCombatSpell`
  rolls a flat `SpellItemEnchantmentEntry::amount` per landed hit for a `ITEM_ENCHANTMENT_TYPE_COMBAT_SPELL`
  enchant (poisons occupy `PERM_ENCHANTMENT_SLOT`), through `ApplySpellMod(..., SPELLMOD_CHANCE_OF_SUCCESS,
  ...)` for Improved Poisons, unless the enchant has a `spell_enchant_proc_data` row with its own PPM or
  custom chance. No such row turned up for the poison enchant ids, so the chance looks weapon-speed
  independent, where `sim/rogue/poisons.go`'s Instant and Wound Poison already use a PPM manager
  ("the former X% normalized to a 1.4 speed weapon"). Deadly Poison rolls a flat chance already
  (`GetDeadlyPoisonProcChance`). Left unresolved: nothing in the offline capture gives the enchant's
  `amount` values, so whether Instant/Wound Poison's PPM model over- or under-rolls away from 1.4 speed
  needs a live `.simval procs`-style capture on the poison enchant ids, not the damage spell ids.
- Killing Spree's two per-hit swings (`sim/rogue/killing_spree.go`) register under
  `ActionID{SpellID: 51690, Tag: 1|2}` with a code comment giving the real ids (57841/57842), so the
  P3-2 serverdata lookup never finds them: `spells_missing.csv` doesn't list them either, since the scan
  only flags ids it expected to resolve and couldn't. Not fixed here: retagging needs the real ids as
  constants and a check that no set bonus or script keys off spell family flags on the wrapper id instead.

**Warrior** (`TestFury`, `TestArms`, `TestProtectionWarrior`, code)
- Slam (`sim/warrior/slam.go`): `spell_warr_slam::HandleDummy` (`spell_warrior.cpp:318-353`) only casts
  50783 once the cast (47475, `SPELL_ATTR0_NO_ACTIVE_DEFENSE`) lands, so the table roll happens once, on
  the cast. The regenerated capture shows 50783 itself carries `SPELL_ATTR3_ALWAYS_HIT`, not the
  dodge/parry-able hit the P3-2 allowlist assumed, so the sim now splits Slam into two spells: 47475
  rolls a hit-or-miss check, 50783 deals the damage. `SpellFlagAlwaysHit` reaches `applyServerData`
  (`spell.go:789-791`), and 50783 uses `OutcomeMeleeSpecialCritOnly` (the "roll happened elsewhere"
  applier Mutilate, Rune Strike and Stormstrike already use), which now reads the flag and skips the
  residual block roll `Unit::isSpellBlocked` rules out (PAR-P7-0e). Whatever keys on
  Slam's damage or crit must key on 50783 (`warrior.SlamHit`): after the split, Recklessness's crit list
  and the Siegebreaker 2pc still keyed on the cast, which never crits (fixed in cross-review).
- Deep Wounds (`sim/warrior/deep_wounds.go`) now goes through `core.DelayedPeriodicApplier` (**DoTs
  and periodic ticks**, PAR-P7-0e), matching the caster's own phase instead of a flat 400 ms; the
  outstanding-damage math is unchanged.
- Rend (`sim/warrior/rend.go`) now reads `Server().SpellTweaks.RendTrauma` for the melee-haste add-ticks
  half and `Talents.Trauma > 0` alone for the crit half, matching `SpellTweaks_classes.cpp:372-395`
  (`spell_tweaks_rend_haste`) and the Trauma talent's static `spell_dbc` override. It needed a
  `CritMultiplier` it never had before, since it couldn't crit until now.
- Titan's Grip (`sim/warrior/talents.go`): the -10% penalty now checks
  `Server().SpellTweaks.TitansGripNoDamagePenalty` first (`SpellTweaks_classes.cpp:610-636`,
  `spell_tweaks_titans_grip_no_penalty::NeutralizePenalty`), live on.
- Shattering Throw and Heroic Throw (`sim/warrior/shattering_throw.go`, `heroic_throw.go`): both carry
  `FlagResetsAutoAttack` in the generated data (57755, 64382), which `cast.go`'s `resetsSwing` already
  resets on cast start and completion, so the hand-rolled `StopMeleeUntil` calls were redundant, and
  modeled the wrong reset shape besides (retail deviation #19). Shattering Throw's cast time is
  unconditionally 1.5 s now; Glyph of Shattering Throw (206953) is a WotLK Classic item this server
  doesn't have, so it's out of the presets (`ui/warrior/presets.ts`, `FuryGlyphs`, `presetOptimizeRequest`'s
  Fury player, `fury_p1.json`) but stays live in `hasGlyph`'s stance-switch and cast condition for a
  hand-built profile, and `ArmsGlyphs` (`dps_warrior_test.go`) keeps it glyphed for coverage of that path.
- Recklessness and Death Wish (`sim/warrior/recklessness.go`, `talents.go`): the P3-2 GCD entries are
  correct as declared. `applyServerData` already forces `DefaultCast.GCD` to 1500 ms, and each
  ability's own `WaitUntil(sim.CurrentTime+GCDDefault)` sets the same absolute GCD-ready time a second
  time, not a second GCD.
- Slam's cast time reset (`sim/warrior/talents.go`): Bloodsurge's and the Ymirjar 4pc's `OnExpire`
  hardcoded 1500 ms, dropping Improved Slam's -500 ms a point. Both now call the `slamCastTime()`
  helper the spell itself registers with.
- `tools/acore/spellaudit -area warrior` found no unresolved warrior spell ids (`spells_missing.csv`'s
  20 rows are all in `sim/core`, `sim/common`, `sim/optimizer` and `sim/rogue`), and no other warrior
  spell's cast, GCD, cooldown or binary flag disagrees with the server beyond what
  `TestServerDataConflicts` already catches.
- Heroic Strike and Cleave already suppress the dual-wield miss penalty through
  `PseudoStats.DisableDWMissPenalty` (`spell_outcome.go:397`); no warrior APL reads `spell.cast_time`,
  so the tick-rounding item doesn't touch this class.
- `sim/core/serverdata/*_auto_gen.go` regenerated (`gen_serverdata`) to add 50783, the only spell id
  this item newly names.
- A held main hand's last-moment APL check could chain straight into another Slam once cast time
  equals GCD (0-point Improved Slam), which re-held the swing before it fired, forever, for
  back-to-back Slams. `Player::Update`'s melee check (`PlayerUpdates.cpp:159`) runs only after
  `Unit::Update`'s `_UpdateSpells` lands the cast and clears `UNIT_STATE_CASTING`, and a cast
  reacting to that landing (`ProcessSpellQueue`, or a new packet) can't start before the next
  update, so it can't preempt the swing that already landed. `attack.go`'s `swing()`/`trySwing()`
  now take a `releasingHold` flag, skipping the re-check only for the swing being released. No
  suite hits this: talented Slam decouples cast time from GCD, and the tested builds never chain
  untalented Slam back-to-back with zero gap (confirmed with and without the fix, all three
  suites' goldens identical). Covered by `TestHeldSwingGoesBeforeAChainedSlamFromItsOwnLanding`.
- Review round: `releasingHold` above only reached `releaseCastHold`'s two `trySwing` calls.
  `resumeMelee` also calls `trySwing` directly once a suspended hand's restored timer is already
  due (the second hand a shared cast landing releases), still passing `false`, so it could hit the
  same race. Both of its calls now pass `true` too. Same caveat: no suite hits it, confirmed by
  full-suite `dock.sh delta` before and after (zero suites, zero DPS moved).
- `sim/optimizer/testdata/search/fury_p1.json` drifts on a plain `-update` well beyond the glyph: slot
  14's enchant options gain id 3851, unchanged even with every warrior and evaluator edit reverted, so
  it's the live reforge/enchant data moving since the file was last regenerated, not this item. The
  committed file keeps that drift out and only drops the glyph by hand; a stale search fixture is a
  finding for whichever item next owns `sim/optimizer/testdata`.
- Live-verified (`TestSimvalWarriorP7`): Rend's tick data, Trauma's scoped crit override, Deep Wounds'
  no-crit and Shattering Throw's flat cast time all match the server. Numbers in **Verified on the live
  server**.

**Retribution Paladin** (`TestSimvalRetribution`, code)
- Seal DoT via strikes was already right: `spell_pal_seal_of_vengeance_aura::HandleApplyDoT`
  (`spell_paladin.cpp:1947-1960`) stacks Holy Vengeance/Blood Corruption only off a white hit or
  Hammer of the Righteous (icon check); mod-spell-tweaks' `spell_tweaks_seal_dot_on_strikes` adds
  Crusader Strike and Divine Storm on top by casting the DoT directly, not through this gate. `HandleSeal`
  (the stack-scaled damage bonus, same file 1962-1984) reads the stack count before `HandleApplyDoT`
  increments it, and the sim's `OnSpellHitDealt` already calls them in that order.
- Found and fixed: `HandleSeal`/`HandleApplyDoT` only run at all when the seal aura's own proc gate
  passes, `Spell.dbc` `procFlags` 20 = `PROC_FLAG_DONE_MELEE_AUTO_ATTACK | PROC_FLAG_DONE_SPELL_MELEE_DMG_CLASS`
  (`SpellMgr.h:113,116`, live on 31801/53736/20375/21084). Hammer of Wrath (48806) is `ProcMaskMeleeMHSpecial`
  in the sim like every other special, but its server damage class is Ranged (confirmed in the spelldump and
  `sim/core/serverdata/spells_auto_gen.go`), so it never reaches any seal there. All three seals
  (`sov.go`, `soc.go`, `sor.go`) used `spell.IsMelee()` alone and let Hammer of Wrath proc; fixed with a
  shared `sealCanProcOn` check. Shield of Righteousness (61411, melee damage class) keeps the damage bonus
  but was also wrongly in the DoT stack list; dropped, since neither the core gate nor the module's addition
  cover it.
- Found and fixed: Holy Vengeance/Blood Corruption and Righteous Vengeance never crit. mod-spell-tweaks
  (`docs/paladin/RetPaladinDotCrit.md`) adds an aura-286 (`SPELL_AURA_ABILITY_PERIODIC_CRIT`) effect to the
  Two-Handed Weapon Specialization talent (20111-20113) scoped to Holy Vengeance/Blood Corruption, and to the
  Righteous Vengeance talent (53380-53382) scoped to itself, so either DoT crits at the paladin's normal
  melee chance once its talent has any points, independent of gear. The sim gated Holy Vengeance's tick on
  nothing (never crit) and Righteous Vengeance's on the Turalyon's/Liadrin's Battlegear T9 2pc alone; both
  now follow their talent instead. The T9 2pc (67188) turned out to carry the same aura-286 unlock plus a
  separate +100% crit-damage effect, but `SpellInfoCorrections.cpp`'s `ApplySpellFix({67118, 67150, 67188}, ...)`
  zeroes that second effect as a known Blizzard DBC bug, so on this server the 2pc adds nothing once the
  talent is trained; the sim no longer references it for Righteous Vengeance.
- Verified on the live server (`spell_script_names`, `glyphproperties_dbc`, `item_template`): Glyph of
  Reckoning now has full server-side support (`spell_tweaks_glyph_of_reckoning` bound to Hand of Reckoning
  62124; glyph 912, item 90060). It's still net-new 3.3.5a content: the module's own docs
  (`docs/paladin/GlyphOfReckoning.md`) call the matching client `Spell.dbc`/`Item.dbc`/etc. rows a manual,
  unapplied step, so a real player still can't train or socket it. The sim's stub stays.
- Exorcism's `ModifyCast` held melee by hand (`AutoAttacks.StopMeleeUntil`); the generated serverdata already
  carries `FlagResetsAutoAttack` for 48801, which `cast.go`'s hardcast path (P7-0c) now holds or resets on its
  own for every spell with a cast time. Dropped the redundant hand hold.
- Judgement proc rules and damage classes check out: Judgement of Wisdom/Light are `DmgClassRanged` server side
  and use `OutcomeRangedHit` (a real hit roll, no dodge/parry/block); the three secondary judgements
  (Judgement of Righteousness/Command/Vengeance) are `DmgClassMelee` with `FlagNoActiveDefense | FlagAlwaysHit`
  and use `OutcomeMeleeSpecialCritOnly`. Coefficients match `spell_bonus_data` for every spell that has a row
  (Judgement of Righteousness 0.32/0.20, Judgement of Vengeance 0.22/0.14, Holy Vengeance 0.013/0.025 a tick,
  Exorcism and Hammer of Wrath 0.15/0.15).
- P6-3's 47661 (Libram of Valiance) already matches the live `spell_proc` row for 67365 (70% chance, 8 s ICD
  off a Holy Vengeance tick); the T9 2pc row folds into the Righteous Vengeance crit fix above.
- Suites moved (SOC/SOR/SOV builds, `dock.sh delta`): SOC roughly flat, -0.35% to +2.8% (Hammer of Wrath's
  execute-phase seal loss can outweigh the Exorcism fix in a no-buff, no-Holy-Vengeance build); SOR +0.1% to
  +2.7% (Shield of Righteousness never carried the DoT anyway, so this is the seal-proc-gate fix and Exorcism
  alone); SOV and SOV 2-target +3.7% to +6.8%, dominated by Holy Vengeance's now-real crit chance. Protection
  moved too, -2.4% to +0.4% (its suite doesn't invest in Righteous Vengeance or Two-Handed Weapon
  Specialization, so it's purely losing the erroneous Hammer of Wrath/Shield of Righteousness seal procs, offset
  a little by whichever seal it runs).
- Recorded run (`TestRecordedRun`, `SIMVAL_RECORD_SPEC=ret`, human paladin, 600 s at 4 yards in front of the
  dummy, `SIMVAL_RECORD_TALENT_SPELLS` set to `StandardTalents`'s 26 ids, `SIMVAL_RECORD_TWOHAND=1`): Retribution
  Aura and Seal of Vengeance before the pull (`SIMVAL_RECORD_START_SPELLS=54043,31801`), then Judgement of
  Wisdom, Crusader Strike, Divine Storm, Consecration and Divine Plea in a fixed 1.5 s cycle; Divine Plea keeps
  every mana spell casting to the end. Hammer of Wrath and Exorcism stay out (the dummy's health never drops,
  module README; Exorcism below). The factory's glyphs are its "ret pve" premade (Judgement, Consecration, Seal
  of Vengeance major), which the setup now records.

  | Run | Server DPS | Sim DPS | Gap |
  |---|---|---|---|
  | Ret (`ret_Svrleadjxcky_1790084304`, boss-only, 600.4 s) | 4576.6 | 6230.4 | -26.5%, rotations (below) |

- Two `recorded_run_test.go` bugs found and fixed while recapturing:
  - `PlayerbotFactory::InitEquipment` never reads the spec `InitTalentsTree` just rolled (its
    `GetPlayerSpecTab` call is commented out), so the gear command's weapon choice ignores
    `SIMVAL_RECORD_TALENT_SPELLS`, and an off hand item it already equipped stays through every later gear
    command on the same character (150 straight calls, same item, confirmed live). `gearedBot` now gears a
    fresh character each attempt behind a new `SIMVAL_RECORD_TWOHAND` flag, since only a character's first
    gearing pass has real variance.
  - `grownLog` picked whichever instance log grew the most, which during one capture was a live raid's
    (15 MB) rather than the dummy fight's (1.3 MB); it now requires the log to name the player.
- The DPS gap compares rotations: rrsim's priority list can't replay the fixed cycle (no APL action gives up
  on a refused cast until the next lap), so it casts Crusader Strike, Divine Storm and the judgement 50-80%
  more often. Per ability (`rrsim -v`, `chronicle -v`; server mean ± standard error, crit rate over landed
  hits), every row sits within 2 standard errors but two:

  | Ability (server non-crits/crits) | Non-crit, server / sim | Crit, server / sim | Crit %, server / sim |
  |---|---|---|---|
  | Melee (54/75) | 2394±25 / 2437 (-1.8%) | 5060±44 / 5045 (+0.3%) | 40.5±3.6 / 45.2 |
  | Melee glancing (56) | 1695±15 / 1713 (-1.1%) | – | – |
  | Seal of Vengeance proc (42463; 206/152) | 1215±10 / 1241 (-2.1%) | 2578±22 / 2555 (+0.9%) | 42.5±2.6 / 42.6 |
  | Holy Vengeance tick (31803; 117/81) | 1254±13 / 1276 (-1.7%) | 2645±35 / 2630 (+0.6%) | 40.9±3.5 / 42.6 |
  | Crusader Strike (35395; 45/31) | 2206±22 / 2208 (-0.1%) | 4549±61 / 4550 (0.0%) | 40.8±5.6 / 42.6 |
  | Divine Storm (53385; 33/24) | 2843±45 / 2821 (+0.8%) | 5720±68 / 5811 (-1.6%) | 42.1±6.5 / 42.4 |
  | Judgement of Vengeance (31804; 17/23) | 3250±128 / 3283 (-1.0%) | 6795±176 / 6770 (+0.4%) | 57.5±7.8 / 60.5 |
  | Consecration tick (48819; 387) | 524±3 / 526 (-0.4%) | – | – |
  | Manifest Anger (71433, trinket; 28/15) | 1238±18 / 1227 (+0.9%) | 2515±42 / 2530 (-0.6%) | 34.9±7.3 / 42.6 |
  | Righteous Vengeance tick (61840; 109/75) | 688±26 / 1085 (-36.6%) | 1437±71 / 2239 (-35.8%) | 40.8±3.6 / 42.5 |

  - Seal of Vengeance's non-crit (-2.6σ) is sampling: its hits caught Piercing Twilight (+1472 AP, 32.5%
    uptime) 28% of the time, its crits 38%. Inside and outside the buff, crit over non-crit is 2.06-2.08, the
    2.06 the judgement's exact values show; the all-outcome mean is +0.3%.
  - A Righteous Vengeance tick is a quarter of a pool the crits feed, so its size follows the rotation. Per
    30% of the crit damage fed to it (Crusader Strike, Divine Storm, the judgement), both deliver the same:
    server 1.40, sim 1.39, its own crits and resists included, with PAR-P7-0e's delayed refresh. The server
    paid an old tick twice on 4 of its 78 procs (**DoTs and periodic ticks**). The sim read 1.50 while it also
    paid a tick due on the refresh's own server tick, fixed in the H3 cross-review.

  At the capture's own outcome counts, the sim's per-outcome means give 0.30% more damage than the capture,
  Righteous Vengeance aside. Chronicle records a dodged or parried spell only as
  `CHRONICLE_SPELL_TARGET_RESULT`: `SPELL_MISSED` comes from `Unit::SendSpellMiss`, which only reports
  hit-time immunity and damage shields. `chronicle` now counts it: 4 of 80 Crusader Strikes and 2 of 59
  Divine Storms were parried. A missed Consecration tick leaves no record at all
  (`AuraEffect::HandlePeriodicDamageAurasTick` returns unlogged), so the sim's 2.4% tick misses have no
  server count to compare.
- The first capture (300 s, no aura, glyphs unrecorded) read 6-15% under the sim on nearly every ability.
  Closed by:
  - rrsim setup: no Avenging Wrath (`autocastOtherCooldowns` cast it, the capture never does); the aura
    `started:` names, none then (Sanctified Retribution's 63531 dropped 0.4 s after login); swings from the
    front; the setup's glyphs; and the live DBC's stats, with a warning, for an enchant the sim's item
    database lacks (3776: +45 AP and +15 crit rating on the shoulders of every Ret and hunter capture). The
    sim's stats then match the pre-fight `.simval info`: AP 5970/5968, crit 40.15%/40.16%, hit 383, expertise
    22, haste 345, ArP 536.
  - Sim bugs, fixed in `sim/paladin`:
    - Sanctified and Swift Retribution work only through the paladin's own aura (63531 area-checks for one,
      `spell_pal_sanctified_retribution_effect::CheckAreaTarget`); `AddRaidBuffs` granted them with none.
    - Two-Handed Weapon Specialization (20113: aura 79, physical school mask) no longer raises seal or
      judgement damage: a holy weapon-percent spell takes weapon damage without the physical % mods
      (`Spell::EffectWeaponDmg`, `CalculateDamage(..., isPhysical)`), and `MeleeDamageBonusDone` and
      `SpellPctDamageModsDone` match the aura by school. Seal of Vengeance's proc read -7% before.
    - Percent damage spell mods multiply (`Player::ApplySpellMod`): Seals of the Pure, The Art of War, Sanctity
      of Battle, the damage glyphs and set bonuses. The judgement's exact values confirm it: Piercing
      Twilight adds 734 to a 3195 non-crit, a ×1.609 multiplier against 1.609 multiplied and 1.561 added.
    - Divine Storm deals normalized weapon damage (`Spell::EffectWeaponDmg` special-cases 53385).
    - Seal of Vengeance's proc truncates its weapon percent per stack (33 × stacks / 5, `HandleSeal`).
  - Suites: Retribution -1.5% on average, -0.4% to -2.1% (SOC -1.3% to -1.8%, SOR and SOV
    -0.7% to -1.6%); Protection -0.004% to -0.03%, from the seal proc's truncation. Only DPS and TPS move.
- Exorcism (48801) hits any creature type: its Spell.dbc `TargetCreatureType` is 0 (all ranks, live DBC), so
  `SpellInfo::CheckTargetCreatureType` passes every target, and `Unit::SpellTakenCritChance` only forces its crit
  on undead and demons. The capture has none because its cycle (the setup's `rotation:`) never casts it. The
  Undead/Demon `ExtraCastCondition` this item added was reverted; `rrsim`'s Ret rotation leaves Exorcism out
  instead. No golden moves either way: every suite's target is `MobTypeDemon`.
- `.simval yellow`/`spell` probes (`TestSimvalRetribution`, live, code): Crusader Strike and Divine Storm roll
  a real defense table from behind the dummy (`CanDodge` true), unlike the secondary judgements' always-hit
  one; parry and block drop out only because attacking from behind removes them structurally (`simval_test.go`'s
  own from-behind checks confirm the same for Heroic Strike), not because either spell is gated. Holy
  Vengeance is `DmgClassMelee`. Judgement of Wisdom rolls the "yellow" table too,
  tagged `attackType: ranged`, not a separate "magic" one: it can miss but is never dodged, parried or
  blocked. All four pass.

**Mage** (`TestArcane`, `TestFire`, `TestFrost`, `TestFrostFire`, code)
- Ignite (`spell_mage_ignite`, `spell_mage.cpp:709-761`): `pct = 8 * rank`, matching the sim's `0.08` a
  point, but the per-tick amount is server-truncated twice — `int32(CalculatePct(critDamage, pct)) /
  igniteDot->GetMaxTicks()` (`Util.h:52`'s `CalculatePct` truncates to a whole number, then the `/`
  truncates again) — and only that new share reaches `Unit::CastDelayedSpellWithPeriodicAmount`
  (`Unit.cpp:16310-16328`), which adds the dot's own remaining share,
  `oldAmount * max(totalTicks - tickNumber, 0) / totalTicks`, all int32, not the sim's old continuous
  float split. Ignite reads that blend at the proc, before the delay, and applies it through
  `core.DelayedPeriodicApplier` (closes PAR-P7-0e's "still owed"). No aura 286
  (`SPELL_AURA_ABILITY_PERIODIC_CRIT`) covers 12654: `TicksCanCrit: false`.
  A crit from an instant cast (Living Bomb's direct hit, or Pyroblast/Frostfire Bolt made instant by
  Hot Streak/Brain Freeze) applies through `ApplyFromInstantCast` instead of `Apply`; a hardcast crit or
  a Living Bomb tick crit still uses `Apply` (DoTs and periodic ticks, above). **Fixed (wave I
  cross-review):** it read `spell.CurCast.CastTime == 0` at the hit, which a missile outlives. At GCD
  1 s a hardcast Frostfire Bolt starts 1071 ms before an instant one lands (30 yards, missile speed 28),
  so each reads the other's cast time. `SpellResult` captures it instead (`FromInstantCast`), at the
  cast. No golden moves: the overlap needs about 40% haste and both cast shapes back to back.
- Water Elemental scaling (`spell_mage_pet_scaling`, `spell_mage.cpp:260-370`, gated on
  `m_scriptSpellId 35657`/`35658`): 30% of the owner's stamina (the sim had 20%) and intellect, 33% of
  the owner's Frost spell power (the sim's one spell-power stat already stands in for the school-masked
  one; its old `0.333` overstated the server's flat 33%), 0% AP. Resistance inheritance (40%
  non-physical / 35% physical) and the pet's own base stats stay unmodeled: neither moves a DPS-only
  sim, and the base numbers are already flagged as rough log reads pending a live capture. The pet had
  no baseline spell crit; nothing in the scaling script touches `SPELL_AURA_MOD_SPELL_CRIT_CHANCE`, so
  it now gets `Unit::GetUnitCriticalChance`'s flat 5% (Pets, above). Already `isGuardian: true` in
  `core.NewPet`, matching its 61017 scaling-aura membership (Pets, above); P7-0d's guardian raid-buff fix
  already reaches it — spot-checked by code, not a live encounter.
- Frostbolt (42842): the spelldump already marks it `"binary":false`, and nothing in `sim/mage`
  declares `SpellFlagBinary`, so P3-2's `syncServerFlag` (`spell.go:781`) already leaves it non-binary.
  The PLAN's premise was a fact to confirm, not a bug: no code change.
- T8 4pc (Kirin Tor Garb, `sets_review.csv` row 29): `T84PcProcChance` was `0.2`; 64869 effect1's
  `basePoints: 9` (the +1 convention) is 10%, read by `roll_chance_i` in `Player.cpp`. Fixed in
  `sim/mage/items.go:77`. The PLAN's "Missile Barrage/Hot Streak/Brain Freeze" scope was also wrong:
  every `SPELL_MAGE_T8_4P_BONUS` (64869) reference in `spell_mage.cpp` shows only
  `spell_mage_missile_barrage_proc::CheckProc` (1584) rolling the save chance; Hot Streak's and Brain
  Freeze's own scripts never touch it, and the one other reference
  (`spell_mage_gen_extra_effects::CheckProc`, 1139) is the unrelated "don't double-proc Hot Streak off
  Arcane Missiles" guard. `pyroblast.go`'s `ModifyCast` and `talents.go`'s Brain Freeze aura no longer
  roll `T84PcProcChance`; only `arcane_missiles.go`'s Missile Barrage aura still does.
- Percent `SPELLMOD_DAMAGE` mods (per-spell checklist, `Player::ApplySpellMod`): Fire Power (11124),
  Spell Impact, Chilled to the Bone and the Frostbolt/Frostfire glyphs are all `aura: 108`
  (`ADD_PCT_MODIFIER`), `miscValue: 0` (SPELLMOD_DAMAGE) in the spelldump, so they multiply on the
  server, not sum. Fireball, Frostbolt, Frostfire Bolt and Flamestrike had summed them in
  `DamageMultiplierAdditive`; `spellModDamage()` (`sim/mage/talents.go`, mirroring `sim/paladin`'s) now
  multiplies them, like that field's set-bonus and glyph terms elsewhere already do. Frostfire Bolt's
  glyph-excluded-from-the-DoT trick (below) moved from subtracting to dividing out the glyph's factor to
  match.
  - Open: Fire Power's second effect, `miscValue: 22` (SPELLMOD_DOT), carries a different classMask
    (`[4194309,135168,0]`) from its damage effect's (`[12845079,69704,0]`), and Spell Impact has no DOT
    effect at all. Whether Fireball's and Flamestrike's ticks should take Fire Power's bonus at all (as
    Frostfire Bolt's dot already excludes its glyph) needs that classMask decoded against each spell's
    family flags; left unchanged, taking the same multiplier as the direct hit.
- `TicksCanCrit`: none of Fireball's, Pyroblast's, Flamestrike's or Frostfire Bolt's dots carry aura
  286, so all four are `false`. Living Bomb only crits with its glyph (the sim already swaps its
  `OnTick` for that), so its declaration follows the same check, keeping the glyph's ticks crit-eligible
  once P8 flips the default.
- Default APLs: no change this wave shifts a mage priority.
- APL `spell.cast_time` (`APLValueSpellCastTime`) read the haste-scaled cast time only; a hardcast
  actually lands on the next server tick (`cast.go`'s `makeCastFunc`), which the value now rounds to
  as well. Fire and FrostFire read it in their rotations, folded into the deltas below; Frost and
  Arcane don't read it and are unaffected.
- **As built:** all four suites moved. Fire -3.716% and FrostFire -1.912% (Average-Default), both
  Ignite-delay-dominated: reverting just the delay leaves Fire at +0.303%, the same direction PAR-P7-0e
  found for Deep Wounds. Frost +0.357% (the spell-mod and Water Elemental spell-power fixes net
  positive; no Ignite points). Arcane +0.114% on one item variant (Kirin Tor Garb), sized like the
  RNG-stream noise a changed roll count leaves on later rolls, not a directional signal. The Kirin Tor
  Garb row moves further than its suite average from losing Hot Streak's and Brain Freeze's save
  chance: Fire -4.419%, FrostFire -3.473%, Frost barely (-0.055%, no points in either talent).
  The delay stage's fix to `DelayedPeriodicApplier` (DoTs and periodic ticks, above) moved Fire and
  FrostFire again, in opposite directions: Fire -3.372% -> -3.716% (Kirin Tor Garb -4.277% -> -4.419%),
  FrostFire -2.311% -> -1.912% (Kirin Tor Garb -3.863% -> -3.473%). Both builds carry Ignite points, but
  in different mixes of hardcast, Living Bomb and Hot Streak/Brain Freeze procs, so a shorter average
  delay changes how often a refresh cuts an already-ticking Ignite off before it finishes its two
  ticks, and the two builds land on opposite sides of that.
- Live (`e2e/p7_mag_test.go`'s `TestSimvalMage`): Frostbolt, Fireball and Living Bomb's explosion (55362) all come
  back magic (dmgClass 1) with partial resists, matching the code stage's fixes; Living Bomb's dot (55360) itself
  carries no periodic-crit effect, confirming only the glyph enables its tick crits.
- Fire capture (`fire_Svrleadggztq_1790099652`, 295.7 s, Scorch/Fireball/Fire Blast/Living Bomb on a 1.5 s cycle):
  recasting a spell — even the same one — while a hardcast is still resolving cancels it
  (`SPELL_FAILED_INTERRUPTED`), so a >1.5 s hardcast's cycle slot needs an unlearned filler id after it, not a
  repeat, to ever complete; worth remembering for the next capture with a long hardcast. Of 84 Ignite applications,
  77 pair to a Fire-school crit (Scorch, Fireball, Fire Blast, Living Bomb's explosion at this character's
  talent-taught rank 44461) inside a 420 ms window, landing mean 228 ms / median 214 ms — matching the delay
  stage's ~200 ms model. Replaying `procIgnite`'s truncation and carry-over against the actual tick damage (each
  tick's independent resist roll undone first) matches 36 of 41 checkable applications exactly, across all three
  `remainingTicks` cases; the 5 misses trace to a chain broken by one of the unpaired applications, not a formula
  error. Confirms the code and delay stages' Ignite model live; no further fix needed.

**Warlock** (`TestAffliction`, `TestDemonology`, `TestDestruction`, code)
- Pet crit follows the generic fix (Pets, above): every demon and the infernal move from a
  Classic-testing agility dependency (`AddStatDependency(Agility, MeleeCrit, ...)`, tuned so a
  90-agility demon landed near 5%) to the server's flat 5% base, `MeleeCrit` and `SpellCrit` alike.
  The imp's old `SpellCrit` had no agility term at all (only `MeleeCrit` did), pinning its Firebolt
  crit near the old hardcoded 0.9075%; it's 5% now, most of Destruction's suite delta below. The
  felguard and felhunter were already near 5% at base agility, but the old dependency also picked up
  raid buffs that raise a pet's agility (Blessing of Kings and the like), which
  `Unit::GetUnitCriticalChance` never reads for a non-player unit — that buffed-agility sliver is what
  Affliction and Demonology lose.
- Pet/infernal AP and spell power inheritance (57%/15% of the owner's spell power,
  `spell_warl_generic_scaling`/`spell_warl_infernal_scaling::CalculateAPAmount`/`CalculateSPAmount`)
  now floors to a whole point (`CalculatePct` on the server's `int32`, retail deviation #53), like the
  pet hit/expertise floor already in `sim/core/pet.go` (#12). `Pet.HitScaling` needed no change: both
  already default to `PetHitScalingDefault` (61017/61013), which PAR-P7-0b built generically.
- Guardians taking raid buffs since PAR-P7-0d already covers the infernal: `applyPetBuffEffects` no
  longer special-cases it, and the DK/hunter fixes needed no infernal-specific follow-up.
- Haunt (59164) and Drain Soul (47855) are non-binary on the server (neither's entry in
  `sim/core/serverdata/spells_auto_gen.go` carries `FlagBinary`), which P3-2's sync already applies since
  neither declares `SpellFlagBinary`: confirmed from the generated table, nothing to fix.
- The imp's Firebolt (47964) 1 s GCD is already live: `spells_audit.csv` shows `gcd_ms=1000`,
  `haste_gcd=no` (fixed, not scaled), outside the server's exact [1000,1000] range for the spell, so
  `applyServerData`'s `syncServerTiming` overrides the class code's 1500 ms declaration at runtime.
  `serverdata_allowlist.go`'s entry just documents the conflict for `serverdata_test.go`.
- **Fixed:** Corruption and Unstable Affliction's `CritMultiplier` hardcoded the full Pandemic bonus
  (`SpellCritMultiplier(1, 1)`, doubling crit damage) regardless of the talent, unlike Haunt's own
  `TernaryFloat64(Talents.Pandemic, 1, 0)` beside it. The live spelldump confirms Pandemic's aura-286
  (`SPELL_AURA_ABILITY_PERIODIC_CRIT`) effect covers only Corruption's family flag (`classMask [2,0,0]`,
  spell 58435) and Unstable Affliction's (`[0,256,0]`), not Curse of Agony's (`[0x400,0,0]`) or Drain
  Soul's (`[0x4000,0,0]`); its crit-damage-bonus effect (aura 108) covers the same two plus Haunt's
  flag (`[0,0x40000,0]`). Both spells now gate `CritMultiplier` on `Talents.Pandemic` like Haunt's.
  `TicksCanCrit` is declared throughout: `canCrit` (Corruption, Unstable Affliction), unconditional
  `true` (Immolate, Conflagrate's dot — no talent gates either), `false` elsewhere (Curse of Agony,
  Curse of Doom, Seed, Drain Soul, the infernal's and Metamorphosis's AOE dots — none use a
  crit-capable outcome function). `periodicCritsNeedDeclaration` is still off, so this changes nothing
  yet; it's what P8's flip should read.
- **Fixed:** every warlock spell's `DamageMultiplierAdditive` summed its talent, glyph and set-bonus
  percentages (`1 + a + b + c + ...`), where the server applies each spell mod independently and
  multiplies the results (`Player::ApplySpellMod`) — the bug the Retribution Paladin item already
  found for seals and judgements. The fourteen files with more than one such term now build
  `DamageMultiplier` through a shared `spellModDamage(bonuses...)` helper (`warlock.go`,
  `(1+a)*(1+b)*...`); Immolate's and Conflagrate's DoT-only exclusions (no Firestone on the tick) now
  multiply/divide `DamageMultiplier` by a periodic-only factor instead of adding/subtracting on
  `DamageMultiplierAdditive`, same net effect. Positive percentages multiply to more than their sum,
  so every affected spell hits harder; Destruction (many talented, glyphed, Firestone-imbued fire
  spells) shows most of it.
- **Fixed:** Seed of Corruption's explosion dealt damage under the applier spell's wrapper id
  (`ActionID{SpellID: 47836}.WithTag(1)`, comment noting the real one), so the P3-2 serverdata lookup
  never resolved it and the class code kept a hardcoded 0.286 coefficient (`Spell.dbc`'s own
  `bonusMultiplier`, Classic's value). It now casts under its real id, `SeedExplosionSpellID = 47834`,
  whose live `spell_bonus_data` is 0.2129 (retail deviation #52); `gen_serverdata` picked up the new
  constant cleanly (10 lines: one `spells_auto_gen.go` row, one `bonus_auto_gen.go` row, nothing else
  moved), listed in this wave's contract change requests for the merge-tree regeneration.
- Curses' and Unstable Affliction's base damage and periodic coefficients already matched
  `spell_bonus_data` exactly (Corruption, Immolate, Seed's dot, Unstable Affliction, Drain Soul, Curse
  of Agony, Curse of Doom): confirmed against `bonus_auto_gen.go`, no change needed. Curse of Elements
  and Curse of Weakness already call the shared, PAR-P7-0a-fixed `core.CurseOfElementsAura`/
  `core.CurseOfWeaknessAura`.
- `tools/acore/spellaudit -area warlock`, rerun into `tmp/audit/` (not committed): no missing or
  unresolved warlock spell ids, no manual-row flags beyond the above.
- Suites moved (`dock.sh delta`; pet-only share isolated by temporarily reverting `pet.go`/`inferno.go`
  and re-running):
  Affliction +1.235% to +2.171% (avg +1.406%; `AfflictionTalents` already talents Pandemic, so the
  `CritMultiplier` gate is a no-op here — the whole move is the `DamageMultiplier` multiplicative fix,
  trimmed ~0.15% by the pet crit/AP-SP-floor fixes on its Felhunter);
  Demonology -0.92% to -0.03% (avg -0.64%), entirely from the Felguard pet fixes, since Demonology's build
  carries none of the Affliction- or Destruction-tree talents any changed `DamageMultiplier` term reads;
  Destruction +2.26% to +4.18% (avg +3.15%; isolating the pet fix alone accounts for +0.53% of it, the
  imp's Firebolt crit jump, the rest the `DamageMultiplier` multiplicative fix across Conflagrate,
  Incinerate, Shadow Bolt, Searing Pain, Soul Fire, Shadowburn and Chaos Bolt).
- Recorded run (`TestRecordedRun`, `SIMVAL_RECORD_SPEC=affliction`, human warlock, 500 s at 20 yards,
  `SIMVAL_RECORD_TALENT_SPELLS` set to `AfflictionTalents` decoded against `ui/core/talents/trees/warlock.json`
  plus the Haunt/Unstable Affliction rank chains a talent point alone doesn't teach, Fel Armor pre-cast, a
  fixed 1.5 s cycle of Corruption, Curse of Agony, Unstable Affliction, Haunt, Shadow Bolt, Life Tap): the
  first attempt at 20 yards landed no Corruption, Curse of Agony, Unstable Affliction, Haunt or Shadow Bolt
  cast in 500 s, only the self-targeted Life Tap and the pre-cast Fel Armor, because `standOff()`
  (`mod-sim-validation/e2e/recorded_run_test.go`) backs the character away along the direction
  `spawnInFront` faced it, which runs into a wall a few yards out in this room; `TestRecordedRunHunter`
  already worked around the same wall by moving due north instead, so `standOff()` now does too. The
  recapture (below) lands every ability.

  | Run | Server DPS | Sim DPS | Gap |
  |---|---|---|---|
  | Affliction (`affliction_Svrleadektqu_1790097370`, boss-only, 495.3 s active of 500 s) | 4384.3 | 6766.5 | +54.3%, rotations (below) |

  As with Ret, the fixed 1.5 s cycle has no APL equivalent (rrsim's `afflictionRotation` keeps every dot up
  and casts Shadow Bolt otherwise), so total DPS compares rotations rather than formulas: Corruption needs
  only its first cast all fight, since Everlasting Affliction (5/5 in this build) has a 100% chance to roll
  it over on every landed Shadow Bolt or Haunt, so the priority list spends nearly every remaining GCD on
  Shadow Bolt filler (202.8 casts an iteration against the capture's 55, one slot in its six). Per ability
  (`rrsim -v`, `chronicle -v -source <player> -sim <dps> <log>`; server mean ± standard error, crit rate
  over landed hits):

  | Ability (server non-crits/crits) | Non-crit, server / sim | Crit, server / sim | Crit %, server / sim |
  |---|---|---|---|
  | Corruption (47813; 103/120) | 2974±38 / 2591 (-12.9%) | 6086±69 / 5416 (-11.0%) | 53.8±3.3 / 48.5 |
  | Shadow Bolt (47809; 38/17) | 5972±99 / 5389 (-9.8%) | 12562±243 / 11271 (-10.3%) | 30.9±6.2 / 44.4 |
  | Unstable Affliction (47843; 85/53) | 2182±31 / 2023 (-7.3%) | 4627±68 / 4229 (-8.6%) | 38.4±4.1 / 48.2 |
  | Haunt (59164; 20/12) | 3126±60 / 2848 (-8.9%) | 6276±184 / 5950 (-5.2%) | 37.5±8.6 / 39.2 |
  | Curse of Agony (47864; 222/0) | 568±12 / 978 (+72.2%) | – | 0.0±0.0 / 0.0 |

  - Every non-crit and crit average but Curse of Agony's reads 5-13% low, on both hit types and across
    four unrelated spells alike: the capture's trinket slot held item 49686 ("Maghia's Misguided Quill"),
    1000 flat Spirit, 5-10x a real ilvl-277 piece's stat magnitude here (Dark Coven's pieces give 60-200,
    `item_template`), and not in the sim's item database. `recordedEquipment` (`tools/simval/rrsim.go`) used
    to fail the whole conversion on an unmatched item; it now leaves that slot empty with a warning, same as
    an unmatched enchant, but the ~300 spell power that Spirit alone would add through Fel Armor's 0.3
    Spirit→spell power dependency accounts for most of the shortfall. The item is likely a debug/placeholder
    row the playerbot factory's `initself=epic` shouldn't have handed out; not something this item can fix.
  - Curse of Agony reads 72% high because its ramp (`curses.go`'s `dot.TickCount%4==0` adding half its base
    tick damage every 4th tick) needs about 24-28 s uninterrupted to reach its top tier. The priority list
    only recasts it once `dotIsActive` clears, 20.4 casts an iteration, one every ~24.3 s, its own duration
    with the glyph, so most casts ramp all the way; the capture's fixed lap recast it every ~8.8 s (56 real
    casts) regardless of remaining duration, cutting most ramps off after two or three ticks.
  - Every crit-capable ability's crit-to-non-crit ratio sits at 2.0-2.1x, matching Pandemic's crit-damage
    bonus cleanly; crit% itself differs by more than a couple of points only for Shadow Bolt and Unstable
    Affliction, both within about 2 standard errors of the capture's own small samples (55 and 138 landed
    hits). No sim/warlock formula bug survives this comparison beyond what the code stage already fixed.
**Shaman** (`TestElemental`, `TestEnhancement`, code; PAR-P7-SHA's "shared-enh" and "elemental" stages)
- Stormstrike, the totems, weapon imbues and pet crit fixed as **Retail deviations** 54-59 describe.
  `TestRestoration` doesn't move: shared code, but Restoration wasn't touched.
- Fire Elemental Totem declares a 1 s GCD directly, dropping the `ServerConflictAllowance` entry that
  papered over the mismatch (server data already forced 1 s at runtime).
- P3-2 allowlist closed: Searing Totem (58704), Magma Totem (58734), Flametongue Weapon (58789/58790)
  and the Fire Nova cast (61657) now trigger a separate spell for their real, non-binary hit (58702,
  58735, 10444, 61654) instead of dealing it under their own binary id. The five now-redundant
  allowances are gone (2894's GCD included); `sim/serverdata_test.go`'s Searing Totem case follows.
  **Fixed (PAR-P7-SHA review):** the two totem dots stay declared on the summon spells, with
  `DotConfig.Spell` pointing at the triggered hit, so only their ticks roll under 58702/58735. Moving
  the whole dot broke every APL naming `dotIsActive(58704)` or `dotRemainingTime(58734)`: `GetAPLDot`
  hands the summon to `Spell.Dot`, which indexes an empty `dots` slice and panics the raid build.
  `sim/optimizer/raidctx`'s `raid25.json` fixture and any saved user APL still name the summons, and
  `TestDeriveFixtureGolden` went red on exactly that. The five preset APL files keep the summon ids too.
  **Fixed (wave I cross-review):** the split also has to carry each id's proc side over.
  - The two totem hits went from the summon's `ProcMaskEmpty` to `ProcMaskSpellDamage`, which let a
    totem tick proc the shaman's own trinkets and Elemental Devastation. `Spell::DoAllEffectOnTarget`
    procs on `m_originalCaster`, the totem here, and only crit reads the owner
    (`DoAllEffectOnLaunchTarget`'s `IsTotem` branch), so both are back on `ProcMaskEmpty`. Elemental 30
    rows move, median -1.15%, -9.61% to -0.02% (`LongMultiTarget` most); Enhancement 207 rows, median
    -0.26%, -1.78% to +0.48%.
  - Fire Nova's hit lost `SpellFlagFocusable`, which the cast kept, so its crits stopped granting
    Elemental Focus's Clearcasting. `spell_sha_elemental_focus::CheckProc` refuses only the two weapon
    imbue attacks, so the flag belongs on whichever half deals the damage. Enhancement 195 rows move,
    median +0.09%, -0.56% to +9.73%; no Elemental row, since no Elemental APL casts it.
- AoE cap (`Spell::DoAllEffectOnLaunchTarget`): the ten-target cap gates on `m_caster->IsPlayer()`
  alone, whoever the spell is aimed at. Magma Totem's pulse (58735) is cast by the totem, so it loses
  the cap (deviation 56). **Fixed (PAR-P7-SHA review):** Fire Nova's pulse (61654) keeps it, since
  `spell_sha_fire_nova::HandleDummy` has the *player* cast it, aimed at the fire totem; the code
  stages read "aimed at the totem" as "the totem casts it" and dropped the cap. The fire elemental's
  own Fire Nova (12470, `fire_elemental_spells.go`) had the opposite error, a cap on a guardian's
  cast, and loses it.
- Frostbrand Weapon's hit now deals damage under Frostbrand Attack (58799) instead of the imbue's own
  id (58796); both are binary, so this moves no golden, only the combat-log id.
- Windfury Weapon's hit stays under 58804: the capture's real "Windfury Attack" ids (25504 2H, 33750
  1H/off-hand) share its binary and school data, so nothing breaks, but the split is still owed.
- Side effect of Stormstrike's split, worth knowing when reading an `AllItems` row: the two hits used
  to run through the 17364 spell object, so anything keyed on "a Stormstrike landed" fired three times
  a cast. Totem of the Avalanche (50463) took all three stacks of its 146 AP buff off one Stormstrike;
  now it takes one, which is what the item says.
- TicksCanCrit: Flame Shock declares `false` (no mod-spell-tweaks entry grants it aura 286). Searing
  and Magma Totem reroll the full hit-and-crit table each tick instead of using a snapshotted `Dot`
  outcome, since they're repeated NPC casts, not periodic ticks, so the flag doesn't apply to them.
- Feral Spirit and the fire elemental (Fire Blast, Fire Nova) dropped their manual
  `AutoAttacks.StopMeleeUntil` calls: `RegisterSpell` already applies the server's `ResetsAutoAttack`
  for all three. Removed a stale TODO on the same reset for Maelstrom Weapon's instant Lightning
  Bolt/Chain Lightning: already handled, since `cast.go`'s `resetsSwing` skips it whenever
  `CurCast.CastTime == 0` on a spell that normally has one, which that talent always produces.
- Left open: Lava Flows' dispel-triggered crit buff (`spell_sha_flame_shock::HandleDispel`) isn't
  modeled, only its flat Lava Burst bonus; no suite dispels Flame Shock. Maelstrom Weapon's
  stack-grant rate (a PPM manager) wasn't re-derived against the talent's five rank ids
  (51528-51532); 53817's own `spell_proc` row (`chance: 100, ppm: 0`) describes its T10 4pc bonus,
  not the base grant.
- Nature's Swiftness's cooldown is 2 min on the server (deviation 60), not retail's 3 min; dropped
  the `ServerConflictAllowance` that papered over it. Neither spec's `StandardTalents` takes the
  talent, so it moves no golden.
- Elemental Mastery's own effect (16166, aura 108, the cast-time mod) carries a 30 s duration in the
  capture, separate from 64701's 15 s haste buff; `emAura` had `NeverExpires`, now matches. No golden
  moves: every APL that takes the talent uses it the moment it's off cooldown, well inside 30 s.
- Closed the three remaining placeholder-cast-time allowances: Healing Wave 3 s, Chain Heal 2.5 s,
  Riptide instant. Also dropped Healing Wave's copy of the Totem of the Third Wind bonus
  (42598/42597/42596/42595): the tooltip reads "Increases spell power of Lesser Healing Wave", so it
  shouldn't also land on Healing Wave (`effects_review.csv` flagged it as a sim bug).
  `TestRestoration`'s rotation is `autocastOtherCooldowns` only and never casts these three heals, so
  none of this moves its golden even though the code is shared.
- P6-3 item rows: Skycall Totem's proc is 100 haste rating, not 101 (`effects_review.csv`); Totem of
  Dueling's Stormstrike proc also grants 120 attack power (60766 effect2), which its own tooltip
  omits; the Third Wind family (42598 et al.) is the Healing Wave fix above. **Fixed (PAR-P7-SHA
  review):** Steamcaller's Totem (45114, `heals.go`) gave Chain Heal Classic's 257, not the server's
  243; the code stages read the row as already matching. Both rows are healing-only, so neither
  moves a golden. Still open on the Third Wind row: the Lesser Healing Wave side is 320 spell power
  scaled by the spell's coefficient on the server, where the sim adds a flat heal (338 at 42598).
- Electrified and Lava Burst's bonus dot (T8/T9 4pc: `spell_sha_t8_elemental_4p_bonus` and
  `spell_sha_t9_elemental_4p_bonus` both route through `Unit::CastDelayedSpellWithPeriodicAmount`) now
  refresh through `core.DelayedPeriodicApplier`, the way Deep Wounds does. That function also folds
  the dot's remaining damage into the new tick before recasting it; Electrified already did that on
  refresh, but Lava Burst's dot didn't, so a Lava Burst crit during an active dot used to overwrite it
  instead of adding to it, now fixed. TicksCanCrit false on both (no mod-spell-tweaks entry grants
  either spell aura 286). No suite in this worktree equips the full T8 or T9 4pc set (Elemental's `p1`
  gear predates both tiers, and `AllItems` swaps one piece at a time), so none of this moves a golden
  either.
- This wave's item text calls 71824 (Lava Burst's bonus dot) the T10 4pc. The server disagrees: its
  own aura is named "T9 Elemental 4P Bonus" (`spell_sha_t9_elemental_4p_bonus`), and the item ids
  (ilvl 200/225/232/251 for Earthshatter/Worldbreaker/Thrall's/Frost Witch's Regalia, matching
  Naxx/Ulduar/ToC/ICC) put Thrall's Regalia at T9 and Frost Witch's at T10, which already carries a
  different 4pc bonus (the Flame Shock tick extension). `ItemSetThrallsRegalia`/`ItemSetWorldbreakerGarb`
  already gate the right bonus on the right spell; left unchanged.
- **Goldens** (`dock.sh delta`): every suite moves a few thousandths of a percent from the Searing
  Totem/Fire Nova coefficient fixes alone (`spell_bonus_data`: 0.1667, 0.214, was 0.167, 0.2142).
  Bisecting found two bigger drivers:
  - Shamanistic Rage's PPM, 15 → 18 (`spell_proc`), the largest: better mana sustain during its 15 s
    window moves every suite that takes the talent, single-target included
    (`WF-default_wf-NoBuffs-LongSingleTarget`: 3048.97 → 3230.97 DPS, about +6%).
  - Magma Totem's dropped AoE cap, `LongMultiTarget` only (20 targets):
    `FT-default_ft-NoBuffs-LongMultiTarget` moves from 12613 to 10756 DPS with just the cap reverted.
    The pet crit fix adds a further point or two, largest on `EleFireElemental` and Feral-Spirit-heavy
    cases.
  - Ruled out by the same bisection: the Flametongue/Frostbrand id changes, the Lava Lash imbue-hand
    fix, the Stormstrike block fix.
  - The elemental stage's own bisections, on top of the above: Totem of Dueling's 120 AP is the only
    further driver worth naming, and only for Enhancement (it's in `p1`'s default gear, so every suite
    gets it): `WF-default_wf-NoBuffs-LongMultiTarget` moves from 14000.18 to 14274.23 DPS (+1.96%) with
    just the AP added back in, on top of the shared-enh stage's own 11016.59 → 14000.18. Everything
    else this stage touched — Nature's Swiftness, Elemental Mastery's window, Skycall's -1 haste, the
    heal cast times, and Electrified/Lava Burst's dot delay — bisects to zero on both `TestElemental`
    and `TestEnhancement` (sorted DPS/TPS/HPS lines byte-identical with and without each change).
  - **PAR-P7-SHA review:** Spirit Wolves' haste-carrier wiring and AP fix (this section, above) move
    every suite a bit further, on top of the above. Direction and size vary with how much gear haste the
    build carries and how big a share of total damage the wolves are: most rows move another -0.5% to
    +2%, `AllItems` rows that swap in a haste-rating or haste-proc piece up to +9% (the wolves benefit
    from it now, where before they didn't), and `LongMultiTarget` rows the least in relative terms (the
    totem/AoE damage the fix doesn't touch dominates those). See the bisected `WF-default_wf-NoBuffs-
    LongSingleTarget` example above for the isolated size of each half of the fix.
  - **PAR-P7-SHA review, the two AoE cap fixes**, bisected against the run with both reverted. Only
    `LongMultiTarget` rows (20 targets) move, and the totem dots going back on the summon spells move
    nothing at all. Fire Nova's restored cap takes all 24 Enhancement `LongMultiTarget` rows down
    -14.6% to -22.1% (`WF-default_ft-FullBuffs-LongMultiTarget`, Troll: 28876.42 → 22492.91). The fire
    elemental's dropped cap lifts the 8 `EleFireElemental-*-LongMultiTarget` rows +19.0% to +31.4%
    (Orc `default-NoBuffs`: 5221.37 → 6858.86), and no other Elemental row.
  - Against the wave base, before the cross-review's own three shaman fixes: Elemental 183 rows, median
    -0.010%, -2.46% to +31.4%; Enhancement 207 rows, median +3.2%, -7.5% to +9.0%. The cross-review then
    moves Elemental another median -1.15% (30 rows) and Enhancement another median -1.11% (all 207 rows,
    -4.04% to +9.77%).
- Live probes (`TestSimvalShaman`): the Stormstrike cast's yellow table, its two hits skipping the
  partial block, Lava Burst's and Flame Shock's magic table, and Flame Shock's dot carrying no aura 286
  all match the sim unchanged; numbers under **Verified on the live server**. `sim/serverdata_test.go`
  now pins the two hits' always-hit and no-active-defense flags, since only those keep
  `OutcomeMeleeSpecialCritOnly` from rolling a partial block in front of the target.
- Suites run: `TestElemental`, `TestEnhancement`, `TestRestoration` (unchanged), `sim`, `sim/core`,
  `sim/optimizer`, `sim/optimizer/raidctx`, `tools/...`, `cmd/...`; `tools/simval` 508/508; `go vet`;
  `gofmt`; `BenchmarkSimulate` on all three shaman packages; `tsc`; `gen_serverdata` (idempotent, no
  new constants). `TestElemental` and `TestEnhancement` are the only red ones, on golden DPS, which is
  the point. The UI APL files end unchanged against the wave base, so there is nothing for eslint to
  re-check.
**Druid** (`TestBalance`, `TestBalancePhase3`, `TestFeral`, `TestFeralApl`, `TestFeralTank`, code)
- Moonfire/Insect Swarm add-ticks and tick crit are mod-spell-tweaks carrier auras
  (`modules/mod-spell-tweaks/data/sql/db-world/base/druid/spell_moonfire_insect_swarm_eclipse_haste_scriptname.sql`,
  `spell_425791_balance_dot_crit.sql`), gated on the Eclipse and Earth and Moon talents respectively
  (`SpellTweaks_classes.cpp:400-512`), both behind `SpellTweaks.BalanceDotScaling.Enable`. The sim gated
  Moonfire's tick crit on the T9 2pc bonus alone (a real, separate carrier: `sets_review.csv` row 39) and
  Insect Swarm's not at all; both now check `druid.Talents.EarthAndMoon` too, and the tick interval hastes
  with `druid.Talents.Eclipse` (`core.SpellHasteAddsTicks`, matching the script's own clamp at 1×).
- **Eclipse's proc condition:** `spell_dru_eclipse::CheckProc` (`spell_druid.cpp:1486-1519`, core AC script,
  not a module) rolls on every landed Wrath or Starfire cast, not just a crit; the chance is the talent's
  own `ProcChance` (33/66/100% by rank, `spelldump.jsonl` ids 48516/48521/48525), full for Starfire and
  ×0.6 for Wrath. The sim's chances already matched that (1/3 and 0.2 a point), so only the crit
  requirement was dropped, in both `OnSpellHitDealt` handlers of `applyEclipse` (retail deviation #61).
- **Typhoon** deals its damage under the cast id (61384); the server triggers it from 53227
  (`spells_auto_gen.go`), which carries the binary flag and the Speed 30 missile the cast doesn't. Split
  into a cast spell and a nested damage spell registered under 53227, the way `sim/deathknight`'s Death
  Coil splits its cast from `DeathCoilDamageActionID`.
- **Typhoon's and Hurricane's crit multiplier** was 1, so both rolled a crit and paid nothing for it.
  Both ids are `critCapable` in the spelldump; both now take `BalanceCritMultiplier()`. No balance APL
  casts either, so no golden moves.
- **Force of Nature's treants** followed no server formula at all (a flat 50% of the owner's spell power
  as Strength). `spell_dru_treant_scaling` (`spell_druid.cpp:389-424`, aura 35669) gives 30% of the
  owner's Intellect and Stamina and attack power at 105% of the owner's spell power, with Brambles adding
  its own percent to that AP conversion rather than a flat treant damage bonus (the sim's old
  `PseudoStats.DamageDealtMultiplier` hack). The resistance and PLAYER_PET_SPELL_POWER-display shares of
  that same aura aren't modeled: neither affects a DPS sim. Pets, guardians and raid buffs already covers
  why the treants needed no `isGuardian` change beyond the classification itself: `applyPetBuffEffects`
  runs for every `PetAgent` regardless.
- **Languish** (Lasherweave Regalia 4pc) now refreshes through `core.DelayedPeriodicApplier`, on the same
  `Unit::CastDelayedSpellWithPeriodicAmount` path as Deep Wounds (DoTs and periodic ticks, above).
- **Idol of the Crying Wind (45270):** `spell_dru_insect_swarm::CalculateAmount` (`spell_druid.cpp:2008-2013`)
  adds the relic's 374 to the tick's own base points, split evenly over the ticks; the sim ran it through
  the dot's 0.2 SP coefficient instead (79/tick instead of ~62). Known gap: the server divides by
  `AuraEffect::GetTotalTicks()`, read after `CalculatePeriodic` has already shortened the period, so with
  Eclipse the relic's total stays 374 however hasted the dot is, while the sim divides by the unhasted 6
  or 7 and pays the haste factor on top. No preset equips the idol, so no golden sees it.
- **Idol of the Corruptor (45509):** the agility proc is 153, not 162 (spell 64951's own effect1); Mangle
  (Bear)'s proc chance is 100%, since its `spell_proc` row (64952) has `Chance=0` and falls back to the
  DBC's default, the same as Mangle (Cat) already had.
- **Moonfire's direct hit** used a 0.15 spell power coefficient; `spell_bonus_data` gives it 0.13, matching
  the dot's own coefficient (retail deviation #62).
- Percent `SPELLMOD_DAMAGE`/`SPELLMOD_DOT` mods multiply (`Player::ApplySpellMod`, sim/paladin's
  `spellModDamage`): Moonfire's Improved Moonfire and Moonfury, Insect Swarm's Genesis/T7 2pc/glyph, and
  Hurricane's Gale Winds/Genesis were summed instead. Starfire and Wrath already multiplied theirs; moved
  to the same `spellModDamage` helper for consistency. Glyph of Moonfire (54829) goes through it too: its
  two halves are percent mods of the same kind, -90% `SPELLMOD_DAMAGE` on the hit and +75% `SPELLMOD_DOT`
  on the tick, so they multiply with the talents rather than being added to and subtracted from them.
  Which mod reaches which is `Unit::SpellDamageBonusDone`'s `damagetype == DOT ? SPELLMOD_DOT :
  SPELLMOD_DAMAGE` (`Unit.cpp:8985`); Moonfury and Improved Moonfire list Moonfire under both, Genesis
  only under `SPELLMOD_DOT`.
- `TicksCanCrit` declared for Moonfire, Insect Swarm (both above) and Languish (`false`: no
  `SPELL_AURA_ABILITY_PERIODIC_CRIT` aura covers it).
- Faerie Fire (Feral)'s wrapper (60089, the other P3-2 allowlist entry from the class row) is Feral, not
  Balance: left for the next stage.
- **Faerie Fire (Feral)'s Bear Form damage** was dealt under 16857 itself, in one binary hit roll. The
  server only puts the armor/dodge-parry debuffs there; in Bear or Dire Bear form it triggers 60089 for
  the damage (`Spell::PrepareTriggersExecutedOnHit`, `Spell.cpp:8916-8921`, `Spell::DoTriggersOnSpellHit`,
  `Spell.cpp:3260-3262`), which resists partially. Split the same way Typhoon splits its cast from its
  damage id, dropping the `ServerBinary` allowlist entry the old single-spell version needed (retail
  deviation #63).
- **Omen of Clarity's proc chance** was WotLK Classic community math: cast time / 60 × 3.5, a 0.666
  "instant suppression" factor, and hand-tuned multipliers for Typhoon, Moonfire and Hurricane. The server
  rolls its own `spell_proc` row (16864: 3.5 PPM, no fixed chance) the way every other proc-trigger aura
  does, which `core.ServerProcFor`/`AuraPPMProcChance` (`sim/core/ppm.go`) already model. Hurricane's tick
  spell already casts once per tick on its own (`sim/druid/hurricane.go`), so the generic roll needs no
  Hurricane special case; the "fake" Gift of the Wild clearcast-fisher spell (`sim/druid/fake_gotw.go`)
  still needs its `1-(1-p)^30` stand-in, since it fires the hit callback once per cast where the real
  spell hits around 30 raid members (retail deviation #64).
- **Faerie Fire (Feral)'s guaranteed Clearcasting** was gated on Glyph of Omen of Clarity (206580), a
  WotLK Classic glyph missing from the capture, the same as Warrior's Glyph of Shattering Throw (206953).
  The real mechanism is mod-spell-tweaks' `spell_tweaks_omen_faerie_fire` (**Server customizations**,
  above), which always procs it off a landed hit on a non-player target, no glyph needed. Wired to
  `Server().SpellTweaks.OmenClarityFaerieFire` and `result.Target.Type == core.EnemyUnit`; dropped the
  glyph from `ui/feral_druid/presets.ts`'s default build, the same as the Warrior preset.
- **Lacerate's initial hit** added a 0.01 AP coefficient; `spell_bonus_data` gives 48568 `ap: 0` on the
  direct effect (`apDot: 0.01` is the tick's, already right). Dropped the direct hit's AP term.
- **Berserk** (`spell_dru_berserk::HandleAfterCast`, `spell_druid.cpp:1226-1256`) drops an active Tiger's
  Fury and clears Mangle (Bear)'s cooldown; the sim did neither. Mangle (Bear) staying off cooldown for
  the rest of Berserk (50334's flat -6000 ms `SPELLMOD_COOLDOWN`) was already modeled in
  `sim/druid/mangle.go`. Tiger's Fury keeps its `ExtraCastCondition`: every rank carries
  `ExcludeCasterAuraSpell` 50334 in `Spell.dbc`, which `Spell::CheckCast` (`Spell.cpp:5793`) enforces, so
  a recast during Berserk fails. `spell_dru_tiger_s_fury` has no `CheckCast` of its own, which is why the
  block is easy to miss in the scripts.
- `TicksCanCrit` declared for Rake (Lasherweave Battlegear 4pc only), Rip and Lacerate (Primal Gore),
  matching the outcome function each already picks.
- `CurrentMangleCatCost` dereferenced a nil `MangleCat` on a build without the Mangle talent. The default
  cat rotation's `InFrontOfTarget` branch and the `CatExcessEnergy` APL value's Mangle-refresh pooling
  (`sim/druid/feral/{rotation,apl_values}.go`) both read it off a bleed/positioning condition that doesn't
  imply Mangle is talented; the AOE rotation's Mangle-builder-cost pick (`rotation_aoe.go`) read it inside
  a `core.Ternary` call, which evaluates both arguments regardless of the condition. All three now check
  `MangleCat != nil` first.
- Snek weave's swing-timer reset (`preRotationCleanup`) didn't model Albino Snake's own GCD; it now also
  sets the caster's GCD timer forward by `core.GCDDefault`.
- **Live probes** confirmed every premise above and moved no sim code: the two split damage ids (60089
  always hits and resists partially, 53227 is binary), Omen's own 3.5 PPM entry with both of its PPM
  bases, and the periodic-crit scoping of the Earth and Moon carrier and of Primal Gore. Numbers under
  **Verified on the live server**.
- **Goldens** (`dock.sh delta` against the wave base), bisected by reverting one change at a time:
  - `TestBalance` 148 rows, median +12.4%, +4.4% to +39.7%; `TestBalancePhase3` 136 rows, median +10.8%,
    +5.3% to +20.0%. Two drivers, compounding: the mod-spell-tweaks balance dot scaling (haste adds ticks
    with Eclipse, tick crit with Earth and Moon) is the larger at a median +8.3% on both, and Eclipse
    rolling on every landed cast (deviation 61) adds a median +3.5% and +2.1%. The rest, under a percent,
    is Moonfire's coefficient, the multiplicative spell mods and Omen's PPM. The `NoBuffs` rows move most,
    since low crit is what the old crit-only Eclipse punished.
  - `TestFeralTank` 136 rows, median +3.8%, +2.3% to +24.7%, nearly all of it Faerie Fire (Feral)'s
    guaranteed Clearcasting (+4.4% median alone): the tank build never had Glyph of Omen of Clarity, which
    is what used to gate it.
  - `TestFeral` 149 rows, median -0.7%, and `TestFeralApl` 136 rows, median -0.4%. The cat build already
    had that glyph, so the Clearcasting rewiring is a wash there and what is left is the Faerie Fire damage
    split and Omen's PPM.

**Items** (`docs/azerothcore-item-diff/data/summary.md`)
- 1509 of 8043 sim items differ.
- Classic raised Ulduar/emblem item levels, e.g. 226→232 on 329 items and 239→252 on 92.
- Trinket values differ: Mjolnir Runestone 45931 is 665 ArP on the server vs 751 in the sim; Flare of the Heavens 45518 is 850 vs 959.
- **Fixed (PAR-P7-0d):** Black Bow of the Betrayer's (32336) proc, `spell_gen_black_bow_of_the_betrayer`
  (`spell_generic.cpp`), has no class check, so a warrior in the ranged slot triggers it same as a
  hunter; the sim's hunter-only cast panicked the optimizer whenever it tried the bow on any other
  class. Now keyed off the wearer's `Character`.
- **Fixed (PAR-P7-0d):** Rune of Razorice's Frost Vulnerability (51714) aura was one `GetOrRegisterAura`
  call keyed by label alone, so two Razorice DKs on the same target shared one stack count. The server
  looks an existing aura up by `(spellId, casterGUID)` (`Unit::_TryStackingOrRefreshingExistingAura`,
  `Unit.cpp:4661`) unless the spell carries `SPELL_ATTR0_CU_SINGLE_AURA_STACK`, which 51714 doesn't, so
  each caster keeps its own count there. The label now folds in the caster.

**Character stats (PAR-P7-0f)** (`applyAllEffects`, `sim/core/character.go`)
- Before finalize, `character.stats` holds no stat deps, while `AddStatsDynamic` adds a build-phase aura's
  bonus with them applied. So the phase snapshots, which only the character stats tooltip reads, multiplied
  Blessing of Might and Battle Shout twice by a 10% AP buff, and Commanding Shout by Strength of Wrynn. They
  now take those bonuses raw.
- `AddStatsDynamic` applies the deps in list order, where those added since the last sort sit last. Might's
  AP went on through Mental Quickness's or Sheath of Light's AP→SP before the buff's ×1.1, and came off in
  sorted order, ×1.1 first. That left Enhancement's and Retribution's spell power, FinalStats and sim alike,
  short by 3% of Might's AP (20.61 with Improved Blessing of Might). The server converts the total AP,
  multiplier included (`Unit::SpellBaseDamageBonusDone` reads `GetTotalAttackPowerValue`, `Unit.cpp:9156`).
  The deps are now sorted before each phase's auras go on: `TestEnhancement` +0.16 to +0.30%,
  `TestRetribution` +0.05 to +0.15%, final spell power +20.61 and no other stat.

**Class sets and item effects (PAR-P7-0f)** (`core.ClassEffect`, `sim/core/item_effects.go`)
- Class set bonuses and item effects cast the wearer to their class's agent, which panicked for another
  class, e.g. a DK in Tidefury Raiment. The server gives any wearer the aura, but it only touches that
  class's spells, so `core.ClassEffect` now gives another class nothing.
  `TestSetsAndClassItemEffectsOnEveryWearer` (`sim/item_wearers_test.go`) sims every set and class item
  effect on each class whose armor and weapon types fit it, except the class an allowlist names.

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
- Proc data (`tools/simval` over a `.simval procs` capture, 102 checks): every generated `spell_proc` row matches the
  live entry field for field, `core.ServerProcFor` reproduces the chance the server computed, and the PPM basis picks
  the main hand for a melee-class spell, the ranged slot for Steady Shot and max(base cast, 1.5 s) for Frostbolt.
- Client vs server talent data (`tools/acore/talentdiff`): the live server's `Talent.dbc`, `TalentTab.dbc`,
  `GlyphProperties.dbc` and talent and glyph rows of `Spell.dbc` match stock. The user's **client** differs: talents
  1341 and 1818 swap rows in tab 363 (the hunter Marksmanship tier swap), glyph 912 is new, and 26 talent-spell fields
  move. But the server lays mod-spell-tweaks' `acore_world.*_dbc` rows over the DBC files talentdiff reads, so most of
  it reaches the server (the spelldump shows it): Virulence 2/4/6% spell hit, Nerves of Cold Steel 2/4/6% hit, Rage
  of Rivendare 2-10 expertise, Runic Power Mastery's aura 286 `ABILITY_PERIODIC_CRIT` on Frost Fever, Boar's Speed's
  second effect, and `talent_dbc`'s tier swap. Client-only: Vicious Strikes' aura 286 (the server's is on Crypt Fever
  instead) and Call of the Wild (53434) at 120 s, which stays 300 s.
- Recorded runs (`TestRecordedRun`, 5 minutes on the boss dummy inside Naxxramas, in playerbot-factory epic gear;
  captures in `sim/core/testdata/chronicle/`): a Protection paladin did 144.7 DPS over 295.7 s with swing intervals of
  1.5-1.6 s — 143.7 of it on the dummy, the rest Consecration splashing a nearby Maggot, so a sim comparison wants
  `-target`. Glancing landed on 39 of 185 swings, within a standard error and a half of the white table's 2500 bp.
  Chronicle timestamps the packet send, so the 100 ms map-update lattice only shows through a few ms of jitter — a tick
  or swing interval reads within about ±50 ms of the server's own.
  Correction: it ran at ×0.3 damage. The factory's init resets mod-individual-progression, and below
  `PROGRESSION_PRE_TBC` the live `VanillaPowerAdjustment` of 0.5 scales a level 80's damage by 1 − 0.5·70/50
  (`ComputeVanillaAdjustment`). `TestRecordedRun` doesn't undo it yet; `TestRecordedRunHunter` runs `.ip set <name> 18`
  after gearing. The Affliction capture (`affliction_Svrleadoxfbj_1789926928`) had the same ×0.3 bug plus the
  playerbot factory's own talents staying live under the learned ones; PAR-P7-WLK replaced it with a fresh capture
  (Warlock findings, above).
- Ret Paladin recorded run (`TestRecordedRun`, `SIMVAL_RECORD_SPEC=ret`, `SIMVAL_RECORD_TWOHAND=1`, 600 s): Divine
  Plea keeps every mana-cost spell casting to the end, Retribution Aura keeps Sanctified Retribution (63531) up,
  and the pre-fight `.simval info` matches rrsim's stats. Per-ability comparison in the Retribution Paladin
  findings above.
- Hunter recorded runs (`TestRecordedRunHunter`, `SIMVAL_RECORD_HUNTER`, 300 s at 20 yards; the factory hunter gets a
  ranged weapon, quiver and ammo, and its pet autocasts only its damage spells). It keeps Serpent Sting up and fires
  Arcane Shot on cooldown, else Steady Shot, with Rapid Fire, Kill Command and Bestial Wrath on cooldown. Each capture
  is compared with `tools/simval chronicle -sim` against `tools/simval rrsim` on its `.setup.txt`; all three are in
  testdata. The SV runs came before the harness cast Track Giants, so their sims drop Improved Tracking
  (`-notracking`). Correction: the first sims, by PAR-P7-HUN's scratch converter, read race 4 as the sim's Gnome
  rather than Night Elf; as night elves each sim loses about 0.2%, which the table has.

  | Run | Server DPS | Sim DPS | Gap |
  |---|---|---|---|
  | BM + serpent (`hunter_Svrleadtbrfb_1789993143`) | 5925.4 | 5842.4 | +1.42% |
  | SV + bat (`hunter_Svrleadtntyo_1789992248`) | 4784.8 | 4785.7 | -0.02% |
  | SV + wasp (`hunter_Svrleadhbjyz_1789991622`) | 5008.1 | 4886.6 | **+2.49%**, over the 2% |

  Correction (PAR-P7-0d): sim DPS was 5816.5/4765.4/4862.4 (+1.87%/+0.41%/+3.00%) before the queued-cast fix below;
  the ~0.1 pt moves are re-sim noise (3000 iterations, no fixed seed), not the fix's effect on these totals.

  Correction (PAR-P7-0d pets): sim DPS was 5813.4/4763.8/4863.7 (+1.93%/+0.44%/+2.97%) after the pets stage's 2 s
  haste resnapshot (below), moving the hunter pet's share of each total by a point or two; still noise-sized, and
  the wasp run's gap is still the open crit question, not this stage's pet work.

  Correction (H3 cross-review): rrsim now adds, from the live DBC, the shoulder enchant 3776 the sim's item database
  lacks (+45 AP, +15 crit rating), which gives the table's numbers.

  - Auto Shot intervals sit on the 100 ms lattice at `NextServerTick` of the hasted speed.
  - **Fixed (PAR-P7-0d):** `cast.go` rounded `CastTime` to the server tick but not `GCD`, so a hasted GCD or cast
    time floating a hair over a round ms (the server truncates both to whole `int32` ms first,
    `Spell::TriggerGlobalCooldown`/`Unit::ModSpellCastTime`) crossed a tick boundary the truncated server value
    didn't, costing the chain a spare 100 ms — the wasp run's queued Steady Shots at 1.6 s where the server's ran
    1.5 s. `castTiming.gcd`/`.castTime` now truncate too. AzerothCore's SpellQueue never enters into it: playerbot
    casts go straight through `Spell::prepare`, not `Player::CanRequestSpellCast`.
  - BM run: the server pet idled through 2 of its 3 Bestial Wraths (10.6 s and 11.1 s): it kept its target and
    ignored attack commands, with no swings or autocasts until the aura dropped. The one at the pull didn't do it.
    Cause unknown. It's why the server pet did 5% less than the sim's.
  - **Checked, no term found (PAR-P7-0d):** `UpdateCritPercentage`/`GetUnitCriticalChance` match the sim's
    `PhysicalCritChance`/`MeleeCritSuppressionPct` term for term. Lethal Shots (ranged-only, weapon-gated) already
    accounts for the whole ranged-vs-melee sheet split in both available live captures (+2%/+5% exactly, at rank 2
    and rank 5). Careful Aim, Survival Instincts (classMask excludes Auto Shot on the server too) and Kill Command
    carry no crit term; Glyph of Steady Shot has no server script. All three shots share one outcome function.
    Pooling hits across the captures against the current code reproduces the claim: Auto+Steady +2.60 points
    (1.66σ; +2.91 before enchant 3776's crit rating), Arcane -0.96 (0.26σ, matched). Left alone; a live `.simval`
    probe would settle whether it's real.
- Death knight probes (`TestSimvalDeathKnight`, a human DK behind the boss dummy): Scourge Strike and Obliterate
  roll the yellow table, Icy Touch the magic one with partial resists, and `tools/simval` passes all 26 checks on
  those records. Both diseases are melee damage class and can't miss. Rage of Rivendare 5/5 adds 10 expertise and
  Virulence 3/3 takes 600 bp off Icy Touch's miss threshold, 2 a point as the spell_dbc rows say. 47632 always hits
  and resists partially. The Death Coil cast (49895) never misses the dummy: it's a positive spell, which
  `Unit::SpellHitResult` lets miss only a hostile target, and the dummies' faction 7 isn't, so on a raid boss it
  rolls the magic table. `tools/simval` doesn't model that, `ALWAYS_HIT` or a binary spell's lack of partials,
  so those records fail it.
- Rogue probes (`TestSimvalRogue`, a human rogue behind the boss dummy): `.simval spell 57970` confirms Deadly
  Poison is damage class 1 (magic), with its miss and resist buckets matching the server's own thresholds over
  200,000 iterations (|z| ≤ 0.93); `.simval yellow 48672` confirms Rupture is damage class 2 (melee), miss and
  dodge matching (|z| ≤ 1.44). A `.simval spelldump` of 45176, 14159 and 30920 confirms Master Poisoner (45176)
  carries `MOD_CRIT_CHANCE_FOR_CASTER` (aura 308) and Murder rank 2 (14159) carries `ABILITY_PERIODIC_CRIT`
  (aura 286) on Deadly Poison's class mask, as the Rogue findings above assume. Weapon Expertise rank 2 (30920)
  carries only `MOD_EXPERTISE` (240), no crit aura: Rupture's crit stays the engine's hardcoded, talent-blind
  case (retail deviation #27), so `sim/rogue/rupture.go`'s unconditional `TicksCanCrit: true` needed no change.
  **Fixed:** `p7_rog_test.go`'s original check expected Weapon Expertise to carry that same aura, which the
  live spelldump disproved; it now asserts the aura's absence instead.
- Warrior probes (`TestSimvalWarriorP7`, human warrior vs. the boss dummy): Rend's yellow table dodges
  (5.70%) and parries (13.25%) but never blocks — no `SPELL_ATTR3_COMPLETELY_BLOCKED` — at the plain
  8.00% miss and 4.40% partial block; Shattering Throw only misses (8.00%), no dodge/parry/block,
  matching its `OutcomeMeleeSpecialNoBlockDodgeParry`; Heroic Strike's yellow miss is the same 8.00%
  (asserted in `TestSimvalWarrior`). The bot wields one weapon, so that doesn't exercise the dual-wield
  penalty; `MeleeSpellMissChance` only adds it when `spellId == 0`. `.simval spelldump` on
  47465/46854/46855/12721/64382: Rend's periodic-damage effect ticks every 3000 ms for a 15000 ms duration, carries family flag 0x20, and has `spell_tweaks_rend_haste` bound;
  Trauma (both ranks) carries the 286 periodic-crit override scoped to that same flag, plus its own 42
  proc-trigger effect; Deep Wounds' periodic spell (12721) has a periodic-damage effect and no
  periodic-crit one; Shattering Throw's base cast time is 1500 ms. All match the sim unchanged.
- Mage (`TestSimvalMage`, human mage vs. the boss dummy): Frostbolt, Fireball and Living Bomb's explosion all roll
  the magic table with partial resists; Living Bomb's dot itself carries no periodic-crit effect. A 295.7 s Fire
  capture (`fire_Svrleadggztq_1790099652`) checks Ignite's delay and tick-damage model against ~80 live
  applications (Findings, Mage).
- Shaman probes (`TestSimvalShaman`, an orc shaman in front of the boss dummy): the Stormstrike cast
  (17364) rolls the yellow table (miss 7.97%, dodge 6.55%, parry 13.89%, partial block 4.40%) and never
  crits, as `OutcomeMeleeSpecialHit` has it. Its two hits (32175/32176) are always-hit and took 0 partial
  blocks in 300,000 rolls each: `isSpellBlocked` refuses an always-hit or a no-active-defense spell
  (`Unit.cpp:3287`) and both ids carry both attributes, which is what `OutcomeMeleeSpecialCritOnly` and
  the generated flags rely on. Lava Burst (60043) and Flame Shock (49233) roll the magic table with
  partial resists, missing 16.83% and 16.72% against the 1700 threshold, buckets and mean resist matching;
  a `.simval spelldump` of 49233 finds no aura 286, so its dot's `TicksCanCrit: false` stands. All match
  the sim unchanged.
  **Fixed:** the probe first asserted the record's `noActiveDefense` on the two hits, but
  `DeriveYellowTable` returns on ALWAYS_HIT before filling that field or `partialBlockChance`, so both
  read 0 whatever the spell carries. It now asserts the empty block roll, which is what the record proves.
  The off-hand hit's crit reads 0 here and means nothing: 32176 needs an off-hand weapon, and an empty
  off hand makes `GetWeaponSkillValue` 0 (`Unit.cpp:3970`), which costs `UpdateCritPercentage` 16 points
  (`StatSystem.cpp:686-687`). The main hand read 3.174%, matching the sheet.
- Druid probes (`TestSimvalDruid`, `TestSimvalDruidOmen`, a naked night elf druid in front of the boss
  dummy, 300k rolls a probe and 100k for 16857): Moonfire and Starfire are damage class 1 with partial
  resists at the plain 1700 bp miss threshold; Shred is class 2 at miss 800, dodge 645, parry 1400, no
  block, partial block 440 bp and the 0.6 crit suppression. Both ids the sim splits off their casts
  behave as the split assumes: Faerie Fire (Feral) 16857 is binary and takes no partial resist, the
  60089 it triggers never misses and resists partially (mean 3.62% against the derived 3.6145%), and
  Typhoon's 53227 is binary. `.simval spelldump`: the Earth and Moon carrier (425791) holds aura 286 on
  Moonfire's and Insect Swarm's class masks while Moonfire holds none of its own, Primal Gore (63503)
  holds it on Rip's and Lacerate's, and Eclipse grants 40% Wrath damage (aura 108, SPELLMOD_DAMAGE) and
  40 Starfire crit (aura 107, SPELLMOD_CRITICAL_CHANCE). `.simval procs` reads Omen of Clarity (16864)
  as 3.5 PPM with no flat chance, 16.917% off the 2.9 s weapon both for a white swing and for Shred
  (melee class) and 8.750% for Moonfire (the 1.5 s floor), what `core.ServerProcFor`/`AuraPPMProcChance`
  compute. `tools/simval` failed 10 of 78 checks on the records, all two known gaps of its own checker:
  `ALWAYS_HIT` (60089's miss threshold) and a binary spell's lack of partials (16857, 53227), the same
  ones the DK probes hit. **Fixed (wave I cross-review):** `check.go` expects no miss on an always-hit
  spell and one unresisted bucket on a binary one, so both read the same as the sim. Nothing disagreed
  with the sim.
- A non-binary magic hit lands fully resisted about once in 300k rolls (seen twice in 900k):
  `Unit::CalcAbsorbResist` builds its eleven discrete resist probabilities as floats, and a roll in the
  rounding gap above their sum walks the loop to the last bucket (`Unit.cpp:2360-2382`). Worth ~2e-6 of
  damage, so the sim models nothing; `p7_dru_test.go` drops that bucket before checking the derived
  distribution, since the harness derives it as exactly 0.

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
| 28 | Swings during a hardcast that doesn't reset them | held: a swing due mid-cast goes on the update the cast lands, after its effects. Melee waits out any cast; Auto Shot only one without `ATTR2_DO_NOT_RESET_COMBAT_TIMERS`, but always follows a cast landing on its own update. With that attribute, a player's melee timers stand still for the updates the cast spans (Slam) | swings go on mid-cast, and before a cast landing the same moment; Slam pushes them back by its unrounded cast time | `Player::Update`, `Unit::Update` (`suspendAttackTimer`, spell events before `_UpdateSpells`), `Unit::IsNonMeleeSpellCast`, `UnitAI::DoMeleeAttackIfReady` |
| 29 | Auto Shot vs melee timers | every Auto Shot restarts both | untouched | `Unit::_UpdateAutoRepeatSpell` |
| 30 | Off hand at the pull | a ready off hand waits max(own timer, main hand timer + half the main hand's hasted attack time) | a random hand waits a random 0-50% of the main hand's weapon speed | `Unit::Attack` |
| 31 | Weapon swap | swing timers keep running; the new weapon only changes the attack time | both melee timers restart | `Player::_ApplyWeaponDamage` |
| 32 | Death Coil with Sigil of the Wild Buck | +80 twice: the flat modifier covers the dummy (49895), whose value becomes the damage spell's custom base points, and the damage spell (47632) | +80 once | `Unit::ApplyEffectModifiers`, `spell_dk_death_coil` |
| 33 | Pet crit | 5% plus crit auras, melee and magic (spell crit auras only), none from agility | agility crit (the hunter pet 3.2% plus agility, melee only) | `Unit::GetUnitCriticalChance`, `Unit::SpellDoneCritChance` |
| 34 | Gargoyle casting | a cast starts on a 400 ms decision, 80% of the time | back to back | `npc_pet_dk_ebon_gargoyle` |
| 35 | Ghoul Claw and the army | the guardian ghoul Claws every 5-10 s, the pet ghoul from 75 energy; army ghouls never Claw and have 24 AP plus 6.5% of the owner's | Claw on energy; the army's AP includes agility | `CombatAI`, `PetAI::UpdateAI`, `AggressorAI`, `Guardian::UpdateAttackPowerAndDamage` |
| 36 | Razor Frost | 2% of a main-hand swing, attack power included, whichever weapon procced it; Frost Vulnerability only helps its caster's frost spells | 2% of the procing weapon's base damage; the vulnerability helps all frost damage | `Spell::EffectWeaponDmg`, 51714's `MOD_DAMAGE_FROM_CASTER` class mask |
| 37 | Pestilence with Glyph of Disease | `Aura::RefreshDuration`: the target's diseases keep their amount, crit and tick interval, and restart at their max duration, Glyph of Scourge Strike's extensions included | the refresh takes the current attack power and haste | `spell_dk_pestilence` |
| 38 | Wandering Plague | 1 s cooldown after a proc | 0.5 s | `spell_dk_wandering_plague_aura` |
| 39 | Volley and Auto Shot | the ranged timer stands still through the channel; a shot already due still goes | Auto Shot cancelled, restarted 500 ms after | `Unit::Update` (`suspendRangedAttackTimer`) |
| 40 | Steady Shot damage | weapon roll and ammo at the weapon's own speed | normalized to 2.8 s | `Spell::EffectSchoolDMG` |
| 41 | Mortal Shots on Serpent Sting | once | twice | `Unit::SpellCriticalDamageBonus` |
| 42 | Black Arrow scaling | 0.02 RAP a tick, Hunter's Mark included | 0.023, without Hunter's Mark | `spell_bonus_data` |
| 43 | Hunter pet scaling | 45% stamina; Hunter vs. Wild's stamina share joins the owner's RAP before the 22% | 30% stamina; the share added to pet AP in full | `spell_hunter.cpp` (`spell_hun_generic_scaling`) |
| 44 | Hunter pet base stats at 80 | Strength 192, agility 158, weapon 60-100 | 331, 113, 50-78 | `pet_levelstats`, `Pet::InitStatsForLevel` |
| 45 | Cobra Reflexes | haste only: each hit's AP bonus reads the unhasted 2 s | faster, weaker hits | `Guardian::UpdateDamagePhysical`, `Unit::GetAttackTime` |
| 46 | Pet happiness | +25% on weapon damage | +25% on all damage | `Guardian::UpdateDamagePhysical` |
| 47 | Hunter pet focus | 24 every 4 s | 5 a second | `Creature::Regenerate` |
| 48 | Hunter pet magic-class abilities | +50% crits, 0.333 of pet spell damage (dots less); Poison Spit and Demoralizing Screech roll the magic table too | +100% crits, 0.049 of pet AP; those two on the melee table | `spell_bonus_data`, `Unit::SpellCriticalDamageBonus` |
| 49 | Explosive Trap damage (49065) | magic class: never misses, crits off spell crit for +50%, no ten-target cap (the trap's trigger creature casts it) | ranged hit and crit, +100%, capped | `GameObject::CastSpell`, `Spell::AddUnitTarget` |
| 50 | Slam | can only miss: the cast (47475) is `NO_ACTIVE_DEFENSE`, its damage (50783) `ALWAYS_HIT` | one yellow roll: miss, dodge, parry, crit | `spell_warr_slam::HandleDummy`, `Spell.dbc` |
| 51 | Two-Handed Weapon Specialization on seals and judgements | none: the aura is physical-only, and a holy weapon-percent spell's weapon damage skips the physical % mods | +2% a rank | `Spell::EffectWeaponDmg`, `Unit::MeleeDamageBonusDone` |
| 52 | Seed of Corruption's explosion (47834) spell power coefficient | 0.2129 (`spell_bonus_data`) | 0.286 (`Spell.dbc`'s own `bonusMultiplier`, Classic's value) | `spell_bonus_data`, confirmed against the live `.simval spelldump` capture |
| 53 | Warlock pet and infernal AP/spell power inheritance (57%/15% of the owner's spell power) | floored to a whole point | exact | `spell_warl_generic_scaling`/`spell_warl_infernal_scaling::CalculateAPAmount`/`CalculateSPAmount` |
| 54 | Stormstrike's two hits | 32175 (MH) and 32176 (OH), `ALWAYS_HIT` and `NO_ACTIVE_DEFENSE`; the cast (17364) rolls the yellow table once, then they always hit and can't be blocked | one yellow roll per hit, each of which can be blocked | `Unit.cpp:3284-3308` (`isSpellBlocked`'s exemption), `Spell.dbc` (32175/32176 attributes) |
| 55 | Searing Totem, Magma Totem, Flametongue Weapon and Fire Nova's damage | a separate, non-binary id the summon, imbue or dummy cast triggers (58702, 58735, 10444, 61654) | one spell, the summon's, imbue's or cast's own id | spelldump (`binary` per id) |
| 56 | Magma Totem's pulse (58735) and the fire elemental's Fire Nova (12470) | the totem and the elemental cast these themselves, and the 10-target AoE cap gates a player caster only, so neither is capped. Fire Nova's own pulse (61654) the player casts, aimed at the totem, so it is | all three capped past 10 targets like other player AoE | `Spell::DoAllEffectOnLaunchTarget`, `spell_sha_fire_nova::HandleDummy` |
| 57 | Lava Lash's Flametongue bonus | +25%/+35% whenever the caster carries the Flametongue Weapon dummy aura on either weapon | main hand and off hand scored separately, off hand only | `spell_sha_lava_lash::HandleDummy` |
| 58 | Shamanistic Rage's mana proc | 18 PPM (`spell_proc`) | 15 PPM | `spell_proc` (30823) |
| 59 | Spirit Wolves' and the fire elemental's crit | `Unit::GetUnitCriticalChance`'s flat 5% base (`m_baseSpellCritChance` 5% for spell crit), a scripted aura's agility scaling for the wolves on top, none for the fire elemental | Spirit Wolves 1.1515% base, the fire elemental 2.61% base plus an Intellect dependency | `Unit.cpp:418`, `3904-3960` |
| 60 | Nature's Swiftness cooldown | 2 min (`category_cooldown_ms` 120000 on 16188) | 3 min | spelldump (16188) |
| 61 | Eclipse proc condition | rolls on every landed Wrath or Starfire cast | only a critical strike rolls it | `spell_dru_eclipse::CheckProc`, `spell_druid.cpp:1486-1519` |
| 62 | Moonfire direct hit coefficient | 0.13 spell power | 0.15 | `spell_bonus_data`, spell 48463 |
| 63 | Faerie Fire (Feral) Bear Form damage | dealt under the triggered 60089, resists partially | dealt under the cast 16857, binary | `Spell::PrepareTriggersExecutedOnHit`, `Spell.cpp:8916-8921` |
| 64 | Omen of Clarity proc chance | the aura's own `spell_proc` row: 3.5 PPM off weapon speed or cast time, floored at 1.5 s | cast time / 60 × 3.5, a 0.666 instant-cast factor, and per-spell multipliers | `Aura::CalcProcChance`, `spell_proc` row for 16864 |

Not yet settled against retail, check before patching: the 200 ms other-hand push (`PlayerUpdates.cpp`), the DoT
refresh tick-timer rule, the max(cast, 1500 ms) PPM basis for spell-triggered aura procs, the rule-based binary
spell list (`SpellMgr.cpp:3405-3466`), the 5 yard missile floor, which `Spell::AddUnitTarget` calls a hack, and
percent `SPELLMOD_DAMAGE` and `SPELLMOD_DOT` mods multiplying where the sim's model added them: Seals of the Pure,
The Art of War and Glyph of Judgement give ×1.392, not ×1.35 (`Player::ApplySpellMod`, which cites Glyph of Renew).

## Performance

PAR-PERF (at `0782b5be8`) and PAR-PERF-2 (at `f126dd8ab`) profiled `BenchmarkSimulate` with pprof while other items'
test runs loaded the machine, so each fix was judged by interleaved A/B runs (base and patched test binaries
alternating; median of per-pair ratios, pairs won) and by profile shares, never against the
[throughput table](../wave-loop/wave-loop.PLAN.md#sim-throughput). Shares are of CPU samples, GC workers included.

**What the benchmarks measure.** Since PAR-PERF-2, each spec bench sims its golden suite's default player the way the
suite's FullBuffs LongSingleTarget test does, APL, talents and glyphs included: TestCombat's (`combat_expose`),
TestRetribution's, TestMM's and TestElemental's. `./sim/` is 8 players (2 Balance, 2 Shadow, 2 Arcane, Elemental,
Enhancement), not 25, each on its suite's APL, talents and glyphs, for 300 s. Each bench has two cases:
- `BenchmarkSimulate/iterations=1`: environment build, one iteration and metrics export, what an optimizer evaluation
  pays per short run.
- `BenchmarkSimulate/iterations=100`: mostly the rotation, as in a UI sim.

Before, each bench ran one iteration through `core.RaidBenchmark`, so `NewEnvironment` was 15-55% of it. Ret, Hunter and
Elemental carried no rotation (Elemental cast nothing), Rogue ran `combat_cleave_snd` as a Troll without talents or
glyphs, and the raid had neither. PAR-PERF gave the raid, Feral, Feral Tank, Enhancement, Fury and Protection Warrior
benches their suites' APLs: they crashed on their nil `Rotation` (`attack.go`'s queued-swing path, `energy.go`). Feral
also needs its `StandardTalents`: the cat rotation reads Mangle (Cat)'s cost without checking the talent.

**Parity's per-event cost is small:** `NextServerTick` (P3-3) ≤0.4%, PPM on the proc path (P3-4) ≤0.7%, `Dot.TickOnce`
including the tick's damage (P3-5) ≤1.9%. The real cost was P3-2's serverdata lookup in `RegisterSpell`: 5-7% of Ret,
Hunter and Elemental, 2% of the raid, and 20% of Rogue's allocated bytes.

**PAR-PERF's fixes**, on the old one-iteration benches. None changes a result: all 37 `.results` byte-identical, simval
508/508.

| Fix | Base share | new/base (pairs won) |
|---|---|---|
| `serverdata.find`'s comparator took the ~400-byte row by value, so every probe copied it and heap-allocated it (the pointer escapes into `key`); it now searches by index | 5-7% of Ret, Hunter, Elemental | Ret 0.822 (7/7), Hunter 0.871 (6/7), Elemental 0.920 (7/7), raid 0.946 (5/7) |
| `HasSetBonus` ranged over `Equipment` by value; `Equipment.Stats` and `Item.TotalStats` added `Stats` by value. Now by index and in place, same addition order | `HasSetBonus` 5% of Ret, 2% of Elemental; `Equipment.Stats` 2.6% of Ret | Ret 0.926 (7/7), Elemental 0.964 (7/7) |
| `runPresims` cloned the whole request even with nothing to presim | Rogue 5%, raid 2.9% | Rogue 0.984 (7/7); raid 1.002 (3/7) and Ret 0.992 (5/7), within noise |
| `sortDeps` scanned every dependency for each of 210 stat pairs; now one stable sort by pair, which keeps each pair's combining order | Elemental 5%, Ret 4.5% | raid 0.934 (5/7), Elemental 0.951 (5/7), Ret 0.985 (4/7) |
| `ActionMetrics.ToProto` allocated each target's message on its own; now one slice per action | raid 6.8%, Elemental 5.1% | raid 0.960 (6/7), Elemental 0.982 (5/7) |

All fixes together, 9 interleaved rounds under load (the other five repaired benches: 5 rounds), medians in ms per sim:

| Bench | Base | New | new/base (pairs won) |
|---|---|---|---|
| Combat Rogue | 1.113 | 1.016 | 0.915 (9/9) |
| Ret Paladin | 0.328 | 0.240 | 0.737 (9/9) |
| Hunter | 0.437 | 0.357 | 0.846 (8/9) |
| Elemental | 0.435 | 0.350 | 0.784 (9/9) |
| Raid (8 players) | 9.65 | 8.47 | 0.874 (9/9) |
| Feral, Feral Tank, Enhancement, Fury, Protection Warrior | | | 0.919, 0.841, 0.892, 0.877, 0.919 |

After them each fixed function was ≤2.2%, except `ActionMetrics.ToProto`: 5.9% of the raid, 3.8% of Elemental.

**PAR-PERF-2's fixes**, on the new benches, 7 interleaved rounds each. None moves a golden: all 37 `.results`
byte-identical, simval 508/508.

| Fix | Base share | new/base (pairs won) |
|---|---|---|
| Nibelung's 10 Val'kyr pets are built only for a player with it equipped or in the item swap, not for every Balance, Shadow, Elemental, Mage and Warlock. Each pet is a unit, and per-spell metrics and attack tables grow with the unit count | `ConstructValkyrPets` 10.8% of Elemental, 4.8% of the raid at 1 iteration; bytes per op 975 KB → 339 KB (Elemental), 15.4 MB → 4.7 MB (raid) | 1 iteration: Elemental 0.720 (7/7), raid 0.637 (7/7). 100: 0.934 (7/7), 0.917 (7/7) |
| `AddPendingAction` binary-searches its slot instead of scanning from the queue's far end | 15.3% of the raid at 100 iterations, 12.2% flat (inlined); now 7.1% | raid 0.884 (7/7) at 100 iterations, 0.932 (6/7) at 1; single-player benches 0.97-1.01, noise |
| `findServerAllowance` reads without the `RWMutex` | ≤1.2% of any bench, the lock itself too small to show | 0.998-1.007 (2-4/7) at 1 iteration, where the lookups run: noise. Its gain is contention between parallel sims, and 6 rounds of the optimizer's all-threads quick evaluation were too noisy under load to show it |

- **Val'kyrs.** Each is the last pet its owner adds, and every golden sims one player, so no other unit's index moves.
  Without Nibelung a Val'kyr is never enabled, draws no random number and deals no damage: results only lose 10 empty
  pet entries per caster. In the raid bench, where later players' pets do renumber, every player's DPS matches base to
  6 decimals over 100 non-test iterations (one shared RNG). Elemental with Nibelung equipped, swapped in or swapped
  out keeps its 10 pets and base's DPS. The goldens never summon one (`NibelungAverageCasts` 0) and can't tell whether
  they're built, so `sim/common/wotlk/nibelung_test.go` checks the count: 0 without Nibelung, 10 equipped or swapped in.
- **Pending queue order:** earliest `NextActionAt` first, then higher `Priority`, then the one added first. The slice
  holds it reversed (next action last, sentinel at 0), and the old scan took the first slot from the far end whose
  action runs before or ties the new one: on a sorted slice, the binary search's slot. It stays sorted because nothing
  writes a queued action's `NextActionAt` or `Priority`: every write comes before the action is added or in its own
  `OnAction` after the pop, and `SetGCDTimer` and `newHardcastAction` reuse an action only once consumed. An assertion
  build (binary against linear slot, plus sortedness, on every insert) passed all 37 suites, simval and every bench.
- **Allowances** all register from package `init` functions (10 files; none from tests, tools or cmd), and init
  finishes before any test or `main` runs, so `serverAllowances` is a plain map read without a lock.

All three together, medians in ms per op under load; the orchestrator takes the idle-machine row:

| Bench | iterations=1: base → new | new/base (won) | iterations=100: base → new | new/base (won) |
|---|---|---|---|---|
| Combat Rogue | 1.486 → 1.485 | 1.000 (4/7) | 129.8 → 129.3 | 0.993 (5/7) |
| Ret Paladin | 0.497 → 0.498 | 1.000 (4/7) | 34.25 → 32.88 | 0.982 (6/7) |
| Hunter | 0.584 → 0.587 | 0.997 (5/7) | 40.76 → 40.53 | 0.993 (5/7) |
| Elemental | 0.469 → 0.329 | 0.696 (7/7) | 15.82 → 14.67 | 0.918 (7/7) |
| Raid (8 players) | 7.785 → 4.620 | 0.609 (7/7) | 367.8 → 309.8 | 0.843 (7/7) |

An iteration now costs ((100-case − 1-case) / 99) 1.29 ms on Rogue, 0.33 Ret, 0.40 Hunter, 0.15 Elemental and 3.08 the
raid, so setup and export are 13% of Rogue's 1-iteration case, about a third of Ret's, Hunter's and the raid's, and 56%
of Elemental's. `NewEnvironment` there: Rogue 13%, Hunter 19%, raid 19%, Ret 25%, Elemental 37%.

**Left for other items** (shares after PAR-PERF-2):
- APL evaluation is upstream's interpreter, not parity: `getNextAction` is 71% of Rogue at 100 iterations
  (`APLValueCompare` and `APLValueAnd` 35% each), 24% of the raid.
- `AddPendingAction` is still 4.5-6.4% of the raid, Ret and Hunter at 100 iterations: each probe dereferences a
  scattered `*PendingAction`, and cancelled actions stay queued until popped.
- `ActionMetrics.ToProto` and `UnitMetrics.ToProto`, 3-5% of the raid and Elemental at 1 iteration: one message per
  unit per action.
- On the old benches, PAR-P7-0c's other files held little flat time: `cast.go` 2.9% of Rogue and 3.8% of the raid,
  nearly all `ApplyCostModifiers` under the APL's cost checks; `attack.go`, `unit.go` and `buffs.go` ≤1.3% each. The
  swing loop's 15-18% cumulative on Rogue, Ret and Hunter is mostly the white hit's damage and procs, plus the APL pass
  after each swing (6.5% of Rogue).
- `NewPet` (`pet.go`) returns the Character-sized `Pet` by value, so each pet is built and copied whole: ≤1.3% now that
  only real pets are built (4% of Elemental with the Val'kyrs).
- **Fixed (PAR-P7-0d):** Feral, Feral Tank, Enhancement, Fury and Protection Warrior's benches had their suites' APLs
  from PAR-PERF but were still one `core.RaidBenchmark` case each, sharing `SimOptions` with the package-level
  `AverageDefaultSimTestOptions`; `RaidBenchmark` sets `Iterations` on whatever it's handed, so this wrote 1 into that
  shared 2000 permanently. They're now on `RaidBenchmarkIterations`, the same `iterations=1`/`iterations=100` pair as
  the rest, which builds its own `SimOptions` per case instead of touching the caller's. The five duplicate
  `benchmarkIterations` copies (hunter, retribution paladin, raid, rogue, elemental) now call it too.
