package core

import (
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Mirrors the web server: one request's NewCharacter adds to the database while another's sims read
// it. Needs -race to catch a missing lock reliably.
func TestDatabaseConcurrentAddAndRead(t *testing.T) {
	// negative ids never clash with real items
	const (
		readItemID    = -1
		readGemID     = -2
		readEnchantID = -3
		firstAddedID  = -1000
		numAdds       = 2000
	)
	AddToDatabase(&proto.SimDatabase{
		Items: []*proto.SimItem{{
			Id:          readItemID,
			Type:        proto.ItemType_ItemTypeHead,
			Stats:       stats.Stats{stats.MeleeCrit: 40, stats.SpellCrit: 40}.ToFloatArray(),
			GemSockets:  []proto.GemColor{proto.GemColor_GemColorRed},
			ServerStats: []*proto.ItemStat{{StatType: 32, Value: 40}},
		}},
		Gems:     []*proto.SimGem{{Id: readGemID, Color: proto.GemColor_GemColorRed}},
		Enchants: []*proto.SimEnchant{{EffectId: readEnchantID}},
	})
	t.Cleanup(func() {
		dbMu.Lock()
		defer dbMu.Unlock()
		for id := int32(readItemID); id >= firstAddedID-numAdds; id-- {
			delete(itemsByID, id)
			delete(gemsByID, id)
			delete(enchantsByEffectID, id)
		}
	})

	spec := ItemSpec{
		ID:      readItemID,
		Enchant: readEnchantID,
		Gems:    []int32{readGemID},
		Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36},
	}

	stop := make(chan struct{})
	var ready, readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		ready.Add(1)
		readers.Add(1)
		go func() {
			defer readers.Done()
			ready.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if item := NewItem(spec, nil); item.ID != readItemID || item.Enchant.EffectID != readEnchantID || item.Gems[0].ID != readGemID {
					t.Errorf("NewItem(%v) = %+v", spec, item)
					return
				}
				if _, ok := LookupItem(readItemID); !ok {
					t.Error("LookupItem lost an item")
					return
				}
			}
		}()
	}

	ready.Wait()
	for i := int32(0); i < numAdds; i++ {
		id := firstAddedID - i
		AddToDatabase(&proto.SimDatabase{
			Items:    []*proto.SimItem{{Id: id}},
			Gems:     []*proto.SimGem{{Id: id}},
			Enchants: []*proto.SimEnchant{{EffectId: id}},
		})
	}
	close(stop)
	readers.Wait()

	for _, id := range []int32{firstAddedID, firstAddedID - numAdds + 1} {
		if _, ok := LookupItem(id); !ok {
			t.Errorf("item %d never made it into the database", id)
		}
		if _, ok := LookupGem(id); !ok {
			t.Errorf("gem %d never made it into the database", id)
		}
		if _, ok := LookupEnchant(id); !ok {
			t.Errorf("enchant %d never made it into the database", id)
		}
	}
}
