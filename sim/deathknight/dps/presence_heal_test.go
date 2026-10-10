package dps

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
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

// checkPresenceHeals holds every 50475 heal in the log to pct percent of a hit that procs it, whole, and
// every such hit to its heal. It returns how many hits of each action healed.
func checkPresenceHeals(t *testing.T, entries []logEntry, pct float64) map[string]int {
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
			want[e.at] = append(want[e.at], math.Trunc(damage*pct/100))
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
	healed := checkPresenceHeals(t, runParity(t, parityPlayer(FrostTalents, "p3_frost", frost), 1), 4)
	for _, action := range []string{"{OtherID: 3, Tag: 1}", "{OtherID: 3, Tag: 2}", "{SpellID: 51425, Tag: 1}",
		"{SpellID: 55268, Tag: 1}", "{SpellID: 55095} tick", "{SpellID: 55078} tick"} {
		if healed[action] == 0 {
			t.Errorf("frost: no %s healed", action)
		}
	}

	unholy := presenceHealRotation(bloodPresence, []string{deathAndDecay49938, icyTouch, plagueStrike},
		[]string{`{"spellId": 55271, "tag": 1}`, bloodStrike, `{"spellId": 49895}`})
	entries := runParity(t, parityPlayer(UnholyTalents, "p3_uh_dw", unholy), 1)
	healed = checkPresenceHeals(t, entries, 4)
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
		healed := checkPresenceHeals(t, runParity(t, parityPlayer(talentsWith(FrostTalents, 0, 19, byte('0'+points)), "p3_frost", rotation), 1), pct)
		if points > 0 && healed["{OtherID: 3, Tag: 1}"] == 0 {
			t.Errorf("%d points: no swing healed", points)
		}
	}
}
