package dps

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
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

// Dirge's class mask covers Obliterate, like Chill of the Grave's: 5 runic power each on top of its 15.
func TestDirgeOnObliterate(t *testing.T) {
	obliterates := castsIn(`{"spellId": 51425, "tag": 1}`, 1, 12, 23, 34)
	gains := func(talents string, amount string) int {
		entries := runParity(t, parityPlayer(talents, "p3_frost", obliterates), 1)
		return len(times(entries, "(#1)", "Gained "+amount+" runic power from {SpellID: 51425, Tag: 1}"))
	}
	// the frost build has Chill of the Grave; Dirge is the unholy tree's 16th talent
	if gains(FrostTalents, "20.000") == 0 {
		t.Error("without Dirge: no Obliterate gave 20 runic power")
	}
	withDirge := talentsWith(FrostTalents, 2, 15, '2')
	if gains(withDirge, "25.000") == 0 || gains(withDirge, "20.000") != 0 {
		t.Error("with Dirge: Obliterate should give 25 runic power every time")
	}
}

// runeWeaponHit is a hit's damage debug line: its base damage and its damage after the attacker's multipliers.
type runeWeaponHit struct{ base, done float64 }

// runeWeaponSwingRoll is the DK's main hand swing the rune weapon's white hit copies, rolled again on him.
var runeWeaponSwingRoll = core.ActionID{OtherID: proto.OtherAction_OtherActionAttack, Tag: 3}

// firstRuneWeaponHits reads the roll of the rune weapon's first white hit and its first Heart Strike.
func firstRuneWeaponHits(entries []logEntry) (swing, heartStrike runeWeaponHit) {
	field := func(text, name string) float64 {
		_, rest, _ := strings.Cut(text, name+":")
		value, _ := strconv.ParseFloat(strings.TrimSuffix(strings.Fields(rest)[0], ","), 64)
		return value
	}
	for _, e := range entries {
		if !strings.Contains(e.text, "[DEBUG]") {
			continue
		}
		hit := runeWeaponHit{field(e.text, "BaseDamage"), field(e.text, "AfterAttackerMods")}
		switch {
		case swing.base == 0 && strings.Contains(e.text, runeWeaponSwingRoll.String()):
			swing = hit
		case heartStrike.base == 0 && strings.Contains(e.unit, "Rune Weapon") && strings.Contains(e.text, "{SpellID: 55262, Tag: 1}"):
			heartStrike = hit
		}
	}
	return swing, heartStrike
}

const runeWeaponAPL = `{"type": "TypeAPL", "priorityList": [` +
	`{"action": {"castSpell": {"spellId": {"spellId": 49028}}}}, ` +
	`{"action": {"castSpell": {"spellId": {"spellId": 55262, "tag": 1}}}}]}`

// Unit::MeleeDamageBonusDone hands the rune weapon's hits to the DK: Elixir of Demonslaying's 105 attack power
// against the demon dummy adds int32(105 / 14 x speed) off the DK's own main hand, normalized for Heart Strike.
// It's the DK's flat bonus on a physical hit, so the DK's aura 79 percents skip it: it takes the halving and
// the spell's own mods only. A white hit is the DK's own swing rolled again, so it gets the bonus like his,
// and the halving comes after.
func TestRuneWeaponCreatureTypeAttackPower(t *testing.T) {
	player := parityPlayer(BloodTalents, "p3_blood", runeWeaponAPL)
	dk := parityEnv(t, player, nil)
	speed := dk.AutoAttacks.MH().SwingSpeed
	spellMods := func(spell *core.Spell) float64 { return spell.DamageMultiplier * spell.DamageMultiplierAdditive }
	swingMods, heartStrikeMods := spellMods(dk.GetSpell(runeWeaponSwingRoll)), spellMods(dk.RuneWeapon.HeartStrike)

	swing, heartStrike := firstRuneWeaponHits(runParity(t, player, 1))
	player.Consumes = &proto.Consumes{BattleElixir: proto.BattleElixir_ElixirOfDemonslaying}
	swingElixir, heartStrikeElixir := firstRuneWeaponHits(runParity(t, player, 1))
	if swing.base == 0 || heartStrike.base == 0 {
		t.Fatalf("no rune weapon swing (%v) or Heart Strike (%v)", swing, heartStrike)
	}
	// each done damage is whole, so the difference is within a point of the bonus
	if got, want := swingElixir.done-swing.done, math.Trunc(float64(float32(105)/14*float32(speed)))*swingMods; math.Abs(got-want) >= 1 {
		t.Errorf("swing damage +%v, want +%.2f (speed %v)", got, want, speed)
	}
	if got, want := heartStrikeElixir.done-heartStrike.done, math.Trunc(float64(float32(105)/14*3.3))*0.5*heartStrikeMods; math.Abs(got-want) >= 1 {
		t.Errorf("Heart Strike damage +%v, want +%.2f", got, want)
	}
}

// A troll's Beast Slaying (DONE_VERSUS) is in the DK's bonus done, so it counts on the rune weapon's hits too.
func TestRuneWeaponBeastSlaying(t *testing.T) {
	player := parityPlayer(BloodTalents, "p3_blood", runeWeaponAPL)
	player.Race = proto.Race_RaceTroll
	swing, heartStrike := firstRuneWeaponHits(runParityVs(t, player, proto.MobType_MobTypeDemon, 1))
	swingBeast, heartStrikeBeast := firstRuneWeaponHits(runParityVs(t, player, proto.MobType_MobTypeBeast, 1))
	for _, c := range []struct {
		name         string
		demon, beast runeWeaponHit
	}{
		{"swing", swing, swingBeast},
		{"Heart Strike", heartStrike, heartStrikeBeast},
	} {
		if c.demon.base == 0 || c.beast.base != c.demon.base {
			t.Fatalf("%s: base damage %v against the demon, %v against the beast", c.name, c.demon.base, c.beast.base)
		}
		if want := c.demon.done * 1.05; math.Abs(c.beast.done-want) > 1.05 {
			t.Errorf("%s: %v against the beast, want 5%% over the demon's %v", c.name, c.beast.done, c.demon.done)
		}
	}
}
