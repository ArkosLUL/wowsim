# ui/raid/acore_harness

Offline checks for the AzerothCore raid importer ([PLAN](../../../docs/azerothcore-raid-import/azerothcore-raid-import.PLAN.md)).
They import the committed roster export through the shipped `acore_roster.ts` and `acore_importer.ts` and
print what came out, so the parts of the importer that aren't about the DOM can be checked without a
browser. The dialog, the grid and the alerts get their browser tests in
[acore_import.spec.ts](../../../tools/uitest/tests/raid/acore_import.spec.ts).

| Harness | Checks |
|---|---|
| `smoke.ts` | every character resolves: spec, preset, race, traits, professions, items, gems, reforges, glyphs |
| `raid.ts` | placement, `applyCharacter` and `newPlayerFromPreset` against a real `Sim` |
| `replace.ts` | Replace mode's `Player.toProto` → `Raid.fromProto` round trip, tanks and party layout |
| `update.ts` | Update mode through the shipped `updateRaid`: kept consumes, replaced specs, removals, remapped assignments |
| `edges.ts` | `parseRoster` rejections, overfull subgroups, main-tank flags |

## Running

```sh
tools/acore/dock.sh exec bash -c '
for n in smoke raid replace update edges; do
  npx esbuild ui/raid/acore_harness/$n.ts --bundle --platform=node --format=cjs \
    --outfile=tmp/acore_harness/$n.cjs --loader:.json=json \
    --tsconfig=ui/raid/acore_harness/tsconfig.json --log-level=error
done
T=ui/raid/acore_harness/testdata/raid.json
node ui/raid/acore_harness/run.cjs tmp/acore_harness/smoke.cjs   $T
node ui/raid/acore_harness/run.cjs tmp/acore_harness/raid.cjs    $T
node ui/raid/acore_harness/run.cjs tmp/acore_harness/replace.cjs $T
node ui/raid/acore_harness/run.cjs tmp/acore_harness/update.cjs
node ui/raid/acore_harness/run.cjs tmp/acore_harness/edges.cjs
'
```

Every line prints what it expects next to what it got, so a run is read rather than asserted.

Three things bite:
- esbuild picks up the repo tsconfig, whose `jsx: preserve` it can't bundle, hence the local
  `tsconfig.json` with `jsx: react` and `jsxFactory: element`.
- `dock.sh tsc` type-checks everything under the repo root, `tmp/` included, so the bundles go to
  `tmp/acore_harness/*.cjs` and no scratch TypeScript may live there.
- `sim.db` is null until `await sim.waitForInit()`, and the `Sim` constructor builds a worker pool, so
  `run.cjs` stubs `window.Worker` along with the rest of the browser.

## Fixtures

`testdata/raid.json` is an acraid export of the live 25-man raid
([acraid](../../../tools/database/acraid/README.md)). The other three are that file with one thing
changed, for cases the plain roster can't reach:

| File | Change | Reaches |
|---|---|---|
| `raid-justice-prot.json` | Justice takes Bulwark's protection talents | Update replacing a raider whose top tree moved |
| `raid-no-tree.json` | Tree is gone | Update removing a raider |
| `raid-leftover.json` | Angry's Finger 1 is Freezing Band (942) | the reload fix, since that item is only in `leftover_db.json` |
