package main

import (
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

// Level 83 plain boss, as optimizerEncounter in ui/core/optimizer/pool_builder.ts builds it.
const (
	bossArmor       = 10643
	bossAttackPower = 805
	// seconds either way the fight length varies
	durationVariation = 5
)

func raidBuffs() *proto.RaidBuffs {
	return &proto.RaidBuffs{
		AbominationsMight:     true,
		ArcaneBrilliance:      true,
		ArcaneEmpowerment:     true,
		BattleShout:           proto.TristateEffect_TristateEffectImproved,
		Bloodlust:             true,
		DemonicPactSp:         500,
		DevotionAura:          proto.TristateEffect_TristateEffectImproved,
		DivineSpirit:          true,
		ElementalOath:         true,
		FerociousInspiration:  true,
		GiftOfTheWild:         proto.TristateEffect_TristateEffectImproved,
		IcyTalons:             true,
		LeaderOfThePack:       proto.TristateEffect_TristateEffectImproved,
		ManaSpringTotem:       proto.TristateEffect_TristateEffectRegular,
		MoonkinAura:           proto.TristateEffect_TristateEffectImproved,
		PowerWordFortitude:    proto.TristateEffect_TristateEffectImproved,
		SanctifiedRetribution: true,
		ShadowProtection:      true,
		StrengthOfEarthTotem:  proto.TristateEffect_TristateEffectImproved,
		SwiftRetribution:      true,
		Thorns:                proto.TristateEffect_TristateEffectImproved,
		TotemOfWrath:          true,
		TrueshotAura:          true,
		UnleashedRage:         true,
		WindfuryTotem:         proto.TristateEffect_TristateEffectImproved,
		WrathOfAirTotem:       true,
	}
}

func partyBuffs() *proto.PartyBuffs {
	return &proto.PartyBuffs{ManaTideTotems: 1, HeroicPresence: true}
}

// Blessings and replenishment sources only. The single-target gifts (Focus Magic, Tricks, Power
// Infusion, Innervate, Hysteria) stay off.
func individualBuffs() *proto.IndividualBuffs {
	return &proto.IndividualBuffs{
		BlessingOfKings:     true,
		BlessingOfMight:     proto.TristateEffect_TristateEffectImproved,
		BlessingOfSanctuary: true,
		BlessingOfWisdom:    proto.TristateEffect_TristateEffectImproved,
		JudgementsOfTheWise: true,
		VampiricTouch:       true,
	}
}

func debuffs() *proto.Debuffs {
	return &proto.Debuffs{
		BloodFrenzy:        true,
		CurseOfElements:    true,
		CurseOfWeakness:    proto.TristateEffect_TristateEffectImproved,
		EarthAndMoon:       true,
		EbonPlaguebringer:  true,
		ExposeArmor:        true,
		FaerieFire:         proto.TristateEffect_TristateEffectImproved,
		GiftOfArthas:       true,
		HeartOfTheCrusader: true,
		ImprovedScorch:     true,
		InsectSwarm:        true,
		JudgementOfLight:   true,
		JudgementOfWisdom:  true,
		Mangle:             true,
		Misery:             true,
		ScorpidSting:       true,
		ShadowEmbrace:      true,
		ShadowMastery:      true,
		SunderArmor:        true,
		ThunderClap:        proto.TristateEffect_TristateEffectImproved,
		TotemOfWrath:       true,
		Vindication:        true,
	}
}

func plainBoss() *proto.Target {
	bossStats := make([]float64, int(stats.Len))
	bossStats[proto.Stat_StatArmor] = bossArmor
	bossStats[proto.Stat_StatAttackPower] = bossAttackPower
	return &proto.Target{
		Level:         core.CharacterLevel + 3,
		MobType:       proto.MobType_MobTypeGiant,
		TankIndex:     0,
		SwingSpeed:    1.5,
		MinBaseDamage: 65000,
		ParryHaste:    true,
		SpellSchool:   proto.SpellSchool_SpellSchoolPhysical,
		Stats:         bossStats,
	}
}

// standardize returns a copy of the captured settings under the standard scenario: the same buffs,
// debuffs and boss for every build, a fight of the given length. Talents, glyphs, spec options,
// flasks and food stay as the page had them.
func standardize(captured *proto.IndividualSimSettings, fightSeconds float64) *proto.IndividualSimSettings {
	out := googleProto.Clone(captured).(*proto.IndividualSimSettings)
	out.Player.Buffs = individualBuffs()
	out.PartyBuffs = partyBuffs()
	out.RaidBuffs = raidBuffs()
	out.Debuffs = debuffs()
	out.Tanks = nil
	out.TargetDummies = 0
	fixPotions(out.Player)

	out.Encounter = &proto.Encounter{
		Duration:             fightSeconds,
		DurationVariation:    durationVariation,
		ExecuteProportion_20: 0.2,
		ExecuteProportion_25: 0.25,
		ExecuteProportion_35: 0.35,
		Targets:              []*proto.Target{plainBoss()},
		RaidDifficulty:       captured.GetEncounter().GetRaidDifficulty(),
		ServerSettings:       captured.GetEncounter().GetServerSettings(),
	}
	return out
}

// A build without a pre-pot drinks its main potion first. Warriors' Indestructible Potion is a
// tanking potion, so Potion of Speed stands in.
func fixPotions(player *proto.Player) {
	if player.Consumes == nil {
		return
	}
	c := player.Consumes
	if c.DefaultPotion == proto.Potions_IndestructiblePotion {
		c.DefaultPotion = proto.Potions_PotionOfSpeed
	}
	if c.PrepopPotion == proto.Potions_IndestructiblePotion {
		c.PrepopPotion = proto.Potions_PotionOfSpeed
	}
	if c.PrepopPotion == proto.Potions_UnknownPotion {
		c.PrepopPotion = c.DefaultPotion
	}
}
