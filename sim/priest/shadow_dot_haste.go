package priest

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// shadowDotAddsTicks is spell_tweaks_shadow_dot_haste: in Shadowform, spell haste shortens SW:P, VT
// and DP's tick interval and the duration stays put. The sim's Shadowform is permanent.
func (priest *Priest) shadowDotAddsTicks() bool {
	return priest.Talents.Shadowform && priest.Server().SpellTweaks.ShadowDotHaste
}

// shadowDotTickHaste is how VT and DP take haste: the tweak adds ticks, stock Shadowform (passive
// 71167) shortens the duration.
func shadowDotTickHaste(addsTicks bool) core.TickHaste {
	if addsTicks {
		return core.SpellHasteAddsTicks
	}
	return core.SpellHasteScalesBoth
}

// devouringPlagueTotalTicks is what AuraEffect::GetTotalTicks gives Improved Devouring Plague's burst
// for a cast now. Call it before the cast puts its dot on.
func (priest *Priest) devouringPlagueTotalTicks(target *core.Unit) int32 {
	dot := priest.DevouringPlague.Dot(target)
	stock := dot.TickHaste == core.SpellHasteScalesBoth
	// Stock haste shortens the duration with the interval, so a new dot counts its 8. A recast over
	// a running one counts more: Aura::RefreshTimers puts the whole duration back before the burst,
	// and the cast only shortens it after.
	if stock && !(dot.AffectedByCastSpeed && dot.IsActive()) {
		return dot.NumberOfTicks
	}

	period := priest.ApplyCastSpeedForSpell(dot.TickLength, priest.DevouringPlague)
	if !stock {
		// the tweak's script doesn't let a slow stretch the interval
		period = min(period, dot.TickLength)
	}
	period = period.Truncate(time.Millisecond)
	return int32(dot.TickLength * time.Duration(dot.NumberOfTicks) / period)
}
