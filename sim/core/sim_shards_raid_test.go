package core_test

import (
	"bytes"
	"fmt"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"

	_ "github.com/wowsims/wotlk/sim/common"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	dpsDeathKnight "github.com/wowsims/wotlk/sim/deathknight/dps"
	"github.com/wowsims/wotlk/sim/hunter"
	"github.com/wowsims/wotlk/sim/mage"
	holyPaladin "github.com/wowsims/wotlk/sim/paladin/holy"
	"github.com/wowsims/wotlk/sim/rogue"
	"github.com/wowsims/wotlk/sim/warlock"
	protectionWarrior "github.com/wowsims/wotlk/sim/warrior/protection"
)

func init() {
	protectionWarrior.RegisterProtectionWarrior()
	rogue.RegisterRogue()
	dpsDeathKnight.RegisterDpsDeathknight()
	hunter.RegisterHunter()
	warlock.RegisterWarlock()
	mage.RegisterMage()
	holyPaladin.RegisterHolyPaladin()
}

// shardTestRaid is part of the optimizer's 25-man fixture: a tank the boss hits, melee, three pet
// classes, a caster and a healer, over five parties. The fixture's shamans stay out, since the core
// tests register fakes for their specs.
func shardTestRaid(t *testing.T, iterations int32) *proto.RaidSimRequest {
	t.Helper()
	data, err := os.ReadFile("../optimizer/raidctx/testdata/raid25.json")
	if err != nil {
		t.Fatal(err)
	}
	rsr := &proto.RaidSimRequest{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, rsr); err != nil {
		t.Fatal(err)
	}
	keep := map[string]bool{"Tankwar": true, "Combat": true, "Unholy": true, "Marks": true, "Arcane": true, "Demo": true, "Holypal": true}
	for _, party := range rsr.Raid.Parties {
		party.Players = slices.DeleteFunc(party.Players, func(p *proto.Player) bool { return !keep[p.Name] })
		if len(party.Players) == 0 {
			t.Fatal("a party lost every player")
		}
	}
	if rsr.Raid.Parties[0].Players[0].Name != "Tankwar" {
		t.Fatal("the tank isn't at raid index 0")
	}
	rsr.Raid.Tanks = rsr.Raid.Tanks[:1]
	rsr.SimOptions = &proto.SimOptions{Iterations: iterations, RandomSeed: 101, DebugFirstIteration: true}
	return rsr
}

// healthFightRaid ends on the boss's health, and the tank's healing model runs a presim first.
func healthFightRaid(t *testing.T, iterations int32) *proto.RaidSimRequest {
	rsr := shardTestRaid(t, iterations)
	rsr.Raid.Parties[0].Players[0].HealingModel = &proto.HealingModel{CadenceSeconds: 2}
	rsr.Encounter.UseHealth = true
	target := rsr.Encounter.Targets[0]
	targetStats := stats.FromFloatArray(target.Stats)
	targetStats[stats.Health] = 1_500_000
	target.Stats = targetStats.ToFloatArray()
	return rsr
}

func clone(rsr *proto.RaidSimRequest) *proto.RaidSimRequest {
	return googleProto.Clone(rsr).(*proto.RaidSimRequest)
}

func idKey(id *proto.ActionID) string {
	b, err := googleProto.MarshalOptions{Deterministic: true}.Marshal(id)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func eachUnit(result *proto.RaidSimResult, visit func(*proto.UnitMetrics)) {
	var walk func(*proto.UnitMetrics)
	walk = func(u *proto.UnitMetrics) {
		visit(u)
		for _, pet := range u.Pets {
			walk(pet)
		}
	}
	for _, party := range result.GetRaidMetrics().GetParties() {
		for _, player := range party.Players {
			walk(player)
		}
	}
	for _, target := range result.GetEncounterMetrics().GetTargets() {
		walk(target)
	}
}

// requireSameResult compares byte for byte, once the actions are sorted: a unit keeps them in a map.
func requireSameResult(t *testing.T, want, got *proto.RaidSimResult) {
	t.Helper()
	for _, result := range []*proto.RaidSimResult{want, got} {
		eachUnit(result, func(u *proto.UnitMetrics) {
			slices.SortFunc(u.Actions, func(a, b *proto.ActionMetrics) int { return strings.Compare(idKey(a.Id), idKey(b.Id)) })
		})
	}
	wantBytes, err := googleProto.MarshalOptions{Deterministic: true}.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	gotBytes, err := googleProto.MarshalOptions{Deterministic: true}.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(wantBytes, gotBytes) {
		diff := cmp.Diff(want, got, protocmp.Transform())
		t.Fatalf("results differ (-want +got):\n%.4000s", diff)
	}
}

// drain collects progress until the runner closes the channel.
func drain(progress chan *proto.ProgressMetrics) <-chan []*proto.ProgressMetrics {
	out := make(chan []*proto.ProgressMetrics, 1)
	go func() {
		var all []*proto.ProgressMetrics
		for pm := range progress {
			all = append(all, pm)
		}
		out <- all
	}()
	return out
}

func checkProgress(t *testing.T, all []*proto.ProgressMetrics, result *proto.RaidSimResult, iterations int32) {
	t.Helper()
	if len(all) == 0 {
		t.Fatal("no progress")
	}
	final := all[len(all)-1]
	if final.FinalRaidResult != result || final.CompletedIterations != iterations || final.TotalIterations != iterations {
		t.Errorf("final progress = %d/%d iterations, result %p, want %d and %p", final.CompletedIterations, final.TotalIterations, final.FinalRaidResult, iterations, result)
	}
	var last int32
	for _, pm := range all[:len(all)-1] {
		if pm.FinalRaidResult != nil || pm.CompletedIterations < last || pm.CompletedIterations > iterations {
			t.Errorf("progress before the final one: %v after %d", pm, last)
		}
		last = pm.CompletedIterations
	}
}

func TestRaidSimOneShardMatchesRunRaidSim(t *testing.T) {
	for _, tc := range []struct {
		name string
		rsr  *proto.RaidSimRequest
	}{
		{"duration", shardTestRaid(t, 40)},
		{"health with presim", healthFightRaid(t, 20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want := core.RunRaidSim(clone(tc.rsr))
			if want.ErrorResult != "" {
				t.Fatal(want.ErrorResult)
			}
			progress := make(chan *proto.ProgressMetrics, 10)
			reports := drain(progress)
			got := core.RunRaidSimShards(clone(tc.rsr), progress, 1)
			checkProgress(t, <-reports, got, tc.rsr.SimOptions.Iterations)
			if got.Logs == "" {
				t.Error("no first-iteration log")
			}
			requireSameResult(t, want, got)
		})
	}
}

// The shards are run by hand as RunRaidSim requests, and merged here from their protos.
func TestRaidSimShardsMatchHandMerge(t *testing.T) {
	const iterations, shards = 103, 4
	rsr := shardTestRaid(t, iterations)
	rsr.SimOptions.SaveAllValues = true

	merge := handMerge{t: t}
	var parts []*proto.RaidSimResult
	var first int32
	for k := range int32(shards) {
		size := int32(iterations / shards)
		if k < iterations%shards {
			size++
		}
		part := clone(rsr)
		part.SimOptions.Iterations = size
		part.SimOptions.RandomSeed += int64(first)
		part.SimOptions.DebugFirstIteration = k == 0
		result := core.RunRaidSim(part)
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		parts = append(parts, result)
		merge.sizes = append(merge.sizes, float64(size))
		merge.total += float64(size)
		first += size
	}

	progress := make(chan *proto.ProgressMetrics, 10)
	reports := drain(progress)
	got := core.RunRaidSimShards(clone(rsr), progress, shards)
	checkProgress(t, <-reports, got, iterations)
	merge.result(got, parts)

	// one seed and shard count, one result
	requireSameResult(t, got, core.RunRaidSimShards(clone(rsr), nil, shards))
}

func TestRaidSimShardsAgreeWithOneStream(t *testing.T) {
	const iterations = 300
	rsr := shardTestRaid(t, iterations)
	one := core.RunRaidSim(clone(rsr))
	sharded := core.RunRaidSimShards(clone(rsr), nil, 8)
	if one.ErrorResult != "" || sharded.ErrorResult != "" {
		t.Fatal(one.ErrorResult, sharded.ErrorResult)
	}

	// 3 standard errors of the difference between two independent runs
	agree := func(name string, want, got *proto.DistributionMetrics) {
		if limit := 3 * math.Sqrt(2) * want.Stdev / math.Sqrt(iterations); math.Abs(got.Avg-want.Avg) > limit {
			t.Errorf("%s: sharded %.2f, one stream %.2f, more than %.2f apart", name, got.Avg, want.Avg, limit)
		}
	}
	agree("raid dps", one.RaidMetrics.Dps, sharded.RaidMetrics.Dps)
	for i, party := range one.RaidMetrics.Parties {
		for j, player := range party.Players {
			other := sharded.RaidMetrics.Parties[i].Players[j]
			agree(player.Name+" dps", player.Dps, other.Dps)
			agree(player.Name+" hps", player.Hps, other.Hps)
			agree(player.Name+" dtps", player.Dtps, other.Dtps)
		}
	}
}

// a player without a spec panics while its shard builds the raid
func TestRaidSimShardsReportAPanic(t *testing.T) {
	rsr := shardTestRaid(t, 40)
	rsr.Raid.Parties[1].Players[0].Spec = nil
	progress := make(chan *proto.ProgressMetrics, 10)
	reports := drain(progress)
	got := core.RunRaidSimShards(rsr, progress, 4)
	all := <-reports
	if got.ErrorResult == "" || got.RaidMetrics != nil {
		t.Fatalf("result = %v, want only an error", got)
	}
	if !strings.Contains(got.ErrorResult, "Stack Trace") {
		t.Errorf("error has no stack: %q", got.ErrorResult)
	}
	if final := all[len(all)-1]; final.FinalRaidResult != got {
		t.Errorf("last progress = %v, want the error result", final)
	}
	for _, pm := range all[:len(all)-1] {
		if pm.FinalRaidResult != nil {
			t.Errorf("a final result came before the last progress: %v", pm)
		}
	}
}

// handMerge checks a sharded result against its shards' results, merged field by field.
type handMerge struct {
	t     *testing.T
	sizes []float64 // each shard's iterations
	total float64
}

func (h *handMerge) near(path string, got, want, tolerance float64) {
	h.t.Helper()
	if math.IsNaN(got) && math.IsNaN(want) {
		return
	}
	if !(math.Abs(got-want) <= tolerance) {
		h.t.Errorf("%s = %v, want %v", path, got, want)
	}
}

func relative(values ...float64) float64 {
	scale := 1.0
	for _, v := range values {
		scale = max(scale, math.Abs(v))
	}
	return 1e-9 * scale
}

// stdev pools each shard's mean and standard deviation. A NaN counts as 0: it's the square root of
// a variance that rounding took below 0.
func (h *handMerge) stdev(path string, got float64, means, stdevs, weights []float64, mean float64) {
	h.t.Helper()
	var sumSq, n float64
	for i := range means {
		sd := stdevs[i]
		if math.IsNaN(sd) {
			sd = 0
		}
		sumSq += weights[i] * (sd*sd + means[i]*means[i])
		n += weights[i]
	}
	want := math.Sqrt(max(0, sumSq/n-mean*mean))
	if math.IsNaN(got) {
		got = 0
	}
	h.near(path, got, want, 1e-6*max(1, math.Abs(mean)))
}

func (h *handMerge) dist(path string, got *proto.DistributionMetrics, parts []*proto.DistributionMetrics) {
	h.t.Helper()
	if got == nil {
		if slices.ContainsFunc(parts, func(p *proto.DistributionMetrics) bool { return p != nil }) {
			h.t.Errorf("%s is missing", path)
		}
		return
	}
	want := &proto.DistributionMetrics{Hist: map[int32]int32{}}
	var means, stdevs []float64
	for k, p := range parts {
		want.Avg += p.Avg * h.sizes[k] / h.total
		means, stdevs = append(means, p.Avg), append(stdevs, p.Stdev)
		if k == 0 || p.Max > want.Max {
			want.Max, want.MaxSeed = p.Max, p.MaxSeed
		}
		if k == 0 || p.Min <= want.Min {
			want.Min, want.MinSeed = p.Min, p.MinSeed
		}
		for bucket, count := range p.Hist {
			want.Hist[bucket] += count
		}
		want.AllValues = append(want.AllValues, p.AllValues...)
	}
	h.near(path+".avg", got.Avg, want.Avg, relative(want.Avg))
	h.stdev(path+".stdev", got.Stdev, means, stdevs, h.sizes, want.Avg)
	if got.Max != want.Max || got.MaxSeed != want.MaxSeed || got.Min != want.Min || got.MinSeed != want.MinSeed {
		h.t.Errorf("%s max %v (seed %d) min %v (seed %d), want %v (%d) and %v (%d)", path,
			got.Max, got.MaxSeed, got.Min, got.MinSeed, want.Max, want.MaxSeed, want.Min, want.MinSeed)
	}
	if !maps.Equal(got.Hist, want.Hist) {
		h.t.Errorf("%s hist = %v, want %v", path, got.Hist, want.Hist)
	}
	if !slices.Equal(got.AllValues, want.AllValues) {
		h.t.Errorf("%s all values differ", path)
	}
}

func pick[T, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

func (h *handMerge) weighted(values []float64) float64 {
	var sum float64
	for k, v := range values {
		sum += v * h.sizes[k] / h.total
	}
	return sum
}

func (h *handMerge) unit(path string, got *proto.UnitMetrics, parts []*proto.UnitMetrics) {
	h.t.Helper()
	path += "/" + got.Name
	if got.Name != parts[0].Name || got.UnitIndex != parts[0].UnitIndex {
		h.t.Errorf("%s is unit %d, want %s at %d", path, got.UnitIndex, parts[0].Name, parts[0].UnitIndex)
	}
	for _, field := range []struct {
		name string
		get  func(*proto.UnitMetrics) *proto.DistributionMetrics
	}{
		{"dps", (*proto.UnitMetrics).GetDps},
		{"dpasp", (*proto.UnitMetrics).GetDpasp},
		{"threat", (*proto.UnitMetrics).GetThreat},
		{"dtps", (*proto.UnitMetrics).GetDtps},
		{"tmi", (*proto.UnitMetrics).GetTmi},
		{"hps", (*proto.UnitMetrics).GetHps},
		{"tto", (*proto.UnitMetrics).GetTto},
	} {
		h.dist(path+"."+field.name, field.get(got), pick(parts, field.get))
	}
	oom := h.weighted(pick(parts, (*proto.UnitMetrics).GetSecondsOomAvg))
	h.near(path+".secondsOomAvg", got.SecondsOomAvg, oom, relative(oom))
	death := h.weighted(pick(parts, (*proto.UnitMetrics).GetChanceOfDeath))
	h.near(path+".chanceOfDeath", got.ChanceOfDeath, death, relative(death))

	h.actions(path, got.Actions, pick(parts, (*proto.UnitMetrics).GetActions))
	h.auras(path, got.Auras, pick(parts, (*proto.UnitMetrics).GetAuras))
	h.resources(path, got.Resources, pick(parts, (*proto.UnitMetrics).GetResources))

	if len(got.Pets) != len(parts[0].Pets) {
		h.t.Fatalf("%s has %d pets, want %d", path, len(got.Pets), len(parts[0].Pets))
	}
	for i, pet := range got.Pets {
		h.unit(path, pet, pick(parts, func(u *proto.UnitMetrics) *proto.UnitMetrics { return u.Pets[i] }))
	}
}

func (h *handMerge) actions(path string, got []*proto.ActionMetrics, parts [][]*proto.ActionMetrics) {
	h.t.Helper()
	want := map[string]*proto.ActionMetrics{}
	for _, actions := range parts {
		for _, action := range actions {
			sum := want[idKey(action.Id)]
			if sum == nil {
				sum = &proto.ActionMetrics{Id: action.Id, IsMelee: action.IsMelee}
				want[idKey(action.Id)] = sum
			}
			for i, target := range action.Targets {
				if i == len(sum.Targets) {
					sum.Targets = append(sum.Targets, &proto.TargetedActionMetrics{UnitIndex: target.UnitIndex})
				}
				s := sum.Targets[i]
				s.Casts += target.Casts
				s.Hits += target.Hits
				s.Crits += target.Crits
				s.Misses += target.Misses
				s.Dodges += target.Dodges
				s.Parries += target.Parries
				s.Blocks += target.Blocks
				s.Glances += target.Glances
				s.Crushes += target.Crushes
				s.Damage += target.Damage
				s.Threat += target.Threat
				s.Healing += target.Healing
				s.Shielding += target.Shielding
				s.CastTimeMs += target.CastTimeMs
			}
		}
	}
	if len(got) != len(want) {
		h.t.Errorf("%s has %d actions, want %d", path, len(got), len(want))
	}
	for _, action := range got {
		sum := want[idKey(action.Id)]
		name := fmt.Sprintf("%s action %v", path, action.Id)
		if sum == nil || sum.IsMelee != action.IsMelee || len(sum.Targets) != len(action.Targets) {
			h.t.Errorf("%s = %v, want %v", name, action, sum)
			continue
		}
		for i, g := range action.Targets {
			w := sum.Targets[i]
			if g.UnitIndex != w.UnitIndex || g.Casts != w.Casts || g.Hits != w.Hits || g.Crits != w.Crits ||
				g.Misses != w.Misses || g.Dodges != w.Dodges || g.Parries != w.Parries || g.Blocks != w.Blocks ||
				g.Glances != w.Glances || g.Crushes != w.Crushes {
				h.t.Errorf("%s target %d counts = %v, want %v", name, i, g, w)
			}
			h.near(name+" damage", g.Damage, w.Damage, relative(w.Damage))
			h.near(name+" threat", g.Threat, w.Threat, relative(w.Threat))
			h.near(name+" healing", g.Healing, w.Healing, relative(w.Healing))
			h.near(name+" shielding", g.Shielding, w.Shielding, relative(w.Shielding))
			// each shard rounds its own total down to whole ms
			h.near(name+" cast time", g.CastTimeMs, w.CastTimeMs, float64(len(parts)))
		}
	}
}

// Auras match by id and, for auras sharing one, by order. A shard without one doesn't count toward
// its averages.
func (h *handMerge) auras(path string, got []*proto.AuraMetrics, parts [][]*proto.AuraMetrics) {
	h.t.Helper()
	keyed := func(auras []*proto.AuraMetrics) map[string]*proto.AuraMetrics {
		out := map[string]*proto.AuraMetrics{}
		seen := map[string]int{}
		for _, aura := range auras {
			id := idKey(aura.Id)
			out[fmt.Sprintf("%s#%d", id, seen[id])] = aura
			seen[id]++
		}
		return out
	}
	gotByKey := keyed(got)
	partsByKey := pick(parts, keyed)
	union := map[string]bool{}
	for _, byKey := range partsByKey {
		for key := range byKey {
			union[key] = true
		}
	}
	if len(gotByKey) != len(union) {
		h.t.Errorf("%s has %d auras, want %d", path, len(gotByKey), len(union))
	}
	for key, aura := range gotByKey {
		name := fmt.Sprintf("%s aura %v", path, aura.Id)
		var weights, uptimes, stdevs []float64
		var n, uptime, procs float64
		for k, byKey := range partsByKey {
			if p := byKey[key]; p != nil {
				weights, uptimes, stdevs = append(weights, h.sizes[k]), append(uptimes, p.UptimeSecondsAvg), append(stdevs, p.UptimeSecondsStdev)
				n += h.sizes[k]
				uptime += p.UptimeSecondsAvg * h.sizes[k]
				procs += p.ProcsAvg * h.sizes[k]
			}
		}
		if n == 0 {
			h.t.Errorf("%s isn't in any shard", name)
			continue
		}
		h.near(name+" uptime", aura.UptimeSecondsAvg, uptime/n, relative(uptime/n))
		h.near(name+" procs", aura.ProcsAvg, procs/n, relative(procs/n))
		h.stdev(name+" uptime stdev", aura.UptimeSecondsStdev, uptimes, stdevs, weights, uptime/n)
	}
}

// Resources sum by id and type: a shard leaves out the ones that had no events.
func (h *handMerge) resources(path string, got []*proto.ResourceMetrics, parts [][]*proto.ResourceMetrics) {
	h.t.Helper()
	sum := func(into map[string]*proto.ResourceMetrics, resources []*proto.ResourceMetrics) {
		for _, r := range resources {
			key := fmt.Sprintf("%s/%d", idKey(r.Id), r.Type)
			s := into[key]
			if s == nil {
				s = &proto.ResourceMetrics{Id: r.Id, Type: r.Type}
				into[key] = s
			}
			s.Events += r.Events
			s.Gain += r.Gain
			s.ActualGain += r.ActualGain
		}
	}
	gotSums, wantSums := map[string]*proto.ResourceMetrics{}, map[string]*proto.ResourceMetrics{}
	sum(gotSums, got)
	for _, resources := range parts {
		sum(wantSums, resources)
	}
	if len(gotSums) != len(wantSums) {
		h.t.Errorf("%s has %d resources, want %d", path, len(gotSums), len(wantSums))
	}
	for key, g := range gotSums {
		w := wantSums[key]
		name := fmt.Sprintf("%s resource %v %v", path, g.Id, g.Type)
		if w == nil || g.Events != w.Events {
			h.t.Errorf("%s = %v, want %v", name, g, w)
			continue
		}
		h.near(name+" gain", g.Gain, w.Gain, relative(w.Gain))
		h.near(name+" actual gain", g.ActualGain, w.ActualGain, relative(w.ActualGain))
	}
}

func (h *handMerge) result(got *proto.RaidSimResult, parts []*proto.RaidSimResult) {
	h.t.Helper()
	if got.ErrorResult != "" {
		h.t.Fatal(got.ErrorResult)
	}
	raids := pick(parts, (*proto.RaidSimResult).GetRaidMetrics)
	h.dist("raid.dps", got.RaidMetrics.Dps, pick(raids, (*proto.RaidMetrics).GetDps))
	h.dist("raid.hps", got.RaidMetrics.Hps, pick(raids, (*proto.RaidMetrics).GetHps))
	for i, party := range got.RaidMetrics.Parties {
		partyParts := pick(raids, func(r *proto.RaidMetrics) *proto.PartyMetrics { return r.Parties[i] })
		path := fmt.Sprintf("party %d", i)
		h.dist(path+".dps", party.Dps, pick(partyParts, (*proto.PartyMetrics).GetDps))
		h.dist(path+".hps", party.Hps, pick(partyParts, (*proto.PartyMetrics).GetHps))
		for j, player := range party.Players {
			h.unit(path, player, pick(partyParts, func(p *proto.PartyMetrics) *proto.UnitMetrics { return p.Players[j] }))
		}
	}
	for i, target := range got.EncounterMetrics.Targets {
		h.unit("targets", target, pick(parts, func(r *proto.RaidSimResult) *proto.UnitMetrics { return r.EncounterMetrics.Targets[i] }))
	}

	if got.Logs == "" || got.Logs != parts[0].Logs {
		h.t.Errorf("the log isn't the first shard's first iteration")
	}
	if got.FirstIterationDuration != parts[0].FirstIterationDuration {
		h.t.Errorf("first iteration duration = %v, want %v", got.FirstIterationDuration, parts[0].FirstIterationDuration)
	}
	avg := h.weighted(pick(parts, (*proto.RaidSimResult).GetAvgIterationDuration))
	h.near("avg iteration duration", got.AvgIterationDuration, avg, relative(avg))
}
