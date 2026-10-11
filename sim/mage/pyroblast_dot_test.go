package mage

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

var (
	pyroblastHasteOff = &proto.ServerSettings{SpellTweaks: &proto.SpellTweaksSettings{PyroblastEmpoweredFire: googleProto.Bool(false)}}
	spellTweaksOff    = &proto.ServerSettings{SpellTweaks: &proto.SpellTweaksSettings{Enable: googleProto.Bool(false)}}
)

// FireTalents with this many points of Empowered Fire, the fire tree's 23rd talent
func fireTalentsWithEmpoweredFire(points byte) string {
	trees := strings.Split(FireTalents, "-")
	fire := []byte(trees[1])
	fire[22] = '0' + points
	return trees[0] + "-" + string(fire)
}

// gearlessSim has a mage with no gear and no rotation, so the cast speed is the test's to set.
func gearlessSim(t *testing.T, talents string, spec *proto.Player_Mage, settings *proto.ServerSettings) (*core.Simulation, *Mage, *core.Unit) {
	t.Helper()
	player := procMage(talents, "p3_fire_alliance", `{"type": "TypeAPL", "priorityList": []}`, spec)
	player.Equipment = &proto.EquipmentSpec{}
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.ServerSettings = settings
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].(*Mage), sim.GetTargetUnit(0)
}

// pyroblastSim is a gearless fire mage, whose cast speed is 1.
func pyroblastSim(t *testing.T, talents string, settings *proto.ServerSettings) (*core.Simulation, *Mage, *core.Unit) {
	t.Helper()
	sim, mage, target := gearlessSim(t, talents, PlayerOptionsFire, settings)
	if mage.CastSpeed != 1 {
		t.Fatalf("a mage with no gear casts at x%v, want x1", mage.CastSpeed)
	}
	return sim, mage, target
}

// spell_tweaks_pyroblast_dot_haste: with Empowered Fire a tick takes int32(3000 x cast speed) ms of the fixed
// 12 s, and a slow changes nothing. SpellTweaks.PyroblastEmpoweredFire and SpellTweaks.Enable gate that; the
// tick crit is Empowered Fire's own spell_dbc effect, there with either off.
func TestPyroblastDotHaste(t *testing.T) {
	const ms = time.Millisecond
	for _, c := range []struct {
		name     string
		talents  string
		settings *proto.ServerSettings
		haste    float64
		period   time.Duration
		ticks    int32
		crits    bool
	}{
		{"no haste", FireTalents, nil, 1, 3000 * ms, 4, true},
		{"24% haste", FireTalents, nil, 1.24, 2419 * ms, 4, true},
		{"25% haste", FireTalents, nil, 1.25, 2400 * ms, 5, true},
		{"50% haste", FireTalents, nil, 1.5, 2000 * ms, 6, true},
		{"slowed", FireTalents, nil, 0.8, 3000 * ms, 4, true},
		{"one point of Empowered Fire", fireTalentsWithEmpoweredFire(1), nil, 1.5, 2000 * ms, 6, true},
		{"switch off", FireTalents, pyroblastHasteOff, 1.5, 3000 * ms, 4, true},
		{"spell tweaks off", FireTalents, spellTweaksOff, 1.5, 3000 * ms, 4, true},
		{"no Empowered Fire", fireTalentsWithEmpoweredFire(0), nil, 1.5, 3000 * ms, 4, false},
	} {
		sim, mage, target := pyroblastSim(t, c.talents, c.settings)
		if (mage.Talents.EmpoweredFire > 0) != c.crits {
			t.Fatalf("%s: %d points of Empowered Fire", c.name, mage.Talents.EmpoweredFire)
		}
		mage.MultiplyCastSpeed(c.haste)
		dot := mage.Pyroblast.Dot(target)
		dot.Apply(sim)
		if dot.TickPeriod() != c.period || dot.TotalTicks() != c.ticks || dot.Aura.Duration != 12*time.Second {
			t.Errorf("%s: %d ticks %v apart over %v, want %d ticks %v apart over 12s",
				c.name, dot.TotalTicks(), dot.TickPeriod(), dot.Aura.Duration, c.ticks, c.period)
		}
		if dot.TicksCanCrit != c.crits || (c.crits && dot.SnapshotCritChance <= 0) {
			t.Errorf("%s: ticks can crit %v at %.3f, want %v", c.name, dot.TicksCanCrit, dot.SnapshotCritChance, c.crits)
		}
		if dot.Spell.ActionID != (core.ActionID{SpellID: 42891, Tag: 1}) {
			t.Errorf("%s: the ticks are dealt as %v, want Pyroblast's tag 1", c.name, dot.Spell.ActionID)
		}

		// haste that comes while it runs waits for the next Pyroblast
		mage.MultiplyCastSpeed(2)
		if dot.TickPeriod() != c.period {
			t.Errorf("%s: the running DoT went from %v to %v a tick on a haste gain", c.name, c.period, dot.TickPeriod())
		}
	}
}

// The DoT takes the cast speed the Pyroblast lands with, not the one it was cast at.
func TestPyroblastDotReadsHasteAsItLands(t *testing.T) {
	sim, mage, target := pyroblastSim(t, FireTalents, nil)
	mage.Pyroblast.BonusHitRating += 100 * core.SpellHitRatingPerHitChance
	mage.HotStreakAura.Activate(sim)
	if !mage.Pyroblast.Cast(sim, target) {
		t.Fatal("Pyroblast didn't cast")
	}
	dot := mage.Pyroblast.Dot(target)
	if dot.IsActive() {
		t.Fatal("the DoT is up before the missile lands")
	}
	mage.MultiplyCastSpeed(1.5)
	for !dot.IsActive() && !sim.Step() {
	}
	if !dot.IsActive() || dot.TickPeriod() != 2*time.Second || dot.TotalTicks() != 6 {
		t.Errorf("DoT up %v, %d ticks %v apart, want 6 ticks 2s apart from the haste at the landing",
			dot.IsActive(), dot.TotalTicks(), dot.TickPeriod())
	}
}

func logCount(entries []procLogEntry, text string) int {
	n := 0
	for _, e := range entries {
		if strings.Contains(e.text, text) {
			n++
		}
	}
	return n
}

// With Empowered Fire the ticks crit, whatever the switches say. A crit tick puts Ignite on at the mage's
// next 400 ms step (spell_proc -11119 takes periodic crits). Hot Streak and Master of Elements have no
// periodic proc flag, and spell_tweaks_burnout_pyroblast_dot keeps Burnout's cost off the ticks.
func TestPyroblastDotCrits(t *testing.T) {
	const fight = 120 * time.Second
	for _, c := range []struct {
		name     string
		talents  string
		settings *proto.ServerSettings
		crits    bool
	}{
		{"tweak on", FireTalents, nil, true},
		{"haste switch off", FireTalents, pyroblastHasteOff, true},
		{"spell tweaks off", FireTalents, spellTweaksOff, true},
		{"no Empowered Fire", fireTalentsWithEmpoweredFire(0), nil, false},
	} {
		ticks, crits, direct := 0, 0, 0
		for seed := int64(1); seed <= 3; seed++ {
			player := procMage(c.talents, "p3_fire_alliance", castApl("", "42891"), PlayerOptionsFire)
			entries := runProcLogOn(t, player, c.settings, fight.Seconds(), seed)

			critTicks := logTimes(entries, "[Target 1] {SpellID: 42891, Tag: 1} tick Crit")
			ticks += len(critTicks) + len(logTimes(entries, "[Target 1] {SpellID: 42891, Tag: 1} tick Hit"))
			crits += len(critTicks)
			if got := logTimes(entries, "[Target 1] {SpellID: 42891} tick"); len(got) != 0 {
				t.Errorf("%s, seed %d: ticks under the cast's own id at %v", c.name, seed, got)
			}

			// the log's times are in hundredths of a second
			ignites := logTimes(entries, "Completed cast {SpellID: 12654}")
			for _, at := range critTicks {
				onTime := func(ignite time.Duration) bool {
					return ignite >= at-10*time.Millisecond && ignite <= at+410*time.Millisecond
				}
				if at < fight-time.Second && !slices.ContainsFunc(ignites, onTime) {
					t.Errorf("%s, seed %d: no Ignite within 400 ms of the crit tick at %v", c.name, seed, at)
				}
			}

			directCrits := len(logTimes(entries, "[Target 1] {SpellID: 42891} Crit"))
			direct += directCrits
			if got := logTimes(entries, "Aura gained: {SpellID: 44448"); len(got) != 0 {
				t.Errorf("%s, seed %d: Hot Streak counted crits at %v with only Pyroblasts cast", c.name, seed, got)
			}
			if got := logCount(entries, "mana from {SpellID: 29076}"); got != directCrits {
				t.Errorf("%s, seed %d: %d Master of Elements refunds for %d direct crits", c.name, seed, got, directCrits)
			}
			if got := logCount(entries, "mana from {SpellID: 44472}"); got != directCrits {
				t.Errorf("%s, seed %d: Burnout burned mana %d times for %d direct crits", c.name, seed, got, directCrits)
			}
		}
		if ticks == 0 || direct == 0 || (crits > 0) != c.crits {
			t.Errorf("%s: %d of %d ticks crit and %d direct hits did, want tick crits %v", c.name, crits, ticks, direct, c.crits)
		}
	}
}

// Unit::_UpdateSpells walks a unit's auras in spell id order, so when Ignite (12654) and the Pyroblast DoT
// (42891) tick on one update, Ignite's tick lands first and the DoT's crit rolls over only what's left.
func TestPyroblastDotCritAfterIgnitesTick(t *testing.T) {
	sim, mage, target := pyroblastSim(t, FireTalents, nil)
	if mage.Talents.Ignite != 5 {
		t.Fatalf("FireTalents has %d points of Ignite, want 5", mage.Talents.Ignite)
	}
	// both tick every 2 s
	mage.MultiplyCastSpeed(1.5)

	pyro := mage.Pyroblast.Dot(target)
	pyro.Spell.BonusCritRating += 1000 * core.CritRatingPerCritChance
	pyro.Apply(sim)

	const igniteTick = 1000.0
	ignite := mage.Ignite.Dot(target)
	ignite.SnapshotAttackerMultiplier = 1
	ignite.SnapshotBaseDamage = igniteTick
	mage.Ignite.Cast(sim, target)

	tickMetrics := &pyro.Spell.SpellMetrics[target.UnitIndex]
	for tickMetrics.Crits == 0 && !sim.Step() {
	}
	if tickMetrics.Crits != 1 || ignite.TickCount != 1 || mage.Ignite.SpellMetrics[target.UnitIndex].TotalDamage == 0 {
		t.Fatalf("at the DoT's first tick: %d crits, Ignite at tick %d with %v dealt, want its first tick in",
			tickMetrics.Crits, ignite.TickCount, mage.Ignite.SpellMetrics[target.UnitIndex].TotalDamage)
	}

	for ignite.SnapshotBaseDamage == igniteTick && !sim.Step() {
	}
	// 40% of the crit over Ignite's two ticks, plus the one tick the old Ignite had left
	want := math.Trunc(math.Trunc(tickMetrics.TotalDamage*40/100)/IgniteTicks) + math.Trunc(igniteTick/IgniteTicks)
	if ignite.SnapshotBaseDamage != want {
		t.Errorf("Ignite ticks for %v after a %v crit tick, want %v", ignite.SnapshotBaseDamage, tickMetrics.TotalDamage, want)
	}
}
