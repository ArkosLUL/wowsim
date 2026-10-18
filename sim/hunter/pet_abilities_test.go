package hunter

import (
	"math"
	"testing"
	"time"

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

// spitTravel fights a minute with the pet against a target of that reach, 0 for the boss dummy's 2.5, the pet's scale
// raised by scaleBonus all along, and gives the flight of its spit's first landing and how far from the target's
// centre the pet stood then.
func spitTravel(t *testing.T, petType proto.Hunter_Options_PetType, spellID int32, reach, scaleBonus float64) (time.Duration, float64) {
	t.Helper()
	options := googleProto.Clone(PlayerOptionsBasic.Hunter.Options).(*proto.Hunter_Options)
	options.PetType = petType
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 60
	// the default target is shared, so the test's goes on a copy
	target := googleProto.Clone(encounter.Targets[0]).(*proto.Target)
	target.CombatReach = reach
	encounter.Targets[0] = target
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassHunter,
			Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv").GearSet,
			Spec:          &proto.Player_Hunter{Hunter: &proto.Hunter{Options: options}},
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
			TalentsString: SVTalents,
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	travel, spot := time.Duration(-1), 0.0
	pet := sim.Raid.Parties[0].Players[0].(*Hunter).pet
	pet.RegisterAura(core.Aura{
		Label:    "spit test log",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
			pet.AddScaleBonus(scaleBonus)
		},
		OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, _ *core.SpellResult) {
			if spell.ActionID.SpellID == spellID && travel < 0 {
				travel, spot = spell.TravelTime(), pet.DistanceFromTarget
			}
		},
	})
	sim.Reset()
	sim.PrePull()
	for !sim.Step() {
	}
	sim.Cleanup()
	if travel < 0 {
		t.Fatalf("spell %d never landed against a reach of %v", spellID, reach)
	}
	return travel, spot
}

// The pet stops both reaches out from the target's centre, its own 1.5 times its family's scale (1.2 for a serpent),
// and a spit flies from there less that reach, 5 yd at least (Spell::AddUnitTarget): the boss dummy's 2.5 leaves the
// floor, a reach of 10 leaves 10. A 30% scale aura like Bloodlust's adds to the family's scale
// (Unit::RecalculateObjectScale), so the reach grows by 0.45 whatever the family, and from the same spot a spit
// against a reach of 10 flies 9.55.
func TestSpitsFlyFromTheTargetsReach(t *testing.T) {
	for _, spit := range []struct {
		name    string
		pet     proto.Hunter_Options_PetType
		spellID int32
		speed   float64
		reach   float64
	}{
		{"Acid Spit", proto.Hunter_Options_Worm, 55754, 24, 1.5},
		{"Poison Spit", proto.Hunter_Options_Serpent, 55557, 40, 1.2},
	} {
		for _, reach := range []float64{0, 10} {
			yards, centre := 5.0, core.BossDummyCombatReach
			if reach > 0 {
				yards, centre = reach, reach
			}
			want := time.Duration(math.Floor(yards/spit.speed*1000)) * time.Millisecond
			got, spot := spitTravel(t, spit.pet, spit.spellID, reach, 0)
			if got != want {
				t.Errorf("%s against a reach of %v flies %v, want %v", spit.name, reach, got, want)
			}
			if math.Abs(spot-centre-spit.reach) > 1e-6 {
				t.Errorf("%s against a reach of %v goes from %v yd off the centre, want %v", spit.name, reach, spot,
					centre+spit.reach)
			}
		}
		want := time.Duration(math.Floor(9.55/spit.speed*1000)) * time.Millisecond
		if got, _ := spitTravel(t, spit.pet, spit.spellID, 10, 0.3); got != want {
			t.Errorf("%s against a reach of 10 with 30%% more scale flies %v, want %v", spit.name, got, want)
		}
	}
}

// Pet::GetNativeObjectScale: a level 80 pet takes its family's maxScale, or 1 for the families it pins there, and
// Creature::SetObjectScale gives a pet 1.5 times that.
func TestHunterPetReachIsItsFamilysScale(t *testing.T) {
	for petType := range PetConfigs {
		if _, ok := hunterPetScales[petType]; !ok {
			t.Errorf("no scale for the %v", petType)
		}
	}
	for _, c := range []struct {
		pet   proto.Hunter_Options_PetType
		reach float64
	}{
		{proto.Hunter_Options_Crocolisk, 0.9},
		{proto.Hunter_Options_Serpent, 1.2},
		{proto.Hunter_Options_Worm, 1.5},
		{proto.Hunter_Options_Cat, 1.65},
		// maxScale 0.5, pinned to 1
		{proto.Hunter_Options_CoreHound, 1.5},
	} {
		if got := hunterPetReach(c.pet); math.Abs(got-c.reach) > 1e-6 {
			t.Errorf("the %v's reach is %v, want %v", c.pet, got, c.reach)
		}
	}
}
