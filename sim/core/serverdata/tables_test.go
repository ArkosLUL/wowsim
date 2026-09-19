package serverdata

import "testing"

// Lookups binary search, so every table must be strictly ascending.
func TestTablesSortedAndFound(t *testing.T) {
	check := func(table string, n int, key func(int) int32, lookup func(int32) bool) {
		t.Helper()
		if n == 0 {
			t.Errorf("%s is empty", table)
		}
		for i := 0; i < n; i++ {
			if i > 0 && key(i) <= key(i-1) {
				t.Errorf("%s: %d follows %d", table, key(i), key(i-1))
			}
			if !lookup(key(i)) {
				t.Errorf("%s: lookup of %d failed", table, key(i))
			}
		}
	}
	check("spells", len(spells), func(i int) int32 { return spells[i].ID },
		func(id int32) bool { return SpellByID(id) != nil && SpellByID(id).ID == id })
	check("procs", len(procs), func(i int) int32 { return procs[i].SpellID },
		func(id int32) bool { return ProcBySpellID(id) != nil && ProcBySpellID(id).SpellID == id })
	check("bonuses", len(bonuses), func(i int) int32 { return bonuses[i].SpellID },
		func(id int32) bool { return BonusBySpellID(id) != nil && BonusBySpellID(id).SpellID == id })
	check("enchantProcs", len(enchantProcs), func(i int) int32 { return enchantProcs[i].EnchantID },
		func(id int32) bool { return EnchantProcByID(id) != nil && EnchantProcByID(id).EnchantID == id })

	for _, p := range procs {
		if SpellByID(p.SpellID) == nil {
			t.Errorf("proc entry for %d, which isn't in spells", p.SpellID)
		}
	}
	for _, b := range bonuses {
		if SpellByID(b.SpellID) == nil {
			t.Errorf("bonus entry for %d, which isn't in spells", b.SpellID)
		}
	}
	if SpellByID(1) != nil || SpellByID(-5) != nil || ProcBySpellID(0) != nil {
		t.Error("lookup of an id that isn't in the tables should return nil")
	}
}

// Values read off the capture, so a generator change that garbles a field shows up here.
func TestKnownSpells(t *testing.T) {
	spell := func(id int32) *Spell {
		t.Helper()
		s := SpellByID(id)
		if s == nil {
			t.Fatalf("spell %d missing", id)
		}
		return s
	}

	frostbolt := spell(42842)
	if frostbolt.DmgClass != DmgClassMagic || frostbolt.SchoolMask != 16 || frostbolt.CastMs != 3000 ||
		frostbolt.GCDMs != 1500 || frostbolt.GCDCategory != 133 || frostbolt.ManaCostPct != 11 {
		t.Errorf("Frostbolt: %+v", frostbolt)
	}
	if frostbolt.Flags != FlagResetsAutoAttack|FlagHasteGCD {
		t.Errorf("Frostbolt flags %b", frostbolt.Flags)
	}
	if e := frostbolt.Effects[1]; e.Effect != 2 || e.BasePoints != 798 || e.DieSides != 63 || e.BonusMultiplier != 0.857 {
		t.Errorf("Frostbolt damage effect: %+v", e)
	}
	if b := BonusBySpellID(42842); b == nil || b.Direct != 0.857 {
		t.Errorf("Frostbolt bonus: %+v", b)
	}

	// Steady Shot: ranged slot adds 500 ms to the 1.5 s cast; the server's corrections make it binary
	steady := spell(49052)
	if steady.DmgClass != DmgClassRanged || steady.CastMs != 2000 || steady.BaseCastMs != 1500 ||
		steady.Flags != FlagUsesRangedSlot|FlagBinary {
		t.Errorf("Steady Shot: %+v", steady)
	}

	mindFlay := spell(48156)
	if mindFlay.DurationMs != 3000 || mindFlay.Flags&(FlagChanneled|FlagHasteAffectsPeriodic) != FlagChanneled|FlagHasteAffectsPeriodic {
		t.Errorf("Mind Flay: %+v", mindFlay)
	}

	heroicStrike := spell(47450)
	if heroicStrike.DmgClass != DmgClassMelee || heroicStrike.PowerType != 1 || heroicStrike.ManaCost != 150 ||
		heroicStrike.Flags&(FlagHasteGCD|FlagResetsAutoAttack) != 0 {
		t.Errorf("Heroic Strike: %+v", heroicStrike)
	}

	corruption := spell(47813)
	if e := corruption.Effects[0]; corruption.DurationMs != 18000 || e.Aura != 3 || e.AmplitudeMs != 3000 {
		t.Errorf("Corruption: %+v", corruption)
	}

	// Seal of Command's own spell_proc row
	if p := ProcBySpellID(20375); p == nil || p.Row != 20375 || p.ProcFlags != 0x14 || p.Chance != 100 {
		t.Errorf("Seal of Command proc: %+v", p)
	}
	// Improved Blizzard rank 3 through the rank chain row of rank 1
	if p := ProcBySpellID(12488); p == nil || p.Row != -11185 {
		t.Errorf("Improved Blizzard proc: %+v", p)
	}
}
