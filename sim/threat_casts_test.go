package sim

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// threatCastsTestSim is the player alone against one target, reset to the pull.
func threatCastsTestSim(t *testing.T, player *proto.Player) (*core.Simulation, *core.Character) {
	t.Helper()
	RegisterAll()
	if player.Equipment == nil {
		player.Equipment = &proto.EquipmentSpec{}
	}
	if player.Consumes == nil {
		player.Consumes = &proto.Consumes{}
	}
	// a power gain asks the rotation what's next
	player.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{core.NewDefaultTarget()}},
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 1},
	})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].GetCharacter()
}

func threatCastsTestWarrior(mainHand int32, professions ...proto.Profession) *proto.Player {
	items := make([]*proto.ItemSpec, len(proto.ItemSlot_name))
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotMainHand].Id = mainHand
	return &proto.Player{
		Race:        proto.Race_RaceHuman,
		Class:       proto.Class_ClassWarrior,
		Equipment:   &proto.EquipmentSpec{Items: items},
		Professions: professions,
		Spec:        &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}},
	}
}

// Bloodrage (2687) triggers 29131, the rage over time, which is a cast with a bonus of its own: its level
// 10, modified. 2687 has none.
func TestBloodrageAuraCastThreat(t *testing.T) {
	sim, char := threatCastsTestSim(t, threatCastsTestWarrior(0))
	char.PseudoStats.ThreatMultiplier *= 2
	bloodrage := char.GetSpell(core.ActionID{SpellID: 2687})
	if bloodrage == nil {
		t.Fatal("no Bloodrage")
	}
	target := sim.Encounter.TargetUnits[0]
	bloodrage.SkipCastAndApplyEffects(sim, target)

	// the 20 rage make 100, unmodified
	want := 100 + 10*char.ThreatSchoolMultiplier(core.SpellSchoolPhysical)
	if got := char.GainThreat(bloodrage.ActionID); math.Abs(got-want) > 1e-6 {
		t.Errorf("threat %v, want %v", got, want)
	}
	if got := bloodrage.SpellMetrics[target.UnitIndex].TotalThreat; got != 0 {
		t.Errorf("Bloodrage's own cast made %v threat", got)
	}
}

// Lifeblood's cast hands out its level 55, modified.
func TestLifebloodCastThreat(t *testing.T) {
	sim, char := threatCastsTestSim(t, threatCastsTestWarrior(0, proto.Profession_Herbalism))
	char.PseudoStats.ThreatMultiplier *= 2
	lifeblood := char.GetSpell(core.ActionID{SpellID: 55503})
	if lifeblood == nil {
		t.Fatal("no Lifeblood")
	}
	lifeblood.SkipCastAndApplyEffects(sim, sim.Encounter.TargetUnits[0])

	want := 55 * char.ThreatSchoolMultiplier(core.SpellSchoolNature)
	if got := char.GainThreat(lifeblood.ActionID); math.Abs(got-want) > 1e-6 {
		t.Errorf("threat %v, want %v", got, want)
	}
}

// Each Heartpierce proc casts the Invigoration of the wearer's power, level 80.
func TestHeartpierceProcCastThreat(t *testing.T) {
	for _, itemID := range []int32{49982, 50641} {
		sim, char := threatCastsTestSim(t, threatCastsTestWarrior(itemID))
		char.PseudoStats.ThreatMultiplier *= 2
		name := "Heartpierce"
		if itemID == 50641 {
			name += " H"
		}
		trigger, proc := char.GetAura(name+" Trigger"), char.GetAura(name)
		if trigger == nil || proc == nil {
			t.Fatalf("item %d: no %s auras", itemID, name)
		}
		swing := &core.SpellResult{Target: sim.Encounter.TargetUnits[0], Outcome: core.OutcomeHit}
		for i := 0; i < 100000 && !proc.IsActive(); i++ {
			trigger.OnSpellHitDealt(trigger, sim, char.AutoAttacks.MHAuto(), swing)
		}
		if !proc.IsActive() {
			t.Fatalf("item %d never procced", itemID)
		}

		want := 80 * char.ThreatSchoolMultiplier(core.SpellSchoolNature)
		if got := char.GainThreat(core.ActionID{ItemID: itemID}); math.Abs(got-want) > 1e-6 {
			t.Errorf("item %d: threat %v, want %v", itemID, got, want)
		}
	}
}

// Silent Resolve is a threat aura on the holy and arcane schools, so every spell of those schools gets it
// once, and nothing else does.
func TestSilentResolveIsASchoolThreatAura(t *testing.T) {
	smitePriest := func(talents string) *proto.Player {
		return &proto.Player{
			Race:          proto.Race_RaceHuman,
			Class:         proto.Class_ClassPriest,
			TalentsString: talents,
			Spec: &proto.Player_SmitePriest{SmitePriest: &proto.SmitePriest{
				Options: &proto.SmitePriest_Options{},
			}},
		}
	}
	for points, cut := range []float64{0, 0.07, 0.14, 0.20} {
		_, char := threatCastsTestSim(t, smitePriest([]string{"", "001", "002", "003"}[points]))
		for school, want := range map[core.SpellSchool]float64{
			core.SpellSchoolHoly:     1 - cut,
			core.SpellSchoolArcane:   1 - cut,
			core.SpellSchoolShadow:   1,
			core.SpellSchoolNature:   1,
			core.SpellSchoolPhysical: 1,
		} {
			if got := char.ThreatSchoolMultiplier(school); math.Abs(got-want) > 1e-9 {
				t.Errorf("%d points: school %d threat x%v, want x%v", points, school, got, want)
			}
		}
		for spellID, want := range map[int32]float64{
			48123: 1000 * (1 - cut), // Smite
			48127: 1000,             // Mind Blast
		} {
			spell := char.GetSpell(core.ActionID{SpellID: spellID})
			if spell == nil {
				t.Fatalf("no spell %d", spellID)
			}
			if got := spell.ThreatFromDamage(core.OutcomeHit, 1000); math.Abs(got-want) > 1e-6 {
				t.Errorf("%d points: spell %d makes %v threat of 1000 damage, want %v", points, spellID, got, want)
			}
		}
	}
}
