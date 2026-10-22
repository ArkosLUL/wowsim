// Package statcaps works out a player's stat caps off their character sheet (core.PlayerSheet):
// hit, expertise, armor penetration, Defense and the haste ceilings, for the caps tab and the
// optimizer's floors.
package statcaps

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// HasteCeilingFloor is the cast time both haste ceilings bisect toward: a caster's hasted GCD
// (Spell::TriggerGlobalCooldown's MIN_GCD) and, since a hunter's GCD isn't hasted, Steady Shot's own
// cast time.
const HasteCeilingFloor = time.Second

// steadyShotBaseCast is Steady Shot's cast time before ranged haste (sim/hunter's registerSteadyShotSpell).
const steadyShotBaseCast = 2 * time.Second

// MaxCapBonus bounds every cap bisection: far more rating than any gear could offer, so a genuinely
// unreachable cap is reported rather than guessed at.
const MaxCapBonus = 4000

// maxDefenseBonus caps the Defense bisection. 2000 rating is 406 defense skill, and 140 over the
// level cap's 400 is all the crit-taken formula asks of a tank with no defense at all.
const maxDefenseBonus = 2000

// Role is which caps a spec gets.
type Role int

const (
	RoleNone Role = iota
	RoleMelee
	RoleHunter
	RoleCaster
	RoleTank
	RoleHealer
)

// RoleOf reads the player's role off its spec, the way the optimizer's referenceStat does.
func RoleOf(player *proto.Player) Role {
	switch player.GetSpec().(type) {
	case *proto.Player_FeralDruid, *proto.Player_Rogue, *proto.Player_RetributionPaladin,
		*proto.Player_EnhancementShaman, *proto.Player_Warrior, *proto.Player_Deathknight:
		return RoleMelee
	case *proto.Player_Hunter:
		return RoleHunter
	case *proto.Player_BalanceDruid, *proto.Player_Mage, *proto.Player_ShadowPriest,
		*proto.Player_SmitePriest, *proto.Player_ElementalShaman, *proto.Player_Warlock:
		return RoleCaster
	case *proto.Player_ProtectionWarrior, *proto.Player_ProtectionPaladin,
		*proto.Player_FeralTankDruid, *proto.Player_TankDeathknight:
		return RoleTank
	case *proto.Player_HolyPaladin, *proto.Player_RestorationDruid, *proto.Player_RestorationShaman,
		*proto.Player_HealingPriest:
		return RoleHealer
	default:
		return RoleNone
	}
}

// SheetAt is one player's character sheet with offset added to their bonus stats.
type SheetAt func(offset stats.Stats) (core.PlayerSheet, error)

// BisectCapBonus finds the smallest bonus to stat st, from -(seed rating) to MaxCapBonus, where ok
// holds; like RequiredDefense, it assumes ok is false below some crossing point and true from it on,
// and bisects downward too since a seed already past the crossing point says nothing about how much
// of its rating could be traded away. It returns the sheet rating at that point.
func BisectCapBonus(sheetAt SheetAt, st stats.Stat, ok func(core.PlayerSheet) bool) (float64, error) {
	at := func(bonus float64) (core.PlayerSheet, error) {
		var offset stats.Stats
		offset[st] = bonus
		return sheetAt(offset)
	}
	seed, err := at(0)
	if err != nil {
		return 0, err
	}
	switch atMax, err := at(MaxCapBonus); {
	case err != nil:
		return 0, err
	case !ok(atMax):
		return 0, fmt.Errorf("even %d more %s rating doesn't reach it", MaxCapBonus, st.StatName())
	}
	lo, hi := -int(math.Ceil(seed.FinalStats[st])), MaxCapBonus
	for lo < hi {
		// lo + (hi-lo)/2, not (lo+hi)/2: lo can be negative, and Go truncates toward zero
		mid := lo + (hi-lo)/2
		sheet, err := at(float64(mid))
		if err != nil {
			return 0, err
		}
		if ok(sheet) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	final, err := at(float64(lo))
	if err != nil {
		return 0, err
	}
	return final.FinalStats[st], nil
}

// RequiredDefense is D*: the sheet Defense rating at which the encounter's boss stops critting the
// player. It bisects bonus Defense around the seed, since defense skill comes in whole points, and
// reads the resulting sheet rather than assuming a rating point is a sheet point.
//
// Everything else the crit formula reads (resilience, talent crit reductions, the boss's own crit)
// comes off the seed, so gear that moves those moves the real answer too.
func RequiredDefense(sheetAt SheetAt) (float64, error) {
	at := func(bonus float64) (core.PlayerSheet, error) {
		var offset stats.Stats
		offset[stats.Defense] = bonus
		return sheetAt(offset)
	}
	seed, err := at(0)
	if err != nil {
		return 0, err
	}
	if !seed.MeleeAttacker {
		return 0, errors.New("nothing in the encounter swings at the target, so there's no melee crit to be immune to")
	}
	immune := func(bonus int) (bool, error) {
		sheet, err := at(float64(bonus))
		return sheet.MeleeAttacker && sheet.MeleeCritTakenChance == 0, err
	}
	switch ok, err := immune(maxDefenseBonus); {
	case err != nil:
		return 0, err
	case !ok:
		return 0, fmt.Errorf("even %d more Defense rating leaves the boss a %.2f%% crit chance", maxDefenseBonus, seed.MeleeCritTakenChance*100)
	}
	// down as well as up: a seed that is already immune says nothing about how much of its defense
	// could be traded away
	lo, hi := -int(math.Ceil(seed.FinalStats[stats.Defense])), maxDefenseBonus
	for lo < hi {
		// lo + (hi-lo)/2, not (lo+hi)/2: lo can be negative, and Go truncates toward zero
		mid := lo + (hi-lo)/2
		ok, err := immune(mid)
		if err != nil {
			return 0, err
		}
		if ok {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	final, err := at(float64(lo))
	if err != nil {
		return 0, err
	}
	return final.FinalStats[stats.Defense], nil
}

// GlobalCooldown is Unit.SpellGCD: the default GCD scaled by castSpeed, floored at 1 s.
func GlobalCooldown(castSpeed float64) time.Duration {
	return max(core.GCDMin, time.Duration(float64(core.GCDDefault)*castSpeed))
}

// SteadyShotCastTime is Steady Shot's cast time at a given ranged swing speed (Unit.ApplyRangedCastSpeed).
func SteadyShotCastTime(rangedSwingSpeed float64) time.Duration {
	return time.Duration(float64(steadyShotBaseCast) / rangedSwingSpeed)
}
