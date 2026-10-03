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

A record of any of these files that disagrees with the sim is a real mismatch: it's dropped from the file and
listed here, and the sim isn't changed.

`implemented_talents.json` is the talent guard's export (`TestTalentCoverage`, `sim/talent_coverage_test.go`):
every talent `sim/<class>` reads, with class, tree, `fieldName`, row, max points and its spell per rank. The
guard fails when it's stale; rewrite it with `go test ./sim -run TestTalentCoverage -update`.

`talent_paths.json` holds the talent sweep's paths, generated and kept current by `tools/simval`
([talent sweep](../../../../tools/simval/README.md#talent-sweep)). `talents.jsonl` holds a `TestSimvalTalentSweep`
run trimmed by `simval talents`, which `TestTalentSweepRecords` replays.

`spellmod_spells.json` lists, per spell-mod talent, the spells the sim names that its mods change on the server;
the spell-mod sweep probes them. Rewrite it with [spellmods](../../../../tools/acore/spellmods/README.md).
`spellmods.jsonl` holds a `TestSimvalSpellModSweep` run's `SPELLMOD` lines, which `TestSpellModRecords` replays
([spell-mod sweep](../../../../tools/simval/README.md#spell-mod-sweep)).

`spell_groups.json` is a SELECT export of `acore_world.spell_group` and `spell_group_stack_rules`, which
`TestSimSpellGroups` (`sim/spell_groups_test.go`) compares with the sim's exclusive effects. Re-export it with the
query in that test's doc comment when the server's rules change.

## Dropped records

`goldchar.jsonl`, each in `droppedGoldchar` (`tools/simval/goldchar_test.go`). A named check keeps its record
and is only logged; the others are removed.

| Setup | Record | Check | Why |
|---|---|---|---|
| DeathknightBlood | info | armor penetration % | the sheet field is the aura's raw sum; the server applies Blood-Gorged to white swings and five strikes by its class mask, and the sim does the same per spell (`applyBloodGorged`), so nothing reaches `BonusArmorPenPct` |
| DruidBalance, DruidBalancePhase3 | info | arcane spell crit % | spell specific crit in the sim (Improved Moonfire; Nature's Majesty and Dreamwalker 4pc on Starfire), spell mods on the server |
| DruidBalance, DruidBalancePhase3, DruidFeral, DruidFeralApl, DruidFeralTank, PriestShadow, PriestSmite | info | max health | the e2e's `.modify hp` pinned max health to the client's value from before a stamina buff or form (Mark of the Wild, Power Word: Fortitude, Dire Bear Form); the record's own stamina gives the sim's number |
| HunterBM, HunterMM, HunterSV, PaladinRetribution, RogueCombat, RogueAssassination, RogueSubtlety, WarriorFury, WarriorArms | info | parry % | the e2e never teaches Parry (3127), a trainer spell, so the server can't parry; the sim's character can |
| HunterMM | info | attack power, ranged attack power | the setup's auras cast Trueshot Aura (19506), which its talents don't take, so the sim has no 10% |
| MageFire, MageFrostFire | info | arcane spell crit % | spell specific crit in the sim (Khadgar's Regalia 4pc, World in Flames), spell mods on the server |
| WarlockDestruction | info | fire spell crit % | Devastation: spell specific crit in the sim, a spell mod on the server |

`talents.jsonl`, each in `droppedSweep` (`tools/simval/talents_test.go`). Records stay; a dropped step (empty field)
or field is only logged.

| Path | Talent | Field | Why |
|---|---|---|---|
| MageFire | improvedScorch | fire spell crit % | the sim gives Fireball and Frostfire Bolt the crit per spell; the server sheet shows school auras only |
| MageFire | livingBomb | fire spell crit % | the sim's school crit is its lowest fire spell's bonus, and the new Living Bomb spell has a lower one |
| WarlockDestruction | devastation | fire spell crit % | spell specific crit in the sim, a spell mod on the server |
| PriestDiscipline | holySpecialization | holy spell crit % | the sheet field is the aura's raw sum; Penance is holy but outside its class mask, so the sim's lowest holy spell stays at 0 |
| RogueCombat | endurance | the whole step | the path learns it last since the sim implements it; the live capture predates the step, so rerun the path (`SIMVAL_TALENT_ONLY=RogueCombat`) and remove this row |
| RogueCombat | maceSpecialization | armor penetration % | the sheet field counts the aura without a mace; the server's damage applies it only with one (`HasItemFitToSpellRequirements`), as the sim does, and the path carries two daggers |
| DeathknightBlood | bloodGorged | armor penetration % | the sheet field is the aura's raw sum; the sim applies it per spell by the server's class mask (white swings, Plague, Blood, Heart and Death Strike, Rune Strike) |

`spellmods.jsonl`, each in `droppedSpellMods` (`tools/simval/spellmods_test.go`). Lines stay; a dropped step (spell 0),
spell (empty check) or check is only logged.

| Path | Talent | Spell | Check | Why |
|---|---|---|---|---|
| DruidFeralCat | berserk | 48564 Mangle (Bear) | cooldown | not a sim gap: under Berserk the sim resets the cooldown after each cast instead of zeroing it |
| ShamanRestoration | naturesSwiftness | 49273 Healing Wave | cast time, cast time before haste | the sim has no Improved Healing Wave (a filler step of the path, so a fix needs a re-record): 3000 ms, the server's 2600 ms before the aura |
| PriestShadow | shadowFocus | 47585 Dispersion | miss | not a sim gap: Dispersion, a self buff, never rolls to hit; the sim's Shadow Focus hit leaves it out |

`timing.jsonl`, each in `knownTimingMismatches` (`tools/simval/timing.go`, keyed by class, spell, pass and check name;
passes are bare, talents and Bloodlust). The record stays and the check still runs; its failure is let through in the
passes named, and an entry no record fails is stale. `droppedTimingRecords` is empty.

| Class | Spell | Check | Passes | Why |
|---|---|---|---|---|
| Rogue | 48672 Rupture, 6774 Slice and Dice | aura duration, e0 ticks | all | the probe has no combo points; the sim's finisher has no duration (Rupture) or its 5-point one (Slice and Dice) |

`threat.jsonl`, each in `droppedThreatRecords` (`tools/simval/threat_test.go`), removed from the file; real
mismatches go in `knownThreatMismatches` (`tools/simval/threat.go`), and the record stays.

| Bot | Spell | Why |
|---|---|---|
| FrostDeathKnight | 45524 Chains of Ice | the sim has no Chains of Ice |
| FrostDeathKnight | 49576 Death Grip | the sim has no Death Grip |
| ProtPaladin | 25742 Seal of Righteousness | the sim's proc reads the main hand's speed; the sim character has no weapon |
| Warlock | 59092 Dark Pact | the sim's Dark Pact drains the pet's mana; the sim character has no pet |
| ElementalShaman | 60103 Lava Lash | the sim's damage is NaN without an off hand weapon |

`threat.jsonl`'s known mismatches (`knownThreatMismatches`), for EC-THREAT-FIX, sim vs server. The capture is one
run's records with the Rogue's from a rerun.

| Check | Probes | Why |
|---|---|---|
| bonus | 71 probes of every bot: sim 0, server the spell's level (1 to 80) | the server adds `SpellLevel` per cast without a `spell_threat` row, unmodified |
| bonus | 53385 Divine Storm 30, 48668 Eviscerate 39.5, 48672 Rupture 39.5, 57993 Envenom 40, 42897 Arcane Blast 40, 48156 Mind Flay 40; sim 0 | the same, split over the dummy and the caster an effect also hits |
| bonus | Defensive Stance 47450 Heroic Strike 537.04 vs 259, 57823 Revenge 250.89 vs 121, 47488 Shield Slam 1596.60 vs 770; Berserker 47450 181.30 vs 259 | the sim multiplies the flat by the unit's threat multiplier |
| bonus | 47520 Cleave 466.54 (Defensive), 157.50 (Berserker) vs 112; on 2 targets 466.54 vs 56 | the sim's flat is 225 and multiplied; the server's 112 is unmodified and split |
| bonus | 48480 Maul 891.29 vs 422, 48568 Lacerate 1083.63 vs 515 | the sim's 424 and 515.5 times Dire Bear Form; the server's rows unmodified |
| bonus | 47498 Devastate 712.04 vs 28.40 | the sim adds 315 and 5% of AP and multiplies; the server's row is the 5% alone |
| bonus | 50783 Slam 290.29 (Defensive), 98 (Berserker) vs 1 | the sim puts Slam's 140 on the damage spell; the server's row is on the cast 47475, whose bonus `SPELL_ATTR3_SUPPRESS_TARGET_PROCS` drops |
| bonus | 47467 Sunder Armor 805.35 vs 77 | the server's 7386 row isn't in this rank's chain, so it adds the level |
| bonus | 47437 Demoralizing Shout 131.05 vs 79, 48560 Demoralizing Roar 260.66 vs 77 | the sim has a flat the server has no row for |
| bonus | 16857 Faerie Fire (Feral) 1328.53 (Bear), 448.72 (Cat) vs 18; 60089 its damage 0 vs 638 | the sim's flat is on the cast, the server's on the damage spell |
| bonus | 55271 Scourge Strike 0 vs 120 (both DKs) | a `spell_threat` row the sim lacks |
| bonus | 61657 Fire Nova 0 vs 56 | a positive spell: the server's level reaches the dummy as assist threat, x0.7 |
| tick bonus | 47867 Curse of Doom 160 vs 0 | the sim adds its 160 to each tick, the server its level (80) to the cast |
| damage threat | Defensive 47488 Shield Slam x2.6955, 12809 Concussion Blow x4.1470, 47471 Execute x2.5919 vs x2.0735; Berserker 1680 Whirlwind, 47471 x0.875, 7384 Overpower x0.525 vs x0.7 | the sim's spell multipliers (1.3, 2, 1.25, 0.75) have no `spell_threat` percent |
| damage threat | RetPaladin's 7 probes x1 vs x0.7 | the sim never calls `applyFanaticism` |
| damage threat | Bear 48480, 48564, 60089 x2.1021 vs x2.0735, 48568 x1.0511 vs x1.0368; 48562 Swipe (Bear) x2.1021 vs x3.1103 (also on 3 targets) | Dire Bear Form's multiplier; Swipe's `spell_threat` x1.5 |
| damage threat | Unholy Presence's 7 probes x0.75 vs x0.55 (52212 x1.425 vs x1.045) | the server's presence takes 20% besides Subversion's 25% |
| damage threat | 42926 Flamestrike x0.9 vs x0.8; 47610 Frostfire Bolt x0.68 vs x0.72 | Burning Soul at 5% a point; the server multiplies the two talents, the sim adds them |
| damage threat | ShadowPriest 48127, 58381, 48125, 48160, 48158 x0.76 vs x0.525; 48300 x0.85; 48123 Smite x1 vs x0.7 | Shadowform's 30% is missing in the sim; Shadow Affinity is 24% there (15% on Devouring Plague), 25% on the server |
| damage threat | 47815 Searing Pain x0.8 vs x1.6 | a `spell_threat` x2 the sim lacks |

## Spells missing from serverdata

`critCapable` assumes they crit, and `rangedWeaponSpell` that they're ranged weapon spells.

| Spell | Why |
|---|---|
| 75 Auto Shot | not in `sim/core/serverdata` |
