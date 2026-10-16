package optimizer

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"google.golang.org/protobuf/encoding/protojson"
)

type batchPick struct {
	raider string
	phase  int32
	result *proto.OptimizerResult
}

// batchPicks reads the tank runs out of a BiS Batch export (the acbis driver's batch-stage1.json).
// Like the requests it's never committed, so the path comes from the environment.
func batchPicks(tb testing.TB) []batchPick {
	tb.Helper()
	path := os.Getenv("BIS_PICKS")
	if path == "" {
		tb.Skip("BIS_PICKS names a BiS Batch export")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	var export struct {
		Entries []struct {
			Raider       string          `json:"raider"`
			ContentPhase int32           `json:"contentPhase"`
			Stage        int32           `json:"stage"`
			Result       json.RawMessage `json:"result"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(data, &export); err != nil {
		tb.Fatal(err)
	}
	var out []batchPick
	for _, e := range export.Entries {
		result := &proto.OptimizerResult{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(e.Result, result); err != nil {
			tb.Fatalf("%s P%d: %v", e.Raider, e.ContentPhase, err)
		}
		if e.Stage == 1 && result.GetSettings().GetTankSurvival() > 0 && result.GetBest().GetEquipment() != nil {
			out = append(out, batchPick{e.Raider, e.ContentPhase, result})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].raider != out[j].raider {
			return out[i].raider < out[j].raider
		}
		return out[i].phase < out[j].phase
	})
	return out
}

// Solid Stormjewel, +30 Stamina, and the jewelcrafter's Solid Dragon's Eye, +51 and 3 at most. The
// server catalog has both from phase 1.
const (
	solidStormjewel = 45880
	solidDragonsEye = 36767
	maxDragonsEyes  = 3
)

// staminaGemmed is l with a Stamina gem in every socket but the meta's: about the most health the
// phase's gear can hold, since it keeps l's items and enchants.
func staminaGemmed(l Loadout, jeweler bool) Loadout {
	eyes := 0
	for slot := range l.Items {
		for i, id := range l.Items[slot].Gems {
			if gem, _ := core.LookupGem(id); id == 0 || gem.Color == proto.GemColor_GemColorMeta {
				continue
			}
			l.Items[slot].Gems[i] = solidStormjewel
			if jeweler && eyes < maxDragonsEyes {
				l.Items[slot].Gems[i] = solidDragonsEye
				eyes++
			}
		}
	}
	return l
}

// Short names for the stats gems carry. Hit, crit and haste gems set the melee and spell stat both.
var gemStatNames = map[stats.Stat]string{
	stats.Strength: "Str", stats.Agility: "Agi", stats.Stamina: "Sta", stats.Intellect: "Int", stats.Spirit: "Spi",
	stats.MeleeHit: "hit", stats.SpellHit: "hit", stats.MeleeCrit: "crit", stats.SpellCrit: "crit",
	stats.MeleeHaste: "haste", stats.SpellHaste: "haste", stats.Expertise: "Exp", stats.Defense: "Def",
	stats.Dodge: "Dodge", stats.Parry: "Parry", stats.Resilience: "Resil", stats.Block: "Block",
	stats.BlockValue: "Block value", stats.AttackPower: "AP", stats.SpellPower: "SP", stats.ArmorPenetration: "ArP",
}

// gemKinds counts l's gems but the meta by the stats they give, most first, and how many give Stamina.
func gemKinds(l Loadout) (kinds string, stamina, total int) {
	counts := map[string]int{}
	for _, c := range l.Items {
		for _, id := range c.Gems {
			gem, ok := core.LookupGem(id)
			if id == 0 || gem.Color == proto.GemColor_GemColorMeta {
				continue
			}
			total++
			var names []string
			switch {
			case !ok:
				names = []string{fmt.Sprintf("gem %d", id)}
			case gem.Color == proto.GemColor_GemColorPrismatic:
				names = []string{"prismatic"}
			default:
				for s, v := range gem.Stats {
					name, known := gemStatNames[stats.Stat(s)]
					if !known {
						name = stats.Stat(s).StatName()
					}
					if v != 0 && !slices.Contains(names, name) {
						names = append(names, name)
					}
				}
				if gem.Stats[stats.Stamina] > 0 {
					stamina++
				}
			}
			counts[strings.Join(names, "+")]++
		}
	}
	order := slices.Collect(maps.Keys(counts))
	sort.Slice(order, func(i, j int) bool {
		if counts[order[i]] != counts[order[j]] {
			return counts[order[i]] > counts[order[j]]
		}
		return order[i] < order[j]
	})
	parts := make([]string, len(order))
	for i, kind := range order {
		parts[i] = fmt.Sprintf("%s %d", kind, counts[kind])
	}
	return strings.Join(parts, ", "), stamina, total
}

// TestMeasureTankPicks takes the tank picks of a BiS Batch export back into the fight they were scored
// on and prints, per run: the run's warnings (ties and gem twins among them); the worn gear, the pick
// and the pick with every socket a Stamina gem, each with its health margin and time to live; the
// pick's stats and gems; the survival normalizers at the worn gear and at the pick; what one more
// point of stamina, agility, dodge and defense rating is worth at the pick, term by term; and what
// the margin gains per point of Stamina against the health it adds. Where the run left death chance
// out, it also walks the pick's Stamina toward the health at which it starts to die, or to survive.
// It asserts nothing.
//
//	BIS_REQUESTS=<dir> BIS_PICKS=<batch-stage1.json> go test -tags with_db -run TestMeasureTankPicks -v ./sim/optimizer/
func TestMeasureTankPicks(t *testing.T) {
	picks := batchPicks(t)
	iterations := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal).Iterations
	ctx := context.Background()
	const bump = 100.0

	for _, p := range picks {
		tag := fmt.Sprintf("%s P%d", p.raider, p.phase)
		r := tankFightRequest(t, p.raider, p.phase)
		eval := NewSimEvaluator(r, tankMetrics...)
		obj, err := NewObjective(ctx, eval, r, iterations)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		best := p.result.GetBest()
		race := best.GetRacialTraits()
		if race == proto.Race_RaceUnknown {
			race = r.Seed.RacialTraits
		}
		pick, err := LoadoutFromProto(best.GetEquipment(), race)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		health := func(l Loadout, offset stats.Stats) float64 {
			sheet, err := playerSheet(r.Base, r.TargetIndex, l, offset)
			if err != nil {
				t.Fatalf("%s: %v", tag, err)
			}
			return sheet.FinalStats[stats.Health]
		}
		stamina := func(amount float64) stats.Stats {
			var offset stats.Stats
			offset[stats.Stamina] = amount
			return offset
		}

		jeweler := slices.Contains(core.ProtoToProfessions(r.Target()), proto.Profession_Jewelcrafting)
		gemmed := staminaGemmed(pick, jeweler)
		var down, up stats.Stats
		down[stats.Armor], up[stats.Armor] = -normalizerOffset(stats.Armor), normalizerOffset(stats.Armor)
		points := []Point{{Loadout: r.Equipped}, {Loadout: pick}, {Loadout: gemmed}, {Loadout: pick, Offset: down}, {Loadout: pick, Offset: up}}
		const bumped = 5
		for _, o := range tankOffsets {
			var offset stats.Stats
			offset[o.stat] = bump
			points = append(points, Point{Loadout: pick, Offset: offset})
		}
		evals, err := eval.Evaluate(ctx, points, iterations)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		worn, picked := evals[0], evals[1]
		lives := timesToLive(t, r, points[:3], iterations)
		line := func(what string, l Loadout, e *Evaluation, life Estimate) {
			margin := e.Metrics[MetricHealthMargin]
			t.Logf("%s %s: health %.0f, DTPS %.0f TMI %.1f TPS %.0f death %.4f margin %.0f ± %.0f, time to live %.2f ± %.2f s", tag, what,
				health(l, stats.Stats{}), e.Metrics[MetricDTPS].Mean, e.Metrics[MetricTMI].Mean, e.Metrics[MetricTPS].Mean,
				e.Metrics[MetricPDeath].Mean, margin.Mean, margin.SE, life.Mean, life.SE)
		}
		t.Logf("%s: pinned %.0f HPS, defensives at %.2f, warnings %v", tag, r.Target().GetHealingModel().GetHps(),
			r.Target().GetCooldowns().GetHpPercentForDefensives(), obj.Warnings)
		for _, w := range p.result.GetWarnings() {
			t.Logf("%s run warning: %s", tag, w)
		}
		line("worn", r.Equipped, worn, lives[0])
		line("pick", pick, picked, lives[1])
		line("pick, every socket Stamina", gemmed, evals[2], lives[2])
		sheet, err := playerSheet(r.Base, r.TargetIndex, pick, stats.Stats{})
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		s := sheet.FinalStats
		kinds, staminaGems, gems := gemKinds(pick)
		t.Logf("%s pick: Sta %.0f Agi %.0f Dodge %.0f Def %.0f Parry %.0f Hit %.0f Exp %.0f Armor %.0f; gems %s; Stamina in %d of %d", tag,
			s[stats.Stamina], s[stats.Agility], s[stats.Dodge], s[stats.Defense], s[stats.Parry], s[stats.MeleeHit], s[stats.Expertise],
			s[stats.Armor], kinds, staminaGems, gems)
		scored, seed := best.GetMetrics(), p.result.GetSeed().GetMetrics()
		t.Logf("%s as the run scored them: worn death %.4f DTPS %.0f margin %.0f, pick death %.4f DTPS %.0f TMI %.1f TPS %.0f margin %.0f; %d sims", tag,
			seed.GetPDeath(), seed.GetDtps(), seed.GetHealthMargin(), scored.GetPDeath(), scored.GetDtps(), scored.GetTmi(), scored.GetTps(),
			scored.GetHealthMargin(), p.result.GetTotalSims())

		// J as the run's last search scored it: a second search counted death chance from its first
		// pick's normalizer, taken here at the final pick
		judge := obj
		if slices.ContainsFunc(p.result.GetWarnings(), func(w string) bool { return strings.Contains(w, "a second search counted death chance") }) {
			dm, err := measureDeath(ctx, eval, r, pick, iterations)
			if err != nil {
				t.Fatalf("%s: %v", tag, err)
			}
			second := *obj
			second.Warnings = slices.Clone(obj.Warnings)
			second.countDeathAt(dm)
			judge = &second
			t.Logf("%s: scored with death chance from the pick, %.1f se: coefficients %s", tag, seCount(MetricPDeath, dm.Norm), coefficientsText(judge))
		}
		d := judge.Delta(worn, picked)
		t.Logf("%s: pick against worn dJ %+.0f (se %.0f)", tag, d.Mean, d.SE)
		t.Logf("%s normalizers per Armor point, worn: %s", tag, normalizersText(obj.Normalizers))
		t.Logf("%s normalizers per Armor point, pick: %s", tag, normalizersText(armorNormalizers(evals[3], evals[4])))
		t.Logf("%s at the pick, per Stamina point: %s", tag,
			marginPerStamina(picked, evals[bumped], health(pick, points[bumped].Offset)-health(pick, stats.Stats{}), bump))
		for i, o := range tankOffsets {
			e := evals[bumped+i]
			d := judge.Delta(picked, e)
			t.Logf("%s at the pick: dJ per %s point %+.4f (se %.4f), per socket of %g %+.0f; by term %s; dDeath %+.6f",
				tag, o.name, d.Mean/bump, d.SE/bump, o.gem, d.Mean/bump*o.gem, byTerm(judge, picked, e, bump), Delta(picked, e, MetricPDeath).Mean/bump)
		}

		if obj.coef[MetricPDeath] != 0 {
			continue
		}
		// the run scored without death chance: walk the pick's health toward where it would count
		edge, step := 1.0, 250.0
		if picked.Metrics[MetricPDeath].Mean < 0.5 {
			edge, step = 0, -step
		}
		sweep := func(from, step float64, n int) (amounts []float64, deaths []float64) {
			var points []Point
			for i := 1; i <= n; i++ {
				// a self-heal off max health panics once Stamina goes far enough under zero
				if health(pick, stamina(from+float64(i)*step)) < 5000 {
					break
				}
				amounts = append(amounts, from+float64(i)*step)
				points = append(points, Point{Loadout: pick, Offset: stamina(from + float64(i)*step)})
			}
			evals, err := eval.Evaluate(ctx, points, iterations)
			if err != nil {
				t.Fatalf("%s: %v", tag, err)
			}
			for _, e := range evals {
				deaths = append(deaths, e.Metrics[MetricPDeath].Mean)
			}
			return amounts, deaths
		}
		amounts, deaths := sweep(0, step, 24)
		left := -1
		for i, death := range deaths {
			t.Logf("%s pick %+.0f Stamina: health %.0f, death %.4f", tag, amounts[i], health(pick, stamina(amounts[i])), death)
			if left < 0 && death != edge {
				left = i
			}
		}
		if left < 0 {
			t.Logf("%s: death chance stays at %.0f out to %+.0f Stamina", tag, edge, amounts[len(amounts)-1])
			continue
		}
		if picked.Metrics[MetricPDeath].Mean != edge {
			continue
		}
		// the tenths of the step it left in, then that step itself
		fine, fineDeaths := sweep(amounts[left]-step, step/10, 9)
		fine, fineDeaths = append(fine, amounts[left]), append(fineDeaths, deaths[left])
		for i, death := range fineDeaths {
			if death != edge {
				t.Logf("%s: death chance leaves %.0f at %+.0f Stamina, health %.0f (%.4f)", tag, edge, fine[i], health(pick, stamina(fine[i])), death)
				break
			}
		}
	}
}

// TestMeasureTankDeathAtPicks prints, per tank pick of a BiS Batch export, the death normalizer at the
// gear as worn and at the pick (what optimize's second search is gated on), the coefficient it gives
// death chance, and that coefficient capped at the Armor the loadout wears. It asserts nothing.
//
//	BIS_REQUESTS=<dir> BIS_PICKS=<batch-stage1.json> go test -tags with_db -run TestMeasureTankDeathAtPicks -v ./sim/optimizer/
func TestMeasureTankDeathAtPicks(t *testing.T) {
	picks := batchPicks(t)
	iterations := EffortBudget(proto.OptimizerEffort_OptimizerEffortNormal).Iterations
	ctx := context.Background()
	for _, p := range picks {
		tag := fmt.Sprintf("%s P%d", p.raider, p.phase)
		r := tankFightRequest(t, p.raider, p.phase)
		eval := NewSimEvaluator(r, MetricDTPS, MetricTMI, MetricTPS, MetricPDeath)
		best := p.result.GetBest()
		race := best.GetRacialTraits()
		if race == proto.Race_RaceUnknown {
			race = r.Seed.RacialTraits
		}
		pick, err := LoadoutFromProto(best.GetEquipment(), race)
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		weight := TankWeights(r.Settings.GetTankSurvival())[MetricPDeath]
		for _, at := range []struct {
			what string
			l    Loadout
		}{{"worn", r.Equipped}, {"pick", pick}} {
			dm, err := measureDeath(ctx, eval, r, at.l, iterations)
			if err != nil {
				t.Fatalf("%s: %v", tag, err)
			}
			coef, left := deathCoefficient(weight, dm)
			uncapped := weight / dm.Norm.Mean
			t.Logf("%s %s: dies %.4f (%.4f at -2000 Armor, %.4f at +2000), %.3g ± %.2g per Armor point = %.1f se, Armor %.0f, coefficient %.0f uncapped, %.0f counted; left out: %q",
				tag, at.what, dm.Dies, dm.Down, dm.Up, dm.Norm.Mean, dm.Norm.SE, -dm.Norm.Mean/max(dm.Norm.SE, 1e-12), dm.Armor, uncapped, coef, left)
		}
	}
}
