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
	testRands map[string]*testRandStream

	// Units with their own per-label streams (UnitRandomFloat, SimOptions.PerUnitRandomSeeds), so
	// reseedRands can find them without walking the whole raid and encounter every iteration.
	unitTestRandUnits []*Unit

	// Current Simulation State
	pendingActions []pendingEntry   // sorted, see AddPendingAction
	pendingSlots   []*PendingAction // the actions queued this iteration, indexed by pendingEntry.slot
	advancingSlot  int32            // the popped action's slot while Step advances time to it, else the sentinel's (0)
	landings       []*landingAction // every missile landing this sim made, see startLanding
	freeLandings   []*landingAction // the ones not queued
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
	weaponAttacks       []*WeaponAttack // the players' first, as Map::Update runs players ahead of creatures
	playerAttacks       int             // how many of them are players'
	shotPassAt          time.Duration   // the tick whose Auto Shots already went ahead of its auras
	meleePassAt         time.Duration   // the tick whose player swings already went ahead of the creatures' auras

	// starts the execute phases the damage taken has reached, see executeDamageReached
	executeAction *PendingAction
	executeQueued bool

	minTaskTime time.Duration
	tasks       []Task

	// The map update, at phase + k*interval: creatures, pets and guardians update on it, players on
	// every world tick, and a unit's swings, cast landings and auras wait for its next update.
	// Interval 0 means exact timing. Every map update is a world tick.
	serverTickInterval time.Duration
	serverTickPhase    time.Duration
	worldTickInterval  time.Duration
	worldTickPhase     time.Duration

	// Set when the updates jitter: it places them, the intervals above are the shortest an update and
	// a tick get, and both phases are the first jittered update. Ask ServerTickBefore and
	// WorldTickBefore for the update before a time: it isn't one interval back.
	updates *updateJitter

	// the unit whose own update is running, while its swing, AI instant cast, dot tick or aura expiry
	// does: an aura it puts on itself then counts from this update, not the one before
	updating *Unit

	// the unit whose m_Events pass runs a cast landing, missile hit or delayed event right now: a missile it
	// launches then starts on its next update, all but the landing hardcast's own (Spell.landsAt)
	eventPass   *Unit
	landingCast *Spell
	landingSeq  uint64 // the last startLanding's, so landings on one key go in launch order

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
		// a pass over the trackers that's under way can skip the one swapped in, so look again
		sim.rescheduleTracker(sim.CurrentTime)
	}
}

// earliestExpiry is when the next aura runs out, if that's before the given time. A tracker's cached
// time is too early once its soonest aura was refreshed or taken off, so the ones that look due are
// counted again first.
func (sim *Simulation) earliestExpiry(before time.Duration) time.Duration {
	earliest := NeverExpires
	for _, tracker := range sim.trackers {
		if tracker.minExpires < before {
			tracker.recountMinExpires()
		}
		earliest = min(earliest, tracker.minExpires)
	}
	sim.minTrackerTime = earliest
	return earliest
}

func (sim *Simulation) rescheduleWeaponAttack(weaponAttackTime time.Duration) {
	sim.minWeaponAttackTime = min(sim.minWeaponAttackTime, weaponAttackTime)
}

func (sim *Simulation) addWeaponAttack(weaponAttack *WeaponAttack) {
	sim.weaponAttacks = append(sim.weaponAttacks, weaponAttack)
	if weaponAttack.unit.Type == PlayerUnit {
		// behind the last player's, and the creature's that was there goes to the back
		last := len(sim.weaponAttacks) - 1
		sim.weaponAttacks[last], sim.weaponAttacks[sim.playerAttacks] = sim.weaponAttacks[sim.playerAttacks], weaponAttack
		sim.playerAttacks++
	}
}

func (sim *Simulation) removeWeaponAttack(weaponAttack *WeaponAttack) {
	idx := slices.Index(sim.weaponAttacks, weaponAttack)
	if idx == -1 {
		return
	}
	if idx < sim.playerAttacks {
		// the last player's takes its place, so the players stay in front
		sim.playerAttacks--
		sim.weaponAttacks[idx] = sim.weaponAttacks[sim.playerAttacks]
		idx = sim.playerAttacks
	}
	sim.weaponAttacks = removeBySwappingToBack(sim.weaponAttacks, idx)
	// a pass over the attacks that's under way can skip the ones moved up, so look again
	sim.rescheduleWeaponAttack(sim.CurrentTime)
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

	world, mapUpdate := env.serverSettings().updateGrids()
	sim := &Simulation{
		Environment: env,
		Options:     simOptions,

		rand:  NewSplitMix(uint64(rseed)),
		rseed: rseed,

		isTest:    simOptions.IsTest,
		testRands: make(map[string]*testRandStream),

		serverTickInterval: mapUpdate,
		worldTickInterval:  world,
	}
	if lengths := env.serverSettings().updateLengths(); lengths != nil {
		sim.updates = newUpdateJitter(lengths)
		sim.serverTickInterval = lengths.shortest
		sim.worldTickInterval = lengths.world
	}
	return sim
}

// The raid's server settings, or the live ones without a raid, same as Character.Server().
func (env *Environment) serverSettings() *ServerSettings {
	if env.Raid == nil || env.Raid.Server == nil {
		return NewServerSettings(nil)
	}
	return env.Raid.Server
}

// NextServerTick is the first map update at or after t: when a creature's, pet's or guardian's timer
// that runs out at t fires on the server. Returns t unchanged with exact timing.
func (sim *Simulation) NextServerTick(t time.Duration) time.Duration {
	if sim.updates != nil {
		return sim.updates.nextUpdate(t)
	}
	return nextTick(t, sim.serverTickInterval, sim.serverTickPhase)
}

// NextWorldTick is NextServerTick for a player: the first world tick at or after t.
func (sim *Simulation) NextWorldTick(t time.Duration) time.Duration {
	if sim.updates != nil {
		return sim.updates.nextWorldTick(t)
	}
	return nextTick(t, sim.worldTickInterval, sim.worldTickPhase)
}

// ServerTickBefore is the last map update before t: the one whose diff a creature's next update takes off
// a timer started at t. Returns t unchanged with exact timing.
func (sim *Simulation) ServerTickBefore(t time.Duration) time.Duration {
	if sim.updates != nil {
		return sim.updates.updateBefore(t)
	}
	if sim.serverTickInterval <= 0 {
		return t
	}
	return nextTick(t, sim.serverTickInterval, sim.serverTickPhase) - sim.serverTickInterval
}

// WorldTickBefore is ServerTickBefore for a player: the last world tick before t.
func (sim *Simulation) WorldTickBefore(t time.Duration) time.Duration {
	if sim.updates != nil {
		return sim.updates.worldTickBefore(t)
	}
	if sim.worldTickInterval <= 0 {
		return t
	}
	return nextTick(t, sim.worldTickInterval, sim.worldTickPhase) - sim.worldTickInterval
}

// nextUpdate is the unit's first update at or after t.
func (sim *Simulation) nextUpdate(unit *Unit, t time.Duration) time.Duration {
	if unit.Type == PlayerUnit {
		return sim.NextWorldTick(t)
	}
	return sim.NextServerTick(t)
}

// updateBefore is the unit's last update before t.
func (sim *Simulation) updateBefore(unit *Unit, t time.Duration) time.Duration {
	if unit.Type == PlayerUnit {
		return sim.WorldTickBefore(t)
	}
	return sim.ServerTickBefore(t)
}

func nextTick(t, interval, phase time.Duration) time.Duration {
	if interval <= 0 || t > NeverExpires-interval {
		return t
	}
	// Go's % keeps the dividend's sign, so a time before the phase needs its own branch
	offset := (t - phase) % interval
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

// UnitRandomFloat is RandomFloat, additionally split per unit (pets and guardians included) once
// SimOptions.PerUnitRandomSeeds is on: only the optimizer's evaluator sets it. Off, or with unit
// nil, every caller shares one stream per label, same as RandomFloat.
func (sim *Simulation) UnitRandomFloat(unit *Unit, label string) float64 {
	return sim.unitLabelRand(unit, label).NextFloat64()
}

func (sim *Simulation) labelRand(label string) Rand {
	if !sim.isTest {
		return sim.rand
	}

	stream, ok := sim.testRands[label]
	if !ok {
		stream = newTestRandStream(label, sim.rseed)
		sim.testRands[label] = stream
	}
	return stream.rand
}

func (sim *Simulation) unitLabelRand(unit *Unit, label string) Rand {
	if !sim.isTest {
		return sim.rand
	}
	if unit == nil || !sim.Options.PerUnitRandomSeeds {
		return sim.labelRand(label)
	}

	if unit.testRands == nil {
		unit.testRands = make(map[string]*testRandStream)
		sim.unitTestRandUnits = append(sim.unitTestRandUnits, unit)
	}
	stream, ok := unit.testRands[label]
	if !ok {
		stream = newUnitTestRandStream(unit, label, sim.rseed)
		unit.testRands[label] = stream
	}
	return stream.rand
}

func (sim *Simulation) reseedRands(i int64) {
	rseed := sim.Options.RandomSeed + i
	sim.rand.Seed(rseed)

	if sim.isTest {
		for _, stream := range sim.testRands {
			stream.reseed(rseed)
		}
		for _, unit := range sim.unitTestRandUnits {
			for _, stream := range unit.testRands {
				stream.reseed(rseed)
			}
		}
	}
}

// testRandStream is an IsTest-only random stream. labelHash folds the label (and, per unit, the
// unit's index) in once at creation, so reseeding every iteration only has to mix in the new seed
// instead of re-hashing the label and building a new string for it.
type testRandStream struct {
	labelHash uint32
	rand      Rand
}

func newTestRandStream(label string, rseed int64) *testRandStream {
	labelHash := fnvHashString(fnvOffset32, label)
	return &testRandStream{labelHash: labelHash, rand: NewSplitMix(uint64(seedFromLabelHash(labelHash, rseed)))}
}

func newUnitTestRandStream(unit *Unit, label string, rseed int64) *testRandStream {
	labelHash := fnvHashUint32(fnvHashString(fnvOffset32, label), uint32(unit.UnitIndex))
	return &testRandStream{labelHash: labelHash, rand: NewSplitMix(uint64(seedFromLabelHash(labelHash, rseed)))}
}

func (s *testRandStream) reseed(rseed int64) {
	s.rand.Seed(seedFromLabelHash(s.labelHash, rseed))
}

// seedFromLabelHash matches hash(label + strconv.FormatInt(rseed, 16)): FNV-1a folds bytes in
// sequence, so hashing the label's bytes and then the seed's hex digits gives the same result as
// hashing them pre-concatenated, without allocating that string on every reseed.
func seedFromLabelHash(labelHash uint32, rseed int64) int64 {
	var buf [20]byte
	return int64(fnvHashBytes(labelHash, strconv.AppendInt(buf[:0], rseed, 16)))
}

const (
	fnvOffset32 = 2166136261
	fnvPrime32  = 16777619
)

func fnvHashByte(h uint32, b byte) uint32 {
	return (h ^ uint32(b)) * fnvPrime32
}

func fnvHashBytes(h uint32, data []byte) uint32 {
	for _, b := range data {
		h = fnvHashByte(h, b)
	}
	return h
}

func fnvHashString(h uint32, s string) uint32 {
	for i := 0; i < len(s); i++ {
		h = fnvHashByte(h, s[i])
	}
	return h
}

func fnvHashUint32(h uint32, v uint32) uint32 {
	h = fnvHashByte(h, byte(v>>24))
	h = fnvHashByte(h, byte(v>>16))
	h = fnvHashByte(h, byte(v>>8))
	return fnvHashByte(h, byte(v))
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

// RollWhole is the server's irand: a whole number from min to max, both ends included.
func (sim *Simulation) RollWhole(min float64, max float64, label string) float64 {
	return min + math.Trunc((max-min+1)*sim.RandomFloat(label))
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

	// the pull lands anywhere between two map updates, and the world ticks line up with them
	sim.serverTickPhase, sim.worldTickPhase = 0, 0
	if sim.updates != nil {
		// the jitter starts on the last update before the sim does, and stops where the fight has to be over
		start, horizon := time.Duration(0), min(sim.Duration, time.Hour)
		if len(sim.prepullActions) > 0 {
			start = min(start, sim.prepullActions[0].DoAt)
		}
		if sim.Encounter.EndFightAtHealth > 0 {
			horizon = 2*horizon + time.Minute
		}
		phase := time.Duration(sim.RandomFloat("Server Tick Phase") * float64(sim.updates.lengths.mean))
		sim.serverTickPhase, sim.worldTickPhase = start-phase, start-phase
		// its own stream, so placing an update never moves another roll
		sim.updates.reset(start-phase, horizon+time.Second, sim.rand.GetSeed()^updateJitterSeed)
	} else if period := max(sim.serverTickInterval, sim.worldTickInterval); period > 0 {
		phase := time.Duration(sim.RandomFloat("Server Tick Phase") * float64(period))
		if sim.serverTickInterval > 0 {
			sim.serverTickPhase = phase
		}
		if sim.worldTickInterval > 0 {
			sim.worldTickPhase = phase % sim.worldTickInterval
		}
	}
	sim.updating = nil
	sim.eventPass, sim.landingCast = nil, nil

	sim.resetPendingActions()

	sim.executePhase = 0
	sim.nextExecutePhase()
	sim.executePhaseCallbacks = nil
	sim.executeQueued = false

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
	sim.playerAttacks = 0
	sim.minWeaponAttackTime = NeverExpires
	sim.shotPassAt = NeverExpires
	sim.meleePassAt = NeverExpires

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

	// an aura runs out at its own time, not with whatever happens to come next
	stops := min(next.at, sim.minWeaponAttackTime, sim.minTaskTime)
	before := min(stops, sim.nextExecuteDuration)
	if sim.minTrackerTime <= before {
		limit := before
		if limit < NeverExpires {
			limit++
		}
		if at := sim.earliestExpiry(limit); at < NeverExpires && at <= before {
			if at > sim.endOfCombatDuration || sim.Encounter.DamageTaken > sim.endOfCombatDamage {
				return true
			}
			swings := at == sim.minWeaponAttackTime
			switch {
			case at < before:
				// an aura that ran out while nothing updated its unit (a pet that was away) is overdue
				sim.advance(max(at, sim.CurrentTime))
			// a tie goes to what a player's update does ahead of its aura pass: landings, then Auto Shot
			case sim.playerEventAt(next, at):
				sim.advanceClock(at)
				return sim.popPendingAction(last, next)
			case swings && sim.playerShotDue(at):
				sim.advanceClock(at)
				sim.advanceWeaponAttacks(playerShots)
			case sim.playerAuraDue(at):
				sim.advancePlayerAuras(at)
			// and the rest of the player's update, its melee, comes ahead of the creatures' auras
			case swings && sim.playerSwingDue(at):
				sim.advanceClock(at)
				sim.advanceWeaponAttacks(playerSwings)
			default:
				sim.advance(max(at, sim.CurrentTime))
			}
			return false
		}
	}

	// an execute phase starts at its own time too
	if x := sim.nextExecuteDuration; x < stops && x > sim.CurrentTime && x <= sim.endOfCombatDuration {
		sim.advance(x)
		return false
	}

	if next.at >= sim.minWeaponAttackTime && sim.minWeaponAttackTime <= sim.minTaskTime && !sim.landingAheadOfSwings(next) {
		if sim.minWeaponAttackTime > sim.endOfCombatDuration || sim.Encounter.DamageTaken > sim.endOfCombatDamage {
			return true
		}
		sim.advanceWeaponAttacks(allAttacks)
		return false
	}

	if next.at >= sim.minTaskTime {
		if sim.minTaskTime > sim.endOfCombatDuration || sim.Encounter.DamageTaken > sim.endOfCombatDamage {
			return true
		}
		sim.advanceTasks()
		return false
	}

	return sim.popPendingAction(last, next)
}

// popPendingAction runs the head of the queue, at index last.
func (sim *Simulation) popPendingAction(last int, next pendingEntry) bool {
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

// playerEventAt is whether the queue's head is a player's cast landing or missile hit due at t: Unit::Update
// runs those (m_Events) ahead of the unit's auras.
func (sim *Simulation) playerEventAt(head pendingEntry, t time.Duration) bool {
	if head.at != t || head.prio != ActionPriorityEvent {
		return false
	}
	pa := sim.pendingSlots[head.slot]
	return pa.playerEvent && !pa.cancelled
}

// landingAheadOfSwings is whether a player's landing due with the weapon attacks goes first.
func (sim *Simulation) landingAheadOfSwings(head pendingEntry) bool {
	return head.at == sim.minWeaponAttackTime && sim.playerEventAt(head, head.at)
}

// playerShotDue is whether a player's Auto Shot is due at t and nothing holds it back: Unit::_UpdateSpells
// fires it ahead of the unit's auras. Only once a tick, so a shot that stays due can't keep the aura pass
// waiting.
func (sim *Simulation) playerShotDue(t time.Duration) bool {
	if sim.shotPassAt == t {
		return false
	}
	for _, wa := range sim.weaponAttacks {
		if wa.isPlayerShot() && wa.swingAt <= t && !wa.held {
			if _, held := wa.castHold(sim); !held {
				return true
			}
		}
	}
	return false
}

// advanceClock moves the clock and runs the execute phases, leaving the auras to the next pass.
func (sim *Simulation) advanceClock(nextTime time.Duration) {
	sim.CurrentTime = nextTime
	sim.runExecutePhases()
}

// playerSwingDue is whether a player's melee swing is due at t and nothing holds it back: Map::Update runs
// every player's update, swings and all, ahead of the creatures'. Only once a tick, as playerShotDue.
func (sim *Simulation) playerSwingDue(t time.Duration) bool {
	if sim.meleePassAt == t {
		return false
	}
	for _, wa := range sim.weaponAttacks[:sim.playerAttacks] {
		if wa != &wa.unit.AutoAttacks.ranged && wa.swingAt <= t && !wa.held {
			if _, held := wa.castHold(sim); !held {
				return true
			}
		}
	}
	return false
}

// playerAuraDue is whether an aura a player's update counts down has run out by t.
func (sim *Simulation) playerAuraDue(t time.Duration) bool {
	for _, tracker := range sim.trackers {
		if tracker.minExpires > t {
			continue
		}
		for _, aura := range tracker.activeAuras {
			if aura.expires <= t && aura.onWorldTicks() {
				return true
			}
		}
	}
	return false
}

// advancePlayerAuras is advance for the auras the players' updates count down, leaving the creatures'.
func (sim *Simulation) advancePlayerAuras(nextTime time.Duration) {
	sim.advanceClock(nextTime)
	sim.minTrackerTime = NeverExpires
	for _, t := range sim.trackers {
		sim.minTrackerTime = min(sim.minTrackerTime, t.tryAdvance(sim, true))
	}
}

// which weapon attacks a pass swings
type attackPass uint8

const (
	allAttacks   attackPass = iota
	playerShots             // the players' Auto Shots, ahead of their auras
	playerSwings            // the players' melee, ahead of the creatures' auras
)

// advanceWeaponAttacks swings what is due, of the attacks the pass takes.
func (sim *Simulation) advanceWeaponAttacks(pass attackPass) {
	if sim.minWeaponAttackTime > sim.CurrentTime {
		sim.advance(sim.minWeaponAttackTime)
	}

	switch pass {
	case playerShots:
		sim.shotPassAt = sim.CurrentTime
	case playerSwings:
		sim.meleePassAt = sim.CurrentTime
	}
	sim.minWeaponAttackTime = NeverExpires
	// by index: a swing can take attacks off the list or put new ones on
	for i := 0; i < len(sim.weaponAttacks); i++ {
		wa := sim.weaponAttacks[i]
		if pass != allAttacks && (i >= sim.playerAttacks || (pass == playerShots) != wa.isPlayerShot()) {
			sim.minWeaponAttackTime = min(sim.minWeaponAttackTime, wa.swingAt)
			continue
		}
		sim.minWeaponAttackTime = min(sim.minWeaponAttackTime, wa.trySwing(sim, wa.releasing))
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
	sim.runExecutePhases()

	if sim.CurrentTime >= sim.minTrackerTime {
		sim.minTrackerTime = NeverExpires
		// Map::Update runs every player ahead of the map's creatures, pets and objects
		for _, t := range sim.trackers {
			t.tryAdvance(sim, true)
		}
		for _, t := range sim.trackers {
			sim.minTrackerTime = min(sim.minTrackerTime, t.tryAdvance(sim, false))
		}
	}
}

// runExecutePhases starts the execute phases the clock has reached. This is a loop to handle duplicate
// ExecuteProportions: if they're all set to 100%, advance reaches 35%, 25% and 20% in one call.
func (sim *Simulation) runExecutePhases() {
	for sim.CurrentTime >= sim.nextExecuteDuration {
		sim.startNextExecutePhase()
	}
}

func (sim *Simulation) startNextExecutePhase() {
	sim.nextExecutePhase()
	for _, callback := range sim.executePhaseCallbacks {
		callback(sim, sim.executePhase)
	}
}

// executeDamageReached queues the execute phases the damage taken has reached. The server only sets a
// unit's under 35% and under 20% aura states in its own update (Unit::Update), so those phases start on
// the target's first map update from the damage on, after the players' part of it. Nothing in the sim
// rides on 25% but Drain Soul, whose script reads the health itself, so that one starts at once.
func (sim *Simulation) executeDamageReached() {
	if sim.executeQueued {
		return
	}
	if sim.executeAction == nil {
		sim.executeAction = &PendingAction{}
		sim.executeAction.OnAction = func(sim *Simulation) {
			sim.executeQueued = false
			for sim.Encounter.DamageTaken >= sim.nextExecuteDamage {
				if at := sim.executePhaseStart(); at > sim.CurrentTime {
					sim.queueExecuteAction(at)
					return
				}
				sim.startNextExecutePhase()
			}
		}
	}
	sim.queueExecuteAction(sim.executePhaseStart())
}

func (sim *Simulation) executePhaseStart() time.Duration {
	if sim.executePhase == 35 {
		return sim.CurrentTime
	}
	return sim.NextServerTick(sim.CurrentTime)
}

func (sim *Simulation) queueExecuteAction(at time.Duration) {
	sim.executeQueued = true
	sim.executeAction.NextActionAt = at
	sim.AddPendingAction(sim.executeAction)
}

// StartTargetAtHealth puts the target at health of maxHealth, with the execute phases following the damage
// it takes from there. Call it after Reset: the fight still ends the way Reset set it up. For replaying a
// fight on a target whose health is known.
func (sim *Simulation) StartTargetAtHealth(maxHealth, health float64) {
	sim.Encounter.EndFightAtHealth = maxHealth
	sim.Encounter.DamageTaken = maxHealth - health
	sim.executePhase = 0
	sim.nextExecutePhase()
	// the phases the target is already in start now, not with the first event after the pull
	for sim.Encounter.DamageTaken >= sim.nextExecuteDamage {
		sim.nextExecutePhase()
		for _, callback := range sim.executePhaseCallbacks {
			callback(sim, sim.executePhase)
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

	for _, la := range sim.landings {
		la.onLand, la.spell, la.result, la.caster = nil, nil, nil, nil
	}
	sim.freeLandings = append(sim.freeLandings[:0], sim.landings...)
}

// One of a unit's own m_Events, a missile's landing or a cast waiting on its SpellEvent: a PendingAction
// the sim reuses. Only the pool and the queue ever point at one, so it's free once it runs, and every one
// is free when an iteration starts.
type landingAction struct {
	PendingAction
	onLand func(*Simulation) // or, when nil, spell deals result
	spell  *Spell
	result *SpellResult
	caster *Unit
	// false for a totem's spell the sim casts as the shaman: it lands in the totem's update
	ownEvent bool
	// its m_Events key, go + flight for a missile, and the order it went in at
	key time.Duration
	seq uint64
}

// startLanding queues the event at the given time, as StartDelayedAction would, without allocating.
func (sim *Simulation) startLanding(at, key time.Duration, caster *Unit, ownEvent bool, onLand func(*Simulation), spell *Spell, result *SpellResult) {
	var la *landingAction
	if n := len(sim.freeLandings); n > 0 {
		la = sim.freeLandings[n-1]
		sim.freeLandings = sim.freeLandings[:n-1]
	} else {
		la = &landingAction{}
		la.OnAction = la.land
		sim.landings = append(sim.landings, la)
	}
	la.NextActionAt = at
	la.playerEvent = ownEvent && caster.Type == PlayerUnit
	la.Priority = 0
	if la.playerEvent {
		la.Priority = ActionPriorityEvent
	}
	sim.landingSeq++
	la.onLand, la.spell, la.result, la.caster = onLand, spell, result, caster
	la.ownEvent, la.key, la.seq = ownEvent, key, sim.landingSeq
	sim.AddPendingAction(&la.PendingAction)
}

// QueueSpellEvent runs onEvent as one of the caster's m_Events on its update at: a non-triggered cast with no
// cast time waiting on its SpellEvent, as a channel's does. Keyed 1 ms past the update before (Spell::prepare),
// so it goes ahead of the landings due then.
func (sim *Simulation) QueueSpellEvent(caster *Unit, at time.Duration, onEvent func(*Simulation)) {
	key := at
	if sim.onUpdateGrid(caster) {
		key = sim.updateBefore(caster, at) + time.Millisecond
	}
	sim.startLanding(at, key, caster, true, onEvent, nil, nil)
}

// onUpdateGrid is whether the unit updates on a grid, world ticks for a player or the map update for anyone
// else, rather than with exact timing.
func (sim *Simulation) onUpdateGrid(unit *Unit) bool {
	if unit.Type == PlayerUnit {
		return sim.worldTickInterval > 0
	}
	return sim.serverTickInterval > 0
}

func (la *landingAction) land(sim *Simulation) {
	caster, ownEvent := la.caster, la.ownEvent
	if ownEvent && sim.onUpdateGrid(caster) {
		// m_Events keys a hardcast's SpellEvent at the update before + 1 ms, so it completes ahead of any
		// landing due on the same update
		caster.landDueCast(sim)
		// and runs the caster's landings due now by key, so a missile launched later with a shorter flight
		// can land first
		for {
			ahead := sim.dueLandingAhead(la)
			if ahead == nil || !sim.detachPendingAction(&ahead.PendingAction) {
				break
			}
			ahead.deal(sim)
		}
	}
	la.deal(sim)
}

// dueLandingAhead is the caster's landing due now that m_Events runs first, with the smallest key below la's,
// or nil. Landings on one key go in launch order, which the queue keeps for la itself.
func (sim *Simulation) dueLandingAhead(la *landingAction) *landingAction {
	var ahead *landingAction
	for _, other := range sim.landings {
		if other == la || other.caster != la.caster || other.ownEvent != la.ownEvent || other.NextActionAt != sim.CurrentTime ||
			other.key >= la.key {
			continue
		}
		if ahead == nil || other.key < ahead.key || other.key == ahead.key && other.seq < ahead.seq {
			ahead = other
		}
	}
	return ahead
}

// deal runs the landing and frees it.
func (la *landingAction) deal(sim *Simulation) {
	onLand, spell, result, caster := la.onLand, la.spell, la.result, la.caster
	la.onLand, la.spell, la.result, la.caster = nil, nil, nil, nil
	sim.freeLandings = append(sim.freeLandings, la)
	outerPass, outerCast := sim.eventPass, sim.landingCast
	sim.eventPass, sim.landingCast = caster, nil
	if onLand != nil {
		onLand(sim)
	} else {
		spell.DealDamage(sim, result)
	}
	sim.eventPass, sim.landingCast = outerPass, outerCast
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
