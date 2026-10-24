package restoration

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

const autocast = `{"type": "TypeAPL", "priorityList": [{"action":{"autocastOtherCooldowns":{}}}]}`

// spell_dru_innervate's int32 per tick drops the 0.6 of 786.6
const innervateMana = 10 * 786

// innervateGained is the mana the unit has gained from Innervate so far, not the cost of casting it on itself.
func innervateGained(unit *core.Unit) float64 {
	var total float64
	for _, r := range unit.Metrics.ToProto().Resources {
		if r.Id.GetSpellId() == 29166 && r.Gain > 0 {
			total += r.Gain
		}
	}
	return total
}

// The mana cooldown casts Innervate once the target is low on mana, on the druid or on another player, and gives
// its mana once; the 3 minute cooldown holds off the next one.
func TestInnervateCastsOncePerCooldown(t *testing.T) {
	for _, c := range []struct {
		name   string
		target *proto.UnitReference
		other  bool
	}{
		{"self", &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0}, false},
		{"another player", &proto.UnitReference{Type: proto.UnitReference_Player, Index: 5}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			sim, druid := healSetup{innervate: c.target, other: c.other, dummies: 1, apl: autocast}.build(t)
			startFight(sim)
			target := druid.GetUnit(c.target)
			spell := druid.GetSpell(core.ActionID{SpellID: 29166, Tag: druid.Index})
			if spell == nil {
				t.Fatal("Innervate isn't registered")
			}
			if cost := spell.StaticCost(); cost != 0 {
				t.Errorf("Innervate costs %v mana, the server's spell costs none", cost)
			}
			target.SpendMana(sim, target.CurrentMana()-100, target.NewManaMetrics(core.ActionID{OtherID: 1}))

			runUntil(sim, 20*time.Second)
			if got := spell.SpellMetrics[druid.CurrentTarget.UnitIndex].Casts; got != 1 {
				t.Errorf("%d casts in the first 20 s, want 1", got)
			}
			if got := innervateGained(target); math.Abs(got-innervateMana) > 0.01 {
				t.Errorf("gave %v mana, want %v", got, innervateMana)
			}

			// still low on mana, the cooldown is the only thing in the way
			target.SpendMana(sim, target.CurrentMana()-100, target.NewManaMetrics(core.ActionID{OtherID: 1}))
			runUntil(sim, 170*time.Second)
			if got := spell.SpellMetrics[druid.CurrentTarget.UnitIndex].Casts; got != 1 {
				t.Errorf("%d casts at 170 s, want 1", got)
			}
			target.SpendMana(sim, target.CurrentMana()-100, target.NewManaMetrics(core.ActionID{OtherID: 1}))
			runUntil(sim, 200*time.Second)
			if got := spell.SpellMetrics[druid.CurrentTarget.UnitIndex].Casts; got != 2 {
				t.Errorf("%d casts at 200 s, want 2", got)
			}
			if got := innervateGained(target); math.Abs(got-2*innervateMana) > 0.01 {
				t.Errorf("gave %v mana in all, want %v", got, 2*innervateMana)
			}
		})
	}
}
