package main

import (
	"slices"
	"testing"
)

const trinketsSrc = `package tbc

func init() {
	core.NewSimpleStatOffensiveTrinketEffect(32483, stats.Stats{stats.SpellHaste: 175}, time.Second*20, time.Minute*2)  // Skull of Gul'dan
	core.NewSimpleStatOffensiveTrinketEffect(33829, stats.Stats{stats.SpellPower: 211}, time.Second*20, time.Minute*2)  // Hex Shrunken Head
	core.NewSimpleStatOffensiveTrinketEffect(34429, stats.Stats{stats.SpellPower: 320}, time.Second*15, time.Second*90) // Shifting Naaru Sliver

	for _, itemID := range core.AlchStoneItemIDs {
		core.NewItemEffect(itemID, func(core.Agent) {})
	}

	core.NewItemEffect(21625, func(agent core.Agent) { // Scarab Brooch
		actionID := core.ActionID{ItemID: 21625}
		shieldSpell := character.GetOrRegisterSpell(core.SpellConfig{
			ActionID: core.ActionID{SpellID: 26470},
			Duration: time.Second * 30,
		})
	})

	newProcStatBonusEffect(ProcStatBonusEffect{Name: "Mjolnir Runestone", ID: 45931, AuraID: 65019, Bonus: stats.Stats{stats.ArmorPenetration: 751}, ICD: time.Second * 45})
}

var procs = map[int32]float64{
	40255: 1.5,
	40256: 3,
}
`

const heroicSrc = `package wotlk

func init() {
	NewItemEffectWithHeroic(func(isHeroic bool) {
		itemID := int32(50352)
		amount := 5712.0
		if isHeroic {
			itemID = 50349
			amount = 6426.0
		}
		core.NewItemEffect(itemID, func(agent core.Agent) {
			procAura := character.NewTemporaryStatsAura("Proc", actionID, stats.Stats{stats.Armor: amount}, time.Second*10)
		})
	})

	core.NewItemEffect(12345, func(agent core.Agent) {})
}
`

const consumesSrc = `package core

var AlchStoneItemIDs = []int32{44322, 44323, 44324}

const T84PcProcChance = 0.15

func (character *Character) alchStone(float64Value float64) {
	for _, itemID := range AlchStoneItemIDs {
		if character.HasTrinketEquipped(itemID) {
			bonus := 1.4
		}
	}
}
`

const setsSrc = `package warlock

var ItemSetDarkCovensRegalia = core.NewItemSet(core.ItemSet{
	Name: "Dark Coven's Regalia",
	Bonuses: map[int32]core.ApplyEffect{
		2: func(agent core.Agent) {},
		4: func(agent core.Agent) {},
	},
})

func (warlock *Warlock) corruptionBonus() float64 {
	return core.TernaryFloat64(warlock.HasSetBonus(ItemSetDarkCovensRegalia, 2), 1.2, 1)
}

func (warlock *Warlock) otherBonus() {
	if warlock.HasSetBonus(ItemSetDarkCovensRegalia, 4) {
		warlock.bonusCrit = 5
	}
}
`

// testIndex adds files in the order indexGoSources walks them.
func testIndex(t *testing.T) *goSourceIndex {
	t.Helper()
	idx := newGoSourceIndex()
	for _, file := range []struct{ path, src string }{
		{"sim/common/tbc/caster_trinkets.go", trinketsSrc},
		{"sim/common/wotlk/other_effects.go", heroicSrc},
		{"sim/core/consumes.go", consumesSrc},
		{"sim/warlock/items.go", setsSrc},
	} {
		if err := idx.addFile(file.path, file.src); err != nil {
			t.Fatal(err)
		}
	}
	return idx
}

func numbersAt(locs []goLocation) []float64 {
	var numbers []float64
	for _, loc := range locs {
		for _, n := range loc.numbers() {
			if !containsNumber(numbers, n) {
				numbers = append(numbers, n)
			}
		}
	}
	slices.Sort(numbers)
	return numbers
}

func TestNumbersStayWithinOneItem(t *testing.T) {
	idx := testIndex(t)
	for _, tc := range []struct {
		name string
		id   int32
		want []float64
	}{
		{"one-line call between siblings, durations also in seconds", 32483, []float64{2, 20, 120, 175, 32483}},
		{"call with a trailing comment", 21625, []float64{30, 21625, 26470}},
		{"struct literal", 45931, []float64{45, 751, 45931, 65019}},
		{"map entry", 40256, []float64{3, 40256}},
		{"ID kept in a variable", 50352, []float64{10, 5712, 6426, 50349, 50352}},
		{"heroic ID kept in a variable", 50349, []float64{10, 5712, 6426, 50349, 50352}},
		{"call without numbers doesn't widen to init", 12345, []float64{12345}},
	} {
		if got := numbersAt(idx.findID(tc.id)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: numbers for %d = %v, want %v", tc.name, tc.id, got, tc.want)
		}
	}
}

func TestSpellIDInWrapperUsesSurroundingConfig(t *testing.T) {
	idx := testIndex(t)
	if got := numbersAt(idx.findLiteral(26470)); !slices.Equal(got, []float64{30, 26470}) {
		t.Errorf("numbers = %v, want the SpellConfig's", got)
	}
}

func TestTopLevelIDListFollowsItsUses(t *testing.T) {
	idx := testIndex(t)
	locs := idx.findID(44323)
	want := "sim/core/consumes.go:3 sim/common/tbc/caster_trinkets.go:8 sim/core/consumes.go:8"
	if got := formatLocations(locs); got != want {
		t.Errorf("locations = %q, want %q", got, want)
	}
	if numbers := numbersAt(locs); !slices.Contains(numbers, 1.4) || slices.Contains(numbers, 0.15) || slices.Contains(numbers, 64) {
		t.Errorf("numbers = %v, want the loop body's 1.4 but not the unrelated constant or float64's digits", numbers)
	}
}

func TestSetUsages(t *testing.T) {
	idx := testIndex(t)
	locs := idx.findSetUsages("Dark Coven's Regalia")
	if got, want := formatLocations(locs), "sim/warlock/items.go:4 sim/warlock/items.go:12 sim/warlock/items.go:16"; got != want {
		t.Errorf("locations = %q, want %q", got, want)
	}
	if got, want := numbersAt(locs), []float64{1, 1.2, 2, 4, 5}; !slices.Equal(got, want) {
		t.Errorf("numbers = %v, want %v", got, want)
	}
}
