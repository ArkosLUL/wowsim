package azerothcore

import (
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// WriteBisLua writes the dataset decoded the way the BisTooltipAC addon decodes it, for checking the
// addon's decoder against this one:
//
//   - Bistooltip_server_meta: the dataset's version, sim commit, catalog date, objective and
//     fingerprint.
//   - Bistooltip_server_bislists: spec subjects, in Bistooltip_wotlk_bislists's shape,
//     [class][spec][phase key][i] = {slot_name, enhs, [1..n] = item ids}, so the addon's lookups work
//     on it unchanged.
//   - Bistooltip_server_roster: roster subjects by character guid, each with name, class, spec,
//     raid_index and phases, where phases[phase key] is a list of slots like the above.
//
// Slots also get extra = {delta, reforge = {from, to}} when they have either. It's a table so the
// addon's reverse lookup, which compares every non-string key's value with an item id, can't mistake
// a delta for an item.
func WriteBisLua(w io.Writer, dataset *BisDataset) error {
	subjects := map[int]BisSubject{}
	for _, subject := range dataset.Subjects {
		subjects[subject.ID] = subject
	}
	phases := map[int][]BisDatasetBlock{}
	for _, block := range dataset.Blocks {
		phases[block.SubjectID] = append(phases[block.SubjectID], block)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Bistooltip_server_meta = {\n")
	for _, field := range [][2]string{
		{"version", dataset.Version}, {"sim_commit", dataset.SimCommit}, {"catalog_date", dataset.CatalogDate},
		{"objective", dataset.Objective}, {"fingerprint", dataset.Fingerprint},
	} {
		fmt.Fprintf(&sb, "    [%s] = %s,\n", luaString(field[0]), luaString(field[1]))
	}
	sb.WriteString("}\n")

	// spec subjects come sorted by class and spec, so each class and spec opens once
	sb.WriteString("Bistooltip_server_bislists = {\n")
	class, spec := "", ""
	for _, subject := range dataset.Subjects {
		if subject.Kind != BisSubjectSpec {
			continue
		}
		className := BisClassNames[subject.ClassID]
		if className != class {
			if class != "" {
				sb.WriteString("        },\n    },\n")
			}
			class, spec = className, ""
			fmt.Fprintf(&sb, "    [%s] = {\n", luaString(class))
		}
		if spec != "" {
			sb.WriteString("        },\n")
		}
		spec = subject.SpecName
		fmt.Fprintf(&sb, "        [%s] = {\n", luaString(spec))
		writeLuaPhases(&sb, "            ", subject, phases[subject.ID])
	}
	if class != "" {
		sb.WriteString("        },\n    },\n")
	}
	sb.WriteString("}\n")

	sb.WriteString("Bistooltip_server_roster = {\n")
	for _, subject := range dataset.Subjects {
		if subject.Kind != BisSubjectRoster {
			continue
		}
		fmt.Fprintf(&sb, "    [%d] = {\n", subject.GUID)
		fmt.Fprintf(&sb, "        [\"name\"] = %s,\n", luaString(subject.Name))
		fmt.Fprintf(&sb, "        [\"class\"] = %s,\n", luaString(BisClassNames[subject.ClassID]))
		fmt.Fprintf(&sb, "        [\"spec\"] = %s,\n", luaString(subject.SpecName))
		fmt.Fprintf(&sb, "        [\"raid_index\"] = %d,\n", subject.RaidIndex)
		sb.WriteString("        [\"phases\"] = {\n")
		writeLuaPhases(&sb, "            ", subject, phases[subject.ID])
		sb.WriteString("        },\n    },\n")
	}
	sb.WriteString("}\n")

	_, err := io.WriteString(w, sb.String())
	return err
}

func writeLuaPhases(sb *strings.Builder, indent string, subject BisSubject, blocks []BisDatasetBlock) {
	blocks = slices.Clone(blocks)
	slices.SortFunc(blocks, func(a, b BisDatasetBlock) int { return int(a.ContentPhase - b.ContentPhase) })
	for _, block := range blocks {
		fmt.Fprintf(sb, "%s[%s] = {\n", indent, luaString(BisPhaseKeys[block.ContentPhase]))
		for i, slot := range block.Block.Slots {
			fmt.Fprintf(sb, "%s    [%d] = %s,\n", indent, i+1, luaSlot(slot, subject.ClassID))
		}
		fmt.Fprintf(sb, "%s},\n", indent)
	}
}

// luaSlot lays enhs out like the addon's own lists: the enchant or a "none" placeholder, then the gems
// with a "none" between each two, and nothing at all when there's neither.
func luaSlot(slot BisSlot, classID int32) string {
	var enhs []string
	if slot.Enchant != 0 || len(slot.Gems) > 0 {
		if slot.Enchant != 0 {
			enhs = append(enhs, luaEnhancement("spell", slot.Enchant))
		} else {
			enhs = append(enhs, luaEnhancement("none", 0))
		}
		for i, gem := range slot.Gems {
			if i > 0 {
				enhs = append(enhs, luaEnhancement("none", 0))
			}
			enhs = append(enhs, luaEnhancement("item", gem))
		}
	}

	fields := []string{
		`["slot_name"] = ` + luaString(BisSlotName(slot.Slot, classID)),
		`["enhs"] = ` + luaList(enhs),
	}
	var extra []string
	if slot.HasDelta {
		extra = append(extra, `["delta"] = `+strconv.Itoa(int(slot.Delta)))
	}
	if slot.ReforgeFrom != 0 {
		extra = append(extra, fmt.Sprintf(`["reforge"] = { ["from"] = %d, ["to"] = %d }`, slot.ReforgeFrom, slot.ReforgeTo))
	}
	if len(extra) > 0 {
		fields = append(fields, `["extra"] = { `+strings.Join(extra, ", ")+" }")
	}
	for i, item := range slot.Items {
		fields = append(fields, fmt.Sprintf("[%d] = %d", i+1, item))
	}
	return "{ " + strings.Join(fields, ", ") + " }"
}

func luaEnhancement(kind string, id int32) string {
	return fmt.Sprintf(`{ ["type"] = "%s", ["id"] = %d }`, kind, id)
}

func luaList(values []string) string {
	if len(values) == 0 {
		return "{ }"
	}
	fields := make([]string, len(values))
	for i, value := range values {
		fields[i] = fmt.Sprintf("[%d] = %s", i+1, value)
	}
	return "{ " + strings.Join(fields, ", ") + " }"
}

// luaString quotes s for Lua 5.1, which has no \u escapes, so non-ASCII bytes go through raw.
func luaString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			sb.WriteByte('\\')
			sb.WriteByte(c)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&sb, "\\%03d", c)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}
