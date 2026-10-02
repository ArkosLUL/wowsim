package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

// serverlessGlyphs are sim glyphs with no GlyphProperties row on the server. The list gives them glyph 0
// and the e2e leaves them off.
var serverlessGlyphs = map[int32]string{
	206580: "Glyph of Omen of Clarity, a WotLK Classic glyph; the sim doesn't implement it either",
}

// goldcharSuiteFields is what the suites decide about a setup, the rest zeroed: auras, probes and the
// GlyphProperties ids are the list's own.
func goldcharSuiteFields(s goldcharSetup) goldcharSetup {
	s.Auras, s.Probes, s.Imbue = nil, nil, 0
	s.Glyphs = slices.Clone(s.Glyphs)
	for i := range s.Glyphs {
		s.Glyphs[i].Glyph = 0
	}
	return s
}

// The list TestSimvalGoldenCharacters builds characters from has to hold every non-healer suite's default
// player as the suites spell it out today. When a suite changes, this prints the entry to put in.
func TestGoldcharSetupsFollowTheSuites(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	want, err := goldcharSetupsFromSuites(root)
	if err != nil {
		t.Fatal(err)
	}
	have, err := readGoldcharSetups(filepath.Join(root, fixtureDir, goldcharSetupsFile))
	if err != nil {
		t.Fatal(err)
	}

	matched := map[int]bool{}
	for _, w := range want {
		i := slices.IndexFunc(have, func(h goldcharSetup) bool { return slices.Equal(h.Suites, w.Suites) })
		if i < 0 {
			text, _ := json.Marshal(w)
			t.Errorf("no setup for %v; the suites give %s", w.Suites, text)
			continue
		}
		matched[i] = true
		got, _ := json.Marshal(goldcharSuiteFields(have[i]))
		text, _ := json.Marshal(goldcharSuiteFields(w))
		if string(got) != string(text) {
			t.Errorf("%s differs from its suites:\n have %s\n want %s", have[i].Name, got, text)
		}
	}
	for i, h := range have {
		if !matched[i] {
			t.Errorf("%s: no non-healer suite plays %v any more", h.Name, h.Suites)
		}
	}

	for _, h := range have {
		for _, g := range h.Glyphs {
			if _, ok := serverlessGlyphs[g.Item]; ok != (g.Glyph == 0) {
				t.Errorf("%s: glyph item %d has GlyphProperties id %d", h.Name, g.Item, g.Glyph)
			}
		}
		if len(h.Probes) == 0 {
			t.Errorf("%s probes no spell", h.Name)
		}
		var ids []int32
		for _, p := range h.Probes {
			ids = append(ids, p.ID)
		}
		if slices.Sort(ids); len(slices.Compact(ids)) != len(h.Probes) {
			t.Errorf("%s probes a spell twice", h.Name)
		}
	}
}

// `.simval talent` refuses a talent before the one it depends on, which can sit later in the same row
// (Dual Wield Specialization before Dual Wield), so every talent with a prerequisite is learned after it.
func TestGoldcharTalentsLearnPrerequisitesFirst(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(root, talentTreesDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no tree files: %v", err)
	}
	checked := 0
	for _, path := range files {
		trees, err := readTalentTreeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var layout talentTreeLayout
		if err := json.Unmarshal(data, &layout); err != nil {
			t.Fatal(err)
		}
		for i, tree := range layout {
			for j, place := range tree.Talents {
				if place.PrereqLocation == nil {
					continue
				}
				k := slices.IndexFunc(tree.Talents, func(p talentPlace) bool { return p.Location == *place.PrereqLocation })
				if k < 0 {
					t.Errorf("%s tree %d: %s's prerequisite isn't in the tree", path, i, trees[i].Talents[j].FieldName)
					continue
				}
				digits := []byte(strings.Repeat("0", len(tree.Talents)))
				digits[j], digits[k] = '1', byte('0'+trees[i].Talents[k].MaxPoints)
				talents := strings.Repeat("-", i) + string(digits)
				spells, err := talentStringSpells(path, talents)
				if err != nil {
					t.Fatal(err)
				}
				pre, talent := trees[i].Talents[k], trees[i].Talents[j]
				want := []int32{rankSpells(pre.SpellIds, pre.MaxPoints)[pre.MaxPoints-1], rankSpells(talent.SpellIds, talent.MaxPoints)[0]}
				if !slices.Equal(spells, want) {
					t.Errorf("%s %s: learns %v, want %s first: %v", path, talent.FieldName, spells, pre.FieldName, want)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no talent with a prerequisite")
	}
}

// `.simval talent` learns the rank its spell is on the server, so the tree files' rank spells have to
// be the server's, rank for rank. They don't always follow id order: Sudden Death's rank 2 is 29725.
func TestTalentTreeRankSpellsAreTheServers(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	dump, err := spellset.ReadDump(filepath.Join(root, spellset.DefaultDumpPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(dump) == 0 {
		t.Skip("no spelldump")
	}
	files, err := filepath.Glob(filepath.Join(root, talentTreesDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no tree files: %v", err)
	}
	for _, path := range files {
		trees, err := readTalentTreeFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, tree := range trees {
			for _, talent := range tree.Talents {
				spells := rankSpells(talent.SpellIds, talent.MaxPoints)
				for i, id := range spells {
					s := dump[id]
					if s == nil {
						t.Errorf("%s %s: rank %d's spell %d isn't in the spelldump", filepath.Base(path), talent.FieldName, i+1, id)
					} else if s.FirstRankID != int64(spells[0]) || s.RankIndex != int64(i+1) {
						t.Errorf("%s %s: rank %d's spell %d is rank %d of %d on the server", filepath.Base(path),
							talent.FieldName, i+1, id, s.RankIndex, s.FirstRankID)
					}
				}
			}
		}
	}
}

// A full simval.jsonl recapture would hold TestSimvalGoldenCharacters' records too, if
// SIMVAL_GOLDCHAR_SETUPS were set for it. A geared info record gets nothing from CheckRecord but the armor
// penetration conversion, and its damage records nothing (TestDamageRecordInTheReplayFixture).
func TestGoldcharInfoRecordInTheReplayFixture(t *testing.T) {
	const rating = 492
	bonus := rating / core.RatingPerPercent(proto.Class_ClassWarrior, core.CRArmorPenetration)
	rec := spellRecord(t, fmt.Sprintf(`{"schema": 1, "command": "info",
		"attacker": {"name": "Bot", "isPlayer": true, "class": 1, "race": 2, "level": 80,
			"attacks": [{"type": "mainhand", "weapon": {"itemId": 51227}}],
			"ratings": [{"name": "armorPenetration", "rating": %d, "bonus": %f}],
			"sheet": {"critMainhand": 30, "spellCrit": [0, 5]}, "auras": [{"id": 58370, "durationMs": -1}]},
		"target": {"name": "Dummy", "entry": 999000, "level": 83},
		"character": {"talents": [12320], "glyphs": [0, 0, 0, 0, 0, 0], "equipment": [{"slot": 15, "itemId": 51227}]}}`,
		rating, bonus))

	checks := CheckRecord(rec)
	if len(checks) != 1 || checks[0].Name != "armor pen %" || !checks[0].Passed {
		t.Errorf("checks %+v, want only a passing armor pen %%", checks)
	}
	if got, want := rec.key(), (recordKey{"info", 0, warrior, boss}); got != want {
		t.Errorf("key %v, want %v", got, want)
	}
}

// goldcharDrop is a record or check of the capture that disagrees with the sim, each with a row in the
// fixture README. Spell 0 is the setup's info record. An empty check drops the whole record, which then
// isn't in the capture (for spell 0, the setup's bot isn't); a named check stays and is only logged.
type goldcharDrop struct {
	setup string
	spell int32
	check string
}

var droppedGoldchar = []goldcharDrop{}

// goldcharReport reports a record's checks, holding them to droppedGoldchar.
type goldcharReport struct {
	used    map[goldcharDrop]bool
	checked int
}

func (r *goldcharReport) dropped(drop goldcharDrop) bool {
	if slices.Contains(droppedGoldchar, drop) {
		r.used[drop] = true
		return true
	}
	return false
}

func (r *goldcharReport) checks(t *testing.T, setup string, spell int32, label string, checks []Check) {
	t.Helper()
	r.checked += len(checks)
	for _, c := range checks {
		switch dropped := r.dropped(goldcharDrop{setup, spell, c.Name}); {
		case dropped && c.Passed:
			t.Errorf("%s: %s passes, so it shouldn't be dropped: %s", label, c.Name, c.Detail)
		case dropped:
			t.Logf("%s: %s (dropped): %s", label, c.Name, c.Detail)
		case !c.Passed:
			t.Errorf("%s: %s: %s", label, c.Name, c.Detail)
		default:
			t.Logf("%s: %s: %s", label, c.Name, c.Detail)
		}
	}
}

// TestGoldenCharacters replays a live TestSimvalGoldenCharacters capture: each bot's sheet and probes
// against its setup's character in the sim. Skips without the capture, like TestDamageRecords.
func TestGoldenCharacters(t *testing.T) {
	needsItemDB(t)
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, fixtureDir, goldcharFixture)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("no capture at %s; record one with mod-sim-validation's TestSimvalGoldenCharacters", path)
	}
	setups, err := readGoldcharSetups(filepath.Join(root, fixtureDir, goldcharSetupsFile))
	if err != nil {
		t.Fatal(err)
	}
	records, err := readGoldcharRecords(path)
	if err != nil {
		t.Fatal(err)
	}
	bots, err := goldcharBots(records)
	if err != nil {
		t.Fatal(err)
	}

	bySetup := map[string]*goldcharBot{}
	for _, bot := range bots {
		s, err := matchGoldcharSetup(setups, bot.info)
		if err != nil {
			t.Error(err)
			continue
		}
		if other := bySetup[s.Name]; other != nil {
			t.Errorf("%s and %s both play %s", other.name, bot.name, s.Name)
			continue
		}
		bySetup[s.Name] = bot
	}

	report := &goldcharReport{used: map[goldcharDrop]bool{}}
	for _, s := range setups {
		t.Run(s.Name, func(t *testing.T) {
			bot := bySetup[s.Name]
			if dropped := report.dropped(goldcharDrop{s.Name, 0, ""}); bot == nil || dropped {
				if bot == nil && !dropped {
					t.Error("no bot in the capture plays it")
				} else if bot != nil {
					t.Errorf("%s plays it, but it's dropped", bot.name)
				}
				return
			}
			report.checks(t, s.Name, 0, bot.info.label(), checkGoldcharInfo(*bot.info, s))

			for _, p := range s.Probes {
				var recs []damageRecord
				for _, rec := range bot.damage {
					if rec.Spell.ID == p.ID {
						recs = append(recs, rec)
					}
				}
				switch dropped := report.dropped(goldcharDrop{s.Name, p.ID, ""}); {
				case len(recs) > 1:
					t.Errorf("%d records for %d %s", len(recs), p.ID, p.Name)
				case len(recs) == 1 && dropped:
					t.Errorf("%s is in the capture, but it's dropped", recs[0].label())
				case len(recs) == 1:
					report.checks(t, s.Name, p.ID, recs[0].label(), checkGoldcharDamage(recs[0], s))
				case !dropped:
					t.Errorf("no record for %d %s", p.ID, p.Name)
				}
			}
			for _, rec := range bot.damage {
				if !slices.ContainsFunc(s.Probes, func(p goldcharProbe) bool { return p.ID == rec.Spell.ID }) {
					t.Errorf("%s isn't one of the setup's probes", rec.label())
				}
			}
		})
	}

	for _, drop := range droppedGoldchar {
		if !report.used[drop] {
			t.Errorf("%+v is dropped, but nothing in the capture or the list matches it", drop)
		}
	}
	if report.checked == 0 {
		t.Fatalf("%s has %d records but nothing to check", path, len(records))
	}
	t.Logf("%d checks over %d records of %d bots", report.checked, len(records), len(bots))
}

func allGoldcharSetups(t *testing.T) []goldcharSetup {
	t.Helper()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	setups, err := readGoldcharSetups(filepath.Join(root, fixtureDir, goldcharSetupsFile))
	if err != nil {
		t.Fatal(err)
	}
	return setups
}

func goldcharSetupNamed(t *testing.T, name string) goldcharSetup {
	t.Helper()
	setups := allGoldcharSetups(t)
	i := slices.IndexFunc(setups, func(s goldcharSetup) bool { return s.Name == name })
	if i < 0 {
		t.Fatalf("no setup %s", name)
	}
	return setups[i]
}

// goldcharCharacterJSON is the info record's character block for the setup, as the module writes it.
func goldcharCharacterJSON(s goldcharSetup) map[string]any {
	glyphs := make([]int32, len(majorGlyphSlots)+len(minorGlyphSlots))
	for _, g := range s.Glyphs {
		glyphs[g.Slot] = g.Glyph
	}
	items := slices.Clone(s.Items)
	slices.SortFunc(items, func(a, b goldcharItem) int { return int(a.Slot) - int(b.Slot) })
	var equipment []map[string]any
	for _, item := range items {
		equipment = append(equipment, map[string]any{"slot": item.Slot, "itemId": item.ID})
	}
	return map[string]any{
		"talents": slices.Sorted(slices.Values(s.TalentSpells)), "glyphs": glyphs, "equipment": equipment,
	}
}

// serverSheet is what an info record says about the character, in the module's units.
type serverSheet struct {
	spellPower float64
	// schools above spellPower
	schoolPower         map[string]float64
	spellCrit, spellHit float64
	hitMelee, hitRanged float64
	attacks             map[string]*serverAttack
	auras               []int32
}

type serverAttack struct {
	unhastedMs, hastedMs int32
	damageMin, damageMax float64
	crit, expertise      float64
}

// goldcharInfo is an info record of the setup's bot with that sheet, at the boss dummy.
func goldcharInfo(t *testing.T, s goldcharSetup, sheet serverSheet) goldcharRecord {
	t.Helper()
	schools := map[string]float64{"physical": 0}
	for _, school := range magicSchools {
		schools[school] = sheet.spellPower
	}
	for school, power := range sheet.schoolPower {
		schools[school] = power
	}
	spellCrit := []float64{0}
	for range magicSchools {
		spellCrit = append(spellCrit, sheet.spellCrit)
	}
	auras := []map[string]any{}
	for _, id := range sheet.auras {
		auras = append(auras, map[string]any{"id": id, "stacks": 1, "durationMs": -1, "fromSelf": true})
	}
	var attacks []map[string]any
	crit := map[string]float64{}
	for _, kind := range []string{"mainhand", "offhand", "ranged"} {
		a := sheet.attacks[kind]
		if a == nil {
			a = &serverAttack{unhastedMs: 2000, hastedMs: 2000}
		}
		crit[kind] = a.crit
		attacks = append(attacks, map[string]any{"type": kind, "unhastedMs": a.unhastedMs, "hastedMs": a.hastedMs,
			"damageMin": a.damageMin, "damageMax": a.damageMax, "attackPower": 1000, "expertiseReduction": a.expertise})
	}
	body := map[string]any{
		"schema": 1, "command": "info",
		"attacker": map[string]any{
			"name": "SvGchBot", "level": 80, "isPlayer": true, "class": s.Class, "race": s.Race,
			"spellPower": schools,
			"hitMods":    map[string]float64{"melee": sheet.hitMelee, "ranged": sheet.hitRanged, "spell": sheet.spellHit},
			"attacks":    attacks,
			"sheet": map[string]any{"critMainhand": crit["mainhand"], "critOffhand": crit["offhand"],
				"critRanged": crit["ranged"], "spellCrit": spellCrit},
			"auras": auras,
		},
		"target": map[string]any{"name": "Dummy", "entry": boss, "level": 83, "worldBoss": true, "creatureType": 5,
			"resistances": []float64{10643, 0, 0, 0, 0, 0, 0}, "auras": []any{}},
		"character": goldcharCharacterJSON(s),
	}
	text, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var rec goldcharRecord
	if err := json.Unmarshal(text, &rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

// simSheet is the sheet a server agreeing with the sim would record for the setup, its whole numbers
// truncated the way the module's are.
func simSheet(t *testing.T, s goldcharSetup) serverSheet {
	t.Helper()
	d, err := newGoldcharSim(goldcharInfo(t, s, serverSheet{}).damageRecord, s)
	if err != nil {
		t.Fatal(err)
	}
	d.start()
	st := d.char.GetStats()
	sheet := serverSheet{
		spellPower: math.Floor(st[stats.SpellPower]),
		spellCrit:  st[stats.SpellCrit] / core.CritRatingPerCritChance,
		spellHit:   st[stats.SpellHit] / core.SpellHitRatingPerHitChance,
		attacks:    map[string]*serverAttack{},
	}
	for _, hand := range d.sheetHands() {
		unhasted := hand.weapon.SwingSpeed * 1000
		item := d.char.Equipment[hand.slot]
		sheet.attacks[hand.attack] = &serverAttack{
			unhastedMs: int32(unhasted),
			hastedMs:   int32(unhasted / hand.speed),
			damageMin:  item.WeaponDamageMin,
			damageMax:  item.WeaponDamageMax,
			crit:       (st[stats.MeleeCrit] + hand.spell.BonusCritRating) / core.CritRatingPerCritChance,
			expertise:  hand.spell.ExpertisePercentage() * 100,
		}
		hit := (st[stats.MeleeHit] + hand.spell.BonusHitRating) / core.MeleeHitRatingPerHitChance
		switch hand.attack {
		case "mainhand":
			sheet.hitMelee = hit
		case "ranged":
			sheet.hitRanged = hit
		}
	}
	return sheet
}

func checkNames(checks []Check) (names, failed []string) {
	for _, c := range checks {
		names = append(names, c.Name)
		if !c.Passed {
			failed = append(failed, c.Name)
		}
	}
	return names, failed
}

// Every setup's character builds in the sim and checks out against a sheet that agrees with it, with
// checks for each hand the sim swings.
func TestGoldcharInfoAgreeingSheet(t *testing.T) {
	needsItemDB(t)
	wantHands := map[string][]string{
		"WarriorFury": {"mainhand", "offhand"}, "WarriorArms": {"mainhand"}, "RogueCombat": {"mainhand", "offhand"},
		"HunterBM": {"ranged"}, "DruidFeral": {"mainhand"}, "MageFrost": nil, "PriestShadow": nil,
	}
	for _, s := range allGoldcharSetups(t) {
		checks := checkGoldcharInfo(goldcharInfo(t, s, simSheet(t, s)), s)
		names, failed := checkNames(checks)
		if len(failed) > 0 {
			t.Errorf("%s: failed %v: %s", s.Name, failed, strings.Join(failedChecks(checks), "; "))
		}
		var hands []string
		for _, name := range names {
			if hand, ok := strings.CutSuffix(name, " speed ms"); ok && !strings.HasSuffix(hand, " hasted") {
				hands = append(hands, hand)
			}
		}
		t.Logf("%s: %d checks, hands %v", s.Name, len(checks), hands)
		if want, ok := wantHands[s.Name]; ok && !slices.Equal(hands, want) {
			t.Errorf("%s: hands %v, want %v", s.Name, hands, want)
		}
	}
}

// The weapons' speed and damage the live item_template has, and the speed the server swings at in Cat
// Form, where it still reports the staff's damage.
func TestGoldcharInfoOnServerWeapons(t *testing.T) {
	needsItemDB(t)
	for _, c := range []struct {
		setup   string
		attacks map[string]serverAttack
	}{
		{"WarriorFury", map[string]serverAttack{
			"mainhand": {unhastedMs: 3400, damageMin: 606, damageMax: 910},
			"offhand":  {unhastedMs: 3400, damageMin: 606, damageMax: 910},
		}},
		{"RogueCombat", map[string]serverAttack{
			"mainhand": {unhastedMs: 2600, damageMin: 356, damageMax: 535},
			"offhand":  {unhastedMs: 1400, damageMin: 153, damageMax: 285},
		}},
		{"HunterBM", map[string]serverAttack{
			"ranged": {unhastedMs: 2900, damageMin: 376, damageMax: 646},
		}},
		{"DruidFeral", map[string]serverAttack{
			"mainhand": {unhastedMs: 1000, damageMin: 428, damageMax: 642},
		}},
	} {
		s := goldcharSetupNamed(t, c.setup)
		sheet := simSheet(t, s)
		for kind, server := range c.attacks {
			a := sheet.attacks[kind]
			if a == nil {
				t.Errorf("%s: the sim doesn't swing the %s", c.setup, kind)
				continue
			}
			a.unhastedMs, a.damageMin, a.damageMax = server.unhastedMs, server.damageMin, server.damageMax
		}
		checks := checkGoldcharInfo(goldcharInfo(t, s, sheet), s)
		if _, failed := checkNames(checks); len(failed) > 0 {
			t.Errorf("%s: %s", c.setup, strings.Join(failedChecks(checks), "; "))
		}
	}
}

func TestGoldcharInfoCatchesMismatches(t *testing.T) {
	needsItemDB(t)
	for _, c := range []struct {
		name   string
		setup  string
		change func(*serverSheet)
		failed []string
	}{
		{"half a point more crit", "WarriorFury", func(s *serverSheet) { s.attacks["mainhand"].crit += 0.5 },
			[]string{"mainhand crit %"}},
		{"one expertise point more", "RogueCombat", func(s *serverSheet) { s.attacks["offhand"].expertise += 0.25 },
			[]string{"offhand expertise %"}},
		{"a hit talent the sim lacks", "HunterMM", func(s *serverSheet) { s.hitRanged++ },
			[]string{"ranged hit %"}},
		{"hasted 3 ms slower", "ShamanEnhancement", func(s *serverSheet) { s.attacks["offhand"].hastedMs += 3 },
			[]string{"offhand hasted speed ms"}},
		{"a different weapon", "DeathknightBlood", func(s *serverSheet) { s.attacks["mainhand"].damageMax += 10 },
			[]string{"mainhand weapon damage max"}},
		{"truncated spell power", "MageFrost", func(s *serverSheet) { s.spellPower-- }, nil},
		{"spell power a stat bonus higher", "MageFrost", func(s *serverSheet) { s.spellPower += 40 },
			[]string{"spell power"}},
		{"one school above the rest", "WarlockAffliction",
			func(s *serverSheet) { s.schoolPower = map[string]float64{"shadow": s.spellPower + 100} }, nil},
		{"spell crit from a talent the sim lacks", "DruidBalance", func(s *serverSheet) { s.spellCrit += 3 },
			[]string{"spell crit %"}},
		{"spell hit short", "PriestShadow", func(s *serverSheet) { s.spellHit-- }, []string{"spell hit %"}},
		// Balance of Power 2/2 is 4% school hit for every magic school, which the sheet leaves out
		{"Balance of Power off the sheet", "DruidBalance", func(s *serverSheet) {
			s.spellHit -= 4
			s.auras = append(s.auras, 33596)
		}, nil},
		{"Balance of Power off the sheet and the auras", "DruidBalance", func(s *serverSheet) { s.spellHit -= 4 },
			[]string{"spell hit %"}},
		// Suppression's is fire and shadow only, where the sim gives every school its 3%
		{"Suppression off the sheet", "WarlockAffliction", func(s *serverSheet) {
			s.spellHit -= 3
			s.auras = append(s.auras, 18176)
		}, []string{"spell hit %"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := goldcharSetupNamed(t, c.setup)
			sheet := simSheet(t, s)
			c.change(&sheet)
			checks := checkGoldcharInfo(goldcharInfo(t, s, sheet), s)
			if _, failed := checkNames(checks); !slices.Equal(failed, c.failed) {
				t.Errorf("failed %v, want %v: %s", failed, c.failed, strings.Join(failedChecks(checks), "; "))
			}
		})
	}
}

func TestGoldcharRecordsGroupByBot(t *testing.T) {
	s := goldcharSetupNamed(t, "WarriorFury")
	info := goldcharInfo(t, s, serverSheet{})
	damage := func(name string, spell int32) goldcharRecord {
		var rec goldcharRecord
		rec.Command, rec.Attacker.Name, rec.Spell.ID = "damage", name, spell
		return rec
	}
	other := info
	other.Attacker.Name = "SvGchOther"

	bots, err := goldcharBots([]goldcharRecord{info, damage("SvGchBot", 47502), other, damage("SvGchBot", 57755)})
	if err != nil || len(bots) != 2 || len(bots[0].damage) != 2 || len(bots[1].damage) != 0 {
		t.Errorf("bots %+v, %v", bots, err)
	}
	if _, err := goldcharBots([]goldcharRecord{damage("SvGchBot", 47502)}); err == nil {
		t.Error("a bot without an info record grouped")
	}
	if _, err := goldcharBots([]goldcharRecord{info, info}); err == nil {
		t.Error("a bot with two info records grouped")
	}
	pet := info
	pet.Character = nil
	if _, err := goldcharBots([]goldcharRecord{info, pet}); err == nil {
		t.Error("an info record without a character grouped")
	}
}

func TestGoldcharInfoMatchesOneSetup(t *testing.T) {
	setups := allGoldcharSetups(t)
	for _, s := range setups {
		info := goldcharInfo(t, s, serverSheet{})
		if got, err := matchGoldcharSetup(setups, &info); err != nil || got.Name != s.Name {
			t.Errorf("%s matched %s, %v", s.Name, got.Name, err)
		}
	}

	s := goldcharSetupNamed(t, "DeathknightFrost")
	for name, change := range map[string]func(c *goldcharCharacter){
		"a talent short":   func(c *goldcharCharacter) { c.Talents = c.Talents[1:] },
		"no glyph":         func(c *goldcharCharacter) { c.Glyphs = make([]int32, 6) },
		"another item":     func(c *goldcharCharacter) { c.Equipment[0].ItemID++ },
		"an item too many": func(c *goldcharCharacter) { c.Equipment = append(c.Equipment, c.Equipment[0]) },
	} {
		info := goldcharInfo(t, s, serverSheet{})
		change(info.Character)
		if got, err := matchGoldcharSetup(setups, &info); err == nil {
			t.Errorf("%s matched %s", name, got.Name)
		}
	}
	info := goldcharInfo(t, s, serverSheet{})
	info.Attacker.Race = 1
	if got, err := matchGoldcharSetup(setups, &info); err == nil {
		t.Errorf("a human matched %s", got.Name)
	}
}

func TestGoldcharDamageHoldsTheShapedReasons(t *testing.T) {
	shaped := func(setup string, spell int32, reason string) []Check {
		s := goldcharSetupNamed(t, setup)
		rec := probeRecord(t, s.Class, s.Race, spell)
		rec.Shaped = reason
		return checkGoldcharDamage(rec, s)
	}
	for _, c := range []struct {
		name   string
		checks []Check
		passed bool
	}{
		{"the reason the list expects", shaped("MageFire", 42833, "target state: Molten Fury (31680)"), true},
		{"another reason", shaped("MageFire", 42833, "script spell_mage_fireball"), false},
		{"compared but shaped", shaped("MageFrost", 42873, "script spell_mage_fire_blast"), false},
		{"expected shaped but compared", shaped("MageArcane", 44781, ""), false},
		{"not a probe", shaped("MageFrost", 42833, ""), false},
	} {
		if len(c.checks) != 1 || c.checks[0].Passed != c.passed {
			t.Errorf("%s: %+v, want one check that passes=%v", c.name, c.checks, c.passed)
		}
	}
}

// unlistedStartingAuras are auras the sim has up from the start that the server character goes
// without, since they change nothing the replay compares.
var unlistedStartingAuras = map[int32]string{
	53307: "Thorns, a damage shield the druid specs put on themselves",
	50449: "Bloody Vengeance, which the sim keeps up at no stacks to count the talent's procs",
}

// serverStartingAuras are auras the server character can't go without, which the replay puts on the sim
// character although the sim doesn't start with them.
var serverStartingAuras = map[int32]string{
	48266: "Blood Presence: a DK always has a presence, Blood from creation; the sim's rotation picks one",
}

// imbueSpells are the spells that put on the temporary enchants the sim gives a main hand by itself.
var imbueSpells = map[int32]int32{3781: 58790, 3780: 58789} // Flametongue Weapon ranks 10 and 9

// classRaidBuffs are the raid buffs the sim's classes give themselves, by proto.RaidBuffs field, with
// the aura that stands for each on the server. 0: one the server character has without a cast, from a
// passive talent or with its form.
var classRaidBuffs = map[protoreflect.Name]int32{
	"horn_of_winter": 57623, "gift_of_the_wild": 48469, "arcane_brilliance": 42995, "power_word_fortitude": 48161,
	"divine_spirit": 48073, "shadow_protection": 48169, "trueshot_aura": 19506, "thorns": 53307,
	"moonkin_aura": 0, "leader_of_the_pack": 0, "rampage": 0, "unleashed_rage": 0, "elemental_oath": 0,
	"arcane_empowerment": 0, "abominations_might": 0, "icy_talons": 0,
}

// The server character has to start the way the sim starts the spec, so each setup lists the auras
// the e2e casts for that: whatever aura with a spell id the sim has up after a reset that isn't a glyph
// or passive (Bone Shield is a talent, but only up once cast), and the raid buffs the class gives
// itself, which the sim adds as stats.
func TestGoldcharAurasCoverTheSimsStart(t *testing.T) {
	needsItemDB(t)
	listed := func(s goldcharSetup, id int32) bool {
		_, unlisted := unlistedStartingAuras[id]
		return unlisted || slices.Contains(s.Auras, id)
	}
	for _, s := range allGoldcharSetups(t) {
		d, err := newGoldcharSim(goldcharInfo(t, s, serverSheet{}).damageRecord, s)
		if err != nil {
			t.Fatal(err)
		}
		// without start's mirrored auras
		d.sim.Reset()
		for _, aura := range d.char.GetAuras() {
			id := aura.ActionID.SpellID
			if !aura.IsActive() || id == 0 || listed(s, id) ||
				slices.ContainsFunc(s.Glyphs, func(g goldcharGlyph) bool { return g.Spell == id }) {
				continue
			}
			// a passive talent or racial is on the server character on its own
			if spell := serverdata.SpellByID(id); spell != nil && spell.Flags&serverdata.FlagPassive != 0 {
				continue
			}
			t.Errorf("%s starts with %d (%s), which isn't in its auras", s.Name, id, aura.Label)
		}
		for _, id := range s.Auras {
			if _, ok := serverStartingAuras[id]; ok {
				continue
			}
			if aura := d.char.GetAuraByID(core.ActionID{SpellID: id}); aura != nil && !aura.IsActive() {
				t.Errorf("%s: the sim has aura %d (%s) but doesn't start with it", s.Name, id, aura.Label)
			}
		}

		var imbue int32
		if enchant := d.char.Equipment[proto.ItemSlot_ItemSlotMainHand].TempEnchant; enchant != 0 {
			var ok bool
			if imbue, ok = imbueSpells[enchant]; !ok {
				t.Errorf("%s: the sim imbues the main hand with enchant %d, which imbueSpells doesn't know", s.Name, enchant)
			}
		}
		if s.Imbue != imbue {
			t.Errorf("%s: imbue %d, want %d for the sim's main hand", s.Name, s.Imbue, imbue)
		}

		buffs := &proto.RaidBuffs{}
		d.sim.Raid.Parties[0].Players[0].AddRaidBuffs(buffs)
		buffs.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
			switch id, ok := classRaidBuffs[fd.Name()]; {
			case !ok:
				t.Errorf("%s gives itself %s, which classRaidBuffs doesn't know", s.Name, fd.Name())
			case id != 0 && !listed(s, id):
				t.Errorf("%s gives itself %s, so its auras need %d", s.Name, fd.Name(), id)
			}
			return true
		})
	}
}

func needsItemDB(t *testing.T) {
	t.Helper()
	if !core.WITH_DB {
		t.Skip("the setups' gear needs the item database (-tags=with_db)")
	}
}

// Every spell a setup expects compared has a sim declaration for each probed effect on the setup's
// character, and the sim's code deals something for it with the base pinned. Less than the base can be
// right: Glyph of Moonfire takes 90% off Moonfire's direct damage.
func TestGoldcharReplayCoversTheProbes(t *testing.T) {
	needsItemDB(t)
	for _, s := range allGoldcharSetups(t) {
		for _, p := range s.Probes {
			if p.Shaped != "" {
				continue
			}
			rec := probeRecord(t, s.Class, s.Race, p.ID)
			if len(rec.Effects) == 0 {
				t.Errorf("%s %d %s: serverdata has no damage, dot or heal effect", s.Name, p.ID, p.Name)
				continue
			}
			d, err := newGoldcharSim(rec, s)
			if err != nil {
				t.Errorf("%s %d: %v", s.Name, p.ID, err)
				continue
			}
			for _, e := range rec.Effects {
				decl := d.declaration(e)
				if decl == nil {
					t.Errorf("%s %d %s e%d %s: no sim declaration", s.Name, p.ID, p.Name, e.Index, e.Kind)
					continue
				}
				for _, base := range []float64{e.Base.Min, e.Base.Max} {
					if amount, err := d.amount(*decl, e, base); err != nil || amount <= 0 {
						t.Errorf("%s %d e%d %s at base %.0f: %.2f, %v", s.Name, p.ID, e.Index, e.Kind, base, amount, err)
					}
				}
			}
		}
	}
}

// The talents reach the sim from the talent string, not from auras: MageFrost's Spell Impact 3/3 adds
// 6% to Scorch, so (382 + int(0.429 * 520)) * 1.06 = 641 at the bottom and 713 at the top.
func TestGoldcharDamageOnHandCheckedRecords(t *testing.T) {
	needsItemDB(t)
	scorch := func(lo, hi float64) damageRecord {
		return handRecord(t, mage, 8, 42859, nil, "", fmt.Sprintf(`
			{"index": 0, "kind": "damage", "coefficient": 0.429, "base": {"min": 382, "max": 450},
				"taken": {"min": %.0f, "max": %.0f}, "crit": {"amount": 10000, "result": 15000}}`, lo, hi))
	}
	s := goldcharSetupNamed(t, "MageFrost")
	for _, c := range []struct {
		name   string
		rec    damageRecord
		failed []string
	}{
		{"with Spell Impact", scorch(641, 713), nil},
		{"without it", scorch(605, 673), []string{"e0 damage min", "e0 damage max"}},
	} {
		checks := checkGoldcharDamage(c.rec, s)
		if _, failed := checkNames(checks); len(checks) == 0 || !slices.Equal(failed, c.failed) {
			t.Errorf("%s: failed %v, want %v: %s", c.name, failed, c.failed, strings.Join(failedChecks(checks), "; "))
		}
	}
}
