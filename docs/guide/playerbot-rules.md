# Playerbot rules

The Playerbot switch, `Player.playerbot` (`proto/api.proto`), sims a character as mod-playerbots plays it.
Off by default: the "Playerbot" checkbox in a sim page's Player settings. presetgen, statweights' capture
and the rankings always turn it on (their READMEs). Goldens run with it off.

## What it changes

All in the core, while the character is built (`sim/core/playerbot.go`, `Character.Playerbot`):

| Rule | Where | Bot evidence ([bots] = `[ac]/modules/mod-playerbots`) |
|---|---|---|
| No pre-pot. A DPS spec gets one offensive potion, Potion of Wild Magic for a caster except Shadow, Balance and Affliction, Potion of Speed otherwise, never Indestructible; a tank or healer none | `playerbotPotion` | `PlayerbotFactory::InitPotions`, `PlayerbotAI::IsDps` and `IsCaster` by spec |
| The potion waits until the fight is 5 s old, drunk by a top-priority action the rotation gets injected; every potion spell is held to that time, so no other entry drinks first | `addPlayerbotPotion` (`apl.go`), `makePotionActivation` (`consumes.go`) | `POTION_HOLD_MS`, `docs/systems/consumables-and-burst.md` |
| Pre-pull keeps only `castSpell` of `playerbotPrepullSpells`: stances, presences, Aspect of the Dragonhawk, Horn of Winter, Bone Shield. The rest is skipped with a "Playerbot:" validation warning, and the automatic -1 s pre-pot is off | `newAPLRotation` | stances and presences persist between fights; `GenericDKNonCombatStrategy` (Horn, Bone Shield), the hunter's `bdps` |
| No explosives, conjured items (Dark Rune, Healthstone, Flame Cap, Thistle Tea) or pet food | `playerbotConsumes` | [bots] never uses Thistle Tea or Flame Cap |
| Flask, elixirs and food from the world-buff matrix's level 80 row for the class and spec | `playerbotWorldBuffs`, generated | `WorldBuffAction::NeedWorldBuffs` |
| Focus Magic gives the mage no crit back, individual sim (uptime 0) or raid; a raid target keeps its buff | `sim/mage/focus_magic.go` | |

- The role reads `AiFactory::GetPlayerSpecTab` (`playerbotTalentTab`): the tree with the most points, ties
  to the lower one, a class default with no talents. A feral with Thick Hide 3 is a tank.
- The matrix's spec is that tree, but spec 3 for a feral without Thick Hide 3 (cat) and a blood DK without
  Blade Barrier 5 (`playerbotWorldBuffSpec`).
- A rotation's cooldown that existed only as a pre-pull cast never fires (Army of the Dead, Recklessness,
  Mirror Image). Fix that in the APL, not here.
- Not modelled: the burst gate (cooldowns, trinkets, racials and the tinker held until a tank has held the
  boss 5 s), pet scrolls, starting rage or runic power options.

## The bot flask and food table

`sim/core/playerbot_world_buffs_auto_gen.go`, written by `gen_server_defaults`
([README](../../tools/acore/gen_server_defaults/README.md)) from `Playerbots.WorldBuffMatrix`: the
`AC_PLAYERBOTS_WORLD_BUFF_MATRIX` line of `[ac]/configurationOverrides/Playerbot.env`, else
`playerbots.conf.dist`. Rerun it when the matrix changes. `TestPlayerbotWorldBuffsCoverInScopeSpecs`
fails when an in-scope spec has no flask or food.

The roster import's `ReadWorldBuffMatrix` (`tools/database/azerothcore/roster.go`) and acraid's
`crosscheck.py` (`matrix_text`) read these keys the same way. Only a file with no settings is the bare
matrix; settings without the keys give no matrix.
