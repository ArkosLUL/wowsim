package rogue

import (
	"fmt"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

const (
	testDaggerMH = 50621 // Lungbreaker
	testDaggerOH = 50736 // Heaven's Fall, Kryss of a Thousand Lies
	testAxe      = 50654 // Scourgeborne Waraxe
	testFist     = 45132 // Golden Saronite Dragon
)

type weaponTestMetrics struct {
	casts, landed, outcomes int32
}

// runWeaponTest sims a rogue with only these weapons and a one-spell APL, and returns each action's
// metrics keyed by spell id and tag
func runWeaponTest(t *testing.T, talents string, mainHand, offHand int32, spellID int32) map[core.ActionID]weaponTestMetrics {
	t.Helper()
	gear := &proto.EquipmentSpec{}
	for range proto.ItemSlot_ItemSlotRanged + 1 {
		gear.Items = append(gear.Items, &proto.ItemSpec{})
	}
	gear.Items[proto.ItemSlot_ItemSlotMainHand].Id = mainHand
	gear.Items[proto.ItemSlot_ItemSlotOffHand].Id = offHand
	player := &proto.Player{
		Race:          proto.Race_RaceHuman,
		Class:         proto.Class_ClassRogue,
		TalentsString: talents,
		Equipment:     gear,
		Spec:          PlayerOptionsCombatDI,
		Rotation: core.APLRotationFromJsonString(fmt.Sprintf(
			`{"type":"TypeAPL","priorityList":[{"action":{"castSpell":{"spellId":{"spellId":%d}}}}]}`, spellID)),
	}
	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 5, RandomSeed: 101},
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}

	metrics := map[core.ActionID]weaponTestMetrics{}
	for _, action := range result.RaidMetrics.Parties[0].Players[0].Actions {
		id := core.ActionID{SpellID: action.Id.GetSpellId(), Tag: action.Id.Tag}
		m := metrics[id]
		for _, target := range action.Targets {
			m.casts += target.Casts
			m.landed += target.Hits + target.Crits
			m.outcomes += target.Hits + target.Crits + target.Misses + target.Dodges + target.Parries + target.Glances
		}
		metrics[id] = m
	}
	return metrics
}

// 48666 has SPELL_ATTR3_REQUIRES_MAIN_HAND_WEAPON and _OFF_HAND_WEAPON with a dagger-only
// EquippedItemSubClassMask, which Spell::CheckItems checks in both hands
func TestMutilateNeedsTwoDaggers(t *testing.T) {
	for _, tc := range []struct {
		name              string
		mainHand, offHand int32
		castable          bool
	}{
		{"two daggers", testDaggerMH, testDaggerOH, true},
		{"axe off hand", testDaggerMH, testAxe, false},
		{"no off hand", testDaggerMH, 0, false},
		{"fist main hand", testFist, testDaggerOH, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			casts := runWeaponTest(t, AssassinationTalents, tc.mainHand, tc.offHand, MutilateSpellID)[core.ActionID{SpellID: MutilateSpellID}].casts
			if tc.castable != (casts > 0) {
				t.Errorf("Mutilate cast %d times, want castable %v", casts, tc.castable)
			}
		})
	}
}

// Killing Spree's 57842, Fan of Knives' 52874 and Shiv's 5938 have SPELL_ATTR3_REQUIRES_OFF_HAND_WEAPON,
// so without an off-hand weapon Spell::CheckItems fails them
func TestOffHandStrikesNeedOffHandWeapon(t *testing.T) {
	killingSpreeMH := core.ActionID{SpellID: 57841}
	killingSpreeOH := core.ActionID{SpellID: 57842}
	fanOfKnivesMH := core.ActionID{SpellID: FanOfKnivesSpellID, Tag: 1}
	fanOfKnivesOH := core.ActionID{SpellID: FanOfKnivesSpellID, Tag: 2}
	shiv := core.ActionID{SpellID: 5938}

	for _, offHand := range []int32{0, testDaggerOH} {
		hasOH := offHand != 0

		ks := runWeaponTest(t, CombatTalents, testDaggerMH, offHand, 51690)
		if ks[killingSpreeMH].landed == 0 {
			t.Fatalf("off hand %d: Killing Spree's main hand never landed", offHand)
		}
		// 57841 only triggers 57842 on a hit
		if want := core.TernaryInt32(hasOH, ks[killingSpreeMH].landed, 0); ks[killingSpreeOH].casts != want {
			t.Errorf("off hand %d: Killing Spree's off hand cast %d times, want %d", offHand, ks[killingSpreeOH].casts, want)
		}

		fok := runWeaponTest(t, CombatTalents, testDaggerMH, offHand, FanOfKnivesSpellID)
		if fok[fanOfKnivesMH].outcomes == 0 || hasOH != (fok[fanOfKnivesOH].outcomes > 0) {
			t.Errorf("off hand %d: Fan of Knives rolled %d main-hand and %d off-hand hits, want off-hand ones %v",
				offHand, fok[fanOfKnivesMH].outcomes, fok[fanOfKnivesOH].outcomes, hasOH)
		}

		if casts := runWeaponTest(t, CombatTalents, testDaggerMH, offHand, 5938)[shiv].casts; hasOH != (casts > 0) {
			t.Errorf("off hand %d: Shiv cast %d times, want castable %v", offHand, casts, hasOH)
		}
	}
}
