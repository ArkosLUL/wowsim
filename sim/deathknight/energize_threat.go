package deathknight

import (
	"github.com/wowsims/wotlk/sim/core"
)

// applyRunicPowerProcThreat makes the threat of Chill of the Grave's and Dirge's energizes (50480,
// 51206), which proc on each landed strike in their class masks. The sim books their runic power with
// the strike's own, which makes none.
func (dk *Deathknight) applyRunicPowerProcThreat() {
	cotg := 2.5 * float64(dk.Talents.ChillOfTheGrave)
	dirge := 2.5 * float64(dk.Talents.Dirge)
	if cotg == 0 && dirge == 0 {
		return
	}
	cotgMetrics := dk.NewRunicPowerMetrics(core.ActionID{SpellID: 50480}).EnergizeThreat(core.EnergizeBySpell)
	dirgeMetrics := dk.NewRunicPowerMetrics(core.ActionID{SpellID: 51206}).EnergizeThreat(core.EnergizeBySpell)

	core.MakePermanent(dk.RegisterAura(core.Aura{
		Label: "Runic Power Proc Threat",
		OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() {
				return
			}
			switch spell {
			case dk.IcyTouch, dk.HowlingBlast:
				if cotg > 0 {
					dk.AddEnergizeThreat(sim, cotgMetrics, cotg)
				}
			case dk.ObliterateMhHit:
				if cotg > 0 {
					dk.AddEnergizeThreat(sim, cotgMetrics, cotg)
				}
				if dirge > 0 {
					dk.AddEnergizeThreat(sim, dirgeMetrics, dirge)
				}
			case dk.DeathStrikeMhHit, dk.PlagueStrikeMhHit, dk.ScourgeStrike:
				if dirge > 0 {
					dk.AddEnergizeThreat(sim, dirgeMetrics, dirge)
				}
			}
		},
	}))
}
