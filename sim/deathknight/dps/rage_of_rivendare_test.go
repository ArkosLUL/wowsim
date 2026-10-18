package dps

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// Every rank of Rage of Rivendare and Tundra Stalker has Heart Strike in its class mask, so the knight's own strike
// takes the bonus through MeleeDamageBonusDone's DoneTotalMod, its chain to a second target too.
func TestHeartStrikeTakesRageOfRivendare(t *testing.T) {
	dealt := func(bonus float64) (main, chained float64) {
		sim, dk := runeWeaponSim(t)
		dk.RoRTSBonus = func(*core.Unit) float64 { return bonus }
		target := sim.GetTargetUnit(0)
		// the blood runes the main strike spends
		dk.HeartStrike.SkipCastAndApplyEffects(sim, target)
		dk.HeartStrike.SkipCastAndApplyEffects(sim, target)
		for range 10 {
			dk.HeartStrikeOffHit.Cast(sim, target)
		}
		return dk.HeartStrike.SpellMetrics[target.UnitIndex].TotalDamage, dk.HeartStrikeOffHit.SpellMetrics[target.UnitIndex].TotalDamage
	}
	plainMain, plainChained := dealt(1)
	main, chained := dealt(1.1)
	// each hit whole
	if plainMain == 0 || math.Abs(main-plainMain*1.1) > 2 {
		t.Errorf("Heart Strike dealt %v under a 10%% bonus and %v without", main, plainMain)
	}
	if plainChained == 0 || math.Abs(chained-plainChained*1.1) > 10 {
		t.Errorf("its chain dealt %v under a 10%% bonus and %v without", chained, plainChained)
	}
}

// Unit::MeleeDamageBonusDone checks the target for anyone's Blood Plague (Rage of Rivendare) or Frost Fever
// (Tundra Stalker), so the rune weapon's count. Not a build anyone takes with the weapon, but another knight's
// talent reads them too.
func TestRageOfRivendareCountsTheRuneWeaponsDiseases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tree    int
		index   int
		disease func(*DpsDeathknight) *core.Spell
		bonus   float64
	}{
		{"Rage of Rivendare", 2, 29, func(dk *DpsDeathknight) *core.Spell { return dk.RuneWeapon.BloodPlagueSpell }, 1.1},
		{"Tundra Stalker", 1, 27, func(dk *DpsDeathknight) *core.Spell { return dk.RuneWeapon.FrostFeverSpell }, 1.15},
	} {
		t.Run(tc.name, func(t *testing.T) {
			trees := strings.Split(BloodTalents, "-")
			trees[tc.tree] += strings.Repeat("0", tc.index-len(trees[tc.tree])) + "5"
			sim, dk := runeWeaponSimWith(t, strings.Join(trees, "-"))
			target := sim.GetTargetUnit(0)
			if got := dk.RoRTSBonus(target); got != 1 {
				t.Fatalf("bonus %v with no disease up, want 1", got)
			}
			dk.RuneWeapon.EnableWithTimeout(sim, dk.RuneWeapon, 12*time.Second)
			tc.disease(dk).Cast(sim, target)
			if dk.FrostFeverSpell.Dot(target).IsActive() || dk.BloodPlagueSpell.Dot(target).IsActive() {
				t.Fatal("the knight's own disease is up")
			}
			if got := dk.RoRTSBonus(target); math.Abs(got-tc.bonus) > 1e-9 {
				t.Errorf("bonus %v with the weapon's disease up, want %v", got, tc.bonus)
			}
		})
	}
}
