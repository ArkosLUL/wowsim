package optimizer

import (
	"fmt"
	"slices"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/shaman"
	"github.com/wowsims/wotlk/sim/warrior"
)

// Pool is a request's candidate pool compiled for the search: what each slot can hold, with its
// stats, sockets, enchants and reforges, the gems, and the equip rules that tie slots together
// (Check in rules.go, Gem in gems.go). CompilePool builds it; it's read-only after that, apart from
// its locked sheet memo, so goroutines can share one.
//
// A loadout the search builds from Slots and Gems still has to pass Check: the per-slot lists
// don't know about the other slots (weapon combos, unique items, limit groups, meta colors).
type Pool struct {
	// Candidates per slot, by proto.ItemSlot, in pool order. An empty slot is always allowed. A
	// locked slot lists only the seed's item, with only its enchant and reforge.
	Slots [NumSlots][]*Candidate
	// Gems the search can socket, in pool order. Meta gems only fit meta sockets.
	Gems []*GemCandidate
	// Slots that keep the seed's choice.
	Locked [NumSlots]bool

	seed        Loadout
	player      playerRules
	index       [NumSlots]map[int32]*Candidate
	gems        map[int32]*GemCandidate
	excluded    map[int32]bool
	catalog     map[int32]*proto.CatalogItem
	limitGroups map[int32]*proto.LimitGroup
	metas       map[int32]*proto.MetaGemCondition
	floors      []*proto.StatMinimum
	base        *proto.RaidSimRequest
	targetIndex int
	reforging   *core.Reforging
	sheets      sheetMemo
}

// Candidate is one item one slot can hold.
type Candidate struct {
	// Core's copy of the item: its own stats (no gems, enchant or reforge), sockets, socket bonus,
	// weapon fields and set name.
	Item core.Item
	// Sockets in ItemChoice.Gems order: the item's own, then the prismatic one a belt buckle or
	// Blacksmithing adds, when it applies and the item has under 3. Only the item's own count for
	// its socket bonus.
	Sockets []proto.GemColor
	// Enchants the pool offers for it in this slot. Unenchanted is always allowed.
	Enchants []EnchantOption
	// Reforges the server allows on it. Not reforging is always allowed.
	Reforges []ReforgeOption
	// The item is worth something its stats don't show, an effect or weapon damage, so the
	// response curves can't price it (see NeedsSim).
	NeedsSim bool
	// nil when the pool has no catalog row for it.
	Catalog *proto.CatalogItem

	// The run's mod-reforging config, for pricing a reforge Reforges doesn't list.
	reforging *core.Reforging
}

type EnchantOption struct {
	// SimEnchant effect id.
	ID    int32
	Stats stats.Stats
	// It has an effect beyond its stats.
	NeedsSim bool
}

// ReforgeOption is one reforge and the stat change it makes on its item.
type ReforgeOption struct {
	// Server ItemModType ids, as in ItemChoice.
	From, To int32
	// Minus the moved amount on From, plus it on To.
	Stats stats.Stats
}

type GemCandidate struct {
	Gem core.Gem
	// It has an effect beyond its stats, e.g. a meta's crit damage.
	NeedsSim bool
	// nil when the pool has no catalog row for it.
	Catalog *proto.CatalogItem
}

// playerRules is what the equip rules need to know about the target.
type playerRules struct {
	dualWield     bool
	titansGrip    bool
	blacksmithing bool
	professions   []proto.Profession
}

// maxServerSockets is the worldserver's MAX_GEM_SOCKETS: gems go in 3 enchant slots, so an item
// with 3 sockets of its own has no room for a buckle or blacksmith socket.
const maxServerSockets = 3

// CompilePool builds r's pool. It drops what the target can never use: excluded items and gems,
// items and gems needing a profession it lacks, items that don't fit their slot, and off-hands it
// can't wield (dual wield, Titan's Grip).
func CompilePool(r *Request) (*Pool, error) {
	player, err := newPlayerRules(r.Target())
	if err != nil {
		return nil, err
	}
	p := &Pool{
		seed:        r.Seed,
		player:      player,
		gems:        map[int32]*GemCandidate{},
		excluded:    map[int32]bool{},
		catalog:     r.Catalog,
		limitGroups: r.LimitGroups,
		metas:       r.MetaConditions,
		floors:      r.Settings.GetStatMinimums(),
		base:        r.Base,
		targetIndex: r.TargetIndex,
		reforging:   &core.NewServerSettings(r.Base.GetEncounter().GetServerSettings()).Reforging,
	}
	for _, id := range r.Settings.GetExcludedItemIds() {
		p.excluded[id] = true
	}
	for _, slot := range r.Settings.GetLockedSlots() {
		if slot < 0 || int(slot) >= NumSlots {
			return nil, fmt.Errorf("locked slot %d doesn't exist", slot)
		}
		p.Locked[slot] = true
	}
	for slot := range p.index {
		p.index[slot] = map[int32]*Candidate{}
	}

	for _, slotPool := range r.Pool.GetSlots() {
		slot := slotPool.Slot
		if slot < 0 || int(slot) >= NumSlots {
			return nil, fmt.Errorf("pool slot %d doesn't exist", slot)
		}
		if p.Locked[slot] {
			continue
		}
		enchants := map[int32][]int32{}
		for _, options := range slotPool.EnchantOptions {
			enchants[options.ItemId] = append(enchants[options.ItemId], options.EnchantIds...)
		}
		for _, id := range slotPool.ItemIds {
			if p.excluded[id] || p.index[slot][id] != nil {
				continue
			}
			item, ok := core.LookupItem(id)
			if !ok {
				return nil, fmt.Errorf("pool slot %s: item %d isn't in the database", slot, id)
			}
			if p.fitsSlot(&item, slot) != nil || p.missingProfession(id) {
				continue
			}
			c := p.newCandidate(item)
			for _, enchantID := range enchants[id] {
				if enchantID == 0 || slices.ContainsFunc(c.Enchants, func(e EnchantOption) bool { return e.ID == enchantID }) {
					continue
				}
				enchant, ok := core.LookupEnchant(enchantID)
				if !ok {
					return nil, fmt.Errorf("pool slot %s: enchant %d for item %d isn't in the database", slot, enchantID, id)
				}
				c.Enchants = append(c.Enchants, EnchantOption{ID: enchantID, Stats: enchant.Stats, NeedsSim: EnchantHasEffect(enchantID)})
			}
			p.Slots[slot] = append(p.Slots[slot], c)
			p.index[slot][id] = c
		}
	}

	for slot := range p.Slots {
		choice := r.Seed.Items[slot]
		if !p.Locked[slot] || choice.ItemID == 0 {
			continue
		}
		item, ok := core.LookupItem(choice.ItemID)
		if !ok {
			return nil, fmt.Errorf("locked slot %s: item %d isn't in the database", proto.ItemSlot(slot), choice.ItemID)
		}
		c := p.newCandidate(item)
		c.Reforges = slices.DeleteFunc(c.Reforges, func(o ReforgeOption) bool { return o.From != choice.ReforgeFrom || o.To != choice.ReforgeTo })
		if choice.Enchant != 0 {
			enchant, _ := core.LookupEnchant(choice.Enchant)
			c.Enchants = []EnchantOption{{ID: choice.Enchant, Stats: enchant.Stats, NeedsSim: EnchantHasEffect(choice.Enchant)}}
		}
		p.Slots[slot] = []*Candidate{c}
		p.index[slot][choice.ItemID] = c
	}

	for _, id := range r.Pool.GetGemIds() {
		if p.excluded[id] || p.gems[id] != nil || p.missingProfession(id) {
			continue
		}
		gem, ok := core.LookupGem(id)
		if !ok {
			return nil, fmt.Errorf("pool gem %d isn't in the database", id)
		}
		g := &GemCandidate{Gem: gem, NeedsSim: ItemHasEffect(id), Catalog: p.catalog[id]}
		p.Gems = append(p.Gems, g)
		p.gems[id] = g
	}
	return p, nil
}

func newPlayerRules(target *proto.Player) (pr playerRules, err error) {
	pr.professions = core.ProtoToProfessions(target)
	pr.blacksmithing = slices.Contains(pr.professions, proto.Profession_Blacksmithing)

	// FillTalentsProto panics on a string that doesn't fit the class's trees
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("target's talents %q: %v", target.TalentsString, e)
		}
	}()
	switch target.Class {
	case proto.Class_ClassWarrior:
		talents := &proto.WarriorTalents{}
		core.FillTalentsProto(talents.ProtoReflect(), target.TalentsString, warrior.TalentTreeSizes)
		pr.dualWield, pr.titansGrip = true, talents.TitansGrip
	case proto.Class_ClassShaman:
		talents := &proto.ShamanTalents{}
		core.FillTalentsProto(talents.ProtoReflect(), target.TalentsString, shaman.TalentTreeSizes)
		pr.dualWield = talents.DualWield
	case proto.Class_ClassRogue, proto.Class_ClassHunter, proto.Class_ClassDeathknight:
		pr.dualWield = true
	}
	return pr, nil
}

func (p *Pool) newCandidate(item core.Item) *Candidate {
	return &Candidate{
		Item:      item,
		Sockets:   p.sockets(&item),
		Reforges:  reforgeOptions(item, p.reforging),
		NeedsSim:  ItemHasEffect(item.ID) || item.WeaponDamageMax > 0,
		Catalog:   p.catalog[item.ID],
		reforging: p.reforging,
	}
}

// sockets adds the extra prismatic socket: a belt buckle fits any belt, Blacksmithing socketing
// only bracers and gloves (and only works for a blacksmith).
func (p *Pool) sockets(item *core.Item) []proto.GemColor {
	sockets := slices.Clone(item.GemSockets)
	if p.hasExtraSocket(item) {
		sockets = append(sockets, proto.GemColor_GemColorPrismatic)
	}
	return sockets[:min(len(sockets), MaxGems)]
}

func (p *Pool) numSockets(item *core.Item) int {
	n := len(item.GemSockets)
	if p.hasExtraSocket(item) {
		n++
	}
	return min(n, MaxGems)
}

func (p *Pool) hasExtraSocket(item *core.Item) bool {
	if len(item.GemSockets) >= maxServerSockets {
		return false
	}
	return item.Type == proto.ItemType_ItemTypeWaist ||
		p.player.blacksmithing && (item.Type == proto.ItemType_ItemTypeWrist || item.Type == proto.ItemType_ItemTypeHands)
}

func (p *Pool) missingProfession(id int32) bool {
	needs := p.catalog[id].GetRequiredProfession()
	return needs != proto.Profession_ProfessionUnknown && !slices.Contains(p.player.professions, needs)
}

// reforgeOptions lists every reforge mod-reforging allows on the item under the run's config: one
// pair from its reforgeable stat list, taken off one of the item's own item_template stats into one
// it doesn't have. core.ReforgeStats has the rule; an item the item database has no server stats
// for gets no reforges, as it would on the server.
func reforgeOptions(item core.Item, reforging *core.Reforging) []ReforgeOption {
	var out []ReforgeOption
	for _, from := range reforging.StatTypes {
		for _, to := range reforging.StatTypes {
			delta := core.ReforgeStats(&item, &proto.ItemReforge{FromStatType: from, ToStatType: to}, reforging)
			if delta == (stats.Stats{}) {
				continue
			}
			out = append(out, ReforgeOption{From: from, To: to, Stats: delta})
		}
	}
	return out
}

// candidate is the pool's entry for an item in a slot, or nil.
func (p *Pool) candidate(slot proto.ItemSlot, id int32) *Candidate {
	return p.index[slot][id]
}

// item is the pool's copy of a slot's item, else core's. The copy skips the database lock, which
// matters since the search checks and regems on every move.
func (p *Pool) item(slot proto.ItemSlot, id int32) (*core.Item, bool) {
	if c := p.candidate(slot, id); c != nil {
		return &c.Item, true
	}
	item, ok := core.LookupItem(id)
	return &item, ok
}
