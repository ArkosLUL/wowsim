package dps

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// runeWeaponLine is a damage line of the log: who dealt it, with which action, and what it came to.
type runeWeaponLine struct {
	at     time.Duration
	weapon bool
	tag    int
	landed bool
	damage float64
}

// runeWeaponLines reads the white hits of the DK and his rune weapon off the log, the rolls of the copies
// (AfterPostOutcome, by time and hand) and when both the weapon and the DK's aura were up.
func runeWeaponLines(entries []logEntry) (lines []runeWeaponLine, rolls map[time.Duration]map[int]float64, up func(time.Duration) bool) {
	rolls = map[time.Duration]map[int]float64{}
	outcomes := []string{"Hit", "Crit", "Glance", "Crush", "Block", "CriticalBlock", "Miss", "Dodge", "Parry"}
	var weaponOut, auraUp [][2]time.Duration
	window := func(spans *[][2]time.Duration, at time.Duration, start bool) {
		if start {
			*spans = append(*spans, [2]time.Duration{at, time.Hour})
		} else if len(*spans) > 0 {
			(*spans)[len(*spans)-1][1] = at
		}
	}
	for _, e := range entries {
		dk, weapon := strings.HasSuffix(e.unit, "(#1)"), strings.HasSuffix(e.unit, "- Rune Weapon")
		switch {
		case weapon && (e.text == "Pet summoned" || e.text == "Pet dismissed"):
			window(&weaponOut, e.at, e.text == "Pet summoned")
		case dk && (e.text == "Aura gained: {SpellID: 49028}" || e.text == "Aura faded: {SpellID: 49028}"):
			window(&auraUp, e.at, e.text == "Aura gained: {SpellID: 49028}")
		}
		if !dk && !weapon {
			continue
		}
		for tag := 1; tag <= 4; tag++ {
			id := " " + core.ActionID{OtherID: proto.OtherAction_OtherActionAttack, Tag: int32(tag)}.String() + " "
			_, rest, ok := strings.Cut(e.text, id)
			if !ok {
				continue
			}
			if strings.HasPrefix(rest, "[DEBUG]") {
				if tag >= 3 {
					_, value, _ := strings.Cut(rest, "AfterPostOutcome:")
					if rolls[e.at] == nil {
						rolls[e.at] = map[int]float64{}
					}
					rolls[e.at][tag-2], _ = strconv.ParseFloat(strings.TrimSpace(value), 64)
				}
				continue
			}
			if fields := strings.Fields(rest); len(fields) == 0 || !slices.Contains(outcomes, fields[0]) {
				continue
			}
			line := runeWeaponLine{at: e.at, weapon: weapon, tag: tag}
			if _, amount, ok := strings.Cut(rest, " for "); ok {
				line.landed = true
				line.damage, _ = strconv.ParseFloat(strings.Fields(amount)[0], 64)
			}
			lines = append(lines, line)
		}
	}
	// a swing on the update either one starts or ends may come on either side of it
	within := func(spans [][2]time.Duration, at time.Duration) bool {
		return slices.ContainsFunc(spans, func(s [2]time.Duration) bool { return at > s[0] && at < s[1] })
	}
	up = func(at time.Duration) bool { return within(weaponOut, at) && within(auraUp, at) }
	return lines, rolls, up
}

// spell_dk_dancing_rune_weapon copies each white swing the DK lands, either hand, on the spot: his own swing
// rolled again (CalculateMeleeDamage), halved. npc_pet_dk_dancing_rune_weapon is a NullCreatureAI, so the
// weapon swings at no other time.
func TestRuneWeaponCopiesTheLandedWhiteSwings(t *testing.T) {
	player := parityPlayer(BloodTalents, "p3_blood", runeWeaponAPL)
	copies, landed := 0, 0
	for seed := int64(1); seed <= 3; seed++ {
		lines, rolls, up := runeWeaponLines(runParity(t, player, seed))
		swung := map[time.Duration]map[int]bool{}
		for _, l := range lines {
			if l.weapon || !l.landed || !up(l.at) {
				continue
			}
			if swung[l.at] == nil {
				swung[l.at] = map[int]bool{}
			}
			swung[l.at][l.tag] = true
			landed++
		}
		for _, l := range lines {
			if !l.weapon || !up(l.at) {
				continue
			}
			copies++
			if !swung[l.at][l.tag] {
				t.Errorf("seed %d: the rune weapon hit with hand %d at %v, with no landed swing of the DK's", seed, l.tag, l.at)
				continue
			}
			delete(swung[l.at], l.tag)
			roll, ok := rolls[l.at][l.tag]
			if !ok {
				t.Errorf("seed %d: no roll on the DK for the copy at %v", seed, l.at)
				continue
			}
			if l.landed && l.damage != math.Floor(roll/2) {
				t.Errorf("seed %d: the copy at %v hit for %v, want half the DK's roll of %v", seed, l.at, l.damage, roll)
			}
		}
		for at, hands := range swung {
			for tag := range hands {
				t.Errorf("seed %d: the DK's swing with hand %d at %v landed and the rune weapon didn't copy it", seed, tag, at)
			}
		}
	}
	t.Logf("%d copies of %d landed swings", copies, landed)
	if landed == 0 || copies != landed {
		t.Errorf("%d copies of %d landed swings", copies, landed)
	}
}

// Every hit the rune weapon deals takes the DK's attack power, crit and damage mods, so a buff on it would
// count twice over the stats it inherits. It has no pet bar either (SummonProperties 208).
func TestRuneWeaponTakesNoBuffs(t *testing.T) {
	weaponStats := func(party *proto.PartyBuffs, raid *proto.RaidBuffs, individual *proto.IndividualBuffs) (stats.Stats, float64) {
		player := parityPlayer(BloodTalents, "p3_blood", noRotation)
		player.Buffs = individual
		env, _, _ := core.NewEnvironment(core.SinglePlayerRaidProto(player, party, raid, nil), core.MakeSingleTargetEncounter(0), false)
		dk := env.Raid.Parties[0].Players[0].(*DpsDeathknight)
		if !dk.RuneWeapon.Uncontrollable {
			t.Error("the rune weapon has a pet bar")
		}
		return dk.RuneWeapon.GetStats(), dk.RuneWeapon.PseudoStats.DamageDealtMultiplier
	}
	bare, bareMultiplier := weaponStats(&proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.IndividualBuffs{})
	buffed, buffedMultiplier := weaponStats(core.FullPartyBuffs, core.FullRaidBuffs, core.FullIndividualBuffs)
	if buffed != bare || buffedMultiplier != bareMultiplier {
		t.Errorf("the raid's buffs moved the rune weapon's own stats: %s, multiplier %v, against %s, %v",
			buffed.FlatString(), buffedMultiplier, bare.FlatString(), bareMultiplier)
	}
}

// The rune weapon's hits take the DK's attack power and crit as they stand, so it follows his changes while it's out.
func TestRuneWeaponFollowsTheKnightsAttackPower(t *testing.T) {
	player := parityPlayer(BloodTalents, "p3_blood", noRotation)
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 1},
	})
	sim.Reset()
	sim.PrePull()
	dk := sim.Raid.Parties[0].Players[0].(*DpsDeathknight)
	dk.RuneWeapon.EnableWithTimeout(sim, dk.RuneWeapon, 12*time.Second)

	before := dk.RuneWeapon.GetStat(stats.AttackPower)
	if before != dk.GetStat(stats.AttackPower) {
		t.Fatalf("the rune weapon has %v attack power, want the DK's %v", before, dk.GetStat(stats.AttackPower))
	}
	dkBefore := dk.GetStat(stats.AttackPower)
	dk.AddStatsDynamic(sim, stats.Stats{stats.AttackPower: 1000})
	gained := dk.GetStat(stats.AttackPower) - dkBefore
	if got := dk.RuneWeapon.GetStat(stats.AttackPower); math.Abs(got-before-gained) > 1e-6 || gained < 1000 {
		t.Errorf("after the DK gained %v attack power the rune weapon has %v, want %v", gained, got, before+gained)
	}
	dk.RuneWeapon.Disable(sim)
	dk.AddStatsDynamic(sim, stats.Stats{stats.AttackPower: 500})
	dk.RuneWeapon.Enable(sim, dk.RuneWeapon)
	if got := dk.RuneWeapon.GetStat(stats.AttackPower); got != dk.GetStat(stats.AttackPower) {
		t.Errorf("summoned again the rune weapon has %v attack power, want the DK's %v", got, dk.GetStat(stats.AttackPower))
	}
}
