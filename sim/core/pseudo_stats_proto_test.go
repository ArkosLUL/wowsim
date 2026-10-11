package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The sheet reads defense skill bonuses off the pseudo stats, so the list has to carry them and reach the
// enum's last value.
func TestPseudoStatsProtoCarriesBonusDefenseSkill(t *testing.T) {
	encounter := &proto.Encounter{Duration: 300, Targets: []*proto.Target{NewDefaultTarget()}}
	env, _, _ := NewEnvironment(bloodElfRageUserRaid(), encounter, false)
	warrior := env.Raid.Parties[0].Players[0].GetCharacter()

	if got := warrior.GetPseudoStatsProto(); len(got) != stats.PseudoStatsLen {
		t.Fatalf("%d pseudo stats, the enum has %d", len(got), stats.PseudoStatsLen)
	} else if got[proto.PseudoStat_PseudoStatBonusDefenseSkill] != 0 {
		t.Errorf("bonus defense skill %v with none given", got[proto.PseudoStat_PseudoStatBonusDefenseSkill])
	}
	warrior.PseudoStats.BonusDefenseSkill += 25
	if got := warrior.GetPseudoStatsProto()[proto.PseudoStat_PseudoStatBonusDefenseSkill]; got != 25 {
		t.Errorf("bonus defense skill %v, want 25", got)
	}
}
