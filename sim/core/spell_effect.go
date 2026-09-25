package core

import (
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// SpellEffect declares the numbers a spell's damage or healing takes from one of its server effects,
// with the server's exact values (0.714, not 0.7143). RegisterSpell checks it against the server data:
// a value the server rules out is a conflict, and the server's value replaces it unless a KeepSim entry
// keeps it. The zero value declares nothing.
//
// These are the rank's values. Talents, glyphs, relics and set bonuses go beside them in SpellConfig.Mods,
// and Spell.Direct and Dot.Tick hold the result.
type SpellEffect struct {
	Effect int32 // server effect index, 0 to 2
	// the spell that has the effect, when it isn't the declaring spell: the server deals Death Coil's
	// damage as 47632, not as the 49895 cast
	FromSpellID int32

	Min, Max float64 // base roll at level 80, before combo points and spell mods
	SP       float64 // spell power coefficient, healing for a heal
	AP       float64 // attack power coefficient, ranged attack power for a ranged spell
	// weapon damage share: 1 for a plain weapon hit, 0 for a spell. It scales the flat roll too when the
	// server lists the flat effect first (Obliterate, not Devastate: Spell::EffectWeaponDmg).
	WeaponPct float64
}

// Roll is the base value, Min to Max. A fixed value takes no random number, so it leaves the rolls that
// follow it alone.
func (e *SpellEffect) Roll(sim *Simulation) float64 {
	if e.Min == e.Max {
		return e.Min
	}
	// sim.Roll written out, label included, to keep Roll under the inlining budget
	return e.Min + (e.Max-e.Min)*sim.RandomFloat("Damage Roll")
}

// Average is the mean of Roll, for expected damage.
func (e *SpellEffect) Average() float64 {
	return (e.Min + e.Max) / 2
}

// SpellModOp is a server SpellModOp (SpellDefines.h) that a declaration takes. The damage percents
// (SPELLMOD_DAMAGE, SPELLMOD_DOT) stay in DamageMultiplier.
type SpellModOp int32

const (
	SpellModEffect1         SpellModOp = 3  // effect 0's value
	SpellModAllEffects      SpellModOp = 8  // every effect's value, before its own op
	SpellModEffect2         SpellModOp = 12 // effect 1's value
	SpellModEffect3         SpellModOp = 23 // effect 2's value
	SpellModBonusMultiplier SpellModOp = 24 // the SP coefficient, in hundredths: Flat 5 is +0.05
)

// SpellMod is a talent's, glyph's, relic's or set bonus's modifier on a declared effect, with the server's
// value: SPELL_AURA_ADD_FLAT_MODIFIER's in Flat, ADD_PCT_MODIFIER's percent in Pct.
type SpellMod struct {
	Op   SpellModOp
	Flat int32
	Pct  int32
}

var effectModOps = [...]SpellModOp{SpellModEffect1, SpellModEffect2, SpellModEffect3}

// applyMods folds config.Mods into the declarations once the check has run on their rank values. A mod
// belongs to the spell, as on the server: each effect mod reaches the declaration of its effect, and op 24
// reaches every coefficient.
func (spell *Spell) applyMods(config *SpellConfig) {
	if len(config.Mods) == 0 {
		return
	}
	var covered [len(effectModOps)]bool
	var declared bool
	for _, e := range [...]*SpellEffect{&spell.Direct, &config.Dot.Tick, &config.Hot.Tick} {
		if *e != (SpellEffect{}) {
			declared = true
			*e = spell.withMods(*e, config.Mods, &covered)
		}
	}
	if !declared {
		panic(fmt.Sprintf("%s has spell mods but declares no effect", spell.ActionID))
	}
	for _, m := range config.Mods {
		switch m.Op {
		case SpellModBonusMultiplier, SpellModAllEffects:
		case SpellModEffect1, SpellModEffect2, SpellModEffect3:
			if i := effectOfModOp(m.Op); !covered[i] {
				panic(fmt.Sprintf("%s has a mod on effect %d, which none of its declarations covers", spell.ActionID, i))
			}
		default:
			panic(fmt.Sprintf("%s has a mod with op %d, which a declaration doesn't take", spell.ActionID, m.Op))
		}
	}
}

// withMods is e with the mods that reach it, the way the server applies them, and marks the effects it
// covers. Effect mods go on the value in SpellEffectInfo::CalcValue: on Min and Max for the declared
// effect, on WeaponPct for the weapon percent effect, which only the server data can place. Op 24 goes on
// SP in SpellDamageBonusDone, which skips it for a 0 coefficient. SpellHealingBonusDone doesn't skip, but
// no op-24 flat reaches a heal without one. No op touches AP.
func (spell *Spell) withMods(e SpellEffect, mods []SpellMod, covered *[len(effectModOps)]bool) SpellEffect {
	if e.Effect < 0 || int(e.Effect) >= len(effectModOps) {
		panic(fmt.Sprintf("%s declares effect %d; a spell has effects 0 to %d", spell.ActionID, e.Effect, len(effectModOps)-1))
	}
	s := spell.serverSpell
	if e.FromSpellID != 0 {
		s = serverdata.SpellByID(e.FromSpellID)
	}
	pctEffect := int32(-1)
	if s != nil {
		for i := range s.Effects {
			if s.Effects[i].Effect == effectWeaponPercentDamage {
				pctEffect = int32(i)
			}
		}
	}
	valueEffect := e.Effect
	if valueEffect == pctEffect {
		valueEffect = -1 // a percent effect rolls no Min or Max
	}

	if valueEffect >= 0 {
		covered[valueEffect] = true
		if hasEffectMods(mods, valueEffect) {
			e.Min = effectValue(e.Min, valueEffect, mods)
			e.Max = effectValue(e.Max, valueEffect, mods)
		}
	}
	if pctEffect >= 0 && e.WeaponPct != 0 {
		covered[pctEffect] = true
		if hasEffectMods(mods, pctEffect) {
			e.WeaponPct = effectValue(math.Round(e.WeaponPct*100), pctEffect, mods) / 100
		}
	}
	var pct, flat float64
	for _, m := range mods {
		if m.Op == SpellModBonusMultiplier {
			pct += float64(m.Pct)
			flat += float64(m.Flat)
		}
	}
	if e.SP != 0 && (pct != 0 || flat != 0) {
		e.SP = (e.SP*100*(1+pct/100) + flat) / 100
	}
	return e
}

func effectOfModOp(op SpellModOp) int32 {
	for i, o := range effectModOps {
		if o == op {
			return int32(i)
		}
	}
	return -1
}

// hasEffectMods leaves out zero mods, a talent's untaken ranks, so they don't truncate a KeepSim value.
func hasEffectMods(mods []SpellMod, effect int32) bool {
	for _, m := range mods {
		if (m.Op == SpellModAllEffects || m.Op == effectModOps[effect]) && m != (SpellMod{Op: m.Op}) {
			return true
		}
	}
	return false
}

// effectValue is Unit::ApplyEffectModifiers on a value, then CalcValue's truncation to int32.
func effectValue(v float64, effect int32, mods []SpellMod) float64 {
	f := applySpellMod(float32(v), SpellModAllEffects, mods)
	f = applySpellMod(f, effectModOps[effect], mods)
	return float64(int32(f))
}

// applySpellMod is Player::ApplySpellMod for an op other than cast time, duration and the damage
// percents: value × (1 + the percents) + the flats, in the server's float32. Percents skip a 0 value.
func applySpellMod(v float32, op SpellModOp, mods []SpellMod) float32 {
	mul, flat := float32(1), int32(0)
	for _, m := range mods {
		if m.Op != op {
			continue
		}
		flat += m.Flat
		if m.Pct != 0 && v != 0 && mul != 0 {
			mul += float32(m.Pct) / 100
		}
	}
	// the conversion keeps the compiler from fusing a multiply-add the server doesn't do
	return float32(v*mul) + float32(flat)
}

// effectFields are the conflict fields of one declaration slot.
type effectFields struct {
	min, max, sp, ap, weaponPct ServerField
}

var (
	directEffectFields = effectFields{ServerMin, ServerMax, ServerSP, ServerAP, ServerWeaponPct}
	tickEffectFields   = effectFields{ServerTickMin, ServerTickMax, ServerTickSP, ServerTickAP, ServerTickWeaponPct}
)

// checkEffect checks a declaration against its server effect. own is the spell's server data, nil when
// none applies, in which case only a declaration naming FromSpellID is checked.
func (spell *Spell) checkEffect(e *SpellEffect, own *serverdata.Spell, f effectFields) {
	if *e == (SpellEffect{}) {
		return
	}
	s := own
	if e.FromSpellID != 0 {
		// a mistyped id would otherwise leave the declaration unchecked
		if s = serverdata.SpellByID(e.FromSpellID); s == nil {
			panic(fmt.Sprintf("%s declares FromSpellID %d, which has no server data", spell.ActionID, e.FromSpellID))
		}
	}
	if s == nil {
		return
	}
	if e.Effect < 0 || int(e.Effect) >= len(s.Effects) {
		panic(fmt.Sprintf("%s declares effect %d; a spell has effects 0 to %d", spell.ActionID, e.Effect, len(s.Effects)-1))
	}

	want := serverEffectOf(s, serverdata.BonusBySpellID(s.ID), int(e.Effect))
	spell.syncDeclared(f.min, &e.Min, want.min, e.Min == want.min)
	spell.syncDeclared(f.max, &e.Max, want.max, e.Max == want.max)
	spell.syncDeclared(f.sp, &e.SP, serverFloat(want.sp), float32(e.SP) == want.sp)
	spell.syncDeclared(f.ap, &e.AP, serverFloat(want.ap), float32(e.AP) == want.ap)
	spell.syncDeclared(f.weaponPct, &e.WeaponPct, want.weaponPct, float32(e.WeaponPct) == float32(want.weaponPct))
}

// syncDeclared takes the server's value unless the declared one matches it or an entry keeps it.
func (spell *Spell) syncDeclared(field ServerField, value *float64, server float64, same bool) {
	if same {
		return
	}
	if spell.recordServerFloatConflict(field, *value, server) {
		*value = server
	}
}

// SpellEffects and AuraType values (SharedDefines.h, SpellAuraDefines.h)
const (
	effectSchoolDamage         = 2
	effectHealthLeech          = 9
	effectHeal                 = 10
	effectWeaponDamageNoSchool = 17
	effectWeaponPercentDamage  = 31
	effectWeaponDamage         = 58
	effectNormalizedWeaponDmg  = 121

	auraTypePeriodicHeal  = 8
	auraTypePeriodicLeech = 53
)

func isWeaponEffect(effect int32) bool {
	switch effect {
	case effectWeaponDamageNoSchool, effectWeaponPercentDamage, effectWeaponDamage, effectNormalizedWeaponDmg:
		return true
	}
	return false
}

// dealsDamageOrHeals reports whether a server effect is one a declaration covers.
func dealsDamageOrHeals(e *serverdata.Effect) bool {
	switch e.Effect {
	case effectSchoolDamage, effectHealthLeech, effectHeal:
		return true
	}
	switch e.Aura {
	case auraTypePeriodicDamage, auraTypePeriodicHeal, auraTypePeriodicLeech:
		return true
	}
	return isWeaponEffect(e.Effect)
}

// firstWeaponEffect is the index a declaration on any of s's weapon effects covers: Spell::EffectWeaponDmg
// deals them together. -1 when s has none.
func firstWeaponEffect(s *serverdata.Spell) int32 {
	for i := range s.Effects {
		if isWeaponEffect(s.Effects[i].Effect) {
			return int32(i)
		}
	}
	return -1
}

// EffectRef is effect Effect of server spell SpellID. A weapon effect goes by the spell's first one.
type EffectRef struct {
	SpellID int32
	Effect  int32
}

func effectRefOf(s *serverdata.Spell, effect int32) EffectRef {
	if effect >= 0 && int(effect) < len(s.Effects) && isWeaponEffect(s.Effects[effect].Effect) {
		effect = firstWeaponEffect(s)
	}
	return EffectRef{SpellID: s.ID, Effect: effect}
}

// DeclaredEffects are the server effects the spell's declarations cover: Direct and the ticks of its Dot
// and Hot.
func (spell *Spell) DeclaredEffects() []EffectRef {
	decls := []SpellEffect{spell.Direct}
	if spell.aoeDot != nil {
		decls = append(decls, spell.aoeDot.Tick)
	}
	for _, dot := range spell.dots {
		if dot != nil {
			decls = append(decls, dot.Tick)
		}
	}
	var refs []EffectRef
	for _, e := range decls {
		if e == (SpellEffect{}) {
			continue
		}
		s := spell.serverSpell
		if e.FromSpellID != 0 {
			s = serverdata.SpellByID(e.FromSpellID)
		}
		if s == nil {
			continue
		}
		if ref := effectRefOf(s, e.Effect); !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	return refs
}

// DeclarableEffects are the effects of s that deal damage or heal, each with the declaration that matches
// the server: its level 80 values before mods, FromSpellID naming s. The weapon effects take one
// declaration between them, on the flat one where there is one. An effect whose values are all 0 has
// nothing to declare.
func DeclarableEffects(s *serverdata.Spell) map[EffectRef]SpellEffect {
	b := serverdata.BonusBySpellID(s.ID)
	decls := map[EffectRef]SpellEffect{}
	for i := range s.Effects {
		if !dealsDamageOrHeals(&s.Effects[i]) {
			continue
		}
		effect := i
		if isWeaponEffect(s.Effects[i].Effect) {
			if int32(i) != firstWeaponEffect(s) {
				continue
			}
			for j := range s.Effects {
				if isWeaponEffect(s.Effects[j].Effect) && s.Effects[j].Effect != effectWeaponPercentDamage {
					effect = j
					break
				}
			}
		}
		v := serverEffectOf(s, b, effect)
		if v == (serverEffect{}) {
			continue
		}
		decls[effectRefOf(s, int32(effect))] = SpellEffect{Effect: int32(effect), FromSpellID: s.ID, Min: v.min, Max: v.max,
			SP: serverFloat(v.sp), AP: serverFloat(v.ap), WeaponPct: v.weaponPct}
	}
	return decls
}

// serverEffect is what the server makes of one effect at level 80.
type serverEffect struct {
	min, max  float64
	sp, ap    float32
	weaponPct float64
}

// serverEffectOf follows the server's damage and healing paths. Spell::EffectWeaponDmg deals the weapon
// effects together through the melee bonuses, which read no coefficient: the percent effects multiply
// the weapon damage and the others add their roll. Every other effect goes through
// SpellDamageBonusDone or SpellHealingBonusDone, which take s's spell_bonus_data row b (its dot columns
// for a periodic aura, and AP only above 0), else the effect's BonusMultiplier, which a DmgClassNone
// spell doesn't get.
func serverEffectOf(s *serverdata.Spell, b *serverdata.Bonus, i int) (v serverEffect) {
	e := &s.Effects[i]
	if e.Effect != effectWeaponPercentDamage {
		lo, hi := s.EffectRange(i)
		v.min, v.max = float64(lo), float64(hi)
	}
	if isWeaponEffect(e.Effect) {
		v.weaponPct = 1
		for j := range s.Effects {
			if s.Effects[j].Effect == effectWeaponPercentDamage {
				pct, _ := s.EffectRange(j)
				v.weaponPct *= float64(pct) / 100
			}
		}
		return v
	}
	periodic := e.Aura == auraTypePeriodicDamage || e.Aura == auraTypePeriodicHeal || e.Aura == auraTypePeriodicLeech
	if b != nil {
		if periodic {
			v.sp, v.ap = b.Dot, max(b.APDot, 0)
		} else {
			v.sp, v.ap = b.Direct, max(b.AP, 0)
		}
	} else if s.DmgClass != serverdata.DmgClassNone {
		v.sp = e.BonusMultiplier
	}
	return v
}

// serverFloat is a server float32 as the decimal the tables write, 0.857 rather than 0.8569999933242798,
// so it equals the same literal typed in Go.
func serverFloat(f float32) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(float64(f), 'g', -1, 32), 64)
	return v
}

// SpellSchoolFromServerMask converts a server SpellSchoolMask (SharedDefines.h: Physical 1, Holy 2,
// Fire 4, Nature 8, Frost 16, Shadow 32, Arcane 64) to a SpellSchool.
func SpellSchoolFromServerMask(mask uint8) SpellSchool {
	var school SpellSchool
	for bit, s := range serverSchools {
		if mask&(1<<bit) != 0 {
			school |= s
		}
	}
	return school
}

// ServerSchoolMask converts a SpellSchool to the server's SpellSchoolMask.
func (ss SpellSchool) ServerSchoolMask() uint8 {
	var mask uint8
	for bit, s := range serverSchools {
		if ss.Matches(s) {
			mask |= 1 << bit
		}
	}
	return mask
}

// by SpellSchools bit
var serverSchools = [...]SpellSchool{
	SpellSchoolPhysical, SpellSchoolHoly, SpellSchoolFire, SpellSchoolNature, SpellSchoolFrost, SpellSchoolShadow, SpellSchoolArcane,
}
