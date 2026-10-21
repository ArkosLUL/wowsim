package warlock

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func newTalentTestSim(t *testing.T, talents string, spec *proto.Player_Warlock, equipment *proto.EquipmentSpec,
	rotation *proto.APLRotation) (*core.Simulation, *Warlock) {
	t.Helper()
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceOrc,
			Class:         proto.Class_ClassWarlock,
			Equipment:     equipment,
			Consumes:      &proto.Consumes{},
			TalentsString: talents,
			Spec:          spec,
			Rotation:      rotation,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].(*Warlock)
}

var emptyRotation = core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`)

// Unit::SpellCriticalDamageBonus: no meta gem here, so a crit is 1 + 0.5 × (1 + the SPELLMOD_CRIT_DAMAGE_BONUS
// percents). mod-spell-tweaks adds Empowered Corruption's 14% a point on Shadow Bolt and Nemesis' 10% a point
// on Shadow Bolt, Incinerate and Soul Fire to Ruin's 20%.
func TestNukeCritDamage(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		talents              string
		spec                 *proto.Player_Warlock
		ec, nemesis, ruin    int32
		shadowBolt, fireNuke float64
	}{
		{"Affliction", AfflictionTalents, DefaultAfflictionWarlock, 3, 0, 5, 2.21, 2},
		{"Demonology", DemonologyTalents, DefaultDemonologyWarlock, 0, 3, 5, 2.15, 2.15},
		{"Destruction", DestructionTalents, DefaultDestroWarlock, 0, 0, 5, 2, 2},
	} {
		_, warlock := newTalentTestSim(t, tc.talents, tc.spec, &proto.EquipmentSpec{}, emptyRotation)
		if got := warlock.Talents; got.EmpoweredCorruption != tc.ec || got.Nemesis != tc.nemesis || got.Ruin != tc.ruin {
			t.Fatalf("%s: Empowered Corruption %d, Nemesis %d, Ruin %d; the case expects %d, %d, %d", tc.name,
				got.EmpoweredCorruption, got.Nemesis, got.Ruin, tc.ec, tc.nemesis, tc.ruin)
		}
		for _, c := range []struct {
			spell *core.Spell
			want  float64
		}{
			{warlock.ShadowBolt, tc.shadowBolt},
			{warlock.Incinerate, tc.fireNuke},
			{warlock.SoulFire, tc.fireNuke},
		} {
			if got := c.spell.CritMultiplier; math.Abs(got-c.want) > 1e-9 {
				t.Errorf("%s: %s crits for %v, want %v", tc.name, c.spell.ActionID, got, c.want)
			}
		}
	}
}

// The Pyroclasm buff's fire and shadow damage, from mod-spell-tweaks' spell_dbc rows for 18093, 63243 and 63244.
func TestPyroclasmBuff(t *testing.T) {
	trees := strings.Split(DestructionTalents, "-")
	for rank, want := range []float64{1: 1.07, 2: 1.13, 3: 1.20} {
		if rank == 0 {
			continue
		}
		// Pyroclasm is the 19th talent of the destruction tree
		destro := []byte(trees[2])
		destro[18] = byte('0' + rank)
		talents := strings.Join([]string{trees[0], trees[1], string(destro)}, "-")
		sim, warlock := newTalentTestSim(t, talents, DefaultDestroWarlock, &proto.EquipmentSpec{}, emptyRotation)
		if warlock.Talents.Pyroclasm != int32(rank) {
			t.Fatalf("talents %s give Pyroclasm %d, want %d", talents, warlock.Talents.Pyroclasm, rank)
		}

		sim.PrePull()
		multipliers := func() (fire, shadow float64) {
			m := warlock.PseudoStats.SchoolDamageDealtMultiplier
			return m[stats.SchoolIndexFire], m[stats.SchoolIndexShadow]
		}
		fire, shadow := multipliers()
		warlock.PyroclasmAura.Activate(sim)
		fireUp, shadowUp := multipliers()
		if math.Abs(fireUp/fire-want) > 1e-9 || math.Abs(shadowUp/shadow-want) > 1e-9 {
			t.Errorf("Pyroclasm %d: fire ×%v and shadow ×%v, want ×%v", rank, fireUp/fire, shadowUp/shadow, want)
		}
	}
}

// Demonic Pact procs off the demon's crits at most every 20 s: spell_proc 53646's cooldown since world update
// 2026_03_16_00.
func TestDemonicPactInternalCooldown(t *testing.T) {
	if icd := core.ServerProcFor(demonicPactPetAuraID).ICD; icd != 20*time.Second {
		t.Fatalf("the demon's Demonic Pact aura has a %v cooldown, want 20 s", icd)
	}

	sim, warlock := newTalentTestSim(t, DemonologyTalents, DefaultDemonologyWarlock,
		core.GetGearSet("../../ui/warlock/gear_sets", "p4_demo").GearSet,
		core.GetAplRotation("../../ui/warlock/apls", "demo").Rotation)
	pact := warlock.DemonicPactAura
	if pact == nil {
		t.Fatal("no Demonic Pact aura on the warlock")
	}

	// each proc that hands out spell power sets the bonus again
	var procs []time.Duration
	bonus := pact.ExclusiveEffects[0]
	onGain := bonus.OnGain
	bonus.OnGain = func(ee *core.ExclusiveEffect, sim *core.Simulation) {
		procs = append(procs, sim.CurrentTime)
		onGain(ee, sim)
	}

	sim.PrePull()
	for sim.CurrentTime < 150*time.Second && !sim.Step() {
	}
	if len(procs) < 3 {
		t.Fatalf("Demonic Pact went on %d times in 150 s, want a few: %v", len(procs), procs)
	}
	for i := 1; i < len(procs); i++ {
		if gap := procs[i] - procs[i-1]; gap < 20*time.Second {
			t.Errorf("Demonic Pact went on at %v and again at %v, %v apart, want 20 s at least", procs[i-1], procs[i], gap)
		}
	}
}
