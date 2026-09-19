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
// (Check in rules.go, Gem in gems.go). CompilePool builds it; it's read-only after that, so
// goroutines can share one.
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

// maxItemProtoStats is the worldserver's MAX_ITEM_PROTO_STATS: mod-reforging refuses items with
// StatsCount 0 or at least this.
const maxItemProtoStats = 10

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
		Item:     item,
		Sockets:  p.sockets(&item),
		Reforges: reforgeOptions(item, p.catalog[item.ID]),
		NeedsSim: ItemHasEffect(item.ID) || item.WeaponDamageMax > 0,
		Catalog:  p.catalog[item.ID],
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

// reforgeOptions lists what mod-reforging allows: one stat pair from core's reforgeable list,
// taken off the item's own stats (no gems or enchant), into a stat the item doesn't have, on an
// item whose StatsCount is 1 to 9. Without a catalog row StatsCount is unknown, so only core's
// rule applies.
//
// The server only reforges template stats of the combined rating types. Core can't tell those
// apart: it folds flat equip spells into the item's stats, and keeps hit, crit and haste as a
// melee and a spell stat each. So a melee- or spell-only rating (the pre-3.0 stat types) doesn't
// count as the rating; a rating from an equip spell still slips through.
func reforgeOptions(item core.Item, row *proto.CatalogItem) []ReforgeOption {
	if row != nil && (row.StatsCount < 1 || row.StatsCount >= maxItemProtoStats) {
		return nil
	}
	var out []ReforgeOption
	for _, from := range core.ReforgeableStatTypes {
		for _, to := range core.ReforgeableStatTypes {
			reforge := &proto.ItemReforge{FromStatType: from, ToStatType: to}
			if !core.CanReforge(item.Stats, reforge) {
				continue
			}
			delta := core.ReforgeStats(item.Stats, reforge)
			if splitRating(item.Stats, delta) {
				continue
			}
			out = append(out, ReforgeOption{From: from, To: to, Stats: delta})
		}
	}
	return out
}

// splitRating is whether the stats a reforge takes from differ on the item, e.g. spell hit
// without melee hit: core takes the larger, but the server has no combined rating to take from.
func splitRating(base, delta stats.Stats) bool {
	from := -1.0
	for s, d := range delta {
		if d >= 0 {
			continue
		}
		if from >= 0 && base[s] != from {
			return true
		}
		from = base[s]
	}
	return false
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
