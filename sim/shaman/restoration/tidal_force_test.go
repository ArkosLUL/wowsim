package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func healCritChances(shaman *RestorationShaman) map[string]float64 {
	return map[string]float64{
		"Healing Wave":        shaman.HealingWave.HealingCritChance(),
		"Lesser Healing Wave": shaman.LesserHealingWave.HealingCritChance(),
		"Chain Heal":          shaman.ChainHeal.HealingCritChance(),
		"Riptide":             shaman.Riptide.HealingCritChance(),
	}
}

// Tidal Force (55198) puts 55166 on the shaman: 3 stacks of 20% crit chance on Healing Wave, Lesser Healing Wave
// and Chain Heal for 20 s, off a 3 minute cooldown.
func TestTidalForceRaisesTheThreeHealsCrit(t *testing.T) {
	sim, shaman := healSetup{dummies: 1}.build(t)
	startFight(sim)
	before := healCritChances(shaman)

	if !shaman.TidalForce.Cast(sim, &shaman.Unit) {
		t.Fatal("Tidal Force wasn't cast")
	}
	if got := shaman.GetAura("Tidal Force").GetStacks(); got != 3 {
		t.Errorf("Tidal Force has %d stacks, want 3", got)
	}
	for name, chance := range healCritChances(shaman) {
		want := before[name]
		if name != "Riptide" {
			want += 0.6
		}
		if math.Abs(chance-want) > 1e-9 {
			t.Errorf("%s crits %.4f with Tidal Force, want %.4f", name, chance, want)
		}
	}
	if shaman.TidalForce.CD.IsReady(sim) {
		t.Errorf("Tidal Force is ready right after the cast")
	}

	runUntil(sim, 19*time.Second)
	if !shaman.GetAura("Tidal Force").IsActive() {
		t.Errorf("Tidal Force ended before 20 s")
	}
	runUntil(sim, 21*time.Second)
	if shaman.GetAura("Tidal Force").IsActive() {
		t.Errorf("Tidal Force lasted past 20 s")
	}
	for name, chance := range healCritChances(shaman) {
		if math.Abs(chance-before[name]) > 1e-9 {
			t.Errorf("%s crits %.4f after Tidal Force ended, want %.4f", name, chance, before[name])
		}
	}
	if shaman.TidalForce.CD.IsReady(sim) {
		t.Errorf("Tidal Force is ready 21 s in, want 3 minutes")
	}
	runUntil(sim, 181*time.Second)
	if !shaman.TidalForce.CD.IsReady(sim) {
		t.Errorf("Tidal Force isn't ready after 3 minutes")
	}
}

// A crit of Healing Wave, Lesser Healing Wave or Chain Heal spends one charge (spell_proc for 55198), a crit of
// Riptide none, and the last charge ends the aura.
func TestTidalForceChargesGoOnCrits(t *testing.T) {
	sim, shaman := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	shaman.AddStatDynamic(sim, stats.SpellCrit, 200*core.CritRatingPerCritChance)
	shaman.TidalForce.Cast(sim, &shaman.Unit)
	aura := shaman.GetAura("Tidal Force")

	for range 10 {
		shaman.Riptide.SkipCastAndApplyEffects(sim, target)
	}
	if got := aura.GetStacks(); got != 3 {
		t.Errorf("%d stacks after 10 crits of Riptide, want 3", got)
	}
	for i, heal := range []*core.Spell{shaman.HealingWave, shaman.LesserHealingWave, shaman.ChainHeal} {
		heal.SkipCastAndApplyEffects(sim, target)
		if got, want := aura.GetStacks(), int32(2-i); got != want {
			t.Errorf("%d stacks after crit %d, want %d", got, i+1, want)
		}
	}
	if aura.IsActive() {
		t.Errorf("Tidal Force is still up with no charges")
	}
}

// Every target of a Chain Heal rolls its crit when the cast launches, before the first heal lands and spends a
// charge: with 45% crit and the full 60% more, all four crit.
func TestTidalForceRollsEveryChainHealTargetAtLaunch(t *testing.T) {
	sim, shaman := healSetup{glyphs: StandardGlyphs, dummies: 4}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	shaman.AddStatDynamic(sim, stats.SpellCrit, (0.45-shaman.ChainHeal.HealingCritChance())*100*core.CritRatingPerCritChance)
	aura := shaman.GetAura("Tidal Force")

	for trial := range 40 {
		aura.Activate(sim)
		aura.SetStacks(sim, 3)
		var crits int32
		for _, dummy := range dummies {
			crits -= shaman.ChainHeal.SpellMetrics[dummy.UnitIndex].Crits
		}
		shaman.ChainHeal.SkipCastAndApplyEffects(sim, dummies[0])
		for _, dummy := range dummies {
			crits += shaman.ChainHeal.SpellMetrics[dummy.UnitIndex].Crits
		}
		if crits != 4 {
			t.Fatalf("cast %d: %d of 4 targets crit, want all", trial, crits)
		}
		if aura.IsActive() {
			t.Fatalf("cast %d: Tidal Force is still up after 4 crits", trial)
		}
	}
}
