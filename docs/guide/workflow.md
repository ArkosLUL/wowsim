# Workflow

How the user runs efforts, beyond the hard rules in [CLAUDE.md](../../CLAUDE.md).

## Planning

- An effort starts with an investigation and a plan in `docs/<slug>/`.
- The user usually rejects the first plan and runs `/grill-me`. Give every question a recommended answer;
  replies are usually "Qn - x, others go with suggested".
- Phases are referred to by ID (P2, Phase 3), and IDs aren't execution order: parity ran P1, P0, P2. If
  "part N" is ambiguous, ask.
- When a phase lands, update the status line in its PLAN.

## Branches and worktrees

- Each effort branches from `master`, on a branch named after its slug, e.g. `azerothcore-raid-import`
  for `docs/azerothcore-raid-import/`. Follow-up branches are fine (raid-import's Phase 2 used
  `azerothcore-roster-export`).
- Each concurrently active effort gets its own worktree, `G:\DevStuff\GitHub\wowsimwotlk-<short name>`,
  e.g. `wowsimwotlk-parity`.
  - Create it with `git worktree add` only when the user asks.
  - The user adds it to VS Code with `code --add <path>`.
- [sim] is shared by every effort that works on `master`. Another session once switched it to `master`
  mid-phase, and a commit meant for the effort branch landed there.
- Sessions sharing a worktree coordinate via SendMessage. For goldens, see
  [testing.md](testing.md#goldens).

## Review and landing

1. Implement a phase and verify it (build and tests in the container). Leave it uncommitted and stop.
2. The user opens a separate session for `/code-review <scope>`, then says "fix all findings". Reviewers
   run Go and TS in the container too.
3. Wait for the user to say "commit and merge". A bare "can be merged" gets the commit blocked by auto
   mode.
4. Re-verify the reviewer's edits, commit on the effort branch, then
   `git switch master && git merge --ff-only <branch>`.
   - If master has moved, first merge `master` into the effort branch and test the merged tree. Set
     another phase's uncommitted work aside with a named stash and re-apply it after.
5. Don't push or delete branches unless asked. Pushes go to `origin`.

## Server-side work

- Server changes go in [ac] modules, each its own git repo under `[ac]/modules/`, following [ac]'s
  `AGENTS.md`.

## Orchestrated waves

- Loop efforts ([workstreams](workstreams.md)) run in waves, per the
  [RUNBOOK](../wave-loop/wave-loop.RUNBOOK.md):
  - Parallel work items, each implemented and then reviewed by separate agents.
  - The orchestrator merges them into `integration`, which fast-forwards `master` after each wave.
- For those efforts this replaces "Branches and worktrees" and "Review and landing". The user checks in
  once per wave.
- The orchestrator is the user's current session, compacted as needed, not a separate interactive
  session: the user doesn't want to tend extra sessions.
