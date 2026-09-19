package optimizer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// Rule is an equip rule Check enforces. They follow the worldserver (Player::CanEquipItem,
// WorldSession::HandleSocketOpcode, mod-reforging) and never allow a loadout core's EquipItem
// would rearrange.
type Rule int

const (
	// A locked slot doesn't hold the seed's choice.
	RuleLocked Rule = iota + 1
	// The item doesn't go in that slot, or core would move it to another one: a ring or trinket in
	// the second slot with the first empty, a weapon in the off hand with the main hand empty, a
	// main-hand weapon in the off hand. Also an empty slot with an enchant, gems or a reforge.
	RuleSlot
	// Weapon combos: a two-hander, polearm or staff with anything in the off hand, an off-hand
	// weapon without dual wield, a two-hander there without Titan's Grip.
	RuleWeapons
	// The item or a gem needs a profession the target doesn't have.
	RuleProfession
	// The settings exclude the item or gem.
	RuleExcluded
	// The pool doesn't offer that item in that slot, or that gem.
	RulePool
	// The pool doesn't offer that enchant for that item in that slot.
	RuleEnchant
	// mod-reforging wouldn't allow that reforge on that item.
	RuleReforge
	// A gem sits in a socket the item doesn't have, or a meta gem and its socket don't match.
	RuleSocket
	// More copies of an item or gem than unique-equipped or maxcount allow.
	RuleUnique
	// More items and gems of one ItemLimitCategory than it allows, e.g. 4 jewelcrafter gems.
	RuleLimitGroup
	// A socketed meta gem's colors aren't met, so the server wouldn't activate it. The sim applies
	// metas regardless.
	RuleMeta
	// A final stat is under its floor.
	RuleFloor
)

var ruleNames = map[Rule]string{
	RuleLocked:     "locked slot",
	RuleSlot:       "slot",
	RuleWeapons:    "weapons",
	RuleProfession: "profession",
	RuleExcluded:   "excluded",
	RulePool:       "not in the pool",
	RuleEnchant:    "enchant",
	RuleReforge:    "reforge",
	RuleSocket:     "socket",
	RuleUnique:     "unique",
	RuleLimitGroup: "limit group",
	RuleMeta:       "inactive meta",
	RuleFloor:      "stat floor",
}

func (r Rule) String() string {
	if name, ok := ruleNames[r]; ok {
		return name
	}
	return fmt.Sprintf("rule %d", int(r))
}

// RuleError is the rule a loadout breaks, where.
type RuleError struct {
	Rule Rule
	// -1 for rules about the whole loadout (floors).
	Slot proto.ItemSlot
	// The item, gem or enchant it's about; 0 for floors.
	ID     int32
	Detail string
}

func (e *RuleError) Error() string {
	var where []string
	if e.Slot >= 0 {
		where = append(where, e.Slot.String())
	}
	if e.ID != 0 {
		where = append(where, fmt.Sprint(e.ID))
	}
	if len(where) == 0 {
		return fmt.Sprintf("%s: %s", e.Rule, e.Detail)
	}
	return fmt.Sprintf("%s (%s): %s", e.Rule, strings.Join(where, " "), e.Detail)
}

func ruleErr(rule Rule, slot proto.ItemSlot, id int32, format string, args ...any) *RuleError {
	return &RuleError{Rule: rule, Slot: slot, ID: id, Detail: fmt.Sprintf(format, args...)}
}

// Check returns nil when l breaks no rule, else a *RuleError naming the first. Floors build the
// target's character, about as costly as setting up one sim, so they come last and only when the
// settings have any; CheckGear skips them.
func (p *Pool) Check(l Loadout) error {
	if err := p.CheckGear(l); err != nil {
		return err
	}
	return p.checkFloors(l)
}

// CheckGear is Check without the stat floors. It's cheap enough for every move of a search.
func (p *Pool) CheckGear(l Loadout) error {
	var items [NumSlots]*core.Item
	for slot := range l.Items {
		item, err := p.checkSlot(l, proto.ItemSlot(slot))
		if err != nil {
			return err
		}
		items[slot] = item
	}
	if err := p.checkPlacement(items); err != nil {
		return err
	}
	if err := p.checkLimits(l, items); err != nil {
		return err
	}
	return p.checkMetas(l, items)
}

// checkSlot checks what's in one slot on its own, and returns its item (nil when empty).
func (p *Pool) checkSlot(l Loadout, slot proto.ItemSlot) (*core.Item, error) {
	c := l.Items[slot]
	if p.Locked[slot] && c != p.seed.Items[slot] {
		return nil, ruleErr(RuleLocked, slot, c.ItemID, "the slot is locked to the seed's choice")
	}
	if c.ItemID == 0 {
		if c != (ItemChoice{}) {
			return nil, ruleErr(RuleSlot, slot, 0, "an empty slot has an enchant, gems or a reforge")
		}
		return nil, nil
	}
	item, ok := p.item(slot, c.ItemID)
	if !ok {
		return nil, ruleErr(RulePool, slot, c.ItemID, "the item isn't in the database")
	}
	if err := p.fitsSlot(item, slot); err != nil {
		return nil, err
	}
	if p.Locked[slot] {
		return item, nil
	}

	cand := p.candidate(slot, c.ItemID)
	switch {
	case p.missingProfession(c.ItemID):
		return nil, ruleErr(RuleProfession, slot, c.ItemID, "the item needs %s", p.catalog[c.ItemID].RequiredProfession)
	case p.excluded[c.ItemID]:
		return nil, ruleErr(RuleExcluded, slot, c.ItemID, "the settings exclude the item")
	case cand == nil:
		return nil, ruleErr(RulePool, slot, c.ItemID, "the pool doesn't offer the item in this slot")
	}
	if c.Enchant != 0 && !slices.ContainsFunc(cand.Enchants, func(e EnchantOption) bool { return e.ID == c.Enchant }) {
		return nil, ruleErr(RuleEnchant, slot, c.Enchant, "the pool doesn't offer enchant %d for item %d here", c.Enchant, c.ItemID)
	}
	if (c.ReforgeFrom != 0 || c.ReforgeTo != 0) &&
		!slices.ContainsFunc(cand.Reforges, func(r ReforgeOption) bool { return r.From == c.ReforgeFrom && r.To == c.ReforgeTo }) {
		return nil, ruleErr(RuleReforge, slot, c.ItemID, "the server doesn't allow reforging %d into %d on this item", c.ReforgeFrom, c.ReforgeTo)
	}
	for i, id := range c.Gems {
		if id == 0 {
			continue
		}
		if i >= len(cand.Sockets) {
			return nil, ruleErr(RuleSocket, slot, id, "item %d has no socket %d", c.ItemID, i+1)
		}
		gem := p.gems[id]
		switch {
		case gem == nil && p.missingProfession(id):
			return nil, ruleErr(RuleProfession, slot, id, "the gem needs %s", p.catalog[id].RequiredProfession)
		case gem == nil && p.excluded[id]:
			return nil, ruleErr(RuleExcluded, slot, id, "the settings exclude the gem")
		case gem == nil:
			return nil, ruleErr(RulePool, slot, id, "the pool doesn't offer the gem")
		case (gem.Gem.Color == proto.GemColor_GemColorMeta) != (cand.Sockets[i] == proto.GemColor_GemColorMeta):
			return nil, ruleErr(RuleSocket, slot, id, "a %s gem doesn't fit socket %d, which is %s", gem.Gem.Color, i+1, cand.Sockets[i])
		}
	}
	return item, nil
}

// fitsSlot is whether the item can sit in the slot at all, whatever the other slots hold.
func (p *Pool) fitsSlot(item *core.Item, slot proto.ItemSlot) *RuleError {
	if item.Type != proto.ItemType_ItemTypeWeapon {
		if !slices.Contains(core.EligibleSlotsForItem(*item), slot) {
			return ruleErr(RuleSlot, slot, item.ID, "a %s item doesn't go here", item.Type)
		}
		return nil
	}
	shieldOrHeld := item.WeaponType == proto.WeaponType_WeaponTypeShield || item.WeaponType == proto.WeaponType_WeaponTypeOffHand
	switch slot {
	case proto.ItemSlot_ItemSlotMainHand:
		if shieldOrHeld || item.HandType == proto.HandType_HandTypeOffHand {
			return ruleErr(RuleSlot, slot, item.ID, "an off-hand item doesn't go in the main hand")
		}
		return nil
	case proto.ItemSlot_ItemSlotOffHand:
		switch {
		// before the shield check: EquipItem can move these to the main hand, shields included
		case item.HandType == proto.HandType_HandTypeMainHand || item.HandType == proto.HandType_HandTypeUnknown:
			return ruleErr(RuleSlot, slot, item.ID, "a main-hand weapon doesn't go in the off hand")
		case shieldOrHeld:
			return nil
		case !p.player.dualWield:
			return ruleErr(RuleWeapons, slot, item.ID, "a weapon in the off hand needs dual wield")
		case item.HandType == proto.HandType_HandTypeTwoHand && !p.player.titansGrip:
			return ruleErr(RuleWeapons, slot, item.ID, "a two-hander in the off hand needs Titan's Grip")
		case fillsBothHands(item):
			return ruleErr(RuleWeapons, slot, item.ID, "polearms, staves and fishing poles don't go in the off hand")
		}
		return nil
	}
	return ruleErr(RuleSlot, slot, item.ID, "a weapon doesn't go here")
}

// fillsBothHands: the server keeps the off hand empty next to a polearm (one-handed ones too),
// staff or fishing pole, Titan's Grip or not. Fishing poles are the sim's only weapons without a
// weapon type.
func fillsBothHands(item *core.Item) bool {
	switch item.WeaponType {
	case proto.WeaponType_WeaponTypePolearm, proto.WeaponType_WeaponTypeStaff, proto.WeaponType_WeaponTypeUnknown:
		return true
	}
	return false
}

// checkPlacement covers what NewEquipmentSet would rearrange (it fills rings, trinkets and
// one-handers first slot first) and the server's two-hander rules.
func (p *Pool) checkPlacement(items [NumSlots]*core.Item) error {
	for _, pair := range [][2]proto.ItemSlot{
		{proto.ItemSlot_ItemSlotFinger1, proto.ItemSlot_ItemSlotFinger2},
		{proto.ItemSlot_ItemSlotTrinket1, proto.ItemSlot_ItemSlotTrinket2},
	} {
		if items[pair[0]] == nil && items[pair[1]] != nil {
			return ruleErr(RuleSlot, pair[1], items[pair[1]].ID, "with %s empty, core would move it there", pair[0])
		}
	}

	mh, oh := items[proto.ItemSlot_ItemSlotMainHand], items[proto.ItemSlot_ItemSlotOffHand]
	if oh == nil {
		return nil
	}
	if mh == nil && (oh.HandType == proto.HandType_HandTypeOneHand || oh.HandType == proto.HandType_HandTypeTwoHand) &&
		oh.WeaponType != proto.WeaponType_WeaponTypeShield {
		return ruleErr(RuleSlot, proto.ItemSlot_ItemSlotOffHand, oh.ID, "with the main hand empty, core would move it there")
	}
	switch {
	case mh == nil:
	case fillsBothHands(mh):
		return ruleErr(RuleWeapons, proto.ItemSlot_ItemSlotOffHand, oh.ID, "nothing goes in the off hand with a polearm, staff or fishing pole")
	case mh.HandType == proto.HandType_HandTypeTwoHand && !p.player.titansGrip:
		return ruleErr(RuleWeapons, proto.ItemSlot_ItemSlotOffHand, oh.ID, "nothing goes in the off hand with a two-hander without Titan's Grip")
	}
	return nil
}

type socketedGem struct {
	id    int32
	color proto.GemColor
}

// socketed lists the gems in sockets the item has; a gem past them isn't there on the server.
func (p *Pool) socketed(c ItemChoice, item *core.Item) []socketedGem {
	sockets := p.numSockets(item)
	var out []socketedGem
	for i, id := range c.Gems {
		if id == 0 || i >= sockets {
			continue
		}
		if gem := p.gems[id]; gem != nil {
			out = append(out, socketedGem{id, gem.Gem.Color})
		} else if gem, ok := core.LookupGem(id); ok {
			out = append(out, socketedGem{id, gem.Color})
		}
	}
	return out
}

// itemLimit is how many copies of an item can be equipped; 0 means no limit.
func itemLimit(row *proto.CatalogItem) int {
	limit := int(row.GetMaxCount())
	if row.GetUniqueEquipped() {
		limit = 1
	}
	return limit
}

// checkLimits counts copies against unique-equipped and maxcount, and items and gems together
// against their ItemLimitCategory, as Player::CanEquipUniqueItem does. A socketed gem's maxcount
// doesn't matter: it's no longer an item.
func (p *Pool) checkLimits(l Loadout, items [NumSlots]*core.Item) error {
	itemCount, gemCount, groupCount := map[int32]int{}, map[int32]int{}, map[int32]int{}
	overGroup := func(slot proto.ItemSlot, id int32, row *proto.CatalogItem) error {
		category := row.GetLimitCategory()
		if category == 0 {
			return nil
		}
		groupCount[category]++
		if group := p.limitGroups[category]; group != nil && groupCount[category] > int(group.MaxEquipped) {
			return ruleErr(RuleLimitGroup, slot, id, "more than %d of %q", group.MaxEquipped, group.Name)
		}
		return nil
	}
	for s, c := range l.Items {
		slot := proto.ItemSlot(s)
		if items[slot] == nil {
			continue
		}
		row := p.catalog[c.ItemID]
		itemCount[c.ItemID]++
		if limit := itemLimit(row); limit > 0 && itemCount[c.ItemID] > limit {
			return ruleErr(RuleUnique, slot, c.ItemID, "at most %d of this item can be equipped", limit)
		}
		if err := overGroup(slot, c.ItemID, row); err != nil {
			return err
		}
		for _, gem := range p.socketed(c, items[slot]) {
			gemRow := p.catalog[gem.id]
			gemCount[gem.id]++
			if gemRow.GetUniqueEquipped() && gemCount[gem.id] > 1 {
				return ruleErr(RuleUnique, slot, gem.id, "the gem is unique-equipped")
			}
			if err := overGroup(slot, gem.id, gemRow); err != nil {
				return err
			}
		}
	}
	return nil
}

// gemColors is how a gem counts toward meta requirements: red, yellow, blue, by ColorIntersects,
// so an orange gem counts as red and yellow and a prismatic one as all three.
func gemColors(color proto.GemColor) [3]int32 {
	var out [3]int32
	for i, primary := range []proto.GemColor{proto.GemColor_GemColorRed, proto.GemColor_GemColorYellow, proto.GemColor_GemColorBlue} {
		if core.ColorIntersects(primary, color) {
			out[i] = 1
		}
	}
	return out
}

func metConstraint(c *proto.MetaColorConstraint, n [3]int32) bool {
	return c.Red*n[0]+c.Yellow*n[1]+c.Blue*n[2] >= c.MinTotal
}

// checkMetas counts every gem in a socket its item has, the extra ones included. A meta the pool
// has no condition for counts as active.
func (p *Pool) checkMetas(l Loadout, items [NumSlots]*core.Item) error {
	var n [3]int32
	type socketedMeta struct {
		slot proto.ItemSlot
		id   int32
	}
	var metas []socketedMeta
	for s, c := range l.Items {
		if items[s] == nil {
			continue
		}
		for _, gem := range p.socketed(c, items[s]) {
			if gem.color == proto.GemColor_GemColorMeta {
				metas = append(metas, socketedMeta{proto.ItemSlot(s), gem.id})
			}
			colors := gemColors(gem.color)
			for i := range n {
				n[i] += colors[i]
			}
		}
	}
	for _, meta := range metas {
		for _, c := range p.metas[meta.id].GetConstraints() {
			if !metConstraint(c, n) {
				return ruleErr(RuleMeta, meta.slot, meta.id, "needs %d*red + %d*yellow + %d*blue >= %d, has %d red, %d yellow, %d blue",
					c.Red, c.Yellow, c.Blue, c.MinTotal, n[0], n[1], n[2])
			}
		}
	}
	return nil
}

func (p *Pool) checkFloors(l Loadout) error {
	if len(p.floors) == 0 {
		return nil
	}
	sheet, err := p.finalStats(l)
	if err != nil {
		return err
	}
	for _, floor := range p.floors {
		if have := sheet[stats.Stat(floor.Stat)]; have < floor.MinValue {
			return ruleErr(RuleFloor, -1, 0, "%s is %.1f, under its floor of %.1f", stats.Stat(floor.Stat).StatName(), have, floor.MinValue)
		}
	}
	return nil
}

// finalStats is the target's character sheet in l: final stats with buffs, as core.ComputeStats
// reports them to the UI.
func (p *Pool) finalStats(l Loadout) (sheet stats.Stats, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("computing the target's stats: %v", e)
		}
	}()
	rsr := goproto.Clone(p.base).(*proto.RaidSimRequest)
	player := rsr.Raid.Parties[p.targetIndex/5].Players[p.targetIndex%5]
	player.Equipment = l.Equipment()
	player.RacialTraits = l.RacialTraits
	result := core.ComputeStats(&proto.ComputeStatsRequest{Raid: rsr.Raid, Encounter: rsr.Encounter})
	if result.ErrorResult != "" {
		return sheet, fmt.Errorf("computing the target's stats: %s", result.ErrorResult)
	}
	parties := result.GetRaidStats().GetParties()
	if p.targetIndex/5 >= len(parties) {
		return sheet, fmt.Errorf("computing the target's stats: no stats for party %d", p.targetIndex/5)
	}
	target := parties[p.targetIndex/5].GetPlayers()[p.targetIndex%5]
	return stats.FromFloatArray(target.GetFinalStats().GetStats()), nil
}
