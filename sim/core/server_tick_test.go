package core

import (
	"slices"
	"testing"
	"time"

	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	RegisterAgentFactory(
		proto.Player_EnhancementShaman{},
		proto.Spec_SpecEnhancementShaman,
		func(char *Character, _ *proto.Player) Agent {
			return &timingTestAgent{Character: *char, setup: timingTestSetup}
		},
		func(player *proto.Player, spec interface{}) {
			player.Spec = spec.(*proto.Player_EnhancementShaman)
		},
	)
}

// timingTestSetup builds the next timing test agent's autos and spells.
var timingTestSetup func(*timingTestAgent)

type timingTestAgent struct {
	Character
	setup func(*timingTestAgent)
}

func (a *timingTestAgent) GetCharacter() *Character { return &a.Character }
func (a *timingTestAgent) Initialize() {
	if a.setup != nil {
		a.setup(a)
	}
}
func (a *timingTestAgent) ApplyTalents()            {}
func (a *timingTestAgent) Reset(_ *Simulation)      {}
func (a *timingTestAgent) OnGCDReady(_ *Simulation) {}

func testWeapon(swingSpeed float64) Weapon {
	return Weapon{BaseDamageMin: 1, BaseDamageMax: 1, SwingSpeed: swingSpeed, CritMultiplier: 2}
}

// newTimingTestSim builds a single player against one target with the given map update interval,
// resets it and queues the pull. Step it with runUntil.
func newTimingTestSim(t *testing.T, mapUpdateMs int32, setup func(*timingTestAgent)) (*Simulation, *timingTestAgent) {
	t.Helper()
	timingTestSetup = setup
	defer func() { timingTestSetup = nil }()

	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true},
		Raid: &proto.Raid{
			Parties: []*proto.Party{{
				Players: []*proto.Player{{
					Name:      "Timing",
					Class:     proto.Class_ClassShaman,
					Consumes:  &proto.Consumes{},
					Buffs:     &proto.IndividualBuffs{},
					Spec:      &proto.Player_EnhancementShaman{},
					Equipment: &proto.EquipmentSpec{},
				}},
				Buffs: &proto.PartyBuffs{},
			}},
		},
		Encounter: &proto.Encounter{
			Targets:        []*proto.Target{{Name: "target", Level: 83, MobType: proto.MobType_MobTypeDemon}},
			Duration:       60,
			ServerSettings: &proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(mapUpdateMs)},
		},
	})
	sim.reset()
	return sim, sim.Raid.Parties[0].Players[0].(*timingTestAgent)
}

// runUntil stops at end, so nothing after it runs.
func runUntil(sim *Simulation, end time.Duration) {
	at(sim, end, func(*Simulation) {})
	for sim.CurrentTime < end {
		if sim.Step() {
			return
		}
	}
}

func at(sim *Simulation, doAt time.Duration, action func(sim *Simulation)) {
	sim.AddPendingAction(&PendingAction{NextActionAt: doAt, OnAction: action})
}

// recordCasts replaces the spell's effects with a log of when it went off.
func recordCasts(spell *Spell) *[]time.Duration {
	var times []time.Duration
	spell.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		times = append(times, sim.CurrentTime)
	}
	return &times
}

func ms(n int) time.Duration {
	return time.Duration(n) * time.Millisecond
}

func TestNextServerTick(t *testing.T) {
	sim := &Simulation{serverTickInterval: ms(100), serverTickPhase: ms(30)}
	for _, tc := range []struct{ in, want time.Duration }{
		{0, ms(30)},
		{ms(30), ms(30)},
		{ms(31), ms(130)},
		{ms(130), ms(130)},
		{ms(2629), ms(2630)},
		{ms(-50), ms(30)},
		{ms(-70), ms(-70)},
		{ms(-71), ms(-70)},
		{NeverExpires, NeverExpires},
	} {
		if got := sim.NextServerTick(tc.in); got != tc.want {
			t.Errorf("NextServerTick(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}

	exact := &Simulation{}
	if got := exact.NextServerTick(ms(1234)); got != ms(1234) {
		t.Errorf("exact NextServerTick(1.234s) = %v, want 1.234s", got)
	}
}

func TestServerTickPhaseIsRolledPerIteration(t *testing.T) {
	sim, _ := newTimingTestSim(t, 100, nil)
	phases := map[time.Duration]bool{}
	for i := int64(0); i < 20; i++ {
		sim.reseedRands(i)
		sim.reset()
		if sim.serverTickPhase < 0 || sim.serverTickPhase >= ms(100) {
			t.Fatalf("phase %v outside [0, 100ms)", sim.serverTickPhase)
		}
		phases[sim.serverTickPhase] = true
	}
	if len(phases) < 10 {
		t.Errorf("only %d distinct phases over 20 iterations", len(phases))
	}
}

func TestSwingsLandOnServerTicks(t *testing.T) {
	for _, tc := range []struct {
		mapUpdateMs int32
		interval    time.Duration
	}{
		{100, ms(2600)}, // 2.55 s rounds up to the next tick every swing
		{0, ms(2550)},
	} {
		sim, a := newTimingTestSim(t, tc.mapUpdateMs, func(a *timingTestAgent) {
			a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2.55), AutoSwingMelee: true})
		})
		swings := recordCasts(a.AutoAttacks.MHAuto())
		sim.PrePull()
		runUntil(sim, 30*time.Second)

		if len(*swings) < 10 {
			t.Fatalf("%d ms: only %d swings", tc.mapUpdateMs, len(*swings))
		}
		if first := (*swings)[0]; first != sim.NextServerTick(0) {
			t.Errorf("%d ms: first swing at %v, want %v", tc.mapUpdateMs, first, sim.NextServerTick(0))
		}
		for i, swing := range *swings {
			if sim.NextServerTick(swing) != swing {
				t.Errorf("%d ms: swing at %v is off the server tick", tc.mapUpdateMs, swing)
			}
			if i > 0 && swing-(*swings)[i-1] != tc.interval {
				t.Errorf("%d ms: swing interval %v, want %v", tc.mapUpdateMs, swing-(*swings)[i-1], tc.interval)
			}
		}
	}
}

func TestHasteRescalesTheSwingTimerNotTheTick(t *testing.T) {
	sim, a := newTimingTestSim(t, 100, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), AutoSwingMelee: true})
	})
	swings := recordCasts(a.AutoAttacks.MHAuto())
	phase := sim.serverTickPhase
	sim.PrePull()
	// halfway to the second swing, which is 1 s of timer away: doubling the speed leaves 0.5 s
	at(sim, phase+time.Second, func(sim *Simulation) { a.MultiplyMeleeSpeed(sim, 2) })
	runUntil(sim, 3*time.Second)

	want := []time.Duration{phase, phase + ms(1500), phase + ms(2500)}
	if !slices.Equal((*swings)[:3], want) {
		t.Errorf("swings at %v, want %v", (*swings)[:3], want)
	}
}

func TestOtherHandPushedBackBySwing(t *testing.T) {
	for _, tc := range []struct {
		name         string
		mh, oh       time.Duration
		reverseOrder bool
		wantMH       []time.Duration
		wantOH       []time.Duration
	}{
		{"off hand due 100ms later", ms(1000), ms(1100), false, []time.Duration{ms(1000), ms(3000)}, []time.Duration{ms(1200), ms(3200)}},
		{"same moment", ms(1000), ms(1000), false, []time.Duration{ms(1000), ms(3000)}, []time.Duration{ms(1200), ms(3200)}},
		{"same moment, off hand first in the list", ms(1000), ms(1000), true, []time.Duration{ms(1000), ms(3000)}, []time.Duration{ms(1200), ms(3200)}},
		{"off hand first pushes the main hand", ms(1100), ms(1000), false, []time.Duration{ms(1200), ms(3200)}, []time.Duration{ms(1000), ms(3000)}},
		{"far enough apart", ms(1000), ms(1500), false, []time.Duration{ms(1000), ms(3000)}, []time.Duration{ms(1500), ms(3500)}},
	} {
		sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
			a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), OffHand: testWeapon(2), AutoSwingMelee: true})
		})
		aa := &a.AutoAttacks
		mhSwings, ohSwings := recordCasts(aa.MHAuto()), recordCasts(aa.OHAuto())
		aa.mh.setTimer(sim, tc.mh)
		aa.oh.setTimer(sim, tc.oh)
		sim.PrePull()
		if tc.reverseOrder {
			at(sim, ms(500), func(sim *Simulation) {
				slices.Reverse(sim.weaponAttacks)
			})
		}
		runUntil(sim, ms(3900))

		if !slices.Equal(*mhSwings, tc.wantMH) || !slices.Equal(*ohSwings, tc.wantOH) {
			t.Errorf("%s: main hand %v, off hand %v; want %v, %v", tc.name, *mhSwings, *ohSwings, tc.wantMH, tc.wantOH)
		}
	}
}

func TestMeleeSwingResetsRangedTimer(t *testing.T) {
	sim, a := newTimingTestSim(t, 0, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{
			MainHand:        testWeapon(2),
			Ranged:          testWeapon(3),
			AutoSwingMelee:  true,
			AutoSwingRanged: true,
		})
	})
	aa := &a.AutoAttacks
	shots := recordCasts(aa.RangedAuto())
	recordCasts(aa.MHAuto())
	aa.mh.setTimer(sim, ms(1000))
	aa.ranged.setTimer(sim, ms(1500))
	sim.PrePull()
	runUntil(sim, ms(3500))

	if len(*shots) != 0 {
		t.Errorf("ranged shots at %v, want none: every melee swing restarts the ranged timer", *shots)
	}
	// the swing at 3 s restarted it last
	if want := ms(6000); aa.ranged.swingAt != want {
		t.Errorf("next ranged shot at %v, want %v", aa.ranged.swingAt, want)
	}
}

func TestAuraExpiresOnServerTick(t *testing.T) {
	for _, mapUpdateMs := range []int32{100, 0} {
		var aura *Aura
		sim, _ := newTimingTestSim(t, mapUpdateMs, func(a *timingTestAgent) {
			aura = a.RegisterAura(Aura{Label: "timed", Duration: ms(1050)})
		})
		sim.PrePull()
		start := ms(420)
		want := sim.NextServerTick(start + ms(1050))
		at(sim, start, func(sim *Simulation) {
			aura.Activate(sim)
			if aura.ExpiresAt() != want {
				t.Errorf("%d ms: expires at %v, want %v", mapUpdateMs, aura.ExpiresAt(), want)
			}
		})
		at(sim, want-1, func(sim *Simulation) {
			if !aura.IsActive() {
				t.Errorf("%d ms: gone before its server tick %v", mapUpdateMs, want)
			}
		})
		at(sim, want, func(sim *Simulation) {
			if aura.IsActive() {
				t.Errorf("%d ms: still active at %v", mapUpdateMs, want)
			}
		})
		runUntil(sim, want+ms(1))
	}
}

func TestHardcastLandsOnServerTick(t *testing.T) {
	for _, mapUpdateMs := range []int32{100, 0} {
		var spell *Spell
		sim, _ := newTimingTestSim(t, mapUpdateMs, func(a *timingTestAgent) {
			spell = a.RegisterSpell(SpellConfig{
				ActionID: ActionID{SpellID: testSpellNoServerData},
				Cast: CastConfig{
					DefaultCast: Cast{GCD: GCDDefault, CastTime: ms(1250)},
					CD:          Cooldown{Timer: a.NewTimer(), Duration: 10 * time.Second},
				},
			})
		})
		casts := recordCasts(spell)
		sim.PrePull()
		at(sim, ms(510), func(sim *Simulation) { spell.Cast(sim, nil) })
		runUntil(sim, 3*time.Second)

		want := sim.NextServerTick(ms(1760))
		if !slices.Equal(*casts, []time.Duration{want}) {
			t.Errorf("%d ms: cast landed at %v, want %v", mapUpdateMs, *casts, want)
		}
		if got := spell.CD.ReadyAt(); got != want+10*time.Second {
			t.Errorf("%d ms: CD ready at %v, want %v: it starts when the cast lands", mapUpdateMs, got, want+10*time.Second)
		}
	}
}

func TestChannelExpiryStaysExact(t *testing.T) {
	var channel *Spell
	newTimingTestSim(t, 100, func(a *timingTestAgent) {
		channel = a.RegisterSpell(SpellConfig{
			ActionID: ActionID{SpellID: 48156},
			Flags:    SpellFlagChanneled,
			Cast:     CastConfig{DefaultCast: Cast{GCD: GCDDefault}},
			Dot: DotConfig{
				Aura:          Aura{Label: "channel"},
				NumberOfTicks: 3,
				TickLength:    time.Second,
				OnTick:        func(*Simulation, *Unit, *Dot) {},
			},
		})
	})
	if dot := channel.CurDot(); !dot.Aura.exactExpiry {
		t.Error("channel's dot aura would expire on the server tick after its last tick")
	}
}
