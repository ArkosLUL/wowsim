package healing

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// pageRequest is what the page sends for one tree at P1: its default fight, buffs and consumes, 5 parties of 5
// slots with the blank ones included, and 9 target dummies in the blanks.
func pageRequest(tree string) *proto.RaidSimRequest {
	talents, glyphs, options := DiscTalents, DiscGlyphs, PlayerOptionsDisc
	if tree == "holy" {
		talents, glyphs, options = HolyTalents, HolyGlyphs, PlayerOptionsHoly
	}
	rsr := &proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(
			&proto.Player{
				Race:          proto.Race_RaceUndead,
				Class:         proto.Class_ClassPriest,
				Equipment:     core.GetGearSet("../../../ui/healing_priest/gear_sets", "p1_"+tree).GearSet,
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
				ReactionTimeMs:     150,
				ChannelClipDelayMs: 50,
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
			&proto.Debuffs{}),
		Encounter:  core.MakeSingleTargetEncounter(5),
		SimOptions: &proto.SimOptions{Iterations: 300, IsTest: true, RandomSeed: 101},
	}
	rsr.Encounter.Duration = 180
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

// At P1 both trees heal with every APL spell and run dry inside the default 180 s, so the mana stats measure.
func TestPageRequestHealsAndRunsShortOfManaAtP1(t *testing.T) {
	for tree, spells := range map[string]map[string]int32{
		"disc": {"Power Word: Shield": 48066, "Prayer of Mending": 48113, "Penance": 53007, "Greater Heal": 48063},
		"holy": {"Circle of Healing": 48089, "Prayer of Mending": 48113, "Renew": 48068, "Greater Heal": 48063},
	} {
		result := core.RunRaidSim(pageRequest(tree))
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		unit := result.RaidMetrics.Parties[0].Players[0]
		if unit.Hps.Avg < 5000 {
			t.Errorf("%s: HPS %.0f, want the dummies healed", tree, unit.Hps.Avg)
		}
		if unit.Tto.Avg >= 180 {
			t.Errorf("%s: out of mana after %.0f s, want before the fight's 180 s", tree, unit.Tto.Avg)
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
