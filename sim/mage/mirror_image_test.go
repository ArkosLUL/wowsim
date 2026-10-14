package mage

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// A Mirror Image (31216) as Guardian::InitStatsForLevel and spell_mage_pet_scaling build it: the
// fallback create stats, a creature's flat 5% crit, and from the owner at the summon 30% of his
// stamina and intellect, 33% of his spell power and 35% of his armor.
func TestMirrorImageStats(t *testing.T) {
	sim, mage := newProcSim(t, procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane))
	image := mage.mirrorImage
	image.Enable(sim, image)

	owner := mage.GetStats()
	if owner[stats.SpellPower] < 1000 || owner[stats.Intellect] < 500 {
		t.Fatalf("owner spell power %v, intellect %v: the gear set isn't on", owner[stats.SpellPower], owner[stats.Intellect])
	}
	stamina := core.PetScalingPct(owner[stats.Stamina], 30)
	intellect := core.PetScalingPct(owner[stats.Intellect], 30)
	got := image.GetStats()
	for _, tc := range []struct {
		name       string
		have, want float64
	}{
		{"strength", got[stats.Strength], 22},
		{"agility", got[stats.Agility], 22},
		{"stamina", got[stats.Stamina], 25 + stamina},
		{"intellect", got[stats.Intellect], 28 + intellect},
		{"spirit", got[stats.Spirit], 27},
		{"health", image.MaxHealth(), 4032 + 10*stamina},
		{"mana", image.MaxMana(), 8814 + 15*intellect},
		{"armor", got[stats.Armor], 4000 + core.PetScalingPct(owner[stats.Armor], 35)},
		{"attack power", got[stats.AttackPower], 2*22 - 20},
		{"spell power", got[stats.SpellPower], core.PetScalingPct(owner[stats.SpellPower], 33)},
		{"spell crit rating", got[stats.SpellCrit], 5 * core.CritRatingPerCritChance},
	} {
		if !core.WithinToleranceFloat64(tc.want, tc.have, 1e-6) {
			t.Errorf("%s %v, want %v", tc.name, tc.have, tc.want)
		}
	}
	if image.BaseMana != 2428 {
		t.Errorf("create mana %v, want 2428", image.BaseMana)
	}
	// 59638 and 59637 cost a flat 90 and 120
	if bolt, blast := image.Frostbolt.DefaultCast.Cost, image.Fireblast.DefaultCast.Cost; bolt != 90 || blast != 120 {
		t.Errorf("Frostbolt costs %v and Fire Blast %v, want 90 and 120", bolt, blast)
	}
}

// A Mirror Image has no pet bar (SummonProperties 1021), so a raid cast skips it (Spell.cpp's
// TARGET_CHECK_RAID): neither the raid's Replenishment nor Bloodlust reaches it.
func TestMirrorImageTakesNoRaidCast(t *testing.T) {
	player := procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane)
	player.Buffs = core.FullIndividualBuffs
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	mage := sim.Raid.Parties[0].Players[0].(*Mage)
	if mage.ReplenishmentAura == nil || !mage.ReplenishmentAura.IsActive() {
		t.Fatal("the mage has no Replenishment from the raid, so the test shows nothing")
	}
	image := mage.mirrorImage
	image.Enable(sim, image)
	if image.ReplenishmentAura != nil {
		t.Error("a Mirror Image carries the raid's Replenishment")
	}

	core.BloodlustAura(&mage.Character, -1).Activate(sim)
	if !mage.HasActiveAuraWithTag(core.BloodlustAuraTag) {
		t.Fatal("the mage has no Bloodlust, so the test shows nothing")
	}
	if image.HasActiveAuraWithTag(core.BloodlustAuraTag) {
		t.Error("a Mirror Image out for Bloodlust has it too")
	}
}

// npc_pet_mage_mirror_image holds an image 1.2 s, then runs one event an update: Frostbolt put back 2.5 s and
// Fire Blast 6.5 s as each is cast, nothing while a cast is going. Its 29.5 s leave room for 9 Frostbolts of
// 3 s and 3 Fire Blasts. Images out before the mage fights open with Frostbolt or Fire Blast as CasterAI
// picks, and a Fire Blast opener makes 4. Images that come into a fight were engaged at the summon with
// nothing to open on (the target's threat list takes them in InitializeAI), so they start with a Frostbolt
// an update after the 1.2 s.
func TestMirrorImageCastsOffItsEventTimers(t *testing.T) {
	// within Fire Blast's and the chase's range, where the images stand still
	player := procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane)
	player.DistanceFromTarget = 10
	// the windows and counts below are for updates of 102.7 ms, every one the same
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  core.OnRegularGrid(core.MakeSingleTargetEncounter(0), 10.27),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	mage := sim.Raid.Parties[0].Players[0].(*Mage)
	image := mage.mirrorImage
	target := sim.GetTargetUnit(0)
	if cast := image.Frostbolt.DefaultCast; cast.CastTime != 3*time.Second || cast.GCD != 0 || image.Fireblast.DefaultCast.GCD != 0 {
		t.Fatalf("Frostbolt takes %v with a GCD of %v, Fire Blast has one of %v: want 3 s and none", cast.CastTime, cast.GCD,
			image.Fireblast.DefaultCast.GCD)
	}

	var first time.Duration
	var blastFirst bool
	record := func(spell *core.Spell, blast bool) {
		effects := spell.ApplyEffects
		spell.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if first == 0 {
				first, blastFirst = sim.CurrentTime, blast
			}
			effects(sim, target, spell)
		}
	}
	record(image.Frostbolt, false)
	record(image.Fireblast, true)

	for _, tc := range []struct {
		name     string
		summonAt time.Duration
		opens    bool
	}{
		{"out before the fight", 0, true},
		{"summoned into a fight", 2 * time.Second, false},
	} {
		openers := map[bool]int{}
		for life := 0; life < 40; life++ {
			// newProcSim hands the sim over reset
			sim.Cleanup()
			sim.Reseed(int64(1000 + life))
			sim.Reset()
			sim.PrePull()
			for sim.CurrentTime < tc.summonAt && !sim.Step() {
			}
			summoned := sim.CurrentTime
			first = 0
			image.summon(sim)
			for image.IsEnabled() && !sim.Step() {
			}
			if lived := sim.CurrentTime - summoned; lived <= 29397*time.Millisecond || lived > 29610*time.Millisecond {
				t.Fatalf("%s, life %d: the images lasted %v, want 29.5 s within one update", tc.name, life, lived)
			}

			bolts := image.Frostbolt.SpellMetrics[target.UnitIndex].Casts
			blasts := image.Fireblast.SpellMetrics[target.UnitIndex].Casts
			wantBlasts := int32(3)
			at := first - summoned
			switch {
			case blastFirst && !tc.opens:
				t.Fatalf("%s, life %d: opened with Fire Blast %v in, want no opener", tc.name, life, at)
			case blastFirst:
				wantBlasts = 4
				// its opener goes off as it attacks, 12 updates in
				if at <= 1129*time.Millisecond || at > 1233*time.Millisecond {
					t.Fatalf("%s, life %d: opened with Fire Blast %v in, want 12 updates less the wait", tc.name, life, at)
				}
			case !tc.opens:
				// the update that ends the wait only picks the target, so its 3 s start on the next
				if at <= 4313*time.Millisecond || at > 4417*time.Millisecond {
					t.Fatalf("%s, life %d: the first Frostbolt landed %v in, want 43 updates less the wait", tc.name, life, at)
				}
			}
			if bolts != 9 || blasts != wantBlasts {
				t.Fatalf("%s, life %d, Fire Blast first %v: %d Frostbolts and %d Fire Blasts, want 9 and %d", tc.name, life, blastFirst,
					bolts, blasts, wantBlasts)
			}
			openers[blastFirst]++
		}
		if tc.opens && (openers[true] < 8 || openers[false] < 8) {
			t.Errorf("%s: of 40 summons %d opened with Fire Blast and %d with Frostbolt, want about half each", tc.name, openers[true],
				openers[false])
		}
	}
}

// Wrath of Air's raid area aura reaches a mage's guardians, and their bolts are magic-class spells, so the
// server cuts each cast to int32(ms * 100 / 105) (Unit::ModSpellCastTime): a Mirror Image's Frostbolt
// from 3 s to 2857 ms, the Water Elemental's Waterbolt from 2.5 s to 2380 ms.
func TestMageGuardiansCastFasterUnderWrathOfAir(t *testing.T) {
	for _, tc := range []struct {
		wrathOfAir      bool
		frostbolt, bolt time.Duration
	}{
		{false, 3000 * time.Millisecond, 2500 * time.Millisecond},
		{true, 2857 * time.Millisecond, 2380 * time.Millisecond},
	} {
		build := func(talents, gear string, spec *proto.Player_Mage) *Mage {
			sim := core.NewSim(&proto.RaidSimRequest{
				Raid: core.SinglePlayerRaidProto(procMage(talents, gear, `{"type": "TypeAPL", "priorityList": []}`, spec),
					&proto.PartyBuffs{}, &proto.RaidBuffs{WrathOfAirTotem: tc.wrathOfAir}, &proto.Debuffs{}),
				Encounter:  core.MakeSingleTargetEncounter(0),
				SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
			})
			sim.Reset()
			return sim.Raid.Parties[0].Players[0].(*Mage)
		}
		// no talents, so nothing of the mage's shortens the image's Frostbolt
		if got := build("", "p5_bis_arcane", PlayerOptionsArcane).mirrorImage.Frostbolt.CastTime(); got != tc.frostbolt {
			t.Errorf("Wrath of Air %v: a Mirror Image's Frostbolt takes %v, want %v", tc.wrathOfAir, got, tc.frostbolt)
		}
		frost := build(FrostTalents, "p3_frost_alliance", PlayerOptionsFrost)
		if frost.waterElemental == nil {
			t.Fatal("FrostTalents has no Water Elemental")
		}
		if got := frost.waterElemental.Waterbolt.CastTime(); got != tc.bolt {
			t.Errorf("Wrath of Air %v: Waterbolt takes %v, want %v", tc.wrathOfAir, got, tc.bolt)
		}
	}
}

// The corpse is unsummoned when the summon's 30 s are up, half a second after the image dies, and takes a
// Frostbolt still in the air with it: each one that goes off hits unless its flight ends past that. From point
// blank every flight is the 5 yd floor, 208 ms. From 30 yd the first one is under the 1187 ms from the mage's
// spot (the 1.5 yd reach comes off), as the image closes in between casts, and by its last Frostbolt it's near
// enough for that one to land too.
func TestMirrorImageFrostboltInTheAirAtTheUnsummonIsLost(t *testing.T) {
	for _, tc := range []struct {
		yards    float64
		travel   time.Duration
		wantLost bool
	}{
		{0, 208 * time.Millisecond, false},
		{30, 1187 * time.Millisecond, false},
	} {
		player := procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane)
		player.DistanceFromTarget = tc.yards
		sim, mage := newProcSim(t, player)
		image := mage.mirrorImage
		target := sim.GetTargetUnit(0)

		var wentOff, flights []time.Duration
		effects := image.Frostbolt.ApplyEffects
		image.Frostbolt.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			wentOff = append(wentOff, sim.CurrentTime)
			flights = append(flights, spell.TravelTime())
			effects(sim, target, spell)
		}

		lost := 0
		for life := 0; life < 40; life++ {
			// newProcSim hands the sim over reset
			sim.Cleanup()
			sim.Reseed(int64(2000 + life))
			sim.Reset()
			sim.PrePull()
			for sim.CurrentTime < 2*time.Second && !sim.Step() {
			}
			wentOff, flights = wentOff[:0], flights[:0]
			unsummon := sim.NextServerTick(image.AIClockStart(sim) + 30*time.Second)
			image.summon(sim)
			for sim.CurrentTime < unsummon+2*time.Second && !sim.Step() {
			}
			if len(flights) == 0 || flights[0] > tc.travel || (tc.yards == 0) != (flights[0] == tc.travel) {
				t.Fatalf("%v yd, life %d: the Frostbolts flew %v, want the first one %v, less once it's moved", tc.yards, life,
					flights, tc.travel)
			}

			want := 0
			for i, at := range wentOff {
				if at+flights[i] <= unsummon {
					want++
				}
			}
			m := image.Frostbolt.SpellMetrics[target.UnitIndex]
			if rolled := int(m.Hits + m.Crits + m.Misses); rolled != want {
				t.Fatalf("%v yd, life %d: %d of the %d Frostbolts that went off hit or missed, want %d (they went off at %v, the corpse goes at %v)",
					tc.yards, life, rolled, len(wentOff), want, wentOff, unsummon)
			}
			lost += len(wentOff) - want
		}
		if (lost > 0) != tc.wantLost {
			t.Errorf("%v yd: %d Frostbolts lost over 40 summons, want some = %v", tc.yards, lost, tc.wantLost)
		}
	}
}

// DeathEvent runs in the image's event pass: behind the landing of a cast that was queued before it, so a
// Frostbolt due on the update the image dies on still goes off, and ahead of UpdateAI, so the image starts
// nothing more on that update.
func TestMirrorImageCastDueOnItsDeathUpdateGoesOff(t *testing.T) {
	player := procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane)
	player.DistanceFromTarget = 0
	sim, mage := newProcSim(t, player)
	image := mage.mirrorImage

	var wentOff []time.Duration
	effects := image.Frostbolt.ApplyEffects
	image.Frostbolt.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		wentOff = append(wentOff, sim.CurrentTime)
		effects(sim, target, spell)
	}

	sim.Cleanup()
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 2*time.Second && !sim.Step() {
	}
	dies := sim.NextServerTick(image.AIClockStart(sim) + mirrorImageLife)
	image.summon(sim)
	for sim.CurrentTime < dies-time.Second && !sim.Step() {
	}
	if sim.CurrentTime >= dies || !image.IsEnabled() {
		t.Fatalf("stepped to %v, past the image's death at %v", sim.CurrentTime, dies)
	}
	// drop what it's casting and keep its AI quiet, for a Frostbolt timed to end on the very update it dies on
	image.InterruptCast(sim, image.Frostbolt)
	wentOff = wentOff[:0]
	image.frostboltDue, image.fireblastDue = core.NeverExpires, core.NeverExpires
	image.Frostbolt.DefaultCast.CastTime = dies - sim.CurrentTime
	image.Frostbolt.Cast(sim, image.CurrentTarget)
	if got := image.Hardcast.Expires; got != dies {
		t.Fatalf("the Frostbolt ends at %v, the test wants it on the death update at %v", got, dies)
	}
	for sim.CurrentTime < dies+time.Second && !sim.Step() {
	}
	if len(wentOff) != 1 || wentOff[0] != dies {
		t.Errorf("Frostbolts went off at %v, want the one due on the death update at %v", wentOff, dies)
	}
	if image.IsEnabled() {
		t.Error("the image is still out past its death")
	}
}

// npc_pet_mage_mirror_image chases its victim when it stands past 15 yd, three quarters of Fire Blast's 20
// (CasterAI), plus both reaches, but only on the updates it starts with no cast going: one update's run at 8 yd
// a second for each Frostbolt, two when a Fire Blast came between. From past ChaseRange's extra half yard the
// run stops on the chase's range (ShortenPathUntilDist), so the image parks there; from nearer it heads for
// GetNearPoint's spot 15 yd plus the target's reach out, inside it. Out past Fire Blast's 20 yd plus both
// reaches, Spell::CheckRange fails the Fire Blast as it starts, and the event's put back all the same. Inside
// the chase's range the image stands where it came up, beside its mage.
func TestMirrorImageClosesInBetweenCasts(t *testing.T) {
	const update = 0.1027 * mirrorImageRunSpeed
	for _, tc := range []struct {
		yards  float64
		blasts int32
		// where its last Frostbolt went off from, when the test pins it
		parks float64
	}{
		{30, 1, 0},
		{23, 3, mirrorImageChaseRange},
		{19.7, 3, mirrorImageChaseRange},
		{19.3, 3, 19.3 - update},
		{16, 3, 16},
	} {
		player := procMage("", "p5_bis_arcane", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsArcane)
		player.DistanceFromTarget = tc.yards
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
			Encounter:  core.OnRegularGrid(core.MakeSingleTargetEncounter(0), 10.27),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		image := sim.Raid.Parties[0].Players[0].(*Mage).mirrorImage
		target := sim.GetTargetUnit(0)

		var boltYards, blastYards []float64
		for _, spell := range []*core.Spell{image.Frostbolt, image.Fireblast} {
			effects, yards := spell.ApplyEffects, &boltYards
			if spell == image.Fireblast {
				yards = &blastYards
			}
			spell.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
				*yards = append(*yards, image.DistanceFromTarget)
				effects(sim, target, spell)
			}
		}
		sim.PrePull()
		for sim.CurrentTime < 2*time.Second && !sim.Step() {
		}
		image.summon(sim)
		for image.IsEnabled() && !sim.Step() {
		}

		t.Logf("%v yd: Frostbolts went off from %.2f yd, Fire Blasts from %.2f", tc.yards, boltYards, blastYards)
		for i, yards := range boltYards {
			before := tc.yards
			if i > 0 {
				before = boltYards[i-1]
			}
			moved, stop := before-yards, mirrorImageChaseStop
			if before >= mirrorImageChaseFar {
				stop = mirrorImageChaseRange
			}
			switch {
			case before <= mirrorImageChaseRange:
				if moved != 0 {
					t.Errorf("%v yd: Frostbolt %d went off %.2f yd out, the one before %.2f: it stands inside the chase's range",
						tc.yards, i+1, yards, before)
				}
			case yards < stop:
				t.Errorf("%v yd: Frostbolt %d went off %.2f yd out, the one before %.2f: past the run's end at %v", tc.yards,
					i+1, yards, before, stop)
			case yards > stop && (moved < update-1e-6 || moved > 2*update+1e-6):
				t.Errorf("%v yd: Frostbolt %d went off %.2f yd out, the one before %.2f: want one or two updates' run closer",
					tc.yards, i+1, yards, before)
			}
		}
		if n := len(boltYards); tc.parks != 0 && (n == 0 || math.Abs(boltYards[n-1]-tc.parks) > 1e-6) {
			t.Errorf("%v yd: Frostbolts went off from %.2f yd, want the last from %.2f", tc.yards, boltYards, tc.parks)
		}
		for i, yards := range blastYards {
			if yards >= mirrorImageFireBlastRange {
				t.Errorf("%v yd: Fire Blast %d went off from %.2f yd, out of its range", tc.yards, i+1, yards)
			}
		}
		if got := image.Fireblast.SpellMetrics[target.UnitIndex].Casts; got != tc.blasts {
			t.Errorf("%v yd: %d Fire Blasts, want %d (they went off from %v yd)", tc.yards, got, tc.blasts, blastYards)
		}
		sim.Cleanup()
	}
}
