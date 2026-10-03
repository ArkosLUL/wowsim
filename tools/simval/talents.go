package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// TestSimvalTalentSweep records `.simval info` before a path's first talent and after each step.
// `simval talents` cuts a run's raw records down to talentsFixture, and TestTalentSweepRecords holds
// each step's sheet change to the sim's change for the same talent.

const (
	talentsFixture = "talents.jsonl"
	// the e2e's bots are named after its SvTsw account prefix: Svtlead plus letters
	talentSweepBotPrefix = "Svtlead"
)

// talentRecord is one line of talentsFixture: a sweep bot's info record cut down to its character and
// the sheet fields the replay compares, with the path it plays and how many of its steps are in.
type talentRecord struct {
	Path      string         `json:"path"`
	Step      int            `json:"step"`
	Bot       string         `json:"bot"`
	Class     uint8          `json:"class"`
	Race      uint8          `json:"race"`
	Talents   []int32        `json:"talents"` // rank spells, sorted
	Equipment []goldcharItem `json:"equipment"`

	Strength  float64 `json:"strength"`
	Agility   float64 `json:"agility"`
	Stamina   float64 `json:"stamina"`
	Intellect float64 `json:"intellect"`
	Spirit    float64 `json:"spirit"`
	// armor, then holy through arcane
	Resistances       []float64 `json:"resistances"`
	MaxHealth         float64   `json:"maxHealth"`
	PowerType         uint32    `json:"powerType"`
	MaxPower          float64   `json:"maxPower"`
	AttackPower       float64   `json:"attackPower"`
	RangedAttackPower float64   `json:"rangedAttackPower"`
	// holy through arcane
	SpellPower   []float64 `json:"spellPower"`
	SpellCrit    []float64 `json:"spellCrit"`
	HitMelee     float64   `json:"hitMelee"`
	HitRanged    float64   `json:"hitRanged"`
	HitSpell     float64   `json:"hitSpell"`
	CritMainhand float64   `json:"critMainhand"`
	CritOffhand  float64   `json:"critOffhand"`
	CritRanged   float64   `json:"critRanged"`
	// percent dodge and parry the hand's expertise takes off
	ExpertiseMainhand float64 `json:"expertiseMainhand"`
	ExpertiseOffhand  float64 `json:"expertiseOffhand"`
	// m_modAttackSpeedPct and UNIT_MOD_CAST_SPEED: attack and cast time multipliers, 1 / haste
	SpeedMainhand float64 `json:"speedMainhand"`
	SpeedRanged   float64 `json:"speedRanged"`
	CastSpeed     float64 `json:"castSpeed"`
	// the chances the combat table rolls: dodge and parry after diminishing returns, no parry without a
	// weapon, no block without a shield
	Dodge      float64 `json:"dodge"`
	Parry      float64 `json:"parry"`
	Block      float64 `json:"block"`
	BlockValue float64 `json:"blockValue"`
	// percent armor penetration the auras give; nil in a record from before the trim kept it
	ArmorPenPct *float64 `json:"armorPenetrationAuraPct,omitempty"`
	// percent chance to be missed by a mainhand, offhand and ranged attack; empty in an older record
	MissTaken []float64 `json:"missChanceTaken,omitempty"`
	Auras     []int32   `json:"auras"`
}

// rawTalentInfo is what the trim reads of the module's info record.
type rawTalentInfo struct {
	Command  string `json:"command"`
	Attacker struct {
		Name      string   `json:"name"`
		Class     uint8    `json:"class"`
		Race      uint8    `json:"race"`
		MaxHealth float64  `json:"maxHealth"`
		PowerType uint32   `json:"powerType"`
		MaxPower  *float64 `json:"maxPower"`
		Stats     struct {
			Strength  float64 `json:"strength"`
			Agility   float64 `json:"agility"`
			Stamina   float64 `json:"stamina"`
			Intellect float64 `json:"intellect"`
			Spirit    float64 `json:"spirit"`
		} `json:"stats"`
		Resistances []float64          `json:"resistances"`
		SpellPower  map[string]float64 `json:"spellPower"`
		HitMods     struct {
			Melee  float64 `json:"melee"`
			Ranged float64 `json:"ranged"`
			Spell  float64 `json:"spell"`
		} `json:"hitMods"`
		CastSpeedMultiplier     float64  `json:"castSpeedMultiplier"`
		ArmorPenetrationAuraPct *float64 `json:"armorPenetrationAuraPct"`
		Defense                 struct {
			Dodge float64 `json:"dodge"`
			Parry float64 `json:"parry"`
			Block float64 `json:"block"`
		} `json:"defense"`
		Sheet *struct {
			CritMainhand float64   `json:"critMainhand"`
			CritOffhand  float64   `json:"critOffhand"`
			CritRanged   float64   `json:"critRanged"`
			SpellCrit    []float64 `json:"spellCrit"`
			ShieldBlock  float64   `json:"shieldBlock"`
		} `json:"sheet"`
		Attacks []struct {
			Type               string   `json:"type"`
			AttackPower        float64  `json:"attackPower"`
			SpeedMultiplier    float64  `json:"speedMultiplier"`
			ExpertiseReduction float64  `json:"expertiseReduction"`
			MissChanceTaken    *float64 `json:"missChanceTaken"`
		} `json:"attacks"`
		Auras []struct {
			ID int32 `json:"id"`
		} `json:"auras"`
	} `json:"attacker"`
	Character *goldcharCharacter `json:"character"`
}

func (raw rawTalentInfo) project() (talentRecord, error) {
	a := raw.Attacker
	switch {
	case a.MaxPower == nil:
		return talentRecord{}, fmt.Errorf("%s: no maxPower, so the worldserver predates it", a.Name)
	case a.Sheet == nil || len(a.Sheet.SpellCrit) != len(magicSchools)+1:
		return talentRecord{}, fmt.Errorf("%s: no sheet with spell crit per school", a.Name)
	case len(a.Resistances) != len(magicSchools)+1:
		return talentRecord{}, fmt.Errorf("%s: %d resistances", a.Name, len(a.Resistances))
	}
	rec := talentRecord{
		Bot: a.Name, Class: a.Class, Race: a.Race, Talents: raw.Character.Talents,
		Strength: a.Stats.Strength, Agility: a.Stats.Agility, Stamina: a.Stats.Stamina,
		Intellect: a.Stats.Intellect, Spirit: a.Stats.Spirit,
		Resistances: a.Resistances, MaxHealth: a.MaxHealth, PowerType: a.PowerType, MaxPower: *a.MaxPower,
		SpellCrit: a.Sheet.SpellCrit[1:],
		HitMelee:  a.HitMods.Melee, HitRanged: a.HitMods.Ranged, HitSpell: a.HitMods.Spell,
		CritMainhand: a.Sheet.CritMainhand, CritOffhand: a.Sheet.CritOffhand, CritRanged: a.Sheet.CritRanged,
		CastSpeed: a.CastSpeedMultiplier,
		Dodge:     a.Defense.Dodge, Parry: a.Defense.Parry, Block: a.Defense.Block, BlockValue: a.Sheet.ShieldBlock,
		Equipment: []goldcharItem{}, Auras: []int32{},
	}
	if rec.Talents == nil {
		rec.Talents = []int32{}
	}
	for _, e := range raw.Character.Equipment {
		rec.Equipment = append(rec.Equipment, goldcharItem{Slot: e.Slot, ID: e.ItemID})
	}
	for _, school := range magicSchools {
		power, ok := a.SpellPower[school]
		if !ok {
			return talentRecord{}, fmt.Errorf("%s: no %s spell power", a.Name, school)
		}
		rec.SpellPower = append(rec.SpellPower, power)
	}
	rec.ArmorPenPct = a.ArmorPenetrationAuraPct
	seen := map[string]bool{}
	miss := map[string]*float64{}
	for _, attack := range a.Attacks {
		seen[attack.Type] = true
		miss[attack.Type] = attack.MissChanceTaken
		switch attack.Type {
		case "mainhand":
			rec.AttackPower, rec.SpeedMainhand, rec.ExpertiseMainhand = attack.AttackPower, attack.SpeedMultiplier, attack.ExpertiseReduction
		case "offhand":
			rec.ExpertiseOffhand = attack.ExpertiseReduction
		case "ranged":
			rec.RangedAttackPower, rec.SpeedRanged = attack.AttackPower, attack.SpeedMultiplier
		}
	}
	if !seen["mainhand"] || !seen["offhand"] || !seen["ranged"] {
		return talentRecord{}, fmt.Errorf("%s: attacks %v, want mainhand, offhand and ranged", a.Name, seen)
	}
	if miss["mainhand"] != nil && miss["offhand"] != nil && miss["ranged"] != nil {
		rec.MissTaken = []float64{*miss["mainhand"], *miss["offhand"], *miss["ranged"]}
	}
	for _, aura := range a.Auras {
		rec.Auras = append(rec.Auras, aura.ID)
	}
	return rec, nil
}

// stepOf is how many of the path's steps a character with these talents has learned, -1 for talents
// that aren't a start of the path.
func (path talentPath) stepOf(talents []int32) int {
	n := len(talents)
	if n > len(path.Steps) {
		return -1
	}
	learned := make([]int32, n)
	for i, step := range path.Steps[:n] {
		learned[i] = step.Spell
	}
	slices.Sort(learned)
	if !slices.Equal(learned, talents) {
		return -1
	}
	return n
}

// wears is whether the items are exactly the path's.
func (path talentPath) wears(items []goldcharItem) bool {
	return len(items) == len(path.Items) &&
		!slices.ContainsFunc(items, func(item goldcharItem) bool { return !slices.Contains(path.Items, item) })
}

// fits is whether the record can be one of the path's.
func (path talentPath) fits(rec talentRecord) bool {
	return rec.Class == path.Class && rec.Race == path.Race && path.wears(rec.Equipment) && path.stepOf(rec.Talents) >= 0
}

// trimTalentRecords picks the sweep bots' info records out of raw simval lines, the worldserver's
// simval.jsonl as streamed or an e2e log's RECORD lines, and puts each on its path and step. A bot is
// left out when its records fit no path (the e2e stopped it on a wrong talent) or more than one (it
// never got past the first talent), and a path several bots recorded keeps the last, a rerun's. Notes
// say what was left out.
func trimTalentRecords(r io.Reader, paths []talentPath) (records []talentRecord, notes []string, err error) {
	type bot struct {
		name    string
		records []talentRecord
	}
	var bots []*bot
	byName := map[string]*bot{}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if i := strings.Index(text, "RECORD {"); i >= 0 {
			text = text[i+len("RECORD "):]
		}
		if !strings.HasPrefix(text, "{") {
			continue
		}
		var raw rawTalentInfo
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			return nil, nil, fmt.Errorf("line %d: %w", line, err)
		}
		if raw.Command != "info" || raw.Character == nil || !strings.HasPrefix(raw.Attacker.Name, talentSweepBotPrefix) {
			continue
		}
		rec, err := raw.project()
		if err != nil {
			return nil, nil, fmt.Errorf("line %d: %w", line, err)
		}
		b := byName[rec.Bot]
		if b == nil {
			b = &bot{name: rec.Bot}
			byName[b.name] = b
			bots = append(bots, b)
		}
		b.records = append(b.records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	byPath := map[string][]talentRecord{}
	for _, b := range bots {
		var fits []talentPath
		for _, path := range paths {
			if !slices.ContainsFunc(b.records, func(rec talentRecord) bool { return !path.fits(rec) }) {
				fits = append(fits, path)
			}
		}
		switch {
		case len(fits) == 0:
			last := b.records[len(b.records)-1]
			notes = append(notes, fmt.Sprintf("%s left out: class %d, race %d, items %v and talents %v fit no path",
				b.name, last.Class, last.Race, last.Equipment, last.Talents))
			continue
		case len(fits) > 1:
			var names []string
			for _, path := range fits {
				names = append(names, path.Name)
			}
			notes = append(notes, fmt.Sprintf("%s left out: its %d records fit %v", b.name, len(b.records), names))
			continue
		}
		path := fits[0]
		steps := map[int]bool{}
		recs := make([]talentRecord, len(b.records))
		for i, rec := range b.records {
			rec.Path, rec.Step = path.Name, path.stepOf(rec.Talents)
			if steps[rec.Step] {
				return nil, nil, fmt.Errorf("%s: two records after step %d of %s", b.name, rec.Step, path.Name)
			}
			steps[rec.Step] = true
			recs[i] = rec
		}
		if earlier := byPath[path.Name]; earlier != nil {
			notes = append(notes, fmt.Sprintf("%s: %s's records replace %s's", path.Name, b.name, earlier[0].Bot))
		}
		byPath[path.Name] = recs
	}

	for _, path := range paths {
		recs := byPath[path.Name]
		slices.SortFunc(recs, func(a, b talentRecord) int { return cmp.Compare(a.Step, b.Step) })
		records = append(records, recs...)
	}
	return records, notes, nil
}

// keepOtherPaths adds the old fixture's records for the paths the new run has none for, so a rerun of
// some paths keeps the rest. A path either run left out is gone from the paths file.
func keepOtherPaths(fresh, old []talentRecord, paths []talentPath) (records []talentRecord, notes []string) {
	byPath, rerun := map[string][]talentRecord{}, map[string]bool{}
	for _, rec := range fresh {
		byPath[rec.Path] = append(byPath[rec.Path], rec)
		rerun[rec.Path] = true
	}
	kept, gone := map[string]int{}, map[string]bool{}
	for _, rec := range old {
		switch {
		case !slices.ContainsFunc(paths, func(p talentPath) bool { return p.Name == rec.Path }):
			gone[rec.Path] = true
		case !rerun[rec.Path]:
			byPath[rec.Path] = append(byPath[rec.Path], rec)
			kept[rec.Path]++
		}
	}
	var keptPaths []string
	for _, path := range paths {
		recs := byPath[path.Name]
		slices.SortFunc(recs, func(a, b talentRecord) int { return cmp.Compare(a.Step, b.Step) })
		records = append(records, recs...)
		if n := kept[path.Name]; n > 0 {
			keptPaths = append(keptPaths, fmt.Sprintf("%s (%d)", path.Name, n))
		}
	}
	if len(keptPaths) > 0 {
		notes = append(notes, "kept the old fixture's records of "+strings.Join(keptPaths, ", "))
	}
	for _, name := range slices.Sorted(maps.Keys(gone)) {
		notes = append(notes, fmt.Sprintf("%s left out of the old fixture: no such path now", name))
	}
	return records, notes
}

func marshalTalentRecords(records []talentRecord) ([]byte, error) {
	var b bytes.Buffer
	for _, rec := range records {
		line, err := json.Marshal(rec)
		if err != nil {
			return nil, err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.Bytes(), nil
}

// runTalentTrim is `simval talents`: it writes the trimmed records, the old fixture's for paths the run
// didn't record, and says which steps lack one.
func runTalentTrim(args []string) int {
	flags := flag.NewFlagSet("simval talents", flag.ExitOnError)
	// no default: the e2e deletes its bots' records from the worldserver's simval.jsonl when they finish
	recordsPath := flags.String("records", "", "raw simval records streamed during the run, or an e2e log with RECORD lines")
	out := flags.String("out", "", "the trimmed file (default the checkout's "+fixtureDir+"/"+talentsFixture+")")
	_ = flags.Parse(args)
	if *recordsPath == "" {
		fmt.Fprintln(os.Stderr, "simval talents: -records is required")
		return 2
	}

	root, err := checkoutRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *out == "" {
		*out = filepath.Join(root, fixtureDir, talentsFixture)
	}
	paths, err := readTalentPaths(filepath.Join(root, fixtureDir, talentPathsFile))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f, err := os.Open(*recordsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer f.Close()
	records, notes, err := trimTalentRecords(f, paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *recordsPath, err)
		return 1
	}
	if len(records) == 0 {
		for _, note := range notes {
			fmt.Println(note)
		}
		fmt.Fprintf(os.Stderr, "%s has no sweep bot's info records; %s is left as it was\n", *recordsPath, *out)
		return 1
	}
	old, err := readJSONLines[talentRecord](*out)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	records, keptNotes := keepOtherPaths(records, old, paths)
	for _, note := range append(notes, keptNotes...) {
		fmt.Println(note)
	}
	data, err := marshalTalentRecords(records)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	have := map[string][]int{}
	for _, rec := range records {
		have[rec.Path] = append(have[rec.Path], rec.Step)
	}
	complete := 0
	for _, path := range paths {
		var missing []int
		for step := 0; step <= len(path.Steps); step++ {
			if !slices.Contains(have[path.Name], step) {
				missing = append(missing, step)
			}
		}
		switch {
		case len(missing) == 0:
			complete++
		case len(have[path.Name]) == 0:
			fmt.Printf("%s: no records\n", path.Name)
		default:
			fmt.Printf("%s: no record after steps %v\n", path.Name, missing)
		}
	}
	fmt.Printf("wrote %d records to %s; %d of %d paths complete\n", len(records), *out, complete, len(paths))
	return 0
}

// sweepFields are the sheet fields a step is compared on, in report order, each with what the server's
// own truncation can take off it: primary stats, health, power, armor, resistances (one more for a
// resistance from a stat) and block value are stored whole, each stat or attack power based spell power
// bonus is truncated before they're added up, and a rating from a stat (a death knight's parry from
// strength) is a whole rating point. Armor penetration from auras is a whole percent per rank on both
// sides, school crit and the chance to be missed come from the same ratings on both sides, so those
// take nothing off. The rest of a field's bound comes from what it's computed from
// (sweepSources).
var sweepFields = []struct {
	name  string
	slack float64
}{
	{"strength", 0}, {"agility", 0}, {"stamina", 0}, {"intellect", 0}, {"spirit", 0},
	{"max health", 1}, {"max mana", 1}, {"max rage", 1}, {"max energy", 1}, {"max runic power", 1},
	{"armor", 1}, {"fire resistance", 2}, {"nature resistance", 2}, {"frost resistance", 2},
	{"shadow resistance", 2}, {"arcane resistance", 2},
	{"attack power", 0}, {"ranged attack power", 0}, {"spell power", 3},
	{"mainhand crit %", 0}, {"offhand crit %", 0}, {"ranged crit %", 0}, {"spell crit %", 0},
	{"holy spell crit %", 0}, {"fire spell crit %", 0}, {"nature spell crit %", 0}, {"frost spell crit %", 0},
	{"shadow spell crit %", 0}, {"arcane spell crit %", 0},
	{"melee hit %", 0}, {"ranged hit %", 0}, {"spell hit %", 0},
	{"mainhand expertise %", 0}, {"offhand expertise %", 0},
	{"melee haste", 0}, {"ranged haste", 0}, {"spell haste", 0},
	{"dodge %", 0}, {"parry %", 1 / core.ParryRatingPerParryChance}, {"block %", 0}, {"block value", 1},
	{"armor penetration %", 0}, {"mainhand miss taken %", 0}, {"offhand miss taken %", 0}, {"ranged miss taken %", 0},
}

// sweepSources are the numbers the server truncates that sheet fields are computed from: primary
// stats to whole points, and attack power's base and mods to whole points before its multiplier, the
// mods twice with Bladed Armor's share. A field's bound gets the sim's change per truncated unit of
// each, times how many units the truncations can take off. Field names a stat's sheet number, whose
// points the rate is per; attack power's is per raw point.
var sweepSources = []struct {
	stat   stats.Stat
	field  string
	weight float64
}{
	{stats.Strength, "strength", 1}, {stats.Agility, "agility", 1}, {stats.Stamina, "stamina", 1},
	{stats.Intellect, "intellect", 1}, {stats.Spirit, "spirit", 1},
	{stats.AttackPower, "", 3}, {stats.RangedAttackPower, "", 3},
}

// sweepEpsilon is the float noise any comparison allows, and what a field has to move by to count as
// moved.
const sweepEpsilon = 1e-3

// sweepAttacks are the attacks whose chance to be missed the record has, in its order.
var sweepAttacks = []string{"mainhand", "offhand", "ranged"}

var sweepResistances = []string{"fire resistance", "nature resistance", "frost resistance", "shadow resistance", "arcane resistance"}

// sweepPowerFields name the max power of each server power type, with the server's units per point.
var sweepPowerFields = map[uint32]struct {
	name  string
	scale float64
}{
	0: {"max mana", 1}, 1: {"max rage", 10}, 3: {"max energy", 1}, 6: {"max runic power", 10},
}

// sweepSheet is a sheet's numbers by sweepFields name.
type sweepSheet map[string]float64

// sheet is the record's numbers in the sim's units. Spell power and "spell crit %" are the lowest school's,
// what every spell gets, while each "<school> spell crit %" is that school's own. Spell hit adds the
// school hit every magic school gets, which the server's sheet leaves out.
func (r talentRecord) sheet() (sweepSheet, error) {
	if len(r.Resistances) != len(magicSchools)+1 || len(r.SpellPower) != len(magicSchools) ||
		len(r.SpellCrit) != len(magicSchools) {
		return nil, fmt.Errorf("%d resistances, %d spell powers and %d spell crits", len(r.Resistances),
			len(r.SpellPower), len(r.SpellCrit))
	}
	unit := unitSnapshot{}
	for _, id := range r.Auras {
		unit.Auras = append(unit.Auras, aura{ID: id})
	}
	schoolHit, err := spellHitEverySchool(unit)
	if err != nil {
		return nil, err
	}
	v := sweepSheet{
		"strength": r.Strength, "agility": r.Agility, "stamina": r.Stamina, "intellect": r.Intellect, "spirit": r.Spirit,
		"max health": r.MaxHealth, "armor": r.Resistances[0],
		"attack power": r.AttackPower, "ranged attack power": r.RangedAttackPower,
		"spell power": slices.Min(r.SpellPower), "spell crit %": slices.Min(r.SpellCrit),
		"mainhand crit %": r.CritMainhand, "offhand crit %": r.CritOffhand, "ranged crit %": r.CritRanged,
		"melee hit %": r.HitMelee, "ranged hit %": r.HitRanged, "spell hit %": r.HitSpell + schoolHit,
		"mainhand expertise %": r.ExpertiseMainhand, "offhand expertise %": r.ExpertiseOffhand,
		"dodge %": r.Dodge, "parry %": r.Parry, "block %": r.Block, "block value": r.BlockValue,
	}
	// the server's resistances go holy, fire, nature, frost, shadow, arcane after armor; the sim has no holy
	for i, name := range sweepResistances {
		v[name] = r.Resistances[2+i]
	}
	for i, school := range magicSchools {
		v[school+" spell crit %"] = r.SpellCrit[i]
	}
	if r.ArmorPenPct != nil {
		v["armor penetration %"] = *r.ArmorPenPct
	}
	if len(r.MissTaken) == len(sweepAttacks) {
		for i, attack := range sweepAttacks {
			v[attack+" miss taken %"] = r.MissTaken[i]
		}
	}
	for name, speed := range map[string]float64{"melee haste": r.SpeedMainhand, "ranged haste": r.SpeedRanged, "spell haste": r.CastSpeed} {
		if speed > 0 {
			v[name] = 1 / speed
		}
	}
	if power, ok := sweepPowerFields[r.PowerType]; ok {
		v[power.name] = r.MaxPower / power.scale
	}
	return v, nil
}

// sweepTarget is the sweep's boss dummy, for a sim of the path's character.
func sweepTarget(path talentPath) damageRecord {
	var rec damageRecord
	rec.Command = "info"
	rec.Attacker.Class, rec.Attacker.Race, rec.Attacker.Level, rec.Attacker.IsPlayer = path.Class, path.Race, 80, true
	rec.Target.Entry, rec.Target.Level, rec.Target.WorldBoss, rec.Target.CreatureType = 999000, 83, true, 5
	return rec
}

// sweepSim is the sim's sheet for a character, with each field's bound: how far below the sim's number
// the server's truncations can put its own.
type sweepSim struct {
	values, bounds sweepSheet
	err            error
}

// sweepSim is the sheet of the path's character after its first n steps, at the sweep's dummy.
func (path talentPath) sweepSim(n int) sweepSim {
	s, err := path.goldcharSetup(n)
	if err != nil {
		return sweepSim{err: err}
	}
	return newSweepSim(sweepTarget(path), s)
}

// newSweepSim is the sheet of the setup's character against the record's dummy.
func newSweepSim(rec damageRecord, s goldcharSetup) sweepSim {
	d, err := newGoldcharSim(rec, s)
	if err != nil {
		return sweepSim{err: err}
	}
	return d.sweepSim()
}

// sweepSim is the built character's sheet with its bounds, after a start.
func (d *damageSim) sweepSim() (out sweepSim) {
	defer func() {
		if r := recover(); r != nil {
			out = sweepSim{err: fmt.Errorf("sim panic: %v", r)}
		}
	}()
	d.start()
	values := d.sweepSheet()
	bounds := sweepSheet{}
	for _, f := range sweepFields {
		if _, ok := values[f.name]; ok {
			bounds[f.name] = f.slack
		}
	}
	for _, src := range sweepSources {
		var bonus stats.Stats
		bonus[src.stat] = 1
		d.char.AddStatsDynamic(d.sim, bonus)
		moved := d.sweepSheet()
		d.char.AddStatsDynamic(d.sim, bonus.Invert())
		per := 1.0
		if src.field != "" {
			per = moved[src.field] - values[src.field]
		}
		if per < 1e-6 {
			continue
		}
		for name, value := range values {
			bounds[name] += src.weight * math.Abs(moved[name]-value) / per
		}
	}
	return sweepSim{values: values, bounds: bounds}
}

// sweepSheet is the sim character's sheet in sweepFields. A field the server shows but means nothing
// for this character stays out: off hand and ranged numbers where the sim doesn't swing, parry without a
// weapon, block without a shield, power bars the class lacks.
func (d *damageSim) sweepSheet() sweepSheet {
	c := d.char
	s := c.GetStats()
	v := sweepSheet{
		"strength": s[stats.Strength], "agility": s[stats.Agility], "stamina": s[stats.Stamina],
		"intellect": s[stats.Intellect], "spirit": s[stats.Spirit],
		"max health": s[stats.Health], "armor": c.Armor(),
		"fire resistance": s[stats.FireResistance], "nature resistance": s[stats.NatureResistance],
		"frost resistance": s[stats.FrostResistance], "shadow resistance": s[stats.ShadowResistance],
		"arcane resistance": s[stats.ArcaneResistance],
		"attack power":      s[stats.AttackPower], "ranged attack power": s[stats.RangedAttackPower],
		"spell power": s[stats.SpellPower], "spell crit %": s[stats.SpellCrit] / core.CritRatingPerCritChance,
		"spell hit %": s[stats.SpellHit] / core.SpellHitRatingPerHitChance,
		// bare-handed, the sim may have no main hand swing to read bonuses off
		"mainhand crit %": s[stats.MeleeCrit] / core.CritRatingPerCritChance,
		"melee hit %":     s[stats.MeleeHit] / core.MeleeHitRatingPerHitChance,
		"melee haste":     c.SwingSpeed(), "ranged haste": c.RangedSwingSpeed(), "spell haste": 1 / c.CastSpeed,
		"dodge %":             c.DodgeChance() * 100,
		"armor penetration %": c.PseudoStats.BonusArmorPenPct,
	}
	// the server's 5 base, then what defense and auras take off an attacker's hit
	miss := 5 + (c.DefenseMissChance()+c.PseudoStats.ReducedPhysicalHitTakenChance)*100
	for _, attack := range sweepAttacks {
		v[attack+" miss taken %"] = miss
		if attack != "ranged" {
			v[attack+" miss taken %"] += c.PseudoStats.ReducedMeleeHitTakenChance * 100
		}
	}
	for i, school := range magicSchools {
		if bonus, ok := d.schoolCritBonus(sweepSchools[i]); ok {
			v[school+" spell crit %"] = (s[stats.SpellCrit] + bonus) / core.CritRatingPerCritChance
		}
	}
	for _, hand := range d.sheetHands() {
		crit := (s[stats.MeleeCrit] + hand.spell.BonusCritRating) / core.CritRatingPerCritChance
		hit := (s[stats.MeleeHit] + hand.spell.BonusHitRating) / core.MeleeHitRatingPerHitChance
		expertise := hand.spell.ExpertisePercentage() * 100
		switch hand.attack {
		case "mainhand":
			v["mainhand crit %"], v["melee hit %"], v["mainhand expertise %"] = crit, hit, expertise
		case "offhand":
			v["offhand crit %"], v["offhand expertise %"] = crit, expertise
		case "ranged":
			v["ranged crit %"], v["ranged hit %"] = crit, hit
		}
	}
	mainHand, offHand := c.Equipment[proto.ItemSlot_ItemSlotMainHand], c.Equipment[proto.ItemSlot_ItemSlotOffHand]
	if mainHand.ID != 0 || offHand.ID != 0 && offHand.WeaponType != proto.WeaponType_WeaponTypeShield {
		v["parry %"] = c.ParryChance() * 100
	}
	if c.PseudoStats.CanBlock {
		defense := float64(core.DefenseSkillFromRating(s[stats.Defense])) * core.PercentPerSkillPoint
		v["block %"] = 5 + s[stats.Block]/core.BlockRatingPerBlockChance + defense
		v["block value"] = c.BlockValue()
	}
	if c.HasManaBar() {
		v["max mana"] = s[stats.Mana]
	}
	if c.HasRageBar() {
		v["max rage"] = core.MaxRage
	}
	if top, ok := barMaximum(&c.Unit, "maxEnergy"); ok && c.HasEnergyBar() {
		v["max energy"] = top
	}
	if top, ok := barMaximum(&c.Unit, "maxRunicPower"); ok && c.HasRunicPowerBar() {
		v["max runic power"] = top
	}
	return v
}

// sweepSchools are magicSchools' sim schools.
var sweepSchools = []core.SpellSchool{core.SpellSchoolHoly, core.SpellSchoolFire, core.SpellSchoolNature,
	core.SpellSchoolFrost, core.SpellSchoolShadow, core.SpellSchoolArcane}

// schoolCritBonus is the crit rating every damage or healing spell of just this school gets on top of
// the stat. The sim keeps school crit talents on each spell, so the lowest of them stands for the
// school. False when the character has no such spell.
func (d *damageSim) schoolCritBonus(school core.SpellSchool) (float64, bool) {
	found, lowest := false, math.Inf(1)
	for _, spell := range d.char.Spellbook {
		if spell.SpellSchool == school && spell.CritMultiplier > 0 &&
			spell.ProcMask.Matches(core.ProcMaskSpellDamage|core.ProcMaskSpellHealing) {
			found, lowest = true, min(lowest, spell.BonusCritRating)
		}
	}
	return lowest, found
}

// barMaximum reads a power bar's maximum, which the sim keeps unexported.
func barMaximum(unit *core.Unit, field string) (float64, bool) {
	v := reflect.ValueOf(unit).Elem().FieldByName(field)
	if !v.IsValid() || v.Kind() != reflect.Float64 {
		return 0, false
	}
	return v.Float(), true
}

const (
	sweepAgrees    = "agrees"
	sweepNoField   = "moves no field"
	sweepDisagrees = "disagrees"
)

// sweepStep is how a step's sheet change compares: a check per field both sides show, and the fields
// that moved on either side.
type sweepStep struct {
	class  string
	checks []Check
	moved  []string
}

// compareSweepStep holds the server's change between two records to the sim's between the same two
// characters. A field passes when the two changes differ by no more than the larger of the two sim
// sheets' bounds, plus float noise.
func compareSweepStep(before, after talentRecord, simBefore, simAfter sweepSim) sweepStep {
	if err := cmp.Or(simBefore.err, simAfter.err); err != nil {
		return sweepStep{class: sweepDisagrees, checks: []Check{fail("sim character", err.Error())}}
	}
	serverBefore, err := before.sheet()
	if err == nil {
		var serverAfter sweepSheet
		if serverAfter, err = after.sheet(); err == nil {
			return compareSweepSheets(serverBefore, serverAfter, simBefore, simAfter)
		}
	}
	return sweepStep{class: sweepDisagrees, checks: []Check{fail("server record", err.Error())}}
}

func compareSweepSheets(serverBefore, serverAfter sweepSheet, simBefore, simAfter sweepSim) sweepStep {
	step := sweepStep{class: sweepNoField}
	for _, f := range sweepFields {
		sb, ok1 := serverBefore[f.name]
		sa, ok2 := serverAfter[f.name]
		mb, ok3 := simBefore.values[f.name]
		ma, ok4 := simAfter.values[f.name]
		if !ok1 || !ok2 || !ok3 || !ok4 {
			continue
		}
		server, sim := sa-sb, ma-mb
		tolerance := max(simBefore.bounds[f.name], simAfter.bounds[f.name]) + sweepEpsilon
		detail := fmt.Sprintf("server %+.4g, sim %+.4g (within %.3g)", server, sim, tolerance)
		if math.Abs(server-sim) <= tolerance {
			step.checks = append(step.checks, pass(f.name, detail))
		} else {
			step.checks = append(step.checks, fail(f.name, detail))
			step.class = sweepDisagrees
		}
		if math.Abs(server) > sweepEpsilon || math.Abs(sim) > sweepEpsilon {
			step.moved = append(step.moved, fmt.Sprintf("%s server %+.4g sim %+.4g", f.name, server, sim))
			if step.class == sweepNoField {
				step.class = sweepAgrees
			}
		}
	}
	return step
}
