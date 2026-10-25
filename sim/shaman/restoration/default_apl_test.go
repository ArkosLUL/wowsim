package restoration

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// pageRequest is what the page sends for a Restoration Shaman at one phase's gear: the page's default fight, 9
// target dummies, and its Raid Healing talents, totems and consumes.
func pageRequest(phase string) *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:      proto.Race_RaceDraenei,
				Class:     proto.Class_ClassShaman,
				Equipment: core.GetGearSet("../../../ui/restoration_shaman/gear_sets", phase).GearSet,
				Consumes: &proto.Consumes{
					Flask:         proto.Flask_FlaskOfTheFrostWyrm,
					Food:          proto.Food_FoodFishFeast,
					DefaultPotion: proto.Potions_RunicManaInjector,
				},
				Spec: &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{Options: &proto.RestorationShaman_Options{
					Shield:         proto.ShamanShield_WaterShield,
					EarthShieldPPM: 30,
					Totems: &proto.ShamanTotems{
						Earth: proto.EarthTotem_StrengthOfEarthTotem,
						Air:   proto.AirTotem_WrathOfAirTotem,
						Fire:  proto.FireTotem_FlametongueTotem,
						Water: proto.WaterTotem_HealingStreamTotem,
					},
				}}},
				Buffs: &proto.IndividualBuffs{
					BlessingOfKings:  true,
					BlessingOfWisdom: proto.TristateEffect_TristateEffectImproved,
					VampiricTouch:    true,
				},
				Rotation:           core.GetAplRotation("../../../ui/restoration_shaman/apls", "default").Rotation,
				TalentsString:      StandardTalents,
				Glyphs:             StandardGlyphs,
				Profession1:        proto.Profession_Engineering,
				ReactionTimeMs:     150,
				ChannelClipDelayMs: 50,
			},
			&proto.PartyBuffs{},
			&proto.RaidBuffs{
				ArcaneBrilliance:      true,
				DivineSpirit:          true,
				GiftOfTheWild:         proto.TristateEffect_TristateEffectImproved,
				MoonkinAura:           proto.TristateEffect_TristateEffectImproved,
				SanctifiedRetribution: true,
			},
			&proto.Debuffs{
				FaerieFire:        proto.TristateEffect_TristateEffectImproved,
				JudgementOfWisdom: true,
				Misery:            true,
				CurseOfElements:   true,
				ShadowMastery:     true,
			}),
		Encounter:  core.MakeSingleTargetEncounter(5),
		SimOptions: &proto.SimOptions{Iterations: 300, IsTest: true, RandomSeed: 101},
	}
	rsr.Encounter.Duration = 300
	// the page sends 5 parties of 5 slots, blank ones included, and the dummies fill the blanks
	rsr.Raid.Parties = append(rsr.Raid.Parties, &proto.Party{}, &proto.Party{}, &proto.Party{}, &proto.Party{})
	for _, party := range rsr.Raid.Parties {
		for len(party.Players) < 5 {
			party.Players = append(party.Players, &proto.Player{})
		}
	}
	rsr.Raid.NumActiveParties = 5
	rsr.Raid.TargetDummies = 9
	core.SetPageIncomingDamage(rsr.Raid, "../../../ui/restoration_shaman", "default", core.PagePhase(phase))
	return rsr
}

func defaultAPLGoldenRequest() *proto.RaidSimRequest {
	rsr := pageRequest("p1")
	rsr.SimOptions = core.AverageDefaultSimTestOptions
	// the golden row is on a 180 s fight
	rsr.Encounter.Duration = 180
	return rsr
}

func castsAndHealing(unit *proto.UnitMetrics, spellID int32) (casts int32, healing float64) {
	for _, action := range unit.Actions {
		if action.Id.GetSpellId() != spellID {
			continue
		}
		for _, target := range action.Targets {
			casts += target.Casts
			healing += target.Healing
		}
	}
	return
}

func TestDefaultAPLHeals(t *testing.T) {
	result := core.RunRaidSim(pageRequest("p1"))
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	unit := result.RaidMetrics.Parties[0].Players[0]
	if unit.Hps.Avg < 5000 {
		t.Errorf("HPS %.0f, want a rotation that heals", unit.Hps.Avg)
	}
	for name, id := range map[string]int32{
		"Riptide": 61301, "Chain Heal": 55459, "Earth Shield": 49284, "Tidal Force": 55198,
		"Healing Stream Totem": 58757, "Nature's Swiftness": 16188,
	} {
		if casts, _ := castsAndHealing(unit, id); casts == 0 {
			t.Errorf("%s is never cast", name)
		}
	}
	if casts, _ := castsAndHealing(unit, 49273); casts != 0 {
		t.Errorf("Healing Wave is cast %d times, the default APL has no line for it", casts)
	}
	for name, id := range map[string]int32{"Riptide": 61301, "Chain Heal": 55459, "Earth Shield": 49284} {
		if _, healing := castsAndHealing(unit, id); healing == 0 {
			t.Errorf("%s heals nothing", name)
		}
	}
}

// the P1 gear has to run dry inside the page's default fight, or mana stats measure nothing
func TestDefaultAPLRunsShortOfManaAtP1(t *testing.T) {
	rsr := pageRequest("p1")
	fight := rsr.Encounter.Duration
	result := core.RunRaidSim(rsr)
	if result.ErrorResult != "" {
		t.Fatal(result.ErrorResult)
	}
	unit := result.RaidMetrics.Parties[0].Players[0]
	if unit.Tto.Avg >= fight {
		t.Errorf("out of mana after %.0f s, want before the fight's %.0f s", unit.Tto.Avg, fight)
	}
}

// the preset gear has to be obtainable at its phase on the server: ICC and Ruby Sanctum open at tiers 16 and 17
func TestPresetGearIsInTheServerCatalog(t *testing.T) {
	data, err := os.ReadFile("../../../assets/database/server_catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Items []struct {
			Id              int32 `json:"id"`
			ProgressionTier int32 `json:"progressionTier"`
			Pvp             bool  `json:"pvp"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	tiers := map[int32]int32{}
	pvp := map[int32]bool{}
	for _, item := range catalog.Items {
		tiers[item.Id] = item.ProgressionTier
		pvp[item.Id] = item.Pvp
	}

	for phase, tier := range map[string]int32{"preraid": 13, "p1": 13, "p2": 14, "p3": 15, "p4": 16, "p5": 17} {
		for _, item := range core.GetGearSet("../../../ui/restoration_shaman/gear_sets", phase).GearSet.Items {
			ids := []int32{item.Id}
			ids = append(ids, item.Gems...)
			for _, id := range ids {
				if id == 0 {
					continue
				}
				got, ok := tiers[id]
				if !ok {
					t.Errorf("%s: %d is not in the server catalog", phase, id)
				} else if got > tier {
					t.Errorf("%s: %d opens at tier %d, after the phase's %d", phase, id, got, tier)
				}
				if pvp[id] {
					t.Errorf("%s: %d is a PvP item", phase, id)
				}
			}
		}
	}
}
