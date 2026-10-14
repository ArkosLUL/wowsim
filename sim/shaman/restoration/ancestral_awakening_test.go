package restoration

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// spell_sha_ancestral_awakening casts 52752 for CalculatePct(int32(crit heal), 30), and 52752 ignores the
// caster's modifiers: live, with Purification 5/5 each one was exactly that, never 10% more.
func TestAncestralAwakeningIsAWholeThirtyPercent(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTroll,
			Class:         proto.Class_ClassShaman,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsStandard,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: StandardTalents,
			Glyphs:        StandardGlyphs,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	shaman := sim.Raid.Parties[0].Players[0].(*RestorationShaman)
	if shaman.Talents.AncestralAwakening != 3 || shaman.Talents.Purification != 5 {
		t.Fatalf("Ancestral Awakening %d, Purification %d", shaman.Talents.AncestralAwakening, shaman.Talents.Purification)
	}
	target := &shaman.Unit
	wave := &shaman.HealingWave.SpellMetrics[target.UnitIndex]
	awakening := &shaman.AncestralAwakening.SpellMetrics[target.UnitIndex]
	for checked, casts := 0, 0; checked < 3; casts++ {
		if casts == 500 {
			t.Fatalf("%d crits in %d Healing Waves", checked, casts)
		}
		waveBefore, awakeningBefore := *wave, *awakening
		shaman.HealingWave.SkipCastAndApplyEffects(sim, target)
		if wave.Crits == waveBefore.Crits {
			continue
		}
		crit := wave.TotalHealing - waveBefore.TotalHealing
		got := awakening.TotalHealing - awakeningBefore.TotalHealing
		if want := float64(int32(float32(crit) * 30 / 100)); got != want {
			t.Errorf("a %v crit's Ancestral Awakening healed %v, want %v", crit, got, want)
		}
		checked++
	}
}
