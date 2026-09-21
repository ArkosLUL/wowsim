package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim/core/proto"
)

const repoRoot = "../.."

func captureSetup(t *testing.T, name string) *recordedSetup {
	t.Helper()
	setup, err := readSetup(filepath.Join(repoRoot, chronicleDir, name+".setup.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return setup
}

func TestParseSetup(t *testing.T) {
	setup := captureSetup(t, "hunter_Svrleadtbrfb_1789993143")

	if setup.Player != "Svrleadtbrfb" || setup.Class != 3 || setup.Race != 4 || setup.Seconds != 300 || setup.Yards != 20 {
		t.Errorf("header = %q class %d race %d, %v s at %v yd", setup.Player, setup.Class, setup.Race, setup.Seconds, setup.Yards)
	}
	if len(setup.Talents) != 28 || !slices.Contains(setup.Talents, 53270) {
		t.Errorf("%d talents %v, want 28 with Beast Mastery (53270)", len(setup.Talents), setup.Talents)
	}
	// 17 rows: every slot but the off hand, shirt included
	if ranged := setup.Items[17]; len(setup.Items) != 17 || ranged.ID != 50733 || len(strings.Fields(ranged.Enchantments)) != 36 {
		t.Errorf("%d items, ranged %+v", len(setup.Items), ranged)
	}
	if setup.Glyphs != [6]int32{693, 440, 441, 356, 439, 368} {
		t.Errorf("glyphs = %v", setup.Glyphs)
	}
	if len(setup.Bags) != 4 || setup.Bags[0] != (setupBag{Slot: 19, Item: 44448, Class: 11}) || setup.Ammo != 52021 {
		t.Errorf("bags %v, ammo %d", setup.Bags, setup.Ammo)
	}
	if pet := setup.Pet; pet == nil || pet.Family != 35 || pet.Name != "Serpent" || len(pet.Spells) != 5 {
		t.Errorf("pet = %+v", pet)
	}
}

// The SV captures are older, with the server snapshot as text lines rather than JSON.
func TestParseSetupTextSnapshot(t *testing.T) {
	setup := captureSetup(t, "hunter_Svrleadtntyo_1789992248")
	if len(setup.Talents) != 24 || len(setup.Items) != 17 || setup.Pet == nil || setup.Pet.Family != 24 || len(setup.Pet.Spells) != 14 {
		t.Errorf("%d talents, %d items, pet %+v", len(setup.Talents), len(setup.Items), setup.Pet)
	}
}

func TestRecordedHunterOptions(t *testing.T) {
	setup := captureSetup(t, "hunter_Svrleadhbjyz_1789991622")
	options, err := hunterOptions(setup, &rrsimData{root: repoRoot})
	if err != nil {
		t.Fatal(err)
	}
	if options.Ammo != proto.Hunter_Options_IcebladeArrow || options.Quiver != proto.Hunter_Options_Quiver15Percent ||
		options.PetType != proto.Hunter_Options_Wasp {
		t.Errorf("ammo %v, quiver %v, pet %v", options.Ammo, options.Quiver, options.PetType)
	}
	// a ferocity wasp; the talents all three trees share read the same from any of them
	want := &proto.HunterPetTalents{
		CobraReflexes: 2, Dive: true, BoarsSpeed: true, SpikedCollar: 3, CullingTheHerd: 3, WildHunt: 1,
		SpidersBite: 3, Rabid: true, CallOfTheWild: true,
	}
	if got := options.PetTalents; !googleProto.Equal(got, want) {
		t.Errorf("pet talents {%v}, want {%v}", got, want)
	}

	setup.Ammo = 1
	if _, err := hunterOptions(setup, &rrsimData{root: repoRoot}); err == nil {
		t.Error("unknown ammo converted without an error")
	}
}

func TestImprovedTrackingRanks(t *testing.T) {
	ranks, err := talentRankSpells(filepath.Join(repoRoot, talentTreesDir, "hunter.json"), "improvedTracking")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ranks, []int32{52783, 52785, 52786, 52787, 52788}) {
		t.Errorf("Improved Tracking ranks = %v", ranks)
	}
}
