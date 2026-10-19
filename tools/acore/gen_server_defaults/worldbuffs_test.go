package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

func TestEnvFileValue(t *testing.T) {
	src := "AC_PLAYERBOTS_OTHER=1\r\n" +
		"       AC_PLAYERBOTS_WORLD_BUFF_MATRIX=\"1:0,1,0,80,80:53760; \r\n" +
		"         2:0,1,1,80,80:53760,57358\"\r\n" +
		"AC_PLAYERBOTS_NEXT=2\r\n"
	got, ok, err := envFileValue(src, "AC_PLAYERBOTS_WORLD_BUFF_MATRIX")
	if err != nil || !ok {
		t.Fatalf("got %v, %v", ok, err)
	}
	// the matrix's parser trims every entry, so only the words count
	if want := "1:0,1,0,80,80:53760; 2:0,1,1,80,80:53760,57358"; strings.Join(strings.Fields(got), " ") != want {
		t.Errorf("got %q, want %q", got, want)
	}

	for name, tc := range map[string]struct {
		src, want string
		found     bool
	}{
		"unquoted":        {"AC_X=1:0,1,0,80,80:53760 # comment\n", "1:0,1,0,80,80:53760", true},
		"yaml style":      {"AC_X: '1:0,1,0,80,80:2'\n", "1:0,1,0,80,80:2", true},
		"the last one":    {"AC_X=1\nAC_X=2\n", "2", true},
		"a longer key":    {"AC_X_Y=1\n", "", false},
		"commented out":   {"# AC_X=1\n", "", false},
		"no such setting": {"AC_Y=1\n", "", false},
	} {
		got, found, err := envFileValue(tc.src, "AC_X")
		if err != nil || got != tc.want || found != tc.found {
			t.Errorf("%s: got %q, %v, %v; want %q, %v", name, got, found, err, tc.want, tc.found)
		}
	}
	if _, _, err := envFileValue("AC_X=\"1:0\n", "AC_X"); err == nil {
		t.Error("an unterminated quote went unnoticed")
	}
}

func TestParseWorldBuffMatrix(t *testing.T) {
	got := parseWorldBuffMatrix("# WARRIOR ARMS 1:0,1,0,80,80:53760,57358; # NO SPELLS 2:0,1,1,80,80:;" +
		" bad:0,1,x,80,80:1; short:0,1,0,80:2; nolabel; 3: 1, 9, 2, 70 ,79 :28520, x ,33256\n")
	want := []worldBuff{
		{0, 1, 0, 80, 80, 53760}, {0, 1, 0, 80, 80, 57358},
		{1, 9, 2, 70, 79, 28520}, {1, 9, 2, 70, 79, 33256},
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestWorldBuffTable(t *testing.T) {
	table, err := worldBuffTable(parseWorldBuffMatrix(
		"1:0,11,3,80,80:53760,57358; 2:0,11,1,80,80:53749,53763,57367; 3:0,11,3,70,79:28520; 4:1,8,1,80,80:53755; 5:2,8,1,80,80:53755"))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[botSpec]*proto.Consumes{
		{proto.Class_ClassDruid, 3}: {Flask: proto.Flask_FlaskOfEndlessRage, Food: proto.Food_FoodHeartyRhino},
		{proto.Class_ClassDruid, 1}: {BattleElixir: proto.BattleElixir_GurusElixir,
			GuardianElixir: proto.GuardianElixir_ElixirOfProtection, Food: proto.Food_FoodBlackenedDragonfin},
		{proto.Class_ClassMage, 1}: {Flask: proto.Flask_FlaskOfTheFrostWyrm},
	} {
		if !googleProto.Equal(table[key], want) {
			t.Errorf("%v: got %v, want %v", key, table[key], want)
		}
	}
	if len(table) != 3 {
		t.Errorf("got %d specs, want 3: %v", len(table), table)
	}

	for name, matrix := range map[string]string{
		"a faction difference": "1:1,8,1,80,80:53755; 2:2,8,1,80,80:54212",
		"a buff the sim lacks": "1:0,8,1,80,80:25898",
		"two flasks":           "1:0,8,1,80,80:53755,54212",
	} {
		if _, err := worldBuffTable(parseWorldBuffMatrix(matrix)); err == nil {
			t.Errorf("%s went unnoticed", name)
		}
	}
}

func TestLiveWorldBuffMatrixSources(t *testing.T) {
	const (
		dist  = "Playerbots.WorldBuffMatrix = # MAGE FIRE 1:0,8,1,80,80:53755,57327\n"
		other = "Playerbots.WorldBuffMatrix = 1:0,8,1,80,80:54212\n"
	)
	for _, tc := range []struct {
		name               string
		env, installed     string
		hasInstalled       bool
		wantSource, wantIn string
		wantErr            bool
	}{
		{"the env file wins", "AC_PLAYERBOTS_WORLD_BUFF_MATRIX=\"1:0,8,1,80,80:\n54212\"\n", other, true,
			playerbotEnvFile, "54212", false},
		{"conf.dist without an install", "AC_PLAYERBOTS_OTHER=1\n", "", false, playerbotsConfDist, "57327", false},
		{"an installed conf like its conf.dist", "", dist, true, playerbotsConfDist, "57327", false},
		{"an installed conf that differs", "", other, true, "", "", true},
	} {
		dir := t.TempDir()
		files := map[string]string{playerbotEnvFile: tc.env, playerbotsConfDist: dist}
		if tc.hasInstalled {
			files[installedPlayerbotsConf] = tc.installed
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
		matrix, source, err := liveWorldBuffMatrix(dir)
		if (err != nil) != tc.wantErr || source != tc.wantSource || !strings.Contains(matrix, tc.wantIn) {
			t.Errorf("%s: got %q from %q, %v", tc.name, matrix, source, err)
		}
	}
}
