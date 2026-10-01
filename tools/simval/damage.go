package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	simpkg "github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

// `.simval damage` records each probed effect of one spell at both ends of its roll, through the
// server's done and taken bonuses, before armor, partial resists and the crit roll. The replay builds
// the sim's character from the record and runs the sim's own code for that effect with the record's
// base pinned, a plain hit forced and resists off, so talents, glyphs and coefficients all show.

const damageFixture = "damage.jsonl"

type damageRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type damageEffect struct {
	Index int32 `json:"index"`
	// damage, dot, heal or hot when probed; unprobed and other carry no amounts
	Kind        string      `json:"kind"`
	Coefficient float64     `json:"coefficient"`
	Base        damageRange `json:"base"`
	Taken       damageRange `json:"taken"`
	Crit        struct {
		Amount float64 `json:"amount"`
		Result float64 `json:"result"`
	} `json:"crit"`
}

func (e damageEffect) probed() bool {
	return e.Kind == "damage" || e.Kind == "dot" || e.Kind == "heal" || e.Kind == "hot"
}
func (e damageEffect) periodic() bool { return e.Kind == "dot" || e.Kind == "hot" }
func (e damageEffect) heal() bool     { return e.Kind == "heal" || e.Kind == "hot" }

type damageUnit struct {
	unitSnapshot
	CreatureType uint32 `json:"creatureType"`
}

// damageRecord is one `.simval damage` line. A non-empty Shaped means the server adds something the
// probe doesn't follow, so the record isn't compared.
type damageRecord struct {
	Command string `json:"command"`
	Spell   struct {
		ID   int32  `json:"id"`
		Name string `json:"name"`
	} `json:"spell"`
	Shaped   string     `json:"shaped"`
	Attacker damageUnit `json:"attacker"`
	Target   damageUnit `json:"target"`

	SpellPower struct {
		Damage  float64 `json:"damage"`
		Healing float64 `json:"healing"`
	} `json:"spellPower"`
	AttackPower struct {
		Type  string  `json:"type"`
		Value float64 `json:"value"`
	} `json:"attackPower"`
	Bonus *struct {
		AP    float64 `json:"ap"`
		APDot float64 `json:"apDot"`
	} `json:"bonus"`
	Effects []damageEffect `json:"effects"`
}

func (r damageRecord) label() string {
	return fmt.Sprintf("damage %d (%s): %s vs %s", r.Spell.ID, r.Spell.Name, r.Attacker.Name, r.Target.Name)
}

func readDamageRecords(path string) ([]damageRecord, error) {
	return readJSONLines[damageRecord](path)
}

func readJSONLines[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var records []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var rec T
		if err := json.Unmarshal([]byte(text), &rec); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

// The test's flat spell power and attack power auras. What they give is already in the record's
// spellPower and attackPower, which the replay hands the sim directly.
var damagePowerAuras = map[int32]bool{54192: true, 54161: true, 47436: true}

// Spells the sim only registers with the talent that teaches them; the server probes them without it.
var talentTaughtSpells = map[int32]string{63672: "blackArrow"}

// The server's CreatureType ids.
var mobTypes = map[uint32]proto.MobType{
	1: proto.MobType_MobTypeBeast, 2: proto.MobType_MobTypeDragonkin, 3: proto.MobType_MobTypeDemon,
	4: proto.MobType_MobTypeElemental, 5: proto.MobType_MobTypeGiant, 6: proto.MobType_MobTypeUndead,
	7: proto.MobType_MobTypeHumanoid, 9: proto.MobType_MobTypeMechanical,
}

type talentSlot struct{ tree, index int }

type classTalents struct {
	sizes  []int
	ranks  map[int32]talentSlotRank // rank spell id
	fields map[string]talentSlot
}

type talentSlotRank struct {
	talentSlot
	rank int32
}

// damageData is what the replay reads from the sim's own files: the talent trees and the glyph items.
type damageData struct {
	talents    map[uint8]*classTalents
	glyphItems map[int32]int32 // glyph spell -> glyph item
}

var loadDamageData = sync.OnceValues(func() (*damageData, error) {
	root, err := checkoutRoot()
	if err != nil {
		return nil, err
	}
	data := &damageData{talents: map[uint8]*classTalents{}}
	for classID, name := range azerothcore.ClassNames {
		file, err := readTalentTreeFile(filepath.Join(root, talentTreesDir, name+".json"))
		if err != nil {
			return nil, err
		}
		class := &classTalents{ranks: map[int32]talentSlotRank{}, fields: map[string]talentSlot{}}
		for i, tree := range file {
			class.sizes = append(class.sizes, len(tree.Talents))
			for j, talent := range tree.Talents {
				slot := talentSlot{i, j}
				class.fields[talent.FieldName] = slot
				for r, spell := range rankSpells(talent.SpellIds, talent.MaxPoints) {
					class.ranks[spell] = talentSlotRank{slot, int32(r + 1)}
				}
			}
		}
		data.talents[uint8(classID)] = class
	}

	raw, err := os.ReadFile(filepath.Join(root, simDBJSON))
	if err != nil {
		return nil, err
	}
	var db struct {
		GlyphIds []struct {
			ItemID  int32 `json:"itemId"`
			SpellID int32 `json:"spellId"`
		} `json:"glyphIds"`
	}
	if err := json.Unmarshal(raw, &db); err != nil {
		return nil, fmt.Errorf("%s: %w", simDBJSON, err)
	}
	data.glyphItems = make(map[int32]int32, len(db.GlyphIds))
	for _, glyph := range db.GlyphIds {
		data.glyphItems[glyph.SpellID] = glyph.ItemID
	}
	return data, nil
})

// checkoutRoot is the checkout the process runs in: the repo root for `go run`, a package dir for `go test`.
func checkoutRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, talentTreesDir)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s above the working directory", talentTreesDir)
		}
		dir = parent
	}
}

// damageSetup is the sim character a record asks for: its talents and glyphs, read off the auras the
// test put on, and the rest of its auras.
type damageSetup struct {
	ranks  [][]int32
	glyphs *proto.Glyphs
	modded bool
	// auras that are neither talents, glyphs nor the power auras, for the sim to put on too where it
	// registers one with the same id
	others []int32
}

func damageSetupFor(rec damageRecord) (damageSetup, error) {
	data, err := loadDamageData()
	if err != nil {
		return damageSetup{}, err
	}
	class, ok := classes[rec.Attacker.Class]
	talents := data.talents[rec.Attacker.Class]
	if !ok || talents == nil {
		return damageSetup{}, fmt.Errorf("no sim class for server class %d", rec.Attacker.Class)
	}

	setup := damageSetup{glyphs: &proto.Glyphs{}}
	for _, size := range talents.sizes {
		setup.ranks = append(setup.ranks, make([]int32, size))
	}
	var major, minor []int32
	for _, a := range rec.Attacker.Auras {
		if damagePowerAuras[a.ID] {
			continue
		}
		if talent, ok := talents.ranks[a.ID]; ok {
			rank := &setup.ranks[talent.tree][talent.index]
			*rank = max(*rank, talent.rank)
			setup.modded = true
			continue
		}
		if item, ok := data.glyphItems[a.ID]; ok {
			if isMinorGlyph(class.class, item) {
				minor = append(minor, item)
			} else {
				major = append(major, item)
			}
			setup.modded = true
			continue
		}
		setup.others = append(setup.others, a.ID)
	}
	if len(major) > 3 || len(minor) > 3 {
		return setup, fmt.Errorf("%d major and %d minor glyphs; a character has 3 of each", len(major), len(minor))
	}
	for i, slot := range []*int32{&setup.glyphs.Major1, &setup.glyphs.Major2, &setup.glyphs.Major3} {
		if i < len(major) {
			*slot = major[i]
		}
	}
	for i, slot := range []*int32{&setup.glyphs.Minor1, &setup.glyphs.Minor2, &setup.glyphs.Minor3} {
		if i < len(minor) {
			*slot = minor[i]
		}
	}
	return setup, nil
}

// isMinorGlyph looks the item up in the class's <Class>MinorGlyph enum.
func isMinorGlyph(class proto.Class, item int32) bool {
	name := protoreflect.FullName("proto." + strings.TrimPrefix(class.String(), "Class") + "MinorGlyph")
	enum, err := protoregistry.GlobalTypes.FindEnumByName(name)
	return err == nil && enum.Descriptor().Values().ByNumber(protoreflect.EnumNumber(item)) != nil
}

func (s damageSetup) talentsString() string {
	trees := make([]string, len(s.ranks))
	for i, tree := range s.ranks {
		digits := make([]byte, len(tree))
		for j, rank := range tree {
			digits[j] = byte('0' + rank)
		}
		trees[i] = strings.TrimRight(string(digits), "0")
	}
	return strings.TrimRight(strings.Join(trees, "-"), "-")
}

// withTalent adds a point in the talent that teaches a spell, for a spell the sim registers only with it.
func (s damageSetup) withTalent(class uint8, field string) (damageSetup, bool) {
	data, _ := loadDamageData()
	slot, ok := data.talents[class].fields[field]
	if !ok {
		return s, false
	}
	out := s
	out.ranks = make([][]int32, len(s.ranks))
	for i := range s.ranks {
		out.ranks[i] = slices.Clone(s.ranks[i])
	}
	out.ranks[slot.tree][slot.index] = max(out.ranks[slot.tree][slot.index], 1)
	return out, true
}

// damageSim is the record's character in a sim of its own, against a target built from the dummy.
type damageSim struct {
	rec    damageRecord
	sim    *core.Simulation
	char   *core.Character
	target *core.Unit
	// the sim's auras for the server auras it was asked to put on
	mirrored []*core.Aura
	// those it has no aura for
	others []int32
	ran    bool
}

// simDecl is the sim's declaration of one server effect: Spell.Direct, or a Dot's Tick.
type simDecl struct {
	spell *core.Spell
	dot   *core.Dot
}

func (d simDecl) effect() core.SpellEffect {
	if d.dot != nil {
		return d.dot.Tick
	}
	return d.spell.Direct
}

// damagePlayer is the record's class and race as the spec, with the setup's talents and glyphs and
// nothing else.
func damagePlayer(rec damageRecord, setup damageSetup, spec protoreflect.Name) (*proto.Player, error) {
	class := classes[rec.Attacker.Class]
	race, ok := races[rec.Attacker.Race]
	if !ok {
		return nil, fmt.Errorf("no sim race for server race %d", rec.Attacker.Race)
	}
	player := &proto.Player{
		Name:          "damage",
		Race:          race,
		Class:         class.class,
		Equipment:     &proto.EquipmentSpec{},
		Consumes:      &proto.Consumes{},
		Buffs:         &proto.IndividualBuffs{},
		TalentsString: setup.talentsString(),
		Glyphs:        setup.glyphs,
	}
	return player, setSpec(player, spec)
}

// setSpec gives the player that spec, its options and rotation empty.
func setSpec(player *proto.Player, spec protoreflect.Name) error {
	m := player.ProtoReflect()
	fd := m.Descriptor().Fields().ByName(spec)
	if fd == nil || fd.ContainingOneof() == nil {
		return fmt.Errorf("proto.Player has no spec %q", spec)
	}
	specMsg := m.NewField(fd).Message()
	fillEmpty(specMsg)
	m.Set(fd, protoreflect.ValueOfMessage(specMsg))
	return nil
}

// newDamageSim puts the player in a sim against the record's dummy. Of the server auras in mirror, the
// ones the sim registers by id go on at every start.
func newDamageSim(rec damageRecord, player *proto.Player, mirror []int32) (d *damageSim, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("building the sim character: %v", r)
		}
	}()
	simpkg.RegisterAll()

	var targetStats stats.Stats
	if len(rec.Target.Resistances) > 0 {
		targetStats[stats.Armor] = rec.Target.Resistances[0]
	}
	worldBoss := rec.Target.WorldBoss
	s := core.NewSim(&proto.RaidSimRequest{
		Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}},
		Encounter: &proto.Encounter{Duration: 180, Targets: []*proto.Target{{
			Level:     rec.Target.Level,
			MobType:   mobTypes[rec.Target.CreatureType],
			Stats:     targetStats.ToFloatArray(),
			WorldBoss: &worldBoss,
		}}},
		SimOptions: &proto.SimOptions{RandomSeed: 1, Iterations: 1},
	})

	d = &damageSim{rec: rec, sim: s, char: s.Raid.Parties[0].Players[0].GetCharacter(), target: s.Encounter.TargetUnits[0]}
	for _, id := range mirror {
		if aura := d.char.GetAuraByID(core.ActionID{SpellID: id}); aura != nil {
			d.mirrored = append(d.mirrored, aura)
		} else {
			d.others = append(d.others, id)
		}
	}
	return d, nil
}

// buildDamageSim tries the class's specs until one registers the record's spell, then again with the
// talent that teaches it.
func buildDamageSim(rec damageRecord) (*damageSim, error) {
	setup, err := damageSetupFor(rec)
	if err != nil {
		return nil, err
	}
	setups := []damageSetup{setup}
	if field, ok := talentTaughtSpells[rec.Spell.ID]; ok {
		if taught, ok := setup.withTalent(rec.Attacker.Class, field); ok {
			setups = append(setups, taught)
		}
	}
	class := classes[rec.Attacker.Class]
	specs := append([]protoreflect.Name{class.spec}, class.otherSpecs...)

	for _, setup := range setups {
		for _, spec := range specs {
			player, err := damagePlayer(rec, setup, spec)
			if err != nil {
				return nil, err
			}
			d, err := newDamageSim(rec, player, setup.others)
			if err != nil {
				return nil, err
			}
			for _, e := range rec.Effects {
				if e.probed() && d.declaration(e) != nil {
					return d, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no %v spec registers spell %d", specs, rec.Spell.ID)
}

// declaration finds the sim spell that declares the effect: by the spell's own id, or by the id a
// declaration takes its numbers from. Of several (a poison's per-source copies), the lowest tag.
func (d *damageSim) declaration(e damageEffect) *simDecl {
	var found *simDecl
	consider := func(decl simDecl) {
		effect := decl.effect()
		if effect == (core.SpellEffect{}) || effect.Effect != e.Index {
			return
		}
		if decl.spell.SpellID != d.rec.Spell.ID && effect.FromSpellID != d.rec.Spell.ID {
			return
		}
		if found == nil || decl.spell.Tag < found.spell.Tag {
			found = &decl
		}
	}
	for _, spell := range d.char.Spellbook {
		if !e.periodic() {
			consider(simDecl{spell: spell})
			continue
		}
		victim := d.target
		if e.heal() {
			victim = &d.char.Unit
		}
		for _, dot := range []*core.Dot{dotOn(spell, victim), spell.AOEDot()} {
			if dot != nil {
				consider(simDecl{spell: spell, dot: dot})
			}
		}
	}
	return found
}

// dotOn is the spell's dot or hot on the unit, nil for a spell without per-target ones.
func dotOn(spell *core.Spell, unit *core.Unit) (dot *core.Dot) {
	defer func() {
		if recover() != nil {
			dot = nil
		}
	}()
	return spell.Dot(unit)
}

// start starts the sim over with the mirrored auras on the character.
func (d *damageSim) start() {
	// Reset panics on an aura still up from the last run, so end that run the way an iteration ends
	if d.ran {
		d.sim.Cleanup()
	}
	d.ran = true
	d.sim.Reset()
	for _, aura := range d.mirrored {
		if !aura.IsActive() {
			aura.Activate(d.sim)
		}
	}
}

// prepare starts the sim over with the record's auras, spell power and attack power on the character.
func (d *damageSim) prepare(spellPower float64) {
	d.start()
	unit := &d.char.Unit
	// attack power first: Sheath of Light and Mental Quickness add spell power off it
	setStat(d.sim, unit, stats.AttackPower, d.attackPower("mainhand"))
	setStat(d.sim, unit, stats.RangedAttackPower, d.attackPower("ranged"))
	setStat(d.sim, unit, stats.SpellPower, spellPower)
}

// attackPower is the snapshot's, or the record's own for the attack type the spell's bonus reads,
// which also counts the target's bonus to attackers.
func (d *damageSim) attackPower(attackType string) float64 {
	if d.rec.AttackPower.Type == attackType {
		return d.rec.AttackPower.Value
	}
	return d.rec.Attacker.attack(attackType).AttackPower
}

// setStat brings a stat to the value through the sim's own stat dependencies, which can scale what's
// added (an attack power multiplier).
func setStat(sim *core.Simulation, unit *core.Unit, stat stats.Stat, want float64) {
	scale := 1.0
	for range 4 {
		have := unit.GetStat(stat)
		diff := want - have
		if math.Abs(diff) < 1e-9 {
			return
		}
		unit.AddStatDynamic(sim, stat, diff/scale)
		if moved := unit.GetStat(stat) - have; moved != 0 {
			scale *= moved / diff
		}
	}
}

// Ratings far past any cap, so the forced outcome is a plain hit.
const (
	forcedHitRating  = 200 * core.MeleeHitRatingPerHitChance
	forcedCritRating = 200 * core.CritRatingPerCritChance
	// a binary spell folds its resist chance into the miss roll, which no rating removes
	maxHitAttempts = 50
)

// amount is what the sim deals or heals for the effect at one base, after every multiplier, as a
// plain hit with no resist.
func (d *damageSim) amount(decl simDecl, e damageEffect, base float64) (amount float64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	spellPower := d.rec.SpellPower.Damage
	victim := d.target
	if e.heal() {
		spellPower = d.rec.SpellPower.Healing
		victim = &d.char.Unit
	}
	if decl.dot != nil {
		return d.tick(decl.dot, e.heal(), victim, base, spellPower), nil
	}
	return d.direct(decl.spell, e.heal(), victim, base, spellPower)
}

// tick applies the dot with its tick base pinned, then works out one tick. Ticking with the dot's own
// aura up is how the sim always meets them: Black Arrow divides its own debuff back out.
func (d *damageSim) tick(dot *core.Dot, heal bool, victim *core.Unit, base, spellPower float64) float64 {
	tick, flags := dot.Tick, dot.Spell.Flags
	defer func() { dot.Tick, dot.Spell.Flags = tick, flags }()
	dot.Tick.Min, dot.Tick.Max = base, base
	dot.Spell.Flags |= core.SpellFlagIgnoreResists

	d.prepare(spellPower)
	dot.Apply(d.sim)
	// the probe works the amount out for one stack (Lacerate), which a spell's own cast adds after Apply
	if dot.MaxStacks > 0 && dot.GetStacks() == 0 {
		dot.SetStacks(d.sim, 1)
		dot.TakeSnapshot(d.sim, true)
	}
	var result *core.SpellResult
	if heal {
		result = dot.CalcSnapshotHealing(d.sim, victim, dot.Spell.OutcomeAlwaysHit)
	} else {
		result = dot.CalcSnapshotDamage(d.sim, victim, dot.Spell.OutcomeAlwaysHit)
	}
	amount := result.Damage
	dot.Spell.DisposeResult(result)
	return amount
}

// direct runs the spell's ApplyEffects with Direct pinned. The damage is read when it's calculated,
// through the target's damage taken hook, so a missile's travel time doesn't matter.
func (d *damageSim) direct(spell *core.Spell, heal bool, victim *core.Unit, base, spellPower float64) (float64, error) {
	direct, flags, hooks := spell.Direct, spell.Flags, d.target.DynamicDamageTakenModifiers
	defer func() { spell.Direct, spell.Flags, d.target.DynamicDamageTakenModifiers = direct, flags, hooks }()
	spell.Direct.Min, spell.Direct.Max = base, base
	spell.Flags |= core.SpellFlagIgnoreResists | core.SpellFlagAlwaysHit

	var amount float64
	landed := false
	d.target.DynamicDamageTakenModifiers = append(slices.Clip(hooks), func(_ *core.Simulation, s *core.Spell, result *core.SpellResult) {
		if s == spell && !landed && result.Landed() {
			amount, landed = result.Damage, true
		}
	})
	for range maxHitAttempts {
		d.prepare(spellPower)
		spell.BonusHitRating += forcedHitRating
		spell.BonusCritRating -= forcedCritRating
		spell.ApplyEffects(d.sim, victim, spell)
		if heal {
			amount = spell.SpellMetrics[victim.UnitIndex].TotalHealing
			landed = amount > 0
		}
		if landed {
			return amount, nil
		}
	}
	return 0, fmt.Errorf("no hit in %d tries", maxHitAttempts)
}

// critMultiplier is the sim's for the effect, and false when the sim never crits it.
func (d *damageSim) critMultiplier(decl simDecl) (float64, bool) {
	if decl.dot != nil {
		return decl.dot.Spell.CritMultiplier, decl.dot.TicksCanCrit
	}
	return decl.spell.CritMultiplier, true
}

// serverCanCrit is whether the server ever crits the effect. A tick skips the crit capable test
// (CalcPeriodicCritChance passes skipEffectCheck), so a pure dot only has to not be CANT_CRIT.
func serverCanCrit(spellID int32, e damageEffect) bool {
	if !e.periodic() {
		capable, _ := critCapable(spellID)
		return capable
	}
	spell := serverdata.SpellByID(spellID)
	return spell == nil || spell.Attributes[2]&attr2CantCrit == 0
}

// unmatchedAuras are the sim's active auras the record lacks and the record's the sim has no aura for,
// to read a mismatch by. Read on a fresh start, not with the last probe's dot still up.
func (d *damageSim) unmatchedAuras() (text string) {
	defer func() {
		if r := recover(); r != nil {
			text = fmt.Sprintf("auras unread: %v", r)
		}
	}()
	d.start()
	server := map[int32]bool{}
	for _, a := range d.rec.Attacker.Auras {
		server[a.ID] = true
	}
	var simOnly []string
	for _, aura := range d.char.GetAuras() {
		if id := aura.ActionID.SpellID; aura.IsActive() && id != 0 && !server[id] {
			simOnly = append(simOnly, strconv.Itoa(int(id)))
		}
	}
	others := make([]string, len(d.others))
	for i, id := range d.others {
		others[i] = strconv.Itoa(int(id))
	}
	return fmt.Sprintf("sim-only auras [%s], server auras not in the sim [%s]",
		strings.Join(simOnly, " "), strings.Join(others, " "))
}

// amountTolerance is how far below and above the server's amount the sim may land: 0.1% or 1 point,
// widened only above. SpellDamageBonusDone and SpellHealingBonusDone truncate each power bonus
// (spell power or attack power times its coefficient) before the done multiplier, so each one can cost
// the server up to the multiplier, never add to it.
func amountTolerance(server, multiplier float64, powerBonuses int) (below, above float64) {
	below = max(server*0.001, 1)
	return below, max(below, 1+float64(powerBonuses)*multiplier)
}

// powerBonuses counts the bonuses the server truncated into the effect's amount.
func (r damageRecord) powerBonuses(e damageEffect) int {
	n := 0
	spellPower := r.SpellPower.Damage
	if e.heal() {
		spellPower = r.SpellPower.Healing
	}
	if e.Coefficient > 0 && spellPower > 0 {
		n++
	}
	if r.Bonus != nil && r.AttackPower.Value > 0 && (e.periodic() && r.Bonus.APDot > 0 || !e.periodic() && r.Bonus.AP > 0) {
		n++
	}
	return n
}

// checkDamage compares each probed effect of a record with the sim: the declared base and spell power
// coefficient, the amount at both ends of the roll, and the crit multiplier. A shaped record gives none.
func checkDamage(rec damageRecord) []Check {
	return replayDamage(rec, buildDamageSim)
}

// replayDamage is checkDamage on the sim character build makes for the record.
func replayDamage(rec damageRecord, build func(damageRecord) (*damageSim, error)) []Check {
	if rec.Shaped != "" {
		return nil
	}
	var probed []damageEffect
	for _, e := range rec.Effects {
		if e.probed() {
			probed = append(probed, e)
		}
	}
	if len(probed) == 0 {
		return []Check{fail("effects", "no probed effect in an unshaped record")}
	}
	if !rec.Attacker.IsPlayer || rec.Attacker.Level != core.CharacterLevel {
		return []Check{fail("attacker", fmt.Sprintf("level %d player=%v; the replay builds a level %d player",
			rec.Attacker.Level, rec.Attacker.IsPlayer, core.CharacterLevel))}
	}

	d, err := build(rec)
	if err != nil {
		return []Check{fail("sim character", err.Error())}
	}

	var checks []Check
	for _, e := range probed {
		name := fmt.Sprintf("e%d %s", e.Index, e.Kind)
		decl := d.declaration(e)
		if decl == nil {
			checks = append(checks, fail(name, "no sim declaration for this effect"))
			continue
		}
		effect := decl.effect()

		// a declaration that takes its numbers from another spell (Death Coil's base from the cast) can't
		// match the base the probe reads off the record's spell
		if effect.FromSpellID == 0 || effect.FromSpellID == rec.Spell.ID {
			detail := fmt.Sprintf("sim %.0f-%.0f, server %.0f-%.0f", effect.Min, effect.Max, e.Base.Min, e.Base.Max)
			if effect.Min == e.Base.Min && effect.Max == e.Base.Max {
				checks = append(checks, pass(name+" base", detail))
			} else {
				checks = append(checks, fail(name+" base", detail))
			}
		}
		checks = append(checks, checkNear(name+" coefficient", effect.SP, e.Coefficient, 5e-4))

		lo, errLo := d.amount(*decl, e, e.Base.Min)
		hi, errHi := d.amount(*decl, e, e.Base.Max)
		if err := firstErr(errLo, errHi); err != nil {
			checks = append(checks, fail(name+" amount", err.Error()))
			continue
		}
		multiplier, err := d.multiplier(*decl, e, lo, hi)
		if err != nil {
			checks = append(checks, fail(name+" amount", err.Error()))
			continue
		}
		bonuses := rec.powerBonuses(e)
		for _, end := range []struct {
			label       string
			sim, server float64
		}{{"min", lo, e.Taken.Min}, {"max", hi, e.Taken.Max}} {
			below, above := amountTolerance(end.server, multiplier, bonuses)
			detail := fmt.Sprintf("sim %.2f, server %.0f (-%.2f/+%.2f)", end.sim, end.server, below, above)
			if diff := end.sim - end.server; diff >= -below && diff <= above {
				checks = append(checks, pass(name+" "+end.label, detail))
			} else {
				checks = append(checks, fail(name+" "+end.label, detail+"; "+d.unmatchedAuras()))
			}
		}

		if e.Crit.Amount > 0 && serverCanCrit(rec.Spell.ID, e) {
			if crit, ok := d.critMultiplier(*decl); ok {
				checks = append(checks, checkMultiplier(name+" crit", crit, e.Crit.Result/e.Crit.Amount))
			}
		}
	}
	return checks
}

// multiplier is the sim's own, from how its amount moves with the base.
func (d *damageSim) multiplier(decl simDecl, e damageEffect, lo, hi float64) (float64, error) {
	if e.Base.Max-e.Base.Min >= 1 {
		return (hi - lo) / (e.Base.Max - e.Base.Min), nil
	}
	const step = 1000
	far, err := d.amount(decl, e, e.Base.Min+step)
	return (far - lo) / step, err
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
