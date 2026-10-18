# DPS Rankings: PLAN

Effort `dps-rankings`: branch `dps-rankings`, worktree `G:\DevStuff\GitHub\wowsimwotlk-rankings`.

Status: done (P0, P1, P2).

## Context

The sim has one page per spec and a raid sim, but nothing that compares DPS specs to each other. The user
wants a tier list page: every DPS build ranked against the others in each content phase (1 Naxx/EoE/OS,
2 Ulduar, 3 ToC, 4 ICC, 5 Ruby Sanctum), on this fork's server-exact sim.

Almost every input already exists:
- **Builds and BiS gear:** `tools/presetgen/gen.py` `BUILDS` lists 28 builds (23 DPS, 5 with `tank=True`).
  `ui/<dir>/gear_sets/p<N>_bis[_<tree>].gear.json` plus `bis_presets.json` (with each pick's
  `racialTraits`) hold each build's BiS for every content phase.
- **Page-exact settings:** `tools/statweights/capture.py` opens each build's sim page in headless Chrome
  through gen.py's `Driver` and saves the `ComputeStatsRequest` it sends, with the resolved APL rotation, as
  `settings/<build>_p<N>.bin`. An exported settings JSON can't be used instead: it keeps the
  rotation as `TypeAuto` (Fire Mage sims at 0). The UI defaults live only in TS, so this capture is the
  only path.
- **Runner pattern:** `tools/statweights/main.go` loads those files, sims them with
  `core.RunRaidSimAsync`, and writes one `results/<build>_p<N>.json` each. `merge.py` checks them against
  the presets (`stale`) and stamps `sim_commit`.

What's missing: those numbers use each spec page's own buff panel, so they aren't comparable. There's also
no page to show them.

## Decisions (user, 2026-10-18)

**Scope and placement:**
- **Page:** "DPS Rankings" at `/wotlk/rankings/`, a page of its own, linked from the homepage and the sim
  title dropdown.
- **Data:** precomputed offline. `assets/rankings/rankings.json` is committed, and the page fetches it.
- **Builds:** the 23 DPS builds plus BM hunter and Unholy 2H, which get presetgen builds in P0. 25 in all.

**Scenario:**
- **Buffs:** one standard set for every build: all raid buffs and boss debuffs, and no single-target gifts.
  It also includes **Heroic Presence for everyone**: a Draenei in every party.
- **Fights:** a single target at 1, 3 and 5 min (±5 s): one ranking per content phase and length. The
  boss is the same in every phase: the optimizer's plain level 83 boss with 25-man normal scaling.
- **Kept as each page has them:**
  - talents and glyphs, one set for all 5 content phases;
  - spec options;
  - flasks and food.
- **Potions:** a build without a pre-pot gets its main potion as the pre-pot. The warrior's Indestructible
  Potion becomes Potion of Speed.
- **BiS consistency:** presetgen gives every build Heroic Presence while optimizing (P0). The queued BiS
  chain then re-optimizes all 30 builds with it.
- **Unholy 2H** stays on two-handers: a presetgen per-build gear filter turns one-handed weapons off.
- **BM hunter** gets the BM pet talents, and MM's BiS presets show only on tree 1.

**Page:**
- **Layout:** tier bands with sorted bars: class colour, DPS, % of the top build.
- **Tiers:** a fixed % of the top build in that content phase and fight: S ≥ 95, A ≥ 90, B ≥ 85, C ≥ 75,
  D below. Measured against the overall top, so the role filter hides rows but never changes a letter.
- **Extras:** an across-phases chart, a role filter, the DPS spread (±1σ), and a link to the spec's sim.
- **Link:** opens the exact simmed setup in a new tab. A tooltip warns that it replaces that sim's saved
  current settings (`__currentSettings__` in localStorage, `individual_sim_ui.ts`).

**Process:**
- **Order:** P0 → P1 → P2, one at a time.
- **Timing:** P0 lands before the wave loop's deferred BiS chain starts (queued after Z13), so the chain
  reruns 30 builds.
- **Refresh:** the rankings re-run as part of every BiS re-baseline. The page shows its sim commit and
  date.
- **Bots:** mod-playerbots' `SimWeightsMgr::ResolveKey` reads the BM rows (class 3, tab 0), in this
  effort. Verified compile-only; deploying is the user's call.

**Fixed defaults:** 10,000 iterations and seed 424242, as statweights uses. The error on the mean (about
±25 DPS) is far below a tier's width (5% ≈ 500–1000 DPS).

## P0: Heroic Presence in presetgen; BM hunter and Unholy 2H builds

Background:
- Heroic Presence follows **racial traits**: `Character.AddPartyBuffs` turns it on when `RacialTraits` is
  Draenei (`sim/core/character.go`). Today's BiS sets got it only where the optimizer picked Draenei traits,
  or from the feral page's own `heroicPresence: true`. All warlock and balance sets, and most rogue sets,
  went without.
- BM hunter and Unholy 2H have a talent preset and a rotation each (`BeastMasteryTalents` +
  `bm.apl.json`; `Unholy2HTalents` + `uh_2h_ss.apl.json`), but no BiS. BM borrows MM's BiS
  (`talentTrees: [0, 1]` in `ui/hunter/presets.ts`).

Steps, in order:
1. In `gen.py`, where it sets `player.race` and `professions` on the settings it applies (`apply_settings`),
   also turn on party Heroic Presence. statweights' capture goes through the same code, so the bots' weights
   are then measured with it too. Scope `page.js`'s `clickPreset` to the player's talent presets: the
   hunter's pet presets reuse names like "Beast Mastery".
2. Append two entries to `BUILDS`. The fields are `key dir label talents seed tree name tank race`:
   - `hunter_bm`: talents = the BM preset's name; seed `p1_mm.gear.json`; tree `bm`; also clicks the pet's
     "Beast Mastery" talent preset.
   - `dk_unholy_2h`: talents `Unholy 2H`; seed a new `ui/deathknight/gear_sets/p1_uh_2h.gear.json`, which
     is `p1_uh_dw` with a P1 two-hander from the catalog and the off hand empty; tree `unholy_2h`; gear
     filter with one-handers off.
3. Run `python tools/presetgen/gen.py run --out <scratch>/presets --effort Normal --builds hunter_bm,dk_unholy_2h`
   (about 10–20 min, presetgen's own sim server). It writes `p<N>_bis_<tree>.gear.json` and the
   `bis_presets.json` entries. Leave the other 28 builds' presets for the BiS chain.
4. Wire the new BiS presets in `ui/hunter/presets.ts` (BM on tree 0, and narrow MM's to tree 1) and in
   `ui/deathknight/presets.ts` (tree 2, next to the DW ones), plus each `sim.ts` presets list.
5. Gate: every Unholy 2H pick must keep a two-hander. If the optimizer drifts to dual wield, stop and
   report; don't build slot locks without asking.
6. Playerbots: in `[ac]/modules/mod-playerbots/apps/simweights/generate_weights.py` (its own repo; follow
   `[ac]`'s `AGENTS.md`), map `hunter_bm` to the BM tab `(3, 0, "AGILITY")` and add `dk_unholy_2h` to
   `SKIPPED`. An unmapped key fails the whole run, and `dk_unholy_2h` would collide with `dk_unholy` on the
   `(class, tab, phase, stat)` key. Make `SimWeightsMgr::ResolveKey` resolve Beast Mastery. Commit there
   only on the user's OK.
7. Docs:
   - in the presetgen and statweights READMEs: Heroic Presence for every build, and 30 builds;
   - in `workstreams.md`: generate_weights' build map joins the stat-weights dependency line.
   - Don't edit `wave-loop.PLAN.md`: the running orchestrator holds uncommitted edits in [int]. Tell the
     user to add "rerun the DPS rankings" to the BiS chain's follow-ups.

bis-optimizer owns `gen.py` and the presets: P0 appends to them and changes nothing else.

## P1: Data pipeline (`tools/rankings/`)

Flow:
1. `python tools/statweights/capture.py --out tmp/rankings` captures all 30 builds; the tank builds are
   skipped later.
2. `tools/acore/dock.sh run --tags=with_db ./tools/rankings --out tmp/rankings` runs the sims.
3. `python tools/rankings/merge.py --out tmp/rankings` writes `assets/rankings/rankings.json`.

There's no new capture code. A fresh statweights work directory's `settings/` can be copied in instead.

**Runner, `tools/rankings/main.go` (+ `scenario.go`):**
- Flags as statweights (`--out`, `--builds`, `--phases`, `--iterations` 10000, `--seed` 424242), plus
  `--fights` (default `60,180,300`). Skip tank specs.
- Load each `.bin` like statweights' `loadSettings` (package main there, so copy its ~15 lines).
- `scenario.go` applies the standard scenario to the loaded `IndividualSimSettings`:
  - Replace `Player.Buffs`, `PartyBuffs`, `RaidBuffs` and `Debuffs` with explicit literals. Start from
    `core.FullIndividualBuffs`/`FullPartyBuffs`/`FullRaidBuffs`/`FullDebuffs` in `sim/core/test_utils.go`
    (copied, not referenced: the goldens own those), and add `PartyBuffs.HeroicPresence = true`.
  - Replacing the whole `IndividualBuffs` message clears the gifts (Focus Magic, Tricks, Power Infusion,
    Hysteria, Innervate, …). Drop any entry that only boosts its caster, if the sim models one as raid-wide.
  - Fix the potions: an unset pre-pot becomes the main potion, and Indestructible becomes Speed. Flasks and
    food stay.
  - Clear `Tanks`.
  - Build the encounter as `optimizerEncounter` in `ui/core/optimizer/pool_builder.ts` does: one
    `plainBossTarget()` (level 83 Giant, armor 10643, AP 805), execute 0.2/0.25/0.35, variation 5 s,
    `duration` per fight, and the captured `raidDifficulty` and `serverSettings` kept.
- Per build × content phase, sim each fight. Record the player's `UnitMetrics.Dps`: avg, stdev, min, max.
  It includes pets through `AddFinalPetMetrics` (`sim/core/metrics_aggregator.go`); confirm the stdev does
  too.
- Result `results/<build>_p<N>.json`:
  - `build`, `phase`, `class`, `spec`, `talents` (the talents string, for the spec icon), `items`,
    `settings_sha256`, `seed`, `iterations`;
  - `fights`: `{"60": {dps, stdev, min, max}, …}`;
  - `link`: base64 of the zlib-deflated `IndividualSimSettings`, with the standard scenario at 180 s and
    `settings.phase` = the content phase. That's what `IndividualLinkImporter.tryParseUrlLocation`
    (`ui/core/components/importers.ts`) reads; `pako.inflate` takes zlib.
- Rerun a result only when its settings hash, seed, iterations or fights changed, as statweights does.
  Write each file atomically (tmp + rename).

**Merge, `tools/rankings/merge.py`:**
- Import `gen` (`BUILDS`) and statweights' `merge` (`stale`, `sim_commit`), as statweights imports `gen`.
- Skip `tank` builds. Fail on missing or stale results, or on results that mix seeds, iterations or fights.
- Write `assets/rankings/rankings.json` atomically:
  - `sim_commit`, `generated_at`, `catalog_date` (the one value across the used `bis_presets.json`
    entries; fail if they differ), `iterations`, `seed`, `fights` [60, 180, 300];
  - `builds[]`: `key`, `name` (gen's `label`), `class`, `spec`, `dir`, `talents`, and `phases[]` with
    `{phase, fights{…}, link}`.
- Expect about 250 KB, mostly links.
- P1 commits a first JSON from the current presets. The BiS chain's rerun replaces it.

**Tests (`tools/rankings/*_test.go`):**
- The scenario replaces every buff message (Heroic Presence on), clears gifts and tanks, fixes the
  potions, and sets the encounter per fight.
- A link round-trips: inflate + unmarshal gives the standardized settings.
- A fixture `.bin` runs end to end at a few iterations into a result with all three fights.

## P2: The page (`ui/rankings/`)

**Scaffolding:**
- Add `ui/rankings/index.html` to `HTML_INDECIES` in `makefile`. The page is generated from
  `ui/index_template.html` (gitignored output), which loads `../scss/sims/rankings/index.scss`,
  `../index.ts` and `./index.ts`.
- The template's title rule would say "Rankings Simulator". Make the page's title "WOTLK DPS Rankings".
- Model it on `ui/detailed_results/index.ts`, a non-sim page: no `SimUI`, mounts on `document.body`.
- Add a "DPS Rankings" `sim-link` in `ui/index.html` next to "Full Raid Sim", and an entry in
  `ui/core/components/sim_title_dropdown.ts`.

**Modules:**
- `data.ts`: types and a cached fetch of `/wotlk/assets/rankings/rankings.json`, like `CATALOG_URL` in
  `ui/core/optimizer/catalog.ts`. Its link builder inflates a build's `link`, sets the encounter's
  duration to the selected fight, re-deflates, and returns `/wotlk/<dir>/#<b64>`. It uses `pako` and
  `IndividualSimSettings`, as `exporters.tsx` does.
- `tiers.ts` (pure): % of the top build per content phase and fight, tier letter, sort, and the role per
  spec:
  - melee: DK, warrior, rogue, ret, enhancement, feral;
  - ranged physical: hunter;
  - caster: mage, warlock, shadow, smite, balance, elemental.
- `rankings_page.tsx`:
  - Controls: content phase tabs labelled as `PHASE_LABELS` (`optimizer_tab.ts`), fight 1/3/5 min, and
    role toggles. Import `PHASE_LABELS` only if that doesn't pull the optimizer tab into the bundle;
    otherwise move it to `ui/core/constants/other.ts`, re-exported from the tab.
  - Tier bands (S…D), each a list of sorted rows. A row shows: spec icon (`getTalentTreeIcon`), build name,
    a bar in `classColors`, a ±1σ whisker, DPS, % of top, and a link icon (new tab, with the warning
    tooltip).
  - A row tooltip with avg, stdev, min and max.
  - A footer with `sim_commit`, catalog date, iterations, the fight, and the buff set in one line (full
    raid buffs and debuffs, a Draenei in every party, no single-target gifts).
  - JSX uses the `element` factory, like `detailed_results/*.tsx`.
- `phase_chart.ts`: a Chart.js line chart (registered globally in `ui/index.ts`) below the tiers.
  - x is content phase 1–5, y is DPS for the selected fight, with a toggle to rank. One line per build:
    class colour, with dash patterns telling a class's builds apart.
  - The role filter applies, and hovering highlights a build.
- State `?phase=&fight=&role=` in the URL. Defaults: `CURRENT_PHASE` (5), 180 s, all roles.
- `ui/scss/sims/rankings/index.scss`, built like `ui/scss/sims/detailed_results/index.scss`. It must stay
  usable down to phone width: bars shrink, numbers don't wrap.

**UI tests:** `tools/uitest/tests/rankings/rankings.spec.ts`. Route the JSON to a small fixture with 4
builds × 2 content phases, and take every expected number from the fixture. Check:
- sort order and tier letters;
- phase and fight switching;
- that the role filter hides rows but keeps their letters;
- that the URL state round-trips;
- that link hrefs point at the spec dir and open in a new tab.

Against the real JSON, check that one build's link opens its sim with that content phase's BiS head item
and the standard buffs. Also check the homepage link.

## Docs (same change as each phase, through `/compact-docs-writer`)

- `tools/rankings/README.md`: run commands, the standard scenario (buff list, potions, fight), output, and
  when to rerun.
- `CLAUDE.md` doc map row: "regenerating the DPS rankings" → the README.
- `docs/guide/sim-architecture.md`: `ui/rankings/` under UI, `tools/rankings` under Tools.
- `docs/guide/workstreams.md`:
  - effort row: owns `tools/rankings/`, `ui/rankings/`, `ui/scss/sims/rankings/`, `assets/rankings/` and
    `tools/uitest/tests/rankings/`;
  - dependency line: imports statweights' `capture.py` and `merge.py` and presetgen's `gen.py`, and reads
    the `p*_bis*` presets;
  - "Re-baseline" line: the rankings re-run after every BiS preset re-baseline.
- `CONTEXT.md`: a term for the standard scenario, if the README and page need one.

## Process

- Each phase follows the phase loop: implement and verify in a subagent, review in a fresh one, stop.
  Commit only on "commit and merge".
- Other efforts' paths touched:
  - P0: `gen.py`, `page.js` and the hunter and DK presets (bis-optimizer); `generate_weights.py` and
    `SimWeightsMgr` (mod-playerbots repo);
  - P2: `sim_title_dropdown.ts` and `ui/index.html` (unowned); `optimizer_tab.ts` only if `PHASE_LABELS`
    moves.
  - Never edit `tools/statweights/` code or other builds' gear presets.

## Verification

**P0:**
- Run `sim/serverdata_test.go` (it checks every preset), plus `go test` and `go vet` in the container.
- `dock.sh tsc`.
- The new presets show on the hunter and DK pages for the right trees.
- Every Unholy 2H pick has a two-hander.
- A `--no-write` presetgen spot check on one existing build shows Heroic Presence on in its applied
  settings.
- `generate_weights.py` runs clean on a weights.json that includes the two keys.
- `~/.claude/scripts/pb-syntax-check.sh` passes on the changed mod-playerbots `.cpp` files.

**P1:**
- `go test --tags=with_db ./tools/rankings/...` and `go vet`, in the container.
- Full run: capture against a sim server whose UI matches the checkout. That's prod :3333 if its image
  matches, else a dev container on :3334 (no AzerothCore server involved). Then the runner (~10 min) and
  merge.
- Sanity: everything simmed and no 0 DPS. The P5 order is plausible against `tmp/statweights/weights.json`.
  Hunter and warlock numbers include pets.
- Open one build's `link` in its sim page and Simulate at 10k iterations. The DPS should land within noise
  of the ranked 3-min value.

**P2:**
- `dock.sh tsc`, and lint each touched file against HEAD (`lint:js` already fails on master).
- `dock.sh exec make dist/wotlk/.dirstamp`, restart the dev container, then
  `UITEST_URL=http://localhost:3334 npx playwright test tests/rankings`.
- Screenshots via `tools/uicheck` at desktop and phone width.

## Out of scope

AoE fights; tanks and healers; raid-contribution rankings; per-phase talents; re-simming live in the
browser.
