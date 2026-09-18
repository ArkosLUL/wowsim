export const meta = {
  name: 'wave-loop',
  description: 'Run one wave: implement, then independently review, each work item in its own worktree',
  whenToUse: 'Step 3 of the wave procedure in docs/wave-loop/wave-loop.RUNBOOK.md',
  phases: [
    { title: 'Implement', detail: 'one implementer per work item' },
    { title: 'Review', detail: 'a fresh reviewer per work item fixes every finding' },
    { title: 'Repair', detail: 'one repair round for items still red, then a second review' },
  ],
}

// args: {wave, baseSha, items: [{id, effort, kind, worktree, branch, specPath, specSection,
// ownedPaths, goldenChanging, fullSuite, verify, server, devPort}]}

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

function brief(item) {
  return [
    `Work item ${item.id} (effort ${item.effort}), wave ${args.wave}.`,
    `Worktree: ${item.worktree} (branch ${item.branch}, based on ${args.baseSha}). Work only there.`,
    `Spec: ${item.worktree}/${item.specPath}, section "${item.specSection}".`,
    `Rules: the "Rules for WI agents" section of ${item.worktree}/docs/wave-loop/wave-loop.RUNBOOK.md. Follow it exactly.`,
    `Owned paths: ${item.ownedPaths.join(', ')}.`,
    item.goldenChanging
      ? `This item changes goldens${item.fullSuite ? ' and runs all 37 suites' : ''}: report each suite's dock.sh delta with the expected effect. Never promote.`
      : 'This item must leave every golden unchanged.',
    `Verification: ${item.verify.join(' ; ')}`,
    item.server
      ? 'It needs the live server: hold the server lock and follow the RUNBOOK server rules.'
      : 'It must not touch the live server beyond SELECTs.',
    item.devPort ? `Dev server, if needed: port ${item.devPort}, container wotlk-dev-${item.id}.` : '',
    'No git writes: leave every change uncommitted.',
  ].filter(Boolean).join('\n')
}

function implement(item) {
  if (item.kind === 'review-only') return { status: 'green', summary: 'review-only item', filesChanged: [], verification: { commands: [], passed: true }, goldens: { changed: false } }
  return agent(
    `${brief(item)}\n\nImplement the spec, then run its verification. Return the report. Use status "blocked" for a problem the spec can't resolve, and "waiting-server" when the offline check or server lock stops you.`,
    { label: `impl:${item.id}`, phase: 'Implement', schema: REPORT },
  )
}

function review(item, prior, round) {
  return agent(
    `${brief(item)}\n\nYou are a fresh reviewer (round ${round}). The implementer reported:\n${JSON.stringify(prior)}\n\n` +
      `1. Review every change in the worktree against ${args.baseSha} (git -C ${item.worktree} diff ${args.baseSha}, plus untracked files) with the code-review skill at high effort${item.kind === 'review-only' ? '. For this review-only item, review the spec\'s commit range instead' : ''}.\n` +
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

const settle = (item, report) => ({ id: item.id, status: report ? report.status : 'red', report })

const results = await pipeline(
  args.items,
  item => implement(item),
  (impl, item) => {
    if (!impl) return { done: settle(item, null) }
    if (impl.status === 'blocked' || impl.status === 'waiting-server') return { done: settle(item, impl) }
    return review(item, impl, 1).then(r => ({ review: r }))
  },
  async (prev, item) => {
    if (prev.done) return prev.done
    const first = prev.review
    if (first && first.status !== 'red') return settle(item, first)
    const fixed = await repair(item, first)
    if (!fixed || fixed.status !== 'green') return settle(item, fixed)
    return settle(item, await review(item, fixed, 2))
  },
)

const out = results.map((r, i) => r || { id: args.items[i].id, status: 'red', report: null })
log(`Wave ${args.wave}: ${out.map(r => `${r.id}=${r.status}`).join(', ')}`)
return out
