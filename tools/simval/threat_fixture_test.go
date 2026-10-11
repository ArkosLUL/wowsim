package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// committedThreatLines are the fixture's lines, CR stripped.
func committedThreatLines(t *testing.T) []string {
	t.Helper()
	lines, _, err := readThreatFixtureLines(filepath.Join("..", "..", fixtureDir, threatFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Skipf("no capture at %s", threatFixture)
	}
	return lines
}

// threatLineWith is a record line with change made to its JSON.
func threatLineWith(t *testing.T, line string, change func(rec map[string]any)) string {
	t.Helper()
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatal(err)
	}
	change(rec)
	out, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func threatLineAt(t *testing.T, line string, unixMs int64) string {
	return threatLineWith(t, line, func(rec map[string]any) { rec["unixMs"] = unixMs })
}

func threatKeyOfLine(t *testing.T, line string) threatKey {
	t.Helper()
	rec, wanted, err := threatLineKey(line, 0)
	if err != nil || !wanted {
		t.Fatalf("not a threat record (%v): %.80s", err, line)
	}
	return rec.key()
}

func cutThreatLines(t *testing.T, sinceMs int64, old []string, stream ...string) threatCut {
	t.Helper()
	cut, err := cutThreatFixture(strings.NewReader(strings.Join(stream, "\r\n")+"\r\n"), sinceMs, old)
	if err != nil {
		t.Fatal(err)
	}
	return cut
}

// The committed fixture came out of the last scratch script, so cutting it must give it back line for line.
func TestThreatFixtureCutGivesTheCommittedFixtureBack(t *testing.T) {
	lines := committedThreatLines(t)
	cut := cutThreatLines(t, 0, nil, lines...)
	if !slices.Equal(cut.lines, lines) {
		t.Errorf("cutting the fixture kept %d of its %d lines, or moved some", len(cut.lines), len(lines))
	}
	if len(cut.notes) > 0 {
		t.Errorf("cutting the fixture noted %q", cut.notes)
	}
	if cut.read != len(lines) || cut.threat != len(lines) {
		t.Errorf("read %d lines, %d threat records, want %d of each", cut.read, cut.threat, len(lines))
	}
}

func TestThreatFixtureCutRules(t *testing.T) {
	lines := committedThreatLines(t)
	a, b, c := threatLineAt(t, lines[0], 1000), threatLineAt(t, lines[1], 2000), threatLineAt(t, lines[2], 3000)
	early := threatLineAt(t, lines[3], 999)
	again := threatLineAt(t, lines[0], 4000)
	other := threatLineWith(t, threatLineAt(t, lines[4], 5000), func(rec map[string]any) { rec["command"] = "info" })

	// a restarted tail writes its first lines again; the last record of a repeated probe goes last
	cut := cutThreatLines(t, 1000, nil, early, a, b, a, "", c, other, again)
	if want := []string{b, c, again}; !slices.Equal(cut.lines, want) {
		t.Errorf("kept %d lines in an order other than the second, third and repeated first probe's", len(cut.lines))
	}
	if cut.read != 6 || cut.threat != 4 {
		t.Errorf("read %d distinct lines with %d threat records in time, want 6 and 4", cut.read, cut.threat)
	}
	if len(cut.notes) != 1 || !strings.HasPrefix(cut.notes[0], "repeated") {
		t.Errorf("notes %q, want the repeated probe alone", cut.notes)
	}

	if _, err := cutThreatFixture(strings.NewReader(a+"\n{\"command\":\"threat\"\n"), 0, nil); err == nil ||
		!strings.Contains(err.Error(), "line 2") {
		t.Errorf("a broken line gave %v, want an error naming line 2", err)
	}
}

func TestThreatFixtureCutLeavesDroppedProbesOut(t *testing.T) {
	lines := committedThreatLines(t)
	// a death knight's record in Frost Presence, turned into the dropped Chains of Ice probe
	dropped := threatKey{deathKnight, 48263, 45524, 0, 0}
	if droppedThreatRecords[dropped].reason == "" || droppedThreatRecords[dropped].recapture {
		t.Fatalf("%+v is no longer a plain drop: pick another", dropped)
	}
	i := slices.IndexFunc(lines, func(line string) bool {
		k := threatKeyOfLine(t, line)
		return k.class == deathKnight && k.form == 48263 && k.override == 0 && k.creatures == 0
	})
	if i < 0 {
		t.Fatal("the fixture has no plain Frost Presence record")
	}
	chains := threatLineWith(t, lines[i], func(rec map[string]any) { rec["spell"].(map[string]any)["id"] = 45524 })
	if got := threatKeyOfLine(t, chains); got != dropped {
		t.Fatalf("the edited record's key is %+v, want %+v", got, dropped)
	}

	cut := cutThreatLines(t, 0, nil, lines[0], chains, lines[1])
	if want := []string{lines[0], lines[1]}; !slices.Equal(cut.lines, want) {
		t.Errorf("kept %d lines, want the two that aren't dropped", len(cut.lines))
	}
	if len(cut.notes) != 1 || !strings.Contains(cut.notes[0], "the sim has no Chains of Ice") {
		t.Errorf("notes %q, want the drop with its reason", cut.notes)
	}

	// a drop the next capture is to bring back is kept, with a note to take its entry out
	back := threatKeyOfLine(t, lines[0])
	if _, had := droppedThreatRecords[back]; had {
		t.Fatalf("%+v is dropped and in the fixture", back)
	}
	droppedThreatRecords[back] = threatDrop{reason: "lost in the last capture", recapture: true}
	t.Cleanup(func() { delete(droppedThreatRecords, back) })
	cut = cutThreatLines(t, 0, nil, lines[0], lines[1])
	if want := []string{lines[0], lines[1]}; !slices.Equal(cut.lines, want) {
		t.Errorf("kept %d lines, want both: the recapture drop's record is back", len(cut.lines))
	}
	if len(cut.notes) != 1 || !strings.Contains(cut.notes[0], "back in the capture") {
		t.Errorf("notes %q, want the record that's back", cut.notes)
	}
}

func TestThreatFixtureCutMergesPartOfARun(t *testing.T) {
	lines := committedThreatLines(t)
	old := lines[:3]
	rerun, added := threatLineAt(t, lines[1], 9000), threatLineAt(t, lines[3], 9100)

	cut := cutThreatLines(t, 0, old, added, rerun)
	if want := []string{lines[0], rerun, lines[2], added}; !slices.Equal(cut.lines, want) {
		t.Errorf("kept %d lines, want the fixture's three with the second replaced in place, then the new probe",
			len(cut.lines))
	}
	if len(cut.notes) > 0 {
		t.Errorf("notes %q, want none: replacing the fixture's record isn't a repeat", cut.notes)
	}
}

func TestThreatFixtureCommand(t *testing.T) {
	lines := committedThreatLines(t)
	dir := t.TempDir()
	stream, out := filepath.Join(dir, "stream.jsonl"), filepath.Join(dir, threatFixture)
	// the fixture as a core.autocrlf checkout holds it
	if err := os.WriteFile(out, []byte(strings.Join(lines[:2], "\r\n")+"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stream, []byte(strings.Join(lines[2:5], "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := runThreatFixture([]string{"-records", stream, "-out", out}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Join(lines[2:5], "\r\n") + "\r\n"; string(data) != want {
		t.Errorf("the fixture isn't the stream's three records in the old file's CRLF")
	}

	if code := runThreatFixture([]string{"-records", stream, "-out", out, "-since", "99999999999999"}); code != 1 {
		t.Errorf("exit %d for a stream with nothing that late, want 1", code)
	}
	if after, _ := os.ReadFile(out); string(after) != string(data) {
		t.Errorf("a stream with no record in time rewrote the fixture")
	}
}
