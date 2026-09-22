package main

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// outcomeStats is one outcome's damage events, kept as sums so the mean comes with its standard error.
type outcomeStats struct {
	N          int64
	Sum, SumSq float64
}

func (o *outcomeStats) add(amount float64) {
	o.N++
	o.Sum += amount
	o.SumSq += amount * amount
}

func (o outcomeStats) mean() float64 {
	if o.N == 0 {
		return 0
	}
	return o.Sum / float64(o.N)
}

// stdErr is the mean's standard error, off the sample standard deviation.
func (o outcomeStats) stdErr() float64 {
	if o.N < 2 {
		return 0
	}
	n := float64(o.N)
	variance := (o.SumSq - o.Sum*o.Sum/n) / (n - 1)
	return math.Sqrt(max(variance, 0) / n)
}

// outcomeBreakdown splits one ability's damage by outcome, the same way for a capture and for the sim:
// a hit's size doesn't depend on how often it's cast, and a crit rate mixed into an average does.
type outcomeBreakdown struct {
	Hit, Crit, Glance outcomeStats
	Blocked           int64            // partial blocks, also counted in Hit or Crit
	Avoided           map[string]int64 // miss, dodge, parry, full block, resist
}

func (b *outcomeBreakdown) addDamage(amount float64, crit, glance, blocked bool) {
	switch {
	case crit:
		b.Crit.add(amount)
	case glance:
		b.Glance.add(amount)
	default:
		b.Hit.add(amount)
	}
	if blocked {
		b.Blocked++
	}
}

func (b *outcomeBreakdown) avoid(kind string) {
	if b.Avoided == nil {
		b.Avoided = map[string]int64{}
	}
	b.Avoided[kind]++
}

func (b *outcomeBreakdown) landed() int64 { return b.Hit.N + b.Crit.N + b.Glance.N }

func (b *outcomeBreakdown) damage() float64 { return b.Hit.Sum + b.Crit.Sum + b.Glance.Sum }

// critRate is the share of landed events that crit, glances included, with its binomial standard error.
func (b *outcomeBreakdown) critRate() (rate, stdErr float64) {
	n := float64(b.landed())
	if n == 0 {
		return 0, 0
	}
	rate = float64(b.Crit.N) / n
	return rate, math.Sqrt(rate * (1 - rate) / n)
}

type outcomeRow struct {
	Name     string
	Outcomes *outcomeBreakdown
}

// printOutcomes prints each ability's non-crit, crit and glance averages and its crit rate, each with one
// standard error. Counts are per iteration: pass 1 for a capture. A row that only ever missed still
// prints, like the sim's Holy Vengeance application roll, whose parries the capture counts too.
func printOutcomes(out io.Writer, rows []outcomeRow, iterations float64) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Outcomes.damage() > rows[j].Outcomes.damage()
	})
	fmt.Fprintf(out, "\n%s %8s %12s %8s %12s %8s %12s %12s  %s\n", padRight("outcomes (mean ± std error)", 34),
		"hits", "hit avg", "crits", "crit avg", "glances", "glance avg", "crit% landed", "other")
	for _, row := range rows {
		b := row.Outcomes
		if b.landed() == 0 && len(b.Avoided) == 0 {
			continue
		}
		critColumn := "-"
		if b.landed() > 0 {
			rate, rateErr := b.critRate()
			critColumn = fmt.Sprintf("%.1f±%.1f", 100*rate, 100*rateErr)
		}
		var other []string
		if b.Blocked > 0 {
			other = append(other, "blocked "+formatCount(b.Blocked, iterations))
		}
		kinds := make([]string, 0, len(b.Avoided))
		for kind := range b.Avoided {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			other = append(other, kind+" "+formatCount(b.Avoided[kind], iterations))
		}
		fmt.Fprintf(out, "%-34s %8s %s %8s %s %8s %s %s  %s\n", trim(row.Name, 34),
			formatCount(b.Hit.N, iterations), padLeft(formatMean(b.Hit), 12),
			formatCount(b.Crit.N, iterations), padLeft(formatMean(b.Crit), 12),
			formatCount(b.Glance.N, iterations), padLeft(formatMean(b.Glance), 12),
			padLeft(critColumn, 12), strings.Join(other, ", "))
	}
}

// padLeft and padRight count runes, not bytes, since ± takes two.
func padLeft(text string, width int) string {
	return strings.Repeat(" ", max(width-utf8.RuneCountInString(text), 0)) + text
}

func padRight(text string, width int) string {
	return text + strings.Repeat(" ", max(width-utf8.RuneCountInString(text), 0))
}

func formatCount(n int64, iterations float64) string {
	if iterations == 1 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1f", float64(n)/iterations)
}

func formatMean(o outcomeStats) string {
	if o.N == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f±%.0f", o.mean(), o.stdErr())
}

type outcomeKey struct {
	Pet    string // empty for the player
	Action core.ActionID
}

// tallyOutcomes reruns the request with a listener on the player and its pets, since the sim's metrics
// count crits and glances but don't split damage by them. It seeds each iteration as RunRaidSim does, so
// it sees the same fights, and returns the DPS it got to check that against RunRaidSim's.
func tallyOutcomes(request *proto.RaidSimRequest) (map[outcomeKey]*outcomeBreakdown, float64) {
	sim := core.NewSim(request)
	character := sim.Raid.Parties[0].Players[0].GetCharacter()
	tally := map[outcomeKey]*outcomeBreakdown{}

	listen := func(unit *core.Unit, pet string) {
		record := func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			// metrics skip the prepull too
			if sim.CurrentTime < 0 || result.Target.Type != core.EnemyUnit {
				return
			}
			// an aura or debuff landing, not a damage event
			if result.Landed() && result.Damage <= 0 {
				return
			}
			key := outcomeKey{Pet: pet, Action: spell.ActionID}
			b, ok := tally[key]
			if !ok {
				b = &outcomeBreakdown{}
				tally[key] = b
			}
			if !result.Landed() {
				b.avoid(simAvoidance(result.Outcome))
				return
			}
			b.addDamage(result.Damage, result.DidCrit(), result.Outcome.Matches(core.OutcomeGlance),
				result.Outcome.Matches(core.OutcomeBlock))
		}
		unit.RegisterAura(core.Aura{
			Label:    "rrsim outcomes",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt:       record,
			OnPeriodicDamageDealt: record,
		})
	}
	listen(&character.Unit, "")
	for _, pet := range character.Pets {
		listen(&pet.Unit, pet.Name)
	}

	for i := int32(0); i < request.SimOptions.Iterations; i++ {
		// the first iteration runs on the seed NewSim started from
		if i > 0 {
			sim.Reseed(int64(i))
		}
		sim.Reset()
		sim.PrePull()
		for !sim.Step() {
		}
		sim.Cleanup()
	}
	return tally, sim.Raid.GetMetrics().Parties[0].Players[0].Dps.Avg
}

func simAvoidance(outcome core.HitOutcome) string {
	switch {
	case outcome.Matches(core.OutcomeDodge):
		return "dodge"
	case outcome.Matches(core.OutcomeParry):
		return "parry"
	case outcome.Matches(core.OutcomeBlock):
		return "block"
	}
	return "miss"
}

func outcomeRows(tally map[outcomeKey]*outcomeBreakdown) []outcomeRow {
	rows := make([]outcomeRow, 0, len(tally))
	for key, b := range tally {
		name := actionLabel(key.Action)
		if key.Pet != "" {
			name = key.Pet + ": " + name
		}
		rows = append(rows, outcomeRow{Name: name, Outcomes: b})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// actionLabel names an action the way chronicle's rows end, by spell id, so the two line up.
func actionLabel(id core.ActionID) string {
	var label string
	switch {
	case id.OtherID == proto.OtherAction_OtherActionAttack:
		label = "Melee"
	case id.OtherID == proto.OtherAction_OtherActionShoot:
		label = "Auto Shot"
	case id.SpellID != 0:
		label = fmt.Sprintf("(%d)", id.SpellID)
	case id.ItemID != 0:
		label = fmt.Sprintf("item %d", id.ItemID)
	default:
		label = id.String()
	}
	if id.Tag != 0 {
		label += fmt.Sprintf(" tag %d", id.Tag)
	}
	return label
}
