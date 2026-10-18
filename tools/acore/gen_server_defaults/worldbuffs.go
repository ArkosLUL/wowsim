package main

import (
	"errors"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// mod-playerbots' WorldBuffAction puts the matrix's flask, elixirs and food on every bot that runs the
// worldbuff strategy. The sim's Playerbot switch reads the level 80 rows.
const (
	worldBuffMatrixKey      = "Playerbots.WorldBuffMatrix"
	playerbotEnvFile        = "configurationOverrides/Playerbot.env"
	playerbotsConfDist      = "modules/mod-playerbots/conf/playerbots.conf.dist"
	installedPlayerbotsConf = installedModulesDir + "/playerbots.conf"
	playerbotsConfigSrc     = "modules/mod-playerbots/src/PlayerbotAIConfig.cpp"
	worldBuffLevel          = 80
)

type worldBuff struct {
	faction, class, spec, minLevel, maxLevel, spell int64
}

// liveWorldBuffMatrix resolves the matrix as GetOption does. Playerbot.env is read for this key only: it
// holds a quoted value over many lines, which addEnvFile doesn't parse. Returns the file it came from.
func liveWorldBuffMatrix(acDir string) (matrix, source string, err error) {
	env, err := os.ReadFile(filepath.Join(acDir, playerbotEnvFile))
	if err != nil {
		return "", "", err
	}
	if value, ok, err := envFileValue(string(env), envVarName(worldBuffMatrixKey)); err != nil {
		return "", "", fmt.Errorf("%s: %w", playerbotEnvFile, err)
	} else if ok {
		return value, playerbotEnvFile, nil
	}

	dist, err := confValue(filepath.Join(acDir, playerbotsConfDist), worldBuffMatrixKey)
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(filepath.Join(acDir, installedModulesDir)); errors.Is(err, fs.ErrNotExist) {
		return dist, playerbotsConfDist, nil
	}
	installed, err := confValue(filepath.Join(acDir, installedPlayerbotsConf), worldBuffMatrixKey)
	if errors.Is(err, fs.ErrNotExist) {
		installed, err = "", nil
	}
	if err != nil {
		return "", "", err
	}
	if installed != dist {
		return "", "", fmt.Errorf("the server loads %s, whose %s doesn't match its conf.dist", installedPlayerbotsConf, worldBuffMatrixKey)
	}
	return dist, playerbotsConfDist, nil
}

func confValue(path, key string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	c := newConfig()
	c.addConf(string(src))
	return c.conf[key], nil
}

// envFileValue reads one key of a docker compose env_file, the last one set, where a quoted value may
// run over several lines. Other lines aren't parsed.
func envFileValue(src, key string) (value string, found bool, err error) {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		line = strings.TrimPrefix(strings.TrimSpace(line), "export ")
		rest, ok := strings.CutPrefix(line, key)
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" || (rest[0] != '=' && rest[0] != ':') {
			continue
		}
		v := strings.TrimSpace(rest[1:])
		if v == "" || (v[0] != '"' && v[0] != '\'') {
			if j := strings.Index(v, " #"); j >= 0 {
				v = strings.TrimSpace(v[:j])
			}
			value, found = v, true
			continue
		}
		quoted := strings.Join(append([]string{v[1:]}, lines[i+1:]...), "\n")
		end := strings.IndexByte(quoted, v[0])
		if end < 0 {
			return "", false, fmt.Errorf("%s: unterminated quote", key)
		}
		value, found = quoted[:end], true
	}
	return value, found, nil
}

// parseWorldBuffMatrix ports PlayerbotAIConfig::LoadWorldBuff: entries split on ';', each
// "label:faction,class,spec,minLevel,maxLevel:spell,spell". What it logs and skips is skipped here too.
func parseWorldBuffMatrix(matrix string) []worldBuff {
	var buffs []worldBuff
	for _, entry := range strings.Split(matrix, ";") {
		entry = strings.Trim(entry, " \t\r\n")
		_, rest, ok := strings.Cut(entry, ":")
		if !ok {
			continue
		}
		metaPart, spellPart, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		var meta []int64
		for _, token := range strings.Split(metaPart, ",") {
			n, ok := stoi(token)
			if !ok {
				break
			}
			meta = append(meta, n)
		}
		if len(meta) != 5 {
			continue
		}
		for _, token := range strings.Split(spellPart, ",") {
			if spell, ok := stoi(token); ok {
				buffs = append(buffs, worldBuff{meta[0], meta[1], meta[2], meta[3], meta[4], spell})
			}
		}
	}
	return buffs
}

// stoi reads a number the way std::stoi does: leading whitespace skipped, anything after the digits ignored.
func stoi(token string) (int64, bool) {
	token = strings.TrimLeft(token, " \t\r\n\v\f")
	end := 0
	if end < len(token) && (token[end] == '+' || token[end] == '-') {
		end++
	}
	digits := end
	var n int64
	for ; end < len(token) && token[end] >= '0' && token[end] <= '9'; end++ {
		n = n*10 + int64(token[end]-'0')
	}
	if end == digits {
		return 0, false
	}
	if token[0] == '-' {
		n = -n
	}
	return n, true
}

// The server's class ids.
var worldBuffClasses = map[int64]proto.Class{
	1: proto.Class_ClassWarrior, 2: proto.Class_ClassPaladin, 3: proto.Class_ClassHunter, 4: proto.Class_ClassRogue,
	5: proto.Class_ClassPriest, 6: proto.Class_ClassDeathknight, 7: proto.Class_ClassShaman, 8: proto.Class_ClassMage,
	9: proto.Class_ClassWarlock, 11: proto.Class_ClassDruid,
}

type consumableSlot int

const (
	flaskSlot consumableSlot = iota
	battleElixirSlot
	guardianElixirSlot
	foodSlot
)

type consumable struct {
	slot  consumableSlot
	value int32
}

// Each consumable by its buff, the spell a matrix entry names.
var worldBuffConsumables = map[int64]consumable{
	53755: {flaskSlot, int32(proto.Flask_FlaskOfTheFrostWyrm)},
	53760: {flaskSlot, int32(proto.Flask_FlaskOfEndlessRage)},
	54212: {flaskSlot, int32(proto.Flask_FlaskOfPureMojo)},
	53758: {flaskSlot, int32(proto.Flask_FlaskOfStoneblood)},
	53752: {flaskSlot, int32(proto.Flask_LesserFlaskOfToughness)},
	62380: {flaskSlot, int32(proto.Flask_LesserFlaskOfResistance)},

	60340: {battleElixirSlot, int32(proto.BattleElixir_ElixirOfAccuracy)},
	60345: {battleElixirSlot, int32(proto.BattleElixir_ElixirOfArmorPiercing)},
	60341: {battleElixirSlot, int32(proto.BattleElixir_ElixirOfDeadlyStrikes)},
	60344: {battleElixirSlot, int32(proto.BattleElixir_ElixirOfExpertise)},
	60346: {battleElixirSlot, int32(proto.BattleElixir_ElixirOfLightningSpeed)},
	28497: {battleElixirSlot, int32(proto.BattleElixir_ElixirOfMightyAgility)},
	53748: {battleElixirSlot, int32(proto.BattleElixir_ElixirOfMightyStrength)},
	53749: {battleElixirSlot, int32(proto.BattleElixir_GurusElixir)},
	33721: {battleElixirSlot, int32(proto.BattleElixir_SpellpowerElixir)},
	53746: {battleElixirSlot, int32(proto.BattleElixir_WrathElixir)},

	60343: {guardianElixirSlot, int32(proto.GuardianElixir_ElixirOfMightyDefense)},
	53751: {guardianElixirSlot, int32(proto.GuardianElixir_ElixirOfMightyFortitude)},
	53764: {guardianElixirSlot, int32(proto.GuardianElixir_ElixirOfMightyMageblood)},
	60347: {guardianElixirSlot, int32(proto.GuardianElixir_ElixirOfMightyThoughts)},
	53763: {guardianElixirSlot, int32(proto.GuardianElixir_ElixirOfProtection)},
	53747: {guardianElixirSlot, int32(proto.GuardianElixir_ElixirOfSpirit)},

	57399: {foodSlot, int32(proto.Food_FoodFishFeast)},
	57294: {foodSlot, int32(proto.Food_FoodGreatFeast)},
	57367: {foodSlot, int32(proto.Food_FoodBlackenedDragonfin)},
	57358: {foodSlot, int32(proto.Food_FoodHeartyRhino)},
	57325: {foodSlot, int32(proto.Food_FoodMegaMammothMeal)},
	57329: {foodSlot, int32(proto.Food_FoodSpicedWormBurger)},
	57356: {foodSlot, int32(proto.Food_FoodRhinoliciousWormsteak)},
	57332: {foodSlot, int32(proto.Food_FoodImperialMantaSteak)},
	57360: {foodSlot, int32(proto.Food_FoodSnapperExtreme)},
	57334: {foodSlot, int32(proto.Food_FoodMightyRhinoDogs)},
	57327: {foodSlot, int32(proto.Food_FoodFirecrackerSalmon)},
	57365: {foodSlot, int32(proto.Food_FoodCuttlesteak)},
	57371: {foodSlot, int32(proto.Food_FoodDragonfinFilet)},
}

// put fills the consumable's slot in c, false when it's taken already.
func (b consumable) put(c *proto.Consumes) bool {
	switch {
	case b.slot == flaskSlot && c.Flask == 0:
		c.Flask = proto.Flask(b.value)
	case b.slot == battleElixirSlot && c.BattleElixir == 0:
		c.BattleElixir = proto.BattleElixir(b.value)
	case b.slot == guardianElixirSlot && c.GuardianElixir == 0:
		c.GuardianElixir = proto.GuardianElixir(b.value)
	case b.slot == foodSlot && c.Food == 0:
		c.Food = proto.Food(b.value)
	default:
		return false
	}
	return true
}

type botSpec struct {
	class proto.Class
	spec  int64
}

// worldBuffTable is NeedWorldBuffs for a level 80 bot of every class and spec (3 is a cat druid or a
// DPS blood DK), as the sim's consumables. It fails on what the sim can't model.
func worldBuffTable(matrix []worldBuff) (map[botSpec]*proto.Consumes, error) {
	table := map[botSpec]*proto.Consumes{}
	for classID, class := range worldBuffClasses {
		for spec := int64(0); spec <= 3; spec++ {
			alliance, horde := matrixSpells(matrix, 1, classID, spec), matrixSpells(matrix, 2, classID, spec)
			if !slices.Equal(alliance, horde) {
				return nil, fmt.Errorf("class %d spec %d: the Alliance gets %v and the Horde %v, and the sim's table has no faction",
					classID, spec, alliance, horde)
			}
			if len(alliance) == 0 {
				continue
			}
			c := &proto.Consumes{}
			for _, spell := range alliance {
				buff, ok := worldBuffConsumables[spell]
				if !ok {
					return nil, fmt.Errorf("class %d spec %d: spell %d is no flask, elixir or food the sim has", classID, spec, spell)
				}
				if !buff.put(c) {
					return nil, fmt.Errorf("class %d spec %d: spell %d and another buff both fill one consumable slot", classID, spec, spell)
				}
			}
			table[botSpec{class, spec}] = c
		}
	}
	return table, nil
}

func matrixSpells(matrix []worldBuff, faction, classID, spec int64) []int64 {
	var spells []int64
	for _, b := range matrix {
		if (b.faction != 0 && b.faction != faction) || (b.class != 0 && b.class != classID) ||
			(b.minLevel != 0 && b.minLevel > worldBuffLevel) || (b.maxLevel != 0 && b.maxLevel < worldBuffLevel) ||
			b.spec != spec || slices.Contains(spells, b.spell) {
			continue
		}
		spells = append(spells, b.spell)
	}
	return spells
}

func renderWorldBuffs(table map[botSpec]*proto.Consumes, source string) ([]byte, error) {
	keys := make([]botSpec, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b botSpec) int {
		if a.class != b.class {
			return int(a.class - b.class)
		}
		return int(a.spec - b.spec)
	})

	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	p("%s\n", header)
	p("// mod-playerbots' %s, from %s.\n\n", worldBuffMatrixKey, source)
	p("package core\n\nimport \"github.com/wowsims/wotlk/sim/core/proto\"\n\n")
	p("// playerbotWorldBuffs is what the bots' worldbuff strategy puts on a level 80 bot, by class and spec.\n")
	p("var playerbotWorldBuffs = map[playerbotSpec]*proto.Consumes{\n")
	for _, key := range keys {
		c := table[key]
		var fields []string
		if c.Flask != 0 {
			fields = append(fields, "Flask: proto.Flask_"+c.Flask.String())
		}
		if c.BattleElixir != 0 {
			fields = append(fields, "BattleElixir: proto.BattleElixir_"+c.BattleElixir.String())
		}
		if c.GuardianElixir != 0 {
			fields = append(fields, "GuardianElixir: proto.GuardianElixir_"+c.GuardianElixir.String())
		}
		if c.Food != 0 {
			fields = append(fields, "Food: proto.Food_"+c.Food.String())
		}
		p("{proto.Class_%s, %d}: {%s},\n", key.class, key.spec, strings.Join(fields, ", "))
	}
	p("}\n")
	return format.Source([]byte(b.String()))
}

// liveWorldBuffs renders the generated table from the live config.
func liveWorldBuffs(acDir string) ([]byte, error) {
	matrix, source, err := liveWorldBuffMatrix(acDir)
	if err != nil {
		return nil, err
	}
	table, err := worldBuffTable(parseWorldBuffMatrix(matrix))
	if err != nil {
		return nil, fmt.Errorf("%s in %s: %w", worldBuffMatrixKey, source, err)
	}
	return renderWorldBuffs(table, source)
}
