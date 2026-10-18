package healing

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// spell_pri_glyph_of_prayer_of_healing gives 56161 int32(CalculatePct(int32(heal), 20) / 2) off the Prayer of
// Healing heal on that target, crit included, and each tick takes the healing done percents (Blessed
// Resilience's 3%) again.
func TestPrayerOfHealingGlyphHotComesOffTheHeal(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsHoly,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: BlessedResilienceTalents,
			Glyphs:        HolyGlyphs,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*HealingPriest)
	if priest.Talents.BlessedResilience != 3 || priest.Talents.FocusedPower != 0 {
		t.Fatalf("Blessed Resilience %d, Focused Power %d", priest.Talents.BlessedResilience, priest.Talents.FocusedPower)
	}
	glyph := priest.GetSpell(core.ActionID{ItemID: 42409})
	if glyph == nil {
		t.Fatal("no Glyph of Prayer of Healing")
	}

	target := &priest.Unit
	var heal float64
	crits := 0
	priest.RegisterAura(core.Aura{
		Label:    "glyph test",
		Duration: core.NeverExpires,
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == priest.PrayerOfHealing {
				heal = result.Damage
				if result.DidCrit() {
					crits++
				}
			}
		},
	}).Activate(sim)

	hot := glyph.Hot(target)
	metrics := &glyph.SpellMetrics[target.UnitIndex]
	for range 40 {
		priest.PrayerOfHealing.SkipCastAndApplyEffects(sim, target)
		if !hot.IsActive() {
			t.Fatal("the heal put no hot on")
		}
		before := metrics.TotalHealing
		hot.TickOnce(sim)
		bp := int32(float32(float32(heal)*20)/100) / 2
		if got, want := metrics.TotalHealing-before, float64(int32(float32(bp)*1.03)); got != want {
			t.Errorf("a tick healed %v after a %v heal, want %v", got, heal, want)
		}
		hot.Deactivate(sim)
	}
	if crits == 0 {
		t.Errorf("no crit in 40 heals")
	}
}
