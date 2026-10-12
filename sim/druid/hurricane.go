package druid

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// HurricaneActionID is the channel, and the aura it keeps on every creature in the storm.
var HurricaneActionID = core.ActionID{SpellID: 48467}

// UPDATE_TARGET_MAP_INTERVAL: how often an aura looks again at who it reaches.
const stormTargetInterval = 500 * time.Millisecond

// nextStormPass is when a storm whose next map update comes at first looks at its targets again: the interval
// counts down on its map updates, and the pass runs on the first one with no more than its own length left.
func nextStormPass(sim *core.Simulation, first time.Duration) time.Duration {
	step := sim.NextServerTick(first+1) - first
	if step <= 1 {
		// exact timing
		return sim.CurrentTime + stormTargetInterval
	}
	return first + (stormTargetInterval+step-1)/step*step - step
}

func (druid *Druid) registerHurricaneSpell() {
	// The second effect slows the attacks of every creature in the storm by 20%, a boss's too. On rank 5 it
	// has no mechanic (ranks 1 to 4: MECHANIC_SLOW_ATTACK), so nothing is immune to it and Torment the Weak
	// doesn't count it. The first effect is a snare, of 0% without the glyph, where snares land.
	slows := druid.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		tag := core.SnareAuraTag
		if target.ImmuneToMechanic(core.MechanicSnare) {
			tag = ""
		}
		aura := target.GetOrRegisterAura(core.Aura{
			Label:    "Hurricane by " + druid.Label,
			ActionID: HurricaneActionID,
			Tag:      tag,
			Duration: core.NeverExpires,
		})
		core.AtkSpeedReductionEffect(aura, 1.2)
		return aura
	})

	// A target immune to the snare holds the aura only half the time. Aura::UpdateTargetMap compares the
	// effects a target has on with the storm's own, immune ones included, takes a mismatch off and puts it
	// back on its next pass. The first pass is the cast's, so the aura starts on.
	var flicker *core.PendingAction
	startFlicker := func(sim *core.Simulation) {
		var partial []*core.Aura
		for _, target := range sim.Encounter.TargetUnits {
			if target.ImmuneToMechanic(core.MechanicSnare) {
				partial = append(partial, slows.Get(target))
			}
		}
		if len(partial) == 0 {
			return
		}
		on := true
		pass := &core.PendingAction{
			NextActionAt: nextStormPass(sim, sim.NextServerTick(sim.CurrentTime)),
			Priority:     core.ActionPriorityDOT,
		}
		pass.OnAction = func(sim *core.Simulation) {
			on = !on
			for _, aura := range partial {
				if on {
					aura.Activate(sim)
				} else {
					aura.Deactivate(sim)
				}
			}
			pass.NextActionAt = nextStormPass(sim, sim.NextServerTick(sim.CurrentTime+1))
			sim.AddPendingAction(pass)
		}
		sim.AddPendingAction(pass)
		flicker = pass
	}

	druid.HurricaneTickSpell = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 48466},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskProc,
		Flags:       SpellFlagOmenTrigger,
		// critCapable in the spelldump, so a crit pays the full spell multiplier, not 1x
		CritMultiplier: druid.DefaultSpellCritMultiplier(),
		DamageMultiplier: spellModDamage(
			0.15*float64(druid.Talents.GaleWinds),
			0.01*float64(druid.Talents.Genesis),
		),
		ThreatMultiplier: 1,
		Direct:           core.SpellEffect{Effect: 0, Min: 451, Max: 451, SP: 0.12898},
		Mods: []core.SpellMod{
			// Glyph of Hurricane's -20 is meant for the slow, but its class mask takes this tick's damage too
			{Op: core.SpellModEffect1, Flat: core.TernaryInt32(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfHurricane), -20, 0)},
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			damage := spell.Direct.Roll(sim) + core.PowerBonus(spell.Direct.SP, spell.SpellPower())
			spell.AoECapUnits = len(sim.Encounter.Targets)
			for _, aoeTarget := range sim.Encounter.TargetUnits {
				spell.CalcAndDealDamage(sim, aoeTarget, damage, spell.OutcomeMagicHitAndCrit)
			}
		},
	})

	druid.Hurricane = druid.RegisterSpell(Humanoid|Moonkin, core.SpellConfig{
		ActionID:    HurricaneActionID,
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagChanneled | core.SpellFlagAPL | SpellFlagOmenTrigger,
		ManaCost: core.ManaCostOptions{
			BaseCost:   0.81,
			Multiplier: 1,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},
		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "Hurricane",
				OnGain: func(_ *core.Aura, sim *core.Simulation) {
					for _, target := range sim.Encounter.TargetUnits {
						slows.Get(target).Activate(sim)
					}
					startFlicker(sim)
				},
				OnExpire: func(_ *core.Aura, sim *core.Simulation) {
					if flicker != nil {
						flicker.Cancel(sim)
						flicker = nil
					}
					for _, target := range sim.Encounter.TargetUnits {
						slows.Get(target).Deactivate(sim)
					}
				},
			},
			NumberOfTicks:       10,
			TickLength:          time.Second * 1,
			TicksCanCrit:        false,
			AffectedByCastSpeed: true,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				druid.HurricaneTickSpell.Cast(sim, target)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			// the cast puts an aura on the druid: a hit with no damage, which rolls Omen of Clarity and gives an
			// Illustration of the Dragon Soul stack before any tick lands. Only the druid's procs see it. It
			// names the cast's target, since a proc that reaches for the hit's target expects an enemy.
			result := spell.NewResult(target)
			result.Outcome = core.OutcomeHit
			druid.OnSpellHitDealt(sim, spell, result)
			spell.DisposeResult(result)
			spell.AOEDot().Apply(sim)
		},
	})
}
