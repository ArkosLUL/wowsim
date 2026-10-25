package restoration

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// With incoming damage Earth Shield's charges come off the tank dummy's hits (one every 2 s), 379's 3.5 s cooldown
// letting every other one through. The stand-in rate (ppm) is off.
func TestEarthShieldChargesFromTheDummiesHits(t *testing.T) {
	for _, c := range []struct {
		at   time.Duration
		want int32
	}{
		{1900 * time.Millisecond, 0},
		{3 * time.Second, 1},
		// hits at 2, 4, 6, 8, 10 s, of which 2, 6 and 10 get through
		{11 * time.Second, 3},
	} {
		sim, shaman := healSetup{ppm: 60, dummies: 1, damage: 1}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		shaman.EarthShield.SkipCastAndApplyEffects(sim, target)
		runUntil(sim, c.at)
		if got := charges(shaman, target); got != c.want {
			t.Errorf("%d charges by %s, want %d", got, c.at, c.want)
		}
	}
}

func TestEarthShieldEndsWithItsLastChargeOnTheDummiesHits(t *testing.T) {
	sim, shaman := healSetup{dummies: 1, damage: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	shaman.EarthShield.SkipCastAndApplyEffects(sim, target)
	runUntil(sim, 120*time.Second)
	if got, want := charges(shaman, target), 6+shaman.Talents.ImprovedEarthShield; got != want {
		t.Errorf("%d charges, want %d", got, want)
	}
	if shaman.EarthShield.Hot(target).IsActive() {
		t.Error("Earth Shield is still up after its last charge")
	}
}

// Chain Heal jumps to the hurt dummies by largest missing health, and stops when nobody else is hurt.
func TestChainHealJumpsToTheHurtDummies(t *testing.T) {
	sim, shaman := healSetup{glyphs: StandardGlyphs, dummies: 4, damage: 1}.build(t)
	order := watchHeals(shaman, func() *core.Spell { return shaman.ChainHeal })
	startFight(sim)
	dummies := hurtDummies(sim, 0, 0, 900, 600)
	if !shaman.ChainHeal.Cast(sim, dummies[0]) {
		t.Fatal("Chain Heal wasn't cast")
	}
	runUntil(sim, 3*time.Second)
	sim.Cleanup()
	want := []*core.Unit{dummies[0], dummies[2], dummies[3]}
	if !slices.Equal(*order, want) {
		t.Errorf("healed %v, want %v", unitLabels(*order), unitLabels(want))
	}
}

// Ancestral Awakening goes to the dummy lowest in health percent.
func TestAncestralAwakeningHealsTheMostHurtDummy(t *testing.T) {
	sim, shaman := healSetup{dummies: 3, damage: 1}.build(t)
	startFight(sim)
	dummies := hurtDummies(sim, 0, 300, 500)
	var healed []*core.Unit
	shaman.RegisterAura(core.Aura{
		Label:    "ancestral awakening target",
		Duration: core.NeverExpires,
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == shaman.AncestralAwakening {
				healed = append(healed, result.Target)
			}
		},
	}).Activate(sim)
	for casts := 0; len(healed) == 0; casts++ {
		if casts == 500 {
			t.Fatal("no Ancestral Awakening in 500 Healing Waves")
		}
		shaman.HealingWave.SkipCastAndApplyEffects(sim, dummies[0])
	}
	if healed[0] != dummies[2] {
		t.Errorf("healed %v, want %v", unitLabels(healed[:1]), unitLabels(dummies[2:3]))
	}
}

func defaultAPL(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../../ui/restoration_shaman/apls/default.apl.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The default APL keeps Earth Shield on the tank, not on the current target.
func TestDefaultAPLShieldsTheTank(t *testing.T) {
	sim, shaman := healSetup{dummies: 3, damage: 1, apl: defaultAPL(t), talents: StandardTalents, glyphs: StandardGlyphs}.build(t)
	dummies := targetDummies(sim)
	shaman.CurrentTarget = dummies[2]
	startFight(sim)
	runUntil(sim, 3*time.Second)
	if !shaman.EarthShield.Hot(dummies[0]).IsActive() {
		t.Error("Earth Shield isn't on the tank")
	}
	if shaman.EarthShield.Hot(dummies[2]).IsActive() {
		t.Error("Earth Shield is on the current target")
	}
}

// The default APL waits while nobody is missing much, then picks a heal by how much the most hurt dummy is missing:
// Riptide from 1500, Lesser Healing Wave from 2500 and Chain Heal from 4500. A Riptide already on the dummy
// leaves the choice to the other two.
func TestDefaultAPLSizesTheHealToTheMissingHealth(t *testing.T) {
	for _, c := range []struct {
		name    string
		missing float64
		riptide bool
		want    string
	}{
		{"nobody hurt", 1000, false, ""},
		{"a little hurt", 2000, false, "Riptide"},
		{"covered and a little hurt", 2000, true, ""},
		{"medium", 3000, true, "Lesser Healing Wave"},
		{"short of Chain Heal", 4400, true, "Lesser Healing Wave"},
		{"badly hurt", 5000, true, "Chain Heal"},
	} {
		sim, shaman := healSetup{dummies: 3, damage: 1, apl: defaultAPL(t), talents: StandardTalents, glyphs: StandardGlyphs}.build(t)
		startFight(sim)
		dummies := hurtDummies(sim, 0, c.missing)
		if c.riptide {
			shaman.Riptide.Hot(dummies[1]).Apply(sim)
		}
		// the first cast has started, a hard cast still in progress
		runUntil(sim, 100*time.Millisecond)
		got := ""
		switch {
		case shaman.Hardcast.Expires > sim.CurrentTime && shaman.Hardcast.ActionID == shaman.ChainHeal.ActionID:
			got = "Chain Heal"
		case shaman.Hardcast.Expires > sim.CurrentTime && shaman.Hardcast.ActionID == shaman.LesserHealingWave.ActionID:
			got = "Lesser Healing Wave"
		case shaman.Riptide.SpellMetrics[dummies[1].UnitIndex].Casts > 0 && !c.riptide:
			got = "Riptide"
		}
		if got != c.want {
			t.Errorf("%s: cast %q first, want %q", c.name, got, c.want)
		}
	}
}
