# simval fixture

`simval.jsonl` is a capture of mod-sim-validation's e2e records. `tools/simval`'s `TestAgainstServerRecords`
replays it through `CheckRecord` on every `go test`, and `int-check.sh` runs it too. Recapture it
(never append) per [testing](../../../../docs/guide/testing.md#against-the-server);
`TestFixtureHasEveryE2ERecord` lists what a full run records.

`damage.jsonl` holds `TestSimvalDamage`'s `damage` records, which `TestDamageRecords` replays
([tools/simval](../../../../tools/simval/README.md#damage-probe)).

`timing.jsonl` holds `TestSimvalTiming`'s `timing` records and its bots' `info` records (regen), which
`TestTimingRecords` replays ([tools/simval](../../../../tools/simval/README.md#timing-probe)).

`threat.jsonl` holds `TestSimvalThreat`'s `threat` records, which `TestThreatRecords` replays
([tools/simval](../../../../tools/simval/README.md#threat-probe)).

`goldchar.jsonl` holds `TestSimvalGoldenCharacters`' `info` and `damage` records, which `TestGoldenCharacters`
replays against `goldchar_setups.json` ([tools/simval](../../../../tools/simval/README.md#golden-characters)).

`strikes.jsonl` holds `TestSimvalStrikes`' weapon strike and pet `damage`, pet `info` and Ghoul Frenzy `timing`
records, which `TestStrikesRecords` replays ([tools/simval](../../../../tools/simval/README.md#damage-probe)).

`pets.jsonl` holds `TestPetfixRecords`' pet `info`, `damage`, `spell`, `melee` and `timing` records (Greater Fire
Elemental, Mirror Image, Imp, Water Elemental, Shadowfiend), which `TestPetsRecords` replays
([tools/simval](../../../../tools/simval/README.md#damage-probe)).

`pets_casts.jsonl` holds 12 Greater Fire Elemental lives (`TestPetfixFireElementalCasts`' `PETFIX_CASTS` lines) and
15 Mirror Image summons (`TestPetfixMirrorImageCasts`' `PETFIX_IMAGES` lines: 6 into a fight, 6 ahead of the pull by
3 s and 3 by 2 s), which `TestPetsCastsRecords` replays ([tools/simval](../../../../tools/simval/README.md#damage-probe)).

`buffs.jsonl` holds `TestSimvalBuffs`' `BUFFS` lines, which `TestBuffRecords` replays
([tools/simval](../../../../tools/simval/README.md#buffs)). A disagreeing state or check goes in `droppedBuffs`
(`tools/simval/buffs_test.go`) with a row here; its line stays. `buffs_pairs.json` lists the pairs both read.

`mobs.jsonl` holds `TestSimvalMobs`' `MOBS` lines, which `TestMobRecords` replays
([tools/simval](../../../../tools/simval/README.md#mob-types)). A disagreeing line or check goes in `droppedMobs`
(`tools/simval/mobs_test.go`) with a row here; its line stays.

A record of any of these files that disagrees with the sim is a real mismatch: it's listed here, and the sim isn't
changed. Each file's section below says whether the record is removed or stays with only its failure let through.

`implemented_talents.json` is the talent guard's export (`TestTalentCoverage`, `sim/talent_coverage_test.go`):
every talent `sim/<class>` reads, with class, tree, `fieldName`, row, max points and its spell per rank. The
guard fails when it's stale; rewrite it with `go test ./sim -run TestTalentCoverage -update`.

`talent_paths.json` holds the talent sweep's paths, generated and kept current by `tools/simval`
([talent sweep](../../../../tools/simval/README.md#talent-sweep)). `talents.jsonl` holds a `TestSimvalTalentSweep`
run trimmed by `simval talents`, which `TestTalentSweepRecords` replays.

`spellmod_spells.json` lists, per spell-mod talent, the spells the sim names that its mods change on the server;
the spell-mod sweep probes them. Rewrite it with [spellmods](../../../../tools/acore/spellmods/README.md).
`spellmods.jsonl` holds a `TestSimvalSpellModSweep` run's `SPELLMOD` lines, which `TestSpellModRecords` replays
([spell-mod sweep](../../../../tools/simval/README.md#spell-mod-sweep)). WarlockAffliction's lines carry Curse of Doom
as shaped by `spell_warl_curse_of_doom`, so its damage isn't compared until a rerun on a module build whose
`NO_AMOUNT_SCRIPTS` lists that script.

`costaura_spells.json` is the same list for the school cost aura talents, and `costauras.jsonl` a
`TestSimvalCostAuraSweep` run's lines, which `TestCostAuraRecords` replays
([cost-aura sweep](../../../../tools/simval/README.md#cost-aura-sweep)); drops go in `droppedCostAuras`
(`tools/simval/costauras_test.go`) with a row here, as for `spellmods.jsonl`.

`spell_groups.json` is a SELECT export of `acore_world.spell_group` and `spell_group_stack_rules`, which
`TestSimSpellGroups` (`sim/spell_groups_test.go`) compares with the sim's exclusive effects. Re-export it with the
query in that test's doc comment when the server's rules change.

## Dropped records

`goldchar.jsonl`, each in `droppedGoldchar` (`tools/simval/goldchar_test.go`). A named check keeps its record
and is only logged; the others are removed. A setup in `goldcharRerecord` changed its enchants after the capture
and replays with those its record wears until a rerun replaces it.

| Setup | Record | Check | Why |
|---|---|---|---|
| DeathknightBlood | info | armor penetration % | the sheet field is the aura's raw sum; the server applies Blood-Gorged to white swings and five strikes by its class mask, and the sim does the same per spell (`applyBloodGorged`), so nothing reaches `BonusArmorPenPct` |
| DruidBalance, DruidBalancePhase3 | info | arcane spell crit % | spell specific crit in the sim (Improved Moonfire; Nature's Majesty and Dreamwalker 4pc on Starfire), spell mods on the server |
| MageFire, MageFrostFire | info | arcane spell crit % | spell specific crit in the sim (Khadgar's Regalia 4pc, World in Flames), spell mods on the server |
| WarlockDestruction | info | fire spell crit % | Devastation: spell specific crit in the sim, a spell mod on the server |

`talents.jsonl`, each in `droppedSweep` (`tools/simval/talents_test.go`). Records stay; a dropped step (empty field)
or field is only logged. A path in `sweepRerecord` changed its items, auras or talent ranks after the capture and
replays as its records stood until a rerun replaces them.

| Path | Talent | Field | Why |
|---|---|---|---|
| MageFire | improvedScorch | fire spell crit % | the sim gives Fireball and Frostfire Bolt the crit per spell; the server sheet shows school auras only |
| MageFire | livingBomb | fire spell crit % | the sim's school crit is its lowest fire spell's bonus, and the new Living Bomb spell has a lower one |
| WarlockDestruction | devastation | fire spell crit % | spell specific crit in the sim, a spell mod on the server |
| PriestDiscipline | holySpecialization | holy spell crit % | the sheet field is the aura's raw sum; Penance is holy but outside its class mask, so the sim's lowest holy spell stays at 0 |
| RogueCombat | maceSpecialization | armor penetration % | the sheet field counts the aura without a mace; the server's damage applies it only with one (`HasItemFitToSpellRequirements`), as the sim does, and the path carries two daggers |
| WarriorArms | maceSpecialization | armor penetration % | the same sheet artifact: the path carries a two-handed axe, so the server's damage and the sim give it nothing |
| DeathknightBlood | bloodGorged | armor penetration % | the sheet field is the aura's raw sum; the sim applies it per spell by the server's class mask (white swings, Plague, Blood, Heart and Death Strike, Rune Strike) |

`spellmods.jsonl`, each in `droppedSpellMods` (`tools/simval/spellmods_test.go`). Lines stay; a dropped step (spell 0),
spell (empty check) or check is only logged.

| Path | Talent | Spell | Check | Why |
|---|---|---|---|---|
| DruidFeralCat | berserk | 48564 Mangle (Bear) | cooldown | not a sim gap: under Berserk the sim resets the cooldown after each cast instead of zeroing it |
| PriestShadow | shadowFocus | 47585 Dispersion | miss | not a sim gap: Dispersion, a self buff, never rolls to hit; the sim's Shadow Focus hit leaves it out |

`costauras.jsonl`, each in `droppedCostAuras` (`tools/simval/costauras_test.go`), same rules as `spellmods.jsonl`.

| Path | Talent | Spell | Check | Why |
|---|---|---|---|---|
| PriestDiscipline | powerInfusion | 53007 Penance | channel | probe artifact: 53007 is a dummy with no channel, and the channel its script casts (52988, 53003) is what the sim's cast carries |

`timing.jsonl`, each in `knownTimingMismatches` (`tools/simval/timing.go`, keyed by class, spell, pass and check name;
passes are bare, talents and Bloodlust). The record stays and the check still runs; its failure is let through in the
passes named, and an entry no record fails is stale. It, `droppedTimingRecords` and `timingRecapture` (finishers
without combo point records yet, `tools/simval/timing_test.go`) are empty.

`threat.jsonl`, each in `droppedThreatRecords` (`tools/simval/threat_fixture.go`), removed from the file, or in
`knownThreatMismatches` (`tools/simval/threat.go`), the record kept and only that check's failure let through. The
capture is one run's records. A `recapture` row replays now; the next `TestSimvalThreat` run restores its record,
then remove the row and the entry.

| Bot | Spell | Why |
|---|---|---|
| FrostDeathKnight | 45524 Chains of Ice | the sim has no Chains of Ice |
| FrostDeathKnight | 49576 Death Grip | the sim has no Death Grip |
| Warlock | 47193 Demonic Empowerment | cast on the pet, so the server hands its bonus to the creatures fighting the pet; the probe's warlock has no pet in a fight |

`buffs.jsonl`, each in `droppedBuffs` (`tools/simval/buffs_test.go`). Lines stay; a dropped state or check is only
logged. None now.

`mobs.jsonl`, each in `droppedMobs` (`tools/simval/mobs_test.go`). Lines stay; a dropped line or check is only logged.
None now.

`strikes.jsonl`: dropped records in `droppedStrikesRecords`, known mismatches in `knownStrikesMismatches` (both
`tools/simval/damage_test.go`, keyed by probe and check; the record stays and only the failure is let through). None now.

## Spells missing from serverdata

`critCapable` assumes they crit, and `rangedWeaponSpell` that they're ranged weapon spells. None now.
