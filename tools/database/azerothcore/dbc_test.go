package azerothcore

import (
	"encoding/binary"
	"testing"
)

func buildDBC(fieldCount int, records [][]uint32, stringBlock string) []byte {
	data := []byte("WDBC")
	for _, v := range []uint32{uint32(len(records)), uint32(fieldCount), uint32(fieldCount * 4), uint32(len(stringBlock))} {
		data = binary.LittleEndian.AppendUint32(data, v)
	}
	for _, record := range records {
		for _, v := range record {
			data = binary.LittleEndian.AppendUint32(data, v)
		}
	}
	return append(data, stringBlock...)
}

func TestParseDBC(t *testing.T) {
	// String block: offset 0 is the empty string, "Titanguard" starts at 1.
	data := buildDBC(3, [][]uint32{
		{45110, 1, uint32(0xFFFFFFFF)},
		{45111, 0, 7},
	}, "\x00Titanguard\x00")

	f, err := ParseDBC(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.RecordCount != 2 || f.FieldCount != 3 {
		t.Fatalf("got %d records x %d fields", f.RecordCount, f.FieldCount)
	}
	if got := f.Uint32(1, 0); got != 45111 {
		t.Errorf("Uint32(1, 0) = %d", got)
	}
	if got := f.Int32(0, 2); got != -1 {
		t.Errorf("Int32(0, 2) = %d", got)
	}
	if got := f.String(0, 1); got != "Titanguard" {
		t.Errorf("String(0, 1) = %q", got)
	}
	if got := f.String(1, 1); got != "" {
		t.Errorf("String(1, 1) = %q", got)
	}
}

func TestParseDBCRejectsBadInput(t *testing.T) {
	if _, err := ParseDBC([]byte("WDB2 not a dbc file")); err == nil {
		t.Error("accepted wrong magic")
	}
	truncated := buildDBC(2, [][]uint32{{1, 2}}, "")
	if _, err := ParseDBC(truncated[:len(truncated)-1]); err == nil {
		t.Error("accepted truncated file")
	}
}

func TestReadItemSets(t *testing.T) {
	record := make([]uint32, 53)
	record[itemSetFieldID] = 843
	record[itemSetFieldName] = 1
	record[itemSetFieldItemID] = 46131
	record[itemSetFieldItemID+1] = 46132
	record[itemSetFieldSpell] = 64928
	record[itemSetFieldThreshold] = 2
	record[itemSetFieldSpell+1] = 64929
	record[itemSetFieldThreshold+1] = 4

	sets := readItemSets(mustParse(t, buildDBC(53, [][]uint32{record}, "\x00Valorous Siegebreaker Battlegear\x00")))
	set := sets[843]
	if set == nil || set.Name != "Valorous Siegebreaker Battlegear" {
		t.Fatalf("set = %+v", set)
	}
	if len(set.ItemIDs) != 2 || len(set.Spells) != 2 || set.Thresholds[1] != 4 {
		t.Errorf("set = %+v", set)
	}
}

func TestReadSpellsStances(t *testing.T) {
	record := make([]uint32, 234)
	record[spellFieldID] = 44916
	record[spellFieldStances] = 0x40000091
	record[spellFieldEffect] = SpellEffectApplyAura
	record[spellFieldEffectApplyAuraName] = AuraModAttackPower
	record[spellFieldEffectBasePoints] = 1058

	spell := readSpells(mustParse(t, buildDBC(234, [][]uint32{record}, "\x00")))[44916]
	if spell == nil || spell.Stances != 0x40000091 || spell.EffectValue(0) != 1058 {
		t.Errorf("spell = %+v", spell)
	}
}

func TestSpellEffectValue(t *testing.T) {
	spell := &SpellEntry{EffectBasePoints: [3]int32{664, 9, -11}, EffectDieSides: [3]int32{1, 0, 1}}
	for i, want := range []int32{665, 9, -10} {
		if got := spell.EffectValue(i); got != want {
			t.Errorf("EffectValue(%d) = %d, want %d", i, got, want)
		}
	}
}

func TestReadScalingStatDistributions(t *testing.T) {
	record := make([]uint32, 22)
	record[ssdFieldID] = 5
	record[ssdFieldStatMod] = 7                    // stamina
	record[ssdFieldStatMod+1] = uint32(0xFFFFFFFF) // unused slot
	record[ssdFieldModifier] = 100
	record[ssdFieldMaxLevel] = 80

	ssd := readScalingStatDistributions(mustParse(t, buildDBC(22, [][]uint32{record}, "")))[5]
	if ssd == nil || ssd.StatMod[0] != 7 || ssd.StatMod[1] != -1 || ssd.Modifier[0] != 100 || ssd.MaxLevel != 80 {
		t.Errorf("ssd = %+v", ssd)
	}
}

func TestReadScalingStatValues(t *testing.T) {
	record := make([]uint32, 24)
	record[ssvFieldID] = 1
	record[ssvFieldLevel] = 80
	record[ssvFieldSSDMultiplier] = 5000
	record[ssvFieldArmorMod] = 200
	record[ssvFieldDPSMod] = 150
	record[ssvFieldSpellPower] = 50
	record[ssvFieldArmorMod2] = 90

	// LookupEntry indexes ScalingStatValues.dbc by Level, not by this row's own ID.
	ssv := readScalingStatValues(mustParse(t, buildDBC(24, [][]uint32{record}, "")))[80]
	if ssv == nil || ssv.ID != 1 || ssv.SSDMultiplierCols[0] != 5000 || ssv.ArmorModCols[0] != 200 ||
		ssv.DPSModCols[0] != 150 || ssv.SpellPower != 50 || ssv.ArmorMod2Cols[0] != 90 {
		t.Errorf("ssv = %+v", ssv)
	}
}

func TestScalingStatValuesGetters(t *testing.T) {
	ssv := &ScalingStatValuesEntry{
		SSDMultiplierCols: [4]int32{100, 200, 300, 400},
		ArmorModCols:      [4]int32{10, 20, 30, 40},
		ArmorMod2Cols:     [5]int32{1, 2, 3, 4, 5},
		DPSModCols:        [6]int32{5, 6, 7, 8, 9, 10},
		SpellPower:        50,
	}
	if got := ssv.SSDMultiplier(0x2); got != 200 { // trinket
		t.Errorf("SSDMultiplier(trinket) = %d, want 200", got)
	}
	if got := ssv.ArmorMod(0x100000); got != 2 { // cloth (armorMod2)
		t.Errorf("ArmorMod(cloth) = %d, want 2", got)
	}
	if got := ssv.DPSMod(0x1000); got != 8 { // caster 2H
		t.Errorf("DPSMod(caster 2H) = %d, want 8", got)
	}
	if !ssv.IsTwoHand(0x1000) || ssv.IsTwoHand(0x800) {
		t.Error("IsTwoHand should be true only for the 2H masks (0x400, 0x1000)")
	}
	if got := ssv.SpellBonus(0x8000); got != 50 {
		t.Errorf("SpellBonus(masked) = %d, want 50", got)
	}
	if got := ssv.SpellBonus(0x1); got != 0 {
		t.Errorf("SpellBonus(unmasked) = %d, want 0", got)
	}
}

func mustParse(t *testing.T, data []byte) *DBCFile {
	t.Helper()
	f, err := ParseDBC(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
