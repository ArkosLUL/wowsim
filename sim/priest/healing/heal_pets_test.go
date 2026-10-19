package healing

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
)

// Acore::MostHPMissingGroupInRange takes a pet only if it's Unit::IsPet, so a Shadowfiend, a guardian, is never
// jumped to however hurt. A real pet (SummonedAsPet: a hunter's or a warlock's) is.
func TestPrayerOfMendingJumpsToPetsOnly(t *testing.T) {
	r := newHealRaid(t, 2)
	caster := r.priests[0]
	label := "PrayerOfMending" + strconv.Itoa(int(caster.Index))
	first, holder := r.units[0], r.units[1]
	fiend := caster.ShadowfiendPet
	fiend.EnableWithTimeout(r.sim, fiend, time.Minute)
	defer func() { fiend.SummonedAsPet = false }()

	for _, tc := range []struct {
		name  string
		isPet bool
		want  *core.Unit
	}{
		{"guardian", false, first},
		{"pet", true, &fiend.Unit},
	} {
		fiend.SummonedAsPet = tc.isPet
		r.setHealth(t, nil, nil)
		fiend.GainHealth(r.sim, fiend.MaxHealth(), r.metrics)
		fiend.RemoveHealth(r.sim, 1000)
		caster.PrayerOfMending.SkipCastAndApplyEffects(r.sim, holder)
		caster.ProcPrayerOfMending(r.sim, holder, caster.PrayerOfMending)
		var got []*core.Unit
		for _, unit := range append(slices.Clone(r.units), &fiend.Unit) {
			if unit.GetAura(label).IsActive() {
				got = append(got, unit)
			}
		}
		if !slices.Equal(got, []*core.Unit{tc.want}) {
			t.Errorf("%s: jumped to %v, want %v", tc.name, unitLabels(got), unitLabels([]*core.Unit{tc.want}))
		}
	}
}

// TARGET_CHECK_PARTY keeps the party's pets and its guardians with a pet bar, a Shadowfiend too, and skips a guardian
// without one (Pet.Uncontrollable: a Rune Weapon, a Gargoyle, a Fire Elemental).
func TestPrayerOfHealingHealsThePartysPets(t *testing.T) {
	r := newHealRaid(t, 2)
	caster := r.priests[0]
	r.spell = caster.PrayerOfHealing
	fiend := caster.ShadowfiendPet
	fiend.EnableWithTimeout(r.sim, fiend, time.Minute)
	defer func() { fiend.Uncontrollable = false }()

	for _, tc := range []struct {
		name           string
		uncontrollable bool
		want           []*core.Unit
	}{
		{"with a pet bar", false, []*core.Unit{r.units[0], r.units[1], &fiend.Unit}},
		{"without", true, []*core.Unit{r.units[0], r.units[1]}},
	} {
		fiend.Uncontrollable = tc.uncontrollable
		r.setHealth(t, nil, nil)
		caster.PrayerOfHealing.SkipCastAndApplyEffects(r.sim, r.units[0])
		if !slices.Equal(r.healed, tc.want) {
			t.Errorf("%s: healed %v, want %v", tc.name, unitLabels(r.healed), unitLabels(tc.want))
		}
	}
}

func unitLabels(units []*core.Unit) []string {
	labels := make([]string, len(units))
	for i, unit := range units {
		labels[i] = unit.Label
	}
	return labels
}
