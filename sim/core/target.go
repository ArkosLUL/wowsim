package core

import (
	"strconv"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type Encounter struct {
	Duration          time.Duration
	DurationVariation time.Duration
	Targets           []*Target
	TargetUnits       []*Unit

	ExecuteProportion_20 float64
	ExecuteProportion_25 float64
	ExecuteProportion_35 float64

	EndFightAtHealth float64
	// DamageTaken is used to track health fights instead of duration fights.
	//  Once primary target has taken its health worth of damage, fight ends.
	DamageTaken float64
	// In health fight: set to true until we get something to base on
	DurationIsEstimate bool

	// Value to multiply by, for damage spells which are subject to the aoe cap.
	aoeCapMultiplier float64
}

func NewEncounter(options *proto.Encounter) Encounter {
	options.ExecuteProportion_25 = max(options.ExecuteProportion_25, options.ExecuteProportion_20)
	options.ExecuteProportion_35 = max(options.ExecuteProportion_35, options.ExecuteProportion_25)

	encounter := Encounter{
		Duration:             DurationFromSeconds(options.Duration),
		DurationVariation:    DurationFromSeconds(options.DurationVariation),
		ExecuteProportion_20: max(options.ExecuteProportion_20, 0),
		ExecuteProportion_25: max(options.ExecuteProportion_25, 0),
		ExecuteProportion_35: max(options.ExecuteProportion_35, 0),
		Targets:              []*Target{},
	}
	// If UseHealth is set, we use the sum of targets health.
	if options.UseHealth {
		for _, t := range options.Targets {
			encounter.EndFightAtHealth += t.Stats[stats.Health]
		}
		if encounter.EndFightAtHealth == 0 {
			encounter.EndFightAtHealth = 1 // default to something so we don't instantly end without anything.
		}
	}

	for targetIndex, targetOptions := range options.Targets {
		target := NewTarget(targetOptions, int32(targetIndex))
		encounter.Targets = append(encounter.Targets, target)
		encounter.TargetUnits = append(encounter.TargetUnits, &target.Unit)
	}
	if len(encounter.Targets) == 0 {
		// Add a dummy target. The only case where targets aren't specified is when
		// computing character stats, and targets won't matter there.
		target := NewTarget(&proto.Target{}, 0)
		encounter.Targets = append(encounter.Targets, target)
		encounter.TargetUnits = append(encounter.TargetUnits, &target.Unit)
	}

	if encounter.EndFightAtHealth > 0 {
		// Until we pre-sim set duration to 10m
		encounter.Duration = time.Minute * 10
		encounter.DurationIsEstimate = true
	}

	encounter.updateAOECapMultiplier()

	return encounter
}

func (encounter *Encounter) AOECapMultiplier() float64 {
	return encounter.aoeCapMultiplier
}
func (encounter *Encounter) updateAOECapMultiplier() {
	encounter.aoeCapMultiplier = min(10/float64(len(encounter.Targets)), 1)
}

func (encounter *Encounter) doneIteration(sim *Simulation) {
	for i := range encounter.Targets {
		target := encounter.Targets[i]
		target.doneIteration(sim)
	}
}

func (encounter *Encounter) GetMetricsProto() *proto.EncounterMetrics {
	metrics := &proto.EncounterMetrics{
		Targets: make([]*proto.UnitMetrics, len(encounter.Targets)),
	}

	i := 0
	for _, target := range encounter.Targets {
		metrics.Targets[i] = target.GetMetricsProto()
		i++
	}

	return metrics
}

// Target is an enemy/boss that can be the target of player attacks/spells.
type Target struct {
	Unit

	AI TargetAI
}

func NewTarget(options *proto.Target, targetIndex int32) *Target {
	unitStats := stats.Stats{}
	if options.Stats != nil {
		copy(unitStats[:], options.Stats)
	}

	target := &Target{
		Unit: Unit{
			Type:        EnemyUnit,
			Index:       targetIndex,
			Label:       "Target " + strconv.Itoa(int(targetIndex)+1),
			Level:       options.Level,
			MobType:     options.MobType,
			auraTracker: newAuraTracker(),
			stats:       unitStats,
			PseudoStats: stats.NewPseudoStats(),
			Metrics:     NewUnitMetrics(),

			StatDependencyManager: stats.NewStatDependencyManager(),
		},
	}
	defaultRaidBossLevel := int32(CharacterLevel + 3)
	target.GCD = target.NewTimer()
	if target.Level == 0 {
		target.Level = defaultRaidBossLevel
	}
	// The boss flag is what gives a creature its 5.85/13.4 avoidance and puts it
	// three levels above whoever it fights. Left unset, anything raid level is one.
	if options.WorldBoss != nil {
		target.IsWorldBoss = *options.WorldBoss
	} else {
		target.IsWorldBoss = target.Level >= defaultRaidBossLevel
	}
	if target.stats[stats.MeleeCrit] == 0 {
		// Creatures crit at a flat 5%; the level difference is handled by the
		// skill term in the attack table.
		target.stats[stats.MeleeCrit] = 5.0 * CritRatingPerCritChance
	}
	if target.stats[stats.BlockValue] == 0 {
		target.stats[stats.BlockValue] = CreatureBlockValue(target.Level, target.stats[stats.Strength])
	}

	if target.Level == defaultRaidBossLevel && options.SuppressDodge {
		// ICC boss Dodge Suppression. -20% dodge only.
		target.PseudoStats.DodgeReduction += 0.2
	}

	target.PseudoStats.CanBlock = true
	target.PseudoStats.CanParry = true
	target.PseudoStats.ParryHaste = options.ParryHaste
	target.PseudoStats.InFrontOfTarget = true
	target.PseudoStats.DamageSpread = options.DamageSpread

	preset := GetPresetTargetWithID(options.Id)
	if preset != nil && preset.AI != nil {
		target.AI = preset.AI()
	}

	return target
}

func (target *Target) Reset(sim *Simulation) {
	target.Unit.reset(sim, nil)
	target.SetGCDTimer(sim, 0)
	if target.AI != nil {
		target.AI.Reset(sim)
	}
}

func (target *Target) NextTarget() *Target {
	nextIndex := target.Index + 1
	if nextIndex >= target.Env.GetNumTargets() {
		nextIndex = 0
	}
	return target.Env.GetTarget(nextIndex)
}

func (target *Target) GetMetricsProto() *proto.UnitMetrics {
	metrics := target.Metrics.ToProto()
	metrics.Name = target.Label
	metrics.UnitIndex = target.UnitIndex
	metrics.Auras = target.auraTracker.GetMetricsProto()
	return metrics
}

// Holds cached values for outcome/damage calculations, for a specific attacker+defender pair.
// These are updated dynamically when attacker or defender stats change.
type AttackTable struct {
	Attacker *Unit
	Defender *Unit

	// Max skill for each side's level as the other sees it, so 415 for a world
	// boss. Nothing raises weapon skill past it, so it's also the attacker's skill;
	// a player defender's defense rating goes on top at roll time.
	AttackerSkill    int32
	DefenderMaxSkill int32

	// Base chances as percentages, before the attacker's hit, expertise and crit.
	BaseMissPct  float32
	BaseDodgePct float32
	BaseParryPct float32
	BaseBlockPct float32

	// The block yellow hits roll separately from the table, against a creature.
	// A player's comes from their live block chance instead.
	PartialBlockBP int32

	GlanceMultiplier float64

	// (defender skill - attacker skill) * 0.04, so 0.6% against a level 83 boss.
	// Magic damage class spells get nothing, so there is no spell counterpart.
	MeleeCritSuppression float64

	// getLevelForTarget in each direction, which the boss flag moves and the
	// template level doesn't.
	AttackerLevel int32
	DefenderLevel int32

	// A non-boss creature only parries if it's humanoid.
	DefenderCanParry bool

	DamageDealtMultiplier        float64 // attacker buff, applied in applyAttackerModifiers()
	DamageTakenMultiplier        float64 // defender debuff, applied in applyTargetModifiers()
	NatureDamageTakenMultiplier  float64
	HauntSEDamageTakenMultiplier float64
	HealingDealtMultiplier       float64
}

func NewAttackTable(attacker *Unit, defender *Unit) *AttackTable {
	table := &AttackTable{
		Attacker: attacker,
		Defender: defender,

		DamageDealtMultiplier:        1,
		DamageTakenMultiplier:        1,
		NatureDamageTakenMultiplier:  1,
		HauntSEDamageTakenMultiplier: 1,
		HealingDealtMultiplier:       1,
	}

	table.AttackerLevel = LevelForTarget(attacker.Level, attacker.IsWorldBoss, defender.Level)
	table.DefenderLevel = LevelForTarget(defender.Level, defender.IsWorldBoss, attacker.Level)

	table.AttackerSkill = MaxSkill(table.AttackerLevel)
	table.DefenderMaxSkill = MaxSkill(table.DefenderLevel)

	table.BaseMissPct = 5
	table.DefenderCanParry = true

	if defender.Type == EnemyUnit {
		if defender.IsWorldBoss {
			table.BaseDodgePct = 5.85
			table.BaseParryPct = 13.4
		} else {
			table.BaseDodgePct = 5
			table.BaseParryPct = 5
			table.DefenderCanParry = defender.MobType == proto.MobType_MobTypeHumanoid
		}
		table.BaseBlockPct = 5
		table.PartialBlockBP = PartialBlockBP(table.BaseBlockPct, MaxSkill(attacker.Level), MaxSkill(defender.Level))
	}
	// A player defender's avoidance is all rating and talent driven, so those
	// percentages are read at roll time instead of cached here.

	table.MeleeCritSuppression = MeleeCritSuppressionPct(table.AttackerSkill, table.DefenderMaxSkill) / 100

	table.GlanceMultiplier = GlancingMultiplier(attacker.Level, defender.Level)

	return table
}

// isPlayerOrPet is Unit::IsPlayer() || IsPet(), the two that glance.
func (unit *Unit) isPlayerOrPet() bool {
	return unit.Type == PlayerUnit || unit.SummonedAsPet
}

// meleeTableInput fills in the pair's fixed half: levels, skills and the
// creature base chances. Callers add whatever moves during a fight.
func (at *AttackTable) meleeTableInput() MeleeTableInput {
	return MeleeTableInput{
		AttackerLevel:    at.AttackerLevel,
		AttackerSkill:    at.AttackerSkill,
		AttackerMaxSkill: at.AttackerSkill,
		DefenderLevel:    at.DefenderLevel,
		DefenderSkill:    at.DefenderMaxSkill,
		DefenderMaxSkill: at.DefenderMaxSkill,

		AttackerIsPlayerOrPet:      at.Attacker.isPlayerOrPet(),
		DefenderIsPlayerOrPet:      at.Defender.isPlayerOrPet(),
		AttackerControlledByPlayer: at.Attacker.Type != EnemyUnit,
		DefenderIsPlayer:           at.Defender.Type == PlayerUnit,

		MissPct:  at.BaseMissPct,
		DodgePct: at.BaseDodgePct,
		ParryPct: at.BaseParryPct,
		BlockPct: at.BaseBlockPct,

		CanDodge: true,
		CanParry: at.DefenderCanParry,
		CanBlock: true,

		// The attacker's "chance to be dodged" reduction: Weapon Mastery on a
		// player. On an ICC boss it stands in for Chill of the Throne, which is
		// really 20% off the tank's own dodge.
		DodgeReductionPct: float32(at.Attacker.PseudoStats.DodgeReduction * 100),

		EnemyDodgeMultiplier: 1,
	}
}
