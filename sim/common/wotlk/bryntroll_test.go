package wotlk_test

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/warrior/dps"
)

func init() {
	dps.RegisterDpsWarrior()
}

const bryntrollArmsTalents = "3022032023335100102012213231251-305-2033"

func bryntrollWarrior(axe int32) *proto.Player {
	gear := core.GetGearSet("../../../ui/warrior/gear_sets", "p1_arms").GearSet
	gear.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: axe}
	return &proto.Player{
		Race:          proto.Race_RaceOrc,
		Class:         proto.Class_ClassWarrior,
		TalentsString: bryntrollArmsTalents,
		Equipment:     gear,
		Consumes:      &proto.Consumes{},
		Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{
			StartingRage: 50,
		}}},
		Rotation: core.GetAplRotation("../../../ui/warrior/apls", "arms").Rotation,
	}
}

// Drain Life heals the wielder for what it deals, whole after the healing done percent, and never crits (both
// ids carry SPELL_ATTR2_CANT_CRIT). Strength of Wrynn's 30% reaches the heal: 71838 is magic class, so
// SpellHealingBonusDone doesn't hand the amount back.
func TestBryntrollHealsForItsDamage(t *testing.T) {
	for _, c := range []struct {
		axe, spell int32
		wrynn      bool
	}{{50415, 71838, false}, {50709, 71839, false}, {50415, 71838, true}} {
		result := core.RunRaidSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(bryntrollWarrior(c.axe), &proto.PartyBuffs{},
				&proto.RaidBuffs{StrengthOfWrynn: c.wrynn}, &proto.Debuffs{}),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 101},
		})
		if result.ErrorResult != "" {
			t.Fatalf("axe %d: %s", c.axe, result.ErrorResult)
		}
		player := result.RaidMetrics.Parties[0].Players[0]

		var damage, crits, hits float64
		for _, action := range player.Actions {
			if action.Id.GetSpellId() != c.spell {
				continue
			}
			for _, target := range action.Targets {
				damage += target.Damage
				crits += float64(target.Crits)
				hits += float64(target.Hits + target.Crits)
			}
		}
		var healed float64
		for _, resource := range player.Resources {
			if resource.Id.GetSpellId() == c.spell && resource.Type == proto.ResourceType_ResourceTypeHealth {
				healed += resource.Gain
			}
		}
		if hits < 100 {
			t.Fatalf("axe %d: %v hits of %d, too few to tell", c.axe, hits, c.spell)
		}
		if crits != 0 {
			t.Errorf("axe %d: %v of %v hits of %d crit, and the server never crits it", c.axe, crits, hits, c.spell)
		}

		// each heal truncates once more under the 30%, so it can fall a point short a hit
		want, slack := damage, 1e-6
		if c.wrynn {
			want, slack = damage*1.3, hits
		}
		if healed > want+1e-6 || healed < want-slack {
			t.Errorf("axe %d, Strength of Wrynn %v: healed %.1f for %.1f damage over %v hits, want %.1f", c.axe, c.wrynn, healed, damage, hits, want)
		}
	}
}

// The item gives 71838 a 500 ms cooldown, so one Whirlwind or Cleave can't proc it twice.
func TestBryntrollProcsHalfASecondApart(t *testing.T) {
	casting := regexp.MustCompile(`^\[(-?\d+\.\d+)\].*Casting \{SpellID: 71838\}`)
	encounter := core.MakeSingleTargetEncounter(0)
	for len(encounter.Targets) < 10 {
		encounter.Targets = append(encounter.Targets, encounter.Targets[0])
	}

	procs := 0
	for seed := int64(1); seed <= 4; seed++ {
		result := core.RunRaidSim(&proto.RaidSimRequest{
			Raid:       core.SinglePlayerRaidProto(bryntrollWarrior(50415), &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
			Encounter:  encounter,
			SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: seed, Debug: true},
		})
		if result.ErrorResult != "" {
			t.Fatal(result.ErrorResult)
		}
		last := -1.0
		for _, line := range strings.Split(result.Logs, "\n") {
			m := casting.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			at, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				t.Fatal(err)
			}
			procs++
			if last >= 0 && at-last < 0.5 {
				t.Errorf("seed %d: Drain Life at %.2f s and again at %.2f s, inside its 500 ms cooldown", seed, last, at)
			}
			last = at
		}
	}
	if procs < 40 {
		t.Fatalf("%d Drain Life casts in the logs, too few to tell", procs)
	}
}
