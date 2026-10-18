package healing

import (
	"slices"
	"strconv"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// healRaid is n holy priests, five to a party, with no rotation, at the pull. The first one casts, and healed
// lists whom spell healed, in order.
type healRaid struct {
	sim     *core.Simulation
	priests []*HealingPriest
	units   []*core.Unit
	metrics *core.ResourceMetrics
	spell   *core.Spell
	healed  []*core.Unit
}

func newHealRaid(t *testing.T, n int) *healRaid {
	t.Helper()
	var parties []*proto.Party
	for i := range n {
		if i%5 == 0 {
			parties = append(parties, &proto.Party{})
		}
		party := parties[len(parties)-1]
		party.Players = append(party.Players, &proto.Player{
			Race:          proto.Race_RaceUndead,
			Class:         proto.Class_ClassPriest,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsHoly,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
			TalentsString: HolyTalents,
			Glyphs:        HolyGlyphs,
		})
	}
	r := &healRaid{sim: core.NewSim(&proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: parties},
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})}
	for _, party := range r.sim.Raid.Parties {
		for _, player := range party.Players {
			priest := player.(*HealingPriest)
			r.priests = append(r.priests, priest)
			r.units = append(r.units, &priest.Unit)
		}
	}
	caster := r.priests[0]
	r.metrics = caster.NewHealthMetrics(core.ActionID{OtherID: proto.OtherAction_OtherActionHealingModel})
	caster.RegisterAura(core.Aura{
		Label:    "heal targets",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == r.spell {
				r.healed = append(r.healed, result.Target)
			}
		},
	})
	r.sim.Reset()
	r.sim.PrePull()
	for r.sim.CurrentTime < 0 && !r.sim.Step() {
	}
	return r
}

// setHealth gives every priest maxHealth, full, then takes hurt off each unit in it.
func (r *healRaid) setHealth(t *testing.T, maxHealth map[*core.Unit]float64, hurt map[*core.Unit]float64) {
	t.Helper()
	for _, unit := range r.units {
		most := 20000.0
		if m, ok := maxHealth[unit]; ok {
			most = m
		}
		unit.AddStatsDynamic(r.sim, stats.Stats{stats.Health: most - unit.MaxHealth()})
		unit.GainHealth(r.sim, unit.MaxHealth(), r.metrics)
		if unit.CurrentHealth() != most {
			t.Fatalf("%v at %v of %v health, want %v", r.labels([]*core.Unit{unit}), unit.CurrentHealth(), unit.MaxHealth(), most)
		}
	}
	for unit, amount := range hurt {
		unit.RemoveHealth(r.sim, amount)
	}
	r.healed = r.healed[:0]
}

// labels names each unit by its place in the raid.
func (r *healRaid) labels(units []*core.Unit) []string {
	var labels []string
	for _, unit := range units {
		labels = append(labels, "priest "+strconv.Itoa(slices.Index(r.units, unit)))
	}
	return labels
}

// Acore::MostHPMissingGroupInRange: the jump goes to the living unit missing the most health, whatever its
// percent, never to the holder or the dead. With no one hurt, the first in raid order.
func TestPrayerOfMendingJumpsToTheMostHealthMissing(t *testing.T) {
	r := newHealRaid(t, 4)
	caster := r.priests[0]
	label := "PrayerOfMending" + strconv.Itoa(int(caster.Index))
	first, holder, small, big := r.units[0], r.units[1], r.units[2], r.units[3]
	sizes := map[*core.Unit]float64{small: 10000, big: 20000}

	for _, tc := range []struct {
		name     string
		hurt     map[*core.Unit]float64
		aboveMax *core.Unit
		want     *core.Unit
	}{
		// 90% against 92.5%
		{"most missing", map[*core.Unit]float64{small: 1000, big: 1500}, nil, big},
		{"dead", map[*core.Unit]float64{small: 1000, big: 20000}, nil, small},
		{"holder", map[*core.Unit]float64{holder: 5000, small: 1000}, nil, small},
		{"no one hurt", nil, nil, first},
		{"health above max", map[*core.Unit]float64{small: 1000}, first, small},
	} {
		r.setHealth(t, sizes, tc.hurt)
		if tc.aboveMax != nil {
			tc.aboveMax.AddStatsDynamic(r.sim, stats.Stats{stats.Health: -500})
		}
		caster.PrayerOfMending.SkipCastAndApplyEffects(r.sim, holder)
		caster.ProcPrayerOfMending(r.sim, holder, caster.PrayerOfMending)
		var got []*core.Unit
		for _, unit := range r.units {
			if unit.GetAura(label).IsActive() {
				got = append(got, unit)
			}
		}
		if !slices.Equal(got, []*core.Unit{tc.want}) {
			t.Errorf("%s: jumped to %v, want %v", tc.name, r.labels(got), r.labels([]*core.Unit{tc.want}))
		}
	}
}

// spell_pri_circle_of_healing: past its 6 targets (glyphed), the living ones with the lowest health percent, raid
// order breaking a tie. SpellInfo::CheckTarget keeps the dead out of the search.
func TestCircleOfHealingHealsTheMostHurt(t *testing.T) {
	r := newHealRaid(t, 8)
	caster := r.priests[0]
	if caster.CircleOfHealing == nil {
		t.Fatal("no Circle of Healing")
	}
	r.spell = caster.CircleOfHealing
	u := r.units

	for _, tc := range []struct {
		name     string
		hurt     map[*core.Unit]float64
		aboveMax *core.Unit
		want     []*core.Unit
	}{
		{"no one hurt", nil, nil, u[:6]},
		{"hurt and dead", map[*core.Unit]float64{u[1]: 20000, u[5]: 4000, u[6]: 6000, u[7]: 2000}, nil, []*core.Unit{u[6], u[5], u[7], u[0], u[2], u[3]}},
		// full on the server, so it keeps its place in the tie
		{"health above max", map[*core.Unit]float64{u[5]: 4000}, u[0], []*core.Unit{u[5], u[0], u[1], u[2], u[3], u[4]}},
	} {
		r.setHealth(t, nil, tc.hurt)
		if tc.aboveMax != nil {
			tc.aboveMax.AddStatsDynamic(r.sim, stats.Stats{stats.Health: -500})
		}
		caster.CircleOfHealing.SkipCastAndApplyEffects(r.sim, u[0])
		if !slices.Equal(r.healed, tc.want) {
			t.Errorf("%s: healed %v, want %v", tc.name, r.labels(r.healed), r.labels(tc.want))
		}
	}
}

// Prayer of Healing's party search goes through SpellInfo::CheckTarget, which keeps the dead out.
func TestPrayerOfHealingSkipsTheDead(t *testing.T) {
	r := newHealRaid(t, 3)
	caster := r.priests[0]
	r.spell = caster.PrayerOfHealing
	u := r.units
	r.setHealth(t, nil, map[*core.Unit]float64{u[1]: 20000, u[2]: 1000})
	caster.PrayerOfHealing.SkipCastAndApplyEffects(r.sim, u[0])
	if want := []*core.Unit{u[0], u[2]}; !slices.Equal(r.healed, want) {
		t.Errorf("healed %v, want %v", r.labels(r.healed), r.labels(want))
	}
	if u[1].CurrentHealth() != 0 {
		t.Errorf("the dead priest is at %v health", u[1].CurrentHealth())
	}
}
