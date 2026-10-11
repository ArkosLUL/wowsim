package hunter

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// replenishmentUptime is how long a petless Survival hunter who casts nothing but spell has Replenishment
// over a minute, with no one else to give it.
func replenishmentUptime(t *testing.T, spell int32) float64 {
	t.Helper()
	options := googleProto.Clone(PlayerOptionsBasic.Hunter.Options).(*proto.Hunter_Options)
	options.PetType = proto.Hunter_Options_PetNone
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = 60
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:      proto.Race_RaceOrc,
			Class:     proto.Class_ClassHunter,
			Equipment: core.GetGearSet("../../ui/hunter/gear_sets", "p1_sv").GearSet,
			Spec:      &proto.Player_Hunter{Hunter: &proto.Hunter{Options: options}},
			Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL, PriorityList: []*proto.APLListItem{{
				Action: &proto.APLAction{Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{
					SpellId: &proto.ActionID{RawId: &proto.ActionID_SpellId{SpellId: spell}},
				}}},
			}}},
			TalentsString: SVTalents,
		}, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	player := res.RaidMetrics.Parties[0].Players[0]
	cast := false
	for _, action := range player.Actions {
		if action.Id.GetSpellId() == spell {
			cast = true
		}
	}
	if !cast {
		t.Fatalf("the hunter never cast %d", spell)
	}
	for _, aura := range player.Auras {
		if aura.Id.GetSpellId() == 57669 {
			return aura.UptimeSecondsAvg
		}
	}
	return 0
}

// Hunting Party's mask holds 53352, the damage each Explosive Shot tick casts, and takes triggered spells
// (PROC_ATTR_TRIGGERED_CAN_PROC): a tick's crit gives Replenishment. Serpent Sting is outside the mask.
func TestHuntingPartyProcsOffExplosiveShotTicks(t *testing.T) {
	if got := replenishmentUptime(t, 60053); got < 30 {
		t.Errorf("Explosive Shot alone kept Replenishment up %.1f s of 60, want most of the fight", got)
	}
	if got := replenishmentUptime(t, 49001); got != 0 {
		t.Errorf("Serpent Sting alone kept Replenishment up %.1f s, want none", got)
	}
}
