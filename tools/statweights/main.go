// Command statweights measures wide-step stat slopes for each build and phase that capture.py saved
// settings for, one results/<build>_p<N>.json each.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/statslopes"
	googleProto "google.golang.org/protobuf/proto"
)

type slope struct {
	Delta float64 `json:"delta"`
	Shift float64 `json:"shift"`
	Dps   float64 `json:"dps"`
	Tps   float64 `json:"tps"`
	Tmi   float64 `json:"tmi"`
}

type result struct {
	Build      string  `json:"build"`
	Phase      int     `json:"phase"`
	Class      string  `json:"class"`
	Spec       string  `json:"spec"`
	Seed       int64   `json:"seed"`
	Iterations int32   `json:"iterations"`
	Settings   string  `json:"settings_sha256"`
	Items      []int32 `json:"items"`
	Dps        float64 `json:"dps"`
	Tps        float64 `json:"tps"`
	Tmi        float64 `json:"tmi"`
	Seconds    float64 `json:"seconds"`
	// keyed by the stat's or pseudo stat's proto enum name
	Slopes map[string]slope `json:"slopes"`
}

type job struct {
	build string
	phase int
	file  string
}

type config struct {
	out        string
	iterations int32
	seed       int64
	log        *os.File
}

var settingsName = regexp.MustCompile(`^(.+)_p([1-5])\.bin$`)

func main() {
	out := flag.String("out", "", "work directory: reads settings/, writes results/ and run.log (required)")
	builds := flag.String("builds", "all", "comma-separated build keys, or all")
	phases := flag.String("phases", "1-5", "e.g. 1-5 or 1,5")
	iterations := flag.Int("iterations", 10000, "iterations per sim")
	seed := flag.Int64("seed", 424242, "random seed of every sim")
	flag.Parse()
	if *out == "" {
		fail("--out is required")
	}
	if flag.NArg() > 0 {
		fail(fmt.Sprintf("unexpected arguments %q", flag.Args()))
	}
	if *iterations < 1 {
		fail("--iterations must be at least 1")
	}
	if !core.WITH_DB {
		fail("the sim has no item database: go run --tags=with_db ./tools/statweights")
	}
	phaseSet, err := parsePhases(*phases)
	if err != nil {
		fail(err.Error())
	}
	jobs, err := listJobs(filepath.Join(*out, "settings"), *builds, phaseSet)
	if err != nil {
		fail(err.Error())
	}
	if err := os.MkdirAll(filepath.Join(*out, "results"), 0o755); err != nil {
		fail(err.Error())
	}
	logFile, err := os.OpenFile(filepath.Join(*out, "run.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fail(err.Error())
	}
	defer logFile.Close()
	cfg := config{out: *out, iterations: int32(*iterations), seed: *seed, log: logFile}

	sim.RegisterAll()
	cfg.logf("run: %d settings files, %d iterations, seed %d", len(jobs), cfg.iterations, cfg.seed)
	start := time.Now()
	ran := 0
	for _, j := range jobs {
		did, err := cfg.run(j)
		if err != nil {
			cfg.logf("! %s P%d: %v", j.build, j.phase, err)
			os.Exit(1)
		}
		if did {
			ran++
		}
	}
	cfg.logf("done: %d run, %d already up to date, %.0f s", ran, len(jobs)-ran, time.Since(start).Seconds())
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "statweights:", msg)
	os.Exit(2)
}

func (c config) logf(format string, args ...any) {
	line := time.Now().Format("2006-01-02 15:04:05 ") + fmt.Sprintf(format, args...) + "\n"
	os.Stdout.WriteString(line)
	c.log.WriteString(line)
}

func parsePhases(text string) (map[int]bool, error) {
	set := map[int]bool{}
	for _, part := range strings.Split(text, ",") {
		lo, hi, isRange := strings.Cut(strings.TrimSpace(part), "-")
		if !isRange {
			hi = lo
		}
		a, err1 := strconv.Atoi(lo)
		b, err2 := strconv.Atoi(hi)
		if err1 != nil || err2 != nil || a < 1 || b > 5 || a > b {
			return nil, fmt.Errorf("phases must be within 1-5: %q", text)
		}
		for p := a; p <= b; p++ {
			set[p] = true
		}
	}
	return set, nil
}

func listJobs(dir, builds string, phases map[int]bool) ([]job, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("no settings to run (capture first): %v", err)
	}
	all := builds == "all"
	found := map[string]bool{}
	if !all {
		for _, b := range strings.Split(builds, ",") {
			found[strings.TrimSpace(b)] = false
		}
	}
	var jobs []job
	for _, e := range entries {
		m := settingsName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		phase, _ := strconv.Atoi(m[2])
		if _, ok := found[m[1]]; (!all && !ok) || !phases[phase] {
			continue
		}
		found[m[1]] = true
		jobs = append(jobs, job{build: m[1], phase: phase, file: filepath.Join(dir, e.Name())})
	}
	var missing []string
	for b, ok := range found {
		if !ok {
			missing = append(missing, b)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("no settings in %s for %s", dir, strings.Join(missing, ", "))
	}
	sort.Slice(jobs, func(a, b int) bool {
		if jobs[a].build != jobs[b].build {
			return jobs[a].build < jobs[b].build
		}
		return jobs[a].phase < jobs[b].phase
	})
	return jobs, nil
}

// loadSettings reads the ComputeStatsRequest the page sent as the settings to sim.
func loadSettings(data []byte) (*proto.IndividualSimSettings, error) {
	req := &proto.ComputeStatsRequest{}
	if err := googleProto.Unmarshal(data, req); err != nil {
		return nil, err
	}
	if len(req.GetRaid().GetParties()) == 0 || len(req.Raid.Parties[0].Players) == 0 {
		return nil, fmt.Errorf("no player in the request")
	}
	party := req.Raid.Parties[0]
	return &proto.IndividualSimSettings{
		Player:     party.Players[0],
		PartyBuffs: party.Buffs,
		RaidBuffs:  req.Raid.Buffs,
		Debuffs:    req.Raid.Debuffs,
		Tanks:      req.Raid.Tanks,
		Encounter:  req.Encounter,
	}, nil
}

func raidOf(settings *proto.IndividualSimSettings) *proto.Raid {
	raid := core.SinglePlayerRaidProto(settings.Player, settings.PartyBuffs, settings.RaidBuffs, settings.Debuffs)
	raid.Tanks = settings.Tanks
	return raid
}

// itemIDs lists the gear in ItemSlot order, 0 for an empty slot.
func itemIDs(player *proto.Player) []int32 {
	ids := make([]int32, len(proto.ItemSlot_name))
	for i, item := range player.GetEquipment().GetItems() {
		if i < len(ids) {
			ids[i] = item.GetId()
		}
	}
	return ids
}

// previous returns the earlier result for this job when it was run from the same settings at the
// same seed and iterations, so its base and any slope whose step is unchanged can be kept.
func (c config) previous(path, settingsHash string) *result {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var prev result
	if json.Unmarshal(data, &prev) != nil || prev.Settings != settingsHash || prev.Seed != c.seed ||
		prev.Iterations != c.iterations {
		return nil
	}
	return &prev
}

func upToDate(prev *result, steps []statslopes.Step) bool {
	if prev == nil || len(prev.Slopes) != len(steps) {
		return false
	}
	for _, s := range steps {
		if p, ok := prev.Slopes[s.Name]; !ok || p.Delta != s.Delta || p.Shift != s.Shift {
			return false
		}
	}
	return true
}

func (c config) run(j job) (bool, error) {
	data, err := os.ReadFile(j.file)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(data)
	settings, err := loadSettings(data)
	if err != nil {
		return false, fmt.Errorf("%s: %v", j.file, err)
	}
	spec := core.PlayerProtoToSpec(settings.Player)
	r, err := statslopes.RoleOf(spec)
	if err != nil {
		return false, err
	}
	raid := raidOf(settings)
	steps := statslopes.CenteredSteps(r, statslopes.FinalStats(raid, settings.Encounter))
	dest := filepath.Join(c.out, "results", fmt.Sprintf("%s_p%d.json", j.build, j.phase))
	prev := c.previous(dest, hex.EncodeToString(sum[:]))
	if upToDate(prev, steps) {
		return false, nil
	}

	start := time.Now()
	res := result{Build: j.build, Phase: j.phase, Class: settings.Player.Class.String(), Spec: spec.String(),
		Seed: c.seed, Iterations: c.iterations, Settings: hex.EncodeToString(sum[:]),
		Items: itemIDs(settings.Player), Slopes: map[string]slope{}}
	if prev != nil {
		res.Dps, res.Tps, res.Tmi = prev.Dps, prev.Tps, prev.Tmi
	} else {
		base, err := statslopes.Simulate(raid, settings.Encounter, c.iterations, c.seed)
		if err != nil {
			return false, err
		}
		res.Dps, res.Tps, res.Tmi = base.GetDps().GetAvg(), base.GetThreat().GetAvg(), base.GetTmi().GetAvg()
	}
	reused := 0
	for _, s := range steps {
		if prev != nil {
			if p, ok := prev.Slopes[s.Name]; ok && p.Delta == s.Delta && p.Shift == s.Shift {
				res.Slopes[s.Name] = p
				reused++
				continue
			}
		}
		m, err := statslopes.Centered(raid, settings.Encounter, s, c.iterations, c.seed)
		if err != nil {
			return false, fmt.Errorf("%s: %v", s.Name, err)
		}
		res.Slopes[s.Name] = slope{Delta: s.Delta, Shift: s.Shift, Dps: m.Dps, Tps: m.Tps, Tmi: m.Tmi}
	}
	res.Seconds = time.Since(start).Seconds()

	out, err := json.MarshalIndent(res, "", " ")
	if err != nil {
		return false, err
	}
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return false, err
	}
	note := ""
	if reused > 0 {
		note = fmt.Sprintf(", %d slopes kept", reused)
	}
	c.logf("%s P%d: dps %.0f, %d slopes in %.0f s%s", j.build, j.phase, res.Dps, len(steps)-reused, res.Seconds, note)
	return true, nil
}
