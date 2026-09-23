#!/usr/bin/env bash
#
# Captures a CPU profile and a runtime/trace, over the same seconds, from a sim server started with
# --pprof, plus that stretch's mutex and block profiles and a heap profile at its end. Then it
# summarizes the CPU profile and the trace through dock.sh. See usage below.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
addr=127.0.0.1:6060
seconds=10
out=
analyze=1

usage() {
	cat <<'EOF'
usage: tools/perf/capture.sh [-a addr] [-s seconds] [-o dir] [-n]

  -a  the server's --pprof address as the host reaches it, default 127.0.0.1:6060
  -s  seconds to capture, default 10
  -o  where the files go, default tmp/perf/capture-<time> in the checkout
  -n  capture only: skip the summaries, which need the dir inside the checkout

Start the run in the UI, then this, or this, then the run: only what runs inside the
window gets captured.

Writes cpu.pprof, trace.out, mutex.pprof, block.pprof and heap.pprof, and unless -n, cpu.txt
(go tool pprof -top) and trace.txt (go run ./tools/perf trace).
EOF
}

while getopts "a:s:o:nh" opt; do
	case "$opt" in
	a) addr=$OPTARG ;;
	s) seconds=$OPTARG ;;
	o) out=$OPTARG ;;
	n) analyze= ;;
	*)
		usage
		[ "$opt" = h ] && exit 0 || exit 2
		;;
	esac
done

out=${out:-$root/tmp/perf/capture-$(date +%Y%m%d-%H%M%S)}
mkdir -p "$out"
out=$(cd "$out" && pwd)
base=http://$addr/debug/pprof

if ! curl -fsS -o /dev/null "$base/"; then
	echo "nothing serves pprof on $addr: start the server with --pprof" >&2
	exit 1
fi

echo "capturing $seconds s from $addr into $out"
pids=()
for target in "profile:cpu.pprof" "trace:trace.out" "mutex:mutex.pprof" "block:block.pprof"; do
	curl -fsS -o "$out/${target#*:}" "$base/${target%%:*}?seconds=$seconds" &
	pids+=($!)
done
failed=0
for pid in "${pids[@]}"; do
	wait "$pid" || failed=1
done
curl -fsS -o "$out/heap.pprof" "$base/heap" || failed=1
if [ "$failed" = 1 ]; then
	echo "some captures failed; another CPU profile or trace running at the same time is the usual cause" >&2
	exit 1
fi

[ -n "$analyze" ] || exit 0
case "$out/" in
"$root"/*) rel=${out#"$root"/} ;;
*)
	echo "$out is outside the checkout, so dock.sh can't see it; skipping the summaries" >&2
	exit 0
	;;
esac
rel=$(printf '%q' "$rel")
"$root/tools/acore/dock.sh" exec bash -c "go tool pprof -top -nodecount=40 $rel/cpu.pprof > $rel/cpu.txt && go run ./tools/perf trace $rel/trace.out > $rel/trace.txt"
head -n 20 "$out/cpu.txt"
echo
# the region table and the total, below the per-window lines
awk '/^ *region +start/ { show = 1 } show' "$out/trace.txt"
