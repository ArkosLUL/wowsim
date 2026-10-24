package restoration

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// The page's current target is a friendly dummy. The server refuses a damaging cast at a friend, so the shocks,
// Lightning Bolt and a damaging on-use item neither cast nor deal anything there, and a heal still does.
func TestNoDamageCastsAtAFriend(t *testing.T) {
	const gnomishLightningGenerator = 41121
	items := make([]int32, proto.ItemSlot_ItemSlotTrinket1+1)
	items[proto.ItemSlot_ItemSlotTrinket1] = gnomishLightningGenerator
	sim, shaman := healSetup{dummies: 3, items: items}.build(t)
	startFight(sim)
	dummy := targetDummies(sim)[0]
	generator := shaman.GetSpell(core.ActionID{SpellID: 55039})
	if generator == nil {
		t.Fatal("no Gnomish Lightning Generator spell")
	}

	for name, spell := range map[string]*core.Spell{
		"Earth Shock":                 shaman.EarthShock,
		"Flame Shock":                 shaman.FlameShock,
		"Lightning Bolt":              shaman.LightningBolt,
		"Chain Lightning":             shaman.ChainLightning,
		"Gnomish Lightning Generator": generator,
	} {
		if spell == nil {
			t.Errorf("%s isn't registered", name)
			continue
		}
		if spell.CanCast(sim, dummy) {
			t.Errorf("%s can be cast at a friendly dummy", name)
		}
		if spell.Cast(sim, dummy) {
			t.Errorf("%s cast at a friendly dummy", name)
		}
		if got := spell.SpellMetrics[dummy.UnitIndex].Casts; got != 0 {
			t.Errorf("%s cast %d times at a friendly dummy", name, got)
		}
	}
	if !shaman.HealingWave.CanCast(sim, dummy) {
		t.Error("Healing Wave can't be cast at a friendly dummy")
	}
	if !shaman.EarthShock.CanCast(sim, sim.Encounter.TargetUnits[0]) {
		t.Error("Earth Shock can't be cast at the boss")
	}
}
