package main

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// `.simval procs` records: the server's own proc chance for every equipped item
// spell, weapon enchant and applied proc aura, per attack type and for one spell.
//
// Two things are compared. The generated spell_proc row has to be the row the live
// server used, field for field, and core.ServerProcFor has to turn it into the same
// chance the server computed: percent for a flat entry, Unit::GetPPMProcChance for a
// PPM one, with PROC_ATTR_REDUCE_PROC_60 taken off both.

// spellPPMFloorMs is the cast time Aura::CalcProcChance gives an instant or fast
// spell, mirroring sim/core/ppm.go.
const spellPPMFloorMs = 1500

// maxChancePct is how far a computed proc chance may sit from the server's, in
// percentage points. The module prints three decimals.
const maxChancePct = 0.001

func checkProcs(rec record) []Check {
	var checks []Check
	for _, proc := range rec.AuraProcs {
		checks = append(checks, checkAuraProc(rec, proc)...)
	}
	for _, proc := range rec.ItemProcs {
		checks = append(checks, checkItemProc(rec, proc)...)
	}
	return checks
}

// checkAuraProc compares one aura's generated spell_proc entry and the chances it
// gives. Auras outside the generated tables are skipped: they cover the spells the
// sim names, not everything a character happens to carry.
func checkAuraProc(rec record, proc auraProc) []Check {
	gen := serverdata.ProcBySpellID(proc.ID)
	if gen == nil {
		return nil
	}
	server := proc.ProcEntry
	label := fmt.Sprintf("proc %d (%s)", proc.ID, proc.Name)

	checks := []Check{
		checkFloat(label+" ppm", float64(gen.ProcsPerMinute), server.ProcsPerMinute),
		checkFloat(label+" chance", float64(gen.Chance), server.Chance),
		checkInt(label+" icd", int64(gen.CooldownMs), server.CooldownMs),
		checkInt(label+" charges", int64(gen.Charges), int64(server.Charges)),
		checkInt(label+" spell family", int64(gen.SpellFamilyName), int64(server.SpellFamilyName)),
		checkHex(label+" attributes", uint64(gen.AttributesMask), uint64(server.AttributesMask)),
		checkHex(label+" proc flags", uint64(gen.ProcFlags), uint64(server.ProcFlags)),
		checkHex(label+" spell types", uint64(gen.SpellTypeMask), uint64(server.SpellTypeMask)),
		checkHex(label+" spell phases", uint64(gen.SpellPhaseMask), uint64(server.SpellPhaseMask)),
		checkHex(label+" hit mask", uint64(gen.HitMask), uint64(server.HitMask)),
		checkHex(label+" school mask", uint64(gen.SchoolMask), uint64(server.SchoolMask)),
		checkHex(label+" disabled effects", uint64(gen.DisableEffectsMask), uint64(server.DisableEffectsMask)),
	}
	for i := range gen.SpellFamilyMask {
		checks = append(checks, checkHex(fmt.Sprintf("%s family mask %d", label, i),
			uint64(gen.SpellFamilyMask[i]), uint64(server.SpellFamilyMask[i])))
	}

	sim := core.ServerProcFor(proc.ID)
	for _, attack := range procAttackTypes(proc.Chance) {
		basis, ok := ppmBasisMs(rec, attack)
		if !ok {
			continue
		}
		want := sim.Chance * 100
		if sim.PPM > 0 {
			want = float64(basis) * sim.PPM / 600
		}
		checks = append(checks, checkPct(label+" "+attack, want, proc.Chance[attack]))
	}
	return checks
}

// checkItemProc compares a chance-on-hit item spell or weapon enchant. The server
// takes its PPM off the item or the spell_enchant_proc_data row rather than a
// spell_proc entry, and measures it against the weapon that swung.
func checkItemProc(rec record, proc itemProc) []Check {
	var checks []Check
	label := fmt.Sprintf("%s proc %d (%s)", proc.Source, proc.SpellID, proc.Name)

	ppm := proc.PPM
	if proc.Source == "enchant" {
		gen := serverdata.EnchantProcByID(proc.EnchantID)
		if gen == nil {
			return nil
		}
		checks = append(checks,
			checkFloat(label+" enchant ppm", float64(gen.PPM), enchantPPM(proc)),
			checkInt(label+" enchant custom chance", int64(gen.CustomChance), int64(enchantCustomChance(proc))),
			checkHex(label+" enchant attributes", uint64(gen.AttributeMask), uint64(enchantAttributes(proc))),
		)
		ppm = float64(gen.PPM)
	}
	if ppm == 0 {
		// A flat chance-on-hit spell carries no PPM anywhere, so there is nothing
		// of the sim's to compare: the chance is Spell.dbc's, which the capture
		// already holds.
		return checks
	}

	for _, attack := range procAttackTypes(proc.Chance) {
		basis, ok := ppmBasisMs(rec, attack)
		if !ok || attack == "spell" {
			// Player::CastItemCombatSpell always measures against the weapon that
			// swung, so the record has no per-spell chance for these.
			continue
		}
		checks = append(checks, checkPct(label+" "+attack, float64(basis)*ppm/600, proc.Chance[attack]))
	}
	return checks
}

func enchantPPM(proc itemProc) float64 {
	if proc.EnchantProc == nil {
		return 0
	}
	return proc.EnchantProc.PPM
}

func enchantCustomChance(proc itemProc) int32 {
	if proc.EnchantProc == nil {
		return 0
	}
	return proc.EnchantProc.CustomChance
}

func enchantAttributes(proc itemProc) uint32 {
	if proc.EnchantProc == nil {
		return 0
	}
	return proc.EnchantProc.AttributeMask
}

// procAttackTypes keeps the record's chance keys in a fixed order, so a failure
// list reads the same way every run.
func procAttackTypes(chance map[string]float64) []string {
	var types []string
	for _, name := range []string{"mainhand", "offhand", "ranged", "spell"} {
		if _, ok := chance[name]; ok {
			types = append(types, name)
		}
	}
	return types
}

// ppmBasisMs is the attackSpeed the server fed GetPPMProcChance: the unhasted
// weapon time for a white swing, and for "spell" whatever Aura::CalcProcChance
// picked for the record's own spell.
func ppmBasisMs(rec record, attack string) (int32, bool) {
	if attack != "spell" {
		ms := rec.Attacker.attack(attack).UnhastedMs
		return ms, ms > 0
	}

	spell := serverdata.SpellByID(rec.ProcSpellID)
	if spell == nil {
		return 0, false
	}
	if !usesWeaponSpeedForPPM(spell) {
		return max(spell.BaseCastMs, spellPPMFloorMs), true
	}
	ms := rec.Attacker.attack(spellAttackType(spell)).UnhastedMs
	return ms, ms > 0
}

// usesWeaponSpeedForPPM mirrors sim/core/ppm.go: SPELL_DAMAGE_CLASS_MELEE or
// SpellInfo::IsRangedWeaponSpell measure PPM against the weapon.
func usesWeaponSpeedForPPM(spell *serverdata.Spell) bool {
	const hunterFamily, hunterNonRangedFlag = 9, 0x10000000
	return spell.DmgClass == serverdata.DmgClassMelee ||
		spell.Flags&serverdata.FlagUsesRangedSlot != 0 ||
		(spell.Family == hunterFamily && spell.FamilyFlags[1]&hunterNonRangedFlag == 0)
}

// spellAttackType is SimValidation::SpellAttackType: which weapon the server reads
// for a spell. The generated data has no off-hand requirement attribute, and no
// spell the sim registers needs one.
func spellAttackType(spell *serverdata.Spell) string {
	switch {
	case spell.DmgClass == serverdata.DmgClassRanged && spell.Flags&serverdata.FlagUsesRangedSlot != 0:
		return "ranged"
	case spell.DmgClass != serverdata.DmgClassMelee && spell.Flags&serverdata.FlagAutoRepeat != 0:
		return "ranged"
	default:
		return "mainhand"
	}
}

func checkPct(name string, got, want float64) Check {
	detail := fmt.Sprintf("%.3f%%, server %.3f%%", got, want)
	if abs(got-want) <= maxChancePct {
		return pass(name, detail)
	}
	return fail(name, detail)
}

func checkFloat(name string, got, want float64) Check {
	// The generated value is a float32 out of the DB, so allow the decimal it
	// prints as to differ in the last place from the module's double.
	detail := fmt.Sprintf("%g, server %g", got, want)
	if abs(got-want) <= 1e-4 {
		return pass(name, detail)
	}
	return fail(name, detail)
}

func checkInt(name string, got, want int64) Check {
	detail := fmt.Sprintf("%d, server %d", got, want)
	if got == want {
		return pass(name, detail)
	}
	return fail(name, detail)
}

func checkHex(name string, got, want uint64) Check {
	detail := fmt.Sprintf("0x%x, server 0x%x", got, want)
	if got == want {
		return pass(name, detail)
	}
	return fail(name, detail)
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
