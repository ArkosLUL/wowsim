package paladin

import (
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// beaconOfLight is the unit wearing the paladin's Beacon of Light and the heals that copy onto it.
type beaconOfLight struct {
	target *core.Unit
	amount float64

	holyLight    *core.Spell // 53652
	flashOfLight *core.Spell // 53653
	holyShock    *core.Spell // 53654, for Holy Shock and Lay on Hands
}

// registerBeaconOfLightSpell is 53563. Its periodic trigger keeps Light's Beacon (53651) on every raid unit within
// 60 yards of the target, the whole raid here, from the moment it goes on (SPELL_ATTR5_EXTRA_INITIAL_PERIOD).
// One target at a time (SPELL_ATTR5_LIMIT_N).
func (paladin *Paladin) registerBeaconOfLightSpell() {
	if !paladin.Talents.BeaconOfLight {
		return
	}
	actionID := core.ActionID{SpellID: 53563}

	paladin.beacon.holyLight = paladin.registerBeaconHeal(53652)
	paladin.beacon.flashOfLight = paladin.registerBeaconHeal(53653)
	paladin.beacon.holyShock = paladin.registerBeaconHeal(53654)

	paladin.BeaconOfLightAuras = paladin.NewAllyAuraArray(func(unit *core.Unit) *core.Aura {
		return unit.RegisterAura(core.Aura{
			Label:    "Beacon of Light-" + strconv.Itoa(int(paladin.UnitIndex)),
			ActionID: actionID,
			Duration: time.Minute + paladin.glyphOfBeaconDuration(),
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				paladin.beacon.target = aura.Unit
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				if paladin.beacon.target == aura.Unit {
					paladin.beacon.target = nil
				}
			},
		})
	})

	paladin.BeaconOfLight = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.35,
			Multiplier: 1 - 0.02*float64(paladin.Talents.Benediction),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if old := paladin.beacon.target; old != nil && old != target {
				paladin.BeaconOfLightAuras.Get(old).Deactivate(sim)
			}
			paladin.BeaconOfLightAuras.Get(target).Activate(sim)
			spell.LandOn(sim, target)
		},
	})
}

// registerBeaconHeal is one of the copies. CastCustomSpell hands it the amount as base points; it can't crit
// (SPELL_ATTR2_CANT_CRIT) and ignores the paladin's modifiers (SPELL_ATTR3_IGNORE_CASTER_MODIFIERS), so only the
// beacon target's healing taken reaches it.
func (paladin *Paladin) registerBeaconHeal(spellID int32) *core.Spell {
	return paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagIgnoreAttackerModifiers | core.SpellFlagNoOnCastComplete,

		DamageMultiplier: 1,
		CritMultiplier:   1,
		ThreatMultiplier: 1,
		// no spell_bonus_data row: an instant heal's default
		HealingTakenCoefficient: core.DefaultHealingTakenCoefficient(0),

		Direct: core.SpellEffect{Effect: 0, Min: 1, Max: 1},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.CalcAndDealHealing(sim, target, paladin.beacon.amount, spell.OutcomeHealing)
		},
	})
}

// copyToBeacon is spell_pal_light_s_beacon: a heal on any raid unit but the beacon target casts the copy on the
// beacon target, for the whole amount given.
func (paladin *Paladin) copyToBeacon(sim *core.Simulation, healed *core.Unit, copySpell *core.Spell, amount float64) {
	target := paladin.beacon.target
	if target == nil || target == healed || copySpell == nil || amount <= 0 {
		return
	}
	// SpellInfo::CheckTarget refuses a heal on the dead
	if target.HasHealthBar() && target.CurrentHealth() <= 0 {
		return
	}
	paladin.beacon.amount = amount
	copySpell.Cast(sim, target)
}
