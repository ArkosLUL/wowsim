package feral

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// A swing's Omen of Clarity proc puts the cat's next move off by its latency, whatever the world tick. At
// 7 ms Clearcasting's 15 s end between two ticks, so its time left is never its duration.
func TestSwingProcDelaysTheNextMoveAtAnyWorldTick(t *testing.T) {
	for _, worldTickMs := range []int32{10, 7} {
		encounter := core.MakeSingleTargetEncounter(0)
		encounter.Duration = 300
		encounter.ServerSettings = &proto.ServerSettings{WorldUpdateIntervalMs: googleProto.Int32(worldTickMs), WorldLoopMs: googleProto.Float64(0)}

		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceTauren,
				Class:         proto.Class_ClassDruid,
				Equipment:     core.GetGearSet("../../../ui/feral_druid/gear_sets", "p1").GearSet,
				Spec:          PlayerOptionsMonoCat,
				Glyphs:        StandardGlyphs,
				TalentsString: StandardTalents,
				Rotation:      core.GetAplRotation("../../../ui/feral_druid/apls", "default").Rotation,
			}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
			Encounter:  encounter,
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		cat := sim.Raid.Parties[0].Players[0].(*FeralDruid)

		procs := 0
		cat.RegisterAura(core.Aura{
			Label:    "swing proc test",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, _ *core.SpellResult) {
				if !spell.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) || !cat.ClearcastingAura.RefreshedNow(sim) {
					return
				}
				procAt := sim.CurrentTime
				want := max(cat.NextGCDAt(), procAt+cat.latency)
				// low priority, so it runs once the swing and the rotation call after it are through
				sim.AddPendingAction(&core.PendingAction{
					NextActionAt: procAt,
					Priority:     core.ActionPriorityLow,
					OnAction: func(sim *core.Simulation) {
						procs++
						if cat.rotationAction == nil || cat.rotationAction.NextActionAt != want {
							var got time.Duration
							if cat.rotationAction != nil {
								got = cat.rotationAction.NextActionAt
							}
							t.Errorf("%d ms world tick: after the proc at %s the next move is at %s, want %s", worldTickMs, procAt, got, want)
						}
					},
				})
			},
		})

		sim.Reset()
		sim.PrePull()
		for !sim.Step() {
		}
		sim.Cleanup()

		if procs < 5 {
			t.Errorf("%d ms world tick: only %d swings procced Clearcasting", worldTickMs, procs)
		}
	}
}
