package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// maxDPSGapPct is how far a recorded run's DPS may sit from the sim's with the
// same gear, talents and rotation (azerothcore-parity.PLAN.md, "Verification").
const maxDPSGapPct = 2.0

type chronicleOptions struct {
	Source  string  // the player to measure; empty picks the log's only one
	Target  string  // only count damage dealt to this unit
	SimDPS  float64 // the sim's result for the same setup, 0 to skip the comparison
	GapSec  float64 // a pause longer than this is left out of the active duration
	Top     int     // how many ability rows to print
	Verbose bool    // also print swing intervals, tick intervals and aura uptimes
}

// abilityKey keeps a pet's abilities apart from its owner's; both end up in the
// same DPS, the way the sim adds a pet's damage to the player's.
type abilityKey struct {
	SpellID int32
	Minion  string // the pet or guardian that used it, empty for the player
}

type abilityStats struct {
	Key      abilityKey
	Name     string
	Hits     int
	Ticks    int
	Crits    int
	Glancing int
	Crushing int
	Damage   int64
	Absorbed int64
	Misses   map[string]int
}

func (a *abilityStats) attempts() int { return a.Hits + a.Ticks + a.missCount() }

func (a *abilityStats) missCount() int {
	var n int
	for _, count := range a.Misses {
		n += count
	}
	return n
}

// targetKey splits a per-target series out of an ability: a DoT on two mobs ticks
// on two timelines, and a SPELL_AURA_REMOVED closes only its own window. Damage
// still adds up across targets, so abilityKey carries no target.
type targetKey struct {
	Ability abilityKey
	Dst     string // the destination's GUID
}

type auraStats struct {
	SpellID    int32
	Name       string
	TargetName string
	Applied    int
	UptimeMs   int64
	openedAt   int64
	open       bool
}

type tickSeries struct {
	Key        targetKey
	TargetName string
	Times      []int64
}

// runStats is everything the report reads out of one player's share of a log.
type runStats struct {
	Source      combatant
	StartMs     int64
	EndMs       int64
	PauseMs     int64 // time spent in gaps longer than GapSec
	Damage      int64
	Abilities   map[abilityKey]*abilityStats
	SwingTimes  []int64
	TickTimes   map[targetKey]*tickSeries
	Auras       map[targetKey]*auraStats
	MinionNames []string
}

func (r *runStats) durationSec() float64 { return float64(r.EndMs-r.StartMs) / 1000 }
func (r *runStats) activeSec() float64   { return float64(r.EndMs-r.StartMs-r.PauseMs) / 1000 }
func (r *runStats) dps() float64         { return perSecond(r.Damage, r.durationSec()) }
func (r *runStats) activeDPS() float64   { return perSecond(r.Damage, r.activeSec()) }

// pause is how much of the span between two actions counts as standing still.
func pause(gapMs, lastMs, nowMs int64) int64 {
	if gapMs <= 0 || lastMs == 0 || nowMs-lastMs <= gapMs {
		return 0
	}
	return nowMs - lastMs
}

func perSecond(amount int64, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return float64(amount) / seconds
}

// analyzeChronicle measures one player's damage, swings, ticks and auras. Damage
// by the pets and guardians it owns counts for the player, the way the sim adds
// its pets to the player's DPS.
func analyzeChronicle(log *chronicleLog, opts chronicleOptions) (*runStats, error) {
	source, err := pickSource(log, opts.Source)
	if err != nil {
		return nil, err
	}

	mine := map[string]bool{source.GUID: true}
	for guid := range log.Owners {
		if ownerOf(log, guid) == source.GUID {
			mine[guid] = true
		}
	}

	stats := &runStats{
		Source:    source,
		Abilities: map[abilityKey]*abilityStats{},
		TickTimes: map[targetKey]*tickSeries{},
		Auras:     map[targetKey]*auraStats{},
	}
	for guid := range mine {
		if guid != source.GUID {
			stats.MinionNames = append(stats.MinionNames, log.Names[guid])
		}
	}
	sort.Strings(stats.MinionNames)

	gapMs := int64(opts.GapSec * 1000)
	// A pause is measured from the last swing or cast the player made, not from
	// the last one that landed: a run of misses is still fighting.
	var lastActionMs int64
	for _, event := range log.Events {
		if !mine[event.Src.GUID] {
			continue
		}
		if opts.Target != "" && (event.isDamage() || event.isMiss()) && !strings.EqualFold(event.Dst.Name, opts.Target) {
			continue
		}

		switch {
		case event.isDamage():
			hit := event.damage()
			ability := stats.ability(source, event)
			if event.Type == "SPELL_PERIODIC_DAMAGE" {
				ability.Ticks++
				series := stats.ticks(ability.Key, event)
				series.Times = append(series.Times, event.TimeMs)
			} else {
				ability.Hits++
			}
			if hit.Crit {
				ability.Crits++
			}
			if hit.Glancing {
				ability.Glancing++
			}
			if hit.Crushing {
				ability.Crushing++
			}
			ability.Damage += hit.Amount
			ability.Absorbed += hit.Absorbed
			stats.Damage += hit.Amount

			if stats.StartMs == 0 {
				stats.StartMs = event.TimeMs
			}
			stats.PauseMs += pause(gapMs, lastActionMs, event.TimeMs)
			lastActionMs = event.TimeMs
			stats.EndMs = event.TimeMs

		case event.isMiss():
			stats.ability(source, event).Misses[event.missType()]++
			if stats.StartMs != 0 {
				stats.PauseMs += pause(gapMs, lastActionMs, event.TimeMs)
				lastActionMs = event.TimeMs
			}

		case event.Type == "SPELL_AURA_APPLIED":
			stats.aura(source, event).apply(event.TimeMs)

		case event.Type == "SPELL_AURA_REMOVED":
			stats.aura(source, event).remove(event.TimeMs)
		}

		if event.isSwing() && event.Src.GUID == source.GUID {
			stats.SwingTimes = append(stats.SwingTimes, event.TimeMs)
		}
	}

	if stats.StartMs == 0 {
		return nil, fmt.Errorf("%s dealt no damage in %s", source.Name, log.Path)
	}
	for _, aura := range stats.Auras {
		aura.remove(stats.EndMs)
	}
	return stats, nil
}

func abilityKeyOf(source combatant, event chronicleEvent) abilityKey {
	key := abilityKey{SpellID: event.Spell.ID}
	if event.Src.GUID != source.GUID {
		key.Minion = event.Src.Name
	}
	return key
}

func (r *runStats) ability(source combatant, event chronicleEvent) *abilityStats {
	key := abilityKeyOf(source, event)

	ability, ok := r.Abilities[key]
	if !ok {
		name := event.Spell.Name
		if key.SpellID == 0 {
			name = "Melee"
		}
		if key.Minion != "" {
			name = key.Minion + ": " + name
		}
		ability = &abilityStats{Key: key, Name: name, Misses: map[string]int{}}
		r.Abilities[key] = ability
	}
	return ability
}

func (r *runStats) ticks(key abilityKey, event chronicleEvent) *tickSeries {
	target := targetKey{Ability: key, Dst: event.Dst.GUID}
	series, ok := r.TickTimes[target]
	if !ok {
		series = &tickSeries{Key: target, TargetName: event.Dst.Name}
		r.TickTimes[target] = series
	}
	return series
}

func (r *runStats) aura(source combatant, event chronicleEvent) *auraStats {
	key := targetKey{Ability: abilityKeyOf(source, event), Dst: event.Dst.GUID}
	aura, ok := r.Auras[key]
	if !ok {
		aura = &auraStats{SpellID: event.Spell.ID, Name: event.Spell.Name, TargetName: event.Dst.Name}
		r.Auras[key] = aura
	}
	return aura
}

// apply counts a refresh as another application: Chronicle re-emits APPLIED when
// an aura is refreshed, and the log carries no stack count.
func (a *auraStats) apply(timeMs int64) {
	a.Applied++
	if !a.open {
		a.open, a.openedAt = true, timeMs
	}
}

func (a *auraStats) remove(timeMs int64) {
	if !a.open {
		return
	}
	a.UptimeMs += timeMs - a.openedAt
	a.open = false
}

// pickSource finds the player to measure. Without a name it only works when one
// player dealt damage, which is what a recorded dummy run looks like.
func pickSource(log *chronicleLog, name string) (combatant, error) {
	seen := map[string]combatant{}
	var order []string
	for _, event := range log.Events {
		if !event.isDamage() || !event.Src.isPlayer() {
			continue
		}
		if _, ok := seen[event.Src.GUID]; !ok {
			seen[event.Src.GUID] = event.Src
			order = append(order, event.Src.GUID)
		}
	}

	if name != "" {
		for _, guid := range order {
			if strings.EqualFold(seen[guid].Name, name) {
				return seen[guid], nil
			}
		}
		// A player whose whole damage comes from a pet never shows up as a source.
		for _, event := range log.Events {
			if event.Src.isPlayer() && strings.EqualFold(event.Src.Name, name) {
				return event.Src, nil
			}
		}
		return combatant{}, fmt.Errorf("no player named %q dealt damage in %s", name, log.Path)
	}

	switch len(order) {
	case 0:
		return combatant{}, fmt.Errorf("%s has no player damage", log.Path)
	case 1:
		return seen[order[0]], nil
	default:
		var names []string
		for _, guid := range order {
			names = append(names, seen[guid].Name)
		}
		return combatant{}, fmt.Errorf("%s has %d players (%s); pass -source",
			log.Path, len(order), strings.Join(names, ", "))
	}
}

// ownerOf walks the owner chain to the player at the top, so a pet's own guardian
// still counts for the player.
func ownerOf(log *chronicleLog, guid string) string {
	for i := 0; i < 8; i++ {
		owner, ok := log.Owners[guid]
		if !ok {
			return guid
		}
		guid = owner
	}
	return guid
}

// reportChronicle prints the run and returns how many comparisons failed.
func reportChronicle(out io.Writer, log *chronicleLog, stats *runStats, opts chronicleOptions) int {
	zone := log.Zone
	if zone == "" {
		zone = "unknown zone"
	}
	fmt.Fprintf(out, "%s: %s (map %d, instance %d), %d events\n",
		log.Path, zone, log.MapID, log.Instance, len(log.Events))

	who := stats.Source.Name
	if len(stats.MinionNames) > 0 {
		who += " + " + strings.Join(stats.MinionNames, ", ")
	}
	fmt.Fprintf(out, "%s: %.1f s (%.1f s active), %d damage, %.1f DPS (%.1f active)\n",
		who, stats.durationSec(), stats.activeSec(), stats.Damage, stats.dps(), stats.activeDPS())

	printAbilities(out, stats, opts.Top)
	if opts.Verbose {
		printSwings(out, stats)
		printTicks(out, stats)
		printAuras(out, stats)
	}
	return printDPSGap(out, stats, opts)
}

func printAbilities(out io.Writer, stats *runStats, top int) {
	abilities := make([]*abilityStats, 0, len(stats.Abilities))
	for _, ability := range stats.Abilities {
		abilities = append(abilities, ability)
	}
	sort.Slice(abilities, func(i, j int) bool {
		if abilities[i].Damage != abilities[j].Damage {
			return abilities[i].Damage > abilities[j].Damage
		}
		return abilities[i].Name < abilities[j].Name
	})
	if top > 0 && len(abilities) > top {
		abilities = abilities[:top]
	}

	fmt.Fprintf(out, "\n%-34s %7s %6s %6s %7s %12s %7s\n",
		"ability", "hits", "ticks", "crit%", "miss%", "damage", "share")
	for _, ability := range abilities {
		name := ability.Name
		if ability.Key.SpellID != 0 {
			name = fmt.Sprintf("%s (%d)", name, ability.Key.SpellID)
		}
		landed := ability.Hits + ability.Ticks
		fmt.Fprintf(out, "%-34s %7d %6d %5.1f%% %6.1f%% %12d %6.1f%%\n",
			trim(name, 34), ability.Hits, ability.Ticks,
			100*ratio(ability.Crits, landed), 100*ratio(ability.missCount(), ability.attempts()),
			ability.Damage, 100*ratio64(ability.Damage, stats.Damage))
	}

	var misses, swings []string
	for _, ability := range abilities {
		for kind, count := range ability.Misses {
			misses = append(misses, fmt.Sprintf("%s %s %d", trim(ability.Name, 20), strings.ToLower(kind), count))
		}
		// The glancing rate is a parity number of its own: 25% against a +3 boss.
		for _, outcome := range []struct {
			name  string
			count int
		}{{"glancing", ability.Glancing}, {"crushing", ability.Crushing}} {
			if outcome.count > 0 {
				swings = append(swings, fmt.Sprintf("%s %s %d (%.1f%%)", trim(ability.Name, 20), outcome.name,
					outcome.count, 100*ratio(outcome.count, ability.attempts())))
			}
		}
	}
	for _, line := range []struct {
		label string
		parts []string
	}{{"misses", misses}, {"outcomes", swings}} {
		if len(line.parts) > 0 {
			sort.Strings(line.parts)
			fmt.Fprintf(out, "%s: %s\n", line.label, strings.Join(line.parts, ", "))
		}
	}
}

// printSwings shows what the 100 ms map update does to the swing timer. Chronicle
// puts no hand on SWING_DAMAGE, so a dual wielder's two hands interleave here.
func printSwings(out io.Writer, stats *runStats) {
	gaps := intervals(stats.SwingTimes)
	if len(gaps) == 0 {
		return
	}
	fmt.Fprintf(out, "\nswings: %d, intervals min %.2f s, median %.2f s, mean %.2f s\n",
		len(stats.SwingTimes), float64(gaps[0])/1000, float64(median(gaps))/1000, meanMs(gaps)/1000)
	fmt.Fprintf(out, "  %s\n", histogram(gaps, 100))
}

func printTicks(out io.Writer, stats *runStats) {
	series := make([]*tickSeries, 0, len(stats.TickTimes))
	for _, one := range stats.TickTimes {
		series = append(series, one)
	}
	sort.Slice(series, func(i, j int) bool {
		nameI, nameJ := stats.Abilities[series[i].Key.Ability].Name, stats.Abilities[series[j].Key.Ability].Name
		if nameI != nameJ {
			return nameI < nameJ
		}
		return series[i].TargetName < series[j].TargetName
	})

	for _, one := range series {
		gaps := intervals(one.Times)
		if len(gaps) == 0 {
			continue
		}
		fmt.Fprintf(out, "\nticks %s (%d) on %s: %d, median %.2f s\n",
			stats.Abilities[one.Key.Ability].Name, one.Key.Ability.SpellID, one.TargetName,
			len(one.Times), float64(median(gaps))/1000)
		fmt.Fprintf(out, "  %s\n", histogram(gaps, 100))
	}
}

func printAuras(out io.Writer, stats *runStats) {
	auras := make([]*auraStats, 0, len(stats.Auras))
	for _, aura := range stats.Auras {
		auras = append(auras, aura)
	}
	sort.Slice(auras, func(i, j int) bool {
		if auras[i].UptimeMs != auras[j].UptimeMs {
			return auras[i].UptimeMs > auras[j].UptimeMs
		}
		if auras[i].SpellID != auras[j].SpellID {
			return auras[i].SpellID < auras[j].SpellID
		}
		return auras[i].TargetName < auras[j].TargetName
	})
	if len(auras) == 0 {
		return
	}

	window := float64(stats.EndMs - stats.StartMs)
	fmt.Fprintf(out, "\n%-34s %-22s %8s %10s %8s\n", "aura", "on", "applied", "uptime", "of fight")
	for _, aura := range auras {
		fmt.Fprintf(out, "%-34s %-22s %8d %9.1fs %7.1f%%\n",
			trim(fmt.Sprintf("%s (%d)", aura.Name, aura.SpellID), 34), trim(aura.TargetName, 22),
			aura.Applied, float64(aura.UptimeMs)/1000, 100*float64(aura.UptimeMs)/max(window, 1))
	}
}

func printDPSGap(out io.Writer, stats *runStats, opts chronicleOptions) int {
	if opts.SimDPS <= 0 {
		fmt.Fprintf(out, "\nno -sim DPS given, nothing to compare\n")
		return 0
	}

	gap := 100 * (stats.dps() - opts.SimDPS) / opts.SimDPS
	status := "PASS"
	failed := 0
	if gap > maxDPSGapPct || gap < -maxDPSGapPct {
		status, failed = "FAIL", 1
	}
	fmt.Fprintf(out, "\n%s recorded %.1f DPS, sim %.1f, gap %+.2f%% (allowed %g%%)\n",
		status, stats.dps(), opts.SimDPS, gap, maxDPSGapPct)
	return failed
}

func intervals(times []int64) []int64 {
	if len(times) < 2 {
		return nil
	}
	gaps := make([]int64, 0, len(times)-1)
	for i := 1; i < len(times); i++ {
		gaps = append(gaps, times[i]-times[i-1])
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	return gaps
}

// histogram buckets sorted gaps by bucketMs, so the 100 ms lattice the server
// swings and ticks on shows up directly.
func histogram(sorted []int64, bucketMs int64) string {
	counts := map[int64]int{}
	for _, gap := range sorted {
		counts[gap/bucketMs*bucketMs]++
	}
	buckets := make([]int64, 0, len(counts))
	for bucket := range counts {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool { return buckets[i] < buckets[j] })

	parts := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		parts = append(parts, fmt.Sprintf("%.1fs:%d", float64(bucket)/1000, counts[bucket]))
	}
	return strings.Join(parts, " ")
}

func median(sorted []int64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[len(sorted)/2]
}

func meanMs(gaps []int64) float64 {
	if len(gaps) == 0 {
		return 0
	}
	var sum int64
	for _, gap := range gaps {
		sum += gap
	}
	return float64(sum) / float64(len(gaps))
}

func ratio(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}

func ratio64(part, whole int64) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}
