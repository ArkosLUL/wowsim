package healing

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Discipline to Penance without Grace, which would raise the later ticks, then Spiritual Healing 5/5 and
// Blessed Resilience 3/3
const penanceHealTalents = "0533203101300502331000100001-0055510020201525003"

// Spiritual Healing and Twin Disciplines are percent mods on 52985, Blessed Resilience and Focused Power healing
// done auras, so a heal tick multiplies all four. Live at 2290 spell power, with the last two only, a tick
// healed 2960 to 3170.
func TestHealPenanceTalentsMultiply(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsDisc,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: penanceHealTalents,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*HealingPriest)
	talents := priest.Talents
	if priest.PenanceHeal == nil || talents.Grace != 0 || talents.SpiritualHealing != 5 || talents.BlessedResilience != 3 ||
		talents.FocusedPower == 0 || talents.TwinDisciplines == 0 {
		t.Fatalf("Penance %v, Grace %d, Spiritual Healing %d, Blessed Resilience %d, Focused Power %d, Twin Disciplines %d",
			priest.PenanceHeal != nil, talents.Grace, talents.SpiritualHealing, talents.BlessedResilience,
			talents.FocusedPower, talents.TwinDisciplines)
	}
	spell := priest.PenanceHeal
	priest.AddStatDynamic(sim, stats.SpellPower, -spell.SpellPower())
	if power := spell.HealingPower(&priest.Unit); power != 0 {
		t.Fatalf("healing power %v, want 0", power)
	}

	multiplier := (1 + .02*float64(talents.SpiritualHealing)) * (1 + .01*float64(talents.BlessedResilience)) *
		(1 + .02*float64(talents.FocusedPower)) * (1 + .01*float64(talents.TwinDisciplines))
	metrics := &spell.SpellMetrics[priest.UnitIndex]
	hot := spell.Hot(&priest.Unit)
	hot.Apply(sim)
	for landed, i := 0, 0; landed < 20; i++ {
		if i == 2000 {
			t.Fatalf("%d of 2000 ticks went without a crit", landed)
		}
		before := *metrics
		hot.TickOnce(sim)
		if metrics.Hits == before.Hits {
			continue
		}
		landed++
		if got := metrics.TotalHealing - before.TotalHealing; got < 1484*multiplier-1 || got > 1676*multiplier+1 {
			t.Fatalf("a tick heals %.1f, want %.1f to %.1f", got, 1484*multiplier, 1676*multiplier)
		}
	}
}
