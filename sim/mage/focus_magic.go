package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

func (mage *Mage) applyFocusMagic() {
	if !mage.Talents.FocusMagic {
		return
	}

	// This is used only for the individual sim.
	if mage.Party.Raid.Size() == 1 {
		if mage.Options.FocusMagicPercentUptime > 0 && !mage.Playerbot {
			selfAura, _ := core.FocusMagicAura(&mage.Unit, nil)
			core.ApplyFixedUptimeAura(selfAura, float64(mage.Options.FocusMagicPercentUptime)/100, time.Second*10, 1)
		}
		return
	}

	focusMagicTarget := mage.GetUnit(mage.Options.FocusMagicTarget)
	if focusMagicTarget == nil {
		return
	} else if focusMagicTarget == &mage.Unit {
		// When self is selected, give permanent self buff.
		if !mage.Playerbot {
			selfAura, _ := core.FocusMagicAura(&mage.Unit, nil)
			core.MakePermanent(selfAura)
		}
		return
	} else if mage.Playerbot {
		// the target keeps its crit, a bot mage gets none back
		core.FocusMagicAura(nil, focusMagicTarget)
		return
	}

	core.FocusMagicAura(&mage.Unit, focusMagicTarget)
}
