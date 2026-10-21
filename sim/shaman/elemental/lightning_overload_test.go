package elemental

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Concussion 5, Elemental Fury 5, Call of Thunder, Lightning Mastery 5, Lightning Overload 3, Shamanism 5,
// and no Elemental Focus, so no Clearcasting and no Elemental Oath.
const overloadTalents = "0500000500010050000300050"

type overloadHit struct {
	overload bool
	target   *core.Unit
	damage   float64
	crit     bool
}

// overloadSim is a naked shaman with 1000 spell power, no misses and half her casts critting, against
// level 80 targets that don't resist. The raid's 3% done percent and a 13% nature damage taken stand in for
// the buffs and debuffs a raid brings.
func overloadSim(t *testing.T, talents string, targets int) (*core.Simulation, *ElementalShaman, *[]overloadHit) {
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 900
	encounter.Targets = nil
	for i := 0; i < targets; i++ {
		encounter.Targets = append(encounter.Targets, &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid})
	}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceTroll,
			Class:     proto.Class_ClassShaman,
			Equipment: &proto.EquipmentSpec{},
			Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{
				Options: &proto.ElementalShaman_Options{Totems: NoTotems},
			}},
			Glyphs:        &proto.Glyphs{Major1: int32(proto.ShamanMajorGlyph_GlyphOfLightningBolt)},
			TalentsString: talents,
			Rotation:      core.APLRotationFromJsonString(`{"type":"TypeAPL","priorityList":[]}`),
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{SanctifiedRetribution: true}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	shaman := sim.Raid.Parties[0].Players[0].(*ElementalShaman)
	if shaman.Talents.LightningOverload != 3 || shaman.Talents.Shamanism != 5 || shaman.Talents.Concussion != 5 {
		t.Fatalf("talents %q didn't give Lightning Overload 3, Shamanism 5 and Concussion 5", talents)
	}

	var hits []overloadHit
	shaman.RegisterAura(core.Aura{
		Label:    "overload test log",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			for _, s := range []*core.Spell{shaman.LightningBolt, shaman.LightningBoltLO, shaman.ChainLightning, shaman.ChainLightningLO} {
				if spell == s {
					hits = append(hits, overloadHit{spell == shaman.LightningBoltLO || spell == shaman.ChainLightningLO, result.Target, result.Damage, result.DidCrit()})
				}
			}
		},
	})

	sim.Reset()
	sim.PrePull()
	for _, target := range sim.Encounter.TargetUnits {
		target.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexNature] *= 1.13
	}
	shaman.AddStatsDynamic(sim, stats.Stats{
		stats.SpellPower: 1000,
		stats.SpellHit:   20 * core.SpellHitRatingPerHitChance,
		stats.SpellCrit:  40 * core.CritRatingPerCritChance,
	})
	// the overloads' own crits would hide the amount they were handed
	shaman.LightningBoltLO.BonusCritRating -= 1000 * core.CritRatingPerCritChance
	shaman.ChainLightningLO.BonusCritRating -= 1000 * core.CritRatingPerCritChance
	return sim, shaman, &hits
}

// castEvery casts spell at the first target every period, n times, then runs the sim past the last landing.
// before runs ahead of each cast.
func castEvery(t *testing.T, sim *core.Simulation, shaman *ElementalShaman, spell *core.Spell, period time.Duration, n int, before func(*core.Simulation)) {
	refill := shaman.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen})
	for i := 0; i < n; i++ {
		sim.AddPendingAction(&core.PendingAction{NextActionAt: time.Duration(i) * period, Priority: core.ActionPriorityLow,
			OnAction: func(sim *core.Simulation) {
				if before != nil {
					before(sim)
				}
				shaman.GCD.Reset()
				if spell.CD.Timer != nil {
					spell.CD.Reset()
				}
				shaman.AddMana(sim, shaman.MaxMana(), refill)
				if !spell.Cast(sim, sim.Encounter.TargetUnits[0]) {
					t.Fatalf("%s didn't cast", spell.ActionID)
				}
			}})
	}
	for sim.CurrentTime < time.Duration(n)*period && !sim.Step() {
	}
}

// overloadAmount is what spell_sha_lightning_overload hands the bolt for a hit.
func overloadAmount(hit overloadHit) float64 {
	amount := int32(hit.damage)
	if hit.crit {
		amount /= 2
	}
	return float64(amount / 2)
}

// spell_sha_lightning_overload casts 45284 for half of what the bolt dealt, a crit's halved first. The bolt adds
// its own spell power share, 0.0625 + Shamanism's 0.2 at (1 + 6) / 80 less 71.25% for its level, and takes
// Concussion's and the glyph's spell mods and the target's taken percent again, but not the raid's done
// percent (SPELL_ATTR6_IGNORE_CASTER_DAMAGE_MODIFIERS).
func TestLightningOverloadIsHalfTheBoltsDamage(t *testing.T) {
	sim, shaman, hits := overloadSim(t, overloadTalents, 1)
	castEvery(t, sim, shaman, shaman.LightningBolt, 8*time.Second, 80, nil)

	power := float32(shaman.GetStat(stats.SpellPower))
	penalty := float32(0.0875) + float32(float32(0.0875)*-71.25)/100
	bonus := float64(int32(float32(power*0.2625) * penalty))
	spellMods := (1 + 0.01*float64(shaman.Talents.Concussion)) * 1.04
	var overloads, fromCrits int
	var last *overloadHit
	for i := range *hits {
		hit := &(*hits)[i]
		if !hit.overload {
			last = hit
			continue
		}
		if last == nil {
			t.Fatal("an overload landed before any bolt")
		}
		overloads++
		if last.crit {
			fromCrits++
		}
		done := math.Trunc((overloadAmount(*last) + 1 + bonus) * spellMods)
		if want := math.Trunc(done * 1.13); hit.damage != want {
			t.Errorf("overload of a %v bolt (crit %v) dealt %v, want %v", last.damage, last.crit, hit.damage, want)
		}
		last = nil
	}
	t.Logf("%d overloads, %d off crits", overloads, fromCrits)
	if overloads < 10 || fromCrits < 3 || overloads-fromCrits < 3 {
		t.Fatalf("%d overloads, %d off crits: too few to check both halvings", overloads, fromCrits)
	}
}

// Clearcasting's Elemental Oath percent is the shaman's own shaman-family aura, so the overload takes it.
func TestLightningOverloadTakesElementalOath(t *testing.T) {
	// overloadTalents plus Elemental Focus and Elemental Oath 2
	sim, shaman, hits := overloadSim(t, "0500001500010050002300050", 1)
	clearcasting := shaman.GetAura("Clearcasting")
	if clearcasting == nil {
		t.Fatal("no Clearcasting")
	}
	// two charges a cast, so it's still up when the overload goes
	castEvery(t, sim, shaman, shaman.LightningBolt, 8*time.Second, 60, func(sim *core.Simulation) {
		clearcasting.Activate(sim)
		clearcasting.SetStacks(sim, 2)
	})

	power := float32(shaman.GetStat(stats.SpellPower))
	penalty := float32(0.0875) + float32(float32(0.0875)*-71.25)/100
	bonus := float64(int32(float32(power*0.2625) * penalty))
	oath := 1 + 0.05*float64(shaman.Talents.ElementalOath)
	spellMods := (1 + 0.01*float64(shaman.Talents.Concussion)) * 1.04
	var last *overloadHit
	var overloads int
	for i := range *hits {
		hit := &(*hits)[i]
		if !hit.overload {
			last = hit
			continue
		}
		overloads++
		done := math.Trunc((overloadAmount(*last) + 1 + bonus) * oath * spellMods)
		if want := math.Trunc(done * 1.13); hit.damage != want {
			t.Errorf("overload of a %v bolt (crit %v) under Clearcasting dealt %v, want %v", last.damage, last.crit, hit.damage, want)
		}
	}
	if overloads < 5 {
		t.Fatalf("%d overloads", overloads)
	}
}

// Elemental Focus (16164) procs ahead of Lightning Overload (30675 on) on the same hit, so a crit bolt's
// overload already has the Clearcasting that crit gave, and one off a normal bolt doesn't.
func TestLightningOverloadOffACritTakesItsClearcasting(t *testing.T) {
	sim, shaman, hits := overloadSim(t, "0500001500010050002300050", 1)
	clearcasting := shaman.GetAura("Clearcasting")
	if clearcasting == nil {
		t.Fatal("no Clearcasting")
	}
	castEvery(t, sim, shaman, shaman.LightningBolt, 8*time.Second, 120, func(sim *core.Simulation) {
		clearcasting.Deactivate(sim)
	})

	power := float32(shaman.GetStat(stats.SpellPower))
	penalty := float32(0.0875) + float32(float32(0.0875)*-71.25)/100
	bonus := float64(int32(float32(power*0.2625) * penalty))
	oath := 1 + 0.05*float64(shaman.Talents.ElementalOath)
	spellMods := (1 + 0.01*float64(shaman.Talents.Concussion)) * 1.04
	var last *overloadHit
	var fromCrits, fromHits int
	for i := range *hits {
		hit := &(*hits)[i]
		if !hit.overload {
			last = hit
			continue
		}
		done := (overloadAmount(*last) + 1 + bonus) * spellMods
		if last.crit {
			fromCrits++
			done *= oath
		} else {
			fromHits++
		}
		if want := math.Trunc(math.Trunc(done) * 1.13); hit.damage != want {
			t.Errorf("overload of a %v bolt (crit %v) dealt %v, want %v", last.damage, last.crit, hit.damage, want)
		}
	}
	if fromCrits < 3 || fromHits < 3 {
		t.Fatalf("%d overloads off crits, %d off normal hits", fromCrits, fromHits)
	}
}

// 45297 has three chain targets and the cast's 0.7 falloff: an overload off any hit jumps on from that hit's
// target. Its base value is the handed amount + 4 to 16 (level 37's 4 points and a 13 sided die, less
// CalcBaseValue's 1), its spell power share 0.2855 + 0.2 at (32 + 6) / 80.
func TestChainLightningOverloadJumps(t *testing.T) {
	sim, shaman, hits := overloadSim(t, overloadTalents, 3)
	castEvery(t, sim, shaman, shaman.ChainLightning, 7*time.Second, 120, nil)

	power := float32(shaman.GetStat(stats.SpellPower))
	bonus := float64(int32(float32(power*0.4855) * (float32(38) / 80)))
	concussion := 1 + 0.01*float64(shaman.Talents.Concussion)
	var overloads int
	check := func(hit overloadHit, group []overloadHit) {
		overloads++
		if len(group) != 3 {
			t.Fatalf("an overload hit %d targets, want 3", len(group))
		}
		if group[0].target != hit.target {
			t.Errorf("the overload started on %s, its hit was on %s", group[0].target.Label, hit.target.Label)
		}
		bounce := 1.0
		for j, jump := range group {
			if j > 0 && jump.target == group[j-1].target {
				t.Errorf("the overload jumped onto %s twice in a row", jump.target.Label)
			}
			lo := math.Trunc(math.Trunc((overloadAmount(hit)+4+bonus)*bounce*concussion) * 1.13)
			hi := math.Trunc(math.Trunc((overloadAmount(hit)+16+bonus)*bounce*concussion) * 1.13)
			if jump.damage < lo || jump.damage > hi {
				t.Errorf("jump %d of the overload of a %v hit (crit %v) dealt %v, want %v to %v", j, hit.damage, hit.crit, jump.damage, lo, hi)
			}
			bounce *= 0.7
		}
	}
	// an overload deals its jumps right after the hit it came off
	var last overloadHit
	var group []overloadHit
	for _, hit := range *hits {
		if hit.overload {
			group = append(group, hit)
			continue
		}
		if len(group) > 0 {
			check(last, group)
			group = group[:0]
		}
		last = hit
	}
	if len(group) > 0 {
		check(last, group)
	}
	t.Logf("%d overloads", overloads)
	if overloads < 10 {
		t.Fatalf("%d overloads", overloads)
	}
}

// mod-spell-tweaks' Call of Thunder adds a 70% crit damage bonus to Lightning Bolt and Chain Lightning, the
// overloads included. It adds to Elemental Fury's 100% (Player::ApplySpellMod adds the percents of this op),
// so a naked shaman's 1.5 crit becomes 1 + 0.5 * 2.7. Lava Burst and Thunderstorm are outside its class mask.
func TestCallOfThunderCritDamage(t *testing.T) {
	// overloadTalents plus Thunderstorm
	_, shaman, _ := overloadSim(t, "0500000500010050000300051", 1)
	for _, spell := range []*core.Spell{shaman.LightningBolt, shaman.LightningBoltLO, shaman.ChainLightning, shaman.ChainLightningLO} {
		if math.Abs(spell.CritMultiplier-2.35) > 1e-9 {
			t.Errorf("%s crits for %v, want 2.35", spell.ActionID, spell.CritMultiplier)
		}
	}
	if shaman.Thunderstorm == nil {
		t.Fatal("no Thunderstorm")
	}
	for _, spell := range []*core.Spell{shaman.LavaBurst, shaman.Thunderstorm} {
		if math.Abs(spell.CritMultiplier-2) > 1e-9 {
			t.Errorf("%s crits for %v, want Elemental Fury's 2", spell.ActionID, spell.CritMultiplier)
		}
	}

	// without Call of Thunder
	_, shaman, _ = overloadSim(t, "0500000500000050000300050", 1)
	if math.Abs(shaman.LightningBolt.CritMultiplier-2) > 1e-9 {
		t.Errorf("Lightning Bolt without Call of Thunder crits for %v, want 2", shaman.LightningBolt.CritMultiplier)
	}
}
