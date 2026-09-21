export const meta = {
  name: 'wave-loop',
  description: 'Run one wave: implement, then independently review, each work item in its own worktree',
  whenToUse: 'Step 3 of the wave procedure in docs/wave-loop/wave-loop.RUNBOOK.md',
  phases: [
    { title: 'Implement', detail: 'one implementer per work item, or one per stage' },
    { title: 'Review', detail: 'a fresh reviewer per work item fixes every finding' },
    { title: 'Repair', detail: 'one repair round for items still red, then a second review' },
  ],
}

// args: {wave, baseSha, items: [{id, effort, kind, worktree, branch, specPath, specSection,
// ownedPaths, goldenChanging, fullSuite, verify, server, devPort, notes?, priorReport?, stages?}]}
// priorReport: a finished implementer's report from an earlier run; the item goes straight to review.
// stages: [{label, notes, server, priorReport?}], fresh implementers run in order in the same worktree,
// each handed the earlier stages' reports. A stage's priorReport skips that stage.

const STRINGS = { type: 'array', items: { type: 'string' } }

const REPORT = {
  type: 'object',
  properties: {
    status: { type: 'string', enum: ['green', 'red', 'blocked', 'waiting-server'] },
    summary: { type: 'string' },
    filesChanged: STRINGS,
    verification: {
      type: 'object',
      properties: { commands: STRINGS, passed: { type: 'boolean' }, tail: { type: 'string' } },
      required: ['commands', 'passed'],
    },
    goldens: {
      type: 'object',
      properties: {
        changed: { type: 'boolean' },
        suites: {
          type: 'array',
          items: {
            type: 'object',
            properties: {
              dir: { type: 'string' },
              dpsDelta: { type: 'string' },
              expected: { type: 'string' },
              explanation: { type: 'string' },
            },
            required: ['dir', 'dpsDelta'],
          },
        },
      },
      required: ['changed'],
    },
    contractChangeRequests: STRINGS,
    serverActions: STRINGS,
    followUps: STRINGS,
    findings: {
      type: 'object',
      properties: { bugs: STRINGS, minor: STRINGS, cleanup: STRINGS },
    },
  },
  required: ['status', 'summary', 'filesChanged', 'verification', 'goldens'],
}

function brief(item, stage) {
  const server = stage ? stage.server : item.server
  const stageList = !stage && item.stages
    ? `It ran in stages: ${item.stages.map(s => `"${s.label}": ${s.notes}`).join(' | ')}`
    : ''
  return [
    `Work item ${item.id} (effort ${item.effort}), wave ${args.wave}.`,
    `Worktree: ${item.worktree} (branch ${item.branch}, based on ${args.baseSha}). Work only there.`,
    `Your shell starts in another checkout and resets there after every command, so use absolute worktree paths for every file tool, and run dock.sh as bash ${item.worktree}/tools/acore/dock.sh (it mounts the checkout it lives in).`,
    `Spec: ${item.worktree}/${item.specPath}, section "${item.specSection}".`,
    item.notes ? `Notes: ${item.notes}` : '',
    stage ? `Your stage, "${stage.label}": ${stage.notes}` : stageList,
    `Rules: the "Rules for WI agents" section of ${item.worktree}/docs/wave-loop/wave-loop.RUNBOOK.md. Follow it exactly.`,
    `Owned paths: ${item.ownedPaths.join(', ')}.`,
    item.goldenChanging
      ? `This item changes goldens${item.fullSuite ? ' and runs all 37 suites' : ''}: report each suite's dock.sh delta with the expected effect. Never promote.`
      : 'This item must leave every golden unchanged.',
    `Verification: ${item.verify.join(' ; ')}`,
    server
      ? 'It needs the live server: hold the server lock and follow the RUNBOOK server rules.'
      : 'It must not touch the live server beyond SELECTs.',
    item.devPort ? `Dev server, if needed: port ${item.devPort}, container wotlk-dev-${item.id}.` : '',
    'No git writes: leave every change uncommitted.',
  ].filter(Boolean).join('\n')
}

const DONE_WHEN = 'Use status "blocked" for a problem the spec can\'t resolve, and "waiting-server" when the offline check or server lock stops you.'

// Each stage is a fresh agent, so no single context has to hold the whole item.
async function implementStages(item) {
  const reports = []
  const pending = []
  for (let i = 0; i < item.stages.length; i++) {
    const stage = item.stages[i]
    const earlier = reports.length
      ? `Earlier stages finished and left their changes uncommitted in the worktree; build on them and don't redo their work. Their reports:\n${JSON.stringify(reports)}`
      : 'You are the first stage.'
    let r = stage.priorReport || await agent(
      `${brief(item, stage)}\n\nThis item runs in ${item.stages.length} stages, each a fresh agent in the same worktree. You are stage ${i + 1}, "${stage.label}". ${earlier}\n\n` +
        `Do your stage only, then run the verification it can affect (the last code stage runs all of it) and return the report. ${DONE_WHEN}`,
      { label: `impl:${item.id}:${stage.label}`, phase: 'Implement', schema: REPORT },
    )
    if (!r) return { status: 'red', incomplete: true, summary: `stage "${stage.label}" returned nothing`, stages: reports }
    // A priorReport may be a pointer to the report's file rather than the report itself.
    if (typeof r === 'string') r = { status: 'green', summary: r }
    reports.push({ stage: stage.label, ...r })
    if (r.status === 'blocked') return { status: 'blocked', summary: `stage "${stage.label}" is blocked`, stages: reports }
    if (r.status === 'waiting-server') {
      if (!stage.server) return { status: 'waiting-server', summary: `stage "${stage.label}" waits for the server`, stages: reports }
      pending.push(stage.label)
    }
  }
  return {
    status: reports.some(r => r.status === 'red') ? 'red' : 'green',
    summary: reports.map(r => `[${r.stage}] ${r.summary}`).join('\n'),
    stages: reports,
    pendingServerStages: pending,
  }
}

function implement(item) {
  if (item.priorReport) return item.priorReport
  if (item.kind === 'review-only') return { status: 'green', summary: 'review-only item', filesChanged: [], verification: { commands: [], passed: true }, goldens: { changed: false } }
  if (item.stages) return implementStages(item)
  return agent(
    `${brief(item)}\n\nImplement the spec, then run its verification. Return the report. ${DONE_WHEN}`,
    { label: `impl:${item.id}`, phase: 'Implement', schema: REPORT },
  )
}

function review(item, prior, round) {
  return agent(
    `${brief(item)}\n\nYou are a fresh reviewer (round ${round}). The implementer reported:\n${JSON.stringify(prior)}\n\n` +
      `1. Review every change in the worktree against ${args.baseSha} (git -C ${item.worktree} diff ${args.baseSha}, plus untracked files) by hand at high effort (the code-review skill reviews the session's main checkout, not your worktree)${item.kind === 'review-only' ? '. For this review-only item, review the spec\'s commit range instead' : ''}.\n` +
      '2. Fix every finding in the worktree.\n' +
      '3. Re-run the verification. Check each golden delta against the spec\'s expected effect.\n' +
      'Return the report with your findings grouped as bugs, minor and cleanup, and status "green" only if the verification passes.',
    { label: `review${round}:${item.id}`, phase: 'Review', schema: REPORT },
  )
}

function repair(item, failed) {
  return agent(
    `${brief(item)}\n\nThe review left this item red:\n${JSON.stringify(failed)}\n\nFix what it reports, re-run the verification, and return the report.`,
    { label: `repair:${item.id}`, phase: 'Repair', schema: REPORT },
  )
}

// pending: server stages that returned waiting-server; the item was reviewed without them.
const settle = (item, report, pending, status) => ({
  id: item.id,
  status: status || (report ? report.status : 'red'),
  report,
  pending: pending || [],
})

const results = await pipeline(
  args.items,
  item => implement(item),
  (impl, item) => {
    if (!impl) return { done: settle(item, null) }
    if (impl.incomplete) return { done: settle(item, impl, [], 'red') }
    if (impl.status === 'blocked' || impl.status === 'waiting-server') return { done: settle(item, impl, impl.pendingServerStages) }
    return review(item, impl, 1).then(r => ({ review: r, pending: impl.pendingServerStages }))
  },
  async (prev, item) => {
    if (prev.done) return prev.done
    const first = prev.review
    if (!first || first.status !== 'red') return settle(item, first, prev.pending)
    const fixed = await repair(item, first)
    if (!fixed || fixed.status !== 'green') return settle(item, fixed, prev.pending)
    return settle(item, await review(item, fixed, 2), prev.pending)
  },
)

const out = results.map((r, i) => r || { id: args.items[i].id, status: 'red', report: null, pending: [] })
log(`Wave ${args.wave}: ${out.map(r => `${r.id}=${r.status}${r.pending.length ? ` (pending: ${r.pending.join(', ')})` : ''}`).join(', ')}`)
return out
