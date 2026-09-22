package paladin

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

const SealDuration = time.Minute * 30

// sealCanProcOn mirrors the seal auras' own proc gate: melee auto attack or melee damage class only
// (Spell.dbc procFlags 0x14). Hammer of Wrath is ProcMaskMeleeMHSpecial like the other specials, but
// its damage class is Ranged, so no seal ever procs off it.
func (paladin *Paladin) sealCanProcOn(spell *core.Spell) bool {
	return spell.IsMelee() && spell != paladin.HammerOfWrath
}
