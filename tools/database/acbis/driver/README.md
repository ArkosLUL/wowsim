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

- A **fresh** `acraid` roster ([README](../../acraid/README.md)), kept outside the repo, exported in the
  same session as pass 1: gear, talents and glyphs drift daily, so a stale one builds lists for builds
  nobody has. Pin the core members with `-names`; `-leader` takes the live group, where a stand-in for
  one boss would replace a raider in the dataset.
- **Read the live party layout first**, every pass: `-names` fills subgroups of 5 in order, so an older
  roster's order silently invents a raid. Wrong parties mis-model the party-scoped buffs, Heroic Presence
  (its 1% hit moves hit-capped reforges and gems) and Mana Tide Totems, and make stage 2's Heroic Presence
  check switch raiders whose party already had a Draenei. Substitute the raider for any stand-in rather
  than copying the group verbatim.

  ```sh
  MSYS_NO_PATHCONV=1 docker exec -i ac-database mysql -uroot -ppassword -B acore_characters \
    -e "SELECT gm.subgroup+1 party, c.name FROM group_member gm JOIN characters c ON c.guid=gm.memberGuid ORDER BY gm.subgroup, c.name;"
  ```
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
- `--sources` ticks the tab's Item sources boxes, so the pool keeps those kinds only. A 25-man realm
  leaves out dungeon, heroic dungeon and both 10-man kinds:

  ```sh
  python tools/database/acbis/driver/driver.py run --roster "$SCRATCH/raid.json" --out "$SCRATCH/bis" \
    --stage 1 --sources raid25,raid25-heroic,vendor,crafted,quest,reputation,achievement,prospecting,world-drop
  ```

  Never excluded: the item a raider already wears in that slot, gems, enchants, and a source the
  catalog couldn't classify (a handful of TBC and holiday-event drops, no raid or dungeon loot). PvP
  gear never counts, nor does outclassed gear the raider doesn't wear
  ([sim-architecture](../../../../docs/guide/sim-architecture.md#ui)). Give both passes the same list: a settled job stays as it ran, so a rerun with
  another doesn't redo it, and the export's log line names what its entries really ran with, flagged
  when that isn't this pass's list.
- A rerun skips what's settled and resumes a cut-off job. **A failed job counts as settled**, so a
  rerun skips it too; `--retry-failed` runs this pass's failed jobs once more instead.
- One driver at a time per `--out` and `--port`: a Chrome already on the port with another profile stops
  the run, and two drivers on one profile would fight over the batch. `stop --out DIR` closes a Chrome
  a killed run left behind.
- At Quick, a stage 1 job takes 4–33 s on the server (a tank up to 94 s), a stage 2 job (BIS-stage2
  narrowed who gets one) 4–7 min, and the Heroic Presence checks about 16 s a party. A whole batch of 21
  raiders, 1 of them with stage 2, took 52 min, about half each pass
  ([INVESTIGATION](../../../../docs/bis-optimizer/bis-optimizer.INVESTIGATION.md#performance)).

## A build that changes by phase

The batch runs each raider in the roster's talents and glyphs, so a raider who switches builds between
phases (an Unholy DK: Morbidity dual-wield through phase 2, Scourge Strike 2H from phase 3) takes one
pass per build:

- The roster as exported for the first build's phases; a copy holding the other build for the rest, with
  its own `--out` and `--port`.
- A DK's Auto rotation follows the main hand he wears when the request is built (`autoRotation`,
  `ui/deathknight/sim.ts`), so the 2H copy wears a two-hander and no off hand.
- The page's item filters are raid-wide, in `settings.filters` of `__wotlk_raid____currentSettings__`,
  and the driver writes `storage-snapshot.json`'s keys back before every load. For a two-hander-only
  pool, open the page on that `--out` once, import the copy, set `oneHandedWeapons` false in that key,
  and save the snapshot before the pass.
- acbis takes one batch: replace the served batch's entries for the rerun raider-phases. The Heroic
  Presence checks run only in the page, so where a rerun pick changes who in a party is Draenei, redo
  that party's switch by hand.

## Unattended runs

- The driver reloads the page when the server goes away (giving it up to `--server-wait` s to come
  back), when the page stops answering (starting a new Chrome if it's gone), or after `--stall` s
  without progress. The reloaded page resumes its batch, starting over whatever job was in flight. Its
  orphaned run keeps the server busy until the server cancels it, 2 unpolled minutes later, and the
  batch retries every 15 s meanwhile.
- **Stage 2 needs a `--stall` longer than the job**: a raid-DPS job goes quiet near the end of its
  Alternatives step, so the 1800 s default reloads it just before it settles, forever. At Normal, Fel's
  phase 2 job froze three times at the same 1,214,750 sims, then settled in 4997 s under `--stall 10800`.
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
| `--sources` | `all` | `all`, or the kinds to keep: `dungeon`, `dungeon-heroic`, `raid10`, `raid10-heroic`, `raid25`, `raid25-heroic`, `vendor`, `crafted`, `quest`, `reputation`, `achievement`, `prospecting`, `world-drop` |
| `--retry-failed` | | run this pass's failed jobs once more |
| `--base` | `http://localhost:3346` | the sim server |
| `--container` | `wotlk-bisdata` | the sim server's container, left out of the load log |
| `--port` | `9346` | Chrome's debugging port |
| `--stall`, `--server-wait` | `1800` | seconds; stage 2 wants a longer `--stall` (see Unattended runs) |
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

## Checking a pass

`check_sources.py` audits an export against the server catalog: every ranked item must be one the server
hands out by the run's content phase, from a source kind the run kept, or it exits 1. It takes the roster
because gear a raider already wears is listed apart, not failed. It re-derives
`ui/core/optimizer/catalog.ts`'s rule instead of calling it, so a pool builder that stopped filtering
shows up here.

```sh
python tools/database/acbis/driver/check_sources.py "$SCRATCH/bis/batch-stage2.json" "$SCRATCH/raid.json"
```

`--catalog` overrides the checkout's.
