package main

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

type fieldDiff struct {
	field  string
	server string
	sim    string
}

type itemDiff struct {
	sim           *proto.UIItem
	server        *azerothcore.ConvertedItem
	diffs         []fieldDiff
	moduleFiles   []string
	unmappedStats []int32
	obtainable    bool
}

// category separates diffs worth acting on (classic) from those explained by the server itself:
// items nobody can get there, and items its module or custom SQL rewrote.
func (d itemDiff) category() string {
	switch {
	case !d.obtainable:
		return "unobtainable"
	case len(d.moduleFiles) > 0:
		return "module"
	}
	return "classic"
}

func statName(i int) string {
	return strings.TrimPrefix(proto.Stat(i).String(), "Stat")
}

func num(v float64) string {
	return fmt.Sprintf("%g", v)
}

func diffStats(prefix string, server, sim []float64) []fieldDiff {
	var diffs []fieldDiff
	for i := 0; i < len(database.Stats{}); i++ {
		var s, m float64
		if i < len(server) {
			s = server[i]
		}
		if i < len(sim) {
			m = sim[i]
		}
		if math.Abs(s-m) > 0.01 {
			diffs = append(diffs, fieldDiff{prefix + statName(i), num(s), num(m)})
		}
	}
	return diffs
}

func colorsString(colors []proto.GemColor) string {
	var parts []string
	for _, c := range colors {
		parts = append(parts, strings.TrimPrefix(c.String(), "GemColor"))
	}
	return strings.Join(parts, "+")
}

func classesString(classes []proto.Class) string {
	var parts []string
	for _, c := range classes {
		parts = append(parts, strings.TrimPrefix(c.String(), "Class"))
	}
	slices.Sort(parts)
	return strings.Join(parts, "+")
}

func compareItems(server, sim *proto.UIItem) []fieldDiff {
	var diffs []fieldDiff
	if server.Ilvl != sim.Ilvl {
		diffs = append(diffs, fieldDiff{"ilvl", fmt.Sprint(server.Ilvl), fmt.Sprint(sim.Ilvl)})
	}
	if server.Quality != sim.Quality {
		diffs = append(diffs, fieldDiff{"quality", server.Quality.String(), sim.Quality.String()})
	}
	diffs = append(diffs, diffStats("stat ", server.Stats, sim.Stats)...)

	if server.WeaponSpeed > 0 || sim.WeaponSpeed > 0 {
		// Server damage is a float; tooltips round it.
		if math.Abs(server.WeaponDamageMin-sim.WeaponDamageMin) > 1 {
			diffs = append(diffs, fieldDiff{"weapon damage min", num(server.WeaponDamageMin), num(sim.WeaponDamageMin)})
		}
		if math.Abs(server.WeaponDamageMax-sim.WeaponDamageMax) > 1 {
			diffs = append(diffs, fieldDiff{"weapon damage max", num(server.WeaponDamageMax), num(sim.WeaponDamageMax)})
		}
		if math.Abs(server.WeaponSpeed-sim.WeaponSpeed) > 0.005 {
			diffs = append(diffs, fieldDiff{"weapon speed", num(server.WeaponSpeed), num(sim.WeaponSpeed)})
		}
	}

	if s, m := colorsString(server.GemSockets), colorsString(sim.GemSockets); s != m {
		diffs = append(diffs, fieldDiff{"sockets", s, m})
	}
	diffs = append(diffs, diffStats("socket bonus ", server.SocketBonus, sim.SocketBonus)...)
	if server.Heroic != sim.Heroic {
		diffs = append(diffs, fieldDiff{"heroic", fmt.Sprint(server.Heroic), fmt.Sprint(sim.Heroic)})
	}
	if s, m := classesString(server.ClassAllowlist), classesString(sim.ClassAllowlist); s != m {
		diffs = append(diffs, fieldDiff{"class allowlist", s, m})
	}
	if server.SetName != sim.SetName {
		diffs = append(diffs, fieldDiff{"set name", server.SetName, sim.SetName})
	}
	return diffs
}

func (c *context) diffItems() (diffs []itemDiff, missing []*proto.UIItem, notComparable []itemDiff) {
	for _, id := range sortedKeys(c.sim.Items) {
		simItem := c.sim.Items[id]
		row := c.items[id]
		if row == nil {
			missing = append(missing, simItem)
			continue
		}
		converted := azerothcore.ConvertItem(row, c.dbc)
		d := itemDiff{
			sim:           simItem,
			server:        converted,
			diffs:         compareItems(converted.Item, simItem),
			obtainable:    len(c.obtainable[id]) > 0,
			moduleFiles:   c.modules.items[id],
			unmappedStats: converted.UnmappedStatTypes,
		}
		if converted.NotComparable != "" {
			notComparable = append(notComparable, d)
			continue
		}
		if len(d.diffs) > 0 {
			diffs = append(diffs, d)
		}
	}
	return diffs, missing, notComparable
}

func (d itemDiff) ilvlChanged() bool {
	return d.server.Item.Ilvl != d.sim.Ilvl
}

func (d itemDiff) diffString() string {
	var parts []string
	for _, f := range d.diffs {
		parts = append(parts, fmt.Sprintf("%s: %s -> %s", f.field, f.server, f.sim))
	}
	return strings.Join(parts, "; ")
}

// simSourceString summarizes where the sim (AtlasLoot/Wowhead Classic) says an item comes from.
func (c *context) simSourceString(item *proto.UIItem) string {
	var parts []string
	for _, src := range item.Sources {
		switch s := src.Source.(type) {
		case *proto.UIItemSource_Drop:
			part := fmt.Sprintf("drop %s %s", c.zoneName(s.Drop.ZoneId), difficultyName(s.Drop.Difficulty))
			if npc := c.sim.Npcs[s.Drop.NpcId]; npc != nil {
				part += " (" + npc.Name + ")"
			} else if s.Drop.OtherName != "" {
				part += " (" + s.Drop.OtherName + ")"
			}
			if s.Drop.Category != "" {
				part += " [" + s.Drop.Category + "]"
			}
			parts = append(parts, part)
		case *proto.UIItemSource_SoldBy:
			parts = append(parts, fmt.Sprintf("sold by %s (%d) in %s", s.SoldBy.NpcName, s.SoldBy.NpcId, c.zoneName(s.SoldBy.ZoneId)))
		case *proto.UIItemSource_Quest:
			parts = append(parts, fmt.Sprintf("quest %s (%d)", s.Quest.Name, s.Quest.Id))
		case *proto.UIItemSource_Crafted:
			parts = append(parts, fmt.Sprintf("crafted %s (spell %d)", s.Crafted.Profession, s.Crafted.SpellId))
		case *proto.UIItemSource_Rep:
			parts = append(parts, "reputation")
		}
	}
	return strings.Join(slices.Compact(parts), " | ")
}

// primarySource picks the first drop (else vendor) zone and difficulty, for grouping.
func (c *context) primarySource(item *proto.UIItem) (zone, difficulty string) {
	for _, src := range item.Sources {
		if drop := src.GetDrop(); drop != nil {
			return c.zoneName(drop.ZoneId), difficultyName(drop.Difficulty)
		}
	}
	for _, src := range item.Sources {
		if soldBy := src.GetSoldBy(); soldBy != nil {
			return "vendor: " + soldBy.NpcName, ""
		}
	}
	if len(item.Sources) > 0 {
		return "other", ""
	}
	return "no source", ""
}

func (c *context) zoneName(id int32) string {
	if zone := c.sim.Zones[id]; zone != nil {
		return zone.Name
	}
	if id == 0 {
		return "unknown zone"
	}
	return fmt.Sprintf("zone %d", id)
}

func difficultyName(d proto.DungeonDifficulty) string {
	if d == proto.DungeonDifficulty_DifficultyUnknown {
		return ""
	}
	return strings.TrimPrefix(d.String(), "Difficulty")
}

// Equippable inventory types, excluding shirts, bags, tabards, ammo and quivers.
func isEquippableSlot(inventoryType int32) bool {
	switch inventoryType {
	case 0, 4, 18, 19, 24, 27:
		return false
	}
	return inventoryType > 0 && inventoryType <= 28
}

type obtainabilityRow struct {
	id      int32
	name    string
	ilvl    int32
	detail  string
	sources string
}

func (c *context) diffObtainability() (unobtainable, missingInSim []obtainabilityRow) {
	for _, id := range sortedKeys(c.sim.Items) {
		item := c.sim.Items[id]
		if c.items[id] == nil || len(c.obtainable[id]) > 0 {
			continue
		}
		unobtainable = append(unobtainable, obtainabilityRow{
			id: id, name: item.Name, ilvl: item.Ilvl, sources: c.simSourceString(item),
		})
	}

	for _, id := range sortedKeys(c.items) {
		row := c.items[id]
		if c.sim.Items[id] != nil || len(c.obtainable[id]) == 0 {
			continue
		}
		if row.Quality < 3 || row.ItemLevel < 187 || !isEquippableSlot(row.InventoryType) {
			continue
		}
		var tags []string
		if c.leftovers.Items[id] != nil {
			tags = append(tags, "in leftover_db (non-simmable)")
		}
		if _, ok := database.ItemDenyList[id]; ok {
			tags = append(tags, "ItemDenyList")
		}
		for _, pattern := range database.DenyListNameRegexes {
			if pattern.MatchString(row.Name) {
				tags = append(tags, "DenyListNameRegexes")
				break
			}
		}
		missingInSim = append(missingInSim, obtainabilityRow{
			id: id, name: row.Name, ilvl: row.ItemLevel,
			detail:  strings.Join(tags, ", "),
			sources: strings.Join(c.obtainable[id], ", "),
		})
	}
	return unobtainable, missingInSim
}
