package dps

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// thassarianSim is the frost knight in p3_frost (Razorice on the main hand) with hit and expertise past every cap, so
// each main-hand strike lands, and a hook that tells each hit he deals.
func thassarianSim(t *testing.T, onHit func(*core.Spell, *core.SpellResult)) (*core.Simulation, *DpsDeathknight) {
	t.Helper()
	player := parityPlayer(FrostTalents, "p3_frost", noRotation)
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 1200
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 7},
	})
	dk := sim.Raid.Parties[0].Players[0].(*DpsDeathknight)
	if dk.Talents.ThreatOfThassarian != 3 || dk.Talents.KillingMachine == 0 || dk.Talents.Rime == 0 {
		t.Fatalf("Threat of Thassarian %d, Killing Machine %d, Rime %d", dk.Talents.ThreatOfThassarian, dk.Talents.KillingMachine, dk.Talents.Rime)
	}
	dk.RegisterAura(core.Aura{
		Label:    "hit order",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			onHit(spell, result)
		},
	})
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	dk.AddStatsDynamic(sim, stats.Stats{stats.MeleeHit: 50 * core.MeleeHitRatingPerHitChance, stats.Expertise: 200 * core.ExpertisePerQuarterPercentReduction})
	return sim, dk
}

// spell_dk_threat_of_thassarian casts the off hand's strike from the main hand's hit procs, in spell id order: after
// Rime (59057) and the strike's launch (Blood Plague), ahead of the main hand's weapon enchants
// (Player::CastItemCombatSpell, Razorice's Razor Frost) and Killing Machine's charge, which goes at the cast's finish.
func TestThreatOfThassarianStrikeOrder(t *testing.T) {
	razorFrost := core.ActionID{SpellID: 50401}

	// the hits dealt during one cast, the off hand's before the main hand's Razor Frost
	checkOffHandFirst := func(t *testing.T, hits []*core.Spell, offHand *core.Spell) {
		t.Helper()
		oh := slices.Index(hits, offHand)
		rf := slices.IndexFunc(hits, func(s *core.Spell) bool { return s.ActionID == razorFrost })
		if oh < 0 || rf < 0 || rf < oh {
			names := make([]string, len(hits))
			for i, s := range hits {
				names[i] = s.ActionID.String()
			}
			t.Errorf("hits %v: the off hand's strike should come before the main hand's Razor Frost", names)
		}
	}

	t.Run("Frost Strike", func(t *testing.T) {
		var hits []*core.Spell
		ohCrit, kmAtOffHand := false, false
		var dk *DpsDeathknight
		var sim *core.Simulation
		sim, dk = thassarianSim(t, func(spell *core.Spell, result *core.SpellResult) {
			hits = append(hits, spell)
			if dk != nil && spell == dk.FrostStrikeOhHit {
				ohCrit, kmAtOffHand = result.DidCrit(), dk.KillingMachineAura.IsActive()
			}
		})
		target := sim.GetTargetUnit(0)
		dk.KillingMachineAura.Activate(sim)
		hits = hits[:0]
		if !dk.FrostStrike.Cast(sim, target) {
			t.Fatal("Frost Strike didn't cast")
		}
		checkOffHandFirst(t, hits, dk.FrostStrikeOhHit)
		if !kmAtOffHand || !ohCrit {
			t.Errorf("the off hand's Frost Strike dealt with Killing Machine up %v, crit %v; want both", kmAtOffHand, ohCrit)
		}
		if dk.KillingMachineAura.IsActive() {
			t.Error("Killing Machine still up after the Frost Strike")
		}
	})

	t.Run("Rune Strike", func(t *testing.T) {
		var hits []*core.Spell
		sim, dk := thassarianSim(t, func(spell *core.Spell, _ *core.SpellResult) { hits = append(hits, spell) })
		target := sim.GetTargetUnit(0)
		dk.RuneStrikeAura.Activate(sim)
		hits = hits[:0]
		if !dk.RuneStrike.Cast(sim, target) {
			t.Fatal("Rune Strike didn't cast")
		}
		checkOffHandFirst(t, hits, dk.RuneStrikeOh)
	})

	t.Run("Plague Strike", func(t *testing.T) {
		bloodPlagueAtOffHand := false
		var dk *DpsDeathknight
		var sim *core.Simulation
		sim, dk = thassarianSim(t, func(spell *core.Spell, result *core.SpellResult) {
			if dk != nil && spell == dk.PlagueStrikeOhHit {
				bloodPlagueAtOffHand = dk.BloodPlagueSpell.Dot(result.Target).IsActive()
			}
		})
		target := sim.GetTargetUnit(0)
		for range 3 {
			for !dk.PlagueStrike.CanCast(sim, target) {
				if sim.Step() {
					t.Fatal("the fight ended")
				}
			}
			dk.BloodPlagueSpell.Dot(target).Deactivate(sim)
			bloodPlagueAtOffHand = false
			if !dk.PlagueStrike.Cast(sim, target) {
				t.Fatal("Plague Strike didn't cast")
			}
			if !bloodPlagueAtOffHand {
				t.Fatal("Blood Plague wasn't up when the off hand's Plague Strike landed")
			}
		}
	})

	t.Run("Obliterate", func(t *testing.T) {
		rimeAtOffHand := false
		var dk *DpsDeathknight
		var sim *core.Simulation
		sim, dk = thassarianSim(t, func(spell *core.Spell, _ *core.SpellResult) {
			if dk != nil && spell == dk.ObliterateOhHit {
				rimeAtOffHand = dk.FreezingFogAura.IsActive()
			}
		})
		target := sim.GetTargetUnit(0)
		rimes := 0
		for casts := 0; rimes < 3 && casts < 200; casts++ {
			for !dk.Obliterate.CanCast(sim, target) {
				if sim.Step() {
					t.Fatal("the fight ended")
				}
			}
			dk.FreezingFogAura.Deactivate(sim)
			rimeAtOffHand = false
			if !dk.Obliterate.Cast(sim, target) {
				t.Fatal("Obliterate didn't cast")
			}
			if dk.FreezingFogAura.IsActive() {
				rimes++
				if !rimeAtOffHand {
					t.Fatalf("Rime procced at %v, after the off hand's Obliterate", sim.CurrentTime.Round(time.Millisecond))
				}
			}
		}
		if rimes < 3 {
			t.Errorf("Rime procced %d times, want 3", rimes)
		}
	})
}
