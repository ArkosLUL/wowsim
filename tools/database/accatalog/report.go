package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

const maxExamples = 12

// phase is the content phase a tier falls in: everything up to 13 is phase 1.
func phase(tier int32) int32 {
	return max(tier-12, 1)
}

func classicTier(p int32) int32 {
	return 12 + max(p, 1)
}

func byID(catalog *proto.ServerCatalog) map[int32]*proto.CatalogItem {
	out := make(map[int32]*proto.CatalogItem, len(catalog.Items))
	for _, item := range catalog.Items {
		out[item.Id] = item
	}
	return out
}

func describe(item *proto.CatalogItem) string {
	flags := ""
	if item.FallbackTier {
		flags += " fallback"
	}
	if item.Pvp {
		flags += " pvp"
	}
	return fmt.Sprintf("%d %s (tier %d%s)", item.Id, item.Name, item.ProgressionTier, flags)
}

// printDiff compares the new catalog with the one it replaces.
func printDiff(w io.Writer, previous, catalog *proto.ServerCatalog) {
	fmt.Fprintln(w, "== Changes since the last catalog ==")
	if previous == nil {
		fmt.Fprintf(w, "no previous catalog; %d items, %d limit groups\n\n", len(catalog.Items), len(catalog.LimitGroups))
		return
	}
	old, cur := byID(previous), byID(catalog)
	var added, removed, retiered, pvp, resourced []string
	for _, id := range slices.Sorted(maps.Keys(cur)) {
		item := cur[id]
		before, ok := old[id]
		if !ok {
			added = append(added, describe(item))
			continue
		}
		// listed even when the tier moved too, since the tier line doesn't show the old flag
		pvpFlipped := before.Pvp != item.Pvp
		if pvpFlipped {
			pvp = append(pvp, describe(item))
		}
		switch {
		case before.ProgressionTier != item.ProgressionTier || before.FallbackTier != item.FallbackTier:
			retiered = append(retiered, fmt.Sprintf("%s, was %d", describe(item), before.ProgressionTier))
		case !pvpFlipped && len(before.Sources) != len(item.Sources):
			resourced = append(resourced, describe(item))
		}
	}
	for _, id := range slices.Sorted(maps.Keys(old)) {
		if _, ok := cur[id]; !ok {
			removed = append(removed, describe(old[id]))
		}
	}
	fmt.Fprintf(w, "date %s -> %s\n", previous.Date, catalog.Date)
	printList(w, "added", added)
	printList(w, "removed", removed)
	printList(w, "tier changed", retiered)
	printList(w, "pvp flag changed", pvp)
	printList(w, "source count changed", resourced)
	fmt.Fprintln(w)
}

func printList(w io.Writer, title string, lines []string) {
	fmt.Fprintf(w, "%s: %d\n", title, len(lines))
	for i, line := range lines {
		if i == maxExamples {
			fmt.Fprintf(w, "  ... %d more\n", len(lines)-maxExamples)
			break
		}
		fmt.Fprintf(w, "  %s\n", line)
	}
}

func histogram(w io.Writer, title string, counts map[int32]int) {
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%d: %d", k, counts[k]))
	}
	fmt.Fprintf(w, "  %s {%s}\n", title, strings.Join(parts, ", "))
}

func hasSource(item *proto.CatalogItem, match func(*proto.CatalogSource) bool) bool {
	return slices.ContainsFunc(item.Sources, match)
}

// printClassicReport compares catalog tiers with db.json's Classic phases and runs the spot checks
// from the BIS-catalog spec.
func printClassicReport(w io.Writer, catalog *proto.ServerCatalog, stats *azerothcore.CatalogStats, classic *database.WowDatabase) {
	cur := byID(catalog)
	phases := map[int32]int32{}
	names := map[int32]string{}
	for id, item := range classic.Items {
		phases[id], names[id] = item.Phase, item.Name
	}
	for id, gem := range classic.Gems {
		phases[id], names[id] = gem.Phase, gem.Name
	}
	unobtainable := map[int32]bool{}
	for _, id := range stats.Unobtainable {
		unobtainable[id] = true
	}

	fmt.Fprintln(w, "== Classic phases vs catalog tiers (db.json items and gems) ==")
	matrix := map[int32]map[string]int{}
	var dropped, unresolvedDropped []string
	for _, id := range slices.Sorted(maps.Keys(phases)) {
		p := max(phases[id], 1)
		if matrix[p] == nil {
			matrix[p] = map[string]int{}
		}
		item, ok := cur[id]
		switch {
		case ok:
			matrix[p][fmt.Sprint(phase(item.ProgressionTier))]++
		case unobtainable[id]:
			matrix[p]["dropped"]++
			dropped = append(dropped, fmt.Sprintf("%d %s (Classic phase %d)", id, names[id], phases[id]))
		default:
			matrix[p]["dropped"]++
			unresolvedDropped = append(unresolvedDropped, fmt.Sprintf("%d %s (Classic phase %d)", id, names[id], phases[id]))
		}
	}
	fmt.Fprintln(w, "Classic phase -> catalog phase (tier-12, 13 and below count as 1)")
	for _, p := range slices.Sorted(maps.Keys(matrix)) {
		var parts []string
		for _, col := range []string{"1", "2", "3", "4", "5", "dropped"} {
			if n := matrix[p][col]; n > 0 {
				parts = append(parts, fmt.Sprintf("%s: %d", col, n))
			}
		}
		fmt.Fprintf(w, "  %d -> {%s}\n", p, strings.Join(parts, ", "))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "== Expected moves ==")
	ulduar10 := func(s *proto.CatalogSource) bool {
		return s.MapId == 603 && (s.Kind == proto.CatalogSourceKind_CatalogSourceRaid10 || s.Kind == proto.CatalogSourceKind_CatalogSourceRaid10Heroic)
	}
	// vanilla Onyxia is "Onyxia's Lair 40"
	onyxia := func(s *proto.CatalogSource) bool {
		return s.MapId == 249 && (strings.HasPrefix(s.Name, "Onyxia's Lair 10") || strings.HasPrefix(s.Name, "Onyxia's Lair 25"))
	}
	moveCheck(w, "Ulduar 10 sources (expect tier 14)", catalog, phases, nil, ulduar10, 14)
	moveCheck(w, "Level-80 Onyxia sources (expect tier 15)", catalog, phases, nil, onyxia, 15)
	moveCheck(w, "Epic gems in db.json (expect tier 13)", catalog, phases, func(item *proto.CatalogItem) bool {
		gem := classic.Gems[item.Id]
		return item.IsGem && gem != nil && gem.Quality == proto.ItemQuality_ItemQualityEpic
	}, nil, 13)
	printList(w, "db.json items nothing on the server awards, dropped", dropped)
	printList(w, "db.json items whose sources don't resolve, dropped", unresolvedDropped)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "== Spot checks ==")
	spotT10(w, catalog, cur, stats)
	spotLimits(w, catalog, cur, []int32{50362, 50363}, "Deathbringer's Will N/H")
	spotToCFaction(w, catalog)
	var dragonsEyes []int32
	for _, item := range catalog.Items {
		if item.IsGem && strings.Contains(item.Name, "Dragon's Eye") {
			dragonsEyes = append(dragonsEyes, item.Id)
		}
	}
	spotLimits(w, catalog, cur, dragonsEyes, "Dragon's Eyes")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "== Emblem tiers ==")
	for _, e := range []struct {
		id   int32
		name string
	}{{40752, "Heroism"}, {40753, "Valor"}, {45624, "Conquest"}, {47241, "Triumph"}, {49426, "Frost"}} {
		fmt.Fprintf(w, "  %s (%d): %s\n", e.name, e.id, tierText(stats.ItemTiers, e.id))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "== Fallback and gaps ==")
	var fallback []string
	for _, id := range stats.Fallback {
		fallback = append(fallback, describe(cur[id]))
	}
	printList(w, "fallback (Classic phase) items", fallback)
	fmt.Fprintf(w, "unobtainable catalog-scope items left out: %d\n", len(stats.Unobtainable))
	fmt.Fprintf(w, "unresolved items without a Classic phase, left out: %d\n", len(stats.Unresolved))
	var holders []string
	for _, h := range stats.UnplacedHolders {
		kind := "creature"
		if h.GameObject {
			kind = "gameobject"
		}
		holders = append(holders, fmt.Sprintf("%s %d %s: %d items", kind, h.Entry, h.Name, h.Items))
	}
	const holdersTitle = "loot holders nothing places (rare+ ilvl 200+ items that resolve nowhere else)"
	if *allHolders {
		fmt.Fprintf(w, "%s: %d\n", holdersTitle, len(holders))
		for _, h := range holders {
			fmt.Fprintf(w, "  %s\n", h)
		}
		return
	}
	printList(w, holdersTitle, holders)
}

func tierText(tiers map[int32]int32, id int32) string {
	if t, ok := tiers[id]; ok {
		return fmt.Sprintf("tier %d", t)
	}
	return "unresolved"
}

// moveCheck reports on the items one of the spec's expected moves covers. With source set it checks
// those sources' own tiers, since an item can also drop somewhere earlier; otherwise the items'.
func moveCheck(w io.Writer, title string, catalog *proto.ServerCatalog, phases map[int32]int32,
	matchItem func(*proto.CatalogItem) bool, matchSource func(*proto.CatalogSource) bool, want int32) {
	itemTiers, sourceTiers, classicPhases := map[int32]int{}, map[int32]int{}, map[int32]int{}
	var off []string
	for _, item := range catalog.Items {
		if matchItem != nil && !matchItem(item) {
			continue
		}
		if matchSource != nil {
			matched := false
			for _, s := range item.Sources {
				if !matchSource(s) {
					continue
				}
				matched = true
				sourceTiers[s.ProgressionTier]++
				if s.ProgressionTier != want {
					off = append(off, fmt.Sprintf("%s: %s tier %d", describe(item), s.Name, s.ProgressionTier))
				}
			}
			if !matched {
				continue
			}
		} else if item.ProgressionTier != want {
			off = append(off, describe(item))
		}
		itemTiers[item.ProgressionTier]++
		if p, ok := phases[item.Id]; ok {
			classicPhases[p]++
		}
	}
	fmt.Fprintf(w, "%s: %d items\n", title, tierCount(itemTiers))
	if matchSource != nil {
		histogram(w, "source tiers", sourceTiers)
	}
	histogram(w, "item tiers", itemTiers)
	histogram(w, "Classic phases", classicPhases)
	printList(w, fmt.Sprintf("  not at tier %d", want), off)
}

func spotT10(w io.Writer, catalog *proto.ServerCatalog, cur map[int32]*proto.CatalogItem, stats *azerothcore.CatalogStats) {
	// Marks of Sanctification: 52025-52027 from 25N and 10H, 52028-52030 from 25H
	markOf := func(item *proto.CatalogItem, heroic bool) bool {
		return hasSource(item, func(s *proto.CatalogSource) bool {
			if heroic {
				return s.ViaItemId >= 52028 && s.ViaItemId <= 52030
			}
			return s.ViaItemId >= 52025 && s.ViaItemId <= 52027
		})
	}
	tiers := map[int32]int{}
	viaToken, older := 0, 0
	var normal, heroic *proto.CatalogItem
	for _, item := range catalog.Items {
		if !strings.HasPrefix(item.Name, "Sanctified ") {
			continue
		}
		if item.ProgressionTier < 13 {
			older++ // vanilla namesakes like the Sanctified Orb
			continue
		}
		tiers[item.ProgressionTier]++
		switch {
		case markOf(item, true):
			viaToken++
			if heroic == nil {
				heroic = item
			}
		case markOf(item, false):
			viaToken++
			if normal == nil {
				normal = item
			}
		}
	}
	var examples []*proto.CatalogItem
	for _, item := range []*proto.CatalogItem{normal, heroic} {
		if item != nil {
			examples = append(examples, item)
		}
	}
	fmt.Fprintf(w, "T10 (Sanctified pieces, %d vanilla namesakes left out): %d, %d bought through a Mark of Sanctification\n",
		older, tierCount(tiers), viaToken)
	histogram(w, "tiers", tiers)
	for _, item := range examples {
		fmt.Fprintf(w, "  %s\n", describe(item))
		for _, s := range item.Sources {
			fmt.Fprintf(w, "    %s tier %d via %d (%s) cost %d npc %d\n", s.Kind, s.ProgressionTier, s.ViaItemId,
				tierText(stats.ItemTiers, s.ViaItemId), s.ExtendedCostId, s.NpcId)
		}
	}
}

func tierCount(counts map[int32]int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}

func spotLimits(w io.Writer, catalog *proto.ServerCatalog, cur map[int32]*proto.CatalogItem, ids []int32, title string) {
	groups := map[int32]*proto.LimitGroup{}
	for _, g := range catalog.LimitGroups {
		groups[g.Id] = g
	}
	fmt.Fprintf(w, "%s: %d items\n", title, len(ids))
	for i, id := range ids {
		if i == maxExamples {
			fmt.Fprintf(w, "  ... %d more\n", len(ids)-maxExamples)
			break
		}
		item := cur[id]
		if item == nil {
			fmt.Fprintf(w, "  %d: not in the catalog\n", id)
			continue
		}
		group := "none"
		if g := groups[item.LimitCategory]; g != nil {
			group = fmt.Sprintf("%d %q max %d", g.Id, g.Name, g.MaxEquipped)
		}
		fmt.Fprintf(w, "  %s: max_count %d, unique_equipped %v, limit group %s, requires %s %d\n",
			describe(item), item.MaxCount, item.UniqueEquipped, group, item.RequiredProfession, item.RequiredSkillRank)
	}
}

func spotToCFaction(w io.Writer, catalog *proto.ServerCatalog) {
	counts := map[proto.Faction]int{}
	byName := map[string][]*proto.CatalogItem{}
	for _, item := range catalog.Items {
		if !hasSource(item, func(s *proto.CatalogSource) bool { return s.MapId == 649 }) {
			continue
		}
		counts[item.Faction]++
		byName[item.Name] = append(byName[item.Name], item)
	}
	fmt.Fprintf(w, "ToC drops by faction: alliance %d, horde %d, both %d\n",
		counts[proto.Faction_Alliance], counts[proto.Faction_Horde], counts[proto.Faction_Unknown])
	shown := 0
	for _, name := range []string{"Solace of the Defeated", "Solace of the Fallen", "Death's Choice", "Death's Verdict"} {
		for _, item := range byName[name] {
			if shown < 8 {
				fmt.Fprintf(w, "  %s: %s\n", describe(item), item.Faction)
				shown++
			}
		}
	}
}
