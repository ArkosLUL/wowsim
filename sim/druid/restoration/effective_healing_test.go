package restoration

import (
	"os"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func hurtDummies(sim *core.Simulation, missing ...float64) []*core.Unit {
	dummies := targetDummies(sim)
	for i, lost := range missing {
		if lost > 0 {
			dummies[i].RemoveHealth(sim, lost)
		}
	}
	return dummies
}

func defaultAPL(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../../ui/restoration_druid/apls/default.apl.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func totalCasts(spell *core.Spell) int32 {
	var casts int32
	for _, metrics := range spell.SpellMetrics {
		casts += metrics.Casts
	}
	return casts
}

// With incoming damage a seed is spent on the seeded dummy's next hit, not a stand-in's: the tank is hit at 2 s
// and a stand-in started at 1 s would fire at 3 s.
func TestLivingSeedBloomsOnTheDummiesHit(t *testing.T) {
	sim, druid := healSetup{talents: LivingSeedTalents, dummies: 1, damage: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	druid.HealingTouch.Spell.Direct.Max = druid.HealingTouch.Spell.Direct.Min
	druid.AddStatDynamic(sim, stats.SpellCrit, 200*core.CritRatingPerCritChance)

	runUntil(sim, time.Second)
	castHeal(sim, druid.HealingTouch.Spell, target)
	runUntil(sim, 1900*time.Millisecond)
	if got := healed(druid.LivingSeedHeal.Spell, target); got != 0 {
		t.Fatalf("the seed bloomed for %v before the hit", got)
	}
	runUntil(sim, 2100*time.Millisecond)
	if healed(druid.LivingSeedHeal.Spell, target) == 0 {
		t.Error("the seed didn't bloom on the 2 s hit")
	}
	bloomed := druid.LivingSeedHeal.SpellMetrics[target.UnitIndex].Casts
	runUntil(sim, 10*time.Second)
	if got := druid.LivingSeedHeal.SpellMetrics[target.UnitIndex].Casts; got != bloomed {
		t.Errorf("%d blooms from one seed", got)
	}
}

// Wild Growth goes to the lowest health percents among the dummies, not the cast's target.
func TestWildGrowthPicksTheHurtDummies(t *testing.T) {
	sim, druid := healSetup{dummies: 8, damage: 1}.build(t)
	startFight(sim)
	dummies := hurtDummies(sim, 0, 1000, 900, 800, 700, 600, 500, 0)
	druid.WildGrowth.SkipCastAndApplyEffects(sim, dummies[0])
	for i, dummy := range dummies {
		want := i >= 1 && i <= 5
		if got := druid.WildGrowth.Hot(dummy).IsActive(); got != want {
			t.Errorf("Wild Growth on dummy %d: %v, want %v", i+1, got, want)
		}
	}
}

// The default APL keeps Lifebloom on the tank, not on the current target.
func TestDefaultAPLLifebloomsTheTank(t *testing.T) {
	sim, druid := healSetup{dummies: 3, damage: 1, apl: defaultAPL(t)}.build(t)
	dummies := targetDummies(sim)
	druid.CurrentTarget = dummies[2]
	startFight(sim)
	runUntil(sim, 5*time.Second)
	if !druid.Lifebloom.Hot(dummies[0]).IsActive() {
		t.Error("Lifebloom isn't on the tank")
	}
	if druid.Lifebloom.Hot(dummies[2]).IsActive() {
		t.Error("Lifebloom is on the current target")
	}
}

// The default APL heals nothing but the tank's Lifebloom while nobody is missing much, then sizes the heal to what
// the most hurt dummy is missing: Rejuvenation from 1500, Nourish from 2500, Wild Growth from 3500, Swiftmend from
// 4500 and Regrowth from 6000, with Healing Touch ahead of it from 5000 on a Clearcasting. Lifebloom is at full
// stacks and the dummy has a Rejuvenation unless noted.
func TestDefaultAPLSizesTheHealToTheMissingHealth(t *testing.T) {
	for _, c := range []struct {
		name       string
		missing    float64
		noRejuv    bool
		clearcast  bool
		onCooldown []string
		want       string
	}{
		{"nobody hurt", 1000, true, false, nil, ""},
		{"a little hurt", 2000, true, false, nil, "Rejuvenation"},
		{"covered and a little hurt", 2000, false, false, nil, ""},
		{"medium", 3000, false, false, nil, "Nourish"},
		{"hurt", 4000, false, false, nil, "Wild Growth"},
		{"hurt, no Wild Growth", 5000, false, false, []string{"Wild Growth"}, "Swiftmend"},
		{"badly hurt", 7000, false, false, []string{"Wild Growth", "Swiftmend"}, "Regrowth"},
		{"Clearcasting", 5500, false, true, []string{"Wild Growth", "Swiftmend"}, "Healing Touch"},
	} {
		sim, druid := healSetup{dummies: 3, damage: 1, apl: defaultAPL(t)}.build(t)
		startFight(sim)
		dummies := hurtDummies(sim, 0, c.missing)
		if c.clearcast {
			druid.ClearcastingAura.Activate(sim)
		}
		for range 3 {
			druid.Lifebloom.SkipCastAndApplyEffects(sim, dummies[0])
		}
		if !c.noRejuv {
			druid.Rejuvenation.Hot(dummies[1]).Apply(sim)
		}
		for _, name := range c.onCooldown {
			spell := map[string]*core.Spell{"Wild Growth": druid.WildGrowth.Spell, "Swiftmend": druid.Swiftmend.Spell}[name]
			spell.CD.Set(sim.CurrentTime + time.Hour)
		}
		spells := map[string]*core.Spell{
			"Rejuvenation": druid.Rejuvenation.Spell, "Nourish": druid.Nourish.Spell, "Wild Growth": druid.WildGrowth.Spell,
			"Swiftmend": druid.Swiftmend.Spell, "Regrowth": druid.Regrowth.Spell,
			"Healing Touch": druid.HealingTouch.Spell,
		}
		before := map[string]int32{}
		for name, spell := range spells {
			before[name] = totalCasts(spell)
		}
		// the first cast has started, a hard cast still in progress
		runUntil(sim, 100*time.Millisecond)
		got := ""
		for name, spell := range spells {
			if totalCasts(spell) > before[name] || (sim.CurrentTime < druid.Hardcast.Expires && druid.Hardcast.ActionID == spell.ActionID) {
				got = name
			}
		}
		if got != c.want {
			t.Errorf("%s: cast %q first, want %q", c.name, got, c.want)
		}
	}
}
