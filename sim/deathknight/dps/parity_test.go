package dps

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/deathknight"
)

func parityPlayer(talents, gear, apl string) *proto.Player {
	return &proto.Player{
		Class:         proto.Class_ClassDeathknight,
		Race:          proto.Race_RaceOrc,
		Equipment:     core.GetGearSet("../../../ui/deathknight/gear_sets", gear).GearSet,
		TalentsString: talents,
		Rotation:      core.APLRotationFromJsonString(apl),
		Spec: &proto.Player_Deathknight{Deathknight: &proto.Deathknight{Options: &proto.Deathknight_Options{
			StartingRunicPower: 100,
			PetUptime:          1,
		}}},
		DistanceFromTarget: 5,
	}
}

func parityEnv(t *testing.T, player *proto.Player, settings *proto.ServerSettings) *deathknight.Deathknight {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.ServerSettings = settings
	env, _, _ := core.NewEnvironment(core.SinglePlayerRaidProto(player, nil, nil, nil), encounter, false)
	return env.Raid.Parties[0].Players[0].(*DpsDeathknight).Deathknight
}

const noRotation = `{"type": "TypeAPL", "priorityList": []}`

// the unholy tree of UnholyTalents with one talent's points set
func unholyWith(index int, points byte) string {
	trees := strings.Split(UnholyTalents, "-")
	unholy := []byte(trees[2])
	unholy[index] = points
	trees[2] = string(unholy)
	return strings.Join(trees, "-")
}

// mod-spell-tweaks' spell_dbc rows give Virulence and Nerves of Cold Steel 2% a rank and Rage of
// Rivendare 2 expertise a rank.
func TestServerTalentValues(t *testing.T) {
	statsWith := func(talents string) stats.Stats {
		return parityEnv(t, parityPlayer(talents, "p3_uh_dw", noRotation), nil).GetStats()
	}
	full := statsWith(UnholyTalents)

	// Virulence is the unholy tree's second talent, Rage of Rivendare its 30th.
	if got, want := full[stats.SpellHit]-statsWith(unholyWith(1, '0'))[stats.SpellHit], 6*core.SpellHitRatingPerHitChance; !core.WithinToleranceFloat64(want, got, 1e-6) {
		t.Errorf("Virulence 3/3 spell hit rating %.3f, want %.3f", got, want)
	}
	if got, want := full[stats.Expertise]-statsWith(unholyWith(29, '0'))[stats.Expertise], 10*core.ExpertisePerQuarterPercentReduction; !core.WithinToleranceFloat64(want, got, 1e-6) {
		t.Errorf("Rage of Rivendare 5/5 expertise rating %.3f, want %.3f", got, want)
	}

	// Nerves of Cold Steel is the frost tree's sixth talent; the gear dual wields.
	trees := strings.Split(UnholyTalents, "-")
	frost := []byte(trees[1])
	frost[5] = '0'
	noNerves := trees[0] + "-" + string(frost) + "-" + trees[2]
	if got, want := full[stats.MeleeHit]-statsWith(noNerves)[stats.MeleeHit], 6*core.MeleeHitRatingPerHitChance; !core.WithinToleranceFloat64(want, got, 1e-6) {
		t.Errorf("Nerves of Cold Steel 3/3 hit rating %.3f, want %.3f", got, want)
	}
}

// The risen ghouls, gargoyle and army carry 67561; the rune weapon and the bloodworms 61017. Only a
// risen ghoul takes the owner's armor pen.
func TestSummonScalingMarkers(t *testing.T) {
	unholy := parityEnv(t, parityPlayer(UnholyTalents, "p3_uh_dw", noRotation), nil)
	for _, c := range []struct {
		name  string
		pet   *core.Pet
		ghoul bool
	}{
		{"permanent ghoul", &unholy.Ghoul.Pet, true},
		{"army ghoul", &unholy.ArmyGhoul[0].Pet, false},
		{"gargoyle", &unholy.Gargoyle.Pet, false},
	} {
		if c.pet.HitScaling != core.PetHitScalingMasterSpell06 || c.pet.RisenGhoul != c.ghoul {
			t.Errorf("%s: hit scaling %v, risen ghoul %v", c.name, c.pet.HitScaling, c.pet.RisenGhoul)
		}
	}

	frost := parityEnv(t, parityPlayer(FrostTalents, "p3_frost", noRotation), nil)
	if g := &frost.Ghoul.Pet; g.HitScaling != core.PetHitScalingMasterSpell06 || !g.RisenGhoul || g.SummonedAsPet {
		t.Errorf("raise dead ghoul: hit scaling %v, risen ghoul %v, pet %v", g.HitScaling, g.RisenGhoul, g.SummonedAsPet)
	}

	blood := parityEnv(t, parityPlayer(BloodTalents, "p3_blood", noRotation), nil)
	if p := &blood.RuneWeapon.Pet; p.HitScaling != core.PetHitScalingDefault || p.RisenGhoul {
		t.Errorf("rune weapon: hit scaling %v, risen ghoul %v", p.HitScaling, p.RisenGhoul)
	}
	if p := &blood.Bloodworm[0].Pet; p.HitScaling != core.PetHitScalingDefault {
		t.Errorf("bloodworm: hit scaling %v", p.HitScaling)
	}
}

// Frost Fever crits with Runic Power Mastery, Blood Plague with Crypt Fever. With Epidemic and
// SpellTweaks.DiseaseHaste, melee haste buys both of them ticks.
func TestDiseaseDeclarations(t *testing.T) {
	for _, c := range []struct {
		name       string
		talents    string
		settings   *proto.ServerSettings
		critFF     bool
		critBP     bool
		addsTicks  bool
		tickLength time.Duration
	}{
		// Unholy: Runic Power Mastery 2, Crypt Fever 3, no Epidemic
		{"unholy", UnholyTalents, nil, true, true, false, 3 * time.Second},
		// Frost UH: Runic Power Mastery 2, Epidemic 2, and Blood Plague crits off the gear's T9 4pc
		{"frost uh", FrostUHTalents, nil, true, true, true, 3 * time.Second},
		{"frost uh without the tweak", FrostUHTalents, &proto.ServerSettings{SpellTweaks: &proto.SpellTweaksSettings{DiseaseHaste: new(bool)}}, true, true, false, 3 * time.Second},
		// Blood: neither talent, and no T9 in its own gear
		{"blood", BloodTalents, nil, false, false, true, 3 * time.Second},
	} {
		gear := "p3_frost"
		if c.name == "blood" {
			gear = "p1_blood"
		}
		dk := parityEnv(t, parityPlayer(c.talents, gear, noRotation), c.settings)
		target := dk.Env.Encounter.TargetUnits[0]
		ff, bp := dk.FrostFeverSpell.Dot(target), dk.BloodPlagueSpell.Dot(target)
		if ff.TicksCanCrit != c.critFF || bp.TicksCanCrit != c.critBP {
			t.Errorf("%s: Frost Fever crits %v, Blood Plague crits %v", c.name, ff.TicksCanCrit, bp.TicksCanCrit)
		}
		for _, dot := range []*core.Dot{ff, bp} {
			if dot.AffectedByCastSpeed != c.addsTicks || (c.addsTicks && dot.TickHaste != core.MeleeHasteAddsTicks) || dot.TickLength != c.tickLength {
				t.Errorf("%s %v: hasted %v (%v), tick %v", c.name, dot.Spell.ActionID, dot.AffectedByCastSpeed, dot.TickHaste, dot.TickLength)
			}
		}
	}
}

// Death Coil's cast is a binary dummy; spell_dk_death_coil sends 47632, a missile that resists partially.
func TestDeathCoilDamageSpell(t *testing.T) {
	dk := parityEnv(t, parityPlayer(UnholyTalents, "p3_uh_dw", noRotation), nil)
	if !dk.DeathCoil.Flags.Matches(core.SpellFlagBinary) {
		t.Errorf("Death Coil cast isn't binary")
	}
	damage := dk.GetSpell(deathknight.DeathCoilDamageActionID)
	if damage == nil || damage.Flags.Matches(core.SpellFlagBinary) || damage.MissileSpeed != 24 {
		t.Fatalf("Death Coil damage spell: %+v", damage)
	}
}

var logLine = regexp.MustCompile(`^\[(-?[\d.]+)\] \[([^\]]*)\] (.*)$`)

type logEntry struct {
	at   time.Duration
	unit string
	text string
}

// runParity runs one logged iteration and returns its log lines.
func runParity(t *testing.T, player *proto.Player, seed int64) []logEntry {
	t.Helper()
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, Debug: true, RandomSeed: seed},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	var entries []logEntry
	for _, line := range strings.Split(res.Logs, "\n") {
		m := logLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		secs, _ := strconv.ParseFloat(m[1], 64)
		entries = append(entries, logEntry{time.Duration(secs*1000+0.5*sign(secs)) * time.Millisecond, strings.TrimSpace(m[2]), m[3]})
	}
	return entries
}

func sign(x float64) float64 {
	if x < 0 {
		return -1
	}
	return 1
}

func times(entries []logEntry, unitSuffix, text string) []time.Duration {
	var out []time.Duration
	for _, e := range entries {
		if strings.HasSuffix(e.unit, unitSuffix) && strings.HasPrefix(e.text, text) {
			out = append(out, e.at)
		}
	}
	return out
}

// Raise Dead's ghoul stands for Risen Ghoul Self Stun's 4.5 s, then npc_pet_dk_ghoul's CombatAI casts
// Claw 5 to 10 s apart.
func TestRaiseDeadGhoulAI(t *testing.T) {
	apl := `{"type": "TypeAPL", "priorityList": [{"action": {"castSpell": {"spellId": {"spellId": 46584}}}}]}`
	for seed := int64(1); seed <= 5; seed++ {
		entries := runParity(t, parityPlayer(FrostTalents, "p3_frost", apl), seed)
		raised := times(entries, "(#1)", "Casting {SpellID: 46584}")
		swings := times(entries, "Ghoul", "Casting {OtherID: 3, Tag: 1}")
		claws := times(entries, "Ghoul", "Casting {SpellID: 47468}")
		if len(raised) == 0 || len(swings) == 0 || len(claws) < 3 {
			t.Fatalf("seed %d: raised %v, %d swings, %d claws", seed, raised, len(swings), len(claws))
		}
		engage := swings[0]
		if engage < raised[0]+4500*time.Millisecond || engage > raised[0]+4600*time.Millisecond {
			t.Errorf("seed %d: first swing %v after a raise at %v", seed, engage, raised[0])
		}
		if claws[0] < engage+5*time.Second || claws[0] > engage+10100*time.Millisecond {
			t.Errorf("seed %d: first Claw %v, engaged %v", seed, claws[0], engage)
		}
		for i := 1; i < len(claws); i++ {
			if gap := claws[i] - claws[i-1]; claws[i] < raised[0]+time.Minute && (gap < 5*time.Second || gap > 10100*time.Millisecond) {
				t.Errorf("seed %d: Claws %v apart", seed, gap)
			}
		}
	}
}

// npc_pet_dk_ebon_gargoyle decides every 400 ms from its summon, casts from 2 s on once it has landed,
// and flies off 32 s in.
func TestGargoyleAI(t *testing.T) {
	apl := `{"type": "TypeAPL", "priorityList": [{"action": {"castSpell": {"spellId": {"spellId": 49206}}}}]}`
	for seed := int64(1); seed <= 5; seed++ {
		entries := runParity(t, parityPlayer(UnholyTalents, "p3_uh_dw", apl), seed)
		summoned := times(entries, "(#1)", "Casting {SpellID: 49206}")
		casts := times(entries, "Gargoyle", "Casting {SpellID: 51963}")
		if len(summoned) == 0 || len(casts) < 10 {
			t.Fatalf("seed %d: summoned %v, %d casts", seed, summoned, len(casts))
		}
		s := summoned[0]
		for _, at := range casts {
			if at > s+3*time.Minute {
				break
			}
			since := at - s
			if since < 2500*time.Millisecond || since >= 32*time.Second {
				t.Errorf("seed %d: cast %v after the summon", seed, since)
			}
			// on a decision, which lands on the first server tick at or after it (the log keeps 10 ms)
			if off := since % (400 * time.Millisecond); off > 110*time.Millisecond && off < 390*time.Millisecond {
				t.Errorf("seed %d: cast %v after the summon, %v past a decision", seed, since, off)
			}
		}
	}
}

// Blood Tap's interrupt flags restart both melee timers, off the GCD.
func TestBloodTapResetsSwings(t *testing.T) {
	apl := `{"type": "TypeAPL", "priorityList": [{"action": {"condition": {"cmp": {"op": "OpGe", "lhs": {"currentTime": {}}, "rhs": {"const": {"val": "3.05s"}}}}, "castSpell": {"spellId": {"spellId": 45529}}}}]}`
	for seed := int64(1); seed <= 5; seed++ {
		player := parityPlayer(FrostTalents, "p3_frost", apl)
		dk := parityEnv(t, player, nil)
		swingTime := time.Duration(float64(time.Second) * dk.AutoAttacks.MH().SwingSpeed / dk.SwingSpeed())

		entries := runParity(t, player, seed)
		tapped := times(entries, "(#1)", "Casting {SpellID: 45529}")
		swings := times(entries, "(#1)", "Casting {OtherID: 3, Tag: 1}")
		if len(tapped) == 0 {
			t.Fatalf("seed %d: no Blood Tap", seed)
		}
		for _, at := range swings {
			if at > tapped[0] {
				if at < tapped[0]+swingTime-10*time.Millisecond {
					t.Errorf("seed %d: swung %v after Blood Tap, a swing takes %v", seed, at-tapped[0], swingTime)
				}
				break
			}
		}
	}
}

// Army of the Dead's channel holds melee without resetting it: nothing swings for its 4 s, and a
// swing that came due goes on the first server tick of its end.
func TestArmyOfTheDeadHoldsMelee(t *testing.T) {
	apl := `{"type": "TypeAPL", "priorityList": [{"action": {"condition": {"cmp": {"op": "OpGe", "lhs": {"currentTime": {}}, "rhs": {"const": {"val": "5s"}}}}, "castSpell": {"spellId": {"spellId": 42650}}}}]}`
	for seed := int64(1); seed <= 5; seed++ {
		entries := runParity(t, parityPlayer(UnholyTalents, "p3_uh_dw", apl), seed)
		cast := times(entries, "(#1)", "Casting {SpellID: 42650}")
		if len(cast) == 0 {
			t.Fatalf("seed %d: no Army of the Dead", seed)
		}
		end := cast[0] + 4*time.Second
		var after []time.Duration
		for _, at := range times(entries, "(#1)", "Casting {OtherID: 3, Tag: 1}") {
			if at > cast[0] && at < end {
				t.Errorf("seed %d: swung %v into the channel", seed, at-cast[0])
			}
			if at >= end {
				after = append(after, at)
			}
		}
		// both weapons swing faster than the 4 s channel, so the main hand comes due in it
		if len(after) == 0 || after[0] < end || after[0] > end+110*time.Millisecond {
			t.Errorf("seed %d: first swing after the channel at %v, the channel ended at %v", seed, after, end)
		}
	}
}
