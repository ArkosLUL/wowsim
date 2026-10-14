package core

import "time"

// CastLandedHandler hears of every target a cast of the unit's lands on, once a cast and target, whatever the
// cast did there: a hit, a heal, an aura or a shield. Spell::DoAllEffectOnTarget raises one hit-phase proc a
// target whatever the result was, so this is where a proc on any landed cast reads. A miss doesn't land, nor
// does a periodic tick, but a tick the server deals as a triggered cast (ticksAsCasts) does.
type CastLandedHandler func(sim *Simulation, spell *Spell, target *Unit)

type castLanded struct {
	handlers []CastLandedHandler

	// the last landing told, so a cast that heals and puts a hot on one unit lands once
	spell  *Spell
	target *Unit
	at     time.Duration
}

func (unit *Unit) RegisterOnCastLanded(handler CastLandedHandler) {
	if len(unit.castLanded.handlers) == 0 {
		unit.RegisterResetEffect(func(*Simulation) {
			unit.castLanded.spell = nil
		})
	}
	unit.castLanded.handlers = append(unit.castLanded.handlers, handler)
}

// LandOn tells the unit's OnCastLanded handlers that the spell landed on target without a result to deal: an
// aura, a hot or a shield put on. A direct hit or heal tells them on its own.
func (spell *Spell) LandOn(sim *Simulation, target *Unit) {
	cl := &spell.Unit.castLanded
	if len(cl.handlers) == 0 || cl.spell == spell && cl.target == target && cl.at == sim.CurrentTime {
		return
	}
	cl.spell, cl.target, cl.at = spell, target, sim.CurrentTime
	for _, handler := range cl.handlers {
		handler(sim, spell, target)
	}
}

func (spell *Spell) landResult(sim *Simulation, isPeriodic bool, result *SpellResult) {
	if len(spell.Unit.castLanded.handlers) != 0 && (!isPeriodic || spell.ticksAsCasts()) && result.Landed() {
		spell.LandOn(sim, result.Target)
	}
}
