package restoration

import (
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// spell_sha_ancestral_awakening casts 52752 for CalculatePct(int32(crit heal), 10 a rank), and 52752 ignores the
// caster's modifiers: live, with Purification 5/5 each one was exactly that, never 10% more.
func TestAncestralAwakeningIsAWholePercentOfTheCrit(t *testing.T) {
	for rank := int32(1); rank <= 3; rank++ {
		// StandardTalents with the rank in Ancestral Awakening, the restoration tree's 22nd talent
		talents := []byte(StandardTalents)
		talents[strings.LastIndex(StandardTalents, "-")+22] = byte('0' + rank)
		sim := core.NewSim(&proto.RaidSimRequest{
			Raid: core.SinglePlayerRaidProto(&proto.Player{
				Race:          proto.Race_RaceTroll,
				Class:         proto.Class_ClassShaman,
				Equipment:     &proto.EquipmentSpec{},
				Consumes:      &proto.Consumes{},
				Spec:          PlayerOptionsStandard,
				Rotation:      core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
				TalentsString: string(talents),
				Glyphs:        StandardGlyphs,
			}, nil, nil, nil),
			Encounter:  core.MakeSingleTargetEncounter(0),
			SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
		})
		sim.Reset()
		shaman := sim.Raid.Parties[0].Players[0].(*RestorationShaman)
		if shaman.Talents.AncestralAwakening != rank || shaman.Talents.Purification != 5 {
			t.Fatalf("Ancestral Awakening %d, want %d, Purification %d", shaman.Talents.AncestralAwakening, rank, shaman.Talents.Purification)
		}
		target := &shaman.Unit
		wave := &shaman.HealingWave.SpellMetrics[target.UnitIndex]
		awakening := &shaman.AncestralAwakening.SpellMetrics[target.UnitIndex]
		for checked, casts := 0, 0; checked < 3; casts++ {
			if casts == 500 {
				t.Fatalf("rank %d: %d crits in %d Healing Waves", rank, checked, casts)
			}
			waveBefore, awakeningBefore := *wave, *awakening
			shaman.HealingWave.SkipCastAndApplyEffects(sim, target)
			if wave.Crits == waveBefore.Crits {
				continue
			}
			crit := wave.TotalHealing - waveBefore.TotalHealing
			got := awakening.TotalHealing - awakeningBefore.TotalHealing
			if want := float64(int32(float32(crit) * float32(10*rank) / 100)); got != want {
				t.Errorf("rank %d: a %v crit's Ancestral Awakening healed %v, want %v", rank, crit, got, want)
			}
			checked++
		}
	}
}

// spell_sha_ancestral_awakening_proc heals the living raid member lowest in health percent, raid order breaking a
// tie, not the crit's own target.
func TestAncestralAwakeningHealsTheLowestHealthPercent(t *testing.T) {
	r := newChainHealRaid()
	caster := r.shamans[0]
	var healed []*core.Unit
	watch := caster.RegisterAura(core.Aura{
		Label:    "ancestral awakening target",
		Duration: core.NeverExpires,
		OnHealDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if spell == caster.AncestralAwakening {
				healed = append(healed, result.Target)
			}
		},
	})
	main, second, third := &r.shamans[0].Unit, &r.shamans[1].Unit, &r.shamans[2].Unit

	for _, c := range []struct {
		name   string
		critOn *core.Unit
		hurt   map[*core.Unit]float64 // the part of max health taken off
		want   *core.Unit
	}{
		{"no one hurt", third, nil, main},
		{"lowest percent", third, map[*core.Unit]float64{main: 0.1, second: 0.3}, second},
		{"the dead", main, map[*core.Unit]float64{second: 1, third: 0.1}, third},
	} {
		sim := r.sim
		sim.Reset()
		sim.PrePull()
		for sim.CurrentTime < 0 && !sim.Step() {
		}
		watch.Activate(sim)
		healed = healed[:0]
		for unit, part := range c.hurt {
			unit.RemoveHealth(sim, part*unit.MaxHealth())
		}
		for casts := 0; len(healed) == 0; casts++ {
			if casts == 500 {
				t.Fatalf("%s: no Ancestral Awakening in %d Healing Waves", c.name, casts)
			}
			caster.HealingWave.SkipCastAndApplyEffects(sim, c.critOn)
		}
		if healed[0] != c.want {
			t.Errorf("%s: Ancestral Awakening healed %v, want %v", c.name, unitLabels(healed[:1]), unitLabels([]*core.Unit{c.want}))
		}
		sim.Cleanup()
	}
}
