package shaman

import (
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// npc_pet_shaman_fire_elemental's event timers, in ms: each spell's first cast after it engages, then
// the wait from one cast to the next. The AI draws a whole ms anywhere in the range.
var (
	fireNovaFirst   = [2]int{5000, 20000}
	fireNovaRepeat  = [2]int{8000, 15000}
	fireBlastFirst  = [2]int{5000, 20000}
	fireBlastRepeat = [2]int{4000, 8000}
)

type FireElemental struct {
	*core.Pet

	FireBlast *core.Spell
	FireNova  *core.Spell

	FireShieldAura *core.Aura

	shamanOwner *Shaman

	// the AI's two events: when each is due, and the update that runs whatever is due by then
	novaDue, blastDue time.Duration
	// two events due on one update run in the order of their times, and level in the order they were
	// scheduled
	novaScheduled, blastScheduled int
	aiAction                      *core.PendingAction
}

func (shaman *Shaman) NewFireElemental(bonusSpellPower float64) *FireElemental {
	fireElemental := &FireElemental{
		Pet:         core.NewPet("Greater Fire Elemental", &shaman.Character, fireElementalPetBaseStats, fireElementalStatInheritance, false, true),
		shamanOwner: shaman,
	}
	fireElemental.BuffImmunity = core.PetImmuneToScaling
	// 32982 summons it with SummonProperties 61, a plain guardian's (Guardian::Guardian)
	fireElemental.Uncontrollable = true
	// pet_levelstats' 2280 mana, and Guardian::UpdateMaxPower's default 15 a point of intellect
	fireElemental.EnableCreatureManaBar(2280, fireElementalPetBaseStats[stats.Intellect], 15, false)
	// InitStatsForLevel's NPC_FIRE_ELEMENTAL case: 3.5 a level -/+ the level, in the template's
	// fire school at its 2 s attack time
	fireElemental.EnableAutoAttacks(fireElemental, core.AutoAttackOptions{
		MainHand: core.Weapon{
			BaseDamageMin:  200,
			BaseDamageMax:  360,
			SwingSpeed:     2,
			CritMultiplier: 2,
			SpellSchool:    core.SpellSchoolFire,
		},
		AutoSwingMelee: true,
		// has a swing ask the AI first: UpdateAI casts what's due ahead of DoMeleeAttackIfReady
		ReplaceMHSwing: func(_ *core.Simulation, swing *core.Spell) *core.Spell { return swing },
	})
	fireElemental.AddStatDependency(stats.Strength, stats.AttackPower, 2)

	// the spell power the totem options add at the summon scales as the owner's own does
	if bonusSpellPower > 0 {
		fireElemental.AddStat(stats.SpellPower, core.PetScalingPct(bonusSpellPower, 100))
		fireElemental.AddStat(stats.AttackPower, core.PetScalingPct(bonusSpellPower, 300))
	}

	fireElemental.OnPetEnable = fireElemental.enable
	fireElemental.OnPetDisable = fireElemental.disable

	shaman.AddPet(fireElemental)

	return fireElemental
}

func (fireElemental *FireElemental) enable(sim *core.Simulation) {
	fireElemental.FireShieldAura.Activate(sim)
	if sim.CurrentTime >= 0 {
		fireElemental.engage(sim)
		return
	}
	// out before the pull, it has nothing to attack until then
	sim.AddPendingAction(&core.PendingAction{
		NextActionAt: 0,
		Priority:     core.ActionPriorityPrePull,
		OnAction: func(sim *core.Simulation) {
			if fireElemental.IsEnabled() && fireElemental.aiAction == nil {
				fireElemental.engage(sim)
			}
		},
	})
}

func (fireElemental *FireElemental) disable(sim *core.Simulation) {
	fireElemental.FireShieldAura.Deactivate(sim)
	if fireElemental.aiAction != nil {
		fireElemental.aiAction.Cancel(sim)
		fireElemental.aiAction = nil
	}
}

func (fireElemental *FireElemental) GetPet() *core.Pet {
	return fireElemental.Pet
}

func (fireElemental *FireElemental) Initialize() {

	fireElemental.registerFireBlast()
	fireElemental.registerFireNova()
	fireElemental.registerFireShieldAura()
}

func (fireElemental *FireElemental) Reset(_ *core.Simulation) {
	fireElemental.aiAction = nil
}

// The AI's events cast for it, so the GCD loop has nothing to do. A swing comes through here before it
// lands: an event due on its update casts first, and that cast takes the swing, a Fire Blast by starting
// the timer over, a Fire Nova by holding it until it lands or breaks.
func (fireElemental *FireElemental) ExecuteCustomRotation(sim *core.Simulation) {
	if pa := fireElemental.aiAction; pa != nil && pa.NextActionAt <= sim.CurrentTime {
		pa.Cancel(sim)
		fireElemental.runEvents(sim)
	}
	// a Fire Nova being cast lands off the GCD's action, so that one stays where it is
	if fireElemental.GCD.IsReady(sim) {
		fireElemental.WaitUntil(sim, core.NeverExpires)
	}
}

// engage is the AI's JustEngagedWith, on the elemental's first update: it attacks the shaman's target
// at once, and schedules its first Fire Nova and Fire Blast.
func (fireElemental *FireElemental) engage(sim *core.Simulation) {
	start := fireElemental.AIClockStart(sim)
	fireElemental.novaScheduled, fireElemental.blastScheduled = 0, 1
	fireElemental.novaDue = start + fireElemental.eventDelay(sim, fireNovaFirst)
	fireElemental.blastDue = start + fireElemental.eventDelay(sim, fireBlastFirst)
	fireElemental.queueEvents(sim)
}

func (fireElemental *FireElemental) eventDelay(sim *core.Simulation, ms [2]int) time.Duration {
	roll := ms[0] + int(sim.RandomFloat("Fire Elemental Event")*float64(ms[1]-ms[0]+1))
	return time.Duration(min(roll, ms[1])) * time.Millisecond
}

// queueEvents wakes the AI on the update its next event is due by.
func (fireElemental *FireElemental) queueEvents(sim *core.Simulation) {
	fireElemental.aiAction = &core.PendingAction{
		NextActionAt: sim.NextServerTick(min(fireElemental.novaDue, fireElemental.blastDue)),
		// a cast landing on this update goes first, as Unit::Update runs before UpdateAI
		Priority: core.ActionPriorityLow,
		OnAction: fireElemental.runEvents,
	}
	sim.AddPendingAction(fireElemental.aiAction)
}

// runEvents is the AI's UpdateAI: every event due by now, earliest first. Neither cast is a DoCast, so
// nothing holds one back while the other is still being cast.
func (fireElemental *FireElemental) runEvents(sim *core.Simulation) {
	if !fireElemental.IsEnabled() {
		return
	}
	for {
		nova, blast := fireElemental.novaDue <= sim.CurrentTime, fireElemental.blastDue <= sim.CurrentTime
		if nova && blast {
			if fireElemental.novaDue != fireElemental.blastDue {
				nova = fireElemental.novaDue < fireElemental.blastDue
			} else {
				nova = fireElemental.novaScheduled < fireElemental.blastScheduled
			}
			blast = !nova
		}
		scheduled := max(fireElemental.novaScheduled, fireElemental.blastScheduled) + 1
		switch {
		case nova:
			fireElemental.FireNova.Cast(sim, fireElemental.CurrentTarget)
			fireElemental.novaDue = sim.CurrentTime + fireElemental.eventDelay(sim, fireNovaRepeat)
			fireElemental.novaScheduled = scheduled
			continue
		case blast:
			// a Fire Nova still being cast is broken off for it, if the elemental can pay for the blast
			if fireElemental.CurrentMana() >= fireElemental.FireBlast.DefaultCast.Cost {
				fireElemental.InterruptCast(sim, fireElemental.FireNova)
			}
			fireElemental.FireBlast.Cast(sim, fireElemental.CurrentTarget)
			fireElemental.blastDue = sim.CurrentTime + fireElemental.eventDelay(sim, fireBlastRepeat)
			fireElemental.blastScheduled = scheduled
			continue
		}
		break
	}
	fireElemental.queueEvents(sim)
}

// pet_levelstats for 15438 at 80, and 2 * Str - 20 attack power
var fireElementalPetBaseStats = core.PetCreateStats(283, 160, 125, 405, 460, 1100).Add(stats.Stats{
	// Guardian::UpdateMaxHealth: 8000 plus 10 a point of stamina over 125. The universal stamina
	// dependency (10 a point, -180) adds the rest
	stats.Health:      8000 - 10*125 + 180,
	stats.AttackPower: -20,

	// Unit::GetUnitCriticalChance and m_baseSpellCritChance give every non-player unit a flat 5%; the fire
	// elemental isn't one of the pets a scripted aura scales further by a stat.
	stats.MeleeCrit: 5 * core.CritRatingPerCritChance,
	stats.SpellCrit: 5 * core.CritRatingPerCritChance,
})

// fireElementalStatInheritance is spell_sha_fire_elemental_scaling on 35665 and 35666: 30% of the
// owner's intellect, 35% of his armor, and 300% and 100% of his fire spell power as attack and spell
// power. No stamina: 35665's first effect is a dummy. Hit and expertise come from core's scaling
// aura, not from here.
func fireElementalStatInheritance(ownerStats stats.Stats) stats.Stats {
	return stats.Stats{
		stats.Intellect:   core.PetScalingPct(ownerStats[stats.Intellect], 30),
		stats.Armor:       core.PetScalingPct(ownerStats[stats.Armor], 35),
		stats.AttackPower: core.PetScalingPct(ownerStats[stats.SpellPower], 300),
		stats.SpellPower:  core.PetScalingPct(ownerStats[stats.SpellPower], 100),
	}
}
