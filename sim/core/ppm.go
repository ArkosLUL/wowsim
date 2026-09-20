package core

import (
	"fmt"
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// ServerProc is how often an item, enchant or talent aura fires, from its generated spell_proc entry.
type ServerProc struct {
	Chance float64 // 0-1; unused when PPM is set, PPM replaces it on any damage or heal event
	PPM    float64
	ICD    time.Duration
}

// ServerProcFor reads spellID's generated proc entry, at CharacterLevel, and is the only place
// PROC_ATTR_REDUCE_PROC_60 is applied. Panics when the entry is missing: rerun
// tools/acore/gen_serverdata after adding the constant.
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
		reduction := max(0, 1-float64(CharacterLevel-60)/30)
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

// spellPPMFloorMs is the cast time Aura::CalcProcChance gives an instant or fast spell.
const spellPPMFloorMs = 1500

// ppmProcChance is Unit::GetPPMProcChance as a 0-1 probability: a proc of `ppm` per minute rolled once
// every `basis`.
func ppmProcChance(basis time.Duration, ppm float64) float64 {
	return float64(basis.Milliseconds()) * ppm / 60000
}

// AuraPPMProcChance is the chance an aura with a spell_proc PPM entry procs off spell, following
// Aura::CalcProcChance. Item chance-on-hit spells and weapon enchants don't come through here: their
// Player::CastItemCombatSpell path always measures against the weapon (PPMManager).
func (unit *Unit) AuraPPMProcChance(ppm float64, spell *Spell) float64 {
	return ppmProcChance(unit.auraPPMBasis(spell), ppm)
}

// auraPPMBasis is the attackSpeed Aura::CalcProcChance feeds GetPPMProcChance: the unhasted weapon
// attack time for a white swing, a melee-class spell and a ranged weapon spell, and the spell's own
// base cast time, floored at 1.5 s, for everything else.
func (unit *Unit) auraPPMBasis(spell *Spell) time.Duration {
	if spell == nil || spell.usesWeaponSpeedForPPM() {
		return unit.weaponPPMBasis(spell)
	}

	castMs := int32(0)
	if ss := spell.ServerSpell(); ss != nil {
		castMs = ss.BaseCastMs
	} else if spell.DefaultCast.CastTime > 0 {
		castMs = int32(spell.DefaultCast.CastTime.Milliseconds())
	}
	return time.Duration(max(castMs, spellPPMFloorMs)) * time.Millisecond
}

func (unit *Unit) weaponPPMBasis(spell *Spell) time.Duration {
	weapon := unit.AutoAttacks.MH()
	if spell != nil {
		switch {
		case spell.ProcMask.Matches(ProcMaskMeleeOH):
			weapon = unit.AutoAttacks.OH()
		case spell.ProcMask.Matches(ProcMaskRanged):
			weapon = unit.AutoAttacks.Ranged()
		}
	}
	return DurationFromSeconds(weapon.SwingSpeed)
}

// usesWeaponSpeedForPPM reports whether Aura::CalcProcChance measures this spell's PPM against the
// weapon: SPELL_DAMAGE_CLASS_MELEE or SpellInfo::IsRangedWeaponSpell. A white swing carries no spell at
// all on the server, so it always does.
//
// The generated data has no EquippedItemSubClassMask, so IsRangedWeaponSpell's third check is missing.
// The hunter family and the ranged slot cover every spell the sim registers.
func (spell *Spell) usesWeaponSpeedForPPM() bool {
	if spell.ProcMask.Matches(ProcMaskWhiteHit) {
		return true
	}
	ss := spell.ServerSpell()
	if ss == nil {
		return spell.ProcMask.Matches(ProcMaskMeleeOrRanged)
	}
	const hunterFamily, hunterNonRangedFlag = 9, 0x10000000
	return ss.DmgClass == serverdata.DmgClassMelee ||
		ss.Flags&serverdata.FlagUsesRangedSlot != 0 ||
		(ss.Family == hunterFamily && ss.FamilyFlags[1]&hunterNonRangedFlag == 0)
}
