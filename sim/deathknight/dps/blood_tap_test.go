package dps

import (
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/deathknight"
)

// talentsWith sets one talent's points, by tree and place in it.
func talentsWith(talents string, tree, index int, points byte) string {
	trees := strings.Split(talents, "-")
	for len(trees) < 3 {
		trees = append(trees, "")
	}
	t := []byte(trees[tree])
	for len(t) <= index {
		t = append(t, '0')
	}
	t[index] = points
	trees[tree] = string(t)
	return strings.Join(trees, "-")
}

type runeStep struct {
	at time.Duration
	do func(sim *core.Simulation, dk *deathknight.Deathknight)
}

// runRuneSteps runs one iteration of the player, who only swings, with each step done at its time.
func runRuneSteps(player *proto.Player, seed int64, steps ...runeStep) {
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: seed},
	})
	sim.Reset()
	sim.PrePull()
	dk := sim.Raid.Parties[0].Players[0].(*DpsDeathknight).Deathknight
	for _, step := range steps {
		sim.AddPendingAction(&core.PendingAction{
			NextActionAt: step.at,
			Priority:     core.ActionPriorityLow,
			OnAction:     func(sim *core.Simulation) { step.do(sim, dk) },
		})
	}
	for !sim.Step() {
	}
	sim.Cleanup()
}

// bloodRunes is the two blood slots: B a blood rune, D a death rune, lower case on cooldown.
func bloodRunes(dk *deathknight.Deathknight) string {
	out := make([]byte, 2)
	for slot := range out {
		out[slot] = 'B'
		if dk.RuneIsDeath(int8(slot)) {
			out[slot] = 'D'
		}
		if !dk.RuneIsActive(int8(slot)) {
			out[slot] += 'a' - 'A'
		}
	}
	return string(out)
}

// tappedSlot is the blood slot Blood Tap's aura holds, -1 for none.
func tappedSlot(dk *deathknight.Deathknight) int {
	for slot := int8(0); slot < 2; slot++ {
		if dk.IsBloodTappedRune(slot) {
			return int(slot)
		}
	}
	return -1
}

// castNow casts past the idle rotation, which holds the GCD while it waits.
func castNow(sim *core.Simulation, dk *deathknight.Deathknight, spell *core.Spell) bool {
	dk.GCD.Reset()
	return spell.Cast(sim, dk.CurrentTarget)
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// Blood Tap by Spell::EffectActivateRune and its SPELL_AURA_CONVERT_RUNE: a rune comes off cooldown only when
// it's a death rune in a blood slot, or when both blood runes are plain and down, and then the one due back
// sooner. Only a plain blood rune that's ready turns.
func TestBloodTapRunePick(t *testing.T) {
	for _, c := range []struct {
		name          string
		talents, gear string
		// Blood Strikes before the tap, in seconds
		strikes []float64
		tap     float64
		before  string
		after   string
		tapped  int
	}{
		{"both ready", BloodTalents, "p3_blood", nil, 1, "BB", "DB", 0},
		{"first down", BloodTalents, "p3_blood", []float64{1}, 3, "bB", "bD", 1},
		{"both down", BloodTalents, "p3_blood", []float64{1, 3}, 4, "bb", "Db", 0},
		{"second down", BloodTalents, "p3_blood", []float64{1, 3}, 12, "Bb", "Db", 0},
		// Blood of the North turns the struck rune, so it's a death rune on cooldown
		{"first a death rune down", FrostTalents, "p3_frost", []float64{1}, 3, "dB", "DD", 1},
		{"both death runes down", FrostTalents, "p3_frost", []float64{1, 3}, 4, "dd", "Dd", -1},
	} {
		t.Run(c.name, func(t *testing.T) {
			compared := false
			// a Blood Strike that misses leaves another setup, so try seeds until the setup is the case's
			for seed := int64(1); seed <= 20 && !compared; seed++ {
				var steps []runeStep
				for _, at := range c.strikes {
					steps = append(steps, runeStep{seconds(at), func(sim *core.Simulation, dk *deathknight.Deathknight) {
						castNow(sim, dk, dk.BloodStrike)
					}})
				}
				steps = append(steps, runeStep{seconds(c.tap), func(sim *core.Simulation, dk *deathknight.Deathknight) {
					if bloodRunes(dk) != c.before {
						return
					}
					compared = true
					if !dk.BloodTap.Cast(sim, dk.CurrentTarget) {
						t.Fatal("Blood Tap didn't cast")
					}
					if got := bloodRunes(dk); got != c.after {
						t.Errorf("blood runes %s after the tap, want %s", got, c.after)
					}
					if got := tappedSlot(dk); got != c.tapped {
						t.Errorf("Blood Tap holds slot %d, want %d", got, c.tapped)
					}
				}})
				runRuneSteps(parityPlayer(c.talents, c.gear, noRotation), seed, steps...)
			}
			if !compared {
				t.Fatalf("no seed got the blood runes to %s", c.before)
			}
		})
	}
}

// spell_dk_death_rune puts its own aura on the rune it turns, so a tapped rune that Blood of the North turns
// again is the talent's from then on: Blood Tap running out leaves it, and the talent's 30 s reverts it.
func TestDeathRuneTalentTakesOverTheTappedRune(t *testing.T) {
	compared := false
	for seed := int64(1); seed <= 20 && !compared; seed++ {
		strike := func(sim *core.Simulation, dk *deathknight.Deathknight) { castNow(sim, dk, dk.BloodStrike) }
		landed := false
		runRuneSteps(parityPlayer(FrostTalents, "p3_frost", noRotation), seed,
			runeStep{seconds(1), func(sim *core.Simulation, dk *deathknight.Deathknight) {
				dk.BloodTap.Cast(sim, dk.CurrentTarget)
			}},
			// takes the plain blood rune
			runeStep{seconds(2), strike},
			// takes the tapped one
			runeStep{seconds(4), func(sim *core.Simulation, dk *deathknight.Deathknight) {
				if bloodRunes(dk) != "Dd" || tappedSlot(dk) != 0 {
					return
				}
				strike(sim, dk)
				// a miss brings the rune back in 1.5 s and procs nothing
				if landed = dk.RuneReadyAt(sim, 0) > sim.CurrentTime+5*time.Second; !landed {
					return
				}
				compared = true
				if got := bloodRunes(dk); got != "dd" || tappedSlot(dk) != -1 {
					t.Errorf("after the strike: blood runes %s with Blood Tap on slot %d, want dd and no slot", got, tappedSlot(dk))
				}
			}},
			// Blood Tap ran out at 21 s
			runeStep{seconds(22), func(sim *core.Simulation, dk *deathknight.Deathknight) {
				if got := bloodRunes(dk); landed && got != "DD" {
					t.Errorf("after Blood Tap ran out: blood runes %s, want DD", got)
				}
			}},
			// the talent's periodic, restarted at 4 s
			runeStep{seconds(35), func(sim *core.Simulation, dk *deathknight.Deathknight) {
				if got := bloodRunes(dk); landed && got != "BB" {
					t.Errorf("30 s after the last proc: blood runes %s, want BB", got)
				}
			}},
		)
	}
	if !compared {
		t.Fatal("no seed landed the strike on the tapped rune")
	}
}

// The death rune talents proc from procs, so Threat of Thassarian's off-hand strike rolls them again for the
// runes the main hand just spent: a 33% Death Rune Mastery turns them 1 - 0.67^2 of the time, a 30% Blood of
// the North 1 - 0.7^2.
func TestOffHandStrikeRerollsDeathRuneTalents(t *testing.T) {
	for _, c := range []struct {
		name    string
		talents string
		spell   func(dk *deathknight.Deathknight) *core.Spell
		slots   []int8
		want    float64
	}{
		// Death Rune Mastery is the blood tree's 9th talent, Blood of the North the frost tree's 23rd
		{"Death Rune Mastery", talentsWith(FrostTalents, 0, 8, '1'),
			func(dk *deathknight.Deathknight) *core.Spell { return dk.Obliterate }, []int8{2, 3, 4, 5}, 1 - 0.67*0.67},
		{"Blood of the North", talentsWith(FrostTalents, 1, 22, '1'),
			func(dk *deathknight.Deathknight) *core.Spell { return dk.BloodStrike }, []int8{0, 1}, 1 - 0.7*0.7},
	} {
		t.Run(c.name, func(t *testing.T) {
			landed, turned := 0, 0
			var steps []runeStep
			// far enough apart for every rune to be back
			for at := 1.0; at < 170; at += 10.5 {
				steps = append(steps, runeStep{seconds(at), func(sim *core.Simulation, dk *deathknight.Deathknight) {
					if !castNow(sim, dk, c.spell(dk)) {
						return
					}
					// the runes a landed strike just spent are the only ones down for this long
					spent, death := 0, 0
					for _, slot := range c.slots {
						if dk.RuneReadyAt(sim, slot) > sim.CurrentTime+5*time.Second {
							spent++
							if dk.RuneIsDeath(slot) {
								death++
							}
						}
					}
					if spent == 0 {
						return
					}
					landed++
					if death == spent {
						turned++
					} else if death != 0 {
						t.Errorf("%d of the %d runes spent turned", death, spent)
					}
				}})
			}
			for seed := int64(1); seed <= 25; seed++ {
				runRuneSteps(parityPlayer(c.talents, "p3_frost", noRotation), seed, steps...)
			}
			if landed < 200 {
				t.Fatalf("only %d strikes landed", landed)
			}
			t.Logf("%d of %d landed strikes turned their runes", turned, landed)
			if rate := float64(turned) / float64(landed); rate < c.want-0.1 || rate > c.want+0.1 {
				t.Errorf("%d of %d landed strikes turned their runes (%.3f), want about %.3f", turned, landed, rate, c.want)
			}
		})
	}
}
