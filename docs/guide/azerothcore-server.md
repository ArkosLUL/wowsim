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
  files match across all three copies.

## Config

- `[ac]/configurationOverrides/*.env` sets `AC_*` vars, which override keys in
  `[ac]/env/dist/etc/**/*.conf` (e.g. `modules/mod_reforging.conf`).
- Live config changes need the user's OK.

## Modules that affect the sim

| Module | On the server | In the sim |
|---|---|---|
| mod-individual-progression | Its `data/sql/world/base/{tbc,vanilla}_item_changes.sql` is applied (438 TBC-era items at pre-3.0 stats). Its `optional/` SQL isn't. It has a damage penalty below tier 13 | items come from the server DB ([ADR 0002](../adr/0002-item-data-from-live-db.md)); the penalty is ignored |
| mod-reforging | moves 40% of one stat into another; stats 6, 13, 14, 31, 32, 36, 37; never into a stat the item already has | `ItemSpec.reforge` |
| mod-racial-trait-swap | swaps racial abilities, not base stats or faction | `Player.racial_traits` |
| mod-shared-professions | shares profession skills and spells per account, applied at login | any number of professions |
| mod-dungeon-scale | raid creature stat multipliers; live boss health ×1.2 | server setting ([ADR 0004](../adr/0004-server-config-as-settings.md)) |
| mod-spell-tweaks | spell and talent changes, listed in the parity INVESTIGATION; swaps two hunter talent tiers | toggles become server settings; talents keep stock tree positions |
| mod-playerbots | bots fill the raid; strategies in `acore_playerbots.playerbots_db_store` | none |
| mod-chronicle | combat logs, inside instances only. The server deletes its copy after upload, so download from the app API | recorded runs |
| mod-sim-validation | ours, and its own git repo. `.simval` GM commands, test dummies 999000–999003, output in `[ac]/env/dist/logs/simval/`, `Enable = 0` by default | validation ([testing.md](testing.md#against-the-server)) |

The other installed modules (transmog, npc-enchanter, token-turnin, mount-scaling, …) only touch a few
items.

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
- Character deletes commit asynchronously, so poll for them. `CharacterCache` refuses a delete from
  another account.
- AzerothGhost, the e2e client, needs the Warden OS FourCC patch, plus `E2E_WORLD_ADDR` because the
  realmlist gives the public address.
- GM command loops freeze the world thread, so cap their iterations (simval's `MaxIterations`).
