# perf

Finds where the sim and the optimizer spend their time: CPU and heap profiles of a CLI run or the live
server, a `runtime/trace` read as threads busy per time window and per optimizer stage, and a scenario
harness that times fixed requests across GOMAXPROCS against a baseline. Run everything through
`tools/acore/dock.sh` ([dev-environment](../../docs/guide/dev-environment.md)).

## Profile a CLI run

`wowsimcli sim` and `optimize` take `--cpuprofile`, `--memprofile` and `--trace`, each a file path:

```sh
tools/acore/dock.sh exec bash -c 'go build --tags=with_db -o tmp/wowsimcli ./cmd/wowsimcli &&
  ./tmp/wowsimcli optimize --infile tmp/quick.json --outfile tmp/opt.json \
    --cpuprofile tmp/opt.cpu --memprofile tmp/opt.mem --trace tmp/opt.trace'
tools/acore/dock.sh exec go tool pprof -top tmp/opt.cpu
```

- `--memprofile` is written at the end, after a GC. Like `go test -memprofile`, pprof shows allocations
  since the start; `-sample_index=inuse_space` shows what's still live.
- A request: `sim/optimizer/testdata/search/fury_p1.json` runs at Normal; set its `settings.effort` to
  `OptimizerEffortQuick` for a run of about 10 s. Its `base` plus `simOptions.iterations` is a
  `sim` request.

## Profile the server

`--pprof <addr>` serves `net/http/pprof` on a listener of its own, with mutex and block profiling on. Off
by default; the sim's own port never serves `/debug/pprof/`. In a container, listen on `:6060` and
publish it on the host's loopback only:

```sh
# dev server
MSYS_NO_PATHCONV=1 docker run -d --name wotlk-dev -p 3334:3333 -p 127.0.0.1:6060:6060 -v <worktree>:/wotlk \
  -v wowsim-gomod:/go/pkg/mod -v wowsim-gocache:/root/.cache/go-build -w /wotlk wowsim-toolchain \
  sh -c 'make dist/wotlk/.dirstamp devserver && ./wowsimwotlk --usefs=true --launch=false --host=":3333" --pprof=":6060"'
# prod: its entrypoint takes flags after the image name
docker run -d --name wowsims-wotlk --restart unless-stopped -p 3333:3333 -p 127.0.0.1:6060:6060 \
  wowsims-wotlk --pprof=:6060
```

## Live capture

`capture.sh` captures, over the same seconds, a CPU profile and a trace from a server started with
`--pprof`, plus that stretch's mutex and block profiles and a heap profile at its end. It then prints
the CPU profile's top entries and the trace's per-region table (below). Run it on the host:

```sh
tools/perf/capture.sh -a 127.0.0.1:6060 -s 10   # -h for the flags
```

- Only what runs inside the window counts: start the run in the UI, then the script, or the reverse.
- Files land in `tmp/perf/capture-<time>/`.
- A trace started mid-run misses the optimizer stage already underway: its region began before
  tracing did. The stages after it show.

## Trace utilization

```sh
tools/acore/dock.sh run ./tools/perf trace [-window 100ms] tmp/opt.trace
```

Per window: `busy`, goroutines running at once over GOMAXPROCS; `util`; `gc`, the share of busy spent
in GC mark workers and mark assists; and the region covering most of the window. Then a line per trace
region (the optimizer's stages, Setup to Alternatives) and the total, which are all `-window 0` prints.

- Idle-priority GC workers and syscalls don't count as busy.
- `busy` counts goroutines the scheduler runs, whether or not the OS gave their thread a core, so a
  loaded machine inflates it. The CPU profile's total is the real CPU.

## Scenario harness

`bench` runs fixed requests through the web server's entry points (`core.RunRaidSimAsync`,
`core.StatWeightsAsync`, `core.RunBulkSimAsync`, and `optimizer.Optimize`, which the server's
`optimizer.RunAsync` wraps) at each GOMAXPROCS of a sweep:

```sh
tools/acore/dock.sh run --tags=with_db ./tools/perf bench [flags]   # -h: flags; -list: scenarios
# a quick check: one sim, one Quick optimization, two points
tools/acore/dock.sh run --tags=with_db ./tools/perf bench -run '^sim/rogue$|^optimizer/quick/fury' -procs 1,16
```

Scenarios, requests in `harness/scenarios/`:
- `sim/<spec>`, `sim/raid`: each `BenchmarkSimulate`'s request at 3000 iterations (the UI's), seed 101.
- `statweights/rogue`: the rogue bench's request on the rogue UI's EP stats. `bulk/rogue`: six P2 combat items at
  the bulk tab's defaults (combinations, fast mode, auto enchant).
- `optimizer/{quick,normal}/<spec>_p<phase>`: the six requests of the optimizer's slow suite (`slow_test.go`).

Columns, per point (median over its runs):
- `wall`; `cpu`, user plus system from getrusage; `util`, cpu / wall / GOMAXPROCS; `gc`, the GC's share of the
  runtime's CPU estimate (idle-priority mark workers left out); `alloc`, bytes allocated; `peak heap`, heap
  objects sampled every 10 ms; `speedup`, wall at the scenario's narrowest point over this one's.
- Optimizer only: `workers`; `sims`, iterations simmed; `busy`, `optimizer.SimBusyTime` over wall × workers.
  The same per stage, below the table.

Each scenario first sims its (base) request at 100 iterations untimed, and each run starts after a GC.
Workers default to GOMAXPROCS, the CLI's default, so the curve runs 1 to 16; the web server runs one fewer
(`-workers` fixes it). Stat weights, the bulk sim and a sim's shards size their goroutine pools by
GOMAXPROCS, so they follow the sweep; a sim runs GOMAXPROCS-1 shards, so at 2 it runs one.

Output, in `-o` (default `tmp/perf/bench-<time>/`): `report.json` with every run, rewritten after each point
so a stopped run keeps what finished; `report.txt`, the printed summary (`perf show report.json` reprints
it); with `-profiles` (on), `<scenario>.cpu.pprof`, the widest point's last run. Top entries:

```sh
tools/acore/dock.sh exec bash -c 'for f in tmp/perf/bench-<time>/*.cpu.pprof; do echo "== $f"; go tool pprof -top -nodecount=15 $f 2>/dev/null; done'
```

### Full baseline

With nothing else running (read `docker stats` first and put the worldserver's load in `-note`). `go run`
records no commit, so pass it:

```sh
tools/acore/dock.sh run --tags=with_db ./tools/perf bench -note 'worldserver 0.6 cores' -commit "$(git rev-parse --short HEAD)"
cp tmp/perf/bench-<time>/report.json tools/perf/baseline.json
```

Its numbers fill the [INVESTIGATION](../../docs/sim-performance/sim-performance.INVESTIGATION.md)'s baseline
tables. About 30 min on the idle 7800X3D: the optimizer at Normal 9, Quick's sweep 17, the rest 4. The
defaults and why:
- `-procs 1,2,4,8,12,16` for everything but Normal: each scenario's full curve, 8 being the core count.
- `-reps 3` at the widest point, `-sweep-reps 1` elsewhere: the comparison's noise comes from the widest
  point, the one the server runs at; a curve point needs one run.
- `-normal-procs 16`, `-normal-reps 1`: combat rogue's Normal run took 14 times its Quick one (7.4 min,
  loaded), and Quick's curve already shows how the stages scale.

### Comparison

`bench` compares with `tools/perf/baseline.json` when it exists (`-baseline`); `perf compare <base.json>
<new.json>` compares any two. A point both reports have flags `wall`, `cpu`, `alloc` or `peak heap` when its
median moved past `-floor` (default 5%; 25% for peak heap, which GC timing moves by 10% between runs) and past
the noise, the wider of the two reports' (max - min) / median, and no sample of one report overlaps the
other's. Any flagged rise exits 1; a point that ran other optimizer workers than the baseline's gets a warning.
At one run per point only the floor applies, so compare curve points by eye.

Take both runs on an idle machine: the noise it measures is within a run, and other load that comes or goes
between runs moves scenarios by a third or more, which it flags like a code change.

### Scenario requests

`harness/scenarios/update.sh` rewrites them: it runs every `BenchmarkSimulate` with a `go test -overlay` hook
that dumps its request, and the slow suite's request builder. Rerun it when those change, then take a new
baseline: the requests are pinned so a comparison only ever sees code changes.

```sh
tools/acore/dock.sh exec bash tools/perf/harness/scenarios/update.sh
```

## Tests

`tools/acore/dock.sh test ./tools/perf/...` traces a small workload in-process, checks the optimizer
evaluator's busy-time counter (`optimizer.SimBusyTime`), and runs the harness on one sim at two points
and its comparison on planted moves.
