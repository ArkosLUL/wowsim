package dps

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// 16459 only queues an extra attack, which the warrior's next update swings: never on the melee hit that
// procced it, and on the first world tick after the last one.
func TestSwordSpecializationSwingsOnTheNextUpdate(t *testing.T) {
	rsr := swordSpecializationTalentsRequest()
	rsr.SimOptions = &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101}
	sim := core.NewSim(rsr)
	sim.Reset()
	war := sim.Raid.Parties[0].Players[0].(*DpsWarrior)
	extra := war.GetSpell(core.ActionID{SpellID: 12281})
	if extra == nil {
		t.Fatal("no Sword Specialization extra attack")
	}

	lastHit := core.NeverExpires
	hitting := false
	for _, spell := range war.Spellbook {
		if spell == extra || !spell.ProcMask.Matches(core.ProcMaskMelee) {
			continue
		}
		effects := spell.ApplyEffects
		spell.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			lastHit, hitting = sim.CurrentTime, true
			effects(sim, target, spell)
			hitting = false
		}
	}
	var extras []time.Duration
	effects := extra.ApplyEffects
	extra.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		now := sim.CurrentTime
		extras = append(extras, now)
		// a hit ahead of the warrior's turn on a tick (a Slam landing, a session cast) procs one for that tick
		if hitting || sim.NextWorldTick(now) != now || lastHit > now || now > sim.NextWorldTick(lastHit+1) {
			t.Errorf("an extra attack at %v, inside a hit %v, the last melee hit at %v", now, hitting, lastHit)
		}
		effects(sim, target, spell)
	}

	sim.PrePull()
	for !sim.Step() {
	}
	if len(extras) < 5 {
		t.Fatalf("%d extra attacks in a fight", len(extras))
	}
}
