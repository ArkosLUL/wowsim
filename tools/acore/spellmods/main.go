// spellmods writes sim/core/testdata/simval/spellmod_spells.json: for every talent the sim implements whose
// rank spells carry spell mods, the spells the sim names that each mod changes on the server. The e2e's
// TestSimvalSpellModSweep probes those spells before and after the talent.
//
//	tools/acore/dock.sh run ./tools/acore/spellmods
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"

	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

func main() {
	root := flag.String("root", ".", "the checkout to scan for spell ids")
	dumpPath := flag.String("dump", spellset.DefaultDumpPath, "spell capture")
	talentsPath := flag.String("talents", "sim/core/testdata/simval/implemented_talents.json", "the talent guard's export")
	outPath := flag.String("out", "sim/core/testdata/simval/spellmod_spells.json", "where to write the list")
	flag.Parse()

	dump, err := spellset.ReadDump(*dumpPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(dump) == 0 {
		log.Fatalf("%s is empty or missing", *dumpPath)
	}
	data, err := os.ReadFile(*talentsPath)
	if err != nil {
		log.Fatal(err)
	}
	var talents []implementedTalent
	if err := json.Unmarshal(data, &talents); err != nil {
		log.Fatalf("%s: %v", *talentsPath, err)
	}
	lits, err := spellset.ScanSim(*root)
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range lits.TypeErrors {
		log.Printf("type error (constants depending on it are lost): %s", e)
	}
	list, stats, err := buildList(dump, talents, lits.Spells)
	if err != nil {
		log.Fatal(err)
	}
	out, err := marshalList(list)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outPath, out, 0o644); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("wrote %s: %d talents (%d with a script-built mod), %d with ranks in more than one group\n",
		*outPath, stats.talents, stats.byScript, stats.multiGroup)
	fmt.Printf("  %d cast (not passive), %d need an equipped item, %d change no spell the sim names\n",
		stats.active, stats.equipped, stats.noSpells)
	fmt.Printf("  %d spells, %d group-spell pairs, %d excluded by the server\n", stats.spells, stats.pairs, stats.excluded)
	fmt.Printf("  left out, though the rule matches them: %d the sim doesn't name, %d it names only for another class\n",
		stats.unnamed, stats.otherClass)
	for _, op := range slices.Sorted(maps.Keys(stats.ops)) {
		fmt.Printf("  %s: %d\n", op, stats.ops[op])
	}
}
