package main

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core"
)

// TestPetfixRecords records the two guardians strikes.jsonl has none of, the Greater Fire Elemental and a
// Mirror Image: `info` and `damage` like a strikes pet's, and `spell`, `melee` and `timing` through pet:<n>.
// An Imp and a Water Elemental add `info` and their nuke's `spell` roll, a Shadowfiend its `melee`. The
// replay compares each on the sim's pet, scaled off the record's owner.

const petsFixture = "pets.jsonl"

// petsRecord is a pets.jsonl line: a guardian's `info`, `damage`, `spell`, `melee` or `timing`.
type petsRecord struct {
	strikesRecord
	Iterations float64            `json:"iterations"`
	Derived    json.RawMessage    `json:"derived"`
	Crit       counted            `json:"crit"`
	Outcomes   map[string]petHits `json:"outcomes"`
	// a `spell` record's SpellHitResult rolls, by result
	HitResults map[string]counted `json:"hitResults"`
	// the same line as a timing record, for a `timing`
	cast *timingRecord
	// the attacker's power pool, which the unit snapshot leaves out
	maxPower float64
}

// petHits is one outcome of a `melee` record: how often it came, and its mean damage with what the dummy
// took off it.
type petHits struct {
	Count        float64 `json:"count"`
	MeanDamage   float64 `json:"meanDamage"`
	MeanBlocked  float64 `json:"meanBlocked"`
	MeanAbsorbed float64 `json:"meanAbsorbed"`
	MeanResisted float64 `json:"meanResisted"`
}

func readPetsRecords(path string) ([]petsRecord, error) {
	lines, err := readJSONLines[json.RawMessage](path)
	if err != nil {
		return nil, err
	}
	records := make([]petsRecord, len(lines))
	for i, line := range lines {
		rec := &records[i]
		var pool struct {
			Attacker struct {
				MaxPower float64 `json:"maxPower"`
			} `json:"attacker"`
		}
		if err := firstErr(json.Unmarshal(line, rec), json.Unmarshal(line, &pool)); err != nil {
			return nil, fmt.Errorf("%s record %d: %w", path, i+1, err)
		}
		rec.maxPower = pool.Attacker.MaxPower
		if rec.Command == "timing" {
			rec.cast = new(timingRecord)
			if err := json.Unmarshal(line, rec.cast); err != nil {
				return nil, fmt.Errorf("%s record %d: %w", path, i+1, err)
			}
		}
	}
	return records, nil
}

// petsKey names a pets.jsonl record by what it probed.
type petsKey struct {
	command string
	spell   int32
	entry   uint32
}

func (k petsKey) String() string {
	if k.spell == 0 {
		return fmt.Sprintf("%s of pet %d", k.command, k.entry)
	}
	return fmt.Sprintf("%s %d of pet %d", k.command, k.spell, k.entry)
}

func (r petsRecord) key() petsKey { return petsKey{r.Command, r.Spell.ID, r.Attacker.Entry} }

// petCadenceAllowed are the pet spells whose GCD the sim keeps as its stand-in for the AI's timers, each
// with a ServerGCD allowance in its class's serverdata_allowlist.go.
var petCadenceAllowed = map[int32]string{
	57984: "npc_pet_shaman_fire_elemental's event timers pace it; sim/shaman/serverdata_allowlist.go keeps the sim's GCD",
	12470: "npc_pet_shaman_fire_elemental's event timers pace it; sim/shaman/serverdata_allowlist.go keeps the sim's GCD",
	59638: "npc_pet_mage_mirror_image's event timers pace it; sim/mage/serverdata_allowlist.go keeps the sim's GCD",
	59637: "npc_pet_mage_mirror_image's event timers pace it; sim/mage/serverdata_allowlist.go keeps the sim's GCD",
}

// checkPetsRecord compares a pets.jsonl record with the sim.
func checkPetsRecord(rec petsRecord) (checks []Check) {
	if !rec.isPet() {
		return []Check{fail("record", rec.Attacker.Name+" is no player's pet")}
	}
	defer func() {
		if r := recover(); r != nil {
			checks = append(checks, fail("sim pet", fmt.Sprintf("panic: %v", r)))
		}
	}()
	switch rec.Command {
	case "info":
		if rec.Pet == nil {
			return []Check{fail("record", "an info without a pet part")}
		}
		return append(checkPetInfo(rec.strikesRecord), checkPetPools(rec)...)
	case "damage":
		return checkPetDamage(rec.damageRecord)
	case "spell":
		return checkPetSpellRoll(rec)
	case "melee":
		return checkPetSwing(rec)
	case "timing":
		if rec.cast != nil {
			return checkPetTiming(rec)
		}
	}
	return []Check{fail("record", fmt.Sprintf("no replay for a pet's %q", rec.Command))}
}

// petsWithoutHealth are the pets the sim gives no health pool of their own, only core's 10 a point of
// stamina. Nothing in the sim reads a pet's health.
var petsWithoutHealth = map[uint32]string{
	416: "the sim's Imp lacks pet_levelstats' health and Guardian::UpdateMaxHealth's 8.4 a point of stamina; nothing reads a pet's health",
	510: "the sim's Water Elemental lacks pet_levelstats' health and Guardian::UpdateMaxHealth's 7.5 a point of stamina; " +
		"nothing reads a pet's health",
}

// checkPetPools holds the pet's health and mana pools to the record's. The server truncates the mana
// pool, so the sim's may sit a point above.
func checkPetPools(rec petsRecord) []Check {
	d, err := buildPetSim(rec.damageRecord)
	if err != nil {
		return []Check{fail("sim pet", err.Error())}
	}
	d.start()
	pet := d.pet.pet
	var checks []Check
	if why, ok := petsWithoutHealth[rec.Attacker.Entry]; ok {
		if math.Abs(pet.MaxHealth()-rec.Attacker.MaxHealth) <= 0.5 {
			checks = append(checks, fail("max health", "the sim's matches now, so take the pet off petsWithoutHealth"))
		} else {
			checks = append(checks, notCompared("max health", why))
		}
	} else {
		checks = append(checks, checkNear("max health", pet.MaxHealth(), rec.Attacker.MaxHealth, 0.5))
	}
	if rec.Attacker.PowerType == powerMana {
		if !pet.HasManaBar() {
			return append(checks, fail("max mana", fmt.Sprintf("server %.0f, the sim's has no mana", rec.maxPower)))
		}
		checks = append(checks, checkNear("max mana", pet.MaxMana(), rec.maxPower, 1))
	}
	return checks
}

// petSpell is the pet's spell with the id, or the one that takes its numbers from it.
func (d *damageSim) petSpell(id int32) *core.Spell {
	var found *core.Spell
	for _, spell := range d.pet.pet.Spellbook {
		if spell.ActionID.ItemID != 0 || spell.ActionID.OtherID != 0 {
			continue
		}
		if spell.SpellID != id && spell.Direct.FromSpellID != id {
			continue
		}
		if found == nil || spell.Tag < found.Tag {
			found = spell
		}
	}
	return found
}

// checkPetSpellRoll holds the spell's crit chance and miss against the dummy to the record's derived
// table, and the server's rolled crits and misses to the sim's chances.
func checkPetSpellRoll(rec petsRecord) []Check {
	var server spellDerived
	if err := json.Unmarshal(rec.Derived, &server); err != nil {
		return []Check{fail("spell table", err.Error())}
	}
	d, err := buildPetSim(rec.damageRecord)
	if err != nil {
		return []Check{fail("sim pet", err.Error())}
	}
	spell := d.petSpell(rec.Spell.ID)
	if spell == nil {
		return []Check{fail("sim spell", fmt.Sprintf("the sim's %s has no spell %d", d.pet.pet.Name, rec.Spell.ID))}
	}
	d.start()
	crit := spell.SpellCritChance(d.target) * 100
	checks := []Check{checkNear("spell crit %", crit, server.CritTaken, 1e-3)}
	if rec.Iterations > 0 {
		checks = append(checks, checkRate("rolled crit", rec.Crit.Count, rec.Iterations, int32(math.Round(crit*100))))
	}
	if server.Magic != nil && !server.Magic.AutoHit {
		miss := int32(math.Round(spell.SpellChanceToMiss(d.pet.pet.AttackTables[d.target.UnitIndex]) * core.MaxRollBP))
		checks = append(checks, checkBP("spell miss threshold", miss, server.Magic.MissThreshold))
		if rec.Iterations > 0 {
			checks = append(checks, checkRate("rolled miss", rec.HitResults["miss"].Count, rec.Iterations, miss))
		}
	}
	return checks
}

// checkPetSwing holds the pet's white table against the dummy to the record's, and its mean plain swing
// before the dummy's block and resists to the server's mean over its rolls.
func checkPetSwing(rec petsRecord) []Check {
	var server whiteTable
	if err := json.Unmarshal(rec.Derived, &server); err != nil {
		return []Check{fail("white table", err.Error())}
	}
	d, err := buildPetSim(rec.damageRecord)
	if err != nil {
		return []Check{fail("sim pet", err.Error())}
	}
	pet := d.pet.pet
	if !pet.AutoAttacks.AutoSwingMelee {
		return []Check{fail("swing", "the sim's "+pet.Name+" never swings")}
	}
	d.start()
	auto := pet.AutoAttacks.MHAuto()
	attackTable := pet.AttackTables[d.target.UnitIndex]
	front := pet.PseudoStats.InFrontOfTarget
	pet.PseudoStats.InFrontOfTarget = rec.Target.OtherInFront
	table := core.WhiteMeleeTableBP(auto.WhiteTableInput(attackTable))
	pet.PseudoStats.InFrontOfTarget = front

	checks := []Check{
		checkBP("white miss", table.Miss, server.MissBp),
		checkBP("white dodge", table.Dodge, server.DodgeBp),
		checkBP("white parry", table.Parry, server.ParryBp),
		checkBP("white block", table.Block, server.BlockBp),
		checkBP("white glancing", table.Glance, server.GlancingBp),
		checkBP("white crushing", table.Crush, server.CrushingBp),
		checkBP("white crit", table.Crit, server.CritBp),
	}
	if hits, ok := rec.Outcomes["crit"]; ok && rec.Iterations > 0 {
		checks = append(checks, checkRate("rolled crit", hits.Count, rec.Iterations, table.Crit))
	}

	normal, ok := rec.Outcomes["normal"]
	if !ok || normal.Count == 0 {
		return append(checks, fail("mean swing", "the record has no plain hit"))
	}
	lo, errLo := d.petSwing(0)
	hi, errHi := d.petSwing(1)
	if err := firstErr(errLo, errHi); err != nil {
		return append(checks, fail("mean swing", err.Error()))
	}
	// 5 standard errors of the even roll, plus 2 points for the server's truncations
	bound := maxZ*(hi-lo)/math.Sqrt(12*normal.Count) + 2
	dealt := normal.MeanDamage + normal.MeanBlocked + normal.MeanAbsorbed + normal.MeanResisted
	checks = append(checks, checkNear("mean swing", (lo+hi)/2, dealt, bound))
	if dealt > 0 {
		checks = append(checks, checkNear("resisted share", attackTable.AverageResist(auto), normal.MeanResisted/dealt, 0.005))
	}
	return checks
}

// petSwing is the pet's plain white swing at one end of the weapon's roll, with no resist.
func (d *damageSim) petSwing(end int) (amount float64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	unit := &d.pet.pet.Unit
	auto := unit.AutoAttacks.MHAuto()
	flags, hooks := auto.Flags, d.target.DynamicDamageTakenModifiers
	hit, crit := auto.BonusHitRating, auto.BonusCritRating
	defer func() {
		auto.Flags, d.target.DynamicDamageTakenModifiers = flags, hooks
		auto.BonusHitRating, auto.BonusCritRating = hit, crit
	}()
	auto.Flags |= core.SpellFlagIgnoreResists
	landed := false
	d.target.DynamicDamageTakenModifiers = append(slices.Clip(hooks), func(_ *core.Simulation, s *core.Spell, result *core.SpellResult) {
		if s == auto && !landed && result.Outcome == core.OutcomeHit {
			amount, landed = result.Damage, true
		}
	})
	for range maxHitAttempts {
		d.prepare(d.rec.SpellPower.Damage)
		auto.BonusHitRating, auto.BonusCritRating = hit+forcedHitRating, crit-forcedCritRating
		restore := pinWeapons(unit, end)
		auto.ApplyEffects(d.sim, d.target, auto)
		restore()
		if landed {
			return amount, nil
		}
	}
	return 0, fmt.Errorf("no plain hit in %d swings", maxHitAttempts)
}

// checkPetTiming holds the spell's cast time and cost to the record's. A creature has no cooldown in the
// record, and its GCD is left to petCadenceAllowed.
func checkPetTiming(rec petsRecord) []Check {
	server := rec.cast
	d, err := buildPetSim(rec.damageRecord)
	if err != nil {
		return []Check{fail("sim pet", err.Error())}
	}
	spell := d.petSpell(rec.Spell.ID)
	if spell == nil {
		return []Check{fail("sim spell", fmt.Sprintf("the sim's %s has no spell %d", d.pet.pet.Name, rec.Spell.ID))}
	}
	d.start()
	checks := []Check{
		checkNear("cast speed", d.pet.pet.CastSpeed, server.Cast.CastSpeed, 1e-4),
		checkInt("cast time before haste", simMs(spell.DefaultCast.CastTime), int64(max(0, server.Cast.AfterSpellModsMs))),
		checkInt("cast time", simMs(spell.CastTime()), int64(server.Cast.Ms)),
		checkInt("cost", int64(math.Floor(spell.ApplyCostModifiers(spell.DefaultCast.Cost)+1e-6)), int64(server.Cost.Amount)),
	}
	if why, ok := petCadenceAllowed[rec.Spell.ID]; ok {
		return append(checks, notCompared("gcd", why))
	}
	ts := &timingSim{damageSim: d, spell: spell}
	return append(checks, ts.checkGCD(*server)...)
}
