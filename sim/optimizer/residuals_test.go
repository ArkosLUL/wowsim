package optimizer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Made-up ids, like request_test.go's.
const (
	testPlainRingID  = 9400101
	testEffectRingID = 9400102
	testWeaponID     = 9400103
	testSocketedID   = 9400104
	testStrGemID     = 9400105
)

func addResidualTestItems() {
	core.AddToDatabase(&proto.SimDatabase{
		Items: []*proto.SimItem{
			{Id: testPlainRingID, Type: proto.ItemType_ItemTypeFinger, Stats: stats.Stats{stats.Strength: 100}.ToFloatArray()},
			{Id: testEffectRingID, Type: proto.ItemType_ItemTypeFinger, Stats: stats.Stats{stats.Strength: 50}.ToFloatArray()},
			{Id: testWeaponID, Type: proto.ItemType_ItemTypeWeapon, WeaponDamageMin: 100, WeaponDamageMax: 200, WeaponSpeed: 2.6},
			{
				Id:          testSocketedID,
				Type:        proto.ItemType_ItemTypeHands,
				Stats:       stats.Stats{stats.Strength: 40, stats.MeleeCrit: 30, stats.SpellCrit: 30}.ToFloatArray(),
				GemSockets:  []proto.GemColor{proto.GemColor_GemColorRed},
				SocketBonus: stats.Stats{stats.Stamina: 6}.ToFloatArray(),
				ServerStats: []*proto.ItemStat{{StatType: 4, Value: 40}, {StatType: 32, Value: 30}},
			},
		},
		Gems: []*proto.SimGem{{Id: testStrGemID, Color: proto.GemColor_GemColorRed, Stats: stats.Stats{stats.Strength: 20}.ToFloatArray()}},
	})
}

// TestHardCodedItemIDsCoverTheSim scans the sim for items checked by id: numbers or named
// constants compared with .ID, passed to the Has*Equipped helpers, or in the cases of a switch on
// .ID. Each one needs a registered effect or a hardCodedItemIDs entry.
func TestHardCodedItemIDsCoverTheSim(t *testing.T) {
	constRe := regexp.MustCompile(`(?m)^\s*(?:const\s+|var\s+)?([A-Za-z_]\w*)\s+(?:int32\s+)?=\s*(\d{4,6})\b`)
	compareRe := regexp.MustCompile(`\.ID\s*(?:==|!=)\s*(\w+)`)
	helperRe := regexp.MustCompile(`Has(?:Trinket|Ring|MetaGem)Equipped\((\w+)\)`)
	switchRe := regexp.MustCompile(`^(\s*)switch\s[^{]*\.ID\s*\{\s*$`)
	caseRe := regexp.MustCompile(`^\s*case\s+([^:]+):`)

	type check struct{ where, token string }
	var checks []check
	constants := map[string]int32{}
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "optimizer" || name == "proto" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(string(data), "\r\n", "\n")
		for _, m := range constRe.FindAllStringSubmatch(text, -1) {
			v, _ := strconv.Atoi(m[2])
			constants[m[1]] = int32(v)
		}
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			where := fmt.Sprintf("%s:%d", filepath.ToSlash(path), i+1)
			for _, m := range compareRe.FindAllStringSubmatch(line, -1) {
				checks = append(checks, check{where, m[1]})
			}
			for _, m := range helperRe.FindAllStringSubmatch(line, -1) {
				checks = append(checks, check{where, m[1]})
			}
			m := switchRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			for j := i + 1; j < len(lines) && strings.TrimRight(lines[j], " \t") != m[1]+"}"; j++ {
				if c := caseRe.FindStringSubmatch(lines[j]); c != nil {
					for _, token := range strings.Split(c[1], ",") {
						checks = append(checks, check{fmt.Sprintf("%s:%d", filepath.ToSlash(path), j+1), strings.TrimSpace(token)})
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	found, bypassing := map[int32]bool{}, map[int32]bool{}
	for _, c := range checks {
		id, ok := constants[c.token]
		if n, err := strconv.Atoi(c.token); err == nil {
			id, ok = int32(n), true
		}
		// unresolved tokens are variables, like the helpers' own parameters
		if !ok || id == 0 {
			continue
		}
		found[id] = true
		if !core.HasItemEffect(id) {
			bypassing[id] = true
		}
		if !core.HasItemEffect(id) && !hardCodedItemIDs[id] {
			t.Errorf("%s checks for item %d, which has no registered effect and isn't in hardCodedItemIDs", c.where, id)
		}
	}
	if len(found) < len(hardCodedItemIDs)/2 {
		t.Errorf("the scan found only %d ids; is it still reading the sim?", len(found))
	}
	t.Logf("%d ids checked in the sim, %d without a registered effect; hardCodedItemIDs has %d", len(found), len(bypassing), len(hardCodedItemIDs))
}

func TestEffectFlags(t *testing.T) {
	addResidualTestItems()
	for _, tc := range []struct {
		name string
		got  bool
		want bool
	}{
		{"registered trinket", ItemHasEffect(40256), true},
		{"hard-coded idol", ItemHasEffect(40321), true},
		{"hard-coded meta", ItemHasEffect(41398), true},
		{"plain ring", ItemHasEffect(testPlainRingID), false},
		{"no item", ItemHasEffect(0), false},
		{"registered enchant", EnchantHasEffect(3789), true},
		{"enchant checked by id", EnchantHasEffect(3247), true},
		{"no enchant", EnchantHasEffect(0), false},

		{"empty slot", NeedsSim(ItemChoice{}), false},
		{"plain ring choice", NeedsSim(ItemChoice{ItemID: testPlainRingID}), false},
		{"socketed with a plain gem", NeedsSim(ItemChoice{ItemID: testSocketedID, Gems: [MaxGems]int32{testStrGemID}}), false},
		{"weapon", NeedsSim(ItemChoice{ItemID: testWeaponID}), true},
		{"effect gem", NeedsSim(ItemChoice{ItemID: testSocketedID, Gems: [MaxGems]int32{41398}}), true},
		{"effect enchant", NeedsSim(ItemChoice{ItemID: testPlainRingID, Enchant: 3247}), true},
		{"effect item", NeedsSim(ItemChoice{ItemID: 40321}), true},
		{"unknown item", NeedsSim(ItemChoice{ItemID: 9499992}), true},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestGearStats(t *testing.T) {
	addResidualTestItems()
	var l Loadout
	l.Items[proto.ItemSlot_ItemSlotHands] = ItemChoice{ItemID: testSocketedID, Gems: [MaxGems]int32{testStrGemID}, ReforgeFrom: 32, ReforgeTo: 36}
	l.Items[proto.ItemSlot_ItemSlotFinger1] = ItemChoice{ItemID: testPlainRingID}

	want := stats.Stats{
		stats.Strength:   160,
		stats.Stamina:    6,
		stats.MeleeCrit:  18,
		stats.SpellCrit:  18,
		stats.MeleeHaste: 12,
		stats.SpellHaste: 12,
	}
	if got := gearStats(l); got != want {
		t.Errorf("gear stats = %v, want %v", got, want)
	}
}

func TestMeasureResiduals(t *testing.T) {
	addResidualTestItems()
	fake := newFakeEvaluator(func(p Point) Metrics {
		dps := 5000 + 2*(gearStats(p.Loadout)[stats.Strength]+p.Offset[stats.Strength])
		for _, c := range p.Loadout.Items {
			if c.ItemID == testEffectRingID {
				dps += 30
			}
		}
		return Metrics{MetricDPS: dps}
	})
	resp := Response{stats.Strength: {Stat: stats.Strength, SlopeBelow: 2, SlopeAbove: 2}}

	var seed Loadout
	seed.Items[proto.ItemSlot_ItemSlotFinger1] = ItemChoice{ItemID: testPlainRingID}
	swapped, added, removed := seed, seed, seed
	swapped.Items[proto.ItemSlot_ItemSlotFinger1] = ItemChoice{ItemID: testEffectRingID}
	added.Items[proto.ItemSlot_ItemSlotFinger2] = ItemChoice{ItemID: testEffectRingID}
	removed.Items[proto.ItemSlot_ItemSlotFinger1] = ItemChoice{}

	got, err := MeasureResiduals(context.Background(), fake, dpsObjective(), resp, seed, seed, []Loadout{swapped, added, removed}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []float64{30, 30, 0} {
		if !near(got[i].Mean, want, 0.1) || got[i].SE > 0.1 {
			t.Errorf("variant %d residual = %+v, want %g", i, got[i], want)
		}
	}

	// from a base other than the seed, the curves still count offsets from the seed
	got, err = MeasureResiduals(context.Background(), fake, dpsObjective(), resp, seed, removed, []Loadout{swapped}, 1000)
	if err != nil || !near(got[0].Mean, 30, 0.1) {
		t.Errorf("residual from another base = %+v, %v; want 30", got, err)
	}
}
