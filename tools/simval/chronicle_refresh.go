package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// refreshDelay is one dot/buff application paired with the crit that (on the server)
// queued it, through core.DelayedPeriodicApplier. The gap between the two is that
// applier's own landing time, which a live capture is the only way to measure: chronicle
// has no window phase to check it against analytically.
type refreshDelay struct {
	FeederSpellID int32
	CritDamage    int64
	CritMs        int64
	RefreshMs     int64
}

func (r refreshDelay) delayMs() int64 { return r.RefreshMs - r.CritMs }

// refreshOptions configures pairRefreshes.
type refreshOptions struct {
	AuraSpellID int32   // the dot/buff whose SPELL_AURA_APPLIED events are refreshes
	Feeders     []int32 // spell ids whose crits (direct hits or periodic ticks) can feed a refresh
	WindowMs    int64   // a crit further back than this can't be the one that fed the refresh
}

// pairRefreshes walks one player's events in order and, for every SPELL_AURA_APPLIED of
// AuraSpellID, claims the most recent still-unclaimed crit from a listed feeder spell
// within WindowMs. Crits don't land their refresh in the order they happened — each
// draws its own 0-400 ms landing, so an earlier crit's application can arrive after a
// later crit's — so every crit is kept pending until some refresh claims it, rather than
// letting a newer crit simply displace an older, still-unfired one.
//
// On the server this aura only ever comes from a crit, so an application chronicle
// can't pair is a sign Feeders is missing an id or WindowMs is too tight, not a proc
// with no cause; callers should treat a nonzero Unpaired count as a report to fix, not
// data to drop silently. A crit that never gets claimed (ExcessFeederCrits) means the
// proc isn't the unconditional one Feeders assumed.
func pairRefreshes(log *chronicleLog, source string, opts refreshOptions) (pairs []refreshDelay, unpairedMs []int64, excessFeederCrits int) {
	feederSet := make(map[int32]bool, len(opts.Feeders))
	for _, id := range opts.Feeders {
		feederSet[id] = true
	}

	var pending []refreshDelay // unclaimed crits, oldest first
	for _, event := range log.Events {
		if !strings.EqualFold(event.Src.Name, source) {
			continue
		}
		switch {
		case event.isDamage() && feederSet[event.Spell.ID]:
			if hit := event.damage(); hit.Crit {
				pending = append(pending, refreshDelay{FeederSpellID: event.Spell.ID, CritDamage: hit.Amount, CritMs: event.TimeMs})
			}
		case event.Type == "SPELL_AURA_APPLIED" && event.Spell.ID == opts.AuraSpellID:
			claimed := -1
			for i := len(pending) - 1; i >= 0; i-- {
				if event.TimeMs-pending[i].CritMs <= opts.WindowMs {
					claimed = i
					break
				}
				// pending is oldest-first, so once one candidate is too old, the ones
				// before it are even older and can be dropped as unclaimable here too.
				excessFeederCrits++
				pending = append(pending[:i], pending[i+1:]...)
			}
			if claimed < 0 {
				unpairedMs = append(unpairedMs, event.TimeMs)
				continue
			}
			p := pending[claimed]
			p.RefreshMs = event.TimeMs
			pairs = append(pairs, p)
			pending = append(pending[:claimed], pending[claimed+1:]...)
		}
	}
	excessFeederCrits += len(pending)
	return pairs, unpairedMs, excessFeederCrits
}

// reportRefreshDelays prints each feeder's paired count, mean and median delay and a
// 50 ms histogram, then the same pooled across every feeder (count-weighted, since
// it's just every pair's mean). Unpaired applications are called out since they mean
// the feeder list or window is incomplete, not that the server refreshed with no crit.
func reportRefreshDelays(out io.Writer, auraName string, auraID int32, pairs []refreshDelay, unpairedMs []int64, excessFeederCrits int, verbose bool) {
	if len(pairs) == 0 {
		fmt.Fprintf(out, "\nno %s (%d) refresh paired with a feeder crit\n", auraName, auraID)
		return
	}
	if verbose {
		fmt.Fprintf(out, "\n%s (%d) refresh pairs (feeder, crit damage, crit ms, refresh ms, delay ms):\n", auraName, auraID)
		for _, p := range pairs {
			fmt.Fprintf(out, "  %d %d %d %d %d\n", p.FeederSpellID, p.CritDamage, p.CritMs, p.RefreshMs, p.delayMs())
		}
	}

	byFeeder := map[int32][]refreshDelay{}
	var feederIDs []int32
	for _, p := range pairs {
		if _, ok := byFeeder[p.FeederSpellID]; !ok {
			feederIDs = append(feederIDs, p.FeederSpellID)
		}
		byFeeder[p.FeederSpellID] = append(byFeeder[p.FeederSpellID], p)
	}
	sort.Slice(feederIDs, func(i, j int) bool { return feederIDs[i] < feederIDs[j] })

	fmt.Fprintf(out, "\n%s (%d) refresh delay, feeder crit -> SPELL_AURA_APPLIED:\n", auraName, auraID)
	for _, id := range feederIDs {
		printDelaySummary(out, "  spell "+strconv.Itoa(int(id)), byFeeder[id])
	}
	if len(feederIDs) > 1 {
		printDelaySummary(out, "  all feeders", pairs)
	}
	if len(unpairedMs) > 0 {
		fmt.Fprintf(out, "  %d application(s) with no feeder crit inside the window: check -refreshFeeders and -refreshWindow\n",
			len(unpairedMs))
	}
	if excessFeederCrits > 0 {
		fmt.Fprintf(out, "  %d feeder crit(s) claimed by no application: the proc isn't unconditional on every one, or the window is too tight\n",
			excessFeederCrits)
	}
}

func printDelaySummary(out io.Writer, label string, pairs []refreshDelay) {
	delays := make([]int64, len(pairs))
	for i, p := range pairs {
		delays[i] = p.delayMs()
	}
	sort.Slice(delays, func(i, j int) bool { return delays[i] < delays[j] })
	fmt.Fprintf(out, "%s: %d, mean %.0f ms, median %d ms\n", label, len(delays), meanMs(delays), median(delays))
	fmt.Fprintf(out, "    %s\n", histogram(delays, 50))
}
