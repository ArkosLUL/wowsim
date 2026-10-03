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

Commands covered: `melee`, `taken`, `yellow`, `spell`, `armor`, `info`, `procs`; `damage` has its own test ([Damage probe](#damage-probe)).

`yellow` and `spell` follow the server's own rules:
- The hit table's attack type comes from the spell's `dmgClass` (ranged, else main hand). It must match the
  server's `derived.yellow.attackType`, except under `ALWAYS_HIT`, where the server never picks one.
- A ranged class spell that isn't a ranged weapon spell (Judgement of Wisdom) rolls with level*5 skill
  whatever's equipped (`Unit::MeleeSpellHitResult`). `rangedWeaponSpell` reads `SpellInfo::IsRangedWeaponSpell`
  off serverdata, minus the equipped item mask serverdata lacks; `TestRangedWeaponSpellMatchesTheSpelldump`
  checks it against the spelldump. An id missing from serverdata counts as a ranged weapon spell.
- Crit suppression is expected only from crit-capable spells (`critcap.go`: a damage or heal effect in
  `sim/core/serverdata`, no `CANT_CRIT`); a spell that can't crit has none. An id missing from serverdata
  is assumed capable and gets a row in the fixture README.
- Partial block is expected for every weapon class spell without `NO_ACTIVE_DEFENSE` or
  `ALWAYS_HIT`, from the front, whatever its school; only physical spells roll it.
- A positive spell at the dummies (faction 7, not hostile, so `Unit::SpellHitResult` never rolls it) has its
  miss threshold checked, but not a rolled miss rate.
- `SPELL_ATTR3_ALWAYS_HIT` and a binary spell's lack of partial resists are modeled.

`info` only rebuilds a `naked()` player: no weapon, no rating beyond defense, dodge, parry and armor
penetration, and no timed aura besides the base stats test's rating auras and raid buffs. A passive aura
added by hand, such as a talent, isn't detected.

`melee` of a naked player, bare-handed or with only a main-hand weapon, also compares `meanDamage` per outcome
(normal, crit, glancing, block) with the sim's attack power, armor after the attacker's armor penetration
(rating plus auras such as Battle Stance's 10%, as in `armor`), glancing, crit and block value, over the
server's integer rolls (the range truncates, armor rounds up), within |z| ≤ 5. The record carries the weapon's
range and speed; an armed swing whose attack power differs from the bare sim character's (a weapon with stats)
is skipped. A weapon enchant's flat damage isn't in the record. `taken` isn't compared.

`TestFixtureHasEveryE2ERecord` fails when the fixture lacks a record a full e2e run produces, unless the record
is dropped with a row in the fixture README.

`procs` checks the generated `spell_proc` rows field for field against the live entry, then checks
`core.ServerProcFor` and the PPM basis against the chance the server computed per attack type. Auras
outside `sim/core/serverdata` are skipped — the tables only cover the spells the sim names, so the
committed replay fixture exercises none of them. `testdata/procs.jsonl` is a separate capture that
does: Judgement of Wisdom (15 PPM), Hand of Justice (1 PPM with `REDUCE_PROC_60`), probed for
Frostbolt, Heroic Strike and Steady Shot, which pick a different PPM basis each. Recapture it with
`e2e/run.sh TestSimvalProcPPM` while streaming `simval.jsonl` out of the worldserver.

## Damage probe

`TestDamageRecords` replays `sim/core/testdata/simval/damage.jsonl`, a capture of mod-sim-validation's
`TestSimvalDamage` (`.simval damage`), on every `go test`; it skips without the file.
`TestDamageFixtureHasEveryProbe` fails when a probe's bare or talented record is missing and not in
`droppedDamageRecords`. `CheckRecord` ignores `damage` records, so a `simval.jsonl` recapture holding them still
passes; move them to `damage.jsonl`. Recapture it like `simval.jsonl`
([testing](../../docs/guide/testing.md#against-the-server)), running only `run.sh TestSimvalDamage` and keeping
its `damage` lines.

Per unshaped record (`damage.go`):
- The character is the record's class and race. Talent rank and glyph auras become talents and glyphs (from
  `ui/core/talents/trees` and `db.json`); other auras the sim registers by id (a presence, a stance) are put on.
  The test's power auras are skipped: the record's `spellPower` and `attackPower` are set as the sim's stats,
  attack power first (Sheath of Light adds spell power off it).
- The spec is the class's `spec`, else an `otherSpecs` entry that registers the spell (`basestats.go`), retried
  with the teaching talent for `talentTaughtSpells` (Black Arrow).
- The target takes the dummy's level, creature type and armor.
- Each probed effect matches the sim's `Direct` or dot `Tick` by effect index and spell id (its own or
  `FromSpellID`). The sim's own code runs with that base pinned to the record's: `ApplyEffects` with a forced
  hit, no crit and no resists, read at calculation through the target's damage taken hook; a dot is applied,
  then one snapshot tick computed. A `probedBeforeOwnAura` dot (Serpent Sting) is cancelled before its tick: the
  server works it out before the dot's own aura, and with it Noxious Stings, is on the target.
- Checks: declared base (skipped when it comes from another spell, e.g. Death Coil's), SP coefficient (±5e-4),
  `taken` at both roll ends, and the crit multiplier where both sides crit (server: a crit-capable spell, or a
  tick unless `CANT_CRIT`; sim: dots only with ticks that crit). The sim may sit 0.1% or 1 point below the
  server, and above it max(0.1%, 1 + bonuses × multiplier): the server truncates each SP or AP bonus before the
  multipliers. A failing amount lists sim-only auras and server auras the sim lacks.

Pets and non-80 characters fail. Shaped records give no checks.

## Timing probe

`TestTimingRecords` replays `sim/core/testdata/simval/timing.jsonl`, a capture of mod-sim-validation's
`TestSimvalTiming` (`.simval timing`, plus each bot's two `info` records for regen), on every `go test`; it skips
without the file. `TestTimingFixtureHasEveryProbe` fails when a probe's bare, talent or Bloodlust record is missing
and not in `droppedTimingRecords`. `CheckRecord` ignores `timing` records, so a `simval.jsonl` recapture holding
them still passes; move them (and the timing bots' `info` lines) to `timing.jsonl`, running only
`run.sh TestSimvalTiming` ([testing](../../docs/guide/testing.md#against-the-server)).

Per record (`timing.go`):
- The character is `damage.go`'s (class, race, talent and glyph auras), plus Bloodlust registered when the record
  has it: its aura carries a tag, so auras are matched by spell id alone. The spec is found as for `damage`, then
  `timingExtraSpecs`, with `timingTaughtSpells` (Mortal Strike, Aimed Shot, Mind Flay...) teaching the spell.
- Exact, in whole ms: cast time before haste (`DefaultCast.CastTime`) and after (`Spell.CastTime`), GCD (the spell's
  effective time with the cast time zeroed), own cooldown (`CD.Duration`) and category cooldown (`SharedCD`),
  `channelMs` and aura duration, amplitude and tick count of the sim's dot or hot (re-run for the haste on the
  character), cost (`ApplyCostModifiers`; rage and runic power in tenths, truncated), rune costs and runic power gain
  (x10; the server lists runes as blood, unholy, frost). Cast speed and ranged speed within 1e-4.
- Not compared, passed with the reason: a GCD outside category 133 (the sim has no GCD there), a cooldown the server
  starts on an event, a category cooldown the sim has no shared timer for, a cast made instant, an aura the sim
  has no dot or aura for, an immune or skipped aura.
- Regen, within 0.1%: mana per second with and without the five second rule, energy per second, and each rune's
  cooldown from `baseCooldownMs` (a timing record) and from `1000 / PLAYER_RUNE_REGEN` (an `info` record). Only
  these bots carry the regen fields.

Pets and non-80 characters fail. A spell no spec registers fails as `sim character`.

`knownTimingMismatches` (class, spell, check) lets a real sim/server difference through, each with a row in the
fixture README; the check still runs and logs.

## Golden characters

`sim/core/testdata/simval/goldchar_setups.json` lists each non-healer golden suite's default player (suites
sharing one share an entry), which mod-sim-validation's `TestSimvalGoldenCharacters` builds on the server
(`goldchar.go`). Per setup:
- From the suites: spec field, server class and race, gear set and its items by server equipment slot (no gems or
  enchants), talent string and its rank spells in learn order (tree file order, a prerequisite first: Dual Wield
  sits after Dual Wield Specialization), glyphs with server slot (majors 0, 3, 5; minors 1, 2, 4), sim item and
  aura spell (`db.json`).
- The list's own: each glyph's GlyphProperties id (from the live GlyphProperties.dbc; 0 for a sim glyph the server
  lacks, allowlisted in `serverlessGlyphs`), `auras`, `imbue` and `probes` (spells for `.simval damage`, with the
  `shaped` reason the server gave).
- `auras`: what the sim starts the spec with, cast on the server before recording: stance, form, presence, seal,
  aspect, Bone Shield, Track Giants for hunters (the sim always counts Improved Tracking; the dummy is a giant),
  and the raid buffs the class gives itself (`AddRaidBuffs`: Horn of Winter, Mark of the Wild, Arcane Intellect, the
  priest buffs, Trueshot Aura), which the sim adds as stats. DKs list Blood Presence, which the server character
  always has and the sim only gets from its rotation (`serverStartingAuras`).
- `imbue`: the temporary enchant the sim always puts on the main hand (Elemental's Flametongue Weapon, 58790).

Edit the list by hand. Four tests keep it honest:
- `TestGoldcharSetupsFollowTheSuites` parses every `CharacterSuiteConfig` in a `sim/**` Test function with
  `go/ast` and fails, printing the expected entry, when the list drifts from the suites.
- `TestGoldcharAurasCoverTheSimsStart` fails on an aura the sim has up after a reset, or a raid buff the class gives
  itself, that isn't in `auras`; passives (serverdata), glyphs and `unlistedStartingAuras` (Thorns) are exempt. It
  also holds `imbue` to the sim's main hand enchant (`imbueSpells`).
- `TestGoldcharTalentsLearnPrerequisitesFirst` checks the learn order on every tree file.
- `TestTalentTreeRankSpellsAreTheServers` holds every tree file's rank spells to the spelldump's ranks.

`TestGoldenCharacters` replays `sim/core/testdata/simval/goldchar.jsonl`, a capture of that e2e (skips without it;
needs `-tags=with_db`). Records group by bot name; the info record's `character` (class, race, talents, glyphs,
items) picks exactly one setup, and the bot's `damage` records belong to it. The sim character is `goldchar.go`'s
`goldcharPlayer`: the setup's spec, race, items, talent string and server-known glyphs, empty buffs, consumes and
spec options, plus `auras` it registers by id. A hunter whose record carries Sniper Training's buff (64420, a bot
that stood still) also gets `SniperTrainingUptime` 1 and its aura 53304. Per setup:
- `info`, against the sim's stats after a reset:
  - spell power, spell crit: the lowest magic school (school bonuses only add); spell power may sit up to 0.1% or 1
    below and one more above (truncated stat bonuses), crit within the dropped stat fraction.
  - spell hit: the sheet plus the school hit (`SPELL_AURA_MOD_INCREASES_SPELL_PCT_TO_HIT`, read from
    `assets/db_inputs/acore/spelldump.jsonl`) every magic school gets; the sheet leaves it out.
  - per hand the sim swings (main hand, dual-wield off hand, hunter ranged): unhasted and hasted ms (server
    truncates), the item's weapon damage (also in a feral form, which swings the form's), crit, hit (main hand and
    ranged) and expertise, from the auto attack spell's bonuses.
- `damage`: the record's `shaped` must contain the probe's; unshaped records replay like `TestDamageRecords`
  (`replayDamage`) on this character. A stacking dot is worked out for one stack.

A failing check lists the sim-only auras. Drops: `droppedGoldchar` (setup, spell, check), each with a fixture README
row. Spell 0 is the info record; an empty check drops the whole record (removed from the capture), a named one is
only logged. A stale drop fails.

A full `simval.jsonl` recapture can hold these records: `info` gets only `CheckRecord`'s armor penetration check
(`TestGoldcharInfoRecordInTheReplayFixture`), `damage` none.

## Talent sweep

`sim/core/testdata/simval/talent_paths.json` holds the paths mod-sim-validation's `TestSimvalTalentSweep` walks
(`talentpaths.go`): per path a server class and race, the sim `spec`, `items` by server slot, `auras` (stance, form,
presence, aspect, the class's own raid buffs; recast after every step) and `steps` (`talent` fieldName, `rank`,
rank `spell`, `filler` for a talent the sim doesn't implement, step `auras` the sim puts up once it's in: Moonkin
Form, Shadowform, Bone Shield, Trueshot Aura). The generator, per class:
- Setups are hand-listed in `sweepClasses`: one per tree (two feral: Cat, Bear), the base stats test's race, the golden
  characters' weapons where talents need them. A talent shows its sheet change only in setups whose items fit its
  rank spell's equipped item requirement when that spell has a sheet aura (spelldump), and in the setups `needs`
  names (stance, form, shield, dual wield, a melee weapon for parry). `after` puts Furor behind Moonkin Form; `last` puts Dark Pact at the end,
  where the capture has it.
- Each setup gets a path over the uncovered implemented talents at max rank: its own tree first, trimmed to 71
  points from the lowest row up, then other trees' from the lowest row while points last. Then, while any are left,
  the path covering the most. Rows open at 5 points per row in the tree; the gaps take fillers, unimplemented
  talents first; a prerequisite goes first at max rank.

Rewrite it with `tools/acore/dock.sh test ./tools/simval -run TestTalentPathsFile -update`; a changed path's
`talents.jsonl` records then match nothing, so rerun it (`SIMVAL_TALENT_ONLY`) and trim its stream: paths it has no
bot for keep their records from the fixture. Tests:
`TestTalentPathsFile` (stale), `TestTalentPathsAreLegal` (points, rows, prerequisites, ranks, every implemented
talent covered in a setup that shows it, on the committed file), `TestTalentPathsCoverTheSimsStart` (after each
step, what the sim has up that isn't passive is in the auras), `TestTalentPathsItemClasses`.

`checkInfo` skips a naked record carrying a talent rank's aura, so a `simval.jsonl` recapture with the sweep's records
stays green; a sweep character before its first talent gets the base stats checks
(`TestTalentSweepInfoRecordInTheReplayFixture`).

**Trim** (`talents.go`): `tools/acore/dock.sh run ./tools/simval talents -records /wotlk/tmp/<raw>.jsonl` reads a
`simval.jsonl` streamed during the run (required: the e2e deletes its bots' records from the worldserver's file) or
an e2e log's `RECORD` lines, keeps the `Svtlead…` bots' info records and writes
`sim/core/testdata/simval/talents.jsonl` (`-out` to change): per record its `path`, `step` (talents learned), bot,
character and the compared sheet fields. Bots go by name; a bot's records must all fit one path (class, race, items,
talents a start of its steps). A bot fitting none or several is left out with a note, a path's later bot replaces an
earlier one (a rerun), the old fixture's paths the stream lacks are kept, a step recorded twice or a record without
`maxPower` fails. It prints the steps still missing.

**Replay** (`TestTalentSweepRecords`, skips without the fixture): per step, the server's change between the records
before and after it against the sim's change between `path.goldcharSetup(n-1)` and `(n)` (stats after a reset, with
the path's and steps' auras). Fields: primary stats, max health, max mana/rage/energy/runic power (the server's power
type), armor, fire to arcane resistance, AP, RAP, spell power and spell crit (lowest magic school, and each school's: the
stat plus the lowest bonus among the sim's crit-capable spells of only that school, skipped without one), armor
penetration % from auras and the chance to be missed per attack (both skipped in a record without them), spell hit (plus
every school's school hit, as in Golden characters), crit, hit and expertise per hand the sim swings (main hand crit
and melee hit always), melee, ranged and spell haste (server: 1 / speed multiplier), dodge and parry after diminishing
returns (parry with a weapon), block chance and value (with a shield).

Tolerance: the server truncates primary stats, health, power, armor, resistances, block value, each stat or AP based
spell power bonus, a rating from a stat, and AP's base and mods before its multiplier; the sim doesn't. Each sim sheet
gets a bound per field: its own truncation (1; spell power 3; a resistance 2; parry one parry rating) plus the sim's
change per truncated unit of each source (+1 of a primary stat, per point of the stat's sheet value; +1 raw AP or
RAP, times 3). A step passes a field when the two changes differ by at most the larger bound of its two sheets plus
1e-3.

Each step is logged on a `SWEEPSIM` line as `agrees`, `moves no field` (nothing past 1e-3 on either side: a proc or
spell mod talent, or one needing gear or a buff the path lacks: item armor, Devotion Aura) or `disagrees`; the end
lists the no-field steps. A sim that can't build the character fails as `sim character`. Drops: `droppedSweep` (path,
talent, field), each with a fixture README row; an empty field drops the step. Records stay, since the next step
starts from them; a dropped step or field is only logged, and one that agrees or matches nothing fails. Hand-built
tests: `TestTalentSweepAgreesWithTheSim` and `TestTalentSweepReplaysAFullRun` (every path, server sheets made from the
sim, through the trim), `TestTalentSweepCatchesMismatches`, `TestTalentTrim`.

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
| `-refreshAura` | a dot/buff spell id: measure the delay from each `-refreshFeeders` crit to its refresh (`core.DelayedPeriodicApplier`'s landing) |
| `-refreshFeeders` | comma-separated spell ids whose crits (direct hits or ticks) can feed `-refreshAura` |
| `-refreshWindow` | 500 ms; a crit further back than this can't be the one that fed a refresh |

Interval histograms bucket at 100 ms, the server's map update. They show the lattice only roughly:
Chronicle timestamps the packet send, which adds a few ms of jitter.

`-refreshAura`/`-refreshFeeders` pairs each of that spell's `SPELL_AURA_APPLIED` events with the most recent
still-unclaimed listed crit within `-refreshWindow`, oldest crit first: a crit's own refresh doesn't always land
in crit order (each draws its own 0-400 ms delay), so a later crit can't simply displace an earlier one still
waiting to fire. Prints each feeder's count, mean and median delay and a 50 ms histogram, then the same pooled
across feeders; with `-v`, also every individual pair (feeder id, crit damage, crit ms, refresh ms, delay ms), for
replaying a formula like `procIgnite`'s against the actual tick damage by hand. An application chronicle can't
pair, or a crit no application claims, means the feeder list or window is incomplete, not that the aura applied
with no cause: the proc this measures (Ignite, Righteous Vengeance, Deep Wounds, ...) is unconditional on a
qualifying crit.

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
from the dummy's front as the run does. A Protection Paladin (the setup's `spec label` is `prot`) runs
`ui/protection_paladin/apls/default.apl.json`'s priority list the same way: Shield of Righteousness or
Hammer of the Righteous (whichever's cooldown favors the other), Holy Shield, Consecration, then Judgement;
Hammer of Wrath is left out; the dummy's health never drops far enough to make it castable either side. An
Affliction Warlock (same env vars, `SIMVAL_RECORD_CLASS=warlock`) runs a priority list keeping Corruption,
Curse of Agony, Unstable Affliction and Haunt up, Life Tap under 60% mana, else Shadow Bolt; no pet. All
three spend their own mana. It prints the DPS, pet included, to pass to `chronicle -sim`. Needs
`--tags=with_db` and the live DBCs. Other classes have no rotation here.

Every fixed-cycle capture (Ret, Protection, Affliction) has no APL equivalent for its 1.5 s round-robin (a
refused spell waits for its next turn), so the priority list casts more often and DPS compares rotations,
not formulas. Compare per ability instead: `-v` here and on `chronicle` prints each ability's non-crit,
crit and glance averages and its crit rate over landed hits, each ± one standard error.

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
