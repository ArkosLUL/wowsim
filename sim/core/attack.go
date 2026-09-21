package core

import (
	"slices"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// ReplaceMHSwing is called right before a main hand auto attack fires.
// It must never return nil, but either a replacement spell or the passed in regular mhSwingSpell.
// This allows for abilities that convert a white attack into a yellow attack.
type ReplaceMHSwing func(sim *Simulation, mhSwingSpell *Spell) *Spell

// Represents a generic weapon. Pets / unarmed / various other cases don't use
// actual weapon items so this is an abstraction of a Weapon.
type Weapon struct {
	BaseDamageMin        float64
	BaseDamageMax        float64
	AttackPowerPerDPS    float64
	SwingSpeed           float64
	NormalizedSwingSpeed float64
	CritMultiplier       float64
	SpellSchool          SpellSchool
}

func (weapon *Weapon) DPS() float64 {
	if weapon.SwingSpeed == 0 {
		return 0
	}
	return (weapon.BaseDamageMin + weapon.BaseDamageMax) / 2.0 / weapon.SwingSpeed
}

func newWeaponFromUnarmed(critMultiplier float64) Weapon {
	// These numbers are probably wrong but nobody cares.
	return Weapon{
		BaseDamageMin:        0,
		BaseDamageMax:        0,
		SwingSpeed:           1,
		NormalizedSwingSpeed: 1,
		CritMultiplier:       critMultiplier,
		AttackPowerPerDPS:    DefaultAttackPowerPerDPS,
	}
}

func newWeaponFromItem(item *Item, critMultiplier float64, bonusDps float64) Weapon {
	normalizedWeaponSpeed := 2.4
	if item.WeaponType == proto.WeaponType_WeaponTypeDagger {
		normalizedWeaponSpeed = 1.7
	} else if item.HandType == proto.HandType_HandTypeTwoHand {
		normalizedWeaponSpeed = 3.3
	} else if item.RangedWeaponType != proto.RangedWeaponType_RangedWeaponTypeUnknown {
		normalizedWeaponSpeed = 2.8
	}

	return Weapon{
		BaseDamageMin:        item.WeaponDamageMin + bonusDps*item.SwingSpeed,
		BaseDamageMax:        item.WeaponDamageMax + bonusDps*item.SwingSpeed,
		SwingSpeed:           item.SwingSpeed,
		NormalizedSwingSpeed: normalizedWeaponSpeed,
		CritMultiplier:       critMultiplier,
		AttackPowerPerDPS:    DefaultAttackPowerPerDPS,
	}
}

// Returns weapon stats using the main hand equipped weapon.
func (character *Character) WeaponFromMainHand(critMultiplier float64) Weapon {
	if weapon := character.GetMHWeapon(); weapon != nil {
		return newWeaponFromItem(weapon, critMultiplier, character.PseudoStats.BonusMHDps)
	} else {
		return newWeaponFromUnarmed(critMultiplier)
	}
}

// Returns weapon stats using the off-hand equipped weapon.
func (character *Character) WeaponFromOffHand(critMultiplier float64) Weapon {
	if weapon := character.GetOHWeapon(); weapon != nil {
		return newWeaponFromItem(weapon, critMultiplier, character.PseudoStats.BonusOHDps)
	} else {
		return Weapon{}
	}
}

// Returns weapon stats using the ranged equipped weapon.
func (character *Character) WeaponFromRanged(critMultiplier float64) Weapon {
	if weapon := character.GetRangedWeapon(); weapon != nil {
		return newWeaponFromItem(weapon, critMultiplier, character.PseudoStats.BonusRangedDps)
	} else {
		return Weapon{}
	}
}

func (weapon *Weapon) GetSpellSchool() SpellSchool {
	if weapon.SpellSchool == SpellSchoolNone {
		return SpellSchoolPhysical
	} else {
		return weapon.SpellSchool
	}
}

func (weapon *Weapon) EnemyWeaponDamage(sim *Simulation, attackPower float64, damageSpread float64) float64 {
	// Maximum damage range is 133% of minimum damage; AP contribution is % of minimum damage roll.
	// Patchwerk follows special damage range rules.
	// TODO: Scrape more logs to determine these values more accurately. AP defined in constants.go

	rand := 1 + damageSpread*sim.RandomFloat("Enemy Weapon Damage")

	return weapon.BaseDamageMin * (rand + attackPower*EnemyAutoAttackAPCoefficient)
}

func (weapon *Weapon) BaseDamage(sim *Simulation) float64 {
	return weapon.BaseDamageMin + (weapon.BaseDamageMax-weapon.BaseDamageMin)*sim.RandomFloat("Weapon Base Damage")
}

func (weapon *Weapon) AverageDamage() float64 {
	return (weapon.BaseDamageMin + weapon.BaseDamageMax) / 2
}

func (weapon *Weapon) CalculateWeaponDamage(sim *Simulation, attackPower float64) float64 {
	return weapon.BaseDamage(sim) + (weapon.SwingSpeed*attackPower)/weapon.AttackPowerPerDPS
}

func (weapon *Weapon) CalculateAverageWeaponDamage(attackPower float64) float64 {
	return weapon.AverageDamage() + (weapon.SwingSpeed*attackPower)/weapon.AttackPowerPerDPS
}

func (weapon *Weapon) CalculateNormalizedWeaponDamage(sim *Simulation, attackPower float64) float64 {
	return weapon.BaseDamage(sim) + (weapon.NormalizedSwingSpeed*attackPower)/weapon.AttackPowerPerDPS
}

func (unit *Unit) MHWeaponDamage(sim *Simulation, attackPower float64) float64 {
	return unit.AutoAttacks.mh.CalculateWeaponDamage(sim, attackPower)
}
func (unit *Unit) MHNormalizedWeaponDamage(sim *Simulation, attackPower float64) float64 {
	return unit.AutoAttacks.mh.CalculateNormalizedWeaponDamage(sim, attackPower)
}

func (unit *Unit) OHWeaponDamage(sim *Simulation, attackPower float64) float64 {
	return 0.5 * unit.AutoAttacks.oh.CalculateWeaponDamage(sim, attackPower)
}
func (unit *Unit) OHNormalizedWeaponDamage(sim *Simulation, attackPower float64) float64 {
	return 0.5 * unit.AutoAttacks.oh.CalculateNormalizedWeaponDamage(sim, attackPower)
}

func (unit *Unit) RangedWeaponDamage(sim *Simulation, attackPower float64) float64 {
	return unit.AutoAttacks.ranged.CalculateWeaponDamage(sim, attackPower)
}

type MeleeDamageCalculator func(attackPower float64, bonusWeaponDamage float64) float64

// Returns whether this hit effect is associated with the main-hand weapon.
func (spell *Spell) IsMH() bool {
	return spell.ProcMask.Matches(ProcMaskMeleeMH)
}

// Returns whether this hit effect is associated with the off-hand weapon.
func (spell *Spell) IsOH() bool {
	return spell.ProcMask.Matches(ProcMaskMeleeOH)
}

// Returns whether this hit effect is associated with either melee weapon.
func (spell *Spell) IsMelee() bool {
	return spell.ProcMask.Matches(ProcMaskMelee)
}

func (aa *AutoAttacks) MH() *Weapon {
	return aa.mh.getWeapon()
}

func (aa *AutoAttacks) SetMH(weapon Weapon) {
	aa.mh.setWeapon(weapon)
}

func (aa *AutoAttacks) OH() *Weapon {
	return aa.oh.getWeapon()
}

func (aa *AutoAttacks) SetOH(weapon Weapon) {
	aa.oh.setWeapon(weapon)
}

func (aa *AutoAttacks) Ranged() *Weapon {
	return aa.ranged.getWeapon()
}

func (aa *AutoAttacks) SetRanged(weapon Weapon) {
	aa.ranged.setWeapon(weapon)
}

func (aa *AutoAttacks) MHAuto() *Spell {
	return aa.mh.spell
}

func (aa *AutoAttacks) OHAuto() *Spell {
	return aa.oh.spell
}

func (aa *AutoAttacks) RangedAuto() *Spell {
	return aa.ranged.spell
}

func (aa *AutoAttacks) OffhandSwingAt() time.Duration {
	return aa.oh.swingAt
}

// SetOffhandSwingAt has no sim to round to a server tick with, so swing() does that.
func (aa *AutoAttacks) SetOffhandSwingAt(offhandSwingAt time.Duration) {
	aa.oh.timerAt = offhandSwingAt
	aa.oh.swingAt = offhandSwingAt
	aa.oh.held = false
}

func (aa *AutoAttacks) SetReplaceMHSwing(replaceSwing ReplaceMHSwing) {
	aa.mh.replaceSwing = replaceSwing
}

func (aa *AutoAttacks) MHConfig() *SpellConfig {
	return &aa.mh.config
}

func (aa *AutoAttacks) OHConfig() *SpellConfig {
	return &aa.oh.config
}

func (aa *AutoAttacks) RangedConfig() *SpellConfig {
	return &aa.ranged.config
}

type WeaponAttack struct {
	Weapon

	agent Agent
	unit  *Unit

	config SpellConfig
	spell  *Spell

	replaceSwing ReplaceMHSwing

	// timerAt is when the swing timer (m_attackTimer) runs out, swingAt the server tick the swing
	// lands on. Haste and the other timer changes work on timerAt.
	timerAt time.Duration
	swingAt time.Duration

	// ran out mid-cast and waits for the cast to land, see castHold
	held bool

	// ranged only: the timer stands still for a channel (SuspendRangedTimer), with suspendedLeft to go
	// once it runs again. Parked while timerAt is NeverExpires.
	suspended     bool
	suspendedLeft time.Duration

	curSwingSpeed    float64
	curSwingDuration time.Duration
}

func (wa *WeaponAttack) getWeapon() *Weapon {
	return &wa.Weapon
}

// setTimer doesn't reschedule: callers that move a running swing earlier call rescheduleWeaponAttack.
func (wa *WeaponAttack) setTimer(sim *Simulation, timerAt time.Duration) {
	wa.timerAt = timerAt
	wa.swingAt = sim.NextServerTick(timerAt)
	wa.held = false
}

func (wa *WeaponAttack) stopTimer() {
	wa.timerAt = NeverExpires
	wa.swingAt = NeverExpires
	wa.held = false
	wa.suspended = false
}

// park stops a suspended timer from running out, keeping what's left of it as of the update at from.
func (wa *WeaponAttack) park(from time.Duration) {
	wa.suspendedLeft = wa.timerAt - from
	wa.timerAt = NeverExpires
	wa.swingAt = NeverExpires
	wa.held = false
}

func (wa *WeaponAttack) setWeapon(weapon Weapon) {
	wa.Weapon = weapon
	wa.spell.CritMultiplier = weapon.CritMultiplier
	wa.updateSwingDuration(wa.curSwingSpeed)
}

// inlineable stub for swing
func (wa *WeaponAttack) trySwing(sim *Simulation) time.Duration {
	if sim.CurrentTime < wa.swingAt {
		return wa.swingAt
	}
	return wa.swing(sim)
}

func (wa *WeaponAttack) swing(sim *Simulation) time.Duration {
	// SetOffhandSwingAt can leave a swing between server ticks
	if at := sim.NextServerTick(wa.swingAt); at > sim.CurrentTime {
		wa.swingAt = at
		return at
	}

	aa := &wa.unit.AutoAttacks
	if until, held := wa.castHold(sim); held {
		return wa.hold(until)
	}

	var otherHand *WeaponAttack
	switch wa {
	case &aa.mh:
		otherHand = &aa.oh
	case &aa.oh:
		// the server checks the main hand first, so on a shared tick it swings and pushes this one back
		if aa.mh.swingAt <= sim.CurrentTime {
			return sim.CurrentTime
		}
		otherHand = &aa.mh
	}

	if wa.replaceSwing != nil {
		// Need to check APL here to allow last-moment HS queue casts.
		wa.unit.Rotation.DoNextAction(sim)

		// the server takes casts before it gets to melee, so one started just now can restart, pause
		// or hold this swing
		if wa.swingAt > sim.CurrentTime {
			return wa.swingAt
		}
		if until, held := wa.castHold(sim); held {
			return wa.hold(until)
		}
	}

	if otherHand != nil && aa.IsDualWielding && otherHand.timerAt-sim.CurrentTime < attackDisplayDelay {
		otherHand.setTimer(sim, sim.CurrentTime+attackDisplayDelay)
	}

	attackSpell := wa.spell
	// Need to check this again in case the DoNextAction call swapped items.
	if wa.replaceSwing != nil {
		// Allow MH swing to be overridden for abilities like Heroic Strike.
		attackSpell = wa.replaceSwing(sim, attackSpell)
	}

	// Update swing timer BEFORE the cast, so that APL checks for TimeToNextAuto behave correctly
	// if the attack causes APL evaluations (e.g. from rage gain).
	// Restarts from this tick, so whatever the timer overshot is lost.
	// Auto Shot fires in _UpdateSpells, one update after its timer runs out, but it also restarts the
	// timer before that update's decrement, so it still lands on NextServerTick(timerAt) like melee.
	wa.setTimer(sim, sim.CurrentTime+wa.curSwingDuration)
	if wa.suspended {
		wa.park(sim.CurrentTime)
	}
	// a melee swing restarts the ranged timer too; a restart can land earlier than a pushed-back timer
	if otherHand != nil && aa.AutoSwingRanged {
		aa.ranged.setTimer(sim, sim.CurrentTime+aa.ranged.curSwingDuration)
		sim.rescheduleWeaponAttack(aa.ranged.swingAt)
	}
	// and every Auto Shot restarts both melee timers (Unit::_UpdateAutoRepeatSpell)
	if wa == &aa.ranged && aa.AutoSwingMelee {
		aa.mh.setTimer(sim, sim.CurrentTime+aa.mh.curSwingDuration)
		sim.rescheduleWeaponAttack(aa.mh.swingAt)
		if aa.IsDualWielding {
			aa.oh.setTimer(sim, sim.CurrentTime+aa.oh.curSwingDuration)
			sim.rescheduleWeaponAttack(aa.oh.swingAt)
		}
	}
	attackSpell.Cast(sim, wa.unit.CurrentTarget)

	if !sim.Options.Interactive && wa.unit.Rotation != nil {
		wa.unit.Rotation.DoNextAction(sim)
	}

	return wa.swingAt
}

// castHold is whether a cast in progress holds this swing, and until when. Player::Update skips melee
// while UNIT_STATE_CASTING is set (UnitAI::DoMeleeAttackIfReady for creatures), and Auto Shot waits out
// every cast but a SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS one (Unit::IsNonMeleeSpellCast). The timer
// keeps running meanwhile, so the swing goes on the update the cast lands.
func (wa *WeaponAttack) castHold(sim *Simulation) (time.Duration, bool) {
	hc := &wa.unit.Hardcast
	// still casting at Expires itself: weapon attacks run before the pending action that lands it
	if hc.OnComplete == nil || hc.Expires == startingCDTime || hc.Expires < sim.CurrentTime {
		return 0, false
	}
	// Even Auto Shot waits on the update the cast lands, since spell events run before
	// Unit::_UpdateSpells. Otherwise the shot's APL call could start a new cast over this one.
	if wa == &wa.unit.AutoAttacks.ranged && hc.keepsCombatTimers && hc.Expires > sim.CurrentTime {
		return 0, false
	}
	return hc.Expires, true
}

// hold parks the swing until the cast lands, where releaseCastHold swings it. The +1 ns wake only
// matters if nothing completes the cast, and keeps the tie at until from looping.
func (wa *WeaponAttack) hold(until time.Duration) time.Duration {
	wa.held = true
	wa.swingAt = until
	return until + 1
}

func (wa *WeaponAttack) updateSwingDuration(curSwingSpeed float64) {
	wa.curSwingSpeed = curSwingSpeed
	wa.curSwingDuration = DurationFromSeconds(wa.SwingSpeed / wa.curSwingSpeed)
}

func (wa *WeaponAttack) addWeaponAttack(sim *Simulation, swingSpeed float64) {
	wa.updateSwingDuration(swingSpeed)
	sim.addWeaponAttack(wa)
	sim.rescheduleWeaponAttack(wa.swingAt)
}

type AutoAttacks struct {
	AutoSwingMelee  bool
	AutoSwingRanged bool

	IsDualWielding bool

	mh     WeaponAttack
	oh     WeaponAttack
	ranged WeaponAttack

	enabled bool
}

// Options for initializing auto attacks.
type AutoAttackOptions struct {
	MainHand        Weapon
	OffHand         Weapon
	Ranged          Weapon
	AutoSwingMelee  bool // If true, core engine will handle calling SwingMelee() for you.
	AutoSwingRanged bool // If true, core engine will handle calling SwingRanged() for you.
	ReplaceMHSwing  ReplaceMHSwing
}

func (unit *Unit) EnableAutoAttacks(agent Agent, options AutoAttackOptions) {
	if options.MainHand.AttackPowerPerDPS == 0 {
		options.MainHand.AttackPowerPerDPS = DefaultAttackPowerPerDPS
	}
	if options.OffHand.AttackPowerPerDPS == 0 {
		options.OffHand.AttackPowerPerDPS = DefaultAttackPowerPerDPS
	}

	unit.AutoAttacks = AutoAttacks{
		AutoSwingMelee:  options.AutoSwingMelee,
		AutoSwingRanged: options.AutoSwingRanged,

		IsDualWielding: options.OffHand.SwingSpeed != 0,

		mh: WeaponAttack{
			agent:        agent,
			unit:         unit,
			Weapon:       options.MainHand,
			replaceSwing: options.ReplaceMHSwing,
		},
		oh: WeaponAttack{
			agent:  agent,
			unit:   unit,
			Weapon: options.OffHand,
		},
		ranged: WeaponAttack{
			agent:  agent,
			unit:   unit,
			Weapon: options.Ranged,
		},
	}

	unit.AutoAttacks.mh.config = SpellConfig{
		ActionID:    ActionID{OtherID: proto.OtherAction_OtherActionAttack, Tag: 1},
		SpellSchool: options.MainHand.GetSpellSchool(),
		ProcMask:    ProcMaskMeleeMHAuto,
		Flags:       SpellFlagMeleeMetrics | SpellFlagIncludeTargetBonusDamage | SpellFlagNoOnCastComplete,

		DamageMultiplier: 1,
		CritMultiplier:   options.MainHand.CritMultiplier,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *Simulation, target *Unit, spell *Spell) {
			baseDamage := spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower()) +
				spell.BonusWeaponDamage()

			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWhite)
		},
	}

	unit.AutoAttacks.oh.config = SpellConfig{
		ActionID:    ActionID{OtherID: proto.OtherAction_OtherActionAttack, Tag: 2},
		SpellSchool: options.OffHand.GetSpellSchool(),
		ProcMask:    ProcMaskMeleeOHAuto,
		Flags:       SpellFlagMeleeMetrics | SpellFlagIncludeTargetBonusDamage | SpellFlagNoOnCastComplete,

		DamageMultiplier: 1,
		CritMultiplier:   options.OffHand.CritMultiplier,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *Simulation, target *Unit, spell *Spell) {
			baseDamage := spell.Unit.OHWeaponDamage(sim, spell.MeleeAttackPower()) +
				spell.BonusWeaponDamage()

			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWhite)
		},
	}

	unit.AutoAttacks.ranged.config = SpellConfig{
		ActionID:    ActionID{OtherID: proto.OtherAction_OtherActionShoot},
		SpellSchool: options.Ranged.GetSpellSchool(),
		ProcMask:    ProcMaskRangedAuto,
		Flags:       SpellFlagMeleeMetrics | SpellFlagIncludeTargetBonusDamage,

		DamageMultiplier: 1,
		CritMultiplier:   options.Ranged.CritMultiplier,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *Simulation, target *Unit, spell *Spell) {
			baseDamage := spell.Unit.RangedWeaponDamage(sim, spell.RangedAttackPower(target)) +
				spell.BonusWeaponDamage()
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
		},
	}

	if unit.Type == EnemyUnit {
		unit.AutoAttacks.mh.config.ApplyEffects = func(sim *Simulation, target *Unit, spell *Spell) {
			ap := max(0, spell.Unit.stats[stats.AttackPower])
			baseDamage := spell.Unit.AutoAttacks.mh.EnemyWeaponDamage(sim, ap, spell.Unit.PseudoStats.DamageSpread)

			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeEnemyMeleeWhite)
		}
		unit.AutoAttacks.oh.config.ApplyEffects = func(sim *Simulation, target *Unit, spell *Spell) {
			ap := max(0, spell.Unit.stats[stats.AttackPower])
			baseDamage := spell.Unit.AutoAttacks.mh.EnemyWeaponDamage(sim, ap, spell.Unit.PseudoStats.DamageSpread) * 0.5

			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeEnemyMeleeWhite)
		}
	}
}

func (aa *AutoAttacks) finalize() {
	if aa.AutoSwingMelee {
		aa.mh.spell = aa.mh.unit.GetOrRegisterSpell(aa.mh.config)

		// Will keep keep the OH spell registered for Item swapping
		aa.oh.spell = aa.oh.unit.GetOrRegisterSpell(aa.oh.config)
	}
	if aa.AutoSwingRanged {
		aa.ranged.spell = aa.ranged.unit.GetOrRegisterSpell(aa.ranged.config)
	}
}

func (aa *AutoAttacks) reset(sim *Simulation) {
	if !aa.AutoSwingMelee && !aa.AutoSwingRanged {
		return
	}

	aa.enabled = false

	aa.mh.stopTimer()
	aa.oh.stopTimer()

	if aa.AutoSwingMelee {
		aa.mh.updateSwingDuration(aa.mh.unit.SwingSpeed())
		aa.mh.setTimer(sim, 0)

		if aa.IsDualWielding {
			aa.oh.updateSwingDuration(aa.mh.curSwingSpeed)
			// startPull staggers it behind the main hand
			aa.oh.setTimer(sim, 0)
		}
	}

	aa.ranged.stopTimer()

	if aa.AutoSwingRanged {
		aa.ranged.updateSwingDuration(aa.ranged.unit.RangedSwingSpeed())
		aa.ranged.setTimer(sim, 0)
	}
}

func (aa *AutoAttacks) startPull(sim *Simulation) {
	if !aa.AutoSwingMelee && !aa.AutoSwingRanged {
		return
	}

	if aa.enabled {
		return
	}

	aa.enabled = true

	if aa.AutoSwingMelee {
		aa.mh.addWeaponAttack(sim, aa.mh.unit.SwingSpeed())
		if aa.IsDualWielding {
			// Unit::Attack: a ready off hand waits half the main hand's hasted attack time behind it
			if aa.oh.timerAt <= sim.CurrentTime {
				aa.oh.setTimer(sim, max(aa.oh.timerAt, aa.mh.timerAt+aa.mh.curSwingDuration/2))
			}
			aa.oh.addWeaponAttack(sim, aa.mh.curSwingSpeed)
		}
	}

	if aa.AutoSwingRanged {
		aa.ranged.addWeaponAttack(sim, aa.ranged.unit.RangedSwingSpeed())
	}
}

// Stops the auto swing action for the rest of the iteration. Used for pets
// after being disabled.
func (aa *AutoAttacks) CancelAutoSwing(sim *Simulation) {
	if !aa.AutoSwingMelee && !aa.AutoSwingRanged {
		return
	}

	if !aa.enabled {
		return
	}

	aa.enabled = false

	if aa.AutoSwingMelee {
		sim.removeWeaponAttack(&aa.mh)
		if aa.IsDualWielding {
			sim.removeWeaponAttack(&aa.oh)
		}
	}

	if aa.AutoSwingRanged {
		sim.removeWeaponAttack(&aa.ranged)
	}
}

// Re-enables the auto swing action for the iteration
func (aa *AutoAttacks) EnableAutoSwing(sim *Simulation) {
	if !aa.AutoSwingMelee && !aa.AutoSwingRanged {
		return
	}

	if aa.enabled {
		return
	}

	aa.enabled = true

	if aa.AutoSwingMelee {
		aa.mh.setTimer(sim, max(aa.mh.timerAt, sim.CurrentTime, 0))
		aa.mh.addWeaponAttack(sim, aa.mh.unit.SwingSpeed())
		if aa.IsDualWielding {
			aa.oh.setTimer(sim, max(aa.oh.timerAt, sim.CurrentTime, 0))
			aa.oh.addWeaponAttack(sim, aa.mh.unit.SwingSpeed())
		}
	}

	if aa.AutoSwingRanged {
		aa.ranged.setTimer(sim, max(aa.ranged.timerAt, sim.CurrentTime, 0))
		aa.ranged.addWeaponAttack(sim, aa.ranged.unit.RangedSwingSpeed())
	}
}

// The amount of time between two MH swings.
func (aa *AutoAttacks) MainhandSwingSpeed() time.Duration {
	return aa.mh.curSwingDuration
}

// The amount of time between two OH swings.
func (aa *AutoAttacks) OffhandSwingSpeed() time.Duration {
	return aa.oh.curSwingDuration
}

// Optionally replaces the given swing spell with an Agent-specified MH Swing replacer.
// This is for effects like Heroic Strike or Raptor Strike.
func (aa *AutoAttacks) MaybeReplaceMHSwing(sim *Simulation, mhSwingSpell *Spell) *Spell {
	if aa.mh.replaceSwing == nil {
		return mhSwingSpell
	}

	// Allow MH swing to be overridden for abilities like Heroic Strike.
	return aa.mh.replaceSwing(sim, mhSwingSpell)
}

func (aa *AutoAttacks) UpdateSwingTimers(sim *Simulation) {
	if !aa.enabled {
		return
	}

	if aa.AutoSwingRanged {
		oldSwingSpeed := aa.ranged.curSwingSpeed
		aa.ranged.updateSwingDuration(aa.ranged.unit.RangedSwingSpeed())
		// Unit::ApplyAttackTimePercentMod keeps the remaining fraction of the ranged timer too
		f := oldSwingSpeed / aa.ranged.curSwingSpeed
		switch remainingSwingTime := aa.ranged.timerAt - sim.CurrentTime; {
		case aa.ranged.timerAt == NeverExpires:
			if aa.ranged.suspended {
				aa.ranged.suspendedLeft = time.Duration(float64(aa.ranged.suspendedLeft) * f)
			}
		case remainingSwingTime > 0:
			aa.ranged.setTimer(sim, sim.CurrentTime+time.Duration(float64(remainingSwingTime)*f))
			sim.rescheduleWeaponAttack(aa.ranged.swingAt)
		}
	}

	if aa.AutoSwingMelee {
		oldSwingSpeed := aa.mh.curSwingSpeed
		aa.mh.updateSwingDuration(aa.mh.unit.SwingSpeed())
		f := oldSwingSpeed / aa.mh.curSwingSpeed

		if remainingSwingTime := aa.mh.timerAt - sim.CurrentTime; remainingSwingTime > 0 {
			aa.mh.setTimer(sim, sim.CurrentTime+time.Duration(float64(remainingSwingTime)*f))
		}

		sim.rescheduleWeaponAttack(aa.mh.swingAt)

		if aa.IsDualWielding {
			aa.oh.updateSwingDuration(aa.mh.curSwingSpeed)

			if remainingSwingTime := aa.oh.timerAt - sim.CurrentTime; remainingSwingTime > 0 {
				aa.oh.setTimer(sim, sim.CurrentTime+time.Duration(float64(remainingSwingTime)*f))
			}

			sim.rescheduleWeaponAttack(aa.oh.swingAt)
		}
	}
}

// StopMeleeUntil should be used whenever a non-melee spell is cast. It stops melee, then restarts it
// at end of cast, but with a reset swing timer (as if swings had just landed).
func (aa *AutoAttacks) StopMeleeUntil(sim *Simulation, readyAt time.Duration, desyncOH bool) {
	if !aa.AutoSwingMelee { // if not auto swinging, don't auto restart.
		return
	}

	aa.mh.setTimer(sim, readyAt+aa.mh.curSwingDuration)
	sim.rescheduleWeaponAttack(aa.mh.swingAt)

	if aa.IsDualWielding {
		ohTimerAt := readyAt + aa.oh.curSwingDuration
		if desyncOH {
			// Used by warrior to desync offhand after unglyphed Shattering Throw.
			ohTimerAt += aa.oh.curSwingDuration / 2
		}
		aa.oh.setTimer(sim, ohTimerAt)
		sim.rescheduleWeaponAttack(aa.oh.swingAt)
	}
}

// resetSwingTimers is Unit::resetAttackTimer on every hand, ranged included, as a cast that resets
// autos does: each timer restarts in full at readyAt.
func (aa *AutoAttacks) resetSwingTimers(sim *Simulation, readyAt time.Duration) {
	if aa.AutoSwingMelee {
		aa.mh.setTimer(sim, readyAt+aa.mh.curSwingDuration)
		sim.rescheduleWeaponAttack(aa.mh.swingAt)
		if aa.IsDualWielding {
			aa.oh.setTimer(sim, readyAt+aa.oh.curSwingDuration)
			sim.rescheduleWeaponAttack(aa.oh.swingAt)
		}
	}
	if aa.AutoSwingRanged {
		aa.ranged.setTimer(sim, readyAt+aa.ranged.curSwingDuration)
		sim.rescheduleWeaponAttack(aa.ranged.swingAt)
	}
}

// pauseMelee pushes both melee timers back by pause, which only ever moves a swing later, so there's
// nothing to reschedule.
func (aa *AutoAttacks) pauseMelee(sim *Simulation, pause time.Duration) {
	if !aa.AutoSwingMelee || pause <= 0 {
		return
	}
	for _, wa := range []*WeaponAttack{&aa.mh, &aa.oh} {
		if (wa == &aa.oh && !aa.IsDualWielding) || wa.timerAt == NeverExpires {
			continue
		}
		wa.setTimer(sim, wa.timerAt+pause)
	}
}

// releaseCastHold swings what a cast held, right as it lands and before anything cast after it. Ranged
// goes first, as Unit::_UpdateSpells runs Auto Shot before Player::Update gets to melee.
func (aa *AutoAttacks) releaseCastHold(sim *Simulation) {
	for _, wa := range []*WeaponAttack{&aa.ranged, &aa.mh, &aa.oh} {
		if !wa.held {
			continue
		}
		// an earlier hand's swing can restart this one, which clears held
		wa.held = false
		if aa.enabled {
			sim.rescheduleWeaponAttack(wa.trySwing(sim))
		}
	}
}

// SuspendRangedTimer is Unit::Update's suspendRangedAttackTimer, for a player channeling a spell with
// SPELL_ATTR2_DO_NOT_RESET_COMBAT_TIMERS (Volley): the ranged timer skips its decrement every update
// the channel is active, starting with the one that takes the cast. Auto Shot isn't held, so a shot
// already due still goes, and its restarted timer stands still too.
func (aa *AutoAttacks) SuspendRangedTimer(sim *Simulation) {
	wa := &aa.ranged
	if !aa.AutoSwingRanged || wa.suspended {
		return
	}
	wa.suspended = true
	if from := sim.NextServerTick(sim.CurrentTime); wa.swingAt > from && wa.timerAt != NeverExpires {
		wa.park(from)
	}
}

// ResumeRangedTimer lets the ranged timer run again as the channel ends. The update the channel ends
// on already decrements it, the way a timer restarted before Unit::Update's decrement counts.
func (aa *AutoAttacks) ResumeRangedTimer(sim *Simulation) {
	wa := &aa.ranged
	if !wa.suspended {
		return
	}
	wa.suspended = false
	if wa.timerAt != NeverExpires {
		return
	}
	wa.setTimer(sim, sim.CurrentTime+wa.suspendedLeft)
	if aa.enabled {
		sim.rescheduleWeaponAttack(wa.swingAt)
	}
}

func (aa *AutoAttacks) DelayRangedUntil(sim *Simulation, readyAt time.Duration) {
	if readyAt <= aa.ranged.timerAt {
		return
	}

	aa.ranged.setTimer(sim, readyAt)
	sim.rescheduleWeaponAttack(aa.ranged.swingAt)
}

// Returns the time at which the next attack will occur.
func (aa *AutoAttacks) NextAttackAt() time.Duration {
	return min(aa.mh.swingAt, aa.oh.swingAt)
}

type PPMManager struct {
	procMasks   []ProcMask
	procChances []float64
}

// Returns whether the effect procced.
func (ppmm *PPMManager) Proc(sim *Simulation, procMask ProcMask, label string) bool {
	for i, m := range ppmm.procMasks {
		if m.Matches(procMask) {
			return sim.RandomFloat(label) < ppmm.procChances[i]
		}
	}
	return false
}

func (ppmm *PPMManager) Chance(procMask ProcMask) float64 {
	for i, m := range ppmm.procMasks {
		if m.Matches(procMask) {
			return ppmm.procChances[i]
		}
	}
	return 0
}

func (aa *AutoAttacks) NewPPMManager(ppm float64, procMask ProcMask) PPMManager {
	if !aa.AutoSwingMelee && !aa.AutoSwingRanged {
		return PPMManager{}
	}

	ppmm := PPMManager{procMasks: make([]ProcMask, 0, 2), procChances: make([]float64, 0, 2)}

	mergeOrAppend := func(speed float64, mask ProcMask) {
		if speed == 0 || mask == 0 {
			return
		}

		if i := slices.Index(ppmm.procChances, speed); i != -1 {
			ppmm.procMasks[i] |= mask
			return
		}

		ppmm.procMasks = append(ppmm.procMasks, mask)
		ppmm.procChances = append(ppmm.procChances, speed)
	}

	mergeOrAppend(aa.mh.SwingSpeed, procMask&^ProcMaskRanged&^ProcMaskMeleeOH) // "everything else", even if not explicitly flagged MH
	mergeOrAppend(aa.oh.SwingSpeed, procMask&ProcMaskMeleeOH)
	mergeOrAppend(aa.ranged.SwingSpeed, procMask&ProcMaskRanged)

	for i := range ppmm.procChances {
		ppmm.procChances[i] *= ppm / 60
	}

	return ppmm
}

// Returns whether a PPM-based effect procced.
// Using NewPPMManager() is preferred; this function should only be used when
// the attacker is not known at initialization time.
func (aa *AutoAttacks) PPMProc(sim *Simulation, ppm float64, procMask ProcMask, label string, spell *Spell) bool {
	if !aa.AutoSwingMelee && !aa.AutoSwingRanged {
		return false
	}

	switch {
	case spell.ProcMask.Matches(procMask &^ ProcMaskMeleeOH &^ ProcMaskRanged):
		return sim.RandomFloat(label) < ppm*aa.mh.SwingSpeed/60.0
	case spell.ProcMask.Matches(procMask & ProcMaskMeleeOH):
		return sim.RandomFloat(label) < ppm*aa.oh.SwingSpeed/60.0
	case spell.ProcMask.Matches(procMask & ProcMaskRanged):
		return sim.RandomFloat(label) < ppm*aa.ranged.SwingSpeed/60.0
	}
	return false
}

func (unit *Unit) applyParryHaste() {
	if !unit.PseudoStats.ParryHaste || !unit.AutoAttacks.AutoSwingMelee {
		return
	}

	unit.RegisterAura(Aura{
		Label:    "Parry Haste",
		Duration: NeverExpires,
		OnReset: func(aura *Aura, sim *Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitTaken: func(aura *Aura, sim *Simulation, spell *Spell, result *SpellResult) {
			if !result.Outcome.Matches(OutcomeParry) {
				return
			}

			mh := &aura.Unit.AutoAttacks.mh
			remainingTime := mh.timerAt - sim.CurrentTime
			swingSpeed := mh.curSwingDuration
			minRemainingTime := time.Duration(float64(swingSpeed) * 0.2) // 20% of Swing Speed
			defaultReduction := minRemainingTime * 2                     // 40% of Swing Speed

			if remainingTime <= minRemainingTime {
				return
			}

			parryHasteReduction := min(defaultReduction, remainingTime-minRemainingTime)
			mh.setTimer(sim, mh.timerAt-parryHasteReduction)
			if sim.Log != nil {
				aura.Unit.Log(sim, "MH Swing reduced by %s due to parry haste, will now occur at %s", parryHasteReduction, mh.swingAt)
			}

			sim.rescheduleWeaponAttack(mh.swingAt)
		},
	})
}
