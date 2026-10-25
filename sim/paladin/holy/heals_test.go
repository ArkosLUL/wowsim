package holy

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// spell_pal_holy_shock: the cast heals a friend with 48821 and hurts anyone else with 48823.
func TestHolyShockHealsFriendsAndHurtsEnemies(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	friend := targetDummies(sim)[0]
	enemy := sim.Encounter.TargetUnits[0]

	cast(sim, holy.HolyShock, friend)
	cast(sim, holy.HolyShock, enemy)

	if got := holy.HolyShockHeal.SpellMetrics[friend.UnitIndex].Casts; got != 1 {
		t.Errorf("48821 cast %d times on the friend, want 1", got)
	}
	if got := holy.HolyShockDamage.SpellMetrics[friend.UnitIndex].Casts; got != 0 {
		t.Errorf("48823 cast %d times on the friend, want 0", got)
	}
	if got := holy.HolyShockDamage.SpellMetrics[enemy.UnitIndex].Casts; got != 1 {
		t.Errorf("48823 cast %d times on the enemy, want 1", got)
	}
	if got := holy.HolyShockHeal.SpellMetrics[enemy.UnitIndex].Casts; got != 0 {
		t.Errorf("48821 cast %d times on the enemy, want 0", got)
	}
}

// spell_pal_light_s_beacon: Holy Light, Flash of Light and Holy Shock on anyone but the beacon target copy the
// heal before the healed unit's taken terms, crit included, to the beacon target as 53652, 53653 and 53654. The
// copy takes none of the paladin's modifiers and can't crit, only the beacon target's healing taken.
func TestBeaconCopiesTheHealBeforeTakenTerms(t *testing.T) {
	for _, crit := range []bool{false, true} {
		sim, holy := healSetup{dummies: 2}.build(t)
		startFight(sim)
		dummies := targetDummies(sim)
		healedUnit, beaconTarget := dummies[0], dummies[1]
		setSpellPower(sim, holy, 3000)
		setCrit(sim, holy, core.TernaryFloat64(crit, 200, -100))
		fixRolls(holy.HolyLight, holy.FlashOfLight, holy.HolyShockHeal)
		healedUnit.AddHealingTakenPct(50)
		beaconTarget.AddHealingTakenPct(20)

		cast(sim, holy.BeaconOfLight, beaconTarget)
		for _, c := range []struct {
			name   string
			spell  *core.Spell
			heal   *core.Spell
			copyID int32
			base   float64
		}{
			{"Holy Light", holy.HolyLight, holy.HolyLight, 53652, 4888},
			{"Flash of Light", holy.FlashOfLight, holy.FlashOfLight, 53653, 788},
			{"Holy Shock", holy.HolyShock, holy.HolyShockHeal, 53654, 2401},
		} {
			copySpell := holy.GetSpell(core.ActionID{SpellID: c.copyID})
			done := doneHeal(c.heal, c.base)
			if crit {
				done = math.Trunc(done * c.heal.CritMultiplier)
			}
			before := healed(copySpell, beaconTarget)
			castHeal(sim, c.spell, c.heal, healedUnit)
			if got, want := healed(copySpell, beaconTarget)-before, math.Trunc(done*1.2); got != want {
				t.Errorf("crit %v, %s: beacon healed %v, want %v", crit, c.name, got, want)
			}
			if got := crits(copySpell, beaconTarget); got != 0 {
				t.Errorf("crit %v, %s: the copy crit %d times", crit, c.name, got)
			}
		}
	}
}

// No copy of a heal on the beacon target itself, and none once the beacon is gone.
func TestBeaconSkipsItsOwnTargetAndEnds(t *testing.T) {
	sim, holy := healSetup{dummies: 2}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	copySpell := holy.GetSpell(core.ActionID{SpellID: 53652})

	cast(sim, holy.BeaconOfLight, dummies[1])
	castHeal(sim, holy.HolyLight, holy.HolyLight, dummies[1])
	if got := healed(copySpell, dummies[1]); got != 0 {
		t.Errorf("a heal on the beacon target copied %v onto it", got)
	}

	runUntil(sim, time.Minute+time.Second)
	castHeal(sim, holy.HolyLight, holy.HolyLight, dummies[0])
	if got := healed(copySpell, dummies[1]); got != 0 {
		t.Errorf("a heal after the beacon's 60 s copied %v", got)
	}
}

// One beacon at a time (SPELL_ATTR5_LIMIT_N): a new target takes it off the old one.
func TestBeaconOnOneTargetAtATime(t *testing.T) {
	sim, holy := healSetup{dummies: 2}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)

	cast(sim, holy.BeaconOfLight, dummies[0])
	cast(sim, holy.BeaconOfLight, dummies[1])
	if holy.BeaconOfLightAuras.Get(dummies[0]).IsActive() {
		t.Errorf("the first target kept its beacon")
	}
	castHeal(sim, holy.HolyLight, holy.HolyLight, dummies[0])
	if got := healed(holy.GetSpell(core.ActionID{SpellID: 53652}), dummies[1]); got == 0 {
		t.Errorf("the second target got no copy")
	}
}

// Lay on Hands copies the paladin's max health, not its heal, as 53654.
func TestBeaconCopiesLayOnHandsMaxHealth(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	cast(sim, holy.BeaconOfLight, target)

	cast(sim, holy.GetSpell(core.ActionID{SpellID: 48788}), &holy.Unit)
	if got, want := healed(holy.GetSpell(core.ActionID{SpellID: 53654}), target), math.Trunc(holy.MaxHealth()); got != want {
		t.Errorf("beacon healed %v, want the paladin's max health %v", got, want)
	}
}

// spell_pal_sacred_shield: 58597 absorbs 500 plus 75% of the healing power, whole, both with Divine Guardian's 10% a
// point, and Strength of Wrynn's 30% on the sum.
func TestSacredShieldAbsorb(t *testing.T) {
	for _, c := range []struct {
		name    string
		talents string
		wrynn   bool
		want    float64
	}{
		// 500 + int32(float32(int32(2001 * 0.75)) * 1.2)
		{"Divine Guardian 2", "", false, 600 + 1800},
		{"no Divine Guardian", "50350151020013053100515221-50023131", false, 500 + 1500},
		{"Strength of Wrynn", "50350151020013053100515221-50023131", true, 2000 + 600},
	} {
		sim, holy := healSetup{talents: c.talents, dummies: 1, buffs: &proto.RaidBuffs{StrengthOfWrynn: c.wrynn}}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		setSpellPower(sim, holy, 2001)

		cast(sim, holy.SacredShield, target)
		if got := shielded(holy.SacredShieldAbsorb, target); got != c.want {
			t.Errorf("%s: absorbed %v, want %v", c.name, got, c.want)
		}
	}
}

// spell_pal_sacred_shield_dummy's cooldown is 53601's effect 0 in seconds, which Divine Guardian's 10% a point
// reaches: int32(6 × 1.1) is 6, int32(6 × 1.2) is 7.
func TestSacredShieldCooldownTakesDivineGuardian(t *testing.T) {
	for _, c := range []struct {
		talents string
		want    time.Duration
	}{
		{"50350151020013053100515221-50023131", 6 * time.Second},
		{"50350151020013053100515221-500231311", 6 * time.Second},
		{"", 7 * time.Second},
	} {
		_, holy := healSetup{talents: c.talents}.build(t)
		if got := holy.SacredShieldCooldown(); got != c.want {
			t.Errorf("Divine Guardian %d: cooldown %v, want %v", holy.Talents.DivineGuardian, got, c.want)
		}
	}
}

// Without incoming damage the target counts as hit all the time: an absorb as Sacred Shield goes on, then one a
// cooldown. A refresh keeps the clock; a new target starts it over.
func TestSacredShieldAbsorbsOnceACooldown(t *testing.T) {
	count := func(holy *HolyPaladin, target *core.Unit) int32 {
		return holy.SacredShieldAbsorb.SpellMetrics[target.UnitIndex].Hits
	}

	sim, holy := healSetup{dummies: 2}.build(t)
	startFight(sim)
	dummies := targetDummies(sim)
	cast(sim, holy.SacredShield, dummies[0])
	// 0, 7 and 14 s
	runUntil(sim, 20*time.Second)
	if got := count(holy, dummies[0]); got != 3 {
		t.Errorf("%d absorbs by 20 s, want 3", got)
	}
	// a refresh at 20 s keeps 21 and 28
	cast(sim, holy.SacredShield, dummies[0])
	runUntil(sim, 29*time.Second)
	if got := count(holy, dummies[0]); got != 5 {
		t.Errorf("%d absorbs by 29 s after a refresh, want 5", got)
	}
	// the second target's at 30 and 37, none more on the first
	runUntil(sim, 30*time.Second)
	cast(sim, holy.SacredShield, dummies[1])
	runUntil(sim, 38*time.Second)
	if got := count(holy, dummies[1]); got != 2 {
		t.Errorf("%d absorbs on the new target by 38 s, want 2", got)
	}
	if got := count(holy, dummies[0]); got != 5 {
		t.Errorf("the first target took %d absorbs, want 5", got)
	}
	if holy.SacredShieldAuras.Get(dummies[0]).IsActive() {
		t.Errorf("the first target kept Sacred Shield")
	}
}

// Sacred Shield lasts 30 s, doubled by Divine Guardian's 2 points (SPELLMOD_DURATION 50% a point).
func TestSacredShieldDurationTakesDivineGuardian(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	cast(sim, holy.SacredShield, target)
	runUntil(sim, 59*time.Second)
	if !holy.SacredShieldAuras.Get(target).IsActive() {
		t.Errorf("Sacred Shield is gone before 60 s")
	}
	runUntil(sim, 61*time.Second)
	if holy.SacredShieldAuras.Get(target).IsActive() {
		t.Errorf("Sacred Shield is still up after 60 s")
	}
}

// 58597's +50% crit chance for Flash of Light: Unit::SpellTakenCritChance looks for it cast by the healed unit
// itself, so only the paladin healing himself under his own shield gets it.
func TestSacredShieldFlashOfLightCritOnlyOnSelf(t *testing.T) {
	flashCrits := func(onSelf, shieldSelf bool) int32 {
		sim, holy := healSetup{dummies: 1}.build(t)
		startFight(sim)
		dummy := targetDummies(sim)[0]
		target := core.Ternary(onSelf, &holy.Unit, dummy)
		cast(sim, holy.SacredShield, core.Ternary(shieldSelf, &holy.Unit, dummy))
		setCritFor(sim, holy, holy.FlashOfLight, 0)
		for range 200 {
			cast(sim, holy.FlashOfLight, target)
		}
		return crits(holy.FlashOfLight, target)
	}
	if got := flashCrits(true, true); got < 70 || got > 130 {
		t.Errorf("Flash of Light on himself under his shield crit %d times in 200, want about 100", got)
	}
	if got := flashCrits(false, false); got != 0 {
		t.Errorf("Flash of Light on a shielded friend crit %d times at 0%% crit", got)
	}
	if got := flashCrits(true, false); got != 0 {
		t.Errorf("Flash of Light on himself, his shield on a friend, crit %d times at 0%% crit", got)
	}
}

// Divine Favor's +100 crit chance reaches the next Holy Light, Flash of Light or Holy Shock, which uses it up.
func TestDivineFavorCritsTheNextHeal(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]

	for _, spell := range []*core.Spell{holy.HolyLight, holy.FlashOfLight, holy.HolyShock} {
		heal := core.Ternary(spell == holy.HolyShock, holy.HolyShockHeal, spell)
		setCritFor(sim, holy, heal, 0)
		cast(sim, holy.DivineFavor, &holy.Unit)
		cast(sim, spell, target)
		cast(sim, spell, target)
		if got := crits(heal, target); got != 1 {
			t.Errorf("%s crit %d times in two casts after Divine Favor, want 1", heal.ActionID, got)
		}
		if holy.DivineFavorAura.IsActive() {
			t.Errorf("Divine Favor is still up after %s", heal.ActionID)
		}
	}
}

// 20216 has SPELL_ATTR0_COOLDOWN_ON_EVENT: no cooldown runs while the buff is up, and its 2 min start as the
// charge is used (Aura::_UnapplyForTarget).
func TestDivineFavorCooldownStartsWhenUsed(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]

	holy.DivineFavor.Cast(sim, &holy.Unit)
	runUntil(sim, 3*time.Minute)
	if holy.DivineFavor.CD.IsReady(sim) {
		t.Fatalf("Divine Favor is ready at %s with its buff still up", sim.CurrentTime)
	}

	cast(sim, holy.FlashOfLight, target)
	if holy.DivineFavorAura.IsActive() {
		t.Fatal("Flash of Light didn't use Divine Favor")
	}
	if got, want := holy.DivineFavor.CD.ReadyAt(), sim.CurrentTime+2*time.Minute; got != want {
		t.Errorf("Divine Favor ready at %s, want %s (2 min after Flash of Light used it)", got, want)
	}
}

// Divine Illumination halves every cost but physical's for 15 s (aura 72).
func TestDivineIlluminationHalvesCosts(t *testing.T) {
	sim, holy := healSetup{}.build(t)
	startFight(sim)
	cost := func() float64 { return holy.HolyLight.ApplyCostModifiers(holy.HolyLight.DefaultCast.Cost) }
	full := cost()

	cast(sim, holy.DivineIllumination, &holy.Unit)
	if got, want := cost(), math.Trunc(full*0.5); got != want {
		t.Errorf("Holy Light costs %v under Divine Illumination, want %v", got, want)
	}
	runUntil(sim, 15*time.Second+time.Millisecond)
	if got := cost(); got != full {
		t.Errorf("Holy Light costs %v after Divine Illumination, want %v", got, full)
	}
}

// Divine Plea's SPELLMOD_DAMAGE halves Holy Light, Flash of Light and Holy Shock's heal.
func TestDivinePleaHalvesTheHeals(t *testing.T) {
	sim, holy := healSetup{dummies: 1}.build(t)
	startFight(sim)
	target := targetDummies(sim)[0]
	setCrit(sim, holy, -100)
	fixRolls(holy.HolyLight, holy.FlashOfLight, holy.HolyShockHeal)

	for _, c := range []struct{ spell, heal *core.Spell }{
		{holy.HolyLight, holy.HolyLight},
		{holy.FlashOfLight, holy.FlashOfLight},
		{holy.HolyShock, holy.HolyShockHeal},
	} {
		full := castHeal(sim, c.spell, c.heal, target)
		holy.DivinePleaAura.Activate(sim)
		halved := castHeal(sim, c.spell, c.heal, target)
		holy.DivinePleaAura.Deactivate(sim)
		if want := math.Trunc((c.heal.Direct.Min + core.PowerBonus(c.heal.Direct.SP, c.heal.SpellPower())) * c.heal.DamageMultiplier * 0.5); halved != want || full <= halved {
			t.Errorf("%s heals %v under Divine Plea, %v without; want %v", c.heal.ActionID, halved, full, want)
		}
	}
}

// spell_pal_judgement: under Seal of Wisdom or Seal of Light, which have no judgement of their own in effect 2,
// a judgement also casts 54158.
func TestJudgingWisdomOrLightCasts54158(t *testing.T) {
	for _, seal := range []string{"wisdom", "light"} {
		sim, holy := healSetup{}.build(t)
		startFight(sim)
		enemy := sim.Encounter.TargetUnits[0]
		cast(sim, core.Ternary(seal == "wisdom", holy.SealOfWisdom, holy.SealOfLight), &holy.Unit)
		for range 10 {
			cast(sim, holy.JudgementOfWisdom, enemy)
		}
		landed := holy.JudgementOfWisdom.SpellMetrics[enemy.UnitIndex].Hits + holy.JudgementOfWisdom.SpellMetrics[enemy.UnitIndex].Crits
		if got := holy.SealJudgement.SpellMetrics[enemy.UnitIndex].Casts; got != landed || got == 0 {
			t.Errorf("seal of %s: 54158 cast %d times for %d landed judgements", seal, got, landed)
		}
	}
}

// The server refuses a judgement on a friend. A rotation's judgement with no target, at the dummy the paladin
// targets, isn't cast; the rotation goes on to the next action.
func TestNoJudgementOnAFriend(t *testing.T) {
	sim, holy := healSetup{dummies: 1, apl: `{"type": "TypeAPL", "priorityList": [
		{"action": {"castSpell": {"spellId": {"spellId": 53408}}}},
		{"action": {"castSpell": {"spellId": {"spellId": 20271}}}},
		{"action": {"castSpell": {"spellId": {"spellId": 48785}}}}
	]}`}.build(t)
	startFight(sim)
	dummy := targetDummies(sim)[0]
	cast(sim, holy.SealOfWisdom, &holy.Unit)
	runUntil(sim, 5*time.Second)

	for _, judgement := range []*core.Spell{holy.JudgementOfWisdom, holy.JudgementOfLight} {
		if got := judgement.SpellMetrics[dummy.UnitIndex].Casts; got != 0 {
			t.Errorf("%s cast %d times on the dummy", judgement.ActionID, got)
		}
	}
	if got := holy.FlashOfLight.SpellMetrics[dummy.UnitIndex].Casts; got == 0 {
		t.Error("the rotation didn't go on to Flash of Light")
	}
}

// Seal of Wisdom's 20168 gives 4% of max mana, rolled at 12 PPM off the main hand's speed on a melee hit, the
// judgement's 54158 included.
func TestSealOfWisdomGivesManaOffJudgements(t *testing.T) {
	// Torch of Holy Fire
	sim, holy := healSetup{items: []int32{40395}}.build(t)
	startFight(sim)
	enemy := sim.Encounter.TargetUnits[0]
	cast(sim, holy.SealOfWisdom, &holy.Unit)
	spent := holy.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionWait})

	const hits = 400
	chance := 12 * holy.AutoAttacks.MH().SwingSpeed / 60
	gain := float64(uint32(float32(uint32(holy.MaxMana())) * 4 / 100))
	procs := 0
	for range hits {
		holy.SpendMana(sim, holy.CurrentMana()-1000, spent)
		before := holy.CurrentMana()
		cast(sim, holy.SealJudgement, enemy)
		switch got := holy.CurrentMana() - before; got {
		case 0:
		case gain:
			procs++
		default:
			t.Fatalf("a proc gave %v mana, want %v", got, gain)
		}
	}
	mean, sd := hits*chance, math.Sqrt(hits*chance*(1-chance))
	if chance == 0 || math.Abs(float64(procs)-mean) > 5*sd {
		t.Errorf("%d procs in %d judgement hits, want about %.0f", procs, hits, mean)
	}
}

// Seal of Light's 20167 heals the paladin for 15% of his attack power and of his spell power, each whole, with his
// healing done percents. It can't crit.
func TestSealOfLightHeal(t *testing.T) {
	sim, holy := healSetup{items: []int32{40395}}.build(t)
	startFight(sim)
	enemy := sim.Encounter.TargetUnits[0]
	setSpellPower(sim, holy, 2001)
	setCrit(sim, holy, 200)
	cast(sim, holy.SealOfLight, &holy.Unit)
	heal := holy.GetSpell(core.ActionID{SpellID: 20167})

	for range 20 {
		cast(sim, holy.SealJudgement, enemy)
	}
	metrics := heal.SpellMetrics[holy.UnitIndex]
	if metrics.Casts == 0 {
		t.Fatalf("no Seal of Light heal in 20 judgement hits")
	}
	done := math.Trunc((core.PowerBonus(0.15, holy.GetStat(stats.AttackPower)) + core.PowerBonus(0.15, 2001)) * heal.DamageMultiplier)
	// Divinity's healing taken on himself
	want := math.Trunc(done * holy.PseudoStats.HealingTakenMultiplier)
	if got := metrics.TotalHealing / float64(metrics.Casts); got != want {
		t.Errorf("Seal of Light healed %v a proc, want %v", got, want)
	}
	if metrics.Crits != 0 {
		t.Errorf("Seal of Light crit %d times", metrics.Crits)
	}
}
