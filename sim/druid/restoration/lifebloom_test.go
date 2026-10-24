package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// lifebloomOneStack is one stack's tick: 53 and 0.09518 of the healing power, the hot mods in, whole.
func lifebloomOneStack(druid *RestorationDruid, target *core.Unit) float64 {
	return math.Floor((53 + power(druid.Lifebloom.Spell, target, lifebloomSP)) * hotMods)
}

// lifebloomBloom is spell_dru_lifebloom's heal for the stacks: 776 a stack, and the healing power times 0.655
// times the stacks as one int32, then Gift of Nature's SPELLMOD_DAMAGE.
func lifebloomBloom(druid *RestorationDruid, target *core.Unit, stacks int32) float64 {
	bonus := int32(float32(float32(bloomSP)*float32(druid.LifebloomBloom.HealingPower(target))) * float32(stacks))
	return math.Floor(float64(776*stacks+bonus) * giftOfNature)
}

// Each Lifebloom adds a stack, up to 3, and starts its ticks over. A tick heals for one stack's whole amount
// times the stacks (AuraEffect::CalculateAmount).
func TestLifebloomStacksToThree(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)
	hot := druid.Lifebloom.Hot(target)

	for i, want := range []int32{1, 2, 3, 3} {
		druid.Lifebloom.SkipCastAndApplyEffects(sim, target)
		if got := hot.GetStacks(); got != want {
			t.Fatalf("after %d casts Lifebloom has %d stacks, want %d", i+1, got, want)
		}
	}

	before := healed(druid.Lifebloom.Spell, target)
	runUntil(sim, 1500*time.Millisecond)
	if got, want := healed(druid.Lifebloom.Spell, target)-before, 3*lifebloomOneStack(druid, target); got != want {
		t.Errorf("a 3 stack tick healed %v, want %v", got, want)
	}
	if hot.TickCount != 1 {
		t.Errorf("%d ticks in 1.5 s, want 1", hot.TickCount)
	}
}

// Unit::SpellHealingBonusTaken takes Lifebloom's stacks into the flat healing taken term too, on the ticks and
// on the bloom: int32(taken x coefficient x stacks), the coefficients unmodded on a target that isn't the druid.
func TestLifebloomFlatHealingTakenCountsTheStacks(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)
	for range 3 {
		druid.Lifebloom.SkipCastAndApplyEffects(sim, target)
	}
	// five stacks of Talisman of Troll Divinity's 60518
	const taken = 290
	target.PseudoStats.BonusHealingTaken = taken
	flat := func(coefficient float64) float64 {
		return float64(int32(float32(coefficient) * float32(taken*3)))
	}

	before := healed(druid.Lifebloom.Spell, target)
	runUntil(sim, 1500*time.Millisecond)
	if got, want := healed(druid.Lifebloom.Spell, target)-before, 3*lifebloomOneStack(druid, target)+flat(0.09518); got != want {
		t.Errorf("a 3 stack tick healed %v with %v flat healing taken, want %v", got, taken, want)
	}
	runUntil(sim, 9500*time.Millisecond)
	if got, want := healed(druid.LifebloomBloom.Spell, target), lifebloomBloom(druid, target, 3)+flat(0.655); got != want {
		t.Errorf("a 3 stack bloom healed %v with %v flat healing taken, want %v", got, taken, want)
	}
	if got := target.PseudoStats.BonusHealingTaken; got != taken {
		t.Errorf("the target's flat healing taken is %v after the heals, want %v back", got, taken)
	}
}

// When Lifebloom runs out it blooms (33778) for 776 and 0.655 of the healing power a stack, and the script gives
// back half the base cost a stack, ahead of the heal: 28% of base mana x stacks / 100 / 2 in whole numbers.
func TestLifebloomBloomsWhenItRunsOut(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)
	// no regen, so the mana only moves with the bloom
	druid.PseudoStats.SpiritRegenMultiplier = 0
	druid.UpdateManaRegenRates()

	for range 3 {
		druid.Lifebloom.SkipCastAndApplyEffects(sim, target)
	}
	druid.SpendMana(sim, 5000, druid.NewManaMetrics(core.ActionID{OtherID: 1}))
	mana := druid.CurrentMana()

	// Nature's Splendor makes it 9 s
	runUntil(sim, 8500*time.Millisecond)
	if got := healed(druid.LifebloomBloom.Spell, target); got != 0 {
		t.Fatalf("bloomed for %v before the aura ran out", got)
	}
	runUntil(sim, 9500*time.Millisecond)
	if druid.Lifebloom.Hot(target).IsActive() {
		t.Fatal("Lifebloom still up 9.5 s on")
	}
	if got, want := healed(druid.LifebloomBloom.Spell, target), lifebloomBloom(druid, target, 3); got != want {
		t.Errorf("a 3 stack bloom healed %v, want %v", got, want)
	}
	if got, want := druid.CurrentMana()-mana, float64(28*int32(druid.BaseMana)*3/100/2); got != want {
		t.Errorf("the bloom gave back %v mana, want %v", got, want)
	}
}

// Only AURA_REMOVE_BY_EXPIRE blooms: a recast refreshes without one, and so does the fight ending first.
func TestLifebloomRefreshAndFightEndDontBloom(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)

	druid.Lifebloom.SkipCastAndApplyEffects(sim, target)
	runUntil(sim, 8*time.Second)
	druid.Lifebloom.SkipCastAndApplyEffects(sim, target)
	// the first cast would have run out at 9 s
	runUntil(sim, 16*time.Second)
	if got := healed(druid.LifebloomBloom.Spell, target); got != 0 {
		t.Fatalf("bloomed for %v on a refresh", got)
	}
	runUntil(sim, 18*time.Second)
	if got, want := healed(druid.LifebloomBloom.Spell, target), lifebloomBloom(druid, target, 2); got != want {
		t.Errorf("the 2 stack bloom healed %v, want %v", got, want)
	}

	runUntil(sim, sim.Duration-5*time.Second)
	druid.Lifebloom.SkipCastAndApplyEffects(sim, target)
	before := healed(druid.LifebloomBloom.Spell, target)
	for !sim.Step() {
	}
	sim.Cleanup()
	if got := healed(druid.LifebloomBloom.Spell, target) - before; got != 0 {
		t.Errorf("a Lifebloom the fight ended on bloomed for %v", got)
	}
}
