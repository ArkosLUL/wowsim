package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Five pieces of each restoration set, one per slot.
var (
	t7Regalia = []int32{39531, 39538, 39539, 39542, 39543}
	t8Regalia = []int32{45345, 45346, 45347, 45348, 45349}
	t9Garb    = []int32{48102, 48129, 48130, 48131, 48132}
	t10Garb   = []int32{50106, 50107, 50108, 50109, 50113}
)

func glyphs(g ...proto.DruidMajorGlyph) *proto.Glyphs {
	out := &proto.Glyphs{}
	slots := []*int32{&out.Major1, &out.Major2, &out.Major3}
	for i, glyph := range g {
		*slots[i] = int32(glyph)
	}
	return out
}

func near(t *testing.T, what string, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s is %v, want %v", what, got, want)
	}
}

// Glyph of Healing Touch (54825): 25% less mana, 50% less healing and 1.5 s off the cast.
func TestGlyphOfHealingTouch(t *testing.T) {
	sim, bare := healSetup{dummies: 1}.build(t)
	_, glyphed := healSetup{dummies: 1, glyphs: glyphs(proto.DruidMajorGlyph_GlyphOfHealingTouch)}.build(t)

	if d := glyphed.HealingTouch.DefaultCast.CastTime - bare.HealingTouch.DefaultCast.CastTime; d != -1500*time.Millisecond {
		t.Errorf("the glyph moves Healing Touch's cast by %s, want -1.5 s", d)
	}
	base := float64(bare.HealingTouch.DefaultCast.Cost)
	near(t, "the glyph's mana saving", bare.HealingTouch.StaticCost()-glyphed.HealingTouch.StaticCost(), 0.25*base, 1.5)

	heal := func(sim *core.Simulation, druid *RestorationDruid) float64 {
		startFight(sim)
		neverCrit(sim, druid, druid.HealingTouch.Spell)
		return castHeal(sim, druid.HealingTouch.Spell, targetDummies(sim)[0])
	}
	sim2, glyphed2 := healSetup{dummies: 1, glyphs: glyphs(proto.DruidMajorGlyph_GlyphOfHealingTouch)}.build(t)
	near(t, "the glyphed heal over the bare one", heal(sim2, glyphed2)/heal(sim, bare), 0.5, 0.001)
}

// Glyph of Lifebloom (54826): a second more on the hot.
func TestGlyphOfLifebloom(t *testing.T) {
	sim, bare := healSetup{dummies: 1}.build(t)
	_, glyphed := healSetup{dummies: 1, glyphs: glyphs(proto.DruidMajorGlyph_GlyphOfLifebloom)}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	if d := glyphed.Lifebloom.Hot(target).NumberOfTicks - bare.Lifebloom.Hot(target).NumberOfTicks; d != 1 {
		t.Errorf("the glyph adds %d ticks to Lifebloom, want 1", d)
	}
}

// spell_dru_nourish: 6% a hot of the druid's on the target, on top of the 20%.
func TestGlyphOfNourish(t *testing.T) {
	heal := func(g *proto.Glyphs) float64 {
		sim, druid := healSetup{dummies: 1, glyphs: g}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		neverCrit(sim, druid, druid.Nourish.Spell)
		druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
		druid.Regrowth.SkipCastAndApplyEffects(sim, target)
		return castHeal(sim, druid.Nourish.Spell, target)
	}
	bare := heal(nil)
	near(t, "Nourish with the glyph and two hots", heal(glyphs(proto.DruidMajorGlyph_GlyphOfNourish)),
		bare+math.Trunc(float64(float32(bare)*12/100)), 0)
}

// Glyph of Rapid Rejuvenation (71013): haste shortens the hot and its ticks, keeping the tick count.
func TestGlyphOfRapidRejuvenation(t *testing.T) {
	var castSpeed float64
	length := func(g *proto.Glyphs) (time.Duration, int32) {
		sim, druid := healSetup{dummies: 1, glyphs: g}.build(t)
		startFight(sim)
		druid.MultiplyCastSpeed(1.25)
		castSpeed = druid.CastSpeed
		target := targetDummies(sim)[0]
		druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
		hot := druid.Rejuvenation.Hot(target)
		return hot.ExpiresAt() - sim.CurrentTime, hot.NumberOfTicks
	}
	bare, ticks := length(nil)
	glyphed, glyphedTicks := length(glyphs(proto.DruidMajorGlyph_GlyphOfRapidRejuvenation))
	near(t, "the glyphed hot's length over the bare one", float64(glyphed)/float64(bare), castSpeed, 0.002)
	if ticks != glyphedTicks {
		t.Errorf("the glyph changes the tick count from %d to %d", ticks, glyphedTicks)
	}
}

// Unit::SpellPctHealingModsDone: Glyph of Regrowth (54743) adds 20% to Regrowth's heal while the target has a
// Regrowth, and to the hot's amount as it is worked out.
func TestGlyphOfRegrowth(t *testing.T) {
	run := func(g *proto.Glyphs, regrowthUp bool) (heal, tick float64) {
		sim, druid := healSetup{dummies: 1, glyphs: g}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		neverCrit(sim, druid, druid.Regrowth.Spell)
		if regrowthUp {
			druid.Regrowth.SkipCastAndApplyEffects(sim, target)
		}
		heal = castHeal(sim, druid.Regrowth.Spell, target)
		return heal, druid.Regrowth.Hot(target).SnapshotBaseDamage * druid.Regrowth.Hot(target).SnapshotAttackerMultiplier
	}
	glyph := glyphs(proto.DruidMajorGlyph_GlyphOfRegrowth)
	bareHeal, bareTick := run(nil, true)
	glyphHeal, glyphTick := run(glyph, true)
	near(t, "the heal over an up Regrowth", glyphHeal/bareHeal, 1.2, 0.001)
	near(t, "the hot's amount over an up Regrowth", glyphTick/bareTick, 1.2, 0.001)

	firstHeal, firstTick := run(glyph, false)
	bareFirstHeal, bareFirstTick := run(nil, false)
	if firstHeal != bareFirstHeal || firstTick != bareFirstTick {
		t.Errorf("the glyph moves a first Regrowth: heal %v to %v, tick %v to %v", bareFirstHeal, firstHeal, bareFirstTick, firstTick)
	}
}

// spell_dru_glyph_of_rejuvenation: a tick on a target under 50% health casts 54755 for half of it.
func TestGlyphOfRejuvenation(t *testing.T) {
	run := func(g *proto.Glyphs, hurt bool) (*RestorationDruid, float64) {
		sim, druid := healSetup{glyphs: g}.build(t)
		startFight(sim)
		if hurt {
			druid.RemoveHealth(sim, druid.MaxHealth()*0.6)
		}
		druid.Rejuvenation.SkipCastAndApplyEffects(sim, &druid.Unit)
		before := healed(druid.Rejuvenation.Spell, &druid.Unit)
		druid.Rejuvenation.Hot(&druid.Unit).TickOnce(sim)
		return druid, healed(druid.Rejuvenation.Spell, &druid.Unit) - before
	}
	glyph := glyphs(proto.DruidMajorGlyph_GlyphOfRejuvenation)
	druid, tick := run(glyph, true)
	if got, want := healed(druid.GetSpell(core.ActionID{SpellID: 54755}), &druid.Unit), math.Trunc(tick*50/100); got != want {
		t.Errorf("the glyph healed %v on a hurt target, want %v", got, want)
	}
	druid, _ = run(glyph, false)
	if got := healed(druid.GetSpell(core.ActionID{SpellID: 54755}), &druid.Unit); got != 0 {
		t.Errorf("the glyph healed %v on a healthy target", got)
	}
}

// Spell::EffectHeal: Swiftmend takes the hot off unless the caster has Glyph of Swiftmend (54824).
func TestGlyphOfSwiftmendKeepsTheHot(t *testing.T) {
	for _, c := range []struct {
		glyphs *proto.Glyphs
		kept   bool
	}{{nil, false}, {glyphs(proto.DruidMajorGlyph_GlyphOfSwiftmend), true}} {
		sim, druid := healSetup{dummies: 1, glyphs: c.glyphs}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
		druid.Swiftmend.SkipCastAndApplyEffects(sim, target)
		if got := druid.Rejuvenation.Hot(target).IsActive(); got != c.kept {
			t.Errorf("Rejuvenation is up after Swiftmend: %v, want %v", got, c.kept)
		}
	}
}

// spell_dru_wild_growth: Glyph of Wild Growth (62970) makes it six targets.
func TestGlyphOfWildGrowth(t *testing.T) {
	count := func(g *proto.Glyphs) int {
		sim, druid := healSetup{dummies: 9, glyphs: g}.build(t)
		startFight(sim)
		druid.WildGrowth.SkipCastAndApplyEffects(sim, targetDummies(sim)[0])
		var n int
		for _, unit := range sim.Raid.AllUnits {
			if druid.WildGrowth.Hot(unit).IsActive() {
				n++
			}
		}
		return n
	}
	if bare, glyphed := count(nil), count(glyphs(proto.DruidMajorGlyph_GlyphOfWildGrowth)); bare != 5 || glyphed != 6 {
		t.Errorf("Wild Growth went on %d units bare and %d with the glyph, want 5 and 6", bare, glyphed)
	}
}

// spell_dru_glyph_of_innervate: 45% of the base mana over 10 s on every Innervate cast, on whoever it goes.
func TestGlyphOfInnervate(t *testing.T) {
	for _, c := range []struct {
		name   string
		target *proto.UnitReference
		other  bool
	}{
		{"self", &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0}, false},
		{"another player", &proto.UnitReference{Type: proto.UnitReference_Player, Index: 5}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			sim, druid := healSetup{innervate: c.target, other: c.other, dummies: 1, apl: autocast,
				glyphs: glyphs(proto.DruidMajorGlyph_GlyphOfInnervate)}.build(t)
			startFight(sim)
			druid.SpendMana(sim, druid.CurrentMana()-100, druid.NewManaMetrics(core.ActionID{OtherID: 1}))
			druid.GetUnit(c.target).SpendMana(sim, druid.GetUnit(c.target).CurrentMana()-100, druid.NewManaMetrics(core.ActionID{OtherID: 1}))
			runUntil(sim, 20*time.Second)

			var gained float64
			for _, r := range druid.Metrics.ToProto().Resources {
				if r.Id.GetSpellId() == 54833 && r.Gain > 0 {
					gained += r.Gain
				}
			}
			near(t, "the glyph's mana", gained, 10*math.Floor(druid.BaseMana*45/100/10), 0.01)
		})
	}
}

// T7 2 piece (60135): Lifebloom costs 5% less. The 4 piece's 60137 is a class script no server code reads.
func TestT7RestorationBonus(t *testing.T) {
	_, bare := healSetup{}.build(t)
	_, two := healSetup{items: t7Regalia[:2]}.build(t)
	near(t, "Lifebloom's cost with T7", two.Lifebloom.StaticCost()/bare.Lifebloom.StaticCost(), 0.95/1, 0.01)
}

// T8: Swiftmend heals 10% more (2 pieces), and a Rejuvenation cast heals for 169 and 0.188 of the healing power
// (4 pieces, 64801). 64801 can't crit and takes Master Shapeshifter's healing done in Tree of Life.
func TestT8RestorationBonuses(t *testing.T) {
	sim, druid := healSetup{dummies: 1, items: t8Regalia[:2]}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid, druid.Swiftmend.Spell)
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	hot := druid.Rejuvenation.Hot(target)
	untouched := 1 + 4*hot.TickHealingTaken(target, hot.HotTickHealing())
	near(t, "Swiftmend with T8's 2 piece", castHeal(sim, druid.Swiftmend.Spell, target),
		math.Trunc(float64(float32(untouched)*(float32(1)+float32(10)/100))), 0)

	t.Run("4 pieces", func(t *testing.T) { t8Rejuvenation(t) })
	t.Run("4 pieces in Tree of Life", func(t *testing.T) {
		sim, druid := healSetup{talents: restoTalents(true), dummies: 1, items: t8Regalia[:4]}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		neverCrit(sim, druid)
		druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
		heal := druid.GetSpell(core.ActionID{SpellID: 64801})
		want := 169 + core.PowerBonus(0.188*rank(5, 0.04), heal.HealingPower(target))
		near(t, "the T8 heal in Tree of Life", healed(heal, target), math.Floor(math.Floor(want*shapeshifter)*1.06), 1)
	})
}

func t8Rejuvenation(t *testing.T) {
	sim, druid := healSetup{dummies: 1, items: t8Regalia[:4]}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	heal := druid.GetSpell(core.ActionID{SpellID: 64801})
	want := 169 + core.PowerBonus(0.188*rank(5, 0.04), heal.HealingPower(target))
	near(t, "the T8 heal", healed(heal, target), math.Floor(want), 1)
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	near(t, "the T8 heals after a refresh", healed(heal, target), 2*math.Floor(want), 2)

	// SPELL_ATTR2_CANT_CRIT holds at a certain crit too
	druid.AddStatDynamic(sim, stats.SpellCrit, 2e6)
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	near(t, "the T8 heals at a certain crit", healed(heal, target), 3*math.Floor(want), 3)
	if crits := heal.SpellMetrics[target.UnitIndex].Crits; crits != 0 {
		t.Errorf("the T8 heal crit %d times", crits)
	}
}

// T9: Nourish crits 5% more (2 pieces), Rejuvenation's ticks can crit (4).
func TestT9RestorationBonuses(t *testing.T) {
	_, bare := healSetup{dummies: 1}.build(t)
	sim, two := healSetup{dummies: 1, items: t9Garb[:2]}.build(t)
	_, four := healSetup{dummies: 1, items: t9Garb[:4]}.build(t)
	near(t, "the Nourish crit bonus", (two.Nourish.BonusCritRating-bare.Nourish.BonusCritRating)/core.CritRatingPerCritChance, 5, 1e-9)
	target := targetDummies(sim)[0]
	if bare.Rejuvenation.Hot(target).TicksCanCrit || two.Rejuvenation.Hot(target).TicksCanCrit || !four.Rejuvenation.Hot(target).TicksCanCrit {
		t.Error("Rejuvenation's ticks crit bare or with 2 pieces, or can't with 4")
	}

	// the ticks crit on the chance the hot took as it went on or refreshed
	sim, four = healSetup{dummies: 1, items: t9Garb[:4]}.build(t)
	startFight(sim)
	target = targetDummies(sim)[0]
	neverCrit(sim, four)
	four.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	hot := four.Rejuvenation.Hot(target)
	amount := math.Floor(hot.SnapshotBaseDamage * hot.SnapshotAttackerMultiplier)
	four.AddStatDynamic(sim, stats.SpellCrit, 2e6)
	hot.TickOnce(sim)
	near(t, "a tick of a Rejuvenation that went on at no crit", healed(four.Rejuvenation.Spell, target), amount, 0)
	four.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	hot.TickOnce(sim)
	near(t, "a tick after a refresh at a certain crit", healed(four.Rejuvenation.Spell, target)-amount, math.Floor(amount*1.5), 1)
}

// spell_dru_wild_growth_aura: T10's 2 piece takes 30% off the 2 percent a tick the bonus declines by. The script
// keeps the decay on the aura and takes the 30% off again whenever the amount is worked out, so a refresh slows it
// once more and only a new aura starts over.
func TestT10WildGrowthDecaysSlower(t *testing.T) {
	ticks := func(base float32, decay float32, n int) (sum float64) {
		for tick := range n {
			bonus := float32(6) - decay*float32(tick)
			sum += float64(int32(base + float32(base*bonus)/100))
		}
		return sum
	}
	cut := func(decay float32) float32 { return decay + float32(decay*-30)/100 }

	sim, druid := healSetup{dummies: 1, items: t10Garb[:2]}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, druid)
	hot := druid.WildGrowth.Hot(target)
	castAndRun := func(until time.Duration) float64 {
		before := healed(druid.WildGrowth.Spell, target)
		druid.WildGrowth.SkipCastAndApplyEffects(sim, target)
		runUntil(sim, until)
		return healed(druid.WildGrowth.Spell, target) - before
	}

	first := castAndRun(6500 * time.Millisecond)
	base := float32(hot.SnapshotBaseDamage * hot.SnapshotAttackerMultiplier)
	near(t, "6 ticks of a new Wild Growth", first, ticks(base, cut(2), 6), 3)
	near(t, "7 ticks after a refresh at 6.5 s", castAndRun(14*time.Second), ticks(base, cut(cut(2)), 7), 3)
	near(t, "7 ticks of the next new one", castAndRun(22*time.Second), ticks(base, cut(2), 7), 3)
	if ticks(base, cut(2), 7) <= ticks(base, 2, 7) {
		t.Errorf("the slower decay heals no more than the bare one")
	}

	sim, bare := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target = targetDummies(sim)[0]
	neverCrit(sim, bare)
	bare.WildGrowth.SkipCastAndApplyEffects(sim, target)
	runUntil(sim, 6500*time.Millisecond)
	before := healed(bare.WildGrowth.Spell, target)
	bare.WildGrowth.SkipCastAndApplyEffects(sim, target)
	bareHot := bare.WildGrowth.Hot(target)
	base = float32(bareHot.SnapshotBaseDamage * bareHot.SnapshotAttackerMultiplier)
	runUntil(sim, 14*time.Second)
	near(t, "7 ticks after a refresh without T10", healed(bare.WildGrowth.Spell, target)-before, ticks(base, 2, 7), 3)
}

// spell_dru_t10_restoration_4p_bonus_dummy: 2% of Rejuvenation's ticks cast Blessing of the Eternal for the tick's
// heal. spell_dru_t10_restoration_4p_bonus puts it on the raid member with the lowest health percent that has no
// druid periodic heal of Rejuvenation's icon, a tie in raid order. 70691 ignores the caster's modifiers, so each
// of its ticks is the Rejuvenation tick's heal, and Nature's Splendor makes it 6 ticks.
func TestT10BlessingOfTheEternal(t *testing.T) {
	sim, druid := healSetup{dummies: 3, items: t10Garb[:4]}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	neverCrit(sim, druid)
	blessing := druid.BlessingOfTheEternal
	casts := func(unit *core.Unit) int32 { return blessing.SpellMetrics[unit.UnitIndex].Casts }

	druid.Rejuvenation.SkipCastAndApplyEffects(sim, dummies[0])
	hot := druid.Rejuvenation.Hot(dummies[0])
	onDruid := blessing.Hot(&druid.Unit)
	var snapshot float64
	var ticks int32
	const tickCount = 3000
	for range tickCount {
		hot.TickOnce(sim)
		// taken off again so the next proc finds the druid free too
		if onDruid.IsActive() {
			snapshot, ticks = onDruid.SnapshotBaseDamage*onDruid.SnapshotAttackerMultiplier, onDruid.NumberOfTicks
			onDruid.Cancel(sim)
		}
	}
	all := casts(&druid.Unit)
	for _, d := range dummies {
		all += casts(d)
	}
	if all < 35 || all > 90 {
		t.Errorf("%d procs in %d ticks, want about 60", all, tickCount)
	}
	// all at full health: the druid is first in raid order
	if got := casts(&druid.Unit); got != all {
		t.Errorf("%d of %d went on the druid", got, all)
	}
	near(t, "Blessing of the Eternal's tick", snapshot, healed(druid.Rejuvenation.Spell, dummies[0])/tickCount, 0)
	if want := hot.NumberOfTicks; ticks != want {
		t.Errorf("Blessing of the Eternal has %d ticks, want Rejuvenation's %d", ticks, want)
	}
}

// Blessing of the Eternal skips a unit with a Rejuvenation, the one whose tick procced it included, or a Blessing of
// the Eternal, but not one with only a Regrowth.
func TestT10BlessingOfTheEternalSkipsRejuvenations(t *testing.T) {
	sim, druid := healSetup{dummies: 3, damage: 1, items: t10Garb[:4]}.build(t)
	startFight(sim)
	neverCrit(sim, druid)
	// 77.5%, 95% and 85%, the druid at full
	dummies := hurtDummies(sim, 9000, 1000, 3000)
	druid.Regrowth.Hot(dummies[2]).Apply(sim)
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, dummies[0])
	hot := druid.Rejuvenation.Hot(dummies[0])

	blessing := druid.BlessingOfTheEternal
	units := []*core.Unit{&druid.Unit, dummies[0], dummies[1], dummies[2]}
	total := func() (n int32) {
		for _, unit := range units {
			n += blessing.SpellMetrics[unit.UnitIndex].Casts
		}
		return n
	}
	for i, want := range []*core.Unit{dummies[2], dummies[1], &druid.Unit} {
		for j := 0; total() == int32(i) && j < 10000; j++ {
			hot.TickOnce(sim)
		}
		if got := blessing.SpellMetrics[want.UnitIndex].Casts; total() != int32(i+1) || got != 1 {
			t.Fatalf("proc %d didn't go on %s", i+1, want.Label)
		}
	}
}

// Swiftmend can eat Blessing of the Eternal, which has Rejuvenation's family flag, and Revitalize rolls on its
// ticks.
func TestBlessingOfTheEternalCountsAsARejuvenation(t *testing.T) {
	sim, druid := healSetup{dummies: 1, items: t10Garb[:4]}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	druid.PseudoStats.SpiritRegenMultiplier = 0
	druid.UpdateManaRegenRates()
	drain := druid.NewManaMetrics(core.ActionID{OtherID: 1})
	druid.SpendMana(sim, druid.CurrentMana(), drain)

	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	hot := druid.Rejuvenation.Hot(target)
	blessing := druid.BlessingOfTheEternal.Hot(&druid.Unit)
	for i := 0; !blessing.IsActive() && i < 10000; i++ {
		hot.TickOnce(sim)
	}
	if !blessing.IsActive() {
		t.Fatal("Blessing of the Eternal never went on the druid")
	}
	hot.Cancel(sim)

	const ticks = 600
	for range ticks {
		blessing.TickOnce(sim)
	}
	gains := druid.CurrentMana() / math.Floor(druid.MaxMana()/100)
	if se := math.Sqrt(ticks * 0.15 * 0.85); math.Abs(gains-0.15*ticks) > 5*se {
		t.Errorf("Revitalize gave mana on %v of %v Blessing of the Eternal ticks, want about %v", gains, ticks, 0.15*ticks)
	}

	druid.AddMana(sim, druid.MaxMana(), drain)
	if !druid.Swiftmend.CanCast(sim, &druid.Unit) {
		t.Fatal("Swiftmend can't be cast on a target with only Blessing of the Eternal")
	}
	druid.Swiftmend.SkipCastAndApplyEffects(sim, &druid.Unit)
	if blessing.IsActive() {
		t.Error("Swiftmend left Blessing of the Eternal up")
	}
}

// The relics' flat mods: Budding Life and Awakening cost, Pure Thoughts' Rejuvenation tick, Lush Moss' Lifebloom
// tick and the Flourishing Life's Nourish heal.
func TestRelicFlatMods(t *testing.T) {
	_, bare := healSetup{dummies: 1}.build(t)
	for _, c := range []struct {
		id   int32
		flat float64
	}{{33508, 36}, {40342, 106}} {
		_, idol := healSetup{dummies: 1, items: []int32{c.id}}.build(t)
		scale := bare.Rejuvenation.StaticCost() / float64(bare.Rejuvenation.DefaultCast.Cost)
		near(t, "the idol's cost saving", bare.Rejuvenation.StaticCost()-idol.Rejuvenation.StaticCost(), c.flat*scale, 1.5)
	}

	tick := func(items []int32, hot func(*RestorationDruid) *core.Spell) float64 {
		sim, druid := healSetup{dummies: 1, items: items}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		neverCrit(sim, druid)
		spell := hot(druid)
		spell.SkipCastAndApplyEffects(sim, target)
		return spell.Hot(target).SnapshotBaseDamage * spell.Hot(target).SnapshotAttackerMultiplier
	}
	rejuv := func(d *RestorationDruid) *core.Spell { return d.Rejuvenation.Spell }
	bloom := func(d *RestorationDruid) *core.Spell { return d.Lifebloom.Spell }
	// Player::ApplySpellMod adds a flat after the percents
	near(t, "Pure Thoughts' Rejuvenation tick", tick([]int32{38366}, rejuv)-tick(nil, rejuv), 33, 1)
	near(t, "Lush Moss' Lifebloom tick", tick([]int32{40711}, bloom)-tick(nil, bloom), 17, 1)

	nourish := func(items []int32) float64 {
		sim, druid := healSetup{dummies: 1, items: items}.build(t)
		startFight(sim)
		neverCrit(sim, druid, druid.Nourish.Spell)
		return castHeal(sim, druid.Nourish.Spell, targetDummies(sim)[0])
	}
	near(t, "the Flourishing Life's Nourish", nourish([]int32{46138})-nourish(nil), 187, 1)
}

// Idol of Tenacity: a flat SPELLMOD_EFFECT2 on the bloom, per stack.
func TestIdolOfTenacityRaisesTheBloom(t *testing.T) {
	bloom := func(items []int32) float64 {
		sim, druid := healSetup{dummies: 1, items: items}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		neverCrit(sim, druid, druid.LifebloomBloom.Spell)
		druid.Lifebloom.SkipCastAndApplyEffects(sim, target)
		runUntil(sim, 30*time.Second)
		return healed(druid.LifebloomBloom.Spell, target)
	}
	near(t, "Wrathful Gladiator's Idol of Tenacity's bloom", bloom([]int32{51423})-bloom(nil), 448*giftOfNature, 1.5)
}

// Idol of Flaring Growth (70%, 5 s apart) and Idol of the Black Willow (every tick, up to 8 stacks) proc on
// Rejuvenation's ticks.
func TestRelicProcsOnRejuvenationTicks(t *testing.T) {
	sim, druid := healSetup{dummies: 1, items: []int32{47671}}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	hot := druid.Rejuvenation.Hot(target)
	spellPower := druid.GetStat(stats.SpellPower)
	for range 30 {
		hot.TickOnce(sim)
		if druid.GetAura("Rejuvenating").IsActive() {
			break
		}
	}
	if !druid.GetAura("Rejuvenating").IsActive() {
		t.Fatal("Idol of Flaring Growth never procs")
	}
	near(t, "the proc's spell power", druid.GetStat(stats.SpellPower)-spellPower, 234, 0)

	sim, druid = healSetup{dummies: 1, items: []int32{50454}}.build(t)
	startFight(sim)
	target = targetDummies(sim)[0]
	druid.Rejuvenation.SkipCastAndApplyEffects(sim, target)
	hot = druid.Rejuvenation.Hot(target)
	spellPower = druid.GetStat(stats.SpellPower)
	for range 12 {
		hot.TickOnce(sim)
	}
	if got := druid.GetAura("Soothing").GetStacks(); got != 8 {
		t.Errorf("Soothing has %d stacks after 12 ticks, want 8", got)
	}
	near(t, "Soothing's spell power", druid.GetStat(stats.SpellPower)-spellPower, 8*32, 0)
}
