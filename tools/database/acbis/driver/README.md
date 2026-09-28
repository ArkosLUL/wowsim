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
both stages ([README](../README.md)): every raider keeps their stage 1 pick unless stage 2 found a
better one for them or a Heroic Presence check switched their racial traits.

- Stage 1 runs every chosen raider. Stage 2 runs only the chosen raiders whose own gear changes
  someone else's damage (Demonic Pact, a targeted Focus Magic), against everyone's stage 1 pick, and
  skips a phase whose stage 1 hasn't settled for every chosen raider. The grid marks who that is with
  "(stage 2)" next to their spec.
- Stage 2 then runs the page's Heroic Presence checks, one Draenei per party
  ([PLAN](../../../../docs/bis-optimizer/bis-optimizer.PLAN.md#bis-stage2b-as-built)). So `--stage 2` runs
  even with no "(stage 2)" raider among `--raiders`, and a phase is done once the page has nothing left
  to start.
- Both passes need the same `--out`, whose Chrome profile holds the batch, and the same roster file:
  the batch is keyed on the roster's specs, races, talents, glyphs and professions, so a changed
  roster starts an empty one.
- A rerun skips what's settled and resumes a cut-off job. **A failed job counts as settled**, so a
  rerun skips it too; `--retry-failed` runs this pass's failed jobs once more instead.
- One driver at a time per `--out` and `--port`: a Chrome already on the port with another profile stops
  the run, and two drivers on one profile would fight over the batch. `stop --out DIR` closes a Chrome
  a killed run left behind.
- At Quick, a stage 1 job takes 4–33 s on the server (a tank up to 94 s), a stage 2 job (BIS-stage2
  narrowed who gets one) 4–7 min, and the Heroic Presence checks about 16 s a party. A whole batch of 21
  raiders, 1 of them with stage 2, took 52 min, about half each pass
  ([INVESTIGATION](../../../../docs/bis-optimizer/bis-optimizer.INVESTIGATION.md#performance)).

## Unattended runs

- The driver reloads the page when the server goes away (giving it up to `--server-wait` s to come
  back), when the page stops answering (starting a new Chrome if it's gone), or after `--stall` s
  without progress. The reloaded page resumes its batch. Its orphaned server run keeps the server busy
  until the server cancels it, 2 unpolled minutes later, and the batch retries every 15 s meanwhile.
- **It gives up** when the server stays away, when a reload can't get the page back in 5 tries 30 s
  apart, when the batch stops with runs left 4 times, or when 3 reloads in a row bring no progress. The
  last two counts reset whenever a job settles.
- Chrome writes localStorage to disk lazily, so a killed Chrome loses the last jobs, or the roster and
  batch outright. After each job, Heroic Presence check and pass, the driver saves the raid sim's keys
  to `storage-snapshot.json` and puts back what a new Chrome lacks, re-importing the roster if the grid
  is still empty.
- localStorage holds 5.24M chars per origin, and a whole two-stage batch of 21 raiders came to 43% of that.
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
  stage 1 entries only, each in the racial traits its own run chose, so it stays own metrics even once
  the batch holds stage 2 runs and Heroic Presence switches.
- `driver.log`: everything the driver prints.
- `timings.jsonl`: a line per job with its state, server seconds, wall seconds since the previous job
  settled (request building and busy waits included), sims, error, and machine load (CPU % and other
  toolchain containers); and a line per Heroic Presence check with its party, kind, who switched,
  seconds and each candidate's raid DPS gain.
- `chrome-profile/` and `storage-snapshot.json`: the batch. Keep both until both passes are done.
