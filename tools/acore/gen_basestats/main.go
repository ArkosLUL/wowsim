// gen_basestats generates the sim's player base stats and rating conversions from an AzerothCore
// server: its gt*.dbc game tables, the world database's player_class_stats and player_race_stats,
// and the constants and formulas in Unit.h, StatSystem.cpp and Player.cpp.
//
//	tools/acore/dock.sh run ./tools/acore/gen_basestats
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// Named rating conversions the sim uses as plain constants. Each lists every rating it stands for;
// they must all agree and no class may scale them.
var namedRatings = []struct {
	goName, tsName string
	ratings        []string
}{
	{"ExpertisePerQuarterPercentReduction", "EXPERTISE_PER_QUARTER_PERCENT_REDUCTION", []string{"EXPERTISE"}},
	{"HasteRatingPerHastePercent", "HASTE_RATING_PER_HASTE_PERCENT", []string{"HASTE_MELEE", "HASTE_RANGED", "HASTE_SPELL"}},
	{"CritRatingPerCritChance", "", []string{"CRIT_MELEE", "CRIT_RANGED", "CRIT_SPELL"}},
	{"", "MELEE_CRIT_RATING_PER_CRIT_CHANCE", []string{"CRIT_MELEE"}},
	{"", "SPELL_CRIT_RATING_PER_CRIT_CHANCE", []string{"CRIT_SPELL"}},
	{"MeleeHitRatingPerHitChance", "MELEE_HIT_RATING_PER_HIT_CHANCE", []string{"HIT_MELEE", "HIT_RANGED"}},
	{"SpellHitRatingPerHitChance", "SPELL_HIT_RATING_PER_HIT_CHANCE", []string{"HIT_SPELL"}},
	{"DefenseRatingPerDefense", "DEFENSE_RATING_PER_DEFENSE", []string{"DEFENSE_SKILL"}},
	{"DodgeRatingPerDodgeChance", "DODGE_RATING_PER_DODGE_CHANCE", []string{"DODGE"}},
	{"ParryRatingPerParryChance", "PARRY_RATING_PER_PARRY_CHANCE", []string{"PARRY"}},
	{"BlockRatingPerBlockChance", "BLOCK_RATING_PER_BLOCK_CHANCE", []string{"BLOCK"}},
	{"ResilienceRatingPerCritReductionChance", "RESILIENCE_RATING_PER_CRIT_REDUCTION_CHANCE",
		[]string{"CRIT_TAKEN_MELEE", "CRIT_TAKEN_RANGED", "CRIT_TAKEN_SPELL"}},
}

// Ratings where classes differ; the sim converts them per class.
var classScaledRatings = []struct {
	name, tsName string
}{
	{"HASTE_MELEE", "MELEE_HASTE_RATING_PER_HASTE_PERCENT_BY_CLASS"},
	{"ARMOR_PENETRATION", "ARMOR_PEN_RATING_PER_PERCENT_BY_CLASS"},
}

type model struct {
	level   int
	ratings []combatRating
	gt      *gtTables

	classStats map[int]classStats
	raceStats  map[int]primaryStats

	meleeAP, rangedAP                   map[int]linearFormula
	dodgeBase, critToDodge              []float32
	dodgeCap, parryCap, missCap, dimK   []float32
	spellCritPerInt, manaRegenPerSpirit float32
}

func main() {
	dbcDir := flag.String("dbc", "/dbc/Clean", "directory with the server's gt*.dbc files")
	acDir := flag.String("ac", "/ac", "AzerothCore checkout the server was built from")
	dsn := flag.String("dsn", azerothcore.ContainerDSN(), "AzerothCore world database DSN")
	level := flag.Int("level", 80, "character level")
	goOut := flag.String("goOut", "sim/core/base_stats_auto_gen.go", "generated Go file")
	tsOut := flag.String("tsOut", "ui/core/constants/ratings_auto_gen.ts", "generated TypeScript file")
	flag.Parse()

	m, err := load(*dbcDir, *acDir, *dsn, *level)
	if err != nil {
		log.Fatal(err)
	}

	goSrc, err := renderGo(m)
	if err != nil {
		log.Fatal(err)
	}
	for path, content := range map[string][]byte{*goOut: goSrc, *tsOut: renderTS(m)} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("wrote", path)
	}
}

func load(dbcDir, acDir, dsn string, level int) (*model, error) {
	read := func(rel string) (string, error) {
		b, err := os.ReadFile(filepath.Join(acDir, rel))
		return string(b), err
	}
	unitH, err := read("src/server/game/Entities/Unit/Unit.h")
	if err != nil {
		return nil, err
	}
	statSystem, err := read("src/server/game/Entities/Unit/StatSystem.cpp")
	if err != nil {
		return nil, err
	}
	player, err := read("src/server/game/Entities/Player/Player.cpp")
	if err != nil {
		return nil, err
	}

	m := &model{level: level}
	if m.ratings, err = parseCombatRatings(unitH); err != nil {
		return nil, err
	}
	if m.meleeAP, m.rangedAP, err = parseAttackPower(statSystem); err != nil {
		return nil, err
	}
	for _, t := range []struct {
		src, name string
		out       *[]float32
	}{
		{statSystem, "m_diminishing_k", &m.dimK},
		{statSystem, "miss_cap", &m.missCap},
		{statSystem, "parry_cap", &m.parryCap},
		{statSystem, "dodge_cap", &m.dodgeCap},
		{player, "dodge_base", &m.dodgeBase},
		{player, "crit_to_dodge", &m.critToDodge},
	} {
		if *t.out, err = parseClassArray(t.src, t.name); err != nil {
			return nil, err
		}
	}

	if m.gt, err = loadGT(dbcDir, level, len(m.ratings)); err != nil {
		return nil, err
	}

	db, err := azerothcore.OpenDB(dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if m.classStats, err = loadClassStats(db, level); err != nil {
		return nil, err
	}
	if m.raceStats, err = loadRaceStats(db); err != nil {
		return nil, err
	}

	if err := m.checkNamedRatings(); err != nil {
		return nil, err
	}
	if m.spellCritPerInt, err = m.casterConstant("gtChanceToSpellCrit", m.gt.spellCritPerInt); err != nil {
		return nil, err
	}
	if m.manaRegenPerSpirit, err = m.casterConstant("gtRegenMPPerSpt", m.gt.regenMPPerSpirit); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *model) rating(name string) int {
	return slices.IndexFunc(m.ratings, func(r combatRating) bool { return r.Name == name })
}

func (m *model) checkNamedRatings() error {
	for _, n := range namedRatings {
		for _, name := range n.ratings {
			cr := m.rating(name)
			if cr < 0 {
				return fmt.Errorf("no CR_%s in enum CombatRating", name)
			}
			if first := m.rating(n.ratings[0]); m.gt.combatRatings[cr] != m.gt.combatRatings[first] {
				return fmt.Errorf("CR_%s (%v) and CR_%s (%v) differ, the sim converts both with one constant",
					name, m.gt.combatRatings[cr], n.ratings[0], m.gt.combatRatings[first])
			}
			if slices.ContainsFunc(classScaledRatings, func(s struct{ name, tsName string }) bool { return s.name == name }) {
				continue
			}
			for _, c := range classes {
				if s := m.gt.classScalars[c.serverID][cr]; s != 1 {
					return fmt.Errorf("CLASS_%s scales CR_%s by %v; the sim needs to convert it per class", c.name, name, s)
				}
			}
		}
	}
	for _, s := range classScaledRatings {
		if m.rating(s.name) < 0 {
			return fmt.Errorf("no CR_%s in enum CombatRating", s.name)
		}
	}
	return nil
}

// casterConstant returns a per-class table's value, which every class with mana must share: the sim
// applies it with the mana bar, to pets as well.
func (m *model) casterConstant(table string, values map[int]float32) (float32, error) {
	var value float32
	var from string
	for _, c := range classes {
		if m.classStats[c.serverID].BaseMana == 0 {
			continue
		}
		v := values[c.serverID]
		if from == "" {
			value, from = v, c.name
		} else if v != value {
			return 0, fmt.Errorf("%s: CLASS_%s has %v but CLASS_%s %v; the sim needs it per class", table, c.name, v, from, value)
		}
	}
	return value, nil
}
