package core

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

type Task interface {
	RunTask(sim *Simulation) time.Duration
}

type Simulation struct {
	*Environment

	Options *proto.SimOptions

	rand  Rand
	rseed int64

	// Used for testing only, see RandomFloat().
	isTest    bool
	testRands map[string]Rand

	// Current Simulation State
	pendingActions []pendingEntry   // sorted, see AddPendingAction
	pendingSlots   []*PendingAction // the actions queued this iteration, indexed by pendingEntry.slot
	advancingSlot  int32            // the popped action's slot while Step advances time to it, else the sentinel's (0)
	CurrentTime    time.Duration    // duration that has elapsed in the sim since starting
	Duration       time.Duration    // Duration of current iteration
	NeedsInput     bool             // Sim is in interactive mode and needs input

	ProgressReport func(*proto.ProgressMetrics)

	Log func(string, ...interface{})

	executePhase int32 // 20, 25, or 35 for the respective execute range, 100 otherwise

	executePhaseCallbacks []func(*Simulation, int32) // 2nd parameter is 35 for 35%, 25 for 25% and 20 for 20%

	nextExecuteDuration time.Duration
	nextExecuteDamage   float64

	endOfCombatDuration time.Duration
	endOfCombatDamage   float64

	minTrackerTime time.Duration
	trackers       []*auraTracker

	minWeaponAttackTime time.Duration
	weaponAttacks       []*WeaponAttack

	minTaskTime time.Duration
	tasks       []Task

	// Map::Update ticks at phase + k*interval. Swings, cast completion and aura expiry wait for
	// the first tick at or after their timer runs out. Interval 0 means exact timing.
	serverTickInterval time.Duration
	serverTickPhase    time.Duration

	// Spell.ResistanceMultiplier's last thresholds. A resist that got here is above 0, so the zero
	// value never matches.
	lastResist struct {
		averageResist float64
		thresholds    Thresholds
	}
}

func (sim *Simulation) rescheduleTracker(trackerTime time.Duration) {
	sim.minTrackerTime = min(sim.minTrackerTime, trackerTime)
}

func (sim *Simulation) addTracker(tracker *auraTracker) {
	sim.trackers = append(sim.trackers, tracker)
	sim.rescheduleTracker(tracker.minExpires)
}

func (sim *Simulation) removeTracker(tracker *auraTracker) {
	if idx := slices.Index(sim.trackers, tracker); idx != -1 {
		sim.trackers = removeBySwappingToBack(sim.trackers, idx)
	}
}

func (sim *Simulation) rescheduleWeaponAttack(weaponAttackTime time.Duration) {
	sim.minWeaponAttackTime = min(sim.minWeaponAttackTime, weaponAttackTime)
}

func (sim *Simulation) addWeaponAttack(weaponAttack *WeaponAttack) {
	sim.weaponAttacks = append(sim.weaponAttacks, weaponAttack)
}

func (sim *Simulation) removeWeaponAttack(weaponAttack *WeaponAttack) {
	if idx := slices.Index(sim.weaponAttacks, weaponAttack); idx != -1 {
		sim.weaponAttacks = removeBySwappingToBack(sim.weaponAttacks, idx)
	}
}

func (sim *Simulation) RescheduleTask(taskTime time.Duration) {
	sim.minTaskTime = min(sim.minTaskTime, taskTime)
}

func (sim *Simulation) AddTask(task Task) {
	sim.tasks = append(sim.tasks, task)
}

func (sim *Simulation) RemoveTask(task Task) {
	if idx := slices.Index(sim.tasks, task); idx != -1 {
		sim.tasks = removeBySwappingToBack(sim.tasks, idx)
	}
}

func RunSim(rsr *proto.RaidSimRequest, progress chan *proto.ProgressMetrics) *proto.RaidSimResult {
	return runSim(rsr, progress, false)
}

func runSim(rsr *proto.RaidSimRequest, progress chan *proto.ProgressMetrics, skipPresim bool) (result *proto.RaidSimResult) {
	if !rsr.SimOptions.IsTest {
		defer func() {
			if err := recover(); err != nil {
				errStr := ""
				switch errt := err.(type) {
				case string:
					errStr = errt
				case error:
					errStr = errt.Error()
				}

				errStr += "\nStack Trace:\n" + string(debug.Stack())
				result = &proto.RaidSimResult{
					ErrorResult: errStr,
				}
				if progress != nil {
					progress <- &proto.ProgressMetrics{
						FinalRaidResult: result,
					}
				}
			}
			if progress != nil {
				close(progress)
			}
		}()
	}

	sim := NewSim(rsr)

	if !skipPresim {
		if progress != nil {
			progress <- &proto.ProgressMetrics{
				TotalIterations: sim.Options.Iterations,
				PresimRunning:   true,
			}
			runtime.Gosched() // allow time for message to make it back out.
		}
		presimResult := sim.runPresims(rsr)
		if presimResult != nil && presimResult.ErrorResult != "" {
			if progress != nil {
				progress <- &proto.ProgressMetrics{
					TotalIterations: sim.Options.Iterations,
					FinalRaidResult: presimResult,
				}
			}
			return presimResult
		}
		if progress != nil {
			progress <- &proto.ProgressMetrics{
				TotalIterations: sim.Options.Iterations,
				PresimRunning:   false,
			}
			sim.ProgressReport = func(progMetric *proto.ProgressMetrics) {
				progress <- progMetric
			}
			runtime.Gosched() // allow time for message to make it back out.
		}
		// Use pre-sim as estimate for length of fight (when using health fight)
		if sim.Encounter.EndFightAtHealth > 0 && presimResult != nil {
			sim.BaseDuration = time.Duration(presimResult.AvgIterationDuration) * time.Second
			sim.Duration = time.Duration(presimResult.AvgIterationDuration) * time.Second
			sim.Encounter.DurationIsEstimate = false // we now have a pretty good value for duration
		}
	}

	// using a variable here allows us to mutate it in the deferred recover, sending out error info
	t0 := time.Now()
	result = sim.run()
	// here rather than in run, which the sharded sims call once per shard
	if d := sim.Options.Iterations; d > 3000 {
		log.Printf("running %d iterations took %s", d, time.Since(t0))
	}

	return result
}

func NewSim(rsr *proto.RaidSimRequest) *Simulation {
	env, _, _ := NewEnvironment(rsr.Raid, rsr.Encounter, false)
	return newSimWithEnv(env, rsr.SimOptions)
}

func newSimWithEnv(env *Environment, simOptions *proto.SimOptions) *Simulation {
	rseed := simOptions.RandomSeed
	if rseed == 0 {
		rseed = time.Now().UnixNano()
	}

	return &Simulation{
		Environment: env,
		Options:     simOptions,

		rand:  NewSplitMix(uint64(rseed)),
		rseed: rseed,

		isTest:    simOptions.IsTest,
		testRands: make(map[string]Rand),

		serverTickInterval: env.serverSettings().MapUpdateInterval,
	}
}

// The raid's server settings, or the live ones without a raid, same as Character.Server().
func (env *Environment) serverSettings() *ServerSettings {
	if env.Raid == nil || env.Raid.Server == nil {
		return NewServerSettings(nil)
	}
	return env.Raid.Server
}

// NextServerTick is the first server tick at or after t: when a timer that runs out at t fires on
// the server. Returns t unchanged with exact timing.
func (sim *Simulation) NextServerTick(t time.Duration) time.Duration {
	interval := sim.serverTickInterval
	if interval <= 0 || t > NeverExpires-interval {
		return t
	}
	// Go's % keeps the dividend's sign, so a time before the phase needs its own branch
	offset := (t - sim.serverTickPhase) % interval
	switch {
	case offset > 0:
		return t + interval - offset
	case offset < 0:
		return t - offset
	}
	return t
}

// Returns a random float64 between 0.0 (inclusive) and 1.0 (exclusive).
//
// In tests, although we can set the initial seed, test results are still very
// sensitive to the exact order of RandomFloat() calls. To mitigate this, when
// testing we use a separate rand object for each RandomFloat callsite,
// distinguished by the label string.
func (sim *Simulation) RandomFloat(label string) float64 {
	return sim.labelRand(label).NextFloat64()
}

func (sim *Simulation) labelRand(label string) Rand {
	if !sim.isTest {
		return sim.rand
	}

	labelRng, ok := sim.testRands[label]
	if !ok {
		// Add rseed to the label, so we still have run-run variance for stat weights.
		labelRng = NewSplitMix(uint64(makeTestRandSeed(sim.rseed, label)))
		sim.testRands[label] = labelRng
	}
	return labelRng
}

func (sim *Simulation) reseedRands(i int64) {
	rseed := sim.Options.RandomSeed + i
	sim.rand.Seed(rseed)

	if sim.isTest {
		for label, rng := range sim.testRands {
			rng.Seed(makeTestRandSeed(rseed, label))
		}
	}
}

func makeTestRandSeed(rseed int64, label string) int64 {
	return int64(hash(label + strconv.FormatInt(rseed, 16)))
}

func (sim *Simulation) RandomExpFloat(label string) float64 {
	return rand.New(sim.labelRand(label)).ExpFloat64()
}

// Shorthand for commonly-used RNG behavior.
// Returns a random number between min and max.
func (sim *Simulation) Roll(min float64, max float64) float64 {
	return sim.RollWithLabel(min, max, "Damage Roll")
}
func (sim *Simulation) RollWithLabel(min float64, max float64, label string) float64 {
	return min + (max-min)*sim.RandomFloat(label)
}

func (sim *Simulation) Proc(p float64, label string) bool {
	switch {
	case p >= 1:
		return true
	case p <= 0:
		return false
	default:
		return sim.RandomFloat(label) < p
	}
}

func (sim *Simulation) Reset() {
	sim.reset()
}

func (sim *Simulation) Reseed(seed int64) {
	sim.reseedRands(seed)
}

// Run runs the simulation for the configured number of iterations, and
// collects all the metrics together.
func (sim *Simulation) run() *proto.RaidSimResult {
	logsBuffer := &strings.Builder{}
	if sim.Options.Debug || sim.Options.DebugFirstIteration {
		sim.Log = func(message string, vals ...interface{}) {
			logsBuffer.WriteString(fmt.Sprintf("[%0.2f] "+message+"\n", append([]interface{}{sim.CurrentTime.Seconds()}, vals...)...))
		}
	}

	// Uncomment this to print logs directly to console.
	// sim.Options.Debug = true
	// sim.Log = func(message string, vals ...interface{}) {
	// 	fmt.Printf(fmt.Sprintf("[%0.1f] "+message+"\n", append([]interface{}{sim.CurrentTime.Seconds()}, vals...)...))
	// }

	sim.runOnce()
	firstIterationDuration := sim.Duration
	if sim.Encounter.EndFightAtHealth != 0 {
		firstIterationDuration = sim.CurrentTime
	}
	totalDuration := firstIterationDuration

	if !sim.Options.Debug {
		sim.Log = nil
	}

	var st time.Time
	for i := int32(1); i < sim.Options.Iterations; i++ {
		// fmt.Printf("Iteration: %d\n", i)
		if sim.ProgressReport != nil && time.Since(st) > time.Millisecond*100 {
			// the raid's averages only: Raid.GetMetrics would build every unit's metrics to get them
			dps, _ := sim.Raid.dpsMetrics.meanAndStdDev()
			hps, _ := sim.Raid.hpsMetrics.meanAndStdDev()
			sim.ProgressReport(&proto.ProgressMetrics{TotalIterations: sim.Options.Iterations, CompletedIterations: i, Dps: dps, Hps: hps})
			runtime.Gosched() // ensure that reporting threads are given time to report, mostly only important in wasm (only 1 thread)
			st = time.Now()
		}

		// Before each iteration, reset state to seed+iterations
		sim.reseedRands(int64(i))

		sim.runOnce()
		iterDuration := sim.Duration
		if sim.Encounter.EndFightAtHealth != 0 {
			iterDuration = sim.CurrentTime
		}
		totalDuration += iterDuration
	}
	result := &proto.RaidSimResult{
		RaidMetrics:      sim.Raid.GetMetrics(),
		EncounterMetrics: sim.Encounter.GetMetricsProto(),

		Logs:                   logsBuffer.String(),
		FirstIterationDuration: firstIterationDuration.Seconds(),
		AvgIterationDuration:   totalDuration.Seconds() / float64(sim.Options.Iterations),
	}

	// Final progress report
	if sim.ProgressReport != nil {
		sim.ProgressReport(&proto.ProgressMetrics{TotalIterations: sim.Options.Iterations, CompletedIterations: sim.Options.Iterations, Dps: result.RaidMetrics.Dps.Avg, FinalRaidResult: result})
	}

	return result
}

// RunOnce is the main event loop. It will run the simulation for number of seconds.
func (sim *Simulation) runOnce() {
	sim.reset()
	sim.PrePull()
	sim.runPendingActions()
	sim.Cleanup()
}

var (
	sentinelPendingAction = &PendingAction{
		NextActionAt: NeverExpires,
		OnAction: func(sim *Simulation) {
			panic("running sentinel pending action")
		},
	}
)

// Reset will set sim back and erase all current state.
// This is automatically called before every 'Run'.
func (sim *Simulation) reset() {
	if sim.Encounter.DurationIsEstimate && sim.CurrentTime != 0 {
		sim.BaseDuration = sim.CurrentTime
		sim.Encounter.DurationIsEstimate = false
	}
	sim.Duration = sim.BaseDuration
	if sim.DurationVariation != 0 {
		variation := sim.DurationVariation * 2
		sim.Duration += time.Duration(sim.RandomFloat("sim duration")*float64(variation)) - sim.DurationVariation
	}

	// the pull lands anywhere between two server ticks
	sim.serverTickPhase = 0
	if sim.serverTickInterval > 0 {
		sim.serverTickPhase = time.Duration(sim.RandomFloat("Server Tick Phase") * float64(sim.serverTickInterval))
	}

	sim.resetPendingActions()

	sim.executePhase = 0
	sim.nextExecutePhase()
	sim.executePhaseCallbacks = nil

	// Use duration as an end check if not using health.
	sim.endOfCombatDuration = sim.Duration
	sim.endOfCombatDamage = math.MaxFloat64
	if sim.Encounter.EndFightAtHealth > 0 {
		sim.endOfCombatDuration = NeverExpires
		sim.endOfCombatDamage = sim.Encounter.EndFightAtHealth
	}

	sim.CurrentTime = 0

	sim.trackers = sim.trackers[:0]
	sim.minTrackerTime = NeverExpires

	sim.weaponAttacks = sim.weaponAttacks[:0]
	sim.minWeaponAttackTime = NeverExpires

	sim.tasks = sim.tasks[:0]
	sim.minTaskTime = NeverExpires

	sim.Environment.reset(sim)

	sim.initManaTickAction()
}

func (sim *Simulation) PrePull() {
	if len(sim.prepullActions) > 0 {
		sim.CurrentTime = sim.prepullActions[0].DoAt

		for i, ppa := range sim.prepullActions {
			sim.AddPendingAction(&PendingAction{
				NextActionAt: ppa.DoAt,
				Priority:     ActionPriorityPrePull + ActionPriority(len(sim.prepullActions)-i),
				OnAction:     ppa.Action,
			})
		}
	}

	sim.AddPendingAction(&PendingAction{
		NextActionAt: 0,
		Priority:     ActionPriorityPrePull,
		OnAction: func(sim *Simulation) {
			for _, unit := range sim.Environment.AllUnits {
				if unit.enabled {
					unit.startPull(sim)
				}
			}
		},
	})
}

func (sim *Simulation) Cleanup() {
	// The last event loop will leave CurrentTime at some value close to but not
	// quite at the Duration. Explicitly set this so that accesses to CurrentTime
	// during the doneIteration phase will return the Duration value, which is
	// intuitive.
	sim.CurrentTime = sim.Duration

	for _, e := range sim.pendingActions {
		if pa := sim.pendingSlots[e.slot]; pa.CleanUp != nil {
			pa.CleanUp(sim)
		}
	}

	sim.Raid.doneIteration(sim)
	sim.Encounter.doneIteration(sim)

	for _, unit := range sim.Raid.AllUnits {
		unit.Metrics.doneIteration(unit, sim)
	}
	for _, target := range sim.Encounter.TargetUnits {
		target.Metrics.doneIteration(target, sim)
	}
}

func (sim *Simulation) runPendingActions() {
	for {
		if finished := sim.Step(); finished {
			return
		}
	}
}

func (sim *Simulation) Step() bool {
	last := len(sim.pendingActions) - 1
	next := sim.pendingActions[last]

	if next.at >= sim.minWeaponAttackTime && sim.minWeaponAttackTime <= sim.minTaskTime {
		if sim.minWeaponAttackTime > sim.endOfCombatDuration || sim.Encounter.DamageTaken > sim.endOfCombatDamage {
			return true
		}
		sim.advanceWeaponAttacks()
		return false
	}

	if next.at >= sim.minTaskTime {
		if sim.minTaskTime > sim.endOfCombatDuration || sim.Encounter.DamageTaken > sim.endOfCombatDamage {
			return true
		}
		sim.advanceTasks()
		return false
	}

	sim.pendingActions = sim.pendingActions[:last]
	pa := sim.pendingSlots[next.slot]
	if pa.cancelled {
		return false
	}

	if next.at > sim.endOfCombatDuration || sim.Encounter.DamageTaken > sim.endOfCombatDamage {
		return true
	}

	if next.at > sim.CurrentTime {
		// pa is out of the queue but hasn't run: a Cancel from here stops it, so detaching mustn't
		sim.advancingSlot = next.slot
		sim.advance(next.at)
		sim.advancingSlot = 0
	}
	pa.consumed = true

	if pa.cancelled {
		return false
	}
	pa.OnAction(sim)
	return false
}

func (sim *Simulation) advanceWeaponAttacks() {
	if sim.minWeaponAttackTime > sim.CurrentTime {
		sim.advance(sim.minWeaponAttackTime)
	}

	sim.minWeaponAttackTime = NeverExpires
	for _, wa := range sim.weaponAttacks {
		sim.minWeaponAttackTime = min(sim.minWeaponAttackTime, wa.trySwing(sim, false))
	}
}

func (sim *Simulation) advanceTasks() {
	if sim.minTaskTime > sim.CurrentTime {
		sim.advance(sim.minTaskTime)
	}

	sim.minTaskTime = NeverExpires
	for _, t := range sim.tasks {
		sim.minTaskTime = min(sim.minTaskTime, t.RunTask(sim)) // RunTask() might alter sim.tasks
	}
}

// Advance moves time forward counting down auras, CDs, mana regen, etc
func (sim *Simulation) advance(nextTime time.Duration) {
	sim.CurrentTime = nextTime

	// this is a loop to handle duplicate ExecuteProportions, e.g. if they're all set to 100%, you reach
	// execute phases 35%, 25%, and 20% in the first advance() call.
	for sim.CurrentTime >= sim.nextExecuteDuration || sim.Encounter.DamageTaken >= sim.nextExecuteDamage {
		sim.nextExecutePhase()
		for _, callback := range sim.executePhaseCallbacks {
			callback(sim, sim.executePhase)
		}
	}

	if sim.CurrentTime >= sim.minTrackerTime {
		sim.minTrackerTime = NeverExpires
		for _, t := range sim.trackers {
			sim.minTrackerTime = min(sim.minTrackerTime, t.tryAdvance(sim))
		}
	}
}

// nextExecutePhase updates nextExecuteDuration and nextExecuteDamage based on executePhase.
func (sim *Simulation) nextExecutePhase() {
	setup := func(phase int32, damage float64, health float64) {
		sim.executePhase = phase
		if sim.Encounter.EndFightAtHealth > 0 {
			sim.nextExecuteDamage = (1 - damage) * sim.Encounter.EndFightAtHealth
		} else {
			sim.nextExecuteDuration = time.Duration((1 - health) * float64(sim.Duration))
		}
	}

	sim.nextExecuteDuration = NeverExpires
	sim.nextExecuteDamage = math.MaxFloat64

	switch sim.executePhase {
	case 0: // reset, waiting for 35%
		setup(100, 0.35, sim.Encounter.ExecuteProportion_35)
	case 100: // at 35%, waiting for 25%
		setup(35, 0.25, sim.Encounter.ExecuteProportion_25)
	case 35: // at 25%, waiting for 20%
		setup(25, 0.20, sim.Encounter.ExecuteProportion_20)
	case 25: // at 20%, done waiting
		sim.executePhase = 20 // could also be used for end of fight handling
	default:
		panic(fmt.Sprintf("executePhase = %d invalid", sim.executePhase))
	}
}

// A queued action's NextActionAt and Priority, copied when it's added, and its index in pendingSlots.
// No pointers in here, so an insert shifts the queue with a plain memmove, no GC write barriers.
type pendingEntry struct {
	at   time.Duration
	prio ActionPriority
	slot int32
}

// Stands in for a queued action detachPendingAction took out. Never written, so shared by every sim.
var detachedPendingAction = &PendingAction{cancelled: true}

// detachPendingAction points pa's slot at a cancelled stand-in, so pa's queued entries stay where
// they are and do nothing when popped: what cancelling pa and dropping it would do, but pa is free
// to be queued again, in a new slot. Reports false, changing nothing, when pa has a CleanUp to run,
// holds no slot this iteration or is the action Step is about to run.
func (sim *Simulation) detachPendingAction(pa *PendingAction) bool {
	if pa.CleanUp != nil || pa.slot == sim.advancingSlot || pa.slot >= int32(len(sim.pendingSlots)) || sim.pendingSlots[pa.slot] != pa {
		return false
	}
	sim.pendingSlots[pa.slot] = detachedPendingAction
	return true
}

func (sim *Simulation) resetPendingActions() {
	clear(sim.pendingSlots)
	sim.pendingSlots = append(sim.pendingSlots[:0], sentinelPendingAction)
	sim.advancingSlot = 0
	sim.pendingActions = append(sim.pendingActions[:0], pendingEntry{at: sentinelPendingAction.NextActionAt, prio: sentinelPendingAction.Priority})
}

// Sorted so the next action is last: NextActionAt descending, then Priority ascending, sentinel at 0.
// A new action goes below every queued one with the same time and priority, so those run in the order
// they were added. The queue keeps the keys the action had when added: set a queued action's
// NextActionAt and Priority only while it's out of the queue.
func (sim *Simulation) AddPendingAction(pa *PendingAction) {
	pa.consumed = false

	// an action keeps its slot for the iteration, unless detached, so requeueing it writes no pointer
	slot := pa.slot
	if slot >= int32(len(sim.pendingSlots)) || sim.pendingSlots[slot] != pa {
		slot = int32(len(sim.pendingSlots))
		sim.pendingSlots = append(sim.pendingSlots, pa)
		pa.slot = slot
	}

	at, prio := pa.NextActionAt, pa.Priority
	lo, hi := 1, len(sim.pendingActions)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if v := &sim.pendingActions[mid]; v.at < at || (v.at == at && v.prio >= prio) {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	sim.pendingActions = append(sim.pendingActions, pendingEntry{})
	copy(sim.pendingActions[lo+1:], sim.pendingActions[lo:])
	sim.pendingActions[lo] = pendingEntry{at: at, prio: prio, slot: slot}
}

func (sim *Simulation) RegisterExecutePhaseCallback(callback func(sim *Simulation, isExecute int32)) {
	sim.executePhaseCallbacks = append(sim.executePhaseCallbacks, callback)
}
func (sim *Simulation) IsExecutePhase20() bool {
	return sim.executePhase <= 20
}
func (sim *Simulation) IsExecutePhase25() bool {
	return sim.executePhase <= 25
}
func (sim *Simulation) IsExecutePhase35() bool {
	return sim.executePhase <= 35
}

func (sim *Simulation) GetRemainingDuration() time.Duration {
	if sim.Encounter.EndFightAtHealth > 0 {
		if !sim.Encounter.DurationIsEstimate || sim.CurrentTime < time.Second*5 {
			return sim.Duration - sim.CurrentTime
		}

		// Estimate time remaining via avg dps
		dps := sim.Encounter.DamageTaken / sim.CurrentTime.Seconds()
		dur := time.Duration((sim.Encounter.EndFightAtHealth-sim.Encounter.DamageTaken)/dps) * time.Second
		return dur
	}
	return sim.Duration - sim.CurrentTime
}

// Returns the percentage of time remaining in the current iteration, as a value from 0-1.
func (sim *Simulation) GetRemainingDurationPercent() float64 {
	if sim.Encounter.EndFightAtHealth > 0 {
		return 1.0 - sim.Encounter.DamageTaken/sim.Encounter.EndFightAtHealth
	}
	return float64(sim.Duration-sim.CurrentTime) / float64(sim.Duration)
}
