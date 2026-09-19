# AzerothCore server

The live server: where it runs, how to reach it, what's installed. For table and DBC layouts, see
[azerothcore-data.md](azerothcore-data.md).

## Source and containers

- [ac] is what runs: `azerothcore-wotlk-pb`, branch `Custom`, origin ArkosLUL/azerothcore-wotlk.
  `G:\DevStuff\GitHub\azerothcore-wotlk` (a mod-playerbots fork) is **not** the live server.
- [ac]'s `AGENTS.md` governs work there:
  - don't build or configure unless asked
  - SQL changes only go in `data/sql/updates/pending_db_*`
  - plans go in `.agents/plans/`
  - credit upstream authors
- Containers, from [ac]'s docker compose:

  | Container | Details |
  |---|---|
  | `ac-database` | mysql 8.4 on `127.0.0.1:3306`, stock `root`/`password`; DBs `acore_world`, `acore_characters`, `acore_auth`, `acore_playerbots` |
  | `ac-worldserver` | :8085. 7878 is published, but SOAP is off |
  | `ac-authserver` | :3724 |
  | `ac-db-import`, `ac-client-data-init` | one-shot |

  - Network: `azerothcore-wotlk-pb_ac-network`.
  - mod-chronicle's app runs alongside, as `chronicle-app-1` on :4000.
- There's no mysql CLI on the host. Query with
  `MSYS_NO_PATHCONV=1 docker exec ac-database mysql -uroot -ppassword -N -e "…"`, and name the database
  in each query.
- Rebuild, only with the user's OK: `cd [ac] && docker compose build ac-db-import ac-worldserver &&
  docker compose up -d`. It's ready when "World Initialized" appears in `docker logs ac-worldserver`.
- Character data reaches the DB only when a character saves. Before reading it, ask the user to run
  `saveall` in the worldserver console. There's no SOAP, and don't `docker attach`.

## DBCs

- The live copies are at `ac-worldserver:/azerothcore/env/dist/data/dbc/`, and in volume
  `azerothcore-wotlk-pb_ac-client-data`: mount it `:ro` and read `<mount>/dbc`. Windows can't browse
  the volume.
- Copy files out with `MSYS_NO_PATHCONV=1 docker cp ac-worldserver:/azerothcore/env/dist/data/dbc/<file> <dir>`.
- `A:\WOW\dbc\Clean` holds stock 3.3.5a; `A:\WOW\dbc\Changed` holds the user's edited client. The gt*.dbc
  files match across all three copies. `A:\WOW\dbc\Server` isn't the live copy (its Spell.dbc differs), so
  copy DBCs out of the container when they must match the server.

## Config

- `[ac]/configurationOverrides/*.env` sets `AC_*` vars, which override keys in
  `[ac]/env/dist/etc/**/*.conf` (e.g. `modules/mod_reforging.conf`).
  - The container copies each conf.dist there once and never updates it. The installed
    `spell_tweaks.conf` lacks FeralSpiritHaste, RuptureWeaponExpertise and RendTrauma, so those run on
    their code defaults (on).
  - Env names follow `IniKeyToEnvVarKey`, with quirks: `StatModifierRaid25M` → `RAID_25_M`,
    `Raid10MHeroic` → `RAID_10_MHEROIC`, `DKGhoul` → `DKGHOUL`.
  - `MapUpdateInterval` is 100 ms live (`ServerPerformance.env`; conf.dist says 10).
  - `tools/acore/gen_server_defaults` turns the live config into the sim's defaults.
- Live config changes need the user's OK.

## Modules that affect the sim

| Module | On the server | In the sim |
|---|---|---|
| mod-individual-progression | Gates content by [progression tier](#progression-tiers). Its `data/sql/world/base/{tbc,vanilla}_item_changes.sql` is applied (438 TBC-era items at pre-3.0 stats). Its `optional/` SQL isn't. It has a damage penalty below tier 13 | items come from the server DB ([ADR 0002](../adr/0002-item-data-from-live-db.md)); item availability per tier from the server catalog ([ADR 0005](../adr/0005-server-item-catalog.md)); the penalty is ignored |
| mod-reforging | moves 40% of one stat into another; stats 6, 13, 14, 31, 32, 36, 37; never into a stat the item already has | `ItemSpec.reforge` |
| mod-racial-trait-swap | swaps racial abilities, not base stats or faction | `Player.racial_traits` |
| mod-shared-professions | shares profession skills and spells per account, applied at login | any number of professions |
| mod-dungeon-scale | raid creature stat multipliers; live boss health ×1.2 | server setting ([ADR 0004](../adr/0004-server-config-as-settings.md)) |
| mod-spell-tweaks | spell and talent changes, listed in the parity INVESTIGATION; swaps two hunter talent tiers | toggles become server settings; talents keep stock tree positions |
| mod-playerbots | bots fill the raid; strategies in `acore_playerbots.playerbots_db_store` | none |
| mod-chronicle | combat logs, inside instances only. The server deletes its copy after upload, so download from the app API | recorded runs |
| mod-sim-validation | ours, and its own git repo. `.simval` GM commands, test dummies 999000–999003 (mod-dungeon-scale's `DisabledID`, so never scaled), output in `[ac]/env/dist/logs/simval/`. `Enable = 0` by default; `configurationOverrides/SimValidation.env` turns it on live. Every `.simval` command refuses while it's off | validation ([testing.md](testing.md#against-the-server)) |
| mod-npc-enchanter | NPC 601015 applies nearly every WotLK enchant free, at any tier. Profession enchants check skill == 450; Hyperspeed Accelerators checks Engineering == 400, so a 450 engineer can't buy it there | enchants count as available in every phase |

The other installed modules (transmog, token-turnin, mount-scaling, …) only touch a few items.

## Progression tiers

mod-individual-progression stores a character's tier as rewarded quest `66000 + tier`.

| Tier | Opens | Reached by killing |
|---|---|---|
| 13 | Northrend, its 5-mans, Naxx 533, OS 615, EoE 616 | – |
| 14 | Ulduar 603 | Kel'Thuzad |
| 15 | ToC 649, ToC5 650 | Yogg-Saron |
| 16 | ICC 631 and FoS 632, which gates PoS 658 and HoR 668 | Anub'arak |
| 17 | RS 724 | the Lich King |

- Map gates: `IndividualProgressionPlayer.cpp`. VoA phases its bosses in (`IndividualProgression.cpp`):
  Archavon 13, Emalon 14, Koralon 15, Toravon 16. Argent Tournament spawns carry `phaseMask` 65536 and
  phase in at 15 (`checkIPPhasing`).
- Bug: level-80 Onyxia (249) has no tier gate, so any level-80 character enters. The user counts her as
  tier 15, which is what the BiS catalog does.
- Vendors:
  - `IndividualProgressionAwareness.cpp` hides emblem vendors below their tier: 33963/33964 below 14,
    35494/35495/35573/35574 below 15, 37941/37942/38858 below 16.
  - `conditions` type 23 rows needing quest `66000 + N` gate single vendor items at tier N. Harold Winston
    (32172) sells the epic gems at 15. `wotlk_vendors.sql` gates Timothy Jones' Jewelcrafting designs (the
    epic cuts, Nightmare Tear) the same way, at 15.
- Emblems (`data/sql/world/base/wotlk_emblems.sql`): Heroism and Valor 13, Conquest 14, Triumph 15, Frost 16.
  A few trickle in earlier (Sartharion's Satchel of Spoils holds a Triumph at 13, and Usuri Brightcoin trades
  Triumph down); the catalog keeps these tiers as floors.
- Epic gems also drop from Titanium Ore prospecting (reference 13005) with no tier condition, so they're
  obtainable at 13.

## Raid characters

- The raid group is led by Deathsong: 25 raiders in subgroups 0–4. It's disbanded and remade often.
- The user plays Agony, Deathsong, Felesta and Nightwarrior (accounts 515 = Felesta, 516 = the other
  three). Every other raider is a playerbot.
- Every raider holds all 11 primary professions and has Toughness r6 and Master of Anatomy r6. Skill
  values don't show who has a bonus ([azerothcore-data.md](azerothcore-data.md#professions)).
- Herbalism in an export doesn't mean the raider knows Lifeblood (55503): only some do. The sim still
  gives every herbalist Lifeblood. Either the user `.learn`s it, or the difference is accepted.

## Server-dev gotchas

- `PSendSysMessage` prints nothing to a console session. Use `SendSysMessage` + `SetSentErrorMessage`.
- Relog after `.maxskill`, or crit isn't recomputed.
- Characters leveled by GM command never learn trainer spells, Parry (3127) included: `.learn` them.
- mod-individual-progression refuses death knights (`CHAR_CREATE_DISABLED`) until a character on the
  account reaches progression tier 12: `.ip set <name> 12` one and log it out first.
- `.additem <id> -1` destroys the item even when it's equipped.
- Character deletes commit asynchronously, so poll for them. `CharacterCache` refuses a delete from
  another account.
- AzerothGhost, the e2e client, needs the Warden OS FourCC patch, plus `E2E_WORLD_ADDR` because the
  realmlist gives the public address.
- GM command loops freeze the world thread, so cap their iterations (simval's `MaxIterations`).
