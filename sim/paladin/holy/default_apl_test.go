package holy

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// pageRequest is what the page sends for a Holy Paladin at one phase's gear: its default fight, 5 parties of 5 slots
// with 9 target dummies in the blanks, and its default talents, glyphs and consumes.
func pageRequest(phase string) *proto.RaidSimRequest {
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:      proto.Race_RaceDraenei,
				Class:     proto.Class_ClassPaladin,
				Equipment: core.GetGearSet("../../../ui/holy_paladin/gear_sets", phase).GearSet,
				Consumes: &proto.Consumes{
					Flask:         proto.Flask_FlaskOfTheFrostWyrm,
					Food:          proto.Food_FoodFishFeast,
					DefaultPotion: proto.Potions_RunicManaInjector,
				},
				Spec: &proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{Options: &proto.HolyPaladin_Options{
					Aura:      proto.PaladinAura_DevotionAura,
					Judgement: proto.PaladinJudgement_NoJudgement,
				}}},
				Buffs: &proto.IndividualBuffs{
					BlessingOfKings:  true,
					BlessingOfWisdom: proto.TristateEffect_TristateEffectImproved,
					VampiricTouch:    true,
				},
				Rotation:           core.GetAplRotation("../../../ui/holy_paladin/apls", "default").Rotation,
				TalentsString:      StandardTalents,
				Glyphs:             pageGlyphs,
				Profession1:        proto.Profession_Engineering,
				Profession2:        proto.Profession_Jewelcrafting,
				ReactionTimeMs:     200,
				ChannelClipDelayMs: 0,
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
	rsr.Raid.Parties = append(rsr.Raid.Parties, &proto.Party{}, &proto.Party{}, &proto.Party{}, &proto.Party{})
	for _, party := range rsr.Raid.Parties {
		for len(party.Players) < 5 {
			party.Players = append(party.Players, &proto.Player{})
		}
	}
	rsr.Raid.NumActiveParties = 5
	rsr.Raid.TargetDummies = 9
	return rsr
}

var pageGlyphs = &proto.Glyphs{
	Major1: int32(proto.PaladinMajorGlyph_GlyphOfHolyLight),
	Major2: int32(proto.PaladinMajorGlyph_GlyphOfSealOfWisdom),
	Major3: int32(proto.PaladinMajorGlyph_GlyphOfBeaconOfLight),
	Minor1: int32(proto.PaladinMinorGlyph_GlyphOfSenseUndead),
	Minor2: int32(proto.PaladinMinorGlyph_GlyphOfLayOnHands),
	Minor3: int32(proto.PaladinMinorGlyph_GlyphOfBlessingOfKings),
}

func healedBy(unit *proto.UnitMetrics, spellID int32) (casts int32, healing float64) {
	for _, action := range unit.Actions {
		if action.Id.GetSpellId() != spellID {
			continue
		}
		for _, target := range action.Targets {
			casts += target.Casts
			healing += target.Healing + target.Shielding
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
		"Holy Light": 48782, "Holy Shock": 48825, "Beacon of Light": 53563, "Sacred Shield": 53601,
		"Judgement of Wisdom": 53408, "Divine Illumination": 31842, "Divine Favor": 20216,
	} {
		if casts, _ := healedBy(unit, id); casts == 0 {
			t.Errorf("%s is never cast", name)
		}
	}
	for name, id := range map[string]int32{"Holy Light": 48782, "Holy Shock heal": 48821, "Sacred Shield": 58597} {
		if _, healing := healedBy(unit, id); healing == 0 {
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
		for _, item := range core.GetGearSet("../../../ui/holy_paladin/gear_sets", phase).GearSet.Items {
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
