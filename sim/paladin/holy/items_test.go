package holy

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

var (
	redemptionRegalia = []int32{40569, 40570, 40571, 40572, 40573}
	aegisRegalia      = []int32{45370, 45371, 45372, 45373, 45374}
	turalyonsGarb     = []int32{48564, 48566, 48568, 48572, 48574}
	lightswornGarb    = []int32{50865, 50866, 50867, 50868, 50869}
)

func majorGlyph(g proto.PaladinMajorGlyph) *proto.Glyphs {
	return &proto.Glyphs{Major1: int32(g)}
}

// spellPowerGain is the spell power over base, rounded to a whole point.
func spellPowerGain(holy *HolyPaladin, base float64) float64 {
	return math.Round(holy.GetStat(stats.SpellPower) - base)
}

func holyLightCost(holy *HolyPaladin) float64 {
	return holy.HolyLight.ApplyCostModifiers(holy.HolyLight.DefaultCast.Cost)
}

// Glyph of Holy Light (54937): a Holy Light that heals casts 54968 on its target, 10% of the heal on up to five
// other raid units, the target not among them.
func TestGlyphOfHolyLightHealsFiveOthers(t *testing.T) {
	sim, holy := healSetup{dummies: 7, glyphs: majorGlyph(proto.PaladinMajorGlyph_GlyphOfHolyLight)}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	setCrit(sim, holy, -100)
	fixRolls(holy.HolyLight)
	glyph := holy.GetSpell(core.ActionID{SpellID: 54968})

	heal := castHeal(sim, holy.HolyLight, holy.HolyLight, target)
	want := math.Trunc(heal / 10)
	if got := healed(glyph, target); got != 0 {
		t.Errorf("the glyph healed the Holy Light's target for %v", got)
	}
	var units, total int
	for _, unit := range append(targetDummies(sim), &holy.Unit) {
		if got := healed(glyph, unit); got != 0 {
			units++
			total += int(got)
			if want := math.Trunc(want * unit.PseudoStats.HealingTakenMultiplier); got != want {
				t.Errorf("%s got %v from the glyph, want %v", unit.Label, got, want)
			}
		}
	}
	if units != 5 {
		t.Errorf("the glyph healed %d units, want 5", units)
	}
	if crits(glyph, &holy.Unit) != 0 {
		t.Errorf("the glyph's heal crit")
	}

	_, bare := healSetup{dummies: 1}.build(t)
	if bare.GetSpell(core.ActionID{SpellID: 54968}) != nil {
		t.Errorf("54968 is registered without the glyph")
	}
}

// Glyph of Flash of Light (54936): +5% crit on Flash of Light alone.
func TestGlyphOfFlashOfLightAddsCrit(t *testing.T) {
	_, with := healSetup{glyphs: majorGlyph(proto.PaladinMajorGlyph_GlyphOfFlashOfLight)}.build(t)
	_, without := healSetup{}.build(t)
	if got := (with.FlashOfLight.BonusCritRating - without.FlashOfLight.BonusCritRating) / core.CritRatingPerCritChance; math.Abs(got-5) > 1e-9 {
		t.Errorf("the glyph adds %v%% crit to Flash of Light, want 5%%", got)
	}
	if with.HolyLight.BonusCritRating != without.HolyLight.BonusCritRating {
		t.Errorf("the glyph changed Holy Light's crit")
	}
}

// Glyph of Holy Shock (63224): 1 s off the 6 s cooldown.
func TestGlyphOfHolyShockCutsTheCooldown(t *testing.T) {
	_, with := healSetup{glyphs: majorGlyph(proto.PaladinMajorGlyph_GlyphOfHolyShock)}.build(t)
	_, without := healSetup{}.build(t)
	if with.HolyShock.CD.Duration != 5*time.Second || without.HolyShock.CD.Duration != 6*time.Second {
		t.Errorf("Holy Shock's cooldown is %v with the glyph, %v without; want 5 s and 6 s", with.HolyShock.CD.Duration, without.HolyShock.CD.Duration)
	}
}

// Glyph of Beacon of Light (63218): 30 s more, 90 s.
func TestGlyphOfBeaconOfLightLengthensIt(t *testing.T) {
	sim, holy := healSetup{dummies: 2, glyphs: majorGlyph(proto.PaladinMajorGlyph_GlyphOfBeaconOfLight)}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	cast(sim, holy.BeaconOfLight, dummies[1])
	runUntil(sim, 80*time.Second)
	castHeal(sim, holy.HolyLight, holy.HolyLight, dummies[0])
	if healed(holy.GetSpell(core.ActionID{SpellID: 53652}), dummies[1]) == 0 {
		t.Errorf("the beacon was gone at 80 s")
	}
	runUntil(sim, 91*time.Second)
	if holy.BeaconOfLightAuras.Get(dummies[1]).IsActive() {
		t.Errorf("the beacon outlived 90 s")
	}
}

// Glyph of Seal of Wisdom (54940): 5% off Holy Light, Flash of Light, Holy Shock and Beacon of Light while Seal of
// Wisdom is up.
func TestGlyphOfSealOfWisdomCutsHealCosts(t *testing.T) {
	sim, holy := healSetup{glyphs: majorGlyph(proto.PaladinMajorGlyph_GlyphOfSealOfWisdom)}.build(t)
	startFight(sim)
	spells := []*core.Spell{holy.HolyLight, holy.FlashOfLight, holy.HolyShock, holy.BeaconOfLight}
	full := make([]float64, len(spells))
	for i, spell := range spells {
		full[i] = spell.ApplyCostModifiers(spell.DefaultCast.Cost)
	}
	holy.SealOfWisdomAura.Activate(sim)
	for i, spell := range spells {
		got := spell.ApplyCostModifiers(spell.DefaultCast.Cost)
		if want := math.Trunc(full[i] / (1 - 0.02*float64(holy.Talents.Benediction)) * (1 - 0.02*float64(holy.Talents.Benediction) - 0.05)); got != want {
			t.Errorf("%s costs %v under Seal of Wisdom, want %v", spell.ActionID, got, want)
		}
	}
	holy.SealOfWisdomAura.Deactivate(sim)
	for i, spell := range spells {
		if got := spell.ApplyCostModifiers(spell.DefaultCast.Cost); got != full[i] {
			t.Errorf("%s costs %v after the seal, want %v", spell.ActionID, got, full[i])
		}
	}
}

// Glyph of Seal of Light (54943): Holy Light, Flash of Light and Holy Shock's heal heal 5% more while Seal of Light
// is up, Holy Shock's damage not.
func TestGlyphOfSealOfLightAddsHealing(t *testing.T) {
	sim, holy := healSetup{dummies: 1, glyphs: majorGlyph(proto.PaladinMajorGlyph_GlyphOfSealOfLight)}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	setCrit(sim, holy, -100)
	fixRolls(holy.HolyLight, holy.FlashOfLight, holy.HolyShockHeal)
	damage := holy.HolyShockDamage.DamageMultiplier

	var before [3]float64
	pairs := []struct{ spell, heal *core.Spell }{
		{holy.HolyLight, holy.HolyLight}, {holy.FlashOfLight, holy.FlashOfLight}, {holy.HolyShock, holy.HolyShockHeal},
	}
	for i, p := range pairs {
		before[i] = castHeal(sim, p.spell, p.heal, target)
	}
	holy.SealOfLightAura.Activate(sim)
	for i, p := range pairs {
		if got := castHeal(sim, p.spell, p.heal, target); math.Abs(got/before[i]-1.05) > 0.001 {
			t.Errorf("%s heals %v under Seal of Light, %v without; want 5%% more", p.heal.ActionID, got, before[i])
		}
	}
	if holy.HolyShockDamage.DamageMultiplier != damage {
		t.Errorf("Holy Shock's damage changed under Seal of Light")
	}
	holy.SealOfLightAura.Deactivate(sim)
	if got := castHeal(sim, holy.HolyLight, holy.HolyLight, target); got != before[0] {
		t.Errorf("Holy Light heals %v after the seal, want %v", got, before[0])
	}
}

// Glyph of the Wise (57954): Seal of Wisdom costs half as much.
func TestGlyphOfTheWiseHalvesSealOfWisdom(t *testing.T) {
	_, with := healSetup{glyphs: &proto.Glyphs{Minor1: int32(proto.PaladinMinorGlyph_GlyphOfTheWise)}}.build(t)
	_, without := healSetup{}.build(t)
	a, b := with.SealOfWisdom.StaticCost(), without.SealOfWisdom.StaticCost()
	if want := math.Trunc(b / (1 - 0.02*float64(without.Talents.Benediction)) * (1 - 0.02*float64(without.Talents.Benediction) - 0.5)); a != want {
		t.Errorf("Seal of Wisdom costs %v with the glyph, %v without; want %v", a, b, want)
	}
	if with.SealOfLight.StaticCost() != without.SealOfLight.StaticCost() {
		t.Errorf("the glyph changed Seal of Light's cost")
	}
}

// T7 holy: 2 piece +10% crit on both of Holy Shock's, 4 piece 5% off Holy Light.
func TestRedemptionRegalia(t *testing.T) {
	_, with := healSetup{items: redemptionRegalia}.build(t)
	_, without := healSetup{}.build(t)
	for _, c := range []struct {
		name          string
		with, without *core.Spell
	}{
		{"Holy Shock heal", with.HolyShockHeal, without.HolyShockHeal},
		{"Holy Shock damage", with.HolyShockDamage, without.HolyShockDamage},
	} {
		if got := (c.with.BonusCritRating - c.without.BonusCritRating) / core.CritRatingPerCritChance; math.Abs(got-10) > 1e-9 {
			t.Errorf("%s: the 2 piece adds %v%% crit, want 10%%", c.name, got)
		}
	}
	if got := (with.HolyLight.BonusCritRating - without.HolyLight.BonusCritRating); got != 0 {
		t.Errorf("the 2 piece changed Holy Light's crit")
	}
	if got, want := with.HolyLight.StaticCost(), math.Trunc(without.HolyLight.StaticCost()*0.95); got != want {
		t.Errorf("Holy Light costs %v with the 4 piece, want %v", got, want)
	}
}

// T8 holy 2 piece (spell_pal_t8_2p_bonus): a Holy Shock crit puts 64891 on its target, int32(int32(heal × 15%) / 3) a
// tick, 3 ticks, with the paladin's healing done percents.
func TestAegisRegalia2PieceHolyMending(t *testing.T) {
	sim, holy := healSetup{items: aegisRegalia, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	mending := holy.GetSpell(core.ActionID{SpellID: 64891})
	setCrit(sim, holy, -100)
	castHeal(sim, holy.HolyShock, holy.HolyShockHeal, target)
	if healed(mending, target) != 0 || mending.Hot(target).IsActive() {
		t.Fatalf("Holy Mending came off a Holy Shock that didn't crit")
	}

	setCrit(sim, holy, 200)
	heal := castHeal(sim, holy.HolyShock, holy.HolyShockHeal, target)
	runUntil(sim, sim.CurrentTime+10*time.Second)
	amount := math.Trunc(math.Trunc(heal*15/100) / 3)
	tick := math.Trunc(float64(float32(amount) * (1 + 0.01*float32(holy.Talents.Divinity))))
	if got := healed(mending, target); got != 3*tick || tick == 0 {
		t.Errorf("Holy Mending healed %v off a %v crit, want 3 × %v", got, heal, tick)
	}
}

// T8 holy 4 piece: Sacred Shield's absorb comes every 4 s whatever Divine Guardian says.
func TestAegisRegalia4PieceSacredShieldCooldown(t *testing.T) {
	_, holy := healSetup{items: aegisRegalia, talents: ""}.build(t)
	if got := holy.SacredShieldCooldown(); got != 4*time.Second {
		t.Errorf("Sacred Shield's cooldown is %v with the 4 piece, want 4 s", got)
	}
}

// T9 holy 2 piece (67190): the paladin's Judgements of Light and Wisdom last 10 s longer.
func TestTuralyonsGarb2PieceLengthensJudgements(t *testing.T) {
	for _, c := range []struct {
		items []int32
		want  time.Duration
	}{{nil, 20 * time.Second}, {turalyonsGarb, 30 * time.Second}} {
		sim, holy := healSetup{items: c.items}.build(t)
		startFight(sim)
		enemy := sim.Encounter.TargetUnits[0]
		cast(sim, holy.SealOfWisdom, &holy.Unit)
		cast(sim, holy.JudgementOfWisdom, enemy)
		cast(sim, holy.JudgementOfLight, enemy)
		for _, label := range []string{core.JudgementOfWisdomAuraLabel, core.JudgementOfLightAuraLabel} {
			aura := enemy.GetAura(label)
			if aura == nil || !aura.IsActive() {
				t.Fatalf("%d pieces: %s isn't up", len(c.items), label)
			}
			// the aura ends on the next update after its time
			if got := aura.RemainingDuration(sim); (got - c.want).Abs() > 100*time.Millisecond {
				t.Errorf("%d pieces: %s has %v left, want %v", len(c.items), label, got, c.want)
			}
		}
	}
}

// T9 holy 4 piece (67191): 66922's tick doubles.
func TestTuralyonsGarb4PieceDoublesTheFlashOfLightHot(t *testing.T) {
	sim, holy := healSetup{items: turalyonsGarb, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	hot := holy.GetSpell(core.ActionID{SpellID: 66922})
	setSpellPower(sim, holy, 3000)

	cast(sim, holy.SacredShield, target)
	heal := castHeal(sim, holy.FlashOfLight, holy.FlashOfLight, target)
	runUntil(sim, sim.CurrentTime+13*time.Second)
	base := 2 * math.Trunc(heal/12)
	tick := math.Trunc(float64(float32(base)*(1+0.01*float32(holy.Talents.Divinity))) * target.PseudoStats.HealingTakenMultiplier)
	if got := hot.SpellMetrics[target.UnitIndex].TotalHealing; got != 12*tick || tick == 0 {
		t.Errorf("the hot healed %v off a %v Flash of Light, want 12 × %v", got, heal, tick)
	}
}

// T10 holy 2 piece (71166): Divine Illumination adds 35% healing done while it lasts.
func TestLightswornGarb2PieceAddsHealingUnderDivineIllumination(t *testing.T) {
	sim, holy := healSetup{items: lightswornGarb, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	setCrit(sim, holy, -100)
	setSpellPower(sim, holy, 3000)
	fixRolls(holy.HolyLight)

	plain := castHeal(sim, holy.HolyLight, holy.HolyLight, target)
	cast(sim, holy.DivineIllumination, &holy.Unit)
	boosted := castHeal(sim, holy.HolyLight, holy.HolyLight, target)
	if math.Abs(boosted/plain-1.35) > 0.001 {
		t.Errorf("Holy Light heals %v under Divine Illumination, %v without; want 35%% more", boosted, plain)
	}
	runUntil(sim, 16*time.Second)
	if got := castHeal(sim, holy.HolyLight, holy.HolyLight, target); got != plain {
		t.Errorf("Holy Light heals %v after Divine Illumination, want %v", got, plain)
	}
}

// T10 holy 4 piece (70756): a Holy Shock heal gives Holiness (70757), 0.3 s off Holy Light's cast for 10 s.
func TestLightswornGarb4PieceHoliness(t *testing.T) {
	sim, holy := healSetup{items: lightswornGarb, dummies: 1, talents: holyTalents(talentLightsGrace, 0)}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]

	plain := hardcast(t, sim, holy, holy.HolyLight, target)
	cast(sim, holy.HolyShock, target)
	boosted := hardcast(t, sim, holy, holy.HolyLight, target)
	// cast times carry a few ms of latency
	if cut := plain - boosted; cut < 250*time.Millisecond || cut > 340*time.Millisecond {
		t.Errorf("Holy Light takes %v under Holiness, %v without; want 0.3 s less", boosted, plain)
	}
	runUntil(sim, sim.CurrentTime+11*time.Second)
	if got := hardcast(t, sim, holy, holy.HolyLight, target); (got - plain).Abs() > 40*time.Millisecond {
		t.Errorf("Holy Light takes %v after Holiness, want %v", got, plain)
	}
}

// Libram of Renewal (60787) takes 113 off Holy Light.
func TestLibramsCutHolyLightCost(t *testing.T) {
	_, bare := healSetup{}.build(t)
	for _, c := range []struct {
		item int32
		off  float64
	}{{40705, 113}} {
		_, holy := healSetup{items: []int32{c.item}}.build(t)
		if got, want := holyLightCost(holy), holyLightCost(bare)-c.off; got != want {
			t.Errorf("libram %d: Holy Light costs %v, want %v", c.item, got, want)
		}
	}
}

// The healing librams' SPELLMOD_DAMAGE flats come after every percent, on their own spell only.
func TestLibramsAddFlatHealing(t *testing.T) {
	for _, c := range []struct {
		item  int32
		spell func(*HolyPaladin) (*core.Spell, *core.Spell)
		flat  float64
	}{
		{45436, func(h *HolyPaladin) (*core.Spell, *core.Spell) { return h.HolyLight, h.HolyLight }, 160},
		{38364, func(h *HolyPaladin) (*core.Spell, *core.Spell) { return h.HolyShock, h.HolyShockHeal }, 69},
	} {
		run := func(items []int32) (own, other float64) {
			sim, holy := healSetup{items: items, dummies: 1}.build(t)
			startFight(sim)
			target := targetDummies(sim)[0]
			setCrit(sim, holy, -100)
			setSpellPower(sim, holy, 2000)
			fixRolls(holy.HolyLight, holy.FlashOfLight, holy.HolyShockHeal)
			cast, heal := c.spell(holy)
			own = castHeal(sim, cast, heal, target)
			if heal == holy.HolyLight {
				other = castHeal(sim, holy.FlashOfLight, holy.FlashOfLight, target)
			} else {
				other = castHeal(sim, holy.HolyLight, holy.HolyLight, target)
			}
			return
		}
		ownWith, otherWith := run([]int32{c.item})
		ownWithout, otherWithout := run(nil)
		if got := ownWith - ownWithout; math.Abs(got-c.flat) > 1 {
			t.Errorf("libram %d adds %v healing, want %v", c.item, got, c.flat)
		}
		if otherWith != otherWithout {
			t.Errorf("libram %d changed another heal: %v, was %v", c.item, otherWith, otherWithout)
		}
	}
}

// Libram of Veracity (67363): a Holy Light has a 70% chance of 234 spell power for 15 s (67364), 8 s apart.
func TestLibramOfVeracity(t *testing.T) {
	sim, holy := healSetup{items: []int32{47662}, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	base := holy.GetStat(stats.SpellPower)
	procs := 0
	for i := 0; i < 20; i++ {
		castHeal(sim, holy.HolyLight, holy.HolyLight, target)
		if spellPowerGain(holy, base) == 234 {
			procs++
			// the 8 s cooldown holds a second proc back inside it
			runUntil(sim, sim.CurrentTime+7*time.Second)
			if aura := holy.GetAura("Holy Judgement"); !aura.IsActive() {
				t.Fatalf("the buff ended inside 15 s")
			}
		}
		runUntil(sim, sim.CurrentTime+16*time.Second)
		if spellPowerGain(holy, base) != 0 {
			t.Fatalf("the spell power is %v over base at %v", spellPowerGain(holy, base), sim.CurrentTime)
		}
	}
	if procs < 8 || procs > 19 {
		t.Errorf("%d procs in 20 Holy Lights, want about 70%%", procs)
	}
	if castHeal(sim, holy.FlashOfLight, holy.FlashOfLight, target); spellPowerGain(holy, base) != 0 {
		t.Errorf("a Flash of Light gave spell power")
	}
}

// Libram of Blinding Light (71191): each Holy Shock heal gives 85 spell power for 15 s (71192), up to 3 times.
func TestLibramOfBlindingLightStacks(t *testing.T) {
	sim, holy := healSetup{items: []int32{50460}, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	base := holy.GetStat(stats.SpellPower)
	for i := 1; i <= 4; i++ {
		cast(sim, holy.HolyShock, target)
		if got, want := spellPowerGain(holy, base), 85*float64(min(i, 3)); got != want {
			t.Errorf("after %d Holy Shocks the libram gives %v spell power, want %v", i, got, want)
		}
	}
	runUntil(sim, sim.CurrentTime+16*time.Second)
	if spellPowerGain(holy, base) != 0 {
		t.Errorf("the spell power outlived 15 s")
	}
}
