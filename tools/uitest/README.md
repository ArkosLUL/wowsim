# uitest

Playwright regression tests for the sim UI. They run on the host: the repo's `node_modules` is installed
from Linux, so this directory has its own.

## Run

Needs Node, Chrome, and a sim server holding the build under test, usually the dev server
([dev-environment, Run](../../docs/guide/dev-environment.md#run)).

```sh
cd tools/uitest
npm install
UITEST_URL=http://localhost:3334 npx playwright test            # :3334 is the default
npx playwright test tests/results                               # one area
npx playwright show-trace test-results/<test>/trace.zip         # a failure's trace
```

- The dev server reads `./dist` from disk: after a UI change, `tools/acore/dock.sh exec make dist/wotlk/.dirstamp`
  is enough. A Go change needs a restart.
- `UITEST_BROWSER=chromium` uses Playwright's pinned Chromium instead, after `npx playwright install chromium`.
- Each test gets a fresh browser context, so localStorage never leaks between tests.

## Layout

| Path | Holds |
|---|---|
| `tests/lib/page.ts` | `SPECS`, `openSim(page, spec)`, `openRaid`, `openSimTab(page, id)`, `watchForErrors(page)` |
| `tests/lib/fixture.ts` | `loadFixture(name)`, `showFixture(page, fixture)`, `label(player)` |
| `tests/smoke.spec.ts` | every spec page, the raid page and the results page load without a console error |
| `tests/<area>/` | one directory per area: `results`, `gear`, `raid`, `settings` |

## Fixtures

The results page takes its data from `postMessage`, so tests replay a recorded run instead of simulating
one. `showFixture` posts the settings first, as the sim page does: until then only the damage tab shows.
`loadFixture` derives each raider's label, raid slot, unit index and DPS from the run, so tests never
hard-code sim numbers.

| Fixture | Holds |
|---|---|
| `raid25.simrun.json.gz` | the 25-player P1 raid, 100 iterations, no log |
| `raid25-logged.simrun.json.gz` | the same raid over 15 s, 10 iterations, with the first iteration's log |

Both come from `sim/optimizer/raidctx/testdata/raid25.json` through `gen_fixture`:

```sh
tools/acore/dock.sh exec go run --tags=with_db ./tools/uitest/gen_fixture \
  -infile sim/optimizer/raidctx/testdata/raid25.json \
  -outfile tools/uitest/fixtures/raid25-logged.simrun.json.gz -iterations 10 -duration 15
```

Flags: `-iterations`, `-seed` (101), `-logs` (on), `-duration` (overrides the encounter's). A name ending in
`.gz` is written gzipped. Only a proto change calls for regenerating: the tests read their numbers from the
fixture, so sim changes don't break them.

## Gotchas

- The repo `tsconfig.json` excludes this directory: its Playwright types break `dock.sh tsc`.
- The healing tab has its own topline: scope a locator to `.damage-content` for the damage one.
