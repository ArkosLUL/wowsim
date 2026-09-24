package core

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func init() {
	RegisterAgentFactory(
		proto.Player_RetributionPaladin{},
		proto.Spec_SpecRetributionPaladin,
		func(char *Character, _ *proto.Player) Agent {
			return &sheathTestAgent{Character: *char}
		},
		func(player *proto.Player, spec interface{}) {
			player.Spec = spec.(*proto.Player_RetributionPaladin)
		},
	)
}

// Turns 30% of its attack power into spell power, like Sheath of Light.
type sheathTestAgent struct {
	Character
}

func (a *sheathTestAgent) GetCharacter() *Character { return &a.Character }
func (a *sheathTestAgent) Initialize()              {}
func (a *sheathTestAgent) ApplyTalents() {
	a.AddStatDependency(stats.AttackPower, stats.SpellPower, 0.3)
}
func (a *sheathTestAgent) Reset(_ *Simulation)      {}
func (a *sheathTestAgent) OnGCDReady(_ *Simulation) {}

func phaseStatsRaid(raidBuffs *proto.RaidBuffs, individualBuffs *proto.IndividualBuffs) *proto.Raid {
	raid := bloodElfRageUserRaid()
	raid.Buffs = raidBuffs
	raid.Parties[0].Players[0].Buffs = individualBuffs
	return raid
}

func computePhaseStats(t *testing.T, raid *proto.Raid) *proto.PlayerStats {
	t.Helper()
	result := ComputeStats(&proto.ComputeStatsRequest{
		Raid:      raid,
		Encounter: &proto.Encounter{Duration: 300, Targets: []*proto.Target{NewDefaultTarget()}},
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	return result.RaidStats.Parties[0].Players[0]
}

// The test agents add nothing past the snapshots, so the last one is the final stats.
func checkConsumesSnapshotIsFinal(t *testing.T, player *proto.PlayerStats) {
	t.Helper()
	consumes, final := player.ConsumesStats.Stats, player.FinalStats.Stats
	for s := range final {
		if math.Abs(consumes[s]-final[s]) > 1e-6 {
			t.Errorf("Consumes snapshot's %s is %.2f, FinalStats %.2f", stats.Stat(s).StatName(), consumes[s], final[s])
		}
	}
}

// A buff aura whose stat a multiplying buff also raises: the Buffs snapshot applies the multiplier to
// the aura's amount once, as FinalStats does.
func TestBuffsSnapshotMultipliesAuraBuffsOnce(t *testing.T) {
	for _, tc := range []struct {
		name            string
		raidBuffs       *proto.RaidBuffs
		individualBuffs *proto.IndividualBuffs
		stat            stats.Stat
		amount          float64
		multiplier      float64
	}{
		{"Blessing of Might", &proto.RaidBuffs{UnleashedRage: true}, &proto.IndividualBuffs{BlessingOfMight: proto.TristateEffect_TristateEffectRegular}, stats.AttackPower, 550, 1.1},
		{"Blessing of Might, ranged", &proto.RaidBuffs{UnleashedRage: true}, &proto.IndividualBuffs{BlessingOfMight: proto.TristateEffect_TristateEffectImproved}, stats.RangedAttackPower, 687, 1.1},
		{"Battle Shout", &proto.RaidBuffs{AbominationsMight: true, BattleShout: proto.TristateEffect_TristateEffectRegular}, &proto.IndividualBuffs{}, stats.AttackPower, 550, 1.1},
		{"Commanding Shout", &proto.RaidBuffs{StrengthOfWrynn: true, CommandingShout: proto.TristateEffect_TristateEffectRegular}, &proto.IndividualBuffs{}, stats.Health, 2255, 1.3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			player := computePhaseStats(t, phaseStatsRaid(tc.raidBuffs, tc.individualBuffs))

			talents := player.TalentsStats.Stats[tc.stat]
			got := player.BuffsStats.Stats[tc.stat] - talents
			want := tc.multiplier*(talents+tc.amount) - talents
			if math.Abs(got-want) > 1e-6 {
				t.Errorf("Buffs part of %s is %.2f, want %.2f: off by %.2f", tc.stat.StatName(), got, want, got-want)
			}
			checkConsumesSnapshotIsFinal(t, player)
		})
	}
}

// SPELL_AURA_MOD_SPELL_DAMAGE_OF_ATTACK_POWER reads GetTotalAttackPowerValue, so spell power takes the
// 10% buff's AP too, even with the buff's multiplier added after the talent's conversion.
func TestAuraAttackPowerConvertsAfterItsMultiplier(t *testing.T) {
	raid := phaseStatsRaid(&proto.RaidBuffs{UnleashedRage: true}, &proto.IndividualBuffs{BlessingOfMight: proto.TristateEffect_TristateEffectImproved})
	paladin := raid.Parties[0].Players[0]
	paladin.Class = proto.Class_ClassPaladin
	paladin.Spec = &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{}}
	player := computePhaseStats(t, raid)

	final := player.FinalStats.Stats
	if want := 0.3 * final[stats.AttackPower]; math.Abs(final[stats.SpellPower]-want) > 1e-6 {
		t.Errorf("final spell power %.2f, want 30%% of %.2f attack power: %.2f", final[stats.SpellPower], final[stats.AttackPower], want)
	}
	checkConsumesSnapshotIsFinal(t, player)
}
