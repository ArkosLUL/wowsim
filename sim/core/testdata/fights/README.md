# Fights

Each golden setup's fight on the boss dummy under mod-chronicle, which `TestFightReplays` replays
([tools/simval](../../../../tools/simval/README.md#fights)).

## cycles.json

One entry per `goldchar_setups.json` setup; the e2e runs it, the replay reads it.

| Field | Meaning |
|---|---|
| `setup` | the golden setup's name |
| `seconds` | how long the bot cycles |
| `yards` | distance from the dummy; up to 5 the bot also swings |
| `behind` | stand behind the dummy (melee DPS); else in front |
| `ammo` | hunters only: the ammo item set as the ammo slot (52020 Shatter Rounds, the sim's Iceblade Arrow setting) |
| `learn` | passives the golden build doesn't teach but the sim character has, which the e2e `.learn`s (Savage Defense) |
| `before` | steps cast once, in order, before the cycle; Auto Shot (75) here starts a hunter shooting |
| `cycle` | steps tried in order, round and round, whatever the server answers |
| step `spell`, `name`, `waitMs` | the spell id, the server's name, and how long the bot waits after trying it |

Rules, held by `TestFightCycles`:
- Every step is a spell of the setup's class the sim character can cast (`SpellFlagAPL`, through `fightSimActions`),
  never an on-next-swing one, and the rank a player gets: Sunder Armor is 7386, as the server turns the sim's
  47467 into an old flat-armor rank.
- A step waits out its GCD, cast time + 100 ms, or channel + 100 ms: a cast sent with more than the server's
  400 ms spell queue window left on the one before interrupts it.
- Proc-blind, and no spell whose cast time a proc changes (Slam with Bloodsurge, Exorcism with Art of War,
  Pyroblast with Hot Streak, Lightning Bolt with Maelstrom), nor a repeat inside its own cooldown (a proc's
  reset would give the server a cast the replay can't follow).
- Every spell goes off at least once, or the e2e fails the setup (`TestFightReplaysTheBotsOwnFight` checks the sim
  bot's): a rune comes back 10 s after use, so three of one rune type take 15 s; Aimed and Multi-Shot share a
  cooldown, as do the shocks.
- Cycles keep 500 ms of slack before each hardcast and after an instant, and rogue, cat and Protection warrior steps
  run slow so random power swings refuse fewer casts.

## Committed fights

`<setup>.log.gz`: the e2e's `<setup>_<player>_<unix>.log` (`<unix>`: the fight's first second on the log's clock),
cut by `simval fight -trim`. Kept: the header and zone lines, the player's and its minions' unit lines, what they
did from the fight's start to its end to themselves or the dummy but a spell's plain `HIT` target result, a heal, a
summon or a cast other than the bot's, and before the start only their auras going on and off (what was up at the
pull, Sniper Training included). The 32 were fought on 2026-10-05 against the worldserver build of 16:52 (rev
3d89793c7): 106 KB.

## Left out

Representation gaps, not mismatches, each with its reason in `tools/simval/fight.go`, which the replay prints:
- `fightAurasLeftOut`: server auras the sim has as no aura (a healing cut, a second aura of one spell, a channel's
  aura on the caster), by id, and where when only there.
- `fightSimAurasLeftOut`: sim auras the server has as no aura (Hot Streak's crit counter), by sim action; left in,
  one would count as the server aura sharing its id.
- `fightMissesLeftOut`: logged misses that decide nothing the sim models or repeat another spell's.
- `fightRefreshesLeftOut`: auras whose refreshes the server does its own way; uptime is still compared.
- `fightEnergizeLeftOut`, and runic power: gains the sim gives some way the comparison doesn't read, or the server
  never logs.

## Dropped checks

Real sim mismatches, each a follow-up, in `droppedFightChecks` (`tools/simval/fight.go`); a check is named by its
subject (`47809`, `melee`, `Bloodworm melee`, `aura 17800 on target`, `energize 57946 mana`) and name. Why, by the
table's cause:
- **world tick**: the server moves a player every 10 ms world tick, so a swing lands at most that late; the sim's
  waits for the 100 ms map update and loses the overshoot, a few swings a fight; its last swing comes early, and
  with it the end of a swing-stacked buff (Fury of the Five Flights).
- **Bloodworms**: the sim's have the setup's Horn of Winter (+155 Str); the server's never get it, as a party or raid
  buff skips an uncontrollable guardian (`Spell.cpp` `TARGET_CHECK_RAID`).
- **Frost Vulnerability**: rolls spell hit on the server and missed; the sim's always lands.
- **Vengeance**: the sim's Insect Swarm ticks crit for Vengeance's doubled bonus, the server's for the plain 1.5 with
  the meta gem.
- **Eclipse**: the sim's Eclipse (Solar) procs on any landed Starfire, the server's on a crit only (spell_proc -48516
  HitMask 2).
- **Master of Elements**: the sim refunds 25% of a crit spell's base cost (30% less Burnout's 5%, which the server
  burns apart, unlogged) and nothing for a crit without a cost of its own (Living Bomb's explosion, 215 on the
  server) or a clearcast one; the server refunds 30% of each (`spell_mage_master_of_elements`).
- **Rake**: its first hit is about 0.65 of the sim's in every feral fight, the boss armor's cut, which the sim leaves
  out as for a bleed.
- **Torment the Weak**: the sim always adds its 12%; the server only against a snared or slowed target, and the boss
  dummy is immune to snares (creature_immunities -361).
- **Brain Freeze**: the sim's Fireball! never expires; the server's (57761) lasts 15 s, so a later proc puts it on
  again.
- **Enduring Winter**: the sim's refreshes Replenishment on every Frostbolt; the server's has a 6 s proc cooldown
  (spell_proc -44557).
- **Vindication**: the sim's goes on with every landed hit; the server's procs at 3 a minute (spell_proc -9452), and
  its 26017 rolls spell hit, so it can miss.
- **Crusader Strike**: the sim's refreshes Judgement of Wisdom; the server's doesn't.
- **Prey on the Weak**: the sim keeps its crit bonus up; the server's 58670 lasts 10 s and the talent's 2 s check puts it
  back only once gone (and only while the rogue's health percent is above the target's), so it's down about 2 s in 12.
- **Hemorrhage**: the sim leaves its debuff out with fewer than two raid members; the server puts it on.
- **Lava Lash**: the server's hits about twice the sim's, which halves it as an off-hand hit (OHWeaponDamage).
- **Rend**: with the +35% for a target above 75% health on both sides, the sim's ticks run 3% (Protection: 286 against
  278) to 11% (Arms, crits too) over the server's; cause not found.

| Setup | Subject | Check | Why |
|---|---|---|---|
| DeathknightBlood | Bloodworm melee | crit mean | Bloodworms |
| DeathknightBlood | Bloodworm melee | hit mean | Bloodworms |
| DeathknightFrost | 51714 | count | Frost Vulnerability |
| DeathknightFrost | 51714 | misses | Frost Vulnerability |
| DeathknightFrostUH | 51714 | misses | Frost Vulnerability |
| DeathknightFrostUH | melee | count | world tick |
| DeathknightUnholy | melee | count | world tick |
| DruidBalance | 48468 | tick crit mean | Vengeance |
| DruidBalance | aura 48517 on self | uptime | Eclipse |
| DruidBalancePhase3 | 48468 | tick crit mean | Vengeance |
| DruidFeral | 48574 | crit mean | Rake |
| DruidFeral | 48574 | hit mean | Rake |
| DruidFeralApl | melee | count | world tick |
| MageFire | energize 29076 mana | gain sizes | Master of Elements |
| MageFrost | aura 44549 on self | applications | Brain Freeze |
| MageFrost | aura 57669 on self | applications | Enduring Winter |
| MageFrostFire | 47610 | crit mean | Torment the Weak |
| MageFrostFire | 47610 | tick hit mean | Torment the Weak |
| MageFrostFire | energize 29076 mana | gain sizes | Master of Elements |
| PaladinProtection | 26016 | misses | Vindication |
| PaladinProtection | aura 26016 on target | applications | Vindication |
| PaladinProtection | aura 26016 on target | uptime | Vindication |
| PaladinRetribution | aura 53408 on target | applications | Crusader Strike |
| PaladinRetribution | aura 60314 on self | uptime | world tick |
| RogueAssassination | melee | count | world tick |
| RogueCombat | aura 58670 on self | applications | Prey on the Weak |
| RogueCombat | aura 58670 on self | uptime | Prey on the Weak |
| RogueCombat | melee | count | world tick |
| RogueSubtlety | aura 48660 on target | applications | Hemorrhage |
| RogueSubtlety | aura 48660 on target | uptime | Hemorrhage |
| RogueSubtlety | melee | count | world tick |
| ShamanEnhancement | 60103 | crit mean | Lava Lash |
| ShamanEnhancement | 60103 | hit mean | Lava Lash |
| WarriorArms | 47465 | tick hit mean | Rend |
| WarriorProtectionWarrior | 47465 | tick hit mean | Rend |
