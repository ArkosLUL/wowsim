// Package acroster serves the core raid's roster straight off the AzerothCore server, as the JSON
// tools/database/acraid writes. It only reads from the server.
package acroster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// Config is what the route reads the server with. DSN, DBCDir and NamesFile are required, and without
// them every request is answered 503.
type Config struct {
	// DSN is a MySQL DSN without a database name, since the queries name their databases.
	DSN string
	// DBCDir holds the server's DBC files.
	DBCDir string
	// NamesFile lists the core raid, one character a line.
	NamesFile string
	// Leader is any member of the raid's group, whose subgroups the roster follows. Without one the
	// raid is seated in the names file's order.
	Leader string
	// WorldBuffMatrix is the file holding the bots' AiPlayerbot.WorldBuffMatrix, e.g. Playerbot.env.
	WorldBuffMatrix string
	// Assets is the file system the server serves the UI from, which has the item database in it.
	Assets fs.FS
	// Timeout is how long a request gets, its wait for the one before it included. 0 means 30 s.
	Timeout time.Duration
}

const defaultTimeout = 30 * time.Second

// WithEnv fills the settings still empty from the environment.
func (cfg Config) WithEnv(getenv func(string) string) Config {
	for _, setting := range []struct {
		value *string
		env   string
	}{
		{&cfg.DSN, "AC_DSN"},
		{&cfg.DBCDir, "AC_DBC_DIR"},
		{&cfg.NamesFile, "AC_RAID_NAMES_FILE"},
		{&cfg.Leader, "AC_RAID_LEADER"},
		{&cfg.WorldBuffMatrix, "AC_WORLD_BUFF_MATRIX"},
	} {
		if *setting.value == "" {
			*setting.value = getenv(setting.env)
		}
	}
	return cfg
}

// missingFlags names the required settings that aren't set.
func (cfg Config) missingFlags() []string {
	var missing []string
	for _, setting := range []struct{ value, flag string }{
		{cfg.DSN, "--acoreDsn"}, {cfg.DBCDir, "--acoreDbcDir"}, {cfg.NamesFile, "--acoreNamesFile"},
	} {
		if setting.value == "" {
			missing = append(missing, setting.flag)
		}
	}
	return missing
}

// BuildFunc builds the roster's JSON. An *Error or an *azerothcore.SelectionError picks the answer;
// any other error counts as the database failing.
type BuildFunc func(ctx context.Context) ([]byte, error)

// Error is a failure with the status and code the route answers it with.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// New serves the roster built from the live server.
func New(cfg Config) http.Handler {
	if missing := cfg.missingFlags(); len(missing) > 0 {
		log.Printf("/acoreRoster is off: no %s", strings.Join(missing, ", "))
	} else {
		log.Printf("/acoreRoster imports the characters in %s", cfg.NamesFile)
	}
	return NewWithBuilder(cfg, newBuilder(cfg).build)
}

// NewWithBuilder is New with the roster coming from build, which never runs twice at once.
func NewWithBuilder(cfg Config, build BuildFunc) http.Handler {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &handler{
		missing: cfg.missingFlags(),
		dsn:     cfg.DSN,
		timeout: timeout,
		build:   build,
		slot:    make(chan struct{}, 1),
	}
}

type handler struct {
	missing []string
	dsn     string
	timeout time.Duration
	build   BuildFunc
	// slot holds a token while a build runs, so only one reads the database at a time
	slot chan struct{}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case r.Method != http.MethodGet:
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, &Error{http.StatusMethodNotAllowed, "method_not_allowed", "/acoreRoster only answers GET"})
		return
	// another site's page can't read the answer, but it could still keep the database busy
	case strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site"):
		writeError(w, &Error{http.StatusForbidden, "cross_site", "/acoreRoster doesn't answer other sites"})
		return
	case r.URL.RawQuery != "":
		writeError(w, &Error{http.StatusBadRequest, "bad_request", "/acoreRoster takes no parameters"})
		return
	case len(h.missing) > 0:
		writeError(w, &Error{http.StatusServiceUnavailable, "not_configured",
			"This sim server isn't set up to import from AzerothCore: it was started without " + strings.Join(h.missing, ", ")})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	select {
	case h.slot <- struct{}{}:
	case <-ctx.Done():
		h.fail(w, ctx.Err())
		return
	}

	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	// the build gets its own goroutine so a stuck one can't hold the answer back, and it keeps the
	// slot until it really returns
	go func() {
		defer func() { <-h.slot }()
		// a panic here would otherwise take the whole sim server down
		defer func() {
			if crash := recover(); crash != nil {
				log.Printf("/acoreRoster build panicked: %s\n%s", redact(fmt.Sprint(crash), h.dsn), debug.Stack())
				done <- result{err: &Error{http.StatusInternalServerError, "server_config",
					"The roster build crashed. The sim server's log says where."}}
			}
		}()
		data, err := h.build(ctx)
		done <- result{data, err}
	}()

	select {
	case built := <-done:
		switch {
		case built.err == nil:
			w.Header().Set("Content-Type", "application/json")
			w.Write(built.data)
		case ctx.Err() != nil:
			h.fail(w, ctx.Err())
		default:
			h.fail(w, built.err)
		}
	case <-ctx.Done():
		h.fail(w, ctx.Err())
	}
}

// fail answers with the status the error stands for, and logs what the answer leaves out.
func (h *handler) fail(w http.ResponseWriter, err error) {
	var failure *Error
	var selection *azerothcore.SelectionError
	switch {
	case errors.As(err, &failure):
	case errors.As(err, &selection):
		failure = &Error{http.StatusConflict, "no_roster", selection.Error()}
	// Canceled is the browser going away, where nobody reads the answer
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		failure = &Error{http.StatusGatewayTimeout, "timeout",
			fmt.Sprintf("The AzerothCore import didn't finish within %s", h.timeout)}
	default:
		failure = &Error{http.StatusBadGateway, "database",
			"The AzerothCore database didn't answer. The sim server's log says why."}
	}
	if failure.Status >= http.StatusInternalServerError {
		log.Printf("/acoreRoster %d %s: %s", failure.Status, failure.Code, redact(err.Error(), h.dsn))
	}
	writeError(w, failure)
}

// redact keeps the DSN, password and all, out of a log line.
func redact(text, dsn string) string {
	if dsn == "" {
		return text
	}
	return strings.ReplaceAll(text, dsn, "<dsn>")
}

func writeError(w http.ResponseWriter, failure *Error) {
	body, _ := json.Marshal(struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}{failure.Code, failure.Message})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(failure.Status)
	w.Write(body)
}
