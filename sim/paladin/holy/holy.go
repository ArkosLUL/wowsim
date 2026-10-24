package holy

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/paladin"
)

func RegisterHolyPaladin() {
	core.RegisterAgentFactory(
		proto.Player_HolyPaladin{},
		proto.Spec_SpecHolyPaladin,
		func(character *core.Character, options *proto.Player) core.Agent {
			return NewHolyPaladin(character, options)
		},
		func(player *proto.Player, spec interface{}) {
			playerSpec, ok := spec.(*proto.Player_HolyPaladin) // I don't really understand this line
			if !ok {
				panic("Invalid spec value for Holy Paladin!")
			}
			player.Spec = playerSpec
		},
	)
}

func NewHolyPaladin(character *core.Character, options *proto.Player) *HolyPaladin {
	holyOptions := options.GetHolyPaladin()

	holy := &HolyPaladin{
		Paladin: paladin.NewPaladin(character, options.TalentsString),
		Options: holyOptions.Options,
	}

	holy.PaladinAura = holyOptions.Options.Aura

	// no swings, but a seal's PPM reads the main hand's speed (Aura::CalcProcChance)
	holy.EnableAutoAttacks(holy, core.AutoAttackOptions{
		MainHand: holy.WeaponFromMainHand(0),
	})

	return holy
}

type HolyPaladin struct {
	*paladin.Paladin

	Options *proto.HolyPaladin_Options
}

func (holy *HolyPaladin) GetPaladin() *paladin.Paladin {
	return holy.Paladin
}

// a rotation's heal with no target goes to the first target dummy, or the paladin himself without one
func (holy *HolyPaladin) Initialize() {
	if dummy := holy.Env.Raid.GetFirstTargetDummy(); dummy != nil {
		holy.CurrentTarget = &dummy.Unit
	} else {
		holy.CurrentTarget = &holy.Unit
	}
	holy.Paladin.Initialize()
}

func (holy *HolyPaladin) Reset(sim *core.Simulation) {
	holy.Paladin.Reset(sim)
}
