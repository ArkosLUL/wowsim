package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// threatKey names a capture's record by what it probed: the player's class, its stance, form, presence or
// Righteous Fury, the spell, the target count the probe was given and the creatures fighting the caster.
type threatKey struct {
	class     uint8
	form      int32
	spell     int32
	override  int32
	creatures int32
}

func (r threatRecord) key() threatKey {
	return threatKey{r.Attacker.Class, r.form(), r.Spell.ID, r.override(), r.creatures()}
}

// threatDrop is why a probe has no record in the capture, with a row in the fixture README: the replay
// can't build it, or it can now (recapture) but the capture lost the record, which the next
// TestSimvalThreat run brings back.
type threatDrop struct {
	reason    string
	recapture bool
}

// the server's class ids of the dropped probes
const (
	threatDeathKnight uint8 = 6
	threatWarlock     uint8 = 9
)

var droppedThreatRecords = map[threatKey]threatDrop{
	{threatDeathKnight, 48263, 45524, 0, 0}: {reason: "the sim has no Chains of Ice"},
	{threatDeathKnight, 48263, 49576, 0, 0}: {reason: "the sim has no Death Grip"},
	{threatWarlock, 0, 47193, 0, 0}: {reason: "cast on the pet, so the server hands its bonus to the creatures fighting the " +
		"pet; the probe's warlock has no pet in a fight, the replay's no Demonic Empowerment"},
}

// threatCut is what cutThreatFixture made of a stream: the fixture's lines as the server wrote them, and what
// it left out or wants done by hand.
type threatCut struct {
	lines []string
	notes []string
	// distinct lines read, and the threat records among them from sinceMs on
	read, threat int
}

// threatLineKey reads a record line's key, and whether it's a threat record at or after sinceMs.
func threatLineKey(line string, sinceMs int64) (rec threatRecord, wanted bool, err error) {
	var head struct {
		Command string `json:"command"`
		UnixMs  int64  `json:"unixMs"`
	}
	if err := json.Unmarshal([]byte(line), &head); err != nil {
		return rec, false, err
	}
	if head.Command != "threat" || head.UnixMs < sinceMs {
		return rec, false, nil
	}
	return rec, true, json.Unmarshal([]byte(line), &rec)
}

// cutThreatFixture picks threat.jsonl's lines out of a stream of simval records: the threat records from
// sinceMs on, one per threatRecord.key (a repeated probe's last, in the order the probes last ran), without
// droppedThreatRecords' probes the replay can't build. A line the stream holds twice counts once.
//
// With old, a fixture's lines, the stream is part of a run: its probes replace old's in place or go at the end.
func cutThreatFixture(stream io.Reader, sinceMs int64, old []string) (threatCut, error) {
	var cut threatCut
	last, labels := map[threatKey]string{}, map[threatKey]string{}
	var order []threatKey
	place := func(key threatKey, line string, move bool) {
		_, have := last[key]
		if have && move {
			order = removeThreatKey(order, key)
		}
		if !have || move {
			order = append(order, key)
		}
		last[key] = line
	}

	for i, line := range old {
		rec, wanted, err := threatLineKey(line, 0)
		if err != nil {
			return cut, fmt.Errorf("the fixture's line %d: %w", i+1, err)
		}
		if !wanted {
			return cut, fmt.Errorf("the fixture's line %d isn't a threat record", i+1)
		}
		place(rec.key(), line, false)
		labels[rec.key()] = rec.label()
	}

	seen, ran := map[string]bool{}, map[threatKey]bool{}
	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if strings.TrimSpace(line) == "" || seen[line] {
			continue
		}
		seen[line] = true
		cut.read++
		rec, wanted, err := threatLineKey(line, sinceMs)
		if err != nil {
			return cut, fmt.Errorf("line %d: %w", n, err)
		}
		if !wanted {
			continue
		}
		cut.threat++
		key := rec.key()
		labels[key] = rec.label()
		// in a part of a run the probe keeps its place in the fixture
		place(key, line, old == nil)
		if ran[key] {
			cut.notes = append(cut.notes, fmt.Sprintf("repeated, keeping the last: %s %+v", rec.label(), key))
		}
		ran[key] = true
	}
	if err := scanner.Err(); err != nil {
		return cut, err
	}

	for _, key := range order {
		label := labels[key]
		drop, dropped := droppedThreatRecords[key]
		switch {
		case dropped && !drop.recapture:
			cut.notes = append(cut.notes, fmt.Sprintf("dropped: %s %+v: %s", label, key, drop.reason))
			continue
		case dropped:
			cut.notes = append(cut.notes, fmt.Sprintf("back in the capture: %s %+v. Take its entry out of "+
				"droppedThreatRecords and its row out of the fixture README", label, key))
		}
		cut.lines = append(cut.lines, last[key])
	}
	return cut, nil
}

func removeThreatKey(keys []threatKey, key threatKey) []threatKey {
	for i, k := range keys {
		if k == key {
			return append(keys[:i:i], keys[i+1:]...)
		}
	}
	return keys
}

// readThreatFixtureLines gives a fixture's lines and its line ending, "\n" for a file that isn't there yet.
func readThreatFixtureLines(path string) (lines []string, newline string, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "\n", nil
	}
	if err != nil {
		return nil, "", err
	}
	newline = "\n"
	// a checkout with core.autocrlf has the fixture in CRLF
	if bytes.Contains(data, []byte("\r\n")) {
		newline = "\r\n"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimRight(line, "\r"); strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines, newline, nil
}

func runThreatFixture(args []string) int {
	flags := flag.NewFlagSet("simval threat", flag.ExitOnError)
	// no default: the e2e deletes its bots' records from the worldserver's simval.jsonl when they finish
	recordsPath := flags.String("records", "", "raw simval records streamed during a TestSimvalThreat run")
	since := flags.Int64("since", 0, "leave out records older than this unix time in ms, the run's start")
	merge := flags.Bool("merge", false, "the stream is a part of a run: keep the fixture's other records")
	out := flags.String("out", "", "the fixture to write (default the checkout's "+fixtureDir+"/"+threatFixture+")")
	_ = flags.Parse(args)
	if *recordsPath == "" {
		fmt.Fprintln(os.Stderr, "simval threat: -records is required")
		return 2
	}
	if *out == "" {
		root, err := checkoutRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		*out = filepath.Join(root, fixtureDir, threatFixture)
	}

	old, newline, err := readThreatFixtureLines(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !*merge {
		old = nil
	}
	f, err := os.Open(*recordsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer f.Close()
	cut, err := cutThreatFixture(f, *since, old)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", *recordsPath, err)
		return 1
	}
	for _, note := range cut.notes {
		fmt.Println(note)
	}
	if cut.threat == 0 {
		fmt.Fprintf(os.Stderr, "%s has no threat record from %d on; %s is left as it was\n", *recordsPath, *since, *out)
		return 1
	}
	if err := os.WriteFile(*out, []byte(strings.Join(cut.lines, newline)+newline), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("%d distinct lines, %d threat records from %d on, %d kept in %s\n", cut.read, cut.threat, *since,
		len(cut.lines), *out)
	return 0
}
