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
  just the quiver's or ammo pouch's, which `Hunter.Options.quiver` picks (×1.15 by default).
- **Talent values** are the spelldump's, not the DBC files': mod-spell-tweaks' `spell_dbc` rows change some
  while the live DBC files stay stock. Rage of Rivendare is 2 expertise a point, Virulence and Nerves of Cold
  Steel 2% hit a point.
- **Glyph of Reckoning** has a server script (mod-spell-tweaks) but no client data, so no player can have it:
  Hand of Reckoning never deals damage (67485) and the sim registers nothing for it.
- **Master Poisoner** (`SPELL_AURA_MOD_CRIT_CHANCE_FOR_CASTER`) raises only its rogue's crit against a
  poisoned target, so the sim gives it to that rogue as a buff, not to the raid as a debuff.
- **Binary spells** follow the spelldump. Mind Flay isn't binary; Steady Shot and Expose Armor are.
- **Damage mods:** percent `SPELLMOD_DAMAGE` and `SPELLMOD_DOT` mods from talents, glyphs and set pieces
  multiply (`Player::ApplySpellMod`); only `sim/paladin` does so far (`spellModDamage`). A holy spell dealing
  weapon damage (seal procs, judgements) skips physical percent mods such as Two-Handed Weapon
  Specialization: `Spell::EffectWeaponDmg` and `Unit::MeleeDamageBonusDone` match auras by school.
- **Armor debuffs:** Sunder ×5 (debuff 58567, not ability 47467) and Expose Armor don't stack. Faerie
  Fire multiplies.
- **Timing and procs** (the rest is under [Spell data and timing](#spell-data-and-timing)):
  - The rage hit factor is truncated to `uint32`.
  - Spell-proc PPM uses max(cast time, 1500 ms).
  - `REDUCE_PROC_60` applies.
  - A DoT refresh resets the tick timer only when StackAmount < 2.
  - Periodic ticks crit only with aura 286.
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

## Spell data and timing

`RegisterSpell` applies the server's own data for the spell it's given (`Spell.ServerSpell()`), so a
spell is only as right as its id.

- **Flags** take the server's value for Binary, NoActiveDefense, CompletelyBlocked and Channeled, and
  only gain AlwaysHit, the ATTR7 no-dodge and no-parry bits, and IgnoreResists (ATTR4_NO_CAST_LOG, on
  non-physical spells). The yellow roll skips its whole table for AlwaysHit, crit aside.
- **Timing** applies to what the spell declares: cast time against `CastMs` (which already carries the
  ranged slot's +500 ms), GCD against `GCDMs` in category 133, and the cooldowns. A declared value
  inside what passive talents and glyphs can reach agrees; anything else is a conflict, and an
  allowlist entry either lets the server win or keeps the sim's value with a reason.
- **Wrapper ids** are the trap. The sim often deals damage under the id of a spell that, on the server,
  only triggers the real one (totems, Faerie Fire (Feral), Typhoon). The wrapper's
  flags are not the damage's, so those entries keep the sim's values until the class's P7 item moves
  the damage to the real id. When a suite moves for a spell you didn't touch, look here first.
- Missiles travel at least 5 yards (`Spell::AddUnitTarget`), in whole ms.

The server only acts on its map update, 100 ms live (`ServerSettings.MapUpdateInterval`; 0 means exact,
which is what unit tests want).

- Swings, hardcast completions and aura expiry land on the next tick, with a phase rolled per iteration.
  A swing restarts its timer from the tick it landed on, so the overshoot is lost, which costs a fast
  dual wielder several percent.
- A landed swing pushes the other hand to at least 200 ms. A melee swing restarts the ranged timer, and
  every Auto Shot restarts both melee timers (`Unit::_UpdateAutoRepeatSpell`).
- At the pull, and when auto attacks restart (breaking stealth), a ready off hand starts half the main hand's
  hasted attack time behind it (`Unit::Attack`).
  A weapon swap doesn't reset swings.
- A cast resets every swing timer when the server's `ResetsAutoAttack` says so. A triggered cast never
  resets, nor does one made instant or one an `SPELL_AURA_IGNORE_MELEE_RESET` aura covers (Maelstrom
  Weapon).
- A swing due during a hardcast waits for the update the cast lands on and goes after its effects; a cast
  started off that landing comes a later update, so it can't hold the same swing again. Melee
  waits out any hardcast, Auto Shot only one without `SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS`. With that
  attribute a player's melee timers stand still through the cast instead (Slam). A cast started by the
  main hand's last APL check comes before that swing: the server handles the session before
  `Player::Update`. A `CancelAutoSwing` from that check stops the swing. A haste change rescales only the
  time left on a frozen timer.
- A channel holds melee too. In the sim a channel opts in with `Spell.HoldMeleeUntil` (Army of the Dead;
  Bladestorm isn't a server channel), and Volley stands the ranged timer still (`suspendRangedAttackTimer`).
- The GCD follows `Spell::TriggerGlobalCooldown`: hasted only with `HasteGCD`, then kept within
  [1000, 1500] ms. Cast time scales by damage class: spell haste for magic, ranged attack speed for
  ranged, nothing for melee. Hasted GCDs and cast times truncate to whole ms before the clamp and the
  tick (`Unit::ModSpellCastTime`), which can save a tick.

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
