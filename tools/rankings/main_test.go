package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

func capturedSettings() *proto.IndividualSimSettings {
	return &proto.IndividualSimSettings{
		Player: &proto.Player{
			Class: proto.Class_ClassWarrior,
			Buffs: &proto.IndividualBuffs{FocusMagic: true, Innervates: 2, PowerInfusions: 1, TricksOfTheTrades: 1, UnholyFrenzy: 1},
			Consumes: &proto.Consumes{
				Flask:         proto.Flask_FlaskOfEndlessRage,
				DefaultPotion: proto.Potions_IndestructiblePotion,
				PrepopPotion:  proto.Potions_UnknownPotion,
				Food:          proto.Food_FoodFishFeast,
			},
		},
		PartyBuffs: &proto.PartyBuffs{AtieshMage: 2},
		RaidBuffs:  &proto.RaidBuffs{ArcaneBrilliance: true},
		Debuffs:    &proto.Debuffs{Misery: true},
		Tanks:      []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}},
		Encounter: &proto.Encounter{
			Duration:       99,
			Targets:        []*proto.Target{{Level: 80}, {Level: 80}},
			RaidDifficulty: proto.RaidDifficulty_RaidDifficulty10Normal,
		},
	}
}

func TestStandardizeReplacesBuffs(t *testing.T) {
	in := capturedSettings()
	out := standardize(in, 300)

	if !out.PartyBuffs.HeroicPresence || out.PartyBuffs.AtieshMage != 0 {
		t.Errorf("party buffs %v: want Heroic Presence on and the page's items gone", out.PartyBuffs)
	}
	if !out.RaidBuffs.Bloodlust || !out.RaidBuffs.WrathOfAirTotem {
		t.Errorf("raid buffs %v: want the full set", out.RaidBuffs)
	}
	if !out.Debuffs.SunderArmor || !out.Debuffs.ShadowMastery {
		t.Errorf("debuffs %v: want the full set", out.Debuffs)
	}
	b := out.Player.Buffs
	if !b.BlessingOfKings || b.FocusMagic || b.Innervates != 0 || b.PowerInfusions != 0 || b.TricksOfTheTrades != 0 || b.UnholyFrenzy != 0 {
		t.Errorf("player buffs %v: want blessings only, no single-target gifts", b)
	}
	if len(out.Tanks) != 0 {
		t.Errorf("tanks %v: want none", out.Tanks)
	}
	if !in.Player.Buffs.FocusMagic || in.Debuffs.SunderArmor || len(in.Tanks) != 1 {
		t.Error("the captured settings were modified")
	}
}

func TestStandardizePotions(t *testing.T) {
	c := standardize(capturedSettings(), 180).Player.Consumes
	if c.DefaultPotion != proto.Potions_PotionOfSpeed || c.PrepopPotion != proto.Potions_PotionOfSpeed {
		t.Errorf("potions %v / %v: want Potion of Speed for both", c.DefaultPotion, c.PrepopPotion)
	}
	if c.Flask != proto.Flask_FlaskOfEndlessRage || c.Food != proto.Food_FoodFishFeast {
		t.Errorf("flask %v, food %v: want the page's", c.Flask, c.Food)
	}

	in := capturedSettings()
	in.Player.Consumes.DefaultPotion = proto.Potions_PotionOfWildMagic
	in.Player.Consumes.PrepopPotion = proto.Potions_HastePotion
	c = standardize(in, 180).Player.Consumes
	if c.DefaultPotion != proto.Potions_PotionOfWildMagic || c.PrepopPotion != proto.Potions_HastePotion {
		t.Errorf("potions %v / %v: a set pre-pot must stay", c.DefaultPotion, c.PrepopPotion)
	}

	in.Player.Consumes.PrepopPotion = proto.Potions_UnknownPotion
	c = standardize(in, 180).Player.Consumes
	if c.PrepopPotion != proto.Potions_PotionOfWildMagic {
		t.Errorf("pre-pot %v: want the main potion", c.PrepopPotion)
	}
}

func TestStandardizeEncounterPerFight(t *testing.T) {
	for _, fight := range []float64{60, 180, 300} {
		e := standardize(capturedSettings(), fight).Encounter
		if e.Duration != fight || e.DurationVariation != 5 {
			t.Errorf("duration %v ±%v: want %v ±5", e.Duration, e.DurationVariation, fight)
		}
		if e.ExecuteProportion_20 != 0.2 || e.ExecuteProportion_25 != 0.25 || e.ExecuteProportion_35 != 0.35 {
			t.Errorf("execute proportions %v/%v/%v", e.ExecuteProportion_20, e.ExecuteProportion_25, e.ExecuteProportion_35)
		}
		if len(e.Targets) != 1 || e.Targets[0].Level != 83 || e.Targets[0].MobType != proto.MobType_MobTypeGiant {
			t.Fatalf("targets %v: want one level 83 giant", e.Targets)
		}
		if e.Targets[0].Stats[proto.Stat_StatArmor] != 10643 || e.Targets[0].Stats[proto.Stat_StatAttackPower] != 805 {
			t.Errorf("boss stats %v", e.Targets[0].Stats)
		}
		if e.RaidDifficulty != proto.RaidDifficulty_RaidDifficulty10Normal {
			t.Errorf("raid difficulty %v: want the captured one", e.RaidDifficulty)
		}
	}
}

func inflateLink(t *testing.T, link string) *proto.IndividualSimSettings {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(link)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	out := &proto.IndividualSimSettings{}
	if err := googleProto.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLinkRoundTrip(t *testing.T) {
	want := standardize(capturedSettings(), 180)
	want.Settings = &proto.SimSettings{Phase: 3, Iterations: 10000}
	link, err := makeLink(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := inflateLink(t, link); !googleProto.Equal(got, want) {
		t.Errorf("link settings differ:\n got %v\nwant %v", got, want)
	}
}

func TestRunFixtureEndToEnd(t *testing.T) {
	sim.RegisterAll()
	out := t.TempDir()
	settings := filepath.Join(out, "settings")
	if err := os.MkdirAll(filepath.Join(out, "results"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(settings, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/fixture_p1.bin")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(settings, "fixture_p1.bin")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(filepath.Join(out, "run.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cfg := config{out: out, iterations: 5, seed: 1, fights: []int{60, 180, 300}, simHash: "a", log: logFile}
	j := job{build: "fixture", phase: 1, file: file}

	did, err := cfg.run(j)
	if err != nil || !did {
		t.Fatalf("run: did %v, err %v", did, err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "results", "fixture_p1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var res result
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Fights) != 3 {
		t.Fatalf("fights %v: want 60, 180 and 300", res.Fights)
	}
	for _, f := range []string{"60", "180", "300"} {
		r := res.Fights[f]
		if r.Dps <= 0 || r.Max < r.Dps || r.Min > r.Dps {
			t.Errorf("fight %s: %+v", f, r)
		}
	}
	if res.Talents == "" || res.Class != "ClassDeathknight" || len(res.Items) == 0 {
		t.Errorf("result header %+v", res)
	}
	link := inflateLink(t, res.Link)
	if link.Encounter.Duration != 180 || link.Settings.Phase != 1 || !link.PartyBuffs.HeroicPresence {
		t.Errorf("link settings: duration %v, phase %v, party buffs %v", link.Encounter.Duration, link.Settings.Phase, link.PartyBuffs)
	}
	if link.Settings.Iterations != 5 || !link.Settings.ShowDamageMetrics || link.Player.Database != nil {
		t.Errorf("link: iterations %v, damage metrics %v, item database %v: want 5, shown, none",
			link.Settings.Iterations, link.Settings.ShowDamageMetrics, link.Player.Database != nil)
	}

	if did, err := cfg.run(j); err != nil || did {
		t.Errorf("second run: did %v, err %v: want a skip", did, err)
	}
	cfg.seed = 2
	if did, err := cfg.run(j); err != nil || !did {
		t.Errorf("run at another seed: did %v, err %v: want a rerun", did, err)
	}
	cfg.simHash = "b"
	if did, err := cfg.run(j); err != nil || !did {
		t.Errorf("run on another sim build: did %v, err %v: want a rerun", did, err)
	}
}
