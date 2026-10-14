package healing

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// spell_pri_renew casts 63544 for 5% an Empowered Renew rank of all of Renew's ticks, each the aura's amount as
// the target takes it (Unit::SpellHealingBonusTaken), and 63544 then takes the target's terms again with its
// own coefficient. The aura's amount carries Renew's SPELLMOD_DOT talents and glyph; the healing done talents
// come on 63544.
func TestEmpoweredRenewTakesTheTargetsTermsTwice(t *testing.T) {
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
	if priest.EmpoweredRenew == nil || talents.BlessedResilience == 0 || talents.SpiritualHealing == 0 {
		t.Fatalf("Empowered Renew %d, Blessed Resilience %d, Spiritual Healing %d", talents.EmpoweredRenew,
			talents.BlessedResilience, talents.SpiritualHealing)
	}
	priest.AddStatDynamic(sim, stats.SpellPower, -priest.Renew.SpellPower())

	target := &priest.Unit
	target.PseudoStats.BonusHealingTaken = 290 // five Talisman of Troll Divinity stacks
	target.AddHealingTakenPct(20)

	hot := priest.Renew.Hot(target)
	metrics := &priest.EmpoweredRenew.SpellMetrics[target.UnitIndex]
	empowered := func() float64 {
		for range 200 {
			before := *metrics
			hot.Apply(sim)
			priest.EmpoweredRenew.Cast(sim, target)
			if metrics.Hits > before.Hits {
				return metrics.TotalHealing - before.TotalHealing
			}
		}
		t.Fatalf("200 Empowered Renews in a row crit")
		return 0
	}

	dotMods := (1 + .02*float64(talents.SpiritualHealing)) * (1 + .01*float64(talents.TwinDisciplines)) *
		(1 + .05*float64(talents.ImprovedRenew)) * 1.25
	doneMods := (1 + .01*float64(talents.BlessedResilience)) * (1 + .02*float64(talents.FocusedPower))
	amount := math.Trunc(280 * dotMods)
	// Renew's own term on the priest: 0.376 with Empowered Renew's bonus multiplier
	tick := math.Trunc((amount + float64(int32(290*0.376*(1+.05*float64(talents.EmpoweredRenew))))) * 1.2)
	basePoints := 5 * talents.EmpoweredRenew * 4 * int32(tick) / 100 // four ticks under the glyph
	// 63544's own: int32(290 × 0.8057) = 233
	want := math.Trunc((math.Trunc(float64(basePoints)*doneMods) + 233) * 1.2)
	if healed := empowered(); math.Abs(healed-want) > 0.001 {
		t.Errorf("Empowered Renew healed %v, want %v (Renew's tick as taken %v, base points %d)", healed, want, tick, basePoints)
	}

	// Strength of Wrynn is a healing done percent, stood in on the target: 63544 takes it, Renew's amount doesn't
	target.PseudoStats.HealingTakenMultiplier *= 1.3
	want = math.Trunc((math.Trunc(float64(basePoints)*doneMods) + 233) * target.PseudoStats.HealingTakenMultiplier)
	if healed := empowered(); math.Abs(healed-want) > 0.001 {
		t.Errorf("under Strength of Wrynn Empowered Renew healed %v, want %v (base points %d)", healed, want, basePoints)
	}
}
