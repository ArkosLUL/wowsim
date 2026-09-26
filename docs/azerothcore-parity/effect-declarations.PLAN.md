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
