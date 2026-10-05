package priest

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type Shadowfiend struct {
	*core.Pet

	Priest          *Priest
	Shadowcrawl     *core.Spell
	ShadowcrawlAura *core.Aura
}

// pet_levelstats for 19668 at 80. Its row's armor is 0, which InitStatsForLevel skips for the 50 a level.
var baseStats = core.PetCreateStats(219, 153, 90, 350, 228, 50*core.CharacterLevel).Add(stats.Stats{
	stats.AttackPower: -20,
	// Unit::GetUnitCriticalChance's flat 5% base, same as every other guardian.
	stats.MeleeCrit: 5 * core.CritRatingPerCritChance,
	stats.SpellCrit: 5 * core.CritRatingPerCritChance,
	// Mana Leech (28305), its own passive
	stats.MeleeHit:  9 * core.MeleeHitRatingPerHitChance,
	stats.Expertise: 56 * core.ExpertisePerQuarterPercentReduction,
})

func (priest *Priest) NewShadowfiend() *Shadowfiend {
	shadowfiend := &Shadowfiend{
		// spell_pri_shadowfiend_scaling's CalculateSPAmount/CalculateAPAmount never set canBeRecalculated,
		// so the server snapshots the owner's stats once at the summon and never resyncs them.
		Pet:    core.NewPet("Shadowfiend", &priest.Character, baseStats, shadowfiendStatInheritance, false, true),
		Priest: priest,
	}
	shadowfiend.BuffImmunity = core.PetImmuneToScaling

	manaMetric := priest.NewManaMetrics(core.ActionID{SpellID: 34433})
	_ = core.MakePermanent(shadowfiend.GetOrRegisterAura(core.Aura{
		Label: "Autoattack mana regen",
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			restoreMana := priest.MaxMana() * 0.05
			priest.AddMana(sim, restoreMana, manaMetric)
		},
	}))

	actionID := core.ActionID{SpellID: 63619}
	shadowfiend.ShadowcrawlAura = shadowfiend.GetOrRegisterAura(core.Aura{
		Label:    "Shadowcrawl",
		ActionID: actionID,
		Duration: time.Second * 5,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			shadowfiend.PseudoStats.DamageDealtMultiplier *= 1.15
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			shadowfiend.PseudoStats.DamageDealtMultiplier /= 1.15
		},
	})

	shadowfiend.Shadowcrawl = shadowfiend.RegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolMagic,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagNoLogs,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    shadowfiend.NewTimer(),
				Duration: time.Second * 6,
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			shadowfiend.ShadowcrawlAura.Activate(sim)
		},
	})

	shadowfiend.PseudoStats.DamageTakenMultiplier *= 0.1

	// InitStatsForLevel's NPC_SHADOWFIEND case
	shadowfiend.EnableAutoAttacks(shadowfiend, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:        120,
			BaseDamageMax:        280,
			SwingSpeed:           1.5,
			NormalizedSwingSpeed: 1.5,
			CritMultiplier:       2,
			SpellSchool:          core.SpellSchoolShadow,
		},
		AutoSwingMelee: true,
	})

	shadowfiend.AddStatDependency(stats.Strength, stats.AttackPower, 2.0)

	priest.AddPet(shadowfiend)

	return shadowfiend
}

// shadowfiendStatInheritance is spell_pri_shadowfiend_scaling: 65% of the owner's stamina, 30% of his
// intellect, 35% of his armor, and 300% and 30% of his shadow spell power, Twisted Faith's share
// included, as attack and spell power.
func shadowfiendStatInheritance(ownerStats stats.Stats) stats.Stats {
	return stats.Stats{
		stats.Stamina:     core.PetScalingPct(ownerStats[stats.Stamina], 65),
		stats.Intellect:   core.PetScalingPct(ownerStats[stats.Intellect], 30),
		stats.Armor:       core.PetScalingPct(ownerStats[stats.Armor], 35),
		stats.AttackPower: core.PetScalingPct(ownerStats[stats.SpellPower], 300),
		stats.SpellPower:  core.PetScalingPct(ownerStats[stats.SpellPower], 30),
	}
}

func (shadowfiend *Shadowfiend) Initialize() {
}

func (shadowfiend *Shadowfiend) ExecuteCustomRotation(sim *core.Simulation) {
	shadowfiend.Shadowcrawl.Cast(sim, nil)
}

func (shadowfiend *Shadowfiend) Reset(sim *core.Simulation) {
	shadowfiend.ShadowcrawlAura.Deactivate(sim)
	shadowfiend.Disable(sim)
}

func (shadowfiend *Shadowfiend) OnPetDisable(sim *core.Simulation) {
	shadowfiend.ShadowcrawlAura.Deactivate(sim)
}

func (shadowfiend *Shadowfiend) GetPet() *core.Pet {
	return shadowfiend.Pet
}
