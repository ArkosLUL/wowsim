package wotlk

import (
	"fmt"
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

func NewItemEffectWithHeroic(f func(isHeroic bool)) {
	f(true)
	f(false)
}

// ServerProc is how often an item or enchant proc aura fires on the server, from its proc entry.
type ServerProc struct {
	Chance float64 // 0-1; unused when PPM is set, PPM replaces it on any damage or heal event
	PPM    float64
	ICD    time.Duration
}

// ServerProcFor reads spellID's generated proc entry, at level 80. Panics when the entry is missing:
// rerun tools/acore/gen_serverdata after adding the constant.
func ServerProcFor(spellID int32) ServerProc {
	proc := serverdata.ProcBySpellID(spellID)
	if proc == nil {
		panic(fmt.Sprintf("no server proc entry for spell %d, rerun tools/acore/gen_serverdata", spellID))
	}
	sp := ServerProc{
		Chance: fromFloat32(proc.Chance) / 100,
		PPM:    fromFloat32(proc.ProcsPerMinute),
		ICD:    time.Duration(proc.CooldownMs) * time.Millisecond,
	}
	// Aura::CalcProcChance: 1/30 less per level past 60
	if proc.AttributesMask&serverdata.ProcAttrReduceProc60 != 0 {
		reduction := 1 - float64(core.CharacterLevel-60)/30
		sp.Chance *= reduction
		sp.PPM *= reduction
	}
	return sp
}

// ServerDuration is spellID's aura duration from the generated spell table.
func ServerDuration(spellID int32) time.Duration {
	spell := serverdata.SpellByID(spellID)
	if spell == nil || spell.DurationMs <= 0 {
		panic(fmt.Sprintf("no server duration for spell %d", spellID))
	}
	return time.Duration(spell.DurationMs) * time.Millisecond
}

// ServerEnchantPPM is a weapon enchant's proc rate from spell_enchant_proc_data. Panics when the enchant
// has no PPM row.
func ServerEnchantPPM(enchantID int32) float64 {
	proc := serverdata.EnchantProcByID(enchantID)
	if proc == nil || proc.PPM == 0 {
		panic(fmt.Sprintf("no spell_enchant_proc_data PPM for enchant %d", enchantID))
	}
	return fromFloat32(proc.PPM)
}

// fromFloat32 keeps the DB's decimal value, so 0.7 stays 0.7 rather than 0.699999988.
func fromFloat32(v float32) float64 {
	f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(v), 'g', -1, 32), 64)
	return f
}
