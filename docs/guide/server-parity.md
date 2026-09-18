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
  Armor), and against a player only with a shield, at their sheet block chance.
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
  just the quiver's. The sim's ×1.15 is right only with a +15% quiver.
- **Binary spells** follow the spelldump. Mind Flay isn't binary; Steady Shot and Expose Armor are.
- **Armor debuffs:** Sunder ×5 (debuff 58567, not ability 47467) and Expose Armor don't stack. Faerie
  Fire multiplies.
- **Timing and procs:**
  - The rage hit factor is truncated to `uint32`.
  - Spell-proc PPM uses max(cast time, 1500 ms).
  - `REDUCE_PROC_60` applies.
  - Ranged-slot spells get +500 ms.
  - GCD is clamped to [1000, 1500] ms.
  - A DoT refresh resets the tick timer only when StackAmount < 2.
  - Periodic ticks crit only with aura 286.
- **Pets:** pet hit is floored to a whole percent. Pet scaling comes from the server's scripts.
- **Enchant PPMs:** Mongoose 1, Icebreaker 3, Deathfrost 3.

## Items

These come from acdiff ([README](../../tools/database/acdiff/README.md)).

- **Item levels:** only Ulduar-era items differ. Classic raised normal mode by +6 (226→232, 232→238) and
  hard mode by +13 (226→239, 232→245, 239→252), with stats rescaled.
  - The same bump hits Dalaran emblem vendors, Ulduar crafted patterns, Furious Gladiator gear,
    Algalon/Val'anyr rewards and Ulduar-10 loot. Classic moved Ulduar-10 loot to Titan Rune dungeons,
    which the server doesn't have.
  - That's 857 obtainable items, or 880 counting unobtainable ones. mod-individual-progression's 438
    rewritten TBC-era items are a separate category. Naxx, EoE, OS, Onyxia, ToC, ICC and RS all match.
- **Tooltip-parser gaps:** taking stats from the server also fixes what the sim's Classic parser misses:
  block value, spell penetration, socket-bonus MP5, old-style crit text.
- **Hardcoded Go effects:**
  - About 26 Ulduar-tier trinkets and relics carry Classic's rescaled values.
  - 19 proc-rate or mechanic rows differ, and so do 4 set bonuses.
  - Verdicts per row are in `docs/azerothcore-item-diff/data/{effects,sets}_review.csv`.
- **Sim bugs on any server:**
  - Forethought Talisman uses spell 3752 instead of 3572, and procs on crits only.
  - Totem of the Third Wind also buffs Healing Wave.
