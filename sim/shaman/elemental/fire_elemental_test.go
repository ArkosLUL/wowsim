package elemental

import (
	"slices"
	"testing"
	"time"

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

// 32982 summons the elemental as a plain guardian (SummonProperties 61), without a pet bar, so a raid cast
// skips it (Spell.cpp's TARGET_CHECK_RAID): neither the raid's Replenishment nor Bloodlust reaches it.
func TestFireElementalTakesNoRaidCast(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceTroll,
			Class:     proto.Class_ClassShaman,
			Equipment: &proto.EquipmentSpec{},
			Consumes:  &proto.Consumes{},
			Buffs:     core.FullIndividualBuffs,
			Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{
				Options: &proto.ElementalShaman_Options{Totems: &proto.ShamanTotems{UseFireElemental: true}},
			}},
			Rotation: core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	shaman := sim.Raid.Parties[0].Players[0].(*ElementalShaman)
	if shaman.ReplenishmentAura == nil || !shaman.ReplenishmentAura.IsActive() {
		t.Fatal("the shaman has no Replenishment from the raid, so the test shows nothing")
	}
	elemental := shaman.FireElemental
	elemental.Enable(sim, elemental)
	if elemental.ReplenishmentAura != nil {
		t.Error("the Fire Elemental carries the raid's Replenishment")
	}

	core.BloodlustAura(&shaman.Character, -1).Activate(sim)
	if !shaman.HasActiveAuraWithTag(core.BloodlustAuraTag) {
		t.Fatal("the shaman has no Bloodlust, so the test shows nothing")
	}
	if elemental.HasActiveAuraWithTag(core.BloodlustAuraTag) {
		t.Error("a Fire Elemental out for Bloodlust has it too")
	}
}

// Wrath of Air's 2895 is a raid area aura, which takes a guardian like anyone else in the raid
// (AnyGroupedUnitInObjectRangeCheck), and Fire Nova is a magic spell, so the elemental's cast speed cuts its
// 2 s (Unit::ModSpellCastTime): int32(2000 * 100 / 105) is 1904 ms.
func TestFireNovaUnderWrathOfAir(t *testing.T) {
	for _, tc := range []struct {
		wrathOfAir bool
		want       time.Duration
	}{
		{false, 2000 * time.Millisecond},
		{true, 1904 * time.Millisecond},
	} {
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
			}, &proto.PartyBuffs{}, &proto.RaidBuffs{WrathOfAirTotem: tc.wrathOfAir}, &proto.Debuffs{}),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		elemental := sim.Raid.Parties[0].Players[0].(*ElementalShaman).FireElemental
		elemental.Enable(sim, elemental)
		if got := elemental.FireNova.CastTime(); got != tc.want {
			t.Errorf("Wrath of Air %v: Fire Nova takes %v, want %v", tc.wrathOfAir, got, tc.want)
		}
	}
}

// npc_pet_shaman_fire_elemental casts off two event timers and nothing else: its first Fire Nova and Fire
// Blast 5 to 20 s in, then a Nova every 8 to 15 s and a Blast every 4 to 8 s. A Blast breaks a Nova still
// being cast, about three in ten of them. Over its 2 min that leaves about 6.6 Novas and 18.3 Blasts, and
// 40 swings between them.
func TestFireElementalCastsOffItsEventTimers(t *testing.T) {
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
	shaman := sim.Raid.Parties[0].Players[0].(*ElementalShaman)
	elemental := shaman.FireElemental
	target := sim.GetTargetUnit(0)

	var blastAt, novaAt, swingAt []time.Duration
	blast, nova := elemental.FireBlast.ApplyEffects, elemental.FireNova.ApplyEffects
	auto := elemental.AutoAttacks.MHAuto()
	swing := auto.ApplyEffects
	auto.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		swingAt = append(swingAt, sim.CurrentTime)
		swing(sim, target, spell)
	}
	elemental.FireBlast.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		blastAt = append(blastAt, sim.CurrentTime)
		blast(sim, target, spell)
	}
	elemental.FireNova.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		novaAt = append(novaAt, sim.CurrentTime)
		nova(sim, target, spell)
	}

	const lives = 600
	const update = 100 * time.Millisecond
	novas, blasts, swings := 0, 0, 0
	for life := 0; life < lives; life++ {
		sim.Reseed(int64(1000 + life))
		sim.Reset()
		sim.PrePull()
		blastAt, novaAt, swingAt = blastAt[:0], novaAt[:0], swingAt[:0]
		elemental.EnableWithTimeout(sim, elemental, 2*time.Minute)
		for sim.CurrentTime < 125*time.Second && !sim.Step() {
		}
		if elemental.IsEnabled() {
			t.Fatalf("life %d: the elemental is still out at %v", life, sim.CurrentTime)
		}
		if got := elemental.FireNova.SpellMetrics[target.UnitIndex].Casts; int(got) != len(novaAt) {
			t.Fatalf("life %d: %d Novas counted, %d landed", life, got, len(novaAt))
		}
		novas += len(novaAt)
		blasts += len(blastAt)
		swings += len(swingAt)

		// the first of each at 4.9 s at the earliest, counted from the update before its first one
		if len(blastAt) == 0 || blastAt[0] < 4900*time.Millisecond || blastAt[0] > 20*time.Second+update {
			t.Fatalf("life %d: first Fire Blast at %v, want 4.9 to 20 s in", life, blastAt)
		}
		for i := 1; i < len(blastAt); i++ {
			if gap := blastAt[i] - blastAt[i-1]; gap < 4*time.Second || gap > 8*time.Second {
				t.Fatalf("life %d: Fire Blasts %v apart, want 4 to 8 s", life, gap)
			}
		}
		for i, at := range novaAt {
			// a Nova lands 2 s after its event, so 6.9 s in at the earliest and 8 s after the last one
			if at < 6900*time.Millisecond || (i > 0 && at-novaAt[i-1] < 8*time.Second) {
				t.Fatalf("life %d: Fire Novas landed at %v", life, novaAt)
			}
			// no Blast went off while it was being cast, or it would have been broken
			for _, b := range blastAt {
				if b > at-2*time.Second && b < at {
					t.Fatalf("life %d: the Nova landing at %v was being cast through the Blast at %v", life, at, b)
				}
			}
		}
		// UpdateAI casts what's due ahead of DoMeleeAttackIfReady, so a swing never shares an update with a
		// Blast or a Nova's start: the cast takes it
		for _, at := range swingAt {
			if slices.Contains(blastAt, at) || slices.Contains(novaAt, at+2*time.Second) {
				t.Fatalf("life %d: a swing at %v, on the update of a cast: Blasts at %v, Novas landed at %v", life, at, blastAt, novaAt)
			}
		}
		sim.Cleanup()
	}

	// the timers alone give 6.63 Novas and 18.29 Blasts a life, with deviations of 1.5 and 1.1
	if mean := float64(novas) / lives; mean < 6.3 || mean > 6.95 {
		t.Errorf("%.2f Fire Novas landed a life, want about 6.63", mean)
	}
	if mean := float64(blasts) / lives; mean < 18.05 || mean > 18.55 {
		t.Errorf("%.2f Fire Blasts a life, want about 18.29", mean)
	}
	// every Blast and landed Nova starts the 2 s swing over, a Nova being cast holds it, and a cast on the
	// update a swing is due takes it: 40.1 swings of the 60 its 2 min have room for, with a deviation of 2.2
	if mean := float64(swings) / lives; mean < 39.65 || mean > 40.55 {
		t.Errorf("%.2f swings a life, want about 40.1", mean)
	}
}

// An elemental out before the pull has nothing to attack until then, so its event timers count from the
// pull: nothing cast ahead of it, and the first Fire Blast 5 to 20 s after it.
func TestFireElementalOutBeforeThePullWaitsForIt(t *testing.T) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceTroll,
			Class:     proto.Class_ClassShaman,
			Equipment: &proto.EquipmentSpec{},
			Consumes:  &proto.Consumes{},
			Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{
				Options: &proto.ElementalShaman_Options{Totems: &proto.ShamanTotems{UseFireElemental: true}},
			}},
			Rotation: core.APLRotationFromJsonString(`{"type": "TypeAPL", "prepullActions": [
				{"action": {"castSpell": {"spellId": {"spellId": 2894}}}, "doAtValue": {"const": {"val": "-10s"}}}
			], "priorityList": []}`),
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	elemental := sim.Raid.Parties[0].Players[0].(*ElementalShaman).FireElemental

	var castAt, blastAt []time.Duration
	blast, nova := elemental.FireBlast.ApplyEffects, elemental.FireNova.ApplyEffects
	elemental.FireBlast.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		castAt, blastAt = append(castAt, sim.CurrentTime), append(blastAt, sim.CurrentTime)
		blast(sim, target, spell)
	}
	elemental.FireNova.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		castAt = append(castAt, sim.CurrentTime)
		nova(sim, target, spell)
	}

	const update = 100 * time.Millisecond
	for life := 0; life < 100; life++ {
		sim.Reseed(int64(1000 + life))
		sim.Reset()
		sim.PrePull()
		castAt, blastAt = castAt[:0], blastAt[:0]
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		if !elemental.IsEnabled() {
			t.Fatalf("life %d: no elemental out at the pull, so the test shows nothing", life)
		}
		for sim.CurrentTime < 25*time.Second && !sim.Step() {
		}
		// a Nova lands 2 s after its event
		if len(castAt) > 0 && castAt[0] < 4900*time.Millisecond {
			t.Fatalf("life %d: a cast landed %v after the pull, want none before 4.9 s", life, castAt[0])
		}
		if len(blastAt) == 0 || blastAt[0] > 20*time.Second+update {
			t.Fatalf("life %d: Fire Blasts at %v, want the first 4.9 to 20 s after the pull", life, blastAt)
		}
		sim.Cleanup()
	}
}
