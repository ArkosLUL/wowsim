package restoration

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Improved Water Shield at 3/3 on a heal's crit: the talent's 100%, then spell_sha_imp_water_shield's 60% of it for
// Lesser Healing Wave and 30% for Chain Heal, and nothing without Water Shield up.
func TestImprovedWaterShieldOnCrits(t *testing.T) {
	procRate := func(shield proto.ShamanShield, heal func(*RestorationShaman) *core.Spell) float64 {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:      proto.Race_RaceTroll,
				Class:     proto.Class_ClassShaman,
				Equipment: &proto.EquipmentSpec{},
				Consumes:  &proto.Consumes{},
				Spec: &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{
					Options: &proto.RestorationShaman_Options{Shield: shield, Totems: BasicTotems},
				}},
				Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
				TalentsString: StandardTalents,
			}, nil, nil, nil),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		shaman := sim.Raid.Parties[0].Players[0].(*RestorationShaman)
		if shaman.Talents.ImprovedWaterShield != 3 {
			t.Fatalf("Improved Water Shield at %d, want 3", shaman.Talents.ImprovedWaterShield)
		}
		shaman.AddStatDynamic(sim, stats.SpellCrit, 100*core.CritRatingPerCritChance)
		spend := shaman.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen})
		spell := heal(shaman)

		const casts = 4000
		procs := 0
		for range casts {
			shaman.SpendMana(sim, max(0, shaman.CurrentMana()-shaman.MaxMana()/2), spend)
			before := shaman.CurrentMana()
			spell.SkipCastAndApplyEffects(sim, &shaman.Unit)
			if shaman.CurrentMana() > before {
				procs++
			}
		}
		return float64(procs) / casts
	}

	for _, c := range []struct {
		name string
		heal func(*RestorationShaman) *core.Spell
		want float64
	}{
		{"Lesser Healing Wave", func(s *RestorationShaman) *core.Spell { return s.LesserHealingWave }, 0.6},
		{"Chain Heal", func(s *RestorationShaman) *core.Spell { return s.ChainHeal }, 0.3},
		{"Riptide", func(s *RestorationShaman) *core.Spell { return s.Riptide }, 1},
	} {
		if got := procRate(proto.ShamanShield_WaterShield, c.heal); math.Abs(got-c.want) > 0.03 {
			t.Errorf("%s: mana on %.3f of its crits, want %v", c.name, got, c.want)
		}
	}
	if got := procRate(proto.ShamanShield_LightningShield, func(s *RestorationShaman) *core.Spell { return s.Riptide }); got != 0 {
		t.Errorf("Riptide under Lightning Shield: mana on %.3f of its crits, want none", got)
	}
}
