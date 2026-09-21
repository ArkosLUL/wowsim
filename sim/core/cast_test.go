package core

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

const (
	testSpellMoonfire        = 48463 // HasteGCD, resets autos
	testSpellCrusaderStrike  = 35395 // melee: GCD not hasted
	testSpellHeroicThrow     = 57755 // instant, resets autos
	testSpellExorcism        = 48801 // 1.5 s cast, resets autos
	testSpellSlam            = 47475 // 1.5 s cast, doesn't reset
	testSpellLightningBolt   = 49238 // resets autos, unless Maelstrom Weapon is up
	testSpellMaelstromWeapon = 53817
	testSpellSteadyShot      = 49052 // ranged class
	testSpellShadowcrawl     = 63619 // server GCD 1500, sim's is 6 s
	testSpellNoServerData    = 42
)

// registeredSpell has the server entry RegisterSpell would have looked up.
func registeredSpell(id int32) *Spell {
	return &Spell{ActionID: ActionID{SpellID: id}, CastTimeMultiplier: 1, serverSpell: serverdata.SpellByID(id)}
}

func TestGCDFollowsTriggerGlobalCooldown(t *testing.T) {
	for _, tc := range []struct {
		name        string
		spellID     int32
		gcd         time.Duration
		castSpeed   float64
		ignoreHaste bool
		want        time.Duration
	}{
		{"HasteGCD", testSpellMoonfire, GCDDefault, 0.8, false, ms(1200)},
		{"HasteGCD ignores IgnoreHaste", testSpellMoonfire, GCDDefault, 0.8, true, ms(1200)},
		{"HasteGCD floored", testSpellMoonfire, GCDDefault, 0.5, false, GCDMin},
		{"HasteGCD slowed, capped", testSpellMoonfire, GCDDefault, 1.2, false, GCDDefault},
		{"no HasteGCD", testSpellCrusaderStrike, GCDDefault, 0.8, false, GCDDefault},
		{"no HasteGCD, floored", testSpellCrusaderStrike, ms(800), 1, true, GCDMin},
		{"sim GCD above 1.5 s left alone", testSpellShadowcrawl, 6 * time.Second, 0.8, false, 6 * time.Second},
		{"no server data: sim rule", testSpellNoServerData, GCDDefault, 0.8, false, ms(1200)},
		{"no server data, IgnoreHaste", testSpellNoServerData, GCDDefault, 0.8, true, GCDDefault},
		{"no server data, floored", testSpellNoServerData, GCDDefault, 0.5, false, GCDMin},
		{"off the GCD", testSpellMoonfire, 0, 0.8, false, 0},
	} {
		timing := newCastTiming(registeredSpell(tc.spellID), tc.ignoreHaste)
		unit := &Unit{CastSpeed: tc.castSpeed}
		if got := timing.gcd(unit, tc.gcd); got != tc.want {
			t.Errorf("%s: GCD %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCastTimeHasteByDamageClass(t *testing.T) {
	unit := &Unit{CastSpeed: 0.8}
	unit.PseudoStats.RangedSpeedMultiplier = 1.25
	for _, tc := range []struct {
		name    string
		spellID int32
		want    time.Duration
	}{
		{"ranged: ranged attack speed", testSpellSteadyShot, ms(1600)},
		{"magic: cast speed", testSpellExorcism, ms(1600)},
		{"melee: none", testSpellSlam, 2 * time.Second},
		{"no server data: cast speed", testSpellNoServerData, ms(1600)},
	} {
		spell := registeredSpell(tc.spellID)
		unit.CastSpeed = 0.8
		if tc.spellID == testSpellSteadyShot {
			unit.CastSpeed = 0.5 // must not matter
		}
		timing := newCastTiming(spell, false)
		if got := timing.castTime(unit, 2*time.Second, spell); got != tc.want {
			t.Errorf("%s: cast time %v, want %v", tc.name, got, tc.want)
		}
	}
}

// swingsAroundCast casts the spell at 0.5 s against a 2 s main hand that swung at 0, and returns the
// main hand's swings up to 7 s.
func swingsAroundCast(t *testing.T, config SpellConfig, setup func(*timingTestAgent, *Spell)) []time.Duration {
	t.Helper()
	var spell *Spell
	sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), OffHand: testWeapon(2), AutoSwingMelee: true})
		spell = a.RegisterSpell(config)
		if setup != nil {
			setup(a, spell)
		}
	})
	aa := &a.AutoAttacks
	swings := recordCasts(aa.MHAuto())
	recordCasts(aa.OHAuto())
	recordCasts(spell)
	aa.mh.setTimer(sim, 0)
	aa.oh.setTimer(sim, time.Second)
	sim.PrePull()
	at(sim, ms(500), func(sim *Simulation) {
		if !spell.Cast(sim, a.CurrentTarget) {
			t.Fatalf("%v didn't cast", spell.ActionID)
		}
	})
	runUntil(sim, 7*time.Second)
	return *swings
}

func castConfig(spellID int32, castTime time.Duration) SpellConfig {
	return SpellConfig{
		ActionID: ActionID{SpellID: spellID},
		Cast:     CastConfig{DefaultCast: Cast{GCD: GCDDefault, CastTime: castTime}},
	}
}

func TestCastResetsSwingTimer(t *testing.T) {
	noReset := []time.Duration{0, ms(2000), ms(4000), ms(6000)}
	for _, tc := range []struct {
		name   string
		config SpellConfig
		setup  func(*timingTestAgent, *Spell)
		want   []time.Duration
	}{
		{"instant", castConfig(testSpellHeroicThrow, 0), nil, []time.Duration{0, ms(2500), ms(4500), ms(6500)}},
		{"hardcast: no swings while casting, restarts when it lands", castConfig(testSpellExorcism, ms(1500)), nil,
			[]time.Duration{0, ms(4000), ms(6000)}},
		{"Slam: no reset, the timers stand still while it casts", castConfig(testSpellSlam, ms(1500)), nil,
			[]time.Duration{0, ms(3500), ms(5500)}},
		{"made instant", func() SpellConfig {
			config := castConfig(testSpellExorcism, ms(1500))
			config.Cast.ModifyCast = func(_ *Simulation, _ *Spell, cast *Cast) { cast.CastTime = 0 }
			return config
		}(), nil, noReset},
		{"no server data", castConfig(testSpellNoServerData, 0), nil, noReset},
		{"cast without a GCD or cast time", SpellConfig{
			ActionID:           ActionID{SpellID: testSpellHeroicThrow},
			ExtraCastCondition: func(*Simulation, *Unit) bool { return true },
		}, nil, noReset},
		{"Maelstrom Weapon up", castConfig(testSpellLightningBolt, ms(2500)), func(a *timingTestAgent, _ *Spell) {
			mw := a.RegisterAura(Aura{Label: "Maelstrom Weapon", ActionID: ActionID{SpellID: testSpellMaelstromWeapon}, Duration: time.Minute})
			a.RegisterResetEffect(func(sim *Simulation) {
				at(sim, ms(100), func(sim *Simulation) { mw.Activate(sim) })
			})
		}, []time.Duration{0, ms(3000), ms(5000), ms(7000)}}, // no reset, but the 2 s swing waits for the cast
		{"Maelstrom Weapon down", castConfig(testSpellLightningBolt, ms(2500)), nil, []time.Duration{0, ms(5000), ms(7000)}},
	} {
		if got := swingsAroundCast(t, tc.config, tc.setup); !slices.Equal(got, tc.want) {
			t.Errorf("%s: main hand swings %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCastResetRestartsEveryHand(t *testing.T) {
	var spell *Spell
	sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{
			MainHand:        testWeapon(2),
			OffHand:         testWeapon(1.5),
			Ranged:          testWeapon(3),
			AutoSwingMelee:  true,
			AutoSwingRanged: true,
		})
		spell = a.RegisterSpell(castConfig(testSpellHeroicThrow, 0))
	})
	aa := &a.AutoAttacks
	for _, s := range []*Spell{aa.MHAuto(), aa.OHAuto(), aa.RangedAuto(), spell} {
		recordCasts(s)
	}
	aa.mh.setTimer(sim, ms(1000))
	aa.oh.setTimer(sim, ms(1400))
	aa.ranged.setTimer(sim, ms(1800))
	sim.PrePull()
	at(sim, ms(500), func(sim *Simulation) { spell.Cast(sim, a.CurrentTarget) })
	runUntil(sim, ms(600))

	// each hand gets its own full swing, so no push between them
	for _, tc := range []struct {
		name string
		wa   *WeaponAttack
		want time.Duration
	}{
		{"main hand", &aa.mh, ms(2500)},
		{"off hand", &aa.oh, ms(2000)},
		{"ranged", &aa.ranged, ms(3500)},
	} {
		if tc.wa.swingAt != tc.want {
			t.Errorf("%s next swing at %v, want %v", tc.name, tc.wa.swingAt, tc.want)
		}
	}
}

func TestPrepullCastDelaysFirstSwing(t *testing.T) {
	var spell *Spell
	sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), AutoSwingMelee: true})
		spell = a.RegisterSpell(castConfig(testSpellHeroicThrow, 0))
	})
	swings := recordCasts(a.AutoAttacks.MHAuto())
	recordCasts(spell)
	sim.prepullActions = []PrepullAction{{DoAt: -time.Second, Action: func(sim *Simulation) { spell.Cast(sim, a.CurrentTarget) }}}
	sim.PrePull()
	runUntil(sim, ms(1500))

	if want := []time.Duration{ms(1000)}; !slices.Equal(*swings, want) {
		t.Errorf("swings %v, want %v: the timer restarted at -1 s", *swings, want)
	}
}

func TestSpellRegistrationFindsIgnoreMeleeResetAuras(t *testing.T) {
	if got := newCastTiming(registeredSpell(testSpellLightningBolt), false).ignoreResetAuras; !slices.Equal(got, []ActionID{{SpellID: testSpellMaelstromWeapon}}) {
		t.Errorf("Lightning Bolt's IGNORE_MELEE_RESET auras = %v, want Maelstrom Weapon", got)
	}
	if got := newCastTiming(registeredSpell(testSpellExorcism), false).ignoreResetAuras; len(got) != 0 {
		t.Errorf("Exorcism's IGNORE_MELEE_RESET auras = %v, want none", got)
	}
}

func TestIgnoreMeleeResetEffectFollowsIsAffected(t *testing.T) {
	shamanSpell := &serverdata.Spell{Family: 11, FamilyFlags: [3]uint32{0x1}}
	for _, tc := range []struct {
		name   string
		effect ignoreMeleeResetEffect
		want   bool
	}{
		{"class mask match", ignoreMeleeResetEffect{family: 11, classMask: [3]uint32{0x3}}, true},
		{"class mask miss", ignoreMeleeResetEffect{family: 11, classMask: [3]uint32{0, 0x1}}, false},
		{"other family", ignoreMeleeResetEffect{family: 10, classMask: [3]uint32{0x1}}, false},
		{"no class mask: the whole family", ignoreMeleeResetEffect{family: 11}, true},
		{"family 0: every spell", ignoreMeleeResetEffect{classMask: [3]uint32{0, 0x1}}, true},
	} {
		if got := tc.effect.affects(shamanSpell); got != tc.want {
			t.Errorf("%s: affects = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// CastTime and EffectiveCastTime are what the APL predicts a cast will cost, so they follow the same
// castTiming the cast itself does.
func TestCastTimePredictionsFollowTheCast(t *testing.T) {
	var steady, exorcism, slam, fixed, moonfire, crusaderStrike *Spell
	_, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		// a spell with server data takes its cast time: Steady Shot 2 s, Exorcism and Slam 1.5 s
		steady = a.RegisterSpell(castConfig(testSpellSteadyShot, 2*time.Second))
		exorcism = a.RegisterSpell(castConfig(testSpellExorcism, ms(1500)))
		slam = a.RegisterSpell(castConfig(testSpellSlam, ms(1500)))
		noHaste := castConfig(testSpellNoServerData, 2*time.Second)
		noHaste.Cast.IgnoreHaste = true
		fixed = a.RegisterSpell(noHaste)
		moonfire = a.RegisterSpell(castConfig(testSpellMoonfire, 0))
		crusaderStrike = a.RegisterSpell(castConfig(testSpellCrusaderStrike, 0))
	})
	a.CastSpeed = 0.8
	a.PseudoStats.RangedSpeedMultiplier = 2

	for _, tc := range []struct {
		name  string
		spell *Spell
		want  time.Duration
	}{
		{"ranged: ranged attack speed", steady, time.Second},
		{"magic: cast speed", exorcism, ms(1200)},
		{"melee: no haste", slam, ms(1500)},
		{"IgnoreHaste", fixed, 2 * time.Second},
	} {
		if got := tc.spell.CastTime(); got != tc.want {
			t.Errorf("%s: CastTime %v, want %v", tc.name, got, tc.want)
		}
	}

	// the GCD rule, not a hasted 1.5 s for everyone
	if got := moonfire.EffectiveCastTime(); got != ms(1200) {
		t.Errorf("Moonfire EffectiveCastTime %v, want 1.2s: its GCD is hasted", got)
	}
	if got := crusaderStrike.EffectiveCastTime(); got != GCDDefault {
		t.Errorf("Crusader Strike EffectiveCastTime %v, want 1.5s: a melee GCD isn't hasted", got)
	}
	if got := steady.EffectiveCastTime(); got != GCDDefault {
		t.Errorf("Steady Shot EffectiveCastTime %v, want 1.5s: its GCD outlasts the 1 s cast", got)
	}
}

// logEffects replaces each spell's effects with a line in one shared log, so the order of things that
// land at the same moment shows.
func logEffects(spells map[string]*Spell) *[]string {
	var events []string
	for name, spell := range spells {
		spell.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
			events = append(events, fmt.Sprintf("%s %v", name, sim.CurrentTime))
		}
	}
	return &events
}

// noGCDCast is a hardcast with neither server data nor a GCD, so another can start the moment it lands.
func noGCDCast(tag int32, castTime time.Duration) SpellConfig {
	return SpellConfig{
		ActionID: ActionID{SpellID: testSpellNoServerData, Tag: tag},
		Cast:     CastConfig{DefaultCast: Cast{CastTime: castTime}},
	}
}

// A hardcast that doesn't reset the swing timer still stops the swings: whatever comes due waits for
// the cast to land, then goes, main hand first.
func TestCastHoldsSwingsUntilItLands(t *testing.T) {
	var spell *Spell
	sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), OffHand: testWeapon(2), AutoSwingMelee: true})
		spell = a.RegisterSpell(castConfig(testSpellNoServerData, time.Second))
	})
	aa := &a.AutoAttacks
	events := logEffects(map[string]*Spell{"mh": aa.MHAuto(), "oh": aa.OHAuto(), "cast": spell})
	aa.mh.setTimer(sim, ms(1000))
	aa.oh.setTimer(sim, ms(1100))
	sim.PrePull()
	at(sim, ms(500), func(sim *Simulation) { spell.Cast(sim, a.CurrentTarget) })
	runUntil(sim, ms(3900))

	// the off hand, held too, gets the usual 200 ms push from the main hand's swing
	want := []string{"cast 1.5s", "mh 1.5s", "oh 1.7s", "mh 3.5s", "oh 3.7s"}
	if !slices.Equal(*events, want) {
		t.Errorf("got %v, want %v", *events, want)
	}
}

// The update a cast lands on swings what it held before anything cast after it can start.
func TestHeldSwingGoesBeforeTheNextCast(t *testing.T) {
	var first, second *Spell
	sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), OffHand: testWeapon(2), AutoSwingMelee: true})
		first = a.RegisterSpell(noGCDCast(1, time.Second))
		second = a.RegisterSpell(noGCDCast(2, time.Second))
	})
	aa := &a.AutoAttacks
	events := logEffects(map[string]*Spell{"mh": aa.MHAuto(), "oh": aa.OHAuto(), "first": first, "second": second})
	aa.mh.setTimer(sim, ms(1000))
	aa.oh.setTimer(sim, ms(1100))
	sim.PrePull()
	at(sim, ms(500), func(sim *Simulation) {
		first.Cast(sim, a.CurrentTarget)
		// queued after the first cast's own action, so it goes once that one has landed
		at(sim, ms(1500), func(sim *Simulation) {
			if !second.Cast(sim, a.CurrentTarget) {
				t.Errorf("second cast failed at %v", sim.CurrentTime)
			}
		})
	})
	runUntil(sim, ms(3900))

	// the off hand's push to 1.7 s lands inside the second cast, so that one holds it
	want := []string{"first 1.5s", "mh 1.5s", "second 2.5s", "oh 2.5s", "mh 3.5s"}
	if !slices.Equal(*events, want) {
		t.Errorf("got %v, want %v", *events, want)
	}
}

// Auto Shot waits out a cast too, unless the spell has SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS
// (Unit::IsNonMeleeSpellCast), like Steady Shot.
func TestAutoShotHeldUnlessTheCastKeepsCombatTimers(t *testing.T) {
	for _, tc := range []struct {
		name     string
		spellID  int32
		castTime time.Duration
		want     time.Duration
	}{
		{"Steady Shot", testSpellSteadyShot, 2 * time.Second, ms(1000)},
		{"any other cast", testSpellNoServerData, time.Second, ms(1500)},
	} {
		var spell *Spell
		sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
			a.EnableAutoAttacks(a, AutoAttackOptions{Ranged: testWeapon(3), AutoSwingRanged: true})
			spell = a.RegisterSpell(castConfig(tc.spellID, tc.castTime))
		})
		aa := &a.AutoAttacks
		shots := recordCasts(aa.RangedAuto())
		recordCasts(spell)
		aa.ranged.setTimer(sim, ms(1000))
		sim.PrePull()
		at(sim, ms(500), func(sim *Simulation) { spell.Cast(sim, a.CurrentTarget) })
		runUntil(sim, ms(2000))

		if want := []time.Duration{tc.want}; !slices.Equal(*shots, want) {
			t.Errorf("%s: shots at %v, want %v", tc.name, *shots, want)
		}
	}
}

// A shot due on the very update Steady Shot lands still goes after it: spell events run before
// Unit::_UpdateSpells gets to Auto Shot.
func TestAutoShotWaitsForTheCastLandingOnItsTick(t *testing.T) {
	var steady *Spell
	sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{Ranged: testWeapon(3), AutoSwingRanged: true})
		steady = a.RegisterSpell(castConfig(testSpellSteadyShot, 2*time.Second))
	})
	aa := &a.AutoAttacks
	events := logEffects(map[string]*Spell{"shot": aa.RangedAuto(), "steady": steady})
	aa.ranged.setTimer(sim, ms(2500))
	sim.PrePull()
	at(sim, ms(500), func(sim *Simulation) { steady.Cast(sim, a.CurrentTarget) })
	runUntil(sim, ms(3000))

	if want := []string{"steady 2.5s", "shot 2.5s"}; !slices.Equal(*events, want) {
		t.Errorf("got %v, want %v", *events, want)
	}
}

// Unit::Update doesn't count a player's melee timers down on the updates a
// SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS cast (Slam) spans: from the one that takes the cast up to the
// one it lands on.
func TestSlamPausesMeleeTimersForTheUpdatesItSpans(t *testing.T) {
	var slam *Spell
	sim, a := newTimingTestSim(t, 100, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), AutoSwingMelee: true})
		slam = a.RegisterSpell(castConfig(testSpellSlam, ms(500)))
	})
	aa := &a.AutoAttacks
	events := logEffects(map[string]*Spell{"mh": aa.MHAuto(), "slam": slam})
	p := sim.serverTickPhase
	// 70 ms short of the tick after the cast
	aa.mh.setTimer(sim, p+ms(830))
	sim.PrePull()
	// between ticks: the server takes it at p+600, and it lands at p+1100
	at(sim, p+ms(550), func(sim *Simulation) { slam.Cast(sim, a.CurrentTarget) })
	runUntil(sim, p+ms(1500))

	// 330 ms left at p+500, frozen p+600 to p+1000, gone below 0 at p+1400
	want := []string{fmt.Sprintf("slam %v", p+ms(1100)), fmt.Sprintf("mh %v", p+ms(1400))}
	if !slices.Equal(*events, want) {
		t.Errorf("got %v, want %v", *events, want)
	}
}

// testAPLAction runs execute whenever ready says so.
type testAPLAction struct {
	defaultAPLActionImpl
	ready   func(*Simulation) bool
	execute func(*Simulation)
}

func (action *testAPLAction) IsReady(sim *Simulation) bool { return action.ready(sim) }
func (action *testAPLAction) Execute(sim *Simulation)      { action.execute(sim) }
func (action *testAPLAction) String() string               { return "test action" }

// Slam started from the main hand's last-moment APL check goes before that swing: the server takes
// casts before melee, so the swing waits for Slam to land.
func TestSlamStartedAsTheSwingComesDueGoesFirst(t *testing.T) {
	var slam *Spell
	sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{
			MainHand:       testWeapon(2),
			AutoSwingMelee: true,
			ReplaceMHSwing: func(_ *Simulation, mhSwingSpell *Spell) *Spell { return mhSwingSpell },
		})
		slam = a.RegisterSpell(castConfig(testSpellSlam, ms(500)))
	})
	aa := &a.AutoAttacks
	events := logEffects(map[string]*Spell{"mh": aa.MHAuto(), "slam": slam})
	cast := false
	a.Rotation = &APLRotation{unit: &a.Unit, priorityList: []*APLAction{{impl: &testAPLAction{
		ready: func(sim *Simulation) bool { return !cast && sim.CurrentTime == ms(2000) },
		execute: func(sim *Simulation) {
			cast = true
			slam.Cast(sim, a.CurrentTarget)
		},
	}}}}
	aa.mh.setTimer(sim, ms(2000))
	sim.PrePull()
	runUntil(sim, ms(4900))

	want := []string{"slam 2.5s", "mh 2.5s", "mh 4.5s"}
	if !slices.Equal(*events, want) {
		t.Errorf("got %v, want %v", *events, want)
	}
}
