# Testing

How to verify a change. Run every command in the toolchain container ([dev-environment.md](dev-environment.md)).

## Go

- Tests: `go test --tags=with_db $(go list ./sim/... | grep -v /sim/web$) ./tools/... ./cmd/...`
  - `sim/web` needs `binary_dist/dist.go`.
  - Add `-count=1` when results come back `(cached)`.
  - On a loaded machine `./tools/simval` can pass go's 10 min default: add `-timeout 45m`.
  - Ready to paste from Git Bash:
    `MSYS_NO_PATHCONV=1 docker compose run --rm toolchain sh -c 'go test --tags=with_db $(go list ./sim/... | grep -v /sim/web$) ./tools/... ./cmd/...'`
  - `dock.sh test [args]` runs `./sim/...` by default and generates `binary_dist` itself.
- Vet: `go vet ./sim/... ./tools/... ./cmd/...`
- Race: `go test --tags=with_db -race ./sim/optimizer/... ./sim/web/...`, plus
  `-run TestDatabaseConcurrentAddAndRead ./sim/core/` for the item DB lock and `-run 'Shard|Merge' ./sim/core/`
  for the Simulate button's shards. Item-effect races need sims
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
- The optimizer's slow suite, which every wave re-runs as the BiS baseline (about 7 min idle):
  `go test --tags=with_db,optimizer_slow -count=1 -timeout 90m -run TestOptimizerSlow -v ./sim/optimizer/`.
  It prints one `slow: spec=… phase=… effort=… J_preset=… J_opt=… delta=…±… dps_preset=… dps_delta=…±…
  norm_<metric>=mean±se/<reference stat>` line per case (J's normalizer is measured fresh, so judge a
  `J_preset` move against `dps_preset`). `J_preset`, the gains and `dps_preset` are against the preset as
  equipped; the pass check is against the preset trimmed to the pool. `-decisions`
  logs what each stage picked. The CLI does the same run end to end:
  `go run --tags=with_db ./cmd/wowsimcli optimize --infile sim/optimizer/testdata/search/fury_p1.json --verbose`,
  whose request comes from `go test --tags=with_db ./sim/optimizer -run TestSearchTestdata -update`.
  `TestCurvesPoolCut` (the slow suite's command, `-run TestCurvesPoolCut`) checks that Fury P1 Normal's pick
  holds when the outclassed cut leaves the pool: cut and uncut within 2 se.
- Iteration i reseeds with `RandomSeed + i`, so runs seeded closer than their iteration count share streams:
  seed independent runs at least that far apart.
- gofmt reads CRLF as a diff, so use `tr -d '\r' < f | gofmt -l`, with the whole pipe in the container:
  `dock.sh exec` doesn't forward stdin.
- Float asserts need a tolerance, e.g. `math.Abs(got-want) > 0.001`.
- Unset, a test's map updates differ in length, as live ([update timing](spell-data-and-timing.md#update-timing)).
  One that counts updates or ticks pins the grid with `core.OnRegularGrid(encounter, loopMs)`: every world tick
  takes `loopMs` and no update strays. For exact 10 ms world ticks set `WorldLoopMs` to 0 beside
  `WorldUpdateIntervalMs`. A bound on a time left has to allow the longest world tick, 59 ms live
  (`unit.DurationThroughLastTick(0)` gives it; `TestTheLongestLiveWorldTickIsUnder60Ms` holds a hardcoded bound
  to it).
- Reviewers build and test too. A review that skipped the build ("Go not installed") once missed a
  compile error.

## Goldens

- There are 37 `sim/**/*.results` files. Each test run writes a `.results.tmp` next to its golden.
- Never `make update-tests`: it deletes every golden before copying.
  - Instead, check each suite's delta for sign and size, then promote only that directory
    (`dock.sh delta`, then `dock.sh promote <dir>`).
  - Without `dock.sh`, promote by hand:
    `for f in <dir>/*.results.tmp; do cp "$f" "${f%.tmp}"; done`.
  - `delta` compares dps, hps, tps and dtps per test, and lists tests that appear or disappear. Under each
    Average test it lists the moved abilities (below), four-space indented, largest damage or healing change
    first. Diff the two files for stats and stat weights.
  - A healer spec's `StatWeights` row holds HPS weights (Int, Spirit, SP, MP5, crit, haste); DPS weights
    would be all zero. A healer suite without `StatsToWeigh` gets those six, SP as reference, from
    `FullCharacterTestSuiteGenerator`. `StatWeightsRequest` has no target dummies, so the healer heals itself.
- `Average` tests also store `ability_results`: one entry per `<unit>|action|<id>` and `<unit>|aura|<id>`,
  `<unit>` being `player`, `pet:<name>` or `target<i>` (target auras only).
  - Fields, per iteration and summed over targets: `casts`, `hits`, `crit_pct`, `amount_per_hit`, `amount`
    (damage + healing + shielding) and, for auras, `uptime_s`. `hits` counts landed hits as the UI does:
    hits, crits, blocks, glances and crushes. Some dots' ticks aren't hits, so only `amount` shows them.
  - Only actions with casts, hits or amount, and auras with uptime. Same-named pets and repeated ids add up.
  - The comparison runs both ways: a key on one side only fails the test.
  - Against a golden without the block, `delta` prints one `abilities new (N entries)` line per test.
- Each suite ends with a `RotationGuard` subtest. It builds every distinct player the suite runs and fails
  on a spell its rotation names but the character doesn't know (`APLRotation.UnknownSpells`), when the spell
  is missing from serverdata or carries the class's family (`serverdata.ClassFamily`).
  - Skipped: other families (racials, other classes' buffs) and tag -1 ids, the raid buffs' cooldowns (a
    shaman's NoBuffs tests drop the raid's Bloodlust).
  - Allowlist: `DroppedSpells` (id → reason) on a `SpecOptionsCombo` or `RotationCombo`, applying to the
    tests that use that combo. An entry without a reason, or one no player drops, fails.
  - A generator outside `FullCharacterTestSuiteGenerator` passes its own allowlist by implementing
    `core.DroppedSpellsGenerator`; `JoinTestGenerators` forwards it.
- Only `Average` runs 2000 iterations; the other rows run 20 (`DefaultSimTestOptions`), so a row's move can
  be sampling. A tank's SwitchInFrontOfTarget dtps moved +13.7% in the golden for +0.1% at 400,000 iterations:
  rerun such a row at depth before reading its sign. TestProtection's Average dtps scatters by 2.8% too, on
  1.1 hits a fight.
- A suite's Average-Default row reruns at depth, under server settings of your own, to tell a real move from
  noise and one setting's share of it (`suiteDepthRun` in `sim/core/test_suite.go`):
  `SUITE_DEPTH_ITERATIONS=200000 SUITE_DEPTH_DIR=/wotlk/tmp/depth SUITE_DEPTH_SETTINGS='{"worldLoopMs":0}' go test --tags=with_db -run 'TestBM$' ./sim/hunter`
  - It writes `<dir>/TestBM.results`, or `.results.tmp` with `SUITE_DEPTH_NEW=1`, so `dock.sh delta <dir>` reads
    one run against another. The directory must be absolute.
  - The settings are a `proto.ServerSettings` in JSON, laid over the suite's. Nothing is held to the goldens.
- Not real coverage of their spec: the three healer suites (Resto Shaman, Holy Paladin, Resto Druid)
  only autocast cooldowns, so they check stats and item heals only (their HPS weight rows are all zero);
  Subtlety runs a Combat APL.
- A `.results` that shows as modified with an empty `git diff` is CRLF noise: leave it out of commits.
- `sim/optimizer/raidctx/testdata/raid25.derived.json` is a golden too. Rewrite it with
  `go test --tags=with_db ./sim/optimizer/raidctx -run TestDeriveFixtureGolden -update`.
- `TestReplayFixtures` replays requests the UI's pool builder really produced
  (`sim/optimizer/testdata/replay/*.json.gz`, written by the driver in
  [ui/core/optimizer](../../ui/core/optimizer/README.md)). An exported request drops in as is.
  `TestReplayEquippedGear` checks every fixture's owned items; it needs `equipped` on the driver's `.json.gz`
  files and skips an export without it. `outclassed_test.go` checks the fixtures' pools leave out outclassed
  gear except where it's worn.
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

Simval replays, the live e2e, the offline cross-checks against server captures and the parity bar are in
[server validation](server-validation.md).
