#!/usr/bin/env bash
#
# Builds apldbg twice into tmp/apldbg: `new` from the working tree, and `base` with every Go file under the
# paths that differs from a commit swapped for that commit's copy (go build -overlay). Running both on one
# rotation tells a rotation change from a model change. Run it on the host, from anywhere in the checkout:
#
#   tools/apldbg/base.sh <commit> [path ...]     (default path: sim)
#   tools/acore/dock.sh exec tmp/apldbg/base -spec frostuh -rows
#   tools/acore/dock.sh exec tmp/apldbg/new -spec frostuh -rows
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
base=${1:?usage: tools/apldbg/base.sh <commit> [path ...]}
shift
paths=("$@")
[ ${#paths[@]} -gt 0 ] || paths=(sim)

out=$root/tmp/apldbg
rm -rf "$out/src" "$out/overlay.json"
mkdir -p "$out/src"

# The container can't run git in a worktree, so the copies are made here. They're named .txt: a .go file
# under tmp/ would join the module.
entries=()
# the presets' characters stay the working tree's, so both builds run the same one
presets=(sim/deathknight/dps/suites.go)
is_preset() {
	local file
	for file in "${presets[@]}"; do
		[ "$file" = "$1" ] && return 0
	done
	return 1
}
swap() {
	local file=$1 copy
	copy=$(printf '%s' "$file" | tr / _).txt
	git -C "$root" show "$base:$file" >"$out/src/$copy"
	entries+=("\"/wotlk/$file\": \"/wotlk/tmp/apldbg/src/$copy\"")
}
leave_out() {
	entries+=("\"/wotlk/$1\": \"\"")
}
while IFS=$'\t' read -r status file; do
	case "$file" in
	*_test.go) continue ;;
	*.go) ;;
	*) continue ;;
	esac
	is_preset "$file" && continue
	# a file the commit doesn't have is left out of the base build
	if [ "$status" = A ]; then leave_out "$file"; else swap "$file"; fi
done < <(git -C "$root" diff --name-status --no-renames "$base" -- "${paths[@]}")
while IFS= read -r file; do
	case "$file" in
	*_test.go) continue ;;
	*.go) is_preset "$file" || leave_out "$file" ;;
	esac
done < <(git -C "$root" ls-files --others --exclude-standard -- "${paths[@]}")

{
	printf '{"Replace": {\n'
	for i in "${!entries[@]}"; do
		[ "$i" = 0 ] || printf ',\n'
		printf '\t%s' "${entries[$i]}"
	done
	printf '\n}}\n'
} >"$out/overlay.json"
echo "${#entries[@]} files differ from $base under ${paths[*]}"

bash "$root/tools/acore/dock.sh" exec go build --tags=with_db -o tmp/apldbg/new ./tools/apldbg
bash "$root/tools/acore/dock.sh" exec go build --tags=with_db -overlay tmp/apldbg/overlay.json -o tmp/apldbg/base ./tools/apldbg
echo "built tmp/apldbg/new and tmp/apldbg/base"
