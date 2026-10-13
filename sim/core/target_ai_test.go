package core

import (
	"slices"
	"testing"
	"time"

	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim/core/proto"
)

const (
	hardcastAITestBossID     = 900001
	dungeonScaleAITestBossID = 900002
)

// hardcastAITest casts one spell whose cast time leaves Hardcast.Expires equal to NextGCDAt (no
// separate GCD beyond the cast, as Hodir's Flash Freeze has none), the case a Target's gcdAction used
// to never complete.
type hardcastAITest struct {
	spell  *Spell
	casted bool
	landed bool
}

func newHardcastAITest() AIFactory {
	return func() TargetAI { return &hardcastAITest{} }
}

func (ai *hardcastAITest) Initialize(target *Target, _ *proto.Target) {
	ai.spell = target.GetOrRegisterSpell(SpellConfig{
		ActionID:    ActionID{SpellID: 1},
		SpellSchool: SpellSchoolPhysical,
		ProcMask:    ProcMaskSpellDamage,

		Cast: CastConfig{
			DefaultCast: Cast{CastTime: time.Millisecond * 500},
		},

		DamageMultiplier: 1,
		CritMultiplier:   1,

		ApplyEffects: func(_ *Simulation, _ *Unit, _ *Spell) {
			ai.landed = true
		},
	})
}

func (ai *hardcastAITest) Reset(*Simulation) {
	ai.casted = false
	ai.landed = false
}

func (ai *hardcastAITest) ExecuteCustomRotation(sim *Simulation) {
	if ai.casted {
		return
	}
	ai.casted = true
	ai.spell.Cast(sim, nil)
}

// dungeonScaleAITest registers one spell flagged SpellFlagIgnoreModifiers, as Anub'arak's Leeching
// Swarm is, to check the dungeon scale Damage multiplier still reaches it.
type dungeonScaleAITest struct {
	spell *Spell
}

func newDungeonScaleAITest() AIFactory {
	return func() TargetAI { return &dungeonScaleAITest{} }
}

func (ai *dungeonScaleAITest) Initialize(target *Target, _ *proto.Target) {
	ai.spell = target.GetOrRegisterSpell(SpellConfig{
		ActionID:    ActionID{SpellID: 2},
		SpellSchool: SpellSchoolNature,
		ProcMask:    ProcMaskSpellDamage,
		Flags:       SpellFlagIgnoreModifiers,

		DamageMultiplier: 1,
		CritMultiplier:   1,

		ApplyEffects: func(*Simulation, *Unit, *Spell) {},
	})
}

func (ai *dungeonScaleAITest) Reset(*Simulation) {}

func (ai *dungeonScaleAITest) ExecuteCustomRotation(*Simulation) {}

func init() {
	AddPresetTarget(&PresetTarget{
		PathPrefix: "TargetAITest",
		Config: &proto.Target{
			Id:    hardcastAITestBossID,
			Name:  "Hardcast Boss",
			Level: CharacterLevel + 3,
		},
		AI: newHardcastAITest(),
	})
	AddPresetTarget(&PresetTarget{
		PathPrefix: "TargetAITest",
		Config: &proto.Target{
			Id:    dungeonScaleAITestBossID,
			Name:  "Dungeon Scale Boss",
			Level: CharacterLevel + 3,
		},
		AI: newDungeonScaleAITest(),
	})

	AddPresetEncounter("Hardcast Boss", []string{"TargetAITest/Hardcast Boss"})
	AddPresetEncounterWithDifficulty("Dungeon Scale Boss", []string{"TargetAITest/Dungeon Scale Boss"}, proto.RaidDifficulty_RaidDifficulty10Normal)
}

func findPresetEncounter(path string) *proto.PresetEncounter {
	for _, preset := range PresetEncounters {
		if preset.Path == path {
			return preset
		}
	}
	return nil
}

// AddPresetEncounter leaves the field at its zero value, and AddPresetEncounterWithDifficulty carries
// whatever a class item picks for a preset built for a specific raid size.
func TestPresetEncounterRaidDifficulty(t *testing.T) {
	if preset := findPresetEncounter("TargetAITest/Hardcast Boss"); preset == nil {
		t.Fatal("AddPresetEncounter didn't register the encounter")
	} else if preset.RaidDifficulty != proto.RaidDifficulty_RaidDifficultyUnknown {
		t.Errorf("RaidDifficulty = %v, want RaidDifficultyUnknown", preset.RaidDifficulty)
	}

	if preset := findPresetEncounter("TargetAITest/Dungeon Scale Boss"); preset == nil {
		t.Fatal("AddPresetEncounterWithDifficulty didn't register the encounter")
	} else if preset.RaidDifficulty != proto.RaidDifficulty_RaidDifficulty10Normal {
		t.Errorf("RaidDifficulty = %v, want RaidDifficulty10Normal", preset.RaidDifficulty)
	}
}

func newTargetAITestRaidSimRequest(target *proto.Target, server *proto.ServerSettings) *proto.RaidSimRequest {
	return &proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 1},
		Raid: &proto.Raid{
			Parties:       []*proto.Party{{}},
			TargetDummies: 1,
			Tanks:         []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}},
		},
		Encounter: &proto.Encounter{
			Duration:       60,
			Targets:        []*proto.Target{target},
			ServerSettings: server,
		},
	}
}

// A hardcast that only ever shares its pending action with the GCD (nothing else schedules a GCD
// beyond the cast) used to never apply its effects: the Target's gcdAction, unlike a player's, didn't
// check for a completed Hardcast before moving on to the next rotation action.
func TestTargetHardcastCompletesOnSharedGCDAction(t *testing.T) {
	sim := NewSim(newTargetAITestRaidSimRequest(&proto.Target{Id: hardcastAITestBossID, Level: CharacterLevel + 3}, nil))
	sim.Reset()

	boss := sim.Encounter.Targets[0]
	ai := boss.AI.(*hardcastAITest)
	if ai.casted {
		t.Fatal("the boss cast before the sim ran")
	}

	for !sim.Step() {
	}

	if !ai.casted {
		t.Fatal("the boss never cast its hardcast")
	}
	if !ai.landed {
		t.Error("the hardcast's effects never landed: the Target's gcdAction skipped its completed Hardcast")
	}
}

// Dungeon scale's Damage multiplier lives on PseudoStats.DamageDealtMultiplier, which
// SpellFlagIgnoreAttackerModifiers skips outright in spell_result.go. The server's DungeonScale hooks
// scale a creature's damage regardless of the spell's attributes, so the scale must still reach it.
func TestTargetDungeonScaleReachesIgnoreAttackerModifiersSpells(t *testing.T) {
	dealt := func(damageMult float64) float64 {
		server := unscaledRaid(&proto.DungeonScaleSettings{Raid25Boss: modifiers(1, 1, 1, damageMult)})
		sim := NewSim(newTargetAITestRaidSimRequest(&proto.Target{Id: dungeonScaleAITestBossID, Level: CharacterLevel + 3}, server))
		sim.Reset()

		boss := sim.Encounter.Targets[0]
		ai := boss.AI.(*dungeonScaleAITest)
		tank := boss.CurrentTarget
		if tank == nil {
			t.Fatal("the boss has no one to hit")
		}
		return ai.spell.CalcAndDealDamage(sim, tank, 1000, ai.spell.OutcomeAlwaysHit).Damage
	}

	base := dealt(1)
	scaled := dealt(1.5)
	if base <= 0 {
		t.Fatalf("base damage was %v, want > 0", base)
	}
	if want := base * 1.5; scaled != want {
		t.Errorf("scaled damage %v, want %v (dungeon scale should still reach a SpellFlagIgnoreAttackerModifiers spell)", scaled, want)
	}
}

const tickOrderAITestBossID = 900003

// tickOrderAITest casts an instant spell once, on the first rotation call at or after castAt, and notes it.
type tickOrderAITest struct {
	spell  *Spell
	castAt time.Duration
	casted bool
	note   func(string)
}

func (ai *tickOrderAITest) Initialize(target *Target, _ *proto.Target) {
	ai.spell = target.GetOrRegisterSpell(SpellConfig{
		ActionID:    ActionID{SpellID: 3},
		SpellSchool: SpellSchoolPhysical,
		ProcMask:    ProcMaskSpellDamage,

		DamageMultiplier: 1,
		CritMultiplier:   1,

		ApplyEffects: func(*Simulation, *Unit, *Spell) { ai.note("cast") },
	})
}

func (ai *tickOrderAITest) Reset(*Simulation) { ai.casted = false }

func (ai *tickOrderAITest) ExecuteCustomRotation(sim *Simulation) {
	if ai.castAt == 0 || sim.CurrentTime < ai.castAt || ai.casted {
		return
	}
	ai.casted = true
	ai.spell.Cast(sim, nil)
}

func init() {
	AddPresetTarget(&PresetTarget{
		PathPrefix: "TargetAITest",
		Config:     &proto.Target{Id: tickOrderAITestBossID, Name: "Tick Order Boss", Level: CharacterLevel + 3},
		AI:         func() TargetAI { return &tickOrderAITest{} },
	})
}

// Creature::Update runs Unit::Update, whose _UpdateSpells ticks the creature's auras, ahead of the AI that
// casts: a tick due on the update the boss casts on goes first, whichever was queued first.
func TestBossInstantCastComesAfterTheTicksOnIt(t *testing.T) {
	// run puts a dot on the boss and, when wakeAt is set, queues the boss's cast for that time ahead of the
	// dot's first tick. It returns the order of events and the time of the first tick.
	run := func(wakeAt time.Duration) ([]string, time.Duration) {
		var dot *Spell
		var order []string
		timingTestSetup = func(a *timingTestAgent) {
			config := DotConfig{NumberOfTicks: 2, TickLength: time.Second, TicksCanCrit: false, Aura: Aura{Label: "dot"}}
			config.OnTick = func(*Simulation, *Unit, *Dot) { order = append(order, "tick") }
			dot = orderTestDot(t, a, testDotLowID, config)
		}
		sim := NewSim(&proto.RaidSimRequest{
			SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true},
			Raid: &proto.Raid{
				Parties: []*proto.Party{{
					Players: []*proto.Player{{
						Name: "Timing", Class: proto.Class_ClassShaman, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
						Spec: &proto.Player_EnhancementShaman{}, Equipment: &proto.EquipmentSpec{},
					}},
					Buffs: &proto.PartyBuffs{},
				}},
				Tanks: []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}},
			},
			Encounter: &proto.Encounter{
				Targets:  []*proto.Target{{Id: tickOrderAITestBossID, Level: CharacterLevel + 3}},
				Duration: 60,
				ServerSettings: &proto.ServerSettings{
					MapUpdateIntervalMs:   googleProto.Int32(100),
					WorldUpdateIntervalMs: googleProto.Int32(10),
					WorldLoopMs:           googleProto.Float64(0),
					MapUpdateJitterMs:     googleProto.Float64(0),
				},
			},
		})
		timingTestSetup = nil
		sim.reset()
		boss := sim.Encounter.Targets[0]
		ai := boss.AI.(*tickOrderAITest)
		ai.note = func(label string) { order = append(order, label) }
		ai.castAt = wakeAt

		sim.PrePull()
		start := sim.serverTickPhase + ms(500)
		var tickAt time.Duration
		at(sim, start, func(sim *Simulation) {
			if wakeAt != 0 {
				boss.WaitUntil(sim, wakeAt)
			}
			dot := dot.Dot(&boss.Unit)
			dot.Apply(sim)
			tickAt = dot.nextTickAt
		})
		runUntil(sim, start+3*time.Second)
		return order, tickAt
	}

	_, tickAt := run(0)
	order, _ := run(tickAt)
	if want := []string{"tick", "cast", "tick"}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v (the first tick at %v)", order, want, tickAt)
	}
}
