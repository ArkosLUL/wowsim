# simval fixture

`simval.jsonl` is a capture of mod-sim-validation's e2e records. `tools/simval`'s `TestAgainstServerRecords`
replays it through `CheckRecord` on every `go test`, and `int-check.sh` runs it too. Recapture it
(never append) per [testing](../../../../docs/guide/testing.md#against-the-server);
`TestFixtureHasEveryE2ERecord` lists what a full run records.

`damage.jsonl` holds `TestSimvalDamage`'s `damage` records, which `TestDamageRecords` replays
([tools/simval](../../../../tools/simval/README.md#damage-probe)).

`goldchar.jsonl` holds `TestSimvalGoldenCharacters`' `info` and `damage` records, which `TestGoldenCharacters`
replays against `goldchar_setups.json` ([tools/simval](../../../../tools/simval/README.md#golden-characters)).

A record of any of these files that disagrees with the sim is a real mismatch: it's dropped from the file and
listed here, and the sim isn't changed.

`implemented_talents.json` is the talent guard's export (`TestTalentCoverage`, `sim/talent_coverage_test.go`):
every talent `sim/<class>` reads, with class, tree, `fieldName`, row, max points and its spell per rank. The
guard fails when it's stale; rewrite it with `go test ./sim -run TestTalentCoverage -update`.

`talent_paths.json` holds the talent sweep's paths, generated and kept current by `tools/simval`
([talent sweep](../../../../tools/simval/README.md#talent-sweep)). `talents.jsonl` holds a `TestSimvalTalentSweep`
run trimmed by `simval talents`, which `TestTalentSweepRecords` replays.

## Dropped records

`goldchar.jsonl`, each in `droppedGoldchar` (`tools/simval/goldchar_test.go`). A named check keeps its record
and is only logged; the others are removed.

| Setup | Record | Check | Why |
|---|---|---|---|
| HunterBM | 49045 Arcane Shot, 49001 Serpent Sting | all | The Beast Within's permanent 10% (34692) is on the server; the sim registers it with Bestial Wrath, only for a hunter with a pet, and the setup has none |
| HunterMM | 49001 Serpent Sting | all | server ×1.05 more: Ranged Weapon Specialization (19509) applies to the sting there, not in the sim |
| HunterSV | 49001 Serpent Sting | all | sim ×1.03 more: Noxious Stings, with the sting already on the target; the probe works the tick out without it |
| HunterSV | 63672 Black Arrow | all | sim 5.5% below the server |
| ShamanElemental | info | spell power | sim 84 above the server: it always adds Glyph of Totem of Wrath's 30% of 280; the server gives it only as aura 63283 after a Totem of Wrath cast, which the e2e doesn't make |
| WarlockAffliction | info | spell hit % | the sim gives Suppression's 3% to every school; the server only to fire and shadow (aura 199) |
| WarriorFury, WarriorArms | 47502 Thunder Clap | all | sim ×1.06: Two-Handed Weapon Specialization (12712) only counts for weapon spells on the server; Arms' Thunder Clap also crits for 2.2 in the sim, 2.32 on the server |

`talents.jsonl`, each in `droppedSweep` (`tools/simval/talents_test.go`). Records stay; a dropped step (empty field)
or field is only logged.

| Path | Talent | Field | Why |
|---|---|---|---|
| WarlockAffliction | darkPact | all | the sim panics building a warlock with Dark Pact and no pet (`sim/warlock/darkpact.go` reads `warlock.Pet`) |
| WarlockAffliction | suppression | spell hit % | the sim gives the 3% to every school; the server only to fire and shadow (as in `goldchar.jsonl`) |
| WarriorProtection, PaladinProtection | shieldMastery, redoubt | block value | server +105, sim +108: the server takes 10 off block value before the % multiplier (`Player::GetShieldBlockValue`); the sim never does |
| HunterBeastMastery, HunterMarksmanship, HunterSurvival | improvedAspectOfTheMonkey, combatExperience, lightningReflexes, huntingParty | dodge % | the server keeps aura dodge % (aspect, Improved Aspect of the Monkey) out of diminishing returns; the sim diminishes it (IAotM +6 vs +4.6; agility's dodge ~20% higher on the server) |
| DeathknightFrost | improvedIcyTalons | ranged haste | the server's 5% haste is melee and ranged; the sim's melee only |
| ShamanRestoration | naturesBlessing | spell power | healing only on the server (`MOD_SPELL_HEALING_OF_STAT_PERCENT`); the sim adds 30% of intellect to spell power |
| MageArcane | magicAbsorption | each resistance | server +80 at 2/2 (1 per level), sim +4 |
| DruidFeralCat, DruidFeralBear | improvedMarkOfTheWild, survivalOfTheFittest | ranged attack power | the server gives druids in Cat or Bear Form no ranged AP from agility; the sim does |

## Spells missing from serverdata

`critCapable` assumes they crit, and `rangedWeaponSpell` that they're ranged weapon spells.

| Spell | Why |
|---|---|
| 75 Auto Shot | not in `sim/core/serverdata` |
