#!/usr/bin/env bash
# RUNBOOK step 4.3 in [int]. BASE is the pre-merge commit: gofmt and eslint look only at files changed since it.
# usage: int-check.sh <base-ref> <log-name>
set -u
INT=G:/DevStuff/GitHub/wowsimwotlk-int
BASE=$1
NAME=$2
DOCK="bash $INT/tools/acore/dock.sh"
LOGS=G:/DevStuff/GitHub/.wave-loop/logs/checks/$NAME
mkdir -p "$LOGS"
cd "$INT" || exit 1

changed_go=$(git diff --name-only --diff-filter=AM "$BASE" HEAD -- '*.go' | tr '\n' ' ')
changed_ui=$(git diff --name-only --diff-filter=AM "$BASE" HEAD -- '*.ts' '*.tsx' '*.js' | grep -v '_auto_gen\.ts$' | tr '\n' ' ')

echo "== test"
$DOCK test -count=1 ./sim/... ./tools/... ./cmd/... >"$LOGS/test.log" 2>&1
echo "exit $?"
grep -E '^(FAIL|--- FAIL|panic)' "$LOGS/test.log" | head -40
grep -cE '^ok ' "$LOGS/test.log" | sed 's/^/ok packages: /'

echo "== delta"
$DOCK delta >"$LOGS/delta.log" 2>&1
grep -v '^  ' "$LOGS/delta.log"

echo "== vet"
$DOCK exec go vet ./sim/... ./tools/... ./cmd/... >"$LOGS/vet.log" 2>&1
echo "exit $?"
tail -20 "$LOGS/vet.log"

echo "== simval"
$DOCK run ./tools/simval -records /wotlk/sim/core/testdata/simval/simval.jsonl >"$LOGS/simval.log" 2>&1
echo "exit $?"
tail -3 "$LOGS/simval.log"

echo "== gofmt ($(echo $changed_go | wc -w) files)"
if [ -n "$changed_go" ]; then
	mkdir -p tmp
	printf 'for f in %s; do if [ -n "$(tr -d "\\r" < "$f" | gofmt -l)" ]; then echo "UNFORMATTED $f"; fi; done; echo gofmt-done\n' "$changed_go" >tmp/gofmt.sh
	$DOCK exec bash tmp/gofmt.sh 2>&1 | tail -20
fi

echo "== tsc"
$DOCK tsc >"$LOGS/tsc.log" 2>&1
echo "exit $?"
tail -5 "$LOGS/tsc.log"

echo "== eslint vs $BASE ($(echo $changed_ui | wc -w) files)"
if [ -n "$changed_ui" ]; then
	rm -rf tmp/eslint-old
	lines=
	for f in $changed_ui; do
		mkdir -p "tmp/eslint-old/$(dirname "$f")"
		if git cat-file -e "$BASE:$f" 2>/dev/null; then
			git show "$BASE:$f" >"tmp/eslint-old/$f.txt"
			lines+="old=\$(npx eslint --stdin --stdin-filename $f < tmp/eslint-old/$f.txt 2>&1 | grep -cE '^ +[0-9]+:[0-9]+ '); "
		else
			lines+="old=0; "
		fi
		lines+="new=\$(npx eslint $f 2>&1 | grep -cE '^ +[0-9]+:[0-9]+ '); echo \"$f old=\$old new=\$new\"; "
	done
	printf '%s\n' "$lines" >tmp/eslint.sh
	$DOCK exec bash tmp/eslint.sh 2>&1 | grep -v "npm notice"
fi
rm -rf tmp/gofmt.sh tmp/eslint.sh tmp/eslint-old
echo "== done"
