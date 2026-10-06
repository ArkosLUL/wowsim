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
- **map update length**: the live map update runs about 104 ms (world ticks of about 10.4 ms, sleeping past
  `MinWorldUpdateTime`), the sim's exactly 100. A whole-second dot's tick lands anywhere in a live update, but right
  on the sim's: Serpent Sting's 4th tick beats the next cast's landing about half the cycles on the server, every
  cycle in the sim.
- **Rime**: not a sim gap but the cycle's: its Howling Blast step finds no Frost, Unholy or death rune on either side,
  so it goes off only free, on a Freezing Fog proc (all 3 logged did), which the replay can't follow. The cycle needs
  runes left for it, then a re-record.
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
- **Lava Lash**: the server's hits about twice the sim's, which halves it as an off-hand hit (OHWeaponDamage).

| Setup | Subject | Check | Why |
|---|---|---|---|
| DeathknightFrost | 51411 | casts | Rime |
| DruidBalance | 48468 | tick crit mean | Vengeance |
| DruidBalance | aura 48517 on self | uptime | Eclipse |
| DruidBalancePhase3 | 48468 | tick crit mean | Vengeance |
| DruidFeral | 48574 | hit mean | Rake |
| HunterMM | 49001 | ticks | map update length |
| MageFire | energize 29076 mana | gain sizes | Master of Elements |
| MageFrost | aura 44549 on self | applications | Brain Freeze |
| MageFrost | aura 57669 on self | applications | Enduring Winter |
| MageFrostFire | 47610 | crit mean | Torment the Weak |
| MageFrostFire | 47610 | tick hit mean | Torment the Weak |
| MageFrostFire | energize 29076 mana | gain sizes | Master of Elements |
| ShamanEnhancement | 60103 | crit mean | Lava Lash |
| ShamanEnhancement | 60103 | hit mean | Lava Lash |
