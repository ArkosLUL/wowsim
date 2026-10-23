package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/druid"
)

// With the talent the druid starts in Tree of Life, whose 34123 gives the whole raid 6% healing taken (a raid area
// aura, effect 65). Without it there's no form and no spell.
func TestTreeOfLifeHealsTheRaidMore(t *testing.T) {
	sim, resto := healSetup{talents: restoTalents(true), dummies: 5, other: true}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	other := &sim.Raid.Parties[1].Players[0].(*RestorationDruid).Unit
	if !resto.TreeOfLifeAura.IsActive() || !resto.InForm(druid.Tree) {
		t.Fatal("the druid isn't in Tree of Life at the pull")
	}
	// the fifth dummy sits in the second party, with the other druid
	for _, unit := range []*core.Unit{&resto.Unit, dummies[0], dummies[3], other, dummies[4]} {
		if got := unit.HealingTakenPctMultiplier(); got != 1.06 {
			t.Errorf("%s takes %v healing in the druid's raid, want 1.06", unit.Label, got)
		}
	}

	neverCrit(sim, resto, resto.HealingTouch.Spell)
	base := math.Floor((3761 + power(resto.HealingTouch.Spell, dummies[4], healingTouchSP)) * (giftOfNature * shapeshifter))
	if got, want := castHeal(sim, resto.HealingTouch.Spell, dummies[4]), math.Floor(base*1.06); got != want {
		t.Errorf("Healing Touch healed a unit in another party for %v, want %v", got, want)
	}

	plain, bare := healSetup{dummies: 1}.build(t)
	startFight(plain)
	if bare.TreeOfLifeAura != nil || bare.TreeOfLife != nil || !bare.InForm(druid.Humanoid) {
		t.Error("a druid without the talent has Tree of Life")
	}
}

// Every heal casts in Tree of Life: the form has SHAPESHIFT_FLAG_STANCE, and the heals' StancesNot lists only
// Moonkin. None takes the druid out of the form.
func TestHealsCastInTreeOfLife(t *testing.T) {
	sim, resto := healSetup{talents: restoTalents(true), dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]

	resto.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	for _, spell := range []*core.Spell{resto.HealingTouch.Spell, resto.Regrowth.Spell, resto.Rejuvenation.Spell,
		resto.Lifebloom.Spell, resto.Nourish.Spell, resto.WildGrowth.Spell, resto.Swiftmend.Spell} {
		runUntil(sim, sim.CurrentTime+4*time.Second)
		// the empty rotation leaves the GCD held
		resto.GCD.Reset()
		if !spell.Cast(sim, target) {
			t.Errorf("%s didn't cast in Tree of Life", spell.ActionID)
		}
		if !resto.TreeOfLifeAura.IsActive() {
			t.Fatalf("%s took the druid out of Tree of Life", spell.ActionID)
		}
	}
}

// Tree of Life Passive (5420) takes 20% off Rejuvenation, Regrowth, Lifebloom and Wild Growth with the talent,
// in any form, adding to Moonglow's 9%. Shifting costs 13% of base mana less Natural Shapeshifter's 30%, and
// puts the raid aura back on.
func TestTreeOfLifeCosts(t *testing.T) {
	cost := func(spell *core.Spell) float64 {
		return spell.StaticCost()
	}
	sim, resto := healSetup{talents: restoTalents(true), dummies: 1}.build(t)
	startFight(sim)
	_, plain := healSetup{dummies: 1}.build(t)

	for _, c := range []struct {
		spell, without *core.Spell
		pct            float64
	}{
		{resto.Rejuvenation.Spell, plain.Rejuvenation.Spell, 18},
		{resto.Regrowth.Spell, plain.Regrowth.Spell, 29},
		{resto.Lifebloom.Spell, plain.Lifebloom.Spell, 28},
		{resto.WildGrowth.Spell, plain.WildGrowth.Spell, 23},
	} {
		moonglow := 0.09
		if c.spell == resto.Lifebloom.Spell || c.spell == resto.WildGrowth.Spell {
			moonglow = 0
		}
		base := math.Floor(c.pct * resto.BaseMana / 100)
		if got, want := cost(c.spell), math.Floor(base*(1-0.2-moonglow)); got != want {
			t.Errorf("%s costs %v with Tree of Life, want %v", c.spell.ActionID, got, want)
		}
		if got, want := cost(c.without), math.Floor(base*(1-moonglow)); got != want {
			t.Errorf("%s costs %v without Tree of Life, want %v", c.spell.ActionID, got, want)
		}
	}

	dummy := targetDummies(sim)[0]
	resto.TreeOfLifeAura.Deactivate(sim)
	if resto.InForm(druid.Tree) || dummy.HealingTakenPctMultiplier() != 1 {
		t.Fatal("leaving Tree of Life left the form or its raid aura on")
	}
	if got, want := cost(resto.TreeOfLife.Spell), math.Floor(math.Floor(13*resto.BaseMana/100)*0.7); got != want {
		t.Errorf("Tree of Life costs %v, want %v", got, want)
	}
	resto.GCD.Reset()
	if !resto.TreeOfLife.Cast(sim, dummy) {
		t.Fatal("Tree of Life didn't cast")
	}
	if !resto.InForm(druid.Tree) || dummy.HealingTakenPctMultiplier() != 1.06 {
		t.Error("casting Tree of Life didn't put the form and its raid aura back on")
	}
	if resto.TreeOfLife.CanCast(sim, dummy) {
		t.Error("Tree of Life can be cast in Tree of Life")
	}
}
