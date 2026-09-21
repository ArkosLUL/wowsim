# Dev environment

Host, toolchain container, protos, builds, running the sim. For verification, see [testing.md](testing.md).

## Host

- Windows 11, with Git Bash and PowerShell. `core.autocrlf=true`, so working-tree files are CRLF.
- Missing: Go, make, the mysql CLI, `bc`.
- Present: Python 3.14, Docker and Node. `node_modules` was installed from Linux, so npx, tsc and eslint
  only work in the container.
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
  docker run -d --name wowsims-wotlk --restart unless-stopped -p 3333:3333 wowsims-wotlk
  ```

  - It's distroless: no shell, no mounts. It serves whatever was built into the image, so rebuild it to
    see changes.
  - Keep the `/wotlk/` path behind a reverse proxy.
  - Without `SIM_COMMIT`, optimizer results say sim "unknown": `.dockerignore` drops `.git`.
- **Dev server** on :3334, since prod holds :3333. `--usefs` serves `./dist` from the mounted checkout.
  There's no hot reload: `docker rm -f wotlk-dev` and start it again after changes. With a worktree
  mounted, add `-e SIM_COMMIT=$(git -C <worktree> rev-parse HEAD)`: its `.git` points outside the mount,
  so the container can't read the commit.

  ```sh
  MSYS_NO_PATHCONV=1 docker run -d --name wotlk-dev -p 3334:3333 -v <worktree>:/wotlk \
    -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build -w /wotlk wowsims-wotlk-dev \
    sh -c 'make dist/wotlk/.dirstamp devserver && ./wowsimwotlk --usefs=true --launch=false --host=":3333"'
  ```

- **API** (`/raidSim`, `/computeStats`, `/version`) takes binary protobuf.
  - Encode and decode with the image's protoc: `protoc -I/p --encode=proto.RaidSimRequest /p/api.proto`,
    with `proto/` mounted at `/p`.
  - A minimal player needs a class options message, `equipment {}` and `rotation { type: TypeAPL }`, or
    it panics.
  - The server binary is built without `with_db`, so requests must carry item data in `player.database`,
    as the UI does.
  - `/optimizeGearAsync` runs one optimization at a time (409 while busy, naming the running id) and
    cancels a run nobody has polled for 2 minutes. `/cancelAsync` cancels by progress id. The CLI
    equivalent: `wowsimcli optimize --infile <OptimizeGearRequest JSON>`.

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
