# Dev environment

Host, toolchain container, protos, builds, running the sim. For verification, see [testing.md](testing.md).

## Host

- Windows 11, with Git Bash and PowerShell. `core.autocrlf=true`, so working-tree files are CRLF.
- Missing: Go, make, the mysql CLI, `bc`.
- Present: Python 3.14, Docker and Node. `node_modules` was installed from Linux, so npx, tsc and eslint
  only work in the container. `tools/uitest` has its own, installed on the host.
- Versions come from `go.mod` (Go 1.23), `Dockerfile` (`golang:1.23-bookworm`, `node:19.8.1`) and
  `.nvmrc`. The README's versions are stale.

## Toolchain container

The Dockerfile's `toolchain` stage has Go, protoc and node. Build it once, then run commands in it with
the checkout mounted:

```sh
docker build --target toolchain --tag wowsims-wotlk-dev .
MSYS_NO_PATHCONV=1 docker run --rm -v G:/DevStuff/GitHub/wowsimwotlk:/wotlk \
  -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build \
  -w /wotlk wowsims-wotlk-dev sh -c '<command>'
```

- Mount the worktree you're working in, not always [sim].
- From PowerShell, drop `MSYS_NO_PATHCONV=1` and quote the mount: `-v "G:\DevStuff\GitHub\wowsimwotlk:/wotlk"`.
- To reach the live DB as `ac-database:3306`, add `--network azerothcore-wotlk-pb_ac-network`.
- The image has no docker CLI. Copy DBCs out on the host first, or mount the client-data volume
  ([azerothcore-server.md](azerothcore-server.md#dbcs)).
- `tools/acore/dock.sh` wraps all of this. It uses image `wowsim-toolchain`,
  volumes `wowsim-gomod`/`wowsim-gocache`, and mounts DBCs at `/dbc` and [ac] at `/ac`. Subcommands:
  `build`, `proto`, `test [args]`, `tsc`, `run <pkg> [args]`, `exec <cmd>`, `dps`, `delta`,
  `promote <dir>`. `exec` quotes its arguments, so compound commands need `exec bash -c '…'`.
- The `wotlk-toolchain` image is a stray duplicate of the toolchain image.

## Protos

The generated `sim/core/proto/*.pb.go` and `ui/core/proto/*.ts` files are gitignored. Regenerate them
after a fresh clone or any `proto/*.proto` change. Run these lines in the container: host `npx protoc`
fails, and `make proto` can re-run `npm ci`.

```sh
protoc -I=./proto --go_out=./sim/core ./proto/*.proto
npx protoc --ts_opt generate_dependencies --ts_out ui/core/proto --proto_path proto proto/api.proto
npx protoc --ts_out ui/core/proto --proto_path proto proto/test.proto
npx protoc --ts_out ui/core/proto --proto_path proto proto/ui.proto
```

## Build

- `make wowsimwotlk` builds the full server with the UI embedded.
  - `make devserver` compiles only the Go server.
  - `make dist/wotlk/.dirstamp` builds the wasm and vite UI.
- `sim/web` doesn't compile without `binary_dist/dist.go` (`make binary_dist/dist.go`), and neither does
  `go mod tidy`.
- Plain `go build` of a tool drops a binary in the repo root. Use `-o /dev/null`, e.g.
  `go build -o /dev/null ./tools/database/gen_db`.
- Item DB: `make items` runs `gen_db -gen=azerothcore`, the Classic build from `assets/db_inputs/` with
  item data from the live server ([ADR 0002](../adr/0002-item-data-from-live-db.md)). It needs the live DB
  and a copy of the live DBCs ([README](../../tools/database/gen_db/README.md)). Rerun `accatalog`
  ([README](../../tools/database/accatalog/README.md)) after it: the server catalog's Classic fallbacks
  read `db.json`.

## Run

- **Prod container** `wowsims-wotlk`, at http://localhost:3333/wotlk/:

  ```sh
  docker build --build-arg SIM_COMMIT=$(git rev-parse HEAD) --tag wowsims-wotlk .
  MSYS_NO_PATHCONV=1 docker run -d --name wowsims-wotlk --restart unless-stopped -p 3333:3333 \
    --add-host=host.docker.internal:host-gateway \
    -v azerothcore-wotlk-pb_ac-client-data:/acdata:ro \
    --mount type=bind,source=G:/DevStuff/GitHub/.wowsim,target=/wowsim,readonly \
    --mount type=bind,source=G:/DevStuff/GitHub/azerothcore-wotlk-pb/configurationOverrides/Playerbot.env,target=/acconf/Playerbot.env,readonly \
    --env-file G:/DevStuff/GitHub/.wowsim/prod.env \
    wowsims-wotlk --acoreDbcDir=/acdata/dbc \
    --acoreNamesFile=/wowsim/core-raid.txt --acoreLeader=Deathsong \
    --acoreWorldBuffMatrix=/acconf/Playerbot.env
  ```

  - `prod.env` holds one line, `AC_DSN=root:password@tcp(host.docker.internal:3306)/` with the real password,
    unquoted, the final `/` included. It stays outside the repo, and the DSN off the command line.
  - It's distroless: no shell. It serves whatever was built into the image, so rebuild it to see
    changes. The mounts and `--acore*` flags only feed `/acoreRoster` (below).
  - The names file's directory is mounted, not the file: an editor that replaces a file leaves a
    single-file mount stale. `Playerbot.env` is mounted alone because `Chronicle.env` sits beside it.
  - `host.docker.internal`, not the AC network: a container on that network can't start once it's gone.
  - The mounted volume blocks `docker compose down -v` in [ac] until the sim container is removed.
  - Keep the `/wotlk/` path behind a reverse proxy.
  - Without `SIM_COMMIT`, optimizer results say sim "unknown": `.dockerignore` drops `.git`.
  - After a rebuild, reload every open sim tab: a page from the old build keeps sending its old requests.
- **Dev server** on :3334, since prod holds :3333. `--usefs` serves `./dist` from the mounted checkout.
  There's no hot reload: `docker rm -f wotlk-dev` and start it again after changes. With a worktree
  mounted, add `-e SIM_COMMIT=$(git -C <worktree> rev-parse HEAD)`: its `.git` points outside the mount,
  so the container can't read the commit. For `/acoreRoster`, add the prod line's `--add-host`, mounts
  and `--env-file` before the image, and its `--acore*` flags after `--host=":3333"` inside the `sh -c`.

  ```sh
  MSYS_NO_PATHCONV=1 docker run -d --name wotlk-dev -p 3334:3333 -v <worktree>:/wotlk \
    -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build -w /wotlk wowsims-wotlk-dev \
    sh -c 'make dist/wotlk/.dirstamp devserver && ./wowsimwotlk --usefs=true --launch=false --host=":3333"'
  ```

- **`GET /acoreRoster`** returns the core raid as acraid's roster JSON
  ([acraid README](../../tools/database/acraid/README.md#from-the-sim-server)), read from the live
  server in one read-only transaction, one request at a time.
  - Flags, each falling back to its env variable; `--help` never shows a DSN:

    | Flag | Env | Required |
    |---|---|---|
    | `--acoreDsn` | `AC_DSN` | yes; no database name needed |
    | `--acoreDbcDir` | `AC_DBC_DIR` | yes |
    | `--acoreNamesFile` | `AC_RAID_NAMES_FILE` | yes; read on every request |
    | `--acoreLeader` | `AC_RAID_LEADER` | no: without it, list order |
    | `--acoreWorldBuffMatrix` | `AC_WORLD_BUFF_MATRIX` | no; read on every request |

  - Errors are `{"code", "error"}`: 400 `bad_request` (any query parameter), 403 `cross_site`,
    405 `method_not_allowed`, 409 `no_roster` (unknown name, empty names file), 500 `server_config`
    (unreadable file, bad DSN), 502 `database` (detail only in the log), 503 `not_configured` (names the
    missing flags), 504 `timeout` (30 s, queue wait included).
  - `AC_DSN` keeps the DSN off the command line. `--pprof`'s listener serves no `/debug/pprof/cmdline`,
    which would hand out `--acoreDsn`.
  - Item stats come from `wotlk/assets/database/db.json` in the files the server serves. It and the DBCs
    are cached by size and mtime, so a changed file needs no restart.
- **API** (`/raidSim`, `/computeStats`) takes binary protobuf. `/version` returns JSON.
  - Encode and decode with the image's protoc: `protoc -I/p --encode=proto.RaidSimRequest /p/api.proto`,
    with `proto/` mounted at `/p`.
  - A minimal player needs a class options message, `equipment {}` and `rotation { type: TypeAPL }`, or
    it panics.
  - The server binary is built without `with_db`, so requests must carry item data in `player.database`,
    as the UI does.
  - An exported settings JSON keeps the rotation as `TypeAuto` without the APL the page resolves, so it
    sims at the wrong DPS (Fire Mage 0). For page-exact inputs, capture the `computeStats` request the
    page posts to its worker, as `tools/statweights` does.
  - `/optimizeGearAsync` runs one optimization at a time (409 while busy, naming the running id) and
    cancels a run nobody has polled for 2 minutes. `/cancelAsync` cancels by progress id. The CLI
    equivalent: `wowsimcli optimize --infile <OptimizeGearRequest JSON>`. A request without item data,
    like `sim/optimizer/testdata/search/*.json`, fails with "isn't in the database" unless the CLI is
    built with `--tags=with_db` (`go run --tags=with_db ./cmd/wowsimcli optimize …`).

## Shell gotchas

- In Git Bash, prefix docker commands with `MSYS_NO_PATHCONV=1`, or you get "Cwd must be an absolute
  path". Use `$(pwd -W)` for mount paths.
- Bash heredocs fail intermittently, so write scripts to files.
- Run container commands that contain `$` from Git Bash: PowerShell quoting mangles them.
- `> $null` in Git Bash creates a file named `$null`.
- MSYS `grep` miscounts CR, with or without `-U`. Count with `od -c` or Python.
- `sed -i` turns a CRLF file into LF; `core.autocrlf` hides that from `git diff`.
- `tar` reads a `C:/...` path as a remote host: add `--force-local`.
- Python exact-string edits miss on CRLF files.
- Host Python can't see Git Bash's `/tmp`: use Windows paths.
- Under `pipefail`, `tr </dev/urandom | head` exits 141.
- `*.sh` must be LF, because CRLF breaks Git Bash. `tools/acore/.gitattributes` enforces it there.
