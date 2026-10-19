# Spec balance: lift every in-scope DPS build to B tier

Paths: [sim] `G:\DevStuff\GitHub\wowsimwotlk` (master), [bal] `G:\DevStuff\GitHub\wowsimwotlk-balance` (this effort's
worktree), [int] `G:\DevStuff\GitHub\wowsimwotlk-int` (wave loop's), [ac] `G:\DevStuff\GitHub\azerothcore-wotlk-pb`,
[tweaks] `[ac]\modules\mod-spell-tweaks`, [bots] `[ac]\modules\mod-playerbots`, [simval]
`[ac]\modules\mod-sim-validation`. Each module is its own git repo.

Status: P0a, P0b, P1, P2 and P3 landed, with the user's P1 decisions. Results, later-phase notes and P2/P3's per-build bot findings in `spec-balance.INVESTIGATION.md`. Next: the baseline (after the BiS chain).

## Context

The sim's DPS rankings (`[sim]/assets/rankings/rankings.json`, page `ui/rankings/`, runner `tools/rankings/`) rank
25 DPS builds at BiS per content phase P1–P5 and fight length (60/180/300 s) in one standard scenario (full raid
buffs, a Draenei in every party, plain level 83 boss). Tier = % of the top build in that phase and fight: S ≥ 95,
A ≥ 90, B ≥ 85, C ≥ 75, D below (`ui/rankings/tiers.ts`). Most builds sit in C/D, far behind BM Hunter, Unholy 2H
and Assassination. The server's raiders are mostly playerbots. Goal: every in-scope build ≥ B as bots play it,
through rotation fixes, new scaling and modifier tuning on the server (mod-spell-tweaks), mirrored in the sim,
which models the server exactly (ADR 0001).

## Decisions (user, 2026-10-18)

**Target and scope**
- Counted cells: 180 s and 300 s, P1–P5. 60 s is reported only.
- Out of scope: Smite Priest, Subtlety Rogue, DK Unholy dual-wield, DK Blood, Frost Mage.
- Buffs only, no nerfs. The reference top is the ranking's top as is (the ranked BM uses a Wolf with BM pet
  talents, `BMDefaultOptions` in `ui/hunter/presets.ts`; the user's real BM runs a Worm and does less).
- Server buffs cap at A: a buffed build stays < 95% in every counted cell; a cell already ≥ 95% before the buff
  may not rise beyond ±0.3%. Design target 86–94%, as margin for gear changes.
- Rotation, talent, glyph, spec-option and bot consumable fixes are exempt from the cap and may change the top build.
- When nothing fits both limits, report the closest result; don't force it.

**Playerbot rules: one switch, used by every tool**
- A per-player sim setting (`Player.playerbot`, bool), implemented once in the sim core. Off by default on the sim
  pages (a "Playerbot" checkbox); presetgen, statweights' capture and the rankings always turn it on. Goldens
  don't move (default off).
- With it on:
  - No pre-pot. One DPS potion per fight, by role (casters Potion of Wild Magic; physical DPS, Shadow, Balance
    and Affliction Potion of Speed; never Indestructible), drunk through an injected top-priority action once the
    fight is 5 s old.
  - Prepull stripped to persistent states bots set up out of combat (stances, presences, aspects; Horn of Winter
    or Bone Shield only if [bots] casts them out of combat). Gone: potions, cooldowns (Army of the Dead,
    Recklessness, Death Wish, Berserk, Mirror Image), precasts, Bloodrage/Berserker Rage, Life Tap stacking,
    trinket pre-procs, totems, Call of the Elements, pre-proc auras (Clearcasting, Overkill).
  - No explosives (Thermal Sapper, Saronite Bomb, all others), no conjured items (Dark Rune; Thistle Tea and Flame
    Cap once confirmed absent from [bots]), no pet food.
  - Flask and food per spec from the `worldbuff` entries of [ac]'s `Playerbot.env` override (on for non-random
    bots; the core raiders are the user's own characters run as bots).
  - Focus Magic's returned crit at 0.
- Not modelled: the burst gate (bots hold cooldowns, trinkets, racials and the tinker until a tank has held the
  boss 5 s; Bloodlust 3 s). Revisit only if P2 shows the sim clearly ahead for that reason.
- Kept: all 11 professions. The server lets every character know every profession, and bots get the passive
  bonuses even unmaxed; Hyperspeed Accelerators stays (bots press it with trinkets).
- The rankings scenario keeps its buff standardization; single-target gifts stay off. The page gets one,
  bot-realistic ranking; README, scenario test and page footer say so.

**Tweak design**
- Sources, in order: WotLK Classic's own class tuning (as the module did for Ret and Omen of Clarity); the module's
  existing patterns (DoT/pet haste and crit scaling behind a talent); base values of existing spells and talents.
  No new spells or mechanics.
- Gear-shaped levers for phase-shaped gaps (flat amounts, base points, proc chance that help more at low gear);
  % multipliers only where a build's gap is flat across phases.
- Gate each buff behind a talent only the target spec takes. Spillover: excluded builds may gain but stay < 95%;
  non-targeted in-scope builds stay within their band.
- Server-only fields (damage, coefficients, proc chance, scaling, aura amounts); stale tooltips accepted.
  Client-side fields (cast time, cooldown, cost, GCD, range) only as a last resort, with a patch MPQ via the
  `client-dbc-edit` skill.
- SQL `spell_dbc` override first, C++ only for dynamic behaviour. Static tweaks get no config switch; reverting
  one is a new `DELETE` SQL file.

**Rotations and bots**
- Fix the sim's APLs (P1) and the playerbots' class strategies (P3), the latter measured by a live bot DPS test
  (P2): the 20 in-scope builds, one 180 s run each in chunks of about 5, the sim's BiS gear for the phase the core
  raid is on now (looked up via SELECT), talents and glyphs from the sim setup; repeats only where a result looks
  off.
- A spec gets bot fixes when its bot/sim DPS ratio is more than 5 points below the median of the 20, or cast
  counts show a clear mistake (a key spell never cast, a DoT/debuff uptime under 80%).
- Regenerate the playerbots' stat weights at the end (P6), with the switch on.

**Process and order**
- Standalone effort `spec-balance` in [sim], not wave-loop items. While P0b, P1 or P5 has edits open in `sim/core`,
  `sim/<class>/` or `ui/<dir>/apls/`, no wave runs items touching those paths; tools- or UI-only waves may run.
- Order: P0a → P0b (Playerbot switch, after Z14) → the wave loop's BiS chain (held until P0b lands), with P1
  alongside → baseline once P1 and the BiS chain are both in → P4 → P5 → P6. P2–P3 run any time after P0a.
- One proposal round (P4) for all builds, reviewed in one sitting; then class-by-class implementation without
  further design check-ins.

**Standing authorizations**
- [sim]: create branch `spec-balance` and worktree [bal]; after each phase's review passes, commit on
  `spec-balance` and `git merge --ff-only` into master. No push.
- [tweaks]: a `feature/balance-<class>` branch per class batch, one commit per tweak, push the branch and open a
  PR to master; show the user the PR text first. The user runs `/code-review` and merges.
- [bots]: commit on `Custom` (its `CLAUDE.md` asks for it). No push, no upstream PRs.
- [simval]: commit, staging only this effort's files (other sessions keep edits open there). No push.
- Commits in any repo need no asking (user, 2026-10-19); pushes and PRs stay as listed.
- Live runs under the server lock (bot test chunks, spelldump recaptures, `.simval` parity probes).
- Still asked each time, batched: image rebuilds, worldserver restarts, `ac-db-import` runs, messages to the
  wave-loop session. Anything not listed needs the user's word. The live DB is SELECT-only.

## In-scope builds and baseline

20 builds. ≥ 85% in every counted cell already: BM Hunter, Unholy 2H, Assassination, Balance. Below B somewhere,
worst and best % of top over the counted cells (rankings at sim `85d65c4c5`, human rules; the switch and the BiS
chain will move these):

| Build | Worst | Best | | Build | Worst | Best |
|---|---|---|---|---|---|---|
| Retribution | 67.8 (P1) | 87.6 (P4) | | Fury | 78.2 (P1) | 98.1 (P5) |
| Arms | 68.2 (P2) | 82.0 (P5) | | Feral | 79.2 (P2) | 94.1 (P4) |
| Demonology | 72.8 (P5) | 80.5 (P1) | | Survival | 79.7 (P2) | 87.9 (P3) |
| Destruction | 73.3 (P5) | 82.7 (P1) | | Fire | 80.2 (P2) | 100 (P5) |
| Enhancement | 74.3 (P4) | 82.2 (P1) | | DK Frost | 80.5 (P2) | 87.5 (P1) |
| Elemental | 75.1 (P5) | 83.1 (P2) | | Shadow | 80.2 (P3) | 91.0 (P4) |
| Combat | 75.8 (P2) | 80.6 (P1) | | Arcane | 82.2 (P2) | 87.5 (P1) |
| Marksmanship | 75.6 (P2) | 91.1 (P4) | | Affliction | 83.2 (P5) | 92.9 (P1) |

Known setup bugs, fixed in P1: all three warlock builds carry Affliction's spec options (Demonology sims with a
Felhunter and Grand Spellstone, `ui/warlock/sim.ts`); Arcane uses Fire's consumes (`ui/mage/sim.ts`).

## Rules that bind every phase

- Each repo's own docs govern its files: [sim] `CLAUDE.md` + its doc map (server-parity, class-mechanics,
  caster-mechanics, testing, spell-data-and-timing, sim-architecture, workstreams, the rankings, presetgen,
  statweights and apldbg READMEs); [tweaks] `CLAUDE.md` and the `spell-tweak` skill; [bots] `AGENTS.md`/`CLAUDE.md`
  (no builds unless asked; `apps/codestyle/codestyle-cpp.py`); [simval] `README.md`.
- Never touch [int] or a running wave's owned paths; check `[int]/docs/wave-loop/wave-loop.PLAN.md` "Current wave".
- Live server lock: `mkdir G:\DevStuff\GitHub\.wave-loop\locks\server` with an `owner` file before any live run; if
  it exists, wait and report the holder; remove it at the end, also on failure; clean test accounts
  (`TestCleanupOrphans`).
- `/compact-docs-writer` before editing any [sim] guide page, README, `CONTEXT.md`, `workstreams.md` or this PLAN.
- Go, make, protoc and npx run in the toolchain container (`tools/acore/dock.sh`), never on the host.
- Never print credential values from `Playerbot.env` or any other override file.

## P0a: Effort setup

1. Branch `spec-balance` off [sim] master, worktree [bal]. Copy this plan to
   `[bal]/docs/spec-balance/spec-balance.PLAN.md`. Add a `workstreams.md` row: owns `docs/spec-balance/`,
   `tools/rankings/balance_check.py`, the Playerbot switch's core files; carve-outs per phase (P0b: `gen.py`'s
   `apply_settings`, `tools/rankings/scenario.go`; P1: the in-scope builds' `ui/<dir>/apls/`, presets and
   `sim.ts` defaults; P5: the touched `sim/<class>/` files and tests).
2. Ask the user to tell the wave-loop session (or, with the user's OK, SendMessage it): the BiS chain waits for
   P0b on master.
3. `tools/rankings/balance_check.py` checks a `rankings.json` against the targets above; its flags and run command
   are in `tools/rankings/README.md`.

## P0b: Playerbot switch

- **Proto:** `playerbot` on the `Player` message (next free field number); regenerate protos in the container.
- **Core**, applied while the character is built, only with the switch on:
  - consumes: clear `PrepopPotion`; `DefaultPotion` by role (caster → Wild Magic, melee and hunter → Speed);
    `ThermalSapper`, `ExplosiveDecoy` off, `FillerExplosive` unknown (`sim/core/consumes.go`); `DefaultConjured`
    unknown; `PetFood` unknown; flask and food from the bot table;
  - prepull: drop every `APLPrepullAction` not on the persistent-state allowlist (registration in
    `sim/core/apl.go`); skip the automatic -1 s pre-pot apl.go adds when `PrepopPotion` is set; check each
    allowlist spell against [bots]' non-combat strategies;
  - potion: inject a top-priority APL action that drinks the main potion once `CurrentTime >= 5s`; the potion's
    cooldown outlasts the fight, so later potion entries never fire (no `Player` falls back to a pre-pot spell);
  - mage `FocusMagicPercentUptime` → 0 (`sim/mage/focus_magic.go`).
- **Bot flask/food table:** generated from `Playerbot.env`'s `worldbuff` entries (class, spec → flask and food
  spell), next to `tools/acore/gen_server_defaults`; a test that every in-scope spec has an entry; rerun when the
  env changes.
- **Tools:** `gen.py` `apply_settings` sets `player.playerbot = true` next to Heroic Presence (append only;
  bis-optimizer owns the file). `scenario.go` sets it and drops `fixPotions`' pre-pot fill and Indestructible swap;
  update `TestStandardizePotions`; a scenario change reruns every result (`scenario_sha256`).
- **UI:** a "Playerbot" checkbox in the player's settings, default off; a uitest for it.
- **Tests:** a core table test per rule (on vs off), Elemental gets exactly one potion at ≥ 5 s, Feral and
  Frost DK lose their prepull cooldowns; goldens unchanged.
- **Docs:** a new guide page `docs/guide/playerbot-rules.md` (what the switch does, where each rule lives) with a
  doc map row; presetgen, statweights and rankings READMEs (the switch is on); `CONTEXT.md` term "Playerbot
  switch"; a `workstreams.md` dependency line: presetgen, statweights and the rankings all turn the switch on.

## P1: Sim rotation, talent and glyph audit (after P0b, alongside the BiS chain)

All measurements with the switch on. For each below-B build, worst gap first:
1. `tools/apldbg -request` on the build's captured settings at P1 and P5 gear, 180 s: casts, uptimes, `-timeline`,
   `WARN` lines. Look for idle GCDs, capped resources, wrong priorities against accepted WotLK play, cooldowns that
   lost their prepull slot and never fire in combat, execute handling, and module tweaks the rotation doesn't
   exploit (Hunter's Mark off GCD; Rupture, Rend, Deadly Poison, disease and Shadow DoT haste; Curse of Doom
   scaling; Arcane Blast cost).
2. Fix the known setup bugs (above). Review the default talent preset, glyphs and spec options.
3. Fix clear mistakes in `ui/<dir>/apls/*.apl.json`, presets, `sim.ts` defaults or APL value code. A change must
   win at P1 and P5 gear (apldbg before/after).
4. Bot consumables: with the switch on, sim each build's flask (Endless Rage, Frost Wyrm, Pure Mojo, battle +
   guardian elixir pairs), food and potion (Speed, Wild Magic) candidates. A pick that beats the bot's beyond noise
   becomes a proposal: flask and food as its row in `Playerbot.env`'s `AC_PLAYERBOTS_WORLD_BUFF_MATRIX` (config
   change: ask; DPS rows only; rerun `gen_server_defaults`), a per-spec potion as a [bots] `PlayerbotFactory` change
   on `Custom` plus the sim's potion rule.
5. Goldens a default APL feeds: promote suite by suite (`docs/guide/testing.md`), never `make update-tests`.
6. Write `docs/spec-balance/spec-balance.INVESTIGATION.md`: per build, what was wrong, the fix, the delta per phase.

Baseline (once P1 and the BiS chain are both on master): rebuild the dev sim container from [bal] (:3334), capture
(~50 min), runner (~5 min), merge; `balance_check.py`; record the table in the PLAN and the remaining gap per build
in the INVESTIGATION. The rankings page footer then says the ranking follows playerbot rules.

Phase loop per [sim] `docs/guide/workflow.md`: implement + verify in a subagent, fresh reviewer, land.

## P2: Live bot DPS test ([simval] e2e; any time after P0a)

1. Spike: a harness character can be put under playerbot AI control and made to attack the Naxx boss dummy
   (e.g. `.playerbots bot add` from the harness GM, group, attack). If not feasible, stop and report.
2. `TestBotDps`, built on `TestRecordedRun` (Naxx dummy, Chronicle log, `.ip set 18`): class, race, talents
   (`SIMVAL_RECORD_TALENT_SPELLS`), glyphs and BiS gear (`.simval equip`) from the build's sim setup at the chosen
   phase; raid buffs as far as the harness can apply them; 180 s; the bot's own strategies cast. Writes the
   Chronicle log + `.setup.txt`.
3. `[sim] tools/simval botdps compare` reads the bot's DPS and casts; the sim runs the same setup with the switch on
   and the build's current APL. Report per spec: bot/sim DPS ratio, cast-count and uptime differences.
4. README entry in [simval] under "Live e2e tests".

Carve-outs from e2e-coverage's trees: [sim] `tools/simval/botdps*.go`, its README's `## botdps`, `fill` and
`glyphItemSpells` in `goldchar.go`; [simval] `e2e/botdps_test.go`, its `run.sh` lines and README paragraph,
`goldcharBuild`'s `learnAll`, `records_test.go`'s `holdRecordDrops`.

## P3: Playerbot rotation fixes ([bots], after P2)

For each spec meeting a trigger:
1. Read `docs/classes/<class>.md`, `docs/systems/consumables-and-burst.md` and `src/Ai/Class/<Class>/`. Compare
   priorities and cast conditions with the corrected sim APL and P2's cast-count differences.
2. Fix clear mismatches within [bots]' per-tick cost rules. Verify: `codestyle-cpp.py`, `clang-format --dry-run
   --Werror`, creator-table greps, `~/.claude/scripts/pb-syntax-check.sh`. Commit on `Custom`.
3. Deploy needs an image rebuild (ask). Re-measure with P2 afterwards.

## P4: Tweak design and the proposal round

For each build still below B at the baseline:
1. Find what it lacks (scaling its peers got, weak base values, procs that fade with gear) via apldbg damage
   breakdowns at P1 and P5, and WotLK Classic's changes for the class.
2. Prototype 1–3 candidates as uncommitted edits in [bal] (sim values set directly; a C++ tweak as its future
   switch), run the runner for that build only (`--builds`), all phases at 180/300, and check spillover builds
   sharing the spell. Undo rejected prototypes by hand before P5 lands anything.
3. One proposal doc, `docs/spec-balance/spec-balance.PROPOSAL.md`: per build, the tweak, its gating talent, spell
   ids and changed fields, predicted % per counted cell, cap and spillover check. The user reviews it in one
   sitting; only approved tweaks go to P5.

## P5: Implement approved tweaks (one class batch at a time)

- **Server ([tweaks], `spell-tweak` skill):** SQL via
  `python tools/dbc_export.py --generate-override <id> --set <Col>=<Val> --out data/sql/db-world/base/<class>/spell_<id>_<slug>.sql`;
  C++ in `src/SpellTweaks_classes.cpp` only for dynamic behaviour, with a switch in `conf/spell_tweaks.conf.dist`,
  default on in code (the live `spell_tweaks.conf` lacks newer keys). Write-up in `docs/<class>/<Feature>.md`.
- **Apply (ask, batched):** `docker compose up -d --force-recreate ac-db-import`, then `docker compose restart
  ac-worldserver`; C++ needs an image rebuild. Check the `updates` table.
- **Sim, SQL tweaks:** the sim takes spell and talent values from the live server's spelldump, not `spell_dbc`
  (`docs/guide/server-parity.md`). After the server applies the SQL: recapture (`TestSpelldumpCapture`, the sim's
  `tools/acore/spellids` README), regenerate the changed ids with `gen_serverdata`, update class code that holds
  the value, tests.
- **Sim, C++ tweaks:** the switch in `SpellTweaksSettings` (`proto/common.proto`, next free field), read in
  `newSpellTweaks`, plus `tools/acore/gen_server_defaults/settings.go` and
  `ui/core/components/server_settings_picker.ts`; the mechanic in `sim/<class>/`; tests with the switch on/off and
  `SpellTweaks.Enable` off (`TestSpellTweaksEnableGatesScriptedTweaks`); a `.simval damage`/`timing` probe
  compared by `tools/simval`.
- Goldens promoted suite by suite. Runner + merge, `balance_check.py` with every build the batch changed, spillover included, as `--buffed`.
- Land: [sim] per the standing OK; [tweaks] branch, commits, push, PR (text shown first).

## P6: Final check

Full rankings rerun and `balance_check.py` on all 20 builds; status and investigation updated; durable learnings
promoted to [sim] guide pages and [tweaks] docs. Regenerate the playerbots' stat weights (`tools/statweights`
README, then [bots] `apps/simweights/generate_weights.py`), switch on. If the BiS presets are re-optimized again
later, rerun the rankings and fine-tune any build that left 85–95%.

## Verification

- `balance_check.py` exits 0 on the final `rankings.json`.
- Playerbot switch: core tests per rule; goldens unchanged with it off; one build captured through `gen.py` shows
  `playerbot: true` in its saved settings; a rankings `link` opened in its page has the checkbox ticked and
  re-sims within noise of the ranked value.
- Per tweak: sim unit tests and goldens in the container; the server's `updates` row; the sim matches the server
  (spelldump-generated values for SQL tweaks, `tools/simval` probe for C++ tweaks).
- Per rotation fix: apldbg before/after at P1 and P5 gear.
- Bot test: a control spec run twice agrees within its stdev; every bot fix re-measured after deployment.

## Out of scope

60 s fights as a target, AoE/cleave and movement fights, tanks and healers, nerfs, the five excluded builds,
per-phase talent sets, the burst gate, the raid importer marking which raiders are bots, upstream mod-playerbots
PRs.
