package main

import (
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

func magicSpell() *spellset.DumpSpell {
	return &spellset.DumpSpell{
		ID: 42842, FirstRankID: 116, Name: "Frostbolt", Rank: "Rank 16", DmgClass: 1, SchoolMask: 16,
		CastTimeMs: 3000, CastTimeBaseMs: 3000, StartRecoveryTimeMs: 1500, StartRecoveryCategory: 133,
		InterruptFlags: 15, PowerType: 4294967294, ProcChance: 101,
		Effects: []spellset.DumpEffect{
			{Index: 0, Effect: 6, Aura: 33, BasePoints: -41, DieSides: 1},
			{Index: 1, Effect: 0, DamageMultiplier: 1},
			{Index: 2, Effect: 2, BasePoints: 798, DieSides: 63, BonusMultiplier: 0.857, TriggerSpell: 4294967295},
		},
	}
}

func TestBuildSpell(t *testing.T) {
	s := buildSpell(magicSpell())
	if s.name != "Frostbolt (Rank 16)" || s.PowerType != -2 || s.CastMs != 3000 || s.DmgClass != serverdata.DmgClassMagic {
		t.Errorf("%+v", s)
	}
	if s.Effects[1] != (serverdata.Effect{}) {
		t.Errorf("an unused effect slot must stay zero: %+v", s.Effects[1])
	}
	if e := s.Effects[2]; e.BasePoints != 798 || e.BonusMultiplier != 0.857 || e.TriggerSpell != -1 {
		t.Errorf("%+v", e)
	}
	if s.Flags != serverdata.FlagResetsAutoAttack|serverdata.FlagHasteGCD {
		t.Errorf("flags %b", s.Flags)
	}
}

func TestFlags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*spellset.DumpSpell)
		check serverdata.Flags
		want  bool
	}{
		{"melee GCD isn't hasted", func(d *spellset.DumpSpell) { d.DmgClass = 2 }, serverdata.FlagHasteGCD, false},
		{"abilities' GCD isn't hasted", func(d *spellset.DumpSpell) { d.Attributes[0] = attr0IsAbility }, serverdata.FlagHasteGCD, false},
		{"only a 1.5 s GCD is hasted", func(d *spellset.DumpSpell) { d.StartRecoveryTimeMs = 1000 }, serverdata.FlagHasteGCD, false},
		{"not interruptible, no swing reset", func(d *spellset.DumpSpell) { d.InterruptFlags = 0x7 }, serverdata.FlagResetsAutoAttack, false},
		{"DO_NOT_RESET_COMBAT_TIMERS", func(d *spellset.DumpSpell) { d.Attributes[2] = attr2DoNotResetCombatTimers }, serverdata.FlagResetsAutoAttack, false},
		{"instant with the attr6 exception", func(d *spellset.DumpSpell) {
			d.CastTimeMs = 0
			d.Attributes[6] = attr6DoesntResetSwingIfInstant
		}, serverdata.FlagResetsAutoAttack, false},
		{"the attr6 exception needs an instant", func(d *spellset.DumpSpell) { d.Attributes[6] = attr6DoesntResetSwingIfInstant }, serverdata.FlagResetsAutoAttack, true},
		{"ranged slot", func(d *spellset.DumpSpell) { d.Attributes[0] = attr0UsesRangedSlot }, serverdata.FlagUsesRangedSlot, true},
		{"binary", func(d *spellset.DumpSpell) { d.Binary = true }, serverdata.FlagBinary, true},
		{"no active defense", func(d *spellset.DumpSpell) { d.Attributes[0] = attr0NoActiveDefense }, serverdata.FlagNoActiveDefense, true},
		{"always hit", func(d *spellset.DumpSpell) { d.Attributes[3] = attr3AlwaysHit }, serverdata.FlagAlwaysHit, true},
		{"completely blocked", func(d *spellset.DumpSpell) { d.Attributes[3] = attr3CompletelyBlocked }, serverdata.FlagCompletelyBlocked, true},
		{"haste affects periodic", func(d *spellset.DumpSpell) { d.Attributes[5] = attr5SpellHasteAffectsPeriodic }, serverdata.FlagHasteAffectsPeriodic, true},
	} {
		d := magicSpell()
		tc.edit(d)
		if got := buildSpell(d).Flags&tc.check != 0; got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestResolveProc(t *testing.T) {
	rankRow := procRow{id: -100, Proc: serverdata.Proc{ProcFlags: 0x14, CooldownMs: 6000}}
	ownRow := procRow{id: 102, Proc: serverdata.Proc{ProcFlags: 0x4, Chance: 50}}
	rows := map[int32]procRow{-100: rankRow, 102: ownRow}

	rank3 := &spellset.DumpSpell{ID: 102, FirstRankID: 100, ProcChance: 2, ProcCharges: 1,
		ProcEntry: &spellset.DumpProc{ProcFlags: 0x14, CooldownMs: 6000, Chance: 2, Charges: 1}}
	p, err := resolveProc(rank3, rows)
	if err != nil {
		t.Fatal(err)
	}
	// the chain row wins over the rank's own row, and Spell.dbc fills chance and charges
	if p.Row != -100 || p.SpellID != 102 || p.Chance != 2 || p.Charges != 1 || p.CooldownMs != 6000 {
		t.Errorf("%+v", p)
	}

	rank3.ProcEntry.Chance = 3
	if _, err := resolveProc(rank3, rows); err == nil || !strings.Contains(err.Error(), "row -100") {
		t.Errorf("a capture that disagrees with the table must fail: %v", err)
	}

	built := &spellset.DumpSpell{ID: 1787, FirstRankID: 1784, ProcEntry: &spellset.DumpProc{ProcFlags: 0xa22a8, Chance: 100}}
	if p, err := resolveProc(built, rows); err != nil || p.Row != 0 || p.ProcFlags != 0xa22a8 {
		t.Errorf("server-built entry: %+v %v", p, err)
	}

	if p, err := resolveProc(&spellset.DumpSpell{ID: 5, FirstRankID: 5}, rows); p != nil || err != nil {
		t.Errorf("no row and no entry: %+v %v", p, err)
	}
	if _, err := resolveProc(&spellset.DumpSpell{ID: 102, FirstRankID: 102}, rows); err == nil {
		t.Error("a row the capture has no entry for must fail")
	}
}

func TestResolveBonus(t *testing.T) {
	rows := map[int32]serverdata.Bonus{116: {Row: 116, Direct: 0.857}}
	d := &spellset.DumpSpell{ID: 42842, FirstRankID: 116, Bonus: &spellset.DumpBonus{Direct: 0.857}}
	b, err := resolveBonus(d, rows)
	if err != nil || b.SpellID != 42842 || b.Row != 116 {
		t.Errorf("first rank's row: %+v %v", b, err)
	}
	d.Bonus.Direct = 0.8
	if _, err := resolveBonus(d, rows); err == nil {
		t.Error("a capture that disagrees with the table must fail")
	}
	if b, err := resolveBonus(&spellset.DumpSpell{ID: 9, FirstRankID: 9}, rows); b != nil || err != nil {
		t.Errorf("no row: %+v %v", b, err)
	}
}

func TestRenderSpells(t *testing.T) {
	src, err := renderSpells([]namedSpell{buildSpell(magicSpell())})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"// Frostbolt (Rank 16)",
		"ID: 42842, SchoolMask: 16, DmgClass: DmgClassMagic, Flags: FlagResetsAutoAttack | FlagHasteGCD",
		"PowerType: -2",
		"{},",
		"BonusMultiplier: 0.857, TriggerSpell: -1",
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("missing %q in\n%s", want, src)
		}
	}

	unnamed := buildSpell(magicSpell())
	unnamed.Flags |= 1 << 31
	if _, err := renderSpells([]namedSpell{unnamed}); err == nil {
		t.Error("a flag missing from flagNames must fail, not vanish from the table")
	}
}
