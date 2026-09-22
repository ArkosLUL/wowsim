# acbis

Turns BiS optimizer results into the dataset `mod-bis-tooltip` serves to the BisTooltipAC addon: an SQL
import for `acore_world`. It never writes to the server; its only query is a SELECT of the roster's
character guids.

This is the CLI. The logic lives in `tools/database/azerothcore/`:
- `bisdata_wire.go`: block payload, BLK framing, checksum and composition fingerprint, shared with the
  module and the addon. The format, and what the dataset version and the fingerprint hash, live in
  `[ac]/modules/mod-bis-tooltip/README.md`
- `bisdata_dataset.go`: subjects, blocks and the dataset version
- `bisdata_slots.go`: the addon's class, spec, slot and phase names
- `bisdata_sql.go`, `bisdata_lua.go`: the two outputs

## Inputs

- `-batch`: the raid batch's `OptimizerBatchExport`. Each raider and phase keeps its latest stage
  that didn't fail, warning about any later one that did.
- `-results`: an index of `OptimizerResult` files, paths relative to the index. Each names a raider from
  the roster, or an addon class and spec (`Bistooltip_spec_icons` keys, e.g. `Fire FFB`):

  ```json
  {"results": [
    {"file": "deathsong-p4.json", "raider": "Deathsong"},
    {"file": "mage-fire-p4.json", "class": "Mage", "spec": "Fire"}
  ]}
  ```

- `-roster`: acraid's roster JSON, for raider identity, raid order, specs and the composition fingerprint.

The content phase comes from each result's settings.

## Running

Go isn't installed on the host, so it runs in the toolchain image. Generate the protos first if
`sim/core/proto/*.pb.go` is missing (see `docs/guide/dev-environment.md`). From Git Bash, with `SCRATCH`
holding the inputs:

```sh
MSYS_NO_PATHCONV=1 docker run --rm --network azerothcore-wotlk-pb_ac-network \
  -v G:/DevStuff/GitHub/wowsimwotlk:/wotlk -v "$SCRATCH:/scratch" \
  -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build -w /wotlk wowsims-wotlk-dev \
  go run ./tools/database/acbis -dsn "root:password@tcp(ac-database:3306)/" \
  -batch /scratch/batch.json -results /scratch/results/index.json -roster /scratch/raid.json \
  -out /scratch/bis_dataset.sql -luaOut /scratch/bis_dataset.lua
```

It prints the dataset's version and provenance, one line per subject with its payload bytes and BLK
frames, how long sending them all takes at 10 frames per 100 ms tick, and a `!` line per warning.
Skipped results (failed runs, unknown specs, raiders missing from the roster) are warnings; two results
for one subject and phase, or a raider without a roster or guid, are errors.

Importing replaces the server's dataset and needs the user's OK:

```sh
MSYS_NO_PATHCONV=1 docker exec -i ac-database mysql -uroot -ppassword acore_world < "$SCRATCH/bis_dataset.sql"
```

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-out` | required | the SQL import to write |
| `-batch`, `-results` | | at least one; both can be given |
| `-roster` | | needed when a result names a raider |
| `-dsn` | `root:password@tcp(127.0.0.1:3306)/` | only for the roster's guids |
| `-simDb` | `assets/database/db.json` | gives each enchant effect its spell |
| `-luaOut` | | the dataset decoded the way the addon decodes it, to check the addon's decoder against |

## Output

- The import creates `bistooltip_dataset`, `bistooltip_subject` and `bistooltip_block` if they're
  missing, then swaps the whole dataset in one transaction.
- Only raiders with results become subjects; the fingerprint still covers the whole roster.
- A raider's spec comes from their main talent tree, so builds within a tree (Fury-Prot) read as the
  tree's spec. The main tank flag picks Blood tank and Feral tank.
