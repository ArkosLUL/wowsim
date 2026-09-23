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

### Class sweep (I5)

Three items:
- **PAR-DECL-1:** mage, warlock
- **PAR-DECL-2:** shaman, druid (Feral Tank included), hunter with pets
- **PAR-DECL-3:** warrior, rogue, paladin, DK, including the tank-spec spells in those folders

Each item:
- declares every damage and heal effect
- moves effect and op-24 mods onto the declarations
- splits `WeaponPct` out of `DamageMultiplier`
- for each mismatch, fixes the number or adds an allowlist entry
- empties its classes' undeclared list

### Wave J

- **PAR-P7-PRI** declares the priest's effects. Holy Fire's dot and Devouring Plague are its known
  mismatches.
- **PAR-P7-TANK** builds on the tank-spec spells of PAR-DECL-2 (Feral Tank) and PAR-DECL-3.

## Not scheduled

- Shared damage spells in `sim/common` and `sim/core`: item procs (give `ProcDamageEffect` its proc spell
  id), explosives, racials.
- Checking the op-24 mod values that talents declare against the server's spell mods. `indexSpellMods`
  (`tools/acore/gen_serverdata/model.go`) indexes them but builds bounds only for ops 10, 11 and 21.

## Verification

- `dock.sh test` is green, `TestServerDataConflicts` included (with the undeclared lists).
- Goldens are promoted suite by suite, with deltas reviewed.
- The simval replay stays all-PASS.
- `BenchmarkSimulate` is recorded, and BiS re-baselines after I4 and after I5.
