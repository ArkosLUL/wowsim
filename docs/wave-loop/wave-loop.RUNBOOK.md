# Wave loop runbook

How the orchestrator session runs a wave, and the rules every work-item (WI) agent follows. The wave
registry, decisions and status are in [wave-loop.PLAN.md](wave-loop.PLAN.md). WI specs live in each effort's
PLAN under a heading with the WI id.

## Paths and branches

| Name | Path | Branch |
|---|---|---|
| [sim] | `G:\DevStuff\GitHub\wowsimwotlk` | `master`, shared with other sessions |
| [int] | `G:\DevStuff\GitHub\wowsimwotlk-int` | `integration` |
| WI worktree | `G:\DevStuff\GitHub\wowsimwotlk-wi-<id>` | `<effort>-<id>`, e.g. `bis-optimizer-catalog` |
| [ac] | `G:\DevStuff\GitHub\azerothcore-wotlk-pb` | the server; `modules/mod-sim-validation` is its own repo |

`[par]` is removed once wave A lands P4 on `master`; branch `azerothcore-parity` stays.

## Standing authorizations

Given by the user in the 2026-09-18 planning session. After a `/compact`, the user's "continue the wave
loop" re-affirms them.

**Git** (orchestrator only; WI agents never write git state):
- Per WI: create its branch and worktree off `integration`; commit it after review; `git merge --no-ff`
  it into `integration`; `git worktree remove` it; `git branch -d` it.
- Commit golden promotions and status updates on `integration`.
- `git merge master` into `integration` at wave start, and whenever the wave-end fast-forward fails.
  Conflicts in parity-owned files take master's side.
- After a green wave, `git -C [sim] merge --ff-only integration`, with [sim] checked to be on `master`.
- Commit a WI's mod-sim-validation changes on that repo's current branch after the WI merges.
- Commit messages go via file, in conversational voice, with no AI attribution.
- Never push, rebase, reset, clean or force.

**Server:**
- The DB stays SELECT-only.
- Allowed without asking: live `.simval` runs (spelldump included), live e2e (`mod-sim-validation/e2e/run.sh`),
  and playerbot recorded runs logged by Chronicle.
- Rebuild or restart (`cd [ac] && docker compose build ac-db-import ac-worldserver && docker compose up -d`,
  ready at "World Initialized") only when this returns no rows:
  `SELECT name FROM acore_characters.characters WHERE name IN ('Agony','Deathsong','Felesta','Nightwarrior') AND online = 1`.
  Otherwise the WI returns `waiting-server`.
- Server config changes still need the user's OK. None is expected: simval is enabled live.

## Wave procedure

1. **Start.** Read the PLAN's "Current wave". If `master` moved, merge it and run step 4.3. Record the wave
   base SHA in the PLAN.
2. **Set up each WI.** `git -C [int] worktree add -b <branch> <path> integration`. In it, through
   `tools/acore/dock.sh`:
   - generate the protos
   - `make binary_dist/dist.go` if it touches Go web code
   - `make node_modules` for UI WIs

   Give it dev port 3335+i and container name `wotlk-dev-<id>`.
3. **Run.** `Workflow({scriptPath: "[int]/docs/wave-loop/wave-loop.workflow.js", args})`, with `args` per
   the contract below. Record the runId in the PLAN, then wait for the notification.
4. **Integrate** green WIs one at a time, in registry order:
   1. Check `git -C <wi> branch --show-current`. Stage only owned paths; skip CRLF-only `.results`. Commit.
   2. `git -C [int] merge --no-ff <branch>`. Resolve conflicts with `/resolving-merge-conflicts`.
   3. In [int], through dock.sh:
      - `test`, `test ./tools/...`
      - `go vet ./sim/... ./tools/... ./cmd/...`
      - `tsc`, and eslint per changed file vs HEAD
      - the simval replay
      - gofmt through `tr -d '\r'`
   4. Golden-changing WI:
      - `dock.sh delta`. The WI was built on the wave base, so judge its delta on top of the earlier merges
        and against its report.
      - `dock.sh promote` only its suites. Diff stats, casts and weights.
      - Commit "Promote goldens for <WI>".
   5. If APLs, encounters or item data changed, regenerate `db.json` once per wave, here.
   6. Commit its mod-sim-validation changes, if any.
   7. Remove the worktree and branch. Update the WI's status in the PLAN and its effort PLAN. Commit.

   A red, blocked or `waiting-server` WI stays unmerged and carries forward.
5. **Cross-review.** One Agent reviews `git diff <wave base>..integration` with `/code-review` at high
   effort, fixes the findings, and re-runs step 4.3. Commit.
6. **BiS re-baseline** (wave D onward): the `optimizer_slow` suite, with its deltas in the report.
7. **Land.** Merge `master` again if it moved, re-verify, then `git -C [sim] merge --ff-only integration`.
   If [sim]'s local changes block it, report and stop.
8. **Report and stop.** Cover:
   - merged WIs and the findings fixed
   - carried WIs
   - golden deltas per suite
   - BiS deltas
   - server actions
   - the next wave

   Update "Current wave". Wait for "continue".

## Rules for WI agents

**Scope and git**
- Work only in your worktree. Point every Docker mount at it. Never touch [sim], [int] or another WI's
  worktree.
- Edit only your WI's owned paths. A change to a shared contract (`sim/optimizer/types.go`, protos,
  another WI's paths) goes in `contractChangeRequests`.
- No git writes. Leave everything uncommitted.

**Build and test**
- Build and test only through `tools/acore/dock.sh`.
- Never `make update-tests`, and never promote.
- Golden-changing WIs report `dock.sh delta` per suite, with the expected effect and a diff of stats,
  casts and weights.
- Golden-neutral WIs report the suites they ran as unchanged.
- Generators: get `sim/core` compiling first (hand-edit the generated file), then regenerate.
- Only the one WI a wave assigns touches `db.json`.

**Live server**
- Hold the lock: `mkdir G:\DevStuff\GitHub\.wave-loop\locks\server` (atomic). Remove it when done. Report
  a lock older than 2 h as stale.
- Live runs go one at a time. Stream simval records with `docker exec ac-worldserver tail -F -n +1 …`.
- New e2e tests go in per-WI files. At most one WI per wave changes the module's C++.

**Code and docs**
- Comments: invoke `use-conversational-language` before writing any.
- Docs: edit only your WI's section. The orchestrator owns status lines and guide promotion.

## Workflow contract

`args`:

```
{wave, baseSha, items: [{id, effort, kind: 'implement'|'review-only', worktree, branch,
specPath, specSection, ownedPaths, goldenChanging, fullSuite, verify: [cmd], server, devPort}]}
```

Each item is a pipeline:
1. **Implement** (skipped for review-only).
2. **Review** by a fresh agent: `/code-review` at high effort over `git diff <baseSha>` plus untracked
   files. It fixes every finding, re-verifies, and checks golden deltas against the spec.
3. If red: one repair agent, then a second review.
4. Returns `{id, status: green|red|blocked|waiting-server, report}`.

`report`:

```
{status, summary, filesChanged, verification{commands, passed, tail},
goldens{changed, suites[{dir, dpsDelta, expected, explanation}]},
contractChangeRequests, serverActions, followUps, findings{bugs, minor, cleanup}}
```

A wave holds at most 4 items. At most 2 are `fullSuite` (golden-changing and running all 37 suites).
Golden-neutral items leave the full run to integration.

## Resume

After a `/compact` or restart:
1. Read this file and the PLAN's "Current wave": the wave, base SHA, runId and WI statuses.
2. A running Workflow: wait for its notification. A killed one: re-run with `resumeFromRunId`.
3. Continue at the first unfinished step.
