package restoration

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Item ids of five pieces of each restoration set.
var (
	t7Regalia = []int32{40510, 40513, 40508, 40509, 40512}
	t8Regalia = []int32{45402, 45404, 45405, 45401, 45403}
	t9Garb    = []int32{48297, 48299, 48295, 48296, 48298}
	t10Garb   = []int32{50837, 50839, 50835, 50836, 50838}
)

// healSetup describes the shaman a spell test builds.
type healSetup struct {
	talents string
	glyphs  *proto.Glyphs
	// item ids, a totem among them
	items   []int32
	shield  proto.ShamanShield
	ppm     int32
	dummies int32
	// the rotation's priority list as APL JSON; the default casts nothing
	apl string
}

func (s healSetup) build(t *testing.T) (*core.Simulation, *RestorationShaman) {
	t.Helper()
	talents := s.talents
	if talents == "" {
		talents = StandardTalents
	}
	equipment := &proto.EquipmentSpec{}
	for _, id := range s.items {
		equipment.Items = append(equipment.Items, &proto.ItemSpec{Id: id})
	}
	apl := s.apl
	if apl == "" {
		apl = `{"type": "TypeAPL", "priorityList": []}`
	}
	raid := core.SinglePlayerRaidProto(&proto.Player{
		Race:      proto.Race_RaceTroll,
		Class:     proto.Class_ClassShaman,
		Equipment: equipment,
		Consumes:  &proto.Consumes{},
		Spec: &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{
			Options: &proto.RestorationShaman_Options{Shield: s.shield, Totems: BasicTotems, EarthShieldPPM: s.ppm},
		}},
		Rotation:      core.APLRotationFromJsonString(apl),
		TalentsString: talents,
		Glyphs:        s.glyphs,
	}, nil, nil, nil)
	raid.TargetDummies = s.dummies
	// long enough for the tests that wait for hours of game time
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 7200
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	return sim, sim.Raid.Parties[0].Players[0].(*RestorationShaman)
}

// startFight takes the sim to time 0 with the shaman able to cast.
func startFight(sim *core.Simulation) {
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
}

// runUntil steps the fight to the given time, which a marker action there keeps from being skipped.
func runUntil(sim *core.Simulation, at time.Duration) {
	core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: at, OnAction: func(*core.Simulation) {}})
	for sim.CurrentTime < at && !sim.Step() {
	}
}

func targetDummies(sim *core.Simulation) []*core.Unit {
	var dummies []*core.Unit
	for _, party := range sim.Raid.Parties {
		for _, player := range party.Players {
			if dummy, ok := player.(*core.TargetDummy); ok {
				dummies = append(dummies, &dummy.Unit)
			}
		}
	}
	return dummies
}

// neverCrit takes the shaman's crit chance below 0 for good and fixes the rolls of the spells given, so a heal is
// the same each time.
func neverCrit(sim *core.Simulation, shaman *RestorationShaman, spells ...*core.Spell) {
	shaman.AddStatDynamic(sim, stats.SpellCrit, -1e6)
	for _, spell := range spells {
		spell.Direct.Max = spell.Direct.Min
	}
}

// healed is what the spell has healed the target for so far.
func healed(spell *core.Spell, target *core.Unit) float64 {
	return spell.SpellMetrics[target.UnitIndex].TotalHealing
}

// castHeal returns the target's heal from one cast of the spell, with no cast time or cost.
func castHeal(sim *core.Simulation, spell *core.Spell, target *core.Unit) float64 {
	before := healed(spell, target)
	spell.SkipCastAndApplyEffects(sim, target)
	return healed(spell, target) - before
}
