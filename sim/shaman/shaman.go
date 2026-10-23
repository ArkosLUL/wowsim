package shaman

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

var TalentTreeSizes = [3]int{25, 29, 26}

// Start looking to refresh 5 minute totems at 4:55.
const TotemRefreshTime5M = time.Second * 295

const (
	SpellFlagShock     = core.SpellFlagAgentReserved1
	SpellFlagElectric  = core.SpellFlagAgentReserved2
	SpellFlagTotem     = core.SpellFlagAgentReserved3
	SpellFlagFocusable = core.SpellFlagAgentReserved4
)

func NewShaman(character *core.Character, talents string, totems *proto.ShamanTotems, selfBuffs SelfBuffs, thunderstormRange bool) *Shaman {
	shaman := &Shaman{
		Character:              *character,
		Talents:                &proto.ShamanTalents{},
		Totems:                 totems,
		SelfBuffs:              selfBuffs,
		thunderstormInRange:    thunderstormRange,
		overloadDoneMultiplier: 1,
	}
	shaman.waterShieldManaMetrics = shaman.NewManaMetrics(core.ActionID{SpellID: 57960}).EnergizeThreat(core.EnergizeBySpell)

	core.FillTalentsProto(shaman.Talents.ProtoReflect(), talents, TalentTreeSizes)
	shaman.EnableManaBar()

	if shaman.Totems.Fire == proto.FireTotem_TotemOfWrath && !shaman.Talents.TotemOfWrath {
		shaman.Totems.Fire = proto.FireTotem_FlametongueTotem
	}

	if selfBuffs.Shield == proto.ShamanShield_WaterShield {
		// Water Shield's second effect, 100 mana per 5 s. Glyph of Water Mastery and T7's 2 piece each add a
		// percent of it
		shaman.AddStat(stats.MP5, 100+core.TernaryFloat64(shaman.HasMajorGlyph(proto.ShamanMajorGlyph_GlyphOfWaterMastery), 30, 0))
	}

	// When using the tier bonus for snapshotting we do not use the bonus spell
	if totems.EnhTierTenBonus {
		totems.BonusSpellpower = 0
	}

	shaman.FireElemental = shaman.NewFireElemental(float64(totems.BonusSpellpower))
	return shaman
}

// Which buffs this shaman is using.
type SelfBuffs struct {
	Shield  proto.ShamanShield
	ImbueMH proto.ShamanImbue
	ImbueOH proto.ShamanImbue
	// Earth Shield's hits taken per minute, in place of damage
	EarthShieldPPM int32
}

// Indexes into NextTotemDrops for self buffs
const (
	AirTotem int = iota
	EarthTotem
	FireTotem
	WaterTotem
)

// Shaman represents a shaman character.
type Shaman struct {
	core.Character

	thunderstormInRange bool // flag if thunderstorm will be in range.

	Talents   *proto.ShamanTalents
	SelfBuffs SelfBuffs

	Totems *proto.ShamanTotems

	// The expiration time of each totem (earth, air, fire, water).
	TotemExpirations [4]time.Duration

	LightningBolt   *core.Spell
	LightningBoltLO *core.Spell

	ChainLightning     *core.Spell
	ChainLightningHits []*core.Spell
	ChainLightningLO   *core.Spell

	overloadAmount int32 // base points handed to the next overload cast
	// the shaman's own nature MOD_DAMAGE_PERCENT_DONE auras, the only done percents an overload takes
	overloadDoneMultiplier float64

	LavaBurst   *core.Spell
	FireNova    *core.Spell
	LavaLash    *core.Spell
	Stormstrike *core.Spell

	LightningShield     *core.Spell
	LightningShieldAura *core.Aura

	Thunderstorm *core.Spell

	EarthShock *core.Spell
	FlameShock *core.Spell
	FrostShock *core.Spell

	FeralSpirit  *core.Spell
	SpiritWolves *SpiritWolves

	FireElemental      *FireElemental
	FireElementalTotem *core.Spell

	MagmaTotem           *core.Spell
	ManaSpringTotem      *core.Spell
	HealingStreamTotem   *core.Spell
	SearingTotem         *core.Spell
	StrengthOfEarthTotem *core.Spell
	TotemOfWrath         *core.Spell
	TremorTotem          *core.Spell
	StoneskinTotem       *core.Spell
	WindfuryTotem        *core.Spell
	WrathOfAirTotem      *core.Spell
	FlametongueTotem     *core.Spell

	MaelstromWeaponAura *core.Aura

	glyphOfTotemOfWrathAura *core.Aura

	// Healing Spells
	tidalWaveProc          *core.Aura
	ancestralHealingAmount float64
	raidPets               map[*core.Unit]*core.Pet
	AncestralAwakening     *core.Spell
	LesserHealingWave      *core.Spell
	HealingWave            *core.Spell
	ChainHeal              *core.Spell
	Riptide                *core.Spell
	EarthShield            *core.Spell
	TidalForce             *core.Spell

	// Earth Shield's stand-in hits, per target
	earthShieldTargets map[*core.Unit]*earthShieldHits
	// the raid's target dummies, which a chain heal's jumps fall back on
	dummyUnits map[*core.Unit]bool

	// Glyph of Healing Wave's heal and the base points the next one carries
	glyphOfHealingWaveHeal *core.Spell
	glyphHealAmount        float64
	// T10's 4 piece hot, the amount per tick the next one carries, and the 400 ms step it lands on
	chainedHeal       *core.Spell
	chainedHealAmount float64
	chainedHealDelay  *core.DelayedPeriodicApplier
	// Water Shield's mana per orb, which Improved Water Shield's procs give
	waterShieldOrbMana float64
	// a cast phase proc on Chain Heal (Totem of Calming Tides), which runs before the cast's heals
	onChainHealCast func(sim *core.Simulation)

	waterShieldManaMetrics *core.ResourceMetrics

	hasHeroicPresence bool
}

// Implemented by each Shaman spec.
type ShamanAgent interface {
	core.Agent

	// The Shaman controlled by this Agent.
	GetShaman() *Shaman
}

func (shaman *Shaman) GetCharacter() *core.Character {
	return &shaman.Character
}

func (shaman *Shaman) HasMajorGlyph(glyph proto.ShamanMajorGlyph) bool {
	return shaman.HasGlyph(int32(glyph))
}
func (shaman *Shaman) HasMinorGlyph(glyph proto.ShamanMinorGlyph) bool {
	return shaman.HasGlyph(int32(glyph))
}

// improveFlametongueTotem raises the totem's 144 spell power by Enhancing Totems' 5% a point, truncated, as the
// totem's aura takes its owner's spell mods: 165 at 3 points. 58655 is a raid area aura, so everyone it reaches
// gets that, pets included.
func (shaman *Shaman) improveFlametongueTotem() {
	if shaman.Talents.EnhancingTotems == 0 {
		return
	}
	spellPower := math.Trunc(144 * (1 + 0.05*float64(shaman.Talents.EnhancingTotems)))
	improve := func(character *core.Character) {
		ee := core.FlametongueTotemAura(character).ExclusiveEffects[0]
		ee.Priority = max(ee.Priority, spellPower)
	}
	for _, party := range shaman.Party.Raid.Parties {
		for _, agent := range party.Players {
			improve(agent.GetCharacter())
			for _, petAgent := range agent.GetCharacter().PetAgents {
				// registering the aura turns it on, so skip a pet that never takes it (dropBuffsItCannotTake)
				if pet := petAgent.GetPet(); pet.BuffImmunity&core.PetImmuneSpellPower == 0 {
					improve(&pet.Character)
				}
			}
		}
	}
}

func (shaman *Shaman) AddRaidBuffs(raidBuffs *proto.RaidBuffs) {
	switch shaman.Totems.Fire {
	case proto.FireTotem_TotemOfWrath:
		raidBuffs.TotemOfWrath = true
	case proto.FireTotem_FlametongueTotem:
		raidBuffs.FlametongueTotem = true
		shaman.improveFlametongueTotem()
	}

	switch shaman.Totems.Water {
	case proto.WaterTotem_ManaSpringTotem:
		raidBuffs.ManaSpringTotem = max(raidBuffs.ManaSpringTotem, proto.TristateEffect_TristateEffectRegular)
		if shaman.Talents.RestorativeTotems == 5 {
			raidBuffs.ManaSpringTotem = proto.TristateEffect_TristateEffectImproved
		}
	}

	switch shaman.Totems.Air {
	case proto.AirTotem_WrathOfAirTotem:
		raidBuffs.WrathOfAirTotem = true
	case proto.AirTotem_WindfuryTotem:
		wfVal := proto.TristateEffect_TristateEffectRegular
		if shaman.Talents.ImprovedWindfuryTotem > 0 {
			wfVal = proto.TristateEffect_TristateEffectImproved
		}
		raidBuffs.WindfuryTotem = max(wfVal, raidBuffs.WindfuryTotem)
	}

	switch shaman.Totems.Earth {
	case proto.EarthTotem_StrengthOfEarthTotem:
		totem := proto.TristateEffect_TristateEffectRegular
		if shaman.Talents.EnhancingTotems == 3 {
			totem = proto.TristateEffect_TristateEffectImproved
		}
		raidBuffs.StrengthOfEarthTotem = max(raidBuffs.StrengthOfEarthTotem, totem)
	case proto.EarthTotem_StoneskinTotem:
		raidBuffs.StoneskinTotem = max(raidBuffs.StoneskinTotem, core.MakeTristateValue(
			true,
			shaman.Talents.GuardianTotems == 2,
		))
	}

	if shaman.Talents.UnleashedRage > 0 {
		raidBuffs.UnleashedRage = true
	}

	if shaman.Talents.ElementalOath > 0 {
		raidBuffs.ElementalOath = true
	}
}
func (shaman *Shaman) AddPartyBuffs(partyBuffs *proto.PartyBuffs) {
	if shaman.Talents.ManaTideTotem {
		partyBuffs.ManaTideTotems++
	}

	shaman.hasHeroicPresence = partyBuffs.HeroicPresence
}

func (shaman *Shaman) Initialize() {
	shaman.registerChainLightningSpell()
	shaman.registerFeralSpirit()
	shaman.registerFireElementalTotem()
	shaman.registerFireNovaSpell()
	shaman.registerLavaBurstSpell()
	shaman.registerLavaLashSpell()
	shaman.registerLightningBoltSpell()
	shaman.registerLightningShieldSpell()
	shaman.registerMagmaTotemSpell()
	shaman.registerManaSpringTotemSpell()
	shaman.registerHealingStreamTotemSpell()
	shaman.registerSearingTotemSpell()
	shaman.registerShocks()
	shaman.registerStormstrikeSpell()
	shaman.registerStrengthOfEarthTotemSpell()
	shaman.registerThunderstormSpell()
	shaman.registerTotemOfWrathSpell()
	shaman.registerFlametongueTotemSpell()
	shaman.registerTremorTotemSpell()
	shaman.registerStoneskinTotemSpell()
	shaman.registerWindfuryTotemSpell()
	shaman.registerWrathOfAirTotemSpell()

	// This registration must come after all the totems are registered
	shaman.registerCallOfTheElements()

	shaman.registerBloodlustCD()

	shaman.NewTemporaryStatsAura("DC Pre-Pull SP Proc", core.ActionID{SpellID: 60494}, stats.Stats{stats.SpellPower: 765}, time.Second*10)
}

func (shaman *Shaman) RegisterHealingSpells() {
	// T7's 2 piece adds a tenth to Improved Shields' share of the orb, in the same spell mod
	shaman.waterShieldOrbMana = 428 * (1 + 0.05*float64(shaman.Talents.ImprovedShields) +
		core.TernaryFloat64(shaman.HasSetBonus(ItemSetEarthshatterRegalia, 2), 0.1, 0))

	shaman.registerAncestralHealingSpell()
	shaman.registerLesserHealingWaveSpell()
	shaman.registerHealingWaveSpell()
	shaman.registerRiptideSpell()
	shaman.registerEarthShieldSpell()
	shaman.registerChainHealSpell()
	shaman.registerTidalForce()
	if shaman.HasSetBonus(ItemSetFrostWitchGarb, 4) {
		shaman.registerChainedHeal()
	}

	if shaman.Talents.TidalWaves > 0 {
		shaman.tidalWaveProc = shaman.GetOrRegisterAura(core.Aura{
			Label:    "Tidal Wave Proc",
			ActionID: core.ActionID{SpellID: 53390},
			Duration: time.Second * 15,
			OnReset: func(aura *core.Aura, sim *core.Simulation) {
				aura.Deactivate(sim)
			},
			OnGain: func(aura *core.Aura, sim *core.Simulation) {
				shaman.HealingWave.CastTimeMultiplier *= 0.7
				shaman.LesserHealingWave.BonusCritRating += core.CritRatingPerCritChance * 25
			},
			OnExpire: func(aura *core.Aura, sim *core.Simulation) {
				shaman.HealingWave.CastTimeMultiplier /= 0.7
				shaman.LesserHealingWave.BonusCritRating -= core.CritRatingPerCritChance * 25
			},
			MaxStacks: 2,
		})
	}
}

func (shaman *Shaman) Reset(sim *core.Simulation) {
	if shaman.Totems.Fire == proto.FireTotem_TotemOfWrath {
		shaman.applyToWDebuff(sim)
	}
	for _, hits := range shaman.earthShieldTargets {
		hits.readyAt = 0
	}
}

func (shaman *Shaman) ElementalCritMultiplier(secondary float64) float64 {
	critBonus := 0.2*float64(shaman.Talents.ElementalFury) + secondary
	return shaman.SpellCritMultiplier(1, critBonus)
}
