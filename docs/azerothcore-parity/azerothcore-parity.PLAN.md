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
- mod-reforging: a manual per-item reforge selector, no optimizer. **One shared model**, `ItemSpec.reforge {from, to}`.
  The raid-import effort currently puts reforges into Bonus Stats; it switches to `ItemSpec.reforge` once P6 lands.
- mod-racial-trait-swap: a new `Player.racial_traits` field, separate from `race` (race keeps base stats).
  The raid importer sets race = real race and traits = swapped race. Invalid-for-class combinations like Draenei traits on a druid are allowed.
- mod-dungeon-scale: the full-raid stat multipliers are **server settings, not constants**. They are
  `DungeonScale.StatModifierRaid[Heroic][.Boss].{Global,Health,Armor,Damage}`. Defaults come from the live config and can be changed in the UI.
  Live today: boss Health 1.2, Damage 1.0, everything else 1.0. Rerun `gen_server_defaults` whenever the config changes.
  No partial-raid curve (bots fill raids).
- mod-individual-progression: the damage penalty below tier 13 is ignored.
- Hunter talent tier swap (Improved Concussive Shot ↔ Go for the Throat): keep stock tree positions. It has no DPS effect, and the importer maps talents by stock positions.

**Rotations, presets, tanks**
- P7 fixes each class's default APL where a server change shifts priorities.
- Gear presets are re-pointed to server items after the loot effort lands.
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
- Stop after every phase for user review: test output, simval comparisons, per-suite DPS deltas.
- Git: read-only unless the user explicitly instructs a specific action. Rebuilding or restarting the live worldserver
  and any live config change need the user's OK each time.

## Environment and coordination

- Go isn't on the host PATH. Build and test via the Dockerfile `toolchain` target (golang 1.23 + node + protoc).
  Python 3.14 is on the host.
- DBCs:
  - stock 3.3.5a: `A:\WOW\dbc\Clean`
  - user-edited client: `A:\WOW\dbc\Changed`
  - live server: container `ac-worldserver`, `/azerothcore/env/dist/data/dbc/`
- Live MySQL: container `ac-database`, `127.0.0.1:3306`, DBs `acore_world`/`acore_characters`. DSN in
  `docs/azerothcore-item-diff/azerothcore-item-diff.PLAN.md`.
- Live config: `[ac]/configurationOverrides/*.env` (`AC_*` vars override conf keys) and `[ac]/env/dist/etc/**/*.conf`.
- **Where the work happens:**
  - [sim] parity work: worktree `G:\DevStuff\GitHub\wowsimwotlk-parity`, branch `azerothcore-parity`, created from
    `azerothcore-item-diff` at 176e93f3b. All P0 and P2-P8 changes go here.
  - `G:\DevStuff\GitHub\wowsimwotlk` is the raid-import session's checkout. Don't touch it.
  - P1 lives in [ac] as its own git repo: `[ac]/modules/mod-sim-validation` (`modules/*` is ignored by the [ac] repo).
- **Concurrent efforts in [sim]:**
  - *item-diff / loot disparities*
    - Committed as 176e93f3b (branches `master` and `azerothcore-item-diff`).
    - Code: `tools/database/azerothcore/` (MySQL + DBC readers, item conversion, `AddItemMod`) and `tools/database/acdiff/`.
    - Docs: `docs/azerothcore-item-diff/`.
    - That effort still owns these paths: reuse its code, and coordinate before changing it.
  - *raid import*
    - Plan `C:\Users\boss2\.claude\plans\i-want-to-simulate-functional-pie.md`.
    - Branch `azerothcore-raid-import`, created from item-diff.
    - Code: `tools/database/acraid`, `ui/raid/acore_*.ts`.
    - Shares the reforge and racial-traits models (see Decisions).
- **Sequencing:** P1 (done, pending verification), then P0, P2, P3, and so on.
- Golden baselines: 37 `sim/**/*.results`. `make update-tests` deletes **all** `.results` before copying the `.tmp`
  files, so never use it for partial runs. Promote only the suites that ran (P0 `promote`) after checking each DPS
  delta's sign and size.

## Phases

### P1 — Server module `[ac]/modules/mod-sim-validation`

**Status:** built, deployed and verified on the live server by the module's own e2e suite
(`[ac]/modules/mod-sim-validation/e2e/run.sh`, results in the INVESTIGATION's "Verified on the live server"). Module repo
commit aa99da4 holds the first version. Uncommitted and not yet deployed: the armor default fix (Sunder debuff 58567
instead of the ability 47467) and `e2e/`. The module's `README.md` documents every command, the output format and the
e2e tests.

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
  debuff and all together (default `47467:5 770`). Temporarily applied auras are removed afterwards.
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
- Loops block the map thread (1M melee swings take seconds); `MaxIterations` caps it.
- The derived tables duplicate server arithmetic; a core change that isn't mirrored shows up as a rolled-vs-derived mismatch.

### P0 — Docker harness (in the parity worktree)
- `tools/acore/dock.sh` (Git Bash):
  - `docker build --target toolchain -t wowsim-toolchain .`
  - Run with mounts: worktree → `/wotlk`, a go-mod cache volume, `A:/WOW/dbc` → `/dbc:ro`, [ac] → `/ac:ro`, plus `--add-host=host.docker.internal:host-gateway`.
  - Subcommands: `proto`, `test [pkgs]`, `promote <dir>`, `tsc`, `run <tool>`.
- Verify: today's tests pass unchanged. Record per-suite DPS as the reference.

### P2 — Core combat tables (`sim/core`)
**Proto:** `Target.world_boss` (`proto/common.proto`). When unset, a target at level ≥ 83 counts as a boss.

**New `sim/core/combat_table.go`**
- Pure functions on `AttackerSnapshot`/`DefenderSnapshot`, working in basis points:
  - `LevelForTarget`, `MaxSkill`
  - `WhiteMeleeTableBP`: server roll order, plus the skill-bonus quirk.
  - `YellowMeleeTableBP`: returns `PartialBlockBP` separately; block is in the table only for `COMPLETELY_BLOCKED` spells.
  - `RangedYellowTableBP`, `SpellHitBP`, `EffectiveResistChance`, `ResistBuckets`, `GlancingMultiplier`, `ArmorMultiplier`, `CreatureBlockValue`
- Creature-vs-player variants: crit 5% + (415 − def)·0.04 − resilience; crushing blows only at +4 levels.
- `rollBP = int32(RandomFloat·10001)`.

**Rewrite**
- `target.go` `NewTarget`/`NewAttackTable`: formula-derived fields replace the `UnitLevelFloat64` tables. No spell crit suppression. Add `IsWorldBoss` on `Unit`.
- `spell_outcome.go`:
  - Rewrite `applyAttackTable*` and `OutcomeMeleeWhite` (server order).
  - All `OutcomeMeleeSpecial*`/`OutcomeMeleeWeaponSpecial*`/`OutcomeRanged*` become table roll + separate crit roll + separate partial-block roll.
  - Enemy-table skill math.
- `spell_result.go`: expertise truncated to bp, physical crit −0.6, spell miss, binary resist folded into the hit roll.
- `spell_resistances.go`:
  - Server resist model.
  - No partials for binary, physical or ignore-resist spells.
  - Holy gets the level resist only.
  - Multi-school spells use the lowest resist.
- `flags.go`: holy fix, `SpellFlagNoActiveDefense`, `SpellFlagCompletelyBlocked`.
- `constants.go:24` and `spell.go:546-551`: replace the 0.94 average-resist constant.
- Remove the pet +1.8% crit hacks. Fix the direct table use in `druid/feral/feral.go:88-89`.

**UI and encounters**
- `ui/core/encounter.ts` defaults: world boss on; block value from P1 `info`.
- World-boss checkbox in `components/encounter_picker.ts`.
- Crit-cap numbers in `ui/core/player.ts:675-720`.
- Set `WorldBoss` in `sim/encounters/**`.

**Tests**
- `combat_table_test.go`, checking every number in the INVESTIGATION attack-table findings:
  - 800/645/1400/560/2500 bp; DW miss 2700; non-boss dodge/parry 560/560/0
  - glancing per level; the 23.4-expertise quirk; yellow expertise caps
  - spell miss 400/500/600/1700 bp; avg resist 0.036144; binary resist 0
- Monte Carlo `spell_outcome_test.go`: P(crit ∧ block) = P(crit)·P(block).
- `combat_table_server_test.go` on P1 fixtures.
- Update `spell_resistances_test.go`.

**Verify**
- `tools/simval` reads the P1 JSONL, rebuilds snapshots and calls the pure functions, printing PASS/FAIL per the Decisions tolerances. `-fixture` saves records into `sim/core/testdata/simval/`.
- It must PASS on boss, non-boss and lvl-80 dummies; MH, OH and DW; 2 yellow abilities; 2 spells.
- Promote goldens.

### P3 — Server spell data, timing, resources, procs, DoTs

**Generated server data**
- `tools/acore/spellids`: collects `SpellID` literals in `sim/**` plus triggered spells, then runs them through `.simval spelldump ids`. The capture is saved as `assets/db_inputs/acore/spelldump.jsonl`.
- `tools/acore/gen_serverdata` → `sim/core/serverdata/{spells,procs,enchant_procs,bonus}_auto_gen.go`:
  - From the spelldump, per spell: damage class, school, cast ms, UsesRangedSlot, GCD and category, CD, durations, StackAmount, Binary, NoActiveDefense, AlwaysHit, CompletelyBlocked, ResetsAutoAttack, HasteAffectsPeriodic, effects.
  - From live MySQL: `spell_proc`, `spell_enchant_proc_data`, `spell_bonus_data`.
- `spell.go` `RegisterSpell`: applies server flags by SpellID (opt out with `SpellFlagNoServerData`) and records conflicts.

**Rage, procs, swing timers**
- `rage.go`: truncated hit factor, doubled on crit after truncation.
- `attack.go` / `aura_helpers.go`:
  - Spell-proc PPM = max(base cast, 1500)·PPM/600.
  - `ReduceProc60`.
  - Other-hand 200 ms push; a melee swing resets the ranged timer.
  - Swing reset driven by `ResetsAutoAttack`, replacing the `StopMeleeUntil` use in `cast.go`.

**Server tick** (`sim.go`)
- Random phase per iteration, plus `NextServerTick`.
- Applied to swing scheduling (overshoot dropped), hardcast completion and aura expiry. Periodic ticks carry over the leftover time.
- Interval from `ServerSettings.map_update_interval_ms`.

**Casting** (`cast.go`, `unit.go` `SpellGCD`)
- GCD is hasted only with `HasteGCD`; clamp to [1000,1500].
- +500 ms for ranged-slot spells.
- Ranged-class cast time scales with ranged speed.
- Missile minimum distance 5.

**DoTs** (`dot.go`)
- A single refresh rule: reset the tick timer iff StackAmount < 2 and the cast isn't triggered.
- Ticks = floor(maxDur/amp), in integer ms.
- `TickHaste` modes:
  - `SpellHasteScalesBoth` (default)
  - `MeleeHasteAddsTicks` / `SpellHasteAddsTicks`: amp = int(amp·min(mult,1)), duration fixed
- `TicksCanCrit` defaults to false.

**Tests**
- `rage_test.go`: 2.6 s → 9, 3.3 s → 11, crit → 18.
- `attack_test.go`: other-hand push, quantized mean swing interval, swing reset.
- `dot_test.go`: reset vs keep; 3000 ms × 0.8 over 15 s = 6 ticks.
- `cast_test.go`.
- `ppm_test.go`: instant spell at 15 PPM → 37.5%.
- `serverdata_test.go`: fails on conflicts that aren't allowlisted.

**Verify**
- `.simval procs` matches the generated proc chances.
- A Chronicle log of a Ret Paladin on the boss dummy in an instance matches the sim debug log for:
  - 2H swing intervals (server tick)
  - Holy Vengeance tick times while stacking
  - Berserking/Mongoose proc counts over ~15 min
- The comparison tool is `tools/simval chronicle` (a parser for the raw lines, `<unix_ms>  EVENT,...`).

### P4 — Generated constants
- `tools/acore/gen_basestats` reuses the item-diff `tools/database/azerothcore/dbc.go` reader.
  - Inputs:
    - Clean gt tables: gtCombatRatings, gtOCTClassCombatRatingScalar (id = (class−1)·32 + cr + 1), gtChanceToMelee/SpellCrit(+Base), gtRegenMPPerSpt.
    - Live `player_class_stats`/`player_race_stats`.
    - DR constants and base AP formulas parsed from `[ac]/.../StatSystem.cpp`.
  - Outputs: `sim/core/base_stats_auto_gen.go` and `ui/core/constants/ratings_auto_gen.ts`.
  - Replaces `tools/base_stats_parser.py` and `assets/db_inputs/basestats/`.
- Per-class rating scalars:
  - Remove the `/1.3` hacks: `deathknight.go:394`, `druid.go:282`, `paladin.go:188`, `shaman.go:46`.
  - ArP conversion becomes per class.
  - Percent-ArP talents move to `PseudoStats.BonusArmorPenPct`: `warrior/stances.go:68`, `warrior/talents.go:418`, `rogue/talents.go:28,363`, `deathknight/talents_blood.go:367`.
- `avoid_dr.go`: per class, with defense rating converted to whole skill points.
- Verify: for a naked lvl-80 character of each class, `.simval info` matches sim stats; repeat with 1400 ArP.

### P5 — Server settings (proto + UI)
- `proto/common.proto`:
  - `ServerSettings` on `Encounter`:
    - map update ms
    - every mod-spell-tweaks toggle, plus exotic pet %
    - dungeon-scale full-raid multipliers (Global, Health, Armor, Damage) for raid and raid heroic, general and boss-specific
    - reforge % and stat list
  - Before implementing, read `DungeonScale.cpp` to learn how Global, per-stat and Boss values combine (multiply or override). The sim has to use the same combination rule.
  - `Player.racial_traits` (0 = own race).
- `tools/acore/gen_server_defaults` parses `[ac]/configurationOverrides/*.env`, module `conf.dist` files and `worldserver.conf.dist` into `sim/core/server_defaults_auto_gen.go` and `ui/core/constants/server_defaults_auto_gen.ts`. A missing message means live defaults.
- Sim:
  - `server_settings.go`, threaded through `environment.go` → `raid.go` (`Raid.Server`) → `Character.Server()`.
  - `racials.go` applies racials from `racial_traits`; base stats still come from `race`.
  - Dungeon-scale multipliers are applied in `target.go`:
    - Health → health-based fight length.
    - Armor → target armor.
    - Damage → boss swing and ability damage (tanks).
    - The raid vs raid-heroic set and the boss vs non-boss values are picked from the encounter's difficulty and `world_boss`.
- UI:
  - "Server (AzerothCore)" section in `components/individual_sim_ui/settings_tab.ts`, with every spell-tweak switch.
  - Racial-traits picker in `other_inputs.ts` (any race).
- Tests:
  - Missing settings message → live defaults.
  - Orc warrior with Human traits: Human weapon expertise, Orc base stats.
  - Dungeon-scale multipliers: changing Health, Armor or Damage in the settings changes only target HP, armor or boss damage, using the server's combination rule.

### P6 — Items from the live DB + reforging (after the item-diff/loot effort lands)
**Item data**
- Read `docs/azerothcore-item-diff/azerothcore-item-diff.INVESTIGATION.md` and that effort's final `gen_db` output first. Consume its item DB; don't duplicate its conversion.
- Remaining work here:
  - Apply the `effects_diff.csv`/`sets_diff.csv` value changes to `sim/common/wotlk/*.go`, `sim/common/tbc/*.go` and `sim/<class>/items*.go`.
  - Replace hardcoded enchant PPM/ICD values with P3 generated data.
  - Re-point gear presets (`ui/<spec>/presets.ts`, `sim/<class>/**/*_test.go` gear sets) to server items and item levels.

**Reforging (shared model)**
- Proto: `UIItem.server_stats` and `SimItem.server_stats` (raw `item_template` stat types), `ItemSpec.reforge {from_stat_type, to_stat_type}`.
- `sim/core/database.go` `NewItem`:
  - Validates the mod-reforging rules and moves floor(0.4·value).
  - Hit, crit and haste update both sim stat slots.
- UI: reforge selector in `ui/core/components/gear_picker.tsx`; `ui/core/proto_utils/equipped_item.ts`.
- Hand-off to the raid-import effort: switch `acraid`/`acore_roster.ts` from Bonus Stats to `ItemSpec.reforge`, and set `racial_traits` (from P5).

**Tests and verification**
- Reforge cases in `sim/core/database_test.go`: 83 crit → 33 haste; the rejection rules; gems untouched.
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
   - Glyph of Reckoning (`proto/paladin.proto:122`, spell 67485).
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
- Expertise comment (`sim/core/spell_result.go:85`).
- Boss block value 76 (`ui/core/encounter.ts:184`).

## Verification (end to end)
- **Every phase:** `tools/acore/dock.sh test` is green; goldens promoted per suite with DPS deltas reviewed.
- **Mechanics:** `tools/simval` shows every attack-table, spell hit/resist, armor and proc check PASS on the boss, non-boss and lvl-80 dummies.
- **Final:** for Ret Paladin, Hunter, Affliction Warlock and Prot Paladin, record a 5-minute run on the boss dummy inside an instance in real server gear.
  - Chronicle's raw-log DPS is within ±2% of the sim's result with the same gear, talents and rotation.
  - Tick counts and proc uptimes match within noise.
