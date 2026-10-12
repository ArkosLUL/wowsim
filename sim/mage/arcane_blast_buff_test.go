package mage

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// Arcane Blast's buff (36032) lasts 6 s, and its spell_proc row takes it off as an Arcane Barrage or Arcane
// Explosion is cast. Spell::_cast works the damage out at the launch, before that proc, so the cast that
// ends the stacks still hits with them.
func TestArcaneBlastBuffEnds(t *testing.T) {
	for _, c := range []struct {
		name  string
		spell func(*Mage) *core.Spell
	}{
		{"Arcane Barrage", func(mage *Mage) *core.Spell { return mage.ArcaneBarrage }},
		{"Arcane Explosion", func(mage *Mage) *core.Spell { return mage.ArcaneExplosion }},
	} {
		sim, mage, target := arcaneSim(t, ArcaneBarrageTalents, nil, 1.06)
		// no crits, so a hit is its roll, the stacks and a partial resist
		mage.AddStatDynamic(sim, stats.SpellCrit, -200*core.CritRatingPerCritChance)
		buff, blast, spell := mage.ArcaneBlastAura, mage.ArcaneBlast, c.spell(mage)
		metrics := &spell.SpellMetrics[target.UnitIndex]

		blast.SkipCastAndApplyEffects(sim, target)
		// it counts from the world tick before the cast, and ends on a tick: within one of the live 10.27 ms
		if got := buff.RemainingDuration(sim); got <= 5989*time.Millisecond || got > 6011*time.Millisecond {
			t.Errorf("Arcane Blast's buff has %v left as it goes on, want 6s", got)
		}
		buff.Deactivate(sim)

		// the mean hit of 200 casts, each after this many Arcane Blasts
		meanHit := func(blasts int32) float64 {
			t.Helper()
			damage, hits := metrics.TotalDamage, metrics.Hits
			for range 200 {
				for range blasts {
					blast.SkipCastAndApplyEffects(sim, target)
				}
				if got := buff.GetStacks(); got != blasts {
					t.Fatalf("%s: %d Arcane Blast stacks after %d Arcane Blasts", c.name, got, blasts)
				}
				spell.SkipCastAndApplyEffects(sim, target)
				if buff.IsActive() {
					t.Fatalf("%s: %d Arcane Blast stacks left after the cast", c.name, buff.GetStacks())
				}
			}
			// Arcane Barrage's missiles are still on their way
			for landed := sim.CurrentTime + 3*time.Second; sim.CurrentTime < landed && !sim.Step(); {
			}
			if metrics.Hits == hits {
				t.Fatalf("%s: no hit in 200 casts", c.name)
			}
			return (metrics.TotalDamage - damage) / float64(metrics.Hits-hits)
		}

		perStack := 0.15
		if mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfArcaneBlast) {
			perStack = 0.18
		}
		bare, stacked := meanHit(0), meanHit(4)
		if got, want := stacked/bare, 1+4*perStack; math.Abs(got/want-1) > 0.05 {
			t.Errorf("%s: cast at four stacks it hits for x%.3f of a bare one (%.0f against %.0f), want x%.2f",
				c.name, got, stacked, bare, want)
		}
	}
}
