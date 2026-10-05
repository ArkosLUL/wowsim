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
| DeathknightBlood | 49909 Icy Touch, 47632 Death Coil | e0 damage crit | magic class on the server: the meta gem's 3% goes on the spell base 1.5 (1.545), then Runic Focus doubles the bonus (2.09); the sim gives them the melee base 2.0 times 1.03 (2.06) |
| DeathknightBloodTank | info | dodge %, parry % | Rune of the Stoneskin Gargoyle's +25 defense is skill on the server, which dodge and parry take undiminished; the sim adds it as defense rating, which diminishes |
| DruidBalance, DruidBalancePhase3 | info | arcane spell crit % | spell specific crit in the sim (Improved Moonfire; Nature's Majesty and Dreamwalker 4pc on Starfire), spell mods on the server |
| DruidBalancePhase3, DruidFeralApl | info | attack power | weapon feral AP: the server truncates dps × 14, then takes 767 off (983, 3175); the sim floors (dps − 54.8) × 14 (982, 3174) |
| DruidBalancePhase3, MageArcane, MageFire, MageFrostFire, MageFrost, PriestShadow, ShamanElemental, WarlockAffliction, WarlockDemonology, WarlockDestruction | info | spirit | Lightweave Embroidery (3722) also gives 1 spirit on the server; the sim's enchant has no stats |
| DruidFeral, DruidFeralApl | 48574 Rake | e0 damage crit | the server adds up the crit damage auras (meta gem 3%, Predatory Instincts 10%: 2.26); the sim multiplies them (2.266) |
| MageFire, MageFrostFire | info | arcane spell crit % | spell specific crit in the sim (Khadgar's Regalia 4pc, World in Flames), spell mods on the server |
| RogueCombat | 57965 Instant Poison IX, 57975 Wound Poison VII | e0, e1 damage crit | as Rake's, with Prey on the Weak's 20% (1.845 against the sim's 1.854) |
| WarlockDestruction | info | fire spell crit % | Devastation: spell specific crit in the sim, a spell mod on the server |
| WarriorArms | 47502 Thunder Clap, 57755 Heroic Throw | e0 damage crit | as Rake's, with Poleaxe Specialization's 5%, before Impale (2.392 against the sim's 2.3956) |

`talents.jsonl`, each in `droppedSweep` (`tools/simval/talents_test.go`). Records stay; a dropped step (empty field)
or field is only logged. A path in `sweepRerecord` changed its items or auras after the capture and replays as its
records stood until a rerun replaces them.

| Path | Talent | Field | Why |
|---|---|---|---|
| MageFire | improvedScorch | fire spell crit % | the sim gives Fireball and Frostfire Bolt the crit per spell; the server sheet shows school auras only |
| MageFire | livingBomb | fire spell crit % | the sim's school crit is its lowest fire spell's bonus, and the new Living Bomb spell has a lower one |
| WarlockDestruction | devastation | fire spell crit % | spell specific crit in the sim, a spell mod on the server |
| PriestDiscipline | holySpecialization | holy spell crit % | the sheet field is the aura's raw sum; Penance is holy but outside its class mask, so the sim's lowest holy spell stays at 0 |
| RogueCombat | maceSpecialization | armor penetration % | the sheet field counts the aura without a mace; the server's damage applies it only with one (`HasItemFitToSpellRequirements`), as the sim does, and the path carries two daggers |
| DeathknightBlood | bloodGorged | armor penetration % | the sheet field is the aura's raw sum; the sim applies it per spell by the server's class mask (white swings, Plague, Blood, Heart and Death Strike, Rune Strike) |
| PaladinProtection | improvedDevotionAura | armor | server +602 (Devotion Aura's armor +50%); the sim improves it only at `ImprovedDevotionAura == 5`, the talent has 3 ranks (`paladin.go`) |
| ShamanEnhancement | enhancingTotems | spell power | server +21 (Flametongue Totem's 58655 +15%); the sim's `FlametongueTotemAura` is a fixed 144 |

`spellmods.jsonl`, each in `droppedSpellMods` (`tools/simval/spellmods_test.go`). Lines stay; a dropped step (spell 0),
spell (empty check) or check is only logged.

| Path | Talent | Spell | Check | Why |
|---|---|---|---|---|
| DruidFeralCat | berserk | 48564 Mangle (Bear) | cooldown | not a sim gap: under Berserk the sim resets the cooldown after each cast instead of zeroing it |
| ShamanRestoration | naturesSwiftness | 49273 Healing Wave | cast time, cast time before haste | the sim has no Improved Healing Wave (a filler step of the path, so a fix needs a re-record): 3000 ms, the server's 2600 ms before the aura |
| PriestShadow | shadowFocus | 47585 Dispersion | miss | not a sim gap: Dispersion, a self buff, never rolls to hit; the sim's Shadow Focus hit leaves it out |
| DruidFeralCat | berserk | 48566 Mangle (Cat), 48572 Shred | cost | the server's `Player::ApplySpellMod` takes a cost's percent mods after its flat ones, (base + flat) × percent: 19 and 21; the sim's `costFor` before them: 16 and 12 |

`timing.jsonl`, each in `knownTimingMismatches` (`tools/simval/timing.go`, keyed by class, spell, pass and check name;
passes are bare, talents and Bloodlust). The record stays and the check still runs; its failure is let through in the
passes named, and an entry no record fails is stale. It, `droppedTimingRecords` and `timingRecapture` (finishers
without combo point records yet, `tools/simval/timing_test.go`) are empty.

`threat.jsonl`, each in `droppedThreatRecords` (`tools/simval/threat_test.go`), removed from the file; real
mismatches would go in `knownThreatMismatches` (`tools/simval/threat.go`), with the record kept: none now. The
capture is one run's records. A `recapture` row replays now; the next `TestSimvalThreat` run restores its record,
then remove the row and the entry.

| Bot | Spell | Why |
|---|---|---|
| FrostDeathKnight | 45524 Chains of Ice | the sim has no Chains of Ice |
| FrostDeathKnight | 49576 Death Grip | the sim has no Death Grip |

`buffs.jsonl`, each in `droppedBuffs` (`tools/simval/buffs_test.go`). Lines stay; a dropped state or check is only
logged.

| Setup | State | Check | Why |
|---|---|---|---|
| RogueAssassination | debuff.curse_of_weakness+debuff.vindication | dummy armor, armor multiplier | AP Debuffs (spell_group 1062) is exclusive highest per aura: Vindication's -574 AP removes the whole Curse of Weakness (-573), its 5% armor too; the sim keeps the armor |
| RogueAssassination | debuff.insect_swarm+debuff.scorpid_sting | dummy melee hit % | -6% on the server: `HandleModHitChance` adds each to a creature's `m_modMeleeHitChance`, which the miss roll reads raw, so Hit Chance Debuffs' same-effect rule (1060) never applies; the sim keeps -3% |

`mobs.jsonl`, each in `droppedMobs` (`tools/simval/mobs_test.go`). Lines stay; a dropped line or check is only logged.

| Bot | State | Dummy | Check | Why |
|---|---|---|---|---|
| HumanWarrior | elixirOfDemonslaying | 999005 | 47450 and 47486 weapon min, max; white normal damage | the server's 11406 gives +105 attack power against demons (Heroic Strike +25 = int(105 / 14 × 3.4)); the sim's Elixir of Demonslaying gives 265 |
| HumanWarrior | undeadSlaying3 | 999004 | 47450 and 47486 weapon min, max | the server keeps each set bonus as its own aura (57943 +1%, 29068 +2%) and multiplies them: ×1.0302; the sim's `makeUndeadSet` gives 2% in all |
| HumanWarrior | undeadSlaying4 | 999004 | 47450 and 47486 weapon min, max; white normal damage | the same plus 57944 +3%: ×1.0611 on the server, ×1.03 in the sim |
| HumanPaladin | scourgebane | 999004 | 48801 e0 and 48817 e0 damage min, max; 35395 weapon min, max | versus attack power only reaches `MeleeDamageBonusDone` on the server, added once after the weapon percent: Exorcism and Holy Wrath don't move (sim +21, +10 through their AP coefficients); Crusader Strike +33 = int(140 / 14 × 3.3) (sim 75% of it) |
| NightElfDruid | scourgebane | 999004 | attack power | the server's Predatory Strikes reads only the weapon's template, never its enchant: +0 form attack power (sim +28, 20% of 140) |

`strikes.jsonl`: dropped records in `droppedStrikesRecords`, known mismatches in `knownStrikesMismatches` (both
`tools/simval/damage_test.go`, keyed by probe and check; the record stays and only the failure is let through).

| Probe | Check | Dropped or known | Why |
|---|---|---|---|
| Cat 48566 Mangle, 48572 Shred, 62078 Swipe; bear 48480 Maul, 48564 Mangle; both passes | min, max | known | the sim's form weapons are 43–66 (cat) and 109–165 (bear); the server's form damage is `min(level, 60)` × 0.85–1.25 × the form's speed (`Player::CalculateMinMaxDamage`): 51–75 and 127.5–187.5 |

## Spells missing from serverdata

`critCapable` assumes they crit, and `rangedWeaponSpell` that they're ranged weapon spells.

| Spell | Why |
|---|---|
| 75 Auto Shot | not in `sim/core/serverdata` |
