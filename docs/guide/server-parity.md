# Server parity

Where the server differs from the Classic behaviour upstream modelled.
- Why the sim follows the server: [ADRs](../adr/).
- How each module is handled: [azerothcore-server.md](azerothcore-server.md#modules-that-affect-the-sim).
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

## Other measured server behaviour

- **Hunter haste:** mod-individual-progression's aura 89507 never changes the speed, so ranged haste is
  just the quiver's or ammo pouch's, which `Hunter.Options.quiver` picks (×1.15 by default), times
  Thori'dal's own ×1.15 (44972).
- **Talent values** are the spelldump's, not the DBC files': mod-spell-tweaks' `spell_dbc` rows change some
  while the live DBC files stay stock. Rage of Rivendare is 2 expertise a point, Virulence and Nerves of Cold
  Steel 2% hit a point.
- **Glyph of Reckoning** has a server script (mod-spell-tweaks) but no client data, so no player can have it:
  Hand of Reckoning never deals damage (67485) and the sim registers nothing for it.
- **Master Poisoner** (`SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER`) raises only its rogue's crit against a
  poisoned target, so the sim gives it to that rogue as a buff, not to the raid as a debuff.
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
  plus the first school's flat aura (Spark of Hope), then × (1 + Σ `SPELLMOD_COST` percents) + Σ flats,
  truncated, then × (1 + the first school's cost-multiplier aura, e.g. Power Infusion, Precision, Beast
  Within), truncated, at least 0. Percents add, never multiply; rage is in tenths. `DefaultCast.Cost` is the
  base; `StaticCost()` has the never-changing talents (options' `Multiplier`, `FlatMod`) and is what rotations
  read. Dynamic mods: `Spell.CostMultiplier`/`CostFlat`, `PseudoStats.CostMultiplier`/`CostFlat`,
  `Unit.AddSchoolCostMultiplier`/`AddSchoolCostFlat`.
- **Stat sheet:** armor on jewelry, weapons and `ArmorDamageModifier` is bonus armor for every class
  (`Player::_ApplyItemBonuses`); block value is half a point a strength for every player. Moonkin Form ×4.7
  item armor (24905) and the weapon's feral AP.
- **Nature's Swiftness** makes instant what its aura's class mask covers (`Spell.AffectedByAuraEffect`): heals
  and Rebirth, not Starfire.
- **Hasted channels:** the server truncates the duration and the tick period each (`int32(x × cast speed)`),
  so the aura can outlast period × ticks by a few ms.
- **AoE cap** counts every unit the effect hits (`AOECapMultiplierFor`): the caster of Thunderstorm and the
  Thermal Sapper, not the target a script drops (Mind Sear tick, Starfall splash). Guardians' casts and
  Explosive Decoy's blast are uncapped.
- **Power Infusion and Arcane Power** share group 1123 with no common aura type: the one cast last removes the
  other.
- **Armor debuffs:** Sunder ×5 (debuff 58567, not ability 47467) and Expose Armor don't stack. Faerie
  Fire multiplies.
- **Timing and procs** (the rest is in [spell data and timing](spell-data-and-timing.md)):
  - The rage hit factor is truncated to `uint32`.
  - Spell-proc PPM uses max(cast time, 1500 ms).
  - `REDUCE_PROC_60` applies.
  - A DoT refresh resets the tick timer only when StackAmount < 2.
  - Periodic ticks crit only with an aura 286 covering the spell, which can be a passive a skill teaches
    (SkillLineAbility AcquireMethod 2: Flame Shock's 75461, Immolate's 75445). Every sim dot declares it
    as `TicksCanCrit` (`sim/dotconfig_declared_test.go`).
  - A munched periodic (Deep Wounds, Unholy Blight, Piercing Shots, Righteous Vengeance, Ignite, Languish) applies
    on the next 400 ms step of the caster's event clock, up to 400 ms late (`CalculateQueueTime`,
    `MunchingBlizzlike.Enabled`), ahead of a swing or tick due on the same server tick. The sim does this for the
    first four (`core.DelayedPeriodicApplier`, a phase per caster per iteration).
- **Pets:** pet hit is floored to a whole percent. Pet scaling comes from the server's scripts.
  - A pet crits 5% plus crit auras, none from agility; the sim does this for the DK's summons but the
    rune weapon, and the hunter pet, so far.
  - The DK's summons carry scaling aura 67561, not 61017: no melee hit. Risen ghouls, not the army, take the
    owner's ArP (`Pet.HitScaling`, `Pet.RisenGhoul`).
  - Guardians take party and raid auras like pets (`AnyGroupedUnitInObjectRangeCheck`).
  - Haste carriers (the hunter pet, the DK's ghouls and gargoyle; `Unit.HasteCarrier`) ignore direct haste
    and slows, Bloodlust included, and take the owner's attack speed (`Pet.OwnerHasteSource`): every 2 s
    for a pet, once at the summon for a guardian.
- **Enchant PPMs:** Mongoose 1, Icebreaker 3, Deathfrost 3. An enchant procs only from the weapon that
  hit.
- **Dungeon scale** (`NewEncounter`): health and armor become `round(float32(value) × multiplier)`, half
  away from zero. Damage multiplies the target's damage dealt, so swings, spells and DoTs all get it,
  except spells with `SpellFlagIgnoreAttackerModifiers`. The server truncates each scaled hit; the sim
  doesn't. Targets with `world_boss` (else level ≥ 83) take the boss set. The placeholder target of an
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
  - Class relics, sigils, totems, idols and a few class set bonuses still carry Classic values until
    their class's P7 item. Totem of the Third Wind also buffs Healing Wave.
  - Verdicts per row are in `docs/azerothcore-item-diff/data/{effects,sets}_review.csv`.
