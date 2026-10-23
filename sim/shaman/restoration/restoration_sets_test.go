package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func setShaman(t *testing.T, items []int32) (*core.Simulation, *RestorationShaman) {
	t.Helper()
	sim, shaman := healSetup{items: items, dummies: 1}.build(t)
	sim.Reset()
	return sim, shaman
}

// T7: 5% more Healing Wave and Chain Heal (aura 108 on their class mask), at 4 pieces.
func TestT7FourPieceRaisesHealingWaveAndChainHeal(t *testing.T) {
	_, three := setShaman(t, t7Regalia[:3])
	_, four := setShaman(t, t7Regalia[:4])
	for _, c := range []struct {
		name  string
		spell func(*RestorationShaman) *core.Spell
		want  float64
	}{
		{"Healing Wave", func(s *RestorationShaman) *core.Spell { return s.HealingWave }, 1.05},
		{"Chain Heal", func(s *RestorationShaman) *core.Spell { return s.ChainHeal }, 1.05},
		{"Lesser Healing Wave", func(s *RestorationShaman) *core.Spell { return s.LesserHealingWave }, 1},
		{"Riptide", func(s *RestorationShaman) *core.Spell { return s.Riptide }, 1},
	} {
		if got := c.spell(four).DamageMultiplier / c.spell(three).DamageMultiplier; math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: 4 pieces multiply it by %v over 3, want %v", c.name, got, c.want)
		}
	}
}

// T8: Riptide's cooldown 1 s shorter at 2 pieces, Chain Heal's cast 0.2 s shorter at 4.
func TestT8RestorationBonuses(t *testing.T) {
	_, one := setShaman(t, t8Regalia[:1])
	_, two := setShaman(t, t8Regalia[:2])
	_, three := setShaman(t, t8Regalia[:3])
	_, four := setShaman(t, t8Regalia[:4])

	if one.Riptide.CD.Duration != 6*time.Second || two.Riptide.CD.Duration != 5*time.Second {
		t.Errorf("Riptide's cooldown is %s with 1 piece and %s with 2, want 6 s and 5 s", one.Riptide.CD.Duration, two.Riptide.CD.Duration)
	}
	if three.ChainHeal.DefaultCast.CastTime != 2500*time.Millisecond || four.ChainHeal.DefaultCast.CastTime != 2300*time.Millisecond {
		t.Errorf("Chain Heal casts for %s with 3 pieces and %s with 4, want 2.5 s and 2.3 s", three.ChainHeal.DefaultCast.CastTime, four.ChainHeal.DefaultCast.CastTime)
	}
}

// T9: Riptide heals 20% more, direct and periodic (aura 108 for damage and for dot), and Chain Heal crits 5% more.
func TestT9RestorationBonuses(t *testing.T) {
	sim, one := setShaman(t, t9Garb[:1])
	_, two := setShaman(t, t9Garb[:2])
	_, three := setShaman(t, t9Garb[:3])
	_, four := setShaman(t, t9Garb[:4])

	if got := two.Riptide.DamageMultiplier / one.Riptide.DamageMultiplier; math.Abs(got-1.2) > 1e-9 {
		t.Errorf("2 pieces multiply Riptide by %v, want 1.2", got)
	}
	if got := (four.ChainHeal.BonusCritRating - three.ChainHeal.BonusCritRating) / core.CritRatingPerCritChance; math.Abs(got-5) > 1e-9 {
		t.Errorf("4 pieces add %v%% crit to Chain Heal, want 5", got)
	}

	// each tick is the hot's amount times 1.2, whole, then times Purification's 1.1, whole
	sim, two = setShaman(t, t9Garb[:2])
	target := targetDummies(sim)[0]
	hot := two.Riptide.Hot(target)
	metrics := &two.Riptide.SpellMetrics[target.UnitIndex]
	hot.Apply(sim)
	before := metrics.TotalHealing
	hot.TickOnce(sim)
	power := two.Riptide.HealingPower(target) + 0.05*float64(two.Talents.NaturesBlessing)*two.GetStat(stats.Intellect)
	amount := math.Trunc((334 + core.PowerBonus(0.188, power)) * 1.2)
	want := math.Trunc(float64(float32(amount) * (float32(1) + float32(10)/100)))
	if got := metrics.TotalHealing - before; math.Abs(got-want) > 1 {
		t.Errorf("a tick healed %v with T9's 2 piece, want %v", got, want)
	}
}

// T10's 2 piece: a Riptide gives the next cast 20% haste (70806) for 10 s, spent by the next cast and not by the
// Riptide itself, which completes after its own heal.
func TestT10TwoPieceGivesTheNextCastHaste(t *testing.T) {
	sim, shaman := healSetup{items: t10Garb[:2], dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	baseSpeed := shaman.CastSpeed
	rapid := shaman.GetAura("Rapid Currents")

	shaman.Riptide.SkipCastAndApplyEffects(sim, target)
	if !rapid.IsActive() || math.Abs(baseSpeed/shaman.CastSpeed-1.2) > 1e-9 {
		t.Fatalf("Rapid Currents active %v with cast speed %v over %v, want 20%% haste", rapid.IsActive(), baseSpeed, shaman.CastSpeed)
	}
	shaman.OnCastComplete(sim, shaman.Riptide)
	if !rapid.IsActive() {
		t.Errorf("the Riptide that gave Rapid Currents used it up")
	}
	shaman.OnCastComplete(sim, shaman.LesserHealingWave)
	if rapid.IsActive() || shaman.CastSpeed != baseSpeed {
		t.Errorf("Rapid Currents is still up after the next cast: active %v, cast speed %v over %v", rapid.IsActive(), baseSpeed, shaman.CastSpeed)
	}

	// unused, it ends after 10 s
	shaman.Riptide.SkipCastAndApplyEffects(sim, target)
	shaman.OnCastComplete(sim, shaman.Riptide)
	runUntil(sim, 9500*time.Millisecond)
	if !rapid.IsActive() {
		t.Fatal("Rapid Currents ended before 10 s")
	}
	runUntil(sim, 10500*time.Millisecond)
	if rapid.IsActive() || shaman.CastSpeed != baseSpeed {
		t.Errorf("Rapid Currents lasted past 10 s: active %v, cast speed %v over %v", rapid.IsActive(), baseSpeed, shaman.CastSpeed)
	}
}

// In a fight, a cast completes with Rapid Currents up when it is a Riptide, which just gave it, or the cast right
// after one, which uses it up: no cast after that finds it still there.
func TestT10TwoPieceInARotation(t *testing.T) {
	sim, shaman := healSetup{items: t10Garb[:2], dummies: 1, apl: `{"type": "TypeAPL", "priorityList": [
		{"action": {"castSpell": {"spellId": {"spellId": 61301}}}},
		{"action": {"castSpell": {"spellId": {"spellId": 49276}}}}
	]}`}.build(t)
	type completed struct {
		riptide bool
		rapid   bool
	}
	var casts []completed
	shaman.RegisterAura(core.Aura{
		Label:    "cast order",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		// ahead of Rapid Currents' own callback, which is activated later
		OnCastComplete: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell) {
			casts = append(casts, completed{spell == shaman.Riptide, shaman.GetAura("Rapid Currents").IsActive()})
		},
	})
	startFight(sim)
	runUntil(sim, 20*time.Second)

	riptides := 0
	for i, cast := range casts {
		if cast.riptide {
			riptides++
		}
		if want := cast.riptide || (i > 0 && casts[i-1].riptide); cast.rapid != want {
			t.Errorf("cast %d (Riptide %v) completed with Rapid Currents %v, want %v", i, cast.riptide, cast.rapid, want)
		}
	}
	if riptides < 3 {
		t.Errorf("%d Riptides in 20 s of %d casts, want at least 3", riptides, len(casts))
	}
}

// T10's 4 piece (spell_sha_t10_restoration_4p_bonus): a Chain Heal crit heals its target over 3 ticks of
// int32(int32(heal × 25%) / 3) each, with the healing done percents again, and a plain heal none.
func TestT10FourPieceChainsAHotOffACrit(t *testing.T) {
	sim, shaman := healSetup{items: t10Garb[:4], dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	chained := shaman.GetSpell(core.ActionID{SpellID: 70809})

	neverCrit(sim, shaman)
	shaman.ChainHeal.SkipCastAndApplyEffects(sim, target)
	if healed(chained, target) != 0 || chained.Hot(target).IsActive() {
		t.Fatalf("a Chain Heal that didn't crit chained %v", healed(chained, target))
	}

	shaman.AddStatDynamic(sim, stats.SpellCrit, 1e6+200*core.CritRatingPerCritChance)
	crit := castHeal(sim, shaman.ChainHeal, target)
	runUntil(sim, 10*time.Second)
	tick := math.Trunc(float64(float32(int32(float32(crit)*25/100)/3) * (float32(1) + float32(10)/100)))
	if got := healed(chained, target); got != 3*tick {
		t.Errorf("a %v crit chained %v over the hot, want 3 ticks of %v", crit, got, tick)
	}
}

// Unit::CastDelayedSpellWithPeriodicAmount: a crit on a target that has the hot adds int32(old × ticks left / 3)
// to the new hot's ticks, read at the crit, and the new hot lands on the shaman's next 400 ms step. On the shaman
// it lands at once.
func TestT10FourPieceRollsTheHotOver(t *testing.T) {
	sim, shaman := healSetup{items: t10Garb[:4], dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	chained := shaman.GetSpell(core.ActionID{SpellID: 70809})
	hot := chained.Hot(target)
	shaman.AddStatDynamic(sim, stats.SpellCrit, 200*core.CritRatingPerCritChance)

	first := castHeal(sim, shaman.ChainHeal, target)
	if hot.IsActive() {
		t.Fatal("the hot landed with the crit, want it on the next 400 ms step")
	}
	runUntil(sim, 400*time.Millisecond)
	old := int32(float32(first)*25/100) / 3
	if !hot.IsActive() || int32(hot.SnapshotBaseDamage) != old {
		t.Fatalf("after 400 ms the hot is up %v at %v a tick, want %v", hot.IsActive(), hot.SnapshotBaseDamage, old)
	}

	runUntil(sim, 4*time.Second)
	if hot.TickCount != 1 {
		t.Fatalf("%d ticks by 4 s, want 1", hot.TickCount)
	}
	second := castHeal(sim, shaman.ChainHeal, target)
	runUntil(sim, 4400*time.Millisecond)
	want := int32(float32(second)*25/100)/3 + old*2/3
	if got := int32(hot.SnapshotBaseDamage); got != want || hot.TickCount != 0 {
		t.Errorf("the second hot ticks %v with %d ticks done, want %v and a fresh hot", got, hot.TickCount, want)
	}

	self := chained.Hot(&shaman.Unit)
	castHeal(sim, shaman.ChainHeal, &shaman.Unit)
	if !self.IsActive() {
		t.Errorf("a crit on the shaman left the hot for later")
	}
}

// Totem of Misery takes 79 off Healing Wave's base mana cost, before Tidal Focus's percent.
func TestTotemOfMiseryDiscountsHealingWave(t *testing.T) {
	_, plain := setShaman(t, nil)
	_, totem := setShaman(t, []int32{39728})
	got := plain.HealingWave.DefaultCast.Cost - totem.HealingWave.DefaultCast.Cost
	if want := 79.0; math.Abs(got-want) > 1 {
		t.Errorf("the totem takes %v off the base cost, want %v", got, want)
	}
	if plain.ChainHeal.DefaultCast.Cost != totem.ChainHeal.DefaultCast.Cost {
		t.Errorf("the totem changed Chain Heal's cost")
	}
}

// The Relentless and Wrathful Totems of the Third Wind add 404 and 459 healing power to Lesser Healing Wave
// (class script 3736), through its coefficient.
func TestThirdWindTotemsAddHealingPower(t *testing.T) {
	lesser := func(totem ...int32) float64 {
		sim, shaman := setShaman(t, totem)
		neverCrit(sim, shaman, shaman.LesserHealingWave)
		return castHeal(sim, shaman.LesserHealingWave, targetDummies(sim)[0])
	}
	plain := lesser()
	_, shaman := setShaman(t, nil)
	for _, c := range []struct {
		id    int32
		power float64
	}{{42599, 404}, {51501, 459}, {42598, 320}} {
		want := core.PowerBonus(shaman.LesserHealingWave.Direct.SP, c.power) * shaman.LesserHealingWave.DamageMultiplier
		if got := lesser(c.id) - plain; math.Abs(got-want) > 2 {
			t.Errorf("totem %d adds %v to a heal, want %v", c.id, got, want)
		}
	}
}

// Totem of Calming Tides: a Chain Heal cast has a 70% chance, 8 s apart, to give 234 spell power for 15 s
// (67388), before the cast's heals, so the cast that rolls it already heals with it.
func TestTotemOfCalmingTidesProcsOnTheCast(t *testing.T) {
	sim, shaman := healSetup{items: []int32{47665}, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	neverCrit(sim, shaman, shaman.ChainHeal)
	trance := shaman.GetAura("Spiritual Trance")

	var plainHeal, procHeal float64
	procs, casts := 0, 300
	for range casts {
		// 16 s apart, past the proc's 15 s
		runUntil(sim, sim.CurrentTime+16*time.Second)
		heal := castHeal(sim, shaman.ChainHeal, target)
		if !trance.IsActive() {
			plainHeal = heal
			continue
		}
		procs++
		procHeal = heal
		if trance.Icd.IsReady(sim) {
			t.Fatalf("the proc left no cooldown")
		}
		runUntil(sim, sim.CurrentTime+7*time.Second)
		if trance.Icd.IsReady(sim) {
			t.Fatalf("the proc's cooldown is up 7 s after it")
		}
	}
	if got := float64(procs) / float64(casts); math.Abs(got-0.7) > 0.07 {
		t.Errorf("%d of %d casts proc'd, want about 70%%", procs, casts)
	}
	want := plainHeal + core.PowerBonus(shaman.ChainHeal.Direct.SP, 234)*shaman.ChainHeal.DamageMultiplier
	if math.Abs(procHeal-want) > 2 {
		t.Errorf("the cast that proc'd healed %v, want %v: %v plain and 234 spell power more", procHeal, want, plainHeal)
	}
}

// Totem of the Surging Sea: each Riptide gives 85 spell power for 15 s, stacking 3 times.
func TestTotemOfTheSurgingSeaStacksSpellPower(t *testing.T) {
	sim, shaman := healSetup{items: []int32{50464}, dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	base := shaman.GetStat(stats.SpellPower)

	for i, want := range []float64{85, 170, 255, 255} {
		shaman.Riptide.SkipCastAndApplyEffects(sim, target)
		if got := shaman.GetStat(stats.SpellPower) - base; got != want {
			t.Errorf("after Riptide %d: %v spell power, want %v", i+1, got, want)
		}
		runUntil(sim, time.Duration(i+1)*time.Second)
	}
	runUntil(sim, 3*time.Second+15*time.Second-time.Millisecond)
	if got := shaman.GetStat(stats.SpellPower) - base; got != 255 {
		t.Errorf("%v spell power just before 15 s after the last Riptide, want 255", got)
	}
	runUntil(sim, 3*time.Second+15*time.Second+time.Second)
	if got := shaman.GetStat(stats.SpellPower) - base; got != 0 {
		t.Errorf("%v spell power after the stacks ended, want 0", got)
	}
}
