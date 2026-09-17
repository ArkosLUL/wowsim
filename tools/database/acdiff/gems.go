package main

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

type gemOrEnchantDiff struct {
	kind  string
	id    int32
	name  string
	issue string
	diffs []fieldDiff
	extra string
}

// collapseRatings evens out hand-authored stat vectors: sim gems and enchants sometimes list a
// generic rating only under melee or only under spell, and attack power without ranged.
func collapseRatings(stats []float64) []float64 {
	var out database.Stats
	copy(out[:], stats)
	pairs := [][2]proto.Stat{
		{proto.Stat_StatMeleeHit, proto.Stat_StatSpellHit},
		{proto.Stat_StatMeleeCrit, proto.Stat_StatSpellCrit},
		{proto.Stat_StatMeleeHaste, proto.Stat_StatSpellHaste},
		{proto.Stat_StatAttackPower, proto.Stat_StatRangedAttackPower},
	}
	for _, pair := range pairs {
		v := max(out[pair[0]], out[pair[1]])
		out[pair[0]], out[pair[1]] = v, v
	}
	return out[:]
}

func (c *context) diffGems() (diffs []gemOrEnchantDiff, effects []effectRow) {
	for _, id := range sortedKeys(c.sim.Gems) {
		simGem := c.sim.Gems[id]
		row := c.items[id]
		if row == nil {
			diffs = append(diffs, gemOrEnchantDiff{kind: "gem", id: id, name: simGem.Name, issue: "missing on server"})
			continue
		}
		serverGem, spells := azerothcore.ConvertGem(row, c.dbc)
		if serverGem == nil {
			diffs = append(diffs, gemOrEnchantDiff{kind: "gem", id: id, name: simGem.Name, issue: "not a gem on server"})
			continue
		}

		var fields []fieldDiff
		if serverGem.Color != simGem.Color {
			fields = append(fields, fieldDiff{"color", serverGem.Color.String(), simGem.Color.String()})
		}
		fields = append(fields, diffStats("stat ", collapseRatings(serverGem.Stats), collapseRatings(simGem.Stats))...)
		if len(fields) > 0 {
			diffs = append(diffs, gemOrEnchantDiff{kind: "gem", id: id, name: simGem.Name, issue: "differs", diffs: fields})
		}

		for _, spell := range spells {
			effect := effectRow{kind: "gem", id: id, name: simGem.Name, context: "gem spell"}
			c.compareSpell(&effect, spell.SpellID, c.enchantCastBy(spell))
			c.compareEffectGo(&effect, id, core.HasItemEffect(id))
			effect.moduleFiles = appendUnique(effect.moduleFiles, c.modules.items[id]...)
			effect.moduleFiles = appendUnique(effect.moduleFiles, c.modules.enchants[spell.EnchantID]...)
			effects = append(effects, effect)
		}
	}
	return diffs, effects
}

func (c *context) diffEnchants() (diffs []gemOrEnchantDiff, effects []effectRow) {
	seenEffects := map[int32]bool{}
	for _, key := range sortedEnchantKeys(c.sim.Enchants) {
		simEnchant := c.sim.Enchants[key]
		id := simEnchant.EffectId
		serverEnchant := c.dbc.Enchantments[id]
		if serverEnchant == nil {
			diffs = append(diffs, gemOrEnchantDiff{kind: "enchant", id: id, name: simEnchant.Name, issue: "missing on server"})
			continue
		}

		stats, spells, weaponDamage := azerothcore.EnchantmentStats(serverEnchant, c.dbc)
		simStats := simEnchant.Stats
		if len(simStats) > len(stats) {
			simStats = simStats[:len(stats)]
		}
		fields := diffStats("stat ", collapseRatings(stats[:]), collapseRatings(simStats))
		if len(fields) > 0 {
			extra := ""
			if weaponDamage != 0 {
				extra = fmt.Sprintf("server weapon damage +%d", weaponDamage)
			}
			diffs = append(diffs, gemOrEnchantDiff{kind: "enchant", id: id, name: simEnchant.Name, issue: "differs", diffs: fields, extra: extra})
		}

		if seenEffects[id] {
			continue
		}
		seenEffects[id] = true
		for _, spell := range spells {
			effect := effectRow{kind: "enchant", id: id, name: simEnchant.Name, context: "enchant spell"}
			c.compareSpell(&effect, spell.SpellID, c.enchantCastBy(spell))
			c.compareEffectGo(&effect, id, core.HasEnchantEffect(id) || core.HasWeaponEffect(id))
			effect.moduleFiles = appendUnique(effect.moduleFiles, c.modules.enchants[id]...)
			effects = append(effects, effect)
		}
	}
	return diffs, effects
}
