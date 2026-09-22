package raidctx

import (
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/deathknight"
	"github.com/wowsims/wotlk/sim/druid"
	"github.com/wowsims/wotlk/sim/hunter"
	"github.com/wowsims/wotlk/sim/mage"
	"github.com/wowsims/wotlk/sim/paladin"
	"github.com/wowsims/wotlk/sim/priest"
	"github.com/wowsims/wotlk/sim/rogue"
	"github.com/wowsims/wotlk/sim/shaman"
	"github.com/wowsims/wotlk/sim/warlock"
	"github.com/wowsims/wotlk/sim/warrior"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// effects is what one raider gets from the rest of the raid, in the fields an individual sim reads.
type effects struct {
	raid       *proto.RaidBuffs
	debuffs    *proto.Debuffs
	individual *proto.IndividualBuffs
}

// provider is one way a raider gives the others something that no core API adds for it.
type provider struct {
	// the RAID_STATS_OPTIONS entry (ui/raid/raid_stats.ts) this row mirrors: section/category/label
	ui    string
	class proto.Class
	// empty: any spec of the class
	specs []proto.Spec
	// a talent's proto field name; the row needs a point in it, or none with missing set
	talent  string
	missing bool
	// anything else the row needs, like a pet or totem choice
	when func(*proto.Player) bool
	// set for targeted buffs: the spec option naming who gets it
	target protoreflect.Name
	apply  func(*effects)
}

func (p *provider) gives(player *proto.Player, spec proto.Spec, talents protoreflect.Message, targetIndex int) bool {
	if player.Class != p.class {
		return false
	}
	if len(p.specs) > 0 && !slices.Contains(p.specs, spec) {
		return false
	}
	if p.talent != "" && hasTalent(talents, p.talent) == p.missing {
		return false
	}
	if p.when != nil && !p.when(player) {
		return false
	}
	if p.target != "" && !pointsAt(specOptionRef(player, p.target), targetIndex) {
		return false
	}
	return true
}

const (
	regular  = proto.TristateEffect_TristateEffectRegular
	improved = proto.TristateEffect_TristateEffectImproved
)

func atLeast(effect *proto.TristateEffect, value proto.TristateEffect) {
	*effect = max(*effect, value)
}

// providers covers debuffs, replenishment, targeted buffs, and the buffs players only give by
// casting in a raid sim (Bloodlust, shouts) or not at all (Renewed Hope, Divine Guardian). Every
// other RAID_STATS_OPTIONS buff comes from core's AddRaidBuffs and AddPartyBuffs, or is a blessing
// the raid UI already wrote into the target's IndividualBuffs. Vigilance and Pain Suppression stay
// out: they land on one player and no spec option says whose. Rows follow the UI's conditions even
// where the sim's differ, e.g. the UI gives every warlock both curses. TestProvidersMirrorRaidStats
// fails when the UI gains a provider nobody sorted.
var providers = []provider{
	{
		ui:    "Buffs/Bloodlust/Bloodlust",
		class: proto.Class_ClassShaman,
		apply: func(e *effects) { e.raid.Bloodlust = true },
	},
	{
		ui:     "Buffs/Atk Pwr/Improved Battle Shout",
		class:  proto.Class_ClassWarrior,
		talent: "commanding_presence",
		when:   shouts(proto.WarriorShout_WarriorShoutBattle),
		apply:  func(e *effects) { atLeast(&e.raid.BattleShout, improved) },
	},
	{
		ui:      "Buffs/Atk Pwr/Battle Shout",
		class:   proto.Class_ClassWarrior,
		talent:  "commanding_presence",
		missing: true,
		when:    shouts(proto.WarriorShout_WarriorShoutBattle),
		apply:   func(e *effects) { atLeast(&e.raid.BattleShout, regular) },
	},
	{
		ui:     "Buffs/Mit %/Renewed Hope",
		class:  proto.Class_ClassPriest,
		talent: "renewed_hope",
		apply:  func(e *effects) { e.individual.RenewedHope = true },
	},
	{
		ui:     "Buffs/Health/Improved Commanding Shout",
		class:  proto.Class_ClassWarrior,
		talent: "commanding_presence",
		when:   shouts(proto.WarriorShout_WarriorShoutCommanding),
		apply:  func(e *effects) { atLeast(&e.raid.CommandingShout, improved) },
	},
	{
		ui:      "Buffs/Health/Commanding Shout",
		class:   proto.Class_ClassWarrior,
		talent:  "commanding_presence",
		missing: true,
		when:    shouts(proto.WarriorShout_WarriorShoutCommanding),
		apply:   func(e *effects) { atLeast(&e.raid.CommandingShout, regular) },
	},
	{
		ui:     "Buffs/Replenishment/Vampiric Touch",
		class:  proto.Class_ClassPriest,
		specs:  []proto.Spec{proto.Spec_SpecShadowPriest},
		talent: "vampiric_touch",
		apply:  func(e *effects) { e.individual.VampiricTouch = true },
	},
	{
		ui:     "Buffs/Replenishment/Judgements of the Wise",
		class:  proto.Class_ClassPaladin,
		specs:  []proto.Spec{proto.Spec_SpecRetributionPaladin},
		talent: "judgements_of_the_wise",
		apply:  func(e *effects) { e.individual.JudgementsOfTheWise = true },
	},
	{
		ui:     "Buffs/Replenishment/Hunting Party",
		class:  proto.Class_ClassHunter,
		specs:  []proto.Spec{proto.Spec_SpecHunter},
		talent: "hunting_party",
		apply:  func(e *effects) { e.individual.HuntingParty = true },
	},
	{
		ui:     "Buffs/Replenishment/Improved Soul Leech",
		class:  proto.Class_ClassWarlock,
		specs:  []proto.Spec{proto.Spec_SpecWarlock},
		talent: "improved_soul_leech",
		apply:  func(e *effects) { e.individual.ImprovedSoulLeech = true },
	},
	{
		ui:     "Buffs/Replenishment/Enduring Winter",
		class:  proto.Class_ClassMage,
		specs:  []proto.Spec{proto.Spec_SpecMage},
		talent: "enduring_winter",
		apply:  func(e *effects) { e.individual.EnduringWinter = true },
	},

	{
		ui:     "External Buffs/Innervate/Innervate",
		class:  proto.Class_ClassDruid,
		target: "innervate_target",
		apply:  func(e *effects) { e.individual.Innervates++ },
	},
	{
		ui:     "External Buffs/Power Infusion/Power Infusion",
		class:  proto.Class_ClassPriest,
		talent: "power_infusion",
		target: "power_infusion_target",
		apply:  func(e *effects) { e.individual.PowerInfusions++ },
	},
	{
		ui:     "External Buffs/Focus Magic/Focus Magic",
		class:  proto.Class_ClassMage,
		talent: "focus_magic",
		target: "focus_magic_target",
		apply:  func(e *effects) { e.individual.FocusMagic = true },
	},
	{
		ui:     "External Buffs/Tricks of the Trade/Tricks of the Trade",
		class:  proto.Class_ClassRogue,
		target: "tricks_of_the_trade_target",
		apply:  func(e *effects) { e.individual.TricksOfTheTrades++ },
	},
	{
		ui:     "External Buffs/Unholy Frenzy/Unholy Frenzy",
		class:  proto.Class_ClassDeathknight,
		talent: "hysteria",
		target: "unholy_frenzy_target",
		apply:  func(e *effects) { e.individual.UnholyFrenzy++ },
	},
	{
		// no target to name: with this talent, Divine Sacrifice shields the whole raid
		ui:     "External Buffs/Divine Guardian/Divine Guardian",
		class:  proto.Class_ClassPaladin,
		talent: "divine_guardian",
		apply:  func(e *effects) { e.individual.DivineGuardians++ },
	},

	{
		ui:    "DPS Debuffs/Major ArP/Sunder Armor",
		class: proto.Class_ClassWarrior,
		apply: func(e *effects) { e.debuffs.SunderArmor = true },
	},
	{
		ui:    "DPS Debuffs/Major ArP/Expose Armor",
		class: proto.Class_ClassRogue,
		apply: func(e *effects) { e.debuffs.ExposeArmor = true },
	},
	{
		ui:    "DPS Debuffs/Major ArP/Acid Spit",
		class: proto.Class_ClassHunter,
		when:  hasPet(proto.Hunter_Options_Worm),
		apply: func(e *effects) { e.debuffs.AcidSpit = true },
	},
	{
		ui:    "DPS Debuffs/Minor ArP/Faerie Fire",
		class: proto.Class_ClassDruid,
		specs: []proto.Spec{proto.Spec_SpecBalanceDruid, proto.Spec_SpecFeralDruid, proto.Spec_SpecFeralTankDruid},
		apply: func(e *effects) { atLeast(&e.debuffs.FaerieFire, regular) },
	},
	{
		ui:    "DPS Debuffs/Minor ArP/Curse of Weakness",
		class: proto.Class_ClassWarlock,
		apply: func(e *effects) { atLeast(&e.debuffs.CurseOfWeakness, regular) },
	},
	{
		ui:    "DPS Debuffs/Minor ArP/Sting",
		class: proto.Class_ClassHunter,
		when:  hasPet(proto.Hunter_Options_Wasp),
		apply: func(e *effects) { e.debuffs.Sting = true },
	},
	{
		ui:    "DPS Debuffs/Minor ArP/Spore Cloud",
		class: proto.Class_ClassHunter,
		when:  hasPet(proto.Hunter_Options_Bat),
		apply: func(e *effects) { e.debuffs.SporeCloud = true },
	},
	{
		ui:     "DPS Debuffs/Phys Vuln/Blood Frenzy",
		class:  proto.Class_ClassWarrior,
		talent: "blood_frenzy",
		apply:  func(e *effects) { e.debuffs.BloodFrenzy = true },
	},
	{
		ui:     "DPS Debuffs/Phys Vuln/Savage Combat",
		class:  proto.Class_ClassRogue,
		talent: "savage_combat",
		apply:  func(e *effects) { e.debuffs.SavageCombat = true },
	},
	{
		ui:    "DPS Debuffs/Bleed/Mangle",
		class: proto.Class_ClassDruid,
		specs: []proto.Spec{proto.Spec_SpecFeralDruid, proto.Spec_SpecFeralTankDruid},
		apply: func(e *effects) { e.debuffs.Mangle = true },
	},
	{
		ui:     "DPS Debuffs/Bleed/Trauma",
		class:  proto.Class_ClassWarrior,
		talent: "trauma",
		apply:  func(e *effects) { e.debuffs.Trauma = true },
	},
	{
		ui:    "DPS Debuffs/Bleed/Stampede",
		class: proto.Class_ClassHunter,
		when:  hasPet(proto.Hunter_Options_Rhino),
		apply: func(e *effects) { e.debuffs.Stampede = true },
	},
	{
		ui:     "DPS Debuffs/Crit/Totem of Wrath",
		class:  proto.Class_ClassShaman,
		talent: "totem_of_wrath",
		when: func(p *proto.Player) bool {
			return shamanTotems(p).GetFire() == proto.FireTotem_TotemOfWrath
		},
		apply: func(e *effects) { e.debuffs.TotemOfWrath = true },
	},
	{
		ui:     "DPS Debuffs/Crit/Heart of the Crusader",
		class:  proto.Class_ClassPaladin,
		specs:  []proto.Spec{proto.Spec_SpecRetributionPaladin, proto.Spec_SpecProtectionPaladin},
		talent: "heart_of_the_crusader",
		apply:  func(e *effects) { e.debuffs.HeartOfTheCrusader = true },
	},
	{
		ui:     "DPS Debuffs/Spell Crit/Improved Shadow Bolt",
		class:  proto.Class_ClassWarlock,
		talent: "improved_shadow_bolt",
		apply:  func(e *effects) { e.debuffs.ShadowMastery = true },
	},
	{
		ui:     "DPS Debuffs/Spell Crit/Improved Scorch",
		class:  proto.Class_ClassMage,
		talent: "improved_scorch",
		apply:  func(e *effects) { e.debuffs.ImprovedScorch = true },
	},
	{
		ui:     "DPS Debuffs/Spell Crit/Winter's Chill",
		class:  proto.Class_ClassMage,
		talent: "winters_chill",
		apply:  func(e *effects) { e.debuffs.WintersChill = true },
	},
	{
		ui:     "DPS Debuffs/Spell Hit/Misery",
		class:  proto.Class_ClassPriest,
		specs:  []proto.Spec{proto.Spec_SpecShadowPriest},
		talent: "misery",
		apply:  func(e *effects) { e.debuffs.Misery = true },
	},
	{
		ui:     "DPS Debuffs/Spell Hit/Improved Faerie Fire",
		class:  proto.Class_ClassDruid,
		specs:  []proto.Spec{proto.Spec_SpecBalanceDruid},
		talent: "improved_faerie_fire",
		apply:  func(e *effects) { atLeast(&e.debuffs.FaerieFire, improved) },
	},
	{
		ui:     "DPS Debuffs/Spell Dmg/Ebon Plaguebringer",
		class:  proto.Class_ClassDeathknight,
		talent: "ebon_plaguebringer",
		apply:  func(e *effects) { e.debuffs.EbonPlaguebringer = true },
	},
	{
		ui:     "DPS Debuffs/Spell Dmg/Earth and Moon",
		class:  proto.Class_ClassDruid,
		specs:  []proto.Spec{proto.Spec_SpecBalanceDruid},
		talent: "earth_and_moon",
		apply:  func(e *effects) { e.debuffs.EarthAndMoon = true },
	},
	{
		ui:    "DPS Debuffs/Spell Dmg/Curse of Elements",
		class: proto.Class_ClassWarlock,
		apply: func(e *effects) { e.debuffs.CurseOfElements = true },
	},

	{
		ui:     "Mitigation Debuffs/Atk Pwr/Vindication",
		class:  proto.Class_ClassPaladin,
		specs:  []proto.Spec{proto.Spec_SpecRetributionPaladin, proto.Spec_SpecProtectionPaladin},
		talent: "vindication",
		apply:  func(e *effects) { e.debuffs.Vindication = true },
	},
	{
		ui:     "Mitigation Debuffs/Atk Pwr/Improved Demoralizing Shout",
		class:  proto.Class_ClassWarrior,
		talent: "improved_demoralizing_shout",
		apply:  func(e *effects) { atLeast(&e.debuffs.DemoralizingShout, improved) },
	},
	{
		ui:      "Mitigation Debuffs/Atk Pwr/Demoralizing Shout",
		class:   proto.Class_ClassWarrior,
		talent:  "improved_demoralizing_shout",
		missing: true,
		apply:   func(e *effects) { atLeast(&e.debuffs.DemoralizingShout, regular) },
	},
	{
		ui:     "Mitigation Debuffs/Atk Pwr/Improved Demoralizing Roar",
		class:  proto.Class_ClassDruid,
		specs:  []proto.Spec{proto.Spec_SpecFeralTankDruid},
		talent: "feral_aggression",
		apply:  func(e *effects) { atLeast(&e.debuffs.DemoralizingRoar, improved) },
	},
	{
		ui:      "Mitigation Debuffs/Atk Pwr/Demoralizing Roar",
		class:   proto.Class_ClassDruid,
		specs:   []proto.Spec{proto.Spec_SpecFeralTankDruid},
		talent:  "feral_aggression",
		missing: true,
		apply:   func(e *effects) { atLeast(&e.debuffs.DemoralizingRoar, regular) },
	},
	{
		ui:     "Mitigation Debuffs/Atk Pwr/Improved Curse of Weakness",
		class:  proto.Class_ClassWarlock,
		talent: "improved_curse_of_weakness",
		apply:  func(e *effects) { atLeast(&e.debuffs.CurseOfWeakness, improved) },
	},
	{
		ui:      "Mitigation Debuffs/Atk Pwr/Curse of Weakness",
		class:   proto.Class_ClassWarlock,
		talent:  "improved_curse_of_weakness",
		missing: true,
		apply:   func(e *effects) { atLeast(&e.debuffs.CurseOfWeakness, regular) },
	},
	{
		ui:    "Mitigation Debuffs/Atk Pwr/Demoralizing Screech",
		class: proto.Class_ClassHunter,
		when:  hasPet(proto.Hunter_Options_CarrionBird),
		apply: func(e *effects) { e.debuffs.DemoralizingScreech = true },
	},
	{
		ui:     "Mitigation Debuffs/Atk Speed/Improved Thunder Clap",
		class:  proto.Class_ClassWarrior,
		talent: "improved_thunder_clap",
		apply:  func(e *effects) { atLeast(&e.debuffs.ThunderClap, improved) },
	},
	{
		ui:      "Mitigation Debuffs/Atk Speed/Thunder Clap",
		class:   proto.Class_ClassWarrior,
		talent:  "improved_thunder_clap",
		missing: true,
		apply:   func(e *effects) { atLeast(&e.debuffs.ThunderClap, regular) },
	},
	{
		ui:     "Mitigation Debuffs/Atk Speed/Improved Frost Fever",
		class:  proto.Class_ClassDeathknight,
		talent: "improved_icy_touch",
		apply:  func(e *effects) { atLeast(&e.debuffs.FrostFever, improved) },
	},
	{
		ui:      "Mitigation Debuffs/Atk Speed/Frost Fever",
		class:   proto.Class_ClassDeathknight,
		talent:  "improved_icy_touch",
		missing: true,
		apply:   func(e *effects) { atLeast(&e.debuffs.FrostFever, regular) },
	},
	{
		ui:     "Mitigation Debuffs/Atk Speed/Judgements of the Just",
		class:  proto.Class_ClassPaladin,
		talent: "judgements_of_the_just",
		apply:  func(e *effects) { e.debuffs.JudgementsOfTheJust = true },
	},
	{
		ui:     "Mitigation Debuffs/Atk Speed/Infected Wounds",
		class:  proto.Class_ClassDruid,
		specs:  []proto.Spec{proto.Spec_SpecFeralDruid, proto.Spec_SpecFeralTankDruid},
		talent: "infected_wounds",
		apply:  func(e *effects) { e.debuffs.InfectedWounds = true },
	},
	{
		ui:     "Mitigation Debuffs/Miss/Insect Swarm",
		class:  proto.Class_ClassDruid,
		specs:  []proto.Spec{proto.Spec_SpecBalanceDruid},
		talent: "insect_swarm",
		apply:  func(e *effects) { e.debuffs.InsectSwarm = true },
	},
	{
		ui:    "Mitigation Debuffs/Miss/Scorpid Sting",
		class: proto.Class_ClassHunter,
		apply: func(e *effects) { e.debuffs.ScorpidSting = true },
	},
}

// demonicPact is shared through the warlock's pet, so without one the sim gives the raid nothing.
// Derive sets its spell power with DemonicPactSP.
var demonicPact = provider{
	ui:     "Buffs/Spell Power/Demonic Pact",
	class:  proto.Class_ClassWarlock,
	talent: "demonic_pact",
	when: func(p *proto.Player) bool {
		return p.GetWarlock().GetOptions().GetSummon() != proto.Warlock_Options_NoSummon
	},
}

// DemonicPactSP is the spell power Demonic Pact gives the raid from a warlock with this much spell
// power, not counting the pact itself, and this many points in the talent. It's the formula the
// warlock's aura uses in the sim (setupDemonicPact): 2% per point, rounded, retaken on the pet's
// crits. With the warlock's sheet spell power it's where the aura sits while no proc is up, so it
// runs below the sim's average: the sheet misses combat-only spell power (Demonic Knowledge, the
// Life Tap glyph, procs), and the aura only takes a lower value in its last 10 s.
func DemonicPactSP(spellPower float64, points int32) int32 {
	return int32(math.Round(0.02 * float64(points) * spellPower))
}

func shouts(shout proto.WarriorShout) func(*proto.Player) bool {
	return func(p *proto.Player) bool {
		if options := p.GetWarrior().GetOptions(); options != nil {
			return options.Shout == shout
		}
		return p.GetProtectionWarrior().GetOptions().GetShout() == shout
	}
}

func hasPet(pet proto.Hunter_Options_PetType) func(*proto.Player) bool {
	return func(p *proto.Player) bool {
		return p.GetHunter().GetOptions().GetPetType() == pet
	}
}

func shamanTotems(p *proto.Player) *proto.ShamanTotems {
	switch spec := p.Spec.(type) {
	case *proto.Player_ElementalShaman:
		return spec.ElementalShaman.GetOptions().GetTotems()
	case *proto.Player_EnhancementShaman:
		return spec.EnhancementShaman.GetOptions().GetTotems()
	case *proto.Player_RestorationShaman:
		return spec.RestorationShaman.GetOptions().GetTotems()
	}
	return nil
}

// specOptionRef reads a UnitReference from the player's spec options, e.g. warlock.options.name.
// Nil when the spec has no such option.
func specOptionRef(p *proto.Player, name protoreflect.Name) *proto.UnitReference {
	options := specOptions(p)
	if options == nil {
		return nil
	}
	fd := options.Descriptor().Fields().ByName(name)
	if fd == nil || !options.Has(fd) {
		return nil
	}
	ref, _ := options.Get(fd).Message().Interface().(*proto.UnitReference)
	return ref
}

func specOptions(p *proto.Player) protoreflect.Message {
	m := p.ProtoReflect()
	fd := m.WhichOneof(m.Descriptor().Oneofs().ByName("spec"))
	if fd == nil {
		return nil
	}
	spec := m.Get(fd).Message()
	options := spec.Descriptor().Fields().ByName("options")
	if options == nil || !spec.Has(options) {
		return nil
	}
	return spec.Get(options).Message()
}

// pointsAt tells whether ref names the player at raid index, which UnitReferences count as party
// index * 5 + position, like core's GetUnit.
func pointsAt(ref *proto.UnitReference, index int) bool {
	return ref.GetType() == proto.UnitReference_Player && int(ref.GetIndex()) == index
}

var talentTrees = map[proto.Class]struct {
	new   func() protoreflect.Message
	sizes [3]int
}{
	proto.Class_ClassDeathknight: {func() protoreflect.Message { return (&proto.DeathknightTalents{}).ProtoReflect() }, deathknight.TalentTreeSizes},
	proto.Class_ClassDruid:       {func() protoreflect.Message { return (&proto.DruidTalents{}).ProtoReflect() }, druid.TalentTreeSizes},
	proto.Class_ClassHunter:      {func() protoreflect.Message { return (&proto.HunterTalents{}).ProtoReflect() }, hunter.TalentTreeSizes},
	proto.Class_ClassMage:        {func() protoreflect.Message { return (&proto.MageTalents{}).ProtoReflect() }, mage.TalentTreeSizes},
	proto.Class_ClassPaladin:     {func() protoreflect.Message { return (&proto.PaladinTalents{}).ProtoReflect() }, paladin.TalentTreeSizes},
	proto.Class_ClassPriest:      {func() protoreflect.Message { return (&proto.PriestTalents{}).ProtoReflect() }, priest.TalentTreeSizes},
	proto.Class_ClassRogue:       {func() protoreflect.Message { return (&proto.RogueTalents{}).ProtoReflect() }, rogue.TalentTreeSizes},
	proto.Class_ClassShaman:      {func() protoreflect.Message { return (&proto.ShamanTalents{}).ProtoReflect() }, shaman.TalentTreeSizes},
	proto.Class_ClassWarlock:     {func() protoreflect.Message { return (&proto.WarlockTalents{}).ProtoReflect() }, warlock.TalentTreeSizes},
	proto.Class_ClassWarrior:     {func() protoreflect.Message { return (&proto.WarriorTalents{}).ProtoReflect() }, warrior.TalentTreeSizes},
}

// parseTalents reads the player's talent string the way its class constructor does.
func parseTalents(p *proto.Player) protoreflect.Message {
	tree, ok := talentTrees[p.Class]
	if !ok {
		return nil
	}
	talents := tree.new()
	core.FillTalentsProto(talents, p.TalentsString, tree.sizes)
	return talents
}

// talentPoints is the points in a talent; 1 for a taken single-point one.
func talentPoints(talents protoreflect.Message, name string) int32 {
	if talents == nil {
		return 0
	}
	fd := talents.Descriptor().Fields().ByName(protoreflect.Name(name))
	if fd == nil {
		return 0
	}
	v := talents.Get(fd)
	if fd.Kind() == protoreflect.BoolKind {
		if v.Bool() {
			return 1
		}
		return 0
	}
	return int32(v.Int())
}

func hasTalent(talents protoreflect.Message, name string) bool {
	return talentPoints(talents, name) > 0
}
