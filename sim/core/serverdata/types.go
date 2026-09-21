// Package serverdata holds spell data taken from the live AzerothCore server: SpellInfo from the committed
// `.simval spelldump` capture, plus spell_proc, spell_bonus_data and spell_enchant_proc_data. The
// *_auto_gen.go tables come from tools/acore/gen_serverdata and cover the spells the sim names by id plus
// the spells those trigger.
//
// It must not import sim/core, so that sim/core can import it.
package serverdata

type DmgClass uint8

// SpellDmgClass: picks the hit table and which haste scales the cast.
const (
	DmgClassNone DmgClass = iota
	DmgClassMagic
	DmgClassMelee
	DmgClassRanged
)

// Flags are yes/no properties worked out from a spell's attributes the way the server reads them. Kept
// apart from core.SpellFlag, which has no bits to spare.
type Flags uint32

const (
	// SPELL_ATTR0_USES_RANGED_SLOT: ranged weapon, ranged haste. CastMs already has its +500 ms, which
	// auto-repeat spells don't get.
	FlagUsesRangedSlot Flags = 1 << iota
	// SPELL_ATTR0_CU_BINARY_SPELL: all or nothing against resistance
	FlagBinary
	// SPELL_ATTR0_NO_ACTIVE_DEFENSE: can't be dodged, parried or blocked
	FlagNoActiveDefense
	// SPELL_ATTR3_ALWAYS_HIT: can't miss or be avoided at all
	FlagAlwaysHit
	// SPELL_ATTR3_COMPLETELY_BLOCKED: a block stops the whole hit instead of part of it
	FlagCompletelyBlocked
	// Spell::IsAutoActionResetSpell for an untriggered cast at CastMs: interruptible (SPELL_INTERRUPT_FLAG_INTERRUPT),
	// no SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS, and not instant with SPELL_ATTR6_DOESNT_RESET_SWING_TIMER_IF_INSTANT.
	// At runtime triggered casts never reset, and neither does a cast spell made instant or one an
	// SPELL_AURA_IGNORE_MELEE_RESET aura covers.
	FlagResetsAutoAttack
	// SPELL_ATTR5_SPELL_HASTE_AFFECTS_PERIODIC: haste shortens the tick interval
	FlagHasteAffectsPeriodic
	// Spell::TriggerGlobalCooldown scales the GCD with cast speed: category 133, 1500 ms, not melee or ranged class,
	// no ranged slot, no SPELL_ATTR0_IS_ABILITY
	FlagHasteGCD
	FlagChanneled
	// SPELL_ATTR2_AUTO_REPEAT: Auto Shot, Shoot
	FlagAutoRepeat
	FlagPassive
	// SpellInfo::IsPositive
	FlagPositive
)

// Spell is one spell's SpellInfo as the server holds it after Spell.dbc, spell_dbc and its corrections.
type Spell struct {
	ID          int32
	Family      int32     // SpellFamilyName: 3 mage ... 15 death knight, 17 pet
	FamilyFlags [3]uint32 // what class masks of talents, glyphs and procs match
	SchoolMask  uint8
	DmgClass    DmgClass
	Flags       Flags

	Attributes   [8]uint32 // Attributes, AttributesEx ... AttributesEx7
	AttributesCu uint32    // the server's SPELL_ATTR0_CU_*

	CastMs             int32 // SpellInfo::CalcCastTime without a caster
	BaseCastMs         int32 // SpellCastTimes.dbc, before the ranged slot's +500
	GCDMs              int32 // StartRecoveryTime
	GCDCategory        int32 // StartRecoveryCategory
	CooldownMs         int32 // RecoveryTime
	CategoryCooldownMs int32
	Category           int32
	DurationMs         int32 // -1 means until cancelled
	MaxDurationMs      int32
	StackAmount        int32

	// Spell.dbc's proc fields. The entry the server really uses is in Procs.
	ProcFlags   uint32
	ProcChance  int32
	ProcCharges int32

	PowerType   int32 // 0 mana, 1 rage, 3 energy, 5 runes, 6 runic power, -2 health
	ManaCost    int32 // rage and runic power in tenths
	ManaCostPct int32 // percent of base mana
	RuneCostID  int32

	Speed      float32 // missile speed, yards per second; 0 hits instantly
	MaxTargets int32

	Effects [3]Effect // unused slots are zero

	// what talents, glyphs, set bonuses and items can do to CastMs, GCDMs and both cooldowns
	CastMods     ModBounds
	GCDMods      ModBounds
	CooldownMods ModBounds
}

// ModBounds is how far the passive spell modifiers that match a spell (SPELL_AURA_ADD_FLAT_MODIFIER and
// ADD_PCT_MODIFIER on its SpellFamilyFlags) can move one of its values, at both extremes, one rank per
// talent. Zero when none reaches it.
type ModBounds struct {
	FlatMin, FlatMax int32 // ms
	PctMin, PctMax   int32 // percent of the base value
}

// CastRange is the span of cast times the modifiers can give, the way Player::ApplySpellMod does
// SPELLMOD_CASTING_TIME: (base + flat) * (100 + pct)%. An instant stays instant.
func (s *Spell) CastRange() (lo, hi int32) {
	if s.CastMs <= 0 {
		return s.CastMs, s.CastMs
	}
	at := func(flat, pct int32) int32 {
		return max(0, int32(float64(s.CastMs+flat)*float64(100+pct)/100))
	}
	return at(s.CastMods.FlatMin, s.CastMods.PctMin), at(s.CastMods.FlatMax, s.CastMods.PctMax)
}

// GCDRange is Spell::TriggerGlobalCooldown without haste: SPELLMOD_GLOBAL_COOLDOWN only reaches a 1 to
// 1.5 s GCD, and the result stays in that span.
func (s *Spell) GCDRange() (lo, hi int32) {
	if s.GCDMs < 1000 || s.GCDMs > 1500 {
		return s.GCDMs, s.GCDMs
	}
	lo, hi = modRange(s.GCDMs, s.GCDMods)
	return min(max(lo, 1000), 1500), min(max(hi, 1000), 1500)
}

// CooldownRange is Player::AddSpellAndCategoryCooldowns for one of the two cooldowns: base * pct + flat,
// no lower than 0. SPELL_ATTR6_NO_CATEGORY_COOLDOWN_MODS keeps the category cooldown unmodified.
func (s *Spell) CooldownRange(baseMs int32, category bool) (lo, hi int32) {
	if category && s.Attributes[6]&attr6NoCategoryCooldownMods != 0 {
		return baseMs, baseMs
	}
	lo, hi = modRange(baseMs, s.CooldownMods)
	return max(lo, 0), max(hi, 0)
}

// OwnCooldownMs is the cooldown the spell puts itself on: RecoveryTime, else its category's.
func (s *Spell) OwnCooldownMs() int32 {
	if s.CooldownMs > 0 {
		return s.CooldownMs
	}
	return s.CategoryCooldownMs
}

const attr6NoCategoryCooldownMods = 0x80000000

// modRange is base * (100 + pct)% + flat. Percent modifiers skip a zero base.
func modRange(base int32, b ModBounds) (lo, hi int32) {
	at := func(flat, pct int32) int32 {
		if base == 0 {
			pct = 0
		}
		return int32(float64(base)*float64(100+pct)/100) + flat
	}
	return at(b.FlatMin, b.PctMin), at(b.FlatMax, b.PctMax)
}

// Effect is a SpellEffectInfo. A roll is BasePoints + 1..DieSides.
type Effect struct {
	Effect              int32 // SpellEffects
	Aura                int32 // AuraType, for aura effects
	AmplitudeMs         int32 // tick interval of periodic auras
	BasePoints          int32
	DieSides            int32
	PointsPerLevel      float32
	PointsPerComboPoint float32
	ValueMultiplier     float32
	DamageMultiplier    float32 // chain jumps
	BonusMultiplier     float32 // spell power coefficient when spell_bonus_data has no row
	MiscValue           int32
	MiscValueB          int32
	TriggerSpell        int32 // -1 in some dummy auras, which isn't a spell
	ChainTargets        int32
	ClassMask           [3]uint32 // spells a modifier or proc aura applies to
}

// Proc is SpellMgr::GetSpellProcEntry: a spell_proc row with Spell.dbc filling in unset ProcFlags,
// Charges and Chance, or the entry the server builds for a proc aura without a row.
type Proc struct {
	SpellID int32
	// the spell_proc SpellId the entry came from: the spell's own id, minus its first rank's for a row
	// covering the whole rank chain, or 0 when the server built it
	Row int32

	SchoolMask         uint8
	SpellFamilyName    int32
	SpellFamilyMask    [3]uint32
	ProcFlags          uint32
	SpellTypeMask      uint32
	SpellPhaseMask     uint32
	HitMask            uint32
	AttributesMask     uint32 // ProcAttr*
	DisableEffectsMask uint32
	ProcsPerMinute     float32
	Chance             float32
	CooldownMs         int32
	Charges            int32
}

// ProcFlagAttributes (SpellMgr.h), for Proc.AttributesMask.
const (
	ProcAttrReqExpOrHonor        uint32 = 0x1
	ProcAttrTriggeredCanProc     uint32 = 0x2
	ProcAttrReqManaCost          uint32 = 0x4
	ProcAttrReqSpellmod          uint32 = 0x8
	ProcAttrUseStacksForCharges  uint32 = 0x10
	ProcAttrReduceProc60         uint32 = 0x80
	ProcAttrCantProcFromItemCast uint32 = 0x100
)

// Bonus is SpellMgr::GetSpellBonusData: the spell's spell_bonus_data row, else its first rank's. The
// values are the table's; Unit::SpellDamageBonusDone decides what they mean.
type Bonus struct {
	SpellID int32
	Row     int32 // the row's entry: the spell or its first rank

	Direct float32 // direct_bonus
	Dot    float32 // dot_bonus, per tick
	AP     float32 // ap_bonus
	APDot  float32 // ap_dot_bonus
}

// EnchantProc is a spell_enchant_proc_data row, which overrides the proc chance of a weapon enchant's
// combat spell (Player::CastItemCombatSpell).
type EnchantProc struct {
	EnchantID     int32 // SpellItemEnchantment id
	CustomChance  int32 // percent, used when PPM is 0
	PPM           float32
	ProcEx        uint32
	AttributeMask uint32 // EnchantProcAttr*
}

const (
	EnchantProcAttrExclusive uint32 = 0x1 // one instance of the effect at a time
	EnchantProcAttrWhiteHit  uint32 = 0x2 // white hits only, no abilities
)

// Spells lists every generated spell, sorted by ID.
func Spells() []Spell { return spells }

// SpellByID returns nil for spells the tables don't cover. The lookups all point into the shared tables,
// so don't modify what they return.
//
// Keep the spell lookups' parameter named spellID: that's how tools/acore/spellids picks up a constant
// passed here, and without it regenerating drops that spell from the tables.
func SpellByID(spellID int32) *Spell {
	return find(spells, spellID, func(s *Spell) int32 { return s.ID })
}

// Procs lists every generated proc entry, sorted by SpellID.
func Procs() []Proc { return procs }

func ProcBySpellID(spellID int32) *Proc {
	return find(procs, spellID, func(p *Proc) int32 { return p.SpellID })
}

// Bonuses lists every generated spell_bonus_data entry, sorted by SpellID.
func Bonuses() []Bonus { return bonuses }

func BonusBySpellID(spellID int32) *Bonus {
	return find(bonuses, spellID, func(b *Bonus) int32 { return b.SpellID })
}

// EnchantProcs lists every spell_enchant_proc_data row, sorted by EnchantID.
func EnchantProcs() []EnchantProc { return enchantProcs }

func EnchantProcByID(id int32) *EnchantProc {
	return find(enchantProcs, id, func(e *EnchantProc) int32 { return e.EnchantID })
}

// Binary search by index: a comparator taking the row by value copies it on every probe, and since
// key's pointer escapes, heap-allocates it too (a Spell is ~400 bytes, looked up per RegisterSpell).
func find[T any](table []T, id int32, key func(*T) int32) *T {
	lo, hi := 0, len(table)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if key(&table[mid]) < id {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == len(table) || key(&table[lo]) != id {
		return nil
	}
	return &table[lo]
}
