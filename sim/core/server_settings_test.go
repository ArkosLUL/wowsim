package core

import (
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var raidDifficulties = []proto.RaidDifficulty{
	proto.RaidDifficulty_RaidDifficulty10Normal,
	proto.RaidDifficulty_RaidDifficulty25Normal,
	proto.RaidDifficulty_RaidDifficulty10Heroic,
	proto.RaidDifficulty_RaidDifficulty25Heroic,
}

func modifiers(global, health, armor, damage float64) *proto.DungeonScaleModifiers {
	return &proto.DungeonScaleModifiers{
		Global: googleProto.Float64(global),
		Health: googleProto.Float64(health),
		Armor:  googleProto.Float64(armor),
		Damage: googleProto.Float64(damage),
	}
}

// isSizeSet says whether a DungeonScaleSettings field is a 10 or 25 player set, which the live
// config usually leaves unset.
func isSizeSet(fd protoreflect.FieldDescriptor) bool {
	return fd.ContainingMessage().Name() == "DungeonScaleSettings" &&
		(strings.HasPrefix(string(fd.Name()), "raid10") || strings.HasPrefix(string(fd.Name()), "raid25"))
}

func TestLiveServerDefaultsSetEveryField(t *testing.T) {
	var check func(m protoreflect.Message)
	check = func(m protoreflect.Message) {
		fields := m.Descriptor().Fields()
		for i := 0; i < fields.Len(); i++ {
			fd := fields.Get(i)
			if isSizeSet(fd) {
				continue
			}
			if !m.Has(fd) {
				t.Errorf("LiveServerDefaults leaves %s unset", fd.FullName())
				continue
			}
			if fd.Kind() == protoreflect.MessageKind && !fd.IsList() {
				check(m.Get(fd).Message())
			}
		}
	}
	check(LiveServerDefaults().ProtoReflect())
}

func TestMissingServerSettingsMeanLive(t *testing.T) {
	live := LiveServerDefaults()
	want := NewServerSettings(live)

	for name, settings := range map[string]*proto.ServerSettings{
		"nil":         nil,
		"empty":       {},
		"empty parts": {SpellTweaks: &proto.SpellTweaksSettings{}, DungeonScale: &proto.DungeonScaleSettings{}, Reforge: &proto.ReforgeSettings{}},
	} {
		if got := NewServerSettings(settings); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %+v, want the live settings %+v", name, got, want)
		}
	}

	got := NewServerSettings(nil)
	if want := time.Duration(live.GetMapUpdateIntervalMs()) * time.Millisecond; got.MapUpdateInterval != want {
		t.Errorf("MapUpdateInterval = %v, want %v", got.MapUpdateInterval, want)
	}
	if got.SpellTweaks.Enabled != live.SpellTweaks.GetEnable() || got.SpellTweaks.DiseaseHaste != (live.SpellTweaks.GetEnable() && live.SpellTweaks.GetDiseaseHaste()) {
		t.Errorf("SpellTweaks = %+v, want the live toggles %v", got.SpellTweaks, live.SpellTweaks)
	}
	if got.Reforging.Percentage != live.Reforge.GetPercentage() || !slices.Equal(got.Reforging.StatTypes, live.Reforge.StatTypes) {
		t.Errorf("Reforging = %+v, want %v", got.Reforging, live.Reforge)
	}
	if liveSetsSizeKeys(live) {
		return // TestDungeonScaleSizeSetsFallBack covers those
	}
	for _, difficulty := range raidDifficulties {
		generic := live.DungeonScale.GetRaidBoss()
		if difficulty == proto.RaidDifficulty_RaidDifficulty10Heroic || difficulty == proto.RaidDifficulty_RaidDifficulty25Heroic {
			generic = live.DungeonScale.GetRaidHeroicBoss()
		}
		if health := got.DungeonScale.Modifiers(difficulty, true).Health; health != generic.GetHealth() {
			t.Errorf("%s boss health = %v, want the live %v", difficulty, health, generic.GetHealth())
		}
	}
}

func liveSetsSizeKeys(live *proto.ServerSettings) bool {
	m := live.DungeonScale.ProtoReflect()
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		if isSizeSet(fields.Get(i)) && m.Has(fields.Get(i)) {
			return true
		}
	}
	return false
}

func TestServerSettingOverridesOnlyItsField(t *testing.T) {
	want := *NewServerSettings(nil)
	want.SpellTweaks.DiseaseHaste = false
	want.MapUpdateInterval = 0

	got := NewServerSettings(&proto.ServerSettings{
		MapUpdateIntervalMs: googleProto.Int32(0),
		SpellTweaks:         &proto.SpellTweaksSettings{DiseaseHaste: googleProto.Bool(false)},
	})
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("got %+v, want %+v", *got, want)
	}
}

func TestMapUpdateInterval(t *testing.T) {
	for _, tc := range []struct {
		ms   int32
		want time.Duration
	}{
		{0, 0}, // exact timing for unit tests
		{-5, 0},
		{50, 50 * time.Millisecond},
	} {
		got := NewServerSettings(&proto.ServerSettings{MapUpdateIntervalMs: googleProto.Int32(tc.ms)}).MapUpdateInterval
		if got != tc.want {
			t.Errorf("%d ms: got %v, want %v", tc.ms, got, tc.want)
		}
	}
}

func TestSpellTweaksEnableGatesScriptedTweaks(t *testing.T) {
	on := googleProto.Bool(true)
	allOn := &proto.SpellTweaksSettings{
		Enable: on, ExoticPetDamagePct: googleProto.Float64(10), HunterPetHaste: on, HunterPetArmorPen: on,
		DkGhoulArmorPen: on, FeralSpiritHaste: on, DiseaseHaste: on, DeadlyPoisonMurder: on,
		RuptureWeaponExpertise: on, RendTrauma: on, BalanceDotScaling: on, TitansGripNoDamagePenalty: on,
		OmenClarityFaerieFire: on,
	}

	got := NewServerSettings(&proto.ServerSettings{SpellTweaks: allOn}).SpellTweaks
	if want := float64(float32(1.1)); got.ExoticPetDamageMultiplier != want {
		t.Errorf("exotic pet multiplier at 10%% = %v, want %v", got.ExoticPetDamageMultiplier, want)
	}

	off := googleProto.Clone(allOn).(*proto.SpellTweaksSettings)
	off.Enable = googleProto.Bool(false)
	got = NewServerSettings(&proto.ServerSettings{SpellTweaks: off}).SpellTweaks
	want := SpellTweaks{ExoticPetDamageMultiplier: 1, HunterPetArmorPen: true, DKGhoulArmorPen: true}
	if got != want {
		t.Errorf("with SpellTweaks.Enable off got %+v, want only the armor pen tweaks %+v", got, want)
	}

	negative := googleProto.Clone(allOn).(*proto.SpellTweaksSettings)
	negative.ExoticPetDamagePct = googleProto.Float64(-5)
	if got := NewServerSettings(&proto.ServerSettings{SpellTweaks: negative}).SpellTweaks.ExoticPetDamageMultiplier; got != 1 {
		t.Errorf("exotic pet multiplier at -5%% = %v, want 1", got)
	}
}

func TestDungeonScaleSizeSetsFallBack(t *testing.T) {
	live := &proto.ServerSettings{DungeonScale: &proto.DungeonScaleSettings{
		Raid:           modifiers(1, 1, 1, 1),
		RaidBoss:       modifiers(1, 1.2, 1, 1),
		RaidHeroic:     modifiers(1, 1, 1, 1),
		RaidHeroicBoss: modifiers(1, 1.3, 1, 1),
		Raid25Boss:     &proto.DungeonScaleModifiers{Health: googleProto.Float64(1.5)},
	}}
	const n10, n25, h10, h25 = proto.RaidDifficulty_RaidDifficulty10Normal, proto.RaidDifficulty_RaidDifficulty25Normal,
		proto.RaidDifficulty_RaidDifficulty10Heroic, proto.RaidDifficulty_RaidDifficulty25Heroic

	for _, tc := range []struct {
		name       string
		settings   *proto.DungeonScaleSettings
		difficulty proto.RaidDifficulty
		boss       bool
		want       DungeonScaleModifiers
	}{
		{"generic raid boss", nil, n10, true, DungeonScaleModifiers{1, 1.2, 1, 1}},
		{"live size key", nil, n25, true, DungeonScaleModifiers{1, 1.5, 1, 1}},
		{"heroic takes raid heroic", nil, h10, true, DungeonScaleModifiers{1, 1.3, 1, 1}},
		{"heroic 25 skips the normal size key", nil, h25, true, DungeonScaleModifiers{1, 1.3, 1, 1}},
		{"creature", nil, n25, false, DungeonScaleModifiers{1, 1, 1, 1}},
		{"unknown reads as 25 normal", nil, proto.RaidDifficulty_RaidDifficultyUnknown, true, DungeonScaleModifiers{1, 1.5, 1, 1}},
		{"setting the generic moves the unset size sets",
			&proto.DungeonScaleSettings{RaidBoss: &proto.DungeonScaleModifiers{Health: googleProto.Float64(2)}}, n10, true, DungeonScaleModifiers{1, 2, 1, 1}},
		{"a live size key still beats the generic",
			&proto.DungeonScaleSettings{RaidBoss: &proto.DungeonScaleModifiers{Health: googleProto.Float64(2)}}, n25, true, DungeonScaleModifiers{1, 1.5, 1, 1}},
		{"size setting",
			&proto.DungeonScaleSettings{Raid25Boss: &proto.DungeonScaleModifiers{Health: googleProto.Float64(3)}}, n25, true, DungeonScaleModifiers{1, 3, 1, 1}},
		{"boss values don't reach creatures",
			&proto.DungeonScaleSettings{RaidBoss: &proto.DungeonScaleModifiers{Armor: googleProto.Float64(0.5)}}, n10, false, DungeonScaleModifiers{1, 1, 1, 1}},
		{"creature values don't reach bosses",
			&proto.DungeonScaleSettings{Raid: &proto.DungeonScaleModifiers{Damage: googleProto.Float64(2)}}, n10, true, DungeonScaleModifiers{1, 1.2, 1, 1}},
	} {
		ds := resolveServerSettings(&proto.ServerSettings{DungeonScale: tc.settings}, live).DungeonScale
		if got := ds.Modifiers(tc.difficulty, tc.boss); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestDungeonScaleMultipliers(t *testing.T) {
	// variables, so the products below are float32 arithmetic like the server's
	f32 := func(v float64) float64 { return float64(float32(v)) }
	half, onePointTwo := float32(0.5), float32(1.2)

	for _, tc := range []struct {
		name      string
		modifiers *proto.DungeonScaleModifiers
		want      DungeonScaleMultipliers
	}{
		{"global times stat", modifiers(0.5, 1.5, 3, 1.2), DungeonScaleMultipliers{0.75, 1.5, float64(half * onePointTwo)}},
		{"live boss health", modifiers(1, 1.2, 1, 1), DungeonScaleMultipliers{f32(1.2), 1, 1}},
		{"floors, armor has none", modifiers(1, 0.001, 0.001, 0.001),
			DungeonScaleMultipliers{f32(DungeonScaleMinHealthModifier), f32(0.001), f32(DungeonScaleMinDamageModifier)}},
	} {
		settings := &proto.ServerSettings{DungeonScale: &proto.DungeonScaleSettings{Raid25Boss: tc.modifiers}}
		ds := NewServerSettings(settings).DungeonScale
		got := ds.Multipliers(proto.RaidDifficulty_RaidDifficulty25Normal, true)
		if got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestReforgingResolution(t *testing.T) {
	live := LiveServerDefaults().Reforge
	tooMany := make([]int32, maxReforgeableStatTypes+1)
	for i := range tooMany {
		tooMany[i] = int32(i + 1)
	}

	for _, tc := range []struct {
		name     string
		settings *proto.ReforgeSettings
		want     Reforging
	}{
		{"live", nil, Reforging{Enabled: live.GetEnable(), Percentage: live.GetPercentage(), StatTypes: live.StatTypes}},
		{"off", &proto.ReforgeSettings{Enable: googleProto.Bool(false)}, Reforging{Percentage: live.GetPercentage(), StatTypes: live.StatTypes}},
		{"edges of the range", &proto.ReforgeSettings{Enable: googleProto.Bool(true), Percentage: googleProto.Float64(90), StatTypes: []int32{31, 37}},
			Reforging{Enabled: true, Percentage: 90, StatTypes: []int32{31, 37}}},
		{"above the range", &proto.ReforgeSettings{Enable: googleProto.Bool(true), Percentage: googleProto.Float64(95)},
			Reforging{Enabled: true, Percentage: 40, StatTypes: live.StatTypes}},
		{"below the range", &proto.ReforgeSettings{Enable: googleProto.Bool(true), Percentage: googleProto.Float64(9.5)},
			Reforging{Enabled: true, Percentage: 40, StatTypes: live.StatTypes}},
		{"too many stats", &proto.ReforgeSettings{Enable: googleProto.Bool(true), Percentage: googleProto.Float64(10), StatTypes: tooMany},
			Reforging{Enabled: true, Percentage: 10}},
	} {
		got := NewServerSettings(&proto.ServerSettings{Reforge: tc.settings}).Reforging
		if got.Enabled != tc.want.Enabled || math.Abs(got.Percentage-tc.want.Percentage) > 1e-9 || !slices.Equal(got.StatTypes, tc.want.StatTypes) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestServerSettingsReachCharacters(t *testing.T) {
	raid := &proto.Raid{Parties: []*proto.Party{{}}, TargetDummies: 2}

	encounter := &proto.Encounter{Duration: 60, ServerSettings: &proto.ServerSettings{
		MapUpdateIntervalMs: googleProto.Int32(0),
		SpellTweaks:         &proto.SpellTweaksSettings{Enable: googleProto.Bool(false)},
	}}
	env, _, _ := NewEnvironment(raid, encounter, false)
	if env.Raid.Server.MapUpdateInterval != 0 || env.Raid.Server.SpellTweaks.Enabled {
		t.Errorf("raid got %+v, want the encounter's settings", env.Raid.Server)
	}
	for _, party := range env.Raid.Parties {
		for _, agent := range party.Players {
			if character := agent.GetCharacter(); character.Server() != env.Raid.Server {
				t.Errorf("%s reads %p, not the raid's settings %p", character.Label, character.Server(), env.Raid.Server)
			}
		}
	}

	env, _, _ = NewEnvironment(raid, &proto.Encounter{Duration: 60}, false)
	if !reflect.DeepEqual(env.Raid.Server, NewServerSettings(nil)) {
		t.Errorf("without settings the raid got %+v, want the live server's", env.Raid.Server)
	}
	if !reflect.DeepEqual((&Character{}).Server(), NewServerSettings(nil)) {
		t.Error("a character outside a raid should read the live server's settings")
	}
}
