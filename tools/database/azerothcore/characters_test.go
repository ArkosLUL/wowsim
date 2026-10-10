package azerothcore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestMarkPlayers(t *testing.T) {
	characters := map[uint32]*CharacterRows{
		1: {Name: "Deathsong"},
		2: {Name: "Nightwarrior"},
		3: {Name: "Felesta"},
	}

	warnings := markPlayers(characters, []string{"deathsong", "Nightwarrior", "Nobody"})
	for guid, wantBot := range map[uint32]bool{1: false, 2: false, 3: true} {
		if characters[guid].Bot != wantBot {
			t.Errorf("%s: bot %v, want %v", characters[guid].Name, characters[guid].Bot, wantBot)
		}
	}
	if len(warnings) != 1 || findWarning(warnings, "Nobody") == "" {
		t.Errorf("warnings = %v", warnings)
	}
}

// groupOf is a live group: subgroups of the given sizes, guids counting up from 1, the first member
// of each flagged main tank.
func groupOf(sizes ...int) []member {
	var group []member
	for subgroup, size := range sizes {
		for i := 0; i < size; i++ {
			flags := int32(0)
			if i == 0 {
				flags = 2
			}
			group = append(group, member{guid: uint32(len(group) + 1), subgroup: int32(subgroup), memberFlags: flags})
		}
	}
	return group
}

// namedAs is what selectNamedMembers gives for a names list: list order, 5 to a subgroup.
func namedAs(guids ...uint32) []member {
	named := make([]member, len(guids))
	for i, guid := range guids {
		named[i] = member{guid: guid, name: fmt.Sprintf("Raider%d", guid), subgroup: int32(i / 5)}
	}
	return named
}

// seats prints members as "guid@subgroup", with "*" on the ones holding a member flag.
func seats(members []member) string {
	parts := make([]string, len(members))
	for i, m := range members {
		parts[i] = fmt.Sprintf("%d@%d", m.guid, m.subgroup)
		if m.memberFlags != 0 {
			parts[i] += "*"
		}
	}
	return strings.Join(parts, " ")
}

func TestSeatCore(t *testing.T) {
	for _, tc := range []struct {
		comment     string
		group       []member
		named       []member
		wantSeated  string
		wantOutside string
		wantLeftOut string
	}{
		{
			comment:    "everyone named is in the group, so they keep their subgroups and flags whatever the list order",
			group:      groupOf(2, 2),
			named:      namedAs(4, 3, 1),
			wantSeated: "1@0* 3@1* 4@1", wantLeftOut: "2@0",
		},
		{
			comment:    "the group as it stands",
			group:      groupOf(5, 3),
			named:      namedAs(1, 2, 3, 4, 5, 6, 7, 8),
			wantSeated: "1@0* 2@0 3@0 4@0 5@0 6@1* 7@1 8@1",
		},
		{
			comment:    "an outsider takes the place an unnamed member leaves in a full subgroup",
			group:      groupOf(5, 1),
			named:      namedAs(1, 2, 3, 4, 6, 90),
			wantSeated: "1@0* 2@0 3@0 4@0 90@0 6@1*", wantOutside: "90@0", wantLeftOut: "5@0",
		},
		{
			comment:    "outsiders skip the subgroups that stay full, in list order",
			group:      groupOf(5, 4),
			named:      namedAs(90, 1, 2, 3, 4, 5, 6, 7, 8, 9, 91),
			wantSeated: "1@0* 2@0 3@0 4@0 5@0 6@1* 7@1 8@1 9@1 90@1 91@2", wantOutside: "90@1 91@2",
		},
		{
			comment:    "nobody named is in the group",
			group:      groupOf(2),
			named:      namedAs(90, 91),
			wantSeated: "90@0 91@0", wantOutside: "90@0 91@0", wantLeftOut: "1@0* 2@0",
		},
		{
			comment:    "a subgroup nobody is kept from is free for outsiders",
			group:      groupOf(1, 5),
			named:      namedAs(2, 3, 4, 5, 6, 90),
			wantSeated: "90@0 2@1* 3@1 4@1 5@1 6@1", wantOutside: "90@0", wantLeftOut: "1@0*",
		},
	} {
		seated, outside, leftOut, err := seatCore(tc.group, tc.named)
		if err != nil {
			t.Errorf("%s: %v", tc.comment, err)
			continue
		}
		if got := seats(seated); got != tc.wantSeated {
			t.Errorf("%s: seated %s, want %s", tc.comment, got, tc.wantSeated)
		}
		if got := seats(outside); got != tc.wantOutside {
			t.Errorf("%s: outside %s, want %s", tc.comment, got, tc.wantOutside)
		}
		if got := seats(leftOut); got != tc.wantLeftOut {
			t.Errorf("%s: left out %s, want %s", tc.comment, got, tc.wantLeftOut)
		}
		for _, m := range seated {
			if want := fmt.Sprintf("Raider%d", m.guid); m.name != want {
				t.Errorf("%s: member %d is named %q, want %q", tc.comment, m.guid, m.name, want)
			}
		}
	}
}

func TestSeatCoreLeavesItsInputsAlone(t *testing.T) {
	group, named := groupOf(5, 1), namedAs(6, 90, 1)
	wantGroup, wantNamed := seats(group), seats(named)
	if _, _, _, err := seatCore(group, named); err != nil {
		t.Fatal(err)
	}
	if seats(group) != wantGroup || seats(named) != wantNamed {
		t.Errorf("group %s, named %s; want %s and %s", seats(group), seats(named), wantGroup, wantNamed)
	}
}

func TestSeatCoreWithoutRoom(t *testing.T) {
	group := groupOf(5, 5, 5, 5, 5, 5, 5, 5)
	guids := []uint32{90}
	for _, m := range group {
		guids = append(guids, m.guid)
	}

	_, _, _, err := seatCore(group, namedAs(guids...))
	var selection *SelectionError
	if !errors.As(err, &selection) {
		t.Errorf("41 named characters, 40 of them a full raid: err = %v, want a SelectionError", err)
	}
}

func TestCoreWarnings(t *testing.T) {
	outside := []member{{guid: 90, name: "Felesta", subgroup: 2}, {guid: 91, name: "Agony", subgroup: 4}}
	warnings := coreWarnings("Deathsong", outside, []string{"Standin", "Guest"})
	if len(warnings) != 3 {
		t.Fatalf("warnings = %q, want one for each outsider and one for the members left out", warnings)
	}
	for i, want := range [][]string{{"Felesta", "Deathsong", "party 3"}, {"Agony", "Deathsong", "party 5"}, {"Standin, Guest", "Deathsong"}} {
		for _, text := range want {
			if !strings.Contains(warnings[i], text) {
				t.Errorf("warning %q doesn't mention %q", warnings[i], text)
			}
		}
	}

	if warnings := coreWarnings("Deathsong", nil, nil); len(warnings) != 0 {
		t.Errorf("a raid that's all in the group warns: %q", warnings)
	}
}

func TestParseNames(t *testing.T) {
	for _, tc := range []struct {
		comment string
		text    string
		want    []string
	}{
		{"one name a line", "Angry\nDeathsong\n", []string{"Angry", "Deathsong"}},
		{"no newline at the end", "Angry\nDeathsong", []string{"Angry", "Deathsong"}},
		{"blank lines and comments", "# party 1\nAngry\n\n  # party 2\n\nElemena\n", []string{"Angry", "Elemena"}},
		{"a comment after a name", "Angry # the tank\nElemena#heals", []string{"Angry", "Elemena"}},
		{"Windows line endings and stray blanks", "Angry\r\n\tDeathsong  \r\n \r\n", []string{"Angry", "Deathsong"}},
		{"a byte order mark", "\xef\xbb\xbfAngry\nDeathsong\n", []string{"Angry", "Deathsong"}},
		{"names outside ASCII", "Ångström\n", []string{"Ångström"}},
		{"only comments", "# nobody yet\n\n", nil},
		{"empty", "", nil},
	} {
		if got := ParseNames(tc.text); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.comment, got, tc.want)
		}
	}
}

// The selectors that fail before any query, so they need no database.
func TestSelectMembersWithoutNames(t *testing.T) {
	for comment, selector := range map[string]Selector{
		"nothing given":          {},
		"NamesOnly, no names":    {NamesOnly: true},
		"NamesOnly, leader only": {NamesOnly: true, Leader: "Deathsong"},
	} {
		_, err := selectMembers(context.Background(), nil, selector, &Roster{})
		var selection *SelectionError
		if !errors.As(err, &selection) {
			t.Errorf("%s: err = %v, want a SelectionError", comment, err)
		}
	}
}

func TestSelectionErrorKeepsItsText(t *testing.T) {
	err := error(&SelectionError{message: "Deathsong isn't in a group", cause: errNotInGroup})
	if err.Error() != "Deathsong isn't in a group" || !errors.Is(err, errNotInGroup) {
		t.Errorf("err = %q, not in a group %v", err, errors.Is(err, errNotInGroup))
	}
	if err := selectionErrorf("no character named %s", "Nobody, Noone"); err.Error() != "no character named Nobody, Noone" ||
		errors.Is(err, errNotInGroup) {
		t.Errorf("err = %q, not in a group %v", err, errors.Is(err, errNotInGroup))
	}
}

// fakeServer stands in for MySQL. It answers the selection queries from its characters and group,
// gives every other query an empty result, and like MySQL holds one open result set a connection.
// Names match by case, as characters.name does with its binary collation.
type fakeServer struct {
	characters  []member // guid and name
	group       []member // in the order the group query sorts them; empty when nobody is grouped
	groupLeader uint32
	openRows    int
	readOnly    bool
	queries     []string
}

func (s *fakeServer) Connect(context.Context) (driver.Conn, error) { return &fakeConn{s}, nil }
func (s *fakeServer) Driver() driver.Driver                        { return fakeDriver{} }

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("the fake server only opens through its connector")
}

type fakeConn struct{ server *fakeServer }

func (c *fakeConn) Close() error              { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return fakeTx{}, nil }
func (c *fakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("the fake server takes no prepared statements")
}

func (c *fakeConn) BeginTx(_ context.Context, options driver.TxOptions) (driver.Tx, error) {
	c.server.readOnly = options.ReadOnly
	return fakeTx{}, nil
}

type fakeTx struct{}

func (fakeTx) Commit() error   { return nil }
func (fakeTx) Rollback() error { return nil }

func (c *fakeConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	server := c.server
	if server.openRows > 0 {
		return nil, fmt.Errorf("busy buffer: a result set was still open when this ran: %s", query)
	}
	server.queries = append(server.queries, query)
	result := &fakeRows{server: server, columns: 1}
	add := func(values ...driver.Value) {
		result.columns = len(values)
		result.rows = append(result.rows, values)
	}
	byGUID := func(guid int64) (member, bool) {
		index := slices.IndexFunc(server.characters, func(m member) bool { return int64(m.guid) == guid })
		if index == -1 {
			return member{}, false
		}
		return server.characters[index], true
	}

	switch {
	case strings.HasPrefix(query, "SELECT guid FROM acore_characters.characters WHERE name = ?"):
		for _, character := range server.characters {
			if character.name == args[0].Value.(string) {
				add(int64(character.guid))
			}
		}
	case strings.HasPrefix(query, "SELECT g.guid, g.leaderGuid FROM acore_characters.group_member gm"):
		if slices.ContainsFunc(server.group, func(m member) bool { return int64(m.guid) == args[0].Value.(int64) }) {
			add(int64(1), int64(server.groupLeader))
		}
	case strings.HasPrefix(query, "SELECT gm.memberGuid, gm.subgroup, gm.memberFlags FROM acore_characters.group_member gm"):
		for _, grouped := range server.group {
			add(int64(grouped.guid), int64(grouped.subgroup), int64(grouped.memberFlags))
		}
	case strings.HasPrefix(query, "SELECT guid, name FROM acore_characters.characters WHERE name IN ("):
		for _, character := range server.characters {
			if slices.ContainsFunc(args, func(arg driver.NamedValue) bool { return arg.Value.(string) == character.name }) {
				add(int64(character.guid), character.name)
			}
		}
	case strings.HasPrefix(query, "SELECT c.guid, c.name, c.class, c.race, c.level, c.activeTalentGroup, c.ammoId,"):
		for _, arg := range args {
			if character, ok := byGUID(arg.Value.(int64)); ok {
				add(int64(character.guid), character.name, int64(classWarrior), int64(1), int64(MaxLevel), int64(0), int64(0), float64(0))
			}
		}
	}
	server.openRows++
	return result, nil
}

type fakeRows struct {
	server  *fakeServer
	columns int
	rows    [][]driver.Value
	closed  bool
}

func (r *fakeRows) Columns() []string { return make([]string, r.columns) }

func (r *fakeRows) Close() error {
	if !r.closed {
		r.closed = true
		r.server.openRows--
	}
	return nil
}

func (r *fakeRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}
	copy(dest, r.rows[0])
	r.rows = r.rows[1:]
	return nil
}

// coreServer is a raid of two full parties and a third of two, led by Deathsong, with Felesta and
// Agony outside it.
func coreServer() *fakeServer {
	server := &fakeServer{groupLeader: 2}
	for i, name := range []string{
		"Angry", "Deathsong", "Ecoterrorist", "Holylight", "Mighty",
		"Assasin", "Elemena", "Shadow", "Totemist", "Trueshot",
		"Hellflame", "Nightwarrior",
	} {
		grouped := member{guid: uint32(i + 1), name: name, subgroup: int32(i / 5)}
		if name == "Angry" {
			grouped.memberFlags = 2
		}
		server.characters = append(server.characters, grouped)
		server.group = append(server.group, grouped)
	}
	server.characters = append(server.characters, member{guid: 90, name: "Felesta"}, member{guid: 91, name: "Agony"})
	return server
}

// seated prints a roster's characters as "name@subgroup", with "*" on the ones holding a member flag.
func seated(roster *Roster) string {
	parts := make([]string, len(roster.Characters))
	for i, character := range roster.Characters {
		parts[i] = fmt.Sprintf("%s@%d", character.Name, character.Subgroup)
		if character.MemberFlags != 0 {
			parts[i] += "*"
		}
	}
	return strings.Join(parts, " ")
}

// coreRoster builds the roster in a read-only transaction, the way the sim server does.
func coreRoster(t *testing.T, server *fakeServer, selector Selector) (*Roster, error) {
	t.Helper()
	db := sql.OpenDB(server)
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if !server.readOnly {
		t.Fatal("the transaction didn't reach the server as read-only")
	}
	return BuildRosterContext(context.Background(), tx, selector, RosterInputs{DBC: &RosterDBC{}, MinSkill: DefaultMinSkill})
}

func TestBuildRosterNamesOnly(t *testing.T) {
	server := coreServer()
	// Mighty and Nightwarrior sit out, Felesta and Agony come in from outside the group
	names := []string{"Felesta", "Angry", "Deathsong", "Ecoterrorist", "Holylight", "Assasin", "Elemena", "Shadow",
		"Totemist", "Trueshot", "Agony", "Hellflame"}
	roster, err := coreRoster(t, server, Selector{Leader: "Deathsong", Names: names, NamesOnly: true})
	if err != nil {
		t.Fatal(err)
	}

	if want := "Angry@0* Deathsong@0 Ecoterrorist@0 Holylight@0 Felesta@0 " +
		"Assasin@1 Elemena@1 Shadow@1 Totemist@1 Trueshot@1 Hellflame@2 Agony@2"; seated(roster) != want {
		t.Errorf("seated %s\n  want %s", seated(roster), want)
	}
	group := roster.Group
	if group.Selector != "core" || group.Leader != "Deathsong" || group.LeaderIsGroupLeader == nil || !*group.LeaderIsGroupLeader ||
		!reflect.DeepEqual(group.Names, names) {
		t.Errorf("group = %+v", group)
	}
	if roster.Version != 2 {
		t.Errorf("roster version %d, want 2", roster.Version)
	}

	for _, want := range [][]string{{"Felesta", "Deathsong's group", "party 1"}, {"Agony", "Deathsong's group", "party 3"},
		{"left out", "Mighty, Nightwarrior"}} {
		warning := findWarning(roster.Warnings, want[0])
		for _, text := range want {
			if !strings.Contains(warning, text) {
				t.Errorf("no warning with %q; warnings: %q", want, roster.Warnings)
				break
			}
		}
	}
	if findWarning(roster.Warnings, "isn't the leader") != "" {
		t.Errorf("warned about the leader, who leads: %q", roster.Warnings)
	}
}

func TestBuildRosterNamesOnlyWithoutAGroup(t *testing.T) {
	names := []string{"Felesta", "Angry", "Deathsong", "Ecoterrorist", "Holylight", "Mighty", "Agony"}
	const want = "Felesta@0 Angry@0 Deathsong@0 Ecoterrorist@0 Holylight@0 Mighty@1 Agony@1"

	server := coreServer()
	server.group = nil
	roster, err := coreRoster(t, server, Selector{Leader: "Deathsong", Names: names, NamesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if seated(roster) != want {
		t.Errorf("leader in no group: seated %s\n  want %s", seated(roster), want)
	}
	if roster.Group.Selector != "core" || roster.Group.Leader != "Deathsong" || roster.Group.LeaderIsGroupLeader != nil {
		t.Errorf("leader in no group: group = %+v", roster.Group)
	}
	if warning := findWarning(roster.Warnings, "isn't in a group"); !strings.Contains(warning, "Deathsong") || !strings.Contains(warning, "list order") {
		t.Errorf("leader in no group: warnings %q", roster.Warnings)
	}

	// no leader given: the same seating, and nothing to warn about
	roster, err = coreRoster(t, coreServer(), Selector{Names: names, NamesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if seated(roster) != want || roster.Group.Selector != "core" || roster.Group.Leader != "" {
		t.Errorf("no leader: seated %s, group %+v", seated(roster), roster.Group)
	}
	if findWarning(roster.Warnings, "group") != "" {
		t.Errorf("no leader: warnings %q", roster.Warnings)
	}
}

func TestBuildRosterNamesOnlyFollowsAnyGroupMember(t *testing.T) {
	roster, err := coreRoster(t, coreServer(), Selector{Leader: "Trueshot", Names: []string{"Trueshot", "Angry"}, NamesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if seated(roster) != "Angry@0* Trueshot@1" {
		t.Errorf("seated %s", seated(roster))
	}
	if leads := roster.Group.LeaderIsGroupLeader; leads == nil || *leads || findWarning(roster.Warnings, "isn't the leader") == "" {
		t.Errorf("Trueshot doesn't lead the group: flag %v, warnings %q", leads, roster.Warnings)
	}
}

func TestBuildRosterSelectionErrors(t *testing.T) {
	for _, tc := range []struct {
		comment  string
		selector Selector
		want     string
	}{
		{"unknown leader, names only", Selector{Leader: "Nobody", Names: []string{"Angry"}, NamesOnly: true}, `no character named "Nobody"`},
		{"unknown names, names only", Selector{Leader: "Deathsong", Names: []string{"Angry", "Nobody", "Noone"}, NamesOnly: true},
			"no character named Nobody, Noone"},
		{"a name listed twice, names only", Selector{Leader: "Deathsong", Names: []string{"Angry", "angry"}, NamesOnly: true},
			"angry is listed twice"},
		{"unknown leader", Selector{Leader: "Nobody"}, `no character named "Nobody"`},
		{"a leader in the wrong case", Selector{Leader: "deathsong"}, `no character named "deathsong"`},
		{"a name in the wrong case", Selector{Names: []string{"Angry", "felesta"}}, "no character named felesta"},
		{"leader in no group", Selector{Leader: "Felesta"}, "Felesta isn't in a group"},
		{"unknown name", Selector{Names: []string{"Nobody"}}, "no character named Nobody"},
	} {
		_, err := coreRoster(t, coreServer(), tc.selector)
		var selection *SelectionError
		if !errors.As(err, &selection) || err.Error() != tc.want {
			t.Errorf("%s: err = %v, want the SelectionError %q", tc.comment, err, tc.want)
		}
	}
}

// The selectors acraid had before -namesOnly, on a pool as BuildRoster runs them.
func TestBuildRosterLeaderAndNames(t *testing.T) {
	server := coreServer()
	db := sql.OpenDB(server)
	defer db.Close()
	inputs := RosterInputs{DBC: &RosterDBC{}, MinSkill: DefaultMinSkill}

	roster, err := BuildRoster(db, Selector{Leader: "Deathsong", Names: []string{"Felesta", "Angry"}}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if want := "Angry@0* Deathsong@0 Ecoterrorist@0 Holylight@0 Mighty@0 Assasin@1 Elemena@1 Shadow@1 Totemist@1 Trueshot@1 " +
		"Hellflame@2 Nightwarrior@2 Felesta@2"; seated(roster) != want || roster.Group.Selector != "leader" {
		t.Errorf("the group topped up: %s, selector %q\n  want %s", seated(roster), roster.Group.Selector, want)
	}

	roster, err = BuildRoster(db, Selector{Names: []string{"Felesta", "Angry"}}, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if seated(roster) != "Felesta@0 Angry@0" || roster.Group.Selector != "names" {
		t.Errorf("names alone: %s, selector %q", seated(roster), roster.Group.Selector)
	}
}

// The roster tests above only show the loaders are safe in a transaction if the fake really turns a
// second open result set down.
func TestFakeServerHoldsOneResultSet(t *testing.T) {
	db := sql.OpenDB(coreServer())
	defer db.Close()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	const everyone = "SELECT gm.memberGuid, gm.subgroup, gm.memberFlags FROM acore_characters.group_member gm"
	rows, err := tx.QueryContext(context.Background(), everyone)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.QueryContext(context.Background(), everyone); err == nil {
		t.Error("a second query ran while the first result set was open")
	}
	rows.Close()
	if again, err := tx.QueryContext(context.Background(), everyone); err != nil {
		t.Errorf("after closing the first result set: %v", err)
	} else {
		again.Close()
	}
}
