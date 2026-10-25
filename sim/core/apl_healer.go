package core

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// mostHurtFriendly is the raid unit with the lowest health percent, a tie to the first in raid order. Without
// incoming damage nobody is hurt and this is the unit's current target.
func (unit *Unit) mostHurtFriendly() *Unit {
	if unit.Env.Raid.incoming == nil {
		return unit.CurrentTarget
	}
	var best *Unit
	bestPercent := 0.0
	for _, other := range unit.Env.Raid.AllUnits {
		if !other.HasHealthBar() || !other.IsEnabled() || other.CurrentHealth() <= 0 {
			continue
		}
		if percent := other.CurrentHealthPercent(); best == nil || percent < bestPercent {
			best, bestPercent = other, percent
		}
	}
	if best == nil {
		return unit.CurrentTarget
	}
	return best
}

type APLValueMissingHealth struct {
	DefaultAPLValueImpl
	unit UnitReference
}

func (rot *APLRotation) newValueMissingHealth(config *proto.APLValueMissingHealth) APLValue {
	unit := rot.GetSourceUnit(config.SourceUnit)
	if unit.Get() == nil {
		return nil
	}
	if !unit.Get().HasHealthBar() && config.SourceUnit.GetType() != proto.UnitReference_MostHurt {
		rot.ValidationWarning("%s does not use Health", unit.Get().Label)
		return nil
	}
	return &APLValueMissingHealth{
		unit: unit,
	}
}
func (value *APLValueMissingHealth) Type() proto.APLValueType {
	return proto.APLValueType_ValueTypeFloat
}
func (value *APLValueMissingHealth) GetFloat(sim *Simulation) float64 {
	unit := value.unit.Get()
	if !unit.HasHealthBar() {
		return 0
	}
	return max(0, unit.MaxHealth()-unit.CurrentHealth())
}
func (value *APLValueMissingHealth) String() string {
	return "Missing Health"
}

// APLActionMultihot casts a heal over time on the most hurt raid unit that is missing health and doesn't have
// the caster's hot, or has less than max overlap left of it.
type APLActionMultihot struct {
	defaultAPLActionImpl
	spell      *Spell
	maxOverlap APLValue

	nextTarget *Unit
}

func (rot *APLRotation) newActionMultihot(config *proto.APLActionMultihot) APLActionImpl {
	spell := rot.GetAPLMultidotSpell(config.SpellId)
	if spell == nil {
		return nil
	}

	maxOverlap := rot.coerceTo(rot.newAPLValue(config.MaxOverlap), proto.APLValueType_ValueTypeDuration)
	if maxOverlap == nil {
		maxOverlap = rot.newValueConst(&proto.APLValueConst{Val: "0ms"})
	}

	return &APLActionMultihot{
		spell:      spell,
		maxOverlap: maxOverlap,
	}
}
func (action *APLActionMultihot) GetAPLValues() []APLValue {
	return []APLValue{action.maxOverlap}
}
func (action *APLActionMultihot) Reset(*Simulation) {
	action.nextTarget = nil
}
func (action *APLActionMultihot) IsReady(sim *Simulation) bool {
	maxOverlap := action.maxOverlap.GetDuration(sim)

	var best *Unit
	bestPercent := 0.0
	for _, target := range sim.Raid.AllUnits {
		if !target.HasHealthBar() || !target.IsEnabled() || target.CurrentHealth() <= 0 || target.CurrentHealth() >= target.MaxHealth() {
			continue
		}
		percent := target.CurrentHealthPercent()
		if best != nil && percent >= bestPercent {
			continue
		}
		dot := action.spell.Dot(target)
		if dot.IsActive() && dot.RemainingDuration(sim) >= maxOverlap {
			continue
		}
		if action.spell.CanCast(sim, target) {
			best, bestPercent = target, percent
		}
	}
	action.nextTarget = best
	return best != nil
}
func (action *APLActionMultihot) Execute(sim *Simulation) {
	action.spell.Cast(sim, action.nextTarget)
}
func (action *APLActionMultihot) String() string {
	return fmt.Sprintf("Multihot(%s)", action.spell.ActionID)
}
