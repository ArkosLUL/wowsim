package core

import (
	"math"
	"slices"
	"testing"
	"time"

	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// evenTicks is the live server with every tick of a map update the same length.
func evenTicks() *proto.ServerSettings {
	return &proto.ServerSettings{WorldLongTickMs: googleProto.Float64(0)}
}

func liveUpdateLengths(t *testing.T, override *proto.ServerSettings) (*ServerSettings, *updateLengths) {
	t.Helper()
	settings := NewServerSettings(override)
	lengths := settings.updateLengths()
	if lengths == nil {
		t.Fatal("the live settings give every map update the same length")
	}
	return settings, lengths
}

// The steps a map update's length is drawn from keep the settings' mean and jitter. With ticks of one
// length none is under the configured interval, with a long tick the lengths go either side of the mean.
func TestUpdateLengthsKeepTheMeanAndTheJitter(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings *proto.ServerSettings
	}{{"even ticks", evenTicks()}, {"a long tick", nil}} {
		settings, lengths := liveUpdateLengths(t, tc.settings)
		_, mapUpdate := settings.updateGrids()

		sum := 0.0
		for i, length := range lengths.length {
			if i > 0 && length < lengths.length[i-1] {
				t.Fatalf("%s: step %d is %v, shorter than the step before", tc.name, i, length)
			}
			sum += float64(length)
		}
		mean := sum / updateLengthCount
		squares := 0.0
		for _, length := range lengths.length {
			squares += math.Pow(float64(length)-mean, 2)
		}
		spread := math.Sqrt(squares / updateLengthCount)

		if math.Abs(mean-float64(mapUpdate)) > float64(time.Microsecond) {
			t.Errorf("%s: the lengths average %v, want %v", tc.name, time.Duration(mean), mapUpdate)
		}
		if math.Abs(spread-float64(settings.MapUpdateJitter)) > float64(10*time.Microsecond) {
			t.Errorf("%s: the lengths spread by %v, want %v", tc.name, time.Duration(spread), settings.MapUpdateJitter)
		}
		if tc.settings != nil && (lengths.length[0] < settings.MapUpdateInterval || lengths.shortest != settings.MapUpdateInterval) {
			t.Errorf("%s: the shortest update is %v with %v as its floor, want none under MapUpdateInterval's %v", tc.name, lengths.length[0], lengths.shortest, settings.MapUpdateInterval)
		}
		if tc.settings == nil && lengths.length[0] >= settings.MapUpdateInterval {
			t.Errorf("%s: no update under the %v interval, but the instance's comes under it when the long tick shrinks", tc.name, settings.MapUpdateInterval)
		}
	}
}

// Every step's short ticks and long one add up to its length, and the long tick lands within half a short
// tick of the setting.
func TestALongTickTakesWhatTheUpdateLeaves(t *testing.T) {
	settings, lengths := liveUpdateLengths(t, nil)
	short := settings.WorldUpdateInterval
	var sumLong time.Duration
	for i, length := range lengths.length {
		if got := time.Duration(lengths.shorts[i])*short + lengths.long[i]; got != length || lengths.short[i] != short {
			t.Fatalf("step %d: %d ticks of %v and a long one of %v add up to %v, want %v", i, lengths.shorts[i], lengths.short[i], lengths.long[i], got, length)
		}
		if diff := lengths.long[i] - settings.WorldLongTick; diff < -short/2 || diff > short/2 {
			t.Errorf("step %d: a long tick of %v, want %v within half a tick", i, lengths.long[i], settings.WorldLongTick)
		}
		sumLong += lengths.long[i]
	}
	if mean := sumLong / updateLengthCount; mean < settings.WorldLongTick-time.Millisecond || mean > settings.WorldLongTick+time.Millisecond {
		t.Errorf("the long ticks average %v, want about %v", mean, settings.WorldLongTick)
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
			WorldLongTickMs:       googleProto.Float64(0),
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
	sim, _ := newSettingsTestSim(t, evenTicks(), nil)
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

// With a long tick an update is short ticks, the long one and a short one, and the lookups agree on where
// each starts.
func TestJitteredUpdatesHaveOneLongWorldTick(t *testing.T) {
	sim, _ := newSettingsTestSim(t, nil, nil)
	settings := sim.Raid.Server
	short := settings.WorldUpdateInterval

	longs := map[time.Duration]bool{}
	for seed := int64(0); seed < 5; seed++ {
		sim.reseedRands(seed)
		sim.reset()
		updates := walkUpdates(sim, sim.updates.at[0], sim.Duration)
		for i, update := range updates[:len(updates)-1] {
			next := updates[i+1]
			var ticks []time.Duration
			for tick := update; tick < next; tick = sim.NextWorldTick(tick + 1) {
				ticks = append(ticks, tick)
			}
			ticks = append(ticks, next)

			long := 0
			for j := 1; j < len(ticks); j++ {
				length := ticks[j] - ticks[j-1]
				switch {
				case length == short:
				case length > short:
					long++
					longs[length] = true
					if j != len(ticks)-2 {
						t.Fatalf("seed %d: the update at %v has its long tick of %v at %d of %d ticks, want the last but one", seed, update, length, j, len(ticks)-1)
					}
				default:
					t.Fatalf("seed %d: a tick of %v in the update at %v, under the %v floor", seed, length, update, short)
				}
				if got := sim.WorldTickBefore(ticks[j]); got != ticks[j-1] {
					t.Fatalf("seed %d: the tick before %v is %v, want %v", seed, ticks[j], got, ticks[j-1])
				}
			}
			if long != 1 {
				t.Fatalf("seed %d: %d long ticks in the update at %v, want 1", seed, long, update)
			}
		}
	}
	if len(longs) < 10 {
		t.Errorf("only %d different long ticks over five fights", len(longs))
	}
}

// A player's event waits for the next world tick: about as long as the live swings do past their attack
// time (10.8 ms, 21% over 12 ms), where even 10.44 ms ticks give 5.2 ms.
func TestAPlayersEventWaitsForALongTickLikeLive(t *testing.T) {
	sim, _ := newSettingsTestSim(t, nil, nil)
	over, late, count := 0.0, 0.0, 0.0
	for seed := int64(0); seed < 50; seed++ {
		sim.reseedRands(seed)
		sim.reset()
		for at := time.Second; at < sim.Duration-time.Second; at += 2*time.Millisecond + 123*time.Microsecond {
			wait := sim.NextWorldTick(at) - at
			over, count = over+float64(wait), count+1
			if wait > 12*time.Millisecond {
				late++
			}
		}
	}
	mean := over / count / float64(time.Millisecond)
	t.Logf("a wait for the next tick: %.2f ms on average, %.1f%% over 12 ms", mean, 100*late/count)
	if mean < 10.0 || mean > 11.6 {
		t.Errorf("an event waits %.2f ms for a world tick on average, want about 10.8", mean)
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

// The bucket table finds the same update as a search through the drawn ones, for any time asked in any order.
func TestUpdateTableFindsTheUpdateAtATime(t *testing.T) {
	for name, settings := range map[string]*proto.ServerSettings{"long tick": nil, "even ticks": evenTicks()} {
		sim, _ := newSettingsTestSim(t, settings, nil)
		updates := sim.updates
		for seed := int64(0); seed < 20; seed++ {
			sim.reseedRands(seed)
			sim.reset()
			start, end := updates.at[0], updates.limit
			var rng SplitMix64
			rng.Seed(seed)
			for range 3000 {
				at := start + time.Duration(rng.Next()%uint64(end-start))
				k := updates.stretch(at)
				want, _ := slices.BinarySearch(updates.at, at+1)
				if want-1 != k {
					t.Fatalf("%s, seed %d: update %d holds %v, the search says %d", name, seed, k, at, want-1)
				}
			}
		}
	}
}
