// Package spellset works out which spells the sim needs server data for and reads the server's
// spell capture (`.simval spelldump`, assets/db_inputs/acore/spelldump.jsonl).
package spellset

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
)

// DefaultDumpPath is the committed capture, relative to the repo root.
const DefaultDumpPath = "assets/db_inputs/acore/spelldump.jsonl"

// DumpSpell is one spelldump line: the worldserver's in-memory SpellInfo after Spell.dbc, spell_dbc and
// its spell corrections. Integers are int64 because the module prints C++ uint32s, e.g. PowerType -2 as
// 4294967294.
type DumpSpell struct {
	ID                       int64        `json:"id"`
	Name                     string       `json:"name"`
	Rank                     string       `json:"rank"`
	FirstRankID              int64        `json:"firstRankId"`
	RankIndex                int64        `json:"rankIndex"`
	Family                   int64        `json:"family"`
	FamilyFlags              [3]int64     `json:"familyFlags"`
	SchoolMask               int64        `json:"schoolMask"`
	DmgClass                 int64        `json:"dmgClass"`
	PreventionType           int64        `json:"preventionType"`
	Dispel                   int64        `json:"dispel"`
	Mechanic                 int64        `json:"mechanic"`
	Attributes               [8]int64     `json:"attributes"`
	AttributesCu             int64        `json:"attributesCu"`
	CastTimeBaseMs           int64        `json:"castTimeBaseMs"`
	CastTimeMs               int64        `json:"castTimeMs"`
	RangedWeaponSpell        bool         `json:"rangedWeaponSpell"`
	AutoRepeatRanged         bool         `json:"autoRepeatRanged"`
	RecoveryTimeMs           int64        `json:"recoveryTimeMs"`
	CategoryRecoveryTimeMs   int64        `json:"categoryRecoveryTimeMs"`
	Category                 int64        `json:"category"`
	StartRecoveryCategory    int64        `json:"startRecoveryCategory"`
	StartRecoveryTimeMs      int64        `json:"startRecoveryTimeMs"`
	DurationMs               int64        `json:"durationMs"`
	MaxDurationMs            int64        `json:"maxDurationMs"`
	StackAmount              int64        `json:"stackAmount"`
	ProcFlags                int64        `json:"procFlags"`
	ProcChance               int64        `json:"procChance"`
	ProcCharges              int64        `json:"procCharges"`
	InterruptFlags           int64        `json:"interruptFlags"`
	AuraInterruptFlags       int64        `json:"auraInterruptFlags"`
	ChannelInterruptFlags    int64        `json:"channelInterruptFlags"`
	SpellLevel               int64        `json:"spellLevel"`
	BaseLevel                int64        `json:"baseLevel"`
	MaxLevel                 int64        `json:"maxLevel"`
	PowerType                int64        `json:"powerType"`
	ManaCost                 int64        `json:"manaCost"`
	ManaCostPercentage       int64        `json:"manaCostPercentage"`
	RuneCostID               int64        `json:"runeCostId"`
	Speed                    float32      `json:"speed"`
	MaxAffectedTargets       int64        `json:"maxAffectedTargets"`
	MaxTargetLevel           int64        `json:"maxTargetLevel"`
	RangeMax                 float32      `json:"rangeMax"`
	EquippedItemClass        int64        `json:"equippedItemClass"`
	EquippedItemSubClassMask int64        `json:"equippedItemSubClassMask"`
	Positive                 bool         `json:"positive"`
	Passive                  bool         `json:"passive"`
	Channeled                bool         `json:"channeled"`
	CritCapable              bool         `json:"critCapable"`
	AffectingArea            bool         `json:"affectingArea"`
	NeedsComboPoints         bool         `json:"needsComboPoints"`
	Binary                   bool         `json:"binary"`
	Effects                  []DumpEffect `json:"effects"`
	Bonus                    *DumpBonus   `json:"bonus"`
	ProcEntry                *DumpProc    `json:"procEntry"`
	Scripts                  []string     `json:"scripts"`
}

type DumpEffect struct {
	Index               int64    `json:"index"`
	Effect              int64    `json:"effect"`
	Aura                int64    `json:"aura"`
	AmplitudeMs         int64    `json:"amplitudeMs"`
	BasePoints          int64    `json:"basePoints"`
	DieSides            int64    `json:"dieSides"`
	RealPointsPerLevel  float32  `json:"realPointsPerLevel"`
	PointsPerComboPoint float32  `json:"pointsPerComboPoint"`
	ValueMultiplier     float32  `json:"valueMultiplier"`
	DamageMultiplier    float32  `json:"damageMultiplier"`
	BonusMultiplier     float32  `json:"bonusMultiplier"`
	MiscValue           int64    `json:"miscValue"`
	MiscValueB          int64    `json:"miscValueB"`
	Mechanic            int64    `json:"mechanic"`
	TargetA             int64    `json:"targetA"`
	TargetB             int64    `json:"targetB"`
	RadiusMax           float32  `json:"radiusMax"`
	ChainTarget         int64    `json:"chainTarget"`
	ItemType            int64    `json:"itemType"`
	TriggerSpell        int64    `json:"triggerSpell"`
	ClassMask           [3]int64 `json:"classMask"`
}

// DumpBonus is SpellMgr::GetSpellBonusData: the spell's spell_bonus_data row, else its first rank's.
type DumpBonus struct {
	Direct float32 `json:"direct"`
	Dot    float32 `json:"dot"`
	AP     float32 `json:"ap"`
	APDot  float32 `json:"apDot"`
}

// DumpProc is SpellMgr::GetSpellProcEntry: a spell_proc row (with DBC defaults filled in), or the
// entry the server generates for proc auras that have none.
type DumpProc struct {
	SchoolMask         int64    `json:"schoolMask"`
	SpellFamilyName    int64    `json:"spellFamilyName"`
	SpellFamilyMask    [3]int64 `json:"spellFamilyMask"`
	ProcFlags          int64    `json:"procFlags"`
	SpellTypeMask      int64    `json:"spellTypeMask"`
	SpellPhaseMask     int64    `json:"spellPhaseMask"`
	HitMask            int64    `json:"hitMask"`
	AttributesMask     int64    `json:"attributesMask"`
	DisableEffectsMask int64    `json:"disableEffectsMask"`
	ProcsPerMinute     float32  `json:"procsPerMinute"`
	Chance             float32  `json:"chance"`
	CooldownMs         int64    `json:"cooldownMs"`
	Charges            int64    `json:"charges"`
}

// Dump is a spell capture keyed by spell id.
type Dump map[int32]*DumpSpell

// ReadDump reads a spelldump JSONL file. A missing file reads as an empty dump, so the id collection
// works before the first capture.
func ReadDump(path string) (Dump, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Dump{}, nil
	}
	if err != nil {
		return nil, err
	}

	dump := Dump{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(nil, 1<<24)
	for line := 1; scanner.Scan(); line++ {
		text := bytes.TrimSpace(scanner.Bytes())
		if len(text) == 0 {
			continue
		}
		spell := &DumpSpell{}
		if err := json.Unmarshal(text, spell); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if _, dup := dump[int32(spell.ID)]; dup {
			return nil, fmt.Errorf("%s:%d: spell %d listed twice", path, line, spell.ID)
		}
		dump[int32(spell.ID)] = spell
	}
	return dump, scanner.Err()
}

// IDs returns the dump's spell ids in ascending order.
func (d Dump) IDs() []int32 {
	ids := make([]int32, 0, len(d))
	for id := range d {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Triggers returns the spells a spell's effects trigger, learn or cast (EffectTriggerSpell). Some dummy
// auras hold uint32(-1) there, which isn't a spell.
func (s *DumpSpell) Triggers() []int32 {
	var ids []int32
	for _, e := range s.Effects {
		if e.TriggerSpell > 0 && e.TriggerSpell <= math.MaxInt32 {
			ids = append(ids, int32(e.TriggerSpell))
		}
	}
	return ids
}

// Closure adds every spell the seeds trigger, directly or through other triggered spells, as far as the
// dump knows them. It returns the added ids and the ids (seeds or triggered) the dump lacks.
func (d Dump) Closure(seeds map[int32]bool) (triggered map[int32]bool, missing []int32) {
	triggered = map[int32]bool{}
	seen := map[int32]bool{}
	queue := make([]int32, 0, len(seeds))
	for id := range seeds {
		queue = append(queue, id)
		seen[id] = true
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		spell := d[id]
		if spell == nil {
			missing = append(missing, id)
			continue
		}
		for _, t := range spell.Triggers() {
			if seen[t] {
				continue
			}
			seen[t] = true
			triggered[t] = true
			queue = append(queue, t)
		}
	}
	slices.Sort(missing)
	return triggered, missing
}

// GeneratedSet is what gen_serverdata writes tables for: the sim's spell literals plus what they trigger.
// ids holds both kinds, triggered only the second; missing lists the ids of either kind the dump lacks.
func (d Dump) GeneratedSet(lits *Literals) (ids, triggered map[int32]bool, missing []int32) {
	ids = make(map[int32]bool, len(lits.Spells))
	for id := range lits.Spells {
		ids[id] = true
	}
	triggered, missing = d.Closure(ids)
	maps.Copy(ids, triggered)
	return ids, triggered, missing
}
