package optimizer

import (
	"math"
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
