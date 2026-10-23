package restoration

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// watchHeals records the targets the spell heals, in order.
func watchHeals(shaman *RestorationShaman, spell func() *core.Spell) *[]*core.Unit {
	var order []*core.Unit
	shaman.RegisterAura(core.Aura{
		Label:    "heal order",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, healing *core.Spell, result *core.SpellResult) {
			if healing == spell() {
				order = append(order, result.Target)
			}
		},
	})
	return &order
}

// The server only jumps to hurt units, and nothing damages the sim's target dummies: a dummy counts as missing a
// sliver of health, in raid order, so Chain Heal reaches the glyph's 3 jumps across them. A real unit at full
// health, the shaman here, is skipped, and a real deficit goes before any dummy.
func TestChainHealJumpsAcrossTheDummies(t *testing.T) {
	for _, c := range []struct {
		name  string
		glyph bool
		hurt  bool
		want  func(shaman *core.Unit, dummies []*core.Unit) []*core.Unit
	}{
		{"glyphed", true, false, func(_ *core.Unit, d []*core.Unit) []*core.Unit { return d[:4] }},
		{"unglyphed", false, false, func(_ *core.Unit, d []*core.Unit) []*core.Unit { return d[:3] }},
		{"a hurt shaman first", true, true, func(s *core.Unit, d []*core.Unit) []*core.Unit {
			return []*core.Unit{d[0], s, d[1], d[2]}
		}},
	} {
		glyphs := &proto.Glyphs{}
		if c.glyph {
			glyphs = StandardGlyphs
		}
		sim, shaman := healSetup{glyphs: glyphs, dummies: 4}.build(t)
		order := watchHeals(shaman, func() *core.Spell { return shaman.ChainHeal })
		startFight(sim)
		dummies := targetDummies(sim)
		if c.hurt {
			shaman.RemoveHealth(sim, 500)
		}

		if !shaman.ChainHeal.Cast(sim, dummies[0]) {
			t.Fatalf("%s: Chain Heal wasn't cast", c.name)
		}
		runUntil(sim, 3*time.Second)
		sim.Cleanup()
		if want := c.want(&shaman.Unit, dummies); !slices.Equal(*order, want) {
			t.Errorf("%s: healed %v, want %v", c.name, unitLabels(*order), unitLabels(want))
		}
	}
}

// spell_sha_chain_heal looks for the caster's Riptide on the first target only, consumes it there and raises every
// heal of the cast by 25%: a Riptide on a later target stays, and one on the first lifts the whole chain.
func TestChainHealRiptideRaisesTheWholeChain(t *testing.T) {
	cast := func(riptideOn ...int) ([]float64, []bool) {
		sim, shaman := healSetup{glyphs: StandardGlyphs, dummies: 4}.build(t)
		sim.Reset()
		dummies := targetDummies(sim)
		neverCrit(sim, shaman, shaman.ChainHeal)
		for _, i := range riptideOn {
			shaman.Riptide.Hot(dummies[i]).Apply(sim)
		}
		shaman.ChainHeal.SkipCastAndApplyEffects(sim, dummies[0])
		var amounts []float64
		var kept []bool
		for _, dummy := range dummies {
			amounts = append(amounts, healed(shaman.ChainHeal, dummy))
			kept = append(kept, shaman.Riptide.Hot(dummy).IsActive())
		}
		return amounts, kept
	}

	plain, _ := cast()
	first, firstKept := cast(0)
	later, laterKept := cast(2)
	for i := range plain {
		if want := float64(int64(plain[i] * 1.25)); first[i] < want-2 || first[i] > want+2 {
			t.Errorf("target %d healed %v with Riptide on the first, want 25%% over %v", i, first[i], plain[i])
		}
		if later[i] != plain[i] {
			t.Errorf("target %d healed %v with Riptide on a later target, want %v", i, later[i], plain[i])
		}
	}
	if firstKept[0] {
		t.Errorf("the first target kept its Riptide")
	}
	if !laterKept[2] {
		t.Errorf("a later target lost its Riptide")
	}
}
