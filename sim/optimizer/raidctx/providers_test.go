package raidctx

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/wowsims/wotlk/sim/core/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	coreBuff = "the class's AddRaidBuffs, through GetRaidBuffs"
	blessing = "a blessing the raid UI writes into each player's IndividualBuffs, which carry over"
	noTarget = "single target, and no spec option says whose"
)

// elsewhere accounts for the RAID_STATS_OPTIONS player entries no provider row mirrors.
var elsewhere = map[string]string{
	"Buffs/Stats/Improved Gift of the Wild":            coreBuff,
	"Buffs/Stats/Gift of the Wild":                     coreBuff,
	"Buffs/Stats %/Blessing of Kings":                  blessing,
	"Buffs/Stats %/Blessing of Sanctuary":              blessing,
	"Buffs/Armor/Improved Devotion Aura":               coreBuff,
	"Buffs/Armor/Devotion Aura":                        coreBuff,
	"Buffs/Armor/Improved Stoneskin Totem":             coreBuff,
	"Buffs/Armor/Stoneskin Totem":                      coreBuff,
	"Buffs/Stamina/Improved Power Word Fortitude":      coreBuff,
	"Buffs/Stamina/Power Word Fortitude":               coreBuff,
	"Buffs/Str + Agi/Improved Strength of Earth Totem": coreBuff,
	"Buffs/Str + Agi/Strength of Earth Totem":          coreBuff,
	"Buffs/Str + Agi/Horn of Winter":                   coreBuff,
	"Buffs/Intellect/Arcane Brilliance":                coreBuff,
	"Buffs/Intellect/Improved Fel Intelligence":        coreBuff,
	"Buffs/Intellect/Fel Intelligence":                 coreBuff,
	"Buffs/Spirit/Divine Spirit":                       coreBuff,
	"Buffs/Spirit/Improved Fel Intelligence":           coreBuff,
	"Buffs/Spirit/Fel Intelligence":                    coreBuff,
	"Buffs/Atk Pwr/Improved Blessing of Might":         blessing,
	"Buffs/Atk Pwr/Blessing of Might":                  blessing,
	"Buffs/Atk Pwr %/Abomination's Might":              coreBuff,
	"Buffs/Atk Pwr %/Unleashed Rage":                   coreBuff,
	"Buffs/Atk Pwr %/Trueshot Aura":                    coreBuff,
	"Buffs/Damage %/Sanctified Retribution":            coreBuff,
	"Buffs/Damage %/Arcane Empowerment":                coreBuff,
	"Buffs/Damage %/Ferocious Inspiration":             coreBuff,
	"Buffs/Mit %/Blessing Of Sanctuary":                blessing,
	"Buffs/Mit %/Vigilance":                            noTarget,
	"Buffs/Haste %/Swift Retribution":                  coreBuff,
	"Buffs/Haste %/Improved Moonkin Form":              coreBuff,
	"Buffs/MP5/Improved Blessing of Wisdom":            blessing,
	"Buffs/MP5/Blessing of Wisdom":                     blessing,
	"Buffs/MP5/Improved Mana Spring Totem":             coreBuff,
	"Buffs/MP5/Mana Spring Totem":                      coreBuff,
	"Buffs/Melee Crit/Leader of the Pack":              coreBuff,
	"Buffs/Melee Crit/Rampage":                         coreBuff,
	"Buffs/Melee Haste/Improved Icy Talons":            coreBuff,
	"Buffs/Melee Haste/Improved Windfury Totem":        coreBuff,
	"Buffs/Melee Haste/Windfury Totem":                 coreBuff,
	"Buffs/Spell Power/Totem of Wrath":                 coreBuff,
	"Buffs/Spell Power/Flametongue Totem":              coreBuff,
	"Buffs/Spell Crit/Moonkin Form":                    coreBuff,
	"Buffs/Spell Crit/Elemental Oath":                  coreBuff,
	"Buffs/Spell Haste/Wrath of Air Totem":             coreBuff,
	"Buffs/Health/Improved Imp":                        coreBuff,
	"Buffs/Health/Blood Pact":                          coreBuff,
	"External Buffs/Pain Suppression/Pain Suppression": noTarget,
}

// uiProvider is a RAID_STATS_OPTIONS effect a player gives, e.g.
// playerClassAndTalent(Class.ClassWarrior, 'bloodFrenzy').
type uiProvider struct {
	key    string
	helper string
	// Class.X or Spec.X, without the prefix
	class, spec string
	talent      string
}

var (
	labelLine  = regexp.MustCompile(`^\s*label: '((?:[^'\\]|\\.)*)',`)
	playerLine = regexp.MustCompile(`^\s*playerData: (?:(\w+)\((?:Class\.(\w+)|Spec\.(\w+))(?:, '(\w+)')?|\{)`)
)

// readRaidStats lists the player entries of ui/raid/raid_stats.ts, keyed section/category/label.
// Entries that only read the raid's own buffs (raidData) carry over with base's raid buffs.
func readRaidStats(t *testing.T) []uiProvider {
	t.Helper()
	f, err := os.Open("../../../ui/raid/raid_stats.ts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var entries []uiProvider
	var section, category, label string
	pending := false
	started := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !started {
			started = strings.HasPrefix(line, "const RAID_STATS_OPTIONS")
			continue
		}
		if m := labelLine.FindStringSubmatch(line); m != nil {
			label = strings.ReplaceAll(m[1], `\'`, `'`)
			pending = true
			continue
		}
		trimmed := strings.TrimSpace(line)
		if pending && trimmed != "" {
			pending = false
			switch {
			case strings.HasPrefix(trimmed, "categories:"):
				section = label
				continue
			case strings.HasPrefix(trimmed, "effects:"):
				category = label
				continue
			}
		}
		m := playerLine.FindStringSubmatch(line)
		if m == nil || m[1] == "" {
			// roles count players, they give nothing
			continue
		}
		entries = append(entries, uiProvider{
			key:    section + "/" + category + "/" + label,
			helper: m[1],
			class:  m[2],
			spec:   m[3],
			talent: m[4],
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(entries) < 50 {
		t.Fatalf("read only %d entries from raid_stats.ts; did its layout change?", len(entries))
	}
	return entries
}

func snakeCase(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsUpper(r) {
			b.WriteByte('_')
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestProvidersMirrorRaidStats fails on a RAID_STATS_OPTIONS provider that neither a row mirrors
// nor elsewhere accounts for, and on a row whose class, spec or talent drifted from the UI's.
func TestProvidersMirrorRaidStats(t *testing.T) {
	rows := make(map[string]*provider)
	for _, row := range append(slices.Clone(providers), demonicPact) {
		if rows[row.ui] != nil {
			t.Errorf("two rows mirror %s", row.ui)
		}
		rows[row.ui] = &row
	}

	seen := make(map[string]bool)
	for _, e := range readRaidStats(t) {
		seen[e.key] = true
		row := rows[e.key]
		if row == nil {
			if elsewhere[e.key] == "" {
				t.Errorf("raid_stats.ts gives %s through %s, but no provider row mirrors it: add one, or say in elsewhere why none is needed", e.key, e.helper)
			}
			continue
		}
		if elsewhere[e.key] != "" {
			t.Errorf("%s has a row and an elsewhere entry", e.key)
		}
		if e.class != "" && row.class.String() != e.class {
			t.Errorf("%s: row class %s, UI %s", e.key, row.class, e.class)
		}
		if e.spec != "" {
			spec := proto.Spec(proto.Spec_value[e.spec])
			if !slices.Equal(row.specs, []proto.Spec{spec}) || row.class != specClass[spec] {
				t.Errorf("%s: row class %s and specs %v, UI %s", e.key, row.class, row.specs, e.spec)
			}
		}
		if want := snakeCase(e.talent); row.talent != want {
			t.Errorf("%s: row talent %q, UI %q", e.key, row.talent, want)
		}
		if missing := strings.Contains(e.helper, "Missing"); row.missing != missing {
			t.Errorf("%s: row missing = %v, UI helper %s", e.key, row.missing, e.helper)
		}
	}
	for key := range rows {
		if !seen[key] {
			t.Errorf("row %s mirrors nothing in raid_stats.ts", key)
		}
	}
	for key := range elsewhere {
		if !seen[key] {
			t.Errorf("elsewhere lists %s, which raid_stats.ts doesn't have", key)
		}
	}
}

// TestTargetedOptionsHaveRows catches a new spec-option UnitReference, i.e. a new targeted buff,
// that no row reads.
func TestTargetedOptionsHaveRows(t *testing.T) {
	unitReference := (&proto.UnitReference{}).ProtoReflect().Descriptor().FullName()
	inProtos := make(map[proto.Class][]protoreflect.Name)
	for spec, class := range specClass {
		options := specOptions(newPlayer("", spec, nil))
		fields := options.Descriptor().Fields()
		for i := 0; i < fields.Len(); i++ {
			fd := fields.Get(i)
			if fd.Message() == nil || fd.Message().FullName() != unitReference {
				continue
			}
			inProtos[class] = append(inProtos[class], fd.Name())

			// specOptionRef has to read it back
			p := newPlayer("", spec, nil)
			ref := playerRef(3)
			specOptions(p).Set(fd, protoreflect.ValueOfMessage(ref.ProtoReflect()))
			if specOptionRef(p, fd.Name()) != ref {
				t.Errorf("%s: specOptionRef can't read %s", spec, fd.Name())
			}
		}
	}

	for class, names := range inProtos {
		for _, name := range names {
			if !slices.ContainsFunc(providers, func(p provider) bool { return p.class == class && p.target == name }) {
				t.Errorf("%s spec options have %s, but no targeted row reads it", class, name)
			}
		}
	}
	for _, row := range providers {
		if row.target != "" && !slices.Contains(inProtos[row.class], row.target) {
			t.Errorf("%s: no %s spec has a %s option", row.ui, row.class, row.target)
		}
	}
}

func TestProviderTalentsExist(t *testing.T) {
	for _, row := range append(slices.Clone(providers), demonicPact) {
		if row.talent == "" {
			continue
		}
		talents := talentTrees[row.class].new()
		if talents.Descriptor().Fields().ByName(protoreflect.Name(row.talent)) == nil {
			t.Errorf("%s: %s talents have no %s", row.ui, row.class, row.talent)
		}
		for _, spec := range row.specs {
			if specClass[spec] != row.class {
				t.Errorf("%s: spec %s isn't a %s", row.ui, spec, row.class)
			}
		}
	}
}

// TestProviderConditions checks the rows' conditions beyond class and talent, each with a raider
// next to a plain warrior target.
func TestProviderConditions(t *testing.T) {
	hunter := func(pet proto.Hunter_Options_PetType) *proto.Player {
		p := newPlayer("Hunter", proto.Spec_SpecHunter, nil)
		p.GetHunter().Options.PetType = pet
		return p
	}
	warrior := func(spec proto.Spec, shout proto.WarriorShout, talents map[string]int32) *proto.Player {
		p := newPlayer("Warrior", spec, talents)
		if spec == proto.Spec_SpecWarrior {
			p.GetWarrior().Options.Shout = shout
		} else {
			p.GetProtectionWarrior().Options.Shout = shout
		}
		return p
	}
	shaman := func(fire proto.FireTotem, talents map[string]int32) *proto.Player {
		p := newPlayer("Shaman", proto.Spec_SpecElementalShaman, talents)
		p.GetElementalShaman().Options.Totems = &proto.ShamanTotems{Fire: fire}
		return p
	}

	for _, c := range []struct {
		name     string
		provider *proto.Player
		want     func(*proto.Raid) bool
	}{
		{"worm", hunter(proto.Hunter_Options_Worm), func(r *proto.Raid) bool { return r.Debuffs.AcidSpit && r.Debuffs.ScorpidSting }},
		{"wolf", hunter(proto.Hunter_Options_Wolf), func(r *proto.Raid) bool {
			return !r.Debuffs.AcidSpit && !r.Debuffs.Sting && !r.Debuffs.SporeCloud && !r.Debuffs.Stampede && !r.Debuffs.DemoralizingScreech
		}},
		{"wasp", hunter(proto.Hunter_Options_Wasp), func(r *proto.Raid) bool { return r.Debuffs.Sting }},
		{"bat", hunter(proto.Hunter_Options_Bat), func(r *proto.Raid) bool { return r.Debuffs.SporeCloud }},
		{"rhino", hunter(proto.Hunter_Options_Rhino), func(r *proto.Raid) bool { return r.Debuffs.Stampede }},
		{"carrion bird", hunter(proto.Hunter_Options_CarrionBird), func(r *proto.Raid) bool { return r.Debuffs.DemoralizingScreech }},
		{"survival", newPlayer("Hunter", proto.Spec_SpecHunter, map[string]int32{"hunting_party": 5}), func(r *proto.Raid) bool {
			return r.Parties[0].Players[0].Buffs.HuntingParty
		}},

		{"battle shout", warrior(proto.Spec_SpecWarrior, proto.WarriorShout_WarriorShoutBattle, nil), func(r *proto.Raid) bool {
			return r.Buffs.BattleShout == regular && r.Buffs.CommandingShout == 0
		}},
		{"improved battle shout", warrior(proto.Spec_SpecWarrior, proto.WarriorShout_WarriorShoutBattle, map[string]int32{"commanding_presence": 5}), func(r *proto.Raid) bool {
			return r.Buffs.BattleShout == improved
		}},
		{"commanding shout", warrior(proto.Spec_SpecProtectionWarrior, proto.WarriorShout_WarriorShoutCommanding, map[string]int32{"improved_thunder_clap": 3}), func(r *proto.Raid) bool {
			return r.Buffs.CommandingShout == regular && r.Buffs.BattleShout == 0 && r.Debuffs.ThunderClap == improved
		}},
		{"no shout", warrior(proto.Spec_SpecWarrior, proto.WarriorShout_WarriorShoutNone, nil), func(r *proto.Raid) bool {
			return r.Buffs.BattleShout == 0 && r.Buffs.CommandingShout == 0 && r.Debuffs.SunderArmor && r.Debuffs.ThunderClap == regular
		}},

		{"resto druid", newPlayer("Druid", proto.Spec_SpecRestorationDruid, nil), func(r *proto.Raid) bool {
			return r.Debuffs.FaerieFire == 0 && !r.Debuffs.Mangle
		}},
		{"cat", newPlayer("Druid", proto.Spec_SpecFeralDruid, map[string]int32{"infected_wounds": 3}), func(r *proto.Raid) bool {
			return r.Debuffs.FaerieFire == regular && r.Debuffs.Mangle && r.Debuffs.InfectedWounds && r.Debuffs.DemoralizingRoar == 0
		}},
		{"bear", newPlayer("Druid", proto.Spec_SpecFeralTankDruid, map[string]int32{"feral_aggression": 5}), func(r *proto.Raid) bool {
			return r.Debuffs.DemoralizingRoar == improved && r.Debuffs.Mangle
		}},
		{"moonkin", newPlayer("Druid", proto.Spec_SpecBalanceDruid, map[string]int32{"improved_faerie_fire": 3, "earth_and_moon": 3, "insect_swarm": 1}), func(r *proto.Raid) bool {
			return r.Debuffs.FaerieFire == improved && r.Debuffs.EarthAndMoon && r.Debuffs.InsectSwarm
		}},

		{"totem of wrath", shaman(proto.FireTotem_TotemOfWrath, map[string]int32{"totem_of_wrath": 1}), func(r *proto.Raid) bool {
			return r.Debuffs.TotemOfWrath && r.Buffs.TotemOfWrath && r.Buffs.Bloodlust
		}},
		{"totem of wrath untalented", shaman(proto.FireTotem_TotemOfWrath, nil), func(r *proto.Raid) bool {
			// core falls back to Flametongue Totem
			return !r.Debuffs.TotemOfWrath && !r.Buffs.TotemOfWrath && r.Buffs.FlametongueTotem
		}},

		{"shadow priest", newPlayer("Priest", proto.Spec_SpecShadowPriest, map[string]int32{"vampiric_touch": 1, "misery": 3}), func(r *proto.Raid) bool {
			return r.Parties[0].Players[0].Buffs.VampiricTouch && r.Debuffs.Misery
		}},
		{"smite priest with vampiric touch", newPlayer("Priest", proto.Spec_SpecSmitePriest, map[string]int32{"vampiric_touch": 1}), func(r *proto.Raid) bool {
			return !r.Parties[0].Players[0].Buffs.VampiricTouch
		}},
		{"renewed hope", newPlayer("Priest", proto.Spec_SpecHealingPriest, map[string]int32{"renewed_hope": 2}), func(r *proto.Raid) bool {
			return r.Parties[0].Players[0].Buffs.RenewedHope
		}},

		{"ret", newPlayer("Paladin", proto.Spec_SpecRetributionPaladin, map[string]int32{"judgements_of_the_wise": 3, "heart_of_the_crusader": 3, "vindication": 2}), func(r *proto.Raid) bool {
			return r.Parties[0].Players[0].Buffs.JudgementsOfTheWise && r.Debuffs.HeartOfTheCrusader && r.Debuffs.Vindication
		}},
		{"holy paladin", newPlayer("Paladin", proto.Spec_SpecHolyPaladin, map[string]int32{"judgements_of_the_wise": 3, "heart_of_the_crusader": 3, "divine_guardian": 2}), func(r *proto.Raid) bool {
			buffs := r.Parties[0].Players[0].Buffs
			return !buffs.JudgementsOfTheWise && !r.Debuffs.HeartOfTheCrusader && buffs.DivineGuardians == 1
		}},

		{"warlock", newPlayer("Warlock", proto.Spec_SpecWarlock, map[string]int32{"improved_shadow_bolt": 5}), func(r *proto.Raid) bool {
			return r.Debuffs.CurseOfElements && r.Debuffs.CurseOfWeakness == regular && r.Debuffs.ShadowMastery
		}},
		{"warlock with improved curse of weakness", newPlayer("Warlock", proto.Spec_SpecWarlock, map[string]int32{"improved_curse_of_weakness": 2}), func(r *proto.Raid) bool {
			return r.Debuffs.CurseOfWeakness == improved
		}},
		{"frost dk", newPlayer("DK", proto.Spec_SpecDeathknight, map[string]int32{"improved_icy_touch": 3, "ebon_plaguebringer": 3}), func(r *proto.Raid) bool {
			return r.Debuffs.FrostFever == improved && r.Debuffs.EbonPlaguebringer
		}},
		{"rogue", newPlayer("Rogue", proto.Spec_SpecRogue, map[string]int32{"savage_combat": 2, "master_poisoner": 3}), func(r *proto.Raid) bool {
			// Master Poisoner only helps the rogue who applies it (core.MasterPoisonerDebuff), so it
			// gives the raid no debuff to provide here.
			return r.Debuffs.ExposeArmor && r.Debuffs.SavageCombat && !r.Debuffs.MasterPoisoner
		}},
		{"mage", newPlayer("Mage", proto.Spec_SpecMage, map[string]int32{"improved_scorch": 3, "winters_chill": 3, "enduring_winter": 3}), func(r *proto.Raid) bool {
			return r.Debuffs.ImprovedScorch && r.Debuffs.WintersChill && r.Parties[0].Players[0].Buffs.EnduringWinter
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			target := newPlayer("Target", proto.Spec_SpecWarrior, nil)
			raid := mustDerive(t, raidOf(target, c.provider), 0)
			if !c.want(raid) {
				t.Errorf("got raid buffs %v, debuffs %v, individual buffs %v", raid.Buffs, raid.Debuffs, raid.Parties[0].Players[0].Buffs)
			}
		})
	}
}
