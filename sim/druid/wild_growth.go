package druid

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

const wildGrowthTargets = 5

// wildGrowthTickDecay is the percent a tick's bonus drops by each tick after the first.
const wildGrowthTickDecay = 2

func (druid *Druid) registerWildGrowthSpell() {
	if !druid.Talents.WildGrowth {
		return
	}

	units := druid.Env.Raid.GetFirstNPlayersOrPets(math.MaxInt32)
	targets := make([]*core.Unit, 0, len(units))
	periodic := druid.periodicHealMultiplier()
	maxTargets := wildGrowthTargets + core.TernaryInt(druid.HasMajorGlyph(proto.DruidMajorGlyph_GlyphOfWildGrowth), int(serverBonus(spellGlyphOfWildGrowth, 0)), 0)
	t10Cut := druid.wildGrowthT10Cut()
	// the decay each target's Wild Growth has reached
	decays := make([]float32, len(druid.Env.AllUnits))

	druid.WildGrowth = druid.RegisterSpell(healForms, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 53251},
		SpellSchool: core.SpellSchoolNature,
		ProcMask:    core.ProcMaskSpellHealing,
		Flags:       core.SpellFlagHelpful | core.SpellFlagAPL | SpellFlagOmenTrigger,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.23,
			Multiplier: druid.hotCostMultiplier(),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Second * 6,
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: druid.healThreatMultiplier(),

		Mods: druid.empoweredRejuvenation(),
		Hot: core.DotConfig{
			Aura: core.Aura{
				Label: "Wild Growth",
			},
			NumberOfTicks: 7,
			TickLength:    time.Second,
			TicksCanCrit:  false,
			Tick:          core.SpellEffect{Effect: 0, Min: 206, Max: 206, SP: 0.115},
			// spell_dru_wild_growth_aura keeps the amount as the tick's base and rewrites it before each tick
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, _ bool) {
				dot.SnapshotBaseDamage = wholeHot(sim, target, dot, periodic, 0)
				dot.SnapshotHotMultiplier(1)
				decay := float32(wildGrowthTickDecay)
				if dot.IsActive() {
					decay = decays[target.UnitIndex]
				}
				decays[target.UnitIndex] = wildGrowthDecay(decay, t10Cut)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				base := dot.SnapshotBaseDamage
				dot.SnapshotBaseDamage = wildGrowthTick(base, dot.TickCount, decays[target.UnitIndex])
				dot.CalcAndDealPeriodicSnapshotHealing(sim, target, dot.OutcomeTick)
				dot.SnapshotBaseDamage = base
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			targets = wildGrowthPick(targets[:0], units, maxTargets)
			for _, target := range targets {
				spell.Hot(target).Apply(sim)
				druid.landHot(sim, spell, target)
			}
		},
	})
}

// wildGrowthTick is the script's tick n amount: 6% more on the first, decay% less each tick after, in float32.
func wildGrowthTick(base float64, tick int32, decay float32) float64 {
	b := float32(base)
	bonus := float32(6 - decay*float32(tick-1))
	return float64(int32(float32(b + float32(float32(b*bonus)/100))))
}

// wildGrowthPick is spell_dru_wild_growth's pick: past maxTargets living raid members and pets, the maxTargets with
// the lowest health percent, a stable sort so a tie keeps raid order. The area is 15 yd round the target, which
// is everyone with no positions in the sim.
func wildGrowthPick(targets []*core.Unit, units []*core.Unit, maxTargets int) []*core.Unit {
	for _, unit := range units {
		// a pet that's away isn't in the world, and SpellInfo::CheckTarget keeps the dead out
		if !unit.IsEnabled() || (unit.HasHealthBar() && unit.CurrentHealth() <= 0) {
			continue
		}
		targets = append(targets, unit)
	}
	if len(targets) > maxTargets {
		slices.SortStableFunc(targets, func(a, b *core.Unit) int {
			return cmp.Compare(healthPctOrder(a), healthPctOrder(b))
		})
		targets = targets[:maxTargets]
	}
	return targets
}

// healthPctOrder is Acore::HealthPctOrderPred's float ratio of whole health, full for a unit with no health bar.
func healthPctOrder(unit *core.Unit) float32 {
	if !unit.HasHealthBar() {
		return 1
	}
	cur, most := uint32(unit.CurrentHealth()), uint32(unit.MaxHealth())
	// health can sit above max after max health drops, where the server caps it
	return float32(min(cur, most)) / float32(most)
}
