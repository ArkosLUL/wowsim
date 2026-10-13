package core

import (
	"math"
	"slices"
	"sync"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// ServerSettings is the AzerothCore config a sim runs under: the encounter's proto.ServerSettings,
// with every field it leaves unset taken from LiveServerDefaults. Shared by the whole raid, so
// read only. Characters get it from Server().
type ServerSettings struct {
	// MapUpdateInterval as configured: creatures, pets and guardians update on it. Zero means exact
	// timing for every unit.
	MapUpdateInterval time.Duration

	// WorldUpdateInterval is MinWorldUpdateTime, the world tick players update on. Zero means exact
	// timing for players.
	WorldUpdateInterval time.Duration

	// WorldLoopTime is how long the host's world loop takes for a tick (proto.ServerSettings.WorldLoopMs).
	// A tick is never shorter than WorldUpdateInterval. Zero means the interval itself.
	WorldLoopTime time.Duration

	// MapUpdateJitter is how far a map update's length strays from its mean, as a standard deviation
	// (proto.ServerSettings.MapUpdateJitterMs). Zero means every update takes the mean.
	MapUpdateJitter time.Duration

	// WorldLongTick is the one slow world tick of a map update (proto.ServerSettings.WorldLongTickMs); the
	// update's other ticks are WorldUpdateInterval. Zero means every tick of an update takes the same time.
	WorldLongTick time.Duration

	SpellTweaks  SpellTweaks
	DungeonScale DungeonScale
	Reforging    Reforging
}

// updateGrids is how far apart the world ticks and the map updates the sim runs on are, on average
// where the updates jitter. Every map update falls on a world tick: MapMgr::Update runs a map's full
// update on the first tick its interval is up by, and its four steps (continents, battlegrounds,
// instances, wait) keep two of them at least three ticks apart.
func (s *ServerSettings) updateGrids() (world, mapUpdate time.Duration) {
	if s.MapUpdateInterval <= 0 {
		return 0, 0
	}
	if s.WorldUpdateInterval <= 0 {
		return 0, s.MapUpdateInterval
	}
	tick := max(s.WorldUpdateInterval, s.WorldLoopTime)
	ticks := max(3, (s.MapUpdateInterval+tick-1)/tick)
	return tick, ticks * tick
}

// updateLengths is what the map updates' lengths are drawn from, or nil when they all take the mean:
// no jitter set, or a world loop that keeps time, which leaves an update nothing to stray by.
// WorldLongTick splits an update into short ticks and one long one.
func (s *ServerSettings) updateLengths() *updateLengths {
	world, mapUpdate := s.updateGrids()
	if world <= 0 || s.MapUpdateJitter <= 0 {
		return nil
	}
	ticks := mapUpdate / world
	// the timer has to pass the interval, and no tick is under MinWorldUpdateTime
	shortest := max(s.MapUpdateInterval, ticks*s.WorldUpdateInterval)
	if mapUpdate <= shortest {
		return nil
	}
	key := updateLengthsKey{shortest: shortest, mean: mapUpdate, jitter: s.MapUpdateJitter, ticks: ticks}
	if s.WorldLongTick > 0 {
		key.interval, key.long = s.WorldUpdateInterval, s.WorldLongTick
	}
	return updateLengthsFor(key)
}

// OnRegularGrid puts the encounter on a server that keeps time: every world tick takes loopMs and no map
// update strays. For tests that count updates or ticks, which the live server's jitter would scatter.
func OnRegularGrid(encounter *proto.Encounter, loopMs float64) *proto.Encounter {
	if encounter.ServerSettings == nil {
		encounter.ServerSettings = &proto.ServerSettings{}
	}
	jitterMs := 0.0
	encounter.ServerSettings.WorldLoopMs = &loopMs
	encounter.ServerSettings.MapUpdateJitterMs = &jitterMs
	return encounter
}

// longestWorldTick is the most a world tick takes: the one length it has, or the longest drawn.
func (s *ServerSettings) longestWorldTick() time.Duration {
	if lengths := s.updateLengths(); lengths != nil {
		return lengths.longest
	}
	world, _ := s.updateGrids()
	return world
}

// SpellTweaks is mod-spell-tweaks with SpellTweaks.Enable already folded in: each field says whether
// that tweak is live.
type SpellTweaks struct {
	// SpellTweaks.Enable, for the tweaks with no switch of their own: Crusader Strike and Divine Storm
	// stacking the seal DoT, and Explosive Trap laid from range (Trap Launcher).
	Enabled bool

	// Damage multiplier for BM exotic pets, 1 when off.
	ExoticPetDamageMultiplier float64

	HunterPetHaste            bool
	HunterPetArmorPen         bool
	DKGhoulArmorPen           bool
	FeralSpiritHaste          bool
	DiseaseHaste              bool
	DeadlyPoisonMurder        bool
	RuptureWeaponExpertise    bool
	RendTrauma                bool
	BalanceDotScaling         bool
	TitansGripNoDamagePenalty bool
	OmenClarityFaerieFire     bool
	ShadowDotHaste            bool
	GlyphOfBackstabTicks      bool
	PyroblastEmpoweredFire    bool
	CurseOfDoomScaling        bool
	ArcaneBlastHasteCost      bool
	ArcaneCritRefund          bool
}

// DungeonScaleModifiers is one mod-dungeon-scale key group, e.g. StatModifierRaid25M.Boss.
type DungeonScaleModifiers struct {
	Global, Health, Armor, Damage float64
}

// DungeonScaleMultipliers is what a creature's health, armor and damage get multiplied by.
type DungeonScaleMultipliers struct {
	Health, Armor, Damage float64
}

// DungeonScale is mod-dungeon-scale's full raid modifiers for each difficulty, creature and boss,
// with the size keys already resolved against the generic ones. proto.DungeonScaleSettings has the
// rules.
type DungeonScale struct {
	modifiers [proto.RaidDifficulty_RaidDifficulty25Heroic + 1][2]DungeonScaleModifiers
}

// Reforging is mod-reforging's config.
type Reforging struct {
	Enabled bool

	// Percentage of the source stat a reforge moves, e.g. 40.
	Percentage float64

	// ItemModType ids a reforge can take from or give to.
	StatTypes []int32
}

// NewServerSettings resolves an encounter's settings over the live server's: nil, or any field left
// unset, means the live value.
func NewServerSettings(settings *proto.ServerSettings) *ServerSettings {
	return resolveServerSettings(settings, LiveServerDefaults())
}

// partyServer is the settings the party's raid runs under, or the live server's for a character
// built outside a raid, like the hand-made ones in tests.
func partyServer(party *Party) *ServerSettings {
	if party == nil || party.Raid == nil || party.Raid.Server == nil {
		return NewServerSettings(nil)
	}
	return party.Raid.Server
}

// LiveReforging is the live server's mod-reforging config, for callers with no raid to take
// settings from, like the item tools and the gear a test builds by hand.
var LiveReforging = sync.OnceValue(func() *Reforging {
	return &NewServerSettings(nil).Reforging
})

func resolveServerSettings(settings, live *proto.ServerSettings) *ServerSettings {
	settings, live = orEmpty(settings), orEmpty(live)
	return &ServerSettings{
		MapUpdateInterval:   time.Duration(max(0, firstSet(0, settings.MapUpdateIntervalMs, live.MapUpdateIntervalMs))) * time.Millisecond,
		WorldUpdateInterval: time.Duration(max(0, firstSet(0, settings.WorldUpdateIntervalMs, live.WorldUpdateIntervalMs))) * time.Millisecond,
		WorldLoopTime:       time.Duration(math.Round(max(0, firstSet(0, settings.WorldLoopMs, live.WorldLoopMs)) * float64(time.Millisecond))),
		MapUpdateJitter:     time.Duration(math.Round(max(0, firstSet(0, settings.MapUpdateJitterMs, live.MapUpdateJitterMs)) * float64(time.Millisecond))),
		WorldLongTick:       time.Duration(math.Round(max(0, firstSet(0, settings.WorldLongTickMs, live.WorldLongTickMs)) * float64(time.Millisecond))),
		SpellTweaks:         newSpellTweaks(orEmpty(settings.SpellTweaks), orEmpty(live.SpellTweaks)),
		DungeonScale:        newDungeonScale(settings.DungeonScale, live.DungeonScale),
		Reforging:           newReforging(orEmpty(settings.Reforge), orEmpty(live.Reforge)),
	}
}

func newSpellTweaks(settings, live *proto.SpellTweaksSettings) SpellTweaks {
	pick := func(setting, liveValue *bool) bool { return firstSet(false, setting, liveValue) }

	// core reads the armor pen switches itself, so SpellTweaks.Enable doesn't gate them
	tweaks := SpellTweaks{
		Enabled:                   pick(settings.Enable, live.Enable),
		ExoticPetDamageMultiplier: 1,
		HunterPetArmorPen:         pick(settings.HunterPetArmorPen, live.HunterPetArmorPen),
		DKGhoulArmorPen:           pick(settings.DkGhoulArmorPen, live.DkGhoulArmorPen),
	}
	if !tweaks.Enabled {
		return tweaks
	}

	// float32 like SpellTweaks.cpp, negative clamped to 0
	pct := max(0, float32(firstSet(0, settings.ExoticPetDamagePct, live.ExoticPetDamagePct)))
	tweaks.ExoticPetDamageMultiplier = float64(1 + pct/100)

	tweaks.HunterPetHaste = pick(settings.HunterPetHaste, live.HunterPetHaste)
	tweaks.FeralSpiritHaste = pick(settings.FeralSpiritHaste, live.FeralSpiritHaste)
	tweaks.DiseaseHaste = pick(settings.DiseaseHaste, live.DiseaseHaste)
	tweaks.DeadlyPoisonMurder = pick(settings.DeadlyPoisonMurder, live.DeadlyPoisonMurder)
	tweaks.RuptureWeaponExpertise = pick(settings.RuptureWeaponExpertise, live.RuptureWeaponExpertise)
	tweaks.RendTrauma = pick(settings.RendTrauma, live.RendTrauma)
	tweaks.BalanceDotScaling = pick(settings.BalanceDotScaling, live.BalanceDotScaling)
	tweaks.TitansGripNoDamagePenalty = pick(settings.TitansGripNoDamagePenalty, live.TitansGripNoDamagePenalty)
	tweaks.OmenClarityFaerieFire = pick(settings.OmenClarityFaerieFire, live.OmenClarityFaerieFire)
	tweaks.ShadowDotHaste = pick(settings.ShadowDotHaste, live.ShadowDotHaste)
	tweaks.GlyphOfBackstabTicks = pick(settings.GlyphOfBackstabTicks, live.GlyphOfBackstabTicks)
	tweaks.PyroblastEmpoweredFire = pick(settings.PyroblastEmpoweredFire, live.PyroblastEmpoweredFire)
	tweaks.CurseOfDoomScaling = pick(settings.CurseOfDoomScaling, live.CurseOfDoomScaling)
	tweaks.ArcaneBlastHasteCost = pick(settings.ArcaneBlastHasteCost, live.ArcaneBlastHasteCost)
	tweaks.ArcaneCritRefund = pick(settings.ArcaneCritRefund, live.ArcaneCritRefund)
	return tweaks
}

func newDungeonScale(settings, live *proto.DungeonScaleSettings) DungeonScale {
	// the server's own default when a key is missing everywhere
	unscaled := DungeonScaleModifiers{Global: 1, Health: 1, Armor: 1, Damage: 1}
	raid := [2]DungeonScaleModifiers{
		resolveModifiers(settings.GetRaid(), live.GetRaid(), unscaled),
		resolveModifiers(settings.GetRaidBoss(), live.GetRaidBoss(), unscaled),
	}
	raidHeroic := [2]DungeonScaleModifiers{
		resolveModifiers(settings.GetRaidHeroic(), live.GetRaidHeroic(), unscaled),
		resolveModifiers(settings.GetRaidHeroicBoss(), live.GetRaidHeroicBoss(), unscaled),
	}

	type sizeSet = func(*proto.DungeonScaleSettings) *proto.DungeonScaleModifiers
	var ds DungeonScale
	for _, size := range []struct {
		difficulty     proto.RaidDifficulty
		generic        [2]DungeonScaleModifiers
		creature, boss sizeSet
	}{
		{proto.RaidDifficulty_RaidDifficulty10Normal, raid, (*proto.DungeonScaleSettings).GetRaid10, (*proto.DungeonScaleSettings).GetRaid10Boss},
		{proto.RaidDifficulty_RaidDifficulty25Normal, raid, (*proto.DungeonScaleSettings).GetRaid25, (*proto.DungeonScaleSettings).GetRaid25Boss},
		{proto.RaidDifficulty_RaidDifficulty10Heroic, raidHeroic, (*proto.DungeonScaleSettings).GetRaid10Heroic, (*proto.DungeonScaleSettings).GetRaid10HeroicBoss},
		{proto.RaidDifficulty_RaidDifficulty25Heroic, raidHeroic, (*proto.DungeonScaleSettings).GetRaid25Heroic, (*proto.DungeonScaleSettings).GetRaid25HeroicBoss},
	} {
		ds.modifiers[size.difficulty] = [2]DungeonScaleModifiers{
			resolveModifiers(size.creature(settings), size.creature(live), size.generic[0]),
			resolveModifiers(size.boss(settings), size.boss(live), size.generic[1]),
		}
	}
	return ds
}

// resolveModifiers takes each value from the setting, else the live config, else the fallback.
func resolveModifiers(setting, live *proto.DungeonScaleModifiers, fallback DungeonScaleModifiers) DungeonScaleModifiers {
	setting, live = orEmpty(setting), orEmpty(live)
	return DungeonScaleModifiers{
		Global: firstSet(fallback.Global, setting.Global, live.Global),
		Health: firstSet(fallback.Health, setting.Health, live.Health),
		Armor:  firstSet(fallback.Armor, setting.Armor, live.Armor),
		Damage: firstSet(fallback.Damage, setting.Damage, live.Damage),
	}
}

// RaidDifficultyOrDefault reads Unknown, or a value this build doesn't know, as 25-player normal.
func RaidDifficultyOrDefault(difficulty proto.RaidDifficulty) proto.RaidDifficulty {
	switch difficulty {
	case proto.RaidDifficulty_RaidDifficulty10Normal, proto.RaidDifficulty_RaidDifficulty25Normal,
		proto.RaidDifficulty_RaidDifficulty10Heroic, proto.RaidDifficulty_RaidDifficulty25Heroic:
		return difficulty
	}
	return proto.RaidDifficulty_RaidDifficulty25Normal
}

// Modifiers is the key group mod-dungeon-scale uses for a creature, or a boss, in this difficulty.
func (ds *DungeonScale) Modifiers(difficulty proto.RaidDifficulty, boss bool) DungeonScaleModifiers {
	row := ds.modifiers[RaidDifficultyOrDefault(difficulty)]
	if boss {
		return row[1]
	}
	return row[0]
}

// Multipliers combines Modifiers the way DungeonScale.cpp ModifyCreatureAttributes does for a full
// raid: global * stat in float32, health and damage floored, armor not. The server then rounds the
// scaled health and armor.
func (ds *DungeonScale) Multipliers(difficulty proto.RaidDifficulty, boss bool) DungeonScaleMultipliers {
	m := ds.Modifiers(difficulty, boss)
	global := float32(m.Global)
	health := global * float32(m.Health)
	if health <= DungeonScaleMinHealthModifier {
		health = DungeonScaleMinHealthModifier
	}
	damage := global * float32(m.Damage)
	if damage <= DungeonScaleMinDamageModifier {
		damage = DungeonScaleMinDamageModifier
	}
	return DungeonScaleMultipliers{
		Health: float64(health),
		Armor:  float64(global * float32(m.Armor)),
		Damage: float64(damage),
	}
}

func newReforging(settings, live *proto.ReforgeSettings) Reforging {
	// SetPercentage falls back to the default outside the range, compared in float32
	percentage := firstSet(float64(ReforgeDefaultPercentage), settings.Percentage, live.Percentage)
	if p := float32(percentage); p < ReforgeMinPercentage || p > ReforgeMaxPercentage {
		percentage = ReforgeDefaultPercentage
	}

	statTypes := settings.StatTypes
	if len(statTypes) == 0 {
		statTypes = live.StatTypes
	}
	// SetReforgeableStats leaves the list empty when it's too long
	if len(statTypes) > MaxReforgeableStatTypes {
		statTypes = nil
	}

	return Reforging{
		Enabled:    firstSet(false, settings.Enable, live.Enable),
		Percentage: percentage,
		StatTypes:  slices.Clone(statTypes),
	}
}

// firstSet returns the first value that's set, else def.
func firstSet[T any](def T, values ...*T) T {
	for _, v := range values {
		if v != nil {
			return *v
		}
	}
	return def
}

func orEmpty[T any](m *T) *T {
	if m == nil {
		return new(T)
	}
	return m
}
