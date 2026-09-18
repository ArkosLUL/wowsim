package azerothcore

import "fmt"

// Field indices follow AzerothCore's DBCStructure.h.
const (
	talentFieldTabID  = 1
	talentFieldRow    = 2
	talentFieldCol    = 3
	talentFieldRankID = 4
	talentRankCount   = 5

	talentTabFieldID        = 0
	talentTabFieldClassMask = 20
	talentTabFieldTabPage   = 22

	glyphFieldID        = 0
	glyphFieldSpellID   = 1
	glyphFieldSlotFlags = 2
	glyphSlotFlagMinor  = 1

	enchantFieldSrcItemID = 33
)

// RosterDBC holds the client tables needed to read a character's talents, glyphs and gems.
type RosterDBC struct {
	TalentRanks map[int32]TalentRank      // talent rank spell -> where it sits in its tree
	TalentTabs  map[int32]TalentTabEntry  // Talent.TabID -> tree
	Glyphs      map[int32]GlyphPropsEntry // character_glyphs id -> glyph
	GemItems    map[int32]int32           // gem enchant -> gem item
}

type TalentRank struct {
	TabID int32
	Row   int32
	Col   int32
	Rank  int32 // 1-based
}

type TalentTabEntry struct {
	ClassMask int32
	TabPage   int32 // tree index within the class
}

type GlyphPropsEntry struct {
	SpellID int32
	Minor   bool
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

	return &RosterDBC{
		TalentRanks: readTalentRanks(files["Talent.dbc"]),
		TalentTabs:  readTalentTabs(files["TalentTab.dbc"]),
		Glyphs:      readGlyphProperties(files["GlyphProperties.dbc"]),
		GemItems:    readGemItems(files["SpellItemEnchantment.dbc"]),
	}, nil
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
			ClassMask: f.Int32(row, talentTabFieldClassMask),
			TabPage:   f.Int32(row, talentTabFieldTabPage),
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
