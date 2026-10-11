package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

func init() {
	sim.RegisterAll()
}

func testRoot(t *testing.T) string {
	t.Helper()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

var goldenDPS = regexp.MustCompile(`key: "([^"]+)"\r?\n value: \{\r?\n  dps: ([0-9.eE+-]+)`)

// goldenRows is the DPS of each test in a .results golden, by its name without the suite's.
func goldenRows(t *testing.T, root, suite string) map[string]float64 {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, suite+".results"))
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]float64{}
	for _, m := range goldenDPS.FindAllSubmatch(data, -1) {
		dps, err := strconv.ParseFloat(string(m[2]), 64)
		if err != nil {
			t.Fatal(err)
		}
		rows[strings.TrimPrefix(string(m[1]), filepath.Base(suite)+"-")] = dps
	}
	return rows
}

func dpsOf(t *testing.T, r row) float64 {
	t.Helper()
	r.rsr.SimOptions = simOptions(r.rsr.SimOptions, 0, 0)
	result := core.RunRaidSim(r.rsr)
	if result.ErrorResult != "" {
		t.Fatalf("%s: %s", r.label, result.ErrorResult)
	}
	return player(result).Dps.Avg
}

// A preset is a copy of its suite's character, so its rows have to be the golden's, to the digit. One that
// drifted gives rows no golden holds, or another DPS.
func TestPresetsRunTheirSuitesRows(t *testing.T) {
	root := testRoot(t)
	for _, name := range presetNames() {
		t.Run(name, func(t *testing.T) {
			p := presets[name]
			golden := goldenRows(t, root, p.suite)
			rows := presetRows(p, root, nil, "")
			want := 0
			for label := range golden {
				if strings.HasPrefix(label, "Settings-") || strings.HasPrefix(label, "Average-") {
					want++
				}
			}
			if len(rows) != want {
				t.Errorf("%d rows, %s.results has %d: copy the suite's races, options and rotations again", len(rows), p.suite, want)
			}
			// a 20-iteration row per preset, and one Average, which takes seconds
			check := []row{rows[0]}
			if name == "frostuh" {
				check = append(check, rows[len(rows)-1])
			}
			for _, r := range check {
				dps, ok := golden[r.label]
				if !ok {
					t.Errorf("%s.results has no %s", p.suite, r.label)
					continue
				}
				if got := dpsOf(t, r); math.Abs(got-dps) > 1e-4 {
					t.Errorf("%s: %.5f DPS, the golden's %.5f. If %s fails too, its golden moved: promote it. If not, "+
						"copy the suite's gear, talents, glyphs, options and consumes again", r.label, got, dps, filepath.Base(p.suite))
				}
			}
		})
	}
}

func TestAplAndGearReplaceTheSuites(t *testing.T) {
	root := testRoot(t)
	apl, err := loadRotation(filepath.Join(root, "ui/deathknight/apls/uh_dnd_aoe.apl.json"))
	if err != nil {
		t.Fatal(err)
	}
	// TestUnholy runs three rotations on one gear set: 12 rows each, and the Average
	rows := presetRows(presets["unholy"], root, &apl, "p3_bis_unholy")
	if len(rows) != 13 {
		t.Fatalf("%d rows, want one rotation's 12 and the Average", len(rows))
	}
	for _, r := range rows[:12] {
		if !strings.Contains(r.label, "-p3_bis_unholy-Basic-uh_dnd_aoe-") {
			t.Errorf("row %s isn't on the gear set and rotation asked for", r.label)
		}
	}
	average := rows[12].rsr.Raid.Parties[0].Players[0]
	if rows[12].label != "Average-Default" || !googleProto.Equal(average.Rotation, apl.Rotation) {
		t.Errorf("the last row is %s on another rotation than the one asked for", rows[12].label)
	}
}

func TestPickRow(t *testing.T) {
	rows := []row{{label: "Settings-Orc-p3_frost-Basic-frost_uh_pesti-NoBuffs-LongMultiTarget"},
		{label: "Settings-Human-p3_frost-Basic-frost_uh_pesti-NoBuffs-LongMultiTarget"}, {label: "Average-Default"}}
	if r, err := pickRow(rows, "orc, nobuffs,MULTI"); err != nil || r.label != rows[0].label {
		t.Errorf("picked %q (%v), want the orc row", r.label, err)
	}
	if r, err := pickRow(rows, "Average"); err != nil || r.label != "Average-Default" {
		t.Errorf("picked %q (%v), want Average", r.label, err)
	}
	if _, err := pickRow(rows, "nobuffs"); err == nil || !strings.Contains(err.Error(), "fits 2 rows") {
		t.Errorf("two rows fit and the error is %v", err)
	}
	if r, err := pickRow(rows[:1], "Average"); err != nil || r.label != rows[0].label {
		t.Errorf("picked %q (%v), want a lone row whatever -row says", r.label, err)
	}
}

func TestRequestFileShapes(t *testing.T) {
	root := testRoot(t)
	suite := presetRows(presets["blood"], root, nil, "")
	want := suite[len(suite)-1].rsr
	player := want.Raid.Parties[0].Players[0]
	apl, err := loadRotation(filepath.Join(root, "ui/deathknight/apls/blood_pesti_aoe.apl.json"))
	if err != nil {
		t.Fatal(err)
	}

	settings := &proto.IndividualSimSettings{Player: player, PartyBuffs: want.Raid.Parties[0].Buffs,
		RaidBuffs: want.Raid.Buffs, Debuffs: want.Raid.Debuffs, Encounter: want.Encounter}
	for shape, message := range map[string]googleProto.Message{"a request": want, "exported settings": settings} {
		data, err := protojson.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "request.json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
		rows, err := requestRows(path, &apl)
		if err != nil {
			t.Fatalf("%s: %v", shape, err)
		}
		got := rows[0].rsr.Raid.Parties[0].Players[0]
		if len(rows) != 1 || got.TalentsString != player.TalentsString || !slices.Equal(itemIDs(got), itemIDs(player)) {
			t.Errorf("%s: read another character than the one written", shape)
		}
		if !googleProto.Equal(got.Rotation, apl.Rotation) {
			t.Errorf("%s: -apl didn't replace the request's rotation", shape)
		}
		if rows[0].rsr.Encounter.GetDuration() != want.Encounter.Duration {
			t.Errorf("%s: encounter of %v s, want %v", shape, rows[0].rsr.Encounter.GetDuration(), want.Encounter.Duration)
		}
	}

	if _, err := parseRequest([]byte(`{"simOptions":{"iterations":5}}`)); err == nil {
		t.Error("a request without a player parsed")
	}
}

func itemIDs(player *proto.Player) []int32 {
	var ids []int32
	for _, item := range player.Equipment.GetItems() {
		ids = append(ids, item.Id)
	}
	return ids
}

func TestSimOptions(t *testing.T) {
	own := &proto.SimOptions{Iterations: 20, IsTest: true, RandomSeed: 101, Debug: true}
	if got := simOptions(own, 0, 0); got.Iterations != 20 || got.RandomSeed != 101 || got.Debug || !own.Debug {
		t.Errorf("a row's own options came back as %v, or changed in place", got)
	}
	if got := simOptions(own, 500, 7); got.Iterations != 500 || got.RandomSeed != 7 {
		t.Errorf("the flags gave %v, want 500 iterations on seed 7", got)
	}
	if got := simOptions(nil, 0, 0); got.Iterations != 1000 || got.RandomSeed != 101 || !got.IsTest {
		t.Errorf("a request without options got %v", got)
	}
}

func timelineOf(log, window string, ids []string, match string) ([]string, error) {
	cut, err := newTimelineCut(window, ids, match)
	if err != nil {
		return nil, err
	}
	return cut.lines(log), nil
}

func TestTimeline(t *testing.T) {
	log := strings.Join([]string{
		"[-1.00] [ (#1)] Casting {SpellID: 48266} (Cost = 0.000, Cast Time = 0s, Effective Time = 0s)",
		"[0.00] [Target 1] Aura gained: {SpellID: 55095}",
		"[0.00] [ (#1)] Casting {SpellID: 49909} (Cost = 1.000, Cast Time = 0s, Effective Time = 1.5s)",
		"[0.00] [ (#1)] Spent 1.000 frost rune from {SpellID: 49909} (1 --> 0).",
		"[0.50] [ (#1)] Casting {OtherID: 3, Tag: 1} (Cost = 0.000, Cast Time = 0s, Effective Time = 0s)",
		"[1.49] [ (#1)] No available actions!",
		"[2.50] [ (#1)] Aura gained: {SpellID: 45529}",
		"[2.50] [ (#1)] Aura gained: {SpellID: 51271}",
		"[2.60] [ (#1) - Ghoul] Casting {SpellID: 47468} (Cost = 40.000, Cast Time = 0s, Effective Time = 0s)",
		"[9.00] [ (#1)] Casting {SpellID: 51425, Tag: 1} (Cost = 2.000, Cast Time = 0s, Effective Time = 1.5s)",
		"not a log line",
	}, "\r\n")

	got, err := timelineOf(log, "0:5", []string{"45529"}, "rune from")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"[0.00] Casting {SpellID: 49909}",
		"[0.00] Spent 1.000 frost rune from {SpellID: 49909} (1 --> 0).",
		"[1.49] No available actions!",
		"[2.50] Aura gained: {SpellID: 45529}",
	}
	if !slices.Equal(got, want) {
		t.Errorf("timeline\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}

	if all, err := timelineOf(log, ":", nil, ""); err != nil || len(all) != 4 {
		t.Errorf("the whole fight has %d lines (%v), want the three casts and the idle one", len(all), err)
	}
	for _, bad := range []string{"5", "a:b"} {
		if _, err := timelineOf(log, bad, nil, ""); err == nil {
			t.Errorf("-timeline %q went through", bad)
		}
	}
	if _, err := timelineOf(log, ":", nil, "("); err == nil {
		t.Error("a broken -match went through")
	}
}

func TestRunPrintsRowsAndRefusesBadFlags(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"-spec", "nope"}, &out); code != 1 {
		t.Errorf("an unknown -spec exits %d, want 1", code)
	}
	if code := run(nil, &out); code != 1 {
		t.Errorf("no character exits %d, want 1", code)
	}
	if code := run([]string{"-spec", "blood", "-request", "x.json"}, &out); code != 1 {
		t.Errorf("-spec with -request exits %d, want 1", code)
	}
	if code := run([]string{"-spec", "blood", "-timeline", "5"}, &out); code != 1 {
		t.Errorf("a broken -timeline exits %d, want 1", code)
	}
	broken := filepath.Join(t.TempDir(), "broken.apl.json")
	if err := os.WriteFile(broken, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := run([]string{"-spec", "blood", "-apl", broken}, &out); code != 1 {
		t.Errorf("a broken -apl file exits %d, want 1", code)
	}
	// a refusal comes before any sim runs
	if out.Len() != 0 {
		t.Errorf("the refusals printed %q", out.String())
	}

	if code := run([]string{"-spec", "blood", "-row", "orc,nobuffs,short", "-iters", "2", "-ids", "49924", "-timeline", "0:3"}, &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	text := out.String()
	if !strings.Contains(text, "Settings-Orc-p3_blood-Basic-blood_dps-NoBuffs-ShortSingleTarget iters=2 seed=101: dps ") {
		t.Errorf("no header for the picked row in\n%s", text)
	}
	if !strings.Contains(text, "] Casting {") {
		t.Errorf("no timeline in\n%s", text)
	}
}
