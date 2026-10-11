package core

import (
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// A meta gem's SpellItemEnchantmentCondition clause, colors and comparators as the DBC numbers them.
type metaGemClause struct {
	color        uint8 // 2 red, 3 yellow, 4 blue
	comparator   uint8 // metaLess, metaMore or metaAtLeast
	compareColor uint8 // compare with that color's count instead of value
	value        uint8
}

const (
	metaRed    = 2
	metaYellow = 3
	metaBlue   = 4

	metaLess    = 2
	metaMore    = 3
	metaAtLeast = 5
)

// metaGemConditions are the meta gems' conditions, through each gem's SpellItemEnchantment row
// (EnchantmentCondition, field 34) to SpellItemEnchantmentCondition.dbc, read from the live DBCs.
var metaGemConditions = map[int32][]metaGemClause{
	25890: {{metaRed, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 2}}, // Destructive Skyfire
	25893: {{metaBlue, metaMore, metaYellow, 0}},                                                          // Mystical Skyfire
	25894: {{metaYellow, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                // Swift Skyfire
	25895: {{metaRed, metaMore, metaYellow, 0}},                                                           // Enigmatic Skyfire
	25896: {{metaBlue, metaAtLeast, 0, 3}},                                                                // Powerful Earthstorm
	25897: {{metaRed, metaMore, metaBlue, 0}},                                                             // Bracing Earthstorm
	25898: {{metaBlue, metaAtLeast, 0, 5}},                                                                // Tenacious Earthstorm
	25899: {{metaRed, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 2}}, // Brutal Earthstorm
	25901: {{metaRed, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 2}}, // Insightful Earthstorm
	28556: {{metaYellow, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                // Swift Windfire
	28557: {{metaYellow, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                // Swift Starfire
	32409: {{metaRed, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 2}}, // Relentless Earthstorm
	32410: {{metaRed, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 2}}, // Thundering Skyfire
	32640: {{metaBlue, metaMore, metaYellow, 0}},                                                          // Potent Unstable
	32641: {{metaYellow, metaAtLeast, 0, 3}},                                                              // Imbued Unstable
	34220: {{metaBlue, metaAtLeast, 0, 2}},                                                                // Chaotic Skyfire
	35501: {{metaBlue, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 1}},                               // Eternal Earthstorm
	35503: {{metaRed, metaAtLeast, 0, 3}},                                                                 // Ember Skyfire
	41285: {{metaBlue, metaAtLeast, 0, 2}},                                                                // Chaotic Skyflare
	41307: {{metaRed, metaAtLeast, 0, 1}, {metaYellow, metaAtLeast, 0, 1}, {metaBlue, metaAtLeast, 0, 1}}, // Destructive Skyflare
	41333: {{metaRed, metaAtLeast, 0, 3}},                                                                 // Ember Skyflare
	41335: {{metaRed, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 1}},                                // Enigmatic Skyflare
	41339: {{metaYellow, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                // Swift Skyflare
	41375: {{metaRed, metaAtLeast, 0, 1}, {metaBlue, metaAtLeast, 0, 1}, {metaYellow, metaAtLeast, 0, 1}}, // Tireless Skyflare
	41376: {{metaRed, metaAtLeast, 0, 2}},                                                                 // Revitalizing Skyflare
	41377: {{metaBlue, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                  // Effulgent Skyflare
	41378: {{metaYellow, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 1}},                               // Forlorn Skyflare
	41379: {{metaRed, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 1}},                                  // Impassive Skyflare
	41380: {{metaBlue, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                  // Austere Earthsiege
	41381: {{metaYellow, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 1}},                               // Persistent Earthsiege
	41382: {{metaRed, metaAtLeast, 0, 1}, {metaYellow, metaAtLeast, 0, 1}, {metaBlue, metaAtLeast, 0, 1}}, // Trenchant Earthsiege
	41385: {{metaBlue, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                  // Invigorating Earthsiege
	41389: {{metaRed, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 1}},                                // Beaming Earthsiege
	41395: {{metaRed, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 1}},                                  // Bracing Earthsiege
	41396: {{metaRed, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 1}},                                  // Eternal Earthsiege
	41397: {{metaBlue, metaAtLeast, 0, 3}},                                                                // Powerful Earthsiege
	41398: {{metaRed, metaAtLeast, 0, 1}, {metaYellow, metaAtLeast, 0, 1}, {metaBlue, metaAtLeast, 0, 1}}, // Relentless Earthsiege
	41400: {{metaRed, metaAtLeast, 0, 1}, {metaYellow, metaAtLeast, 0, 1}, {metaBlue, metaAtLeast, 0, 1}}, // Thundering Skyflare
	41401: {{metaRed, metaAtLeast, 0, 1}, {metaYellow, metaAtLeast, 0, 1}, {metaBlue, metaAtLeast, 0, 1}}, // Insightful Earthsiege
	44076: {{metaYellow, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                // Swift Starflare
	44078: {{metaRed, metaAtLeast, 0, 1}, {metaBlue, metaAtLeast, 0, 1}, {metaYellow, metaAtLeast, 0, 1}}, // Tireless Starflare
	44081: {{metaRed, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 1}},                                  // Enigmatic Starflare
	44082: {{metaBlue, metaAtLeast, 0, 2}, {metaRed, metaAtLeast, 0, 1}},                                  // Impassive Starflare
	44084: {{metaYellow, metaAtLeast, 0, 2}, {metaBlue, metaAtLeast, 0, 1}},                               // Forlorn Starflare
	44087: {{metaBlue, metaAtLeast, 0, 3}},                                                                // Persistent Earthshatter
	44088: {{metaBlue, metaAtLeast, 0, 2}, {metaYellow, metaAtLeast, 0, 1}},                               // Powerful Earthshatter
	44089: {{metaRed, metaAtLeast, 0, 1}, {metaYellow, metaAtLeast, 0, 1}, {metaBlue, metaAtLeast, 0, 1}}, // Trenchant Earthshatter
}

// The meta gems whose enchant is SPELL_AURA_MOD_CRIT_DAMAGE_BONUS (163), 3% on every school, and
// Revitalizing Skyflare, whose 3% is on healing.
var (
	critDamageMetaGems = []int32{34220, 32409, 41285, 41398}
	healingCritMetaGem = []int32{41376}
)

// metaGemSwap is the head's meta gem on a character whose item swap meets or breaks the gem's condition.
// The server checks it on every equip and puts the whole enchant on or takes it off
// (Player::CorrectMetaGemEnchants): the stats, the effect and the crit damage.
type metaGemSwap struct {
	// on while the gear worn meets the condition
	aura *Aura
	// each puts the gem's effect on or takes it off
	toggles []func(sim *Simulation, on bool)

	// the crit multipliers worked out with the gem to the same ones without it, and back
	critWithout, critWith map[float64]float64
	// which of the two the character's spells and weapons hold, and which they were built with
	critOn, critBuiltOn bool
}

// initMetaGemSwap sets the character up for a meta gem that goes on or off with the item swap.
func (character *Character) initMetaGemSwap() {
	swap := &character.ItemSwap
	if len(swap.slots) == 0 {
		return
	}
	meta := character.Equipment.metaGem()
	if meta == nil {
		return
	}
	other := swap.otherSet(&character.Equipment)
	startsOn := character.Equipment.inactiveMetaGem() == nil
	if startsOn == (other.inactiveMetaGem() == nil) {
		return
	}
	character.metaSwap = &metaGemSwap{
		critWithout: map[float64]float64{},
		critWith:    map[float64]float64{},
		critOn:      startsOn,
		critBuiltOn: startsOn,
	}
}

// MetaGemSwaps is whether an item swap turns the head's meta gem on or off during the fight. The gem's
// effect then goes through OnMetaGemSwap instead of applying for good.
func (character *Character) MetaGemSwaps() bool {
	return character.metaSwap != nil
}

// OnMetaGemSwap registers what puts the meta gem's effect on (on a reset the gear starts with its
// condition met, and on a swap that meets it) and what takes it off again.
func (character *Character) OnMetaGemSwap(toggle func(sim *Simulation, on bool)) {
	character.metaSwap.toggles = append(character.metaSwap.toggles, toggle)
}

func (character *Character) registerMetaGemSwapAura() {
	ms := character.metaSwap
	ms.aura = character.RegisterAura(Aura{
		Label:    "Meta Gem",
		Duration: NeverExpires,
		OnGain: func(_ *Aura, sim *Simulation) {
			for _, toggle := range ms.toggles {
				toggle(sim, true)
			}
			character.setMetaGemCrit(true)
		},
		OnExpire: func(_ *Aura, sim *Simulation) {
			for _, toggle := range ms.toggles {
				toggle(sim, false)
			}
			character.setMetaGemCrit(false)
		},
	})
}

// updateMetaGemSwap puts the gem on or takes it off after the gear changed.
func (character *Character) updateMetaGemSwap(sim *Simulation) {
	ms := character.metaSwap
	if ms == nil || ms.aura == nil {
		return
	}
	if character.Equipment.inactiveMetaGem() == nil {
		ms.aura.Activate(sim)
	} else {
		ms.aura.Deactivate(sim)
	}
}

// metaGemCrit picks a crit multiplier by whether the gem behind it is on, and notes the pair when an item
// swap can change that. gems are the ones that carry the bonus.
func (character *Character) metaGemCrit(gems []int32, with, without float64) float64 {
	meta := character.Equipment.metaGem()
	if meta == nil || !slices.Contains(gems, meta.ID) {
		return without
	}
	if ms := character.metaSwap; ms != nil {
		ms.noteCrit(ms.critWithout, with, without)
		ms.noteCrit(ms.critWith, without, with)
	}
	if character.Equipment.inactiveMetaGem() != nil {
		return without
	}
	return with
}

func (ms *metaGemSwap) noteCrit(table map[float64]float64, from, to float64) {
	if old, ok := table[from]; ok && old != to {
		// two crits share this multiplier and part ways without the gem: it can't follow the swap
		to = math.NaN()
	}
	table[from] = to
}

// setMetaGemCrit moves every crit multiplier the character holds to the one with or without the gem.
func (character *Character) setMetaGemCrit(on bool) {
	ms := character.metaSwap
	if ms.critOn == on {
		return
	}
	ms.critOn = on
	table := ms.critWithout
	if on {
		table = ms.critWith
	}
	move := func(multiplier *float64) {
		if to, ok := table[*multiplier]; ok && !math.IsNaN(to) {
			*multiplier = to
		}
	}
	// a heal's multiplier can equal a damage spell's, and each gem's bonus is on only one of the two
	healing := slices.Contains(healingCritMetaGem, character.Equipment.metaGem().ID)
	for _, spell := range character.Spellbook {
		if spell.Flags.Matches(SpellFlagHelpful) == healing {
			move(&spell.CritMultiplier)
		}
	}
	if healing {
		return
	}
	aa := &character.AutoAttacks
	for _, wa := range []*WeaponAttack{&aa.mh, &aa.oh, &aa.ranged} {
		move(&wa.Weapon.CritMultiplier)
	}
	swap := &character.ItemSwap
	move(&swap.mhCritMultiplier)
	move(&swap.ohCritMultiplier)
	move(&swap.rangedCritMultiplier)
}

// resetMetaGemCrit runs ahead of a reset, which puts the spells back at what they were built with.
func (character *Character) resetMetaGemCrit() {
	if ms := character.metaSwap; ms != nil {
		character.setMetaGemCrit(ms.critBuiltOn)
	}
}

// metaGem is the head's meta gem, on or off.
func (equipment *Equipment) metaGem() *Gem {
	head := &equipment[proto.ItemSlot_ItemSlotHead]
	for i := range head.Gems {
		if head.Gems[i].Color == proto.GemColor_GemColorMeta {
			return &head.Gems[i]
		}
	}
	return nil
}

// gemColorCounts counts every socketed gem by GemProperties color, a gem counting once for each color it
// has (Player::EnchantmentFitsRequirements): index 1 red, 2 yellow, 3 blue.
func (equipment *Equipment) gemColorCounts() (counts [4]int) {
	for i := range equipment {
		for _, gem := range equipment[i].Gems {
			if gem.ID == 0 {
				continue
			}
			switch gem.Color {
			case proto.GemColor_GemColorRed:
				counts[1]++
			case proto.GemColor_GemColorYellow:
				counts[2]++
			case proto.GemColor_GemColorBlue:
				counts[3]++
			case proto.GemColor_GemColorOrange:
				counts[1]++
				counts[2]++
			case proto.GemColor_GemColorPurple:
				counts[1]++
				counts[3]++
			case proto.GemColor_GemColorGreen:
				counts[2]++
				counts[3]++
			case proto.GemColor_GemColorPrismatic:
				counts[1]++
				counts[2]++
				counts[3]++
			}
		}
	}
	return counts
}

// inactiveMetaGem is the head's meta gem when the other gems don't meet its condition, which keeps
// the server from applying its enchant at all: no stats and no effect.
func (equipment *Equipment) inactiveMetaGem() *Gem {
	head := &equipment[proto.ItemSlot_ItemSlotHead]
	for i := range head.Gems {
		meta := &head.Gems[i]
		if meta.Color != proto.GemColor_GemColorMeta {
			continue
		}
		clauses, ok := metaGemConditions[meta.ID]
		if !ok {
			return nil
		}
		counts := equipment.gemColorCounts()
		for _, c := range clauses {
			have, want := counts[c.color-1], int(c.value)
			if c.compareColor != 0 {
				want = counts[c.compareColor-1]
			}
			met := true
			switch c.comparator {
			case metaLess:
				met = have < want
			case metaMore:
				met = have > want
			case metaAtLeast:
				met = have >= want
			}
			if !met {
				return meta
			}
		}
		return nil
	}
	return nil
}
