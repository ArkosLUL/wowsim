package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type classInfo struct {
	serverID int
	name     string // CLASS_ suffix in the server's sources
	proto    string // proto.Class name
}

var classes = []classInfo{
	{1, "WARRIOR", "ClassWarrior"},
	{2, "PALADIN", "ClassPaladin"},
	{3, "HUNTER", "ClassHunter"},
	{4, "ROGUE", "ClassRogue"},
	{5, "PRIEST", "ClassPriest"},
	{6, "DEATH_KNIGHT", "ClassDeathknight"},
	{7, "SHAMAN", "ClassShaman"},
	{8, "MAGE", "ClassMage"},
	{9, "WARLOCK", "ClassWarlock"},
	{11, "DRUID", "ClassDruid"},
}

var races = []struct {
	serverID int
	proto    string
}{
	{1, "RaceHuman"},
	{2, "RaceOrc"},
	{3, "RaceDwarf"},
	{4, "RaceNightElf"},
	{5, "RaceUndead"},
	{6, "RaceTauren"},
	{7, "RaceGnome"},
	{8, "RaceTroll"},
	{10, "RaceBloodElf"},
	{11, "RaceDraenei"},
}

func classByName(name string) (classInfo, bool) {
	for _, c := range classes {
		if c.name == name {
			return c, true
		}
	}
	return classInfo{}, false
}

type combatRating struct {
	Name  string // CR_ suffix, e.g. HASTE_MELEE
	Value int
}

// parseCombatRatings reads `enum CombatRating` from Unit.h.
func parseCombatRatings(unitH string) ([]combatRating, error) {
	block, err := braceBlockAfter(unitH, regexp.MustCompile(`enum CombatRating\b[^{]*`))
	if err != nil {
		return nil, fmt.Errorf("enum CombatRating: %w", err)
	}
	var ratings []combatRating
	for _, m := range regexp.MustCompile(`\bCR_([A-Z_]+)\s*=\s*(\d+)`).FindAllStringSubmatch(block, -1) {
		v, _ := strconv.Atoi(m[2])
		if v != len(ratings) {
			return nil, fmt.Errorf("enum CombatRating: CR_%s is %d, expected %d", m[1], v, len(ratings))
		}
		ratings = append(ratings, combatRating{Name: m[1], Value: v})
	}
	if len(ratings) == 0 {
		return nil, fmt.Errorf("enum CombatRating: no values")
	}
	return ratings, nil
}

// parseClassArray reads a per-class `float name[MAX_CLASSES] = { ... };` table, indexed by class id − 1.
// Entries are float literals or a quotient of two, evaluated in float like the compiler does.
func parseClassArray(src, name string) ([]float32, error) {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*\[MAX_CLASSES\]\s*=\s*`)
	block, err := braceBlockAfter(src, re)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	var values []float32
	for _, entry := range strings.Split(stripComments(block), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "/")
		if len(parts) > 2 {
			return nil, fmt.Errorf("%s: can't evaluate %q", name, entry)
		}
		v, err := parseFloatLiteral(parts[0])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if len(parts) == 2 {
			d, err := parseFloatLiteral(parts[1])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			v /= d
		}
		values = append(values, v)
	}
	if len(values) != maxClassID {
		return nil, fmt.Errorf("%s: %d entries, expected one per class id 1-%d", name, len(values), maxClassID)
	}
	return values, nil
}

func parseFloatLiteral(s string) (float32, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "f")
	v, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, fmt.Errorf("bad float literal %q", s)
	}
	return float32(v), nil
}

// linearFormula is a·level + b·Str + c·Agi + d, the shape of every attack power base value.
type linearFormula struct {
	PerLevel, PerStrength, PerAgility, Base float32
}

// parseAttackPower reads the per-class base value (val2) of Player::UpdateAttackPowerAndDamage, for
// melee and ranged. Druids use the formula outside of forms; the form-specific ones stay in the sim.
func parseAttackPower(statSystem string) (melee, ranged map[int]linearFormula, err error) {
	body, err := braceBlockAfter(statSystem, regexp.MustCompile(`void Player::UpdateAttackPowerAndDamage\(bool ranged\)\s*`))
	if err != nil {
		return nil, nil, fmt.Errorf("UpdateAttackPowerAndDamage: %w", err)
	}
	body = stripComments(body)

	// The first `if (ranged) {...} else {...}` holds the class formulas.
	loc := regexp.MustCompile(`if\s*\(\s*ranged\s*\)\s*`).FindStringIndex(body)
	if loc == nil {
		return nil, nil, fmt.Errorf("UpdateAttackPowerAndDamage: no `if (ranged)`")
	}
	rangedBlock, end, err := braceBlockAt(body, loc[1])
	if err != nil {
		return nil, nil, fmt.Errorf("ranged block: %w", err)
	}
	rest := body[end:]
	elseLoc := regexp.MustCompile(`^\s*else\s*`).FindStringIndex(rest)
	if elseLoc == nil {
		return nil, nil, fmt.Errorf("UpdateAttackPowerAndDamage: no `else` after `if (ranged)`")
	}
	meleeBlock, _, err := braceBlockAt(rest, elseLoc[1])
	if err != nil {
		return nil, nil, fmt.Errorf("melee block: %w", err)
	}

	if ranged, err = parseClassChain(rangedBlock); err != nil {
		return nil, nil, fmt.Errorf("ranged: %w", err)
	}
	if melee, err = parseClassChain(meleeBlock); err != nil {
		return nil, nil, fmt.Errorf("melee: %w", err)
	}
	return melee, ranged, nil
}

var (
	chainIfRe    = regexp.MustCompile(`^\s*(else\s+)?if\s*\(`)
	chainElseRe  = regexp.MustCompile(`^\s*else\s*\{`)
	isClassRe    = regexp.MustCompile(`IsClass\(CLASS_([A-Z_]+)`)
	val2Re       = regexp.MustCompile(`\bval2\s*=\s*([^;]+);`)
	defaultRe    = regexp.MustCompile(`\bdefault\s*:`)
	formSwitchRe = regexp.MustCompile(`switch\s*\(\s*GetShapeshiftForm\(\)\s*\)\s*`)
)

// parseClassChain walks an `if (IsClass(...)) {...} else if ... else {...}` chain and returns each class's
// val2 formula. The trailing else covers every class not named before it.
func parseClassChain(block string) (map[int]linearFormula, error) {
	formulas := map[int]linearFormula{}
	// Skip statements before the chain (the ranged block sets its field indices first).
	start := regexp.MustCompile(`\bif\s*\(\s*IsClass`).FindStringIndex(block)
	if start == nil {
		return nil, fmt.Errorf("no IsClass chain")
	}
	rest := block[start[0]:]
	for {
		if loc := chainIfRe.FindStringIndex(rest); loc != nil {
			cond, condEnd, err := parenBlockAt(rest, loc[1]-1)
			if err != nil {
				return nil, err
			}
			branch, end, err := braceBlockAt(rest, condEnd)
			if err != nil {
				return nil, err
			}
			f, err := branchFormula(branch)
			if err != nil {
				return nil, fmt.Errorf("branch %q: %w", cond, err)
			}
			named := isClassRe.FindAllStringSubmatch(cond, -1)
			if len(named) == 0 {
				return nil, fmt.Errorf("condition %q names no class", cond)
			}
			for _, m := range named {
				c, ok := classByName(m[1])
				if !ok {
					return nil, fmt.Errorf("unknown class CLASS_%s", m[1])
				}
				if _, seen := formulas[c.serverID]; !seen {
					formulas[c.serverID] = f
				}
			}
			rest = rest[end:]
			continue
		}
		if loc := chainElseRe.FindStringIndex(rest); loc != nil {
			branch, _, err := braceBlockAt(rest, loc[1]-1)
			if err != nil {
				return nil, err
			}
			f, err := branchFormula(branch)
			if err != nil {
				return nil, fmt.Errorf("else branch: %w", err)
			}
			for _, c := range classes {
				if _, seen := formulas[c.serverID]; !seen {
					formulas[c.serverID] = f
				}
			}
		}
		break
	}
	for _, c := range classes {
		if _, ok := formulas[c.serverID]; !ok {
			return nil, fmt.Errorf("no formula for CLASS_%s", c.name)
		}
	}
	return formulas, nil
}

// branchFormula is the branch's only val2 assignment, or the one under `default:` when the branch
// switches on shapeshift form.
func branchFormula(branch string) (linearFormula, error) {
	if loc := formSwitchRe.FindStringIndex(branch); loc != nil {
		cases, _, err := braceBlockAt(branch, loc[1])
		if err != nil {
			return linearFormula{}, fmt.Errorf("form switch: %w", err)
		}
		d := defaultRe.FindStringIndex(cases)
		if d == nil {
			return linearFormula{}, fmt.Errorf("form switch has no default:")
		}
		m := val2Re.FindStringSubmatch(cases[d[1]:])
		if m == nil {
			return linearFormula{}, fmt.Errorf("no val2 under default:")
		}
		return parseLinear(m[1])
	}
	ms := val2Re.FindAllStringSubmatch(branch, -1)
	if len(ms) != 1 {
		return linearFormula{}, fmt.Errorf("%d val2 assignments, expected 1", len(ms))
	}
	return parseLinear(ms[0][1])
}

var factorRe = regexp.MustCompile(`^(level|GetStat\(STAT_(STRENGTH|AGILITY)\)|[0-9]+(\.[0-9]+)?f?)$`)

// parseLinear evaluates sums of products of level, GetStat(STAT_STRENGTH/AGILITY) and constants.
func parseLinear(expr string) (linearFormula, error) {
	var f linearFormula
	expr = strings.Join(strings.Fields(expr), "")
	if expr != "" && expr[0] != '-' {
		expr = "+" + expr
	}
	for _, term := range regexp.MustCompile(`[+-][^+-]+`).FindAllString(expr, -1) {
		coeff := float32(1)
		if term[0] == '-' {
			coeff = -1
		}
		var variable string
		for _, factor := range strings.Split(term[1:], "*") {
			m := factorRe.FindStringSubmatch(factor)
			if m == nil {
				return f, fmt.Errorf("can't read factor %q in %q", factor, expr)
			}
			switch {
			case m[1] == "level" || m[2] != "":
				if variable != "" {
					return f, fmt.Errorf("nonlinear term %q", term)
				}
				variable = m[1]
			default:
				v, err := parseFloatLiteral(factor)
				if err != nil {
					return f, err
				}
				coeff *= v
			}
		}
		switch variable {
		case "level":
			f.PerLevel += coeff
		case "GetStat(STAT_STRENGTH)":
			f.PerStrength += coeff
		case "GetStat(STAT_AGILITY)":
			f.PerAgility += coeff
		default:
			f.Base += coeff
		}
	}
	return f, nil
}

// braceBlockAfter returns the contents of the first {...} block that follows a match of re.
func braceBlockAfter(src string, re *regexp.Regexp) (string, error) {
	loc := re.FindStringIndex(src)
	if loc == nil {
		return "", fmt.Errorf("not found")
	}
	block, _, err := braceBlockAt(src, loc[1])
	return block, err
}

// braceBlockAt expects `{` at src[i] after whitespace and returns what's inside it and the index past `}`.
func braceBlockAt(src string, i int) (string, int, error) {
	return delimitedAt(src, i, '{', '}')
}

func parenBlockAt(src string, i int) (string, int, error) {
	return delimitedAt(src, i, '(', ')')
}

func delimitedAt(src string, i int, open, close byte) (string, int, error) {
	for i < len(src) && strings.ContainsRune(" \t\r\n", rune(src[i])) {
		i++
	}
	if i >= len(src) || src[i] != open {
		return "", 0, fmt.Errorf("expected %q at offset %d", open, i)
	}
	depth := 0
	for j := i; j < len(src); j++ {
		switch src[j] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return src[i+1 : j], j + 1, nil
			}
		}
	}
	return "", 0, fmt.Errorf("unbalanced %q at offset %d", open, i)
}

func stripComments(src string) string {
	src = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(src, "")
	return regexp.MustCompile(`//[^\n]*`).ReplaceAllString(src, "")
}
