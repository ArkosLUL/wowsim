package statcaps

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func init() {
	sim.RegisterAll()
}

func furyPlayer() *proto.Player {
	return &proto.Player{
		Name:          "Fury",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		Equipment:     core.GetGearSet("../../ui/warrior/gear_sets", "p1_fury").GearSet,
		Rotation:      core.GetAplRotation("../../ui/warrior/apls", "fury").Rotation,
		TalentsString: "302023102331-305052000520310053120500351",
		Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{
			StartingRage: 50,
			Shout:        proto.WarriorShout_WarriorShoutBattle,
		}}},
		Consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfEndlessRage, Food: proto.Food_FoodFishFeast},
		Buffs:    core.FullIndividualBuffs,
	}
}

// a Night Elf rogue with no talents: nothing of the player's moves an expertise cap
func plainRogue() *proto.Player {
	return &proto.Player{
		Name:            "Rogue",
		Race:            proto.Race_RaceNightElf,
		Class:           proto.Class_ClassRogue,
		Equipment:       core.GetGearSet("../../ui/rogue/gear_sets", "p1_combat").GearSet,
		Rotation:        core.GetAplRotation("../../ui/rogue/apls", "combat").Rotation,
		Spec:            &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.Rogue_Options{}}},
		Buffs:           &proto.IndividualBuffs{},
		InFrontOfTarget: true,
	}
}

// a Troll mage with no talents, so no hit talent of any kind
func plainMage() *proto.Player {
	return &proto.Player{
		Name:      "Mage",
		Race:      proto.Race_RaceTroll,
		Class:     proto.Class_ClassMage,
		Equipment: core.GetGearSet("../../ui/mage/gear_sets", "p1_fire").GearSet,
		Rotation:  core.GetAplRotation("../../ui/mage/apls", "fire").Rotation,
		Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{
			Armor: proto.Mage_Options_MoltenArmor,
		}}},
		Buffs: &proto.IndividualBuffs{},
	}
}

func arcaneMage() *proto.Player {
	player := plainMage()
	player.Equipment = core.GetGearSet("../../ui/mage/gear_sets", "p3_arcane_alliance").GearSet
	player.Rotation = core.GetAplRotation("../../ui/mage/apls", "arcane").Rotation
	player.TalentsString = "23000513310033015032310250532-03-023303001"
	return player
}

func shadowPriest() *proto.Player {
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
		Buffs: core.FullIndividualBuffs,
	}
}

func mmHunter() *proto.Player {
	return &proto.Player{
		Name:          "Marksmanship",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassHunter,
		Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_mm").GearSet,
		Rotation:      core.GetAplRotation("../../ui/hunter/apls", "mm").Rotation,
		TalentsString: "502-035335131030013233035031051-5000002",
		Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{
			Ammo:    proto.Hunter_Options_SaroniteRazorheads,
			PetType: proto.Hunter_Options_Wolf,
		}}},
		Buffs:              core.FullIndividualBuffs,
		DistanceFromTarget: 30,
	}
}

func protPaladin() *proto.Player {
	return &proto.Player{
		Name:          "Prot",
		Race:          proto.Race_RaceHuman,
		Class:         proto.Class_ClassPaladin,
		Equipment:     core.GetGearSet("../../ui/protection_paladin/gear_sets", "p3").GearSet,
		Rotation:      core.GetAplRotation("../../ui/protection_paladin/apls", "default").Rotation,
		TalentsString: "-05005135200132311333312321-511302012003",
		Spec: &proto.Player_ProtectionPaladin{ProtectionPaladin: &proto.ProtectionPaladin{Options: &proto.ProtectionPaladin_Options{
			Judgement: proto.PaladinJudgement_JudgementOfWisdom,
			Seal:      proto.PaladinSeal_Vengeance,
			Aura:      proto.PaladinAura_RetributionAura,
		}}},
		Buffs:           core.FullIndividualBuffs,
		InFrontOfTarget: true,
	}
}

func holyPaladin() *proto.Player {
	return &proto.Player{
		Name:          "Holy",
		Race:          proto.Race_RaceBloodElf,
		Class:         proto.Class_ClassPaladin,
		Equipment:     core.GetGearSet("../../ui/holy_paladin/gear_sets", "p1").GearSet,
		TalentsString: "50350151020013053100515221-50023131203",
		Rotation: core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": [
			{"action":{"autocastOtherCooldowns":{}}}
		]}`),
		Spec:  &proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{Options: &proto.HolyPaladin_Options{}}},
		Buffs: core.FullIndividualBuffs,
	}
}

type setup struct {
	party   *proto.PartyBuffs
	debuffs *proto.Debuffs
	tank    bool
}

func raidFor(player *proto.Player, s setup) *proto.Raid {
	if s.party == nil {
		s.party = &proto.PartyBuffs{}
	}
	if s.debuffs == nil {
		s.debuffs = &proto.Debuffs{}
	}
	raid := core.SinglePlayerRaidProto(player, s.party, core.FullRaidBuffs, s.debuffs)
	if s.tank {
		raid.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	}
	return raid
}

func encounter() *proto.Encounter {
	return &proto.Encounter{
		Duration:             180,
		ExecuteProportion_20: 0.2,
		ExecuteProportion_25: 0.25,
		ExecuteProportion_35: 0.35,
		Targets:              []*proto.Target{core.NewDefaultTarget()},
	}
}

func compute(t *testing.T, player *proto.Player, s setup) []*proto.StatCap {
	t.Helper()
	if !core.WITH_DB {
		t.Skip("needs the with_db item data")
	}
	start := time.Now()
	caps, err := Compute(raidFor(player, s), encounter(), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d caps in %v", player.Name, len(caps), time.Since(start).Round(time.Millisecond))
	for _, c := range caps {
		t.Log(describe(c))
	}
	return caps
}

func describe(c *proto.StatCap) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s [%v %v] rating %.1f needed %.1f", c.Label, c.Hand, c.Kind, c.Rating, c.Needed)
	for _, s := range c.Sources {
		fmt.Fprintf(&b, "\n    %s: %.1f (%.2f%%)", s.Label, s.Rating, s.Percent)
	}
	if c.Note != "" {
		fmt.Fprintf(&b, "\n    note: %s", c.Note)
	}
	return b.String()
}

func findCap(t *testing.T, caps []*proto.StatCap, label string, hand proto.StatCapHand) *proto.StatCap {
	t.Helper()
	for _, c := range caps {
		if c.Label == label && c.Hand == hand {
			return c
		}
	}
	t.Fatalf("no %q cap for %v", label, hand)
	return nil
}

func findSource(t *testing.T, c *proto.StatCap, label string) *proto.StatCapSource {
	t.Helper()
	for _, s := range c.Sources {
		if s.Label == label {
			return s
		}
	}
	t.Fatalf("%s has no %q row: %s", c.Label, label, describe(c))
	return nil
}

func near(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}

// sheetAt is the sheet Compute bisects for player in s.
func sheetAt(player *proto.Player, s setup) SheetAt {
	return PlayerSheetAt(raidFor(player, s), encounter(), 0)
}

// sheetAtCap is the sheet with gear rating moved from the cap's rating to its needed, plus bonus.
func sheetAtCap(t *testing.T, at SheetAt, c *proto.StatCap, bonus float64) core.PlayerSheet {
	t.Helper()
	var offset stats.Stats
	offset[stats.Stat(c.Stat)] = c.Needed - c.Rating + bonus
	sheet, err := at(offset)
	if err != nil {
		t.Fatal(err)
	}
	return sheet
}

func TestRoleOf(t *testing.T) {
	for _, tc := range []struct {
		player *proto.Player
		want   Role
	}{
		{&proto.Player{Spec: &proto.Player_Warrior{}}, RoleMelee},
		{&proto.Player{Spec: &proto.Player_Hunter{}}, RoleHunter},
		{&proto.Player{Spec: &proto.Player_ShadowPriest{}}, RoleCaster},
		{&proto.Player{Spec: &proto.Player_TankDeathknight{}}, RoleTank},
		{&proto.Player{Spec: &proto.Player_HolyPaladin{}}, RoleHealer},
		{&proto.Player{Spec: &proto.Player_RestorationDruid{}}, RoleHealer},
		{&proto.Player{Spec: &proto.Player_RestorationShaman{}}, RoleHealer},
		{&proto.Player{Spec: &proto.Player_HealingPriest{}}, RoleHealer},
		{&proto.Player{}, RoleNone},
	} {
		if got := RoleOf(tc.player); got != tc.want {
			t.Errorf("%T: role %v, want %v", tc.player.Spec, got, tc.want)
		}
	}
}

// Melee hit against a level 83 boss is 8%, 262 rating: 261.99 already truncates the miss to 0.
func TestBareHitIs262(t *testing.T) {
	caps := compute(t, furyPlayer(), setup{})
	hit := findCap(t, caps, "Hit", proto.StatCapHand_StatCapHandBoth)
	bare := findSource(t, hit, "Level 83 target")
	if bare.Rating != 262 || !near(bare.Percent, 8, 1e-9) {
		t.Errorf("bare hit %.2f rating (%.2f%%), want 262 (8%%)", bare.Rating, bare.Percent)
	}
	// the armor penetration cap doesn't depend on the target's level
	arp := findCap(t, caps, "Armor penetration", proto.StatCapHand_StatCapHandBoth)
	if bare := findSource(t, arp, "100% armor penetration"); bare.Rating != 1400 {
		t.Errorf("bare armor penetration %.2f rating, want 1400", bare.Rating)
	}
}

// Precision's 3% and Heroic Presence's 1% leave Fury 4% to find on gear, about 131 rating, and
// Heroic Presence alone is worth 33 of it.
func TestFuryWithPrecisionAndHeroicPresence(t *testing.T) {
	withHP := findCap(t, compute(t, furyPlayer(), setup{party: &proto.PartyBuffs{HeroicPresence: true}}), "Hit", proto.StatCapHand_StatCapHandBoth)
	withoutHP := findCap(t, compute(t, furyPlayer(), setup{}), "Hit", proto.StatCapHand_StatCapHandBoth)
	if !near(withHP.Needed, 131, 1) {
		t.Errorf("Fury with Heroic Presence needs %.1f hit rating, want about 131", withHP.Needed)
	}
	if got := math.Round(withoutHP.Needed) - math.Round(withHP.Needed); got != 33 {
		t.Errorf("Heroic Presence moves the needed hit by %.0f, want 33", got)
	}
	if !near(findSource(t, withHP, "Talents").Percent, -3, 1e-6) || !near(findSource(t, withHP, "Heroic Presence").Percent, -1, 1e-6) {
		t.Errorf("want Precision's -3%% and Heroic Presence's -1%%: %s", describe(withHP))
	}
	if withHP.Rating != withoutHP.Rating {
		t.Errorf("Heroic Presence moved the gear rating from %.1f to %.1f", withoutHP.Rating, withHP.Rating)
	}
	at := sheetAt(furyPlayer(), setup{party: &proto.PartyBuffs{HeroicPresence: true}})
	if s := sheetAtCap(t, at, withHP, 0); s.PhysicalMissChance != 0 {
		t.Errorf("at the needed hit the miss chance is %.4f", s.PhysicalMissChance)
	}
	if s := sheetAtCap(t, at, withHP, -1); s.PhysicalMissChance == 0 {
		t.Error("one rating under the needed hit nothing misses already")
	}
}

// Spell hit against a level 83 boss is 446 rating with nothing else, 367 under Misery: the server's
// roll misses only below a 1 bp threshold, so 16.99% of rating is enough.
func TestSpellHitCap(t *testing.T) {
	plain := findCap(t, compute(t, plainMage(), setup{}), "Spell hit", proto.StatCapHand_StatCapHandBoth)
	misery := findCap(t, compute(t, plainMage(), setup{debuffs: &proto.Debuffs{Misery: true}}), "Spell hit", proto.StatCapHand_StatCapHandBoth)
	if plain.Needed != 446 || misery.Needed != 367 {
		t.Errorf("spell hit needs %.1f, %.1f under Misery; want 446 and 367", plain.Needed, misery.Needed)
	}
	if bare := findSource(t, plain, "Level 83 target"); bare.Rating != 446 || !near(bare.Percent, 16.99, 1e-9) {
		t.Errorf("bare spell hit %.1f (%.2f%%), want 446 (16.99%%)", bare.Rating, bare.Percent)
	}
	if debuff := findSource(t, misery, "Misery"); !near(debuff.Rating, -3*core.SpellHitRatingPerHitChance, 1e-6) {
		t.Errorf("Misery's row is %.2f, want -%.2f", debuff.Rating, 3*core.SpellHitRatingPerHitChance)
	}
	at := sheetAt(plainMage(), setup{debuffs: &proto.Debuffs{Misery: true}})
	if s := sheetAtCap(t, at, misery, -1); math.Round(s.SpellMissChance*core.MaxRollBP) <= 1 {
		t.Error("one rating under the needed spell hit nothing misses already")
	}
}

// Shadow Focus adds its 3% to shadow spells only, so it shows in the per-spell row.
func TestShadowFocusIsThePerSpellRow(t *testing.T) {
	hit := findCap(t, compute(t, shadowPriest(), setup{debuffs: core.FullDebuffs}), "Spell hit", proto.StatCapHand_StatCapHandBoth)
	if row := findSource(t, hit, "Talents on your spells"); !near(row.Rating, -3*core.SpellHitRatingPerHitChance, 1) {
		t.Errorf("per-spell row %.1f, want about -%.1f (Shadow Focus)", row.Rating, 3*core.SpellHitRatingPerHitChance)
	}
	findSource(t, hit, "Heroic Presence")
	findSource(t, hit, "Misery or Improved Faerie Fire")
}

// heldSword is the first two-handed sword in the item database.
func heldSword(t *testing.T) int32 {
	t.Helper()
	sword, items := int32(0), 0
	core.ForEachItem(func(item core.Item) {
		items++
		if item.WeaponType == proto.WeaponType_WeaponTypeSword && item.HandType == proto.HandType_HandTypeTwoHand &&
			(sword == 0 || item.ID < sword) {
			sword = item.ID
		}
	})
	if sword == 0 {
		t.Fatalf("no two-handed sword among %d items", items)
	}
	return sword
}

// An Orc's axe expertise lands on the axe's hand only when the other hand holds a sword, so each hand
// gets its own rows, 5 expertise (41 rating) apart.
func TestOrcAxeAndSwordHandsGetTheirOwnRows(t *testing.T) {
	player := furyPlayer()
	player.Equipment.Items[proto.ItemSlot_ItemSlotOffHand].Id = heldSword(t)
	player.InFrontOfTarget = true
	caps := compute(t, player, setup{})
	for _, label := range []string{"Expertise (dodge)", "Expertise (parry)"} {
		axe := findCap(t, caps, label, proto.StatCapHand_StatCapHandMain)
		sword := findCap(t, caps, label, proto.StatCapHand_StatCapHandOff)
		if got := sword.Needed - axe.Needed; !near(got, 5*core.ExpertisePerQuarterPercentReduction, 1) {
			t.Errorf("%s: the sword hand needs %.1f more than the axe hand, want about 41", label, got)
		}
		if axe.Rating != sword.Rating {
			t.Errorf("%s: the hands' gear rating differs, %.1f and %.1f", label, axe.Rating, sword.Rating)
		}
		findSource(t, axe, "Orc axe: main hand only")
	}
}

// Without anything of the player's, yellow dodge goes at 212 and parry at 460, white swings' at 192
// and 440: the white table takes the level gap in only while the chance is still positive.
func TestWhiteSwingNotes(t *testing.T) {
	caps := compute(t, plainRogue(), setup{})
	dodge := findCap(t, caps, "Expertise (dodge)", proto.StatCapHand_StatCapHandBoth)
	parry := findCap(t, caps, "Expertise (parry)", proto.StatCapHand_StatCapHandBoth)
	if dodge.Needed != 212 || parry.Needed != 460 {
		t.Errorf("expertise needs %.1f for dodge and %.1f for parry, want 212 and 460", dodge.Needed, parry.Needed)
	}
	if !strings.Contains(dodge.Note, "at 192 ") || !strings.Contains(parry.Note, "at 440 ") {
		t.Errorf("white notes %q and %q, want 192 and 440", dodge.Note, parry.Note)
	}
	if len(findCap(t, caps, "White hit (dual wield)", proto.StatCapHand_StatCapHandBoth).Sources) == 0 {
		t.Error("the dual wield white hit cap lists no sources")
	}
}

// A melee player behind the target gets no parry row.
func TestParryOnlyInFront(t *testing.T) {
	player := plainRogue()
	player.InFrontOfTarget = false
	for _, c := range compute(t, player, setup{}) {
		if c.Label == "Expertise (parry)" {
			t.Errorf("a rogue behind the target got %s", describe(c))
		}
	}
}

func checkHasteCeiling(t *testing.T, player *proto.Player, s setup, castTime func(core.PlayerSheet) time.Duration) {
	t.Helper()
	haste := findCap(t, compute(t, player, s), "Haste", proto.StatCapHand_StatCapHandBoth)
	if haste.Kind != proto.StatCapKind_StatCapCeiling || math.IsInf(haste.Needed, 0) || haste.Note == "" {
		t.Fatalf("want a reachable ceiling with a note: %s", describe(haste))
	}
	at := sheetAt(player, s)
	if got := castTime(sheetAtCap(t, at, haste, 0)); got > HasteCeilingFloor {
		t.Errorf("at the ceiling it takes %v", got)
	}
	if got := castTime(sheetAtCap(t, at, haste, -1)); got <= HasteCeilingFloor {
		t.Errorf("one rating under the ceiling it already takes %v", got)
	}
}

func TestHunterHasteCeiling(t *testing.T) {
	checkHasteCeiling(t, mmHunter(), setup{}, func(s core.PlayerSheet) time.Duration { return SteadyShotCastTime(s.RangedSwingSpeed) })
}

func TestCasterHasteCeiling(t *testing.T) {
	checkHasteCeiling(t, arcaneMage(), setup{}, func(s core.PlayerSheet) time.Duration { return GlobalCooldown(s.CastSpeed) })
}

func TestHealerGetsTheGCDCeilingOnly(t *testing.T) {
	caps := compute(t, holyPaladin(), setup{})
	if len(caps) != 1 {
		t.Errorf("a healer got %d caps, want the haste ceiling alone", len(caps))
	}
	checkHasteCeiling(t, holyPaladin(), setup{}, func(s core.PlayerSheet) time.Duration { return GlobalCooldown(s.CastSpeed) })
}

// A tank the boss swings at gets a Defense floor at exactly D*, in gear rating.
func TestTankDefenseFloorIsRequiredDefense(t *testing.T) {
	s := setup{tank: true}
	defense := findCap(t, compute(t, protPaladin(), s), "Defense (crit immunity)", proto.StatCapHand_StatCapHandBoth)
	at := sheetAt(protPaladin(), s)
	want, err := RequiredDefense(at)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := at(stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	if got := defense.Needed - defense.Rating + seed.FinalStats[stats.Defense]; !near(got, want, 1e-6) {
		t.Errorf("the floor sits at %.2f sheet Defense, want D* = %.2f", got, want)
	}
	if sheet := sheetAtCap(t, at, defense, 0); sheet.MeleeCritTakenChance != 0 {
		t.Errorf("at the floor the boss still crits %.4f", sheet.MeleeCritTakenChance)
	}

	for _, c := range compute(t, protPaladin(), setup{}) {
		if c.Label == "Defense (crit immunity)" {
			t.Errorf("a tank nothing swings at got %s", describe(c))
		}
	}
}

// A cap no gear reaches is a note, not an error.
func TestUnreachableCapIsANote(t *testing.T) {
	c := &computer{seed: core.PlayerSheet{}, player: furyPlayer()}
	got := c.compute(capDef{
		label:    "Hit",
		stat:     stats.MeleeHit,
		sheetCap: func() (float64, error) { return 0, fmt.Errorf("even %d more Hit rating doesn't reach it", MaxCapBonus) },
	})
	if !math.IsInf(got.Needed, 1) || !strings.HasPrefix(got.Note, "Out of reach") {
		t.Errorf("got %s", describe(got))
	}
}

func TestComputeRequestAnswersTheFirstPlayer(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("needs the with_db item data")
	}
	result := ComputeRequest(&proto.StatCapsRequest{Raid: raidFor(plainMage(), setup{}), Encounter: encounter()})
	if result.ErrorResult != "" || len(result.Caps) != 2 {
		t.Errorf("result %v", result)
	}
	if result := ComputeRequest(&proto.StatCapsRequest{Raid: raidFor(plainMage(), setup{}), Encounter: &proto.Encounter{}}); result.ErrorResult == "" {
		t.Error("an encounter without a target got caps")
	}
}
