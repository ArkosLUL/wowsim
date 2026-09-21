// acbis turns BiS optimizer results into the dataset mod-bis-tooltip serves to the BisTooltipAC addon:
// an SQL import for acore_world. It never writes to the server; the import is run by hand.
//
// go run ./tools/database/acbis -batch batch.json -roster raid.json -out bis_dataset.sql
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"text/tabwriter"
	"time"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

var (
	batch   = flag.String("batch", "", "raid batch export (OptimizerBatchExport protojson); each raider and phase keeps its latest stage")
	results = flag.String("results", "", "result index JSON listing OptimizerResult protojson files and whose BiS each is")
	roster  = flag.String("roster", "", "acraid roster JSON, needed when a result names a raider")
	dsn     = flag.String("dsn", azerothcore.DefaultCharactersDSN, "AzerothCore database DSN, to look up the roster's character guids")
	simDB   = flag.String("simDb", "assets/database/db.json", "sim item database, which gives each enchant's spell")
	out     = flag.String("out", "", "SQL import to write (required)")
	luaOut  = flag.String("luaOut", "", "also write the dataset as Lua, decoded the way the addon decodes it")
)

func main() {
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}
	if *batch == "" && *results == "" {
		log.Fatal("give -batch, -results or both")
	}

	var all []azerothcore.BisResult
	if *batch != "" {
		batchResults, err := azerothcore.LoadBisBatch(*batch)
		check(err)
		all = append(all, batchResults...)
	}
	if *results != "" {
		indexResults, err := azerothcore.LoadBisResults(*results)
		check(err)
		all = append(all, indexResults...)
	}

	var raid *azerothcore.Roster
	var guids map[string]uint32
	if *roster != "" {
		var err error
		raid, err = azerothcore.LoadRoster(*roster)
		check(err)
		guids, err = lookUpGUIDs(raid)
		check(err)
	}

	enchants, err := azerothcore.SimEnchantSpells(*simDB)
	check(err)

	dataset, err := azerothcore.BuildBisDataset(all, raid, guids, enchants, time.Now())
	check(err)

	var sql bytes.Buffer
	check(azerothcore.WriteBisSQL(&sql, dataset))
	check(os.WriteFile(*out, sql.Bytes(), 0o644))
	if *luaOut != "" {
		var lua bytes.Buffer
		check(azerothcore.WriteBisLua(&lua, dataset))
		check(os.WriteFile(*luaOut, lua.Bytes(), 0o644))
	}

	printSummary(dataset)
	log.Printf("wrote dataset %s to %s", dataset.Version, *out)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func lookUpGUIDs(raid *azerothcore.Roster) (map[string]uint32, error) {
	db, err := azerothcore.OpenDB(*dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	names := make([]string, len(raid.Characters))
	for i, character := range raid.Characters {
		names[i] = character.Name
	}
	return azerothcore.CharacterGUIDs(db, names)
}

// printSummary includes what a first sync costs, since that's what a 255-byte wire makes slow.
func printSummary(dataset *azerothcore.BisDataset) {
	for _, warning := range dataset.Warnings {
		fmt.Printf("! %s\n", warning)
	}
	fmt.Printf("dataset %s: sim %s, catalog %s, objective %s, fingerprint %s\n",
		dataset.Version, dataset.SimCommit, dataset.CatalogDate, dataset.Objective, dataset.Fingerprint)

	type totals struct{ phases, bytes, frames int }
	perSubject := map[int]*totals{}
	var all totals
	for i := range dataset.Blocks {
		block := &dataset.Blocks[i]
		frames, err := azerothcore.BlockFrames(dataset.Version, block.ID(), block.Payload)
		check(err)
		t := perSubject[block.SubjectID]
		if t == nil {
			t = &totals{}
			perSubject[block.SubjectID] = t
		}
		for _, sum := range []*totals{t, &all} {
			sum.phases++
			sum.bytes += len(block.Payload)
			sum.frames += len(frames)
		}
	}

	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, subject := range dataset.Subjects {
		name := subject.Name
		if subject.Kind == azerothcore.BisSubjectSpec {
			name = "(spec)"
		}
		t := perSubject[subject.ID]
		fmt.Fprintf(table, "%d\t%s\t%s\t%s\t%d phases\t%d bytes\t%d frames\n", subject.ID, name,
			azerothcore.BisClassNames[subject.ClassID], subject.SpecName, t.phases, t.bytes, t.frames)
	}
	table.Flush()
	fmt.Printf("%d subjects, %d blocks, %d bytes, %d BLK frames (%s at 10 frames per 100 ms tick)\n",
		len(dataset.Subjects), len(dataset.Blocks), all.bytes, all.frames,
		(time.Duration(all.frames) * 10 * time.Millisecond).String())
	if len(dataset.Warnings) > 0 {
		fmt.Printf("%d warnings above\n", len(dataset.Warnings))
	}
}
