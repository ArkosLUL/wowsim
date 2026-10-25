package healing

import (
	"os"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const emptyAPL = `{"type": "TypeAPL", "priorityList": []}`

// dummyRaid is a priest of the tree ("disc" or "holy") with the given APL and four target dummies taking the damage
// a second, at the pull.
// The last result lists whom Prayer of Mending healed, in order.
func dummyRaid(t *testing.T, tree, apl string, damage float64) (*core.Simulation, *HealingPriest, []*core.Unit, *[]*core.Unit) {
	t.Helper()
	talents, glyphs, options := HolyTalents, HolyGlyphs, PlayerOptionsHoly
	if tree == "disc" {
		talents, glyphs, options = DiscTalents, DiscGlyphs, PlayerOptionsDisc
	}
	raid := core.SinglePlayerRaidProto(&proto.Player{
		Race:          proto.Race_RaceUndead,
		Class:         proto.Class_ClassPriest,
		Equipment:     &proto.EquipmentSpec{},
		Consumes:      &proto.Consumes{},
		Spec:          options,
		Rotation:      core.APLRotationFromJsonString(apl),
		TalentsString: talents,
		Glyphs:        glyphs,
	}, nil, nil, nil)
	raid.TargetDummies = 4
	raid.IncomingDamagePerSecond = damage
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 600
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       raid,
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	priest := sim.Raid.Parties[0].Players[0].(*HealingPriest)
	var pomHeals []*core.Unit
	priest.RegisterAura(core.Aura{
		Label:    "pom heals",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			pomHeals = pomHeals[:0]
			aura.Activate(sim)
		},
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == priest.PrayerOfMending {
				pomHeals = append(pomHeals, result.Target)
			}
		},
	})
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	var dummies []*core.Unit
	for _, party := range sim.Raid.Parties {
		for _, player := range party.Players {
			if dummy, ok := player.(*core.TargetDummy); ok {
				dummies = append(dummies, &dummy.Unit)
			}
		}
	}
	return sim, priest, dummies, &pomHeals
}

// runTo steps the fight to the given time, which a marker action there keeps from being skipped past.
func runTo(sim *core.Simulation, at time.Duration) {
	core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: at, OnAction: func(*core.Simulation) {}})
	for sim.CurrentTime < at && !sim.Step() {
	}
}

// Prayer of Mending heals on a hit its holder takes. With incoming damage that is the tank's next hit, and a
// holder nobody hits keeps its charge rather than losing it to a timer.
func TestPrayerOfMendingProcsOnTheHoldersHit(t *testing.T) {
	sim, priest, dummies, healed := dummyRaid(t, "holy", emptyAPL, 100)
	tank := dummies[0]
	priest.PrayerOfMending.SkipCastAndApplyEffects(sim, tank)
	runTo(sim, 1900*time.Millisecond)
	if len(*healed) != 0 {
		t.Errorf("%d heals before the tank's first hit at 2 s", len(*healed))
	}
	runTo(sim, 2100*time.Millisecond)
	if len(*healed) != 1 || (*healed)[0] != tank {
		t.Errorf("%d heals after the tank's hit, want one on the tank", len(*healed))
	}

	// the charge went to the priest, the first in raid order with nobody missing health, whom nothing hits
	runTo(sim, 22*time.Second)
	if len(*healed) != 1 {
		t.Errorf("%d Prayer of Mending heals by 22 s, want only the one from the tank's hit", len(*healed))
	}
}

// Without incoming damage the 5 s timer stands in for the hit.
func TestPrayerOfMendingProcsOnATimerWithoutDamage(t *testing.T) {
	sim, priest, dummies, healed := dummyRaid(t, "holy", emptyAPL, 0)
	priest.PrayerOfMending.SkipCastAndApplyEffects(sim, dummies[0])
	runTo(sim, 4900*time.Millisecond)
	if len(*healed) != 0 {
		t.Errorf("%d heals before 5 s", len(*healed))
	}
	runTo(sim, 5100*time.Millisecond)
	if len(*healed) != 1 {
		t.Errorf("%d heals at 5 s, want 1", len(*healed))
	}
}

// 48111 has 5 charges. A proc takes one before AuraEffect::HandleRaidProcFromChargeWithValueAuraProc jumps with
// the rest, so a cast heals 5 times.
func TestPrayerOfMendingHealsFiveTimesACast(t *testing.T) {
	sim, priest, dummies, healed := dummyRaid(t, "holy", emptyAPL, 0)
	priest.PrayerOfMending.SkipCastAndApplyEffects(sim, dummies[0])
	runTo(sim, time.Minute)
	if len(*healed) != 5 {
		t.Errorf("%d heals from one cast, want 5", len(*healed))
	}
}

func treeAPL(t *testing.T, tree string) string {
	t.Helper()
	data, err := os.ReadFile("../../../ui/healing_priest/apls/" + tree + ".apl.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// castsOn is how many times the spells, the ones the priest has, were cast on the unit.
func castsOn(unit *core.Unit, spells ...*core.Spell) (total int32) {
	for _, spell := range spells {
		if spell != nil {
			total += spell.SpellMetrics[unit.UnitIndex].Casts
		}
	}
	return
}

// The default rotations put Prayer of Mending (and a Disc priest's Power Word: Shield) on the tank and heal nobody
// while nobody is hurt.
func TestDefaultAPLsWaitWhenNobodyIsHurt(t *testing.T) {
	for _, tree := range []string{"disc", "holy"} {
		sim, priest, dummies, _ := dummyRaid(t, tree, treeAPL(t, tree), 1)
		runTo(sim, 20*time.Second)
		tank := dummies[0]
		for i, unit := range dummies {
			got := castsOn(unit, priest.PenanceHeal, priest.GreaterHeal, priest.FlashHeal, priest.CircleOfHealing, priest.Renew)
			if got != 0 {
				t.Errorf("%s: dummy %d was healed %d times with nobody hurt", tree, i+1, got)
			}
		}
		if castsOn(tank, priest.PrayerOfMending) == 0 {
			t.Errorf("%s: no Prayer of Mending on the tank", tree)
		}
		if tree == "disc" && castsOn(tank, priest.PowerWordShield) == 0 {
			t.Errorf("%s: no Power Word: Shield on the tank", tree)
		}
	}
}

// Each tree answers the most hurt unit with the heal that fits what it is missing.
func TestDefaultAPLsSizeTheHealToTheMissingHealth(t *testing.T) {
	type spells func(*HealingPriest) []*core.Spell
	flash := func(p *HealingPriest) []*core.Spell { return []*core.Spell{p.FlashHeal} }
	greater := func(p *HealingPriest) []*core.Spell { return []*core.Spell{p.GreaterHeal} }
	penance := func(p *HealingPriest) []*core.Spell { return []*core.Spell{p.PenanceHeal} }
	circle := func(p *HealingPriest) []*core.Spell { return []*core.Spell{p.CircleOfHealing} }
	none := func(p *HealingPriest) []*core.Spell { return nil }
	for _, c := range []struct {
		name    string
		tree    string
		missing float64
		// spells on cooldown
		down spells
		want spells
		not  spells
	}{
		{"disc too little", "disc", 2500, none, none, func(p *HealingPriest) []*core.Spell { return []*core.Spell{p.FlashHeal, p.GreaterHeal, p.PenanceHeal} }},
		{"disc small", "disc", 4000, none, flash, func(p *HealingPriest) []*core.Spell { return []*core.Spell{p.GreaterHeal, p.PenanceHeal} }},
		{"disc medium", "disc", 7000, none, penance, greater},
		{"disc large with Penance down", "disc", 12000, penance, greater, none},
		{"holy too little", "holy", 1500, none, none, func(p *HealingPriest) []*core.Spell {
			return []*core.Spell{p.FlashHeal, p.GreaterHeal, p.CircleOfHealing, p.Renew}
		}},
		{"holy medium", "holy", 5000, none, circle, greater},
		{"holy large with Circle of Healing down", "holy", 16000, circle, greater, none},
		{"holy small with Circle of Healing down", "holy", 6500, circle, flash, greater},
	} {
		sim, priest, dummies, _ := dummyRaid(t, c.tree, treeAPL(t, c.tree), 1)
		runTo(sim, 5*time.Second)
		priest.AddStatDynamic(sim, stats.SpellPower, 3000)
		priest.AddStatDynamic(sim, stats.Mana, 20000)
		priest.AddMana(sim, 20000, priest.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen}))
		for _, spell := range c.down(priest) {
			if spell != nil {
				spell.CD.Set(sim.CurrentTime + time.Minute)
			}
		}
		// a Renew on everyone keeps the multihot from spending the window on units missing a hit point
		if priest.Renew != nil {
			for _, unit := range dummies {
				priest.Renew.Dot(unit).Apply(sim)
			}
		}
		hurt := dummies[2]
		hurt.RemoveHealth(sim, c.missing)
		runTo(sim, sim.CurrentTime+3*time.Second)

		for _, spell := range c.want(priest) {
			if castsOn(hurt, spell) == 0 {
				t.Errorf("%s: %s never cast on the unit missing %v", c.name, spell.ActionID, c.missing)
			}
		}
		for _, spell := range c.not(priest) {
			if spell != nil && castsOn(hurt, spell) != 0 {
				t.Errorf("%s: %s cast on the unit missing %v", c.name, spell.ActionID, c.missing)
			}
		}
	}
}

// The unit with the lower health percent is healed first, the raid dummies having 20000 health.
func TestDefaultAPLsHealTheLowerPercentFirst(t *testing.T) {
	for _, tree := range []string{"disc", "holy"} {
		sim, priest, dummies, _ := dummyRaid(t, tree, treeAPL(t, tree), 1)
		runTo(sim, 5*time.Second)
		priest.AddStatDynamic(sim, stats.SpellPower, 3000)
		dummies[1].RemoveHealth(sim, 6000)
		dummies[2].RemoveHealth(sim, 9000)
		runTo(sim, sim.CurrentTime+200*time.Millisecond)

		all := []*core.Spell{priest.PenanceHeal, priest.GreaterHeal, priest.FlashHeal, priest.CircleOfHealing}
		if castsOn(dummies[2], all...) == 0 || castsOn(dummies[1], all...) != 0 {
			t.Errorf("%s: %d heals on the 70%% unit, %d on the 55%% one, want the lower one first",
				tree, castsOn(dummies[1], all...), castsOn(dummies[2], all...))
		}
	}
}
