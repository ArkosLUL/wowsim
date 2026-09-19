package core

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

func scaleTestTarget(worldBoss bool, health, armor float64) *proto.Target {
	return &proto.Target{
		Level:     CharacterLevel + 3,
		WorldBoss: googleProto.Bool(worldBoss),
		Stats:     stats.Stats{stats.Health: health, stats.Armor: armor}.ToFloatArray(),
	}
}

// unscaledRaid pins every generic set to 1, so a test only sees what it sets.
func unscaledRaid(sets *proto.DungeonScaleSettings) *proto.ServerSettings {
	one := modifiers(1, 1, 1, 1)
	for _, set := range []**proto.DungeonScaleModifiers{&sets.Raid, &sets.RaidBoss, &sets.RaidHeroic, &sets.RaidHeroicBoss} {
		if *set == nil {
			*set = one
		}
	}
	return &proto.ServerSettings{DungeonScale: sets}
}

type scaledTarget struct {
	health, armor, damage float64
}

func scaledTargetOf(target *Target) scaledTarget {
	return scaledTarget{target.stats[stats.Health], target.stats[stats.Armor], target.PseudoStats.DamageDealtMultiplier}
}

func TestDungeonScaleChangesOnlyItsStat(t *testing.T) {
	const health, armor = 1001, 10643
	const n25, h25 = proto.RaidDifficulty_RaidDifficulty25Normal, proto.RaidDifficulty_RaidDifficulty25Heroic

	for _, tc := range []struct {
		name       string
		sets       *proto.DungeonScaleSettings
		difficulty proto.RaidDifficulty
		worldBoss  bool
		want       scaledTarget
	}{
		{"unscaled", &proto.DungeonScaleSettings{}, n25, true, scaledTarget{health, armor, 1}},
		// halves round away from zero like C++ round: 15964.5 goes up, not to even
		{"health", &proto.DungeonScaleSettings{Raid25Boss: modifiers(1, 1.5, 1, 1)}, n25, true, scaledTarget{1502, armor, 1}},
		{"armor", &proto.DungeonScaleSettings{Raid25Boss: modifiers(1, 1, 1.5, 1)}, n25, true, scaledTarget{health, 15965, 1}},
		{"damage", &proto.DungeonScaleSettings{Raid25Boss: modifiers(1, 1, 1, 2)}, n25, true, scaledTarget{health, armor, 2}},
		{"global times each stat", &proto.DungeonScaleSettings{Raid25Boss: modifiers(2, 1.5, 1, 0.5)}, n25, true, scaledTarget{3003, 2 * armor, 1}},
		// float32(1001) * float32(1.2) is 1201.2
		{"live boss health", &proto.DungeonScaleSettings{Raid25Boss: modifiers(1, 1.2, 1, 1)}, n25, true, scaledTarget{1201, armor, 1}},
		{"creatures take the creature set", &proto.DungeonScaleSettings{Raid25: modifiers(1, 2, 1, 1), Raid25Boss: modifiers(1, 3, 1, 1)}, n25, false, scaledTarget{2002, armor, 1}},
		{"heroic takes the heroic set", &proto.DungeonScaleSettings{Raid25Boss: modifiers(1, 2, 1, 1), Raid25HeroicBoss: modifiers(1, 3, 1, 1)}, h25, true, scaledTarget{3003, armor, 1}},
		{"unknown difficulty is 25 normal", &proto.DungeonScaleSettings{Raid25Boss: modifiers(1, 2, 1, 1)}, proto.RaidDifficulty_RaidDifficultyUnknown, true, scaledTarget{2002, armor, 1}},
	} {
		options := &proto.Encounter{
			RaidDifficulty: tc.difficulty,
			Targets:        []*proto.Target{scaleTestTarget(tc.worldBoss, health, armor)},
		}
		encounter := NewEncounter(options, NewServerSettings(unscaledRaid(tc.sets)))
		if got := scaledTargetOf(encounter.Targets[0]); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestDungeonScaleLiveDefaults(t *testing.T) {
	const health, armor = 1_234_567, 10643
	encounter := NewEncounter(&proto.Encounter{Targets: []*proto.Target{
		scaleTestTarget(true, health, armor),
		scaleTestTarget(false, health, armor),
	}}, NewServerSettings(nil))

	live := NewServerSettings(LiveServerDefaults()).DungeonScale
	for i, boss := range []bool{true, false} {
		m := live.Multipliers(proto.RaidDifficulty_RaidDifficulty25Normal, boss)
		want := scaledTarget{
			math.Round(float64(float32(health) * float32(m.Health))),
			math.Round(float64(float32(armor) * float32(m.Armor))),
			m.Damage,
		}
		if got := scaledTargetOf(encounter.Targets[i]); got != want {
			t.Errorf("boss %v: got %+v, want the live %+v", boss, got, want)
		}
	}
}

// The server converts health to float before multiplying, so anything past 2^24 loses its low bits.
func TestDungeonScaleHealthIsFloat32(t *testing.T) {
	encounter := NewEncounter(&proto.Encounter{Targets: []*proto.Target{scaleTestTarget(true, 1<<24+1, 0)}},
		NewServerSettings(unscaledRaid(&proto.DungeonScaleSettings{})))
	if got, want := encounter.Targets[0].stats[stats.Health], float64(1<<24); got != want {
		t.Errorf("health = %v, want %v", got, want)
	}
}

// the placeholder stands in when there are no targets (stat sheets, simval), so it never scales
func TestDungeonScaleSkipsPlaceholderTarget(t *testing.T) {
	server := NewServerSettings(unscaledRaid(&proto.DungeonScaleSettings{
		Raid25Boss: modifiers(2, 2, 2, 2),
		Raid25:     modifiers(2, 2, 2, 2),
	}))
	encounter := NewEncounter(&proto.Encounter{}, server)
	if got, want := scaledTargetOf(encounter.Targets[0]), (scaledTarget{0, 0, 1}); got != want {
		t.Errorf("placeholder got %+v, want %+v", got, want)
	}
}

func TestDungeonScaleSetsHealthFightLength(t *testing.T) {
	options := &proto.Encounter{
		UseHealth: true,
		Targets: []*proto.Target{
			scaleTestTarget(true, 1000, 0),
			scaleTestTarget(false, 1000, 0),
		},
	}
	server := NewServerSettings(unscaledRaid(&proto.DungeonScaleSettings{
		Raid25Boss: modifiers(1, 1.5, 1, 1),
		Raid25:     modifiers(1, 0.5, 1, 1),
	}))
	if got := NewEncounter(options, server).EndFightAtHealth; got != 2000 {
		t.Errorf("EndFightAtHealth = %v, want 1500 + 500", got)
	}
}

// Damage multiplies what the boss deals, and nothing the raid deals to it.
func TestDungeonScaleDamageReachesBossSwings(t *testing.T) {
	swing := func(damage float64) (bossHit, raidHit float64) {
		target := scaleTestTarget(true, 0, 0)
		target.SwingSpeed = 2
		target.MinBaseDamage = 1000
		sim := NewSim(&proto.RaidSimRequest{
			SimOptions: &proto.SimOptions{RandomSeed: 1},
			Raid: &proto.Raid{
				Parties:       []*proto.Party{{}},
				TargetDummies: 1,
				Tanks:         []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}},
			},
			Encounter: &proto.Encounter{
				Duration:       60,
				Targets:        []*proto.Target{target},
				ServerSettings: unscaledRaid(&proto.DungeonScaleSettings{Raid25Boss: modifiers(1, 1, 1, damage)}),
			},
		})
		sim.Reset()

		boss := sim.Encounter.Targets[0]
		tank := boss.CurrentTarget
		if tank == nil || boss.AutoAttacks.MHAuto() == nil {
			t.Fatal("the boss has no one to swing at")
		}
		bossHit = boss.AutoAttacks.MHAuto().CalcDamage(sim, tank, 1000, boss.AutoAttacks.MHAuto().OutcomeAlwaysHit).Damage

		raidSpell := &Spell{Unit: tank, SpellSchool: SpellSchoolPhysical, DamageMultiplier: 1, DamageMultiplierAdditive: 1,
			SpellMetrics: make([]SpellMetrics, len(sim.AllUnits))}
		raidHit = raidSpell.CalcDamage(sim, &boss.Unit, 1000, raidSpell.OutcomeAlwaysHit).Damage
		return bossHit, raidHit
	}

	bossBase, raidBase := swing(1)
	bossScaled, raidScaled := swing(1.5)
	if bossBase <= 0 || math.Abs(bossScaled/bossBase-1.5) > 1e-9 {
		t.Errorf("boss hit %v at damage 1.5, want 1.5 x %v", bossScaled, bossBase)
	}
	if raidScaled != raidBase {
		t.Errorf("raid hit %v at damage 1.5, want it unchanged at %v", raidScaled, raidBase)
	}
}
