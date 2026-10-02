# E2e coverage: investigation

Which further live e2e tests would catch more sim bugs. Measured on `master` at `c27854a8d`, after test-enrichment
([INVESTIGATION](../test-enrichment/test-enrichment.INVESTIGATION.md), which this doesn't repeat) and wave Q
(PAR-FU-4..7), which fixed every record the fixture README dropped.

## Findings

### 1. What found bugs

26 sim bugs came from server comparisons:

| Source | Bugs |
|---|---|
| Recorded runs (the Ret capture alone: 7) | 9 |
| Golden characters | 8 |
| Talent sweep | 7 |
| Direct live reads | 2 |
| Formula probes (`simval.jsonl`), `TestDamageRecords` (346 checks) | 0: they confirmed fixes made from reading server code |

By class: talent value or reach 10; haste and timing 4; base stats and forms 3; buff or glyph 2; one each for spell-mod
stacking (spread to every class), weapon damage, rounding, proc rate, item data, talent data, a crash.

Whole-character comparisons and sweeps find bugs; single-formula probes confirm.

### 2. Never checked against the server

| Mechanic | Sim | Server |
|---|---|---|
| Cast time, GCD under haste | `sim/core/cast.go`, `gcd.go` | `SpellInfo::CalcCastTime`, `Spell::TriggerGlobalCooldown` |
| Cooldowns after mods | `sim/core/spell.go` `applyServerData` (offline bounds only) | `Player::AddSpellAndCategoryCooldowns` |
| Durations, tick counts (beyond the shadow dots, pfu4) | `sim/core/dot.go`, `aura.go` | `Aura::CalcMaxDuration`, `AuraEffect::CalculatePeriodic` |
| Power costs | `sim/core/spell.go`, `mana.go`, `energy.go`, `rage.go`, `runic_power.go` | `SpellInfo::CalcPowerCost`, `Spell::TakeRunePower` |
| Rage, energy, mana, rune regen | same | `Unit::RewardRage`, `Player::UpdateEnergyRegen`, `UpdateManaRegen`, `GetRuneBaseCooldown` |
| Threat | `sim/core/spell_result.go`, stances, forms, presences | `ThreatManager::CalculateModifiedThreat`, `Spell::HandleThreatSpells` |
| Pet and guardian stats, damage | `sim/core/pet.go`, class pets | `Guardian::UpdateStats`, scaling auras (`PetDefines.h`) |
| Weapon-strike amounts | `sim/*/weapon_strikes.go` etc. | `Spell::EffectWeaponDmg`; `.simval damage` marks them unprobed |
| Proc magnitudes and ICDs, set bonuses, gems, meta gems, enchants | `sim/common/wotlk/*` | golden characters wear no gems or enchants |
| Raid buff and debuff magnitudes, stacking | `sim/core/buffs.go`, `debuffs.go`, `exclusive_effect.go` | `spell_group_stack_rules` |
| Creature types | `sim/core/target.go` `MobType` | dummies are giant, humanoid, beast; golden targets are demons |
| Chain and AoE, execute range, haste stacking, talent ArP | | |

### 3. Leads

- **Threat, every tank.** `Spell::HandleThreatSpells` adds `spell_threat`'s flat and AP bonus with
  `ignoreModifiers=true`: stance, form, presence and Righteous Fury don't multiply it. The sim does
  (`ThreatFromDamage`: `(damage·ThreatMultiplier + FlatThreatBonus) · PseudoStats.ThreatMultiplier`). Live values
  differ too:

  | Spell | Server | Sim |
  |---|---|---|
  | Sunder Armor 7386 | 345 + 5% AP | 360 |
  | Cleave 47520 | 112 | 225 |
  | Maul 48480 | 422 | 424 |
  | Lacerate 48568 | 515, ×0.5 | 515.5 |
  | Thunder Clap 6343 | ×1.85 | none |

  A spell with no entry adds its `SpellLevel`; the bonus is split across targets.
  (`SELECT entry, flatMod, pctMod, apPctMod FROM acore_world.spell_threat`, 106 rows.)
- **Spell-mod talents.** 276 of 615 implemented talents carry spell mods (aura 107/108): damage 56, cost 39, cooldown 38,
  dot 35, crit chance 34, crit damage 17, cast time 16, duration 11, GCD 2. Cost, cooldown, cast time, duration and GCD
  mods are never checked live; damage and crit only on the ~130 probed spells. 149 talents are proc-only. Reproduce:
  `python tmp/coverage/spellmod_talents.py.txt` (gitignored; joins `implemented_talents.json` with
  `assets/db_inputs/acore/spelldump.jsonl`).
- **Captured, never compared.** `goldchar.jsonl` holds stats, health, max power, AP, RAP, armor, avoidance,
  resistances, ArP% and per-school spell power and crit; `checkGoldcharInfo` compares spell power, lowest-school spell
  crit, hit, swing speed, weapon damage, melee crit and expertise. `talents.jsonl` keeps per-school spell crit; the
  replay compares the lowest school, missing Holy Specialization and Critical Mass.
- **Harness.** It misparses `SMSG_SPELL_COOLDOWN` (skips no flags byte), never processes `SMSG_POWER_UPDATE` for
  players, and parses no combat-log packets (tests hook raw packets: `collectSpellHits`, `periodicTicks`).
- **Dummy.** It zeroes damage in `DamageTaken`, so `DealDamage` returns before adding threat: no damage threat in a
  real fight.
- **`.simval damage`** skips `ModifySpellDamageTaken` (mod-individual-progression's 0.3×/0.6×); harmless at `.ip set 18`.
- **rrsim** still needs the live DBCs; a golden setup doesn't, its character coming from the sim's own preset.

## Options

Ranked by expected bugs per cost.

**A. Offline, on captured fixtures**
- **A1.** Compare the full golden-character sheet (the fields above), reusing the talent sweep's sheet reader and
  sim-derived tolerances.
- **A2.** Talent sweep: compare per-school spell crit; add ArP% and miss chance taken to the trim, then rerun the
  affected paths (`SIMVAL_TALENT_ONLY`).
- **A3.** Sim exclusive-effect categories against `spell_group` and `spell_group_stack_rules` (SELECT export).
- **A4.** Sim chain multipliers, max targets and AoE cap (×10/targets past 10) against spelldump's `chainTarget`,
  `damageMultiplier`, `maxAffectedTargets`.

**B. New `.simval` probes (C++) and live runs**
- **B1.** `.simval timing <spellId>`: cast time (`CalcCastTime(caster, &probe)`, a stack `Spell` so percent mods
  count), GCD (`TriggerGlobalCooldown`'s arithmetic), `rec`/`catrec` (`AddSpellAndCategoryCooldowns`' arithmetic),
  duration, amplitude and ticks (`AddAura` on the dummy, then remove), cost (`CalcPowerCost`), rune cost and runic
  power gain.
- **B2.** Spell-mod sweep: learn each spell-mod talent on a golden setup and probe every spell its class mask hits
  (B1, `damage`, derived crit from `yellow`/`spell`) before and after. Covers the 276.
- **B3.** `.simval threat <spellId> [amount]`: the `spell_threat` row, `SPELLMOD_THREAT`, school multiplier, bonus as
  the server adds it; live via `AddThreat`/`GetThreat` on the dummy (fails under `.gm on`). A fix moves tank TPS
  goldens.
- **B4.** Weapon strikes in `.simval damage`: `EffectWeaponDmg` steps at both weapon-range ends; a `WeaponFamilyCode`
  marks the per-class branches shaped; melee state overrides join `StateBonuses`.
- **B5.** Pets and guardians: probe every unit in `m_Controlled`, recording scaling-aura amounts with the owner's
  inputs; the damage replay accepts pets. Leads: focus 24 every 4 s, Water Elemental base stats, Infernal buffs,
  Ghoul Frenzy cadence.
- **B6.** `.simval enchant`, `.simval gem` (reference: playerbots' `ApplyEnchantAndGemsNew`), so golden characters
  wear gems, socket bonuses, meta gems and enchants.
- **B7.** Regen fields in the `info` snapshot (`UNIT_FIELD_POWER_REGEN_*`, `PLAYER_RUNE_REGEN`), free for golden
  characters and the sweep.

**C. Live whole-character fights and sweeps**
- **C1.** Golden-character fights, 1–2 min each in Naxxramas under Chronicle: the bot runs a fixed proc-blind cycle;
  the sim replays its `SPELL_CAST_SUCCESS` times as a generated `waitUntil` + `castSpell` APL. Per ability: casts the
  sim makes on time, crit %, mean non-crit and crit, ticks per application and spacing, proc counts, aura uptime,
  energize totals. Extends the most productive source to all 32 specs as a committed regression test (backlog E6),
  without DBCs. Limits: dummy health never drops and it never attacks; swings have no hand flag; a refresh counts
  as an application.
- **C2.** Buff and debuff sweep: each FullBuffs buff on a golden setup (sheet); each FullDebuffs debuff on the dummy
  (`damage` taken bonus, crit, armor); pairs within each exclusive category.
- **C3.** Undead, demon, elemental, dragonkin and mechanical dummies via the module's SQL, for creature-type modifiers
  (backlog).

**Recommended order:** A1, A2 → B1 + B2 → B3 → C1 → B4, B5, B6 → C2 → C3, B7, A3, A4.

## Constraints

- The server lock; one live run at a time; live runs need the user's OK outside the wave loop.
- At most one C++ module item per wave; rebuilds only while Deathsong is offline.
- Every probe freezes the world thread: never interleave probes with an observed fight.
- The DB is SELECT-only: new dummies need module SQL and a rebuild.
- Run times: `run.sh` ~7 min, golden characters ~15 min, talent sweep ~25 min.
