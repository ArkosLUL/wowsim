package core

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func init() {
	RegisterAgentFactory(
		proto.Player_ElementalShaman{},
		proto.Spec_SpecElementalShaman,
		NewFakeElementalShaman,
		func(player *proto.Player, spec interface{}) {
			playerSpec, ok := spec.(*proto.Player_ElementalShaman)
			if !ok {
				panic("Invalid spec value for Elemental Shaman!")
			}
			player.Spec = playerSpec
		},
	)
}

// stackingDotSpellID is Holy Vengeance, which the server stacks to 5.
const stackingDotSpellID = 31803

type FakeAgent struct {
	Spell *Spell
	Dot   *Dot
	// the same dot under a spell that stacks
	StackingDot *Dot
	Character
	Init func()
}

func (fa *FakeAgent) GetCharacter() *Character {
	return &fa.Character
}

func (fa *FakeAgent) Initialize() {
	if fa.Init != nil {
		fa.Init()
	}
}

func (fa *FakeAgent) ApplyTalents()            {}
func (fa *FakeAgent) Reset(_ *Simulation)      {}
func (fa *FakeAgent) OnGCDReady(_ *Simulation) {}

func NewFakeElementalShaman(char *Character, _ *proto.Player) Agent {
	fa := &FakeAgent{
		Character: *char,
	}

	fa.Init = func() {
		fa.Spell = fa.RegisterSpell(SpellConfig{
			ActionID:    ActionID{SpellID: 42},
			SpellSchool: SpellSchoolShadow,
			ProcMask:    ProcMaskSpellDamage,
			Flags:       SpellFlagIgnoreResists,
			Cast:        CastConfig{},

			BonusCritRating:  3 * CritRatingPerCritChance,
			DamageMultiplier: 1.5,
			ThreatMultiplier: 1,

			Dot: DotConfig{
				Aura: Aura{
					Label: "fakedot",
				},
				NumberOfTicks:       6,
				TickLength:          time.Second * 3,
				AffectedByCastSpeed: true,
				OnSnapshot: func(sim *Simulation, target *Unit, dot *Dot, isRollover bool) {
					dot.SnapshotBaseDamage = 100 + 1*dot.Spell.SpellPower()
					if !isRollover {
						attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex]
						dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable)
					}
				},
				OnTick: func(sim *Simulation, target *Unit, dot *Dot) {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
				},
			},

			ApplyEffects: func(sim *Simulation, target *Unit, spell *Spell) {
				result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
				if result.Landed() {
					spell.Dot(target).Apply(sim)
				}
				spell.DealOutcome(sim, result)
			},
		})
		fa.Dot = fa.Spell.CurDot()

		fa.StackingDot = fa.RegisterSpell(SpellConfig{
			ActionID:         ActionID{SpellID: stackingDotSpellID},
			SpellSchool:      SpellSchoolHoly,
			ProcMask:         ProcMaskSpellDamage,
			DamageMultiplier: 1,
			ThreatMultiplier: 1,
			Dot: DotConfig{
				Aura:          Aura{Label: "stackingdot"},
				NumberOfTicks: 5,
				TickLength:    time.Second * 3,
				OnTick: func(sim *Simulation, target *Unit, dot *Dot) {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
				},
			},
		}).CurDot()
	}

	return fa
}

func SetupFakeSim() *Simulation {
	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{
			RandomSeed: 100,
		},
		Raid: &proto.Raid{
			Parties: []*proto.Party{
				{
					Players: []*proto.Player{
						{
							Name:      "Caster",
							Class:     proto.Class_ClassShaman,
							Consumes:  &proto.Consumes{},
							Buffs:     &proto.IndividualBuffs{},
							Spec:      &proto.Player_ElementalShaman{},
							Equipment: &proto.EquipmentSpec{},
						},
					},
					Buffs: &proto.PartyBuffs{},
				},
			},
		},
		Encounter: &proto.Encounter{
			Targets: []*proto.Target{
				{Name: "target", Level: 83, MobType: proto.MobType_MobTypeDemon},
			},
			Duration: 180,
		},
	})
	sim.Reset()

	return sim
}

func expectDotTickDamage(t *testing.T, sim *Simulation, dot *Dot, expectedDamage float64) {
	damageBefore := dot.Spell.SpellMetrics[0].TotalDamage
	dot.TickOnce(sim)
	damageAfter := dot.Spell.SpellMetrics[0].TotalDamage
	delta := damageAfter - damageBefore

	if !WithinToleranceFloat64(expectedDamage, delta, 0.01) {
		t.Fatalf("Incorrect tick damage applied: Expected: %0.3f, Actual: %0.3f", expectedDamage, delta)
	}
}

func TestDotSnapshot(t *testing.T) {
	sim := SetupFakeSim()
	fa := sim.Raid.Parties[0].Players[0].(*FakeAgent)

	fa.Dot.Apply(sim)
	expectDotTickDamage(t, sim, fa.Dot, 150) // (100) * 1.5

	fa.Dot.Rollover(sim)
	expectDotTickDamage(t, sim, fa.Dot, 150) // (100) * 1.5
}

func TestDotSnapshotSpellPower(t *testing.T) {
	sim := SetupFakeSim()
	fa := sim.Raid.Parties[0].Players[0].(*FakeAgent)

	fa.Dot.Apply(sim)
	expectDotTickDamage(t, sim, fa.Dot, 150) // (100) * 1.5

	// Spell power shouldn't get applied because dot was already snapshot.
	fa.GetCharacter().AddStatDynamic(sim, stats.SpellPower, 100)
	expectDotTickDamage(t, sim, fa.Dot, 150) // (100) * 1.5

	fa.Dot.Deactivate(sim)
	fa.Dot.Apply(sim)
	expectDotTickDamage(t, sim, fa.Dot, 300) // (100 + 100) * 1.5
}

func TestDotSnapshotSpellMultiplier(t *testing.T) {
	sim := SetupFakeSim()
	fa := sim.Raid.Parties[0].Players[0].(*FakeAgent)
	spell := fa.GetCharacter().Spellbook[0]
	spell.DamageMultiplier *= 2

	fa.Dot.Apply(sim)
	expectDotTickDamage(t, sim, fa.Dot, 300) // (100) * 1.5 * 2

	fa.Dot.Rollover(sim)
	expectDotTickDamage(t, sim, fa.Dot, 300) // (100) * 1.5 * 2
}

func TestDotRefreshResetsTheTickTimerOnlyBelowTwoStacks(t *testing.T) {
	for _, tc := range []struct {
		name   string
		pick   func(fa *FakeAgent) *Dot
		resets bool
	}{
		{"spell that doesn't stack", func(fa *FakeAgent) *Dot { return fa.Dot }, true},
		{"spell that stacks to 5", func(fa *FakeAgent) *Dot { return fa.StackingDot }, false},
	} {
		sim := SetupFakeSim()
		dot := tc.pick(sim.Raid.Parties[0].Players[0].(*FakeAgent))

		dot.Apply(sim)
		firstTick := dot.NextTickAt()

		sim.CurrentTime = time.Second
		dot.Apply(sim)

		want := firstTick
		if tc.resets {
			want = sim.NextServerTick(sim.CurrentTime + dot.TickPeriod())
		}
		if got := dot.NextTickAt(); got != want {
			t.Errorf("%s: next tick at %v after reapplying, want %v", tc.name, got, want)
		}
		if got, want := dot.ExpiresAt(), sim.NextServerTick(sim.CurrentTime+dot.Aura.Duration); got != want {
			t.Errorf("%s: expires at %v, want %v: a refresh always renews the duration", tc.name, got, want)
		}
	}
}

func TestDotTickHasteModes(t *testing.T) {
	castSpeed := func(mult float64) func(*Simulation, *FakeAgent) {
		return func(_ *Simulation, fa *FakeAgent) { fa.MultiplyCastSpeed(mult) }
	}
	meleeSpeed := func(mult float64) func(*Simulation, *FakeAgent) {
		return func(sim *Simulation, fa *FakeAgent) { fa.MultiplyMeleeSpeed(sim, mult) }
	}

	for _, tc := range []struct {
		name       string
		mode       TickHaste
		haste      func(*Simulation, *FakeAgent)
		wantPeriod time.Duration
		wantDur    time.Duration
		wantTicks  int32
	}{
		{"spell haste scales both", SpellHasteScalesBoth, castSpeed(1.25), ms(2400), ms(12000), 5},
		{"spell haste adds ticks", SpellHasteAddsTicks, castSpeed(1.25), ms(2400), ms(15000), 6},
		{"melee haste adds ticks", MeleeHasteAddsTicks, meleeSpeed(1.25), ms(2400), ms(15000), 6},
		{"a slow leaves the ticks alone", SpellHasteAddsTicks, castSpeed(0.8), ms(3000), ms(15000), 5},
	} {
		sim := SetupFakeSim()
		fa := sim.Raid.Parties[0].Players[0].(*FakeAgent)
		dot := fa.Dot
		dot.NumberOfTicks = 5 // 5 x 3 s = 15 s
		dot.TickHaste = tc.mode
		tc.haste(sim, fa)

		dot.Apply(sim)

		if got := dot.TickPeriod(); got != tc.wantPeriod {
			t.Errorf("%s: tick period %v, want %v", tc.name, got, tc.wantPeriod)
		}
		if got := dot.Aura.Duration; got != tc.wantDur {
			t.Errorf("%s: duration %v, want %v", tc.name, got, tc.wantDur)
		}
		if got := dot.TotalTicks(); got != tc.wantTicks {
			t.Errorf("%s: %d ticks, want %d", tc.name, got, tc.wantTicks)
		}
	}
}

func TestTicksCanCritGatesTheCritRoll(t *testing.T) {
	sim := SetupFakeSim()
	dot := sim.Raid.Parties[0].Players[0].(*FakeAgent).Dot
	dot.SnapshotCritChance = 1
	dot.Spell.CritMultiplier = 2

	defer func(old bool) { periodicCritsNeedDeclaration = old }(periodicCritsNeedDeclaration)
	for _, tc := range []struct {
		needsDeclaration, declared, wantCrit bool
	}{
		{false, false, true}, // what the sim does today
		{true, false, false},
		{true, true, true},
	} {
		periodicCritsNeedDeclaration = tc.needsDeclaration
		dot.TicksCanCrit = tc.declared

		result := &SpellResult{Target: dot.Unit, Damage: 100}
		dot.OutcomeSnapshotCrit(sim, result, nil)

		if got := result.DidCrit(); got != tc.wantCrit {
			t.Errorf("declaration needed %v, declared %v: crit %v, want %v", tc.needsDeclaration, tc.declared, got, tc.wantCrit)
		}
	}
}
