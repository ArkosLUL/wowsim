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

**Wave**:
One round of up to four work items, run in parallel and merged together. The orchestrator stops after each.
_Avoid_: batch, sprint

**Work item** (WI):
One reviewable piece of an effort, run in a wave on its own branch and worktree.
_Avoid_: task, ticket

**Integration branch**:
`integration`, which collects each wave's work items before they reach `master`.

**Orchestrator**:
The session that runs the waves and does all their git.

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
A mod-reforging change that moves a share of one item stat into another, per item instance. The share
and the stats it may touch are server settings, 40% live.

**Racial traits**:
The race whose racial abilities a character uses (mod-racial-trait-swap). Distinct from **race**, which
sets base stats and faction.

**Shared professions**:
mod-shared-professions: profession skills and spells shared by all characters on an account.

**Dungeon scale**:
mod-dungeon-scale's multipliers on raid creature stats.

### BiS optimizer

**BiS**:
The best items, gems, enchants, reforges and racial traits for a character in a content phase, as the
optimizer finds them.
_Avoid_: best gear, optimal set

**Content phase**:
One of the five cumulative raid releases: 1 Naxx/EoE/OS, 2 Ulduar, 3 ToC, 4 ICC, 5 Ruby Sanctum.
Content phase N is progression tier 12+N.
_Avoid_: bare "phase", which is an effort step; patch

**Progression tier**:
mod-individual-progression's per-character unlock level. Tiers 13–17 cover the WotLK raids.
_Avoid_: bare "tier", which can mean tier gear

**Catalog**:
The server-derived record of which items and gems are obtainable, from which progression tier and source,
with their equip limits.
_Avoid_: item phases, loot list

**Candidate pool**:
The items, gems and enchants one optimization may choose from.
_Avoid_: item list

**Raid contribution**:
A raider's effect on the raid's total DPS: their own damage plus what they give others.
_Avoid_: utility, raid value
