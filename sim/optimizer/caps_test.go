package optimizer

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func TestDpsCapRole(t *testing.T) {
	for _, tc := range []struct {
		name   string
		player *proto.Player
		want   capRole
	}{
		{"warrior", &proto.Player{Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{}}}, capRoleMelee},
		{"feral druid", &proto.Player{Spec: &proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{}}}, capRoleMelee},
		{"rogue", &proto.Player{Spec: &proto.Player_Rogue{Rogue: &proto.Rogue{}}}, capRoleMelee},
		{"retribution paladin", &proto.Player{Spec: &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{}}}, capRoleMelee},
		{"enhancement shaman", &proto.Player{Spec: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{}}}, capRoleMelee},
		{"death knight", &proto.Player{Spec: &proto.Player_Deathknight{Deathknight: &proto.Deathknight{}}}, capRoleMelee},
		{"hunter", &proto.Player{Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{}}}, capRoleHunter},
		{"balance druid", &proto.Player{Spec: &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{}}}, capRoleCaster},
		{"mage", &proto.Player{Spec: &proto.Player_Mage{Mage: &proto.Mage{}}}, capRoleCaster},
		{"shadow priest", &proto.Player{Spec: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{}}}, capRoleCaster},
		{"smite priest", &proto.Player{Spec: &proto.Player_SmitePriest{SmitePriest: &proto.SmitePriest{}}}, capRoleCaster},
		{"elemental shaman", &proto.Player{Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{}}}, capRoleCaster},
		{"warlock", &proto.Player{Spec: &proto.Player_Warlock{Warlock: &proto.Warlock{}}}, capRoleCaster},
		{"protection warrior", &proto.Player{Spec: &proto.Player_ProtectionWarrior{ProtectionWarrior: &proto.ProtectionWarrior{}}}, capRoleNone},
		{"protection paladin", &proto.Player{Spec: &proto.Player_ProtectionPaladin{ProtectionPaladin: &proto.ProtectionPaladin{}}}, capRoleNone},
		{"feral tank druid", &proto.Player{Spec: &proto.Player_FeralTankDruid{FeralTankDruid: &proto.FeralTankDruid{}}}, capRoleNone},
		{"tank death knight", &proto.Player{Spec: &proto.Player_TankDeathknight{TankDeathknight: &proto.TankDeathknight{}}}, capRoleNone},
		{"holy paladin", &proto.Player{Spec: &proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{}}}, capRoleNone},
		{"no spec", &proto.Player{}, capRoleNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dpsCapRole(tc.player); got != tc.want {
				t.Errorf("role %v, want %v", got, tc.want)
			}
		})
	}
}

// checkCapExact bisects st's cap and checks it's exactly where ok flips: satisfied there, not one
// rating under, the way TestRequiredDefenseIsTheThreshold checks D*.
func checkCapExact(t *testing.T, r *Request, st stats.Stat, ok func(core.PlayerSheet) bool, read func(core.PlayerSheet) float64) float64 {
	t.Helper()
	got, err := bisectCapBonus(r, st, ok)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := playerSheet(r.Base, r.TargetIndex, r.Seed, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	at := func(bonus float64) core.PlayerSheet {
		t.Helper()
		var offset stats.Stats
		offset[st] = bonus
		sheet, err := playerSheet(r.Base, r.TargetIndex, r.Seed, offset)
		if err != nil {
			t.Fatal(err)
		}
		return sheet
	}
	// bonus stats move the sheet's rating one for one, so the cap sits this far from the seed's
	bonus := got - seed.FinalStats[st]
	if !ok(at(bonus)) {
		t.Errorf("%s cap is %.0f, but the sheet there still reads %.4f", st.StatName(), got, read(at(bonus)))
	}
	if ok(at(bonus - 1)) {
		t.Errorf("%s cap is %.0f, but one rating less already satisfies it", st.StatName(), got)
	}
	t.Logf("%s: seed %.0f, cap %.0f (%+.0f from the seed), reading %.4f at the cap", st.StatName(), seed.FinalStats[st], got, bonus, read(at(bonus)))
	return got
}

func meleeMissOK(s core.PlayerSheet) bool { return s.PhysicalMissChance == 0 }
func expertiseOK(s core.PlayerSheet) bool { return s.ExpertiseDodgeChance == 0 }
func spellMissOK(s core.PlayerSheet) bool { return s.SpellMissChance == 0 }
func casterHasteOK(s core.PlayerSheet) bool {
	return globalCooldown(s.CastSpeed) <= hasteCeilingFloor
}
func hunterHasteOK(s core.PlayerSheet) bool {
	return steadyShotCastTime(s.RangedSwingSpeed) <= hasteCeilingFloor
}

// A dual-wielding melee DPS's hit floor is the special-attack miss cap, and its expertise floor is
// the dodge cap: both exact, one rating under and the boss can still land the attack or dodge it.
func TestMeleeCapsAreExact(t *testing.T) {
	r := presetRequest(t, "fury_p1")
	if role := dpsCapRole(r.Target()); role != capRoleMelee {
		t.Fatalf("fury warrior classified as %v, want melee", role)
	}
	checkCapExact(t, r, stats.MeleeHit, meleeMissOK, func(s core.PlayerSheet) float64 { return s.PhysicalMissChance })
	checkCapExact(t, r, stats.Expertise, expertiseOK, func(s core.PlayerSheet) float64 { return s.ExpertiseDodgeChance })
}

// A caster's floor is the spell miss cap, exact against the run's target.
func TestCasterSpellHitCapIsExact(t *testing.T) {
	r := presetRequest(t, "arcane_p3")
	if role := dpsCapRole(r.Target()); role != capRoleCaster {
		t.Fatalf("arcane mage classified as %v, want caster", role)
	}
	checkCapExact(t, r, stats.SpellHit, spellMissOK, func(s core.PlayerSheet) float64 { return s.SpellMissChance })
}

// A caster's haste ceiling is where a hasted GCD reaches its 1 s floor.
func TestCasterHasteCeilingIsExact(t *testing.T) {
	r := presetRequest(t, "arcane_p3")
	checkCapExact(t, r, stats.SpellHaste, casterHasteOK, func(s core.PlayerSheet) float64 { return globalCooldown(s.CastSpeed).Seconds() })
}

// A hunter's floor is the same physical miss cap melee reads (WotLK has one hit rating for both), and
// its ceiling is where Steady Shot's own cast time, not a hasted GCD, reaches 1 s.
func TestHunterCapsAreExact(t *testing.T) {
	r, err := PrepareRequest(hunterRequest(t, mmHunter()))
	if err != nil {
		t.Fatal(err)
	}
	if role := dpsCapRole(r.Target()); role != capRoleHunter {
		t.Fatalf("hunter classified as %v, want hunter", role)
	}
	checkCapExact(t, r, stats.MeleeHit, meleeMissOK, func(s core.PlayerSheet) float64 { return s.PhysicalMissChance })
	checkCapExact(t, r, stats.MeleeHaste, hunterHasteOK, func(s core.PlayerSheet) float64 { return steadyShotCastTime(s.RangedSwingSpeed).Seconds() })
}

// The floor follows the run's debuffs: Improved Faerie Fire and Misery both add flat spell hit
// against the target (AttackTable.Defender.PseudoStats.BonusSpellHitRatingTaken), on the opposite
// side of the miss formula from the caster's own rating, so dropping them raises how much sheet
// spell hit the caster needs by about what 3% hit costs in rating. A player-side rating buff such as
// Heroic Presence can't show the same thing: it adds straight to the stat the floor bisects, so it
// moves the seed's own rating, never the sheet total the miss chance zeroes out at (a fixed point of
// the miss formula). The two debuffs share an exclusive effect and don't stack, in the sim or the
// game, so dropping only one leaves the other's 3% standing.
func TestTheFloorFollowsTargetDebuffs(t *testing.T) {
	with := presetRequest(t, "arcane_p3")
	withFloor, err := bisectCapBonus(with, stats.SpellHit, spellMissOK)
	if err != nil {
		t.Fatal(err)
	}

	without := presetRequest(t, "arcane_p3")
	without.Base.Raid.Debuffs.FaerieFire = proto.TristateEffect_TristateEffectMissing
	without.Base.Raid.Debuffs.Misery = false
	withoutFloor, err := bisectCapBonus(without, stats.SpellHit, spellMissOK)
	if err != nil {
		t.Fatal(err)
	}

	if withFloor >= withoutFloor {
		t.Errorf("with the spell hit debuffs the floor is %.0f, without them %.0f; want dropping them to raise it", withFloor, withoutFloor)
	}
	want := 3 * core.SpellHitRatingPerHitChance
	if got := withoutFloor - withFloor; math.Abs(got-want) > 1 {
		t.Errorf("dropping the spell hit debuffs raises the floor by %.1f, want about %.1f (their flat 3%% hit)", got, want)
	}
	t.Logf("spell hit floor %.0f with the debuffs, %.0f without them", withFloor, withoutFloor)
}

// Tanks get none of the DPS cap floors or the haste ceiling.
func TestTankGetsNoCapFloors(t *testing.T) {
	r, err := PrepareRequest(tankOptimizeRequest(t, protPaladin(t, "p3"), 3))
	if err != nil {
		t.Fatal(err)
	}
	if role := dpsCapRole(r.Target()); role != capRoleNone {
		t.Fatalf("prot paladin classified as %v, want none", role)
	}
	rn := &run{asked: r, r: r}
	rn.dpsCapFloors()
	if len(r.Settings.GetStatMinimums()) != 0 || len(r.Settings.GetStatMaximums()) != 0 {
		t.Errorf("a tank got floors %v and ceilings %v, want none", r.Settings.StatMinimums, r.Settings.StatMaximums)
	}
}

// With the caps off, dpsCapFloors adds nothing, so an unticked run picks exactly what it did before
// this feature existed.
func TestDisableStatCapsIsANoOp(t *testing.T) {
	r := presetRequest(t, "fury_p1")
	r.Settings.DisableStatCaps = true
	rn := &run{asked: r, r: r}
	rn.dpsCapFloors()
	if len(r.Settings.GetStatMinimums()) != 0 || len(r.Settings.GetStatMaximums()) != 0 {
		t.Errorf("caps disabled but floors %v and ceilings %v were added anyway", r.Settings.StatMinimums, r.Settings.StatMaximums)
	}
}

// The surrogate's ceiling penalty is floorShortfall's mirror: 0 under the ceiling, and over it a pull
// back down worth the same weight a floor pulls up with.
func TestCeilingPenalizesOverage(t *testing.T) {
	grad := stats.Stats{stats.SpellHaste: 1}
	s := &surrogate{
		floorWeight: 10,
		ceilings:    []ceilingTerm{{stat: stats.SpellHaste, max: 100, seed: 80, gradient: grad, sparse: sparseGradient(grad)}},
	}
	under := stats.Stats{stats.SpellHaste: 10} // have = 90
	over := stats.Stats{stats.SpellHaste: 30}  // have = 110, 10 over

	if o := s.ceilingOverage(under); o != 0 {
		t.Errorf("under the ceiling: overage %.1f, want 0", o)
	}
	if o := s.ceilingOverage(over); o != 10 {
		t.Errorf("10 over the ceiling: overage %.1f, want 10", o)
	}
	if v := s.slopes(under)[stats.SpellHaste]; v != 0 {
		t.Errorf("under the ceiling: slope pull %.1f, want 0", v)
	}
	if v := s.slopes(over)[stats.SpellHaste]; v != -s.floorWeight {
		t.Errorf("over the ceiling: slope pull %.1f, want %.1f", v, -s.floorWeight)
	}
	if underScore, overScore := s.score(under, 0, nil), s.score(over, 0, nil); overScore >= underScore {
		t.Errorf("20 more SpellHaste while going 10 over the ceiling scores %.1f, want it worse than %.1f", overScore, underScore)
	}
}

// realCapRequest is name's preset over a realistic pool, Quick effort, current racial traits kept.
func realCapRequest(t *testing.T, name string, phase int32, caps bool) *proto.OptimizeGearRequest {
	t.Helper()
	req := presetOptimizeRequest(t, name)
	player := req.Base.Raid.Parties[0].Players[0]
	req.Settings = &proto.OptimizerSettings{
		ContentPhase:    phase,
		Effort:          proto.OptimizerEffort_OptimizerEffortQuick,
		RacialMode:      proto.OptimizerRacialMode_OptimizerRacialKeepCurrent,
		DisableStatCaps: !caps,
	}
	req.Pool = realisticPool(t, player, phase, nil)
	return req
}

// hunterCapRequest is mmHunter over a realistic pool at phase, Quick effort, current racial traits
// kept, the way realCapRequest is for a preset.
func hunterCapRequest(t *testing.T, phase int32, caps bool) *proto.OptimizeGearRequest {
	t.Helper()
	player := mmHunter()
	req := presetOptimizeRequest(t, "fury_p1")
	req.Base.Raid = core.SinglePlayerRaidProto(player, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs)
	req.Settings = &proto.OptimizerSettings{
		ContentPhase:    phase,
		Effort:          proto.OptimizerEffort_OptimizerEffortQuick,
		RacialMode:      proto.OptimizerRacialMode_OptimizerRacialKeepCurrent,
		DisableStatCaps: !caps,
	}
	req.Pool = realisticPool(t, player, phase, nil)
	return req
}

// optimizedSheet runs req and reads the pick's sheet.
func optimizedSheet(t *testing.T, req *proto.OptimizeGearRequest) (stats.Stats, *proto.OptimizerResult) {
	t.Helper()
	result := Optimize(context.Background(), req, nil)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	best, err := LoadoutFromProto(result.Best.GetEquipment(), result.Best.GetRacialTraits())
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := playerSheet(r.Base, r.TargetIndex, best, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	return sheet.FinalStats, result
}

func floorOf(settings *proto.OptimizerSettings, st stats.Stat) float64 {
	for _, f := range settings.GetStatMinimums() {
		if stats.Stat(f.Stat) == st {
			return f.MinValue
		}
	}
	return 0
}

func ceilingOf(settings *proto.OptimizerSettings, st stats.Stat) float64 {
	for _, c := range settings.GetStatMaximums() {
		if stats.Stat(c.Stat) == st {
			return c.MaxValue
		}
	}
	return 0
}

// A melee DPS's search reaches both the hit and expertise floors.
func TestMeleeCapFloorsReachTheCap(t *testing.T) {
	without, _ := optimizedSheet(t, realCapRequest(t, "fury_p1", 1, false))
	with, result := optimizedSheet(t, realCapRequest(t, "fury_p1", 1, true))

	hitFloor, expFloor := floorOf(result.Settings, stats.MeleeHit), floorOf(result.Settings, stats.Expertise)
	if hitFloor == 0 || expFloor == 0 {
		t.Fatalf("no hit/expertise floor in the result's settings: %v", result.Settings.GetStatMinimums())
	}
	if without[stats.MeleeHit] >= hitFloor && without[stats.Expertise] >= expFloor {
		t.Fatalf("unconstrained hit is %.1f (floor %.1f) and expertise %.1f (floor %.1f), both already clear; want at least one under its floor for this test to mean anything",
			without[stats.MeleeHit], hitFloor, without[stats.Expertise], expFloor)
	}
	if with[stats.MeleeHit] < hitFloor-0.5 {
		t.Errorf("with the caps on, hit is %.1f, under its floor of %.1f", with[stats.MeleeHit], hitFloor)
	}
	if with[stats.Expertise] < expFloor-0.5 {
		t.Errorf("with the caps on, expertise is %.1f, under its floor of %.1f", with[stats.Expertise], expFloor)
	}
	t.Logf("fury p1: caps off hit=%.0f exp=%.0f; caps on hit=%.0f (floor %.0f) exp=%.0f (floor %.0f)",
		without[stats.MeleeHit], without[stats.Expertise], with[stats.MeleeHit], hitFloor, with[stats.Expertise], expFloor)
}

// capShortfallAgainst is how far l's sheet sits under settings' floors (0 if it meets every one),
// with a detail string. Settings comes from a finished result: the independent check's own
// PrepareRequest never runs dpsCapFloors, so its Pool carries no floors to check l against.
func capShortfallAgainst(t *testing.T, r *Request, settings *proto.OptimizerSettings, l Loadout) (float64, string) {
	t.Helper()
	sheet, err := playerSheet(r.Base, r.TargetIndex, l, stats.Stats{})
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	var parts []string
	for _, f := range settings.GetStatMinimums() {
		st := stats.Stat(f.Stat)
		if short := f.MinValue - sheet.FinalStats[st]; short > 0 {
			total += short
			parts = append(parts, fmt.Sprintf("%s %.0f short", st.StatName(), short))
		}
	}
	return total, strings.Join(parts, ", ")
}

// capMargins reports settings' floors and ceilings against sheet's actual readings, for the slow
// suite's per-case log line; "" when the run added none.
func capMargins(settings *proto.OptimizerSettings, sheet core.PlayerSheet) string {
	var parts []string
	for _, f := range settings.GetStatMinimums() {
		st := stats.Stat(f.Stat)
		parts = append(parts, fmt.Sprintf("%s=%.0f/floor=%.0f", st.StatName(), sheet.FinalStats[st], f.MinValue))
	}
	for _, c := range settings.GetStatMaximums() {
		st := stats.Stat(c.Stat)
		parts = append(parts, fmt.Sprintf("%s=%.0f/ceiling=%.0f", st.StatName(), sheet.FinalStats[st], c.MaxValue))
	}
	return strings.Join(parts, " ")
}

// A caster's search reaches the spell hit floor.
func TestCasterCapFloorReachesTheCap(t *testing.T) {
	without, _ := optimizedSheet(t, realCapRequest(t, "arcane_p3", 3, false))
	with, result := optimizedSheet(t, realCapRequest(t, "arcane_p3", 3, true))
	floor := floorOf(result.Settings, stats.SpellHit)
	if floor == 0 {
		t.Fatalf("no spell hit floor in the result's settings: %v", result.Settings.GetStatMinimums())
	}
	if without[stats.SpellHit] >= floor {
		t.Fatalf("unconstrained spell hit is %.1f, want it under the floor of %.1f for this test to mean anything", without[stats.SpellHit], floor)
	}
	if got := with[stats.SpellHit]; got < floor-0.5 {
		t.Errorf("spell hit is %.1f, under its floor of %.1f", got, floor)
	}
	t.Logf("arcane p3: spell hit %.1f unconstrained, %.1f with the caps on, floor %.1f", without[stats.SpellHit], with[stats.SpellHit], floor)
}

// A caster's unconstrained search, over a late enough pool, stacks SpellHaste past the ceiling; with
// the caps on it stops there instead.
func TestCasterHasteCeilingStopsTheSearch(t *testing.T) {
	without, _ := optimizedSheet(t, realCapRequest(t, "arcane_p3", 4, false))
	with, result := optimizedSheet(t, realCapRequest(t, "arcane_p3", 4, true))

	ceiling := ceilingOf(result.Settings, stats.SpellHaste)
	if ceiling == 0 {
		t.Fatalf("no haste ceiling in the result's settings: %v", result.Settings.GetStatMaximums())
	}
	if without[stats.SpellHaste] <= ceiling {
		t.Fatalf("unconstrained SpellHaste is %.1f, want it over the ceiling of %.1f for this test to mean anything", without[stats.SpellHaste], ceiling)
	}
	if got := with[stats.SpellHaste]; got > ceiling+0.5 {
		t.Errorf("with the caps on, SpellHaste is %.1f, over its ceiling of %.1f", got, ceiling)
	}
	t.Logf("arcane p4: SpellHaste %.1f unconstrained, %.1f with the caps on, ceiling %.1f", without[stats.SpellHaste], with[stats.SpellHaste], ceiling)
}

// A hunter's haste ceiling sits far above anything a realistic pool can reach: doubling ranged attack
// speed (Steady Shot's base 2 s cast down to 1 s) takes about 100% ranged haste, and DPS-optimal MM
// gearing doesn't stack anywhere close even unconstrained (phase 5, no caps: 199 MeleeHaste against a
// ~2258 ceiling), unlike a caster's GCD-based ceiling. TestCasterHasteCeilingStopsTheSearch's "stops
// the search" scenario has no hunter equivalent for that reason; TestHunterCapsAreExact already covers
// the ceiling's own bisection for a hunter, and TestCeilingPenalizesOverage covers the surrogate term
// spec-agnostically.
func TestHunterHasteCeilingNeverBindsARealisticPool(t *testing.T) {
	without, _ := optimizedSheet(t, hunterCapRequest(t, 5, false))
	with, result := optimizedSheet(t, hunterCapRequest(t, 5, true))

	ceiling := ceilingOf(result.Settings, stats.MeleeHaste)
	if ceiling == 0 {
		t.Fatalf("no haste ceiling in the result's settings: %v", result.Settings.GetStatMaximums())
	}
	if got := with[stats.MeleeHaste]; got > ceiling+0.5 {
		t.Errorf("with the caps on, MeleeHaste is %.1f, over its ceiling of %.1f", got, ceiling)
	}
	t.Logf("hunter p5: MeleeHaste %.1f unconstrained, %.1f with the caps on, ceiling %.1f", without[stats.MeleeHaste], with[stats.MeleeHaste], ceiling)
}

// With a hit floor the seed already meets, the effects budget ranks every other item by a finite
// prior, and a tight budget still keeps the seed's own items priced so the search can rebuild it.
func TestTrimToBudgetKeepsTheSeedPricedUnderAFloor(t *testing.T) {
	r, err := PrepareRequest(realCapRequest(t, "fury_p1", 1, true))
	if err != nil {
		t.Fatal(err)
	}
	pool, err := CompilePool(r)
	if err != nil {
		t.Fatal(err)
	}
	resp := Response{
		stats.Strength:         {Stat: stats.Strength, SlopeBelow: 2.5, SlopeAbove: 2.5},
		stats.Agility:          {Stat: stats.Agility, SlopeBelow: 1.5, SlopeAbove: 1.5},
		stats.AttackPower:      {Stat: stats.AttackPower, SlopeBelow: 1, SlopeAbove: 1},
		stats.MeleeHit:         {Stat: stats.MeleeHit, SlopeBelow: 3.5, SlopeAbove: 0},
		stats.MeleeCrit:        {Stat: stats.MeleeCrit, SlopeBelow: 2, SlopeAbove: 2},
		stats.MeleeHaste:       {Stat: stats.MeleeHaste, SlopeBelow: 1.5, SlopeAbove: 1.5},
		stats.Expertise:        {Stat: stats.Expertise, SlopeBelow: 3, SlopeAbove: 3},
		stats.ArmorPenetration: {Stat: stats.ArmorPenetration, SlopeBelow: 2, SlopeAbove: 2},
	}
	s := newSurrogate(pool, r.Seed, resp)
	seedSheet, err := pool.finalStats(r.Seed)
	if err != nil {
		t.Fatal(err)
	}
	floor := []*proto.StatMinimum{{Stat: proto.Stat_StatMeleeHit, MinValue: seedSheet[stats.MeleeHit]}}
	if err := s.setFloors(floor, statsOf(resp)); err != nil {
		t.Fatal(err)
	}

	rn := &run{asked: r, r: r, pool: pool, eval: &countingEvaluator{have: map[Point]int{}}, total: math.MaxInt64 / 2}
	v := s.slopes(s.seedStats)
	fx := &effects{raw: map[effectKey]Estimate{}, missing: map[effectKey]bool{}, pairBase: map[proto.ItemSlot]Loadout{}}
	families := rn.effectFamilies(s, v, s.newGemPlan(v), fx)

	seedItems := map[int32]bool{}
	for _, c := range r.Seed.Items {
		seedItems[c.ItemID] = true
	}
	const iterations = 1000
	cost := 0.0
	for _, f := range families {
		for i, k := range f.keys {
			if k.kind == effectItem && !seedItems[k.id] && math.IsInf(f.prior[i], 0) {
				t.Errorf("%s: item %d isn't the seed's, but its prior is %v", f.name, k.id, f.prior[i])
			}
		}
		cost += float64(rn.eval.cost(pool.familyPoints(f.base, f.variants, f.matched), iterations))
	}

	kept := rn.trimToBudget(families, fx, iterations, 0.01*cost)
	for slot, c := range r.Seed.Items {
		if !itemNeedsSim(c.ItemID) || pool.Locked[slot] {
			continue
		}
		k := effectKey{effectItem, familySlot(proto.ItemSlot(slot)), c.ItemID}
		_, measured := fx.raw[k]
		simmed := slices.ContainsFunc(kept, func(f *effectFamily) bool { return slices.Contains(f.keys, k) })
		if fx.missing[k] || !measured && !simmed {
			t.Errorf("the seed's %s, item %d, was cut from the effects budget", proto.ItemSlot(slot), c.ItemID)
		}
	}
}
