package optimizer

import (
	"math"
	"math/rand"
	"slices"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func linearValue(weights stats.Stats) func(stats.Stats) float64 {
	return func(s stats.Stats) float64 {
		var v float64
		for i := range s {
			v += weights[i] * s[i]
		}
		return v
	}
}

// gemValue is what a slot's gems and socket bonus add, by core's own TotalStats: the item with its
// gems, minus the item, its enchant and its meta gems.
func gemValue(l Loadout, slots []proto.ItemSlot, value func(stats.Stats) float64) float64 {
	var total float64
	for _, slot := range slots {
		c := l.Items[slot]
		if c.ItemID == 0 {
			continue
		}
		item := core.NewItem(c.CoreSpec(), nil)
		base := item.Stats.Add(item.Enchant.Stats)
		for _, gem := range item.Gems {
			if gem.Color == proto.GemColor_GemColorMeta {
				base = base.Add(gem.Stats)
			}
		}
		total += value(item.TotalStats()) - value(base)
	}
	return total
}

func countGems(l Loadout, match func(int32) bool) int {
	n := 0
	for _, c := range l.Items {
		for _, id := range c.Gems {
			if id != 0 && match(id) {
				n++
			}
		}
	}
	return n
}

// The DP's pick is the best of every gemming that passes CheckGear, by core's own stat totals.
func TestGemMatchesBruteForce(t *testing.T) {
	regem := []proto.ItemSlot{proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotWrist, proto.ItemSlot_ItemSlotWaist}
	options := []int32{0, rtRed, rtYellow, rtBlue, rtOrange, rtPrismatic, rtJCRed, rtUniqueGem}
	p := rtPool(t, func(req *proto.OptimizeGearRequest) {
		req.Pool.GemIds = append([]int32{rtMeta2Blue, rtMetaRYB, rtMeta3Red, rtMetaRedGtB}, options[1:]...)
	})
	base := rtLoadout()
	// two jewelcrafter gems already sit in the legs, so one more fits
	base.Items[proto.ItemSlot_ItemSlotLegs] = ItemChoice{ItemID: rtLegs, Gems: rtGems(rtJCRed, rtJCRed)}

	type position struct {
		slot   proto.ItemSlot
		socket int
	}
	var positions []position
	for _, slot := range regem {
		for i, color := range p.candidate(slot, base.Items[slot].ItemID).Sockets {
			if color != proto.GemColor_GemColorMeta {
				positions = append(positions, position{slot, i})
			}
		}
	}
	if len(positions) != 5 {
		t.Fatalf("%d positions, want 5", len(positions))
	}

	rng := rand.New(rand.NewSource(7))
	for _, meta := range []int32{0, rtMetaRYB, rtMeta2Blue, rtMeta3Red, rtMetaRedGtB} {
		for round := 0; round < 2; round++ {
			var weights stats.Stats
			for _, s := range []stats.Stat{stats.Strength, stats.MeleeCrit, stats.Stamina, stats.Agility} {
				weights[s] = rng.Float64()*2 - 0.4
			}
			value := linearValue(weights)
			l := base
			l.Items[proto.ItemSlot_ItemSlotHead].Gems[0] = meta

			best, found := math.Inf(-1), false
			var try func(k int, l Loadout)
			try = func(k int, l Loadout) {
				if k == len(positions) {
					if p.CheckGear(l) == nil {
						found = true
						best = max(best, gemValue(l, regem, value))
					}
					return
				}
				for _, id := range options {
					l.Items[positions[k].slot].Gems[positions[k].socket] = id
					try(k+1, l)
				}
			}
			try(0, l)

			got, gotValue, err := p.Gem(l, regem, value)
			if !found {
				if err == nil {
					t.Errorf("meta %d: no gemming passes, but Gem found %v", meta, got)
				}
				continue
			}
			if err != nil {
				t.Fatalf("meta %d, weights %v: %v", meta, weights, err)
			}
			if err := p.CheckGear(got); err != nil {
				t.Errorf("meta %d: Gem's pick breaks a rule: %v", meta, err)
			}
			if math.Abs(gotValue-best) > 1e-9 || math.Abs(gemValue(got, regem, value)-best) > 1e-9 {
				t.Errorf("meta %d, weights %v: Gem's value %.4f (by core %.4f), brute force %.4f", meta, weights, gotValue,
					gemValue(got, regem, value), best)
			}
		}
	}
}

func TestGem(t *testing.T) {
	const head, wrist, hands, waist, legs = proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotWrist, proto.ItemSlot_ItemSlotHands,
		proto.ItemSlot_ItemSlotWaist, proto.ItemSlot_ItemSlotLegs
	all := []proto.ItemSlot{head, wrist, hands, waist, legs}
	strength := linearValue(stats.Stats{stats.Strength: 1, stats.MeleeCrit: 0.5, stats.Stamina: 0.1})
	withLegs := func(l *Loadout) { l.Items[legs] = ItemChoice{ItemID: rtLegs} }
	withHands := func(l *Loadout) { l.Items[hands] = ItemChoice{ItemID: rtHands} }
	isJC := func(id int32) bool { return id == rtJCRed || id == rtJCYellow }

	tests := []struct {
		name    string
		pool    func(*proto.OptimizeGearRequest)
		change  func(*Loadout)
		slots   []proto.ItemSlot
		wantErr bool
		check   func(t *testing.T, before, after Loadout)
	}{
		{name: "fills every socket with the best gems, 3 of them jewelcrafter", change: withLegs, slots: all,
			check: func(t *testing.T, _, after Loadout) {
				if n := countGems(after, isJC); n != 3 {
					t.Errorf("%d jewelcrafter gems, want 3", n)
				}
				if n := countGems(after, func(id int32) bool { return id == rtUniqueGem }); n > 1 {
					t.Errorf("%d unique gems", n)
				}
			}},
		{name: "no jewelcrafting, no jewelcrafter gems", pool: func(req *proto.OptimizeGearRequest) {
			rtTarget(req).Professions = nil
		}, change: withLegs, slots: all, check: func(t *testing.T, _, after Loadout) {
			if n := countGems(after, isJC); n != 0 {
				t.Errorf("%d jewelcrafter gems", n)
			}
		}},
		{name: "blacksmith sockets get filled", pool: withBlacksmithing, change: withHands, slots: all,
			check: func(t *testing.T, _, after Loadout) {
				if after.Items[wrist].Gems[1] == 0 || after.Items[hands].Gems[0] == 0 {
					t.Errorf("wrist %v, hands %v: want the blacksmith sockets filled", after.Items[wrist].Gems, after.Items[hands].Gems)
				}
			}},
		{name: "no blacksmith sockets without blacksmithing", change: withHands, slots: all,
			check: func(t *testing.T, _, after Loadout) {
				if after.Items[wrist].Gems[1] != 0 || after.Items[hands].Gems[0] != 0 {
					t.Errorf("wrist %v, hands %v: want no blacksmith gems", after.Items[wrist].Gems, after.Items[hands].Gems)
				}
			}},
		{name: "2-blue meta takes blue-counting gems", change: func(l *Loadout) {
			l.Items[head].Gems[0] = rtMeta2Blue
		}, slots: all, check: func(t *testing.T, _, after Loadout) {
			blue := countGems(after, func(id int32) bool { return gemColors(lookupColor(id))[2] == 1 })
			if blue < 2 {
				t.Errorf("%d blue-counting gems, want 2", blue)
			}
		}},
		{name: "one slot: the rest stays and counts", change: func(l *Loadout) {
			l.Items[head].Gems = rtGems(rtMeta2Blue, rtBlue, rtRed)
			l.Items[waist].Gems = rtGems(rtRed, rtRed)
		}, slots: []proto.ItemSlot{wrist}, check: func(t *testing.T, before, after Loadout) {
			for slot := range after.Items {
				if slot != int(wrist) && after.Items[slot] != before.Items[slot] {
					t.Errorf("slot %s changed", proto.ItemSlot(slot))
				}
			}
			if id := after.Items[wrist].Gems[0]; gemColors(lookupColor(id))[2] != 1 {
				t.Errorf("wrist gem %d doesn't count as blue, which the meta needs", id)
			}
		}},
		{name: "meta the sockets can't activate", change: func(l *Loadout) {
			l.Items[head].Gems = rtGems(rtMeta3Red, rtBlue, rtBlue)
			l.Items[waist].Gems = rtGems(rtBlue, rtBlue)
		}, slots: []proto.ItemSlot{wrist}, wantErr: true},
		{name: "locked slots keep their gems", pool: func(req *proto.OptimizeGearRequest) {
			rtTarget(req).Equipment.Items[waist] = &proto.ItemSpec{Id: rtWaist, Gems: []int32{rtBlue}}
			req.Settings.LockedSlots = []proto.ItemSlot{waist}
		}, change: func(l *Loadout) { l.Items[waist] = ItemChoice{ItemID: rtWaist, Gems: rtGems(rtBlue)} }, slots: all,
			check: func(t *testing.T, _, after Loadout) {
				if after.Items[waist].Gems != rtGems(rtBlue) {
					t.Errorf("locked waist gems = %v", after.Items[waist].Gems)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := rtPool(t, tt.pool)
			before := rtLoadout()
			if tt.change != nil {
				tt.change(&before)
			}
			after, value, err := p.Gem(before, tt.slots, strength)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Gem = %v, want an error", after)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := p.CheckGear(after); err != nil {
				t.Errorf("Gem's pick breaks a rule: %v", err)
			}
			regemmed := slices.DeleteFunc(slices.Clone(tt.slots), func(s proto.ItemSlot) bool { return p.Locked[s] })
			if got := gemValue(after, regemmed, strength); math.Abs(got-value) > 1e-9 {
				t.Errorf("Gem says its gems are worth %.3f, core's totals say %.3f", value, got)
			}
			if after.Items[head].Gems[0] != before.Items[head].Gems[0] {
				t.Error("the meta gem changed")
			}
			if tt.check != nil {
				tt.check(t, before, after)
			}
		})
	}
}

// With every gem worth less than nothing, only what the meta needs goes in: one prismatic gem, in a
// socket that doesn't complete a costly socket bonus.
func TestGemLeavesSocketsEmpty(t *testing.T) {
	p := rtPool(t, nil)
	l := rtLoadout()
	l.Items[proto.ItemSlot_ItemSlotLegs] = ItemChoice{ItemID: rtLegs}
	negative := linearValue(stats.Stats{stats.Strength: -1, stats.MeleeCrit: -1, stats.Stamina: -1, stats.Agility: -1})
	slots := []proto.ItemSlot{proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotWrist, proto.ItemSlot_ItemSlotWaist, proto.ItemSlot_ItemSlotLegs}
	after, value, err := p.Gem(l, slots, negative)
	if err != nil {
		t.Fatal(err)
	}
	if n := countGems(after, func(id int32) bool { return id != rtMetaRYB }); n != 1 || countGems(after, func(id int32) bool { return id == rtPrismatic }) != 1 ||
		math.Abs(value+10) > 1e-9 {
		t.Errorf("gems %v for %.1f, want one prismatic gem for -10", after.Items, value)
	}
}

func lookupColor(id int32) proto.GemColor {
	gem, _ := core.LookupGem(id)
	return gem.Color
}

// Limited and unique gems no better than a free gem of the same colors never make the cut.
func TestGemOptionsPrune(t *testing.T) {
	p := rtPool(t, nil)
	value := linearValue(stats.Stats{stats.Strength: 1})
	var ids []int32
	for _, o := range p.gemOptions(value, 4) {
		ids = append(ids, o.id)
	}
	// red: the jewelcrafter one beats the plain one; orange and purple count as red too but
	// differ in their other colors, so they stay
	for _, want := range []int32{rtRed, rtJCRed, rtOrange, rtPurple, rtUniqueGem} {
		if !slices.Contains(ids, want) {
			t.Errorf("options %v lack %d", ids, want)
		}
	}
	// the unique prismatic gem (10 str) beats the free prismatic one (5 str) but the free one stays
	if !slices.Contains(ids, rtPrismatic) {
		t.Errorf("options %v lack the free prismatic gem", ids)
	}

	stamina := linearValue(stats.Stats{stats.Stamina: 1})
	ids = ids[:0]
	for _, o := range p.gemOptions(stamina, 4) {
		ids = append(ids, o.id)
	}
	// by stamina the free prismatic gem (5) is below the unique one (10), and both jewelcrafter
	// gems (0) are no better than a free gem of their color
	if slices.Contains(ids, rtJCRed) || slices.Contains(ids, rtJCYellow) {
		t.Errorf("options %v keep a jewelcrafter gem worth nothing", ids)
	}
}
