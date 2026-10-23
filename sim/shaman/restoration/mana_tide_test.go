package restoration

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// The shaman's own Mana Tide restores its party, so its talent adds no external Mana Tide in its own party. One
// the party buffs name, another shaman's, still comes.
func TestOwnManaTideIsNoExternalTotem(t *testing.T) {
	for _, partyTotems := range []int32{0, 1} {
		rsr := pageRequest("p1")
		rsr.Raid.Parties[0].Buffs = &proto.PartyBuffs{ManaTideTotems: partyTotems}
		sim := core.NewSim(rsr)
		shaman := sim.Raid.Parties[0].Players[0].(*RestorationShaman)

		if shaman.GetSpell(core.ManaTideTotemActionID) == nil {
			t.Fatalf("party buffs %d: the shaman's own Mana Tide isn't registered", partyTotems)
		}
		external := shaman.GetSpell(core.ManaTideTotemActionID.WithTag(-1)) != nil
		if external != (partyTotems > 0) {
			t.Errorf("party buffs %d: external Mana Tide registered %v, want %v", partyTotems, external, partyTotems > 0)
		}
	}
}

// Tidal Waves' buff (53390) lasts 15 s, whatever charges are left.
func TestTidalWavesBuffExpires(t *testing.T) {
	sim, shaman := healSetup{}.build(t)
	startFight(sim)
	shaman.Riptide.SkipCastAndApplyEffects(sim, &shaman.Unit)

	buff := shaman.GetAuraByID(core.ActionID{SpellID: 53390})
	if buff == nil || !buff.IsActive() || buff.GetStacks() != 2 {
		t.Fatalf("Riptide gave no Tidal Waves buff with 2 charges: %v", buff)
	}
	// its end falls on an update, so give it half a second either way
	runUntil(sim, 14500*time.Millisecond)
	if !buff.IsActive() {
		t.Fatalf("the buff was gone before 14.5 s")
	}
	runUntil(sim, 15500*time.Millisecond)
	if buff.IsActive() {
		t.Errorf("the buff is still up after 15.5 s")
	}
}
