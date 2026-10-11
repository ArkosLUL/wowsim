package acroster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// setUp is a config with the three required settings, none of which a test builder reads.
var setUp = Config{DSN: testDSN, DBCDir: "/dbc", NamesFile: "/names.txt"}

// rosterBytes stands for acraid's output: the route has to pass it on byte for byte.
var rosterBytes = []byte("{\n  \"version\": 2,\n  \"characters\": [\"Ångström\"]\n}\n")

func builds(data []byte, err error) BuildFunc {
	return func(context.Context) ([]byte, error) { return data, err }
}

func serve(handler http.Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func get(handler http.Handler) *httptest.ResponseRecorder {
	return serve(handler, httptest.NewRequest(http.MethodGet, "/acoreRoster", nil))
}

// captureLog collects what the handler logs until the test ends.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	logged := &bytes.Buffer{}
	log.SetOutput(logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return logged
}

// wantError checks the status and the {"code", "error"} body, and returns the error text.
func wantError(t *testing.T, comment string, response *httptest.ResponseRecorder, status int, code string) string {
	t.Helper()
	var body struct {
		Code  string  `json:"code"`
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Error == nil {
		t.Fatalf("%s: the body isn't {\"code\", \"error\"}: %q", comment, response.Body)
	}
	if response.Code != status || body.Code != code {
		t.Errorf("%s: %d %s (%s), want %d %s", comment, response.Code, body.Code, *body.Error, status, code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("%s: Content-Type %q", comment, contentType)
	}
	return *body.Error
}

// selectionError is a real one from the library, which fails on an empty selector before it queries.
func selectionError(t *testing.T) error {
	t.Helper()
	_, err := azerothcore.BuildRosterContext(context.Background(), nil, azerothcore.Selector{}, azerothcore.RosterInputs{})
	var selection *azerothcore.SelectionError
	if !errors.As(err, &selection) {
		t.Fatalf("an empty selector fails with %v, want a SelectionError", err)
	}
	return err
}

func TestRosterIsPassedOnByteForByte(t *testing.T) {
	response := get(NewWithBuilder(setUp, builds(rosterBytes, nil)))
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), rosterBytes) {
		t.Errorf("%d %q, want 200 and the builder's bytes", response.Code, response.Body)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Content-Type %q", contentType)
	}
}

func TestEveryAnswerIsUncachedAndSameOriginOnly(t *testing.T) {
	handler := NewWithBuilder(setUp, builds(rosterBytes, nil))
	post := httptest.NewRequest(http.MethodPost, "/acoreRoster", nil)
	for comment, response := range map[string]*httptest.ResponseRecorder{
		"200": get(handler),
		"405": serve(handler, post),
		"503": get(NewWithBuilder(Config{}, builds(rosterBytes, nil))),
		"502": get(NewWithBuilder(setUp, builds(nil, errors.New("connection refused")))),
	} {
		if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
			t.Errorf("%s: Cache-Control %q, want no-store", comment, cache)
		}
		for header := range response.Header() {
			if strings.HasPrefix(header, "Access-Control-") {
				t.Errorf("%s: answers with the CORS header %s", comment, header)
			}
		}
	}
}

func TestRequestsTheRouteTurnsDown(t *testing.T) {
	for _, tc := range []struct {
		comment string
		request func() *http.Request
		status  int
		code    string
	}{
		{"a query parameter", func() *http.Request {
			return httptest.NewRequest(http.MethodGet, "/acoreRoster?leader=Deathsong", nil)
		}, http.StatusBadRequest, "bad_request"},
		{"a parameter without a value", func() *http.Request {
			return httptest.NewRequest(http.MethodGet, "/acoreRoster?saveall", nil)
		}, http.StatusBadRequest, "bad_request"},
		{"another site", func() *http.Request {
			request := httptest.NewRequest(http.MethodGet, "/acoreRoster", nil)
			request.Header.Set("Sec-Fetch-Site", "cross-site")
			return request
		}, http.StatusForbidden, "cross_site"},
		{"POST", func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/acoreRoster", strings.NewReader("{}"))
		}, http.StatusMethodNotAllowed, "method_not_allowed"},
		{"HEAD", func() *http.Request {
			return httptest.NewRequest(http.MethodHead, "/acoreRoster", nil)
		}, http.StatusMethodNotAllowed, "method_not_allowed"},
	} {
		built := false
		handler := NewWithBuilder(setUp, func(context.Context) ([]byte, error) {
			built = true
			return rosterBytes, nil
		})
		response := serve(handler, tc.request())
		wantError(t, tc.comment, response, tc.status, tc.code)
		if built {
			t.Errorf("%s: the roster was built anyway", tc.comment)
		}
		if tc.status == http.StatusMethodNotAllowed && response.Header().Get("Allow") != http.MethodGet {
			t.Errorf("%s: Allow %q, want GET", tc.comment, response.Header().Get("Allow"))
		}
	}
}

func TestRequestsFromTheSimsOwnPagePass(t *testing.T) {
	handler := NewWithBuilder(setUp, builds(rosterBytes, nil))
	for _, site := range []string{"", "same-origin", "same-site", "none"} {
		request := httptest.NewRequest(http.MethodGet, "/acoreRoster", nil)
		if site != "" {
			request.Header.Set("Sec-Fetch-Site", site)
		}
		if response := serve(handler, request); response.Code != http.StatusOK {
			t.Errorf("Sec-Fetch-Site %q: %d, want 200", site, response.Code)
		}
	}
}

func TestNotConfiguredNamesTheMissingFlags(t *testing.T) {
	for _, tc := range []struct {
		cfg     Config
		missing []string
		set     []string
	}{
		{Config{}, []string{"--acoreDsn", "--acoreDbcDir", "--acoreNamesFile"}, nil},
		{Config{DSN: testDSN, Leader: "Deathsong"}, []string{"--acoreDbcDir", "--acoreNamesFile"}, []string{"--acoreDsn"}},
		{Config{DSN: testDSN, DBCDir: "/dbc"}, []string{"--acoreNamesFile"}, []string{"--acoreDsn", "--acoreDbcDir"}},
	} {
		built := false
		handler := NewWithBuilder(tc.cfg, func(context.Context) ([]byte, error) {
			built = true
			return rosterBytes, nil
		})
		message := wantError(t, fmt.Sprint(tc.missing), get(handler), http.StatusServiceUnavailable, "not_configured")
		for _, flag := range tc.missing {
			if !strings.Contains(message, flag) {
				t.Errorf("%q doesn't name %s", message, flag)
			}
		}
		for _, flag := range tc.set {
			if strings.Contains(message, flag) {
				t.Errorf("%q names %s, which is set", message, flag)
			}
		}
		if built || strings.Contains(message, testPassword) {
			t.Errorf("%v: built %v, message %q", tc.missing, built, message)
		}
	}

	// the leader and the matrix are optional
	if response := get(NewWithBuilder(setUp, builds(rosterBytes, nil))); response.Code != http.StatusOK {
		t.Errorf("with the three required settings: %d", response.Code)
	}
}

func TestBuildFailures(t *testing.T) {
	selection := selectionError(t)
	for _, tc := range []struct {
		comment string
		err     error
		status  int
		code    string
		message string // the whole error text, when it's fixed
	}{
		{"a selection error", selection, http.StatusConflict, "no_roster", selection.Error()},
		{"a wrapped selection error", fmt.Errorf("building: %w", selection), http.StatusConflict, "no_roster", selection.Error()},
		{"an empty names file", &Error{http.StatusConflict, "no_roster", "The names file lists no characters"},
			http.StatusConflict, "no_roster", "The names file lists no characters"},
		{"a file the server can't read", configError("Can't read the names file: %v", os.ErrNotExist),
			http.StatusInternalServerError, "server_config", "Can't read the names file: file does not exist"},
		{"a failed query", errors.New("Error 1146 (42S02): Table 'acore_characters.characters' doesn't exist"),
			http.StatusBadGateway, "database", ""},
		{"an unreachable database", errors.New("dial tcp 10.1.2.3:3306: connect: connection refused"),
			http.StatusBadGateway, "database", ""},
	} {
		logged := captureLog(t)
		message := wantError(t, tc.comment, get(NewWithBuilder(setUp, builds(nil, tc.err))), tc.status, tc.code)
		if tc.message != "" && message != tc.message {
			t.Errorf("%s: error %q, want %q", tc.comment, message, tc.message)
		}
		if tc.code == "database" {
			// the detail goes to the log and nowhere else
			if strings.Contains(message, "1146") || strings.Contains(message, "10.1.2.3") {
				t.Errorf("%s: the answer carries the database's own error: %q", tc.comment, message)
			}
			if !strings.Contains(logged.String(), tc.err.Error()) {
				t.Errorf("%s: the log doesn't have the database's error: %q", tc.comment, logged)
			}
		}
		if tc.status == http.StatusConflict && logged.Len() != 0 {
			t.Errorf("%s: logged as if the server had failed: %q", tc.comment, logged)
		}
	}
}

func TestBuilderPanicIsAnswered(t *testing.T) {
	logged := captureLog(t)
	handler := NewWithBuilder(setUp, func(context.Context) ([]byte, error) { panic("index out of range") })
	wantError(t, "a panic", get(handler), http.StatusInternalServerError, "server_config")
	if !strings.Contains(logged.String(), "index out of range") {
		t.Errorf("the log doesn't have the panic: %q", logged)
	}
	// the slot is free again
	wantError(t, "the request after a panic", get(handler), http.StatusInternalServerError, "server_config")
}

func TestBlockedBuilderTimesOut(t *testing.T) {
	captureLog(t)
	cfg := setUp
	cfg.Timeout = 50 * time.Millisecond
	release := make(chan struct{})
	var started atomic.Int32
	// deaf to its context, like a query stuck past every timeout
	handler := NewWithBuilder(cfg, func(context.Context) ([]byte, error) {
		started.Add(1)
		<-release
		return rosterBytes, nil
	})

	start := time.Now()
	message := wantError(t, "a blocked build", get(handler), http.StatusGatewayTimeout, "timeout")
	if waited := time.Since(start); waited < cfg.Timeout || waited > 100*cfg.Timeout {
		t.Errorf("answered after %s, with a timeout of %s", waited, cfg.Timeout)
	}
	if !strings.Contains(message, "50ms") {
		t.Errorf("%q doesn't say how long it waited", message)
	}

	// the stuck build still holds the slot, so the next request spends its whole timeout queueing
	wantError(t, "a request queued behind it", get(handler), http.StatusGatewayTimeout, "timeout")
	if started.Load() != 1 {
		t.Errorf("%d builds started while the first was stuck, want 1", started.Load())
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		response := get(handler)
		if response.Code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("still %d after the stuck build returned", response.Code)
		}
	}
}

func TestBuilderSeesTheDeadline(t *testing.T) {
	captureLog(t)
	cfg := setUp
	cfg.Timeout = 50 * time.Millisecond
	// a database call that gives up when its context does
	handler := NewWithBuilder(cfg, func(ctx context.Context) ([]byte, error) {
		<-ctx.Done()
		return nil, fmt.Errorf("query: %w", ctx.Err())
	})
	wantError(t, "a build that outlives the timeout", get(handler), http.StatusGatewayTimeout, "timeout")
}

func TestRequestsNeverOverlap(t *testing.T) {
	var running, most, total atomic.Int32
	handler := NewWithBuilder(setUp, func(context.Context) ([]byte, error) {
		now := running.Add(1)
		for {
			seen := most.Load()
			if now <= seen || most.CompareAndSwap(seen, now) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		total.Add(1)
		running.Add(-1)
		return rosterBytes, nil
	})

	const requests = 6
	var wait sync.WaitGroup
	codes := make([]int, requests)
	for i := range codes {
		wait.Add(1)
		go func() {
			defer wait.Done()
			codes[i] = get(handler).Code
		}()
	}
	wait.Wait()

	for i, code := range codes {
		if code != http.StatusOK {
			t.Errorf("request %d: %d, want 200", i, code)
		}
	}
	if most.Load() != 1 || total.Load() != requests {
		t.Errorf("%d builds ran, up to %d at once; want %d, one at a time", total.Load(), most.Load(), requests)
	}
}

func TestQueueWaitCountsTowardsTheTimeout(t *testing.T) {
	captureLog(t)
	cfg := setUp
	cfg.Timeout = 600 * time.Millisecond
	started := make(chan struct{}, 2)
	handler := NewWithBuilder(cfg, func(context.Context) ([]byte, error) {
		started <- struct{}{}
		time.Sleep(400 * time.Millisecond)
		return rosterBytes, nil
	})

	first := make(chan int, 1)
	go func() { first <- get(handler).Code }()
	<-started
	// 400 ms in the queue leaves this one 200 ms for a build that takes 400
	wantError(t, "the second request", get(handler), http.StatusGatewayTimeout, "timeout")
	if code := <-first; code != http.StatusOK {
		t.Errorf("the first request: %d, want 200", code)
	}
}

func TestTheRealBuilderThroughTheRoute(t *testing.T) {
	logged := captureLog(t)
	cfg := workingConfig(t)

	// everything but the database works, so this gets as far as dialling it
	message := wantError(t, "an unreachable database", get(New(cfg)), http.StatusBadGateway, "database")
	if strings.Contains(message, "127.0.0.1") {
		t.Errorf("the answer says where the database is: %q", message)
	}
	if !strings.Contains(logged.String(), "127.0.0.1:1") {
		t.Errorf("the log doesn't say what failed: %q", logged)
	}

	empty := cfg
	empty.NamesFile = writeFile(t, "# nobody yet\n")
	wantError(t, "an empty names file", get(New(empty)), http.StatusConflict, "no_roster")

	noDBC := cfg
	noDBC.DBCDir = t.TempDir()
	wantError(t, "an empty DBC directory", get(New(noDBC)), http.StatusInternalServerError, "server_config")

	badDSN := cfg
	badDSN.DSN = "sim:" + testPassword + "@127.0.0.1:3306/"
	message = wantError(t, "a bad DSN", get(New(badDSN)), http.StatusInternalServerError, "server_config")
	if !strings.Contains(message, "--acoreDsn") {
		t.Errorf("%q doesn't name the flag", message)
	}

	wantError(t, "no settings", get(New(Config{})), http.StatusServiceUnavailable, "not_configured")

	if strings.Contains(logged.String(), testPassword) {
		t.Errorf("the log has the DSN's password: %q", logged)
	}
}

func TestRedact(t *testing.T) {
	if got := redact("dial "+testDSN+" failed", testDSN); strings.Contains(got, testPassword) || !strings.Contains(got, "dial") {
		t.Errorf("redact left %q", got)
	}
	if got := redact("connection refused", ""); got != "connection refused" {
		t.Errorf("without a DSN, redact made %q", got)
	}
}
