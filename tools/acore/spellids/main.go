// spellids collects the spell ids the sim and its item data need server data for, checks them against
// the committed spell capture, and writes the ones still missing as an ids file for `.simval spelldump`.
//
//	tools/acore/dock.sh run ./tools/acore/spellids [-out tmp/spelldump_ids.txt]
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// SharedDefines.h SpellFamilyNames captured whole, beside the id sources below.
var families = []struct {
	id   int32
	name string
}{
	{3, "mage"}, {4, "warrior"}, {5, "warlock"}, {6, "priest"}, {7, "druid"}, {8, "rogue"},
	{9, "hunter"}, {10, "paladin"}, {11, "shaman"}, {15, "death knight"}, {17, "pet"},
}

// Item spell triggers worth a capture: on use, on equip, chance on hit, use without delay.
var itemSpellTriggers = []int32{0, 1, 2, 5}

type source struct {
	name string
	ids  map[int32]bool
}

func main() {
	dumpPath := flag.String("dump", spellset.DefaultDumpPath, "committed spell capture")
	dbPath := flag.String("db", "assets/database/db.json", "the sim's item DB")
	dbcDir := flag.String("dbc", "/dbc/Clean", "DBCs: Spell, SpellItemEnchantment, GemProperties, ItemSet, Talent, TalentTab, GlyphProperties")
	dsn := flag.String("dsn", azerothcore.ContainerDSN(), "AzerothCore world database DSN")
	out := flag.String("out", "", "write the ids for `.simval spelldump ids` here")
	all := flag.Bool("all", false, "with -out, write every id, not only the ones the capture lacks")
	verbose := flag.Bool("v", false, "list unresolved spell id expressions and every missing id")
	flag.Parse()

	dump, err := spellset.ReadDump(*dumpPath)
	if err != nil {
		log.Fatal(err)
	}
	lits, err := spellset.ScanSim(".")
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range lits.TypeErrors {
		log.Printf("type error (constants depending on it are lost): %s", e)
	}

	db, err := azerothcore.OpenDB(*dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	dbc, err := azerothcore.LoadDBC(*dbcDir)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := azerothcore.ApplySpellDBCOverrides(db, dbc); err != nil {
		log.Fatal(err)
	}
	roster, err := azerothcore.LoadRosterDBC(*dbcDir)
	if err != nil {
		log.Fatal(err)
	}
	simDB, err := readSimDB(*dbPath)
	if err != nil {
		log.Fatal(err)
	}

	items, err := itemSources(db, dbc, simDB, lits)
	if err != nil {
		log.Fatal(err)
	}
	sources := []source{{"sim literals", keys(lits.Spells)}}
	sources = append(sources, items...)
	sources = append(sources, talentSources(roster)...)

	capture := map[int32]bool{}
	for _, s := range sources {
		maps.Copy(capture, s.ids)
	}
	triggered, _ := dump.Closure(capture)
	maps.Copy(capture, triggered)

	fmt.Printf("%-18s %6s %8s %8s %8s\n", "source", "ids", "in dump", "missing", "unknown")
	report := func(name string, ids map[int32]bool) {
		in, missing, unknown := 0, 0, 0
		for id := range ids {
			switch {
			case dump[id] != nil:
				in++
			case dbc.Spells[id] != nil:
				missing++
			default:
				unknown++
			}
		}
		fmt.Printf("%-18s %6d %8d %8d %8d\n", name, len(ids), in, missing, unknown)
	}
	for _, s := range sources {
		report(s.name, s.ids)
	}
	report("triggered", triggered)
	report("capture (union)", capture)
	generated, _, _ := dump.GeneratedSet(lits)
	report("gen_serverdata set", generated)
	fmt.Println("missing: in Spell.dbc or spell_dbc but not in the capture; unknown: in neither, so the server lacks it")

	fmt.Printf("\n%-18s %6s\n", "family", "in dump")
	perFamily := map[int64]int{}
	for _, spell := range dump {
		perFamily[spell.Family]++
	}
	for _, f := range families {
		fmt.Printf("%-18s %6d\n", fmt.Sprintf("%d %s", f.id, f.name), perFamily[int64(f.id)])
	}
	fmt.Printf("%-18s %6d\n", "whole capture", len(dump))

	fmt.Printf("\n%d spell id expressions aren't constants (-v lists them)\n", len(lits.Unresolved))
	if *verbose {
		for _, ref := range lits.Unresolved {
			fmt.Println("  unresolved", ref)
		}
		for _, id := range sortedIDs(capture) {
			if dump[id] == nil {
				fmt.Printf("  missing %d %s\n", id, describe(id, sources, triggered, dbc, lits))
			}
		}
	}

	if *out != "" {
		if err := writeIDs(*out, capture, dump, dbc, *all, sources, triggered, lits); err != nil {
			log.Fatal(err)
		}
	}
}

type simDB struct {
	Items []struct {
		ID int32 `json:"id"`
	} `json:"items"`
	Enchants []struct {
		EffectID int32 `json:"effectId"`
	} `json:"enchants"`
	Gems []struct {
		ID int32 `json:"id"`
	} `json:"gems"`
}

func readSimDB(path string) (*simDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	db := &simDB{}
	return db, json.Unmarshal(data, db)
}

// itemSources are the spells of db.json's items and of the items the sim names by id (consumables,
// explosives), their item sets, and the spells behind db.json's enchants and gems.
func itemSources(db *sql.DB, dbc *azerothcore.DBC, simDB *simDB, lits *spellset.Literals) ([]source, error) {
	rows, err := azerothcore.LoadItems(db)
	if err != nil {
		return nil, err
	}

	itemIDs := keys(lits.Items)
	for _, item := range simDB.Items {
		itemIDs[item.ID] = true
	}
	itemSpells, setSpells := map[int32]bool{}, map[int32]bool{}
	for id := range itemIDs {
		row := rows[id]
		if row == nil {
			continue
		}
		for i, spell := range row.SpellIDs {
			if spell > 0 && slices.Contains(itemSpellTriggers, row.SpellTriggers[i]) {
				itemSpells[spell] = true
			}
		}
		if set := dbc.ItemSets[row.ItemSet]; set != nil {
			for _, spell := range set.Spells {
				setSpells[spell] = true
			}
		}
	}

	enchantSpells := map[int32]bool{}
	addEnchant := func(enchantID int32) {
		enchant := dbc.Enchantments[enchantID]
		if enchant == nil {
			return
		}
		for i, kind := range enchant.Type {
			switch kind {
			case azerothcore.EnchantTypeCombatSpell, azerothcore.EnchantTypeEquipSpell, azerothcore.EnchantTypeUseSpell:
				if enchant.Arg[i] > 0 {
					enchantSpells[enchant.Arg[i]] = true
				}
			}
		}
	}
	for _, enchant := range simDB.Enchants {
		addEnchant(enchant.EffectID)
	}
	for _, gem := range simDB.Gems {
		if row := rows[gem.ID]; row != nil {
			if props := dbc.GemProperties[row.GemProperties]; props != nil {
				addEnchant(props.EnchantID)
			}
		}
	}

	return []source{{"items", itemSpells}, {"item sets", setSpells}, {"enchants and gems", enchantSpells}}, nil
}

// talentSources are every talent rank and glyph, class and pet, whatever family the spell is in.
func talentSources(roster *azerothcore.RosterDBC) []source {
	talents, glyphs := map[int32]bool{}, map[int32]bool{}
	for spell := range roster.TalentRanks {
		talents[spell] = true
	}
	for _, glyph := range roster.Glyphs {
		if glyph.SpellID > 0 {
			glyphs[glyph.SpellID] = true
		}
	}
	return []source{{"talents", talents}, {"glyphs", glyphs}}
}

func keys[V any](m map[int32]V) map[int32]bool {
	out := make(map[int32]bool, len(m))
	for id := range m {
		out[id] = true
	}
	return out
}

func sortedIDs(ids map[int32]bool) []int32 {
	return slices.Sorted(maps.Keys(ids))
}

// describe names a spell and where its id comes from, for the ids file and -v.
func describe(id int32, sources []source, triggered map[int32]bool, dbc *azerothcore.DBC, lits *spellset.Literals) string {
	var from []string
	for _, s := range sources {
		if s.ids[id] {
			from = append(from, s.name)
		}
	}
	if triggered[id] {
		from = append(from, "triggered")
	}
	name := "not in Spell.dbc or spell_dbc"
	if spell := dbc.Spells[id]; spell != nil {
		name = spell.Name
	}
	text := fmt.Sprintf("%s [%s]", name, strings.Join(from, ", "))
	if refs := lits.Spells[id]; len(refs) > 0 {
		text += " " + refs[0].String()
	}
	return text
}

// writeIDs writes one id per line; the spelldump parser skips everything after a '#'.
func writeIDs(path string, capture map[int32]bool, dump spellset.Dump, dbc *azerothcore.DBC, all bool,
	sources []source, triggered map[int32]bool, lits *spellset.Literals) error {
	var b strings.Builder
	count := 0
	for _, id := range sortedIDs(capture) {
		if !all && dump[id] != nil {
			continue
		}
		fmt.Fprintf(&b, "%d # %s\n", id, strings.ReplaceAll(describe(id, sources, triggered, dbc, lits), "\n", " "))
		count++
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("\nwrote %d ids to %s\n", count, path)
	return nil
}
