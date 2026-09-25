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

	petTab[talentTabFieldPetTalentMask] = 2 // tenacity

	tabs := readTalentTabs(mustParse(t, buildDBC(24, [][]uint32{retribution, petTab}, "")))
	if got := tabs[383]; got != (TalentTabEntry{ClassMask: 2, TabPage: 2}) {
		t.Errorf("paladin tab = %+v", got)
	}
	if got := tabs[409]; got != (TalentTabEntry{PetTalentMask: 2}) {
		t.Errorf("pet tab = %+v, want no class and the tenacity mask", got)
	}
}

func TestReadPetTalents(t *testing.T) {
	talent := func(tab, row, col uint32, ranks ...uint32) []uint32 {
		record := make([]uint32, 23)
		record[talentFieldTabID], record[talentFieldRow], record[talentFieldCol] = tab, row, col
		copy(record[talentFieldRankID:], ranks)
		return record
	}
	tabs := map[int32]TalentTabEntry{409: {PetTalentMask: 2}, 410: {PetTalentMask: 1}, 382: {ClassMask: 2}}
	// Cobra Reflexes sits at the top left of both pet trees, Charge at a different place in each
	file := mustParse(t, buildDBC(23, [][]uint32{
		talent(409, 0, 0, 61682, 61683), talent(410, 0, 0, 61682, 61683),
		talent(409, 0, 1, 61685), talent(410, 2, 3, 61685),
		talent(382, 0, 2, 20224),
	}, ""))

	talents := readPetTalents(file, tabs)
	if got := talents[61683]; len(got) != 2 || got[0] != (TalentRank{TabID: 409, Rank: 2}) || got[1] != (TalentRank{TabID: 410, Rank: 2}) {
		t.Errorf("cobra reflexes rank 2 = %+v", got)
	}
	if got := talents[61685]; len(got) != 2 || got[1] != (TalentRank{TabID: 410, Row: 2, Col: 3, Rank: 1}) {
		t.Errorf("charge = %+v", got)
	}
	if _, ok := talents[20224]; ok {
		t.Error("a class talent was read as a pet talent")
	}
}

func TestReadCreatureFamilies(t *testing.T) {
	worm := make([]uint32, 28)
	worm[familyFieldID], worm[familyFieldPetTalentType], worm[familyFieldName] = 42, 1, 1
	imp := make([]uint32, 28)
	imp[familyFieldID], imp[familyFieldPetTalentType], imp[familyFieldName] = 23, 0xFFFFFFFF, 6

	families := readCreatureFamilies(mustParse(t, buildDBC(28, [][]uint32{worm, imp}, "\x00Worm\x00Imp\x00")))
	if got := families[42]; got != (CreatureFamilyEntry{Name: "Worm", PetTalentType: 1}) {
		t.Errorf("worm = %+v", got)
	}
	if got := families[23]; got != (CreatureFamilyEntry{Name: "Imp", PetTalentType: -1}) {
		t.Errorf("imp = %+v", got)
	}
}

func TestReadConsumableSpells(t *testing.T) {
	spell := func(id, effect, trigger, name uint32) []uint32 {
		record := make([]uint32, spellFieldName+1)
		record[spellFieldID], record[spellFieldEffect+1], record[spellFieldEffectTriggerSpell+2] = id, effect, trigger
		record[spellFieldName] = name
		return record
	}
	const applyAura = 6
	file := mustParse(t, buildDBC(spellFieldName+1, [][]uint32{
		spell(43186, spellEffectEnergize, 0, 1), // Runic Mana Potion's spell
		spell(57371, applyAura, 0, 14),          // Dragonfin Filet's Well Fed
		spell(57370, applyAura, 57371, 23),      // its Refreshment, which triggers it
		spell(53760, applyAura, 0, 1),
	}, "\x00Restore Mana\x00Well Fed\x00Refreshment\x00"))

	mana, wellFed, foodBuffs := readConsumableSpells(file)
	if !mana[43186] || mana[53760] {
		t.Errorf("mana spells = %v", mana)
	}
	if !wellFed[57371] || len(wellFed) != 1 {
		t.Errorf("Well Fed auras = %v", wellFed)
	}
	if foodBuffs[57370] != 57371 || len(foodBuffs) != 1 {
		t.Errorf("food buffs = %v", foodBuffs)
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
