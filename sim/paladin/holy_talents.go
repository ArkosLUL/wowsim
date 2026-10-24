package paladin

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// lightsGrace is 31834, Light's Grace's buff on the paladin.
type lightsGrace struct {
	aura   *core.Aura
	spell  *core.Spell
	chance float64
	// the Holy Light being cast had its cast time cut by the buff
	used bool
}

// infusionOfLight is 53672 or 54149, Infusion of Light's buff on the paladin, and 66922, the talent's Flash of
// Light hot.
type infusionOfLight struct {
	aura          *core.Aura
	spell         *core.Spell
	holyLightCrit float64
	flashCut      time.Duration
	// the Flash of Light being cast had its cast time cut by the buff
	flashUsed bool

	hot       *core.Spell
	hotPct    int32
	hotAmount float64
}

// illumination is Illumination's chance and the mana its 20272 gives.
type illumination struct {
	chance  float64
	metrics *core.ResourceMetrics
}

const sacredShieldTag = "Sacred Shield"

// applyHolyTalents is the holy talents that work through the paladin's stats and every spell he registers.
func (paladin *Paladin) applyHolyTalents() {
	// Holy Guidance, auras 174 and 175: 4% of intellect a point as spell damage and healing
	if points := paladin.Talents.HolyGuidance; points > 0 {
		paladin.AddStatDependency(stats.Intellect, stats.SpellPower, 0.04*float64(points))
	}

	// Enlightened Judgements, auras 54 and 55: 2% melee, ranged and spell hit a point
	if points := paladin.Talents.EnlightenedJudgements; points > 0 {
		paladin.AddStat(stats.MeleeHit, 2*float64(points)*core.MeleeHitRatingPerHitChance)
		paladin.AddStat(stats.SpellHit, 2*float64(points)*core.SpellHitRatingPerHitChance)
	}

	// Holy Power, aura 71 on holy: 1% crit a point. Unit::SpellDoneCritChance adds it to melee and ranged class
	// holy spells too, judgements and seals included.
	if points := paladin.Talents.HolyPower; points > 0 {
		bonus := float64(points) * core.CritRatingPerCritChance
		paladin.OnSpellRegistered(func(spell *core.Spell) {
			if spell.SpellSchool.Matches(core.SpellSchoolHoly) {
				spell.BonusCritRating += bonus
			}
		})
	}
}

// healingLightMultiplier is Healing Light's SPELLMOD_DAMAGE, 4% a point on Holy Light, Flash of Light and both of
// Holy Shock's.
func (paladin *Paladin) healingLightMultiplier() float64 {
	return 1 + 0.04*float64(paladin.Talents.HealingLight)
}

// sanctifiedLightCrit is Sanctified Light's crit chance, 2% a point on Holy Light and both of Holy Shock's.
func (paladin *Paladin) sanctifiedLightCrit() float64 {
	return 2 * float64(paladin.Talents.SanctifiedLight) * core.CritRatingPerCritChance
}

// getTalentJudgementsOfThePureBonus is Judgements of the Pure's SPELLMOD_DAMAGE and SPELLMOD_DOT, 5% a point on the
// seals' damage and the judgements', 54158 and Judgement of Command included.
func (paladin *Paladin) getTalentJudgementsOfThePureBonus() float64 {
	return 0.05 * float64(paladin.Talents.JudgementsOfThePure)
}

// registerSelfBuff is the spell a talent's proc casts on the paladin to put its aura up.
func (paladin *Paladin) registerSelfBuff(aura *core.Aura) *core.Spell {
	return paladin.RegisterSpell(core.SpellConfig{
		ActionID:    aura.ActionID,
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			aura.Activate(sim)
			spell.LandOn(sim, &paladin.Unit)
		},
	})
}

func (paladin *Paladin) registerHolyTalentProcs() {
	paladin.registerLightsGrace()
	paladin.registerInfusionOfLight()
	paladin.registerIllumination()
	paladin.registerJudgementsOfThePure()
}

// registerLightsGrace: a landed Holy Light procs 31834 at 33% a point, 0.5 s off the next Holy Light's cast for
// 15 s. The buff's one charge has no spell_proc row, so Player::RemoveSpellMods takes it when the Holy Light it
// sped up finishes, unless that Holy Light's own proc refreshed it (Aura::RefreshSpellMods).
func (paladin *Paladin) registerLightsGrace() {
	points := paladin.Talents.LightsGrace
	if points == 0 {
		return
	}
	lg := &paladin.lightsGrace
	lg.chance = core.ServerProcFor([]int32{0, 31833, 31835, 31836}[points]).Chance
	lg.aura = paladin.RegisterAura(core.Aura{
		Label:    "Light's Grace",
		ActionID: core.ActionID{SpellID: 31834},
		Duration: core.ServerDuration(31834),
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			lg.used = false
		},
	})
	lg.spell = paladin.registerSelfBuff(lg.aura)
}

// lightsGraceCastTime is the buff's cut on a Holy Light starting its cast now.
func (paladin *Paladin) lightsGraceCastTime(cast *core.Cast) {
	lg := &paladin.lightsGrace
	lg.used = lg.aura != nil && lg.aura.IsActive()
	if lg.used {
		cast.CastTime -= 500 * time.Millisecond
	}
}

func (paladin *Paladin) lightsGraceOnHolyLight(sim *core.Simulation) {
	lg := &paladin.lightsGrace
	if lg.aura == nil {
		return
	}
	if sim.RandomFloat("Light's Grace") < lg.chance {
		lg.spell.Cast(sim, &paladin.Unit)
		lg.used = false
	}
	if lg.used {
		lg.used = false
		lg.aura.Deactivate(sim)
	}
}

// registerInfusionOfLight is spell_pal_infusion_of_light. A Holy Shock crit, heal or damage, puts up 53672 or
// 54149 for 15 s: +10% crit a point on the next Holy Light, or 0.75 s a point off the next Flash of Light's cast.
// Its one charge goes on the hit of whichever used it (PROC_ATTR_REQ_SPELLMOD). A Flash of Light on a target with
// anyone's Sacred Shield (53601) puts 66922 on it instead: 50% of the heal a point over 12 one-second ticks.
func (paladin *Paladin) registerInfusionOfLight() {
	points := paladin.Talents.InfusionOfLight
	if points == 0 {
		return
	}
	infusion := &paladin.infusionOfLight
	infusion.holyLightCrit = 10 * float64(points) * core.CritRatingPerCritChance
	infusion.flashCut = 750 * time.Millisecond * time.Duration(points)
	infusion.hotPct = 50 * points

	buffSpellID := []int32{0, 53672, 54149}[points]
	infusion.aura = paladin.RegisterAura(core.Aura{
		Label:    "Infusion of Light",
		ActionID: core.ActionID{SpellID: buffSpellID},
		Duration: core.ServerDuration(buffSpellID),
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			infusion.flashUsed = false
		},
	})
	infusion.spell = paladin.registerSelfBuff(infusion.aura)

	infusion.hot = paladin.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 66922},
		SpellSchool: core.SpellSchoolHoly,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagNoOnCastComplete,

		// family 0 and no damage class: no spell mod, spell power or flat healing taken
		DamageMultiplier: 1,
		CritMultiplier:   1,
		ThreatMultiplier: 1,

		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Infusion of Light Hot",
			},
			NumberOfTicks: 12,
			TickLength:    time.Second,
			TicksCanCrit:  false,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = infusion.hotAmount
				dot.SnapshotHotMultiplier(1)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Hot(target).Apply(sim)
			spell.LandOn(sim, target)
		},
	})
}

// infusionFlashCastTime is the buff's cut on a Flash of Light starting its cast now.
func (paladin *Paladin) infusionFlashCastTime(cast *core.Cast) {
	infusion := &paladin.infusionOfLight
	infusion.flashUsed = infusion.aura != nil && infusion.aura.IsActive()
	if infusion.flashUsed {
		cast.CastTime = max(0, cast.CastTime-infusion.flashCut)
	}
}

// infusionHolyLightCrit is the crit chance the buff gives a Holy Light landing now, which then uses it up.
func (paladin *Paladin) infusionHolyLightCrit() float64 {
	if infusion := &paladin.infusionOfLight; infusion.aura != nil && infusion.aura.IsActive() {
		return infusion.holyLightCrit
	}
	return 0
}

func (paladin *Paladin) infusionOnHolyShockCrit(sim *core.Simulation) {
	if infusion := &paladin.infusionOfLight; infusion.spell != nil {
		infusion.spell.Cast(sim, &paladin.Unit)
	}
}

// infusionOnFlashOfLight takes the buff a Flash of Light used, and puts the hot on a target under Sacred Shield:
// CalculatePct(heal / 12, 50% a point) a tick.
func (paladin *Paladin) infusionOnFlashOfLight(sim *core.Simulation, target *core.Unit, heal float64) {
	infusion := &paladin.infusionOfLight
	if infusion.hot == nil {
		return
	}
	if infusion.flashUsed {
		infusion.flashUsed = false
		infusion.aura.Deactivate(sim)
	}
	if heal <= 0 || !target.HasActiveAuraWithTag(sacredShieldTag) {
		return
	}
	infusion.hotAmount = float64(uint32(float32(uint32(heal)/12) * float32(infusion.hotPct) / 100))
	// T9's 4 piece (67191) adds its 100% to the tick
	if paladin.holyItems.t9InfusionHot {
		infusion.hotAmount *= 2
	}
	infusion.hot.Cast(sim, target)
}

// registerIllumination is spell_pal_illumination: a crit of Holy Light, Flash of Light or Holy Shock's heal gives,
// at 20% a point, 30% of that heal's base cost back as 20272.
func (paladin *Paladin) registerIllumination() {
	points := paladin.Talents.Illumination
	if points == 0 {
		return
	}
	paladin.illumination.chance = core.ServerProcFor([]int32{0, 20210, 20212, 20213, 20214, 20215}[points]).Chance
	paladin.illumination.metrics = paladin.NewManaMetrics(core.ActionID{SpellID: 20272})
}

// illuminate rolls Illumination for a heal crit whose spell costs costPct of base mana. Holy Shock's heal costs
// nothing, so the script reads 48825's 18%.
func (paladin *Paladin) illuminate(sim *core.Simulation, costPct int32) {
	il := &paladin.illumination
	if il.chance == 0 || sim.RandomFloat("Illumination") >= il.chance {
		return
	}
	baseCost := int32(uint32(float32(uint32(paladin.BaseMana)) * float32(costPct) / 100))
	paladin.AddMana(sim, float64(int32(float32(baseCost)*30/100)), il.metrics)
}

// registerJudgementsOfThePure: a landed judgement casts 53655 to 54153 on the paladin, 3% cast, melee and ranged
// haste a point (aura 193) for 60 s. The talent's damage percents are in each seal's and judgement's multiplier.
func (paladin *Paladin) registerJudgementsOfThePure() {
	points := paladin.Talents.JudgementsOfThePure
	if points == 0 {
		return
	}
	hasteSpellID := []int32{0, 53655, 53656, 53657, 54152, 54153}[points]
	haste := 1 + 0.03*float64(points)

	aura := paladin.RegisterAura(core.Aura{
		Label:    "Judgements of the Pure",
		ActionID: core.ActionID{SpellID: hasteSpellID},
		Duration: core.ServerDuration(hasteSpellID),
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.MultiplyCastSpeed(haste)
			aura.Unit.MultiplyAttackSpeed(sim, haste)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.MultiplyCastSpeed(1 / haste)
			aura.Unit.MultiplyAttackSpeed(sim, 1/haste)
		},
	})
	buff := paladin.registerSelfBuff(aura)

	paladin.RegisterAura(core.Aura{
		Label:    "Judgements of the Pure Talent",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if result.Landed() && spell.Flags.Matches(SpellFlagPrimaryJudgement|SpellFlagSecondaryJudgement) {
				buff.Cast(sim, &paladin.Unit)
			}
		},
	})
}
