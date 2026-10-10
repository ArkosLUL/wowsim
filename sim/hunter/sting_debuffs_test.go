package hunter

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
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

// With a bow, gun or crossbow the talent's 5% reaches every non-physical spell (Serpent Sting, Explosive Trap,
// item procs) and, of physical damage, only the ranged attacks.
func TestRangedWeaponSpecializationSchools(t *testing.T) {
	with := hunterWithTalents(t, MMTalents, proto.Hunter_Options_Wolf)
	without := hunterWithTalents(t, "502-035335131030013203035031051-5000002", proto.Hunter_Options_Wolf)
	if with.Talents.RangedWeaponSpecialization != 3 || without.Talents.RangedWeaponSpecialization != 0 {
		t.Fatalf("talent strings: %v and %v", with.Talents.RangedWeaponSpecialization, without.Talents.RangedWeaponSpecialization)
	}
	near := func(got, want float64) bool { return got > want-1e-9 && got < want+1e-9 }
	for school := stats.SchoolIndexPhysical; school < stats.SchoolLen; school++ {
		want := core.TernaryFloat64(school == stats.SchoolIndexPhysical, 1, 1.05)
		if got := with.PseudoStats.SchoolDamageDealtMultiplier[school] / without.PseudoStats.SchoolDamageDealtMultiplier[school]; !near(got, want) {
			t.Errorf("school %d: x%v, want x%v", school, got, want)
		}
	}
	if got := with.SteadyShot.DamageMultiplier / without.SteadyShot.DamageMultiplier; !near(got, 1.05) {
		t.Errorf("Steady Shot x%v, want x1.05", got)
	}
	if got := with.RaptorStrike.DamageMultiplier / without.RaptorStrike.DamageMultiplier; !near(got, 1) {
		t.Errorf("Raptor Strike x%v, want x1", got)
	}
	// the ranged weapon's TOTAL_PCT, which Hunter's Mark's flat bonus on a weapon hit leaves out
	if got := with.SteadyShot.TotalPctMultiplier; !near(got, 1.05) {
		t.Errorf("Steady Shot's TOTAL_PCT x%v, want x1.05", got)
	}
	if got := with.RaptorStrike.TotalPctMultiplier; !near(got, 1) {
		t.Errorf("Raptor Strike's TOTAL_PCT x%v, want x1", got)
	}
	if got := with.SerpentSting.DamageMultiplier / without.SerpentSting.DamageMultiplier; !near(got, 1) {
		t.Errorf("Serpent Sting's own multiplier x%v, want x1: its school carries the talent", got)
	}
}

// Spirit Bond's 24529, Blood of the Rhino and Improved Devotion Aura's 63514 are all
// SPELL_AURA_MOD_HEALING_PCT, and Unit::SpellHealingBonusTaken counts only the highest one on a unit: 10% on
// the hunter and 40% on the pet, under the raid's 6%.
func TestHealingTakenTalentsDontStack(t *testing.T) {
	options := googleProto.Clone(PlayerOptionsBasic.Hunter.Options).(*proto.Hunter_Options)
	options.PetType = proto.Hunter_Options_Wolf
	options.PetTalents = &proto.HunterPetTalents{BloodOfTheRhino: 2}
	raid := core.SinglePlayerRaidProto(&proto.Player{
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassHunter,
		Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv").GearSet,
		Consumes:      FullConsumes,
		Spec:          &proto.Player_Hunter{Hunter: &proto.Hunter{Options: options}},
		Rotation:      core.GetAplRotation("../../ui/hunter/apls", "bm").Rotation,
		TalentsString: SpiritBondTalents,
	}, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	if raid.Buffs.DevotionAura != proto.TristateEffect_TristateEffectImproved || raid.Buffs.StrengthOfWrynn {
		t.Fatalf("the full raid buffs: Devotion Aura %v, Strength of Wrynn %v", raid.Buffs.DevotionAura, raid.Buffs.StrengthOfWrynn)
	}
	env, _, _ := core.NewEnvironment(raid, core.MakeSingleTargetEncounter(0), false)
	hunter := env.Raid.Parties[0].Players[0].(*Hunter)
	if hunter.Talents.SpiritBond != 2 {
		t.Fatalf("Spirit Bond %d, want 2", hunter.Talents.SpiritBond)
	}
	if got := hunter.PseudoStats.HealingTakenMultiplier; got != 1.1 {
		t.Errorf("the hunter's healing taken x%v, want Spirit Bond's x1.1", got)
	}
	if got := hunter.pet.PseudoStats.HealingTakenMultiplier; got != 1.4 {
		t.Errorf("the pet's healing taken x%v, want Blood of the Rhino's x1.4", got)
	}
}
