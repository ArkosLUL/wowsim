# Stat caps

The caps the optimizer holds and the Caps & Priority tab shows, the tab's stat values and its reforge
suggestions (`sim/statcaps`, `sim/statslopes`, `sim/optimizer/caps.go`,
`ui/core/components/individual_sim_ui/caps_tab.ts`, `ui/core/stat_priority.ts`).

## Caps

`sim/statcaps` bisects each cap on the player's character sheet (`core.PlayerSheet`, through
`PlayerSheetAt`), so race, talents, buffs and target debuffs count. The role comes from the spec (`RoleOf`):

| Role | Caps |
|---|---|
| Melee | hit; dual wield white hit (soft, only when the off hand swings); expertise dodge, plus parry when in front; ArP 100%; spell hit when a rotation spell does spell damage (`CastsSpellDamage`) |
| Hunter | hit; ArP 100%; haste ceiling where Steady Shot casts in 1 s |
| Caster | spell hit; haste ceiling where the GCD reaches 1 s |
| Tank | hit; expertise dodge and parry; Defense for crit immunity (`RequiredDefense`), only when the boss swings at them |
| Healer | the GCD haste ceiling |

There is no crit cap.

- **Hit:** yellow at `PhysicalMissChance` 0. White at `WhiteMissChance` 0, the main hand's swing with the dual
  wield penalty; warriors get a note that a queued Heroic Strike or Cleave drops the off hand's penalty.
- **Spell hit:** the server misses only on `irand(1, 10000) < threshold`, so a 1 bp threshold never misses:
  446 bare, 367 under Misery. The optimizer's floor still asks for 0 bp, sometimes one rating more.
- **Expertise:** the cap is the yellow table's, 212 for dodge and 460 for parry (459 rating truncates to
  1399 bp). The note gives the white swing's lower point (192, 440): the white table takes the level gap in
  after expertise.
  - Main- and off-hand rows only when the hands' own expertise differs: a one-hand racial lands on that hand's
    spells (Orc axe main hand, sword off hand).
  - A target that never parries gets no parry row.
- **ArP:** `ArmorPenPercent` at 100 after percent auras, stances included: Arms (Battle Stance 10%, Endless Rage
  14%) needs 1064, Fury (Berserker Stance) 1400.

### Gear-side frame

Caps read as gear rating, like the in-game sheet, from the sheet's `PlayerStats` snapshots:
- frame = base + talentsΔ + buffsΔ of the stat; consumables count as gear;
- yours = final − frame; needed = the bisected sheet cap − frame. Fury with Precision 3/3 and Heroic Presence
  needs about 131, not 262.

Needed at or below 0 reads "already reach it". An unreachable cap returns needed +Infinity and an "Out of reach"
note, not an error.

Each cap lists its sources in rating and %, zero rows dropped:
- the bare cap, "Level N target": the core table function (`MeleeMissBP`, `SpellMissBP`, `YellowMeleeTableBP`)
  at no bonus on `BareTable`, the target's table with nothing of the player's; ArP's is "100% armor penetration";
- race; a one-hand racial as "<race> <weapon>: <hand> only";
- talents; buffs, named "Heroic Presence" on hit when it's on; target debuffs, named from the debuff settings;
- the remainder, a rating or more: "Talents on your attacks/spells" (Shadow Focus, Weapon Mastery), "Stances and
  talents" for ArP.

Haste ceilings carry only a note: the cast speed and the GCD or Steady Shot time. Defense lists race, talent
and buff rows only, since crit reductions aren't rating.

`/statCaps` (`StatCapsRequest` → `StatCapsResult`, `statcaps.ComputeRequest`) is synchronous and caps the raid's
first player. Caps bisect in parallel goroutines; a spec takes milliseconds.

### In the optimizer

`sim/optimizer/caps.go` and `critimmunity.go` wrap `statcaps` under their old names; `dpsCapRole` maps a healer
to none. The expertise floor stays stat-based, which always equals the worse hand.

Unless `disable_stat_caps` is set ("Respect hit, expertise and haste caps" unticked), `dpsCapFloors` sets the
caps before the pool compiles, each bisected on the sheet so talents, racials and the run's buffs count: melee
get hit and expertise floors, hunters a hit floor and a haste ceiling, casters a spell hit floor and a haste
ceiling, tanks the hit floor below.

- The haste ceiling is a hard rule, a floor's mirror (`RuleCeiling`): verify, the gem check and neighborhood
  adoptions keep it, and a seed over it is broken. Haste past it can still pay (a caster whose casts stay above
  1 s), so the search's penalty alone doesn't hold it.
- Where nothing meets every cap, `closestToCaps` keeps the loadout missing them by the least rating, each cap
  relaxed only back to what the settings set themselves, and the run warns.

A tank run's only cap floor is melee hit (`capRoleTank`): enough to bring `PhysicalMissChance` to 0, 262 rating
bare. No expertise floor. It stays there by the user's call: the tanks glyph their taunts. A tank run without
its class's taunt glyph warns with the miss chance at the floor (`warnTauntGlyph`).

That isn't a taunt cap. Taunt 355, Growl 6795, Dark Command 56222 and Hand of Reckoning 62124 are
`DmgClass MAGIC`: they roll spell hit (17% miss on a boss, 26.23 rating a point) and are never dodged or
parried. The floor's 262 rating is 10 points of it and the taunt glyphs' 8 cover the rest. Unglyphed, a taunt
caps at 446 rating, 367 under Misery or Improved Faerie Fire.

## Stat values

### Shipped

`tools/statweights` measures centered slopes at each phase's BiS gear, and `publish.py` writes them to
`assets/statweights/stat_values.json` ([README](../../tools/statweights/README.md)). `ui/core/stat_values.ts`
loads it.

- **Build:** `chooseBuild` matches the spec, the main tree (`player.getTalentTree()`) and a two-hand flag when
  the main hand is two-handed (`dk_unholy_2h`). With no build for the tree, the spec's first, flagged.
- **Stats as gear carries them** (`GEAR_STATS`): hit, crit and haste rating = melee + spell slope; attack power =
  AP + RAP; weapon DPS.
- **Phase picker:** follows `sim.getPhase()` until the user picks one. `caps-tab.v1` stores `{phase, metric}`.
- **Metric:** tanks switch Survival (TMI, first) / Threat (TPS). Ratios are to `epReferenceStat`; Survival's to
  the stat weights tool's tank reference (`tankRefStat`, default armor), since AP and SP barely move TMI.
  Healers have no shipped values.
- **Cap rule:** melee hit, spell hit, expertise and ArP are measured below the cap. Past every hard cap on the
  stat (both hands, dodge and parry) more is worth 0 and reads "N over: reforge away". Hit with one part capped
  keeps the other's value ("capped for attacks (N over)"). A dual wielder past yellow but short of white reads
  "white swings only: measure at your gear". Defense and haste are measured past their caps at BiS and keep
  their value.

### Live

"Measure at my gear" runs `/statSlopesAsync` (`statslopes.MeasureAsync`) through `/asyncProgress`:
- **Steps:** `OneSidedSteps`, the centered deltas with no shift. Gain = (sim(+δ) − sim(0)) / δ; loss =
  (sim(0) − sim(−low)) / low, where low stops at the stat's final value, so ArP never goes below 0. At 0,
  loss is unmeasured.
- **Healer role:** Int, Spirit, SP, MP5, spell crit and spell haste, in HPS.
- **Sims:** one seed (the page's `nextRngSeed()`: repeatable only with a fixed seed), `IsTest`, the page's
  iterations. Each runs single-threaded through `core.RunRaidSim`, one per core minus one: IsTest sims don't
  recover their panics, so `runOne` does and the server survives.
- **No cap rule:** the run measured at the player's gear.
- **Tab:** once a run exists, a "Phase N BiS / My gear" switch; My gear disables the phase picker and adds a
  "Losing one" column. Values go stale once the page's raid or encounter proto no longer equals the run's, and
  aren't saved across reloads.

## Reforge suggestions

`suggestReforges`, per equipped item:
- **Candidates:** `validReforges` minus the current reforge, plus "remove" when reforged. `reforgeDelta` is zero
  when the server settings refuse the reforge, like `core.ReforgeStats`. `REFORGE_TYPE_STATS` copies
  `reforgeStatTypeToStats` (`sim/core/reforging.go`): change both together.
- **Score:** amount × value of the net change once the current reforge comes off.
  - Shipped values take the cap rule on both ends: added rating counts up to the hard cap's room; removed rating
    costs its value below the soft cap too, so white-only hit is never spent.
  - Live values score added rating at gain and removed at loss.
- **Picks:** greedy, best gain first; each spends the surplus or room later ones are scored against.
- **Floors**, for shipped and live values alike: every `StatCapFloor` cap with a finite `needed` (hit, spell
  hit, each expertise row, Defense). Surplus = `round(rating) - round(needed)`; no pick takes a floor below
  `min(0, surplus)`: a met cap stays met, a short one never gets shorter. A candidate moves a floor by its net
  change on the cap's `stat` (generic hit moves melee and spell hit). Soft caps and ceilings aren't floors.
- **Noise:** a pick's gain must exceed 1e-6 and be at least 5% (`MIN_RELATIVE_GAIN`) of the value it gives up.
- **Apply:** `gear.withEquippedItem` and `player.setGear`, the gear picker's path; Apply all is one gear change.
  Rows disable from any change until fresh caps arrive.

## Apply on server

The "Your character" block's "Apply reforges on server…" button (sim server only, hidden under wasm) opens
`acore_reforge_dialog.ts`, which sends the sim's reforges to a core-raid character on the live server.
- mod-reforging reads `character_reforging` only at startup, so the change goes through the GM command
  `.reforge apply <character> <slot>:<item>:<from>:<to> …` over SOAP, never a DB write
  ([route](dev-environment.md#run), [server side](azerothcore-server.md#modules-that-affect-the-sim)).
  Slots are server `EQUIPMENT_SLOT_*` ids; `0:0` removes.
- The dialog lists the page's class from a fresh `fetchServerRoster()`, preselecting the importer's saved pick
  (`acore-import.v1`). A row per equipped slot (`acSlotToItemSlot`) shows the server reforge as of the
  character's last save and the sim's, with a status:
  - `Will change` or `Same`: sent;
  - left out: different item on the server, nothing on the server, nothing in the sim, or a sim reforge
    the live server can't take.
  - Reforges are checked against `LIVE_REFORGING`, never the page's server settings: with reforging off
    there, Send would strip every reforge.
- Send posts every `Will change` and `Same` slot to `POST /acoreReforge`, then each row shows the server's
  `<status>: <text>` (`applied`, `removed`, `unchanged`, `mismatch`, `invalid`, `failed`) and whether the
  character was online or offline.
- Online, the item is re-reforged live. Offline, the stored reforge is written and applied at next login.
  A bot redoes its own reforges when it refreshes its gear.
- Tests: `sim/web/acreforge/handler_test.go` (against an `httptest` SOAP stub),
  `tools/uitest/tests/caps/server_reforge.spec.ts` (stubbed `/acoreRoster` and `/acoreReforge`).

## The tab

- `CapsTab` sits after the BiS Optimizer on every individual sim page, never in the raid sim.
- It refreshes only while shown, debounced, dropping stale replies, on the player's stats, gear, buffs, talents
  and in-front setting, party and raid buffs, debuffs, tanks and the encounter.
- "Import from roster…" calls `IndividualSimUI.openAcoreImporter()`, as the Import menu does; it shows only on a
  sim server (`simServerAvailable()`).
- Tests: `sim/statcaps/statcaps_test.go`, `sim/statslopes/statslopes_test.go`, `tools/uitest/tests/caps/`.
  `TestRequestsSideBySide` runs both RPCs beside `/computeStats` and `/raidSimAsync`, as the sim server does;
  after a concurrency change run `dock.sh test -race ./sim/statcaps/ ./sim/statslopes/`.
