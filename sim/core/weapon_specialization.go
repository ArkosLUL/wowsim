package core

import (
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// WeaponSpecialization is a racial weapon specialization: expertise rating on the weapon types it covers.
type WeaponSpecialization struct {
	Expertise   float64
	WeaponTypes []proto.WeaponType
}

var weaponSpecializations = map[proto.Race]WeaponSpecialization{
	proto.Race_RaceDwarf: {5 * ExpertisePerQuarterPercentReduction, []proto.WeaponType{proto.WeaponType_WeaponTypeMace}},
	proto.Race_RaceHuman: {3 * ExpertisePerQuarterPercentReduction, []proto.WeaponType{proto.WeaponType_WeaponTypeMace, proto.WeaponType_WeaponTypeSword}},
	proto.Race_RaceOrc:   {5 * ExpertisePerQuarterPercentReduction, []proto.WeaponType{proto.WeaponType_WeaponTypeAxe, proto.WeaponType_WeaponTypeFist}},
}

// RacialWeaponSpecialization is the weapon specialization race's traits give; false when they give none.
func RacialWeaponSpecialization(race proto.Race) (WeaponSpecialization, bool) {
	spec, ok := weaponSpecializations[race]
	return spec, ok
}

// hands is the hands holding a weapon type spec covers, and whether the bonus goes on the Expertise
// stat: only when every hand holding a weapon is covered. Otherwise it goes on the covered hand's
// spells, which the character sheet doesn't show.
func (spec WeaponSpecialization) hands(mh, oh *Item) (mask ProcMask, onStat bool) {
	if slices.Contains(spec.WeaponTypes, mh.WeaponType) {
		mask |= ProcMaskMeleeMH
	}
	if slices.Contains(spec.WeaponTypes, oh.WeaponType) {
		mask |= ProcMaskMeleeOH
	}
	ohWeapon := oh.ID != 0 && oh.WeaponType != proto.WeaponType_WeaponTypeShield && oh.WeaponType != proto.WeaponType_WeaponTypeOffHand
	return mask, mask == ProcMaskMelee || mask == ProcMaskMeleeMH && !ohWeapon
}

// StatBonus is what spec adds to the Expertise stat with mh and oh equipped (an empty hand is a zero Item).
func (spec WeaponSpecialization) StatBonus(mh, oh *Item) float64 {
	if _, onStat := spec.hands(mh, oh); onStat {
		return spec.Expertise
	}
	return 0
}
