package wotlk_test

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/mage"
	"github.com/wowsims/wotlk/sim/paladin/holy"
	"github.com/wowsims/wotlk/sim/priest/healing"
	"github.com/wowsims/wotlk/sim/shaman/elemental"
)

func init() {
	mage.RegisterMage()
	holy.RegisterHolyPaladin()
}

const meteoriteCrystal = 46051

// meteoriteSim is one player wearing Meteorite Crystal, at time 0 with the item used, plus dummies to heal.
func meteoriteSim(t *testing.T, player *proto.Player, dummies int32) *core.Simulation {
	t.Helper()
	items := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotTrinket1] = &proto.ItemSpec{Id: meteoriteCrystal}
	player.Equipment = &proto.EquipmentSpec{Items: items}
	player.Consumes = &proto.Consumes{}
	player.Rotation = core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`)
	raid := core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{})
	raid.TargetDummies = dummies
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	character := sim.Raid.Parties[0].Players[0].GetCharacter()
	if !character.GetSpell(core.ActionID{ItemID: meteoriteCrystal}).Cast(sim, nil) {
		t.Fatal("Meteorite Crystal didn't cast")
	}
	return sim
}

func meteoriteStacks(character *core.Character) int32 {
	return character.GetAuraByID(core.ActionID{SpellID: 65000}).GetStacks()
}

// castAndWait casts at full mana off the GCD and steps on by wait, a marker keeping the clock from passing it.
func castAndWait(t *testing.T, sim *core.Simulation, character *core.Character, spell *core.Spell, target *core.Unit, wait time.Duration) {
	t.Helper()
	character.AddMana(sim, character.MaxMana()-character.CurrentMana(), character.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen}))
	character.GCD.Reset()
	if !spell.Cast(sim, target) {
		t.Fatalf("%s didn't cast", spell.ActionID)
	}
	at := sim.CurrentTime + wait
	core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: at, OnAction: func(*core.Simulation) {}})
	for sim.CurrentTime < at && !sim.Step() {
	}
}

// A cast that costs mana stacks once, Penance (53007) once more for each of its three bolts; Hymn of Hope costs
// none.
func TestMeteoricInspirationStacksOnManaCastsAndPenanceBolts(t *testing.T) {
	sim := meteoriteSim(t, &proto.Player{
		Race:          proto.Race_RaceUndead,
		Class:         proto.Class_ClassPriest,
		Spec:          &proto.Player_HealingPriest{HealingPriest: &proto.HealingPriest{Options: &proto.HealingPriest_Options{}}},
		TalentsString: "0503203130300512301313231251-2351010303",
	}, 0)
	priest := sim.Raid.Parties[0].Players[0].(*healing.HealingPriest)
	self := &priest.Unit
	character := priest.GetCharacter()

	// the item's aura lasts 20 s
	for _, step := range []struct {
		name  string
		spell *core.Spell
		wait  time.Duration
		want  int32
	}{
		{"Flash Heal", priest.FlashHeal, 2 * time.Second, 1},
		{"Renew", priest.Renew, time.Second, 2},
		{"Penance", priest.PenanceHeal, 3 * time.Second, 6},
		{"Hymn of Hope", priest.GetSpell(core.ActionID{SpellID: 64901}), time.Second, 6},
	} {
		if step.spell == nil {
			t.Fatalf("no %s", step.name)
		}
		castAndWait(t, sim, character, step.spell, self, step.wait)
		if got := meteoriteStacks(character); got != step.want {
			t.Errorf("%d stacks after %s, want %d", got, step.name, step.want)
		}
	}
}

// A missile's finish phase comes as it lands: Frostbolt's stack waits for its flight, and each of Arcane
// Missiles' missiles (42845, SPELL_ATTR3_NOT_A_PROC) stacks as it lands, on top of the channel's own.
func TestMeteoricInspirationStacksAtAMissilesLanding(t *testing.T) {
	sim := meteoriteSim(t, &proto.Player{
		Race:               proto.Race_RaceTroll,
		Class:              proto.Class_ClassMage,
		Spec:               &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{}}},
		DistanceFromTarget: 28,
	}, 0)
	m := sim.Raid.Parties[0].Players[0].(*mage.Mage)
	character := m.GetCharacter()
	target := sim.Encounter.TargetUnits[0]

	flight := m.Frostbolt.TravelTime()
	if flight < 500*time.Millisecond {
		t.Fatalf("Frostbolt flies %s, want a flight to wait through", flight)
	}
	castTime := m.ApplyCastSpeed(m.Frostbolt.DefaultCast.CastTime)
	castAndWait(t, sim, character, m.Frostbolt, target, castTime+flight/2)
	if got := meteoriteStacks(character); got != 0 {
		t.Errorf("%d stacks with Frostbolt in flight, want 0", got)
	}
	castAndWait(t, sim, character, m.ArcaneMissiles, target, 7*time.Second)
	if got := meteoriteStacks(character); got != 1+1+5 {
		t.Errorf("%d stacks after Frostbolt landed and Arcane Missiles, want %d", got, 1+1+5)
	}
}

// Holy Light and Holy Shock stack once more for the copy Light's Beacon casts; Lay on Hands and Divine Plea cost no
// mana, so neither they nor Lay on Hands' copy stack.
func TestMeteoricInspirationStacksOnBeaconCopiesOfManaHeals(t *testing.T) {
	sim := meteoriteSim(t, &proto.Player{
		Race:          proto.Race_RaceBloodElf,
		Class:         proto.Class_ClassPaladin,
		Spec:          &proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{Options: &proto.HolyPaladin_Options{}}},
		TalentsString: "50350151020013053100515221-50023131203",
	}, 1)
	paladin := sim.Raid.Parties[0].Players[0].(*holy.HolyPaladin)
	character := paladin.GetCharacter()
	self := &paladin.Unit
	var dummy *core.Unit
	for _, player := range sim.Raid.Parties[0].Players {
		if d, ok := player.(*core.TargetDummy); ok {
			dummy = &d.Unit
		}
	}
	if dummy == nil {
		t.Fatal("no dummy to put the beacon on")
	}

	castAndWait(t, sim, character, paladin.BeaconOfLight, dummy, time.Second)
	if got := meteoriteStacks(character); got != 1 {
		t.Errorf("%d stacks after Beacon of Light, want 1", got)
	}
	for _, step := range []struct {
		name  string
		spell *core.Spell
		want  int32
	}{
		{"Holy Light", paladin.HolyLight, 3},
		{"Holy Shock", paladin.HolyShock, 5},
		{"Lay on Hands", paladin.GetSpell(core.ActionID{SpellID: 48788}), 5},
		{"Divine Plea", paladin.DivinePlea, 5},
	} {
		paladin.RemoveHealth(sim, paladin.CurrentHealth()/2)
		castAndWait(t, sim, character, step.spell, self, 3*time.Second)
		if got := meteoriteStacks(character); got != step.want {
			t.Errorf("%d stacks after %s, want %d", got, step.name, step.want)
		}
	}
}

// Call of the Elements (66842) costs nothing itself; each totem it drops costs mana and stacks.
func TestMeteoricInspirationStacksOnCallOfTheElementsTotems(t *testing.T) {
	sim := meteoriteSim(t, &proto.Player{
		Race:  proto.Race_RaceTroll,
		Class: proto.Class_ClassShaman,
		Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{
			Totems: &proto.ShamanTotems{
				Earth: proto.EarthTotem_StrengthOfEarthTotem,
				Air:   proto.AirTotem_WrathOfAirTotem,
				Fire:  proto.FireTotem_FlametongueTotem,
				Water: proto.WaterTotem_ManaSpringTotem,
			},
		}}},
	}, 0)
	shaman := sim.Raid.Parties[0].Players[0].(*elemental.ElementalShaman)
	character := shaman.GetCharacter()

	castAndWait(t, sim, character, shaman.GetSpell(core.ActionID{SpellID: 66842}), sim.Encounter.TargetUnits[0], 2*time.Second)
	if got := meteoriteStacks(character); got != 4 {
		t.Errorf("%d stacks after Call of the Elements dropped four totems, want 4", got)
	}
}
