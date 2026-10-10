package elemental

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The Greater Fire Elemental (15438) as Guardian::InitStatsForLevel and spell_sha_fire_elemental_scaling
// build it: pet_levelstats' row, a 200-360 weapon, 2 * Str - 20 attack power, and from the owner at the
// summon 30% of his intellect, 35% of his armor and 300% and 100% of his spell power.
func TestFireElementalStats(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceTroll,
			Class:     proto.Class_ClassShaman,
			Equipment: core.GetGearSet("../../../ui/elemental_shaman/gear_sets", "p1").GearSet,
			Consumes:  &proto.Consumes{},
			Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{
				Options: &proto.ElementalShaman_Options{Totems: &proto.ShamanTotems{UseFireElemental: true}},
			}},
			Rotation: core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	shaman := sim.Raid.Parties[0].Players[0].(*ElementalShaman)
	elemental := shaman.FireElemental
	elemental.Enable(sim, elemental)

	owner := shaman.GetStats()
	if owner[stats.SpellPower] < 1000 || owner[stats.Intellect] < 500 {
		t.Fatalf("owner spell power %v, intellect %v: the gear set isn't on", owner[stats.SpellPower], owner[stats.Intellect])
	}
	intellect := core.PetScalingPct(owner[stats.Intellect], 30)
	got := elemental.GetStats()
	for _, tc := range []struct {
		name       string
		have, want float64
	}{
		{"strength", got[stats.Strength], 283},
		{"agility", got[stats.Agility], 160},
		{"stamina", got[stats.Stamina], 125},
		{"intellect", got[stats.Intellect], 405 + intellect},
		{"spirit", got[stats.Spirit], 460},
		{"health", elemental.MaxHealth(), 8000},
		{"mana", elemental.MaxMana(), 2280 + 15*intellect},
		{"armor", got[stats.Armor], 1100 + core.PetScalingPct(owner[stats.Armor], 35)},
		{"attack power", got[stats.AttackPower], 2*283 - 20 + core.PetScalingPct(owner[stats.SpellPower], 300)},
		{"spell power", got[stats.SpellPower], core.PetScalingPct(owner[stats.SpellPower], 100)},
		{"melee crit rating", got[stats.MeleeCrit], 5 * core.CritRatingPerCritChance},
		{"spell crit rating", got[stats.SpellCrit], 5 * core.CritRatingPerCritChance},
	} {
		if !core.WithinToleranceFloat64(tc.want, tc.have, 1e-6) {
			t.Errorf("%s %v, want %v", tc.name, tc.have, tc.want)
		}
	}

	weapon := elemental.AutoAttacks.MH()
	if weapon.BaseDamageMin != 200 || weapon.BaseDamageMax != 360 || weapon.SwingSpeed != 2 {
		t.Errorf("weapon %v-%v at %v s, want 200-360 at 2 s", weapon.BaseDamageMin, weapon.BaseDamageMax, weapon.SwingSpeed)
	}
	// 57984 and 12470 cost a flat 40 and 30
	if blast, nova := elemental.FireBlast.DefaultCast.Cost, elemental.FireNova.DefaultCast.Cost; blast != 40 || nova != 30 {
		t.Errorf("Fire Blast costs %v and Fire Nova %v, want 40 and 30", blast, nova)
	}
}

// Fire Shield's 13376 is magic class and has no always-hit attribute, so each tick rolls the elemental's spell
// hit (WorldObject::MagicSpellHitResult): 17% misses against a level 83 target for a shaman with no hit.
func TestFireShieldRollsSpellHit(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceTroll,
			Class:     proto.Class_ClassShaman,
			Equipment: &proto.EquipmentSpec{},
			Consumes:  &proto.Consumes{},
			Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{
				Options: &proto.ElementalShaman_Options{Totems: &proto.ShamanTotems{UseFireElemental: true}},
			}},
			Rotation: core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	shaman := sim.Raid.Parties[0].Players[0].(*ElementalShaman)
	elemental := shaman.FireElemental
	elemental.Enable(sim, elemental)
	if hit := elemental.GetStat(stats.SpellHit); hit != 0 {
		t.Fatalf("the elemental has %v spell hit rating off a shaman with none", hit)
	}

	shield := elemental.GetSpell(core.ActionID{SpellID: 13376})
	if shield == nil || !shield.AOEDot().IsActive() {
		t.Fatalf("Fire Shield isn't ticking on the summoned elemental")
	}
	const ticks = 4000
	for i := 0; i < ticks; i++ {
		shield.AOEDot().TickOnce(sim)
	}
	metrics := shield.SpellMetrics[sim.GetTargetUnit(0).UnitIndex]
	if landed := metrics.Hits + metrics.Crits; landed+metrics.Misses != ticks {
		t.Fatalf("%d ticks landed and %d missed, of %d", landed, metrics.Misses, ticks)
	}
	// 16.99% is 680 of 4000, give or take 5 deviations of 24
	if metrics.Misses < 560 || metrics.Misses > 800 {
		t.Errorf("%d of %d Fire Shield ticks missed, want about 680", metrics.Misses, ticks)
	}
}
