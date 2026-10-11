package core

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// SpellDamageBonusDone adds int32(spell power × coefficient): 0.857 × 520 = 445.64 is 445.
func TestPowerBonusIsItsOwnWholeNumber(t *testing.T) {
	for _, tc := range []struct{ coefficient, power, want float64 }{
		{0.857, 520, 445},
		{0.2, 1000, 200},
		// 56.99999999999999 as a float64, 57 in the server's float32
		{0.57, 100, 57},
		// 221.00000000000003 as a float64 product, 220.99998 as the server's two float32s
		{0.13, 1700, 220},
		{-0.127, 100, -12},
		{0.5, 0, 0},
	} {
		if got := PowerBonus(tc.coefficient, tc.power); got != tc.want {
			t.Errorf("PowerBonus(%v, %v) = %v, want %v", tc.coefficient, tc.power, got, tc.want)
		}
	}
}

// SpellEffectInfo::CalcValue rolls irand(1, DieSides) on the base points: every whole value, both ends included.
func TestSpellEffectRollsWholeValues(t *testing.T) {
	sim := newResistSim()
	sim.Reset()
	effect := SpellEffect{Min: 803, Max: 806}
	seen := map[float64]int{}
	for range 4000 {
		seen[effect.Roll(sim)]++
	}
	if len(seen) != 4 {
		t.Fatalf("rolled %v, want only 803 to 806", seen)
	}
	for v := 803.0; v <= 806; v++ {
		// a quarter each, 1000 ± 5 standard deviations
		if n := seen[v]; n < 860 || n > 1140 {
			t.Errorf("rolled %v %d times of 4000, want about 1000", v, n)
		}
	}

	fixed := SpellEffect{Min: 74, Max: 74}
	if got := fixed.Roll(sim); got != 74 {
		t.Errorf("a fixed value rolled %v", got)
	}
}

// Unit::CalcAbsorbResist takes damage × i / 10, rounded down, off what SpellCriticalDamageBonus made of the
// hit. Resisting first and rounding up, as armor does, would leave 909 × 1.5 = 1363.
func TestPartialResistComesOffTheCrit(t *testing.T) {
	frostbolt := &Spell{SpellSchool: SpellSchoolFrost}
	result := &SpellResult{Damage: 1009 * 1.5, ResistanceMultiplier: 0.9}
	result.applyPartialResist(frostbolt)
	if result.Damage != 1362 {
		t.Errorf("a 10%% resist of a 1513 crit leaves %v, want 1513 - 151 = 1362", result.Damage)
	}

	unresisted := &SpellResult{Damage: 1513.5, ResistanceMultiplier: 1}
	unresisted.applyPartialResist(frostbolt)
	if unresisted.Damage != 1513.5 {
		t.Errorf("no resist changed the damage to %v", unresisted.Damage)
	}

	// armor came off before the crit
	strike := &Spell{SpellSchool: SpellSchoolPhysical}
	armored := &SpellResult{Damage: 1513.5, ResistanceMultiplier: 0.7}
	armored.applyPartialResist(strike)
	if armored.Damage != 1513.5 {
		t.Errorf("a physical hit lost its armor share twice: %v", armored.Damage)
	}
}

// A magic hit keeps its damage through the resist roll, which only notes the bracket.
func TestMagicResistWaitsForTheOutcome(t *testing.T) {
	sim := SetupFakeSim()
	fa := sim.Raid.Parties[0].Players[0].(*FakeAgent)
	spell := fa.StackingDot.Spell // holy, resisted by a creature
	target := sim.Encounter.TargetUnits[0]
	attackTable := spell.Unit.AttackTables[target.UnitIndex]

	sawResist := false
	for range 200 {
		result := spell.NewResult(target)
		result.Damage = 1009
		result.applyResistances(sim, spell, false, attackTable)
		if result.Damage != 1009 || result.PreOutcomeDamage != 1009 {
			t.Fatalf("the resist roll changed the damage to %v", result.Damage)
		}
		if result.ResistanceMultiplier != 1 {
			sawResist = true
			result.applyPartialResist(spell)
			if want := math.Ceil(1009*result.ResistanceMultiplier - 1e-6); result.Damage != want {
				t.Fatalf("a x%v resist leaves %v, want %v", result.ResistanceMultiplier, result.Damage, want)
			}
		}
		spell.DisposeResult(result)
	}
	if !sawResist {
		t.Error("200 holy hits on a level 83 creature and no partial resist")
	}
}

// AuraEffect::CalculateAmount truncates one stack's done damage, then multiplies by the stacks:
// int(99.5 × 1.07 = 106.465) × 5 = 530, where the five stacks together would be int(532.325).
func TestStackingDotTruncatesOneStack(t *testing.T) {
	sim := SetupFakeSim()
	fa := sim.Raid.Parties[0].Players[0].(*FakeAgent)
	dot := fa.StackingDot
	dot.Spell.Flags |= SpellFlagIgnoreResists
	target := sim.Encounter.TargetUnits[0]

	tick := func() float64 {
		result := dot.CalcSnapshotDamage(sim, target, dot.OutcomeTick)
		defer dot.Spell.DisposeResult(result)
		return result.Damage
	}

	dot.SnapshotBaseDamage, dot.SnapshotAttackerMultiplier, dot.SnapshotStacks = 99.5, 1.07, 5
	if got := tick(); got != 530 {
		t.Errorf("five stacks tick for %v, want 106 x 5 = 530", got)
	}
	dot.SnapshotStacks = 1
	if got := tick(); got != 106 {
		t.Errorf("one stack ticks for %v, want 106", got)
	}
	// a base that already counts its stacks
	dot.SnapshotBaseDamage, dot.SnapshotStacks = 99.5*5, 0
	if got := tick(); got != 532 {
		t.Errorf("an unstacked base ticks for %v, want 532", got)
	}
}

// SpellHealingBonusDone and SpellHealingBonusTaken each return a uint32, and SpellCriticalHealingBonus
// works in whole numbers: int(1001 × 1.07 = 1071.07) = 1071, int(1071 × 1.3 = 1392.3) = 1392, crit 2088.
func TestHealsAreWholeAtEveryStep(t *testing.T) {
	sim := SetupFakeSim()
	fa := sim.Raid.Parties[0].Players[0].(*FakeAgent)
	character := fa.GetCharacter()
	heal := character.RegisterSpell(SpellConfig{
		ActionID:         ActionID{SpellID: 43},
		SpellSchool:      SpellSchoolNature,
		ProcMask:         ProcMaskSpellHealing,
		Flags:            SpellFlagHelpful,
		DamageMultiplier: 1.07,
		CritMultiplier:   1.5,
		ThreatMultiplier: 1,
	})
	character.PseudoStats.HealingTakenMultiplier = 1.3

	hit := heal.CalcHealing(sim, &character.Unit, 1001, heal.OutcomeHealing)
	if hit.Damage != 1392 {
		t.Errorf("a heal of %v, want 1392", hit.Damage)
	}
	heal.DisposeResult(hit)

	character.AddStatDynamic(sim, stats.SpellCrit, 200*CritRatingPerCritChance)
	crit := heal.CalcHealing(sim, &character.Unit, 1001, heal.OutcomeHealingCrit)
	if !crit.DidCrit() || crit.Damage != 2088 {
		t.Errorf("a crit heal of %v (crit %v), want 2088", crit.Damage, crit.DidCrit())
	}
	heal.DisposeResult(crit)
}

// Player::CorrectMetaGemEnchants checks the meta gem's colors again on every equip: swapping out the weapon
// with the second blue gem takes Chaotic Skyflare's stats off, and the swap back returns them.
func TestItemSwapRechecksTheMetaGem(t *testing.T) {
	const chaotic = 41285
	blue := proto.GemColor_GemColorBlue
	character := &Character{}
	character.Equipment = metaTestEquipment(chaotic, blue)
	mainHand := &character.Equipment[proto.ItemSlot_ItemSlotMainHand]
	*mainHand = NewItem(ItemSpec{ID: itemTestSwapPlain}, nil)
	mainHand.Gems = []Gem{{ID: 40100, Color: blue}}
	swap := ItemSwap{character: character}
	*swap.GetItem(proto.ItemSlot_ItemSlotMainHand) = NewItem(ItemSpec{ID: itemTestSwapSocketed}, nil)

	if character.Equipment.inactiveMetaGem() != nil {
		t.Fatal("two blue gems leave Chaotic Skyflare off")
	}
	// 30 stamina and 50 crit for 40 stamina, and the meta's 21 crit goes
	got := swap.CalcStatChanges([]proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand})
	want := stats.Stats{stats.Stamina: -10, stats.MeleeCrit: 50 - 21, stats.SpellCrit: 50}
	if got != want {
		t.Errorf("stat changes = %v, want %v", got, want)
	}

	before := character.Equipment
	after := before
	after[proto.ItemSlot_ItemSlotMainHand] = *swap.GetItem(proto.ItemSlot_ItemSlotMainHand)
	if off := metaGemStatChange(&before, &after); off != (stats.Stats{stats.MeleeCrit: -21}) {
		t.Errorf("swapping the blue gem out changes %v, want -21 crit", off)
	}
	if on := metaGemStatChange(&after, &before); on != (stats.Stats{stats.MeleeCrit: 21}) {
		t.Errorf("swapping it back changes %v, want +21 crit", on)
	}
	if same := metaGemStatChange(&before, &before); same != (stats.Stats{}) {
		t.Errorf("no swap changes %v", same)
	}
}

// Rage of Rivendare is part of MeleeDamageBonusDone's DoneTotalMod, which scales the flat creature-type
// bonus with the weapon damage: (1000 + int(105 / 14 × 3.4)) × 1.1 = 1127.5.
func TestWeaponBonusForDonePct(t *testing.T) {
	unit := &Unit{Type: PlayerUnit}
	unit.PseudoStats = stats.NewPseudoStats()
	unit.PseudoStats.MobTypeAttackPower = 105
	unit.PseudoStats.DamageDealtMultiplier = 1.04
	unit.AutoAttacks.mh.Weapon = Weapon{SwingSpeed: 3.4, NormalizedSwingSpeed: 3.3}
	target := &Unit{Type: EnemyUnit}
	target.PseudoStats = stats.NewPseudoStats()

	heroicStrike := serverDataSpell(t, unit, 47450, ProcMaskMeleeMHSpecial)
	heroicStrike.SpellSchool = SpellSchoolPhysical
	heroicStrike.TotalPctMultiplier = 1
	const donePct, attackerMultiplier = 1.1, 1.04
	// the weapon damage carries the 4% as its hand's TOTAL_PCT, the flat bonus never gets it
	base := 1000/attackerMultiplier*donePct + heroicStrike.WeaponBonusForDonePct(target, donePct)
	if got := heroicStrike.doneDamage(target, base, attackerMultiplier, false); got != 1127 {
		t.Errorf("done damage %v, want (1000 + 25) x 1.1 = 1127", got)
	}

	exorcism := serverDataSpell(t, unit, 48801, ProcMaskSpellDamage)
	if got := exorcism.WeaponBonusForDonePct(target, donePct); got != 0 {
		t.Errorf("a spell without weapon damage owes %v", got)
	}
}
