package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

const (
	lifebloomCostPct   = 28
	lifebloomMaxStacks = 3
)

// lifebloomState is a target's Lifebloom as its bloom reads it: when the aura runs out, and its stacks, which
// the aura has lost by the time it ends.
type lifebloomState struct {
	expires time.Duration
	stacks  int32
}

func (druid *Druid) registerLifebloomSpell() {
	druid.lifeblooms = make([]lifebloomState, len(druid.Env.AllUnits))
	// effect 1 is a dummy the bloom reads its base from
	bloomPerStack, _ := serverdata.SpellByID(48451).EffectRange(1)
	manaMetrics := druid.NewManaMetrics(core.ActionID{SpellID: 64372})
	periodic := druid.periodicHealMultiplier()

	// 33778 heals for what spell_dru_lifebloom hands it, which already holds the caster's and the target's terms
	// (Spell::EffectHeal skips them for it), and crits on the druid's chance
	druid.LifebloomBloom = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 33778},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete | SpellFlagOmenTrigger,

		DamageMultiplier: druid.directHealMultiplier(),
		CritMultiplier:   druid.DefaultHealingCritMultiplier(),
		ThreatMultiplier: druid.healThreatMultiplier(),

		Direct: core.SpellEffect{Effect: 0, SP: 0.655},
		Mods:   druid.empoweredRejuvenation(),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			stacks := druid.lifeblooms[target.UnitIndex].stacks
			// SpellHealingBonusDone with the stacks as its factor: int32(power x coefficient x stacks)
			power := float32(float32(spell.Direct.SP) * float32(spell.HealingPower(target)))
			baseHealing := float64(bloomPerStack*stacks) + float64(int32(power*float32(stacks)))
			taken := scaleFlatTaken(target, stacks)
			result := spell.CalcHealing(sim, target, baseHealing, spell.OutcomeHealingCrit)
			target.PseudoStats.BonusHealingTaken = taken
			spell.DealHealing(sim, result)
		},
	})

	bloom := func(sim *core.Simulation, target *core.Unit) {
		stacks := druid.lifeblooms[target.UnitIndex].stacks
		// the script casts the mana back before the heal
		druid.AddMana(sim, float64(lifebloomCostPct*int32(druid.BaseMana)*stacks/100/2), manaMetrics)
		druid.LifebloomBloom.Cast(sim, target)
	}

	druid.Lifebloom = druid.RegisterSpell(healForms, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48451},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL | SpellFlagOmenTrigger,

		ManaCost: core.ManaCostOptions{
			BaseCost:   lifebloomCostPct / 100.0,
			Multiplier: druid.hotCostMultiplier(),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				// Gift of the Earthmother's SPELLMOD_GLOBAL_COOLDOWN, ahead of the haste
				GCD: core.GCDDefault - 30*time.Millisecond*time.Duration(druid.Talents.GiftOfTheEarthmother),
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: druid.healThreatMultiplier(),

		Mods: druid.empoweredRejuvenation(),
		Hot: core.DotConfig{
			Aura: core.Aura{
				Label:     "Lifebloom",
				MaxStacks: lifebloomMaxStacks,
				// blooms only when it runs out (AURA_REMOVE_BY_EXPIRE), not when the fight ends first
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					if sim.CurrentTime >= druid.lifeblooms[aura.Unit.UnitIndex].expires {
						bloom(sim, aura.Unit)
					}
				},
			},
			// Nature's Splendor adds 2 s
			NumberOfTicks: 7 + core.TernaryInt32(druid.Talents.NaturesSplendor, 2, 0),
			TickLength:    time.Second,
			TicksCanCrit:  false,
			Tick:          core.SpellEffect{Effect: 0, Min: 53, Max: 53, SP: 0.09518},
			// AuraEffect::CalculateAmount works out one stack's whole amount, then multiplies it by the stacks
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = wholeHot(sim, target, dot, periodic) * float64(druid.lifeblooms[target.UnitIndex].stacks)
				dot.SnapshotHotMultiplier(1)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				taken := scaleFlatTaken(target, druid.lifeblooms[target.UnitIndex].stacks)
				result := dot.CalcSnapshotHealing(sim, target, dot.OutcomeTick)
				target.PseudoStats.BonusHealingTaken = taken
				dot.Spell.DealPeriodicHealing(sim, result)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			hot := spell.Hot(target)
			state := &druid.lifeblooms[target.UnitIndex]
			if hot.IsActive() {
				// Aura::ModStackAmount: the timers restart, then the amount is worked out again for the new stacks
				state.stacks = min(state.stacks+1, lifebloomMaxStacks)
				hot.SetStacks(sim, state.stacks)
				hot.ApplyOrReset(sim)
			} else {
				state.stacks = 1
				hot.Apply(sim)
				hot.SetStacks(sim, 1)
			}
			state.expires = hot.ExpiresAt()
			druid.landHot(sim, spell, target)
		},
	})
}

// scaleFlatTaken multiplies the target's flat healing taken by Lifebloom's stacks, as SpellHealingBonusTaken's
// factorMod does for the ticks and the bloom, and returns the old value. Put it back before dealing the heal: its
// procs can change it.
func scaleFlatTaken(target *core.Unit, stacks int32) float64 {
	taken := target.PseudoStats.BonusHealingTaken
	target.PseudoStats.BonusHealingTaken *= float64(stacks)
	return taken
}
