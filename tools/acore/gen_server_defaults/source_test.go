package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const acDir = "/ac"

func readAC(t *testing.T, rel string) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(acDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// sourceDefault finds the default the server passes for a key: what follows `"key",` in a
// GetOption or SetConfigValue call, up to the next comma or parenthesis.
func sourceDefault(t *testing.T, src, key string) string {
	t.Helper()
	m := regexp.MustCompile(regexp.QuoteMeta(`"`+key+`"`) + `\s*,\s*([^,)]+)`).FindStringSubmatch(src)
	if m == nil {
		t.Errorf("%s isn't read in the server source", key)
		return ""
	}
	return strings.TrimSpace(m[1])
}

func checkNumber(t *testing.T, key, literal string, want float64) {
	t.Helper()
	got, err := strconv.ParseFloat(strings.TrimSuffix(literal, "f"), 64)
	if err != nil || float32(got) != float32(want) {
		t.Errorf("%s defaults to %s in the server, the generator assumes %v", key, literal, want)
	}
}

func TestCodeDefaultsMatchServerSource(t *testing.T) {
	if _, err := os.Stat(acDir); err != nil {
		t.Skip("no AzerothCore checkout at " + acDir)
	}

	for _, toggle := range spellTweakToggles {
		if got := sourceDefault(t, readAC(t, toggle.src), toggle.key); got != "true" {
			t.Errorf("%s defaults to %q in the server, the generator assumes true", toggle.key, got)
		}
	}
	checkNumber(t, exoticPetDamagePctKey, sourceDefault(t, readAC(t, spellTweaksSrc), exoticPetDamagePctKey), exoticPetDamagePctDef)
	checkNumber(t, mapUpdateIntervalKey, sourceDefault(t, readAC(t, worldConfigSrc), mapUpdateIntervalKey), mapUpdateIntervalDef)

	dungeonScale := readAC(t, dungeonScaleSrc)
	checkNumber(t, minHPModifierKey, sourceDefault(t, dungeonScale, minHPModifierKey), minHPModifierDef)
	checkNumber(t, minDamageModifierKey, sourceDefault(t, dungeonScale, minDamageModifierKey), minDamageModifierDef)
	for _, set := range dungeonScaleSets {
		for _, stat := range dungeonScaleStats {
			key := "DungeonScale." + set.key + "." + stat.key
			want := `sConfigMgr->GetOption<float>("` + stat.legacyKey + `"`
			if set.sized {
				// e.g. StatModifierRaid25MHeroic.Boss falls back to the StatModifierRaidHeroic_Boss_ variables
				generic := strings.NewReplacer("10M", "", "25M", "").Replace(set.key)
				want = strings.ReplaceAll(generic, ".", "_") + "_" + stat.key
			}
			if got := sourceDefault(t, dungeonScale, key); got != want {
				t.Errorf("%s defaults to %q in the server, the generator assumes %q", key, got, want)
			}
		}
	}

	reforging := readAC(t, reforgingSrc)
	for _, key := range []string{reforgeEnableKey, reforgeStatsKey, reforgePercentageKey} {
		sourceDefault(t, reforging, key)
	}
	header := readAC(t, "modules/mod-reforging/src/item_reforge.h")
	for _, want := range []string{
		`DefaultReforgeableStats = "` + reforgeStatsDef + `"`,
		"PERCENTAGE_DEFAULT = " + strconv.Itoa(reforgePercentageDef) + ".0f",
		"PERCENTAGE_MIN = 10.0f",
		"PERCENTAGE_MAX = 90.0f",
		"MAX_REFORGEABLE_STATS = " + strconv.Itoa(maxReforgeableStatTypes),
	} {
		if !strings.Contains(header, want) {
			t.Errorf("item_reforge.h no longer has %s", want)
		}
	}
}

func TestLiveConfigBuilds(t *testing.T) {
	if _, err := os.Stat(acDir); err != nil {
		t.Skip("no AzerothCore checkout at " + acDir)
	}
	if _, _, err := liveSettings(acDir); err != nil {
		t.Fatal(err)
	}
}
