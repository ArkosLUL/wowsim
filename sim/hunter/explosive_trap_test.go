package hunter

import (
	"testing"
	"time"

	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// GameObject::Update arms a hunter's trap startDelay (1 s) after it's laid only while the hunter is in combat. One
// laid half a second before the pull goes off with it; one laid in the fight waits its second.
func TestExplosiveTrapArmsAtOnceOutOfCombat(t *testing.T) {
	options := googleProto.Clone(PlayerOptionsBasic.Hunter.Options).(*proto.Hunter_Options)
	options.PetType = proto.Hunter_Options_PetNone
	for _, tc := range []struct {
		name    string
		apl     string
		layAt   time.Duration
		quietAt time.Duration
		goneAt  time.Duration
	}{
		{"before the pull", `{"type": "TypeAPL", "priorityList": [], "prepullActions": [` +
			`{"action": {"castSpell": {"spellId": {"spellId": 49067}}}, "doAtValue": {"const": {"val": "-0.5s"}}}]}`,
			-500 * time.Millisecond, -time.Millisecond, 100 * time.Millisecond},
		{"in the fight", `{"type": "TypeAPL", "priorityList": []}`, 2 * time.Second, 2900 * time.Millisecond, 3500 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sim := core.NewSim(&proto.RaidSimRequest{
				Raid: core.SinglePlayerRaidProto(&proto.Player{
					Race:          proto.Race_RaceOrc,
					Class:         proto.Class_ClassHunter,
					Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv").GearSet,
					Consumes:      &proto.Consumes{},
					Spec:          &proto.Player_Hunter{Hunter: &proto.Hunter{Options: options}},
					Rotation:      core.APLRotationFromJsonString(tc.apl),
					TalentsString: SVTalents,
				}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
				Encounter:  core.MakeSingleTargetEncounter(0),
				SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
			})
			sim.Reset()
			sim.PrePull()
			hunter := sim.Raid.Parties[0].Players[0].(*Hunter)
			runTo := func(at time.Duration) {
				core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: at, OnAction: func(*core.Simulation) {}})
				for sim.CurrentTime < at && !sim.Step() {
				}
			}
			if tc.layAt >= 0 {
				runTo(tc.layAt)
				hunter.GCD.Reset()
				if !hunter.ExplosiveTrap.Cast(sim, hunter.CurrentTarget) {
					t.Fatal("Explosive Trap didn't cast")
				}
			}
			trap := hunter.ExplosiveTrap.AOEDot()
			runTo(tc.quietAt)
			if trap.IsActive() {
				t.Errorf("the trap went off by %s", tc.quietAt)
			}
			runTo(tc.goneAt)
			if !trap.IsActive() {
				t.Errorf("the trap hadn't gone off by %s", tc.goneAt)
			}
		})
	}
}
