package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Where each key's code default lives in the AzerothCore source. source_test.go checks the keys and
// defaults against it.
const (
	worldConfigSrc  = "src/server/game/World/WorldConfig.cpp"
	spellTweaksSrc  = "modules/mod-spell-tweaks/src/SpellTweaks.cpp"
	dungeonScaleSrc = "modules/mod-dungeon-scale/src/DungeonScale.cpp"
	reforgingSrc    = "modules/mod-reforging/src/mod_reforging_worldscript.cpp"
)

// SpellTweaksSettings fields and their keys. Every one defaults to on.
var spellTweakToggles = []struct {
	field, key, src string
}{
	{"enable", "SpellTweaks.Enable", spellTweaksSrc},
	{"hunter_pet_haste", "SpellTweaks.HunterPetHaste.Enable", spellTweaksSrc},
	{"hunter_pet_armor_pen", "SpellTweaks.HunterPetArmorPen.Enable", worldConfigSrc},
	{"dk_ghoul_armor_pen", "SpellTweaks.DKGhoulArmorPen.Enable", worldConfigSrc},
	{"feral_spirit_haste", "SpellTweaks.FeralSpiritHaste.Enable", spellTweaksSrc},
	{"disease_haste", "SpellTweaks.DiseaseHaste.Enable", spellTweaksSrc},
	{"deadly_poison_murder", "SpellTweaks.DeadlyPoisonMurder.Enable", spellTweaksSrc},
	{"rupture_weapon_expertise", "SpellTweaks.RuptureWeaponExpertise.Enable", spellTweaksSrc},
	{"rend_trauma", "SpellTweaks.RendTrauma.Enable", spellTweaksSrc},
	{"balance_dot_scaling", "SpellTweaks.BalanceDotScaling.Enable", spellTweaksSrc},
	{"titans_grip_no_damage_penalty", "SpellTweaks.TitansGrip.NoDamagePenalty.Enable", spellTweaksSrc},
	{"omen_clarity_faerie_fire", "SpellTweaks.OmenClarityFaerieFire.Enable", spellTweaksSrc},
}

const (
	mapUpdateIntervalKey    = "MapUpdateInterval"
	mapUpdateIntervalDef    = 10
	exoticPetDamagePctKey   = "SpellTweaks.ExoticPetDamage.Pct"
	exoticPetDamagePctDef   = 10
	minHPModifierKey        = "DungeonScale.MinHPModifier"
	minHPModifierDef        = 0.1
	minDamageModifierKey    = "DungeonScale.MinDamageModifier"
	minDamageModifierDef    = 0.01
	reforgeEnableKey        = "Reforging.Enable"
	reforgePercentageKey    = "Reforging.Percentage"
	reforgeStatsKey         = "Reforging.ReforgeableStats"
	reforgePercentageDef    = 40 // item_reforge.h PERCENTAGE_DEFAULT, used outside [10, 90] too
	reforgeStatsDef         = "6,13,14,31,32,36,37"
	maxReforgeableStatTypes = 15
)

// DungeonScaleModifiers fields: the stat key, and the pre-rename key its generic value falls back on.
var dungeonScaleStats = []struct {
	field, key, legacyKey string
}{
	{"global", "Global", "DungeonScale.rate.global"},
	{"health", "Health", "DungeonScale.rate.health"},
	{"armor", "Armor", "DungeonScale.rate.armor"},
	{"damage", "Damage", "DungeonScale.rate.damage"},
}

// DungeonScaleSettings fields. The size ones stay unset unless the config sets them; the sim then
// falls back to the generic set, as the loader does.
var dungeonScaleSets = []struct {
	field, key string
	sized      bool
}{
	{"raid", "StatModifierRaid", false},
	{"raid_boss", "StatModifierRaid.Boss", false},
	{"raid_heroic", "StatModifierRaidHeroic", false},
	{"raid_heroic_boss", "StatModifierRaidHeroic.Boss", false},
	{"raid10", "StatModifierRaid10M", true},
	{"raid10_boss", "StatModifierRaid10M.Boss", true},
	{"raid10_heroic", "StatModifierRaid10MHeroic", true},
	{"raid10_heroic_boss", "StatModifierRaid10MHeroic.Boss", true},
	{"raid25", "StatModifierRaid25M", true},
	{"raid25_boss", "StatModifierRaid25M.Boss", true},
	{"raid25_heroic", "StatModifierRaid25MHeroic", true},
	{"raid25_heroic_boss", "StatModifierRaid25MHeroic.Boss", true},
}

// Map ids of every WotLK raid: Onyxia, Naxxramas, Ulduar, OS, EoE, VoA, ICC, ToC, RS.
var wotlkRaidMaps = []int{249, 533, 603, 615, 616, 624, 631, 649, 724}

// Creatures DungeonScale.DisabledID may list: the module's stock list (player vehicles and dungeon
// NPCs, no raid boss) and the mod-sim-validation dummies, which keep their template stats.
var knownDisabledCreatures = []int{
	26499, 27692, 27755, 27756, 33062, 33060, 33109, 30161, 30248, 35644, 36558, 36838, 36839,
	999000, 999001, 999002, 999003,
}

type floors struct {
	minHealth, minDamage float32
}

// build reads every ServerSettings field from the config, as the server reads it.
func build(c *config) (*proto.ServerSettings, floors, error) {
	interval := c.uint32(mapUpdateIntervalKey, mapUpdateIntervalDef)
	if interval < 1 { // WorldConfig's MIN_MAP_UPDATE_DELAY check
		interval = mapUpdateIntervalDef
	}
	if interval > math.MaxInt32 {
		return nil, floors{}, fmt.Errorf("%s = %d doesn't fit the proto's int32", mapUpdateIntervalKey, interval)
	}

	tweaks := &proto.SpellTweaksSettings{
		ExoticPetDamagePct: googleProto.Float64(f64(max(0, c.float(exoticPetDamagePctKey, exoticPetDamagePctDef)))),
	}
	for _, t := range spellTweakToggles {
		setField(tweaks, t.field, protoreflect.ValueOfBool(c.bool(t.key, true)))
	}

	scale := &proto.DungeonScaleSettings{}
	for _, set := range dungeonScaleSets {
		modifiers := &proto.DungeonScaleModifiers{}
		for _, stat := range dungeonScaleStats {
			key := "DungeonScale." + set.key + "." + stat.key
			if set.sized {
				if v, ok := c.optionalFloat(key); ok {
					setField(modifiers, stat.field, protoreflect.ValueOfFloat64(f64(v)))
				}
				continue
			}
			v := c.float(key, c.float(stat.legacyKey, 1))
			setField(modifiers, stat.field, protoreflect.ValueOfFloat64(f64(v)))
		}
		if !isEmpty(modifiers) {
			setField(scale, set.field, protoreflect.ValueOfMessage(modifiers.ProtoReflect()))
		}
	}
	if problems := unmodeledDungeonScale(c); len(problems) > 0 {
		return nil, floors{}, fmt.Errorf("the live dungeon scale config does something the sim doesn't model:\n  %s",
			strings.Join(problems, "\n  "))
	}

	reforge, err := buildReforge(c)
	if err != nil {
		return nil, floors{}, err
	}

	settings := &proto.ServerSettings{
		MapUpdateIntervalMs: googleProto.Int32(int32(interval)),
		SpellTweaks:         tweaks,
		DungeonScale:        scale,
		Reforge:             reforge,
	}
	return settings, floors{
		minHealth: c.float(minHPModifierKey, minHPModifierDef),
		minDamage: c.float(minDamageModifierKey, minDamageModifierDef),
	}, nil
}

func buildReforge(c *config) (*proto.ReforgeSettings, error) {
	pct := c.float(reforgePercentageKey, reforgePercentageDef)
	if pct < 10 || pct > 90 { // ItemReforge::SetPercentage
		pct = reforgePercentageDef
	}

	var statTypes []int32
	var tokens []string
	for _, token := range strings.Split(c.string(reforgeStatsKey, reforgeStatsDef), ",") {
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	// over the limit, SetReforgeableStats leaves the list empty
	if len(tokens) <= maxReforgeableStatTypes {
		for _, token := range tokens {
			v, err := strconv.ParseUint(strings.TrimSpace(token), 10, 31)
			if err != nil {
				return nil, fmt.Errorf("%s: bad stat type %q", reforgeStatsKey, token)
			}
			statTypes = append(statTypes, int32(v))
		}
	}

	return &proto.ReforgeSettings{
		Enable:     googleProto.Bool(c.bool(reforgeEnableKey, true)),
		Percentage: googleProto.Float64(f64(pct)),
		StatTypes:  statTypes,
	}, nil
}

// unmodeledDungeonScale lists what would make the sim's multipliers wrong for a full WotLK raid.
func unmodeledDungeonScale(c *config) []string {
	var problems []string

	legacyEnable := c.bool("DungeonScale.enable", true)
	for _, key := range []string{"Enable.Global", "Enable.Autoscale", "Enable.10M", "Enable.25M", "Enable.10MHeroic", "Enable.25MHeroic"} {
		def := legacyEnable
		if key == "Enable.Autoscale" {
			def = true
		}
		if !c.bool("DungeonScale."+key, def) {
			problems = append(problems, "DungeonScale."+key+" is off")
		}
	}

	// A full raid sits at the top of the player count curve, which is CurveCeiling when the floor is 0.
	for _, curve := range []struct{ sized, generic string }{
		{"Raid10M", "Raid"}, {"Raid25M", "Raid"}, {"Raid10MHeroic", "RaidHeroic"}, {"Raid25MHeroic", "RaidHeroic"},
	} {
		prefix := "DungeonScale.InflectionPoint"
		floor := c.float(prefix+curve.sized+".CurveFloor", c.float(prefix+curve.generic+".CurveFloor", 0))
		ceiling := c.float(prefix+curve.sized+".CurveCeiling", c.float(prefix+curve.generic+".CurveCeiling", 1))
		if floor != 0 || ceiling != 1 {
			problems = append(problems, fmt.Sprintf("%s%s curve is %v to %v, not 0 to 1", prefix, curve.sized, floor, ceiling))
		}
	}
	if offset := c.uint32("DungeonScale.playerCountDifficultyOffset", 0); offset != 0 {
		problems = append(problems, fmt.Sprintf("DungeonScale.playerCountDifficultyOffset is %d", offset))
	}

	for _, o := range []struct{ key, legacyKey string }{
		{"DungeonScale.Disable.PerInstance", ""},
		{"DungeonScale.StatModifier.PerInstance", ""},
		{"DungeonScale.StatModifier.Boss.PerInstance", ""},
		{"DungeonScale.InflectionPoint.PerInstance", "DungeonScale.PerDungeonScaling"},
		{"DungeonScale.InflectionPoint.Boss.PerInstance", "DungeonScale.PerDungeonBossScaling"},
	} {
		value := c.string(o.key, c.string(o.legacyKey, ""))
		for _, entry := range strings.Split(value, ",") {
			fields := strings.Fields(entry)
			if len(fields) == 0 {
				continue
			}
			if mapID := atoi(fields[0]); slices.Contains(wotlkRaidMaps, mapID) {
				problems = append(problems, fmt.Sprintf("%s names raid map %d", o.key, mapID))
			}
		}
	}

	// the sim doesn't know which creature it fights, so any of these could be the boss
	if value := c.string("DungeonScale.StatModifier.PerCreature", ""); strings.TrimSpace(value) != "" {
		problems = append(problems, "DungeonScale.StatModifier.PerCreature is set")
	}
	for _, size := range []string{"40", "25", "10", "5", "2"} { // the loader skips ForcedID20
		key := "DungeonScale.ForcedID" + size
		if value := c.string(key, ""); len(creatureIDs(value)) > 0 {
			problems = append(problems, fmt.Sprintf("%s lists %s", key, value))
		}
	}
	for _, id := range creatureIDs(c.string("DungeonScale.DisabledID", "")) {
		if !slices.Contains(knownDisabledCreatures, id) {
			problems = append(problems, fmt.Sprintf("DungeonScale.DisabledID lists creature %d", id))
		}
	}
	return problems
}

// creatureIDs reads a ForcedID or DisabledID list the way LoadForcedCreatureIdsFromString does: atoi
// on each comma separated entry. Blank or junk entries come out 0 and negative ones get skipped, so
// neither names a creature.
func creatureIDs(list string) []int {
	var ids []int
	for _, entry := range strings.Split(list, ",") {
		if id := atoi(entry); id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// atoi is C's atoi: leading spaces, a sign, then digits up to the first non-digit, or 0.
func atoi(s string) int {
	s = strings.TrimLeft(s, " \t\n\v\f\r")
	sign := 1
	if s != "" && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9' && n <= math.MaxInt32; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return sign * n
}

// f64 turns a float32 the server parsed into the shortest double that converts back to it, so the
// generated files read 1.2 rather than 1.2000000476837158.
func f64(v float32) float64 {
	d, _ := strconv.ParseFloat(strconv.FormatFloat(float64(v), 'g', -1, 32), 64)
	return d
}

func setField(m googleProto.Message, name string, v protoreflect.Value) {
	r := m.ProtoReflect()
	fd := r.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil {
		panic(fmt.Sprintf("%s has no field %s", r.Descriptor().FullName(), name))
	}
	r.Set(fd, v)
}

func isEmpty(m googleProto.Message) bool {
	empty := true
	m.ProtoReflect().Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool {
		empty = false
		return false
	})
	return empty
}
