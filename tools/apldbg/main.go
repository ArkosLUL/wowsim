// Command apldbg runs one character on a rotation and prints what it did: a golden suite's rows, the casts,
// damage and aura uptimes behind one of them, and one iteration's log or timeline. Its README has the flags.
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// row is one sim a suite runs, or the request file's.
type row struct {
	label string
	rsr   *proto.RaidSimRequest
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, out io.Writer) int {
	flags := flag.NewFlagSet("apldbg", flag.ContinueOnError)
	spec := flags.String("spec", "", "a golden suite's character: "+strings.Join(presetNames(), ", "))
	requestPath := flags.String("request", "", "a RaidSimRequest or the UI's exported settings as JSON, gzipped or not, in place of -spec")
	aplPath := flags.String("apl", "", "an .apl.json to run in place of the character's own rotations")
	gear := flags.String("gear", "", "with -spec, another gear set of its gear_sets directory, without .gear.json")
	rows := flags.Bool("rows", false, "print the DPS of every row and stop")
	pick := flags.String("row", "Average", "the row to look into, as comma-separated parts of its label, any case")
	iters := flags.Int("iters", 0, "iterations, in place of the row's own (20, Average's 2000, the request's)")
	seed := flags.Int64("seed", 0, "random seed, in place of the row's own (the suites' 101)")
	ids := flags.String("ids", "", "only print actions and auras whose id holds one of these comma-separated parts, e.g. 45529,OtherID: 3")
	logPath := flags.String("log", "", "write one debug iteration's log here")
	timeline := flags.String("timeline", "", "print that iteration's casts, and the -ids auras going on and off, between from:to seconds (\":\" for all)")
	match := flags.String("match", "", "with -timeline, also print the character's log lines matching this regexp, e.g. 'rune from'")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintln(os.Stderr, "apldbg:", err)
		return 1
	}
	// before the sims, which can take a minute
	var cut timelineCut
	if *timeline != "" {
		var err error
		if cut, err = newTimelineCut(*timeline, splitParts(*ids), *match); err != nil {
			return fail(err)
		}
	}

	sim.RegisterAll()
	all, err := loadRows(*spec, *requestPath, *aplPath, *gear)
	if err != nil {
		return fail(err)
	}
	for i := range all {
		all[i].rsr.SimOptions = simOptions(all[i].rsr.SimOptions, *iters, *seed)
	}

	if *rows {
		for _, r := range all {
			result := core.RunRaidSim(r.rsr)
			if result.ErrorResult != "" {
				return fail(fmt.Errorf("%s: %s", r.label, result.ErrorResult))
			}
			fmt.Fprintf(out, "%s %.3f\n", r.label, player(result).Dps.Avg)
		}
		return 0
	}

	r, err := pickRow(all, *pick)
	if err != nil {
		return fail(err)
	}
	for _, warning := range rotationWarnings(r.rsr) {
		fmt.Fprintln(out, "WARN", warning)
	}
	result := core.RunRaidSim(r.rsr)
	if result.ErrorResult != "" {
		return fail(fmt.Errorf("%s: %s", r.label, result.ErrorResult))
	}
	report(out, r, result, splitParts(*ids))

	if *logPath == "" && *timeline == "" {
		return 0
	}
	debug := googleProto.Clone(r.rsr).(*proto.RaidSimRequest)
	debug.SimOptions.Iterations, debug.SimOptions.Debug = 1, true
	logged := core.RunRaidSim(debug)
	if logged.ErrorResult != "" {
		return fail(fmt.Errorf("%s: %s", r.label, logged.ErrorResult))
	}
	if *logPath != "" {
		if err := os.WriteFile(*logPath, []byte(logged.Logs), 0o644); err != nil {
			return fail(err)
		}
	}
	if *timeline != "" {
		fmt.Fprintln(out, strings.Join(cut.lines(logged.Logs), "\n"))
	}
	return 0
}

func presetNames() []string {
	names := make([]string, 0, len(presets))
	for name := range presets {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func splitParts(list string) []string {
	var parts []string
	for _, part := range strings.Split(list, ",") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// checkoutRoot is the directory holding go.mod, from the working directory up: the presets' gear sets and
// rotations are named from there.
func checkoutRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above the working directory")
		}
		dir = parent
	}
}

// loadRotation reads an .apl.json, labelled like the suites label theirs: the file's name.
func loadRotation(path string) (core.RotationCombo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.RotationCombo{}, err
	}
	rotation := &proto.APLRotation{}
	if err := protojson.Unmarshal(data, rotation); err != nil {
		return core.RotationCombo{}, fmt.Errorf("%s: %w", path, err)
	}
	label := strings.TrimSuffix(filepath.Base(path), ".apl.json")
	return core.RotationCombo{Label: label, Rotation: rotation}, nil
}

func loadRows(spec, requestPath, aplPath, gear string) ([]row, error) {
	var apl *core.RotationCombo
	if aplPath != "" {
		rotation, err := loadRotation(aplPath)
		if err != nil {
			return nil, err
		}
		apl = &rotation
	}
	switch {
	case spec != "" && requestPath != "":
		return nil, fmt.Errorf("-spec and -request both name the character: give one")
	case requestPath != "":
		if gear != "" {
			return nil, fmt.Errorf("-gear goes with -spec: a request holds its own gear")
		}
		return requestRows(requestPath, apl)
	case spec != "":
		p, ok := presets[spec]
		if !ok {
			return nil, fmt.Errorf("no -spec %q: there's %s", spec, strings.Join(presetNames(), ", "))
		}
		root, err := checkoutRoot()
		if err != nil {
			return nil, err
		}
		return presetRows(p, root, apl, gear), nil
	}
	return nil, fmt.Errorf("give -spec or -request")
}

// presetRows are the suite's Settings rows and its Average, on apl alone when there is one.
func presetRows(p preset, root string, apl *core.RotationCombo, gear string) []row {
	config := p.config(root)
	if apl != nil {
		config.Rotation, config.OtherRotations = *apl, nil
	}
	if gear != "" {
		config.GearSet, config.OtherGearSets = core.GetGearSet(filepath.Join(root, p.ui, "gear_sets"), gear), nil
	}
	generator := core.FullCharacterTestSuiteGenerator(config)
	var rows []row
	for i := range generator.NumTests() {
		label, _, _, rsr := generator.GetTest(i)
		if rsr != nil && (strings.HasPrefix(label, "Settings-") || strings.HasPrefix(label, "Average-")) {
			rows = append(rows, row{label, googleProto.Clone(rsr).(*proto.RaidSimRequest)})
		}
	}
	return rows
}

// requestRows is the file's one sim: a RaidSimRequest, or the settings the UI exports for one character.
func requestRows(path string, apl *core.RotationCombo) ([]row, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte{0x1f, 0x8b}) {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if data, err = io.ReadAll(gz); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	rsr, err := parseRequest(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if apl != nil {
		rsr.Raid.Parties[0].Players[0].Rotation = apl.Rotation
	}
	return []row{{"Request", rsr}}, nil
}

func parseRequest(data []byte) (*proto.RaidSimRequest, error) {
	lenient := protojson.UnmarshalOptions{DiscardUnknown: true}
	rsr := &proto.RaidSimRequest{}
	if err := lenient.Unmarshal(data, rsr); err != nil {
		return nil, err
	}
	if parties := rsr.GetRaid().GetParties(); len(parties) > 0 && len(parties[0].Players) > 0 {
		return rsr, nil
	}
	settings := &proto.IndividualSimSettings{}
	if err := lenient.Unmarshal(data, settings); err != nil {
		return nil, err
	}
	if settings.Player == nil {
		return nil, fmt.Errorf("neither a RaidSimRequest with a player nor exported settings with one")
	}
	raid := core.SinglePlayerRaidProto(settings.Player, settings.PartyBuffs, settings.RaidBuffs, settings.Debuffs)
	raid.Tanks = settings.Tanks
	return &proto.RaidSimRequest{Raid: raid, Encounter: settings.Encounter}, nil
}

// simOptions are the row's own with the flags' iterations and seed over them. A request without any gets the
// suites' test options at 1000 iterations.
func simOptions(own *proto.SimOptions, iters int, seed int64) *proto.SimOptions {
	options := &proto.SimOptions{Iterations: 1000, IsTest: true, RandomSeed: 101}
	if own != nil {
		options = googleProto.Clone(own).(*proto.SimOptions)
	}
	if iters > 0 {
		options.Iterations = int32(iters)
	}
	if seed != 0 {
		options.RandomSeed = seed
	}
	options.Debug = false
	return options
}

// pickRow is the one row whose label holds every part, whatever the case. A request's lone row needs none.
func pickRow(rows []row, parts string) (row, error) {
	if len(rows) == 1 {
		return rows[0], nil
	}
	var found []row
	for _, r := range rows {
		label := strings.ToLower(r.label)
		if !slices.ContainsFunc(splitParts(parts), func(part string) bool { return !strings.Contains(label, strings.ToLower(part)) }) {
			found = append(found, r)
		}
	}
	if len(found) == 1 {
		return found[0], nil
	}
	labels := make([]string, len(rows))
	for i, r := range rows {
		labels[i] = r.label
	}
	return row{}, fmt.Errorf("-row %q fits %d rows, want one of:\n  %s", parts, len(found), strings.Join(labels, "\n  "))
}

func player(result *proto.RaidSimResult) *proto.UnitMetrics {
	return result.RaidMetrics.Parties[0].Players[0]
}

// rotationWarnings are what the rotation's actions say is wrong with them, a spell the character doesn't know
// among them.
func rotationWarnings(rsr *proto.RaidSimRequest) []string {
	stats := core.ComputeStats(&proto.ComputeStatsRequest{Raid: rsr.Raid, Encounter: rsr.Encounter})
	if stats.ErrorResult != "" {
		return []string{stats.ErrorResult}
	}
	rotation := stats.RaidStats.Parties[0].Players[0].RotationStats
	if rotation == nil {
		return nil
	}
	var warnings []string
	for i, action := range rotation.PrepullActions {
		for _, w := range action.Warnings {
			warnings = append(warnings, fmt.Sprintf("prepull %d: %s", i, w))
		}
	}
	for i, action := range rotation.PriorityList {
		for _, w := range action.Warnings {
			warnings = append(warnings, fmt.Sprintf("prio %d: %s", i, w))
		}
	}
	return warnings
}

func wanted(key string, ids []string) bool {
	return len(ids) == 0 || slices.ContainsFunc(ids, func(id string) bool { return strings.Contains(key, id) })
}

// report prints the row's DPS, then per iteration each action's casts and damage, the auras' uptimes in
// seconds and the pets' DPS and auras.
func report(out io.Writer, r row, result *proto.RaidSimResult, ids []string) {
	pm := player(result)
	n := float64(r.rsr.SimOptions.Iterations)
	fmt.Fprintf(out, "%s iters=%d seed=%d: dps %.3f (se %.2f) tps %.3f hps %.3f\n", r.label, r.rsr.SimOptions.Iterations,
		r.rsr.SimOptions.RandomSeed, pm.Dps.Avg, pm.Dps.Stdev/math.Sqrt(n), pm.Threat.Avg, pm.Hps.Avg)

	type line struct {
		key         string
		casts, dmg  float64
		hits, crits float64
	}
	var lines []line
	for _, action := range pm.Actions {
		l := line{key: core.ProtoToActionID(action.Id).String()}
		for _, t := range action.Targets {
			l.casts += float64(t.Casts)
			l.dmg += t.Damage
			l.hits += float64(t.Hits + t.Crits + t.Glances + t.Blocks + t.Crushes)
			l.crits += float64(t.Crits)
		}
		if (l.casts != 0 || l.dmg != 0) && wanted(l.key, ids) {
			lines = append(lines, l)
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].dmg > lines[j].dmg })
	for _, l := range lines {
		fmt.Fprintf(out, "  %-40s casts %7.2f  hits %7.2f  crits %7.2f  dmg %10.0f\n", l.key, l.casts/n, l.hits/n, l.crits/n, l.dmg/n)
	}
	for _, aura := range auraLines(pm, ids) {
		fmt.Fprintf(out, "  aura %s\n", aura)
	}
	for _, pet := range pm.Pets {
		fmt.Fprintf(out, "  pet %s dps %.2f\n", pet.Name, pet.Dps.Avg)
		for _, aura := range auraLines(pet, ids) {
			fmt.Fprintf(out, "    aura %s\n", aura)
		}
	}
}

func auraLines(unit *proto.UnitMetrics, ids []string) []string {
	var lines []string
	for _, aura := range unit.Auras {
		key := core.ProtoToActionID(aura.Id).String()
		if aura.UptimeSecondsAvg > 0 && wanted(key, ids) {
			lines = append(lines, fmt.Sprintf("%-34s uptime %7.2f", key, aura.UptimeSecondsAvg))
		}
	}
	sort.Strings(lines)
	return lines
}

var (
	logTime = regexp.MustCompile(`^\[(-?[0-9.]+)\] `)
	// the first player's lines, whatever it's named
	logPlayer = regexp.MustCompile(`^\[-?[0-9.]+\] \[[^\]]*\(#1\)\] `)
	logCost   = regexp.MustCompile(` \(Cost = [^)]*\)$`)
	logAura   = regexp.MustCompile(`Aura (gained|faded): (\{[^}]*\})`)
)

// timelineCut is what -timeline, -ids and -match keep of a debug log.
type timelineCut struct {
	from, to float64
	ids      []string
	extra    *regexp.Regexp
}

func newTimelineCut(window string, ids []string, match string) (timelineCut, error) {
	cut := timelineCut{from: math.Inf(-1), to: math.Inf(1), ids: ids}
	before, after, ok := strings.Cut(window, ":")
	if !ok {
		return cut, fmt.Errorf("-timeline %q: want from:to in seconds, either side empty for the fight's end", window)
	}
	for _, bound := range []struct {
		text string
		into *float64
	}{{before, &cut.from}, {after, &cut.to}} {
		if bound.text == "" {
			continue
		}
		if _, err := fmt.Sscanf(bound.text, "%g", bound.into); err != nil {
			return cut, fmt.Errorf("-timeline %q: %q isn't a time in seconds", window, bound.text)
		}
	}
	if match != "" {
		var err error
		if cut.extra, err = regexp.Compile(match); err != nil {
			return cut, fmt.Errorf("-match: %w", err)
		}
	}
	return cut, nil
}

// lines cuts a debug log down to what the first player did between the two times: its casts but swings, the
// times nothing was castable, its ids' auras going on and off, and the lines match fits.
func (cut timelineCut) lines(log string) []string {
	from, to, ids, extra := cut.from, cut.to, cut.ids, cut.extra
	var lines []string
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimRight(line, "\r")
		m := logTime.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var at float64
		if _, err := fmt.Sscanf(m[1], "%g", &at); err != nil || at < from || at > to {
			continue
		}
		who := logPlayer.FindString(line)
		if who == "" {
			continue
		}
		rest := line[len(who):]
		aura := logAura.FindStringSubmatch(rest)
		switch {
		case strings.HasPrefix(rest, "Casting ") && !strings.Contains(rest, "{OtherID: 3"):
			rest = logCost.ReplaceAllString(rest, "")
		case strings.Contains(rest, "No available actions"):
		case aura != nil && len(ids) > 0 && wanted(aura[2], ids):
		case extra != nil && extra.MatchString(rest):
		default:
			continue
		}
		lines = append(lines, m[0]+rest)
	}
	return lines
}
