package core

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

const (
	bloodPresenceSpellID     = 48266 // a state bots keep between fights
	empowerRuneWeaponSpellID = 47568 // a cooldown
)

func init() {
	RegisterAgentFactory(
		proto.Player_TankDeathknight{},
		proto.Spec_SpecTankDeathknight,
		func(char *Character, _ *proto.Player) Agent {
			return &playerbotTestAgent{Character: *char}
		},
		func(player *proto.Player, spec interface{}) {
			player.Spec = spec.(*proto.Player_TankDeathknight)
		},
	)
}

// Knows one pre-pull cast bots keep and one they don't.
type playerbotTestAgent struct {
	Character
}

func (a *playerbotTestAgent) GetCharacter() *Character { return &a.Character }
func (a *playerbotTestAgent) Initialize() {
	for _, id := range []int32{bloodPresenceSpellID, empowerRuneWeaponSpellID} {
		a.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: id}, Flags: SpellFlagAPL})
	}
}
func (a *playerbotTestAgent) ApplyTalents()            {}
func (a *playerbotTestAgent) Reset(_ *Simulation)      {}
func (a *playerbotTestAgent) OnGCDReady(_ *Simulation) {}

// everything a bot leaves out or brings its own of
func pageConsumes() *proto.Consumes {
	return &proto.Consumes{
		Flask:              proto.Flask_FlaskOfStoneblood,
		BattleElixir:       proto.BattleElixir_ElixirOfAccuracy,
		Food:               proto.Food_FoodFishFeast,
		PetFood:            proto.PetFood_PetFoodSpicedMammothTreats,
		DefaultPotion:      proto.Potions_IndestructiblePotion,
		PrepopPotion:       proto.Potions_PotionOfSpeed,
		DefaultConjured:    proto.Conjured_ConjuredDarkRune,
		ThermalSapper:      true,
		ExplosiveDecoy:     true,
		FillerExplosive:    proto.Explosive_ExplosiveSaroniteBomb,
		PetScrollOfAgility: 5,
	}
}

func castAt(id int32, doAt string) *proto.APLPrepullAction {
	return &proto.APLPrepullAction{
		Action:    &proto.APLAction{Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{SpellId: ActionID{SpellID: id}.ToProto()}}},
		DoAtValue: &proto.APLValue{Value: &proto.APLValue_Const{Const: &proto.APLValueConst{Val: doAt}}},
	}
}

func playerbotTestRaid(playerbot bool) *proto.Raid {
	return &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{
		Name:      "Death Knight",
		Race:      proto.Race_RaceDwarf,
		Class:     proto.Class_ClassDeathknight,
		Equipment: &proto.EquipmentSpec{},
		Consumes:  pageConsumes(),
		Spec:      &proto.Player_TankDeathknight{TankDeathknight: &proto.TankDeathknight{}},
		Playerbot: playerbot,
		Rotation: &proto.APLRotation{
			Type: proto.APLRotation_TypeAPL,
			PrepullActions: []*proto.APLPrepullAction{
				castAt(bloodPresenceSpellID, "-3s"),
				castAt(empowerRuneWeaponSpellID, "-2s"),
				{
					Action:    &proto.APLAction{Action: &proto.APLAction_ActivateAura{ActivateAura: &proto.APLActionActivateAura{AuraId: ActionID{SpellID: 16870}.ToProto()}}},
					DoAtValue: &proto.APLValue{Value: &proto.APLValue_Const{Const: &proto.APLValueConst{Val: "-1s"}}},
				},
			},
			PriorityList: []*proto.APLListItem{
				{Action: &proto.APLAction{Action: &proto.APLAction_AutocastOtherCooldowns{AutocastOtherCooldowns: &proto.APLActionAutocastOtherCooldowns{}}}},
			},
		},
	}}}}}
}

func playerbotTestEncounter() *proto.Encounter {
	return &proto.Encounter{Duration: 30, Targets: []*proto.Target{NewDefaultTarget()}}
}

func TestPlayerbotConsumes(t *testing.T) {
	bot := googleProto.Clone(pageConsumes()).(*proto.Consumes)
	bot.Flask = playerbotWorldBuffs[playerbotSpec{proto.Class_ClassDeathknight, 1}].GetFlask()
	bot.BattleElixir = proto.BattleElixir_BattleElixirUnknown
	bot.Food = playerbotWorldBuffs[playerbotSpec{proto.Class_ClassDeathknight, 1}].GetFood()
	bot.PetFood = proto.PetFood_PetFoodUnknown
	bot.DefaultPotion = proto.Potions_PotionOfSpeed
	bot.PrepopPotion = proto.Potions_UnknownPotion
	bot.DefaultConjured = proto.Conjured_ConjuredUnknown
	bot.ThermalSapper = false
	bot.ExplosiveDecoy = false
	bot.FillerExplosive = proto.Explosive_ExplosiveUnknown
	if bot.Flask == proto.Flask_FlaskUnknown || bot.Food == proto.Food_FoodUnknown {
		t.Fatalf("no bot flask or food for a frost death knight: %v", bot)
	}

	for _, playerbot := range []bool{false, true} {
		raid := playerbotTestRaid(playerbot)
		env, _, _ := NewEnvironment(raid, playerbotTestEncounter(), false)
		got := env.Raid.Parties[0].Players[0].GetCharacter().Consumes
		want := map[bool]*proto.Consumes{false: pageConsumes(), true: bot}[playerbot]
		if !googleProto.Equal(got, want) {
			t.Errorf("playerbot %v: consumes %v, want %v", playerbot, got, want)
		}
		if !googleProto.Equal(raid.Parties[0].Players[0].Consumes, pageConsumes()) {
			t.Errorf("playerbot %v: the player proto's consumes changed", playerbot)
		}
	}
}

// Tanks and healers stock no offensive potion.
func TestPlayerbotPotionBySpec(t *testing.T) {
	const (
		none      = proto.Potions_UnknownPotion
		speed     = proto.Potions_PotionOfSpeed
		wildMagic = proto.Potions_PotionOfWildMagic
	)
	for _, tc := range []struct {
		name    string
		class   proto.Class
		talents string
		want    proto.Potions
	}{
		{"balance", proto.Class_ClassDruid, "5", speed},
		{"cat", proto.Class_ClassDruid, "-5", speed},
		{"bear, Thick Hide 3", proto.Class_ClassDruid, "-00003", none},
		{"restoration druid", proto.Class_ClassDruid, "--5", none},
		{"shadow", proto.Class_ClassPriest, "--5", speed},
		{"discipline", proto.Class_ClassPriest, "5", none},
		{"no talents: a priest is holy", proto.Class_ClassPriest, "", none},
		{"elemental", proto.Class_ClassShaman, "5", wildMagic},
		{"enhancement", proto.Class_ClassShaman, "-5", speed},
		{"restoration shaman", proto.Class_ClassShaman, "--5", none},
		{"holy paladin", proto.Class_ClassPaladin, "5", none},
		{"protection paladin", proto.Class_ClassPaladin, "-5", none},
		{"retribution", proto.Class_ClassPaladin, "--5", speed},
		{"fury", proto.Class_ClassWarrior, "-5", speed},
		{"protection warrior", proto.Class_ClassWarrior, "--5", none},
		{"blood DPS", proto.Class_ClassDeathknight, "5", none},
		{"blood tank", proto.Class_ClassDeathknight, "005", none},
		{"no talents: a DK is frost", proto.Class_ClassDeathknight, "", speed},
		{"unholy", proto.Class_ClassDeathknight, "--5", speed},
		{"hunter", proto.Class_ClassHunter, "5", speed},
		{"rogue", proto.Class_ClassRogue, "-5", speed},
		{"mage", proto.Class_ClassMage, "5", wildMagic},
		{"affliction", proto.Class_ClassWarlock, "5", speed},
		{"no talents: a warlock is demonology", proto.Class_ClassWarlock, "", wildMagic},
		{"demonology", proto.Class_ClassWarlock, "-5", wildMagic},
		{"destruction", proto.Class_ClassWarlock, "--5", wildMagic},
	} {
		if got := playerbotConsumes(&proto.Consumes{}, tc.class, tc.talents).DefaultPotion; got != tc.want {
			t.Errorf("%s: potion %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPlayerbotWorldBuffSpec(t *testing.T) {
	for _, tc := range []struct {
		name    string
		class   proto.Class
		talents string
		want    int32
	}{
		{"cat", proto.Class_ClassDruid, "-543202132322010053120030310511-203503012", 3},
		{"bear, Thick Hide 3", proto.Class_ClassDruid, "-503232132322010353120300313511-20350001", 1},
		{"balance", proto.Class_ClassDruid, "5032003115331303213305311231--205003012", 0},
		{"blood DPS", proto.Class_ClassDeathknight, "2305120530003303231023001351--230220305003", 3},
		{"blood tank, Blade Barrier 5", proto.Class_ClassDeathknight, "005512153330030320102013-3050505000023-005", 0},
		{"unholy", proto.Class_ClassDeathknight, "-320033500002-2300303050032152000150013133151", 2},
		{"a tie goes to the lower tree", proto.Class_ClassRogue, "05-5", 0},
		{"no talents: a mage is frost", proto.Class_ClassMage, "", 2},
		{"no talents: a priest is holy", proto.Class_ClassPriest, "0-0-0", 1},
		{"no talents: a warrior is arms", proto.Class_ClassWarrior, "", 0},
	} {
		tab := playerbotTalentTab(tc.class, tc.talents)
		if got := playerbotWorldBuffSpec(tc.class, tab, tc.talents); got != tc.want {
			t.Errorf("%s: spec %d, want %d", tc.name, got, tc.want)
		}
	}

	// the positions talentRank reads, against the talent protos' layout
	druid := &proto.DruidTalents{}
	FillTalentsProto(druid.ProtoReflect(), "-00003", [3]int{28, 30, 27})
	dk := &proto.DeathknightTalents{}
	FillTalentsProto(dk.ProtoReflect(), "005", [3]int{28, 29, 31})
	if druid.ThickHide != 3 || dk.BladeBarrier != 5 {
		t.Errorf("Thick Hide %d, Blade Barrier %d: talentRank reads the wrong talents", druid.ThickHide, dk.BladeBarrier)
	}
}

func TestPlayerbotPrepull(t *testing.T) {
	for _, tc := range []struct {
		playerbot bool
		kept      []int32
	}{
		{false, []int32{bloodPresenceSpellID, empowerRuneWeaponSpellID}},
		{true, []int32{bloodPresenceSpellID}},
	} {
		env, _, _ := NewEnvironment(playerbotTestRaid(tc.playerbot), playerbotTestEncounter(), false)
		rot := env.Raid.Parties[0].Players[0].GetCharacter().Rotation

		var kept []int32
		for _, action := range rot.prepullActions {
			if cast, ok := action.impl.(*APLActionCastSpell); ok {
				kept = append(kept, cast.spell.ActionID.SpellID)
			}
		}
		if !slices.Equal(kept, tc.kept) {
			t.Errorf("playerbot %v: pre-pull casts %v, want %v", tc.playerbot, kept, tc.kept)
		}

		skipped := 0
		for _, warnings := range rot.prepullWarnings {
			for _, w := range warnings {
				if strings.HasPrefix(w, "Playerbot:") {
					skipped++
				}
			}
		}
		if want := map[bool]int{false: 0, true: 2}[tc.playerbot]; skipped != want {
			t.Errorf("playerbot %v: %d pre-pull actions flagged as skipped, want %d", tc.playerbot, skipped, want)
		}

		// the page's pre-pot comes in at -1 s unless the switch is on
		prepot := slices.ContainsFunc(env.prepullActions, func(a PrepullAction) bool { return a.DoAt == -time.Second })
		if prepot == tc.playerbot {
			t.Errorf("playerbot %v: automatic pre-pot %v", tc.playerbot, prepot)
		}
	}
}

// Neither autocast nor a potion entry of the rotation may drink before the injected action.
func TestPlayerbotPotion(t *testing.T) {
	potionEntry := &proto.APLListItem{Action: &proto.APLAction{Action: &proto.APLAction_CastSpell{
		CastSpell: &proto.APLActionCastSpell{SpellId: ActionID{OtherID: proto.OtherAction_OtherActionPotion}.ToProto()}}}}
	rotations := []struct {
		name         string
		priorityList []*proto.APLListItem
	}{
		{"no rotation", nil},
		{"autocast", playerbotTestRaid(false).Parties[0].Players[0].Rotation.PriorityList},
		{"a potion entry", []*proto.APLListItem{potionEntry}},
	}
	for _, tc := range []struct {
		name      string
		playerbot bool
		talents   string
		potions   int
		earliest  time.Duration
		latest    time.Duration
	}{
		{"off", false, "", 1, -time.Second, -time.Second},
		// the rotation looks again every 50 ms while idle
		{"frost DK", true, "", 1, PlayerbotPotionAt, PlayerbotPotionAt + 50*time.Millisecond},
		{"blood tank", true, "005", 0, 0, 0},
	} {
		for _, rotation := range rotations {
			raid := playerbotTestRaid(tc.playerbot)
			raid.Parties[0].Players[0].TalentsString = tc.talents
			raid.Parties[0].Players[0].Rotation.PriorityList = rotation.priorityList
			sim := NewSim(&proto.RaidSimRequest{
				Raid:       raid,
				Encounter:  playerbotTestEncounter(),
				SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
			})
			var drunk []time.Duration
			for _, spell := range sim.Raid.Parties[0].Players[0].GetCharacter().Spellbook {
				if spell.Flags.Matches(SpellFlagPotion) {
					apply := spell.ApplyEffects
					spell.ApplyEffects = func(sim *Simulation, target *Unit, s *Spell) {
						if s.ActionID != (ActionID{ItemID: 40211}) {
							t.Errorf("%s, %s: drank %v, want Potion of Speed", tc.name, rotation.name, s.ActionID)
						}
						drunk = append(drunk, sim.CurrentTime)
						apply(sim, target, s)
					}
				}
			}
			for i := 0; i < 3; i++ {
				drunk = nil
				sim.runOnce()
				if len(drunk) != tc.potions || (tc.potions > 0 && (drunk[0] < tc.earliest || drunk[0] > tc.latest)) {
					t.Errorf("%s, %s, iteration %d: potions at %v, want %d in [%v, %v]",
						tc.name, rotation.name, i, drunk, tc.potions, tc.earliest, tc.latest)
				}
			}
		}
	}
}

// presetgen's and the rankings' in-scope builds, by class and the spec the matrix reads
func TestPlayerbotWorldBuffsCoverInScopeSpecs(t *testing.T) {
	for name, key := range map[string]playerbotSpec{
		"Retribution": {proto.Class_ClassPaladin, 2}, "Arms": {proto.Class_ClassWarrior, 0},
		"Fury": {proto.Class_ClassWarrior, 1}, "Affliction": {proto.Class_ClassWarlock, 0},
		"Demonology": {proto.Class_ClassWarlock, 1}, "Destruction": {proto.Class_ClassWarlock, 2},
		"Elemental": {proto.Class_ClassShaman, 0}, "Enhancement": {proto.Class_ClassShaman, 1},
		"Assassination": {proto.Class_ClassRogue, 0}, "Combat": {proto.Class_ClassRogue, 1},
		"Beast Mastery": {proto.Class_ClassHunter, 0}, "Marksmanship": {proto.Class_ClassHunter, 1},
		"Survival": {proto.Class_ClassHunter, 2}, "Balance": {proto.Class_ClassDruid, 0},
		"Feral": {proto.Class_ClassDruid, 3}, "Arcane": {proto.Class_ClassMage, 0},
		"Fire": {proto.Class_ClassMage, 1}, "DK Frost": {proto.Class_ClassDeathknight, 1},
		"DK Unholy 2H": {proto.Class_ClassDeathknight, 2}, "Shadow": {proto.Class_ClassPriest, 2},
	} {
		buffs := playerbotWorldBuffs[key]
		if buffs.GetFlask() == proto.Flask_FlaskUnknown || buffs.GetFood() == proto.Food_FoodUnknown {
			t.Errorf("%s: bot flask %v, food %v; rerun gen_server_defaults or check the matrix", name, buffs.GetFlask(), buffs.GetFood())
		}
	}
}
