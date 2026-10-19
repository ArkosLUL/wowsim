# Damage and healing amounts

How the server works out the amount of a hit or a heal, where the Classic sim did it differently: percents,
rounding, caps and crit damage. Everything else the server does differently is in
[server parity](server-parity.md); declaring a spell's base values and coefficients is in
[spell data and timing](spell-data-and-timing.md#server-data).

## Damage

- **Damage mods:** percent `SPELLMOD_DAMAGE` and `SPELLMOD_DOT` mods from talents, glyphs, set pieces and
  procs multiply (`Player::ApplySpellMod`; other ops' percents add). Class spells stack them through
  `spellModDamage`. A holy spell dealing weapon damage (seal procs, judgements)
  skips physical percent mods such as Two-Handed Weapon Specialization: `Spell::EffectWeaponDmg` and
  `Unit::MeleeDamageBonusDone` match auras by school.
- **Damage rounding** (`doneDamage`, `ApplyPostOutcomeDamageModifiers`): every step is whole.
  `SpellDamageBonusDone` and `MeleeDamageBonusDone` (a dot's amount at application) and the `…Taken` steps
  truncate, `CalcArmorReducedDamage` rounds up, a partial resist takes `damage × i / 10` in integers; crit,
  glancing and absorbs stay whole. Each SP or AP bonus is its own `int32` off two float32s
  (`core.PowerBonus(coefficient, power)`; `Spell.SpellPower()` is whole), a base roll a whole step
  (`SpellEffect.Roll`, or `Simulation.RollWhole` inline), a stacked dot one stack truncated, then × stacks
  (`Dot.SnapshotStacks`); a magic crit comes before the resist.
  - A weapon strike or white swing rolls whole between the truncated ends, with the AP at speed, the flat
    bonuses and the hand's TOTAL_PCT inside (`Spell.StrikeDamage` in `sim/core/weapon_strike.go`;
    `Unit::CalculateDamage`), so `doneDamage` leaves that percent out. `StrikeFixed` scales a fixed bonus,
    `core.ApplyPct` is each percent step (`Spell::EffectWeaponDmg`). Off the physical school there's no
    TOTAL_PCT, the off hand's half included.
  - A hit whose multipliers come after the roll takes `Weapon.WholeDamage` (Sweeping Strikes' copy); a
    script adding its own weapon roll, `Weapon.WholeBaseDamage` (Steady Shot's `irand(int32(min),
    int32(max))`). Manifest Anger is `ApplyPct` 50% of a whole strike.
  - A script's AP share keeps the server's float32 order: Eviscerate `int32(ap × combo × 0.07f)`, Envenom
    `int32(ap × 0.09f × combo)`.
- **Crit damage:** spells and specials add up the aura 163 bonuses (meta gem, Predatory Instincts, Prey on the
  Weak, Poleaxe) before `SPELLMOD_CRIT_DAMAGE_BONUS` scales the part over 1; white swings multiply them
  (`Character.WhiteCritMultiplier`). Magic-class spells crit from 1.5, the DK's Icy Touch, Death Coil, Blood Boil,
  Howling Blast and Death and Decay too.
- **AoE cap:** past 10 units the server cuts each hit of a player's area effect to `damage × 10 / units` in
  whole numbers, after the done and taken percents and before armor and crit
  (`Spell::DoAllEffectOnLaunchTarget`). An area spell sets `Spell.AoECapUnits` at its cast to every unit the
  effect hits: the caster of Thunderstorm and the Thermal Sapper, not the target a script drops (Mind Sear
  tick, Starfall splash). A tick the server casts as a spell of its own is cut too (Volley). Guardians' casts
  and Explosive Decoy's blast are uncapped. `TestSimAOESpells` holds the list
  ([server validation](server-validation.md#offline-cross-checks)).
- **Bleed bonus** (Mangle, Trauma, Stampede; `Unit::SpellDamageBonusTaken`) goes by the spell's mechanic
  (`serverdata.FlagBleed`, set by `gen_serverdata`), hit and ticks alike, and to Shred and Maul on a weapon
  hit, at each victim's own percent: Savage Rend's and the pet Rake's first hit take it, Pin's ticks don't.
- **Creature-type attack power** (Elixir of Demonslaying, Scourgebane) and, on a ranged hit, Hunter's Mark reach
  only weapon damage: a white swing or a spell with a weapon effect adds `int32(sum / 14 × speed)` after its
  weapon percent (`Spell.weaponAttackPowerBonus`).
  - A physical hit's bonus skips the hand's TOTAL_PCT (`weaponBonusMultiplier`): `DamageDealtMultiplier`, the
    physical school's, `Spell.TotalPctMultiplier` (Ranged Weapon Specialization on ranged attacks; the off
    hand's aura 122, Dual Wield Specialization and Nerves of Cold Steel).
  - DONE_VERSUS (aura 168: Troll Beast Slaying, Crusade's second percent, the undead sets' 57943 and 29068) sits
    per target in the attack tables and scales it. So do the DK's DoneTotalMod talents on its strikes (Rage of
    Rivendare, Tundra Stalker; Glacier Rot and Merciless Combat on Frost Strike and Obliterate).
  - The rune weapon's hits add the DK's bonus, halved, with the DK's per-target multipliers.
- **Creature melee** is (weapon damage + AP/14 × variance) × `creature_template.DamageModifier` × attack
  time in seconds (`Creature::CalculateMinMaxDamage`). A target's `MinBaseDamage` scales its AP part too, so
  177.074 × DamageModifier × attack time gives a level 83, class 1 creature (`encounters.GenericBossTarget`).

## Healing

- **Whole heals:** a heal is whole after the caster's multiplier, the target's and the crit, one dealt outside
  a spell through `GainHealing`.
- A heal's target search skips a unit at 0 health (`SpellInfo::CheckTarget`): Chain Heal's jumps
  ([caster mechanics](caster-mechanics.md#shaman)), Althor's Abacus, Prayer of Healing's party, Healing Stream
  Totem's tick (58761) and the picks below. A unit without a health bar counts as alive and full, and so does
  health above max, which the sim doesn't cap when max health drops, where the server does. Which pets each
  search takes: [pets](pets.md#buffs-and-casts).
  - An APL's helpful cast, channel, multidot or multishield pick skips a dead unit with a health bar
    (`Spell::CheckCast`; `helpfulAtTheDead` in `sim/core/apl_actions_casting.go`), never the caster: the sim
    plays a unit on at 0 health.
  - Ancestral Awakening (52759) heals the living raid unit lowest in health percent
    (`spell_sha_ancestral_awakening_proc`, `HealthPctOrderPred`; `procAncestralAwakening`), a tie to the first
    in raid order.
  - Prayer of Mending's jump (`prayerOfMendingJump`; `Acore::MostHPMissingGroupInRange`) takes the unit missing
    the most whole health, not the holder, a tie to the first in raid order.
  - Circle of Healing, past its 5 or 6 targets, keeps the lowest health percents (`circleOfHealingTargets`;
    `HealthPctOrderPred`), a stable sort, so a tie keeps raid order.
- **Healing done percents** (aura 136: Focused Power, Blessed Resilience, Purification, Avenging Wrath,
  Divinity; `Unit.AddHealingDonePct`) reach a hot at each tick, after its whole amount:
  `int32(float32(amount) × the caster's float32 AddPct chain at that tick)` (`sim/core/healing_done.go`;
  `HandlePeriodicHealAurasTick`). Every unit-aura hot takes them, an item's too, so it snapshots its amount
  without them (`Dot.SnapshotHotMultiplier`); Ghoul Frenzy (`OBS_MOD_HEALTH`) doesn't.
  - A class's direct heal carries its talents in its own multiplier, so Avenging Wrath's reaches hots only. A
    direct heal with no damage class (an item's) takes none (`SpellHealingBonusDone` returns early).
  - Earth Shield's charge is whole at each step (`spell_sha_earth_shield`; `sim/shaman/heals.go`): 49284's
    healing bonus with Purification in float32, truncated, then the glyph's 20% of it, then Improved Shields'
    percent of what the bonuses added over the base, each truncated. 379 ignores caster modifiers but crits off
    the shaman, its original caster; Tidal Mastery's crit reaches it only at ranks 4 and 5, whose
    class mask lists it.
  - The priest T10 2-piece's 70772 ticks `int32(Flash Heal × 33%) / 3` three times
    (`spell_pri_t10_heal_2p_bonus`; `blessedHealingTick`), then takes only aura 136 and the target's healing
    taken.
- **Healing taken** percents (aura 118) don't stack: only the highest bonus and the lowest penalty count,
  multiplied (`Unit.AddHealingTakenPct`), so Divinity's 1% a point adds nothing under three points of Improved
  Devotion Aura (6%). Strength of Wrynn is healing done, which multiplies; a heal with no damage class, or one
  that ignores caster modifiers (`SpellFlagIgnoreAttackerModifiers`: `Unit::SpellPctHealingModsDone` returns 1),
  takes none of that, only `Unit.HealingTakenPctMultiplier`.
- **Flat healing taken** (`BonusHealingTaken`: Talisman of Troll Divinity) is a term of its own:
  `int32(taken × coefficient)` for each target and tick, added after the caster's percent and ahead of the
  target's (`Unit::SpellHealingBonusTaken`; `flatHealingTaken`). `Spell.HealingPower` is the caster's alone.
  - The coefficient (`healTakenCoefficient`) is the declared one where the sim's spell is the server's heal, with
    the `SPELLMOD_BONUS_MULTIPLIER` mods of the target's spell-mod owner, not the caster's (`Unit.spellModOwner`:
    a pet's or guardian's owner): the caster's modded one when that's the caster, else the owner's own copy of
    the spell, else the declaration without mods.
  - A sim spell that isn't the server's heal sets `SpellConfig.HealingTakenCoefficient` to the server heal's
    (Prayer of Mending heals as 33110; Penance; Earthliving's hot 52000) or, where that has no
    `spell_bonus_data` row, to `core.DefaultHealingTakenCoefficient` (Empowered Renew's 63544, Ancestral
    Awakening's 52752, Earth Shield's 379, `itemHealSpell`'s item heals).
  - On a guardian, an instant heal's default is 1 × 1.88, not 1.5 / 3.5 × 1.88: `Unit::GetCastingTimeForBonus`
    gives a creature that isn't `IsPet()` 3500 ms (`Unit.isGuardian`; the summon flags:
    [pets](pets.md#buffs-and-casts)).
  - An absorb takes none, nor does a heal with neither.
- A heal whose server spell has `SPELL_ATTR6_IGNORE_HEALTH_MODIFIERS` or `SPELL_ATTR0_CU_NO_POSITIVE_TAKEN_BONUS`
  loses the flat term and the taken percent when that percent is over 1 (`noPositiveHealingTaken`). Among the
  sim's heals only Ancestral Awakening (52752) has one; Earth Shield's 49284 does, but heals as 379.
- **Heals off another heal:** Empowered Renew's 63544 is 5% a rank of every Renew tick as the target takes it
  (`Dot.TickHealingTaken`: flat term and percents), then takes the healing done talents and its own taken terms
  again (`sim/priest/renew.go`). Glyph of Prayer of Healing's 56161 ticks `int32(heal × 20%) / 2` off the
  Prayer of Healing heal on that target, crit and healing taken in, none off a heal of 0; its class mask is 0,
  so each tick takes only aura 136 and the target's taken terms again (`sim/priest/prayer_of_healing.go`).
  Ancestral Awakening is `int32(crit × 10% a rank)` and ignores caster modifiers, Purification included
  (`sim/shaman/heals.go`).
- **Mark of Blood**'s 61607 heals the unit the marked target hits for 4% of that unit's max health, Vampiric
  Blood's included, through its healing taken percents only, never a dead unit (`core.MarkOfBloodAura`). Its
  charges: [class mechanics](class-mechanics.md#death-knight).
