package core

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func newTestOwner(class proto.Class, power PowerBarType, ownerStats stats.Stats) *Character {
	raid := &Raid{}
	owner := &Character{
		Class: class,
		Party: &Party{Raid: raid},
		Unit: Unit{
			Type:                  PlayerUnit,
			PseudoStats:           newPseudoStats(),
			currentPowerBar:       power,
			StatDependencyManager: stats.NewStatDependencyManager(),
		},
	}
	owner.stats = ownerStats
	return owner
}

func newTestPet(owner *Character) *Pet {
	return NewPet("Pet", owner, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, false)
}

// The scaling aura's amounts: the owner's hit chance rescaled to each stat's cap, truncated to a
// whole point.
func TestPetOwnerHitScaling(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		class                          proto.Class
		power                          PowerBarType
		scaling                        PetHitScaling
		ownerHit                       stats.Stats
		hitPct, spellHitPct, expertise float64
	}{
		{
			// Nothing caps the amounts: past 8% ranged hit the pet gets more than 17% spell hit.
			name:  "hunter over the ranged hit cap",
			class: proto.Class_ClassHunter, power: ManaBar,
			ownerHit:    stats.Stats{stats.MeleeHit: 8.5 * MeleeHitRatingPerHitChance},
			hitPct:      8,
			spellHitPct: 18,
			expertise:   27,
		},
		{
			name:  "hunter below it, every amount truncated",
			class: proto.Class_ClassHunter, power: ManaBar,
			// 7.2% -> 15.3% spell hit and 23.4 expertise
			ownerHit:    stats.Stats{stats.MeleeHit: 7.2 * MeleeHitRatingPerHitChance},
			hitPct:      7,
			spellHitPct: 15,
			expertise:   23,
		},
		{
			name:  "a hunter's spell hit doesn't count",
			class: proto.Class_ClassHunter, power: ManaBar,
			ownerHit:    stats.Stats{stats.MeleeHit: 3.4 * MeleeHitRatingPerHitChance, stats.SpellHit: 17 * SpellHitRatingPerHitChance},
			hitPct:      3,
			spellHitPct: 7,
			expertise:   11,
		},
		{
			name:  "a warlock's pet goes by spell hit, over a 17% cap",
			class: proto.Class_ClassWarlock, power: ManaBar,
			// 10% of a 17% cap -> 4.7% hit, 15.29 expertise
			ownerHit:    stats.Stats{stats.SpellHit: 10 * SpellHitRatingPerHitChance},
			hitPct:      4,
			spellHitPct: 10,
			expertise:   15,
		},
		{
			name:  "a death knight's goes by melee hit",
			class: proto.Class_ClassDeathknight, power: RunicPower,
			// 5% of an 8% cap -> 10.625% spell hit, 16.25 expertise
			ownerHit:    stats.Stats{stats.MeleeHit: 5 * MeleeHitRatingPerHitChance, stats.SpellHit: 17 * SpellHitRatingPerHitChance},
			hitPct:      5,
			spellHitPct: 10,
			expertise:   16,
		},
		{
			name:  "a risen ghoul's 67561 hands out no melee hit",
			class: proto.Class_ClassDeathknight, power: RunicPower, scaling: PetHitScalingMasterSpell06,
			ownerHit:    stats.Stats{stats.MeleeHit: 5 * MeleeHitRatingPerHitChance, stats.SpellHit: 17 * SpellHitRatingPerHitChance},
			hitPct:      0,
			spellHitPct: 10,
			expertise:   16,
		},
		{
			name:  "nor does it cap past 8% melee hit",
			class: proto.Class_ClassDeathknight, power: RunicPower, scaling: PetHitScalingMasterSpell06,
			// 9% of an 8% cap -> 19.125% spell hit, 29.25 expertise
			ownerHit:    stats.Stats{stats.MeleeHit: 9 * MeleeHitRatingPerHitChance},
			hitPct:      0,
			spellHitPct: 19,
			expertise:   29,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pet := newTestPet(newTestOwner(tc.class, tc.power, tc.ownerHit))
			pet.HitScaling = tc.scaling
			scaling := pet.ownerHitScaling()

			for _, got := range []struct {
				name       string
				have, want float64
			}{
				{"hit", scaling[stats.MeleeHit], tc.hitPct * MeleeHitRatingPerHitChance},
				{"spell hit", scaling[stats.SpellHit], tc.spellHitPct * SpellHitRatingPerHitChance},
				{"expertise", scaling[stats.Expertise], tc.expertise * ExpertisePerQuarterPercentReduction},
			} {
				if !WithinToleranceFloat64(got.want, got.have, 1e-6) {
					t.Errorf("%s: got %.4f, want %.4f", got.name, got.have, got.want)
				}
			}
		})
	}
}

// The haste carrier aura's amount is whole percent, and a slowed owner hands over nothing.
func TestPetInheritedSwingSpeed(t *testing.T) {
	for _, tc := range []struct {
		owner, want float64
	}{
		{1, 1},
		{0.8, 1},
		{1.15, 1.15},
		{1.2, 1.2},
		// Bloodlust alone: float32 leaves 1/0.7692308 - 1 just under 0.3, as it does on the server
		{1.3, 1.29},
		{1.5189343389857697, 1.51},
		{2.005, 2},
	} {
		if got := inheritedSwingSpeed(tc.owner); !WithinToleranceFloat64(tc.want, got, 1e-9) {
			t.Errorf("owner swing speed %.4f: got %.4f, want %.4f", tc.owner, got, tc.want)
		}
	}
}

// A carrier that ticks re-reads the owner's swing speed on the pet's first map update 2 s after the summon,
// and every 2 s from there: a real pet's, and a guardian's with OwnerHasteTicksOnGuardian. Any other
// guardian keeps what it took at the summon. Either way a dismissal takes off whatever is on by then.
func TestPetHasteCarrierFollowsTheOwnerWhereItTicks(t *testing.T) {
	const hasted = 1.29
	for _, tc := range []struct {
		name                           string
		summonedAsPet, ticksOnGuardian bool
		follows                        bool
	}{
		{name: "guardian", follows: false},
		{name: "guardian whose carrier ticks", ticksOnGuardian: true, follows: true},
		{name: "real pet", summonedAsPet: true, follows: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pet *timingTestPet
			timingTestConstruct = func(a *timingTestAgent) {
				pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, false, !tc.summonedAsPet)}
				pet.SummonedAsPet = tc.summonedAsPet
				pet.HasteCarrier = true
				pet.OwnerHasteSource = func() float64 { return a.SwingSpeed() }
				pet.OwnerHasteTicksOnGuardian = tc.ticksOnGuardian
				pet.EnableAutoAttacks(pet, AutoAttackOptions{MainHand: testWeapon(1.5), AutoSwingMelee: true})
				a.AddPet(pet)
			}
			sim, a := newTickTestSim(t, 100, 10, nil)
			timingTestConstruct = nil
			sim.PrePull()
			if a.SwingSpeed() != 1 {
				t.Fatalf("the owner starts at swing speed %v, want 1", a.SwingSpeed())
			}
			base := pet.PseudoStats.MeleeSpeedMultiplier
			expect := func(when string, multiplier float64) {
				t.Helper()
				if got := pet.PseudoStats.MeleeSpeedMultiplier / base; math.Abs(got-multiplier) > 1e-9 {
					t.Errorf("%s: the pet's melee speed is x%v, want x%v", when, got, multiplier)
				}
			}
			var summoned time.Duration
			summon := func() {
				summoned = sim.CurrentTime
				if sim.NextServerTick(summoned) == summoned {
					t.Fatalf("the summon at %v is on a map update, so the test shows nothing", summoned)
				}
				pet.Enable(sim, pet)
			}
			// The owner's speed changes now. The pet keeps its own until the carrier's next tick, and past
			// it too when the carrier doesn't tick.
			follow := func(when string, ownerChange, before, after float64) {
				t.Helper()
				tick := sim.NextServerTick(summoned) - sim.serverTickInterval + petOwnerHasteRefreshInterval
				for tick <= sim.CurrentTime {
					tick += petOwnerHasteRefreshInterval
				}
				if !tc.follows {
					after = before
					if pet.ownerHasteRefresh != nil {
						t.Fatalf("%s: the carrier ticks, next at %v", when, pet.ownerHasteRefresh.NextActionAt)
					}
				} else if refresh := pet.ownerHasteRefresh; refresh == nil || refresh.NextActionAt != tick {
					t.Fatalf("%s: the carrier's next tick is %+v, want one at %v", when, refresh, tick)
				}

				a.MultiplyMeleeSpeed(sim, ownerChange)
				expect(when+", at once", before)
				runUntil(sim, tick)
				expect(when+", before the tick's turn", before)
				runUntil(sim, tick+1)
				expect(when+", after the tick", after)
				want := DurationFromSeconds(1.5 / after)
				if got := pet.AutoAttacks.MainhandSwingSpeed(); got < want-time.Microsecond || got > want+time.Microsecond {
					t.Errorf("%s: the pet swings every %v, want %v", when, got, want)
				}
			}

			// the owner's haste comes 1540 ms after the summon, before the first tick, as in the live run
			runUntil(sim, ms(1234)+567*time.Microsecond)
			summon()
			expect("summoned", 1)
			runUntil(sim, summoned+ms(1540))
			follow("owner hasted after the summon", 1.3, 1, hasted)

			pet.Disable(sim)
			expect("dismissed", 1)
			if pet.ownerHasteRefresh != nil {
				t.Errorf("the carrier still ticks after the dismissal")
			}
			runUntil(sim, sim.CurrentTime+3*time.Second)
			expect("3 s after the dismissal", 1)

			// summoned under the owner's haste, every carrier has it at once
			summon()
			expect("summoned again, under the owner's haste", hasted)
			runUntil(sim, summoned+ms(2540))
			expect("past the first tick with the owner's haste as it was", hasted)
			follow("owner's haste gone", 1/1.3, hasted, 1)

			pet.Disable(sim)
			expect("dismissed again", 1)
		})
	}
}

// Unit::CalcArmorReducedDamage hands the owner's armor pen to a hunter's pet and to any risen ghoul,
// Raise Dead's guardian included, but not to the army or a hunter's guardians.
func TestPetArmorPenInheritance(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		class                proto.Class
		summonedAsPet, ghoul bool
		want                 bool
	}{
		{"hunter pet", proto.Class_ClassHunter, true, false, true},
		{"hunter guardian", proto.Class_ClassHunter, false, false, false},
		{"permanent ghoul", proto.Class_ClassDeathknight, true, true, true},
		{"raise dead ghoul", proto.Class_ClassDeathknight, false, true, true},
		{"army ghoul", proto.Class_ClassDeathknight, false, false, false},
		{"warlock pet", proto.Class_ClassWarlock, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pet := newTestPet(newTestOwner(tc.class, ManaBar, stats.Stats{}))
			pet.SummonedAsPet = tc.summonedAsPet
			pet.RisenGhoul = tc.ghoul
			pet.inheritOwnerArmorPen()
			if got := pet.armorPenSource == &pet.Owner.Unit; got != tc.want {
				t.Errorf("inherits the owner's armor pen = %v, want %v", got, tc.want)
			}
		})
	}
}

// A pet avoids like the creature it is: a flat 5% dodge that agility and defense don't move, and no
// parry.
func TestPetAvoidance(t *testing.T) {
	pet := newTestPet(newTestOwner(proto.Class_ClassHunter, ManaBar, stats.Stats{}))
	if got := pet.DodgeChance(); got != CreatureDodgeChance {
		t.Errorf("dodge %.4f, want %.4f", got, CreatureDodgeChance)
	}

	pet.stats[stats.Agility] = 2000
	if got := pet.DodgeChance(); got != CreatureDodgeChance {
		t.Errorf("dodge with agility %.4f, want %.4f", got, CreatureDodgeChance)
	}
	if got := pet.ParryChance(); got != 0 {
		t.Errorf("parry %.4f, want 0", got)
	}
}

type timingTestPet struct{ *Pet }

func (p *timingTestPet) GetPet() *Pet                        { return p.Pet }
func (p *timingTestPet) Initialize()                         {}
func (p *timingTestPet) Reset(_ *Simulation)                 {}
func (p *timingTestPet) ExecuteCustomRotation(_ *Simulation) {}

// The haste carrier makes an inheriting pet immune to Bloodlust's cast speed as well as its attack
// speed, so the pet gets none of it. Any other pet takes both, guardian or real pet alike.
func TestBloodlustSkipsAPetThatInheritsHaste(t *testing.T) {
	for _, tc := range []struct {
		name         string
		hasteCarrier bool
		isGuardian   bool
		wantPetAura  bool
	}{
		{name: "haste carrier", hasteCarrier: true, wantPetAura: false},
		{name: "plain pet", wantPetAura: true},
		{name: "plain guardian", isGuardian: true, wantPetAura: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pet *timingTestPet
			var bloodlust *Aura
			timingTestConstruct = func(a *timingTestAgent) {
				pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{}, func(stats.Stats) stats.Stats { return stats.Stats{} }, true, tc.isGuardian)}
				pet.SummonedAsPet = true
				pet.HasteCarrier = tc.hasteCarrier
				a.AddPet(pet)
			}
			sim, _ := newTimingTestSim(t, 0, func(a *timingTestAgent) {
				bloodlust = BloodlustAura(&a.Character, -1)
			})
			timingTestConstruct = nil
			sim.PrePull()
			runUntil(sim, ms(100))
			castSpeed := pet.CastSpeed
			bloodlust.Activate(sim)

			petAura := pet.GetAura(bloodlust.Label)
			if got := petAura != nil && petAura.IsActive(); got != tc.wantPetAura {
				t.Errorf("pet has Bloodlust = %v, want %v", got, tc.wantPetAura)
			}
			if hasted := pet.CastSpeed != castSpeed; hasted != tc.wantPetAura {
				t.Errorf("pet cast speed %.4f -> %.4f, want it hasted = %v", castSpeed, pet.CastSpeed, tc.wantPetAura)
			}
		})
	}
}

// a heal on an undamaged summon gains nothing, so it makes no heal threat
func TestPetSummonsAtFullHealth(t *testing.T) {
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{stats.Health: 1000},
			func(stats.Stats) stats.Stats { return stats.Stats{stats.Health: 500} }, true, false)}
		a.AddPet(pet)
	}
	sim, _ := newTimingTestSim(t, 0, nil)
	timingTestConstruct = nil
	sim.PrePull()
	runUntil(sim, ms(100))

	if !pet.HasHealthBar() || !pet.IsEnabled() {
		t.Fatalf("health bar %v, enabled %v", pet.HasHealthBar(), pet.IsEnabled())
	}
	if pet.MaxHealth() <= 1000 || pet.CurrentHealth() != pet.MaxHealth() {
		t.Errorf("health %v of %v, want full with the inherited 500 in it", pet.CurrentHealth(), pet.MaxHealth())
	}
}

// both scaling ticks without their shortcuts
func refRefreshScaling(pet *Pet, sim *Simulation) {
	scaling := pet.statInheritance(pet.Owner.GetStats())
	scaling[stats.MeleeHit] = 0
	scaling[stats.SpellHit] = 0
	scaling[stats.Expertise] = 0
	scaling = scaling.Add(pet.ownerHit)
	change := scaling.Subtract(pet.inheritedStats)
	if change.Equals(stats.Stats{}) {
		return
	}
	pet.inheritedStats = scaling
	pet.AddStatsDynamic(sim, change)
}

func refRefreshOwnerHitScaling(pet *Pet, sim *Simulation) {
	scaling := pet.ownerHitScaling()
	change := scaling.Subtract(pet.ownerHit)
	if change.Equals(stats.Stats{}) {
		return
	}
	pet.ownerHit = scaling
	pet.inheritedStats.AddInplace(&change)
	pet.AddStatsDynamic(sim, change)
}

// Twin pets of one owner, one ticking as the sim does and one through the plain ticks above, stay bit-equal
// while the owner's stats move, back and forth too, and the pets get dismissed and summoned again.
func TestPetScalingTickWalk(t *testing.T) {
	inheritance := func(owner stats.Stats) stats.Stats {
		return stats.Stats{
			stats.Stamina:     PetScalingPct(owner[stats.Stamina], 45),
			stats.AttackPower: PetScalingPct(owner[stats.RangedAttackPower]+owner[stats.Stamina]*0.3, 22),
			stats.SpellPower:  PetScalingPct(owner[stats.RangedAttackPower], 12.87),
			// not truncated, so a change and its undo can leave a residue
			stats.Armor: owner[stats.Armor] * 0.35,
			// the hit scaling's, so it's dropped
			stats.MeleeHit: owner[stats.MeleeHit],
		}
	}
	var pets [2]*timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		for i := range pets {
			pets[i] = &timingTestPet{Pet: NewPet("Pet "+string(rune('A'+i)), &a.Character, stats.Stats{stats.Health: 1000},
				inheritance, true, true)}
			a.AddPet(pets[i])
		}
	}
	sim, owner := newTimingTestSim(t, 0, nil)
	timingTestConstruct = nil
	sim.PrePull()
	cached, ref := pets[0].Pet, pets[1].Pet
	if !cached.IsEnabled() || !ref.IsEnabled() {
		t.Fatal("the pets aren't summoned")
	}

	same := func(a, b stats.Stats) bool {
		for i := range a {
			if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
				return false
			}
		}
		return true
	}
	// melee hit, which the owner's hit scaling reads, often enough that a hit tick lands between two
	// scaling ticks: undoing its change in float leaves a residue the next scaling tick has to hand over
	ownerStats := []stats.Stat{stats.Stamina, stats.RangedAttackPower, stats.Armor, stats.MeleeHit, stats.MeleeHit,
		stats.MeleeHit, stats.SpellHit, stats.MeleeHaste}
	amounts := []float64{1, -1, 0.1, 0.3, 37.5, -37.5, 400, MeleeHitRatingPerHitChance, -2 * MeleeHitRatingPerHitChance}
	var applied []stats.Stats
	rng := rand.New(rand.NewPCG(5, 6))
	still, moved := 0, 0
	for step := range 50000 {
		switch op := rng.IntN(16); {
		case op < 4:
			var change stats.Stats
			change[ownerStats[rng.IntN(len(ownerStats))]] = amounts[rng.IntN(len(amounts))]
			owner.AddStatsDynamic(sim, change)
			applied = append(applied, change)
		case op < 8:
			if n := len(applied); n > 0 {
				owner.AddStatsDynamic(sim, applied[n-1].Invert())
				applied = applied[:n-1]
			}
		case op < 11:
			cached.refreshOwnerHitScaling(sim)
			refRefreshOwnerHitScaling(ref, sim)
		case op < 15:
			before := cached.inheritedStats
			cached.refreshScaling(sim)
			refRefreshScaling(ref, sim)
			if before == cached.inheritedStats {
				still++
			} else {
				moved++
			}
		default:
			for _, pet := range pets {
				pet.Disable(sim)
				pet.Enable(sim, pet)
			}
		}

		for _, got := range []struct {
			name             string
			cached, expected stats.Stats
		}{
			{"stats", cached.stats, ref.stats},
			{"stats without deps", cached.statsWithoutDeps, ref.statsWithoutDeps},
			{"inherited", cached.inheritedStats, ref.inheritedStats},
			{"owner hit", cached.ownerHit, ref.ownerHit},
		} {
			if !same(got.cached, got.expected) {
				t.Fatalf("step %d: %s %v, want %v", step, got.name, got.cached, got.expected)
			}
		}
	}
	if still == 0 || moved == 0 {
		t.Errorf("%d ticks moved the pet, %d didn't: the walk misses a path", moved, still)
	}
}

// CalculatePct on a truncated owner value, the result truncated too.
func TestPetScalingPct(t *testing.T) {
	for _, tc := range []struct {
		owner, pct, want float64
	}{
		{520.9, 57, 296},
		{520, 300, 1560},
		{134, 30, 40},
		{1103, 12.87, 141},
		{-5, 30, 0},
	} {
		if got := PetScalingPct(tc.owner, tc.pct); got != tc.want {
			t.Errorf("%v at %v%%: got %v, want %v", tc.owner, tc.pct, got, tc.want)
		}
	}
}

// Creature::Regenerate: spirit/5 + 17 a tick, under the five second rule spirit/8 + 17 for the Imp
// and the Water Elemental and nothing for the rest.
func TestCreatureManaRegen(t *testing.T) {
	for _, tc := range []struct {
		whileCasting, underFSR bool
		want                   float64
	}{
		{false, false, 90},
		{true, false, 90},
		{true, true, 62},
		{false, true, 0},
	} {
		pet := newTestPet(newTestOwner(proto.Class_ClassWarlock, ManaBar, stats.Stats{}))
		pet.stats[stats.Spirit] = 367.6
		pet.regensWhileCasting = tc.whileCasting
		if got := pet.CreatureManaRegen(tc.underFSR); got != tc.want {
			t.Errorf("while casting %v, under the rule %v: got %v, want %v", tc.whileCasting, tc.underFSR, got, tc.want)
		}
	}
}

// A pet's own timers count from the map update before its first one and tick on the update each
// period is up by. The wait for that update isn't carried over (timer += period).
func TestPetUpdateTimersFollowTheMapUpdate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mapMs int32
		want  []time.Duration
	}{
		{"exact timing", 0, []time.Duration{ms(2000), ms(4000), ms(6000)}},
		{"live", 100, []time.Duration{ms(2000), ms(4000), ms(6000)}},
		{"updates that don't divide the period", 150, []time.Duration{ms(2100), ms(4050), ms(6000)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sim, a := newTickTestSim(t, tc.mapMs, 10, nil)
			pet := newTestPet(&a.Character)
			sim.PrePull()
			runUntil(sim, ms(1234)+567*time.Microsecond)
			now := sim.CurrentTime

			start := pet.updateTimersStart(sim, now, false)
			wantStart := now
			if tc.mapMs > 0 {
				wantStart = sim.NextServerTick(now) - sim.serverTickInterval
				if sim.NextServerTick(now) == now {
					t.Fatalf("the summon at %v is on a map update, so the test shows nothing", now)
				}
			}
			if start != wantStart {
				t.Fatalf("timers start at %v, want %v (summoned at %v)", start, wantStart, now)
			}

			var ticks []time.Duration
			pet.startUpdateTimer(sim, start, now, 2*time.Second, petAuraTickPriority, func(sim *Simulation) {
				ticks = append(ticks, sim.CurrentTime-start)
			})
			runUntil(sim, now+7*time.Second)
			if !slices.Equal(ticks, tc.want) {
				t.Errorf("ticks %v after the start, want %v", ticks, tc.want)
			}
		})
	}
}

// A pet out since before the pull is anywhere in its timers' cycles, and the 2 s and 3 s ones stay
// locked to the same start.
func TestPetOutSinceBeforeThePullIsAnywhereInItsTimers(t *testing.T) {
	sim, a := newTickTestSim(t, 100, 10, nil)
	pet := newTestPet(&a.Character)
	now := sim.CurrentTime
	noop := func(*Simulation) {}

	early, late := 0, 0
	for range 400 {
		start := pet.updateTimersStart(sim, now, true)
		if start > now || sim.NextServerTick(start) != start {
			t.Fatalf("timers start at %v: want a map update at or before %v", start, now)
		}
		scaling := pet.startUpdateTimer(sim, start, now, 2*time.Second, petAuraTickPriority, noop)
		hit := pet.startUpdateTimer(sim, start, now, 3*time.Second, petAuraTickPriority, noop)
		for _, timer := range []struct {
			next, period time.Duration
		}{{scaling.NextActionAt, 2 * time.Second}, {hit.NextActionAt, 3 * time.Second}} {
			if wait := timer.next - now; wait <= 0 || wait > timer.period || (timer.next-start)%timer.period != 0 {
				t.Fatalf("%v timer from %v ticks next at %v, now %v", timer.period, start, timer.next, now)
			}
		}
		switch wait := scaling.NextActionAt - now; {
		case wait < 500*time.Millisecond:
			early++
		case wait > 1500*time.Millisecond:
			late++
		}
		scaling.Cancel(sim)
		hit.Cancel(sim)
	}
	if early < 50 || late < 50 {
		t.Errorf("of 400 pets, %d tick in the first quarter of the period and %d in the last: want about 100 each", early, late)
	}

	exact, a := newTickTestSim(t, 0, 10, nil)
	if start := newTestPet(&a.Character).updateTimersStart(exact, exact.CurrentTime, true); start != exact.CurrentTime {
		t.Errorf("exact timing: timers start at %v, want %v", start, exact.CurrentTime)
	}
}

// A real pet picks up its owner's stats on its next scaling tick, a map update at most 2 s away, and
// regenerates on the same updates. Whatever else happens on that update comes first.
func TestPetScalingAndRegenTickOnItsOwnUpdates(t *testing.T) {
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{stats.Spirit: 100}, func(owner stats.Stats) stats.Stats {
			return stats.Stats{stats.AttackPower: owner[stats.AttackPower]}
		}, true, false)}
		pet.SummonedAsPet = true
		pet.ScalingPeriod = 2 * time.Second
		pet.EnableCreatureManaBar(1000, 0, 15, false)
		a.AddPet(pet)
	}
	sim, a := newTickTestSim(t, 100, 10, nil)
	timingTestConstruct = nil
	sim.PrePull()
	runUntil(sim, ms(3210))

	before := pet.GetStat(stats.AttackPower)
	a.AddStatsDynamic(sim, stats.Stats{stats.AttackPower: 100})
	pet.currentMana = 0

	tick := pet.scalingRefresh.NextActionAt
	if wait := tick - sim.CurrentTime; wait <= 0 || wait > 2*time.Second || sim.NextServerTick(tick) != tick {
		t.Fatalf("next scaling tick at %v, now %v: want a map update within 2 s", tick, sim.CurrentTime)
	}
	if regen := pet.creatureManaRefresh.NextActionAt; regen != tick {
		t.Errorf("next regen tick at %v, want the scaling's %v", regen, tick)
	}
	if hit := pet.ownerHitRefresh.NextActionAt; sim.NextServerTick(hit) != hit || hit-sim.CurrentTime > 3*time.Second {
		t.Errorf("next hit tick at %v, now %v: want a map update within 3 s", hit, sim.CurrentTime)
	}

	runUntil(sim, tick)
	if got := pet.GetStat(stats.AttackPower); got != before {
		t.Errorf("attack power %v before the tick's turn at %v, want %v", got, tick, before)
	}
	if pet.CurrentMana() != 0 {
		t.Errorf("mana %v before the tick's turn at %v, want 0", pet.CurrentMana(), tick)
	}
	runUntil(sim, tick+1)
	if got := pet.GetStat(stats.AttackPower); got != before+100 {
		t.Errorf("attack power %v after the tick at %v, want %v", got, tick, before+100)
	}
	if got, want := pet.CurrentMana(), pet.CreatureManaRegen(false); got != want || want != 37 {
		t.Errorf("mana %v after the tick at %v, want %v", got, tick, want)
	}
}

// A creature's intellect raises its mana, not its spell crit (Unit::SpellDoneCritChance).
func TestCreatureManaBarGivesNoCritFromIntellect(t *testing.T) {
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character,
			stats.Stats{stats.Intellect: 405, stats.SpellCrit: 5 * CritRatingPerCritChance},
			func(stats.Stats) stats.Stats { return stats.Stats{stats.Intellect: 300} }, true, false)}
		pet.EnableCreatureManaBar(2280, 405, 15, false)
		a.AddPet(pet)
	}
	sim, _ := newTimingTestSim(t, 0, nil)
	timingTestConstruct = nil
	sim.PrePull()
	runUntil(sim, ms(100))

	if got, want := pet.GetStat(stats.SpellCrit), 5*CritRatingPerCritChance; got != want {
		t.Errorf("spell crit rating %v at %v intellect, want %v", got, pet.GetStat(stats.Intellect), want)
	}
	if got, want := pet.MaxMana(), 2280+15*300.0; got != want || pet.BaseMana != 2280 {
		t.Errorf("mana %v on a base of %v, want %v on 2280", got, pet.BaseMana, want)
	}
}

// A swing in any school but physical rolls Creature::CalculateMinMaxDamage's range, the attack time on
// the weapon as well as on the attack power. A physical one keeps the sheet's.
func TestPetSwingOffPhysicalRollsTheCreatureRange(t *testing.T) {
	for _, tc := range []struct {
		name   string
		school SpellSchool
		want   float64
	}{
		// a 100 weapon at 2 s with 1400 attack power
		{"physical", SpellSchoolPhysical, 100 + 1400.0/14*2},
		{"fire", SpellSchoolFire, (100 + 1400.0/14) * 2},
		{"shadow", SpellSchoolShadow, (100 + 1400.0/14) * 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pet *timingTestPet
			timingTestConstruct = func(a *timingTestAgent) {
				pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{stats.AttackPower: 1400},
					func(stats.Stats) stats.Stats { return stats.Stats{} }, true, true)}
				pet.EnableAutoAttacks(pet, AutoAttackOptions{
					MainHand:       Weapon{BaseDamageMin: 100, BaseDamageMax: 100, SwingSpeed: 2, CritMultiplier: 2, SpellSchool: tc.school},
					AutoSwingMelee: true,
				})
				a.AddPet(pet)
			}
			sim, _ := newTimingTestSim(t, 0, nil)
			timingTestConstruct = nil

			auto, target := pet.AutoAttacks.MHAuto(), sim.Encounter.TargetUnits[0]
			// no armor and no partial resist, so a plain hit is the roll itself
			auto.Flags |= SpellFlagIgnoreResists
			got, landed := 0.0, false
			target.DynamicDamageTakenModifiers = append(target.DynamicDamageTakenModifiers, func(_ *Simulation, spell *Spell, result *SpellResult) {
				if spell == auto && !landed && result.Outcome == OutcomeHit {
					got, landed = result.Damage, true
				}
			})
			for range 200 {
				if landed {
					break
				}
				auto.ApplyEffects(sim, target, auto)
			}
			if !landed {
				t.Fatal("no plain hit in 200 swings")
			}
			if got != tc.want {
				t.Errorf("a plain swing deals %v, want %v", got, tc.want)
			}
		})
	}
}

// Unit::CalculateDamage rolls that range as a whole number between its two truncated ends, each as likely
// as the next: 300.75 to 302.25 gives 300, 301 and 302 a third of the time each.
func TestPetSwingOffPhysicalRollsAWholeNumber(t *testing.T) {
	var pet *timingTestPet
	timingTestConstruct = func(a *timingTestAgent) {
		// 1407 attack power is 100.5 a swing before the 1.5 s
		pet = &timingTestPet{Pet: NewPet("Pet", &a.Character, stats.Stats{stats.AttackPower: 1407},
			func(stats.Stats) stats.Stats { return stats.Stats{} }, true, true)}
		pet.EnableAutoAttacks(pet, AutoAttackOptions{
			MainHand:       Weapon{BaseDamageMin: 100, BaseDamageMax: 101, SwingSpeed: 1.5, CritMultiplier: 2, SpellSchool: SpellSchoolShadow},
			AutoSwingMelee: true,
		})
		a.AddPet(pet)
	}
	sim, _ := newTimingTestSim(t, 0, nil)
	timingTestConstruct = nil

	auto, target := pet.AutoAttacks.MHAuto(), sim.Encounter.TargetUnits[0]
	auto.Flags |= SpellFlagIgnoreResists
	const hits = 3000
	rolled, landed := map[float64]int{}, 0
	target.DynamicDamageTakenModifiers = append(target.DynamicDamageTakenModifiers, func(_ *Simulation, spell *Spell, result *SpellResult) {
		if spell == auto && result.Outcome == OutcomeHit && landed < hits {
			rolled[result.Damage]++
			landed++
		}
	})
	for swings := 0; landed < hits && swings < 20*hits; swings++ {
		auto.ApplyEffects(sim, target, auto)
	}
	if landed < hits {
		t.Fatalf("%d plain hits, want %d", landed, hits)
	}
	// 1000 each, give or take 5 deviations of 26
	for _, want := range []float64{300, 301, 302} {
		if n := rolled[want]; n < 870 || n > 1130 {
			t.Errorf("%d of %d plain swings deal %v, want about a third: %v", n, hits, want, rolled)
		}
	}
	if len(rolled) != 3 {
		t.Errorf("plain swings deal %v, want only 300, 301 and 302", rolled)
	}
}

// A scaling script's immunities drop the buffs of those aura types. An uncontrollable guardian only
// loses the party and raid casts, not the area auras.
func TestPetDropsBuffsItCannotTake(t *testing.T) {
	buffs := func() (*proto.RaidBuffs, *proto.PartyBuffs, *proto.IndividualBuffs) {
		return &proto.RaidBuffs{
				HornOfWinter:         true,
				StrengthOfEarthTotem: proto.TristateEffect_TristateEffectRegular,
				BattleShout:          proto.TristateEffect_TristateEffectRegular,
				TrueshotAura:         true,
				FlametongueTotem:     true,
				DevotionAura:         proto.TristateEffect_TristateEffectRegular,
				LeaderOfThePack:      proto.TristateEffect_TristateEffectRegular,
			}, &proto.PartyBuffs{}, &proto.IndividualBuffs{
				BlessingOfKings: true,
				VampiricTouch:   true,
			}
	}
	pet := newTestPet(newTestOwner(proto.Class_ClassWarlock, ManaBar, stats.Stats{}))

	pet.BuffImmunity = PetImmuneToScaling
	raid, party, individual := buffs()
	pet.dropBuffsItCannotTake(raid, party, individual)
	if raid.HornOfWinter || raid.StrengthOfEarthTotem != 0 || raid.BattleShout != 0 || raid.TrueshotAura ||
		raid.FlametongueTotem || raid.DevotionAura != 0 || individual.BlessingOfKings {
		t.Errorf("immune to its scaling's types, kept %v %v", raid, individual)
	}
	if raid.LeaderOfThePack == 0 || !individual.VampiricTouch {
		t.Errorf("immune to its scaling's types, lost Leader of the Pack or Replenishment: %v %v", raid, individual)
	}

	pet.BuffImmunity = 0
	pet.Uncontrollable = true
	raid, party, individual = buffs()
	pet.dropBuffsItCannotTake(raid, party, individual)
	if raid.HornOfWinter || raid.BattleShout != 0 || individual.BlessingOfKings || individual.VampiricTouch {
		t.Errorf("uncontrollable, kept a party or raid cast: %v %v", raid, individual)
	}
	if raid.StrengthOfEarthTotem == 0 || !raid.TrueshotAura || !raid.FlametongueTotem || raid.DevotionAura == 0 {
		t.Errorf("uncontrollable, lost an area aura: %v", raid)
	}
}
