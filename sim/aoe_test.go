package sim

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/tools/acore/spellids/spellset"
)

// aoeCaster is who casts a spell on the server, where only a player's damage gets the ten-target cap.
type aoeCaster int

const (
	byPlayer aoeCaster = iota
	byPet              // pets and guardians, the rune weapon too
	byTotem
	byTrap // the world trigger GameObject::CastSpell summons for a trap
	byNPC
)

var aoeCasterNames = [...]string{"player", "pet", "totem", "trap", "boss"}

type aoeKey struct {
	spell  int32
	caster aoeCaster
}

// aoeSpell is what the class code hard-codes for one spell.
type aoeSpell struct {
	chain      int32               // targets the chain reaches counting the first, 0 when it doesn't chain
	perJump    float64             // share of the damage or healing each jump keeps, 0 means all of it
	maxTargets int32               // most targets an area hit picks, 0 for all of them
	capped     bool                // multiplied by Encounter.AOECapMultiplier
	mods       map[int32]aoeValues // the spell with one talent, glyph or set bonus on, by its spell id
}

// aoeValues are a spell's chain targets, per-jump multiplier and max targets. In aoeSpell.mods a zero
// field keeps the spell's own value.
type aoeValues struct {
	chain      int32
	perJump    float64
	maxTargets int32
}

// normalized reads 0 and 1 chain targets alike, and a per-jump multiplier only where there's a jump.
func (v aoeValues) normalized() aoeValues {
	v.chain = max(v.chain, 1)
	if v.perJump == 0 || v.chain == 1 {
		v.perJump = 1
	}
	return v
}

func (s aoeSpell) values() aoeValues {
	return aoeValues{s.chain, s.perJump, s.maxTargets}.normalized()
}

func (s aoeSpell) with(mod aoeValues) aoeValues {
	v := aoeValues{s.chain, s.perJump, s.maxTargets}
	v.chain = cmp.Or(mod.chain, v.chain)
	v.perJump = cmp.Or(mod.perJump, v.perJump)
	v.maxTargets = cmp.Or(mod.maxTargets, v.maxTargets)
	return v.normalized()
}

// aoeSpells is every spell the sim names (tools/acore/spellids) that chains, picks a max number of
// targets, has a modifier that moves either, or deals area or non-magic chain damage the server's
// ten-target cap would take for a player, plus the explosives.
var aoeSpells = map[aoeKey]aoeSpell{
	{1680, byPlayer}:  {maxTargets: 4}, // Whirlwind
	{12470, byPet}:    {},              // Fire Nova, the Fire Elemental's
	{13376, byPet}:    {},              // Fire Shield, the Fire Elemental's
	{20153, byPet}:    {},              // Immolation, the Infernal's
	{20424, byPlayer}: {chain: 3},      // Seal of Command
	{21992, byPlayer}: {chain: 5},      // Thunderfury
	{22703, byPlayer}: {capped: true},  // Inferno Effect
	{42921, byPlayer}: {capped: true},  // Arcane Explosion
	{42925, byPlayer}: {capped: true},  // Flamestrike rank 8
	{42926, byPlayer}: {capped: true},  // Flamestrike
	{42938, byPlayer}: {capped: true},  // Blizzard
	{42945, byPlayer}: {capped: true},  // Blast Wave
	{42950, byPlayer}: {capped: true},  // Dragon's Breath
	{44949, byPlayer}: {maxTargets: 4}, // Whirlwind's off-hand, Bladestorm's too
	{46968, byPlayer}: {capped: true},  // Shockwave
	{47502, byPlayer}: {capped: true},  // Thunder Clap
	// Cleave, Glyph of Cleaving
	{47520, byPlayer}: {chain: 2, mods: map[int32]aoeValues{58366: {chain: 3}}},
	{47834, byPlayer}: {capped: true}, // Seed of Corruption
	{47994, byPet}:    {chain: 2},     // Cleave, the Felguard's
	{48466, byPlayer}: {capped: true}, // Hurricane
	// Maul, Glyph of Maul
	{48480, byPlayer}: {mods: map[int32]aoeValues{54811: {chain: 2}}},
	{48562, byPlayer}: {capped: true},                                 // Swipe (Bear)
	{48564, byPlayer}: {mods: map[int32]aoeValues{58923: {chain: 3}}}, // Mangle (Bear), Berserk
	{48817, byPlayer}: {capped: true},                                 // Holy Wrath
	// Avenger's Shield, its glyph
	{48827, byPlayer}: {chain: 3, mods: map[int32]aoeValues{54930: {chain: 1}}},
	{49048, byPlayer}: {chain: 3}, // Multi-Shot
	{49065, byTrap}:   {},         // Explosive Trap Effect
	// Chain Lightning, Glyph of Chain Lightning, Tidefury Raiment's 2 pieces
	{49271, byPlayer}: {chain: 3, perJump: 0.7, mods: map[int32]aoeValues{55449: {chain: 4}, 37207: {perJump: 0.833}}},
	{49273, byPlayer}: {},              // Healing Wave
	{49941, byPlayer}: {capped: true},  // Blood Boil
	{49941, byPet}:    {},              // Blood Boil, the rune weapon's
	{50526, byPlayer}: {capped: true},  // Wandering Plague
	{50590, byPlayer}: {capped: true},  // Immolation Aura's tick
	{50622, byPlayer}: {maxTargets: 4}, // Bladestorm's Whirlwind
	{51411, byPlayer}: {capped: true},  // Howling Blast
	{51723, byPlayer}: {capped: true},  // Fan of Knives
	{52874, byPlayer}: {capped: true},  // Fan of Knives' off-hand
	{53022, byPlayer}: {capped: true},  // Mind Sear's tick
	{53190, byPlayer}: {capped: true},  // Starfall's splash
	{53227, byPlayer}: {capped: true},  // Typhoon
	{53385, byPlayer}: {maxTargets: 4}, // Divine Storm
	{53533, byPet}:    {chain: 1000},   // Swipe, the hunter pet's
	// Hammer of the Righteous, its glyph
	{53595, byPlayer}: {chain: 3, mods: map[int32]aoeValues{63219: {chain: 4}}},
	{55262, byPlayer}: {chain: 2, perJump: 0.5}, // Heart Strike
	{55262, byPet}:    {chain: 2, perJump: 0.5}, // Heart Strike, the rune weapon's
	{55362, byPlayer}: {capped: true},           // Living Bomb's explosion
	// Chain Heal, its glyph
	{55459, byPlayer}: {chain: 3, perJump: 0.6, mods: map[int32]aoeValues{55437: {chain: 4}}},
	// Revenge, Improved Revenge's two ranks
	{57823, byPlayer}: {mods: map[int32]aoeValues{12797: {chain: 2, perJump: 0.5}, 12799: {chain: 2}}},
	{57842, byPlayer}: {maxTargets: 1},             // Killing Spree's strike
	{58433, byPlayer}: {capped: true},              // Volley's tick
	{58735, byTotem}:  {},                          // Magma Totem
	{59159, byPlayer}: {capped: true},              // Thunderstorm
	{61654, byPlayer}: {capped: true},              // Fire Nova, cast by the shaman at the totem
	{62078, byPlayer}: {chain: 1000, capped: true}, // Swipe (Cat): every target
	{64596, byNPC}:    {},                          // Cosmic Smash
	{65108, byNPC}:    {},                          // Black Hole Explosion
	{66330, byNPC}:    {},                          // Staggering Stomp
	{69649, byNPC}:    {},                          // Frost Breath
	{71904, byPlayer}: {capped: true},              // Chaos Bane
	{73061, byNPC}:    {},                          // Frost Breath
	{56350, byPlayer}: {capped: true},              // Saronite Bomb
	{56488, byPlayer}: {capped: true},              // Global Thermal Sapper Charge
	{67769, byPlayer}: {capped: true},              // Cobalt Frag Bomb
}

// aoeDiff is one value the sim and the server disagree on: field is "listed", "mod", "chain",
// "per jump", "max targets" or "capped", and mod the modifier it comes with, or 0.
type aoeDiff struct {
	aoeKey
	mod   int32
	field string
}

// aoeAllowances are the values the sim and the server disagree on, with the reason. Each sim mismatch
// is a follow-up.
var aoeAllowances = map[aoeDiff]string{
	{aoeKey{47994, byPet}, 58366, "mod"}:    "Glyph of Cleaving is a warrior glyph: a pet's spell mods come from its owner (Unit::GetSpellModOwner), and no warlock has it.",
	{aoeKey{49271, byPlayer}, 26123, "mod"}: "Gift of the Gathering Storm's 3 pieces: the sim doesn't model the set. Follow-up: sim mismatch.",
	{aoeKey{55459, byPlayer}, 21899, "mod"}: "The Ten Storms' 3 pieces: the sim doesn't model the set. Follow-up: sim mismatch.",
	{aoeKey{49273, byPlayer}, 23573, "mod"}: "The Earthfury's 8 pieces: the sim doesn't model the set. Follow-up: sim mismatch.",
}

// SpellImplicitTargetInfo::_data's targets in TARGET_SELECT_CATEGORY_AREA or _CONE.
var areaTargets = map[int64]bool{7: true, 8: true, 15: true, 16: true, 20: true, 24: true, 30: true, 31: true,
	33: true, 34: true, 37: true, 51: true, 52: true, 54: true, 56: true, 59: true, 60: true, 61: true, 93: true,
	104: true, 108: true}

// The targets SelectImplicitTargetObjectTargets and SelectImplicitNearbyTargets pick, the only ones
// Spell::SelectImplicitChainTargets jumps from. An effect on the caster never chains.
var chainTargets = map[int64]bool{2: true, 3: true, 4: true, 6: true, 21: true, 23: true, 25: true, 26: true,
	35: true, 38: true, 40: true, 45: true, 46: true, 57: true, 58: true, 90: true, 95: true}

// The effects whose handlers fill m_damage at SPELL_EFFECT_HANDLE_LAUNCH_TARGET: school damage and the
// four weapon damage effects. Heals land as negative damage, which the cap skips.
var launchDamageEffects = map[int64]bool{2: true, 17: true, 31: true, 58: true, 121: true}

const (
	dmgClassMagic              = 1
	attr3IgnoreCasterModifiers = 0x20000000
	auraAddFlatModifier        = 107
	auraAddPctModifier         = 108
	auraModMaxAffectedTargets  = 277
	spellModJumpTargets        = 17
	spellModDamageMultiplier   = 20
)

func targetsArea(e spellset.DumpEffect) bool {
	return areaTargets[e.TargetA] || areaTargets[e.TargetB]
}

func chains(e spellset.DumpEffect) bool {
	return chainTargets[e.TargetA] || chainTargets[e.TargetB]
}

// capReach is how many targets the launch damage the cap covers can reach: unbounded for an area hit
// without max targets, else its max targets or its chain. 0 when the cap covers none of its damage.
func capReach(s *spellset.DumpSpell) int64 {
	var reach int64
	for _, e := range s.Effects {
		switch {
		case !launchDamageEffects[e.Effect]:
		case targetsArea(e) && s.MaxAffectedTargets == 0:
			return math.MaxInt64
		case targetsArea(e):
			reach = max(reach, s.MaxAffectedTargets)
		case e.ChainTarget > 1 && s.DmgClass != dmgClassMagic:
			reach = max(reach, e.ChainTarget)
		}
	}
	return reach
}

// serverCaps is whether the cap can cut the spell's damage: it only kicks in past ten targets, so a
// chain or a max-targets hit that stops short of that never gets it.
func serverCaps(s *spellset.DumpSpell, caster aoeCaster) bool {
	return caster == byPlayer && capReach(s) > 10
}

// aoeModifier is one effect that moves chains or max targets: a spell mod on SPELLMOD_JUMP_TARGETS or
// SPELLMOD_DAMAGE_MULTIPLIER, or SPELL_AURA_MOD_MAX_AFFECTED_TARGETS.
type aoeModifier struct {
	spell  *spellset.DumpSpell
	effect spellset.DumpEffect
}

func aoeModifiers(t *testing.T, dump spellset.Dump) []aoeModifier {
	var mods []aoeModifier
	for _, id := range dump.IDs() {
		for _, e := range dump[id].Effects {
			spellMod := (e.Aura == auraAddFlatModifier || e.Aura == auraAddPctModifier) &&
				(e.MiscValue == spellModJumpTargets || e.MiscValue == spellModDamageMultiplier)
			if !spellMod && e.Aura != auraModMaxAffectedTargets {
				continue
			}
			if e.DieSides > 1 {
				t.Errorf("%s (%d) rolls its modifier between values, which serverAOE can't apply", dump[id].Name, id)
			}
			mods = append(mods, aoeModifier{dump[id], e})
		}
	}
	return mods
}

// affects is SpellInfo::IsAffectedBySpellMod for a spell mod and AuraEffect::IsAffectedOnSpell for the
// aura. Both come down to SpellInfo::IsAffected.
func (m aoeModifier) affects(s *spellset.DumpSpell) bool {
	if m.effect.Aura != auraModMaxAffectedTargets && s.Attributes[3]&attr3IgnoreCasterModifiers != 0 {
		return false
	}
	switch mask := m.effect.ClassMask; {
	case m.spell.Family == 0:
		return true
	case m.spell.Family != s.Family:
		return false
	default:
		return mask == [3]int64{} || mask[0]&s.FamilyFlags[0] != 0 || mask[1]&s.FamilyFlags[1] != 0 || mask[2]&s.FamilyFlags[2] != 0
	}
}

// applyMods is Player::ApplySpellMod's base × (100 + pct)% + flat for one op. Only cast time and
// duration add the flat part first.
func applyMods(base float64, mods []aoeModifier, op int64) float64 {
	var flat, pct float64
	for _, m := range mods {
		amount := float64(m.effect.BasePoints + m.effect.DieSides)
		switch {
		case m.effect.Aura == auraAddFlatModifier && m.effect.MiscValue == op:
			flat += amount
		case m.effect.Aura == auraAddPctModifier && m.effect.MiscValue == op:
			pct += amount
		}
	}
	return base*(100+pct)/100 + flat
}

// serverAOE is the spell's chain, per-jump multiplier and max targets with mods on:
//   - chain: an effect's ChainTarget plus SPELLMOD_JUMP_TARGETS (Spell::SelectImplicitChainTargets),
//     only for an effect aimed at the explicit target or a nearby one.
//   - per jump: that effect's DamageMultiplier with SPELLMOD_DAMAGE_MULTIPLIER
//     (SpellEffectInfo::CalcDamageMultiplier, applied by Spell::HandleLaunchPhase).
//   - max targets: a nonzero MaxAffectedTargets plus SPELL_AURA_MOD_MAX_AFFECTED_TARGETS
//     (Spell::SelectImplicitAreaTargets).
func serverAOE(s *spellset.DumpSpell, mods []aoeModifier) aoeValues {
	var v aoeValues
	for _, e := range s.Effects {
		if e.Effect == 0 || !chains(e) {
			continue
		}
		if chain := int32(applyMods(float64(e.ChainTarget), mods, spellModJumpTargets)); chain > v.chain {
			v.chain = chain
			v.perJump = applyMods(float64(e.DamageMultiplier), mods, spellModDamageMultiplier)
		}
	}
	if s.MaxAffectedTargets > 0 {
		v.maxTargets = int32(s.MaxAffectedTargets)
		for _, m := range mods {
			if m.effect.Aura == auraModMaxAffectedTargets {
				v.maxTargets += int32(m.effect.BasePoints + m.effect.DieSides)
			}
		}
	}
	return v.normalized()
}

func (v aoeValues) diff(server aoeValues) map[string][2]string {
	diffs := map[string][2]string{}
	if v.chain != server.chain {
		diffs["chain"] = [2]string{fmt.Sprint(v.chain), fmt.Sprint(server.chain)}
	}
	if math.Abs(v.perJump-server.perJump) > 1e-4 {
		diffs["per jump"] = [2]string{fmt.Sprintf("%.4g", v.perJump), fmt.Sprintf("%.4g", server.perJump)}
	}
	if v.maxTargets != server.maxTargets {
		diffs["max targets"] = [2]string{fmt.Sprint(v.maxTargets), fmt.Sprint(server.maxTargets)}
	}
	return diffs
}

func spellLabel(s *spellset.DumpSpell) string {
	return fmt.Sprintf("%s (%d)", s.Name, s.ID)
}

func aoeLabel(s *spellset.DumpSpell, caster aoeCaster) string {
	if caster == byPlayer {
		return spellLabel(s)
	}
	return fmt.Sprintf("%s from a %s", spellLabel(s), aoeCasterNames[caster])
}

func compareAOEKeys(a, b aoeKey) int {
	return cmp.Or(cmp.Compare(a.spell, b.spell), cmp.Compare(a.caster, b.caster))
}

// TestSimAOESpells checks the chains, max targets and ten-target caps the class code hard-codes against
// the server's spell data in assets/db_inputs/acore/spelldump.jsonl. TestAOECapMultiplier covers the cap's
// formula, this covers which spells get it.
//
// The cap is Spell::DoAllEffectOnLaunchTarget's damage × 10 / targets past ten targets, counting every
// unit any of the spell's effects hits, the caster too. It takes the launch damage of an effect that
// targets an area or chains a hit that isn't magic, and only when a player casts the spell: a pet,
// guardian, totem, trap or boss never gets it, so aoeSpells says who casts each one.
//
// A spell the sim names that needs an entry and lacks one fails, and so does every value the sim and the
// server disagree on, unless aoeAllowances gives the reason. A stale allowance fails too.
func TestSimAOESpells(t *testing.T) {
	dump, err := spellset.ReadDump(filepath.Join("..", spellset.DefaultDumpPath))
	if err != nil {
		t.Fatal(err)
	}
	if len(dump) == 0 {
		t.Fatalf("no spell capture at %s", spellset.DefaultDumpPath)
	}
	lits, err := spellset.ScanSim("..")
	if err != nil {
		t.Fatal(err)
	}
	if len(lits.TypeErrors) > 0 {
		t.Fatalf("the sim scan lost constants to type errors:\n%s", strings.Join(lits.TypeErrors, "\n"))
	}
	modifiers := aoeModifiers(t, dump)
	// the modifiers that change what serverAOE gives for a spell, by modifier spell id
	movedBy := func(s *spellset.DumpSpell) map[int32][]aoeModifier {
		base := serverAOE(s, nil)
		moved := map[int32][]aoeModifier{}
		for _, m := range modifiers {
			if m.affects(s) {
				moved[int32(m.spell.ID)] = append(moved[int32(m.spell.ID)], m)
			}
		}
		for id, mods := range moved {
			if serverAOE(s, mods) == base {
				delete(moved, id)
			}
		}
		return moved
	}

	diffs := map[aoeDiff]string{}
	listed := map[int32]bool{}
	for _, key := range slices.SortedFunc(maps.Keys(aoeSpells), compareAOEKeys) {
		entry := aoeSpells[key]
		listed[key.spell] = true
		s := dump[key.spell]
		if s == nil {
			t.Errorf("%d isn't in the spell capture", key.spell)
			continue
		}
		label := aoeLabel(s, key.caster)
		if len(lits.Spells[key.spell]) == 0 {
			t.Errorf("%s: the sim no longer names it, so the entry is stale", label)
		}

		if sd := serverdata.SpellByID(key.spell); sd != nil {
			if int64(sd.MaxTargets) != s.MaxAffectedTargets {
				t.Errorf("%s: serverdata has %d max targets, the capture %d; rerun gen_serverdata", label, sd.MaxTargets, s.MaxAffectedTargets)
			}
			for _, e := range s.Effects {
				if g := sd.Effects[e.Index]; e.Effect != 0 && (int64(g.ChainTargets) != e.ChainTarget || g.DamageMultiplier != e.DamageMultiplier) {
					t.Errorf("%s effect %d: serverdata chains %d at %v, the capture %d at %v; rerun gen_serverdata",
						label, e.Index, g.ChainTargets, g.DamageMultiplier, e.ChainTarget, e.DamageMultiplier)
				}
			}
		}

		for field, values := range entry.values().diff(serverAOE(s, nil)) {
			diffs[aoeDiff{key, 0, field}] = fmt.Sprintf("%s: %s: sim %s, server %s", label, field, values[0], values[1])
		}
		if server := serverCaps(s, key.caster); server != entry.capped {
			diffs[aoeDiff{key, 0, "capped"}] = fmt.Sprintf("%s: capped past ten targets: sim %v, server %v", label, entry.capped, server)
		}

		moved := movedBy(s)
		for _, id := range slices.Sorted(maps.Keys(moved)) {
			modLabel := fmt.Sprintf("%s with %s", label, spellLabel(dump[id]))
			mod, ok := entry.mods[id]
			if !ok {
				diffs[aoeDiff{key, id, "mod"}] = fmt.Sprintf("%s: the server's chain becomes %+v, and the entry doesn't list the modifier",
					modLabel, serverAOE(s, moved[id]))
				continue
			}
			for field, values := range entry.with(mod).diff(serverAOE(s, moved[id])) {
				diffs[aoeDiff{key, id, field}] = fmt.Sprintf("%s: %s: sim %s, server %s", modLabel, field, values[0], values[1])
			}
		}
		for _, id := range slices.Sorted(maps.Keys(entry.mods)) {
			if _, ok := moved[id]; !ok {
				t.Errorf("%s: %d doesn't move its chain on the server, so the mod is stale", label, id)
			}
		}
	}

	for _, id := range slices.Sorted(maps.Keys(lits.Spells)) {
		s := dump[id]
		if s == nil || listed[id] {
			continue
		}
		var why []string
		if serverAOE(s, nil).chain > 1 {
			why = append(why, "it chains")
		}
		if s.MaxAffectedTargets > 0 {
			why = append(why, "it picks at most a number of targets")
		}
		for _, mod := range slices.Sorted(maps.Keys(movedBy(s))) {
			why = append(why, spellLabel(dump[mod])+" moves its chain")
		}
		if capReach(s) > 0 {
			why = append(why, "the server caps its damage for a player")
		}
		if len(why) > 0 {
			diffs[aoeDiff{aoeKey{id, byPlayer}, 0, "listed"}] = fmt.Sprintf("%s (%s): add it to aoeSpells, since %s",
				spellLabel(s), lits.Spells[id][0], strings.Join(why, ", "))
		}
	}

	compareDiffs := func(a, b aoeDiff) int {
		return cmp.Or(compareAOEKeys(a.aoeKey, b.aoeKey), cmp.Compare(a.mod, b.mod), cmp.Compare(a.field, b.field))
	}
	for _, d := range slices.SortedFunc(maps.Keys(diffs), compareDiffs) {
		if _, ok := aoeAllowances[d]; !ok {
			t.Error(diffs[d])
		}
	}
	for _, d := range slices.SortedFunc(maps.Keys(aoeAllowances), compareDiffs) {
		label := fmt.Sprintf("%d", d.spell)
		if s := dump[d.spell]; s != nil {
			label = aoeLabel(s, d.caster)
		}
		if d.mod != 0 {
			label += fmt.Sprintf(" with %d", d.mod)
		}
		if strings.TrimSpace(aoeAllowances[d]) == "" {
			t.Errorf("%s %s: the allowance needs a reason", label, d.field)
		}
		if _, ok := diffs[d]; !ok {
			t.Errorf("%s %s: no longer differs, so its allowance is stale", label, d.field)
		}
	}
}
