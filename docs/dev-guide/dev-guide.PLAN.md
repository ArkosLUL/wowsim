# Living dev guide for the AzerothCore wowsim fork

**Status:** Done. The guide is applied, and the fresh-agent test's findings are fixed in the guide.
Findings about other efforts' PLANs are left to those efforts.

## Context
Nine Claude Code sessions (2026-09-17/18, transcripts in
`C:\Users\boss2\.claude\projects\g--DevStuff-GitHub-wowsimwotlk\*.jsonl`) adapted this wowsims/wotlk
fork to the user's AzerothCore 3.3.5a server. The workstreams were: the Docker build fix (`ddae4f9d2`),
the item diff (`176e93f3b`), raid import phases 1-2 (`23d0796b5`, `7155b74c2`), and AzerothCore parity
(P1 in the [ac] module repo; P0 and docs on branch `azerothcore-parity`; P2 and P4 uncommitted in worktree
`G:\DevStuff\GitHub\wowsimwotlk-parity`). Several separate review sessions ran alongside them.

What those sessions learned lives only in transcripts: the commands that work on this Windows host,
server/DB/DBC facts, settled decisions, pitfalls and user corrections. The repo has no CLAUDE.md/AGENTS.md.
Every session so far rediscovered things like "Go isn't on the host", "never `make update-tests`" and
"read both spell tables for profession bonuses".

Goal: a repo-committed, agent-facing guide that future sessions load automatically, keep updated as they
work, and that can grow without restructuring.

User decisions:
- Repo docs plus a root `CLAUDE.md`.
- The reader is future agent sessions, so write terse and rule-like.
- The structure must be robust from day one.

## File structure

```
CLAUDE.md                  always loaded: purpose, hard rules, doc map, placement + upkeep rules
CONTEXT.md                 glossary only (domain-modeling CONTEXT-FORMAT), no implementation detail
docs/
  guide/<topic>.md         living knowledge base, one topic per file, kebab-case
  adr/NNNN-slug.md         decisions that are hard to reverse, surprising, and a real trade-off
  <slug>/<slug>.<TYPE>.md  per-task PLAN / INVESTIGATION docs (existing convention, unchanged)
tools/**/README.md         how to run a tool lives next to the tool; the guide links to it
```

Why this scales:
- **One home per kind of fact.** The placement rules in CLAUDE.md give every new learning exactly one
  place, so nothing is duplicated.
- **Growth means a new file plus one map line.** Split a page when it covers two topics or passes about
  200 lines.
- **Pages load on demand.** Only CLAUDE.md is always in context. The map's "read when…" triggers pull in
  pages, so the per-session token cost stays flat as the guide grows.
- **Parallel sessions rarely conflict.** Topic files are small, ADRs are separate files, and volatile
  status stays in each task's own PLAN doc, so there's no shared status table.
- **Plans are history, the guide is current truth.** When a task finishes, its durable learnings are
  promoted into the guide, ADRs or CONTEXT. The PLAN and INVESTIGATION docs stay as the record.

## Deliverables

All files are new and written in the main worktree `G:\DevStuff\GitHub\wowsimwotlk` (master). Nothing
goes in the parity worktree: another session is editing it. Cite code by path, not line number, and
don't paste code. Facts that only exist on an unmerged branch are tagged `(azerothcore-parity only)`.

### 1. `CLAUDE.md`: always loaded, target under 700 words
- **Purpose:** this fork models the user's AzerothCore 3.3.5a server exactly, not WotLK Classic.
  - It's a permanent fork. `origin` is ArkosLUL/wowsim; `upstream` is wowsims/wotlk, kept for reference
    only, with no pulls.
  - Paths: [sim] is this repo, [par] is `G:\DevStuff\GitHub\wowsimwotlk-parity`, and [ac] is
    `G:\DevStuff\GitHub\azerothcore-wotlk-pb`, the live server.
- **Hard rules.** Project-specific only; don't restate the user's global `~/.claude/rules`.
  - Go isn't on the host. Run go, protoc, npx and make in the toolchain container
    (see `dev-environment.md`).
  - Never `make update-tests`: it deletes every `.results` file. Promote suite by suite.
  - Server work needs the user's OK each time for any rebuild, restart or config change. The live DB is
    SELECT-only.
  - One worktree per effort. Don't touch another effort's worktree or owned paths, and assume
    concurrent sessions. Check `git branch --show-current` before committing.
  - Phase loop:
    1. Implement, verify, then stop.
    2. A separate session runs `/code-review` and fixes the findings.
    3. On "commit and merge": commit on the feature branch, then `git merge --ff-only` into master.
    4. Push only when asked.
  - Keep useful verification tooling in the repo, next to the code it tests, with a run command.
    This moves the auto-memory `keep-useful-test-tooling` in.
  - Never read `github-recovery-codes.txt` or `Important data.txt`, and never copy credential values out
    of configs.
- **Doc map:** a table with one row per guide page, CONTEXT.md, `docs/adr/` and the tool READMEs, each
  with a "read when…" trigger.
- **Placement rules:**

  | Kind of fact | Where it goes |
  |---|---|
  | Term | CONTEXT.md |
  | Decision that meets the ADR bar | `docs/adr` (next number) |
  | Durable fact, command or gotcha | the matching guide page, next to its topic (no pitfalls page) |
  | How to run a tool | that tool's README |
  | Task plan or status | `docs/<slug>/` |
  | Volatile status | never in the guide |

- **Upkeep (what makes it live):**
  - Before starting, read the pages the map points to.
  - In the same change as the work, fix any guide fact found wrong.
  - Also record any new durable fact or command, and any user correction about project conventions.
  - When a task or phase completes, promote its PLAN learnings and update `workstreams.md`.
  - New topic: add a page and a map row.

### 2. `CONTEXT.md`: glossary, about 20 terms
Define and pick one canonical name, with `_Avoid_` lists. Terms:
- **Sources of truth:** the server (AC); Classic; parity; server-exact; retail deviation.
- **Effort and phases:** effort (Avoid: workstream, session); phase (P0–P8, IDs aren't execution order).
- **Testing:** golden (`.results` baseline); promote; simval; spelldump; recorded run.
- **Character import:** roster export; raider; human-played vs playerbot character.
- **Server modules:** reforge; racial traits (vs race); shared professions; dungeon scale;
  server settings.

### 3. `docs/guide/` pages

**`dev-environment.md`: host, toolchain container, build, run**
- **Host:**
  - Windows 11 with Git Bash and PowerShell. Go, make and the mysql CLI are absent. Python 3.14 is
    present.
  - Node is present, but `node_modules` was installed from Linux, so run npx, tsc and eslint in the
    container.
  - `core.autocrlf=true`, so files are CRLF.
- **Versions:** from `go.mod` (1.23), the Dockerfile (`golang:1.23-bookworm`, `node:19.8.1`) and
  `.nvmrc`. The README versions are stale.
- **Toolchain image:** `docker build --target toolchain --tag wowsims-wotlk-dev .`
  - The Git Bash template: `MSYS_NO_PATHCONV=1 docker run --rm -v G:/…:/wotlk -v wotlk-gomod:/go/pkg/mod
    -v wotlk-gocache:/root/.cache/go-build -w /wotlk wowsims-wotlk-dev sh -c '…'`, plus a PowerShell
    variant.
  - Join `--network azerothcore-wotlk-pb_ac-network` to reach `ac-database:3306`.
  - `tools/acore/dock.sh` (azerothcore-parity only) wraps this, with subcommands
    `build|proto|tsc|test|exec|run|dps|delta|promote`. `exec` needs `bash -c` for compound commands.
  - Duplicate images and volumes exist (`wotlk-toolchain`, `wowsim-toolchain`, `wowsim-gomod`). Name the
    canonical ones.
- **Protos:** the generated Go and TS files are gitignored.
  - After a fresh clone or any `.proto` change, run the `make proto` lines in the container (list them).
    Host `npx protoc` fails.
- **Builds:**
  - `make wowsimwotlk` does the full build. `make devserver` doesn't build wasm and vite; use
    `make dist/wotlk/.dirstamp` for that.
  - `go build` of `gen_db` drops a binary in the repo root, so use `-o /dev/null`.
  - `go mod tidy` fails until `binary_dist` is generated.
- **Running:**
  - Prod container `wowsims-wotlk` on :3333 (`/wotlk/`) is distroless and has no shell. Its image is
    stale until rebuilt.
  - A dev server container runs on :3334. The command, `docker stop`/`rm`, and restart after changes.
  - API smoke test with protoc encode/decode. The minimal player fields are required, and the server is
    built without `with_db`.
- **Shell gotchas:**
  - Bash heredocs fail intermittently, so write scripts to files.
  - `$null` redirected in Git Bash creates a literal file.
  - `grep` under MSYS strips CR, so use `grep -U` or `od -c`.
  - Host Python can't see Git Bash's `/tmp`.
  - `bc` is missing.
  - Under `pipefail`, `tr </dev/urandom | head` exits 141.

**`testing.md`: verification**
- **Go tests:** `go test --tags=with_db $(go list ./sim/... | grep -v /sim/web$) ./tools/...`
  - `sim/web` needs `binary_dist`.
  - Use `-count=1` when results come back cached.
  - Also `go vet`, and gofmt through `tr -d '\r'`. `sim/warrior/rend.go` already fails gofmt.
- **Goldens:**
  - 37 `.results` files; test runs write `.results.tmp` next to them.
  - Never `make update-tests`. Review the per-suite delta, then promote one directory at a time.
  - `delta` only compares DPS, so diff the files for stats and weights.
  - CRLF-only `.results` changes are noise; leave them out of commits.
  - Parallel sessions in one worktree collide on `.tmp` files.
- **TS:** run `npx tsc --noEmit` in the container.
  - eslint already fails on master (`bulk_tab.ts`, `equipped_item.ts`, `importers.ts`). Compare per file
    against HEAD with the `git show HEAD:$f | npx eslint --stdin` recipe.
  - Merge new imports into existing lines to avoid `import/no-duplicates`.
- **Float asserts:** compare with a tolerance.
- **Reviewers must build:** "Go not installed" missed a real compile error once.
- **Server-truth checks:**
  - `tools/simval` replays `sim/core/testdata/simval/simval.jsonl` (azerothcore-parity only).
  - Live e2e: `[ac]/modules/mod-sim-validation/e2e/run.sh` needs the user's OK. It covers fixture
    capture via `docker exec … tail -F` and orphan cleanup.
  - `acraid/crosscheck.py` plus `crosscheck_test.py`.
- **Definition of done for parity:** ±1 bp on computed chances, |z| ≤ 5 on rolled rates, ±0.1% on
  multipliers, ±2% DPS on recorded runs.

**`sim-architecture.md`: code map for this fork**
- **`sim/core` files and their roles:**
  - `target.go`, `spell_outcome.go`, `spell_result.go`, `spell_resistances.go`, `flags.go`,
    `constants.go`, `unit.go`, `avoid_dr.go`.
  - `base_stats*.go`, with the generator changing in P4.
  - `character.go`, `mana.go`, `racials.go`, `debuffs.go`, `buffs.go`, `attack.go`, `rage.go`,
    `dot.go`, `database.go`, `test_suite.go`.
  - `combat_table.go` (azerothcore-parity only).
- **Class code:** `sim/<class>`, including the `/1.3` hacks and hunter ×1.15.
  - Item effects: `sim/common/{wotlk,tbc}`.
  - Encounters: `sim/encounters/*`.
  - `sim/web` is the server binary.
- **Protos:**
  - `proto/api.proto`, `common.proto`, `ui.proto`.
  - Fork fields: `ItemSpec.reforge`, `Player.racial_traits = 47`, `Player.professions = 48`,
    `SavedSettings.racial_traits = 19`. `profession1/2` are kept for old links.
- **UI files:**
  - `ui/core/player.ts`, `encounter.ts`, `constants/mechanics.ts`.
  - `components/gear_picker.tsx`, `individual_sim_ui/settings_tab.ts` (the racial traits and
    professions pickers live here), `proto_utils/reforging.ts`.
  - `ui/<spec>/presets.ts`.
- **Fork features in the sim:**
  - Reforge rules: `sim/core/reforging.go`, mirrored in TS. `Item.TotalStats()` includes socket bonuses.
  - `BaseStatsRace` vs `RacialTraits`.
  - The professions slice.
- **Tools:**
  - `tools/database/gen_db`: the Classic item DB pipeline; there's no AC mode yet.
  - `tools/database/azerothcore`: the shared MySQL and DBC readers, owned by the item-diff effort.
  - `acdiff`, `acraid`, and `tools/acore` (parity).

**`azerothcore-server.md`: infrastructure and access**
- **[ac] is live; the other AC checkout isn't.** `azerothcore-wotlk-pb` on branch `Custom` is the live
  server. `G:\DevStuff\GitHub\azerothcore-wotlk` isn't live.
- **Containers:** `ac-database` (mysql 8.4, :3306, stock root creds, DBs acore_world, acore_characters
  and acore_auth), `ac-worldserver`, `ac-authserver`.
  - Network: `azerothcore-wotlk-pb_ac-network`.
  - Client-data volume: `azerothcore-wotlk-pb_ac-client-data`, mounted read-only as `/acdata/dbc`.
- **DBCs:** Clean, Changed and live paths; the gt DBCs are identical across the three.
- **Queries:** `docker exec ac-database mysql …`.
- **Config:** `[ac]/configurationOverrides/*.env`, where `AC_*` overrides conf keys, and
  `env/dist/etc/**/*.conf`.
- **Saving characters:** SOAP is off, so the user must run `saveall` before an export.
- **Rebuild and e2e:** commands, each needing the user's OK.
- **[ac] AGENTS.md rules:** don't build unless asked; SQL only in `pending_db_*`; DELETE before INSERT.
- **Modules and their sim relevance:**
  - playerbots, individual-progression (applied base SQL, not `optional/`), reforging,
    racial-trait-swap, shared-professions, dungeon-scale, spell-tweaks, chronicle, sim-validation,
    and others.
- **Server-dev gotchas:**
  - `PSendSysMessage` prints nothing on the console.
  - Relog after `.maxskill`.
  - Character deletes are async.
  - AzerothGhost needs the Warden patch and `E2E_WORLD_ADDR`.

**`azerothcore-data.md`: DB and DBC facts for parsing**
- **`item_template`:**
  - Use COALESCE on nullable columns.
  - Armor is the total, with `ArmorDamageModifier` as the bonus part.
  - `block` is shields only.
  - `VerifiedBuild` 15595 marks placeholder rows.
  - Also has the `spellppmRate`/`spellcooldown` columns.
- **Spell tables:** `spell_proc` (a negative id applies to the whole chain), `spell_dbc` and the
  `*_dbc` tables (they override DBC files), `spell_cooldown_overrides`, `spell_enchant_proc_data`.
  - Don't use `SpellDB*`.
- **Obtainability sources:** loot, vendors, quests and achievement rewards.
- **Characters:**
  - `character_inventory` (bag 0, slot < 19, no shirt or tabard).
  - `item_instance.enchantments`: 36 tokens (0 enchant; 6, 9, 12 gems; 18 socket-adding, never
    emitted). Gem enchant maps to gem item through SpellItemEnchantment field 33.
  - `randomPropertyId` sign.
- **Talents:** `character_talent` with specMask, mapped through the Talent and TalentTab DBC files
  (field indices).
  - Use the DBC files, not `talent_dbc`, which holds the hunter tier swap.
  - Only the active spec is exported.
- **Glyphs:** mapped through GlyphProperties.
- **Groups:** `groups` and `group_member` (flags, subgroup).
- **Professions:**
  - `character_skills` with the primary skill ids.
  - Profession spells come from both `character_spell` and `shared_professions_account_spells`.
  - Skill value doesn't show bonus ownership: GM `.learn` grants and account-shared spells.
- **Module tables:** `character_reforging` (stored amount is floor 40% of the server stat, 1–2 off the
  sim's), `character_racial_swap`.
- **Spell.dbc:** the field indices used.
- **Mechanics confirmed in AC source:** 10/25-man tiers share an ItemSet; proc-triggered spells never
  start cooldowns; the cooldown and PPM precedence functions.

**`server-parity.md`: Classic vs server catalog (links to ADRs, doesn't restate them)**
- **Mechanics table** (implemented in P2, azerothcore-parity only): crit suppression, glancing, dodge
  and parry, expertise cliffs, table order, yellow block roll, magic resist and miss, creature vs
  player, the ArP cap by victim level, block value 41.
- **Measured and planned:** the P3/P7 items, hunter quiver haste, armor debuff stacking, enchant PPMs.
- **Items:**
  - Only Ulduar-era ilvls differ (+6 normal, +13 hard mode in Classic): 857 obtainable items. Other
    raids match.
  - The tooltip-parser gaps.
  - The Ulduar-tier trinket values and proc-rate rows (count only; details in the item-diff
    INVESTIGATION).
  - Sim bugs: Forethought Talisman, Totem of the Third Wind.
- **Module handling table:**
  - spell-tweaks toggles default to the live config.
  - dungeon-scale: boss HP 1.2 is a setting.
  - individual-progression penalty ignored.
  - Hunter tier swap: stock positions.
  - Reforge and racial-traits models.
  - Every profession can be known at once.
- **Status and full detail:** point to the PLAN and INVESTIGATION docs.

**`workflow.md`: how the user runs efforts**
- **Planning:**
  - Expect `/grill-me` on a first plan. Give every question a recommended answer; the user usually
    replies "go with suggested".
  - Refer to phases by ID.
- **Worktrees:**
  - Branch per effort, named `azerothcore-<slug>`.
  - Create a worktree with `git worktree add ../wowsimwotlk-<slug>` (the user must ask for it), then
    `code --add <path>`.
- **Review loop and "commit and merge":**
  - "Can be merged" alone isn't enough.
  - Before committing, re-verify the reviewer's edits.
  - Merge with `--ff-only`. No push and no branch deletion unless asked.
- **Concurrency:**
  - Another session once switched the shared tree, so a commit landed on master. Check the branch before
    committing.
  - Sessions sharing a worktree coordinate via SendMessage and promote goldens only after both finish.
- **Reviews:** readable findings, as in the global rule. Reviewers build and test in the container.

**`workstreams.md`: effort registry (stable facts only; status lives in each PLAN)**
- One entry per effort: slug, one-line scope, branch/worktree, docs dir, owned code paths, and
  dependencies.
  - `azerothcore-item-diff`
  - `azerothcore-raid-import` (+ `-roster-export`)
  - `azerothcore-parity` (+ `[ac]/modules/mod-sim-validation`)
  - `docker-build`
- **Cross-effort dependencies:**
  - Parity lacks `23d0796b5`. Merge master and reconcile with reforge and racial traits before P5/P6.
  - Parity P6 consumes the item-diff rework (the `gen_db` AC mode).
  - Raid-import Phase 3 (UI importers) needs master's roster JSON v1.

### 4. `docs/adr/`: the four decisions that meet the bar (1–3 sentences each)
- `0001-server-exact-permanent-fork.md`:
  - Copy the server exactly, retail deviations and bugs included, and log each deviation.
  - Replace Classic rules in place: no toggle, no upstream pulls.
  - Single local user, so no share-link migration.
- `0002-item-data-from-live-db.md`: item stats come from the live MySQL through a gen_db AC mode, not an
  override list, since 880 overrides would amount to a second DB. Wowhead and AtlasLoot stay for icons
  and sources, and effects and set bonuses stay in Go.
- `0003-spell-data-split.md`: flags and timing are generated from server data. Damage numbers stay
  hardcoded in Go and are checked against the server by a test.
- `0004-server-config-as-settings.md`: server module config values (dungeon scale, spell-tweaks toggles,
  reforge %) are UI settings that default to the live config, not constants, because the user tunes the
  server.

### 5. Tool READMEs (reference prose)
- **`tools/database/acdiff/README.md`:**
  - Purpose, flags (`-acRepo` is required), and the confirmed docker run command. It's SELECT-only and
    takes about 10 s.
  - Output files. `effects_review.csv` and `sets_review.csv` are hand-kept and not regenerated.
  - Known heuristic gaps.
- **`tools/database/acraid/README.md`:**
  - Purpose, `saveall` first, flags (`-leader`, `-names` top-up, `-minSkill` defaults to 1: don't raise
    it).
  - DBC copy or volume mount, and the confirmed docker run command.
  - The roster JSON v1 shape. It isn't importable in the UI until Phase 3.
  - `crosscheck.py` usage.

### 6. Memory cleanup
Once the rule is in CLAUDE.md, delete `memory/keep-useful-test-tooling.md` and its `MEMORY.md` line, so
the rule isn't duplicated.

## Execution notes
- The user's global rule applies: invoke `/compact-docs-writer` before drafting, then follow its loop.
  1. Draft everything into the scratchpad.
  2. Present CLAUDE.md in full, plus each file's path and measured word count.
  3. Write into the repo only on approval.
- Before stating a fact, verify it against the repo or a read-only command. The session reports are the
  source, and a few conflicted: Node is present on the host; "no upstream pulls" is confirmed in the
  parity PLAN.
- No git actions. At the end, ask whether to commit and on which branch. Mention that [par] only gets
  the guide once master is merged into `azerothcore-parity`.

## Verification
1. **Link and path check:** a scratchpad Python script resolves every relative link and backticked repo
   path in CLAUDE.md, CONTEXT.md, `docs/guide`, `docs/adr` and the tool READMEs against master. Paths
   tagged `(azerothcore-parity only)` are checked in [par] instead.
2. **Command spot-check** (read-only) in the toolchain container:
   - `go version`
   - `go vet ./tools/...`
   - `go test ./tools/database/...`
   - `npx tsc --noEmit`
   - `docker exec ac-database mysql … -e "SELECT 1"`, if the server is up
3. **Fresh-agent test:** an Explore agent given only "read CLAUDE.md first" answers six questions. Pass
   means every answer is correct and each was found by following the map.
   - How do I run Go tests?
   - May I run `make update-tests`?
   - Where do item stats come from?
   - How do I export the raid roster?
   - Where does parity work happen?
   - Who has the Toughness bonus?
4. **Size check:** `wc -w` CLAUDE.md is under 700 words, and every guide page is under 200 lines.
