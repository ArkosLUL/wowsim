package raidctx

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	goproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var update = flag.Bool("update", false, "rewrite testdata/raid25.derived.json")

func init() {
	sim.RegisterAll()
}

var specClass = map[proto.Spec]proto.Class{
	proto.Spec_SpecBalanceDruid:       proto.Class_ClassDruid,
	proto.Spec_SpecFeralDruid:         proto.Class_ClassDruid,
	proto.Spec_SpecFeralTankDruid:     proto.Class_ClassDruid,
	proto.Spec_SpecRestorationDruid:   proto.Class_ClassDruid,
	proto.Spec_SpecElementalShaman:    proto.Class_ClassShaman,
	proto.Spec_SpecEnhancementShaman:  proto.Class_ClassShaman,
	proto.Spec_SpecRestorationShaman:  proto.Class_ClassShaman,
	proto.Spec_SpecHunter:             proto.Class_ClassHunter,
	proto.Spec_SpecMage:               proto.Class_ClassMage,
	proto.Spec_SpecHolyPaladin:        proto.Class_ClassPaladin,
	proto.Spec_SpecProtectionPaladin:  proto.Class_ClassPaladin,
	proto.Spec_SpecRetributionPaladin: proto.Class_ClassPaladin,
	proto.Spec_SpecRogue:              proto.Class_ClassRogue,
	proto.Spec_SpecHealingPriest:      proto.Class_ClassPriest,
	proto.Spec_SpecShadowPriest:       proto.Class_ClassPriest,
	proto.Spec_SpecSmitePriest:        proto.Class_ClassPriest,
	proto.Spec_SpecWarlock:            proto.Class_ClassWarlock,
	proto.Spec_SpecWarrior:            proto.Class_ClassWarrior,
	proto.Spec_SpecProtectionWarrior:  proto.Class_ClassWarrior,
	proto.Spec_SpecDeathknight:        proto.Class_ClassDeathknight,
	proto.Spec_SpecTankDeathknight:    proto.Class_ClassDeathknight,
}

var classRace = map[proto.Class]proto.Race{
	proto.Class_ClassDruid:       proto.Race_RaceNightElf,
	proto.Class_ClassShaman:      proto.Race_RaceDraenei,
	proto.Class_ClassHunter:      proto.Race_RaceDwarf,
	proto.Class_ClassMage:        proto.Race_RaceGnome,
	proto.Class_ClassPaladin:     proto.Race_RaceHuman,
	proto.Class_ClassRogue:       proto.Race_RaceHuman,
	proto.Class_ClassPriest:      proto.Race_RaceHuman,
	proto.Class_ClassWarlock:     proto.Race_RaceHuman,
	proto.Class_ClassWarrior:     proto.Race_RaceHuman,
	proto.Class_ClassDeathknight: proto.Race_RaceHuman,
}

// specField finds the Player.spec oneof field core maps to spec.
func specField(spec proto.Spec) protoreflect.FieldDescriptor {
	fields := (&proto.Player{}).ProtoReflect().Descriptor().Oneofs().ByName("spec").Fields()
	for i := 0; i < fields.Len(); i++ {
		p := &proto.Player{}
		p.ProtoReflect().Set(fields.Get(i), p.ProtoReflect().NewField(fields.Get(i)))
		if core.PlayerProtoToSpec(p) == spec {
			return fields.Get(i)
		}
	}
	panic(fmt.Sprintf("no Player.spec field for %s", spec))
}

// newPlayer makes a naked player with empty spec options and these talents, by proto field name.
func newPlayer(name string, spec proto.Spec, talents map[string]int32) *proto.Player {
	class := specClass[spec]
	p := &proto.Player{Name: name, Class: class, Race: classRace[class], Equipment: &proto.EquipmentSpec{}}
	m := p.ProtoReflect()
	fd := specField(spec)
	specMsg := m.NewField(fd).Message()
	options := specMsg.Descriptor().Fields().ByName("options")
	specMsg.Set(options, specMsg.NewField(options))
	m.Set(fd, protoreflect.ValueOfMessage(specMsg))
	p.TalentsString = talentString(class, talents)
	return p
}

// talentString writes talents the way core.FillTalentsProto reads them.
func talentString(class proto.Class, talents map[string]int32) string {
	tree := talentTrees[class]
	m := tree.new()
	for name, points := range talents {
		fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
		if fd == nil {
			panic(fmt.Sprintf("%s has no talent %s", class, name))
		}
		if fd.Kind() == protoreflect.BoolKind {
			m.Set(fd, protoreflect.ValueOfBool(points > 0))
		} else {
			m.Set(fd, protoreflect.ValueOfInt32(points))
		}
	}
	var trees []string
	offset := 0
	for _, size := range tree.sizes {
		digits := make([]byte, size)
		for i := range digits {
			fd := m.Descriptor().Fields().ByNumber(protowire.Number(offset + i + 1))
			digits[i] = byte('0' + talentPoints(m, string(fd.Name())))
		}
		trees = append(trees, strings.TrimRight(string(digits), "0"))
		offset += size
	}
	return strings.TrimRight(strings.Join(trees, "-"), "-")
}

func playerRef(index int32) *proto.UnitReference {
	return &proto.UnitReference{Type: proto.UnitReference_Player, Index: index}
}

// raidOf seats the players five to a party, in order; nil leaves a slot empty.
func raidOf(players ...*proto.Player) *proto.RaidSimRequest {
	raid := &proto.Raid{}
	for i, p := range players {
		if i%5 == 0 {
			raid.Parties = append(raid.Parties, &proto.Party{})
		}
		if p == nil {
			p = &proto.Player{}
		}
		party := raid.Parties[len(raid.Parties)-1]
		party.Players = append(party.Players, p)
	}
	return &proto.RaidSimRequest{
		Raid: raid,
		Encounter: &proto.Encounter{
			Duration: 180,
			Targets:  []*proto.Target{{Level: 83, MobType: proto.MobType_MobTypeGiant}},
		},
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, IsTest: true},
	}
}

func mustDerive(t *testing.T, base *proto.RaidSimRequest, targetIndex int) *proto.Raid {
	t.Helper()
	before := goproto.Clone(base)
	derived, err := Derive(base, targetIndex)
	if err != nil {
		t.Fatal(err)
	}
	if !goproto.Equal(before, base) {
		t.Fatal("Derive changed its input")
	}
	if n := len(derived.Raid.Parties); n != 1 || len(derived.Raid.Parties[0].Players) != 1 {
		t.Fatalf("derived raid has %d parties, want the target alone in one", n)
	}
	return derived.Raid
}

func TestTalentStringRoundTrips(t *testing.T) {
	want := map[string]int32{"blood_frenzy": 2, "trauma": 2, "commanding_presence": 5, "vigilance": 1}
	talents := parseTalents(&proto.Player{Class: proto.Class_ClassWarrior, TalentsString: talentString(proto.Class_ClassWarrior, want)})
	for name, points := range want {
		if got := talentPoints(talents, name); got != points {
			t.Errorf("%s: %d points, want %d", name, got, points)
		}
	}
}

func TestDeriveTakesNothingFromTheTarget(t *testing.T) {
	warrior := func(name string) *proto.Player {
		p := newPlayer(name, proto.Spec_SpecWarrior, map[string]int32{"blood_frenzy": 2, "rampage": 1})
		p.GetWarrior().Options.Shout = proto.WarriorShout_WarriorShoutBattle
		return p
	}

	alone := mustDerive(t, raidOf(warrior("A")), 0)
	if alone.Debuffs.BloodFrenzy || alone.Debuffs.SunderArmor || alone.Buffs.Rampage || alone.Buffs.BattleShout != 0 {
		t.Errorf("a raider alone got its own effects: debuffs %v, buffs %v", alone.Debuffs, alone.Buffs)
	}

	pair := mustDerive(t, raidOf(warrior("A"), warrior("B")), 0)
	if !pair.Debuffs.BloodFrenzy || !pair.Debuffs.SunderArmor || !pair.Buffs.Rampage || pair.Buffs.BattleShout != regular {
		t.Errorf("the other warrior's effects are missing: debuffs %v, buffs %v", pair.Debuffs, pair.Buffs)
	}
}

func TestDeriveKeepsBaseBuffsAndBlessings(t *testing.T) {
	target := newPlayer("Target", proto.Spec_SpecWarrior, nil)
	target.Buffs = &proto.IndividualBuffs{BlessingOfKings: true, Innervates: 1}
	base := raidOf(target, newPlayer("Mage", proto.Spec_SpecMage, nil))
	base.Raid.Buffs = &proto.RaidBuffs{DrumsOfTheWild: true}
	base.Raid.Debuffs = &proto.Debuffs{JudgementOfWisdom: true}
	base.Raid.Parties[0].Buffs = &proto.PartyBuffs{BraidedEterniumChain: true}

	raid := mustDerive(t, base, 0)
	if !raid.Buffs.DrumsOfTheWild || !raid.Buffs.ArcaneBrilliance {
		t.Errorf("raid buffs %v, want base's drums and the mage's Arcane Brilliance", raid.Buffs)
	}
	if !raid.Debuffs.JudgementOfWisdom {
		t.Errorf("debuffs %v lost base's Judgement of Wisdom", raid.Debuffs)
	}
	if !raid.Parties[0].Buffs.BraidedEterniumChain {
		t.Errorf("party buffs %v lost base's", raid.Parties[0].Buffs)
	}
	if buffs := raid.Parties[0].Players[0].Buffs; !buffs.BlessingOfKings || buffs.Innervates != 1 {
		t.Errorf("the target's own buffs %v didn't carry over", buffs)
	}
}

func TestDerivePartyBuffsComeFromTheTargetsParty(t *testing.T) {
	draenei := func() *proto.Player {
		p := newPlayer("Draenei", proto.Spec_SpecElementalShaman, nil)
		p.Race = proto.Race_RaceDraenei
		return p
	}
	target := newPlayer("Target", proto.Spec_SpecWarrior, nil)

	same := mustDerive(t, raidOf(target, draenei()), 0)
	if !same.Parties[0].Buffs.HeroicPresence {
		t.Error("a Draenei in the target's party didn't give Heroic Presence")
	}
	other := mustDerive(t, raidOf(target, nil, nil, nil, nil, draenei()), 0)
	if other.Parties[0].Buffs.GetHeroicPresence() {
		t.Error("a Draenei in another party gave Heroic Presence")
	}
	if !other.Buffs.Bloodlust {
		t.Error("a shaman in another party didn't give Bloodlust")
	}
}

func TestDeriveTargetedBuffs(t *testing.T) {
	const target = 6
	priest := func(powerInfusion bool, on int32) *proto.Player {
		talents := map[string]int32{}
		if powerInfusion {
			talents["power_infusion"] = 1
		}
		p := newPlayer("Priest", proto.Spec_SpecSmitePriest, talents)
		p.GetSmitePriest().Options.PowerInfusionTarget = playerRef(on)
		return p
	}
	rogue := func(on int32) *proto.Player {
		p := newPlayer("Rogue", proto.Spec_SpecRogue, nil)
		p.GetRogue().Options.TricksOfTheTradeTarget = playerRef(on)
		return p
	}
	mage := newPlayer("Mage", proto.Spec_SpecMage, map[string]int32{"focus_magic": 1})
	mage.GetMage().Options.FocusMagicTarget = playerRef(target)
	druid := newPlayer("Druid", proto.Spec_SpecRestorationDruid, nil)
	druid.GetRestorationDruid().Options.InnervateTarget = playerRef(target)
	dk := func(hysteria bool) *proto.Player {
		talents := map[string]int32{}
		if hysteria {
			talents["hysteria"] = 1
		}
		p := newPlayer("DK", proto.Spec_SpecDeathknight, talents)
		p.GetDeathknight().Options.UnholyFrenzyTarget = playerRef(target)
		return p
	}

	base := raidOf(
		priest(true, target), priest(true, 3), priest(false, target), rogue(target), rogue(target),
		mage, newPlayer("Target", proto.Spec_SpecWarrior, nil), druid, dk(true), dk(false),
	)
	got := mustDerive(t, base, target).Parties[0].Players[0].Buffs
	want := &proto.IndividualBuffs{PowerInfusions: 1, TricksOfTheTrades: 2, FocusMagic: true, Innervates: 1, UnholyFrenzy: 1}
	if !goproto.Equal(got, want) {
		t.Errorf("targeted buffs %v, want %v", got, want)
	}
}

func TestDeriveMovesReferencesWithTheTarget(t *testing.T) {
	const target = 7
	balance := newPlayer("Balance", proto.Spec_SpecBalanceDruid, nil)
	balance.GetBalanceDruid().Options.InnervateTarget = playerRef(target)
	balance.Rotation = &proto.APLRotation{PriorityList: []*proto.APLListItem{
		{Action: &proto.APLAction{Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{
			SpellId: &proto.ActionID{RawId: &proto.ActionID_SpellId{SpellId: 29166}},
			Target:  playerRef(0),
		}}}},
		{Action: &proto.APLAction{Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{
			SpellId: &proto.ActionID{RawId: &proto.ActionID_SpellId{SpellId: 29166}},
			Target:  &proto.UnitReference{Type: proto.UnitReference_Pet, Owner: playerRef(target)},
		}}}},
	}}
	players := make([]*proto.Player, 8)
	for i := range players {
		players[i] = newPlayer(fmt.Sprint("Warrior", i), proto.Spec_SpecWarrior, nil)
	}
	players[target] = balance
	base := raidOf(players...)
	base.Raid.Tanks = []*proto.UnitReference{playerRef(0), playerRef(target), {Type: proto.UnitReference_Target}}
	base.Raid.TargetDummies = 2
	base.Encounter.Targets[0].TankIndex = 1

	derived, err := Derive(base, target)
	if err != nil {
		t.Fatal(err)
	}
	raid := derived.Raid
	options := raid.Parties[0].Players[0].GetBalanceDruid().Options
	if !goproto.Equal(options.InnervateTarget, playerRef(TargetIndex)) {
		t.Errorf("self Innervate points at %v, want the target's new index", options.InnervateTarget)
	}
	actions := raid.Parties[0].Players[0].Rotation.PriorityList
	if ref := actions[0].Action.GetCastSpell().Target; !goproto.Equal(ref, &proto.UnitReference{}) {
		t.Errorf("a reference to another raider became %v, want it cleared", ref)
	}
	if ref := actions[1].Action.GetCastSpell().Target; !goproto.Equal(ref.Owner, playerRef(TargetIndex)) {
		t.Errorf("the target's pet reference became %v, want its owner moved", ref)
	}
	wantTanks := []*proto.UnitReference{{}, playerRef(TargetIndex), {Type: proto.UnitReference_Target}}
	for i := range wantTanks {
		if !goproto.Equal(raid.Tanks[i], wantTanks[i]) {
			t.Errorf("tank %d is %v, want %v", i, raid.Tanks[i], wantTanks[i])
		}
	}
	if derived.Encounter.Targets[0].TankIndex != 1 {
		t.Error("the encounter's tank index moved; the tank list keeps its positions instead")
	}
	if raid.NumActiveParties != 1 || raid.TargetDummies != 0 {
		t.Errorf("derived raid has %d active parties and %d dummies, want 1 and 0", raid.NumActiveParties, raid.TargetDummies)
	}

	// the derived raid must build: the boss has to find its tank at the new index
	env, _, _ := core.NewEnvironment(goproto.Clone(raid).(*proto.Raid), goproto.Clone(derived.Encounter).(*proto.Encounter), false)
	if env.Encounter.Targets[0].CurrentTarget != &env.Raid.Parties[0].Players[0].GetCharacter().Unit {
		t.Error("the boss doesn't tank on the target in the derived raid")
	}
}

func TestDeriveIgnoresTheBench(t *testing.T) {
	base := raidOf(newPlayer("Target", proto.Spec_SpecWarrior, nil), nil, nil, nil, nil,
		newPlayer("Bench", proto.Spec_SpecElementalShaman, nil))
	base.Raid.NumActiveParties = 1
	if mustDerive(t, base, 0).Buffs.Bloodlust {
		t.Error("a benched shaman gave Bloodlust")
	}
	if _, err := Derive(base, 5); err == nil {
		t.Error("deriving for a benched raider didn't fail")
	}
}

func TestDeriveRejectsBadTargets(t *testing.T) {
	base := raidOf(newPlayer("Target", proto.Spec_SpecWarrior, nil), nil)
	for _, index := range []int{-1, 1, 4, 5} {
		if _, err := Derive(base, index); err == nil {
			t.Errorf("raid index %d: no error", index)
		}
	}
	if _, err := Derive(&proto.RaidSimRequest{}, 0); err == nil {
		t.Error("no raid: no error")
	}
	broken := raidOf(newPlayer("Target", proto.Spec_SpecWarrior, nil), newPlayer("Broken", proto.Spec_SpecWarrior, nil))
	broken.Raid.Parties[0].Players[1].Spec = nil
	if _, err := Derive(broken, 0); err == nil {
		t.Error("a raider core can't build: no error")
	}
}

func TestDemonicPactSP(t *testing.T) {
	for _, c := range []struct {
		spellPower float64
		points     int32
		want       int32
	}{{1000, 5, 100}, {2884, 5, 288}, {2885, 5, 289}, {2000, 3, 120}, {3000, 0, 0}} {
		if got := DemonicPactSP(c.spellPower, c.points); got != c.want {
			t.Errorf("DemonicPactSP(%v, %d) = %d, want %d", c.spellPower, c.points, got, c.want)
		}
	}
}

func TestDeriveDemonicPact(t *testing.T) {
	warlock := func(summon proto.Warlock_Options_Summon) *proto.Player {
		p := newPlayer("Warlock", proto.Spec_SpecWarlock, map[string]int32{"demonic_pact": 5})
		p.GetWarlock().Options.Summon = summon
		p.GetWarlock().Options.Armor = proto.Warlock_Options_FelArmor
		return p
	}
	target := newPlayer("Target", proto.Spec_SpecMage, nil)

	base := raidOf(target, warlock(proto.Warlock_Options_Felguard))
	result := core.ComputeStats(&proto.ComputeStatsRequest{Raid: goproto.Clone(base.Raid).(*proto.Raid)})
	sheet := result.RaidStats.Parties[0].Players[1].FinalStats.Stats[stats.SpellPower]
	want := DemonicPactSP(sheet, 5)
	if want == 0 {
		t.Fatal("the warlock's sheet has no spell power to test with")
	}
	if got := mustDerive(t, base, 0).Buffs.DemonicPactSp; got != want {
		t.Errorf("Demonic Pact gives %d spell power, want %d from the warlock's sheet %.0f", got, want, sheet)
	}

	if got := mustDerive(t, raidOf(target, warlock(proto.Warlock_Options_NoSummon)), 0).Buffs.DemonicPactSp; got != 0 {
		t.Errorf("a warlock without a pet gave %d Demonic Pact spell power", got)
	}
	if got := mustDerive(t, base, 1).Buffs.DemonicPactSp; got != 0 {
		t.Errorf("the target warlock gave itself %d Demonic Pact spell power", got)
	}
	base.Raid.Buffs = &proto.RaidBuffs{DemonicPactSp: 5000}
	if got := mustDerive(t, base, 0).Buffs.DemonicPactSp; got != 5000 {
		t.Errorf("a stronger Demonic Pact in base's raid buffs became %d", got)
	}
}

func loadFixture(t *testing.T) *proto.RaidSimRequest {
	t.Helper()
	if !core.WITH_DB {
		t.Skip("the fixture's gear needs the with_db item data")
	}
	data, err := os.ReadFile("testdata/raid25.json")
	if err != nil {
		t.Fatal(err)
	}
	req := &proto.RaidSimRequest{}
	if err := protojson.Unmarshal(data, req); err != nil {
		t.Fatal(err)
	}
	return req
}

// TestDeriveFixtureGolden derives every raider of the 25-player fixture, which is built from the
// ui/<spec> P1 presets and the talents in the spec tests. Rewrite the golden with
// go test --tags=with_db ./sim/optimizer/raidctx -run TestDeriveFixtureGolden -update
func TestDeriveFixtureGolden(t *testing.T) {
	base := loadFixture(t)
	before := goproto.Clone(base)

	type flags struct {
		Index      int    `json:"index"`
		Name       string `json:"name"`
		RaidBuffs  any    `json:"raidBuffs"`
		PartyBuffs any    `json:"partyBuffs"`
		Debuffs    any    `json:"debuffs"`
		Individual any    `json:"individualBuffs"`
		Tanks      any    `json:"tanks"`
	}
	var all []flags
	for _, r := range activePlayers(base.Raid, activeParties(base.Raid)) {
		derived, err := Derive(base, r.index)
		if err != nil {
			t.Fatalf("%s: %v", r.player.Name, err)
		}
		raid := derived.Raid
		core.NewEnvironment(goproto.Clone(raid).(*proto.Raid), goproto.Clone(derived.Encounter).(*proto.Encounter), false)
		tanks := &proto.Raid{Tanks: raid.Tanks}
		all = append(all, flags{
			Index:      r.index,
			Name:       r.player.Name,
			RaidBuffs:  plainJSON(t, raid.Buffs),
			PartyBuffs: plainJSON(t, raid.Parties[0].Buffs),
			Debuffs:    plainJSON(t, raid.Debuffs),
			Individual: plainJSON(t, raid.Parties[0].Players[0].Buffs),
			Tanks:      plainJSON(t, tanks).(map[string]any)["tanks"],
		})
	}
	if !goproto.Equal(before, base) {
		t.Fatal("Derive changed the fixture")
	}

	got, err := json.MarshalIndent(all, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const golden = "testdata/raid25.derived.json"
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		gotLines, wantLines := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
		for i := 0; i < min(len(gotLines), len(wantLines)); i++ {
			if gotLines[i] != wantLines[i] {
				t.Fatalf("%s differs from line %d: got %q, want %q; rerun with -update if that's expected", golden, i+1, gotLines[i], wantLines[i])
			}
		}
		t.Fatalf("%s has %d lines, got %d; rerun with -update if that's expected", golden, len(wantLines), len(gotLines))
	}
}

// plainJSON is m's protojson with stable formatting: protojson randomizes its whitespace.
func plainJSON(t *testing.T, m goproto.Message) any {
	t.Helper()
	data, err := protojson.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
