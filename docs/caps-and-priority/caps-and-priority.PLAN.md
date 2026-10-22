# Caps & Priority tab

A new tab on every individual sim page. It shows:
- each spec's caps as gear rating, after talents, debuffs and Heroic Presence;
- a stat priority per content phase, measured at that phase's BiS or at the player's own gear;
- reforge suggestions.

The page's AzerothCore importer also fetches the roster from the server.

Runs in the [wave loop](../wave-loop/wave-loop.PLAN.md). The [RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md)'s agent rules apply to each item.

## Decisions (settled with the user, 2026-10-22)

**Caps**
- **Rating from gear**, like the in-game sheet:
  - "Yours" is the rating from gear, gems, enchants, food, elixirs and bonus stats.
  - "Needed" is the boss's cap minus what race, talents, buffs, target debuffs and per-spell talents give.
  - Example: Fury with Precision 3/3 and Heroic Presence needs 131, not 262.
- **Caps per role:**

  | Role | Caps |
  |---|---|
  | Melee | hit (yellow); dual wield white hit (soft, off-hand weapon only; warriors get a note that a queued Heroic Strike or Cleave drops the off-hand penalty, `DisableDWMissPenalty`); expertise dodge; expertise parry (in front only); ArP 100%; spell hit when the rotation casts spell damage |
  | Hunter | hit; ArP 100%; haste ceiling (Steady Shot at 1 s) |
  | Caster | spell hit; haste ceiling (GCD at 1 s) |
  | Tank | hit; expertise dodge and parry; Defense for crit immunity (only when the boss swings at them) |
  | Healer | haste ceiling (GCD at 1 s) |
- **No crit cap.** The sidebar stays untouched.
- **Expertise per hand:** separate main-hand and off-hand rows, only when they differ (Orc axe main hand, sword off hand).
  - The cap is the yellow table's: 212 removes dodge, 459 removes parry.
  - A note gives the white-swing points: 192 for dodge, 439 for parry.

**Stat values**
- Two sources:
  - shipped values, measured at each phase's BiS gear and shown instantly;
  - "Measure at my gear", a live run.
- A **phase picker**.
- Each value shows per point, plus its ratio to the spec's `epReferenceStat`.
- Tanks get a Survival (TMI) / Threat (TPS) switch, Survival first.
- Healers get live values only. No healer BiS chain work.
- The shipped asset is seeded from spec-balance's 2026-10-22 run, read-only.
- The live run is one-sided: gain from +δ, loss from −δ.

**Reforging**
- Per-item suggestions, with Apply and Apply all, in the sim only.
- A suggestion may replace an existing reforge.

**Roster import**
- Brings in the **whole character**: gear with gems, enchants and reforges, talents, glyphs, racial traits and professions.
- Sets Heroic Presence from the raider's live party.
- Reached from the Import menu and from a button in the tab.
- A dropdown lists the page's class, preselects the spec match and remembers the pick.

**Tab**
- Called "Caps & Priority", placed after BiS Optimizer.
- Shown on every spec page, never inside the raid sim.

## Design

### Caps engine: `sim/statcaps`

**Moved, not copied, out of `sim/optimizer/caps.go` and `critimmunity.go`:**
- the spec role table (`dpsCapRole`, plus a healer role);
- `bisectCapBonus`, which now takes a `sheetAt(offset stats.Stats)` func;
- `globalCooldown`, `steadyShotCastTime` and `requiredDefense`.

The optimizer keeps thin wrappers under the old names, so its caps tests pass unchanged. Its expertise floor stays stat-based: that always equals the worse hand.

**`core.PlayerSheet` gains fields** (`sim/core/sheet.go`). The existing fields keep their meaning.
- **`WhiteMissChance`:** the main-hand white swing, with the dual wield penalty.
- **Per hand,** from `AutoAttacks.MHAuto()` and `OHAuto()`:
  - the off hand only when `IsDualWielding`;
  - nil guards, since both are nil without `AutoSwingMelee`;
  - the hand's yellow dodge and parry from `YellowMeleeTableBP(auto.YellowTableInput(at))`, with in front forced for parry;
  - the hand's white dodge and parry from `WhiteMeleeTableBP(auto.WhiteTableInput(at))`;
  - the hand's `BonusExpertiseRating`. A one-hand racial lands on that hand's spells (`racials.go`), and the table input includes it (`spell_result.go`).
- **`ArmorPenPercent`:** the unit's capped ArP %, after percent auras.
- **`CastsSpellDamage`:** true when a rotation spell has `ProcMaskSpellDamage`.
- **`SpellHitTaken` / `MeleeHitTaken`:** the target's bonus hit rating taken.
- **The player's `PlayerStats` snapshots:** base, gear, talents, buffs, consumes and final.

**`statcaps.Compute(raid, encounter, raidIndex)`** returns `[]StatCap`.

The gear-side frame:
- gear rating = final − base − talentsΔ − buffsΔ of the stat;
- needed = bisected sheet cap − base − talentsΔ − buffsΔ.

Each cap lists its sources in rating and %, dropping zero rows:
- **bare cap:** the core table function (`MeleeMissBP`, `SpellMissBP`, `YellowMeleeTableBP`) at no bonus, at the sheet's level difference;
- **race:** the base snapshot. A one-hand racial is named "<race> <weapon>: <hand> only".
- **talents:** talentsΔ;
- **buffs:** buffsΔ, named "Heroic Presence" when it's on;
- **target debuffs:** named from the debuff settings;
- **"talents on your attacks/spells":** the remainder, which covers Shadow Focus, Weapon Mastery and the like.

Ceilings get a note instead: the cast speed multiplier and the GCD or Steady Shot time.

An unreachable cap returns a note, not an error.

### Caps RPC

New messages in `proto/api.proto`:
- `StatCapsRequest {raid, encounter}`;
- `StatCapsResult {repeated StatCap caps, error_result}`;
- `StatCap {label, stat, hand, kind (floor | ceiling | soft), rating, needed, repeated StatCapSource sources, note}`.

Wired the way `computeStats` is:
- `/statCaps` in `sim/web/main.go`;
- the wasm export in `sim/wasm/main.go`;
- `ui/worker/sim_worker.js` and the net worker;
- `ui/core/worker_pool.ts`;
- `ui/core/sim.ts` `statCaps(player)`.

It is synchronous, at about 15 environment builds per cap.

### Tab: `ui/core/components/individual_sim_ui/caps_tab.ts`

**Shape:**
- `CapsTab extends SimTab`, identifier `caps-tab`.
- Registered in `individual_sim_ui.ts` after the optimizer tab, and skipped when `isWithinRaidSim`.
- Styles in `ui/scss/core/components/individual_sim_ui/_caps_tab.scss`, imported in `ui/scss/core/individual_sim_ui/index.scss`.
- Built from `ContentBlock` and the DOM helpers `optimizer_tab.ts` exports.

**Caps block:**
- Columns: cap, yours, needed, over/short.
- A row expands into its sources.
- Expertise also shows points.
- Refreshes only while the tab is shown, on `currentStatsEmitter` and on buff, debuff, talent and encounter changes. Debounced; stale replies are dropped.

**"Import from roster…" button:**
- Opens the page's AzerothCore importer through `IndividualSimUI.openAcoreImporter()`, which the Import menu entry uses too.
- Shown only on a sim server (`simServerAvailable()`).

**Priority block:**
- **Build choice:** spec, main talent tree (`player.getTalentTree()`), and a two-hand flag when the main hand is two-handed (`dk_unholy_2h`).
- **Phase picker:** phases 1–5, labelled with `PHASE_LABELS`.
  - Follows `sim.getPhase()` until the user picks one.
  - The pick is saved under `simUI.getStorageKey('caps-tab.v1')`, in try/catch.
  - Changing phase re-ranks the priority and the suggestions.
- **Stats as gear carries them:**
  - hit, crit and haste rating = melee slope + spell slope (`reforgeStatTypeToStats`);
  - the rest one each;
  - plus weapon DPS.
- **Cap rule:** shipped values were measured below the cap.
  - At or past a hard cap, more of that stat is worth 0, and the surplus reads "N over: reforge away".
  - For dual wield past the yellow cap, it reads "white swings only: measure at your gear".
- **Reforgeable marker:** from `reforgingFor(encounter server settings)`.
- **"Phase N BiS / My gear" switch:** shown once a live run exists.

**Reforge suggestions block:**
- Candidates per equipped item come from `validReforges`.
- Score = amount × (target value − source value), with the cap rule applied on both ends.
- Picks greedily, best first, spending each cap's surplus or room as it goes.
- Each row shows the current reforge and either the suggestion with its estimated gain, or "keep".
- Apply sets the reforge the way the gear picker's Reforging tab does. Apply all applies every row.

### Shipped values

**`tools/statweights/publish.py`:**
- Writes `assets/statweights/stat_values.json` from `weights.json` and presetgen's `BUILDS`, imported read-only.
- Per build: key, class, spec, talent tree index (a table in `publish.py`) and the two-hand flag.
- Per phase: rounded slopes (dps, tps, tmi).
- Top level: `sim_commit` and `generated_at`.

**`tools/bischain/chain.py`:** its `weights` step runs `publish.py` after `merge.py`, except on a narrowed run without `--from-work`.

**`ui/core/stat_values.ts`:** fetches the asset as `ui/core/optimizer/catalog.ts` does.

**`ui/core/stat_priority.ts`:** pure functions for the build choice, the priority, the cap rule and reforge scoring.

### Live measurement: `sim/statslopes`

**Content:** the role lists, step rules and `measure` from `tools/statweights/steps.go` and `main.go`, plus a healer role: Int, Spirit, SP, MP5, spell crit, spell haste, measured in HPS.

**Modes:**
- **Centered:** keeps `tools/statweights` byte-identical.
- **One-sided:** for the UI, at the player's own gear.
  - gain = (sim(+δ) − sim(0)) / δ;
  - loss = (sim(0) − sim(−δ)) / δ;
  - one fixed seed, `IsTest`, at the page's iterations.

**`/statSlopesAsync`:** runs through `/asyncProgress`, with a wasm export, the worker, `worker_pool.ts` and `sim.ts`.

**The tab's button:** shows progress. Its values go stale on gear, talent, buff or encounter changes.

### Roster import: `IndividualAcoreImporter` (`ui/core/components/importers.ts`)

- **Server fetch:** with `simServerAvailable()`, opening the dialog fetches `fetchServerRoster()` (`ui/raid/acore_server.ts`). File and paste import still work.
- **Save note:** the dialog says gear is as of each character's last save (`saveall`).
- **Preselection:** the dropdown lists the page's class.
  - First choice: the character whose `inferSpec` equals the page's spec.
  - Then: the one whose main tree matches.
  - The pick is saved under `simUI.getStorageKey('acore-import.v1')`.
- **Import:** goes through the existing `onImport`.
- **Heroic Presence:** on when the raider's subgroup holds a Draenei `raceId` or `swapRaceId`, otherwise off. The import notes say which.

## Rules for every CAP item

- **Goldens:** golden-neutral, with no BiS re-baseline. The optimizer's caps tests and every golden stay unchanged.
- **Builds and tests:**
  - All Go, tsc and make runs go through `tools/acore/dock.sh`.
  - eslint each changed `.ts`/`.tsx` file.
  - UI changes get Playwright tests in `tools/uitest/tests/caps/`.
  - Run them against the item's own dev server, built from its worktree on its port, container `wotlk-dev-<id>`. Stop it by name when done.
- **Live DB:** the roster's `/acoreRoster` reads it SELECT-only. Tests stub it. Only an optional `UITEST_ACORE_LIVE=1` check reads the live DB.

## Waves

| Wave | Items | Why |
|---|---|---|
| Z16 | CAP-TAB · CAP-ROSTER | the user's call, 2026-10-22, before Z15's follow-ups (Z17) |

## Work items

### CAP-TAB (Z16; golden-neutral; stages: caps, values, reforge, live, docs)

The tab, its packages and RPCs, the shipped values and the docs. Stages:

1. **caps**
   - `sim/statcaps`, the `PlayerSheet` fields, and the optimizer moved onto `statcaps`.
   - `/statCaps` end to end.
   - The tab with its caps block and the "Import from roster…" button.
   - `statcaps` tests pin these:
     - bare hit 262;
     - Fury with Precision 3/3 plus Heroic Presence needing about 131;
     - spell hit 446, 367 under Misery;
     - Shadow Focus in the per-spell row;
     - an Orc axe/sword dual wielder's expertise rows differing by 41;
     - white notes at 192 and 439;
     - a hunter's and a caster's haste ceiling;
     - a tank's Defense floor equal to `requiredDefense`;
     - a healer's GCD ceiling.
   - Playwright:
     - the tab shows on every spec page, healers included, and not in the raid sim;
     - Heroic Presence moves Fury's needed hit by 33;
     - Misery moves Shadow's needed spell hit by 79.
2. **values**
   - `publish.py`.
   - The asset, seeded from `G:\DevStuff\GitHub\wowsimwotlk-balance\tmp\p6\sw\weights.json`. Read it in place and never write there. Its `sim_commit` `cc854cd47` is master's sim: check that no `sim/`, `proto/` or `assets/` commit since then changes it.
   - The bischain step, `stat_values.ts`, `stat_priority.ts`, and the priority block with its phase picker, tank switch and healer message.
   - Playwright: the priority renders from a stubbed `stat_values.json`; the phase picker re-orders it, and the pick survives a reload.
3. **reforge**
   - The suggestions block, with Apply and Apply all.
   - Playwright: Apply sets the item's reforge, checked in the gear picker.
4. **live**
   - `sim/statslopes`, with `tools/statweights` moved onto it. Its result JSON is byte-identical before and after on one settings file at one seed.
   - The healer role and `/statSlopesAsync`.
   - The button, the "Phase N BiS / My gear" switch, and stale marking.
   - Playwright: the button works against a stubbed `/statSlopesAsync`.
5. **docs**, through `/compact-docs-writer`:
   - **New `docs/guide/stat-caps.md`:**
     - caps per role, the gear-side frame, sources, per-hand rows;
     - shipped and live values, the cap rule, reforge scoring;
     - `tank-modeling.md`'s "Stat caps" section moves there and leaves a link.
   - **CLAUDE.md:** a doc-map row: "changing a cap, the Caps & Priority tab, or how stat values are shipped or measured".
   - **`sim-architecture.md`:** rows for `sim/statcaps`, `sim/statslopes`, the tab and the asset.
   - **READMEs:**
     - statweights: `publish.py` and the `statslopes` library;
     - bischain: the publish step;
     - uitest: the Layout row for `tests/caps/` and `tests/lib/acore.ts`.
   - **CONTEXT.md:** terms, if new names arise.
   - **This section:** an `#### As built`.

**Owns:**
- `sim/statcaps/`, `sim/statslopes/`;
- the new `PlayerSheet` fields in `sim/core/sheet.go`;
- the move in `sim/optimizer/caps.go` and `critimmunity.go`, with their tests untouched;
- `tools/statweights/{steps.go,main.go,publish.py}` and the README;
- the publish step in `tools/bischain/chain.py` and the README;
- `assets/statweights/`;
- `ui/core/stat_values.ts`, `ui/core/stat_priority.ts`, `caps_tab.ts`, `_caps_tab.scss`;
- in inline hunks:
  - `proto/api.proto`;
  - `sim/web/main.go`, `sim/wasm/main.go`;
  - `ui/worker/*`, `ui/core/worker_pool.ts`, `ui/core/sim.ts`;
  - `ui/core/individual_sim_ui.ts`: the tab, and `openAcoreImporter()` used by the Import menu entry;
  - the scss index;
- `tools/uitest/tests/caps/caps_*.spec.ts`;
- the docs of stage 5.

### CAP-ROSTER (Z16; golden-neutral; stages: code)

The importer's server fetch, as described under "Roster import".

Playwright, against the stubbed `/acoreRoster`:
- Deathsong is preselected on the DK page;
- the pick survives a reload;
- gear, talents and Heroic Presence apply;
- no fetch under fake wasm;
- the file import still works.

Plus an optional `UITEST_ACORE_LIVE=1` check against a dev server started with the `--acore*` flags and `--env-file` (`docs/guide/dev-environment.md`).

**Owns:**
- `IndividualAcoreImporter` in `ui/core/components/importers.ts`. Its constructor signature stays unchanged, since CAP-TAB's `openAcoreImporter()` calls it.
- A helper in `ui/raid/acore_roster.ts` that reads Heroic Presence for a character's subgroup.
- `stubServer`, moved from `tools/uitest/tests/raid/acore_server.spec.ts` into `tools/uitest/tests/lib/acore.ts`, with that spec importing it.
- `tools/uitest/tests/caps/roster_import.spec.ts`.
- This section's `#### As built`.
