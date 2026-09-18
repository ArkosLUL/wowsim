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
- `tools/acore/dock.sh` (azerothcore-parity only) wraps all of this. It uses image `wowsim-toolchain`,
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
- Item DB: `make items` runs `gen_db -gen=db`, which builds `assets/database/` from the Classic sources in
  `assets/db_inputs/`. There's no AzerothCore mode yet ([ADR 0002](../adr/0002-item-data-from-live-db.md)).

## Run

- **Prod container** `wowsims-wotlk`, at http://localhost:3333/wotlk/:

  ```sh
  docker build --tag wowsims-wotlk .
  docker run -d --name wowsims-wotlk --restart unless-stopped -p 3333:3333 wowsims-wotlk
  ```

  - It's distroless: no shell, no mounts. It serves whatever was built into the image, so rebuild it to
    see changes.
  - Keep the `/wotlk/` path behind a reverse proxy.
- **Dev server** on :3334, since prod holds :3333. `--usefs` serves `./dist` from the mounted checkout.
  There's no hot reload: `docker rm -f wotlk-dev` and start it again after changes.

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

## Shell gotchas

- In Git Bash, prefix docker commands with `MSYS_NO_PATHCONV=1`, or you get "Cwd must be an absolute
  path". Use `$(pwd -W)` for mount paths.
- Bash heredocs fail intermittently, so write scripts to files.
- Run container commands that contain `$` from Git Bash: PowerShell quoting mangles them.
- `> $null` in Git Bash creates a file named `$null`.
- MSYS `grep` strips CR, so CR counts read 0. Use `grep -U` or `od -c`.
- Python exact-string edits miss on CRLF files.
- Host Python can't see Git Bash's `/tmp`: use Windows paths.
- Under `pipefail`, `tr </dev/urandom | head` exits 141.
- `*.sh` must be LF, because CRLF breaks Git Bash. `tools/acore/.gitattributes` (azerothcore-parity only)
  enforces it there.
