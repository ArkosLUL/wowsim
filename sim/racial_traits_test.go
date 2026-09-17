package sim

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const testTwoHandSwordID = 9000001

var racialTraitsTestDatabase = &proto.SimDatabase{
	Items: []*proto.SimItem{{
		Id:              testTwoHandSwordID,
		Type:            proto.ItemType_ItemTypeWeapon,
		WeaponType:      proto.WeaponType_WeaponTypeSword,
		HandType:        proto.HandType_HandTypeTwoHand,
		WeaponDamageMin: 500,
		WeaponDamageMax: 700,
		WeaponSpeed:     3.6,
	}},
}

func racialTraitsTestWarrior(race proto.Race, traits proto.Race) *proto.Player {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand].Id = testTwoHandSwordID

	return &proto.Player{
		Name:         "Warrior",
		Race:         race,
		RacialTraits: traits,
		Class:        proto.Class_ClassWarrior,
		Equipment:    equipment,
		Spec:         &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
		Database:     racialTraitsTestDatabase,
	}
}

func racialTraitsTestDruid(traits proto.Race) *proto.Player {
	return &proto.Player{
		Name:         "Druid",
		Race:         proto.Race_RaceNightElf,
		RacialTraits: traits,
		Class:        proto.Class_ClassDruid,
		Equipment:    &proto.EquipmentSpec{},
		Spec:         &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{}}},
	}
}

func computePlayerStats(t *testing.T, players ...*proto.Player) []*proto.PlayerStats {
	t.Helper()
	result := core.ComputeStats(&proto.ComputeStatsRequest{
		Raid: &proto.Raid{Parties: []*proto.Party{{Players: players}}},
	})
	if result.ErrorResult != "" {
		t.Fatalf("ComputeStats failed: %s", result.ErrorResult)
	}
	return result.RaidStats.Parties[0].Players
}

func TestRacialTraitsApplyRacialsButKeepBaseStats(t *testing.T) {
	orc := computePlayerStats(t, racialTraitsTestWarrior(proto.Race_RaceOrc, proto.Race_RaceUnknown))[0]
	orcWithHumanTraits := computePlayerStats(t, racialTraitsTestWarrior(proto.Race_RaceOrc, proto.Race_RaceHuman))[0]
	human := computePlayerStats(t, racialTraitsTestWarrior(proto.Race_RaceHuman, proto.Race_RaceUnknown))[0]

	if got, want := orcWithHumanTraits.BaseStats.Stats[stats.Strength], orc.BaseStats.Stats[stats.Strength]; got != want {
		t.Errorf("base strength with Human traits = %v, want Orc's %v", got, want)
	}
	if orc.BaseStats.Stats[stats.Strength] == human.BaseStats.Stats[stats.Strength] {
		t.Fatalf("Orc and Human base strength are equal, test can't tell them apart")
	}

	gotExpertise := orcWithHumanTraits.FinalStats.Stats[stats.Expertise] - orc.FinalStats.Stats[stats.Expertise]
	if want := 3 * core.ExpertisePerQuarterPercentReduction; math.Abs(gotExpertise-want) > 0.001 {
		t.Errorf("expertise from Human sword specialization = %v, want %v", gotExpertise, want)
	}
}

func TestRacialTraitsDraeneiGivesPartyHeroicPresence(t *testing.T) {
	withoutTraits := computePlayerStats(t, racialTraitsTestDruid(proto.Race_RaceUnknown), racialTraitsTestWarrior(proto.Race_RaceOrc, proto.Race_RaceUnknown))[1]
	withDraeneiTraits := computePlayerStats(t, racialTraitsTestDruid(proto.Race_RaceDraenei), racialTraitsTestWarrior(proto.Race_RaceOrc, proto.Race_RaceUnknown))[1]

	gotHit := withDraeneiTraits.FinalStats.Stats[stats.MeleeHit] - withoutTraits.FinalStats.Stats[stats.MeleeHit]
	if want := core.MeleeHitRatingPerHitChance; math.Abs(gotHit-want) > 0.001 {
		t.Errorf("party member melee hit from Heroic Presence = %v, want %v", gotHit, want)
	}
}
