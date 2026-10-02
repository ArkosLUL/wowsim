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

// devouringPlagueTotalTicks is what AuraEffect::GetTotalTicks gives the dot a cast now would apply.
func (priest *Priest) devouringPlagueTotalTicks(target *core.Unit) int32 {
	dot := priest.DevouringPlague.Dot(target)
	if dot.TickHaste == core.SpellHasteScalesBoth {
		return dot.NumberOfTicks
	}
	period := min(priest.ApplyCastSpeedForSpell(dot.TickLength, priest.DevouringPlague), dot.TickLength).Truncate(time.Millisecond)
	return int32(dot.TickLength * time.Duration(dot.NumberOfTicks) / period)
}
