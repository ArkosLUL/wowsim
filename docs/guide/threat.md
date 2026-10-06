# Threat

How the sim computes threat, matching the server (`sim/core/threat.go`; `ThreatManager::CalculateModifiedThreat`,
`Spell::HandleThreatSpells`). Probes and replays: the `tools/simval` README's Threat probe.

## Hits and heals

- A hit's threat is its damage × `spell_threat` pctMod × `Spell.ThreatMultiplier` (`SPELLMOD_THREAT`) × the school
  multiplier: `PseudoStats.ThreatMultiplier` times each `AddThreatSchoolMultiplier` aura covering the spell, once
  per aura. `ThreatMultiplier` 0 means no threat at all, bonus included, as does `ATTR1_NO_THREAT` (Healing Stream
  Totem).
- A heal's threat is half the health it restores (a quarter for a paladin), modified like a hit and shared as
  assist threat; overhealing makes none. Self heals through `GainHealth` count too (`HealThreat(metrics,
  spellID)`): Death Strike, (Improved) Blood Presence, Rune Tap, Frenzied Regeneration, Lifeward, potions,
  healthstone, Lifeblood. The sim doesn't truncate heals; the server does (`SpellHealingBonusDone`).
- `SPELL_EFFECT_THREAT` adds its flat amount, unmodified, on each enemy hit; a cut never takes threat below 0
  (Feint).

## Energize threat

An energize gives the caster's enemies half its amount, unmodified, as assist threat (rage and runic power in
tenths): `EnergizeBySpell` the amount asked for, the cap's cut included, a periodic energize or `OBS_MOD_POWER`
tick the gain, both only for the unit's current power; a potion (`SPELLFAMILY_POTION`) for any power.
- A `ResourceMetrics` built with `EnergizeThreat(kind)` books it at each gain (none before the pull);
  `AddEnergizeThreat` books one the sim gains inside another gain (Chill of the Grave, Dirge).
- None: `ATTR1_NO_THREAT` energizes (Life Tap, Soul Leech, Viper, Judgement of Wisdom), regen auras (Anger
  Management, Butchery's tick), rune spells' runic power, refunds, cost mods, `SetPower` (Sudden Death).

## Cast bonus

- The bonus is the row's flatMod + apPctMod × main hand AP, else the spell's level (none under
  `NO_INITIAL_THREAT` or `ATTR3_SUPPRESS_TARGET_PROCS`). Stance, form and presence multipliers never touch it.
- It's split over the distinct enemies the cast reached (misses count) plus the caster when an effect lands on
  it; ticks get none, nor does anything before the pull, but a spell an aura casts each tick adds its own
  (`ThreatTick`: Bladestorm, Volley, Penance's 52985 and 53000), and `SetThreatCastOf` splits one bonus over a
  chain (Heart Strike, the rune weapon's copy).
- A positive spell's bonus, self buffs' included, is modified and shared as assist threat, evenly over the
  creatures fighting its target, none to one under crowd control (`ThreatManager::ForwardThreatForAssistingMe`).

## Data

- The rows are `sim/core/serverdata/threat_auto_gen.go` (gen_serverdata), so class code carries no flats or
  percents and `FlatThreatBonus` panics.
- `SpellConfig.ThreatSpellID` names the server spell a sim spell stands for (Sunder Armor reads 7386, 345 + 5%
  AP, the rank warriors know, and adds 58567's 10 on landing; its ActionID stays 47467). `NoThreatBonus` marks
  damage the server deals through an aura (damage shields, proc trigger damage), `Spell.LandThreatBonus` counts an
  enemy a cast selects without a hit (Consecration).

## Multipliers

- Dire Bear Form is ×1.45 × 1.43; Blood and Unholy Presence −20% with Subversion's flat added to that same effect
  (−45% at 3 points); Shadowform ×0.7 in every school; Fanaticism −10% a point, kept under Righteous Fury.
- Arcane Subtlety, Burning Soul, Frost Channeling, Righteous Fury (holy ×1.8) and Elemental Precision (fire, frost,
  nature) are school auras, so Frostfire Bolt gets two.
- Thunderfury is one cast of 21992; its slow (27648) is `ATTR1_NO_THREAT`.
