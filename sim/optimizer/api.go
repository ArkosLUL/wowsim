package optimizer

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/optimizer/raidctx"
	goproto "google.golang.org/protobuf/proto"
)

// SimCommit is the sim's git commit, stamped on every result. Builds can set it with
// -ldflags "-X github.com/wowsims/wotlk/sim/optimizer.SimCommit=<sha>"; without that it comes from
// the Go build's VCS info, which Docker builds don't have (.git is ignored).
var SimCommit string

func init() {
	if SimCommit == "" {
		SimCommit = vcsCommit()
	}
}

func vcsCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	revision, modified := "", false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if revision == "" {
		return "unknown"
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}

// Optimize finds the target's BiS. It always returns a result: failures go in its error_result, and
// a cancelled ctx returns the best found so far with cancelled set.
//
// A base raid with players besides the target is a raid batch: the target is simmed alone, with the
// buffs and debuffs the others give (raidctx.Derive). A lone target keeps its own buffs.
func Optimize(ctx context.Context, req *proto.OptimizeGearRequest, progress ProgressFunc) (result *proto.OptimizerResult) {
	start := time.Now()
	defer func() {
		if err := recover(); err != nil {
			result = &proto.OptimizerResult{
				SimCommit:   SimCommit,
				ErrorResult: fmt.Sprintf("%v\nStack Trace:\n%s", err, debug.Stack()),
			}
		}
	}()

	r, err := PrepareRequest(req)
	if err != nil {
		return errorResult(err)
	}
	if ctx.Err() != nil {
		return cancelledBeforeStart(r, start)
	}
	keep, err := WeightedMetrics(r.Settings)
	if err != nil {
		return errorResult(err)
	}
	simmed, err := simmedRequest(r)
	if err != nil {
		return errorResult(err)
	}
	return optimize(ctx, r, simmed, NewSimEvaluator(simmed, keep...), progress, start)
}

func errorResult(err error) *proto.OptimizerResult {
	return &proto.OptimizerResult{SimCommit: SimCommit, ErrorResult: err.Error()}
}

// cancelledBeforeStart is the seed, unscored, for a run cancelled before it simmed anything.
func cancelledBeforeStart(r *Request, start time.Time) *proto.OptimizerResult {
	seed := &proto.OptimizerLoadoutResult{Equipment: r.Seed.Equipment(), RacialTraits: r.Seed.RacialTraits}
	return &proto.OptimizerResult{
		Best:            seed,
		Seed:            goproto.Clone(seed).(*proto.OptimizerLoadoutResult),
		Settings:        r.Settings,
		TargetRaidIndex: int32(r.TargetIndex),
		SimCommit:       SimCommit,
		CatalogDate:     r.Pool.GetCatalogDate(),
		ElapsedSeconds:  time.Since(start).Seconds(),
		Cancelled:       true,
	}
}

// simmedRequest is r as the search sims it: for a raid batch, the target alone in its derived
// individual context, at raidctx.TargetIndex.
func simmedRequest(r *Request) (*Request, error) {
	if !hasOtherPlayers(r.Base.Raid, r.TargetIndex) {
		return r, nil
	}
	derived, err := raidctx.Derive(r.Base, r.TargetIndex)
	if err != nil {
		return nil, err
	}
	out := *r
	out.Base = derived
	out.TargetIndex = raidctx.TargetIndex
	return &out, nil
}

func hasOtherPlayers(raid *proto.Raid, targetIndex int) bool {
	numParties := int(raid.NumActiveParties)
	if numParties == 0 || numParties > len(raid.Parties) {
		numParties = len(raid.Parties)
	}
	for partyIndex, party := range raid.Parties[:numParties] {
		for slot, player := range party.GetPlayers() {
			if partyIndex*5+slot != targetIndex && player.GetClass() != proto.Class_ClassUnknown {
				return true
			}
		}
	}
	return false
}

// RunAsync runs Optimize in the background. Progress goes to reporter as optimizer_progress, then the
// result as final_optimize_result, and then reporter is closed. Drain it until it's closed.
func RunAsync(ctx context.Context, req *proto.OptimizeGearRequest, reporter chan *proto.ProgressMetrics) {
	go func() {
		defer close(reporter)
		result := Optimize(ctx, req, func(p *proto.OptimizerProgress) {
			select {
			case reporter <- &proto.ProgressMetrics{OptimizerProgress: p, CompletedSims: p.CompletedSims, TotalSims: p.TotalSims}:
			default:
				// readers only show the latest update, so skipping one beats stalling the search
			}
		})
		reporter <- &proto.ProgressMetrics{FinalOptimizeResult: result}
	}()
}

// The stages a run reports, in order.
var stages = []string{"Setup", "Objective", "Stat curves", "Effects", "Search", "Verify", "Alternatives"}

// run is one optimization: the request as asked (for the result) and as simmed, and what the stages
// have found so far.
type run struct {
	ctx    context.Context
	asked  *Request
	r      *Request
	eval   *countingEvaluator
	budget Budget
	// Iterations the whole run may sim, and per screening sim.
	total  int64
	screen int
	rng    *rand.Rand

	progress ProgressFunc
	progMu   sync.Mutex
	start    time.Time
	stage    int

	warnMu   sync.Mutex
	warnings []string

	pool *Pool
	// Why the seed isn't a legal pick, or nil.
	seedBroken error
	obj        *Objective
	resp       Response
	seedEval   *Evaluation
	seedJ      Estimate

	// The best loadout verified so far, and its evaluation; the seed until verification picks another.
	best     Loadout
	bestEval *Evaluation
	// What verification simmed, for a result cut short by a cancel.
	verified []*verifiedLoadout

	// The seed's character sheet, for caps.
	seedSheet func() (stats.Stats, error)
}

// traceHook, when set, gets a line for each decision a run makes. Tests set it to t.Logf.
var traceHook func(format string, args ...any)

func (r *run) trace(format string, args ...any) {
	if traceHook != nil {
		traceHook(format, args...)
	}
}

func optimize(ctx context.Context, asked, simmed *Request, eval Evaluator, progress ProgressFunc, start time.Time) *proto.OptimizerResult {
	budget := EffortBudget(asked.Settings.GetEffort())
	seed := simmed.Base.GetSimOptions().GetRandomSeed()
	if seed == 0 {
		seed = defaultRandomSeed
	}
	r := &run{
		ctx:      ctx,
		asked:    asked,
		r:        simmed,
		budget:   budget,
		total:    int64(budget.Evaluations) * int64(budget.Iterations),
		screen:   roundShards(budget.Iterations / 4),
		rng:      rand.New(rand.NewSource(seed)),
		progress: progress,
		start:    start,
		best:     simmed.Seed,
	}
	r.eval = &countingEvaluator{inner: eval, have: map[Point]int{}, after: r.report}
	r.seedSheet = sync.OnceValues(func() (stats.Stats, error) { return r.pool.finalStats(r.r.Seed) })
	return r.execute()
}

func (r *run) execute() *proto.OptimizerResult {
	settings := r.asked.Settings
	if settings.GetRacialMode() == proto.OptimizerRacialMode_OptimizerRacialSearch {
		r.warn("racial traits aren't searched yet: kept the seed's %s traits", r.r.Seed.RacialTraits)
	}
	if settings.GetRequireCritImmunity() {
		r.warn("crit immunity isn't enforced yet: the result may be critable")
	}
	r.report()

	pool, err := CompilePool(r.r)
	if err != nil {
		return r.fail(err)
	}
	r.pool = pool
	if err := pool.Check(r.r.Seed); err != nil {
		r.seedBroken = err
		r.warn("the seed gear breaks a rule (%v), so the best loadout that doesn't replaces it, whatever it scores", err)
	}
	if !r.anythingToChange() {
		r.warn("the pool offers nothing to change, so the seed wasn't simmed")
		return r.result(nil, nil)
	}

	r.setStage("Objective")
	obj, err := NewObjective(r.ctx, r.eval, r.r, r.budget.Iterations)
	if err != nil {
		return r.stop(err)
	}
	r.obj = obj
	for _, w := range obj.Warnings {
		r.warn("%s", w)
	}
	evals, err := r.evaluate([]Point{{Loadout: r.r.Seed}}, r.budget.Iterations)
	if err != nil {
		return r.stop(fmt.Errorf("seed sims: %w", err))
	}
	r.seedEval, r.bestEval = evals[0], evals[0]
	r.seedJ = obj.Score(r.seedEval)

	r.setStage("Stat curves")
	resp, rangeStats, err := r.measureCurves()
	if err != nil {
		return r.stop(err)
	}
	r.resp = resp

	s := newSurrogate(pool, r.r.Seed, resp)
	if err := s.setFloors(settings.GetStatMinimums(), rangeStats); err != nil {
		return r.fail(err)
	}
	r.setStage("Effects")
	if err := r.measureEffects(s); err != nil {
		return r.stop(err)
	}

	r.setStage("Search")
	se := newSearcher(s, r)
	if traceHook != nil {
		for slot := range se.opts {
			r.trace("slot %s: %d options of %d priced, %d in the pool", proto.ItemSlot(slot), len(se.opts[slot]), len(se.all[slot]), len(pool.Slots[slot]))
		}
		for st, c := range resp {
			r.trace("curve %s: breakpoint %.1f, slopes %.4f / %.4f", st.StatName(), c.Breakpoint, c.SlopeBelow, c.SlopeAbove)
		}
		for _, set := range s.sets {
			r.trace("set %q: 2pc %v %+v, 4pc %v %+v", set.name, set.hasTwo, set.two, set.hasFour, set.four)
		}
	}
	candidates, err := se.search()
	if err != nil {
		return r.stop(err)
	}
	for i, l := range candidates {
		r.trace("candidate %d: surrogate %+.1f", i, s.value(l)-s.value(r.r.Seed))
	}

	r.setStage("Verify")
	verified, err := r.verify(s, candidates)
	if err != nil {
		return r.stop(err)
	}
	r.verified = verified

	r.setStage("Alternatives")
	alternatives, verified, err := r.neighborhood(s, verified)
	if err != nil {
		return r.stop(err)
	}
	return r.result(verified, alternatives)
}

// anythingToChange: an unlocked slot with a candidate. Gems alone can't change anything: Gem leaves
// locked slots alone, and an unlocked slot's item has to come from the pool.
func (r *run) anythingToChange() bool {
	for slot, cands := range r.pool.Slots {
		if !r.pool.Locked[slot] && len(cands) > 0 {
			return true
		}
	}
	return false
}

// stop ends the run early: a cancel returns the best verified so far, anything else fails it.
func (r *run) stop(err error) *proto.OptimizerResult {
	if r.ctx.Err() != nil {
		result := r.result(r.verified, nil)
		result.Cancelled = true
		return result
	}
	return r.fail(err)
}

func (r *run) fail(err error) *proto.OptimizerResult {
	return &proto.OptimizerResult{
		Settings:        r.asked.Settings,
		TargetRaidIndex: int32(r.asked.TargetIndex),
		SimCommit:       SimCommit,
		CatalogDate:     r.asked.Pool.GetCatalogDate(),
		TotalSims:       int32(r.eval.simmed()),
		ElapsedSeconds:  time.Since(r.start).Seconds(),
		Warnings:        r.allWarnings(),
		ErrorResult:     err.Error(),
	}
}

func (r *run) warn(format string, args ...any) {
	r.warnMu.Lock()
	defer r.warnMu.Unlock()
	msg := fmt.Sprintf(format, args...)
	if !slices.Contains(r.warnings, msg) {
		r.warnings = append(r.warnings, msg)
	}
}

func (r *run) allWarnings() []string {
	r.warnMu.Lock()
	defer r.warnMu.Unlock()
	return slices.Clone(r.warnings)
}

func (r *run) setStage(name string) {
	r.progMu.Lock()
	r.stage = slices.Index(stages, name)
	r.progMu.Unlock()
	r.report()
}

// report sends a progress update. Calls come from several goroutines, so they queue on the mutex:
// the ProgressFunc gets one at a time.
func (r *run) report() {
	if r.progress == nil {
		return
	}
	r.progMu.Lock()
	defer r.progMu.Unlock()
	p := &proto.OptimizerProgress{
		Stage:          stages[r.stage],
		CompletedSteps: int32(r.stage),
		TotalSteps:     int32(len(stages)),
		CompletedSims:  int32(r.eval.simmed()),
		TotalSims:      int32(max(r.total, r.eval.simmed())),
		RacialTraits:   r.r.Seed.RacialTraits,
		ElapsedSeconds: time.Since(r.start).Seconds(),
	}
	if r.obj != nil && r.bestEval != nil && r.seedEval != nil {
		p.BestScore = r.obj.Score(r.bestEval).Mean
		p.BestScoreDelta = r.obj.Delta(r.seedEval, r.bestEval).Mean
	}
	r.progress(p)
}

// evaluate sims points, dropping any whose sim fails and asking again. A dropped point gets a nil
// evaluation and a warning; the seed failing fails the call.
func (r *run) evaluate(points []Point, iterations int) ([]*Evaluation, error) {
	out := make([]*Evaluation, len(points))
	todo := make([]int, len(points))
	for i := range todo {
		todo[i] = i
	}
	for len(todo) > 0 {
		batch := make([]Point, len(todo))
		for i, j := range todo {
			batch[i] = points[j]
		}
		evals, err := r.eval.Evaluate(r.ctx, batch, iterations)
		var simErr *SimError
		if errors.As(err, &simErr) {
			n := len(todo)
			todo = slices.DeleteFunc(todo, func(j int) bool { return points[j] == simErr.Point })
			if simErr.Point == (Point{Loadout: r.r.Seed}) || len(todo) == n {
				return nil, err
			}
			r.warn("dropped a loadout whose sim failed: %s", firstLine(simErr.Message))
			continue
		}
		if err != nil {
			return nil, err
		}
		for i, j := range todo {
			out[j] = evals[i]
		}
		break
	}
	return out, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// countingEvaluator tracks how many iterations each point has, to count what a run sims and to
// price a batch before asking for it.
type countingEvaluator struct {
	inner Evaluator
	mu    sync.Mutex
	have  map[Point]int
	used  int64
	after func()
}

// roundShards rounds up to whole SimEvaluator shards, as it sims them.
func roundShards(iterations int) int {
	shards := max(1, (iterations+DefaultShardIterations-1)/DefaultShardIterations)
	return shards * DefaultShardIterations
}

func (c *countingEvaluator) Evaluate(ctx context.Context, points []Point, iterations int) ([]*Evaluation, error) {
	evals, err := c.inner.Evaluate(ctx, points, iterations)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	for i, p := range points {
		if n := evals[i].Iterations; n > c.have[p] {
			c.used += int64(n - c.have[p])
			c.have[p] = n
		}
	}
	c.mu.Unlock()
	if c.after != nil {
		c.after()
	}
	return evals, nil
}

// cost is how many iterations sims of points at iterations would add.
func (c *countingEvaluator) cost(points []Point, iterations int) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	want := roundShards(iterations)
	seen := make(map[Point]bool, len(points))
	var total int64
	for _, p := range points {
		if seen[p] {
			continue
		}
		seen[p] = true
		total += int64(max(0, want-c.have[p]))
	}
	return total
}

func (c *countingEvaluator) iterations(p Point) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.have[p]
}

func (c *countingEvaluator) simmed() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used
}

func (r *run) remaining() int64 {
	return r.total - r.eval.simmed()
}

func statsOf(resp Response) []stats.Stat {
	var out []stats.Stat
	for st := stats.Stat(0); st < stats.Len; st++ {
		if resp[st] != nil {
			out = append(out, st)
		}
	}
	return out
}

// result assembles the answer: the best verified loadout (the seed when nothing beat it), the seed,
// the verified top sets and the alternatives. verified is empty when the run stopped early.
func (r *run) result(verified []*verifiedLoadout, alternatives []*proto.OptimizerSlotAlternative) *proto.OptimizerResult {
	result := &proto.OptimizerResult{
		Settings:        r.asked.Settings,
		TargetRaidIndex: int32(r.asked.TargetIndex),
		SimCommit:       SimCommit,
		CatalogDate:     r.asked.Pool.GetCatalogDate(),
		Alternatives:    alternatives,
	}
	result.Seed = r.loadoutResult(r.r.Seed, r.seedEval)
	// the neighborhood resims the pick and the seed at more iterations, where its gain can shrink
	if r.best != r.r.Seed && !r.beatsSeed(r.bestEval) {
		r.warn("the pick's gain didn't hold up against the seed at %d iterations, so the seed stays", min(r.seedEval.Iterations, r.bestEval.Iterations))
		r.best, r.bestEval = r.r.Seed, r.seedEval
		// they were runners-up to the dropped pick, not to the seed
		result.Alternatives = nil
	}
	// the seed may have more iterations than when they were sorted, which moves their deltas a bit
	if r.seedEval != nil {
		r.sortVerified(verified)
	}
	if r.best == r.r.Seed {
		result.Best = goproto.Clone(result.Seed).(*proto.OptimizerLoadoutResult)
	} else {
		result.Best = r.loadoutResult(r.best, r.bestEval)
		result.Improved = true
	}
	// the pick first, then the rest by score; a ring or trinket swap isn't another set
	seen := map[[NumSlots]int32]bool{}
	addTop := func(l Loadout, eval *Evaluation) {
		if key := setKey(l); len(result.Top) < maxTop && !seen[key] {
			seen[key] = true
			result.Top = append(result.Top, r.loadoutResult(l, eval))
		}
	}
	if result.Improved {
		addTop(r.best, r.bestEval)
	}
	for _, v := range verified {
		addTop(v.loadout, v.eval)
	}
	result.TotalSims = int32(r.eval.simmed())
	result.ElapsedSeconds = time.Since(r.start).Seconds()
	result.Warnings = r.allWarnings()
	return result
}

// maxTop is how many verified sets a result lists.
const maxTop = 20

// loadoutResult scores l against the seed. eval is nil when l wasn't simmed.
func (r *run) loadoutResult(l Loadout, eval *Evaluation) *proto.OptimizerLoadoutResult {
	out := &proto.OptimizerLoadoutResult{
		Equipment:    l.Equipment(),
		RacialTraits: l.RacialTraits,
	}
	if eval != nil && r.obj != nil {
		out.Score = r.obj.Score(eval).Mean
		if r.seedEval != nil {
			d := r.obj.Delta(r.seedEval, eval)
			out.ScoreDelta, out.ScoreDeltaSe = d.Mean, d.SE
		}
		var m Metrics
		for i := range m {
			m[i] = eval.Metrics[i].Mean
		}
		out.Metrics = m.ToProto()
	}
	if r.pool == nil {
		return out
	}
	if err := r.pool.Check(l); err != nil {
		out.Warnings = append(out.Warnings, err.Error())
	}
	for _, c := range l.Items {
		if c.ItemID == 0 {
			continue
		}
		for _, id := range append([]int32{c.ItemID}, c.Gems[:]...) {
			if id != 0 && r.r.Catalog[id].GetHasEffect() && !ItemHasEffect(id) && !slices.Contains(out.UnmodeledEffectItemIds, id) {
				out.UnmodeledEffectItemIds = append(out.UnmodeledEffectItemIds, id)
			}
		}
	}
	sheet, err := r.pool.finalStats(l)
	if err != nil {
		out.Warnings = append(out.Warnings, err.Error())
		return out
	}
	out.FinalStats = &proto.UnitStats{Stats: sheet.ToFloatArray()}
	out.Caps = r.caps(sheet)
	return out
}

// caps are the breakpoints the response curves found where the stat drops off, on the sheet: the
// seed's sheet value plus the curve's breakpoint, which sits in gear stats. The cap-prone stats are
// ratings, which no multiplier scales, so the two line up. A breakpoint where the slope goes up (ArP
// in some rotations) isn't a cap.
func (r *run) caps(sheet stats.Stats) []*proto.OptimizerStatCap {
	if r.resp == nil {
		return nil
	}
	seedSheet, err := r.seedSheet()
	if err != nil {
		return nil
	}
	var out []*proto.OptimizerStatCap
	for _, st := range statsOf(r.resp) {
		c := r.resp[st]
		if !capProneStats[st] || c.SlopeAbove >= c.SlopeBelow {
			continue
		}
		out = append(out, &proto.OptimizerStatCap{Stat: proto.Stat(st), Value: sheet[st], Cap: seedSheet[st] + c.Breakpoint})
	}
	return out
}

// beatsSeed is the acceptance test against the seed. A seed that breaks a rule can't be the pick, so
// any verified loadout beats it.
func (r *run) beatsSeed(eval *Evaluation) bool {
	return r.seedBroken != nil || r.accepts(r.obj.Delta(r.seedEval, eval))
}

// accepts is the acceptance test: a paired gain over 2 standard errors and over 0.05% of the seed's J.
func (r *run) accepts(d Estimate) bool {
	return d.Mean > 2*d.SE && d.Mean > acceptFraction*abs(r.seedJ.Mean)
}

// acceptFraction is the smallest gain, as a fraction of the seed's J, that counts as an improvement.
const acceptFraction = 0.0005

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
