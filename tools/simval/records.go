package main

import "encoding/json"

// The JSONL mod-sim-validation writes to <LogsDir>/simval/simval.jsonl. Only the
// fields the combat tables need are here; the module writes a good deal more.

type attackSnapshot struct {
	Type               string  `json:"type"` // mainhand, offhand, ranged
	UnhastedMs         int32   `json:"unhastedMs"`
	HastedMs           int32   `json:"hastedMs"`
	DamageMin          float64 `json:"damageMin"` // the weapon's own range, before attack power
	DamageMax          float64 `json:"damageMax"`
	MissChanceTaken    float64 `json:"missChanceTaken"`
	ExpertiseReduction float64 `json:"expertiseReduction"`
	WeaponSkillVsOther int32   `json:"weaponSkillVsOther"`
	CritVsOther        float64 `json:"critVsOther"`
	AttackPower        float64 `json:"attackPower"`
	Weapon             *struct {
		ItemID int32 `json:"itemId"`
	} `json:"weapon"`
}

type unitSnapshot struct {
	Name             string  `json:"name"`
	Entry            uint32  `json:"entry"`
	Level            int32   `json:"level"`
	IsPlayer         bool    `json:"isPlayer"`
	IsPet            bool    `json:"isPet"`
	WorldBoss        bool    `json:"worldBoss"`
	FlagsExtra       uint32  `json:"flagsExtra"`
	LevelForOther    int32   `json:"levelForOther"`
	MaxSkillForOther int32   `json:"maxSkillForOther"`
	DefenseSkill     int32   `json:"defenseSkillVsOther"`
	OtherInFront     bool    `json:"otherInFront"`
	ArmorPenAuraPct  float64 `json:"armorPenetrationAuraPct"`

	// The server's class and race ids.
	Class     uint8   `json:"class"`
	Race      uint8   `json:"race"`
	MaxHealth float64 `json:"maxHealth"`
	PowerType uint32  `json:"powerType"`
	Power     float64 `json:"power"`

	Stats struct {
		Strength  float64 `json:"strength"`
		Agility   float64 `json:"agility"`
		Stamina   float64 `json:"stamina"`
		Intellect float64 `json:"intellect"`
		Spirit    float64 `json:"spirit"`
	} `json:"stats"`
	// Armor first, then the magic schools.
	Resistances []float64 `json:"resistances"`

	// SpellBaseDamageBonusDone per school, physical through arcane.
	SpellPower map[string]float64 `json:"spellPower"`

	// The character sheet; players only.
	Sheet *struct {
		// Ranged crit is left out: without a ranged weapon the server's is 0.
		CritMainhand float64   `json:"critMainhand"`
		SpellCrit    []float64 `json:"spellCrit"`
		Dodge        float64   `json:"dodge"`
		Parry        float64   `json:"parry"`
		Block        float64   `json:"block"`
		RealDodge    float64   `json:"realDodge"`
	} `json:"sheet"`
	Ratings []struct {
		Name   string  `json:"name"`
		Rating float64 `json:"rating"`
		Bonus  float64 `json:"bonus"`
	} `json:"ratings"`
	Auras []aura `json:"auras"`

	Defense struct {
		Dodge      float64 `json:"dodge"`
		Parry      float64 `json:"parry"`
		Block      float64 `json:"block"`
		BlockValue float64 `json:"blockValue"`
	} `json:"defense"`

	HitMods struct {
		Melee  float64 `json:"melee"`
		Ranged float64 `json:"ranged"`
		Spell  float64 `json:"spell"`
	} `json:"hitMods"`

	Attacks []attackSnapshot `json:"attacks"`
}

type aura struct {
	ID         int32 `json:"id"`
	DurationMs int32 `json:"durationMs"` // -1 for passives
}

func (u unitSnapshot) rating(name string) (rating, bonus float64) {
	for _, r := range u.Ratings {
		if r.Name == name {
			return r.Rating, r.Bonus
		}
	}
	return 0, 0
}

// bareRatings are the only ratings a character the sim can build bare may carry: the base stats test
// grants defense, dodge and armor penetration through auras, and a death knight's parry comes from
// strength.
var bareRatings = map[string]bool{"defenseSkill": true, "dodge": true, "parry": true, "armorPenetration": true}

// bareBuffAuras are the timed auras the base stats test puts on its characters: the rating auras and
// the raid buffs the sim gives each class. Any other timed aura is a buff the bare sim character
// lacks. Passive auras are left alone, since racial and class passives sit on every character.
var bareBuffAuras = map[int32]bool{
	24774: true, 24775: true, 15804: true, 67694: true, // defense and dodge rating
	71557: true, 71403: true, 42976: true, // armor penetration rating
	48161: true, 48073: true, 48169: true, 57623: true, 42995: true, 48469: true, // raid buffs
}

// naked is a player the sim's bare character can stand in for: no weapon in any hand, no rating beyond
// bareRatings and no timed aura beyond bareBuffAuras.
func (u unitSnapshot) naked() bool {
	for _, a := range u.Attacks {
		if a.Weapon != nil {
			return false
		}
	}
	for _, r := range u.Ratings {
		if r.Rating != 0 && !bareRatings[r.Name] {
			return false
		}
	}
	for _, a := range u.Auras {
		if a.DurationMs > 0 && !bareBuffAuras[a.ID] {
			return false
		}
	}
	return u.IsPlayer && u.Sheet != nil
}

func (u unitSnapshot) attack(kind string) attackSnapshot {
	for _, a := range u.Attacks {
		if a.Type == kind {
			return a
		}
	}
	return attackSnapshot{}
}

// Creature flags_extra, the ones that take an outcome off the table entirely.
const (
	flagExtraNoParry         = 0x00000004
	flagExtraNoBlock         = 0x00000010
	flagExtraNoCrushingBlows = 0x00000020
	flagExtraNoCrit          = 0x00020000
	flagExtraNoDodge         = 0x00800000
)

func (u unitSnapshot) canDodge() bool { return u.FlagsExtra&flagExtraNoDodge == 0 }
func (u unitSnapshot) canParry() bool { return u.FlagsExtra&flagExtraNoParry == 0 }
func (u unitSnapshot) canBlock() bool { return u.FlagsExtra&flagExtraNoBlock == 0 }

type whiteTable struct {
	MissBp     int32 `json:"missBp"`
	DodgeBp    int32 `json:"dodgeBp"`
	ParryBp    int32 `json:"parryBp"`
	BlockBp    int32 `json:"blockBp"`
	GlancingBp int32 `json:"glancingBp"`
	CrushingBp int32 `json:"crushingBp"`
	CritBp     int32 `json:"critBp"`
	SkillBonus int32 `json:"skillBonus"`
}

type yellowTable struct {
	AlwaysHit          bool    `json:"alwaysHit"`
	NoActiveDefense    bool    `json:"noActiveDefense"`
	MissBp             int32   `json:"missBp"`
	CanDodge           bool    `json:"canDodge"`
	CanParry           bool    `json:"canParry"`
	CanBlock           bool    `json:"canBlock"`
	AttackType         string  `json:"attackType"`
	DodgeBp            int32   `json:"dodgeBp"`
	ParryBp            int32   `json:"parryBp"`
	BlockBp            int32   `json:"blockBp"`
	PartialBlockChance float64 `json:"partialBlockChance"`
}

type magicTable struct {
	AutoHit       bool  `json:"autoHit"`
	MissThreshold int32 `json:"missThreshold"`
}

type spellDerived struct {
	Yellow                   *yellowTable `json:"yellow"`
	Magic                    *magicTable  `json:"magic"`
	CritDone                 float64      `json:"critDone"`
	CritTaken                float64      `json:"critTaken"`
	PartialResistsApply      bool         `json:"partialResistsApply"`
	EffectiveResist          float64      `json:"effectiveResist"`
	EffectiveResistWithSpell float64      `json:"effectiveResistWithSpell"`
}

type counted struct {
	Count       float64 `json:"count"`
	MeanDamage  float64 `json:"meanDamage"`
	MeanBlocked float64 `json:"meanBlocked"`
}

type record struct {
	Command    string             `json:"command"`
	AttackType string             `json:"attackType"`
	Iterations float64            `json:"iterations"`
	Attacker   unitSnapshot       `json:"attacker"`
	Target     unitSnapshot       `json:"target"`
	Derived    json.RawMessage    `json:"derived"`
	Outcomes   map[string]counted `json:"outcomes"`
	HitResults map[string]counted `json:"hitResults"`

	Spell struct {
		ID         int32  `json:"id"`
		Name       string `json:"name"`
		SchoolMask uint32 `json:"schoolMask"`
		DmgClass   uint8  `json:"dmgClass"`
		Positive   bool   `json:"positive"`
		Binary     bool   `json:"binary"`
		AttackType string `json:"attackType"`
	} `json:"spell"`

	PartialBlock struct {
		Rolled bool    `json:"rolled"`
		Count  float64 `json:"count"`
	} `json:"partialBlock"`

	Resists *struct {
		Buckets              []float64 `json:"buckets"`
		MeanResistedFraction float64   `json:"meanResistedFraction"`
	} `json:"resists"`

	Scenarios []struct {
		Armor      float64 `json:"armor"`
		Multiplier float64 `json:"multiplier"`
	} `json:"scenarios"`

	// `.simval procs` only. Its spell id is top level, since the record has no
	// single spell the way the table commands do.
	ProcSpellID int32      `json:"spellId"`
	ItemProcs   []itemProc `json:"itemProcs"`
	AuraProcs   []auraProc `json:"auraProcs"`
}

// itemProc is one chance-on-hit item spell or weapon enchant, with the chance the
// server computed per attack type (percent).
type itemProc struct {
	Source        string  `json:"source"` // item or enchant
	Slot          int32   `json:"slot"`
	ItemID        int32   `json:"itemId"`
	EnchantID     int32   `json:"enchantId"`
	SpellID       int32   `json:"spellId"`
	Name          string  `json:"name"`
	PPM           float64 `json:"ppm"`           // item_template's SpellPPMRate
	DBCProcChance int32   `json:"dbcProcChance"` // Spell.dbc ProcChance
	EnchantAmount int32   `json:"enchantAmount"`
	EnchantProc   *struct {
		CustomChance  int32   `json:"customChance"`
		PPM           float64 `json:"ppm"`
		ProcEx        uint32  `json:"procEx"`
		AttributeMask uint32  `json:"attributeMask"`
	} `json:"enchantProcEntry"`
	Chance map[string]float64 `json:"chance"`
}

// auraProc is an applied aura's spell_proc entry and the chance Aura::CalcProcChance
// gives it per attack type (percent). The "spell" key is the chance for the record's
// own spell id.
type auraProc struct {
	ID        int32  `json:"id"`
	Name      string `json:"name"`
	FromSelf  bool   `json:"fromSelf"`
	ProcEntry struct {
		SchoolMask         uint8     `json:"schoolMask"`
		SpellFamilyName    int32     `json:"spellFamilyName"`
		SpellFamilyMask    [3]uint32 `json:"spellFamilyMask"`
		ProcFlags          uint32    `json:"procFlags"`
		SpellTypeMask      uint32    `json:"spellTypeMask"`
		SpellPhaseMask     uint32    `json:"spellPhaseMask"`
		HitMask            uint32    `json:"hitMask"`
		AttributesMask     uint32    `json:"attributesMask"`
		DisableEffectsMask uint32    `json:"disableEffectsMask"`
		ProcsPerMinute     float64   `json:"procsPerMinute"`
		Chance             float64   `json:"chance"`
		CooldownMs         int64     `json:"cooldownMs"`
		Charges            int32     `json:"charges"`
	} `json:"procEntry"`
	Chance map[string]float64 `json:"chance"`
}

// SPELL_SCHOOL_MASK_NORMAL. Physical spells never partially resist, and they're
// the only ones isSpellBlocked is rolled for.
const schoolMaskNormal = 1

func (r record) physical() bool { return r.Spell.SchoolMask&schoolMaskNormal != 0 }

// attackType is top level for the white table commands and under the spell for
// the rest.
func (r record) attackType() string {
	if r.AttackType != "" {
		return r.AttackType
	}
	return r.Spell.AttackType
}

// SPELL_DAMAGE_CLASS_RANGED. The server picks the hit table's attack type from it, not from the weapon
// the spell uses: a judgement is ranged, with no ranged weapon in sight.
const dmgClassRanged = 3

// tableAttackType is the attack the yellow table was rolled for.
func (r record) tableAttackType() string {
	if r.Spell.DmgClass == dmgClassRanged {
		return "ranged"
	}
	return "mainhand"
}

// The dummies' entries; they all share faction 7.
func isDummy(entry uint32) bool { return entry >= 999000 && entry <= 999003 }

func (r record) positiveAtDummy() bool { return r.Spell.Positive && isDummy(r.Target.Entry) }

func (r record) label() string {
	name := r.Command
	switch {
	case r.Spell.ID != 0:
		name += " " + itoa(r.Spell.ID) + " (" + r.Spell.Name + ")"
	case r.ProcSpellID != 0:
		name += " " + itoa(r.ProcSpellID)
	}
	if t := r.attackType(); t != "" {
		name += " " + t
	}
	return name + ": " + r.Attacker.Name + " vs " + r.Target.Name
}
