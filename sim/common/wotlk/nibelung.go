package wotlk

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

const (
	nibelungID       = 49992
	nibelungHeroicID = 50648

	// creature_template's attack time for 38391 and 38392. The AI casts Smite where a swing would come
	// (DoSpellAttackIfReady), so this timer paces it, not a cooldown or a GCD.
	valkyrAttackTime = 2 * time.Second
)

var valkyrStats = stats.Stats{
	stats.Stamina: 1260,
}

type ValkyrPet struct {
	*core.Pet
	smite         *core.Spell
	healthMetrics *core.ResourceMetrics
}

func newValkyr(character *core.Character) *ValkyrPet {
	valkyr := &ValkyrPet{
		Pet: core.NewPet("Valkyr", character, valkyrStats, func(ownerStats stats.Stats) stats.Stats {
			return stats.Stats{}
		}, false, true),
	}
	// SummonProperties 2909 is a guardian's, with neither a pet's type nor its category (Guardian::Guardian)
	valkyr.Uncontrollable = true
	return valkyr
}

func getSmiteConfig(valkyr *ValkyrPet, spellId int32, damageMin float64, damageMax float64) core.SpellConfig {
	return core.SpellConfig{
		ActionID:         core.ActionID{SpellID: spellId},
		SpellSchool:      core.SpellSchoolHoly,
		ProcMask:         core.ProcMaskSpellDamage,
		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(damageMin, damageMax)
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicCrit)
			spell.DealDamage(sim, result)
			valkyr.GainHealth(sim, valkyr.MaxHealth()*0.25, valkyr.healthMetrics)
		},
		CritMultiplier: valkyr.DefaultSpellCritMultiplier(),
	}

}

func (valkyr *ValkyrPet) registerSmite(isHeroic bool) {
	spellId := int32(71841)
	if isHeroic {
		spellId = 71842
	}

	if valkyr.healthMetrics == nil {
		valkyr.healthMetrics = valkyr.NewHealthMetrics(core.ActionID{SpellID: spellId})
	}

	if isHeroic {
		smite := getSmiteConfig(valkyr, spellId, 1804, 2022)
		valkyr.smite = valkyr.GetOrRegisterSpell(smite)
	} else {
		smite := getSmiteConfig(valkyr, spellId, 1591, 1785)
		valkyr.smite = valkyr.GetOrRegisterSpell(smite)
	}
}

func (valkyr *ValkyrPet) Initialize() {}

func (valkyr *ValkyrPet) Reset(_ *core.Simulation) {}

// npc_pet_gen_valkyr_guardian's UpdateAI, on the Val'kyr's own updates: Smite whenever its attack timer
// is up, which starts the timer over.
func (valkyr *ValkyrPet) ExecuteCustomRotation(sim *core.Simulation) {
	if next := sim.NextServerTick(sim.CurrentTime); next > sim.CurrentTime {
		valkyr.WaitUntil(sim, next)
		return
	}
	valkyr.smite.Cast(sim, valkyr.CurrentTarget)
	valkyr.WaitUntil(sim, sim.NextServerTick(sim.CurrentTime+valkyrAttackTime))
}

func (valkyr *ValkyrPet) GetPet() *core.Pet {
	return valkyr.Pet
}

// valkyrLife is how long a Val'kyr that gets averageCasts Smites off is out: an attack time for each, the
// summon's 30 s at most.
func valkyrLife(averageCasts float64) time.Duration {
	return min(time.Duration(averageCasts*float64(valkyrAttackTime)), time.Second*30)
}

func MakeNibelungTriggerAura(agent core.Agent, isHeroic bool) {
	var auraSpellId, procSpellId int32

	if isHeroic {
		auraSpellId = 71844
		procSpellId = 71846
	} else {
		auraSpellId = 71843
		procSpellId = 71845
	}

	character := agent.GetCharacter()
	valkyrAura := character.RegisterAura(core.Aura{
		Label:    "Summon Val'kyr",
		ActionID: core.ActionID{SpellID: auraSpellId},
		Duration: time.Second * 30,
	})

	core.MakeProcTriggerAura(&character.Unit, core.ProcTrigger{
		Name:            "Nibelung Proc",
		Callback:        core.CallbackOnCastComplete,
		ProcMask:        core.ProcMaskSpellOrProc,
		ProcMaskExclude: core.ProcMaskNotInSpellbook,
		Harmful:         true,
		ProcChance:      0.02,
		ICD:             time.Millisecond * 250,
		ActionID:        core.ActionID{SpellID: procSpellId},
		Handler: func(sim *core.Simulation, spell *core.Spell, _ *core.SpellResult) {
			for _, petAgent := range character.PetAgents {
				if valkyr, ok := petAgent.(*ValkyrPet); ok && !valkyr.IsEnabled() {
					valkyr.registerSmite(isHeroic)

					averageCasts := character.NibelungAverageCasts
					valkyrAura.Duration = max(valkyrLife(averageCasts), time.Millisecond*250)

					if averageCasts > 0 {
						valkyr.EnableWithTimeout(sim, petAgent, valkyrAura.Duration)
					}
					break
				}
			}

			valkyrAura.Activate(sim)
		},
	})
}

// Only for a character with Nibelung equipped or in its item swap: every pet is a unit, and each
// spell's metrics and attack tables grow with the unit count.
func ConstructValkyrPets(character *core.Character) {
	if !hasNibelung(character) {
		return
	}
	for i := 0; i < 10; i++ {
		valkyr := newValkyr(character)
		character.AddPet(valkyr)
	}
}

// a staff, so only the main hand can hold it
func hasNibelung(character *core.Character) bool {
	isNibelung := func(item *core.Item) bool {
		return item.ID == nibelungID || item.ID == nibelungHeroicID
	}
	return isNibelung(character.MainHand()) || isNibelung(character.ItemSwap.GetItem(proto.ItemSlot_ItemSlotMainHand))
}
