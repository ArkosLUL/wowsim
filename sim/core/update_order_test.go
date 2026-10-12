package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

// spell ids the server data doesn't have, in the order a unit's update walks them
const (
	testDotLowID  = 9900001
	testDotHighID = 9900002
)

func orderTestDot(t *testing.T, a *timingTestAgent, id int32, config DotConfig) *Spell {
	t.Helper()
	if serverdata.SpellByID(id) != nil {
		t.Fatalf("test spell %d has server data", id)
	}
	return a.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: id}, Dot: config})
}

// An aura runs out at its own time, whatever comes next and whichever unit it's on.
func TestAurasExpireInTimeOrder(t *testing.T) {
	var early, late *Aura
	var expiredAt []time.Duration
	lateSawEarly := false
	sim, _ := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		// the target is ahead of the player in the sim's list of units
		late = a.Env.Encounter.TargetUnits[0].RegisterAura(Aura{
			Label:    "late",
			Duration: ms(700),
			OnExpire: func(_ *Aura, sim *Simulation) {
				lateSawEarly = early.IsActive()
				expiredAt = append(expiredAt, sim.CurrentTime)
			},
		})
		early = a.RegisterAura(Aura{
			Label:    "early",
			Duration: ms(500),
			OnExpire: func(_ *Aura, sim *Simulation) { expiredAt = append(expiredAt, sim.CurrentTime) },
		})
	})
	sim.PrePull()
	at(sim, ms(100), func(sim *Simulation) {
		early.Activate(sim)
		late.Activate(sim)
	})
	runUntil(sim, 5*time.Second)

	if want := []time.Duration{ms(600), ms(800)}; !slices.Equal(expiredAt, want) {
		t.Errorf("auras expired at %v, want %v", expiredAt, want)
	}
	if lateSawEarly {
		t.Error("the later aura ran out with the earlier one still on")
	}
}

// On one world tick the players' auras go before the creatures' (Map::Update).
func TestPlayerAurasExpireAheadOfCreatureAurasOnOneUpdate(t *testing.T) {
	var players, targets *Aura
	var order []string
	sim, _ := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		note := func(label string) OnExpire {
			return func(_ *Aura, _ *Simulation) { order = append(order, label) }
		}
		targets = a.Env.Encounter.TargetUnits[0].RegisterAura(Aura{Label: "targets", Duration: ms(1000), OnExpire: note("target")})
		players = a.RegisterAura(Aura{Label: "players", Duration: ms(1000), OnExpire: note("player")})
	})
	sim.PrePull()
	phase := sim.serverTickPhase
	// the target's counts from the map update before, the player's from the world tick before
	at(sim, phase+ms(495), func(sim *Simulation) { targets.Activate(sim) })
	at(sim, phase+ms(410), func(sim *Simulation) { players.Activate(sim) })
	runUntil(sim, phase+2*time.Second)

	if players.ExpiresAt() != 0 || targets.ExpiresAt() != 0 {
		t.Fatal("both auras should be gone")
	}
	if want := []string{"player", "target"}; !slices.Equal(order, want) {
		t.Errorf("expired in the order %v, want %v", order, want)
	}
}

// Only a cast or a landing comes ahead of the player's update in its world tick. An aura the player's
// own swing puts on, or a creature's update after it, first counts down a tick later.
func TestPlayerAuraCountsFromWhatPutItOn(t *testing.T) {
	var fromSwing, fromTargetsTick *Aura
	var dot *Spell
	var swungAt, tickedAt time.Duration
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2.55), AutoSwingMelee: true})
		fromSwing = a.RegisterAura(Aura{Label: "from swing", Duration: ms(1050)})
		fromTargetsTick = a.RegisterAura(Aura{Label: "from the target's tick", Duration: ms(1050)})
		dot = orderTestDot(t, a, testDotLowID, DotConfig{
			Aura:          Aura{Label: "dot"},
			NumberOfTicks: 1,
			TickLength:    ms(1450),
			TicksCanCrit:  false,
			OnTick: func(sim *Simulation, _ *Unit, _ *Dot) {
				tickedAt = sim.CurrentTime
				fromTargetsTick.Activate(sim)
			},
		})
	})
	swung := false
	var swingsExpires time.Duration
	a.AutoAttacks.MHAuto().ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		if !swung {
			swung, swungAt = true, sim.CurrentTime
			fromSwing.Activate(sim)
			swingsExpires = fromSwing.ExpiresAt()
		}
	}
	sim.PrePull()
	phase := sim.serverTickPhase
	at(sim, phase+ms(480), func(sim *Simulation) { dot.Dot(sim.Encounter.TargetUnits[0]).Apply(sim) })
	runUntil(sim, phase+ms(1950))

	if !swung || tickedAt != phase+ms(1900) {
		t.Fatalf("swung %v at %v, tick at %v", swung, swungAt, tickedAt)
	}
	if got, want := swingsExpires, swungAt+ms(1050); got != want {
		t.Errorf("aura from the player's swing expires at %v, want %v", got, want)
	}
	if got, want := fromTargetsTick.ExpiresAt(), tickedAt+ms(1050); got != want {
		t.Errorf("aura from the target's tick expires at %v, want %v", got, want)
	}
}

// A cast from the main hand's last-moment APL check is the session's, ahead of the player's update, so
// an aura it puts on counts from the world tick before, unlike one the swing itself puts on.
func TestAuraFromTheLastMomentAPLCheckCountsFromTheTickBefore(t *testing.T) {
	var fromCheck *Aura
	onBeforeTheSwing := false
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{
			MainHand:       testWeapon(2),
			AutoSwingMelee: true,
			ReplaceMHSwing: func(sim *Simulation, mhSwingSpell *Spell) *Spell {
				onBeforeTheSwing = onBeforeTheSwing || fromCheck.RefreshedNow(sim)
				return mhSwingSpell
			},
		})
		fromCheck = a.RegisterAura(Aura{Label: "from the check", Duration: ms(1050)})
	})
	aa := &a.AutoAttacks
	aa.mh.setTimer(sim, ms(2000))
	swingAt := aa.mh.swingAt
	var expires time.Duration
	a.Rotation = &APLRotation{unit: &a.Unit, priorityList: []*APLAction{{impl: &testAPLAction{
		ready: func(sim *Simulation) bool { return expires == 0 && sim.CurrentTime == swingAt },
		execute: func(sim *Simulation) {
			fromCheck.Activate(sim)
			expires = fromCheck.ExpiresAt()
		},
	}}}}
	sim.PrePull()
	runUntil(sim, swingAt+ms(100))

	if !onBeforeTheSwing {
		t.Fatal("the APL check ahead of the swing should have put the aura on")
	}
	if want := swingAt - ms(10) + ms(1050); expires != want {
		t.Errorf("aura from the APL check expires at %v, want %v", expires, want)
	}
}

// Class code asks RefreshedNow whether an aura just went on. A player's has less than its duration left
// from the start, so the time left can't say.
func TestRefreshedNowIsTheMomentAnAuraGoesOnOrIsRefreshed(t *testing.T) {
	var aura *Aura
	sim, _ := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		aura = a.RegisterAura(Aura{Label: "buff", Duration: time.Second})
	})
	sim.PrePull()
	start := sim.serverTickPhase + ms(500)
	var got []bool
	at(sim, start, func(sim *Simulation) {
		got = append(got, aura.RefreshedNow(sim))
		aura.Activate(sim)
		got = append(got, aura.RefreshedNow(sim), aura.RemainingDuration(sim) == aura.Duration)
	})
	at(sim, start+ms(300), func(sim *Simulation) {
		got = append(got, aura.RefreshedNow(sim))
		aura.Activate(sim)
		got = append(got, aura.RefreshedNow(sim))
	})
	runUntil(sim, start+ms(400))

	if want := []bool{false, true, false, false, true}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A fixed-uptime aura set to be always up is: each tick finds the aura from the tick before still on.
func TestFixedUptimeAuraAtFullUptimeHasNoGaps(t *testing.T) {
	var aura *Aura
	faded := 0
	sim, _ := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		aura = a.RegisterAura(Aura{
			Label:    "always up",
			Duration: time.Second,
			OnExpire: func(_ *Aura, _ *Simulation) { faded++ },
		})
		ApplyFixedUptimeAura(aura, 1, time.Second, 1)
	})
	sim.PrePull()
	runUntil(sim, ms(5500))

	if faded != 0 || !aura.IsActive() {
		t.Errorf("faded %d times in 5.5 s, on at the end %v", faded, aura.IsActive())
	}
}

// Unit::_UpdateSpells walks a unit's auras in spell id order and removes the expired ones after: ticks
// due on one update run lowest id first, ahead of anything that ends on it.
func TestDotsDueOnOneUpdateTickInSpellIDOrder(t *testing.T) {
	for _, gridMs := range []int32{0, 100} {
		var low, high *Spell
		var debuff *Aura
		var order []string
		sim, _ := newTickTestSim(t, gridMs, gridMs/10, func(a *timingTestAgent) {
			note := func(label string) OnTick {
				return func(sim *Simulation, _ *Unit, _ *Dot) { order = append(order, label) }
			}
			config := DotConfig{NumberOfTicks: 2, TickLength: time.Second, TicksCanCrit: false}
			config.Aura, config.OnTick = Aura{Label: "high"}, note("high")
			high = orderTestDot(t, a, testDotHighID, config)
			config.Aura, config.OnTick = Aura{Label: "low"}, note("low")
			low = orderTestDot(t, a, testDotLowID, config)
			debuff = a.Env.Encounter.TargetUnits[0].RegisterAura(Aura{
				Label:    "debuff",
				Duration: time.Second,
				OnExpire: func(_ *Aura, _ *Simulation) { order = append(order, "debuff ends") },
			})
		})
		sim.PrePull()
		start := sim.serverTickPhase + ms(500)
		at(sim, start, func(sim *Simulation) {
			target := sim.Encounter.TargetUnits[0]
			// the higher id goes on first, so its ticks are queued first
			high.Dot(target).Apply(sim)
			low.Dot(target).Apply(sim)
			debuff.Activate(sim)
		})
		runUntil(sim, start+3*time.Second)

		want := []string{"low", "high", "debuff ends", "low", "high"}
		if !slices.Equal(order, want) {
			t.Errorf("%d ms updates: order %v, want %v", gridMs, order, want)
		}
	}
}

// A dot's own last tick shares its update with the aura's end, and still comes after a lower id's.
func TestLastTickComesAfterALowerSpellsOnTheExpirysUpdate(t *testing.T) {
	var low, high *Spell
	var order []string
	sim, _ := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		high = orderTestDot(t, a, testDotHighID, DotConfig{
			Aura: Aura{Label: "high", OnExpire: func(_ *Aura, _ *Simulation) { order = append(order, "high ends") }},
			// one tick, on the update the aura ends
			NumberOfTicks: 1,
			TickLength:    2 * time.Second,
			TicksCanCrit:  false,
			OnTick:        func(_ *Simulation, _ *Unit, _ *Dot) { order = append(order, "high") },
		})
		low = orderTestDot(t, a, testDotLowID, DotConfig{
			Aura:          Aura{Label: "low"},
			NumberOfTicks: 4,
			TickLength:    time.Second,
			TicksCanCrit:  false,
			OnTick:        func(_ *Simulation, _ *Unit, _ *Dot) { order = append(order, "low") },
		})
	})
	sim.PrePull()
	start := sim.serverTickPhase + ms(500)
	at(sim, start, func(sim *Simulation) {
		target := sim.Encounter.TargetUnits[0]
		high.Dot(target).Apply(sim)
		low.Dot(target).Apply(sim)
	})
	runUntil(sim, start+ms(2050))

	if want := []string{"low", "low", "high", "high ends"}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
}

// AuraEffect::Update stops at GetTotalTicks. A rollover leaves the tick timer running and starts the
// count over, so a tick can come due inside the new duration and still be one too many. The next
// refresh lets it through on the dot's following update, and the timer keeps its place.
func TestDotStopsAtItsTotalTicks(t *testing.T) {
	var spell *Spell
	var ticks *[]time.Duration
	sim, _ := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		spell, ticks = dotSpell(a, SpellFlagNone, DotConfig{
			Aura: Aura{Label: "dot"},
			// 12 s holds four 2401 ms ticks and most of a fifth
			NumberOfTicks: 4,
			TickLength:    ms(2401),
			TicksCanCrit:  false,
			AuraDuration:  12 * time.Second,
		})
	})
	sim.PrePull()
	dot := spell.Dot(sim.Encounter.TargetUnits[0])
	at(sim, 0, func(sim *Simulation) { dot.Apply(sim) })
	at(sim, ms(6500), func(sim *Simulation) { dot.Rollover(sim) })
	at(sim, ms(17000)+500*time.Microsecond, func(sim *Simulation) { dot.Rollover(sim) })
	runUntil(sim, 40*time.Second)

	want := []time.Duration{
		ms(2401), ms(4802),
		// four after the rollover at 6.5 s. The one due at 16.807 s is a fifth, 1.7 s before the aura ends
		ms(7203), ms(9604), ms(12005), ms(14406),
		// let through by the second rollover, then three more on the old timer
		ms(17001), ms(19208), ms(21609), ms(24010),
	}
	if !slices.Equal(*ticks, want) {
		t.Errorf("ticks at %v, want %v", *ticks, want)
	}
	if dot.IsActive() {
		t.Error("the dot should have run out 12 s after its last rollover")
	}
}

// Nothing updates a unit that's away, a dismissed pet say, so nothing ends an aura left on it. A dot
// there ends itself the next time a tick comes due, once it's out of ticks and past its time.
func TestDotOnAUnitNothingUpdatesEndsItself(t *testing.T) {
	var spell *Spell
	var ticks *[]time.Duration
	sim, _ := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		spell, ticks = dotSpell(a, SpellFlagNone, DotConfig{
			Aura:          Aura{Label: "dot"},
			NumberOfTicks: 2,
			TickLength:    time.Second,
			TicksCanCrit:  false,
		})
	})
	target := sim.Encounter.TargetUnits[0]
	sim.removeTracker(&target.auraTracker)
	sim.PrePull()
	dot := spell.Dot(target)
	start := sim.serverTickPhase + ms(500)
	at(sim, start, func(sim *Simulation) { dot.Apply(sim) })
	runUntil(sim, start+ms(3500))

	if len(*ticks) != 2 {
		t.Errorf("ticks at %v, want two", *ticks)
	}
	if dot.IsActive() {
		t.Error("the dot should have ended as its third tick came due")
	}
}

// Dot.Refresh is the server's refresh of a stacking dot: the duration and the tick count start over.
func TestRefreshedDotKeepsTicking(t *testing.T) {
	var spell *Spell
	var ticks *[]time.Duration
	sim, _ := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
		spell, ticks = dotSpell(a, SpellFlagNone, DotConfig{
			Aura:          Aura{Label: "dot"},
			NumberOfTicks: 2,
			TickLength:    time.Second,
			TicksCanCrit:  false,
		})
	})
	sim.PrePull()
	dot := spell.Dot(sim.Encounter.TargetUnits[0])
	at(sim, 0, func(sim *Simulation) { dot.Apply(sim) })
	for _, refresh := range []time.Duration{ms(1500), ms(3200)} {
		at(sim, refresh, func(sim *Simulation) { dot.Refresh(sim) })
	}
	runUntil(sim, 10*time.Second)

	// the last refresh leaves room for two more
	want := []time.Duration{ms(1000), ms(2000), ms(3000), ms(4000), ms(5000)}
	if !slices.Equal(*ticks, want) {
		t.Errorf("ticks at %v, want %v", *ticks, want)
	}
}

// A glyph that lengthens a running dot raises its max duration with the time left, so the added time
// ticks. Time added to the expiry alone doesn't: the dot is out of ticks by then.
func TestTimeAddedToADotTicksOnlyWithTheMaxDuration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		extend func(dot *Dot)
		want   []time.Duration
	}{
		{"max duration too", func(dot *Dot) { dot.UpdateExpires(dot.ExpiresAt() + 2*time.Second) },
			[]time.Duration{ms(1000), ms(2000), ms(3000), ms(4000)}},
		{"expiry alone", func(dot *Dot) { dot.Aura.UpdateExpires(dot.ExpiresAt() + 2*time.Second) },
			[]time.Duration{ms(1000), ms(2000)}},
	} {
		var spell *Spell
		var ticks *[]time.Duration
		sim, _ := newTickTestSim(t, 0, 0, func(a *timingTestAgent) {
			spell, ticks = dotSpell(a, SpellFlagNone, DotConfig{
				Aura:          Aura{Label: "dot"},
				NumberOfTicks: 2,
				TickLength:    time.Second,
				TicksCanCrit:  false,
			})
		})
		sim.PrePull()
		dot := spell.Dot(sim.Encounter.TargetUnits[0])
		at(sim, 0, func(sim *Simulation) { dot.Apply(sim) })
		at(sim, ms(1500), func(sim *Simulation) { tc.extend(dot) })
		runUntil(sim, 10*time.Second)

		if !slices.Equal(*ticks, tc.want) {
			t.Errorf("%s: ticks at %v, want %v", tc.name, *ticks, tc.want)
		}
		if dot.IsActive() {
			t.Errorf("%s: the dot should have ended at 4 s", tc.name)
		}
	}
}

// An aura left on a unit nothing updated (a pet that was away) is overdue when the unit is back. It goes
// then, and the clock doesn't go back to when it ran out.
func TestOverdueAuraLeavesTheClockAlone(t *testing.T) {
	var aura *Aura
	expiredAt := time.Duration(-1)
	sim, _ := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		aura = a.Env.Encounter.TargetUnits[0].RegisterAura(Aura{
			Label:    "left on",
			Duration: time.Second,
			OnExpire: func(_ *Aura, sim *Simulation) { expiredAt = sim.CurrentTime },
		})
	})
	target := sim.Encounter.TargetUnits[0]
	sim.removeTracker(&target.auraTracker)
	sim.PrePull()
	at(sim, ms(500), func(sim *Simulation) { aura.Activate(sim) })
	back := 5 * time.Second
	at(sim, back, func(sim *Simulation) { sim.addTracker(&target.auraTracker) })
	at(sim, back+ms(300), func(*Simulation) {})

	last := sim.CurrentTime
	for sim.CurrentTime < back+ms(300) && !sim.Step() {
		if sim.CurrentTime < last {
			t.Fatalf("the clock went from %v back to %v", last, sim.CurrentTime)
		}
		last = sim.CurrentTime
	}
	if expiredAt != back {
		t.Errorf("the overdue aura went at %v, want %v, when its unit was back", expiredAt, back)
	}
}

// A target that starts in the execute range is there from the pull.
func TestStartTargetAtHealthStartsTheExecutePhasesAtOnce(t *testing.T) {
	sim, _ := newTickTestSim(t, 0, 0, nil)
	var phases []int32
	sim.RegisterExecutePhaseCallback(func(_ *Simulation, phase int32) { phases = append(phases, phase) })

	sim.StartTargetAtHealth(1000, 220)

	if !sim.IsExecutePhase25() || sim.IsExecutePhase20() {
		t.Errorf("at 22%% health: under 25%% %v, under 20%% %v", sim.IsExecutePhase25(), sim.IsExecutePhase20())
	}
	if want := []int32{35, 25}; !slices.Equal(phases, want) {
		t.Errorf("phase callbacks %v, want %v", phases, want)
	}
}

// Unit::Update runs a player's events ahead of its auras: a hardcast landing on the world tick a buff
// runs out on still finds it.
func TestHardcastLandingOnAnAurasLastTickFindsIt(t *testing.T) {
	var buff *Aura
	var spell *Spell
	var order []string
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		buff = a.RegisterAura(Aura{
			Label:    "buff",
			Duration: time.Second,
			OnExpire: func(_ *Aura, _ *Simulation) { order = append(order, "expires") },
		})
		spell = a.RegisterSpell(castConfig(testSpellSlam, ms(1500)))
	})
	spell.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		order = append(order, fmt.Sprintf("lands, buff up %v", buff.IsActive()))
	}
	sim.PrePull()
	start := sim.NextWorldTick(ms(500))
	var landsAt time.Duration
	at(sim, start, func(sim *Simulation) {
		if !spell.Cast(sim, a.CurrentTarget) {
			t.Fatal("the cast failed")
		}
		landsAt = a.Hardcast.Expires
		// counts from the tick before, ends on a tick: expires with the landing
		buff.Duration = landsAt - start + sim.worldTickInterval
		buff.Activate(sim)
	})
	runUntil(sim, start+2*time.Second)

	if buff.ExpiresAt() != 0 {
		t.Fatalf("the buff should be gone, expires at %v", buff.ExpiresAt())
	}
	if want := []string{"lands, buff up true", "expires"}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v (landing at %v)", order, want, landsAt)
	}
}

// A periodic action with an owner is one of its periodic aura ticks: an aura it puts on counts from that
// tick, and it comes ahead of the aura the previous tick put on running out then.
func TestOwnedPeriodicActionPutsItsAuraOnFromItsOwnTick(t *testing.T) {
	var buff *Aura
	var order []string
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		buff = a.RegisterAura(Aura{
			Label:    "buff",
			Duration: 10 * time.Second,
			OnExpire: func(_ *Aura, _ *Simulation) { order = append(order, "expires") },
		})
	})
	sim.PrePull()
	first := sim.NextWorldTick(ms(500))
	var expires []time.Duration
	StartPeriodicAction(sim, PeriodicActionOptions{
		Owner:       &a.Unit,
		FirstTickAt: first,
		Period:      2 * time.Second,
		OnAction: func(sim *Simulation) {
			if !buff.IsActive() {
				buff.Activate(sim)
				expires = append(expires, buff.ExpiresAt()-sim.CurrentTime)
				return
			}
			order = append(order, "check sees it")
		},
	})
	runUntil(sim, first+13*time.Second)

	if len(expires) < 2 || expires[0] != 10*time.Second {
		t.Errorf("the buff has %v left as the tick puts it on, want 10 s", expires)
	}
	// the fifth check shares an update with the end of the buff and still sees it, so it is up again two seconds later
	want := []string{"check sees it", "check sees it", "check sees it", "check sees it", "check sees it", "expires"}
	if len(order) < len(want) || !slices.Equal(order[:len(want)], want) {
		t.Errorf("order %v, want it to start %v", order, want)
	}
}

// An execute phase begins at its own time, not with the next thing that moves the clock.
func TestExecutePhaseStartsAtItsTime(t *testing.T) {
	sim, _ := newTickTestSim(t, 100, 10, nil)
	sim.Encounter.ExecuteProportion_35 = 0.5
	sim.Reset()
	var got []time.Duration
	sim.RegisterExecutePhaseCallback(func(sim *Simulation, phase int32) {
		got = append(got, sim.CurrentTime)
	})
	sim.PrePull()
	// nothing else is queued until the end
	at(sim, 50*time.Second, func(*Simulation) {})
	for sim.CurrentTime < 50*time.Second {
		if sim.Step() {
			break
		}
	}
	if want := sim.Duration / 2; len(got) == 0 || got[0] != want {
		t.Errorf("the 35%% phase began at %v, want %v", got, want)
	}
}

// The world loop time makes every world tick, and with it a map update, longer than MinWorldUpdateTime.
func TestWorldLoopTimeStretchesTheTicks(t *testing.T) {
	settings := NewServerSettings(nil)
	world, mapUpdate := settings.updateGrids()
	if world != 10460*time.Microsecond || mapUpdate != 104600*time.Microsecond {
		t.Errorf("live world tick %v and map update %v, want 10.46 and 104.6 ms", world, mapUpdate)
	}
	settings.WorldLoopTime = ms(4)
	if world, mapUpdate = settings.updateGrids(); world != ms(10) || mapUpdate != ms(100) {
		t.Errorf("a loop under the floor: tick %v and map update %v, want 10 and 100 ms", world, mapUpdate)
	}
}
