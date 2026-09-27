package optimizer

import (
	"context"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// titanguardID is a one-handed sword, so Human weapon specialization applies to it and Orc's axes
// and fists don't.
const titanguardID = 45110

func protWarrior(tb testing.TB) *proto.Player {
	tb.Helper()
	gear := core.GetGearSet("../../ui/protection_warrior/gear_sets", "p1_balanced").GearSet
	gear.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: titanguardID}
	return &proto.Player{
		Name:          "Prot",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		Equipment:     gear,
		Rotation:      core.GetAplRotation("../../ui/protection_warrior/apls", "default").Rotation,
		TalentsString: "2500030023-302-053351225000012521030113321",
		Glyphs: &proto.Glyphs{
			Major1: int32(proto.WarriorMajorGlyph_GlyphOfBlocking),
			Major2: int32(proto.WarriorMajorGlyph_GlyphOfDevastate),
			Major3: int32(proto.WarriorMajorGlyph_GlyphOfVigilance),
		},
		Spec: &proto.Player_ProtectionWarrior{ProtectionWarrior: &proto.ProtectionWarrior{Options: &proto.ProtectionWarrior_Options{
			Shout: proto.WarriorShout_WarriorShoutCommanding,
		}}},
		Consumes: &proto.Consumes{
			BattleElixir:   proto.BattleElixir_ElixirOfMastery,
			GuardianElixir: proto.GuardianElixir_GiftOfArthas,
		},
		Buffs:           core.FullIndividualBuffs,
		InFrontOfTarget: true,
		HealingModel:    &proto.HealingModel{CadenceSeconds: 2.5, BurstWindow: 6},
	}
}

// The traits, not the race, decide what the sim applies: an Orc wearing Human traits gets their
// sword and mace expertise, and loses the axe and fist expertise of their own.
func TestRacialTraitsGiveWeaponSpecialization(t *testing.T) {
	req := tankOptimizeRequest(t, protWarrior(t), 1)
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Seed.RacialTraits != proto.Race_RaceOrc {
		t.Fatalf("the seed wears %v traits, want the player's own Orc", r.Seed.RacialTraits)
	}
	expertise := func(race proto.Race) float64 {
		t.Helper()
		l := r.Seed
		l.RacialTraits = race
		sheet, err := playerSheet(r.Base, r.TargetIndex, l, stats.Stats{})
		if err != nil {
			t.Fatal(err)
		}
		return sheet.FinalStats[stats.Expertise]
	}
	// Human weapon specialization is 3 quarter-percent of expertise on swords and maces
	want := 3 * core.ExpertisePerQuarterPercentReduction
	if got := expertise(proto.Race_RaceHuman) - expertise(proto.Race_RaceOrc); math.Abs(got-want) > 0.001 {
		t.Errorf("Human traits on a sword add %.2f expertise rating, want %.2f", got, want)
	}
}

// The screen scores every race, keeps the best three plus anything tied with the third, and hands
// them to the search.
func TestScreenRacialsPicksFinalists(t *testing.T) {
	// a fake sim where each race's traits are worth a fixed amount of DPS
	value := map[proto.Race]float64{
		proto.Race_RaceOrc:      1000,
		proto.Race_RaceTroll:    900,
		proto.Race_RaceHuman:    800,
		proto.Race_RaceDraenei:  800,
		proto.Race_RaceBloodElf: 400,
	}
	eval := newFakeEvaluator(func(p Point) Metrics {
		var m Metrics
		m[MetricDPS] = value[p.Loadout.RacialTraits] + p.Offset[stats.AttackPower]
		return m
	})
	eval.shared, eval.own = 0, 0.01

	r := &run{
		ctx:    context.Background(),
		r:      presetRequest(t, "fury_p1"),
		budget: EffortBudget(proto.OptimizerEffort_OptimizerEffortQuick),
		screen: 100,
	}
	r.asked = r.r
	r.eval = &countingEvaluator{inner: eval, have: map[Point]int{}}
	obj, err := NewObjective(r.ctx, r.eval, r.r, r.budget.Iterations)
	if err != nil {
		t.Fatal(err)
	}
	r.obj = obj

	finalists, err := r.screenRacials()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.racialScreen) != len(allRaces) {
		t.Errorf("the screen reports %d races, want all %d", len(r.racialScreen), len(allRaces))
	}
	want := []proto.Race{proto.Race_RaceOrc, proto.Race_RaceTroll, proto.Race_RaceHuman, proto.Race_RaceDraenei}
	for _, race := range want {
		if !slices.Contains(finalists, race) {
			t.Errorf("%v isn't a finalist, but it should be (screen: %v)", race, finalists)
		}
	}
	if slices.Contains(finalists, proto.Race_RaceBloodElf) {
		t.Errorf("Blood Elf made the finals with 400 DPS against the leader's 1000 (screen: %v)", finalists)
	}
	for _, screened := range r.racialScreen {
		if screened.Finalist != slices.Contains(finalists, screened.RacialTraits) {
			t.Errorf("%v is flagged finalist=%v but the finalists are %v", screened.RacialTraits, screened.Finalist, finalists)
		}
	}
}

// Every gear set the search found comes back once per finalist, with its variants together.
func TestWithRacialTraitsPairsEverySet(t *testing.T) {
	var a, b Loadout
	a.Items[0].ItemID, b.Items[0].ItemID = 1, 2
	races := []proto.Race{proto.Race_RaceOrc, proto.Race_RaceHuman}
	got := withRacialTraits([]Loadout{a, b}, races)
	want := []Loadout{
		{Items: a.Items, RacialTraits: proto.Race_RaceOrc},
		{Items: a.Items, RacialTraits: proto.Race_RaceHuman},
		{Items: b.Items, RacialTraits: proto.Race_RaceOrc},
		{Items: b.Items, RacialTraits: proto.Race_RaceHuman},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if again := withRacialTraits([]Loadout{a}, nil); !slices.Equal(again, []Loadout{a}) {
		t.Errorf("without finalists the candidates should pass through, got %v", again)
	}
}

// End to end over the known-answer pool: the screen, the finalists and verification together land
// on the race the sim likes best, and the result reports what every race scored.
func TestOptimizeSearchesRacialTraits(t *testing.T) {
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	req.Settings.RacialMode = proto.OptimizerRacialMode_OptimizerRacialSearch
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = proto.Race_RaceTauren
	fake := newKnownEvaluator(func(p Point) (Metrics, error) {
		m, err := kaMetrics(p)
		if err == nil && p.Loadout.RacialTraits == want {
			m[MetricDPS] += 400
		}
		return m, err
	})

	result := optimize(context.Background(), r, r, fake, nil, time.Now())
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	if result.Best.RacialTraits != want {
		t.Errorf("the pick wears %v traits, want %v (screen: %v)", result.Best.RacialTraits, want, result.RacialScreen)
	}
	if len(result.RacialScreen) != len(allRaces) {
		t.Errorf("the screen reports %d races, want all %d", len(result.RacialScreen), len(allRaces))
	}
	finalists := 0
	for _, screened := range result.RacialScreen {
		if screened.Finalist {
			finalists++
		}
	}
	if finalists < racialFinalists || finalists > maxRacialFinalists {
		t.Errorf("%d finalists, want between %d and %d", finalists, racialFinalists, maxRacialFinalists)
	}
	if result.RacialScreen[0].RacialTraits != want || !result.RacialScreen[0].Finalist {
		t.Errorf("the screen's best is %v, want %v first and a finalist", result.RacialScreen[0], want)
	}
	if !result.Improved || result.Best.ScoreDelta < 300 {
		t.Errorf("improved = %v with a %.1f gain; the traits alone are worth 400 DPS", result.Improved, result.Best.ScoreDelta)
	}
	t.Logf("pick %v, %d sims; screen %v", result.Best.RacialTraits, result.TotalSims, result.RacialScreen)
}

// compareRacesRequest is kaRequest's Orc warrior asking for a racial comparison, wearing traits.
func compareRacesRequest(t *testing.T, traits proto.Race) *Request {
	t.Helper()
	req := kaRequest(proto.OptimizerEffort_OptimizerEffortQuick)
	req.Settings.CompareRacialTraits = true
	req.Base.Raid.Parties[0].Players[0].RacialTraits = traits
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.Seed.RacialTraits != traits {
		t.Fatalf("seed traits = %v, want %v", r.Seed.RacialTraits, traits)
	}
	return r
}

// A fake sim where each race's traits add a fixed amount of DPS to kaMetrics, recording which races
// it was asked about.
func racialValueEvaluator(value map[proto.Race]float64, simmed map[proto.Race]bool) *knownEvaluator {
	return newKnownEvaluator(func(p Point) (Metrics, error) {
		simmed[p.Loadout.RacialTraits] = true
		m, err := kaMetrics(p)
		if err == nil {
			m[MetricDPS] += value[p.Loadout.RacialTraits]
		}
		return m, err
	})
}

// A Draenei whose party already has one weighs every other race and moves to the one that gains
// the raid most.
func TestCompareRacesMovesADraeneiToTheRaceThatGainsMost(t *testing.T) {
	r := compareRacesRequest(t, proto.Race_RaceDraenei)
	simmed := map[proto.Race]bool{}
	fake := racialValueEvaluator(map[proto.Race]float64{proto.Race_RaceTauren: 400, proto.Race_RaceOrc: 200}, simmed)

	result := compareRaces(context.Background(), r, fake, time.Now())
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	if !result.Improved || result.Best.RacialTraits != proto.Race_RaceTauren {
		t.Errorf("best = %v, improved %v, want Tauren", result.Best.RacialTraits, result.Improved)
	}
	if result.Best.RaidDpsDelta < 300 {
		t.Errorf("raid_dps_delta = %.1f, want close to the 400 DPS the traits are worth", result.Best.RaidDpsDelta)
	}
	if !goproto.Equal(result.Best.Equipment, result.Seed.Equipment) {
		t.Error("compareRaces changed the gear; it should only ever change racial traits")
	}
	if len(simmed) != len(allRaces) || len(result.RacialScreen) != len(allRaces) {
		t.Errorf("simmed %d races and reported %d, want all %d", len(simmed), len(result.RacialScreen), len(allRaces))
	}
}

// Anyone else only weighs Draenei, for their party's Heroic Presence: a race that would be worth
// more on its own was stage 1's call, so it never gets simmed here.
func TestCompareRacesWeighsEveryoneElseAgainstDraeneiOnly(t *testing.T) {
	r := compareRacesRequest(t, proto.Race_RaceOrc)
	simmed := map[proto.Race]bool{}
	fake := racialValueEvaluator(map[proto.Race]float64{proto.Race_RaceTauren: 400, proto.Race_RaceDraenei: 200}, simmed)

	result := compareRaces(context.Background(), r, fake, time.Now())
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	if !result.Improved || result.Best.RacialTraits != proto.Race_RaceDraenei {
		t.Errorf("best = %v, improved %v, want Draenei", result.Best.RacialTraits, result.Improved)
	}
	if result.Best.RaidDpsDelta < 150 {
		t.Errorf("raid_dps_delta = %.1f, want close to the 200 DPS Draenei is worth", result.Best.RaidDpsDelta)
	}
	if len(simmed) != 2 || !simmed[proto.Race_RaceOrc] || !simmed[proto.Race_RaceDraenei] {
		t.Errorf("simmed %v, want just Orc and Draenei", simmed)
	}
	if len(result.RacialScreen) != 2 || result.RacialScreen[0].RacialTraits != proto.Race_RaceOrc {
		t.Errorf("screen = %v, want Orc then Draenei", result.RacialScreen)
	}
}

// Every race scores the seed gear the same (kaMetrics never reads racial traits), so nothing clears
// the noise and the current race stays, Draenei or not.
func TestCompareRacesKeepsTheCurrentRaceWhenNothingClearsTheNoise(t *testing.T) {
	for _, race := range []proto.Race{proto.Race_RaceOrc, proto.Race_RaceDraenei} {
		r := compareRacesRequest(t, race)
		result := compareRaces(context.Background(), r, newKnownEvaluator(kaMetrics), time.Now())
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		if result.Improved || result.Best.RacialTraits != race {
			t.Errorf("%v: improved = %v, best = %v, want %v kept", race, result.Improved, result.Best.RacialTraits, race)
		}
	}
}

// compareRacesInParty compares the first of players' racial traits in real raid sims, all of them in
// one party.
func compareRacesInParty(t *testing.T, players ...*proto.Player) *proto.OptimizerResult {
	t.Helper()
	req := raidContribRequest(t, 0, players...)
	req.Settings.CompareRacialTraits = true
	r, err := PrepareRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	result := compareRaces(context.Background(), r, NewRaidEvaluator(r, MetricDPS), time.Now())
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	t.Logf("best %v, raid DPS %+.1f ± %.1f, screen %v", result.Best.RacialTraits, result.Best.RaidDpsDelta, result.Best.RaidDpsDeltaSe, result.RacialScreen)
	return result
}

// withHit is a fury_p1 raider wearing traits, its hit rating moved by bonus.
func withHit(t *testing.T, name string, traits proto.Race, bonus float64) *proto.Player {
	t.Helper()
	player := presetPlayer(t, "fury_p1", name)
	player.RacialTraits = traits
	player.BonusStats = &proto.UnitStats{Stats: stats.Stats{stats.MeleeHit: bonus}.ToFloatArray()}
	return player
}

// A party without a Draenei, whose other member misses a lot: switching the target to Draenei gives
// the party Heroic Presence, which gains the raid more than Night Elf traits (worth nothing to a fury
// warrior) do.
func TestCompareRacesGivesAPartyItsDraenei(t *testing.T) {
	result := compareRacesInParty(t, withHit(t, "Target", proto.Race_RaceNightElf, 0), withHit(t, "Needs Hit", proto.Race_RaceNightElf, -300))
	if !result.Improved || result.Best.RacialTraits != proto.Race_RaceDraenei {
		t.Errorf("best = %v, improved %v, want Draenei", result.Best.RacialTraits, result.Improved)
	}
	if d, se := result.Best.RaidDpsDelta, result.Best.RaidDpsDeltaSe; d <= 2*se {
		t.Errorf("raid DPS %+.1f ± %.1f, want a gain past 2 SE", d, se)
	}
}

// The same party, but both members are far past every hit cap, so Heroic Presence gains nothing and
// the party is left as it is.
func TestCompareRacesLeavesAPartyAloneWhenNothingClearsTheBar(t *testing.T) {
	result := compareRacesInParty(t, withHit(t, "Target", proto.Race_RaceNightElf, 1500), withHit(t, "Capped", proto.Race_RaceNightElf, 1500))
	if result.Improved || result.Best.RacialTraits != proto.Race_RaceNightElf {
		t.Errorf("best = %v, improved %v, want Night Elf kept", result.Best.RacialTraits, result.Improved)
	}
}
