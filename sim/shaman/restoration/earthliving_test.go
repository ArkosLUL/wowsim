package restoration

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// spell_sha_earthliving_weapon: 20% a heal, and Blessing of the Eternals' 80% more on a target under 35% health
// before the heal. Glyph of Earthliving Weapon adds nothing to that roll.
func TestEarthlivingProcChance(t *testing.T) {
	const torchOfHolyFire = 40395
	items := make([]int32, proto.ItemSlot_ItemSlotMainHand+1)
	items[proto.ItemSlot_ItemSlotMainHand] = torchOfHolyFire
	sim, shaman := healSetup{dummies: 2, damage: 1, items: items, glyphs: StandardGlyphs}.build(t)
	startFight(sim)
	neverCrit(sim, shaman, shaman.LesserHealingWave)
	earthliving := shaman.GetSpell(core.ActionID{SpellID: 51994})
	if earthliving == nil {
		t.Fatal("no Earthliving")
	}
	procs := func(target *core.Unit) int32 { return earthliving.SpellMetrics[target.UnitIndex].Casts }
	dummies := targetDummies(sim)
	tank, raid := dummies[0], dummies[1]

	// 30% before each heal and over 35% after it
	const low = 50
	for range low {
		raid.RemoveHealth(sim, raid.CurrentHealth()-0.3*raid.MaxHealth())
		castHeal(sim, shaman.LesserHealingWave, raid)
		if raid.CurrentHealth() < raid.CountPctFromMaxHealth(35) {
			t.Fatalf("the heal left the dummy at %v of %v, want it over 35%%", raid.CurrentHealth(), raid.MaxHealth())
		}
	}
	if got := procs(raid); got != low {
		t.Errorf("%d procs from %d heals on a dummy under 35%%, want all", got, low)
	}

	const full = 2000
	for range full {
		castHeal(sim, shaman.LesserHealingWave, tank)
	}
	if got, se := float64(procs(tank)), math.Sqrt(full*0.2*0.8); math.Abs(got-0.2*full) > 5*se {
		t.Errorf("%v procs from %d heals on a full dummy, want about %v", got, full, 0.2*full)
	}
}
