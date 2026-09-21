package hunter

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

var TalentTreeSizes = [3]int{26, 27, 28}

const ThoridalTheStarsFuryItemID = 34334

func RegisterHunter() {
	core.RegisterAgentFactory(
		proto.Player_Hunter{},
		proto.Spec_SpecHunter,
		func(character *core.Character, options *proto.Player) core.Agent {
			return NewHunter(character, options)
		},
		func(player *proto.Player, spec interface{}) {
			playerSpec, ok := spec.(*proto.Player_Hunter)
			if !ok {
				panic("Invalid spec value for Hunter!")
			}
			player.Spec = playerSpec
		},
	)
}

type Hunter struct {
	core.Character

	Talents *proto.HunterTalents
	Options *proto.Hunter_Options

	pet *HunterPet

	// Ammo adds its DPS times the weapon's own speed to a plain weapon hit, and times 2.8 to a
	// normalized one (Player::CalculateMinMaxDamage, Unit::GetAPMultiplier).
	AmmoDPS                   float64
	AmmoDamageBonus           float64
	NormalizedAmmoDamageBonus float64

	// The most recent time at which moving could have started, for trap weaving.
	mayMoveAt time.Duration

	AspectOfTheDragonhawk *core.Spell
	AspectOfTheViper      *core.Spell

	AimedShot       *core.Spell
	ArcaneShot      *core.Spell
	BlackArrow      *core.Spell
	ChimeraShot     *core.Spell
	ExplosiveShotR4 *core.Spell
	ExplosiveShotR3 *core.Spell
	ExplosiveTrap   *core.Spell
	KillCommand     *core.Spell
	KillShot        *core.Spell
	MultiShot       *core.Spell
	RapidFire       *core.Spell
	RaptorStrike    *core.Spell
	ScorpidSting    *core.Spell
	SerpentSting    *core.Spell
	SilencingShot   *core.Spell
	SteadyShot      *core.Spell
	Volley          *core.Spell

	// Fake spells to encapsulate weaving logic.
	TrapWeaveSpell *core.Spell

	AspectOfTheDragonhawkAura *core.Aura
	AspectOfTheViperAura      *core.Aura
	ImprovedSteadyShotAura    *core.Aura
	LockAndLoadAura           *core.Aura
	RapidFireAura             *core.Aura
	ScorpidStingAuras         core.AuraArray
	TalonOfAlarAura           *core.Aura
}

func (hunter *Hunter) GetCharacter() *core.Character {
	return &hunter.Character
}

func (hunter *Hunter) HasMajorGlyph(glyph proto.HunterMajorGlyph) bool {
	return hunter.HasGlyph(int32(glyph))
}
func (hunter *Hunter) HasMinorGlyph(glyph proto.HunterMinorGlyph) bool {
	return hunter.HasGlyph(int32(glyph))
}

func (hunter *Hunter) GetHunter() *Hunter {
	return hunter
}

func (hunter *Hunter) AddRaidBuffs(raidBuffs *proto.RaidBuffs) {
	if hunter.Talents.TrueshotAura {
		raidBuffs.TrueshotAura = true
	}
	if hunter.Talents.FerociousInspiration == 3 && hunter.pet != nil {
		raidBuffs.FerociousInspiration = true
	}
}
func (hunter *Hunter) AddPartyBuffs(_ *proto.PartyBuffs) {
}

func (hunter *Hunter) Initialize() {
	// Update auto crit multipliers now that we have the targets.
	hunter.AutoAttacks.MHConfig().CritMultiplier = hunter.critMultiplier(false, false)
	hunter.AutoAttacks.OHConfig().CritMultiplier = hunter.critMultiplier(false, false)
	hunter.AutoAttacks.RangedConfig().CritMultiplier = hunter.critMultiplier(false, false)

	hunter.registerAspectOfTheDragonhawkSpell()
	hunter.registerAspectOfTheViperSpell()

	multiShotTimer := hunter.NewTimer()
	arcaneShotTimer := hunter.NewTimer()
	fireTrapTimer := hunter.NewTimer()

	hunter.registerAimedShotSpell(multiShotTimer)
	hunter.registerArcaneShotSpell(arcaneShotTimer)
	hunter.registerBlackArrowSpell(fireTrapTimer)
	hunter.registerChimeraShotSpell()
	hunter.registerExplosiveShotSpell(arcaneShotTimer)
	hunter.registerExplosiveTrapSpell(fireTrapTimer)
	hunter.registerKillShotSpell()
	hunter.registerMultiShotSpell(multiShotTimer)
	hunter.registerRaptorStrikeSpell()
	hunter.registerScorpidStingSpell()
	hunter.registerSerpentStingSpell()
	hunter.registerSilencingShotSpell()
	hunter.registerSteadyShotSpell()
	hunter.registerVolleySpell()

	hunter.registerKillCommandCD()
	hunter.registerRapidFireCD()

	if hunter.Options.UseHuntersMark {
		hunter.RegisterPrepullAction(0, func(sim *core.Simulation) {
			huntersMarkAura := core.HuntersMarkAura(hunter.CurrentTarget, hunter.Talents.ImprovedHuntersMark, hunter.HasMajorGlyph(proto.HunterMajorGlyph_GlyphOfHuntersMark))
			huntersMarkAura.Activate(sim)
		})
	}
}

func (hunter *Hunter) Reset(_ *core.Simulation) {
	hunter.mayMoveAt = 0
}

func NewHunter(character *core.Character, options *proto.Player) *Hunter {
	hunterOptions := options.GetHunter()

	hunter := &Hunter{
		Character: *character,
		Talents:   &proto.HunterTalents{},
		Options:   hunterOptions.Options,
	}
	core.FillTalentsProto(hunter.Talents.ProtoReflect(), options.TalentsString, TalentTreeSizes)
	hunter.EnableManaBar()

	hunter.PseudoStats.CanParry = true

	rangedWeapon := hunter.WeaponFromRanged(0)

	hunter.PseudoStats.RangedSpeedMultiplier *= quiverHaste(hunter.Options.Quiver, hunter.GetRangedWeapon())

	if hunter.HasRangedWeapon() && hunter.GetRangedWeapon().ID != ThoridalTheStarsFuryItemID {
		switch hunter.Options.Ammo {
		case proto.Hunter_Options_IcebladeArrow:
			hunter.AmmoDPS = 91.5
		case proto.Hunter_Options_SaroniteRazorheads:
			hunter.AmmoDPS = 67.5
		case proto.Hunter_Options_TerrorshaftArrow:
			hunter.AmmoDPS = 46.5
		case proto.Hunter_Options_TimelessArrow:
			hunter.AmmoDPS = 53
		case proto.Hunter_Options_MysteriousArrow:
			hunter.AmmoDPS = 46.5
		case proto.Hunter_Options_AdamantiteStinger:
			hunter.AmmoDPS = 43
		case proto.Hunter_Options_BlackflightArrow:
			hunter.AmmoDPS = 32
		}
		hunter.AmmoDamageBonus = hunter.AmmoDPS * rangedWeapon.SwingSpeed
		hunter.NormalizedAmmoDamageBonus = hunter.AmmoDPS * 2.8
	}

	hunter.EnableAutoAttacks(hunter, core.AutoAttackOptions{
		// We don't know crit multiplier until later when we see the target so just
		// use 0 for now.
		MainHand:        hunter.WeaponFromMainHand(0),
		OffHand:         hunter.WeaponFromOffHand(0),
		Ranged:          rangedWeapon,
		ReplaceMHSwing:  hunter.TryRaptorStrike,
		AutoSwingRanged: true,
	})
	hunter.AutoAttacks.RangedConfig().ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		baseDamage := hunter.RangedWeaponDamage(sim, spell.RangedAttackPower(target)) +
			hunter.AmmoDamageBonus +
			spell.BonusWeaponDamage()
		spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
	}

	hunter.pet = hunter.NewHunterPet()

	return hunter
}

// quiverHaste is the ranged attack speed the quiver or ammo pouch gives. The core has no hunter haste
// of its own, and mod-individual-progression's 89507 never changes the speed (TestSimvalHunter
// measured it), so this is all of it: the bag's SPELL_AURA_MOD_RANGED_AMMO_HASTE
// (AuraEffect::HandleRangedAmmoHaste), which needs a weapon the bag fits.
func quiverHaste(quiver proto.Hunter_Options_Quiver, weapon *core.Item) float64 {
	if quiver != proto.Hunter_Options_Quiver15Percent || weapon == nil {
		return 1
	}
	switch weapon.RangedWeaponType {
	case proto.RangedWeaponType_RangedWeaponTypeBow, proto.RangedWeaponType_RangedWeaponTypeCrossbow, proto.RangedWeaponType_RangedWeaponTypeGun:
		return 1.15
	}
	return 1
}

// Agent is a generic way to access underlying hunter on any of the agents.
type HunterAgent interface {
	GetHunter() *Hunter
}
