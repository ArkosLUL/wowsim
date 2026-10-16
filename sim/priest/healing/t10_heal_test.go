package healing

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// spell_pri_t10_heal_2p_bonus casts 70772 for 33% of the Flash Heal shared out over its 3 ticks, so a tick is about
// 11% of the heal, then the healing done talents (Blessed Resilience's 3%) as any hot's tick takes them.
func TestCrimsonAcolytesRaimentHotIsAThirdOfItsShareATick(t *testing.T) {
	items := make([]*proto.ItemSpec, proto.ItemSlot_ItemSlotRanged+1)
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	items[proto.ItemSlot_ItemSlotHead] = &proto.ItemSpec{Id: 50765}
	items[proto.ItemSlot_ItemSlotHands] = &proto.ItemSpec{Id: 50766}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid: core.SinglePlayerRaidProto(&proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{Items: items},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsHoly,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: BlessedResilienceTalents,
		}, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	priest := sim.Raid.Parties[0].Players[0].(*HealingPriest)
	if priest.Talents.BlessedResilience != 3 || priest.Talents.FocusedPower != 0 {
		t.Fatalf("Blessed Resilience %d, Focused Power %d", priest.Talents.BlessedResilience, priest.Talents.FocusedPower)
	}
	proc := priest.GetSpell(core.ActionID{SpellID: 70770})
	if proc == nil {
		t.Fatal("no 2-piece bonus")
	}

	target := &priest.Unit
	var flashHeal float64
	priest.RegisterAura(core.Aura{
		Label:    "t10 test",
		Duration: core.NeverExpires,
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == priest.FlashHeal {
				flashHeal = result.Damage
			}
		},
	}).Activate(sim)
	hot := proc.Hot(target)
	for i := 0; i < 200 && !hot.IsActive(); i++ {
		priest.FlashHeal.SkipCastAndApplyEffects(sim, target)
	}
	if !hot.IsActive() {
		t.Fatal("200 Flash Heals never procced the hot")
	}

	metrics := &proc.SpellMetrics[target.UnitIndex]
	before := metrics.TotalHealing
	hot.TickOnce(sim)
	if got, want := metrics.TotalHealing-before, flashHeal*0.11*1.03; math.Abs(got-want) > 2 {
		t.Errorf("a tick healed %v after a %v Flash Heal, want about %v", got, flashHeal, want)
	}
}
