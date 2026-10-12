package balance

import (
	"math"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// nakedDruidWith is the tests' druid with nothing on but one trinket.
func nakedDruidWith(trinket int32, apl string) *proto.Player {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotTrinket1] = &proto.ItemSpec{Id: trinket}
	return &proto.Player{
		Race:          proto.Race_RaceTauren,
		Class:         proto.Class_ClassDruid,
		Spec:          PlayerOptionsAdaptive,
		Consumes:      &proto.Consumes{},
		Rotation:      core.APLRotationFromJsonString(apl),
		TalentsString: StandardTalents,
		Equipment:     equipment,
	}
}

// An item's damage rolls Omen of Clarity like the druid's own spells, at 3.5 PPM over its own cast time entry:
// Extract of Necromantic Power's bolt (60488) has 2.2 s, so 12.83%, where an instant gets 8.75%. Live,
// Clearcasting came at 4 of 29 bolts. Only Insect Swarm is cast, whose ticks roll nothing.
func TestItemDamageRollsOmenAtItsCastTime(t *testing.T) {
	const iterations = 1000
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 600
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(nakedDruidWith(40373, `{"type": "TypeAPL", "priorityList": [{"action": {`+
			`"condition": {"not": {"val": {"dotIsActive": {"spellId": {"spellId": 48468}}}}}, `+
			`"castSpell": {"spellId": {"spellId": 48468}}}}]}`), nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: iterations, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	player := res.RaidMetrics.Parties[0].Players[0]
	landed := map[int32]float64{}
	for _, action := range player.Actions {
		for _, target := range action.Targets {
			landed[action.Id.GetSpellId()] += float64(target.Casts - target.Misses)
		}
	}
	var procs float64
	for _, aura := range player.Auras {
		if aura.Id.GetSpellId() == 16870 {
			procs = aura.ProcsAvg * iterations
		}
	}
	swarms, bolts := landed[48468], landed[60488]
	if swarms < 20*iterations || bolts < 5*iterations {
		t.Fatalf("%.0f Insect Swarms and %.0f bolts landed", swarms, bolts)
	}

	const instant, bolt = 3.5 * 1.5 / 60, 3.5 * 2.2 / 60
	want := instant*swarms + bolt*bolts
	tolerance := 5 * math.Sqrt(instant*(1-instant)*swarms+bolt*(1-bolt)*bolts)
	if math.Abs(procs-want) > tolerance {
		t.Errorf("%.0f Clearcasting procs from %.0f Insect Swarms and %.0f bolts, want %.0f give or take %.0f",
			procs, swarms, bolts, want, tolerance)
	}
	// the run can tell the bolt's own cast time from an instant's
	if asInstant := instant * (swarms + bolts); math.Abs(procs-asInstant) <= tolerance {
		t.Errorf("%.0f Clearcasting procs also fit the bolts rolling 8.75%% (%.0f give or take %.0f)", procs, asInstant, tolerance)
	}
	t.Logf("%.0f Insect Swarms and %.0f bolts landed, %.0f Clearcasting procs, want %.0f", swarms, bolts, procs, want)
}

// Hurricane's cast (48467) lands on the druid, and that hit gives one Illustration of the Dragon Soul stack.
// Its ticks are triggered casts of 48466, which the trinket doesn't take: live, a Hurricane gave one stack.
func TestHurricaneCastGivesOneIllustrationStack(t *testing.T) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 9
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(nakedDruidWith(40432,
			`{"type": "TypeAPL", "priorityList": [{"action": {"castSpell": {"spellId": {"spellId": 48467}}}}]}`), nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, Debug: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	if ticks := strings.Count(res.Logs, "{SpellID: 48466}"); ticks < 5 {
		t.Fatalf("%d lines of Hurricane's ticks in the log", ticks)
	}
	if first := strings.Count(res.Logs, "{SpellID: 60486} stacks: 0 --> 1"); first != 1 {
		t.Errorf("the first Illustration of the Dragon Soul stack came %d times, want once", first)
	}
	if strings.Contains(res.Logs, "{SpellID: 60486} stacks: 1 --> 2") {
		t.Error("a Hurricane gave a second Illustration of the Dragon Soul stack")
	}
}

// A proc off the cast's hit that goes on the hit's target finds the cast's target there: Shiny Shard of
// the Gods' Searing Flames, a dot the druid has on enemies only.
func TestHurricaneCastProcsGoOnItsTarget(t *testing.T) {
	player := nakedDruidWith(49488, `{"type": "TypeAPL", "priorityList": [{"action": {"castSpell": {"spellId": {"spellId": 48467}}}}]}`)
	player.Equipment.Items[proto.ItemSlot_ItemSlotTrinket2] = &proto.ItemSpec{Id: 49464}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 120
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 20, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	var burned float64
	for _, action := range res.RaidMetrics.Parties[0].Players[0].Actions {
		if action.Id.GetSpellId() == 69729 {
			for _, target := range action.Targets {
				burned += target.Damage
			}
		}
	}
	// the ticks are triggered casts, which the set doesn't take, so only a cast's hit gives the dot
	if burned == 0 {
		t.Error("Searing Flames never burned the target of a druid who only casts Hurricane")
	}
}

// An item's magic-class heal rolls Omen of Clarity like its damage, 8.75% for an instant: Bauble of True Blood's
// Release of Light (71607) and the Echoes of Light (71610) Althor's Abacus answers it with. Nothing else is cast.
func TestItemHealsRollOmen(t *testing.T) {
	const iterations = 1000
	player := nakedDruidWith(50354, `{"type": "TypeAPL", "priorityList": [`+
		`{"action": {"castSpell": {"spellId": {"itemId": 50354}, "target": {"type": "Self"}}}}]}`)
	player.Equipment.Items[proto.ItemSlot_ItemSlotTrinket2] = &proto.ItemSpec{Id: 50359}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 600
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: iterations, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	metrics := res.RaidMetrics.Parties[0].Players[0]
	heals := map[int32]float64{}
	for _, action := range metrics.Actions {
		for _, target := range action.Targets {
			if target.Healing > 0 {
				heals[action.Id.GetSpellId()] += float64(target.Casts)
			}
		}
	}
	var procs float64
	for _, aura := range metrics.Auras {
		if aura.Id.GetSpellId() == 16870 {
			procs = aura.ProcsAvg * iterations
		}
	}
	baubles, echoes := heals[71607], heals[71610]
	if baubles < 4*iterations || echoes < iterations/2 {
		t.Fatalf("%.0f Releases of Light and %.0f Echoes of Light healed", baubles, echoes)
	}

	const instant = 3.5 * 1.5 / 60
	rolls := baubles + echoes
	want := instant * rolls
	tolerance := 5 * math.Sqrt(instant*(1-instant)*rolls)
	if math.Abs(procs-want) > tolerance {
		t.Errorf("%.0f Clearcasting procs from %.0f item heals, want %.0f give or take %.0f", procs, rolls, want, tolerance)
	}
	// the run can tell the two heals from the Bauble's alone
	if alone := instant * baubles; math.Abs(procs-alone) <= tolerance {
		t.Errorf("%.0f Clearcasting procs also fit the Bauble's heals alone (%.0f give or take %.0f)", procs, alone, tolerance)
	}
	t.Logf("%.0f Releases of Light, %.0f Echoes of Light, %.0f Clearcasting procs, want %.0f", baubles, echoes, procs, want)
}

// Glowing Twilight Scale's Twilight Renewal (75493) is a heal over time with no damage class: its ticks heal and
// roll nothing, so only the Bauble's heals that start it do.
func TestItemHealOverTimeRollsNoOmen(t *testing.T) {
	const iterations = 1000
	player := nakedDruidWith(54573, `{"type": "TypeAPL", "priorityList": [`+
		`{"action": {"castSpell": {"spellId": {"itemId": 54573}}}}, `+
		`{"action": {"castSpell": {"spellId": {"itemId": 50354}, "target": {"type": "Self"}}}}]}`)
	player.Equipment.Items[proto.ItemSlot_ItemSlotTrinket2] = &proto.ItemSpec{Id: 50354}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 600
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: iterations, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	metrics := res.RaidMetrics.Parties[0].Players[0]
	var baubles, renewed float64
	for _, action := range metrics.Actions {
		for _, target := range action.Targets {
			switch action.Id.GetSpellId() {
			case 71607:
				baubles += float64(target.Casts)
			case 75493:
				renewed += target.Healing
			}
		}
	}
	var procs float64
	for _, aura := range metrics.Auras {
		if aura.Id.GetSpellId() == 16870 {
			procs = aura.ProcsAvg * iterations
		}
	}
	// 6 ticks of 356 for each Bauble heal but the one at the fight's end
	if baubles < 4*iterations || renewed < 6*356*(baubles-iterations) {
		t.Fatalf("%.0f Releases of Light healed, Twilight Renewal healed %.0f", baubles, renewed)
	}
	const instant = 3.5 * 1.5 / 60
	want := instant * baubles
	if tolerance := 5 * math.Sqrt(instant*(1-instant)*baubles); math.Abs(procs-want) > tolerance {
		t.Errorf("%.0f Clearcasting procs from %.0f Bauble heals, want %.0f give or take %.0f", procs, baubles, want, tolerance)
	}
}
