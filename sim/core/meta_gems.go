package core

import (
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
