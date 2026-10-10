package warlock

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

func doomSettings(tweaks *proto.SpellTweaksSettings) *proto.ServerSettings {
	// exact timing, so the hit lands on the ms the curse runs out
	return &proto.ServerSettings{
		MapUpdateIntervalMs:   googleProto.Int32(0),
		WorldUpdateIntervalMs: googleProto.Int32(0),
		SpellTweaks:           tweaks,
	}
}

var (
	doomScalingOn     = doomSettings(nil)
	doomScalingOff    = doomSettings(&proto.SpellTweaksSettings{CurseOfDoomScaling: googleProto.Bool(false)})
	doomSpellTweakOff = doomSettings(&proto.SpellTweaksSettings{Enable: googleProto.Bool(false)})
)

// doomSim has a warlock with no gear or weapon stone, so the cast speed is the test's to set, and a level
// 80 target, which resists nothing. Curse of Doom never misses.
func doomSim(t *testing.T, talents string, settings *proto.ServerSettings) (*core.Simulation, *Warlock, *core.Unit) {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Targets = []*proto.Target{{Level: 80, MobType: proto.MobType_MobTypeDemon}}
	encounter.ServerSettings = settings
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceOrc,
			Class:     proto.Class_ClassWarlock,
			Equipment: &proto.EquipmentSpec{},
			Consumes:  &proto.Consumes{},
			Spec: &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.Warlock_Options{
				Armor:  proto.Warlock_Options_FelArmor,
				Summon: proto.Warlock_Options_Imp,
			}}},
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: talents,
		}, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	warlock := sim.Raid.Parties[0].Players[0].(*Warlock)
	if warlock.CastSpeed != 1 {
		t.Fatalf("a warlock with no gear casts at x%v, want x1", warlock.CastSpeed)
	}
	warlock.CurseOfDoom.BonusHitRating += 100 * core.SpellHitRatingPerHitChance
	return sim, warlock, sim.GetTargetUnit(0)
}

// spell_tweaks_doom_scaling: with Demonic Pact the curse's duration, its one tick and its cooldown are each
// max(int32(60000 x cast speed), 1000) ms, read at the cast, and a slow changes nothing. A recast is refused
// while the warlock's own curse is on the target. SpellTweaks.CurseOfDoomScaling and SpellTweaks.Enable gate
// all of it, the tick crit too, which rides the tweak's aura.
func TestCurseOfDoomScaling(t *testing.T) {
	const ms = time.Millisecond
	for _, c := range []struct {
		name     string
		talents  string
		settings *proto.ServerSettings
		haste    float64
		length   time.Duration
		scaling  bool
	}{
		{"no haste", DemonologyTalents, doomScalingOn, 1, 60000 * ms, true},
		{"30% haste", DemonologyTalents, doomScalingOn, 1.3, 46153 * ms, true},
		{"50% haste", DemonologyTalents, doomScalingOn, 1.5, 40000 * ms, true},
		{"slowed", DemonologyTalents, doomScalingOn, 0.8, 60000 * ms, true},
		{"never under a second", DemonologyTalents, doomScalingOn, 100, 1000 * ms, true},
		{"switch off", DemonologyTalents, doomScalingOff, 1.3, 60000 * ms, false},
		{"spell tweaks off", DemonologyTalents, doomSpellTweakOff, 1.3, 60000 * ms, false},
		{"no Demonic Pact", DestructionTalents, doomScalingOn, 1.3, 60000 * ms, false},
	} {
		sim, warlock, target := doomSim(t, c.talents, c.settings)
		if c.talents == DemonologyTalents && warlock.Talents.DemonicPact == 0 {
			t.Fatal("DemonologyTalents has no Demonic Pact")
		}
		doom := warlock.CurseOfDoom
		dot := doom.Dot(target)
		warlock.MultiplyCastSpeed(c.haste)
		cast := sim.CurrentTime
		if !doom.Cast(sim, target) || !dot.IsActive() {
			t.Fatalf("%s: Curse of Doom didn't land", c.name)
		}
		if dot.Aura.Duration != c.length || dot.TickPeriod() != c.length || doom.CD.ReadyAt() != cast+c.length {
			t.Errorf("%s: curse of %v, tick after %v, cooldown of %v, want %v each",
				c.name, dot.Aura.Duration, dot.TickPeriod(), doom.CD.ReadyAt()-cast, c.length)
		}
		if dot.TicksCanCrit != c.scaling {
			t.Errorf("%s: the hit can crit %v, want %v", c.name, dot.TicksCanCrit, c.scaling)
		}

		// haste that comes after the cast changes nothing
		warlock.MultiplyCastSpeed(2)
		if dot.TickPeriod() != c.length || doom.CD.ReadyAt() != cast+c.length {
			t.Errorf("%s: a haste gain moved the running curse or its cooldown", c.name)
		}

		// stock lets a recast wipe the curse; the tweak refuses it, cooldown or not
		doom.CD.Reset()
		warlock.GCD.Reset()
		if got := doom.CanCast(sim, target); got == c.scaling {
			t.Errorf("%s: with the curse up and the cooldown ready, a recast is allowed %v", c.name, got)
		}
	}
}

// The hit lands as the hasted curse runs out, with the cooldown ready for the next one. With the tweak it
// can crit, for 1.5 times: no crit talent lists Curse of Doom.
func TestCurseOfDoomHit(t *testing.T) {
	hit := func(settings *proto.ServerSettings) (damage float64, crit bool, after time.Duration) {
		sim, warlock, target := doomSim(t, DemonologyTalents, settings)
		doom := warlock.CurseOfDoom
		doom.BonusCritRating += 1000 * core.CritRatingPerCritChance
		warlock.MultiplyCastSpeed(1.3)
		cast := sim.CurrentTime
		doom.Cast(sim, target)
		metrics := &doom.SpellMetrics[target.UnitIndex]
		for metrics.TotalDamage == 0 && !sim.Step() {
		}
		after = sim.CurrentTime - cast
		for doom.Dot(target).IsActive() && !sim.Step() {
		}
		// the idle rotation parks the GCD timer
		warlock.GCD.Reset()
		if sim.CurrentTime-cast != after || !doom.CanCast(sim, target) {
			t.Errorf("the curse can't be recast as its hit lands, %v after the cast", after)
		}
		if metrics.Hits+metrics.Crits != 1 {
			t.Errorf("%d hits and %d crits from one curse, want one of them", metrics.Hits, metrics.Crits)
		}
		return metrics.TotalDamage, metrics.Crits == 1, after
	}

	stock, stockCrit, stockAfter := hit(doomScalingOff)
	if stock == 0 || stockCrit || stockAfter != time.Minute {
		t.Errorf("stock: %v damage, crit %v, %v after the cast, want a plain hit after 1m0s", stock, stockCrit, stockAfter)
	}
	scaled, scaledCrit, scaledAfter := hit(doomScalingOn)
	if !scaledCrit || scaledAfter != 46153*time.Millisecond {
		t.Errorf("tweak: crit %v, %v after the cast, want a crit after 46.153s", scaledCrit, scaledAfter)
	}
	if math.Abs(scaled-1.5*stock) > 1 {
		t.Errorf("tweak: the crit deals %v, want 1.5 times the %v hit", scaled, stock)
	}
}

// The Demonology rotation recasts the hasted curse as it lands, and never over one still on the target.
func TestDemonologyRotationRecastsCurseOfDoom(t *testing.T) {
	const doomCast = "Completed cast {SpellID: 47867}"
	run := func(settings *proto.ServerSettings, seed int64) (casts, hits []time.Duration) {
		encounter := core.MakeSingleTargetEncounter(0)
		encounter.Duration = 300
		encounter.ServerSettings = settings
		res := core.RunRaidSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassWarlock,
				Equipment:     core.GetGearSet("../../ui/warlock/gear_sets", "p4_demo").GearSet,
				Consumes:      FullConsumes,
				Spec:          DefaultDemonologyWarlock,
				Buffs:         core.FullIndividualBuffs,
				Rotation:      core.GetAplRotation("../../ui/warlock/apls", "demo").Rotation,
				TalentsString: DemonologyTalents,
				Glyphs:        DemonologyGlyphs,
			}, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
			Encounter:  encounter,
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, Debug: true, RandomSeed: seed},
		})
		if res.ErrorResult != "" {
			t.Fatal(res.ErrorResult)
		}
		landed := map[time.Duration]bool{}
		for _, line := range strings.Split(res.Logs, "\n") {
			end := strings.Index(line, "]")
			if !strings.HasPrefix(line, "[") || end < 0 {
				continue
			}
			secs, err := strconv.ParseFloat(line[1:end], 64)
			if err != nil {
				continue
			}
			at := time.Duration(math.Round(secs*1000)) * time.Millisecond
			switch {
			case strings.Contains(line, doomCast):
				casts = append(casts, at)
			case strings.Contains(line, "Aura gained: {SpellID: 47867}"):
				landed[at] = true
			case strings.Contains(line, "{SpellID: 47867} tick"):
				hits = append(hits, at)
			}
		}
		for i, cast := range casts {
			if !landed[cast] || i == len(casts)-1 {
				continue
			}
			hit := false
			for _, at := range hits {
				hit = hit || (at > cast && at <= casts[i+1])
			}
			if !hit {
				t.Errorf("seed %d: the curse cast at %v never hit before the next cast at %v", seed, cast, casts[i+1])
			}
		}
		return casts, hits
	}

	for seed := int64(1); seed <= 3; seed++ {
		stock, _ := run(&proto.ServerSettings{SpellTweaks: &proto.SpellTweaksSettings{CurseOfDoomScaling: googleProto.Bool(false)}}, seed)
		for i := 1; i < len(stock); i++ {
			if gap := stock[i] - stock[i-1]; gap < time.Minute {
				t.Errorf("seed %d, stock: curses %v apart, on a 60 s cooldown", seed, gap)
			}
		}
		scaled, hits := run(nil, seed)
		if len(scaled) <= len(stock) || len(hits) < len(scaled)-1 {
			t.Errorf("seed %d: %d curses and %d hits with the tweak, %d curses without", seed, len(scaled), len(hits), len(stock))
		}
		for i := 1; i < len(scaled); i++ {
			if gap := scaled[i] - scaled[i-1]; gap >= time.Minute {
				t.Errorf("seed %d, tweak: curses %v apart, want under the stock 60 s", seed, gap)
			}
		}
	}
}
