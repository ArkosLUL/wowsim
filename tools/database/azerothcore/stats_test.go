package azerothcore

import (
	"slices"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
)

func TestAddItemModDualStats(t *testing.T) {
	var s database.Stats
	AddItemMod(&s, 31, 30) // hit rating
	AddItemMod(&s, 32, 40) // crit rating
	AddItemMod(&s, 36, 50) // haste rating
	AddItemMod(&s, 38, 60) // attack power

	checks := map[proto.Stat]float64{
		proto.Stat_StatMeleeHit: 30, proto.Stat_StatSpellHit: 30,
		proto.Stat_StatMeleeCrit: 40, proto.Stat_StatSpellCrit: 40,
		proto.Stat_StatMeleeHaste: 50, proto.Stat_StatSpellHaste: 50,
		proto.Stat_StatAttackPower: 60, proto.Stat_StatRangedAttackPower: 60,
	}
	for stat, want := range checks {
		if s[stat] != want {
			t.Errorf("%s = %v, want %v", stat, s[stat], want)
		}
	}
}

func TestAddItemModSingleSchoolRatings(t *testing.T) {
	var s database.Stats
	AddItemMod(&s, 18, 12) // spell hit only
	AddItemMod(&s, 19, 14) // melee crit only
	AddItemMod(&s, 39, 20) // ranged attack power only

	if s[proto.Stat_StatSpellHit] != 12 || s[proto.Stat_StatMeleeHit] != 0 {
		t.Errorf("spell hit rating leaked: %v", s)
	}
	if s[proto.Stat_StatMeleeCrit] != 14 || s[proto.Stat_StatSpellCrit] != 0 {
		t.Errorf("melee crit rating leaked: %v", s)
	}
	if s[proto.Stat_StatRangedAttackPower] != 20 || s[proto.Stat_StatAttackPower] != 0 {
		t.Errorf("ranged attack power leaked: %v", s)
	}
}

func TestAddItemModUnmapped(t *testing.T) {
	var s database.Stats
	if AddItemMod(&s, 34, 10) { // crit taken rating
		t.Error("crit taken rating should be unmapped")
	}
	if s != (database.Stats{}) {
		t.Errorf("unmapped mod changed stats: %v", s)
	}
}

func applyAura(aura, misc, value int32) SpellEntry {
	return SpellEntry{
		Effect:              [3]int32{SpellEffectApplyAura},
		EffectApplyAuraName: [3]int32{aura},
		EffectMiscValue:     [3]int32{misc},
		EffectBasePoints:    [3]int32{value - 1},
		EffectDieSides:      [3]int32{1},
	}
}

func TestAddEquipSpellStatsSpellPower(t *testing.T) {
	spell := applyAura(AuraModDamageDone, allMagicSchools, 29)
	spell.Effect[1] = SpellEffectApplyAura
	spell.EffectApplyAuraName[1] = AuraModHealingDone
	spell.EffectBasePoints[1] = 28
	spell.EffectDieSides[1] = 1

	var s database.Stats
	if !AddEquipSpellStats(&s, &spell) {
		t.Fatal("spell power equip spell not resolved")
	}
	if s[proto.Stat_StatSpellPower] != 29 {
		t.Errorf("spell power = %v, want 29 (healing aura must not double it)", s[proto.Stat_StatSpellPower])
	}
}

func TestAddEquipSpellStatsAttackPower(t *testing.T) {
	spell := applyAura(AuraModAttackPower, 0, 20)
	spell.Effect[1] = SpellEffectApplyAura
	spell.EffectApplyAuraName[1] = AuraModRangedAttack
	spell.EffectBasePoints[1] = 19
	spell.EffectDieSides[1] = 1

	var s database.Stats
	if !AddEquipSpellStats(&s, &spell) {
		t.Fatal("attack power equip spell not resolved")
	}
	if s[proto.Stat_StatAttackPower] != 20 || s[proto.Stat_StatRangedAttackPower] != 20 {
		t.Errorf("melee+ranged attack power spell gave AP %v, RAP %v; want 20 each", s[proto.Stat_StatAttackPower], s[proto.Stat_StatRangedAttackPower])
	}
}

func TestAddEquipSpellStatsRatingMask(t *testing.T) {
	spell := applyAura(AuraModRating, 1<<crHitMelee|1<<crHitRanged, 15)
	var s database.Stats
	if !AddEquipSpellStats(&s, &spell) {
		t.Fatal("rating equip spell not resolved")
	}
	if s[proto.Stat_StatMeleeHit] != 15 || s[proto.Stat_StatSpellHit] != 0 {
		t.Errorf("melee+ranged hit mask gave %v", s)
	}

	resilience := applyAura(AuraModRating, 1<<crCritTakenMelee|1<<crCritTakenRanged|1<<crCritTakenSpell, 20)
	s = database.Stats{}
	AddEquipSpellStats(&s, &resilience)
	if s[proto.Stat_StatResilience] != 20 {
		t.Errorf("resilience = %v, want 20", s[proto.Stat_StatResilience])
	}
}

func TestAddEquipSpellStatsRejectsEffects(t *testing.T) {
	proc := applyAura(AuraProcTriggerSpell, 0, 0)
	proc.EffectTriggerSpell[0] = 65019
	fireOnly := applyAura(AuraModDamageDone, 4, 30)
	mixed := applyAura(AuraModStat, 2, 10)
	mixed.Effect[1] = SpellEffectApplyAura
	mixed.EffectApplyAuraName[1] = AuraProcTriggerSpell

	for name, spell := range map[string]SpellEntry{"proc": proc, "fire spell damage": fireOnly, "stat plus proc": mixed} {
		var s database.Stats
		if AddEquipSpellStats(&s, &spell) {
			t.Errorf("%s: resolved as flat stats", name)
		}
		if s != (database.Stats{}) {
			t.Errorf("%s: changed stats: %v", name, s)
		}
	}
}

func TestSocketAndGemColors(t *testing.T) {
	sockets := map[int32]proto.GemColor{1: proto.GemColor_GemColorMeta, 2: proto.GemColor_GemColorRed, 4: proto.GemColor_GemColorYellow, 8: proto.GemColor_GemColorBlue}
	for socket, want := range sockets {
		if got := SocketGemColor(socket); got != want {
			t.Errorf("SocketGemColor(%d) = %s, want %s", socket, got, want)
		}
	}
	gems := map[int32]proto.GemColor{6: proto.GemColor_GemColorOrange, 10: proto.GemColor_GemColorPurple, 12: proto.GemColor_GemColorGreen, 14: proto.GemColor_GemColorPrismatic}
	for mask, want := range gems {
		if got := GemPropertiesColor(mask); got != want {
			t.Errorf("GemPropertiesColor(%d) = %s, want %s", mask, got, want)
		}
	}
}

func TestEnchantmentStats(t *testing.T) {
	enchant := &SpellItemEnchantmentEntry{
		Type:   [3]int32{EnchantTypeStat, EnchantTypeResistance, EnchantTypeCombatSpell},
		Amount: [3]int32{20, 225, 0},
		Arg:    [3]int32{7, 0, 59620},
	}
	enchant.ID = 3789
	enchant.Amount[2] = 5
	stats, spells, _ := EnchantmentStats(enchant, &DBC{})
	if stats[proto.Stat_StatStamina] != 20 || stats[proto.Stat_StatBonusArmor] != 225 {
		t.Errorf("stats = %v", stats)
	}
	want := []EnchantSpell{{EnchantID: 3789, Type: EnchantTypeCombatSpell, SpellID: 59620, Amount: 5}}
	if !slices.Equal(spells, want) {
		t.Errorf("spells = %v, want %v", spells, want)
	}
}

func TestEnchantmentStatsEquipSpells(t *testing.T) {
	spellPen := applyAura(AuraModTargetResistance, allMagicSchools, -15)
	spellPen.ID = 1
	proc := applyAura(AuraProcTriggerSpell, 0, 0)
	proc.ID = 2
	dbc := &DBC{Spells: map[int32]*SpellEntry{1: &spellPen, 2: &proc}}

	enchant := &SpellItemEnchantmentEntry{
		Type: [3]int32{EnchantTypeEquipSpell, EnchantTypeEquipSpell},
		Arg:  [3]int32{1, 2},
	}
	stats, spells, _ := EnchantmentStats(enchant, dbc)
	if stats[proto.Stat_StatSpellPenetration] != 15 {
		t.Errorf("spell penetration = %v, want 15", stats[proto.Stat_StatSpellPenetration])
	}
	if len(spells) != 1 || spells[0].SpellID != 2 {
		t.Errorf("spells = %v, want only the proc spell", spells)
	}
}

func TestAddEquipSpellStatsTargetResistance(t *testing.T) {
	for _, misc := range []int32{allMagicSchools, spellPenetrationSchools} {
		spell := applyAura(AuraModTargetResistance, misc, -35)
		var s database.Stats
		if !AddEquipSpellStats(&s, &spell) || s[proto.Stat_StatSpellPenetration] != 35 {
			t.Errorf("misc %d: spell penetration = %v, want 35", misc, s[proto.Stat_StatSpellPenetration])
		}
	}

	ignoreArmor := applyAura(AuraModTargetResistance, 1, -436)
	var s database.Stats
	if AddEquipSpellStats(&s, &ignoreArmor) {
		t.Errorf("physical target resistance (ignore armor) resolved as flat stats: %v", s)
	}
}

func TestAllowedClasses(t *testing.T) {
	if got := AllowedClasses(-1); got != nil {
		t.Errorf("AllowedClasses(-1) = %v", got)
	}
	if got := AllowedClasses(1<<11 - 1); got != nil {
		t.Errorf("all classes mask = %v", got)
	}
	got := AllowedClasses(1<<(11-1) | 1<<(6-1)) // druid, death knight
	if !slices.Equal(got, []proto.Class{proto.Class_ClassDeathknight, proto.Class_ClassDruid}) {
		t.Errorf("druid+DK mask = %v", got)
	}
}
