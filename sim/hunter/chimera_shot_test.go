package hunter

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

// stingThenChimera puts Serpent Sting on at 0 and shoots Chimera Shot at 3 s, with between run at 2 s, on a
// hunter who otherwise only shoots. It returns the sting's tick as the shot found it, base and done
// multiplier, and 53353's hit, of the first seed where that hit lands without a crit.
func stingThenChimera(t *testing.T, debuffs *proto.Debuffs, between func(*Hunter)) (base, done, hit float64) {
	t.Helper()
	options := googleProto.Clone(PlayerOptionsBasic.Hunter.Options).(*proto.Hunter_Options)
	options.PetType = proto.Hunter_Options_PetNone
	for seed := int64(1); seed <= 40; seed++ {
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceOrc,
				Class:         proto.Class_ClassHunter,
				Equipment:     core.GetGearSet("../../ui/hunter/gear_sets", "p1_mm").GearSet,
				Spec:          &proto.Player_Hunter{Hunter: &proto.Hunter{Options: options}},
				Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
				TalentsString: MMTalents,
			}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, debuffs),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: seed},
		})
		hunter := sim.Raid.Parties[0].Players[0].(*Hunter)
		var hits []core.SpellResult
		hunter.RegisterAura(core.Aura{
			Label:    "chimera test log",
			Duration: core.NeverExpires,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Activate(sim)
			},
			OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
				if spell.ActionID.SpellID == 53353 {
					hits = append(hits, *result)
				}
			},
		})

		sim.Reset()
		sim.PrePull()
		hunter.AddStatsDynamic(sim, stats.Stats{stats.MeleeHit: 20 * core.MeleeHitRatingPerHitChance})
		at := func(when time.Duration, do func(sim *core.Simulation)) {
			sim.AddPendingAction(&core.PendingAction{NextActionAt: when, Priority: core.ActionPriorityLow, OnAction: do})
		}
		at(0, func(sim *core.Simulation) {
			hunter.GCD.Reset()
			hunter.SerpentSting.Cast(sim, hunter.CurrentTarget)
		})
		at(2*time.Second, func(*core.Simulation) { between(hunter) })
		at(3*time.Second, func(sim *core.Simulation) {
			sting := hunter.SerpentSting.Dot(hunter.CurrentTarget)
			if !sting.IsActive() {
				t.Fatal("Serpent Sting isn't on the target at 3 s")
			}
			base, done = sting.SnapshotBaseDamage, sting.SnapshotAttackerMultiplier
			hunter.GCD.Reset()
			hunter.ChimeraShot.Cast(sim, hunter.CurrentTarget)
		})
		for !sim.Step() {
		}
		sim.Cleanup()
		if len(hits) == 1 && hits[0].Landed() && !hits[0].DidCrit() {
			if base <= 0 || done <= 0 || hits[0].Damage <= 0 {
				t.Fatalf("the sting's tick %v x %v, 53353's hit %v", base, done, hits[0].Damage)
			}
			return base, done, hits[0].Damage
		}
	}
	t.Fatal("no seed gave one Chimera Shot - Serpent hit without a crit")
	return 0, 0, 0
}

// within is whether got is want give or take the whole steps on the way, 3 points.
func within(got, want float64) bool {
	return math.Abs(got-want) <= 3
}

// spell_hun_chimera_shot takes the sting's stored tick for 53353, 40% of five ticks of it: the sting's done
// percents are the ones it went on with (AuraEffect::CalculateAmount reuses m_pctMods), and 53353 itself
// ignores the caster's.
func TestChimeraShotSerpentKeepsTheStingsDonePercents(t *testing.T) {
	base, done, hit := stingThenChimera(t, &proto.Debuffs{}, func(*Hunter) {})
	if want := base * done * 2; !within(hit, want) {
		t.Errorf("53353 hit for %v, want the sting's %v x %v x 2 = %v", hit, base, done, want)
	}

	// halved after the sting went on, as under Aspect of the Viper
	base, done, hit = stingThenChimera(t, &proto.Debuffs{}, func(hunter *Hunter) {
		hunter.PseudoStats.DamageDealtMultiplier *= 0.5
	})
	if want := base * done * 2; !within(hit, want) {
		t.Errorf("with the hunter's damage halved since: 53353 hit for %v, want the sting's %v x %v x 2 = %v", hit, base, done, want)
	}
}

// The script runs the tick through the target's damage taken as the sting's (SpellDamageBonusTaken). 53353's
// own hit takes no bonus above 1 (SPELL_ATTR0_CU_NO_POSITIVE_TAKEN_BONUS), so Ebon Plaguebringer's 13% counts
// once: live, a 603 sting under Curse of the Elements gave 1907 a hit, not 2154.
func TestChimeraShotSerpentTakesTheTargetsBonusOnce(t *testing.T) {
	base, done, hit := stingThenChimera(t, &proto.Debuffs{EbonPlaguebringer: true}, func(*Hunter) {})
	if want := base * done * 2 * 1.13; !within(hit, want) {
		t.Errorf("53353 hit for %v, want the sting's %v x %v x 2 x 1.13 = %v", hit, base, done, want)
	}
}

// A damage taken multiplier below 1 isn't a bonus, so it reaches both the sting's tick and 53353's own hit.
func TestChimeraShotSerpentTakesAReductionTwice(t *testing.T) {
	base, done, hit := stingThenChimera(t, &proto.Debuffs{}, func(hunter *Hunter) {
		hunter.CurrentTarget.PseudoStats.DamageTakenMultiplier *= 0.8
	})
	if want := base * done * 2 * 0.8 * 0.8; !within(hit, want) {
		t.Errorf("53353 hit for %v, want the sting's %v x %v x 2 x 0.8 x 0.8 = %v", hit, base, done, want)
	}
}
