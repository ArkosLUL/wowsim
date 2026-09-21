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
  can, by inspect. The addon compares it to your live group only when you're in a group of 10+, and on
  mismatch shows a one-line warning in the roster section header while still showing the data.
  Degraded data beats none.

### Wire format (frozen)

Every message is `BIST\t<op>~<field>~...`, at most 255 bytes including `BIST\t`. `acbis`'s
`bisdata_wire.go` is the reference for the payload, BLK framing, checksum and fingerprint; the module
and the addon must match it.

| Op | Dir | Fields |
|---|---|---|
| `HELLO` | C→S | protocol version (1), addon build (integer), cached dataset version (empty for none) |
| `META` | S→C | dataset version, sim commit, catalog date, objective, fingerprint, viewer tier, subject count, block count |
| `SUBJ` | S→C | dataset version, then subject entries joined with `;` |
| `WANT` | C→S | dataset version, block ids joined with `,`, chunked to fit |
| `BLK` | S→C | dataset version, block id, `<seq>/<total>` (seq from 1), payload chunk |
| `DONE` | S→C | dataset version, block id, checksum (`-` for no such block); `*` for the block id closes a WANT |
| `ERR` | S→C | code (`protocol`, `disabled`, `denied`), message |
| `NOTE` | S→C | message, e.g. the update nudge |

- **Subject entry:** `<id>,<kind>,<classId>,<specName>,<phases>[,<raidIndex>,<guid>,<name>]`. Kind 0 is
  a roster raider, 1 a spec; the last three fields are roster-only. `phases` lists the content phases
  that have a block, e.g. `1245`. Class ids are AzerothCore's (1 warrior .. 11 druid).
- **Block id** = subject id × 10 + content phase.
- **Checksum:** Adler-32 of the payload as 8 lowercase hex digits (`a=1, b=0`; per byte
  `a=(a+byte)%65521, b=(b+a)%65521`; `b*65536+a`). `"Wikipedia"` → `11e60398`.
- **Fingerprint:** the checksum of `<classId>:<tree>` per raider, sorted as strings, joined with `,`;
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
blocks, ~200 KB. First sync fetches roster + your own spec = 130 blocks ≈ 520 frames ≈ **5.2 s** at 10
frames/tick on the live 100 ms `MapUpdateInterval`; healers have no blocks, so less in practice.

**No compression in v1.** Plain decimal stays readable on the wire, which makes bugs in a
two-language codec far cheaper to find, and a ~6 s sync once per dataset change isn't worth buying
complexity to fix. Two levers if the measured number annoys: base-36 item ids (~20% off) or raw
deflate with LibDeflate client-side (~55% off, at the cost of a printable-encoding layer and a second
way for the codecs to disagree).

### Protocol behaviour

**Pull-only, with one deliberate exception.** The module sends nothing until an addon says `HELLO`,
so a login storm costs zero for the ~53k bot accounts. The exception: when `.bistooltip reload` runs,
it sends a single unsolicited `META` to sessions that already handshook *this session*, so an
eight-hour login doesn't sit stale after a re-import. That preserves the property pull-only was
protecting.

**Sync triggers:** `HELLO` on every login and `/reload` (2 frames when the version matches, the
common case), plus a manual "Sync now" button.

**Failure semantics — the block is the atomic unit.** A bad checksum or a sequence gap re-requests
*that block* once, then marks it failed and keeps everything else; never fail a whole sync over one
block. Completed blocks persist to cache as they land, so a disconnect resumes by requesting only
what's missing. Every `SUBJ`, `BLK` and `DONE` carries the dataset version; the addon discards frames
tagged with anything other than what `META` announced, then re-handshakes.

**Three version numbers, kept distinct** because they change for unrelated reasons: the **protocol
version** (mismatch refuses with `ERR~protocol`), the **addon build** (drives the `NOTE` update nudge
only), and the **dataset version** (drives cache invalidation only).

**Access is open, rate-limited.** BiS lists aren't secret; the real risk is load, not disclosure, and
a config allowlist would mean editing a conf and restarting to add a friend. Limits: one `HELLO` per
session per 10 s, a cap on blocks in flight per session, 10 frames/tick per session, and a global
frame ceiling across all sessions so one syncing client can't starve the world thread. A config
switch can restrict to a guild id or account list later if it ever matters.

## Execution: sessions and gates

**One phase per session**, following the project's phase loop: implement, verify, leave uncommitted,
stop — a separate session reviews and fixes. Commit on the effort branch and `git merge --ff-only`
into `master` only on the user's explicit "commit and merge". This is **not** a wave-loop effort: the
phases are a dependency chain, not parallel work items, so the orchestrator machinery would cost more
than it buys.

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

- rebuilding the worldserver (`docker compose build ac-worldserver`) — a hard rule in `CLAUDE.md`
- importing the dataset `.sql` — the live DB is otherwise SELECT-only
- any in-client verification, and the ~245 optimizer runs that produce real data
- `git init` on the addon folder, creating the effort branch, and every commit or merge

**Picking this up cold.** A fresh session should read this PLAN, then `git branch --show-current` and
`git status` in whichever repo the session's phase names (sessions run concurrently in this project).
Sessions 2 and 3 touch entirely different repos and could be run in parallel if you change your mind
about sequencing; sessions 4 and 5 both touch the addon and must not overlap.

## Work

### Phase 1 — Exporter (`wowsim`, branch `bis-tooltip-addon`)

**Status:** done. Committed on `bis-tooltip-addon` and merged into `master`; the user skipped the
separate review session.

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
  `phases`. A slot's delta and reforge sit in an `extra` table, since the addon's reverse lookup
  compares every numeric field with the hovered item id.

`acbis` **consumes only** — it never runs the optimizer. A thin separate driver script orchestrates
the ~245 runs, keeping `acbis` pure and testable, and letting wave G's batch export feed it directly.
Regeneration is **manual**: one documented command, run when you want it, never coupled to the wave
loop's re-baseline. Don't commit the payload; record the dataset version, sim commit, catalog date
and objective in this PLAN so any dataset on the server traces back to a sim state.

Tables in `acore_world` (DDL: `BisTablesSQL`):

| Table | Columns |
|---|---|
| `bistooltip_dataset` | `version`, `sim_commit`, `catalog_date`, `objective`, `fingerprint`, `exported_at` |
| `bistooltip_subject` | `id`, `kind` (0 roster / 1 spec), `guid`, `name`, `class_id`, `spec_name`, `raid_index` |
| `bistooltip_block` | `subject_id`, `content_phase`, `payload` TEXT, `checksum` |

**Verified:** `go test ./tools/database/azerothcore/` passes: round-trip over every slot, malformed
payloads, the frame cap and reassembly up to 131 frames, the checksum's known value, dataset ordering,
skips and errors, batch stages, and the SQL and Lua output. End to end, one real `wowsimcli optimize`
result fanned out to the live roster's 25 raiders and 30 specs × 5 phases gave 55 subjects, 275 blocks
and 1,100 frames; the `.sql` imported twice cleanly into a throwaway MySQL 8.4, and the `-luaOut`
parsed into the expected tables.

### Phase 2 — Server module (`[ac]`, its own branch)

`modules/mod-bis-tooltip/`, mirroring mod-multibot-bridge's layout:

- `src/mod_bis_tooltip.cpp` — the `AddSC_bis_tooltip()` shim.
- `src/BisTooltipBridge.cpp` — load the three tables into memory on
  `WorldScript::OnAfterConfigLoad`; a `.bistooltip reload` GM command so a re-import needs no restart
  (and fires the unsolicited `META` nudge). A `PlayerScript` handles addon whispers; a per-session
  frame queue drains in `WorldScript::OnUpdate`. **Never send a dataset synchronously** — the server
  guide warns that world-thread loops freeze the world.
- Frame blocks exactly as `BlockFrames` does, and pack `SUBJ` entries whole into frames under the cap.
- `conf/BisTooltip.conf.dist` — `Enable`, `FramesPerTick` (10), `GlobalFramesPerTick`,
  `HelloCooldownMs` (10000), `MaxBlocksInFlight`, `MinAddonVersion`, `UpdateMessage`, and an optional
  `Access` / `AllowedGuildId` / `AllowedAccounts` left open by default.
- Treat all addon input as untrusted: validate the protocol version, cap `WANT` length, answer unknown
  block ids with `DONE` checksum `-`, drop requests that fail rate limits without replying.
- Schema DDL in the module's own `data/sql/db-world/base/`, identical to `BisTablesSQL`. Dataset rows
  are **not** committed.

Follow `[ac]/AGENTS.md`: read `.agents/docs/cpp-guidelines.md` before writing C++, plans go in
`.agents/plans/`, don't add live e2e tests, never touch SQL outside the module's own dirs.

**Verify:** compiles. The rebuild (`cd [ac] && docker compose build ac-worldserver && docker compose
up -d`) **needs the user's explicit OK** — a hard rule in `CLAUDE.md`. Then check `docker logs
ac-worldserver` for the module's load line and "World Initialized".

### Phase 3 — Addon fork: transport and cache

Fork [Bistooltip](A:/WOW/world%20of%20warcraft%203.3.5a%20hd/interface/addons/Bistooltip) to a new
folder `BisTooltipAC`, leaving the original installed but disabled — reversible, and stops both
hooking the tooltip. MIT: keep the attribution in `README.md` and `LICENSE`, add the fork's line.
`git init` in place (the MultiBot pattern in that folder), **not pushed**; `git init` needs the user's
go-ahead per their git rule.

**Drop `Bistooltip_tbc_bislists.lua` (593 KB) and `Bistooltip_classic_bislists.lua` (409 KB).** The
realm is WotLK-only; this halves the addon and reduces the data-source selector to "server" vs
"shipped WotLK". Keep the Wowhead-derived WotLK list unchanged as the offline and healer fallback,
labelled as not-yours — a baked server snapshot would go stale, compete with the live dataset for
authority, and put you back to hand-distributing data.

- New `Bistooltip_server.lua`, in the `.toc` before `Core.lua`:
  - Comm: event frame on `CHAT_MSG_ADDON` filtering `prefix == "BIST"`; drive
    `HELLO → META → SUBJ → WANT → BLK/DONE`; reassemble by `blockId~seq/total`; verify checksums;
    implement the retry and version-discard rules above.
  - Cache: decoded blocks and the dataset version in AceDB's **`global`** scope, not `char`, so one
    install syncs once for every character on it. `SavedVariables: BisTooltipDB` is already declared.
  - Decode into exactly what `acbis -luaOut` writes: `Bistooltip_server_bislists`, which
    `searchIDInBislistsClassSpec`, the browser and the checkmark code read unchanged, and
    `Bistooltip_server_roster`.
  - Fetch policy: all roster blocks plus your own spec on first sync; other specs on demand.
- `Config.lua`: add `Server (live)` to the existing "Data source" select, a "Sync now" button, and a
  status line (dataset version, catalog date, objective, last sync, fallback state). When the module
  doesn't answer, fall back and **say so** rather than silently.

**Verify:** cold sync completes; a warm login costs two frames; `Enable = 0` falls back cleanly;
killing the connection mid-sync resumes without re-fetching completed blocks; the cached tables match
`acbis -luaOut` for the imported dataset.

### Phase 4 — Tooltip (the shippable milestone)

`Bistooltip.lua`, in `OnGameTooltipSetItem`: add a roster section above the existing spec section —
one line per raider wanting the item, `<class icon> <Raider> (<spec>)` left, phases/ranks right,
reusing the `"P4 BIS" / "P4 alt 2"` string shape. Add the rank-1 delta where present, and the reforge
text line. Respect the existing `tooltip_with_ctrl` and spec filters; add a roster toggle. Keep the
`Bistooltip_LastItemId` guard so the modifier-key refresh still works.

Default to showing phases at or below the viewer's progression tier plus one — the module supplies the
tier in `META` — with a config toggle for all five, so the tooltip doesn't become a wall.

**This phase is the product.** It must be shippable without phase 5.

**Verify:** by hand in the client. Hover a known P4 trinket, confirm the named raiders match the
dataset; confirm a healer item falls back and is labelled; confirm the tier-based phase default and
the all-phases toggle.

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
  - The module's `README.md`: the protocol table (promoted from this PLAN) and config keys.

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
