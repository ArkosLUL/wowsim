package main

import (
	"fmt"
	"slices"

	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

const (
	auraDummy           = 4
	auraAddFlatModifier = 107
	auraAddPctModifier  = 108

	spellModDuration = 1

	attr3IgnoreCasterModifiers = 0x20000000
)

// opNames are SpellModOp's names, SPELLMOD_ dropped.
var opNames = map[int64]string{
	0: "DAMAGE", 1: "DURATION", 2: "THREAT", 3: "EFFECT1", 4: "CHARGES", 5: "RANGE", 6: "RADIUS",
	7: "CRITICAL_CHANCE", 8: "ALL_EFFECTS", 9: "NOT_LOSE_CASTING_TIME", 10: "CASTING_TIME", 11: "COOLDOWN",
	12: "EFFECT2", 13: "IGNORE_ARMOR", 14: "COST", 15: "CRIT_DAMAGE_BONUS", 16: "RESIST_MISS_CHANCE",
	17: "JUMP_TARGETS", 18: "CHANCE_OF_SUCCESS", 19: "ACTIVATION_TIME", 20: "DAMAGE_MULTIPLIER",
	21: "GLOBAL_COOLDOWN", 22: "DOT", 23: "EFFECT3", 24: "BONUS_MULTIPLIER", 26: "PROC_PER_MINUTE",
	27: "VALUE_MULTIPLIER", 28: "RESIST_DISPEL_CHANCE", 29: "CRIT_DAMAGE_BONUS_2", 30: "SPELL_COST_REFUND_ON_FAIL",
}

// spellMod is one SpellModifier a talent's rank spell puts on the player.
type spellMod struct {
	Effect    int64    `json:"effect"`
	Type      string   `json:"type"` // flat (aura 107) or pct (aura 108)
	Op        string   `json:"op"`
	ClassMask [3]int64 `json:"classMask"`
	// the spell script that builds this mod, when no 107 or 108 aura does
	Script string `json:"script,omitempty"`

	op     int64
	family int64 // the rank spell's SpellFamilyName, which IsAffected matches against
}

// spellModScripts are the server's spell scripts that build or rewrite a SpellModifier
// (DoEffectCalcSpellMod), by script name. A nil entry isn't on any talent's rank spell, and the
// generator fails if one shows up there. TestSpellModScriptsMatchServer holds the keys to the server.
var spellModScripts = map[string]func(s *spellset.DumpSpell) []spellMod{
	// a flat mod per dummy effect, on the pet passives that pass on the owner's crit
	"spell_warl_improved_demonic_tactics": func(s *spellset.DumpSpell) []spellMod {
		var mods []spellMod
		for _, e := range s.Effects {
			if e.Aura == auraDummy {
				mods = append(mods, newSpellMod(s, e.Index, "flat", e.MiscValue, [3]int64{0, 0x2000, 0},
					"spell_warl_improved_demonic_tactics"))
			}
		}
		return mods
	},
	"spell_dk_killing_machine":                 nil, // on the proc aura 51124
	"spell_dru_idol_lifebloom":                 nil,
	"spell_pal_exorcism_and_holy_wrath_damage": nil,
	"spell_warr_glyph_of_sunder_armor":         nil,
}

func newSpellMod(s *spellset.DumpSpell, effect int64, typ string, op int64, mask [3]int64, script string) spellMod {
	name, ok := opNames[op]
	if !ok {
		name = fmt.Sprint(op)
	}
	return spellMod{Effect: effect, Type: typ, Op: name, ClassMask: mask, Script: script, op: op, family: s.Family}
}

// spellMods is AuraEffect::CalculateSpellMod for every effect of a rank spell, then its scripts.
func spellMods(s *spellset.DumpSpell) ([]spellMod, error) {
	var mods []spellMod
	for _, e := range s.Effects {
		switch e.Aura {
		case auraAddFlatModifier:
			mods = append(mods, newSpellMod(s, e.Index, "flat", e.MiscValue, e.ClassMask, ""))
		case auraAddPctModifier:
			mods = append(mods, newSpellMod(s, e.Index, "pct", e.MiscValue, e.ClassMask, ""))
		}
	}
	for _, name := range s.Scripts {
		build, known := spellModScripts[name]
		if !known {
			continue
		}
		if build == nil {
			return nil, fmt.Errorf("spell %d %s: script %s changes its spell mods and the generator doesn't model it",
				s.ID, s.Name, name)
		}
		mods = append(mods, build(s)...)
	}
	return mods, nil
}

// sameMods compares two ranks' mods, values aside.
func sameMods(a, b []spellMod) bool {
	return slices.EqualFunc(a, b, func(x, y spellMod) bool {
		return x.Effect == y.Effect && x.Type == y.Type && x.op == y.op && x.ClassMask == y.ClassMask &&
			x.Script == y.Script && x.family == y.family
	})
}

// matches is SpellInfo::IsAffected: the mod's family (its rank spell's) and class mask against the spell's.
// Family 0 matches every spell, and so does an empty mask within the family.
func (m spellMod) matches(s *spellset.DumpSpell) bool {
	if m.family == 0 {
		return true
	}
	if s.Family != m.family {
		return false
	}
	if m.ClassMask == [3]int64{} {
		return true
	}
	for i := range m.ClassMask {
		if m.ClassMask[i]&s.FamilyFlags[i] != 0 {
			return true
		}
	}
	return false
}

// excludedBy is what keeps a matching mod off the spell: Player::IsAffectedBySpellmod skips duration mods
// on infinite-duration spells, and SpellInfo::IsAffectedBySpellMod skips every other op on a spell that
// ignores caster modifiers. Empty when the mod applies. No server script hooks OnIsAffectedBySpellModCheck
// (TestSpellModScriptsMatchServer), so nothing forces a mod on.
func (m spellMod) excludedBy(s *spellset.DumpSpell) string {
	if m.op == spellModDuration {
		if s.DurationMs == -1 {
			return "infinite duration"
		}
		return ""
	}
	if s.Attributes[3]&attr3IgnoreCasterModifiers != 0 {
		return "SPELL_ATTR3_IGNORE_CASTER_MODIFIERS"
	}
	return ""
}
