package tank

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Unit::AttackerStateUpdate runs the queued strike through CheckCast at the swing and finishes it on a failure.
// Out of runic power the white swing still goes; without 56817, which CheckCast reads first, the swing is lost.
func TestRuneStrikeQueueAtTheSwing(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	start()
	target := dk.CurrentTarget
	white := dk.AutoAttacks.MHAuto()
	rpMetrics := dk.NewRunicPowerMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionRefund})

	swing := func(window bool) *core.Spell {
		t.Helper()
		dk.RuneStrikeAura.Activate(sim)
		if !dk.RuneStrikeQueue.Cast(sim, target) || !dk.RuneStrikeQueued {
			t.Fatal("Rune Strike didn't queue with its window up")
		}
		if !window {
			dk.RuneStrikeAura.Deactivate(sim)
		}
		got := dk.AutoAttacks.MaybeReplaceMHSwing(sim, white)
		if dk.RuneStrikeQueued {
			t.Errorf("window %v, %v runic power: still queued after the swing", window, dk.CurrentRunicPower())
		}
		return got
	}

	if dk.CurrentRunicPower() >= 20 {
		t.Fatalf("starts with %v runic power, want under 20", dk.CurrentRunicPower())
	}
	if got := swing(true); got != white {
		t.Errorf("short of runic power: swing is %v, want the white swing", got.ActionID)
	}
	if got := swing(false); got == white || got == dk.RuneStrike {
		t.Errorf("short of runic power without the window: swing is %v, want it lost", got.ActionID)
	}

	dk.AddRunicPower(sim, 20, rpMetrics)
	if got := swing(true); got != dk.RuneStrike {
		t.Errorf("window up with 20 runic power: swing is %v, want Rune Strike", got.ActionID)
	}

	lost := swing(false)
	if lost == white || lost == dk.RuneStrike {
		t.Fatalf("window gone with 20 runic power: swing is %v, want it lost", lost.ActionID)
	}
	whiteDamage := white.SpellMetrics[target.UnitIndex].TotalDamage
	strikeDamage := dk.RuneStrike.SpellMetrics[target.UnitIndex].TotalDamage
	lost.Cast(sim, target)
	if white.SpellMetrics[target.UnitIndex].TotalDamage != whiteDamage ||
		dk.RuneStrike.SpellMetrics[target.UnitIndex].TotalDamage != strikeDamage || dk.CurrentRunicPower() != 20 {
		t.Errorf("the lost swing dealt damage or spent runic power (%v left)", dk.CurrentRunicPower())
	}
}

// A strike queued when an iteration ends doesn't carry over to the next pull.
func TestRuneStrikeQueueClearsOnReset(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	start()
	dk.RuneStrikeAura.Activate(sim)
	if !dk.RuneStrikeQueue.Cast(sim, dk.CurrentTarget) {
		t.Fatal("Rune Strike didn't queue with its window up")
	}
	start()
	if dk.RuneStrikeQueued {
		t.Error("still queued after a reset")
	}
}
