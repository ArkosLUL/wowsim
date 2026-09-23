# ui/core/optimizer

Request building for the BiS optimizer tab ([PLAN](../../../docs/bis-optimizer/bis-optimizer.PLAN.md#bis-ui-tab-wave-d)):
- `pool_builder.ts`: `buildOptimizeRequest`. Pure (no DOM, no Player), so it runs under Node.
- `catalog.ts`: loads `server_catalog.json` and decides what a phase, faction and source set allow.
- `item_filters.ts`: the gear picker's filters, shared with `Player.filterItemData`.

## Replay fixtures

`fixture_driver.ts` runs the pool builder under Node over `db.json`, `server_catalog.json` and the
Retribution presets, writing `sim/optimizer/testdata/replay/*.json.gz` for `TestReplayFixtures` and
`TestReplayEquippedGear`. Rerun it after changing the pool builder, those presets, `db.json` or the catalog:

```sh
tools/acore/dock.sh exec bash -c 'npx esbuild ui/core/optimizer/fixture_driver.ts --bundle --platform=node --format=esm --target=node18 --outfile=tmp/optimizer/fixture_driver.mjs && node tmp/optimizer/fixture_driver.mjs write sim/optimizer/testdata/replay'
```

It prints each pool's size and what each seed lost to fit its pool. The tab's "Export request" button
downloads the same request as plain JSON, which the test takes as is.

`post` sends a fixture (`.json` or `.json.gz`) to a running sim server the way `net_worker.js` does, then
prints the final `OptimizerResult`. The bundle needs only Node 18+, so it runs on the host too:

```sh
node tmp/optimizer/fixture_driver.mjs post http://localhost:3334 sim/optimizer/testdata/replay/ret_p1.json.gz
```
