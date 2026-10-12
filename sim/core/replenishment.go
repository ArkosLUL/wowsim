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
	// With exact timing that tick's update was the one before, and it goes first. So it does when the cast
	// comes out of an update that runs after the unit's.
	r.stopTicks(sim, !r.onUpdateGrid(sim) || r.updatedAheadOf(sim.updating))

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

// updatedAheadOf is whether the unit's auras have ticked on this world tick by the time caster's update
// casts: a player's have when a creature or pet casts (Hunting Party off Explosive Shot's tick on the
// target), and the unit's own when its swing does.
func (r *replenishment) updatedAheadOf(caster *Unit) bool {
	if caster == nil {
		return false
	}
	return caster == r.unit || (caster.Type != PlayerUnit && r.aura.onWorldTicks())
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
	if dueTickLands && pa.NextActionAt == sim.CurrentTime {
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

// ReplenishmentManaPerSecond is what the unit's Replenishment gives it a second right now.
func (unit *Unit) ReplenishmentManaPerSecond() float64 {
	r := unit.replenishment
	if r == nil || r.metrics == nil || !r.aura.IsActive() || !r.manaIsCurrent() {
		return 0
	}
	amount := r.amount
	if r.caster == nil {
		amount = r.scriptedAmount()
	}
	return amount / replenishmentTickPeriod.Seconds()
}

// replenishmentMaxTargets is spell_gen_replenishment's cap.
const replenishmentMaxTargets = 10

// takesReplenishment is whether a cast counts the unit right now: out, and on mana
// (spell_gen_replenishment drops every other power before it counts).
func takesReplenishment(unit *Unit) bool {
	if !unit.enabled {
		return false
	}
	if r := unit.replenishment; r != nil {
		return r.manaIsCurrent()
	}
	return true
}

// replenishmentOrder is PowerPctOrderPred's key: whole mana over the whole pool in float32, 0 without
// a pool, 1 for a pool that is always full.
func (raid *Raid) replenishmentOrder(unit *Unit) float32 {
	if raid.replenishmentFullPool[unit] {
		return 1
	}
	if !unit.HasManaBar() {
		return 0
	}
	pool := float32(math.Floor(unit.MaxMana()))
	if pool <= 0 {
		return 0
	}
	return float32(math.Floor(unit.CurrentMana())) / pool
}

type ReplenishmentSource int

// Returns a new source whose procs give the Replenishment buff to 10 party/raid members, cast by caster.
func (raid *Raid) NewReplenishmentSource(caster *Unit, _ ActionID) ReplenishmentSource {
	newReplSource := ReplenishmentSource(len(raid.replenishmentCasts))
	raid.replenishmentCasts = append(raid.replenishmentCasts, caster.NewCastThreat(replenishmentActionID.SpellID))

	if raid.replenishmentUnits == nil {
		// TARGET_UNIT_CASTER_AREA_RAID: the raid's players on mana and what they control. Pets and the
		// guardians with a pet bar count, the other guardians don't (TARGET_CHECK_RAID).
		var units []*Unit
		for _, party := range raid.Parties {
			for _, player := range party.Players {
				character := player.GetCharacter()
				if character.HasManaBar() {
					units = append(units, &character.Unit)
				}
			}
			for _, petAgent := range party.Pets {
				pet := petAgent.GetPet()
				if (pet.HasManaBar() || pet.EmptyManaPool || pet.FullManaPool) && !pet.Uncontrollable {
					units = append(units, &pet.Unit)
					if pet.FullManaPool {
						if raid.replenishmentFullPool == nil {
							raid.replenishmentFullPool = map[*Unit]bool{}
						}
						raid.replenishmentFullPool[&pet.Unit] = true
					}
				}
			}
		}
		raid.replenishmentUnits = units

		for _, unit := range units {
			if unit.HasManaBar() {
				replenishmentAura(unit)
			}
		}
	}

	if caster.replenishment == nil || caster.replenishment.metrics == nil {
		panic("Replenishment source " + caster.Label + " has no mana bar to book its threat under")
	}
	return newReplSource
}

// ProcReplenishment is a cast of 57669. It lands on every raid unit on mana, or with more than ten of
// them on the ten with the least mana left for their pool (spell_gen_replenishment). The others keep
// whatever aura they have.
func (raid *Raid) ProcReplenishment(sim *Simulation, src ReplenishmentSource) {
	cast := raid.replenishmentCasts[src]

	picks := raid.replenishmentPicks[:0]
	for _, unit := range raid.replenishmentUnits {
		if takesReplenishment(unit) {
			picks = append(picks, unit)
		}
	}
	if len(picks) > replenishmentMaxTargets {
		// std::list::sort is stable: units level on mana keep the order they were found in, the raid's here
		slices.SortStableFunc(picks, func(a, b *Unit) int {
			return cmp.Compare(raid.replenishmentOrder(a), raid.replenishmentOrder(b))
		})
		picks = picks[:replenishmentMaxTargets]
	}
	raid.replenishmentPicks = picks

	landed := false
	for _, unit := range picks {
		if r := unit.replenishment; r != nil {
			landed = r.apply(sim, cast.unit) || landed
		} else {
			// an empty pool takes the aura and nothing from it
			landed = true
		}
	}

	// a cast that takes nobody has no targets to split its bonus over (Spell::HandleThreatSpells)
	if landed {
		cast.Add(sim, cast.unit.replenishment.metrics)
	}
}
