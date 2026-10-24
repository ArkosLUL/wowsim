package holy

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// healSetup describes the paladin a spell test builds.
type healSetup struct {
	talents string
	items   []int32
	glyphs  *proto.Glyphs
	dummies int32
	buffs   *proto.RaidBuffs
	// the rotation's priority list as APL JSON; the default casts nothing
	apl string
}

func (s healSetup) build(t *testing.T) (*core.Simulation, *HolyPaladin) {
	t.Helper()
	talents := s.talents
	if talents == "" {
		talents = StandardTalents
	}
	apl := s.apl
	if apl == "" {
		apl = `{"type": "TypeAPL", "priorityList": []}`
	}
	equipment := &proto.EquipmentSpec{}
	for _, id := range s.items {
		equipment.Items = append(equipment.Items, &proto.ItemSpec{Id: id})
	}
	raid := core.SinglePlayerRaidProto(&proto.Player{
		Race:          proto.Race_RaceBloodElf,
		Class:         proto.Class_ClassPaladin,
		Equipment:     equipment,
		Consumes:      &proto.Consumes{},
		Spec:          BasicOptions,
		Rotation:      core.APLRotationFromJsonString(apl),
		TalentsString: talents,
		Glyphs:        s.glyphs,
	}, nil, s.buffs, nil)
	// a party holds five, the paladin and four dummies
	for len(raid.Parties)*5 < 1+int(s.dummies) {
		raid.Parties = append(raid.Parties, &proto.Party{})
	}
	raid.TargetDummies = s.dummies
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 600
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	return sim, sim.Raid.Parties[0].Players[0].(*HolyPaladin)
}

// startFight takes the sim to time 0 with the paladin able to cast.
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

// fixRolls sets each spell's base roll to its least, so a heal is the same each time.
func fixRolls(spells ...*core.Spell) {
	for _, spell := range spells {
		spell.Direct.Max = spell.Direct.Min
	}
}

// setCrit sets the paladin's spell crit chance to pct percent.
func setCrit(sim *core.Simulation, holy *HolyPaladin, pct float64) {
	holy.AddStatDynamic(sim, stats.SpellCrit, pct*core.CritRatingPerCritChance-holy.GetStat(stats.SpellCrit))
}

// setSpellPower sets the paladin's spell power to sp.
func setSpellPower(sim *core.Simulation, holy *HolyPaladin, sp float64) {
	holy.AddStatDynamic(sim, stats.SpellPower, sp-holy.GetStat(stats.SpellPower))
}

func healed(spell *core.Spell, target *core.Unit) float64 {
	return spell.SpellMetrics[target.UnitIndex].TotalHealing
}

func shielded(spell *core.Spell, target *core.Unit) float64 {
	return spell.SpellMetrics[target.UnitIndex].TotalShielding
}

func crits(spell *core.Spell, target *core.Unit) int32 {
	return spell.SpellMetrics[target.UnitIndex].Crits
}

// cast is the spell's effects on the target, with no cast time or cost.
func cast(sim *core.Simulation, spell *core.Spell, target *core.Unit) {
	spell.SkipCastAndApplyEffects(sim, target)
}

// castHeal returns the target's heal from one cast of the spell.
func castHeal(sim *core.Simulation, spell, healSpell *core.Spell, target *core.Unit) float64 {
	before := healed(healSpell, target)
	cast(sim, spell, target)
	return healed(healSpell, target) - before
}

// doneHeal is SpellHealingBonusDone of a heal of base with the given coefficient: the whole spell power share,
// then the spell's multiplier.
func doneHeal(spell *core.Spell, base float64) float64 {
	return math.Trunc((base + core.PowerBonus(spell.Direct.SP, spell.SpellPower())) * spell.DamageMultiplier)
}
