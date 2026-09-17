package main

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/tools/database"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

type valueKind int

const (
	effectAmount valueKind = iota
	durationSeconds
	cooldownSeconds
	procChancePercent
	procsPerMinute
)

type spellValue struct {
	label string
	kind  valueKind
	value float64
	// inTooltip marks values a Classic spell tooltip can show. PPM and anything set by spell_proc,
	// item_template or spell_enchant_proc_data never appear there.
	inTooltip bool
}

type effectRow struct {
	kind    string // item, gem, enchant, set
	id      int32
	name    string
	context string // trigger for items, piece count for sets

	chain             []int32
	serverValues      []spellValue
	serverDescription string
	classicText       string
	classicNumbers    []float64
	notInClassic      []spellValue

	goLocations []goLocation
	goNumbers   []float64
	notInGo     []spellValue
	moduleFiles []string

	classicVerdict string
	goVerdict      string
}

// castBy is what casts the root spell of a chain. Items and enchants can set its cooldown and proc
// rate in their own data, replacing the spell's.
type castBy struct {
	label string // "item 45466", "enchant 3789"
	// cast means an item casts the root (on use, chance on hit, enchant procs), which starts its
	// cooldown. Spell::SendSpellCooldown skips passive auras and spells triggered without an item.
	cast       bool
	cooldownMs int32 // -1 keeps the spell's own
	// procRateSet means procChance and ppm replace the root's DBC chance and spell_proc entry.
	procRateSet bool
	procChance  float64
	ppm         float64
}

// castBySpell is a passive aura, like a set bonus.
var castBySpell = castBy{cooldownMs: -1}

const maxChainDepth = 4

var tooltipProcChance = regexp.MustCompile(`(?i)\$h`)

// spellChainValues collects the numbers a spell and the spells it triggers apply on the server.
func (c *context) spellChainValues(rootID int32, by castBy) (values []spellValue, chain []int32) {
	add := func(label string, kind valueKind, value float64, inTooltip bool) {
		values = append(values, spellValue{label, kind, value, inTooltip})
	}
	visited := map[int32]bool{}
	var walk func(id int32, depth int)
	walk = func(id int32, depth int) {
		spell := c.dbc.Spells[id]
		if spell == nil || visited[id] || depth > maxChainDepth {
			return
		}
		visited[id] = true
		chain = append(chain, id)
		isRoot := depth == 0

		for i := 0; i < 3; i++ {
			if spell.Effect[i] == 0 || isTriggerOnly(spell, i) {
				continue
			}
			value := math.Abs(float64(spell.EffectValue(i)))
			if value > 1 {
				add(fmt.Sprintf("%d effect%d", id, i+1), effectAmount, value, true)
			}
			if spell.EffectDieSides[i] > 1 {
				add(fmt.Sprintf("%d effect%d min", id, i+1), effectAmount, math.Abs(float64(spell.EffectBasePoints[i]+1)), true)
			}
		}
		// Longer durations belong to passive equip auras, not to buffs the sim models.
		if seconds := c.dbc.SpellDurationSeconds(spell); seconds > 1 && seconds < 300 {
			add(fmt.Sprintf("%d duration", id), durationSeconds, seconds, true)
		}

		if isRoot && by.cast {
			ms, label, inTooltip := by.cooldownMs, by.label+" cooldown", false
			if ms < 0 {
				ms, label, inTooltip = spell.CooldownMs(), fmt.Sprintf("%d cooldown", id), true
			}
			if ms > 1000 {
				add(label, cooldownSeconds, float64(ms)/1000, inTooltip)
			}
		}

		if isRoot && by.procRateSet {
			if by.procChance > 1 && by.procChance < 100 {
				add(by.label+" proc chance", procChancePercent, by.procChance, false)
			}
			if by.ppm > 0 {
				add(by.label+" ppm", procsPerMinute, by.ppm, false)
			}
		} else {
			proc, hasProc := c.procs[id]
			if hasProc && proc.Chance > 0 {
				if proc.Chance > 1 && proc.Chance < 100 {
					add(fmt.Sprintf("%d proc chance", id), procChancePercent, proc.Chance, false)
				}
			} else if chance := float64(spell.ProcChance); chance > 1 && chance < 100 {
				add(fmt.Sprintf("%d proc chance", id), procChancePercent, chance, tooltipProcChance.MatchString(spell.Description))
			}
			if hasProc && proc.ProcsPerMinute > 0 {
				add(fmt.Sprintf("%d ppm", id), procsPerMinute, proc.ProcsPerMinute, false)
			}
			if hasProc && proc.CooldownMs > 1000 {
				add(fmt.Sprintf("%d proc cooldown", id), cooldownSeconds, float64(proc.CooldownMs)/1000, false)
			}
		}

		for i := 0; i < 3; i++ {
			if trigger := spell.EffectTriggerSpell[i]; trigger > 0 {
				walk(trigger, depth+1)
			}
		}
	}
	walk(rootID, 0)
	return values, chain
}

// isTriggerOnly reports effects whose base points carry no gameplay value; they only point at
// the spell they trigger, often with a 100 that reads like a chance.
func isTriggerOnly(spell *azerothcore.SpellEntry, i int) bool {
	if spell.Effect[i] == azerothcore.SpellEffectTriggerSpell || spell.EffectTriggerSpell[i] > 0 {
		return true
	}
	aura := spell.EffectApplyAuraName[i]
	return spell.Effect[i] == azerothcore.SpellEffectApplyAura &&
		(aura == azerothcore.AuraProcTriggerSpell || aura == azerothcore.AuraPeriodicTriggerSpell)
}

// goForms lists the ways Go code tends to write a server value: percentages as fractions or
// multipliers, times in minutes or milliseconds. Converted forms that come out as 0 or 1 are left
// out, since nearly every block has those.
func goForms(v spellValue) []float64 {
	var converted []float64
	switch v.kind {
	case effectAmount:
		converted = []float64{v.value / 100, 1 + v.value/100, 1 - v.value/100}
	case procChancePercent:
		converted = []float64{v.value / 100}
	case durationSeconds, cooldownSeconds:
		converted = []float64{v.value * 1000}
		if v.value >= 60 {
			converted = append(converted, v.value/60)
		}
	}
	converted = slices.DeleteFunc(converted, func(f float64) bool { return f == 0 || f == 1 })
	return append([]float64{v.value}, converted...)
}

func goHasValue(numbers []float64, v spellValue) bool {
	return slices.ContainsFunc(goForms(v), func(form float64) bool { return containsNumber(numbers, form) })
}

func classicHasValue(numbers []float64, v spellValue) bool {
	// Tooltips show long durations and cooldowns in minutes, e.g. "1.75 min".
	isTime := v.kind == durationSeconds || v.kind == cooldownSeconds
	return containsNumber(numbers, v.value) || isTime && v.value >= 60 && containsNumber(numbers, v.value/60)
}

// compareSpell collects the server values of a spell chain and compares them with the Classic
// tooltips of the same spell IDs.
func (c *context) compareSpell(row *effectRow, spellID int32, by castBy) {
	row.serverValues, row.chain = c.spellChainValues(spellID, by)
	if spell := c.dbc.Spells[spellID]; spell != nil {
		row.serverDescription = spell.Description
	}
	row.classicText, _ = classicText(c.tooltips, spellID)

	hasTooltip := false
	for _, id := range row.chain {
		if numbers, ok := classicNumbers(c.tooltips, id); ok {
			hasTooltip = true
			row.classicNumbers = append(row.classicNumbers, numbers...)
		}
		row.moduleFiles = appendUnique(row.moduleFiles, c.modules.spells[id]...)
	}
	comparable := 0
	for _, v := range row.serverValues {
		if !v.inTooltip {
			continue
		}
		comparable++
		if !classicHasValue(row.classicNumbers, v) {
			row.notInClassic = append(row.notInClassic, v)
		}
	}

	switch {
	case c.dbc.Spells[spellID] == nil:
		row.classicVerdict = "manual: spell missing on server"
	case len(row.serverValues) == 0:
		row.classicVerdict = "manual: no numeric values (scripted spell)"
	case !hasTooltip:
		row.classicVerdict = "manual: no Classic tooltip"
	case comparable == 0:
		row.classicVerdict = "manual: no values a tooltip shows"
	case len(row.notInClassic) == 0:
		row.classicVerdict = "match"
	default:
		row.classicVerdict = "differs"
	}
}

// compareEffectGo compares an item, gem or enchant effect with the sim's Go code for id.
// registered comes from the sim's effect registry after sim.RegisterAll. It misses effects that
// class code registers per character (death knight items and runes) or checks by ID (some relics),
// so an unregistered ID found in Go is still compared, but flagged: it may be a coincidence.
func (c *context) compareEffectGo(row *effectRow, id int32, registered bool) {
	row.goLocations = c.goIndex.findID(id)
	// Aura spell IDs only narrow down a definition already found by ID; on their own they'd match
	// unrelated uses of the same spell.
	if len(row.goLocations) > 0 {
		for _, spellID := range row.chain {
			row.goLocations = append(row.goLocations, c.goIndex.findLiteral(spellID)...)
		}
	}
	c.collectGoNumbers(row)

	switch {
	case len(row.goLocations) == 0 && registered:
		row.goVerdict = "manual: registered, Go not located"
	case len(row.goLocations) == 0:
		row.goVerdict = "not in sim Go"
	case len(row.serverValues) == 0:
		row.goVerdict = "manual: no numeric values"
	default:
		for _, v := range row.serverValues {
			if !goHasValue(row.goNumbers, v) {
				row.notInGo = append(row.notInGo, v)
			}
		}
		row.goVerdict = "matches server"
		if len(row.notInGo) > 0 {
			row.goVerdict = "differs from server"
		}
	}
	if len(row.goLocations) > 0 && !registered {
		row.goVerdict += " (not registered)"
	}
}

// locateSetGo lists a set bonus's Go code for review; set bonus values are spread across class
// spell code, too loosely to compare automatically.
func (c *context) locateSetGo(row *effectRow, setName string) {
	row.goLocations = c.goIndex.findSetUsages(setName)
	c.collectGoNumbers(row)
	row.goVerdict = "not in sim Go"
	if len(row.goLocations) > 0 {
		row.goVerdict = "review Go locations"
	}
}

func (c *context) collectGoNumbers(row *effectRow) {
	ids := append([]int32{row.id}, row.chain...)
	for _, loc := range row.goLocations {
		for _, n := range loc.numbers() {
			if !slices.Contains(ids, int32(n)) && !containsNumber(row.goNumbers, n) {
				row.goNumbers = append(row.goNumbers, n)
			}
		}
	}
}

func triggerName(trigger int32) string {
	switch trigger {
	case azerothcore.ItemSpellTriggerOnUse:
		return "use"
	case azerothcore.ItemSpellTriggerOnEquip:
		return "equip"
	case azerothcore.ItemSpellTriggerChanceOnHit:
		return "chance on hit"
	}
	return "trigger " + strconv.Itoa(int(trigger))
}

// itemCastBy applies item_template's cooldown and, for chance-on-hit spells, its PPM
// (Player::CastItemCombatSpell uses the spell's DBC chance when the item has none, never spell_proc).
func (c *context) itemCastBy(itemID int32, spell azerothcore.ItemSpell) castBy {
	by := castBy{
		label:      fmt.Sprintf("item %d", itemID),
		cast:       spell.Trigger == azerothcore.ItemSpellTriggerOnUse || spell.Trigger == azerothcore.ItemSpellTriggerChanceOnHit,
		cooldownMs: spell.CooldownMs(),
	}
	if spell.Trigger == azerothcore.ItemSpellTriggerChanceOnHit {
		by.procRateSet = true
		by.ppm = spell.PPMRate
		if s := c.dbc.Spells[spell.SpellID]; s != nil && by.ppm == 0 {
			by.procChance = float64(s.ProcChance)
		}
	}
	return by
}

// enchantCastBy follows Player::CastItemCombatSpell: a combat spell procs with the enchantment's
// amount as its chance, unless spell_enchant_proc_data sets a PPM or a custom chance.
func (c *context) enchantCastBy(spell azerothcore.EnchantSpell) castBy {
	by := castBy{
		label:      fmt.Sprintf("enchant %d", spell.EnchantID),
		cast:       spell.Type == azerothcore.EnchantTypeCombatSpell || spell.Type == azerothcore.EnchantTypeUseSpell,
		cooldownMs: -1,
	}
	if spell.Type != azerothcore.EnchantTypeCombatSpell {
		return by
	}
	by.procRateSet = true
	by.procChance = float64(spell.Amount)
	if proc, ok := c.enchantProcs[spell.EnchantID]; ok {
		if proc.PPM > 0 {
			by.procChance, by.ppm = 0, proc.PPM
		} else if proc.CustomChance > 0 {
			by.procChance = proc.CustomChance
		}
	}
	return by
}

func (c *context) diffItemEffects() []effectRow {
	var rows []effectRow
	for _, id := range sortedKeys(c.sim.Items) {
		serverRow := c.items[id]
		if serverRow == nil {
			continue
		}
		converted := azerothcore.ConvertItem(serverRow, c.dbc)
		for _, spell := range converted.EffectSpells {
			row := effectRow{kind: "item", id: id, name: serverRow.Name, context: triggerName(spell.Trigger)}
			c.compareSpell(&row, spell.SpellID, c.itemCastBy(id, spell))
			c.compareEffectGo(&row, id, core.HasItemEffect(id))
			row.moduleFiles = appendUnique(row.moduleFiles, c.modules.items[id]...)
			rows = append(rows, row)
		}
	}
	return rows
}

type setRow struct {
	name   string
	issue  string
	detail string
}

func (c *context) diffSets() (effects []effectRow, issues []setRow) {
	simSets := map[string][]int32{}
	for _, id := range sortedKeys(c.sim.Items) {
		if name := c.sim.Items[id].SetName; name != "" {
			simSets[name] = append(simSets[name], id)
		}
	}

	names := make([]string, 0, len(simSets))
	for name := range simSets {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		var setIDs []int32
		for _, itemID := range simSets[name] {
			if row := c.items[itemID]; row != nil && row.ItemSet != 0 && !slices.Contains(setIDs, row.ItemSet) {
				setIDs = append(setIDs, row.ItemSet)
			}
		}
		slices.Sort(setIDs)

		if len(setIDs) == 0 {
			issues = append(issues, setRow{name, "no set on server", fmt.Sprintf("sim items %v have no itemset on the server", simSets[name])})
			continue
		}
		if len(setIDs) > 1 {
			var parts []string
			for _, setID := range setIDs {
				if set := c.dbc.ItemSets[setID]; set != nil {
					parts = append(parts, fmt.Sprintf("%d %q", setID, set.Name))
				}
			}
			issues = append(issues, setRow{name, "split on server",
				"sim counts these pieces as one set; the server tracks them as separate sets: " + strings.Join(parts, ", ")})
		}

		for _, setID := range setIDs {
			set := c.dbc.ItemSets[setID]
			if set == nil {
				issues = append(issues, setRow{name, "itemset missing from ItemSet.dbc", fmt.Sprint(setID)})
				continue
			}
			if database.NormalizeSetName(set.Name) != name {
				issues = append(issues, setRow{name, "name differs", fmt.Sprintf("server set %d is %q", setID, set.Name)})
			}
			for i, spellID := range set.Spells {
				row := effectRow{kind: "set", id: setID, name: set.Name, context: fmt.Sprintf("%d pieces", set.Thresholds[i])}
				c.compareSpell(&row, spellID, castBySpell)
				c.locateSetGo(&row, name)
				effects = append(effects, row)
			}
		}
	}
	return effects, issues
}

func appendUnique(dst []string, values ...string) []string {
	for _, v := range values {
		if !slices.Contains(dst, v) {
			dst = append(dst, v)
		}
	}
	return dst
}

func valuesString(values []spellValue) string {
	var parts []string
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%s=%s", v.label, num(v.value)))
	}
	return strings.Join(parts, "; ")
}

func numbersString(numbers []float64) string {
	var parts []string
	for _, n := range numbers {
		parts = append(parts, num(n))
	}
	return strings.Join(slices.Compact(parts), " ")
}

func idsString(ids []int32) string {
	var parts []string
	for _, id := range ids {
		parts = append(parts, fmt.Sprint(id))
	}
	return strings.Join(parts, " > ")
}
