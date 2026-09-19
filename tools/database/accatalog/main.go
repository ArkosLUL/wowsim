// accatalog builds assets/database/server_catalog.json, the server's item catalog: every
// obtainable equippable item and gem with its progression tier, sources, equip limits and faction
// (ADR 0005). It only runs SELECTs.
//
// go run ./tools/database/accatalog
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools"
	"github.com/wowsims/wotlk/tools/database"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
	googleProto "google.golang.org/protobuf/proto"
)

var (
	dsn        = flag.String("dsn", azerothcore.ContainerDSN(), "AzerothCore world database")
	dbcDir     = flag.String("dbcDir", "/dbc", "the server's DBC files")
	classicDB  = flag.String("classicDb", "assets/database/db.json", "sim item DB, for Classic phases: the fallback tier and the comparison")
	out        = flag.String("out", "assets/database/server_catalog.json", "catalog to write")
	date       = flag.String("date", "", "catalog date, YYYY-MM-DD; default keeps the old date when nothing else changed, else today")
	explain    = flag.String("explain", "", "comma separated item ids: print every way to get each, then exit without writing")
	allHolders = flag.Bool("holders", false, "list every loot holder nothing places on a map")
)

func main() {
	flag.Parse()

	db, err := azerothcore.OpenDB(*dsn)
	check(err)
	defer db.Close()

	rows, err := azerothcore.LoadCatalogRows(db, *dbcDir)
	check(err)
	classic := database.ReadDatabaseFromJson(tools.ReadFile(*classicDB))
	rows.ClassicPhases = map[int32]int32{}
	for id, item := range classic.Items {
		rows.ClassicPhases[id] = item.Phase
	}
	for id, gem := range classic.Gems {
		rows.ClassicPhases[id] = gem.Phase
	}

	catalog, stats := azerothcore.ResolveCatalog(rows)
	if *explain != "" {
		for _, field := range strings.Split(*explain, ",") {
			id, err := strconv.Atoi(strings.TrimSpace(field))
			check(err)
			fmt.Printf("item %d:\n", id)
			for _, line := range stats.Explain(int32(id)) {
				fmt.Println("  " + line)
			}
		}
		return
	}

	var previous *proto.ServerCatalog
	if data, err := os.ReadFile(*out); err == nil {
		if previous, err = azerothcore.ParseCatalogJSON(data); err != nil {
			log.Printf("warning: can't read the old catalog, so no diff: %v", err)
			previous = nil
		}
	}
	catalog.Date = catalogDate(catalog, previous)

	data, err := azerothcore.MarshalCatalogJSON(catalog)
	check(err)
	if old, err := os.ReadFile(*out); err == nil && bytes.Equal(old, data) {
		log.Printf("%s is unchanged", *out)
	} else {
		check(os.WriteFile(*out, data, 0o644))
		log.Printf("wrote %d items and %d limit groups to %s", len(catalog.Items), len(catalog.LimitGroups), *out)
	}

	printDiff(os.Stdout, previous, catalog)
	printClassicReport(os.Stdout, catalog, stats, classic)
}

// catalogDate keeps the old date when only the date would change, so a rerun on another day
// leaves the file alone and results keep pointing at the same catalog.
func catalogDate(catalog, previous *proto.ServerCatalog) string {
	if *date != "" {
		return *date
	}
	if previous != nil {
		candidate := googleProto.Clone(catalog).(*proto.ServerCatalog)
		candidate.Date = previous.Date
		if googleProto.Equal(candidate, previous) {
			return previous.Date
		}
	}
	return time.Now().Format(time.DateOnly)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
