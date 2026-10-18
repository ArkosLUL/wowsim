// Command rankings sims each DPS build at each phase's BiS gear under one standard scenario, at a few
// fight lengths, one results/<build>_p<N>.json each. It reads the settings capture.py saved.
package main

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

type fightResult struct {
	Dps   float64 `json:"dps"`
	Stdev float64 `json:"stdev"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

type result struct {
	Build      string  `json:"build"`
	Phase      int     `json:"phase"`
	Class      string  `json:"class"`
	Spec       string  `json:"spec"`
	Talents    string  `json:"talents"`
	Items      []int32 `json:"items"`
	Settings   string  `json:"settings_sha256"`
	Scenario   string  `json:"scenario_sha256"`
	Sim        string  `json:"sim_sha256"`
	Seed       int64   `json:"seed"`
	Iterations int32   `json:"iterations"`
	// keyed by the fight length in seconds
	Fights  map[string]fightResult `json:"fights"`
	Link    string                 `json:"link"`
	Seconds float64                `json:"seconds"`
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
	fights     []int
	simHash    string
	log        *os.File
}

var settingsName = regexp.MustCompile(`^(.+)_p([1-5])\.bin$`)

// the link and the sha of the standard scenario are made at this length
const linkFight = 180

func main() {
	out := flag.String("out", "", "work directory: reads settings/, writes results/ and run.log (required)")
	builds := flag.String("builds", "all", "comma-separated build keys, or all")
	phases := flag.String("phases", "1-5", "e.g. 1-5 or 1,5")
	fights := flag.String("fights", "60,180,300", "fight lengths in seconds")
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
		fail("the sim has no item database: go run --tags=with_db ./tools/rankings")
	}
	phaseSet, err := parsePhases(*phases)
	if err != nil {
		fail(err.Error())
	}
	fightList, err := parseFights(*fights)
	if err != nil {
		fail(err.Error())
	}
	jobs, err := listJobs(filepath.Join(*out, "settings"), *builds, phaseSet)
	if err != nil {
		fail(err.Error())
	}
	simHash, err := executableHash()
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
	cfg := config{out: *out, iterations: int32(*iterations), seed: *seed, fights: fightList, simHash: simHash,
		log: logFile}

	sim.RegisterAll()
	cfg.logf("run: %d settings files, fights %v, %d iterations, seed %d", len(jobs), fightList, cfg.iterations, cfg.seed)
	start := time.Now()
	ran, skipped := 0, 0
	for _, j := range jobs {
		did, err := cfg.run(j)
		if err != nil {
			cfg.logf("! %s P%d: %v", j.build, j.phase, err)
			os.Exit(1)
		}
		if did {
			ran++
		} else {
			skipped++
		}
	}
	cfg.logf("done: %d run, %d already up to date or skipped, %.0f s", ran, skipped, time.Since(start).Seconds())
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "rankings:", msg)
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

func parseFights(text string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(text, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("fights must be lengths in seconds: %q", text)
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out, nil
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

// executableHash stands for the sim build: a change to the sim, the embedded item DB or this tool
// relinks the binary.
func executableHash() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func isTank(spec proto.Spec) bool {
	switch spec {
	case proto.Spec_SpecTankDeathknight, proto.Spec_SpecFeralTankDruid, proto.Spec_SpecProtectionPaladin,
		proto.Spec_SpecProtectionWarrior:
		return true
	}
	return false
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

func marshal(m googleProto.Message) ([]byte, error) {
	return googleProto.MarshalOptions{Deterministic: true}.Marshal(m)
}

// makeLink is what IndividualLinkImporter.tryParseUrlLocation reads: the zlib-deflated settings,
// base64 encoded.
func makeLink(settings *proto.IndividualSimSettings) (string, error) {
	raw, err := marshal(settings)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func simulate(settings *proto.IndividualSimSettings, iterations int32, seed int64) (*proto.UnitMetrics, error) {
	raid := core.SinglePlayerRaidProto(settings.Player, settings.PartyBuffs, settings.RaidBuffs, settings.Debuffs)
	raid.Tanks = settings.Tanks
	req := &proto.RaidSimRequest{Raid: raid, Encounter: settings.Encounter,
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: seed, IsTest: true}}
	progress := make(chan *proto.ProgressMetrics, 100)
	core.RunRaidSimAsync(req, progress)
	for p := range progress {
		if r := p.FinalRaidResult; r != nil {
			if r.ErrorResult != "" {
				return nil, fmt.Errorf("sim: %s", r.ErrorResult)
			}
			return r.RaidMetrics.Parties[0].Players[0], nil
		}
	}
	return nil, fmt.Errorf("sim ended without a result")
}

// previous returns the earlier result when nothing it depends on changed.
func (c config) previous(path string, settingsHash, scenarioHash string) *result {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var prev result
	if json.Unmarshal(data, &prev) != nil || prev.Settings != settingsHash || prev.Scenario != scenarioHash ||
		prev.Sim != c.simHash || prev.Seed != c.seed || prev.Iterations != c.iterations ||
		len(prev.Fights) != len(c.fights) {
		return nil
	}
	for _, f := range c.fights {
		if _, ok := prev.Fights[strconv.Itoa(f)]; !ok {
			return nil
		}
	}
	return &prev
}

func (c config) run(j job) (bool, error) {
	data, err := os.ReadFile(j.file)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(data)
	captured, err := loadSettings(data)
	if err != nil {
		return false, fmt.Errorf("%s: %v", j.file, err)
	}
	spec := core.PlayerProtoToSpec(captured.Player)
	if isTank(spec) {
		return false, nil
	}

	linkSettings := standardize(captured, linkFight)
	// the page has its own item DB, and its share links leave this out too
	linkSettings.Player.Database = nil
	// the page only reads this with u in the URL's ?i=, and unset ShowDamageMetrics would hide the DPS
	linkSettings.Settings = &proto.SimSettings{Iterations: c.iterations, Phase: int32(j.phase),
		ShowDamageMetrics: true}
	link, err := makeLink(linkSettings)
	if err != nil {
		return false, err
	}
	scenarioRaw, err := marshal(linkSettings)
	if err != nil {
		return false, err
	}
	scenarioSum := sha256.Sum256(scenarioRaw)
	settingsHash, scenarioHash := hex.EncodeToString(sum[:]), hex.EncodeToString(scenarioSum[:])

	dest := filepath.Join(c.out, "results", fmt.Sprintf("%s_p%d.json", j.build, j.phase))
	if c.previous(dest, settingsHash, scenarioHash) != nil {
		return false, nil
	}

	start := time.Now()
	res := result{Build: j.build, Phase: j.phase, Class: captured.Player.Class.String(), Spec: spec.String(),
		Talents: captured.Player.TalentsString, Items: itemIDs(captured.Player), Settings: settingsHash,
		Scenario: scenarioHash, Sim: c.simHash, Seed: c.seed, Iterations: c.iterations,
		Fights: map[string]fightResult{}, Link: link}
	for _, f := range c.fights {
		m, err := simulate(standardize(captured, float64(f)), c.iterations, c.seed)
		if err != nil {
			return false, fmt.Errorf("%ds fight: %v", f, err)
		}
		d := m.GetDps()
		res.Fights[strconv.Itoa(f)] = fightResult{Dps: d.GetAvg(), Stdev: d.GetStdev(), Min: d.GetMin(), Max: d.GetMax()}
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
	line := ""
	for _, f := range c.fights {
		line += fmt.Sprintf(" %ds %.0f", f, res.Fights[strconv.Itoa(f)].Dps)
	}
	c.logf("%s P%d: dps%s in %.0f s", j.build, j.phase, line, res.Seconds)
	return true, nil
}
