package main

import (
	"flag"
	"log"
	"os"
	"slices"

	"github.com/wowsims/wotlk/tools/database"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

var acDSN = flag.String("dsn", azerothcore.ContainerDSN(), "AzerothCore world database, for -gen=azerothcore")
var acDBCDir = flag.String("dbcDir", "", "the server's DBC files, for -gen=azerothcore; copied from -acContainer when empty")
var acContainer = flag.String("acContainer", "ac-worldserver", "worldserver container to copy DBC files from when -dbcDir is empty")

// serverData is what -gen=azerothcore reads from the live server, with SELECTs only.
type serverData struct {
	dbc        *azerothcore.DBC
	items      map[int32]*azerothcore.ItemRow
	obtainable map[int32][]string
}

func loadServerData() (*serverData, error) {
	world, err := azerothcore.OpenDB(*acDSN)
	if err != nil {
		return nil, err
	}
	defer world.Close()

	server := &serverData{}
	if server.dbc, err = loadServerDBC(); err != nil {
		return nil, err
	}
	if _, err = azerothcore.ApplySpellDBCOverrides(world, server.dbc); err != nil {
		return nil, err
	}
	if server.items, err = azerothcore.LoadItems(world); err != nil {
		return nil, err
	}
	if server.obtainable, err = azerothcore.LoadObtainableItemIDs(world, server.dbc); err != nil {
		return nil, err
	}
	return server, nil
}

func loadServerDBC() (*azerothcore.DBC, error) {
	if *acDBCDir != "" {
		return azerothcore.LoadDBC(*acDBCDir)
	}
	dir, err := os.MkdirTemp("", "gen_db-dbc")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := azerothcore.CopyDBCFromContainer(*acContainer, dir); err != nil {
		return nil, err
	}
	return azerothcore.LoadDBC(dir)
}

// applyServerData overwrites item data with the server's for every item a player can get there.
// Items the server lacks or nothing awards keep their Wowhead data, and so do heirlooms and random
// enchant items, whose stats item_template doesn't hold. Gems whose id the server uses for a
// non-gem item are dropped.
func applyServerData(db *database.WowDatabase, server *serverData) {
	var applied, missing, unobtainable, notComparable int
	for id, item := range db.Items {
		row := server.items[id]
		if row == nil {
			missing++
			continue
		}
		if len(server.obtainable[id]) == 0 {
			unobtainable++
			continue
		}
		converted := azerothcore.ConvertItem(row, server.dbc)
		if converted.NotComparable != "" {
			notComparable++
			continue
		}
		converted.ApplyTo(item)
		applied++
	}
	log.Printf("azerothcore: server data on %d items; kept Wowhead data on %d missing on the server, %d unobtainable there, %d not comparable",
		applied, missing, unobtainable, notComparable)

	var dropped []int32
	for id := range db.Gems {
		row := server.items[id]
		if row == nil {
			continue
		}
		if gem, _ := azerothcore.ConvertGem(row, server.dbc); gem == nil {
			delete(db.Gems, id)
			dropped = append(dropped, id)
		}
	}
	slices.Sort(dropped)
	log.Printf("azerothcore: dropped gems that aren't gems on the server: %v", dropped)
}
