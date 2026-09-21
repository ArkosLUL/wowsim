package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func newTestOwner(class proto.Class, power PowerBarType, ownerStats stats.Stats) *Character {
	raid := &Raid{}
	owner := &Character{
		Class: class,
		Party: &Party{Raid: raid},
		Unit: Unit{
			Type:                  PlayerUnit,
			PseudoStats:           newPseudoStats(),
			currentPowerBar:       power,
			StatDependencyManager: stats.NewStatDependencyManager(),
		},
	}
	owner.stats = ownerStats
	return owner
}

func newTestPet(owner *Character) *Pet {
	pet := NewPet("Pet", owner, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)
	return &pet
}

// The scaling aura's amounts: the owner's hit chance rescaled to each stat's cap, truncated to a
// whole point.
func TestPetOwnerHitScaling(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		class                          proto.Class
		power                          PowerBarType
		ownerHit                       stats.Stats
		hitPct, spellHitPct, expertise float64
	}{
		{
			// Nothing caps the amounts: past 8% ranged hit the pet gets more than 17% spell hit.
			name:  "hunter over the ranged hit cap",
			class: proto.Class_ClassHunter, power: ManaBar,
			ownerHit:    stats.Stats{stats.MeleeHit: 8.5 * MeleeHitRatingPerHitChance},
			hitPct:      8,
			spellHitPct: 18,
			expertise:   27,
		},
		{
			name:  "hunter below it, every amount truncated",
			class: proto.Class_ClassHunter, power: ManaBar,
			// 7.2% -> 15.3% spell hit and 23.4 expertise
			ownerHit:    stats.Stats{stats.MeleeHit: 7.2 * MeleeHitRatingPerHitChance},
			hitPct:      7,
			spellHitPct: 15,
			expertise:   23,
		},
		{
			name:  "a hunter's spell hit doesn't count",
			class: proto.Class_ClassHunter, power: ManaBar,
			ownerHit:    stats.Stats{stats.MeleeHit: 3.4 * MeleeHitRatingPerHitChance, stats.SpellHit: 17 * SpellHitRatingPerHitChance},
			hitPct:      3,
			spellHitPct: 7,
			expertise:   11,
		},
		{
			name:  "a warlock's pet goes by spell hit, over a 17% cap",
			class: proto.Class_ClassWarlock, power: ManaBar,
			// 10% of a 17% cap -> 4.7% hit, 15.29 expertise
			ownerHit:    stats.Stats{stats.SpellHit: 10 * SpellHitRatingPerHitChance},
			hitPct:      4,
			spellHitPct: 10,
			expertise:   15,
		},
		{
			name:  "a death knight's goes by melee hit",
			class: proto.Class_ClassDeathknight, power: RunicPower,
			// 5% of an 8% cap -> 10.625% spell hit, 16.25 expertise
			ownerHit:    stats.Stats{stats.MeleeHit: 5 * MeleeHitRatingPerHitChance, stats.SpellHit: 17 * SpellHitRatingPerHitChance},
			hitPct:      5,
			spellHitPct: 10,
			expertise:   16,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pet := newTestPet(newTestOwner(tc.class, tc.power, tc.ownerHit))
			scaling := pet.ownerHitScaling()

			for _, got := range []struct {
				name       string
				have, want float64
			}{
				{"hit", scaling[stats.MeleeHit], tc.hitPct * MeleeHitRatingPerHitChance},
				{"spell hit", scaling[stats.SpellHit], tc.spellHitPct * SpellHitRatingPerHitChance},
				{"expertise", scaling[stats.Expertise], tc.expertise * ExpertisePerQuarterPercentReduction},
			} {
				if !WithinToleranceFloat64(got.want, got.have, 1e-6) {
					t.Errorf("%s: got %.4f, want %.4f", got.name, got.have, got.want)
				}
			}
		})
	}
}

// The haste carrier aura's amount is whole percent, and a slowed owner hands over nothing.
func TestPetInheritedSwingSpeed(t *testing.T) {
	for _, tc := range []struct {
		owner, want float64
	}{
		{1, 1},
		{0.8, 1},
		{1.15, 1.15},
		{1.2, 1.2},
		{1.5189343389857697, 1.51},
		{2.005, 2},
	} {
		if got := inheritedSwingSpeed(tc.owner); !WithinToleranceFloat64(tc.want, got, 1e-9) {
			t.Errorf("owner swing speed %.4f: got %.4f, want %.4f", tc.owner, got, tc.want)
		}
	}
}

// A pet avoids like the creature it is: a flat 5% dodge that agility and defense don't move, and no
// parry.
func TestPetAvoidance(t *testing.T) {
	pet := newTestPet(newTestOwner(proto.Class_ClassHunter, ManaBar, stats.Stats{}))
	if got := pet.DodgeChance(); got != CreatureDodgeChance {
		t.Errorf("dodge %.4f, want %.4f", got, CreatureDodgeChance)
	}

	pet.stats[stats.Agility] = 2000
	if got := pet.DodgeChance(); got != CreatureDodgeChance {
		t.Errorf("dodge with agility %.4f, want %.4f", got, CreatureDodgeChance)
	}
	if got := pet.ParryChance(); got != 0 {
		t.Errorf("parry %.4f, want 0", got)
	}
}

type timingTestPet struct{ Pet }

func (p *timingTestPet) GetPet() *Pet                        { return &p.Pet }
func (p *timingTestPet) Initialize()                         {}
func (p *timingTestPet) Reset(_ *Simulation)                 {}
func (p *timingTestPet) ExecuteCustomRotation(_ *Simulation) {}

// The haste carrier makes an inheriting pet immune to Bloodlust's cast speed as well as its attack
// speed, so the pet gets none of it. Any other pet takes both.
func TestBloodlustSkipsAPetThatInheritsHaste(t *testing.T) {
	for _, tc := range []struct {
		owner       proto.Class
		wantPetAura bool
	}{
		{proto.Class_ClassHunter, false},
		{proto.Class_ClassShaman, true},
	} {
		var pet *timingTestPet
		var bloodlust *Aura
		timingTestConstruct = func(a *timingTestAgent) {
			a.Class = tc.owner
			pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)}
			pet.SummonedAsPet = true
			a.AddPet(pet)
		}
		sim, _ := newTimingTestSim(t, 0, func(a *timingTestAgent) {
			bloodlust = BloodlustAura(&a.Character, -1)
		})
		timingTestConstruct = nil
		sim.PrePull()
		runUntil(sim, ms(100))
		castSpeed := pet.CastSpeed
		bloodlust.Activate(sim)

		petAura := pet.GetAura(bloodlust.Label)
		if got := petAura != nil && petAura.IsActive(); got != tc.wantPetAura {
			t.Errorf("%v owner: pet has Bloodlust = %v, want %v", tc.owner, got, tc.wantPetAura)
		}
		if hasted := pet.CastSpeed != castSpeed; hasted != tc.wantPetAura {
			t.Errorf("%v owner: pet cast speed %.4f -> %.4f, want it hasted = %v", tc.owner, castSpeed, pet.CastSpeed, tc.wantPetAura)
		}
	}
}
