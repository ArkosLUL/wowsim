// acdiff compares the sim's item database (built from WotLK Classic Wowhead data) with an
// AzerothCore server's items, item effects, set bonuses, gems and enchants. It only reads from
// the server.
//
// go run ./tools/database/acdiff
package main

import (
	"cmp"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/tools"
	"github.com/wowsims/wotlk/tools/database"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

var (
	dsn           = flag.String("dsn", azerothcore.DefaultDSN, "AzerothCore world database DSN")
	acContainer   = flag.String("acContainer", "ac-worldserver", "worldserver container to copy DBC files from when -dbcDir is empty")
	dbcDir        = flag.String("dbcDir", "", "directory holding the server's DBC files; copied from -acContainer when empty")
	acRepo        = flag.String("acRepo", "", "AzerothCore repo, scanned for module and custom SQL touching items or spells (required)")
	simDB         = flag.String("simDb", "assets/database/db.json", "sim item database")
	leftoverDB    = flag.String("leftoverDb", "assets/database/leftover_db.json", "sim non-simmable item database")
	spellTooltips = flag.String("spellTooltips", "assets/db_inputs/wowhead_spell_tooltips.csv", "Classic spell tooltips scraped from Wowhead")
	simSrc        = flag.String("simSrc", "sim", "sim Go sources, searched for hardcoded item effects and set bonuses")
	outDir        = flag.String("outDir", "docs/azerothcore-item-diff/data", "output directory for CSVs and summary.md")
)

type context struct {
	world        *sql.DB
	dbc          *azerothcore.DBC
	items        map[int32]*azerothcore.ItemRow
	procs        map[int32]azerothcore.SpellProc
	enchantProcs map[int32]azerothcore.EnchantProc
	obtainable   map[int32][]string

	sim       *database.WowDatabase
	leftovers *database.WowDatabase
	tooltips  map[int32]database.WowheadItemResponse
	goIndex   *goSourceIndex
	modules   moduleRefs
}

func main() {
	flag.Parse()
	if *acRepo == "" {
		log.Fatal("-acRepo is required: the AzerothCore repo the server was built from")
	}

	world, err := azerothcore.OpenWorldDB(*dsn)
	check(err)
	defer world.Close()

	dbc, err := loadDBC()
	check(err)
	overrides, err := azerothcore.ApplySpellDBCOverrides(world, dbc)
	check(err)
	cooldownOverrides, err := azerothcore.ApplySpellCooldownOverrides(world, dbc)
	check(err)
	log.Printf("loaded %d spells (%d from spell_dbc, %d cooldowns from spell_cooldown_overrides), %d item sets",
		len(dbc.Spells), overrides, cooldownOverrides, len(dbc.ItemSets))

	c := &context{world: world, dbc: dbc}
	c.items, err = azerothcore.LoadItems(world)
	check(err)
	c.procs, err = azerothcore.LoadSpellProcs(world)
	check(err)
	c.enchantProcs, err = azerothcore.LoadSpellEnchantProcs(world)
	check(err)
	c.obtainable, err = azerothcore.LoadObtainableItemIDs(world, dbc)
	check(err)
	log.Printf("loaded %d server items, %d obtainable", len(c.items), len(c.obtainable))

	c.sim = database.ReadDatabaseFromJson(tools.ReadFile(*simDB))
	c.leftovers = database.ReadDatabaseFromJson(tools.ReadFile(*leftoverDB))
	c.tooltips = database.NewWowheadSpellTooltipManager(*spellTooltips).Read()
	c.goIndex, err = indexGoSources(*simSrc)
	check(err)
	c.modules, err = scanModuleSQL(*acRepo)
	check(err)
	sim.RegisterAll()
	log.Printf("sim: %d items, %d gems, %d enchants; %d Go files; module SQL refs: %d items, %d spells, %d enchants",
		len(c.sim.Items), len(c.sim.Gems), len(c.sim.Enchants), len(c.goIndex.files),
		len(c.modules.items), len(c.modules.spells), len(c.modules.enchants))

	check(os.MkdirAll(*outDir, 0o755))

	itemDiffs, missingOnServer, notComparable := c.diffItems()
	unobtainable, missingInSim := c.diffObtainability()
	itemEffects := c.diffItemEffects()
	setEffects, setIssues := c.diffSets()
	gemDiffs, gemEffects := c.diffGems()
	enchantDiffs, enchantEffects := c.diffEnchants()

	effects := slices.Concat(itemEffects, gemEffects, enchantEffects)

	c.writeItemDiffs(itemDiffs, notComparable)
	c.writeMissingOnServer(missingOnServer)
	writeObtainability("items_unobtainable_on_server.csv", []string{"id", "name", "sim_ilvl", "sim_sources"}, unobtainable, false)
	writeObtainability("items_missing_in_sim.csv", []string{"id", "name", "server_ilvl", "sim_filter_tags", "server_sources"}, missingInSim, true)
	writeEffects("effects_diff.csv", effects)
	writeEffects("sets_diff.csv", setEffects)
	writeSetIssues(setIssues)
	writeGemOrEnchantDiffs("gems_diff.csv", gemDiffs)
	writeGemOrEnchantDiffs("enchants_diff.csv", enchantDiffs)

	c.writeSummary(summaryInput{
		itemDiffs: itemDiffs, missingOnServer: missingOnServer, notComparable: notComparable,
		unobtainable: unobtainable, missingInSim: missingInSim,
		effects: effects, setEffects: setEffects, setIssues: setIssues,
		gemDiffs: gemDiffs, enchantDiffs: enchantDiffs,
	})
	log.Printf("wrote results to %s", *outDir)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// loadDBC reads -dbcDir, or copies the DBCs out of -acContainer into a temp dir it removes before
// returning, since check's log.Fatal would skip a deferred cleanup in main.
func loadDBC() (*azerothcore.DBC, error) {
	if *dbcDir != "" {
		return azerothcore.LoadDBC(*dbcDir)
	}
	dir, err := os.MkdirTemp("", "acdiff-dbc")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := azerothcore.CopyDBCFromContainer(*acContainer, dir); err != nil {
		return nil, err
	}
	return azerothcore.LoadDBC(dir)
}

func sortedKeys[V any](m map[int32]V) []int32 {
	keys := make([]int32, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func sortedEnchantKeys[V any](m map[database.EnchantDBKey]V) []database.EnchantDBKey {
	keys := make([]database.EnchantDBKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b database.EnchantDBKey) int {
		return cmp.Or(cmp.Compare(a.EffectID, b.EffectID), cmp.Compare(a.ItemID, b.ItemID), cmp.Compare(a.SpellID, b.SpellID))
	})
	return keys
}

func outPath(name string) string {
	return fmt.Sprintf("%s/%s", *outDir, name)
}
