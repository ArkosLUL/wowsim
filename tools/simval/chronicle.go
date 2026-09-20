package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// A mod-chronicle raw log is one event per line, `<unix_ms>  EVENT,field,...`,
// with two spaces after the timestamp. Fields are comma separated, strings are
// quoted, flags and GUIDs are hex, and a false boolean prints as `nil`.
//
// Damage and aura events start with the WotLK base params, source then
// destination, each `guid,"name",0xflags`. Everything in the SPELL_ family also
// carries `spellId,"spellName",0xschool` before its own fields.

// COMBATLOG_OBJECT_TYPE_PLAYER, as mod-chronicle writes the flags. A pet or
// guardian is told apart by its owner in CHRONICLE_UNIT_INFO instead, since the
// flags say what a unit is but not whose it is.
const objectTypePlayer = 0x0400

type combatant struct {
	GUID  string
	Name  string
	Flags uint64
}

func (c combatant) isPlayer() bool { return c.Flags&objectTypePlayer != 0 }

type spellRef struct {
	ID     int32
	Name   string
	School uint64
}

type chronicleEvent struct {
	TimeMs int64
	Type   string
	Src    combatant
	Dst    combatant
	Spell  spellRef
	// Args is what follows the base params and the spell prefix: the damage
	// suffix, the miss type, the aura kind, and so on.
	Args []string
}

// chronicleLog is one instance's log: its zone header, the units it named, and
// every event in file order.
type chronicleLog struct {
	Path     string
	Zone     string
	MapID    int64
	Instance int64
	Events   []chronicleEvent
	// Owners maps a pet or guardian GUID to the player that owns it, from
	// CHRONICLE_UNIT_INFO. Damage by a minion counts for its owner.
	Owners map[string]string
	Names  map[string]string
}

// readChronicleLog reads a raw log, or a gzipped one: five minutes of combat is a
// megabyte of text, so the committed captures are kept compressed.
func readChronicleLog(path string) (*chronicleLog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var reader io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		unzip, err := gzip.NewReader(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		defer unzip.Close()
		reader = unzip
	}
	return parseChronicleLog(path, reader)
}

func parseChronicleLog(path string, r io.Reader) (*chronicleLog, error) {
	log := &chronicleLog{Path: path, Owners: map[string]string{}, Names: map[string]string{}}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.TrimSpace(text) == "" {
			continue
		}

		event, err := parseChronicleEvent(text)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		switch event.Type {
		case "CHRONICLE_ZONE_INFO":
			log.Zone = unquote(arg(event.Args, 0))
			log.MapID = parseInt(arg(event.Args, 1))
			log.Instance = parseInt(arg(event.Args, 2))
		case "CHRONICLE_UNIT_INFO":
			log.readUnitInfo(event.Args)
		default:
			log.Events = append(log.Events, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(log.Events) == 0 {
		return nil, fmt.Errorf("%s has no combat events", path)
	}
	return log, nil
}

// readUnitInfo takes the owner chain out of
// `guid,"name",level,0xflags,ownerGuid,maxHealth,"affiliation",isBoss`.
func (l *chronicleLog) readUnitInfo(args []string) {
	guid := arg(args, 0)
	if guid == "" {
		return
	}
	l.Names[guid] = unquote(arg(args, 1))
	if owner := arg(args, 4); owner != "" && !isEmptyGUID(owner) {
		l.Owners[guid] = owner
	}
}

func parseChronicleEvent(text string) (chronicleEvent, error) {
	sep := strings.Index(text, "  ")
	if sep < 0 {
		return chronicleEvent{}, fmt.Errorf("no timestamp separator in %q", trim(text, 60))
	}
	timeMs, err := strconv.ParseInt(strings.TrimSpace(text[:sep]), 10, 64)
	if err != nil {
		return chronicleEvent{}, fmt.Errorf("timestamp %q: %w", text[:sep], err)
	}

	fields := splitLogFields(strings.TrimLeft(text[sep:], " "))
	if len(fields) == 0 {
		return chronicleEvent{}, fmt.Errorf("no event type in %q", trim(text, 60))
	}

	event := chronicleEvent{TimeMs: timeMs, Type: fields[0]}
	rest := fields[1:]
	if hasBaseParams(event.Type) {
		if len(rest) < 6 {
			return chronicleEvent{}, fmt.Errorf("%s has %d fields, want at least 6", event.Type, len(rest))
		}
		event.Src = combatant{GUID: rest[0], Name: unquote(rest[1]), Flags: uint64(parseInt(rest[2]))}
		event.Dst = combatant{GUID: rest[3], Name: unquote(rest[4]), Flags: uint64(parseInt(rest[5]))}
		rest = rest[6:]
	}
	if hasSpellPrefix(event.Type) {
		if len(rest) < 3 {
			return chronicleEvent{}, fmt.Errorf("%s has no spell prefix", event.Type)
		}
		event.Spell = spellRef{
			ID:     int32(parseInt(rest[0])),
			Name:   unquote(rest[1]),
			School: uint64(parseInt(rest[2])),
		}
		rest = rest[3:]
	}
	event.Args = rest
	return event, nil
}

// hasBaseParams reports whether the event starts with the source and destination
// triples. The CHRONICLE_ extensions each have their own shape.
func hasBaseParams(eventType string) bool {
	switch eventType {
	case "UNIT_DIED", "DAMAGE_SHIELD", "ENVIRONMENTAL_DAMAGE":
		return true
	}
	return strings.HasPrefix(eventType, "SWING_") ||
		strings.HasPrefix(eventType, "SPELL_") ||
		strings.HasPrefix(eventType, "RANGE_")
}

func hasSpellPrefix(eventType string) bool {
	switch eventType {
	case "DAMAGE_SHIELD":
		return true
	case "ENVIRONMENTAL_DAMAGE":
		return false
	}
	return strings.HasPrefix(eventType, "SPELL_") || strings.HasPrefix(eventType, "RANGE_")
}

// splitLogFields splits on commas outside double quotes, keeping the quotes so a
// caller can tell a quoted name from a bare value.
func splitLogFields(text string) []string {
	var fields []string
	var start int
	quoted := false
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '"':
			quoted = !quoted
		case ',':
			if !quoted {
				fields = append(fields, text[start:i])
				start = i + 1
			}
		}
	}
	return append(fields, text[start:])
}

func arg(args []string, i int) string {
	if i < 0 || i >= len(args) {
		return ""
	}
	return args[i]
}

func unquote(field string) string {
	return strings.Trim(field, `"`)
}

// parseInt reads a decimal or 0x-prefixed field. `nil`, an empty field and
// anything unparseable read as 0, which is what the log means by them.
func parseInt(field string) int64 {
	field = strings.TrimSpace(unquote(field))
	if field == "" || field == "nil" {
		return 0
	}
	if rest, ok := strings.CutPrefix(field, "0x"); ok {
		v, err := strconv.ParseUint(rest, 16, 64)
		if err != nil {
			return 0
		}
		return int64(v)
	}
	v, err := strconv.ParseInt(field, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// logFlag reads a WoW combat log boolean: `1` for true, `nil` for false.
func logFlag(field string) bool { return strings.TrimSpace(field) == "1" }

func isEmptyGUID(guid string) bool {
	return strings.Trim(strings.TrimPrefix(guid, "0x"), "0") == ""
}

// trim cuts text down to n characters, ellipsis included, so a long name still
// fits the column it prints in.
func trim(text string, n int) string {
	if len(text) <= n {
		return text
	}
	if n <= 3 {
		return text[:n]
	}
	return text[:n-3] + "..."
}

// damage is the `amount,overkill,school,resisted,blocked,absorbed,critical,glancing,crushing`
// suffix every damage event ends with.
type damage struct {
	Amount   int64
	Overkill int64
	Resisted int64
	Blocked  int64
	Absorbed int64
	Crit     bool
	Glancing bool
	Crushing bool
}

func (e chronicleEvent) damage() damage {
	return damage{
		Amount:   parseInt(arg(e.Args, 0)),
		Overkill: parseInt(arg(e.Args, 1)),
		Resisted: parseInt(arg(e.Args, 3)),
		Blocked:  parseInt(arg(e.Args, 4)),
		Absorbed: parseInt(arg(e.Args, 5)),
		Crit:     logFlag(arg(e.Args, 6)),
		Glancing: logFlag(arg(e.Args, 7)),
		Crushing: logFlag(arg(e.Args, 8)),
	}
}

// missType is the SWING_MISSED / SPELL_MISSED result: MISS, DODGE, PARRY, RESIST,
// ABSORB and so on.
func (e chronicleEvent) missType() string { return unquote(arg(e.Args, 0)) }

func (e chronicleEvent) isDamage() bool {
	switch e.Type {
	case "SWING_DAMAGE", "SPELL_DAMAGE", "SPELL_PERIODIC_DAMAGE", "RANGE_DAMAGE", "DAMAGE_SHIELD":
		return true
	}
	return false
}

func (e chronicleEvent) isMiss() bool {
	switch e.Type {
	case "SWING_MISSED", "SPELL_MISSED", "RANGE_MISSED":
		return true
	}
	return false
}

func (e chronicleEvent) isSwing() bool {
	return e.Type == "SWING_DAMAGE" || e.Type == "SWING_MISSED"
}
