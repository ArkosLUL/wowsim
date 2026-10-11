package core

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/stats"
)

func strikeTestUnits() (unit, target *Unit) {
	unit = &Unit{Type: PlayerUnit}
	unit.PseudoStats = stats.NewPseudoStats()
	target = &Unit{Type: EnemyUnit}
	target.PseudoStats = stats.NewPseudoStats()
	unit.AttackTables = []*AttackTable{{Attacker: unit, Defender: target, DamageDealtMultiplier: 1, DamageTakenMultiplier: 1,
		NatureDamageTakenMultiplier: 1, HauntSEDamageTakenMultiplier: 1}}
	return unit, target
}

// The strikes probe's Mortal Strike with Two-Handed Weapon Specialization and Improved Mortal Strike: a 606-910
// sword at 1344 attack power is int((606 + 316.8) × 1.06) = 978 to 1300, the 380 fixed int(402.8) = 402, and
// the 10% comes last, int(1380 × 1.1) = 1518 to 1872. One truncation at the end gives 1519.
func TestWholeStrikeFollowsEffectWeaponDmg(t *testing.T) {
	sim := SetupFakeSim()
	unit, target := strikeTestUnits()
	unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] = 1.06
	mortalStrike := serverDataSpell(t, unit, 47486, ProcMaskMeleeMHSpecial)
	mortalStrike.SpellSchool, mortalStrike.SchoolIndex = SpellSchoolPhysical, stats.SchoolIndexPhysical
	mortalStrike.TotalPctMultiplier, mortalStrike.DamageMultiplier, mortalStrike.DamageMultiplierAdditive = 1, 1.1, 1

	for _, tc := range []struct{ weapon, wantWeapon, wantDone float64 }{{606, 978, 1518}, {910, 1300, 1872}} {
		unit.AutoAttacks.mh.Weapon = Weapon{BaseDamageMin: tc.weapon, BaseDamageMax: tc.weapon, SwingSpeed: 3.4,
			NormalizedSwingSpeed: 3.3, AttackPowerPerDPS: DefaultAttackPowerPerDPS}
		weapon := mortalStrike.MHNormalizedStrikeDamage(sim, 1344)
		if weapon != tc.wantWeapon {
			t.Errorf("weapon damage %v, want %v", weapon, tc.wantWeapon)
		}
		base := ApplyPct(mortalStrike.StrikeFixed(380, MainHand)+weapon, 1)
		attackerMultiplier := mortalStrike.AttackerDamageMultiplier(unit.AttackTables[0])
		if got := mortalStrike.doneDamage(target, base, attackerMultiplier, false); got != tc.wantDone {
			t.Errorf("done damage %v, want %v", got, tc.wantDone)
		}
	}
}

// A strike's weapon roll is urand between the truncated ends: every whole value, both ends included.
func TestStrikeDamageRollsWholeValues(t *testing.T) {
	sim := SetupFakeSim()
	unit, _ := strikeTestUnits()
	unit.AutoAttacks.mh.Weapon = Weapon{BaseDamageMin: 10.4, BaseDamageMax: 12.9, SwingSpeed: 2, NormalizedSwingSpeed: 2.4,
		AttackPowerPerDPS: DefaultAttackPowerPerDPS}
	spell := &Spell{Unit: unit, ProcMask: ProcMaskMeleeMHAuto, SpellSchool: SpellSchoolPhysical, TotalPctMultiplier: 1}
	seen := map[float64]int{}
	for range 300 {
		seen[spell.MHStrikeDamage(sim, 0)]++
	}
	if len(seen) != 3 || seen[10] == 0 || seen[11] == 0 || seen[12] == 0 {
		t.Errorf("rolls %v, want 10, 11 and 12", seen)
	}
	if !spell.wholeStrike {
		t.Error("the roll didn't mark the spell")
	}
}

// The off hand's half and Dual Wield Specialization are its TOTAL_PCT, which a physical strike's weapon and
// fixed bonus take before truncating: the probe's Mutilate, int(181 × 0.78) = 141. A strike off the physical
// school takes neither (Lava Lash, Frost Strike).
func TestStrikeHandPercent(t *testing.T) {
	sim := SetupFakeSim()
	unit, target := strikeTestUnits()
	unit.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] = 1.04
	unit.AutoAttacks.oh.Weapon = Weapon{BaseDamageMin: 153, BaseDamageMax: 153, SwingSpeed: 1.8, NormalizedSwingSpeed: 1.7,
		AttackPowerPerDPS: DefaultAttackPowerPerDPS}

	mutilate := &Spell{Unit: unit, ProcMask: ProcMaskMeleeOHSpecial, SpellSchool: SpellSchoolPhysical,
		TotalPctMultiplier: 1.5, DamageMultiplier: 1.2 * 1.5, DamageMultiplierAdditive: 1}
	if got := mutilate.StrikeFixed(181, OffHand); got != 141 {
		t.Errorf("off-hand fixed bonus %v, want 141", got)
	}
	// int((153 + 1461 / 14 × 1.7) × 0.78) = 257, as recorded
	weapon := mutilate.OHNormalizedStrikeDamage(sim, 1461)
	if weapon != 257 {
		t.Errorf("off-hand weapon damage %v, want 257", weapon)
	}
	// Opportunity's 20% is all that's left for the done step: int(398 × 1.2) = 477
	if got := mutilate.doneDamage(target, 141+weapon, 0, false); got != 477 {
		t.Errorf("off-hand done damage %v, want 477", got)
	}

	lavaLash := &Spell{Unit: unit, ProcMask: ProcMaskMeleeOHSpecial, SpellSchool: SpellSchoolFire, TotalPctMultiplier: 1}
	if got := lavaLash.StrikeFixed(181, OffHand); got != 181 {
		t.Errorf("a fire strike's fixed bonus %v, want 181", got)
	}
	// int(153 + 177.4)
	if got := lavaLash.OHNormalizedStrikeDamage(sim, 1461); got != 330 {
		t.Errorf("a fire strike's off-hand weapon damage %v, want 330", got)
	}
}

func TestApplyPct(t *testing.T) {
	for _, tc := range []struct{ damage, pct, want float64 }{
		// Crusader Strike's 75% and the ghoul's Claw at 150%, both recorded
		{911, 0.75, 683},
		{163, 1.5, 244},
		{1380, 1, 1380},
		{-7, 0.5, -3},
	} {
		if got := ApplyPct(tc.damage, tc.pct); got != tc.want {
			t.Errorf("ApplyPct(%v, %v) = %v, want %v", tc.damage, tc.pct, got, tc.want)
		}
	}
	if got := ApplyPct(math.NaN(), 1.5); !math.IsNaN(got) {
		t.Errorf("a hand without a weapon deals %v, want NaN", got)
	}
}

// The bleed auras' percent goes by the spell's mechanic mask, hit and ticks alike: Savage Rend's first hit
// takes it, Pin's ticks, a root's, don't. Shred counts as a bleed on a weapon hit.
func TestBleedBonusGoesByMechanic(t *testing.T) {
	unit, target := strikeTestUnits()
	target.PseudoStats.PeriodicPhysicalDamageTakenMultiplier = 1.3
	table := unit.AttackTables[0]
	for _, tc := range []struct {
		name     string
		spell    *Spell
		periodic bool
		want     float64
	}{
		{"Savage Rend's hit", serverDataSpell(t, unit, 53582, ProcMaskSpellDamage), false, 1.3},
		{"Savage Rend's tick", serverDataSpell(t, unit, 53582, ProcMaskSpellDamage), true, 1.3},
		{"Pin's tick", serverDataSpell(t, unit, 53548, ProcMaskEmpty), true, 1},
		{"Lacerate's hit", serverDataSpell(t, unit, 48568, ProcMaskMeleeMHSpecial), false, 1.3},
		{"Shred", serverDataSpell(t, unit, 48572, ProcMaskMeleeMHSpecial), false, 1.3},
		{"Mangle (Cat)", serverDataSpell(t, unit, 48566, ProcMaskMeleeMHSpecial), false, 1},
		{"a physical dot without server data", &Spell{Unit: unit}, true, 1.3},
		{"a physical hit without server data", &Spell{Unit: unit}, false, 1},
	} {
		tc.spell.SpellSchool, tc.spell.SchoolIndex = SpellSchoolPhysical, stats.SchoolIndexPhysical
		if got := tc.spell.TargetDamageMultiplier(table, tc.periodic); got != tc.want {
			t.Errorf("%s: target multiplier %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A heal outside a sim spell is whole before and after the healing taken percent: int(int(1000.7) × 1.07) = 1070.
func TestGainHealingIsWhole(t *testing.T) {
	sim := SetupFakeSim()
	character := sim.Raid.Parties[0].Players[0].(*FakeAgent).GetCharacter()
	character.PseudoStats.HealingTakenMultiplier = 1.07
	metrics := character.NewHealthMetrics(ActionID{SpellID: 43})
	character.stats[stats.Health] = 10000
	character.EnableHealthBar()
	character.healthBar.currentHealth = 5000
	before := character.CurrentHealth()
	character.GainHealing(sim, 1000.7, metrics)
	if got := character.CurrentHealth() - before; got != 1070 {
		t.Errorf("healed %v, want 1070", got)
	}
}

func TestRollWholeCoversBothEnds(t *testing.T) {
	sim := SetupFakeSim()
	seen := map[float64]int{}
	for range 300 {
		seen[sim.RollWhole(310, 312, "test")]++
	}
	if len(seen) != 3 || seen[310] == 0 || seen[312] == 0 {
		t.Errorf("rolls %v, want 310, 311 and 312", seen)
	}
}
