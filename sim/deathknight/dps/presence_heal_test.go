package dps

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

var (
	damageLine = regexp.MustCompile(`^\[[^\]]*\] (\{[^}]*\}) (tick )?.* for ([\d.]+) damage`)
	healLine   = regexp.MustCompile(`^Gained ([\d.]+) health from \{SpellID: 50475\}`)
)

// bloodPresenceHealSources are the DK's direct hits that proc 63611: auto attacks, the spells the DK casts,
// and the triggered ones with NOT_A_PROC (Death Coil's damage, Necrosis).
var bloodPresenceHealSources = map[string]bool{
	"{OtherID: 3, Tag: 1}":     true, // main hand swing
	"{OtherID: 3, Tag: 2}":     true, // off hand swing
	"{SpellID: 49909}":         true, // Icy Touch
	"{SpellID: 49921, Tag: 1}": true, // Plague Strike
	"{SpellID: 49930, Tag: 1}": true, // Blood Strike
	"{SpellID: 51425, Tag: 1}": true, // Obliterate
	"{SpellID: 55268, Tag: 1}": true, // Frost Strike
	"{SpellID: 55271, Tag: 1}": true, // Scourge Strike
	"{SpellID: 47632}":         true, // Death Coil
	"{SpellID: 51460}":         true, // Necrosis
	"{SpellID: 56350}":         true, // Saronite Bomb
	"{SpellID: 56488}":         true, // Global Thermal Sapper Charge
	"{SpellID: 55039}":         true, // Gnomish Lightning Generator
}

// deathAndDecay's ticks are triggered casts of 52212, so they proc nothing.
const (
	deathAndDecay      = "{SpellID: 49938}"
	deathAndDecay49938 = `{"spellId": 49938}`
)

// presenceHealRotation goes into the presence before the pull, casts the opener a GCD apart, then whatever
// of the rest is ready.
func presenceHealRotation(presence int, opener, rest []string) string {
	timeIs := func(op string, seconds float64) string {
		return `{"cmp": {"op": "` + op + `", "lhs": {"currentTime": {}}, "rhs": {"const": {"val": "` + strconv.FormatFloat(seconds, 'g', -1, 64) + `s"}}}}`
	}
	var actions []string
	start := 1.0
	for _, spell := range opener {
		actions = append(actions, `{"action": {"condition": {"and": {"vals": [`+timeIs("OpGe", start)+`, `+timeIs("OpLt", start+0.9)+
			`]}}, "castSpell": {"spellId": `+spell+`}}}`)
		start += 1.5
	}
	for _, spell := range rest {
		actions = append(actions, `{"action": {"condition": `+timeIs("OpGe", start)+`, "castSpell": {"spellId": `+spell+`}}}`)
	}
	return `{"type": "TypeAPL", "prepullActions": [{"action": {"castSpell": {"spellId": {"spellId": ` + strconv.Itoa(presence) +
		`}}}, "doAtValue": {"const": {"val": "-10s"}}}], "priorityList": [` + strings.Join(actions, ", ") + `]}`
}

// checkPresenceHeals holds every 50475 heal in the log to pct percent of a hit that procs it, whole, times
// the healing taken, whole again, and every such hit to its heal. It returns how many hits of each action
// healed.
func checkPresenceHeals(t *testing.T, entries []logEntry, pct, taken float64) map[string]int {
	t.Helper()
	want, got := map[time.Duration][]float64{}, map[time.Duration][]float64{}
	healed := map[string]int{}
	for _, e := range entries {
		if !strings.HasSuffix(e.unit, "(#1)") {
			continue
		}
		if m := healLine.FindStringSubmatch(e.text); m != nil {
			amount, _ := strconv.ParseFloat(m[1], 64)
			got[e.at] = append(got[e.at], amount)
			continue
		}
		m := damageLine.FindStringSubmatch(e.text)
		if m == nil {
			continue
		}
		action, tick := m[1], m[2] != ""
		if tick && action == deathAndDecay || !tick && !bloodPresenceHealSources[action] {
			continue
		}
		damage, _ := strconv.ParseFloat(m[3], 64)
		if damage > 0 && pct > 0 {
			want[e.at] = append(want[e.at], math.Trunc(math.Trunc(damage*pct/100)*taken))
			if tick {
				action += " tick"
			}
			healed[action]++
		}
	}
	for at := range got {
		if _, ok := want[at]; !ok {
			want[at] = nil
		}
	}
	for at, amounts := range want {
		slices.Sort(amounts)
		slices.Sort(got[at])
		if !slices.Equal(amounts, got[at]) {
			t.Errorf("at %v: heals %v, want %v", at, got[at], amounts)
		}
	}
	return healed
}

// 63611 heals every death knight in Blood Presence for 4% of the damage of each swing, cast and dot tick. The
// off-hand strikes, Razorice, Blood-Caked Blade, Scourge Strike's shadow part, Wandering Plague and Death and
// Decay's ticks are triggered spells, which don't proc it.
func TestBloodPresenceHeal(t *testing.T) {
	const bloodPresence = 48266
	const icyTouch, plagueStrike = `{"spellId": 49909}`, `{"spellId": 49921, "tag": 1}`
	frost := presenceHealRotation(bloodPresence, []string{icyTouch, plagueStrike},
		[]string{`{"spellId": 51425, "tag": 1}`, bloodStrike, `{"spellId": 55268, "tag": 1}`})
	healed := checkPresenceHeals(t, runParity(t, parityPlayer(FrostTalents, "p3_frost", frost), 1), 4, 1)
	for _, action := range []string{"{OtherID: 3, Tag: 1}", "{OtherID: 3, Tag: 2}", "{SpellID: 51425, Tag: 1}",
		"{SpellID: 55268, Tag: 1}", "{SpellID: 55095} tick", "{SpellID: 55078} tick"} {
		if healed[action] == 0 {
			t.Errorf("frost: no %s healed", action)
		}
	}

	unholy := presenceHealRotation(bloodPresence, []string{deathAndDecay49938, icyTouch, plagueStrike},
		[]string{`{"spellId": 55271, "tag": 1}`, bloodStrike, `{"spellId": 49895}`})
	entries := runParity(t, parityPlayer(UnholyTalents, "p3_uh_dw", unholy), 1)
	healed = checkPresenceHeals(t, entries, 4, 1)
	for _, action := range []string{"{SpellID: 55271, Tag: 1}", "{SpellID: 47632}", "{SpellID: 51460}", "{SpellID: 50536} tick"} {
		if healed[action] == 0 {
			t.Errorf("unholy: no %s healed", action)
		}
	}
	if len(times(entries, "(#1)", "[Target 1] "+deathAndDecay+" tick")) == 0 {
		t.Error("unholy: Death and Decay never ticked, so nothing shows its ticks don't heal")
	}
}

// In Frost and Unholy Presence only Improved Blood Presence keeps the heal, at 2% a point.
func TestImprovedBloodPresenceHeal(t *testing.T) {
	const frostPresence = 48263
	rotation := presenceHealRotation(frostPresence, []string{`{"spellId": 49909}`, `{"spellId": 49921, "tag": 1}`},
		[]string{`{"spellId": 51425, "tag": 1}`})
	// Improved Blood Presence is the blood tree's 20th talent
	for points, pct := range []float64{0, 2, 4} {
		healed := checkPresenceHeals(t, runParity(t, parityPlayer(talentsWith(FrostTalents, 0, 19, byte('0'+points)), "p3_frost", rotation), 1), pct, 1)
		if points > 0 && healed["{OtherID: 3, Tag: 1}"] == 0 {
			t.Errorf("%d points: no swing healed", points)
		}
	}
}

// improvedBloodPresence puts points into Improved Blood Presence, the blood tree's 20th talent.
func improvedBloodPresence(talents string, points int) string {
	return talentsWith(talents, 0, 19, byte('0'+points))
}

func swingRotation(presence int) string {
	return presenceHealRotation(presence, []string{`{"spellId": 49909}`, `{"spellId": 49921, "tag": 1}`}, []string{`{"spellId": 51425, "tag": 1}`})
}

// Improved Blood Presence's second effect adds 5 a point to Blood Presence's healing taken aura (48266's
// effect 1, 0 by itself), so the heal grows only in Blood Presence.
func TestImprovedBloodPresenceHealingTaken(t *testing.T) {
	const bloodPresence, frostPresence = 48266, 48263
	for points, taken := range []float64{1, 1.05, 1.10} {
		player := parityPlayer(improvedBloodPresence(FrostTalents, points), "p3_frost", swingRotation(bloodPresence))
		if healed := checkPresenceHeals(t, runParity(t, player, 1), 4, taken); healed["{OtherID: 3, Tag: 1}"] == 0 {
			t.Errorf("%d points: no swing healed", points)
		}
	}
	player := parityPlayer(improvedBloodPresence(FrostTalents, 2), "p3_frost", swingRotation(frostPresence))
	if healed := checkPresenceHeals(t, runParity(t, player, 1), 4, 1); healed["{OtherID: 3, Tag: 1}"] == 0 {
		t.Error("Frost Presence: no swing healed")
	}
}

// 50475 has no damage class and no spell_bonus_data row, so it takes no healing done percent: Strength of
// Wrynn's 30% leaves it alone, where Improved Blood Presence's healing taken still counts.
func TestBloodPresenceHealSkipsStrengthOfWrynn(t *testing.T) {
	wrynn := &proto.RaidBuffs{StrengthOfWrynn: true}
	for points, taken := range []float64{1, 1.05, 1.10} {
		player := parityPlayer(improvedBloodPresence(FrostTalents, points), "p3_frost", swingRotation(48266))
		entries := runParityIn(t, player, wrynn, func(*proto.Target) {}, 1)
		if healed := checkPresenceHeals(t, entries, 4, taken); healed["{OtherID: 3, Tag: 1}"] == 0 {
			t.Errorf("%d points: no swing healed", points)
		}
	}
}

// 63611 asks for a target that gives experience or honor (PROC_ATTR_REQ_EXP_OR_HONOR): one above the gray
// level, 71 for a level 80.
func TestBloodPresenceHealNeedsATargetAboveGray(t *testing.T) {
	for level, heals := range map[int32]bool{71: false, 72: true} {
		player := parityPlayer(FrostTalents, "p3_frost", swingRotation(48266))
		entries := runParityIn(t, player, nil, func(target *proto.Target) { target.Level = level }, 1)
		healed := 0
		for _, e := range entries {
			if healLine.MatchString(e.text) {
				healed++
			}
		}
		if heals != (healed > 0) {
			t.Errorf("level %d target: %d heals", level, healed)
		}
		if heals {
			checkPresenceHeals(t, entries, 4, 1)
		}
	}
}

// An item's use spell is an untriggered cast (Player::CastItemUseSpell), so a bomb's damage heals like the
// DK's own spells, the sapper's on the DK too.
func TestBloodPresenceHealFromOnUseItems(t *testing.T) {
	explosives := `{"type": "TypeAPL", "prepullActions": [{"action": {"castSpell": {"spellId": {"spellId": 48266}}}, "doAtValue": {"const": {"val": "-10s"}}}],` +
		` "priorityList": [{"action": {"autocastOtherCooldowns": {}}}]}`
	player := parityPlayer(FrostTalents, "p3_frost", explosives)
	player.Profession1 = proto.Profession_Engineering
	player.Consumes = &proto.Consumes{ThermalSapper: true, FillerExplosive: proto.Explosive_ExplosiveSaroniteBomb}
	player.Equipment.Items[proto.ItemSlot_ItemSlotTrinket1] = &proto.ItemSpec{Id: 41121}
	healed := checkPresenceHeals(t, runParity(t, player, 1), 4, 1)
	for _, action := range []string{"{SpellID: 56350}", "{SpellID: 56488}", "{SpellID: 55039}"} {
		if healed[action] == 0 {
			t.Errorf("no %s hit healed", action)
		}
	}
}

// A NO_XP creature (the simval dummies) gives no experience or honor whatever its level, so 63611 never
// procs on it.
func TestBloodPresenceHealNeedsAnXPTarget(t *testing.T) {
	for _, noXP := range []bool{false, true} {
		player := parityPlayer(FrostTalents, "p3_frost", swingRotation(48266))
		entries := runParityIn(t, player, nil, func(target *proto.Target) { target.NoXp = noXP }, 1)
		healed := 0
		for _, e := range entries {
			if healLine.MatchString(e.text) {
				healed++
			}
		}
		if noXP != (healed == 0) {
			t.Errorf("NO_XP %v target: %d heals", noXP, healed)
		}
	}
}

var deathStrikeHealLine = regexp.MustCompile(`^Gained ([\d.]+) health from \{SpellID: 49924\}`)

// Death Strike's heal (45470) has no damage class and no spell_bonus_data row either, so Strength of Wrynn's
// healing done doesn't reach it: only its 30% more health, which the heal is a share of, does.
func TestDeathStrikeHealSkipsStrengthOfWrynn(t *testing.T) {
	rotation := presenceHealRotation(48266, []string{`{"spellId": 49909}`, `{"spellId": 49921, "tag": 1}`},
		[]string{`{"spellId": 49924, "tag": 1}`})
	heals := func(buffs *proto.RaidBuffs) []float64 {
		player := parityPlayer(FrostTalents, "p3_frost", rotation)
		var amounts []float64
		for _, e := range runParityIn(t, player, buffs, func(*proto.Target) {}, 1) {
			if m := deathStrikeHealLine.FindStringSubmatch(e.text); m != nil {
				if amount, _ := strconv.ParseFloat(m[1], 64); amount > 0 {
					amounts = append(amounts, amount)
				}
			}
		}
		return amounts
	}
	plain, wrynn := heals(nil), heals(&proto.RaidBuffs{StrengthOfWrynn: true})
	if len(plain) == 0 {
		t.Fatal("no Death Strike healed")
	}
	if len(wrynn) != len(plain) {
		t.Fatalf("%d heals under Strength of Wrynn, %d without", len(wrynn), len(plain))
	}
	for i := range plain {
		if ratio := wrynn[i] / plain[i]; ratio < 1.29 || ratio > 1.31 {
			t.Errorf("heal %d: %v under Strength of Wrynn, %v without, want x1.3 for the health", i, wrynn[i], plain[i])
		}
	}
}
