package deathknight

import (
	"math"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func (dk *Deathknight) registerDancingRuneWeaponCD() {
	if !dk.Talents.DancingRuneWeapon {
		return
	}

	duration := time.Second * 12
	if dk.HasMajorGlyph(proto.DeathknightMajorGlyph_GlyphOfDancingRuneWeapon) {
		duration += time.Second * 5
	}

	dancingRuneWeaponAura := dk.RegisterAura(core.Aura{
		Label:    "Dancing Rune Weapon",
		ActionID: core.ActionID{SpellID: 49028},
		Duration: duration,
		// Casts
		OnCastComplete: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell) {
			// the weapon is unsummoned on its own update, which can come before this aura runs out
			if !dk.RuneWeapon.IsEnabled() {
				return
			}
			switch spell {
			case dk.IcyTouch:
				dk.RuneWeapon.IcyTouch.Cast(sim, spell.Unit.CurrentTarget)
			case dk.PlagueStrike:
				dk.RuneWeapon.PlagueStrike.Cast(sim, spell.Unit.CurrentTarget)
			case dk.DeathStrike:
				dk.RuneWeapon.DeathStrike.Cast(sim, spell.Unit.CurrentTarget)
			case dk.BloodStrike:
				dk.RuneWeapon.BloodStrike.Cast(sim, spell.Unit.CurrentTarget)
			case dk.HeartStrike:
				dk.RuneWeapon.HeartStrike.Cast(sim, spell.Unit.CurrentTarget)
			case dk.RuneStrike:
				dk.RuneWeapon.RuneStrike.Cast(sim, spell.Unit.CurrentTarget)
			case dk.Pestilence:
				dk.RuneWeapon.Pestilence.Cast(sim, spell.Unit.CurrentTarget)
			case dk.BloodBoil:
				dk.RuneWeapon.BloodBoil.Cast(sim, spell.Unit.CurrentTarget)
			}
		},
		// spell_dk_dancing_rune_weapon's Death Coil exception matches on 47632's own damage
		// event, so a miss (no damage dealt) mirrors nothing and a Sudden Doom-triggered cast
		// mirrors the same as a normal one; dk.DeathCoil's OnCastComplete fires on both a miss
		// and a hit, so the mirror hooks the inner damage spell's outcome instead.
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || !dk.RuneWeapon.IsEnabled() {
				return
			}
			if spell == dk.DeathCoilDamage {
				dk.RuneWeapon.DeathCoil.Cast(sim, result.Target)
			}
			// a white swing, extra attacks too, but not Rune Strike: that one has a spell to copy
			if spell.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) && !spell.ProcMask.Matches(core.ProcMaskMeleeSpecial) {
				hand := core.TernaryInt(spell.ProcMask.Matches(core.ProcMaskMeleeOHAuto), 1, 0)
				dk.RuneWeapon.swingCopies[hand].Cast(sim, result.Target)
			}
		},
	})

	dk.RuneWeapon.ownerSwings = [2]*core.Spell{
		dk.registerDrwOwnerSwing(dk.AutoAttacks.MHConfig(), 3),
		dk.registerDrwOwnerSwing(dk.AutoAttacks.OHConfig(), 4),
	}

	dk.DancingRuneWeapon = dk.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 49028},
		Flags:    core.SpellFlagAPL,

		RuneCost: core.RuneCostOptions{
			RunicPowerCost: 60,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    dk.NewTimer(),
				Duration: time.Second * 90,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			dk.RuneWeapon.EnableWithTimeout(sim, dk.RuneWeapon, duration)
			dk.RuneWeapon.CancelGCDTimer(sim)
			dancingRuneWeaponAura.Activate(sim)
		},
	})
}

func (runeWeapon *RuneWeaponPet) getImpurityBonus(spell *core.Spell) float64 {
	return spell.MeleeAttackPower()
}

type RuneWeaponPet struct {
	*core.Pet

	dkOwner            *Deathknight
	ownerTablesApplied bool

	IcyTouch     *core.Spell
	PlagueStrike *core.Spell

	DeathStrike *core.Spell
	DeathCoil   *core.Spell

	BloodStrike       *core.Spell
	HeartStrike       *core.Spell
	HeartStrikeOffHit *core.Spell

	RuneStrike *core.Spell

	Pestilence *core.Spell
	BloodBoil  *core.Spell

	// Diseases
	FrostFeverSpell  *core.Spell
	BloodPlagueSpell *core.Spell

	// by hand: the DK's own white swing, rolled again but never dealt, and the weapon's half of it
	ownerSwings [2]*core.Spell
	swingCopies [2]*core.Spell
}

// registerDrwOwnerSwing is the white swing spell_dk_dancing_rune_weapon rolls again on the DK for each one
// of his that lands (CalculateMeleeDamage): his table, weapon, attack power and mods for that hand. Its own
// result is never dealt, so it fires nothing.
func (dk *Deathknight) registerDrwOwnerSwing(swing *core.SpellConfig, tag int32) *core.Spell {
	config := *swing
	config.ActionID = core.ActionID{OtherID: proto.OtherAction_OtherActionAttack, Tag: tag}
	config.Flags |= core.SpellFlagNoMetrics | core.SpellFlagNoLogs | core.SpellFlagNoOnCastComplete | core.SpellFlagNoOnDamageDealt
	config.ApplyEffects = nil
	return dk.RegisterSpell(config)
}

// registerSwingCopy is the weapon's copy of a landed white swing of the DK's: a fresh roll of his own,
// halved, dealt by the weapon with no procs (DealMeleeDamage alone).
func (runeWeapon *RuneWeaponPet) registerSwingCopy(hand int) *core.Spell {
	ownerSwing := runeWeapon.ownerSwings[hand]
	return runeWeapon.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{OtherID: proto.OtherAction_OtherActionAttack, Tag: int32(hand + 1)},
		SpellSchool: ownerSwing.SpellSchool,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagNoOnCastComplete | core.SpellFlagNoOnDamageDealt,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			var base float64
			if hand == 0 {
				base = ownerSwing.MHStrikeDamage(sim, ownerSwing.MeleeAttackPower())
			} else {
				base = ownerSwing.OHStrikeDamage(sim, ownerSwing.MeleeAttackPower())
			}
			roll := ownerSwing.CalcDamage(sim, target, base, ownerSwing.OutcomeMeleeWhite)
			result := spell.NewResult(target)
			result.Outcome = roll.Outcome
			// HandleProc halves a uint32
			result.Damage = math.Floor(roll.Damage / 2)
			result.Threat = spell.ThreatFromDamage(result.Outcome, result.Damage)
			ownerSwing.DisposeResult(roll)
			countSwingOutcome(&spell.SpellMetrics[target.UnitIndex], result.Outcome)
			spell.DealDamage(sim, result)
		},
	})
}

// countSwingOutcome books an outcome the way the white table books it on the swing that rolls it.
func countSwingOutcome(metrics *core.SpellMetrics, outcome core.HitOutcome) {
	switch {
	case outcome.Matches(core.OutcomeMiss):
		metrics.Misses++
	case outcome.Matches(core.OutcomeDodge):
		metrics.Dodges++
	case outcome.Matches(core.OutcomeParry):
		metrics.Parries++
	case outcome.Matches(core.OutcomeBlock):
		metrics.Blocks++
	case outcome.Matches(core.OutcomeGlance):
		metrics.Glances++
	case outcome.Matches(core.OutcomeCrush):
		metrics.Crushes++
	case outcome.Matches(core.OutcomeCrit):
		metrics.Crits++
	default:
		metrics.Hits++
	}
}

func (runeWeapon *RuneWeaponPet) Initialize() {
	runeWeapon.swingCopies = [2]*core.Spell{runeWeapon.registerSwingCopy(0), runeWeapon.registerSwingCopy(1)}

	runeWeapon.dkOwner.registerDrwDiseaseDots()
	runeWeapon.dkOwner.registerDrwPestilenceSpell()
	runeWeapon.dkOwner.registerDrwBloodBoilSpell()

	runeWeapon.dkOwner.registerDrwIcyTouchSpell()
	runeWeapon.dkOwner.registerDrwPlagueStrikeSpell()
	runeWeapon.dkOwner.registerDrwDeathStrikeSpell()
	runeWeapon.dkOwner.registerDrwBloodStrikeSpell()
	runeWeapon.dkOwner.registerDrwHeartStrikeSpell()
	runeWeapon.dkOwner.registerDrwDeathCoilSpell()
	runeWeapon.dkOwner.registerDrwRuneStrikeSpell()
}

func (dk *Deathknight) DrwWeaponDamage(sim *core.Simulation, spell *core.Spell) float64 {
	weapon := spell.Unit.AutoAttacks.MH()
	return weapon.WholeDamage(sim, weapon.SwingSpeed, spell.MeleeAttackPower(), spell.BonusWeaponDamage())
}

// drwVersusAttackPowerBonus is the creature-type attack power on a rune weapon hit: Unit::MeleeDamageBonusDone
// hands entry 27893's hit to its owner and halves what comes back, so it's the DK's own int32(AP / 14 x speed)
// off the DK's main hand, in float32 like the server, halved with the rest of the hit. The rune weapon has none
// of its own.
// The owner skips aura 79 on this physical flat bonus, but the rune weapon's damage multipliers hold the owner's
// aura 79s and the halving, so it comes back divided by them: the hit then gives it only the halving, the
// per-target multipliers and the spell's own mods.
func (dk *Deathknight) drwVersusAttackPowerBonus(spell *core.Spell, normalized bool) float64 {
	ap := dk.PseudoStats.MobTypeAttackPower
	if ap == 0 {
		return 0
	}
	weapon := dk.AutoAttacks.MH()
	speed := weapon.SwingSpeed
	if normalized {
		speed = weapon.NormalizedSwingSpeed
	}
	bonus := math.Trunc(float64(float32(ap) / float32(core.DefaultAttackPowerPerDPS) * float32(speed)))
	ps := &spell.Unit.PseudoStats
	return bonus * 0.5 / (ps.DamageDealtMultiplier * ps.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical])
}

func (dk *Deathknight) NewRuneWeapon() *RuneWeaponPet {
	// Its hit and expertise come from the 61017 npc_pet_dk_dancing_rune_weapon gives it. Its
	// crit chance is the owner's own (Spell.cpp:8466-8478, "totem's inherit owner crit chance
	// and dancing rune weapon").
	runeWeapon := &RuneWeaponPet{
		Pet: core.NewPet("Rune Weapon", &dk.Character, stats.Stats{
			stats.Stamina: 100,
		}, func(ownerStats stats.Stats) stats.Stats {
			return stats.Stats{
				stats.AttackPower: ownerStats[stats.AttackPower],

				stats.MeleeCrit: ownerStats[stats.MeleeCrit],
				stats.SpellCrit: ownerStats[stats.SpellCrit],
			}
		}, false, true),
		dkOwner: dk,
	}

	// SummonProperties 208 is a guardian's with no pet bar
	runeWeapon.Uncontrollable = true
	runeWeapon.OwnerBuffsOnly = true
	runeWeapon.OnPetEnable = runeWeapon.enable
	runeWeapon.OnPetDisable = runeWeapon.disable

	mhWeapon := dk.WeaponFromMainHand(dk.DefaultMeleeCritMultiplier())

	baseDamage := mhWeapon.AverageDamage() / mhWeapon.SwingSpeed * 3.5
	mhWeapon.BaseDamageMin = baseDamage - 150
	mhWeapon.BaseDamageMax = baseDamage + 150

	mhWeapon.SwingSpeed = 3.5
	mhWeapon.NormalizedSwingSpeed = 3.3

	// npc_pet_dk_dancing_rune_weapon is a NullCreatureAI and never swings: its white hits are swingCopies.
	// The weapon is only for its strikes.
	runeWeapon.EnableAutoAttacks(runeWeapon, core.AutoAttackOptions{
		MainHand: mhWeapon,
	})

	runeWeapon.PseudoStats.DamageTakenMultiplier = 0
	// the orc's Command (65221) only goes to risen ghouls and the gargoyle
	if dk.RacialTraits == proto.Race_RaceOrc {
		runeWeapon.PseudoStats.DamageDealtMultiplier /= 1.05
	}

	dk.AddPet(runeWeapon)

	return runeWeapon
}

func (runeWeapon *RuneWeaponPet) GetPet() *core.Pet {
	return runeWeapon.Pet
}

func (runeWeapon *RuneWeaponPet) Reset(_ *core.Simulation) {
	// its bonus done is the owner's, so the owner's per-target multipliers (DONE_VERSUS, like Troll Beast
	// Slaying) count on every hit. Attack tables outlive a reset, and the owner sets its own on its first.
	if !runeWeapon.ownerTablesApplied {
		runeWeapon.ownerTablesApplied = true
		for i, at := range runeWeapon.AttackTables {
			at.DamageDealtMultiplier *= runeWeapon.dkOwner.AttackTables[i].DamageDealtMultiplier
		}
	}
}

func (runeWeapon *RuneWeaponPet) ExecuteCustomRotation(_ *core.Simulation) {
}

func (runeWeapon *RuneWeaponPet) enable(_ *core.Simulation) {
	// Unit::SpellDamageBonusDone and MeleeDamageBonusDone give entry 27893 the owner's own
	// bonus (AP/SP, percent mods included) and halve it; this instead halves everything the
	// rune weapon deals, base roll included, which overstates the cut on abilities whose base
	// roll is a large share of the hit.
	runeWeapon.dkOwner.drwDmgSnapshot = runeWeapon.dkOwner.PseudoStats.DamageDealtMultiplier * 0.5
	runeWeapon.dkOwner.RuneWeapon.PseudoStats.DamageDealtMultiplier *= runeWeapon.dkOwner.drwDmgSnapshot

	runeWeapon.dkOwner.drwPhysSnapshot = runeWeapon.dkOwner.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]
	runeWeapon.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= runeWeapon.dkOwner.drwPhysSnapshot
}

func (runeWeapon *RuneWeaponPet) disable(_ *core.Simulation) {
	// Clear snapshot damage multipliers
	runeWeapon.dkOwner.RuneWeapon.PseudoStats.DamageDealtMultiplier /= runeWeapon.dkOwner.drwDmgSnapshot
	runeWeapon.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] /= runeWeapon.dkOwner.drwPhysSnapshot
	runeWeapon.dkOwner.drwPhysSnapshot = 1
	runeWeapon.dkOwner.drwDmgSnapshot = 1
}
