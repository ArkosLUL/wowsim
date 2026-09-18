# wowsimwotlk: AzerothCore fork

A WotLK DPS simulator, forked from wowsims/wotlk, reworked to reproduce one private AzerothCore 3.3.5a
server exactly.

## Language

### Sources of truth

**Server**:
The user's live AzerothCore 3.3.5a realm: its code, DB, DBCs, modules and config. The sim's only source
of truth.
_Avoid_: realm, backend, AC (for the live realm)

**Classic**:
WotLK Classic (3.4 client data, Classic hotfixes, Wowhead Classic items): what upstream wowsims models
and this fork replaces.
_Avoid_: retail

**Retail**:
Original Blizzard 3.3.5a behaviour.

**Retail deviation**:
A point where the server differs from retail. The sim follows the server and the deviation is logged.

**Parity**:
The sim matching the server within the agreed tolerances.
_Avoid_: accuracy, correctness

**Server settings**:
Server config values the sim exposes as settings defaulting to the live config.
_Avoid_: server constants

### Work

**Effort**:
A named body of work with its own branch, docs directory and owned paths.
_Avoid_: workstream, project, session

**Phase**:
A separately reviewed step of an effort, named by ID (P2, Phase 3). IDs aren't execution order.

### Verification

**Golden**:
A committed `.results` baseline that a sim test compares against.
_Avoid_: snapshot, fixture

**Promote**:
Copying a suite's reviewed `.results.tmp` over its golden.

**Simval**:
mod-sim-validation, the server module that measures mechanics on the live server and records them for
replay against the sim.

**Spelldump**:
Simval's export of server spell data.

**Recorded run**:
A real fight against a server test dummy, logged by mod-chronicle, that the sim must match within ±2% DPS.

### Raid import

**Roster export**:
A JSON snapshot of a raid group's characters as the server stores them.
_Avoid_: character import (that's the UI side)

**Raider**:
A character in the exported group.

**Human-played character**:
A raider the user plays; profession spells come from the account.
_Avoid_: player (clashes with the sim's Player)

**Playerbot**:
A raider driven by mod-playerbots.
_Avoid_: bot account

### Server modules

**Reforge**:
A mod-reforging change that moves 40% of one item stat into another, per item instance.

**Racial traits**:
The race whose racial abilities a character uses (mod-racial-trait-swap). Distinct from **race**, which
sets base stats and faction.

**Shared professions**:
mod-shared-professions: profession skills and spells shared by all characters on an account.

**Dungeon scale**:
mod-dungeon-scale's multipliers on raid creature stats.
