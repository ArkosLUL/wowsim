package core

import (
	"math"
	"sync"
	"time"
)

// A map update takes one of this many lengths, each as likely as the next.
const (
	updateLengthBits  = 10
	updateLengthCount = 1 << updateLengthBits

	// what sets the updates' random stream apart from the iteration's own
	updateJitterSeed = 0x6a69747465726564
)

// updateLengths is how long a map update can take on a host whose world loop runs late. MapMgr::Update
// starts one on the first world tick its interval is up by, and WorldUpdateLoop sleeps out what's left of
// MinWorldUpdateTime, so a late wake-up or a stall only adds: no update is under the configured interval,
// and what the ticks add scatters above it. That part is lognormal, cut into equally likely steps that
// keep its mean and its standard deviation.
type updateLengths struct {
	ticks    time.Duration // world ticks in a map update
	mean     time.Duration // of a map update
	shortest time.Duration // a map update with every tick on time

	// a world tick's length in each step, shortest first: the update takes ticks of them
	tick [updateLengthCount]time.Duration
}

type updateLengthsKey struct {
	shortest, mean, jitter, ticks time.Duration
}

var (
	updateLengthsMu    sync.Mutex
	updateLengthsCache = map[updateLengthsKey]*updateLengths{}
)

// updateLengthsFor builds each set of lengths once: every sim under the same settings reads the same one.
func updateLengthsFor(key updateLengthsKey) *updateLengths {
	updateLengthsMu.Lock()
	defer updateLengthsMu.Unlock()
	lengths := updateLengthsCache[key]
	if lengths == nil {
		lengths = newUpdateLengths(key)
		updateLengthsCache[key] = lengths
	}
	return lengths
}

// updateLengthScores is the standard normal at the middle of each step.
var updateLengthScores = sync.OnceValue(func() *[updateLengthCount]float64 {
	var scores [updateLengthCount]float64
	for i := range scores {
		scores[i] = math.Sqrt2 * math.Erfinv(2*(float64(i)+0.5)/updateLengthCount-1)
	}
	return &scores
})

func newUpdateLengths(key updateLengthsKey) *updateLengths {
	scores := updateLengthScores()
	excess := float64(key.mean - key.shortest)

	// what each step adds for a lognormal of this shape, scaled to the mean, and how far they spread
	var steps [updateLengthCount]float64
	spread := func(shape float64) float64 {
		sum := 0.0
		for i, score := range scores {
			steps[i] = math.Exp(shape * score)
			sum += steps[i]
		}
		scale := excess * updateLengthCount / sum
		squares := 0.0
		for i := range steps {
			steps[i] *= scale
			squares += (steps[i] - excess) * (steps[i] - excess)
		}
		return math.Sqrt(squares / updateLengthCount)
	}
	// the steps cut the tail off, which loses spread: widen the shape until they have all of it
	narrow, wide := 0.0, 8.0
	for range 60 {
		shape := (narrow + wide) / 2
		if spread(shape) < float64(key.jitter) {
			narrow = shape
		} else {
			wide = shape
		}
	}
	spread(wide)

	onTime := (key.shortest + key.ticks - 1) / key.ticks
	lengths := &updateLengths{ticks: key.ticks, mean: key.mean, shortest: onTime * key.ticks}
	for i, step := range steps {
		lengths.tick[i] = onTime + time.Duration(math.Round(step/float64(key.ticks)))
	}
	return lengths
}

// updateJitter lays out one iteration's map updates on a host that doesn't keep time. Each update draws its
// own length, so the grid wanders the way the server's does: a creature's 2 s timer runs out on its 19th
// update or its 20th. The world ticks split their update evenly. Before the first update and from limit on
// every update takes the mean, so a time far outside the fight costs nothing to place.
type updateJitter struct {
	lengths *updateLengths
	rand    SplitMix64

	at    []time.Duration // the updates drawn so far
	tick  []time.Duration // the world tick between at[k] and at[k+1]
	limit time.Duration

	perMean float64 // one over the mean update
	cursor  int     // where the last lookup landed
	bits    uint64  // random bits not used yet
	nbits   int
}

func newUpdateJitter(lengths *updateLengths) *updateJitter {
	jitter := &updateJitter{lengths: lengths, perMean: 1 / float64(lengths.mean)}
	jitter.reset(0, 0, 0)
	return jitter
}

// reset starts a new grid with an update at first, jittered up to horizon.
func (j *updateJitter) reset(first, horizon time.Duration, seed int64) {
	mean := j.lengths.mean
	// a whole number of mean updates on, so the grid past the limit starts on an update too
	j.limit = first + (max(horizon-first, 0)/mean+2)*mean
	j.at = append(j.at[:0], first)
	j.tick = j.tick[:0]
	j.cursor, j.nbits = 0, 0
	j.rand.Seed(seed)
}

// grow draws the next update.
func (j *updateJitter) grow() {
	if j.nbits < updateLengthBits {
		j.bits, j.nbits = j.rand.Next(), 64/updateLengthBits*updateLengthBits
	}
	tick := j.lengths.tick[j.bits&(updateLengthCount-1)]
	j.bits >>= updateLengthBits
	j.nbits -= updateLengthBits

	last := j.at[len(j.at)-1]
	next := last + tick*j.lengths.ticks
	if next > j.limit-j.lengths.shortest {
		// no room for another one after it, so this one runs up to the limit
		next, tick = j.limit, (j.limit-last)/j.lengths.ticks
	}
	j.at = append(j.at, next)
	j.tick = append(j.tick, tick)
}

// drawn is whether t falls among the jittered updates.
func (j *updateJitter) drawn(t time.Duration) bool {
	return t >= j.at[0] && t < j.limit
}

// meanGrid is where the mean grid t falls on is anchored.
func (j *updateJitter) meanGrid(t time.Duration) time.Duration {
	if t <= j.at[0] {
		return j.at[0]
	}
	return j.limit
}

// stretch is the k with at[k] <= t < at[k+1], for a drawn t.
func (j *updateJitter) stretch(t time.Duration) int {
	for j.at[len(j.at)-1] <= t {
		j.grow()
	}
	k := j.cursor
	if t >= j.at[k] && t < j.at[k+1] {
		return k
	}
	// the mean gets within a step or two of it
	k += int(float64(t-j.at[k]) * j.perMean)
	k = min(max(k, 0), len(j.at)-2)
	for j.at[k] > t {
		k--
	}
	for j.at[k+1] <= t {
		k++
	}
	j.cursor = k
	return k
}

// nextUpdate is the first map update at or after t.
func (j *updateJitter) nextUpdate(t time.Duration) time.Duration {
	if !j.drawn(t) {
		return nextTick(t, j.lengths.mean, j.meanGrid(t))
	}
	k := j.stretch(t)
	if j.at[k] == t {
		return t
	}
	return j.at[k+1]
}

// nextWorldTick is the first world tick at or after t.
func (j *updateJitter) nextWorldTick(t time.Duration) time.Duration {
	if !j.drawn(t) {
		return nextTick(t, j.lengths.mean/j.lengths.ticks, j.meanGrid(t))
	}
	k := j.stretch(t)
	if over := (t - j.at[k]) % j.tick[k]; over > 0 {
		return t + j.tick[k] - over
	}
	return t
}

// updateBefore is the last map update before t.
func (j *updateJitter) updateBefore(t time.Duration) time.Duration {
	if !j.drawn(t - 1) {
		return nextTick(t, j.lengths.mean, j.meanGrid(t)) - j.lengths.mean
	}
	return j.at[j.stretch(t-1)]
}

// worldTickBefore is the last world tick before t.
func (j *updateJitter) worldTickBefore(t time.Duration) time.Duration {
	if !j.drawn(t - 1) {
		tick := j.lengths.mean / j.lengths.ticks
		return nextTick(t, tick, j.meanGrid(t)) - tick
	}
	k := j.stretch(t - 1)
	return t - 1 - (t-1-j.at[k])%j.tick[k]
}
