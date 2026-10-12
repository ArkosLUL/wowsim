package deathknight

import (
	"math"
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/common/wotlk"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type Presence uint8

const (
	BloodPresence Presence = 1 << iota
	FrostPresence
	UnholyPresence
	UnsetPresence
)

const presenceEffectCategory = "Presence"

var BloodPresenceActionID = core.ActionID{SpellID: 48266}

// the heal Blood Presence and Improved Blood Presence cast off the damage dealt
var bloodPresenceHealActionID = core.ActionID{SpellID: 50475}

func (dk *Deathknight) PresenceMatches(other Presence) bool {
	return (dk.Presence & other) != 0
}

func (dk *Deathknight) ChangePresence(sim *core.Simulation, newPresence Presence) {
	if dk.PresenceMatches(newPresence) {
		return
	}

	dk.Presence = newPresence
	if dk.PresenceMatches(BloodPresence) {
		dk.BloodPresenceAura.Activate(sim)
		dk.FrostPresenceAura.Deactivate(sim)
		dk.UnholyPresenceAura.Deactivate(sim)
	} else if dk.PresenceMatches(FrostPresence) {
		dk.FrostPresenceAura.Activate(sim)
		dk.BloodPresenceAura.Deactivate(sim)
		dk.UnholyPresenceAura.Deactivate(sim)
	} else if dk.PresenceMatches(UnholyPresence) {
		dk.UnholyPresenceAura.Activate(sim)
		dk.BloodPresenceAura.Deactivate(sim)
		dk.FrostPresenceAura.Deactivate(sim)
	}
}

func (dk *Deathknight) registerBloodPresenceAura(timer *core.Timer) {
	threatMult := dk.presenceThreatMultiplier()
	damageBonusCoeff := 0.15
	// Improved Blood Presence is a flat mod on the presence's own healing taken aura, 0 without it
	healingTaken := 5 * dk.Talents.ImprovedBloodPresence
	staminaMult := 1.0 + 0.04*float64(dk.Talents.ImprovedFrostPresence)
	damageTakenMult := 1.0 - 0.01*float64(dk.Talents.ImprovedFrostPresence)

	dk.BloodPresence = dk.RegisterSpell(core.SpellConfig{
		ActionID: BloodPresenceActionID,
		Flags:    core.SpellFlagAPL,

		RuneCost: core.RuneCostOptions{
			BloodRuneCost: 1,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second,
			},
		},
		ApplyEffects: func(sim *core.Simulation, unit *core.Unit, spell *core.Spell) {
			dk.ChangePresence(sim, BloodPresence)
		},
	})

	actionID := BloodPresenceActionID
	statDep := dk.NewDynamicMultiplyStat(stats.Stamina, staminaMult)

	aura := core.Aura{
		Label:    "Blood Presence",
		ActionID: actionID,
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.ThreatMultiplier *= threatMult
			aura.Unit.PseudoStats.DamageTakenMultiplier *= damageTakenMult

			dk.ModifyDamageModifier(damageBonusCoeff)
			aura.Unit.EnableDynamicStatDep(sim, statDep)
			aura.Unit.AddHealingTakenPct(healingTaken)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.ThreatMultiplier /= threatMult
			aura.Unit.PseudoStats.DamageTakenMultiplier /= damageTakenMult

			dk.ModifyDamageModifier(-damageBonusCoeff)
			aura.Unit.DisableDynamicStatDep(sim, statDep)
			aura.Unit.RemoveHealingTakenPct(healingTaken)
		},
	}

	dk.BloodPresenceAura = dk.GetOrRegisterAura(aura)
	dk.BloodPresenceAura.NewExclusiveEffect(presenceEffectCategory, true, core.ExclusiveEffect{})
}

// registerBloodPresenceHeal is 63611, which every death knight carries in Blood Presence at 4% and Improved
// Blood Presence keeps in the other two at 2% a point: a heal (50475) for that share of the damage of each
// auto attack, cast and dot tick. Of the triggered spells only those with NOT_A_PROC proc it.
func (dk *Deathknight) registerBloodPresenceHeal() {
	healthMetrics := dk.HealThreat(dk.NewHealthMetrics(bloodPresenceHealActionID), 50475)
	improved := 2 * float64(dk.Talents.ImprovedBloodPresence)

	dk.bloodPresenceHeal = func(sim *core.Simulation, target *core.Unit, damage float64) {
		pct := improved
		if dk.PresenceMatches(BloodPresence) {
			pct = 4
		} else if !dk.PresenceMatches(FrostPresence | UnholyPresence) {
			return
		}
		if pct == 0 || damage <= 0 || !dk.givesXPOrHonor(target) {
			return
		}
		// the script hands the heal a whole amount, and the healing taken percent truncates again. 50475 has
		// no damage class and no spell_bonus_data row, so Unit::SpellHealingBonusDone hands it back before
		// any healing done percent: Strength of Wrynn's doesn't reach it
		heal := math.Trunc(damage * pct / 100)
		dk.GainHealth(sim, math.Trunc(heal*dk.HealingTakenPctMultiplier()), healthMetrics)
	}

	aura := core.MakePermanent(dk.RegisterAura(core.Aura{
		Label: "Blood Presence Heal",
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if dk.procsBloodPresenceHeal(spell) {
				dk.bloodPresenceHeal(sim, result.Target, result.Damage)
			}
		},
	}))
	// every dot tick but the ones the server deals as triggered casts (Death and Decay)
	core.ApplyProcTriggerCallback(&dk.Unit, aura, core.ProcTrigger{
		Name:        "Blood Presence Heal",
		Callback:    core.CallbackOnPeriodicDamageDealt,
		AnyPeriodic: true,
		Harmful:     true,
		Handler: func(sim *core.Simulation, _ *core.Spell, result *core.SpellResult) {
			dk.bloodPresenceHeal(sim, result.Target, result.Damage)
		},
	})
}

// gainHealingTakenOnly is a heal of a spell with no damage class and no spell_bonus_data row (Death Strike's
// 45470, Death Pact's 48743): Unit::SpellHealingBonusDone hands the amount back, so no healing done percent
// reaches it, Strength of Wrynn's included, only the healing taken percents.
func (dk *Deathknight) gainHealingTakenOnly(sim *core.Simulation, amount float64, metrics *core.ResourceMetrics) {
	dk.GainHealth(sim, math.Trunc(math.Trunc(amount)*dk.HealingTakenPctMultiplier()), metrics)
}

// givesXPOrHonor is Player::isHonorOrXPTarget, which 63611's proc asks for (PROC_ATTR_REQ_EXP_OR_HONOR): the
// victim has to be above the gray level, 9 under a level 60+ player's own, and not NO_XP.
func (dk *Deathknight) givesXPOrHonor(target *core.Unit) bool {
	return target.Level > dk.Level-9 && !target.NoXP
}

// onUseItemDamage are the items whose use spell deals the damage itself. Player::CastItemUseSpell casts an
// item's first spell untriggered, so it procs like one of the DK's own casts.
var onUseItemDamage = []core.ActionID{
	core.SaroniteBombActionID,
	core.CobaltFragBombActionID,
	core.ThermalSapperActionID,
	{SpellID: 54757}, // Hand-Mounted Pyro Rocket
	{SpellID: wotlk.GnomishLightningGeneratorSpellID},
}

// procsBloodPresenceHeal is whether a direct hit of the spell procs 63611: auto attacks, the spells the DK
// casts, an on-use item's damage and Death Coil's damage (47632, NOT_A_PROC). Not the other triggered spells:
// Threat of Thassarian's off-hand strikes, Scourge Strike's shadow part, Blood-Caked Blade, Wandering Plague,
// item procs. Necrosis (NOT_A_PROC too) and Unholy Blight's ticks skip the damage callbacks, so they heal
// where they deal damage.
func (dk *Deathknight) procsBloodPresenceHeal(spell *core.Spell) bool {
	switch spell {
	case dk.AutoAttacks.MHAuto(), dk.AutoAttacks.OHAuto(),
		dk.IcyTouch, dk.PlagueStrikeMhHit, dk.BloodStrikeMhHit, dk.HeartStrike, dk.HeartStrikeOffHit,
		dk.DeathStrikeMhHit, dk.ObliterateMhHit, dk.FrostStrikeMhHit, dk.ScourgeStrike, dk.RuneStrike,
		dk.HowlingBlast, dk.BloodBoil, dk.DeathCoilDamage:
		return true
	}
	return slices.ContainsFunc(onUseItemDamage, spell.ActionID.SameAction)
}

func (dk *Deathknight) registerFrostPresenceAura(timer *core.Timer) {

	dk.FrostPresence = dk.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 48263},
		Flags:    core.SpellFlagAPL,

		RuneCost: core.RuneCostOptions{
			FrostRuneCost: 1,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second,
			},
		},
		ApplyEffects: func(sim *core.Simulation, unit *core.Unit, spell *core.Spell) {
			dk.ChangePresence(sim, FrostPresence)
		},
	})

	threatMult := 2.0735
	dmgMitigation := 1.0 - (0.08 + 0.01*float64(dk.Talents.ImprovedFrostPresence))
	stamDep := dk.NewDynamicMultiplyStat(stats.Stamina, 1.08)
	dk.FrostPresenceAura = dk.GetOrRegisterAura(core.Aura{
		Label:    "Frost Presence",
		ActionID: core.ActionID{SpellID: 48263},
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.ThreatMultiplier *= threatMult
			aura.Unit.PseudoStats.DamageTakenMultiplier *= dmgMitigation

			aura.Unit.EnableDynamicStatDep(sim, stamDep)

			dk.ApplyDynamicEquipScaling(sim, stats.Armor, 1.6)
			dk.IcyTouch.ThreatMultiplier *= 7
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.PseudoStats.ThreatMultiplier /= threatMult
			aura.Unit.PseudoStats.DamageTakenMultiplier /= dmgMitigation

			aura.Unit.DisableDynamicStatDep(sim, stamDep)

			dk.RemoveDynamicEquipScaling(sim, stats.Armor, 1.6)
			dk.IcyTouch.ThreatMultiplier /= 7
		},
	})
	dk.FrostPresenceAura.NewExclusiveEffect(presenceEffectCategory, true, core.ExclusiveEffect{})
}

func (dk *Deathknight) registerUnholyPresenceAura(timer *core.Timer) {
	threatMult := dk.presenceThreatMultiplier()

	dk.UnholyPresence = dk.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 48265},
		Flags:    core.SpellFlagAPL,

		RuneCost: core.RuneCostOptions{
			UnholyRuneCost: 1,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second,
			},
		},
		ApplyEffects: func(sim *core.Simulation, unit *core.Unit, spell *core.Spell) {
			dk.ChangePresence(sim, UnholyPresence)
		},
	})

	// 63622's rune regen percent, from the talent's second effect
	impUp := []int64{0, 5, 10}[dk.Talents.ImprovedUnholyPresence]
	stamDep := dk.NewDynamicMultiplyStat(stats.Stamina, 1.0+0.04*float64(dk.Talents.ImprovedFrostPresence))
	var gcdAffectedSpells []*core.Spell
	dk.UnholyPresenceAura = dk.GetOrRegisterAura(core.Aura{
		Label:    "Unholy Presence",
		ActionID: core.ActionID{SpellID: 48265},
		Duration: core.NeverExpires,
		OnInit: func(aura *core.Aura, sim *core.Simulation) {
			gcdAffectedSpells = core.FilterSlice([]*core.Spell{
				dk.HowlingBlast,
				dk.ScourgeStrike,
				dk.Obliterate,
				dk.Pestilence,
				dk.HornOfWinter,
				dk.DancingRuneWeapon,
				dk.IcyTouch,
				dk.BloodBoil,
				dk.MarkOfBlood,
				dk.PlagueStrike,
				dk.HeartStrike,
				dk.DeathStrike,
				dk.BoneShield,
				dk.RaiseDead,
				dk.GhoulFrenzy,
				dk.DeathPact,
				dk.FrostStrike,
				dk.BloodStrike,
				dk.DeathAndDecay,
				dk.DeathCoil,
				dk.ArmyOfTheDead,
				dk.SummonGargoyle,
			}, func(spell *core.Spell) bool { return spell != nil })
		},
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range gcdAffectedSpells {
				spell.DefaultCast.GCD = time.Second
			}
			aura.Unit.SetRuneRegenPercent(impUp)
			aura.Unit.PseudoStats.ThreatMultiplier *= threatMult
			aura.Unit.EnableDynamicStatDep(sim, stamDep)
			dk.MultiplyMeleeSpeed(sim, 1.15)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			for _, spell := range gcdAffectedSpells {
				spell.DefaultCast.GCD = core.GCDDefault
			}
			aura.Unit.SetRuneRegenPercent(0)
			aura.Unit.PseudoStats.ThreatMultiplier /= threatMult
			aura.Unit.DisableDynamicStatDep(sim, stamDep)
			dk.MultiplyMeleeSpeed(sim, 1/1.15)
		},
	})
	dk.UnholyPresenceAura.NewExclusiveEffect(presenceEffectCategory, true, core.ExclusiveEffect{})
}

// Blood and Unholy Presence's -20% threat. Subversion adds its flat -8, -16 or -25 to that same aura
// effect, so at 3 points it's -45%, not 0.8 x 0.75
func (dk *Deathknight) presenceThreatMultiplier() float64 {
	return 0.8 - dk.subversionThreatBonus()
}

func (dk *Deathknight) registerPresences() {
	presenceTimer := dk.NewTimer()
	dk.registerBloodPresenceAura(presenceTimer)
	dk.registerUnholyPresenceAura(presenceTimer)
	dk.registerFrostPresenceAura(presenceTimer)
	dk.registerBloodPresenceHeal()
}
