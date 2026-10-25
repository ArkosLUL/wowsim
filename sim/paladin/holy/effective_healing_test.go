package holy

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// spell_pal_sacred_shield_dummy: with incoming damage the absorb goes on with the aura, then on the first hit the
// shielded unit takes once its cooldown (7 s with the standard talents) is up. A hit on anyone else adds nothing.
func TestSacredShieldAbsorbsOnHitsTaken(t *testing.T) {
	count := func(holy *HolyPaladin, target *core.Unit) int32 {
		return holy.SacredShieldAbsorb.SpellMetrics[target.UnitIndex].Hits
	}

	sim, holy := healSetup{dummies: 5, damage: 100}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	tank, raid := dummies[0], dummies[1:]
	cast(sim, holy.SacredShield, tank)
	runUntil(sim, 7500*time.Millisecond)
	if got := count(holy, tank); got != 1 {
		t.Errorf("%d absorbs by 7.5 s, want only the one with the aura: the cooldown is up at 7 s but no hit lands before 8 s", got)
	}
	// the hit at 8 s is the first past the cooldown, the next one at 16 s
	runUntil(sim, 8500*time.Millisecond)
	if got := count(holy, tank); got != 2 {
		t.Errorf("%d absorbs by 8.5 s, want 2", got)
	}
	runUntil(sim, 20*time.Second)
	if got := count(holy, tank); got != 3 {
		t.Errorf("%d absorbs by 20 s, want 3", got)
	}
	for _, unit := range raid {
		if got := count(holy, unit); got != 0 {
			t.Errorf("%s took %d absorbs without Sacred Shield", unit.Label, got)
		}
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
