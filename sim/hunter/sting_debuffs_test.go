package hunter

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	googleProto "google.golang.org/protobuf/proto"
)

func TestStingDebuffMasks(t *testing.T) {
	noxious, arrow := debuffMask(49001), debuffMask(63672)
	for _, tc := range []struct {
		name          string
		spellID       int32
		noxious, both bool
	}{
		{"Arcane Shot", 49045, true, true},
		{"Multi-Shot", 49048, true, true},
		{"Volley", 58434, true, true},
		{"Serpent Sting", 49001, true, true},
		{"Aimed Shot", 49050, true, true},
		{"Explosive Shot", 60053, true, true},
		{"Kill Shot", 61006, true, true},
		{"Steady Shot", 49052, true, true},
		{"Chimera Shot", 53209, true, true},
		{"Black Arrow", 63672, true, false},
		{"Explosive Trap's effect", 49065, true, false},
		{"Raptor Strike", 48996, true, false},
		{"Scorpid Sting", 3043, false, false},
		{"Silencing Shot", 34490, false, false},
		{"Chimera Shot's sting", 53353, false, false},
	} {
		flags := serverdata.SpellByID(tc.spellID).FamilyFlags
		if got := covers(noxious, flags); got != tc.noxious {
			t.Errorf("%s: Serpent Sting's aura covers it is %v, want %v", tc.name, got, tc.noxious)
		}
		if got := covers(arrow, flags); got != tc.both {
			t.Errorf("%s: Black Arrow's aura covers it is %v, want %v", tc.name, got, tc.both)
		}
	}
}

func hunterWithTalents(t *testing.T, talents string, petType proto.Hunter_Options_PetType) *Hunter {
	t.Helper()
	options := googleProto.Clone(PlayerOptionsBasic.Hunter.Options).(*proto.Hunter_Options)
	options.PetType = petType
	spec := &proto.Player_Hunter{Hunter: &proto.Hunter{Options: options}}
	raid := core.SinglePlayerRaidProto(&proto.Player{
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassHunter,
		Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv").GearSet,
		Consumes:      FullConsumes,
		Spec:          spec,
		Rotation:      core.GetAplRotation("../../ui/hunter/apls", "sv").Rotation,
		TalentsString: talents,
	}, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	env, _, _ := core.NewEnvironment(raid, core.MakeSingleTargetEncounter(0), false)
	return env.Raid.Parties[0].Players[0].(*Hunter)
}

// The talent's passive (34692) is permanent whether or not the hunter has a pet.
func TestBeastWithinWithoutPet(t *testing.T) {
	withTalent := hunterWithTalents(t, BMTalents, proto.Hunter_Options_PetNone)
	without := hunterWithTalents(t, "51200201515012233110530351-005305001-4", proto.Hunter_Options_PetNone)
	if !withTalent.Talents.TheBeastWithin || without.Talents.TheBeastWithin {
		t.Fatalf("talent strings: %v and %v", withTalent.Talents.TheBeastWithin, without.Talents.TheBeastWithin)
	}
	if got, want := withTalent.PseudoStats.DamageDealtMultiplier/without.PseudoStats.DamageDealtMultiplier, 1.1; got < want-1e-9 || got > want+1e-9 {
		t.Errorf("damage dealt x%v with The Beast Within and no pet, want x%v", got, want)
	}
}

// The talent is an aura limited to spells that need a ranged weapon, which Serpent Sting is.
func TestRangedWeaponSpecializationReachesSerpentSting(t *testing.T) {
	hunter := hunterWithTalents(t, MMTalents, proto.Hunter_Options_Wolf)
	if hunter.Talents.RangedWeaponSpecialization == 0 {
		t.Fatal("MMTalents has no Ranged Weapon Specialization")
	}
	want := hunter.rangedWeaponSpecializationMultiplier()
	if got := hunter.SerpentSting.DamageMultiplier; got != want {
		t.Errorf("Serpent Sting damage multiplier %v, want %v", got, want)
	}
}
