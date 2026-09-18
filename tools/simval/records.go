package main

import "encoding/json"

// The JSONL mod-sim-validation writes to <LogsDir>/simval/simval.jsonl. Only the
// fields the combat tables need are here; the module writes a good deal more.

type attackSnapshot struct {
	Type               string  `json:"type"` // mainhand, offhand, ranged
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

// naked is a player with no weapon in any hand, which the base stats checks need.
func (u unitSnapshot) naked() bool {
	for _, a := range u.Attacks {
		if a.Weapon != nil {
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
	Count float64 `json:"count"`
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

func (r record) label() string {
	name := r.Command
	if r.Spell.ID != 0 {
		name += " " + itoa(r.Spell.ID) + " (" + r.Spell.Name + ")"
	}
	if t := r.attackType(); t != "" {
		name += " " + t
	}
	return name + ": " + r.Attacker.Name + " vs " + r.Target.Name
}
