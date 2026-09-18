package core

import (
	"math"
	"testing"
)

// The probes in mod-sim-validation's e2e suite, as a level 80 player with maxed
// weapon skill sees them. Every expected value below was read off the live server.
func bossDummy() MeleeTableInput {
	return MeleeTableInput{
		AttackerLevel:              80,
		AttackerSkill:              400,
		AttackerMaxSkill:           400,
		DefenderLevel:              83,
		DefenderSkill:              415,
		DefenderMaxSkill:           415,
		AttackerIsPlayerOrPet:      true,
		AttackerControlledByPlayer: true,
		MissPct:                    5,
		DodgePct:                   5.85,
		ParryPct:                   13.4,
		BlockPct:                   5,
		InFront:                    true,
		CanDodge:                   true,
		CanParry:                   true,
		CanBlock:                   true,
		EnemyDodgeMultiplier:       1,
	}
}

// The level 83 elite that isn't flagged as a boss: same skill, ordinary avoidance.
func eliteDummy() MeleeTableInput {
	in := bossDummy()
	in.DodgePct = 5
	in.ParryPct = 5
	return in
}

func levelEightyDummy() MeleeTableInput {
	in := eliteDummy()
	in.DefenderLevel = 80
	in.DefenderSkill = 400
	in.DefenderMaxSkill = 400
	return in
}

func checkTable(t *testing.T, name string, got MeleeTableBP, want MeleeTableBP) {
	t.Helper()
	if got != want {
		t.Errorf("%s table = %+v, want %+v", name, got, want)
	}
}

func TestWhiteMeleeTableVsBoss(t *testing.T) {
	checkTable(t, "boss", WhiteMeleeTableBP(bossDummy()), MeleeTableBP{
		Miss:   800,
		Dodge:  645,
		Parry:  1400,
		Block:  560,
		Glance: 2500,
	})

	// Sword Specialization is 3 expertise, so 0.75% off dodge and parry.
	expertised := bossDummy()
	expertised.ExpertisePct = 0.75
	checkTable(t, "boss with 3 expertise", WhiteMeleeTableBP(expertised), MeleeTableBP{
		Miss:   800,
		Dodge:  570,
		Parry:  1325,
		Block:  560,
		Glance: 2500,
	})

	behind := bossDummy()
	behind.InFront = false
	checkTable(t, "boss from behind", WhiteMeleeTableBP(behind), MeleeTableBP{
		Miss:   800,
		Dodge:  645,
		Glance: 2500,
	})

	dualWield := bossDummy()
	dualWield.DualWield = true
	if got := WhiteMeleeTableBP(dualWield).Miss; got != 2700 {
		t.Errorf("dual wield miss = %d, want 2700", got)
	}

	hitCapped := bossDummy()
	hitCapped.HitPct = 8
	if got := WhiteMeleeTableBP(hitCapped).Miss; got != 0 {
		t.Errorf("miss at 8%% hit = %d, want 0", got)
	}
}

func TestWhiteMeleeTableVsNonBoss(t *testing.T) {
	checkTable(t, "level 83 humanoid", WhiteMeleeTableBP(eliteDummy()), MeleeTableBP{
		Miss:   800,
		Dodge:  560,
		Parry:  560,
		Block:  560,
		Glance: 2500,
	})

	beast := eliteDummy()
	beast.CanParry = false
	if got := WhiteMeleeTableBP(beast).Parry; got != 0 {
		t.Errorf("beast parry = %d, want 0", got)
	}

	checkTable(t, "level 80", WhiteMeleeTableBP(levelEightyDummy()), MeleeTableBP{
		Miss:  500,
		Dodge: 500,
		Parry: 500,
		Block: 500,
	})
}

// The skill bonus is only handed out once the chance is still positive after
// expertise, so white avoidance drops to nothing a fraction of a point early.
func TestWhiteAvoidanceExpertiseCliff(t *testing.T) {
	justUnder := bossDummy()
	justUnder.ExpertisePct = 23.39 * 0.25
	if got := WhiteMeleeTableBP(justUnder).Dodge; got != 61 {
		t.Errorf("dodge at 23.39 expertise = %d, want 61", got)
	}

	atCliff := bossDummy()
	atCliff.ExpertisePct = 23.4 * 0.25
	if got := WhiteMeleeTableBP(atCliff).Dodge; got != 0 {
		t.Errorf("dodge at 23.4 expertise = %d, want 0", got)
	}

	parryCliff := bossDummy()
	parryCliff.ExpertisePct = 53.6 * 0.25
	if got := WhiteMeleeTableBP(parryCliff).Parry; got != 0 {
		t.Errorf("parry at 53.6 expertise = %d, want 0", got)
	}
}

func TestYellowMeleeTable(t *testing.T) {
	// No dual wield penalty, no glancing, no crit, and the skill bonus goes in
	// before expertise rather than after.
	checkTable(t, "yellow vs boss", YellowMeleeTableBP(bossDummy(), YellowOptions{}), MeleeTableBP{
		Miss:  800,
		Dodge: 645,
		Parry: 1400,
	})

	blocked := YellowMeleeTableBP(bossDummy(), YellowOptions{BlockedInTable: true})
	if blocked.Block != 560 {
		t.Errorf("COMPLETELY_BLOCKED block = %d, want 560", blocked.Block)
	}

	ranged := YellowMeleeTableBP(bossDummy(), YellowOptions{Ranged: true})
	checkTable(t, "ranged", ranged, MeleeTableBP{Miss: 800})

	noDefense := YellowMeleeTableBP(bossDummy(), YellowOptions{NoActiveDefense: true})
	checkTable(t, "no active defense", noDefense, MeleeTableBP{Miss: 800})
}

// Yellow avoidance survives to the full chance, so the caps are higher than the
// white table's 23.4 and 53.6.
func TestYellowAvoidanceExpertiseCaps(t *testing.T) {
	dodge := bossDummy()
	dodge.ExpertisePct = 25.8 * 0.25
	if got := YellowMeleeTableBP(dodge, YellowOptions{}).Dodge; got != 0 {
		t.Errorf("yellow dodge at 25.8 expertise = %d, want 0", got)
	}

	almost := bossDummy()
	almost.ExpertisePct = 25.7 * 0.25
	if got := YellowMeleeTableBP(almost, YellowOptions{}).Dodge; got != 3 {
		t.Errorf("yellow dodge at 25.7 expertise = %d, want 3", got)
	}

	parry := bossDummy()
	parry.ExpertisePct = 56 * 0.25
	if got := YellowMeleeTableBP(parry, YellowOptions{}).Parry; got != 0 {
		t.Errorf("yellow parry at 56 expertise = %d, want 0", got)
	}
}

// isSpellBlocked looks up both skills without a target, which flips the sign of
// the skill term: 4.4% where the white table gives 5.6%.
func TestPartialBlock(t *testing.T) {
	if got := PartialBlockBP(5, 400, 415); got != 440 {
		t.Errorf("partial block vs boss = %d, want 440", got)
	}
	if got := PartialBlockBP(5, 400, 400); got != 500 {
		t.Errorf("partial block vs level 80 = %d, want 500", got)
	}
}

func TestGlancing(t *testing.T) {
	for _, tc := range []struct {
		defenderLevel int32
		chanceBP      int32
		multiplier    float64
	}{
		{80, 0, 1},
		{81, 1500, 0.9},
		{82, 2000, 0.8},
		{83, 2500, 0.7},
	} {
		in := eliteDummy()
		in.DefenderLevel = tc.defenderLevel
		in.DefenderSkill = MaxSkill(tc.defenderLevel)
		in.DefenderMaxSkill = in.DefenderSkill

		if got := GlanceBP(in); got != tc.chanceBP {
			t.Errorf("glance chance vs level %d = %d, want %d", tc.defenderLevel, got, tc.chanceBP)
		}
		if got := GlancingMultiplier(80, tc.defenderLevel); math.Abs(got-tc.multiplier) > 1e-12 {
			t.Errorf("glance multiplier vs level %d = %.2f, want %.2f", tc.defenderLevel, got, tc.multiplier)
		}
	}

	guardian := eliteDummy()
	guardian.AttackerIsPlayerOrPet = false
	if got := GlanceBP(guardian); got != 0 {
		t.Errorf("guardian glance chance = %d, want 0", got)
	}
}

// The boss attacking a naked level 80 warrior, from .simval taken.
func TestCreatureVsPlayerTable(t *testing.T) {
	in := MeleeTableInput{
		AttackerLevel:         83,
		AttackerSkill:         415,
		AttackerMaxSkill:      415,
		DefenderLevel:         80,
		DefenderSkill:         400,
		DefenderMaxSkill:      400,
		DefenderIsPlayer:      true,
		DefenderIsPlayerOrPet: true,
		MissPct:               5,
		BlockPct:              5,
		CritPct:               5 - float32(MeleeCritSuppressionPct(415, 400)),
		InFront:               true,
		CanDodge:              true,
		CanBlock:              true,
		EnemyDodgeMultiplier:  1,
	}

	table := WhiteMeleeTableBP(in)

	// 5% less 15 skill points at 0.02 each. The odd point is the server's float32
	// arithmetic landing just under 470.
	if table.Miss != 469 {
		t.Errorf("miss = %d, want 469", table.Miss)
	}
	if table.Block != 440 {
		t.Errorf("block = %d, want 440 (5%% less the 60bp skill bonus)", table.Block)
	}
	if table.Crit != 560 {
		t.Errorf("crit = %d, want 560", table.Crit)
	}
	if table.Crush != 0 {
		t.Errorf("crush = %d, want 0: three levels isn't enough", table.Crush)
	}

	crushing := in
	crushing.AttackerLevel = 84
	crushing.AttackerSkill = 420
	crushing.AttackerMaxSkill = 420
	if got := CrushBP(crushing); got != 2500 {
		t.Errorf("crush at +4 levels = %d, want 2500", got)
	}
}

func TestMeleeCritSuppression(t *testing.T) {
	if got := MeleeCritSuppressionPct(400, 415); math.Abs(got-0.6) > 1e-12 {
		t.Errorf("crit suppression vs boss = %.2f%%, want 0.6%%", got)
	}
	if got := MeleeCritSuppressionPct(400, 400); got != 0 {
		t.Errorf("crit suppression vs level 80 = %.2f%%, want 0", got)
	}
}

func TestSpellMiss(t *testing.T) {
	for levelDiff, want := range map[int32]int32{0: 400, 1: 500, 2: 600, 3: 1700} {
		if got := SpellMissBP(levelDiff, 0, false); got != want {
			t.Errorf("spell miss at +%d = %d, want %d", levelDiff, got, want)
		}
	}

	if got := SpellMissBP(3, 17, false); got != 0 {
		t.Errorf("spell miss at the hit cap = %d, want 0", got)
	}
	// Against a player the per-level penalty is 7 rather than 11.
	if got := SpellMissBP(3, 0, true); got != 1300 {
		t.Errorf("spell miss vs player at +3 = %d, want 1300", got)
	}
}

func TestResists(t *testing.T) {
	if got := ResistanceConstant(80); got != 400 {
		t.Errorf("resistance constant at 80 = %.1f, want 400", got)
	}

	avg := EffectiveResistChance(0, 0, 80, 83, false)
	if math.Abs(avg-15.0/415.0) > 1e-12 {
		t.Errorf("average resist vs boss = %.6f, want %.6f", avg, 15.0/415.0)
	}
	if math.Abs(avg-0.0361446) > 1e-6 {
		t.Errorf("average resist vs boss = %.6f, want 0.036145", avg)
	}

	// Spell penetration can eat the target's own resistance but not the level
	// difference's.
	if got := EffectiveResistChance(100, 200, 80, 83, false); math.Abs(got-avg) > 1e-12 {
		t.Errorf("average resist with excess penetration = %.6f, want %.6f", got, avg)
	}
	if got := EffectiveResistChance(0, 0, 80, 83, true); got != 0 {
		t.Errorf("binary resist = %.6f, want 0", got)
	}
	if got := EffectiveResistChance(100000, 0, 80, 83, false); got != 0.75 {
		t.Errorf("resist cap = %.4f, want 0.75", got)
	}
}

func TestLevelForTarget(t *testing.T) {
	if got := LevelForTarget(83, false, 80); got != 83 {
		t.Errorf("non-boss level = %d, want 83", got)
	}
	if got := LevelForTarget(80, true, 80); got != 83 {
		t.Errorf("boss level = %d, want 83: the flag beats the template", got)
	}
}

func TestCreatureBlockValue(t *testing.T) {
	if got := CreatureBlockValue(83, 0); got != 41 {
		t.Errorf("block value = %.0f, want 41", got)
	}
}
