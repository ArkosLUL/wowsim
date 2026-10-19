package dps

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Spell::EffectWeaponDmg runs at launch, which a missed, dodged or parried Obliterate never gets: with no Annihilation
// a landed one takes the knight's diseases off, an avoided one keeps them, and only a landed one procs Rime.
func TestObliterateTakesTheDiseasesOnlyWhenItLands(t *testing.T) {
	// the blood DPS knight with Rime's three points: a two-hander, so no off-hand strike takes them
	player := parityPlayer(talentsWith(BloodTalents, 1, 17, '3'), "p3_blood", noRotation)
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 1200
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 1},
	})
	sim.Reset()
	sim.PrePull()
	dk := sim.Raid.Parties[0].Players[0].(*DpsDeathknight)
	if dk.Talents.Annihilation != 0 || dk.FreezingFogAura == nil {
		t.Fatalf("Annihilation %d, Rime %d", dk.Talents.Annihilation, dk.Talents.Rime)
	}
	// no hit or expertise, so about one in seven is avoided
	dk.AddStatsDynamic(sim, stats.Stats{stats.MeleeHit: -dk.GetStat(stats.MeleeHit), stats.Expertise: -dk.GetStat(stats.Expertise)})
	target := sim.GetTargetUnit(0)
	m := &dk.Obliterate.SpellMetrics[target.UnitIndex]

	landed, avoided := 0, 0
	for avoided < 5 && sim.CurrentTime < 1000*time.Second {
		for !dk.Obliterate.CanCast(sim, target) {
			if sim.Step() {
				t.Fatal("the fight ended")
			}
		}
		dk.FrostFeverSpell.Dot(target).Apply(sim)
		dk.BloodPlagueSpell.Dot(target).Apply(sim)
		dk.FreezingFogAura.Deactivate(sim)
		before := *m
		if !dk.Obliterate.Cast(sim, target) {
			t.Fatal("Obliterate didn't cast")
		}
		missed := m.Misses+m.Dodges+m.Parries > before.Misses+before.Dodges+before.Parries
		if missed {
			avoided++
		} else {
			landed++
		}
		for _, dot := range []*core.Dot{dk.FrostFeverSpell.Dot(target), dk.BloodPlagueSpell.Dot(target)} {
			if dot.IsActive() != missed {
				t.Fatalf("%s up %v after an Obliterate avoided %v", dot.Spell.ActionID, dot.IsActive(), missed)
			}
		}
		if missed && dk.FreezingFogAura.IsActive() {
			t.Fatal("an avoided Obliterate procced Rime")
		}
	}
	if avoided < 5 {
		t.Errorf("%d Obliterates avoided of %d, want 5", avoided, avoided+landed)
	}
}

// The consume takes the knight's whole 55095 aura, its attack speed slow too, but not the raid's Frost Fever debuff,
// which stands in for another knight's.
func TestObliterateTakesTheFrostFeverSlow(t *testing.T) {
	for _, raid := range []bool{false, true} {
		var debuffs *proto.Debuffs
		if raid {
			debuffs = &proto.Debuffs{FrostFever: proto.TristateEffect_TristateEffectRegular}
		}
		// no Annihilation, so a landed Obliterate always takes them
		player := parityPlayer(BloodTalents, "p3_blood", noRotation)
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, debuffs),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 1},
		})
		sim.Reset()
		sim.PrePull()
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		dk := sim.Raid.Parties[0].Players[0].(*DpsDeathknight)
		if dk.Talents.Annihilation != 0 {
			t.Fatalf("Annihilation %d", dk.Talents.Annihilation)
		}
		dk.AddStatsDynamic(sim, stats.Stats{stats.MeleeHit: 50 * core.MeleeHitRatingPerHitChance, stats.Expertise: 200 * core.ExpertisePerQuarterPercentReduction})
		target := sim.GetTargetUnit(0)
		for !dk.Obliterate.CanCast(sim, target) {
			if sim.Step() {
				t.Fatal("the fight ended")
			}
		}
		dk.FrostFeverSpell.Cast(sim, target)
		slow := dk.FrostFeverDebuffAura.Get(target)
		if !slow.IsActive() {
			t.Fatal("no slow with his Frost Fever")
		}
		if !dk.Obliterate.Cast(sim, target) {
			t.Fatal("Obliterate didn't cast")
		}
		if dk.FrostFeverSpell.Dot(target).IsActive() || slow.IsActive() != raid {
			t.Errorf("raid debuff %v: his Frost Fever up %v and the slow %v after a landed Obliterate, want the slow only with the raid's",
				raid, dk.FrostFeverSpell.Dot(target).IsActive(), slow.IsActive())
		}
	}
}
