package sim

import (
	"bufio"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// The server's spell group stack rules, as sim/core/testdata/simval/spell_groups.json holds them. Run
// against acore_world (SELECT only), then compact the result to one group per line:
//
//	SELECT JSON_PRETTY(JSON_OBJECT('groups', (SELECT JSON_ARRAYAGG(JSON_OBJECT('id', g.id,
//	  'rule', COALESCE(r.stack_rule, 0), 'description', COALESCE(r.description, ''), 'members', g.members))
//	  FROM (SELECT id, JSON_ARRAYAGG(spell_id) members FROM
//	    (SELECT * FROM spell_group ORDER BY id, spell_id) s GROUP BY id) g
//	  LEFT JOIN spell_group_stack_rules r ON r.group_id = g.id)));
//
// A negative member is a nested group. Rules are SpellGroupStackRule: 1 EXCLUSIVE, 2 EXCLUSIVE_FROM_SAME_CASTER,
// 3 EXCLUSIVE_SAME_EFFECT, 4 EXCLUSIVE_HIGHEST; 0 means the group sets none.
//
// TestSimSpellGroups compares them with the sim's ExclusiveEffect categories, which are effect types
// ("spellDamage", "AttackPowerBonus"), so it maps auras by spell id, ranks folded to the first rank.
// Spell-specific exclusivity (SpellInfo::IsAuraExclusiveBySpecificWith: foods, scrolls, armors, presences)
// isn't in these tables, so it sits in spellGroupAllowances.
type spellGroupRow struct {
	ID          int32   `json:"id"`
	Rule        int     `json:"rule"`
	Description string  `json:"description"`
	Members     []int32 `json:"members"`
}

const (
	ruleDefault = iota
	ruleExclusive
	ruleFromSameCaster
	ruleSameEffect
	ruleHighest
)

var ruleNames = [...]string{"none", "EXCLUSIVE", "EXCLUSIVE_FROM_SAME_CASTER", "EXCLUSIVE_SAME_EFFECT", "EXCLUSIVE_HIGHEST"}

// spellGroups answers what SpellMgr::CheckSpellGroupStackRules does.
type spellGroups struct {
	rows     map[int32]spellGroupRow
	expanded map[int32]map[int32]bool // group -> every spell in it, nested groups included
	of       map[int32][]int32        // spell -> its groups, ascending
	// the aura types each EXCLUSIVE_SAME_EFFECT group counts, as SpellMgr::LoadSpellGroupStackRules guesses them
	sameEffect map[int32]map[int32]bool
	names      *spellNames
}

func loadSpellGroups(t *testing.T, names *spellNames) *spellGroups {
	raw, err := os.ReadFile("core/testdata/simval/spell_groups.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct{ Groups []spellGroupRow }
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	g := &spellGroups{rows: map[int32]spellGroupRow{}, expanded: map[int32]map[int32]bool{}, of: map[int32][]int32{},
		sameEffect: map[int32]map[int32]bool{}, names: names}
	for _, row := range file.Groups {
		g.rows[row.ID] = row
	}
	var expand func(id int32, into map[int32]bool, seen map[int32]bool)
	expand = func(id int32, into map[int32]bool, seen map[int32]bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, m := range g.rows[id].Members {
			if m < 0 {
				expand(-m, into, seen)
			} else {
				into[m] = true
			}
		}
	}
	for id := range g.rows {
		g.expanded[id] = map[int32]bool{}
		expand(id, g.expanded[id], map[int32]bool{})
	}
	for _, id := range slices.Sorted(maps.Keys(g.rows)) {
		for spell := range g.expanded[id] {
			g.of[spell] = append(g.of[spell], id)
		}
	}
	for spell := range g.of {
		slices.Sort(g.of[spell])
	}
	const meleeHaste, rangedHaste, meleeRangedHaste = 138, 140, 192
	for id, row := range g.rows {
		if row.Rule != ruleSameEffect {
			continue
		}
		// the most common aura type among the members; a tie goes to the lowest id, the server's order is unspecified
		count := map[int32]int{}
		for spell := range g.expanded[id] {
			for _, aura := range names.auras[spell] {
				if aura == rangedHaste || aura == meleeRangedHaste {
					aura = meleeHaste
				}
				count[aura]++
			}
		}
		var best int32
		for _, aura := range slices.Sorted(maps.Keys(count)) {
			if count[aura] > count[best] {
				best = aura
			}
		}
		g.sameEffect[id] = map[int32]bool{best: true}
		if best == meleeHaste {
			g.sameEffect[id] = map[int32]bool{meleeHaste: true, rangedHaste: true, meleeRangedHaste: true}
		}
	}
	return g
}

// countsFor is whether the group's guessed aura type is on the spell or one of its ranks.
func (g *spellGroups) countsFor(group, spell int32) bool {
	for aura := range g.sameEffect[group] {
		if g.names.chain[spell][aura] {
			return true
		}
	}
	return false
}

// rule is the first nonzero rule among the groups both spells share. A shared group is skipped when one of
// its nested groups holds both spells too: that nested group speaks for them.
func (g *spellGroups) rule(a, b int32) (int, int32) {
	for _, id := range g.of[a] {
		if !g.expanded[id][b] {
			continue
		}
		nested := false
		for _, m := range g.rows[id].Members {
			if m < 0 && g.expanded[-m][a] && g.expanded[-m][b] {
				nested = true
				break
			}
		}
		if nested || g.rows[id].Rule == ruleDefault {
			continue
		}
		// a same-effect group only compares the aura type it counts: two spells without it don't interact
		if g.rows[id].Rule == ruleSameEffect && !(g.countsFor(id, a) && g.countsFor(id, b)) {
			return ruleDefault, id
		}
		return g.rows[id].Rule, id
	}
	return ruleDefault, 0
}

type spellInfoRow struct {
	ID          int32  `json:"id"`
	Name        string `json:"name"`
	FirstRankID int32  `json:"firstRankId"`
	Effects     []struct {
		Aura int32 `json:"aura"`
	} `json:"effects"`
}

type spellNames struct {
	first map[int32]int32
	name  map[int32]string
	auras map[int32][]int32        // a spell's own aura types, one per aura effect
	chain map[int32]map[int32]bool // a first rank's aura types across all its ranks
}

func loadSpellNames(t *testing.T) *spellNames {
	f, err := os.Open("../assets/db_inputs/acore/spelldump.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := &spellNames{first: map[int32]int32{}, name: map[int32]string{}, auras: map[int32][]int32{}, chain: map[int32]map[int32]bool{}}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<26)
	for scanner.Scan() {
		var row spellInfoRow
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		s.first[row.ID] = row.FirstRankID
		s.name[row.ID] = row.Name
		first := row.FirstRankID
		if first == 0 {
			first = row.ID
		}
		if s.chain[first] == nil {
			s.chain[first] = map[int32]bool{}
		}
		for _, e := range row.Effects {
			if e.Aura != 0 {
				s.chain[first][e.Aura] = true
				s.auras[row.ID] = append(s.auras[row.ID], e.Aura)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return s
}

func (s *spellNames) firstRank(id int32) int32 {
	if f, ok := s.first[id]; ok && f != 0 {
		return f
	}
	return id
}

func (s *spellNames) label(id int32) string {
	return fmt.Sprintf("%s (%d)", s.name[id], id)
}

type spellPair [2]int32

func pairOf(a, b int32) spellPair {
	if a > b {
		a, b = b, a
	}
	return spellPair{a, b}
}

// simAuraSpells maps the spell a sim aura is registered under to the aura spell the server applies, where
// they differ: the sim names a talent or a cast spell, the server's aura is the spell it triggers.
var simAuraSpells = map[int32]int32{
	9452:  67,    // Vindication
	48506: 60431, // Earth and Moon
	51099: 51726, // Ebon Plaguebringer
	48483: 58179, // Infected Wounds
	53695: 68055, // Judgements of the Just
	11095: 22959, // Improved Scorch
	11180: 12579, // Winter's Chill
	51682: 58684, // Savage Combat
	29836: 30069, // Blood Frenzy
	46854: 46856, // Trauma
	20335: 21183, // Heart of the Crusader
	30706: 57658, // Totem of Wrath
	8227:  52109, // Flametongue Totem
	47236: 48090, // Demonic Pact
	47467: 58567, // Sunder Armor
}

// spellGroupAllowances are the pairs on which the sim and the server differ, with the reason. Each is a
// spell-specific rule the tables lack or a tie that never changes a value, so the sim matches the server.
var spellGroupAllowances = map[spellPair]string{
	{71, 2457}:     stanceReason,
	{71, 2458}:     stanceReason,
	{2457, 2458}:   stanceReason,
	{2818, 13218}:  poisonReason,
	{2818, 21183}:  poisonReason,
	{2818, 30708}:  poisonReason,
	{13218, 21183}: poisonReason,
	{13218, 30708}: poisonReason,
	{34074, 61846}: "server: SpellInfo::IsAuraExclusiveBySpecificPerCasterWith makes a caster's aspects exclusive, not a group row. The sim's Aspect category matches that.",
	{48263, 48265}: presenceReason,
	{48263, 48266}: presenceReason,
	{48265, 48266}: presenceReason,
}

const (
	stanceReason   = "server: a shapeshift form replaces the previous one (Unit::SetShapeshiftForm), not a group row. The sim's Stance category matches that."
	presenceReason = "server: SpellInfo::IsAuraExclusiveBySpecificWith makes presences exclusive (SPELL_SPECIFIC_PRESENCE), not a group row. The sim's Presence category matches that."
	poisonReason   = "server: group 1058 ties the poisons to the crit-taken debuffs, but every poison rank's aura-197 effect is 0 and Master Poisoner's crit is the caster aura 45176 (type 308, outside the group), so the tie never moves a total. The sim stacks them, which is the same result."
)

// everyOn turns every switch of a buff, debuff or party message on: bools, counts and tristates.
func everyOn[T googleProto.Message](m T) T {
	r := m.ProtoReflect()
	fields := r.Descriptor().Fields()
	for i := range fields.Len() {
		switch fd := fields.Get(i); {
		case fd.Kind() == protoreflect.BoolKind:
			r.Set(fd, protoreflect.ValueOfBool(true))
		case fd.Kind() == protoreflect.Int32Kind:
			r.Set(fd, protoreflect.ValueOfInt32(1))
		case fd.Kind() == protoreflect.EnumKind && fd.Enum().Name() == "TristateEffect":
			r.Set(fd, protoreflect.ValueOfEnum(protoreflect.EnumNumber(proto.TristateEffect_TristateEffectImproved)))
		}
	}
	return m
}

// consumeVariant sets each consumable slot to its n-th value, or leaves the slot alone past its last.
func consumeVariant(base *proto.Consumes, n int) *proto.Consumes {
	c := googleProto.Clone(base).(*proto.Consumes)
	r := c.ProtoReflect()
	fields := r.Descriptor().Fields()
	for i := range fields.Len() {
		if fd := fields.Get(i); fd.Kind() == protoreflect.EnumKind && n < fd.Enum().Values().Len() {
			r.Set(fd, protoreflect.ValueOfEnum(fd.Enum().Values().Get(n).Number()))
		}
	}
	return c
}

// stackScenario is one raid and the units it builds.
type stackScenario struct {
	label string
	raid  *proto.Raid
}

func stackScenarios() []stackScenario {
	var scenarios []stackScenario
	buffed := func(p serverDataPreset, consumes *proto.Consumes) *proto.Raid {
		raid := p.raid()
		raid.Buffs = everyOn(googleProto.Clone(core.FullRaidBuffs).(*proto.RaidBuffs))
		raid.Debuffs = everyOn(googleProto.Clone(core.FullDebuffs).(*proto.Debuffs))
		party := raid.Parties[0]
		party.Buffs = everyOn(googleProto.Clone(core.FullPartyBuffs).(*proto.PartyBuffs))
		player := party.Players[0]
		player.Buffs = everyOn(googleProto.Clone(player.Buffs).(*proto.IndividualBuffs))
		player.Consumes = consumes
		return raid
	}
	for _, p := range serverDataPresets {
		scenarios = append(scenarios, stackScenario{p.name, buffed(p, serverDataConsumes)})
	}
	// every item, set and meta gem with an effect, one at a time: their procs are spells too
	swept := map[proto.Class]bool{}
	for _, p := range serverDataPresets {
		if swept[p.class] {
			continue
		}
		swept[p.class] = true
		raid := buffed(p, serverDataConsumes)
		items := &core.ItemsTestGenerator{Player: raid.Parties[0].Players[0], PartyBuffs: raid.Parties[0].Buffs,
			RaidBuffs: raid.Buffs, Debuffs: raid.Debuffs, Encounter: core.MakeSingleTargetEncounter(0)}
		for i := range items.NumTests() {
			label, _, _, request := items.GetTest(i)
			scenarios = append(scenarios, stackScenario{p.name + " with " + label, request.Raid})
		}
	}
	most := 0
	fields := (&proto.Consumes{}).ProtoReflect().Descriptor().Fields()
	for i := range fields.Len() {
		if fd := fields.Get(i); fd.Kind() == protoreflect.EnumKind {
			most = max(most, fd.Enum().Values().Len())
		}
	}
	for _, name := range []string{"Fire", "Frost", "Holy paladin"} {
		p := presetNamed(name)
		for n := 1; n < most; n++ {
			scenarios = append(scenarios, stackScenario{fmt.Sprintf("%s, consumes %d", name, n), buffed(p, consumeVariant(serverDataConsumes, n))})
		}
	}
	return scenarios
}

// skippedAuras counts the auras the mapping can't use, by reason.
type skippedAuras map[string]int

type unitAuras struct {
	label      string
	spells     map[int32]bool            // first-rank ids of every registered aura with a spell id
	categories map[int32]map[string]bool // spell -> its sim exclusive categories
	single     map[string]bool           // category -> SingleAura
}

func collectUnitAuras(env *core.Environment, names *spellNames, skipped skippedAuras, aliased map[int32]bool) []*unitAuras {
	var units []*unitAuras
	for _, unit := range env.AllUnits {
		u := &unitAuras{label: unit.Label, spells: map[int32]bool{}, categories: map[int32]map[string]bool{}, single: map[string]bool{}}
		for _, aura := range unit.GetAuras() {
			id := aura.ActionID
			switch {
			case id.ItemID != 0:
				skipped["an item aura"]++
				continue
			case id.OtherID != 0:
				skipped["an OtherID aura"]++
				continue
			case id.SpellID == 0:
				skipped["no spell id"]++
				continue
			case id.Tag != 0 && id.Tag != -1:
				// -1 marks an external cooldown (Bloodlust, Power Infusion), still one aura per spell
				skipped["a tagged aura"]++
				continue
			}
			spell := names.firstRank(id.SpellID)
			if server, ok := simAuraSpells[spell]; ok {
				aliased[spell] = true
				spell = server
			}
			u.spells[spell] = true
			for _, ee := range aura.ExclusiveEffects {
				if u.categories[spell] == nil {
					u.categories[spell] = map[string]bool{}
				}
				u.categories[spell][ee.Category.Name] = true
				u.single[ee.Category.Name] = ee.Category.SingleAura
			}
		}
		units = append(units, u)
	}
	return units
}

type pairFinding struct {
	where  string
	reason string
}

func TestSimSpellGroups(t *testing.T) {
	names := loadSpellNames(t)
	groups := loadSpellGroups(t, names)
	skipped := skippedAuras{}
	aliased := map[int32]bool{}

	failures := map[spellPair]pairFinding{} // sim and server disagree
	formDiffers := map[spellPair]string{}   // both keep them apart, but not the same way
	checked := 0

	check := func(label string, env *core.Environment) {
		for _, u := range collectUnitAuras(env, names, skipped, aliased) {
			spells := slices.Sorted(maps.Keys(u.spells))
			for i, a := range spells {
				for _, b := range spells[i+1:] {
					checked++
					rule, group := groups.rule(a, b)
					var shared []string
					for c := range u.categories[a] {
						if u.categories[b][c] {
							shared = append(shared, c)
						}
					}
					slices.Sort(shared)
					pair := pairOf(a, b)
					where := label + ", " + u.label
					switch exclusiveOnServer := rule == ruleExclusive || rule == ruleHighest || rule == ruleSameEffect; {
					case len(shared) > 0 && !exclusiveOnServer:
						failures[pair] = pairFinding{where, fmt.Sprintf("the sim makes them exclusive (%s) and the server lets them stack (rule %s)", strings.Join(shared, ", "), ruleNames[rule])}
					case len(shared) == 0 && exclusiveOnServer:
						failures[pair] = pairFinding{where, fmt.Sprintf("the server makes them exclusive (group %d %q, %s) and the sim stacks them", group, groups.rows[group].Description, ruleNames[rule])}
					case len(shared) > 0:
						singleInSim := u.single[shared[0]]
						if singleInSim != (rule != ruleSameEffect) {
							formDiffers[pair] = fmt.Sprintf("sim category %s (SingleAura %v), server %s", shared[0], singleInSim, ruleNames[rule])
						}
					}
				}
			}
		}
	}

	for _, s := range stackScenarios() {
		env, _, _ := core.NewEnvironment(s.raid, core.MakeSingleTargetEncounter(0), false)
		check(s.label, env)
	}

	used := map[spellPair]bool{}
	for _, pair := range slices.SortedFunc(maps.Keys(failures), func(x, y spellPair) int { return slices.Compare(x[:], y[:]) }) {
		f := failures[pair]
		if _, ok := spellGroupAllowances[pair]; ok {
			used[pair] = true
			continue
		}
		t.Errorf("%s and %s: %s (%s)", names.label(pair[0]), names.label(pair[1]), f.reason, f.where)
	}
	for pair, why := range spellGroupAllowances {
		if strings.TrimSpace(why) == "" {
			t.Errorf("%s and %s: the allowance needs a reason", names.label(pair[0]), names.label(pair[1]))
		}
		if !used[pair] {
			t.Errorf("%s and %s: no longer differs, so its allowance is stale", names.label(pair[0]), names.label(pair[1]))
		}
	}
	for _, spell := range slices.Sorted(maps.Keys(simAuraSpells)) {
		if !aliased[spell] {
			t.Errorf("%s is in simAuraSpells, but no sim aura of the scenarios is registered under it", names.label(spell))
		}
	}
	for _, pair := range slices.SortedFunc(maps.Keys(formDiffers), func(x, y spellPair) int { return slices.Compare(x[:], y[:]) }) {
		t.Logf("same rule up to form: %s and %s: %s", names.label(pair[0]), names.label(pair[1]), formDiffers[pair])
	}
	for _, reason := range slices.Sorted(maps.Keys(skipped)) {
		t.Logf("skipped %d auras: %s", skipped[reason], reason)
	}
	t.Logf("checked %d aura pairs", checked)
}
