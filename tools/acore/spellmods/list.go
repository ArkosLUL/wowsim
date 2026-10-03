package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

// implementedTalent is one line of implemented_talents.json, the talent guard's export.
type implementedTalent struct {
	Class      string  `json:"class"`
	Tree       string  `json:"tree"`
	FieldName  string  `json:"fieldName"`
	RankSpells []int32 `json:"rankSpells"`
}

type spellModList struct {
	Talents []talentMods `json:"talents"`
}

// talentMods is a talent whose rank spells carry spell mods.
type talentMods struct {
	Class  string `json:"class"`
	Tree   string `json:"tree"`
	Talent string `json:"talent"` // the tree file's fieldName, as talent_paths.json names it
	// false when the rank spell is a cast (Presence of Mind): its mods only apply while that aura is up
	Passive bool `json:"passive"`
	// the rank spell's EquippedItemClass and subclass mask: its aura only applies with such an item on
	Equipped *equippedItem `json:"equipped,omitempty"`
	Groups   []rankGroup   `json:"groups"`
}

type equippedItem struct {
	Class        int64 `json:"class"`
	SubClassMask int64 `json:"subClassMask"`
}

// rankGroup is the ranks of a talent with the same mods, values aside. Most talents have one; a few
// ranks carry another class mask or another op in the DBC.
type rankGroup struct {
	Ranks    []int32         `json:"ranks"` // rank spell ids
	Mods     []spellMod      `json:"mods"`
	Spells   []affectedSpell `json:"spells"`
	Excluded []excludedSpell `json:"excluded,omitempty"`
}

// affectedSpell is a spell the sim names that the group's mods change, with the ops that change it.
type affectedSpell struct {
	ID   int32    `json:"id"`
	Name string   `json:"name"`
	Ops  []string `json:"ops"`
}

// excludedSpell is a spell a mod's family and mask match that the server still leaves alone.
type excludedSpell struct {
	ID     int32  `json:"id"`
	Name   string `json:"name"`
	Op     string `json:"op"`
	Reason string `json:"reason"`
}

// listStats are the counts the generator prints.
type listStats struct {
	talents, byScript, multiGroup, active, equipped, noSpells int
	spells, pairs, excluded                                   int
	// rule matches the sim doesn't name at all, or names only in another class's package
	unnamed, otherClass int
	ops                 map[string]int
}

// namedFor tells whether the sim names a spell for a class: in sim/<class>/, or outside every class
// package (sim/core, sim/common). A spell another class's code names, like the Felguard's Cleave
// (warrior family) or the Fire Elemental's Fire Blast (mage family), never reaches this class's spell mods.
func namedFor(class string, refs []spellset.Ref, classes map[string]bool) bool {
	for _, r := range refs {
		parts := strings.SplitN(r.File, "/", 3)
		if len(parts) < 3 || parts[1] == class || !classes[parts[1]] {
			return true
		}
	}
	return false
}

// buildList applies the server's rule to every sim-named spell of the dump, for each implemented talent
// whose rank spells carry spell mods.
func buildList(dump spellset.Dump, talents []implementedTalent, named map[int32][]spellset.Ref) (*spellModList, listStats, error) {
	stats := listStats{ops: map[string]int{}}
	var ids []int32
	for id := range named {
		if dump[id] != nil {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	classes := map[string]bool{}
	for _, t := range talents {
		classes[t.Class] = true
	}
	allSpells := map[int32]bool{}
	unnamed := map[int32]bool{}
	otherClass := map[int32]bool{}

	list := &spellModList{}
	for _, t := range talents {
		entry := talentMods{Class: t.Class, Tree: t.Tree, Talent: t.FieldName}
		var groupMods [][]spellMod
		hasMods := false
		for i, id := range t.RankSpells {
			s := dump[id]
			if s == nil {
				return nil, stats, fmt.Errorf("%s %s rank %d: spell %d isn't in the capture", t.Class, t.FieldName, i+1, id)
			}
			mods, err := spellMods(s)
			if err != nil {
				return nil, stats, err
			}
			if len(mods) > 0 {
				hasMods = true
			} else {
				mods = []spellMod{}
			}
			if i == 0 {
				entry.Passive = s.Passive
				if s.EquippedItemClass >= 0 {
					entry.Equipped = &equippedItem{Class: s.EquippedItemClass, SubClassMask: s.EquippedItemSubClassMask}
				}
			} else if s.Passive != entry.Passive {
				return nil, stats, fmt.Errorf("%s %s: rank %d is passive=%t, rank 1 isn't", t.Class, t.FieldName, i+1, s.Passive)
			}
			g := slices.IndexFunc(groupMods, func(m []spellMod) bool { return sameMods(m, mods) })
			if g < 0 {
				groupMods = append(groupMods, mods)
				entry.Groups = append(entry.Groups, rankGroup{Mods: mods})
				g = len(entry.Groups) - 1
			}
			entry.Groups[g].Ranks = append(entry.Groups[g].Ranks, id)
		}
		if !hasMods {
			continue
		}

		stats.talents++
		if slices.ContainsFunc(entry.Groups, func(g rankGroup) bool {
			return slices.ContainsFunc(g.Mods, func(m spellMod) bool { return m.Script != "" })
		}) {
			stats.byScript++
		}
		if len(entry.Groups) > 1 {
			stats.multiGroup++
		}
		if !entry.Passive {
			stats.active++
		}
		if entry.Equipped != nil {
			stats.equipped++
		}
		talentSpells := 0
		for g := range entry.Groups {
			group := &entry.Groups[g]
			for _, id := range ids {
				s := dump[id]
				var ops []spellMod
				for _, m := range group.Mods {
					if !m.matches(s) {
						continue
					}
					if !namedFor(t.Class, named[id], classes) {
						if m.excludedBy(s) == "" {
							otherClass[id] = true
						}
						continue
					}
					if reason := m.excludedBy(s); reason != "" {
						group.Excluded = append(group.Excluded, excludedSpell{ID: id, Name: s.Name, Op: m.Op, Reason: reason})
						continue
					}
					ops = append(ops, m)
				}
				if len(ops) == 0 {
					continue
				}
				slices.SortFunc(ops, func(a, b spellMod) int { return cmp.Compare(a.op, b.op) })
				spell := affectedSpell{ID: id, Name: s.Name}
				for _, m := range ops {
					if !slices.Contains(spell.Ops, m.Op) {
						spell.Ops = append(spell.Ops, m.Op)
						stats.ops[m.Op]++
					}
				}
				group.Spells = append(group.Spells, spell)
				allSpells[id] = true
			}
			if group.Spells == nil {
				group.Spells = []affectedSpell{}
			}
			talentSpells += len(group.Spells)
			stats.pairs += len(group.Spells)
			stats.excluded += len(group.Excluded)
			for _, s := range dump {
				if named[int32(s.ID)] != nil {
					continue
				}
				for _, m := range group.Mods {
					if m.matches(s) && m.excludedBy(s) == "" {
						unnamed[int32(s.ID)] = true
						break
					}
				}
			}
		}
		if talentSpells == 0 {
			stats.noSpells++
		}
		list.Talents = append(list.Talents, entry)
	}
	stats.spells = len(allSpells)
	stats.unnamed = len(unnamed)
	stats.otherClass = len(otherClass)
	return list, stats, nil
}

// marshalList writes one line per talent head, group head and spell, so a regenerated file diffs by spell.
func marshalList(list *spellModList) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{\"talents\": [\n")
	for i, t := range list.Talents {
		groups := t.Groups
		t.Groups = nil
		head, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		b.Write(bytes.TrimSuffix(head, []byte(`"groups":null}`)))
		b.WriteString("\"groups\": [\n")
		for j, g := range groups {
			b.WriteString(" {")
			ranks, err := json.Marshal(g.Ranks)
			if err != nil {
				return nil, err
			}
			mods, err := json.Marshal(g.Mods)
			if err != nil {
				return nil, err
			}
			fmt.Fprintf(&b, "\"ranks\":%s,\"mods\":%s,\n  \"spells\": [", ranks, mods)
			if err := writeLines(&b, g.Spells); err != nil {
				return nil, err
			}
			b.WriteString("]")
			if len(g.Excluded) > 0 {
				b.WriteString(",\n  \"excluded\": [")
				if err := writeLines(&b, g.Excluded); err != nil {
					return nil, err
				}
				b.WriteString("]")
			}
			b.WriteString("}")
			if j < len(groups)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString("]}")
		if i < len(list.Talents)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("]}\n")
	return b.Bytes(), nil
}

func writeLines[T any](b *bytes.Buffer, items []T) error {
	if len(items) == 0 {
		return nil
	}
	b.WriteByte('\n')
	for i, item := range items {
		line, err := json.Marshal(item)
		if err != nil {
			return err
		}
		b.WriteString("    ")
		b.Write(line)
		if i < len(items)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("  ")
	return nil
}
