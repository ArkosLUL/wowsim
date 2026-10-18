# Server parity

Where the server differs from the Classic behaviour upstream modelled.
- Why the sim follows the server: [ADRs](../adr/).
- How each module is handled: [azerothcore-server.md](azerothcore-server.md#modules-that-affect-the-sim).
- How a hit's or heal's amount is worked out: [damage and healing](damage-and-healing.md).
- Class spells, talents and procs: [class mechanics](class-mechanics.md) and
  [caster mechanics](caster-mechanics.md). Threat: [threat](threat.md). Pets and guardians: [pets](pets.md).
  Items, gems, enchants and item swaps: [items](items.md).
- Full evidence and phase status: `docs/azerothcore-parity/azerothcore-parity.{PLAN,INVESTIGATION}.md`. The
  INVESTIGATION also lists every retail deviation and every mod-spell-tweaks change.

## Combat tables

Verified on the server by simval.

| Mechanic | Classic sim | Server |
|---|---|---|
| Melee crit suppression vs +3 boss | 4.8% | 0.6% |
| Spell crit suppression | 2.1% | none |
| Glancing | 24%, ×0.75 | 25%, ×0.70 |
| Who glances | players and every pet | players and `SUMMON_PET` pets (hunter, warlock, Master of Ghouls ghoul), never guardians |
| Boss dodge / parry | 6.5% / 14% | 6.45% / 14% |
| Level-83 non-boss dodge = parry | – | 5.6% |
| White dodge/parry vs expertise | linear | cliffs at 23.4 / 53.6 |
| White table order | glance before block | block before glance |
| Yellow block | in the table | separate roll, 4.4%, front only, can crit |
| Average magic resist | 6% | 3.6145% (K = 400, +15 level resist) |
| Multi-school resist | 0 | lowest of its schools; holy counts 0 |
| Spell miss vs +3 | 17% | 16.99% ((threshold − 1) / 10000) |
| Creature vs player | – | miss 469 bp, crit 560 bp, no crushing |
| ArP cap vs level 83 | 15232.5 | 16635 (uses the victim's level) |
| Boss block value | 76 | 41 (level / 2 + STR / 20) |

Other differences:
- Holy gets the level-based resist vs creatures. The Classic sim read Strength as its resistance stat.
- The pets' flat +1.8% crit (hunter pet, fire elemental, spirit wolves) is Classic-only.
- The partial block is rolled in `CalculateSpellDamageTaken`: never on a hit check without damage (Sunder
  Armor) or for an ALWAYS_HIT or NO_ACTIVE_DEFENSE spell (`Unit::isSpellBlocked`), and against a player only
  with a shield, at their sheet block chance.
- `SPELL_ATTR3_COMPLETELY_BLOCKED` without direct damage blocks inside the yellow table, as
  `SPELL_MISS_BLOCK`, which stops the spell.
- Chill of the Throne (aura 49) is -20% dodge on the player, not the boss.

## Base stats and ratings

Generated from the server ([gen_basestats](../../tools/acore/gen_basestats/README.md)) and verified per
class by simval.

| Mechanic | Classic sim | Server |
|---|---|---|
| Shaman / Warlock base HP | 6960 / 7164 | 6939 / 7136 |
| DK base mana | 1000 | 0 |
| ArP rating per 1% | 13.99 | 15.3953 / 1.1 = 13.9957, every class |
| Avoidance diminishing returns | druid and non-druid constants | per-class k and dodge, parry and miss caps |
| Dodge from base agility | diminishes | doesn't |
| Defense rating | continuous | truncated to whole skill points first |

- Percent-ArP auras (Battle Stance, Mace Specialization) add to the rating's percentage, under one cap.
  Blood Gorged's class mask limits it to white swings and Plague, Blood, Heart, Death and Rune Strike.
- The server truncates primary stats to integers and the sim doesn't, so anything derived can be off by
  one point's worth.
- Defense skill bonuses (`PseudoStats.BonusDefenseSkill`: Rune of the Stoneskin Gargoyle) add after the
  rating's truncation, so dodge, parry and miss take them undiminished; `Unit.DefenseSkill()` is the total,
  for the attack table, crit taken, block and every talent reading defense. The sheet reads the bonus from
  the sim (`PseudoStatBonusDefenseSkill`), never off the gear.

## Other measured server behaviour

- **Talent values** are the spelldump's, not the DBC files': mod-spell-tweaks' `spell_dbc` rows change some
  while the live DBC files stay stock. Rage of Rivendare is 2 expertise a point, Virulence and Nerves of Cold
  Steel 2% hit a point.
- **mod-spell-tweaks' switches** are `SpellTweaksSettings` (`proto/common.proto`, a comment a field), read as
  `Character.Server().SpellTweaks` with `SpellTweaks.Enable` folded in (`newSpellTweaks`). `Enable` off drops
  the exotic pet bonus, every switch but the two armor pen ones (core reads those itself) and the tweaks
  without a switch: the seal DoT from Crusader Strike and Divine Storm, Explosive Trap laid from range. A new
  switch also goes in `tools/acore/gen_server_defaults/settings.go` and `server_settings_picker.ts`;
  `TestSpellTweaksEnableGatesScriptedTweaks` fails for one `newSpellTweaks` doesn't read.
- **Mechanic immunity:** a target immune to a spell's spell-level mechanic takes none of it
  (`Unit::IsImmunedToSpell`), and a snare or root effect never goes on (`Unit.ImmuneToMechanic`). The mask is
  `proto.Target.mechanic_immune_mask`. Unset, a world boss gets `core.RaidBossMechanicImmunities`
  (`creature_immunities` -361, the boss dummy's and the Ulduar, ToC and ICC bosses': root, stun, snare and
  disarm among others, not attack slows or bleeds) and any other target none. The Naxxramas presets carry
  -286, the same without interrupts. The pet specials Pin, Venom Web Spray, Ravage, Tendon Rip and Snatch cast
  and miss on an immune target.
- **Hit rules by damage class:** a spell with no damage class always lands and never crits
  (`WorldObject::SpellHitResult`, `Unit::SpellDoneCritChance`: Saronite Bomb), and a spell never misses its
  own caster (the sapper). The sim applies both only to the explosives (`sim/core/consumes.go`). A
  `SPELL_ATTR2_CANT_CRIT` spell never crits either (`Unit::SpellCriticalHitChance`): the spelldump's
  `critCapable` is false for it, and simval's crit check reads that (`tools/simval/critcap.go`).
- **Binary spells** follow the spelldump. Mind Flay isn't binary; Steady Shot and Expose Armor are.
- **Weapon requirements:** a spell with `SPELL_ATTR3_REQUIRES_MAIN_HAND_WEAPON` or `_OFF_HAND_WEAPON` and
  an item subclass mask fails without that weapon, triggered casts too (`Spell::CheckItems`): Mutilate
  needs two daggers, and rogue off-hand strikes an off-hand weapon. So does one whose `EquippedItemClass` the
  player doesn't wear: a hunter's shots, stings and Volley need a bow, gun or crossbow, Raptor Strike a melee
  weapon, which the sim reads as a main hand (`sim/hunter/weapon_requirements.go`).
- **Spell costs** (`sim/core/cost.go`; `SpellInfo::CalcPowerCost`, `Player::ApplySpellMod`): truncated base
  plus the first school's flat aura (Spark of Hope), then (that + Σ `SPELLMOD_COST` flats) × (1 + Σ percents),
  0 once the flats reach it, truncated, then × (1 + the first school's cost-multiplier aura, e.g. Power Infusion,
  Precision, Beast Within), truncated, at least 0. Percents add, never multiply; rage and runic power are in
  tenths. A rune count takes the spell mods only (`runeCountFor`), and a rune spell's runic power gain needs no
  cost left to pay (`Spell::TakeRunePower`). `DefaultCast.Cost` is the
  base; `StaticCost()` has the never-changing talents (options' `Multiplier`, `FlatMod`) and is what rotations
  read. Dynamic mods: `Spell.CostMultiplier`/`CostFlat`, `PseudoStats.CostMultiplier`/`CostFlat`,
  `Unit.AddSchoolCostMultiplier`/`AddSchoolCostFlat`. `ApplyCostModifiers` caches per spell, comparing every
  input (`costCache`): a new input joins that key.
- **Stat sheet:** armor on jewelry, weapons and `ArmorDamageModifier` is bonus armor for every class
  (`Player::_ApplyItemBonuses`); block value is half a point a strength for every player. Moonkin Form ×4.7
  item armor (24905) and the weapon's feral AP.
- **Runes** (`sim/core/runic_power.go`; `Player::GetRuneBaseCooldown`, `Spell::TakePower`): grace cuts the
  overshoot less 250 ms, at most 2250; death runes go first to a base type owed, then blood, unholy, frost. A
  refundable spell missing its main target gives no runic power, puts its runes on a 1.5 s cooldown with death
  runes kept, and takes a runic power cost's [miss cost](class-mechanics.md#miss-cost).
- **Exclusive auras** of equal amount in a single-aura category (`keepsEqualAura`, `sim/core/exclusive_effect.go`;
  `Unit::IsHighestExclusiveAuraEffect`): of two spells' the longer full duration stays, a permanent one longest;
  the new one wins a tie, and always against its own spell. Time left doesn't count. The server reads the new
  spell's DBC duration, the sim its configured one, so a glyph or combo points changing it can differ.
- **Power Infusion and Arcane Power** share group 1123 with no common aura type: the one cast last removes the
  other.
- **Armor debuffs:** Sunder ×5 (debuff 58567, not ability 47467) and Expose Armor don't stack. Faerie
  Fire multiplies. Curse of Weakness' armor holds only while its AP is the AP debuff that counts (group 1062,
  highest per aura).
- **Hit debuffs** on a creature add up (`HandleModHitChance`): Insect Swarm and Scorpid Sting stack.
- **Replenishment** (`sim/core/replenishment.go`): 15 ticks a second apart of int32(max mana × 0.002f), read as
  the aura goes on or is refreshed (1 if it went on at no mana). A refresh restarts the timer and drops a tick
  due on that update, unless the cast comes out of an update that runs after the unit's: a creature's or
  pet's (Hunting Party off Explosive Shot's tick), or the unit's own swing.
  - A cast (`ProcReplenishment`) lands on every raid unit that's out and on mana, pets and guardians with a
    pet bar included; over ten, on the ten lowest on mana for their pool (`PowerPctOrderPred`: float32,
    stable), the rest keeping their aura. A Spirit Wolf's pool of 0 sorts first (`Pet.EmptyManaPool`); a pool
    that stays full sorts last, at 100% (`Pet.FullManaPool`: treants, Shadowfiend, Infernal). Either takes the
    aura and nothing from it.
  - The raid's buff goes only on a unit with mana and ticks every second, reading the max each time. A pet
    that isn't out queues no tick; its timer keeps its place.
  - The potion, rune and gem heuristics add `Unit.ReplenishmentManaPerSecond`:
    `ManaRegenPerSecondWhileCasting` doesn't hold it.
- **A cast that fails** its GCD, hardcast or shared-cooldown check starts no cooldown.
- **Timing and procs** (the rest is in [spell data and timing](spell-data-and-timing.md)):
  - The rage hit factor is truncated to `uint32`.
  - Spell-proc PPM uses max(cast time, 1500 ms).
  - `REDUCE_PROC_60` applies.
  - A DoT refresh resets the tick timer only when StackAmount < 2.
  - Any dot or hot tick procs a `DONE_PERIODIC` aura, whatever its mask (`ProcTrigger.AnyPeriodic`). A tick the
    server deals as a triggered cast (auras 23, 226, 227) procs only an aura with
    `SPELL_ATTR3_CAN_PROC_FROM_PROCS` (`ProcTrigger.CanProcFromProcs`: Greatness, Dying Curse), as a hit of its
    damage class.
  - **Landed casts:** `Spell::DoAllEffectOnTarget` raises a hit-phase proc for every target a cast lands on,
    whatever it did there. A proc that reads that registers `Unit.RegisterOnCastLanded`
    (`sim/core/cast_landed.go`): raised once a spell, target and instant by every landed direct hit or heal and
    every `ticksAsCasts` tick, never a plain tick or a `SpellFlagNoOnDamageDealt` spell. A cast that lands with
    nothing to deal (a hot, shield or aura put on: Renew, Power Word: Shield, Earth Shield, Ignite) calls
    `Spell.LandOn` itself, on each unit the server lands it on: the caster for a self buff (Arcane Torrent,
    Dancing Rune Weapon), the pet, or every raid unit (Horn of Winter). A spell the server casts from another
    unit (the totems' attacks, Healing Stream's heal) is that unit's landing, not the caster's
    (`Spell.CastFromAnotherUnit`).
  - Periodic ticks crit only with an aura 286 covering the spell, which can be a passive a skill teaches
    (SkillLineAbility AcquireMethod 2: Flame Shock's 75461, Immolate's 75445). Every sim dot declares it
    as `TicksCanCrit` (`sim/dotconfig_declared_test.go`).
- **Dungeon scale** (`NewEncounter`): health and armor become `round(float32(value) × multiplier)`, half
  away from zero. Damage multiplies the target's damage dealt, so swings, spells and DoTs all get it,
  except spells with `SpellFlagIgnoreAttackerModifiers`. Targets with `world_boss` (else level ≥ 83) take the boss set. The placeholder target of an
  encounter without targets is never scaled.
