package acroster

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
	"github.com/wowsims/wotlk/ui/core/talents/trees"
)

// itemDBPath is the item database inside Config.Assets.
const itemDBPath = "wotlk/assets/database/db.json"

// What the database gets when the DSN sets no timeout or readTimeout of its own.
const (
	dialTimeout = 5 * time.Second
	readTimeout = 20 * time.Second
)

// builder builds the roster off the live server. It reads the names file and the world-buff matrix
// on every build, and keeps the parsed talent trees, DBC files and item stats between builds.
type builder struct {
	cfg Config
	// open connects to the database for one build
	open func(dsn string) (*sql.DB, error)

	treesOnce sync.Once
	trees     azerothcore.TalentTrees
	petTrees  azerothcore.PetTalentTrees
	treesErr  error

	// mu guards the two caches below, each kept until its files' sizes or times change
	mu             sync.Mutex
	dbc            *azerothcore.RosterDBC
	dbcStamp       string
	itemStats      azerothcore.ItemStats
	itemStatsStamp string
}

func newBuilder(cfg Config) *builder {
	return &builder{cfg: cfg, open: openDB}
}

func configError(format string, args ...any) error {
	return &Error{http.StatusInternalServerError, "server_config", fmt.Sprintf(format, args...)}
}

func (b *builder) build(ctx context.Context) ([]byte, error) {
	selector, inputs, err := b.inputs()
	if err != nil {
		return nil, err
	}

	db, err := b.open(b.cfg.DSN)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	// one read-only transaction, so every query sees the same moment and none can write
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	roster, err := azerothcore.BuildRosterContext(ctx, tx, selector, inputs)
	if err != nil {
		return nil, err
	}
	return roster.JSON()
}

// inputs is everything a build reads besides the database.
func (b *builder) inputs() (azerothcore.Selector, azerothcore.RosterInputs, error) {
	selector := azerothcore.Selector{Leader: b.cfg.Leader, NamesOnly: true}
	inputs := azerothcore.RosterInputs{MinSkill: azerothcore.DefaultMinSkill}

	names, err := azerothcore.ReadNamesFile(b.cfg.NamesFile)
	if err != nil {
		return selector, inputs, configError("Can't read the names file: %v", err)
	}
	if len(names) == 0 {
		return selector, inputs, &Error{http.StatusConflict, "no_roster",
			fmt.Sprintf("The names file %s lists no characters", b.cfg.NamesFile)}
	}
	selector.Names = names

	if b.cfg.WorldBuffMatrix != "" {
		var problems []string
		inputs.WorldBuffs, problems, err = azerothcore.ReadWorldBuffMatrix(b.cfg.WorldBuffMatrix)
		if err != nil {
			return selector, inputs, configError("Can't read the world-buff matrix: %v", err)
		}
		for _, problem := range problems {
			log.Printf("/acoreRoster world-buff matrix: %s, skipped as mod-playerbots skips it", problem)
		}
	}

	if inputs.DBC, err = b.loadDBC(); err != nil {
		return selector, inputs, configError("Can't read the DBC files in %s: %v", b.cfg.DBCDir, err)
	}
	if inputs.Trees, inputs.PetTrees, err = b.loadTrees(); err != nil {
		return selector, inputs, configError("Can't read the talent trees: %v", err)
	}
	if inputs.ItemStats, err = b.loadItemStats(); err != nil {
		return selector, inputs, configError("Can't read the item database: %v", err)
	}
	return selector, inputs, nil
}

func (b *builder) loadTrees() (azerothcore.TalentTrees, azerothcore.PetTalentTrees, error) {
	b.treesOnce.Do(func() {
		if b.trees, b.treesErr = azerothcore.LoadTalentTreesFS(trees.FS); b.treesErr == nil {
			b.petTrees, b.treesErr = azerothcore.LoadPetTalentTreesFS(trees.FS)
		}
	})
	return b.trees, b.petTrees, b.treesErr
}

func (b *builder) loadDBC() (*azerothcore.RosterDBC, error) {
	stamp := ""
	for _, name := range azerothcore.RosterDBCFileNames() {
		info, err := os.Stat(filepath.Join(b.cfg.DBCDir, name))
		if err != nil {
			return nil, err
		}
		stamp += fileStamp(info)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dbc == nil || stamp != b.dbcStamp {
		dbc, err := azerothcore.LoadRosterDBC(b.cfg.DBCDir)
		if err != nil {
			return nil, err
		}
		b.dbc, b.dbcStamp = dbc, stamp
	}
	return b.dbc, nil
}

func (b *builder) loadItemStats() (azerothcore.ItemStats, error) {
	if b.cfg.Assets == nil {
		return nil, fmt.Errorf("the server has no files to read %s from", itemDBPath)
	}
	info, err := fs.Stat(b.cfg.Assets, itemDBPath)
	if err != nil {
		return nil, err
	}
	stamp := fileStamp(info)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.itemStats == nil || stamp != b.itemStatsStamp {
		itemStats, err := azerothcore.SimItemStatsFS(b.cfg.Assets, itemDBPath)
		if err != nil {
			return nil, err
		}
		b.itemStats, b.itemStatsStamp = itemStats, stamp
	}
	return b.itemStats, nil
}

// fileStamp changes when the file is replaced or rewritten. It's taken before the file is read, so
// a write in between only costs another parse on the next build.
func fileStamp(info fs.FileInfo) string {
	// not UnixNano, which is undefined for the zero time embedded files carry
	return fmt.Sprintf("%s %d %d.%d;", info.Name(), info.Size(), info.ModTime().Unix(), info.ModTime().Nanosecond())
}

// openDB opens a pool for one build.
func openDB(dsn string) (*sql.DB, error) {
	cfg, err := driverConfig(dsn)
	if err != nil {
		return nil, err
	}
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, configError("--acoreDsn has settings the MySQL driver turns down")
	}
	return sql.OpenDB(connector), nil
}

// driverConfig parses the DSN and gives it the default timeouts. Its error leaves the driver's own
// out, which can quote parts of the DSN, password included.
func driverConfig(dsn string) (*mysql.Config, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, configError("--acoreDsn isn't a MySQL DSN like user:password@tcp(host:3306)/")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = dialTimeout
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = readTimeout
	}
	return cfg, nil
}
