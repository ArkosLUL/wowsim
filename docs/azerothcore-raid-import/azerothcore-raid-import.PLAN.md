# AzerothCore raid import and per-item reforges for wowsim

## Context

The user runs an AzerothCore 3.3.5a server (`G:\DevStuff\GitHub\azerothcore-wotlk-pb`, Docker) and wants to
DPS-sim their raid group in this wowsim WotLK fork. The local sim container is `wowsims-wotlk` on :3333.
Nothing today can pull characters from a 3.3.5a server. The server runs mod-reforging, which this WotLK sim
doesn't model at all.

Outcome:
- **Phase 1:** reforges become a first-class, editable per-item property in the sim.
- **Phase 2:** a Go exporter turns the leader's raid group (or a name list) into a roster JSON.
- **Phase 3:** a Raid Sim importer loads or updates the whole raid from that roster, and an individual sim
  importer loads one character.

## Decisions (settled with the user)

**Data and export**
- Read the server's MySQL and DBC files directly. Export to a roster file. Keep the Go logic in a package that
  a sim-server endpoint can wrap later; no endpoint now.
- Select by `-leader` (their saved raid group, keeping subgroups). Fallback `-names a,b,c` fills parties of 5
  in the given order.
- Export all group members, tanks and healers included, using each character's **active** talent group.
- Race: `race` = the real race (base stats). A new `Player.racial_traits` holds the mod-racial-trait-swap race,
  any race allowed (e.g. Draenei traits on a druid). This is the model from the parity plan, pulled forward.
- Accept known inaccuracies. Item stats stay the sim DB's Classic values until the separate item-data rework.
  mod-spell-tweaks and mod-individual-progression changes aren't modeled.

**Shared models with the parity plan** (`docs/azerothcore-parity/azerothcore-parity.PLAN.md`, another session)
- That plan defines one reforge model (its P6) and `Player.racial_traits` (its P5). The user chose to build
  both here now, exactly as that plan specifies. Parity P5/P6 then reuse them. For reforges, parity P6 keeps
  `server_stats` and reads the % and stat list from P5 server settings.
- Don't edit that plan doc or any parity paths. Tell the user what landed, so they can update that session.

**Reforges**
- `ItemSpec.reforge {from_stat_type, to_stat_type}` holds **raw server ItemModType ids**. `NewItem` computes
  `floor(40% × the sim item's own from-stat)` and moves it.
- Rules match the server:
  - Both types are in {6 spirit, 13 dodge, 14 parry, 31 hit, 32 crit, 36 haste, 37 expertise}, and from ≠ to.
  - The item has the from-stat, lacks the to-stat, and the amount is ≥ 1.
  - The % and the type list are constants in one Go file and one TS file, citing `Reforging.Percentage` /
    `Reforging.ReforgeableStats` in `mod_reforging.conf`, until parity P5 adds server settings.
  - The server's "fewer than 10 item stats" rule needs raw template stats, so it waits for parity P6
    `server_stats`.
- Editable in the gear editor. Manually picking a different item clears the reforge.
- Bulk sim: a candidate without a reforge reuses the replaced item's reforge when that reforge is valid on the
  candidate.

**Raid Sim importer**
- The dialog offers **Replace** or **Update**. It defaults to Update when the raid has players, Replace when
  it's empty.
- Update:
  - Matches players by name. Refreshes gear (with reforges), talents, glyphs, race, racial traits and
    professions. Keeps rotation, consumes and spec options.
  - Re-infers the spec only when the player's top talent tree changed. If it did, the player is replaced by a
    new preset-based player of the inferred spec.
  - Moves players to their roster subgroups, adds new raiders and removes raiders not in the roster.
- Both modes keep the current encounter and raid buffs. New players get their spec preset's consumes and
  options and the Auto rotation.
- One summary alert covers: specs, missing ids, dropped reforges, added/removed/replaced players, and accuracy
  caveats.

**Individual sim importer**
- Pick one roster character, limited to the page's class. It imports what the Addon Import does, plus
  reforges, and warns when the inferred spec differs from the page.

**Also**
- Include the Raid Sim leftover-item reload fix.

## Execution order and review gates

1. **Precondition (met):** the item-diff work (`tools/database/azerothcore/`, `tools/database/acdiff/`,
   `docs/azerothcore-item-diff/`) is committed on `master` as `176e93f3b`. `azerothcore-item-diff` points at the
   same commit.
   - The untracked `docs/azerothcore-parity/` belongs to the parity session; leave it alone.
2. `git switch -c azerothcore-raid-import` from `master` @ `176e93f3b` (user-approved). Re-read the
   `azerothcore` package, since it changed at commit time.
   - Go isn't on the host PATH. Run Go builds and tests through the Dockerfile `toolchain` target
     (golang 1.23 + node + protoc), with the repo mounted at `/wotlk`.
   - Toolchain image: `wowsims-wotlk-dev`. From Git Bash:
     `MSYS_NO_PATHCONV=1 docker run --rm -v G:/DevStuff/GitHub/wowsimwotlk:/wotlk -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build -w /wotlk wowsims-wotlk-dev sh -c '...'`.
     Without `MSYS_NO_PATHCONV=1`, Git Bash rewrites `/wotlk`.
   - `node_modules` has no Windows shims, so run `npx protoc`, `npx tsc --noEmit` and `npx eslint` in the
     container too.
   - `make proto`'s TS target depends on `node_modules`. Run its commands directly: `protoc -I=./proto --go_out=./sim/core ./proto/*.proto`,
     then the three `npx protoc --ts_out ui/core/proto ...` lines from the makefile.
   - `go test ./sim/...` fails on `sim/web` (needs `binary_dist`). Test `$(go list ./sim/... | grep -v /sim/web$)` with `--tags=with_db`.
   - `make devserver` doesn't build the UI. Also run `make dist/wotlk/.dirstamp` (wasm + vite).
   - The repo uses CRLF (`core.autocrlf=true`); gofmt checks need `tr -d '\r' < f | gofmt -d`. `git status` may
     list every `.results` as modified with an empty `git diff`, which is line endings only.
   - `npm run lint:js` already fails on master. Compare eslint findings per changed file against `git show HEAD:<file>`.
   - Bash heredocs fail intermittently here. Write scripts to the scratchpad and run them.
3. Save this plan as `docs/azerothcore-raid-import/azerothcore-raid-import.PLAN.md`.
4. Run Phase 1, then Phase 2, then Phase 3. **After each phase, run its verification, then stop and ask the
   user to review before continuing.**
   - **Status:** Phases 1 and 2 are reviewed, committed and merged into `master` at the user's request, as
     `23d0796b5` and `7155b74c2`. Phase 3 not started.
5. Leave work **uncommitted** unless the user asks. Don't commit, merge, push or create other branches on your own.

## Established facts

### Server (verified against the live DB, 2026-09-17)
- **MySQL:** container `ac-database`, `127.0.0.1:3306`, `root`/`password`. DBs `acore_characters`,
  `acore_world`, `acore_auth`. SELECT only. Backtick `groups`, a reserved word in MySQL 8.
- **DBC files:** container `ac-worldserver` at `/azerothcore/env/dist/data/dbc/`. There's also a local copy
  at `A:\WOW\dbc\Changed`.
- **Freshness:** gear, talents and glyphs are saved only on character save, so run `saveall` in the
  worldserver console first.
- **Live raid:** one raid group (`groupType & 2`), leader **Deathsong**, subgroups 0–4, all members level 80.
  Every member but Deathsong is a playerbot. The group is disbanded and remade often, so its size (24–25),
  `memberFlags` (bits: 1 assistant, 2 main tank, 4 main assist) and reforge count drift; `groups` is empty
  while no group exists. The bots hold every primary profession at 450.
- **Group lookup:** `group_member(guid, memberGuid PK, memberFlags, subgroup)`. Find the row for the leader's
  guid, then load all members of that group `guid`. Warn if the named character isn't `groups.leaderGuid`.
- **`characters`:** `guid, name, race, class, level, activeTalentGroup`.
  - Class ids: 1 War, 2 Pal, 3 Hun, 4 Rog, 5 Pri, 6 DK, 7 Sha, 8 Mag, 9 Lock, 11 Dru.
  - Race ids: 1 Human, 2 Orc, 3 Dwarf, 4 NightElf, 5 Undead, 6 Tauren, 7 Gnome, 8 Troll, 10 BloodElf,
    11 Draenei.
- **Equipped gear:** `character_inventory` with `bag = 0 AND slot < 19`, skipping 3 (shirt) and 18 (tabard).
  Join `item_instance.guid = ci.item` (`itemEntry, enchantments, randomPropertyId`).
  - AC slots: 0 head, 1 neck, 2 shoulders, 4 chest, 5 waist, 6 legs, 7 feet, 8 wrists, 9 hands,
    10–11 fingers, 12–13 trinkets, 14 back, 15 main hand, 16 off hand, 17 ranged/relic.
- **`item_instance.enchantments`:** exactly 36 uint32s (12 slots × id, duration, charges).
  - `tok[0]` is the permanent enchant, a SpellItemEnchantment id; this is wowsim's `ItemSpec.enchant`.
  - `tok[6]`, `tok[9]`, `tok[12]` are socket 1–3 gem enchant ids.
  - `tok[18]` is the socket-adding enchant (3729 buckle, 3717/3723 Blacksmithing); never emit it.
- **Gems:** gem enchant → gem item via `SpellItemEnchantment.dbc` field 33 (Src_ItemID; the file has 38
  fields). All 321 raid gems map to gems present in `assets/database/db.json`.
  - Native socket count is the number of non-zero `acore_world.item_template.socketColor_1..3`.
  - If `tok[18] != 0` and the socket token at `6 + 3*nativeCount` is non-zero, that gem is the extra gem.
- **Talents:** `character_talent(guid, spell, specMask)`; keep `specMask & (1 << activeTalentGroup)`.
  - `Talent.dbc`: 0 id, 1 TabID, 2 Row, 3 Col, 4–8 RankID[5]. `TalentTab.dbc`: 20 ClassMask, 22 tabpage.
  - A spell gives (tabpage, row, col, rank). The digit goes at the index in
    `ui/core/talents/trees/<class>.json` `[tabpage].talents` whose `location {rowIdx, colIdx}` matches.
  - Trim trailing `0`s per tree and trailing `-`s.
  - Use the DBC **file only**: `acore_world.talent_dbc` holds a hunter tier swap from mod-spell-tweaks.
  - Don't use the JSON `spellIds`; they're incomplete (paladin Anticipation lists one rank).
  - Verified: all 25 members come out at 71 points. **Mighty** = `3022032023335100102012213231251-305-2033`.
- **Glyphs:** `character_glyphs(guid, talentGroup, glyph1..6)`, GlyphProperties ids; use the active group.
  - `GlyphProperties.dbc`: 0 id, 1 SpellID, 2 GlyphSlotFlags (0 major, 1 minor).
  - SpellID equals `assets/db_inputs/glyph_id_map.json` `spellId` (170 → 54733 → item 40909). The UI
    converts it with `db.glyphSpellToItemId` (`ui/core/proto_utils/database.ts:238`).
  - Ids not in the file (custom glyph 912) → warning.
- **Professions:** `character_skills(guid, skill, value)`. Every primary profession at `-minSkill` (default 1)
  or above is exported, since the server's characters hold more than the usual two: the raid's playerbots have
  the 7 crafting ones at 450 and the 3 gathering ones at 1–4. `MaxPrimaryTradeSkill = 2` in worldserver.conf
  doesn't apply to them, and mod-shared-professions shares both skills and profession spells account-wide.
  - Why the default is 1, not 450: the skill value doesn't say whether the character has the profession's
    bonus. All 25 raiders hold Toughness and Master of Anatomy however low their Mining and Skinning skill is,
    so `-minSkill 450` would drop the bonus from 21 of them. See the profession-bonus note below.
  - A gem in a wrist or hands extra socket needs Blacksmithing, which `sim.ts:180` drops it without. A belt
    buckle needs no profession.
  164 Blacksmithing, 165 Leatherworking, 171 Alchemy, 182 Herbalism, 186 Mining, 197 Tailoring,
  202 Engineering, 333 Enchanting, 393 Skinning, 755 Jewelcrafting, 773 Inscription.
- **Reforges:** `character_reforging(guid, item_guid, stat_decrease, stat_increase, stat_value)`.
  - ItemModType ids: 6 spirit, 13 dodge, 14 parry, 31 hit, 32 crit, 36 haste, 37 expertise.
  - Server config: `Reforging.Percentage = 40`, `Reforging.ReforgeableStats = 6,13,14,31,32,36,37`
    (`env/dist/etc/modules/mod_reforging.conf`).
  - The server stores `floor(40% × server item stat)`. 168 equipped reforges; 122 differ by 1–2 from 40% of
    the sim's item stat, which is accepted.
  - The server refuses a reforge into a stat the item already has (`item_reforge.cpp`).
- **Racial swap:** `character_racial_swap(guid, selected_race)`; 12 raiders have one. It swaps racial
  abilities only, and base stats stay the real race's. Example: Druidica, a Night Elf druid with Draenei traits.
- **Random property/suffix:** `randomPropertyId != 0` → warning (none today). Sim DB gap today: permanent
  enchant 3851 (Bulwark main hand).
- **Dual spec:** Agony, Dragon, Ecoterrorist, Elemena and Shadow have two talent groups. Only the active one
  is exported.

### Code to reuse
- `tools/database/azerothcore/` (from item-diff):
  - `dbc.go`: `ParseDBC`/`DBCFile.Int32`, and `CopyDBCFromContainer(container, destDir)`, which copies the
    fixed `DBCFileNames`. Add a name-list variant. Don't use `LoadDBC` here; it loads Spell/ItemSet.
  - `mysql.go`: open + Ping pattern, `coalesced()`.
  - `stats.go`: `AddItemMod`.
  - Test style: `dbc_test.go`, `stats_test.go` (table-driven, hand-built WDBC bytes).
  - `go.mod` already has `github.com/go-sql-driver/mysql`.
- `ui/raid/import_export.ts`: `RaidWCLImporter`/`WCLSimPlayer` (77-635) is the Replace-flow model.
  - `clearRaid` + `simUI.fromProto(RaidSimSettings{raid, encounter, blessings})` at 284-303.
  - The raid proto with tanks is built at 500-521.
- Presets: `playerPresets` (`ui/raid/presets.ts:58`) and new-player setup (`ui/raid/raid_picker.ts:669-686`).
- Player (`ui/core/player.ts`):
  - `readonly spec`: a spec change needs `new Player`.
  - `applySharedDefaults` 1436 (Auto rotation), `setName` 500, `setRace` 518, `setConsumes` 582,
    `setGear` 607, `setTalentsString` 828, `setGlyphs` 854, `setProfessions` 546, `setSpecOptions` 887,
    `getTalentTree` 837.
- Raid (`ui/core/raid.ts`):
  - `getPlayers()` 89 (40 entries, index = raid index), `setPlayer(eventID, index, player|null)` 112,
    `setNumActiveParties` 193, `getTanks`/`setTanks` 164-176, `clear` 239.
  - `Party.setPlayer` (`party.ts:77-106`) drops whoever is in the target slot.
  - The raid sim auto-saves to localStorage on change (`raid_sim_ui.ts:96-99`).
- Importer base: `Importer(parent, simUI, title, includeFile)` (`ui/core/components/importers.ts:32`) has
  `descriptionElem`, `textElem`, `importButton` and `onImport`. Extra controls: prepend to `this.body` like
  `IndividualLinkExporter` (`exporters.tsx:135-152`), or use `EnumPicker` (`enum_picker.ts:18`).
- `finishIndividualImport` (`importers.ts:83-136`) checks class only; always `await` it.
- Registration: individual sim `individual_sim_ui.ts:427-447`; raid sim `raid_sim_ui.ts:108-110`.
- Helpers:
  - `getTalentTreePoints` `utils.ts:300` (pad to 3 entries), `getTalentTree` `:313`.
  - `specToEligibleRaces` `:1152`, `makeDefaultBlessings` `:1846`.
  - `playerTalentStringToProto` `ui/core/talents/factory.ts:71`.
  - `Database.loadLeftoversIfNecessary` `database.ts:69`.
- **UI gotchas:**
  - Errors inside `TypedEvent.freezeAllAndDo` are swallowed, so validate and throw before it. Use one eventID
    per freeze.
  - `db.lookupEquipmentSpec` ignores array position, fills the first eligible slot, and throws
    "No slots left".
  - Sim gems: `gems[i]` ↔ `item.gemSockets[i]`, extra gem at index `gemSockets.length`
    (`equipped_item.ts:33-44`). Trim trailing zero gems.
  - Import links are hidden inside the raid sim's player Edit modal.

---

## Phase 1: shared sim models (per-item reforges + racial traits)

**Proto**
- `proto/common.proto`: `message ItemReforge { int32 from_stat_type = 1; int32 to_stat_type = 2; }` holds raw
  server ItemModType ids, and `ItemSpec.reforge = 5`.
- `proto/api.proto`: `Player.racial_traits` (type `Race`, next free field number). `RaceUnknown` means the
  player's own race.
- Regenerate with `make proto`; generated files are gitignored.

**Go: reforges**
- New `sim/core/reforging.go`:
  - `ReforgePercentage = 0.4` and `ReforgeableStatTypes = {6, 13, 14, 31, 32, 36, 37}`, citing
    `mod_reforging.conf`.
  - Map ItemModType → sim stats: 6 Spirit, 13 Dodge, 14 Parry, 31 → MeleeHit+SpellHit,
    32 → MeleeCrit+SpellCrit, 36 → MeleeHaste+SpellHaste, 37 Expertise.
    - This follows `tools/database/azerothcore.AddItemMod`, but is a local copy: `sim/core` must not import
      `tools/`.
    - The item DB stores dual ratings twice (`tools/database/wowhead_tooltips.go:225-230`).
  - `ReforgeStats(base stats.Stats, r *proto.ItemReforge) stats.Stats`:
    - Amount = `floor(0.4 × base[first sim stat of from])`.
    - Subtract the amount from every from sim stat and add it to every to sim stat.
    - Return zero stats when invalid: nil, a type not in the list, from == to, amount < 1, or the item already
      has the to-stat.
- `sim/core/database.go`:
  - Add `Reforge *proto.ItemReforge` to `Item` (36-62) and the core `ItemSpec` (119-123).
  - `NewItem` (222-258) adds `ReforgeStats(base ItemsByID stats)` into `item.Stats` and keeps `Reforge` only
    when valid. No separate reforge stats field, so every stat-summing path gets it for free.
  - Carry the field in `ProtoToEquipmentSpec` (210-220) and `ToItemSpecProto` (83-89).
  - `Item.TotalStats()` (item, enchant, gems, socket bonus) is the one per-item sum, used by
    `Equipment.Stats()` and item swaps.
- `sim/core/item_swaps.go`:
  - Carry the reforge in `toItem` (276-286).
  - The slot-swap decision (54-65) stays id-only: a same-id swap item never swaps, whatever its enchant, gems
    or reforge.
- `sim/core/bulksim.go` `createNewRequestWithSubstitution` (654-682):
  - When the old item has a reforge, the candidate has none, and `ReforgeStats(candidateBase, oldReforge)` is
    non-zero: `goproto.Clone` the candidate and set the reforge.
  - The reforge must end up in `BulkComboResult.items_added`, so the results "Equip" button applies it.
  - Dedupe by id (443-516, 686-705) can stay, since each candidate has at most one reforge.
- Tests:
  - `sim/core/database_test.go`, table-driven (the name the parity plan uses):
    - 83 crit → 33 haste (both melee and spell slots).
    - Each rejection rule; gems and enchant untouched.
    - `TotalStats` includes the socket bonus only with matching gems.
  - `sim/core/item_swaps_test.go`: swap stat changes include socket bonus and reforge; a same-id swap item
    with a different reforge adds no swap slot.
  - Bulk carry-over cases in `sim/core/bulksim_test.go` (pattern: `tinyItemDatabase` at 40-52).

**Go: racial traits**
- `sim/core/character.go`: add `RacialTraits proto.Race`, set to `player.RacialTraits`, or `player.Race`
  when unset.
  - `Character.Race` is renamed `BaseStatsRace`, used only for base stats (`character.go:150`), so a new
    racial check can't silently read the base race.
  - Racial effects switch to `RacialTraits`:
    - `sim/core/racials.go:13` `applyRaceEffects`
    - `character.go:396` Heroic Presence in `AddPartyBuffs`
    - `sim/shaman/fire_elemental_pet.go:53`
    - `sim/deathknight/ghoul_pet.go:68`
    - `sim/warlock/inferno.go:107`
  - Grep `Race_Race` again before editing, in case new uses appeared.
- Test (from the parity plan): an Orc warrior with Human traits gets Human weapon expertise and Orc base stats.
  A Night Elf druid with Draenei traits gives its party Heroic Presence.

**UI: racial traits**
- `ui/core/player.ts`:
  - `getRacialTraits`/`setRacialTraits`, a `racialTraitsChangeEmitter` added to the player's change emitters
    (like `raceChangeEmitter` at 282/333), and `getEffectiveRacialTraits()` (traits, or race when unset).
  - Emit in `toProto` (~1352). Read in `fromProto` (~1411, Miscellaneous category).
- Racial traits picker next to the race picker (`ui/core/components/individual_sim_ui/settings_tab.ts:110-120`):
  "Same as race" plus every race.
  - Include it in saved settings (`settings_tab.ts:270, 292`, new `SavedSettings.racial_traits = 19` in
    `proto/ui.proto`) and in the change listeners (`:310`).
- `IndividualSimUI.applyDefaults` and `finishIndividualImport` reset it to unset next to `setRace`.
- Racial-dependent UI uses the effective traits: `ui/hunter/sim.ts:145-148`.
  - `icon_inputs.ts` listens to `raceChangeEmitter` only for faction filtering, so it needs no traits emitter.
- Faction (`player.ts:560`) stays on `race`.

**UI model: reforges**
- New `ui/core/proto_utils/reforging.ts`, mirroring the Go constants and rules:
  - `reforgeAmount(item, fromStatType)`, `isValidReforge(item, reforge)`, `validReforges(item)`.
  - Labels per ItemModType (Spirit, Dodge, Parry, Hit, Crit, Haste, Expertise), built on `statNames`
    (`names.ts:155-196`).
- `ui/core/proto_utils/equipped_item.ts`:
  - Add a `reforge` constructor arg and field; the constructor clones it.
  - `equals` (65-87) must compare it; `setGear` skips equal gear.
  - `withEnchant` (120-122) and `withGemHelper` (127-136) pass it through, and so does every other
    `new EquippedItem(...)` copy (`withoutMetaGem`, `withoutBlacksmithSockets`, …).
  - `withItem` (92-115) clears it.
  - Add `withReforge(r | null)`, and emit it in `asSpec` (178-184).
- `database.ts` `lookupItemSpec` (178-198) reads `itemSpec.reforge`, keeping it only if `isValidReforge`.

**UI gear editor** (`ui/core/components/gear_picker.tsx`):
- Add `SelectorModalTabs.Reforging` after Enchants.
- Build it as a **custom simple tab**, not an `ItemList`: `ItemList` branches on label strings, and
  `ItemData` requires ids/phases.
  - Content: "No reforge" plus one row per `validReforges(item)`, e.g. "−17 Critical Strike → +17 Hit".
  - Selecting a row calls `gearData.equipItem(eventID, equippedItem.withReforge(r))`.
  - Show the tab only when a valid reforge exists, and rebuild it when the item changes, like the gem tabs at
    673-677. Removing the tab also drops its `gearData.changeEvent` listener.
  - The bulk tab's `BulkItemPicker` opens it from the reforge label, and falls back to it when the item has no
    enchants or gems.
  - Make sure `onShow` (521-530), which assumes `ilists[0]/[1]`, still works.
- `ItemRenderer.update` (159-211): add a "Reforged: 17 Crit → Hit" label next to the enchant, styled in
  `ui/scss/core/components/_gear_picker.scss`. It also shows in bulk results and item swap pickers.
- Not changed: the Wowhead import/export binary format (no reforge slot) and `computeItemEP` (by item id).

**Verification (Phase 1)**
1. Through the toolchain container: `make proto`, `go vet ./sim/...`, `make test`, with all 37 `.results`
   unchanged.
   - Don't use `make update-tests`; it deletes every `.results` file.
   - New Go unit tests pass.
2. On the host: `npm run type-check` and `npm run lint:js` pass.
3. Run the dev server (`make rundevserver` in the toolchain container, port published). In the individual sim:
   - Reforge a crit item to hit: the gear stats panel changes by exactly `floor(0.4 × item crit)` on melee and
     spell crit and hit.
   - Swapping to a different item clears the reforge.
   - A share link and a saved gear set keep the reforge.
4. Bulk tab: a candidate replacing a reforged item is simmed with the carried reforge, and "Equip" applies it.
5. Racial traits: an Orc warrior set to Human traits shows Human expertise with Orc base stats in the stats
   panel. In the raid sim, a druid with Draenei traits gives their party Heroic Presence.
6. **Stop for user review.**

## Phase 2: Go exporter

**Shared sim model: any number of professions** (decided with the user during Phase 2, done)
- The sim held 2 professions per player, the server holds more. They drive Mining's +60 stamina, Skinning's
  +40 crit, Herbalism's Lifeblood, Alchemy's mixology, Engineering's injectors and the Blacksmithing socket.
- `proto/api.proto`: `repeated Profession professions = 48` on `Player`. `profession1`/`profession2` stay as
  the first two, so older links and presets still load.
- `sim/core/professions.go` `ProtoToProfessions(player)` reads the list, falling back to the two fields when
  it's empty, dropping Unknown and duplicates. `Character.professions` is a slice, `HasProfession` scans it.
- `ui/core/player.ts` holds `professions` as the one source of truth, sorted so the same set always makes the
  same proto. `setProfessions` is the only setter; `getProfession1/2` read the first two slots for the compat
  fields. `playerProtoProfessions` in `proto_utils/utils.ts` mirrors the Go fallback and is what `sim.ts` uses
  for its Blacksmithing check.
- The settings tab shows one checkbox per profession instead of the two dropdowns.
- Tests: `sim/professions_test.go`.

**Files** (package `tools/database/azerothcore/` unless noted):
- `characters.go` exposes `BuildRoster(db *sql.DB, dbc *RosterDBC, trees TalentTrees, sel Selector) (*Roster, error)`.
  - Kept separate from main so a future endpoint can call it.
  - Queries are fully qualified (`acore_characters.`, `acore_world.`) and batched by guid `IN (...)`:
    group lookup or names, `characters`, equipped items joined with `item_template.socketColor_1..3`,
    `character_talent`, `character_glyphs`, `character_skills`, `character_reforging` (joined through the
    equipped item guids), `character_racial_swap`.
  - A missing optional table (reforging, racial swap) is skipped with a warning.
- `roster_dbc.go`:
  - `LoadRosterDBC(dir)` reads Talent, TalentTab, GlyphProperties and SpellItemEnchantment field 33 into maps
    via `ParseDBC`.
  - Add a copy-from-container variant that takes file names.
- `roster.go`, pure conversions:
  - `ParseEnchantments(s) (perm int32, sockets [3]int32, prismatic int32, err)` requires 36 tokens.
  - `BuildGems(...)` returns native gem item ids plus `extraGem`, and the gems it had to drop. Without an
    `item_template` row the socket count comes off the filled sockets, the last holding the added gem.
  - `BuildTalentString(...)` returns the string plus unmatched spells. Warn when points ≠ `level − 9`.
  - `SplitGlyphs`; `TopProfessions` returns English names.
  - Reforges pass through as raw ids (`stat_decrease` → `fromStatType`, `stat_increase` → `toStatType`).
    `NewRosterReforge` drops one the sim would ignore, with a warning saying which rule it broke: the stat
    types go through `core.ReforgeableStatPair`, and the rest through `core.CanReforge` against the item's
    stats in `-simDb`, since `item_template` and the sim's database can disagree.
  - Warnings: level ≠ 80, random property/suffix, unknown glyph, unmapped gem, off-hand with an empty main
    hand.
- `roster_test.go`: table tests for each function above (hand-built Talent/TalentTab WDBC bytes). Include an
  extra-gem case (2 native sockets + buckle), trailing-zero trimming and an out-of-list reforge type.
- `tools/database/acraid/main.go`, a thin main:
  - `go run ./tools/database/acraid -leader Deathsong -out raid.json`
  - Flags: `-dsn` (default `root:password@tcp(127.0.0.1:3306)/`), `-leader` and/or `-names`, `-dbcDir`
    (empty → copy from `-acContainer ac-worldserver` to a temp dir), `-trees` (default
    `ui/core/talents/trees`), `-simDb` (default `assets/database/db.json`), `-minSkill` (default 1), `-out`.
  - Prints one summary line per character.
- `tools/database/acraid/crosscheck_test.py` runs crosscheck.py against a stubbed server, so its queries and
  the subgroups it expects can be checked without one: `python tools/database/acraid/crosscheck_test.py`.

**Roster JSON (version 1)**, values illustrative:
```json
{ "version": 1, "exportedAt": "2026-09-17T18:00:00Z",
  "group": { "selector": "leader", "leader": "Deathsong", "leaderIsGroupLeader": true },
  "warnings": [],
  "characters": [{
    "name": "Bulwark", "classId": 2, "raceId": 1, "swapRaceId": 0, "level": 80,
    "subgroup": 4, "memberFlags": 3,
    "talents": "-05005135203102311333312321-502302012003",
    "glyphs": { "major": [54733], "minor": [52648] },
    "professions": ["Blacksmithing", "Jewelcrafting"],
    "gear": [{ "acSlot": 5, "id": 47112, "enchant": 0, "gems": [40119, 40119], "extraGem": 40119,
               "reforge": { "fromStatType": 13, "toStatType": 37 } }],
    "warnings": [] }] }
```
`reforge` uses the proto JSON field names, so the UI can parse it with `ItemReforge.fromJson`.
Top-level `warnings` covers the whole export, e.g. a module table the server doesn't have.
`leaderIsGroupLeader` is only present for the `leader` selector.

**Profession bonuses vs skill values on this server (checked 2026-09-17)**

All 25 raiders hold Toughness rank 6 (53040, +60 stamina) and Master of Anatomy rank 6 (53666, +40 crit
rating), whatever their Mining and Skinning skill says. Skill is therefore not the signal, and `-minSkill`
must stay at 1: raising it to 450 would drop the bonus from the 21 characters sitting at Mining 300-303 and
Skinning 1-4 who genuinely have it.

The bonuses reach a character two different ways, which is the trap:

| characters | Mining / Skinning skill | where the spells live |
|---|---|---|
| 21 playerbots | 300-303 / 1-4 | `character_spell`, granted by GM command |
| 4 human-played (Agony, Deathsong, Felesta, Nightwarrior) | 450 / 450 | `shared_professions_account_spells`, keyed by `characters.account` |

mod-shared-professions keeps its own account-level tables, `shared_professions_account_skills` and
`shared_professions_account_spells(account_id, spell_id)`, and applies the spells at login without writing
them to `character_spell`. Querying `character_spell` alone makes those four look like they have no profession
bonuses, which is wrong; in game `.learn 53040` on them answers "already knows that spell". Any future check
has to read both tables, or trust `character_skills` and `-minSkill 1` as the export already does.

The one real mismatch is Herbalism: 9 of the 25 know Lifeblood (55503), while all 25 export Herbalism, so the
other 16 would get a Lifeblood cooldown in the sim that they don't have in game. Alchemy is clean, since all
25 know Mixology (53042).

**Verification (Phase 2), re-run 2026-09-17 after the code review**

The review made acraid check reforges against `-simDb`, and taught crosscheck.py to batch its queries and to
expect the subgroups acraid really assigns, so everything below is the state after those changes. It also
moved `-minSkill`'s default to 450, which the profession-bonus note above shows was wrong for this server;
that part is reverted and the default is 1 again. Run against the live `ac-database` / `ac-worldserver` containers, with the DBCs
copied out to a scratch dir and `-dsn root:password@tcp(ac-database:3306)/`.

1. `go vet ./tools/... ./sim/...` and `go test --tags=with_db ./sim/... ./tools/...` pass (27 packages), every
   changed Go file is gofmt clean, `npx tsc --noEmit` passes, `make wowsimwotlk` builds, no `.results` file
   changed, and eslint reports nothing new per changed file.
2. `-leader Deathsong` exported the live raid: 25 characters, subgroups 0-4, 414 items, 161 reforges, 249 gems
   and 73 extra gems, every talent string at 71 points with no unmatched spells, and no warnings at all.
3. `crosscheck.py` rebuilt that export straight from SQL, the DBC files and `db.json`: no differences, in 0.8s
   over 7 queries. The same for `-leader Deathsong -names Belaria,Saeteath` (27 characters, 448 items, both
   top-ups in subgroup 5, the first with room) and for `-names` alone over 7 characters (119 items; its only
   report is the 9 random properties nothing exports, which acraid warns about too).
   `crosscheck_test.py` covers the same ground against a stubbed server, and fails if the subgroup rule or the
   batching regresses.
4. Every exported item, gem and glyph spell resolves in `assets/database/db.json`. The only ids it doesn't know
   are enchants 3851 (Bulwark's main hand) and 3776 (Belaria's and Saeteath's shoulders), which Phase 3 reports.
5. `-names` alone fills subgroups of 5 in order (5 in subgroup 0, 2 in subgroup 1 for 7 names). An unknown
   name, a repeated name, a character without a group, no selector at all and a missing `-simDb` each fail
   with their own message.
6. Every one of the 25 characters holds all 11 primary professions, and the default `-minSkill 1` exports all
   11 for each, which is what the sim needs: all 25 hold the Mining and Skinning bonuses whatever their skill,
   per the note above.
7. Not exercised on this server, so they rest on the Go tests alone: an equipped item with no `item_template`
   row (0 here), a reforge outside the server's stat list (0 of 161), and gems past the sockets a template
   declares. All 161 reforges survived the new `db.json` check, which says the item-stat lookup agrees with
   the server on real data.
8. **Stop for user review.**

**Notes for later**
- SOAP is disabled on the server (`SOAP.Enabled = 0`), so `saveall` can't be run from outside the worldserver
  console. The export is only as fresh as the server's last character save.

## Phase 3: importers and reload fix

**Shared logic** in `ui/raid/acore_roster.ts` (pure, no DOM):
- Roster types and version check. AC class/race ids → `Class`/`Race`.
- **Spec inference** from the talent string (`tab` = the tree with the most points):
  - Warrior: tab 2 → Protection, else `SpecWarrior`.
  - Paladin: 0 Holy, 1 Protection, 2 Retribution.
  - Shaman: 0 Elemental, 1 Enhancement, 2 Restoration.
  - Priest: tab 2 → Shadow, else `SpecHealingPriest` (never Smite).
  - Druid: 0 Balance, 2 Restoration. For 1: `SpecFeralTankDruid` if `thickHide == 3` or
    (`naturalReaction > 0 && protectorOfThePack > 0`), else `SpecFeralDruid`.
  - DK: `SpecTankDeathknight` if at least 2 of `toughness`, `frigidDreadplate`, `anticipation`,
    `willOfTheNecropolis` have points, else `SpecDeathknight`.
  - Hunter/Mage/Rogue/Warlock: the single spec.
  - `memberFlags & 2` forces the class's tank spec if one exists.
  - Talent ranks come from `playerTalentStringToProto`.
- **Preset:** among `playerPresets` of that spec, the smallest summed |tree points − preset tree points|.
- **Race:** `race` = `raceId`; `racialTraits` = `swapRaceId` (0 → unset), with no eligibility check.
- **Gear spec**, built in sim slot order:
  - `n` = the sim item's `gemSockets.length` (via `db.lookupItemSpec(ItemSpec.create({id}))`).
  - Native gems are truncated or padded to `n`; `extraGem` goes at index `n`; trailing zeros are trimmed.
    Warn when the server's native count ≠ `n`.
  - `reforge`: drop it with a warning when `isValidReforge` fails on the sim item.
- **Glyphs:** `db.glyphSpellToItemId` into major1–3/minor1–3; 0 → warning.
- **`applyCharacter(player, char, spec, eventID)`** sets name, race, racial traits, professions, talents,
  glyphs and gear.
  **`newPlayerFromPreset(spec, preset, sim, eventID)`**:
  - `new Player`, then `applySharedDefaults`.
  - `setSpecOptions`, `setConsumes`, distance/`otherDefaults` as in `raid_picker.ts:669-679`.
- **Placement:** raid index = `subgroup * 5 + position in subgroup`.
  `numActiveParties = max(5, maxSubgroup + 1)`.

**Raid importer** `ui/raid/acore_importer.ts`: `RaidAcoreImporter extends Importer` with file upload, plus a
Replace/Update radio prepended to the body (default as decided). Registered as 'AzerothCore' in
`raid_sim_ui.ts:108-110`. `onImport` does all async work and validation **before** the freeze:
1. Parse, `await sim.waitForInit()`, `await Database.loadLeftoversIfNecessary` on all roster items merged.
   Build per-character results (spec, preset, gear spec, warnings). A failing character is skipped and listed.
2. **Replace:**
   - Build `RaidProto` like `import_export.ts:500-521`.
   - Copy the current raid's buffs, debuffs and target dummies, and set `numActiveParties`.
   - `tanks`: main-tank-flagged players first, then other tank specs.
   - Then `RaidSimSettings{raid, encounter: current encounter proto, blessings: makeDefaultBlessings(numPaladins)}`,
     and inside one freeze: `clearRaid` + `fromProto`.
3. **Update**, inside one freeze:
   1. Snapshot the current players by name. Also snapshot each player's raid-index references (tank list, and
      spec-option targets: innervate, power infusion, focus magic, tricks of the trade, unholy frenzy) as
      player objects.
   2. For each roster character:
      - Matched with the same top tree: `applyCharacter` in place.
      - Matched but the top tree changed: `newPlayerFromPreset` with the new spec, then `applyCharacter`.
      - Unmatched: `newPlayerFromPreset` + `applyCharacter`.
   3. Clear every slot (`raid.setPlayer(i, null)`), then place the roster players at their raid indexes. This
      also re-renders stale grid tiles. Unplaced old players are removed.
   4. Rebuild tanks as in Replace. Remap the snapshotted target references to the players' new raid indexes,
      clearing any that point at removed players.
   5. Set `numActiveParties`. Keep the blessings as they are.
4. One `alert` summary: mode, per-player spec (flag inferred-vs-kept differences), added/replaced/removed
   players, missing item/enchant/gem ids, dropped reforges, roster warnings, accuracy caveats.

**Individual importer:** `IndividualAcoreImporter` in `ui/core/components/importers.ts` (or its own file),
registered in `individual_sim_ui.ts:427-447`.
- File upload, then a character dropdown limited to the page's class.
- On import:
  - Build the gear spec, race and glyphs with the shared logic.
  - `await finishIndividualImport(...)`; reforges travel on `ItemSpec`. Then set racial traits.
  - Show roster warnings and a warning when the inferred spec ≠ `simUI.player.spec`.

**Reload fix:** in `RaidSimUI.loadSettings` (`raid_sim_ui.ts:75-100`) and `RaidJsonImporter.onImport`
(`import_export.ts:56-60`), `await Database.loadLeftoversIfNecessary` on every player's merged
`equipment.items` before `fromProto`.

**Verification (Phase 3)**
1. `npm run type-check` and `npm run lint:js` pass.
2. `make rundevserver`, then Raid Sim → Import → AzerothCore with `raid.json`, **Replace**:
   - 25 players in 5 parties matching subgroups; Bulwark is the first tank as Protection Paladin.
   - The summary lists missing enchant 3851.
   - Druidica is a Night Elf with Draenei racial traits, and her party shows Heroic Presence.
   - Spot-check 3 players in the Edit modal against the game: gems (including a buckle and a Blacksmithing
     socket), reforges in the gear editor, talents, glyphs, professions, race and racial traits.
   - Run the raid sim: every DPS player does damage.
3. **Update** mode:
   - Change one player's consumes and set Innervate on a druid, drag two players to other parties, then
     re-import. Consumes are kept, players are back in their subgroups, and the Innervate target follows its
     player.
   - Edit the roster JSON to give one character a different top tree: that player is replaced with the new
     spec.
   - Delete a character from the JSON: they're removed and listed in the summary.
4. Individual sim (e.g. Retribution Paladin page): import Justice. Gear, reforges, talents and glyphs load.
   Importing Holylight warns about the spec mismatch.
5. Reload the page and confirm the imported raid is unchanged.
6. **Stop for user review.**

## Out of scope
- Sim-server endpoint for one-click import.
- Server item stats (item-data rework).
- mod-spell-tweaks and mod-individual-progression modeling.
- Random suffix items.
- Auto best-reforge from stat weights.
- Hunter/warlock pet data from `character_pet`.
