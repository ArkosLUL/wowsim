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

// OptimizerTankAbility is a boss spell that damages its current tank and that armor and avoidance
// can't remove: it always hits, so only the school's resistance and damage taken multipliers touch it.
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
	// Seconds added to the first cast, to offset a paired ability (Soul Reaper's delayed tick).
	StartSeconds float64
}

// OptimizerTankBoss is one phase boss as the optimizer's tank runs fight it: the swinging generic
// boss at the phase boss's own numbers, plus the abilities of its fight that mitigation can't touch.
type OptimizerTankBoss struct {
	Name string
	// creature_template.DamageModifier and BaseAttackTime (0 falls back to the server's 2000 ms).
	DamageModifier float64
	AttackTimeMs   int32
	// creature_classlevelstats damage_exp2 and attackpower of the boss's class row; 0 means the class 1
	// row GenericBossTarget carries.
	DamageExp2  float64
	AttackPower float64
	DualWield   bool
	// ICC bosses' Chill of the Throne.
	SuppressDodge bool
	Abilities     []OptimizerTankAbility
}

// Class 1, level 83 creature_classlevelstats.
const (
	genericDamageExp2  = 177.074
	genericAttackPower = 805
)

// The labels of a TargetInput group, in order; abilities travel as consecutive groups.
var optimizerTankAbilityLabels = [...]string{"Spell id", "School", "Damage", "Period min", "Period max", "Cast time", "Land chance", "Start"}

func (a OptimizerTankAbility) inputs() []*proto.TargetInput {
	values := [...]float64{float64(a.SpellID), float64(a.School), a.Damage, a.PeriodMin, a.PeriodMax, a.CastSeconds, a.LandChance, a.StartSeconds}
	out := make([]*proto.TargetInput, len(values))
	for i, v := range values {
		out[i] = &proto.TargetInput{InputType: proto.InputType_Number, Label: optimizerTankAbilityLabels[i], NumberValue: v}
	}
	return out
}

// OptimizerTankBossTarget is GenericBossTargetAt for a phase boss, with its stat row, off hand and
// abilities. Creature::CalculateMinMaxDamage scales the weapon by DamageModifier and the attack time
// in seconds, so a faster boss swings smaller, not less per second.
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
	// the server's off hand takes the dual wield miss penalty on both hands
	target.DualWieldPenalty = boss.DualWield
	target.SuppressDodge = boss.SuppressDodge
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
	nextAt time.Duration
}

type optimizerTankBossAI struct {
	target *core.Target
	spells []optimizerTankSpell
	armed  bool
}

func NewOptimizerTankBossAI() core.TargetAI {
	return &optimizerTankBossAI{}
}

func abilitiesFromInputs(inputs []*proto.TargetInput) []OptimizerTankAbility {
	group := len(optimizerTankAbilityLabels)
	if len(inputs)%group != 0 {
		panic(fmt.Sprintf("optimizer tank boss: %d target inputs, want a multiple of %d", len(inputs), group))
	}
	var out []OptimizerTankAbility
	for i := 0; i < len(inputs); i += group {
		for j, label := range optimizerTankAbilityLabels {
			if inputs[i+j].Label != label {
				panic(fmt.Sprintf("optimizer tank boss: input %d is %q, want %q", i+j, inputs[i+j].Label, label))
			}
		}
		v := func(j int) float64 { return inputs[i+j].NumberValue }
		out = append(out, OptimizerTankAbility{
			SpellID: int32(v(0)), School: proto.SpellSchool(v(1)), Damage: v(2),
			PeriodMin: v(3), PeriodMax: v(4), CastSeconds: v(5), LandChance: v(6), StartSeconds: v(7),
		})
	}
	return out
}

func (ai *optimizerTankBossAI) Initialize(target *core.Target, config *proto.Target) {
	ai.target = target
	for i, ability := range abilitiesFromInputs(config.TargetInputs) {
		ai.spells = append(ai.spells, optimizerTankSpell{OptimizerTankAbility: ability, spell: ai.register(target, ability, int32(i))})
	}
}

func (ai *optimizerTankBossAI) register(target *core.Target, ability OptimizerTankAbility, index int32) *core.Spell {
	return target.RegisterSpell(core.SpellConfig{
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
			spell.CalcAndDealDamage(sim, tank, ability.Damage, spell.OutcomeAlwaysHit)
		},
	})
}

func (ai *optimizerTankBossAI) Reset(*core.Simulation) {
	ai.armed = false
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
			landed := s.LandChance == 0 || s.LandChance >= 1 || sim.RandomFloat("Optimizer Tank Boss Interrupt") < s.LandChance
			if landed {
				s.spell.Cast(sim, ai.target.CurrentTarget)
				if ai.target.Hardcast.Expires > sim.CurrentTime {
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
