# AzerothCore vs WotLK Classic item data: investigation

The sim's item data (`assets/database/db.json`) comes from WotLK **Classic** Wowhead scrapes; its item
effects and set bonuses are hardcoded in Go with Classic values. This report compares all of that with
the user's AzerothCore 3.3.5a server (`G:\DevStuff\GitHub\azerothcore-wotlk-pb`, live DB + DBCs) and
lists what a rework to server-sourced data must change. All numbers are from the run on 2026-09-17.

## Key findings

1. **Ulduar is the only raid whose loot differs.** All 283 sim Ulduar items differ: Classic raised
   normal-mode loot by +6 ilvl and hard-mode loot by +13, with stats rescaled. Naxxramas, Eye of
   Eternity, Obsidian Sanctum, Onyxia, Trial of the Crusader, Icecrown Citadel and Ruby Sanctum loot
   match the server (their 15 differing ToC/ICC rows are sim parser gaps, plus one real stat change on
   46991 Belt of the Ice Burrower).
2. **The same +6/+13 applies to Ulduar-tier gear from other sources**: Dalaran emblem vendors, Ulduar
   crafted patterns, Furious Gladiator (S6) arena gear, and Ulduar 10-man loot that Classic moved into
   Titan Rune dungeons. 857 obtainable items change ilvl in total, Ulduar included.
3. **Hardcoded effects use Classic values** for Ulduar-tier trinkets and more (e.g. Mjolnir Runestone 751
   ArP in Go vs 665 on the server). See [Item effects](#item-effects).
4. **The sim's Wowhead tooltip parser misses some stats** the server has (block value stat 48, spell
   penetration, socket-bonus MP5, old non-rating tooltip formats). Server-sourced stats fix these.
5. **mod-individual-progression rewrote 438 TBC-era items** (ilvl 130-164) to pre-3.0 stats. Server
   truth, but irrelevant for level-80 sims.
6. Gems match the server. Enchants differ only in trivial 3.3.5 values.

## Data sources and re-running

Tool: `tools/database/acdiff/` (diff + report) on top of `tools/database/azerothcore/` (reusable reader:
MySQL `item_template`/`spell_proc`/`spell_dbc`/loot tables, WDBC reader, row → `UIItem`/`UIGem`
conversion with unit tests). Read-only: SELECTs only, never writes to the AC repo.

- Server items: live `acore_world` (base + updates + applied module SQL). Module `optional/` SQL is not
  applied on this server and is ignored.
- Server spells/sets/gems/enchants: `Spell.dbc`, `SpellDuration.dbc`, `SpellItemEnchantment.dbc`,
  `ItemSet.dbc`, `GemProperties.dbc` from the `ac-worldserver` container, with `acore_world.spell_dbc`
  rows overriding same-ID DBC spells and `spell_cooldown_overrides` replacing cooldowns (as the
  worldserver does).
- Proc rates and cooldowns: `spell_proc`, `spell_enchant_proc_data`, `item_template`
  `spellppmRate_N`/`spellcooldown_N`/`spellcategorycooldown_N` and DBC `ProcChance`/`RecoveryTime`,
  following AC's rules for which one applies (`Player::CastItemCombatSpell`,
  `Player::AddSpellAndCategoryCooldowns`). Cooldowns only count for spells an item or enchant casts;
  AC starts none for passive auras or spells they trigger.
- Sim: `assets/database/db.json`, `leftover_db.json`, Classic spell tooltips in
  `assets/db_inputs/wowhead_spell_tooltips.csv`, Go sources under `sim/`.

Run on a host with Go 1.23 and Docker (`-dsn` and `-acContainer` defaults point at the stock AC docker
setup; DBCs are copied out of `ac-worldserver` with `docker cp`; `-acRepo` is required):

```
go run ./tools/database/acdiff -acRepo G:/DevStuff/GitHub/azerothcore-wotlk-pb
```

Go isn't installed on this machine; the run used the repo's toolchain image instead (generate protos
first with `make proto` inside it if `sim/core/proto/*.pb.go` is missing; from Git Bash, prefix with
`MSYS_NO_PATHCONV=1`):

```
docker run --rm --network azerothcore-wotlk-pb_ac-network \
  -v G:/DevStuff/GitHub/wowsimwotlk:/wotlk -v G:/DevStuff/GitHub/azerothcore-wotlk-pb:/acrepo:ro \
  -v azerothcore-wotlk-pb_ac-client-data:/acdata:ro -w /wotlk wowsims-wotlk-dev \
  go run ./tools/database/acdiff -dsn "root:password@tcp(ac-database:3306)/acore_world" -dbcDir /acdata/dbc -acRepo /acrepo
```

Outputs in `data/` (diffs read server → sim):

| File | Content |
|---|---|
| `summary.md` | Counts and histograms for every section |
| `items_diff.csv` | Sim items whose server row differs; `category` = `classic` (obtainable, not touched by module SQL), `module` (touched by module/custom SQL), `unobtainable` |
| `items_not_comparable.csv` | Heirlooms (`ScalingStatDistribution`); stats come from scaling DBCs, not `item_template` |
| `items_missing_on_server.csv` / `items_unobtainable_on_server.csv` | Sim items absent from `item_template` / not in any loot, vendor, quest, achievement or create-item source |
| `items_missing_in_sim.csv` | Obtainable, equippable, rare+, ilvl 187+ server items absent from the sim |
| `effects_diff.csv` | Every item/gem/enchant spell: server values vs Go (`go_verdict`) and vs Classic tooltip (`classic_verdict`) |
| `sets_diff.csv` / `sets_issues.csv` | Set bonus spells vs Classic tooltips with Go locations / set name and membership issues |
| `gems_diff.csv` / `enchants_diff.csv` | Gem and enchant stat differences (ratings collapsed across melee/spell, since sim gem/enchant data is hand-authored) |
| `effects_review.csv` / `sets_review.csv` | Manual review of every flagged implemented effect / set bonus row: verdict (`real`, `false_positive`, `not_modeled`), Go location, Go/server/Classic values, reason. Not regenerated by the tool |

Heuristic limits: effect verdicts compare numbers, not meaning.

- `go_verdict = differs from server`: a server number isn't in the Go code around the ID, in any form Go
  uses for its kind (percent as fraction or multiplier; seconds, minutes or ms). "Around" is found by
  parsing the Go: the smallest call or composite literal holding the ID, else its statement (the whole
  function when the ID is stored in a variable), plus the uses of a package-level ID list.
- `(not registered)` suffix: the ID is in Go, but the sim's effect registry (after `sim.RegisterAll`)
  has no effect for it. It can be a coincidental number, but DK items and runes (registered per
  character) and relics checked by ID also land here.
- `classic_verdict = differs`: a server number a Classic spell tooltip can show isn't in it. PPM and
  values from `spell_proc`, `item_template` or `spell_enchant_proc_data` are never compared.

Both need the manual review summarized below.

## Items

8,043 sim items: 5 missing on the server, 39 heirlooms not comparable, 1,509 with differences
(990 classic, 438 module, 81 unobtainable). 880 have a different ilvl.

### Ilvl changes (classic category, 857 items)

| Content (sim source) | Server → sim ilvl | Items |
|---|---|---|
| Ulduar normal-mode loot | 226 → 232, 232 → 238 | 135, 26 |
| Ulduar hard-mode loot (incl. 25-man top end) | 226 → 239, 232 → 245, 239 → 252 | 43, 18, 61 |
| Ulduar 10-man loot that Classic moved to Titan Rune dungeons (on the server it still drops in Ulduar, e.g. 45282 Ironsoul from Flame Leviathan) | 219 → 225 | 109 (+10 without a sim source) |
| Dalaran emblem vendors (Valor/Conquest) | 213 → 225, 219 → 225, 226 → 232 | 4, 95, 129 |
| Ulduar crafted patterns (BS/LW/Tailoring) | 226 → 232 | 18 |
| Furious Gladiator (S6) arena gear | 226 → 232, 232 → 238, 239 → 252 | 47, 126, 27 |
| Algalon / Val'anyr quest rewards | 226 → 239, 239 → 252, 245 → 258 | 4, 4, 1 |

Stats scale with the ilvl, e.g. 45110 Titanguard: server 232 Str 37 / Sta 78 / Def 35 / Parry 33 vs sim
238 Str 39 / Sta 82 / Def 37 / Parry 35; weapon damage too.

### Same-ilvl differences (classic category)

| Kind | Items | Examples | Cause |
|---|---|---|---|
| Set name | 84 | TBC arena sets: server `Gladiator's Pursuit`, sim `Vengeful/Brutal Gladiator's Pursuit`; T8 mage: server `Kirin Tor Garb`, sim `Kirin'dor Garb` | Naming only; Go already lists both Kirin Tor spellings (`sim/mage/items.go:41`) |
| Block value | 17 | ToC tank shoulders 47720/47877 (151 → 0), shields 47835/47910 (324 → 223), ICC gun 51561 (63 → 0) | Sim parser gap: ignores item stat 48 (block value) and only reads the shield's base block |
| Socket bonus MP5 | 7 | 49329, 50784, 51006 (+3/4 mp5 → 0) | Sim parser gap |
| Spell penetration | 4 | PvP wands/off-hands 42521, 51408 (35/79 → 0) | Sim parser gap |
| Crit rating on old items | 8 | Stormshroud set, 18843 PvP fist weapons, 30318 Netherstrand Longbow (ranged crit 50) | Sim parser gap (non-`rtg` tooltip text, ranged-only rating) |
| Class allowlist | 6 | 33811-33813 Vindicator's plate: server Paladin+Warrior, sim adds Death Knight; 33876, 34014: the reverse | Real data difference (Death Knight on TBC PvP gear) |
| Real stat changes | 10 | 46991 Belt of the Ice Burrower SP 124 → 114, crit 68 → 60; 39707-39711 Verdant Tundra set has no spell power on the server; 38387-38390 hats have no armor on the server | Real data differences |
| Socket bonus | 1 | 34485 Lightbringer Girdle: server melee-crit-only bonus vs sim crit | Minor |

### Module-caused differences (438 items)

All touched by `modules/mod-individual-progression/data/sql/world/base/tbc_item_changes.sql` (435) or
`vanilla_item_changes.sql` (3); 430 are ilvl 130-164. They restore pre-3.0 itemization: spell-only haste
(server MeleeHaste 0 where the sim fills both), melee-only crit/hit, feral attack power (30883 Pillar of
Ferocity AP 1059), different primary stats. Server-sourced data will carry these as-is.

### Obtainability

- **Missing on server (5):** Classic-only Hallow's End items 211817, 211844, 211847, 211850, 211851.
- **Unobtainable on server (202 sim items, 148 at ilvl 200+):** Savage/Hateful Gladiator (S5) weapons,
  shields and relics (not sold by any vendor in AC's data); unreleased items with Cataclysm-build rows
  (`VerifiedBuild` 15595, e.g. 37174 Rippling Azure Cloak, 34144 Branch of Destruction; their stats are
  placeholders); unreleased Conqueror's T8.5 boots 46326-46335; a few Classic-only sources.
- **Server gear missing from the sim (4):** 37254 Super Simian Sphere, 45994 Lost Ring, 45995 Forgotten
  Necklace, 49227 Skoll's Fang.
- **Heirlooms (39):** stats come from `ScalingStatDistribution.dbc`/`ScalingStatValues.dbc` at level 80;
  `item_template` holds none of them.

## Gems and enchants

- **Gems:** all 366 sim gems match the server's stats and colors, except 33633 Forceful Earthstorm Diamond,
  which isn't a gem on the server (no `GemProperties`).
- **Enchants:** 225 sim enchants; differences (server → sim):
  - Real, trivial 3.3.5 values: 2583 Presence of Might block value 30 → 15; 2656 Boots - Vitality
    health and mana every 5 sec 5 → 4; 3150 Chest - Restore Mana Prime MP5 7 → 6; 2658 Surefooted also
    gives +10 crit on the server; 3722/3728/3730 tailoring embroideries carry a hidden +1 Spirit.
  - Not a difference: scopes 2523, 2724, 3607, 3608 grant ranged-only ratings; the sim models them in Go
    rather than as enchant stats.

## Item effects

`effects_diff.csv` has 855 spells on sim items, gems and enchants. 76 rows (71 items/gems/enchants) are
flagged `differs from server`. All of them, plus 40 unflagged items, were reviewed against Go, DBC,
`spell_proc` and AC scripts (`data/effects_review.csv`): 50 items/gems/enchants have real differences
(55 findings). 43 of them are flagged; Val'anyr and the six Ashen Band rings only came up in review. The
rest are false positives (value in a helper or another file, bookkeeping numbers, scripted spells
matching) or aspects the sim doesn't model. Rows marked `matches server` or `not in sim Go` were not
reviewed beyond those 40.

### Ulduar-tier trinkets and relics use Classic's rescaled values (Go → server)

| Item | Aspect | Go | Server | Go location |
|---|---|---|---|---|
| 45931 Mjolnir Runestone | ArP proc | 751 | 665 | `sim/common/wotlk/stat_bonus_procs.go:405` |
| 45518 Flare of the Heavens | SP proc | 959 | 850 | `stat_bonus_procs.go:349` |
| 45522 Blood of the Old God | AP proc | 1358 | 1284 | `stat_bonus_procs.go:337` |
| 45609 Comet's Trail | haste proc | 819 (Classic 768) | 726 | `stat_bonus_procs.go:370` |
| 46038 Dark Matter | crit proc | 692 (Classic 647) | 612 | `stat_bonus_procs.go:417` |
| 45866 Elemental Focus Stone | haste proc | 552 | 522 | `stat_bonus_procs.go:382` |
| 45286 Pyrite Infuser | AP proc | 1305 | 1234 | `stat_bonus_procs.go:314` |
| 45490 Pandora's Plea | SP proc | 794 | 751 | `stat_bonus_procs.go:326` |
| 45535 Show of Faith | MP5 proc | 272 | 241 | `stat_bonus_procs.go:360` |
| 45929 Sif's Remembrance | MP5 proc | 220 | 195 | `stat_bonus_procs.go:395` |
| 45263 Wrathstone | AP on use | 905 | 856 | `sim/common/wotlk/stat_bonus_cds.go:60` |
| 45148 Living Flame | SP on use | 534 (Classic 505) | 505 | `stat_bonus_cds.go:86` |
| 45292 Energy Siphon | SP on use | 431 | 408 | `stat_bonus_cds.go:87` |
| 45466 Scale of Fates | haste on use | 457 | 432 | `stat_bonus_cds.go:43` |
| 45158 Heart of Iron | dodge on use | 457 (Classic 432) | 432 | `stat_bonus_cds.go:142` |
| 45313 Furnace Stone | armor on use | 5448 | 5152 | `stat_bonus_cds.go:122` |
| 46021 Royal Seal of King Llane | parry on use | 402 | 380 | `stat_bonus_cds.go:150` |
| 45308 Eye of the Broodmother | SP per stack | 26 | 25 | `sim/common/wotlk/stat_bonus_stacking.go:223` |
| 46051 Meteorite Crystal | MP5 per stack | 85 (Classic 79) | 75 | `stat_bonus_stacking.go:331` |
| 45703 Spark of Hope | base mana cost reduction | 44 | 42 | `sim/core/mana.go:301` |
| 45509 Idol of the Corruptor | agility proc; Bear Mangle proc chance | 162; 50% | 153; 100% | `sim/druid/items.go:419`, `:422` |
| 45270 Idol of the Crying Wind | Insect Swarm bonus | 396 SP (~79/tick) | 374 flat damage over the DoT (~62/tick, AC `spell_dru_insect_swarm`) | `sim/druid/insect_swarm.go:15` |
| 45114 Steamcaller's Totem | Chain Heal bonus | 257 | 243 | `sim/shaman/heals.go:320` |
| 45144 Sigil of Deflection | dodge proc | 144 | 136 | `sim/deathknight/items.go:621` |
| 45254 Sigil of the Vengeful Heart | Death Coil; Frost Strike bonus | 403; 218 | 380; 205 | `sim/deathknight/items.go:300`, `:304` |
| 46017 Val'anyr | absorb duration; stacking | 30s, each heal re-applies | 8s (Classic 8s), heals add into one shield capped at 20000 | `sim/common/wotlk/other_effects.go:318`, `:330` |

### Proc rates, cooldowns and other mechanics (Go → server)

| Item / enchant | Go | Server | Go location |
|---|---|---|---|
| 50401/50402/52571/52572 Ashen Band of Vengeance/Might (ICC ring AP proc) | 1 PPM | flat 10% chance (`spell_proc` 72413), 60s ICD | `stat_bonus_procs.go:599`, `:612`, `:651`, `:664` |
| 50403/50404 Ashen Band of Courage | 60s ICD | no `spell_proc` row → 3% chance, no ICD | `stat_bonus_procs.go:626`, `:639` |
| 47661 Libram of Valiance | 15s Strength buff | 16s | `sim/paladin/items.go:411` |
| 32410 Thundering Skyfire Diamond | 1.5 PPM | 0.7 PPM | `sim/common/tbc/metagems.go:75` |
| 25901 Insightful Earthstorm Diamond | 4% | 5% | `metagems.go:58` |
| Enchant 3251 Giant Slayer | 4 PPM, 237 damage | 3 PPM (`spell_enchant_proc_data`), 237-323 | `sim/common/wotlk/enchant_effects.go:22`, `:34` |
| Enchant 3239 Icebreaker | 4 PPM | 3 PPM (`spell_enchant_proc_data`) | `enchant_effects.go:66`, `:99` |
| Enchant 3273 Deathfrost | 2.15 PPM on melee hits | 3 PPM (`spell_enchant_proc_data`) | `sim/common/tbc/enchant_effects.go:192`, `:218` |
| Enchant 3241 Lifeward | 300 heal (marked stand-in) | 310-356 | `enchant_effects.go:223` |
| Enchant 3370 Rune of Razorice | 2% of average base weapon damage | 2% of weapon damage including the AP bonus (`Spell::EffectWeaponDmg`) | `sim/deathknight/items.go:331` |
| 28830 Dragonspine Trophy | 1.0 PPM | 1.5 | `sim/common/tbc/melee_trinkets.go:78` |
| 32505 Madness of the Betrayer | 1.0 PPM | 3 | `melee_trinkets.go:143` |
| 11815 Hand of Justice | flat 1.33% | 1 PPM, one third at level 80 | `melee_trinkets.go:37` |
| 32375 Bulwark of Azzinoth | flat 2% | 6 PPM | `sim/common/tbc/melee_items.go:301` |
| 19019 Thunderfury | 6 PPM | 8 (`item_template.spellppmRate_1`, set by mod-individual-progression) | `melee_items.go:26` |
| 29996 Rod of the Sun King | 1.0 PPM | 6 (`item_template.spellppmRate_1`) | `melee_items.go:178` |
| 40322 Totem of Dueling | 60 haste only | also +120 AP for 6s (spell 60766 effect 2) | `sim/shaman/stormstrike.go:86` |
| 42598 Furious Gladiator's Totem of the Third Wind | 338 flat heal | 320 spell power | `sim/shaman/heals.go:34` |
| 33506 Skycall Totem | 101 haste | 100 | `sim/shaman/items.go:70` |

### Sim bugs regardless of server

- 40258 Forethought Talisman: HoT total 3752 is a digit swap of 3572 (server and Classic), and it procs
  only on crits where both say any direct heal (`sim/common/wotlk/other_effects.go:225`, `:237`).
- Living Flame, Heart of Iron, Comet's Trail, Dark Matter, Meteorite Crystal: Go matches neither the
  Classic tooltip nor the server (see table above).
- Totem of the Third Wind's bonus is also added to Healing Wave (`sim/shaman/heals.go:185`).

Not modeled by the sim (same on server and Classic, no action): absorb caps of Essence of Gossamer
(4000) and The General's Heart (5200), Thunderfury's attack-speed slow, Jade Pendant of Blasting's party
buff, Idol of the Raven Goddess's Tree of Life bonus, Crusader's self-heal, stun/disarm/silence parts of
meta gems and DK runes.

Tool gaps found in review:

- Values in a helper the effect calls (Darkmoon Card: Greatness, Deathfrost's spell procs) show as
  missing from Go.
- Spells applied by scripts or area auras aren't in the trigger chain, so their values are missing
  (Shifting Naaru Sliver's buff 45044) or never compared (Val'anyr's shield 64413).
- A server number can match an unrelated Go number, since Go literals carry no kind: Ashen Band of
  Vengeance/Might's 10% chance matches the buff's 10s duration while Go uses 1 PPM.
- Only server values are looked up in Go, not the reverse, so a Go-only ICD (Ashen Band of Courage)
  isn't flagged.

## Set bonuses

No set is split differently: 10- and 25-man tier pieces share one `ItemSet` on the server, as the sim
assumes. Of 409 set bonus spells, 75 implemented ones were flagged (Classic tooltip differs, or shows
none of the server's values) and reviewed against the Go (`data/sets_review.csv`); 71 were false
positives (ms shown as seconds, rage/runic power in tenths, placeholder values, rounding, proc rates
matching Go). Set bonus Go code isn't compared automatically, so the two PPM rows below came from review,
not from a flag. Real differences (Go → server):

| Set bonus | Go | Server | Go location |
|---|---|---|---|
| Kirin Tor Garb (T8 mage) 4pc: chance to keep Missile Barrage / Hot Streak / Brain Freeze | 20% (`T84PcProcChance = 0.2`) | 10% (spell 64869) | `sim/mage/items.go:77` |
| Liadrin's / Turalyon's Battlegear (T9 ret) 2pc: Righteous Vengeance crits | plain melee crit multiplier | server spell 67188 also adds +100% crit damage to Righteous Vengeance (61840) | `sim/paladin/talents.go:589` |
| The Fists of Fury 2pc: fire proc rate | 2.0 PPM | 1.5 PPM (`spell_proc` 41989) | `sim/common/tbc/melee_sets.go:33` |
| The Twin Blades of Azzinoth 2pc: haste proc rate | 1.0 PPM | 2 PPM (`spell_proc` 41434) | `sim/common/tbc/melee_sets.go:138` |

Not implemented in the sim (both Classic and the server have them): Siegebreaker Plate 4pc (NYI; its
comment says 10%, both sources say 20%), Ymirjar Lord's Plate 4pc (NYI).

Set names: 35 TBC arena sets are named without the season on the server (`Gladiator's Pursuit` vs
`Vengeful/Brutal Gladiator's Pursuit`), and T8 mage is `Kirin Tor Garb` on the server vs `Kirin'dor Garb`
from Wowhead. Server-sourced set names must still match `core.ItemSet.Name`/`AlternativeName` in Go.

## Implications for the rework

1. **Item stats from the server.** Add a `gen_db -gen=azerothcore` step (or merge pass) that overwrites
   ilvl, quality, stats, sockets, socket bonus, weapon damage/speed, heroic, class allowlist and set name
   with `azerothcore.ConvertItem` (`tools/database/azerothcore/convert.go`). This fixes the 857 ilvl
   changes, the real stat changes and the parser gaps at once. Keep Wowhead/AtlasLoot for icons, phases,
   item/armor/weapon types and sources. Not converted yet: `unique`, required profession, faction
   restriction, per-item PPM and cooldowns (read into `azerothcore.ItemSpell`, but `UIItem` has no field
   for them).
2. **Heirlooms:** compute level-80 stats from `ScalingStatDistribution.dbc` + `ScalingStatValues.dbc`
   (not read yet) or keep the Wowhead values.
3. **Obtainability:** decide whether to drop or flag the 202 sim items that can't be obtained on the
   server (`azerothcore.LoadObtainableItemIDs`), and whether to add the 4 missing server items.
4. **Gems:** no change needed. **Enchants:** optionally update the five trivial values in
   `tools/database/enchant_overrides.go`.
5. **Item effects and set bonuses stay in Go:** apply the Go → server value changes in the tables above
   (26 Ulduar-tier rows, 19 proc-rate/mechanic rows, 4 set bonuses) and fix the listed sim bugs.
   Longer term, generating proc amounts from server spell data would stop this drift.
6. **Set names:** server names differ for 35 TBC arena sets and T8 mage; Go matches sets by
   `ItemSet.Name`/`AlternativeName`, so either normalize server names to the Go names or add aliases.
7. **Module-driven changes:** server-sourced stats will pull mod-individual-progression's pre-3.0
   itemization into the sim for 438 TBC-era items (spell-only haste, melee-only crit, feral AP), which the
   sim's stat model may not represent faithfully (e.g. feral AP counted as plain AP).
8. **Verify** by re-running `acdiff` after the rework: `items_diff.csv` should keep only unobtainable
   rows, and `effects_diff.csv` flagged rows should drop to the known false positives.

## Not covered

- Classic vs 3.3.5 tuning of class talents, abilities and mechanics in the sim's Go code. Classic changed
  many of these independently of items; this needs its own investigation.
- Loot source attribution in the UI (zone/boss/difficulty filters still come from AtlasLoot Classic,
  e.g. Ulduar 10-man loot is labeled as Titan Rune dungeon drops).
- Item phases (Classic release phases) versus the server's progression.
