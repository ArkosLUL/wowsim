package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
)

const (
	botPlayerGUID = "0x0000000000000001"
	botPlayer     = `0x0000000000000001,"Botfight",0x511`
	botDummy      = `0xF130000000000002,"SimVal Boss Dummy",0xa28`
	botLogBaseMs  = 1790000000000
	// the bot's log names a sim action that has only an item by this plus the item id, and aliases it back
	botItemSpellBase = 9000000
	// the bot stops this long before the fight's end, so its last hardcast lands inside it
	botStopEarly = 5 * time.Second
)

func fightCyclesForTest(t *testing.T) []fightCycle {
	t.Helper()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	cycles, err := readFightCycles(filepath.Join(root, fightsDir, fightCyclesFile))
	if err != nil {
		t.Fatal(err)
	}
	return cycles
}

// botLog writes what happens in a sim fight as Chronicle lines.
type botLog struct {
	lines []botLine
	pets  map[string]string // sim pet name to GUID
}

type botLine struct {
	at   time.Duration
	text string
}

func (b *botLog) add(at time.Duration, format string, args ...any) {
	b.lines = append(b.lines, botLine{at, fmt.Sprintf(format, args...)})
}

func (b *botLog) unit(minion string) string {
	if minion == "" {
		return botPlayer
	}
	return fmt.Sprintf(`%s,"%s",0x1111`, b.pets[minion], minion)
}

func (b *botLog) on(on string) string {
	switch on {
	case "self":
		return botPlayer
	case "target":
		return botDummy
	}
	return b.unit(on)
}

// botSpell is the log's spell id for a sim action: its own, or one botItemSpellBase stands in for, aliased back.
func botSpell(spell, item int32) int32 {
	if item == 0 {
		return spell
	}
	id := botItemSpellBase + item
	fightSimActions[id] = core.ActionID{ItemID: item}
	return id
}

func logBool(on bool) string {
	if on {
		return "1"
	}
	return "nil"
}

func (b *botLog) damage(at time.Duration, key fightKey, periodic bool, result *core.SpellResult) {
	spell := botSpell(key.Spell, key.Item)
	src := b.unit(key.Minion)
	if !result.Landed() {
		kind := strings.ToUpper(simAvoidance(result.Outcome))
		if spell == 0 {
			b.add(at, "SWING_MISSED,%s,%s,%s,nil,0", src, botDummy, kind)
		} else {
			b.add(at, `CHRONICLE_SPELL_TARGET_RESULT,%s,%s,%d,"S%d",0x1,"%s",nil,0x1`, src, botDummy, spell, spell, kind)
		}
		return
	}
	suffix := fmt.Sprintf("%d,0,1,0,%d,0,%s,%s,nil", int64(math.Round(result.Damage)),
		map[bool]int{false: 0, true: 1}[result.Outcome.Matches(core.OutcomeBlock)], logBool(result.DidCrit()),
		logBool(result.Outcome.Matches(core.OutcomeGlance)))
	switch {
	case spell == 0:
		b.add(at, "SWING_DAMAGE,%s,%s,%s", src, botDummy, suffix)
	case periodic:
		b.add(at, `SPELL_PERIODIC_DAMAGE,%s,%s,%d,"S%d",0x1,%s`, src, botDummy, spell, spell, suffix)
	default:
		b.add(at, `SPELL_DAMAGE,%s,%s,%d,"S%d",0x1,%s`, src, botDummy, spell, spell, suffix)
	}
	if spell == autoShotSpell && key.Minion == "" {
		b.add(at, `SPELL_CAST_SUCCESS,%s,%s,75,"Auto Shot",0x1`, botPlayer, botDummy)
	}
}

func (b *botLog) aura(at time.Duration, key fightAuraKey, on bool) {
	event := "SPELL_AURA_REMOVED"
	if on {
		event = "SPELL_AURA_APPLIED"
	}
	spell := botSpell(key.Spell, key.Item)
	src := botPlayer
	if key.On != "self" && key.On != "target" {
		src = b.unit(key.On)
	}
	b.add(at, `%s,%s,%s,%d,"S%d",0x1,BUFF`, event, src, b.on(key.On), spell, spell)
}

func (b *botLog) text() string {
	sort.SliceStable(b.lines, func(i, j int) bool { return b.lines[i].at < b.lines[j].at })
	var out strings.Builder
	head := botLogBaseMs - 5000
	fmt.Fprintf(&out, "%d  CHRONICLE_HEADER,\"AzerothCore\",\"3.3.5a\",12340\n", head)
	fmt.Fprintf(&out, "%d  CHRONICLE_ZONE_INFO,\"Naxxramas\",533,1,\"raid\",\"NORMAL\"\n", head)
	for name, guid := range b.pets {
		fmt.Fprintf(&out, "%d  CHRONICLE_UNIT_INFO,%s,\"%s\",80,0x1111,%s,10000,\"FRIENDLY\",false\n", head, guid, name, botPlayerGUID)
	}
	for _, l := range b.lines {
		fmt.Fprintf(&out, "%d  %s\n", botLogBaseMs+l.at.Milliseconds(), l.text)
	}
	return out.String()
}

var logPowerTypes = map[string]uint32{"mana": 0, "rage": 1, "focus": 2, "energy": 3, "runic power": 6}

// botFight runs the setup's cycle in the sim the way the e2e's bot does, each step tried in turn waitMs
// apart and a refusal passed over, and writes what came of it as the fight's Chronicle log.
func botFight(t *testing.T, s goldcharSetup, c fightCycle, seed int64) string {
	t.Helper()
	end := time.Duration(c.Seconds) * time.Second
	fl := &fightLog{setup: c.Setup, cycle: c, endMs: end.Milliseconds(), names: map[int32]string{}}
	if c.melee() || c.Ammo != 0 {
		fl.hasAutos, fl.autos = true, [2]time.Duration{0, end}
	}
	f, err := newFightSim(s, fl)
	if err != nil {
		t.Fatal(err)
	}
	b := &botLog{pets: map[string]string{}}
	for i, pet := range f.d.char.Pets {
		fl.minions = append(fl.minions, pet.Name)
		b.pets[pet.Name] = fmt.Sprintf("0xF1400000000000%02X", i+16)
	}

	// power goes in the log when something else happens, a little late but inside the fight, a line per gain:
	// gains since the last line share what they gave
	given := map[fightEnergizeKey]gainCount{}
	energize := func(at time.Duration) {
		for key, total := range f.powerGainCounts() {
			was := given[key]
			amount, n := total.amount-was.amount, total.events-was.events
			// a gain of nothing (Thrill of the Hunt off a free shot) counts an event but writes no line: use it up
			// here, or the next real gain gets split over it
			given[key] = total
			if amount <= 0.5 || n <= 0 {
				continue
			}
			power := logPowerTypes[key.Power]
			spell := botSpell(key.Spell, key.Item)
			for range n {
				b.add(at, `SPELL_ENERGIZE,%s,%s,%d,"S%d",0x1,%d,%d`, botPlayer, botPlayer, spell, spell,
					int64(math.Round(amount/float64(n)*powerScale(power))), power)
			}
		}
	}
	f.onDamage = func(sim *core.Simulation, key fightKey, periodic bool, result *core.SpellResult) {
		b.damage(sim.CurrentTime, key, periodic, result)
		energize(sim.CurrentTime)
	}
	f.onAura = func(sim *core.Simulation, key fightAuraKey, on bool) { b.aura(sim.CurrentTime, key, on) }

	steps := slices.Concat(c.Before, c.Cycle)
	f.runIteration(seed, func(sim *core.Simulation) {
		// what's up at the pull went on before the fight
		addAction(sim, 0, core.ActionPriorityPrePull-2, func(sim *core.Simulation) {
			for key, n := range f.auraUp {
				if n > 0 {
					b.aura(-time.Second, key, true)
				}
			}
		})
		next := 0
		var step func(sim *core.Simulation)
		step = func(sim *core.Simulation) {
			if sim.CurrentTime > end-botStopEarly {
				return
			}
			k := next
			if k >= len(steps) {
				k = len(c.Before) + (k-len(c.Before))%len(c.Cycle)
			}
			next++
			st := steps[k]
			if spell := f.spells[st.Spell]; spell != nil {
				if ok, _ := f.cast(sim, spell); ok {
					b.add(sim.CurrentTime+spell.CurCast.CastTime, `SPELL_CAST_SUCCESS,%s,%s,%d,"S%d",0x1`,
						botPlayer, botDummy, st.Spell, st.Spell)
					energize(sim.CurrentTime)
				}
			}
			addAction(sim, sim.CurrentTime+time.Duration(st.WaitMs)*time.Millisecond, castPriority, step)
		}
		addAction(sim, 0, castPriority, step)
	})

	return b.text()
}

func writeFightFile(t *testing.T, name, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func keepFightAliases(t *testing.T) {
	t.Helper()
	saved := fightSimActions
	fightSimActions = map[int32]core.ActionID{}
	for k, v := range saved {
		fightSimActions[k] = v
	}
	t.Cleanup(func() { fightSimActions = saved })
}

func replayBotFight(t *testing.T, s goldcharSetup, c fightCycle, text string, iterations int) *fightComparison {
	t.Helper()
	fl, err := readFightLog(writeFightFile(t, s.Name+"_Botfight_1.log", text), c, "")
	if err != nil {
		t.Fatal(err)
	}
	cmp, err := replayFight(s, fl, iterations)
	if err != nil {
		t.Fatal(err)
	}
	return cmp
}

// The replay agrees with a fight the sim itself fought: every cycle runs, every cast it landed lands again on
// time, and the rest stays inside the sim's own spread.
func TestFightReplaysTheBotsOwnFight(t *testing.T) {
	needsItemDB(t)
	keepFightAliases(t)
	cycles := fightCyclesForTest(t)
	for _, s := range allGoldcharSetups(t) {
		t.Run(s.Name, func(t *testing.T) {
			c, err := fightCycleFor(cycles, s.Name)
			if err != nil {
				t.Fatal(err)
			}
			cmp := replayBotFight(t, s, c, botFight(t, s, c, 1), defaultFightIterations)
			// the e2e fails a setup whose bot never lands one of the cycle's spells
			for _, spell := range cmp.fl.spellsWithoutCastEvent {
				t.Errorf("the bot never landed %d", spell)
			}
			for _, check := range cmp.checks {
				if !check.Passed {
					t.Errorf("%s %s: %s", check.Label, check.Name, check.Detail)
				}
			}
			t.Logf("%d checks, %d casts, log %.0f DPS, sim %.0f", len(cmp.checks), len(cmp.fl.casts), cmp.logDPS, cmp.simDPS)
		})
	}
}

// fightLines edits a log line by line: edit gets each line's timestamp and fields and returns them changed,
// or no fields to leave the line out.
func fightLines(text string, edit func(ms int64, fields []string) (int64, []string)) string {
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		msText, rest, _ := strings.Cut(line, "  ")
		ms, _ := strconv.ParseInt(msText, 10, 64)
		ms, fields := edit(ms, splitLogFields(rest))
		if len(fields) > 0 {
			fmt.Fprintf(&out, "%d  %s\n", ms, strings.Join(fields, ","))
		}
	}
	return out.String()
}

func scaleField(fields []string, i int, factor float64) []string {
	v, _ := strconv.ParseFloat(fields[i], 64)
	fields[i] = strconv.Itoa(int(math.Round(v * factor)))
	return fields
}

func failsCheck(cmp *fightComparison, subject, check string) (Check, bool) {
	for _, c := range cmp.checks {
		if c.Subject == subject && c.Name == check {
			return c.Check, !c.Passed
		}
	}
	return Check{}, false
}

// Each kind of disagreement fails its own check: a harder hitting spell, a cast the sim can't land then, an
// aura the server never put on, power the server gave twice, and swings that never crit.
func TestFightCatchesMismatches(t *testing.T) {
	needsItemDB(t)
	keepFightAliases(t)
	cycles := fightCyclesForTest(t)

	type mismatch struct {
		name, subject, check string
		edit                 func(ms int64, fields []string) (int64, []string)
	}
	run := func(setupName string, mismatches func(fl *fightLog, text string) []mismatch) {
		s := goldcharSetupNamed(t, setupName)
		c, err := fightCycleFor(cycles, setupName)
		if err != nil {
			t.Fatal(err)
		}
		text := botFight(t, s, c, 1)
		base := replayBotFight(t, s, c, text, defaultFightIterations)
		for _, m := range mismatches(base.fl, text) {
			if check, failed := failsCheck(base, m.subject, m.check); failed || check.Name == "" {
				t.Fatalf("%s: %s %s fails or is missing before the edit: %+v", setupName, m.subject, m.check, check)
			}
			cmp := replayBotFight(t, s, c, fightLines(text, m.edit), defaultFightIterations)
			if check, failed := failsCheck(cmp, m.subject, m.check); !failed {
				t.Errorf("%s, %s: %s %s still passes: %s", setupName, m.name, m.subject, m.check, check.Detail)
			}
		}
	}

	run("WarlockAffliction", func(fl *fightLog, text string) []mismatch {
		// Corruption, then Curse of Agony a GCD later
		corruption, agony := fl.casts[0], fl.casts[1]
		if corruption.Spell != 47813 || agony.Spell != 47864 || agony.At-corruption.At != 1500*time.Millisecond {
			t.Fatalf("the cycle's first casts are %+v, %+v", corruption, agony)
		}
		tally := fl.tally()
		var aura fightAuraKey
		for k, a := range tally.samples[0].auras {
			if k.On == "target" && a.applied > 2 && k.Spell != 47813 && k.Spell != 47864 && k.Spell != 47843 && k.Spell != 59164 {
				aura = k
			}
		}
		if aura.Spell == 0 {
			t.Fatal("no proc aura on the target")
		}
		var energize fightEnergizeKey
		for k, amount := range tally.samples[0].energize {
			if k.Power == "mana" && amount > 1000 {
				energize = k
			}
		}
		return []mismatch{
			// the hit mean's range ends at one iteration's tail, so a 40% harder Shadow Bolt can still fall inside it
			{"Shadow Bolt hits twice as hard", "47809", "hit mean", func(ms int64, f []string) (int64, []string) {
				if f[0] == "SPELL_DAMAGE" && f[7] == "47809" && f[16] == "nil" {
					return ms, scaleField(f, 10, 2)
				}
				return ms, f
			}},
			{"Curse of Agony a second into Corruption's GCD", "47864", "casts", func(ms int64, f []string) (int64, []string) {
				if f[0] == "SPELL_CAST_SUCCESS" && f[7] == "47864" {
					return ms - 1000, f
				}
				return ms, f
			}},
			{"an aura that never went on", aura.subject(), "applications", func(ms int64, f []string) (int64, []string) {
				if strings.HasPrefix(f[0], "SPELL_AURA_") && f[7] == strconv.Itoa(int(aura.Spell)) {
					return ms, nil
				}
				return ms, f
			}},
			{"twice the mana", energize.subject(), "amount", func(ms int64, f []string) (int64, []string) {
				if f[0] == "SPELL_ENERGIZE" && f[7] == strconv.Itoa(int(energize.Spell)) {
					return ms, scaleField(f, 10, 2)
				}
				return ms, f
			}},
		}
	})

	run("WarriorFury", func(fl *fightLog, text string) []mismatch {
		return []mismatch{
			{"swings that never crit", "melee", "crits", func(ms int64, f []string) (int64, []string) {
				if f[0] == "SWING_DAMAGE" {
					f[13] = "nil"
				}
				return ms, f
			}},
		}
	})
}

// A hand-built log: a warlock's Shadow Bolt and Corruption, a swing, its ghoul's swing, a power gain, auras up
// before the fight, and another mob's events, which the replay leaves out.
const handFightLog = `1000  CHRONICLE_HEADER,"AzerothCore","3.3.5a",12340
1000  CHRONICLE_ZONE_INFO,"Naxxramas",533,7,"raid","NORMAL"
1000  CHRONICLE_UNIT_INFO,0xF130000000000002,"SimVal Boss Dummy",83,0xa28,0x0000000000000000,24009944,"NEUTRAL",true
1000  CHRONICLE_UNIT_INFO,0xF140000000000010,"Risen Ghoul",80,0x1111,0x0000000000000001,8000,"FRIENDLY",false
1000  CHRONICLE_UNIT_INFO,0xF130000000000099,"Sludge Belcher",82,0xa28,0x0000000000000000,134810,"NEUTRAL",false
2000  SPELL_AURA_APPLIED,0x0000000000000001,"Testlock",0x511,0x0000000000000001,"Testlock",0x511,48266,"Blood Presence",0x1,BUFF
2500  SPELL_AURA_APPLIED,0x0000000000000001,"Testlock",0x511,0x0000000000000001,"Testlock",0x511,53365,"Unholy Strength",0x1,BUFF
3000  SPELL_CAST_SUCCESS,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47813,"Corruption",0x20
3000  SPELL_DAMAGE,0xF130000000000099,"Sludge Belcher",0xa28,0x0000000000000001,"Testlock",0x511,28157,"Slime Spray",0x8,900,0,8,0,0,0,nil,nil,nil
8000  SWING_DAMAGE,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,500,0,1,0,0,0,nil,1,nil
9000  SPELL_AURA_REMOVED,0x0000000000000001,"Testlock",0x511,0x0000000000000001,"Testlock",0x511,53365,"Unholy Strength",0x1,BUFF
10000  SPELL_CAST_SUCCESS,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47809,"Shadow Bolt",0x20
10000  SWING_MISSED,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,PARRY,nil,0
10500  SPELL_CAST_SUCCESS,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47813,"Corruption",0x20
10500  CHRONICLE_SPELL_TARGET_RESULT,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47813,"Corruption",0x20,"HIT",nil,0x1
10501  SPELL_CAST_SUCCESS,0x0000000000000001,"Testlock",0x511,0x0000000000000001,"Testlock",0x511,31818,"Life Tap",0x20
10501  SPELL_ENERGIZE,0x0000000000000001,"Testlock",0x511,0x0000000000000001,"Testlock",0x511,2687,"Bloodrage",0x1,100,1
10510  SPELL_AURA_APPLIED,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47813,"Corruption",0x20,DEBUFF
10600  SPELL_DAMAGE,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47809,"Shadow Bolt",0x20,2000,0,32,0,0,0,1,nil,nil
10700  CHRONICLE_SPELL_TARGET_RESULT,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47809,"Shadow Bolt",0x20,"RESIST",nil,0x1
11000  SWING_DAMAGE,0xF140000000000010,"Risen Ghoul",0x1111,0xF130000000000002,"SimVal Boss Dummy",0xa28,300,0,1,0,0,0,nil,nil,nil
12000  SPELL_AURA_APPLIED,0x0000000000000001,"Testlock",0x511,0x0000000000000001,"Testlock",0x511,17941,"Shadow Trance",0x20,BUFF
13500  SPELL_PERIODIC_DAMAGE,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47813,"Corruption",0x20,1000,0,32,0,0,0,nil,nil,nil
16500  SPELL_PERIODIC_DAMAGE,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47813,"Corruption",0x20,2000,0,32,0,0,0,1,nil,nil
17000  SPELL_DAMAGE,0xF130000000000099,"Sludge Belcher",0xa28,0x0000000000000001,"Testlock",0x511,28157,"Slime Spray",0x8,900,0,8,0,0,0,nil,nil,nil
18000  SPELL_AURA_REMOVED,0x0000000000000001,"Testlock",0x511,0xF130000000000002,"SimVal Boss Dummy",0xa28,47813,"Corruption",0x20,DEBUFF
`

var handCycle = fightCycle{Setup: "Test", Seconds: 60, Yards: 4, Cycle: []fightStep{
	{Spell: 47809, Name: "Shadow Bolt", WaitMs: 3200}, {Spell: 47813, Name: "Corruption", WaitMs: 1500},
}}

func handFight(t *testing.T, text string) *fightLog {
	t.Helper()
	saved := fightMinionNames
	fightMinionNames = map[string]string{"Risen Ghoul": "Ghoul"}
	t.Cleanup(func() { fightMinionNames = saved })
	fl, err := readFightLog(writeFightFile(t, "Test_Testlock_1.log", text), handCycle, "")
	if err != nil {
		t.Fatal(err)
	}
	return fl
}

func TestFightLogWindowAndCasts(t *testing.T) {
	fl := handFight(t, handFightLog)
	if fl.player.Name != "Testlock" || fl.setup != "Test" {
		t.Errorf("player %q, setup %q", fl.player.Name, fl.setup)
	}
	// the 3 s Shadow Bolt landing at 10 s started no later than 7 s, after the Corruption cast before it
	if fl.startMs != 3000 || fl.endMs != 16500 {
		t.Errorf("window %d to %d, want 3000 to 16500", fl.startMs, fl.endMs)
	}
	want := []fightCast{{47813, 0}, {47809, 7 * time.Second}, {47813, 7500 * time.Millisecond}}
	if !slices.Equal(fl.casts, want) {
		t.Errorf("casts %+v, want %+v; Life Tap's triggered cast isn't the bot's", fl.casts, want)
	}
	if !fl.hasAutos || fl.autos != [2]time.Duration{5 * time.Second, 7 * time.Second} {
		t.Errorf("autos %v %v", fl.hasAutos, fl.autos)
	}
	if !slices.Equal(fl.minions, []string{"Ghoul"}) {
		t.Errorf("minions %v", fl.minions)
	}
}

func TestFightLogTally(t *testing.T) {
	fl := handFight(t, handFightLog)
	s := fl.tally().samples[0]

	melee := s.direct[fightKey{}]
	if attempts(melee) != 2 || melee.Glance.N != 1 || melee.Avoided["parry"] != 1 {
		t.Errorf("melee %+v", melee)
	}
	bolt := s.direct[fightKey{Spell: 47809}]
	if attempts(bolt) != 2 || bolt.Crit.N != 1 || bolt.Crit.mean() != 2000 || bolt.Avoided["miss"] != 1 {
		t.Errorf("Shadow Bolt %+v; a resist counts as a miss", bolt)
	}
	corruption := s.periodic[fightKey{Spell: 47813}]
	if corruption.landed() != 2 || corruption.Crit.N != 1 || !slices.Equal(s.ticks[fightKey{Spell: 47813}], []int64{10500, 13500}) {
		t.Errorf("Corruption ticks %v, %+v", s.ticks[fightKey{Spell: 47813}], corruption)
	}
	if ghoul := s.direct[fightKey{Minion: "Ghoul"}]; ghoul == nil || ghoul.Hit.N != 1 {
		t.Errorf("the ghoul's swing %+v", ghoul)
	}
	if _, ok := s.direct[fightKey{Spell: 28157}]; ok {
		t.Error("another mob's damage counted")
	}
	if got := s.energize[fightEnergizeKey{Spell: 2687, Power: "rage"}]; got != 10 {
		t.Errorf("rage %v, want 10: the log has tenths", got)
	}

	if _, ok := s.auras[fightAuraKey{Spell: 48266, On: "self"}]; ok {
		t.Error("Blood Presence was up all fight, which leaves it to the golden characters")
	}
	for key, want := range map[fightAuraKey]auraSample{
		// applied before the fight, off 6 s in
		{Spell: 53365, On: "self"}:   {uptime: 6 * time.Second},
		{Spell: 47813, On: "target"}: {applied: 1, gained: 1, uptime: 5990 * time.Millisecond},
		// still up at the end
		{Spell: 17941, On: "self"}: {applied: 1, gained: 1, uptime: 4500 * time.Millisecond},
	} {
		if got := s.auras[key]; got == nil || *got != want {
			t.Errorf("%s: %+v, want %+v", key.subject(), got, want)
		}
	}
}

func TestTrimFight(t *testing.T) {
	saved := fightMinionNames
	fightMinionNames = map[string]string{"Risen Ghoul": "Ghoul"}
	t.Cleanup(func() { fightMinionNames = saved })
	raw := writeFightFile(t, "Test_Testlock_1.log", handFightLog)
	dir := t.TempDir()
	dest, err := trimFight(raw, dir, "", []fightCycle{handCycle})
	if err != nil {
		t.Fatal(err)
	}
	if dest != filepath.Join(dir, "Test.log.gz") {
		t.Errorf("saved %s", dest)
	}
	data, err := readMaybeGzipped(dest)
	if err != nil {
		t.Fatal(err)
	}
	var times []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		ms, rest, _ := strings.Cut(line, "  ")
		event, _, _ := strings.Cut(rest, ",")
		times = append(times, ms+" "+event)
	}
	want := []string{
		"1000 CHRONICLE_HEADER", "1000 CHRONICLE_ZONE_INFO", "1000 CHRONICLE_UNIT_INFO",
		"2000 SPELL_AURA_APPLIED", "2500 SPELL_AURA_APPLIED",
		"3000 SPELL_CAST_SUCCESS", "8000 SWING_DAMAGE", "9000 SPELL_AURA_REMOVED", "10000 SPELL_CAST_SUCCESS",
		"10000 SWING_MISSED", "10500 SPELL_CAST_SUCCESS", "10501 SPELL_ENERGIZE",
		"10510 SPELL_AURA_APPLIED", "10600 SPELL_DAMAGE", "10700 CHRONICLE_SPELL_TARGET_RESULT", "11000 SWING_DAMAGE",
		"12000 SPELL_AURA_APPLIED", "13500 SPELL_PERIODIC_DAMAGE", "16500 SPELL_PERIODIC_DAMAGE",
	}
	if !slices.Equal(times, want) {
		t.Errorf("kept\n%s\nwant\n%s", strings.Join(times, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(string(data), `"Risen Ghoul"`) || strings.Contains(string(data), "Sludge Belcher") {
		t.Error("the unit lines kept should be the ghoul's only")
	}

	trimmed, err := readFightLog(dest, handCycle, "")
	if err != nil {
		t.Fatal(err)
	}
	full := handFight(t, handFightLog)
	if trimmed.startMs != full.startMs || trimmed.endMs != full.endMs || !slices.Equal(trimmed.casts, full.casts) {
		t.Errorf("trimmed window %d-%d casts %v, full %d-%d %v", trimmed.startMs, trimmed.endMs, trimmed.casts,
			full.startMs, full.endMs, full.casts)
	}
}

func TestFightSetupAndPlayerFromTheFileName(t *testing.T) {
	for path, want := range map[string][2]string{
		"sim/core/testdata/fights/WarriorFury.log.gz":     {"WarriorFury", ""},
		"/simval/fights/HunterMM_Svfgtabc_1790000000.log": {"HunterMM", "Svfgtabc"},
	} {
		if setup, player := fightSetupName(path), fightPlayerName(path); setup != want[0] || player != want[1] {
			t.Errorf("%s: setup %q player %q", path, setup, player)
		}
	}
}

// The e2e names its log after the fight's first second on the log's clock. A cycle spell the player cast
// before it is the golden build's, left out of the replay and the trim.
func TestFightLogStartsAtTheNamedSecond(t *testing.T) {
	built := `2200  SPELL_CAST_SUCCESS,0x0000000000000001,"Testlock",0x511,0x0000000000000001,"Testlock",0x511,47813,"Corruption",0x20` + "\n"
	text := strings.Replace(handFightLog, "2500  ", built+"2500  ", 1)
	full := handFight(t, handFightLog)

	if early := handFight(t, text); early.startMs != 2200 {
		t.Errorf("named second 1: window from %d, want the build's cast at 2200", early.startMs)
	}
	raw := writeFightFile(t, "Test_Testlock_3.log", text)
	fl, err := readFightLog(raw, handCycle, "")
	if err != nil {
		t.Fatal(err)
	}
	if fl.startMs != full.startMs || !slices.Equal(fl.casts, full.casts) {
		t.Errorf("window from %d, casts %v; want %d, %v", fl.startMs, fl.casts, full.startMs, full.casts)
	}

	dest, err := trimFight(raw, t.TempDir(), "", []fightCycle{handCycle})
	if err != nil {
		t.Fatal(err)
	}
	trimmed, err := readFightLog(dest, handCycle, "")
	if err != nil {
		t.Fatal(err)
	}
	if trimmed.startMs != full.startMs || !slices.Equal(trimmed.casts, full.casts) {
		t.Errorf("trimmed: window from %d, casts %v", trimmed.startMs, trimmed.casts)
	}
}

func TestFightChecks(t *testing.T) {
	for _, c := range []struct {
		log   float64
		sim   []float64
		floor float64
		pass  bool
	}{
		{10, []float64{10, 10, 10}, 0.5, true},
		// a steady count, a floor of half a count: one off passes, two fail
		{11, []float64{10, 10, 10}, 0.5, true},
		{12, []float64{10, 10, 10}, 0.5, false},
		// inside the range
		{14, []float64{10, 12, 14, 11}, 0, true},
		// past it by more than two deviations
		{20, []float64{10, 12, 14, 11}, 0, false},
		// a rate the sim never rolled
		{25, []float64{0, 0, 0}, 0, false},
	} {
		if got := checkRange("x", c.log, c.sim, c.floor, ""); got.Passed != c.pass {
			t.Errorf("log %v against %v (floor %v): %s", c.log, c.sim, c.floor, got.Detail)
		}
	}
	if got := checkRange("x", 1, nil, 0, ""); !got.Passed || !strings.Contains(got.Name, "not compared") {
		t.Errorf("nothing to compare with: %+v", got)
	}
}

// spelldumpSpell is what TestFightCycles reads of the live server's spells.
type spelldumpSpell struct {
	ID         int32    `json:"id"`
	Name       string   `json:"name"`
	Family     int32    `json:"family"`
	Attributes []uint32 `json:"attributes"`
	CastTimeMs int32    `json:"castTimeMs"`
	GCDMs      int32    `json:"startRecoveryTimeMs"`
	DurationMs int32    `json:"durationMs"`
	Channeled  bool     `json:"channeled"`
	Passive    bool     `json:"passive"`
	RangeMax   float64  `json:"rangeMax"`
}

func readSpelldump(t *testing.T, ids map[int32]bool) map[int32]spelldumpSpell {
	t.Helper()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(root, spelldumpPath))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[int32]spelldumpSpell{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for scanner.Scan() {
		var s spelldumpSpell
		if err := json.Unmarshal(scanner.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		if ids[s.ID] {
			out[s.ID] = s
		}
	}
	return out
}

const (
	attr0OnNextSwingNoDamage = 0x4
	attr0OnNextSwing         = 0x400
)

// Every golden setup has one cycle, every spell in it is one the setup's sim character can cast and the
// server's class has, nothing waits on the next swing, and each step waits out its GCD, its cast or its
// channel before the bot sends the next: a cast sent with more than the 400 ms spell queue window left on
// the one before interrupts it.
func TestFightCycles(t *testing.T) {
	needsItemDB(t)
	cycles := fightCyclesForTest(t)
	setups := allGoldcharSetups(t)
	ids := map[int32]bool{}
	for _, c := range cycles {
		for _, step := range slices.Concat(c.Before, c.Cycle) {
			ids[step.Spell] = true
		}
		for _, spell := range c.Learn {
			ids[spell] = true
		}
	}
	dump := readSpelldump(t, ids)

	for _, s := range setups {
		if n := len(slices.DeleteFunc(slices.Clone(cycles), func(c fightCycle) bool { return c.Setup != s.Name })); n != 1 {
			t.Errorf("%s has %d cycles", s.Name, n)
		}
	}
	for _, c := range cycles {
		i := slices.IndexFunc(setups, func(s goldcharSetup) bool { return s.Name == c.Setup })
		if i < 0 {
			t.Errorf("cycle for %q, which isn't a golden setup", c.Setup)
			continue
		}
		s := setups[i]
		if c.Seconds < 60 || c.Seconds > 120 {
			t.Errorf("%s: %d s, want a 1 to 2 min fight", c.Setup, c.Seconds)
		}
		if c.Behind && !c.melee() {
			t.Errorf("%s: behind the dummy at %g yards", c.Setup, c.Yards)
		}
		_, hasAmmo := ammoTypes[c.Ammo]
		if (s.Class == serverHunter) != (c.Ammo != 0 && hasAmmo) {
			t.Errorf("%s: ammo %d", c.Setup, c.Ammo)
		}
		if len(c.Cycle) == 0 {
			t.Errorf("%s: an empty cycle", c.Setup)
		}
		fl := &fightLog{setup: c.Setup, cycle: c, endMs: 1000, names: map[int32]string{}}
		f, err := newFightSim(s, fl)
		if err != nil {
			t.Errorf("%s: %v", c.Setup, err)
			continue
		}
		family := serverdata.ClassFamily(classes[s.Class].class)
		for _, spell := range c.Learn {
			if sd, ok := dump[spell]; ok && !sd.Passive {
				t.Errorf("%s: learns %d, which isn't a passive", c.Setup, spell)
			}
		}
		for k, step := range slices.Concat(c.Before, c.Cycle) {
			name := fmt.Sprintf("%s step %d (%d %s)", c.Setup, k, step.Spell, step.Name)
			sd, ok := dump[step.Spell]
			switch {
			case !ok:
				t.Errorf("%s: not in the spelldump", name)
				continue
			case sd.Name != step.Name:
				t.Errorf("%s: the server calls it %s", name, sd.Name)
			case sd.Family != family:
				t.Errorf("%s: family %d, the class's is %d", name, sd.Family, family)
			case sd.Passive:
				t.Errorf("%s: passive", name)
			case sd.Attributes[0]&(attr0OnNextSwing|attr0OnNextSwingNoDamage) != 0:
				t.Errorf("%s: waits on the next swing, which the replay can't land on the logged time", name)
			case sd.RangeMax > 0 && sd.RangeMax < c.Yards:
				t.Errorf("%s: %g yards of range at %g", name, sd.RangeMax, c.Yards)
			}
			if step.Spell == autoShotSpell {
				if s.Class != serverHunter || k >= len(c.Before) {
					t.Errorf("%s: Auto Shot only starts a hunter's shooting, before the cycle", name)
				}
				continue
			}
			if f.spells[step.Spell] == nil {
				t.Errorf("%s: the sim character can't cast it", name)
			}
			wait := sd.GCDMs
			switch {
			case sd.Channeled:
				wait = max(wait, sd.DurationMs+100)
			case sd.CastTimeMs > 0:
				wait = max(wait, sd.CastTimeMs+100)
			case sd.GCDMs == 0:
				wait = 100
			}
			if int32(step.WaitMs) < wait {
				t.Errorf("%s: waits %d ms, needs %d", name, step.WaitMs, wait)
			}
		}
	}
}

// The fights stand in front of goldchar.jsonl's boss dummy.
func TestFightBossDummyIsGoldcharsDummy(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	records, err := readGoldcharRecords(filepath.Join(root, fixtureDir, goldcharFixture))
	if err != nil {
		t.Skip(err)
	}
	for _, rec := range records {
		if rec.Target.Entry != boss {
			continue
		}
		got := rec.Target
		if got.Level != bossDummy.Level || got.WorldBoss != bossDummy.WorldBoss || got.CreatureType != bossDummy.CreatureType ||
			len(got.Resistances) == 0 || got.Resistances[0] != bossDummy.Resistances[0] || got.MaxHealth != bossDummy.MaxHealth {
			t.Errorf("goldchar.jsonl's dummy: level %d world boss %v type %d armor %v max health %.0f; the fights' %+v",
				got.Level, got.WorldBoss, got.CreatureType, got.Resistances, got.MaxHealth, bossDummy)
		}
		return
	}
	t.Skip("goldchar.jsonl has no boss dummy")
}

// TestFightReplays replays every committed fight, one per golden setup, holding each failing check to
// droppedFightChecks.
func TestFightReplays(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(root, fightsDir, "*.log.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Skipf("no fights in %s yet", fightsDir)
	}
	needsItemDB(t)
	setups := allGoldcharSetups(t)
	cycles := fightCyclesForTest(t)
	fought := map[string]bool{}
	for _, path := range paths {
		s, fl, err := loadFight(path, "", setups, cycles)
		if err != nil {
			t.Error(err)
			continue
		}
		fought[s.Name] = true
		cmp, err := replayFight(s, fl, defaultFightIterations)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		for _, drop := range cmp.applyDrops(droppedFightChecks) {
			t.Errorf("%s: drop %+v matches no failing check", s.Name, drop)
		}
		var dropped int
		for _, check := range cmp.checks {
			switch {
			case check.Dropped:
				dropped++
				t.Logf("%s %s %s (dropped): %s", s.Name, check.Label, check.Name, check.Detail)
			case !check.Passed:
				t.Errorf("%s %s %s: %s", s.Name, check.Label, check.Name, check.Detail)
			}
		}
		t.Logf("%s: %d checks, %d dropped, %d casts, log %.0f DPS, sim %.0f", s.Name, len(cmp.checks), dropped,
			len(fl.casts), cmp.logDPS, cmp.simDPS)
	}
	for _, s := range setups {
		if !fought[s.Name] {
			t.Errorf("%s has no committed fight", s.Name)
		}
	}
	for _, drop := range droppedFightChecks {
		if !fought[drop.Setup] {
			t.Errorf("drop %+v: no fight for %s", drop, drop.Setup)
		}
	}
}

// Every drop has its row in the fights README.
func TestFightDropsHaveReadmeRows(t *testing.T) {
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	readme, err := os.ReadFile(filepath.Join(root, fightsDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, drop := range droppedFightChecks {
		// \r?: a checkout with core.autocrlf has CRLF line ends
		row := regexp.MustCompile(`(?m)^\| ` + regexp.QuoteMeta(drop.Setup) + ` \| ` + regexp.QuoteMeta(drop.Subject) +
			` \| ` + regexp.QuoteMeta(drop.Check) + ` \| .+ \|\r?$`)
		if !row.Match(readme) {
			t.Errorf("no README row for %+v", drop)
		}
	}
}
