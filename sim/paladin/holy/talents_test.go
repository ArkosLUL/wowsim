package holy

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// holy tree positions in the talent string
const (
	talentHealingLight          = 2
	talentIllumination          = 6
	talentSanctifiedLight       = 13
	talentHolyPower             = 15
	talentLightsGrace           = 16
	talentHolyGuidance          = 20
	talentJudgementsOfThePure   = 22
	talentInfusionOfLight       = 23
	talentEnlightenedJudgements = 24
)

// holyTalents is StandardTalents with the holy talent at index set to points.
func holyTalents(index int, points byte) string {
	talents := []byte(StandardTalents)
	talents[index] = '0' + points
	return string(talents)
}

// setCritFor sets the paladin's spell crit so the spell's own chance, its bonuses included, is pct percent.
func setCritFor(sim *core.Simulation, holy *HolyPaladin, spell *core.Spell, pct float64) {
	setCrit(sim, holy, pct-spell.BonusCritRating/core.CritRatingPerCritChance)
}

// hardcast casts the spell once the paladin is free, full on mana, lets it land and returns its cast time.
func hardcast(t *testing.T, sim *core.Simulation, holy *HolyPaladin, spell *core.Spell, target *core.Unit) time.Duration {
	t.Helper()
	if free := max(holy.Hardcast.Expires, holy.GCD.ReadyAt()); free > sim.CurrentTime {
		runUntil(sim, free)
		if sim.CurrentTime < free {
			t.Fatalf("the fight ended at %s, before the paladin was free to cast", sim.CurrentTime)
		}
	}
	// the empty rotation keeps putting the GCD back by a short wait
	holy.GCD.Reset()
	holy.AddMana(sim, holy.MaxMana()-holy.CurrentMana(), holy.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionWait}))
	if !spell.Cast(sim, target) {
		t.Fatalf("%s didn't cast at %s (GCD ready %s, hardcast until %s, mana %v)", spell.ActionID, sim.CurrentTime,
			holy.GCD.ReadyAt(), holy.Hardcast.Expires, holy.CurrentMana())
	}
	castTime := spell.CurCast.CastTime
	landed := sim.CurrentTime + castTime + time.Millisecond
	runUntil(sim, landed)
	if sim.CurrentTime < landed {
		t.Fatalf("the fight ended at %s, before %s landed", sim.CurrentTime, spell.ActionID)
	}
	return castTime
}

// Healing Light's SPELLMOD_DAMAGE, 4% a point, reaches Holy Light, Flash of Light and both of Holy Shock's.
// Sanctified Light's 2% crit a point reaches Holy Light and both of Holy Shock's, not Flash of Light.
func TestHealingLightAndSanctifiedLight(t *testing.T) {
	_, with := healSetup{}.build(t)
	_, without := healSetup{talents: holyTalents(talentHealingLight, 0)}.build(t)
	_, noSanctified := healSetup{talents: holyTalents(talentSanctifiedLight, 0)}.build(t)
	for _, c := range []struct {
		name  string
		spell func(*HolyPaladin) *core.Spell
		crit  float64
	}{
		{"Holy Light", func(h *HolyPaladin) *core.Spell { return h.HolyLight }, 6},
		{"Flash of Light", func(h *HolyPaladin) *core.Spell { return h.FlashOfLight }, 0},
		{"Holy Shock heal", func(h *HolyPaladin) *core.Spell { return h.HolyShockHeal }, 6},
		{"Holy Shock damage", func(h *HolyPaladin) *core.Spell { return h.HolyShockDamage }, 6},
	} {
		if got := c.spell(with).DamageMultiplier / c.spell(without).DamageMultiplier; math.Abs(got-1.12) > 1e-9 {
			t.Errorf("%s: Healing Light 3 multiplies it by %v, want 1.12", c.name, got)
		}
		gotCrit := (c.spell(with).BonusCritRating - c.spell(noSanctified).BonusCritRating) / core.CritRatingPerCritChance
		if math.Abs(gotCrit-c.crit) > 1e-9 {
			t.Errorf("%s: Sanctified Light 3 adds %v%% crit, want %v%%", c.name, gotCrit, c.crit)
		}
	}
}

// Holy Power, aura 71 on the holy school: 1% crit a point on every holy spell, melee class ones like 54158 too,
// and none on a physical one.
func TestHolyPowerCritsEveryHolySpell(t *testing.T) {
	_, with := healSetup{}.build(t)
	_, without := healSetup{talents: holyTalents(talentHolyPower, 0)}.build(t)
	for _, c := range []struct {
		name  string
		spell func(*HolyPaladin) *core.Spell
		crit  float64
	}{
		{"Holy Light", func(h *HolyPaladin) *core.Spell { return h.HolyLight }, 5},
		{"Flash of Light", func(h *HolyPaladin) *core.Spell { return h.FlashOfLight }, 5},
		{"Holy Shock heal", func(h *HolyPaladin) *core.Spell { return h.HolyShockHeal }, 5},
		{"Holy Shock damage", func(h *HolyPaladin) *core.Spell { return h.HolyShockDamage }, 5},
		{"Exorcism", func(h *HolyPaladin) *core.Spell { return h.Exorcism }, 5},
		{"Judgement", func(h *HolyPaladin) *core.Spell { return h.SealJudgement }, 5},
		{"Crusader Strike", func(h *HolyPaladin) *core.Spell { return h.CrusaderStrike }, 0},
	} {
		got := (c.spell(with).BonusCritRating - c.spell(without).BonusCritRating) / core.CritRatingPerCritChance
		if math.Abs(got-c.crit) > 1e-9 {
			t.Errorf("%s: Holy Power 5 adds %v%% crit, want %v%%", c.name, got, c.crit)
		}
	}
}

// Holy Guidance, auras 174 and 175: 4% of intellect a point as spell power.
func TestHolyGuidanceTurnsIntellectIntoSpellPower(t *testing.T) {
	_, with := healSetup{}.build(t)
	_, without := healSetup{talents: holyTalents(talentHolyGuidance, 0)}.build(t)
	got := with.GetStat(stats.SpellPower) - without.GetStat(stats.SpellPower)
	if want := 0.2 * with.GetStat(stats.Intellect); math.Abs(got-want) > 1e-6 || want == 0 {
		t.Errorf("Holy Guidance 5 adds %v spell power, want 20%% of %v intellect", got, with.GetStat(stats.Intellect))
	}
}

// Enlightened Judgements, auras 54 and 55: 2% melee, ranged and spell hit a point.
func TestEnlightenedJudgementsAddsHit(t *testing.T) {
	_, with := healSetup{}.build(t)
	_, without := healSetup{talents: holyTalents(talentEnlightenedJudgements, 0)}.build(t)
	if got := (with.GetStat(stats.MeleeHit) - without.GetStat(stats.MeleeHit)) / core.MeleeHitRatingPerHitChance; math.Abs(got-4) > 1e-9 {
		t.Errorf("Enlightened Judgements 2 adds %v%% melee hit, want 4%%", got)
	}
	if got := (with.GetStat(stats.SpellHit) - without.GetStat(stats.SpellHit)) / core.SpellHitRatingPerHitChance; math.Abs(got-4) > 1e-9 {
		t.Errorf("Enlightened Judgements 2 adds %v%% spell hit, want 4%%", got)
	}
}

// Light's Grace at 3 points: every Holy Light procs 31834, so after the first each one casts 0.5 s faster. The
// Holy Light that used the buff refreshes it, which keeps its charge (Aura::RefreshSpellMods).
func TestLightsGraceSpeedsUpTheNextHolyLight(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	buff := holy.GetAuraByID(core.ActionID{SpellID: 31834})

	if got := hardcast(t, sim, holy, holy.HolyLight, target); got < 2500*time.Millisecond || got > 2600*time.Millisecond {
		t.Errorf("the first Holy Light took %v, want 2.5 s", got)
	}
	for i := range 3 {
		if !buff.IsActive() {
			t.Fatalf("no Light's Grace after Holy Light %d", i+1)
		}
		if got := hardcast(t, sim, holy, holy.HolyLight, target); got < 2000*time.Millisecond || got > 2100*time.Millisecond {
			t.Errorf("Holy Light %d took %v under Light's Grace, want 2 s", i+2, got)
		}
	}
}

// At one point the buff comes off 33% of Holy Lights, and the Holy Light it speeds up takes its charge unless that
// Holy Light procs it again. So a third of Holy Lights cast fast, where a buff kept for its 15 s would speed up
// nearly all of them.
func TestLightsGraceIsUsedUpUnlessItProcsAgain(t *testing.T) {
	sim, holy := healSetup{talents: holyTalents(talentLightsGrace, 1), dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]

	const casts = 200
	fast := 0
	for range casts {
		if hardcast(t, sim, holy, holy.HolyLight, target) < 2250*time.Millisecond {
			fast++
		}
	}
	mean, sd := casts*0.33, math.Sqrt(casts*0.33*0.67)
	if math.Abs(float64(fast)-mean) > 5*sd {
		t.Errorf("%d of %d Holy Lights cast fast, want about %.0f", fast, casts, mean)
	}
}

// spell_pal_infusion_of_light: a Holy Shock crit, heal or damage, puts up 54149 at 2 points. A Holy Shock that
// doesn't crit doesn't.
func TestInfusionOfLightComesFromHolyShockCrits(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	friend := targetDummies(sim)[0]
	enemy := sim.Encounter.TargetUnits[0]
	buff := holy.GetAuraByID(core.ActionID{SpellID: 54149})

	setCritFor(sim, holy, holy.HolyShockHeal, 0)
	cast(sim, holy.HolyShock, friend)
	if buff.IsActive() {
		t.Errorf("a Holy Shock heal that didn't crit gave Infusion of Light")
	}

	setCrit(sim, holy, 200)
	cast(sim, holy.HolyShock, friend)
	if !buff.IsActive() {
		t.Errorf("a Holy Shock heal crit gave no Infusion of Light")
	}

	buff.Deactivate(sim)
	for holy.HolyShockDamage.SpellMetrics[enemy.UnitIndex].Crits == 0 {
		cast(sim, holy.HolyShock, enemy)
	}
	if !buff.IsActive() {
		t.Errorf("a Holy Shock damage crit gave no Infusion of Light")
	}
}

// 54149 gives the next Holy Light 20% crit, and that Holy Light uses it up.
func TestInfusionOfLightCritsTheNextHolyLight(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	buff := holy.GetAuraByID(core.ActionID{SpellID: 54149})
	setCritFor(sim, holy, holy.HolyLight, 0)

	const casts = 300
	for range casts {
		buff.Activate(sim)
		cast(sim, holy.HolyLight, target)
		if buff.IsActive() {
			t.Fatalf("Holy Light left Infusion of Light up")
		}
	}
	got := crits(holy.HolyLight, target)
	mean, sd := casts*0.2, math.Sqrt(casts*0.2*0.8)
	if math.Abs(float64(got)-mean) > 5*sd {
		t.Errorf("Holy Light crit %d times in %d under Infusion of Light at 0%% crit, want about %.0f", got, casts, mean)
	}
	cast(sim, holy.HolyLight, target)
	if got2 := crits(holy.HolyLight, target); got2 != got {
		t.Errorf("Holy Light crit at 0%% crit without Infusion of Light")
	}
}

// 54149 takes 1.5 s off the next Flash of Light, so it's instant, and that Flash of Light uses it up.
func TestInfusionOfLightMakesFlashOfLightInstant(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	buff := holy.GetAuraByID(core.ActionID{SpellID: 54149})

	buff.Activate(sim)
	if got := hardcast(t, sim, holy, holy.FlashOfLight, target); got != 0 {
		t.Errorf("Flash of Light took %v under Infusion of Light, want instant", got)
	}
	if buff.IsActive() {
		t.Errorf("Flash of Light left Infusion of Light up")
	}
	if got := hardcast(t, sim, holy, holy.FlashOfLight, target); got < 1500*time.Millisecond {
		t.Errorf("the next Flash of Light took %v, want 1.5 s", got)
	}
}

// A Flash of Light on a target with Sacred Shield puts 66922 on it: CalculatePct(heal / 12, 100%) a tick over 12
// one-second ticks, each with the paladin's healing done percents. No hot on a target without Sacred Shield.
func TestInfusionOfLightFlashOfLightHot(t *testing.T) {
	sim, holy := healSetup{dummies: 2}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	shielded, bare := dummies[0], dummies[1]
	hot := holy.GetSpell(core.ActionID{SpellID: 66922})
	setSpellPower(sim, holy, 3000)

	cast(sim, holy.SacredShield, shielded)
	heal := castHeal(sim, holy.FlashOfLight, holy.FlashOfLight, shielded)
	castHeal(sim, holy.FlashOfLight, holy.FlashOfLight, bare)
	if !hot.Hot(shielded).IsActive() || hot.Hot(bare).IsActive() {
		t.Fatalf("hot on the shielded target %v, on the bare one %v; want it on the shielded one only",
			hot.Hot(shielded).IsActive(), hot.Hot(bare).IsActive())
	}
	runUntil(sim, 13*time.Second)

	// Divinity's 5% healing done, then the target's healing taken
	tick := math.Trunc(math.Trunc(float64(float32(math.Trunc(heal/12))*1.05)) * shielded.PseudoStats.HealingTakenMultiplier)
	if got := hot.SpellMetrics[shielded.UnitIndex].TotalHealing; got != 12*tick {
		t.Errorf("the hot healed %v off a %v Flash of Light, want 12 × %v", got, heal, tick)
	}
	if hot.Hot(shielded).IsActive() {
		t.Errorf("the hot is still up after 12 s")
	}
}

// spell_pal_illumination at 5 points: every crit of Holy Light, Flash of Light or Holy Shock's heal gives back 30% of
// the heal's base cost, Holy Shock's being 48825's 18%, off 4394 base mana. A heal that doesn't crit gives nothing.
func TestIlluminationGivesBackCritHealsBaseCost(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	spent := holy.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionWait})

	for _, c := range []struct {
		name   string
		spell  *core.Spell
		refund float64
	}{
		// int32(int32(4394 × 29%) × 30%)
		{"Holy Light", holy.HolyLight, 382},
		{"Flash of Light", holy.FlashOfLight, 92},
		{"Holy Shock", holy.HolyShock, 237},
	} {
		for _, crit := range []bool{true, false} {
			setCrit(sim, holy, core.TernaryFloat64(crit, 200, -100))
			holy.SpendMana(sim, holy.CurrentMana()-1000, spent)
			before := holy.CurrentMana()
			cast(sim, c.spell, target)
			want := core.TernaryFloat64(crit, c.refund, 0)
			if got := holy.CurrentMana() - before; got != want {
				t.Errorf("%s, crit %v: got %v mana, want %v", c.name, crit, got, want)
			}
		}
	}
}

// At one point Illumination gives the mana on 20% of crits.
func TestIlluminationChanceTakesItsPoints(t *testing.T) {
	sim, holy := healSetup{talents: holyTalents(talentIllumination, 1), dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	spent := holy.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionWait})
	setCrit(sim, holy, 200)

	const casts = 300
	procs := 0
	for range casts {
		holy.SpendMana(sim, holy.CurrentMana()-1000, spent)
		before := holy.CurrentMana()
		cast(sim, holy.HolyLight, target)
		if holy.CurrentMana() > before {
			procs++
		}
	}
	mean, sd := casts*0.2, math.Sqrt(casts*0.2*0.8)
	if math.Abs(float64(procs)-mean) > 5*sd {
		t.Errorf("%d procs in %d Holy Light crits, want about %.0f", procs, casts, mean)
	}
}

// Judgements of the Pure at 5 points: a landed judgement gives 54153, 15% cast and attack haste for 60 s.
func TestJudgementsOfThePureHaste(t *testing.T) {
	sim, holy := healSetup{}.build(t)
	startFight(sim)
	enemy := sim.Encounter.TargetUnits[0]
	buff := holy.GetAuraByID(core.ActionID{SpellID: 54153})
	cast(sim, holy.SealOfWisdom, &holy.Unit)
	castSpeed, swingSpeed := holy.CastSpeed, holy.PseudoStats.MeleeSpeedMultiplier

	for holy.JudgementOfWisdom.SpellMetrics[enemy.UnitIndex].Hits == 0 {
		cast(sim, holy.JudgementOfWisdom, enemy)
	}
	if !buff.IsActive() {
		t.Fatalf("a landed judgement gave no Judgements of the Pure")
	}
	if got := castSpeed / holy.CastSpeed; math.Abs(got-1.15) > 1e-9 {
		t.Errorf("cast speed went up %vx, want 1.15x", got)
	}
	if got := holy.PseudoStats.MeleeSpeedMultiplier / swingSpeed; math.Abs(got-1.15) > 1e-9 {
		t.Errorf("melee speed went up %vx, want 1.15x", got)
	}

	runUntil(sim, sim.CurrentTime+61*time.Second)
	if buff.IsActive() || math.Abs(holy.CastSpeed-castSpeed) > 1e-12 {
		t.Errorf("Judgements of the Pure still up after 60 s, cast speed %v, want %v", holy.CastSpeed, castSpeed)
	}
}

// Judgements of the Pure's SPELLMOD_DAMAGE and SPELLMOD_DOT, 5% a point, on the seals and their judgements, 54158
// included. Judgement of Command is in through its flags[1] 0x200.
func TestJudgementsOfThePureDamage(t *testing.T) {
	_, with := healSetup{}.build(t)
	_, without := healSetup{talents: holyTalents(talentJudgementsOfThePure, 0)}.build(t)
	for _, c := range []struct {
		id   core.ActionID
		want float64
	}{
		{core.ActionID{SpellID: 54158}, 1.25},
		{core.ActionID{SpellID: 20187}, 1.25},
		{core.ActionID{SpellID: 20154}, 1.25},
		{core.ActionID{SpellID: 20424}, 1.25},
		{core.ActionID{SpellID: 31804}, 1.25},
		{core.ActionID{SpellID: 42463}, 1.25},
		{core.ActionID{SpellID: 31803, Tag: 2}, 1.25},
		{core.ActionID{SpellID: 20467}, 1.25},
	} {
		got := with.GetSpell(c.id).DamageMultiplier / without.GetSpell(c.id).DamageMultiplier
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: Judgements of the Pure 5 multiplies it by %v, want %v", c.id, got, c.want)
		}
	}
}
