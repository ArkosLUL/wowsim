package optimizer

import (
	"math"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/mage"
	"github.com/wowsims/wotlk/sim/priest"
	"github.com/wowsims/wotlk/sim/shaman"
	"github.com/wowsims/wotlk/sim/warlock"
	goproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func shadowPriestPlayer() *proto.Player {
	return &proto.Player{
		Name:          "Shadow",
		Race:          proto.Race_RaceDraenei,
		Class:         proto.Class_ClassPriest,
		Equipment:     core.GetGearSet("../../ui/shadow_priest/gear_sets", "p3").GearSet,
		Rotation:      core.GetAplRotation("../../ui/shadow_priest/apls", "default").Rotation,
		TalentsString: "05032031--325023051223010323151301351",
		Spec: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{
			Armor: proto.ShadowPriest_Options_InnerFire,
		}}},
		Consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfTheFrostWyrm, Food: proto.Food_FoodFishFeast},
		Buffs:    core.FullIndividualBuffs,
	}
}

func elementalShamanPlayer() *proto.Player {
	return &proto.Player{
		Name:          "Elemental",
		Race:          proto.Race_RaceDraenei,
		Class:         proto.Class_ClassShaman,
		Equipment:     core.GetGearSet("../../ui/elemental_shaman/gear_sets", "p3_alliance").GearSet,
		Rotation:      core.GetAplRotation("../../ui/elemental_shaman/apls", "default").Rotation,
		TalentsString: "0533001523213351322301351-005050031",
		Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{
			Shield: proto.ShamanShield_WaterShield,
			Totems: &proto.ShamanTotems{
				Earth: proto.EarthTotem_StrengthOfEarthTotem,
				Air:   proto.AirTotem_WrathOfAirTotem,
				Water: proto.WaterTotem_ManaSpringTotem,
				Fire:  proto.FireTotem_TotemOfWrath,
			},
		}}},
		Consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfTheFrostWyrm, Food: proto.Food_FoodFishFeast},
		Buffs:    core.FullIndividualBuffs,
	}
}

// fireMagePlayer takes Arcane Focus too, like the Fire talent preset, but casts no arcane spell.
func fireMagePlayer(gearSet string) *proto.Player {
	return &proto.Player{
		Name:          "Fire",
		Race:          proto.Race_RaceTroll,
		Class:         proto.Class_ClassMage,
		Equipment:     core.GetGearSet("../../ui/mage/gear_sets", gearSet).GearSet,
		Rotation:      core.GetAplRotation("../../ui/mage/apls", "fire").Rotation,
		TalentsString: "23000503110003-0055030012303331053120301351",
		Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{
			Armor: proto.Mage_Options_MoltenArmor,
		}}},
		Consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfTheFrostWyrm, Food: proto.Food_FoodFirecrackerSalmon},
		Buffs:    core.FullIndividualBuffs,
	}
}

func afflictionWarlockPlayer() *proto.Player {
	return &proto.Player{
		Name:          "Affliction",
		Race:          proto.Race_RaceGnome,
		Class:         proto.Class_ClassWarlock,
		Equipment:     core.GetGearSet("../../ui/warlock/gear_sets", "p3_affliction_alliance").GearSet,
		Rotation:      core.GetAplRotation("../../ui/warlock/apls", "affliction").Rotation,
		TalentsString: "2350002030023510253500331151--550000051",
		Spec: &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.Warlock_Options{
			Armor:        proto.Warlock_Options_FelArmor,
			Summon:       proto.Warlock_Options_Felhunter,
			WeaponImbue:  proto.Warlock_Options_GrandSpellstone,
			DetonateSeed: true,
		}}},
		Consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfTheFrostWyrm, Food: proto.Food_FoodFishFeast},
		Buffs:    core.FullIndividualBuffs,
	}
}

// playerCapRequest is fury_p1's request with player in the raid instead.
func playerCapRequest(tb testing.TB, player *proto.Player) *Request {
	tb.Helper()
	req := presetOptimizeRequest(tb, "fury_p1")
	req.Base.Raid = core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	r, err := PrepareRequest(req)
	if err != nil {
		tb.Fatal(err)
	}
	return r
}

// withoutTalent takes the talent at proto field number field back to 0, numbered the way
// core.FillTalentsProto reads a talent string: tree by tree, in order.
func withoutTalent(tb testing.TB, talents string, treeSizes [3]int, field protoreflect.FieldNumber) string {
	tb.Helper()
	trees := strings.Split(talents, "-")
	for len(trees) < 3 {
		trees = append(trees, "")
	}
	offset := 0
	for i, size := range treeSizes {
		if pos := int(field) - offset - 1; pos >= 0 && pos < size {
			if pos >= len(trees[i]) || trees[i][pos] == '0' {
				tb.Fatalf("talent #%d isn't taken in %q", field, talents)
			}
			tree := []byte(trees[i])
			tree[pos] = '0'
			trees[i] = string(tree)
			return strings.Join(trees, "-")
		}
		offset += size
	}
	tb.Fatalf("no talent #%d", field)
	return ""
}

func talentField(msg protoreflect.ProtoMessage, name protoreflect.Name) protoreflect.FieldNumber {
	return msg.ProtoReflect().Descriptor().Fields().ByName(name).Number()
}

// spellAt is spell id and its attack table against the encounter's first target, with the target
// wearing the seed and bonus more st rating, in a freshly reset sim.
func spellAt(t *testing.T, r *Request, id core.ActionID, st stats.Stat, bonus float64) (*core.Spell, *core.AttackTable) {
	t.Helper()
	var offset stats.Stats
	offset[st] = bonus
	sim := core.NewSim(sheetRequest(r.Base, r.TargetIndex, r.Seed, offset))
	sim.Reset()
	character := sim.Raid.Parties[r.TargetIndex/5].Players[r.TargetIndex%5].GetCharacter()
	spell := character.GetSpell(id)
	if spell == nil {
		t.Fatalf("no spell %s", id)
	}
	return spell, character.AttackTables[sim.Encounter.TargetUnits[0].UnitIndex]
}

// spellMissBP is MagicSpellHitResult's miss threshold for spell id, read off the spell itself, with
// bonus more spell hit rating.
func spellMissBP(t *testing.T, r *Request, id core.ActionID, bonus float64) int32 {
	t.Helper()
	spell, at := spellAt(t, r, id, stats.SpellHit, bonus)
	return core.SpellMissBP(at.DefenderLevel-at.AttackerLevel, float32(spell.SpellHitChance(at.Defender)*100), at.Defender.Type == core.PlayerUnit)
}

// rangedMissBP is the ranged yellow table's miss for spell id, read off the spell itself, with bonus
// more hit rating.
func rangedMissBP(t *testing.T, r *Request, id core.ActionID, bonus float64) int32 {
	t.Helper()
	spell, at := spellAt(t, r, id, stats.MeleeHit, bonus)
	return core.YellowMeleeTableBP(spell.YellowTableInput(at), core.YellowOptions{Ranged: true}).Miss
}

// A talent that adds hit to each spell rather than to the sheet (Shadow Focus, Arcane Focus,
// Elemental Precision, Suppression) lowers the spell hit floor by exactly that hit: the spell the rotation leans
// on stops missing right at the floor, and still misses one rating under it.
func TestHitTalentsOnSpellsLowerTheSpellHitFloor(t *testing.T) {
	for _, tc := range []struct {
		name      string
		player    func(testing.TB) *proto.Player
		treeSizes [3]int
		talent    protoreflect.FieldNumber
		spell     core.ActionID
	}{
		{"shadow priest", func(testing.TB) *proto.Player { return shadowPriestPlayer() }, priest.TalentTreeSizes,
			talentField(&proto.PriestTalents{}, "shadow_focus"), core.ActionID{SpellID: 48156}},
		{"arcane mage", func(tb testing.TB) *proto.Player { return presetPlayer(tb, "arcane_p3", "Arcane") }, mage.TalentTreeSizes,
			talentField(&proto.MageTalents{}, "arcane_focus"), core.ActionID{SpellID: 42897}},
		{"elemental shaman", func(testing.TB) *proto.Player { return elementalShamanPlayer() }, shaman.TalentTreeSizes,
			talentField(&proto.ShamanTalents{}, "elemental_precision"), core.ActionID{SpellID: 49238}},
		{"affliction warlock", func(testing.TB) *proto.Player { return afflictionWarlockPlayer() }, warlock.TalentTreeSizes,
			talentField(&proto.WarlockTalents{}, "suppression"), core.ActionID{SpellID: 59164}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			player := tc.player(t)
			r := playerCapRequest(t, player)
			floor, err := bisectCapBonus(r, stats.SpellHit, spellMissOK)
			if err != nil {
				t.Fatal(err)
			}

			talentless := goproto.Clone(player).(*proto.Player)
			talentless.TalentsString = withoutTalent(t, player.TalentsString, tc.treeSizes, tc.talent)
			baseline, err := bisectCapBonus(playerCapRequest(t, talentless), stats.SpellHit, spellMissOK)
			if err != nil {
				t.Fatal(err)
			}
			// 3/3 in each talent is 3% hit; both floors are whole rating steps from the same seed
			talentHit := 3 * core.SpellHitRatingPerHitChance
			if got := baseline - floor; math.Abs(got-talentHit) >= 1 {
				t.Errorf("floor %.0f, talentless %.0f: %.1f apart, want the talent's %.1f", floor, baseline, got, talentHit)
			}

			seed, err := playerSheet(r.Base, r.TargetIndex, r.Seed, stats.Stats{})
			if err != nil {
				t.Fatal(err)
			}
			bonus := floor - seed.FinalStats[stats.SpellHit]
			if miss := spellMissBP(t, r, tc.spell, bonus); miss != 0 {
				t.Errorf("at the floor of %.0f, %s's miss threshold is %d bp, want 0", floor, tc.spell, miss)
			}
			if miss := spellMissBP(t, r, tc.spell, bonus-1); miss <= 0 {
				t.Errorf("one rating under the floor of %.0f, %s's miss threshold is %d bp, want it over 0", floor, tc.spell, miss)
			}
			t.Logf("spell hit floor %.0f, %.0f without the talent", floor, baseline)
		})
	}
}

// Specs whose rotation has no per-spell hit keep the floor the sheet's own rating gives, the one they
// get with no rotation at all. Fire mages take Arcane Focus but cast no arcane spell.
func TestHitFloorsWithoutPerSpellHitStayAsTheSheetGives(t *testing.T) {
	for _, tc := range []struct {
		name   string
		player func(testing.TB) *proto.Player
		stat   stats.Stat
		ok     func(core.PlayerSheet) bool
	}{
		{"fire mage", func(testing.TB) *proto.Player { return fireMagePlayer("p3_fire_horde") }, stats.SpellHit, spellMissOK},
		{"fury warrior", func(tb testing.TB) *proto.Player { return presetPlayer(tb, "fury_p1", "Fury") }, stats.MeleeHit, meleeMissOK},
		{"marksmanship hunter", func(testing.TB) *proto.Player { return mmHunter() }, stats.MeleeHit, meleeMissOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			player := tc.player(t)
			floor, err := bisectCapBonus(playerCapRequest(t, player), tc.stat, tc.ok)
			if err != nil {
				t.Fatal(err)
			}
			noRotation := goproto.Clone(player).(*proto.Player)
			noRotation.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
			sheetOnly, err := bisectCapBonus(playerCapRequest(t, noRotation), tc.stat, tc.ok)
			if err != nil {
				t.Fatal(err)
			}
			if floor != sheetOnly {
				t.Errorf("%s floor %.0f with the rotation, %.0f without it; want them equal", tc.stat.StatName(), floor, sheetOnly)
			}
			t.Logf("%s floor %.0f", tc.stat.StatName(), floor)
		})
	}
}

// A ranged scope's hit reaches only ranged specials, and Serpent Sting (ProcMaskEmpty) rolls ranged
// hit without it, so a hunter rotation casting it keeps the floor the sheet's own rating gives.
func TestScopeLeavesTheHunterHitFloorToSerpentSting(t *testing.T) {
	const accurascope = 2523
	serpentSting, steadyShot := core.ActionID{SpellID: 49001}, core.ActionID{SpellID: 49052}
	player := mmHunter()
	player.Equipment.Items[proto.ItemSlot_ItemSlotRanged].Enchant = accurascope
	r := playerCapRequest(t, player)
	floor, err := bisectCapBonus(r, stats.MeleeHit, meleeMissOK)
	if err != nil {
		t.Fatal(err)
	}
	noRotation := goproto.Clone(player).(*proto.Player)
	noRotation.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
	sheetOnly, err := bisectCapBonus(playerCapRequest(t, noRotation), stats.MeleeHit, meleeMissOK)
	if err != nil {
		t.Fatal(err)
	}
	if floor != sheetOnly {
		t.Errorf("MeleeHit floor %.0f with the rotation, %.0f without it; want them equal", floor, sheetOnly)
	}

	seed, err := playerSheet(r.Base, r.TargetIndex, r.Seed, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	bonus := floor - seed.FinalStats[stats.MeleeHit]
	if miss := rangedMissBP(t, r, serpentSting, bonus); miss != 0 {
		t.Errorf("at the floor of %.0f, Serpent Sting misses %d bp, want 0", floor, miss)
	}
	if miss := rangedMissBP(t, r, serpentSting, bonus-1); miss <= 0 {
		t.Errorf("one rating under the floor of %.0f, Serpent Sting misses %d bp, want it over 0", floor, miss)
	}
	// the scope does apply: Steady Shot stops missing 30 rating sooner
	if miss := rangedMissBP(t, r, steadyShot, bonus-30); miss != 0 {
		t.Errorf("30 rating under the floor of %.0f, Steady Shot misses %d bp, want 0 with the scope", floor, miss)
	}
	t.Logf("MeleeHit floor %.0f with the scope", floor)
}
