package azerothcore

import (
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
)

// AzerothCore enum values used below (ItemTemplate.h, SharedDefines.h, SpellAuraDefines.h, Unit.h).
const (
	SpellEffectApplyAura    = 6
	SpellEffectCreateItem   = 24
	SpellEffectTriggerSpell = 64
	SpellEffectCreateItem2  = 157

	AuraModDamageDone        = 13
	AuraModResistance        = 22
	AuraPeriodicTriggerSpell = 23
	AuraModStat              = 29
	AuraModIncreaseHealth    = 34
	AuraProcTriggerSpell     = 42
	AuraModPowerRegen        = 85
	AuraModAttackPower       = 99
	AuraModTargetResistance  = 123
	AuraModRangedAttack      = 124
	AuraModHealingDone       = 135
	AuraModShieldBlockValue  = 158
	AuraModRating            = 189

	EnchantTypeCombatSpell = 1
	EnchantTypeDamage      = 2
	EnchantTypeEquipSpell  = 3
	EnchantTypeResistance  = 4
	EnchantTypeStat        = 5
	EnchantTypeUseSpell    = 7

	ItemSpellTriggerOnUse       = 0
	ItemSpellTriggerOnEquip     = 1
	ItemSpellTriggerChanceOnHit = 2

	ItemFlagHeroicTooltip = 0x8

	allMagicSchools         = 126 // holy|fire|nature|frost|shadow|arcane
	spellPenetrationSchools = 124 // fire|nature|frost|shadow|arcane, holy optional
)

// Combat rating indices; MOD_RATING auras use them as a bit mask.
const (
	crDefenseSkill    = 1
	crDodge           = 2
	crParry           = 3
	crBlock           = 4
	crHitMelee        = 5
	crHitRanged       = 6
	crHitSpell        = 7
	crCritMelee       = 8
	crCritRanged      = 9
	crCritSpell       = 10
	crCritTakenMelee  = 14
	crCritTakenRanged = 15
	crCritTakenSpell  = 16
	crHasteMelee      = 17
	crHasteRanged     = 18
	crHasteSpell      = 19
	crExpertise       = 23
	crArmorPen        = 24
)

// AddItemMod adds an item_template stat_type/stat_value pair (ItemModType) to s, using the same
// layout the Wowhead tooltip parser produces: generic hit/crit/haste ratings fill both the melee
// and spell stats, and attack power also counts as ranged attack power. Returns false for mod types
// the sim has no stat for.
func AddItemMod(s *database.Stats, modType, value int32) bool {
	v := float64(value)
	switch modType {
	case 0:
		s[proto.Stat_StatMana] += v
	case 1:
		s[proto.Stat_StatHealth] += v
	case 3:
		s[proto.Stat_StatAgility] += v
	case 4:
		s[proto.Stat_StatStrength] += v
	case 5:
		s[proto.Stat_StatIntellect] += v
	case 6:
		s[proto.Stat_StatSpirit] += v
	case 7:
		s[proto.Stat_StatStamina] += v
	case 12:
		s[proto.Stat_StatDefense] += v
	case 13:
		s[proto.Stat_StatDodge] += v
	case 14:
		s[proto.Stat_StatParry] += v
	case 15:
		s[proto.Stat_StatBlock] += v
	case 16, 17:
		s[proto.Stat_StatMeleeHit] += v
	case 18:
		s[proto.Stat_StatSpellHit] += v
	case 19, 20:
		s[proto.Stat_StatMeleeCrit] += v
	case 21:
		s[proto.Stat_StatSpellCrit] += v
	case 28, 29:
		s[proto.Stat_StatMeleeHaste] += v
	case 30:
		s[proto.Stat_StatSpellHaste] += v
	case 31:
		s[proto.Stat_StatMeleeHit] += v
		s[proto.Stat_StatSpellHit] += v
	case 32:
		s[proto.Stat_StatMeleeCrit] += v
		s[proto.Stat_StatSpellCrit] += v
	case 35:
		s[proto.Stat_StatResilience] += v
	case 36:
		s[proto.Stat_StatMeleeHaste] += v
		s[proto.Stat_StatSpellHaste] += v
	case 37:
		s[proto.Stat_StatExpertise] += v
	case 38:
		s[proto.Stat_StatAttackPower] += v
		s[proto.Stat_StatRangedAttackPower] += v
	case 39:
		s[proto.Stat_StatRangedAttackPower] += v
	case 42, 45: // 42 is the pre-3.0 spell damage mod, which the 3.3.5 client shows as spell power
		s[proto.Stat_StatSpellPower] += v
	case 43:
		s[proto.Stat_StatMP5] += v
	case 44:
		s[proto.Stat_StatArmorPenetration] += v
	case 47:
		s[proto.Stat_StatSpellPenetration] += v
	case 48:
		s[proto.Stat_StatBlockValue] += v
	default:
		return false
	}
	return true
}

// AddEquipSpellStats adds the flat stats granted by an on-equip spell. It returns false, leaving s
// untouched, when any effect is something other than a flat stat aura; such spells are item effects
// the sim implements in code.
func AddEquipSpellStats(s *database.Stats, spell *SpellEntry) bool {
	var add database.Stats
	hasDamageDone := false
	for i := 0; i < 3; i++ {
		if spell.Effect[i] == SpellEffectApplyAura && spell.EffectApplyAuraName[i] == AuraModDamageDone {
			hasDamageDone = true
		}
	}

	found := false
	for i := 0; i < 3; i++ {
		if spell.Effect[i] == 0 {
			continue
		}
		if spell.Effect[i] != SpellEffectApplyAura {
			return false
		}
		value := float64(spell.EffectValue(i))
		misc := spell.EffectMiscValue[i]
		switch spell.EffectApplyAuraName[i] {
		case AuraModDamageDone:
			if misc&allMagicSchools != allMagicSchools {
				return false
			}
			add[proto.Stat_StatSpellPower] += value
		case AuraModHealingDone:
			// 3.x spell power spells pair damage done with an equal healing done aura.
			if !hasDamageDone {
				return false
			}
		case AuraModAttackPower:
			// Melee only; attack power spells carry a separate ranged aura, unlike item stat 38.
			add[proto.Stat_StatAttackPower] += value
		case AuraModRangedAttack:
			add[proto.Stat_StatRangedAttackPower] += value
		case AuraModPowerRegen:
			if misc != 0 { // 0 = mana
				return false
			}
			add[proto.Stat_StatMP5] += value
		case AuraModShieldBlockValue:
			add[proto.Stat_StatBlockValue] += value
		case AuraModTargetResistance:
			// Only the magic school masks are spell penetration; the physical bit ignores armor.
			if misc&1 != 0 || misc&spellPenetrationSchools != spellPenetrationSchools {
				return false
			}
			add[proto.Stat_StatSpellPenetration] += -value
		case AuraModIncreaseHealth:
			add[proto.Stat_StatHealth] += value
		case AuraModStat:
			if !addPrimaryStat(&add, misc, value) {
				return false
			}
		case AuraModResistance:
			addResistances(&add, misc, value)
		case AuraModRating:
			if !addRatings(&add, misc, value) {
				return false
			}
		default:
			return false
		}
		found = true
	}
	if !found {
		return false
	}
	for i := range add {
		s[i] += add[i]
	}
	return true
}

func addPrimaryStat(s *database.Stats, stat int32, value float64) bool {
	stats := map[int32]proto.Stat{
		0: proto.Stat_StatStrength,
		1: proto.Stat_StatAgility,
		2: proto.Stat_StatStamina,
		3: proto.Stat_StatIntellect,
		4: proto.Stat_StatSpirit,
	}
	if stat == -1 {
		for _, st := range stats {
			s[st] += value
		}
		return true
	}
	st, ok := stats[stat]
	if ok {
		s[st] += value
	}
	return ok
}

// addResistances handles a SpellSchoolMask where bit 0 (physical) means armor.
func addResistances(s *database.Stats, schoolMask int32, value float64) {
	schools := []struct {
		mask int32
		stat proto.Stat
	}{
		{1, proto.Stat_StatBonusArmor},
		{4, proto.Stat_StatFireResistance},
		{8, proto.Stat_StatNatureResistance},
		{16, proto.Stat_StatFrostResistance},
		{32, proto.Stat_StatShadowResistance},
		{64, proto.Stat_StatArcaneResistance},
	}
	for _, school := range schools {
		if schoolMask&school.mask != 0 {
			s[school.stat] += value
		}
	}
}

func addRatings(s *database.Stats, mask int32, value float64) bool {
	has := func(cr int) bool { return mask&(1<<cr) != 0 }
	found := false
	apply := func(cond bool, stat proto.Stat) {
		if cond {
			s[stat] += value
			found = true
		}
	}
	apply(has(crDefenseSkill), proto.Stat_StatDefense)
	apply(has(crDodge), proto.Stat_StatDodge)
	apply(has(crParry), proto.Stat_StatParry)
	apply(has(crBlock), proto.Stat_StatBlock)
	apply(has(crHitMelee) || has(crHitRanged), proto.Stat_StatMeleeHit)
	apply(has(crHitSpell), proto.Stat_StatSpellHit)
	apply(has(crCritMelee) || has(crCritRanged), proto.Stat_StatMeleeCrit)
	apply(has(crCritSpell), proto.Stat_StatSpellCrit)
	apply(has(crCritTakenMelee) || has(crCritTakenRanged) || has(crCritTakenSpell), proto.Stat_StatResilience)
	apply(has(crHasteMelee) || has(crHasteRanged), proto.Stat_StatMeleeHaste)
	apply(has(crHasteSpell), proto.Stat_StatSpellHaste)
	apply(has(crExpertise), proto.Stat_StatExpertise)
	apply(has(crArmorPen), proto.Stat_StatArmorPenetration)
	return found
}

// EnchantSpell is a spell an enchantment casts or applies. Amount is the proc chance for combat
// spells.
type EnchantSpell struct {
	EnchantID int32
	Type      int32
	SpellID   int32
	Amount    int32
}

// EnchantmentStats sums the stat and resistance effects of a SpellItemEnchantment, including equip
// spells that only grant flat stats (spell penetration and all-stats gems work that way). Effects
// that cast other spells or add weapon damage are returned separately so callers can handle them.
func EnchantmentStats(enchant *SpellItemEnchantmentEntry, dbc *DBC) (stats database.Stats, spells []EnchantSpell, weaponDamage int32) {
	addSpell := func(i int) {
		if enchant.Arg[i] != 0 {
			spells = append(spells, EnchantSpell{EnchantID: enchant.ID, Type: enchant.Type[i], SpellID: enchant.Arg[i], Amount: enchant.Amount[i]})
		}
	}
	for i := 0; i < 3; i++ {
		switch enchant.Type[i] {
		case EnchantTypeStat:
			AddItemMod(&stats, enchant.Arg[i], enchant.Amount[i])
		case EnchantTypeResistance:
			// Arg is a school index here, not a mask.
			addResistances(&stats, 1<<enchant.Arg[i], float64(enchant.Amount[i]))
		case EnchantTypeDamage:
			weaponDamage += enchant.Amount[i]
		case EnchantTypeEquipSpell:
			if spell := dbc.Spells[enchant.Arg[i]]; spell != nil && AddEquipSpellStats(&stats, spell) {
				continue
			}
			addSpell(i)
		case EnchantTypeCombatSpell, EnchantTypeUseSpell:
			addSpell(i)
		}
	}
	return stats, spells, weaponDamage
}

// SocketGemColor maps item_template socketColor values to the sim's GemColor.
func SocketGemColor(socketColor int32) proto.GemColor {
	switch socketColor {
	case 1:
		return proto.GemColor_GemColorMeta
	case 2:
		return proto.GemColor_GemColorRed
	case 4:
		return proto.GemColor_GemColorYellow
	case 8:
		return proto.GemColor_GemColorBlue
	}
	return proto.GemColor_GemColorUnknown
}

// GemPropertiesColor maps a GemProperties.dbc color mask (the sockets a gem matches) to GemColor.
func GemPropertiesColor(mask int32) proto.GemColor {
	switch mask {
	case 1:
		return proto.GemColor_GemColorMeta
	case 2:
		return proto.GemColor_GemColorRed
	case 4:
		return proto.GemColor_GemColorYellow
	case 8:
		return proto.GemColor_GemColorBlue
	case 6:
		return proto.GemColor_GemColorOrange
	case 10:
		return proto.GemColor_GemColorPurple
	case 12:
		return proto.GemColor_GemColorGreen
	case 14:
		return proto.GemColor_GemColorPrismatic
	}
	return proto.GemColor_GemColorUnknown
}

// AllowedClasses converts an AllowableClass bit mask (bit n = class ID n+1) to the sim's classes.
// Masks that allow every class return nil, matching the sim's "no restriction".
func AllowedClasses(mask int32) []proto.Class {
	byClassID := []struct {
		id    int
		class proto.Class
	}{
		{1, proto.Class_ClassWarrior},
		{2, proto.Class_ClassPaladin},
		{3, proto.Class_ClassHunter},
		{4, proto.Class_ClassRogue},
		{5, proto.Class_ClassPriest},
		{6, proto.Class_ClassDeathknight},
		{7, proto.Class_ClassShaman},
		{8, proto.Class_ClassMage},
		{9, proto.Class_ClassWarlock},
		{11, proto.Class_ClassDruid},
	}
	var classes []proto.Class
	for _, c := range byClassID {
		if mask&(1<<(c.id-1)) != 0 {
			classes = append(classes, c.class)
		}
	}
	if mask <= 0 || len(classes) == len(byClassID) {
		return nil
	}
	return classes
}
