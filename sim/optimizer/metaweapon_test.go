package optimizer

import (
	"slices"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Two one-handers for the meta test: the socketed one carries the red gem the head's meta needs, the
// plain one has no sockets and swings harder.
const (
	mwSocketed = 9781101
	mwPlain    = 9781102
)

var mwOnce sync.Once

func mwRegister() {
	ohpRegister()
	mwOnce.Do(func() {
		socketed := kaWeapon(mwSocketed, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 100, 160, 2.6, stats.Stats{})
		socketed.GemSockets = []proto.GemColor{proto.GemColor_GemColorRed}
		core.AddToDatabase(&proto.SimDatabase{Items: []*proto.SimItem{
			socketed,
			kaWeapon(mwPlain, proto.HandType_HandTypeOneHand, proto.WeaponType_WeaponTypeSword, 200, 300, 2.6, stats.Stats{stats.Strength: 20}),
		}})
	})
}

// A main hand without sockets can replace the one whose red gem keeps the head's meta active: the
// move takes the red gem to a socket of the head, whose own socket held a yellow one, and says so in
// its changes. A change to the main hand's own sockets alone can't pass.
func TestRunnersUpMoveGemsToKeepTheMetaWhenAWeaponLeaves(t *testing.T) {
	mwRegister()
	req := ohpRequest(mwSocketed, 0, []int32{mwSocketed, mwPlain}, nil)
	head, mh := proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotMainHand
	req.Settings.LockedSlots = slices.DeleteFunc(req.Settings.LockedSlots, func(s proto.ItemSlot) bool { return s == head })
	req.Pool.Slots = append(req.Pool.Slots, &proto.SlotPool{Slot: head, ItemIds: []int32{kaHeadMeta}})
	req.Pool.GemIds = []int32{kaRed, kaYellow, kaMetaProc, kaMetaPlain}
	req.Pool.MetaConditions = []*proto.MetaGemCondition{
		{GemId: kaMetaProc, Constraints: []*proto.MetaColorConstraint{{Red: 1, MinTotal: 1}}},
	}
	equipment := req.Base.Raid.Parties[0].Players[0].Equipment
	equipment.Items[head] = &proto.ItemSpec{Id: kaHeadMeta, Gems: []int32{kaMetaProc, kaYellow}}
	equipment.Items[mh] = &proto.ItemSpec{Id: mwSocketed, Gems: []int32{kaRed}}

	rn, s := ohpRun(t, req)
	if err := rn.pool.CheckGear(rn.best); err != nil {
		t.Fatalf("the seed breaks a rule: %v", err)
	}
	se := newSearcher(s, rn)
	st := se.newState(rn.best)

	if se.apply(st.clone(), change{mh, ItemChoice{ItemID: mwPlain}}) {
		t.Fatalf("the plain weapon fits without moving a gem, so this tests nothing")
	}
	var found []change
	for _, changes := range se.runnersUp(st, mh, candidatesPerSlot, se.all[mh]) {
		if changes[0].choice.ItemID == mwPlain {
			found = changes
		}
	}
	if found == nil {
		t.Fatalf("the plain weapon isn't among the main hand's runners-up")
	}
	l := st.l
	for _, ch := range found {
		l.Items[ch.slot] = ch.choice
	}
	if err := se.check(l); err != nil {
		t.Errorf("the move breaks a rule: %v", err)
	}
	if !slices.Contains(l.Items[head].Gems[:], int32(kaRed)) {
		t.Errorf("the head holds gems %v, want a red one for the meta", l.Items[head].Gems)
	}
	if len(found) != 2 || found[1].slot != head {
		t.Errorf("the move changes %v, want the main hand and the head's gems", found)
	}
}
