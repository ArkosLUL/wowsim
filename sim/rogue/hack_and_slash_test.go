package rogue

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// 66923 only queues an extra attack, which the rogue's next update swings: never on the melee hit that procced it,
// and on the first world tick after the last one. It ends the chain, so its own swing procs no other.
func TestHackAndSlashSwingsOnTheNextUpdate(t *testing.T) {
	rsr := hackAndSlashTalentsRequest()
	rsr.SimOptions = &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101}
	sim := core.NewSim(rsr)
	sim.Reset()
	rogue := sim.Raid.Parties[0].Players[0].(*Rogue)
	extra := rogue.GetSpell(core.ActionID{SpellID: 13964})
	if extra == nil {
		t.Fatal("no Hack and Slash extra attack")
	}

	lastHit := core.NeverExpires
	hitting := false
	for _, spell := range rogue.Spellbook {
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
		// one proc waits for its pass at most, so two in a pass would be a chain
		if len(extras) > 0 && extras[len(extras)-1] == now {
			t.Errorf("two extra attacks at %v", now)
		}
		extras = append(extras, now)
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
