package restoration

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// restoTalents is StandardTalents with Tree of Life taken or not.
func restoTalents(tree bool) string {
	if tree {
		return StandardTalents
	}
	// Tree of Life is the restoration tree's 23rd talent, Improved Tree of Life the 24th
	return withResto(StandardTalents, map[int]byte{22: '0', 23: '0'})
}

// withResto sets the points of the restoration talents at the given places in the tree, from 0.
func withResto(talents string, points map[int]byte) string {
	treeStart := len(talents) - 27
	b := []byte(talents)
	for i, p := range points {
		b[treeStart+i] = p
	}
	return string(b)
}

// rank is a talent's multiplier at its points, worked out as the druid does.
func rank(points int, pct float64) float64 {
	return 1 + pct*float64(points)
}

// The standard build's spell mod percents on the heals: Gift of Nature's 2% a point (SPELLMOD_DAMAGE on the
// direct heals, SPELLMOD_DOT on the hots), Genesis' 1% and Improved Rejuvenation's 5% (SPELLMOD_DOT), and Master
// Shapeshifter's healing done in Tree of Life. The SPELLMOD_DOT percents multiply.
var (
	giftOfNature = rank(5, 0.02)
	hotMods      = 1 * rank(5, 0.01) * giftOfNature
	rejuvMods    = hotMods * (1 * rank(3, 0.05))
	shapeshifter = rank(2, 0.02)
)

// The standard build's coefficients: Empowered Touch's 40 and 20 hundredths on Healing Touch and Nourish, and
// Empowered Rejuvenation's 20% on the hots, Regrowth's heal and the bloom.
const (
	healingTouchSP = 1.61 + 0.4
	nourishSP      = 0.672 + 0.2
	regrowthSP     = 0.3 * 1.2
	regrowthHotSP  = 0.1878 * 1.2
	rejuvenationSP = 0.376 * 1.2
	lifebloomSP    = 0.09518 * 1.2
	bloomSP        = 0.655 * 1.2
	wildGrowthSP   = 0.115 * 1.2
)

// healSetup describes the druid a spell test builds.
type healSetup struct {
	talents string
	dummies int32
	// incoming damage per second on the dummies; 0 leaves them without health bars
	damage float64
	// a second resto druid, in the second party, that heals nothing
	other bool
	// the rotation's priority list as APL JSON; the default casts nothing
	apl string
	// whom the druid Innervates; nobody by default
	innervate *proto.UnitReference
	glyphs    *proto.Glyphs
	// item ids, a relic among them
	items []int32
}

func (s healSetup) build(t *testing.T) (*core.Simulation, *RestorationDruid) {
	t.Helper()
	talents := s.talents
	if talents == "" {
		talents = restoTalents(false)
	}
	apl := s.apl
	if apl == "" {
		apl = `{"type": "TypeAPL", "priorityList": []}`
	}
	player := func(talents string, apl string) *proto.Player {
		return &proto.Player{
			Race:          proto.Race_RaceTauren,
			Class:         proto.Class_ClassDruid,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          &proto.Player_RestorationDruid{RestorationDruid: &proto.RestorationDruid{Options: &proto.RestorationDruid_Options{}}},
			Rotation:      core.APLRotationFromJsonString(apl),
			TalentsString: talents,
		}
	}
	main := player(talents, apl)
	main.Glyphs = s.glyphs
	for _, id := range s.items {
		main.Equipment.Items = append(main.Equipment.Items, &proto.ItemSpec{Id: id})
	}
	main.GetRestorationDruid().Options.InnervateTarget = s.innervate
	raid := core.SinglePlayerRaidProto(main, nil, nil, nil)
	if s.other {
		raid.Parties = append(raid.Parties, &proto.Party{Players: []*proto.Player{
			player(restoTalents(false), `{"type": "TypeAPL", "priorityList": []}`),
		}})
	}
	// room for the dummies, which take the first free slots in party order
	for len(raid.Parties) < 5 {
		raid.Parties = append(raid.Parties, &proto.Party{})
	}
	raid.TargetDummies = s.dummies
	raid.IncomingDamagePerSecond = s.damage
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 600
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	return sim, sim.Raid.Parties[0].Players[0].(*RestorationDruid)
}

// startFight takes the sim to time 0 with the druid able to cast.
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

// neverCrit takes the druid's crit chance below 0 for good and fixes the rolls of the spells given, so a heal is
// the same each time.
func neverCrit(sim *core.Simulation, druid *RestorationDruid, spells ...*core.Spell) {
	druid.AddStatDynamic(sim, stats.SpellCrit, -1e6)
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

// power is a coefficient's share of the druid's healing power, as SpellHealingBonusDone adds it.
func power(spell *core.Spell, target *core.Unit, coefficient float64) float64 {
	return core.PowerBonus(coefficient, spell.HealingPower(target))
}
