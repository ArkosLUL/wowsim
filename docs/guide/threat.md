# Threat

How the sim computes threat, matching the server (`sim/core/threat.go`; `ThreatManager::CalculateModifiedThreat`,
`Spell::HandleThreatSpells`). Probes and replays: the `tools/simval` README's Threat probe.

## Hits and heals

- A hit's threat is its damage × `spell_threat` pctMod × `Spell.ThreatMultiplier` (`SPELLMOD_THREAT`) × the school
  multiplier: `PseudoStats.ThreatMultiplier` times each `AddThreatSchoolMultiplier` aura covering the spell, once
  per aura. `ThreatMultiplier` 0 means no threat at all, bonus included, as does `ATTR1_NO_THREAT` (Healing Stream
  Totem). A spell with no school takes no school multiplier (`ThreatSchoolMultiplier` returns 1: no `MOD_THREAT`
  aura matches the empty mask); only `CastThreat` and `HealThreat` pass one, as `RegisterSpell` refuses threat
  without a school.
- A heal's threat is half the health it restores (a quarter for a paladin), modified like a hit and shared as
  assist threat; overhealing makes none. Self heals dealt outside a spell count too (`HealThreat(metrics,
  spellID)`): Death Strike, (Improved) Blood Presence, Rune Tap, Mark of Blood's 61607 on its knight,
  Frenzied Regeneration, Lifeward, potions, healthstone, Lifeblood. Every heal is whole, as the server's.
  Judgement of Light's heal makes none for the healed ([class mechanics](class-mechanics.md#paladin)), nor
  does Mark of Blood's on another unit. Not modelled: the server's assist threat to the knight for the latter
  (`ForwardThreatForAssistingMe`).
- `SPELL_EFFECT_THREAT` adds its flat amount, unmodified, on each enemy hit; a cut never takes threat below 0
  (Feint).

## Energize threat

An energize gives the caster's enemies half its amount, unmodified, as assist threat (rage and runic power in
tenths): `EnergizeBySpell` the amount asked for (Lay on Hands' mana too), the cap's cut included, a periodic
energize or `OBS_MOD_POWER` tick the gain, both only for the unit's current power; a potion
(`SPELLFAMILY_POTION`) for any power.
- Replenishment (57669) is a periodic energize: each tick gives its caster half the mana the target really
  gained (`Unit.AddEnergizeTickThreat`); the raid's buff has no caster and makes none.
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
  chain (Heart Strike).
- `CastThreat` (`Unit.NewCastThreat(id).Add`) books the bonus of a positive spell the sim puts on without a
  cast: Bloodrage's 29131 (10), Lifeblood's 55503 (55), Heartpierce's Invigoration (80), Replenishment (1 a
  cast that takes a target), and 56817 (67, no school), which a death knight's dodge or parry procs to enable
  Rune Strike.
- A positive spell's bonus, self buffs' included, is modified and shared as assist threat, evenly over the
  creatures fighting its target, none to one under crowd control (`ThreatManager::ForwardThreatForAssistingMe`).

## Data

- The rows are `sim/core/serverdata/threat_auto_gen.go` (gen_serverdata), so class code carries no flats or
  percents and `FlatThreatBonus` panics.
- `SpellConfig.ThreatSpellID` names the server spell a sim spell stands for (Sunder Armor reads 7386, 345 + 5%
  AP, the rank warriors know, and adds 58567's 10 on landing; its ActionID stays 47467). `NoThreatBonus` marks
  damage the server deals through an aura (damage shields, proc trigger damage), `Spell.LandThreatBonus` counts an
  enemy a cast selects without a hit (Consecration).
- Item damage dealt under its server spell takes that spell's row: the explosives' 56350, 67769 and 56488,
  whose sapper row counts the caster.

## Multipliers

- Dire Bear Form is ×1.45 × 1.43; Blood and Unholy Presence −20% with Subversion's flat added to that same effect
  (−45% at 3 points); Shadowform ×0.7 in every school; Fanaticism −10% a point, kept under Righteous Fury.
- Arcane Subtlety, Burning Soul, Frost Channeling, Righteous Fury (holy ×1.8) and Elemental Precision (fire, frost,
  nature) are school auras, so Frostfire Bolt gets two. So is Silent Resolve (holy and arcane, 7/14/20%); its
  `SPELLMOD_THREAT` covers only Mana Burn.
- Thunderfury is one cast of 21992; its slow (27648) is `ATTR1_NO_THREAT`.
