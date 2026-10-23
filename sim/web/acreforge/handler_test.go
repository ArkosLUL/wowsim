package acreforge

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testUser     = "soapadmin"
	testPassword = "hunter2-secret"
)

type soapStub struct {
	mu      sync.Mutex
	status  int
	body    string
	delay   time.Duration
	user    string
	pass    string
	command string
	calls   int
}

func (s *soapStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.calls++
	s.user, s.pass, _ = r.BasicAuth()
	if _, rest, ok := strings.Cut(string(raw), "<command>"); ok {
		s.command, _, _ = strings.Cut(rest, "</command>")
	}
	status, body, delay := s.status, s.body, s.delay
	s.mu.Unlock()
	time.Sleep(delay)
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	io.WriteString(w, body)
}

func result(text string) string {
	return `<?xml version="1.0"?><SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns1="urn:AC">` +
		`<SOAP-ENV:Body><ns1:executeCommandResponse><result>` + text + `</result></ns1:executeCommandResponse></SOAP-ENV:Body></SOAP-ENV:Envelope>`
}

func fault(text string) string {
	return `<?xml version="1.0"?><SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/">` +
		`<SOAP-ENV:Body><SOAP-ENV:Fault><faultcode>SOAP-ENV:Server</faultcode><faultstring>` + text +
		`</faultstring></SOAP-ENV:Fault></SOAP-ENV:Body></SOAP-ENV:Envelope>`
}

func setup(t *testing.T, stub *soapStub, edit func(*Config)) http.Handler {
	t.Helper()
	server := httptest.NewServer(stub)
	t.Cleanup(server.Close)
	names := filepath.Join(t.TempDir(), "core-raid.txt")
	if err := os.WriteFile(names, []byte("# raid\nAngry\nDeathsong\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Config{SoapURL: server.URL, SoapUser: testUser, SoapPassword: testPassword, NamesFile: names, Timeout: 5 * time.Second}
	if edit != nil {
		edit(&cfg)
	}
	return New(cfg)
}

func post(h http.Handler, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/acoreReforge", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct{ Code, Error string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not an error body: %s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), testPassword) {
		t.Errorf("the password is in the reply: %s", rec.Body)
	}
	return body.Code
}

const goodBody = `{"character":"angry","items":[{"acSlot":4,"itemId":100,"from":32,"to":36},{"acSlot":0,"itemId":200,"from":0,"to":0}]}`

func TestApplyBuildsTheCommandAndParsesTheReply(t *testing.T) {
	stub := &soapStub{body: result("character Angry online\n0 removed Crit -&gt; Haste\n4 applied Crit -&gt; Haste\n")}
	rec := post(setup(t, stub, nil), goodBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if stub.user != testUser || stub.pass != testPassword {
		t.Errorf("basic auth was %q / %q", stub.user, stub.pass)
	}
	if want := "reforge apply Angry 0:200:0:0 4:100:32:36"; stub.command != want {
		t.Errorf("command %q, want %q", stub.command, want)
	}
	var got reply
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []slotResult{{0, "removed", "Crit -> Haste"}, {4, "applied", "Crit -> Haste"}}
	if !got.Online || len(got.Slots) != 2 || got.Slots[0] != want[0] || got.Slots[1] != want[1] {
		t.Errorf("reply %+v", got)
	}
}

func TestOfflineReply(t *testing.T) {
	stub := &soapStub{body: result("character Angry offline\n4 unchanged same reforge")}
	rec := post(setup(t, stub, nil), goodBody)
	var got reply
	json.Unmarshal(rec.Body.Bytes(), &got)
	if rec.Code != 200 || got.Online || len(got.Slots) != 1 || got.Slots[0].Text != "same reforge" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestFaultPassesThrough(t *testing.T) {
	stub := &soapStub{status: 500, body: fault("No character called Nobody")}
	rec := post(setup(t, stub, nil), goodBody)
	if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "soap" || !strings.Contains(rec.Body.String(), "No character called Nobody") {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestRefusedLogin(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		stub := &soapStub{status: status, body: "nope"}
		rec := post(setup(t, stub, nil), goodBody)
		if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "soap" || !strings.Contains(rec.Body.String(), "SOAP login was refused") {
			t.Errorf("%d: %d %s", status, rec.Code, rec.Body)
		}
	}
}

// captureLog collects the route's log lines for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &logged
}

func checkLogHasNoLogin(t *testing.T, logged *bytes.Buffer) {
	t.Helper()
	if strings.Contains(logged.String(), testPassword[:5]) || strings.Contains(logged.String(), testUser) {
		t.Errorf("the login is in the log: %s", logged)
	}
}

func TestUnexpectedReplyIsReportedAndRedacted(t *testing.T) {
	logged := captureLog(t)
	stub := &soapStub{body: result("There is no such command " + testPassword)}
	rec := post(setup(t, stub, nil), goodBody)
	if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "soap" || !strings.Contains(rec.Body.String(), "no such command") {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	checkLogHasNoLogin(t, logged)
}

func TestFaultTextIsRedacted(t *testing.T) {
	logged := captureLog(t)
	// the second fault puts the password across the cut a long quote gets
	for _, text := range []string{"bad login " + testPassword, strings.Repeat("x", maxQuoted-5) + testPassword} {
		stub := &soapStub{status: 500, body: fault(text)}
		rec := post(setup(t, stub, nil), goodBody)
		errorCode(t, rec)
		if strings.Contains(rec.Body.String(), testPassword[:5]) {
			t.Errorf("leaked: %s", rec.Body)
		}
	}
	if !strings.Contains(logged.String(), "bad login") {
		t.Errorf("the fault wasn't logged: %s", logged)
	}
	checkLogHasNoLogin(t, logged)
}

func TestTimeout(t *testing.T) {
	stub := &soapStub{body: result("character Angry online"), delay: 600 * time.Millisecond}
	rec := post(setup(t, stub, func(cfg *Config) { cfg.Timeout = 100 * time.Millisecond }), goodBody)
	if rec.Code != http.StatusGatewayTimeout || errorCode(t, rec) != "timeout" || !strings.Contains(rec.Body.String(), "may still apply") {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
}

func TestUnreachableSoapDoesNotLeak(t *testing.T) {
	logged := captureLog(t)
	stub := &soapStub{}
	rec := post(setup(t, stub, func(cfg *Config) { cfg.SoapURL = "http://" + testUser + ":" + testPassword + "@127.0.0.1:1/" }), goodBody)
	if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "soap" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(logged.String(), "127.0.0.1:1") {
		t.Errorf("the dial error wasn't logged: %s", logged)
	}
	checkLogHasNoLogin(t, logged)
}

func TestBadSoapURLIsLogged(t *testing.T) {
	logged := captureLog(t)
	rec := post(setup(t, &soapStub{}, func(cfg *Config) { cfg.SoapURL = "http://" + testUser + ":" + testPassword + "@[::1" }), goodBody)
	if rec.Code != http.StatusBadGateway || errorCode(t, rec) != "soap" {
		t.Errorf("%d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(logged.String(), "missing ']'") {
		t.Errorf("the parse error wasn't logged: %s", logged)
	}
	checkLogHasNoLogin(t, logged)
}

func TestNotInRoster(t *testing.T) {
	stub := &soapStub{body: result("character X online")}
	rec := post(setup(t, stub, nil), strings.Replace(goodBody, "angry", "Stranger", 1))
	if rec.Code != http.StatusConflict || errorCode(t, rec) != "not_in_roster" || stub.calls != 0 {
		t.Errorf("%d %s calls=%d", rec.Code, rec.Body, stub.calls)
	}
}

func TestNotConfiguredAnswersBeforeReadingTheBody(t *testing.T) {
	for _, drop := range []string{"url", "user", "password", "names"} {
		h := setup(t, &soapStub{}, func(cfg *Config) {
			switch drop {
			case "url":
				cfg.SoapURL = ""
			case "user":
				cfg.SoapUser = ""
			case "password":
				cfg.SoapPassword = ""
			case "names":
				cfg.NamesFile = ""
			}
		})
		for _, body := range []string{"{}", "", "not json"} {
			rec := post(h, body)
			if rec.Code != http.StatusServiceUnavailable || errorCode(t, rec) != "not_configured" {
				t.Errorf("%s/%q: %d %s", drop, body, rec.Code, rec.Body)
			}
		}
	}
}

func TestNotConfiguredNamesWhatIsMissing(t *testing.T) {
	h := New(Config{}.WithEnv(func(string) string { return "" }))
	rec := post(h, "{}")
	for _, want := range []string{"--acoreSoapUrl", "--acoreSoapUser", "AC_SOAP_PASSWORD", "--acoreNamesFile"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%q missing from %s", want, rec.Body)
		}
	}
}

func TestWithEnvKeepsFlags(t *testing.T) {
	env := map[string]string{"AC_SOAP_URL": "env-url", "AC_SOAP_USER": "env-user", "AC_SOAP_PASSWORD": "env-pass", "AC_RAID_NAMES_FILE": "env-names"}
	cfg := Config{SoapUser: "flag-user"}.WithEnv(func(key string) string { return env[key] })
	if cfg.SoapURL != "env-url" || cfg.SoapUser != "flag-user" || cfg.SoapPassword != "env-pass" || cfg.NamesFile != "env-names" {
		t.Errorf("%+v", cfg)
	}
}

func TestMethodAndCrossSite(t *testing.T) {
	stub := &soapStub{body: result("character Angry online")}
	h := setup(t, stub, nil)
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/acoreReforge", nil))
	if get.Code != http.StatusMethodNotAllowed || errorCode(t, get) != "method_not_allowed" || get.Header().Get("Allow") != "POST" {
		t.Errorf("get: %d %s", get.Code, get.Body)
	}
	rec := post(h, goodBody, "Sec-Fetch-Site", "cross-site")
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != "cross_site" || stub.calls != 0 {
		t.Errorf("cross-site: %d %s", rec.Code, rec.Body)
	}
	if rec := post(h, goodBody, "Sec-Fetch-Site", "same-origin"); rec.Code != http.StatusOK {
		t.Errorf("same-origin: %d %s", rec.Code, rec.Body)
	}
}

func TestBadRequests(t *testing.T) {
	item := func(slot, from, to int) string {
		return `{"acSlot":` + itoa(slot) + `,"itemId":5,"from":` + itoa(from) + `,"to":` + itoa(to) + `}`
	}
	many := make([]string, 0, 20)
	for slot := 0; slot < 20; slot++ {
		many = append(many, item(slot%19, 0, 0))
	}
	cases := map[string]string{
		"not json":         `nope`,
		"unknown field":    `{"character":"Angry","items":[` + item(0, 0, 0) + `],"extra":1}`,
		"trailing value":   goodBody + goodBody,
		"no character":     `{"items":[` + item(0, 0, 0) + `]}`,
		"no items":         `{"character":"Angry","items":[]}`,
		"too many":         `{"character":"Angry","items":[` + strings.Join(many, ",") + `]}`,
		"slot 19":          `{"character":"Angry","items":[` + item(19, 0, 0) + `]}`,
		"negative slot":    `{"character":"Angry","items":[` + item(-1, 0, 0) + `]}`,
		"duplicate slot":   `{"character":"Angry","items":[` + item(3, 0, 0) + `,` + item(3, 0, 0) + `]}`,
		"half a reforge":   `{"character":"Angry","items":[` + item(3, 32, 0) + `]}`,
		"same stats":       `{"character":"Angry","items":[` + item(3, 32, 32) + `]}`,
		"unreforgeable":    `{"character":"Angry","items":[` + item(3, 32, 7) + `]}`,
		"stat past int32":  `{"character":"Angry","items":[` + item(3, 32, 1<<32+37) + `]}`,
		"no item id":       `{"character":"Angry","items":[{"acSlot":3,"itemId":0,"from":0,"to":0}]}`,
		"command injected": `{"character":"Angry 1:2:3:4","items":[` + item(3, 0, 0) + `]}`,
	}
	stub := &soapStub{body: result("character Angry online")}
	h := setup(t, stub, nil)
	for name, body := range cases {
		rec := post(h, body)
		wantCode, wantStatus := "bad_request", http.StatusBadRequest
		if name == "command injected" {
			wantCode, wantStatus = "not_in_roster", http.StatusConflict
		}
		if rec.Code != wantStatus || errorCode(t, rec) != wantCode {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
	form := httptest.NewRequest(http.MethodPost, "/acoreReforge", strings.NewReader(goodBody))
	form.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, form)
	if rec.Code != http.StatusBadRequest || errorCode(t, rec) != "bad_request" {
		t.Errorf("text/plain: %d %s", rec.Code, rec.Body)
	}
	if stub.calls != 0 {
		t.Errorf("a bad request reached the worldserver %d times", stub.calls)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestOneRequestAtATime(t *testing.T) {
	stub := &soapStub{body: result("character Angry online"), delay: 150 * time.Millisecond}
	h := setup(t, stub, nil)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if rec := post(h, goodBody); rec.Code != http.StatusOK {
				t.Errorf("%d %s", rec.Code, rec.Body)
			}
		}()
	}
	wg.Wait()
	if time.Since(start) < 290*time.Millisecond {
		t.Errorf("both requests ran at once: %s", time.Since(start))
	}
}
