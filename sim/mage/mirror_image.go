package mage

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

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
			mage.mirrorImage.summon(sim)
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

// npc_pet_mage_mirror_image's timers: it waits before it attacks, dies half a second short of the
// summon's 30 s, and puts each spell's event back this far when it runs.
const (
	mirrorImageAttackDelay = 1200 * time.Millisecond
	mirrorImageLife        = 29500 * time.Millisecond
	mirrorImageFrostbolt   = 2500 * time.Millisecond
	mirrorImageFireBlast   = 6500 * time.Millisecond
	// CasterAI::JustEngagedWith's realCooldown for Fire Blast, its 6 s recovery time. Frostbolt's is 0.
	mirrorImageFireBlastFirst = 6 * time.Second
)

// The summon's own duration (58831), which takes the corpse away.
const mirrorImageSummon = 30 * time.Second

type MirrorImage struct {
	*core.Pet

	mageOwner *Mage

	Frostbolt *core.Spell
	Fireblast *core.Spell

	// when each of the AI's two events is due
	frostboltDue, fireblastDue time.Duration
	// events level on time run in the order they were scheduled
	frostboltScheduled, fireblastScheduled int
	// the mage was fighting as they came: his target took them onto its threat list then, which engaged
	// their AI before it had anything to attack
	engagedAtSummon bool
	summonedAt      time.Duration
}

func (mage *Mage) NewMirrorImage() *MirrorImage {
	mirrorImage := &MirrorImage{
		// an image carries the Water Elemental's scaling auras (spell_mage_pet_scaling)
		Pet:       core.NewPet("Mirror Image", &mage.Character, mirrorImageBaseStats, waterElementalStatInheritance, false, true),
		mageOwner: mage,
	}
	mirrorImage.BuffImmunity = core.PetImmuneToScaling
	// SummonProperties 1021 has neither a pet's type nor its category (Guardian::Guardian)
	mirrorImage.Uncontrollable = true
	// an image casts from beside its mage: it's summoned on him, and CasterAI only moves it between casts
	mirrorImage.DistanceFromTarget = mage.DistanceFromTarget
	// InitStatsForLevel's NPC_MIRROR_IMAGE case sets the create mana to 28 + 30 a level, but the
	// pool keeps the base it got before that: creature_classlevelstats' 8814 for a level 80 mage
	mirrorImage.EnableCreatureManaBar(2428, mirrorImageBaseStats[stats.Intellect], 15, false)
	mirrorImage.AddStat(stats.Mana, 8814-2428)
	mirrorImage.AddStatDependency(stats.Strength, stats.AttackPower, 2)

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

// The AI's events cast for it, so the GCD loop has nothing to do.
func (mi *MirrorImage) ExecuteCustomRotation(sim *core.Simulation) {
	mi.WaitUntil(sim, core.NeverExpires)
}

// summon brings the images out. Their AI counts from their first update: it holds them 1.2 s before
// they attack, which they only do once the mage is fighting, and kills them at 29.5 s.
func (mi *MirrorImage) summon(sim *core.Simulation) {
	start := mi.AIClockStart(sim)
	mi.engagedAtSummon = sim.CurrentTime > 0
	mi.Enable(sim, mi)
	mi.SetUnsummonAt(sim.NextServerTick(start + mirrorImageSummon))

	summonedAt := sim.CurrentTime
	mi.summonedAt = summonedAt
	sim.AddPendingAction(&core.PendingAction{
		NextActionAt: sim.NextServerTick(start + mirrorImageLife),
		// DeathEvent runs in Unit::Update's event pass: behind a cast landing on that update, which was
		// queued first, and ahead of UpdateAI
		Priority: core.ActionPriorityLow,
		OnAction: func(sim *core.Simulation) {
			if mi.summonedAt == summonedAt && mi.IsEnabled() {
				mi.Disable(sim)
			}
		},
	})

	sim.AddPendingAction(&core.PendingAction{
		NextActionAt: sim.NextServerTick(max(start+mirrorImageAttackDelay, 0)),
		Priority:     core.ActionPriorityLow,
		OnAction:     mi.engage,
	})
}

// engage is CasterAI::JustEngagedWith: it casts one of the two spells, picked evenly, and schedules both,
// the one it cast for when the cast is over. An image's AI only runs one event an update after that.
// Images that come into a fight were engaged at the summon, with no victim to open on: both events were
// scheduled then, on a clock that only starts now, and this update just picks the target.
func (mi *MirrorImage) engage(sim *core.Simulation) {
	if !mi.IsEnabled() {
		return
	}
	mi.frostboltScheduled, mi.fireblastScheduled = 0, 1
	if mi.engagedAtSummon {
		mi.frostboltDue = mi.AIClockStart(sim)
		mi.fireblastDue = mi.frostboltDue + mirrorImageFireBlastFirst
		mi.queueEvent(sim, mi.UpdateAfter(sim, sim.CurrentTime))
		return
	}
	mi.frostboltDue = sim.CurrentTime
	mi.fireblastDue = sim.CurrentTime + mirrorImageFireBlastFirst
	if sim.RandomFloat("Mirror Image Opener") < 0.5 {
		if mi.Frostbolt.Cast(sim, mi.CurrentTarget) {
			mi.frostboltDue += mi.Frostbolt.CastTime()
		}
	} else {
		mi.Fireblast.Cast(sim, mi.CurrentTarget)
	}
	mi.queueEvent(sim, mi.UpdateAfter(sim, sim.CurrentTime))
}

// queueEvent wakes the AI on the first update from earliest on that it can cast on and has an event due by.
func (mi *MirrorImage) queueEvent(sim *core.Simulation, earliest time.Duration) {
	at := max(earliest, mi.Hardcast.Expires, min(mi.frostboltDue, mi.fireblastDue))
	sim.AddPendingAction(&core.PendingAction{
		NextActionAt: sim.NextServerTick(at),
		// a cast landing on this update goes first, as Unit::Update runs before UpdateAI
		Priority: core.ActionPriorityLow,
		OnAction: mi.runEvent,
	})
}

// runEvent is the AI's UpdateAI once it's fighting: the earliest event due, put back its 2.5 or 6.5 s,
// and that spell cast. It does nothing while a cast is going.
func (mi *MirrorImage) runEvent(sim *core.Simulation) {
	if !mi.IsEnabled() {
		return
	}
	if mi.Hardcast.Expires > sim.CurrentTime {
		mi.queueEvent(sim, sim.CurrentTime)
		return
	}

	frostbolt, fireblast := mi.frostboltDue <= sim.CurrentTime, mi.fireblastDue <= sim.CurrentTime
	if frostbolt && fireblast {
		if mi.frostboltDue != mi.fireblastDue {
			frostbolt = mi.frostboltDue < mi.fireblastDue
		} else {
			frostbolt = mi.frostboltScheduled < mi.fireblastScheduled
		}
		fireblast = !frostbolt
	}
	scheduled := max(mi.frostboltScheduled, mi.fireblastScheduled) + 1
	switch {
	case frostbolt:
		mi.frostboltDue, mi.frostboltScheduled = sim.CurrentTime+mirrorImageFrostbolt, scheduled
		mi.Frostbolt.Cast(sim, mi.CurrentTarget)
	case fireblast:
		mi.fireblastDue, mi.fireblastScheduled = sim.CurrentTime+mirrorImageFireBlast, scheduled
		mi.Fireblast.Cast(sim, mi.CurrentTarget)
	}
	mi.queueEvent(sim, mi.UpdateAfter(sim, sim.CurrentTime))
}

// No pet_levelstats row for 31216, so InitStatsForLevel's fallback stats, 2 * Str - 20 attack power
// and a creature's flat 5% crit.
var mirrorImageBaseStats = core.GuardianCreateStats.Add(stats.Stats{
	// Guardian::UpdateMaxHealth: creature_classlevelstats' 10080 at the template's 0.4, plus 10 a
	// point of stamina over 25. The universal stamina dependency (10 a point, -180) adds the rest
	stats.Health:      4032 - 10*25 + 180,
	stats.AttackPower: -20,
	stats.SpellCrit:   5 * core.CritRatingPerCritChance,
})

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
			FlatCost: 90,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
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
			// the corpse goes when the summon's 30 s are up, and a bolt still in the air goes with it
			if mi.MissileLost(sim, spell) {
				return
			}
			//3x damage for 3 mirror images
			baseDamage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			// each image's hit is whole on its own
			result.Damage, result.Threat = result.Damage*numImages, result.Threat*numImages
			if mage.chills == nil || mage.chills.Get(target) == nil {
				spell.DealDamageAfterTravel(sim, result)
				return
			}
			// 59638 slows for 4 s, which counts for the mage's Torment the Weak
			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
				if result.Landed() {
					mage.chill(sim, target, time.Second*4)
				}
			})
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
			FlatCost: 120,
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
			baseDamage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
			// each image's hit is whole on its own
			result.Damage, result.Threat = result.Damage*numImages, result.Threat*numImages
			spell.DealDamage(sim, result)
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
