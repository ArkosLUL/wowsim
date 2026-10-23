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

**Wave Z17 (2026-10-23)**
- **Caps are hard limits** for reforge suggestions: one you meet stays met, one you're short of never gets shorter. No checkbox.
- **Apply on server:** a tab button. The sim server sends `.reforge apply` to the worldserver over SOAP.
  - Online characters change at once; offline ones get the reforges at next login.
  - Only core-raid characters (`AC_RAID_NAMES_FILE`).
  - SOAP is published on `192.168.49.35:7878` and `127.0.0.1:7878` only, never the internet. The server is public; the user keeps 7878 unforwarded.
  - A dedicated administrator account, created by the user. Its login sits in `.wowsim/prod.env` as `AC_SOAP_USER`, `AC_SOAP_PASSWORD`; agents never read it.
- **mod-reforging commits:** its uncommitted thread-safety edits (September BiS dataset work, already live) go in first as their own commit, then CAP-SERVER's. Both are approved.

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
| Z16 | CAP-TAB · CAP-ROSTER | the user's call, 2026-10-22, before Z15's follow-ups (Z18) |
| Z17 | CAP-FLOORS · CAP-SERVER | the user's call, 2026-10-23: Apply all dropped hit and expertise below their caps; reforges applied on the server |

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

#### As built

**Stage 1, caps:**
- `sim/statcaps`: `bisect.go` holds the moved engine (`RoleOf` with `RoleHealer`, `BisectCapBonus(SheetAt, …)`,
  `RequiredDefense(SheetAt)`, `GlobalCooldown`, `SteadyShotCastTime`); `compute.go` holds `Compute`,
  `ComputeRequest` (the RPC, raid index 0) and `PlayerSheetAt`. Caps bisect in parallel goroutines; a spec takes
  milliseconds.
- The optimizer's wrappers keep the old names; `dpsCapRole` maps a healer to none.
- `PlayerSheet.MainHand`/`OffHand` are `HandSheet` values with a `Swings` flag, not pointers:
  `TestPoolSheetMemo` compares sheets with `!=`. Also added: `BareTable`, the target's table with nothing of the
  player's, for the bare-cap rows. `ArmorPenPercent` is 0–100.
- Proto enums `StatCapKind`, `StatCapHand`. An unreachable cap has `needed` = +Infinity and an "Out of reach" note.
- Rows: ArP is a ceiling with sources (rest: "Stances and talents"); haste ceilings carry only the note; Defense
  lists race, talent and buff rows only, since crit reductions aren't rating.
- The tab: `tr.caps-row` carries `data-label`, `data-hand`, `data-rating`, `data-needed`; the roster button sits in
  a "Your character" block on the right. Tests: `tools/uitest/tests/caps/caps_tab.spec.ts`.

**Deviations:**
- Yellow parry is 460 and white parry 440, not 459/439: 459 rating truncates to 1399 bp.
- Spell hit is capped at a 1 bp threshold: the server misses only on `irand(1, 10000) < threshold`. That gives 446
  and 367 exactly. The optimizer's floor still asks for 0 bp, sometimes one rating more.
- Misery is 78.7 rating, so on whole-rating gear it moves the cap by 78 or 79; the default Shadow page gets 78. The
  Playwright test accepts either and checks the source row reads −79.
- The default Fury page is Draenei and the default Shadow page has Improved Faerie Fire: the tests switch to
  Human and clear it first.
- Not a deviation (cross-review): stance ArP auras count on the sheet. Arms' Battle Stance and Endless Rage
  (24%) put its cap at 1064 (`TestArmsArmorPenCountsBattleStance`); Fury's Berserker Stance has none, so 1400.

**Stage 2, values:**
- `publish.py --out <work dir>` (`--dest` overrides the asset) carries `weights.json`'s `sim_commit` and
  `generated_at`; per build adds `name` (presetgen's label), `tree` (its `TREES` table) and `two_hand` (`TWO_HAND`).
  Slopes keep 4 significant digits. It refuses unless every presetgen build has phases 1–5 and a tree.
- The asset is the seed, published as is: no `sim/`, `proto/` or `assets/` commit since `cc854cd47`.
- bischain publishes after the rankings merge, whose `sim_commit` would read the new asset as a dirty sim. With
  `--rankings-dest` it writes `<statweights work dir>/stat_values.json` instead.
- `stat_priority.ts`: `GEAR_STATS` (attack power = AP + RAP too), `chooseBuild` (spec, tree, two-hand; with no
  build for the tree, the spec's first, flagged), `capState`, `priority`.
- The cap rule covers only melee hit, spell hit, expertise and ArP, the stats statweights measures below the cap.
  A stat is capped once past every hard cap on it (both hands, dodge and parry). Hit with only one part capped
  keeps the other and reads "capped for attacks (N over)".
- The tab: a "Stat priority" block beside Caps. `caps-tab.v1` holds `{phase, metric}`. Healer pages fetch
  nothing. Rows are `tr.priority-row` with `data-stat`, `data-value`. Tests: `tools/uitest/tests/caps/caps_priority.spec.ts`.

**Deviations (stage 2):**
- Survival ratios are to the stat weights tool's tank reference (`tankRefStat`, default armor), not
  `epReferenceStat`: AP and SP barely move TMI.

**Stage 3, reforge:**
- `stat_priority.ts`: `REFORGE_TYPE_STATS` (TS copy of `reforgeStatTypeToStats`), `reforgeDelta` (zero when the
  settings refuse the reforge, like `core.ReforgeStats`), `suggestReforges`. Candidates are `validReforges` minus
  the current one, plus "remove" when reforged; each is scored as the net change after the current reforge comes
  off. Per stat, the cap rule is piecewise: adding is worth the value up to the hard cap room; taking away costs the
  value below the soft cap too. Picks need a gain above 1e-6.
- The tab: a "Reforge suggestions" block under Caps and Priority (now in `.caps-tab-top`), scored with the priority's
  phase and metric. "Apply all (+gain)" sets every pick in one `setGear`; Apply sets one. Rows grey out and buttons
  disable on any change until the new caps arrive (`gearChangeEmitter` added to the refresh triggers). Messages for
  healers, reforging off, missing values or caps. Rows are `tr.reforge-row` with `data-slot` (ItemSlot),
  `data-item`, `data-current`, `data-suggestion` (label, `keep` or `remove`). Tests:
  `tools/uitest/tests/caps/caps_reforge.spec.ts`, which also unit-tests `suggestReforges` without a page.

**Deviations (stage 3):**
- Past the yellow hit cap but short of the white one, removing hit costs its full shipped value while adding gains
  nothing: the shipped value can't price white-only hit, so suggestions never spend it.
- A suggestion can also be "Remove the reforge".

**Stage 4, live:**
- `sim/statslopes`: `statslopes.go` holds the moved roles (plus `Healer`: Int, Spirit, SP, MP5, spell crit, spell
  haste, read in HPS), `CenteredSteps`, `Simulate`, `Shifted`, `Centered`; `onesided.go` holds `OneSidedSteps`
  (centered deltas, no shift), `Measure` and `MeasureAsync`. `tools/statweights/steps.go` is gone; the tool maps
  `Metrics` onto its own JSON `slope`, so its output has no `hps`. Results matched the old tool byte for byte
  bar `seconds` (Fury and Prot warrior P5, 300 iterations, seed 424242).
- One-sided runs each sim single-threaded via `core.RunRaidSim`, one per core minus one: IsTest sims don't
  recover panics, and this way `runOne` does instead of the server dying. The low step stops at the stat's
  final value (`low_delta`; 0 leaves loss unmeasured).
- Proto: `StatSlopesRequest {raid, encounter, sim_options}`, `StatSlope`, `StatSlopeMetrics`,
  `ProgressMetrics.final_slopes_result` (13), counted final in `isFinalProgress`, wasm's
  `processAsyncProgress` and `newProgressHandler`. `sim.statSlopes(player, onProgress)` sends the page's
  iterations and `nextRngSeed()`.
- `stat_values.ts`: `Slope.hps`, `SlopeSet`, `LiveValues {gain, loss}`, `liveValues(result)`.
  `stat_priority.ts`: `Metric` adds `hps`; `livePriority` (gain-sorted, no cap rule); `suggestReforges` takes
  `ShippedPhase | LiveValues` (`isLive`), scoring added rating at gain and removed at loss, no cap rule.
- The tab: "Measure at my gear" (`.priority-measure`, progress "Measuring… N of M sims" in
  `.priority-measure-status`) in the priority block, healers included (metric `hps`, no switch). After a run
  the `.priority-source-switch` (`data-source` `shipped`/`live`, "Phase N BiS" / "My gear") shows and picks
  live; live disables the phase select and adds a "Losing one" column (`data-loss`). Stale = the run's raid and
  encounter protos no longer `equals` the page's, checked on every refresh trigger: the table gets
  `.priority-stale` and the suggestions block says to measure again. Live values aren't saved.
- Tests: `sim/statslopes/statslopes_test.go`; `tools/uitest/tests/caps/caps_live.spec.ts` (stubbed
  `/statSlopesAsync` + `/asyncProgress` by progress id, and one real run); a live scoring case in
  `caps_reforge.spec.ts`.

**Stage 5, docs:**
- New `docs/guide/stat-caps.md`: caps per role, the gear-side frame and sources, the optimizer's caps (moved from
  `tank-modeling.md`'s "Stat caps", now a link), shipped and live values, the cap rule, reforge scoring, the tab.
  CLAUDE.md maps it; the tank-modeling row drops "any run's hit, expertise or haste caps".
- `sim-architecture.md`: `sim/statcaps`, `sim/statslopes`, the tab with `stat_values.ts` and `stat_priority.ts`,
  `tools/statweights` and the asset.
- READMEs: statweights (`publish.py`, its output, `TREES`/`TWO_HAND`, the `statslopes` library); bischain (the
  publish step, the asset to commit, `--rankings-dest`); uitest (`tests/caps/`, and `tests/lib/acore.ts` tagged
  `(caps-and-priority-roster only)`).
- CONTEXT.md: a Caps & Priority section: gear rating, soft cap, shipped values, live values, cap rule.

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

#### As built

- **Fetch:** with `simServerAvailable()`, opening fetches once and a **Fetch from server** button fetches again. The open fetch keeps a non-empty roster pasted or uploaded meanwhile; the button overwrites it. Status and errors show inline in `.acore-server-status`, never as an alert.
- **Preselection:** the remembered pick (`{"character": name}`) when the roster holds it, else the best score: spec match 2, main-tree match 1, ties in roster order. A roster refresh keeps the current pick if still listed. The pick is saved on change and on Import.
- **Heroic Presence:** `heroicPresenceSource(roster, char)` in `acore_roster.ts` returns the character or the first subgroup member with Draenei racials. Racials follow `swapRaceId` ([azerothcore-data](../guide/azerothcore-data.md#module-tables)): a Draenei swapped away gives none, a Night Elf swapped to Draenei does. Import sets the page party's buff and adds an "on"/"off" note naming who.
- **Tests:** `tests/lib/acore.ts` holds `stubServer`, `errorReply`, their types, the server's error texts, and `dropServerRoster`. That aborts `/acoreRoster` for a test opening the importer without testing its fetch: `watchForErrors` counts a plain dev server's 503, not a dropped request. `roster_import.spec.ts`: 7 stubbed tests plus the live one, which passed against a dev server with prod's `--acore*` flags.

### CAP-FLOORS (Z17; golden-neutral; stages: code)

Apply all could leave hit or expertise below a cap: `suggestReforges` priced rating under a cap at the stat's value but never forbade it. Shipped values made it worse: hit 2.13 vs haste 2.14 per point (Unholy DW P3) moved every point of hit; dual wielders' hit was measured mostly past the yellow cap; some expertise partly above the dodge cap (Enhancement P1–P2, Unholy P3, Combat P1–P3, Ret P3–P4, up to 25%), tanks' fully. Live values had no cap rule, and a loss measured over its step was applied to any amount.

**Floors**, in `suggestReforges` (`ui/core/stat_priority.ts`), for shipped and live values alike:
- Every `StatCap` of kind `StatCapFloor` with a finite `needed`: hit, spell hit, each expertise row (hand, dodge, parry), Defense. Soft caps and ceilings aren't floors.
- Start surplus = `Math.round(rating) - Math.round(needed)` (as `roomTo`); limit = `min(0, start)`.
- A candidate moves each floor by its net change on the cap's `stat` (generic hit moves `MeleeHit` and `SpellHit`; expertise moves every hand's rows). The greedy skips any candidate taking a floor's running surplus below its limit.

**Noise rule:** a candidate counts only when its gain is at least `MIN_RELATIVE_GAIN` (0.05) × the value it gives up (its negative `moveValue` parts); giving up nothing, it only has to beat `MIN_GAIN`.

**Tab:** one hint line in the reforge block, "Suggestions keep every hit and expertise cap you meet." `priority()`, `livePriority()` and the cap notes stay. Re-measuring the shipped windows is a follow-up (the stat weights chain).

**Tests:**
- `caps_reforge.spec.ts` units: met hit and expertise caps stay met (shipped and live); a short cap never gets shorter; near-equal values are skipped; existing tests pass, or change only where the old greedy went under on purpose.
- Page test on the item's dev server for Unholy DK (dual wield, P3), Fury, Combat rogue and Prot Paladin: Apply all, wait for fresh caps, no met cap is short.

**Owns:** `suggestReforges` and its helpers in `stat_priority.ts`; the hint line in `caps_tab.ts`'s reforge block; `tools/uitest/tests/caps/caps_reforge.spec.ts`; [stat-caps](../guide/stat-caps.md)'s "Reforge suggestions"; this section's `#### As built`.

#### As built

- `suggestReforges` builds floors (`floorsOf`) from the caps; `keepsFloors` skips any candidate taking one below `min(0, start surplus)`, counting earlier picks. `gainOf` also returns the value given up for the 5% noise rule.
- The hint line is `p.reforge-floor-hint`. No Go change; goldens untouched.
- Tests in `caps_reforge.spec.ts`: floor, short-cap, spell-hit, live and noise units; a page test per Unholy DK, Fury and Prot paladin (default gear) and Combat rogue (Combat Fists, P3 Combat gear), at phase 3, checks no floor ends below `min(0, before)` after Apply all. Floors on the page are read from cap labels, since the row carries no kind.

### CAP-SERVER (Z17; golden-neutral; C++; stages: server, route, live, docs)

A tab button sends the sim's reforges to a core-raid character on the live server. mod-reforging reads `character_reforging` into memory only at startup, so the change goes through a GM command over SOAP, never a DB write.

**Stage 1, server command** (`[ac]/modules/mod-reforging`, edited in place; read [ac]'s `AGENTS.md` and `.agents/docs/cpp-guidelines.md` first):
- `src/mod_reforging_commandscript.cpp`, registered in `mod_reforging_loader.cpp`: `.reforge apply <character> <slot>:<item>:<from>:<to> …`, `SEC_ADMINISTRATOR`, `Console::Yes`. Server equipment slots (`EQUIPMENT_SLOT_*`), ItemModType ids, `0:0` removes. SOAP runs commands on the world thread (`ACSoap.cpp` queues a `CliCommandHolder`).
- New `ItemReforge` members, under `reforgingDataLock`:
  - `ReforgeAmount(ItemTemplate const*, from, to)`: the amount, or 0 unless both stats are reforgeable, `from` is on the item with a positive value and `to` isn't, quality ≤ legendary, `StatsCount` in range, and `CalculateReforgePct(value) ≥ 1`;
  - `GetReforgingData(uint32 itemGuid)`;
  - `SetStoredReforge(ownerGuid, itemGuid, from, to, value)`, `EraseStoredReforge(itemGuid)`: map plus `DirectExecute` on `character_reforging`.
- Lookup: `sCharacterCache->GetCharacterGuidByName`, then `ObjectAccessor::FindConnectedPlayer`.
- Online, per slot: the item at `(INVENTORY_SLOT_BAG_0, slot)` must have that entry (else `mismatch`); the same reforge is `unchanged`; else `RemoveReforge`, then `Reforge` for a non-zero target (both re-apply stats and send the item packet); a failed `Reforge` puts the old one back and reports `failed`.
- Offline: one `SELECT ci.slot, ci.item, ii.itemEntry FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ci.bag = 0`, `ReforgeAmount` on the template, then the stored-reforge members; `OnPlayerApplyItemModsBefore` applies them at login.
- Reply: `character <name> online|offline`, then one line per slot, `<slot> <applied|removed|unchanged|mismatch|invalid|failed> <text>`. An unknown character fails the command (a SOAP fault).
- Compile check: `PB_REPO=<module> sh ~/.claude/scripts/pb-syntax-check.sh <files>`, else the [azerothcore-server](../guide/azerothcore-server.md) fallback. No build: the live stage rebuilds.

**Stage 2, route and dialog:**
- `sim/web/acreforge`: `Config{SoapURL, SoapUser, SoapPassword, NamesFile}` with `WithEnv`, like `acroster.Config`: `AC_SOAP_URL` (`--acoreSoapUrl`), `AC_SOAP_USER` (`--acoreSoapUser`), `AC_SOAP_PASSWORD` (env only), the roster's names file. Wired in `sim/web/main.go` beside `/acoreRoster`; `compose.yaml`'s `x-acore-env` adds `AC_SOAP_URL: http://192.168.49.35:7878/`.
- `POST /acoreReforge`, `Content-Type: application/json`: `{character, items: [{acSlot, itemId, from, to}]}`. The character must be in the names file; 1–19 items, unique `acSlot` 0–18; `from`/`to` reforgeable ids or both 0.
- Builds the command and posts the `urn:AC` `executeCommand` envelope with basic auth; one request at a time, 30 s; parses `<result>` or `<faultstring>` (`encoding/xml`). Replies `{online, slots: [{acSlot, status, text}]}`.
- Errors `{code, error}`: `bad_request` 400, `cross_site` 403 (`Sec-Fetch-Site`), `method_not_allowed` 405, `not_in_roster` 409, `not_configured` 503 (names what's missing), `soap` 502 (a 401/403 reads "the SOAP login was refused"; fault text passes through), `timeout` 504. The password never reaches a log, an error or a reply.
- Tab: an "Apply reforges on server…" button in the "Your character" block, only on a sim server. Dialog (`acore_reforge_dialog.ts`, a `BaseModal`): a character dropdown of the page's class from a fresh `fetchServerRoster()`, preselected from the importer's saved pick (`acore-import.v1`); a row per equipped slot via `acSlotToItemSlot`: item, server reforge "as of last save", sim reforge, status (will change, same, or "different item on the server", left out). Send posts every matching slot, then shows each slot's reply. Notes: logged-out characters get them at next login; a bot redoes its own reforges when it refreshes its gear.
- Tests: `dock.sh test ./sim/web/acreforge/` with an `httptest` SOAP server (auth header, command text, parsed reply, fault, 401, timeout, `not_in_roster`, `not_configured`, no password in any error); `tools/uitest/tests/caps/server_reforge.spec.ts` against stubbed `/acoreRoster` and `/acoreReforge` (preselection, a mismatched slot left out, the posted body, results, no button under fake wasm).

**Stage 3, live** (returns `blocked`, naming what's missing, until the user has made the SOAP account and added its login to `prod.env`):
- The orchestrator's config, before this stage: `[ac]/configurationOverrides/Soap.env` (`AC_SOAP_ENABLED=1`, `AC_SOAP_IP=0.0.0.0`), its `env_file` line and a `127.0.0.1:7878:7878` port in `[ac]/docker-compose.override.yml`, `DOCKER_SOAP_EXTERNAL_PORT=192.168.49.35:7878` in `[ac]/.env`. All gitignored.
- Rebuild the worldserver once Deathsong is offline ([RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md#standing-authorizations)); `docker port ac-worldserver` must list 7878 on `192.168.49.35` and `127.0.0.1` only.
- A dev server with the `--acore*` flags and `--env-file` prod.env. If its container can't reach `192.168.49.35:7878`, switch to the AzerothCore Docker network (`ac-worldserver:7878`) and report it.
- Round trip on **Angry** (the user's pick): read the roster, flip one item's reforge through `/acoreReforge`, check `/acoreRoster` shows it, put it back. Nothing else changes. Report whether the online or offline path ran.

**Stage 4, docs** (via `/compact-docs-writer`): [stat-caps](../guide/stat-caps.md) "Apply on server"; [dev-environment](../guide/dev-environment.md) a `/acoreReforge` block beside `/acoreRoster` and prod.env's three lines; [azerothcore-server](../guide/azerothcore-server.md) SOAP on via `Soap.env` on those two addresses (drop "There's no SOAP" twice), `.reforge apply` in the mod-reforging row, the reboot caveat (if `192.168.49.35` isn't up when Docker starts the worldserver, Docker may refuse to start it); [sim-architecture](../guide/sim-architecture.md) a `sim/web/acreforge` row; the uitest README's new spec; `#### As built`.

**Owns:** `[ac]/modules/mod-reforging/src/`; `sim/web/acreforge/` and its lines in `sim/web/main.go` and `compose.yaml`; the "Your character" block in `caps_tab.ts`, `acore_reforge_dialog.ts` and its styles in `_caps_tab.scss`; `tools/uitest/tests/caps/server_reforge.spec.ts`; the stage 4 docs.
