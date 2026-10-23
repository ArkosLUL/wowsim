package healing

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// pageRequest is what the page sends for one tree at one phase's gear (preraid, p1 to p5): its default fight,
// buffs and consumes, 5 parties of 5 slots with the blank ones included, and 9 target dummies in the blanks.
func pageRequest(tree, phase string) *proto.RaidSimRequest {
	talents, glyphs, options := DiscTalents, DiscGlyphs, PlayerOptionsDisc
	if tree == "holy" {
		talents, glyphs, options = HolyTalents, HolyGlyphs, PlayerOptionsHoly
	}
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:          proto.Race_RaceDraenei,
				Class:         proto.Class_ClassPriest,
				Equipment:     core.GetGearSet("../../../ui/healing_priest/gear_sets", phase+"_"+tree).GearSet,
				Consumes:      FullConsumes,
				Spec:          options,
				Rotation:      core.GetAplRotation("../../../ui/healing_priest/apls", tree).Rotation,
				TalentsString: talents,
				Glyphs:        glyphs,
				Buffs: &proto.IndividualBuffs{
					BlessingOfKings:  true,
					BlessingOfWisdom: proto.TristateEffect_TristateEffectImproved,
					VampiricTouch:    true,
				},
				Profession1:    proto.Profession_Engineering,
				Profession2:    proto.Profession_Jewelcrafting,
				ReactionTimeMs: 200,
			},
			&proto.PartyBuffs{},
			&proto.RaidBuffs{
				GiftOfTheWild:         proto.TristateEffect_TristateEffectImproved,
				PowerWordFortitude:    proto.TristateEffect_TristateEffectImproved,
				StrengthOfEarthTotem:  proto.TristateEffect_TristateEffectImproved,
				ArcaneBrilliance:      true,
				DivineSpirit:          true,
				TrueshotAura:          true,
				LeaderOfThePack:       proto.TristateEffect_TristateEffectImproved,
				IcyTalons:             true,
				TotemOfWrath:          true,
				MoonkinAura:           proto.TristateEffect_TristateEffectImproved,
				WrathOfAirTotem:       true,
				SanctifiedRetribution: true,
				Bloodlust:             true,
				DemonicPactSp:         500,
			},
			&proto.Debuffs{TotemOfWrath: true}),
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

// At P1 both trees heal with every APL spell and run dry inside the page's default fight, so the mana stats measure.
func TestPageRequestHealsAndRunsShortOfManaAtP1(t *testing.T) {
	for tree, spells := range map[string]map[string]int32{
		"disc": {"Power Word: Shield": 48066, "Prayer of Mending": 48113, "Penance": 53007, "Greater Heal": 48063},
		"holy": {"Circle of Healing": 48089, "Prayer of Mending": 48113, "Renew": 48068, "Greater Heal": 48063},
	} {
		rsr := pageRequest(tree, "p1")
		fight := rsr.Encounter.Duration
		result := core.RunRaidSim(rsr)
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		unit := result.RaidMetrics.Parties[0].Players[0]
		if unit.Hps.Avg < 5000 {
			t.Errorf("%s: HPS %.0f, want the dummies healed", tree, unit.Hps.Avg)
		}
		if unit.Tto.Avg >= fight {
			t.Errorf("%s: out of mana after %.0f s, want before the fight's %.0f s", tree, unit.Tto.Avg, fight)
		}
		for name, id := range spells {
			var healed float64
			for _, action := range unit.Actions {
				if action.Id.GetSpellId() == id {
					for _, target := range action.Targets {
						healed += target.Healing + target.Shielding
					}
				}
			}
			if healed == 0 {
				t.Errorf("%s: %s heals nothing", tree, name)
			}
		}
	}
}
