package restoration

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// restoTalents is StandardTalents with the restoration talents at the given 1-based positions set to a rank.
func restoTalents(ranks map[int]byte) string {
	tree := strings.LastIndex(StandardTalents, "-") + 1
	talents := []byte(StandardTalents)
	for position, rank := range ranks {
		talents[tree+position-1] = '0' + rank
	}
	return string(talents)
}

const (
	talentTidalForce  = 8
	talentHealingWay  = 12
	talentEarthShield = 23
	talentTidalWaves  = 25
	talentRiptide     = 26
)

func glyphsOf(major ...proto.ShamanMajorGlyph) *proto.Glyphs {
	glyphs := &proto.Glyphs{}
	slots := []*int32{&glyphs.Major1, &glyphs.Major2, &glyphs.Major3}
	for i, glyph := range major {
		*slots[i] = int32(glyph)
	}
	return glyphs
}

// Healing Way (aura 108, damage, on Healing Wave) is 8, 16 or 25% more, and nothing else changes.
func TestHealingWayRaisesHealingWave(t *testing.T) {
	heal := func(rank byte) (wave, lesser float64) {
		sim, shaman := healSetup{talents: restoTalents(map[int]byte{talentHealingWay: rank}), dummies: 1}.build(t)
		sim.Reset()
		neverCrit(sim, shaman, shaman.HealingWave, shaman.LesserHealingWave)
		target := targetDummies(sim)[0]
		return castHeal(sim, shaman.HealingWave, target), castHeal(sim, shaman.LesserHealingWave, target)
	}

	plainWave, plainLesser := heal(0)
	for rank, bonus := range []float64{0, .08, .16, .25} {
		wave, lesser := heal(byte(rank))
		if got := wave / plainWave; math.Abs(got-(1+bonus)) > 0.0005 {
			t.Errorf("rank %d: Healing Wave heals %.4f of rank 0, want %.2f", rank, got, 1+bonus)
		}
		if lesser != plainLesser {
			t.Errorf("rank %d: Lesser Healing Wave heals %v, want %v", rank, lesser, plainLesser)
		}
	}
}

// spell_sha_glyph_of_healing_wave heals the shaman for CalculatePct(int32(heal), 20) off each Healing Wave that
// lands on someone else, under 55533: whole, and SPELL_ATTR6_IGNORE_HEALTH_MODIFIERS keeps Purification's healing
// done percent off it.
func TestGlyphOfHealingWaveHealsTheShaman(t *testing.T) {
	sim, shaman := healSetup{glyphs: glyphsOf(proto.ShamanMajorGlyph_GlyphOfHealingWave), dummies: 1}.build(t)
	sim.Reset()
	neverCrit(sim, shaman, shaman.HealingWave)
	target := targetDummies(sim)[0]
	glyph := shaman.GetSpell(core.ActionID{SpellID: 55533})
	if glyph == nil {
		t.Fatal("Glyph of Healing Wave's heal isn't registered")
	}

	wave := castHeal(sim, shaman.HealingWave, target)
	if shaman.Talents.Purification != 5 {
		t.Fatalf("Purification %d, want 5/5 to show it isn't applied", shaman.Talents.Purification)
	}
	want := float64(int32(float32(wave) * 20 / 100))
	if got := healed(glyph, &shaman.Unit); got != want {
		t.Errorf("a %v Healing Wave healed the shaman for %v, want %v", wave, got, want)
	}

	before := healed(glyph, &shaman.Unit)
	castHeal(sim, shaman.HealingWave, &shaman.Unit)
	if got := healed(glyph, &shaman.Unit); got != before {
		t.Errorf("a Healing Wave on the shaman healed %v more through the glyph, want none", got-before)
	}

	sim, shaman = healSetup{dummies: 1}.build(t)
	if shaman.GetSpell(core.ActionID{SpellID: 55533}) != nil {
		t.Errorf("55533 is registered without the glyph")
	}
}

// Glyph of Riptide adds 6 s to the hot's 15, 2 more ticks.
func TestGlyphOfRiptideLengthensTheHot(t *testing.T) {
	for _, c := range []struct {
		name  string
		glyph bool
		ticks int32
		long  time.Duration
	}{{"with the glyph", true, 7, 21 * time.Second}, {"without", false, 5, 15 * time.Second}} {
		glyphs := &proto.Glyphs{}
		if c.glyph {
			glyphs = glyphsOf(proto.ShamanMajorGlyph_GlyphOfRiptide)
		}
		sim, shaman := healSetup{glyphs: glyphs, dummies: 1}.build(t)
		startFight(sim)
		target := targetDummies(sim)[0]
		shaman.Riptide.SkipCastAndApplyEffects(sim, target)
		hot := shaman.Riptide.Hot(target)
		if left := hot.Aura.RemainingDuration(sim); hot.TotalTicks() != c.ticks || left > c.long || left < c.long-150*time.Millisecond {
			t.Errorf("%s: Riptide ticks %d times over %s, want %d over %s", c.name, hot.TotalTicks(), left, c.ticks, c.long)
		}
	}
}

// Glyph of Mana Tide adds 1 to the 6 percent each player gets a tick.
func TestGlyphOfManaTideAddsAPercent(t *testing.T) {
	for _, c := range []struct {
		name  string
		glyph bool
		share float64
	}{{"with the glyph", true, 0.07}, {"without", false, 0.06}} {
		glyphs := &proto.Glyphs{}
		if c.glyph {
			glyphs = glyphsOf(proto.ShamanMajorGlyph_GlyphOfManaTide)
		}
		sim, shaman := healSetup{glyphs: glyphs}.build(t)
		startFight(sim)
		if !shaman.GetSpell(core.ManaTideTotemActionID).Cast(sim, &shaman.Unit) {
			t.Fatal("Mana Tide Totem wasn't cast")
		}
		spend := shaman.NewManaMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionManaRegen})
		shaman.SpendMana(sim, shaman.CurrentMana()-shaman.MaxMana()/2, spend)
		runUntil(sim, 2900*time.Millisecond)
		before := shaman.CurrentMana()
		runUntil(sim, 3100*time.Millisecond)
		if got := (shaman.CurrentMana() - before) / shaman.MaxMana(); math.Abs(got-c.share) > 0.003 {
			t.Errorf("%s: the first tick gave %.4f of the mana, want %.2f", c.name, got, c.share)
		}
	}
}

// Glyph of Water Mastery (aura 108, effect 2, +30%) and T7's 2 piece (+10%, the same spell mod) add to Water
// Shield's 100 mana per 5 s, and only with Water Shield up.
func TestWaterShieldMana(t *testing.T) {
	mp5 := func(s healSetup) float64 {
		sim, shaman := s.build(t)
		sim.Reset()
		return shaman.GetStat(stats.MP5)
	}
	mastery := glyphsOf(proto.ShamanMajorGlyph_GlyphOfWaterMastery)
	shield := proto.ShamanShield_WaterShield

	base := mp5(healSetup{shield: shield})
	if got := mp5(healSetup{shield: shield, glyphs: mastery}) - base; got != 30 {
		t.Errorf("Glyph of Water Mastery adds %v mana per 5 s, want 30", got)
	}
	// the pieces' own stats are in both
	withPieces := mp5(healSetup{shield: shield, items: t7Regalia[:2]}) - mp5(healSetup{items: t7Regalia[:2]})
	if got := withPieces - base + mp5(healSetup{}); got != 10 {
		t.Errorf("T7's 2 piece adds %v mana per 5 s to Water Shield, want 10", got)
	}
	if got := mp5(healSetup{shield: shield, glyphs: mastery, items: t7Regalia[:2]}) - mp5(healSetup{shield: shield, items: t7Regalia[:2]}); got != 30 {
		t.Errorf("the glyph adds %v with T7's 2 piece, want 30: the mods add", got)
	}
	if got := mp5(healSetup{glyphs: mastery}) - mp5(healSetup{}); got != 0 {
		t.Errorf("the glyph adds %v without Water Shield, want none", got)
	}
}

// Riptide and Earth Shield only exist with their talents, and no heal needs Tidal Waves.
func TestHealsWithoutTheirTalents(t *testing.T) {
	sim, shaman := healSetup{
		talents: restoTalents(map[int]byte{talentRiptide: 0, talentEarthShield: 0, talentTidalWaves: 0, talentTidalForce: 0}),
		dummies: 1,
	}.build(t)
	sim.Reset()
	if shaman.Riptide != nil || shaman.EarthShield != nil || shaman.TidalForce != nil {
		t.Errorf("Riptide %v, Earth Shield %v and Tidal Force %v are registered without their talents", shaman.Riptide, shaman.EarthShield, shaman.TidalForce)
	}
	target := targetDummies(sim)[0]
	for _, heal := range []*core.Spell{shaman.HealingWave, shaman.LesserHealingWave, shaman.ChainHeal} {
		if castHeal(sim, heal, target) <= 0 {
			t.Errorf("%s healed nothing without Tidal Waves", heal.ActionID)
		}
	}
}
