# Testing

How to verify a change. Run every command in the toolchain container ([dev-environment.md](dev-environment.md)).

## Go

- Tests: `go test --tags=with_db $(go list ./sim/... | grep -v /sim/web$) ./tools/...`
  - `sim/web` needs `binary_dist/dist.go`.
  - Add `-count=1` when results come back `(cached)`.
  - Ready to paste from Git Bash:
    `MSYS_NO_PATHCONV=1 docker run --rm -v G:/DevStuff/GitHub/wowsimwotlk:/wotlk -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build -w /wotlk wowsims-wotlk-dev sh -c 'go test --tags=with_db $(go list ./sim/... | grep -v /sim/web$) ./tools/...'`
  - `dock.sh test [args]` runs `./sim/...` by default and generates `binary_dist` itself.
- Vet: `go vet ./sim/... ./tools/...`
- gofmt reads CRLF as a diff, so use `tr -d '\r' < f | gofmt -l`. `sim/warrior/rend.go` already fails
  on master.
- Float asserts need a tolerance, e.g. `math.Abs(got-want) > 0.001`.
- Reviewers build and test too. A review that skipped the build ("Go not installed") once missed a
  compile error.

## Goldens

- There are 37 `sim/**/*.results` files. Each test run writes a `.results.tmp` next to its golden.
- Never `make update-tests`: it deletes every golden before copying.
  - Instead, check each suite's delta for sign and size, then promote only that directory
    (`dock.sh delta`, then `dock.sh promote <dir>`).
  - Without `dock.sh`, promote by hand:
    `for f in <dir>/*.results.tmp; do cp "$f" "${f%.tmp}"; done`.
  - `delta` compares DPS only. Diff the two files for stats, casts and stat weights.
- A `.results` that shows as modified with an empty `git diff` is CRLF noise: leave it out of commits.
- Two sessions testing in one worktree mix each other's changes and overwrite each other's `.tmp` files.
  Promote only when both are done.
- To commit one phase out of a worktree that holds two, rebuild its tree in scratch from `git archive HEAD`,
  re-promote its goldens there, then stage that exact tree: `git add -A` into a scratch `GIT_INDEX_FILE`,
  `git diff-index --cached HEAD`, and feed the result to `git update-index --index-info`.

## UI

- Type-check: `npx tsc --noEmit`.
- `npm run lint:js` already fails on master (`bulk_tab.ts`, `equipped_item.ts`, `importers.ts`). Compare
  each touched file against HEAD instead:
  `git -c safe.directory=/wotlk show HEAD:$f | npx eslint --stdin --stdin-filename $f` vs `npx eslint $f`.
- Add a new import to the existing import line for that module (`import/no-duplicates`).
- `npm run build` and `npm test` call bazel and don't work.

## Against the server

- `tools/simval` replays simval records captured on the server:
  `dock.sh run ./tools/simval -records /wotlk/sim/core/testdata/simval/simval.jsonl`.
- Live e2e, which needs the user's OK: `cd [ac]/modules/mod-sim-validation && ./e2e/run.sh [TestName]`,
  about 30 s. The module's `README.md` covers its commands, env vars and orphan cleanup.
  - The suite deletes its own records, and the host side of the log mount is stale. To keep a fixture,
    stream it from inside the container while the suite runs, then dedupe:
    `docker exec ac-worldserver tail -F -n +1 /azerothcore/env/dist/logs/simval/simval.jsonl > stream.jsonl &`
- Roster export: `tools/database/acraid/crosscheck.py`
  ([acraid README](../../tools/database/acraid/README.md#cross-check)).
- Parity is done when:
  - computed chances match within ±1 bp
  - rolled rates are within |z| ≤ 5
  - multipliers are within ±0.1%
  - recorded runs are within ±2% DPS
