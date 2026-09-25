package core

import (
	"maps"
	"math"
	"slices"
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// ids no generated table has, so no bonus row or allowlist entry reaches the hand-built rows
const (
	testSpellID     = 990001
	testKeptSpellID = 990002
)

func TestServerEffectOf(t *testing.T) {
	bolt := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMagic, SpellLevel: 79, BaseLevel: 79, MaxLevel: 83,
		Effects: [3]serverdata.Effect{
			{Effect: 2, BasePoints: 798, DieSides: 63, PointsPerLevel: 4.8, BonusMultiplier: 0.857},
			{Effect: 6, Aura: auraTypePeriodicDamage, BasePoints: 179, DieSides: 1, BonusMultiplier: 0.5},
		}}
	strike := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMelee,
		Effects: [3]serverdata.Effect{
			{Effect: effectWeaponPercentDamage, BasePoints: 79, DieSides: 1},
			{Effect: 3},
			{Effect: effectNormalizedWeaponDmg, BasePoints: 583, DieSides: 1, BonusMultiplier: 1},
		}}
	plainStrike := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMelee,
		Effects: [3]serverdata.Effect{{Effect: effectNormalizedWeaponDmg, BasePoints: 379, DieSides: 1}}}
	noClass := &serverdata.Spell{ID: testSpellID,
		Effects: [3]serverdata.Effect{{Effect: 10, BasePoints: 99, DieSides: 1, BonusMultiplier: 1}}}
	row := &serverdata.Bonus{Direct: 0.2, Dot: 0.1, AP: 0.15, APDot: -1}

	for _, tc := range []struct {
		name  string
		s     *serverdata.Spell
		bonus *serverdata.Bonus
		index int
		want  serverEffect
	}{
		{"level 80 range, no row: BonusMultiplier", bolt, nil, 0, serverEffect{min: 803, max: 865, sp: 0.857}},
		{"a row replaces BonusMultiplier", bolt, row, 0, serverEffect{min: 803, max: 865, sp: 0.2, ap: 0.15}},
		{"a periodic aura takes the dot columns, AP only above 0", bolt, row, 1, serverEffect{min: 180, max: 180, sp: 0.1}},
		{"weapon: the flat roll, the percent, no coefficient", strike, row, 2, serverEffect{min: 584, max: 584, weaponPct: 0.8}},
		{"the percent effect itself rolls nothing", strike, nil, 0, serverEffect{weaponPct: 0.8}},
		{"a weapon hit without a percent effect is the whole weapon", plainStrike, nil, 0, serverEffect{min: 380, max: 380, weaponPct: 1}},
		{"DmgClassNone gets no BonusMultiplier", noClass, nil, 0, serverEffect{min: 100, max: 100}},
		{"but does get a row", noClass, row, 0, serverEffect{min: 100, max: 100, sp: 0.2, ap: 0.15}},
	} {
		if got := serverEffectOf(tc.s, tc.bonus, tc.index); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestCheckEffect(t *testing.T) {
	row := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMagic,
		Effects: [3]serverdata.Effect{{}, {Effect: 2, BasePoints: 802, DieSides: 63, BonusMultiplier: 0.857}}}
	exact := SpellEffect{Effect: 1, Min: 803, Max: 865, SP: 0.857}

	spell := &Spell{ActionID: ActionID{SpellID: testSpellID}}
	e := exact
	spell.checkEffect(&e, row, directEffectFields)
	if e != exact || len(spell.ServerConflicts()) != 0 {
		t.Errorf("an exact declaration: %+v, %v", e, spell.ServerConflicts())
	}
	var none SpellEffect
	spell.checkEffect(&none, row, directEffectFields)
	if none != (SpellEffect{}) || len(spell.ServerConflicts()) != 0 {
		t.Errorf("the zero value declares nothing: %+v, %v", none, spell.ServerConflicts())
	}

	// off by one, and an AP the server doesn't have: the server's values replace them
	e = SpellEffect{Effect: 1, Min: 804, Max: 866, SP: 0.857, AP: 0.1}
	spell.checkEffect(&e, row, directEffectFields)
	if e != exact {
		t.Errorf("the server's values apply: %+v", e)
	}
	want := []ServerConflict{
		{Spell: spell.ActionID, Field: ServerMin, SimFloat: 804, ServerFloat: 803},
		{Spell: spell.ActionID, Field: ServerMax, SimFloat: 866, ServerFloat: 865},
		{Spell: spell.ActionID, Field: ServerAP, SimFloat: 0.1, ServerFloat: 0},
	}
	if got := spell.ServerConflicts(); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("conflicts %v, want %v", got, want)
	}

	// an undeclared coefficient is a conflict, and takes the table's decimal
	tick := &Spell{ActionID: ActionID{SpellID: testSpellID}}
	e = SpellEffect{Effect: 1, Min: 803, Max: 865}
	tick.checkEffect(&e, row, tickEffectFields)
	if got := tick.ServerConflicts(); e.SP != 0.857 || len(got) != 1 || got[0].Field != ServerTickSP || got[0].ServerFloat != 0.857 {
		t.Errorf("undeclared SP: %+v, %v", e, got)
	}

	// a KeepSim entry keeps the declared value
	kept := &Spell{ActionID: ActionID{SpellID: testKeptSpellID}}
	key := serverConflictKey{kept.ActionID, ServerSP}
	serverAllowances[key] = &ServerConflictAllowance{Spell: kept.ActionID, Field: ServerSP, SimFloat: 0.9, ServerFloat: 0.857, KeepSim: true, Why: "test"}
	defer delete(serverAllowances, key)
	e = SpellEffect{Effect: 1, Min: 803, Max: 865, SP: 0.9}
	kept.checkEffect(&e, row, directEffectFields)
	if got := kept.ServerConflicts(); e.SP != 0.9 || len(got) != 1 || got[0].Allowed != serverAllowances[key] {
		t.Errorf("KeepSim: %+v, %v", e, got)
	}

	// FromSpellID checks the named spell, with or without the declaring spell's own data
	frostbolt := SpellEffect{Effect: 1, FromSpellID: 42842, Min: 803, Max: 865, SP: 0.857}
	from := &Spell{ActionID: ActionID{SpellID: testSpellID}}
	e = frostbolt
	from.checkEffect(&e, nil, directEffectFields)
	e = frostbolt
	e.Min = 804
	from.checkEffect(&e, row, directEffectFields)
	if got := from.ServerConflicts(); len(got) != 1 || got[0].Field != ServerMin || got[0].ServerFloat != 803 {
		t.Errorf("FromSpellID: %v", got)
	}

	// without server data, nothing to check
	orphan := &Spell{ActionID: ActionID{SpellID: testSpellID}}
	e = SpellEffect{Effect: 1, Min: 1}
	orphan.checkEffect(&e, nil, directEffectFields)
	if e.Min != 1 || len(orphan.ServerConflicts()) != 0 {
		t.Errorf("no server data: %+v, %v", e, orphan.ServerConflicts())
	}
}

func TestServerSchoolAndMissileSpeed(t *testing.T) {
	for mask, want := range map[uint8]SpellSchool{
		1: SpellSchoolPhysical, 2: SpellSchoolHoly, 4: SpellSchoolFire, 8: SpellSchoolNature, 16: SpellSchoolFrost,
		32: SpellSchoolShadow, 64: SpellSchoolArcane, 20: SpellSchoolFire | SpellSchoolFrost, 126: SpellSchoolMagic,
	} {
		if got := SpellSchoolFromServerMask(mask); got != want {
			t.Errorf("mask %d: %v, want %v", mask, got, want)
		}
		if got := want.ServerSchoolMask(); got != mask {
			t.Errorf("%v: mask %d, want %d", want, got, mask)
		}
	}

	arcane := &serverdata.Spell{ID: testSpellID, SchoolMask: 64, Speed: 24.5}
	spell := &Spell{ActionID: ActionID{SpellID: testSpellID}, SpellSchool: SpellSchoolFrost, SchoolIndex: stats.SchoolIndexFrost}
	spell.syncServerSchool(arcane)
	spell.syncServerMissileSpeed(arcane)
	if spell.SpellSchool != SpellSchoolArcane || spell.SchoolIndex != stats.SchoolIndexArcane || spell.MissileSpeed != 24.5 {
		t.Errorf("the server's school and speed apply: %v, %v, %v", spell.SpellSchool, spell.SchoolIndex, spell.MissileSpeed)
	}
	want := []ServerConflict{
		{Spell: spell.ActionID, Field: ServerSchool, Sim: int64(SpellSchoolFrost), Server: int64(SpellSchoolArcane)},
		{Spell: spell.ActionID, Field: ServerMissileSpeed, SimFloat: 0, ServerFloat: 24.5},
	}
	if got := spell.ServerConflicts(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("conflicts %v, want %v", got, want)
	}

	schoolless := &Spell{ActionID: ActionID{SpellID: testSpellID}, MissileSpeed: 24.5}
	schoolless.syncServerSchool(arcane)
	schoolless.syncServerMissileSpeed(arcane)
	if schoolless.SpellSchool != SpellSchoolNone || len(schoolless.ServerConflicts()) != 0 {
		t.Errorf("a spell with no school isn't checked, a matching speed is fine: %v, %v", schoolless.SpellSchool, schoolless.ServerConflicts())
	}

	// Empower Rune Weapon's row has no school
	fire := &Spell{ActionID: ActionID{SpellID: testSpellID}, SpellSchool: SpellSchoolFire, SchoolIndex: stats.SchoolIndexFire}
	fire.syncServerSchool(&serverdata.Spell{ID: testSpellID})
	if fire.SpellSchool != SpellSchoolFire || fire.SchoolIndex != stats.SchoolIndexFire || len(fire.ServerConflicts()) != 0 {
		t.Errorf("a server row with no school isn't checked: %v, %v, %v", fire.SpellSchool, fire.SchoolIndex, fire.ServerConflicts())
	}
}

func TestSpellEffectRoll(t *testing.T) {
	fixed := SpellEffect{Min: 180, Max: 180}
	// a nil sim would panic on a roll
	if got := fixed.Roll(nil); got != 180 || fixed.Average() != 180 {
		t.Errorf("fixed value %v", got)
	}
	sim := &Simulation{rand: NewSplitMix(1)}
	ranged := SpellEffect{Min: 803, Max: 865}
	for range 1000 {
		if got := ranged.Roll(sim); got < 803 || got > 865 {
			t.Fatalf("roll %v outside 803..865", got)
		}
	}
	if ranged.Average() != 834 {
		t.Errorf("average %v", ranged.Average())
	}
}

func TestWithMods(t *testing.T) {
	bolt := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMagic,
		Effects: [3]serverdata.Effect{{Effect: 6}, {Effect: 2, BasePoints: 802, DieSides: 63}}}
	// Obliterate's layout: the flat roll, the percent, then a dummy
	strike := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMelee,
		Effects: [3]serverdata.Effect{{Effect: effectNormalizedWeaponDmg}, {Effect: effectWeaponPercentDamage}, {Effect: 3}}}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-12 }
	boltEffect := SpellEffect{Effect: 1, Min: 803, Max: 865, SP: 0.857}
	strikeEffect := SpellEffect{Effect: 0, Min: 584, Max: 584, WeaponPct: 0.8}

	for _, tc := range []struct {
		name string
		s    *serverdata.Spell
		e    SpellEffect
		mods []SpellMod
		want SpellEffect
	}{
		{"op 24 flats add hundredths", bolt, SpellEffect{Effect: 1, SP: 0.2},
			[]SpellMod{{Op: SpellModBonusMultiplier, Flat: 6}, {Op: SpellModBonusMultiplier, Flat: 5}}, SpellEffect{Effect: 1, SP: 0.31}},
		{"op 24 percents add up", bolt, SpellEffect{Effect: 1, SP: 0.5},
			[]SpellMod{{Op: SpellModBonusMultiplier, Pct: 10}, {Op: SpellModBonusMultiplier, Pct: 20}}, SpellEffect{Effect: 1, SP: 0.65}},
		{"op 24 skips a 0 coefficient", bolt, SpellEffect{Effect: 1, Min: 803, Max: 865, AP: 0.15},
			[]SpellMod{{Op: SpellModBonusMultiplier, Flat: 10}}, SpellEffect{Effect: 1, Min: 803, Max: 865, AP: 0.15}},
		{"the effect's own op and all effects go on the roll", bolt, boltEffect,
			[]SpellMod{{Op: SpellModEffect2, Flat: 10}, {Op: SpellModAllEffects, Pct: 10}},
			SpellEffect{Effect: 1, Min: 893, Max: 961, SP: 0.857}},
		{"in float32, truncated", bolt, SpellEffect{Effect: 1, Min: 100, Max: 100},
			[]SpellMod{{Op: SpellModEffect2, Pct: 15}}, SpellEffect{Effect: 1, Min: 115, Max: 115}},
		{"the percent effect's op goes on WeaponPct", strike, strikeEffect,
			[]SpellMod{{Op: SpellModEffect1, Flat: 420}, {Op: SpellModEffect2, Flat: 20}},
			SpellEffect{Effect: 0, Min: 1004, Max: 1004, WeaponPct: 1}},
		{"all effects reach both", strike, strikeEffect,
			[]SpellMod{{Op: SpellModAllEffects, Pct: 10}}, SpellEffect{Effect: 0, Min: 642, Max: 642, WeaponPct: 0.88}},
		{"a percent effect declared on its own rolls nothing", strike, SpellEffect{Effect: 1, WeaponPct: 0.8},
			[]SpellMod{{Op: SpellModEffect2, Flat: 20}}, SpellEffect{Effect: 1, WeaponPct: 1}},
		{"an untaken rank changes nothing, a kept value included", bolt, SpellEffect{Effect: 1, Min: 179.5, Max: 179.5, SP: 0.857},
			[]SpellMod{{Op: SpellModEffect2}, {Op: SpellModBonusMultiplier}}, SpellEffect{Effect: 1, Min: 179.5, Max: 179.5, SP: 0.857}},
		{"no server data: all effects only reach the roll", nil, strikeEffect,
			[]SpellMod{{Op: SpellModAllEffects, Flat: 1}}, SpellEffect{Effect: 0, Min: 585, Max: 585, WeaponPct: 0.8}},
	} {
		spell := &Spell{ActionID: ActionID{SpellID: testSpellID}, serverSpell: tc.s}
		var covered [len(effectModOps)]bool
		got := spell.withMods(tc.e, tc.mods, &covered)
		if got.Effect != tc.want.Effect || got.Min != tc.want.Min || got.Max != tc.want.Max || !near(got.SP, tc.want.SP) ||
			got.AP != tc.want.AP || !near(got.WeaponPct, tc.want.WeaponPct) {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}

	// FromSpellID places the percent effect by the named spell: Obliterate's off-hand strike
	spell := &Spell{ActionID: ActionID{SpellID: testSpellID}, Direct: SpellEffect{FromSpellID: 66974, Min: 292, Max: 292, WeaponPct: 0.8}}
	spell.applyMods(&SpellConfig{Mods: []SpellMod{{Op: SpellModEffect2, Flat: 20}}})
	if spell.Direct.WeaponPct != 1 {
		t.Errorf("FromSpellID: %+v", spell.Direct)
	}

	// one spell's mods reach each declaration by its effect, and op 24 every coefficient
	immolate := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMagic,
		Effects: [3]serverdata.Effect{{Effect: 2}, {Effect: 6, Aura: auraTypePeriodicDamage}}}
	spell = &Spell{ActionID: ActionID{SpellID: testSpellID}, serverSpell: immolate, Direct: SpellEffect{Effect: 0, Min: 100, Max: 100, SP: 0.5}}
	config := &SpellConfig{Dot: DotConfig{Tick: SpellEffect{Effect: 1, Min: 50, Max: 50, SP: 0.1}},
		Mods: []SpellMod{{Op: SpellModEffect2, Flat: 10}, {Op: SpellModBonusMultiplier, Flat: 10}}}
	spell.applyMods(config)
	if d, tick := spell.Direct, config.Dot.Tick; d.Min != 100 || !near(d.SP, 0.6) || tick.Min != 60 || tick.Max != 60 || !near(tick.SP, 0.2) {
		t.Errorf("direct %+v, tick %+v", d, tick)
	}

	for _, tc := range []struct {
		name string
		s    *serverdata.Spell
		e    SpellEffect
		mods []SpellMod
	}{
		{"an effect no declaration covers", strike, strikeEffect, []SpellMod{{Op: SpellModEffect3, Flat: 1}}},
		{"a percent effect only the server data places", nil, strikeEffect, []SpellMod{{Op: SpellModEffect2, Flat: 20}}},
		{"a damage percent", bolt, boltEffect, []SpellMod{{Op: 0, Pct: 5}}},
		{"mods on nothing declared", bolt, SpellEffect{}, []SpellMod{{Op: SpellModBonusMultiplier, Flat: 5}}},
		{"an effect index past 2", nil, SpellEffect{Effect: 3, Min: 1, Max: 1}, []SpellMod{{Op: SpellModAllEffects, Flat: 1}}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", tc.name)
				}
			}()
			spell := &Spell{ActionID: ActionID{SpellID: testSpellID}, serverSpell: tc.s, Direct: tc.e}
			spell.applyMods(&SpellConfig{Mods: tc.mods})
		}()
	}
}

func TestDeclarableAndDeclaredEffects(t *testing.T) {
	bolt := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMagic,
		Effects: [3]serverdata.Effect{
			{Effect: 2, BasePoints: 802, DieSides: 63, BonusMultiplier: 0.857},
			{Effect: 6, Aura: auraTypePeriodicDamage, BasePoints: 29, DieSides: 1},
			{Effect: 6, Aura: auraTypePeriodicDamage},
		}}
	strike := &serverdata.Spell{ID: testSpellID, DmgClass: serverdata.DmgClassMelee,
		Effects: [3]serverdata.Effect{
			{Effect: effectWeaponPercentDamage, BasePoints: 119, DieSides: 1},
			{Effect: 3},
			{Effect: effectWeaponDamage, BasePoints: 241, DieSides: 1},
		}}

	// the empty third effect has nothing to declare
	want := map[EffectRef]SpellEffect{
		{testSpellID, 0}: {Effect: 0, FromSpellID: testSpellID, Min: 803, Max: 865, SP: 0.857},
		{testSpellID, 1}: {Effect: 1, FromSpellID: testSpellID, Min: 30, Max: 30},
	}
	if got := DeclarableEffects(bolt); !maps.Equal(got, want) {
		t.Errorf("bolt: %+v, want %+v", got, want)
	}
	// the weapon effects take one declaration, on the flat effect, under the first of them
	want = map[EffectRef]SpellEffect{{testSpellID, 0}: {Effect: 2, FromSpellID: testSpellID, Min: 242, Max: 242, WeaponPct: 1.2}}
	if got := DeclarableEffects(strike); !maps.Equal(got, want) {
		t.Errorf("strike: %+v, want %+v", got, want)
	}

	spell := &Spell{serverSpell: strike, Direct: SpellEffect{Effect: 2, Min: 242, Max: 242, WeaponPct: 1.2}}
	if got := spell.DeclaredEffects(); !slices.Equal(got, []EffectRef{{testSpellID, 0}}) {
		t.Errorf("strike declares %v", got)
	}
	// a declaration naming another spell covers that spell's effect, and a spell without server data covers
	// nothing of its own
	spell = &Spell{Direct: SpellEffect{Effect: 1, Min: 1, Max: 1}, aoeDot: &Dot{Tick: SpellEffect{FromSpellID: 42842, Effect: 1, Min: 1}}}
	if got := spell.DeclaredEffects(); !slices.Equal(got, []EffectRef{{42842, 1}}) {
		t.Errorf("FromSpellID declares %v", got)
	}
}

func TestServerFloat(t *testing.T) {
	for f, want := range map[float32]float64{0.857: 0.857, 0.119658: 0.119658, 0.0299: 0.0299, 28: 28, 0: 0} {
		if got := serverFloat(f); got != want {
			t.Errorf("serverFloat(%v) = %v, want %v", f, got, want)
		}
	}
}
