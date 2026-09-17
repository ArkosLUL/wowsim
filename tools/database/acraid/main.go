// acraid exports characters from an AzerothCore server into the roster JSON the sim's AzerothCore
// importers read. It only reads from the server.
//
// The server writes gear, talents and glyphs when it saves a character, so run saveall in the
// worldserver console first, or the export is as old as the last save.
//
// go run ./tools/database/acraid -leader Deathsong -out raid.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

var (
	dsn         = flag.String("dsn", azerothcore.DefaultCharactersDSN, "AzerothCore database DSN")
	leader      = flag.String("leader", "", "export the group this character is in, keeping its subgroups")
	names       = flag.String("names", "", "comma separated characters to export, filling subgroups of 5 in order; with -leader they top up the group instead")
	acContainer = flag.String("acContainer", "ac-worldserver", "worldserver container to copy DBC files from when -dbcDir is empty")
	dbcDir      = flag.String("dbcDir", "", "directory holding the server's DBC files; copied from -acContainer when empty")
	trees       = flag.String("trees", "ui/core/talents/trees", "sim talent tree layouts")
	simDB       = flag.String("simDb", "assets/database/db.json", "sim item database, which decides whether a reforge does anything")
	minSkill    = flag.Int("minSkill", azerothcore.DefaultMinSkill, "skill level a profession counts as known at")
	out         = flag.String("out", "", "roster JSON to write (required)")
)

func main() {
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}
	selector := azerothcore.Selector{Leader: *leader}
	for _, name := range strings.Split(*names, ",") {
		if name = strings.TrimSpace(name); name != "" {
			selector.Names = append(selector.Names, name)
		}
	}

	db, err := azerothcore.OpenDB(*dsn)
	check(err)
	defer db.Close()

	dbc, err := loadDBC()
	check(err)
	talentTrees, err := azerothcore.LoadTalentTrees(*trees)
	check(err)
	itemStats, err := azerothcore.SimItemStats(*simDB)
	check(err)

	roster, err := azerothcore.BuildRoster(db, dbc, talentTrees, selector, int32(*minSkill), itemStats)
	check(err)

	data, err := json.MarshalIndent(roster, "", "  ")
	check(err)
	check(os.WriteFile(*out, append(data, '\n'), 0o644))

	printSummary(roster)
	log.Printf("wrote %d characters to %s", len(roster.Characters), *out)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// loadDBC reads -dbcDir, or copies the DBCs out of -acContainer into a temp dir it removes before
// returning, since check's log.Fatal would skip a deferred cleanup in main.
func loadDBC() (*azerothcore.RosterDBC, error) {
	if *dbcDir != "" {
		return azerothcore.LoadRosterDBC(*dbcDir)
	}
	dir, err := os.MkdirTemp("", "acraid-dbc")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := azerothcore.CopyDBCFilesFromContainer(*acContainer, dir, azerothcore.RosterDBCFileNames()); err != nil {
		return nil, err
	}
	return azerothcore.LoadRosterDBC(dir)
}

func printSummary(roster *azerothcore.Roster) {
	for _, warning := range roster.Warnings {
		fmt.Printf("! %s\n", warning)
	}

	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, character := range roster.Characters {
		race := azerothcore.RaceNames[character.RaceID]
		if character.SwapRaceID != 0 {
			race += " as " + azerothcore.RaceNames[character.SwapRaceID]
		}
		role := ""
		if character.MemberFlags&2 != 0 {
			role = "main tank"
		}
		reforges := 0
		for _, item := range character.Gear {
			if item.Reforge != nil {
				reforges++
			}
		}
		fmt.Fprintf(table, "%s\tsg %d\t%s\t%s\t%s\t%d pts\t%d items\t%d reforges\t%d/%d glyphs\t%s\n",
			character.Name, character.Subgroup, azerothcore.ClassNames[character.ClassID], race, role,
			talentPoints(character.Talents), len(character.Gear), reforges,
			len(character.Glyphs.Major), len(character.Glyphs.Minor), strings.Join(character.Professions, ", "))
	}
	table.Flush()

	for _, character := range roster.Characters {
		for _, warning := range character.Warnings {
			fmt.Printf("! %s: %s\n", character.Name, warning)
		}
	}
}

func talentPoints(talents string) int {
	points := 0
	for _, digit := range talents {
		if digit >= '0' && digit <= '9' {
			points += int(digit - '0')
		}
	}
	return points
}
