package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// mod-sim-validation's TestSimvalSpellModSweep walks the talent sweep's paths and, before and after each
// talent of spellModSpellsFile, probes every spell the talent's mods change: `.simval timing`, `.simval
// damage` and `.simval yellow <id> 1`. Each spell's probes at one point of a path are a SPELLMOD line of its
// log, collected in spellModFixture. TestSpellModRecords holds each probed number's change across a talent
// to the sim's change between the same two characters.

const (
	spellModFixture    = "spellmods.jsonl"
	spellModSpellsFile = "spellmod_spells.json"
)

// spellModGroup is a group of a talent's ranks in spellModSpellsFile, which tools/acore/spellmods writes:
// ranks with the same mods and the spells the sim names that those change on the server.
type spellModGroup struct {
	Ranks  []int32 `json:"ranks"`
	Spells []struct {
		ID   int32    `json:"id"`
		Name string   `json:"name"`
		Ops  []string `json:"ops"`
	} `json:"spells"`
}

// spellModStep is what the list says about a path step's rank spell. A cast talent (Presence of Mind)
// changes its spells only while its aura is up, so the e2e casts it for the after probes.
type spellModStep struct {
	passive bool
	group   spellModGroup
}

// readSpellModList maps every rank spell of the list to its step.
func readSpellModList(path string) (map[int32]spellModStep, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list struct {
		Talents []struct {
			Passive bool            `json:"passive"`
			Groups  []spellModGroup `json:"groups"`
		} `json:"talents"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	steps := map[int32]spellModStep{}
	for _, t := range list.Talents {
		for _, g := range t.Groups {
			for _, rank := range g.Ranks {
				steps[rank] = spellModStep{passive: t.Passive, group: g}
			}
		}
	}
	return steps, nil
}

// spellModLine is one SPELLMOD line: a spell's probes after a path's first Steps steps, with Cast's aura up
// for a cast talent's after probes. Each probe keeps part of its record: timing its cast, gcd, cooldown,
// cost, channelMs and aura; damage its shaped, comboPoints, spellPower, attackPower, bonus, effects and
// each attack's attackPower under attacker; yellow its derived table.
type spellModLine struct {
	Path   string       `json:"path"`
	Bot    string       `json:"bot"`
	Steps  int          `json:"steps"`
	Cast   int32        `json:"cast,omitempty"`
	Spell  int32        `json:"spell"`
	Name   string       `json:"name"`
	Timing timingRecord `json:"timing"`
	Damage damageRecord `json:"damage"`
	Yellow struct {
		Derived spellDerived `json:"derived"`
	} `json:"yellow"`
}

type spellModKey struct {
	path  string
	steps int
	cast  int32
	spell int32
}

func (l spellModLine) key() spellModKey { return spellModKey{l.Path, l.Steps, l.Cast, l.Spell} }

// latestSpellModLines keys the lines, keeping each path's last bot only: a rerun of some paths replaces
// their lines and keeps the others. Notes say whose lines were replaced.
func latestSpellModLines(lines []spellModLine) (map[spellModKey]spellModLine, []string, error) {
	last := map[string]string{}
	for _, l := range lines {
		last[l.Path] = l.Bot
	}
	replaced := map[[2]string]bool{}
	keyed := map[spellModKey]spellModLine{}
	for _, l := range lines {
		if l.Bot != last[l.Path] {
			replaced[[2]string{l.Path, l.Bot}] = true
			continue
		}
		if _, ok := keyed[l.key()]; ok {
			return nil, nil, fmt.Errorf("%s probed %d twice after %d steps of %s (cast %d)", l.Bot, l.Spell, l.Steps, l.Path, l.Cast)
		}
		keyed[l.key()] = l
	}
	var notes []string
	for _, pair := range slices.SortedFunc(maps.Keys(replaced), func(a, b [2]string) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	}) {
		notes = append(notes, fmt.Sprintf("%s: %s's lines replace %s's", pair[0], last[pair[0]], pair[1]))
	}
	return keyed, notes, nil
}

// spellModSims builds the sim's characters of one path, each once: after its first steps, with a cast
// talent's aura up or not, as one of the class's specs, with a talent that teaches a spell or not.
type spellModSims struct {
	path  talentPath
	built map[spellModSimKey]*spellModSim
}

type spellModSimKey struct {
	steps       int
	cast        int32
	spec, teach string
}

type spellModSim struct {
	d   *damageSim
	err error
}

func newSpellModSims(path talentPath) *spellModSims {
	return &spellModSims{path: path, built: map[spellModSimKey]*spellModSim{}}
}

// get is path.goldcharSetup(steps) as spec, with the cast talent's aura mirrored, and a point in the teach
// talent when the steps have none.
func (s *spellModSims) get(steps int, cast int32, spec, teach string) *spellModSim {
	key := spellModSimKey{steps, cast, spec, teach}
	if sim, ok := s.built[key]; ok {
		return sim
	}
	sim := &spellModSim{}
	s.built[key] = sim
	path := s.path
	if teach != "" && !slices.ContainsFunc(path.Steps[:steps], func(step talentStep) bool { return step.Talent == teach }) {
		path.Steps = append(slices.Clone(path.Steps[:steps]), talentStep{Talent: teach, Rank: 1})
		steps++
	}
	setup, err := path.goldcharSetup(steps)
	if err != nil {
		sim.err = err
		return sim
	}
	setup.Spec = spec
	if cast != 0 {
		setup.Auras = append(setup.Auras, cast)
	}
	defer func() {
		if r := recover(); r != nil {
			sim.d, sim.err = nil, fmt.Errorf("sim panic: %v", r)
		}
	}()
	sim.d, sim.err = newPathSim(sweepTarget(path), setup)
	if sim.err != nil {
		return sim
	}
	// a channeled dot's expiry writes to the rotation, and a dot the damage replay applied expires at the next start
	if sim.d.char.Rotation == nil {
		sim.d.char.Rotation = &core.APLRotation{}
	}
	sim.d.start()
	return sim
}

// spellModSpecs are the path's spec, then the class's others, for a spell the path's spec doesn't register
// (a priest's heals, a shaman's Healing Wave).
func (path talentPath) spellModSpecs() []string {
	class := classes[path.Class]
	specs := []string{path.Spec}
	others := append([]protoreflect.Name{class.spec}, class.otherSpecs...)
	for _, spec := range append(others, timingExtraSpecs[path.Class]...) {
		if !slices.Contains(specs, string(spec)) {
			specs = append(specs, string(spec))
		}
	}
	return specs
}

// loadFirstRanks maps every spell of the spelldump to its first rank.
var loadFirstRanks = sync.OnceValues(func() (map[int32]int32, error) {
	root, err := checkoutRoot()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(root, spelldumpPath))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	first := map[int32]int32{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for scanner.Scan() {
		var s struct {
			ID          int32 `json:"id"`
			FirstRankID int32 `json:"firstRankId"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &s); err != nil {
			return nil, fmt.Errorf("%s: %w", spelldumpPath, err)
		}
		first[s.ID] = cmp.Or(s.FirstRankID, s.ID)
	}
	return first, scanner.Err()
})

// spellModTeacher is the class's talent whose rank spell is the spell's first rank: the sim registers such a
// spell (Mortal Strike, Living Bomb, Chaos Bolt) only with the talent in.
func (path talentPath) spellModTeacher(spellID int32) (string, error) {
	first, err := loadFirstRanks()
	if err != nil {
		return "", err
	}
	data, err := loadDamageData()
	if err != nil {
		return "", err
	}
	class := data.talents[path.Class]
	rank, ok := class.ranks[cmp.Or(first[spellID], spellID)]
	if !ok {
		return "", nil
	}
	for field, slot := range class.fields {
		if slot == rank.talentSlot {
			return field, nil
		}
	}
	return "", nil
}

// spellModPair is the sim's spell before and after a step, in the first spec that registers it on the
// character both times, given a point in its teach talent when it takes one. Note says why there's none
// to compare.
type spellModPair struct {
	before, after *timingSim
	spec, teach   string
	note          string
	err           error
}

func (s *spellModSims) pair(k int, cast, spellID int32) spellModPair {
	// Presence of Mind has no sim aura: the sim casts its one chosen spell with no cast time instead
	if cast != 0 {
		if after := s.get(k, cast, s.path.Spec, ""); after.err == nil && slices.Contains(after.d.others, cast) {
			return spellModPair{note: fmt.Sprintf("the sim has no aura for cast talent %d to put on", cast)}
		}
	}
	teach, err := s.path.spellModTeacher(spellID)
	if err != nil {
		return spellModPair{err: err}
	}
	teaches := []string{""}
	// the step's own talent stays out of the character before it
	if teach != "" && teach != s.path.Steps[k-1].Talent {
		teaches = append(teaches, teach)
	}
	for _, t := range teaches {
		for i, spec := range s.path.spellModSpecs() {
			before, after := s.get(k-1, 0, spec, t), s.get(k, cast, spec, t)
			if err := cmp.Or(before.err, after.err); err != nil {
				if i == 0 {
					return spellModPair{err: err}
				}
				continue
			}
			b, a := spellBySpell(&before.d.char.Unit, spellID), spellBySpell(&after.d.char.Unit, spellID)
			if b != nil && a != nil {
				return spellModPair{before: &timingSim{damageSim: before.d, spell: b},
					after: &timingSim{damageSim: after.d, spell: a}, spec: spec, teach: t}
			}
		}
	}
	d := s.get(k, cast, s.path.Spec, "").d
	for _, pet := range d.char.Pets {
		if spellBySpell(&pet.Unit, spellID) != nil {
			return spellModPair{note: fmt.Sprintf("the sim's %s casts it; the server probed it as the player", pet.Name)}
		}
	}
	return spellModPair{note: fmt.Sprintf("no spec of %v registers it on the character before and after the step",
		s.path.spellModSpecs())}
}

// label says how the pair's sim character differs from the path's own.
func (p spellModPair) label(path talentPath) string {
	var parts []string
	if p.spec != "" && p.spec != path.Spec {
		parts = append(parts, "as "+p.spec)
	}
	if p.teach != "" {
		parts = append(parts, "with a point in "+p.teach)
	}
	if len(parts) == 0 {
		return ""
	}
	return " (sim " + strings.Join(parts, ", ") + ")"
}

// Ways a spell's probes can compare across a step.
const (
	spellModAgrees       = "agrees"
	spellModMovesNothing = "moves nothing"
	spellModDisagrees    = "disagrees"
	spellModNotCompared  = "not compared"
)

// spellModResult is how one spell's probes compare across a step: a check per number all four sides have,
// the numbers that moved on either side, and the parts left out.
type spellModResult struct {
	class   string
	checks  []Check
	moved   []string
	skipped []string
}

func (r *spellModResult) add(c Check, moved string) {
	r.checks = append(r.checks, c)
	if !c.Passed {
		r.class = spellModDisagrees
	}
	if moved != "" {
		r.moved = append(r.moved, moved)
		if r.class == spellModMovesNothing {
			r.class = spellModAgrees
		}
	}
}

// delta holds the server's change in a number to the sim's, within tolerance.
func (r *spellModResult) delta(name string, sb, sa, mb, ma, tolerance float64) {
	server, sim := sa-sb, ma-mb
	detail := fmt.Sprintf("server %g -> %g, sim %g -> %g (within %g)", sb, sa, mb, ma, tolerance)
	c := fail(name, detail)
	if math.Abs(server-sim) <= tolerance+1e-9 {
		c = pass(name, detail)
	}
	moved := ""
	if eps := max(tolerance, 1e-6); math.Abs(server) > eps || math.Abs(sim) > eps {
		moved = fmt.Sprintf("%s server %+.4g sim %+.4g", name, server, sim)
	}
	r.add(c, moved)
}

// ratio holds the server's ratio of an amount after to before the step to the sim's. Each side's bound is
// how far the server's truncation can put it from the sim's amount, so the ratios may differ by their sum
// relative to each amount.
func (r *spellModResult) ratio(name string, sb, sa, mb, ma, boundB, boundA float64) {
	if sb <= 0 || sa <= 0 || mb <= 0 || ma <= 0 {
		r.delta(name, sb, sa, mb, ma, boundB+boundA)
		return
	}
	server, sim := sa/sb, ma/mb
	tolerance := server*(boundA/sa+boundB/sb) + 1e-9
	detail := fmt.Sprintf("server %.0f -> %.0f (x%.5f), sim %.2f -> %.2f (x%.5f), within %.5f", sb, sa, server, mb, ma, sim, tolerance)
	c := fail(name, detail)
	if math.Abs(server-sim) <= tolerance {
		c = pass(name, detail)
	}
	moved := ""
	if sa != sb || math.Abs(sim-1) > 1e-6 {
		moved = fmt.Sprintf("%s server x%.4f sim x%.4f", name, server, sim)
	}
	r.add(c, moved)
}

// compareSpellMod holds a spell's probes before and after a step to the sim's spell in the same two
// characters: timing exactly in whole ms and whole cost, damage per probed effect, crit chance and the
// derived miss.
func compareSpellMod(path talentPath, before, after spellModLine, pair spellModPair) (r spellModResult) {
	switch {
	case pair.err != nil:
		return spellModResult{class: spellModDisagrees, checks: []Check{fail("sim character", pair.err.Error())}}
	case pair.note != "":
		return spellModResult{class: spellModNotCompared, skipped: []string{pair.note}}
	}
	r.class = spellModMovesNothing
	defer func() {
		if p := recover(); p != nil {
			r.add(fail("sim", fmt.Sprintf("panic: %v", p)), "")
		}
	}()

	sb, sa := spellModServerTiming(before.Timing), spellModServerTiming(after.Timing)
	mb, ma := pair.before.spellModTiming(before.Timing), pair.after.spellModTiming(after.Timing)
	leaveOut := func(reason string, out func(name string) bool) {
		for _, m := range []map[string]float64{sb, sa} {
			maps.DeleteFunc(m, func(name string, _ float64) bool { return out(name) })
		}
		r.skipped = append(r.skipped, reason)
	}
	if !castsOnItsOwn(pair.before.spell) && !castsOnItsOwn(pair.after.spell) {
		leaveOut("cast time, GCD, cost and cooldown: the sim only procs or triggers it", func(name string) bool {
			return name != "aura duration" && !strings.HasSuffix(name, " amplitude") && !strings.HasSuffix(name, " ticks")
		})
	}
	if comboPointDuration(before.Spell) {
		leaveOut("aura duration and ticks: they scale with combo points, which the server probe has none of "+
			"and the sim's aura doesn't take from it", func(name string) bool {
			return name == "aura duration" || name == "channel" || strings.HasSuffix(name, " ticks")
		})
	}
	names := maps.Clone(sb)
	maps.Copy(names, sa)
	for _, name := range slices.Sorted(maps.Keys(names)) {
		r.compare(name, sb, sa, mb, ma, 0)
	}

	compareSpellModDamage(&r, path, before, after, pair)

	yb, ya := spellModServerYellow(before.Yellow.Derived), spellModServerYellow(after.Yellow.Derived)
	// critDone 0 on one side only is a real 0, not a spell the server never crits: a death knight's
	// Icy Touch has none until Rime or Deathchill
	_, critB := yb["crit chance"]
	_, critA := ya["crit chance"]
	if critB || critA {
		yb["crit chance"], ya["crit chance"] = min(100, before.Yellow.Derived.CritDone), min(100, after.Yellow.Derived.CritDone)
	}
	weapon := before.Yellow.Derived.Yellow != nil
	cb, ca := pair.before.spellModYellow(path, before, weapon), pair.after.spellModYellow(path, after, weapon)
	for _, f := range spellModYellowFields {
		r.compare(f.name, yb, ya, cb, ca, f.tolerance)
	}
	return r
}

// compare holds the server's change in a number to the sim's when all four sides have it. One the server
// moved but can't be held to the sim is left out with a note, so it can't pass for moving nothing.
func (r *spellModResult) compare(name string, sb, sa, mb, ma map[string]float64, tolerance float64) {
	vb, okB := sb[name]
	va, okA := sa[name]
	_, simB := mb[name]
	_, simA := ma[name]
	switch {
	case !okB && !okA:
	case okB && okA && simB && simA:
		r.delta(name, vb, va, mb[name], ma[name], tolerance)
	case okB && okA && vb == va:
	case okB && okA:
		r.skipped = append(r.skipped, fmt.Sprintf("%s: the server's %g -> %g, the sim has none", name, vb, va))
	case okB:
		r.skipped = append(r.skipped, name+": the server has it before only")
	default:
		r.skipped = append(r.skipped, name+": the server has it after only")
	}
}

// castsOnItsOwn is false for a sim spell with no cast time, GCD, cost or cooldown, one the sim only procs or
// triggers under the server's id of a cast: Slam's hit 50783, which spell_warr_slam triggers, or Seal of
// Righteousness' damage on 20154.
func castsOnItsOwn(spell *core.Spell) bool {
	return spell.DefaultCast != (core.Cast{}) || spell.Cost != nil || spell.CD.Timer != nil || spell.SharedCD.Timer != nil
}

// SPELL_ATTR1_FINISHING_MOVE_DAMAGE and _DURATION, SpellInfo::NeedsComboPoints
const attr1FinishingMove = 0x00100000 | 0x00400000

// comboPointDuration is whether the spell is a finisher whose duration Unit::CalcSpellDuration scales with
// combo points (Slice and Dice, Rupture).
func comboPointDuration(spellID int32) bool {
	s := serverdata.SpellByID(spellID)
	return s != nil && s.Attributes[1]&attr1FinishingMove != 0 && s.MaxDurationMs != s.DurationMs
}

// spellModServerTiming are a timing record's numbers the sweep compares, by check name.
func spellModServerTiming(rec timingRecord) map[string]float64 {
	f := map[string]float64{
		"cast time before haste": float64(max(0, rec.Cast.AfterSpellModsMs)),
		"cost":                   float64(rec.Cost.Amount),
	}
	// the sim has no rule for a cast the server made instant, and its one GCD is category 133's
	if rec.Cast.Haste != "instant" {
		f["cast time"] = float64(rec.Cast.Ms)
	}
	if rec.GCD.Category == 133 || rec.GCD.Ms == 0 {
		f["gcd"] = float64(rec.GCD.Ms)
	}
	if cd := rec.Cooldown; cd != nil {
		if !cd.StartsOnEvent {
			f["cooldown"] = float64(cd.Ms)
		}
		if cd.CategoryMs > 0 {
			f["category cooldown"] = float64(cd.CategoryMs)
		}
	}
	if runes := rec.Cost.Runes; runes != nil && len(runes.Cost) == 3 {
		delete(f, "cost")
		f["runic power cost"] = float64(rec.Cost.Amount)
		f["blood rune cost"], f["unholy rune cost"], f["frost rune cost"] = float64(runes.Cost[0]), float64(runes.Cost[1]), float64(runes.Cost[2])
		f["runic power gain"] = float64(runes.RunicPowerGain)
	}
	if rec.ChannelMs != nil {
		f["channel"] = float64(*rec.ChannelMs)
	}
	if a := rec.Aura; a.Applies && !a.Immune && a.Skipped == "" {
		f["aura duration"] = float64(a.Ms)
		for _, e := range a.Effects {
			if e.Periodic {
				f[fmt.Sprintf("e%d amplitude", e.Index)] = float64(e.AmplitudeMs)
				f[fmt.Sprintf("e%d ticks", e.Index)] = float64(e.Ticks)
			}
		}
	}
	return f
}

// spellModTiming are the sim spell's numbers for the record's checks, as the timing replay reads them.
// Cast time before haste keeps the spell's own multiplier, which the sim puts percent cast time talents in.
func (ts *timingSim) spellModTiming(rec timingRecord) map[string]float64 {
	ts.start()
	spell := ts.spell
	f := map[string]float64{
		"cast time before haste": float64(simMs(time.Duration(float64(spell.DefaultCast.CastTime) * spell.CastTimeMultiplier))),
		"cast time":              float64(simMs(spell.CastTime())),
		"gcd":                    float64(simMs(ts.gcd())),
		"cooldown":               0,
	}
	if spell.CD.Timer != nil {
		f["cooldown"] = float64(simMs(spell.CD.Duration))
	}
	if spell.SharedCD.Timer != nil {
		f["category cooldown"] = float64(simMs(spell.SharedCD.Duration))
	}
	if impl, ok := spell.Cost.(*core.RuneCostImpl); ok {
		f["blood rune cost"], f["unholy rune cost"], f["frost rune cost"] = float64(impl.BloodRuneCost), float64(impl.UnholyRuneCost), float64(impl.FrostRuneCost)
		f["runic power gain"] = math.Round(impl.RunicPowerGain * 10)
		f["runic power cost"] = math.Round(impl.RunicPowerCost * 10)
	} else {
		f["cost"] = math.Floor(spell.ApplyCostModifiers(spell.DefaultCast.Cost)*powerScale(rec.Cost.PowerType) + 1e-6)
	}

	if dot := ts.dot(rec.Aura.OnCaster); dot != nil {
		dot.RecomputeAuraDuration()
		f["channel"] = float64(simMs(dot.Aura.Duration))
		f["aura duration"] = float64(simMs(dot.Aura.Duration))
		for _, e := range rec.Aura.Effects {
			if e.Periodic {
				f[fmt.Sprintf("e%d amplitude", e.Index)] = float64(simMs(dot.TickPeriod()))
				f[fmt.Sprintf("e%d ticks", e.Index)] = float64(dot.TotalTicks())
			}
		}
		return f
	}
	unit := ts.target
	if rec.Aura.OnCaster {
		unit = &ts.char.Unit
	}
	if aura := auraBySpell(unit.GetAuras(), spell.ActionID.SpellID); aura != nil && rec.Aura.BaseMs > 0 {
		f["aura duration"] = float64(simMs(aura.Duration))
	}
	return f
}

// The derived table's numbers: the crit chance before the target's suppression (percent), and the miss
// threshold of the magic table or the yellow table's miss (basis points).
var spellModYellowFields = []struct {
	name      string
	tolerance float64
}{{"crit chance", 0.02}, {"miss", maxBPDiff}}

func spellModServerYellow(d spellDerived) map[string]float64 {
	f := map[string]float64{}
	// 0 is a spell the server never crits
	if d.CritDone != 0 {
		f["crit chance"] = min(100, d.CritDone)
	}
	switch {
	case d.Magic != nil && !d.Magic.AutoHit:
		f["miss"] = float64(d.Magic.MissThreshold)
	case d.Yellow != nil && !d.Yellow.AlwaysHit:
		f["miss"] = float64(d.Yellow.MissBp)
	}
	return f
}

// spellModYellow is the sim's crit chance and miss for the spell on the dummy: melee crit and hit for a
// melee or ranged class spell, spell crit and hit for the rest. Only their changes are compared, so the
// yellow miss is the hit chance taken off.
func (ts *timingSim) spellModYellow(path talentPath, line spellModLine, weapon bool) map[string]float64 {
	spell := ts.hitSpell(path, line)
	ts.start()
	table := ts.char.AttackTables[ts.target.UnitIndex]
	stat, miss := stats.SpellCrit, spell.SpellChanceToMiss(table)*core.MaxRollBP
	if weapon {
		stat, miss = stats.MeleeCrit, -spell.PhysicalHitChance(table)*core.MaxRollBP
	}
	return map[string]float64{
		// past 100 every hit crits, and the server stops adding flat crit mods at 100 (Player::ApplySpellMod),
		// so Cold Blood with Puncturing Wounds is 100 on both sides
		"crit chance": min(100, (ts.char.GetStat(stat)+spell.BonusCritRating)/core.CritRatingPerCritChance),
		"miss":        miss,
	}
}

// hitSpell is the sim spell whose hits the line's spell deals: the one declaring its first probed effect,
// else the spell itself, else when it declares nothing (Fan of Knives' and Rune Strike's casts, whose weapon
// hits the damage probe leaves out) the lowest tag of it that declares a direct effect.
func (ts *timingSim) hitSpell(path talentPath, line spellModLine) *core.Spell {
	ts.useDamageRecord(path, line)
	for _, e := range line.Damage.Effects {
		if !e.probed() {
			continue
		}
		if decl := ts.declaration(e); decl != nil {
			return decl.spell
		}
	}
	if ts.spell.Direct != (core.SpellEffect{}) || dotOn(ts.spell, ts.target) != nil || dotOn(ts.spell, &ts.char.Unit) != nil {
		return ts.spell
	}
	var found *core.Spell
	for _, s := range ts.char.Spellbook {
		if s.ActionID.SpellID == ts.spell.SpellID && s.ActionID.ItemID == 0 && s.ActionID.OtherID == 0 &&
			s.Direct != (core.SpellEffect{}) && (found == nil || s.ActionID.Tag < found.ActionID.Tag) {
			found = s
		}
	}
	return cmp.Or(found, ts.spell)
}

// useDamageRecord points the sim's damage replay at the line's damage record, whose spell and attack power
// the amounts are worked out at.
func (ts *timingSim) useDamageRecord(path talentPath, line spellModLine) {
	rec := line.Damage
	rec.Command = "damage"
	rec.Spell.ID, rec.Spell.Name = line.Spell, line.Name
	rec.Attacker.Class, rec.Attacker.Race, rec.Attacker.Level, rec.Attacker.IsPlayer = path.Class, path.Race, core.CharacterLevel, true
	rec.Target = sweepTarget(path).Target
	ts.rec = rec
}

// spellModEffect is the sim's side of one probed effect at one state.
type spellModEffect struct {
	decl *simDecl
	// the record's base, worked out for the min and max
	lo, hi float64
	// how far from the server's amount the sim's may be
	boundLo, boundHi float64
	crit             float64
	crits            bool
	err              error
}

func (ts *timingSim) spellModEffect(e damageEffect) spellModEffect {
	out := spellModEffect{decl: ts.declaration(e)}
	if out.decl == nil {
		return out
	}
	lo, errLo := ts.amount(*out.decl, e, e.Base.Min)
	hi, errHi := ts.amount(*out.decl, e, e.Base.Max)
	again, errAgain := ts.amount(*out.decl, e, e.Base.Min)
	if out.err = firstErr(errLo, errHi, errAgain); out.err != nil {
		return out
	}
	// a weapon roll in the sim's amount, which the server leaves to its own probe
	if math.Abs(again-lo) > 1e-6 {
		out.err = fmt.Errorf("the amount rolls: %.2f, then %.2f", lo, again)
		return out
	}
	out.lo, out.boundLo = lo, amountTolerance(e.Taken.Min)
	out.hi, out.boundHi = hi, amountTolerance(e.Taken.Max)
	out.crit, out.crits = ts.critMultiplier(*out.decl)
	return out
}

// compareSpellModDamage compares each effect both damage records probe: the declared base (when the
// declaration is the record's spell's) and coefficient exactly, the amounts as ratios after to before,
// and the crit multiplier where both sides crit it. A shaped record isn't compared.
func compareSpellModDamage(r *spellModResult, path talentPath, before, after spellModLine, pair spellModPair) {
	if before.Damage.Shaped != "" || after.Damage.Shaped != "" {
		r.skipped = append(r.skipped, "damage: shaped "+cmp.Or(after.Damage.Shaped, before.Damage.Shaped))
		return
	}
	pair.before.useDamageRecord(path, before)
	pair.after.useDamageRecord(path, after)
	for _, eb := range before.Damage.Effects {
		i := slices.IndexFunc(after.Damage.Effects, func(e damageEffect) bool { return e.Index == eb.Index && e.Kind == eb.Kind })
		if !eb.probed() || i < 0 {
			continue
		}
		ea := after.Damage.Effects[i]
		name := fmt.Sprintf("e%d %s", eb.Index, eb.Kind)
		mb, ma := pair.before.spellModEffect(eb), pair.after.spellModEffect(ea)
		switch {
		case mb.decl == nil || ma.decl == nil:
			r.skipped = append(r.skipped, name+": no sim declaration")
			continue
		case mb.err != nil || ma.err != nil:
			r.skipped = append(r.skipped, fmt.Sprintf("%s: sim amount: %v", name, cmp.Or(mb.err, ma.err)))
			continue
		}

		db, da := mb.decl.effect(), ma.decl.effect()
		if (db.FromSpellID == 0 || db.FromSpellID == before.Spell) && (da.FromSpellID == 0 || da.FromSpellID == after.Spell) {
			r.delta(name+" base min", eb.Base.Min, ea.Base.Min, db.Min, da.Min, 0)
			r.delta(name+" base max", eb.Base.Max, ea.Base.Max, db.Max, da.Max, 0)
		}
		r.delta(name+" coefficient", eb.Coefficient, ea.Coefficient, db.SP, da.SP, 5e-4)
		r.ratio(name+" min", eb.Taken.Min, ea.Taken.Min, mb.lo, ma.lo, mb.boundLo, ma.boundLo)
		r.ratio(name+" max", eb.Taken.Max, ea.Taken.Max, mb.hi, ma.hi, mb.boundHi, ma.boundHi)
		switch {
		case eb.Crit.Amount <= 0 || ea.Crit.Amount <= 0:
		case serverCanCrit(before.Spell, eb) && mb.crits && ma.crits:
			r.delta(name+" crit", eb.Crit.Result/eb.Crit.Amount, ea.Crit.Result/ea.Crit.Amount, mb.crit, ma.crit, maxMultiplier)
		case eb.Crit.Result/eb.Crit.Amount != ea.Crit.Result/ea.Crit.Amount:
			r.skipped = append(r.skipped, name+" crit: the server's multiplier moved, but one side never crits it")
		}
	}
}

// spellModDrop is a probed spell of a path step, or one check of it, that disagrees with the sim, each with
// a row in the fixture README. Spell 0 drops the whole step, an empty check every check of the spell.
type spellModDrop struct {
	path, talent string
	spell        int32
	check        string
}

// spellModReport is a replay's outcome: what fails, what's logged, and how many spell steps compared how.
type spellModReport struct {
	errors, logs []string
	counts       map[string]int
}

func (r *spellModReport) errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *spellModReport) logf(format string, args ...any) {
	r.logs = append(r.logs, fmt.Sprintf(format, args...))
}

// replaySpellMods compares every listed spell of every spell-mod step of the paths across the step,
// holding the results to drops. A dropped step, spell or check only logs its failures, and one that
// fails nothing fails the replay.
func replaySpellMods(paths []talentPath, steps map[int32]spellModStep, lines map[spellModKey]spellModLine,
	drops []spellModDrop) spellModReport {

	report := spellModReport{counts: map[string]int{}}
	used := map[spellModDrop]bool{}
	dropped := func(d spellModDrop) bool {
		if slices.Contains(drops, d) {
			used[d] = true
			return true
		}
		return false
	}
	var nothing []string
	for _, path := range paths {
		sims := newSpellModSims(path)
		for n, step := range path.Steps {
			k := n + 1
			listed, ok := steps[step.Spell]
			if !ok {
				continue
			}
			label := fmt.Sprintf("%s step %d %s %d/%d", path.Name, k, step.Talent, step.Rank, step.Spell)
			if len(listed.group.Spells) == 0 {
				report.logf("SPELLMODSIM %s: changes no spell the sim names", label)
				continue
			}
			var cast int32
			if !listed.passive {
				cast = step.Spell
			}
			whole := spellModDrop{path.Name, step.Talent, 0, ""}
			stepDropped := dropped(whole)
			stepFailed := false
			for _, s := range listed.group.Spells {
				spellLabel := fmt.Sprintf("%s %d %s", label, s.ID, s.Name)
				spellDrop := spellModDrop{path.Name, step.Talent, s.ID, ""}
				spellDropped := stepDropped || dropped(spellDrop)
				before, ok1 := lines[spellModKey{path.Name, k - 1, 0, s.ID}]
				after, ok2 := lines[spellModKey{path.Name, k, cast, s.ID}]
				if !ok1 || !ok2 {
					if spellDropped {
						report.logf("%s (dropped): no probe before or after it", spellLabel)
					} else {
						report.errorf("%s: no probe before or after it", spellLabel)
					}
					continue
				}

				pair := sims.pair(k, cast, s.ID)
				result := compareSpellMod(path, before, after, pair)
				report.counts[result.class]++
				if result.class == spellModMovesNothing {
					nothing = append(nothing, fmt.Sprintf("%s %s %d", path.Name, step.Talent, s.ID))
				}
				line := fmt.Sprintf("SPELLMODSIM %s%s: %s", spellLabel, pair.label(path), result.class)
				if len(result.moved) > 0 {
					line += "; " + strings.Join(result.moved, ", ")
				}
				if len(result.skipped) > 0 {
					line += "; not compared: " + strings.Join(result.skipped, ", ")
				}
				report.logf("%s", line)

				for _, c := range result.checks {
					checkDropped := dropped(spellModDrop{path.Name, step.Talent, s.ID, c.Name})
					switch {
					case checkDropped && c.Passed:
						report.errorf("%s: %s passes, so it shouldn't be dropped: %s", spellLabel, c.Name, c.Detail)
					case !c.Passed && (checkDropped || spellDropped):
						stepFailed = true
						report.logf("%s: %s (dropped): %s", spellLabel, c.Name, c.Detail)
					case !c.Passed:
						report.errorf("%s: %s: %s", spellLabel, c.Name, c.Detail)
					}
				}
				if spellDropped && !stepDropped && result.class != spellModDisagrees {
					report.errorf("%s %s, so it shouldn't be dropped", spellLabel, result.class)
				}
			}
			if stepDropped && !stepFailed {
				report.errorf("%s: no spell disagrees, so it shouldn't be dropped", label)
			}
		}
	}
	for _, d := range drops {
		if !used[d] {
			report.errorf("%+v is dropped, but no step of the paths matches it", d)
		}
	}
	report.logf("%d spell steps agree, %d move nothing, %d disagree, %d aren't compared", report.counts[spellModAgrees],
		report.counts[spellModMovesNothing], report.counts[spellModDisagrees], report.counts[spellModNotCompared])
	report.logf("moving nothing on either side: %s", strings.Join(nothing, ", "))
	return report
}
