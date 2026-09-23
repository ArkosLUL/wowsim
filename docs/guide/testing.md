# Testing

How to verify a change. Run every command in the toolchain container ([dev-environment.md](dev-environment.md)).

## Go

- Tests: `go test --tags=with_db $(go list ./sim/... | grep -v /sim/web$) ./tools/... ./cmd/...`
  - `sim/web` needs `binary_dist/dist.go`.
  - Add `-count=1` when results come back `(cached)`.
  - Ready to paste from Git Bash:
    `MSYS_NO_PATHCONV=1 docker run --rm -v G:/DevStuff/GitHub/wowsimwotlk:/wotlk -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build -w /wotlk wowsims-wotlk-dev sh -c 'go test --tags=with_db $(go list ./sim/... | grep -v /sim/web$) ./tools/... ./cmd/...'`
  - `dock.sh test [args]` runs `./sim/...` by default and generates `binary_dist` itself.
- Vet: `go vet ./sim/... ./tools/... ./cmd/...`
- Race: `go test --tags=with_db -race ./sim/optimizer/... ./sim/web/...`, plus
  `-run TestDatabaseConcurrentAddAndRead ./sim/core/` for the item DB lock. Item-effect races need sims
  running in parallel on gear that has the effect, which the unit tests' presets lack:
  `-race -run '^$' -bench BenchmarkOptimizerEval -benchtime=1x ./sim/optimizer/`.
- Benchmarks, both under `go test --tags=with_db -run '^$' -bench`:
  - `BenchmarkOptimizerEval ./sim/optimizer/`, with its numbers in the
    [BiS INVESTIGATION](../bis-optimizer/bis-optimizer.INVESTIGATION.md#performance).
  - `BenchmarkSimulate ./sim/rogue/ ./sim/paladin/retribution/ ./sim/hunter/ ./sim/shaman/elemental/ ./sim/`
    for sim throughput, covering melee, mana melee, ranged with a pet, caster and a raid. Every wave records
    it ([wave loop](../wave-loop/wave-loop.PLAN.md#sim-throughput)), because no golden measures time.
    Run it on an idle machine, with no sim, container build or golden run alongside: what else is
    running moves them all by a fifth or more at once, which is enough to hide or invent a regression.
    Each spec case copies its golden suite's default player; `./sim/` is an 8-player raid on its specs'
    golden APLs and `StandardTalents`. All five run `iterations=1` and `iterations=100` through
    `core.RaidBenchmarkIterations` (the holy and protection paladin and resto shaman benches use
    `core.RaidBenchmark`), and the raid's 100 takes about 0.3 s an op, so give it a `-benchtime` of several.
  - A/B against a base without touching the tree: `go test -c -overlay overlay.json`, whose `Replace`
    maps each changed file to a `git show <base>:<path>` copy under `tmp/`. Interleave the two binaries
    for 10+ rounds of `-test.count 1`, appending to `base.txt` and `new.txt`, then
    `benchstat base.txt new.txt` (in the toolchain image): its medians hold up on a busy machine.
  - Profiles, traces, and a harness timing every bench's request, stat weights, bulk and the optimizer across
    GOMAXPROCS against a baseline: [tools/perf](../../tools/perf/README.md).
- The optimizer's slow suite, which every wave re-runs as the BiS baseline (about 8 min):
  `go test --tags=with_db,optimizer_slow -count=1 -timeout 90m -run TestOptimizerSlow -v ./sim/optimizer/`.
  It prints one `slow: spec=… phase=… effort=… J_preset=… J_opt=… delta=…±… dps_preset=… dps_delta=…±…
  norm_<metric>=mean±se/<reference stat>` line per case (J's normalizer is measured fresh, so judge a
  `J_preset` move against `dps_preset`), and
  `-decisions` logs what each stage picked. The CLI does the same run end to end:
  `go run --tags=with_db ./cmd/wowsimcli optimize --infile sim/optimizer/testdata/search/fury_p1.json --verbose`,
  whose request comes from `go test --tags=with_db ./sim/optimizer -run TestSearchTestdata -update`.
- gofmt reads CRLF as a diff, so use `tr -d '\r' < f | gofmt -l`, with the whole pipe in the container:
  `dock.sh exec` doesn't forward stdin. `sim/warrior/rend.go` already fails on master.
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
  - `delta` compares dps, hps, tps and dtps per test, and lists tests that appear or disappear. Diff the
    two files for stats, casts and stat weights.
- A `.results` that shows as modified with an empty `git diff` is CRLF noise: leave it out of commits.
- `sim/optimizer/raidctx/testdata/raid25.derived.json` is a golden too. Rewrite it with
  `go test --tags=with_db ./sim/optimizer/raidctx -run TestDeriveFixtureGolden -update`.
- `TestReplayFixtures` replays requests the UI's pool builder really produced
  (`sim/optimizer/testdata/replay/*.json.gz`, written by the driver in
  [ui/core/optimizer](../../ui/core/optimizer/README.md)). An exported request drops in as is.
- Two sessions testing in one worktree mix each other's changes and overwrite each other's `.tmp` files.
  Promote only when both are done.
- To commit one phase out of a worktree that holds two, rebuild its tree in scratch from `git archive HEAD`,
  re-promote its goldens there, then stage that exact tree: `git add -A` into a scratch `GIT_INDEX_FILE`,
  `git diff-index --cached HEAD`, and feed the result to `git update-index --index-info`.

## UI

- Behaviour: the Playwright suite in `tools/uitest` ([README](../../tools/uitest/README.md)), against a dev
  server. Verify a UI change there, with a test for the behaviour it changes.
- Type-check: `npx tsc --noEmit`.
- `npm run lint:js` already fails on master (`bulk_tab.ts`, `equipped_item.ts`, `importers.ts`). Compare
  each touched file against HEAD instead:
  `git -c safe.directory=/wotlk show HEAD:$f | npx eslint --stdin --stdin-filename $f` vs `npx eslint $f`.
  In a worktree, git fails in the container (`.git` points outside the mount): `git show` the HEAD copy
  into the gitignored `tmp/` on the host, then lint `< tmp/$f` in the container. Name that copy `.txt`:
  `tsconfig.json` includes `.` with `allowJs`, so a stray `.ts`, `.js` or `.cjs` file under `tmp/` fails the
  type-check and `make dist`.
- Add a new import to the existing import line for that module (`import/no-duplicates`).
- `npm run build` and `npm test` call bazel and don't work.

## Against the server

- `tools/simval` replays simval records captured on the server, and rebuilds naked `info` characters in
  the sim:
  `dock.sh run ./tools/simval -records /wotlk/sim/core/testdata/simval/simval.jsonl`.
- Live e2e, which needs the user's OK: `cd [ac]/modules/mod-sim-validation && ./e2e/run.sh [TestName]`,
  about 2 min. The module's `README.md` covers its commands, env vars and orphan cleanup.
  - The suite deletes its own records, and the host side of the log mount is stale. To keep a fixture,
    stream it from inside the container while the suite runs, then dedupe:
    `docker exec ac-worldserver tail -F -n +1 /azerothcore/env/dist/logs/simval/simval.jsonl > stream.jsonl &`
- Spell data: `tools/acore/spellids` lists the spell ids the sim needs and checks them against the committed
  capture. Its README's [Capturing](../../tools/acore/spellids/README.md#capturing) section recaptures
  missing ones live.
- Roster export: `tools/database/acraid/crosscheck.py`
  ([acraid README](../../tools/database/acraid/README.md#cross-check)).
- Parity is done when:
  - computed chances match within ±1 bp
  - rolled rates are within |z| ≤ 5
  - multipliers are within ±0.1%
  - recorded runs are within ±2% DPS
