package mage

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// A Mirror Image (31216) as Guardian::InitStatsForLevel and spell_mage_pet_scaling build it: the
// fallback create stats, a creature's flat 5% crit, and from the owner at the summon 30% of his
// stamina and intellect, 33% of his spell power and 35% of his armor.
func TestMirrorImageStats(t *testing.T) {
	sim, mage := newProcSim(t, procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane))
	image := mage.mirrorImage
	image.Enable(sim, image)

	owner := mage.GetStats()
	if owner[stats.SpellPower] < 1000 || owner[stats.Intellect] < 500 {
		t.Fatalf("owner spell power %v, intellect %v: the gear set isn't on", owner[stats.SpellPower], owner[stats.Intellect])
	}
	stamina := core.PetScalingPct(owner[stats.Stamina], 30)
	intellect := core.PetScalingPct(owner[stats.Intellect], 30)
	got := image.GetStats()
	for _, tc := range []struct {
		name       string
		have, want float64
	}{
		{"strength", got[stats.Strength], 22},
		{"agility", got[stats.Agility], 22},
		{"stamina", got[stats.Stamina], 25 + stamina},
		{"intellect", got[stats.Intellect], 28 + intellect},
		{"spirit", got[stats.Spirit], 27},
		{"health", image.MaxHealth(), 4032 + 10*stamina},
		{"mana", image.MaxMana(), 8814 + 15*intellect},
		{"armor", got[stats.Armor], 4000 + core.PetScalingPct(owner[stats.Armor], 35)},
		{"attack power", got[stats.AttackPower], 2*22 - 20},
		{"spell power", got[stats.SpellPower], core.PetScalingPct(owner[stats.SpellPower], 33)},
		{"spell crit rating", got[stats.SpellCrit], 5 * core.CritRatingPerCritChance},
	} {
		if !core.WithinToleranceFloat64(tc.want, tc.have, 1e-6) {
			t.Errorf("%s %v, want %v", tc.name, tc.have, tc.want)
		}
	}
	if image.BaseMana != 2428 {
		t.Errorf("create mana %v, want 2428", image.BaseMana)
	}
	// 59638 and 59637 cost a flat 90 and 120
	if bolt, blast := image.Frostbolt.DefaultCast.Cost, image.Fireblast.DefaultCast.Cost; bolt != 90 || blast != 120 {
		t.Errorf("Frostbolt costs %v and Fire Blast %v, want 90 and 120", bolt, blast)
	}
}

// A Mirror Image has no pet bar (SummonProperties 1021), so a raid cast skips it (Spell.cpp's
// TARGET_CHECK_RAID): neither the raid's Replenishment nor Bloodlust reaches it.
func TestMirrorImageTakesNoRaidCast(t *testing.T) {
	player := procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane)
	player.Buffs = core.FullIndividualBuffs
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	mage := sim.Raid.Parties[0].Players[0].(*Mage)
	if mage.ReplenishmentAura == nil || !mage.ReplenishmentAura.IsActive() {
		t.Fatal("the mage has no Replenishment from the raid, so the test shows nothing")
	}
	image := mage.mirrorImage
	image.Enable(sim, image)
	if image.ReplenishmentAura != nil {
		t.Error("a Mirror Image carries the raid's Replenishment")
	}

	core.BloodlustAura(&mage.Character, -1).Activate(sim)
	if !mage.HasActiveAuraWithTag(core.BloodlustAuraTag) {
		t.Fatal("the mage has no Bloodlust, so the test shows nothing")
	}
	if image.HasActiveAuraWithTag(core.BloodlustAuraTag) {
		t.Error("a Mirror Image out for Bloodlust has it too")
	}
}

// npc_pet_mage_mirror_image holds an image 1.2 s, then runs one event an update: Frostbolt put back 2.5 s and
// Fire Blast 6.5 s as each is cast, nothing while a cast is going. Its 29.5 s leave room for 9 Frostbolts of
// 3 s and 3 Fire Blasts. Images out before the mage fights open with Frostbolt or Fire Blast as CasterAI
// picks, and a Fire Blast opener makes 4. Images that come into a fight were engaged at the summon with
// nothing to open on (the target's threat list takes them in InitializeAI), so they start with a Frostbolt
// an update after the 1.2 s.
func TestMirrorImageCastsOffItsEventTimers(t *testing.T) {
	sim, mage := newProcSim(t, procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane))
	image := mage.mirrorImage
	target := sim.GetTargetUnit(0)
	if cast := image.Frostbolt.DefaultCast; cast.CastTime != 3*time.Second || cast.GCD != 0 || image.Fireblast.DefaultCast.GCD != 0 {
		t.Fatalf("Frostbolt takes %v with a GCD of %v, Fire Blast has one of %v: want 3 s and none", cast.CastTime, cast.GCD,
			image.Fireblast.DefaultCast.GCD)
	}

	var first time.Duration
	var blastFirst bool
	record := func(spell *core.Spell, blast bool) {
		effects := spell.ApplyEffects
		spell.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if first == 0 {
				first, blastFirst = sim.CurrentTime, blast
			}
			effects(sim, target, spell)
		}
	}
	record(image.Frostbolt, false)
	record(image.Fireblast, true)

	for _, tc := range []struct {
		name     string
		summonAt time.Duration
		opens    bool
	}{
		{"out before the fight", 0, true},
		{"summoned into a fight", 2 * time.Second, false},
	} {
		openers := map[bool]int{}
		for life := 0; life < 40; life++ {
			// newProcSim hands the sim over reset
			sim.Cleanup()
			sim.Reseed(int64(1000 + life))
			sim.Reset()
			sim.PrePull()
			for sim.CurrentTime < tc.summonAt && !sim.Step() {
			}
			summoned := sim.CurrentTime
			first = 0
			image.summon(sim)
			for image.IsEnabled() && !sim.Step() {
			}
			if lived := sim.CurrentTime - summoned; lived <= 29397*time.Millisecond || lived > 29610*time.Millisecond {
				t.Fatalf("%s, life %d: the images lasted %v, want 29.5 s within one update", tc.name, life, lived)
			}

			bolts := image.Frostbolt.SpellMetrics[target.UnitIndex].Casts
			blasts := image.Fireblast.SpellMetrics[target.UnitIndex].Casts
			wantBlasts := int32(3)
			at := first - summoned
			switch {
			case blastFirst && !tc.opens:
				t.Fatalf("%s, life %d: opened with Fire Blast %v in, want no opener", tc.name, life, at)
			case blastFirst:
				wantBlasts = 4
				// its opener goes off as it attacks, 12 updates in
				if at <= 1129*time.Millisecond || at > 1233*time.Millisecond {
					t.Fatalf("%s, life %d: opened with Fire Blast %v in, want 12 updates less the wait", tc.name, life, at)
				}
			case !tc.opens:
				// the update that ends the wait only picks the target, so its 3 s start on the next
				if at <= 4313*time.Millisecond || at > 4417*time.Millisecond {
					t.Fatalf("%s, life %d: the first Frostbolt landed %v in, want 43 updates less the wait", tc.name, life, at)
				}
			}
			if bolts != 9 || blasts != wantBlasts {
				t.Fatalf("%s, life %d, Fire Blast first %v: %d Frostbolts and %d Fire Blasts, want 9 and %d", tc.name, life, blastFirst,
					bolts, blasts, wantBlasts)
			}
			openers[blastFirst]++
		}
		if tc.opens && (openers[true] < 8 || openers[false] < 8) {
			t.Errorf("%s: of 40 summons %d opened with Fire Blast and %d with Frostbolt, want about half each", tc.name, openers[true],
				openers[false])
		}
	}
}
