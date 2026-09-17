#!/usr/bin/env bash
#
# Runs the sim's Go/TS toolchain in the Dockerfile's `toolchain` image, since
# neither Go nor node is installed on the host. See `usage` below.
set -euo pipefail

IMAGE=${WOWSIM_IMAGE:-wowsim-toolchain}
DBC_DIR=${DBC_DIR:-A:/WOW/dbc}
AC_DIR=${AC_DIR:-G:/DevStuff/GitHub/azerothcore-wotlk-pb}
GOMOD_VOLUME=${GOMOD_VOLUME:-wowsim-gomod}
GOCACHE_VOLUME=${GOCACHE_VOLUME:-wowsim-gocache}

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)

usage() {
	cat <<'EOF'
usage: tools/acore/dock.sh <command> [args]

  build             (re)build the toolchain image
  proto             regenerate the Go and TypeScript protobuf code
  test [args]       go test --tags=with_db, default ./sim/... ; args go to `go test`
  tsc               type-check the UI
  run <pkg> [args]  go run a tool package, e.g. run ./tools/database/acdiff
  exec <cmd> [args] anything else inside the container
  dps [dir]         print DPS per test from the .results goldens under dir
  delta [dir]       DPS of the last run against the goldens; stats, casts and
                    stat weights only show up in a plain diff of the two files
  promote <dir>     copy that dir's .results.tmp over its .results

Mounts: the checkout at /wotlk, DBCs at /dbc, the AzerothCore checkout at /ac.
Override with DBC_DIR and AC_DIR; both are skipped when missing. The host is
reachable as host.docker.internal, so tools can talk to the live MySQL.
EOF
}

# Docker wants host paths, and MSYS rewrites anything that looks like one.
hostpath() {
	case "$(uname -s)" in
	MINGW* | MSYS* | CYGWIN*) (cd "$1" && pwd -W) ;;
	*) (cd "$1" && pwd) ;;
	esac
}

quote() {
	local out=
	local arg
	for arg in "$@"; do
		out+=" $(printf '%q' "$arg")"
	done
	printf '%s' "${out# }"
}

build_image() {
	MSYS_NO_PATHCONV=1 docker build --target toolchain -t "$IMAGE" "$(hostpath "$root")"
}

ensure_image() {
	if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
		echo "building $IMAGE" >&2
		build_image
	fi
}

dock() {
	ensure_image
	local args=(
		--rm
		-w /wotlk
		-v "$(hostpath "$root"):/wotlk"
		-v "$GOMOD_VOLUME:/go/pkg/mod"
		-v "$GOCACHE_VOLUME:/root/.cache/go-build"
		--add-host=host.docker.internal:host-gateway
	)
	if [ -d "$DBC_DIR" ]; then
		args+=(-v "$(hostpath "$DBC_DIR"):/dbc:ro")
	fi
	if [ -d "$AC_DIR" ]; then
		args+=(-v "$(hostpath "$AC_DIR"):/ac:ro")
	fi
	if [ -n "${AC_DSN:-}" ]; then
		args+=(-e "AC_DSN=$AC_DSN")
	fi
	MSYS_NO_PATHCONV=1 docker run "${args[@]}" "$IMAGE" bash -c "$1"
}

# sim/web embeds the built client, so ./sim/... won't even compile without it.
ensure_go_deps() {
	dock 'set -e; test -f sim/core/proto/api.pb.go || make sim/core/proto/api.pb.go; make -s binary_dist/dist.go'
}

# Pulls "<test name><tab><dps>" out of a .results prototext file.
dps_pairs() {
	awk '
		/^dps_results: \{/ { inrec = 1; key = ""; dps = ""; next }
		inrec && /^ key: / { key = $0; sub(/^ key: "/, "", key); sub(/"$/, "", key); next }
		inrec && /^  dps: / { dps = $2; next }
		inrec && /^\}/ { if (key != "" && dps != "") print key "\t" dps; inrec = 0 }
	' "$1"
}

cmd_dps() {
	local dir=${1:-sim}
	local file
	while IFS= read -r file; do
		dps_pairs "$file"
	done < <(find "$root/$dir" -name '*.results' | sort)
}

cmd_delta() {
	local dir=${1:-sim}
	local tmp golden changes
	local suites=0 unchanged=0
	while IFS= read -r tmp; do
		golden=${tmp%.tmp}
		suites=$((suites + 1))
		if [ ! -f "$golden" ]; then
			echo "${golden#"$root/"} (no golden yet)"
			dps_pairs "$tmp" | awk -F'\t' '{ printf "  %s\tnew\t%.5f\n", $1, $2 }'
			continue
		fi
		changes=$(awk -F'\t' '
			NR == FNR { old[$1] = $2; next }
			{
				if (!($1 in old)) { printf "  %s\tnew\t%.5f\n", $1, $2; next }
				o = old[$1] + 0; n = $2 + 0
				if (o == n) next
				printf "  %s\t%.5f -> %.5f\t%+.3f%%\n", $1, o, n, (o != 0 ? (n - o) / o * 100 : 0)
			}
		' <(dps_pairs "$golden") <(dps_pairs "$tmp"))
		if [ -n "$changes" ]; then
			printf '%s\n%s\n' "${golden#"$root/"}" "$changes"
		else
			unchanged=$((unchanged + 1))
		fi
	done < <(find "$root/$dir" -name '*.results.tmp' | sort)
	if [ "$suites" = 0 ]; then
		echo "no .results.tmp under $dir; run a test first" >&2
		return 1
	fi
	echo "$unchanged of $suites suites unchanged"
}

# Deliberately per directory: `make update-tests` wipes every golden in the
# repo, which silently promotes suites the run never touched.
cmd_promote() {
	local dir=${1:?promote needs a directory}
	local tmp golden count=0
	while IFS= read -r tmp; do
		golden=${tmp%.tmp}
		# the container writes LF; keep whatever the checkout uses so git stays quiet.
		# grep needs -U here, or MSYS strips the CR before matching.
		if [ -f "$golden" ] && grep -qU $'\r' "$golden"; then
			sed -e 's/\r$//' -e 's/$/\r/' "$tmp" >"$golden"
		else
			cp "$tmp" "$golden"
		fi
		echo "promoted ${golden#"$root/"}"
		count=$((count + 1))
	done < <(find "$root/$dir" -name '*.results.tmp' | sort)
	echo "$count promoted"
}

cmd=${1:-}
[ $# -gt 0 ] && shift || true

case "$cmd" in
build) build_image ;;
proto) dock 'make proto' ;;
test)
	ensure_go_deps
	dock "go test --tags=with_db $(quote "${@:-./sim/...}")"
	;;
tsc) dock 'set -e; make node_modules ui/core/proto/api.ts ui/core/index.ts; npx tsc --noEmit' ;;
run)
	[ $# -gt 0 ] || { usage; exit 1; }
	ensure_go_deps
	dock "go run $(quote "$@")"
	;;
exec)
	[ $# -gt 0 ] || { usage; exit 1; }
	dock "$(quote "$@")"
	;;
dps) cmd_dps "$@" ;;
delta) cmd_delta "$@" ;;
promote) cmd_promote "$@" ;;
*)
	usage
	[ -z "$cmd" ] && exit 1 || exit 2
	;;
esac
