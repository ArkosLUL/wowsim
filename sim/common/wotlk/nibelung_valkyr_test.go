package wotlk

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// valkyrTestAgent is a bare player holding Nibelung, so its Val'kyrs get built.
type valkyrTestAgent struct{ core.Character }

func (a *valkyrTestAgent) GetCharacter() *core.Character { return &a.Character }
func (a *valkyrTestAgent) Initialize()                   {}
func (a *valkyrTestAgent) ApplyTalents()                 {}
func (a *valkyrTestAgent) Reset(_ *core.Simulation)      {}
func (a *valkyrTestAgent) OnGCDReady(_ *core.Simulation) {}

func init() {
	// a spec no other test in this package's binary plays
	core.RegisterAgentFactory(
		proto.Player_BalanceDruid{},
		proto.Spec_SpecBalanceDruid,
		func(char *core.Character, _ *proto.Player) core.Agent {
			a := &valkyrTestAgent{Character: *char}
			ConstructValkyrPets(&a.Character)
			return a
		},
		func(player *proto.Player, spec interface{}) {
			player.Spec = spec.(*proto.Player_BalanceDruid)
		},
	)
}

// newValkyrTestSim runs on a regular 102.7 ms map update when server is nil: the tests count updates.
func newValkyrTestSim(t *testing.T, server *proto.ServerSettings) (*core.Simulation, *valkyrTestAgent, *ValkyrPet) {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	if server == nil {
		core.OnRegularGrid(encounter, 10.27)
	} else {
		encounter.ServerSettings = server
	}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceTauren,
			Class:     proto.Class_ClassDruid,
			Equipment: &proto.EquipmentSpec{Items: []*proto.ItemSpec{{Id: nibelungID}}},
			Consumes:  &proto.Consumes{},
			Spec:      &proto.Player_BalanceDruid{},
			Rotation:  &proto.APLRotation{},
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	owner := sim.Raid.Parties[0].Players[0].(*valkyrTestAgent)
	for _, petAgent := range owner.PetAgents {
		if valkyr, ok := petAgent.(*ValkyrPet); ok {
			valkyr.registerSmite(false)
			return sim, owner, valkyr
		}
	}
	t.Fatal("no Val'kyr built for a player holding Nibelung")
	return nil, nil, nil
}

// npc_pet_gen_valkyr_guardian casts Smite where a swing would come (DoSpellAttackIfReady), so the creature's
// 2 s attack time paces it, and that runs out on a map update: a Smite on the Val'kyr's first update and
// one every 20 updates after, 15 in its 30 s. It has no pet bar (SummonProperties 2909), so the owner's
// Bloodlust, a raid cast, never lands on it and its Smites come no faster.
func TestValkyrSmitesOnItsAttackTimer(t *testing.T) {
	for _, tc := range []struct {
		name         string
		averageCasts float64
		bloodlust    bool
		want         int
	}{
		{"its whole 30 s", 16, false, 15},
		{"its whole 30 s under Bloodlust", 16, true, 15},
		{"11 casts", 11, false, 11},
		{"1 cast", 1, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sim, owner, valkyr := newValkyrTestSim(t, nil)
			var wentOff []time.Duration
			valkyr.smite.ApplyEffects = func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
				wentOff = append(wentOff, sim.CurrentTime)
			}
			sim.PrePull()
			for sim.CurrentTime < 1037*time.Millisecond && !sim.Step() {
			}

			start := valkyr.AIClockStart(sim)
			first := sim.NextServerTick(sim.CurrentTime)
			update := first - start
			if update != 102700*time.Microsecond {
				t.Fatalf("a map update of %v: the test wants the live 102.7 ms", update)
			}
			valkyr.EnableWithTimeout(sim, valkyr, valkyrLife(tc.averageCasts))
			if tc.bloodlust {
				core.BloodlustAura(&owner.Character, -1).Activate(sim)
				if !owner.HasActiveAuraWithTag(core.BloodlustAuraTag) {
					t.Fatal("the owner has no Bloodlust, so the test shows nothing")
				}
				if valkyr.HasActiveAuraWithTag(core.BloodlustAuraTag) {
					t.Error("the Val'kyr carries its owner's Bloodlust")
				}
			}
			for valkyr.IsEnabled() && !sim.Step() {
			}

			if len(wentOff) != tc.want {
				t.Fatalf("%d Smites, want %d: %v", len(wentOff), tc.want, wentOff)
			}
			for i, at := range wentOff {
				if want := first + time.Duration(i)*20*update; at != want {
					t.Errorf("Smite %d went off at %v, want %v: 20 updates after the one before", i+1, at, want)
				}
			}
		})
	}
}

// With MapUpdateInterval 0 nothing waits for an update: a Smite every 2 s to the millisecond, and the summon's
// time is up ahead of the Smite that comes due at that very moment. So 11 casts are still 11.
func TestValkyrSmitesOnExactTiming(t *testing.T) {
	sim, _, valkyr := newValkyrTestSim(t, &proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(0)})
	var wentOff []time.Duration
	valkyr.smite.ApplyEffects = func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
		wentOff = append(wentOff, sim.CurrentTime)
	}
	sim.PrePull()
	for sim.CurrentTime < 1037*time.Millisecond && !sim.Step() {
	}
	came := sim.CurrentTime
	if sim.NextServerTick(came+1) != came+1 {
		t.Fatal("the sim runs on map updates, so the test shows nothing")
	}
	valkyr.EnableWithTimeout(sim, valkyr, valkyrLife(11))
	for valkyr.IsEnabled() && !sim.Step() {
	}

	if len(wentOff) != 11 {
		t.Fatalf("%d Smites, want 11: %v", len(wentOff), wentOff)
	}
	for i, at := range wentOff {
		if want := came + time.Duration(i)*valkyrAttackTime; at != want {
			t.Errorf("Smite %d went off at %v, want %v", i+1, at, want)
		}
	}
}
