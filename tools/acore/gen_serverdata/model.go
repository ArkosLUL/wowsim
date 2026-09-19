package main

import (
	"database/sql"
	"fmt"
	"slices"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

// Attribute bits the flags come from (SharedDefines.h, SpellInfo.h, SpellDefines.h).
const (
	attr0UsesRangedSlot            = 0x00000002
	attr0IsAbility                 = 0x00000010
	attr0NoActiveDefense           = 0x00200000
	attr2DoNotResetCombatTimers    = 0x00020000
	attr3CompletelyBlocked         = 0x00000008
	attr3AlwaysHit                 = 0x00040000
	attr5SpellHasteAffectsPeriodic = 0x00002000
	attr6DoesntResetSwingIfInstant = 0x02000000
	interruptFlagInterrupt         = 0x08
	gcdCategoryHasted              = 133
)

type namedSpell struct {
	serverdata.Spell
	name string
}

type model struct {
	spells       []namedSpell
	procs        []serverdata.Proc
	bonuses      []serverdata.Bonus
	enchantProcs []serverdata.EnchantProc
}

// buildSpell carries a dump line over. int32(uint32(v)) turns the module's uint32 prints back into the
// signed values the server means, e.g. PowerType 4294967294 into -2.
func buildSpell(d *spellset.DumpSpell) namedSpell {
	i32 := func(v int64) int32 { return int32(uint32(v)) }
	u32 := func(v int64) uint32 { return uint32(v) }

	s := serverdata.Spell{
		ID:                 i32(d.ID),
		Family:             i32(d.Family),
		SchoolMask:         uint8(d.SchoolMask),
		DmgClass:           serverdata.DmgClass(d.DmgClass),
		AttributesCu:       u32(d.AttributesCu),
		CastMs:             i32(d.CastTimeMs),
		BaseCastMs:         i32(d.CastTimeBaseMs),
		GCDMs:              i32(d.StartRecoveryTimeMs),
		GCDCategory:        i32(d.StartRecoveryCategory),
		CooldownMs:         i32(d.RecoveryTimeMs),
		CategoryCooldownMs: i32(d.CategoryRecoveryTimeMs),
		Category:           i32(d.Category),
		DurationMs:         i32(d.DurationMs),
		MaxDurationMs:      i32(d.MaxDurationMs),
		StackAmount:        i32(d.StackAmount),
		ProcFlags:          u32(d.ProcFlags),
		ProcChance:         i32(d.ProcChance),
		ProcCharges:        i32(d.ProcCharges),
		PowerType:          i32(d.PowerType),
		ManaCost:           i32(d.ManaCost),
		ManaCostPct:        i32(d.ManaCostPercentage),
		RuneCostID:         i32(d.RuneCostID),
		Speed:              d.Speed,
		MaxTargets:         i32(d.MaxAffectedTargets),
	}
	for i := range s.FamilyFlags {
		s.FamilyFlags[i] = u32(d.FamilyFlags[i])
	}
	for i := range s.Attributes {
		s.Attributes[i] = u32(d.Attributes[i])
	}
	for _, e := range d.Effects {
		if e.Index < 0 || e.Index >= int64(len(s.Effects)) || e.Effect == 0 {
			continue
		}
		eff := serverdata.Effect{
			Effect:              i32(e.Effect),
			Aura:                i32(e.Aura),
			AmplitudeMs:         i32(e.AmplitudeMs),
			BasePoints:          i32(e.BasePoints),
			DieSides:            i32(e.DieSides),
			PointsPerLevel:      e.RealPointsPerLevel,
			PointsPerComboPoint: e.PointsPerComboPoint,
			ValueMultiplier:     e.ValueMultiplier,
			DamageMultiplier:    e.DamageMultiplier,
			BonusMultiplier:     e.BonusMultiplier,
			MiscValue:           i32(e.MiscValue),
			MiscValueB:          i32(e.MiscValueB),
			TriggerSpell:        i32(e.TriggerSpell),
			ChainTargets:        i32(e.ChainTarget),
		}
		for i := range eff.ClassMask {
			eff.ClassMask[i] = u32(e.ClassMask[i])
		}
		s.Effects[e.Index] = eff
	}
	s.Flags = flags(d, &s)

	name := d.Name
	if d.Rank != "" {
		name += " (" + d.Rank + ")"
	}
	return namedSpell{Spell: s, name: name}
}

func flags(d *spellset.DumpSpell, s *serverdata.Spell) serverdata.Flags {
	a := s.Attributes
	var f serverdata.Flags
	set := func(flag serverdata.Flags, on bool) {
		if on {
			f |= flag
		}
	}
	set(serverdata.FlagUsesRangedSlot, a[0]&attr0UsesRangedSlot != 0)
	set(serverdata.FlagBinary, d.Binary)
	set(serverdata.FlagNoActiveDefense, a[0]&attr0NoActiveDefense != 0)
	set(serverdata.FlagAlwaysHit, a[3]&attr3AlwaysHit != 0)
	set(serverdata.FlagCompletelyBlocked, a[3]&attr3CompletelyBlocked != 0)
	set(serverdata.FlagResetsAutoAttack, d.InterruptFlags&interruptFlagInterrupt != 0 &&
		a[2]&attr2DoNotResetCombatTimers == 0 &&
		!(s.CastMs == 0 && a[6]&attr6DoesntResetSwingIfInstant != 0))
	set(serverdata.FlagHasteAffectsPeriodic, a[5]&attr5SpellHasteAffectsPeriodic != 0)
	set(serverdata.FlagHasteGCD, s.GCDCategory == gcdCategoryHasted && s.GCDMs == 1500 &&
		s.DmgClass != serverdata.DmgClassMelee && s.DmgClass != serverdata.DmgClassRanged &&
		a[0]&(attr0UsesRangedSlot|attr0IsAbility) == 0)
	set(serverdata.FlagChanneled, d.Channeled)
	set(serverdata.FlagAutoRepeat, d.AutoRepeatRanged)
	set(serverdata.FlagPassive, d.Passive)
	set(serverdata.FlagPositive, d.Positive)
	return f
}

type procRow struct {
	serverdata.Proc
	id int32
}

func loadProcRows(db *sql.DB) (map[int32]procRow, error) {
	rows, err := db.Query(`SELECT SpellId, SchoolMask, SpellFamilyName, SpellFamilyMask0, SpellFamilyMask1,
		SpellFamilyMask2, ProcFlags, SpellTypeMask, SpellPhaseMask, HitMask, AttributesMask, DisableEffectsMask,
		ProcsPerMinute, Chance, Cooldown, Charges FROM spell_proc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int32]procRow{}
	for rows.Next() {
		var r procRow
		p := &r.Proc
		if err := rows.Scan(&r.id, &p.SchoolMask, &p.SpellFamilyName, &p.SpellFamilyMask[0], &p.SpellFamilyMask[1],
			&p.SpellFamilyMask[2], &p.ProcFlags, &p.SpellTypeMask, &p.SpellPhaseMask, &p.HitMask, &p.AttributesMask,
			&p.DisableEffectsMask, &p.ProcsPerMinute, &p.Chance, &p.CooldownMs, &p.Charges); err != nil {
			return nil, err
		}
		out[r.id] = r
	}
	return out, rows.Err()
}

// resolveProc mirrors SpellMgr::LoadSpellProcs for one spell. Rows load in SpellId order, so a negative
// row, which covers its whole rank chain, claims every rank before a positive row of a later rank would.
// Without a row the entry is whatever the server built from Spell.dbc, which only the dump has.
func resolveProc(d *spellset.DumpSpell, rows map[int32]procRow) (*serverdata.Proc, error) {
	id, first := int32(d.ID), int32(d.FirstRankID)
	row, ok := rows[-first]
	if !ok {
		row, ok = rows[id]
	}

	if !ok {
		if d.ProcEntry == nil {
			return nil, nil
		}
		p := dumpProc(d.ProcEntry)
		p.SpellID = id
		return &p, nil
	}

	p := row.Proc
	p.SpellID, p.Row = id, row.id
	if p.ProcFlags == 0 {
		p.ProcFlags = uint32(d.ProcFlags)
	}
	if p.Charges == 0 {
		p.Charges = int32(d.ProcCharges)
	}
	if p.Chance == 0 && p.ProcsPerMinute == 0 {
		p.Chance = float32(d.ProcChance)
	}
	p.Chance = max(p.Chance, 0)
	p.ProcsPerMinute = max(p.ProcsPerMinute, 0)
	p.Charges = min(p.Charges, 99)

	if d.ProcEntry == nil {
		return nil, fmt.Errorf("spell_proc row %d covers %d, which has no proc entry in the capture", row.id, id)
	}
	if got := dumpProc(d.ProcEntry); got != withoutSource(p) {
		return nil, fmt.Errorf("spell_proc row %d resolves to %+v for %d, the capture has %+v", row.id, withoutSource(p), id, got)
	}
	return &p, nil
}

func withoutSource(p serverdata.Proc) serverdata.Proc {
	p.SpellID, p.Row = 0, 0
	return p
}

func dumpProc(e *spellset.DumpProc) serverdata.Proc {
	return serverdata.Proc{
		SchoolMask:         uint8(e.SchoolMask),
		SpellFamilyName:    int32(e.SpellFamilyName),
		SpellFamilyMask:    [3]uint32{uint32(e.SpellFamilyMask[0]), uint32(e.SpellFamilyMask[1]), uint32(e.SpellFamilyMask[2])},
		ProcFlags:          uint32(e.ProcFlags),
		SpellTypeMask:      uint32(e.SpellTypeMask),
		SpellPhaseMask:     uint32(e.SpellPhaseMask),
		HitMask:            uint32(e.HitMask),
		AttributesMask:     uint32(e.AttributesMask),
		DisableEffectsMask: uint32(e.DisableEffectsMask),
		ProcsPerMinute:     e.ProcsPerMinute,
		Chance:             e.Chance,
		CooldownMs:         int32(e.CooldownMs),
		Charges:            int32(e.Charges),
	}
}

func loadBonusRows(db *sql.DB) (map[int32]serverdata.Bonus, error) {
	rows, err := db.Query("SELECT entry, direct_bonus, dot_bonus, ap_bonus, ap_dot_bonus FROM spell_bonus_data")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int32]serverdata.Bonus{}
	for rows.Next() {
		var b serverdata.Bonus
		if err := rows.Scan(&b.Row, &b.Direct, &b.Dot, &b.AP, &b.APDot); err != nil {
			return nil, err
		}
		out[b.Row] = b
	}
	return out, rows.Err()
}

// resolveBonus mirrors SpellMgr::GetSpellBonusData: the spell's own row, else its first rank's.
func resolveBonus(d *spellset.DumpSpell, rows map[int32]serverdata.Bonus) (*serverdata.Bonus, error) {
	id := int32(d.ID)
	b, ok := rows[id]
	if !ok {
		b, ok = rows[int32(d.FirstRankID)]
	}
	if !ok {
		if d.Bonus != nil {
			return nil, fmt.Errorf("spell %d has bonus data in the capture but no spell_bonus_data row", id)
		}
		return nil, nil
	}
	b.SpellID = id
	if d.Bonus == nil || (serverdata.Bonus{Direct: d.Bonus.Direct, Dot: d.Bonus.Dot, AP: d.Bonus.AP, APDot: d.Bonus.APDot} !=
		serverdata.Bonus{Direct: b.Direct, Dot: b.Dot, AP: b.AP, APDot: b.APDot}) {
		return nil, fmt.Errorf("spell_bonus_data row %d gives %d %+v, the capture has %+v", b.Row, id, b, d.Bonus)
	}
	return &b, nil
}

func loadEnchantProcs(db *sql.DB) ([]serverdata.EnchantProc, error) {
	rows, err := db.Query("SELECT entry, customChance, PPMChance, procEx, attributeMask FROM spell_enchant_proc_data ORDER BY entry")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []serverdata.EnchantProc
	for rows.Next() {
		var e serverdata.EnchantProc
		if err := rows.Scan(&e.EnchantID, &e.CustomChance, &e.PPM, &e.ProcEx, &e.AttributeMask); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// buildModel covers the given spells, which must all be in the dump. Every disagreement between the
// capture and the live tables is returned together: the two only match while the server runs on the
// tables it loaded, so a mismatch calls for a recapture or a server restart.
func buildModel(dump spellset.Dump, ids []int32, db *sql.DB) (*model, []error, error) {
	procRows, err := loadProcRows(db)
	if err != nil {
		return nil, nil, fmt.Errorf("spell_proc: %w", err)
	}
	bonusRows, err := loadBonusRows(db)
	if err != nil {
		return nil, nil, fmt.Errorf("spell_bonus_data: %w", err)
	}

	m := &model{}
	if m.enchantProcs, err = loadEnchantProcs(db); err != nil {
		return nil, nil, fmt.Errorf("spell_enchant_proc_data: %w", err)
	}

	var mismatches []error
	for _, id := range slices.Sorted(slices.Values(ids)) {
		d := dump[id]
		m.spells = append(m.spells, buildSpell(d))
		if p, err := resolveProc(d, procRows); err != nil {
			mismatches = append(mismatches, err)
		} else if p != nil {
			m.procs = append(m.procs, *p)
		}
		if b, err := resolveBonus(d, bonusRows); err != nil {
			mismatches = append(mismatches, err)
		} else if b != nil {
			m.bonuses = append(m.bonuses, *b)
		}
	}
	return m, mismatches, nil
}
