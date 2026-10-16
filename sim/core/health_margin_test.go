package core

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// marginTank is the timing test agent tanking the one target, at maxHealth, with the death tracking a
// healing model brings but no healing: the test's hits and heals are all that happens to its health.
type marginTank struct {
	t     *testing.T
	sim   *Simulation
	agent *timingTestAgent
	death *Aura
	heals *ResourceMetrics
}

func newMarginTank(t *testing.T, healing *proto.HealingModel, maxHealth float64, saveAll bool) *marginTank {
	t.Helper()
	sim := NewSim(&proto.RaidSimRequest{
		SimOptions: &proto.SimOptions{RandomSeed: 101, IsTest: true, Iterations: 1, SaveAllValues: saveAll},
		Raid: &proto.Raid{
			Parties: []*proto.Party{{
				Players: []*proto.Player{{
					Name:         "Tank",
					Class:        proto.Class_ClassShaman,
					Consumes:     &proto.Consumes{},
					Buffs:        &proto.IndividualBuffs{},
					Spec:         &proto.Player_EnhancementShaman{},
					Equipment:    &proto.EquipmentSpec{},
					HealingModel: healing,
				}},
				Buffs: &proto.PartyBuffs{},
			}},
			Tanks: []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}},
		},
		Encounter: &proto.Encounter{
			Targets:  []*proto.Target{{Name: "target", Level: 83, MobType: proto.MobType_MobTypeDemon}},
			Duration: 60,
		},
	})
	sim.reset()
	agent := sim.Raid.Parties[0].Players[0].(*timingTestAgent)
	agent.stats[stats.Health] = maxHealth
	agent.healthBar.reset(sim)
	return &marginTank{t: t, sim: sim, agent: agent, death: agent.GetAura(ChanceOfDeathAuraLabel), heals: &ResourceMetrics{}}
}

// hit is damage as the death tracking takes it, after every reduction.
func (m *marginTank) hit(at time.Duration, amount float64) {
	m.sim.CurrentTime = at
	m.death.OnSpellHitTaken(m.death, m.sim, nil, &SpellResult{Damage: amount})
}

func (m *marginTank) heal(amount float64) {
	m.agent.GainHealth(m.sim, amount, m.heals)
}

// done ends the iteration and returns the unit's metrics.
func (m *marginTank) done() *proto.UnitMetrics {
	m.agent.Metrics.doneIteration(&m.agent.Unit, m.sim)
	return m.agent.Metrics.ToProto()
}

func TestHealthMarginIsALivingTanksLowestHealth(t *testing.T) {
	m := newMarginTank(t, &proto.HealingModel{BurstWindow: 6}, 50000, true)
	if m.death == nil || !m.agent.Metrics.isTanking {
		t.Fatal("the agent doesn't tank with death tracking")
	}
	m.hit(time.Second, 20000)
	m.heal(5000)
	m.hit(2*time.Second, 25000)
	lowest := m.agent.CurrentHealth()
	m.heal(80000)
	m.hit(3*time.Second, 1000)
	got := m.done()
	if lowest != 10000 || !slices.Equal(got.IterationHealthMargins, []float64{10000}) {
		t.Errorf("margins %v, want the lowest health %v", got.IterationHealthMargins, lowest)
	}
	if got.HealthMarginAvg == nil || *got.HealthMarginAvg != 10000 || got.HealthMarginStdev != 0 {
		t.Errorf("margin %v ± %v, want 10000 ± 0", got.HealthMarginAvg, got.HealthMarginStdev)
	}
	if !slices.Equal(got.IterationDeaths, []bool{false}) || !slices.Equal(got.IterationFirstDeathSeconds, []float64{60}) {
		t.Errorf("deaths %v at %v s, want none and the fight's 60 s", got.IterationDeaths, got.IterationFirstDeathSeconds)
	}
}

// Past 0 the margin counts the overkill, and heals pay it back before they raise health: it's the
// health a bar that never stops at 0 would show.
func TestHealthMarginGoesPastZeroForADeath(t *testing.T) {
	m := newMarginTank(t, &proto.HealingModel{BurstWindow: 6}, 50000, true)
	m.hit(time.Second, 30000)
	m.hit(12*time.Second, 26000) // 6000 past 0
	m.heal(4000)                 // 4000 on the bar, -2000 behind it
	m.hit(13*time.Second, 1000)  // -3000
	m.heal(60000)                // 13000 over the top pays the rest
	if m.agent.healthBar.healthDebt != 0 || m.agent.healthBar.shadowHealth() != 50000 {
		t.Errorf("debt %v and shadow %v after healing past the debt, want 0 and full health", m.agent.healthBar.healthDebt, m.agent.healthBar.shadowHealth())
	}
	got := m.done()
	if !slices.Equal(got.IterationHealthMargins, []float64{-6000}) {
		t.Errorf("margins %v, want -6000", got.IterationHealthMargins)
	}
	if !slices.Equal(got.IterationDeaths, []bool{true}) || !slices.Equal(got.IterationFirstDeathSeconds, []float64{12}) {
		t.Errorf("deaths %v at %v s, want one at 12 s", got.IterationDeaths, got.IterationFirstDeathSeconds)
	}
}

// A margin of 0 or less is a death and nothing else: a hit for exactly the health left kills at 0,
// one short of it leaves 1.
func TestHealthMarginIsZeroOrLessExactlyWhenDead(t *testing.T) {
	for _, tc := range []struct {
		hit    float64
		margin float64
		dead   bool
	}{
		{49999, 1, false},
		{50000, 0, true},
		{50001, -1, true},
	} {
		m := newMarginTank(t, &proto.HealingModel{BurstWindow: 6}, 50000, true)
		m.hit(time.Second, tc.hit)
		got := m.done()
		margin := got.IterationHealthMargins[0]
		if margin != tc.margin || got.IterationDeaths[0] != tc.dead || (margin <= 0) != got.IterationDeaths[0] {
			t.Errorf("a %v hit: margin %v, dead %v; want %v and %v", tc.hit, margin, got.IterationDeaths[0], tc.margin, tc.dead)
		}
	}
}

// With nothing tied to a percent of max health acting, X more max health is X more margin, dead or alive.
func TestHealthMarginShiftsByTheHealthAdded(t *testing.T) {
	const bonus = 1234.5
	fight := func(maxHealth float64) float64 {
		m := newMarginTank(t, &proto.HealingModel{BurstWindow: 6}, maxHealth, true)
		m.hit(time.Second, 30000)
		m.heal(12000)
		m.hit(2*time.Second, 40000)
		m.heal(9000)
		m.hit(3*time.Second, 3000)
		m.heal(70000)
		m.hit(4*time.Second, 45000)
		return m.done().IterationHealthMargins[0]
	}
	for _, maxHealth := range []float64{50000, 30000} {
		base, more := fight(maxHealth), fight(maxHealth+bonus)
		if math.Abs(more-base-bonus) > 1e-9 {
			t.Errorf("at %v max health: margin %v, %v with %v more, want exactly %v more", maxHealth, base, more, bonus, bonus)
		}
	}
}

// A lethal hit that a save absorbs whole, as Ardent Defender's does, takes no health: the margin reads
// the health before it, and the tank lives.
func TestHealthMarginAtAnAbsorbedLethalHit(t *testing.T) {
	m := newMarginTank(t, &proto.HealingModel{BurstWindow: 6}, 50000, true)
	m.hit(time.Second, 40000)
	m.hit(2*time.Second, 0)
	m.heal(15000)
	got := m.done()
	if !slices.Equal(got.IterationHealthMargins, []float64{10000}) || got.IterationDeaths[0] {
		t.Errorf("margin %v, dead %v; want the 10000 before the hit, alive", got.IterationHealthMargins, got.IterationDeaths)
	}
}

func TestHealthMarginNeedsAHealingModel(t *testing.T) {
	m := newMarginTank(t, nil, 50000, true)
	if !m.agent.Metrics.isTanking {
		t.Fatal("the agent doesn't tank")
	}
	m.agent.RemoveHealth(m.sim, 60000)
	got := m.done()
	if got.IterationHealthMargins != nil || got.HealthMarginAvg != nil || got.IterationFirstDeathSeconds != nil {
		t.Errorf("margins %v, mean %v, first deaths %v without a healing model, want none", got.IterationHealthMargins, got.HealthMarginAvg, got.IterationFirstDeathSeconds)
	}
}

// The mean and standard deviation come with every tank's metrics; the per-iteration values only under
// SaveAllValues.
func TestHealthMarginMeanWithoutSavedValues(t *testing.T) {
	m := newMarginTank(t, &proto.HealingModel{BurstWindow: 6}, 50000, false)
	for _, amount := range []float64{10000, 30000} {
		m.agent.Metrics.reset()
		m.agent.healthBar.reset(m.sim)
		m.hit(time.Second, amount)
		m.agent.Metrics.doneIteration(&m.agent.Unit, m.sim)
	}
	got := m.agent.Metrics.ToProto()
	if got.IterationHealthMargins != nil || got.IterationFirstDeathSeconds != nil {
		t.Errorf("per-iteration values %v and %v without SaveAllValues", got.IterationHealthMargins, got.IterationFirstDeathSeconds)
	}
	if got.HealthMarginAvg == nil || *got.HealthMarginAvg != 30000 || math.Abs(got.HealthMarginStdev-10000) > 1e-9 {
		t.Errorf("margin %v ± %v, want 30000 ± 10000", got.HealthMarginAvg, got.HealthMarginStdev)
	}
}
