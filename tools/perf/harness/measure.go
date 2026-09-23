package harness

import (
	"runtime"
	"runtime/metrics"
	"sync"
	"time"
)

const (
	metricAllocs = "/gc/heap/allocs:bytes"
	metricHeap   = "/memory/classes/heap/objects:bytes"
	metricGC     = "/cpu/classes/gc/total:cpu-seconds"
	metricGCIdle = "/cpu/classes/gc/mark/idle:cpu-seconds"
	metricTotal  = "/cpu/classes/total:cpu-seconds"
	metricIdle   = "/cpu/classes/idle:cpu-seconds"
)

const heapSampleEvery = 10 * time.Millisecond

type runtimeStats struct {
	allocs                        uint64
	gc, gcIdle, cpuTotal, cpuIdle float64
}

func readRuntime() runtimeStats {
	samples := []metrics.Sample{{Name: metricAllocs}, {Name: metricGC}, {Name: metricGCIdle}, {Name: metricTotal}, {Name: metricIdle}}
	metrics.Read(samples)
	return runtimeStats{
		allocs:   samples[0].Value.Uint64(),
		gc:       samples[1].Value.Float64(),
		gcIdle:   samples[2].Value.Float64(),
		cpuTotal: samples[3].Value.Float64(),
		cpuIdle:  samples[4].Value.Float64(),
	}
}

// gcShare is the GC's share of the CPU the runtime counted between two reads. The runtime's CPU
// classes are estimates that only move at the end of a GC cycle, so this compares them with each
// other and never with the real CPU time. Idle-priority mark workers only run on Ps nothing else
// wanted, so they count on neither side.
func gcShare(before, after runtimeStats) float64 {
	gc := (after.gc - before.gc) - (after.gcIdle - before.gcIdle)
	used := (after.cpuTotal - before.cpuTotal) - (after.cpuIdle - before.cpuIdle) - (after.gcIdle - before.gcIdle)
	if used <= 0 {
		return 0
	}
	return gc / used
}

// heapSampler tracks the largest heap it sees.
type heapSampler struct {
	stopc chan struct{}
	wg    sync.WaitGroup
	peak  uint64
}

func startHeapSampler() *heapSampler {
	h := &heapSampler{stopc: make(chan struct{})}
	h.sample()
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		tick := time.NewTicker(heapSampleEvery)
		defer tick.Stop()
		for {
			select {
			case <-h.stopc:
				return
			case <-tick.C:
				h.sample()
			}
		}
	}()
	return h
}

func (h *heapSampler) sample() {
	s := []metrics.Sample{{Name: metricHeap}}
	metrics.Read(s)
	h.peak = max(h.peak, s[0].Value.Uint64())
}

func (h *heapSampler) stop() uint64 {
	close(h.stopc)
	h.wg.Wait()
	h.sample()
	return h.peak
}

// measure times one run of fn, which may fill in the optimizer's fields of the sample it returns.
func measure(fn func() (*Sample, error)) (Sample, error) {
	runtime.GC()
	before := readRuntime()
	cpuBefore, err := cpuTime()
	if err != nil {
		return Sample{}, err
	}
	heap := startHeapSampler()
	start := time.Now()
	extra, err := fn()
	wall := time.Since(start)
	cpuAfter, cpuErr := cpuTime()
	peak := heap.stop()
	after := readRuntime()
	if err != nil {
		return Sample{}, err
	}
	if cpuErr != nil {
		return Sample{}, cpuErr
	}

	var s Sample
	if extra != nil {
		s = *extra
	}
	s.Wall = wall.Seconds()
	s.CPU = (cpuAfter - cpuBefore).Seconds()
	s.Util = s.CPU / s.Wall / float64(runtime.GOMAXPROCS(0))
	s.GCShare = gcShare(before, after)
	s.Alloc = after.allocs - before.allocs
	s.PeakHeap = peak
	return s, nil
}
