package optimizer

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// hasteCeilingFloor is the cast time both haste ceilings bisect toward: a caster's hasted GCD
// (Spell::TriggerGlobalCooldown's MIN_GCD) and, since a hunter's GCD isn't hasted, Steady Shot's own
// cast time.
const hasteCeilingFloor = time.Second

// steadyShotBaseCast is Steady Shot's cast time before ranged haste (sim/hunter's registerSteadyShotSpell).
const steadyShotBaseCast = 2 * time.Second

// maxCapBonus bounds every cap bisection: far more rating than any pool could offer, so a genuinely
// unreachable cap is reported rather than guessed at.
const maxCapBonus = 4000

// capRole is which of the DPS cap floors and the haste ceiling a spec gets. A tank gets the melee hit
// floor only, a healer none.
type capRole int

const (
	capRoleNone capRole = iota
	capRoleMelee
	capRoleHunter
	capRoleCaster
	capRoleTank
)

// dpsCapRole reads the target's role off its spec, the way objective.go's referenceStat does.
func dpsCapRole(player *proto.Player) capRole {
	switch player.GetSpec().(type) {
	case *proto.Player_FeralDruid, *proto.Player_Rogue, *proto.Player_RetributionPaladin,
		*proto.Player_EnhancementShaman, *proto.Player_Warrior, *proto.Player_Deathknight:
		return capRoleMelee
	case *proto.Player_Hunter:
		return capRoleHunter
	case *proto.Player_BalanceDruid, *proto.Player_Mage, *proto.Player_ShadowPriest,
		*proto.Player_SmitePriest, *proto.Player_ElementalShaman, *proto.Player_Warlock:
		return capRoleCaster
	case *proto.Player_ProtectionWarrior, *proto.Player_ProtectionPaladin,
		*proto.Player_FeralTankDruid, *proto.Player_TankDeathknight:
		return capRoleTank
	default:
		return capRoleNone
	}
}

// bisectCapBonus finds the smallest bonus to stat st, from -(seed rating) to maxCapBonus, where ok
// holds; like requiredDefense, it assumes ok is false below some crossing point and true from it on,
// and bisects downward too since a seed already past the crossing point says nothing about how much
// of its rating the search may trade away. It returns the sheet rating at that point.
func bisectCapBonus(r *Request, st stats.Stat, ok func(core.PlayerSheet) bool) (float64, error) {
	sheetAt := func(bonus float64) (core.PlayerSheet, error) {
		var offset stats.Stats
		offset[st] = bonus
		return playerSheet(r.Base, r.TargetIndex, r.Seed, offset)
	}
	seed, err := sheetAt(0)
	if err != nil {
		return 0, err
	}
	switch atMax, err := sheetAt(maxCapBonus); {
	case err != nil:
		return 0, err
	case !ok(atMax):
		return 0, fmt.Errorf("even %d more %s rating doesn't reach it", maxCapBonus, st.StatName())
	}
	lo, hi := -int(math.Ceil(seed.FinalStats[st])), maxCapBonus
	for lo < hi {
		// lo + (hi-lo)/2, not (lo+hi)/2: lo can be negative, and Go truncates toward zero
		mid := lo + (hi-lo)/2
		sheet, err := sheetAt(float64(mid))
		if err != nil {
			return 0, err
		}
		if ok(sheet) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	final, err := sheetAt(float64(lo))
	if err != nil {
		return 0, err
	}
	return final.FinalStats[st], nil
}

// addStatFloor raises settings' floor on st to at least value, the way critImmunityFloor raises
// Defense: one floor per stat, so the surrogate's penalty never prices the same stat twice. It returns
// the floor st had before (0 when it had none), and whether value raised it.
func addStatFloor(settings *proto.OptimizerSettings, st stats.Stat, value float64) (before float64, raised bool) {
	for _, floor := range settings.StatMinimums {
		if stats.Stat(floor.Stat) == st {
			if value <= floor.MinValue {
				return floor.MinValue, false
			}
			before, floor.MinValue = floor.MinValue, value
			return before, true
		}
	}
	settings.StatMinimums = append(settings.StatMinimums, &proto.StatMinimum{Stat: proto.Stat(st), MinValue: value})
	return 0, true
}

// addStatCeiling lowers settings' ceiling on st to at most value.
func addStatCeiling(settings *proto.OptimizerSettings, st stats.Stat, value float64) {
	for _, ceiling := range settings.StatMaximums {
		if stats.Stat(ceiling.Stat) == st {
			ceiling.MaxValue = min(ceiling.MaxValue, value)
			return
		}
	}
	settings.StatMaximums = append(settings.StatMaximums, &proto.StatMaximum{Stat: proto.Stat(st), MaxValue: value})
}

// dpsCapFloors adds the run's DPS cap floors (melee/ranged hit, expertise, spell hit) and haste
// ceiling for the target's role, the way critImmunityFloor adds Defense: before CompilePool, so the
// pool, the surrogate's penalty and every rule check see them. It records each floor it raised in
// r.capFloors, so verify can relax exactly those, back to where they were, when nothing reaches every
// floor.
func (r *run) dpsCapFloors() {
	if r.asked.Settings.GetDisableStatCaps() {
		return
	}
	role := dpsCapRole(r.r.Target())
	if role == capRoleNone {
		return
	}
	settings := r.r.Settings
	r.capFloors = map[stats.Stat]float64{}

	floor := func(name string, st stats.Stat, ok func(core.PlayerSheet) bool) {
		value, err := bisectCapBonus(r.r, st, ok)
		if err != nil {
			r.warn("the %s cap isn't enforced: %v", name, err)
			return
		}
		if before, raised := addStatFloor(settings, st, value); raised {
			r.capFloors[st] = before
		}
		r.trace("%s cap needs %.0f %s rating", name, value, st.StatName())
	}
	ceiling := func(name string, st stats.Stat, ok func(core.PlayerSheet) bool) {
		value, err := bisectCapBonus(r.r, st, ok)
		if err != nil {
			r.warn("the %s ceiling isn't enforced: %v", name, err)
			return
		}
		addStatCeiling(settings, st, value)
		r.trace("%s ceiling is %.0f %s rating", name, value, st.StatName())
	}

	switch role {
	case capRoleTank:
		// A taunt rolls spell hit, 17% against a boss. This floor's rating is 10 of it and the taunt
		// glyph adds 8, so a taunt without the glyph still misses (warnTauntGlyph).
		floor("hit", stats.MeleeHit, func(s core.PlayerSheet) bool { return s.PhysicalMissChance == 0 })
	case capRoleMelee, capRoleHunter:
		floor("hit", stats.MeleeHit, func(s core.PlayerSheet) bool { return s.PhysicalMissChance == 0 })
		if role == capRoleMelee {
			floor("expertise", stats.Expertise, func(s core.PlayerSheet) bool { return s.ExpertiseDodgeChance == 0 })
		} else {
			ceiling("haste", stats.MeleeHaste, func(s core.PlayerSheet) bool {
				return steadyShotCastTime(s.RangedSwingSpeed) <= hasteCeilingFloor
			})
		}
	case capRoleCaster:
		floor("spell hit", stats.SpellHit, func(s core.PlayerSheet) bool { return s.SpellMissChance == 0 })
		ceiling("haste", stats.SpellHaste, func(s core.PlayerSheet) bool {
			return globalCooldown(s.CastSpeed) <= hasteCeilingFloor
		})
	}
}

// tauntGlyph is the major glyph that adds 8 points of hit to a tank spec's taunt
// (SPELLMOD_RESIST_MISS_CHANCE on Taunt, Growl, Dark Command, and Hand of Reckoning with Righteous
// Defense), as the item id a player's glyphs hold, and its name.
func tauntGlyph(player *proto.Player) (id int32, name string) {
	switch player.GetSpec().(type) {
	case *proto.Player_ProtectionWarrior:
		return int32(proto.WarriorMajorGlyph_GlyphOfTaunt), "Glyph of Taunt"
	case *proto.Player_FeralTankDruid:
		return int32(proto.DruidMajorGlyph_GlyphOfGrowling), "Glyph of Growling"
	case *proto.Player_TankDeathknight:
		return int32(proto.DeathknightMajorGlyph_GlyphOfDarkCommand), "Glyph of Dark Command"
	case *proto.Player_ProtectionPaladin:
		return int32(proto.PaladinMajorGlyph_GlyphOfRighteousDefense), "Glyph of Righteous Defense"
	}
	return 0, ""
}

// A raid boss reads three levels above a level 80, which is what a taunt's 17% miss comes from.
const bossLevelDiff = 3

// The spell hit Misery and Improved Faerie Fire put on the boss, in points.
const spellHitDebuff = 3

// warnTauntGlyph warns a tank whose glyphs lack the class's taunt glyph how often a taunt then misses
// a boss. A taunt rolls spell hit, and the hit floor is only 10 of the 17 points it needs: the glyph
// is the rest. The floor's rating counts as spell hit too, since gear gives both at once; without a
// floor it's the gear as equipped that's measured.
func (r *run) warnTauntGlyph() {
	player := r.r.Target()
	glyph, name := tauntGlyph(player)
	glyphs := player.GetGlyphs()
	if glyph == 0 || glyph == glyphs.GetMajor1() || glyph == glyphs.GetMajor2() || glyph == glyphs.GetMajor3() {
		return
	}
	sheet, err := playerSheet(r.r.Base, r.r.TargetIndex, r.r.Equipped, stats.Stats{})
	if err != nil {
		r.warn("the glyphs have no %s, so a taunt can miss a boss: %v", name, err)
		return
	}
	spellHit, where := sheet.FinalStats[stats.SpellHit], "in the gear as equipped"
	for _, floor := range r.r.Settings.GetStatMinimums() {
		if stats.Stat(floor.Stat) == stats.MeleeHit {
			spellHit += floor.MinValue - sheet.FinalStats[stats.MeleeHit]
			where = "at the hit floor"
		}
	}
	missAt := func(debuff float64) float64 {
		hitPct := spellHit/core.SpellHitRatingPerHitChance + debuff
		return float64(core.SpellMissBP(bossLevelDiff, float32(hitPct), false)) / 100
	}
	r.warn("the glyphs have no %s, so %s a taunt misses a boss %.0f%% of the time (%.0f%% under Misery or Improved Faerie Fire)",
		name, where, missAt(0), missAt(spellHitDebuff))
}

// globalCooldown is Unit.SpellGCD: the default GCD scaled by castSpeed, floored at 1 s.
func globalCooldown(castSpeed float64) time.Duration {
	return max(core.GCDMin, time.Duration(float64(core.GCDDefault)*castSpeed))
}

// steadyShotCastTime is Steady Shot's cast time at a given ranged swing speed (Unit.ApplyRangedCastSpeed).
func steadyShotCastTime(rangedSwingSpeed float64) time.Duration {
	return time.Duration(float64(steadyShotBaseCast) / rangedSwingSpeed)
}

// capShortfall is the summed rating final is short of the floors in r.capFloors, plus a detail
// string naming each one that's short.
func (r *run) capShortfall(final stats.Stats) (float64, string) {
	total := 0.0
	var parts []string
	for _, floor := range r.r.Settings.StatMinimums {
		st := stats.Stat(floor.Stat)
		if _, capped := r.capFloors[st]; !capped {
			continue
		}
		if short := floor.MinValue - final[st]; short > 0 {
			total += short
			parts = append(parts, fmt.Sprintf("%s %.0f short", st.StatName(), short))
		}
	}
	return total, strings.Join(parts, ", ")
}

// closestToCapFloors picks, among candidates plus the seed, the one nearest the run's DPS cap floors
// while still keeping every other rule, for when nothing reaches every floor. "Nearest" is the summed
// rating shortfall over just those floors, each relaxed back to what it was before the caps raised
// it, so a floor the settings set themselves stays enforced.
func (r *run) closestToCapFloors(candidates []Loadout) (pick Loadout, detail string, ok bool) {
	// a seed that keeps every rule reaches every floor, so nothing short of one may replace it
	if len(r.capFloors) == 0 || r.seedBroken == nil {
		return Loadout{}, "", false
	}
	bestShort := math.Inf(1)
	all := make([]Loadout, 0, len(candidates)+1)
	all = append(all, r.r.Seed)
	all = append(all, candidates...)
	for _, l := range all {
		// the seed, unlike a search candidate, isn't already known to pass CheckGear
		if r.pool.CheckGear(l) != nil {
			continue
		}
		if err := r.pool.checkFloorsRelaxed(l, r.capFloors); err != nil {
			continue
		}
		sheet, err := r.pool.sheet(l)
		if err != nil {
			continue
		}
		if short, d := r.capShortfall(sheet.FinalStats); short < bestShort {
			pick, bestShort, detail, ok = l, short, d, true
		}
	}
	return pick, detail, ok
}
