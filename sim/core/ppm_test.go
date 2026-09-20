package core

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/serverdata"
)

func TestServerProcFor(t *testing.T) {
	cases := []struct {
		name    string
		spellID int32
		want    ServerProc
	}{
		// spell_proc: PPM 1, 2 s ICD, REDUCE_PROC_60
		{"Hand of Justice", 15600, ServerProc{Chance: 0.02 / 3, PPM: 1.0 / 3, ICD: 2 * time.Second}},
		{"Black Magic", 59630, ServerProc{Chance: 0.35, ICD: 35 * time.Second}},
		{"Thundering Skyfire Diamond", 39958, ServerProc{PPM: 0.7, ICD: 40 * time.Second}},
		{"Judgement of Wisdom", judgementOfWisdomAuraID, ServerProc{PPM: 15}},
		// no spell_proc row: Spell.dbc's 3% and no ICD
		{"Ashen Band of Courage", 72415, ServerProc{Chance: 0.03}},
	}
	for _, c := range cases {
		got := ServerProcFor(c.spellID)
		if math.Abs(got.Chance-c.want.Chance) > 1e-9 || math.Abs(got.PPM-c.want.PPM) > 1e-9 || got.ICD != c.want.ICD {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestServerEnchantPPM(t *testing.T) {
	for enchantID, want := range map[int32]float64{2673: 1, 3239: 3, 3273: 3, 3789: 1} {
		if got := ServerEnchantPPM(enchantID); got != want {
			t.Errorf("enchant %d: got PPM %v, want %v", enchantID, got, want)
		}
	}
}

func TestServerDuration(t *testing.T) {
	// Protection of Ancient Kings
	if got := ServerDuration(64413); got != 8*time.Second {
		t.Errorf("Protection of Ancient Kings: got %v, want 8s", got)
	}
}

// A dual wielder with a bow, so each hand's PPM basis is distinguishable.
func newPPMUnit() *Unit {
	unit := &Unit{Type: PlayerUnit, Level: 80, PseudoStats: newPseudoStats()}
	unit.AutoAttacks = AutoAttacks{
		AutoSwingMelee:  true,
		AutoSwingRanged: true,
		IsDualWielding:  true,
		mh:              WeaponAttack{Weapon: Weapon{SwingSpeed: 2.6}},
		oh:              WeaponAttack{Weapon: Weapon{SwingSpeed: 1.4}},
		ranged:          WeaponAttack{Weapon: Weapon{SwingSpeed: 3.0}},
	}
	return unit
}

func ppmTestSpell(procMask ProcMask, serverSpellID int32) *Spell {
	spell := &Spell{ActionID: ActionID{SpellID: serverSpellID}, ProcMask: procMask}
	if serverSpellID != 0 {
		spell.serverSpell = serverdata.SpellByID(serverSpellID)
		if spell.serverSpell == nil {
			panic("spell not in the generated tables")
		}
	}
	return spell
}

// Aura::CalcProcChance measures a spell proc against max(base cast, 1500 ms) and a weapon proc against
// the unhasted attack time.
func TestAuraPPMProcChance(t *testing.T) {
	unit := newPPMUnit()

	cases := []struct {
		name  string
		spell *Spell
		want  float64
	}{
		{"instant spell", ppmTestSpell(ProcMaskSpellDamage, 0), 0.375},
		{"Mind Blast, a 1.5 s cast", ppmTestSpell(ProcMaskSpellDamage, 48127), 0.375},
		{"Frostbolt, a 3 s cast", ppmTestSpell(ProcMaskSpellDamage, 42842), 0.75},
		{"main hand swing", ppmTestSpell(ProcMaskMeleeMHAuto, 0), 0.65},
		{"off hand swing", ppmTestSpell(ProcMaskMeleeOHAuto, 0), 0.35},
		{"ranged auto", ppmTestSpell(ProcMaskRangedAuto, 0), 0.75},
		{"Heroic Strike, a melee-class spell", ppmTestSpell(ProcMaskMeleeMHSpecial, 47450), 0.65},
		{"Steady Shot, a ranged weapon spell", ppmTestSpell(ProcMaskRangedSpecial, 49052), 0.75},
	}

	for _, c := range cases {
		if got := unit.AuraPPMProcChance(15, c.spell); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// Without server data the proc mask decides, so a melee ability still reads the weapon.
func TestAuraPPMProcChanceWithoutServerData(t *testing.T) {
	unit := newPPMUnit()

	if got := unit.AuraPPMProcChance(15, ppmTestSpell(ProcMaskMeleeOHSpecial, 0)); math.Abs(got-0.35) > 1e-9 {
		t.Errorf("off hand ability: got %v, want 0.35", got)
	}

	spell := ppmTestSpell(ProcMaskSpellDamage, 0)
	spell.DefaultCast.CastTime = 2 * time.Second
	if got := unit.AuraPPMProcChance(15, spell); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("2 s cast: got %v, want 0.5", got)
	}
}
