package deathknight

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (dk *Deathknight) registerSummonGargoyleCD() {
	if !dk.Talents.SummonGargoyle {
		return
	}

	dk.SummonGargoyleAura = dk.RegisterAura(core.Aura{
		Label:    "Summon Gargoyle",
		ActionID: core.ActionID{SpellID: 49206},
		Duration: time.Second * 30,
	})

	dk.SummonGargoyle = dk.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 49206},
		Flags:    core.SpellFlagAPL,

		RuneCost: core.RuneCostOptions{
			RunicPowerCost: 60,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    dk.NewTimer(),
				Duration: time.Minute * 3,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			// npc_pet_dk_ebon_gargoyle flies off, interrupting its cast, once 32 of its 36 s are up
			dk.Gargoyle.EnableWithTimeout(sim, dk.Gargoyle, gargoyleCastWindow)
			dk.Gargoyle.CancelGCDTimer(sim)

			// Add a dummy aura to show in metrics
			dk.SummonGargoyleAura.Activate(sim)

			dk.Gargoyle.startAI(sim)
		},
	})

	dk.AddMajorCooldown(core.MajorCooldown{
		Spell: dk.SummonGargoyle,
		Type:  core.CooldownTypeDPS,
	})
	if dk.Inputs.IsDps {
		// We use this for defining the min cast time of gargoyle,
		// but we don't cast it with the MCD system in the dps sim
		dk.GetMajorCooldown(dk.SummonGargoyle.ActionID).Disable()
	}
}

// npc_pet_dk_ebon_gargoyle's timers, from its first UpdateAI
const (
	gargoyleDecisionInterval = 400 * time.Millisecond
	gargoyleFirstCast        = 2000 * time.Millisecond
	gargoyleCastWindow       = 32 * time.Second
)

type GargoylePet struct {
	core.Pet

	dkOwner *Deathknight

	GargoyleStrike *core.Spell

	summonedAt     time.Duration
	firstCast      bool
	decisionAction *core.PendingAction
}

func (dk *Deathknight) NewGargoyle() *GargoylePet {
	// Impurity raises the 75% spell_dk_pet_scaling hands over, in whole percent
	spellPowerPct := int64(75 + 75*4*dk.Talents.Impurity/100)

	gargoyle := &GargoylePet{
		Pet: core.NewPet("Gargoyle", &dk.Character, stats.Stats{
			stats.Stamina: 1000,
			// m_baseSpellCritChance, the only spell crit a creature has
			stats.SpellCrit: 5 * core.CritRatingPerCritChance,
		}, func(ownerStats stats.Stats) stats.Stats {
			// CalculateSPAmount: the owner's attack power as spell damage
			return stats.Stats{
				stats.SpellPower: calculatePct(ownerStats[stats.AttackPower], spellPowerPct),
			}
		}, false, true),
		dkOwner: dk,
	}
	gargoyle.HitScaling = core.PetHitScalingMasterSpell06

	// NightOfTheDead
	gargoyle.PseudoStats.DamageTakenMultiplier *= 1.0 - float64(dk.Talents.NightOfTheDead)*0.45

	// A guardian's scaling auras never tick, so the owner's haste is what it was at the summon. Its
	// immunities keep Bloodlust off it.
	gargoyle.OnPetEnable = func(sim *core.Simulation) {
		gargoyle.PseudoStats.CastSpeedMultiplier = 1
		gargoyle.MultiplyCastSpeed(dkPetHaste(dk.SwingSpeed()))
	}

	dk.AddPet(gargoyle)

	return gargoyle
}

func (garg *GargoylePet) GetPet() *core.Pet {
	return &garg.Pet
}

func (garg *GargoylePet) Initialize() {
	garg.registerGargoyleStrikeSpell()
}

func (garg *GargoylePet) Reset(_ *core.Simulation) {
	garg.decisionAction = nil
}

func (garg *GargoylePet) ExecuteCustomRotation(_ *core.Simulation) {
}

// startAI runs npc_pet_dk_ebon_gargoyle's UpdateAI: every 400 ms, from 2 s on and once it has landed,
// an 80% chance to start Gargoyle Strike if it isn't already casting one.
func (garg *GargoylePet) startAI(sim *core.Simulation) {
	garg.summonedAt = sim.CurrentTime
	garg.firstCast = true
	if garg.decisionAction != nil {
		garg.decisionAction.Cancel(sim)
	}

	landedAt := sim.CurrentTime + max(gargoyleFirstCast, garg.dkOwner.GargoyleSummonDelay)
	nominal := sim.CurrentTime
	var decide func(sim *core.Simulation)
	decide = func(sim *core.Simulation) {
		if !garg.IsEnabled() {
			return
		}
		if sim.CurrentTime >= landedAt && garg.Hardcast.Expires <= sim.CurrentTime && sim.RandomFloat("Gargoyle Decision") < 0.8 {
			if garg.firstCast {
				garg.firstCast = false
				garg.dkOwner.OnGargoyleStartFirstCast()
			}
			garg.GargoyleStrike.Cast(sim, garg.dkOwner.CurrentTarget)
		}

		nominal += gargoyleDecisionInterval
		if nominal >= garg.summonedAt+gargoyleCastWindow {
			return
		}
		garg.decisionAction = &core.PendingAction{
			NextActionAt: sim.NextServerTick(nominal),
			// the cast landing this update goes first, as Unit::Update runs before UpdateAI
			Priority: core.ActionPriorityLow,
			OnAction: decide,
		}
		sim.AddPendingAction(garg.decisionAction)
	}

	nominal += gargoyleDecisionInterval
	garg.decisionAction = &core.PendingAction{
		NextActionAt: sim.NextServerTick(nominal),
		Priority:     core.ActionPriorityLow,
		OnAction:     decide,
	}
	sim.AddPendingAction(garg.decisionAction)
}

func (garg *GargoylePet) registerGargoyleStrikeSpell() {
	garg.GargoyleStrike = garg.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 51963},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellDamage,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				CastTime: time.Millisecond * 2000,
			},
		},

		DamageMultiplier: 1,
		CritMultiplier:   1.5,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// spell_pet_dk_gargoyle_strike adds 3 a level past 60 to the 51-69 roll
			baseDamage := sim.Roll(51, 69) + 3*float64(core.CharacterLevel-60) + 0.453*spell.SpellPower()
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
		},
	})
}
