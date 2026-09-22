package core

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func newOutcomeSim() *Simulation {
	return NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{},
		Encounter:  &proto.Encounter{},
		Raid:       &proto.Raid{},
	})
}

// A level 80 player swinging at the boss dummy, with crit rating chosen so the
// sheet reads a round number.
func newOutcomePair(critPct float64) (*Spell, *Unit, *AttackTable) {
	attacker := &Unit{
		Type:        PlayerUnit,
		Level:       80,
		stats:       stats.Stats{stats.MeleeCrit: critPct * CritRatingPerCritChance},
		PseudoStats: newPseudoStats(),
	}
	attacker.PseudoStats.InFrontOfTarget = true

	defender := &Unit{
		Type:        EnemyUnit,
		Level:       83,
		IsWorldBoss: true,
		PseudoStats: newPseudoStats(),
	}
	defender.PseudoStats.CanBlock = true

	spell := &Spell{
		ActionID:       ActionID{SpellID: 1},
		SpellSchool:    SpellSchoolPhysical,
		CritMultiplier: 2,
		SpellMetrics:   make([]SpellMetrics, 1),
		Unit:           attacker,
	}

	return spell, defender, NewAttackTable(attacker, defender)
}

// Crit and block are separate rolls on this server, so a yellow hit can be both.
func TestYellowCritAndBlockAreIndependent(t *testing.T) {
	const iterations = 400_000

	sim := newOutcomeSim()
	spell, defender, attackTable := newOutcomePair(40)

	var crits, blocks, both, landed int
	for i := 0; i < iterations; i++ {
		result := &SpellResult{Target: defender, Damage: 10000}
		spell.OutcomeMeleeSpecialHitAndCrit(sim, result, attackTable)

		if !result.Landed() {
			continue
		}
		landed++
		isCrit := result.Outcome.Matches(OutcomeCrit)
		isBlock := result.Outcome.Matches(OutcomeBlock)
		if isCrit {
			crits++
		}
		if isBlock {
			blocks++
		}
		if isCrit && isBlock {
			both++
		}
	}

	if landed == 0 {
		t.Fatal("nothing landed")
	}

	pCrit := float64(crits) / float64(landed)
	pBlock := float64(blocks) / float64(landed)
	pBoth := float64(both) / float64(landed)

	// 40% crit rating less the 0.6% the skill difference takes off.
	if math.Abs(pCrit-0.394) > 0.005 {
		t.Errorf("crit rate = %.4f, want 0.394", pCrit)
	}
	if math.Abs(pBlock-0.044) > 0.003 {
		t.Errorf("block rate = %.4f, want 0.044", pBlock)
	}
	if math.Abs(pBoth-pCrit*pBlock) > 0.003 {
		t.Errorf("P(crit and block) = %.4f, want %.4f", pBoth, pCrit*pBlock)
	}
}

// SPELL_ATTR3_ALWAYS_HIT spells (Shiv, the judgements) skip the yellow table
// outright, per Unit::MeleeSpellHitResult, but crit still rolls independently
// and isSpellBlocked still exempts them from the partial block roll.
func TestAlwaysHitNeverMissesOrBlocks(t *testing.T) {
	const iterations = 200_000

	sim := newOutcomeSim()
	spell, defender, attackTable := newOutcomePair(40)
	spell.Flags |= SpellFlagAlwaysHit

	crits, blocks := 0, 0
	for i := 0; i < iterations; i++ {
		result := &SpellResult{Target: defender, Damage: 10000}
		spell.OutcomeMeleeSpecialHitAndCrit(sim, result, attackTable)

		if !result.Landed() {
			t.Fatalf("an always-hit spell missed: %s", result.Outcome)
		}
		if result.Outcome.Matches(OutcomeCrit) {
			crits++
		}
		if result.Outcome.Matches(OutcomeBlock) {
			blocks++
		}
	}

	if blocks != 0 {
		t.Errorf("always-hit blocks = %d, want 0", blocks)
	}
	// Same 39.4% as TestYellowCritAndBlockAreIndependent: crit is unaffected.
	if pCrit := float64(crits) / iterations; math.Abs(pCrit-0.394) > 0.005 {
		t.Errorf("crit rate = %.4f, want 0.394", pCrit)
	}
}

// The white table's rates, which the e2e suite read off the server over a million
// swings.
func TestWhiteSwingRates(t *testing.T) {
	const iterations = 400_000

	sim := newOutcomeSim()
	spell, defender, attackTable := newOutcomePair(20)

	counts := map[HitOutcome]int{}
	for i := 0; i < iterations; i++ {
		result := &SpellResult{Target: defender, Damage: 10000}
		spell.OutcomeMeleeWhite(sim, result, attackTable)
		counts[result.Outcome]++
	}

	for _, tc := range []struct {
		outcome HitOutcome
		name    string
		want    float64
	}{
		{OutcomeMiss, "miss", 0.08},
		{OutcomeDodge, "dodge", 0.0645},
		{OutcomeParry, "parry", 0.14},
		{OutcomeHit | OutcomeBlock, "block", 0.056},
		{OutcomeGlance, "glance", 0.25},
		{OutcomeCrit, "crit", 0.194},
	} {
		got := float64(counts[tc.outcome]) / iterations
		if math.Abs(got-tc.want) > 0.004 {
			t.Errorf("%s rate = %.4f, want %.4f", tc.name, got, tc.want)
		}
	}
}

// Magic spells roll irand(1, 10000), which leaves the true miss rate a hundredth
// of a point under the nominal 17%.
func TestMagicMissRate(t *testing.T) {
	const iterations = 400_000

	sim := newOutcomeSim()
	spell, _, attackTable := newOutcomePair(0)
	spell.SpellSchool = SpellSchoolFrost

	if want := 0.1699; math.Abs(spell.SpellChanceToMiss(attackTable)-want) > 1e-9 {
		t.Errorf("spell miss chance = %.6f, want %.4f", spell.SpellChanceToMiss(attackTable), want)
	}

	misses := 0
	for i := 0; i < iterations; i++ {
		if !spell.MagicHitCheck(sim, attackTable) {
			misses++
		}
	}

	if got := float64(misses) / iterations; math.Abs(got-0.1699) > 0.003 {
		t.Errorf("magic miss rate = %.4f, want 0.1699", got)
	}
}

// A level 83 boss swinging at a bare level 80 player, the other way round from
// newOutcomePair.
func newEnemyPair() (*Spell, *Unit, *AttackTable) {
	boss := &Unit{
		Type:        EnemyUnit,
		Level:       83,
		IsWorldBoss: true,
		stats:       stats.Stats{stats.MeleeCrit: 5 * CritRatingPerCritChance},
		PseudoStats: newPseudoStats(),
	}
	boss.PseudoStats.InFrontOfTarget = true

	player := &Unit{
		Type:        PlayerUnit,
		Level:       80,
		PseudoStats: newPseudoStats(),
	}

	spell := &Spell{
		ActionID:       ActionID{SpellID: 2},
		SpellSchool:    SpellSchoolPhysical,
		CritMultiplier: 2,
		SpellMetrics:   make([]SpellMetrics, 1),
		Unit:           boss,
	}

	return spell, player, NewAttackTable(boss, player)
}

// Chill of the Throne sits on the boss as a cut to its own swings being dodged,
// so it takes 20% off the tank's dodge and leaves the boss's dodge alone.
func TestDodgeReductionOnlyAppliesToTheAttacker(t *testing.T) {
	spell, boss, attackTable := newOutcomePair(0)
	boss.PseudoStats.DodgeReduction = 0.2
	if got := WhiteMeleeTableBP(spell.WhiteTableInput(attackTable)).Dodge; got != 645 {
		t.Errorf("boss dodge = %d, want 645 whatever its own dodge reduction", got)
	}

	bossSwing, tank, bossTable := newEnemyPair()
	bossSwing.Unit.PseudoStats.DodgeReduction = 0.2
	tank.PseudoStats.BaseDodge = 0.3
	// 30% less 20%, less the boss's 0.6% skill bonus.
	if got := WhiteMeleeTableBP(bossSwing.WhiteTableInput(bossTable)).Dodge; got != 940 {
		t.Errorf("tank dodge under Chill of the Throne = %d, want 940", got)
	}
}

// Each swing lands in exactly one counter, since the results page adds them up
// into attempts: a blocked crit is a crit and a blocked hit is a block.
func TestYellowMetricsCountEachSwingOnce(t *testing.T) {
	const iterations = 100_000

	sim := newOutcomeSim()
	spell, defender, attackTable := newOutcomePair(40)

	blockedHits := 0
	for i := 0; i < iterations; i++ {
		result := &SpellResult{Target: defender, Damage: 10000}
		spell.OutcomeMeleeSpecialHitAndCrit(sim, result, attackTable)
		if result.Outcome.Matches(OutcomeBlock) && !result.Outcome.Matches(OutcomeCrit) {
			blockedHits++
		}
	}

	m := spell.SpellMetrics[0]
	if total := m.Misses + m.Dodges + m.Parries + m.Hits + m.Crits + m.Blocks + m.Glances + m.Crushes; total != iterations {
		t.Errorf("outcomes counted = %d, want %d", total, iterations)
	}
	if blockedHits == 0 || int(m.Blocks) != blockedHits {
		t.Errorf("blocks counted = %d, want %d (and more than none)", m.Blocks, blockedHits)
	}
}

// A hit check with no damage, like Sunder Armor's, never reaches the server's
// block roll.
func TestNoPartialBlockOnHitChecks(t *testing.T) {
	sim := newOutcomeSim()
	spell, defender, attackTable := newOutcomePair(0)

	for i := 0; i < 20_000; i++ {
		result := &SpellResult{Target: defender}
		spell.OutcomeMeleeSpecialHit(sim, result, attackTable)
		if result.Outcome.Matches(OutcomeBlock) {
			t.Fatalf("a hit check was blocked: %s", result.Outcome)
		}
	}
}

// Hits whose table roll happened elsewhere, like Mutilate's strikes, still take
// the 4.4% block roll.
func TestCritOnlyHitsCanBeBlocked(t *testing.T) {
	const iterations = 200_000

	sim := newOutcomeSim()
	spell, defender, attackTable := newOutcomePair(0)

	blocks := 0
	for i := 0; i < iterations; i++ {
		result := &SpellResult{Target: defender, Damage: 10000}
		spell.OutcomeMeleeSpecialCritOnly(sim, result, attackTable)
		if result.Outcome.Matches(OutcomeBlock) {
			blocks++
		}
	}

	if got := float64(blocks) / iterations; math.Abs(got-0.044) > 0.003 {
		t.Errorf("block rate = %.4f, want 0.044", got)
	}
}

// isSpellBlocked exempts NO_ACTIVE_DEFENSE and ALWAYS_HIT spells, so a secondary
// judgement (NoActiveDefense) or Righteous Vengeance's tick (AlwaysHit) never
// takes the 4.4% roll TestCritOnlyHitsCanBeBlocked found.
func TestCritOnlyHitsSkipBlockWhenExempt(t *testing.T) {
	const iterations = 200_000

	for _, tc := range []struct {
		name string
		flag SpellFlag
	}{
		{"no active defense", SpellFlagNoActiveDefense},
		{"always hit", SpellFlagAlwaysHit},
	} {
		sim := newOutcomeSim()
		spell, defender, attackTable := newOutcomePair(0)
		spell.Flags |= tc.flag

		blocks := 0
		for i := 0; i < iterations; i++ {
			result := &SpellResult{Target: defender, Damage: 10000}
			spell.OutcomeMeleeSpecialCritOnly(sim, result, attackTable)
			if result.Outcome.Matches(OutcomeBlock) {
				blocks++
			}
		}

		if blocks != 0 {
			t.Errorf("%s: blocks = %d, want 0", tc.name, blocks)
		}
	}
}

// A block in the yellow table is SPELL_MISS_BLOCK, which stops the spell
// outright instead of letting it land for less.
func TestCompletelyBlockedSpellsDontLand(t *testing.T) {
	const iterations = 200_000

	sim := newOutcomeSim()
	spell, defender, attackTable := newOutcomePair(0)
	spell.Flags |= SpellFlagCompletelyBlocked

	blocks := 0
	for i := 0; i < iterations; i++ {
		result := &SpellResult{Target: defender}
		spell.OutcomeMeleeSpecialHit(sim, result, attackTable)
		if !result.Outcome.Matches(OutcomeBlock) {
			continue
		}
		blocks++
		if result.Landed() {
			t.Fatalf("a full block landed: %s", result.Outcome)
		}
	}

	// 5% plus the boss's 0.6% skill bonus, as in the white table.
	if got := float64(blocks) / iterations; math.Abs(got-0.056) > 0.003 {
		t.Errorf("full block rate = %.4f, want 0.056", got)
	}
}

// A creature's yellow hits read the player's own sheet: dodge, block (which
// needs a shield) and the defense skill that pushes crits down.
func TestEnemyYellowHitsAgainstPlayer(t *testing.T) {
	spell, player, attackTable := newEnemyPair()
	player.PseudoStats.BaseDodge = 0.1
	player.stats[stats.Block] = 20 * BlockRatingPerBlockChance

	// 10% less the boss's 0.6% skill bonus.
	if got := YellowMeleeTableBP(spell.YellowTableInput(attackTable), YellowOptions{}).Dodge; got != 940 {
		t.Errorf("yellow dodge = %d, want 940", got)
	}

	if got := attackTable.partialBlockBP(); got != 0 {
		t.Errorf("partial block without a shield = %d, want 0", got)
	}
	player.PseudoStats.CanBlock = true
	// 25% off the sheet, and isSpellBlocked adds the boss's 0.6% on top.
	if got := attackTable.partialBlockBP(); got != 2560 {
		t.Errorf("partial block with a shield = %d, want 2560", got)
	}

	// 5% plus the 0.6% the boss's skill adds against a player with no defense.
	if got := spell.PhysicalCritChance(attackTable); math.Abs(got-0.056) > 1e-9 {
		t.Errorf("crit chance = %.4f, want 0.056", got)
	}
	player.stats[stats.Defense] = 100 * DefenseRatingPerDefense
	want := 0.056 - float64(DefenseSkillFromRating(player.stats[stats.Defense]))*PercentPerSkillPoint/100
	if got := spell.PhysicalCritChance(attackTable); math.Abs(got-want) > 1e-9 {
		t.Errorf("crit chance with defense = %.4f, want %.4f", got, want)
	}
}

// Only real pets glance. Spirit wolves, treants and the like are guardians to
// the server.
func TestOnlyPlayersAndPetsGlance(t *testing.T) {
	boss := &Unit{Type: EnemyUnit, Level: 83, IsWorldBoss: true, PseudoStats: newPseudoStats()}
	guardian := &Unit{Type: PetUnit, Level: 80, PseudoStats: newPseudoStats()}
	pet := &Unit{Type: PetUnit, Level: 80, SummonedAsPet: true, PseudoStats: newPseudoStats()}

	if got := GlanceBP(NewAttackTable(guardian, boss).meleeTableInput()); got != 0 {
		t.Errorf("guardian glance chance = %d, want 0", got)
	}
	if got := GlanceBP(NewAttackTable(pet, boss).meleeTableInput()); got != 2500 {
		t.Errorf("pet glance chance = %d, want 2500", got)
	}
}

// The boss flag defaults on at raid boss level, but the encounter can still turn
// it off, or on for a lower level creature.
func TestWorldBossFlag(t *testing.T) {
	on, off := true, false
	for _, tc := range []struct {
		name    string
		options *proto.Target
		want    bool
	}{
		{"level 83, unset", &proto.Target{Level: 83}, true},
		{"level 83, turned off", &proto.Target{Level: 83, WorldBoss: &off}, false},
		{"level 80, unset", &proto.Target{Level: 80}, false},
		{"level 80, turned on", &proto.Target{Level: 80, WorldBoss: &on}, true},
	} {
		if got := NewTarget(tc.options, 0).IsWorldBoss; got != tc.want {
			t.Errorf("%s: world boss = %t, want %t", tc.name, got, tc.want)
		}
	}
}
