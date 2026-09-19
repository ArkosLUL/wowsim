package azerothcore

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// CatalogStats is what ResolveCatalog left out or guessed, for the tool's report.
type CatalogStats struct {
	// Catalog-scope items that nothing on the server awards. They're not in the catalog.
	Unobtainable []int32
	// Items with sources that resolve to no tier and no Classic phase to fall back on. Also left out.
	Unresolved []int32
	// Items that took their Classic phase (CatalogItem.fallback_tier).
	Fallback []int32
	// Loot holders nothing places on a map, with how many rare or better catalog items of item
	// level 200+ they drop that resolve nowhere else. Extend knownScriptSummons when one matters.
	UnplacedHolders []UnplacedHolder
	// Every item's resolved PvE tier, catalog or not (emblems, tokens, reagents).
	ItemTiers map[int32]int32
	// Explain lists every way to get an item, catalog or not, with each one's PvE tier.
	Explain func(item int32) []string
}

type UnplacedHolder struct {
	GameObject bool
	Entry      int32
	Name       string
	Items      int
}

// Collapsed into one source past this many creatures, objects or vendors.
const maxListedHolders = 3

// ResolveCatalog applies the progression tier rules to rows. It's pure: rows in, catalog out.
//
// An item's tier is the lowest over its sources; a source's tier is the highest over what it
// needs: its map (mapTier), script and condition gates, and the items it's made from, bought with,
// contained in or handed in for, resolved recursively. Sources needing PvP currencies or found on
// PvP maps only count when an item has no other source, which flags it pvp.
func ResolveCatalog(rows *CatalogRows) (*proto.ServerCatalog, *CatalogStats) {
	r := newResolver(rows)
	r.placeHolders()
	r.addLootSources()
	r.addVendorSources()
	r.addQuestSources()
	r.addSpellSources()
	r.addAchievementSources()
	r.addTokenTurnIns()
	pve, all := r.pruneQuestNeeds()
	r.reportUnplaced(all)
	return r.buildCatalog(pve, all)
}

type mapContext struct{ Map, Difficulty int32 }

type creatureParent struct {
	base  int32
	index int32 // 1..3: difficulty_entry_N
}

type condKey struct{ sourceType, group, entry int32 }

type gate struct {
	lo   int32
	team proto.Faction
}

type lootDrop struct {
	item int32
	gate gate
}

type nodeKind int8

const (
	nodeItem nodeKind = iota
	nodeQuest
	nodeQuestStart
	nodeSpell
	nodeDisenchant
	nodeMail
)

type nodeID struct {
	kind nodeKind
	id   int32
}

// alt is one way to get a node: its own tier (static) and the nodes it needs. On item nodes it
// also carries what's needed to describe it as a CatalogSource.
type alt struct {
	static int32
	needs  []nodeID
	pvp    bool
	team   proto.Faction

	kind       proto.CatalogSourceKind
	label      string
	mapID      int32
	holders    []int32 // creatures, gameobjects or vendors; the first maxListedHolders+1 distinct
	holderGO   bool
	holderN    int
	questID    int32
	spellID    int32
	extCost    int32
	achieve    int32
	repFaction int32
	repRank    int32
	profession proto.Profession
	// Items in needs that can explain the tier; via_item_id is the one with the highest.
	viaItems []int32
	// The source takes its kind from via_item_id's own best source (containers, item use).
	kindFromVia bool
}

type node struct {
	alts []*alt
}

type holderKey struct {
	item       int32
	kind       proto.CatalogSourceKind
	static     int32
	mapID      int32
	difficulty int32
	team       proto.Faction
	pvp        bool
	gameObject bool
	vendor     bool
	extCost    int32
	repFaction int32
	repRank    int32
}

type resolver struct {
	rows *CatalogRows

	items       map[int32]*CatalogItemRow
	maps        map[int32]CatalogMapRow
	maxPlayers  map[mapContext]int32
	difficulty  map[int32][]int32
	creatures   map[int32]*CatalogCreatureRow
	parents     map[int32]creatureParent
	gameObjects map[int32]*CatalogGameObjectRow
	conditions  map[condKey][]ConditionRow
	loot        map[LootStore]map[int32][]LootRow
	extCosts    map[int32]*ExtendedCostRow
	quests      map[int32]*QuestRow
	spells      map[int32]*CreateSpellRow
	transports  map[int32]int32 // transport map -> route map

	dropMemo map[LootStore]map[int32][]lootDrop
	refBusy  map[int32]bool

	creatureCtx map[int32]map[mapContext]int32 // base creature -> context -> gate
	goCtx       map[int32]map[mapContext]int32

	nodes   map[nodeID]*node
	holders map[holderKey]*alt
	// quest requirement alts, pruned once every source is in
	questNeeds []*alt
	// loot holders nothing places: {1 for gameobjects, entry} -> what they drop
	unplaced map[[2]int32][]int32

	stats CatalogStats
}

func newResolver(rows *CatalogRows) *resolver {
	r := &resolver{
		rows:        rows,
		items:       map[int32]*CatalogItemRow{},
		maps:        map[int32]CatalogMapRow{},
		maxPlayers:  map[mapContext]int32{},
		difficulty:  map[int32][]int32{},
		creatures:   map[int32]*CatalogCreatureRow{},
		parents:     map[int32]creatureParent{},
		gameObjects: map[int32]*CatalogGameObjectRow{},
		conditions:  map[condKey][]ConditionRow{},
		loot:        map[LootStore]map[int32][]LootRow{},
		extCosts:    map[int32]*ExtendedCostRow{},
		quests:      map[int32]*QuestRow{},
		spells:      map[int32]*CreateSpellRow{},
		dropMemo:    map[LootStore]map[int32][]lootDrop{},
		refBusy:     map[int32]bool{},
		creatureCtx: map[int32]map[mapContext]int32{},
		goCtx:       map[int32]map[mapContext]int32{},
		nodes:       map[nodeID]*node{},
		holders:     map[holderKey]*alt{},
		unplaced:    map[[2]int32][]int32{},
	}
	for i := range rows.Items {
		r.items[rows.Items[i].Entry] = &rows.Items[i]
	}
	for _, m := range rows.Maps {
		r.maps[m.ID] = m
	}
	for _, d := range rows.MapDifficulties {
		r.maxPlayers[mapContext{d.Map, d.Difficulty}] = d.MaxPlayers
		r.difficulty[d.Map] = append(r.difficulty[d.Map], d.Difficulty)
	}
	for i := range rows.Creatures {
		c := &rows.Creatures[i]
		r.creatures[c.Entry] = c
	}
	for _, c := range rows.Creatures {
		for i, child := range c.DifficultyEntries {
			if _, taken := r.parents[child]; child != 0 && !taken {
				r.parents[child] = creatureParent{base: c.Entry, index: int32(i + 1)}
			}
		}
	}
	for i := range rows.GameObjects {
		r.gameObjects[rows.GameObjects[i].Entry] = &rows.GameObjects[i]
	}
	for _, c := range rows.Conditions {
		key := condKey{c.SourceType, c.SourceGroup, c.SourceEntry}
		r.conditions[key] = append(r.conditions[key], c)
	}
	for _, l := range rows.Loot {
		if r.loot[l.Store] == nil {
			r.loot[l.Store] = map[int32][]LootRow{}
		}
		r.loot[l.Store][l.Entry] = append(r.loot[l.Store][l.Entry], l)
	}
	for i := range rows.ExtendedCosts {
		r.extCosts[rows.ExtendedCosts[i].ID] = &rows.ExtendedCosts[i]
	}
	for i := range rows.Quests {
		r.quests[rows.Quests[i].ID] = &rows.Quests[i]
	}
	for i := range rows.Spells {
		r.spells[rows.Spells[i].ID] = &rows.Spells[i]
	}
	r.transports = map[int32]int32{}
	for _, t := range rows.Transports {
		r.transports[t.Map] = t.RouteMap
	}
	return r
}

func (r *resolver) node(id nodeID) *node {
	n := r.nodes[id]
	if n == nil {
		n = &node{}
		r.nodes[id] = n
	}
	return n
}

func (r *resolver) addAlt(id nodeID, a *alt) {
	n := r.node(id)
	n.alts = append(n.alts, a)
}

func item(id int32) nodeID { return nodeID{nodeItem, id} }

// ---- conditions ----

func lootConditionType(store LootStore) int32 {
	switch store {
	case LootCreature:
		return 1
	case LootDisenchant:
		return 2
	case LootFishing:
		return 3
	case LootGameObject:
		return 4
	case LootItem:
		return 5
	case LootMail:
		return 6
	case LootMilling:
		return 7
	case LootPickpocketing:
		return 8
	case LootProspecting:
		return 9
	case LootReference:
		return 10
	case LootSkinning:
		return 11
	case LootSpell:
		return 12
	}
	return 0
}

const (
	conditionSourceQuestAvailable = 19
	conditionSourceVendor         = 23

	conditionTeam          = 6
	conditionQuestRewarded = 8
	conditionQuestNone     = 14
)

// conditionGate folds the conditions on one source into the lowest tier that passes them. Else
// groups are alternatives; within one, every condition must hold. Only progression quests and
// team conditions matter here; anything else is assumed passable. ok is false when no group can
// pass at any tier.
func (r *resolver) conditionGate(sourceType, group, entry int32) (gate, bool) {
	rows := r.conditions[condKey{sourceType, group, entry}]
	if len(rows) == 0 {
		return gate{}, true
	}
	type elseGroup struct {
		lo, hi   int32
		team     proto.Faction
		conflict bool
	}
	groups := map[int32]*elseGroup{}
	for _, c := range rows {
		g := groups[c.ElseGroup]
		if g == nil {
			g = &elseGroup{hi: tierUnknown}
			groups[c.ElseGroup] = g
		}
		switch c.Type {
		case conditionQuestRewarded, conditionQuestNone:
			tier := c.Value1 - progressionQuestBase
			if tier < 0 || tier > 20 {
				continue
			}
			// rewarded quest 66000+N means tier >= N; QUEST_NONE is its negation
			atLeast := (c.Type == conditionQuestRewarded) != c.Negative
			if atLeast {
				g.lo = max(g.lo, tier)
			} else {
				g.hi = min(g.hi, tier)
			}
		case conditionTeam:
			var team proto.Faction
			switch c.Value1 {
			case conditionTeamAlliance:
				team = proto.Faction_Alliance
			case conditionTeamHorde:
				team = proto.Faction_Horde
			default:
				continue
			}
			if c.Negative {
				team = otherFaction(team)
			}
			if g.team != proto.Faction_Unknown && team != g.team {
				g.conflict = true
			}
			g.team = team
		}
	}
	result := gate{lo: tierUnknown}
	first := true
	for _, g := range groups {
		if g.conflict || g.lo >= g.hi {
			continue
		}
		result.lo = min(result.lo, g.lo)
		if first {
			result.team = g.team
			first = false
		} else if result.team != g.team {
			result.team = proto.Faction_Unknown
		}
	}
	return result, !first
}

func otherFaction(f proto.Faction) proto.Faction {
	if f == proto.Faction_Alliance {
		return proto.Faction_Horde
	}
	return proto.Faction_Alliance
}

func combineGates(a, b gate) (gate, bool) {
	team := a.team
	if team == proto.Faction_Unknown {
		team = b.team
	} else if b.team != proto.Faction_Unknown && b.team != team {
		return gate{}, false
	}
	return gate{lo: max(a.lo, b.lo), team: team}, true
}

// ---- loot ----

// drops flattens a loot entry through its references, with each drop's condition gate.
func (r *resolver) drops(store LootStore, entry int32) []lootDrop {
	if entry == 0 {
		return nil
	}
	if memo, ok := r.dropMemo[store][entry]; ok {
		return memo
	}
	if store == LootReference {
		if r.refBusy[entry] {
			return nil
		}
		r.refBusy[entry] = true
		defer delete(r.refBusy, entry)
	}
	var out []lootDrop
	for _, row := range r.loot[store][entry] {
		g, ok := r.conditionGate(lootConditionType(store), entry, row.Item)
		if !ok {
			continue
		}
		if row.Reference == 0 {
			out = append(out, lootDrop{row.Item, g})
			continue
		}
		ref := row.Reference
		if ref < 0 {
			ref = -ref
		}
		for _, d := range r.drops(LootReference, ref) {
			if combined, ok := combineGates(g, d.gate); ok {
				out = append(out, lootDrop{d.item, combined})
			}
		}
	}
	if r.dropMemo[store] == nil {
		r.dropMemo[store] = map[int32][]lootDrop{}
	}
	r.dropMemo[store][entry] = out
	return out
}

// ---- placing creatures and gameobjects ----

func (r *resolver) baseCreature(entry int32) (int32, int32) {
	if p, ok := r.parents[entry]; ok {
		return p.base, p.index
	}
	return entry, 0
}

func addContext(ctxs map[int32]map[mapContext]int32, entry int32, c mapContext, g int32) bool {
	m := ctxs[entry]
	if m == nil {
		m = map[mapContext]int32{}
		ctxs[entry] = m
	}
	if old, ok := m[c]; ok && old <= g {
		return false
	}
	m[c] = g
	return true
}

// spawnDifficulties turns a spawnMask into difficulties. Continents only have difficulty 0.
func (r *resolver) spawnDifficulties(mapID, spawnMask int32) []int32 {
	if spawnMask == 0 {
		return nil
	}
	m := r.maps[mapID]
	if m.Type != mapTypeDungeon && m.Type != mapTypeRaid {
		return []int32{0}
	}
	var out []int32
	for d := int32(0); d < 4; d++ {
		if spawnMask&(1<<d) == 0 {
			continue
		}
		// legacy raids spawn with masks wider than their difficulties
		if _, listed := r.maxPlayers[mapContext{mapID, d}]; d <= 1 || listed {
			out = append(out, d)
		}
	}
	return out
}

func (r *resolver) creatureGate(entry int32, spawnScript string) int32 {
	if spawnScript != "" {
		return scriptGates[spawnScript]
	}
	if c := r.creatures[entry]; c != nil {
		return scriptGates[c.ScriptName]
	}
	return 0
}

func (r *resolver) goGate(entry int32, spawnScript string) int32 {
	if spawnScript != "" {
		return scriptGates[spawnScript]
	}
	if g := r.gameObjects[entry]; g != nil {
		return scriptGates[g.ScriptName]
	}
	return 0
}

// placeHolders finds every map and difficulty each creature and gameobject appears on: spawns,
// encounter credits, known script summons, then summons from those, transitively.
func (r *resolver) placeHolders() {
	for _, s := range r.rows.CreatureSpawns {
		base, index := r.baseCreature(s.Entry)
		mapID := r.routeMap(s.Map)
		g := max(r.creatureGate(s.Entry, s.ScriptName), phaseGate(mapID, s.PhaseMask))
		if index > 0 {
			if s.SpawnMask != 0 {
				addContext(r.creatureCtx, base, mapContext{mapID, index}, g)
			}
			continue
		}
		for _, d := range r.spawnDifficulties(mapID, s.SpawnMask) {
			addContext(r.creatureCtx, base, mapContext{mapID, d}, g)
		}
	}
	for _, s := range r.rows.GameObjectSpawns {
		mapID := r.routeMap(s.Map)
		g := max(r.goGate(s.Entry, s.ScriptName), phaseGate(mapID, s.PhaseMask))
		for _, d := range r.spawnDifficulties(mapID, s.SpawnMask) {
			addContext(r.goCtx, s.Entry, mapContext{mapID, d}, g)
		}
	}
	for _, e := range r.rows.Encounters {
		base, _ := r.baseCreature(e.CreditEntry)
		for _, d := range r.encounterDifficulties(e) {
			addContext(r.creatureCtx, base, mapContext{e.Map, d}, r.creatureGate(base, ""))
		}
	}
	for _, s := range knownScriptSummons {
		for _, d := range s.Difficulties {
			if s.GameObject {
				addContext(r.goCtx, s.Entry, mapContext{s.Map, d}, r.goGate(s.Entry, ""))
			} else {
				base, _ := r.baseCreature(s.Entry)
				addContext(r.creatureCtx, base, mapContext{s.Map, d}, r.creatureGate(base, ""))
			}
		}
	}

	for changed := true; changed; {
		changed = false
		for _, s := range r.rows.Summons {
			var from map[mapContext]int32
			switch s.SummonerKind {
			case SummonerCreature:
				base, _ := r.baseCreature(s.SummonerID)
				from = r.creatureCtx[base]
			case SummonerGameObject:
				from = r.goCtx[s.SummonerID]
			case SummonerMap:
				from = map[mapContext]int32{}
				for _, d := range r.mapDifficulties(s.SummonerID) {
					from[mapContext{s.SummonerID, d}] = 0
				}
			}
			for c, g := range from {
				if s.GameObject {
					changed = addContext(r.goCtx, s.Entry, c, max(g, r.goGate(s.Entry, ""))) || changed
				} else {
					base, _ := r.baseCreature(s.Entry)
					changed = addContext(r.creatureCtx, base, c, max(g, r.creatureGate(base, ""))) || changed
				}
			}
		}
	}
}

// encounterDifficulties: DungeonEncounter.dbc lists ICC and RS encounters under 0 and 1 only, but
// their bosses fight in the heroic of each size too (Sindragosa has no spawn row). 40-man modes
// on difficulty 2 aren't heroics, so they're left alone.
func (r *resolver) encounterDifficulties(e EncounterRow) []int32 {
	ds := []int32{e.Difficulty}
	if r.maps[e.Map].Type == mapTypeRaid && e.Difficulty < 2 {
		if players, ok := r.maxPlayers[mapContext{e.Map, e.Difficulty + 2}]; ok && players <= 25 {
			ds = append(ds, e.Difficulty+2)
		}
	}
	return ds
}

// routeMap puts a spawn on a transport (the ICC and HoR gunships) in the instance it flies through.
func (r *resolver) routeMap(mapID int32) int32 {
	if route, ok := r.transports[mapID]; ok {
		return route
	}
	return mapID
}

func (r *resolver) mapDifficulties(mapID int32) []int32 {
	if ds := r.difficulty[mapID]; len(ds) > 0 {
		return ds
	}
	return []int32{0}
}

// contextTier is where a holder in context c opens: its map's tier and its script gate.
func (r *resolver) contextTier(c mapContext, g, boss int32) int32 {
	return max(mapTier(r.maps, c.Map, c.Difficulty, boss), g)
}

// holderTier is the lowest tier over a holder's contexts, and that context, or tierUnknown.
func holderTier(r *resolver, ctxs map[mapContext]int32, boss int32) (int32, mapContext) {
	best, bestCtx := tierUnknown, mapContext{}
	for c, g := range ctxs {
		t := r.contextTier(c, g, boss)
		if t < best || t == best && (c.Map < bestCtx.Map || c.Map == bestCtx.Map && c.Difficulty < bestCtx.Difficulty) {
			best, bestCtx = t, c
		}
	}
	return best, bestCtx
}

func (r *resolver) creatureTier(entry int32) int32 {
	base, _ := r.baseCreature(entry)
	t, _ := holderTier(r, r.creatureCtx[base], base)
	return t
}

func (r *resolver) goTier(entry int32) int32 {
	t, _ := holderTier(r, r.goCtx[entry], 0)
	return t
}

// effectiveCreature follows Creature::InitEntry: a missing difficulty entry falls back to the one
// below it, and raid heroics fall back to their normal size.
func (r *resolver) effectiveCreature(base *CatalogCreatureRow, mapType, difficulty int32) *CatalogCreatureRow {
	for d := difficulty; d > 0; {
		if e := base.DifficultyEntries[d-1]; e != 0 {
			if t := r.creatures[e]; t != nil {
				return t
			}
		}
		if d >= 2 && mapType == mapTypeRaid {
			d -= 2
		} else {
			d--
		}
	}
	return base
}

// ---- sources ----

func (r *resolver) addHolderDrop(itemID int32, key holderKey, holder int32) {
	a := r.holders[key]
	if a == nil {
		a = &alt{static: key.static, pvp: key.pvp, team: key.team, kind: key.kind, mapID: key.mapID,
			holderGO: key.gameObject, extCost: key.extCost, repFaction: key.repFaction, repRank: key.repRank}
		if key.static != tierUnknown {
			a.label = r.contextLabel(mapContext{key.mapID, key.difficulty})
		}
		r.holders[key] = a
		r.addAlt(item(itemID), a)
	}
	if !slices.Contains(a.holders, holder) {
		if len(a.holders) <= maxListedHolders {
			a.holders = append(a.holders, holder)
		}
		a.holderN++
	}
}

func (r *resolver) addLootSources() {
	creatureStores := []func(*CatalogCreatureRow) (LootStore, int32){
		func(c *CatalogCreatureRow) (LootStore, int32) { return LootCreature, c.LootID },
		func(c *CatalogCreatureRow) (LootStore, int32) { return LootSkinning, c.SkinLootID },
		func(c *CatalogCreatureRow) (LootStore, int32) { return LootPickpocketing, c.PickpocketLootID },
	}
	for _, base := range r.rows.Creatures {
		if _, child := r.parents[base.Entry]; child {
			continue
		}
		used := map[int32]bool{}
		for c, g := range r.creatureCtx[base.Entry] {
			m := r.maps[c.Map]
			eff := r.effectiveCreature(&base, m.Type, c.Difficulty)
			used[eff.Entry] = true
			static := r.contextTier(c, g, base.Entry)
			kind := sourceKind(m.Type, c.Difficulty, r.maxPlayers[c])
			for _, store := range creatureStores {
				s, id := store(eff)
				for _, d := range r.drops(s, id) {
					r.addHolderDrop(d.item, holderKey{item: d.item, kind: kind, static: max(static, d.gate.lo), mapID: c.Map,
						difficulty: c.Difficulty, team: d.gate.team, pvp: isPvPMap(m.Type)}, base.Entry)
				}
			}
		}
		// templates no placed context uses (all of them when nothing places the creature): their loot
		// is still awarded by a table, so keep it resolvable in principle for the fallback, and report it
		templates := []*CatalogCreatureRow{&base}
		for _, e := range base.DifficultyEntries {
			if t := r.creatures[e]; t != nil {
				templates = append(templates, t)
			}
		}
		for _, t := range templates {
			if used[t.Entry] {
				continue
			}
			for _, store := range creatureStores {
				s, id := store(t)
				for _, d := range r.drops(s, id) {
					r.addHolderDrop(d.item, holderKey{item: d.item, static: tierUnknown, team: d.gate.team}, base.Entry)
					key := [2]int32{0, t.Entry}
					r.unplaced[key] = append(r.unplaced[key], d.item)
				}
			}
		}
	}

	for _, gobj := range r.rows.GameObjects {
		if gobj.LootID == 0 || gobj.Type != 3 && gobj.Type != 25 {
			continue
		}
		ctxs := r.goCtx[gobj.Entry]
		drops := r.drops(LootGameObject, gobj.LootID)
		if len(ctxs) == 0 {
			for _, d := range drops {
				r.addHolderDrop(d.item, holderKey{item: d.item, static: tierUnknown, team: d.gate.team, gameObject: true}, gobj.Entry)
				r.unplaced[[2]int32{1, gobj.Entry}] = append(r.unplaced[[2]int32{1, gobj.Entry}], d.item)
			}
			continue
		}
		for c, g := range ctxs {
			m := r.maps[c.Map]
			static := r.contextTier(c, g, 0)
			kind := sourceKind(m.Type, c.Difficulty, r.maxPlayers[c])
			for _, d := range drops {
				r.addHolderDrop(d.item, holderKey{item: d.item, kind: kind, static: max(static, d.gate.lo), mapID: c.Map,
					difficulty: c.Difficulty, team: d.gate.team, pvp: isPvPMap(m.Type), gameObject: true}, gobj.Entry)
			}
		}
	}

	// loot opened from, or processed out of, another item
	for _, s := range []struct {
		store LootStore
		kind  proto.CatalogSourceKind
		label string
	}{
		{LootItem, proto.CatalogSourceKind_CatalogSourceUnknown, "Contained in"},
		{LootProspecting, proto.CatalogSourceKind_CatalogSourceProspecting, "Prospecting"},
		{LootMilling, proto.CatalogSourceKind_CatalogSourceProspecting, "Milling"},
	} {
		for _, entry := range sortedKeys(r.loot[s.store]) {
			for _, d := range r.drops(s.store, entry) {
				r.addAlt(item(d.item), &alt{static: d.gate.lo, team: d.gate.team, needs: []nodeID{item(entry)},
					kind: s.kind, label: s.label, viaItems: []int32{entry}, kindFromVia: s.store == LootItem})
			}
		}
	}

	for _, it := range r.rows.Items {
		if it.DisenchantID != 0 {
			r.addAlt(nodeID{nodeDisenchant, it.DisenchantID}, &alt{needs: []nodeID{item(it.Entry)}})
		}
	}
	for _, entry := range sortedKeys(r.loot[LootDisenchant]) {
		for _, d := range r.drops(LootDisenchant, entry) {
			r.addAlt(item(d.item), &alt{static: d.gate.lo, team: d.gate.team, needs: []nodeID{{nodeDisenchant, entry}},
				kind: proto.CatalogSourceKind_CatalogSourceWorldDrop, label: "Disenchanting"})
		}
	}

	areaMaps := map[int32]int32{}
	for _, a := range r.rows.Areas {
		areaMaps[a.ID] = a.Map
	}
	for _, entry := range sortedKeys(r.loot[LootFishing]) {
		static := tierUnknown
		mapID, ok := areaMaps[entry]
		if ok {
			static = mapTier(r.maps, mapID, 0, 0)
		}
		for _, d := range r.drops(LootFishing, entry) {
			r.addAlt(item(d.item), &alt{static: max(static, d.gate.lo), team: d.gate.team, mapID: mapID,
				kind: proto.CatalogSourceKind_CatalogSourceWorldDrop, label: "Fishing: " + r.maps[mapID].Name})
		}
	}

	for _, q := range r.rows.Quests {
		if q.RewardMailTemplateID != 0 {
			r.addAlt(nodeID{nodeMail, q.RewardMailTemplateID}, &alt{needs: []nodeID{{nodeQuest, q.ID}}})
		}
	}
	for _, a := range r.rows.Achievements {
		if a.MailTemplateID != 0 {
			r.addAlt(nodeID{nodeMail, a.MailTemplateID}, &alt{static: r.achievementTier(a)})
		}
	}
	for _, entry := range sortedKeys(r.loot[LootMail]) {
		for _, d := range r.drops(LootMail, entry) {
			r.addAlt(item(d.item), &alt{static: d.gate.lo, team: d.gate.team, needs: []nodeID{{nodeMail, entry}},
				kind: proto.CatalogSourceKind_CatalogSourceQuest, label: "Mail reward"})
		}
	}

	// spell loot comes from a profession spell, or from using the items that cast the spell
	usedBy := map[int32][]int32{}
	for _, it := range r.rows.Items {
		for _, s := range it.UseSpells {
			usedBy[s] = append(usedBy[s], it.Entry)
		}
	}
	for _, entry := range sortedKeys(r.loot[LootSpell]) {
		for _, d := range r.drops(LootSpell, entry) {
			a := &alt{static: d.gate.lo, team: d.gate.team, needs: []nodeID{{nodeSpell, entry}}, spellID: entry}
			if s := r.spells[entry]; s != nil && s.Profession != proto.Profession_ProfessionUnknown {
				a.kind, a.profession, a.label = proto.CatalogSourceKind_CatalogSourceCrafted, s.Profession, s.Name
			} else {
				a.kindFromVia, a.viaItems, a.label = true, usedBy[entry], "Use"
			}
			r.addAlt(item(d.item), a)
		}
	}
}

// reportUnplaced lists the unplaced holders that leave level-80 catalog items without a tier.
func (r *resolver) reportUnplaced(all map[nodeID]int32) {
	for key, drops := range r.unplaced {
		seen := map[int32]bool{}
		for _, id := range drops {
			it := r.items[id]
			if it != nil && isCatalogItem(it) && it.Quality >= 3 && it.ItemLevel >= 200 && all[item(id)] == tierUnknown {
				seen[id] = true
			}
		}
		if len(seen) == 0 {
			continue
		}
		h := UnplacedHolder{GameObject: key[0] == 1, Entry: key[1], Items: len(seen)}
		if h.GameObject {
			h.Name = r.gameObjects[h.Entry].Name
		} else {
			h.Name = r.creatures[h.Entry].Name
		}
		r.stats.UnplacedHolders = append(r.stats.UnplacedHolders, h)
	}
	slices.SortFunc(r.stats.UnplacedHolders, func(a, b UnplacedHolder) int {
		return cmp.Or(cmp.Compare(b.Items, a.Items), cmp.Compare(a.Entry, b.Entry))
	})
}

func (r *resolver) addVendorSources() {
	for _, v := range r.rows.Vendors {
		it := r.items[v.Item]
		if it == nil {
			continue
		}
		base, _ := r.baseCreature(v.Entry)
		tier, c := holderTier(r, r.creatureCtx[base], base)
		g, ok := r.conditionGate(conditionSourceVendor, v.Entry, v.Item)
		if !ok {
			continue
		}
		static := tier
		if tier != tierUnknown {
			static = max(tier, g.lo)
		}
		kind := proto.CatalogSourceKind_CatalogSourceVendor
		key := holderKey{item: v.Item, static: static, mapID: c.Map, difficulty: c.Difficulty, team: g.team,
			vendor: true, extCost: v.ExtendedCost, pvp: isPvPMap(r.maps[c.Map].Type)}
		if it.RequiredReputationFaction > 0 {
			kind = proto.CatalogSourceKind_CatalogSourceReputation
			key.repFaction, key.repRank = it.RequiredReputationFaction, it.RequiredReputationRank
		}
		key.kind = kind
		var needs []nodeID
		var via []int32
		if cost := r.extCosts[v.ExtendedCost]; cost != nil {
			if cost.HonorPoints > 0 || cost.ArenaPoints > 0 || cost.ArenaRating > 0 {
				key.pvp = true
			}
			for _, id := range cost.Items {
				if id == 0 {
					continue
				}
				if pvpCurrencies[id] {
					key.pvp = true
				}
				needs = append(needs, item(id))
				via = append(via, id)
			}
		}
		fresh := r.holders[key] == nil
		r.addHolderDrop(v.Item, key, base)
		if a := r.holders[key]; fresh {
			a.needs, a.viaItems = needs, via
			if static != tierUnknown {
				a.label = r.maps[c.Map].Name
			}
		}
	}
}

func (r *resolver) addTokenTurnIns() {
	for _, t := range r.rows.TokenTurnIns {
		a := &alt{kind: proto.CatalogSourceKind_CatalogSourceVendor, label: "Token turn-in",
			needs: []nodeID{item(t.Token)}, viaItems: []int32{t.Token}}
		if t.Secondary != 0 {
			a.needs = append(a.needs, item(t.Secondary))
			a.viaItems = append(a.viaItems, t.Secondary)
		}
		r.addAlt(item(t.Result), a)
	}
}

func (r *resolver) achievementTier(a AchievementRewardRow) int32 {
	if a.Map < 0 {
		return tierUnknown
	}
	return mapTier(r.maps, a.Map, 0, 0)
}

func (r *resolver) addAchievementSources() {
	for _, a := range r.rows.Achievements {
		if a.Item == 0 {
			continue
		}
		r.addAlt(item(a.Item), &alt{static: r.achievementTier(a), kind: proto.CatalogSourceKind_CatalogSourceAchievement,
			achieve: a.ID, label: "Achievement: " + a.Name})
	}
}

// addQuestSources: a quest reward has the tier of everything the quest needs, which is where it
// starts, the quest before it, the items it asks for and what it asks you to kill.
func (r *resolver) addQuestSources() {
	for _, s := range r.rows.QuestStarters {
		start := nodeID{nodeQuestStart, s.Quest}
		switch s.Kind {
		case QuestStarterCreature:
			r.addAlt(start, &alt{static: r.creatureTier(s.Entry)})
		case QuestStarterGameObject:
			r.addAlt(start, &alt{static: r.goTier(s.Entry)})
		case QuestStarterItem:
			r.addAlt(start, &alt{needs: []nodeID{item(s.Entry)}})
		}
	}
	for _, q := range r.rows.Quests {
		if q.RewardNextQuest != 0 {
			r.addAlt(nodeID{nodeQuestStart, q.RewardNextQuest}, &alt{needs: []nodeID{{nodeQuest, q.ID}}})
		}
	}

	for _, q := range r.rows.Quests {
		g, ok := r.conditionGate(conditionSourceQuestAvailable, 0, q.ID)
		if !ok {
			continue
		}
		need := &alt{static: g.lo, needs: []nodeID{{nodeQuestStart, q.ID}}}
		if q.PrevQuest != 0 {
			prev := q.PrevQuest
			if prev < 0 {
				prev = -prev
			}
			if r.quests[prev] != nil {
				need.needs = append(need.needs, nodeID{nodeQuest, prev})
			}
		}
		for _, id := range q.RequiredItems {
			if id != 0 && !slices.Contains(q.ProvidedItems, id) {
				need.needs = append(need.needs, item(id))
			}
		}
		r.questNeeds = append(r.questNeeds, need)
		for _, target := range q.RequiredNpcOrGo {
			t := tierUnknown
			switch {
			case target > 0:
				t = r.creatureTier(target)
			case target < 0:
				t = r.goTier(-target)
			}
			// a kill target nothing places doesn't tell us anything
			if t != tierUnknown {
				need.static = max(need.static, t)
			}
		}
		r.addAlt(nodeID{nodeQuest, q.ID}, need)

		team := raceMaskFaction(q.AllowableRaces)
		for _, id := range q.RewardItems {
			if id == 0 {
				continue
			}
			a := &alt{needs: []nodeID{{nodeQuest, q.ID}}, team: team, questID: q.ID,
				kind: proto.CatalogSourceKind_CatalogSourceQuest, label: "Quest: " + q.Title}
			if q.RequiredRepFaction > 0 {
				a.kind = proto.CatalogSourceKind_CatalogSourceReputation
				a.repFaction, a.repRank = q.RequiredRepFaction, reputationRank(q.RequiredRepValue)
			}
			r.addAlt(item(id), a)
		}
	}
}

// pruneQuestNeeds drops required items whose tier nothing tells us: items nothing awards, which
// the quest's own scripts hand out, and items that only come from objects scripts summon mid-quest
// (Light's Vengeance for Shadow's Edge, Restored Quel'Delar). The rest of the chain carries the
// tier.
func (r *resolver) pruneQuestNeeds() (pve, all map[nodeID]int32) {
	for _, need := range r.questNeeds {
		need.needs = slices.DeleteFunc(need.needs, func(n nodeID) bool {
			return n.kind == nodeItem && r.nodes[n] == nil
		})
	}
	// judge each required item with every quest's required items waived: a quest item that only
	// looks unresolved because its own quest waits on a script item still gates what asks for it
	kept := make([][]nodeID, len(r.questNeeds))
	for i, need := range r.questNeeds {
		kept[i] = need.needs
		need.needs = slices.DeleteFunc(slices.Clone(need.needs), func(n nodeID) bool { return n.kind == nodeItem })
	}
	open := r.solve(true)
	for i, need := range r.questNeeds {
		need.needs = slices.DeleteFunc(kept[i], func(n nodeID) bool {
			return n.kind == nodeItem && open[n] == tierUnknown
		})
	}
	// what's left unresolved hangs on a loop of quest items, so drop it until nothing changes
	for {
		all = r.solve(true)
		pruned := false
		for _, need := range r.questNeeds {
			before := len(need.needs)
			need.needs = slices.DeleteFunc(need.needs, func(n nodeID) bool {
				return n.kind == nodeItem && all[n] == tierUnknown
			})
			pruned = pruned || len(need.needs) != before
		}
		if !pruned {
			return r.solve(false), all
		}
	}
}

// addSpellSources: a crafted item has the tier of its latest reagent. On-use spells also need the
// item that casts them.
func (r *resolver) addSpellSources() {
	usedBy := map[int32][]int32{}
	for _, it := range r.rows.Items {
		for _, s := range it.UseSpells {
			usedBy[s] = append(usedBy[s], it.Entry)
		}
	}
	for _, s := range r.rows.Spells {
		var reagents []nodeID
		for _, id := range s.Reagents {
			reagents = append(reagents, item(id))
		}
		ways := []*alt{}
		if s.Profession != proto.Profession_ProfessionUnknown {
			ways = append(ways, &alt{needs: reagents, viaItems: s.Reagents, kind: proto.CatalogSourceKind_CatalogSourceCrafted,
				profession: s.Profession, label: s.Name})
		}
		for _, user := range usedBy[s.ID] {
			ways = append(ways, &alt{needs: append([]nodeID{item(user)}, reagents...), viaItems: []int32{user},
				kindFromVia: true, label: "Use"})
		}
		for _, way := range ways {
			r.addAlt(nodeID{nodeSpell, s.ID}, &alt{needs: way.needs})
			for _, created := range s.Creates {
				a := *way
				a.spellID = s.ID
				r.addAlt(item(created), &a)
			}
		}
	}
}

// ---- solving ----

// solve finds every node's lowest tier by relaxing from "unknown" until nothing changes, so
// cycles (transmutes, containers) never settle below what their sources allow.
func (r *resolver) solve(withPvP bool) map[nodeID]int32 {
	tiers := make(map[nodeID]int32, len(r.nodes))
	ids := make([]nodeID, 0, len(r.nodes))
	for id := range r.nodes {
		ids = append(ids, id)
		tiers[id] = tierUnknown
	}
	for changed := true; changed; {
		changed = false
		for _, id := range ids {
			best := tiers[id]
			for _, a := range r.nodes[id].alts {
				if a.pvp && !withPvP {
					continue
				}
				if t := altTier(a, tiers); t < best {
					best = t
				}
			}
			if floor, ok := currencyTiers[id.id]; ok && id.kind == nodeItem && best != tierUnknown {
				best = max(best, floor)
			}
			if best < tiers[id] {
				tiers[id] = best
				changed = true
			}
		}
	}
	return tiers
}

func altTier(a *alt, tiers map[nodeID]int32) int32 {
	t := a.static
	for _, n := range a.needs {
		if t == tierUnknown {
			break
		}
		nt, ok := tiers[n]
		if !ok {
			return tierUnknown
		}
		t = max(t, nt)
	}
	return t
}

// ---- output ----

func (r *resolver) buildCatalog(pve, all map[nodeID]int32) (*proto.ServerCatalog, *CatalogStats) {
	catalog := &proto.ServerCatalog{}
	usedLimits := map[int32]bool{}

	ids := make([]int32, 0, len(r.items))
	for id, it := range r.items {
		if isCatalogItem(it) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	for _, id := range ids {
		row := r.items[id]
		n := r.nodes[item(id)]
		if n == nil || len(n.alts) == 0 {
			r.stats.Unobtainable = append(r.stats.Unobtainable, id)
			continue
		}
		ci := &proto.CatalogItem{
			Id:                 id,
			Name:               row.Name,
			IsGem:              isGem(row),
			Pvp:                row.HasResilience,
			StatsCount:         row.StatsCount,
			MaxCount:           max(row.MaxCount, 0),
			UniqueEquipped:     row.Flags&itemFlagUniqueEquipped != 0,
			LimitCategory:      row.ItemLimitCategory,
			RequiredProfession: skillProfessions[row.RequiredSkill],
			HasEffect:          row.HasEffect,
		}
		if ci.RequiredProfession != proto.Profession_ProfessionUnknown {
			ci.RequiredSkillRank = row.RequiredSkillRank
		}

		var chosen []*alt
		tiers := pve
		switch {
		case pve[item(id)] != tierUnknown:
			ci.ProgressionTier = pve[item(id)]
			for _, a := range n.alts {
				if !a.pvp && altTier(a, pve) != tierUnknown {
					chosen = append(chosen, a)
				}
			}
		case all[item(id)] != tierUnknown:
			ci.ProgressionTier = all[item(id)]
			ci.Pvp = true
			tiers = all
			for _, a := range n.alts {
				if altTier(a, all) != tierUnknown {
					chosen = append(chosen, a)
				}
			}
		default:
			phase, ok := r.rows.ClassicPhases[id]
			if !ok {
				r.stats.Unresolved = append(r.stats.Unresolved, id)
				continue
			}
			ci.ProgressionTier = 12 + max(phase, 1)
			ci.FallbackTier = true
			r.stats.Fallback = append(r.stats.Fallback, id)
			tiers = all
			chosen = n.alts
			allPvP := true
			for _, a := range n.alts {
				allPvP = allPvP && a.pvp
			}
			ci.Pvp = ci.Pvp || allPvP
		}

		ci.Faction = itemFaction(row)
		if ci.Faction == proto.Faction_Unknown {
			ci.Faction = sharedTeam(chosen)
		}
		limit := maxSources
		if ci.ProgressionTier < firstWotlkTier {
			limit = maxOldSources
		}
		ci.Sources = r.mergeSources(r.describeSources(chosen, tiers, ci.FallbackTier, ci.ProgressionTier), limit)
		if ci.LimitCategory != 0 {
			usedLimits[ci.LimitCategory] = true
		}
		catalog.Items = append(catalog.Items, ci)
	}

	for _, lc := range r.rows.LimitCategories {
		if usedLimits[lc.ID] {
			catalog.LimitGroups = append(catalog.LimitGroups, &proto.LimitGroup{Id: lc.ID, Name: lc.Name, MaxEquipped: lc.MaxCount})
		}
	}
	slices.SortFunc(catalog.LimitGroups, func(a, b *proto.LimitGroup) int { return cmp.Compare(a.Id, b.Id) })

	r.stats.ItemTiers = map[int32]int32{}
	for id, t := range pve {
		if id.kind == nodeItem && t != tierUnknown {
			r.stats.ItemTiers[id.id] = t
		}
	}
	r.stats.Explain = func(id int32) []string { return r.explain(item(id), pve, 0) }
	return catalog, &r.stats
}

func (r *resolver) explain(id nodeID, tiers map[nodeID]int32, depth int) []string {
	n := r.nodes[id]
	if n == nil {
		return nil
	}
	indent := strings.Repeat("  ", depth)
	var out []string
	for i, a := range n.alts {
		if i == 200 {
			out = append(out, fmt.Sprintf("%s... %d more", indent, len(n.alts)-200))
			break
		}
		var needs []string
		for _, need := range a.needs {
			t, ok := tiers[need]
			if !ok {
				t = tierUnknown
			}
			needs = append(needs, fmt.Sprintf("%d:%d=%s", need.kind, need.id, tierString(t)))
		}
		out = append(out, fmt.Sprintf("%s%s tier %s static %s pvp %v label %q map %d holders %v (%d) quest %d spell %d cost %d needs [%s]",
			indent, a.kind, tierString(altTier(a, tiers)), tierString(a.static), a.pvp, a.label, a.mapID, a.holders, a.holderN,
			a.questID, a.spellID, a.extCost, strings.Join(needs, " ")))
		if depth < 2 {
			for _, need := range a.needs {
				if need.kind != nodeItem {
					out = append(out, r.explain(need, tiers, depth+1)...)
				}
			}
		}
	}
	return out
}

func tierString(t int32) string {
	if t == tierUnknown {
		return "?"
	}
	return fmt.Sprint(t)
}

func sharedTeam(alts []*alt) proto.Faction {
	team := proto.Faction_Unknown
	for i, a := range alts {
		if a.team == proto.Faction_Unknown || i > 0 && a.team != team {
			return proto.Faction_Unknown
		}
		team = a.team
	}
	return team
}

func (r *resolver) describeSources(alts []*alt, tiers map[nodeID]int32, fallback bool, fallbackTier int32) []*proto.CatalogSource {
	var out []*proto.CatalogSource
	for _, a := range alts {
		tier := altTier(a, tiers)
		if fallback {
			tier = fallbackTier
		}
		base := &proto.CatalogSource{
			Kind:                a.kind,
			ProgressionTier:     tier,
			MapId:               a.mapID,
			QuestId:             a.questID,
			SpellId:             a.spellID,
			Profession:          a.profession,
			ExtendedCostId:      a.extCost,
			AchievementId:       a.achieve,
			ReputationFactionId: a.repFaction,
			ReputationRank:      a.repRank,
			ViaItemId:           r.viaItem(a, tiers),
		}
		if a.kindFromVia {
			base.Kind = r.bestKind(base.ViaItemId, tiers, 0)
		}
		base.Name = r.sourceName(a, base)

		if len(a.holders) == 0 {
			out = append(out, base)
			continue
		}
		if a.holderN > maxListedHolders {
			noun := "creatures"
			switch {
			case a.holderGO:
				noun = "objects"
			case a.extCost != 0 || a.kind == proto.CatalogSourceKind_CatalogSourceVendor || a.kind == proto.CatalogSourceKind_CatalogSourceReputation:
				noun = "vendors"
			}
			base.Name = joinLabel(a.label, fmt.Sprintf("%d %s", a.holderN, noun))
			out = append(out, base)
			continue
		}
		for _, h := range a.holders {
			s := googleProto.Clone(base).(*proto.CatalogSource)
			name := ""
			if a.holderGO {
				s.GameobjectId = h
				if g := r.gameObjects[h]; g != nil {
					name = g.Name
				}
			} else {
				s.NpcId = h
				if c := r.creatures[h]; c != nil {
					name = c.Name
				}
			}
			s.Name = joinLabel(a.label, name)
			out = append(out, s)
		}
	}
	return dedupeSources(out)
}

func joinLabel(prefix, name string) string {
	switch {
	case prefix == "":
		return name
	case name == "":
		return prefix
	}
	return prefix + ": " + name
}

func (r *resolver) sourceName(a *alt, s *proto.CatalogSource) string {
	switch {
	case a.questID != 0 || a.achieve != 0:
		return a.label
	case a.profession != proto.Profession_ProfessionUnknown:
		return joinLabel(a.profession.String(), a.label)
	case s.ViaItemId != 0 && len(a.holders) == 0:
		name := ""
		if it := r.items[s.ViaItemId]; it != nil {
			name = it.Name
		}
		return joinLabel(a.label, name)
	}
	return a.label
}

// viaItem is the needed item with the highest tier; ties go to the first that isn't itself a
// catalog item (the token, not the piece it upgrades). Unresolved items only win when nothing
// else resolves, e.g. one of several items casting the same spell.
func (r *resolver) viaItem(a *alt, tiers map[nodeID]int32) int32 {
	best, bestTier, bestCatalog := int32(0), int32(-2), true
	for _, id := range a.viaItems {
		t, ok := tiers[item(id)]
		if !ok || t == tierUnknown {
			t = -1
		}
		catalogItem := r.items[id] != nil && isCatalogItem(r.items[id])
		if t > bestTier || t == bestTier && bestCatalog && !catalogItem {
			best, bestTier, bestCatalog = id, t, catalogItem
		}
	}
	return best
}

// bestKind is the kind of an item's own best source, for things opened from or used from it.
// Follows containers in containers this deep.
const maxViaDepth = 4

func (r *resolver) bestKind(id int32, tiers map[nodeID]int32, depth int) proto.CatalogSourceKind {
	n := r.nodes[item(id)]
	if n == nil || depth > maxViaDepth {
		return proto.CatalogSourceKind_CatalogSourceUnknown
	}
	best, bestTier := proto.CatalogSourceKind_CatalogSourceUnknown, tierUnknown
	for _, a := range n.alts {
		kind := a.kind
		if a.kindFromVia {
			kind = r.bestKind(r.viaItem(a, tiers), tiers, depth+1)
		}
		if kind == proto.CatalogSourceKind_CatalogSourceUnknown {
			continue
		}
		if t := altTier(a, tiers); t < bestTier || t == bestTier && kind < best {
			best, bestTier = kind, t
		}
	}
	return best
}

func dedupeSources(sources []*proto.CatalogSource) []*proto.CatalogSource {
	slices.SortFunc(sources, compareSources)
	return slices.CompactFunc(sources, func(a, b *proto.CatalogSource) bool { return googleProto.Equal(a, b) })
}

// Past this many sources an item is a world drop or common vendor item, and the list only needs
// what the pool builder filters on: kind and tier. Anything from before tier 13 is there in every
// phase, so it keeps less.
const (
	maxSources     = 8
	maxOldSources  = 3
	firstWotlkTier = 13
)

// mergeSources folds a long source list by kind, tier and map, then by kind and tier alone if
// that's still too long.
func (r *resolver) mergeSources(sources []*proto.CatalogSource, limit int) []*proto.CatalogSource {
	if len(sources) <= limit {
		return sources
	}
	merge := func(sources []*proto.CatalogSource, byMap bool) []*proto.CatalogSource {
		type key struct {
			kind  proto.CatalogSourceKind
			tier  int32
			mapID int32
		}
		groups := map[key][]*proto.CatalogSource{}
		var order []key
		for _, s := range sources {
			k := key{kind: s.Kind, tier: s.ProgressionTier}
			if byMap {
				k.mapID = s.MapId
			}
			if groups[k] == nil {
				order = append(order, k)
			}
			groups[k] = append(groups[k], s)
		}
		var out []*proto.CatalogSource
		for _, k := range order {
			group := groups[k]
			if len(group) == 1 {
				out = append(out, group[0])
				continue
			}
			merged := &proto.CatalogSource{Kind: k.kind, ProgressionTier: k.tier, Name: fmt.Sprintf("%d sources", len(group))}
			if byMap && k.mapID != 0 {
				merged.MapId = k.mapID
				merged.Name = joinLabel(r.maps[k.mapID].Name, merged.Name)
			}
			out = append(out, merged)
		}
		return dedupeSources(out)
	}
	sources = merge(sources, true)
	if len(sources) > limit {
		sources = merge(sources, false)
	}
	return sources
}

func compareSources(a, b *proto.CatalogSource) int {
	return cmp.Or(
		cmp.Compare(a.ProgressionTier, b.ProgressionTier),
		cmp.Compare(a.Kind, b.Kind),
		cmp.Compare(a.MapId, b.MapId),
		cmp.Compare(a.NpcId, b.NpcId),
		cmp.Compare(a.GameobjectId, b.GameobjectId),
		cmp.Compare(a.QuestId, b.QuestId),
		cmp.Compare(a.SpellId, b.SpellId),
		cmp.Compare(a.ExtendedCostId, b.ExtendedCostId),
		cmp.Compare(a.AchievementId, b.AchievementId),
		cmp.Compare(a.ViaItemId, b.ViaItemId),
		cmp.Compare(a.ReputationFactionId, b.ReputationFactionId),
		cmp.Compare(a.ReputationRank, b.ReputationRank),
		cmp.Compare(a.Profession, b.Profession),
		strings.Compare(a.Name, b.Name),
	)
}

// contextLabel names a map and difficulty the way raiders say it, e.g. "Naxxramas 25".
func (r *resolver) contextLabel(c mapContext) string {
	m := r.maps[c.Map]
	name := m.Name
	if name == "" {
		name = fmt.Sprintf("Map %d", c.Map)
	}
	if m.Type == mapTypeRaid && r.maxPlayers[c] > 25 {
		return fmt.Sprintf("%s %d", name, r.maxPlayers[c])
	}
	switch sourceKind(m.Type, c.Difficulty, r.maxPlayers[c]) {
	case proto.CatalogSourceKind_CatalogSourceDungeonHeroic:
		return name + " Heroic"
	case proto.CatalogSourceKind_CatalogSourceRaid10:
		return name + " 10"
	case proto.CatalogSourceKind_CatalogSourceRaid25:
		return name + " 25"
	case proto.CatalogSourceKind_CatalogSourceRaid10Heroic:
		return name + " 10 Heroic"
	case proto.CatalogSourceKind_CatalogSourceRaid25Heroic:
		return name + " 25 Heroic"
	}
	return name
}

func sortedKeys[V any](m map[int32]V) []int32 {
	keys := make([]int32, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
