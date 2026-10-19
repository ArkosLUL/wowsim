package core

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

const testExtraAttackID = 9900021

func extraAttackTestSpell(t *testing.T, a *timingTestAgent, tag int32) *Spell {
	t.Helper()
	if serverdata.SpellByID(testExtraAttackID) != nil {
		t.Fatalf("test spell %d has server data", testExtraAttackID)
	}
	return a.RegisterSpell(SpellConfig{
		ActionID:     ActionID{SpellID: testExtraAttackID, Tag: tag},
		ProcMask:     ProcMaskMeleeMHAuto,
		Flags:        SpellFlagNoOnCastComplete,
		ApplyEffects: func(*Simulation, *Unit, *Spell) {},
	})
}

// The unit's next Unit::Update swings an extra attack: one off its own swing a world tick later, one off
// anything before that update's turn on that update, and one off an extra attack along with it.
func TestExtraAttackSwingsOnTheNextUpdate(t *testing.T) {
	var extra *Spell
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), AutoSwingMelee: true})
		extra = extraAttackTestSpell(t, a, 0)
	})
	target := sim.Encounter.TargetUnits[0]
	var swings, extras []time.Duration
	a.AutoAttacks.MHAuto().ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		swings = append(swings, sim.CurrentTime)
		if len(swings) == 1 {
			a.AutoAttacks.QueueExtraAttack(sim, extra, target, false)
		}
	}
	extra.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		extras = append(extras, sim.CurrentTime)
		if len(extras) == 1 {
			a.AutoAttacks.QueueExtraAttack(sim, extra, target, false)
		}
	}
	sim.PrePull()
	tick := sim.NextWorldTick(ms(1000))
	at(sim, tick-ms(3), func(sim *Simulation) { a.AutoAttacks.QueueExtraAttack(sim, extra, target, false) })
	runUntil(sim, ms(1500))

	if len(swings) != 1 {
		t.Fatalf("swings at %v, want one", swings)
	}
	first := sim.NextWorldTick(swings[0] + 1)
	if want := []time.Duration{first, first, tick}; !slices.Equal(extras, want) {
		t.Errorf("extra attacks at %v, want %v (the swing at %v)", extras, want, swings[0])
	}
}

// On the update an extra attack goes, it comes after the unit's auras and before its melee, the off hand's
// swing due then included.
func TestExtraAttackGoesBetweenTheAurasAndTheMelee(t *testing.T) {
	var extra *Spell
	var aura *Aura
	var order []string
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(2), OffHand: testWeapon(2), AutoSwingMelee: true})
		extra = extraAttackTestSpell(t, a, 0)
		aura = a.RegisterAura(Aura{
			Label:    "expiring",
			Duration: time.Second,
			OnExpire: func(*Aura, *Simulation) { order = append(order, "aura") },
		})
	})
	target := sim.Encounter.TargetUnits[0]
	extra.ApplyEffects = func(*Simulation, *Unit, *Spell) { order = append(order, "extra") }
	a.AutoAttacks.OHAuto().ApplyEffects = func(*Simulation, *Unit, *Spell) { order = append(order, "off hand") }
	sim.PrePull()
	var offHandAt time.Duration
	at(sim, ms(105), func(sim *Simulation) {
		offHandAt = a.AutoAttacks.OffhandSwingAt()
		aura.Duration = offHandAt - sim.WorldTickBefore(sim.CurrentTime)
		aura.Activate(sim)
		if aura.ExpiresAt() != offHandAt {
			t.Fatalf("the aura runs out at %v, the off hand swings at %v", aura.ExpiresAt(), offHandAt)
		}
		at(sim, offHandAt-ms(3), func(sim *Simulation) { a.AutoAttacks.QueueExtraAttack(sim, extra, target, false) })
	})
	runUntil(sim, ms(1500))

	if want := []string{"aura", "extra", "off hand"}; !slices.Equal(order, want) {
		t.Errorf("went in the order %v at %v, want %v", order, offHandAt, want)
	}
}

// A pass for another player's Auto Shot skips this player's melee, its extra attack due then included, which still
// goes on that update.
func TestExtraAttackSwingsBesideAnotherPlayersAutoShot(t *testing.T) {
	var extra *Spell
	var aura *Aura
	timingTestSetup = func(a *timingTestAgent) {
		if a.Name == "shooter" {
			a.EnableAutoAttacks(a, AutoAttackOptions{Ranged: testWeapon(3), AutoSwingRanged: true})
			return
		}
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(3.6), AutoSwingMelee: true})
		extra = extraAttackTestSpell(t, a, 0)
		aura = a.RegisterAura(Aura{Label: "expiring", Duration: time.Second})
	}
	defer func() { timingTestSetup = nil }()
	player := func(name string) *proto.Player {
		return &proto.Player{Name: name, Class: proto.Class_ClassShaman, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
			Spec: &proto.Player_EnhancementShaman{}, Equipment: &proto.EquipmentSpec{}}
	}
	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true},
		Raid: &proto.Raid{Parties: []*proto.Party{{
			Players: []*proto.Player{player("melee"), player("shooter")},
			Buffs:   &proto.PartyBuffs{},
		}}},
		Encounter: &proto.Encounter{
			Targets:  []*proto.Target{{Name: "target", Level: 83, MobType: proto.MobType_MobTypeDemon}},
			Duration: 60,
		},
	})
	sim.reset()
	melee := sim.Raid.Parties[0].Players[0].(*timingTestAgent)
	shooter := sim.Raid.Parties[0].Players[1].(*timingTestAgent)
	target := sim.Encounter.TargetUnits[0]
	var extras []time.Duration
	extra.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) { extras = append(extras, sim.CurrentTime) }
	sim.PrePull()
	var shotAt time.Duration
	at(sim, ms(1005), func(sim *Simulation) {
		// the aura makes the shot's pass a players' Auto Shot pass, ahead of the auras
		shotAt = shooter.AutoAttacks.ranged.swingAt
		aura.Duration = shotAt - sim.WorldTickBefore(sim.CurrentTime)
		aura.Activate(sim)
		if aura.ExpiresAt() != shotAt || melee.AutoAttacks.mh.swingAt <= shotAt {
			t.Fatalf("the aura runs out at %v, the main hand swings at %v, the shot at %v", aura.ExpiresAt(), melee.AutoAttacks.mh.swingAt, shotAt)
		}
		at(sim, shotAt-ms(3), func(sim *Simulation) { melee.AutoAttacks.QueueExtraAttack(sim, extra, target, false) })
	})
	runUntil(sim, ms(6000))

	if len(extras) == 0 || extras[0] != shotAt {
		t.Errorf("extra attacks at %v, want the first at %v", extras, shotAt)
	}
}

// AuraEffect::CheckEffectProc: no extra attack procs while it's the last one queued, and none at all after Sword
// Specialization's, its own swing included, till the pass that swings them is over.
func TestExtraAttackProcGate(t *testing.T) {
	var first, last *Spell
	sim, a := newTickTestSim(t, 100, 10, func(a *timingTestAgent) {
		a.EnableAutoAttacks(a, AutoAttackOptions{MainHand: testWeapon(3.6), AutoSwingMelee: true})
		first = extraAttackTestSpell(t, a, 0)
		last = extraAttackTestSpell(t, a, 1)
	})
	target := sim.Encounter.TargetUnits[0]
	aa := &a.AutoAttacks
	check := func(when string, wantFirst, wantLast bool) {
		if got := aa.CanProcExtraAttack(first); got != wantFirst {
			t.Errorf("%s: the first can proc %v, want %v", when, got, wantFirst)
		}
		if got := aa.CanProcExtraAttack(last); got != wantLast {
			t.Errorf("%s: the chain's last can proc %v, want %v", when, got, wantLast)
		}
	}
	var swung []string
	first.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		swung = append(swung, "first at "+sim.CurrentTime.String())
		check("in the first's swing", false, true)
		aa.QueueExtraAttack(sim, last, target, true)
	}
	last.ApplyEffects = func(sim *Simulation, _ *Unit, _ *Spell) {
		swung = append(swung, "last at "+sim.CurrentTime.String())
		check("in the last's swing", false, false)
	}
	sim.PrePull()
	tick := sim.NextWorldTick(ms(1000))
	at(sim, tick-ms(3), func(sim *Simulation) {
		check("before any", true, true)
		aa.QueueExtraAttack(sim, first, target, false)
		check("queued", false, true)
	})
	at(sim, tick+ms(3), func(*Simulation) { check("after the pass", true, true) })
	runUntil(sim, ms(1500))

	if want := []string{"first at " + tick.String(), "last at " + tick.String()}; !slices.Equal(swung, want) {
		t.Errorf("swung %v, want %v", swung, want)
	}
}
