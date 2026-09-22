# BiS Tooltip addon, served from the AzerothCore server

## Context

The user wants a BiS tooltip addon for their AzerothCore 3.3.5a realm showing **their server's** BiS
data, produced by the wowsim optimizer, instead of Wowhead's. Two properties the current addon lacks:

1. **Data lives on the server**, so no one hand-copies Lua files to stay current.
2. **BiS accounts for the raid**, not a character in a vacuum. The roster is static: 25 raiders led by
   Deathsong, the only character the user still plays; the other 24 are playerbots.

Outcome: hover an item anywhere in the client and see, from live server data, which of your raiders
wants it, in which phase, at what rank — plus the generic per-spec lists.

Everything in this plan was settled with the user in a design interview. Where a decision looks
arbitrary, it was made deliberately; the reasoning is kept where it changes how you'd implement.

## What already exists (reuse, don't rebuild)

**The transport is proven on this exact server.** `mod-multibot-bridge` is a C++ module talking to a
3.3.5a addon over `CHAT_MSG_ADDON`: `MBOT\t` envelope, `~` field separator, capability handshake,
255-byte wire cap. Copy its shape.

- Send: `ChatHandler::BuildChatPacket(data, CHAT_MSG_WHISPER, LANG_ADDON, player, nullptr, wire)` then
  `player->SendDirectMessage(&data)` —
  [MultiBotBridge.cpp:10930-10955](G:/DevStuff/GitHub/azerothcore-wotlk-pb/modules/mod-multibot-bridge/src/MultiBotBridge.cpp)
- Module skeleton: `src/mod_multibot_bridge.cpp` (the `AddSC` shim) + one implementation `.cpp` +
  `conf/*.conf.dist`. No `CMakeLists.txt` — AzerothCore's module glob picks it up.
- 3.3.5a has **no** `RegisterAddonMessagePrefix`. `CHAT_MSG_ADDON` fires for every prefix, args
  `(prefix, message, channel, sender)`. MultiBot relies on this
  ([MultiBotComm.lua:675-690](A:/WOW/world%20of%20warcraft%203.3.5a%20hd/interface/addons/MultiBot/Core/MultiBotComm.lua)).

**The sim already produces the data shapes.**

- `proto/optimizer.proto`: `OptimizerResult` (best `EquipmentSpec`, `alternatives` = top 3 per slot
  with `score_delta`, `sim_commit`, `catalog_date`), `OptimizerBatchExport` / `OptimizerBatchEntry`.
- `sim/optimizer/raidctx/derive.go` (merged, wave C) derives a player's raid buffs, party buffs,
  debuffs and replenishment **from the actual roster composition** — the mechanism behind feature 2.
- `cmd/wowsimcli/cmd/optimize.go`: `wowsimcli optimize --infile <OptimizeGearRequest.json> --outfile
  <OptimizerResult.json>`.
- `tools/database/acraid/`: roster JSON v1 — raider names, class, subgroup, talents. Source of subject
  identity.

**The addon is a good base.** [Bistooltip](A:/WOW/world%20of%20warcraft%203.3.5a%20hd/interface/addons/Bistooltip)
is MIT, Ace3-based, with a "Data source" selector (`Config.lua`), talent-based spec detection
(`Bislist.lua:47-116`), owned-item checkmarks, an `enhs` renderer for enchant/gem icons, and a
tooltip hook already doing the reverse lookup we need (`searchIDInBislistsClassSpec`,
`Bistooltip.lua:53-81`). Not a git repo; carries local fixes (DataStore source lookup, shift-compare,
modifier-key refresh) worth keeping.

## Facts established during design (don't re-derive)

- **Progression tier** is rewarded quest `66000 + tier`
  ([IndividualProgression.cpp:20-27](G:/DevStuff/GitHub/azerothcore-wotlk-pb/modules/mod-individual-progression/src/IndividualProgression.cpp)).
  The module reads it server-side and puts it in `META`; no client-side quest probing.
- **Reforges cannot render natively.** mod-reforging works by rewriting
  `SMSG_ITEM_QUERY_SINGLE_RESPONSE` ([item_reforge.cpp:640](G:/DevStuff/GitHub/azerothcore-wotlk-pb/modules/mod-reforging/src/item_reforge.cpp)),
  which only applies to item instances that exist. A BiS recommendation's reforge is hypothetical, so
  the addon draws that text itself from stat ids.
- **The realm has 53,558 accounts and 21 guilds**, twenty of them 15-member playerbot guilds. Guild 21
  *Yebateli Online* (5 members) is the real one. Virtually every account is a bot that will never run
  this addon — which is why the protocol is pull-only.
- **Optimizer score J is in reference-stat points** (AP, SP, or Armor for survival;
  `sim/optimizer/objective.go`), so a slot's delta travels as a rounded integer.
- **Every enchant in the sim DB has a spell id**, 136 of 226 an item id too. The wire carries the spell,
  which the addon shows as a spell link.
- **acraid's roster has no GUIDs**; acbis looks them up by name (`-dsn`).
- **The addon's lists** have one `Finger` and one `Trinket` entry, separate `Weapon` and `Off hand`
  entries, and `Ranged` or `Relic` (paladin, DK, shaman, druid). Phase keys are `PR T7 T8 T9 T10 RS`;
  content phases 1-5 map to `T7`..`RS`. Its `enhs` layout is the enchant or a `none` placeholder, then
  the gems with a `none` between each two.

## Sequencing

The wave loop is at **wave F**. From
[bis-optimizer.PLAN.md](../bis-optimizer/bis-optimizer.PLAN.md):

| Wave | Item | Gives this project |
|---|---|---|
| G | `BIS-batch-ui` | the 25-raider × 5-phase batch and its `OptimizerBatchExport` |
| H | `BIS-raid-contrib` | the raid-contribution objective — genuinely roster-aware BiS |
| K | `BIS-presets` | `p{1..5}_bis` gear sets per spec, a cheaper source for spec subjects |

Nothing blocks. The exporter consumes `OptimizerResult` / `OptimizerBatchExport`, which exist today
(wave A). Seed the first dataset from per-subject `wowsimcli optimize` runs. When G and H land,
re-run the export — **the module and addon are unchanged**, only the numbers improve. The dataset
records its `objective` so the addon can label pre-H data as own-metrics.

**Healers are out of scope for the optimizer.** No Holy, Discipline, Restoration or Holy Paladin
blocks will exist. Those specs fall back to the shipped Wowhead lists, labelled as not-yours.

## Architecture

```
wowsim repo                 [ac] repo                      WoW client
-----------                 ---------                      ----------
wowsimcli optimize          mod-bis-tooltip                 BisTooltipAC addon
   |  OptimizerResult          |  blocks loaded into RAM       |  HELLO (cached dataset version)
   v                           |                          <-- |  META + SUBJ
tools/database/acbis  --sql--> acore_world tables         --> |  WANT (missing block ids)
   |                           |  frame queue, N/tick      --> |  BLK xN, DONE
   v                           |                              |  decode -> SavedVariables (global)
bis_dataset.sql                                               v
  (imported by hand)                                   Bistooltip_server_bislists
                                                       + Bistooltip_server_roster
```

Three repos, one dataset. **The exporter never writes to the live DB** — it emits a `.sql` file the
user imports, because the live DB is SELECT-only per `CLAUDE.md`.

### Subject and dataset model

A **subject** is either a roster raider or a class/spec. A **block** is one (subject, content phase).

- Roster subjects are keyed by **character GUID** (stable across renames) and carry the name for
  display. Every raider with results is one, playerbots included — that is the loot question. Ordered
  by raid index (subgroup × 5 + position in acraid's order), so the list matches the raid frame. No
  special-casing of human-played characters.
- A raider's spec is named after their main talent tree (most points, first on a tie); the main tank
  flag picks `Blood tank` and `Feral tank`. Builds within a tree (Fury-Prot) read as the tree's spec.
- Spec subjects are the sim's non-healer specs under the addon's spec names (`Bistooltip_spec_icons`
  keys — mind the oddities `Fire FFB`, `Blood dps`, `Fury-Prot`, `Destruction fire`). A result naming
  an unknown spec is skipped with a warning, not a failure.
- The dataset carries a **composition fingerprint** over the whole roster, healers included: the
  sorted (class id, main talent tree) multiset. Not names — swapping one Fury warrior for another
  changes no buffs — and not spec names, which the addon can't read off a group member; the tree it
  can, by inspect. In a raid of 10+, once it has every online member's tree, the addon compares it to
  theirs and on mismatch shows a one-line warning in the roster section header while still showing the
  data. Degraded data beats none. Offline members give no buffs and can't be inspected, so they're left
  out and named in the warning.

### Wire format (frozen)

Every message is `BIST\t<op>~<field>~...`, at most 255 bytes including `BIST\t`. `acbis`'s
`bisdata_wire.go` is the reference for the payload, BLK framing, checksum and fingerprint; the module
and the addon must match it.

| Op | Dir | Fields |
|---|---|---|
| `HELLO` | C→S | protocol version (1), addon build (integer), cached dataset version (empty for none) |
| `META` | S→C | dataset version, sim commit, catalog date, objective, fingerprint, viewer tier (mod-individual-progression's, 0 for none), subject count, block count |
| `SUBJ` | S→C | dataset version, then subject entries joined with `;` |
| `WANT` | C→S | dataset version, block ids joined with `,`, chunked to fit |
| `BLK` | S→C | dataset version, block id, `<seq>/<total>` (seq from 1), payload chunk |
| `DONE` | S→C | dataset version, block id, checksum (`-` for no such block). `DONE~<version>~*`, with no checksum, closes a WANT |
| `ERR` | S→C | code (`protocol`, `disabled` also for no dataset, `denied`), message |
| `NOTE` | S→C | message, e.g. the update nudge |

- **Subject entry:** `<id>,<kind>,<classId>,<specName>,<phases>[,<raidIndex>,<guid>,<name>]`. Kind 0 is
  a roster raider, 1 a spec; the last three fields are roster-only. `phases` lists the content phases
  that have a block, e.g. `1245`. Class ids are AzerothCore's (1 warrior .. 11 druid).
- **Block id** = subject id × 10 + content phase.
- **Checksum:** Adler-32 of the payload as 8 lowercase hex digits (`a=1, b=0`; per byte
  `a=(a+byte)%65521, b=(b+a)%65521`; `b*65536+a`). `"Wikipedia"` → `11e60398`.
- **Fingerprint:** the checksum of `<classId>:<tree>` per raider, sorted byte-wise (Lua 5.1's `<` follows the
  locale, so the addon compares bytes), joined with `,`;
  tree is the inspected talent tab with the most points (first on a tie), minus 1.
- **Dataset version:** 8 hex digits of a SHA-256 over everything the addon caches, excluding the export
  time, so re-exporting unchanged results doesn't trigger a sync.
- **Sim commit:** 12 hex digits plus any suffix such as `-dirty`.

Block payload: the filled slots in slot order, joined with `;`. Each slot:

```
0:51227,50712,50713,51866(e59954,g41398,g40111,r31-37)+142
^ ^                       ^      ^      ^      ^       ^
| up to 4 ranked item ids |      gems in socket order  rank 1's delta over rank 2, may be "+-3"
proto ItemSlot 0-16       enchant spell        reforge, stat 31 -> 37
```

Rings and trinkets are separate slots (10/11, 12/13), each with its own ranks. Enhancements are rank
1's only, in the order enchant, gems, reforge, each optional; so is the delta, which needs a rank 2.
Four ranks per slot: the optimizer's best plus its 3 `OptimizerSlotAlternative`s — matching the source
exactly, so there's no truncation policy to regret. Gems and enchants reuse the addon's `enhs` icon
renderer; the reforge becomes a plain text line. Only the rank-1 delta travels, so the tooltip can say
what the slot is worth over its runner-up; full per-item deltas wait until wave H, because pre-H
deltas are own-metrics and would mislead.

**Sizing,** measured on a real fury P1 result (17 filled slots): 716 B and 4 frames per block. 25
raiders × 5 phases = 125 roster blocks, up to 30 non-healer addon specs × 5 = 150 spec blocks: ~275
blocks, ~200 KB. A cold sync fetches them all: ≈ 1,100 frames at 10 per `SendIntervalMs` (100 ms), **~14
s** from HELLO in the addon harness's model of the module; healers have no blocks, so less in practice.

**No compression in v1.** Plain decimal stays readable on the wire, which makes bugs in a
two-language codec far cheaper to find, and a ~14 s sync once per dataset change isn't worth buying
complexity to fix. Two levers if the measured number annoys: base-36 item ids (~20% off) or raw
deflate with LibDeflate client-side (~55% off, at the cost of a printable-encoding layer and a second
way for the codecs to disagree).

### Protocol behaviour

**Pull-only, with one deliberate exception.** The module sends nothing until an addon says `HELLO`,
so a login storm costs zero for the ~53k bot accounts. The exception: a reload (`.bistooltip reload`,
`.reload config`) sends one message to each addon that said HELLO *this login* and whose state changed:
`META` when it's newly served or served a new dataset version, the `ERR` when it no longer is. So an
eight-hour login doesn't sit stale after a re-import, and the property pull-only was protecting holds.

**Sync triggers:** `HELLO` on every login and `/reload` (2 frames when the version matches, the
common case), plus a manual "Sync now" button.

**Failure semantics — the block is the atomic unit.** A bad checksum or a sequence gap re-requests
*that block* once, then marks it failed and keeps everything else; never fail a whole sync over one
block. Completed blocks persist to cache as they land, so a disconnect resumes by requesting only
what's missing. Every `SUBJ`, `BLK` and `DONE` carries the dataset version; the addon discards frames
tagged with anything other than what `META` announced, then re-handshakes. A WANT naming a stale version
gets only `DONE~<current version>~*`, which triggers exactly that.

**Three version numbers, kept distinct** because they change for unrelated reasons: the **protocol
version** (mismatch refuses with `ERR~protocol`), the **addon build** (drives the `NOTE` update nudge
only), and the **dataset version** (drives cache invalidation only).

**Access is open, rate-limited.** BiS lists aren't secret; the real risk is load, not disclosure, and
a config allowlist would mean editing a conf and restarting to add a friend. Limits: one `HELLO` per
session per 10 s, a cap on blocks in flight per session, 10 frames/tick per session, and a global
frame ceiling across all sessions so one syncing client can't starve the world thread. A config
switch can restrict to a guild id or account list later if it ever matters.

## Execution: sessions and gates

**One phase at a time**, via the phase loop ([workflow](../guide/workflow.md)). This is **not** a
wave-loop effort: the phases are a dependency chain, not parallel work items, so the wave loop's
machinery would cost more than it buys.

| Session | Phase | Repo | Ends on |
|---|---|---|---|
| 1 | Phase 1 — exporter + wire codec | wowsim | Go tests — fully verifiable without the user |
| 2 | Phase 2 — server module | [ac] | compiles; **user's OK** to rebuild the worldserver |
| 3 | Phase 3 — addon transport + cache | addon | **user's** cold-sync / warm-login check in the client |
| 4 | Phase 4 — tooltip | addon | **user's** in-client hover check. Shippable here |
| 5 | Phases 5–6 — browser, real data, docs | all three | **user's** SQL import + final hover check |

**The freeze point.** The wire format above is frozen with Phase 1. The module and the addon's decoder
are written against it; changing it later means touching three repos and bumping the protocol
version.

**Gates that need the user**, none of which a session may do on its own:

- rebuilding the worldserver ([azerothcore-server](../guide/azerothcore-server.md)) — a hard rule in `CLAUDE.md`
- importing the dataset `.sql` — the live DB is otherwise SELECT-only
- any in-client verification, and the ~245 optimizer runs that produce real data
- `git init` on the module and addon folders, creating the effort branch, and every commit or merge

**Picking this up cold.** A fresh session should read this PLAN, then `git branch --show-current` and
`git status` in whichever repo the session's phase names (sessions run concurrently in this project).
Sessions 2 and 3 touch entirely different repos and could be run in parallel if you change your mind
about sequencing; sessions 4 and 5 both touch the addon and must not overlap.

## Work

### Phase 1 — Exporter (`wowsim`, branch `bis-tooltip-addon`)

**Status:** done. Committed on `bis-tooltip-addon` and merged into `master`, then reviewed there.

A **thin wowsim effort** owning only `tools/database/acbis/` and `tools/database/azerothcore/bisdata*.go`,
off `master`, **not** in the wave loop — the module and addon are their own repos, and a client-facing
artifact shouldn't be coupled to an in-flight sim. How to run it:
[acbis README](../../tools/database/acbis/README.md).

- `bisdata_wire.go`: `EncodeBlock`/`DecodeBlock` over `BisBlock`, `BlockFrames`, `BisChecksum`,
  `CompositionFingerprint`.
- `bisdata_dataset.go`: `BuildBisBlock` (an `OptimizerResult` to a block; enchant effects to spells via
  the sim DB), `BuildBisDataset` (subjects, blocks, fingerprint, version), the input loaders.
- `bisdata_slots.go`: the addon's class, spec, slot and phase names; `RaiderSpecName`.
- `bisdata_sql.go`: the import, which creates the tables if missing and swaps the whole dataset in one
  transaction (`DELETE` + `INSERT`: `REPLACE INTO` would leave a dropped subject's rows behind);
  `CharacterGUIDs`.
- `bisdata_lua.go`: `-luaOut`, the dataset in the addon's decoded shape, the contract the Lua decoder is
  checked against. Spec subjects go to `Bistooltip_server_bislists` in `Bistooltip_wotlk_bislists`'s
  shape; raiders to `Bistooltip_server_roster[guid]` with `name`, `class`, `spec`, `raid_index`,
  `phases`. A slot's delta and reforge sit in an `extra` table, so the slot keeps only the shipped
  lists' keys.

`acbis` **consumes only** — it never runs the optimizer. A thin separate driver script orchestrates
the ~245 runs, keeping `acbis` pure and testable, and letting wave G's batch export feed it directly.
Regeneration is **manual**: one documented command, run when you want it, never coupled to the wave
loop's re-baseline. Don't commit the payload; record the dataset version, sim commit, catalog date
and objective in this PLAN so any dataset on the server traces back to a sim state.

**Live dataset:** `b235db6c`, a placeholder: one fury P1 result fanned out to every subject and phase.
Sim `unknown`, catalog 2026-09-19, objective `own`, imported 2026-09-21.

Tables in `acore_world` (DDL: `BisTablesSQL`):

| Table | Columns |
|---|---|
| `bistooltip_dataset` | `version`, `sim_commit`, `catalog_date`, `objective`, `fingerprint`, `exported_at` |
| `bistooltip_subject` | `id`, `kind` (0 roster / 1 spec), `guid`, `name`, `class_id`, `spec_name`, `raid_index` |
| `bistooltip_block` | `subject_id`, `content_phase`, `payload` TEXT, `checksum` |

**Verified:** `go test ./tools/database/azerothcore/` passes: round-trip over every slot, malformed
payloads, the frame cap and reassembly up to 134 frames, the checksum's known value, dataset ordering,
skips and errors, batch stages, and the SQL and Lua output. End to end, one real `wowsimcli optimize`
result fanned out to the live roster's 25 raiders and 30 specs × 5 phases gave 55 subjects, 275 blocks
and 1,100 frames; the `.sql` imported twice cleanly into a throwaway MySQL 8.4, and the `-luaOut`
parsed into the expected tables.

### Phase 2 — Server module (`[ac]/modules/mod-bis-tooltip`)

**Status:** done. Its own git repo like mod-sim-validation, on `master`, not pushed. Setup, config, commands and the
server's side of the protocol: `[ac]/modules/mod-bis-tooltip/README.md`.

- `src/BisTooltipWire.*`: the Go codec's C++ twin (checksum, `BlockFrames`' chunking, `SUBJ` packing, field checks).
- `src/BisTooltipDataset.*`: builds every served message from the table rows. Any bad row rejects the whole dataset.
- Both are standard library only, so `test/` builds them without the server.
- `src/BisTooltipBridge.cpp`: loads the dataset at startup (`OnLoadCustomDatabaseTable`) and on `.bistooltip reload`;
  chat hooks; per-player queues.
- `data/sql/db-world/base/`: `BisTablesSQL` verbatim, and the command rows.

Changed from the plan:

- `MinAddonVersion` is `MinAddonBuild`, as HELLO calls it. The conf is `mod_bis_tooltip.conf.dist`, like
  mod-sim-validation's. `.bistooltip status` is new.
- `SendIntervalMs` (100) paces the queues: `WorldScript::OnUpdate` runs every world loop (`MinWorldUpdateTime`, 10 ms
  live), not per 100 ms map tick.
- No logout hook: bots fire it from map threads. Logged-out characters are pruned every world tick instead.
- BIST messages that aren't a self-whisper are swallowed, so no client can fake replies to another.
- A HELLO in another protocol gets `ERR~protocol` whatever its other fields.
- A reload only disturbs addons whose state changed (see Protocol behaviour); re-importing the same version restarts
  no sync.

**Verified:** `test/run-tests.sh` passes under g++ 13 and clang 18. Checksums and frame splits match the Go codec's on
the same inputs, and 18 malformed datasets are rejected. Phase 1's export, loaded into a throwaway MySQL over the module
SQL (applied twice), loads whole: 55 subjects, 275 blocks, 1,100 BLK frames as acbis counted, each within 255 bytes and
reassembling to its payload; 1,384 messages send it all. `test/syntax-check.sh` compiles `src/` against current [ac]
headers with the last build's flags plus `-Werror`. `codestyle-cpp.py` passes. A separate reviewer checked the
threading, hook, load-order and packet claims against the core source. Live, `ac-db-import` applied both SQL files and
the worldserver logs `mod-bis-tooltip: no dataset loaded`, then "World Initialized".

### Phase 3 — Addon fork: transport and cache

**Status:** done. `BisTooltipAC` is its own git repo in the AddOns folder, on `master`, not pushed: the
original Bistooltip as installed, then this phase, then Phase 4. Options, sync and tests: its `README.md`.

- A fork of [Bistooltip](A:/WOW/world%20of%20warcraft%203.3.5a%20hd/interface/addons/Bistooltip), which
  stays installed but disabled. MIT attribution kept in `README.md` and `LICENSE`, plus the fork's line.
- The TBC and Classic lists are dropped: the realm is WotLK-only. The shipped Wowhead WotLK list stays
  unchanged as the offline and healer fallback. A baked server snapshot would go stale, compete with the
  live dataset for authority, and put you back to hand-distributing data.
- `Bistooltip_server.lua`: the codec, the sync, and the API the options and tooltip use. It decodes into
  exactly what `acbis -luaOut` writes. The cache sits in `BisTooltipDB.global.server`, so one install
  syncs once for every character on it.
- `Config.lua`: the `Server (live)` data source, the default, plus "Sync now" and a status line. A
  fallback is said in chat and in the status line.

Changed from the plan:

- HELLO is retried 12 s apart, 3 times, then the addon falls back. A sync with no progress for 15 s
  handshakes again, up to 3 times, then reports it stalled.
- `SavedVariables` is still `BisTooltipDB`, the original's name. With both addons enabled, the client
  writes one table into both files, so the fork inherits the original's per-character data source. The
  fork warns in chat when both are enabled.

**Verified:** `test/run-tests.sh` runs the addon's real code under Lua 5.1, with the WoW API and Ace
stubbed, against a model of the module's session logic serving messages the module's own C++ built;
decoded tables equal `acbis -luaOut`. In the client, on the live dataset: a cold sync, a warm login at
one message each way, resuming a sync cut off by logout without refetching, and switching source both
ways. `Enable = 0` is checked in the harness only.

### Phase 4 — Tooltip (the shippable milestone)

**Status:** done, in the addon repo.

- `Bistooltip.lua`: a roster section, "BiS for your raid", above the spec section. Each line reads
  `<class icon> <Raider> (<spec>)`, then `T8 BIS +142 / T9 alt 2` on the right, the delta being rank 1's.
  Under it goes a dim sub-line per reforge, named like mod-reforging's `ItemReforge::StatTypeToString`.
  With the server selected, spec lines from the shipped list are marked with a grey `(Wowhead)`. Hide specs
  and Highlight spec now apply, and Alt shows everything.
- Phases are shown up to the one after the viewer's: `min(5, max(0, tier - 12) + 1)`, since tier 13 opens
  T7 and 17 opens RS. `PR` always shows, and a nil or 0 tier shows all. "Show all phases" and "Show raid
  roster" toggles.
- `Bistooltip_raid.lua`: the fingerprint check (see Subject and dataset model), with its progress in the
  options status line.
  - Inspects one member at a time, 1.5 s apart with a 5 s timeout, least recently tried first.
  - Skips offline members and those out of the 28 yd inspect range.
  - Waits while the InspectFrame is shown, and after anyone else's `NotifyInspect`:
    `INSPECT_TALENT_READY` doesn't say whose talents it carries.
  - Reads the inspected unit's active talent group, and caches trees by GUID for the session.

Changed from the plan:

- Every spec's blocks are fetched, not just your own with the rest on demand, because the tooltip reads
  every spec on every hover. The order is your spec, then the roster, then the other specs.
- Offline members are left out of the fingerprint.

**Verified:**
- `test/run-tests.sh` covers each behaviour, and the full export's 25 raiders reproduce its fingerprint
  `0c5c1449`. Deliberately broken copies were each caught.
- In the client: hovers across phases, the shipped label, both toggles, source switching, the filters,
  Ctrl gating and shift-compare.
- `CanInspect` and `CheckInteractDistance(unit, 1)` return 1 for online bots in range.
- The offline-member handling is checked in the harness only.

Open:

- Renaming `SavedVariables` to e.g. `BisTooltipACDB`, so the fork never shares the original's settings.
  The user's call.
- Skipping the raid check in battlegrounds and arenas, which count as raids of 10+.
- `Bislist.lua`'s comment says `CanInspect` is false for bots. In range it's true.

### Phase 5 — Browser roster mode

`Bislist.lua`: the class dropdown gains a "My raid" entry that swaps the spec dropdown for roster
names; `updateForTarget` prefers a roster subject when the inspected target is a known raider. May
slip without blocking phase 4.

### Phase 6 — Real data, zip, docs

- Run the optimizer for each subject and phase via the driver, export, and import the `.sql`. **The
  import needs the user's OK** — the live DB is otherwise SELECT-only.
- A small script builds `BisTooltipAC-<version>.zip` for hand-distribution. Set `UpdateMessage` in the
  module config (e.g. "ask Arkos for the latest zip"), shown once when an addon reports below
  `MinAddonVersion`. **Only refuse a client on a genuine wire-format break** — otherwise keep serving
  old addons rather than bricking a friend mid-raid, since there's no pull-based update path.
- Docs, in the same change as the work, per `CLAUDE.md`:
  - `docs/guide/azerothcore-server.md`: `mod-bis-tooltip` in the modules table.
  - `CONTEXT.md`: *dataset*, *block*, *subject*, *composition fingerprint*.
  - The module's `README.md`: the wire table, promoted from this PLAN.

## Verification summary

| Level | How |
|---|---|
| Exporter | `go test ./tools/database/azerothcore/` in the toolchain container — round-trip, slot coverage, frame-size cap |
| Codec agreement | `acbis -luaOut` dumps the decoded structure; diff against what the addon caches, the way `acraid`'s `crosscheck.py` independently re-derives its export |
| Module | compiles; loads (worldserver log); `.bistooltip reload` picks up a re-import and nudges live sessions |
| Protocol | cold sync ~6 s; warm login 2 frames; mid-sync disconnect resumes; `Enable = 0` falls back |
| End to end | in-client hover across phases, roster + spec sections, healer fallback, tier default |

## Risks and calls made

- **The rebuild and the SQL import both need the user's explicit OK.** Neither happens unprompted.
- **Wire format churn.** Frozen with Phase 1. The protocol version lets the module refuse a stale addon
  with a clear message instead of sending garbage.
- **Two codecs, one format** (Go and Lua). The `-luaOut` cross-check is what keeps them honest.
- **Stale data by design.** Regeneration is manual, so the dataset lags the sim. The dataset's
  `objective`, sim commit and catalog date are shown in the addon so it's always clear what you're
  looking at. Current-gear "is this an upgrade for X" is deliberately **out** of v1: gear changes on
  every loot, and baked-in gear would be confidently wrong more often than useful.
- **Playerbot raiders** are ordinary characters in `acore_characters`, so acraid already exports them.
  No special-casing.
