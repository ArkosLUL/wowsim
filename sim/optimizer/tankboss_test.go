package optimizer

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/encounters"
	goproto "google.golang.org/protobuf/proto"
)

// bossAbilityDamage is what each ability of the boss's fight dealt to the tank per cast and how often
// it was cast per second, over a sim of the phase's fight.
func bossAbilityDamage(tb testing.TB, player *proto.Player, phase int32, iterations int32) (perCast, castsPerSecond []float64) {
	tb.Helper()
	req := tankOptimizeRequest(tb, player, phase)
	rsr := goproto.Clone(req.Base).(*proto.RaidSimRequest)
	rsr.SimOptions = &proto.SimOptions{Iterations: iterations, RandomSeed: 7, IsTest: true}
	rsr.Encounter.Duration, rsr.Encounter.DurationVariation = 300, 0
	rsr.Raid.Parties[0].Players[0].HealingModel = nil
	result := core.RunRaidSim(rsr)
	if result.ErrorResult != "" {
		tb.Fatal(result.ErrorResult)
	}
	boss := tankBosses[phase]
	perCast = make([]float64, len(boss.Abilities))
	castsPerSecond = make([]float64, len(boss.Abilities))
	for i, ability := range boss.Abilities {
		for _, action := range result.GetEncounterMetrics().GetTargets()[0].GetActions() {
			if action.GetId().GetSpellId() != ability.SpellID || action.GetId().GetTag() != int32(i) {
				continue
			}
			var casts, damage float64
			for _, target := range action.GetTargets() {
				casts += float64(target.GetCasts())
				damage += target.GetDamage()
			}
			if casts > 0 {
				perCast[i] = damage / casts
			}
			castsPerSecond[i] = casts / float64(iterations) / 300
		}
	}
	return perCast, castsPerSecond
}

func bossSwapsTank(boss tankBoss) bool {
	return slices.ContainsFunc(boss.Abilities, func(a encounters.OptimizerTankAbility) bool { return a.SwapsTank })
}

// The boss casts what TANK_BOSSES lists, at the cadence it lists it, and every cast lands for no more
// than its damage, follow-up included. The tank's damage taken multipliers, resistance and Ardent
// Defender take part of it, which is why the lower bound is loose; armor takes most of a Physical one.
func TestTankBossCastsTheListedAbilities(t *testing.T) {
	for phase := int32(1); phase <= 5; phase++ {
		boss := tankBosses[phase]
		perCast, castsPerSecond := bossAbilityDamage(t, protPaladin(t, "p3"), phase, 200)
		for i, ability := range boss.Abilities {
			// a cast that lands holds the boss, so the rate is per attempt only for the share that lands
			wantRate := 1 / ((ability.PeriodMin + ability.PeriodMax) / 2)
			if ability.LandChance > 0 {
				wantRate *= ability.LandChance
			}
			if bossSwapsTank(boss) && !ability.Raidwide {
				// the modelled tank holds him every other interval, and takes every other cast aimed at his victim
				wantRate /= 2
			}
			t.Logf("phase %d %s (%d): %.0f per cast against %.0f, %.4f casts/s against %.4f", phase, boss.Name, ability.SpellID,
				perCast[i], ability.Damage+ability.FollowUpDamage, castsPerSecond[i], wantRate)
			damage, least := ability.Damage+ability.FollowUpDamage, 0.3
			if ability.School == proto.SpellSchool_SpellSchoolPhysical {
				least = 0.15
			}
			if perCast[i] < least*damage || perCast[i] > damage {
				t.Errorf("phase %d spell %d: the tank takes %.0f per cast, want about %.0f", phase, ability.SpellID, perCast[i], damage)
			}
			if math.Abs(castsPerSecond[i]-wantRate) > 0.15*wantRate {
				t.Errorf("phase %d spell %d: %.4f casts a second, want about %.4f", phase, ability.SpellID, castsPerSecond[i], wantRate)
			}
		}
	}
}

var (
	bossCastLanded = regexp.MustCompile(`^\[(\d+\.\d+)\] \[Target 1\] Completed cast \{SpellID: (\d+)(?:, Tag: (\d+))?\}`)
	bossSwing      = regexp.MustCompile(`^\[(\d+\.\d+)\] \[Target 1\] Completed cast \{OtherID: 3, Tag: 1\}`)
	bossTick       = regexp.MustCompile(`^\[(\d+\.\d+)\] \[Target 1\] .*\{SpellID: (\d+)(?:, Tag: (\d+))?\} tick `)
)

// What the boss did, off a sim's log. index is the ability's, for a cast and a tick.
type bossEvent struct {
	at    float64
	kind  bossEventKind
	index int
}

type bossEventKind int

const (
	bossEventSwing bossEventKind = iota
	bossEventCast
	bossEventTick
)

// bossFightLog is one 300 s fight of the phase's boss against the P3 paladin preset with no healing,
// as the boss's swings, casts and ticks in log order. The log stamps to 10 ms.
func bossFightLog(t *testing.T, phase int32) []bossEvent {
	t.Helper()
	req := tankOptimizeRequest(t, protPaladin(t, "p3"), phase)
	rsr := goproto.Clone(req.Base).(*proto.RaidSimRequest)
	rsr.SimOptions = &proto.SimOptions{Iterations: 1, RandomSeed: 7, IsTest: true, Debug: true}
	rsr.Encounter.Duration, rsr.Encounter.DurationVariation = 300, 0
	rsr.Raid.Parties[0].Players[0].HealingModel = nil
	result := core.RunRaidSim(rsr)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	var events []bossEvent
	swings := 0
	for _, line := range strings.Split(result.Logs, "\n") {
		kind, m := bossEventSwing, bossSwing.FindStringSubmatch(line)
		if m == nil {
			kind, m = bossEventCast, bossCastLanded.FindStringSubmatch(line)
		}
		if m == nil {
			kind, m = bossEventTick, bossTick.FindStringSubmatch(line)
		}
		if m == nil {
			continue
		}
		event := bossEvent{kind: kind}
		event.at, _ = strconv.ParseFloat(m[1], 64)
		if kind == bossEventSwing {
			swings++
		} else {
			event.index, _ = strconv.Atoi(m[3])
		}
		events = append(events, event)
	}
	if swings < 60 {
		t.Fatalf("phase %d: read %d boss swings off the log, want a fight's worth", phase, swings)
	}
	return events
}

// hastedBetween is the stretches in which a follow-up tick has the boss swinging faster.
func hastedBetween(boss tankBoss, events []bossEvent) [][2]float64 {
	var out [][2]float64
	for _, e := range events {
		if e.kind == bossEventTick {
			ability := boss.Abilities[e.index]
			out = append(out, [2]float64{e.at, e.at + ability.FollowUpHasteSeconds})
			if ability.SwapsTank {
				// the other tank's cast, a period on, hastes the boss the same way
				out = append(out, [2]float64{e.at + ability.PeriodMin, e.at + ability.PeriodMin + ability.FollowUpHasteSeconds})
			}
		}
	}
	return out
}

// A cast that restarts the boss's swing timer leaves no swing for a whole swing time after it goes
// off: an instant (Freezing Slash, Soul Reaper) the whole of it, a cast with a cast time (Infest) one
// map update less, as it lands ahead of that update's attack timers. The other casts leave the timer
// alone. Anub'arak and the Lich King have no parry haste to shorten it; a cast inside Soul Reaper's
// haste is left out, since the swing after it comes sooner.
func TestTankBossSwingRestartsAfterItsCasts(t *testing.T) {
	for _, phase := range []int32{3, 4} {
		boss := tankBosses[phase]
		swingTime := float64(boss.AttackTimeMs) / 1000
		if swingTime == 0 {
			swingTime = 2
		}
		events := bossFightLog(t, phase)
		hasted := hastedBetween(boss, events)

		resets, keeps, early := 0, 0, 0
	casts:
		for i, cast := range events {
			if cast.kind != bossEventCast {
				continue
			}
			for _, h := range hasted {
				if cast.at+swingTime > h[0] && cast.at < h[1] {
					continue casts
				}
			}
			next := math.Inf(1)
			for _, later := range events[i+1:] {
				if later.kind == bossEventSwing {
					next = later.at
					break
				}
			}
			if boss.Abilities[cast.index].SwapsTank {
				// the boss leaves for the other tank, see TestTankSwapsOnSoulReaper
				continue
			}
			whole := swingTime
			if boss.Abilities[cast.index].CastSeconds > 0 {
				whole -= 0.1027
			}
			soon := next-cast.at < whole-0.02
			if !boss.Abilities[cast.index].ResetsSwing {
				keeps++
				if soon {
					early++
				}
				continue
			}
			resets++
			if soon {
				t.Errorf("phase %d: %s swings %.2f s after spell %d went off at %.2f, want %.2f s",
					phase, boss.Name, next-cast.at, boss.Abilities[cast.index].SpellID, cast.at, whole)
			}
		}
		if resets < 5 {
			t.Errorf("phase %d: only %d casts that restart the swing in the log", phase, resets)
		}
		if keeps > 5 && early == 0 {
			t.Errorf("phase %d: none of the %d casts that leave the swing timer alone had a swing within %.1f s", phase, keeps, swingTime)
		}
	}
}

// usualSwingGap is the time between two of the boss's swings as the fight has it: the raid's attack
// slow and the map updates make it longer than the creature's own attack time. The median, so hasted
// stretches and casts that hold the melee don't count.
func usualSwingGap(events []bossEvent) float64 {
	var gaps []float64
	last := math.Inf(-1)
	for _, e := range events {
		if e.kind != bossEventSwing {
			continue
		}
		if !math.IsInf(last, -1) {
			gaps = append(gaps, e.at-last)
		}
		last = e.at
	}
	slices.Sort(gaps)
	return gaps[len(gaps)/2]
}

// Soul Reaper's aura ticks once, 5 s after the cast that left it, though the boss has gone to the other
// tank by then.
func TestTankBossFollowUpTicksOnce(t *testing.T) {
	const phase = 4
	boss := tankBosses[phase]
	events := bossFightLog(t, phase)
	followUps := 0
	for i, cast := range events {
		ability := boss.Abilities[cast.index]
		if cast.kind != bossEventCast || ability.FollowUpSeconds == 0 || cast.at+ability.FollowUpSeconds > 299 {
			continue
		}
		followUps++
		var ticks []float64
		for _, later := range events[i+1:] {
			if later.kind == bossEventCast && later.index == cast.index {
				break
			}
			if later.kind == bossEventTick && later.index == cast.index {
				ticks = append(ticks, later.at)
			}
		}
		// the tick waits for the tank's next world tick, and the long one takes up to 59 ms
		if tickAt := cast.at + ability.FollowUpSeconds; len(ticks) != 1 || math.Abs(ticks[0]-tickAt) > 0.07 {
			t.Errorf("spell %d cast at %.2f ticked at %v, want once at %.2f", ability.SpellID, cast.at, ticks, tickAt)
		}
	}
	if followUps < 3 {
		t.Errorf("%d casts with a follow-up in the log, want at least 3", followUps)
	}
}

// The tanks swap after each Soul Reaper. The modelled tank takes the first one and the boss leaves it
// until the next cast, which lands on the other tank. That cast's haste falls on the modelled tank's
// swings. Infest is cast on the boss itself and hits everyone around it, so it keeps landing on the modelled
// tank while the other tank holds the boss.
func TestTankSwapsOnSoulReaper(t *testing.T) {
	const phase = 4
	boss := tankBosses[phase]
	events := bossFightLog(t, phase)
	gap := usualSwingGap(events)
	ability := boss.Abilities[0]
	if !ability.SwapsTank {
		t.Fatal("the Lich King's first ability should swap the tank")
	}
	period := ability.PeriodMin

	swingsIn := func(from, to float64) (n int) {
		for _, e := range events {
			if e.kind == bossEventSwing && e.at > from && e.at <= to {
				n++
			}
		}
		return n
	}
	castIn := func(index int, from, to float64) bool {
		for _, e := range events {
			if e.kind == bossEventCast && e.index == index && e.at > from && e.at < to {
				return true
			}
		}
		return false
	}
	var reapers []float64
	for _, e := range events {
		if e.kind == bossEventCast && e.index == 0 {
			reapers = append(reapers, e.at)
		}
	}
	if len(reapers) < 4 || math.Abs(reapers[0]-period) > 0.1 || math.Abs(reapers[1]-reapers[0]-2*period) > 2.5 {
		t.Fatalf("Soul Reaper landed on the tank at %v, want every %.1f s from %.1f s", reapers, 2*period, period)
	}
	checked, awayInfests := 0, 0
	infests := 0
	for _, e := range events {
		if e.kind == bossEventCast && e.index == 1 {
			infests++
		}
	}
	// one every 22.5 s, whoever holds the boss
	if want := 300 / boss.Abilities[1].PeriodMin; float64(infests) < want-2 || float64(infests) > want+1 {
		t.Errorf("%d Infests landed on the tank in 300 s, want about %.0f", infests, want)
	}
	for _, cast := range reapers {
		if cast+period > 299 {
			continue
		}
		if n := swingsIn(cast+0.1, cast+period-0.1); n != 0 {
			t.Errorf("%d swings at the tank while the other tank holds the boss after the cast at %.2f", n, cast)
		}
		if castIn(1, cast+0.1, cast+period-0.1) {
			awayInfests++
		}
		// the other tank's Soul Reaper comes at cast + period, its haste 5 s later
		hasteFrom := cast + period + ability.FollowUpSeconds
		hasteTo := hasteFrom + ability.FollowUpHasteSeconds
		if hasteTo+ability.FollowUpHasteSeconds > 299 || castIn(1, hasteFrom-3*gap, hasteTo+ability.FollowUpHasteSeconds) {
			continue
		}
		checked++
		fast, after := swingsIn(hasteFrom, hasteTo), swingsIn(hasteTo, hasteTo+ability.FollowUpHasteSeconds)
		want := ability.FollowUpHasteSeconds / gap
		if float64(fast) < want*ability.FollowUpHaste-1 || float64(fast) > want*ability.FollowUpHaste+1 || float64(after) > want+1 {
			t.Errorf("after the cast at %.2f: %d swings in the %.0f s of haste and %d in the same time after, want about %.1f and %.1f",
				cast, fast, ability.FollowUpHasteSeconds, after, want*ability.FollowUpHaste, want)
		}
	}
	if awayInfests == 0 {
		t.Error("no Infest landed on the tank while the other tank held the boss")
	}
	if checked < 1 {
		t.Errorf("only %d hasted stretches without an Infest cast inside to check", checked)
	}
	if n := swingsIn(0, period); n < int(period/gap)-2 {
		t.Errorf("%d swings in the first %.1f s, want the tank holding the boss first", n, period)
	}
}

// A spell that rolls the server's magic hit misses a level 80 tank 1% of the time, one that doesn't
// never does.
func TestTankBossSpellsMissAsTheServerRolls(t *testing.T) {
	for _, tc := range []struct {
		phase int32
		index int
	}{{2, 0}, {2, 1}, {4, 0}} {
		boss := tankBosses[tc.phase]
		req := tankOptimizeRequest(t, protPaladin(t, "p3"), tc.phase)
		rsr := goproto.Clone(req.Base).(*proto.RaidSimRequest)
		rsr.SimOptions = &proto.SimOptions{Iterations: 1000, RandomSeed: 11, IsTest: true}
		rsr.Encounter.Duration, rsr.Encounter.DurationVariation = 300, 0
		rsr.Raid.Parties[0].Players[0].HealingModel = nil
		result := core.RunRaidSim(rsr)
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		ability := boss.Abilities[tc.index]
		var casts, misses float64
		for _, action := range result.GetEncounterMetrics().GetTargets()[0].GetActions() {
			if action.GetId().GetSpellId() == ability.SpellID && action.GetId().GetTag() == int32(tc.index) {
				for _, target := range action.GetTargets() {
					casts += float64(target.GetCasts())
					misses += float64(target.GetMisses())
				}
			}
		}
		t.Logf("%s %d: %.0f casts, %.0f missed (%.2f%%)", boss.Name, ability.SpellID, casts, misses, misses/casts*100)
		if casts < 3000 {
			t.Fatalf("spell %d: %.0f casts, want a sample", ability.SpellID, casts)
		}
		if rate := misses / casts; ability.CanMiss && (rate < 0.007 || rate > 0.013) || !ability.CanMiss && misses > 0 {
			t.Errorf("spell %d (can miss %v) missed %.2f%% of %.0f casts, want 1%% or none", ability.SpellID, ability.CanMiss, rate*100, casts)
		}
	}
}

// bossSwingTable is the boss's main hand swing table against the tank, in basis points.
func bossSwingTable(t *testing.T, boss encounters.OptimizerTankBoss) core.MeleeTableBP {
	t.Helper()
	req := tankOptimizeRequest(t, protPaladin(t, "p3"), 5)
	req.Base.Encounter.Targets = []*proto.Target{encounters.OptimizerTankBossTarget(boss)}
	env, _, _ := core.NewEnvironment(req.Base.Raid, req.Base.Encounter, false)
	target := env.Encounter.Targets[0]
	tank := env.Raid.AllPlayerUnits[0]
	swing := target.AutoAttacks.MHAuto()
	if swing == nil || target.CurrentTarget != tank {
		t.Fatal("the boss doesn't swing at the tank")
	}
	return core.WhiteMeleeTableBP(swing.WhiteTableInput(target.AttackTables[tank.UnitIndex]))
}

// Halion's Twilight Precision (78243) takes 5 points off his miss chance and a fifth off the tank's
// dodge, before the skill term, as Unit::RollMeleeOutcomeAgainst does. Parry and block stay.
func TestHalionsTwilightPrecision(t *testing.T) {
	halion := tankBosses[5].OptimizerTankBoss
	if halion.HitChance != 5 || halion.EnemyDodgeMultiplier != 0.8 {
		t.Fatalf("Halion has %v hit and a dodge multiplier of %v, want 5 and 0.8", halion.HitChance, halion.EnemyDodgeMultiplier)
	}
	with := bossSwingTable(t, halion)
	plain := halion
	plain.HitChance, plain.EnemyDodgeMultiplier = 0, 0
	without := bossSwingTable(t, plain)
	t.Logf("with %+v, without %+v", with, without)

	// a basis point of slack: the two miss chances round on their own
	if without.Miss < 600 || with.Miss < without.Miss-501 || with.Miss > without.Miss-499 {
		t.Errorf("miss %d bp with the aura and %d without, want 500 less", with.Miss, without.Miss)
	}
	// the skill bonus comes off after the multiplier: 4 bp for each of the 15 points the boss has over the tank
	const skillBonus = 60
	wantDodge := int32(float32(without.Dodge+skillBonus)*0.8) - skillBonus
	if without.Dodge < 1000 || with.Dodge != wantDodge {
		t.Errorf("dodge %d bp with the aura and %d without, want %d", with.Dodge, without.Dodge, wantDodge)
	}
	if with.Parry != without.Parry || with.Block != without.Block || with.Crit != without.Crit {
		t.Errorf("the aura moved more than miss and dodge: %+v against %+v", with, without)
	}
}

// The pinned HPS follows the gear the tank wears, not the trimmed seed: a seed missing slots takes
// far more damage, and its 1.5x would leave the worn gear nothing to fear.
func TestPinnedHealingFollowsTheEquippedGear(t *testing.T) {
	pin := func(trim bool) float64 {
		req := tankOptimizeRequest(t, protPaladin(t, "p3"), 3)
		req.Base.Raid.Parties[0].Players[0].HealingModel = &proto.HealingModel{CadenceSeconds: 2.5, BurstWindow: 6}
		if trim {
			req.Equipped = goproto.Clone(req.Base.Raid.Parties[0].Players[0].Equipment).(*proto.EquipmentSpec)
			items := req.Base.Raid.Parties[0].Players[0].Equipment.Items
			for _, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotHead, proto.ItemSlot_ItemSlotChest, proto.ItemSlot_ItemSlotLegs, proto.ItemSlot_ItemSlotMainHand} {
				items[slot] = &proto.ItemSpec{}
			}
		}
		r, err := PrepareRequest(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := (&run{r: r}).pinHealingModel(); err != nil {
			t.Fatal(err)
		}
		return r.Target().GetHealingModel().GetHps()
	}
	whole, trimmed := pin(false), pin(true)
	if whole == 0 || math.Abs(whole-trimmed) > 1e-6 {
		t.Errorf("pinned %.1f HPS with a trimmed seed and the equipped gear given, want the whole gear's %.1f", trimmed, whole)
	}
}
