# Server validation

Checking the sim against the server: replays of committed captures, the live e2e that records them, and
offline cross-checks. Go tests, goldens and the UI are in [testing](testing.md). Run every command in the
toolchain container ([dev-environment.md](dev-environment.md)).

## Replays

- `tools/simval` replays simval records captured on the server, and rebuilds naked `info` characters and
  white swings (unarmed or main hand only) in the sim:
  `dock.sh run ./tools/simval -records /wotlk/sim/core/testdata/simval/simval.jsonl`.
  - `sim/core/testdata/simval/simval.jsonl` is the committed capture of the whole e2e suite; its README lists
    dropped records. `TestFixtureHasEveryE2ERecord` keys it on (command, spell id, class, dummy entry).
  - The simval sims build each dummy with `no_xp` and its own `creature_immunities` row
    (`dummyMechanicImmunities` in `tools/simval/damage.go`): -361 for the boss dummy 999000, -286 for 999001
    to 999008.
- `damage.jsonl`, `timing.jsonl`, `threat.jsonl`, `strikes.jsonl`, `buffs.jsonl`, `mobs.jsonl`, `goldchar.jsonl`,
  `talents.jsonl`, `spellmods.jsonl`, `costauras.jsonl`, `pets.jsonl` and `pets_casts.jsonl` in the same folder
  each have their own replay test and live capture: the
  [simval README](../../tools/simval/README.md)'s Damage probe, Timing probe, Threat probe, Buffs, Mob types,
  Golden characters, Talent sweep and Spell-mod sweep.
  - `timing.jsonl`, `threat.jsonl` and `strikes.jsonl`: `run.sh TestSimvalTiming`, `TestSimvalThreat` or
    `TestSimvalStrikes`, streamed as [below](#live-e2e). Cut `threat.jsonl` from the stream with
    `simval threat` (the Threat probe's Fixture cutter).
  - `buffs.jsonl` and `mobs.jsonl`: the `BUFFS` or `MOBS` log lines of `run.sh TestSimvalBuffs`, which needs
    `SIMVAL_GOLDCHAR_SETUPS` and `SIMVAL_BUFFS_PAIRS`, or `TestSimvalMobs`.
- `sim/core/testdata/fights/` holds a Chronicle-logged fight per golden setup, replayed by `TestFightReplays`
  (the simval README's Fights); recaptured with `run.sh TestSimvalFight` (the module README).

## Live e2e

Needs the user's OK: `cd [ac]/modules/mod-sim-validation && ./e2e/run.sh [TestName]`. The default is
`-run TestSimval` minus `TestSimvalTiming`, `TestSimvalThreat`, `TestSimvalStrikes`, `TestSimvalBuffs`,
`TestSimvalMobs`, `TestSimvalFight`, `TestSimvalEcCore3LowHealthFight`, `TestSimvalEcClass2Fight` and
`TestSimvalEcClass3Fight`, about 7 min. The module's `README.md` covers its commands, env vars, orphan cleanup
and every test's run line.

- Recapture the fixture, never append: the suite deletes its own records and the host side of the log mount is
  stale, so stream from inside the container, started before the run:
  1. `MSYS_NO_PATHCONV=1 docker exec ac-worldserver tail -F -n +1 /azerothcore/env/dist/logs/simval/simval.jsonl > tmp/stream.jsonl &`,
     note the PID and the start time in ms.
  2. `e2e/run.sh`, then stop the tail by PID (and its `tail` process in the container, which outlives it).
  3. Keep lines whose `unixMs` is at or after the start, dedupe by line (`awk '!seen[$0]++'`).
  4. `dock.sh run ./tools/simval -records /wotlk/tmp/<capture> -fixture` replaces the fixture. Every
     failing record is a sim/server mismatch: remove it, add a row to the fixture README, and open a
     follow-up. Never fix the sim here. Move the `damage` lines to `damage.jsonl`.
- Measurements, each run by name:
  - `TestEcCore6WorldTick`: the world ticks' lengths off `.server info`
    ([update timing](spell-data-and-timing.md#update-timing)).
  - `TestEcCore6MissileReach`: a caster's distance from the fight dummy and its combat reach
    ([missiles](spell-data-and-timing.md#server-data)).
  - `TestEcClass6…`: the rage and energy a miss takes ([miss cost](class-mechanics.md#miss-cost)), a summon's
    casts and unsummon by world update (`.simval trace`), the spelldump's `critCapable`.
  - `TestEcCore8…`: the healing taken terms on another bot and on a pet, Ancestral Awakening
    ([healing](damage-and-healing.md#healing)), a missile's flight under Bloodlust
    ([missiles](spell-data-and-timing.md#server-data)).
  - `TestEcTools3…`: a creature's swing against a tick on one update, and the execute phase's start, against
    mobs that swing back ([update timing](spell-data-and-timing.md#update-timing)).

Gotchas (GM commands' own are in [server-dev gotchas](azerothcore-server.md#server-dev-gotchas)):
- Armor can't be equipped or removed in combat (equip error 60), and `.simval threat` engages the dummy, so
  `.combatstop` first.
- Equipping starts item proc cooldowns (`Player::ApplyEquipCooldown`), so equip under `.cheat cooldown`.
- In a druid form, `.simval info` shows no main-hand weapon.
- An item's `SMSG_SPELL_GO` names the item as caster.
- An aura from `.aura` shows its effects after the unit's next update, and an area aura reaches another unit
  on that unit's next aura update: wait before reading them.
- `.modify energy N` sets the energy and its maximum to 10 × N (`HandleModifyEnergyCommand`), so a change read
  right after it can be lost to the cap: spend energy with casts before the one you measure.
- The dummy's AI zeroes every hit, `.damage` included (set its health with `.simval hp`), and it gives no XP,
  so `REQ_EXP_OR_HONOR` procs never fire on it. It never swings either: for a target that swings and loses
  health, `fightMobs` and `fightMobsEngage` (the module README). Engaged mobs spread around the bot, so a cast
  that needs its target in front is refused (`UNIT_NOT_INFRONT`) unless `fightFace` turns the bot first, and a
  temp spawn outlasts the bot: kill the mobs at the end.
- A creature dodges from behind too (`Unit::MeleeSpellHitResult` turns dodge off only for a player victim), so
  a strike from behind the dummy can still fail to land.
- A spell whose mechanic the boss dummy is immune to comes out immune on every cast (Hamstring's snare, Kidney
  Shot's stun): the deterministic miss for a rule that reads one.
- To time the swing after a hardcast, don't space the casts by the attack time plus the cast time: no swing
  then falls between two casts.
- Every test's dummy stands on the hub's outer floor (`fightGround`, where `newBot` puts the bot), in sight of
  its centre. Critters wander there and can take an area or random-target spell's hit: `fightKillCritters`
  kills every creature within 40 yards (`fightBuild` first thing). At the entrance a spot south of the dummy
  has no line of sight past a few yards (4 for a short race), 18 yards north hides its centre from area
  scripts (Starfall), and a pet has no line of sight to a target beside the dummy (its cast is refused,
  `DONT_REPORT`).
- After a `.die`, select the bot again: `.simval hp` refuses a dead selection.
- Naxxramas, where the dummies stand, resets weekly (Saturday 17:00 local,
  `acore_characters.instance_reset`, map 533) and ports every bot inside out, so a path running across it
  fails with "Select the target dummy first": rerun it.
- The default run's Mind Flay can land in the server's 100% resist bucket (`Unit::CalcAbsorbResist`, when
  `float(rand_norm())` rounds to 1.0f: about one full capture in six), which the replay can't absorb: rerun
  `TestSimvalPriest` alone.
- mod-individual-progression cancels a vanilla or TBC spell rank as its cast time is up, for a level 70
  player past TBC (`OnPlayerSpellCast`; `IndividualProgression.EnableAllSpellRanks = 0`): cast the WotLK rank.

## Offline cross-checks

- `TestSimSpellGroups` (`sim/spell_groups_test.go`) checks the sim's exclusive-effect categories against the
  server's `spell_group` rules in `sim/core/testdata/simval/spell_groups.json`; the export query is in its
  doc comment. A pair the two disagree on needs a reason in `spellGroupAllowances`, and a stale entry fails.
- `TestSimAOESpells` (`sim/aoe_test.go`) checks the class code's chain targets, per-jump multipliers, max
  targets and ten-target caps against the spell capture. A sim-named spell that chains, picks targets or
  gets the cap needs an `aoeSpells` entry, a disagreement a reason in `aoeAllowances`; stale ones fail.
- Spell data: `tools/acore/spellids` lists the spell ids the sim needs and checks them against the committed
  capture. Its README's [Capturing](../../tools/acore/spellids/README.md#capturing) section recaptures
  missing ones live.
- Roster export: `tools/database/acraid/crosscheck.py`
  ([acraid README](../../tools/database/acraid/README.md#cross-check)).

## Parity is done when

- computed chances match within ±1 bp
- rolled rates are within |z| ≤ 5
- multipliers are within ±0.1%
- recorded runs are within ±2% DPS
