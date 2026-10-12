package optimizer

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
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

// The boss casts what TANK_BOSSES lists, at the cadence it lists it, and every cast lands for no more
// than its damage. The tank's damage taken multipliers, resistance and Ardent Defender take part of it,
// which is why the lower bound is loose.
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
			t.Logf("phase %d %s (%d): %.0f per cast against %.0f, %.4f casts/s against %.4f", phase, boss.Name, ability.SpellID,
				perCast[i], ability.Damage, castsPerSecond[i], wantRate)
			if perCast[i] < 0.3*ability.Damage || perCast[i] > ability.Damage {
				t.Errorf("phase %d spell %d: the tank takes %.0f per cast, want about %.0f", phase, ability.SpellID, perCast[i], ability.Damage)
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
)

// A cast that restarts the boss's swing timer leaves no swing for a whole swing time after it goes
// off: an instant (Freezing Slash, Soul Reaper) the whole of it, a cast with a cast time (Infest) one
// map update less, as it lands ahead of that update's attack timers. The other casts leave the timer
// alone. Anub'arak and the Lich King have no parry haste to shorten it.
func TestTankBossSwingRestartsAfterItsCasts(t *testing.T) {
	for _, phase := range []int32{3, 4} {
		boss := tankBosses[phase]
		swingTime := float64(boss.AttackTimeMs) / 1000
		if swingTime == 0 {
			swingTime = 2
		}
		req := tankOptimizeRequest(t, protPaladin(t, "p3"), phase)
		rsr := goproto.Clone(req.Base).(*proto.RaidSimRequest)
		rsr.SimOptions = &proto.SimOptions{Iterations: 1, RandomSeed: 7, IsTest: true, Debug: true}
		rsr.Encounter.Duration, rsr.Encounter.DurationVariation = 300, 0
		rsr.Raid.Parties[0].Players[0].HealingModel = nil
		result := core.RunRaidSim(rsr)
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}

		// in log order; index is the ability's, -1 for a swing
		type event struct {
			at    float64
			index int
		}
		var events []event
		swings := 0
		for _, line := range strings.Split(result.Logs, "\n") {
			if m := bossSwing.FindStringSubmatch(line); m != nil {
				at, _ := strconv.ParseFloat(m[1], 64)
				events = append(events, event{at, -1})
				swings++
			} else if m := bossCastLanded.FindStringSubmatch(line); m != nil {
				at, _ := strconv.ParseFloat(m[1], 64)
				index, _ := strconv.Atoi(m[3])
				events = append(events, event{at, index})
			}
		}
		if swings < 100 {
			t.Fatalf("phase %d: read %d boss swings off the log, want a fight's worth", phase, swings)
		}

		resets, keeps, early := 0, 0, 0
		for i, cast := range events {
			if cast.index < 0 {
				continue
			}
			next := math.Inf(1)
			for _, later := range events[i+1:] {
				if later.index < 0 {
					next = later.at
					break
				}
			}
			// the log stamps to 10 ms
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
