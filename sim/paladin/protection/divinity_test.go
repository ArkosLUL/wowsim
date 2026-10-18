package protection

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/paladin"
)

// Divinity 5/5 on the standard build. Its aura 118 is a healing taken percent, so it doesn't add to Improved
// Devotion Aura's: Unit::SpellHealingBonusTaken takes the larger. Lay on Hands and Ardent Defender's 66235 take it.
func TestDivinity(t *testing.T) {
	newPaladin := func(talents string, aura proto.PaladinAura) *tankFixesPaladin {
		options := &proto.Player_ProtectionPaladin{ProtectionPaladin: &proto.ProtectionPaladin{
			Options: &proto.ProtectionPaladin_Options{Aura: aura},
		}}
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceHuman,
				Class:         proto.Class_ClassPaladin,
				Equipment:     &proto.EquipmentSpec{},
				Consumes:      &proto.Consumes{},
				Spec:          options,
				TalentsString: talents,
				Glyphs:        StandardGlyphs,
				Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		p := &tankFixesPaladin{sim: sim, pal: sim.Raid.Parties[0].Players[0].(*ProtectionPaladin)}
		p.metrics = p.pal.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel})
		if p.pal.Talents.Divinity != 5 {
			t.Fatalf("Divinity at %d, want 5", p.pal.Talents.Divinity)
		}
		return p
	}
	// at 0 s with 50,000 max health, the given health and 540 defense skill
	start := func(p *tankFixesPaladin, health float64) {
		sim, pal := p.sim, p.pal
		sim.Reset()
		sim.PrePull()
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		pal.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - pal.MaxHealth()})
		pal.PseudoStats.BonusDefenseSkill += 540 - core.MaxSkill(core.CharacterLevel) - pal.DefenseSkill()
		pal.GainHealth(sim, pal.MaxHealth(), p.metrics)
		pal.RemoveHealth(sim, 50000-health)
	}

	p := newPaladin("-5"+StandardTalents[2:], proto.PaladinAura_RetributionAura)
	if got := p.pal.HealingTakenPctMultiplier(); got != 1.05 {
		t.Errorf("healing taken x%v, want x1.05", got)
	}

	start(p, 10000)
	loh := p.pal.GetSpell(paladin.LayOnHandsActionID)
	if !loh.Cast(p.sim, p.pal.CurrentTarget) {
		t.Fatal("Lay on Hands wasn't cast")
	}
	if got := loh.SpellMetrics[p.pal.UnitIndex].TotalHealing; got != 52500 {
		t.Errorf("Lay on Hands healed %v, want 52500", got)
	}
	p.sim.Cleanup()

	// a lethal 11,000 at 10,000: 30% of 50,000, then 5% more
	start(p, 10000)
	p.hit(11000)
	p.expectHealth(t, "the save", 25750)
	p.sim.Cleanup()

	// Improved Devotion Aura at 0 to 3 points, 2% a point
	for points, want := range []float64{1.05, 1.05, 1.05, 1.06} {
		p = newPaladin("-5000000000"+string(rune('0'+points)), proto.PaladinAura_DevotionAura)
		if got := p.pal.HealingTakenPctMultiplier(); p.pal.Talents.ImprovedDevotionAura != int32(points) || got != want {
			t.Errorf("Improved Devotion Aura %d/3: healing taken x%v, want x%v", p.pal.Talents.ImprovedDevotionAura, got, want)
		}
	}
}
