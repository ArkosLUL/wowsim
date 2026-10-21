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
	"time"

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
		"character": {"talents": [12320], "glyphs": [0, 0, 0, 0, 0, 0], "equipment": [{"slot": 15, "itemId": 51227,
			"enchant": 3789, "temp": 0, "gems": [40117, 0, 0], "socketBonus": 0, "prismatic": 0, "inactive": []}]}}`,
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

var droppedGoldchar = []goldcharDrop{
	{"DeathknightBlood", 0, "armor penetration %"},
	{"DruidBalance", 0, "arcane spell crit %"},
	{"DruidBalancePhase3", 0, "arcane spell crit %"},
	{"MageFire", 0, "arcane spell crit %"},
	{"MageFrostFire", 0, "arcane spell crit %"},
	{"WarlockDestruction", 0, "fire spell crit %"},
	{"WarriorArms", 0, "armor penetration %"},
	{"RogueCombat", 0, "mainhand hasted speed ms"},
	{"RogueCombat", 0, "offhand hasted speed ms"},
	{"PriestShadow", 0, "holy spell power"},
	{"PriestShadow", 0, "fire spell power"},
	{"PriestShadow", 0, "nature spell power"},
	{"PriestShadow", 0, "frost spell power"},
	{"PriestShadow", 0, "shadow spell power"},
	{"PriestShadow", 0, "arcane spell power"},
}

// goldcharRerecord are setups whose enchants changed after goldcharFixture was recorded. Until a rerun replaces
// their records, the replay builds them with the enchants their records wear. A listed setup its record
// already wears fails, so the entry goes with the rerun.
var goldcharRerecord = []string{}

// withRecordedEnchants is the setup with the enchants the character wears in place of its own.
func (s goldcharSetup) withRecordedEnchants(c *goldcharCharacter) goldcharSetup {
	s.Enchants = nil
	for _, e := range c.Equipment {
		for _, id := range []int32{e.Enchant, e.Prismatic} {
			if id != 0 {
				s.Enchants = append(s.Enchants, goldcharEnchant{Slot: e.Slot, Enchant: id})
			}
		}
	}
	return s
}

// goldcharReport reports a record's checks, holding them to droppedGoldchar.
type goldcharReport struct {
	used    map[goldcharDrop]bool
	checked int
	// info records from before the module wrote maxPower, and a player's regen
	noMaxPower, noRegen int
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
	worn := map[string]goldcharSetup{}
	bare := 0
	for _, bot := range bots {
		s, err := matchGoldcharSetup(setups, bot.info)
		if err == nil && slices.Contains(goldcharRerecord, s.Name) {
			if _, wornErr := s.wornBy(bot.info.Character); wornErr == nil {
				err = fmt.Errorf("%s's record wears its enchants, so take it off goldcharRerecord", s.Name)
			} else {
				s = s.withRecordedEnchants(bot.info.Character)
				t.Logf("%s: replayed with the enchants its record wears", s.Name)
			}
		}
		if err == nil {
			s, err = s.wornBy(bot.info.Character)
		}
		if err != nil {
			t.Error(err)
			continue
		}
		if other := bySetup[s.Name]; other != nil {
			t.Errorf("%s and %s both play %s", other.name, bot.name, s.Name)
			continue
		}
		bySetup[s.Name], worn[s.Name] = bot, s
		if !s.geared {
			bare++
		}
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
			s := worn[s.Name]
			report.checks(t, s.Name, 0, bot.info.label(), checkGoldcharInfo(*bot.info, s))
			if bot.info.regen == nil {
				report.noRegen++
			}
			if bot.info.raw.Attacker.MaxPower == nil {
				report.noMaxPower++
				t.Logf("%s: no maxPower, so max power isn't compared", bot.info.label())
			}

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
	t.Logf("%d checks over %d records of %d bots; %d info records have no max power, %d no regen, and %d bots "+
		"predate the enchantments in the record, so they replay without enchants and gems", report.checked,
		len(records), len(bots), report.noMaxPower, report.noRegen, bare)
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
	// schools whose crit isn't spellCrit
	schoolCrit          map[string]float64
	hitMelee, hitRanged float64
	attacks             map[string]*serverAttack
	auras               []int32
	// the talent sweep's sheet fields besides spell power and crit, in the sim's units; 0 when missing
	fields     sweepSheet
	powerType  uint32
	noMaxPower bool
	// nil for a record from before the module wrote it
	regen *timingRegen
	// a geared setup's socket bonuses by server slot, and the enchantments the server skips
	socketBonus map[uint8]bool
	inactive    map[uint8][]goldcharInactive
}

func goldcharGeared(s goldcharSetup) goldcharSetup {
	s.geared = true
	return s
}

// goldcharGearJSON adds a geared setup's enchants and gems to the character block's items as the module
// writes them, with the sheet's socket bonuses and skipped enchantments.
func goldcharGearJSON(character map[string]any, s goldcharSetup, sheet serverSheet) {
	for _, item := range character["equipment"].([]map[string]any) {
		slot := item["slot"].(uint8)
		var enchant, prismatic, bonus int32
		for _, e := range s.Enchants {
			switch {
			case e.Slot != slot:
			case socketEnchants[slot] == e.Enchant:
				prismatic = e.Enchant
			default:
				enchant = e.Enchant
			}
		}
		gems := make([]int32, maxGemSockets)
		for _, g := range s.Gems {
			if g.Slot == slot {
				gems[g.Socket] = g.Gem
			}
		}
		// the bonus's SpellItemEnchantment id; the replay only reads whether there is one
		if sheet.socketBonus[slot] {
			bonus = 1
		}
		inactive := sheet.inactive[slot]
		if inactive == nil {
			inactive = []goldcharInactive{}
		}
		item["enchant"], item["temp"], item["gems"], item["socketBonus"], item["prismatic"], item["inactive"] =
			enchant, 0, gems, bonus, prismatic, inactive
	}
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
	for _, school := range magicSchools {
		crit, ok := sheet.schoolCrit[school]
		if !ok {
			crit = sheet.spellCrit
		}
		spellCrit = append(spellCrit, crit)
	}
	auras := []map[string]any{}
	for _, id := range sheet.auras {
		auras = append(auras, map[string]any{"id": id, "stacks": 1, "durationMs": -1, "fromSelf": true})
	}
	v := sheet.fields
	var attacks []map[string]any
	crit := map[string]float64{}
	for _, kind := range []string{"mainhand", "offhand", "ranged"} {
		a := sheet.attacks[kind]
		if a == nil {
			a = &serverAttack{unhastedMs: 2000, hastedMs: 2000}
		}
		crit[kind] = a.crit
		ap := v["attack power"]
		if kind == "ranged" {
			ap = v["ranged attack power"]
		}
		attacks = append(attacks, map[string]any{"type": kind, "unhastedMs": a.unhastedMs, "hastedMs": a.hastedMs,
			"damageMin": a.damageMin, "damageMax": a.damageMax, "attackPower": ap, "missChanceTaken": 5,
			"expertiseReduction": a.expertise})
	}
	attacker := map[string]any{
		"name": "SvGchBot", "level": 80, "isPlayer": true, "class": s.Class, "race": s.Race,
		"maxHealth": v["max health"], "powerType": sheet.powerType,
		"stats": map[string]float64{"strength": v["strength"], "agility": v["agility"], "stamina": v["stamina"],
			"intellect": v["intellect"], "spirit": v["spirit"]},
		"resistances": []float64{v["armor"], 0, v["fire resistance"], v["nature resistance"], v["frost resistance"],
			v["shadow resistance"], v["arcane resistance"]},
		"spellPower":              schools,
		"hitMods":                 map[string]float64{"melee": sheet.hitMelee, "ranged": sheet.hitRanged, "spell": sheet.spellHit},
		"armorPenetrationAuraPct": v["armor penetration %"],
		"defense":                 map[string]float64{"dodge": v["dodge %"], "parry": v["parry %"], "block": v["block %"]},
		"attacks":                 attacks,
		"sheet": map[string]any{"critMainhand": crit["mainhand"], "critOffhand": crit["offhand"],
			"critRanged": crit["ranged"], "spellCrit": spellCrit, "shieldBlock": v["block value"]},
		"auras": auras,
	}
	if !sheet.noMaxPower {
		power := sweepPowerFields[sheet.powerType]
		attacker["maxPower"] = v[power.name] * power.scale
	}
	if sheet.regen != nil {
		attacker["regen"] = sheet.regen
	}
	character := goldcharCharacterJSON(s)
	if s.geared {
		goldcharGearJSON(character, s, sheet)
	}
	body := map[string]any{
		"schema": 1, "command": "info",
		"attacker": attacker,
		"target": map[string]any{"name": "Dummy", "entry": boss, "level": 83, "worldBoss": true, "creatureType": 5,
			"resistances": []float64{10643, 0, 0, 0, 0, 0, 0}, "auras": []any{}},
		"character": character,
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

// newSweepSim is the sheet of the setup's character against the record's dummy.
func newSweepSim(rec damageRecord, s goldcharSetup) sweepSim {
	d, err := newGoldcharSim(rec, s)
	if err != nil {
		return sweepSim{err: err}
	}
	return d.sweepSim()
}

// simSheet is the sheet a server agreeing with the sim would record for the setup, its whole numbers
// truncated the way the module's are.
func simSheet(t *testing.T, s goldcharSetup) serverSheet {
	t.Helper()
	target := goldcharInfo(t, s, serverSheet{}).damageRecord
	sim := newSweepSim(target, s)
	if sim.err != nil {
		t.Fatal(sim.err)
	}
	d, err := newGoldcharSim(target, s)
	if err != nil {
		t.Fatal(err)
	}
	d.start()
	st := d.char.GetStats()
	sheet := serverSheet{
		spellPower: math.Floor(st[stats.SpellPower]),
		spellCrit:  st[stats.SpellCrit] / core.CritRatingPerCritChance,
		spellHit:   st[stats.SpellHit] / core.SpellHitRatingPerHitChance,
		schoolCrit: map[string]float64{},
		attacks:    map[string]*serverAttack{},
		fields:     sweepSheet{},
	}
	for name, value := range sim.values {
		switch name {
		case "dodge %", "parry %", "block %", "armor penetration %":
			sheet.fields[name] = value
		default:
			sheet.fields[name] = math.Floor(value + 1e-6)
		}
	}
	for _, school := range magicSchools {
		if crit, ok := sim.values[school+" spell crit %"]; ok {
			sheet.schoolCrit[school] = crit
		}
	}
	// a druid in Cat or Bear Form has mana too, but the server shows the form's power
	for _, powerType := range []uint32{6, 3, 1, 0} {
		power := sweepPowerFields[powerType]
		if value, ok := sim.values[power.name]; ok {
			sheet.powerType, sheet.fields[power.name] = powerType, math.Floor(value*power.scale+1e-6)/power.scale
			break
		}
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
	if s.geared {
		sheet.socketBonus = map[uint8]bool{}
		for i, slot := range serverEquipSlots {
			if on, ok := simSocketBonus(&d.char.Equipment[i]); ok {
				sheet.socketBonus[slot] = on
			}
		}
	}
	sheet.regen = simRegen(d, sheet.fields)
	return sheet
}

// simRegen is the regen a server agreeing with the sim would record, read at the sheet's whole intellect
// and spirit the way checkRegen reads it.
func simRegen(d *damageSim, fields sweepSheet) *timingRegen {
	d.start()
	unit := &d.char.Unit
	regen := &timingRegen{Flat: make([]float64, 7), Interrupted: make([]float64, 7), Rune: make([]float64, 4),
		ManaRate: 1, EnergyRate: 1}
	if unit.HasManaBar() {
		setStat(d.sim, unit, stats.Intellect, fields["intellect"])
		setStat(d.sim, unit, stats.Spirit, fields["spirit"])
		regen.Flat[0], regen.Interrupted[0] = unit.ManaRegenPerSecondWhileNotCasting(), unit.ManaRegenPerSecondWhileCasting()
	}
	if unit.HasEnergyBar() {
		regen.Flat[3] = core.EnergyPerTick*unit.EnergyTickMultiplier*float64(time.Second/core.EnergyTickDuration) - 10
	}
	if unit.HasRunicPowerBar() {
		cooldown := float64((&timingSim{damageSim: d}).runeCooldown())
		for i := range regen.Rune {
			regen.Rune[i] = 1000 / cooldown
		}
	}
	return regen
}

// everySchool is a check name for each magic school, e.g. "holy spell power" for " spell power".
func everySchool(suffix string) []string {
	var names []string
	for _, school := range magicSchools {
		names = append(names, school+suffix)
	}
	return names
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

// Every setup's character builds in the sim, bare and geared, and checks out against a sheet that
// agrees with it, with checks for each hand the sim swings, the regen of its power bar, and when geared
// each socket bonus and the meta gem.
func TestGoldcharInfoAgreeingSheet(t *testing.T) {
	needsItemDB(t)
	wantHands := map[string][]string{
		"WarriorFury": {"mainhand", "offhand"}, "WarriorArms": {"mainhand"}, "RogueCombat": {"mainhand", "offhand"},
		"HunterBM": {"ranged"}, "DruidFeral": {"mainhand"}, "MageFrost": nil, "PriestShadow": nil,
	}
	wantRegen := map[string][]string{
		"MageFrost": {"mana regen", "mana regen while casting"}, "RogueCombat": {"energy regen"},
		"DruidFeral": {"energy regen"}, "DeathknightBlood": {"rune 0 cooldown", "rune 1 cooldown", "rune 2 cooldown"},
		"WarriorFury": nil,
	}
	for _, setup := range allGoldcharSetups(t) {
		for _, s := range []goldcharSetup{setup, goldcharGeared(setup)} {
			label := fmt.Sprintf("%s geared=%v", s.Name, s.geared)
			checks := checkGoldcharInfo(goldcharInfo(t, s, simSheet(t, s)), s)
			names, failed := checkNames(checks)
			if len(failed) > 0 {
				t.Errorf("%s: failed %v: %s", label, failed, strings.Join(failedChecks(checks), "; "))
			}
			var hands, regen []string
			bonuses := 0
			for _, name := range names {
				if hand, ok := strings.CutSuffix(name, " speed ms"); ok && !strings.HasSuffix(hand, " hasted") {
					hands = append(hands, hand)
				}
				if strings.Contains(name, " regen") || strings.HasPrefix(name, "rune ") {
					regen = append(regen, name)
				}
				if strings.HasSuffix(name, " socket bonus") {
					bonuses++
				}
			}
			t.Logf("%s: %d checks, hands %v, regen %v, %d socket bonuses", label, len(checks), hands, regen, bonuses)
			if want, ok := wantHands[s.Name]; ok && !slices.Equal(hands, want) {
				t.Errorf("%s: hands %v, want %v", label, hands, want)
			}
			if want, ok := wantRegen[s.Name]; ok && !slices.Equal(regen, want) {
				t.Errorf("%s: regen checks %v, want %v", label, regen, want)
			}
			if s.geared != (bonuses > 0) || s.geared != slices.Contains(names, "slot 0 meta gem") {
				t.Errorf("%s: %d socket bonus checks, meta gem checked: %v", label, bonuses, slices.Contains(names, "slot 0 meta gem"))
			}
		}
	}
}

// The replay builds a geared record's character with the setup's enchants and gems, and a bare one's
// without: each fails the other's sheet.
func TestGoldcharReplayWearsTheGear(t *testing.T) {
	needsItemDB(t)
	for _, bare := range allGoldcharSetups(t) {
		geared := goldcharGeared(bare)
		bareSheet, gearedSheet := simSheet(t, bare), simSheet(t, geared)
		bareSheet.socketBonus = gearedSheet.socketBonus
		if _, failed := checkNames(checkGoldcharInfo(goldcharInfo(t, geared, bareSheet), geared)); len(failed) == 0 {
			t.Errorf("%s: a geared record with the bare character's sheet passes", bare.Name)
		}
		if _, failed := checkNames(checkGoldcharInfo(goldcharInfo(t, bare, gearedSheet), bare)); len(failed) == 0 {
			t.Errorf("%s: a bare record with the geared character's sheet passes", bare.Name)
		}
	}
}

// A record's character wears the setup's enchants and gems, or predates the module writing them; nothing
// in between replays.
func TestGoldcharWornBy(t *testing.T) {
	s := goldcharSetupNamed(t, "PaladinProtection")
	worn := func(change func(c *goldcharCharacter)) (bool, error) {
		info := goldcharInfo(t, goldcharGeared(s), serverSheet{})
		change(info.Character)
		got, err := s.wornBy(info.Character)
		return got.geared, err
	}
	slot := func(c *goldcharCharacter, slot uint8) *goldcharEquipped {
		i := slices.IndexFunc(c.Equipment, func(e goldcharEquipped) bool { return e.Slot == slot })
		if i < 0 {
			t.Fatalf("no slot %d", slot)
		}
		return &c.Equipment[i]
	}

	if geared, err := worn(func(*goldcharCharacter) {}); !geared || err != nil {
		t.Errorf("the setup's gear: geared %v, %v", geared, err)
	}
	if geared, err := worn(func(c *goldcharCharacter) {
		for i := range c.Equipment {
			c.Equipment[i] = goldcharEquipped{Slot: c.Equipment[i].Slot, ItemID: c.Equipment[i].ItemID}
		}
	}); geared || err != nil {
		t.Errorf("a record from before the enchantments: geared %v, %v", geared, err)
	}
	for name, change := range map[string]func(c *goldcharCharacter){
		"nothing on": func(c *goldcharCharacter) {
			for i := range c.Equipment {
				e := &c.Equipment[i]
				e.Enchant, e.Prismatic, e.Gems = 0, 0, make([]int32, maxGemSockets)
			}
		},
		"no belt buckle":         func(c *goldcharCharacter) { slot(c, 5).Prismatic = 0 },
		"the buckle as enchant":  func(c *goldcharCharacter) { e := slot(c, 5); e.Enchant, e.Prismatic = e.Prismatic, 0 },
		"a gem in the next slot": func(c *goldcharCharacter) { e := slot(c, 5); e.Gems[1], e.Gems[2] = 0, e.Gems[1] },
		"another chest enchant":  func(c *goldcharCharacter) { slot(c, 4).Enchant++ },
	} {
		if geared, err := worn(change); err == nil {
			t.Errorf("%s: geared %v and no error", name, geared)
		}
	}
}

// gearChecks hold the server's socket bonuses and skipped enchantments to the sim's geared items.
func TestGoldcharGearChecks(t *testing.T) {
	needsItemDB(t)
	s := goldcharGeared(goldcharSetupNamed(t, "PaladinProtection"))
	agreeing := simSheet(t, s)
	var bonusSlot uint8
	for _, slot := range serverEquipSlots {
		if agreeing.socketBonus[slot] {
			bonusSlot = slot
			break
		}
	}
	if !agreeing.socketBonus[bonusSlot] {
		t.Fatalf("no item gets its socket bonus: %v", agreeing.socketBonus)
	}
	var waistEnchant int32
	for _, e := range s.Enchants {
		if e.Slot == 5 && e.Enchant != socketEnchants[5] {
			waistEnchant = e.Enchant
		}
	}

	for _, c := range []struct {
		name   string
		change func(*serverSheet)
		failed []string
	}{
		{"a socket bonus the server leaves off", func(sh *serverSheet) { sh.socketBonus[bonusSlot] = false },
			[]string{fmt.Sprintf("slot %d socket bonus", bonusSlot)}},
		{"a meta gem whose colors the gear doesn't meet", func(sh *serverSheet) {
			sh.inactive = map[uint8][]goldcharInactive{0: {{Enchant: 1, Reason: "condition"}}}
		}, []string{"slot 0 meta gem"}},
		{"an enchant the server's skill check skips", func(sh *serverSheet) {
			sh.inactive = map[uint8][]goldcharInactive{5: {{Enchant: waistEnchant, Reason: "skill"}}}
		}, []string{fmt.Sprintf("slot 5 enchant %d", waistEnchant)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			sheet := simSheet(t, s)
			c.change(&sheet)
			checks := checkGoldcharInfo(goldcharInfo(t, s, sheet), s)
			if _, failed := checkNames(checks); !slices.Equal(failed, c.failed) {
				t.Errorf("failed %v, want %v: %s", failed, c.failed, strings.Join(failedChecks(checks), "; "))
			}
		})
	}

	// the bonus is the sim's own call: an empty socket takes it off
	emptied := s
	emptied.Gems = slices.DeleteFunc(slices.Clone(s.Gems), func(g goldcharGem) bool { return g.Slot == bonusSlot && g.Socket == 0 })
	if on := simSheet(t, emptied).socketBonus[bonusSlot]; on {
		t.Errorf("slot %d keeps its socket bonus with socket 0 empty", bonusSlot)
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
			everySchool(" spell power")},
		{"one school above the rest", "WarlockAffliction",
			func(s *serverSheet) { s.schoolPower = map[string]float64{"shadow": s.spellPower + 100} },
			[]string{"shadow spell power"}},
		{"school crit from a talent the sim lacks", "DruidBalance", func(s *serverSheet) { s.schoolCrit["nature"] += 3 },
			[]string{"nature spell crit %"}},
		{"Critical Mass off the fire sheet", "MageFire", func(s *serverSheet) { s.schoolCrit["fire"] -= 6 },
			[]string{"fire spell crit %"}},
		// no holy spell in the sim's fury warrior, so its holy crit is the stat's
		{"crit in a school the sim casts nothing of", "WarriorFury", func(s *serverSheet) { s.schoolCrit["holy"] = s.spellCrit + 1 },
			[]string{"holy spell crit %"}},
		{"spell hit short", "PriestShadow", func(s *serverSheet) { s.spellHit-- }, []string{"spell hit %"}},
		{"a stamina talent the sim lacks", "RogueCombat", func(s *serverSheet) {
			s.fields["stamina"] += 20
			s.fields["max health"] += 200
		}, []string{"stamina", "max health"}},
		{"bonus armor on a ranged weapon", "HunterBM", func(s *serverSheet) { s.fields["armor"] += 21 }, []string{"armor"}},
		{"a truncated agility's armor", "HunterBM", func(s *serverSheet) { s.fields["armor"] -= 2 }, nil},
		{"Parry never learned", "WarriorFury", func(s *serverSheet) { s.fields["parry %"] = 0 }, []string{"parry %"}},
		{"dodge above the sim's", "RogueCombat", func(s *serverSheet) { s.fields["dodge %"] += 0.01 }, []string{"dodge %"}},
		{"block value from strength", "ShamanElemental", func(s *serverSheet) { s.fields["block value"] += 60 },
			[]string{"block value"}},
		{"block value without a shield", "DeathknightBlood", func(s *serverSheet) { s.fields["block value"] = 955 }, nil},
		{"block without a shield", "DeathknightBlood", func(s *serverSheet) { s.fields["block %"] = 5 }, []string{"block %"}},
		{"Blood-Gorged's armor penetration", "DeathknightBlood", func(s *serverSheet) { s.fields["armor penetration %"] = 10 },
			[]string{"armor penetration %"}},
		{"max mana short", "MageArcane", func(s *serverSheet) { s.fields["max mana"] -= 500 }, []string{"max mana"}},
		{"max mana in a record from before maxPower", "MageArcane", func(s *serverSheet) {
			s.fields["max mana"] -= 500
			s.noMaxPower = true
		}, nil},
		{"max energy in Cat Form", "DruidFeral", func(s *serverSheet) { s.fields["max energy"] += 10 }, []string{"max energy"}},
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
		{"mana regen a percent above", "MageFrost", func(s *serverSheet) { s.regen.Flat[0] *= 1.01 }, []string{"mana regen"}},
		{"mana regen while casting a percent above", "MageArcane", func(s *serverSheet) { s.regen.Interrupted[0] *= 1.01 },
			[]string{"mana regen while casting"}},
		{"a record from before regen", "MageFrost", func(s *serverSheet) {
			s.regen.Flat[0] *= 1.01
			s.regen = nil
		}, nil},
		{"an energy regen talent the sim lacks", "RogueCombat", func(s *serverSheet) { s.regen.Flat[3]++ },
			[]string{"energy regen"}},
		{"a slower blood rune", "DeathknightBlood", func(s *serverSheet) { s.regen.Rune[0] *= 0.9 },
			[]string{"rune 0 cooldown"}},
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
		for _, s := range []goldcharSetup{s, goldcharGeared(s)} {
			info := goldcharInfo(t, s, serverSheet{})
			if got, err := matchGoldcharSetup(setups, &info); err != nil || got.Name != s.Name {
				t.Errorf("%s geared=%v matched %s, %v", s.Name, s.geared, got.Name, err)
			}
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
// itself, which the sim adds as stats. That holds bare and geared; an aura only the geared character
// starts with comes with an enchant or gem (Rune of Razorice's Razor Frost), which the server character
// wears too.
func TestGoldcharAurasCoverTheSimsStart(t *testing.T) {
	needsItemDB(t)
	listed := func(s goldcharSetup, id int32) bool {
		_, unlisted := unlistedStartingAuras[id]
		return unlisted || slices.Contains(s.Auras, id)
	}
	var setups []goldcharSetup
	for _, s := range allGoldcharSetups(t) {
		setups = append(setups, s, goldcharGeared(s))
	}
	bareStart := map[string][]int32{}
	for _, s := range setups {
		name := fmt.Sprintf("%s geared=%v", s.Name, s.geared)
		d, err := newGoldcharSim(goldcharInfo(t, s, serverSheet{}).damageRecord, s)
		if err != nil {
			t.Fatal(err)
		}
		// without start's mirrored auras
		d.sim.Reset()
		for _, aura := range d.char.GetAuras() {
			id := aura.ActionID.SpellID
			if !aura.IsActive() || id == 0 {
				continue
			}
			if !s.geared {
				bareStart[s.Name] = append(bareStart[s.Name], id)
			}
			if listed(s, id) || s.geared && !slices.Contains(bareStart[s.Name], id) ||
				slices.ContainsFunc(s.Glyphs, func(g goldcharGlyph) bool { return g.Spell == id }) {
				continue
			}
			// a passive talent or racial is on the server character on its own
			if spell := serverdata.SpellByID(id); spell != nil && spell.Flags&serverdata.FlagPassive != 0 {
				continue
			}
			t.Errorf("%s starts with %d (%s), which isn't in its auras", name, id, aura.Label)
		}
		for _, id := range s.Auras {
			if _, ok := serverStartingAuras[id]; ok {
				continue
			}
			if aura := d.char.GetAuraByID(core.ActionID{SpellID: id}); aura != nil && !aura.IsActive() {
				t.Errorf("%s: the sim has aura %d (%s) but doesn't start with it", name, id, aura.Label)
			}
		}

		var imbue int32
		if enchant := d.char.Equipment[proto.ItemSlot_ItemSlotMainHand].TempEnchant; enchant != 0 {
			var ok bool
			if imbue, ok = imbueSpells[enchant]; !ok {
				t.Errorf("%s: the sim imbues the main hand with enchant %d, which imbueSpells doesn't know", name, enchant)
			}
		}
		if s.Imbue != imbue {
			t.Errorf("%s: imbue %d, want %d for the sim's main hand", name, s.Imbue, imbue)
		}

		buffs := &proto.RaidBuffs{}
		d.sim.Raid.Parties[0].Players[0].AddRaidBuffs(buffs)
		buffs.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
			switch id, ok := classRaidBuffs[fd.Name()]; {
			case !ok:
				t.Errorf("%s gives itself %s, which classRaidBuffs doesn't know", name, fd.Name())
			case id != 0 && !listed(s, id):
				t.Errorf("%s gives itself %s, so its auras need %d", name, fd.Name(), id)
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
// right: Glyph of Moonfire takes 90% off Moonfire's direct damage. Bare and geared.
func TestGoldcharReplayCoversTheProbes(t *testing.T) {
	needsItemDB(t)
	var setups []goldcharSetup
	for _, s := range allGoldcharSetups(t) {
		setups = append(setups, s, goldcharGeared(s))
	}
	for _, s := range setups {
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
// 6% to Scorch, so (382 + int(0.429 * 520)) * 1.06 = 641 at the bottom and 713 at the top. Geared, its
// Chaotic Skyflare Diamond (SPELL_AURA_MOD_CRIT_DAMAGE_BONUS 3) makes a crit 1.5 * 1.03.
func TestGoldcharDamageOnHandCheckedRecords(t *testing.T) {
	needsItemDB(t)
	scorch := func(lo, hi, crit float64) damageRecord {
		return handRecord(t, mage, 8, 42859, nil, "", fmt.Sprintf(`
			{"index": 0, "kind": "damage", "coefficient": 0.429, "base": {"min": 382, "max": 450},
				"taken": {"min": %.0f, "max": %.0f}, "crit": {"amount": 10000, "result": %.0f}}`, lo, hi, crit))
	}
	s := goldcharSetupNamed(t, "MageFrost")
	for _, c := range []struct {
		name   string
		rec    damageRecord
		s      goldcharSetup
		failed []string
	}{
		{"with Spell Impact", scorch(641, 713, 15000), s, nil},
		{"without it", scorch(605, 673, 15000), s, []string{"e0 damage min", "e0 damage max"}},
		{"geared", scorch(641, 713, 15450), goldcharGeared(s), nil},
		{"geared without the meta gem's crit", scorch(641, 713, 15000), goldcharGeared(s), []string{"e0 damage crit"}},
		{"bare with it", scorch(641, 713, 15450), s, []string{"e0 damage crit"}},
	} {
		checks := checkGoldcharDamage(c.rec, c.s)
		if _, failed := checkNames(checks); len(checks) == 0 || !slices.Equal(failed, c.failed) {
			t.Errorf("%s: failed %v, want %v: %s", c.name, failed, c.failed, strings.Join(failedChecks(checks), "; "))
		}
	}
}
