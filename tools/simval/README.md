# simval

Compares this sim with the live AzerothCore server: the `.simval` recordings
[mod-sim-validation](../../docs/guide/azerothcore-server.md) writes, and the raw combat logs
mod-chronicle writes for a real fight.

```bash
tools/acore/dock.sh run ./tools/simval                             # replay the live recording
tools/acore/dock.sh run ./tools/simval -records /wotlk/sim/core/testdata/simval/simval.jsonl
tools/acore/dock.sh run ./tools/simval chronicle -v -sim 8123 <log>
DBC_DIR=<live DBC copy> tools/acore/dock.sh run --tags=with_db ./tools/simval rrsim -seconds 300.8 <setup.txt>
```

## Replay (no subcommand)

Rebuilds each record's combat table from the two unit snapshots and compares it with the one the
server derived, so a mismatch points at the sim's formula rather than at noise. Tolerances: ±1 bp on a
computed chance, |z| ≤ 5 on a rolled rate, ±0.1% on a multiplier. It exits non-zero on any failure.

| Flag | Default |
|---|---|
| `-records` | `/ac/env/dist/logs/simval/simval.jsonl`, the live worldserver's |
| `-fixture` | also copy the records to `sim/core/testdata/simval/` |
| `-v` | print passing checks too |

Commands covered: `melee`, `taken`, `yellow`, `spell`, `armor`, `info`, `procs`.

`spell` doesn't model `SPELL_ATTR3_ALWAYS_HIT`, a positive spell cast at the dummies (their faction 7
isn't hostile, and `Unit::SpellHitResult` lets a positive spell miss only a hostile target), or a binary
spell's lack of partial resists, so records of those fail it.

`procs` checks the generated `spell_proc` rows field for field against the live entry, then checks
`core.ServerProcFor` and the PPM basis against the chance the server computed per attack type. Auras
outside `sim/core/serverdata` are skipped — the tables only cover the spells the sim names, so the
committed replay fixture exercises none of them. `testdata/procs.jsonl` is a separate capture that
does: Judgement of Wisdom (15 PPM), Hand of Justice (1 PPM with `REDUCE_PROC_60`), probed for
Frostbolt, Heroic Strike and Steady Shot, which pick a different PPM basis each. Recapture it with
`e2e/run.sh TestSimvalProcPPM` while streaming `simval.jsonl` out of the worldserver.

## chronicle

Reads a mod-chronicle raw log (`<unix_ms>  EVENT,field,…`, gzipped or not) and reports one player's
run: DPS, ability breakdown, swing and tick intervals, and aura uptimes. Damage adds up across
targets; tick timelines and aura windows are per target, since a DoT on two mobs ticks on two of
them. Damage by the pets and guardians the player owns counts for the player. With `-sim` it checks
the DPS gap against the plan's ±2% and exits non-zero outside it.

| Flag | Default |
|---|---|
| `-source` | the log's only damaging player; needed when there are several |
| `-target` | count only damage to this unit |
| `-sim` | the sim's DPS for the same gear, talents and rotation; 0 skips the check |
| `-gap` | 5 s; a longer pause is left out of the active duration |
| `-top` | 20 ability rows |
| `-v` | also print per-outcome averages (see [rrsim](#rrsim)), swing intervals, tick intervals and aura uptimes |

Interval histograms bucket at 100 ms, the server's map update. They show the lattice only roughly:
Chronicle timestamps the packet send, which adds a few ms of jitter.

Chronicle gaps to keep in mind:
- `SWING_DAMAGE` carries no main/off hand flag, so a dual wielder's two hands interleave in one swing timeline.
- An aura refresh re-emits `SPELL_AURA_APPLIED`, so applications count refreshes, and there are no stack counts.
- A spell's dodge or parry comes only as `CHRONICLE_SPELL_TARGET_RESULT`: `SPELL_MISSED` covers only immunity
  and damage shields. `chronicle` counts both.
- A persistent area aura's missed tick isn't logged, only a longer gap between its ticks (Consecration).

## rrsim

Sims a recorded run from its `.setup.txt`: gear, talents and glyphs as recorded, plus ammo, quiver and
pet for a hunter (`TestRecordedRunHunter`'s rotation). A Retribution Paladin (`TestRecordedRun`,
`SIMVAL_RECORD_TALENT_SPELLS` set) runs `ui/retribution_paladin/apls/default.apl.json`'s priority list
cut to what the recorded cycle casts (Divine Plea, Judgement of Wisdom, Crusader Strike, Divine Storm,
Consecration; no Avenging Wrath), with the aura its `started:` spells name (none by default), swinging
from the dummy's front as the run does. An Affliction Warlock (same env vars, `SIMVAL_RECORD_CLASS=warlock`)
runs a priority list keeping Corruption, Curse of Agony, Unstable Affliction and Haunt up, Life Tap under
60% mana, else Shadow Bolt; no pet. Both spend their own mana. It prints the DPS, pet included, to pass to
`chronicle -sim`. Needs `--tags=with_db` and the live DBCs. Other classes have no rotation here.

Both fixed-cycle captures (Ret, Affliction) have no APL equivalent for their 1.5 s round-robin (a refused
spell waits for its next turn), so the priority list casts more often and DPS compares rotations, not
formulas. Compare per ability instead: `-v` here and on `chronicle` prints each ability's non-crit, crit
and glance averages and its crit rate over landed hits, each ± one standard error.

| Flag | Default |
|---|---|
| `-seconds` | the setup's; pass the active duration `chronicle` reports |
| `-notracking` | off; drops Improved Tracking for a run that tracked nothing, like the two SV captures |
| `-iterations` | 3000 |
| `-dbc` | `/dbc`, the live DBC copy |
| `-out` | also write the sim request here as JSON |
| `-v` | also print final stats, pet talents, glyphs, an ability breakdown and per-outcome averages |

## Captures

`sim/core/testdata/chronicle/` holds recorded runs as `<spec>_<player>_<unix>.log.gz`, each with a
`.setup.txt` naming the gear, talents, glyphs and rotation it was fought with, which the sim needs to be
comparable, and the server's pre-fight `.simval info` snapshot to check the sim's stats against. The oldest
captures lack glyphs, and some the snapshot. `TestChronicleCaptures` parses every one of them, so a capture
whose format drifted fails there. Record new ones with mod-sim-validation's `TestRecordedRun`, or for a hunter
`TestRecordedRunHunter` (`SIMVAL_RECORD_HUNTER=<label>`); its README has the run line.
