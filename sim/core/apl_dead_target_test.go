package core

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// Spell::CheckCast refuses a dead target (SPELL_FAILED_TARGETS_DEAD), so the APL neither casts a heal at a dead
// raid member nor picks one for a multidot or multishield. A caster the sim plays on at 0 health still heals
// itself.
func TestHelpfulCastsSkipTheDead(t *testing.T) {
	var heals []*Spell
	timingTestSetup = func(a *timingTestAgent) {
		heals = append(heals, a.RegisterSpell(SpellConfig{
			ActionID:     ActionID{SpellID: testSpellNoServerData},
			Flags:        SpellFlagAPL | SpellFlagHelpful,
			ApplyEffects: func(*Simulation, *Unit, *Spell) {},
			Hot: DotConfig{
				Aura:          Aura{Label: "test hot"},
				NumberOfTicks: 1,
				TickLength:    20 * time.Second,
				TicksCanCrit:  false,
				OnTick:        func(*Simulation, *Unit, *Dot) {},
			},
			Shield: ShieldConfig{Aura: Aura{Label: "test shield", Duration: 20 * time.Second}},
		}))
	}
	defer func() { timingTestSetup = nil }()
	player := func(name string) *proto.Player {
		return &proto.Player{
			Name:      name,
			Class:     proto.Class_ClassShaman,
			Consumes:  &proto.Consumes{},
			Buffs:     &proto.IndividualBuffs{},
			Spec:      &proto.Player_EnhancementShaman{},
			Equipment: &proto.EquipmentSpec{},
		}
	}
	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true},
		Raid: &proto.Raid{Parties: []*proto.Party{{
			Players: []*proto.Player{player("caster"), player("other")},
			Buffs:   &proto.PartyBuffs{},
		}}},
		Encounter: &proto.Encounter{
			Targets:  []*proto.Target{{Name: "target", Level: 83, MobType: proto.MobType_MobTypeDemon}},
			Duration: 60,
		},
	})
	sim.reset()
	caster := &sim.Raid.Parties[0].Players[0].(*timingTestAgent).Unit
	other := &sim.Raid.Parties[0].Players[1].(*timingTestAgent).Unit
	heal := heals[0]
	if heal.Unit != caster || !other.HasHealthBar() || !caster.HasHealthBar() {
		t.Fatal("the heal isn't the caster's, or someone has no health bar")
	}
	noOverlap := &APLValueConst{valType: proto.APLValueType_ValueTypeDuration}
	atOther := &APLActionCastSpell{spell: heal, target: UnitReference{fixedUnit: other}}
	atSelf := &APLActionCastSpell{spell: heal, target: UnitReference{fixedUnit: caster}}
	channelAtOther := &APLActionChannelSpell{spell: heal, target: UnitReference{fixedUnit: other}}
	multidot := &APLActionMultidot{spell: heal, maxDots: 2, maxOverlap: noOverlap}
	multishield := &APLActionMultishield{spell: heal, maxShields: 2, maxOverlap: noOverlap}
	// the caster has both already, so the picks come down to the other player
	heal.Hot(caster).Apply(sim)
	heal.Shield(caster).Apply(sim, 100)

	for _, tc := range []struct {
		name string
		dead bool
	}{{"alive", false}, {"dead", true}} {
		if tc.dead {
			other.RemoveHealth(sim, other.CurrentHealth())
			caster.RemoveHealth(sim, caster.CurrentHealth())
		}
		ready := !tc.dead
		for _, c := range []struct {
			name string
			got  bool
		}{
			{"cast", atOther.IsReady(sim)},
			{"cast past the gate", atOther.isReadyPastGate(sim)},
			{"channel", channelAtOther.IsReady(sim)},
			{"channel past the gate", channelAtOther.isReadyPastGate(sim)},
			{"multidot", multidot.IsReady(sim) && multidot.nextTarget == other},
			{"multishield", multishield.IsReady(sim) && multishield.nextTarget == other},
		} {
			if c.got != ready {
				t.Errorf("%s, the other player %s: ready %v, want %v", c.name, tc.name, c.got, ready)
			}
		}
		if !atSelf.IsReady(sim) || !atSelf.isReadyPastGate(sim) {
			t.Errorf("the caster %s can't heal itself", tc.name)
		}
	}
}
