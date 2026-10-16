package core

import (
	"fmt"
	"slices"
)

// AddHealingDonePct puts a SPELL_AURA_MOD_HEALING_DONE_PERCENT (aura 136) aura of pct percent on the unit, and
// RemoveHealingDonePct takes it off. Only a hot's tick reads them (Dot.SnapshotHotMultiplier): a class's direct
// heals carry their healing done talents in their own multipliers.
func (unit *Unit) AddHealingDonePct(pct int32) {
	if pct != 0 {
		unit.healingDonePcts = append(unit.healingDonePcts, pct)
	}
}

func (unit *Unit) RemoveHealingDonePct(pct int32) {
	if pct == 0 {
		return
	}
	i := slices.Index(unit.healingDonePcts, pct)
	if i == -1 {
		panic(fmt.Sprintf("%s has no %d%% healing done aura to take off", unit.Label, pct))
	}
	unit.healingDonePcts = slices.Delete(unit.healingDonePcts, i, i+1)
}

// healingDoneMultiplier is Unit::GetTotalAuraMultiplier(SPELL_AURA_MOD_HEALING_DONE_PERCENT): each aura's AddPct
// in turn, in float32.
func (unit *Unit) healingDoneMultiplier() float32 {
	m := float32(1)
	for _, pct := range unit.healingDonePcts {
		// explicit float32 so Go can't fuse it into an FMA: the server rounds the product on its own
		m += float32(m*float32(pct)) / 100
	}
	return m
}

// SnapshotHotMultiplier sets the hot's caster multiplier to what its aura's amount carries, without the healing
// done percents: AuraEffect::HandlePeriodicHealAurasTick takes the caster's at each tick, after the whole amount.
func (dot *Dot) SnapshotHotMultiplier(multiplier float64) {
	dot.SnapshotAttackerMultiplier = multiplier
	dot.healingDoneAtTick = true
}

// hotTick is a tick of a hot set by SnapshotHotMultiplier before the target's terms: the aura's whole amount,
// then int32(float(amount) × the caster's healing done multiplier).
func (dot *Dot) hotTick() float64 {
	amount := wholeHeal(dot.SnapshotBaseDamage * dot.SnapshotAttackerMultiplier)
	return float64(int32(float32(float32(amount) * dot.Spell.Unit.healingDoneMultiplier())))
}
