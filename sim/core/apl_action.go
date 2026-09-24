package core

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core/proto"
)

type APLAction struct {
	condition APLValue
	impl      APLActionImpl

	// set when condition only reads the sim, so IsReady can skip it while impl can't be ready:
	// the spell impl casts or channels, or impl itself when it's a sequence
	gatedSpell    *Spell
	gatedImpl     aplGatedImpl
	gatedSequence aplSequenceGate
}

// impl.IsReady minus what the gate already checked on gatedSpell
type aplGatedImpl interface {
	isReadyPastGate(sim *Simulation) bool
}

// a cheap subset of a sequence's !IsReady that reads no subaction's condition
type aplSequenceGate interface {
	blocked(sim *Simulation) bool
}

func (action *APLAction) Finalize(rot *APLRotation) {
	action.impl.Finalize(rot)
	for _, value := range action.GetAllAPLValues() {
		value.Finalize(rot)
	}
}

func (action *APLAction) IsReady(sim *Simulation) bool {
	// with logs on, take the long way so an ExtraCastCondition's log lines stay
	if sim.Log == nil {
		if spell := action.gatedSpell; spell != nil {
			// aplSpellBlocked by hand: it's too big to inline, castBlocked isn't
			if spell.castBlocked(sim) || aplCostBlocked(spell) {
				return false
			}
			return (action.condition == nil || action.condition.GetBool(sim)) && action.gatedImpl.isReadyPastGate(sim)
		} else if sequence := action.gatedSequence; sequence != nil && sequence.blocked(sim) {
			return false
		}
	}
	return (action.condition == nil || action.condition.GetBool(sim)) && action.impl.IsReady(sim)
}

// true when spell.CanCast would fail whatever its ExtraCastCondition says
func aplSpellBlocked(spell *Spell, sim *Simulation) bool {
	return spell.castBlocked(sim) || aplCostBlocked(spell)
}

// CanCast's energy check without its write to CurCast.Cost: nothing reads an energy spell's
// CurCast.Cost outside its own cast, which resets it first. Energy only: rage rarely blocks, so
// checking it here cost the rage specs more than it saved, and a mana check can start an OOM event.
func aplCostBlocked(spell *Spell) bool {
	if _, ok := spell.Cost.(*EnergyCost); ok {
		return !(spell.Unit.CurrentEnergy() >= spell.ApplyCostModifiers(spell.DefaultCast.Cost))
	}
	return false
}

// CanCast right after castBlocked and aplCostBlocked came back false, with nothing changed since
func aplCanCastPastGate(spell *Spell, sim *Simulation, target *Unit) bool {
	if spell.ExtraCastCondition != nil && !spell.ExtraCastCondition(sim, target) {
		return false
	}
	switch spell.Cost.(type) {
	case nil, *EnergyCost:
		return true
	}
	spell.CurCast.Cost = spell.DefaultCast.Cost
	return spell.Cost.MeetsRequirement(sim, spell)
}

func (action *APLAction) Execute(sim *Simulation) {
	action.impl.Execute(sim)
}

// Returns this Action, along with all inner Actions.
func (action *APLAction) GetAllActions() []*APLAction {
	actions := action.impl.GetInnerActions()
	actions = append(actions, action)
	return actions
}

// Returns all APLValues used by this action and all of its inner Actions.
func (action *APLAction) GetAllAPLValues() []APLValue {
	var values []APLValue
	for _, a := range action.GetAllActions() {
		unprocessed := a.impl.GetAPLValues()
		if a.condition != nil {
			unprocessed = append(unprocessed, a.condition)
		}

		for len(unprocessed) > 0 {
			next := unprocessed[len(unprocessed)-1]
			unprocessed = unprocessed[:len(unprocessed)-1]
			values = append(values, next)
			unprocessed = append(unprocessed, next.GetInnerValues()...)
		}
	}
	return FilterSlice(values, func(val APLValue) bool { return val != nil })
}

func (action *APLAction) GetAllSpells() []*Spell {
	var spells []*Spell
	for _, a := range action.GetAllActions() {
		if impl, ok := a.impl.(*APLActionCastSpell); ok {
			spells = append(spells, impl.spell)
		} else if impl, ok := a.impl.(*APLActionMultidot); ok {
			spells = append(spells, impl.spell)
		} else if impl, ok := a.impl.(*APLActionMultishield); ok {
			spells = append(spells, impl.spell)
		}
	}
	return spells
}

func (action *APLAction) String() string {
	if action.condition == nil {
		return fmt.Sprintf("ACTION = %s", action.impl)
	} else {
		return fmt.Sprintf("IF = %s\nACTION = %s", action.condition, action.impl)
	}
}

type APLActionImpl interface {
	// Returns all inner APL Actions.
	GetInnerActions() []*APLAction

	// Returns all APLValues used by this Action (but not by inner Actions).
	GetAPLValues() []APLValue

	// Performs optional post-processing.
	Finalize(*APLRotation)

	// Invoked before each sim iteration.
	Reset(*Simulation)

	// Whether this action is available to be used right now.
	IsReady(*Simulation) bool

	// Performs the action.
	Execute(*Simulation)

	// Called only while this action is controlling the rotation.
	GetNextAction(sim *Simulation) *APLAction

	// Pretty-print string for debugging.
	String() string
}

// Provides empty implementations for the action impl interface functions.
type defaultAPLActionImpl struct {
}

func (impl defaultAPLActionImpl) GetInnerActions() []*APLAction        { return nil }
func (impl defaultAPLActionImpl) GetAPLValues() []APLValue             { return nil }
func (impl defaultAPLActionImpl) Finalize(*APLRotation)                {}
func (impl defaultAPLActionImpl) Reset(*Simulation)                    {}
func (impl defaultAPLActionImpl) GetNextAction(*Simulation) *APLAction { return nil }

func (rot *APLRotation) newAPLAction(config *proto.APLAction) *APLAction {
	if config == nil {
		return nil
	}

	action := &APLAction{
		condition: rot.coerceTo(rot.newAPLValue(config.Condition), proto.APLValueType_ValueTypeBool),
		impl:      rot.newAPLActionImpl(config),
	}

	if action.impl == nil {
		return nil
	}
	if action.condition == nil || aplValueIsPure(action.condition) {
		switch impl := action.impl.(type) {
		case *APLActionCastSpell:
			action.gatedSpell = impl.spell
			action.gatedImpl = impl
		case *APLActionChannelSpell:
			action.gatedSpell = impl.spell
			action.gatedImpl = impl
		case *APLActionSequence:
			action.gatedSequence = impl
		case *APLActionStrictSequence:
			action.gatedSequence = impl
		}
	}
	return action
}

func (rot *APLRotation) newAPLActionImpl(config *proto.APLAction) APLActionImpl {
	if config == nil {
		return nil
	}

	customAction := rot.unit.Env.GetAgentFromUnit(rot.unit).NewAPLAction(rot, config)
	if customAction != nil {
		return customAction
	}

	switch config.Action.(type) {
	// Casting
	case *proto.APLAction_CastSpell:
		return rot.newActionCastSpell(config.GetCastSpell())
	case *proto.APLAction_ChannelSpell:
		return rot.newActionChannelSpell(config.GetChannelSpell())
	case *proto.APLAction_Multidot:
		return rot.newActionMultidot(config.GetMultidot())
	case *proto.APLAction_Multishield:
		return rot.newActionMultishield(config.GetMultishield())
	case *proto.APLAction_AutocastOtherCooldowns:
		return rot.newActionAutocastOtherCooldowns(config.GetAutocastOtherCooldowns())

	// Timing
	case *proto.APLAction_Wait:
		return rot.newActionWait(config.GetWait())
	case *proto.APLAction_WaitUntil:
		return rot.newActionWaitUntil(config.GetWaitUntil())
	case *proto.APLAction_Schedule:
		return rot.newActionSchedule(config.GetSchedule())

	// Sequences
	case *proto.APLAction_Sequence:
		return rot.newActionSequence(config.GetSequence())
	case *proto.APLAction_ResetSequence:
		return rot.newActionResetSequence(config.GetResetSequence())
	case *proto.APLAction_StrictSequence:
		return rot.newActionStrictSequence(config.GetStrictSequence())

	// Misc
	case *proto.APLAction_ChangeTarget:
		return rot.newActionChangeTarget(config.GetChangeTarget())
	case *proto.APLAction_ActivateAura:
		return rot.newActionActivateAura(config.GetActivateAura())
	case *proto.APLAction_CancelAura:
		return rot.newActionCancelAura(config.GetCancelAura())
	case *proto.APLAction_TriggerIcd:
		return rot.newActionTriggerICD(config.GetTriggerIcd())
	case *proto.APLAction_ItemSwap:
		return rot.newActionItemSwap(config.GetItemSwap())

	case *proto.APLAction_CustomRotation:
		return rot.newActionCustomRotation(config.GetCustomRotation())

	default:
		return nil
	}
}

// Default implementation of Agent.NewAPLAction so each spec doesn't need this boilerplate.
func (unit *Unit) NewAPLAction(rot *APLRotation, config *proto.APLAction) APLActionImpl {
	return nil
}
