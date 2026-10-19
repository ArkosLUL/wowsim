package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// botdpsSetupOf on a golden character's sim player gives back the server's side of goldchar_setups.json:
// the same items, enchants (socket ones included), gems, talent spells and glyphs.
func TestBotDpsSetupFollowsTheGoldenCharacters(t *testing.T) {
	needsItemDB(t)
	sim.RegisterAll()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range allGoldcharSetups(t) {
		player, err := goldcharPlayer(goldcharGeared(s))
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		ids := botdpsTestIDs(t)
		var glyphs []goldcharGlyph
		for _, g := range s.Glyphs {
			if g.Glyph != 0 {
				ids.glyphs[g.Spell] = g.Glyph
				glyphs = append(glyphs, g)
			}
		}
		got, _, err := botdpsSetupOf(root, s.Name, &proto.IndividualSimSettings{Player: player}, ids)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		want := botdpsSetup{Class: s.Class, Race: s.Race, Items: s.Items, Enchants: s.Enchants, Gems: s.Gems,
			TalentSpells: s.TalentSpells, Glyphs: glyphs}
		have := botdpsSetup{Class: got.Class, Race: got.Race, Items: got.Items, Enchants: got.Enchants, Gems: got.Gems,
			TalentSpells: got.TalentSpells, Glyphs: got.Glyphs}
		if !reflect.DeepEqual(have, want) {
			t.Errorf("%s:\n got %+v\nwant %+v", s.Name, have, want)
		}
		if got.Settings == "" {
			t.Errorf("%s: no settings", s.Name)
		}
	}
}

// botdpsTestIDs are the server ids botdpsSetupOf needs, without the DBC files: no GlyphProperties ids, and
// each pet's family named after the sim's pet.
func botdpsTestIDs(t *testing.T) botdpsServerIDs {
	t.Helper()
	glyphSpells, err := glyphItemSpells()
	if err != nil {
		t.Fatal(err)
	}
	ids := botdpsServerIDs{glyphs: map[int32]int32{}, glyphSpells: glyphSpells,
		petFamilies: map[proto.Hunter_Options_PetType]string{}}
	for _, pet := range petFamilies {
		ids.petFamilies[pet] = pet.String()
	}
	return ids
}

// The potion, the blessing and the professions follow the build: Wild Magic and Wisdom for a Fire mage,
// Speed and Might for an Assassination rogue, and each profession's skill line.
func TestBotDpsSetupPotionBlessingAndSkills(t *testing.T) {
	needsItemDB(t)
	sim.RegisterAll()
	root, err := checkoutRoot()
	if err != nil {
		t.Fatal(err)
	}
	setups := allGoldcharSetups(t)
	for _, c := range []struct {
		name   string
		potion int32
		caster bool
	}{{"MageFire", 40212, true}, {"RogueAssassination", 40211, false}} {
		i := slices.IndexFunc(setups, func(s goldcharSetup) bool { return s.Name == c.name })
		if i < 0 {
			t.Fatalf("no golden setup %s", c.name)
		}
		player, err := goldcharPlayer(setups[i])
		if err != nil {
			t.Fatal(err)
		}
		player.Professions = []proto.Profession{proto.Profession_Skinning, proto.Profession_Mining}
		got, _, err := botdpsSetupOf(root, c.name, &proto.IndividualSimSettings{Player: player}, botdpsTestIDs(t))
		if err != nil {
			t.Fatal(err)
		}
		if got.Potion != c.potion || got.Caster != c.caster || !slices.Equal(got.Skills, []int32{393, 186}) {
			t.Errorf("%s: potion %d, caster %v, skills %v; want %d, %v, [393 186]", c.name, got.Potion, got.Caster,
				got.Skills, c.potion, c.caster)
		}
	}
}

// The compare's sim has the Playerbot switch on, only the buffs the bot had, and the dummy for the fight's
// length, with no execute phase and unscaled by the page's raid.
func TestBotDpsRequest(t *testing.T) {
	settings := &proto.IndividualSimSettings{
		Player:    &proto.Player{Class: proto.Class_ClassMage, Race: proto.Race_RaceGnome},
		RaidBuffs: &proto.RaidBuffs{Bloodlust: true, GiftOfTheWild: proto.TristateEffect_TristateEffectImproved},
		Encounter: &proto.Encounter{RaidDifficulty: proto.RaidDifficulty_RaidDifficulty25Heroic},
	}
	request, err := botdpsRequest(settings, []string{"raid.arcane_brilliance", "debuff.sunder_armor"}, 181.5, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	player := request.Raid.Parties[0].Players[0]
	if !player.Playerbot || settings.Player.Playerbot {
		t.Errorf("Playerbot %v on the request, %v on the settings; want true, false", player.Playerbot, settings.Player.Playerbot)
	}
	raid := request.Raid.Buffs
	if !raid.ArcaneBrilliance || raid.Bloodlust || raid.GiftOfTheWild != proto.TristateEffect_TristateEffectMissing {
		t.Errorf("raid buffs %v, want Arcane Brilliance alone", raid)
	}
	if !request.Raid.Debuffs.SunderArmor || request.Raid.Debuffs.FaerieFire != proto.TristateEffect_TristateEffectMissing {
		t.Errorf("debuffs %v, want Sunder Armor alone", request.Raid.Debuffs)
	}
	e := request.Encounter
	target := e.Targets[0]
	if e.Duration != 181.5 || e.DurationVariation != 0 || e.ExecuteProportion_20 != 0 || e.ExecuteProportion_35 != 0 ||
		e.RaidDifficulty != proto.RaidDifficulty_RaidDifficultyUnknown {
		t.Errorf("encounter %v", e)
	}
	if target.Level != 83 || !target.GetWorldBoss() || target.MobType != proto.MobType_MobTypeGiant || !target.NoXp ||
		target.Stats[proto.Stat_StatArmor] != 10643 {
		t.Errorf("target %v, want the boss dummy", target)
	}
}

func TestReadBotDpsRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mage_fire_SvBdpabc_1792400000.setup.txt")
	text := "build: mage_fire\nplayer: SvBdpabc\npull: 1792400000123\nseconds: 180\n" +
		"buffs: raid.arcane_brilliance debuff.misery\nbuffs lost: raid.battle_shout\n\n" +
		"the bot before the pull (.simval info, as the target):\n  {\"a\": 1}\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	run, err := readBotDpsRun(path)
	if err != nil {
		t.Fatal(err)
	}
	want := botdpsRun{Build: "mage_fire", Player: "SvBdpabc", PullMs: 1792400000123, Seconds: 180,
		Buffs: []string{"raid.arcane_brilliance", "debuff.misery"}, Lost: []string{"raid.battle_shout"}}
	if !reflect.DeepEqual(run, want) {
		t.Errorf("got %+v, want %+v", run, want)
	}

	if err := os.WriteFile(path, []byte("build: mage_fire\nplayer: SvBdpabc\npull: 1792400000123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readBotDpsRun(path); err == nil {
		t.Error("a run without its seconds read without an error")
	}
}

// The bot's side counts from the pull for the fight's length: damage to the dummy alone, casts in the window,
// and an aura from the pull or its application to its removal or the end. A log that ends before the fight
// does is refused.
func TestMeasureBotDps(t *testing.T) {
	const text = `1789900000000  CHRONICLE_ZONE_INFO,"Naxxramas",533,42,"raid","NORMAL"
1789900000000  CHRONICLE_UNIT_INFO,0x1,"Testlock",80,0x511,0x0000000000000000,30000,"MINE",false
1789900000000  CHRONICLE_UNIT_INFO,0xA,"Dummy",83,0xa28,0x0000000000000000,24009944,"NEUTRAL",true
1789900000000  CHRONICLE_UNIT_INFO,0xB,"Maggot",80,0xa28,0x0000000000000000,9000,"NEUTRAL",false
1789900000000  CHRONICLE_UNIT_INFO,0x2,"Imp",80,0x1111,0x1,5000,"MINE",false
1789900005000  SPELL_CAST_SUCCESS,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47813,"Corruption",0x20
1789900005000  SPELL_AURA_APPLIED,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47813,"Corruption",0x20,DEBUFF
1789900008000  SPELL_PERIODIC_DAMAGE,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47813,"Corruption",0x20,300,0,32,0,0,0,nil,nil,nil
1789900011000  SPELL_PERIODIC_DAMAGE,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47813,"Corruption",0x20,300,0,32,0,0,0,nil,nil,nil
1789900012000  SPELL_CAST_SUCCESS,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47809,"Shadow Bolt",0x20
1789900012500  SWING_DAMAGE,0x2,"Imp",0x1111,0xA,"Dummy",0xa28,100,0,1,0,0,0,nil,nil,nil
1789900013000  SPELL_DAMAGE,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47809,"Shadow Bolt",0x20,2000,0,32,0,0,0,nil,nil,nil
1789900014000  SPELL_DAMAGE,0x1,"Testlock",0x511,0xB,"Maggot",0xa28,47809,"Shadow Bolt",0x20,500,0,32,0,0,0,nil,nil,nil
1789900015000  SPELL_AURA_REMOVED,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47813,"Corruption",0x20,DEBUFF
1789900016000  SPELL_AURA_APPLIED,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47811,"Immolate",0x4,DEBUFF
1789900021000  SPELL_DAMAGE,0x1,"Testlock",0x511,0xA,"Dummy",0xa28,47809,"Shadow Bolt",0x20,2000,0,32,0,0,0,nil,nil,nil
`
	log, err := parseChronicleLog("test", strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	run := botdpsRun{Player: "Testlock", PullMs: 1789900010000, Seconds: 10}
	got, err := measureBotDps(log, run)
	if err != nil {
		t.Fatal(err)
	}
	if got.BotDPS != 240 {
		t.Errorf("DPS %v, want 240: a tick, a Shadow Bolt and the Imp's swing on the dummy in the 10 s", got.BotDPS)
	}
	wantDamage := botdpsDamageMap{
		{ID: 47813}:                  {Amount: 300, Hits: 1, Direct: 300},
		{ID: 47809}:                  {Amount: 2000, Hits: 1, Direct: 2000},
		{Pet: true, ID: botdpsMelee}: {Amount: 100, Hits: 1, Direct: 100},
	}
	if !reflect.DeepEqual(got.BotDamage, wantDamage) {
		t.Errorf("damage %v, want %v", got.BotDamage, wantDamage)
	}
	if !reflect.DeepEqual(got.BotCasts, map[int32]int{47809: 1}) {
		t.Errorf("casts %v, want the one Shadow Bolt after the pull", got.BotCasts)
	}
	if !reflect.DeepEqual(got.BotUptime, map[int32]float64{47813: 50, 47811: 40}) {
		t.Errorf("uptimes %v, want Corruption 50%% and Immolate 40%%", got.BotUptime)
	}

	run.Seconds = 15
	if _, err := measureBotDps(log, run); err == nil {
		t.Error("a log ending 11 s into a 15 s fight measured without an error")
	}
}

// The sim's casts are each spell's lowest tag, and a channel's casts, booked under the spell its ticks cast
// with the tick count as the tag, all go to the channel.
func TestBotDpsSimCasts(t *testing.T) {
	action := func(id, tag, casts int32) *proto.ActionMetrics {
		return &proto.ActionMetrics{Id: &proto.ActionID{RawId: &proto.ActionID_SpellId{SpellId: id}, Tag: tag},
			Targets: []*proto.TargetedActionMetrics{{Casts: casts}}}
	}
	player := &proto.UnitMetrics{
		Actions: []*proto.ActionMetrics{action(42891, 1, 40), action(42891, 0, 20), action(58381, 0, 60),
			action(58381, 2, 20), action(58381, 1, 10)},
		Pets: []*proto.UnitMetrics{{Actions: []*proto.ActionMetrics{action(52474, 0, 100)}}},
	}
	want := map[int32]float64{42891: 2, 48156: 9, 52474: 10}
	if got := botdpsSimCasts(player, 10); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// The sim's damage is every tag of a spell together, the player's and its pets' apart, with melee, Shoot and
// item procs under their own sources and actions that did no damage left out. Hits are every landed outcome,
// and a tag that lands none adds no damage to theirs.
func TestBotDpsSimDamage(t *testing.T) {
	action := func(id *proto.ActionID, damage float64, hits int32) *proto.ActionMetrics {
		return &proto.ActionMetrics{Id: id, Targets: []*proto.TargetedActionMetrics{{Damage: damage, Hits: hits}}}
	}
	spell := func(id, tag int32) *proto.ActionID {
		return &proto.ActionID{RawId: &proto.ActionID_SpellId{SpellId: id}, Tag: tag}
	}
	other := func(id proto.OtherAction) *proto.ActionID {
		return &proto.ActionID{RawId: &proto.ActionID_OtherId{OtherId: id}}
	}
	melee := action(other(proto.OtherAction_OtherActionAttack), 1000, 6)
	melee.Targets[0].Crits, melee.Targets[0].Glances = 2, 2
	player := &proto.UnitMetrics{
		Actions: []*proto.ActionMetrics{action(spell(42891, 1), 300, 3), action(spell(42891, 2), 100, 0),
			action(other(proto.OtherAction_OtherActionShoot), 200, 2), action(spell(54758, 0), 0, 0),
			action(&proto.ActionID{RawId: &proto.ActionID_ItemId{ItemId: 40255}}, 50, 1)},
		Pets: []*proto.UnitMetrics{{Actions: []*proto.ActionMetrics{melee, action(spell(52474, 0), 400, 4)}}},
	}
	want := botdpsDamageMap{
		{ID: 42891}:                  {Amount: 40, Hits: 0.3, Direct: 30},
		{ID: botdpsRanged}:           {Amount: 20, Hits: 0.2, Direct: 20},
		{ID: botdpsItems}:            {Amount: 5, Hits: 0.1, Direct: 5},
		{Pet: true, ID: botdpsMelee}: {Amount: 100, Hits: 1, Direct: 100, Crits: 0.2},
		{Pet: true, ID: 52474}:       {Amount: 40, Hits: 0.4, Direct: 40},
	}
	if got := botdpsSimDamage(player, 10); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A build more than 5 points below the median bot/sim ratio is flagged, one within them isn't.
func TestBotDpsSummaryFlagsTheLaggards(t *testing.T) {
	results := []botdpsResult{
		{Run: botdpsRun{Build: "a"}, BotDPS: 100, SimDPS: 100},
		{Run: botdpsRun{Build: "b"}, BotDPS: 96, SimDPS: 100},
		{Run: botdpsRun{Build: "c"}, BotDPS: 90, SimDPS: 100},
	}
	var out bytes.Buffer
	reportBotDpsSummary(&out, results)
	for _, line := range strings.Split(out.String(), "\n") {
		flagged := strings.Contains(line, "below the median")
		if strings.HasPrefix(line, "c ") != flagged {
			t.Errorf("%q", line)
		}
	}
	if !strings.Contains(out.String(), "median bot/sim 96.0%") {
		t.Errorf("no median 96.0%% in\n%s", out.String())
	}
}
