package healing

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Power Word: Shield puts the buff (59887 to 59891) on, and spell_proc -59887 takes its one charge in the cast phase
// of the next untriggered magic-class spell, an instant like Renew too. Penance (53007) and Prayer of Mending (48113)
// have no damage class, so they leave the buff on.
func TestBorrowedTimeGoesWithTheNextMagicSpell(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsDisc,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: DiscTalents,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*HealingPriest)
	if priest.Talents.BorrowedTime != 5 || priest.PenanceHeal == nil {
		t.Fatalf("Borrowed Time %d, Penance %v", priest.Talents.BorrowedTime, priest.PenanceHeal != nil)
	}
	self := &priest.Unit
	buff := priest.GetAura("Borrowed Time")
	mana := priest.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen})
	cast := func(spell *core.Spell) {
		t.Helper()
		priest.AddMana(sim, priest.MaxMana()-priest.CurrentMana(), mana)
		priest.GCD.Reset()
		priest.WeakenedSouls.Get(self).Deactivate(sim)
		if !spell.Cast(sim, self) {
			t.Fatalf("%s didn't cast", spell.ActionID)
		}
		runFor(sim, 100*time.Millisecond)
	}

	for _, step := range []struct {
		name  string
		spell *core.Spell
		want  bool
	}{
		{"Power Word: Shield", priest.PowerWordShield, true},
		{"Penance", priest.PenanceHeal, true},
		{"Prayer of Mending", priest.PrayerOfMending, true},
		{"Renew", priest.Renew, false},
		{"Power Word: Shield", priest.PowerWordShield, true},
		{"Power Word: Shield again", priest.PowerWordShield, true},
	} {
		cast(step.spell)
		if got := buff.IsActive(); got != step.want {
			t.Errorf("buff up %v after %s, want %v", got, step.name, step.want)
		}
	}
	// Greater Heal's cast took the buff's haste, and its cast phase uses the charge
	cast(priest.GreaterHeal)
	if !buff.IsActive() {
		t.Fatal("the buff went before Greater Heal's cast ended")
	}
	runFor(sim, 3*time.Second)
	if buff.IsActive() {
		t.Error("the buff outlasted Greater Heal's cast")
	}
}
