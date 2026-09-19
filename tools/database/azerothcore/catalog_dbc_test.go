package azerothcore

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
)

func TestReadLimitCategories(t *testing.T) {
	record := make([]uint32, 20)
	record[limitCategoryFieldID] = 2
	record[limitCategoryFieldName] = 1
	record[limitCategoryFieldMaxCount] = 3
	record[limitCategoryFieldMode] = 1

	got := readLimitCategories(mustParse(t, buildDBC(20, [][]uint32{record}, "\x00Jeweler's Gems\x00")))
	if got[2] != (LimitCategoryRow{ID: 2, Name: "Jeweler's Gems", MaxCount: 3, Mode: 1}) {
		t.Errorf("limit category = %+v", got[2])
	}
}

func TestReadExtendedCosts(t *testing.T) {
	record := make([]uint32, 16)
	record[extendedCostFieldID] = 2795
	record[extendedCostFieldHonorPoints] = 0
	record[extendedCostFieldItem] = 52025
	record[extendedCostFieldItem+2] = 49426 // a gap between items
	record[extendedCostFieldArenaRating] = 1800
	pvp := make([]uint32, 16)
	pvp[extendedCostFieldID] = 2
	pvp[extendedCostFieldHonorPoints] = 1000
	pvp[extendedCostFieldArenaPoints] = 50

	got := readExtendedCosts(mustParse(t, buildDBC(16, [][]uint32{record, pvp}, "")))
	if c := got[2795]; len(c.Items) != 2 || c.Items[0] != 52025 || c.Items[1] != 49426 || c.ArenaRating != 1800 {
		t.Errorf("token cost = %+v", c)
	}
	if c := got[2]; c.HonorPoints != 1000 || c.ArenaPoints != 50 || len(c.Items) != 0 {
		t.Errorf("honor cost = %+v", c)
	}
}

func TestReadEncountersMapsAndDifficulties(t *testing.T) {
	encounter := make([]uint32, 23)
	encounter[0] = 855
	encounter[encounterFieldMap] = 631
	encounter[encounterFieldDifficulty] = 3
	if got := readEncounters(mustParse(t, buildDBC(23, [][]uint32{encounter}, "")))[855]; got != (EncounterRow{Map: 631, Difficulty: 3}) {
		t.Errorf("encounter = %+v", got)
	}

	m := make([]uint32, 66)
	m[mapFieldID] = 603
	m[mapFieldType] = mapTypeRaid
	m[mapFieldName] = 1
	m[mapFieldExpansion] = 2
	if got := readMaps(mustParse(t, buildDBC(66, [][]uint32{m}, "\x00Ulduar\x00")))[603]; got != (CatalogMapRow{ID: 603, Type: mapTypeRaid, Name: "Ulduar", Expansion: 2}) {
		t.Errorf("map = %+v", got)
	}

	d := make([]uint32, 23)
	d[0] = 754
	d[mapDifficultyFieldMap] = 533
	d[mapDifficultyFieldDifficulty] = 2
	d[mapDifficultyFieldMaxPlayers] = 40
	if got := readMapDifficulties(mustParse(t, buildDBC(23, [][]uint32{d}, "")))[754]; got != (MapDifficultyRow{Map: 533, Difficulty: 2, MaxPlayers: 40}) {
		t.Errorf("map difficulty = %+v", got)
	}
}

func TestReadSkillAbilities(t *testing.T) {
	jewelcrafting := make([]uint32, 14)
	jewelcrafting[skillAbilityFieldSkillLine] = 755
	jewelcrafting[skillAbilityFieldSpell] = 66447
	cooking := make([]uint32, 14)
	cooking[skillAbilityFieldSkillLine] = 185
	cooking[skillAbilityFieldSpell] = 45554

	got := readSkillAbilities(mustParse(t, buildDBC(14, [][]uint32{jewelcrafting, cooking}, "")))
	if createSpellProfession(got, 66447) != proto.Profession_Jewelcrafting {
		t.Errorf("jewelcrafting spell = %v", got[66447])
	}
	if _, ok := got[45554]; ok {
		t.Error("cooking isn't a primary profession")
	}
}

func TestReadCreateSpells(t *testing.T) {
	cut := make([]uint32, 234)
	cut[spellFieldID] = 66447
	cut[spellFieldName] = 1
	cut[spellFieldEffect] = SpellEffectCreateItem
	cut[spellFieldEffectItemType] = 40111
	cut[spellFieldReagent] = 36919
	cut[spellFieldReagentCount] = 1
	cut[spellFieldReagent+1] = 99999 // a reagent slot with no count
	randomLoot := make([]uint32, 234)
	randomLoot[spellFieldID] = 2
	randomLoot[spellFieldEffect+1] = spellEffectCreateRandomItem
	aura := make([]uint32, 234)
	aura[spellFieldID] = 3
	aura[spellFieldEffect] = SpellEffectApplyAura

	got := readCreateSpells(mustParse(t, buildDBC(234, [][]uint32{cut, randomLoot, aura}, "\x00Bold Cardinal Ruby\x00")))
	s := got[66447]
	if s.Name != "Bold Cardinal Ruby" || len(s.Creates) != 1 || s.Creates[0] != 40111 || len(s.Reagents) != 1 || s.Reagents[0] != 36919 {
		t.Errorf("cut = %+v", s)
	}
	if !got[2].SpellLoot || len(got[2].Creates) != 0 {
		t.Errorf("random item spell = %+v", got[2])
	}
	if _, ok := got[3]; ok {
		t.Error("kept a spell that creates nothing")
	}
}

func TestReadAchievementsAreasAndTaxiPaths(t *testing.T) {
	achievement := make([]uint32, 62)
	achievement[achievementFieldID] = 4788
	achievement[achievementFieldMap] = uint32(0xFFFFFFFF)
	achievement[achievementFieldName] = 1
	if got := readAchievements(mustParse(t, buildDBC(62, [][]uint32{achievement}, "\x00The Argent Champion\x00")))[4788]; got.Map != -1 || got.Name != "The Argent Champion" {
		t.Errorf("achievement = %+v", got)
	}

	area := make([]uint32, 36)
	area[areaFieldID] = 4395
	area[areaFieldMap] = 571
	if got := readAreas(mustParse(t, buildDBC(36, [][]uint32{area}, "")))[4395]; got != 571 {
		t.Errorf("area map = %d", got)
	}

	first := make([]uint32, 11)
	first[taxiNodeFieldPath] = 1814
	first[taxiNodeFieldMap] = 631
	second := make([]uint32, 11)
	second[taxiNodeFieldPath] = 1814
	second[taxiNodeFieldMap] = 0
	if got := readTaxiPathMaps(mustParse(t, buildDBC(11, [][]uint32{first, second}, "")))[1814]; got != 631 {
		t.Errorf("gunship path map = %d, want its first node's", got)
	}
}

func TestReadCatalogDBCRejectsShortFiles(t *testing.T) {
	files := map[string]*DBCFile{}
	for _, f := range catalogDBCFiles {
		files[f.name] = mustParse(t, buildDBC(f.lastField+1, nil, ""))
	}
	files["Spell.dbc"] = mustParse(t, buildDBC(234, nil, ""))
	if _, err := readCatalogDBC(files); err != nil {
		t.Fatalf("full-width files: %v", err)
	}
	files["Map.dbc"] = mustParse(t, buildDBC(mapFieldExpansion, nil, ""))
	if _, err := readCatalogDBC(files); err == nil {
		t.Error("accepted a Map.dbc too narrow for the expansion field")
	}
}
