package core

import (
	"math"
	"math/bits"
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
//
// With a long world tick set, an update is instead short ticks of MinWorldUpdateTime and the one long tick
// (the open world's maps), and its length is normal around the mean: the cycle's own floor is 100 ms, but
// an instance updates a tick after the long one, so it comes under that when the long tick shrinks.
type updateLengths struct {
	ticks    time.Duration // world ticks in a mean map update
	mean     time.Duration // of a map update
	shortest time.Duration // the shortest map update
	longest  time.Duration // the longest world tick of any step
	world    time.Duration // the shortest world tick

	// each step, shortest update first: the update is shorts[i] ticks of short[i], then a tick of long[i],
	// then one more of short[i]
	length [updateLengthCount]time.Duration
	short  [updateLengthCount]time.Duration
	long   [updateLengthCount]time.Duration
	shorts [updateLengthCount]int32

	// the same steps in one table, a step's numbers on a cache line
	steps [updateLengthCount]updateStep

	// the long tick the steps aim for, 0 for ticks of one length
	longTarget time.Duration
}

// updateStep is one update's length and the world ticks it is cut into.
type updateStep struct {
	length, short, long time.Duration
	shorts              int32
}

func (l *updateLengths) packSteps() *updateLengths {
	for i := range l.steps {
		l.steps[i] = updateStep{l.length[i], l.short[i], l.long[i], l.shorts[i]}
	}
	return l
}

type updateLengthsKey struct {
	shortest, mean, jitter, ticks, interval, long, longJitter time.Duration
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
	if key.long > 0 {
		return newUnevenUpdateLengths(key)
	}
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
	lengths := &updateLengths{ticks: key.ticks, mean: key.mean, shortest: onTime * key.ticks, world: onTime}
	for i, step := range steps {
		tick := onTime + time.Duration(math.Round(step/float64(key.ticks)))
		lengths.length[i] = tick * key.ticks
		lengths.short[i], lengths.long[i], lengths.shorts[i] = tick, tick, int32(key.ticks-1)
	}
	lengths.longest = lengths.short[updateLengthCount-1]
	return lengths.packSteps()
}

// newUnevenUpdateLengths draws an update's length from a normal around the mean.
func newUnevenUpdateLengths(key updateLengthsKey) *updateLengths {
	scores := updateLengthScores()
	squares := 0.0
	for _, score := range scores {
		squares += score * score
	}
	// the steps cut the tail off, so the scores are scaled to a standard deviation of exactly 1
	unit := float64(key.jitter) / math.Sqrt(squares/updateLengthCount)

	// Each step's long tick aims at its own target, normal around the setting. Taking whole short ticks off
	// the update spreads the long tick by a short tick's width on top, so the targets spread by what that
	// leaves of longJitter.
	aimUnit := 0.0
	if aim := math.Pow(float64(key.longJitter), 2) - math.Pow(float64(key.interval), 2)/12; aim > 0 {
		aimUnit = math.Sqrt(aim) / math.Sqrt(squares/updateLengthCount)
	}

	lengths := &updateLengths{ticks: key.ticks, mean: key.mean, longTarget: key.long, world: key.interval}
	for i, score := range scores {
		length := key.mean + time.Duration(math.Round(score*unit))
		length = max(length, minUnevenTicks*key.interval)
		lengths.length[i] = length
		lengths.short[i] = key.interval
		target := key.long + time.Duration(math.Round(scores[longTickPairing(i)]*aimUnit))
		lengths.shorts[i], lengths.long[i] = layoutUpdate(length, key.interval, target)
		lengths.longest = max(lengths.longest, lengths.long[i], key.interval)
	}
	lengths.shortest = lengths.length[0]
	return lengths.packSteps()
}

// the fewest ticks an update with a long one has: the long tick, the short ones before and after it
const minUnevenTicks = 3

// longTickPairing is the score a step's long tick aims by. The steps go shortest update first, and the
// open world's run doesn't follow the update's length, so a step takes a score far from its own place:
// 633 is the count over the golden ratio, odd, so every score is used once.
func longTickPairing(step int) int {
	return (step*633 + updateLengthCount/2) % updateLengthCount
}

// layoutUpdate splits an update of this length into short ticks and a long one: as many short ones as
// leave the long tick nearest target, and at least one that isn't the tick before the next update.
func layoutUpdate(length, short, target time.Duration) (shorts int32, long time.Duration) {
	n := int64(math.Round(float64(length-target) / float64(short)))
	n = min(max(n, 1), int64(length/short)-1)
	return int32(n), length - time.Duration(n)*short
}

// updateJitter lays out one iteration's map updates on a host that doesn't keep time. Each update draws its
// own length, so the grid wanders the way the server's does: a creature's 2 s timer runs out on its 19th
// update or its 20th. Before the first update and from limit on every update takes the mean, in even world
// ticks, so a time far outside the fight costs nothing to place.
type updateJitter struct {
	lengths *updateLengths
	rand    SplitMix64

	at    []time.Duration // the updates drawn so far
	info  []updateStep    // how the update at at[k] is cut: its short world ticks (shorts of them, the one after the long tick included) and its long one
	limit time.Duration

	// tab[b] is the update holding the start of the b-th bucket, 1<<tabShift wide from at[0]. A bucket is
	// no wider than the shortest update, so a time in it falls in tab[b] or the update after
	tab      []int32
	tabShift uint

	bits  uint64 // random bits not used yet
	nbits int
}

func newUpdateJitter(lengths *updateLengths) *updateJitter {
	jitter := &updateJitter{lengths: lengths}
	if lengths.shortest > 1 {
		jitter.tabShift = uint(bits.Len64(uint64(lengths.shortest)) - 1)
	}
	jitter.reset(0, 0, 0)
	return jitter
}

// reset starts a new grid with an update at first, jittered up to horizon.
func (j *updateJitter) reset(first, horizon time.Duration, seed int64) {
	mean := j.lengths.mean
	// a whole number of mean updates on, so the grid past the limit starts on an update too
	j.limit = first + (max(horizon-first, 0)/mean+2)*mean
	j.at = append(j.at[:0], first)
	j.info = j.info[:0]
	j.tab, j.nbits = j.tab[:0], 0
	j.rand.Seed(seed)
}

// growTo draws updates until one starts after t.
func (j *updateJitter) growTo(t time.Duration) {
	lengths := j.lengths
	at0 := j.at[0]
	last := j.at[len(j.at)-1]
	for last <= t {
		if j.nbits < updateLengthBits {
			j.bits, j.nbits = j.rand.Next(), 64/updateLengthBits*updateLengthBits
		}
		step := j.bits & (updateLengthCount - 1)
		j.bits >>= updateLengthBits
		j.nbits -= updateLengthBits

		info := lengths.steps[step]
		next := last + info.length
		if next > j.limit-lengths.shortest {
			// no room for another one after it, so this one runs up to the limit
			next = j.limit
			if lengths.longTarget > 0 {
				info.shorts, info.long = layoutUpdate(next-last, info.short, lengths.longTarget)
			} else {
				info.short = (j.limit - last) / lengths.ticks
				info.long = info.short
			}
		}
		// the buckets starting inside this update
		for at0+time.Duration(len(j.tab))<<j.tabShift < next {
			j.tab = append(j.tab, int32(len(j.at)-1))
		}
		j.at = append(j.at, next)
		j.info = append(j.info, info)
		last = next
	}
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
	if j.at[len(j.at)-1] <= t {
		j.growTo(t)
	}
	k := int(j.tab[uint64(t-j.at[0])>>j.tabShift])
	if j.at[k+1] <= t {
		k++
	}
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
	at, over, info := j.at[k], t-j.at[k], &j.info[k]
	switch head := time.Duration(info.shorts-1) * info.short; {
	case over <= head:
		if rest := over % info.short; rest > 0 {
			return t + info.short - rest
		}
		return t
	case over <= head+info.long:
		return at + head + info.long
	}
	return j.at[k+1]
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
	at, over, info := j.at[k], t-1-j.at[k], &j.info[k]
	switch head := time.Duration(info.shorts-1) * info.short; {
	case over >= head+info.long:
		return at + head + info.long
	case over >= head:
		return at + head
	}
	return t - 1 - over%info.short
}
