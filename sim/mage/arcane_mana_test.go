package mage

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

var (
	arcaneHasteCostOff  = &proto.ServerSettings{SpellTweaks: &proto.SpellTweaksSettings{ArcaneBlastHasteCost: googleProto.Bool(false)}}
	arcaneCritRefundOff = &proto.ServerSettings{SpellTweaks: &proto.SpellTweaksSettings{ArcaneCritRefund: googleProto.Bool(false)}}
)

// ArcaneTalents with this many points of Netherwind Presence and Spell Power, the arcane tree's 28th and
// 29th talents
func arcaneTalentsWith(netherwindPresence, spellPower byte) string {
	trees := strings.SplitN(ArcaneTalents, "-", 2)
	arcane := []byte(trees[0])
	arcane[27] = '0' + netherwindPresence
	arcane[28] = '0' + spellPower
	return string(arcane) + "-" + trees[1]
}

// arcaneSim is a gearless arcane mage casting at the given total haste, 1.3 for 30%, its talents' included.
func arcaneSim(t *testing.T, talents string, settings *proto.ServerSettings, haste float64) (*core.Simulation, *Mage, *core.Unit) {
	t.Helper()
	sim, mage, target := gearlessSim(t, talents, PlayerOptionsArcane, settings)
	mage.MultiplyCastSpeed(haste * mage.CastSpeed)
	return sim, mage, target
}

// arcaneBlastCost is the server's arithmetic for an Arcane Blast at these stacks: 7% of base mana, the stacks'
// percent as one spell mod, then Arcane Focus's and Precision's school cut, each step in whole mana.
func arcaneBlastCost(mage *Mage, stackPct, stacks int) float64 {
	base := int(mage.BaseMana) * 7 / 100
	cut := int(mage.Talents.ArcaneFocus + mage.Talents.Precision)
	return float64(base * (100 + stacks*stackPct) / 100 * (100 - cut) / 100)
}

// a Clearcasting proc would make the next cost 0
func dropClearcasting(sim *core.Simulation, mage *Mage) {
	if mage.ClearcastingAura.IsActive() {
		mage.ClearcastingAura.Deactivate(sim)
	}
}

// spell_tweaks_arcane_blast_haste_cost: with Netherwind Presence each Arcane Blast stack adds int32(175 x cast
// speed) percent to the cost, and a slow changes nothing. SpellTweaks.ArcaneBlastHasteCost and
// SpellTweaks.Enable gate it; the refund's switch doesn't.
func TestArcaneBlastStackCost(t *testing.T) {
	for _, c := range []struct {
		name     string
		talents  string
		settings *proto.ServerSettings
		haste    float64
		stackPct int
	}{
		{"Netherwind Presence alone", ArcaneTalents, nil, 1.06, 165},
		{"25% haste", ArcaneTalents, nil, 1.25, 140},
		{"30% haste", ArcaneTalents, nil, 1.3, 134},
		{"50% haste", ArcaneTalents, nil, 1.5, 116},
		{"slowed", ArcaneTalents, nil, 0.8, 175},
		{"one point of Netherwind Presence", arcaneTalentsWith(1, 2), nil, 1.3, 134},
		{"refund switch off", ArcaneTalents, arcaneCritRefundOff, 1.3, 134},
		{"switch off", ArcaneTalents, arcaneHasteCostOff, 1.3, 175},
		{"spell tweaks off", ArcaneTalents, spellTweaksOff, 1.3, 175},
		{"no Netherwind Presence", arcaneTalentsWith(0, 2), nil, 1.3, 175},
	} {
		sim, mage, target := arcaneSim(t, c.talents, c.settings, c.haste)
		if got := mage.Talents.NetherwindPresence > 0; got != (c.talents != arcaneTalentsWith(0, 2)) {
			t.Fatalf("%s: %d points of Netherwind Presence", c.name, mage.Talents.NetherwindPresence)
		}
		blast := mage.ArcaneBlast
		cost := func() float64 { return blast.ApplyCostModifiers(blast.DefaultCast.Cost) }

		// the fifth cast refreshes four stacks
		for cast := 1; cast <= 5; cast++ {
			blast.SkipCastAndApplyEffects(sim, target)
			dropClearcasting(sim, mage)
			stacks := min(cast, 4)
			if got := mage.ArcaneBlastAura.GetStacks(); int(got) != stacks {
				t.Fatalf("%s: %d stacks after %d casts", c.name, got, cast)
			}
			if got, want := cost(), arcaneBlastCost(mage, c.stackPct, stacks); got != want {
				t.Errorf("%s: Arcane Blast costs %v at %d stacks, want %v (%d%% a stack)", c.name, got, stacks, want, c.stackPct)
			}
		}

		mage.ArcaneBlastAura.Deactivate(sim)
		if got, want := cost(), arcaneBlastCost(mage, 0, 0); got != want || math.Abs(blast.CostMultiplier-1) > 1e-9 {
			t.Errorf("%s: Arcane Blast costs %v with the stacks gone (multiplier %v), want %v", c.name, got, blast.CostMultiplier, want)
		}
	}
}

// The stacks' cost is worked out as an Arcane Blast lands and kept until the next one lands (Aura::SetStackAmount),
// at four stacks too. Haste gained in between changes neither the cast under way nor the next cast's cost.
func TestArcaneBlastStackCostReadsHasteAsItLands(t *testing.T) {
	sim, mage, target := arcaneSim(t, ArcaneTalents, nil, 1.06)
	blast := mage.ArcaneBlast
	casts := &blast.SpellMetrics[target.UnitIndex].Casts
	var spent float64
	var landed int32
	start := func() {
		t.Helper()
		// the global cooldown ends a little after the last cast landed, with nothing queued to step to
		sim.CurrentTime = max(sim.CurrentTime, mage.NextGCDAt())
		spent, landed = mage.Metrics.ManaSpent, *casts
		if !blast.Cast(sim, target) {
			t.Fatalf("Arcane Blast didn't cast at %v with %v mana", sim.CurrentTime, mage.CurrentMana())
		}
	}
	// finish lands the cast under way and returns what it paid
	finish := func() float64 {
		t.Helper()
		for *casts == landed && !sim.Step() {
		}
		if *casts == landed {
			t.Fatal("the Arcane Blast never landed")
		}
		dropClearcasting(sim, mage)
		return mage.Metrics.ManaSpent - spent
	}
	land := func() float64 {
		t.Helper()
		start()
		return finish()
	}
	expect := func(when string, got float64, stackPct, stacks int) {
		t.Helper()
		if want := arcaneBlastCost(mage, stackPct, stacks); got != want {
			t.Errorf("%s: %v mana, want %v (%d stacks at %d%%)", when, got, want, stacks, stackPct)
		}
	}
	nextCost := func() float64 { return blast.ApplyCostModifiers(blast.DefaultCast.Cost) }

	// 6% haste gives 165% a stack, 30% on top of it 126%
	expect("the first cast", land(), 0, 0)
	expect("after one stack", nextCost(), 165, 1)

	mage.MultiplyCastSpeed(1.3)
	expect("haste gained at one stack", nextCost(), 165, 1)
	expect("the cast after the haste", land(), 165, 1)
	expect("after the hasted stack", nextCost(), 126, 2)

	mage.MultiplyCastSpeed(1 / 1.3)
	expect("haste lost at two stacks", nextCost(), 126, 2)
	expect("the cast after the loss", land(), 126, 2)
	expect("the fourth cast", land(), 165, 3)
	expect("at four stacks", nextCost(), 165, 4)

	// haste gained mid-cast: the cast pays what it started at, and its refresh takes the new speed
	start()
	mage.MultiplyCastSpeed(1.3)
	expect("a cast that gains haste on the way", finish(), 165, 4)
	expect("after the refresh at four stacks", nextCost(), 126, 4)
}

var (
	castCostLine     = regexp.MustCompile(`^Casting \{SpellID: (\d+)\} \(Cost = ([\d.]+),`)
	critLine         = regexp.MustCompile(`^\[Target \d+\] \{SpellID: (\d+)\} Crit for`)
	refundLine       = regexp.MustCompile(`^Gained ([\d.]+) mana from \{SpellID: (\d+)\}`)
	refundThreatLine = regexp.MustCompile(`^\{SpellID: (\d+)\} assist threat ([\d.]+) over`)
)

const (
	arcaneMissilesChannel = "42846"
	arcaneMissile         = "42845"
)

type arcaneRefundCounts struct {
	crits   int // casts with a crit, a missile counting as one
	refunds int
	// crits of a free cast, and missile refunds of a free channel
	freeCrits, freeChannelRefunds int
	// casts that crit more than one target
	multiCrits int
}

// checkArcaneRefunds holds a log to spell_tweaks_arcane_crit_refund for a mage with the talent of that id:
// each cast that crits gives back pct of what it paid, once, so nothing when it was free. A missile gives
// missileRefund whatever its channel paid. Each refund makes half its amount in threat.
func checkArcaneRefunds(t *testing.T, label string, entries []procLogEntry, talent string, pct, missileRefund float64) arcaneRefundCounts {
	t.Helper()
	var counts arcaneRefundCounts
	paid := map[string]float64{}
	type cast struct {
		at    time.Duration
		spell string
	}
	critTargets := map[cast]int{}
	want, got, threat := map[time.Duration][]float64{}, map[time.Duration][]float64{}, map[time.Duration][]float64{}

	for _, e := range entries {
		if m := castCostLine.FindStringSubmatch(e.text); m != nil {
			paid[m[1]], _ = strconv.ParseFloat(m[2], 64)
		} else if m := critLine.FindStringSubmatch(e.text); m != nil {
			key := cast{e.at, m[1]}
			if critTargets[key]++; critTargets[key] == 2 {
				counts.multiCrits++
			}
			if critTargets[key] > 1 {
				continue
			}
			counts.crits++
			refund := math.Floor(paid[m[1]] * pct / 100)
			if m[1] == arcaneMissile {
				refund = missileRefund
				if paid[arcaneMissilesChannel] == 0 {
					counts.freeChannelRefunds++
				}
			} else if paid[m[1]] == 0 {
				counts.freeCrits++
			}
			if refund > 0 {
				want[e.at] = append(want[e.at], refund)
			}
		} else if m := refundLine.FindStringSubmatch(e.text); m != nil && m[2] == talent {
			amount, _ := strconv.ParseFloat(m[1], 64)
			got[e.at] = append(got[e.at], amount)
			counts.refunds++
		} else if m := refundThreatLine.FindStringSubmatch(e.text); m != nil && m[1] == talent {
			amount, _ := strconv.ParseFloat(m[2], 64)
			threat[e.at] = append(threat[e.at], amount*2)
		}
	}

	times := map[time.Duration]bool{}
	for _, byTime := range []map[time.Duration][]float64{want, got, threat} {
		for at, amounts := range byTime {
			slices.Sort(amounts)
			times[at] = true
		}
	}
	for at := range times {
		if !slices.Equal(got[at], want[at]) {
			t.Errorf("%s, %v: refunds %v, want %v", label, at, got[at], want[at])
		}
		if !slices.Equal(threat[at], got[at]) {
			t.Errorf("%s, %v: threat for refunds of %v, with %v refunded", label, at, threat[at], got[at])
		}
	}
	return counts
}

// spell_tweaks_arcane_crit_refund, on Arcane Blast and Arcane Missiles: 30% of what the crit's cast paid with two
// points of Spell Power and 15% with one, so nothing under Clearcasting. A missile takes a sixth of the channel's
// base cost (trunc(1013 / 6) = 168, so 50 and 25), a free channel's too.
func TestArcaneCritRefund(t *testing.T) {
	const fight = 120
	apl := castApl(auraUp(44401), arcaneMissilesChannel, "", "42897")

	var sum arcaneRefundCounts
	for seed := int64(1); seed <= 3; seed++ {
		entries := runProcLogOn(t, procMage(ArcaneTalents, "p3_arcane_alliance", apl, PlayerOptionsArcane), nil, fight, seed)
		counts := checkArcaneRefunds(t, "two points, seed "+strconv.FormatInt(seed, 10), entries, "35581", 30, 50)
		sum.crits += counts.crits
		sum.refunds += counts.refunds
		sum.freeCrits += counts.freeCrits
		sum.freeChannelRefunds += counts.freeChannelRefunds
	}
	if sum.refunds == 0 || sum.freeCrits == 0 || sum.freeChannelRefunds == 0 {
		t.Errorf("two points: %d refunds for %d crits, %d of them free casts, and %d missile refunds of a free channel",
			sum.refunds, sum.crits, sum.freeCrits, sum.freeChannelRefunds)
	}

	entries := runProcLogOn(t, procMage(arcaneTalentsWith(3, 1), "p3_arcane_alliance", apl, PlayerOptionsArcane), nil, fight, 1)
	if counts := checkArcaneRefunds(t, "one point", entries, "35578", 15, 25); counts.refunds == 0 {
		t.Errorf("one point: no refund for %d crits", counts.crits)
	}

	// the other tweak's switch leaves the refund alone
	entries = runProcLogOn(t, procMage(ArcaneTalents, "p3_arcane_alliance", apl, PlayerOptionsArcane), arcaneHasteCostOff, fight, 1)
	if counts := checkArcaneRefunds(t, "haste cost off", entries, "35581", 30, 50); counts.refunds == 0 {
		t.Errorf("haste cost off: no refund for %d crits", counts.crits)
	}
}

// Arcane Barrage refunds as it lands, from what its cast paid. An Arcane Explosion that crits several targets
// refunds once (spell_proc -35578's 8 ms cooldown).
func TestArcaneCritRefundBarrageAndExplosion(t *testing.T) {
	const fight = 120
	barrage := procMage(ArcaneBarrageTalents, "p3_arcane_alliance", castApl("", "44781", "", "42897"), PlayerOptionsArcane)
	entries := runProcLogOn(t, barrage, nil, fight, 1)
	barrageCrits := len(logTimes(entries, "[Target 1] {SpellID: 44781} Crit for"))
	if counts := checkArcaneRefunds(t, "Arcane Barrage", entries, "35581", 30, 50); barrageCrits == 0 || counts.refunds == 0 {
		t.Errorf("Arcane Barrage: %d crits of it, %d refunds in all", barrageCrits, counts.refunds)
	}

	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = fight
	encounter.Targets = []*proto.Target{core.NewDefaultTarget(), core.NewDefaultTarget(), core.NewDefaultTarget()}
	explosion := procMage(ArcaneTalents, "p3_arcane_alliance", castApl("", "42921"), PlayerOptionsArcane)
	entries = runProcLogIn(t, explosion, encounter, 1)
	if counts := checkArcaneRefunds(t, "Arcane Explosion", entries, "35581", 30, 50); counts.multiCrits == 0 || counts.refunds == 0 {
		t.Errorf("Arcane Explosion: %d refunds, %d casts crit more than one target", counts.refunds, counts.multiCrits)
	}
}

// No refund with SpellTweaks.ArcaneCritRefund or SpellTweaks.Enable off, without Spell Power, or from another
// school's crits.
func TestArcaneCritRefundOff(t *testing.T) {
	arcane := castApl(auraUp(44401), arcaneMissilesChannel, "", "42897")
	for _, c := range []struct {
		name     string
		talents  string
		settings *proto.ServerSettings
		apl      string
		crit     string
	}{
		{"switch off", ArcaneTalents, arcaneCritRefundOff, arcane, "42897"},
		{"spell tweaks off", ArcaneTalents, spellTweaksOff, arcane, "42897"},
		{"no Spell Power", arcaneTalentsWith(3, 0), nil, arcane, "42897"},
		{"Fireball", ArcaneTalents, nil, castApl("", "42833"), "42833"},
		{"Frostbolt", ArcaneTalents, nil, castApl("", "42842"), "42842"},
	} {
		entries := runProcLogOn(t, procMage(c.talents, "p3_arcane_alliance", c.apl, PlayerOptionsArcane), c.settings, 120, 1)
		crits := len(logTimes(entries, "[Target 1] {SpellID: "+c.crit+"} Crit for"))
		refunds := 0
		for _, e := range entries {
			if m := refundLine.FindStringSubmatch(e.text); m != nil && (m[2] == "35578" || m[2] == "35581") {
				refunds++
			}
		}
		if crits == 0 || refunds != 0 {
			t.Errorf("%s: %d refunds for %d crits, want none", c.name, refunds, crits)
		}
	}
}
