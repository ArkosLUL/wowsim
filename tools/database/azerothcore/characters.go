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

// RosterInputs is what BuildRoster reads besides the databases.
type RosterInputs struct {
	DBC       *RosterDBC
	Trees     TalentTrees
	PetTrees  PetTalentTrees
	MinSkill  int32 // where a profession counts as known
	ItemStats ItemStats
	// WorldBuffs is the bots' AiPlayerbot.WorldBuffMatrix, nil when none was given.
	WorldBuffs []WorldBuff
	// Players, when set, are the only characters someone plays; everyone else counts as a bot.
	Players []string
}

// BuildRoster reads a group of characters off the server. It only reads, and the server writes gear,
// talents and glyphs on character save, so run saveall in the worldserver console first.
func BuildRoster(db *sql.DB, selector Selector, inputs RosterInputs) (*Roster, error) {
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
	roster.Group.Players = inputs.Players
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
		loadPets, loadAuras, loadBags, loadShadowfiend, loadBotState,
	} {
		warning, err := load(db, guids, rows)
		if err != nil {
			return nil, err
		}
		if warning != "" {
			roster.Warnings = append(roster.Warnings, warning)
		}
	}
	if inputs.Players != nil {
		roster.Warnings = append(roster.Warnings, markPlayers(rows, inputs.Players)...)
	}

	auraItems, err := loadAuraItems(db)
	if err != nil {
		return nil, err
	}
	if inputs.WorldBuffs == nil {
		roster.Warnings = append(roster.Warnings, "no world-buff matrix given, so the bots' flask, elixirs and food stay as they are")
	}
	loadout := Loadout{DBC: inputs.DBC, PetTrees: inputs.PetTrees, WorldBuffs: inputs.WorldBuffs, AuraItems: auraItems}
	for _, member := range members {
		character := BuildCharacter(rows[member.guid], inputs.DBC, inputs.Trees, inputs.MinSkill, inputs.ItemStats)
		BuildLoadout(character, rows[member.guid], loadout)
		roster.Characters = append(roster.Characters, character)
	}
	return roster, nil
}

// markPlayers makes exactly the named characters the played ones, and warns about names the export
// doesn't have.
func markPlayers(characters map[uint32]*CharacterRows, players []string) []string {
	found := map[string]bool{}
	for _, character := range characters {
		played := slices.ContainsFunc(players, func(name string) bool { return strings.EqualFold(name, character.Name) })
		character.Bot = !played
		if played {
			found[strings.ToLower(character.Name)] = true
		}
	}
	var warnings []string
	for _, name := range players {
		if !found[strings.ToLower(name)] {
			warnings = append(warnings, fmt.Sprintf("-players names %s, who isn't in the export", name))
		}
	}
	return warnings
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
	// an ammo's DPS is the middle of its damage range: Iceblade Arrow's 91.5 is stored as 91 to 92
	rows, err := db.Query("SELECT c.guid, c.name, c.class, c.race, c.level, c.activeTalentGroup, c.ammoId,"+
		" COALESCE((it.dmg_min1 + it.dmg_max1) / 2, 0) FROM acore_characters.characters c"+
		" LEFT JOIN acore_world.item_template it ON it.entry = c.ammoId WHERE c.guid IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	characters := make(map[uint32]*CharacterRows, len(guids))
	for rows.Next() {
		var guid uint32
		character := &CharacterRows{Skills: map[int32]int32{}}
		if err := rows.Scan(&guid, &character.Name, &character.ClassID, &character.RaceID, &character.Level,
			&character.ActiveTalentGroup, &character.AmmoID, &character.AmmoDPS); err != nil {
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

// loadPets reads the pet each character has out, character_pet slot 0, and its spells.
func loadPets(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT p.owner, p.id, p.entry, p.name, COALESCE(ct.family, 0) FROM acore_characters.character_pet p"+
		" LEFT JOIN acore_world.creature_template ct ON ct.entry = p.entry WHERE p.slot = 0 AND p.owner IN ("+placeholders+")"+
		" ORDER BY p.id", args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	owners := map[uint32]*PetRows{} // pet id -> the owner's pet
	for rows.Next() {
		var owner, id uint32
		pet := &PetRows{}
		if err := rows.Scan(&owner, &id, &pet.Entry, &pet.Name, &pet.Family); err != nil {
			return "", err
		}
		if character := characters[owner]; character != nil && character.Pet == nil {
			character.Pet = pet
			owners[id] = pet
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	spells, err := db.Query("SELECT ps.guid, ps.spell FROM acore_characters.pet_spell ps"+
		" JOIN acore_characters.character_pet p ON p.id = ps.guid WHERE p.slot = 0 AND p.owner IN ("+placeholders+")", args...)
	if err != nil {
		return "", err
	}
	defer spells.Close()
	for spells.Next() {
		var id uint32
		var spell int32
		if err := spells.Scan(&id, &spell); err != nil {
			return "", err
		}
		if pet := owners[id]; pet != nil {
			pet.Spells = append(pet.Spells, spell)
		}
	}
	return "", spells.Err()
}

func loadAuras(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT guid, spell FROM acore_characters.character_aura WHERE guid IN ("+placeholders+")", args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		var spell int32
		if err := rows.Scan(&guid, &spell); err != nil {
			return "", err
		}
		if character := characters[guid]; character != nil {
			character.Auras = append(character.Auras, spell)
		}
	}
	return "", rows.Err()
}

// loadBags reads the consumables in the backpack and the bags in the 4 bag slots, one row per item
// however many stacks there are. The bank is left out, since nobody drinks from it.
func loadBags(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query(fmt.Sprintf(`SELECT ci.guid, it.entry, it.subclass, it.Quality, it.ItemLevel,
			it.spellid_1, it.spellid_2, it.spellid_3, it.spellid_4, it.spellid_5
		FROM acore_characters.character_inventory ci
		JOIN acore_characters.item_instance ii ON ii.guid = ci.item
		JOIN acore_world.item_template it ON it.entry = ii.itemEntry
		LEFT JOIN acore_characters.character_inventory holder ON holder.item = ci.bag
		WHERE it.class = %d AND ci.guid IN (%s)
			AND ((ci.bag = 0 AND ci.slot BETWEEN %d AND %d) OR (holder.bag = 0 AND holder.slot BETWEEN %d AND %d))
		ORDER BY ci.guid, it.entry`,
		ItemClassConsumable, placeholders, ACSlotBackpackStart, ACSlotBackpackEnd, ACSlotBagStart, ACSlotBagEnd), args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		var item BagItem
		if err := rows.Scan(&guid, &item.ItemID, &item.SubClass, &item.Quality, &item.ItemLevel,
			&item.Spells[0], &item.Spells[1], &item.Spells[2], &item.Spells[3], &item.Spells[4]); err != nil {
			return "", err
		}
		character := characters[guid]
		if character != nil && !slices.ContainsFunc(character.Bags, func(bagged BagItem) bool { return bagged.ItemID == item.ItemID }) {
			character.Bags = append(character.Bags, item)
		}
	}
	return "", rows.Err()
}

// loadShadowfiend flags the priests who know Shadowfiend, which a bot shadow priest drinks no mana
// potions for.
func loadShadowfiend(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	const shadowfiend = 34433
	placeholders, args := inClause(guids)
	rows, err := db.Query(fmt.Sprintf("SELECT guid FROM acore_characters.character_spell WHERE spell = %d AND guid IN (%s)",
		shadowfiend, placeholders), args...)
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
			character.KnowsShadowfiend = true
		}
	}
	return "", rows.Err()
}

// loadBotState tells bots from characters someone plays. mod-playerbots saves a bot's strategies in
// playerbots_db_store whenever it logs one out, and a random bot's account is typed 1, so a character
// with neither has never been a bot.
func loadBotState(db *sql.DB, guids []uint32, characters map[uint32]*CharacterRows) (string, error) {
	placeholders, args := inClause(guids)
	rows, err := db.Query("SELECT guid, `key`, value FROM acore_playerbots.playerbots_db_store WHERE guid IN ("+placeholders+")", args...)
	if err != nil {
		if warning := missingTableWarning(err, "mod-playerbots"); warning != "" {
			return warning + ", so every character counts as played", nil
		}
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var guid uint32
		var key string
		var value sql.NullString
		if err := rows.Scan(&guid, &key, &value); err != nil {
			return "", err
		}
		character := characters[guid]
		if character == nil {
			continue
		}
		character.Bot = true
		if key != "co" && key != "nc" && key != "dead" {
			continue
		}
		if character.Strategies == nil {
			character.Strategies = &BotStrategies{}
		}
		switch strategies := ParseStrategies(value.String); key {
		case "co":
			character.Strategies.Combat = strategies
		case "nc":
			character.Strategies.NonCombat = strategies
		default:
			character.Strategies.Dead = strategies
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	const randomBotAccount = 1
	accounts, err := db.Query(fmt.Sprintf("SELECT c.guid FROM acore_characters.characters c"+
		" JOIN acore_playerbots.playerbots_account_type t ON t.account_id = c.account"+
		" WHERE t.account_type = %d AND c.guid IN (%s)", randomBotAccount, placeholders), args...)
	if err != nil {
		if warning := missingTableWarning(err, "mod-playerbots"); warning != "" {
			return warning + ", so only characters with saved bot strategies count as bots", nil
		}
		return "", err
	}
	defer accounts.Close()
	for accounts.Next() {
		var guid uint32
		if err := accounts.Scan(&guid); err != nil {
			return "", err
		}
		if character := characters[guid]; character != nil {
			character.Bot = true
		}
	}
	return "", accounts.Err()
}

// loadAuraItems maps each flask's and elixir's buff to the item, the lowest entry when several give it.
func loadAuraItems(db *sql.DB) (map[int32]AuraItem, error) {
	rows, err := db.Query(fmt.Sprintf("SELECT spellid_1, entry, subclass FROM acore_world.item_template"+
		" WHERE class = %d AND subclass IN (%d, %d) AND spellid_1 <> 0 ORDER BY entry",
		ItemClassConsumable, itemSubClassElixir, itemSubClassFlask))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := map[int32]AuraItem{}
	for rows.Next() {
		var spell int32
		var item AuraItem
		if err := rows.Scan(&spell, &item.ItemID, &item.SubClass); err != nil {
			return nil, err
		}
		if _, ok := items[spell]; !ok {
			items[spell] = item
		}
	}
	return items, rows.Err()
}

// missingTableWarning turns "no such table" or "no such database" into a warning, since a server
// without the module simply has nothing to export. Everything else stays an error.
func missingTableWarning(err error, module string) string {
	const errNoSuchDatabase, errNoSuchTable = 1049, 1146
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && (mysqlErr.Number == errNoSuchTable || mysqlErr.Number == errNoSuchDatabase) {
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
