package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAC lays out an AzerothCore checkout with liveLikeConf in every module conf.dist, empty env
// files and, unless it's nil, the given installed module confs.
func fakeAC(t *testing.T, installed map[string]string) string {
	t.Helper()
	files := map[string]string{}
	for _, rel := range confFiles {
		files[rel] = liveLikeConf
	}
	// both builds read it, so it mustn't hold the module keys
	files[confFiles[0]] = "MapUpdateInterval = 10\n"
	for _, rel := range envFiles {
		files[rel] = ""
	}
	for name, src := range installed {
		files[installedModulesDir+"/"+name] = src
	}
	files[reforgeHeaderSrc] = liveLikeReforgeHeader

	dir := t.TempDir()
	if installed != nil {
		if err := os.MkdirAll(filepath.Join(dir, installedModulesDir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for rel, src := range files {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The constants parseReforgeLimits reads, as item_reforge.h declares them.
const liveLikeReforgeHeader = `
    static constexpr float PERCENTAGE_MIN = 10.0f;
    static constexpr float PERCENTAGE_MAX = 90.0f;
    static constexpr uint32 MAX_REFORGEABLE_STATS = 15;
    static constexpr const char* DefaultReforgeableStats = "6,13,14,31,32,36,37";
    static constexpr float PERCENTAGE_DEFAULT = 40.0f;
`

func TestParseReforgeLimits(t *testing.T) {
	limits, err := parseReforgeLimits(liveLikeReforgeHeader)
	if err != nil {
		t.Fatal(err)
	}
	want := reforgeLimits{MinPercentage: 10, MaxPercentage: 90, DefaultPercentage: 40, MaxStatTypes: 15,
		DefaultStatTypes: "6,13,14,31,32,36,37"}
	if limits != want {
		t.Errorf("parseReforgeLimits = %+v, want %+v", limits, want)
	}

	renamed := strings.Replace(liveLikeReforgeHeader, "MAX_REFORGEABLE_STATS", "MAX_REFORGE_STATS", 1)
	if _, err := parseReforgeLimits(renamed); err == nil {
		t.Error("a renamed constant went unnoticed")
	}
}

func TestInstalledConfsMustMatch(t *testing.T) {
	stale := strings.Replace(liveLikeConf, "SpellTweaks.ExoticPetDamage.Pct = 10\n", "", 1)
	if stale == liveLikeConf {
		t.Fatal("liveLikeConf no longer sets SpellTweaks.ExoticPetDamage.Pct")
	}

	for _, tc := range []struct {
		name      string
		installed map[string]string
		wantErr   bool
	}{
		{"nothing installed", nil, false},
		{"same as conf.dist", map[string]string{"spell_tweaks.conf": liveLikeConf}, false},
		{"a missing key whose code default matches", map[string]string{"spell_tweaks.conf": stale}, false},
		{"a changed key", map[string]string{"spell_tweaks.conf": liveLikeConf + "SpellTweaks.RendTrauma.Enable = 0\n"}, true},
		{"no module confs, so code defaults (MinHPModifier 0.1)", map[string]string{}, true},
	} {
		_, _, _, err := liveSettings(fakeAC(t, tc.installed))
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: got error %v, want one: %v", tc.name, err, tc.wantErr)
		}
	}
}
