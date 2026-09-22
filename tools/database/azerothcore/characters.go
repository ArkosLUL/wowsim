package azerothcore

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// DefaultCharactersDSN reaches both databases this needs, so the queries name them.
const DefaultCharactersDSN = "root:password@tcp(127.0.0.1:3306)/"

// Selector picks the characters to export: everyone in a group, found through one of its members, or
// a list of names. Both together export the group plus anyone on the list who isn't in it.
type Selector struct {
	Leader string
	Names  []string
}

// maxRaidSize is what the sim can hold: 8 subgroups of 5.
const maxRaidSize = 40

// BuildRoster reads a group of characters off the server. It only reads, and the server writes gear,
// talents and glyphs on character save, so run saveall in the worldserver console first. minSkill is
// where a profession counts as known.
func BuildRoster(db *sql.DB, dbc *RosterDBC, trees TalentTrees, selector Selector, minSkill int32,
	itemStats ItemStats) (*Roster, error) {
	roster := &Roster{
		Version:    RosterVersion,
		ExportedAt: time.Now().UTC().Truncate(time.Second),
		Warnings:   []string{},
		Characters: []*RosterCharacter{},
	}

	members, err := selectMembers(db, selector, roster)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, errors.New("no characters selected")
	}
	if len(members) > maxRaidSize {
		return nil, fmt.Errorf("%d characters, the sim holds %d", len(members), maxRaidSize)
	}

	guids := make([]uint32, len(members))
	for i, member := range members {
		guids[i] = member.guid
	}
	rows, err := loadCharacters(db, guids)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		character := rows[member.guid]
		if character == nil {
			return nil, fmt.Errorf("character %d has no row in `characters`", member.guid)
		}
		character.Subgroup, character.MemberFlags = member.subgroup, member.memberFlags
	}

	// loadEquippedItems has to come before loadReforges, which hangs each reforge off the item it finds
	for _, load := range []func(*sql.DB, []uint32, map[uint32]*CharacterRows) (string, error){
		loadEquippedItems, loadTalents, loadGlyphs, loadSkills, loadReforges, loadRacialSwaps, loadQuivers,
	} {
		warning, err := load(db, guids, rows)
		if err != nil {
			return nil, err
		}
		if warning != "" {
			roster.Warnings = append(roster.Warnings, warning)
		}
	}

	for _, member := range members {
		roster.Characters = append(roster.Characters, BuildCharacter(rows[member.guid], dbc, trees, minSkill, itemStats))
	}
	return roster, nil
}

type member struct {
	guid        uint32
	subgroup    int32
	memberFlags int32
}

func selectMembers(db *sql.DB, selector Selector, roster *Roster) ([]member, error) {
	switch {
	case selector.Leader != "":
		members, leaderGUID, groupLeaderGUID, err := selectGroupMembers(db, selector.Leader)
		if err != nil {
			return nil, err
		}
		leadsGroup := leaderGUID == groupLeaderGUID
		roster.Group = RosterGroup{Selector: "leader", Leader: selector.Leader, LeaderIsGroupLeader: &leadsGroup,
			Names: selector.Names}
		if !leadsGroup {
			roster.Warnings = append(roster.Warnings, fmt.Sprintf("%s isn't the leader of their group", selector.Leader))
		}
		if len(selector.Names) == 0 {
			return members, nil
		}

		extras, err := selectNamedMembers(db, selector.Names)
		if err != nil {
			return nil, err
		}
		return addExtraMembers(members, extras)
	case len(selector.Names) > 0:
		members, err := selectNamedMembers(db, selector.Names)
		if err != nil {
			return nil, err
		}
		roster.Group = RosterGroup{Selector: "names", Names: selector.Names}
		return members, nil
	}
	return nil, errors.New("no leader or names given")
}

// addExtraMembers puts characters who aren't in the group into the first subgroup with room, which is
// how -names tops up a raid someone is missing from.
func addExtraMembers(members, extras []member) ([]member, error) {
	sizes := map[int32]int{}
	for _, member := range members {
		sizes[member.subgroup]++
	}

	for _, extra := range extras {
		if slices.ContainsFunc(members, func(m member) bool { return m.guid == extra.guid }) {
			continue
		}
		subgroup := int32(-1)
		for group := int32(0); group < maxRaidSize/5; group++ {
			if sizes[group] < 5 {
				subgroup = group
				break
			}
		}
		if subgroup < 0 {
			return nil, fmt.Errorf("the group is full, so there's no room for the characters named")
		}
		extra.subgroup = subgroup
		sizes[subgroup]++
		members = append(members, extra)
	}

	slices.SortStableFunc(members, func(a, b member) int { return cmp.Compare(a.subgroup, b.subgroup) })
	return members, nil
}

// selectGroupMembers takes any member of the group, since a raid's saved leader isn't always the one
// running it.
func selectGroupMembers(db *sql.DB, name string) (members []member, memberGUID, groupLeaderGUID uint32, err error) {
	err = db.QueryRow("SELECT guid FROM acore_characters.characters WHERE name = ?", name).Scan(&memberGUID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, 0, fmt.Errorf("no character named %q", name)
	} else if err != nil {
		return nil, 0, 0, err
	}

	var groupGUID uint32
	err = db.QueryRow("SELECT g.guid, g.leaderGuid FROM acore_characters.group_member gm"+
		" JOIN acore_characters.`groups` g ON g.guid = gm.guid WHERE gm.memberGuid = ?", memberGUID).Scan(&groupGUID, &groupLeaderGUID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, 0, fmt.Errorf("%s isn't in a group", name)
	} else if err != nil {
		return nil, 0, 0, err
	}

	// ordered so the export is stable, since the game doesn't store a position within a subgroup
	rows, err := db.Query("SELECT gm.memberGuid, gm.subgroup, gm.memberFlags FROM acore_characters.group_member gm"+
		" JOIN acore_characters.characters c ON c.guid = gm.memberGuid WHERE gm.guid = ? ORDER BY gm.subgroup, c.name", groupGUID)
	if err != nil {
		return nil, 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var member member
		if err := rows.Scan(&member.guid, &member.subgroup, &member.memberFlags); err != nil {
			return nil, 0, 0, err
		}
		members = append(members, member)
	}
	return members, memberGUID, groupLeaderGUID, rows.Err()
}

// selectNamedMembers keeps the given order and fills subgroups of 5 with it.
func selectNamedMembers(db *sql.DB, names []string) ([]member, error) {
	placeholders, args := inClause(names)
	rows, err := db.Query("SELECT guid, name FROM acore_characters.characters WHERE name IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	guids := make(map[string]uint32, len(names))
	for rows.Next() {
		var guid uint32
		var name string
		if err := rows.Scan(&guid, &name); err != nil {
			return nil, err
		}
		guids[strings.ToLower(name)] = guid
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var missing []string
	for _, name := range names {
		if _, ok := guids[strings.ToLower(name)]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("no character named %s", strings.Join(missing, ", "))
	}

	members := make([]member, 0, len(names))
	for _, name := range names {
		guid := guids[strings.ToLower(name)]
		if slices.ContainsFunc(members, func(m member) bool { return m.guid == guid }) {
			return nil, fmt.Errorf("%s is listed twice", name)
		}
		members = append(members, member{guid: guid, subgroup: int32(len(members) / 5)})
	}
	return members, nil
}

func loadCharacters(db *sql.DB, guids []uint32) (map[uint32]*CharacterRows, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT guid, name, class, race, level, activeTalentGroup FROM acore_characters.characters"+
		" WHERE guid IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	characters := make(map[uint32]*CharacterRows, len(guids))
	for rows.Next() {
		var guid uint32
		character := &CharacterRows{Skills: map[int32]int32{}}
		if err := rows.Scan(&guid, &character.Name, &character.ClassID, &character.RaceID, &character.Level,
			&character.ActiveTalentGroup); err != nil {
			return nil, err
		}
		characters[guid] = character
	}
	return characters, rows.Err()
}

func loadEquippedItems(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query(fmt.Sprintf(`SELECT ci.guid, ci.slot, ii.guid, ii.itemEntry, ii.enchantments,
			COALESCE(ii.randomPropertyId, 0), it.entry IS NOT NULL,
			(COALESCE(it.socketColor_1, 0) <> 0) + (COALESCE(it.socketColor_2, 0) <> 0) + (COALESCE(it.socketColor_3, 0) <> 0)
		FROM acore_characters.character_inventory ci
		JOIN acore_characters.item_instance ii ON ii.guid = ci.item
		LEFT JOIN acore_world.item_template it ON it.entry = ii.itemEntry
		WHERE ci.bag = 0 AND ci.slot < %d AND ci.slot NOT IN (%d, %d) AND ci.guid IN (%s)`,
		ACSlotCount, ACSlotShirt, ACSlotTabard, placeholders), args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		var knownTemplate int32
		var item EquippedItem
		if err := rows.Scan(&guid, &item.ACSlot, &item.GUID, &item.ItemID, &item.Enchantments, &item.RandomPropertyID,
			&knownTemplate, &item.NativeSockets); err != nil {
			return "", err
		}
		item.KnownTemplate = knownTemplate != 0
		if character := characters[guid]; character != nil {
			character.Items = append(character.Items, item)
		}
	}
	return "", rows.Err()
}

func loadTalents(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT guid, spell, specMask FROM acore_characters.character_talent WHERE guid IN ("+placeholders+")", args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		var spell, specMask int32
		if err := rows.Scan(&guid, &spell, &specMask); err != nil {
			return "", err
		}
		character := characters[guid]
		if character != nil && specMask&(1<<character.ActiveTalentGroup) != 0 {
			character.TalentSpells = append(character.TalentSpells, spell)
		}
	}
	return "", rows.Err()
}

func loadGlyphs(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT guid, talentGroup, glyph1, glyph2, glyph3, glyph4, glyph5, glyph6"+
		" FROM acore_characters.character_glyphs WHERE guid IN ("+placeholders+")", args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		var talentGroup int32
		var glyphs [6]int32
		if err := rows.Scan(&guid, &talentGroup, &glyphs[0], &glyphs[1], &glyphs[2], &glyphs[3], &glyphs[4], &glyphs[5]); err != nil {
			return "", err
		}
		if character := characters[guid]; character != nil && talentGroup == character.ActiveTalentGroup {
			character.Glyphs = glyphs
		}
	}
	return "", rows.Err()
}

func loadSkills(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT guid, skill, value FROM acore_characters.character_skills WHERE guid IN ("+placeholders+")", args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		var skill, value int32
		if err := rows.Scan(&guid, &skill, &value); err != nil {
			return "", err
		}
		if character := characters[guid]; character != nil {
			character.Skills[skill] = value
		}
	}
	return "", rows.Err()
}

// loadReforges attaches mod-reforging's rows to the equipped items they belong to.
func loadReforges(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT guid, item_guid, stat_decrease, stat_increase FROM acore_characters.character_reforging"+
		" WHERE guid IN ("+placeholders+")", args...)
	if err != nil {
		if warning := missingTableWarning(err, "mod-reforging"); warning != "" {
			return warning, nil
		}
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid, itemGUID uint32
		var reforge ReforgeRow
		if err := rows.Scan(&guid, &itemGUID, &reforge.StatDecrease, &reforge.StatIncrease); err != nil {
			return "", err
		}
		character := characters[guid]
		if character == nil {
			continue
		}
		// most rows are for bagged items, which aren't exported
		for i := range character.Items {
			if character.Items[i].GUID == itemGUID {
				character.Items[i].Reforge = &reforge
				break
			}
		}
	}
	return "", rows.Err()
}

// loadQuivers flags a character carrying a quiver or ammo pouch in one of their bag slots.
func loadQuivers(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query(fmt.Sprintf(`SELECT ci.guid FROM acore_characters.character_inventory ci
			JOIN acore_characters.item_instance ii ON ii.guid = ci.item
			JOIN acore_world.item_template it ON it.entry = ii.itemEntry
			WHERE ci.bag = 0 AND ci.slot BETWEEN %d AND %d AND it.class = %d AND ci.guid IN (%s)`,
		ACSlotBagStart, ACSlotBagEnd, ItemClassQuiver, placeholders), args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		if err := rows.Scan(&guid); err != nil {
			return "", err
		}
		if character := characters[guid]; character != nil {
			character.HasQuiver = true
		}
	}
	return "", rows.Err()
}

func loadRacialSwaps(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT guid, selected_race FROM acore_characters.character_racial_swap WHERE guid IN ("+placeholders+")", args...)
	if err != nil {
		if warning := missingTableWarning(err, "mod-racial-trait-swap"); warning != "" {
			return warning, nil
		}
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		var race int32
		if err := rows.Scan(&guid, &race); err != nil {
			return "", err
		}
		if character := characters[guid]; character != nil {
			character.SwapRaceID = race
		}
	}
	return "", rows.Err()
}

// missingTableWarning turns "no such table" into a warning, since a server without the module simply
// has nothing to export. Everything else stays an error.
func missingTableWarning(err error, module string) string {
	const errNoSuchTable = 1146
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == errNoSuchTable {
		return fmt.Sprintf("%s isn't installed on the server, skipped: %s", module, mysqlErr.Message)
	}
	return ""
}

// inClause builds the placeholders for an IN list, which MySQL has no array parameter for.
func inClause[T any](values []T) (string, []any) {
	args := make([]any, len(values))
	for i, value := range values {
		args[i] = value
	}
	return strings.TrimPrefix(strings.Repeat(",?", len(values)), ","), args
}
