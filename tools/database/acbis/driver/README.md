# acbis driver

Runs the raid sim's BiS Batch tab in headless Chrome and exports it for `acbis -batch`. It drives the
page with `tools/uicheck/cdp.py` and `raid.js`; `page.js` adds download capture and the batch's state.

## Before running

- **A sim server of its own**, from the toolchain image over this checkout: not prod's 3333, not
  `wotlk-dev`'s 3334. Pass `SIM_COMMIT`, or results say sim `unknown`:

  ```sh
  MSYS_NO_PATHCONV=1 docker run -d --name wotlk-bisdata -p 3346:3333 -e SIM_COMMIT=$(git rev-parse HEAD) \
    -v G:/DevStuff/GitHub/wowsimwotlk:/wotlk -v wotlk-gomod:/go/pkg/mod -v wotlk-gocache:/root/.cache/go-build \
    -w /wotlk wowsims-wotlk-dev \
    sh -c 'make dist/wotlk/.dirstamp devserver && ./wowsimwotlk --usefs=true --launch=false --host=":3333"'
  ```

  **Keep `master` still while a pass runs.** Restarting the server rebuilds it from whatever the
  checkout holds then, but its results keep the `SIM_COMMIT` the container was created with, so one
  batch would mix sims under a single stamp. The driver logs the move when it notices one.

- An `acraid` roster ([README](../../acraid/README.md)), kept outside the repo.
- Host Python with `websockets`, Chrome at its default install path, and PowerShell — the driver asks it
  which process holds the debugging port, ends a Chrome that stopped answering, and reads the CPU load.

## Running

From Git Bash, with `SCRATCH` outside the repo:

```sh
python tools/database/acbis/driver/driver.py run --roster "$SCRATCH/raid.json" --out "$SCRATCH/bis" --stage 1
python tools/database/acbis/driver/driver.py run --roster "$SCRATCH/raid.json" --out "$SCRATCH/bis" --stage 2
```

Give acbis `-batch "$SCRATCH/bis/batch-stage1.json"` for own metrics only, or `batch-stage2.json` for
both stages ([README](../README.md)).

- Stage 1 runs every chosen raider. Stage 2 runs the chosen DPS raiders against everyone's stage 1 pick,
  and skips a phase whose stage 1 hasn't settled for every chosen raider.
- Both passes need the same `--out`, whose Chrome profile holds the batch, and the same roster file:
  the batch is keyed on the roster's specs, races, talents, glyphs and professions, so a changed
  roster starts an empty one.
- A rerun skips what's settled and resumes a cut-off job. **A failed job counts as settled**, so a
  rerun skips it too; `--retry-failed` runs this pass's failed jobs once more instead.
- One driver at a time per `--out` and `--port`: a Chrome already on the port with another profile stops
  the run, and two drivers on one profile would fight over the batch. `stop --out DIR` closes a Chrome
  a killed run left behind.
- At Quick, a stage 1 job takes 7–54 s on the server (the slowest all with another container up) and a
  stage 2 job about 9 min.

## Unattended runs

- The driver reloads the page when the server goes away (giving it up to `--server-wait` s to come
  back), when the page stops answering (starting a new Chrome if it's gone), or after `--stall` s
  without progress. The reloaded page resumes its batch. Its orphaned server run keeps the server busy
  until the server cancels it, 2 unpolled minutes later, and the batch retries every 15 s meanwhile.
- **It gives up** when the server stays away, when a reload can't get the page back in 5 tries 30 s
  apart, when the batch stops with runs left 4 times, or when 3 reloads in a row bring no progress. The
  last two counts reset whenever a job settles.
- Chrome writes localStorage to disk lazily, so a killed Chrome loses the last jobs, or the roster and
  batch outright. After each job the driver saves the raid sim's keys to `storage-snapshot.json` and
  puts back what a new Chrome lacks, re-importing the roster if the grid is still empty.
- localStorage holds 5.24M chars per origin, and a full two-stage roster comes to about 85% of that.
  Each job's log line has the share. Once the page can't store the batch, the exports and the snapshot
  still carry everything, but a reload re-runs every job since the last store.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--out` | required | the output directory below, for both commands |
| `--roster`, `--stage` | required for `run` | |
| `--raiders` | `all` | comma-separated names from the grid (healers sit out) |
| `--phases` | `1-5` | e.g. `3` or `2,4` |
| `--effort` | `Quick` | `Quick`, `Normal` or `Thorough`. A job already done stays as it ran |
| `--retry-failed` | | run this pass's failed jobs once more |
| `--base` | `http://localhost:3346` | the sim server |
| `--container` | `wotlk-bisdata` | the sim server's container, left out of the load log |
| `--port` | `9346` | Chrome's debugging port |
| `--stall`, `--server-wait` | `1800` | seconds |
| `--poll` | `5` | seconds between checks |
| `--keep-chrome` | | leave Chrome running |

## Output

In `--out`:
- `batch-stage<N>.json`: the batch export, rewritten after each phase of pass N. Pass 1's file keeps
  stage 1 entries only, even once the batch holds stage 2 runs, so it stays own metrics.
- `driver.log`: everything the driver prints.
- `timings.jsonl`: a line per job with its state, server seconds, wall seconds since the previous job
  settled (request building and busy waits included), sims, error, and machine load (CPU % and other
  toolchain containers).
- `chrome-profile/` and `storage-snapshot.json`: the batch. Keep both until both passes are done.
