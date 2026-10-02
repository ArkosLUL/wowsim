package warlock

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

func petlessWarlock(t *testing.T, talents string) (*Warlock, *core.Simulation) {
	t.Helper()
	options := googleProto.Clone(DefaultAfflictionWarlock.Warlock.Options).(*proto.Warlock_Options)
	options.Summon = proto.Warlock_Options_NoSummon
	raid := core.SinglePlayerRaidProto(&proto.Player{
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarlock,
		Equipment:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_affliction").GearSet,
		Consumes:      FullConsumes,
		Spec:          &proto.Player_Warlock{Warlock: &proto.Warlock{Options: options}},
		Rotation:      core.GetAplRotation("../../ui/warlock/apls", "affliction").Rotation,
		TalentsString: talents,
	}, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	return sim.Raid.Parties[0].Players[0].(*Warlock), sim
}

func TestDarkPactWithoutPet(t *testing.T) {
	warlock, sim := petlessWarlock(t, DarkPactTalents)
	if warlock.DarkPact == nil {
		t.Fatal("no Dark Pact")
	}
	if warlock.Pet != nil {
		t.Fatal("the warlock has a pet")
	}
	sim.Reset()
	if warlock.DarkPact.CanCast(sim, warlock.CurrentTarget) {
		t.Error("Dark Pact can be cast without a pet")
	}
}

// Suppression's hit goes on fire and shadow spells, not on the sheet's spell hit.
func TestSuppressionHitIsPerSpell(t *testing.T) {
	with, _ := petlessWarlock(t, AfflictionTalents)
	if with.Talents.Suppression == 0 {
		t.Fatal("AfflictionTalents has no Suppression")
	}
	want := float64(with.Talents.Suppression) * core.SpellHitRatingPerHitChance
	for _, spell := range []*core.Spell{with.ShadowBolt, with.Immolate, with.CurseOfAgony} {
		if spell != nil && spell.BonusHitRating != want {
			t.Errorf("spell %v bonus hit rating %v, want %v", spell.ActionID, spell.BonusHitRating, want)
		}
	}
	without, _ := petlessWarlock(t, "")
	if a, b := with.GetStat(stats.SpellHit), without.GetStat(stats.SpellHit); a != b {
		t.Errorf("sheet spell hit %v with Suppression, %v without", a, b)
	}
}
