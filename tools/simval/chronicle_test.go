package main

import (
	"io"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

const sampleLog = "testdata/sample_chronicle.log"

func loadSample(t *testing.T) (*chronicleLog, *runStats) {
	t.Helper()
	log, err := readChronicleLog(sampleLog)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := analyzeChronicle(log, chronicleOptions{GapSec: 5})
	if err != nil {
		t.Fatal(err)
	}
	return log, stats
}

func TestChronicleParsesHeaderAndUnits(t *testing.T) {
	log, _ := loadSample(t)

	if log.Zone != "Naxxramas" || log.MapID != 533 || log.Instance != 42 {
		t.Errorf("zone %q map %d instance %d", log.Zone, log.MapID, log.Instance)
	}
	if owner := log.Owners["0xF14000000000000B"]; owner != "0x0000000000000001" {
		t.Errorf("pet owner = %q", owner)
	}
	if _, ok := log.Owners["0xF13000000000000A"]; ok {
		t.Errorf("the dummy has an empty owner guid and should not be in Owners")
	}
	// The dummy's name holds a comma, which only splits correctly inside quotes.
	if name := log.Names["0xF13000000000000A"]; name != "Boss Dummy, lvl 83" {
		t.Errorf("dummy name = %q", name)
	}
}

func TestChronicleMeasuresDamage(t *testing.T) {
	_, stats := loadSample(t)

	if stats.Source.Name != "Testpala" {
		t.Fatalf("source = %q, want Testpala", stats.Source.Name)
	}
	if len(stats.MinionNames) != 1 || stats.MinionNames[0] != "Testpet" {
		t.Errorf("minions = %v, want [Testpet]", stats.MinionNames)
	}
	// 4 swings of 1000/2000/1000/1000, Crusader Strike 3000, three 500 ticks and
	// the pet's 400.
	if stats.Damage != 9900 {
		t.Errorf("damage = %d, want 9900", stats.Damage)
	}
	if got := stats.durationSec(); math.Abs(got-6.4) > 1e-9 {
		t.Errorf("duration = %v s, want 6.4", got)
	}
	if got := stats.dps(); math.Abs(got-9900/6.4) > 1e-9 {
		t.Errorf("dps = %v, want %v", got, 9900/6.4)
	}
	// No pause is longer than the 5 s gap, so nothing is dropped.
	if stats.activeSec() != stats.durationSec() {
		t.Errorf("active %v s, duration %v s", stats.activeSec(), stats.durationSec())
	}
}

func TestChronicleSplitsAbilities(t *testing.T) {
	_, stats := loadSample(t)

	melee := stats.Abilities[abilityKey{}]
	if melee == nil {
		t.Fatal("no melee ability")
	}
	if melee.Hits != 4 || melee.Crits != 1 || melee.Damage != 5000 {
		t.Errorf("melee hits %d crits %d damage %d, want 4/1/5000", melee.Hits, melee.Crits, melee.Damage)
	}
	if melee.Misses["DODGE"] != 1 || melee.missCount() != 1 {
		t.Errorf("melee misses = %v", melee.Misses)
	}

	strike := stats.Abilities[abilityKey{SpellID: 35395}]
	if strike == nil {
		t.Fatal("no Crusader Strike")
	}
	if strike.Hits != 1 || strike.Damage != 3000 || strike.Absorbed != 150 || strike.Misses["MISS"] != 1 {
		t.Errorf("Crusader Strike %+v", strike)
	}

	vengeance := stats.Abilities[abilityKey{SpellID: 31803}]
	if vengeance == nil || vengeance.Ticks != 3 || vengeance.Hits != 0 || vengeance.Damage != 1500 {
		t.Errorf("Holy Vengeance %+v", vengeance)
	}

	pet := stats.Abilities[abilityKey{Minion: "Testpet"}]
	if pet == nil || pet.Hits != 1 || pet.Damage != 400 || pet.Name != "Testpet: Melee" {
		t.Errorf("pet melee %+v", pet)
	}
}

func TestChronicleSplitsOutcomes(t *testing.T) {
	_, stats := loadSample(t)

	melee := stats.Abilities[abilityKey{}].Outcomes
	if melee.Hit.N != 3 || melee.Hit.mean() != 1000 || melee.Crit.N != 1 || melee.Crit.mean() != 2000 || melee.Glance.N != 0 {
		t.Errorf("melee outcomes %+v", melee)
	}
	if rate, stdErr := melee.critRate(); rate != 0.25 || math.Abs(stdErr-math.Sqrt(0.25*0.75/4)) > 1e-12 {
		t.Errorf("melee crit rate %v ± %v, want 0.25 of 4 landed", rate, stdErr)
	}
	if melee.Avoided["dodge"] != 1 {
		t.Errorf("melee avoided %v", melee.Avoided)
	}

	strike := stats.Abilities[abilityKey{SpellID: 35395}].Outcomes
	if strike.Hit.N != 1 || strike.Hit.mean() != 3000 || strike.Avoided["miss"] != 1 {
		t.Errorf("Crusader Strike outcomes %+v", strike)
	}
}

func TestChronicleReadsSpellTargetMisses(t *testing.T) {
	for _, tc := range []struct {
		result string
		miss   bool
	}{{"PARRY", true}, {"HIT", false}} {
		line := `1790084304000  CHRONICLE_SPELL_TARGET_RESULT,0x0000000000083EE3,"Pala",0x528,` +
			`0xF1300F3E5800036F,"SimVal Boss Dummy",0xa28,35395,"Crusader Strike",0x1,"` + tc.result + `",nil,0x1`
		event, err := parseChronicleEvent(line)
		if err != nil {
			t.Fatal(err)
		}
		if event.isMiss() != tc.miss || event.Spell.ID != 35395 || event.Dst.Name != "SimVal Boss Dummy" || event.missType() != tc.result {
			t.Errorf("%s: miss %v, spell %d on %q, type %q", tc.result, event.isMiss(), event.Spell.ID, event.Dst.Name, event.missType())
		}
	}
}

func TestChronicleTimesSwingsAndTicks(t *testing.T) {
	_, stats := loadSample(t)

	// The pet's swing must not land in the player's swing timeline.
	if len(stats.SwingTimes) != 5 {
		t.Fatalf("%d swings, want 5", len(stats.SwingTimes))
	}
	for _, gap := range intervals(stats.SwingTimes) {
		if gap != 1600 {
			t.Errorf("swing interval %d ms, want 1600", gap)
		}
	}
	if got := histogram(intervals(stats.SwingTimes), 100); got != "1.6s:4" {
		t.Errorf("swing histogram = %q", got)
	}

	ticks := tickTimes(stats, 31803, "Boss Dummy, lvl 83")
	if len(ticks) != 3 {
		t.Fatalf("%d ticks, want 3", len(ticks))
	}
	if got := histogram(intervals(ticks), 100); got != "1.0s:2" {
		t.Errorf("tick histogram = %q", got)
	}
}

// tickTimes and auraOn find one spell's series on one target, the way the report
// prints them.
func tickTimes(stats *runStats, spellID int32, target string) []int64 {
	for _, series := range stats.TickTimes {
		if series.Key.Ability.SpellID == spellID && series.TargetName == target {
			return series.Times
		}
	}
	return nil
}

func auraOn(stats *runStats, spellID int32, target string) *auraStats {
	for _, aura := range stats.Auras {
		if aura.SpellID == spellID && aura.TargetName == target {
			return aura
		}
	}
	return nil
}

func TestChronicleTracksAuras(t *testing.T) {
	_, stats := loadSample(t)

	aura := auraOn(stats, 31884, "Testpala")
	if aura == nil {
		t.Fatal("Avenging Wrath missing")
	}
	if aura.Applied != 1 || aura.UptimeMs != 5000 {
		t.Errorf("Avenging Wrath applied %d, uptime %d ms; want 1 and 5000", aura.Applied, aura.UptimeMs)
	}
}

// A DoT on two mobs ticks on two timelines and holds two aura windows. Merged
// they would read as a tick every 0 ms and one window covering both.
func TestChronicleKeepsTargetsApart(t *testing.T) {
	const text = `1789900000000  CHRONICLE_ZONE_INFO,"Naxxramas",533,42,"raid","NORMAL"
1789900000000  CHRONICLE_UNIT_INFO,0x1,"Testlock",80,0x511,0x0000000000000000,30000,"MINE",false
1789900000000  CHRONICLE_UNIT_INFO,0xA,"Dummy",83,0xa28,0x0000000000000000,24009944,"NEUTRAL",true
1789900000000  CHRONICLE_UNIT_INFO,0xB,"Maggot",80,0xa28,0x0000000000000000,9000,"NEUTRAL",false
1789900001000  SPELL_AURA_APPLIED,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47813,"Corruption",0x20,DEBUFF
1789900001000  SPELL_AURA_APPLIED,0x1,"Testlock",0x511,0xB,"Maggot",0xa28,47813,"Corruption",0x20,DEBUFF
1789900004000  SPELL_PERIODIC_DAMAGE,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47813,"Corruption",0x20,300,0,32,0,0,0,nil,nil,nil
1789900004000  SPELL_PERIODIC_DAMAGE,0x1,"Testlock",0x511,0xB,"Maggot",0xa28,47813,"Corruption",0x20,300,0,32,0,0,0,nil,nil,nil
1789900007000  SPELL_PERIODIC_DAMAGE,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47813,"Corruption",0x20,300,0,32,0,0,0,nil,nil,nil
1789900007000  SPELL_PERIODIC_DAMAGE,0x1,"Testlock",0x511,0xB,"Maggot",0xa28,47813,"Corruption",0x20,300,0,32,0,0,0,nil,nil,nil
1789900008000  SPELL_AURA_REMOVED,0x1,"Testlock",0x511,0xB,"Maggot",0xa28,47813,"Corruption",0x20,DEBUFF
`
	log, err := parseChronicleLog("test", strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	stats, err := analyzeChronicle(log, chronicleOptions{GapSec: 5})
	if err != nil {
		t.Fatal(err)
	}

	// One ability row for the two targets: the damage still adds up.
	if corruption := stats.Abilities[abilityKey{SpellID: 47813}]; corruption == nil || corruption.Ticks != 4 {
		t.Errorf("Corruption = %+v, want 4 ticks", corruption)
	}
	for _, target := range []string{"Dummy", "Maggot"} {
		gaps := intervals(tickTimes(stats, 47813, target))
		if len(gaps) != 1 || gaps[0] != 3000 {
			t.Errorf("%s tick intervals = %v ms, want [3000]", target, gaps)
		}
	}

	// The Maggot's aura was removed at 8 s; the dummy's ran to the last tick.
	if aura := auraOn(stats, 47813, "Maggot"); aura == nil || aura.UptimeMs != 7000 {
		t.Errorf("Corruption on the Maggot = %+v, want 7000 ms uptime", aura)
	}
	if aura := auraOn(stats, 47813, "Dummy"); aura == nil || aura.UptimeMs != 6000 {
		t.Errorf("Corruption on the dummy = %+v, want 6000 ms uptime", aura)
	}
}

func TestChronicleDPSGap(t *testing.T) {
	_, stats := loadSample(t)

	for _, tc := range []struct {
		name   string
		sim    float64
		failed int
	}{
		{"within 2%", stats.dps() * 1.01, 0},
		{"over 2%", stats.dps() * 1.05, 1},
		{"under 2%", stats.dps() * 0.9, 1},
		{"no sim number", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := printDPSGap(io.Discard, stats, chronicleOptions{SimDPS: tc.sim}); got != tc.failed {
				t.Errorf("failed = %d, want %d", got, tc.failed)
			}
		})
	}
}

func TestChroniclePauseLeavesActiveDuration(t *testing.T) {
	log, err := readChronicleLog(sampleLog)
	if err != nil {
		t.Fatal(err)
	}
	// Every gap in the sample is under a second, so a sub-second threshold turns
	// them all into pauses and leaves only the events themselves.
	stats, err := analyzeChronicle(log, chronicleOptions{GapSec: 0.4})
	if err != nil {
		t.Fatal(err)
	}
	if stats.activeSec() >= stats.durationSec() {
		t.Errorf("active %v s is not shorter than the %v s window", stats.activeSec(), stats.durationSec())
	}
}

func TestChronicleRejectsGarbage(t *testing.T) {
	for _, text := range []string{
		"not a chronicle line at all\n",
		"abc  SWING_DAMAGE,1,2,3\n",
		"1789900001000  SWING_DAMAGE,0x1,\"a\",0x511\n",
	} {
		if _, err := parseChronicleLog("test", strings.NewReader(text)); err == nil {
			t.Errorf("%q parsed without an error", text)
		}
	}
}

// TestChronicleCaptures runs every captured run through the parser, so a capture
// whose format drifted fails here rather than in a comparison.
func TestChronicleCaptures(t *testing.T) {
	dir := filepath.Join("..", "..", chronicleDir)
	logs, err := filepath.Glob(filepath.Join(dir, "*.log.gz"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		t.Fatal(err)
	}
	logs = append(logs, raw...)
	if len(logs) == 0 {
		t.Skipf("no captured runs in %s", dir)
	}

	for _, path := range logs {
		t.Run(filepath.Base(path), func(t *testing.T) {
			log, err := readChronicleLog(path)
			if err != nil {
				t.Fatal(err)
			}
			stats, err := analyzeChronicle(log, chronicleOptions{Source: sourceFor(path), GapSec: 5})
			if err != nil {
				t.Fatal(err)
			}
			if stats.Damage <= 0 || stats.durationSec() <= 0 {
				t.Fatalf("%s: %d damage over %v s", path, stats.Damage, stats.durationSec())
			}
			t.Logf("%s: %s, %.1f DPS over %.1f s", filepath.Base(path), stats.Source.Name,
				stats.dps(), stats.durationSec())
		})
	}
}

// sourceFor reads the player name out of a capture's file name,
// `<spec>_<player>_<unix>.log`, so a log that also holds other players still
// measures the right one.
func sourceFor(path string) string {
	name := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".gz"), ".log")
	parts := strings.Split(name, "_")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}
