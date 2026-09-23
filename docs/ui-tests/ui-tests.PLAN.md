# UI tests: a Playwright regression suite for the sim UI

The user was finding UI bugs by hand: the results page's player dropdown showed a neighbour's numbers, and
hovering a damage row stretched it 400px. This effort gives agents a browser suite to test UI behaviour
with, and fixes what it finds. It runs in the wave loop ([RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md)).

## Decisions (settled with the user)

- Playwright, on the host against the installed Chrome ([README](../../tools/uitest/README.md)).
- Tests replay a recorded result where they can: fast and deterministic. A real sim runs only where the
  round trip is the behaviour under test, at low iterations.
- A fix lands with its test, in the same item.
- Wave U, before I2 (2026-09-23).
- After wave U (2026-09-23): keep the log tab's search and raider filter; UI-FIX in I2 takes the rest.

## Done at wave U setup

- The harness, the fixtures and `gen_fixture`, the shared helpers, and a smoke test that loads every spec
  page, the raid page and the results page without a console error.
- `results_filter.ts` built its options from the raid slot but filtered on the sim's unit index
  (targets first), so every raider showed the one before, and the first showed the boss.
- `player_damage.ts` appended the tooltip's 600×400 chart into the table row, which grew to 413px while
  the tooltip stayed empty.

## Done in wave U

All four items merged: 294 tests passed, and 3 waited as `test.fixme` on sim bugs UI-FIX then fixed.
Each bug went test first; the fixes are in the items' commits ([wave-loop status](../wave-loop/wave-loop.PLAN.md#status)).

## Done in wave I2

UI-FIX merged: 319 tests pass, 7 skip on the Might bug PAR-P7-0f fixes in I3. The tooltip's late part is "Stance & Shout"
for warriors (their own shout lands after the Buffs snapshot too) and "Form" for druids. A Blood Elf
warrior's Arcane Torrent is 50613, the one mod-racial-trait-swap teaches; it restores nothing, so it never
casts itself.

## Follow-ups

**Sim, for a sim item:**
- Server parity: mod-racial-trait-swap teaches a Blood Elf druid both 28730 (mana) and 25046 (energy); the
  sim registers one.
- Server parity, unchecked: the sim gives Demonic Pact's spell power to the whole raid. Its aura, 48090,
  isn't in the spelldump, so whether the server limits it to the party is open.

**For a later UI wave:**
- Closed components stay in memory: every closed item picker is kept alive by `input.tsx`, which never
  drops its `changedEvent` listener, and in `gear_picker.tsx` by `ItemList`'s experimental-toggle listener
  and the EP and favourite-star Bootstrap tooltips. It spans owners, so it needs one item; `detached(page,
  '.modal')` in `tests/gear/gear.ts` measures it.
- `BulkSimResultRenderer` (`bulk_tab.ts`) never disposes its `ItemRenderer`s: one listener per wrist or
  hands result, per batch run.
- Opening the Log tab builds a row for every line: about 1.16M elements after a 25-man, 3-minute raid sim.
  Virtualize it.
- The damage-row pie leaves out pet damage, which the row's Amount includes.
- The DPS histogram doesn't narrow to a picked target, as the run has no per-target distribution. Hide it
  then?
- Each click on the Timeline tab renders `dpsResourcesPlot` again, adding a chart instance.
- `icon_enum_picker.tsx` `setImage` with a colour-only value doesn't cancel a pending
  `ActionId.fillAndSet`, so a late icon paints over a blank.
- The Link exporter with no category ticked exports everything; the glyph modal allows one glyph in two
  slots and stays open after a pick; every exporter downloads as `wowsims.json`.
- Loading a raid log prints 'Unmatched aura stacks change log' warnings: the sim logs `stacks: N --> 0`
  after 'Aura faded'.

## Work items

### UI-FIX (wave I2, done)

Four fixes wave U left open. Items 1, 3 and 4 have a `test.fixme` to turn into a test, red first.

1. The character stats tooltip's parts don't add up to the Total for stats a stance, form or presence
   multiplies (warrior Strength 2327 vs 2792, bear Armor 10915 vs 31079): the parts are snapshots taken
   before those multipliers. Add a tooltip row for what they add; the sim stays as is
   (`tests/gear/character_stats.spec.ts`).
2. The Batch tab's Sim Talents list offers only saved loadouts: add the spec's preset talents
   (`tests/gear/bulk.spec.ts`, a new test).
3. `setupDemonicPact` sizes its aura array at 25 and indexes it by raid index, so a 40-player raid with a
   Demonic Pact warlock panics (`tests/raid/raid_picker.spec.ts`).
4. Blood Elf racial traits on a rage user register Arcane Torrent with ActionID 0 (only runic power,
   energy and mana get one): the APL Cast list shows a nameless cooldown and the UI fetches spell 0's
   tooltip. Give it the spell the server gives a rage user under mod-racial-trait-swap (Spell.dbc,
   `[ac]/modules/`), or don't register it if there's none (`tests/settings/spec_inputs.spec.ts`; swap its
   fixed 1.5 s wait for a condition).

No test runs a 40-player raid or a Blood Elf rage user, so goldens shouldn't move: run the warlock and
warrior suites and report any delta.

**Owns:** `ui/core/components/character_stats.tsx`, `ui/core/components/individual_sim_ui/bulk_tab.ts`,
`sim/warlock/talents.go`, `sim/core/racials.go`, and those four test files.

### UI-FIX2 (wave I3)

Two bugs the user found (2026-09-24):

1. The Raid page's BiS Batch tab has no item-source filter: `buildRequest` (`ui/raid/optimizer_batch.ts`)
   runs every job on `defaultTabSettings`, so every source counts. Give it the BiS tab's "Item sources"
   checkboxes (`SOURCE_KINDS`, `ui/core/optimizer/catalog.ts`), saved and restored with the batch's other
   settings (`tests/raid/bis_batch.spec.ts`).
2. After a reload, the Raid page's Edit window shows every character stat as 0 until the raid changes:
   `updateCharacterStats` (`ui/core/sim.ts`) skips event ID 0, and `RaidSimUI.loadSettings`
   (`ui/raid/raid_sim_ui.ts`) runs as the page's first event, 0. The rotation warnings, Suggest gems and the
   set-bonus spec inputs read the same stats. Compute them once the saved raid has loaded (`tests/raid/`:
   the Edit window's stats match before and after a reload).

Golden-neutral.

**Owns:** `ui/raid/optimizer_batch.ts`, `ui/raid/raid_sim_ui.ts`, `updateCharacterStats` in `ui/core/sim.ts`,
and those tests.

### Every UI-* item

- Tests go in `tools/uitest/tests/<area>/`, helpers for the area beside them. `tests/lib/**`,
  `tests/smoke.spec.ts`, `playwright.config.ts`, `package.json` and the two `fixtures/raid25*` files
  are shared and frozen for the wave: changes go in `contractChangeRequests`. So do changes to
  `ui/core/{player,sim,sim_ui}.ts`, `ui/core/proto_utils/` files not listed below, `proto/`, and
  `ui/core/optimizer/**` and `optimizer_tab.ts`, which BIS-seed reworks in I2.
- Each bug: a failing test first, then the fix, then green. A bug outside your owned paths keeps its test
  as `test.fixme('<reason>')` and goes in `followUps`.
- Assert what a player would notice: the right numbers and names, what survives a reload. Derive
  expected values from the fixture or the page, never hard-coded sim numbers: goldens move every wave.
- Never edit `ui/<spec>/apls/*` or `ui/<spec>/gear_sets/*` (the Go tests load them), nor `db.json`.
- Set iterations to 100 or fewer before a real Simulate.
- Dev server: container `wotlk-dev-<id>` on your port, started as in
  [dev-environment, Run](../guide/dev-environment.md#run) with your worktree mounted, image
  `wowsim-toolchain` and volumes `wowsim-gomod`/`wowsim-gocache`. After a UI change,
  `dock.sh exec make dist/wotlk/.dirstamp`, no restart; after a Go change, restart. Leave it running.
- Verify: `cd <worktree>/tools/uitest && npm install && UITEST_URL=http://localhost:<port> npx playwright test --workers=4`,
  the whole suite; `dock.sh tsc`; eslint per changed file vs HEAD ([testing, UI](../guide/testing.md#ui)).
- The report's summary lists every bug found, one line each, fixed or fixme'd.
- The lists below are in priority order. Cover them, then hunt in your area until the finds dry up.

### UI-RESULTS

The results page, standalone (`/wotlk/detailed_results/`) and embedded in a sim page.

1. The target filter, on a multi-target fixture: damage narrows to the target picked, labels match.
2. Every metrics table (damage, healing, damage taken, buffs, debuffs, casts, resources) shows the picked
   unit's rows (as `results/results_tabs.spec.ts` does for casts), and sorts by its headers.
3. The timeline's charts and selectors, per raider.
4. The log tab's search and filters, and its debug toggle.
5. A reference run (`SimRunData.referenceRun`) and its comparison.
6. Individual-sim mode (`?isIndividualSim`), on a single-player fixture.
7. Tank and healer metrics (damage taken, TMI, HPS) for raid25's tanks and healers.
8. Embedded: a small Simulate on a spec page fills its results tab, and "View in Separate Tab" gets the
   same data.

**Owns:** `ui/core/components/detailed_results.ts`, `ui/core/components/detailed_results/**`,
`ui/detailed_results/**`, `ui/core/components/{results_viewer.tsx,raid_sim_action.ts,unit_picker.ts}`,
`ui/core/proto_utils/{sim_result.ts,logs_parser.tsx}`, `ui/scss/core/components/detailed_results/**`,
`ui/scss/core/components/{_detailed_results,_raid_sim_action,_unit_picker}.scss`,
`ui/scss/sims/detailed_results/**`, `tools/uitest/gen_fixture/**`, `tools/uitest/tests/results/**`, new
`tools/uitest/fixtures/results-*` (inputs beside them).

### UI-GEAR

The individual sim's Gear and Bulk tabs.

1. The item picker per slot: search, phase and source filters, equip. The slot shows the item, and the
   character stats move the way the item's stats say.
2. Enchants and gems: sockets by colour, the meta gem's active state, socket bonuses.
3. The Reforging tab: a reforge moves the two stats, and only reforges mod-reforging allows are offered.
4. Gear sets: save, load, delete, and all three survive a reload.
5. Unequip, Clear, Unequip All Gems, Suggest Gems.
6. The Bulk tab: add items, a small batch, its results.
7. Item swaps, on a spec that has them.

`ItemRenderer` draws items on the bulk and optimizer tabs too, so its styles in `_gear_picker.scss` are
global ([sim-architecture, UI](../guide/sim-architecture.md#ui)).

**Owns:** `ui/core/components/{gear_picker.tsx,item_swap_picker.tsx,character_stats.tsx,suggest_gems_action.ts,filters_menu.ts}`,
`ui/core/components/virtual_scroll/**`, `ui/core/components/individual_sim_ui/{gear_tab.ts,gem_summary.tsx,bulk_tab.ts}`,
`ui/core/proto_utils/{gear.ts,equipped_item.ts,gems.ts,enchants.ts,reforging.ts}`, their scss partials
(`_gear_picker`, `_item_swap_picker`, `_character_stats`, `_filters_menu`, `_bulk`, `_gear_tab`,
`_gem_summary`), `tools/uitest/tests/gear/**`.

### UI-RAID

The raid page.

1. The raid picker: add a player from the presets, move them between parties and slots, rename, remove,
   and edit one; the changes stick.
2. The raid stats panel follows the roster as players come and go.
3. The blessings, assignments and tanks pickers, and the raid settings tab.
4. Raid export then import gives the same raid.
5. A real raid sim, few iterations: the results name the right raider in the player dropdown and the
   damage rows, live rather than replayed.
6. The AzerothCore importer with `ui/raid/acore_harness/testdata/raid.json`: its dialog, the grid and the
   alerts, which [its harness](../../ui/raid/acore_harness/README.md) leaves to a person.
7. The BiS Batch tab on that roster: the grid, and ticking raiders and phases. At most one Quick job.

**Owns:** `ui/raid/**`, `ui/core/{raid,party}.ts`, `ui/core/components/raid_target_picker.ts`,
`ui/scss/sims/raid/**`, `ui/scss/core/components/_raid_target_picker.scss`, `tools/uitest/tests/raid/**`.

### UI-SETTINGS

The individual sim's other tabs, the header, and the generic inputs.

1. The Settings tab: buffs, debuffs, consumes, cooldowns, the encounter (presets, targets, execute ranges),
   racial traits, professions and the "Server (AzerothCore)" section. Every change survives a reload.
2. Talents: points, prerequisites, reset, glyphs, saved talents.
3. Rotation: add, remove and reorder APL actions and prepull actions, edit values, save; it survives a
   reload.
4. Every exporter's output imports back to the same settings, and each importer the page offers works.
5. The header: the settings menu (iterations and the rest) and the sim title dropdown.
6. Simulate and Stat Weights at low iterations: the results and the weights table appear.

**Owns:** `ui/core/components/individual_sim_ui/{settings_tab.ts,talents_tab.ts,rotation_tab.ts,consumes_picker.ts,cooldowns_picker.ts}`,
`ui/core/components/individual_sim_ui/apl_*.ts`, `ui/core/components/inputs/**`,
`ui/core/components/{encounter_picker,other_inputs,icon_inputs,totem_inputs,fire_elemental_inputs,server_settings_picker,importers,saved_data_manager,settings_menu,sim_title_dropdown,stat_weights_action,input_helpers,boolean_picker,enum_picker,number_picker,string_picker,number_list_picker,multi_icon_picker,dropdown_picker,base_modal}.ts`,
`ui/core/components/{exporters,sim_header,input,list_picker,icon_picker,icon_enum_picker,content_block,copy_button}.tsx`,
`ui/core/talents/**`, `ui/core/individual_sim_ui.ts`, `ui/core/encounter.ts`, `ui/<spec>/{inputs,sim}.ts`,
the scss partials for those components (`ui/scss/core/components/` and `individual_sim_ui/`, not the
other items'), `tools/uitest/tests/settings/**`.
