package core

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterAgentFactory(
		proto.Player_Warrior{},
		proto.Spec_SpecWarrior,
		func(char *Character, _ *proto.Player) Agent {
			a := &rageTestAgent{Character: *char}
			a.EnableRageBar(RageBarOptions{RageMultiplier: 1, MHSwingSpeed: 3.6})
			return a
		},
		func(player *proto.Player, spec interface{}) {
			player.Spec = spec.(*proto.Player_Warrior)
		},
	)
}

// A class with rage and nothing else, like a warrior.
type rageTestAgent struct {
	Character
}

func (a *rageTestAgent) GetCharacter() *Character { return &a.Character }
func (a *rageTestAgent) Initialize()              {}
func (a *rageTestAgent) ApplyTalents()            {}
func (a *rageTestAgent) Reset(_ *Simulation)      {}
func (a *rageTestAgent) OnGCDReady(_ *Simulation) {}

var arcaneTorrentRunicPower = ActionID{SpellID: 50613}

func bloodElfRageUserRaid() *proto.Raid {
	return &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{
		Name:         "Warrior",
		Race:         proto.Race_RaceHuman,
		RacialTraits: proto.Race_RaceBloodElf,
		Class:        proto.Class_ClassWarrior,
		Equipment:    &proto.EquipmentSpec{},
		Spec:         &proto.Player_Warrior{Warrior: &proto.Warrior{}},
		Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL, PriorityList: []*proto.APLListItem{{
			Action: &proto.APLAction{Action: &proto.APLAction_AutocastOtherCooldowns{AutocastOtherCooldowns: &proto.APLActionAutocastOtherCooldowns{}}},
		}}},
	}}}}}
}

// mod-racial-trait-swap teaches a Blood Elf warrior the death knight's Arcane Torrent.
func TestArcaneTorrentOnRageUser(t *testing.T) {
	encounter := &proto.Encounter{Duration: 300, Targets: []*proto.Target{NewDefaultTarget()}}
	env, _, _ := NewEnvironment(bloodElfRageUserRaid(), encounter, false)
	warrior := env.Raid.Parties[0].Players[0].GetCharacter()

	for _, spell := range warrior.Spellbook {
		if spell.ActionID.IsEmptyAction() {
			t.Errorf("a spell with no ID is registered, flags %v", spell.Flags)
		}
	}
	torrent := warrior.GetSpell(arcaneTorrentRunicPower)
	if torrent == nil || !torrent.Flags.Matches(SpellFlagMCD) {
		t.Fatalf("Arcane Torrent %v isn't a major cooldown", arcaneTorrentRunicPower)
	}

	// it gives nothing without a runic power bar, so the sim leaves it alone
	result := RunRaidSim(&proto.RaidSimRequest{
		Raid:       bloodElfRageUserRaid(),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	for _, action := range result.RaidMetrics.Parties[0].Players[0].Actions {
		if ProtoToActionID(action.Id) == arcaneTorrentRunicPower {
			for _, target := range action.Targets {
				if target.Casts > 0 {
					t.Errorf("Arcane Torrent was cast %d times", target.Casts)
				}
			}
		}
	}
}
