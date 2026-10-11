package core

import (
	"cmp"
	"math"
	"slices"
	"time"
)

const ReplenishmentAuraDuration = time.Second * 15

// 57669 ticks once a second
const (
	replenishmentTickPeriod = time.Second
	replenishmentTicks      = int(ReplenishmentAuraDuration / replenishmentTickPeriod)
)

var replenishmentActionID = ActionID{SpellID: 57669}

// replenishment is a unit's Replenishment (57669): a periodic energize of 0.2% of its max mana a second,
// each tick making threat for whoever cast it.
type replenishment struct {
	unit *Unit
	aura *Aura
	// nil without a mana bar, and then nothing ticks
	metrics *ResourceMetrics

	// whose proc the running aura is from, nil for the raid's buff, which nobody in the sim casts
	caster *Unit
	// the proc's aura went on at no mana, so spell_gen_replenishment_aura never loaded: a tick is the DBC's 1
	unscripted bool
	// mana a tick of the proc's aura, set as it goes on or is refreshed
	amount float64
	// of the proc's aura, when it runs over the raid's buff, which never expires
	ticksLeft int

	tickAction *PendingAction
	// the timer's next tick, before it's put on one of the unit's updates
	tickDue time.Duration
	// a pet that isn't out: the timer keeps its place but queues nothing
	paused bool
	// when the running aura runs out: the sim only gets to an expiry with whatever happens next
	endsAt time.Duration
}

// Creates the actual replenishment aura for a unit.
func replenishmentAura(unit *Unit) *Aura {
	if unit.ReplenishmentAura != nil {
		return unit.ReplenishmentAura
	}

	r := &replenishment{unit: unit}
	if unit.HasManaBar() {
		r.metrics = unit.NewManaMetrics(replenishmentActionID)
	}
	r.aura = unit.RegisterAura(Aura{
		Label:    "Replenishment",
		ActionID: replenishmentActionID,
		Duration: ReplenishmentAuraDuration,
		OnGain: func(_ *Aura, sim *Simulation) {
			r.startTicks(sim)
		},
		OnExpire: func(_ *Aura, sim *Simulation) {
			// the update that ends the aura runs its last tick too
			r.stopTicks(sim, true)
			r.caster = nil
		},
	})

	unit.replenishment = r
	unit.ReplenishmentAura = r.aura
	return r.aura
}

func (r *replenishment) manaIsCurrent() bool {
	return r.unit.currentPowerBar == ManaBar
}

// onUpdateGrid is whether the unit's updates come on a grid, world ticks or map updates, not at exact times.
func (r *replenishment) onUpdateGrid(sim *Simulation) bool {
	if r.aura.onWorldTicks() {
		return sim.worldTickInterval > 0
	}
	return sim.serverTickInterval > 0
}

// apply is caster's 57669 landing on the unit. It's false when the cast doesn't take the unit:
// spell_gen_replenishment drops a target whose power isn't mana just then.
func (r *replenishment) apply(sim *Simulation, caster *Unit) bool {
	if !r.manaIsCurrent() {
		return false
	}
	// On a grid the cast lands ahead of the unit's update, so a tick due on that update is lost to the restart.
	// With exact timing that tick's update was the one before, and it goes first.
	r.stopTicks(sim, !r.onUpdateGrid(sim))

	// the same caster's refreshes the aura, anyone else's replaces it
	if !r.aura.IsActive() || r.caster != caster {
		r.unscripted = r.unit.CurrentMana() < 1
	}
	r.caster = caster
	r.amount = 1
	if !r.unscripted {
		r.amount = r.scriptedAmount()
	}
	r.ticksLeft = replenishmentTicks

	r.aura.Activate(sim)
	if r.tickAction == nil {
		// a refresh: a new aura's ticks start as it goes on
		r.startTicks(sim)
	}
	return true
}

// scriptedAmount is spell_gen_replenishment_aura's CalculateAmount: a float multiply cut to int32.
func (r *replenishment) scriptedAmount() float64 {
	return float64(int32(float32(math.Floor(r.unit.MaxMana())) * 0.002))
}

// startTicks restarts the tick timer, as a refresh of a periodic energize does (AuraEffect::CalculatePeriodic).
// Like a dot's, each tick lands on the unit's first update at or after its time on the timer's own lattice.
func (r *replenishment) startTicks(sim *Simulation) {
	if r.metrics == nil {
		return
	}
	r.tickDue = r.aura.countdownStart(sim) + replenishmentTickPeriod
	r.endsAt = r.aura.ExpiresAt()
	r.queueTick(sim)
}

// queueTick puts the timer's next tick on the unit's first update at or after its time.
func (r *replenishment) queueTick(sim *Simulation) {
	if r.paused {
		return
	}
	pa := &PendingAction{NextActionAt: r.aura.nextUpdate(sim, r.tickDue)}
	if r.onUpdateGrid(sim) {
		// the unit's update comes after the casts of its world tick
		pa.Priority = ActionPriorityLow
	}
	pa.OnAction = func(sim *Simulation) {
		r.tick(sim)
		r.tickDue += replenishmentTickPeriod
		pa.NextActionAt = r.aura.nextUpdate(sim, r.tickDue)
		sim.AddPendingAction(pa)
	}
	r.tickAction = pa
	sim.AddPendingAction(pa)
}

// pauseTicks is the pet going away: nothing is queued for it until resumeTicks. The raid's buff never ends,
// and a tick a second for every mana pet that isn't out adds up. r is nil for a pet the buff never reached.
func (r *replenishment) pauseTicks(sim *Simulation) {
	if r == nil {
		return
	}
	r.paused = true
	if pa := r.tickAction; pa != nil {
		r.tickAction = nil
		pa.Cancel(sim)
	}
}

// resumeTicks is the pet coming out. The timer kept its place while the pet was away: the ticks it skipped
// had no pet to fill, and the next one lands on the update it always would have.
func (r *replenishment) resumeTicks(sim *Simulation) {
	if r == nil || !r.paused {
		return
	}
	r.paused = false
	if r.metrics == nil || !r.aura.IsActive() {
		return
	}
	for r.aura.nextUpdate(sim, r.tickDue) < sim.CurrentTime {
		r.countTick()
		r.tickDue += replenishmentTickPeriod
	}
	r.queueTick(sim)
}

// stopTicks ends the tick timer as the aura runs out, is taken off or is refreshed. The sim can get there
// ahead of a tick due at that same time: dueTickLands says whether the server runs that tick first.
func (r *replenishment) stopTicks(sim *Simulation, dueTickLands bool) {
	pa := r.tickAction
	if pa == nil {
		return
	}
	r.tickAction = nil
	pa.Cancel(sim)
	if dueTickLands && pa.NextActionAt == sim.CurrentTime && pa.NextActionAt <= r.endsAt {
		r.tick(sim)
	}
}

func (r *replenishment) tick(sim *Simulation) {
	amount := r.amount
	if r.caster == nil {
		// the raid's buff comes from casts the sim doesn't have, so a tick reads the max as if just put on
		amount = r.scriptedAmount()
	}
	// a player's tick only fills the power it's on (AuraEffect::HandlePeriodicEnergizeAuraTick)
	if amount > 0 && r.manaIsCurrent() {
		before := r.unit.CurrentMana()
		r.unit.AddMana(sim, amount, r.metrics)
		if r.caster != nil {
			r.caster.AddEnergizeTickThreat(sim, r.caster.replenishment.metrics, r.unit.CurrentMana()-before)
		}
	}
	r.countTick()
}

// countTick takes a tick off a proc's aura that runs over the raid's buff, whether the tick gave mana or not.
func (r *replenishment) countTick() {
	if r.caster != nil && r.aura.Duration == NeverExpires {
		// the raid's buff is back as the proc's 15 s run out
		if r.ticksLeft--; r.ticksLeft == 0 {
			r.caster = nil
		}
	}
}

type ReplenishmentSource int

// Returns a new source whose procs give the Replenishment buff to 10 party/raid members, cast by caster.
func (raid *Raid) NewReplenishmentSource(caster *Unit, _ ActionID) ReplenishmentSource {
	newReplSource := ReplenishmentSource(len(raid.curReplenishmentUnits))
	raid.curReplenishmentUnits = append(raid.curReplenishmentUnits, []*Unit{})
	raid.replenishmentCasts = append(raid.replenishmentCasts, caster.NewCastThreat(replenishmentActionID.SpellID))

	if raid.replenishmentUnits == nil {
		// Get the list of all eligible units (party/raid members + their pets, but no guardians).
		var manaUsers []*Unit
		for _, party := range raid.Parties {
			for _, player := range party.Players {
				character := player.GetCharacter()
				if character.HasManaBar() {
					manaUsers = append(manaUsers, &character.Unit)
				}
			}
			for _, petAgent := range party.Pets {
				pet := petAgent.GetPet()
				if pet.HasManaBar() && !pet.IsGuardian() {
					manaUsers = append(manaUsers, &pet.Unit)
				}
			}
		}
		raid.replenishmentUnits = manaUsers

		// Initialize replenishment aura for all applicable units.
		for _, unit := range raid.replenishmentUnits {
			replenishmentAura(unit)
		}
	}

	if caster.replenishment == nil || caster.replenishment.metrics == nil {
		panic("Replenishment source " + caster.Label + " has no mana bar to book its threat under")
	}
	return newReplSource
}

func (raid *Raid) resetReplenishment(_ *Simulation) {
	raid.leftoverReplenishmentUnits = raid.replenishmentUnits
	for i := 0; i < len(raid.curReplenishmentUnits); i++ {
		raid.curReplenishmentUnits[i] = nil
	}
}

func (raid *Raid) ProcReplenishment(sim *Simulation, src ReplenishmentSource) {
	cast := raid.replenishmentCasts[src]
	landed := false

	// If the raid is fully covered by one or more replenishment sources, we can
	// skip the mana sorting.
	if len(raid.curReplenishmentUnits)*10 >= len(raid.replenishmentUnits) {
		if len(raid.curReplenishmentUnits[src]) == 0 {
			if len(raid.leftoverReplenishmentUnits) > 10 {
				raid.curReplenishmentUnits[src] = raid.leftoverReplenishmentUnits[:10]
				raid.leftoverReplenishmentUnits = raid.leftoverReplenishmentUnits[10:]
			} else {
				raid.curReplenishmentUnits[src] = raid.leftoverReplenishmentUnits
				raid.leftoverReplenishmentUnits = nil
			}
		}
		for _, unit := range raid.curReplenishmentUnits[src] {
			landed = unit.replenishment.apply(sim, cast.unit) || landed
		}
	} else {
		eligible := append(raid.curReplenishmentUnits[src], raid.leftoverReplenishmentUnits...)
		slices.SortFunc(eligible, func(v1, v2 *Unit) int {
			return cmp.Compare(v1.CurrentManaPercent(), v2.CurrentManaPercent())
		})
		raid.curReplenishmentUnits[src] = eligible[:10]
		raid.leftoverReplenishmentUnits = eligible[10:]
		for _, unit := range raid.curReplenishmentUnits[src] {
			landed = unit.replenishment.apply(sim, cast.unit) || landed
		}
		for _, unit := range raid.leftoverReplenishmentUnits {
			unit.ReplenishmentAura.Deactivate(sim)
		}
	}

	// a cast that takes nobody has no targets to split its bonus over (Spell::HandleThreatSpells)
	if landed {
		cast.Add(sim, cast.unit.replenishment.metrics)
	}
}
