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
- Phase loop: implement, verify, leave uncommitted, stop. A separate session reviews and fixes. Only on
  the user's "commit and merge": commit on the effort branch, then `git merge --ff-only` into `master`.
  Push only when asked ([workflow](docs/guide/workflow.md)). Loop-driven efforts follow the
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
| [testing](docs/guide/testing.md) | verifying or reviewing a change, touching goldens |
| [sim-architecture](docs/guide/sim-architecture.md) | locating sim, proto, UI or tool code |
| [server-parity](docs/guide/server-parity.md) | changing a mechanic, item value or module behaviour |
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
| [uicheck README](tools/uicheck/README.md) | checking a UI change in a real browser |

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
