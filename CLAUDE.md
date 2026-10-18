# wowsimwotlk: AzerothCore fork

This fork of wowsims/wotlk models the user's AzerothCore 3.3.5a server exactly, not WotLK Classic
([ADR 0001](docs/adr/0001-server-exact-permanent-fork.md)). `origin` is the user's fork
(ArkosLUL/wowsim); `upstream` (wowsims/wotlk) is reference only: never pull from or push to it.

Paths used across the docs:
- **[sim]** `G:\DevStuff\GitHub\wowsimwotlk`: main checkout.
- **[int]** `G:\DevStuff\GitHub\wowsimwotlk-int`: the wave loop's worktree, on `integration`.
- **[ac]** `G:\DevStuff\GitHub\azerothcore-wotlk-pb`: the live server's source. Its own `AGENTS.md`
  governs work there.

## Hard rules

- Go and make aren't on the host: run go, protoc, npx and make in the toolchain container
  ([dev-environment](docs/guide/dev-environment.md)).
- Never `make update-tests`: it deletes every `.results` golden. Promote suite by suite
  ([testing](docs/guide/testing.md)).
- The live DB is SELECT-only. Every server rebuild, restart or config change needs the user's OK, beyond
  the wave loop's [standing authorizations](docs/wave-loop/wave-loop.RUNBOOK.md#standing-authorizations).
- One worktree per effort, or per work item in the wave loop. Don't touch another effort's worktree or
  owned paths ([workstreams](docs/guide/workstreams.md)). Sessions run concurrently: check
  `git branch --show-current` right before committing.
- Phase loop, run from this session as orchestrator: a subagent implements and verifies, a fresh one
  reviews, fixes and re-verifies; then land it without waiting for the user: commit on the effort branch
  and in any sibling repo the phase changed, then `git merge --ff-only` into `master`. Never skip the
  review; push only when asked ([workflow](docs/guide/workflow.md)). Loop-driven efforts follow the
  [wave-loop RUNBOOK](docs/wave-loop/wave-loop.RUNBOOK.md) instead.
- Verification tooling that proves useful (harnesses, e2e, cross-checks) moves out of scratch, next to the
  code it tests, with a run command in its README.
- Never read `github-recovery-codes.txt` or `Important data.txt`; never copy credential values out of
  configs.

## Doc map

Before starting, read every doc whose trigger matches the task.

| Doc | Read when |
|---|---|
| [CONTEXT.md](CONTEXT.md) | a project term is unclear, or you're naming something |
| [dev-environment](docs/guide/dev-environment.md) | building, running, regenerating protos, any Docker or shell work |
| [testing](docs/guide/testing.md) | verifying or reviewing a change with Go or UI tests, touching goldens |
| [server-validation](docs/guide/server-validation.md) | replaying or recapturing server records, running or writing a live e2e test, judging parity |
| [sim-architecture](docs/guide/sim-architecture.md) | locating sim, proto, UI or tool code |
| [server-parity](docs/guide/server-parity.md) | changing a combat table, cost, proc, immunity or other core mechanic, or module behaviour |
| [items](docs/guide/items.md) | changing an item's, gem's or enchant's value or effect, or the item swap |
| [damage-and-healing](docs/guide/damage-and-healing.md) | changing how a hit's or heal's amount is worked out: percents, rounding, the AoE cap, crit damage |
| [class-mechanics](docs/guide/class-mechanics.md) | changing a death knight's, warrior's, paladin's, rogue's or hunter's spells, talents or procs |
| [caster-mechanics](docs/guide/caster-mechanics.md) | changing a druid's, shaman's, mage's, priest's or warlock's spells, talents or procs |
| [pets](docs/guide/pets.md) | changing a pet's or guardian's stats, timers, AI, or the buffs and casts that reach it |
| [threat](docs/guide/threat.md) | changing threat, energizes or heals, or a spell's threat data |
| [tank-modeling](docs/guide/tank-modeling.md) | changing the optimizer's tank boss, healing model, survival score, or any run's hit, expertise or haste caps |
| [playerbot-rules](docs/guide/playerbot-rules.md) | changing what the Playerbot switch does, or a tool that turns it on |
| [spell-data-and-timing](docs/guide/spell-data-and-timing.md) | declaring a spell's server data; update, aura, swing, cast or GCD timing |
| [missiles](docs/guide/missiles.md) | changing a missile's flight or landing, or a unit's or target's combat reach |
| [azerothcore-server](docs/guide/azerothcore-server.md) | touching the live server, its containers, config, modules or characters |
| [azerothcore-data](docs/guide/azerothcore-data.md) | querying server tables or parsing DBCs |
| [workflow](docs/guide/workflow.md) | planning, branching, reviewing, committing |
| [workstreams](docs/guide/workstreams.md) | starting or resuming an effort; checking who owns a path |
| [wave-loop RUNBOOK](docs/wave-loop/wave-loop.RUNBOOK.md) | running or resuming the wave loop, or working one of its items |
| [docs/adr/](docs/adr/) | before reversing a settled design choice |
| [acdiff README](tools/database/acdiff/README.md) | diffing sim item data against the server |
| [acraid README](tools/database/acraid/README.md) | exporting the raid roster |
| [accatalog README](tools/database/accatalog/README.md) | regenerating the server item catalog, or changing its tier rules |
| [acbis README](tools/database/acbis/README.md) | exporting the BiS tooltip dataset |
| [acbis driver README](tools/database/acbis/driver/README.md) | running the optimizer batch the dataset is built from |
| [presetgen README](tools/presetgen/README.md) | regenerating the BiS gear presets |
| [statweights README](tools/statweights/README.md) | regenerating the stat slopes behind the playerbots' stat weights |
| [rankings README](tools/rankings/README.md) | regenerating the DPS rankings |
| [mod-bis-tooltip README](../azerothcore-wotlk-pb/modules/mod-bis-tooltip/README.md) | changing the BiS tooltip wire format, or the module's config, commands or tests |
| [uitest README](tools/uitest/README.md) | verifying a UI change, or writing a UI test |
| [uicheck README](tools/uicheck/README.md) | driving a page by hand in headless Chrome, e.g. for screenshots |
| [perf README](tools/perf/README.md) | profiling a sim, the optimizer or the server; timing a change against the baseline |
| [apldbg README](tools/apldbg/README.md) | tuning or debugging a rotation; comparing a rotation across a sim change |

## Keeping the docs live

Every fact has exactly one home; elsewhere, link to it.

| Fact | Home |
|---|---|
| Project term | `CONTEXT.md` (glossary only) |
| Decision that's hard to reverse, surprising, and a real trade-off | `docs/adr/NNNN-slug.md`, next free number |
| Durable fact, command or gotcha | its topic's page in `docs/guide/`, under that topic's heading |
| How to run a tool | the tool's `README.md` |
| Plan, investigation, phase status | `docs/<slug>/<slug>.<TYPE>.md` |

In the same change as the work:
- Fix any doc fact found wrong. Add new durable facts, commands, gotchas, and user corrections about how
  this project works.
- When a phase or effort lands, promote its durable learnings from PLAN/INVESTIGATION into the guide,
  ADRs or CONTEXT, and update `workstreams.md`.
- New topic: new `docs/guide/<topic>.md` plus a map row. Split a page past ~200 lines or two topics.
- Guide pages state current truth only: no status, history or dates; cite code by path, not line. Tag a
  fact that exists only on an unmerged branch `(<branch> only)`; drop the tag when it merges.
