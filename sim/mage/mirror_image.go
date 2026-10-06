package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The numbers in this file are VERY rough approximations based on logs.

func (mage *Mage) registerMirrorImageCD() {
	var t10Aura *core.Aura
	if mage.HasSetBonus(ItemSetBloodmagesRegalia, 4) {
		t10Aura = mage.RegisterAura(core.Aura{
			Label:    "Mirror Image Bonus Damage T10 4PC",
			ActionID: core.ActionID{SpellID: 70748},
			Duration: time.Second * 30,
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				mage.PseudoStats.DamageDealtMultiplier *= 1.18
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				mage.PseudoStats.DamageDealtMultiplier /= 1.18
			},
		})
	}

	mage.MirrorImage = mage.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 55342},
		Flags:    core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.1,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    mage.NewTimer(),
				Duration: time.Minute * 3,
			},
		},

		SpellSchool:      core.SpellSchoolArcane,
		ProcMask:         core.ProcMaskEmpty,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			mage.mirrorImage.EnableWithTimeout(sim, mage.mirrorImage, time.Second*30)
			if t10Aura != nil {
				t10Aura.Activate(sim)
			}
		},
	})

	mage.AddMajorCooldown(core.MajorCooldown{
		Spell:    mage.MirrorImage,
		Priority: core.CooldownPriorityDrums + 1000, // Always prefer to cast before drums or lust so the ele gets their benefits.
		Type:     core.CooldownTypeDPS,
	})
}

type MirrorImage struct {
	*core.Pet

	mageOwner *Mage

	Frostbolt *core.Spell
	Fireblast *core.Spell
}

func (mage *Mage) NewMirrorImage() *MirrorImage {
	mirrorImage := &MirrorImage{
		Pet:       core.NewPet("Mirror Image", &mage.Character, mirrorImageBaseStats, createMirrorImageInheritance(mage), false, true),
		mageOwner: mage,
	}
	mirrorImage.BuffImmunity = core.PetImmuneToScaling
	mirrorImage.EnableManaBar()

	mage.AddPet(mirrorImage)

	return mirrorImage
}

func (mi *MirrorImage) GetPet() *core.Pet {
	return mi.Pet
}

func (mi *MirrorImage) Initialize() {
	mi.registerFireblastSpell()
	mi.registerFrostboltSpell()
}

func (mi *MirrorImage) Reset(_ *core.Simulation) {
}

func (mi *MirrorImage) ExecuteCustomRotation(sim *core.Simulation) {
	spell := mi.Frostbolt
	if mi.Fireblast.CD.IsReady(sim) && sim.RandomFloat("MirrorImage FB") < 0.5 {
		spell = mi.Fireblast
	}

	if success := spell.Cast(sim, mi.CurrentTarget); !success {
		mi.Disable(sim)
	}
}

var mirrorImageBaseStats = stats.Stats{
	stats.Mana: 3000, // Unknown
}

var createMirrorImageInheritance = func(mage *Mage) func(stats.Stats) stats.Stats {
	return func(ownerStats stats.Stats) stats.Stats {
		return stats.Stats{
			stats.SpellHit: ownerStats[stats.SpellHit] - float64(mage.Talents.Precision),
			// seems to be about 8% baseline
			stats.SpellCrit:  8 * core.CritRatingPerCritChance,
			stats.SpellPower: ownerStats[stats.SpellPower] * 0.33,
		}
	}
}

func (mi *MirrorImage) registerFrostboltSpell() {
	mage := mi.mageOwner
	numImages := core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfMirrorImage), 4, 3)
	talents := mage.Talents

	mi.Frostbolt = mi.RegisterSpell(core.SpellConfig{
		ActionID:     core.ActionID{SpellID: 59638},
		SpellSchool:  core.SpellSchoolFrost,
		ProcMask:     core.ProcMaskSpellDamage,
		MissileSpeed: 24,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.01,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
				// shares Frostbolt's own class mask, so its cast-time mods cut this cast too
				CastTime: time.Second*3 - time.Millisecond*100*time.Duration(talents.ImprovedFrostbolt+talents.EmpoweredFrostbolt),
			},
		},

		BonusCritRating: 0 +
			float64(talents.WintersChill)*core.CritRatingPerCritChance +
			core.TernaryFloat64(mage.HasSetBonus(ItemSetKhadgarsRegalia, 4), 5*core.CritRatingPerCritChance, 0),
		DamageMultiplier: 1,
		// images cast with the mage's spell mods and share Frostbolt's class mask (Unit::GetSpellModOwner),
		// so the same SPELLMOD_DAMAGE percents that Frostbolt itself takes reach this cast too
		DamageMultiplierAdditive: spellModDamage(
			.01*float64(talents.ChilledToTheBone),
			core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfFrostbolt), .05, 0),
			core.TernaryFloat64(mage.HasSetBonus(ItemSetTempestRegalia, 4), .05, 0),
		),
		// the image's own multiplier: the meta gem's crit damage aura sits on the mage, not the image
		CritMultiplier:   mi.SpellCritMultiplier(1, mage.bonusCritDamage+float64(talents.IceShards)/3),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 163, Max: 169, SP: 0.3},
		// images cast with the mage's spell mods and share Frostbolt's class mask, so the effect 0 mods meant
		// for Frostbolt's slow (Permafrost, Chilled to the Bone) cut this damage
		Mods: []core.SpellMod{
			{Op: core.SpellModEffect1, Flat: []int32{0, -4, -7, -10}[talents.Permafrost]},
			{Op: core.SpellModEffect1, Flat: -2 * talents.ChilledToTheBone},
			{Op: core.SpellModBonusMultiplier, Flat: 5 * talents.EmpoweredFrostbolt},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			//3x damage for 3 mirror images
			baseDamage := (spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()) * numImages
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			spell.DealDamageAfterTravel(sim, result)
		},
	})

	// Arcane Power's classMask also reaches 59638 (Unit::GetSpellModOwner); it isn't a charged mod, so
	// Player::ApplySpellMod's temporaryPet check doesn't drop it.
	if mage.ArcanePowerAura != nil {
		frostbolt := mi.Frostbolt
		mage.ArcanePowerAura.ApplyOnGain(func(aura *core.Aura, sim *core.Simulation) {
			frostbolt.DamageMultiplierAdditive *= 1.2
		})
		mage.ArcanePowerAura.ApplyOnExpire(func(aura *core.Aura, sim *core.Simulation) {
			frostbolt.DamageMultiplierAdditive /= 1.2
		})
	}
}

func (mi *MirrorImage) registerFireblastSpell() {
	mage := mi.mageOwner
	numImages := core.TernaryFloat64(mage.HasMajorGlyph(proto.MageMajorGlyph_GlyphOfMirrorImage), 4, 3)
	talents := mage.Talents

	mi.Fireblast = mi.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 59637},
		SpellSchool: core.SpellSchoolFire,
		ProcMask:    core.ProcMaskSpellDamage,

		ManaCost: core.ManaCostOptions{
			BaseCost: 0.01,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDMin,
			},
			CD: core.Cooldown{
				Timer: mi.NewTimer(),
				// Improved Fire Blast does reach 59637's class mask, but npc_pet_mage_mirror_image only
				// retries Fire Blast every 6.5 s, so a shorter cooldown never shows
				Duration: time.Second * 6,
			},
		},

		// Critical Mass is a school crit aura on the mage (SPELL_AURA_MOD_SPELL_CRIT_CHANCE_SCHOOL), not a
		// spell mod, so it doesn't reach the image
		BonusCritRating:  float64(talents.Incineration) * 2 * core.CritRatingPerCritChance,
		DamageMultiplier: 1,
		// 59637's class mask (Unit::GetSpellModOwner) overlaps both the real Fire Blast's (Fire Power,
		// Spell Impact) and Ice Lance's (Chilled to the Bone, Ice Shards)
		DamageMultiplierAdditive: spellModDamage(
			.02*float64(talents.SpellImpact),
			.02*float64(talents.FirePower),
			.01*float64(talents.ChilledToTheBone),
		),
		CritMultiplier:   mi.fireblastCritMultiplier(0),
		ThreatMultiplier: 1,

		Direct: core.SpellEffect{Effect: 0, Min: 88, Max: 98, SP: 0.15},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			//3x damage for 3 mirror images
			baseDamage := (spell.Direct.Roll(sim) + spell.Direct.SP*spell.SpellPower()) * numImages
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
		},
	})

	if mage.ArcanePowerAura != nil {
		fireblast := mi.Fireblast
		mage.ArcanePowerAura.ApplyOnGain(func(aura *core.Aura, sim *core.Simulation) {
			fireblast.DamageMultiplierAdditive *= 1.2
		})
		mage.ArcanePowerAura.ApplyOnExpire(func(aura *core.Aura, sim *core.Simulation) {
			fireblast.DamageMultiplierAdditive /= 1.2
		})
	}
}

// fireblastCritMultiplier is 59637's crit multiplier with extra crit damage on top (Combustion's).
// The image's own, since the meta gem's crit damage aura sits on the mage.
func (mi *MirrorImage) fireblastCritMultiplier(extra float64) float64 {
	mage := mi.mageOwner
	return mi.SpellCritMultiplier(1, mage.bonusCritDamage+float64(mage.Talents.IceShards)/3+extra)
}
