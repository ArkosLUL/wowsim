// Package azerothcore reads items, spells and characters from an AzerothCore (3.3.5a) server: its
// world and character databases, and the client DBC files the worldserver loads. It only ever
// SELECTs from the databases.
package azerothcore

import (
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// DBCFile is a raw WDBC table. Every DBC used here has 4-byte fields only.
type DBCFile struct {
	RecordCount int
	FieldCount  int
	records     []byte
	strings     []byte
}

// ParseDBC splits a WDBC file into its records and string block. Files with fields other than 4
// bytes wide are rejected.
func ParseDBC(data []byte) (*DBCFile, error) {
	if len(data) < 20 || string(data[:4]) != "WDBC" {
		return nil, fmt.Errorf("not a WDBC file")
	}
	recordCount := int(binary.LittleEndian.Uint32(data[4:]))
	fieldCount := int(binary.LittleEndian.Uint32(data[8:]))
	recordSize := int(binary.LittleEndian.Uint32(data[12:]))
	stringSize := int(binary.LittleEndian.Uint32(data[16:]))

	if recordSize != fieldCount*4 {
		return nil, fmt.Errorf("record size %d is not %d fields of 4 bytes", recordSize, fieldCount)
	}
	recordsEnd := 20 + recordCount*recordSize
	if len(data) < recordsEnd+stringSize {
		return nil, fmt.Errorf("file truncated: need %d bytes, have %d", recordsEnd+stringSize, len(data))
	}

	return &DBCFile{
		RecordCount: recordCount,
		FieldCount:  fieldCount,
		records:     data[20:recordsEnd],
		strings:     data[recordsEnd : recordsEnd+stringSize],
	}, nil
}

// Uint32 reads a field by its DBCStructure.h index. field isn't checked against FieldCount.
func (f *DBCFile) Uint32(row, field int) uint32 {
	offset := (row*f.FieldCount + field) * 4
	return binary.LittleEndian.Uint32(f.records[offset:])
}

func (f *DBCFile) Int32(row, field int) int32 {
	return int32(f.Uint32(row, field))
}

// String reads a field holding a string block offset. An offset past the block reads as "".
func (f *DBCFile) String(row, field int) string {
	offset := int(f.Uint32(row, field))
	if offset >= len(f.strings) {
		return ""
	}
	end := offset
	for end < len(f.strings) && f.strings[end] != 0 {
		end++
	}
	return string(f.strings[offset:end])
}

// Field indices follow AzerothCore's DBCStructure.h. Localized strings start with enUS.
const (
	spellFieldID                   = 0
	spellFieldStances              = 12
	spellFieldRecoveryTime         = 29
	spellFieldCategoryRecoveryTime = 30
	spellFieldProcChance           = 35
	spellFieldDurationIndex        = 40
	spellFieldEffect               = 71
	spellFieldEffectDieSides       = 74
	spellFieldEffectBasePoints     = 80
	spellFieldEffectApplyAuraName  = 95
	spellFieldEffectItemType       = 107
	spellFieldEffectMiscValue      = 110
	spellFieldEffectTriggerSpell   = 116
	spellFieldName                 = 136
	spellFieldDescription          = 170
	spellFieldAuraDescription      = 187

	enchantFieldID     = 0
	enchantFieldType   = 2
	enchantFieldAmount = 5
	enchantFieldArg    = 11
	enchantFieldName   = 14

	itemSetFieldID        = 0
	itemSetFieldName      = 1
	itemSetFieldItemID    = 18
	itemSetItemCount      = 10
	itemSetFieldSpell     = 35
	itemSetFieldThreshold = 43
	itemSetSpellCount     = 8

	gemFieldID      = 0
	gemFieldEnchant = 1
	gemFieldColor   = 4

	durationFieldID       = 0
	durationFieldDuration = 1
)

// SpellEntry holds the Spell.dbc columns this package reads. ApplySpellDBCOverrides can swap in a
// spell_dbc row.
type SpellEntry struct {
	ID                   int32
	Stances              uint32 // shapeshift forms it needs, bit n-1 for form n; 0 for any
	RecoveryTime         int32  // ms
	CategoryRecoveryTime int32  // ms
	ProcChance           int32
	DurationIndex        int32

	Effect              [3]int32
	EffectDieSides      [3]int32
	EffectBasePoints    [3]int32
	EffectApplyAuraName [3]int32
	EffectItemType      [3]int32
	EffectMiscValue     [3]int32
	EffectTriggerSpell  [3]int32

	Name            string
	Description     string
	AuraDescription string
}

// EffectValue is the flat amount an effect applies, matching AzerothCore's
// SpellEffectInfo::CalcValue for die sides 0 or 1. Random ranges return their max.
func (s *SpellEntry) EffectValue(i int) int32 {
	return s.EffectBasePoints[i] + s.EffectDieSides[i]
}

// CooldownMs is how long until the spell can be cast again: its own and its category's cooldown
// both run, so the longer one wins.
func (s *SpellEntry) CooldownMs() int32 {
	return max(s.RecoveryTime, s.CategoryRecoveryTime)
}

type SpellItemEnchantmentEntry struct {
	ID     int32
	Type   [3]int32
	Amount [3]int32
	Arg    [3]int32
	Name   string
}

// ItemSetEntry lists the set's bonuses in order: Spells[i] needs Thresholds[i] pieces.
type ItemSetEntry struct {
	ID         int32
	Name       string
	ItemIDs    []int32
	Spells     []int32
	Thresholds []int32
}

type GemPropertiesEntry struct {
	ID        int32
	EnchantID int32
	Color     int32
}

// DBC holds the client tables needed to interpret item_template rows.
type DBC struct {
	Spells         map[int32]*SpellEntry
	SpellDurations map[int32]int32 // duration index -> milliseconds
	Enchantments   map[int32]*SpellItemEnchantmentEntry
	ItemSets       map[int32]*ItemSetEntry
	GemProperties  map[int32]*GemPropertiesEntry
}

// DBCFileNames are the files LoadDBC reads.
var DBCFileNames = []string{"Spell.dbc", "SpellDuration.dbc", "SpellItemEnchantment.dbc", "ItemSet.dbc", "GemProperties.dbc"}

// CopyDBCFromContainer copies the needed DBCs out of a running worldserver container, since the
// client data lives in a Docker volume the host can't read directly.
func CopyDBCFromContainer(container, destDir string) error {
	return CopyDBCFilesFromContainer(container, destDir, DBCFileNames)
}

// CopyDBCFilesFromContainer shells out to docker cp, so it works from the host but not inside
// tools/acore/dock.sh's container, which has no docker CLI.
func CopyDBCFilesFromContainer(container, destDir string, names []string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	for _, name := range names {
		src := fmt.Sprintf("%s:/azerothcore/env/dist/data/dbc/%s", container, name)
		if out, err := exec.Command("docker", "cp", src, filepath.Join(destDir, name)).CombinedOutput(); err != nil {
			return fmt.Errorf("docker cp %s: %v: %s", src, err, out)
		}
	}
	return nil
}

func readDBCFiles(dir string, names []string) (map[string]*DBCFile, error) {
	files := make(map[string]*DBCFile, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		file, err := ParseDBC(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		files[name] = file
	}
	return files, nil
}

// LoadDBC reads DBCFileNames from dir. The worldserver loads acore_world.spell_dbc on top of
// Spell.dbc, so follow up with ApplySpellDBCOverrides to see the spells it sees.
func LoadDBC(dir string) (*DBC, error) {
	files, err := readDBCFiles(dir, DBCFileNames)
	if err != nil {
		return nil, err
	}
	return dbcFromFiles(files), nil
}

// dbcFromFiles reads DBCFileNames out of files, which can hold more.
func dbcFromFiles(files map[string]*DBCFile) *DBC {
	return &DBC{
		Spells:         readSpells(files["Spell.dbc"]),
		SpellDurations: readSpellDurations(files["SpellDuration.dbc"]),
		Enchantments:   readEnchantments(files["SpellItemEnchantment.dbc"]),
		ItemSets:       readItemSets(files["ItemSet.dbc"]),
		GemProperties:  readGemProperties(files["GemProperties.dbc"]),
	}
}

func readSpells(f *DBCFile) map[int32]*SpellEntry {
	spells := make(map[int32]*SpellEntry, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		spell := &SpellEntry{
			ID:                   f.Int32(row, spellFieldID),
			Stances:              f.Uint32(row, spellFieldStances),
			RecoveryTime:         f.Int32(row, spellFieldRecoveryTime),
			CategoryRecoveryTime: f.Int32(row, spellFieldCategoryRecoveryTime),
			ProcChance:           f.Int32(row, spellFieldProcChance),
			DurationIndex:        f.Int32(row, spellFieldDurationIndex),
			Name:                 f.String(row, spellFieldName),
			Description:          f.String(row, spellFieldDescription),
			AuraDescription:      f.String(row, spellFieldAuraDescription),
		}
		for i := 0; i < 3; i++ {
			spell.Effect[i] = f.Int32(row, spellFieldEffect+i)
			spell.EffectDieSides[i] = f.Int32(row, spellFieldEffectDieSides+i)
			spell.EffectBasePoints[i] = f.Int32(row, spellFieldEffectBasePoints+i)
			spell.EffectApplyAuraName[i] = f.Int32(row, spellFieldEffectApplyAuraName+i)
			spell.EffectItemType[i] = f.Int32(row, spellFieldEffectItemType+i)
			spell.EffectMiscValue[i] = f.Int32(row, spellFieldEffectMiscValue+i)
			spell.EffectTriggerSpell[i] = f.Int32(row, spellFieldEffectTriggerSpell+i)
		}
		spells[spell.ID] = spell
	}
	return spells
}

func readSpellDurations(f *DBCFile) map[int32]int32 {
	durations := make(map[int32]int32, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		durations[f.Int32(row, durationFieldID)] = f.Int32(row, durationFieldDuration)
	}
	return durations
}

func readEnchantments(f *DBCFile) map[int32]*SpellItemEnchantmentEntry {
	enchants := make(map[int32]*SpellItemEnchantmentEntry, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		enchant := &SpellItemEnchantmentEntry{
			ID:   f.Int32(row, enchantFieldID),
			Name: f.String(row, enchantFieldName),
		}
		for i := 0; i < 3; i++ {
			enchant.Type[i] = f.Int32(row, enchantFieldType+i)
			enchant.Amount[i] = f.Int32(row, enchantFieldAmount+i)
			enchant.Arg[i] = f.Int32(row, enchantFieldArg+i)
		}
		enchants[enchant.ID] = enchant
	}
	return enchants
}

func readItemSets(f *DBCFile) map[int32]*ItemSetEntry {
	sets := make(map[int32]*ItemSetEntry, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		set := &ItemSetEntry{
			ID:   f.Int32(row, itemSetFieldID),
			Name: f.String(row, itemSetFieldName),
		}
		for i := 0; i < itemSetItemCount; i++ {
			if id := f.Int32(row, itemSetFieldItemID+i); id != 0 {
				set.ItemIDs = append(set.ItemIDs, id)
			}
		}
		for i := 0; i < itemSetSpellCount; i++ {
			if spell := f.Int32(row, itemSetFieldSpell+i); spell != 0 {
				set.Spells = append(set.Spells, spell)
				set.Thresholds = append(set.Thresholds, f.Int32(row, itemSetFieldThreshold+i))
			}
		}
		sets[set.ID] = set
	}
	return sets
}

func readGemProperties(f *DBCFile) map[int32]*GemPropertiesEntry {
	gems := make(map[int32]*GemPropertiesEntry, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		gem := &GemPropertiesEntry{
			ID:        f.Int32(row, gemFieldID),
			EnchantID: f.Int32(row, gemFieldEnchant),
			Color:     f.Int32(row, gemFieldColor),
		}
		gems[gem.ID] = gem
	}
	return gems
}

// SpellDurationSeconds returns 0 for spells without a duration or with an unknown index.
func (d *DBC) SpellDurationSeconds(spell *SpellEntry) float64 {
	ms := d.SpellDurations[spell.DurationIndex]
	if ms <= 0 {
		return 0
	}
	return float64(ms) / 1000
}
