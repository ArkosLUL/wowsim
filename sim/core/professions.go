package core

import (
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// ProtoToProfessions reads the player's professions, falling back to the two slots for links and
// presets saved before the list existed. A character can know more than two on an AzerothCore server.
func ProtoToProfessions(player *proto.Player) []proto.Profession {
	known := player.Professions
	if len(known) == 0 {
		known = []proto.Profession{player.Profession1, player.Profession2}
	}

	professions := make([]proto.Profession, 0, len(known))
	for _, profession := range known {
		if profession != proto.Profession_ProfessionUnknown && !slices.Contains(professions, profession) {
			professions = append(professions, profession)
		}
	}
	return professions
}

// This is just the static bonuses. Most professions are handled elsewhere.
func (character *Character) applyProfessionEffects() {
	if character.HasProfession(proto.Profession_Mining) {
		character.AddStat(stats.Stamina, 60)
	}

	if character.HasProfession(proto.Profession_Skinning) {
		character.AddStats(stats.Stats{stats.MeleeCrit: 40, stats.SpellCrit: 40})
	}

	if character.HasProfession(proto.Profession_Herbalism) {
		actionID := ActionID{SpellID: 55503}
		healthMetrics := character.NewHealthMetrics(actionID)

		spell := character.RegisterSpell(SpellConfig{
			ActionID:    actionID,
			SpellSchool: SpellSchoolNature,
			Cast: CastConfig{
				CD: Cooldown{
					Timer:    character.NewTimer(),
					Duration: time.Minute * 3,
				},
			},
			ApplyEffects: func(sim *Simulation, _ *Unit, _ *Spell) {
				amount := (3600 + character.MaxHealth()*0.016) / 5
				StartPeriodicAction(sim, PeriodicActionOptions{
					Period:   time.Second,
					NumTicks: 5,
					OnAction: func(sim *Simulation) {
						character.GainHealth(sim, amount*character.PseudoStats.HealingTakenMultiplier, healthMetrics)
					},
				})
			},
		})
		character.AddMajorCooldown(MajorCooldown{
			Type:  CooldownTypeSurvival,
			Spell: spell,
		})
	}
}
