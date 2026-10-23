package statslopes

import (
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/statcaps"
	googleProto "google.golang.org/protobuf/proto"
)

func init() {
	sim.RegisterAll()
}

func furyPlayer() *proto.Player {
	return &proto.Player{
		Name:          "Fury",
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		Equipment:     core.GetGearSet("../../ui/warrior/gear_sets", "p1_fury").GearSet,
		Rotation:      core.GetAplRotation("../../ui/warrior/apls", "fury").Rotation,
		TalentsString: "302023102331-305052000520310053120500351",
		Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{
			StartingRage: 50,
			Shout:        proto.WarriorShout_WarriorShoutBattle,
		}}},
		Consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfEndlessRage, Food: proto.Food_FoodFishFeast},
		Buffs:    core.FullIndividualBuffs,
	}
}

// a Troll mage with no talents, short of the spell hit cap on P1 gear
func plainMage() *proto.Player {
	return &proto.Player{
		Name:      "Mage",
		Race:      proto.Race_RaceTroll,
		Class:     proto.Class_ClassMage,
		Equipment: core.GetGearSet("../../ui/mage/gear_sets", "p1_fire").GearSet,
		Rotation:  core.GetAplRotation("../../ui/mage/apls", "fire").Rotation,
		Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{
			Armor: proto.Mage_Options_MoltenArmor,
		}}},
		Buffs: &proto.IndividualBuffs{},
	}
}

func holyPriest() *proto.Player {
	return &proto.Player{
		Name:          "Holy",
		Race:          proto.Race_RaceUndead,
		Class:         proto.Class_ClassPriest,
		Equipment:     core.GetGearSet("../../ui/healing_priest/gear_sets", "p1_holy").GearSet,
		Rotation:      core.GetAplRotation("../../ui/healing_priest/apls", "holy").Rotation,
		TalentsString: "05032031103-234051032002152530004311051",
		Spec: &proto.Player_HealingPriest{HealingPriest: &proto.HealingPriest{Options: &proto.HealingPriest_Options{
			UseInnerFire: true,
		}}},
		Buffs: core.FullIndividualBuffs,
	}
}

var castsNothing = core.APLRotationFromJsonString(`{
	"type": "TypeAPL",
	"priorityList": [
		{"action":{"autocastOtherCooldowns":{}}}
	]
}`)

func holyPaladin() *proto.Player {
	return &proto.Player{
		Name:          "HolyPaladin",
		Race:          proto.Race_RaceHuman,
		Class:         proto.Class_ClassPaladin,
		Equipment:     core.GetGearSet("../../ui/holy_paladin/gear_sets", "p1").GearSet,
		Rotation:      castsNothing,
		TalentsString: "50350151020013053100515221-50023131203",
		Spec: &proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{Options: &proto.HolyPaladin_Options{
			Judgement: proto.PaladinJudgement_JudgementOfWisdom,
			Aura:      proto.PaladinAura_DevotionAura,
		}}},
		Buffs: core.FullIndividualBuffs,
	}
}

func restorationDruid() *proto.Player {
	return &proto.Player{
		Name:          "Resto",
		Race:          proto.Race_RaceTauren,
		Class:         proto.Class_ClassDruid,
		Equipment:     core.GetGearSet("../../ui/restoration_druid/gear_sets", "p1").GearSet,
		Rotation:      castsNothing,
		TalentsString: "05320031103--230023312131502331050313051",
		Spec: &proto.Player_RestorationDruid{RestorationDruid: &proto.RestorationDruid{Options: &proto.RestorationDruid_Options{
			InnervateTarget: &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0},
		}}},
		Buffs: core.FullIndividualBuffs,
	}
}

func restorationShaman() *proto.Player {
	return &proto.Player{
		Name:          "RestoShaman",
		Race:          proto.Race_RaceTroll,
		Class:         proto.Class_ClassShaman,
		Equipment:     core.GetGearSet("../../ui/restoration_shaman/gear_sets", "p1").GearSet,
		Rotation:      castsNothing,
		TalentsString: "-3020503-50005331335310501122331251",
		Spec: &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{Options: &proto.RestorationShaman_Options{
			Shield: proto.ShamanShield_WaterShield,
			Totems: &proto.ShamanTotems{
				Earth: proto.EarthTotem_TremorTotem,
				Air:   proto.AirTotem_WrathOfAirTotem,
				Water: proto.WaterTotem_ManaSpringTotem,
				Fire:  proto.FireTotem_FlametongueTotem,
			},
		}}},
		Buffs: core.FullIndividualBuffs,
	}
}

func request(player *proto.Player, iterations int32) *proto.StatSlopesRequest {
	raid := core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, core.FullRaidBuffs, &proto.Debuffs{})
	if core.IsHealerSpec(core.PlayerProtoToSpec(player)) {
		// what a healer heals in an individual sim
		raid.TargetDummies = 1
	}
	return &proto.StatSlopesRequest{
		Raid: raid,
		Encounter: &proto.Encounter{
			Duration:             180,
			ExecuteProportion_20: 0.2,
			ExecuteProportion_25: 0.25,
			ExecuteProportion_35: 0.35,
			Targets:              []*proto.Target{core.NewDefaultTarget()},
		},
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: 101},
	}
}

func needDB(t *testing.T) {
	t.Helper()
	if !core.WITH_DB {
		t.Skip("needs the with_db item data")
	}
}

func measure(t *testing.T, req *proto.StatSlopesRequest) *proto.StatSlopesResult {
	t.Helper()
	result := Measure(req, nil)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	return result
}

func slopeOf(t *testing.T, result *proto.StatSlopesResult, st proto.Stat) *proto.StatSlope {
	t.Helper()
	for _, s := range result.Slopes {
		if !s.Pseudo && s.Stat == st {
			return s
		}
	}
	t.Fatalf("no slope for %s", st)
	return nil
}

func TestRoleOfCoversEverySpec(t *testing.T) {
	healers := map[proto.Spec]bool{proto.Spec_SpecHolyPaladin: true, proto.Spec_SpecRestorationDruid: true,
		proto.Spec_SpecRestorationShaman: true, proto.Spec_SpecHealingPriest: true}
	for value := range proto.Spec_name {
		spec := proto.Spec(value)
		r, err := RoleOf(spec)
		if err != nil {
			t.Errorf("%s: %v", spec, err)
			continue
		}
		if isHealer := reflect.DeepEqual(r, Healer); isHealer != healers[spec] {
			t.Errorf("%s: healer role %v, want %v", spec, isHealer, healers[spec])
		}
	}
}

// tools/statweights' shipped values depend on these staying as they are
func TestCenteredStepsKeepTheirRules(t *testing.T) {
	final := make([]float64, stats.Len)
	final[proto.Stat_StatArmorPenetration] = 30
	want := map[string]Step{
		"StatStrength":          {Delta: 100},
		"StatAttackPower":       {Delta: 200},
		"StatMeleeHaste":        {Delta: 300},
		"StatSpellHaste":        {Delta: 300},
		"StatExpertise":         {Delta: 30, Shift: -60},
		"StatMeleeHit":          {Delta: 100, Shift: -200},
		"StatSpellHit":          {Delta: 100, Shift: -250},
		"StatArmorPenetration":  {Delta: 100, Shift: 70},
		"PseudoStatMainHandDps": {Delta: 10, Pseudo: true},
	}
	steps := CenteredSteps(Melee, final)
	if len(steps) != len(Melee.Stats)+len(Melee.PseudoStats) {
		t.Fatalf("%d steps for %d stats", len(steps), len(Melee.Stats)+len(Melee.PseudoStats))
	}
	for _, s := range steps {
		if w, ok := want[s.Name]; ok && (s.Delta != w.Delta || s.Shift != w.Shift || s.Pseudo != w.Pseudo) {
			t.Errorf("%s: delta %v shift %v pseudo %v, want %v %v %v", s.Name, s.Delta, s.Shift, s.Pseudo, w.Delta, w.Shift, w.Pseudo)
		}
	}

	final[proto.Stat_StatArmorPenetration] = 500
	for _, s := range CenteredSteps(Tank, final) {
		if s.Name == "StatArmorPenetration" && s.Shift != -100 {
			t.Errorf("ArP shift %v at 500 rating, want -100", s.Shift)
		}
		if s.Name == "StatArmor" && (s.Delta != 1000 || s.Shift != 0) {
			t.Errorf("armor delta %v shift %v, want 1000 0", s.Delta, s.Shift)
		}
	}
}

func TestOneSidedStepsAreTheCenteredDeltasUnshifted(t *testing.T) {
	centered := CenteredSteps(Tank, make([]float64, stats.Len))
	for i, s := range OneSidedSteps(Tank) {
		if s.Shift != 0 || s.Name != centered[i].Name || s.Delta != centered[i].Delta || s.Pseudo != centered[i].Pseudo {
			t.Errorf("%+v, centered %+v", s, centered[i])
		}
	}
}

func TestLowDeltaStopsAtZeroRating(t *testing.T) {
	arp := Step{Name: "StatArmorPenetration", Index: int(proto.Stat_StatArmorPenetration), Delta: 100}
	final := make([]float64, stats.Len)
	for _, c := range []struct{ rating, want float64 }{{0, 0}, {40, 40}, {500, 100}} {
		final[arp.Index] = c.rating
		if got := lowDelta(arp, final); got != c.want {
			t.Errorf("at %v ArP: %v, want %v", c.rating, got, c.want)
		}
	}
	if got := lowDelta(Step{Pseudo: true, Delta: 10}, final); got != 10 {
		t.Errorf("pseudo stat: %v, want 10", got)
	}
}

func TestMeasureAtTheGear(t *testing.T) {
	needDB(t)
	var done, total int32
	reports := 0
	result := Measure(request(furyPlayer(), 100), func(d, n int32) {
		reports++
		done, total = d, n
	})
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	sims := int32(1)
	for _, s := range result.Slopes {
		sims++
		if s.LowDelta > 0 {
			sims++
		}
	}
	if len(result.Slopes) != len(Melee.Stats)+len(Melee.PseudoStats) || total != sims || done != total || reports != int(total) {
		t.Fatalf("%d slopes, %d of %d sims reported in %d reports, want %d sims", len(result.Slopes), done, total, reports, sims)
	}
	if result.Base.Dps <= 0 {
		t.Fatalf("base dps %v", result.Base.Dps)
	}
	t.Logf("base %.0f DPS", result.Base.Dps)
	for _, s := range result.Slopes {
		name := s.Stat.String()
		if s.Pseudo {
			name = s.PseudoStat.String()
		}
		t.Logf("%s: +%v %.3f, -%v %.3f", name, s.Delta, s.Gain.Dps, s.LowDelta, s.Loss.GetDps())
	}
	for _, st := range []proto.Stat{proto.Stat_StatAttackPower, proto.Stat_StatStrength} {
		s := slopeOf(t, result, st)
		if s.Gain.Dps <= 0 || s.Loss.Dps <= 0 {
			t.Errorf("%s: gain %v, loss %v per point", st, s.Gain.Dps, s.Loss.Dps)
		}
	}
}

func TestHitPastTheCapIsWorthNothingEitherWay(t *testing.T) {
	needDB(t)
	below := slopeOf(t, measure(t, request(plainMage(), 100)), proto.Stat_StatSpellHit)
	if below.Gain.Dps <= 0 || below.Loss.Dps <= 0 {
		t.Errorf("below the cap: gain %v, loss %v per point", below.Gain.Dps, below.Loss.Dps)
	}

	// far enough past the cap that 100 less is still past it
	mage := plainMage()
	mage.BonusStats = &proto.UnitStats{Stats: make([]float64, stats.Len)}
	mage.BonusStats.Stats[proto.Stat_StatSpellHit] = 700
	past := slopeOf(t, measure(t, request(mage, 100)), proto.Stat_StatSpellHit)
	if math.Abs(past.Gain.Dps) > 1e-9 || math.Abs(past.Loss.Dps) > 1e-9 {
		t.Errorf("past the cap: gain %v, loss %v per point", past.Gain.Dps, past.Loss.Dps)
	}
}

func TestHealerIsMeasuredInHPS(t *testing.T) {
	needDB(t)
	for _, tc := range []struct {
		name   string
		player func() *proto.Player
		// false while the spec's default APL casts no heals
		heals bool
	}{
		{"HealingPriest", holyPriest, true},
		{"HolyPaladin", holyPaladin, false},
		{"RestorationDruid", restorationDruid, false},
		{"RestorationShaman", restorationShaman, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			player := tc.player()
			if role, err := RoleOf(core.PlayerProtoToSpec(player)); err != nil || !reflect.DeepEqual(role, Healer) {
				t.Fatalf("role %v, err %v", role, err)
			}
			result := measure(t, request(player, 50))
			if len(result.Slopes) != len(Healer.Stats) || (tc.heals && result.Base.Hps <= 0) {
				t.Fatalf("%d slopes, base hps %v", len(result.Slopes), result.Base.Hps)
			}
			if sp := slopeOf(t, result, proto.Stat_StatSpellPower); tc.heals && sp.Gain.Hps <= 0 {
				t.Errorf("spell power gains %v HPS per point", sp.Gain.Hps)
			}
		})
	}
}

func TestMeasureAsyncReportsEachSimThenTheResult(t *testing.T) {
	needDB(t)
	progress := make(chan *proto.ProgressMetrics, 100)
	MeasureAsync(request(plainMage(), 20), progress)
	var done, total int32
	for p := range progress {
		if p.FinalSlopesResult != nil {
			if p.FinalSlopesResult.ErrorResult != "" {
				t.Fatal(p.FinalSlopesResult.ErrorResult)
			}
			if total == 0 || done != total {
				t.Fatalf("the result came after %d of %d sims", done, total)
			}
			return
		}
		if p.CompletedSims != done+1 || p.TotalSims < p.CompletedSims || p.TotalIterations != 20*p.TotalSims {
			t.Fatalf("progress %d of %d sims after %d, %d iterations", p.CompletedSims, p.TotalSims, done, p.TotalIterations)
		}
		done, total = p.CompletedSims, p.TotalSims
	}
}

// The sim server runs these side by side, each on a request of its own. Under -race this catches
// anything they share.
func TestRequestsSideBySide(t *testing.T) {
	needDB(t)
	// decoded requests share nothing, request() shares core's FullRaidBuffs
	own := func(player *proto.Player, iterations int32) *proto.StatSlopesRequest {
		return googleProto.Clone(request(player, iterations)).(*proto.StatSlopesRequest)
	}
	var wg sync.WaitGroup
	run := func(name string, f func() string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(); err != "" {
				t.Errorf("%s: %s", name, err)
			}
		}()
	}
	for range 2 {
		for _, player := range []func() *proto.Player{furyPlayer, plainMage} {
			run("statCaps", func() string {
				req := own(player(), 1)
				return statcaps.ComputeRequest(&proto.StatCapsRequest{Raid: req.Raid, Encounter: req.Encounter}).ErrorResult
			})
		}
		run("statSlopesAsync", func() string {
			progress := make(chan *proto.ProgressMetrics, 100)
			MeasureAsync(own(plainMage(), 3), progress)
			for p := range progress {
				if p.FinalSlopesResult != nil {
					return p.FinalSlopesResult.ErrorResult
				}
			}
			return "no result"
		})
		run("computeStats", func() string {
			req := own(furyPlayer(), 1)
			return core.ComputeStats(&proto.ComputeStatsRequest{Raid: req.Raid, Encounter: req.Encounter}).ErrorResult
		})
		run("raidSimAsync", func() string {
			req := own(furyPlayer(), 20)
			progress := make(chan *proto.ProgressMetrics, 100)
			core.RunRaidSimAsync(&proto.RaidSimRequest{Raid: req.Raid, Encounter: req.Encounter, SimOptions: req.SimOptions}, progress)
			for p := range progress {
				if p.FinalRaidResult != nil {
					return p.FinalRaidResult.ErrorResult
				}
			}
			return "no result"
		})
	}
	wg.Wait()
}

func TestMeasureRefusesARequestItCantRun(t *testing.T) {
	noPlayer := request(plainMage(), 10)
	noPlayer.Raid.Parties[0].Players = nil
	noIterations := request(plainMage(), 0)
	for name, req := range map[string]*proto.StatSlopesRequest{"no player": noPlayer, "no iterations": noIterations} {
		if result := Measure(req, nil); result.ErrorResult == "" {
			t.Errorf("%s: no error", name)
		}
	}
}
