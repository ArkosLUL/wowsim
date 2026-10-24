package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// enough healing power for the coefficients to show, and no whole number near a rounding edge
const testHealingPower = 1000

// Gift of Nature's SPELLMOD_DAMAGE on Healing Touch, Regrowth's heal and Nourish, Empowered Touch's flat 40 and 20
// hundredths on Healing Touch's and Nourish's coefficients, and Empowered Rejuvenation's 20% on Regrowth's. Without
// the points, none of it.
func TestHealingTalentsOnTheDirectHeals(t *testing.T) {
	bare := withResto(restoTalents(false), map[int]byte{10: '0', 12: '0', 14: '0', 19: '0'})
	for _, c := range []struct {
		talents         string
		mods            float64
		ht, nourish, rg float64
	}{
		{restoTalents(false), giftOfNature, healingTouchSP, nourishSP, regrowthSP},
		{bare, 1, 1.61, 0.672, 0.3},
	} {
		sim, druid := healSetup{talents: c.talents, dummies: 1}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		neverCrit(sim, druid, druid.HealingTouch.Spell, druid.Nourish.Spell, druid.Regrowth.Spell)
		druid.AddStatDynamic(sim, stats.SpellPower, testHealingPower)

		// Regrowth last: its hot would give Nourish its 20%
		for _, h := range []struct {
			spell    *core.Spell
			base, sp float64
		}{
			{druid.HealingTouch.Spell, 3761, c.ht},
			{druid.Nourish.Spell, 1883, c.nourish},
			{druid.Regrowth.Spell, 2257, c.rg},
		} {
			want := math.Floor((h.base + power(h.spell, target, h.sp)) * c.mods)
			if got := castHeal(sim, h.spell, target); got != want {
				t.Errorf("%s healed %v with talents %s, want %v", h.spell.ActionID, got, c.talents, want)
			}
		}
	}
}

// The hots' amounts: Genesis' and Gift of Nature's SPELLMOD_DOT on all four and Improved Rejuvenation's on
// Rejuvenation, multiplied, and Empowered Rejuvenation's 20% on their coefficients. The bloom takes Gift of Nature's
// SPELLMOD_DAMAGE and Empowered Rejuvenation too.
func TestHealingTalentsOnTheHots(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)
	druid.AddStatDynamic(sim, stats.SpellPower, testHealingPower)

	for _, c := range []struct {
		spell    *core.Spell
		base, sp float64
		mods     float64
		tick     time.Duration
	}{
		{druid.Rejuvenation.Spell, 338, rejuvenationSP, rejuvMods, 3 * time.Second},
		{druid.Regrowth.Spell, 335, regrowthHotSP, hotMods, 3 * time.Second},
		{druid.Lifebloom.Spell, 53, lifebloomSP, hotMods, time.Second},
	} {
		c.spell.SkipCastAndApplyEffects(sim, target)
		before := healed(c.spell, target)
		runUntil(sim, sim.CurrentTime+c.tick+500*time.Millisecond)
		want := math.Floor((c.base + power(c.spell, target, c.sp)) * c.mods)
		if got := healed(c.spell, target) - before; got != want {
			t.Errorf("a %s tick healed %v, want %v", c.spell.ActionID, got, want)
		}
		if c.spell != druid.Lifebloom.Spell {
			c.spell.Hot(target).Cancel(sim)
		}
	}

	// the Lifebloom above runs out 9 s after it went on, with Nature's Splendor
	runUntil(sim, sim.CurrentTime+9*time.Second)
	if got, want := healed(druid.LifebloomBloom.Spell, target), lifebloomBloom(druid, target, 1); got != want {
		t.Errorf("a 1 stack bloom healed %v, want %v", got, want)
	}

	druid.WildGrowth.SkipCastAndApplyEffects(sim, target)
	before := healed(druid.WildGrowth.Spell, target)
	runUntil(sim, sim.CurrentTime+1500*time.Millisecond)
	b := float32(math.Floor((206 + power(druid.WildGrowth.Spell, target, wildGrowthSP)) * hotMods))
	if got, want := healed(druid.WildGrowth.Spell, target)-before, float64(int32(b+float32(b*6)/100)); got != want {
		t.Errorf("Wild Growth's first tick healed %v, want %v", got, want)
	}
}

// Nature's Bounty is 5% crit a point on Regrowth and Nourish, adding to Nature's Majesty's 2% a point on Healing
// Touch and Nourish. Swiftmend has neither.
func TestNaturesBountyCrit(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	base := druid.Swiftmend.HealingCritChance()

	for _, c := range []struct {
		spell *core.Spell
		bonus float64
	}{
		{druid.Regrowth.Spell, 0.15},
		{druid.Nourish.Spell, 0.15 + 0.04},
		{druid.HealingTouch.Spell, 0.04},
	} {
		if got := c.spell.HealingCritChance() - base; math.Abs(got-c.bonus) > 1e-9 {
			t.Errorf("%s has %.4f more crit than Swiftmend, want %.4f", c.spell.ActionID, got, c.bonus)
		}
	}
}

// Tranquil Spirit takes 2% a point off Healing Touch and Nourish, adding to Moonglow's 9%.
func TestTranquilSpiritCost(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)

	for _, c := range []struct {
		spell *core.Spell
		pct   float64
	}{
		{druid.HealingTouch.Spell, 33},
		{druid.Nourish.Spell, 18},
	} {
		base := math.Floor(c.pct * druid.BaseMana / 100)
		if got, want := c.spell.StaticCost(), math.Floor(base*(1-0.09-0.02)); got != want {
			t.Errorf("%s costs %v, want %v", c.spell.ActionID, got, want)
		}
	}
}

// Subtlety's SPELLMOD_THREAT takes 10% a point off every heal's threat.
func TestSubtletyCutsHealThreat(t *testing.T) {
	threat := func(subtlety byte) (float64, []float64) {
		sim, druid := healSetup{talents: withResto(restoTalents(false), map[int]byte{4: subtlety})}.build(t)
		startFight(sim)
		neverCrit(sim, druid, druid.HealingTouch.Spell)
		druid.RemoveHealth(sim, druid.MaxHealth()-1)
		druid.HealingTouch.SkipCastAndApplyEffects(sim, &druid.Unit)
		var multipliers []float64
		for _, spell := range []*core.Spell{druid.HealingTouch.Spell, druid.Regrowth.Spell, druid.Rejuvenation.Spell,
			druid.Lifebloom.Spell, druid.LifebloomBloom.Spell, druid.Nourish.Spell, druid.WildGrowth.Spell,
			druid.Swiftmend.Spell} {
			multipliers = append(multipliers, spell.ThreatMultiplier)
		}
		return druid.HealingTouch.SpellMetrics[sim.Encounter.TargetUnits[0].UnitIndex].TotalThreat, multipliers
	}

	without, _ := threat('0')
	with, multipliers := threat('2')
	if without == 0 || math.Abs(with/without-0.8) > 1e-9 {
		t.Errorf("Healing Touch made %v threat with 2 points of Subtlety and %v without, want 80%%", with, without)
	}
	for i, m := range multipliers {
		if math.Abs(m-0.8) > 1e-9 {
			t.Errorf("heal %d has a threat multiplier of %v with 2 points, want 0.8", i, m)
		}
	}
}

// Gift of the Earthmother is 2% cast speed a point (aura 65, multiplying Celestial Focus' 3%) and takes 30 ms a
// point off Lifebloom's 1.5 s GCD ahead of the haste: int32(1350 x cast speed) at 5/5.
func TestGiftOfTheEarthmother(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]

	if want := 1 / (rank(3, 0.01) * rank(5, 0.02)); math.Abs(druid.CastSpeed-want) > 1e-12 {
		t.Errorf("cast speed is %v, want %v", druid.CastSpeed, want)
	}
	for _, c := range []struct {
		spell *core.Spell
		gcd   time.Duration
	}{
		{druid.Lifebloom.Spell, 1191 * time.Millisecond},
		{druid.Rejuvenation.Spell, 1323 * time.Millisecond},
	} {
		// the empty rotation leaves the GCD held
		druid.GCD.Reset()
		if !c.spell.Cast(sim, target) {
			t.Fatalf("%s didn't cast", c.spell.ActionID)
		}
		if c.spell.CurCast.GCD != c.gcd {
			t.Errorf("%s's GCD is %v, want %v", c.spell.ActionID, c.spell.CurCast.GCD, c.gcd)
		}
	}
}

// Master Shapeshifter casts 48422 in Tree of Life: 2% healing done a point (aura 136), which a direct heal takes
// with its SPELLMOD_DAMAGE and a hot as int32(tick x it). Improved Tree of Life's 5% of Spirit a point as healing
// power holds only in the form too (its Stances).
func TestMasterShapeshifterAndImprovedTreeOfLife(t *testing.T) {
	sim, druid := healSetup{talents: restoTalents(true), dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid, druid.HealingTouch.Spell)
	druid.AddStatDynamic(sim, stats.SpellPower, testHealingPower)

	healingTouch := func() float64 {
		return 3761 + power(druid.HealingTouch.Spell, target, healingTouchSP)
	}
	rejuvenationTick := func() float64 {
		druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
		before := healed(druid.Rejuvenation.Spell, target)
		runUntil(sim, sim.CurrentTime+3500*time.Millisecond)
		druid.Rejuvenation.Hot(target).Cancel(sim)
		return healed(druid.Rejuvenation.Spell, target) - before
	}

	treePower := druid.GetStat(stats.SpellPower)
	want := math.Floor(math.Floor(healingTouch()*(giftOfNature*shapeshifter)) * 1.06)
	if got := castHeal(sim, druid.HealingTouch.Spell, target); got != want {
		t.Errorf("Healing Touch healed %v in Tree of Life, want %v", got, want)
	}
	amount := math.Floor((338 + power(druid.Rejuvenation.Spell, target, rejuvenationSP)) * rejuvMods)
	if got, want := rejuvenationTick(), math.Floor(float64(int32(float32(amount)*float32(shapeshifter)))*1.06); got != want {
		t.Errorf("a Rejuvenation tick healed %v in Tree of Life, want %v", got, want)
	}

	druid.TreeOfLifeAura.Deactivate(sim)
	if got, want := treePower-druid.GetStat(stats.SpellPower), 0.15*druid.GetStat(stats.Spirit); math.Abs(got-want) > 1e-6 {
		t.Errorf("Tree of Life gave %v healing power, want %v", got, want)
	}
	if got, want := castHeal(sim, druid.HealingTouch.Spell, target), math.Floor(healingTouch()*giftOfNature); got != want {
		t.Errorf("Healing Touch healed %v out of Tree of Life, want %v", got, want)
	}
	amount = math.Floor((338 + power(druid.Rejuvenation.Spell, target, rejuvenationSP)) * rejuvMods)
	if got := rejuvenationTick(); got != amount {
		t.Errorf("a Rejuvenation tick healed %v out of Tree of Life, want %v", got, amount)
	}
}

// spell_dru_living_seed: a crit of Healing Touch, Regrowth's heal, Nourish or Swiftmend puts a seed of 30% of it on
// the target, a new one in place of the old. The target's next hit taken (the stand-in's, 2 s on) spends it on
// 48503, which neither crits nor takes the druid's healing done, only the target's healing taken.
func TestLivingSeed(t *testing.T) {
	sim, druid := healSetup{talents: LivingSeedTalents, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	seed := func() *core.Aura { return target.GetAuraByID(core.ActionID{SpellID: 48504}) }
	for _, spell := range []*core.Spell{druid.HealingTouch.Spell, druid.Nourish.Spell, druid.Regrowth.Spell} {
		spell.Direct.Max = spell.Direct.Min
	}
	druid.AddStatDynamic(sim, stats.SpellCrit, 200*core.CritRatingPerCritChance)

	first := castHeal(sim, druid.HealingTouch.Spell, target)
	if seed() == nil || !seed().IsActive() {
		t.Fatal("a crit Healing Touch planted no seed")
	}
	runUntil(sim, sim.CurrentTime+time.Second)
	second := castHeal(sim, druid.Nourish.Spell, target)
	if first == second {
		t.Fatal("the two heals are the same size, so the test can't tell the seeds apart")
	}
	runUntil(sim, sim.CurrentTime+500*time.Millisecond)
	if got := healed(druid.LivingSeedHeal.Spell, target); got != 0 {
		t.Fatalf("the seed bloomed for %v before the hit", got)
	}
	runUntil(sim, sim.CurrentTime+time.Second)
	want := math.Floor(float64(uint32(float32(float32(second)*30)/100)) * 1.06)
	if got := healed(druid.LivingSeedHeal.Spell, target); got != want {
		t.Errorf("the seed bloomed for %v, want %v (Nourish's seed in place of Healing Touch's)", got, want)
	}
	if seed().IsActive() {
		t.Error("the seed is still up after it bloomed")
	}

	// Swiftmend eats the Regrowth's hot
	for _, spell := range []*core.Spell{druid.Regrowth.Spell, druid.Swiftmend.Spell} {
		castHeal(sim, spell, target)
		if !seed().IsActive() {
			t.Errorf("a crit %s planted no seed", spell.ActionID)
		}
		seed().Deactivate(sim)
	}

	// the bloom and the hots never plant one
	for range 3 {
		druid.Lifebloom.SkipCastAndApplyEffects(sim, target)
	}
	runUntil(sim, sim.CurrentTime+9500*time.Millisecond)
	if healed(druid.LifebloomBloom.Spell, target) == 0 || seed().IsActive() {
		t.Error("Lifebloom's crit bloom planted a seed, or didn't bloom")
	}

	neverCrit(sim, druid)
	castHeal(sim, druid.HealingTouch.Spell, target)
	if seed().IsActive() {
		t.Error("a Healing Touch that didn't crit planted a seed")
	}
}

// Living Seed's first rank plants a seed on 33% of the crits (spell_proc -48496).
func TestLivingSeedChance(t *testing.T) {
	sim, druid := healSetup{talents: withResto(LivingSeedTalents, map[int]byte{20: '1'}), dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	seed := target.GetAuraByID(core.ActionID{SpellID: 48504})
	druid.AddStatDynamic(sim, stats.SpellCrit, 200*core.CritRatingPerCritChance)

	const casts = 4000
	seeds := 0
	for range casts {
		seed.Deactivate(sim)
		druid.HealingTouch.SkipCastAndApplyEffects(sim, target)
		if seed.IsActive() {
			seeds++
		}
	}
	rate, want := float64(seeds)/casts, 0.33
	if se := math.Sqrt(want * (1 - want) / casts); math.Abs(rate-want) > 5*se {
		t.Errorf("a crit planted a seed %.4f of the time, want %.2f", rate, want)
	}
}

// spell_dru_revitalize: each tick of the druid's Rejuvenation or Wild Growth has 5% a point to energize its target by
// its power type, 1% of max mana for mana (48542, Spell::EffectEnergize). Regrowth's and Lifebloom's ticks never do,
// nor a target with no power.
func TestRevitalize(t *testing.T) {
	sim, druid := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	// no regen, so the mana only moves with Revitalize
	druid.PseudoStats.SpiritRegenMultiplier = 0
	druid.UpdateManaRegenRates()
	drain := druid.NewManaMetrics(core.ActionID{OtherID: 1})
	perGain := math.Floor(druid.MaxMana() / 100)

	gained := 0.0
	spend := func() {
		gained += druid.CurrentMana()
		druid.SpendMana(sim, druid.CurrentMana(), drain)
	}
	spend()
	gained = 0

	// Regrowth and Lifebloom on the druid tick for 27 s and 9 s, Lifebloom taken off before its bloom gives mana
	druid.Regrowth.SkipCastAndApplyEffects(sim, &druid.Unit)
	druid.Lifebloom.SkipCastAndApplyEffects(sim, &druid.Unit)
	runUntil(sim, sim.CurrentTime+8500*time.Millisecond)
	druid.Lifebloom.Hot(&druid.Unit).Cancel(sim)
	runUntil(sim, sim.CurrentTime+20*time.Second)
	spend()
	if gained != 0 {
		t.Fatalf("Regrowth and Lifebloom gave %v mana", gained)
	}

	// Wild Growth goes on the druid and the dummy, 7 ticks each
	const casts = 70
	for range casts {
		druid.WildGrowth.SkipCastAndApplyEffects(sim, target)
		runUntil(sim, sim.CurrentTime+7500*time.Millisecond)
		spend()
	}
	gains := gained / perGain
	if gains != math.Floor(gains) {
		t.Fatalf("Revitalize gave %v mana, not a whole number of %v", gained, perGain)
	}
	ticks := float64(casts * 7)
	if se := math.Sqrt(ticks * 0.15 * 0.85); math.Abs(gains-0.15*ticks) > 5*se {
		t.Errorf("Revitalize gave mana on %v of %v ticks, want about %v", gains, ticks, 0.15*ticks)
	}
}

// Rejuvenation's ticks roll Revitalize too, on the druid and on another mana user.
func TestRevitalizeOnRejuvenation(t *testing.T) {
	sim, druid := healSetup{dummies: 1, other: true}.build(t)
	startFight(sim)
	targets := []*RestorationDruid{druid, sim.Raid.Parties[1].Players[0].(*RestorationDruid)}
	drains := make([]*core.ResourceMetrics, len(targets))
	for i, target := range targets {
		target.PseudoStats.SpiritRegenMultiplier = 0
		target.UpdateManaRegenRates()
		drains[i] = target.NewManaMetrics(core.ActionID{OtherID: 1})
		target.SpendMana(sim, target.CurrentMana(), drains[i])
	}

	// 6 ticks each with Nature's Splendor
	const casts = 30
	gains := 0.0
	for range casts {
		for _, target := range targets {
			druid.Rejuvenation.SkipCastAndApplyEffects(sim, &target.Unit)
		}
		runUntil(sim, sim.CurrentTime+18500*time.Millisecond)
		for i, target := range targets {
			gains += target.CurrentMana() / math.Floor(target.MaxMana()/100)
			target.SpendMana(sim, target.CurrentMana(), drains[i])
		}
	}
	if gains != math.Floor(gains) {
		t.Fatalf("Revitalize gave %v gains, not a whole number", gains)
	}
	ticks := float64(casts * 2 * 6)
	if se := math.Sqrt(ticks * 0.15 * 0.85); math.Abs(gains-0.15*ticks) > 5*se {
		t.Errorf("Revitalize gave mana on %v of %v Rejuvenation ticks, want about %v", gains, ticks, 0.15*ticks)
	}
}
