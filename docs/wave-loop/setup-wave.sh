#!/usr/bin/env bash
# RUNBOOK step 2: protos (which also runs npm ci), then binary_dist/dist.go, in every worktree given, in parallel.
# usage: setup-wave.sh <worktree>...
set -u
log_dir=G:/DevStuff/GitHub/.wave-loop/logs/setup
mkdir -p "$log_dir"
pids=()
for wt in "$@"; do
	name=$(basename "$wt")
	(
		bash "$wt/tools/acore/dock.sh" proto &&
			bash "$wt/tools/acore/dock.sh" exec make binary_dist/dist.go &&
			echo "SETUP OK"
	) >"$log_dir/$name.log" 2>&1 &
	pids+=($!)
done
for p in "${pids[@]}"; do wait "$p"; done
for wt in "$@"; do
	name=$(basename "$wt")
	echo "$name: $(tail -1 "$log_dir/$name.log")"
done
