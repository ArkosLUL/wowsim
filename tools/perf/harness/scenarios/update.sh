#!/usr/bin/env bash
#
# Rebuilds the harness's scenario requests: every BenchmarkSimulate's request and the optimizer slow
# suite's six. Run it in the toolchain container, from the checkout's root:
#
#   tools/acore/dock.sh exec bash tools/perf/harness/scenarios/update.sh
set -euo pipefail

here=tools/perf/harness/scenarios
if [ ! -f "$here/update.sh" ]; then
	echo "run it from the checkout's root" >&2
	exit 1
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir "$work/out"

# the benches build their requests inline, and all of them hand them to one of these two helpers.
# A Windows checkout has CRLFs, which would keep $ from matching.
tr -d '\r' <sim/core/test_utils.go |
	sed 's/^\trsr\.Encounter\.Duration = LongDuration$/&\n\tperfDumpRequest(rsr)/' >"$work/test_utils.go"
hooks=$(grep -c '^	perfDumpRequest(rsr)$' "$work/test_utils.go" || true)
if [ "$hooks" != 2 ]; then
	echo "expected to hook RaidBenchmark and RaidBenchmarkIterations in sim/core/test_utils.go, hooked $hooks" >&2
	exit 1
fi
cat >"$work/overlay.json" <<EOF
{"Replace": {
	"$PWD/sim/core/test_utils.go": "$work/test_utils.go",
	"$PWD/sim/core/perf_dump.go": "$PWD/$here/_dump/core_dump.go",
	"$PWD/sim/optimizer/perf_dump_test.go": "$PWD/$here/_dump/optimizer_dump_test.go"
}}
EOF

export PERF_DUMP_DIR=$work/out
echo "dumping the BenchmarkSimulate requests"
if ! go test --tags=with_db -vet=off -overlay "$work/overlay.json" -run '^$' -bench '^BenchmarkSimulate$' -benchtime 1x \
	$(go list ./sim/... | grep -v /sim/web$) >"$work/bench.log" 2>&1; then
	cat "$work/bench.log" >&2
	exit 1
fi
echo "dumping the optimizer slow suite's requests"
if ! go test --tags=with_db,optimizer_slow -vet=off -overlay "$work/overlay.json" -count=1 -run '^TestPerfDumpSlowRequests$' \
	./sim/optimizer/ >"$work/opt.log" 2>&1; then
	cat "$work/opt.log" >&2
	exit 1
fi

rm -f "$here"/*.json.gz
for f in "$work"/out/*.json; do
	gzip -9n <"$f" >"$here/$(basename "$f").gz"
done
ls -l "$here"/*.json.gz
