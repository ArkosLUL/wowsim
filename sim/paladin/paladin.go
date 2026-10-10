package paladin

import (
	"math"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const (
	SpellFlagSecondaryJudgement = core.SpellFlagAgentReserved1
	SpellFlagPrimaryJudgement   = core.SpellFlagAgentReserved2
)

var TalentTreeSizes = [3]int{26, 26, 26}

type Paladin struct {
	core.Character

	PaladinAura proto.PaladinAura

	Talents *proto.PaladinTalents

	CurrentSeal      *core.Aura
	CurrentJudgement *core.Aura

	DivinePlea            *core.Spell
	DivineStorm           *core.Spell
	HolyWrath             *core.Spell
	Consecration          *core.Spell
	CrusaderStrike        *core.Spell
	Exorcism              *core.Spell
	HolyShield            *core.Spell
	HammerOfTheRighteous  *core.Spell
	ShieldOfRighteousness *core.Spell
	AvengersShield        *core.Spell
	JudgementOfWisdom     *core.Spell
	JudgementOfLight      *core.Spell
	HammerOfWrath         *core.Spell
	SealOfVengeance       *core.Spell
	SealOfRighteousness   *core.Spell
	SealOfCommand         *core.Spell
	AvengingWrath         *core.Spell
	DivineProtection      *core.Spell
	SovDotSpell           *core.Spell
	// SealOfWisdom        *core.Spell
	// SealOfLight         *core.Spell

	HolyShieldAura          *core.Aura
	RighteousFuryAura       *core.Aura
	DivinePleaAura          *core.Aura
	JudgementOfWisdomAura   *core.Aura
	JudgementOfLightAura    *core.Aura
	SealOfVengeanceAura     *core.Aura
	SealOfCommandAura       *core.Aura
	SealOfRighteousnessAura *core.Aura
	AvengingWrathAura       *core.Aura
	DivineProtectionAura    *core.Aura
	ForbearanceAura         *core.Aura
	VengeanceAura           *core.Aura

	// SealOfWisdomAura        *core.Aura
	// SealOfLightAura         *core.Aura

	ArtOfWarInstantCast *core.Aura

	SpiritualAttunementMetrics *core.ResourceMetrics

	DemonAndUndeadTargetCount int32

	mutualLockoutDPAW *core.Timer

	// the raid's buffs, final by Initialize
	raidBuffs *proto.RaidBuffs
}

// Implemented by each Paladin spec.
type PaladinAgent interface {
	GetPaladin() *Paladin
}

func (paladin *Paladin) GetCharacter() *core.Character {
	return &paladin.Character
}

func (paladin *Paladin) HasMajorGlyph(glyph proto.PaladinMajorGlyph) bool {
	return paladin.HasGlyph(int32(glyph))
}
func (paladin *Paladin) HasMinorGlyph(glyph proto.PaladinMinorGlyph) bool {
	return paladin.HasGlyph(int32(glyph))
}

func (paladin *Paladin) GetPaladin() *Paladin {
	return paladin
}

func (paladin *Paladin) AddRaidBuffs(raidBuffs *proto.RaidBuffs) {
	paladin.raidBuffs = raidBuffs
	raidBuffs.DevotionAura = max(raidBuffs.DevotionAura, core.MakeTristateValue(
		paladin.PaladinAura == proto.PaladinAura_DevotionAura,
		paladin.Talents.ImprovedDevotionAura == 3))

	if paladin.PaladinAura == proto.PaladinAura_RetributionAura {
		raidBuffs.RetributionAura = true
	}

	// Both talents work through the paladin's own aura: their shared buff (63531) drops off anyone
	// without one (spell_paladin.cpp, spell_pal_sanctified_retribution_effect::CheckAreaTarget).
	hasAura := paladin.PaladinAura != proto.PaladinAura_NoPaladinAura

	if paladin.Talents.SanctifiedRetribution && hasAura {
		raidBuffs.SanctifiedRetribution = true
	}

	if paladin.Talents.SwiftRetribution == 3 && hasAura {
		raidBuffs.SwiftRetribution = paladin.Talents.SwiftRetribution == 3 // TODO: Fix-- though having something between 0/3 and 3/3 is unlikely
	}

	// TODO: Figure out a way to just start with 1 DG cooldown available without making a redundant Spell
	//if paladin.Talents.DivineGuardian == 2 {
	//	raidBuffs.divineGuardians++
	//}
}

func (paladin *Paladin) AddPartyBuffs(_ *proto.PartyBuffs) {
}

// improveDevotionAura gives the raid's Devotion Aura Improved Devotion Aura's 17% a point at 1 or 2 points,
// which the raid buff can't carry: it only knows none or all 3. The paladin's spell mod scales the aura's
// armor (Unit::ApplyEffectModifiers), so every raider and pet under it gets that amount. The talent's 63514
// reaches the same units with 2% healing taken a point.
func (paladin *Paladin) improveDevotionAura() {
	points := paladin.Talents.ImprovedDevotionAura
	if paladin.PaladinAura != proto.PaladinAura_DevotionAura || points == 0 || points == 3 ||
		paladin.raidBuffs == nil || paladin.raidBuffs.DevotionAura != proto.TristateEffect_TristateEffectRegular {
		return
	}
	// the best Devotion Aura counts, the first of equals adds it
	for _, agent := range paladin.Env.Raid.GetPlayersOfClass(proto.Class_ClassPaladin) {
		other := agent.(PaladinAgent).GetPaladin()
		if other == paladin || other.PaladinAura != proto.PaladinAura_DevotionAura {
			continue
		}
		if p := other.Talents.ImprovedDevotionAura; p > points || p == points && other.Index < paladin.Index {
			return
		}
	}
	bonus := math.Trunc(1205 * float64(17*points) / 100)
	healingTaken := 2 * points
	for _, party := range paladin.Env.Raid.Parties {
		for _, player := range party.Players {
			if _, ok := player.(*core.TargetDummy); ok {
				continue
			}
			character := player.GetCharacter()
			character.AddStat(stats.Armor, bonus)
			character.AddHealingTakenPct(healingTaken)
			for _, pet := range character.Pets {
				// 63514 needs the paladin's Devotion Aura on the unit, which an armor-immune summon never takes
				if pet.BuffImmunity&core.PetImmuneArmor == 0 {
					pet.AddStat(stats.Armor, bonus)
					pet.AddHealingTakenPct(healingTaken)
				}
			}
		}
	}
}

func (paladin *Paladin) Initialize() {
	// Update auto crit multipliers now that we have the targets.
	paladin.AutoAttacks.MHConfig().CritMultiplier = paladin.MeleeCritMultiplier()

	paladin.registerSealOfVengeanceSpellAndAura()
	paladin.registerSealOfRighteousnessSpellAndAura()
	paladin.registerSealOfCommandSpellAndAura()
	// paladin.setupSealOfTheCrusader()
	// paladin.setupSealOfWisdom()
	// paladin.setupSealOfLight()
	// paladin.setupSealOfRighteousness()
	// paladin.setupJudgementRefresh()

	paladin.registerCrusaderStrikeSpell()
	paladin.registerDivineStormSpell()
	paladin.registerConsecrationSpell()
	paladin.registerHammerOfWrathSpell()
	paladin.registerHolyWrathSpell()

	paladin.registerExorcismSpell()
	paladin.registerHolyShieldSpell()
	paladin.registerHammerOfTheRighteousSpell()
	paladin.registerHandOfReckoningSpell()
	paladin.registerShieldOfRighteousnessSpell()
	paladin.registerAvengersShieldSpell()
	paladin.registerJudgements()

	paladin.registerSpiritualAttunement()
	paladin.registerDivinePleaSpell()
	paladin.registerDivineProtectionSpell()
	paladin.registerForbearanceDebuff()

	for i := int32(0); i < paladin.Env.GetNumTargets(); i++ {
		unit := paladin.Env.GetTargetUnit(i)
		if unit.MobType == proto.MobType_MobTypeDemon || unit.MobType == proto.MobType_MobTypeUndead {
			paladin.DemonAndUndeadTargetCount += 1
		}
	}

	paladin.improveDevotionAura()
}

func (paladin *Paladin) Reset(_ *core.Simulation) {
	paladin.CurrentSeal = nil
	paladin.CurrentJudgement = nil
}

// maybe need to add stat dependencies
func NewPaladin(character *core.Character, talentsStr string) *Paladin {
	paladin := &Paladin{
		Character: *character,
		Talents:   &proto.PaladinTalents{},
	}
	core.FillTalentsProto(paladin.Talents.ProtoReflect(), talentsStr, TalentTreeSizes)

	paladin.PseudoStats.CanParry = true

	paladin.EnableManaBar()

	return paladin
}

// Shared 30sec cooldown for Divine Protection and Avenging Wrath
func (paladin *Paladin) GetMutualLockoutDPAW() *core.Timer {
	return paladin.Character.GetOrInitTimer(&paladin.mutualLockoutDPAW)
}
