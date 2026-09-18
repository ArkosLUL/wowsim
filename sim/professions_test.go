package sim

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func professionsTestWarrior(professions []proto.Profession, slots ...proto.Profession) *proto.Player {
	player := racialTraitsTestWarrior(proto.Race_RaceOrc, proto.Race_RaceUnknown)
	player.Professions = professions
	if len(slots) > 0 {
		player.Profession1 = slots[0]
	}
	if len(slots) > 1 {
		player.Profession2 = slots[1]
	}
	return player
}

// An AzerothCore character can know more than two professions, so every one of them counts.
func TestProfessionsBeyondTwoSlotsApply(t *testing.T) {
	none := computePlayerStats(t, professionsTestWarrior(nil))[0]
	all := computePlayerStats(t, professionsTestWarrior([]proto.Profession{
		proto.Profession_Blacksmithing, proto.Profession_Engineering, proto.Profession_Mining, proto.Profession_Skinning,
	}))[0]

	for stat, want := range map[stats.Stat]float64{stats.Stamina: 60, stats.MeleeCrit: 40, stats.SpellCrit: 40} {
		if got := all.FinalStats.Stats[stat] - none.FinalStats.Stats[stat]; math.Abs(got-want) > 0.001 {
			t.Errorf("%s from the third and fourth profession = %v, want %v", stat.StatName(), got, want)
		}
	}
}

func TestProfessionsFallBackToTheTwoSlots(t *testing.T) {
	slots := computePlayerStats(t, professionsTestWarrior(nil, proto.Profession_Mining, proto.Profession_Skinning))[0]
	list := computePlayerStats(t, professionsTestWarrior([]proto.Profession{proto.Profession_Mining, proto.Profession_Skinning}))[0]

	if slots.FinalStats.Stats[stats.Stamina] != list.FinalStats.Stats[stats.Stamina] ||
		slots.FinalStats.Stats[stats.MeleeCrit] != list.FinalStats.Stats[stats.MeleeCrit] {
		t.Errorf("older links lost their professions: slots %v, list %v", slots.FinalStats.Stats, list.FinalStats.Stats)
	}

	// the list wins, so a player who dropped a profession doesn't keep it through the old fields
	dropped := computePlayerStats(t, professionsTestWarrior([]proto.Profession{proto.Profession_Mining},
		proto.Profession_Mining, proto.Profession_Skinning))[0]
	if got := list.FinalStats.Stats[stats.MeleeCrit] - dropped.FinalStats.Stats[stats.MeleeCrit]; math.Abs(got-40) > 0.001 {
		t.Errorf("Skinning crit still applied from the old fields: %v", got)
	}
}
