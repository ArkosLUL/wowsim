// gen_server_defaults generates the live server's value for every proto.ServerSettings field, from
// an AzerothCore checkout's conf.dist files and the env overrides the live containers run with.
//
//	tools/acore/dock.sh run ./tools/acore/gen_server_defaults
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// Conf files, in the order worldserver loads them.
var confFiles = []string{
	"src/server/apps/worldserver/worldserver.conf.dist",
	"modules/mod-spell-tweaks/conf/spell_tweaks.conf.dist",
	"modules/mod-dungeon-scale/conf/DungeonScale.conf.dist",
	"modules/mod-reforging/conf/mod_reforging.conf.dist",
}

// The module confs worldserver really loads. The container copies each conf.dist in once and never
// updates it, so these can fall behind. The installed worldserver.conf holds credentials, so its
// conf.dist stands in for it.
const installedModulesDir = "env/dist/etc/modules"

var installedConfFiles = []string{
	"src/server/apps/worldserver/worldserver.conf.dist",
	installedModulesDir + "/spell_tweaks.conf",
	installedModulesDir + "/DungeonScale.conf",
	installedModulesDir + "/mod_reforging.conf",
}

// Only the override files holding keys read here, in docker-compose.override.yml's env_file order.
// The others hold unrelated settings and credentials, so they're never opened.
var envFiles = []string{
	"configurationOverrides/ServerPerformance.env",
	"configurationOverrides/SpellTweaks.env",
	"configurationOverrides/DungeonScale.env",
}

func main() {
	acDir := flag.String("ac", "/ac", "AzerothCore checkout the live server runs")
	goOut := flag.String("goOut", "sim/core/server_defaults_auto_gen.go", "generated Go file")
	tsOut := flag.String("tsOut", "ui/core/constants/server_defaults_auto_gen.ts", "generated TypeScript file")
	flag.Parse()

	settings, f, err := liveSettings(*acDir)
	if err != nil {
		log.Fatal(err)
	}
	goSrc, err := renderGo(settings, f)
	if err != nil {
		log.Fatal(err)
	}
	tsSrc, err := renderTS(settings)
	if err != nil {
		log.Fatal(err)
	}
	for path, content := range map[string][]byte{*goOut: goSrc, *tsOut: tsSrc} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("wrote", path)
	}
}

// liveSettings builds the settings from the conf.dist files, then checks that the installed module
// confs give the same ones.
func liveSettings(acDir string) (*proto.ServerSettings, floors, error) {
	c, err := loadConfig(acDir, confFiles, false)
	if err != nil {
		return nil, floors{}, err
	}
	settings, f, err := build(c)
	if err != nil {
		return nil, floors{}, err
	}

	if _, err := os.Stat(filepath.Join(acDir, installedModulesDir)); errors.Is(err, fs.ErrNotExist) {
		log.Printf("no %s, so the installed confs weren't checked", installedModulesDir)
		return settings, f, nil
	}
	installed, err := loadConfig(acDir, installedConfFiles, true)
	if err != nil {
		return nil, floors{}, err
	}
	installedSettings, installedFloors, err := build(installed)
	if err != nil {
		return nil, floors{}, fmt.Errorf("with the confs in %s: %w", installedModulesDir, err)
	}
	if !googleProto.Equal(settings, installedSettings) || f != installedFloors {
		return nil, floors{}, fmt.Errorf("the server loads the module confs in %s, and they don't match their conf.dist files:\n"+
			"  conf.dist: %v, floors %+v\n  installed: %v, floors %+v",
			installedModulesDir, settings, f, installedSettings, installedFloors)
	}
	return settings, f, nil
}

// loadConfig reads confs, then envFiles. With missingOK a missing conf is skipped, as worldserver
// skips a missing module conf and runs that module on its code defaults.
func loadConfig(acDir string, confs []string, missingOK bool) (*config, error) {
	c := newConfig()
	for _, rel := range confs {
		src, err := os.ReadFile(filepath.Join(acDir, rel))
		if missingOK && errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		c.addConf(string(src))
	}
	for _, rel := range envFiles {
		src, err := os.ReadFile(filepath.Join(acDir, rel))
		if err != nil {
			return nil, err
		}
		if err := c.addEnvFile(rel, string(src)); err != nil {
			return nil, err
		}
	}
	return c, nil
}
