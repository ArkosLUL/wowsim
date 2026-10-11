package warlock

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func (warlock *Warlock) registerCurseOfElementsSpell() {
	warlock.CurseOfElementsAuras = warlock.NewEnemyAuraArray(core.CurseOfElementsAura)

	warlock.CurseOfElements = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47865},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.1,
			Multiplier: 1 - 0.02*float64(warlock.Talents.Suppression),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault - core.TernaryDuration(warlock.Talents.AmplifyCurse, 1, 0)*500*time.Millisecond,
			},
		},

		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				warlock.CurseOfElementsAuras.Get(target).Activate(sim)
			}
		},

		RelatedAuras: []core.AuraArray{warlock.CurseOfElementsAuras},
	})
}

func (warlock *Warlock) registerCurseOfWeaknessSpell() {
	warlock.CurseOfWeaknessAuras = warlock.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return core.CurseOfWeaknessAura(target, warlock.Talents.ImprovedCurseOfWeakness)
	})

	warlock.CurseOfWeakness = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 50511},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.1,
			Multiplier: 1 - 0.02*float64(warlock.Talents.Suppression),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault - core.TernaryDuration(warlock.Talents.AmplifyCurse, 1, 0)*500*time.Millisecond,
			},
		},

		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				warlock.CurseOfWeaknessAuras.Get(target).Activate(sim)
			}
		},

		RelatedAuras: []core.AuraArray{warlock.CurseOfWeaknessAuras},
	})
}

func (warlock *Warlock) registerCurseOfTonguesSpell() {
	actionID := core.ActionID{SpellID: 11719}

	// Empty aura so we can simulate cost/time to keep tongues up
	warlock.CurseOfTonguesAuras = warlock.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return target.GetOrRegisterAura(core.Aura{
			Label:    "Curse of Tongues",
			ActionID: actionID,
			Duration: time.Second * 30,
		})
	})

	warlock.CurseOfTongues = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.04,
			Multiplier: 1 - 0.02*float64(warlock.Talents.Suppression),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault - core.TernaryDuration(warlock.Talents.AmplifyCurse, 1, 0)*500*time.Millisecond,
			},
		},

		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				warlock.CurseOfTonguesAuras.Get(target).Activate(sim)
			}
		},

		RelatedAuras: []core.AuraArray{warlock.CurseOfTonguesAuras},
	})
}

func (warlock *Warlock) registerCurseOfAgonySpell() {
	numberOfTicks := core.TernaryInt32(warlock.HasMajorGlyph(proto.WarlockMajorGlyph_GlyphOfCurseOfAgony), 14, 12)

	warlock.CurseOfAgony = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47864},
		SpellSchool: core.SpellSchoolShadow,
		Flags:       core.SpellFlagHauntSE | core.SpellFlagAPL,
		ProcMask:    core.ProcMaskSpellDamage,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.1,
			Multiplier: 1 - 0.02*float64(warlock.Talents.Suppression),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault - core.TernaryDuration(warlock.Talents.AmplifyCurse, 1, 0)*500*time.Millisecond,
			},
		},

		DamageMultiplier: spellModDamage(
			0.03*float64(warlock.Talents.ShadowMastery),
			0.01*float64(warlock.Talents.Contagion),
			0.05*float64(warlock.Talents.ImprovedCurseOfAgony),
		),
		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "CurseofAgony",
			},
			NumberOfTicks: numberOfTicks,
			TickLength:    time.Second * 2,
			TicksCanCrit:  false,

			Tick: core.SpellEffect{Effect: 0, Min: 145, Max: 145, SP: 0.1},

			// spell_warl_curse_of_agony ramps the whole tick, spell power included: half for ticks 1 to 4,
			// then 1, 1.5 and 2 times from ticks 5, 9 and 13
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = 0.5 * (dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.SP*dot.Spell.SpellPower()))
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTickCounted)
				if dot.TickCount > 0 && dot.TickCount%4 == 0 {
					step := float64(dot.TickCount / 4)
					dot.SnapshotBaseDamage *= (step + 1) / step
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				spell.SpellMetrics[target.UnitIndex].Hits--
				warlock.CurseOfDoom.Dot(target).Cancel(sim)
				spell.Dot(target).Apply(sim)
			}
		},
	})
}

func (warlock *Warlock) registerCurseOfDoomSpell() {
	scaling := warlock.curseOfDoomScaling()
	// the cast's length, which its cooldown, the curse and its one tick all take
	length := time.Minute

	castConfig := core.CastConfig{
		DefaultCast: core.Cast{
			GCD: core.GCDDefault - core.TernaryDuration(warlock.Talents.AmplifyCurse, 1, 0)*500*time.Millisecond,
		},
		CD: core.Cooldown{
			Timer:    warlock.NewTimer(),
			Duration: time.Minute,
		},
	}
	onTick := func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
		dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
	}
	var extraCastCondition core.CanCastCondition
	if scaling {
		castConfig.ModifyCast = func(_ *core.Simulation, spell *core.Spell, _ *core.Cast) {
			length = warlock.curseOfDoomLength()
			spell.CD.Duration = length
		}
		onTick = func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
			dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeSnapshotCrit)
		}
		// spell_tweaks_curse_of_doom refuses the cast while the warlock's own curse is on the target: the
		// cooldown can be up an update before the hit, and a recast would wipe it
		extraCastCondition = func(_ *core.Simulation, target *core.Unit) bool {
			return !warlock.CurseOfDoom.Dot(target).IsActive()
		}
	}

	warlock.CurseOfDoom = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 47867},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.15,
			Multiplier: 1 - 0.02*float64(warlock.Talents.Suppression),
		},
		Cast:               castConfig,
		ExtraCastCondition: extraCastCondition,

		DamageMultiplierAdditive: 1 +
			0.03*float64(warlock.Talents.ShadowMastery),
		// no crit talent lists Curse of Doom
		CritMultiplier:   warlock.DefaultSpellCritMultiplier(),
		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "CurseofDoom",
			},
			NumberOfTicks: 1,
			TickLength:    time.Minute,
			// the tweak's carrier aura 425793 has the periodic crit effect
			TicksCanCrit: scaling,

			Tick: core.SpellEffect{Effect: 0, Min: 7300, Max: 7300, SP: 2},

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.SnapshotBaseDamage = dot.Tick.Roll(sim) + core.PowerBonus(dot.Tick.SP*dot.Spell.SpellPower())
				dot.SnapshotCritChance = dot.Spell.SpellCritChance(target)
				dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(dot.Spell.Unit.AttackTables[target.UnitIndex])
			},
			OnTick: onTick,
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHit)
			if result.Landed() {
				warlock.CurseOfAgony.Dot(target).Cancel(sim)
				dot := spell.Dot(target)
				if scaling {
					// the tick counts as the hit, as it can crit
					spell.SpellMetrics[target.UnitIndex].Hits--
					dot.TickLength = length
				}
				dot.Apply(sim)
			}
		},
	})
}

// curseOfDoomScaling is whether the warlock carries mod-spell-tweaks' Curse of Doom aura (425793),
// which Demonic Pact gives.
func (warlock *Warlock) curseOfDoomScaling() bool {
	return warlock.Talents.DemonicPact > 0 && warlock.Server().SpellTweaks.CurseOfDoomScaling
}

// curseOfDoomLength is spell_tweaks_doom_scaling's 60 s times the cast speed, in whole ms. A slow
// changes nothing, and it never goes under 1 s.
func (warlock *Warlock) curseOfDoomLength() time.Duration {
	hasted := min(warlock.ApplyCastSpeed(time.Minute), time.Minute).Truncate(time.Millisecond)
	return max(hasted, time.Second)
}
