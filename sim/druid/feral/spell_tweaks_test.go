package feral

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

const idolOfTheRavenousBeast = 40713

// StandardTalents with Rend and Tear (the feral tree's 28th digit) and Primal Gore (29th) set
func feralTalents(rendAndTear int, primalGore bool) string {
	tree := []byte("503202132322010053120230310511")
	tree[27] = byte('0' + rendAndTear)
	tree[28] = '0'
	if primalGore {
		tree[28] = '1'
	}
	return "-" + string(tree) + "-205503012"
}

// newTestCat is a naked cat with an empty rotation, the given relic in its ranged slot, on exact timing.
func newTestCat(t *testing.T, talents string, spec *proto.Player_FeralDruid, relic int32) (*core.Simulation, *FeralDruid, *core.Unit) {
	t.Helper()
	gear := &proto.EquipmentSpec{}
	for range proto.ItemSlot_ItemSlotRanged + 1 {
		gear.Items = append(gear.Items, &proto.ItemSpec{})
	}
	gear.Items[proto.ItemSlot_ItemSlotRanged].Id = relic
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 300
	encounter.ServerSettings = &proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(0)}

	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceTauren,
			Class:         proto.Class_ClassDruid,
			Equipment:     gear,
			Spec:          spec,
			Glyphs:        &proto.Glyphs{},
			TalentsString: talents,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	return sim, sim.Raid.Parties[0].Players[0].(*FeralDruid), sim.GetTargetUnit(0)
}

// rakeTicks lands a Rake every 10 s through the fight at 40% more crit, and counts its ticks and the ones
// that crit.
func rakeTicks(t *testing.T, talents string) (ticks, crits int) {
	sim, cat, target := newTestCat(t, talents, PlayerOptionsMonoCatNoBleed, 0)
	cat.RegisterAura(core.Aura{
		Label:    "Rake tick log",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnPeriodicDamageDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if cat.Rake.IsEqual(spell) {
				ticks++
				if result.DidCrit() {
					crits++
				}
			}
		},
	})

	sim.Reset()
	sim.PrePull()
	// no miss, no dodge
	cat.AddStatsDynamic(sim, stats.Stats{
		stats.MeleeHit:  20 * core.MeleeHitRatingPerHitChance,
		stats.Expertise: 100 * core.ExpertisePerQuarterPercentReduction,
		stats.MeleeCrit: 40 * core.CritRatingPerCritChance,
	})
	for at := time.Second; at < 290*time.Second; at += 10 * time.Second {
		sim.AddPendingAction(&core.PendingAction{
			NextActionAt: at,
			Priority:     core.ActionPriorityLow,
			OnAction: func(sim *core.Simulation) {
				cat.AddEnergy(sim, 100-cat.CurrentEnergy(), cat.EnergyRefundMetrics)
				cat.GCD.Reset()
				if !cat.Rake.Cast(sim, target) {
					t.Fatalf("Rake didn't cast at %s", sim.CurrentTime)
				}
			},
		})
	}
	for !sim.Step() {
	}
	sim.Cleanup()
	return ticks, crits
}

// mod-spell-tweaks adds Rake to Primal Gore's periodic crit (63503), so its ticks crit at the snapshot's
// chance with the talent and never without it.
func TestPrimalGoreLetsRakeTicksCrit(t *testing.T) {
	ticks, crits := rakeTicks(t, feralTalents(5, true))
	if ticks < 60 || crits < ticks/4 || crits > ticks*3/4 {
		t.Errorf("with Primal Gore %d of %d Rake ticks crit, want about half", crits, ticks)
	}
	ticks, crits = rakeTicks(t, feralTalents(5, false))
	if ticks < 60 || crits != 0 {
		t.Errorf("without Primal Gore %d of %d Rake ticks crit, want none", crits, ticks)
	}
}

// Rend and Tear gives Ferocious Bite 10% crit a rank on a bleeding target (mod-spell-tweaks raises effect 2
// of 48432, 48433, 48434, 51268 and 51269), and always with AssumeBleedActive.
func TestRendAndTearBiteCritOnABleedingTarget(t *testing.T) {
	var bledAt5 float64
	for _, rank := range []int{0, 3, 5} {
		sim, cat, target := newTestCat(t, feralTalents(rank, true), PlayerOptionsMonoCatNoBleed, 0)
		sim.Reset()
		table := cat.AttackTables[target.UnitIndex]
		before := cat.FerociousBite.PhysicalCritChance(table)
		cat.Rake.Dot(target).Apply(sim)
		bled := cat.FerociousBite.PhysicalCritChance(table)
		if got, want := bled-before, 0.1*float64(rank); math.Abs(got-want) > 1e-9 {
			t.Errorf("rank %d: a bleed adds %.4f to Ferocious Bite's crit chance, want %.4f", rank, got, want)
		}
		if rank == 5 {
			bledAt5 = bled
		}
	}

	sim, assumed, target := newTestCat(t, feralTalents(5, true), PlayerOptionsMonoCat, 0)
	sim.Reset()
	if got := assumed.FerociousBite.PhysicalCritChance(assumed.AttackTables[target.UnitIndex]); math.Abs(got-bledAt5) > 1e-9 {
		t.Errorf("AssumeBleedActive gives Ferocious Bite %.4f crit chance with no bleed up, want %.4f", got, bledAt5)
	}
}

// mod-spell-tweaks raises Idol of the Ravenous Beast's Shred bonus (60773, a flat mod on effect 1) from 90
// to 135, which Shred's 225% weapon damage makes about 304.
func TestIdolOfTheRavenousBeastAddsToShred(t *testing.T) {
	_, bare, _ := newTestCat(t, StandardTalents, PlayerOptionsMonoCat, 0)
	_, idol, _ := newTestCat(t, StandardTalents, PlayerOptionsMonoCat, idolOfTheRavenousBeast)
	if got := idol.Shred.Direct.Min - bare.Shred.Direct.Min; got != 135 {
		t.Errorf("the idol adds %v to Shred's base damage, want 135", got)
	}
	if got := idol.Shred.Direct.Max - bare.Shred.Direct.Max; got != 135 {
		t.Errorf("the idol adds %v to Shred's top roll, want 135", got)
	}
	if got := idol.Shred.Direct.WeaponPct; got != 2.25 {
		t.Errorf("Shred's weapon percent is %v under the idol, want 2.25", got)
	}
}
