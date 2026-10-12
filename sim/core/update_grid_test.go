package core

import (
	"math"
	"slices"
	"testing"
	"time"

	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func liveUpdateLengths(t *testing.T) (*ServerSettings, *updateLengths) {
	t.Helper()
	settings := NewServerSettings(nil)
	lengths := settings.updateLengths()
	if lengths == nil {
		t.Fatal("the live settings give every map update the same length")
	}
	return settings, lengths
}

// The steps a map update's length is drawn from keep the settings' mean and jitter, and none is under the
// configured interval.
func TestUpdateLengthsKeepTheMeanAndTheJitter(t *testing.T) {
	settings, lengths := liveUpdateLengths(t)
	_, mapUpdate := settings.updateGrids()

	sum := 0.0
	for i, tick := range lengths.tick {
		if i > 0 && tick < lengths.tick[i-1] {
			t.Fatalf("step %d is %v a tick, shorter than the step before", i, tick)
		}
		sum += float64(tick * lengths.ticks)
	}
	mean := sum / updateLengthCount
	squares := 0.0
	for _, tick := range lengths.tick {
		squares += math.Pow(float64(tick*lengths.ticks)-mean, 2)
	}
	spread := math.Sqrt(squares / updateLengthCount)

	if math.Abs(mean-float64(mapUpdate)) > float64(time.Microsecond) {
		t.Errorf("the lengths average %v, want %v", time.Duration(mean), mapUpdate)
	}
	if math.Abs(spread-float64(settings.MapUpdateJitter)) > float64(10*time.Microsecond) {
		t.Errorf("the lengths spread by %v, want %v", time.Duration(spread), settings.MapUpdateJitter)
	}
	if shortest := lengths.tick[0] * lengths.ticks; shortest < settings.MapUpdateInterval || lengths.shortest != settings.MapUpdateInterval {
		t.Errorf("the shortest update is %v with %v as its floor, want none under MapUpdateInterval's %v", shortest, lengths.shortest, settings.MapUpdateInterval)
	}
}

// An update can only stray when the world loop runs late: with every tick on time, ten of them are the
// interval and there's nothing to add.
func TestUpdatesOnlyJitterUnderALateWorldLoop(t *testing.T) {
	for _, tc := range []struct {
		name               string
		mapMs, worldMs     int32
		loopMs, jitterMs   float64
		wantJitter         bool
		wantShortestUpdate time.Duration
	}{
		{"live", 100, 10, 10.46, 6, true, ms(100)},
		{"no jitter set", 100, 10, 10.46, 0, false, 0},
		{"a loop on time", 100, 10, 0, 6, false, 0},
		{"exact players", 100, 0, 10.46, 6, false, 0},
		{"exact for everyone", 0, 10, 10.46, 6, false, 0},
		// eleven ticks of at least 10 ms, whatever the interval says
		{"an interval off the ticks", 105, 10, 10.46, 6, true, ms(110)},
	} {
		lengths := NewServerSettings(&proto.ServerSettings{
			MapUpdateIntervalMs:   googleProto.Int32(tc.mapMs),
			WorldUpdateIntervalMs: googleProto.Int32(tc.worldMs),
			WorldLoopMs:           googleProto.Float64(tc.loopMs),
			MapUpdateJitterMs:     googleProto.Float64(tc.jitterMs),
		}).updateLengths()
		if (lengths != nil) != tc.wantJitter {
			t.Errorf("%s: jitter %v, want %v", tc.name, lengths != nil, tc.wantJitter)
			continue
		}
		if lengths != nil && lengths.shortest != tc.wantShortestUpdate {
			t.Errorf("%s: shortest update %v, want %v", tc.name, lengths.shortest, tc.wantShortestUpdate)
		}
	}
}

// walkUpdates lists the map updates from the first at or after from up to the first past until.
func walkUpdates(sim *Simulation, from, until time.Duration) []time.Duration {
	updates := []time.Duration{sim.NextServerTick(from)}
	for updates[len(updates)-1] <= until {
		updates = append(updates, sim.NextServerTick(updates[len(updates)-1]+1))
	}
	return updates
}

// Whatever its length, an update is ten even world ticks, and the lookups agree on where it starts and ends.
func TestJitteredUpdatesAreTenEvenWorldTicks(t *testing.T) {
	sim, _ := newSettingsTestSim(t, nil, nil)
	lengths := sim.updates.lengths
	if sim.serverTickInterval != lengths.shortest || sim.worldTickInterval*lengths.ticks != lengths.shortest {
		t.Fatalf("intervals %v and %v, want the shortest update's %v and its tick", sim.serverTickInterval, sim.worldTickInterval, lengths.shortest)
	}

	seen := map[time.Duration]bool{}
	for seed := int64(0); seed < 5; seed++ {
		sim.reseedRands(seed)
		sim.reset()
		updates := walkUpdates(sim, -2*time.Second, sim.Duration)
		for i, update := range updates[:len(updates)-1] {
			next := updates[i+1]
			length := next - update
			seen[length] = true
			if length < lengths.shortest {
				t.Fatalf("seed %d: the update at %v takes %v, under the %v floor", seed, update, length, lengths.shortest)
			}
			tick := length / lengths.ticks
			if tick*lengths.ticks != length {
				t.Fatalf("seed %d: the update at %v takes %v, not a whole number of ticks", seed, update, length)
			}
			if got := sim.NextWorldTick(update); got != update {
				t.Fatalf("seed %d: the update at %v isn't a world tick, the next one is %v", seed, update, got)
			}
			for n := time.Duration(1); n <= lengths.ticks; n++ {
				worldTick := update + n*tick
				if got := sim.NextWorldTick(worldTick - tick + 1); got != worldTick {
					t.Fatalf("seed %d: world tick %d after %v at %v, want %v", seed, n, update, got, worldTick)
				}
				if got := sim.WorldTickBefore(worldTick); got != worldTick-tick {
					t.Fatalf("seed %d: the tick before %v is %v, want %v", seed, worldTick, got, worldTick-tick)
				}
			}
			middle := update + length/2
			if got := sim.NextServerTick(middle); got != next {
				t.Fatalf("seed %d: the update after %v is %v, want %v", seed, middle, got, next)
			}
			if before, atOrBefore := sim.ServerTickBefore(next), sim.ServerTickBefore(update+1); before != update || atOrBefore != update {
				t.Fatalf("seed %d: the update before %v is %v and the one at %v is %v", seed, next, before, update, atOrBefore)
			}
			if got := sim.ServerTickBefore(middle); got != update {
				t.Fatalf("seed %d: the update before %v is %v, want %v", seed, middle, got, update)
			}
		}
	}
	if len(seen) < 100 {
		t.Errorf("only %d different update lengths over five fights", len(seen))
	}
}

// What an update turns out to be never depends on which times were asked about first.
func TestJitteredUpdatesDontDependOnTheLookupOrder(t *testing.T) {
	sim, _ := newSettingsTestSim(t, nil, nil)
	sim.reseedRands(7)
	sim.reset()
	inOrder := walkUpdates(sim, -time.Second, sim.Duration)

	sim.reseedRands(7)
	sim.reset()
	sim.NextWorldTick(40 * time.Second)
	sim.ServerTickBefore(5 * time.Second)
	sim.NextServerTick(NeverExpires)
	sim.WorldTickBefore(55 * time.Second)
	if got := walkUpdates(sim, -time.Second, sim.Duration); !slices.Equal(got, inOrder) {
		t.Error("the updates moved with the order they were looked up in")
	}

	sim.reseedRands(8)
	sim.reset()
	if got := walkUpdates(sim, -time.Second, sim.Duration); slices.Equal(got, inOrder) {
		t.Error("another iteration has the same updates")
	}
}

// Before the sim starts and once the fight has to be over, the updates take the mean, so placing a time
// far off draws nothing.
func TestUpdatesTakeTheMeanOutsideTheFight(t *testing.T) {
	sim, _ := newSettingsTestSim(t, nil, nil)
	sim.reseedRands(3)
	sim.reset()
	mean := sim.updates.lengths.mean
	tick := mean / sim.updates.lengths.ticks

	for _, at := range []time.Duration{-10 * time.Minute, -time.Minute, sim.Duration + time.Minute, sim.Duration + 30*time.Minute} {
		next := sim.NextServerTick(at)
		if next < at || next-at >= mean {
			t.Errorf("the update after %v is %v", at, next)
		}
		if after, before := sim.NextServerTick(next+1), sim.ServerTickBefore(next); after-next != mean || next-before != mean {
			t.Errorf("around %v the updates are %v and %v apart, want the mean %v", next, next-before, after-next, mean)
		}
		if after, before := sim.NextWorldTick(next+1), sim.WorldTickBefore(next); after-next != tick || next-before != tick {
			t.Errorf("around %v the world ticks are %v and %v apart, want %v", next, next-before, after-next, tick)
		}
	}
	if got := sim.NextServerTick(NeverExpires); got != NeverExpires {
		t.Errorf("NextServerTick(NeverExpires) = %v", got)
	}
	if drawn := len(sim.updates.at); drawn != 1 {
		t.Errorf("%d updates drawn for times outside the fight, want none past the first", drawn-1)
	}

	// the two grids meet on an update at both ends
	first, limit := sim.updates.at[0], sim.updates.limit
	if sim.NextServerTick(first) != first || sim.ServerTickBefore(first) != first-mean {
		t.Errorf("the first drawn update, %v, isn't one of the grid before it", first)
	}
	if last := sim.ServerTickBefore(limit); sim.NextServerTick(limit) != limit || limit-last < sim.updates.lengths.shortest {
		t.Errorf("the grid past %v starts %v after the last drawn update", limit, limit-last)
	}
}

// A creature's timer runs out on the first update it's up by, so on a grid that wanders its length isn't
// one number. The live records: the Naxxramas slimes' 2 s cast timer takes 2065 ms on average, on the 19th
// update a quarter of the time, and a Mirror Image's 3 s Frostbolt 3048 ms, anywhere from 3.00 to 3.10 s.
func TestACreaturesTimersWanderWithTheUpdates(t *testing.T) {
	sim, _ := newSettingsTestSim(t, nil, nil)
	gaps := func(timer time.Duration) (mean float64, early float64, shortest, longest time.Duration) {
		sum, count, earlies := 0.0, 0.0, 0.0
		shortest = NeverExpires
		for seed := int64(0); seed < 300; seed++ {
			sim.reseedRands(seed)
			sim.reset()
			for at := sim.NextServerTick(0); ; {
				next := sim.NextServerTick(at + timer)
				if next > sim.Duration {
					break
				}
				gap := next - at
				sum, count = sum+float64(gap), count+1
				if gap < timer+35*time.Millisecond {
					earlies++
				}
				shortest, longest = min(shortest, gap), max(longest, gap)
				at = next
			}
		}
		return sum / count / float64(time.Millisecond), earlies / count, shortest, longest
	}

	mean, early, shortest, longest := gaps(2 * time.Second)
	t.Logf("a 2 s timer: %.1f ms on average, %.1f%% under 2035 ms, %v to %v", mean, 100*early, shortest, longest)
	if mean < 2061 || mean > 2069 || early < 0.15 || early > 0.30 || shortest < 2*time.Second {
		t.Errorf("a 2 s timer takes %.1f ms on average, %.0f%% of them under 2035 ms, the shortest %v; want about 2065 ms and a fifth to a quarter", mean, 100*early, shortest)
	}
	mean, _, shortest, longest = gaps(3 * time.Second)
	t.Logf("a 3 s timer: %.1f ms on average, %v to %v", mean, shortest, longest)
	if mean < 3039 || mean > 3050 || shortest < 3*time.Second || longest > 3200*time.Millisecond {
		t.Errorf("a 3 s timer takes %.1f ms on average, from %v to %v; want about 3045 ms", mean, shortest, longest)
	}
}
