// Package statslopes measures what one point of a stat is worth from sims stepped wide around the
// player's gear: centered for tools/statweights' shipped values, one-sided for the caps tab's live
// run at the player's own gear.
package statslopes

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

// Role is the stats a spec gets measured on.
type Role struct {
	Stats       []proto.Stat
	PseudoStats []proto.PseudoStat
}

var (
	Melee = Role{
		Stats: []proto.Stat{proto.Stat_StatStrength, proto.Stat_StatAgility, proto.Stat_StatIntellect, proto.Stat_StatSpellPower,
			proto.Stat_StatAttackPower, proto.Stat_StatMeleeHit, proto.Stat_StatSpellHit, proto.Stat_StatMeleeCrit,
			proto.Stat_StatSpellCrit, proto.Stat_StatMeleeHaste, proto.Stat_StatSpellHaste, proto.Stat_StatArmorPenetration,
			proto.Stat_StatExpertise},
		PseudoStats: []proto.PseudoStat{proto.PseudoStat_PseudoStatMainHandDps, proto.PseudoStat_PseudoStatOffHandDps},
	}
	Hunter = Role{
		Stats: []proto.Stat{proto.Stat_StatAgility, proto.Stat_StatIntellect, proto.Stat_StatAttackPower, proto.Stat_StatRangedAttackPower,
			proto.Stat_StatMeleeHit, proto.Stat_StatMeleeCrit, proto.Stat_StatMeleeHaste, proto.Stat_StatArmorPenetration},
		PseudoStats: []proto.PseudoStat{proto.PseudoStat_PseudoStatRangedDps},
	}
	Caster = Role{
		Stats: []proto.Stat{proto.Stat_StatIntellect, proto.Stat_StatSpirit, proto.Stat_StatSpellPower, proto.Stat_StatMP5,
			proto.Stat_StatSpellHit, proto.Stat_StatSpellCrit, proto.Stat_StatSpellHaste},
	}
	Tank = Role{
		Stats: []proto.Stat{proto.Stat_StatStrength, proto.Stat_StatAgility, proto.Stat_StatStamina, proto.Stat_StatSpellPower,
			proto.Stat_StatAttackPower, proto.Stat_StatMeleeHit, proto.Stat_StatSpellHit, proto.Stat_StatMeleeCrit,
			proto.Stat_StatSpellCrit, proto.Stat_StatMeleeHaste, proto.Stat_StatSpellHaste, proto.Stat_StatArmorPenetration,
			proto.Stat_StatExpertise, proto.Stat_StatArmor, proto.Stat_StatDefense, proto.Stat_StatDodge, proto.Stat_StatParry,
			proto.Stat_StatBlock, proto.Stat_StatBlockValue},
		PseudoStats: []proto.PseudoStat{proto.PseudoStat_PseudoStatMainHandDps},
	}
	// read in HPS
	Healer = Role{
		Stats: []proto.Stat{proto.Stat_StatIntellect, proto.Stat_StatSpirit, proto.Stat_StatSpellPower, proto.Stat_StatMP5,
			proto.Stat_StatSpellCrit, proto.Stat_StatSpellHaste},
	}
)

func RoleOf(spec proto.Spec) (Role, error) {
	switch spec {
	case proto.Spec_SpecTankDeathknight, proto.Spec_SpecFeralTankDruid, proto.Spec_SpecProtectionPaladin,
		proto.Spec_SpecProtectionWarrior:
		return Tank, nil
	case proto.Spec_SpecHunter:
		return Hunter, nil
	case proto.Spec_SpecMage, proto.Spec_SpecWarlock, proto.Spec_SpecBalanceDruid, proto.Spec_SpecElementalShaman,
		proto.Spec_SpecShadowPriest, proto.Spec_SpecSmitePriest:
		return Caster, nil
	case proto.Spec_SpecWarrior, proto.Spec_SpecDeathknight, proto.Spec_SpecRogue, proto.Spec_SpecRetributionPaladin,
		proto.Spec_SpecEnhancementShaman, proto.Spec_SpecFeralDruid:
		return Melee, nil
	case proto.Spec_SpecHolyPaladin, proto.Spec_SpecRestorationDruid, proto.Spec_SpecRestorationShaman,
		proto.Spec_SpecHealingPriest:
		return Healer, nil
	}
	return Role{}, fmt.Errorf("no stat list for %s", spec)
}

// Step is how far a stat moves from the gear. Much wider than the sim's own ±20 stat weights so a
// rotation breakpoint in range averages out.
type Step struct {
	// the Stat's or PseudoStat's proto enum name
	Name   string
	Index  int
	Pseudo bool
	Delta  float64
	Shift  float64
}

func stepDelta(st proto.Stat) float64 {
	switch st {
	case proto.Stat_StatMeleeHaste, proto.Stat_StatSpellHaste:
		// ±100 zig-zags between phases on haste breakpoints
		return 300
	case proto.Stat_StatAttackPower, proto.Stat_StatRangedAttackPower, proto.Stat_StatSpellPower:
		return 200
	case proto.Stat_StatExpertise:
		return 30
	case proto.Stat_StatArmor:
		return 1000
	}
	return 100
}

const pseudoDelta = 10

// CenteredSteps are the steps for slope = (sim(gear+Shift+Delta) − sim(gear+Shift−Delta)) / 2Delta.
// final is the player's final stats.
func CenteredSteps(r Role, final []float64) []Step {
	var out []Step
	for _, st := range r.Stats {
		s := Step{Name: st.String(), Index: int(st), Delta: stepDelta(st)}
		switch st {
		// hit and expertise are measured below the cap, where they're still worth something
		case proto.Stat_StatExpertise:
			s.Shift = -60
		case proto.Stat_StatMeleeHit:
			s.Shift = -200
		case proto.Stat_StatSpellHit:
			s.Shift = -250
		case proto.Stat_StatArmorPenetration:
			// ArP clamps at 0%, so the low end has to stay at or above 0 rating or low-ArP gear reads 0
			s.Shift = max(-100, s.Delta-final[st])
		}
		out = append(out, s)
	}
	for _, ps := range r.PseudoStats {
		out = append(out, Step{Name: ps.String(), Index: int(ps), Pseudo: true, Delta: pseudoDelta})
	}
	return out
}

// Metrics are the first player's sim averages, or their change per point.
type Metrics struct {
	Dps, Tps, Tmi, Hps float64
}

func metricsOf(u *proto.UnitMetrics) Metrics {
	return Metrics{Dps: u.GetDps().GetAvg(), Tps: u.GetThreat().GetAvg(), Tmi: u.GetTmi().GetAvg(), Hps: u.GetHps().GetAvg()}
}

// per point, from a to b over d points
func perPoint(a, b Metrics, d float64) Metrics {
	return Metrics{Dps: (b.Dps - a.Dps) / d, Tps: (b.Tps - a.Tps) / d, Tmi: (b.Tmi - a.Tmi) / d, Hps: (b.Hps - a.Hps) / d}
}

// FinalStats is the raid's first player's final stats.
func FinalStats(raid *proto.Raid, encounter *proto.Encounter) []float64 {
	cs := core.ComputeStats(&proto.ComputeStatsRequest{Raid: raid, Encounter: encounter})
	return cs.RaidStats.Parties[0].Players[0].FinalStats.Stats
}

func simRequest(raid *proto.Raid, encounter *proto.Encounter, iterations int32, seed int64) *proto.RaidSimRequest {
	return &proto.RaidSimRequest{Raid: raid, Encounter: encounter,
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: seed, IsTest: true}}
}

// Simulate runs the raid at one seed with IsTest, its iterations spread over the cores, and returns
// the first player's metrics.
func Simulate(raid *proto.Raid, encounter *proto.Encounter, iterations int32, seed int64) (*proto.UnitMetrics, error) {
	progress := make(chan *proto.ProgressMetrics, 100)
	core.RunRaidSimAsync(simRequest(raid, encounter, iterations, seed), progress)
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

// Shifted is a copy of the raid with its first player's bonus stats moved by `by` along s.
func Shifted(raid *proto.Raid, s Step, by float64) *proto.Raid {
	out := googleProto.Clone(raid).(*proto.Raid)
	p := out.Parties[0].Players[0]
	if p.BonusStats == nil {
		p.BonusStats = &proto.UnitStats{}
	}
	if len(p.BonusStats.Stats) < int(stats.Len) {
		p.BonusStats.Stats = append(p.BonusStats.Stats, make([]float64, int(stats.Len)-len(p.BonusStats.Stats))...)
	}
	if len(p.BonusStats.PseudoStats) < stats.PseudoStatsLen {
		p.BonusStats.PseudoStats = append(p.BonusStats.PseudoStats, make([]float64, stats.PseudoStatsLen-len(p.BonusStats.PseudoStats))...)
	}
	if s.Pseudo {
		p.BonusStats.PseudoStats[s.Index] += by
	} else {
		p.BonusStats.Stats[s.Index] += by
	}
	return out
}

// Centered is s's slope per point, both ends at the same seed.
func Centered(raid *proto.Raid, encounter *proto.Encounter, s Step, iterations int32, seed int64) (Metrics, error) {
	hi, err := Simulate(Shifted(raid, s, s.Shift+s.Delta), encounter, iterations, seed)
	if err != nil {
		return Metrics{}, err
	}
	lo, err := Simulate(Shifted(raid, s, s.Shift-s.Delta), encounter, iterations, seed)
	if err != nil {
		return Metrics{}, err
	}
	return perPoint(metricsOf(lo), metricsOf(hi), 2*s.Delta), nil
}
