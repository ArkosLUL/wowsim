package elemental

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

var (
	potionOfWildMagic  = core.ActionID{ItemID: 40212}
	destructionPotion  = core.ActionID{ItemID: 22839}
	superManaPotion    = core.ActionID{ItemID: 22832}
	totemOfWrathCastID = core.ActionID{SpellID: 57722}
)

// The default APL pre-pots and drops its totems before the pull; a bot does neither and drinks Wild Magic
// once the fight is 5 s old.
func TestPlayerbotElemental(t *testing.T) {
	for _, playerbot := range []bool{false, true} {
		res := core.RunRaidSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:               proto.Race_RaceTroll,
				Class:              proto.Class_ClassShaman,
				Equipment:          core.GetGearSet("../../../ui/elemental_shaman/gear_sets", "p1").GearSet,
				TalentsString:      StandardTalents,
				Glyphs:             StandardGlyphs,
				Consumes:           FullConsumes,
				Spec:               PlayerOptionsAdaptive,
				Buffs:              core.FullIndividualBuffs,
				Rotation:           core.GetAplRotation("../../../ui/elemental_shaman/apls", "default").Rotation,
				Profession1:        proto.Profession_Engineering,
				DistanceFromTarget: 30,
				ReactionTimeMs:     150,
				ChannelClipDelayMs: 50,
				Playerbot:          playerbot,
			}, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, Debug: true, RandomSeed: 101},
		})
		if res.ErrorResult != "" {
			t.Fatal(res.ErrorResult)
		}

		wildMagic := core.CastTimes(res.Logs, potionOfWildMagic)
		others := len(core.CastTimes(res.Logs, destructionPotion)) + len(core.CastTimes(res.Logs, superManaPotion))
		prepullTotem := slices.ContainsFunc(core.CastTimes(res.Logs, totemOfWrathCastID), func(at time.Duration) bool { return at < 0 })
		if playerbot {
			if len(wildMagic) != 1 || wildMagic[0] < core.PlayerbotPotionAt || others != 0 {
				t.Errorf("playerbot: Wild Magic at %v and %d other potions, want one Wild Magic from %v", wildMagic, others, core.PlayerbotPotionAt)
			}
			if prepullTotem {
				t.Error("playerbot: Totem of Wrath went down before the pull")
			}
		} else {
			if pots := core.CastTimes(res.Logs, destructionPotion); len(pots) == 0 || pots[0] != -time.Second {
				t.Errorf("Destruction Potion at %v, want the pre-pot at -1s", pots)
			}
			if !prepullTotem {
				t.Error("no Totem of Wrath before the pull")
			}
		}
	}
}
