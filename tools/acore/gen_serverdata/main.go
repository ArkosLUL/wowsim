// gen_serverdata writes sim/core/serverdata's tables from the committed spell capture and the live world
// DB: SpellInfo, proc entries and spell_bonus_data for the spells the sim names by id plus the spells those
// trigger, and all of spell_enchant_proc_data.
//
//	tools/acore/dock.sh run ./tools/acore/gen_serverdata
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

func main() {
	dumpPath := flag.String("dump", spellset.DefaultDumpPath, "spell capture")
	dsn := flag.String("dsn", azerothcore.ContainerDSN(), "AzerothCore world database DSN")
	outDir := flag.String("out", "sim/core/serverdata", "package directory to write the tables to")
	flag.Parse()

	dump, err := spellset.ReadDump(*dumpPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(dump) == 0 {
		log.Fatalf("%s is empty or missing; capture it first (tools/acore/spellids/README.md)", *dumpPath)
	}
	lits, err := spellset.ScanSim(".")
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range lits.TypeErrors {
		log.Printf("type error (constants depending on it are lost): %s", e)
	}

	set, triggered, missing := dump.GeneratedSet(lits)
	var ids []int32
	for id := range set {
		if dump[id] != nil {
			ids = append(ids, id)
		}
	}
	for _, id := range missing {
		where := "triggered"
		if refs := lits.Spells[id]; len(refs) > 0 {
			where = refs[0].String()
		}
		log.Printf("spell %d (%s) isn't in the capture: unknown to the server, or not captured yet (tools/acore/spellids)", id, where)
	}

	db, err := azerothcore.OpenDB(*dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	m, mismatches, err := buildModel(dump, ids, db)
	if err != nil {
		log.Fatal(err)
	}
	if len(mismatches) > 0 {
		log.Fatalf("the capture and the live DB disagree, so the server hasn't loaded these tables yet or the "+
			"capture is stale; wrote nothing:\n%v", errors.Join(mismatches...))
	}

	names := map[int32]string{}
	for _, s := range m.spells {
		names[s.ID] = s.name
	}
	outputs := []struct {
		file   string
		render func() ([]byte, error)
		rows   int
	}{
		{"spells_auto_gen.go", func() ([]byte, error) { return renderSpells(m.spells) }, len(m.spells)},
		{"procs_auto_gen.go", func() ([]byte, error) { return renderProcs(m.procs, names) }, len(m.procs)},
		{"bonus_auto_gen.go", func() ([]byte, error) { return renderBonuses(m.bonuses, names) }, len(m.bonuses)},
		{"enchant_procs_auto_gen.go", func() ([]byte, error) { return renderEnchantProcs(m.enchantProcs) }, len(m.enchantProcs)},
	}
	rendered := make([][]byte, len(outputs))
	for i, o := range outputs {
		if rendered[i], err = o.render(); err != nil {
			log.Fatalf("%s: %v", o.file, err)
		}
	}
	for i, o := range outputs {
		path := filepath.Join(*outDir, o.file)
		if err := os.WriteFile(path, rendered[i], 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("wrote %s: %d rows\n", path, o.rows)
	}
	fmt.Printf("%d spells: %d named in sim/, %d more triggered; %d ids not in the capture\n",
		len(m.spells), len(m.spells)-countIn(triggered, dump), countIn(triggered, dump), len(missing))
}

func countIn(ids map[int32]bool, dump spellset.Dump) int {
	n := 0
	for id := range ids {
		if dump[id] != nil {
			n++
		}
	}
	return n
}
