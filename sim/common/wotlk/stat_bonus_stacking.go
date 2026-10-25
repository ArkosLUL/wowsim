package wotlk

import (
	"sync"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

type StackingStatBonusEffect struct {
	Name       string
	ID         int32
	AuraID     int32
	Bonus      stats.Stats
	Duration   time.Duration
	MaxStacks  int32
	Callback   core.AuraCallback
	ProcMask   core.ProcMask
	SpellFlags core.SpellFlag
	Outcome    core.HitOutcome
	Harmful    bool
	ProcChance float64

	AnyPeriodic bool
}

func newStackingStatBonusEffect(config StackingStatBonusEffect) {
	core.NewItemEffect(config.ID, func(agent core.Agent) {
		character := agent.GetCharacter()

		auraID := core.ActionID{SpellID: config.AuraID}
		if auraID.IsEmptyAction() {
			auraID = core.ActionID{ItemID: config.ID}
		}
		procAura := core.MakeStackingAura(character, core.StackingStatAura{
			Aura: core.Aura{
				Label:     config.Name + " Proc",
				ActionID:  auraID,
				Duration:  config.Duration,
				MaxStacks: config.MaxStacks,
			},
			BonusPerStack: config.Bonus,
		})

		core.MakeProcTriggerAura(&character.Unit, core.ProcTrigger{
			ActionID:    core.ActionID{ItemID: config.ID},
			Name:        config.Name,
			Callback:    config.Callback,
			ProcMask:    config.ProcMask,
			SpellFlags:  config.SpellFlags,
			Outcome:     config.Outcome,
			Harmful:     config.Harmful,
			ProcChance:  config.ProcChance,
			AnyPeriodic: config.AnyPeriodic,
			Handler: func(sim *core.Simulation, _ *core.Spell, _ *core.SpellResult) {
				procAura.Activate(sim)
				procAura.AddStack(sim)
			},
		})
	})
}

type StackingStatBonusCD struct {
	Name        string
	ID          int32
	AuraID      int32
	Bonus       stats.Stats
	Duration    time.Duration
	MaxStacks   int32
	CD          time.Duration
	Callback    core.AuraCallback
	ProcMask    core.ProcMask
	SpellFlags  core.SpellFlag
	Outcome     core.HitOutcome
	Harmful     bool
	ProcChance  float64
	IsDefensive bool

	// Stacking, when set, adds the stacks instead of the ProcTrigger the fields above describe
	Stacking func(character *core.Character, buffAura *core.Aura)
}

func newStackingStatBonusCD(config StackingStatBonusCD) {
	core.NewItemEffect(config.ID, func(agent core.Agent) {
		character := agent.GetCharacter()

		auraID := core.ActionID{SpellID: config.AuraID}
		if auraID.IsEmptyAction() {
			auraID = core.ActionID{ItemID: config.ID}
		}
		buffAura := core.MakeStackingAura(character, core.StackingStatAura{
			Aura: core.Aura{
				Label:     config.Name + " Aura",
				ActionID:  auraID,
				Duration:  config.Duration,
				MaxStacks: config.MaxStacks,
			},
			BonusPerStack: config.Bonus,
		})

		if config.Stacking != nil {
			config.Stacking(character, buffAura)
		} else {
			core.ApplyProcTriggerCallback(&character.Unit, buffAura, core.ProcTrigger{
				Name:       config.Name,
				Callback:   config.Callback,
				ProcMask:   config.ProcMask,
				SpellFlags: config.SpellFlags,
				Outcome:    config.Outcome,
				Harmful:    config.Harmful,
				ProcChance: config.ProcChance,
				Handler: func(sim *core.Simulation, _ *core.Spell, _ *core.SpellResult) {
					buffAura.AddStack(sim)
				},
			})
		}

		var sharedTimer *core.Timer
		if config.IsDefensive {
			sharedTimer = character.GetDefensiveTrinketCD()
		} else {
			sharedTimer = character.GetOffensiveTrinketCD()
		}

		spell := character.RegisterSpell(core.SpellConfig{
			ActionID: core.ActionID{ItemID: config.ID},
			Flags:    core.SpellFlagNoOnCastComplete,

			Cast: core.CastConfig{
				CD: core.Cooldown{
					Timer:    character.NewTimer(),
					Duration: config.CD,
				},
				SharedCD: core.Cooldown{
					Timer:    sharedTimer,
					Duration: config.Duration,
				},
			},

			ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
				buffAura.Activate(sim)
			},
		})

		character.AddMajorCooldown(core.MajorCooldown{
			Spell: spell,
			Type:  core.CooldownTypeDPS,
		})
	})
}

func init() {
	core.NewItemEffect(38212, func(agent core.Agent) {
		character := agent.GetCharacter()

		procAura := core.MakeStackingAura(character, core.StackingStatAura{
			Aura: core.Aura{
				Label:     "Death Knight's Anguish Proc",
				ActionID:  core.ActionID{SpellID: 54697},
				Duration:  time.Second * 20,
				MaxStacks: 10,
				OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
					if result.Landed() && spell.ProcMask.Matches(core.ProcMaskMeleeOrRanged) {
						aura.AddStack(sim)
					}
				},
			},
			BonusPerStack: stats.Stats{stats.MeleeCrit: 15, stats.SpellCrit: 15},
		})

		triggerAura := core.MakeProcTriggerAura(&character.Unit, core.ProcTrigger{
			Name:        "Death Knight's Anguish",
			Callback:    core.CallbackOnSpellHitDealt,
			ProcMask:    core.ProcMaskMeleeOrRanged,
			Outcome:     core.OutcomeLanded,
			ProcSpellID: 54695,
			ActionID:    core.ActionID{SpellID: 54696},
			Handler: func(sim *core.Simulation, _ *core.Spell, _ *core.SpellResult) {
				procAura.Activate(sim)
			},
		})
		procAura.Icd = triggerAura.Icd
	})

	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Majestic Dragon Figurine",
		ID:        40430,
		AuraID:    60525,
		Duration:  time.Second * 10,
		MaxStacks: 10,
		Bonus:     stats.Stats{stats.Spirit: 18},
		Callback:  core.CallbackOnCastComplete,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Fury of the Five Fights",
		ID:        40431,
		AuraID:    60314,
		Duration:  time.Second * 10,
		MaxStacks: 20,
		Bonus:     stats.Stats{stats.AttackPower: 16, stats.RangedAttackPower: 16},
		Callback:  core.CallbackOnSpellHitDealt,
		ProcMask:  core.ProcMaskMeleeOrRanged,
		Harmful:   true,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Illustration of the Dragon Soul",
		ID:        40432,
		AuraID:    60486,
		Duration:  time.Second * 10,
		MaxStacks: 10,
		Bonus:     stats.Stats{stats.SpellPower: 20},
		// 60485: every landed magic spell, damage or not, and every tick; a triggered hit only with
		// SPELL_ATTR3_NOT_A_PROC (Arcane Missiles' missiles)
		Callback:    core.CallbackOnSpellHitDealt | core.CallbackOnHealDealt | core.CallbackOnPeriodicDamageDealt | core.CallbackOnPeriodicHealDealt,
		ProcMask:    core.ProcMaskSpellHealing | core.ProcMaskSpellDamage,
		Outcome:     core.OutcomeLanded,
		AnyPeriodic: true,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:       "DMC Berserker",
		ID:         42989,
		AuraID:     60196,
		Duration:   time.Second * 12,
		MaxStacks:  3,
		Bonus:      stats.Stats{stats.MeleeCrit: 35, stats.SpellCrit: 35},
		Callback:   core.CallbackOnSpellHitDealt | core.CallbackOnSpellHitTaken,
		Harmful:    true,
		ProcChance: 0.5,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Eye of the Broodmother",
		ID:        45308,
		AuraID:    65006,
		Duration:  time.Second * 10,
		MaxStacks: 5,
		Bonus:     stats.Stats{stats.SpellPower: 25},
		Callback:  core.CallbackOnHealDealt | core.CallbackOnSpellHitDealt | core.CallbackOnPeriodicHealDealt | core.CallbackOnPeriodicDamageDealt,
	})

	core.AddEffectsToTest = false

	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Solance of the Defeated",
		ID:        47041,
		AuraID:    67696,
		Duration:  time.Second * 10,
		MaxStacks: 8,
		Bonus:     stats.Stats{stats.MP5: 16},
		Callback:  core.CallbackOnCastComplete,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Solace of the Defeated H",
		ID:        47059,
		AuraID:    67750,
		Duration:  time.Second * 10,
		MaxStacks: 8,
		Bonus:     stats.Stats{stats.MP5: 18},
		Callback:  core.CallbackOnCastComplete,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Solace of the Fallen",
		ID:        47271,
		AuraID:    67696,
		Duration:  time.Second * 10,
		MaxStacks: 8,
		Bonus:     stats.Stats{stats.MP5: 16},
		Callback:  core.CallbackOnCastComplete,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Solace of the Fallen H",
		ID:        47432,
		AuraID:    67750,
		Duration:  time.Second * 10,
		MaxStacks: 8,
		Bonus:     stats.Stats{stats.MP5: 18},
		Callback:  core.CallbackOnCastComplete,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Muradin's Spyglass",
		ID:        50340,
		AuraID:    71570,
		Duration:  time.Second * 10,
		MaxStacks: 10,
		Bonus:     stats.Stats{stats.SpellPower: 18},
		Callback:  core.CallbackOnSpellHitDealt | core.CallbackOnPeriodicDamageDealt,
		ProcMask:  core.ProcMaskSpellOrProc | core.ProcMaskWeaponProc | core.ProcMaskSuppressedProc,
		Harmful:   true,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Muradin's Spyglass H",
		ID:        50345,
		AuraID:    71572,
		Duration:  time.Second * 10,
		MaxStacks: 10,
		Bonus:     stats.Stats{stats.SpellPower: 20},
		Callback:  core.CallbackOnSpellHitDealt | core.CallbackOnPeriodicDamageDealt,
		ProcMask:  core.ProcMaskSpellOrProc | core.ProcMaskWeaponProc | core.ProcMaskSuppressedProc,
		Harmful:   true,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:       "Unidentifiable Organ",
		ID:         50341,
		AuraID:     71575,
		Duration:   time.Second * 10,
		MaxStacks:  10,
		Bonus:      stats.Stats{stats.Stamina: 24},
		Callback:   core.CallbackOnSpellHitTaken,
		ProcMask:   core.ProcMaskMelee,
		Outcome:    core.OutcomeLanded,
		ProcChance: 0.6,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:       "Unidentifiable Organ H",
		ID:         50344,
		AuraID:     71577,
		Duration:   time.Second * 10,
		MaxStacks:  10,
		Bonus:      stats.Stats{stats.Stamina: 27},
		Callback:   core.CallbackOnSpellHitTaken,
		ProcMask:   core.ProcMaskMelee,
		Outcome:    core.OutcomeLanded,
		ProcChance: 0.6,
	})
	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Herkuml War Token",
		ID:        50355,
		AuraID:    71396,
		Duration:  time.Second * 10,
		MaxStacks: 20,
		Bonus:     stats.Stats{stats.AttackPower: 17, stats.RangedAttackPower: 17},
		Callback:  core.CallbackOnSpellHitDealt,
		ProcMask:  core.ProcMaskMeleeOrRanged,
		Harmful:   true,
	})

	// Stacking CD effects

	newStackingStatBonusCD(StackingStatBonusCD{
		Name:        "Meteorite Crystal",
		ID:          46051,
		AuraID:      65000,
		Duration:    time.Second * 20,
		MaxStacks:   20,
		Bonus:       stats.Stats{stats.MP5: 75},
		CD:          time.Minute * 2,
		IsDefensive: true,
		Stacking:    meteoricInspiration,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Victor's Call",
		ID:        47725,
		AuraID:    67737,
		Duration:  time.Second * 20,
		MaxStacks: 5,
		Bonus:     stats.Stats{stats.AttackPower: 215, stats.RangedAttackPower: 215},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnSpellHitDealt,
		ProcMask:  core.ProcMaskMelee,
		Outcome:   core.OutcomeLanded,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Victor's Call H",
		ID:        47948,
		AuraID:    67746,
		Duration:  time.Second * 20,
		MaxStacks: 5,
		Bonus:     stats.Stats{stats.AttackPower: 250, stats.RangedAttackPower: 250},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnSpellHitDealt,
		ProcMask:  core.ProcMaskMelee,
		Outcome:   core.OutcomeLanded,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Talisman of Volatile Power",
		ID:        47726,
		AuraID:    67735,
		Duration:  time.Second * 20,
		MaxStacks: 8,
		Bonus:     stats.Stats{stats.MeleeHaste: 57, stats.SpellHaste: 57},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnCastComplete,
		ProcMask:  core.ProcMaskSpellOrProc | core.ProcMaskWeaponProc | core.ProcMaskSuppressedProc,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Talisman of Volatile Power H",
		ID:        47946,
		AuraID:    67743,
		Duration:  time.Second * 20,
		MaxStacks: 8,
		Bonus:     stats.Stats{stats.MeleeHaste: 64, stats.SpellHaste: 64},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnCastComplete,
		ProcMask:  core.ProcMaskSpellOrProc | core.ProcMaskWeaponProc | core.ProcMaskSuppressedProc,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:        "Fervor of the Frostborn",
		ID:          47727,
		AuraID:      67727,
		Duration:    time.Second * 20,
		MaxStacks:   5,
		Bonus:       stats.Stats{stats.Armor: 1265},
		CD:          time.Minute * 2,
		Callback:    core.CallbackOnSpellHitTaken,
		Outcome:     core.OutcomeLanded,
		IsDefensive: true,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:        "Ferver of the Frostborn H",
		ID:          47949,
		AuraID:      67741,
		Duration:    time.Second * 20,
		MaxStacks:   5,
		Bonus:       stats.Stats{stats.Armor: 1422},
		CD:          time.Minute * 2,
		Callback:    core.CallbackOnSpellHitTaken,
		Outcome:     core.OutcomeLanded,
		IsDefensive: true,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:       "Binding Light",
		ID:         47728,
		AuraID:     67723,
		Duration:   time.Second * 20,
		MaxStacks:  8,
		Bonus:      stats.Stats{stats.SpellPower: 66},
		CD:         time.Minute * 2,
		Callback:   core.CallbackOnCastComplete,
		SpellFlags: core.SpellFlagHelpful,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:       "Binding Light H",
		ID:         47947,
		AuraID:     67739,
		Duration:   time.Second * 20,
		MaxStacks:  8,
		Bonus:      stats.Stats{stats.SpellPower: 74},
		CD:         time.Minute * 2,
		Callback:   core.CallbackOnCastComplete,
		SpellFlags: core.SpellFlagHelpful,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Fetish of Volatile Power",
		ID:        47879,
		AuraID:    67735,
		Duration:  time.Second * 20,
		MaxStacks: 8,
		Bonus:     stats.Stats{stats.MeleeHaste: 57, stats.SpellHaste: 57},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnCastComplete,
		ProcMask:  core.ProcMaskSpellOrProc | core.ProcMaskWeaponProc | core.ProcMaskSuppressedProc,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Fetish of Volatile Power H",
		ID:        48018,
		AuraID:    67743,
		Duration:  time.Second * 20,
		MaxStacks: 8,
		Bonus:     stats.Stats{stats.MeleeHaste: 64, stats.SpellHaste: 64},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnCastComplete,
		ProcMask:  core.ProcMaskSpellOrProc | core.ProcMaskWeaponProc | core.ProcMaskSuppressedProc,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:       "Binding Stone",
		ID:         47880,
		AuraID:     67723,
		Duration:   time.Second * 20,
		MaxStacks:  8,
		Bonus:      stats.Stats{stats.SpellPower: 66},
		CD:         time.Minute * 2,
		Callback:   core.CallbackOnCastComplete,
		SpellFlags: core.SpellFlagHelpful,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:       "Binding Stone H",
		ID:         48019,
		AuraID:     67739,
		Duration:   time.Second * 20,
		MaxStacks:  8,
		Bonus:      stats.Stats{stats.SpellPower: 74},
		CD:         time.Minute * 2,
		Callback:   core.CallbackOnCastComplete,
		SpellFlags: core.SpellFlagHelpful,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Vengeance of the Forsaken",
		ID:        47881,
		AuraID:    67737,
		Duration:  time.Second * 20,
		MaxStacks: 5,
		Bonus:     stats.Stats{stats.AttackPower: 215, stats.RangedAttackPower: 215},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnSpellHitDealt,
		ProcMask:  core.ProcMaskMelee,
		Outcome:   core.OutcomeLanded,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Vengeance of the Forsaken H",
		ID:        48020,
		AuraID:    67746,
		Duration:  time.Second * 20,
		MaxStacks: 5,
		Bonus:     stats.Stats{stats.AttackPower: 250, stats.RangedAttackPower: 250},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnSpellHitDealt,
		ProcMask:  core.ProcMaskMelee,
		Outcome:   core.OutcomeLanded,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:        "Eitrigg's Oath",
		ID:          47882,
		AuraID:      67727,
		Duration:    time.Second * 20,
		MaxStacks:   5,
		Bonus:       stats.Stats{stats.Armor: 1265},
		CD:          time.Minute * 2,
		Callback:    core.CallbackOnSpellHitTaken,
		Outcome:     core.OutcomeLanded,
		IsDefensive: true,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:        "Eitrigg's Oath H",
		ID:          48021,
		AuraID:      67741,
		Duration:    time.Second * 20,
		MaxStacks:   5,
		Bonus:       stats.Stats{stats.Armor: 1422},
		CD:          time.Minute * 2,
		Callback:    core.CallbackOnSpellHitTaken,
		Outcome:     core.OutcomeLanded,
		IsDefensive: true,
	})

	core.AddEffectsToTest = true
}

const attr3NotAProc = 0x200

// Penance's casts, whose channels cost no mana but whose bolts still stack
var penanceCasts = map[int32]bool{47540: true, 53005: true, 53006: true, 53007: true}

// meteoricTicks are the tick spells of the channels that cost mana, those with SPELL_ATTR3_NOT_A_PROC
// (Arcane Missiles' missiles, Tranquility's heals)
var meteoricTicks = sync.OnceValue(func() map[int32]bool {
	ticks := map[int32]bool{}
	for i := range serverdata.Spells() {
		s := &serverdata.Spells()[i]
		if s.Flags&serverdata.FlagChanneled == 0 || !serverCostsMana(s) {
			continue
		}
		for _, e := range s.Effects {
			switch e.Aura {
			case 23, 227: // periodic trigger spell, with or without a value
				if t := serverdata.SpellByID(e.TriggerSpell); t != nil && t.Attributes[3]&attr3NotAProc != 0 {
					ticks[t.ID] = true
				}
			}
		}
	}
	return ticks
})

func serverCostsMana(s *serverdata.Spell) bool {
	return s.PowerType == 0 && (s.ManaCost > 0 || s.ManaCostPct > 0)
}

func costsMana(spell *core.Spell) bool {
	if s := spell.ServerSpell(); s != nil {
		return serverCostsMana(s)
	}
	_, ok := spell.Cost.(*core.ManaCost)
	return ok
}

// meteoricInspiration is 64999's spell_item_meteoric_inspiration: a stack at the finish phase of an untriggered
// spell that costs mana (Call of the Elements' totems too), of a mana channel's or Penance's tick, or of a Beacon
// of Light copy of a heal that costs mana. The finish phase is the cast's end, or its landing for a missile.
func meteoricInspiration(character *core.Character, buffAura *core.Aura) {
	// a Beacon of Light copy lands inside the heal it copies, so it's counted there and stacks with that cast
	beaconCopies := int32(0)
	addStacks := func(sim *core.Simulation, n int32) {
		if n > 0 && buffAura.IsActive() {
			buffAura.SetStacks(sim, buffAura.GetStacks()+n)
		}
	}
	atFinish := func(sim *core.Simulation, spell *core.Spell, n int32) {
		if spell.MissileSpeed > 0 && !spell.Flags.Matches(core.SpellFlagChanneled) {
			spell.WaitTravelTime(sim, func(sim *core.Simulation) { addStacks(sim, n) })
			return
		}
		addStacks(sim, n)
	}
	onCast := func(sim *core.Simulation, spell *core.Spell) {
		n := 1 + beaconCopies
		beaconCopies = 0
		if costsMana(spell) {
			atFinish(sim, spell, n)
		}
	}

	buffAura.OnInit = func(_ *core.Aura, _ *core.Simulation) {
		// casts kept out of OnCastComplete (Icy Veins, Tree of Life, Avenging Wrath) stack too. A helper that
		// casts the real spell, like the trap weave, has no cost of its own and doesn't.
		for _, spell := range character.Spellbook {
			if spell.Flags.Matches(core.SpellFlagAPL) && spell.Flags.Matches(core.SpellFlagNoOnCastComplete) && spell.Cost != nil {
				applyEffects := spell.ApplyEffects
				spell.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
					applyEffects(sim, target, spell)
					onCast(sim, spell)
				}
			}
		}
	}
	buffAura.OnGain = func(_ *core.Aura, _ *core.Simulation) {
		beaconCopies = 0
	}
	buffAura.OnCastComplete = func(_ *core.Aura, sim *core.Simulation, spell *core.Spell) {
		switch {
		case spell.Flags.Matches(core.SpellFlagAPL):
			onCast(sim, spell)
		case meteoricTicks()[spell.ActionID.SpellID]:
			atFinish(sim, spell, 1)
		}
	}
	// the sim deals Penance's bolts as 53007's own heals and ticks, as they land
	buffAura.OnHealDealt = func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, _ *core.SpellResult) {
		switch id := spell.ActionID.SpellID; {
		case id == 53652 || id == 53653 || id == 53654:
			beaconCopies++
		case penanceCasts[id]:
			addStacks(sim, 1)
		}
	}
	buffAura.OnPeriodicDamageDealt = func(_ *core.Aura, sim *core.Simulation, spell *core.Spell, _ *core.SpellResult) {
		if penanceCasts[spell.ActionID.SpellID] {
			addStacks(sim, 1)
		}
	}
}
