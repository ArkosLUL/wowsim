package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

// The talents whose rank spells carry a school cost aura, SPELL_AURA_MOD_POWER_COST_SCHOOL_PCT (72) or
// _SCHOOL (73), which the spell-mod sweep's list leaves out: SpellInfo::CalcPowerCost takes them by the
// spell's first school, not its class mask. mod-sim-validation's TestSimvalCostAuraSweep walks the talent
// paths with costAuraSpellsFile as its list, the spell-mod sweep's probes and SPELLMOD lines, collected in
// costAuraFixture. TestCostAuraRecords replays those lines as TestSpellModRecords does.
const (
	costAuraFixture    = "costauras.jsonl"
	costAuraSpellsFile = "costaura_spells.json"

	auraModPowerCostSchoolPct = 72
	auraModPowerCostSchool    = 73
)

// Power Infusion is a cast talent, so the sweep's after probes have it up. The sim's priest only registers its
// aura with a target, and no other sweep casts it.
func init() {
	sweepBuffOptions[10060] = func(p *proto.Player) {
		self := &proto.UnitReference{Type: proto.UnitReference_Self}
		switch spec := p.Spec.(type) {
		case *proto.Player_HealingPriest:
			spec.HealingPriest.Options.PowerInfusionTarget = self
		case *proto.Player_ShadowPriest:
			spec.ShadowPriest.Options.PowerInfusionTarget = self
		}
	}
}

// SpellFamilyName of each class, by implemented_talents.json's class name.
var classFamilies = map[string]int64{
	"warrior": 4, "paladin": 10, "hunter": 9, "rogue": 8, "priest": 6, "deathknight": 15, "shaman": 11,
	"mage": 3, "warlock": 5, "druid": 7,
}

// costAuraList has spellmod_spells.json's shape, so both the e2e's sweep and readSpellModList read it.
type costAuraList struct {
	Talents []costAuraTalent `json:"talents"`
}

type costAuraTalent struct {
	Class   string          `json:"class"`
	Tree    string          `json:"tree"`
	Talent  string          `json:"talent"`
	Passive bool            `json:"passive"`
	Groups  []costAuraGroup `json:"groups"`
}

// costAuraGroup is the ranks with the same cost auras, values aside, and the spells those change.
type costAuraGroup struct {
	Ranks  []int32         `json:"ranks"`
	Mods   []costAuraMod   `json:"mods"`
	Spells []costAuraSpell `json:"spells"`
}

type costAuraMod struct {
	Effect     int64 `json:"effect"`
	Aura       int64 `json:"aura"`
	SchoolMask int64 `json:"schoolMask"`
}

type costAuraSpell struct {
	ID   int32    `json:"id"`
	Name string   `json:"name"`
	Ops  []string `json:"ops"`
}

func costAuraMods(s *spellset.DumpSpell) []costAuraMod {
	var mods []costAuraMod
	for _, e := range s.Effects {
		if e.Aura == auraModPowerCostSchoolPct || e.Aura == auraModPowerCostSchool {
			mods = append(mods, costAuraMod{Effect: e.Index, Aura: e.Aura, SchoolMask: e.MiscValue})
		}
	}
	return mods
}

// firstSchoolMask is GetFirstSchoolInMask's school as a mask: the one whose cost auras a spell takes.
func firstSchoolMask(mask int64) int64 {
	return mask & -mask
}

// costedBy is whether a cast of the spell pays a cost the mod changes: a cast of the class's own family with
// a power cost, its first school in the mod's mask.
func (m costAuraMod) costedBy(s *spellset.DumpSpell, family int64) bool {
	return s.Family == family && !s.Passive && (s.ManaCost > 0 || s.ManaCostPercentage > 0) &&
		firstSchoolMask(s.SchoolMask)&m.SchoolMask != 0
}

// classNamedFor is tools/acore/spellmods' rule: the sim names the spell in sim/<class>/ or outside every
// class package.
func classNamedFor(class string, refs []spellset.Ref) bool {
	for _, r := range refs {
		parts := strings.SplitN(r.File, "/", 3)
		if len(parts) < 3 || parts[1] == class || classFamilies[parts[1]] == 0 {
			return true
		}
	}
	return false
}

// buildCostAuraList finds every implemented talent whose rank spells carry a school cost aura, and the
// spells the sim names for its class whose cost those change.
func buildCostAuraList(dump spellset.Dump, talents []implementedTalent, named map[int32][]spellset.Ref) (costAuraList, error) {
	var ids []int32
	for id := range named {
		if dump[id] != nil {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	list := costAuraList{Talents: []costAuraTalent{}}
	for _, t := range talents {
		family, ok := classFamilies[t.Class]
		if !ok {
			return list, fmt.Errorf("%s %s: unknown class", t.Class, t.FieldName)
		}
		entry := costAuraTalent{Class: t.Class, Tree: t.Tree, Talent: t.FieldName}
		hasMods := false
		for i, id := range t.RankSpells {
			s := dump[id]
			if s == nil {
				return list, fmt.Errorf("%s %s rank %d: spell %d isn't in the capture", t.Class, t.FieldName, i+1, id)
			}
			mods := costAuraMods(s)
			if len(mods) > 0 {
				hasMods = true
			}
			if i == 0 {
				entry.Passive = s.Passive
			}
			g := slices.IndexFunc(entry.Groups, func(g costAuraGroup) bool { return slices.Equal(g.Mods, mods) })
			if g < 0 {
				entry.Groups = append(entry.Groups, costAuraGroup{Mods: mods, Spells: []costAuraSpell{}})
				g = len(entry.Groups) - 1
			}
			entry.Groups[g].Ranks = append(entry.Groups[g].Ranks, id)
		}
		if !hasMods {
			continue
		}
		for g := range entry.Groups {
			group := &entry.Groups[g]
			for _, id := range ids {
				s := dump[id]
				if !classNamedFor(t.Class, named[id]) ||
					!slices.ContainsFunc(group.Mods, func(m costAuraMod) bool { return m.costedBy(s, family) }) {
					continue
				}
				group.Spells = append(group.Spells, costAuraSpell{ID: id, Name: s.Name, Ops: []string{"COST"}})
			}
		}
		list.Talents = append(list.Talents, entry)
	}
	return list, nil
}

// marshalCostAuraList writes the list one talent, group and spell a line, as spellmod_spells.json is.
func marshalCostAuraList(list costAuraList) ([]byte, error) {
	var b bytes.Buffer
	line := func(v any) (string, error) {
		data, err := json.Marshal(v)
		return string(data), err
	}
	b.WriteString("{\"talents\": [\n")
	for i, t := range list.Talents {
		head, err := line(struct {
			Class   string `json:"class"`
			Tree    string `json:"tree"`
			Talent  string `json:"talent"`
			Passive bool   `json:"passive"`
		}{t.Class, t.Tree, t.Talent, t.Passive})
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "%s,\"groups\": [\n", strings.TrimSuffix(head, "}"))
		for j, g := range t.Groups {
			ranks, err := line(g.Ranks)
			if err != nil {
				return nil, err
			}
			mods, err := line(g.Mods)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&b, " {\"ranks\":%s,\"mods\":%s,\n  \"spells\": [", ranks, mods)
			for k, s := range g.Spells {
				spell, err := line(s)
				if err != nil {
					return nil, err
				}
				b.WriteString("\n    " + spell)
				if k < len(g.Spells)-1 {
					b.WriteString(",")
				}
			}
			if len(g.Spells) > 0 {
				b.WriteString("\n  ")
			}
			b.WriteString("]}")
			if j < len(t.Groups)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString("]}")
		if i < len(list.Talents)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("]}\n")
	return b.Bytes(), nil
}
