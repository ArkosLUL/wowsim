package healing

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Power Word: Shield puts the buff (59887 to 59891) on, and spell_proc -59887 takes its one charge in the cast phase
// of the next untriggered magic-class spell, an instant like Renew or Inner Focus too. Penance (53007) and Prayer of
// Mending (48113) have no damage class, so they leave the buff on. The cast phase comes before the effects, so Hymn
// of Hope's channel (SPELL_ATTR5_SPELL_HASTE_AFFECTS_PERIODIC) ticks every 2 s, without the buff's 25%.
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
	hymnOfHope := priest.GetSpell(core.ActionID{SpellID: 64901})
	if priest.Talents.BorrowedTime != 5 || priest.PenanceHeal == nil || priest.InnerFocus == nil || hymnOfHope == nil {
		t.Fatalf("Borrowed Time %d, Penance %v, Inner Focus %v, Hymn of Hope %v", priest.Talents.BorrowedTime,
			priest.PenanceHeal != nil, priest.InnerFocus != nil, hymnOfHope != nil)
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
		{"Inner Focus", priest.InnerFocus, false},
		{"Power Word: Shield after Inner Focus", priest.PowerWordShield, true},
	} {
		cast(step.spell)
		if got := buff.IsActive(); got != step.want {
			t.Errorf("buff up %v after %s, want %v", got, step.name, step.want)
		}
	}
	// a hardcast keeps the buff until it lands
	cast(priest.GreaterHeal)
	if !buff.IsActive() {
		t.Fatal("the buff went before Greater Heal's cast ended")
	}
	runFor(sim, 3*time.Second)
	if buff.IsActive() {
		t.Error("the buff outlasted Greater Heal's cast")
	}

	cast(priest.PowerWordShield)
	priest.GCD.Reset()
	priest.SpendMana(sim, priest.CurrentMana()/2, mana)
	before, start := priest.CurrentMana(), sim.CurrentTime
	if !hymnOfHope.Cast(sim, self) {
		t.Fatal("Hymn of Hope didn't cast")
	}
	if buff.IsActive() {
		t.Error("the buff outlasted Hymn of Hope's cast")
	}
	runTo := func(at time.Duration) {
		core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: at, OnAction: func(*core.Simulation) {}})
		for sim.CurrentTime < at && !sim.Step() {
		}
	}
	tick := 0.036 * priest.MaxMana()
	runTo(start + 1800*time.Millisecond)
	if got := priest.CurrentMana() - before; got >= tick {
		t.Errorf("Hymn of Hope gave %.0f mana by 1.8 s, a tick hasted by the buff", got)
	}
	runTo(start + 2100*time.Millisecond)
	if got := priest.CurrentMana() - before; got < tick {
		t.Errorf("Hymn of Hope gave %.0f mana by 2.1 s, want its first tick of %.0f", got, tick)
	}
}
