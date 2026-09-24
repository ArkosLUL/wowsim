package core

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func TestAPLValueIsPure(t *testing.T) {
	aura := &APLValueAuraRemainingTime{}
	two := &APLValueConst{valType: proto.APLValueType_ValueTypeDuration, durationVal: 2 * time.Second}
	for _, tc := range []struct {
		name  string
		value APLValue
		want  bool
	}{
		{"compare in and", &APLValueAnd{vals: []APLValue{
			&APLValueCompare{op: proto.APLValueCompare_OpLe, lhs: aura, rhs: two},
			&APLValueNot{val: &APLValueGCDIsReady{}},
		}}, true},
		{"sum", &APLValueMath{op: proto.APLValueMath_OpAdd, lhs: &APLValueRemainingTime{}, rhs: two}, true},
		{"division", &APLValueMath{op: proto.APLValueMath_OpDiv, lhs: &APLValueRemainingTime{}, rhs: two}, false},
		{"spell can cast deep inside", &APLValueOr{vals: []APLValue{
			&APLValueGCDIsReady{},
			&APLValueNot{val: &APLValueSpellCanCast{}},
		}}, false},
		{"sequence is ready", &APLValueSequenceIsReady{}, false},
	} {
		if got := aplValueIsPure(tc.value); got != tc.want {
			t.Errorf("%s: aplValueIsPure = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// The gate must only skip what couldn't be cast anyway: in every state, IsReady with logs off (gated)
// matches IsReady with logs on (the long way), and the gated one reads no ExtraCastCondition while
// the gate blocks.
func TestAPLActionGateMatchesTheLongWay(t *testing.T) {
	var spell, rageSpell *Spell
	extraConditionCalls := 0
	extraConditionPasses := true
	sim, agent := newTimingTestSim(t, 100, func(a *timingTestAgent) {
		a.EnableEnergyBar(100)
		spell = a.RegisterSpell(SpellConfig{
			ActionID:   ActionID{SpellID: testSpellNoServerData},
			Flags:      SpellFlagAPL,
			EnergyCost: EnergyCostOptions{Cost: 40},
			Cast: CastConfig{
				DefaultCast: Cast{GCD: GCDDefault},
				CD:          Cooldown{Timer: a.NewTimer(), Duration: 10 * time.Second},
			},
			ExtraCastCondition: func(*Simulation, *Unit) bool {
				extraConditionCalls++
				return extraConditionPasses
			},
		})
		// no rage bar, so never castable, and the gate leaves rage to CanCast
		rageSpell = a.RegisterSpell(SpellConfig{
			ActionID: ActionID{SpellID: testSpellNoServerData, Tag: 1},
			Flags:    SpellFlagAPL,
			RageCost: RageCostOptions{Cost: 10},
			Cast:     CastConfig{DefaultCast: Cast{GCD: GCDDefault}},
		})
	})
	unit := &agent.Unit
	rot := &APLRotation{unit: unit}
	castSpell := &proto.APLAction{
		Condition: &proto.APLValue{Value: &proto.APLValue_Const{Const: &proto.APLValueConst{Val: "true"}}},
		Action:    &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{SpellId: spell.ActionID.ToProto()}},
	}
	gatedCast := rot.newAPLAction(castSpell)
	gatedSequence := rot.newAPLAction(&proto.APLAction{
		Condition: castSpell.Condition,
		Action: &proto.APLAction_Sequence{Sequence: &proto.APLActionSequence{
			Actions: []*proto.APLAction{{Action: castSpell.Action}},
		}},
	})
	gatedStrictSequence := rot.newAPLAction(&proto.APLAction{
		Condition: castSpell.Condition,
		Action: &proto.APLAction_StrictSequence{StrictSequence: &proto.APLActionStrictSequence{
			Actions: []*proto.APLAction{{Action: castSpell.Action}},
		}},
	})
	gatedRageCast := rot.newAPLAction(&proto.APLAction{
		Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{SpellId: rageSpell.ActionID.ToProto()}},
	})
	ungatedCast := rot.newAPLAction(&proto.APLAction{
		Condition: &proto.APLValue{Value: &proto.APLValue_SpellCanCast{SpellCanCast: &proto.APLValueSpellCanCast{SpellId: spell.ActionID.ToProto()}}},
		Action:    castSpell.Action,
	})
	if gatedCast.gatedSpell != spell || gatedSequence.gatedSequence == nil || gatedStrictSequence.gatedSequence == nil ||
		gatedRageCast.gatedSpell != rageSpell || ungatedCast.gatedSpell != nil {
		t.Fatalf("gates: cast %v, sequence %v, strict sequence %v, rage cast %v, spell can cast condition %v",
			gatedCast.gatedSpell, gatedSequence.gatedSequence, gatedStrictSequence.gatedSequence, gatedRageCast.gatedSpell, ungatedCast.gatedSpell)
	}
	gated := []*APLAction{gatedCast, gatedSequence, gatedStrictSequence}
	for _, action := range append(gated, gatedRageCast) {
		action.Finalize(rot)
	}

	now := sim.CurrentTime
	for _, tc := range []struct {
		name       string
		ready      bool
		gateBlocks bool
		set        func()
	}{
		{"ready", true, false, func() {}},
		{"extra condition fails", false, false, func() { extraConditionPasses = false }},
		{"on cooldown", false, true, func() { spell.CD.Set(now + time.Second) }},
		{"on the GCD", false, true, func() { unit.GCD.Set(now + time.Second) }},
		{"casting", false, true, func() { unit.Hardcast.Expires = now + time.Second }},
		{"short on energy", false, true, func() { unit.currentEnergy = 30 }},
	} {
		for _, action := range gated {
			spell.CD.Reset()
			unit.GCD.Reset()
			unit.Hardcast.Expires = startingCDTime
			unit.currentEnergy = 100
			extraConditionPasses = true
			tc.set()

			sim.Log = func(string, ...interface{}) {}
			longWay := action.IsReady(sim)
			sim.Log = nil
			extraConditionCalls = 0
			gated := action.IsReady(sim)

			if gated != longWay || gated != tc.ready {
				t.Errorf("%s, %s: gated %v, the long way %v", tc.name, action, gated, longWay)
			}
			if tc.gateBlocks && extraConditionCalls != 0 {
				t.Errorf("%s, %s: the gated IsReady read the extra cast condition", tc.name, action)
			}
		}
	}

	unit.GCD.Reset()
	if aplSpellBlocked(rageSpell, sim) {
		t.Fatalf("%s: the gate blocked it, so this doesn't test what's left after the gate", gatedRageCast)
	}
	if gatedRageCast.IsReady(sim) {
		t.Errorf("%s: ready without the rage to cast it", gatedRageCast)
	}
}
