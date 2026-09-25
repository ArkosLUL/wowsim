package azerothcore

import "fmt"

// Field indices follow AzerothCore's DBCStructure.h.
const (
	talentFieldTabID  = 1
	talentFieldRow    = 2
	talentFieldCol    = 3
	talentFieldRankID = 4
	talentRankCount   = 5

	talentTabFieldID            = 0
	talentTabFieldClassMask     = 20
	talentTabFieldPetTalentMask = 21
	talentTabFieldTabPage       = 22

	glyphFieldID        = 0
	glyphFieldSpellID   = 1
	glyphFieldSlotFlags = 2
	glyphSlotFlagMinor  = 1

	enchantFieldSrcItemID = 33

	familyFieldID            = 0
	familyFieldPetTalentType = 8
	familyFieldName          = 10

	spellEffectEnergize = 30
)

// RosterDBC holds the client tables needed to read a character's talents, glyphs, gems, pet and
// consumables.
type RosterDBC struct {
	TalentRanks map[int32]TalentRank      // talent rank spell -> where it sits in its tree
	TalentTabs  map[int32]TalentTabEntry  // Talent.TabID -> tree
	Glyphs      map[int32]GlyphPropsEntry // character_glyphs id -> glyph
	GemItems    map[int32]int32           // gem enchant -> gem item
	// PetTalents lists every pet tree a rank spell sits in. The talents all three pet trees share,
	// like Cobra Reflexes, use the same spell in each, at a different place.
	PetTalents map[int32][]TalentRank
	Families   map[int32]CreatureFamilyEntry // creature_template.family -> family
	// ManaSpells have an energize effect, which is all mod-playerbots checks to call a potion a
	// mana potion. It doesn't look at the power type, so a rage potion counts too.
	ManaSpells map[int32]bool
	// WellFedAuras are the buffs food leaves, and FoodBuffs maps a food's own spell to the one it
	// leaves.
	WellFedAuras map[int32]bool
	FoodBuffs    map[int32]int32
}

type TalentRank struct {
	TabID int32
	Row   int32
	Col   int32
	Rank  int32 // 1-based
}

type TalentTabEntry struct {
	ClassMask     int32
	PetTalentMask int32 // 1 << CreatureFamily.PetTalentType on a pet tree, 0 on a class tree
	TabPage       int32 // tree index within the class
}

type GlyphPropsEntry struct {
	SpellID int32
	Minor   bool
}

type CreatureFamilyEntry struct {
	Name string
	// PetTalentType picks the hunter pet tree: 0 ferocity, 1 tenacity, 2 cunning. -1 for families
	// no hunter can tame, like demons.
	PetTalentType int32
}

// rosterDBCFiles are the files LoadRosterDBC needs, with the highest field index each is read at.
var rosterDBCFiles = []struct {
	name      string
	lastField int
}{
	{"Talent.dbc", talentFieldRankID + talentRankCount - 1},
	{"TalentTab.dbc", talentTabFieldTabPage},
	{"GlyphProperties.dbc", glyphFieldSlotFlags},
	{"SpellItemEnchantment.dbc", enchantFieldSrcItemID},
	{"CreatureFamily.dbc", familyFieldName},
	{"Spell.dbc", spellFieldName},
}

func RosterDBCFileNames() []string {
	names := make([]string, len(rosterDBCFiles))
	for i, file := range rosterDBCFiles {
		names[i] = file.name
	}
	return names
}

func LoadRosterDBC(dir string) (*RosterDBC, error) {
	files, err := readDBCFiles(dir, RosterDBCFileNames())
	if err != nil {
		return nil, err
	}
	// a DBC from another expansion parses fine but reads garbage out of the fields
	for _, file := range rosterDBCFiles {
		if fields := files[file.name].FieldCount; fields <= file.lastField {
			return nil, fmt.Errorf("%s has %d fields, need more than %d", file.name, fields, file.lastField)
		}
	}

	tabs := readTalentTabs(files["TalentTab.dbc"])
	dbc := &RosterDBC{
		TalentRanks: readTalentRanks(files["Talent.dbc"]),
		TalentTabs:  tabs,
		Glyphs:      readGlyphProperties(files["GlyphProperties.dbc"]),
		GemItems:    readGemItems(files["SpellItemEnchantment.dbc"]),
		PetTalents:  readPetTalents(files["Talent.dbc"], tabs),
		Families:    readCreatureFamilies(files["CreatureFamily.dbc"]),
	}
	dbc.ManaSpells, dbc.WellFedAuras, dbc.FoodBuffs = readConsumableSpells(files["Spell.dbc"])
	return dbc, nil
}

// readTalentRanks keys by rank spell, since that's what character_talent stores.
func readTalentRanks(f *DBCFile) map[int32]TalentRank {
	ranks := make(map[int32]TalentRank, f.RecordCount*2)
	for row := 0; row < f.RecordCount; row++ {
		for rank := 0; rank < talentRankCount; rank++ {
			spell := f.Int32(row, talentFieldRankID+rank)
			if spell == 0 {
				continue
			}
			ranks[spell] = TalentRank{
				TabID: f.Int32(row, talentFieldTabID),
				Row:   f.Int32(row, talentFieldRow),
				Col:   f.Int32(row, talentFieldCol),
				Rank:  int32(rank + 1),
			}
		}
	}
	return ranks
}

func readTalentTabs(f *DBCFile) map[int32]TalentTabEntry {
	tabs := make(map[int32]TalentTabEntry, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		tabs[f.Int32(row, talentTabFieldID)] = TalentTabEntry{
			ClassMask:     f.Int32(row, talentTabFieldClassMask),
			PetTalentMask: f.Int32(row, talentTabFieldPetTalentMask),
			TabPage:       f.Int32(row, talentTabFieldTabPage),
		}
	}
	return tabs
}

func readGlyphProperties(f *DBCFile) map[int32]GlyphPropsEntry {
	glyphs := make(map[int32]GlyphPropsEntry, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		glyphs[f.Int32(row, glyphFieldID)] = GlyphPropsEntry{
			SpellID: f.Int32(row, glyphFieldSpellID),
			Minor:   f.Int32(row, glyphFieldSlotFlags)&glyphSlotFlagMinor != 0,
		}
	}
	return glyphs
}

// readPetTalents keys by rank spell like readTalentRanks, but keeps every pet tree the spell is in.
func readPetTalents(f *DBCFile, tabs map[int32]TalentTabEntry) map[int32][]TalentRank {
	talents := map[int32][]TalentRank{}
	for row := 0; row < f.RecordCount; row++ {
		tabID := f.Int32(row, talentFieldTabID)
		if tabs[tabID].PetTalentMask == 0 {
			continue
		}
		for rank := 0; rank < talentRankCount; rank++ {
			if spell := f.Int32(row, talentFieldRankID+rank); spell != 0 {
				talents[spell] = append(talents[spell], TalentRank{TabID: tabID, Row: f.Int32(row, talentFieldRow),
					Col: f.Int32(row, talentFieldCol), Rank: int32(rank + 1)})
			}
		}
	}
	return talents
}

func readCreatureFamilies(f *DBCFile) map[int32]CreatureFamilyEntry {
	families := make(map[int32]CreatureFamilyEntry, f.RecordCount)
	for row := 0; row < f.RecordCount; row++ {
		families[f.Int32(row, familyFieldID)] = CreatureFamilyEntry{
			Name:          f.String(row, familyFieldName),
			PetTalentType: f.Int32(row, familyFieldPetTalentType),
		}
	}
	return families
}

// readConsumableSpells finds the mana potion spells and the food buffs in Spell.dbc. A food's own
// spell only triggers its Well Fed buff, so it's matched by the buff's name.
func readConsumableSpells(f *DBCFile) (mana, wellFed map[int32]bool, foodBuffs map[int32]int32) {
	mana, wellFed, foodBuffs = map[int32]bool{}, map[int32]bool{}, map[int32]int32{}
	for row := 0; row < f.RecordCount; row++ {
		id := f.Int32(row, spellFieldID)
		for i := 0; i < 3; i++ {
			if f.Int32(row, spellFieldEffect+i) == spellEffectEnergize {
				mana[id] = true
			}
		}
		if f.String(row, spellFieldName) == "Well Fed" {
			wellFed[id] = true
		}
	}
	for row := 0; row < f.RecordCount; row++ {
		for i := 0; i < 3; i++ {
			if trigger := f.Int32(row, spellFieldEffectTriggerSpell+i); wellFed[trigger] {
				foodBuffs[f.Int32(row, spellFieldID)] = trigger
			}
		}
	}
	return mana, wellFed, foodBuffs
}

// readGemItems maps a gem's enchant to the gem item, the only way back from a socketed enchant id.
func readGemItems(f *DBCFile) map[int32]int32 {
	items := make(map[int32]int32)
	for row := 0; row < f.RecordCount; row++ {
		if item := f.Int32(row, enchantFieldSrcItemID); item != 0 {
			items[f.Int32(row, enchantFieldID)] = item
		}
	}
	return items
}
