package main

import (
	"slices"
	"strings"
	"testing"
)

func TestEnvVarName(t *testing.T) {
	for key, want := range map[string]string{
		// the examples in Config.cpp
		"SomeConfig":          "AC_SOME_CONFIG",
		"myNestedConfig.opt1": "AC_MY_NESTED_CONFIG_OPT_1",
		"LogDB.Opt.ClearTime": "AC_LOG_DB_OPT_CLEAR_TIME",
		// names the live override files use
		"MapUpdateInterval":                             "AC_MAP_UPDATE_INTERVAL",
		"SpellTweaks.ExoticPetDamage.Pct":               "AC_SPELL_TWEAKS_EXOTIC_PET_DAMAGE_PCT",
		"DungeonScale.StatModifierRaid.Boss.Health":     "AC_DUNGEON_SCALE_STAT_MODIFIER_RAID_BOSS_HEALTH",
		"DungeonScale.StatModifierRaidHeroic.Boss.Mana": "AC_DUNGEON_SCALE_STAT_MODIFIER_RAID_HEROIC_BOSS_MANA",
		"DungeonScale.DisabledID":                       "AC_DUNGEON_SCALE_DISABLED_ID",
		// quirks: digits split off, but an upper case letter after M doesn't
		"DungeonScale.StatModifierRaid25M.Boss.Health":  "AC_DUNGEON_SCALE_STAT_MODIFIER_RAID_25_M_BOSS_HEALTH",
		"DungeonScale.StatModifierRaid10MHeroic.Health": "AC_DUNGEON_SCALE_STAT_MODIFIER_RAID_10_MHEROIC_HEALTH",
		"SpellTweaks.DKGhoulArmorPen.Enable":            "AC_SPELL_TWEAKS_DKGHOUL_ARMOR_PEN_ENABLE",
	} {
		if got := envVarName(key); got != want {
			t.Errorf("envVarName(%q) = %s, want %s", key, got, want)
		}
	}
}

func TestAddConf(t *testing.T) {
	c := newConfig()
	c.addConf("[worldserver]\r\n# Key = 1\r\n  Key = 2  \r\nKey = 3\r\nQuoted = \"4,5\"\r\nBlank =\r\nnot a setting\r\n")
	for key, want := range map[string]string{"Key": "2", "Quoted": "4,5", "Blank": ""} {
		if got, ok := c.conf[key]; !ok || got != want {
			t.Errorf("%s = %q, %v; want %q", key, got, ok, want)
		}
	}
	if len(c.conf) != 3 {
		t.Errorf("got keys %v, want Key, Quoted and Blank", c.conf)
	}
}

func TestAddEnvFile(t *testing.T) {
	c := newConfig()
	err := c.addEnvFile("x.env", "#Performance configs\n      AC_MAP_UPDATE_INTERVAL: \"100\"\nexport AC_A=1 # comment\nAC_B='x # y'\n\nAC_C:2\n")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"AC_MAP_UPDATE_INTERVAL": "100", "AC_A": "1", "AC_B": "x # y", "AC_C": "2"} {
		if got := c.env[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if err := c.addEnvFile("bad.env", "AC_D \"1\"\n"); err == nil {
		t.Error("a line with no separator should fail")
	}
}

func TestLookupOrder(t *testing.T) {
	c := newConfig()
	c.addConf("A = 1.5\nB = 2\nC = junk\nD =\nE = Yes\nF = 7\nG = -3")
	if err := c.addEnvFile("x.env", "AC_A: \"2.5\""); err != nil {
		t.Fatal(err)
	}

	if got := c.float("A", 0); got != 2.5 {
		t.Errorf("env should beat the conf file: got %v", got)
	}
	if got := c.float("B", 0); got != 2 {
		t.Errorf("B = %v, want 2", got)
	}
	if got := c.float("C", 9); got != 9 {
		t.Errorf("a bad value should take the default: got %v", got)
	}
	if _, ok := c.optionalFloat("D"); ok {
		t.Error("a blank value should count as unset")
	}
	if got := c.float("Missing", 4); got != 4 {
		t.Errorf("a missing key should take the default: got %v", got)
	}
	if !c.bool("E", false) || c.bool("C", false) {
		t.Error("bool should read yes as true and junk as the default")
	}
	if c.uint32("F", 0) != 7 || c.uint32("G", 5) != 5 {
		t.Error("uint32 should read 7 and reject a sign")
	}
}

// liveLikeConf is the part of the conf.dist files build reads, at their stock values.
const liveLikeConf = `
MapUpdateInterval = 10
SpellTweaks.Enable = 1
SpellTweaks.ExoticPetDamage.Pct = 10
DungeonScale.StatModifierRaid.Global=1.0
DungeonScale.StatModifierRaid.Boss.Health=1.0
DungeonScale.StatModifierRaid10M.Global=
DungeonScale.StatModifierRaid25M.Boss.Health=
DungeonScale.MinHPModifier=0.01
Reforging.Enable = 1
Reforging.ReforgeableStats = 6,13,14,31,32,36,37
Reforging.Percentage = 40
`

func buildFrom(t *testing.T, conf, env string) *config {
	t.Helper()
	c := newConfig()
	c.addConf(conf)
	if err := c.addEnvFile("x.env", env); err != nil {
		t.Fatal(err)
	}
	return c
}

// liveReforgeLimits is what item_reforge.h declares today, parsed as the generator parses it.
func liveReforgeLimits(t *testing.T) reforgeLimits {
	t.Helper()
	limits, err := parseReforgeLimits(liveLikeReforgeHeader)
	if err != nil {
		t.Fatal(err)
	}
	return limits
}

func TestBuild(t *testing.T) {
	c := buildFrom(t, liveLikeConf+"DungeonScale.rate.armor = 0.8\n", `
      AC_MAP_UPDATE_INTERVAL: "100"
      AC_SPELL_TWEAKS_EXOTIC_PET_DAMAGE_PCT: "-4"
      AC_SPELL_TWEAKS_RUPTURE_WEAPON_EXPERTISE_ENABLE: "0"
      AC_DUNGEON_SCALE_STAT_MODIFIER_RAID_BOSS_HEALTH: "1.2"
      AC_DUNGEON_SCALE_STAT_MODIFIER_RAID_25_M_BOSS_DAMAGE: "1.1"
      AC_REFORGING_PERCENTAGE: "95"
`)
	settings, f, err := build(c, liveReforgeLimits(t))
	if err != nil {
		t.Fatal(err)
	}

	if got := settings.GetMapUpdateIntervalMs(); got != 100 {
		t.Errorf("map update interval = %d, want the override's 100", got)
	}
	tweaks := settings.SpellTweaks
	if tweaks.GetExoticPetDamagePct() != 0 || tweaks.GetRuptureWeaponExpertise() || !tweaks.GetRendTrauma() {
		t.Errorf("spell tweaks = %v, want pct clamped to 0, rupture off, the rest on", tweaks)
	}

	scale := settings.DungeonScale
	if got := scale.GetRaidBoss().GetHealth(); got != 1.2 {
		t.Errorf("raid boss health = %v, want 1.2 printed as such", got)
	}
	if got := scale.GetRaid().GetArmor(); got != 0.8 {
		t.Errorf("raid armor = %v, want DungeonScale.rate.armor's 0.8 since the key is missing", got)
	}
	if scale.Raid10 != nil {
		t.Errorf("blank 10M keys should stay unset, got %v", scale.Raid10)
	}
	if boss := scale.GetRaid25Boss(); boss.Damage == nil || boss.GetDamage() != 1.1 || boss.Health != nil {
		t.Errorf("25M boss = %v, want only the damage the override sets", boss)
	}
	if f.minHealth != 0.01 || f.minDamage != 0.01 {
		t.Errorf("floors = %+v, want the conf's 0.01 and the code's 0.01", f)
	}

	reforge := settings.Reforge
	if reforge.GetPercentage() != 40 || !reforge.GetEnable() || !slices.Equal(reforge.StatTypes, []int32{6, 13, 14, 31, 32, 36, 37}) {
		t.Errorf("reforge = %v, want 40%% (95 is out of range) and the stock list", reforge)
	}
}

func TestBuildReforgeStatList(t *testing.T) {
	sixteen := strings.Repeat("6,", 15) + "6"
	c := buildFrom(t, liveLikeConf, "AC_REFORGING_REFORGEABLE_STATS: \""+sixteen+"\"")
	settings, _, err := build(c, liveReforgeLimits(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Reforge.StatTypes) != 0 {
		t.Errorf("16 stats should leave the list empty, got %v", settings.Reforge.StatTypes)
	}

	c = buildFrom(t, liveLikeConf, "AC_REFORGING_REFORGEABLE_STATS: \"6,x\"")
	if _, _, err := build(c, liveReforgeLimits(t)); err == nil {
		t.Error("a bad stat type should fail")
	}
}

func TestUnmodeledDungeonScale(t *testing.T) {
	for _, tc := range []struct {
		env      string
		problems int
	}{
		{"", 0},
		{`AC_DUNGEON_SCALE_ENABLE_25_MHEROIC: "0"`, 1},
		{`AC_DUNGEON_SCALE_ENABLE: "0"`, 5}, // the legacy switch is every size's default, not Autoscale's
		{`AC_DUNGEON_SCALE_INFLECTION_POINT_RAID_CURVE_CEILING: "1.5"`, 2},
		{`AC_DUNGEON_SCALE_INFLECTION_POINT_RAID_25_MHEROIC_CURVE_FLOOR: "0.2"`, 1},
		{`AC_DUNGEON_SCALE_STAT_MODIFIER_PER_INSTANCE: "33 1 2, 631 1 2"`, 1}, // ICC, not Shadowfang Keep
		{`AC_DUNGEON_SCALE_STAT_MODIFIER_PER_INSTANCE: "631x 1 2"`, 1},        // atoi reads 631
		{`AC_DUNGEON_SCALE_STAT_MODIFIER_PER_CREATURE: "36597 1 2"`, 1},
		{`AC_DUNGEON_SCALE_PLAYER_COUNT_DIFFICULTY_OFFSET: "2"`, 1},
		{`AC_DUNGEON_SCALE_FORCED_ID_25: "36597"`, 1},
		{`AC_DUNGEON_SCALE_FORCED_ID_10: ", -5, x"`, 0}, // names no creature
		{`AC_DUNGEON_SCALE_DISABLED_ID: "26499, 36839,999000"`, 0},
		{`AC_DUNGEON_SCALE_DISABLED_ID: "26499,36597,x,-3"`, 1}, // the Lich King
	} {
		c := buildFrom(t, liveLikeConf, tc.env)
		if got := unmodeledDungeonScale(c); len(got) != tc.problems {
			t.Errorf("%q: got problems %v, want %d", tc.env, got, tc.problems)
		}
	}
}
