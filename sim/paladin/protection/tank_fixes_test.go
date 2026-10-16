package protection

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/paladin"
)

// tankFixesPaladin is a lone ungeared Protection Paladin with the standard talents and glyphs and Avenging Wrath,
// under rotation, at 0 s of a fight on a dummy that never swings.
type tankFixesPaladin struct {
	sim     *core.Simulation
	pal     *ProtectionPaladin
	metrics *core.ResourceMetrics
	started bool
}

var tankFixesOptions = &proto.Player_ProtectionPaladin{ProtectionPaladin: &proto.ProtectionPaladin{
	Options: &proto.ProtectionPaladin_Options{
		Judgement:        proto.PaladinJudgement_JudgementOfWisdom,
		Seal:             proto.PaladinSeal_Vengeance,
		Aura:             proto.PaladinAura_RetributionAura,
		UseAvengingWrath: true,
	},
}}

func newTankFixesPaladin(t *testing.T, rotation *proto.APLRotation) *tankFixesPaladin {
	t.Helper()
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassPaladin,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          tankFixesOptions,
			TalentsString: StandardTalents,
			Glyphs:        StandardGlyphs,
			Rotation:      rotation,
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	p := &tankFixesPaladin{sim: sim, pal: sim.Raid.Parties[0].Players[0].(*ProtectionPaladin)}
	p.metrics = p.pal.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel})
	return p
}

// start resets the fight to 0 s with 50,000 max health, the given health and defense skill.
func (p *tankFixesPaladin) start(t *testing.T, health float64, defense int32) {
	t.Helper()
	sim, pal := p.sim, p.pal
	if p.started {
		sim.Cleanup()
	}
	p.started = true
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	pal.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - pal.MaxHealth()})
	pal.PseudoStats.BonusDefenseSkill += defense - core.MaxSkill(core.CharacterLevel) - pal.DefenseSkill()
	pal.GainHealth(sim, pal.MaxHealth(), p.metrics)
	if health < 50000 {
		pal.RemoveHealth(sim, 50000-health)
	}
	if pal.HealingTakenPctMultiplier() != 1 {
		t.Fatalf("healing taken %v, want 1", pal.HealingTakenPctMultiplier())
	}
}

// hit runs a hit of damage through the damage taken modifiers and takes off what is left.
func (p *tankFixesPaladin) hit(damage float64) {
	result := &core.SpellResult{Target: &p.pal.Unit, Damage: damage}
	for _, modifier := range p.pal.DynamicDamageTakenModifiers {
		modifier(p.sim, nil, result)
	}
	if result.Damage > 0 {
		p.pal.RemoveHealth(p.sim, result.Damage)
	}
}

func (p *tankFixesPaladin) expectHealth(t *testing.T, what string, want float64) {
	t.Helper()
	if got := p.pal.CurrentHealth(); got != want {
		t.Errorf("%s: health %v, want %v", what, got, want)
	}
}

// spell_pal_ardent_defender::Absorb at 3/3: 20% absorb, 30% heal scaled by defense skill over 540.
func TestArdentDefender(t *testing.T) {
	p := newTankFixesPaladin(t, core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`))
	if p.pal.Talents.ArdentDefender != 3 {
		t.Fatalf("Ardent Defender at %d, want 3", p.pal.Talents.ArdentDefender)
	}
	debuff := p.pal.GetAura("Ardent Defender")

	// lethal on the raw hit: absorbed whole, healed for 29% (uint32(30 x 535/540)) of 50,000
	p.start(t, 10000, 535)
	p.hit(11000)
	p.expectHealth(t, "a raw 11,000 hit at 10,000 health and 535 defense", 24500)
	if !debuff.IsActive() {
		t.Errorf("66233 isn't up after the save")
	}
	p.hit(30000)
	p.expectHealth(t, "a second lethal hit with 66233 up", 0)

	// already under 35%: the whole hit loses 20%
	p.start(t, 15000, 540)
	p.hit(5000)
	p.expectHealth(t, "a 5,000 hit at 15,000", 11000)

	// crossing 35% (17,500): only the 2,500 under it loses 20%
	p.start(t, 20000, 540)
	p.hit(5000)
	p.expectHealth(t, "a 5,000 hit at 20,000", 15500)

	p.start(t, 40000, 540)
	p.hit(5000)
	p.expectHealth(t, "a 5,000 hit ending over 35%", 35000)

	// the heal reads defense skill when it goes off, so a buff after talents counts
	p.start(t, 10000, 535)
	before := p.pal.DefenseSkill()
	p.pal.AddStatsDynamic(p.sim, stats.Stats{stats.Defense: 25})
	if p.pal.DefenseSkill() != before+5 {
		t.Fatalf("25 defense rating gave %d skill, want 5", p.pal.DefenseSkill()-before)
	}
	p.hit(11000)
	p.expectHealth(t, "the save at 540 defense", 25000)

	// no done percent or crit, but the healing taken percents count
	p.start(t, 10000, 540)
	p.pal.AddHealingTakenPct(20)
	p.hit(11000)
	p.pal.RemoveHealingTakenPct(20)
	p.expectHealth(t, "the save under a 20% healing taken aura", 28000)
}

// A boss swing Ardent Defender saves takes nothing off before the death tracking sees it, so with a healing
// model the tank lives and its health margin is the health it had going into the swing.
func TestArdentDefenderSaveIsSurvived(t *testing.T) {
	raid := core.SinglePlayerRaidProto(&proto.Player{
		Race:          proto.Race_RaceHuman,
		Class:         proto.Class_ClassPaladin,
		Equipment:     &proto.EquipmentSpec{},
		Consumes:      &proto.Consumes{},
		Spec:          tankFixesOptions,
		TalentsString: StandardTalents,
		Glyphs:        StandardGlyphs,
		Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		HealingModel:  &proto.HealingModel{BurstWindow: 6},
	}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{})
	raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101, SaveAllValues: true},
	})
	pal := sim.Raid.Parties[0].Players[0].(*ProtectionPaladin)
	saved := pal.GetAura("Ardent Defender")

	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	// any swing of the boss's that lands is lethal from 1,000
	pal.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - pal.MaxHealth()})
	pal.GainHealth(sim, pal.MaxHealth(), pal.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel}))
	pal.RemoveHealth(sim, 49000)
	for !saved.IsActive() && pal.CurrentHealth() > 0 && sim.CurrentTime < time.Minute && !sim.Step() {
	}
	if !saved.IsActive() {
		t.Fatalf("no save by %v, at %v health", sim.CurrentTime, pal.CurrentHealth())
	}
	if pal.CurrentHealth() <= 1000 {
		t.Errorf("health %v after the save, want the heal on top of 1000", pal.CurrentHealth())
	}
	sim.Cleanup()

	got := pal.Metrics.ToProto()
	if len(got.IterationDeaths) != 1 || got.IterationDeaths[0] {
		t.Errorf("deaths %v, want none", got.IterationDeaths)
	}
	if len(got.IterationHealthMargins) != 1 || got.IterationHealthMargins[0] != 1000 {
		t.Errorf("margins %v, want the 1000 before the swing", got.IterationHealthMargins)
	}
	if len(got.IterationFirstDeathSeconds) != 1 || got.IterationFirstDeathSeconds[0] != sim.Duration.Seconds() {
		t.Errorf("first deaths %v s, want the fight's %v", got.IterationFirstDeathSeconds, sim.Duration.Seconds())
	}
}

// Lay on Hands on himself: his max health, 1950 mana, 15 min with the glyph, and Forbearance and the Avenging
// Wrath marker both ways with Divine Protection and Avenging Wrath.
func TestLayOnHands(t *testing.T) {
	p := newTankFixesPaladin(t, core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`))
	sim, pal := p.sim, p.pal
	loh := pal.GetSpell(paladin.LayOnHandsActionID)
	if loh.CD.Duration != 15*time.Minute {
		t.Errorf("cooldown %v with the glyph, want 15m", loh.CD.Duration)
	}

	p.start(t, 10000, 540)
	pal.SpendMana(sim, pal.CurrentMana(), pal.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen}))
	mana := pal.CurrentMana()
	if !loh.Cast(sim, pal.CurrentTarget) {
		t.Fatal("Lay on Hands wasn't cast")
	}
	p.expectHealth(t, "Lay on Hands at 10,000", 50000)
	if got := pal.CurrentMana() - mana; got != 1950 {
		t.Errorf("Lay on Hands gave %v mana, want 1950", got)
	}
	if !pal.ForbearanceAura.IsActive() || pal.DivineProtection.CanCast(sim, pal.CurrentTarget) {
		t.Errorf("Divine Protection is castable after Lay on Hands")
	}

	p.start(t, 10000, 540)
	pal.DivineProtection.Cast(sim, pal.CurrentTarget)
	if loh.CanCast(sim, pal.CurrentTarget) {
		t.Errorf("Lay on Hands is castable after Divine Protection")
	}

	p.start(t, 10000, 540)
	pal.AvengingWrath.Cast(sim, pal.CurrentTarget)
	if loh.CanCast(sim, pal.CurrentTarget) {
		t.Errorf("Lay on Hands is castable inside Avenging Wrath's 30 s")
	}
}

// The default rotation casts Lay on Hands under 25% health, mod-playerbots' critical health.
func TestLayOnHandsInTheDefaultRotation(t *testing.T) {
	p := newTankFixesPaladin(t, core.GetAplRotation("../../../ui/protection_paladin/apls", "default").Rotation)
	loh := p.pal.GetSpell(paladin.LayOnHandsActionID)
	for _, tc := range []struct {
		health float64
		casts  int32
	}{
		{12600, 0}, // 25.2%
		{12400, 1}, // 24.8%
	} {
		p.start(t, tc.health, 540)
		for p.sim.CurrentTime < 2*time.Second && !p.sim.Step() {
		}
		if got := loh.SpellMetrics[p.pal.CurrentTarget.UnitIndex].Casts; got != tc.casts {
			t.Errorf("at %v health: %d casts, want %d", tc.health, got, tc.casts)
		}
	}
}
