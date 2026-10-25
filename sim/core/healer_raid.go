package core

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

const (
	// Max health of the tank dummy and of each raid dummy at phase 1; each later phase adds
	// dummyHealthPerPhase.
	DefaultDummyTankHealth  = 40000
	DefaultDummyRaidHealth  = 20000
	dummyTankHealthPerPhase = 5000
	dummyRaidHealthPerPhase = 2500

	// dummyHealthWithoutDamage is what a dummy has when nothing damages it, as before health bars.
	dummyHealthWithoutDamage = 10000

	incomingTankPeriod  = 2 * time.Second
	incomingRaidPeriod  = 4 * time.Second
	incomingRaidTargets = 4
)

// DummyHealthForPhase is the max health of the tank dummy and of each raid dummy at a content phase (1 to 5).
func DummyHealthForPhase(phase int32) (tank int32, raid int32) {
	phase = max(phase, 1)
	return DefaultDummyTankHealth + dummyTankHealthPerPhase*(phase-1), DefaultDummyRaidHealth + dummyRaidHealthPerPhase*(phase-1)
}

// incomingDamage is the damage a healer sim deals its target dummies. Half of it hits dummy 1 (the tank) every
// 2 s, the other half hits four random raid dummies every 4 s. It takes no mitigation and ignores shields. A
// dummy stops at 1 health.
type incomingDamage struct {
	perSecond  float64
	tankHealth int32
	raidHealth int32

	tank  *TargetDummy
	raid  []*TargetDummy
	spell *Spell
	// the damage dealt so far this iteration
	dealt float64

	picks []*TargetDummy
}

func newIncomingDamage(raidConfig *proto.Raid) *incomingDamage {
	incoming := &incomingDamage{
		perSecond:  raidConfig.IncomingDamagePerSecond,
		tankHealth: raidConfig.DummyTankHealth,
		raidHealth: raidConfig.DummyRaidHealth,
	}
	if incoming.tankHealth == 0 {
		incoming.tankHealth = DefaultDummyTankHealth
	}
	if incoming.raidHealth == 0 {
		incoming.raidHealth = DefaultDummyRaidHealth
	}
	return incoming
}

func (incoming *incomingDamage) dummyHealth(dummyIndex int) float64 {
	switch {
	case incoming == nil:
		return dummyHealthWithoutDamage
	case dummyIndex == 0:
		return float64(incoming.tankHealth)
	default:
		return float64(incoming.raidHealth)
	}
}

// hits is whether the damage lands on the unit, one of the dummies.
func (incoming *incomingDamage) hits(unit *Unit) bool {
	if incoming == nil {
		return false
	}
	if incoming.tank != nil && unit == &incoming.tank.Unit {
		return true
	}
	for _, dummy := range incoming.raid {
		if unit == &dummy.Unit {
			return true
		}
	}
	return false
}

// add records dummy i, the first being the tank.
func (incoming *incomingDamage) add(dummyIndex int, dummy *TargetDummy) {
	if dummyIndex == 0 {
		incoming.tank = dummy
	} else {
		incoming.raid = append(incoming.raid, dummy)
	}
}

// register puts the damage spell on the encounter's first target, which deals every hit.
func (incoming *incomingDamage) register(env *Environment) {
	if incoming == nil || len(env.Encounter.TargetUnits) == 0 {
		return
	}
	incoming.spell = env.Encounter.TargetUnits[0].RegisterSpell(SpellConfig{
		ActionID:    ActionID{OtherID: proto.OtherAction_OtherActionIncomingDamage},
		SpellSchool: SpellSchoolPhysical,
		ProcMask:    ProcMaskSpellDamage,
		Flags:       SpellFlagNoMetrics | SpellFlagNoLogs | SpellFlagNoOnCastComplete | SpellFlagIgnoreModifiers | SpellFlagIgnoreResists | SpellFlagNoServerData,

		DamageMultiplier: 1,
		CritMultiplier:   1,
		ThreatMultiplier: 0,
	})
}

func (incoming *incomingDamage) reset(sim *Simulation) {
	if incoming == nil || incoming.spell == nil {
		return
	}
	incoming.dealt = 0

	tankHit := math.Round(incoming.perSecond / 2 * incomingTankPeriod.Seconds())
	raidHit := math.Round(incoming.perSecond / 2 * incomingRaidPeriod.Seconds() / incomingRaidTargets)

	if incoming.tank != nil {
		pa := &PendingAction{NextActionAt: incomingTankPeriod}
		pa.OnAction = func(sim *Simulation) {
			incoming.hit(sim, incoming.tank, tankHit)
			pa.NextActionAt = sim.CurrentTime + incomingTankPeriod
			sim.AddPendingAction(pa)
		}
		sim.AddPendingAction(pa)
	}

	if len(incoming.raid) > 0 {
		pa := &PendingAction{NextActionAt: incomingRaidPeriod}
		pa.OnAction = func(sim *Simulation) {
			incoming.raidHits(sim, raidHit)
			pa.NextActionAt = sim.CurrentTime + incomingRaidPeriod
			sim.AddPendingAction(pa)
		}
		sim.AddPendingAction(pa)
	}
}

// raidHits hits incomingRaidTargets of the raid dummies, drawn without replacement.
func (incoming *incomingDamage) raidHits(sim *Simulation, amount float64) {
	incoming.picks = append(incoming.picks[:0], incoming.raid...)
	for i := 0; i < incomingRaidTargets && i < len(incoming.picks); i++ {
		j := i + int(sim.RandomFloat("Incoming Damage Targets")*float64(len(incoming.picks)-i))
		incoming.picks[i], incoming.picks[j] = incoming.picks[j], incoming.picks[i]
		incoming.hit(sim, incoming.picks[i], amount)
	}
}

func (incoming *incomingDamage) hit(sim *Simulation, dummy *TargetDummy, amount float64) {
	unit := &dummy.Unit
	incoming.dealt += amount
	if lost := min(amount, unit.CurrentHealth()-1); lost > 0 {
		unit.RemoveHealth(sim, lost)
	}
	result := incoming.spell.CalcDamageDone(sim, unit, amount, incoming.spell.OutcomeAlwaysHit)
	incoming.spell.DealDamage(sim, result)
}

// IncomingDamageDealt is the incoming damage the raid's dummies were dealt so far this iteration, 0 without any.
func (raid *Raid) IncomingDamageDealt() float64 {
	if raid.incoming == nil {
		return 0
	}
	return raid.incoming.dealt
}

// HasIncomingDamage is whether the raid's dummies take damage. Effects that wait on a hit taken see those hits
// and use a stand-in only when this is false.
func (raid *Raid) HasIncomingDamage() bool {
	return raid.incoming != nil
}

// CountsOverheal is whether a heal on the unit counts only up to its missing health.
func (unit *Unit) CountsOverheal() bool {
	return unit.countsOverheal
}
