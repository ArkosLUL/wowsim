package rogue

import (
	"github.com/wowsims/wotlk/sim/core"
)

func (rogue *Rogue) registerHackAndSlash(mask core.ProcMask) {
	// https://wotlk.wowhead.com/spell=13964/sword-specialization, proc mask = 20.
	var hackAndSlashSpell *core.Spell
	// spell_proc's -13960 has no cooldown and keeps each rank's own chance
	procChance := 0.01 * float64(rogue.Talents.HackAndSlash)

	rogue.RegisterAura(core.Aura{
		Label:    "Hack and Slash",
		Duration: core.NeverExpires,
		OnInit: func(aura *core.Aura, sim *core.Simulation) {
			config := *rogue.AutoAttacks.MHConfig()
			config.ActionID = core.ActionID{SpellID: 13964}
			hackAndSlashSpell = rogue.GetOrRegisterSpell(config)
		},
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}
			if !spell.ProcMask.Matches(mask) {
				return
			}
			if !rogue.AutoAttacks.CanProcExtraAttack(hackAndSlashSpell) {
				return
			}
			if sim.RandomFloat("Hack and Slash") < procChance {
				// 66923 ends the chain, as Sword Specialization's does
				rogue.AutoAttacks.QueueExtraAttack(sim, hackAndSlashSpell, result.Target, true)
			}
		},
	})
}
