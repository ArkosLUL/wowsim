package priest

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (priest *Priest) registerPrayerOfHealingSpell() {
	var glyphSpell *core.Spell
	// the Prayer of Healing heal the glyph's hot comes off, crit and healing taken in
	var glyphHeal float64

	priest.PrayerOfHealing = priest.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48072},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.48,
			Multiplier: 1 -
				.1*float64(priest.Talents.HealingPrayers) -
				core.TernaryFloat64(priest.HasSetBonus(ItemSetVestmentsOfAbsolution, 2), 0.1, 0),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Second * 3,
			},
		},

		BonusCritRating: 0 +
			1*float64(priest.Talents.HolySpecialization)*core.CritRatingPerCritChance +
			core.TernaryFloat64(priest.HasSetBonus(ItemSetSanctificationRegalia, 2), 10*core.CritRatingPerCritChance, 0),
		DamageMultiplier: 1 *
			(1 + .02*float64(priest.Talents.SpiritualHealing)) *
			(1 + .01*float64(priest.Talents.BlessedResilience)) *
			(1 + .02*float64(priest.Talents.FocusedPower)) *
			(1 + .02*float64(priest.Talents.DivineProvidence)),
		CritMultiplier:   priest.DefaultHealingCritMultiplier(),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 2109, Max: 2227, SP: 0.526},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			targetAgent := target.Env.Raid.GetPlayerFromUnitIndex(target.UnitIndex)
			party := targetAgent.GetCharacter().Party

			for _, partyAgent := range party.PlayersAndPets {
				partyTarget := &partyAgent.GetCharacter().Unit
				// a pet that's away isn't in the world to be healed, and SpellInfo::CheckTarget keeps the dead out
				if !partyTarget.IsEnabled() || (partyTarget.HasHealthBar() && partyTarget.CurrentHealth() <= 0) {
					continue
				}
				// TARGET_CHECK_PARTY skips a guardian without a pet bar; a Shadowfiend has one
				if pet, ok := partyAgent.(core.PetAgent); ok && pet.GetPet().Uncontrollable {
					continue
				}
				baseHealing := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.HealingPower(partyTarget))
				result := spell.CalcHealing(sim, partyTarget, baseHealing, spell.OutcomeHealingCrit)
				heal := result.Damage
				spell.DealHealing(sim, result)
				// spell_pri_glyph_of_prayer_of_healing gives nothing off a heal of 0
				if glyphSpell != nil && heal > 0 {
					glyphHeal = heal
					glyphSpell.Hot(partyTarget).Apply(sim)
					glyphSpell.LandOn(sim, partyTarget)
				}
			}
		},
	})

	if priest.HasMajorGlyph(proto.PriestMajorGlyph_GlyphOfPrayerOfHealing) {
		glyphSpell = priest.GetOrRegisterSpell(core.SpellConfig{
			ActionID:    core.ActionID{ItemID: 42409},
			SpellSchool: core.SpellSchoolHoly,
			ProcMask:    core.ProcMaskSpellHealing,
			// the hot is 56161, without a spell_bonus_data row: 6 s of 15, over its 2 ticks
			HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(6.0 / 15 / 2),
			Flags:                   core.SpellFlagNoOnCastComplete | core.SpellFlagHelpful,

			// 56161's class mask is 0, so no talent or spell mod reaches its amount
			DamageMultiplier: 1,
			ThreatMultiplier: 1,

			Hot: core.DotConfig{
				Aura: core.Aura{
					Label: "PoH Glyph",
				},
				NumberOfTicks: 2,
				TickLength:    time.Second * 3,
				TicksCanCrit:  false,
				OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
					// int32(CalculatePct(int32(heal), 20) / GetMaxTicks()); each tick then takes the healing done
					// percents and the target's healing taken again
					dot.SnapshotBaseDamage = float64(int32(core.ApplyPct(glyphHeal, 0.2)) / 2)
					dot.SnapshotHotMultiplier(1)
				},
				OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
					dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
				},
			},
		})
	}
}
