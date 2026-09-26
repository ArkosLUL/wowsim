package core

import (
	"log"

	"github.com/wowsims/wotlk/sim/core/proto"
)

type TargetAI interface {
	Initialize(*Target, *proto.Target)
	Reset(*Simulation)
	ExecuteCustomRotation(*Simulation)
}

func (target *Target) initialize(config *proto.Target) {
	if config == nil {
		return
	}

	if target.CurrentTarget != nil {
		if config.SwingSpeed > 0 {
			aaOptions := AutoAttackOptions{
				MainHand: Weapon{
					BaseDamageMin:  config.MinBaseDamage,
					SwingSpeed:     config.SwingSpeed,
					CritMultiplier: 2,
					SpellSchool:    SpellSchoolFromProto(config.SpellSchool),
				},
				AutoSwingMelee: true,
			}
			if config.DualWield {
				aaOptions.OffHand = aaOptions.MainHand
				if !config.DualWieldPenalty {
					target.PseudoStats.DisableDWMissPenalty = true
				}
			}
			target.EnableAutoAttacks(target, aaOptions)
		}
	}

	if target.AI != nil {
		target.AI.Initialize(target, config)

		// applyDungeonScale folds the Damage multiplier into PseudoStats.DamageDealtMultiplier, which a
		// spell flagged SpellFlagIgnoreAttackerModifiers skips outright (spell_result.go). The server's
		// DungeonScale hooks (ModifySpellDamageTaken, ModifyMeleeDamage, ModifyPeriodicDamageAurasTick)
		// scale a creature's damage regardless of the spell's attributes, so an ability that ignores
		// attacker modifiers still needs the scale, folded into its own DamageMultiplier instead, which
		// nothing skips.
		if dungeonScale := target.PseudoStats.DamageDealtMultiplier; dungeonScale != 1 {
			for _, spell := range target.Spellbook {
				if spell.Flags.Matches(SpellFlagIgnoreAttackerModifiers) {
					spell.DamageMultiplier *= dungeonScale
				}
			}
		}

		target.gcdAction = &PendingAction{
			Priority: ActionPriorityGCD,
			OnAction: func(sim *Simulation) {
				// A hardcast whose Expires lands on the same tick as the GCD shares this same pending
				// action (SetGCDTimer), so it never gets its own hardcastAction: complete it here first,
				// as character.go's gcdAction does for players.
				if hc := &target.Hardcast; hc.Expires != startingCDTime && hc.Expires <= sim.CurrentTime {
					hc.Expires = startingCDTime
					if hc.OnComplete != nil {
						hc.OnComplete(sim, hc.Target)
					}
				}

				target.Rotation.DoNextAction(sim)
			},
		}
	}
}

// Empty Agent interface functions.
func (target *Target) AddRaidBuffs(_ *proto.RaidBuffs)   {}
func (target *Target) AddPartyBuffs(_ *proto.PartyBuffs) {}
func (target *Target) ApplyTalents()                     {}
func (target *Target) GetCharacter() *Character          { return nil }
func (target *Target) Initialize()                       {}

func (target *Target) ExecuteCustomRotation(sim *Simulation) {
	if target.AI != nil {
		target.AI.ExecuteCustomRotation(sim)
	}
}

type AIFactory func() TargetAI

type PresetTarget struct {
	// String in folder-structure format identifying a category for this unit, e.g. "Black Temple/Bosses".
	PathPrefix string

	Config *proto.Target

	AI AIFactory
}

func (pt PresetTarget) Path() string {
	return pt.PathPrefix + "/" + pt.Config.Name
}
func (pt PresetTarget) ToProto() *proto.PresetTarget {
	// CHECKME might need cloning
	return &proto.PresetTarget{
		Path:   pt.Path(),
		Target: pt.Config,
	}
}

var presetTargets []*PresetTarget
var PresetEncounters []*proto.PresetEncounter

func AddPresetTarget(newPreset *PresetTarget) {
	for _, preset := range presetTargets {
		if preset.Path() == newPreset.Path() {
			log.Fatalf("Preset Target with path %s already added!", newPreset.Path())
		}
	}
	presetTargets = append(presetTargets, newPreset)
}

func GetPresetTargetWithPath(path string) *PresetTarget {
	for _, preset := range presetTargets {
		if preset.Path() == path {
			return preset
		}
	}
	return nil
}

func GetPresetTargetWithID(id int32) *PresetTarget {
	for _, preset := range presetTargets {
		if preset.Config.Id == id {
			return preset
		}
	}
	return nil
}

func AddPresetEncounter(name string, targetPaths []string) {
	AddPresetEncounterWithDifficulty(name, targetPaths, proto.RaidDifficulty_RaidDifficultyUnknown)
}

// AddPresetEncounterWithDifficulty is AddPresetEncounter for a preset that also picks a raid
// difficulty, e.g. one built for a specific raid size so its dungeon scale applies without the player
// setting it by hand.
func AddPresetEncounterWithDifficulty(name string, targetPaths []string, difficulty proto.RaidDifficulty) {
	if len(targetPaths) == 0 {
		log.Fatalf("Encounter must have targets!")
	}

	var path string
	targetProtos := make([]*proto.PresetTarget, len(targetPaths))

	for i, targetPath := range targetPaths {
		presetTarget := GetPresetTargetWithPath(targetPath)
		if presetTarget == nil {
			log.Fatalf("No preset target with path: %s", targetPath)
		}
		targetProtos[i] = presetTarget.ToProto()

		if i == 0 {
			path = presetTarget.PathPrefix + "/" + name
		}
	}

	for _, preset := range PresetEncounters {
		if preset.Path == path {
			log.Fatalf("Preset Encounter with path %s already added!", path)
		}
	}

	PresetEncounters = append(PresetEncounters, &proto.PresetEncounter{
		Path:           path,
		Targets:        targetProtos,
		RaidDifficulty: difficulty,
	})
}
