package main

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

type role struct {
	stats  []proto.Stat
	pseudo []proto.PseudoStat
}

var (
	melee = role{
		stats: []proto.Stat{proto.Stat_StatStrength, proto.Stat_StatAgility, proto.Stat_StatIntellect, proto.Stat_StatSpellPower,
			proto.Stat_StatAttackPower, proto.Stat_StatMeleeHit, proto.Stat_StatSpellHit, proto.Stat_StatMeleeCrit,
			proto.Stat_StatSpellCrit, proto.Stat_StatMeleeHaste, proto.Stat_StatSpellHaste, proto.Stat_StatArmorPenetration,
			proto.Stat_StatExpertise},
		pseudo: []proto.PseudoStat{proto.PseudoStat_PseudoStatMainHandDps, proto.PseudoStat_PseudoStatOffHandDps},
	}
	hunter = role{
		stats: []proto.Stat{proto.Stat_StatAgility, proto.Stat_StatIntellect, proto.Stat_StatAttackPower, proto.Stat_StatRangedAttackPower,
			proto.Stat_StatMeleeHit, proto.Stat_StatMeleeCrit, proto.Stat_StatMeleeHaste, proto.Stat_StatArmorPenetration},
		pseudo: []proto.PseudoStat{proto.PseudoStat_PseudoStatRangedDps},
	}
	caster = role{
		stats: []proto.Stat{proto.Stat_StatIntellect, proto.Stat_StatSpirit, proto.Stat_StatSpellPower, proto.Stat_StatMP5,
			proto.Stat_StatSpellHit, proto.Stat_StatSpellCrit, proto.Stat_StatSpellHaste},
	}
	tank = role{
		stats: []proto.Stat{proto.Stat_StatStrength, proto.Stat_StatAgility, proto.Stat_StatStamina, proto.Stat_StatSpellPower,
			proto.Stat_StatAttackPower, proto.Stat_StatMeleeHit, proto.Stat_StatSpellHit, proto.Stat_StatMeleeCrit,
			proto.Stat_StatSpellCrit, proto.Stat_StatMeleeHaste, proto.Stat_StatSpellHaste, proto.Stat_StatArmorPenetration,
			proto.Stat_StatExpertise, proto.Stat_StatArmor, proto.Stat_StatDefense, proto.Stat_StatDodge, proto.Stat_StatParry,
			proto.Stat_StatBlock, proto.Stat_StatBlockValue},
		pseudo: []proto.PseudoStat{proto.PseudoStat_PseudoStatMainHandDps},
	}
)

func roleOf(spec proto.Spec) (role, error) {
	switch spec {
	case proto.Spec_SpecTankDeathknight, proto.Spec_SpecFeralTankDruid, proto.Spec_SpecProtectionPaladin,
		proto.Spec_SpecProtectionWarrior:
		return tank, nil
	case proto.Spec_SpecHunter:
		return hunter, nil
	case proto.Spec_SpecMage, proto.Spec_SpecWarlock, proto.Spec_SpecBalanceDruid, proto.Spec_SpecElementalShaman,
		proto.Spec_SpecShadowPriest, proto.Spec_SpecSmitePriest:
		return caster, nil
	case proto.Spec_SpecWarrior, proto.Spec_SpecDeathknight, proto.Spec_SpecRogue, proto.Spec_SpecRetributionPaladin,
		proto.Spec_SpecEnhancementShaman, proto.Spec_SpecFeralDruid:
		return melee, nil
	}
	return role{}, fmt.Errorf("no stat list for %s", spec)
}

// slope = (sim(base+shift+delta) - sim(base+shift-delta)) / 2delta, both ends on one seed. Much wider
// than the sim's own ±20 stat weights so a rotation breakpoint in range averages out.
type step struct {
	name   string
	index  int
	pseudo bool
	delta  float64
	shift  float64
}

func stepsFor(r role, final []float64) []step {
	var out []step
	for _, st := range r.stats {
		s := step{name: st.String(), index: int(st), delta: 100}
		switch st {
		case proto.Stat_StatMeleeHaste, proto.Stat_StatSpellHaste:
			// ±100 zig-zags between phases on haste breakpoints
			s.delta = 300
		case proto.Stat_StatAttackPower, proto.Stat_StatRangedAttackPower, proto.Stat_StatSpellPower:
			s.delta = 200
		// hit and expertise are measured below the cap, where they're still worth something
		case proto.Stat_StatExpertise:
			s.delta, s.shift = 30, -60
		case proto.Stat_StatMeleeHit:
			s.shift = -200
		case proto.Stat_StatSpellHit:
			s.shift = -250
		case proto.Stat_StatArmorPenetration:
			// ArP clamps at 0%, so the low end has to stay at or above 0 rating or low-ArP gear reads 0
			s.shift = max(-100, s.delta-final[st])
		case proto.Stat_StatArmor:
			s.delta = 1000
		}
		out = append(out, s)
	}
	for _, ps := range r.pseudo {
		out = append(out, step{name: ps.String(), index: int(ps), pseudo: true, delta: 10})
	}
	return out
}

func raidOf(settings *proto.IndividualSimSettings) *proto.Raid {
	raid := core.SinglePlayerRaidProto(settings.Player, settings.PartyBuffs, settings.RaidBuffs, settings.Debuffs)
	raid.Tanks = settings.Tanks
	return raid
}

func finalStats(settings *proto.IndividualSimSettings) []float64 {
	cs := core.ComputeStats(&proto.ComputeStatsRequest{Raid: raidOf(settings), Encounter: settings.Encounter})
	return cs.RaidStats.Parties[0].Players[0].FinalStats.Stats
}

func simulate(settings *proto.IndividualSimSettings, iterations int32, seed int64) (*proto.UnitMetrics, error) {
	req := &proto.RaidSimRequest{Raid: raidOf(settings), Encounter: settings.Encounter,
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: seed, IsTest: true}}
	progress := make(chan *proto.ProgressMetrics, 100)
	core.RunRaidSimAsync(req, progress)
	for p := range progress {
		if r := p.FinalRaidResult; r != nil {
			if r.ErrorResult != "" {
				return nil, fmt.Errorf("sim: %s", r.ErrorResult)
			}
			return r.RaidMetrics.Parties[0].Players[0], nil
		}
	}
	return nil, fmt.Errorf("sim ended without a result")
}

func shifted(settings *proto.IndividualSimSettings, s step, by float64) *proto.IndividualSimSettings {
	out := googleProto.Clone(settings).(*proto.IndividualSimSettings)
	p := out.Player
	if p.BonusStats == nil {
		p.BonusStats = &proto.UnitStats{}
	}
	if len(p.BonusStats.Stats) < int(stats.Len) {
		p.BonusStats.Stats = append(p.BonusStats.Stats, make([]float64, int(stats.Len)-len(p.BonusStats.Stats))...)
	}
	if len(p.BonusStats.PseudoStats) < stats.PseudoStatsLen {
		p.BonusStats.PseudoStats = append(p.BonusStats.PseudoStats, make([]float64, stats.PseudoStatsLen-len(p.BonusStats.PseudoStats))...)
	}
	if s.pseudo {
		p.BonusStats.PseudoStats[s.index] += by
	} else {
		p.BonusStats.Stats[s.index] += by
	}
	return out
}

func measure(settings *proto.IndividualSimSettings, s step, iterations int32, seed int64) (slope, error) {
	hi, err := simulate(shifted(settings, s, s.shift+s.delta), iterations, seed)
	if err != nil {
		return slope{}, err
	}
	lo, err := simulate(shifted(settings, s, s.shift-s.delta), iterations, seed)
	if err != nil {
		return slope{}, err
	}
	d := 2 * s.delta
	return slope{Delta: s.delta, Shift: s.shift, Dps: (hi.GetDps().GetAvg() - lo.GetDps().GetAvg()) / d,
		Tps: (hi.GetThreat().GetAvg() - lo.GetThreat().GetAvg()) / d, Tmi: (hi.GetTmi().GetAvg() - lo.GetTmi().GetAvg()) / d}, nil
}
