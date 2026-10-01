# simval fixture

`simval.jsonl` is a capture of mod-sim-validation's e2e records. `tools/simval`'s `TestAgainstServerRecords`
replays it through `CheckRecord` on every `go test`, and `int-check.sh` runs it too. Recapture it
(never append) per [testing](../../../../docs/guide/testing.md#against-the-server);
`TestFixtureHasEveryE2ERecord` lists what a full run records.

A record that disagrees with the sim is a real mismatch: it's dropped from the file and listed here, and
the sim isn't changed.

## Dropped records

| Record (command, spell, class, dummy) | Disagreement |
|---|---|
| melee, unarmed warrior, boss dummy (the armed warrior's record under the same key stays) | the server rolls 51 normal, 102 crit, 35 glancing, 10 blocked; the model 49, 98, 34, 8, at equal attack power |

## Spells missing from serverdata

`critCapable` can't read their crit capability, so it assumes they crit.

| Spell | Why |
|---|---|
| 75 Auto Shot | not in `sim/core/serverdata` |
