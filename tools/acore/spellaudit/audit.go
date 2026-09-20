package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

// maxRefs is how many source positions a row carries. A spell named in a dozen
// places only needs enough to find it.
const maxRefs = 3

// row is one sim-named spell: what the server holds for it and where the sim
// names it.
type row struct {
	Areas   []string
	Refs    []spellset.Ref
	SpellID int32
	Dump    *spellset.DumpSpell
	Gen     *serverdata.Spell
	Proc    *serverdata.Proc
}

type audit struct {
	Rows []row
	// Missing are sim-named ids the capture lacks: the sim asks the server for a
	// spell it has never seen.
	Missing    []row
	Unresolved []spellset.Ref
	TypeErrors []string
}

func buildAudit(literals *spellset.Literals, capture spellset.Dump, area string) *audit {
	result := &audit{Unresolved: literals.Unresolved, TypeErrors: literals.TypeErrors}

	ids := make([]int32, 0, len(literals.Spells))
	for id := range literals.Spells {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	for _, id := range ids {
		refs := literals.Spells[id]
		entry := row{
			SpellID: id,
			Areas:   areasOf(refs),
			Refs:    refs,
			Dump:    capture[id],
			Gen:     serverdata.SpellByID(id),
			Proc:    serverdata.ProcBySpellID(id),
		}
		if area != "" && !slices.Contains(entry.Areas, area) {
			continue
		}
		if entry.Dump == nil {
			result.Missing = append(result.Missing, entry)
			continue
		}
		result.Rows = append(result.Rows, entry)
	}
	return result
}

// areasOf names the parts of the sim a spell is used from: the class directory
// under sim/, or the deeper path for core and the shared item code.
func areasOf(refs []spellset.Ref) []string {
	seen := map[string]bool{}
	var areas []string
	for _, ref := range refs {
		area := areaOf(ref.File)
		if area != "" && !seen[area] {
			seen[area] = true
			areas = append(areas, area)
		}
	}
	sort.Strings(areas)
	return areas
}

func areaOf(file string) string {
	parts := strings.Split(filepath.ToSlash(file), "/")
	if len(parts) < 2 || parts[0] != "sim" {
		return ""
	}
	if parts[1] == "common" && len(parts) > 2 {
		return "common/" + parts[2]
	}
	return parts[1]
}

func (a *audit) write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := []struct {
		name  string
		write func(*csv.Writer) error
	}{
		{"spells_audit.csv", a.writeSpells},
		{"procs_audit.csv", a.writeProcs},
		{"spells_missing.csv", a.writeMissing},
	}
	for _, file := range files {
		if err := writeCSV(filepath.Join(dir, file.name), file.write); err != nil {
			return err
		}
	}
	return nil
}

func writeCSV(path string, write func(*csv.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := write(w); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func (a *audit) writeSpells(w *csv.Writer) error {
	header := []string{
		"area", "spell_id", "name", "rank", "family", "dmg_class", "school_mask",
		"cast_ms", "base_cast_ms", "gcd_ms", "gcd_category", "cooldown_ms", "category_cooldown_ms",
		"duration_ms", "max_duration_ms", "stacks", "tick_ms", "speed", "max_targets",
		"binary", "channeled", "ranged_slot", "auto_repeat", "resets_swing", "haste_gcd",
		"haste_periodic", "no_active_defense", "always_hit", "positive", "passive",
		"proc_ppm", "proc_chance", "proc_icd_ms", "triggers", "scripts", "sim_refs",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, entry := range a.Rows {
		dump := entry.Dump
		if err := w.Write([]string{
			strings.Join(entry.Areas, " "),
			itoa(entry.SpellID),
			dump.Name,
			dump.Rank,
			itoa64(dump.Family),
			itoa64(dump.DmgClass),
			itoa64(dump.SchoolMask),
			itoa64(dump.CastTimeMs),
			itoa64(dump.CastTimeBaseMs),
			itoa64(dump.StartRecoveryTimeMs),
			itoa64(dump.StartRecoveryCategory),
			itoa64(dump.RecoveryTimeMs),
			itoa64(dump.CategoryRecoveryTimeMs),
			itoa64(dump.DurationMs),
			itoa64(dump.MaxDurationMs),
			itoa64(dump.StackAmount),
			itoa64(tickMs(dump)),
			ftoa(dump.Speed),
			itoa64(dump.MaxAffectedTargets),
			btoa(dump.Binary),
			btoa(dump.Channeled),
			flagOf(entry.Gen, serverdata.FlagUsesRangedSlot),
			flagOf(entry.Gen, serverdata.FlagAutoRepeat),
			flagOf(entry.Gen, serverdata.FlagResetsAutoAttack),
			flagOf(entry.Gen, serverdata.FlagHasteGCD),
			flagOf(entry.Gen, serverdata.FlagHasteAffectsPeriodic),
			flagOf(entry.Gen, serverdata.FlagNoActiveDefense),
			flagOf(entry.Gen, serverdata.FlagAlwaysHit),
			btoa(dump.Positive),
			btoa(dump.Passive),
			procPPM(entry.Proc),
			procChance(entry.Proc),
			procICD(entry.Proc),
			joinIDs(dump.Triggers()),
			strings.Join(dump.Scripts, " "),
			refsOf(entry.Refs),
		}); err != nil {
			return err
		}
	}
	return nil
}

// writeProcs is the subset with a spell_proc entry, the rows P7 and the item work
// read when they wire a proc up.
func (a *audit) writeProcs(w *csv.Writer) error {
	header := []string{
		"area", "spell_id", "name", "row", "ppm", "chance", "icd_ms", "charges",
		"proc_flags", "spell_type_mask", "spell_phase_mask", "hit_mask", "attributes",
		"school_mask", "family", "family_mask", "sim_refs",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, entry := range a.Rows {
		if entry.Proc == nil {
			continue
		}
		proc := entry.Proc
		if err := w.Write([]string{
			strings.Join(entry.Areas, " "),
			itoa(entry.SpellID),
			entry.Dump.Name,
			itoa(proc.Row),
			ftoa(proc.ProcsPerMinute),
			ftoa(proc.Chance),
			itoa(proc.CooldownMs),
			itoa(proc.Charges),
			hex(uint64(proc.ProcFlags)),
			hex(uint64(proc.SpellTypeMask)),
			hex(uint64(proc.SpellPhaseMask)),
			hex(uint64(proc.HitMask)),
			procAttributes(proc.AttributesMask),
			hex(uint64(proc.SchoolMask)),
			itoa(proc.SpellFamilyName),
			fmt.Sprintf("%#x %#x %#x", proc.SpellFamilyMask[0], proc.SpellFamilyMask[1], proc.SpellFamilyMask[2]),
			refsOf(entry.Refs),
		}); err != nil {
			return err
		}
	}
	return nil
}

// writeMissing lists what the audit could not answer: ids the capture lacks and
// the places the scan could not resolve to a constant.
func (a *audit) writeMissing(w *csv.Writer) error {
	if err := w.Write([]string{"kind", "spell_id", "sim_refs"}); err != nil {
		return err
	}
	for _, entry := range a.Missing {
		if err := w.Write([]string{"not captured", itoa(entry.SpellID), refsOf(entry.Refs)}); err != nil {
			return err
		}
	}
	for _, ref := range a.Unresolved {
		if err := w.Write([]string{"not a constant", "", ref.String()}); err != nil {
			return err
		}
	}
	for _, problem := range a.TypeErrors {
		if err := w.Write([]string{"type error", "", problem}); err != nil {
			return err
		}
	}
	return nil
}

func (a *audit) summarize(out io.Writer) {
	counts := map[string]int{}
	procs := map[string]int{}
	for _, entry := range a.Rows {
		for _, area := range entry.Areas {
			counts[area]++
			if entry.Proc != nil {
				procs[area]++
			}
		}
	}

	areas := make([]string, 0, len(counts))
	for area := range counts {
		areas = append(areas, area)
	}
	sort.Strings(areas)

	fmt.Fprintf(out, "%-16s %8s %8s\n", "area", "spells", "procs")
	for _, area := range areas {
		fmt.Fprintf(out, "%-16s %8d %8d\n", area, counts[area], procs[area])
	}
	fmt.Fprintf(out, "\n%d spells audited, %d not captured, %d ids the scan could not resolve\n",
		len(a.Rows), len(a.Missing), len(a.Unresolved))
	if len(a.TypeErrors) > 0 {
		fmt.Fprintf(out, "%d type errors: the constants behind them are missing from the audit\n", len(a.TypeErrors))
	}
}

// tickMs is the periodic interval of the spell's first periodic effect, which is
// what a DoT or HoT ticks on.
func tickMs(spell *spellset.DumpSpell) int64 {
	for _, effect := range spell.Effects {
		if effect.AmplitudeMs > 0 {
			return effect.AmplitudeMs
		}
	}
	return 0
}

func flagOf(spell *serverdata.Spell, flag serverdata.Flags) string {
	if spell == nil {
		return ""
	}
	return btoa(spell.Flags&flag != 0)
}

func procPPM(proc *serverdata.Proc) string {
	if proc == nil {
		return ""
	}
	return ftoa(proc.ProcsPerMinute)
}

func procChance(proc *serverdata.Proc) string {
	if proc == nil {
		return ""
	}
	return ftoa(proc.Chance)
}

func procICD(proc *serverdata.Proc) string {
	if proc == nil {
		return ""
	}
	return itoa(proc.CooldownMs)
}

// procAttributes spells out the PROC_ATTR_* bits, since the raw mask says nothing
// on its own.
func procAttributes(mask uint32) string {
	named := []struct {
		bit  uint32
		name string
	}{
		{serverdata.ProcAttrReqExpOrHonor, "req_exp_or_honor"},
		{serverdata.ProcAttrTriggeredCanProc, "triggered_can_proc"},
		{serverdata.ProcAttrReqManaCost, "req_mana_cost"},
		{serverdata.ProcAttrReqSpellmod, "req_spellmod"},
		{serverdata.ProcAttrUseStacksForCharges, "use_stacks_for_charges"},
		{serverdata.ProcAttrReduceProc60, "reduce_proc_60"},
		{serverdata.ProcAttrCantProcFromItemCast, "cant_proc_from_item_cast"},
	}

	var names []string
	rest := mask
	for _, attr := range named {
		if mask&attr.bit != 0 {
			names = append(names, attr.name)
			rest &^= attr.bit
		}
	}
	if rest != 0 {
		names = append(names, hex(uint64(rest)))
	}
	return strings.Join(names, " ")
}

func refsOf(refs []spellset.Ref) string {
	texts := make([]string, 0, maxRefs)
	for i, ref := range refs {
		if i == maxRefs {
			texts = append(texts, fmt.Sprintf("+%d more", len(refs)-maxRefs))
			break
		}
		texts = append(texts, ref.String())
	}
	return strings.Join(texts, " ")
}

func joinIDs(ids []int32) string {
	texts := make([]string, len(ids))
	for i, id := range ids {
		texts[i] = itoa(id)
	}
	return strings.Join(texts, " ")
}

func itoa(v int32) string   { return strconv.Itoa(int(v)) }
func itoa64(v int64) string { return strconv.FormatInt(v, 10) }
func hex(v uint64) string   { return "0x" + strconv.FormatUint(v, 16) }

// btoa prints a boolean the way a reviewer scanning a column wants it. An empty
// cell means the audit does not know, never "no".
func btoa(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// ftoa keeps the decimal the DB holds: a float32 0.7 prints as 0.7, not as the
// 0.699999988079071 it widens to.
func ftoa(v float32) string {
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(float64(v), 'g', -1, 32)
}
