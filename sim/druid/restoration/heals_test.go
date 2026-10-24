package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Unit::SpellHealingBonusTaken multiplies a Nourish by 1.2f when the target has the caster's own Rejuvenation,
// Regrowth, Lifebloom or Wild Growth. Another druid's hot doesn't count.
func TestNourishBonusWithOwnHot(t *testing.T) {
	sim, druid := healSetup{dummies: 1, other: true}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	other := sim.Raid.Parties[1].Players[0].(*RestorationDruid)
	neverCrit(sim, druid, druid.Nourish.Spell)

	bare := castHeal(sim, druid.Nourish.Spell, target)
	if want := math.Floor((1883 + power(druid.Nourish.Spell, target, nourishSP)) * giftOfNature); bare != want {
		t.Fatalf("Nourish healed %v with no hot, want %v", bare, want)
	}
	boosted := float64(int32(float32(bare) * float32(1.2)))

	other.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	if got := castHeal(sim, druid.Nourish.Spell, target); got != bare {
		t.Errorf("Nourish healed %v with another druid's Rejuvenation up, want %v", got, bare)
	}
	other.Rejuvenation.Hot(target).Cancel(sim)

	for _, hot := range []*core.Spell{druid.Rejuvenation.Spell, druid.Regrowth.Spell, druid.Lifebloom.Spell, druid.WildGrowth.Spell} {
		hot.SkipCastAndApplyEffects(sim, target)
		if got := castHeal(sim, druid.Nourish.Spell, target); got != boosted {
			t.Errorf("Nourish healed %v with %s up, want %v", got, hot.ActionID, boosted)
		}
		hot.Hot(target).Cancel(sim)
	}
}

// spell_dru_wild_growth_aura takes the amount as the tick's base and rewrites it before each tick: 6% more on the
// first, 2% less on each one after, in float32.
func TestWildGrowthTicksDecline(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)

	druid.WildGrowth.SkipCastAndApplyEffects(sim, target)
	base := float32(math.Floor((206 + power(druid.WildGrowth.Spell, target, wildGrowthSP)) * hotMods))
	var sum float64
	for tick := range 7 {
		before := healed(druid.WildGrowth.Spell, target)
		runUntil(sim, time.Duration(tick)*time.Second+1500*time.Millisecond)
		bonus := float32(6 - 2*tick)
		want := float64(int32(base + float32(base*bonus)/100))
		got := healed(druid.WildGrowth.Spell, target) - before
		if got != want {
			t.Errorf("tick %d healed %v, want %v", tick+1, got, want)
		}
		sum += got
	}
	if druid.WildGrowth.Hot(target).IsActive() {
		t.Error("Wild Growth still up after 7 ticks")
	}
	if math.Abs(sum-7*float64(base)) > 7 {
		t.Errorf("the 7 ticks healed %v, want about 7 x %v", sum, base)
	}
}

// spell_dru_wild_growth: past five targets, the five living raid units with the lowest health percent, a stable
// sort, so a tie keeps raid order.
func TestWildGrowthPicksTheFiveLowest(t *testing.T) {
	sim, druid := healSetup{dummies: 9, other: true}.build(t)
	startFight(sim)
	other := &sim.Raid.Parties[1].Players[0].(*RestorationDruid).Unit
	dummies := targetDummies(sim)
	other.RemoveHealth(sim, other.MaxHealth()/2)

	druid.WildGrowth.SkipCastAndApplyEffects(sim, dummies[0])
	want := []*core.Unit{other, &druid.Unit, dummies[0], dummies[1], dummies[2]}
	got := 0
	for _, unit := range sim.Raid.AllUnits {
		on := druid.WildGrowth.Hot(unit).IsActive()
		if on {
			got++
		}
		shouldBe := false
		for _, w := range want {
			shouldBe = shouldBe || w == unit
		}
		if on != shouldBe {
			t.Errorf("%s has Wild Growth: %v, want %v", unit.Label, on, shouldBe)
		}
	}
	if got != 5 {
		t.Errorf("Wild Growth went on %d units, want 5", got)
	}
}

// Spell::EffectHeal: Swiftmend heals for its effect's 1 plus 4 ticks of the druid's Rejuvenation or 6 of its
// Regrowth, whichever has less time left, each tick as the target takes it, and takes that hot off. It can't go
// out with neither on the target (AURA_STATE_SWIFTMEND).
func TestSwiftmendEatsTheHotWithLessTimeLeft(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid, druid.Regrowth.Spell)
	swiftmend := druid.Swiftmend

	if swiftmend.CanCast(sim, target) {
		t.Fatal("Swiftmend can go out with no hot on the target")
	}

	// Rejuvenation runs 18 s and Regrowth 27 s with Nature's Splendor
	druid.Regrowth.SkipCastAndApplyEffects(sim, target)
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	rejuvTick := math.Floor((338 + power(druid.Rejuvenation.Spell, target, rejuvenationSP)) * rejuvMods)
	regrowthTick := math.Floor((335 + power(druid.Regrowth.Spell, target, regrowthHotSP)) * hotMods)

	if !swiftmend.CanCast(sim, target) {
		t.Fatal("Swiftmend can't go out with both hots on the target")
	}
	if got, want := castHeal(sim, swiftmend.Spell, target), 1+4*rejuvTick; got != want {
		t.Errorf("Swiftmend on Rejuvenation healed %v, want %v", got, want)
	}
	if druid.Rejuvenation.Hot(target).IsActive() || !druid.Regrowth.Hot(target).IsActive() {
		t.Errorf("after Swiftmend Rejuvenation is up: %v, Regrowth: %v; want only Regrowth",
			druid.Rejuvenation.Hot(target).IsActive(), druid.Regrowth.Hot(target).IsActive())
	}

	// Regrowth now has 15 s left, a new Rejuvenation 18
	runUntil(sim, 12*time.Second)
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	if got, want := castHeal(sim, swiftmend.Spell, target), 1+6*regrowthTick; got != want {
		t.Errorf("Swiftmend on Regrowth healed %v, want %v", got, want)
	}
	if druid.Regrowth.Hot(target).IsActive() || !druid.Rejuvenation.Hot(target).IsActive() {
		t.Errorf("after Swiftmend Regrowth is up: %v, Rejuvenation: %v; want only Rejuvenation",
			druid.Regrowth.Hot(target).IsActive(), druid.Rejuvenation.Hot(target).IsActive())
	}
}

// Swiftmend takes the target's healing taken once, on the ticks it reads: Tree of Life's 6%.
// Each tick carries Master Shapeshifter's healing done in the form, as int32(amount × aura 136).
func TestSwiftmendTakesHealingTakenOnce(t *testing.T) {
	sim, druid := healSetup{talents: restoTalents(true), dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)

	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	amount := math.Floor((338 + power(druid.Rejuvenation.Spell, target, rejuvenationSP)) * rejuvMods)
	tick := math.Floor(float64(int32(float32(amount)*float32(shapeshifter))) * 1.06)
	if got, want := castHeal(sim, druid.Swiftmend.Spell, target), 1+4*tick; got != want {
		t.Errorf("Swiftmend healed %v under Tree of Life, want %v", got, want)
	}
}

// Strength of Wrynn's 30% is healing done, so the ticks Swiftmend reads take it with the hot caster's healing
// done, as a Rejuvenation tick does. The sim stands it in on the healed unit's HealingTakenMultiplier.
func TestSwiftmendTakesStrengthOfWrynn(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)
	target.PseudoStats.HealingTakenMultiplier *= 1.3

	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	before := healed(druid.Rejuvenation.Spell, target)
	runUntil(sim, 3500*time.Millisecond)
	tick := healed(druid.Rejuvenation.Spell, target) - before
	amount := math.Floor((338 + power(druid.Rejuvenation.Spell, target, rejuvenationSP)) * rejuvMods)
	if want := math.Floor(amount * 1.3); tick != want {
		t.Fatalf("a Rejuvenation tick healed %v under Strength of Wrynn, want %v", tick, want)
	}
	if got, want := castHeal(sim, druid.Swiftmend.Spell, target), 1+4*tick; got != want {
		t.Errorf("Swiftmend healed %v under Strength of Wrynn, want %v", got, want)
	}
}

// AURA_STATE_SWIFTMEND isn't per caster: with none of its own on the target, Swiftmend eats another druid's
// Rejuvenation or Regrowth (Spell::EffectHeal), as that druid's tick. With one of its own up it never does.
func TestSwiftmendFallsBackToAnotherDruidsHot(t *testing.T) {
	sim, druid := healSetup{dummies: 1, other: true}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	other := sim.Raid.Parties[1].Players[0].(*RestorationDruid)
	neverCrit(sim, druid)

	other.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	if !druid.Swiftmend.CanCast(sim, target) {
		t.Fatal("Swiftmend can't go out on another druid's Rejuvenation")
	}
	tick := math.Floor((338 + power(other.Rejuvenation.Spell, target, rejuvenationSP)) * rejuvMods)
	if got, want := castHeal(sim, druid.Swiftmend.Spell, target), 1+4*tick; got != want {
		t.Errorf("Swiftmend on another druid's Rejuvenation healed %v, want %v", got, want)
	}
	if other.Rejuvenation.Hot(target).IsActive() {
		t.Error("Swiftmend left the other druid's Rejuvenation up")
	}

	// the other druid's Rejuvenation has less time left than the druid's own Regrowth, which still goes first
	druid.Regrowth.SkipCastAndApplyEffects(sim, target)
	other.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	castHeal(sim, druid.Swiftmend.Spell, target)
	if druid.Regrowth.Hot(target).IsActive() || !other.Rejuvenation.Hot(target).IsActive() {
		t.Errorf("after Swiftmend the druid's Regrowth is up: %v, the other druid's Rejuvenation: %v; want only the Rejuvenation",
			druid.Regrowth.Hot(target).IsActive(), other.Rejuvenation.Hot(target).IsActive())
	}
}

// Nature's Grace's mask (spell_proc -16880) has Healing Touch, Regrowth, Nourish and Swiftmend, not the hots or
// the bloom: a crit of one of the four gives 16886 at 3/3, any other heal never does.
func TestNaturesGraceOnHealCrits(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	grace := druid.NaturesGraceProcAura
	if grace == nil {
		t.Fatal("no Nature's Grace")
	}
	druid.AddStatDynamic(sim, stats.SpellCrit, 200*core.CritRatingPerCritChance)

	for _, c := range []struct {
		spell *core.Spell
		gives bool
	}{
		{druid.HealingTouch.Spell, true},
		{druid.Regrowth.Spell, true},
		{druid.Nourish.Spell, true},
		{druid.Rejuvenation.Spell, false},
		{druid.Lifebloom.Spell, false},
		{druid.WildGrowth.Spell, false},
		{druid.LifebloomBloom.Spell, false},
		{druid.Swiftmend.Spell, true},
	} {
		grace.Deactivate(sim)
		if c.spell == druid.Swiftmend.Spell {
			druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
			grace.Deactivate(sim)
		}
		c.spell.SkipCastAndApplyEffects(sim, target)
		if grace.IsActive() != c.gives {
			t.Errorf("%s gave Nature's Grace: %v, want %v", c.spell.ActionID, grace.IsActive(), c.gives)
		}
	}
}

// Clearcasting's class mask covers every heal, so each goes out free.
func TestClearcastingMakesHealsFree(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	clearcasting := druid.ClearcastingAura

	for _, spell := range []*core.Spell{druid.HealingTouch.Spell, druid.Regrowth.Spell, druid.Rejuvenation.Spell,
		druid.Lifebloom.Spell, druid.Nourish.Spell, druid.WildGrowth.Spell, druid.Swiftmend.Spell} {
		if spell == druid.Swiftmend.Spell {
			druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
		}
		clearcasting.Activate(sim)
		// the empty rotation leaves the GCD held
		druid.GCD.Reset()
		if !spell.Cast(sim, target) {
			t.Fatalf("%s didn't cast", spell.ActionID)
		}
		if spell.CurCast.Cost != 0 {
			t.Errorf("%s cost %v under Clearcasting", spell.ActionID, spell.CurCast.Cost)
		}
		runUntil(sim, sim.CurrentTime+4*time.Second)
	}
}

// Omen of Clarity (16864, 3.5 PPM over the cast time entry, at least 1.5 s) rolls once for each target a heal
// lands on, the hots' landings with nothing to heal and the bloom too (spell_dru_omen_of_clarity::CheckProc
// lets a druid spell through, CAN_PROC_FROM_PROCS the triggered bloom).
func TestOmenOfClarityRollsOnHealLandings(t *testing.T) {
	sim, druid := healSetup{dummies: 4}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	clearcasting := druid.ClearcastingAura

	const casts = 6000
	instant := 3.5 * 1.5 / 60
	for _, c := range []struct {
		spell *core.Spell
		want  float64
	}{
		{druid.Rejuvenation.Spell, instant},
		{druid.Lifebloom.Spell, instant},
		{druid.LifebloomBloom.Spell, instant},
		// the druid and the four dummies
		{druid.WildGrowth.Spell, 1 - math.Pow(1-instant, 5)},
		{druid.HealingTouch.Spell, 3.5 * 3 / 60},
	} {
		procs := 0
		for range casts {
			clearcasting.Deactivate(sim)
			c.spell.SkipCastAndApplyEffects(sim, target)
			if clearcasting.IsActive() {
				procs++
			}
		}
		rate := float64(procs) / casts
		if se := math.Sqrt(c.want * (1 - c.want) / casts); math.Abs(rate-c.want) > 5*se {
			t.Errorf("%s gave Clearcasting at %.4f a cast, want %.4f", c.spell.ActionID, rate, c.want)
		}
	}
}
