package retribution

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

// sealDotStacks is Holy Vengeance's stacks after a Crusader Strike, after a Divine Storm on top, and after
// the white hits that follow.
type sealDotStacks struct {
	crusaderStrike, divineStorm, swings int32
}

// runSealStrikes has a Seal of Vengeance paladin cast Crusader Strike at 1 s and Divine Storm at 3 s with
// no swing before them, then swing from 5 s on. Nothing misses.
func runSealStrikes(t *testing.T, tweaks *proto.SpellTweaksSettings) sealDotStacks {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 15
	encounter.ServerSettings = &proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(0), SpellTweaks: tweaks}

	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceBloodElf,
			Class:         proto.Class_ClassPaladin,
			Equipment:     core.GetGearSet("../../../ui/retribution_paladin/gear_sets", "p1").GearSet,
			Spec:          DefaultOptions,
			Glyphs:        &proto.Glyphs{},
			TalentsString: StandardTalents,
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	ret := sim.Raid.Parties[0].Players[0].(*RetributionPaladin)
	target := sim.Encounter.TargetUnits[0]
	dot := ret.SovDotSpell.Dot(target)

	sim.Reset()
	sim.PrePull()
	// or the pull starts the swings
	ret.AutoAttacks.AutoSwingMelee = false
	// no miss, no dodge, no parry
	ret.AddStatsDynamic(sim, stats.Stats{
		stats.MeleeHit:  20 * core.MeleeHitRatingPerHitChance,
		stats.Expertise: 100 * core.ExpertisePerQuarterPercentReduction,
	})

	var stacks sealDotStacks
	at := func(when time.Duration, do func(sim *core.Simulation)) {
		sim.AddPendingAction(&core.PendingAction{NextActionAt: when, Priority: core.ActionPriorityLow, OnAction: do})
	}
	cast := func(spell *core.Spell, into *int32) func(sim *core.Simulation) {
		return func(sim *core.Simulation) {
			ret.GCD.Reset()
			if !spell.Cast(sim, target) {
				t.Fatalf("%s didn't cast", spell.ActionID)
			}
			*into = dot.GetStacks()
		}
	}
	at(time.Second, cast(ret.CrusaderStrike, &stacks.crusaderStrike))
	at(3*time.Second, cast(ret.DivineStorm, &stacks.divineStorm))
	at(5*time.Second, func(sim *core.Simulation) {
		ret.AutoAttacks.AutoSwingMelee = true
		ret.AutoAttacks.EnableAutoSwing(sim)
	})
	at(14*time.Second, func(*core.Simulation) { stacks.swings = dot.GetStacks() })

	for !sim.Step() {
	}
	sim.Cleanup()
	return stacks
}

// Core stacks the seal's DoT off white hits and Hammer of the Righteous only
// (spell_pal_seal_of_vengeance_aura::HandleApplyDoT). Crusader Strike and Divine Storm stacking it is
// mod-spell-tweaks' spell_tweaks_seal_dot_on_strikes, which has no switch but SpellTweaks.Enable.
func TestSealDotStacksFromStrikes(t *testing.T) {
	on := runSealStrikes(t, nil)
	if on.crusaderStrike != 1 || on.divineStorm != 2 {
		t.Errorf("%d stacks after Crusader Strike and %d after Divine Storm, want 1 and 2", on.crusaderStrike, on.divineStorm)
	}
	if on.swings <= on.divineStorm {
		t.Errorf("%d stacks after the white hits, want more than the strikes' %d", on.swings, on.divineStorm)
	}

	off := runSealStrikes(t, &proto.SpellTweaksSettings{Enable: googleProto.Bool(false)})
	if off.crusaderStrike != 0 || off.divineStorm != 0 {
		t.Errorf("SpellTweaks.Enable off: %d stacks after Crusader Strike and %d after Divine Storm, want none",
			off.crusaderStrike, off.divineStorm)
	}
	if off.swings == 0 {
		t.Error("SpellTweaks.Enable off: no stack after the white hits, want them to go on stacking it")
	}
}
