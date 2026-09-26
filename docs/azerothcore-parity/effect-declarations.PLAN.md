# Effect declarations

## Context

[ADR 0003](../adr/0003-spell-data-split.md) keeps spell damage numbers hand-written in Go "and a test fails
on any mismatch with server data". No such test exists:
- Class code writes base rolls and coefficients inside `ApplyEffects`/`OnSnapshot` closures, where nothing
  can read them.
- Outside the generator, `serverdata.Bonus` and `Effect.BasePoints` are read only by
  `sim/core/serverdata/tables_test.go`.
- Goldens catch changes, not wrong values.

The ADR also lists school as generated from server data, but `applyServerData` never syncs it.

Known wrong today:
- Arcane Barrage 44781 is declared Frost; the server says Arcane.
- Holy Fire's dot uses 0.024; its bonus row says 0.0529.
- Devouring Plague uses 0.1849; its row says 0.18.
- Frostbolt is 804–866 where the server has 803–865.
- Fireball, Shadow Bolt and Pyroblast each roll one over the server's maximum.

The goal: each spell declares its numbers per server effect, and `RegisterSpell` checks them the way P3-2
checks timing.

## Decisions (settled with the user, 2026-09-23)

- **The declared values drive the damage** ([ADR 0003](../adr/0003-spell-data-split.md)).
- **Declared on the config, per server effect,** with the server's exact values (0.7143 → 0.714).
- **What's checked:**
  - per effect: the base range; the SP, AP and healing coefficients; the weapon percent (effect 31)
  - per spell: the school and the missile speed, synced like P3-2's timing

  The `Outcome*` choice isn't checked.
- **Heals and hunter pet abilities** (`PetSpecialAbilityConfig`) go through the same path. The check covers
  class spells only. Shared spells in `sim/common` and `sim/core` (item procs, explosives, racials) wait:
  see Not scheduled.
- **Mods follow `Player::ApplySpellMod`.** Talents, glyphs, relics and set bonuses are mods on top of the
  declared rank values:
  - Every op gives `value × pct + flat`, except cast time and duration: `(value + flat) × pct`.
  - Percents multiply each other for `SPELLMOD_DAMAGE`/`SPELLMOD_DOT`, and add up for every other op.
  - Effect mods (`SPELLMOD_ALL_EFFECTS`, `SPELLMOD_EFFECTn`) apply to the roll in `CalcValue`, before the
    coefficient. The coefficient's own mods are op 24 (`SPELLMOD_BONUS_MULTIPLIER`), e.g. Empowered Fire.
  - The declarations take effect and op-24 mods. `SPELLMOD_DAMAGE`/`DOT` percents stay in
    `DamageMultiplier` and belong to PAR-P8.

  Conditions checked during the fight stay in the closure, e.g. Incinerate's range while Immolate is up.
- **Mismatch:** fix the number, or add an allowlist entry with `Why` (P3-2's convention).
- **Damage from a script or another spell:**
  - A declaration names its source spell with `From`, e.g. DK Death Coil 47632 → 49895.
  - Script math gets an allowlist entry that names the script: Bloodthirst's 50% AP, Execute per rage,
    Concussion Blow, Shockwave.

## Server data facts

- `serverdata.Effect` has `BasePoints`, `DieSides`, `PointsPerLevel`, and `BonusMultiplier` (the
  coefficient when no bonus row exists).
- `Bonus` has `Direct`, `Dot` (per tick), `AP` and `APDot`. `resolveBonus`
  (`tools/acore/gen_serverdata/model.go`) uses the spell's own row, else its first rank's.
- `BasePoints` are raw. The level-80 value is
  `bp + int32((min(80, maxLevel) − max(baseLevel, spellLevel)) × ppl) + 1..DieSides`
  (`SpellEffectInfo::CalcValue`; `maxLevel` 0 means no cap). The capture has the three levels
  (`tools/acore/spellids/spellset/dump.go`), but `gen_serverdata` drops them.
- School encodings differ:
  - sim: Physical 2, Arcane 4, Fire 8, Frost 16, Holy 32, Nature 64, Shadow 128
  - server: Physical 1, Holy 2, Fire 4, Nature 8, Frost 16, Shadow 32, Arcane 64
- `ServerConflict` and `ServerConflictAllowance` hold int64 values, and the test matches allowances on
  `Sim`/`Server`.
- `applyServerData` returns early for triggered spells (empty `DefaultCast`, no cooldown) and for enemy
  units.

## Work items

### PAR-DECL (I4)

- **Declaration:** a type on `SpellConfig` and `DotConfig` (per tick), with these fields:
  - `Effect`: the server effect index
  - optional `From`: the source spell id
  - `Min`/`Max`: level-80, server-exact
  - SP/healing and AP/RAP coefficients
  - `WeaponPct`

  The spell rolls the value for its closure. API names are this item's call.
- **Mods:** effect and op-24 mods, applied as in Decisions.
- **Check:** `applyServerData` compares:
  - the range with the source effect's level-80 range
  - coefficients with the bonus row, else the effect's `BonusMultiplier`
  - `WeaponPct` with effect 31
  - the school, through a server-mask ↔ `SpellSchool` converter
  - `MissileSpeed` with `Speed`

  All of these run before the early return for triggered spells, or 47632 and every proc go unchecked.
  Conflicts and allowances carry float values next to the int64 ones. Like timing, an undeclared value
  (0) is a conflict and the server's value wins.
- **Generator:** `gen_serverdata` keeps the spell levels, and `serverdata` offers an effect's level-80
  range. PAR-DECL is the only item in I4 and I5 that regenerates `serverdata`. A sweep that needs a new
  spell id there raises a follow-up.
- **Hunter pets:** `PetSpecialAbilityConfig` feeds the same check.
- **Undeclared list:** per class, next to its allowlist (`sim/<class>/`), so the I5 items stay
  file-disjoint. It lists class spells that have a server damage or heal effect but no declaration. The
  test fails on a new undeclared spell and on a stale entry.
- **Proof spells**, one per shape:
  - Frostbolt: direct
  - Corruption: dot, with Empowered Corruption as an op-24 mod
  - Mortal Strike: flat weapon bonus
  - Obliterate: `WeaponPct`, split out of `DamageMultiplier`
  - Arcane Shot: RAP
- **One-line fixes the new checks flag:**
  - School: Arcane Barrage, Mirror Image Fire Blast (59637), Ghoul Frenzy. Summon or aura ids that deal
    the sim's damage get allowlist entries instead.
  - Missile speed, for spells declaring none (about 22): every hunter shot (40), Avenger's Shield, Hammer of
    the Righteous, Hammer of Wrath, Holy Wrath, Chaos Bolt, Heroic Throw, Shattering Throw, Waterbolt,
    Searing Totem, felhunter 47964, Shadowmourne 71904. Their travel time moves goldens.

#### As built

**API** (`sim/core/spell_effect.go`):
- `core.SpellEffect{Effect, FromSpellID, Min, Max, SP, AP, WeaponPct}` goes on `SpellConfig.Direct` and
  `DotConfig.Tick` (Dot or Hot). Closures read `spell.Direct` and `dot.Tick`, which hold the server's value
  where a conflict applied, then the mods: `spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()`.
  `Roll` draws no random number when `Min == Max`.
- `SpellConfig.Mods []core.SpellMod{Op, Flat, Pct}`, one list per spell, in server units (BasePoints+1,
  op-24 flats in hundredths). Ops: `SpellModEffect1/2/3`, `SpellModAllEffects`, `SpellModBonusMultiplier`.
  `RegisterSpell` checks the rank values, then folds the mods like `CalcValue` (float32, truncated). Op 24
  skips a 0 coefficient; no op touches AP.
- Conflict fields: `ServerSchool` (unchecked when either side has no school), then the floats
  (`ServerField.IsFloat`): `ServerMissileSpeed`, `ServerMin/Max/SP/AP/WeaponPct`, `ServerTick*`. A float
  entry sets `SimFloat`/`ServerFloat`, as the table's decimal (0.857).

**Declaring:**
- A physical spell declares the server's SP but doesn't apply it: the server scales only the physical
  damage-done bonus, which the sim lacks (pet abilities, Bloodthirst, Shield Slam).
- `WeaponPct` also scales the flat roll when the server lists the flat effect first, so not for Devastate
  20243 or Blood-Caked Strike 50463.
- An off-hand strike's flat bonus takes the off-hand's 0.5 too (`fixed_bonus × weapon_total_pct`); the
  sim halves only the weapon part.
- `CalcValue` adds `PointsPerComboPoint` × points before the effect mods. No field holds it: the combo
  part stays in the closure, with the effect percent applied.
- A dot dealt as another spell (`DotConfig.Spell`: Searing and Magma Totem) is checked against the
  registering spell, so declare it with `FromSpellID`.
- `PetSpecialAbilityConfig` passes `Damage` and `MissileSpeed` through, not `Mods`.

**Undeclared lists:** `UndeclaredSpells []int32` in `sim/<class>/serverdata_undeclared.go`, 194 spells.
`TestServerDataConflicts` fails on a missing or stale entry and prints the server's values to declare.
- A class spell is registered by that class alone (player or pet) and, on a player, has its
  SpellFamilyName. Items, relics and racials (family 0) aren't listed.
- Counted: school damage, leech, heal, periodic damage/heal/leech, and the weapon effects as one, plus
  those of a spell triggered through TRIGGER_SPELL, TRIGGER_MISSILE or PERIODIC_TRIGGER_SPELL (Volley's
  58433: declare with `FromSpellID`). An effect all 0 at level 80 isn't.
- Missed: damage a script deals behind a dummy effect (Execute 47471's 20647, Explosive Shot, Death and
  Decay, Holy Shock). Declare those anyway.
- An effect the sim doesn't deal stays listed, with a comment saying so.

**Missile speed:** only a closure calling `spell.WaitTravelTime` waits, and none of the spec's missile
spells do, so declaring their speeds moved no golden. The sweeps add the wait: hunter shots, the paladin
four, Chaos Bolt, both throws, Waterbolt, Searing Totem's bolt, Imp Firebolt (the spec's "felhunter
47964"). Shadowmourne 71904 is shared, so no item owns its wait.

**Entries:**
- KeepSim, for an id carrying another server spell's damage, until the class deals it as that spell:
  Windfury 58804 (25504), Summon Infernal 1122 (22703), Scourge Strike 55271 tag 2 (70890), Divine Aegis
  47515 (47753), Empowered Renew 63543 (63544), T10 70770 (70772), The Fists of Fury 41989 (41990), Hodir's
  swing 63511 tag 1. Also the stand-in harmful spell 52789, the external Shattering Throw, and Empower
  Rune Weapon 47568's 1 yd/s (a self cast never travels).
- Not KeepSim, so the server's value applies until the class declares it: missile speeds of Lightning
  Bolt, Lava Burst, Fan of Knives, Unholy Blight, Gargoyle Strike and Typhoon's cast 61384 (declare, then
  wait); Mind Freeze 47528 Frost; the totem summons 58704/58734 Physical; Shadowcrawl 63619 Arcane
  (PAR-P7-PRI). Shared: Hyperspeed
  Acceleration 54758, Frozen Blows 63512 and Leeching Swarm 66118 (PAR-P7-TANK).

**Goldens:** Frost mage -0.02%, from Frostbolt's 803–865 and 0.857. Every other suite holds.

**BenchmarkSimulate** against a4292b95a: raid, hunter, Ret and rogue within ±2%, Elemental +2.5% (1
iteration) and +3.5% (100). No new code shows in its profile, but layout moves it: keeping `Direct` and
`Tick` last in `Spell` and `Dot` saved about 1%, and 40 bytes of `SpellConfig` padding cut an earlier
+6.8% to +1.8%. Moving `AutoAttacks` to the end of `Unit` didn't help, so the cause is open.

### Class sweep (I5)

Three items:
- **PAR-DECL-1:** mage, warlock
- **PAR-DECL-2:** shaman, druid (Feral Tank included), hunter with pets
- **PAR-DECL-3:** warrior, rogue, paladin, DK, including the tank-spec spells in those folders

Each item:
- declares every damage and heal effect
- moves effect and op-24 mods onto the declarations
- splits `WeaponPct` out of `DamageMultiplier`
- halves an off-hand strike's flat bonus too (As built, Declaring), e.g. Obliterate
- adds `spell.WaitTravelTime` to its missile spells (As built, Missile speed)
- for each mismatch, fixes the number or adds an allowlist entry
- empties its classes' undeclared list

#### PAR-DECL-1 as built

**Mage** (`sim/mage/serverdata_undeclared.go` is empty):
- Every damage spell declares, Mirror Image's and the Water Elemental's included, except Deep Freeze: the
  server deals it as 71757 (2369–2641, SP 2.143), which `serverdata` lacks, so 44572 keeps its hand-written
  numbers until a regeneration adds it.
- Op-24 mods: Empowered Fire (Flat 5 a rank) on Fireball, Frostfire Bolt and Pyroblast (dot included),
  Arcane Empowerment (Flat 3 a rank) on Arcane Blast and the missile 42845.
- Mirror Image's spells take the mage's mods (`Unit::GetSpellModOwner`), and its Frostbolt 59638 has
  Frostbolt's class mask: Empowered Frostbolt's op 24, plus Permafrost's and Chilled to the Bone's
  `SpellModEffect1` flats, which land on its damage (effect 0) instead of a slow.
- Fixed numbers: Ice Lance 223–257, Arcane Missiles 361, Scorch, Fireball, Pyroblast and Flamestrike one
  lower at the top, Flamestrike's direct SP 0.2357 (Spell.dbc's 0.243 before), Waterbolt SP 0.83, Mirror
  Image's rolls 88–98 and 163–169 (flat before), and every `x/3.5` coefficient as the table's decimal.
- Flamestrike rank 8's downranking factor is `Unit::CalculateLevelPenalty`'s (72 + 6) / 80 = 0.975, not 0.9.
- Waterbolt waits its travel; the other mage missiles already did. Pets have no `DistanceFromTarget`, so
  they wait the 5 yd floor.
- Goldens (Average-Default): Frost +0.383%, from Mirror Image's Frostbolt mods (+0.43%) less Waterbolt's
  SP and wait; Arcane -0.026% (Arcane Blast and Missiles); Fire +0.005% and FrostFire +0.003%. Mirror
  Image's new rolls shift the random stream, up to ±0.23% on single rows; with averages in their place,
  every Arcane, Fire and FrostFire single-target row stays within 0.03%. `frost_aoe`, all pet damage,
  +2.0 to +4.6%; `fire_aoe` +0.05 to +0.43% (Flamestrike).

**Warlock** (`sim/warlock/serverdata_undeclared.go` is empty):
- Every damage spell declares, the pets' and the Infernal's included. Immolation Aura declares its tick
  from 50590 and Summon Infernal its landing from 22703 (`FromSpellID`); 1122 keeps its KeepSim school.
- Mods: Shadow and Flame (op 24, Pct 4 a rank) on Shadow Bolt, Shadowburn, Incinerate and Chaos Bolt;
  Everlasting Affliction (Flat 1 a rank) on Unstable Affliction, and on both Seed spells at rank 1 only,
  the one rank whose class mask has Seed. Improved Imp (op 8, Pct 10 a rank) moved out of Firebolt's
  `DamageMultiplier`, so it scales the roll only.
- Fixed numbers: Shadow Bolt max 774, Searing Pain 347–409, SP 0.4286 on Searing Pain, Shadowburn and
  Haunt, Firebolt SP 0.714 (0.571 before).
- Script math stays in the closure, on the declared values:
  - Incinerate adds a quarter of its roll while Immolate is up.
  - Conflagrate declares effect 0's 1 and effect 1's 60, the hit's percent of the consumed Immolate's
    five snapshot ticks. Each dot tick deals 13% of them.
  - Curse of Agony ramps the whole tick, spell power included: 0.5, 1, 1.5 and 2 from ticks 1, 5, 9, 13.
- The Infernal's Immolation is 40 + 1.35 × its own SP (15% of the warlock's, spirit-based included),
  not 0.2 × the warlock's SP less the spirit part. The sim never ticks it (INVESTIGATION).
- Chaos Bolt and Firebolt wait their travel, the Imp from the 5 yd pet floor.
- Goldens (Average-Default): Destruction -0.536%: Conflagrate -0.31 (the Immolate snapshot's SP misses
  Phylactery and Dislodged Foreign Object procs that land after it, -0.17; 13% for 13.3%, -0.14),
  Firebolt -0.20 (0.742 × SP before, 0.714 now), Chaos Bolt's wait -0.04. Its FullBuffs
  ShortSingleTarget -2.29%: a Chaos Bolt still in flight at the 60 s end (-1.57) and Conflagrate (-0.72).
  Affliction -0.007% (Shadow Bolt's max, Haunt's SP, Curse of Agony), Demonology -0.002% (Shadow Bolt).

**For wave J and PAR-P8:**
- A pet's or guardian's spells take the owner's spell mods where the class mask matches
  (`Unit::GetSpellModOwner`): Mirror Image, Firebolt. A pet spell's family can differ from its owner's
  (Waterbolt 5, Felguard's Cleave 4).
- A script that reads an effect's value (Conflagrate's 60) still declares the server's value, and the
  closure reads the declaration.
- PAR-P8: warlock percent mods already multiply (`spellModDamage`). Conflagrate's `DamageMultiplier`
  stands in for the consumed Immolate's done mods: the server's hit skips `SpellDamageBonusDone`, and its
  dot takes Conflagrate's own done mods and the target's taken mods twice. Mirror Image still lacks the
  mage's `SPELLMOD_DAMAGE` percents.

**Allowlist entries left** (PAR-DECL-1 added none):
- Mage 59637 and 59638 GCD, KeepSim: the GCD paces Mirror Image's AI, which the server scripts.
- Warlock 47964 GCD: the server's 1 s applies.
- Warlock 23720 cooldown, KeepSim: The Black Book's 5 min is item_template's, which Spell.dbc lacks.
- Warlock 1122 school, KeepSim: the sim deals the Infernal's landing under the summon's id.

#### PAR-DECL-2 as built

**Hunter:**
- Travel: every shot rolls at the cast and lands after `WaitTravelTime`: Arcane, Steady, Aimed, Multi-,
  Chimera, Kill and Explosive Shot, Serpent and Scorpid Sting, Black Arrow, Wild Quiver, Chimera's Serpent
  (53353, which travels again), and Auto Shot, whose 40 yd/s is set by hand (its action id has no server
  data). Hit procs (Glyph of Arcane Shot, Improved Steady Shot, Chimera's sting refresh) move to the landing.
  Pet missiles wait too: `newSpecialAbility` when `MissileSpeed` isn't 0 (Acid Spit), and Poison Spit. No
  wait: Volley (`Spell::_cast` handles channels at once) and Silencing Shot (speed 0).
- `WeaponPct`: Aimed, Multi- and Raptor Strike 1; Chimera 1.25 and Kill Shot 2, out of the closure; Silencing
  Shot 0.5 and Wild Quiver 0.8, out of `DamageMultiplier`. The normalized shots' 0.2 RAP is the weapon's AP
  part, inside the percent. Multi-Shot's bonus row (0.2 AP) goes unread: weapon damage takes no coefficient.
- `FromSpellID`: Volley's tick 58433; Chimera's Serpent takes Serpent Sting 49001's tick, times the script's
  40% a tick in `DamageMultiplier`.
- Mods: none reach a declared effect. Noxious Stings' `SPELLMOD_EFFECT2` hits Serpent Sting's damage-taken
  aura.
- KeepSim (`sim/hunter/serverdata_allowlist.go`): Kill Shot's 0.4 AP, which `Spell::EffectWeaponDmg` adds
  after the 200%; Explosive Shot's tick AP, 53352's.
- Fixed: Explosive Shot's tick AP 0.14 → 0.16 (53352's live row); Wolverine Bite 400 + 0.07 AP → 405 flat.
- Undeclared: Explosive Trap (49065 isn't in serverdata); Piercing Shots, a share of the crit.
- Goldens, Average rows: BM -0.01%, MM -0.11%, SV +3.33%, of which Explosive Shot's AP is +3.65% (measured
  without the waits). Without the waits only BM's multi-target rows also move (-0.01%): Multi-Shot rolls
  every target before dealing any. Character stats hold.

**Shaman:**
- Travel: Lightning Bolt (its overload too), Lava Burst and Searing Totem's bolt roll at the cast and land
  after `WaitTravelTime`, with Lightning Overload and the T8/T9 4pc dots at the landing. Charges go at the
  launch, where `spell_proc` drops them in the cast phase: Maelstrom Weapon and Elemental Mastery on
  `OnCastComplete`, not on the hit. Clearcasting has 2 charges, not the 3-stack stand-in, taken only from
  a cast it was up for as it began (`PROC_ATTR_REQ_SPELLMOD`), and never from the cast whose own instant
  hit refreshed it.
- Mods: Shamanism, Glyph of Lava, Tidal Waves (op 24); Thunderfall Totem, Totem of the Dancing Flame,
  Improved Shields and Improved Earth Shield on Earth Shield (op 3). The Lightning Bolt/Chain Lightning
  relics and the Totems of the Third Wind add spell power (class scripts), so the coefficient scales them,
  op 24 included.
- `WeaponPct` 1: Stormstrike's hits, whose off-hand flat bonus is halved and truncated, and Lava Lash.
- `FromSpellID`: Searing and Magma Totem's ticks (58702, 58735); Earthliving's HoT (52000).
- Also declared: Earth Shield, Healing Stream Totem's heal (52042), Flametongue Attack. The totem summons
  58704/58734 declare Physical; their entries and the missile-speed ones are gone.
- KeepSim (`sim/shaman/serverdata_allowlist.go`): Fire Blast 13339's 0.2 (57984's row); Flametongue
  Attack's script values, its downranked hit tagged 1 for its own entries; Healing Stream's 25 (58761's).
- Fixed: Healing Wave 1624–1852 + 0.807 → 3034–3466 + 1.611, Tidal Waves 4 a rank; Earth Shield 377 +
  0.286 → 337 + 0.5371; Earthliving 280 + 0.171 → 163 + 0.164; Earth and Frost Shock 0.386 → 0.3858;
  downranked Flametongue 64 → 60 a second, both ranks 0.0385 → 0.03811 SP, clipped and truncated; Furious
  Totem of the Third Wind 338 → 320; Healing Wave no longer takes Glyph of Lesser Healing Wave's 20%. The
  fire elemental: Fire Blast 714–844 + 0.429 → 110–130 + 0.2, Fire Nova 955–1098 + 1.0 → 148–170 + 0.5,
  Fire Shield 11350 → 13377/13376 at 95 + 0.015 a tick.
- Undeclared: none. Left out as shares of other damage or healing: Ancestral Awakening, Electrified, Lava
  Burst's T9 dot. Windfury keeps its 58804 entry.
- Goldens, Average rows: Elemental -1.55%, Enhancement -2.86%, Restoration holds. By cause, per-row
  medians: travel waits Elemental -2.14% (to -8.6% on NoBuffs rows, as Clearcasting's charges now land a
  cast late), Enhancement -0.16%; the relics through Shamanism Elemental +0.54%; the fire elemental's
  numbers Enhancement -2.50% (its FT rows, to -10.8%), Elemental's 24 fire elemental rows -2.7% to -31.9%;
  Flametongue Enhancement -0.31%. Elemental's weights: Intellect 0.37 → 3.03, spell crit 1.15 → 1.45.
  Character stats hold.

**Druid:**
- Travel: Wrath already waited. Typhoon's cast 61384 declares 27 yd/s but doesn't wait: `Spell::EffectTriggerSpell`
  casts 53227 at launch, so the damage rolls at the cast and lands on every target after 53227's own 30 yd/s.
  No other druid spell has a speed.
- Mods: Wrath of Cenarius (op 24, 2 a rank on Wrath, 4 on Starfire); Savage Fury on both Mangles (op 23, 10% a
  rank of the weapon percent, flat included: 240% and 138%); the Maul, Swipe (Bear) and Shred idols (op 3) and Idol
  of Ursoc on Lacerate (op 8); Glyph of Hurricane's -20 (op 3), which its class mask puts on Hurricane's tick 48466
  too.
- `WeaponPct`, out of `DamageMultiplier`: Mangle (Bear) 1.15, Mangle (Cat) 2 and Shred 2.25, each scaling the flat
  roll too; Maul 1; Swipe (Cat) 2.5.
- Also declared: Moonfire's hit (effect 1) and tick, Insect Swarm's tick, Hurricane's tick 48466 (covering 48467),
  both Starfall spells, Typhoon's 53227 (covering 61384), Faerie Fire (Feral) 60089, Lacerate, Rake, Swipe (Bear),
  Rip's tick and Ferocious Bite's roll. Their combo point parts (93 and 290 a point), Ferocious Bite's energy and AP
  shares (`Spell::EffectSchoolDMG`) and Rip's AP share and idol (`spell_dru_rip`) stay in the closures.
- Entries: no KeepSim; the 61384 missile speed entry is gone. Only Rebirth 48477's cast time stays: a Moonkin's
  Rebirth folds in the GCD of shifting back.
- Fixed: Improved Insect Swarm raises Wrath's roll, before spell power, not the whole hit; the Starfire idols add
  spell power (override class script 5148) that the op-24-modded coefficient scales; Idol of the Beast's +14 a combo
  point on Ferocious Bite is gone (32410 has no handler). Glyph of Hurricane, which the sim ignored, also gives Insect
  Swarm +30%.
- Undeclared: none. Restoration registers no heal; treants only swing.
- Goldens, Average rows: Balance -0.26%, BalancePhase3 -0.51%; Feral, FeralApl, Feral Tank and Restoration hold.
  By cause, per-row medians: Improved Insect Swarm Balance -0.48%, BalancePhase3 -0.50%; the Starfire idol +0.24% on
  Balance's 126 Idol of the Shooting Star rows. With both reverted, no druid suite moves. Character stats hold.
- For wave J and P8: the `SPELLMOD_DAMAGE`/`DOT` percents stay in `DamageMultiplier`. On the server an op-0 flat
  (Wrath's idols, +25 and +70) goes on after those percents, and a script's addition after `SpellDamageBonusDone`
  skips the caster's done percents (Rip's AP share and idol, Crying Wind's 374 over Insect Swarm's ticks); the sim
  multiplies both. A class mask can reach an effect the tooltip doesn't name (Glyph of Hurricane), and a talent's
  points can be a percent (Savage Fury's op 23 is aura 108), so check effect mods against Spell.dbc. PAR-P7-TANK:
  Feral Tank's Maul, Swipe (Bear), Mangle (Bear), Lacerate and Faerie Fire (Feral) are declared.

#### PAR-DECL-3 as built

**Death knight:**
- Declared every spell the undeclared list named, plus Death and Decay (a script behind a dummy effect),
  Scourge Strike's shadow part (55271's effect 2, the percent `spell_dk_scourge_strike` reads) and Mind
  Freeze's Frost school. Wandering Plague (family 0, a script's base points) isn't.
- Mods: the sigils (flat `SpellModEffect1`), Glyph of Death and Decay (`SpellModEffect1` +20%) and
  Scourgelord's Plate 2pc (`SpellModAllEffects` +20%).
- `WeaponPct` left `DamageMultiplier` in every strike, the rune weapon's and Claw included.
  `normalizedStrikeBase` (`sim/deathknight/weapon_strikes.go`) halves an off hand's fixed bonus.
- Off-hand strikes but Obliterate's keep hand-written numbers: serverdata lacks 66953, 66962, 66979, 66992
  and 66217.
- KeepSim AP entries, for AP from another spell's `spell_bonus_data` or a script: Death Coil 47632 (declared
  from 49895), Death and Decay's tick (52212's row), Rune Strike (`Spell::EffectWeaponDmg`).
- Travel: Unholy Blight, after its queue delay; Gargoyle Strike and both Death Coils, worked out at launch.
- What the declarations exposed is in the INVESTIGATION (**Effect declarations (PAR-DECL-3)**).
- Goldens, Average-Default: Frost +1.86%, Frost UH +1.52%, Unholy +0.91%, Blood +0.06%, Blood Tank -2.66%.

**Warrior:**
- Declared every spell the undeclared list named, plus the script damage behind dummy effects: Execute,
  Concussion Blow, Shockwave, Damage Shield. Not Deep Wounds or Sweeping Strikes: their amounts are all
  script, and serverdata lacks Deep Wounds' 12721.
- Bloodthirst, Concussion Blow, Shockwave and Damage Shield declare the effect their script reads as a
  percent (of attack power, or block value), so it drives the damage and needs no entry. Execute's 20% AP
  is a KeepSim entry naming `spell_warr_execute`; its 3.8 per tenth of rage stays in the closure.
- Mods: Improved Rend (`SpellModEffect1`), Improved Cleave (`SpellModAllEffects`), Gag Order
  (`SpellModEffect2`, out of `DamageMultiplier`).
- Devastate's 1.2 `WeaponPct` skips its flat, which counts this Devastate's own Sunder stacks.
- Off hand: Whirlwind's and Bladestorm's declare 44949 (flat 0); `normalizedStrike`
  (`sim/warrior/weapon_strikes.go`) halves an off-hand fixed bonus.
- Travel: Heroic Throw and Shattering Throw, rolled at launch; Shattering's armor debuff lands with them.
- Damage Shield registers as its rank's spell, 58872 or 58874.
- Goldens, Average-Default: Fury +0.06%, Arms +0.01%, Protection +0.09%. Fury and Arms: the declarations
  alone -0.003% to -0.03%, then the throws' travel reshuffles the RNG (-0.7% to +1.2% a test).
  Protection: Shield Slam's block cap and Gag Order (Shield Slam +0.3%; high block value sets up to +4% a
  test), and Devastate's flat without a raid Sunder (NoBuffs +1.4%).

**Rogue:**
- Declared every spell the undeclared list named. Not Killing Spree (57841/57842), Shiv's hit (5940) or
  Fan of Knives' off hand (52874): serverdata lacks them. That off hand declares on 51723 tag 2, whose
  values 52874 shares.
- Mods: Sinister Calling (`SpellModEffect2` on Backstab's and Hemorrhage's weapon percent) and Glyph of
  Ghostly Strike (`SpellModEffect1`), both out of `DamageMultiplier`.
- `WeaponPct` left `DamageMultiplier` in Backstab, Ambush, Hemorrhage, Ghostly Strike and Fan of Knives; the
  last three take the server's dagger 1.5 (`daggerPct`, `sim/rogue/weapon_strikes.go`) on top.
- Off hand: `normalizedStrike` halves Mutilate's 181.
- Combo parts stay in the closures, none with an effect mod: Eviscerate's 370 and 7% AP a point, Envenom's
  9% AP a point, Rupture's 18 and AP table. Rupture's AP part skips the caster's damage modifiers.
- No entries: the Fan of Knives missile speed entry went, and with it the rogue allowlist file.
- Travel: Fan of Knives, both hands rolled at launch.
- Goldens, Average-Default: Assassination +1.63%, Combat -0.73%, Subtlety -0.66%. Poisons +7 to +8% (AP
  coefficients), Rupture -13 to -25%, Mutilate's off hand -13%; `fan_aoe` up to +4.6% (poisons on every
  target). No suite's APL casts Backstab, Ambush, Hemorrhage, Ghostly Strike or Garrote.

**Paladin:**
- Declared every spell the undeclared list named, plus Holy Shield's proc (48952 tag 1, effect 1). Not the
  Seal of Righteousness proc: serverdata lacks 25742, so it keeps 20154's id. The sim has no Holy Shock or
  Holy spec heal (the Holy suite casts nothing); Ardent Defender heals as 66235, which serverdata lacks.
- Values fixed: Exorcism 1033–1151, Holy Wrath 1058–1242, Holy Shield 0.09 SP and 0.056 AP, Judgement of
  Command 24% weapon (was 19%).
- Mods: Libram of Radiance is `SpellModEffect1` +105 on Crusader Strike, so the 75% scales it. The 31033
  libram check went: that id is a necklace.
- `WeaponPct` left `DamageMultiplier` in Crusader Strike, Divine Storm (effect 2) and Seal of Command.
- Script math in whole points: Judgement of Command's 8% AP and 13% SP after the percent, Hammer of the
  Righteous's dps, Shield of Righteousness's block cap (T8 4pc included), the Seal of Righteousness proc,
  Righteous Vengeance.
- Travel: Avenger's Shield, Hammer of the Righteous, Hammer of Wrath and Holy Wrath roll every target at
  launch.
- Goldens, Average-Default: Retribution -0.05% (Hammer of Wrath's last missile, Exorcism +0.2%,
  Righteous Vengeance -0.2%), Protection -1.09% (Holy Shield -15% where the boss attacks, Shield of
  Righteousness -0.5% from the cap), Holy unchanged. Retribution SOC single target +0.9 to +1.3%
  (Judgement of Command +9%).

**For wave J and PAR-P8** (details in the INVESTIGATION):
- The undeclared list misses script damage behind a dummy (Execute, Death and Decay, Holy Shock) and
  `SPELL_AURA_PROC_TRIGGER_DAMAGE`/`DAMAGE_SHIELD` auras (Holy Shield, Damage Shield): declare them anyway.
- Custom base points (`CastCustomSpell`) deal exactly the handed value: declare the effect, add no roll.
- A script reading an effect's percent: declare that effect, use `Roll()/100`, no entry. A script
  constant: declare it with a KeepSim entry naming the script.
- Missiles roll damage and crit at launch; AoE and chain targets all fly from the caster. Waits use
  `DistanceFromTarget`, 30 yd in the suites even for melee-range Fan of Knives and Hammer of the Righteous.
- PAR-P7-TANK, open: Holy Shield's proc never misses or crits on the server; Ardent Defender's heal amount;
  Concussion Blow gets no caster modifiers; the rune weapon's spell bonus runs as its owner.
- PAR-P8: `SpellDamageBonusDone` truncates its SP and AP parts separately, which most closures don't.
  `SPELLMOD_DAMAGE` flats come after every percent; Divine Storm's librams still sit inside its 110%.
  Rupture's AP part and Concussion Blow skip caster modifiers. Shadowstep is `SPELLMOD_DAMAGE` on the
  builders but an effect mod on Garrote's and Rupture's base.
- Serverdata lacks, for the next regenerating item: 66953, 66962, 66979, 66992, 66217, 52212 (DK); 20647,
  12721, 58872, 59653 (warrior); 57841, 57842, 5940, 52874 (rogue); 25742, 66235 (paladin).

**Effect entries left**, all KeepSim:
- Death knight: Death Coil 47632 AP 0.15 (47632's own row); Death and Decay 49938 tick AP 0.04805 (52212's
  row); Rune Strike 56815 AP 0.15 (`Spell::EffectWeaponDmg`); Scourge Strike 55271 tag 2 Shadow (dealt as
  70890); Empower Rune Weapon 47568's missile speed (a self cast never travels).
- Warrior: Execute 47471 AP 0.2 (`spell_warr_execute`).
- Paladin: Judgement of Command 20467 SP 0.13 and AP 0.08 (`Spell::EffectWeaponDmg`).
- Rogue: none.

The P3-2 timing entries stand: warrior Recklessness and Death Wish GCD, Sweeping Strikes 12723's cooldown,
Bladestorm's channel; paladin 498/31884's shared cooldown.

### PAR-DECL-4 (I6)

Adds to `serverdata` the ids the sweeps lacked, declares them, and drops the KeepSim entries standing in:
- Mage: Deep Freeze's 71757 (44572's numbers are hand-written).
- Hunter: Explosive Trap 49065; Explosive Shot's 53352 (60052/60053's tick AP entries).
- Death knight: the off-hand strikes 66953, 66962, 66979, 66992, 66217; Death and Decay's tick 52212.
- Warrior: Execute's 20647, Deep Wounds' 12721, Damage Shield's 58872 and 59653.
- Rogue: Killing Spree's 57841/57842, Shiv's hit 5940, Fan of Knives' off hand 52874.
- Paladin: the Seal of Righteousness proc 25742 (dealt as 20154), Ardent Defender's heal 66235.
- Any other KeepSim entry whose `Why` names another spell's row, e.g. the shaman's 57984 and 58761.

The committed capture holds them all, so none needs a live one: name each in its declaration (`FromSpellID`,
keeping the sim's spell ids), then regenerate `serverdata`, which adds only their rows. Fix or allowlist every
mismatch; entries naming a script stay. Runs all 37 suites, since the tables are shared.

### Wave J

- **PAR-P7-PRI** declares the priest's effects. Holy Fire's dot and Devouring Plague are its known
  mismatches.
- **PAR-P7-TANK** builds on the tank-spec spells of PAR-DECL-2 (Feral Tank) and PAR-DECL-3.

## Not scheduled

- Shared damage spells in `sim/common` and `sim/core`: item procs (give `ProcDamageEffect` its proc spell
  id), explosives, racials, and Shadowmourne 71904's travel wait.
- Checking the op-24 mod values that talents declare against the server's spell mods. `indexSpellMods`
  (`tools/acore/gen_serverdata/model.go`) indexes them but builds bounds only for ops 10, 11 and 21.

## Verification

- `dock.sh test` is green, `TestServerDataConflicts` included (with the undeclared lists).
- Goldens are promoted suite by suite, with deltas reviewed.
- The simval replay stays all-PASS.
- `BenchmarkSimulate` is recorded, and BiS re-baselines after I4 and after I5.
