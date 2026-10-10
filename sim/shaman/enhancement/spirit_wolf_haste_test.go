package enhancement

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

type wolfStep struct {
	at time.Duration
	do func(sim *core.Simulation, enh *EnhancementShaman)
}

// runWolfSteps runs one iteration of a naked tauren whose only talent is Feral Spirit, with each step done at
// its time. No totems, no Flurry and an empty rotation, so only the steps move the shaman's swing speed.
func runWolfSteps(settings *proto.ServerSettings, steps ...wolfStep) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.ServerSettings = settings
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceTauren,
			Class:     proto.Class_ClassShaman,
			Equipment: &proto.EquipmentSpec{},
			Consumes:  &proto.Consumes{},
			Spec: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{
				Options: &proto.EnhancementShaman_Options{},
			}},
			// the enhancement tree's 29th talent
			TalentsString: "-" + strings.Repeat("0", 28) + "1",
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	sim.PrePull()
	enh := sim.Raid.Parties[0].Players[0].(*EnhancementShaman)
	for _, step := range steps {
		sim.AddPendingAction(&core.PendingAction{
			NextActionAt: step.at,
			Priority:     core.ActionPriorityLow,
			OnAction:     func(sim *core.Simulation) { step.do(sim, enh) },
		})
	}
	for !sim.Step() {
	}
	sim.Cleanup()
}

// 425792 on a Spirit Wolf: no haste lands on it directly, and it takes the shaman's swing speed at the summon
// and again every 2 s from there. Bloodlust's 30% reads 29, so its 1.5 s swing goes to 1162 ms, as on the
// server. Without the tweak the wolf is stock: Bloodlust lands on a wolf that's out for the cast, and that's
// all the haste it gets.
func TestSpiritWolvesFollowTheShamansHaste(t *testing.T) {
	const (
		bareMs      = 1500
		inheritedMs = 1162 // 1500 / 1.29
		bloodlustMs = 1153 // 1500 / 1.3
	)
	off := new(bool)
	for _, tc := range []struct {
		name     string
		settings *proto.ServerSettings
		tweak    bool
	}{
		{"live settings", nil, true},
		{"FeralSpiritHaste off", &proto.ServerSettings{SpellTweaks: &proto.SpellTweaksSettings{FeralSpiritHaste: off}}, false},
		{"SpellTweaks.Enable off", &proto.ServerSettings{SpellTweaks: &proto.SpellTweaksSettings{Enable: off}}, false},
	} {
		pick := func(withTweak, stock int64) int64 {
			if tc.tweak {
				return withTweak
			}
			return stock
		}
		cast := func(t *testing.T, sim *core.Simulation, enh *EnhancementShaman, spell *core.Spell) {
			t.Helper()
			// the idle rotation holds the GCD while it waits
			enh.GCD.Reset()
			if spell == nil || !spell.Cast(sim, enh.CurrentTarget) {
				t.Fatalf("%v didn't cast at %v", spell, sim.CurrentTime)
			}
		}
		summon := func(t *testing.T, sim *core.Simulation, enh *EnhancementShaman) {
			t.Helper()
			if !enh.Talents.FeralSpirit || enh.Talents.Flurry != 0 {
				t.Fatalf("talents %+v: want Feral Spirit alone", enh.Talents)
			}
			cast(t, sim, enh, enh.FeralSpirit)
		}
		bloodlust := func(t *testing.T, sim *core.Simulation, enh *EnhancementShaman) {
			t.Helper()
			if got := enh.SwingSpeed(); got != 1 {
				t.Fatalf("the shaman's swing speed is %v before Bloodlust, want 1", got)
			}
			cast(t, sim, enh, enh.GetSpell(enh.BloodlustActionID()))
			if got := enh.SwingSpeed(); math.Abs(got-1.3) > 1e-9 {
				t.Fatalf("the shaman's swing speed is %v under Bloodlust, want 1.3", got)
			}
		}
		// both wolves' swing time in whole ms, and whether Bloodlust itself is on them
		expect := func(t *testing.T, when string, enh *EnhancementShaman, swingMs int64, hasBloodlust bool) {
			t.Helper()
			for _, wolf := range []*core.Pet{enh.SpiritWolves.SpiritWolf1.Pet, enh.SpiritWolves.SpiritWolf2.Pet} {
				if !wolf.IsEnabled() {
					t.Fatalf("%s: %s isn't out", when, wolf.Label)
				}
				// 1.3 * (1 / 1.3) isn't exactly 1, so a swing can come out a hair under its whole ms
				if got := (wolf.AutoAttacks.MainhandSwingSpeed() + time.Microsecond).Milliseconds(); got != swingMs {
					t.Errorf("%s: %s swings every %d ms, want %d", when, wolf.Label, got, swingMs)
				}
				if got := wolf.HasActiveAuraWithTag(core.BloodlustAuraTag); got != hasBloodlust {
					t.Errorf("%s: %s has Bloodlust = %v, want %v", when, wolf.Label, got, hasBloodlust)
				}
			}
		}

		t.Run(tc.name+"/Bloodlust after the summon", func(t *testing.T) {
			const summoned = 1000 * time.Millisecond
			var before [2]float64
			wolves := func(enh *EnhancementShaman) [2]*core.Pet {
				return [2]*core.Pet{enh.SpiritWolves.SpiritWolf1.Pet, enh.SpiritWolves.SpiritWolf2.Pet}
			}
			ran := 0
			step := func(after time.Duration, do func(sim *core.Simulation, enh *EnhancementShaman)) wolfStep {
				return wolfStep{summoned + after, func(sim *core.Simulation, enh *EnhancementShaman) {
					ran++
					do(sim, enh)
				}}
			}
			runWolfSteps(tc.settings,
				step(0, func(sim *core.Simulation, enh *EnhancementShaman) {
					for i, wolf := range wolves(enh) {
						if wolf.HasteCarrier != tc.tweak || wolf.OwnerHasteTicksOnGuardian != tc.tweak {
							t.Errorf("%s: haste carrier %v, ticking %v, want both %v", wolf.Label, wolf.HasteCarrier,
								wolf.OwnerHasteTicksOnGuardian, tc.tweak)
						}
						before[i] = wolf.PseudoStats.MeleeSpeedMultiplier
					}
					summon(t, sim, enh)
					expect(t, "summoned", enh, bareMs, false)
				}),
				// the live run's gap: 1540 ms, so before the wolves' first tick
				step(1540*time.Millisecond, func(sim *core.Simulation, enh *EnhancementShaman) {
					bloodlust(t, sim, enh)
					expect(t, "right after Bloodlust", enh, pick(bareMs, bloodlustMs), !tc.tweak)
				}),
				step(1800*time.Millisecond, func(sim *core.Simulation, enh *EnhancementShaman) {
					expect(t, "1.8 s after the summon", enh, pick(bareMs, bloodlustMs), !tc.tweak)
				}),
				// the first tick is on the wolves' last map update before this
				step(2*time.Second, func(sim *core.Simulation, enh *EnhancementShaman) {
					expect(t, "2 s after the summon", enh, pick(inheritedMs, bloodlustMs), !tc.tweak)
				}),
				step(30*time.Second, func(sim *core.Simulation, enh *EnhancementShaman) {
					expect(t, "30 s after the summon", enh, pick(inheritedMs, bloodlustMs), !tc.tweak)
				}),
				// Bloodlust ran out 41.54 s after the summon, and the tick after that is at 42 s or just before
				step(42*time.Second, func(sim *core.Simulation, enh *EnhancementShaman) {
					if got := enh.SwingSpeed(); math.Abs(got-1) > 1e-9 {
						t.Fatalf("the shaman's swing speed is %v after Bloodlust, want 1", got)
					}
					expect(t, "after Bloodlust ran out", enh, bareMs, false)
				}),
				step(46*time.Second, func(sim *core.Simulation, enh *EnhancementShaman) {
					for i, wolf := range wolves(enh) {
						if wolf.IsEnabled() {
							t.Errorf("%s is still out after 45 s", wolf.Label)
						}
						if got := wolf.PseudoStats.MeleeSpeedMultiplier; math.Abs(got-before[i]) > 1e-9 {
							t.Errorf("%s: melee speed x%v once gone, x%v before the summon", wolf.Label, got, before[i])
						}
					}
				}),
			)
			if ran != 7 {
				t.Errorf("%d of the 7 steps ran", ran)
			}
		})

		// a raid cast only reaches who's there for it, so stock wolves that come later get nothing
		t.Run(tc.name+"/Bloodlust before the summon", func(t *testing.T) {
			const summoned = 2600 * time.Millisecond
			ran := 0
			step := func(at time.Duration, do func(sim *core.Simulation, enh *EnhancementShaman)) wolfStep {
				return wolfStep{at, func(sim *core.Simulation, enh *EnhancementShaman) {
					ran++
					do(sim, enh)
				}}
			}
			runWolfSteps(tc.settings,
				step(time.Second, func(sim *core.Simulation, enh *EnhancementShaman) { bloodlust(t, sim, enh) }),
				step(summoned, func(sim *core.Simulation, enh *EnhancementShaman) {
					summon(t, sim, enh)
					expect(t, "summoned under Bloodlust", enh, pick(inheritedMs, bareMs), false)
				}),
				step(summoned+5*time.Second, func(sim *core.Simulation, enh *EnhancementShaman) {
					expect(t, "5 s after the summon", enh, pick(inheritedMs, bareMs), false)
				}),
			)
			if ran != 3 {
				t.Errorf("%d of the 3 steps ran", ran)
			}
		})
	}
}

type wolfFight struct {
	dps float64
	// white swings a wolf, an iteration
	wolfSwings float64
}

// spiritWolfFight runs TestEnhancement-Average-Default's request for that many iterations under the settings.
func spiritWolfFight(t *testing.T, iterations int32, settings *proto.ServerSettings) wolfFight {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(5)
	encounter.ServerSettings = settings
	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:               proto.Race_RaceTroll,
				Class:              proto.Class_ClassShaman,
				Equipment:          core.GetGearSet("../../../ui/enhancement_shaman/gear_sets", "p1").GearSet,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsFTFT,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.GetAplRotation("../../../ui/enhancement_shaman/apls", "default_ft").Rotation,
				TalentsString:      StandardTalents,
				Glyphs:             StandardGlyphs,
				Profession1:        proto.Profession_Engineering,
				DistanceFromTarget: 30,
				ReactionTimeMs:     150,
				ChannelClipDelayMs: 50,
			},
			core.FullPartyBuffs,
			core.FullRaidBuffs,
			core.FullDebuffs),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: iterations, IsTest: true, RandomSeed: 101},
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}

	fight := wolfFight{dps: result.RaidMetrics.Dps.Avg}
	wolves := 0
	for _, pet := range result.RaidMetrics.Parties[0].Players[0].Pets {
		if !strings.HasPrefix(pet.Name, "Spirit Wolf") {
			continue
		}
		wolves++
		for _, action := range pet.Actions {
			if action.Id.GetOtherId() != proto.OtherAction_OtherActionAttack {
				continue
			}
			for _, target := range action.Targets {
				fight.wolfSwings += float64(target.Casts)
			}
		}
	}
	if wolves != 2 {
		t.Fatalf("%d Spirit Wolves in the result, want 2", wolves)
	}
	fight.wolfSwings /= float64(wolves) * float64(iterations)
	return fight
}

// On the suite's default fight the wolves come out before Bloodlust and with Flurry down. The tweak has to
// hand them both later, on top of the shaman's haste rating, which stock wolves never see. The summon's
// speed alone is worth about 24% more swings than stock, the ticks about 77%.
func TestSpiritWolfHasteTweakOnTheDefaultFight(t *testing.T) {
	const iterations = 200
	on := spiritWolfFight(t, iterations, nil)
	off := spiritWolfFight(t, iterations, &proto.ServerSettings{
		SpellTweaks: &proto.SpellTweaksSettings{FeralSpiritHaste: new(bool)},
	})
	t.Logf("tweak on: %.2f dps, %.2f swings a wolf; off: %.2f dps, %.2f swings a wolf", on.dps, on.wolfSwings,
		off.dps, off.wolfSwings)
	if on.wolfSwings < 1.5*off.wolfSwings {
		t.Errorf("a wolf swings %.2f times with the tweak and %.2f without: want at least 50%% more with it",
			on.wolfSwings, off.wolfSwings)
	}
}
