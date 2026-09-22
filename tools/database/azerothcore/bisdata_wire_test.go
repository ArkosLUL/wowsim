package azerothcore

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func TestEncodeBlock(t *testing.T) {
	block := BisBlock{Slots: []BisSlot{
		{Slot: proto.ItemSlot_ItemSlotHead, Items: []int32{51227, 50712, 50713, 51866}, Enchant: 59954,
			Gems: []int32{41398, 40111}, ReforgeFrom: 31, ReforgeTo: 37, Delta: 142, HasDelta: true},
		{Slot: proto.ItemSlot_ItemSlotNeck, Items: []int32{50633}},
		{Slot: proto.ItemSlot_ItemSlotBack, Items: []int32{47545, 51933}, Enchant: 47898, Delta: -3, HasDelta: true},
		{Slot: proto.ItemSlot_ItemSlotRanged, Items: []int32{50733}, ReforgeFrom: 32, ReforgeTo: 36},
	}}
	got, err := EncodeBlock(block)
	if err != nil {
		t.Fatal(err)
	}
	want := "0:51227,50712,50713,51866(e59954,g41398,g40111,r31-37)+142;1:50633;3:47545,51933(e47898)+-3;16:50733(r32-36)"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestBlockRoundTrip(t *testing.T) {
	var everySlot BisBlock
	for slot := proto.ItemSlot_ItemSlotHead; slot <= proto.ItemSlot_ItemSlotRanged; slot++ {
		everySlot.Slots = append(everySlot.Slots, BisSlot{Slot: slot, Items: []int32{40000 + int32(slot), 41000, 42000, 43000, 44000, 45000},
			Enchant: 60000, Gems: []int32{40111, 40117, 41398}, ReforgeFrom: 13, ReforgeTo: 31, Delta: int32(slot) * 7, HasDelta: true})
	}

	for _, tc := range []struct {
		comment string
		block   BisBlock
	}{
		{"every slot, full", everySlot},
		{"one item, nothing else", BisBlock{Slots: []BisSlot{{Slot: proto.ItemSlot_ItemSlotTrinket2, Items: []int32{50363}}}}},
		{"gems only", BisBlock{Slots: []BisSlot{{Slot: proto.ItemSlot_ItemSlotWaist, Items: []int32{50620}, Gems: []int32{40125}}}}},
		{"zero delta still has a rank 2", BisBlock{Slots: []BisSlot{{Slot: proto.ItemSlot_ItemSlotFinger1, Items: []int32{50402, 50618}, HasDelta: true}}}},
		{"negative delta", BisBlock{Slots: []BisSlot{{Slot: proto.ItemSlot_ItemSlotFinger2, Items: []int32{50402, 50618}, Delta: -12, HasDelta: true}}}},
		{"gaps between slots", BisBlock{Slots: []BisSlot{
			{Slot: proto.ItemSlot_ItemSlotHead, Items: []int32{1}},
			{Slot: proto.ItemSlot_ItemSlotMainHand, Items: []int32{2}, Enchant: 3},
		}}},
	} {
		payload, err := EncodeBlock(tc.block)
		if err != nil {
			t.Errorf("%s: %v", tc.comment, err)
			continue
		}
		if strings.ContainsAny(payload, "~|\t") {
			t.Errorf("%s: %q holds a wire separator", tc.comment, payload)
		}
		got, err := DecodeBlock(payload)
		if err != nil {
			t.Errorf("%s: decoding %q: %v", tc.comment, payload, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.block) {
			t.Errorf("%s: %q decoded to %+v, want %+v", tc.comment, payload, got, tc.block)
		}
	}
}

func TestEncodeBlockRejects(t *testing.T) {
	head := func(slot BisSlot) BisBlock {
		slot.Slot = proto.ItemSlot_ItemSlotHead
		return BisBlock{Slots: []BisSlot{slot}}
	}
	for comment, block := range map[string]BisBlock{
		"no slots":           {},
		"no items":           head(BisSlot{}),
		"seven items":        head(BisSlot{Items: []int32{1, 2, 3, 4, 5, 6, 7}}),
		"zero item":          head(BisSlot{Items: []int32{0}}),
		"zero gem":           head(BisSlot{Items: []int32{1}, Gems: []int32{0}}),
		"half a reforge":     head(BisSlot{Items: []int32{1}, ReforgeFrom: 13}),
		"item twice":         head(BisSlot{Items: []int32{1, 1}}),
		"delta, no rank 2":   head(BisSlot{Items: []int32{1}, HasDelta: true}),
		"delta, no HasDelta": head(BisSlot{Items: []int32{1, 2}, Delta: 5}),
		"slot past ranged":   {Slots: []BisSlot{{Slot: proto.ItemSlot_ItemSlotRanged + 1, Items: []int32{1}}}},
		"slots out of order": {Slots: []BisSlot{{Slot: 2, Items: []int32{1}}, {Slot: 1, Items: []int32{1}}}},
		"slot twice":         {Slots: []BisSlot{{Slot: 1, Items: []int32{1}}, {Slot: 1, Items: []int32{2}}}},
	} {
		if payload, err := EncodeBlock(block); err == nil {
			t.Errorf("%s: encoded as %q", comment, payload)
		}
	}
}

func TestDecodeBlockRejects(t *testing.T) {
	for _, payload := range []string{
		"", "0", "0:", ":1", "x:1", "17:1", "-1:1", "0:1,", "0:1,,2", "0:01", "0:+1", "0:1 ", "0:1,2,3,4,5,6,7",
		"0:1(", "0:1()", "0:1(e)", "0:1(e0)", "0:1(e1,e2)", "0:1(x1)", "0:1(r13)", "0:1(r13-31,r6-13)", "0:1(g-5)",
		"0:1(g2,e3)", "0:1(r13-31,g2)", "0:1(r13-31,e3)", "0:1,1",
		"0:1+", "0:1+x", "0:1+-0", "0:1+1+2", "0:1+5", "0:1;", "1:1;0:1", "0:1;0:2",
	} {
		if block, err := DecodeBlock(payload); err == nil {
			t.Errorf("%q: decoded to %+v", payload, block)
		}
	}
}

func TestBlockFrames(t *testing.T) {
	// block 11's frames hold 230 payload bytes while the count takes 1 digit, 228 at 2 and 226 at 3.
	// The module has to frame the same way, so the counts are pinned.
	wantFrames := map[int]int{1: 1, 230: 1, 231: 2, 2070: 9, 2071: 10, 3000: 14, 30000: 133}
	for _, size := range []int{1, 100, 229, 230, 231, 900, 2000, 2070, 2071, 3000, 30000} {
		payload := strings.Repeat("0123456789;", size/11+1)[:size]
		for _, blockID := range []int{11, 505} {
			frames, err := BlockFrames("1a2b3c4d", blockID, payload)
			if err != nil {
				t.Fatalf("%d bytes: %v", size, err)
			}
			if want, ok := wantFrames[size]; ok && blockID == 11 && len(frames) != want {
				t.Errorf("%d bytes: %d frames, want %d", size, len(frames), want)
			}
			var joined strings.Builder
			for i, frame := range frames {
				if wire := BisAddonPrefix + "\t" + frame; len(wire) > BisMaxWireLength {
					t.Errorf("%d bytes, frame %d: %d bytes on the wire", size, i+1, len(wire))
				}
				fields := strings.SplitN(frame, "~", 5)
				if len(fields) != 5 || fields[0] != "BLK" || fields[1] != "1a2b3c4d" || fields[2] != strconv.Itoa(blockID) {
					t.Fatalf("%d bytes, frame %d: %q", size, i+1, frame)
				}
				if want := fmt.Sprintf("%d/%d", i+1, len(frames)); fields[3] != want {
					t.Errorf("%d bytes, frame %d: numbered %s, want %s", size, i+1, fields[3], want)
				}
				if i > 0 && i < len(frames)-1 && joined.Len() != i*len(fields[4]) {
					t.Errorf("%d bytes, frame %d: chunks aren't all the same size", size, i+1)
				}
				joined.WriteString(fields[4])
			}
			if joined.String() != payload {
				t.Errorf("%d bytes: the frames don't join back into the payload", size)
			}
		}
	}
	if _, err := BlockFrames("v", 1, ""); err == nil {
		t.Error("framed an empty payload")
	}
}

// The addon's Lua port checks itself against the same known value.
func TestBisChecksum(t *testing.T) {
	if got := BisChecksum("Wikipedia"); got != "11e60398" {
		t.Errorf("got %s", got)
	}
	if got := BisChecksum(""); got != "00000001" {
		t.Errorf("empty: got %s", got)
	}
}

func TestCompositionFingerprint(t *testing.T) {
	raid := []BisCompositionMember{{1, 1}, {8, 2}, {6, 0}, {1, 1}}
	shuffled := []BisCompositionMember{{6, 0}, {1, 1}, {1, 1}, {8, 2}}
	if CompositionFingerprint(raid) != CompositionFingerprint(shuffled) {
		t.Error("order changes the fingerprint")
	}
	if got, want := CompositionFingerprint(raid), BisChecksum("1:1,1:1,6:0,8:2"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	armsInstead := []BisCompositionMember{{1, 0}, {8, 2}, {6, 0}, {1, 1}}
	if CompositionFingerprint(raid) == CompositionFingerprint(armsInstead) {
		t.Error("a tree change keeps the fingerprint")
	}
}
