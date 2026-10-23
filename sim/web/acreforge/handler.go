// Package acreforge applies a character's reforges on the live AzerothCore server, by sending
// mod-reforging's `.reforge apply` command to the worldserver over SOAP.
package acreforge

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// Config is what the route reaches the server with. All four settings are required, and without them
// every request is answered 503.
type Config struct {
	// SoapURL is the worldserver's SOAP endpoint, e.g. http://192.168.49.35:7878/.
	SoapURL string
	// SoapUser is a SOAP-enabled administrator account.
	SoapUser string
	// SoapPassword is SoapUser's password. Only the environment sets it: a flag would show in the
	// process list.
	SoapPassword string
	// NamesFile lists the core raid, one character a line. Only they can be reforged.
	NamesFile string
	// Timeout is how long a request gets, its wait for the one before it included. 0 means 30 s.
	Timeout time.Duration
}

const (
	defaultTimeout = 30 * time.Second
	maxBodyBytes   = 64 << 10
	maxReplyBytes  = 1 << 20
	// equipment slots on the server, head to ranged
	slotCount = 19
	// the longest stretch of the server's own text an error repeats
	maxQuoted = 300
)

// reforgeablePair checks a pair against the live Reforging.ReforgeableStats, the list the UI offers
// reforges from too.
func reforgeablePair(from, to int) bool {
	return from == int(int32(from)) && to == int(int32(to)) &&
		core.LiveReforging().ReforgeableStatPair(int32(from), int32(to))
}

// WithEnv fills the settings still empty from the environment.
func (cfg Config) WithEnv(getenv func(string) string) Config {
	for _, setting := range []struct {
		value *string
		env   string
	}{
		{&cfg.SoapURL, "AC_SOAP_URL"},
		{&cfg.SoapUser, "AC_SOAP_USER"},
		{&cfg.SoapPassword, "AC_SOAP_PASSWORD"},
		{&cfg.NamesFile, "AC_RAID_NAMES_FILE"},
	} {
		if *setting.value == "" {
			*setting.value = getenv(setting.env)
		}
	}
	return cfg
}

// missingFlags names the settings that aren't set.
func (cfg Config) missingFlags() []string {
	var missing []string
	for _, setting := range []struct{ value, name string }{
		{cfg.SoapURL, "--acoreSoapUrl"}, {cfg.SoapUser, "--acoreSoapUser"},
		{cfg.SoapPassword, "$AC_SOAP_PASSWORD"}, {cfg.NamesFile, "--acoreNamesFile"},
	} {
		if setting.value == "" {
			missing = append(missing, setting.name)
		}
	}
	return missing
}

// Error is a failure with the status and code the route answers it with.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// New serves POST /acoreReforge.
func New(cfg Config) http.Handler {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	h := &handler{
		cfg:     cfg,
		missing: cfg.missingFlags(),
		timeout: timeout,
		client: &http.Client{
			// a redirect would carry the login to wherever it points
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		slot: make(chan struct{}, 1),
	}
	if len(h.missing) > 0 {
		log.Printf("/acoreReforge is off: no %s", strings.Join(h.missing, ", "))
	} else {
		log.Printf("/acoreReforge sends reforges for the characters in %s to %s", cfg.NamesFile, h.redact(cfg.SoapURL))
	}
	return h
}

type handler struct {
	cfg     Config
	missing []string
	timeout time.Duration
	client  *http.Client
	// slot holds a token while a request runs, so the worldserver gets one command at a time
	slot chan struct{}
}

type request struct {
	Character string        `json:"character"`
	Items     []requestItem `json:"items"`
}

type requestItem struct {
	AcSlot int `json:"acSlot"`
	ItemID int `json:"itemId"`
	From   int `json:"from"`
	To     int `json:"to"`
}

type slotResult struct {
	AcSlot int    `json:"acSlot"`
	Status string `json:"status"`
	Text   string `json:"text"`
}

type reply struct {
	Online bool         `json:"online"`
	Slots  []slotResult `json:"slots"`
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	switch {
	case r.Method != http.MethodPost:
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, &Error{http.StatusMethodNotAllowed, "method_not_allowed", "/acoreReforge only answers POST"})
		return
	// another site's page must not be able to change the server's characters
	case strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site"):
		writeError(w, &Error{http.StatusForbidden, "cross_site", "/acoreReforge doesn't answer other sites"})
		return
	case len(h.missing) > 0:
		writeError(w, &Error{http.StatusServiceUnavailable, "not_configured",
			"This sim server isn't set up to reforge on AzerothCore: it was started without " + strings.Join(h.missing, ", ")})
		return
	}

	req, failure := h.decode(r)
	if failure == nil {
		failure = h.checkRoster(&req)
	}
	if failure != nil {
		writeError(w, failure)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	select {
	case h.slot <- struct{}{}:
		defer func() { <-h.slot }()
	case <-ctx.Done():
		h.fail(w, ctx.Err())
		return
	}

	result, err := h.send(ctx, req)
	if err != nil {
		h.fail(w, err)
		return
	}
	body, _ := json.Marshal(result)
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// decode reads and checks the request body.
func (h *handler) decode(r *http.Request) (request, *Error) {
	var req request
	bad := func(format string, args ...any) (request, *Error) {
		return req, &Error{http.StatusBadRequest, "bad_request", fmt.Sprintf(format, args...)}
	}
	if mediaType, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); !strings.EqualFold(strings.TrimSpace(mediaType), "application/json") {
		return bad("/acoreReforge takes application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return bad("The request isn't valid: %v", err)
	}
	if decoder.More() {
		return bad("The request has more than one JSON value")
	}

	if strings.TrimSpace(req.Character) == "" {
		return bad("The request names no character")
	}
	if len(req.Items) < 1 || len(req.Items) > slotCount {
		return bad("The request needs 1 to %d items, not %d", slotCount, len(req.Items))
	}
	seen := map[int]bool{}
	for _, item := range req.Items {
		switch {
		case item.AcSlot < 0 || item.AcSlot >= slotCount:
			return bad("Slot %d isn't an equipment slot (0 to %d)", item.AcSlot, slotCount-1)
		case seen[item.AcSlot]:
			return bad("Slot %d is given twice", item.AcSlot)
		case item.ItemID <= 0:
			return bad("Slot %d has no item id", item.AcSlot)
		case (item.From == 0) != (item.To == 0):
			return bad("Slot %d: from and to must both be 0 or both be stats", item.AcSlot)
		case item.From != 0 && !reforgeablePair(item.From, item.To):
			return bad("Slot %d: %d to %d isn't a reforgeable stat pair", item.AcSlot, item.From, item.To)
		}
		seen[item.AcSlot] = true
	}
	return req, nil
}

// checkRoster swaps the request's character for the names file's spelling of it, which is also what
// keeps anything but a listed name out of the command.
func (h *handler) checkRoster(req *request) *Error {
	text, err := os.ReadFile(h.cfg.NamesFile)
	if err != nil {
		log.Printf("/acoreReforge can't read the names file: %v", err)
		return &Error{http.StatusServiceUnavailable, "not_configured", "The names file can't be read. The sim server's log says why."}
	}
	for _, name := range azerothcore.ParseNames(string(text)) {
		if strings.EqualFold(name, strings.TrimSpace(req.Character)) && !strings.ContainsAny(name, " \t") {
			req.Character = name
			return nil
		}
	}
	return &Error{http.StatusConflict, "not_in_roster", fmt.Sprintf("%s isn't in the core raid", quote(req.Character))}
}

// command is the console command for the request.
func command(req request) string {
	items := append([]requestItem(nil), req.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].AcSlot < items[j].AcSlot })
	var command strings.Builder
	command.WriteString("reforge apply " + req.Character)
	for _, item := range items {
		fmt.Fprintf(&command, " %d:%d:%d:%d", item.AcSlot, item.ItemID, item.From, item.To)
	}
	return command.String()
}

type envelope struct {
	Result string `xml:"Body>executeCommandResponse>result"`
	Fault  string `xml:"Body>Fault>faultstring"`
}

func (h *handler) send(ctx context.Context, req request) (*reply, error) {
	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns1="urn:AC">` +
		`<SOAP-ENV:Body><ns1:executeCommand><command>`)
	xml.EscapeText(&body, []byte(command(req)))
	body.WriteString(`</command></ns1:executeCommand></SOAP-ENV:Body></SOAP-ENV:Envelope>`)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.SoapURL, &body)
	if err != nil {
		log.Printf("/acoreReforge: %s", h.redact(err.Error()))
		return nil, &Error{http.StatusBadGateway, "soap", "The SOAP address isn't valid. The sim server's log says why."}
	}
	httpReq.Header.Set("Content-Type", "text/xml; charset=utf-8")
	httpReq.SetBasicAuth(h.cfg.SoapUser, h.cfg.SoapPassword)

	response, err := h.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		log.Printf("/acoreReforge: %s", h.redact(err.Error()))
		return nil, &Error{http.StatusBadGateway, "soap", "The worldserver's SOAP port didn't answer. The sim server's log says why."}
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxReplyBytes))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &Error{http.StatusBadGateway, "soap", "The worldserver's SOAP reply broke off."}
	}

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, &Error{http.StatusBadGateway, "soap", "The SOAP login was refused"}
	}
	var parsed envelope
	xmlErr := xml.Unmarshal(raw, &parsed)
	switch {
	case parsed.Fault != "":
		return nil, &Error{http.StatusBadGateway, "soap", quote(h.redact(parsed.Fault))}
	case xmlErr != nil || response.StatusCode != http.StatusOK:
		log.Printf("/acoreReforge: SOAP status %d, unreadable reply (%v)", response.StatusCode, xmlErr)
		return nil, &Error{http.StatusBadGateway, "soap", fmt.Sprintf("The worldserver's SOAP reply wasn't readable (HTTP %d)", response.StatusCode)}
	}
	return parseResult(parsed.Result, h.redact)
}

// parseResult reads the command's reply: `character <name> online|offline`, then
// `<slot> <status> <text>` a line. What it quotes of an unexpected reply goes through redact.
func parseResult(result string, redact func(string) string) (*reply, error) {
	unexpected := func() error {
		return &Error{http.StatusBadGateway, "soap",
			"The worldserver didn't answer like .reforge apply does, is mod-reforging up to date? It said: " + quote(redact(result))}
	}
	lines := strings.Split(strings.ReplaceAll(result, "\r", ""), "\n")
	out := &reply{Slots: []slotResult{}}
	first := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, " ", 3)
		if first {
			first = false
			fields = strings.Fields(line)
			if len(fields) != 3 || fields[0] != "character" || (fields[2] != "online" && fields[2] != "offline") {
				return nil, unexpected()
			}
			out.Online = fields[2] == "online"
			continue
		}
		slot, err := strconv.Atoi(fields[0])
		if err != nil || len(fields) < 2 {
			return nil, unexpected()
		}
		text := ""
		if len(fields) == 3 {
			text = strings.TrimSpace(fields[2])
		}
		out.Slots = append(out.Slots, slotResult{slot, fields[1], text})
	}
	if first {
		return nil, unexpected()
	}
	return out, nil
}

func quote(text string) string {
	text = strings.TrimSpace(text)
	if len(text) > maxQuoted {
		text = text[:maxQuoted] + "..."
	}
	return text
}

// fail answers with the status the error stands for, and logs what the answer leaves out.
func (h *handler) fail(w http.ResponseWriter, err error) {
	var failure *Error
	switch {
	case errors.As(err, &failure):
	// Canceled is the browser going away, where nobody reads the answer
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		// the worldserver runs a command it got however long it takes, so a sent one can still land
		failure = &Error{http.StatusGatewayTimeout, "timeout",
			fmt.Sprintf("The worldserver didn't answer within %s and may still apply the reforges. Send again to see where each slot stands.", h.timeout)}
	default:
		failure = &Error{http.StatusBadGateway, "soap", "The worldserver's SOAP port failed. The sim server's log says why."}
		log.Printf("/acoreReforge: %s", h.redact(err.Error()))
	}
	if failure.Status >= http.StatusInternalServerError {
		log.Printf("/acoreReforge %d %s: %s", failure.Status, failure.Code, h.redact(failure.Message))
	}
	writeError(w, failure)
}

// redact keeps the password and the SOAP account's name out of a log line, an error or a reply.
func (h *handler) redact(text string) string {
	for _, secret := range []struct{ value, mask string }{
		{h.cfg.SoapPassword, "<password>"}, {h.cfg.SoapUser, "<user>"},
	} {
		if secret.value != "" {
			text = strings.ReplaceAll(text, secret.value, secret.mask)
		}
	}
	return text
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
