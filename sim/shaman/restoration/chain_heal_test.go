package restoration

import (
	"slices"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/shaman"
)

// chainHealRaid is three glyphed shamans and their elementals, with nothing else to heal. The first one's Chain
// Heal goes on its own target, and order returns whom it healed, in the order it did.
type chainHealRaid struct {
	sim     *core.Simulation
	shamans []*RestorationShaman
	healed  []*core.Unit
	watch   *core.Aura
}

func newChainHealRaid() *chainHealRaid {
	players := make([]*proto.Player, 3)
	for i := range players {
		players[i] = &proto.Player{
			Race:          proto.Race_RaceTroll,
			Class:         proto.Class_ClassShaman,
			Equipment:     &proto.EquipmentSpec{},
			Consumes:      &proto.Consumes{},
			Spec:          PlayerOptionsStandard,
			TalentsString: StandardTalents,
			Glyphs:        StandardGlyphs,
			Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
		}
	}
	r := &chainHealRaid{sim: core.NewSim(&proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: players}}},
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})}
	for _, player := range r.sim.Raid.Parties[0].Players {
		r.shamans = append(r.shamans, player.(*RestorationShaman))
	}
	caster := r.shamans[0]
	r.watch = caster.RegisterAura(core.Aura{
		Label:    "chain heal order",
		Duration: core.NeverExpires,
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == caster.ChainHeal {
				r.healed = append(r.healed, result.Target)
			}
		},
	})
	return r
}

// order casts the Chain Heal with each unit of hurt down by its amount and the summoned pets out.
func (r *chainHealRaid) order(t *testing.T, target *core.Unit, hurt map[*core.Unit]float64, summoned []*shaman.FireElemental) []*core.Unit {
	t.Helper()
	sim := r.sim
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	r.healed = r.healed[:0]
	r.watch.Activate(sim)
	for _, pet := range summoned {
		pet.EnableWithTimeout(sim, pet, time.Minute)
	}
	for unit, amount := range hurt {
		if unit.MaxHealth() <= amount {
			t.Fatalf("%s has %.0f health, can't take %.0f off", unit.Label, unit.MaxHealth(), amount)
		}
		unit.RemoveHealth(sim, amount)
	}
	if !r.shamans[0].ChainHeal.Cast(sim, target) {
		t.Fatal("Chain Heal wasn't cast")
	}
	for sim.CurrentTime < 3*time.Second && !sim.Step() {
	}
	sim.Cleanup()
	return slices.Clone(r.healed)
}

func unitLabels(units []*core.Unit) []string {
	labels := make([]string, len(units))
	for i, unit := range units {
		labels[i] = unit.Label
	}
	return labels
}

// Spell::SearchChainTargets: a chain heal jumps to the most hurt unit it hasn't healed yet, never back to its
// main target, and stops when no one else is hurt. The glyph allows 3 jumps, and a pet that's away isn't in
// the world to jump to.
func TestChainHealJumpsToTheMostHurt(t *testing.T) {
	r := newChainHealRaid()
	main, second, third := &r.shamans[0].Unit, &r.shamans[1].Unit, &r.shamans[2].Unit
	secondPet, thirdPet := r.shamans[1].FireElemental, r.shamans[2].FireElemental

	for _, c := range []struct {
		name     string
		hurt     map[*core.Unit]float64
		summoned []*shaman.FireElemental
		want     []*core.Unit
	}{
		{"no one hurt", nil, nil, []*core.Unit{main}},
		{"only the main target hurt", map[*core.Unit]float64{main: 3000}, nil, []*core.Unit{main}},
		{"most hurt first", map[*core.Unit]float64{main: 3000, second: 100, &secondPet.Unit: 500},
			[]*shaman.FireElemental{secondPet}, []*core.Unit{main, &secondPet.Unit, second}},
		{"an away pet", map[*core.Unit]float64{second: 100, &secondPet.Unit: 500}, nil, []*core.Unit{main, second}},
		{"3 jumps at most", map[*core.Unit]float64{second: 100, third: 200, &secondPet.Unit: 500, &thirdPet.Unit: 300},
			[]*shaman.FireElemental{secondPet, thirdPet}, []*core.Unit{main, &secondPet.Unit, &thirdPet.Unit, third}},
	} {
		if got := r.order(t, main, c.hurt, c.summoned); !slices.Equal(got, c.want) {
			t.Errorf("%s: healed %v, want %v", c.name, unitLabels(got), unitLabels(c.want))
		}
	}
}
