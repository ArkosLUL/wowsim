package core

import (
	"math"
	"math/rand/v2"
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
	c, byStacks := dp.takenCoefficient(sd, true)
	if !byStacks {
		t.Fatal("Deadly Poison's coefficient doesn't take its stacks")
	}
	if got := c * 5 * 5; math.Abs(float64(got)-4.75) > 1e-5 {
		t.Errorf("5 stacks: %v, want 4.75 (38 a tick from Gift of Arthas)", got)
	}
}

// flatDamageTaken and takenCoefficient before the cache
func refFlatDamageTaken(spell *Spell, amount float64, target *Unit, isPeriodic bool) float64 {
	if weapon, _ := spell.resolveWeaponDamage(); weapon && !isPeriodic {
		return amount
	}
	sd := spell.ServerSpell()
	if sd == nil {
		return 0
	}
	stacks := float32(1)
	if isPeriodic && int(target.UnitIndex) < len(spell.dots) {
		if dot := spell.dots.Get(target); dot != nil && dot.GetStacks() > 1 {
			stacks = float32(dot.GetStacks())
		}
	}
	return math.Trunc(float64(float32(amount) * refTakenCoefficient(spell, sd, isPeriodic, stacks) * stacks))
}

func refTakenCoefficient(spell *Spell, sd *serverdata.Spell, isPeriodic bool, stacks float32) float32 {
	if b := serverdata.BonusBySpellID(sd.ID); b != nil {
		c := b.Direct
		if isPeriodic {
			c = b.Dot
		}
		if c > 0 {
			return c
		}
	}
	return spell.defaultCoefficient(sd, isPeriodic) * stacks
}

// The cached server data reads against the uncached ones, while every input they key on changes under them.
func TestServerDerivedWalk(t *testing.T) {
	// weapon, normalized, bonus rows, default coefficients (instant, area, periodic, stacking, channeled),
	// direct plus dot, and ticks dealt as casts
	ids := []int32{47450, 47486, 42833, 57965, 47502, 48676, 57970, 48156, 48463, 47811, 49233, 47813, 42940,
		58434, 60053, 49938}
	var data []*serverdata.Spell
	for _, id := range ids {
		sd := serverdata.SpellByID(id)
		if sd == nil {
			t.Fatalf("no server data for %d", id)
		}
		data = append(data, sd)
	}
	kinds := []struct {
		typ UnitType
		pet bool
	}{{PlayerUnit, false}, {PetUnit, false}, {PetUnit, true}}
	units := []*Unit{{Type: PlayerUnit}, {Type: PetUnit}, {Type: PetUnit, SummonedAsPet: true}}
	targets := []*Unit{{Type: EnemyUnit, UnitIndex: 0}, {Type: EnemyUnit, UnitIndex: 1}, {Type: EnemyUnit, UnitIndex: 2}}
	masks := []ProcMask{ProcMaskEmpty, ProcMaskMeleeMHAuto, ProcMaskMeleeOHAuto, ProcMaskRangedAuto,
		ProcMaskMeleeMHSpecial, ProcMaskRangedSpecial, ProcMaskSpellDamage}
	flags := []SpellFlag{SpellFlagNoServerData, SpellFlagIncludeTargetBonusDamage, SpellFlagBinary, SpellFlagChanneled}
	amounts := []float64{8, -8, 0, 3, 1e6, 0.4}

	var spells []*Spell
	for i, sd := range data {
		spell := &Spell{Unit: units[i%len(units)], ActionID: ActionID{SpellID: sd.ID}, ProcMask: masks[i%len(masks)],
			serverSpell: sd}
		// two targets with a dot, the third past the array
		spell.dots = DotArray{{Aura: &Aura{}}, {}}
		spells = append(spells, spell)
	}
	spells = append(spells, &Spell{Unit: units[0], ProcMask: ProcMaskMeleeMHAuto})

	rng := rand.New(rand.NewPCG(3, 4))
	for step := range 20000 {
		spell := spells[rng.IntN(len(spells))]
		switch op := rng.IntN(16); {
		case op < 2:
			spell.Flags ^= flags[rng.IntN(len(flags))]
		case op < 4:
			spell.ProcMask = masks[rng.IntN(len(masks))]
		case op < 5:
			spell.serverSpell = nil
			if rng.IntN(4) > 0 {
				spell.serverSpell = data[rng.IntN(len(data))]
			}
		case op < 6:
			if rng.IntN(2) == 0 {
				spell.Unit = units[rng.IntN(len(units))]
			} else {
				kind := kinds[rng.IntN(len(kinds))]
				spell.Unit.Type, spell.Unit.SummonedAsPet = kind.typ, kind.pet
			}
		case op < 8:
			if len(spell.dots) > 0 {
				spell.dots[0].Aura.stacks = rng.Int32N(6)
				if rng.IntN(5) == 0 {
					spell.dots[1] = nil
				} else if spell.dots[1] == nil || rng.IntN(3) == 0 {
					spell.dots[1] = &Dot{Aura: &Aura{stacks: rng.Int32N(6)}}
				}
			}
		default:
			target := targets[rng.IntN(len(targets))]
			amount := amounts[rng.IntN(len(amounts))]
			isPeriodic := rng.IntN(2) == 0
			got, want := spell.flatDamageTaken(amount, target, isPeriodic), refFlatDamageTaken(spell, amount, target, isPeriodic)
			if math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("step %d: %d's taken %v (periodic %v), want %v", step, spell.ActionID.SpellID, got, isPeriodic, want)
			}
			gw, gn := spell.dealsWeaponDamage()
			ww, wn := spell.resolveWeaponDamage()
			if gw != ww || gn != wn {
				t.Fatalf("step %d: %d's weapon damage %v %v, want %v %v", step, spell.ActionID.SpellID, gw, gn, ww, wn)
			}
			if got, want := spell.ticksAsCasts(), ticksAsCastsOn(spell.ServerSpell()); got != want {
				t.Fatalf("step %d: %d's ticks as casts %v, want %v", step, spell.ActionID.SpellID, got, want)
			}
		}
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
