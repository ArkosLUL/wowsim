// Package raidctx turns a raid into the individual context the optimizer's search sims: one raider
// alone, with the buffs and debuffs the rest of the roster gives it.
package raidctx

import (
	"errors"
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TargetIndex is where Derive puts the target: alone in the first party.
const TargetIndex = 0

// Derive returns a request that sims base's raider at targetIndex (party index * 5 + position) on
// its own, with what the rest of the active raid gives it:
//   - raid and party buffs from the other players' AddRaidBuffs and AddPartyBuffs, on top of base's
//   - debuffs, replenishment and the buffs no player adds itself, from providers
//   - targeted buffs whose spec-option UnitReference names the target
//   - Demonic Pact, at the spell power DemonicPactSP gives from the warlock's sheet in the full raid
//
// Nothing comes from the target itself: the sim adds that on its own. The target's IndividualBuffs,
// which hold the raid UI's blessings, carry over. References to the target, in its own settings and
// in Raid.tanks, move with it to TargetIndex; references to anyone else are cleared. Tank entries
// keep their positions, so the encounter's tank indexes still line up. Target dummies are dropped:
// a mage alone in the raid uses its Focus Magic uptime option, like the individual sim.
//
// base is never changed.
func Derive(base *proto.RaidSimRequest, targetIndex int) (derived *proto.RaidSimRequest, err error) {
	if base.GetRaid() == nil {
		return nil, errors.New("request has no raid")
	}
	numParties := activeParties(base.Raid)
	if err := checkTarget(base.Raid, numParties, targetIndex); err != nil {
		return nil, err
	}
	defer func() {
		if r := recover(); r != nil {
			derived, err = nil, fmt.Errorf("deriving raid index %d: %v", targetIndex, r)
		}
	}()

	req := goproto.Clone(base).(*proto.RaidSimRequest)
	partyIndex, slot := targetIndex/5, targetIndex%5
	target := req.Raid.Parties[partyIndex].Players[slot]

	// core's buff APIs write into the raid they get
	others := goproto.Clone(base.Raid).(*proto.Raid)
	others.Parties[partyIndex].Players[slot] = &proto.Player{}
	others.TargetDummies = 0
	raid := core.NewRaid(others, core.NewServerSettings(base.Encounter.GetServerSettings()))

	got := &effects{
		raid:       raid.GetRaidBuffs(others.Buffs),
		debuffs:    req.Raid.Debuffs,
		individual: target.Buffs,
	}
	if got.debuffs == nil {
		got.debuffs = &proto.Debuffs{}
	}
	if got.individual == nil {
		got.individual = &proto.IndividualBuffs{}
	}
	partyBuffs := req.Raid.Parties[partyIndex].Buffs
	for _, party := range raid.Parties {
		if party.Index == partyIndex {
			partyBuffs = party.GetPartyBuffs(partyBuffs)
		}
	}

	var pacts []pact
	for _, r := range activePlayers(req.Raid, numParties) {
		if r.index == targetIndex {
			continue
		}
		spec := core.PlayerProtoToSpec(r.player)
		talents := parseTalents(r.player)
		for i := range providers {
			if providers[i].gives(r.player, spec, talents, targetIndex) {
				providers[i].apply(got)
			}
		}
		if demonicPact.gives(r.player, spec, talents, targetIndex) {
			pacts = append(pacts, pact{index: r.index, points: talentPoints(talents, demonicPact.talent)})
		}
	}
	if len(pacts) > 0 {
		got.raid.DemonicPactSp = max(got.raid.DemonicPactSp, demonicPactSP(base, pacts))
	}

	target.Buffs = got.individual
	followTarget(target.ProtoReflect(), targetIndex)
	for _, tank := range req.Raid.Tanks {
		if tank != nil {
			followTarget(tank.ProtoReflect(), targetIndex)
		}
	}
	req.Raid.Parties = []*proto.Party{{Players: []*proto.Player{target}, Buffs: partyBuffs}}
	req.Raid.NumActiveParties = 1
	req.Raid.Buffs = got.raid
	req.Raid.Debuffs = got.debuffs
	req.Raid.TargetDummies = 0
	return req, nil
}

// activeParties counts the parties that sim, like core.NewRaid: 0 means all of them.
func activeParties(raid *proto.Raid) int {
	n := int(raid.NumActiveParties)
	if n == 0 || n > len(raid.Parties) {
		n = len(raid.Parties)
	}
	return n
}

func checkTarget(raid *proto.Raid, numParties, index int) error {
	if index < 0 || index/5 >= numParties {
		return fmt.Errorf("raid index %d is outside the raid's %d active parties", index, numParties)
	}
	players := raid.Parties[index/5].GetPlayers()
	if index%5 >= len(players) || players[index%5].GetClass() == proto.Class_ClassUnknown {
		return fmt.Errorf("raid index %d is an empty raid slot", index)
	}
	return nil
}

type raider struct {
	index  int
	player *proto.Player
}

// activePlayers lists the players in the parties that sim, by raid index.
func activePlayers(raid *proto.Raid, numParties int) []raider {
	var raiders []raider
	for partyIndex, party := range raid.Parties[:numParties] {
		for slot, player := range party.GetPlayers() {
			if player.GetClass() != proto.Class_ClassUnknown {
				raiders = append(raiders, raider{partyIndex*5 + slot, player})
			}
		}
	}
	return raiders
}

type pact struct {
	index  int
	points int32
}

// demonicPactSP is the strongest pact these warlocks give, from their sheet spell power in the full
// raid, target included. The sim's aura takes the warlock's spell power minus what the aura itself
// adds, so a Demonic Pact already in base's raid buffs comes off first.
func demonicPactSP(base *proto.RaidSimRequest, pacts []pact) int32 {
	encounter := base.Encounter
	if encounter == nil {
		encounter = &proto.Encounter{}
	}
	// NewEnvironment writes into both protos, like NewRaid
	env, raidStats, _ := core.NewEnvironment(
		goproto.Clone(base.Raid).(*proto.Raid),
		goproto.Clone(encounter).(*proto.Encounter),
		false)
	var best int32
	for _, p := range pacts {
		for i, party := range env.Raid.Parties {
			if party.Index != p.index/5 {
				continue
			}
			sheet := raidStats.Parties[i].Players[p.index%5].GetFinalStats().GetStats()
			spellPower := sheet[stats.SpellPower] - float64(base.Raid.Buffs.GetDemonicPactSp())
			best = max(best, DemonicPactSP(spellPower, p.points))
		}
	}
	return best
}

// followTarget rewrites the player references in m for the derived raid: ones naming the target
// point at TargetIndex, and anyone else's are cleared, since TargetIndex now means the target.
// Note: cleared isn't always nobody. An APL reads it as its default unit (self as a source, the
// current target for a cast), and Unholy Frenzy without a target goes on the DK itself. A pet
// reference follows its owner's.
func followTarget(m protoreflect.Message, from int) {
	if ref, ok := m.Interface().(*proto.UnitReference); ok && ref.Type == proto.UnitReference_Player {
		if int(ref.Index) != from {
			goproto.Reset(ref)
			return
		}
		ref.Index = TargetIndex
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			if fd.MapValue().Message() != nil {
				v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
					followTarget(mv.Message(), from)
					return true
				})
			}
		case fd.IsList():
			if fd.Message() != nil {
				for i := 0; i < v.List().Len(); i++ {
					followTarget(v.List().Get(i).Message(), from)
				}
			}
		case fd.Message() != nil:
			followTarget(v.Message(), from)
		}
		return true
	})
}
