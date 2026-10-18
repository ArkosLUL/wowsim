package restoration

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// spell_sha_earth_shield's charge is 49284's SpellHealingBonusDone, which takes Purification's healing done
// percent, and 379 heals for it with no caster mods of its own: 10% more at 5/5 than at 0/5.
func TestEarthShieldChargeTakesPurification(t *testing.T) {
	// StandardTalents without Improved Shields, with Purification as given
	charge := func(talents string, purification int32) float64 {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceTroll,
				Class:         proto.Class_ClassShaman,
				Equipment:     &proto.EquipmentSpec{},
				Consumes:      &proto.Consumes{},
				Spec:          PlayerOptionsStandard,
				Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
				TalentsString: talents,
			}, nil, nil, nil),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		shaman := sim.Raid.Parties[0].Players[0].(*RestorationShaman)
		if shaman.Talents.Purification != purification || shaman.Talents.ImprovedShields != 0 || !shaman.Talents.EarthShield {
			t.Fatalf("Purification %d, Improved Shields %d, Earth Shield %v", shaman.Talents.Purification,
				shaman.Talents.ImprovedShields, shaman.Talents.EarthShield)
		}
		target := &shaman.Unit
		hot := shaman.EarthShield.Hot(target)
		hot.Apply(sim)
		return uncritCharge(t, sim, hot)
	}

	with := charge("-3020500-50005331335310501122331251", 5)
	without := charge("-3020500-50005331335310001122331251", 0)
	if without <= 0 || math.Abs(with-1.1*without) > 1.1 {
		t.Errorf("a charge healed %v with Purification 5/5 and %v without, want 10%% more", with, without)
	}
}

// The charge is whole at each step of spell_sha_earth_shield::CalculateAmount: SpellHealingBonusDone's uint32,
// the glyph's AddPct, then Improved Shields' ApplyPct on what the bonuses added, each in float32 and truncated.
func TestEarthShieldChargeIsWhole(t *testing.T) {
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
	if shaman.Talents.Purification != 5 || shaman.Talents.ImprovedShields != 3 {
		t.Fatalf("Purification %d, Improved Shields %d", shaman.Talents.Purification, shaman.Talents.ImprovedShields)
	}
	target := &shaman.Unit
	hot := shaman.EarthShield.Hot(target)

	for _, spellPower := range []float64{0, 1000, 1337, 2113} {
		shaman.AddStatDynamic(sim, stats.SpellPower, spellPower)
		hot.Apply(sim)
		got := uncritCharge(t, sim, hot)
		hot.Deactivate(sim)

		base := float32(hot.Tick.Min)
		power := shaman.EarthShield.HealingPower(target) + 0.05*float64(shaman.Talents.NaturesBlessing)*shaman.GetStat(stats.Intellect)
		charge := float32(int32(float32((base + float32(core.PowerBonus(hot.Tick.SP, power))) * 1.1)))
		charge += float32(int32(float32(charge*20) / 100))
		charge += float32(int32(float32((charge-base)*15) / 100))
		if got != float64(charge) {
			t.Errorf("with %v more spell power a charge healed %v, want %v", spellPower, got, charge)
		}
		shaman.AddStatDynamic(sim, stats.SpellPower, -spellPower)
	}
}

// 379 crits off the shaman (Spell::DoAllEffectOnLaunchTarget rolls with the original caster), for half again
// as much, as a magic heal does. Tidal Mastery's class mask lists 379 only from rank 4 on, where every rank
// covers Healing Wave.
func TestEarthShieldChargeCanCrit(t *testing.T) {
	newShaman := func(talents string) (*core.Simulation, *RestorationShaman) {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceTroll,
				Class:         proto.Class_ClassShaman,
				Equipment:     &proto.EquipmentSpec{},
				Consumes:      &proto.Consumes{},
				Spec:          PlayerOptionsStandard,
				Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
				TalentsString: talents,
			}, nil, nil, nil),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		return sim, sim.Raid.Parties[0].Players[0].(*RestorationShaman)
	}

	sim, shaman := newShaman(StandardTalents)
	if shaman.Talents.TidalMastery != 5 {
		t.Fatalf("Tidal Mastery %d, want 5", shaman.Talents.TidalMastery)
	}
	if es, hw := shaman.EarthShield.HealingCritChance(), shaman.HealingWave.HealingCritChance(); es != hw {
		t.Errorf("at Tidal Mastery 5/5 a charge crits %.4f, Healing Wave %.4f, want the same", es, hw)
	}
	hot := shaman.EarthShield.Hot(&shaman.Unit)
	metrics := &shaman.EarthShield.SpellMetrics[shaman.UnitIndex]
	hot.Apply(sim)
	normal, crit := 0.0, 0.0
	for i := 0; i < 1000 && (normal == 0 || crit == 0); i++ {
		before := *metrics
		hot.TickOnce(sim)
		switch healed := metrics.TotalHealing - before.TotalHealing; {
		case metrics.Crits > before.Crits:
			crit = healed
		case metrics.Hits > before.Hits:
			normal = healed
		}
	}
	if normal == 0 || crit == 0 {
		t.Fatalf("1000 charges: normal %v, crit %v", normal, crit)
	}
	if want := math.Trunc(normal * 1.5); crit != want {
		t.Errorf("a charge heals %v, its crit %v, want %v", normal, crit, want)
	}

	// Tidal Mastery 3/5
	_, shaman = newShaman("-3020503-50005331333310501122331251")
	if shaman.Talents.TidalMastery != 3 {
		t.Fatalf("Tidal Mastery %d, want 3", shaman.Talents.TidalMastery)
	}
	if es, hw := shaman.EarthShield.HealingCritChance(), shaman.HealingWave.HealingCritChance(); math.Abs(hw-es-0.03) > 1e-9 {
		t.Errorf("at Tidal Mastery 3/5 a charge crits %.4f, Healing Wave %.4f, want 3%% less", es, hw)
	}
}

// uncritCharge ticks the charge until one doesn't crit, and gives what that one healed.
func uncritCharge(t *testing.T, sim *core.Simulation, hot *core.Dot) float64 {
	t.Helper()
	metrics := &hot.Spell.SpellMetrics[hot.Unit.UnitIndex]
	for i := 0; i < 100; i++ {
		before := *metrics
		hot.TickOnce(sim)
		if metrics.Hits > before.Hits {
			return metrics.TotalHealing - before.TotalHealing
		}
	}
	t.Fatal("100 charges crit")
	return 0
}
