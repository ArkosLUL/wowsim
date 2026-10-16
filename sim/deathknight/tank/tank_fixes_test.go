package tank

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// newTankFixesDK is a lone ungeared Blood tank with no rotation at 0 s and 50,000 max health, at full.
func newTankFixesDK(t *testing.T) (*core.Simulation, *TankDeathknight, func()) {
	t.Helper()
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassDeathknight,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsBloodTank,
			TalentsString: BloodTankTalents,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	dk := sim.Raid.Parties[0].Players[0].(*TankDeathknight)
	metrics := dk.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel})
	started := false
	start := func() {
		if started {
			sim.Cleanup()
		}
		started = true
		sim.Reset()
		sim.PrePull()
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		dk.AddStatsDynamic(sim, stats.Stats{stats.Health: 50000 - dk.MaxHealth()})
		dk.GainHealth(sim, dk.MaxHealth(), metrics)
	}
	return sim, dk, start
}

// Vampiric Blood's 7,500 (15%) comes off on expiry as AuraEffect::HandleAuraModIncreaseHealth takes it off.
func TestVampiricBloodBonusHealthOnExpiry(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	if dk.VampiricBloodAura == nil {
		t.Fatal("no Vampiric Blood")
	}
	// health with the bonus up
	for _, tc := range []struct{ health, want float64 }{
		{57500, 50000},
		{15000, 7500},
		{7500, 1},
		{0, 0},
	} {
		start()
		dk.VampiricBloodAura.Activate(sim)
		if dk.MaxHealth() != 57500 || dk.CurrentHealth() != 57500 {
			t.Fatalf("%v of %v health with Vampiric Blood up, want 57500 of 57500", dk.CurrentHealth(), dk.MaxHealth())
		}
		if tc.health < 57500 {
			dk.RemoveHealth(sim, 57500-tc.health)
		}
		dk.VampiricBloodAura.Deactivate(sim)
		if dk.CurrentHealth() != tc.want || dk.MaxHealth() != 50000 {
			t.Errorf("at %v: %v of %v health after it, want %v of 50000", tc.health, dk.CurrentHealth(), dk.MaxHealth(), tc.want)
		}
	}
}

// 56816 procs 56817 on each dodge or parry: Spell::HandleThreatSpells hands its level, 67, to the boss, and with
// no school no threat aura touches it, Frost Presence's included.
func TestRuneStrikeProcThreat(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	start()
	dk.FrostPresenceAura.Activate(sim)
	if dk.PseudoStats.ThreatMultiplier == 1 {
		t.Fatal("Frost Presence left the threat multiplier at 1")
	}
	trigger := dk.GetAura("Rune Strike Trigger")
	for _, outcome := range []core.HitOutcome{core.OutcomeDodge, core.OutcomeParry, core.OutcomeHit} {
		trigger.OnSpellHitTaken(trigger, sim, nil, &core.SpellResult{Target: &dk.Unit, Outcome: outcome})
	}
	if got := dk.GainThreat(core.ActionID{SpellID: 56817}); got != 2*67 {
		t.Errorf("a dodge and a parry made %v threat, want 134", got)
	}
}

// spell_dk_death_strike: 5% of max health a disease, plus Improved Death Strike's effect 2, 50% at 2/2.
func TestImprovedDeathStrikeHeal(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	if dk.Talents.ImprovedDeathStrike != 2 {
		t.Fatalf("Improved Death Strike at %d, want 2", dk.Talents.ImprovedDeathStrike)
	}
	start()
	target := dk.CurrentTarget
	dk.FrostFeverSpell.Dot(target).Activate(sim)
	dk.BloodPlagueSpell.Dot(target).Activate(sim)
	// nothing between the strike and landing
	dk.AddStatsDynamic(sim, stats.Stats{stats.MeleeHit: 1000, stats.Expertise: 1000})

	dk.DeathStrike.SkipCastAndApplyEffects(sim, target)
	if n := len(dk.DeathStrikeHeals); n != 1 || dk.DeathStrikeHeals[0] != 7500 {
		t.Errorf("Death Strike heals %v, want [7500]", dk.DeathStrikeHeals)
	}
}
