package core

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func TestAddPctTruncatesLikeServer(t *testing.T) {
	for _, tc := range []struct {
		name            string
		base, pct, want float64
	}{
		{"no talent", 550, 0, 550},
		{"Improved Power Word: Fortitude", 165, 30, 214},
		{"Improved Devotion Aura", 1205, 50, 1807},
		{"Improved Mark of the Wild", 37, 40, 51},
		{"Commanding Presence on Battle Shout", 550, 25, 687},
		{"Improved Blessing of Might", 550, 25, 687},
		{"Improved Blessing of Wisdom", 92, 20, 110},
		{"Restorative Totems on Mana Spring", 91, 20, 109},
		{"Improved Demoralizing Shout", 411, 40, 575},
		{"Feral Aggression on Demoralizing Roar", 411, 40, 575},
		{"Sanctified Retribution on Retribution Aura", 112, 50, 168},
	} {
		if got := addPct(tc.base, tc.pct); got != tc.want {
			t.Errorf("%s: addPct(%v, %v) = %v, want %v", tc.name, tc.base, tc.pct, got, tc.want)
		}
	}
}

func buffTestUnit() *Unit {
	return &Unit{Type: PlayerUnit, Level: CharacterLevel, auraTracker: newAuraTracker()}
}

func buffTestTarget() *Unit {
	return &Unit{Type: EnemyUnit, Level: CharacterLevel + 3, auraTracker: newAuraTracker()}
}

// The amount an exclusive stat effect hands out is its priority.
func effectAmount(t *testing.T, aura *Aura, category string) float64 {
	t.Helper()
	for _, ee := range aura.ExclusiveEffects {
		if ee.Category.Name == category {
			return ee.Priority
		}
	}
	t.Fatalf("%s: no %s effect", aura.Label, category)
	return 0
}

func TestBuffAmountsMatchServer(t *testing.T) {
	for _, tc := range []struct {
		name     string
		aura     func() *Aura
		category string
		want     float64
	}{
		{"Battle Shout", func() *Aura { return BattleShoutAura(buffTestUnit(), 0, 0, false) }, "AttackPowerBonus", 550},
		{"Battle Shout with Commanding Presence", func() *Aura { return BattleShoutAura(buffTestUnit(), 5, 0, false) }, "AttackPowerBonus", 687},
		{"Blessing of Might", func() *Aura { return BlessingOfMightAura(buffTestUnit(), 0) }, "AttackPowerBonus", 550},
		{"Blessing of Might improved", func() *Aura {
			return BlessingOfMightAura(buffTestUnit(), int32(proto.TristateEffect_TristateEffectImproved))
		}, "AttackPowerBonus", 687},
		{"Commanding Shout", func() *Aura { return CommandingShoutAura(buffTestUnit(), 0, 0, false) }, "HealthBonus", 2255},
		{"Blood Pact", func() *Aura { return BloodPactAura(buffTestUnit(), 0) }, "HealthBonus", 1330},
		{"Blood Pact with Improved Imp", func() *Aura { return BloodPactAura(buffTestUnit(), 3) }, "HealthBonus", 1729},
	} {
		if got := effectAmount(t, tc.aura(), tc.category); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestDebuffAmountsMatchServer(t *testing.T) {
	for _, tc := range []struct {
		name string
		aura func() *Aura
		want float64
	}{
		{"Demoralizing Shout", func() *Aura { return DemoralizingShoutAura(buffTestTarget(), 0, 0) }, 411},
		{"Demoralizing Shout improved", func() *Aura { return DemoralizingShoutAura(buffTestTarget(), 0, 5) }, 575},
		{"Demoralizing Roar", func() *Aura { return DemoralizingRoarAura(buffTestTarget(), 0) }, 411},
		{"Demoralizing Roar with Feral Aggression", func() *Aura { return DemoralizingRoarAura(buffTestTarget(), 5) }, 575},
		{"Demoralizing Screech", func() *Aura { return DemoralizingScreechAura(buffTestTarget()) }, 574},
		{"Vindication", func() *Aura { return VindicationAura(buffTestTarget(), 2) }, 574},
		{"Curse of Weakness", func() *Aura { return CurseOfWeaknessAura(buffTestTarget(), 0) }, 478},
		{"Curse of Weakness improved", func() *Aura { return CurseOfWeaknessAura(buffTestTarget(), 2) }, 573},
	} {
		if got := effectAmount(t, tc.aura(), "APReduction"); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Booming Voice's class mask covers Demoralizing Shout as well as the two buff shouts.
func TestBoomingVoiceExtendsEveryShout(t *testing.T) {
	for _, tc := range []struct {
		name string
		aura func() *Aura
		want time.Duration
	}{
		{"Battle Shout", func() *Aura { return BattleShoutAura(buffTestUnit(), 0, 2, false) }, 3 * time.Minute},
		{"Commanding Shout", func() *Aura { return CommandingShoutAura(buffTestUnit(), 0, 2, false) }, 3 * time.Minute},
		{"Demoralizing Shout", func() *Aura { return DemoralizingShoutAura(buffTestTarget(), 2, 0) }, 45 * time.Second},
	} {
		if got := tc.aura().Duration; got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// AzerothCore caps a player's area damage at ten targets' worth, so the multiplier stays.
func TestAOECapMultiplier(t *testing.T) {
	for _, tc := range []struct {
		targets int
		want    float64
	}{
		{1, 1},
		{10, 1},
		{11, 10.0 / 11},
		{25, 0.4},
	} {
		options := &proto.Encounter{}
		for i := 0; i < tc.targets; i++ {
			options.Targets = append(options.Targets, &proto.Target{Level: CharacterLevel + 3})
		}
		encounter := NewEncounter(options, nil)
		if got := encounter.AOECapMultiplier(); got != tc.want {
			t.Errorf("%d targets: got %v, want %v", tc.targets, got, tc.want)
		}
	}
}
