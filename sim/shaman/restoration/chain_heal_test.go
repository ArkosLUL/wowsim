package restoration

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Chain Heal jumps to units in the world only: a Greater Fire Elemental that isn't out can't be healed. With
// the glyph it heals 4, and two shamans and their elementals are all there is to jump to: the raid's pets
// come after its players.
func TestChainHealSkipsAnAwayFireElemental(t *testing.T) {
	shamanProto := func() *proto.Player {
		return &proto.Player{
			Race:          proto.Race_RaceTroll,
			Class:         proto.Class_ClassShaman,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsStandard,
			TalentsString: StandardTalents,
			Glyphs:        StandardGlyphs,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}
	}
	rsr := &proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{shamanProto(), shamanProto()}}}},
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	}
	sim := core.NewSim(rsr)
	shaman := sim.Raid.Parties[0].Players[0].(*RestorationShaman)
	elemental := &shaman.FireElemental.Unit

	hits := func(summoned bool) (onElemental, all int32) {
		sim.Reset()
		sim.PrePull()
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		if summoned {
			shaman.FireElemental.EnableWithTimeout(sim, shaman.FireElemental, time.Minute)
		}
		if !shaman.ChainHeal.Cast(sim, &shaman.Unit) {
			t.Fatal("Chain Heal wasn't cast")
		}
		for sim.CurrentTime < 3*time.Second && !sim.Step() {
		}
		for _, unit := range sim.Raid.AllUnits {
			m := shaman.ChainHeal.SpellMetrics[unit.UnitIndex]
			all += m.Hits + m.Crits
			if unit == elemental {
				onElemental = m.Hits + m.Crits
			}
		}
		sim.Cleanup()
		return onElemental, all
	}

	if onElemental, all := hits(false); onElemental != 0 || all != 3 {
		t.Errorf("with the elemental away: %d of %d heals on it, want none of 3", onElemental, all)
	}
	if onElemental, all := hits(true); onElemental != 1 || all != 4 {
		t.Errorf("with the elemental out: %d of %d heals on it, want 1 of 4", onElemental, all)
	}
}
