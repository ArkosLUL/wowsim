package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (shaman *Shaman) registerChainLightningSpell() {
	shaman.ChainLightning = shaman.newChainLightningSpell(false)
	shaman.ChainLightningLO = shaman.newChainLightningSpell(true)
}

func (shaman *Shaman) newChainLightningSpell(isLightningOverload bool) *core.Spell {
	spellConfig := shaman.newElectricSpellConfig(
		core.ActionID{SpellID: 49271},
		0.26,
		time.Millisecond*2000,
		isLightningOverload)
	spellConfig.Direct = core.SpellEffect{Effect: 0, Min: 973, Max: 1111, SP: 0.571}
	spellConfig.Mods = []core.SpellMod{{Op: core.SpellModBonusMultiplier, Flat: 4 * shaman.Talents.Shamanism}}

	if !isLightningOverload {
		spellConfig.Cast.CD = core.Cooldown{
			Timer:    shaman.NewTimer(),
			Duration: time.Second*6 - []time.Duration{0, 750 * time.Millisecond, 1500 * time.Millisecond, 2500 * time.Millisecond}[shaman.Talents.StormEarthAndFire],
		}
	}

	numHits := min(core.TernaryInt32(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfChainLightning), 4, 3), shaman.Env.GetNumTargets())
	dmgReductionPerBounce := core.TernaryFloat64(shaman.HasSetBonus(ItemSetTidefury, 2), 0.7*1.19, 0.7)
	relicSpellPower := shaman.electricSpellRelicSpellPower()

	if isLightningOverload {
		// 45297 jumps like the cast, from the target whose hit it came off
		spellConfig.Direct = core.SpellEffect{Effect: 0, FromSpellID: overloadChainSpellID, Min: 100, Max: 112, SP: 0.2855}
		bolt := newOverloadBolt(overloadChainSpellID)
		spellConfig.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			bounceCoeff := 1.0
			curTarget := target
			for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
				result := bolt.calcDamage(sim, shaman, spell, curTarget, shaman.overloadAmount, relicSpellPower, bounceCoeff)
				spell.DealDamage(sim, result)

				bounceCoeff *= dmgReductionPerBounce
				curTarget = sim.Environment.NextTargetUnit(curTarget)
			}
		}
		return shaman.RegisterSpell(spellConfig)
	}

	canLO := shaman.Talents.LightningOverload > 0
	// the aura's chance, then the script's roll_chance_i(33) for Chain Lightning
	lightningOverloadChance := float64(shaman.Talents.LightningOverload) * 0.11 * 0.33

	spellConfig.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		bounceCoeff := 1.0
		curTarget := target
		for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
			baseDamage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, (spell.SpellPower()+relicSpellPower))
			baseDamage *= bounceCoeff
			result := spell.CalcDamage(sim, curTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.DealDamage(sim, result)

			// after the hit's Elemental Focus proc, as on Lightning Bolt
			if canLO && result.Landed() && sim.RandomFloat("CL Lightning Overload") < lightningOverloadChance {
				shaman.overloadAmount = overloadBasePoints(result)
				shaman.ChainLightningLO.Cast(sim, curTarget)
			}

			bounceCoeff *= dmgReductionPerBounce
			curTarget = sim.Environment.NextTargetUnit(curTarget)
		}
	}

	return shaman.RegisterSpell(spellConfig)
}
