package main

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wowsims/wotlk/tools/database"
)

var numberLiteral = regexp.MustCompile(`\d+(?:\.\d+)?`)

var htmlTag = regexp.MustCompile(`<[^>]*>`)

var whitespace = regexp.MustCompile(`\s+`)

// classicText returns a Classic Wowhead spell tooltip as plain text.
func classicText(tooltips map[int32]database.WowheadItemResponse, spellID int32) (string, bool) {
	tooltip, ok := tooltips[spellID]
	if !ok || tooltip.Tooltip == "" {
		return "", false
	}
	text := strings.ReplaceAll(htmlTag.ReplaceAllString(tooltip.Tooltip, " "), "&nbsp;", " ")
	return strings.TrimSpace(whitespace.ReplaceAllString(text, " ")), true
}

// classicNumbers extracts the numbers shown in a Classic Wowhead spell tooltip.
func classicNumbers(tooltips map[int32]database.WowheadItemResponse, spellID int32) ([]float64, bool) {
	text, ok := classicText(tooltips, spellID)
	if !ok {
		return nil, false
	}
	var numbers []float64
	for _, lit := range numberLiteral.FindAllString(text, -1) {
		if v, err := strconv.ParseFloat(lit, 64); err == nil {
			numbers = append(numbers, v)
		}
	}
	return numbers, true
}

func containsNumber(numbers []float64, v float64) bool {
	for _, n := range numbers {
		if math.Abs(n-v) < 1e-6 {
			return true
		}
	}
	return false
}

// moduleRefs records which SQL files outside AzerothCore's base data (modules and custom SQL)
// touch an item, spell or enchant, so diffs caused by the server's own modules can be told apart.
type moduleRefs struct {
	items    map[int32][]string
	spells   map[int32][]string
	enchants map[int32][]string
}

var (
	// \b keeps longer columns ending in the same letters (DisenchantID, displayid, class_id) out.
	sqlIDEquals = regexp.MustCompile("(?i)\\b(?:entry|ID|SpellId)\\b`?\\s*=\\s*(-?\\d+)")
	sqlIDIn     = regexp.MustCompile("(?i)\\b(?:entry|ID|SpellId)\\b`?\\s+IN\\s*\\(([-\\d,\\s]+)\\)")
	sqlTupleID  = regexp.MustCompile(`(?i)(?:VALUES\s*|\)\s*,\s*)\(\s*(-?\d+)`)
	sqlSetList  = regexp.MustCompile(`(?is)\bupdate\b.*?\bset\b(.*?)\bwhere\b`)
	// item_template columns that feed the comparison; other updates (stack sizes, reputation
	// requirements, ...) can't explain a diff.
	sqlItemStatColumn = regexp.MustCompile(`(?i)stat_type|stat_value|armor|dmg_|delay|spellid_|spelltrigger_|spellppmrate_|spellcooldown_|spellcategorycooldown_|socketcolor|socketbonus|gemproperties|itemlevel|quality|\bblock\b|_res\b|allowableclass|itemset|flags|scalingstat|randomproperty|randomsuffix`)
)

func touchesItemStats(stmt string) bool {
	if m := sqlSetList.FindStringSubmatch(stmt); m != nil {
		return sqlItemStatColumn.MatchString(m[1])
	}
	lower := strings.ToLower(stmt)
	return strings.Contains(lower, "insert") || strings.Contains(lower, "replace") || strings.Contains(lower, "delete")
}

func scanModuleSQL(acRepo string) (moduleRefs, error) {
	refs := moduleRefs{items: map[int32][]string{}, spells: map[int32][]string{}, enchants: map[int32][]string{}}
	for _, root := range []string{filepath.Join(acRepo, "modules"), filepath.Join(acRepo, "data", "sql", "custom")} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Module "optional" SQL is only applied by hand.
			if d.IsDir() && d.Name() == "optional" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".sql") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(acRepo, path)
			if err != nil {
				return err
			}
			refs.addSQL(string(data), filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return refs, fmt.Errorf("scanning AzerothCore SQL (-acRepo): %w", err)
		}
	}
	return refs, nil
}

func (refs moduleRefs) addSQL(sql, file string) {
	add := func(m map[int32][]string, id int) {
		if id < 0 {
			id = -id
		}
		if !slices.Contains(m[int32(id)], file) {
			m[int32(id)] = append(m[int32(id)], file)
		}
	}

	for _, stmt := range strings.Split(sql, ";") {
		lower := strings.ToLower(stmt)
		var target map[int32][]string
		switch {
		case strings.Contains(lower, "item_template"):
			if !touchesItemStats(stmt) {
				continue
			}
			target = refs.items
		case strings.Contains(lower, "spell_enchant_proc_data"):
			target = refs.enchants
		case strings.Contains(lower, "spell_dbc") || strings.Contains(lower, "spell_proc") || strings.Contains(lower, "spell_cooldown_overrides"):
			target = refs.spells
		default:
			continue
		}
		for _, m := range sqlIDEquals.FindAllStringSubmatch(stmt, -1) {
			id, _ := strconv.Atoi(m[1])
			add(target, id)
		}
		for _, m := range sqlIDIn.FindAllStringSubmatch(stmt, -1) {
			for _, part := range strings.Split(m[1], ",") {
				if id, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
					add(target, id)
				}
			}
		}
		for _, m := range sqlTupleID.FindAllStringSubmatch(stmt, -1) {
			id, _ := strconv.Atoi(m[1])
			add(target, id)
		}
	}
}
