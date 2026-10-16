package healing

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Renew's aura amount carries its SPELLMOD_DOT talents and glyph, whole, and each tick multiplies that by Blessed
// Resilience's healing done percent in float32 (AuraEffect::HandlePeriodicHealAurasTick): 464 × 1.03f is 477,
// where one truncation of 280 × all the talents would give 478.
func TestRenewTicksTakeTheHealingDoneTalentsLast(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsHoly,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: BlessedResilienceTalents,
			Glyphs:        HolyGlyphs,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*HealingPriest)
	talents := priest.Talents
	if talents.BlessedResilience != 3 || talents.FocusedPower != 0 {
		t.Fatalf("Blessed Resilience %d, Focused Power %d", talents.BlessedResilience, talents.FocusedPower)
	}
	priest.AddStatDynamic(sim, stats.SpellPower, -priest.Renew.SpellPower())

	target := &priest.Unit
	hot := priest.Renew.Hot(target)
	hot.Apply(sim)
	metrics := &priest.Renew.SpellMetrics[target.UnitIndex]
	before := metrics.TotalHealing
	hot.TickOnce(sim)

	dotMods := (1 + .02*float64(talents.SpiritualHealing)) * (1 + .01*float64(talents.TwinDisciplines)) *
		(1 + .05*float64(talents.ImprovedRenew)) * 1.25
	amount := math.Trunc(280 * dotMods)
	want := float64(int32(float32(amount) * float32(1.03)))
	if got := metrics.TotalHealing - before; got != want || want != 477 {
		t.Errorf("Renew ticked %v, want %v (amount %v)", got, want, amount)
	}
}
