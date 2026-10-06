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
	return NewPet("Pet", owner, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)
}

// The scaling aura's amounts: the owner's hit chance rescaled to each stat's cap, truncated to a
// whole point.
func TestPetOwnerHitScaling(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		class                          proto.Class
		power                          PowerBarType
		scaling                        PetHitScaling
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
		{
			name:  "a risen ghoul's 67561 hands out no melee hit",
			class: proto.Class_ClassDeathknight, power: RunicPower, scaling: PetHitScalingMasterSpell06,
			ownerHit:    stats.Stats{stats.MeleeHit: 5 * MeleeHitRatingPerHitChance, stats.SpellHit: 17 * SpellHitRatingPerHitChance},
			hitPct:      0,
			spellHitPct: 10,
			expertise:   16,
		},
		{
			name:  "nor does it cap past 8% melee hit",
			class: proto.Class_ClassDeathknight, power: RunicPower, scaling: PetHitScalingMasterSpell06,
			// 9% of an 8% cap -> 19.125% spell hit, 29.25 expertise
			ownerHit:    stats.Stats{stats.MeleeHit: 9 * MeleeHitRatingPerHitChance},
			hitPct:      0,
			spellHitPct: 19,
			expertise:   29,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pet := newTestPet(newTestOwner(tc.class, tc.power, tc.ownerHit))
			pet.HitScaling = tc.scaling
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

// Unit::CalcArmorReducedDamage hands the owner's armor pen to a hunter's pet and to any risen ghoul,
// Raise Dead's guardian included, but not to the army or a hunter's guardians.
func TestPetArmorPenInheritance(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		class                proto.Class
		summonedAsPet, ghoul bool
		want                 bool
	}{
		{"hunter pet", proto.Class_ClassHunter, true, false, true},
		{"hunter guardian", proto.Class_ClassHunter, false, false, false},
		{"permanent ghoul", proto.Class_ClassDeathknight, true, true, true},
		{"raise dead ghoul", proto.Class_ClassDeathknight, false, true, true},
		{"army ghoul", proto.Class_ClassDeathknight, false, false, false},
		{"warlock pet", proto.Class_ClassWarlock, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pet := newTestPet(newTestOwner(tc.class, ManaBar, stats.Stats{}))
			pet.SummonedAsPet = tc.summonedAsPet
			pet.RisenGhoul = tc.ghoul
			pet.inheritOwnerArmorPen()
			if got := pet.armorPenSource == &pet.Owner.Unit; got != tc.want {
				t.Errorf("inherits the owner's armor pen = %v, want %v", got, tc.want)
			}
		})
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

type timingTestPet struct{ *Pet }

func (p *timingTestPet) GetPet() *Pet                        { return p.Pet }
func (p *timingTestPet) Initialize()                         {}
func (p *timingTestPet) Reset(_ *Simulation)                 {}
func (p *timingTestPet) ExecuteCustomRotation(_ *Simulation) {}

// The haste carrier makes an inheriting pet immune to Bloodlust's cast speed as well as its attack
// speed, so the pet gets none of it. Any other pet takes both, guardian or real pet alike.
func TestBloodlustSkipsAPetThatInheritsHaste(t *testing.T) {
	for _, tc := range []struct {
		name         string
		hasteCarrier bool
		isGuardian   bool
		wantPetAura  bool
	}{
		{name: "haste carrier", hasteCarrier: true, wantPetAura: false},
		{name: "plain pet", wantPetAura: true},
		{name: "plain guardian", isGuardian: true, wantPetAura: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pet *timingTestPet
			var bloodlust *Aura
			timingTestConstruct = func(a *timingTestAgent) {
				pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, tc.isGuardian)}
				pet.SummonedAsPet = true
				pet.HasteCarrier = tc.hasteCarrier
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
				t.Errorf("pet has Bloodlust = %v, want %v", got, tc.wantPetAura)
			}
			if hasted := pet.CastSpeed != castSpeed; hasted != tc.wantPetAura {
				t.Errorf("pet cast speed %.4f -> %.4f, want it hasted = %v", castSpeed, pet.CastSpeed, tc.wantPetAura)
			}
		})
	}
}

// a heal on an undamaged summon gains nothing, so it makes no heal threat
func TestPetSummonsAtFullHealth(t *testing.T) {
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{stats.Health: 1000},
			func(stats.Stats) stats.Stats { return stats.Stats{stats.Health: 500} }, true, false)}
		a.AddPet(pet)
	}
	sim, _ := newTimingTestSim(t, 0, nil)
	timingTestConstruct = nil
	sim.PrePull()
	runUntil(sim, ms(100))

	if !pet.HasHealthBar() || !pet.IsEnabled() {
		t.Fatalf("health bar %v, enabled %v", pet.HasHealthBar(), pet.IsEnabled())
	}
	if pet.MaxHealth() <= 1000 || pet.CurrentHealth() != pet.MaxHealth() {
		t.Errorf("health %v of %v, want full with the inherited 500 in it", pet.CurrentHealth(), pet.MaxHealth())
	}
}

// CalculatePct on a truncated owner value, the result truncated too.
func TestPetScalingPct(t *testing.T) {
	for _, tc := range []struct {
		owner, pct, want float64
	}{
		{520.9, 57, 296},
		{520, 300, 1560},
		{134, 30, 40},
		{1103, 12.87, 141},
		{-5, 30, 0},
	} {
		if got := PetScalingPct(tc.owner, tc.pct); got != tc.want {
			t.Errorf("%v at %v%%: got %v, want %v", tc.owner, tc.pct, got, tc.want)
		}
	}
}

// Creature::Regenerate: spirit/5 + 17 a tick, under the five second rule spirit/8 + 17 for the Imp
// and the Water Elemental and nothing for the rest.
func TestCreatureManaRegen(t *testing.T) {
	for _, tc := range []struct {
		whileCasting, underFSR bool
		want                   float64
	}{
		{false, false, 90},
		{true, false, 90},
		{true, true, 62},
		{false, true, 0},
	} {
		pet := newTestPet(newTestOwner(proto.Class_ClassWarlock, ManaBar, stats.Stats{}))
		pet.stats[stats.Spirit] = 367.6
		pet.regensWhileCasting = tc.whileCasting
		if got := pet.CreatureManaRegen(tc.underFSR); got != tc.want {
			t.Errorf("while casting %v, under the rule %v: got %v, want %v", tc.whileCasting, tc.underFSR, got, tc.want)
		}
	}
}

// A scaling script's immunities drop the buffs of those aura types. An uncontrollable guardian only
// loses the party and raid casts, not the area auras.
func TestPetDropsBuffsItCannotTake(t *testing.T) {
	buffs := func() (*proto.RaidBuffs, *proto.PartyBuffs, *proto.IndividualBuffs) {
		return &proto.RaidBuffs{
				HornOfWinter:         true,
				StrengthOfEarthTotem: proto.TristateEffect_TristateEffectRegular,
				BattleShout:          proto.TristateEffect_TristateEffectRegular,
				TrueshotAura:         true,
				FlametongueTotem:     true,
				DevotionAura:         proto.TristateEffect_TristateEffectRegular,
				LeaderOfThePack:      proto.TristateEffect_TristateEffectRegular,
			}, &proto.PartyBuffs{}, &proto.IndividualBuffs{
				BlessingOfKings: true,
			}
	}
	pet := newTestPet(newTestOwner(proto.Class_ClassWarlock, ManaBar, stats.Stats{}))

	pet.BuffImmunity = PetImmuneToScaling
	raid, party, individual := buffs()
	pet.dropBuffsItCannotTake(raid, party, individual)
	if raid.HornOfWinter || raid.StrengthOfEarthTotem != 0 || raid.BattleShout != 0 || raid.TrueshotAura ||
		raid.FlametongueTotem || raid.DevotionAura != 0 || individual.BlessingOfKings {
		t.Errorf("immune to its scaling's types, kept %v %v", raid, individual)
	}
	if raid.LeaderOfThePack == 0 {
		t.Error("immune to its scaling's types, lost Leader of the Pack")
	}

	pet.BuffImmunity = 0
	pet.Uncontrollable = true
	raid, party, individual = buffs()
	pet.dropBuffsItCannotTake(raid, party, individual)
	if raid.HornOfWinter || raid.BattleShout != 0 || individual.BlessingOfKings {
		t.Errorf("uncontrollable, kept a party or raid cast: %v %v", raid, individual)
	}
	if raid.StrengthOfEarthTotem == 0 || !raid.TrueshotAura || !raid.FlametongueTotem || raid.DevotionAura == 0 {
		t.Errorf("uncontrollable, lost an area aura: %v", raid)
	}
}
