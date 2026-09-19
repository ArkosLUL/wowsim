package core

import (
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
		{"hardcast without the flag", castConfig(testSpellSlam, ms(1500)), nil, noReset},
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
		}, noReset},
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
