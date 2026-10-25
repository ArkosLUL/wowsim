package holy

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// spell_pal_sacred_shield_dummy: with incoming damage 58597 goes on at a hit the shielded unit takes, the first
// one once its cooldown (7 s with the standard talents) is up. The cooldown lives on the aura: a refresh keeps
// it, a fresh aura has none. A hit on anyone else adds nothing.
func TestSacredShieldAbsorbsOnHitsTaken(t *testing.T) {
	// the tank is hit every 2 s; four raid dummies means all of them every 4 s
	sim, holy := healSetup{dummies: 5, damage: 100}.build(t)
	check := func(at time.Duration, unit *core.Unit, want int32) {
		t.Helper()
		if got := holy.SacredShieldAbsorb.SpellMetrics[unit.UnitIndex].Hits; got != want {
			t.Errorf("%s: %d absorbs by %v, want %d", unit.Label, got, at, want)
		}
	}
	startFight(sim)
	dummies := targetDummies(sim)
	tank, raid := dummies[0], dummies[1:]
	cast(sim, holy.SacredShield, tank)
	for _, step := range []struct {
		at   time.Duration
		want int32
	}{
		// none with the aura, one at the first hit
		{1900 * time.Millisecond, 0},
		{2100 * time.Millisecond, 1},
		// up again at 9 s, the next hit at 10 s
		{9500 * time.Millisecond, 1},
		{10500 * time.Millisecond, 2},
	} {
		runUntil(sim, step.at)
		check(step.at, tank, step.want)
	}

	// a refresh at 11 s keeps the cooldown, up at 17 s
	cast(sim, holy.SacredShield, tank)
	runUntil(sim, 17500*time.Millisecond)
	check(17500*time.Millisecond, tank, 2)
	runUntil(sim, 18500*time.Millisecond)
	check(18500*time.Millisecond, tank, 3)

	// moved at 19 s with the tank's cooldown running to 25 s, the new aura still takes the raid's hit at 20 s
	cast(sim, holy.SacredShield, raid[0])
	runUntil(sim, 19500*time.Millisecond)
	check(19500*time.Millisecond, raid[0], 0)
	runUntil(sim, 20500*time.Millisecond)
	check(20500*time.Millisecond, raid[0], 1)
	check(20500*time.Millisecond, tank, 3)
	for _, unit := range raid[1:] {
		check(20500*time.Millisecond, unit, 0)
	}
}

// spell_pal_light_s_beacon copies the heal before taken terms, not what landed: a heal on a full unit still puts
// its whole amount on a hurt beacon target.
func TestBeaconCopiesAHealThatOverhealed(t *testing.T) {
	sim, holy := healSetup{dummies: 2, damage: 1}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	full, beaconTarget := dummies[1], dummies[0]
	copySpell := holy.GetSpell(core.ActionID{SpellID: 53652})
	setSpellPower(sim, holy, 3000)
	setCrit(sim, holy, -100)
	fixRolls(holy.HolyLight)

	cast(sim, holy.BeaconOfLight, beaconTarget)
	beaconTarget.RemoveHealth(sim, 15000)
	heal := castHeal(sim, holy.HolyLight, holy.HolyLight, full)
	if got := holy.HolyLight.SpellMetrics[full.UnitIndex].TotalOverheal; heal < 4000 || got != heal {
		t.Fatalf("Holy Light of %v on a full unit overhealed %v, want all of it", heal, got)
	}
	if got := healed(copySpell, beaconTarget); got != math.Trunc(heal) {
		t.Errorf("the beacon copy healed %v, want the Holy Light's %v", got, heal)
	}
	if got := copySpell.SpellMetrics[beaconTarget.UnitIndex].TotalOverheal; got != 0 {
		t.Errorf("the copy overhealed %v on a unit missing 15000", got)
	}
}

func defaultAPL(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../../ui/holy_paladin/apls/default.apl.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// castsOn is how many times the spells were cast on the unit.
func castsOn(unit *core.Unit, spells ...*core.Spell) (total int32) {
	for _, spell := range spells {
		total += spell.SpellMetrics[unit.UnitIndex].Casts
	}
	return
}

// The default rotation puts Beacon of Light and Sacred Shield on the tank and heals nobody while nobody is hurt.
func TestDefaultAPLWaitsWhenNobodyIsHurt(t *testing.T) {
	sim, holy := healSetup{dummies: 3, damage: 1, apl: defaultAPL(t)}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	runUntil(sim, 20*time.Second)

	for i, unit := range dummies {
		if got := castsOn(unit, holy.HolyLight, holy.FlashOfLight, holy.HolyShock); got != 0 {
			t.Errorf("dummy %d was healed %d times with nobody hurt", i+1, got)
		}
	}
	if !holy.BeaconOfLightAuras.Get(dummies[0]).IsActive() {
		t.Error("Beacon of Light isn't on the tank")
	}
	if !holy.SacredShieldAuras.Get(dummies[0]).IsActive() {
		t.Error("Sacred Shield isn't on the tank")
	}
}

// The default rotation answers the most hurt unit, with the heal that fits what it
// is missing: Flash of Light from 2500, Holy Shock from 3500 and Holy Light from 7000 when Holy Shock is down.
func TestDefaultAPLSizesTheHealToTheMissingHealth(t *testing.T) {
	for _, c := range []struct {
		name       string
		missing    float64
		shockReady bool
		want       func(*HolyPaladin) *core.Spell
	}{
		{"too little", 2000, true, nil},
		{"small", 3000, true, func(holy *HolyPaladin) *core.Spell { return holy.FlashOfLight }},
		{"medium", 5000, true, func(holy *HolyPaladin) *core.Spell { return holy.HolyShock }},
		{"large with Holy Shock down", 8000, false, func(holy *HolyPaladin) *core.Spell { return holy.HolyLight }},
	} {
		sim, holy := healSetup{dummies: 3, damage: 1, apl: defaultAPL(t)}.build(t)
		startFight(sim)
		runUntil(sim, 5*time.Second)
		setSpellPower(sim, holy, 3000)
		hurt := targetDummies(sim)[2]
		if !c.shockReady {
			holy.HolyShock.CD.Set(sim.CurrentTime + time.Minute)
		}
		hurt.RemoveHealth(sim, c.missing)
		runUntil(sim, sim.CurrentTime+4*time.Second)

		all := []*core.Spell{holy.HolyLight, holy.FlashOfLight, holy.HolyShock}
		for _, spell := range all {
			want := c.want != nil && c.want(holy) == spell
			if got := castsOn(hurt, spell) > 0; got != want {
				t.Errorf("%s (missing %v): %s cast %v, want %v", c.name, c.missing, spell.ActionID, got, want)
			}
		}
	}
}

// With two units hurt the one with the lower health percent comes first, the raid dummies having 20000 health.
func TestDefaultAPLHealsTheLowerPercentFirst(t *testing.T) {
	sim, holy := healSetup{dummies: 3, damage: 1, apl: defaultAPL(t)}.build(t)
	startFight(sim)
	runUntil(sim, 5*time.Second)
	dummies := targetDummies(sim)
	setSpellPower(sim, holy, 3000)
	dummies[1].RemoveHealth(sim, 6000)
	dummies[2].RemoveHealth(sim, 9000)
	runUntil(sim, sim.CurrentTime+200*time.Millisecond)

	all := []*core.Spell{holy.HolyLight, holy.FlashOfLight, holy.HolyShock}
	if castsOn(dummies[2], all...) == 0 || castsOn(dummies[1], all...) != 0 {
		t.Errorf("heals on the 70%% unit %d, on the 55%% unit %d, want the lower one first",
			castsOn(dummies[1], all...), castsOn(dummies[2], all...))
	}
}
