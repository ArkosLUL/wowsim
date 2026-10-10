package acroster

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// testDSN points at a port nothing listens on. Its password is the string no log or answer may hold.
const (
	testPassword = "s3cr3t-pass"
	testDSN      = "sim:" + testPassword + "@tcp(127.0.0.1:1)/"
)

// writeDBC writes a WDBC file of fieldCount 4-byte fields, each row given as field index -> value.
func writeDBC(t *testing.T, path string, fieldCount int, rows ...map[int]uint32) {
	t.Helper()
	var file bytes.Buffer
	file.WriteString("WDBC")
	for _, value := range []uint32{uint32(len(rows)), uint32(fieldCount), uint32(fieldCount * 4), 1} {
		binary.Write(&file, binary.LittleEndian, value)
	}
	for _, row := range rows {
		record := make([]uint32, fieldCount)
		for field, value := range row {
			record[field] = value
		}
		binary.Write(&file, binary.LittleEndian, record)
	}
	file.WriteByte(0)
	if err := os.WriteFile(path, file.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeDBCDir writes the six files the roster needs, with the client's field counts. The one glyph
// is id 170 with the given spell.
func writeDBCDir(t *testing.T, glyphSpell uint32) string {
	t.Helper()
	dir := t.TempDir()
	writeDBC(t, filepath.Join(dir, "Talent.dbc"), 23)
	writeDBC(t, filepath.Join(dir, "TalentTab.dbc"), 24)
	writeDBC(t, filepath.Join(dir, "GlyphProperties.dbc"), 4, map[int]uint32{0: 170, 1: glyphSpell})
	writeDBC(t, filepath.Join(dir, "SpellItemEnchantment.dbc"), 38, map[int]uint32{0: 3563, 33: 40111})
	writeDBC(t, filepath.Join(dir, "CreatureFamily.dbc"), 28)
	writeDBC(t, filepath.Join(dir, "Spell.dbc"), 234)
	return dir
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func itemDB(json string) fstest.MapFS {
	return fstest.MapFS{itemDBPath: {Data: []byte(json)}}
}

const oneItem = `{"items": [{"id": 47112, "stats": [0, 0, 55], "serverStats": [{"statType": 7, "value": 55}]}]}`

// workingConfig has everything a build reads but a database that answers.
func workingConfig(t *testing.T) Config {
	t.Helper()
	return Config{
		DSN:             testDSN,
		DBCDir:          writeDBCDir(t, 54733),
		NamesFile:       writeFile(t, "# party 1\nAngry\nDeathsong\n"),
		Leader:          "Deathsong",
		WorldBuffMatrix: writeFile(t, `AC_AI_PLAYERBOT_WORLD_BUFF_MATRIX="1:0,3,0,80,80:53760,57371"`),
		Assets:          itemDB(oneItem),
	}
}

// wantFailure checks err is the *Error for that status and code, and returns its message.
func wantFailure(t *testing.T, comment string, err error, status int, code string) string {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) {
		t.Fatalf("%s: err = %v, want an *Error", comment, err)
	}
	if failure.Status != status || failure.Code != code {
		t.Errorf("%s: %d %s (%s), want %d %s", comment, failure.Status, failure.Code, failure.Message, status, code)
	}
	return failure.Message
}

func TestDBCCacheReloadsWhenAFileChanges(t *testing.T) {
	cfg := workingConfig(t)
	b := newBuilder(cfg)

	first, err := b.loadDBC()
	if err != nil {
		t.Fatal(err)
	}
	if first.Glyphs[170].SpellID != 54733 || first.GemItems[3563] != 40111 {
		t.Fatalf("the DBC files read as %+v", first)
	}
	if again, _ := b.loadDBC(); again != first {
		t.Error("untouched files were parsed again")
	}

	// same size, so only the time tells the cache the file was rewritten
	glyphs := filepath.Join(cfg.DBCDir, "GlyphProperties.dbc")
	writeDBC(t, glyphs, 4, map[int]uint32{0: 170, 1: 58386})
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(glyphs, later, later); err != nil {
		t.Fatal(err)
	}
	reloaded, err := b.loadDBC()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded == first || reloaded.Glyphs[170].SpellID != 58386 {
		t.Errorf("after os.Chtimes the glyph's spell is %d, want the rewritten file's 58386", reloaded.Glyphs[170].SpellID)
	}
	if again, _ := b.loadDBC(); again != reloaded {
		t.Error("the reloaded files were parsed again")
	}

	if err := os.Remove(filepath.Join(cfg.DBCDir, "Spell.dbc")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.loadDBC(); err == nil {
		t.Error("a directory without Spell.dbc still gives the cached tables")
	}
}

func TestItemStatsFromTheServedFiles(t *testing.T) {
	assets := itemDB(oneItem)
	b := newBuilder(Config{Assets: assets})

	itemStats, err := b.loadItemStats()
	if err != nil {
		t.Fatal(err)
	}
	item, ok := itemStats(47112)
	if !ok || item.Stats[2] != 55 || len(item.ServerStats) != 1 {
		t.Fatalf("item 47112 = %+v, found %v", item, ok)
	}

	// same size and time, so the cached stats stand
	assets[itemDBPath] = &fstest.MapFile{Data: []byte(strings.Replace(oneItem, "47112", "47113", 1))}
	if itemStats, _ = b.loadItemStats(); itemStats == nil {
		t.Fatal("no item stats")
	}
	if _, ok := itemStats(47113); ok {
		t.Error("an unchanged file was parsed again")
	}

	assets[itemDBPath].ModTime = time.Now()
	if itemStats, err = b.loadItemStats(); err != nil {
		t.Fatal(err)
	}
	if _, ok := itemStats(47113); !ok {
		t.Error("a file with a new time wasn't parsed again")
	}

	for comment, broken := range map[string]*builder{
		"no item database":     newBuilder(Config{Assets: fstest.MapFS{}}),
		"broken item database": newBuilder(Config{Assets: itemDB(`{"items": [`)}),
		"no files at all":      newBuilder(Config{}),
	} {
		if _, err := broken.loadItemStats(); err == nil {
			t.Errorf("%s: loaded", comment)
		}
	}
}

func TestEmbeddedTreesLoad(t *testing.T) {
	b := newBuilder(Config{})
	trees, petTrees, err := b.loadTrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(trees) != len(azerothcore.ClassNames) || len(petTrees) != 3 {
		t.Fatalf("%d class trees and %d pet trees", len(trees), len(petTrees))
	}
	for classID, name := range azerothcore.ClassNames {
		if classTrees := trees[classID]; len(classTrees) != 3 || len(classTrees[0]) == 0 {
			t.Errorf("%s has %d trees", name, len(classTrees))
		}
	}
	for talentType, tree := range petTrees {
		if len(tree) == 0 {
			t.Errorf("pet tree %d is empty", talentType)
		}
	}

	again, _, _ := b.loadTrees()
	if reflect.ValueOf(again).Pointer() != reflect.ValueOf(trees).Pointer() {
		t.Error("the embedded trees were parsed twice")
	}
}

func TestInputs(t *testing.T) {
	cfg := workingConfig(t)
	selector, inputs, err := newBuilder(cfg).inputs()
	if err != nil {
		t.Fatal(err)
	}
	if want := (azerothcore.Selector{Leader: "Deathsong", Names: []string{"Angry", "Deathsong"}, NamesOnly: true}); !reflect.DeepEqual(selector, want) {
		t.Errorf("selector = %+v, want %+v", selector, want)
	}
	if inputs.MinSkill != azerothcore.DefaultMinSkill || inputs.Players != nil {
		t.Errorf("min skill %d, players %v: not what acraid defaults to", inputs.MinSkill, inputs.Players)
	}
	if inputs.DBC == nil || inputs.Trees == nil || inputs.PetTrees == nil || inputs.ItemStats == nil || len(inputs.WorldBuffs) != 2 {
		t.Errorf("inputs = %+v", inputs)
	}

	// the names file is read on every build
	b := newBuilder(cfg)
	if err := os.WriteFile(cfg.NamesFile, []byte("Felesta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if selector, _, err = b.inputs(); err != nil || !reflect.DeepEqual(selector.Names, []string{"Felesta"}) {
		t.Errorf("after an edit the names are %v (%v)", selector.Names, err)
	}

	noMatrix := cfg
	noMatrix.WorldBuffMatrix = ""
	if _, inputs, err = newBuilder(noMatrix).inputs(); err != nil || inputs.WorldBuffs != nil {
		t.Errorf("without a matrix: %v, world buffs %v", err, inputs.WorldBuffs)
	}
}

func TestInputsFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	for _, tc := range []struct {
		comment string
		change  func(*Config)
		status  int
		code    string
		mention string
	}{
		{"names file missing", func(cfg *Config) { cfg.NamesFile = missing }, http.StatusInternalServerError, "server_config", "names file"},
		{"names file empty", func(cfg *Config) { cfg.NamesFile = writeFile(t, "# nobody\n\n") }, http.StatusConflict, "no_roster", "lists no characters"},
		{"matrix missing", func(cfg *Config) { cfg.WorldBuffMatrix = missing }, http.StatusInternalServerError, "server_config", "world-buff matrix"},
		{"matrix without entries", func(cfg *Config) { cfg.WorldBuffMatrix = writeFile(t, "# nothing\n") }, http.StatusInternalServerError, "server_config", "world-buff matrix"},
		{"DBC directory missing", func(cfg *Config) { cfg.DBCDir = missing }, http.StatusInternalServerError, "server_config", "DBC"},
		{"DBC from another client", func(cfg *Config) { writeDBC(t, filepath.Join(cfg.DBCDir, "Spell.dbc"), 100) }, http.StatusInternalServerError, "server_config", "Spell.dbc"},
		{"item database missing", func(cfg *Config) { cfg.Assets = fstest.MapFS{} }, http.StatusInternalServerError, "server_config", "item database"},
	} {
		cfg := workingConfig(t)
		tc.change(&cfg)
		_, _, err := newBuilder(cfg).inputs()
		if message := wantFailure(t, tc.comment, err, tc.status, tc.code); !strings.Contains(message, tc.mention) {
			t.Errorf("%s: %q doesn't mention %q", tc.comment, message, tc.mention)
		}
	}
}

func TestDriverConfig(t *testing.T) {
	cfg, err := driverConfig(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Timeout != 5*time.Second || cfg.ReadTimeout != 20*time.Second || cfg.Passwd != testPassword {
		t.Errorf("timeout %s, read timeout %s", cfg.Timeout, cfg.ReadTimeout)
	}

	cfg, err = driverConfig(testDSN + "?timeout=1s&readTimeout=2s")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Timeout != time.Second || cfg.ReadTimeout != 2*time.Second {
		t.Errorf("the DSN's own timeouts became %s and %s", cfg.Timeout, cfg.ReadTimeout)
	}

	for _, dsn := range []string{
		"sim:" + testPassword + "@tcp(127.0.0.1:3306",
		"sim:" + testPassword + "@127.0.0.1:3306/",
		"sim:" + testPassword + "@tcp(127.0.0.1:3306)/?timeout=" + testPassword,
	} {
		_, err := driverConfig(dsn)
		if message := wantFailure(t, dsn, err, http.StatusInternalServerError, "server_config"); strings.Contains(message, testPassword) {
			t.Errorf("the error for a bad DSN quotes it: %q", message)
		}
	}
}

func TestBuildFailsOnAnUnreachableDatabase(t *testing.T) {
	_, err := newBuilder(workingConfig(t)).build(context.Background())
	if err == nil {
		t.Fatal("built a roster without a database")
	}
	var failure *Error
	var selection *azerothcore.SelectionError
	if errors.As(err, &failure) || errors.As(err, &selection) {
		t.Errorf("err = %v, want a plain error, which the route answers as the database failing", err)
	}
	if strings.Contains(err.Error(), testPassword) {
		t.Errorf("the error quotes the password: %v", err)
	}
}

func TestWithEnv(t *testing.T) {
	env := map[string]string{
		"AC_DSN": "env-dsn", "AC_DBC_DIR": "env-dbc", "AC_RAID_NAMES_FILE": "env-names", "AC_RAID_LEADER": "env-leader",
		"AC_WORLD_BUFF_MATRIX": "env-matrix",
	}
	getenv := func(name string) string { return env[name] }

	got := Config{}.WithEnv(getenv)
	if want := (Config{DSN: "env-dsn", DBCDir: "env-dbc", NamesFile: "env-names", Leader: "env-leader", WorldBuffMatrix: "env-matrix"}); !reflect.DeepEqual(got, want) {
		t.Errorf("from the environment alone: %+v, want %+v", got, want)
	}

	flags := Config{DSN: "flag-dsn", DBCDir: "flag-dbc", NamesFile: "flag-names", Leader: "flag-leader", WorldBuffMatrix: "flag-matrix", Timeout: time.Second}
	if got := flags.WithEnv(getenv); !reflect.DeepEqual(got, flags) {
		t.Errorf("flags lost to the environment: %+v", got)
	}

	if got := (Config{DSN: "flag-dsn"}).WithEnv(func(string) string { return "" }); len(got.missingFlags()) != 2 {
		t.Errorf("missing flags = %v, want the DBC directory and the names file", got.missingFlags())
	}
}

// oneCharacterServer is a database holding the warrior Angry and nothing else. It records how a
// build used it.
type oneCharacterServer struct {
	readOnly   bool
	inTx       bool
	closed     bool
	queries    int
	outsideTx  int
	afterClose int
}

func (s *oneCharacterServer) Connect(context.Context) (driver.Conn, error) {
	return &oneCharacterConn{s}, nil
}
func (s *oneCharacterServer) Driver() driver.Driver { return nil }

type oneCharacterConn struct{ server *oneCharacterServer }

func (c *oneCharacterConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}
func (c *oneCharacterConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("this server takes no prepared statements")
}

func (c *oneCharacterConn) Close() error {
	c.server.closed = true
	return nil
}

func (c *oneCharacterConn) BeginTx(_ context.Context, options driver.TxOptions) (driver.Tx, error) {
	c.server.readOnly, c.server.inTx = options.ReadOnly, true
	return oneCharacterTx{c.server}, nil
}

type oneCharacterTx struct{ server *oneCharacterServer }

func (tx oneCharacterTx) Commit() error   { return errors.New("a build has nothing to commit") }
func (tx oneCharacterTx) Rollback() error { tx.server.inTx = false; return nil }

func (c *oneCharacterConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.server.queries++
	if !c.server.inTx {
		c.server.outsideTx++
	}
	if c.server.closed {
		c.server.afterClose++
	}
	named := func() bool {
		for _, arg := range args {
			if name, ok := arg.Value.(string); ok && strings.EqualFold(name, "Angry") {
				return true
			}
		}
		return false
	}
	switch {
	case strings.HasPrefix(query, "SELECT guid, name FROM acore_characters.characters WHERE name IN (") && named():
		return &oneRow{values: []driver.Value{int64(1), "Angry"}}, nil
	case strings.HasPrefix(query, "SELECT c.guid, c.name, c.class, c.race, c.level, c.activeTalentGroup, c.ammoId,"):
		return &oneRow{values: []driver.Value{int64(1), "Angry", int64(1), int64(1), int64(80), int64(0), int64(0), float64(0)}}, nil
	}
	return &oneRow{}, nil
}

// oneRow is a result set of one row, or none when it has no values.
type oneRow struct {
	values []driver.Value
	read   bool
}

func (r *oneRow) Columns() []string { return make([]string, max(len(r.values), 1)) }
func (r *oneRow) Close() error      { return nil }

func (r *oneRow) Next(dest []driver.Value) error {
	if r.read || r.values == nil {
		return io.EOF
	}
	r.read = true
	copy(dest, r.values)
	return nil
}

// builderOn is a builder whose database is that server.
func builderOn(cfg Config, server *oneCharacterServer) *builder {
	b := newBuilder(cfg)
	b.open = func(string) (*sql.DB, error) { return sql.OpenDB(server), nil }
	return b
}

func TestBuildReadsInOneReadOnlyTransaction(t *testing.T) {
	cfg := workingConfig(t)
	cfg.Leader = ""
	cfg.NamesFile = writeFile(t, "Angry\n")
	server := &oneCharacterServer{}

	data, err := builderOn(cfg, server).build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !server.readOnly || server.outsideTx != 0 || server.queries == 0 {
		t.Errorf("read-only %v, %d of %d queries outside the transaction", server.readOnly, server.outsideTx, server.queries)
	}
	if server.inTx || !server.closed || server.afterClose != 0 {
		t.Errorf("after the build: transaction open %v, connection closed %v, %d queries after closing", server.inTx, server.closed, server.afterClose)
	}

	var roster azerothcore.Roster
	if err := json.Unmarshal(data, &roster); err != nil {
		t.Fatalf("the build's output isn't a roster: %v\n%s", err, data)
	}
	if roster.Version != azerothcore.RosterVersion || roster.Group.Selector != "core" || len(roster.Characters) != 1 ||
		roster.Characters[0].Name != "Angry" {
		t.Errorf("roster = %s", data)
	}
	// acraid writes Roster.JSON to its file, so reading the output back has to give the same bytes
	if again, err := roster.JSON(); err != nil || !bytes.Equal(again, data) {
		t.Errorf("the build's output isn't what acraid writes (%v):\n%s\nwant:\n%s", err, data, again)
	}
}

func TestBuildPassesSelectionErrorsOn(t *testing.T) {
	// the leader is Deathsong, whom the server doesn't have
	server := &oneCharacterServer{}
	_, err := builderOn(workingConfig(t), server).build(context.Background())
	var selection *azerothcore.SelectionError
	if !errors.As(err, &selection) || !strings.Contains(err.Error(), "Deathsong") {
		t.Errorf("err = %v, want the SelectionError naming Deathsong", err)
	}
	if !server.closed || server.inTx {
		t.Errorf("after a failed build: connection closed %v, transaction open %v", server.closed, server.inTx)
	}
}
