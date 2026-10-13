package encounters

import (
	"fmt"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// OptimizerTankBossID is a synthetic id like GenericBossID, so the gear optimizer's tank runs get an AI
// attached. Only the optimizer sends it; the tank suites keep GenericBossTarget.
const OptimizerTankBossID = 999998

// OptimizerTankAbility is a boss spell that damages its current tank and that avoidance can't remove.
// It lands unless CanMiss says it rolls the server's magic hit. The school's resistance and damage
// taken multipliers still cut it, and armor cuts a Physical one.
type OptimizerTankAbility struct {
	SpellID int32
	School  proto.SpellSchool
	// Damage per cast, before the tank's modifiers.
	Damage float64
	// Seconds between casts, rolled uniformly in [PeriodMin, PeriodMax]. The first one rolls the same.
	PeriodMin, PeriodMax float64
	// Seconds the cast holds the boss's melee. 0 is an instant.
	CastSeconds float64
	// Share of the casts that land; players interrupt the rest before they hold any melee. 0 means 1.
	LandChance float64
	// Seconds added to the first cast.
	StartSeconds float64
	// The boss's swing timer restarts when the cast goes off: an untriggered spell with
	// SPELL_INTERRUPT_FLAG_INTERRUPT (Spell::IsAutoActionResetSpell).
	ResetsSwing bool
	// The cast leaves a periodic aura on the tank that ticks once, FollowUpSeconds after it lands, for
	// FollowUpDamage of the same school. 0 seconds means no aura.
	FollowUpSeconds, FollowUpDamage float64
	// As that tick fires the boss swings FollowUpHaste times as fast for FollowUpHasteSeconds (Soul
	// Reaper's 69410). 0 means no haste.
	FollowUpHaste, FollowUpHasteSeconds float64
	// The spell goes through MagicSpellHitResult, where a level 83 creature misses a level 80 player 1% of
	// the time: DmgClass MAGIC, or a damage-only DmgClass NONE with no spell family. A spell with a
	// non-damage effect, or SPELL_ATTR3_ALWAYS_HIT, never misses.
	CanMiss bool
	// The other tank taunts right after this cast (instant casts only), so the boss leaves the modelled
	// tank until the ability's next cast. That one lands on the other tank, who has just taunted
	// back: the modelled tank holds the boss every other interval, and the haste of a cast it
	// didn't take falls on it.
	SwapsTank bool
}

// OptimizerTankBoss is one phase boss as the optimizer's tank runs fight it: the swinging generic
// boss at the phase boss's own numbers, plus the abilities of its fight that avoidance can't touch.
type OptimizerTankBoss struct {
	Name string
	// creature_template.DamageModifier and BaseAttackTime (0 falls back to the server's 2000 ms).
	DamageModifier float64
	AttackTimeMs   int32
	// creature_classlevelstats damage_exp2 and attackpower of the boss's class row; 0 means the class 1
	// row GenericBossTarget carries.
	DamageExp2  float64
	AttackPower float64
	// An off hand from Dual Wield (42459), whose 20% hit takes the 19% dual wield miss back off.
	DualWield bool
	// creature_template.flags_extra has CREATURE_FLAG_EXTRA_NO_PARRY_HASTEN.
	NoParryHaste bool
	// ICC bosses' Chill of the Throne.
	SuppressDodge bool
	// SPELL_AURA_MOD_HIT_CHANCE on the boss, in points: its swings miss that much less.
	HitChance float64
	// SPELL_AURA_MOD_ENEMY_DODGE on the boss: the tank's dodge against it times this. 0 means 1.
	EnemyDodgeMultiplier float64
	Abilities            []OptimizerTankAbility
}

// Class 1, level 83 creature_classlevelstats.
const (
	genericDamageExp2  = 177.074
	genericAttackPower = 805
)

// The target inputs' labels: the boss's own first, then each ability's, starting at its spell id.
const (
	hitChanceLabel = "Hit chance"
	spellIDLabel   = "Spell id"
)

// fields is every other input of an ability, by label.
func (a *OptimizerTankAbility) fields() []struct {
	label string
	value *float64
} {
	return []struct {
		label string
		value *float64
	}{
		{"Damage", &a.Damage}, {"Period min", &a.PeriodMin}, {"Period max", &a.PeriodMax},
		{"Cast time", &a.CastSeconds}, {"Land chance", &a.LandChance}, {"Start", &a.StartSeconds},
		{"Follow-up delay", &a.FollowUpSeconds}, {"Follow-up damage", &a.FollowUpDamage},
		{"Follow-up haste", &a.FollowUpHaste}, {"Follow-up haste time", &a.FollowUpHasteSeconds},
	}
}

func numberInput(label string, value float64) *proto.TargetInput {
	return &proto.TargetInput{InputType: proto.InputType_Number, Label: label, NumberValue: value}
}

func flag(on bool) float64 {
	if on {
		return 1
	}
	return 0
}

func (a OptimizerTankAbility) inputs() []*proto.TargetInput {
	out := []*proto.TargetInput{
		numberInput(spellIDLabel, float64(a.SpellID)), numberInput("School", float64(a.School)),
		numberInput("Resets swing", flag(a.ResetsSwing)), numberInput("Can miss", flag(a.CanMiss)), numberInput("Swaps tank", flag(a.SwapsTank)),
	}
	for _, field := range a.fields() {
		out = append(out, numberInput(field.label, *field.value))
	}
	return out
}

// OptimizerTankBossTarget is GenericBossTargetAt for a phase boss, with its stat row, off hand, auras
// and abilities. Creature::CalculateMinMaxDamage scales the weapon by DamageModifier and the attack
// time in seconds, so a faster boss swings smaller, not less per second.
func OptimizerTankBossTarget(boss OptimizerTankBoss) *proto.Target {
	attackTimeMs := boss.AttackTimeMs
	if attackTimeMs <= 0 {
		attackTimeMs = baseAttackTimeMs
	}
	exp2, attackPower := boss.DamageExp2, boss.AttackPower
	if exp2 == 0 {
		exp2 = genericDamageExp2
	}
	if attackPower == 0 {
		attackPower = genericAttackPower
	}
	target := GenericBossTarget(GenericBossDamageModifier)
	target.Id = OptimizerTankBossID
	target.Name = boss.Name
	target.Stats[stats.AttackPower] = attackPower
	target.SwingSpeed = float64(attackTimeMs) / 1000
	target.MinBaseDamage = exp2 * boss.DamageModifier * target.SwingSpeed
	target.DualWield = boss.DualWield
	target.ParryHaste = !boss.NoParryHaste
	target.SuppressDodge = boss.SuppressDodge
	target.EnemyDodgeMultiplier = boss.EnemyDodgeMultiplier
	target.TargetInputs = []*proto.TargetInput{numberInput(hitChanceLabel, boss.HitChance)}
	for _, ability := range boss.Abilities {
		target.TargetInputs = append(target.TargetInputs, ability.inputs()...)
	}
	return target
}

func RegisterOptimizerTankBoss() {
	config := OptimizerTankBossTarget(OptimizerTankBoss{Name: "Optimizer Tank Boss", DamageModifier: GenericBossDamageModifier})
	core.AddPresetTarget(&core.PresetTarget{
		PathPrefix: "Generic",
		Config:     config,
		AI:         NewOptimizerTankBossAI,
	})
}

type optimizerTankSpell struct {
	OptimizerTankAbility
	spell  *core.Spell
	haste  *core.Aura
	nextAt time.Duration
}

type optimizerTankBossAI struct {
	target *core.Target
	spells []optimizerTankSpell
	armed  bool
	// The other tank holds the boss: it neither swings at nor casts on the modelled tank.
	away bool
}

func NewOptimizerTankBossAI() core.TargetAI {
	return &optimizerTankBossAI{}
}

// bossFromInputs reads back what OptimizerTankBossTarget put in the target inputs: the boss's hit
// chance and its abilities. It goes by label, so a request saved before an input existed still reads,
// with that field at 0.
func bossFromInputs(inputs []*proto.TargetInput) (hitChance float64, abilities []OptimizerTankAbility) {
	for i, input := range inputs {
		switch {
		case input.Label == hitChanceLabel:
			hitChance = input.NumberValue
			continue
		case input.Label == spellIDLabel:
			abilities = append(abilities, OptimizerTankAbility{SpellID: int32(input.NumberValue)})
			continue
		case len(abilities) == 0:
			panic(fmt.Sprintf("optimizer tank boss: input %d is %q, ahead of any ability's %q", i, input.Label, spellIDLabel))
		}
		ability := &abilities[len(abilities)-1]
		known := true
		switch input.Label {
		case "School":
			ability.School = proto.SpellSchool(input.NumberValue)
		case "Resets swing":
			ability.ResetsSwing = input.NumberValue != 0
		case "Can miss":
			ability.CanMiss = input.NumberValue != 0
		case "Swaps tank":
			ability.SwapsTank = input.NumberValue != 0
		default:
			known = false
			for _, field := range ability.fields() {
				if field.label == input.Label {
					*field.value, known = input.NumberValue, true
				}
			}
		}
		if !known {
			panic(fmt.Sprintf("optimizer tank boss: input %d has the unknown label %q", i, input.Label))
		}
	}
	return hitChance, abilities
}

func (ai *optimizerTankBossAI) Initialize(target *core.Target, config *proto.Target) {
	ai.target = target
	hitChance, abilities := bossFromInputs(config.TargetInputs)
	// core gives an enemy's swing no hit of its own, so the aura's points come off its miss chance,
	// which is where Unit::MeleeSpellMissChance takes them off too
	target.PseudoStats.IncreasedMissChance -= hitChance / 100
	for i, ability := range abilities {
		if ability.SwapsTank && ability.CastSeconds > 0 {
			panic(fmt.Sprintf("optimizer tank boss: spell %d swaps the tank and has a cast time", ability.SpellID))
		}
		spell, haste := ai.register(target, ability, int32(i))
		ai.spells = append(ai.spells, optimizerTankSpell{OptimizerTankAbility: ability, spell: spell, haste: haste})
	}
}

func (ai *optimizerTankBossAI) register(target *core.Target, ability OptimizerTankAbility, index int32) (*core.Spell, *core.Aura) {
	config := core.SpellConfig{
		ActionID:    core.ActionID{SpellID: ability.SpellID, Tag: index},
		SpellSchool: core.SpellSchoolFromProto(ability.School),
		ProcMask:    core.ProcMaskSpellDamage,
		Cast: core.CastConfig{
			DefaultCast: core.Cast{CastTime: core.DurationFromSeconds(ability.CastSeconds)},
		},
		DamageMultiplier: 1,
		// creatures never crit with spells
		CritMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, tank *core.Unit, spell *core.Spell) {
			outcome := spell.OutcomeAlwaysHit
			if ability.CanMiss {
				outcome = spell.OutcomeMagicHit
			}
			result := spell.CalcAndDealDamage(sim, tank, ability.Damage, outcome)
			if ability.FollowUpSeconds > 0 && result.Landed() {
				spell.Dot(tank).Apply(sim)
			}
		},
	}
	var haste *core.Aura
	if ability.FollowUpSeconds > 0 {
		if ability.FollowUpHaste > 0 && ability.FollowUpHasteSeconds > 0 {
			haste = target.RegisterAura(core.Aura{
				Label:    fmt.Sprintf("Optimizer Tank Boss Haste %d", index),
				Duration: core.DurationFromSeconds(ability.FollowUpHasteSeconds),
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					aura.Unit.MultiplyMeleeSpeed(sim, ability.FollowUpHaste)
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					aura.Unit.MultiplyMeleeSpeed(sim, 1/ability.FollowUpHaste)
				},
			})
		}
		config.Dot = core.DotConfig{
			Aura:          core.Aura{Label: fmt.Sprintf("Optimizer Tank Boss Follow-up %d", index)},
			NumberOfTicks: 1,
			TicksCanCrit:  false,
			TickLength:    core.DurationFromSeconds(ability.FollowUpSeconds),
			OnTick: func(sim *core.Simulation, tank *core.Unit, dot *core.Dot) {
				dot.Spell.CalcAndDealPeriodicDamage(sim, tank, ability.FollowUpDamage, dot.Spell.OutcomeAlwaysHit)
				if haste != nil {
					haste.Activate(sim)
				}
			},
		}
	}
	return target.RegisterSpell(config), haste
}

func (ai *optimizerTankBossAI) Reset(*core.Simulation) {
	ai.armed = false
	ai.away = false
}

// handOff is the other tank's taunt after a cast on the modelled tank. The follow-up already on it
// still ticks.
func (ai *optimizerTankBossAI) handOff(sim *core.Simulation) {
	ai.away = true
	ai.target.AutoAttacks.CancelAutoSwing(sim)
}

// takeBack is the cast that lands on the other tank, who has just taunted back to the modelled one.
// Its haste starts all the same, on the swings the modelled tank now takes.
func (ai *optimizerTankBossAI) takeBack(sim *core.Simulation, s *optimizerTankSpell) {
	ai.away = false
	ai.target.AutoAttacks.EnableAutoSwing(sim)
	if s.ResetsSwing {
		ai.target.AutoAttacks.StopMeleeUntil(sim, sim.CurrentTime, false)
	}
	if s.haste != nil {
		haste := s.haste
		core.StartDelayedAction(sim, core.DelayedActionOptions{
			DoAt:     sim.CurrentTime + core.DurationFromSeconds(s.FollowUpSeconds),
			OnAction: func(sim *core.Simulation) { haste.Activate(sim) },
		})
	}
}

func (s *optimizerTankSpell) gap(sim *core.Simulation) time.Duration {
	seconds := s.PeriodMin + (s.PeriodMax-s.PeriodMin)*sim.RandomFloat("Optimizer Tank Boss Cadence")
	return core.DurationFromSeconds(seconds)
}

func (ai *optimizerTankBossAI) ExecuteCustomRotation(sim *core.Simulation) {
	if !ai.target.GCD.IsReady(sim) || ai.target.CurrentTarget == nil || len(ai.spells) == 0 {
		return
	}
	if !ai.armed {
		ai.armed = true
		for i := range ai.spells {
			ai.spells[i].nextAt = sim.CurrentTime + core.DurationFromSeconds(ai.spells[i].StartSeconds) + ai.spells[i].gap(sim)
		}
	}

	next := core.NeverExpires
	for i := range ai.spells {
		s := &ai.spells[i]
		if sim.CurrentTime >= s.nextAt {
			s.nextAt = sim.CurrentTime + max(s.gap(sim), time.Millisecond)
			if ai.away {
				if s.SwapsTank {
					ai.takeBack(sim, s)
				}
				next = min(next, s.nextAt)
				continue
			}
			landed := s.LandChance == 0 || s.LandChance >= 1 || sim.RandomFloat("Optimizer Tank Boss Interrupt") < s.LandChance
			if landed && s.spell.Cast(sim, ai.target.CurrentTarget) {
				castEnd := max(ai.target.Hardcast.Expires, sim.CurrentTime)
				if s.ResetsSwing {
					readyAt := castEnd
					if castEnd > sim.CurrentTime {
						// a cast with a cast time goes off in the boss's event pass, ahead of that update's
						// attack timers, so the restarted swing has an update off it already
						readyAt -= castEnd - sim.ServerTickBefore(castEnd)
					}
					ai.target.AutoAttacks.StopMeleeUntil(sim, readyAt, false)
				}
				if s.SwapsTank {
					ai.handOff(sim)
				}
				if castEnd > sim.CurrentTime {
					// the cast holds the boss; this runs again when it lands
					return
				}
			}
		}
		next = min(next, s.nextAt)
	}
	if next > sim.CurrentTime {
		ai.target.WaitUntil(sim, next)
	}
}
