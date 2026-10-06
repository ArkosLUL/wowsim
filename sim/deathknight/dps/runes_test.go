package dps

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// castsIn is an APL that casts the spell, by its APL id, once in each window, given as start seconds.
func castsIn(spell string, starts ...float64) string {
	var actions []string
	for _, start := range starts {
		actions = append(actions, fmt.Sprintf(`{"action": {"condition": {"and": {"vals": [`+
			`{"cmp": {"op": "OpGe", "lhs": {"currentTime": {}}, "rhs": {"const": {"val": "%gs"}}}}, `+
			`{"cmp": {"op": "OpLt", "lhs": {"currentTime": {}}, "rhs": {"const": {"val": "%gs"}}}}]}}, `+
			`"castSpell": {"spellId": %s}}}`, start, start+0.9, spell))
	}
	return `{"type": "TypeAPL", "priorityList": [` + strings.Join(actions, ", ") + `]}`
}

const bloodStrike = `{"spellId": 49930, "tag": 1}`

// spell_dk_death_rune: a Blood of the North death rune reverts when the talent's 30 s periodic runs out,
// used or not, and each proc restarts it, pushing back every rune the talent turned.
func TestDeathRuneTalentRevert(t *testing.T) {
	reverts := func(entries []logEntry) []time.Duration {
		return times(entries, "(#1)", "Spent 1.000 death rune from {OtherID:")
	}
	near := func(got, want time.Duration) bool {
		return got >= want && got <= want+20*time.Millisecond
	}

	entries := runParity(t, parityPlayer(FrostTalents, "p3_frost", castsIn(bloodStrike, 1)), 1)
	strikes := times(entries, "(#1)", "Casting {SpellID: 49930, Tag: 1}")
	if len(strikes) != 1 {
		t.Fatalf("one Blood Strike: got %v", strikes)
	}
	if got := reverts(entries); len(got) != 1 || !near(got[0], strikes[0]+30*time.Second) {
		t.Errorf("Blood Strike at %v: death runes reverted at %v, want one 30 s on", strikes[0], got)
	}

	// the second strike takes the other blood rune and restarts the periodic, so both revert together
	entries = runParity(t, parityPlayer(FrostTalents, "p3_frost", castsIn(bloodStrike, 1, 12)), 1)
	strikes = times(entries, "(#1)", "Casting {SpellID: 49930, Tag: 1}")
	if len(strikes) != 2 {
		t.Fatalf("two Blood Strikes: got %v", strikes)
	}
	got := reverts(entries)
	if len(got) != 2 || !near(got[0], strikes[1]+30*time.Second) || got[1] != got[0] {
		t.Errorf("Blood Strikes at %v: death runes reverted at %v, want both 30 s after the second", strikes, got)
	}
}

// Player::GetRuneBaseCooldown takes the grace off first, then Improved Unholy Presence's 10% off what's
// left, in whole ms.
func TestImprovedUnholyPresenceRuneCooldown(t *testing.T) {
	// Unholy Presence before the pull, then Icy Touch on each frost rune and again once the first is back
	apl := strings.Replace(castsIn(`{"spellId": 49909}`, 1, 3, 12), `"priorityList": [`,
		`"prepullActions": [{"action": {"castSpell": {"spellId": {"spellId": 48265}}}, "doAtValue": {"const": {"val": "-10s"}}}], "priorityList": [`, 1)
	entries := runParity(t, parityPlayer(unholyWith(22, '2'), "p3_uh_dw", apl), 1)
	touches := times(entries, "(#1)", "Casting {SpellID: 49909")
	back := times(entries, "(#1)", "Gained 1.000 frost rune from {OtherID:")
	if len(touches) != 3 || len(back) < 3 {
		t.Fatalf("Icy Touches at %v, frost runes back at %v", touches, back)
	}
	// the first two runes had no grace: 9 s
	for i := range 2 {
		if d := back[i] - touches[i]; d < 9*time.Second-10*time.Millisecond || d > 9*time.Second+10*time.Millisecond {
			t.Errorf("rune spent at %v back after %v, want 9s", touches[i], d)
		}
	}
	grace := min(2500*time.Millisecond, touches[2]-back[0]) - 250*time.Millisecond
	want := (10*time.Second - grace) * 9 / 10
	if d := back[2] - touches[2]; d < want-20*time.Millisecond || d > want+20*time.Millisecond {
		t.Errorf("rune spent %v after it came back: back after %v, want %v", touches[2]-back[0], d, want)
	}
}

// Spell::TakePower: a Death Coil that misses takes irand(0, cost / 4) of its 40 runic power, a hit all 40.
func TestDeathCoilMissCost(t *testing.T) {
	apl := `{"type": "TypeAPL", "priorityList": [` +
		`{"action": {"castSpell": {"spellId": {"spellId": 49895}}}}, ` +
		`{"action": {"castSpell": {"spellId": {"spellId": 49930, "tag": 1}}}}, ` +
		`{"action": {"castSpell": {"spellId": {"spellId": 49909}}}}, ` +
		`{"action": {"castSpell": {"spellId": {"spellId": 49921, "tag": 1}}}}]}`
	misses, hits := 0, 0
	for seed := int64(1); seed <= 3; seed++ {
		// no Virulence, so it misses now and then
		entries := runParity(t, parityPlayer(FrostTalents, "p3_frost", apl), seed)
		missed := map[time.Duration]bool{}
		for _, e := range entries {
			if strings.HasSuffix(e.unit, "(#1)") && strings.Contains(e.text, "{SpellID: 49895} Miss") {
				missed[e.at] = true
			}
		}
		for _, e := range entries {
			rest, ok := strings.CutPrefix(e.text, "Spent ")
			if !strings.HasSuffix(e.unit, "(#1)") || !ok || !strings.Contains(rest, "runic power from {SpellID: 49895}") {
				continue
			}
			spent, _ := strconv.ParseFloat(strings.Fields(rest)[0], 64)
			if missed[e.at] {
				misses++
				if spent > 10 {
					t.Errorf("seed %d: a miss at %v took %v runic power", seed, e.at, spent)
				}
			} else {
				hits++
				if spent != 40 {
					t.Errorf("seed %d: a hit at %v took %v runic power", seed, e.at, spent)
				}
			}
		}
	}
	if misses == 0 || hits == 0 {
		t.Errorf("%d misses and %d hits paid, want some of each", misses, hits)
	}
}

// Unit::MeleeDamageBonusDone hands the rune weapon's hits to the DK: Elixir of Demonslaying's 105 attack power
// against the demon dummy adds int32(105 / 14 x speed) off the DK's own main hand, normalized for Heart Strike.
func TestRuneWeaponCreatureTypeAttackPower(t *testing.T) {
	apl := `{"type": "TypeAPL", "priorityList": [` +
		`{"action": {"castSpell": {"spellId": {"spellId": 49028}}}}, ` +
		`{"action": {"castSpell": {"spellId": {"spellId": 55262, "tag": 1}}}}]}`
	baseDamage := func(elixir bool) (swing, heartStrike float64, speed float64) {
		player := parityPlayer(BloodTalents, "p3_blood", apl)
		if elixir {
			player.Consumes = &proto.Consumes{BattleElixir: proto.BattleElixir_ElixirOfDemonslaying}
		}
		speed = parityEnv(t, player, nil).AutoAttacks.MH().SwingSpeed
		for _, e := range runParity(t, player, 1) {
			if !strings.Contains(e.unit, "Rune Weapon") || !strings.Contains(e.text, "[DEBUG]") {
				continue
			}
			_, rest, _ := strings.Cut(e.text, "BaseDamage:")
			value, _ := strconv.ParseFloat(strings.TrimSuffix(strings.Fields(rest)[0], ","), 64)
			switch {
			case swing == 0 && strings.Contains(e.text, "{OtherID: 3, Tag: 1}"):
				swing = value
			case heartStrike == 0 && strings.Contains(e.text, "{SpellID: 55262, Tag: 1}"):
				heartStrike = value
			}
		}
		return swing, heartStrike, speed
	}
	swing, heartStrike, speed := baseDamage(false)
	swingElixir, heartStrikeElixir, _ := baseDamage(true)
	if swing == 0 || heartStrike == 0 {
		t.Fatalf("no rune weapon swing (%v) or Heart Strike (%v)", swing, heartStrike)
	}
	if got, want := swingElixir-swing, math.Trunc(float64(float32(105)/14*float32(speed))); math.Abs(got-want) > 0.1 {
		t.Errorf("swing base damage +%.1f, want +%v (speed %v)", got, want, speed)
	}
	if got, want := heartStrikeElixir-heartStrike, math.Trunc(float64(float32(105)/14*3.3)); math.Abs(got-want) > 0.1 {
		t.Errorf("Heart Strike base damage +%.1f, want +%v", got, want)
	}
}
