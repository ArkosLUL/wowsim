package core

import (
	"fmt"
	"slices"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The server's threat arithmetic: ThreatManager::CalculateModifiedThreat on each hit's damage or healing,
// plus the bonus Spell::HandleThreatSpells adds once per cast. A spell whose ThreatMultiplier is 0 makes
// no threat at all, bonus included.

// Attribute bits the bonus reads (SharedDefines.h, SpellInfo.h).
const (
	attr1NoThreat            = 0x00000400
	attr3SuppressTargetProcs = 0x00020000
	attrCuNoInitialThreat    = 0x00000010
	attrCuNegativeEffects    = 0x00007000
)

// spellThreat is what the spell's server data says about its threat, set at registration, and the
// enemies its open casts have hit so far.
type spellThreat struct {
	pct float64 // spell_threat's pctMod

	noBonus     bool
	flat, ap    float64 // the bonus: flatMod and apPctMod, else the spell's level
	assist      bool    // a positive spell's bonus is assist threat
	casterShare bool    // the caster is one of the units the bonus is divided between
	split       float64 // those units for a hit outside a cast, from the server data alone

	openCasts int
	hits      []threatHit
}

type threatHit struct {
	target *Unit
	landed bool
}

func (spell *Spell) setThreatFromServer(threatSpellID int32, noBonus bool) {
	spell.threat = spellThreat{pct: 1, noBonus: true, split: 1}
	s := spell.serverSpell
	if threatSpellID != 0 {
		if s = serverdata.SpellByID(threatSpellID); s == nil {
			panic(fmt.Sprintf("ThreatSpellID %d of spell %s isn't in the server data", threatSpellID, spell.ActionID))
		}
	}
	if s == nil {
		return
	}
	t := serverdata.ThreatBySpellID(s.ID)
	spell.threat.pct = float64(t.Pct())
	if noBonus || spell.Unit.Type == EnemyUnit || spell.Unit.Env == nil ||
		s.Attributes[1]&attr1NoThreat != 0 || s.Attributes[3]&attr3SuppressTargetProcs != 0 {
		return
	}

	switch {
	case t != nil && t.Row != 0:
		spell.threat.flat, spell.threat.ap = float64(t.FlatMod), float64(t.APPctMod)
	case s.AttributesCu&attrCuNoInitialThreat == 0:
		spell.threat.flat = float64(s.SpellLevel)
	}
	spell.threat.noBonus = spell.threat.flat == 0 && spell.threat.ap == 0
	// the NEGATIVE_EFFn bits are _IsPositiveSpell's answer per effect; unlike IsPositive they ignore
	// spell_custom_attr's POSITIVE_EFFn (Envenom)
	spell.threat.assist = s.AttributesCu&attrCuNegativeEffects == 0
	spell.threat.casterShare = t != nil && t.Targets&serverdata.ThreatTargetsCaster != 0
	spell.threat.split = float64(threatSplit(s, t, spell.Unit.Env.GetNumTargets()))
}

// threatSplit is how many units a cast selects with every enemy in range, by the server data: the enemies
// its area or chain effects reach, else one, plus the caster when an effect lands on it. Spell mods on
// the jump targets aren't in it.
func threatSplit(s *serverdata.Spell, t *serverdata.Threat, numTargets int32) int32 {
	var targets serverdata.ThreatTargets
	if t != nil {
		targets = t.Targets
	}
	enemies := int32(1)
	if targets&serverdata.ThreatTargetsEnemyArea != 0 {
		enemies = numTargets
		if s.MaxTargets > 0 {
			enemies = min(enemies, s.MaxTargets)
		}
	}
	for _, e := range s.Effects {
		if e.ChainTargets > 1 {
			enemies = max(enemies, min(numTargets, e.ChainTargets))
		}
	}
	enemies = max(enemies, 1)
	if targets&serverdata.ThreatTargetsCaster != 0 {
		enemies++
	}
	return enemies
}

// ThreatFromDamage is CalculateModifiedThreat on a hit's damage or healing, 0 unless it landed. The
// cast's bonus isn't in it.
func (spell *Spell) ThreatFromDamage(outcome HitOutcome, damage float64) float64 {
	if !outcome.Matches(OutcomeLanded) {
		return 0
	}
	return spell.modifiedThreat(damage)
}

// modifiedThreat is spell_threat's pctMod, then the spell's threat mods, then the unit's school auras.
func (spell *Spell) modifiedThreat(amount float64) float64 {
	return amount * spell.threat.pct * spell.ThreatMultiplier * spell.Unit.ThreatSchoolMultiplier(spell.SpellSchool)
}

// main hand attack power without the creature type bonus (Unit::GetTotalAttackPowerValue)
func (spell *Spell) threatBonusBase() float64 {
	return spell.threat.flat + spell.threat.ap*spell.Unit.stats[stats.AttackPower]
}

func (spell *Spell) hasEnemyThreatBonus() bool {
	return !spell.threat.noBonus && !spell.threat.assist && spell.ThreatMultiplier != 0
}

// threatBonus is the bonus a hit that isn't a periodic tick adds to its own threat. A hit inside a cast
// adds nothing yet: the cast hands out its bonus at the end, once it knows how many enemies it reached.
func (spell *Spell) threatBonus(result *SpellResult) float64 {
	if !spell.hasEnemyThreatBonus() || result.Target.Type != EnemyUnit {
		return 0
	}
	if spell.threat.openCasts > 0 {
		spell.threat.hits = append(spell.threat.hits, threatHit{result.Target, result.Landed()})
		return 0
	}
	if !result.Landed() {
		return 0
	}
	return spell.threatBonusBase() / spell.threat.split
}

// LandThreatBonus counts a cast landing on an enemy without a hit of its own, like the enemies a
// persistent area aura selects as it's cast, so the enemy gets a share of the cast's bonus.
func (spell *Spell) LandThreatBonus(sim *Simulation, target *Unit) {
	if !spell.hasEnemyThreatBonus() || target.Type != EnemyUnit {
		return
	}
	if spell.threat.openCasts > 0 {
		spell.threat.hits = append(spell.threat.hits, threatHit{target, true})
	} else if sim.CurrentTime >= 0 {
		spell.SpellMetrics[target.UnitIndex].TotalThreat += spell.threatBonusBase() / spell.threat.split
	}
}

// openThreatCast starts collecting the cast's hits, and adds a positive spell's bonus.
func (spell *Spell) openThreatCast(sim *Simulation) int {
	spell.addAssistThreat(sim)
	spell.threat.openCasts++
	return len(spell.threat.hits)
}

// closeThreatCast splits the bonus between the enemies the cast reached and the caster if an effect lands
// on it. An enemy it missed every time gets no share, but still counts.
func (spell *Spell) closeThreatCast(sim *Simulation, start int) {
	t := &spell.threat
	t.openCasts--
	hits := t.hits[start:]
	if len(hits) == 0 {
		return
	}
	defer func() { t.hits = t.hits[:start] }()
	if sim.CurrentTime < 0 {
		return
	}

	units := 0
	if t.casterShare {
		units++
	}
	for i, hit := range hits {
		if !slices.ContainsFunc(hits[:i], func(h threatHit) bool { return h.target == hit.target }) {
			units++
		}
	}
	share := spell.threatBonusBase() / float64(units)
	for i, hit := range hits {
		first := !slices.ContainsFunc(hits[:i], func(h threatHit) bool { return h.target == hit.target })
		landed := slices.ContainsFunc(hits[i:], func(h threatHit) bool { return h.target == hit.target && h.landed })
		if first && landed {
			spell.SpellMetrics[hit.target.UnitIndex].TotalThreat += share
			if sim.Log != nil {
				spell.Unit.Log(sim, "%s %s threat bonus %0.3f, 1 of %d units", hit.target.LogLabel(), spell.ActionID, share, units)
			}
		}
	}
}

// addAssistThreat hands a positive spell's bonus, modified, to the enemies in even shares
// (ThreatManager::ForwardThreatForAssistingMe). Nothing's fighting the unit before the pull.
func (spell *Spell) addAssistThreat(sim *Simulation) {
	t := &spell.threat
	if t.noBonus || !t.assist || spell.ThreatMultiplier == 0 || sim.CurrentTime < 0 {
		return
	}
	enemies := sim.Encounter.TargetUnits
	threat := spell.modifiedThreat(spell.threatBonusBase()) / float64(len(enemies))
	for _, enemy := range enemies {
		spell.SpellMetrics[enemy.UnitIndex].TotalThreat += threat
	}
	if sim.Log != nil {
		spell.Unit.Log(sim, "%s assist threat %0.3f on each of %d enemies", spell.ActionID, threat, len(enemies))
	}
}

// threatSchoolMod is a SPELL_AURA_MOD_THREAT aura on some schools only.
type threatSchoolMod struct {
	school     SpellSchool
	multiplier float64
}

// AddThreatSchoolMultiplier adds a SPELL_AURA_MOD_THREAT aura covering some schools. It multiplies a
// spell's threat once if the spell has any of them (Unit::GetTotalAuraMultiplierByMiscMask), so a mixed
// school spell isn't hit twice. An aura on every school goes on PseudoStats.ThreatMultiplier instead.
func (unit *Unit) AddThreatSchoolMultiplier(school SpellSchool, multiplier float64) {
	unit.threatSchoolMods = append(unit.threatSchoolMods, threatSchoolMod{school, multiplier})
}

// RemoveThreatSchoolMultiplier takes off an aura AddThreatSchoolMultiplier added.
func (unit *Unit) RemoveThreatSchoolMultiplier(school SpellSchool, multiplier float64) {
	i := slices.Index(unit.threatSchoolMods, threatSchoolMod{school, multiplier})
	if i < 0 {
		panic(fmt.Sprintf("%s has no school threat aura %v x%v to remove", unit.Label, school, multiplier))
	}
	unit.threatSchoolMods = slices.Delete(unit.threatSchoolMods, i, i+1)
}

// ThreatSchoolMultiplier is CalculateModifiedThreat's school part for a spell of these schools.
func (unit *Unit) ThreatSchoolMultiplier(school SpellSchool) float64 {
	m := unit.PseudoStats.ThreatMultiplier
	for _, mod := range unit.threatSchoolMods {
		if mod.school&school != 0 {
			m *= mod.multiplier
		}
	}
	return m
}
