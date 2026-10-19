package core

import (
	"strings"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

// PlayerbotPotionAt is POTION_HOLD_MS, the bots' wait before their offensive potion.
const PlayerbotPotionAt = 5 * time.Second

// playerbotSpec keys the generated world-buff table: a server class's spec as WorldBuffAction picks it.
type playerbotSpec struct {
	class proto.Class
	tab   int32
}

// playerbotPrepullSpells are the pre-pull casts a bot still has up at the pull: states it keeps between
// fights, and the buffs its non-combat strategies recast (Horn of Winter, Bone Shield).
var playerbotPrepullSpells = map[int32]bool{
	71:    true, // Defensive Stance
	2457:  true, // Battle Stance
	2458:  true, // Berserker Stance
	48263: true, // Frost Presence
	48265: true, // Unholy Presence
	48266: true, // Blood Presence
	61847: true, // Aspect of the Dragonhawk
	57623: true, // Horn of Winter
	49222: true, // Bone Shield
}

func playerbotKeepsPrepull(action *proto.APLAction) bool {
	id := action.GetCastSpell().GetSpellId()
	return id != nil && id.GetSpellId() != 0 && playerbotPrepullSpells[id.GetSpellId()]
}

// isPlayerbot is false for pets and targets: only a player character carries the switch.
func (unit *Unit) isPlayerbot() bool {
	if unit.Type != PlayerUnit {
		return false
	}
	agent := unit.Env.GetAgentFromUnit(unit)
	return agent != nil && agent.GetCharacter() != nil && agent.GetCharacter().Playerbot
}

// AiFactory::GetPlayerSpecTab's pick for a character with no talents.
var playerbotDefaultTab = map[proto.Class]int32{
	proto.Class_ClassMage:        2,
	proto.Class_ClassPaladin:     2,
	proto.Class_ClassPriest:      1,
	proto.Class_ClassWarlock:     1,
	proto.Class_ClassDeathknight: 1,
}

// playerbotTalentTab is AiFactory::GetPlayerSpecTab: the tree with the most points, ties to the lower one.
func playerbotTalentTab(class proto.Class, talents string) int32 {
	if strings.Trim(talents, "0-") == "" {
		return playerbotDefaultTab[class]
	}
	return int32(GetPrimaryTalentTreeIndex(talents))
}

// A feral with Thick Hide 3 is a bear to IsTank and WorldBuffAction alike.
func playerbotBear(class proto.Class, tab int32, talents string) bool {
	return class == proto.Class_ClassDruid && tab == 1 && talentRank(talents, 1, 4) >= 3
}

// playerbotWorldBuffSpec is the spec WorldBuffAction reads the matrix by. 3 is a cat, or a blood DK
// without Blade Barrier 5.
func playerbotWorldBuffSpec(class proto.Class, tab int32, talents string) int32 {
	switch {
	case class == proto.Class_ClassDruid && tab == 1 && !playerbotBear(class, tab, talents):
		return 3
	case class == proto.Class_ClassDeathknight && tab == 0 && talentRank(talents, 0, 2) < 5:
		return 3
	}
	return tab
}

// talentRank reads one talent of a wowhead talent string, by tree and position in it.
func talentRank(talents string, tree, index int) int {
	trees := strings.Split(talents, "-")
	if tree >= len(trees) || index >= len(trees[tree]) {
		return 0
	}
	return int(trees[tree][index] - '0')
}

// playerbotPotion is InitPotions' offensive potion at level 80: none unless PlayerbotAI::IsDps by spec,
// Wild Magic for IsCaster, else Speed. Shadow, Balance and Affliction drink Speed instead.
func playerbotPotion(class proto.Class, tab int32, bear bool) proto.Potions {
	const speed, wildMagic = proto.Potions_PotionOfSpeed, proto.Potions_PotionOfWildMagic
	switch {
	case class == proto.Class_ClassWarlock && tab == 0:
		return speed
	case class == proto.Class_ClassMage, class == proto.Class_ClassWarlock:
		return wildMagic
	case class == proto.Class_ClassHunter, class == proto.Class_ClassRogue:
		return speed
	case class == proto.Class_ClassPriest && tab == 2:
		return speed
	case class == proto.Class_ClassDruid && tab == 0:
		return speed
	case class == proto.Class_ClassDruid && tab == 1 && !bear:
		return speed
	case class == proto.Class_ClassShaman && tab == 0:
		return wildMagic
	case class == proto.Class_ClassShaman && tab == 1:
		return speed
	case class == proto.Class_ClassPaladin && tab == 2:
		return speed
	case class == proto.Class_ClassDeathknight && tab != 0:
		return speed
	case class == proto.Class_ClassWarrior && tab != 2:
		return speed
	}
	return proto.Potions_UnknownPotion
}

// playerbotConsumes is what a bot brings: one offensive potion, the world-buff matrix's flask, elixirs
// and food, and nothing else. It copies, since the player proto is shared across sims.
func playerbotConsumes(page *proto.Consumes, class proto.Class, talents string) *proto.Consumes {
	tab := playerbotTalentTab(class, talents)
	c := googleProto.Clone(page).(*proto.Consumes)
	c.PrepopPotion = proto.Potions_UnknownPotion
	c.DefaultPotion = playerbotPotion(class, tab, playerbotBear(class, tab, talents))
	c.DefaultConjured = proto.Conjured_ConjuredUnknown
	c.ThermalSapper = false
	c.ExplosiveDecoy = false
	c.FillerExplosive = proto.Explosive_ExplosiveUnknown
	c.PetFood = proto.PetFood_PetFoodUnknown

	buffs := playerbotWorldBuffs[playerbotSpec{class, playerbotWorldBuffSpec(class, tab, talents)}]
	c.Flask = buffs.GetFlask()
	c.BattleElixir = buffs.GetBattleElixir()
	c.GuardianElixir = buffs.GetGuardianElixir()
	c.Food = buffs.GetFood()
	return c
}

// playerbotPotionAction drinks the combat potion; the potion spell itself holds it to PlayerbotPotionAt.
var playerbotPotionAction = &proto.APLAction{
	Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{
		SpellId: ActionID{OtherID: proto.OtherAction_OtherActionPotion}.ToProto(),
	}},
}

func (rot *APLRotation) addPlayerbotPotion() {
	hasPotion := false
	for _, spell := range rot.unit.Spellbook {
		hasPotion = hasPotion || spell.Flags.Matches(SpellFlagCombatPotion)
	}
	if !hasPotion {
		return
	}
	rot.doAndRecordWarnings(nil, false, func() {
		if action := rot.newAPLAction(playerbotPotionAction); action != nil {
			action.Finalize(rot)
			rot.priorityList = append([]*APLAction{action}, rot.priorityList...)
		}
	})
}
