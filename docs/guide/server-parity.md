# Server parity

Where the server differs from the Classic behaviour upstream modelled.
- Why the sim follows the server: [ADRs](../adr/).
- How each module is handled: [azerothcore-server.md](azerothcore-server.md#modules-that-affect-the-sim).
- Class spells, talents and procs: [class mechanics](class-mechanics.md). Threat: [threat](threat.md).
  Pets and guardians: [pets](pets.md).
- Full evidence and phase status:
  - `docs/azerothcore-parity/azerothcore-parity.{PLAN,INVESTIGATION}.md`.
    The INVESTIGATION also lists every retail deviation and every mod-spell-tweaks change.
  - `docs/azerothcore-item-diff/`.

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
  own caster (the sapper). The sim applies both only to the explosives (`sim/core/consumes.go`).
- **Binary spells** follow the spelldump. Mind Flay isn't binary; Steady Shot and Expose Armor are.
- **Weapon requirements:** a spell with `SPELL_ATTR3_REQUIRES_MAIN_HAND_WEAPON` or `_OFF_HAND_WEAPON` and
  an item subclass mask fails without that weapon, triggered casts too (`Spell::CheckItems`): Mutilate
  needs two daggers, and rogue off-hand strikes an off-hand weapon.
- **Damage mods:** percent `SPELLMOD_DAMAGE` and `SPELLMOD_DOT` mods from talents, glyphs, set pieces and
  procs multiply (`Player::ApplySpellMod`; other ops' percents add). Class spells stack them through
  `spellModDamage`. A holy spell dealing weapon damage (seal procs, judgements)
  skips physical percent mods such as Two-Handed Weapon Specialization: `Spell::EffectWeaponDmg` and
  `Unit::MeleeDamageBonusDone` match auras by school.
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
  runes kept, and a runic power cost takes irand(0, cost / 4).
- **AoE cap** counts every unit the effect hits (`AOECapMultiplierFor`): the caster of Thunderstorm and the
  Thermal Sapper, not the target a script drops (Mind Sear tick, Starfall splash). Guardians' casts and
  Explosive Decoy's blast are uncapped. Past 10 units the server cuts each hit to `damage × 10 / units` in
  whole numbers, after the done and taken percents and before armor (`Spell::DoAllEffectOnLaunchTarget`):
  `Spell.AoECapUnits`, set by Swipe and Fan of Knives. The other callers still scale the base as a float.
- **Power Infusion and Arcane Power** share group 1123 with no common aura type: the one cast last removes the
  other.
- **Armor debuffs:** Sunder ×5 (debuff 58567, not ability 47467) and Expose Armor don't stack. Faerie
  Fire multiplies. Curse of Weakness' armor holds only while its AP is the AP debuff that counts (group 1062,
  highest per aura).
- **Hit debuffs** on a creature add up (`HandleModHitChance`): Insect Swarm and Scorpid Sting stack.
- **Crit damage:** spells and specials add up the aura 163 bonuses (meta gem, Predatory Instincts, Prey on the
  Weak, Poleaxe) before `SPELLMOD_CRIT_DAMAGE_BONUS` scales the part over 1; white swings multiply them
  (`Character.WhiteCritMultiplier`). Magic-class spells crit from 1.5, the DK's Icy Touch, Death Coil, Blood Boil,
  Howling Blast and Death and Decay too.
- **Damage rounding** (`doneDamage`, `ApplyPostOutcomeDamageModifiers`): every step is whole.
  `SpellDamageBonusDone` and `MeleeDamageBonusDone` (a dot's amount at application) and the `…Taken` steps
  truncate, `CalcArmorReducedDamage` rounds up, a partial resist takes `damage × i / 10` in integers; crit,
  glancing and absorbs stay whole. Each SP or AP bonus is its own `int32` off two float32s
  (`core.PowerBonus(coefficient, power)`; `Spell.SpellPower()` is whole), a base roll a whole step
  (`SpellEffect.Roll`, or `Simulation.RollWhole` inline), a stacked dot one stack truncated, then × stacks
  (`Dot.SnapshotStacks`); a magic crit comes before the resist; heals are whole after the caster's multiplier,
  the target's and the crit, one dealt outside a spell through `GainHealing`.
  - A weapon strike or white swing rolls whole between the truncated ends, with the AP at speed, the flat
    bonuses and the hand's TOTAL_PCT inside (`Spell.StrikeDamage` in `sim/core/weapon_strike.go`;
    `Unit::CalculateDamage`), so `doneDamage` leaves that percent out. `StrikeFixed` scales a fixed bonus,
    `core.ApplyPct` is each percent step (`Spell::EffectWeaponDmg`). Off the physical school there's no
    TOTAL_PCT, the off hand's half included.
  - A hit whose multipliers come after the roll takes `Weapon.WholeDamage` (the rune weapon's strikes and
    swing); a script adding its own weapon roll, `Weapon.WholeBaseDamage` (Steady Shot's `irand(int32(min),
    int32(max))`). Manifest Anger is `ApplyPct` 50% of a whole strike.
  - A script's AP share keeps the server's float32 order: Eviscerate `int32(ap × combo × 0.07f)`, Envenom
    `int32(ap × 0.09f × combo)`.
- **Bleed bonus** (Mangle, Trauma, Stampede; `Unit::SpellDamageBonusTaken`) goes by the spell's mechanic
  (`serverdata.FlagBleed`, set by `gen_serverdata`), hit and ticks alike, and to Shred and Maul on a weapon
  hit, at each victim's own percent: Savage Rend's and the pet Rake's first hit take it, Pin's ticks don't.
- **Healing taken** percents (aura 118) don't stack: only the highest bonus and the lowest penalty count,
  multiplied (`Unit.AddHealingTakenPct`). Strength of Wrynn is healing done, which multiplies; a heal with no
  damage class takes none of that, only `Unit.HealingTakenPctMultiplier`. Not modelled: flat healing taken
  (`BonusHealingTaken`: Talisman of Troll Divinity) is its own whole step after the caster's multiplier; the
  sim adds it to spell power ahead of the coefficient (`Spell.HealingPower`).
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
- **Meta gems** work only with their `SpellItemEnchantmentCondition` met (`sim/core/meta_gems.go`); an inactive
  one gives nothing. An item swap that meets or breaks the condition moves the whole enchant
  (`Player::CorrectMetaGemEnchants`; `metaGemSwap`): the stats, the effect (its `OnMetaGemSwap` toggles, for a
  gem registered with `NewMetaGemEffect`) and every crit multiplier, heals' or damage's by the gem. Class code
  that touches a crit multiplier mid-fight has to follow the swap: a factor held on it goes through
  `Character.ScaleCrit` (Combustion, Hodir's Storm Power), a value worked out ahead of time through
  `MetaCritFollow` and `MetaCritBuilt` (Prey on the Weak, Predatory Instincts).
- **Timing and procs** (the rest is in [spell data and timing](spell-data-and-timing.md)):
  - The rage hit factor is truncated to `uint32`.
  - Spell-proc PPM uses max(cast time, 1500 ms).
  - `REDUCE_PROC_60` applies.
  - A DoT refresh resets the tick timer only when StackAmount < 2.
  - Any dot or hot tick procs a `DONE_PERIODIC` aura, whatever its mask (`ProcTrigger.AnyPeriodic`). A tick the
    server deals as a triggered cast (auras 23, 226, 227) procs only an aura with
    `SPELL_ATTR3_CAN_PROC_FROM_PROCS` (`ProcTrigger.CanProcFromProcs`: Greatness, Dying Curse), as a hit of its
    damage class.
  - Periodic ticks crit only with an aura 286 covering the spell, which can be a passive a skill teaches
    (SkillLineAbility AcquireMethod 2: Flame Shock's 75461, Immolate's 75445). Every sim dot declares it
    as `TicksCanCrit` (`sim/dotconfig_declared_test.go`).
  - A munched periodic (Deep Wounds, Unholy Blight, Piercing Shots, Righteous Vengeance, Ignite, Languish) applies
    on the next 400 ms step of the caster's event clock, up to 400 ms late (`CalculateQueueTime`,
    `MunchingBlizzlike.Enabled`), ahead of a swing or tick due on the same server tick. The sim does this with
    `core.DelayedPeriodicApplier`, a phase per caster per iteration. A player's instant cast draws the step off a
    clock one world tick stale (the session pass runs before `Player::Update`), so it can land that update.
- **Enchant PPMs:** Mongoose 1, Icebreaker 3, Deathfrost 3. An enchant procs only from the weapon that
  hit.
- **Dungeon scale** (`NewEncounter`): health and armor become `round(float32(value) × multiplier)`, half
  away from zero. Damage multiplies the target's damage dealt, so swings, spells and DoTs all get it,
  except spells with `SpellFlagIgnoreAttackerModifiers`. Targets with `world_boss` (else level ≥ 83) take the boss set. The placeholder target of an
  encounter without targets is never scaled.
- **Creature melee** is (weapon damage + AP/14 × variance) × `creature_template.DamageModifier` × attack
  time in seconds (`Creature::CalculateMinMaxDamage`). A target's `MinBaseDamage` scales its AP part too, so
  177.074 × DamageModifier × attack time gives a level 83, class 1 creature (`encounters.GenericBossTarget`).

## Items

`db.json` takes item stats, ilvls, sockets and set names from the server (`make items`,
[ADR 0002](../adr/0002-item-data-from-live-db.md)). acdiff ([README](../../tools/database/acdiff/README.md))
checks it and the Go effects.

- **Kept from Wowhead:** items nothing on the server awards (the only rows left in `items_diff.csv`),
  heirlooms and random-enchant items.
- **Hardcoded Go effects:**
  - Shared ones (`sim/common`) match the server. Thunderfury's and Rod of the Sun King's PPMs are
    literals: `serverdata` doesn't carry `item_template`'s `spellppmRate`.
  - Item damage is dealt as the item's server spell, with its range, whole rolls and missile travel, so class
    auras read its damage class and cast time: the proc trinkets, capacitors and Gnomish Lightning Generator
    (`itemDamageSpell` in `sim/common/wotlk`), and the explosives by their use spells (56350, 67769, 56488;
    `sim/core/consumes.go`). `legacyAPLItemIDs` keeps a saved rotation that names the item working. Still
    under the item: the items' heals.
  - Bryntroll's proc is Drain Life 71838 (heroic 71839), magic class, so it can crit. Its heal to the wearer
    isn't modelled.
  - Shard of the Gods: the Purified set (bonus 69755) casts only Cauterizing Heal 69733, the Shiny set (69739)
    only Searing Flames 69729, with no heroic ids: 25%, 45 s cooldown, off any landed magic damage or heal. A
    hit of the wrong kind uses the proc up and casts nothing (`SPELL_FAILED_BAD_TARGETS`).
  - Class relics, sigils, totems, idols and a few class set bonuses still carry Classic values until
    their class's P7 item. Totem of the Third Wind also buffs Healing Wave.
  - Verdicts per row are in `docs/azerothcore-item-diff/data/{effects,sets}_review.csv`.
