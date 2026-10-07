package azerothcore

import (
	"errors"
	"fmt"
	"hash/adler32"
	"slices"
	"strconv"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// The BiS tooltip wire format, shared by acbis, mod-bis-tooltip and the BisTooltipAC addon. A change
// here has to land in the module's framing and the addon's decoder too, and bumps BisProtocolVersion
// when old addons can't read it.
const (
	BisAddonPrefix     = "BIST"
	BisProtocolVersion = 3
	// 3.3.5a addon messages carry prefix, tab and message in at most this many bytes
	BisMaxWireLength = 255
	// the optimizer's pick plus up to 5 runners-up
	BisMaxRanks = 6
)

// BisBlock is the BiS of one subject in one content phase.
type BisBlock struct {
	// in slot order, empty slots left out
	Slots []BisSlot
}

// BisSlot is one equipment slot of a block. Only the rank 1 item carries enhancements.
type BisSlot struct {
	Slot proto.ItemSlot
	// rank 1 first
	Items []int32
	// spell id, 0 for none: every enchant has one, and the addon shows it as a spell link
	Enchant int32
	// gem item ids in socket order, empty sockets left out
	Gems []int32
	// server ItemModType ids, both 0 for none
	ReforgeFrom, ReforgeTo int32
	// one value per rank from 2 upward: that rank's loss against rank 1, in tenths of a percent of
	// rank 1's damage, positive meaning worse. Empty for none; otherwise len(Items)-1 values.
	Deltas []int32
	// the off hand each runner-up was simmed with, where it isn't rank 1's, by ascending rank. Sparse:
	// a rank with no entry keeps rank 1's off hand, one with item 0 drops it.
	Pairings []BisPairing
}

// BisPairing is one runner-up's off hand. Item 0 means that rank drops the off hand, which is a
// different fact from the rank carrying no pairing at all, so the two mustn't be collapsed.
type BisPairing struct {
	// 2 upward, so rank r's loss is Deltas[r-2]
	Rank int32
	Item int32
}

// EncodeBlock writes a block's payload: its slots joined with ';', each
//
//	<slot>:<item>[,<item>...][(<enh>[,<enh>...])][(<pair>[,<pair>...])][+<delta>[,<delta>...]]
//
// slot is the proto ItemSlot number. An enh is e<enchant spell>, g<gem item> or r<from>-<to> for a
// reforge, in that order, gems in socket order. A pair is o<rank>-<off hand item>, ranks ascending
// from 2 up, naming the off hand that rank was simmed with where it isn't rank 1's; item 0 means that
// rank drops the off hand, and a rank left out keeps rank 1's. Both groups are parenthesised, so a
// decoder tells them apart by what their entries start with. The deltas are one per rank from 2
// upward, each that rank's loss against rank 1 in tenths of a percent of rank 1's damage, so a slot
// carries either none or one fewer than it has items. A value can be negative, as in "+-3,5". For
// example "0:51227,50712,50713(e59954,g41398,g40111,r31-37)+4,21" and
// "14:50730,50708,47520(e59621)(o2-50412,o3-0)+64,180". The result is plain ASCII with no '~', '|' or
// tab, so it travels inside an addon message as is.
func EncodeBlock(block BisBlock) (string, error) {
	if len(block.Slots) == 0 {
		return "", errors.New("block has no slots")
	}
	var sb strings.Builder
	for i, slot := range block.Slots {
		if err := checkBisSlot(slot); err != nil {
			return "", err
		}
		if i > 0 {
			if slot.Slot <= block.Slots[i-1].Slot {
				return "", fmt.Errorf("slot %d comes after slot %d", slot.Slot, block.Slots[i-1].Slot)
			}
			sb.WriteByte(';')
		}
		sb.WriteString(strconv.Itoa(int(slot.Slot)))
		sb.WriteByte(':')
		for j, item := range slot.Items {
			if j > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(strconv.Itoa(int(item)))
		}
		if enhs := slot.enhancements(); len(enhs) > 0 {
			sb.WriteString("(" + strings.Join(enhs, ",") + ")")
		}
		if pairs := slot.pairings(); len(pairs) > 0 {
			sb.WriteString("(" + strings.Join(pairs, ",") + ")")
		}
		if len(slot.Deltas) > 0 {
			sb.WriteByte('+')
			for j, delta := range slot.Deltas {
				if j > 0 {
					sb.WriteByte(',')
				}
				sb.WriteString(strconv.Itoa(int(delta)))
			}
		}
	}
	return sb.String(), nil
}

func checkBisSlot(slot BisSlot) error {
	if slot.Slot < proto.ItemSlot_ItemSlotHead || slot.Slot > proto.ItemSlot_ItemSlotRanged {
		return fmt.Errorf("slot %d isn't an equipment slot", slot.Slot)
	}
	if len(slot.Items) == 0 || len(slot.Items) > BisMaxRanks {
		return fmt.Errorf("slot %d has %d items, want 1 to %d", slot.Slot, len(slot.Items), BisMaxRanks)
	}
	for i, id := range slot.Items {
		if id <= 0 {
			return fmt.Errorf("slot %d has item id %d", slot.Slot, id)
		}
		if slices.Contains(slot.Items[:i], id) {
			return fmt.Errorf("slot %d ranks item %d twice", slot.Slot, id)
		}
	}
	if n := len(slot.Deltas); n > 0 && n != len(slot.Items)-1 {
		return fmt.Errorf("slot %d has %d deltas for %d items, want %d", slot.Slot, n, len(slot.Items), len(slot.Items)-1)
	}
	if len(slot.Pairings) > 0 && len(slot.Items) < 2 {
		return fmt.Errorf("slot %d pairs off hands with only %d item", slot.Slot, len(slot.Items))
	}
	for i, pair := range slot.Pairings {
		if pair.Rank < 2 || int(pair.Rank) > len(slot.Items) {
			return fmt.Errorf("slot %d pairs rank %d, want 2 to %d", slot.Slot, pair.Rank, len(slot.Items))
		}
		if i > 0 && pair.Rank <= slot.Pairings[i-1].Rank {
			return fmt.Errorf("slot %d pairs rank %d after rank %d", slot.Slot, pair.Rank, slot.Pairings[i-1].Rank)
		}
		if pair.Item < 0 {
			return fmt.Errorf("slot %d pairs rank %d with item %d", slot.Slot, pair.Rank, pair.Item)
		}
	}
	if slot.Enchant < 0 {
		return fmt.Errorf("slot %d has enchant %d", slot.Slot, slot.Enchant)
	}
	for _, gem := range slot.Gems {
		if gem <= 0 {
			return fmt.Errorf("slot %d has gem %d", slot.Slot, gem)
		}
	}
	if (slot.ReforgeFrom == 0) != (slot.ReforgeTo == 0) || slot.ReforgeFrom < 0 || slot.ReforgeTo < 0 {
		return fmt.Errorf("slot %d has reforge %d to %d", slot.Slot, slot.ReforgeFrom, slot.ReforgeTo)
	}
	return nil
}

func (slot BisSlot) enhancements() []string {
	var enhs []string
	if slot.Enchant != 0 {
		enhs = append(enhs, "e"+strconv.Itoa(int(slot.Enchant)))
	}
	for _, gem := range slot.Gems {
		enhs = append(enhs, "g"+strconv.Itoa(int(gem)))
	}
	if slot.ReforgeFrom != 0 {
		enhs = append(enhs, fmt.Sprintf("r%d-%d", slot.ReforgeFrom, slot.ReforgeTo))
	}
	return enhs
}

func (slot BisSlot) pairings() []string {
	pairs := make([]string, len(slot.Pairings))
	for i, pair := range slot.Pairings {
		pairs[i] = fmt.Sprintf("o%d-%d", pair.Rank, pair.Item)
	}
	return pairs
}

// DecodeBlock reads a payload EncodeBlock wrote. The addon has its own decoder; this one is the
// reference its output is checked against.
func DecodeBlock(payload string) (BisBlock, error) {
	if payload == "" {
		return BisBlock{}, errors.New("empty payload")
	}
	var block BisBlock
	for _, text := range strings.Split(payload, ";") {
		slot, err := decodeBisSlot(text)
		if err != nil {
			return BisBlock{}, fmt.Errorf("%q: %w", text, err)
		}
		if n := len(block.Slots); n > 0 && slot.Slot <= block.Slots[n-1].Slot {
			return BisBlock{}, fmt.Errorf("slot %d comes after slot %d", slot.Slot, block.Slots[n-1].Slot)
		}
		block.Slots = append(block.Slots, slot)
	}
	return block, nil
}

func decodeBisSlot(text string) (BisSlot, error) {
	var slot BisSlot
	id, rest, ok := strings.Cut(text, ":")
	if !ok {
		return slot, errors.New("no ':' after the slot")
	}
	slotID, err := parseBisInt(id)
	if err != nil {
		return slot, err
	}
	slot.Slot = proto.ItemSlot(slotID)

	if i := strings.IndexByte(rest, '+'); i >= 0 {
		for _, text := range strings.Split(rest[i+1:], ",") {
			delta, err := parseBisInt(text)
			if err != nil {
				return slot, fmt.Errorf("delta: %w", err)
			}
			slot.Deltas = append(slot.Deltas, delta)
		}
		rest = rest[:i]
	}

	// both groups are parenthesised and both optional, so their contents say which is which. They come
	// off the end one at a time, since neither holds a paren of its own.
	var groups []string
	for strings.HasSuffix(rest, ")") {
		open := strings.LastIndexByte(rest, '(')
		if open < 0 {
			return slot, errors.New("unmatched ')'")
		}
		groups = append(groups, rest[open+1:len(rest)-1])
		rest = rest[:open]
	}
	if strings.IndexByte(rest, '(') >= 0 {
		return slot, errors.New("unclosed '('")
	}
	slices.Reverse(groups)
	stage := 0
	for _, group := range groups {
		kind, err := bisGroupKind(group)
		if err != nil {
			return slot, err
		}
		if kind <= stage {
			return slot, errors.New("groups out of order, want enhancements then pairings")
		}
		stage = kind
		switch kind {
		case bisEnhancementGroup:
			err = slot.decodeEnhancements(group)
		case bisPairingGroup:
			err = slot.decodePairings(group)
		}
		if err != nil {
			return slot, err
		}
	}

	for _, item := range strings.Split(rest, ",") {
		id, err := parseBisInt(item)
		if err != nil {
			return slot, fmt.Errorf("item: %w", err)
		}
		slot.Items = append(slot.Items, id)
	}
	return slot, checkBisSlot(slot)
}

// the group kinds in the order they're written
const (
	bisEnhancementGroup = 1
	bisPairingGroup     = 2
)

// bisGroupKind tells one parenthesised group from the other by what its first entry starts with: an
// enhancement opens with 'e', 'g' or 'r', a pairing with 'o'.
func bisGroupKind(group string) (int, error) {
	if group == "" {
		return 0, errors.New("empty group")
	}
	switch group[0] {
	case 'e', 'g', 'r':
		return bisEnhancementGroup, nil
	case 'o':
		return bisPairingGroup, nil
	}
	return 0, fmt.Errorf("unknown group %q", group)
}

func (slot *BisSlot) decodeEnhancements(group string) error {
	for _, enh := range strings.Split(group, ",") {
		if err := slot.decodeEnhancement(enh); err != nil {
			return err
		}
	}
	// the values are canonical already, so a mismatch can only be the order
	if strings.Join(slot.enhancements(), ",") != group {
		return errors.New("enhancements out of order, want enchant, gems, reforge")
	}
	return nil
}

func (slot *BisSlot) decodePairings(group string) error {
	for _, pair := range strings.Split(group, ",") {
		if pair == "" || pair[0] != 'o' {
			return fmt.Errorf("pairing %q", pair)
		}
		rank, item, ok := strings.Cut(pair[1:], "-")
		if !ok {
			return fmt.Errorf("pairing %q", pair)
		}
		at, err1 := parseBisInt(rank)
		id, err2 := parseBisInt(item)
		if errors.Join(err1, err2) != nil {
			return fmt.Errorf("pairing %q", pair)
		}
		slot.Pairings = append(slot.Pairings, BisPairing{Rank: at, Item: id})
	}
	return nil
}

func (slot *BisSlot) decodeEnhancement(enh string) error {
	if enh == "" {
		return errors.New("empty enhancement")
	}
	value := enh[1:]
	switch enh[0] {
	case 'e':
		if slot.Enchant != 0 {
			return errors.New("two enchants")
		}
		enchant, err := parseBisInt(value)
		if err != nil || enchant == 0 {
			return fmt.Errorf("enchant %q", value)
		}
		slot.Enchant = enchant
	case 'g':
		gem, err := parseBisInt(value)
		if err != nil {
			return fmt.Errorf("gem: %w", err)
		}
		slot.Gems = append(slot.Gems, gem)
	case 'r':
		if slot.ReforgeFrom != 0 {
			return errors.New("two reforges")
		}
		from, to, ok := strings.Cut(value, "-")
		if !ok {
			return fmt.Errorf("reforge %q", value)
		}
		var err1, err2 error
		slot.ReforgeFrom, err1 = parseBisInt(from)
		slot.ReforgeTo, err2 = parseBisInt(to)
		if err := errors.Join(err1, err2); err != nil || slot.ReforgeFrom == 0 {
			return fmt.Errorf("reforge %q", value)
		}
	default:
		return fmt.Errorf("unknown enhancement %q", enh)
	}
	return nil
}

// parseBisInt takes plain decimal only: no '+', spaces or leading zeros, so each value has exactly
// one encoding.
func parseBisInt(s string) (int32, error) {
	digits := strings.TrimPrefix(s, "-")
	if digits == "" || (digits[0] == '0' && len(digits) > 1) || digits == "0" && s != "0" {
		return 0, fmt.Errorf("%q isn't a number", s)
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%q isn't a number", s)
		}
	}
	value, err := strconv.ParseInt(s, 10, 32)
	return int32(value), err
}

// BlockFrames splits a block payload into the BLK messages the module sends:
//
//	BLK~<dataset version>~<block id>~<seq>/<total>~<chunk>
//
// seq counts from 1. Each message fits BisMaxWireLength with the "BIST\t" prefix in front.
func BlockFrames(datasetVersion string, blockID int, payload string) ([]string, error) {
	if payload == "" {
		return nil, errors.New("empty payload")
	}
	header := fmt.Sprintf("BLK~%s~%d~", datasetVersion, blockID)
	fixed := len(BisAddonPrefix) + 1 + len(header) + len("/~")
	// the chunk size depends on how many digits seq and total take, and total on the chunk size
	for digits := 1; ; digits++ {
		room := BisMaxWireLength - fixed - 2*digits
		if room <= 0 {
			return nil, fmt.Errorf("dataset version %q and block id %d leave no room for the payload", datasetVersion, blockID)
		}
		total := (len(payload) + room - 1) / room
		if len(strconv.Itoa(total)) > digits {
			continue
		}
		frames := make([]string, total)
		for i := range frames {
			chunk := payload[i*room : min(len(payload), (i+1)*room)]
			frames[i] = fmt.Sprintf("%s%d/%d~%s", header, i+1, total, chunk)
		}
		return frames, nil
	}
}

// BisChecksum is the Adler-32 of s as 8 lowercase hex digits. It checks a reassembled block payload
// (DONE) and hashes the composition fingerprint. Adler-32 because Lua 5.1 can compute it with plain
// arithmetic: a and b start at 1 and 0, and each byte does a = (a + byte) % 65521, b = (b + a) % 65521;
// the checksum is b * 65536 + a.
func BisChecksum(s string) string {
	return fmt.Sprintf("%08x", adler32.Checksum([]byte(s)))
}

// BisCompositionMember is one raider as the composition fingerprint sees them. Class and main talent
// tree decide which buffs and debuffs someone brings; their name doesn't.
type BisCompositionMember struct {
	ClassID int32
	// 0 to 2, in tab order
	Tree int
}

// CompositionFingerprint hashes a group's makeup: "<class id>:<tree>" per member, sorted as strings
// and joined with ',', through BisChecksum. The addon computes the same over its live group, where
// tree is the inspected talent tab with the most points (the first on a tie), minus 1.
func CompositionFingerprint(members []BisCompositionMember) string {
	parts := make([]string, len(members))
	for i, member := range members {
		parts[i] = fmt.Sprintf("%d:%d", member.ClassID, member.Tree)
	}
	slices.Sort(parts)
	return BisChecksum(strings.Join(parts, ","))
}
