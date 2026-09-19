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
		timing := newCastTiming(&Spell{ActionID: ActionID{SpellID: tc.spellID}})
		unit := &Unit{CastSpeed: tc.castSpeed}
		if got := timing.gcd(unit, tc.gcd, tc.ignoreHaste); got != tc.want {
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
		spell := &Spell{ActionID: ActionID{SpellID: tc.spellID}, CastTimeMultiplier: 1}
		unit.CastSpeed = 0.8
		if tc.spellID == testSpellSteadyShot {
			unit.CastSpeed = 0.5 // must not matter
		}
		timing := newCastTiming(spell)
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
		{"Maelstrom Weapon up", castConfig(testSpellLightningBolt, ms(1500)), func(a *timingTestAgent, _ *Spell) {
			mw := a.RegisterAura(Aura{Label: "Maelstrom Weapon", ActionID: ActionID{SpellID: testSpellMaelstromWeapon}, Duration: time.Minute})
			a.RegisterResetEffect(func(sim *Simulation) {
				at(sim, ms(100), func(sim *Simulation) { mw.Activate(sim) })
			})
		}, noReset},
		{"Maelstrom Weapon down", castConfig(testSpellLightningBolt, ms(1500)), nil, []time.Duration{0, ms(4000), ms(6000)}},
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
	if got := newCastTiming(&Spell{ActionID: ActionID{SpellID: testSpellLightningBolt}}).ignoreResetAuras; !slices.Equal(got, []ActionID{{SpellID: testSpellMaelstromWeapon}}) {
		t.Errorf("Lightning Bolt's IGNORE_MELEE_RESET auras = %v, want Maelstrom Weapon", got)
	}
	if got := newCastTiming(&Spell{ActionID: ActionID{SpellID: testSpellExorcism}}).ignoreResetAuras; len(got) != 0 {
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
