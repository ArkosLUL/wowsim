package core

import (
	"math"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

func serverDataSpell(t *testing.T, unit *Unit, id int32, procMask ProcMask) *Spell {
	t.Helper()
	sd := serverdata.SpellByID(id)
	if sd == nil {
		t.Fatalf("no server data for %d", id)
	}
	return &Spell{Unit: unit, ActionID: ActionID{SpellID: id}, ProcMask: procMask, serverSpell: sd}
}

// Gift of Arthas' +8 on the EC-BUFFS probes: a weapon hit takes all of it, a spell its taken coefficient
func TestGiftOfArthasTaken(t *testing.T) {
	player := &Unit{Type: PlayerUnit}
	target := &Unit{Type: EnemyUnit}
	for _, tc := range []struct {
		name     string
		spell    *Spell
		periodic bool
		want     float64
	}{
		{"white swing", &Spell{Unit: player, ProcMask: ProcMaskMeleeMHAuto}, false, 8},
		{"Mortal Strike", serverDataSpell(t, player, 47486, ProcMaskMeleeMHSpecial), false, 8},
		// no bonus row: 1500 ms of 3500
		{"Instant Poison", serverDataSpell(t, player, 57965, ProcMaskEmpty), false, 3},
		// 18 s / 15 s over 6 ticks: 0.2
		{"Garrote tick", serverDataSpell(t, player, 48676, ProcMaskEmpty), true, 1},
		// a bonus row: Fireball's 1.0
		{"Fireball", serverDataSpell(t, player, 42833, ProcMaskSpellDamage), false, 8},
		// halved for its area: 1500 ms / 2 of 3500
		{"Thunder Clap", serverDataSpell(t, player, 47502, ProcMaskMeleeMHSpecial), false, 1},
	} {
		if got := tc.spell.flatDamageTaken(8, target, tc.periodic); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Deadly Poison's default coefficient takes its stacks, and Unit::SpellDamageBonusTaken does again:
// 0.2 a tick, less 5% for its crit aura, at 5 stacks twice
func TestTakenCoefficientStacks(t *testing.T) {
	dp := serverDataSpell(t, &Unit{Type: PlayerUnit}, 57970, ProcMaskEmpty)
	sd := dp.ServerSpell()
	if got := dp.takenCoefficient(sd, true, 5) * 5; math.Abs(float64(got)-4.75) > 1e-5 {
		t.Errorf("5 stacks: %v, want 4.75 (38 a tick from Gift of Arthas)", got)
	}
}

func TestVersusAttackPower(t *testing.T) {
	unit := &Unit{Type: PlayerUnit}
	unit.PseudoStats = stats.NewPseudoStats()
	unit.PseudoStats.MobTypeAttackPower = 105
	unit.AutoAttacks.mh.Weapon = Weapon{SwingSpeed: 3.4, NormalizedSwingSpeed: 3.3}
	for _, tc := range []struct {
		name  string
		spell *Spell
		want  float64
	}{
		// the mobs probe's Elixir of Demonslaying: int(105 / 14 * 3.4)
		{"white swing", &Spell{Unit: unit, ProcMask: ProcMaskMeleeMHAuto}, 25},
		{"Heroic Strike", serverDataSpell(t, unit, 47450, ProcMaskMeleeMHSpecial), 25},
		{"Mortal Strike, normalized", serverDataSpell(t, unit, 47486, ProcMaskMeleeMHSpecial), 24},
		{"Exorcism, a spell", serverDataSpell(t, unit, 48801, ProcMaskSpellDamage), 0},
	} {
		if got := tc.spell.versusAttackPowerBonus(); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

func metaTestEquipment(meta int32, colors ...proto.GemColor) Equipment {
	var eq Equipment
	head := &eq[proto.ItemSlot_ItemSlotHead]
	head.Gems = []Gem{{ID: meta, Color: proto.GemColor_GemColorMeta, Stats: stats.Stats{stats.MeleeCrit: 21}}}
	for i, c := range colors {
		item := &eq[proto.ItemSlot_ItemSlotChest+proto.ItemSlot(i%3)]
		item.Gems = append(item.Gems, Gem{ID: 40000 + int32(i), Color: c})
	}
	return eq
}

func TestMetaGemConditions(t *testing.T) {
	const relentless, chaotic, mystical = 41398, 41285, 25893
	red, yellow, blue := proto.GemColor_GemColorRed, proto.GemColor_GemColorYellow, proto.GemColor_GemColorBlue
	for _, tc := range []struct {
		name   string
		eq     Equipment
		active bool
	}{
		{"Relentless with one of each", metaTestEquipment(relentless, red, yellow, blue), true},
		{"Relentless without blue", metaTestEquipment(relentless, red, yellow, yellow), false},
		{"Relentless on a prismatic gem", metaTestEquipment(relentless, proto.GemColor_GemColorPrismatic), true},
		{"Chaotic on purple and green", metaTestEquipment(chaotic, proto.GemColor_GemColorPurple, proto.GemColor_GemColorGreen), true},
		{"Chaotic on one blue", metaTestEquipment(chaotic, blue, red), false},
		{"Mystical: more blue than yellow", metaTestEquipment(mystical, blue, blue, yellow), true},
		{"Mystical: as many", metaTestEquipment(mystical, blue, yellow), false},
	} {
		inactive := tc.eq.inactiveMetaGem() != nil
		if inactive == tc.active {
			t.Errorf("%s: active %v, want %v", tc.name, !inactive, tc.active)
		}
		crit := tc.eq.Stats()[stats.MeleeCrit]
		if want := map[bool]float64{true: 21, false: 0}[tc.active]; crit != want {
			t.Errorf("%s: the meta's crit %v, want %v", tc.name, crit, want)
		}
	}
}
