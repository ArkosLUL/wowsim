package hunter

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

type petAbilityHit struct {
	// what armor left of the hit, and what it leaves of the pet's physical hits on that target
	armor, petArmor float64
}

// runPetAbility fights two minutes with the pet and returns every landed hit and tick of its spell. The
// hunter has 500 armor penetration rating. The target takes every mechanic: a raid boss is immune to roots,
// so Pin would never land.
func runPetAbility(t *testing.T, petType proto.Hunter_Options_PetType, spellID int32, tweaks *proto.SpellTweaksSettings) (direct, ticks []petAbilityHit) {
	t.Helper()
	return runPetAbilityOn(t, petType, spellID, tweaks, googleProto.Uint64(0))
}

// runPetAbilityOn is runPetAbility against a target with these mechanic immunities, nil for a raid boss's.
func runPetAbilityOn(t *testing.T, petType proto.Hunter_Options_PetType, spellID int32, tweaks *proto.SpellTweaksSettings,
	immunities *uint64) (direct, ticks []petAbilityHit) {
	t.Helper()
	options := googleProto.Clone(PlayerOptionsBasic.Hunter.Options).(*proto.Hunter_Options)
	options.PetType = petType
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 120
	encounter.ServerSettings = &proto.ServerSettings{SpellTweaks: tweaks}
	// the default target is shared, so the test's goes on a copy
	target := googleProto.Clone(encounter.Targets[0]).(*proto.Target)
	target.MechanicImmuneMask = immunities
	encounter.Targets[0] = target
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceOrc,
			Class:     proto.Class_ClassHunter,
			Equipment: core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv").GearSet,
			Spec:      &proto.Player_Hunter{Hunter: &proto.Hunter{Options: options}},
			// Savage Rend is a cooldown of the hunter's
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[{"action":{"autocastOtherCooldowns":{}}}]}`),
			TalentsString: SVTalents,
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	hunter := sim.Raid.Parties[0].Players[0].(*Hunter)
	pet := hunter.pet

	record := func(into *[]petAbilityHit) func(*core.Aura, *core.Simulation, *core.Spell, *core.SpellResult) {
		return func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell.ActionID.SpellID != spellID || !result.Landed() {
				return
			}
			petArmor := pet.AttackTables[result.Target.UnitIndex].GetArmorDamageModifier(spell)
			*into = append(*into, petAbilityHit{result.ResistanceMultiplier, petArmor})
		}
	}
	pet.RegisterAura(core.Aura{
		Label:    "pet ability test log",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt:       record(&direct),
		OnPeriodicDamageDealt: record(&ticks),
	})

	sim.Reset()
	sim.PrePull()
	hunter.AddStatsDynamic(sim, stats.Stats{stats.ArmorPenetration: 500})
	for !sim.Step() {
	}
	sim.Cleanup()
	return direct, ticks
}

// Pin (53548) is a root with a physical dot, no bleed, so Unit::IsDamageReducedByArmor lets armor cut its
// ticks, less the armor penetration a hunter pet takes from its owner (Unit::CalcArmorReducedDamage).
func TestPinTicksMeetArmor(t *testing.T) {
	_, ticks := runPetAbility(t, proto.Hunter_Options_Crab, 53548, nil)
	if len(ticks) < 4 {
		t.Fatalf("Pin ticked %d times, want a whole Pin at least", len(ticks))
	}
	for _, tick := range ticks {
		if tick.armor != tick.petArmor || tick.armor >= 1 {
			t.Fatalf("armor left x%v of a Pin tick, want the pet's x%v", tick.armor, tick.petArmor)
		}
	}

	_, bare := runPetAbility(t, proto.Hunter_Options_Crab, 53548, &proto.SpellTweaksSettings{HunterPetArmorPen: googleProto.Bool(false)})
	if len(bare) == 0 {
		t.Fatal("Pin never ticked with HunterPetArmorPen off")
	}
	if ticks[0].armor <= bare[0].armor {
		t.Errorf("armor left x%v of a Pin tick with the hunter's armor penetration and x%v without, want more with",
			ticks[0].armor, bare[0].armor)
	}
}

// Savage Rend (53582) has the bleed mechanic, so armor skips its ticks. The first hit still meets it:
// Unit::CalculateSpellDamageTaken asks without an effect index, and only an effect's own check finds the bleed.
func TestSavageRendTicksSkipArmor(t *testing.T) {
	direct, ticks := runPetAbility(t, proto.Hunter_Options_Raptor, 53582, nil)
	if len(direct) == 0 || len(ticks) == 0 {
		t.Fatalf("Savage Rend hit %d times and ticked %d, want both", len(direct), len(ticks))
	}
	for _, hit := range direct {
		if hit.armor != hit.petArmor || hit.armor >= 1 {
			t.Fatalf("armor left x%v of Savage Rend's hit, want the pet's x%v", hit.armor, hit.petArmor)
		}
	}
	for _, tick := range ticks {
		if tick.armor != 1 {
			t.Fatalf("armor left x%v of a Savage Rend tick, want all of it", tick.armor)
		}
	}
}

// A spell whose own mechanic the target is immune to never lands (Creature::IsImmunedToSpell): live, Pin on the
// boss dummy came back immune and nothing ticked. Raid bosses are immune to roots, stuns, snares and disarms,
// which takes out Pin, Venom Web Spray, Ravage, Tendon Rip and Snatch.
func TestPetSpecialsNeverLandOnAnImmuneTarget(t *testing.T) {
	for _, special := range []struct {
		name     string
		pet      proto.Hunter_Options_PetType
		spellID  int32
		mechanic core.Mechanic
	}{
		{"Pin", proto.Hunter_Options_Crab, 53548, core.MechanicRoot},
		{"Venom Web Spray", proto.Hunter_Options_Silithid, 55509, core.MechanicRoot},
		{"Ravage", proto.Hunter_Options_Ravager, 53562, core.MechanicStun},
		{"Tendon Rip", proto.Hunter_Options_Hyena, 53575, core.MechanicSnare},
		{"Snatch", proto.Hunter_Options_BirdOfPrey, 53543, core.MechanicDisarm},
	} {
		direct, ticks := runPetAbilityOn(t, special.pet, special.spellID, nil, nil)
		if len(direct)+len(ticks) > 0 {
			t.Errorf("%s landed %d times and ticked %d on a raid boss, want neither", special.name, len(direct), len(ticks))
		}
		// immune to every mechanic a raid boss is but the spell's own
		others := googleProto.Uint64(core.RaidBossMechanicImmunities &^ (1 << special.mechanic))
		direct, ticks = runPetAbilityOn(t, special.pet, special.spellID, nil, others)
		if len(direct)+len(ticks) == 0 {
			t.Errorf("%s never landed on a target that takes its mechanic", special.name)
		}
	}
}
