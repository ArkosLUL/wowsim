package core

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type TargetDummy struct {
	Character

	// takesDamage is set when the raid has incoming damage: the dummy then has a health bar and heals on it
	// count only up to its missing health.
	takesDamage bool
}

func NewTargetDummy(dummyIndex int, party *Party, partyIndex int, maxHealth float64) *TargetDummy {
	name := fmt.Sprintf("Target Dummy %d", dummyIndex+1)
	td := &TargetDummy{
		Character: Character{
			Unit: Unit{
				Type:        PlayerUnit,
				Index:       int32(party.Index*5 + partyIndex),
				Level:       CharacterLevel,
				PseudoStats: newPseudoStats(),
				auraTracker: newAuraTracker(),
				Metrics:     NewUnitMetrics(),

				StatDependencyManager: stats.NewStatDependencyManager(),
			},
			Name:       name,
			Party:      party,
			PartyIndex: partyIndex,
			baseStats: stats.Stats{
				stats.Health: maxHealth,
			},
		},
	}

	td.Label = fmt.Sprintf("%s (#%d)", td.Name, td.Index+1)
	td.GCD = td.NewTimer()

	return td
}

func (td *TargetDummy) GetCharacter() *Character {
	return &td.Character
}
func (td *TargetDummy) AddRaidBuffs(raidBuffs *proto.RaidBuffs)    {}
func (td *TargetDummy) AddPartyBuffs(partyBuffs *proto.PartyBuffs) {}
func (td *TargetDummy) ApplyTalents()                              {}
func (td *TargetDummy) Initialize()                                {}
func (td *TargetDummy) Reset(sim *Simulation)                      {}
func (td *TargetDummy) ExecuteCustomRotation(sim *Simulation)      {}
