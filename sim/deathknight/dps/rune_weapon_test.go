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
	googleProto "google.golang.org/protobuf/proto"
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

// runeWeaponSim is a blood DK with no rotation, 30 yd from the target, at the pull.
func runeWeaponSim(t *testing.T) (*core.Simulation, *DpsDeathknight) {
	t.Helper()
	player := parityPlayer(BloodTalents, "p3_blood", noRotation)
	player.DistanceFromTarget = 30
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 1},
	})
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 0 && !sim.Step() {
	}
	return sim, sim.Raid.Parties[0].Players[0].(*DpsDeathknight)
}

// Spell::EffectWeaponDmg rolls a rune weapon strike on the DK's own main hand (owner->CalculateDamage): his weapon
// at its normalized speed, or its own for Rune Strike, his attack power, flat bonuses and TOTAL_PCT inside.
func TestRuneWeaponStrikesRollTheKnightsWeapon(t *testing.T) {
	sim, dk := runeWeaponSim(t)
	dk.RuneWeapon.EnableWithTimeout(sim, dk.RuneWeapon, 12*time.Second)
	// a percent he gains once it's out counts too
	dk.PseudoStats.DamageDealtMultiplier *= 1.2

	weapon := dk.AutoAttacks.MH()
	pct := dk.PseudoStats.DamageDealtMultiplier * dk.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]
	for _, normalized := range []bool{true, false} {
		speed := weapon.SwingSpeed
		if normalized {
			speed = weapon.NormalizedSwingSpeed
		}
		added := dk.GetStat(stats.AttackPower)/core.DefaultAttackPowerPerDPS*speed + dk.PseudoStats.BonusDamage
		low, high := math.Trunc((weapon.BaseDamageMin+added)*pct), math.Trunc((weapon.BaseDamageMax+added)*pct)
		seenLow, seenHigh := math.Inf(1), math.Inf(-1)
		for range 5000 {
			roll := dk.DrwWeaponDamage(sim, normalized)
			seenLow, seenHigh = min(seenLow, roll), max(seenHigh, roll)
		}
		// float32 steps move an end by a point at most, and 5000 rolls come within 2% of each
		slack := 1 + 0.02*(high-low)
		if seenLow < low-1 || seenLow > low+slack || seenHigh > high+1 || seenHigh < high-slack {
			t.Errorf("normalized %v: rolls from %v to %v, want the DK's own %v to %v", normalized, seenLow, seenHigh, low, high)
		}
	}
}

// A copy takes the weapon's own percents, halved, and none of the DK's. His physical ones are in a strike's weapon
// roll, and Unit::SpellDamageBonusDone's branch for the weapon passes TotalMod and stack one argument early, which
// hands his call a TotalMod of 1, so a magic hit never reads his. After a spell it copies, spell_dk_dancing_rune_weapon
// leaves his UNIT_FIELD_COMBATREACH at 0.01, so his Death Coil flies 1.49 yd further. The weapon follows him at
// 0.01 yd with a reach of 0.01 (53160), so its own Death Coil does too.
func TestRuneWeaponCopiesTakeNoneOfTheKnightsPercents(t *testing.T) {
	sim, dk := runeWeaponSim(t)
	target := sim.GetTargetUnit(0)
	if got := dk.DeathCoilDamage.TravelTime(); got != 1187*time.Millisecond {
		t.Errorf("the DK's Death Coil flies %v before any copy, want 1187 ms", got)
	}
	if !dk.DancingRuneWeapon.Cast(sim, target) {
		t.Fatal("no Dancing Rune Weapon")
	}
	for until := sim.CurrentTime + 2*time.Second; sim.CurrentTime < until && !sim.Step(); {
	}

	shadow := stats.SchoolIndexShadow
	dk.PseudoStats.DamageDealtMultiplier *= 1.2
	dk.PseudoStats.SchoolDamageDealtMultiplier[shadow] *= 1.1
	// the damage spell alone, as Sudden Doom casts it: it can't miss, and its landing is what gets copied
	dk.DeathCoilDamage.Cast(sim, target)
	for until := sim.CurrentTime + 1500*time.Millisecond; sim.CurrentTime < until && !sim.Step(); {
	}
	if dk.RuneWeapon.DeathCoil.SpellMetrics[target.UnitIndex].Casts != 1 {
		t.Fatalf("the weapon copied the Death Coil %d times, want once", dk.RuneWeapon.DeathCoil.SpellMetrics[target.UnitIndex].Casts)
	}
	weapon := &dk.RuneWeapon.PseudoStats
	// an orc's Command is taken off the weapon, so its own percents are 1
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"all schools", weapon.DamageDealtMultiplier, 0.5},
		{"physical", weapon.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical], 1},
		{"shadow", weapon.SchoolDamageDealtMultiplier[shadow], 1},
	} {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Errorf("the weapon's %s multiplier after the copy is %v, want %v", c.name, c.got, c.want)
		}
	}
	for _, spell := range []*core.Spell{dk.DeathCoilDamage, dk.RuneWeapon.DeathCoil} {
		if got := spell.TravelTime(); got != 1249*time.Millisecond {
			t.Errorf("%s's Death Coil flies %v after a copy, want 1249 ms", spell.Unit.Label, got)
		}
	}
}

// runeWeaponCopies counts, by the weapon's spell, the DK's hits it copies that landed and that didn't while it was
// out, and the weapon's casts of each. landedAoE counts his Pestilences and Blood Boils that landed then, others
// what else the weapon cast.
type runeWeaponCopies struct {
	landed, missed, copies map[*core.Spell]int
	landedAoE              int
	others                 []string
}

// countRuneWeaponCopies fights that many targets on the rotation and counts what the weapon copied.
func countRuneWeaponCopies(t *testing.T, apl string, targets int, seed int64) runeWeaponCopies {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	for len(encounter.Targets) < targets {
		encounter.Targets = append(encounter.Targets, googleProto.Clone(encounter.Targets[0]).(*proto.Target))
	}
	player := parityPlayer(BloodTalents, "p3_blood", apl)
	// in front, the targets parry his strikes
	player.InFrontOfTarget = true
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: seed},
	})
	dk := sim.Raid.Parties[0].Players[0].(*DpsDeathknight)
	weapon := dk.RuneWeapon
	copyOf := map[*core.Spell]*core.Spell{
		dk.IcyTouch:          weapon.IcyTouch,
		dk.PlagueStrike:      weapon.PlagueStrike,
		dk.DeathStrike:       weapon.DeathStrike,
		dk.BloodStrike:       weapon.BloodStrike,
		dk.HeartStrike:       weapon.HeartStrike,
		dk.HeartStrikeOffHit: weapon.HeartStrike,
		dk.RuneStrike:        weapon.RuneStrike,
		dk.DeathCoilDamage:   weapon.DeathCoil,
	}
	c := runeWeaponCopies{landed: map[*core.Spell]int{}, missed: map[*core.Spell]int{}, copies: map[*core.Spell]int{}}
	drw := dk.GetAura("Dancing Rune Weapon")
	dk.RegisterAura(core.Aura{
		Label:    "rune weapon copy count",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(_ *core.Aura, _ *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !weapon.IsEnabled() || !drw.IsActive() {
				return
			}
			if copied, ok := copyOf[spell]; ok {
				if result.Landed() {
					c.landed[copied]++
				} else {
					c.missed[copied]++
				}
			} else if (spell == dk.Pestilence || spell == dk.BloodBoil) && result.Landed() {
				c.landedAoE++
			}
		},
	})
	sim.Reset()
	sim.PrePull()
	for !sim.Step() {
	}

	copies := map[*core.Spell]bool{}
	for _, copied := range copyOf {
		copies[copied] = true
	}
	// besides its copies: the diseases its Icy Touch and Plague Strike put on, its Heart Strike's chain and its
	// copies of white swings
	ownCasts := map[*core.Spell]bool{weapon.FrostFeverSpell: true, weapon.BloodPlagueSpell: true, weapon.HeartStrikeOffHit: true}
	for _, spell := range weapon.Spellbook {
		casts := 0
		for _, m := range spell.SpellMetrics {
			casts += int(m.Casts)
		}
		switch {
		case casts == 0, ownCasts[spell], spell.ActionID.OtherID == proto.OtherAction_OtherActionAttack:
		case copies[spell]:
			c.copies[spell] += casts
		default:
			c.others = append(c.others, spell.ActionID.String())
		}
	}
	sim.Cleanup()
	return c
}

// 49028 procs in the hit phase on spell_proc's default hit mask, so the weapon copies each normal or critical hit of
// the DK's while it's out, once for each target it lands on (Heart Strike's chain too), and nothing that missed or
// was dodged. Pestilence and Blood Boil are area spells, which CheckProc only takes off a hit on the DK himself, so
// it never copies them.
func TestRuneWeaponCopiesEachLandedHit(t *testing.T) {
	cast := func(id, tag int) string {
		return `{"action": {"castSpell": {"spellId": {"spellId": ` + strconv.Itoa(id) + `, "tag": ` + strconv.Itoa(tag) + `}}}}`
	}
	apl := func(casts ...string) string {
		return `{"type": "TypeAPL", "priorityList": [` + strings.Join(casts, ", ") + `]}`
	}
	strikes := apl(cast(49028, 0), cast(49909, 0), cast(49921, 1), cast(49924, 1), cast(55262, 1), cast(49895, 0))
	areas := apl(cast(49028, 0), cast(49909, 0), cast(49921, 1), cast(50842, 0), cast(49941, 0), cast(49930, 1))

	missed, landed, landedAoE := 0, 0, 0
	for seed := int64(1); seed <= 3; seed++ {
		for _, rotation := range []string{strikes, areas} {
			c := countRuneWeaponCopies(t, rotation, 2, seed)
			for spell := range c.copies {
				if _, ok := c.landed[spell]; !ok {
					c.landed[spell] = 0
				}
			}
			for spell, n := range c.landed {
				if c.copies[spell] != n {
					t.Errorf("seed %d: the weapon cast %s %d times for %d landed hits of the DK's", seed, spell.ActionID,
						c.copies[spell], n)
				}
				landed += n
			}
			for _, n := range c.missed {
				missed += n
			}
			landedAoE += c.landedAoE
			if len(c.others) > 0 {
				t.Errorf("seed %d: the weapon cast %v too", seed, c.others)
			}
		}
	}
	t.Logf("%d landed hits copied, %d missed ones not, %d area spells not", landed, missed, landedAoE)
	if missed == 0 {
		t.Error("none of the DK's hits missed with the weapon out, so nothing shows a miss isn't copied")
	}
	if landedAoE == 0 {
		t.Error("no Pestilence or Blood Boil of the DK's landed with the weapon out")
	}
}

// The weapon's strikes go through the DK's MeleeDamageBonusDone, so Rage of Rivendare and Tundra Stalker scale the
// whole hit. Its Icy Touch goes through his SpellDamageBonusDone, which never reads them.
func TestRuneWeaponStrikesTakeRageOfRivendare(t *testing.T) {
	dealt := func(bonus float64) map[string]float64 {
		sim, dk := runeWeaponSim(t)
		dk.RoRTSBonus = func(*core.Unit) float64 { return bonus }
		target := sim.GetTargetUnit(0)
		dk.RuneWeapon.EnableWithTimeout(sim, dk.RuneWeapon, 12*time.Second)
		w := dk.RuneWeapon
		damage := map[string]float64{}
		for _, spell := range []*core.Spell{w.HeartStrike, w.DeathStrike, w.PlagueStrike, w.BloodStrike, w.RuneStrike, w.IcyTouch} {
			metrics := &spell.SpellMetrics[target.UnitIndex]
			for range 10 {
				before := metrics.TotalDamage
				spell.Cast(sim, target)
				damage[spell.ActionID.String()] += metrics.TotalDamage - before
			}
		}
		return damage
	}
	plain, under := dealt(1), dealt(1.1)
	for name, d := range plain {
		want := d * 1.1
		if strings.HasPrefix(name, "{SpellID: "+strconv.Itoa(49909)) {
			want = d
		}
		// ten hits, each whole
		if d == 0 || math.Abs(under[name]-want) > 10 {
			t.Errorf("%s dealt %v under a 10%% bonus and %v without, want %v", name, under[name], d, want)
		}
	}
}

// 49028 puts its own aura on the DK, so the cast lands on him, a magic-class landing the Shard of the Gods sets
// roll on.
func TestDancingRuneWeaponLandsOnTheKnight(t *testing.T) {
	sim, dk := runeWeaponSim(t)
	var landed []*core.Unit
	dk.RegisterOnCastLanded(func(_ *core.Simulation, spell *core.Spell, target *core.Unit) {
		if spell == dk.DancingRuneWeapon {
			landed = append(landed, target)
		}
	})
	if !dk.DancingRuneWeapon.Cast(sim, dk.CurrentTarget) {
		t.Fatal("Dancing Rune Weapon wasn't cast")
	}
	if len(landed) != 1 || landed[0] != &dk.Unit {
		t.Errorf("landed on %d units, want the DK alone", len(landed))
	}
}
