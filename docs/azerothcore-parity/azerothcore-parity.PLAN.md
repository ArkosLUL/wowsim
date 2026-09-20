# AzerothCore parity rework for wowsimwotlk

## Context

`G:\DevStuff\GitHub\wowsimwotlk` (**[sim]**) models WotLK *Classic*: 3.4 client data, Classic hotfixes, Wowhead
Classic items. The user runs an AzerothCore 3.3.5a server, `G:\DevStuff\GitHub\azerothcore-wotlk-pb` (**[ac]**),
in Docker with many modules, and wants DPS numbers that exactly match that server.

An investigation compared the sim's formulas with the server's code, SQL, DBCs, modules and live config.
Its findings, verified against code, and the list of AzerothCore deviations from retail are in
`azerothcore-parity.INVESTIGATION.md` (same directory). The Phases turn the sim into a server-exact model.

## Decisions (settled with the user)

**Scope and truth**
- Copy the server exactly, including where AC differs from retail. Log every retail deviation in the
  INVESTIGATION doc, so the user can later patch the server and update the sim.
- Replace Classic rules in place: no ruleset toggle, a permanent fork with no upstream pulls.
- Audience: the user only, running locally (container `wowsims-wotlk`). No share-link migrations, no "unverified" labels.
- Specs: DPS first, then tanks. Healers are out of scope; their golden baselines may move.
- Done means:
  - every mechanic matches the server within noise (computed chances ±1 bp, rolled rates |z| ≤ 5, multipliers ±0.1%);
  - a recorded dummy run's DPS is within ±2% of the sim using the same gear, talents and rotation.

**Data and modeling**
- Spell flags and timing (damage class, school, cast, GCD, CD, duration, stacks, binary/defense attributes) come
  from generated server data.
- Damage numbers and coefficients stay hardcoded in Go, and a test fails on any mismatch with server data.
- Items, gems and enchants come from the live MySQL world DB.
- **Classic-only / unobtainable items are owned by the separate loot-disparity (item-diff) effort.** This plan consumes its DB output.
- Model the 100 ms server update (`AC_MAP_UPDATE_INTERVAL = 100`) as a setting; 0 means exact time for unit tests.

**Server modules**
- mod-spell-tweaks: every toggle is exposed in the UI, defaulting to the live config, so tweaks can be evaluated before deploying.
- mod-reforging: a manual per-item reforge selector, no optimizer. **One shared model**, master's `ItemSpec.reforge`
  (`sim/core/reforging.go`), which acraid already exports.
- mod-racial-trait-swap: master's `Player.racial_traits` field, separate from `race` (race keeps base stats).
  The raid importer sets race = real race and traits = swapped race. Invalid-for-class combinations like Draenei traits on a druid are allowed.
- mod-dungeon-scale: the full-raid stat multipliers are **server settings, not constants**. They are
  `DungeonScale.StatModifierRaid[Heroic][.Boss].{Global,Health,Armor,Damage}`. Defaults come from the live config and can be changed in the UI.
  Live today: boss Health 1.2, Damage 1.0, everything else 1.0. Rerun `gen_server_defaults` whenever the config changes.
  No partial-raid curve (bots fill raids).
- mod-individual-progression: the damage penalty below tier 13 is ignored.
- Hunter talent tier swap (Improved Concussive Shot ↔ Go for the Throat): keep stock tree positions. It has no DPS effect, and the importer maps talents by stock positions.

**Rotations, presets, tanks**
- P7 fixes each class's default APL where a server change shifts priorities.
- Gear presets are rebuilt on server items by the BiS optimizer (BIS-presets).
- Rotations of the recorded-run specs are matched to what the user actually presses.
- Tanks use a generic boss built from AC creature stats (level 83 boss, `creature_classlevelstats` damage/armor).
  Specific encounter AIs are left for later.

**Validation**
- New `[ac]/modules/mod-sim-validation`: its own local git repo, `Enable = 0` by default, GM-only, built into the live server.
- Synthetic `.simval` commands for every mechanic. No custom record mode.
- Recorded runs use **mod-chronicle raw logs**:
  - Chronicle only logs inside dungeon/raid instances, so spawn the boss dummy inside an instance.
  - Download the raw file from the Chronicle app API (`GET /api/v1/.../logs/{logID}/files/{fileID}/download`), because the server deletes its copy after upload.
  - Chronicle gaps (no MH/OH flag on swings, no white-hit rage or rune RP, aura refresh re-emits APPLIED) are covered by the synthetic commands and code reading.
  - If a check truly needs a missing event, add a narrow hook to mod-sim-validation. Don't fork mod-chronicle (third-party, Emyrk).
- Recorded-run specs: **Retribution Paladin, Hunter, Affliction Warlock, Protection Paladin**.

**Process**
- Loop-driven: the wave loop stops after every wave for user review (test output, simval comparisons,
  per-suite DPS deltas). Git and server actions follow the [RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md)'s
  standing authorizations.

## Environment and coordination

- Go isn't on the host PATH. Build and test through `tools/acore/dock.sh` (P0), which runs the Dockerfile's `toolchain`
  target (golang 1.23 + node + protoc). Python 3.14 is on the host.
- DBCs:
  - stock 3.3.5a: `A:\WOW\dbc\Clean`
  - user-edited client: `A:\WOW\dbc\Changed`
  - live server: container `ac-worldserver`, `/azerothcore/env/dist/data/dbc/`
- Live MySQL: container `ac-database`, `127.0.0.1:3306`, DBs `acore_world`/`acore_characters`. DSN in
  `docs/azerothcore-item-diff/azerothcore-item-diff.PLAN.md`.
- Live config: `[ac]/configurationOverrides/*.env` (`AC_*` vars override conf keys) and `[ac]/env/dist/etc/**/*.conf`.
- **Where the work happens:**
  - From P4's review on, parity runs as `PAR-` work items ([below](#loop-work-items)) in the
    [wave loop](../wave-loop/wave-loop.RUNBOOK.md). Each item gets its own worktree off `integration`.
  - P1 lives in [ac] as its own git repo: `[ac]/modules/mod-sim-validation` (`modules/*` is ignored by the
    [ac] repo).
  - Item-diff and raid-import run in the loop too ([workstreams](../guide/workstreams.md)).
- **Sequencing:** P1, P0, P2 and P4 are built. P4's review and everything after it follow the
  [wave registry](../wave-loop/wave-loop.PLAN.md#wave-registry).
- Golden baselines: 37 `sim/**/*.results`. `make update-tests` deletes **all** `.results` before copying the `.tmp`
  files, so never use it for partial runs. Promote only the suites that ran (P0 `promote`) after checking each DPS
  delta's sign and size.

## Phases

### P1 — Server module `[ac]/modules/mod-sim-validation`

**Status:** built, deployed and verified on the live server by the module's own e2e suite
(`[ac]/modules/mod-sim-validation/e2e/run.sh`, results in the INVESTIGATION's "Verified on the live server"). Module repo
commits aa99da4 (module), 69e75fb (armor default fix, `e2e/`) and 5b57288 (review fixes). Everything after aa99da4
ships with the next server rebuild; until then pass the armor debuff explicitly (`58567:5`). The module's `README.md`
documents every command, the output format and the e2e tests.

**As built**
- `src/SimValidation.{h,cpp}`: config (`SimValidation.Enable` = 0, `MaxIterations` = 1000000, `OutputDir` = `simval`,
  `Dummy.CombatTimeoutSeconds` = 30), mutex-guarded writes to `<LogsDir>/simval/` (host: `[ac]/env/dist/logs/simval/`).
- `src/SimValidationJson.h`: minimal JSONL writer.
- `src/SimValidationSnapshot.{h,cpp}`: unit snapshots, plus `DeriveWhiteTable`, `DeriveYellowTable` and
  `DeriveMagicTable`, which redo the arithmetic of `RollMeleeOutcomeAgainst`, `MeleeSpellHitResult`/`isSpellBlocked`
  and `MagicSpellHitResult` on the server's own inputs. They must track those functions.
- `src/SimValidationCommands.cpp`: `.simval` command table (SEC_GAMEMASTER).
- `src/SimValidationDummy.cpp`: `npc_simval_dummy` (NullCreatureAI; zeroes damage in `DamageTaken`, which leaves
  combat logs, Chronicle and rage untouched; DoT ticks keep combat alive).
- `data/sql/db-world/base/simval_dummies.sql` (applied by db-import as module SQL):

  | Entry | Dummy | Type | Rank / type_flags | Immunities |
  |---|---|---|---|---|
  | 999000 | lvl 83 boss | 5 giant (sim default) | 3 / 0x4 BOSS_MOB | -361 (raid boss set) |
  | 999001 | lvl 83 elite humanoid | 7 | 1 / 0 | -286 |
  | 999002 | lvl 83 elite beast | 1 | 1 / 0 | -286 |
  | 999003 | lvl 80 normal | 5 | 0 / 0 | -286 |

  All: faction 7, `unit_class` 1, pacified, rooted, `RegenHealth` 0, `flags_extra` NO_XP | NO_SKILL_GAINS.
- `data/sql/db-world/base/simval_commands.sql`: `command` help rows.
- Live config (approved): `configurationOverrides/DungeonScale.env` sets `AC_DUNGEON_SCALE_DISABLED_ID` to the stock
  list plus 999000-999003; new `configurationOverrides/SimValidation.env` (`AC_SIM_VALIDATION_ENABLE: "1"`) is in the
  worldserver `env_file` list of `docker-compose.override.yml`.
- Rebuild: `docker compose build ac-db-import ac-worldserver`, then `docker compose up -d` (in [ac]).

**Commands.** Select the dummy; `pet` switches the attacker to the player's pet/guardian. Each writes one JSON line
(`schema`, `command`, `map`, `attacker` and `target` snapshots, `derived`, results) to `simval.jsonl`.
- `info [pet]`: snapshot only.
- `melee <n> [mh|oh] [pet]`: `CalculateMeleeDamage` loop → outcome counts, mean damage/blocked/absorbed/resisted per outcome.
- `taken <n>`: the dummy's white swings against the player (tank tables).
- `yellow <spellId> <n> [pet]`: `SpellHitResult` histogram, `isSpellBlocked` rate, crit chance and rolled crits.
- `spell <spellId> <n> [pet]`: `yellow` plus `CalcAbsorbResist` buckets on 10000 damage and both `GetEffectiveResistChance` variants.
- `armor [spell:<id>] [pet] [<auraId>[:<stacks>] ...]`: `CalcArmorReducedDamage` on 100000 damage, alone and with each
  debuff and all together (default `58567:5 770`). Temporarily applied auras are removed afterwards.
- `procs [<spellId>]`: item chance-on-hit spells and weapon enchants per attack type (mirrors `CastItemCombatSpell`),
  and `Aura::CalcProcChance` for every applied aura with a `spell_proc` entry (MH/OH/ranged white hit, given spell).
- `face`, `hp <percent>`: turn the dummy toward the player; set its health (execute ranges).
- `spelldump id <id...> | ids <file> | family <n>` (console too): in-memory `SpellInfo` after DBC, `spell_dbc` and
  corrections, per-effect data, bonus data, proc entry and script names, one line per spell in `spelldump.jsonl`
  (overwritten each run). Relative `ids` files are read from the output dir.

**Verification** is automated: `e2e/run.sh` (about 30 s) creates GM test characters, runs every command against the
four dummies inside Naxxramas, asserts the records, and deletes the characters and accounts. Rerun it after any change
to the module or to the combat code it mirrors. The e2e tests also produce the JSONL fixtures P2's `tools/simval` needs.

**Risks**
- GM commands run on the world thread, so the whole server stands still for the length of a loop (1M melee swings take
  seconds); `MaxIterations` caps it.
- The derived tables duplicate server arithmetic; a core change that isn't mirrored shows up as a rolled-vs-derived mismatch.

### P0 — Docker harness (in the parity worktree)

**Status:** built and verified. All 37 suites pass with every golden unchanged, so the committed `.results` are the
reference every later phase measures against.

**As built.** `tools/acore/dock.sh` (Git Bash), run from anywhere in the worktree. It builds the Dockerfile's
`toolchain` target as `wowsim-toolchain` on first use and runs everything in it, mounting the worktree at `/wotlk`,
`A:/WOW/dbc` at `/dbc` and [ac] at `/ac` (both read-only, skipped when missing, overridable with `DBC_DIR`/`AC_DIR`),
named volumes for the Go module and build caches, and `host.docker.internal` for the live MySQL. The P1 fixtures land
in the container at `/ac/env/dist/logs/simval/`.

| Command | Does |
|---|---|
| `build` | rebuild the image (otherwise built on demand) |
| `proto` | `make proto`: Go and TypeScript protobuf code |
| `test [args]` | `go test --tags=with_db`, default `./sim/...`; args go to `go test`, e.g. `test -run TestBlood ./sim/deathknight/dps` |
| `tsc` | generate the UI's protobuf and index, then `npx tsc --noEmit` |
| `run <pkg> [args]` | `go run` a tool package |
| `exec <cmd>` | anything else in the container |
| `dps [dir]` | DPS per test from the `.results` goldens |
| `delta [dir]` | dps, hps, tps and dtps of the last run against the goldens, per moved test, plus new and gone tests |
| `promote <dir>` | copy that dir's `.results.tmp` over its `.results` |

- `test` generates the Go protobuf code and `binary_dist/dist.go` when missing; without the latter `sim/web` doesn't compile.
- `promote` only touches the given directory, unlike `make update-tests`, which deletes every golden in the repo and so
  silently promotes suites the run never touched. It also keeps the goldens' CRLF line endings, which the container's
  Go writes as LF.
- Character stats, casts and stat weights need a plain diff of `.results` against `.results.tmp`.

**Verified:** `dock.sh test` green, `dock.sh delta` reports 37 of 37 suites unchanged, `dock.sh tsc` clean, `proto`,
`run` and the `/dbc`, `/ac` and `host.docker.internal` mounts all work, and a `promote` leaves the goldens
byte-identical.

### P2 — Core combat tables (`sim/core`)

**Status:** built and verified against the live server. All 37 suites pass and their goldens are promoted; every spec
gained DPS, mean +4.07% (median +3.99%, range -0.20% to +10.90%). `tools/simval` replays a 22-record capture from
mod-sim-validation and all 135 checks pass. The code-review fixes (below, "Review fixes") landed after that promotion
and move DPS again: specs with guardians gain 0.1% to 0.7% on average now that guardians don't glance, and attacks
from the front shift by 0.001% from the block fixes. Their goldens are promoted too.

**As built.**

`sim/core/combat_table.go` holds the pure functions, in integer basis points built from 32-bit floats because that is
what the server does — a float64 version of the same arithmetic lands a point off on values like the 4.7% a boss
misses a player for. The plan's `AttackerSnapshot`/`DefenderSnapshot` became a single `MeleeTableInput`, since the
server's tables read the pair, not one side at a time.

- `LevelForTarget`, `MaxSkill`
- `WhiteMeleeTableBP` → `MeleeTableBP{Miss, Dodge, Parry, Block, Glance, Crush, Crit}`, each field that outcome's own
  width in the server's roll order. Widths can be negative, which the server also allows.
- `YellowMeleeTableBP(in, YellowOptions{Ranged, NoActiveDefense, BlockedInTable, NoDodge, NoParry})`
- `MeleeMissBP`, `GlanceBP`, `GlancingMultiplier`, `CrushBP`, `PartialBlockBP`, `MeleeCritSuppressionPct`
- `SpellMissBP`, `ResistanceConstant`, `EffectiveResistChance`, `ResistBuckets`
- `ArmorMultiplier`, `ArmorPenetrationCap`, `CreatureBlockValue`

`AttackTable` caches the pair's fixed half — levels, skills, the creature base percentages, glancing, crushing,
partial block and crit suppression — and `Spell.WhiteTableInput`/`YellowTableInput` add the hit, expertise, crit and
facing that move during a fight. `Unit.IsWorldBoss` comes from `Target.world_boss`, a proto3 `optional`: unset
means on at level ≥ 83, an explicit value always wins.

`spell_outcome.go` rolls `urand(0, 10000)` once and walks the table. Every `OutcomeMeleeSpecial*`/`OutcomeRanged*` is
the yellow table plus an independent crit roll plus an independent partial block roll, weapon abilities included (the
old `OutcomeMeleeWeaponSpecial*`, `OutcomeRangedHitAndCritNoBlock` and `OutcomeRangedCritOnly` aliases are gone).
The creature-vs-player path uses the same tables with the player's sheet avoidance; only crit and crushing read
defense skill.

**Review fixes** (code review of the P2 diff, all in the working tree):
- Partial block (`rollPartialBlock`): physical damage from the front only, and only when the result carries damage,
  since a `CalcOutcome` hit check never reaches `isSpellBlocked`. `OutcomeMeleeSpecialCritOnly` rolls it too. Against
  a player it uses their live block chance and needs `PseudoStats.CanBlock`.
- Metrics: every landed yellow hit increments exactly one counter (crit, else block, else hit), because the UI sums
  them into attempts. Crushes are aggregated (`TargetedActionMetrics.crushes`) and shown as Crush % in the damage
  taken table.
- A block inside the yellow table (`SpellFlagCompletelyBlocked`) is `SPELL_MISS_BLOCK`: bare `OutcomeBlock`, no
  damage, not landed. `OutcomeLanded` no longer contains `OutcomeBlock`; a partial block always carries Hit or Crit.
- `DodgeReduction` applies to the attacker's own swings only: Weapon Mastery, and Chill of the Throne on an ICC boss
  (really `SPELL_AURA_MOD_DODGE_PERCENT` -20 on the player, so it cuts the tank's dodge).
- Creature crits, white or yellow, come from `enemyCritChance` (defense, resilience, crit-taken reductions);
  `PhysicalCritChance` branches to it for enemy units. Hateful Strike crits ×2.
- Glancing needs `Unit::IsPlayer() || IsPet()`. Only `SPELL_EFFECT_SUMMON_PET` summons are pets, marked
  `Unit.SummonedAsPet`: hunter and warlock pets and the Master of Ghouls ghoul. Guardians (wolves, treants,
  shadowfiend, water elemental, gargoyle, AotD, fire elemental, ...) don't glance. Crushing still excludes anything a
  player controls.
- Multi-school spells meet the lowest of their schools' resistances (`Unit.schoolResistance`, `Unit::GetResistance(mask)`);
  holy counts as 0.
- `SpellFlagCannotBeParried`, `SpellFlagNoActiveDefense` and `SpellFlagCompletelyBlocked` are still set by no spell;
  P3's generated server data sets them.

Also changed:
- `spell_result.go`: spell miss from `SpellMissBP`, binary resist folded into the hit roll, spell crit with no
  suppression, expertise left continuous (the attack table truncates it to basis points).
- `spell_resistances.go`: the server's resist model. No partials for binary spells; holy only vs creatures, and
  through the level difference alone since it has no resistance stat. `AverageMagicPartialResistMultiplier` is now
  `1 - 15/415`.
- `flags.go`: `ResistanceStat()` returns `(stat, ok)` for a single school so holy stops reading the target's
  Strength, plus `SpellFlagNoActiveDefense`, `SpellFlagCompletelyBlocked` and `SpellFlagCannotBeParried`.
- Removed the pet "+1.8% crit" hacks (hunter pet, fire elemental, spirit wolves).
- `druid/feral/feral.go` reads the yellow table instead of the attack table's fields.
- UI: world boss checkbox in `encounter_picker.ts`, showing the derived value while `world_boss` is unset (presets
  and the default target leave it unset); crit cap in `player.ts` uses 0.6% suppression, 25% glancing, and remaining
  white avoidance `cap + 0.6 − expertise` below the 5.85/13.4 cliffs, 0 above them.

**Two sim bugs the rewrite exposed**, both verified against the server and neither a retail deviation:
- The armor penetration cap read the *attacker's* level where `CalcArmorReducedDamage` reads the *victim's*, so it
  capped at 15232.5 instead of 16635 against a level 83 target. Both are the same formula, `467.5·L - 22167.5`, at
  different levels. Full armor penetration now reaches through noticeably more armor.
- Boss block value was a hardcoded 76. `Creature::GetShieldBlockValue` is `level/2 + STR/20`, and
  `creature_classlevelstats` gives level 83 creatures 0 strength, so it is 41. The presets no longer carry the value
  at all; `NewTarget` derives it.

**Tests**
- `combat_table_test.go`: every number the e2e suite read off the server — 800/645/1400/560/2500 bp, DW miss 2700,
  non-boss 560/560/0, level 80's 500 and no glancing, glancing per level, the 23.4/53.6 white expertise cliffs against
  the 25.8/56 yellow caps, partial block 440, creature-vs-player 469/440/560 with no crushing, spell miss
  400/500/600/1700, average resist 0.036145, binary resist 0.
- `spell_outcome_test.go`: Monte Carlo over 400k rolls. White rates against the server's, P(crit ∧ block) =
  P(crit)·P(block), and the magic miss rate at 16.99% rather than 17%. For the review fixes: dodge reduction only on
  the attacker, one metric per swing, no block on hit checks, crit-only hits blocked at 4.4%, full blocks not landing,
  creature yellow hits against a player's sheet (dodge, shield-gated block, defense vs crit), only pets glance, and the
  world boss flag's default and overrides.
- `spell_resistances_test.go`: also the lowest-school rule for Frostfire and fire+holy.
- `spell_resistances_test.go`: rewritten for the server's model, plus the boss dummy's 72.89/18.07/9.04 buckets.
- `armor_test.go`: the armor penetration cap expectations follow the victim-level cap.

**`tools/simval`** replays the module's JSONL through the pure functions and reports PASS/FAIL per record.
`dock.sh run ./tools/simval` reads `/ac/env/dist/logs/simval/simval.jsonl`; `-fixture` copies it to
`sim/core/testdata/simval/` and `tools/simval/check_test.go` re-runs the same comparison from there, skipping when
there is no fixture. It compares basis point for basis point against the table the server derived in the same record,
so a mismatch points at a formula rather than at noise; rolled counts get a |z| ≤ 5 check on top. The plan called this
`combat_table_server_test.go` in `sim/core`; it lives beside the tool instead so the record types and the comparison
have one home.

**The fixture.** `sim/core/testdata/simval/simval.jsonl` is 22 records from a live run of
`[ac]/modules/mod-sim-validation/e2e`: `info`, `melee` (boss, level 83 humanoid, beast, level 80, and off hand),
`taken` (the boss swinging at the player), `yellow` for Heroic Strike, Auto Shot and Steady Shot, `spell` for
Frostbolt and Mind Flay, `armor` and `procs`. All 135 checks pass.

Capturing one is awkward: the e2e suite drops its own records on cleanup, so the file is back to empty by the time the
run finishes. Stream them out of the worldserver while it runs and deduplicate afterwards:

```
docker exec ac-worldserver tail -F -n +1 /azerothcore/env/dist/logs/simval/simval.jsonl > stream.jsonl &
modules/mod-sim-validation/e2e/run.sh
```

Reading the host side of the bind mount instead loses most of the records. A failed run can leave a character behind
whose account is already gone; `SIMVAL_ORPHANS=<guid>:<race>:<class>:<accountId> e2e/run.sh TestCleanupOrphans`
removes it.

### P3 — Server spell data, timing, resources, procs, DoTs

**Generated server data**
- `tools/acore/spellids` ([README](../../tools/acore/spellids/README.md)) collects spell ids: constants in `sim/**`
  named like spell or aura ids (810), the spells of db.json's items, sets, enchants and gems, every talent and
  glyph, and what all of these trigger.
- The capture, `assets/db_inputs/acore/spelldump.jsonl` (10,835 spells, 22 MB), holds SpellFamilyNames 3–11, 15
  and 17 plus those ids, merged by the module's `e2e/spelldump_capture_test.go`. Only 26545 and 53258, triggered
  by Lightning Shield and Empower Rune Weapon, are unknown to the server. P7 needs no recapture; an item that
  adds a spell reruns spellids and gen_serverdata.
- `tools/acore/gen_serverdata` ([README](../../tools/acore/gen_serverdata/README.md)) →
  `sim/core/serverdata/{spells,procs,enchant_procs,bonus}_auto_gen.go`, for the sim's ids and their triggers
  (972 spells), not the whole capture:
  - From the capture, per spell: damage class, school, cast ms (with the ranged slot's +500), GCD and category,
    CD, durations, StackAmount, costs, raw attributes, effects, the bounds passive spell modifiers put on cast
    time, GCD and cooldown (`CastMods`, `GCDMods`, `CooldownMods`), and `Flags`: UsesRangedSlot, Binary,
    NoActiveDefense, AlwaysHit, CompletelyBlocked, ResetsAutoAttack, HasteAffectsPeriodic, HasteGCD, Channeled,
    AutoRepeat, Passive, Positive.
  - From live MySQL: `spell_proc` (185 entries, counting the ones the server builds, which come from the
    capture), `spell_bonus_data` (149), `spell_enchant_proc_data` (all 42 rows). Rows resolved from the DB must
    match the capture, or nothing is written.
  - `serverdata` doesn't import `sim/core`, and its `Flags` type is independent of `core.SpellFlag`.
- `spell.go` `RegisterSpell` looks the spell up by SpellID. `Spell.ServerSpell()` returns the entry: nil without
  one, with `SpellFlagNoServerData` (`SpellFlag` is now `uint64`) or under a `ServerSpellID` entry.
  - Flags: Binary, NoActiveDefense, CompletelyBlocked (without CU_DIRECT_DAMAGE) and Channeled take the server's
    value. AlwaysHit (nothing reads it yet), ATTR7 no-dodge/no-parry and, on non-physical spells, IgnoreResists
    (ATTR4_NO_CAST_LOG) are only added.
  - Timing, for a spell with a cast, GCD or cooldown that isn't an enemy's: cast time vs `CastMs`, GCD vs
    `GCDMs` (0 outside `GCDCategory` 133, the only one the sim's GCD models), `CD` vs the own cooldown (else the
    category's; a missing timer is added), `SharedCD` vs the category cooldown. A value within the modifier bounds
    (±1 ms) agrees.
  - A declared value the server rules out is a conflict: the server's value applies unless the allowlist entry
    has `KeepSim`. A `KeepSim` entry can also turn down a flag the spell left out (a dummy cast whose damage a
    triggered spell deals). A `ServerSpellID` entry skips the data of a wrong id.
  - Entries live in `sim/<class>/serverdata_allowlist.go`, shared ones in `sim/core/serverdata_allowlist.go`.

**Rage, procs, swing timers**
- `rage.go`: truncated hit factor, doubled on crit after truncation.
- `aura_helpers.go`:
  - Spell-proc PPM = max(base cast, 1500)·PPM/600.
  - `ReduceProc60`.
- `attack.go`: each swing pushes the other hand's timer to at least 200 ms (`attackDisplayDelay`); on a shared
  tick the main hand swings first. A melee swing restarts the ranged timer.
- Swing reset (`cast.go` `castTiming.resetsSwing`): a `makeCastFunc` cast of a `ResetsAutoAttack` spell restarts
  every hand in full (`AutoAttacks.resetSwingTimers`), except when a server cast time was made instant or an active
  `SPELL_AURA_IGNORE_MELEE_RESET` aura covers the spell (Maelstrom Weapon). A resetting hardcast also allows no
  swings while casting. `makeCastFuncSimple`/`makeCastFuncAutosOrProcs` casts never reset: procs and autos are
  triggered, and off-GCD cooldowns the server resets with (Barkskin, Blood Tap) need class code.

**Server tick** (`sim.go`)
- `Simulation.NextServerTick(t)`: the first tick at or after t. Ticks sit at phase + k·interval: the interval is
  `Raid.Server.MapUpdateInterval` (0 returns t), the phase is rolled per iteration from the "Server Tick Phase" label.
- On the tick:
  - Swings. `WeaponAttack.timerAt` is the timer that haste and delays change, `swingAt` its tick; a swing
    restarts the timer from its tick, dropping the overshoot. `SetOffhandSwingAt` has no sim, so `swing()` rounds it.
  - Hardcast completion, and the CD starting then: `makeCastFunc` rounds `CurCast.CastTime` up.
  - Aura expiry (`Aura.Refresh`).
- Periodic ticks carry the leftover (`AuraEffect::Update`); P3-5 moves dot ticks to `NextServerTick` of their
  nominal time. Until then a channel's dot aura keeps its exact expiry (`Aura.exactExpiry`, set in `makeCastFunc`):
  outlasting its last tick would stall the rotation.

**Casting** (`cast.go` `castTiming`, read once per spell at registration from `Spell.ServerSpell()`)
- GCD, as `Spell::TriggerGlobalCooldown`: with a server GCD of 1000-1500 ms, hasted only with `HasteGCD` (whatever
  `IgnoreHaste` says), then at least 1000 and, unless the sim's GCD is above 1500 (hunter pets' 1.6 s, Shadowcrawl's
  6 s stand-ins), at most 1500. A server GCD of 0 leaves the sim's alone. Without server data: hasted unless
  `IgnoreHaste`, at least 1000. `Cast.EffectiveTime` no longer floors.
- Cast time, unless `IgnoreHaste`, as `Unit::ModSpellCastTime` by damage class: magic by cast speed, ranged by
  ranged attack speed (`unit.go` `ApplyRangedCastSpeed`), melee not at all, none only with `HasteAffectsPeriodic`.
  Without server data: cast speed. Hunter shots keep their own `CastTime` funcs under `IgnoreHaste`.
- +500 ms for ranged-slot spells: in `CastMs`, applied by `RegisterSpell`.
- Missile minimum distance 5: `Spell.TravelTime`, in whole ms.

**DoTs** (`dot.go`)
- A single refresh rule: reset the tick timer iff StackAmount < 2 and the cast isn't triggered.
- Ticks = floor(maxDur/amp), in integer ms.
- `TickHaste` modes:
  - `SpellHasteScalesBoth` (default)
  - `MeleeHasteAddsTicks` / `SpellHasteAddsTicks`: amp = int(amp·min(mult,1)), duration fixed
- `TicksCanCrit` defaults to false.

**Tests**
- `rage_test.go`: 2.6 s → 9, 3.3 s → 11, crit → 18.
- `server_tick_test.go`: `NextServerTick`, the per-iteration phase, swings on the tick (2.55 s → 2.6 s), haste
  rescaling the timer, the other-hand push in either list order, a swing restarting the ranged timer, aura expiry
  and hardcast completion on the tick, channels exact.
- `dot_test.go`: reset vs keep; 3000 ms × 0.8 over 15 s = 6 ticks.
- `cast_test.go`: the GCD rule, cast haste by damage class, swing resets (instant, hardcast, no flag, made instant,
  no server data, no cast, Maelstrom Weapon), every hand restarting, a prepull reset delaying the first swing.
- `ppm_test.go`: instant spell at 15 PPM → 37.5%.
- `sim/serverdata_test.go` builds every golden suite's character, each again over its class's glyphs, plus every
  race, hunter pet, pet talent, warlock summon and preset encounter. It fails on a conflict no entry covers, a
  stale entry (other values, or a registered spell without the conflict) and an entry without a reason.

**Verify**
- `.simval procs` matches the generated proc chances.
- A Chronicle log of a Ret Paladin on the boss dummy in an instance matches the sim debug log for:
  - 2H swing intervals (server tick)
  - Holy Vengeance tick times while stacking
  - Berserking/Mongoose proc counts over ~15 min
- The comparison tool is `tools/simval chronicle` (a parser for the raw lines, `<unix_ms>  EVENT,...`).

### P4 — Generated constants

**Status:** built and verified against the live server, then reviewed in wave A (PAR-P4R). All 37 suites pass with their goldens promoted.
Against P2's committed goldens 25 suites move, with suite means from −0.063% to +0.060% (ArP 13.99 → 13.9957 per 1%,
tank avoidance) and single short tests from −0.65% to +0.91% as their RNG paths diverge. `tools/simval` passes 508
checks over the 82-record fixture.

**As built.**

`tools/acore/gen_basestats` (`dock.sh run ./tools/acore/gen_basestats`) writes `sim/core/base_stats_auto_gen.go` and
`ui/core/constants/ratings_auto_gen.ts`. Flags: `-dbc` (default `/dbc/Clean`), `-ac` (`/ac`), `-dsn` (`AC_DSN`, else
the item-diff DSN on `host.docker.internal`), `-level` (80). Inputs:
- gt tables through the item-diff `azerothcore.ParseDBC`: gtCombatRatings (row cr·100 + level − 1),
  gtOCTClassCombatRatingScalar (keyed by its id column, (class−1)·32 + cr + 1), gtChanceToMeleeCrit(+Base),
  gtChanceToSpellCrit(+Base), gtRegenMPPerSpt. Clean, Changed and the live server's copies are byte-identical.
- Live `player_class_stats` (level 80) and `player_race_stats`.
- `Unit.h` `enum CombatRating`; `StatSystem.cpp` `UpdateAttackPowerAndDamage` (per-class `val2`, druids' `default:`
  form) and the `m_diminishing_k`, `miss_cap`, `parry_cap`, `dodge_cap` arrays; `Player.cpp` `dodge_base` and
  `crit_to_dodge` (`GetDodgeFromAgility`).

It fails when a rating the sim converts with one constant differs between classes or aliases (e.g. crit melee/spell),
and when the mana classes disagree on spell crit per intellect or mana regen per spirit (both stay single constants
because pets use them too). It imports `tools/database`, which pulls in `sim/core`, so `sim/core` must compile before
it runs: to change the generated file's shape, hand-edit it first, then regenerate.

Generated: the `CombatRating` enum, the named rating constants (`CritRatingPerCritChance`, …, exact float32 values
such as 32.78999), `SpellCritPerIntellect`, `ManaRegenPerSpirit`, `CombatRatingBase`, `CombatRatingClassScalars`
(melee haste 1.3 for Paladin, DK, Shaman, Druid; ArP 1.1 for every class; everything else 1), `ClassBaseStats`,
`RaceStatOffsets`, `ClassStatScaling`.

Sim:
- `base_stats.go`: `BaseStats(race, class)` works for any combination (P5's racial traits need that);
  `addClassStatDependencies` adds Str/Agi → AP/RAP and Agi → crit in `NewCharacter` (pets set up their own);
  `RatingPerPercent(class, cr)`.
- `PseudoStats.MeleeHasteRatingPerHastePercent` and the new `ArmorPenRatingPerPercent` are set per class in
  `NewCharacter`. `newPseudoStats` gives class-less units the unscaled values, and pets copy their owner's ArP
  conversion. The new `BonusArmorPenPct` (percent) holds Battle Stance (+6 with Wrynn's 2pc), Mace Specialization
  (warrior, rogue) and Serrated Blades. Blood Gorged's is class-masked, so it's `BonusArmorPenRating` worth the same
  percent on white swings and Plague, Blood, Heart, Death and Rune Strike only. `ArmorPenetrationPercentage` =
  rating / per-1% + bonus, capped at 100, as `Unit::CalcArmorReducedDamage` adds them.
- `avoid_dr.go`: `DodgeChance`, `ParryChance`, `DefenseMissChance`, `DefenseSkillFromRating` (float32, truncated to
  whole points). Only players diminish (`Unit.playerAvoidance`); creatures, pets included, add up plainly.
  Non-diminishing: class dodge base, base agility's dodge, `BaseDodge`/`BaseParry` auras, parry's 5%. Diminishing:
  agility above base, dodge/parry rating, defense skill · 0.04. No parry without `CanParry` or a parry cap (Priest,
  Mage, Warlock, Druid).
- Removed: the `/1.3` hacks, every class's Str/Agi → AP, Agi → crit and Agi → dodge dependencies and dodge/parry base
  constants, `ArmorPenPerPercentArmor`, `DefenseRatingToChanceReduction`, `MissDodgeParryBlockCritChancePerDefense`,
  `tools/base_stats_parser.py`, `assets/db_inputs/basestats/`.
- Value changes: Shaman base HP 6939 (was 6960), Warlock 7136 (7164), DK base mana 0 (1000). Mage and Priest get
  Str − 10 AP, non-hunters the server's ranged AP (unused), Hunter/Rogue/Shaman agility dodge, Hunter/Rogue the 5%
  base parry.
- UI: `mechanics.ts` re-exports the generated constants; the character sheet converts melee haste and ArP with
  `MELEE_HASTE_RATING_PER_HASTE_PERCENT_BY_CLASS` and `ARMOR_PEN_RATING_PER_PERCENT_BY_CLASS`.

**Tests**
- `sim/core/base_stats_test.go`: rating per 1% (25.223 hybrid melee haste, 13.9957 ArP); Orc Warrior base stats and
  AP; Shaman and Warlock HP; defense truncation (400 rating → 81); warrior avoidance naked and with 100 agility, 512
  dodge and 400 defense rating; no parry for priests; ArP with Battle Stance and the shared cap.
- `tools/acore/gen_basestats/source_test.go`: the parsers on a snippet with the druid switch traps, and on the real
  `[ac]` sources (skipped without `/ac`).

**Verification.** `[ac]/modules/mod-sim-validation/e2e` `TestSimvalBaseStats` (in the module's working tree,
uncommitted), about 90 s. One level-80 character per class, covering all ten races:
- The starting outfit is destroyed (`.additem <id> -1` for each equipped item from `character_inventory`).
- Warrior, Paladin, Hunter, Rogue and DK `.learn 3127` (Parry, a trainer spell GM-leveled characters never get).
- Each gets the raid buffs the sim adds for its class: Priest Fortitude 48161, Divine Spirit 48073, Shadow Protection
  48169; Mage Arcane Intellect 42995; Druid Mark of the Wild 48469; DK Horn of Winter 57623.
- The DK's account first gets a warrior at progression tier 18: mod-individual-progression only allows death knights on
  accounts that reached tier 12 (`CHAR_CREATE_DISABLED` otherwise).
- Records: `.simval info` naked; `info` with 400 defense rating (24774, 24775, 15804) and 512 dodge rating (67694);
  `info` + `armor` at 1378 ArP (71557, 71403) and at 1498 (+ 42976).

`tools/simval` gained an `info` check: it builds the sim character through `core.NewEnvironment` with the snapshot's
defense and dodge rating as bonus stats and compares primary stats, max health, armor, AP and RAP (as whole numbers),
melee and spell crit, real dodge, miss chance taken, parry (the sheet's, so only without defense or parry rating), mana
(the snapshot has current mana, so only without timed auras) and the ArP rating's percentage. The `armor` check now
runs the rating through `ArmorPenetrationPercentage`. All 373 checks over the 60 records pass; the records are in the
fixture.

**Left open**
- The server truncates primary stats to integers and the sim doesn't (a Gnome's 256.2 intellect is 256), worth up to
  one point of anything derived; `tools/simval` allows for exactly that.
- Talents that are percent auras on the server but rating in the sim (Lightning Reflexes, Catlike Reflexes, Shaman
  Anticipation, Deflection for rogues) still diminish in the sim: P7.
- Pet avoidance (server creatures: flat 5% dodge, no agility): P7.
- A `maxPower` field in mod-sim-validation's snapshot (next server rebuild) would let the mana check run with buffs.

### P5 — Server settings (proto + UI)
**Status:** proto, generator and threading done in wave A (PAR-P5-1), dungeon scale and the settings UI in wave C
(PAR-P5-23). P3-3 applies the map update interval, P6-1 the reforge settings.
Racial traits already landed on master (23d0796b5, tests in `sim/racial_traits_test.go`).
- `proto/common.proto`:
  - `ServerSettings` on `Encounter`:
    - map update ms
    - every mod-spell-tweaks toggle, plus exotic pet %
    - dungeon-scale full-raid multipliers (Global, Health, Armor, Damage) for raid and raid heroic, general and boss-specific
    - reforge % and stat list, now constants in `sim/core/reforging.go`
  - Before implementing, read `DungeonScale.cpp` to learn how Global, per-stat and Boss values combine (multiply or override). The sim has to use the same combination rule.
- `tools/acore/gen_server_defaults` parses `[ac]/configurationOverrides/*.env`, module `conf.dist` files and `worldserver.conf.dist` into `sim/core/server_defaults_auto_gen.go` and `ui/core/constants/server_defaults_auto_gen.ts`. A missing message means live defaults.
- Sim:
  - `server_settings.go`, threaded through `environment.go` → `raid.go` (`Raid.Server`) → `Character.Server()`.
  - `NewEncounter` (`target.go`) applies dungeon scale from the raid's `ServerSettings`:
    - The set comes from the encounter's `raid_difficulty` and `Target.IsWorldBoss` (`world_boss`, else level ≥ 83).
    - Health and armor: `round(float32(value) * multiplier)`, half away from zero, as the server's `round(uint32 * float)`.
      Health-based fight length sums the scaled health.
    - Damage multiplies the target's `PseudoStats.DamageDealtMultiplier`, reaching swings, spells and DoTs like the server's melee,
      spell and periodic hooks. The server truncates each scaled hit; the sim keeps damage fractional. Spells with
      `SpellFlagIgnoreAttackerModifiers` miss it.
    - Never scaled: the placeholder target of an encounter without targets, the only target simval builds (`info`; the other checks read record values).
- UI: `components/server_settings_picker.ts`, the "Server (AzerothCore)" section of the individual settings tab (not inside the raid
  sim) and of the raid settings tab.
  - `ui/core/encounter.ts` holds `raidDifficulty` and `serverSettings`, saved with the encounter; `serverSettingsChangeEmitter` fires for both.
  - Each input shows the live value (`server_defaults_auto_gen.ts`). A change writes its field; a value equal to live clears it.
  - Inputs: raid difficulty; a Global/Health/Armor/Damage × Boss/Other grid that edits the selected difficulty's size set (a live size key
    would hide a generic edit); the Spell tweaks switch (`enable`), which greys out and unchecks the 9 switches it gates; the 11 switches;
    exotic pet damage %.
  - Map update interval and reforging have no inputs (P3-3, P6-1).
- Tests (`server_settings_test.go`, `dungeon_scale_test.go`):
  - Missing settings message → live defaults.
  - Dungeon-scale multipliers: changing Health, Armor or Damage in the settings changes only target HP, armor or boss damage, using the server's combination rule.

### P6 — Items from the live DB (after the item-diff/loot effort lands)
**Item data**
- Read `docs/azerothcore-item-diff/azerothcore-item-diff.INVESTIGATION.md` and that effort's final `gen_db` output first. Consume its item DB; don't duplicate its conversion.
- Remaining work here:
  - Apply the `effects_diff.csv`/`sets_diff.csv` value changes to `sim/common/wotlk/*.go`, `sim/common/tbc/*.go` and `sim/<class>/items*.go`.
  - Replace hardcoded enchant PPM/ICD values with P3 generated data.
  - Re-point gear presets (`ui/<spec>/presets.ts`, `sim/<class>/**/*_test.go` gear sets) to server items and item levels.

**Reforging** is on master (23d0796b5): `ItemSpec.reforge`, `sim/core/reforging.go`, the gear editor's Reforging
tab and acraid's export. Only its percentage and stat list move into P5's server settings. It reads the
item's sim stats, so the planned `UIItem`/`SimItem.server_stats` (raw `item_template` stat types) are
needed only if P6's item data calls for them.

**Tests and verification**
- `tsc` passes.
- Same gear on a server character: `.simval info` ratings match the sim, with and without a reforge.

### P7 — Per-class audit (DPS specs, then tanks)
**Tooling**
- `tools/acore/spellaudit`:
  - go/ast scans `sim/<class>/**` for SpellIDs and nearby numbers: roll ranges, coefficients, cast/CD/duration/ticks, proc chance/PPM.
  - Joins them with the spelldump, `spell_bonus_data` (dot_bonus is per tick; fallback EffectBonusMultiplier, `SpellMgr.cpp:947-961`), `spell_proc` and `[ac]/modules/**` references.
  - Writes `docs/azerothcore-parity/audit/<class>.csv` with automatic verdicts, or "manual".
- `tools/acore/talentdiff`: talent/glyph **values** from Clean vs Changed DBC and live `talent_dbc`. Tree positions stay stock.

**Per-spell checklist**
- Base damage and coefficient.
- Cast time, GCD, CD.
- Duration, amplitude, tick count, haste mode.
- Damage class ↔ outcome function; school; binary flag.
- Tick-crit enablers; proc entries.
- Talent/glyph values and class masks.
- Bound scripts (`[ac]/src/server/scripts/Spells/spell_<class>.cpp`).
- Spell-tweak switches wired to P5.

**Rotations**
- Fix each class's default APL where a server change shifts priorities.
- For the recorded-run specs (Ret, Hunter, Affliction, Prot Paladin), match the APL to the user's real rotation.

**P7.0 shared**
- Check `sim/core/buffs.go`, `debuffs.go`, `consumes.go` and `racials.go` against the spelldump (armor debuff stacking via `.simval armor`).
- Remove the 10-target AoE cap (`target.go:78-83`) only if AC doesn't have one: grep `Spell*.cpp`, then test with 11 dummies.

**Class order and server-specific tasks**
1. **Death Knight:**
   - Disease add-tick haste with Epidemic.
   - Virulence, Nerves of Cold Steel and Rage of Rivendare values.
   - Blood Plague/Frost Fever tick crits; Frost Fever as magic damage class.
   - Ghoul: 70/30 scaling, owner haste and ArP, avoidance.
   - Gargoyle: 75% AP→SP.
2. **Hunter (recorded):**
   - Measured haste: remove ×1.15, model 89507 plus the quiver.
   - +500 ms on shots.
   - Pet: 22%/12.87% scaling, floored hit/expertise, owner haste and ArP.
   - Beast Mastery 22 pet points (`ui/core/talents/hunter_pet.ts:213`, `ui/hunter/sim.ts:90`).
   - Hunter's Mark off GCD; Explosive Trap via Trap Launcher.
3. **Rogue:**
   - Deadly Poison add-ticks and crit (tick timer kept while stacking).
   - Rupture add-ticks and core tick crit.
   - Poison proc data.
4. **Warrior:**
   - Rend add-ticks and snapshotted crit.
   - Titan's Grip without penalty.
   - Heroic Strike/Cleave without the DW miss penalty.
   - Deep Wounds munching.
5. **Retribution Paladin (recorded):**
   - CS/DS apply seal stacks; seal DoT crit.
   - Glyph of Reckoning (`proto/paladin.proto:122`, spell 67485) is unimplemented on the server, so the sim
     registers nothing for it. Model it only once the server does.
   - Judgement proc rules and damage classes.
6. **Shaman:** Feral Spirit 30% AP, haste inheritance and swing reset from data. Elemental: core rules only.
7. **Druid:** FF(Feral) → Clearcasting, Omen PPM rule, Moonfire/IS add-ticks and tick crit, Treants.
8. **Mage:** Frostbolt is non-binary; Ignite munching; Water Elemental.
9. **Warlock (Affliction recorded):** pet scaling with floored hit; Haunt and Drain Soul non-binary; curses and UA per the spelldump.
10. **Shadow Priest:** Mind Flay binary; Shadowfiend.
11. **Tanks** (Prot Warrior, Prot Paladin (recorded), Bear, DK tank):
    - Creature-vs-player tables; per-class DR; no crushing at +3.
    - Pet avoidance; Holy Shield and Shield Block data.
    - A **generic AC boss** preset from `creature_classlevelstats` (lvl 83, class 1, damage/armor/attack speed) replaces Classic encounter AIs as the tank default.

**Per class**
- Unit tests for new mechanics.
- Promote only that class's goldens.
- `.simval yellow|spell` for 2–3 key abilities.
- For recorded specs: a Chronicle log compared via `tools/simval chronicle`.

### P8 — Classic-only cleanup
Done when every "Classic" or `wotlk-classic-bugs` reference in `sim/` has been reviewed.
- JoW instant = 0.75 s (`sim/core/debuffs.go:221-239`) → use the P3 rule.
- Omen of Clarity cast-time handling (`sim/druid/talents.go:395-440`).
- Feral Spirit swing reset (`sim/shaman/feral_spirit.go:44-46`).
- Warlock and hunter pet hit from Classic tests (`sim/warlock/pet.go:313-340`, `sim/hunter/pet.go:152-168`).
- Warlock issues #328 and #329 (`sim/warlock/pet.go:34`, `inferno.go:160`).
- Serpent Sting tick crit (`sim/hunter/serpent_sting.go:38`).
- Expertise comment and the hardcoded boss block value of 76: both done in P2.

## Loop work items

Specs for the `PAR-` items in the [wave registry](../wave-loop/wave-loop.PLAN.md#wave-registry). The
[RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md) agent rules apply to every item, plus these:

- **Replay and deviations:**
  - The simval replay stays all-PASS. Thresholds: ±1 bp, |z| ≤ 5, ±0.1% on multipliers, ±2% DPS on
    recorded runs.
  - Log each retail deviation in the INVESTIGATION.
- **Core files:**
  - Class items never edit `sim/core/*`. A core fix a class item needs becomes a follow-up P7-0c item.
  - `SpellFlag` is a `uint32` with one bit left: P3-2 either widens it or keeps server data in a separate
    struct.
- **Spell-data split:** to keep items file-disjoint,
  - P3-4 stays out of `attack.go`: spell-proc PPM goes in `aura_helpers.go`/`ppm.go`.
  - P3-3 puts ranged-class cast scaling in `unit.go`, not in `RegisterSpell`.
- **`sim/core/serverdata`:** regenerate it with `gen_serverdata`, never by hand. It covers the sim's spell-id
  constants and what they trigger, so name any spell id the sim computes (Relentless Strikes'
  `58420 + talentPoints`) as a constant. One item per wave owns the generated files: P6-2 (C), P3-2 (D),
  P3-4 (E).
- **`TicksCanCrit`:** P3-5 adds it with default false. Each class item declares which of its DoTs may crit,
  and P8 flips the default.

| Item (wave) | Scope | Owns | Goldens | Needs |
|---|---|---|---|---|
| PAR-P4R (A, done) | Review-only: `git diff 7778c94d3 0400022b5`, fixing every finding | P4's files | may move all | – |
| PAR-P5-1 (A, done) | `ServerSettings` on `Encounter`, plus a raid size/difficulty field; `gen_server_defaults`; defaults resolution; threading | `proto/common.proto` (Encounter), `tools/acore/gen_server_defaults/`, `sim/core/{server_settings,server_defaults_auto_gen,environment,raid,character}.go`, `ui/core/constants/server_defaults_auto_gen.ts` | none | – |
| PAR-P3-1 (B, done) | `spellids` + `gen_serverdata`. One live `.simval spelldump` of all 10 class families plus item and enchant spells, so P7 needs no second capture | `tools/acore/{spellids,gen_serverdata}/`, `assets/db_inputs/acore/spelldump.jsonl`, `sim/core/serverdata/*_auto_gen.go` | none | – |
| PAR-P6-2 (C, done) | Apply the "real" rows of `effects_review.csv`/`sets_review.csv` in shared code; take enchant PPM/ICD from P3-1 data (the ICDs are `spell_proc` rows of the enchants' equip spells, e.g. Black Magic 59630, Lightweave 55640, Swordguard 55776, which the generated set lacks until looked up by constant). Also run testing.md's item-effect race check | `sim/common/{wotlk,tbc}/*`, `sim/core/mana.go` (45703), `sim/core/serverdata/*_auto_gen.go` | 14: DK dps ×4, balance ×2, mage ×4, FeralApl, Subtlety, FeralTank, Disc | P3-1; AC-2 (soft) |
| PAR-P5-23 (C, done) | Apply dungeon scale in `target.go`. `Raid.Server.DungeonScale.Multipliers` (P5-1) already picks the set and combines default × global × stat, so round health and armor in float32 as the server does, and scale boss swing and ability damage. `NewEncounter` needs the settings too, for health-based fight length. No per-instance or per-creature overrides, and simval dummies are never scaled. Plus the "Server (AzerothCore)" settings UI with the 11 spell-tweak toggles | `sim/core/target.go` + tests, `environment.go` (`construct`), `settings_tab.ts`, `ui/core/encounter.ts`, optionally `ui/raid/settings_tab.ts` | none | P5-1 |
| PAR-P3-2 (D, done) | `RegisterSpell` applies server flags and timing by SpellID; a conflict allowlist per class; missile minimum distance 5 | `sim/core/{spell,flags}.go`, `serverdata_test.go`, the allowlist files, `sim/core/serverdata/*_auto_gen.go` | broad | P3-1 |
| PAR-P3-3 (D, done) | Server tick from `Character.Server().MapUpdateInterval` (live 100 ms; 0 = exact, for unit tests); other hand pushed to ≥ 200 ms; ranged timer reset; `ResetsAutoAttack` (a per-spell flag: also apply the runtime rules for triggered casts, casts made instant and `SPELL_AURA_IGNORE_MELEE_RESET` auras); GCD via `HasteGCD`, clamped to [1000,1500]; +500 ms for ranged-slot spells; a map update interval input in the Server section | `sim/core/{sim,attack,cast,unit,aura,constants}.go` + tests, `ui/core/components/server_settings_picker.ts` | all 37 | P3-1, P5-1 |
| PAR-P3-4 (E, done) | Truncated rage factor; spell-proc PPM uses max(cast, 1500 ms); `ReduceProc60`; `spell_proc` chance and ICD; the JoW hack. Fold `sim/common/wotlk/proc_helpers.go` in: its `ServerProcFor` already applies `REDUCE_PROC_60`, so don't apply it twice. `spell_proc` phases: Elemental Focus Stone (65005) and Black Magic (59630) proc on cast, misses included; Flare of the Heavens, Show of Faith, Sif's Remembrance, Lightweave and Darkglow on hit, not on cast complete | `sim/core/{rage,aura_helpers,ppm}.go`, `debuffs.go` (JoW) + tests, `sim/core/serverdata/*_auto_gen.go` (for item proc spells), `sim/common/{wotlk,tbc}/*` (proc wiring) | ~29 | P3-1 |
| PAR-P3-5 (E) | DoT refresh rule; integer-ms ticks; `TickHaste` modes; `TicksCanCrit`. Put periodic ticks on `sim.NextServerTick` of their nominal time, keeping the nominal lattice so the leftover carries over (`AuraEffect::Update`), then drop P3-3's channel exemption (`Spell.keepChannelExpiryExact`, `Aura.exactExpiry`) | `sim/core/{dot,periodic_action,spell_outcome}.go`, `cast.go` and `aura.go` (dropping the exemption) + tests | nearly all | P3-1; P3-3 (soft) |
| PAR-P6-1 (E) | `UIItem`/`SimItem.server_stats`; reforge % and stat list from `Character.Server().Reforging` (the server floors `float32(value) × pct/100`; disabled drops reforges); generate the reforge limits `server_settings.go` hand-copies today; the "fewer than 10 stats" rule; item maps only through `AddToDatabase`/`Lookup*`; keep `core.CanReforge`'s signature (BIS-rules' `pool.go` calls it) or request the change; an in-game reforge e2e test; reforge inputs in the Server section. `pool.go`'s reforge options then follow the request's `ServerSettings`, and its template-only rule reads `server_stats` (BIS-rules' contract request): today a rating only an equip spell gives, and equal melee and spell stat types, pass as sources the server refuses | `sim/optimizer/pool.go` (reforge options), `ui/core/components/server_settings_picker.ts`, `tools/acore/gen_server_defaults/`, `proto/ui.proto`, `proto/common.proto` (SimItem), `sim/core/{database,database_load,reforging,bulksim}.go`, `reforging.ts`, `gear_picker.tsx`, `tools/database/azerothcore/roster.go` | none | AC-1, P5-1, BIS-contract |
| PAR-P7-0a (F) | Audit buffs, debuffs, consumes and racials against the spelldump; the AoE cap. Thorns (53307) isn't binary on the server: drop `SpellFlagBinary` in `buffs.go` and the shared allowlist entry | `sim/core/{buffs,debuffs,consumes,racials}.go`, `target.go` (`updateAOECapMultiplier`) | all 37 | P3-4, P5-23 |
| PAR-P7-0b (F) | Pet core: owner hit and expertise floored and refreshed every 3 s; haste and ArP inheritance (with spell tweaks' hunter pet and ghoul ArP, pets take the owner's ArP rating and percent-ArP auras, filtered by the pet spell's class mask; Blood Gorged lives on the DK's spells, so the ghoul's white swings need it explicitly); pet avoidance | `sim/core/{pet,avoid_dr,unit}.go` | ~17 pet suites | P3-3, P5-1 |
| PAR-TOOLS-RR (F) | `tools/simval chronicle` and a `procs` check; `spellaudit` and `talentdiff`; a playerbot recorded-run harness. It captures Hunter, Ret, Affliction and Prot Paladin runs against a dummy inside an instance, downloading raw logs from the Chronicle app API (:4000) | `tools/simval/*`, `tools/acore/{spellaudit,talentdiff}/`, `docs/azerothcore-parity/audit/*.csv`, `sim/core/testdata/chronicle/`, its e2e file | none | P3-1 |
| PAR-P7-\<class\> (G–J) | The class's P7 checklist, spell-tweak wiring, APL fixes, its P8 rows, its `TicksCanCrit` declarations, and a recorded-run comparison where one exists. Plus its P3-2 allowlist entries, which list what to fix: deal each wrapper spell's damage under the id the server uses and drop the entry (DK Death Coil 47632, Icy Touch 49909, Blood Presence 48266; druid Faerie Fire (Feral) 60089, Typhoon 53227; hunter Wild Quiver 53254; shaman Searing Totem 58702, Magma Totem 58735, Flametongue 10444, Fire Nova 61654; warrior Slam 47475 triggering 50783), declare the timing the sim got wrong (warrior Recklessness, Death Wish and Shattering Throw, whose Glyph 206953 is a Cataclysm item the presets shouldn't equip; shaman Nature's Swiftness 2 min, Fire Elemental Totem's 1 s GCD, the placeholder heal cast times; warlock imp Firebolt's 1 s GCD; hunter Nether Shock 40 s with Longevity, Improved Arcane Shot at +15% damage), and drop the swing resets the core now does (warrior Heroic and Shattering Throw, paladin Exorcism, shaman Feral Spirit and the fire elemental), adding one where it doesn't (DK Blood Tap 45529, off the GCD, like Barkskin's). Hunter pets' 1.6 s `PetGCD` and Shadowfiend's 6 s Shadowcrawl GCD are stand-ins the GCD rule leaves alone: replace them with the server's GCD plus an AI delay or Shadowcrawl's real cooldown, and Steady Shot and Multi-Shot can drop their `CastTime` funcs for the core's ranged rule. Tick-rounding the APL's `spell.cast_time` prediction belongs here too: it shifts rotations (measured: Elemental -2.2..+1.3%, Enhancement -1.0..+1.4%, Shadow, Fire and FrostFire smaller). Plus its P6-3 item rows: DK 45144/45254/Razorice; druid 45509/45270; paladin 47661/T9 2pc; shaman 40322/42598/45114/33506; mage T8 4pc | `sim/<class>/**`, `ui/<spec>/apls/*`, `ui/<spec>/{presets,inputs,sim}.ts`, `proto/<class>.proto`, its allowlist and e2e files | its class's suites | P3-2..5, P5-1, P7-0a/b; P7-T (soft) |
| PAR-P7-TANK (J) | The Lich King's Soul Reaper and Sindragosa's Frost Breath `StopMeleeUntil` calls overlap the core reset for these `ResetsAutoAttack` spells (69409, 69649/73061): drop them or check they agree. Generic AC boss from `creature_classlevelstats`; Holy Shield and Shield Block data; percent-aura talents (Lightning Reflexes, …); Classic references in encounter AIs. Boss spells with `SpellFlagIgnoreAttackerModifiers` (so `SpellFlagIgnoreModifiers` too) miss the dungeon-scale Damage multiplier the server applies (Leeching Swarm, 66240). Preset encounters set no `raid_difficulty` (a preset field is a proto change). Bulwark of Azzinoth's hits-taken PPM needs the attacker's hand: the server uses the wearer's attack time for it, so a dual-wielding boss's off-hand swings proc it and the sim's don't | `sim/encounters/**`, tank dirs, tank `ui/*/presets.ts` | 4 tank suites | DK, WAR, RET, DRU items; P7-0b |
| PAR-P7-0c (F2) | Core fixes the class items need, found in wave D. Swings aren't held during a hardcast that doesn't reset them (`UNIT_STATE_CASTING` gates `Player::UpdateMeleeAttackingState` for any cast; only Slam is affected today, and its `DelayMeleeBy` approximates it). Every Auto Shot also restarts the melee timers (`Unit.cpp:4129-4133`). `AutoAttacks.reset` desyncs a player's off hand by a random 0-50% of the main-hand swing, while `Unit::Attack` uses a deterministic 50%. `item_swaps.go` and the feral cat shift reset melee, which no server code seems to do | `sim/core/{attack,cast,item_swaps}.go` | several | P3-3 |
| PAR-P8 (K) | Sweep `classic`/`wotlk-classic-bugs` in `sim/`; flip `TicksCanCrit`; close this plan | `sim/**` references | all | every P7 item |

- **Class order:** DK and HUN (G); ROG, WAR and RET (H); SHA, DRU, MAG and WLK (I); PRI (J).
- **P8's listed items** go to P3-4 (JoW), DRU (Omen), SHA (Feral Spirit), WLK and HUN (pet hit, Serpent
  Sting) and TANK (encounter AIs).
- **Presets:** P6's re-pointing of presets moves to BIS-presets.

## Verification (end to end)
- **Every phase:** `tools/acore/dock.sh test` is green; goldens promoted per suite with DPS deltas reviewed.
- **Mechanics:** `tools/simval` shows every attack-table, spell hit/resist, armor and proc check PASS on the boss, non-boss and lvl-80 dummies.
- **Final:** for Ret Paladin, Hunter, Affliction Warlock and Prot Paladin, a playerbot records a 5-minute run on the boss dummy inside an instance, in real server gear (PAR-TOOLS-RR). A mismatch is a finding for that class's work item, not a blocker.
  - Chronicle's raw-log DPS is within ±2% of the sim's result with the same gear, talents and rotation.
  - Tick counts and proc uptimes match within noise.
