package azerothcore

import "testing"

func TestReadTalentRanks(t *testing.T) {
	// Seals of the Pure: tab 382, row 0, col 2, 5 ranks.
	record := make([]uint32, 23)
	record[talentFieldTabID] = 382
	record[talentFieldRow] = 0
	record[talentFieldCol] = 2
	for rank, spell := range []uint32{20224, 20225, 20330, 20331, 20332} {
		record[talentFieldRankID+rank] = spell
	}
	unranked := make([]uint32, 23)
	unranked[talentFieldTabID] = 382
	unranked[talentFieldRow] = 1
	unranked[talentFieldRankID] = 20237

	ranks := readTalentRanks(mustParse(t, buildDBC(23, [][]uint32{record, unranked}, "")))
	if len(ranks) != 6 {
		t.Fatalf("got %d rank spells", len(ranks))
	}
	if got := ranks[20330]; got != (TalentRank{TabID: 382, Row: 0, Col: 2, Rank: 3}) {
		t.Errorf("rank 3 spell = %+v", got)
	}
	if got := ranks[20237]; got.Rank != 1 || got.Row != 1 {
		t.Errorf("single rank talent = %+v", got)
	}
	if _, ok := ranks[0]; ok {
		t.Error("empty rank slots were read as spells")
	}
}

func TestReadTalentTabs(t *testing.T) {
	retribution := make([]uint32, 24)
	retribution[talentTabFieldID] = 383
	retribution[talentTabFieldClassMask] = 1 << 1 // paladin
	retribution[talentTabFieldTabPage] = 2
	petTab := make([]uint32, 24)
	petTab[talentTabFieldID] = 409
	petTab[talentTabFieldTabPage] = 0

	tabs := readTalentTabs(mustParse(t, buildDBC(24, [][]uint32{retribution, petTab}, "")))
	if got := tabs[383]; got != (TalentTabEntry{ClassMask: 2, TabPage: 2}) {
		t.Errorf("paladin tab = %+v", got)
	}
	if got := tabs[409]; got.ClassMask != 0 {
		t.Errorf("pet tab = %+v, want no class", got)
	}
}

func TestReadGlyphProperties(t *testing.T) {
	major := []uint32{170, 54733, 0, 0}
	minor := []uint32{399, 58386, glyphSlotFlagMinor, 0}

	glyphs := readGlyphProperties(mustParse(t, buildDBC(4, [][]uint32{major, minor}, "")))
	if got := glyphs[170]; got != (GlyphPropsEntry{SpellID: 54733}) {
		t.Errorf("major glyph = %+v", got)
	}
	if got := glyphs[399]; got != (GlyphPropsEntry{SpellID: 58386, Minor: true}) {
		t.Errorf("minor glyph = %+v", got)
	}
}

func TestReadGemItems(t *testing.T) {
	gem := make([]uint32, 38)
	gem[enchantFieldID] = 3563
	gem[enchantFieldSrcItemID] = 40111
	notAGem := make([]uint32, 38)
	notAGem[enchantFieldID] = 3817

	items := readGemItems(mustParse(t, buildDBC(38, [][]uint32{gem, notAGem}, "")))
	if items[3563] != 40111 {
		t.Errorf("gem enchant = %d", items[3563])
	}
	if _, ok := items[3817]; ok {
		t.Error("enchant without a source item was read as a gem")
	}
}
