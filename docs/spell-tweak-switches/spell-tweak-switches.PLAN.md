# Spell-tweak switches: test two findings on the live server, then file the fixes as wave-loop items

Paths: [sim] `G:\DevStuff\GitHub\wowsimwotlk` (master), [int] `G:\DevStuff\GitHub\wowsimwotlk-int`
(integration, the wave loop's, wave Z5 running), [ac] `G:\DevStuff\GitHub\azerothcore-wotlk-pb`, [mod]
`[ac]\modules\mod-sim-validation` (its own repo, shared with the running wave).

## Context

The user toggled "Feral Spirit haste" in the Enhancement sim, saw no DPS change, and asked for the other
server-setting checkboxes to be checked too. A source audit (sim `integration`, mod-spell-tweaks `338d826`,
[ac] `3d89793c7`, what the live server runs; nothing run) found the wiring fine and these mismatches.

The user's calls so far:
- Fixes go through the wave loop as parity work items, no sim code from this session.
- Before anything is decided on Glyph of Backstab, test the behaviour in game, and this session runs that
  test now with the bot harness, from a private copy, without disturbing wave Z5.

**Wrong with the switch on, which is the live state:**

| Switch | Sim | Server, from source |
|---|---|---|
| Feral Spirit haste | a wolf copies the shaman's swing speed once, at the summon (`Pet.Enable` re-reads only for `SummonedAsPet`) | 425792 re-reads every 2 s, no `IsPet()` check (`spell_tweaks_feral_spirit_haste::CalcPeriodic`); that check is only in `spell_dk_pet_scaling` |
| Hunter pet armor pen | Pin's ticks never meet armor; Savage Rend's ticks are armor-reduced | Pin isn't a bleed, so armor and the inherited armor pen apply; Savage Rend's ticks are a bleed, no armor |
| Rupture haste | Glyph of Backstab adds 2 s and the ticks that fit (`sim/rogue/backstab.go`) | `spell_rog_glyph_of_backstab` only calls `SetDuration`; `AuraEffect::Update` stops at `GetTotalTicks()` = max duration / amplitude, so the extra seconds add no tick, switch on or off. The module's write-up claims the opposite |

**Wrong only with a switch off:** the master switch gates only Explosive Trap (the seal DoT keeps stacking
from Crusader Strike and Divine Storm; Glyph of Reckoning and the Kel'Thuzad nerfs aren't modelled); Omen
never rolls on Faerie Fire (Feral), where the server's stock 3.5 PPM still does; Improved Devouring Plague's
burst on a recast uses 8 ticks, the server the hasted count; a two-hander with a shield escapes Titan's
Grip's penalty.

**Right, but invisible on the default build:** Disease haste (no Epidemic), Rupture haste (no Weapon
Expertise, no Rupture cast), Rend haste (no Trauma), Exotic pet damage (a Wolf, and the live value is 0).

**No difference found:** Hunter pet haste, Ghoul armor pen, Disease, Deadly Poison and Rend haste, Balance
DoT scaling, Shadow DoT haste (on), Titan's Grip (dual two-handers), Omen (on).

## Step 1: the live test (now)

One throwaway Go test against the live server, two questions.

**Setup, nothing in a shared checkout changes:**
1. Take the wave loop's server lock: `mkdir G:\DevStuff\GitHub\.wave-loop\locks\server`, with an `owner`
   file naming this session. If it exists, don't run: report who holds it and wait. Remove it at the end,
   also on failure.
2. Private copy of the harness: `git -C [mod] archive HEAD e2e` unpacked into the session scratchpad (HEAD
   `97ca11a`, not the working tree, which holds the wave's edits). Add one file there,
   `tweakcheck_test.go`, tag `e2e`, package `e2e`, its own account prefix through `registerAccountPrefix`.
3. Run only it: the copy's `run.sh TestTweakcheck` from Git Bash with
   `AC_ROOT=G:/DevStuff/GitHub/azerothcore-wotlk-pb`,
   `SIMVAL_CHRONICLE_LOGS=<AC_ROOT>/env/dist/logs/chronicle_logs`, `SIMVAL_FIGHT_OUT=<scratch>/fights`,
   and `SIMVAL_GOLDCHAR_SETUPS=[int]/sim/core/testdata/simval/goldchar_setups.json` if the rogue is built
   from a golden setup. Read [mod]'s `README.md` ("Commands", "Output", "Live e2e tests", orphan cleanup)
   first.

No rebuild, restart or config change; no DB access but the harness's own test accounts, as every e2e run
makes; only short GM commands (they run on the world thread). If the run dies, clean its accounts with
the README's `TestCleanupOrphans`.

**A. Glyph of Backstab and Rupture's ticks.** A real fight under mod-chronicle, built like `ecCore3Fight`
(`eccore3_fight_test.go`: `fightBuild`, `fightPosition` behind the dummy, `logSizes`, `grownLog`): a
dagger rogue, Backstab 48657 and Rupture 48672 learned, `.simval combopoints 5`, Rupture, then three
Backstabs about 5 s apart (25 + 3 × 60 energy fits the regen), then wait 30 s. From the Chronicle log,
per run: Rupture's tick count and tick times after its apply line, and when its aura came off. Three
runs:

| Run | Build | If the source reading holds | If the glyph works as its tooltip says |
|---|---|---|---|
| control | no glyph | 8 ticks, off at 16 s | same |
| glyph | Glyph of Backstab (`.simval glyph`, the GlyphProperties id whose spell is 56800) | 8 ticks, off at about 22 s | 11 ticks |
| glyph + haste | plus Weapon Expertise (`.simval talent 30919`, `30920`) and Slice and Dice | floor(16000 / tick interval) ticks | about 22000 / tick interval |

Each Backstab must show as landed in the log, or the run proves nothing.

**B. Spirit Wolves and the shaman's haste.** No fight: `.simval info pet:<n>` snapshots, built like
`TestPetfixRecords` (`petfix_test.go`: `petfixSummon`, `strikesPetIndex(29264)`, `strikesPetRecord`). A
shaman with `.learn 51533` and Bloodlust or Heroism:
- Run 1: summon the wolves; snapshot a wolf at once and the shaman; cast Bloodlust; snapshot the wolf at
  about +0.5 s, +3 s and +5 s. Re-read every 2 s means the wolf's attack time drops from 1500 to about
  1150 ms by +3 s; a summon-only copy means it stays.
- Run 2: Bloodlust first, then summon; snapshot the wolf at once and at +3 s. This shows what a fresh wolf
  starts with (the audit suspects 0 until the first tick for the hunter pet's twin aura).
- From each snapshot: the wolf's attack time and whether it carries 425792.

Nothing here is committed, so no phase loop: one agent writes and runs the test, and this session checks its
numbers against the raw log lines and records it returns.

## Step 2: report and stop

Report, per run, what was measured against both expectations, with the log lines behind each number. Then
wait for the user's decision on the glyph: the sim copies the server, or it's a server bug to fix and the
sim stays.

## Step 3: file the work items (after the user's decision)

One doc edit in [sim] on master, left uncommitted.

1. Invoke `compact-docs-writer` first (the parity PLAN is reached from `CLAUDE.md`).
2. In `docs/azerothcore-parity/azerothcore-parity.PLAN.md`, "Loop work items": add rows PAR-FU-8 and
   PAR-FU-9 after PAR-FU-7 and the bullet after the "Each PAR-FU-4..7 fix removes its drops" line, as
   below, with step 1's measurements in place of "from source" where they now exist.
3. `git diff --stat` shows that one file; both rows have the table's five columns.

Do not touch: [int], `docs/wave-loop/wave-loop.PLAN.md` (the orchestrator's, changing on integration), the
guide pages and the INVESTIGATION (the items correct them), [mod]'s checkout.

Needs the user's word, each separately: committing the edit (the rows reach `integration` only once
committed on master and merged at the next wave start), and telling the wave-loop session ("add PAR-FU-8
and PAR-FU-9 from the parity PLAN to the next wave").

**PAR-FU-8 (next wave).** Scope: Spirit Wolves re-read the shaman's haste every 2 s. With
`SpellTweaks.FeralSpiritHaste` on, `Pet.Enable` (`sim/core/pet.go`) starts `refreshOwnerHaste` only for
`SummonedAsPet`, so a wolf keeps the swing speed from the summon; cast before Bloodlust and Flurry, as every
default APL does, it never gets them. 425792's `CalcPeriodic` sets a 2 s period with no `IsPet()` check;
that check is only in `spell_dk_pet_scaling::CalcPeriodic`, so the DK's guardians stay as they are. Add a
`Pet` field the class sets, start the timer through `startUpdateTimer` when it's set, set it in
`spirit_wolves.go`. Tests: a core table test (a guardian carrier without the field keeps its summon value;
with it, and a real pet, follow an owner speed change within 2 s plus a map update; a dismiss and resummon
leaves `MeleeSpeedMultiplier` where it started) and the wolves' wiring with the setting on, off and
`SpellTweaks.Enable` off. Report on against off on `TestEnhancement-Average-Default`, DPS and wolf swings
(today 97.85 a wolf on, about 100 off). Correct the INVESTIGATION's "Correction (PAR-P7-0d)" and
server-parity's haste carrier line. Its e2e file is step 1's test B. Owns: `sim/core/pet.go` (the carrier's
tick gate) and tests, `sim/shaman/spirit_wolves.go`, a new `sim/shaman/enhancement` test file, its e2e file.
Goldens: Enhancement, up (estimate +1 to 2.5%: wolf `casts` +30 to 45%, `amount_per_hit` unchanged); any
other suite moving is a finding. Needs: –.

**PAR-FU-9 (next wave).** Scope: the other spell-tweak switches, from the source audit; re-check each first
and drop what the server doesn't do.
- Pin (53548) isn't a bleed, so its ticks meet armor and the inherited armor pen; the sim's never do.
- Savage Rend's (53582) ticks are a bleed with no armor; the sim's carry `SpellFlagApplyArmorReduction`.
- Glyph of Backstab: as step 1's test A measured, and as the user decided.
- `SpellTweaks.Enable` off: Crusader Strike and Divine Storm stop stacking the seal DoT
  (`spell_tweaks_seal_dot_on_strikes`); `sim/paladin/sov.go` always stacks. Gate it.
- `OmenClarityFaerieFire` off: stock Omen still rolls on Faerie Fire (Feral), 3.5 PPM on a 1.5 s basis; the
  sim's never rolls. Build on EC-CLASS-FIX2's Omen work.
- `ShadowDotHaste` off: recasting Devouring Plague while it's up, Improved Devouring Plague's burst uses the
  hasted tick count (`Aura::RefreshTimers` runs before 71167 shortens the duration), 8 in the sim.
- Titan's Grip's −10% also applies to a two-hander with a shield (49152's amount); the sim needs dual wield.
- The picker's "Spell tweaks" tooltip and `SpellTweaksSettings.enable`'s comment name tweaks the sim doesn't
  model (Glyph of Reckoning's damage, the Detonate Mana and Frost Blast nerfs). Say what the switch gates.
- Left as they are: a 1 ms period difference from float32; an exotic pet without Beast Mastery, which the
  UI warns about.

Owns: `sim/hunter/pet_abilities.go` (Pin, Savage Rend), `sim/rogue/backstab.go`, `sim/paladin/sov.go`,
`sim/druid/{faerie_fire,talents}.go` (Omen's roll on Faerie Fire), `sim/priest/shadow_dot_haste.go`,
`sim/warrior/talents.go` (Titan's Grip), `ui/core/components/server_settings_picker.ts` (the tooltip),
`proto/common.proto` (the comment), their tests, its e2e file (step 1's test A). Goldens: none expected on
the live defaults (no suite runs a Crab, a Raptor, Glyph of Backstab or a two-hander with a shield; the
rest shows only with a switch off, each of which gets a test); report any move. Needs: –; file-disjoint
from PAR-FU-8.

**Bullet under the table.** Spell tweaks the sim has no model for (mod-spell-tweaks `338d826`): Hunter's
Mark off the global cooldown (the sim pre-applies it), Boar's Speed's −45% pet threat, hunter pet and ghoul
Avoidance cutting all damage taken, the Improved Concussive Shot and Go for the Throat tier swap (the UI
tree keeps stock rows), Glyph of Reckoning's damage, the Detonate Mana and Frost Blast nerfs.

## Verification

- Step 1: each reported number is backed by quoted Chronicle lines or `.simval info` records; the control
  run gives 8 ticks and 16 s, or the counting is wrong; the server lock is gone and no test account is left
  (`TestCleanupOrphans` finds none); `git -C [mod] status` is as before the run.
- Step 3: the rows read complete to someone holding only the parity PLAN; every file they name exists on
  master (checked 2026-10-10); `git diff --stat` in [sim] shows the parity PLAN only.
- The fixes themselves are verified inside the wave loop, per the RUNBOOK.
