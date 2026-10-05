package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

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

func (r damageRange) at(end int) float64 {
	if end == 0 {
		return r.Min
	}
	return r.Max
}

type damageEffect struct {
	Index  int32 `json:"index"`
	Effect int32 `json:"effect"` // the server's SpellEffects id
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

	// a pet's or guardian's owner, when the attacker is one
	Owner *damageUnit `json:"owner"`
	// a weapon strike's own probe; its effects stay unprobed, so Shaped always says there's none
	Weapon *weaponStrike `json:"weapon"`
}

// isPet is whether the attacker is a player's pet or guardian.
func (r damageRecord) isPet() bool {
	return r.Owner != nil && !r.Attacker.IsPlayer
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
	// the record's attacker when that's the character's pet, nil for the character itself
	pet *damagePet
}

// casterChar is the record's attacker in the sim: the character, or its pet.
func (d *damageSim) casterChar() *core.Character {
	if d.pet != nil {
		return &d.pet.pet.Character
	}
	return d.char
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

// newDamageSim puts the player in a sim against `targets` copies of the record's dummy. Of the server
// auras in mirror, the ones the sim registers by spell id go on at every start.
func newDamageSim(rec damageRecord, player *proto.Player, mirror []int32, targets int) (*damageSim, error) {
	return newSimWith(rec, player, mirror, targets, nil)
}

// newSimWith is newDamageSim with raid buffs, for a record taken under one. A buff's aura carries a tag,
// so the mirrored auras are found by spell id alone.
func newSimWith(rec damageRecord, player *proto.Player, mirror []int32, targets int, buffs *proto.RaidBuffs) (d *damageSim, err error) {
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
	dummies := make([]*proto.Target, max(1, targets))
	for i := range dummies {
		dummies[i] = &proto.Target{
			Level:     rec.Target.Level,
			MobType:   mobTypes[rec.Target.CreatureType],
			Stats:     targetStats.ToFloatArray(),
			WorldBoss: &worldBoss,
		}
	}
	s := core.NewSim(&proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: buffs},
		Encounter:  &proto.Encounter{Duration: 180, Targets: dummies},
		SimOptions: &proto.SimOptions{RandomSeed: 1, Iterations: 1},
	})

	d = &damageSim{rec: rec, sim: s, char: s.Raid.Parties[0].Players[0].GetCharacter(), target: s.Encounter.TargetUnits[0]}
	for _, id := range mirror {
		if aura := auraBySpell(d.char.GetAuras(), id); aura != nil {
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
			d, err := newDamageSim(rec, player, setup.others, 1)
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
	caster := d.casterChar()
	for _, spell := range caster.Spellbook {
		if !e.periodic() {
			consider(simDecl{spell: spell})
			continue
		}
		victim := d.target
		if e.heal() {
			victim = &caster.Unit
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
	if d.pet != nil {
		d.pet.start(d)
	}
}

// prepare starts the sim over with the record's auras, spell power and attack power on the attacker.
func (d *damageSim) prepare(spellPower float64) {
	d.start()
	unit := &d.casterChar().Unit
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
		victim = &d.casterChar().Unit
	}
	if decl.dot != nil && !e.periodic() {
		return d.triggeredTick(decl.dot, base, spellPower)
	}
	if decl.dot != nil {
		return d.tick(decl.dot, e.heal(), victim, base, spellPower), nil
	}
	return d.direct(decl.spell, e.heal(), victim, base, spellPower)
}

// probedBeforeOwnAura are dots whose own aura raises their tick, which the probe never sees: it works the tick
// out before the aura is on the target. Serpent Sting's aura adds Noxious Stings to its own ticks.
var probedBeforeOwnAura = map[int32]bool{49001: true}

// tick applies the dot with its tick base pinned, then works out one tick. The dot's own aura stays up, as
// the sim always meets it, except for probedBeforeOwnAura.
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
	if probedBeforeOwnAura[dot.Spell.ActionID.SpellID] {
		dot.Cancel(d.sim)
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
	for _, aura := range d.casterChar().GetAuras() {
		if id := aura.ActionID.SpellID; aura.IsActive() && id != 0 && !server[id] {
			simOnly = append(simOnly, strconv.Itoa(int(id)))
		}
	}
	missing := d.others
	if d.pet != nil {
		missing = d.pet.others
	}
	others := make([]string, len(missing))
	for i, id := range missing {
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
	probed := rec.probedEffects()
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
	return d.compareEffects(probed, d.declaration)
}

func (r damageRecord) probedEffects() []damageEffect {
	var probed []damageEffect
	for _, e := range r.Effects {
		if e.probed() {
			probed = append(probed, e)
		}
	}
	return probed
}

// compareEffects checks each probed effect against the sim declaration declare finds for it.
func (d *damageSim) compareEffects(probed []damageEffect, declare func(damageEffect) *simDecl) []Check {
	rec := d.rec
	var checks []Check
	for _, e := range probed {
		name := fmt.Sprintf("e%d %s", e.Index, e.Kind)
		decl := declare(e)
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

// TestSimvalStrikes records `.simval damage` for weapon strikes, whose `weapon` part the replay compares
// with the sim's strike, and `info` and `damage` for a pet or guardian of each pet class, compared on the
// sim's pet with the owner built from the record's owner. Its death knight's Ghoul Frenzy `timing` goes
// through the timing replay.

const strikesFixture = "strikes.jsonl"

// weaponStrike is Spell::EffectWeaponDmg's generic steps at both ends of the weapon's roll and the
// effects' rolls, then MeleeDamageBonusDone and MeleeDamageBonusTaken. A non-empty Shaped means the server
// adds something it doesn't follow, so it isn't compared.
type weaponStrike struct {
	Index        int32   `json:"index"`
	Shaped       string  `json:"shaped"`
	AttackType   string  `json:"attackType"`
	Physical     bool    `json:"physical"`
	Normalized   bool    `json:"normalized"`
	APMultiplier float64 `json:"apMultiplier"`
	// the wielder's attack power for the hand, without the target's bonus
	AttackPower float64 `json:"attackPower"`
	// the hand's TOTAL_PCT, which a physical strike's fixed bonus takes
	TotalPct     float64     `json:"totalPct"`
	WeaponDamage damageRange `json:"weaponDamage"`
	// the summed fixed bonuses and the weapon percent, with effect spell mods
	Fixed   damageRange `json:"fixed"`
	Percent damageRange `json:"percent"`
	Base    damageRange `json:"base"`
	Done    damageRange `json:"done"`
	Taken   damageRange `json:"taken"`
	Crit    struct {
		Amount float64 `json:"amount"`
		Result float64 `json:"result"`
	} `json:"crit"`
}

// petInfo is an `info` record's account of where a pet's or guardian's stats come from.
type petInfo struct {
	// strength through spirit before any aura
	CreateStats []float64 `json:"createStats"`
	// the PetDefines.h scaling auras on it
	Scaling []struct {
		ID      int32 `json:"id"`
		Effects []struct {
			Aura      int32   `json:"aura"`
			MiscValue int32   `json:"miscValue"`
			Amount    float64 `json:"amount"`
		} `json:"effects"`
	} `json:"scaling"`
	// Creature::Regenerate's amount for one tick
	Regen *struct {
		Power       uint32  `json:"power"`
		IntervalMs  float64 `json:"intervalMs"`
		PerTick     float64 `json:"perTick"`
		LastManaUse bool    `json:"lastManaUse"`
	} `json:"regen"`
}

// strikesRecord is a strikes.jsonl line: a strike's or a pet's `damage`, a pet's `info`, or a `timing`.
type strikesRecord struct {
	damageRecord
	Pet *petInfo `json:"pet"`
	// the same line as a timing record, for a `timing`
	timing *timingRecord
	// for a buffed pet's info, the same pet's last info before the buffs
	bare *strikesRecord
}

func readStrikesRecords(path string) ([]strikesRecord, error) {
	lines, err := readJSONLines[json.RawMessage](path)
	if err != nil {
		return nil, err
	}
	records := make([]strikesRecord, len(lines))
	for i, line := range lines {
		if err := json.Unmarshal(line, &records[i]); err != nil {
			return nil, fmt.Errorf("%s record %d: %w", path, i+1, err)
		}
		if records[i].Command == "timing" {
			records[i].timing = new(timingRecord)
			if err := json.Unmarshal(line, records[i].timing); err != nil {
				return nil, fmt.Errorf("%s record %d: %w", path, i+1, err)
			}
		}
	}
	pairBuffedPetInfos(records)
	return records, nil
}

// pairBuffedPetInfos points each buffed pet info at its owner's last info of that pet without the buffs.
func pairBuffedPetInfos(records []strikesRecord) {
	for i := range records {
		rec := &records[i]
		if !rec.buffedPetInfo() {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			o := &records[j]
			if o.Command == "info" && o.isPet() && !o.buffedPetInfo() && o.Owner.Name == rec.Owner.Name &&
				o.Attacker.Entry == rec.Attacker.Entry {
				rec.bare = o
				break
			}
		}
	}
}

func (r strikesRecord) label() string {
	who := r.Attacker.Name
	if r.isPet() {
		who = fmt.Sprintf("%s's %s (%d)", r.Owner.Name, r.Attacker.Name, r.Attacker.Entry)
	}
	if r.Command == "info" {
		if r.buffedPetInfo() {
			return "buffed info: " + who
		}
		return "info: " + who
	}
	return fmt.Sprintf("%s %d (%s): %s", r.Command, r.Spell.ID, r.Spell.Name, who)
}

// strikesKey names a strikes.jsonl record by what it probed: the command and spell, the player's class (the
// owner's for a pet), the pet's entry (0 for the character's own record), and whether the character carried
// its talents, or for a pet's info its owner's buffs.
type strikesKey struct {
	command string
	spell   int32
	class   uint8
	entry   uint32
	modded  bool
}

func (k strikesKey) String() string {
	pass := "bare"
	if k.modded {
		pass = "with talents"
	}
	if k.entry != 0 {
		if k.modded {
			return fmt.Sprintf("%s spell %d of class %d's pet %d, buffed", k.command, k.spell, k.class, k.entry)
		}
		return fmt.Sprintf("%s spell %d of class %d's pet %d", k.command, k.spell, k.class, k.entry)
	}
	return fmt.Sprintf("%s spell %d class %d, %s", k.command, k.spell, k.class, pass)
}

func (r strikesRecord) key() (strikesKey, error) {
	k := strikesKey{command: r.Command, spell: r.Spell.ID, class: r.Attacker.Class}
	if r.isPet() {
		k.class, k.entry, k.modded = r.Owner.Class, r.Attacker.Entry, r.buffedPetInfo()
		return k, nil
	}
	setup, err := damageSetupFor(r.damageRecord)
	k.modded = setup.modded
	return k, err
}

// checkStrikesRecord compares a strikes.jsonl record with the sim. A shaped strike or spell gives none.
func checkStrikesRecord(rec strikesRecord) []Check {
	switch {
	case rec.Command == "timing" && rec.timing != nil:
		return replayTiming(*rec.timing, buildStrikesTimingSim)
	case rec.buffedPetInfo():
		return checkPetBuffs(rec)
	case rec.Command == "info" && rec.Pet != nil && rec.isPet():
		return checkPetInfo(rec)
	case rec.Command == "damage" && rec.Weapon != nil:
		return checkStrike(rec.damageRecord)
	case rec.Command == "damage" && rec.isPet():
		return checkPetDamage(rec.damageRecord)
	}
	return []Check{fail("record", fmt.Sprintf("no replay for a %q record without a weapon or pet part", rec.Command))}
}

// strikeWeaponEffects are the effects Spell::EffectWeaponDmg deals together: weapon damage without school,
// weapon percent, weapon damage, normalized weapon damage.
var strikeWeaponEffects = map[int32]bool{17: true, 31: true, 58: true, 121: true}

// strikeTaughtSpells are the strikes the sim only registers with the talent that teaches them; the server
// probes them without it.
var strikeTaughtSpells = map[int32]string{
	47486: "mortalStrike", 47498: "devastate",
	35395: "crusaderStrike", 53385: "divineStorm", 20424: "sealOfCommand", 20467: "sealOfCommand",
	48665: "mutilate", 48664: "mutilate", 48660: "hemorrhage", 14278: "ghostlyStrike",
	55262: "heartStrike", 55268: "frostStrike", 55271: "scourgeStrike", 50463: "bloodCakedBlade",
	32175: "stormstrike", 32176: "stormstrike", 60103: "lavaLash",
	49050: "aimedShot", 34490: "silencingShot", 53209: "chimeraShot",
	48566: "mangle", 48564: "mangle",
}

// strikeExtraSpecs are specs beyond the damage replay's that register a strike, by server class id.
var strikeExtraSpecs = map[uint8][]protoreflect.Name{
	1: {"protection_warrior"}, 2: {"protection_paladin"}, 6: {"tank_deathknight"}, 11: {"feral_tank_druid"},
}

// weaponSlots are the sim's slots for the weapons of the snapshot's attack types.
var weaponSlots = map[string]proto.ItemSlot{
	"mainhand": proto.ItemSlot_ItemSlotMainHand,
	"offhand":  proto.ItemSlot_ItemSlotOffHand,
	"ranged":   proto.ItemSlot_ItemSlotRanged,
}

// strikePlayer is damagePlayer wearing the record's weapons.
func strikePlayer(rec damageRecord, setup damageSetup, spec protoreflect.Name) (*proto.Player, error) {
	player, err := damagePlayer(rec, setup, spec)
	if err != nil {
		return nil, err
	}
	items := make([]*proto.ItemSpec, len(serverEquipSlots))
	for i := range items {
		items[i] = &proto.ItemSpec{}
	}
	for _, a := range rec.Attacker.Attacks {
		if slot, ok := weaponSlots[a.Type]; ok && a.Weapon != nil {
			items[slot].Id = a.Weapon.ItemID
		}
	}
	player.Equipment = &proto.EquipmentSpec{Items: items}
	return player, nil
}

// buildStrikeSim builds the record's attacker with the strike: a pet on its owner, else the character in
// the record's weapons, trying the class's specs and then the talent that teaches the strike.
func buildStrikeSim(rec damageRecord) (*damageSim, *core.Spell, error) {
	if rec.isPet() {
		d, err := buildPetSim(rec)
		if err != nil {
			return nil, nil, err
		}
		if spell := d.strikeDeclaration(); spell != nil {
			return d, spell, nil
		}
		return nil, nil, fmt.Errorf("the sim's %s declares no strike %d", d.pet.pet.Name, rec.Spell.ID)
	}

	setup, err := damageSetupFor(rec)
	if err != nil {
		return nil, nil, err
	}
	setups := []damageSetup{setup}
	if field, ok := strikeTaughtSpells[rec.Spell.ID]; ok {
		if taught, ok := setup.withTalent(rec.Attacker.Class, field); ok {
			setups = append(setups, taught)
		}
	}
	class := classes[rec.Attacker.Class]
	specs := slices.Concat([]protoreflect.Name{class.spec}, class.otherSpecs, strikeExtraSpecs[rec.Attacker.Class])
	for _, setup := range setups {
		for _, spec := range specs {
			player, err := strikePlayer(rec, setup, spec)
			if err != nil {
				return nil, nil, err
			}
			d, err := newDamageSim(rec, player, setup.others, 1)
			if err != nil {
				return nil, nil, err
			}
			// a spec that never swings (elemental) leaves its weapons empty, which deal NaN
			spell := d.strikeDeclaration()
			if spell != nil && d.strikeHand().AttackPowerPerDPS > 0 {
				return d, spell, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("no %v spec declares strike %d with a weapon it swings", specs, rec.Spell.ID)
}

// strikeHand is the sim weapon of the record's strike.
func (d *damageSim) strikeHand() *core.Weapon {
	attackType := "mainhand"
	if d.rec.Weapon != nil {
		attackType = d.rec.Weapon.AttackType
	}
	return strikeWeapon(&d.casterChar().Unit, attackType)
}

// strikeDeclaration is the sim spell whose Direct declares the record's strike: on one of its weapon
// effects, by the spell's own id or FromSpellID. Of several (one per hand), the lowest tag.
func (d *damageSim) strikeDeclaration() *core.Spell {
	weapon := map[int32]bool{}
	for _, e := range d.rec.Effects {
		if strikeWeaponEffects[e.Effect] {
			weapon[e.Index] = true
		}
	}
	id := d.rec.Spell.ID
	var found *core.Spell
	for _, spell := range d.casterChar().Spellbook {
		e := spell.Direct
		if e == (core.SpellEffect{}) || !weapon[e.Effect] {
			continue
		}
		if e.FromSpellID != id && (e.FromSpellID != 0 || spell.SpellID != id) {
			continue
		}
		if found == nil || spell.Tag < found.Tag {
			found = spell
		}
	}
	return found
}

// strikeProbeStates put the target in the state the probe's amounts assume. The probe reads Seal of
// Vengeance's 33% as it stands; spell_pal_seal_of_vengeance_aura scales it by Holy Vengeance's stacks, all
// of it at 5.
var strikeProbeStates = map[int32]func(d *damageSim){
	42463: func(d *damageSim) { d.stackDot(31803) },
}

// stackDot puts the spell's dot on the target at its most stacks.
func (d *damageSim) stackDot(id int32) {
	for _, spell := range d.casterChar().Spellbook {
		if spell.SpellID != id {
			continue
		}
		if dot := dotOn(spell, d.target); dot != nil {
			dot.Apply(d.sim)
			dot.SetStacks(d.sim, dot.MaxStacks)
			return
		}
	}
}

// pinWeapons sets each weapon's roll to one end of its range, and returns what puts them back.
func pinWeapons(unit *core.Unit, end int) func() {
	aa := &unit.AutoAttacks
	var restore []func()
	for _, w := range []*core.Weapon{aa.MH(), aa.OH(), aa.Ranged()} {
		lo, hi := w.BaseDamageMin, w.BaseDamageMax
		pin := lo
		if end == 1 {
			pin = hi
		}
		w.BaseDamageMin, w.BaseDamageMax = pin, pin
		restore = append(restore, func() { w.BaseDamageMin, w.BaseDamageMax = lo, hi })
	}
	return func() {
		for _, r := range restore {
			r()
		}
	}
}

func strikeAttackStat(attackType string) stats.Stat {
	if attackType == "ranged" {
		return stats.RangedAttackPower
	}
	return stats.AttackPower
}

func strikeWeapon(unit *core.Unit, attackType string) *core.Weapon {
	switch attackType {
	case "offhand":
		return unit.AutoAttacks.OH()
	case "ranged":
		return unit.AutoAttacks.Ranged()
	}
	return unit.AutoAttacks.MH()
}

// strikeAmount is the sim's strike at one end of the roll, after every multiplier, as a plain hit with no
// armor or resist: each weapon pinned to that end of its range, the declaration to the record's fixed bonus
// and percent at that end, and the attack power the strike reads to the record's. Read when the damage is
// calculated, through the target's damage taken hook.
func (d *damageSim) strikeAmount(spell *core.Spell, w *weaponStrike, end int) (amount float64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("sim panic: %v", r)
		}
	}()
	direct, flags, hooks := spell.Direct, spell.Flags, d.target.DynamicDamageTakenModifiers
	hit, crit := spell.BonusHitRating, spell.BonusCritRating
	defer func() {
		spell.Direct, spell.Flags, d.target.DynamicDamageTakenModifiers = direct, flags, hooks
		spell.BonusHitRating, spell.BonusCritRating = hit, crit
	}()
	spell.Direct.Min, spell.Direct.Max = w.Fixed.at(end), w.Fixed.at(end)
	spell.Direct.WeaponPct = w.Percent.at(end) / 100
	spell.Flags |= core.SpellFlagIgnoreResists | core.SpellFlagAlwaysHit

	landed := false
	d.target.DynamicDamageTakenModifiers = append(slices.Clip(hooks), func(_ *core.Simulation, s *core.Spell, result *core.SpellResult) {
		if s == spell && !landed && result.Landed() {
			amount, landed = result.Damage, true
		}
	})
	unit := &d.casterChar().Unit
	for range maxHitAttempts {
		d.prepare(d.rec.SpellPower.Damage)
		setStat(d.sim, unit, strikeAttackStat(w.AttackType), w.AttackPower)
		if state := strikeProbeStates[d.rec.Spell.ID]; state != nil {
			state(d)
		}
		// after prepare, since its reset puts back the spell's own hit and crit
		spell.BonusHitRating += forcedHitRating
		spell.BonusCritRating -= forcedCritRating
		restore := pinWeapons(unit, end)
		spell.ApplyEffects(d.sim, d.target, spell)
		restore()
		if landed && math.IsNaN(amount) {
			return 0, fmt.Errorf("the sim's strike deals NaN")
		}
		if landed {
			return amount, nil
		}
	}
	return 0, fmt.Errorf("no hit in %d tries", maxHitAttempts)
}

// strikeTolerance is how far below and above the server's amount the sim may land: 0.1% or 1 point, widened
// above for the server's truncations, which only ever lower its number. The weapon damage truncates before
// the percent, the fixed bonus after the hand's percent, then the percent, the done amount and the taken
// one, each losing up to a point at that step.
func strikeTolerance(server, multiplier float64, w *weaponStrike, end int) (below, above float64) {
	pct := w.Percent.at(end) / 100
	steps := pct + 1
	if w.Physical && w.Fixed.at(end) != 0 {
		steps += max(pct, 1)
	}
	if w.Percent.at(end) != 100 {
		steps++
	}
	below = max(server*0.001, 1)
	return below, max(below, 1+steps*multiplier)
}

// checkStrike compares a weapon strike with the sim's: the declared fixed bonus and percent, the amount at
// both ends of the roll, and the crit multiplier. A shaped strike gives none.
func checkStrike(rec damageRecord) []Check {
	w := rec.Weapon
	if w == nil {
		return []Check{fail("weapon", "no weapon part")}
	}
	if w.Shaped != "" {
		return nil
	}
	if !rec.isPet() && (!rec.Attacker.IsPlayer || rec.Attacker.Level != core.CharacterLevel) {
		return []Check{fail("attacker", fmt.Sprintf("level %d player=%v; the replay builds a level %d player",
			rec.Attacker.Level, rec.Attacker.IsPlayer, core.CharacterLevel))}
	}
	d, spell, err := buildStrikeSim(rec)
	if err != nil {
		return []Check{fail("sim strike", err.Error())}
	}

	decl := spell.Direct
	detail := fmt.Sprintf("sim %.0f-%.0f, server %.0f-%.0f", decl.Min, decl.Max, w.Fixed.Min, w.Fixed.Max)
	checks := []Check{fail("fixed", detail)}
	if decl.Min == w.Fixed.Min && decl.Max == w.Fixed.Max {
		checks[0] = pass("fixed", detail)
	}
	checks = append(checks, checkNear("percent", decl.WeaponPct*100, w.Percent.Min, 1e-3))
	if w.Percent.Max != w.Percent.Min {
		checks = append(checks, checkNear("percent max", decl.WeaponPct*100, w.Percent.Max, 1e-3))
	}

	lo, errLo := d.strikeAmount(spell, w, 0)
	hi, errHi := d.strikeAmount(spell, w, 1)
	if err := firstErr(errLo, errHi); err != nil {
		return append(checks, fail("amount", err.Error()))
	}
	multiplier := hi / max(w.Base.Max, 1)
	if w.Base.Max-w.Base.Min >= 1 {
		multiplier = (hi - lo) / (w.Base.Max - w.Base.Min)
	}
	for _, end := range []struct {
		label       string
		i           int
		sim, server float64
	}{{"min", 0, lo, w.Taken.Min}, {"max", 1, hi, w.Taken.Max}} {
		below, above := strikeTolerance(end.server, multiplier, w, end.i)
		detail := fmt.Sprintf("sim %.2f, server %.0f (-%.2f/+%.2f)", end.sim, end.server, below, above)
		if diff := end.sim - end.server; diff >= -below && diff <= above {
			checks = append(checks, pass(end.label, detail))
		} else {
			checks = append(checks, fail(end.label, detail+"; "+d.strikeInputs(w)+"; "+d.unmatchedAuras()))
		}
	}

	if capable, _ := critCapable(rec.Spell.ID); capable && w.Crit.Amount > 0 {
		if spell.CritMultiplier == 0 {
			checks = append(checks, fail("crit", fmt.Sprintf("the sim never crits it, server x%.4f", w.Crit.Result/w.Crit.Amount)))
		} else {
			checks = append(checks, checkMultiplier("crit", spell.CritMultiplier, w.Crit.Result/w.Crit.Amount))
		}
	}
	return checks
}

// strikeInputs puts the server's weapon numbers next to the sim's weapon, to read a mismatch by.
func (d *damageSim) strikeInputs(w *weaponStrike) string {
	weapon := d.strikeHand()
	return fmt.Sprintf("server %s weapon %.0f-%.0f (normalized %v, ap x%.2f, AP %.0f, hand x%.3f), base %.0f-%.0f, done %.0f-%.0f; "+
		"sim weapon %.1f-%.1f, speed %.2f, normalized %.2f", w.AttackType, w.WeaponDamage.Min, w.WeaponDamage.Max,
		w.Normalized, w.APMultiplier, w.AttackPower, w.TotalPct, w.Base.Min, w.Base.Max, w.Done.Min, w.Done.Max,
		weapon.BaseDamageMin, weapon.BaseDamageMax, weapon.SwingSpeed, weapon.NormalizedSwingSpeed)
}

// simPet is the sim's stand-in for a server pet or guardian: the owner's class and spec, what gives the
// sim the pet, and the sim pet's name (the first of that name).
type simPet struct {
	class  uint8
	spec   protoreflect.Name
	name   string
	talent string
	setup  func(*proto.Player)
}

func warlockSummon(summon proto.Warlock_Options_Summon) func(*proto.Player) {
	return func(p *proto.Player) { p.GetWarlock().Options.Summon = summon }
}

// simPets are the sim pets of the server's pet and guardian entries. The tamed wolf keeps its creature
// entry; Force of Nature's talent rank summons Treant 1964.
var simPets = map[uint32]simPet{
	37294: {3, "hunter", "Wolf", "", func(p *proto.Player) {
		o := p.GetHunter().Options
		o.PetType, o.PetUptime = proto.Hunter_Options_Wolf, 1
	}},
	416:   {9, "warlock", "Imp", "", warlockSummon(proto.Warlock_Options_Imp)},
	17252: {9, "warlock", "Felguard", "", warlockSummon(proto.Warlock_Options_Felguard)},
	89:    {9, "warlock", "Infernal", "", nil},
	26125: {6, "deathknight", "Ghoul", "", nil},
	27829: {6, "deathknight", "Gargoyle", "summonGargoyle", nil},
	29264: {7, "enhancement_shaman", "Spirit Wolf 1", "", nil},
	510:   {8, "mage", "Water Elemental", "summonWaterElemental", nil},
	1964:  {11, "balance_druid", "Treant", "forceOfNature", nil},
	19668: {5, "shadow_priest", "Shadowfiend", "", nil},
}

// damagePet is the record's pet in the owner's sim, with the server auras of its own it was asked to put on.
type damagePet struct {
	agent    core.PetAgent
	pet      *core.Pet
	mirrored []*core.Aura
	others   []int32
}

// start brings the owner's stats to its snapshot's, then summons the pet, a guardian only now, so it scales
// off them, and puts its auras on.
func (p *damagePet) start(d *damageSim) {
	d.pinOwner()
	if !p.pet.IsEnabled() {
		p.pet.Enable(d.sim, p.agent)
	}
	for _, aura := range p.mirrored {
		if !aura.IsActive() {
			aura.Activate(d.sim)
		}
	}
}

// buildPetSim builds the record's owner with the pet: its class in the pet's spec, the owner's talents and
// glyphs, and the pet's mirrored auras.
func buildPetSim(rec damageRecord) (d *damageSim, err error) {
	return buildPetSimWith(rec, nil)
}

// buildPetSimWith is buildPetSim in a raid with the buffs, which the sim hands the pet as well
// (applyPetBuffEffects).
func buildPetSimWith(rec damageRecord, buffs *proto.RaidBuffs) (d *damageSim, err error) {
	if !rec.isPet() {
		return nil, fmt.Errorf("%s is no player's pet", rec.Attacker.Name)
	}
	def, ok := simPets[rec.Attacker.Entry]
	if !ok {
		return nil, fmt.Errorf("no sim pet for creature %d (%s)", rec.Attacker.Entry, rec.Attacker.Name)
	}
	if rec.Owner.Class != def.class {
		return nil, fmt.Errorf("creature %d's owner is class %d, the sim's pet is class %d's", rec.Attacker.Entry, rec.Owner.Class, def.class)
	}
	owner := rec
	owner.Attacker = *rec.Owner
	setup, err := damageSetupFor(owner)
	if err != nil {
		return nil, err
	}
	if def.talent != "" {
		if setup, ok = setup.withTalent(def.class, def.talent); !ok {
			return nil, fmt.Errorf("class %d has no talent %s", def.class, def.talent)
		}
	}
	player, err := damagePlayer(owner, setup, def.spec)
	if err != nil {
		return nil, err
	}
	if def.setup != nil {
		def.setup(player)
	}
	if d, err = newSimWith(rec, player, setup.others, 1, buffs); err != nil {
		return nil, err
	}

	for _, agent := range d.char.PetAgents {
		if pet := agent.GetPet(); pet.Name == def.name {
			d.pet = &damagePet{agent: agent, pet: pet}
			break
		}
	}
	if d.pet == nil {
		return nil, fmt.Errorf("the sim's %s has no %s", def.spec, def.name)
	}
	for _, a := range rec.Attacker.Auras {
		if aura := auraBySpell(d.pet.pet.GetAuras(), a.ID); aura != nil {
			d.pet.mirrored = append(d.pet.mirrored, aura)
		} else {
			d.pet.others = append(d.pet.others, a.ID)
		}
	}
	return d, nil
}

// pinOwner brings the owner's stats to its snapshot's, primary stats first, since attack power, armor and
// spell power follow from them.
func (d *damageSim) pinOwner() {
	o, unit := d.rec.Owner, &d.char.Unit
	if o == nil {
		return
	}
	for _, s := range []struct {
		stat  stats.Stat
		value float64
	}{
		{stats.Strength, o.Stats.Strength}, {stats.Agility, o.Stats.Agility}, {stats.Stamina, o.Stats.Stamina},
		{stats.Intellect, o.Stats.Intellect}, {stats.Spirit, o.Stats.Spirit},
	} {
		if s.value > 0 {
			setStat(d.sim, unit, s.stat, s.value)
		}
	}
	if len(o.Resistances) > 0 {
		setStat(d.sim, unit, stats.Armor, o.Resistances[0])
	}
	for _, kind := range []string{"mainhand", "ranged"} {
		if a := o.attack(kind); a.Type != "" {
			setStat(d.sim, unit, strikeAttackStat(kind), a.AttackPower)
		}
	}
	if power := magicSpellPower(o.SpellPower); power > 0 {
		setStat(d.sim, unit, stats.SpellPower, power)
	}
}

// magicSpellPower is a snapshot's highest magic school spell power, the one the sim's single stat stands for.
func magicSpellPower(power map[string]float64) float64 {
	best := 0.0
	for _, school := range magicSchools {
		best = max(best, power[school])
	}
	return best
}

// petDeclaration is declaration, or for a direct damage effect the sim folds into the ticks of the aura that
// triggers it (the Infernal's Immolation), that dot's tick.
func (d *damageSim) petDeclaration(e damageEffect) *simDecl {
	if decl := d.declaration(e); decl != nil || e.periodic() {
		return decl
	}
	var found *simDecl
	for _, spell := range d.casterChar().Spellbook {
		if spell.SpellID != d.rec.Spell.ID {
			continue
		}
		for _, dot := range []*core.Dot{dotOn(spell, d.target), spell.AOEDot()} {
			if dot != nil && dot.Tick != (core.SpellEffect{}) && dot.Tick.Effect == e.Index &&
				(found == nil || spell.Tag < found.spell.Tag) {
				found = &simDecl{spell: spell, dot: dot}
			}
		}
	}
	return found
}

// triggeredTick is one tick of the dot with its base pinned, read through the target's damage taken hook,
// for a dot whose own tick code deals the damage.
func (d *damageSim) triggeredTick(dot *core.Dot, base, spellPower float64) (amount float64, err error) {
	spell := dot.Spell
	tick, flags, hooks := dot.Tick, spell.Flags, d.target.DynamicDamageTakenModifiers
	hit, crit := spell.BonusHitRating, spell.BonusCritRating
	defer func() {
		dot.Tick, spell.Flags, d.target.DynamicDamageTakenModifiers = tick, flags, hooks
		spell.BonusHitRating, spell.BonusCritRating = hit, crit
	}()
	dot.Tick.Min, dot.Tick.Max = base, base
	spell.Flags |= core.SpellFlagIgnoreResists | core.SpellFlagAlwaysHit

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
		dot.Apply(d.sim)
		dot.TickOnce(d.sim)
		if landed {
			return amount, nil
		}
	}
	return 0, fmt.Errorf("no hit in %d tries", maxHitAttempts)
}

// checkPetDamage compares a pet's or guardian's spell like checkDamage does, on the sim's pet with its own
// spell power and attack power pinned to the record's.
func checkPetDamage(rec damageRecord) []Check {
	if rec.Shaped != "" {
		return nil
	}
	probed := rec.probedEffects()
	if len(probed) == 0 {
		return []Check{fail("effects", "no probed effect in an unshaped record")}
	}
	d, err := buildPetSim(rec)
	if err != nil {
		return []Check{fail("sim pet", err.Error())}
	}
	return d.compareEffects(probed, d.petDeclaration)
}

// checkPetInfo compares a pet's stats with the sim's pet scaled off the record's owner: its stats before any
// aura, its stats after them, and its power regen.
func checkPetInfo(rec strikesRecord) (checks []Check) {
	d, err := buildPetSim(rec.damageRecord)
	if err != nil {
		return []Check{fail("sim pet", err.Error())}
	}
	defer func() {
		if r := recover(); r != nil {
			checks = append(checks, fail("sim pet", fmt.Sprintf("panic: %v", r)))
		}
	}()
	d.start()
	pet, server := d.pet.pet, rec.Attacker

	primary := []stats.Stat{stats.Strength, stats.Agility, stats.Stamina, stats.Intellect, stats.Spirit}
	base := pet.GetBaseStats()
	if len(rec.Pet.CreateStats) == len(primary) {
		for i, stat := range primary {
			checks = append(checks, checkNear("base "+stat.StatName(), base[stat], rec.Pet.CreateStats[i], 0.5))
		}
	} else {
		checks = append(checks, fail("base stats", fmt.Sprintf("%d create stats", len(rec.Pet.CreateStats))))
	}

	s := pet.GetStats()
	for i, value := range []float64{server.Stats.Strength, server.Stats.Agility, server.Stats.Stamina,
		server.Stats.Intellect, server.Stats.Spirit} {
		checks = append(checks, checkPetStat(primary[i].StatName(), s[primary[i]], value))
	}
	if a := server.attack("mainhand"); a.Type != "" {
		checks = append(checks, checkPetStat("attack power", s[stats.AttackPower], a.AttackPower))
	}
	checks = append(checks, checkPetStat("spell power", s[stats.SpellPower], magicSpellPower(server.SpellPower)))
	if len(server.Resistances) > 0 {
		checks = append(checks, checkPetStat("armor", s[stats.Armor], server.Resistances[0]))
	}
	checks = append(checks, d.petRegen(rec)...)

	var inputs string
	for i, c := range checks {
		if c.Passed {
			continue
		}
		if inputs == "" {
			inputs = rec.scalingSummary() + "; " + d.unmatchedAuras()
		}
		checks[i].Detail += "; " + inputs
	}
	return checks
}

// petBuff is a buff a pet's owner casts for the pet's buffed info: its aura, and the raid buff giving it in
// the sim.
type petBuff struct {
	aura int32
	give func(*proto.RaidBuffs)
}

// strikesPetBuffs are TestSimvalStrikes' buffs per pet entry: area auras and raid-wide casts, which target
// a controllable guardian like a party member. The castable ones a pet summoned mid-fight misses stay off
// (applyPetBuffEffects).
var strikesPetBuffs = map[uint32][]petBuff{
	89: {
		{58646, func(b *proto.RaidBuffs) { b.StrengthOfEarthTotem = proto.TristateEffect_TristateEffectRegular }},
		{48942, func(b *proto.RaidBuffs) { b.DevotionAura = proto.TristateEffect_TristateEffectRegular }},
		{58655, func(b *proto.RaidBuffs) { b.FlametongueTotem = true }},
		{47436, func(b *proto.RaidBuffs) { b.BattleShout = proto.TristateEffect_TristateEffectRegular }},
	},
}

// buffedPetInfo is whether the record is a pet's info taken with every buff strikesPetBuffs lists for it on
// its owner.
func (r strikesRecord) buffedPetInfo() bool {
	buffs := strikesPetBuffs[r.Attacker.Entry]
	return r.Command == "info" && r.Pet != nil && r.isPet() && len(buffs) > 0 &&
		!slices.ContainsFunc(buffs, func(b petBuff) bool { return !hasAuraID(r.Owner.Auras, b.aura) })
}

func hasAuraID(auras []aura, id int32) bool {
	return slices.ContainsFunc(auras, func(a aura) bool { return a.ID == id })
}

// checkPetBuffs holds what the owner's buffs added to a pet, from its bare info to its buffed one, to what
// the same raid buffs add to the sim's pet, so the pet models' own differences stay out. Both sim pets scale
// off the bare record's owner: the Infernal's scaling auras take the owner's stats once, at the summon
// (spell_warl_infernal_scaling), so the owner's own buffs never reach it that way.
func checkPetBuffs(rec strikesRecord) (checks []Check) {
	if rec.bare == nil {
		return []Check{fail("bare info", "no info of the pet before its owner's buffs")}
	}
	buffs := &proto.RaidBuffs{}
	for _, b := range strikesPetBuffs[rec.Attacker.Entry] {
		b.give(buffs)
	}
	defer func() {
		if r := recover(); r != nil {
			checks = append(checks, fail("sim pet", fmt.Sprintf("panic: %v", r)))
		}
	}()
	before, err := petSimStats(rec.bare.damageRecord, nil)
	if err != nil {
		return []Check{fail("sim pet", err.Error())}
	}
	after, err := petSimStats(rec.bare.damageRecord, buffs)
	if err != nil {
		return []Check{fail("sim pet", err.Error())}
	}
	serverBefore, serverAfter := petServerStats(rec.bare.Attacker), petServerStats(rec.Attacker)
	for _, name := range petStatNames {
		sb, ok1 := serverBefore[name]
		sa, ok2 := serverAfter[name]
		if !ok1 || !ok2 {
			continue
		}
		sim, server := after[name]-before[name], sa-sb
		detail := fmt.Sprintf("sim %+.2f, server %+.0f", sim, server)
		// each side's numbers are truncated on the server, a point each
		if math.Abs(sim-server) <= 2 {
			checks = append(checks, pass(name+" from buffs", detail))
		} else {
			checks = append(checks, fail(name+" from buffs", detail))
		}
	}
	return checks
}

// petStatNames are the pet stats checkPetInfo compares, by its check names.
var petStatNames = []string{"Strength", "Agility", "Stamina", "Intellect", "Spirit", "attack power", "spell power", "armor"}

// petSimStats are the sim pet's petStatNames after a start, with the owner pinned to the record's.
func petSimStats(rec damageRecord, buffs *proto.RaidBuffs) (map[string]float64, error) {
	d, err := buildPetSimWith(rec, buffs)
	if err != nil {
		return nil, err
	}
	d.start()
	s := d.pet.pet.GetStats()
	return map[string]float64{
		"Strength": s[stats.Strength], "Agility": s[stats.Agility], "Stamina": s[stats.Stamina],
		"Intellect": s[stats.Intellect], "Spirit": s[stats.Spirit], "attack power": s[stats.AttackPower],
		"spell power": s[stats.SpellPower], "armor": s[stats.Armor],
	}, nil
}

// petServerStats are a pet snapshot's petStatNames, without attack power or armor when it has none.
func petServerStats(u damageUnit) map[string]float64 {
	out := map[string]float64{
		"Strength": u.Stats.Strength, "Agility": u.Stats.Agility, "Stamina": u.Stats.Stamina,
		"Intellect": u.Stats.Intellect, "Spirit": u.Stats.Spirit, "spell power": magicSpellPower(u.SpellPower),
	}
	if a := u.attack("mainhand"); a.Type != "" {
		out["attack power"] = a.AttackPower
	}
	if len(u.Resistances) > 0 {
		out["armor"] = u.Resistances[0]
	}
	return out
}

// checkPetStat holds a pet stat to the server's: each scaling aura's amount is a truncated whole number, so
// the sim may sit a point off, or 0.1%.
func checkPetStat(name string, sim, server float64) Check {
	detail := fmt.Sprintf("sim %.2f, server %.2f", sim, server)
	if math.Abs(sim-server) <= 1+0.001*math.Abs(server) {
		return pass(name, detail)
	}
	return fail(name, detail)
}

// scalingSummary lists the record's scaling auras as id: aura/misc value=amount.
func (r strikesRecord) scalingSummary() string {
	var parts []string
	for _, s := range r.Pet.Scaling {
		var effects []string
		for _, e := range s.Effects {
			effects = append(effects, fmt.Sprintf("%d/%d=%.0f", e.Aura, e.MiscValue, e.Amount))
		}
		parts = append(parts, fmt.Sprintf("%d: %s", s.ID, strings.Join(effects, " ")))
	}
	return "server scaling [" + strings.Join(parts, ", ") + "]"
}

// Server power types (Powers).
const (
	powerMana   = 0
	powerFocus  = 2
	powerEnergy = 3
)

// petRegen compares one regen tick of the pet's power: focus at the sim's rate, mana at the record's
// intellect and spirit with the five second rule as the server had it, energy at the sim's tick rate.
func (d *damageSim) petRegen(rec strikesRecord) []Check {
	regen, unit := rec.Pet.Regen, &d.pet.pet.Unit
	if regen == nil {
		return nil
	}
	perSecond := 0.0
	switch {
	case regen.Power == powerFocus && unit.HasFocusBar():
		var ok bool
		if perSecond, ok = focusPerSecond(unit); !ok {
			return []Check{fail("focus regen", "core's focus bar has no focusPerTick to read")}
		}
	case regen.Power == powerMana && unit.HasManaBar():
		if st := rec.Attacker.Stats; st.Intellect > 0 && st.Spirit > 0 {
			setStat(d.sim, unit, stats.Intellect, st.Intellect)
			setStat(d.sim, unit, stats.Spirit, st.Spirit)
		}
		perSecond = unit.ManaRegenPerSecondWhileNotCasting()
		if regen.LastManaUse {
			perSecond = unit.ManaRegenPerSecondWhileCasting()
		}
	case regen.Power == powerEnergy && unit.HasEnergyBar():
		perSecond = core.EnergyPerTick * unit.EnergyTickMultiplier * float64(time.Second/core.EnergyTickDuration)
	default:
		return []Check{notCompared("regen", fmt.Sprintf("power %d, which the sim pet has no bar for", regen.Power))}
	}
	// the server truncates each tick to a whole amount
	return []Check{checkNear("regen per tick", perSecond*regen.IntervalMs/1000, regen.PerTick, 1)}
}

// focusPerSecond is the unit's focus regen. Core keeps it unexported: focusBar.focusPerTick, one tick a
// second.
func focusPerSecond(unit *core.Unit) (float64, bool) {
	bar := reflect.ValueOf(unit).Elem().FieldByName("focusBar")
	if !bar.IsValid() {
		return 0, false
	}
	perTick := bar.FieldByName("focusPerTick")
	if !perTick.IsValid() || perTick.Kind() != reflect.Float64 {
		return 0, false
	}
	return perTick.Float(), true
}

// strikesTimingTaught are the spells of strikes.jsonl's timing records the sim only registers with a talent.
var strikesTimingTaught = map[int32]string{63560: "ghoulFrenzy"}

// buildStrikesTimingSim is buildTimingSim with strikesTimingTaught's talents.
func buildStrikesTimingSim(rec timingRecord) (*timingSim, error) {
	dr := rec.damageRecord()
	setup, err := damageSetupFor(dr)
	if err != nil {
		return nil, err
	}
	if field, ok := strikesTimingTaught[rec.Spell.ID]; ok {
		if setup, ok = setup.withTalent(rec.Attacker.Class, field); !ok {
			return nil, fmt.Errorf("class %d has no talent %s", rec.Attacker.Class, field)
		}
	}
	class := classes[rec.Attacker.Class]
	specs := append([]protoreflect.Name{class.spec}, class.otherSpecs...)
	for _, spec := range specs {
		player, err := damagePlayer(dr, setup, spec)
		if err != nil {
			return nil, err
		}
		d, err := newTimingSim(dr, player, setup.others)
		if err != nil {
			return nil, err
		}
		if spell := spellBySpell(&d.char.Unit, rec.Spell.ID); spell != nil {
			return &timingSim{damageSim: d, spell: spell}, nil
		}
	}
	return nil, fmt.Errorf("no %v spec registers spell %d", specs, rec.Spell.ID)
}
