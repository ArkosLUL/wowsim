package tank

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// Spell::prepare's CheckCast checks the runic power along with the window, so a press short of 20 queues nothing.
func TestRuneStrikePressNeedsTheRunicPower(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	start()
	if dk.CurrentRunicPower() >= 20 {
		t.Fatalf("starts with %v runic power, want under 20", dk.CurrentRunicPower())
	}
	dk.RuneStrikeAura.Activate(sim)
	if dk.RuneStrikeQueue.Cast(sim, dk.CurrentTarget) || dk.RuneStrikeQueued {
		t.Errorf("Rune Strike queued with %v runic power", dk.CurrentRunicPower())
	}
	dk.AddRunicPower(sim, 20, dk.NewRunicPowerMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionRefund}))
	if !dk.RuneStrikeQueue.Cast(sim, dk.CurrentTarget) || !dk.RuneStrikeQueued {
		t.Errorf("Rune Strike didn't queue with %v runic power", dk.CurrentRunicPower())
	}
}

// Unit::AttackerStateUpdate runs the queued strike through CheckCast at the swing and finishes it on a failure.
// Out of runic power the white swing still goes; without 56817, which CheckCast reads first, the swing is lost.
func TestRuneStrikeQueueAtTheSwing(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	start()
	target := dk.CurrentTarget
	white := dk.AutoAttacks.MHAuto()
	rpMetrics := dk.NewRunicPowerMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionRefund})

	// spend, cast between the press and the swing, takes the runic power the press needed
	swing := func(window bool, spend *core.Spell) *core.Spell {
		t.Helper()
		dk.RuneStrikeAura.Activate(sim)
		if !dk.RuneStrikeQueue.Cast(sim, target) || !dk.RuneStrikeQueued {
			t.Fatalf("Rune Strike didn't queue with its window up and %v runic power", dk.CurrentRunicPower())
		}
		if spend != nil && !spend.Cast(sim, target) {
			t.Fatalf("%v didn't go", spend.ActionID)
		}
		if !window {
			dk.RuneStrikeAura.Deactivate(sim)
		}
		got := dk.runeStrikeAtSwing(sim, white)
		if dk.RuneStrikeQueued {
			t.Errorf("window %v, %v runic power: still queued after the swing", window, dk.CurrentRunicPower())
		}
		return got
	}

	if dk.CurrentRunicPower() >= 20 {
		t.Fatalf("starts with %v runic power, want under 20", dk.CurrentRunicPower())
	}
	dk.AddRunicPower(sim, 20, rpMetrics)
	if got := swing(true, dk.IceboundFortitude); got != white {
		t.Errorf("short of runic power: swing is %v, want the white swing", got.ActionID)
	}
	dk.AddRunicPower(sim, 20, rpMetrics)
	if got := swing(false, dk.AntiMagicShell); got == white || got == dk.RuneStrike {
		t.Errorf("short of runic power without the window: swing is %v, want it lost", got.ActionID)
	}

	dk.AddRunicPower(sim, 20, rpMetrics)
	if got := swing(true, nil); got != dk.RuneStrike {
		t.Errorf("window up with %v runic power: swing is %v, want Rune Strike", dk.CurrentRunicPower(), got.ActionID)
	}

	lost := swing(false, nil)
	if lost == white || lost == dk.RuneStrike {
		t.Fatalf("window gone with %v runic power: swing is %v, want it lost", dk.CurrentRunicPower(), lost.ActionID)
	}
	runicPower := dk.CurrentRunicPower()
	whiteDamage := white.SpellMetrics[target.UnitIndex].TotalDamage
	strikeDamage := dk.RuneStrike.SpellMetrics[target.UnitIndex].TotalDamage
	lost.Cast(sim, target)
	if white.SpellMetrics[target.UnitIndex].TotalDamage != whiteDamage ||
		dk.RuneStrike.SpellMetrics[target.UnitIndex].TotalDamage != strikeDamage || dk.CurrentRunicPower() != runicPower {
		t.Errorf("the lost swing dealt damage or spent runic power (%v left)", dk.CurrentRunicPower())
	}
}

// A strike queued when an iteration ends doesn't carry over to the next pull.
func TestRuneStrikeQueueClearsOnReset(t *testing.T) {
	sim, dk, start := newTankFixesDK(t)
	start()
	dk.AddRunicPower(sim, 20, dk.NewRunicPowerMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionRefund}))
	dk.RuneStrikeAura.Activate(sim)
	if !dk.RuneStrikeQueue.Cast(sim, dk.CurrentTarget) {
		t.Fatal("Rune Strike didn't queue with its window up")
	}
	start()
	if dk.RuneStrikeQueued {
		t.Error("still queued after a reset")
	}
}
