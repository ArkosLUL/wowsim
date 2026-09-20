package optimizer

import (
	"context"
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// hardCodedItemIDs are items and gems that sim code checks for by id. 62 of them have no registered
// effect, so core.HasItemEffect misses them. TestHardCodedItemIDsCoverTheSim scans the sim for new
// checks.
var hardCodedItemIDs = map[int32]bool{
	// core/character.go: crit multiplier metas
	34220: true, 32409: true, 41285: true, 41398: true, 41376: true,
	// core/character.go: party buffs from Atiesh and the TBC jewelcrafting necks
	22589: true, 22630: true, 24114: true, 24121: true, 24116: true,
	// core/consumes.go: alchemist stones; core/mana.go: Spark of Hope
	44322: true, 44323: true, 44324: true, 45703: true,
	// mage/mana_gems.go: Serpent-Coil Braid
	30720: true,
	// hunter/hunter.go: Thori'dal skips ammo
	34334: true,
	// deathknight/items.go: sigils
	39208: true, 40207: true, 40822: true, 40867: true, 40875: true, 45254: true,
	// druid: idols in the spell files and feral/rotation.go
	25667: true, 45270: true, 27744: true, 23198: true, 38365: true, 28372: true, 39757: true,
	29390: true, 40713: true, 27518: true, 40321: true, 31025: true, 40712: true,
	45509: true, 47668: true, 50456: true,
	// paladin: librams in the spell files, gladiator's gloves in items.go
	27917: true, 40337: true, 31033: true, 40191: true, 45510: true, 38362: true, 29388: true,
	40798: true, 40802: true, 40805: true, 40808: true, 40812: true, 51475: true,
	// shaman: totems in the spell files
	32330: true, 23199: true, 28248: true, 40267: true, 42598: true, 42597: true, 42596: true,
	42595: true, 28523: true, 38368: true, 45114: true, 40709: true, 38361: true, 45255: true,
	38367: true, 42607: true, 42608: true, 42609: true, 51507: true, 45169: true, 40322: true,
	27815: true, 40710: true,
}

// ItemHasEffect says whether an item or gem does something its stats don't show.
func ItemHasEffect(id int32) bool {
	return id != 0 && (core.HasItemEffect(id) || hardCodedItemIDs[id])
}

// EnchantHasEffect says the same for an enchant. Scourgebane, which druid/forms.go also checks by
// id, has a registered effect, so it needs no hard-coded list.
func EnchantHasEffect(id int32) bool {
	return id != 0 && (core.HasEnchantEffect(id) || core.HasWeaponEffect(id))
}

// NeedsSim says whether a slot's choice is worth something its stats don't show, so the search has
// to sim it rather than trust the response curves: an effect on the item, a gem or the enchant, or
// weapon damage. Set bonuses aren't covered; they need a whole set swapped in.
func NeedsSim(c ItemChoice) bool {
	if c.ItemID == 0 {
		return false
	}
	if ItemHasEffect(c.ItemID) || EnchantHasEffect(c.Enchant) {
		return true
	}
	for _, g := range c.Gems {
		if ItemHasEffect(g) {
			return true
		}
	}
	item, ok := core.LookupItem(c.ItemID)
	return !ok || item.WeaponDamageMax > 0
}

// gearStats is what a loadout's items, enchants, gems, socket bonuses and reforges add up to, in the
// same space as a Point's offsets: before the character's stat multipliers.
func gearStats(l Loadout) stats.Stats {
	var total stats.Stats
	for _, c := range l.Items {
		if c.ItemID == 0 {
			continue
		}
		item := core.NewItem(c.CoreSpec(), nil)
		total = total.Add(item.TotalStats())
	}
	return total
}

// MeasureResiduals returns, per variant, what its changes from base are worth beyond their stats:
// the paired sim delta minus what the response curves make of the gear stat change. The curves
// were measured around seed.
func MeasureResiduals(ctx context.Context, eval Evaluator, obj *Objective, resp Response, seed, base Loadout, variants []Loadout, iterations int) ([]Estimate, error) {
	points := make([]Point, 0, len(variants)+1)
	points = append(points, Point{Loadout: base})
	for _, v := range variants {
		points = append(points, Point{Loadout: v})
	}
	evals, err := eval.Evaluate(ctx, points, iterations)
	if err != nil {
		return nil, fmt.Errorf("residual sims: %w", err)
	}

	seedStats := gearStats(seed)
	baseValue := resp.Value(gearStats(base).Subtract(seedStats))
	out := make([]Estimate, len(variants))
	for i, v := range variants {
		simmed := obj.Delta(evals[0], evals[i+1])
		predicted := resp.Value(gearStats(v).Subtract(seedStats)) - baseValue
		out[i] = Estimate{Mean: simmed.Mean - predicted, SE: simmed.SE}
	}
	return out, nil
}
