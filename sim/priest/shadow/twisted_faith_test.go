package shadow

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// twistedFaithStats returns a naked priest's spirit and spell power with Twisted Faith at this rank.
func twistedFaithStats(t *testing.T, rank int) (spirit, spellPower float64) {
	t.Helper()
	// Twisted Faith is the 26th Shadow talent
	talents := []byte(DefaultTalents)
	talents[len("05032031--")+25] = byte('0' + rank)
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Spec:          PlayerOptionsBasic,
			Glyphs:        &proto.Glyphs{},
			TalentsString: string(talents),
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	priest := sim.Raid.Parties[0].Players[0].(*ShadowPriest)
	if int(priest.Talents.TwistedFaith) != rank {
		t.Fatalf("Twisted Faith at %d, want %d", priest.Talents.TwistedFaith, rank)
	}
	return priest.GetStat(stats.Spirit), priest.GetStat(stats.SpellPower)
}

// Rank 5 gives 53% of Spirit as spell power under mod-spell-tweaks (51167's spell_dbc row), ranks 1-4 stock 4% a rank.
func TestTwistedFaithSpiritToSpellPower(t *testing.T) {
	spirit, untalented := twistedFaithStats(t, 0)
	if spirit <= 0 {
		t.Fatalf("spirit %.2f", spirit)
	}
	for rank, pct := range map[int]float64{4: 0.16, 5: 0.53} {
		gotSpirit, sp := twistedFaithStats(t, rank)
		if math.Abs(gotSpirit-spirit) > 0.001 {
			t.Errorf("rank %d: spirit %.3f, want %.3f", rank, gotSpirit, spirit)
		}
		if got, want := sp-untalented, pct*spirit; math.Abs(got-want) > 0.001 {
			t.Errorf("rank %d: %.3f spell power from %.3f spirit, want %.3f", rank, got, spirit, want)
		}
	}
}
