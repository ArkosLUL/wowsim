package retribution

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// Retribution tree positions in the talent string
const (
	improvedJudgements = 2
	swiftRetribution   = 21
)

const divineStormReset = 70769

// retTalents is StandardTalents with the Retribution talent at index i set to points.
func retTalents(i int, points byte) string {
	trees := strings.Split(StandardTalents, "-")
	ret := []byte(trees[2])
	ret[i] = '0' + points
	trees[2] = string(ret)
	return strings.Join(trees, "-")
}

func retPlayer(talents string, gear *proto.EquipmentSpec, apl string) *proto.Player {
	return &proto.Player{
		Race:          proto.Race_RaceBloodElf,
		Class:         proto.Class_ClassPaladin,
		Equipment:     gear,
		Spec:          DefaultOptions,
		Glyphs:        &proto.Glyphs{},
		TalentsString: talents,
		Rotation:      core.APLRotationFromJsonString(apl),
	}
}

func p1Gear() *proto.EquipmentSpec {
	return googleProto.Clone(core.GetGearSet("../../../ui/retribution_paladin/gear_sets", "p1").GearSet).(*proto.EquipmentSpec)
}

func newRetribution(t *testing.T, player *proto.Player) (*core.Simulation, *RetributionPaladin) {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.ServerSettings = &proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(0)}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	return sim, sim.Raid.Parties[0].Players[0].(*RetributionPaladin)
}

// mod-spell-tweaks gives Swift Retribution a flat SPELLMOD_EFFECT1 of 400 a rank on Consecration, whose tick
// base is 113.
func TestSwiftRetributionRaisesConsecrationTick(t *testing.T) {
	for points := byte(0); points <= 3; points++ {
		_, ret := newRetribution(t, retPlayer(retTalents(swiftRetribution, points), p1Gear(), `{"type":"TypeAPL","priorityList":[]}`))
		if ret.Talents.SwiftRetribution != int32(points) {
			t.Fatalf("talent string gave Swift Retribution %d points, want %d", ret.Talents.SwiftRetribution, points)
		}
		tick := ret.Consecration.AOEDot().Tick
		if want := 113 + 400*float64(points); tick.Min != want || tick.Max != want {
			t.Errorf("Swift Retribution %d/3: Consecration ticks for %v to %v base, want %v", points, tick.Min, tick.Max, want)
		}
	}
}

func withT10TwoPiece(gear *proto.EquipmentSpec) *proto.EquipmentSpec {
	gear.Items[proto.ItemSlot_ItemSlotChest] = &proto.ItemSpec{Id: 50328}
	gear.Items[proto.ItemSlot_ItemSlotHands] = &proto.ItemSpec{Id: 50327}
	return gear
}

// A Divine Storm reset rolls on each landed swing, as spell_proc's default done hit mask has it: Improved
// Judgements' at 20% a point (spell_proc -25956), the T10 2pc's 70765 at 40%. With the 2pc on, the conditions
// row blocks the talent's, leaving the 2pc's alone.
func TestDivineStormResetRate(t *testing.T) {
	const iterations = 200
	for _, tc := range []struct {
		name   string
		points byte
		t10    bool
		want   float64
	}{
		{"0/2", 0, false, 0},
		{"1/2", 1, false, 0.2},
		{"2/2", 2, false, 0.4},
		{"0/2 with the T10 2pc", 0, true, 0.4},
		{"2/2 with the T10 2pc", 2, true, 0.4},
	} {
		// the weapon alone leaves enough misses and dodges to tell a roll on every swing apart
		p1 := p1Gear()
		gear := &proto.EquipmentSpec{}
		for slot, item := range p1.Items {
			if slot != int(proto.ItemSlot_ItemSlotMainHand) {
				item = &proto.ItemSpec{}
			}
			gear.Items = append(gear.Items, item)
		}
		if tc.t10 {
			withT10TwoPiece(gear)
		}
		encounter := core.MakeSingleTargetEncounter(0)
		encounter.Duration = 300
		res := core.RunRaidSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(retPlayer(retTalents(improvedJudgements, tc.points), gear,
				`{"type":"TypeAPL","priorityList":[]}`), &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
			Encounter:  encounter,
			SimOptions: &proto.SimOptions{Iterations: iterations, IsTest: true, RandomSeed: 101},
		})
		if res.ErrorResult != "" {
			t.Fatalf("%s: %s", tc.name, res.ErrorResult)
		}

		var resets, swings, landed float64
		for _, action := range res.RaidMetrics.Parties[0].Players[0].Actions {
			for _, target := range action.Targets {
				switch {
				case action.Id.GetSpellId() == divineStormReset:
					resets += float64(target.Casts)
				case action.Id.GetOtherId() == proto.OtherAction_OtherActionAttack:
					swings += float64(target.Casts)
					landed += float64(target.Casts - target.Misses - target.Dodges - target.Parries)
				}
			}
		}
		if landed < 50*iterations || swings-landed < 0.1*swings {
			t.Fatalf("%s: %.0f of %.0f swings landed", tc.name, landed, swings)
		}
		want, tolerance := tc.want*landed, 5*math.Sqrt(tc.want*(1-tc.want)*landed)
		if math.Abs(resets-want) > tolerance {
			t.Errorf("%s: %.0f resets from %.0f landed swings, want %.0f give or take %.0f", tc.name, resets, landed, want, tolerance)
		}
	}
}

// 70769 ends Divine Storm's cooldown. It's a triggered cast with SPELL_ATTR3_SUPPRESS_CASTER_PROCS, so it
// raises no cast procs.
func TestDivineStormResetEndsCooldown(t *testing.T) {
	for _, tc := range []struct {
		name string
		ret  *proto.Player
	}{
		{"Improved Judgements 2/2", retPlayer(StandardTalents, p1Gear(), `{"type":"TypeAPL","priorityList":[]}`)},
		{"T10 2pc", retPlayer(retTalents(improvedJudgements, 0), withT10TwoPiece(p1Gear()), `{"type":"TypeAPL","priorityList":[]}`)},
	} {
		sim, ret := newRetribution(t, tc.ret)
		reset := ret.GetSpell(core.ActionID{SpellID: divineStormReset})
		if reset == nil {
			t.Fatalf("%s registered no reset", tc.name)
		}
		if !reset.Flags.Matches(core.SpellFlagNoOnCastComplete) || reset.ProcMask != core.ProcMaskEmpty {
			t.Errorf("%s: the reset raises cast procs (flags %v, proc mask %v)", tc.name, reset.Flags, reset.ProcMask)
		}
		target := sim.Encounter.TargetUnits[0]

		sim.Reset()
		sim.PrePull()
		sim.AddPendingAction(&core.PendingAction{NextActionAt: time.Second, Priority: core.ActionPriorityLow, OnAction: func(sim *core.Simulation) {
			ret.GCD.Reset()
			if !ret.DivineStorm.Cast(sim, target) {
				t.Fatalf("%s: Divine Storm didn't cast", tc.name)
			}
			if ret.DivineStorm.CD.IsReady(sim) {
				t.Fatalf("%s: Divine Storm is off cooldown right after its cast", tc.name)
			}
			reset.Cast(sim, &ret.Unit)
			if !ret.DivineStorm.CD.IsReady(sim) {
				t.Errorf("%s: Divine Storm is still on cooldown after the reset", tc.name)
			}
		}})
		for !sim.Step() {
		}
		sim.Cleanup()
	}
}
