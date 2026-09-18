package main

import (
	"fmt"
	"math"
	"slices"

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The server's class and race ids. Each class is built as one of its specs, named by the Player.spec
// field.
var (
	classes = map[uint8]struct {
		class proto.Class
		spec  protoreflect.Name
	}{
		1:  {proto.Class_ClassWarrior, "warrior"},
		2:  {proto.Class_ClassPaladin, "retribution_paladin"},
		3:  {proto.Class_ClassHunter, "hunter"},
		4:  {proto.Class_ClassRogue, "rogue"},
		5:  {proto.Class_ClassPriest, "shadow_priest"},
		6:  {proto.Class_ClassDeathknight, "deathknight"},
		7:  {proto.Class_ClassShaman, "elemental_shaman"},
		8:  {proto.Class_ClassMage, "mage"},
		9:  {proto.Class_ClassWarlock, "warlock"},
		11: {proto.Class_ClassDruid, "balance_druid"},
	}
	races = map[uint8]proto.Race{
		1:  proto.Race_RaceHuman,
		2:  proto.Race_RaceOrc,
		3:  proto.Race_RaceDwarf,
		4:  proto.Race_RaceNightElf,
		5:  proto.Race_RaceUndead,
		6:  proto.Race_RaceTauren,
		7:  proto.Race_RaceGnome,
		8:  proto.Race_RaceTroll,
		10: proto.Race_RaceBloodElf,
		11: proto.Race_RaceDraenei,
	}
)

// nakedCharacter is the sim's character of that race and class with no gear, talents or buffs, and
// the given ratings as bonus stats.
func nakedCharacter(race proto.Race, classID uint8, ratings stats.Stats) (character *core.Character, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("building the sim character: %v", r)
		}
	}()
	c, ok := classes[classID]
	if !ok {
		return nil, fmt.Errorf("no sim class for server class %d", classID)
	}
	sim.RegisterAll()

	player := &proto.Player{
		Name:       "naked",
		Race:       race,
		Class:      c.class,
		Equipment:  &proto.EquipmentSpec{},
		Consumes:   &proto.Consumes{},
		Buffs:      &proto.IndividualBuffs{},
		BonusStats: &proto.UnitStats{Stats: ratings.ToFloatArray()},
	}
	m := player.ProtoReflect()
	fd := m.Descriptor().Fields().ByName(c.spec)
	spec := m.NewField(fd).Message()
	fillEmpty(spec)
	m.Set(fd, protoreflect.ValueOfMessage(spec))

	env, _, _ := core.NewEnvironment(&proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}},
		&proto.Encounter{}, false)
	return env.Raid.Parties[0].Players[0].GetCharacter(), nil
}

// fillEmpty sets every unset message field to an empty message, since spec constructors read their
// options and rotation without checking for nil.
func fillEmpty(m protoreflect.Message) {
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Kind() == protoreflect.MessageKind && !fd.IsList() && !fd.IsMap() && !m.Has(fd) {
			m.Set(fd, m.NewField(fd))
		}
	}
}

// checkInfo compares a naked player's character sheet with the sim's base stats, stat scaling and
// avoidance, and every player's armor penetration rating with the sim's conversion.
func checkInfo(rec record) []Check {
	unit := rec.Attacker
	if !unit.IsPlayer || unit.Sheet == nil {
		return nil
	}
	class, ok := classes[unit.Class]
	if !ok {
		return []Check{fail("class", fmt.Sprintf("no sim class for server class %d", unit.Class))}
	}

	var checks []Check
	armorPen, armorPenPct := unit.rating("armorPenetration")
	if armorPen > 0 {
		got := armorPen / core.RatingPerPercent(class.class, core.CRArmorPenetration)
		checks = append(checks, checkNear("armor pen %", got, armorPenPct, 1e-4))
	}
	if !unit.naked() || armorPen > 0 {
		return checks
	}

	race, ok := races[unit.Race]
	if !ok {
		return append(checks, fail("race", fmt.Sprintf("no sim race for server race %d", unit.Race)))
	}
	// Only ratings from auras; a death knight's parry rating comes from strength, which the sim already
	// gives it.
	var ratings stats.Stats
	ratings[stats.Defense], _ = unit.rating("defenseSkill")
	ratings[stats.Dodge], _ = unit.rating("dodge")
	character, err := nakedCharacter(race, unit.Class, ratings)
	if err != nil {
		return append(checks, fail("sim character", err.Error()))
	}
	s := character.GetStats()

	// The server truncates primary stats, health and armor to integers. The sim doesn't, so what it
	// derives from a stat can be off by the dropped fraction's worth.
	whole := func(name string, server, sim float64) Check {
		return checkNear(name, math.Floor(sim+1e-6), server, 0)
	}
	truncation := func(stat stats.Stat, pctPerPoint float64) float64 {
		return 1e-3 + (s[stat]-math.Floor(s[stat]+1e-6))*pctPerPoint
	}
	scaling := core.ClassStatScaling[class.class]
	critPerAgility := 100 * scaling.MeleeCritPerAgility
	checks = append(checks,
		whole("strength", unit.Stats.Strength, s[stats.Strength]),
		whole("agility", unit.Stats.Agility, s[stats.Agility]),
		whole("stamina", unit.Stats.Stamina, s[stats.Stamina]),
		whole("intellect", unit.Stats.Intellect, s[stats.Intellect]),
		whole("spirit", unit.Stats.Spirit, s[stats.Spirit]),
		whole("max health", unit.MaxHealth, s[stats.Health]),
		whole("armor", unit.Resistances[0], s[stats.Armor]),
		whole("attack power", unit.attack("mainhand").AttackPower, s[stats.AttackPower]),
		whole("ranged attack power", unit.attack("ranged").AttackPower, s[stats.RangedAttackPower]),
		checkNear("melee crit %", s[stats.MeleeCrit]/core.CritRatingPerCritChance, unit.Sheet.CritMainhand,
			truncation(stats.Agility, critPerAgility)),
		checkNear("spell crit %", s[stats.SpellCrit]/core.CritRatingPerCritChance, unit.Sheet.SpellCrit[1],
			truncation(stats.Intellect, 100*core.SpellCritPerIntellect)),
		checkNear("dodge %", character.DodgeChance()*100, unit.Sheet.RealDodge,
			truncation(stats.Agility, critPerAgility*scaling.CritToDodge)),
		checkNear("miss taken %", 5+(character.DefenseMissChance()+character.PseudoStats.ReducedPhysicalHitTakenChance)*100,
			unit.attack("mainhand").MissChanceTaken, 1e-3),
	)
	// The sheet's parry is before diminishing returns, which is only the same thing without any.
	if parryRating, _ := unit.rating("parry"); ratings[stats.Defense] == 0 && parryRating == 0 {
		checks = append(checks, checkNear("parry %", character.ParryChance()*100, unit.Sheet.Parry, 1e-3))
	}
	// The snapshot has current mana, which is only the maximum until a buff raises it.
	if unit.PowerType == 0 && !slices.ContainsFunc(unit.Auras, func(a aura) bool { return a.DurationMs > 0 }) {
		checks = append(checks, whole("mana", unit.Power, s[stats.Mana]))
	}
	return checks
}

func checkNear(name string, got, want, tolerance float64) Check {
	detail := fmt.Sprintf("sim %.4f, server %.4f", got, want)
	if math.Abs(got-want) <= tolerance {
		return pass(name, detail)
	}
	return fail(name, detail)
}
