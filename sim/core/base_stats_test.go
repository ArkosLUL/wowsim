package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func TestRatingPerPercent(t *testing.T) {
	for _, tc := range []struct {
		class proto.Class
		cr    CombatRating
		want  float64
	}{
		{proto.Class_ClassWarrior, CRHasteMelee, 32.79},
		{proto.Class_ClassRogue, CRHasteMelee, 32.79},
		{proto.Class_ClassPaladin, CRHasteMelee, 25.223},
		{proto.Class_ClassDeathknight, CRHasteMelee, 25.223},
		{proto.Class_ClassShaman, CRHasteMelee, 25.223},
		{proto.Class_ClassDruid, CRHasteMelee, 25.223},
		{proto.Class_ClassHunter, CRArmorPenetration, 13.9957},
		{proto.Class_ClassWarrior, CRArmorPenetration, 13.9957},
		{proto.Class_ClassUnknown, CRArmorPenetration, 15.3953},
		{proto.Class_ClassMage, CRCritSpell, 45.906},
	} {
		if got := RatingPerPercent(tc.class, tc.cr); !WithinToleranceFloat64(tc.want, got, 0.001) {
			t.Errorf("%s rating %d: got %.4f, want %.4f", tc.class, tc.cr, got, tc.want)
		}
	}
}

func TestBaseStats(t *testing.T) {
	orcWarrior := BaseStats(proto.Race_RaceOrc, proto.Class_ClassWarrior)
	for stat, want := range map[stats.Stat]float64{
		stats.Health:            8121,
		stats.Strength:          177,
		stats.Agility:           110,
		stats.Stamina:           160,
		stats.Intellect:         33,
		stats.Spirit:            61,
		stats.AttackPower:       220, // level·3 − 20
		stats.RangedAttackPower: 70,  // level − 10
		stats.MeleeCrit:         3.1891 * CritRatingPerCritChance,
	} {
		if got := orcWarrior[stat]; !WithinToleranceFloat64(want, got, 0.001) {
			t.Errorf("orc warrior %s: got %.4f, want %.4f", stat.StatName(), got, want)
		}
	}

	// The server's player_class_stats, where Classic had 6960 and 7164.
	if got := BaseStats(proto.Race_RaceTroll, proto.Class_ClassShaman)[stats.Health]; got != 6939 {
		t.Errorf("shaman base health %.0f, want 6939", got)
	}
	if got := BaseStats(proto.Race_RaceOrc, proto.Class_ClassWarlock)[stats.Health]; got != 7136 {
		t.Errorf("warlock base health %.0f, want 7136", got)
	}
}

func TestDefenseSkillFromRating(t *testing.T) {
	for rating, want := range map[float64]int32{0: 0, 4: 0, 5: 1, 400: 81, 689: 140, 1000: 203} {
		if got := DefenseSkillFromRating(rating); got != want {
			t.Errorf("%.0f defense rating: got %d skill, want %d", rating, got, want)
		}
	}
}

func newAvoidanceUnit(race proto.Race, class proto.Class, bonus stats.Stats) *Unit {
	base := BaseStats(race, class)
	unit := &Unit{
		PseudoStats:     newPseudoStats(),
		playerAvoidance: newPlayerAvoidance(ClassStatScaling[class], base),
	}
	unit.stats = base.Add(bonus)
	unit.PseudoStats.CanParry = true
	return unit
}

func TestPlayerAvoidance(t *testing.T) {
	// A naked human warrior's dodge is all base: 3.664% plus 113 base agility.
	naked := newAvoidanceUnit(proto.Race_RaceHuman, proto.Class_ClassWarrior, stats.Stats{})
	for name, tc := range map[string]struct{ got, want float64 }{
		"dodge": {naked.DodgeChance(), 0.0500035},
		"parry": {naked.ParryChance(), 0.05},
		"miss":  {naked.DefenseMissChance(), 0},
	} {
		if !WithinToleranceFloat64(tc.want, tc.got, 1e-6) {
			t.Errorf("naked %s: got %.7f, want %.7f", name, tc.got, tc.want)
		}
	}

	// 400 defense rating is 81 whole skill points; it, the dodge rating and agility above base all diminish.
	geared := newAvoidanceUnit(proto.Race_RaceHuman, proto.Class_ClassWarrior, stats.Stats{
		stats.Agility: 100,
		stats.Dodge:   512,
		stats.Defense: 400,
	})
	for name, tc := range map[string]struct{ got, want float64 }{
		"dodge": {geared.DodgeChance(), 0.1887118},
		"parry": {geared.ParryChance(), 0.0816119},
		"miss":  {geared.DefenseMissChance(), 0.0279672},
	} {
		if !WithinToleranceFloat64(tc.want, tc.got, 1e-6) {
			t.Errorf("geared %s: got %.7f, want %.7f", name, tc.got, tc.want)
		}
	}

	if got := newAvoidanceUnit(proto.Race_RaceHuman, proto.Class_ClassPriest, stats.Stats{}).ParryChance(); got != 0 {
		t.Errorf("priests have no parry cap, so no parry; got %.4f", got)
	}
}

func TestArmorPenetrationPercentage(t *testing.T) {
	unit := &Unit{PseudoStats: newPseudoStats()}
	unit.PseudoStats.ArmorPenRatingPerPercent = RatingPerPercent(proto.Class_ClassWarrior, CRArmorPenetration)
	unit.PseudoStats.BonusArmorPenPct = 10 // Battle Stance

	if got := unit.ArmorPenetrationPercentage(700); !WithinToleranceFloat64(0.6001527, got, 1e-6) {
		t.Errorf("700 rating in Battle Stance: got %.7f", got)
	}
	// Rating and auras share the one cap.
	if got := unit.ArmorPenetrationPercentage(1300); got != 1 {
		t.Errorf("1300 rating in Battle Stance: got %.7f, want the cap", got)
	}
}
