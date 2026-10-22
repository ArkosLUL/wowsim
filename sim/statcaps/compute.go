package statcaps

import (
	"errors"
	"fmt"
	"math"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// ComputeRequest answers the StatCaps RPC for the raid's first player.
func ComputeRequest(request *proto.StatCapsRequest) (result *proto.StatCapsResult) {
	defer func() {
		if e := recover(); e != nil {
			result = &proto.StatCapsResult{ErrorResult: fmt.Sprintf("%v\nStack Trace:\n%s", e, debug.Stack())}
		}
	}()
	caps, err := Compute(request.GetRaid(), request.GetEncounter(), 0)
	if err != nil {
		return &proto.StatCapsResult{ErrorResult: err.Error()}
	}
	return &proto.StatCapsResult{Caps: caps}
}

// Compute is the caps of the player at raidIndex for their role, against the encounter's first
// target. Each is gear rating, the way the in-game sheet shows it: "rating" is what gear, gems,
// enchants, food, elixirs and bonus stats give, "needed" what's left of the cap once race, talents,
// buffs and the target's debuffs have had their share. A spec without caps gets none.
func Compute(raid *proto.Raid, encounter *proto.Encounter, raidIndex int) ([]*proto.StatCap, error) {
	player := playerAt(raid, raidIndex)
	if player == nil {
		return nil, fmt.Errorf("no player at raid index %d", raidIndex)
	}
	if len(encounter.GetTargets()) == 0 {
		return nil, errors.New("the encounter has no target")
	}
	role := RoleOf(player)
	if role == RoleNone {
		return nil, nil
	}
	sheetAt := PlayerSheetAt(raid, encounter, raidIndex)
	seed, err := sheetAt(stats.Stats{})
	if err != nil {
		return nil, err
	}
	c := &computer{player: player, raid: raid, raidIndex: raidIndex, sheetAt: sheetAt, seed: seed}

	defs := c.defs(role)
	caps := make([]*proto.StatCap, len(defs))
	errs := make([]error, len(defs))
	var wg sync.WaitGroup
	for i, def := range defs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if e := recover(); e != nil {
					errs[i] = fmt.Errorf("%s cap: %v", def.label, e)
				}
			}()
			caps[i] = c.compute(def)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return caps, nil
}

// PlayerSheetAt is the sheet of the player at raidIndex, offset added to their bonus stats. It clones
// both protos every time: building an environment can write to them, and the caps bisect in parallel.
func PlayerSheetAt(raid *proto.Raid, encounter *proto.Encounter, raidIndex int) SheetAt {
	return func(offset stats.Stats) (core.PlayerSheet, error) {
		raid := goproto.Clone(raid).(*proto.Raid)
		if offset != (stats.Stats{}) {
			p := playerAt(raid, raidIndex)
			if p == nil {
				return core.PlayerSheet{}, fmt.Errorf("no player at raid index %d", raidIndex)
			}
			if p.BonusStats == nil {
				p.BonusStats = &proto.UnitStats{}
			}
			p.BonusStats.Stats = stats.FromFloatArray(p.BonusStats.Stats).Add(offset).ToFloatArray()
		}
		return core.ComputePlayerSheet(raid, goproto.Clone(encounter).(*proto.Encounter), raidIndex)
	}
}

func playerAt(raid *proto.Raid, raidIndex int) *proto.Player {
	parties := raid.GetParties()
	if raidIndex < 0 || raidIndex/5 >= len(parties) || raidIndex%5 >= len(parties[raidIndex/5].GetPlayers()) {
		return nil
	}
	return parties[raidIndex/5].Players[raidIndex%5]
}

type computer struct {
	player    *proto.Player
	raid      *proto.Raid
	raidIndex int
	sheetAt   SheetAt
	seed      core.PlayerSheet
}

// capDef is one cap to work out.
type capDef struct {
	label string
	stat  stats.Stat
	hand  proto.StatCapHand
	kind  proto.StatCapKind
	// the sheet rating the cap sits at
	sheetCap func() (float64, error)
	// the rating and percent the cap asks for with nothing of the player's; with no rating, only the
	// race, talent and buff rows are listed
	bare, barePct float64
	// names the bare row when the target's level isn't what sets it
	bareLabel string
	// expertise on the hand's own spells, and the row naming it
	handBonus      float64
	handBonusLabel string
	debuffs        float64
	debuffLabel    string
	// names what's left once every other row is counted
	restLabel string
	note      func(frame float64) string
}

func (c *computer) defs(role Role) []capDef {
	var defs []capDef
	switch role {
	case RoleMelee:
		defs = append(defs, c.meleeHit())
		if c.seed.OffHand.Swings {
			defs = append(defs, c.whiteHit())
		}
		defs = append(defs, c.expertise(c.player.GetInFrontOfTarget())...)
		defs = append(defs, c.armorPen())
		if c.seed.CastsSpellDamage {
			defs = append(defs, c.spellHit())
		}
	case RoleHunter:
		defs = append(defs, c.meleeHit(), c.armorPen(), c.rangedHaste())
	case RoleCaster:
		defs = append(defs, c.spellHit(), c.spellHaste())
	case RoleTank:
		defs = append(defs, c.meleeHit())
		defs = append(defs, c.expertise(true)...)
		if c.seed.MeleeAttacker {
			defs = append(defs, c.defense())
		}
	case RoleHealer:
		defs = append(defs, c.spellHaste())
	}
	return defs
}

func (c *computer) bisect(st stats.Stat, ok func(core.PlayerSheet) bool) func() (float64, error) {
	return func() (float64, error) { return BisectCapBonus(c.sheetAt, st, ok) }
}

func (c *computer) meleeHit() capDef {
	rating, pct := bareRating(func(r float64) int32 {
		in := c.seed.BareTable
		in.HitPct = float32(r / core.MeleeHitRatingPerHitChance)
		return core.MeleeMissBP(in, true)
	})
	return capDef{
		label:       "Hit",
		stat:        stats.MeleeHit,
		kind:        proto.StatCapKind_StatCapFloor,
		sheetCap:    c.bisect(stats.MeleeHit, func(s core.PlayerSheet) bool { return s.PhysicalMissChance == 0 }),
		bare:        rating,
		barePct:     pct,
		debuffs:     c.seed.MeleeHitTaken,
		debuffLabel: "Target debuffs",
		restLabel:   "Talents on your attacks",
	}
}

func (c *computer) whiteHit() capDef {
	rating, pct := bareRating(func(r float64) int32 {
		in := c.seed.BareTable
		in.DualWield = true
		in.HitPct = float32(r / core.MeleeHitRatingPerHitChance)
		return core.MeleeMissBP(in, false)
	})
	def := capDef{
		label:       "White hit (dual wield)",
		stat:        stats.MeleeHit,
		kind:        proto.StatCapKind_StatCapSoft,
		sheetCap:    c.bisect(stats.MeleeHit, func(s core.PlayerSheet) bool { return s.WhiteMissChance == 0 }),
		bare:        rating,
		barePct:     pct,
		debuffs:     c.seed.MeleeHitTaken,
		debuffLabel: "Target debuffs",
		restLabel:   "Talents on your attacks",
	}
	if c.player.GetClass() == proto.Class_ClassWarrior {
		def.note = func(float64) string {
			return "While Heroic Strike or Cleave is queued, your off-hand swings skip the dual wield penalty."
		}
	}
	return def
}

// spellHit is where spells stop missing. The server rolls irand(1, 10000) and misses below the
// threshold SpellMissBP gives, so a threshold of 1 already never misses.
func (c *computer) spellHit() capDef {
	rating, pct := bareRating(func(r float64) int32 {
		in := c.seed.BareTable
		return max(core.SpellMissBP(in.DefenderLevel-in.AttackerLevel, float32(r/core.SpellHitRatingPerHitChance), in.DefenderIsPlayer)-1, 0)
	})
	return capDef{
		label: "Spell hit",
		stat:  stats.SpellHit,
		kind:  proto.StatCapKind_StatCapFloor,
		sheetCap: c.bisect(stats.SpellHit, func(s core.PlayerSheet) bool {
			return math.Round(s.SpellMissChance*core.MaxRollBP) <= 1
		}),
		bare:        rating,
		barePct:     pct,
		debuffs:     c.seed.SpellHitTaken,
		debuffLabel: spellHitDebuffLabel(c.raid.GetDebuffs()),
		restLabel:   "Talents on your spells",
	}
}

// expertise is a dodge row and, with parry, a parry row, split per hand when the hands' own
// expertise differs (an Orc with an axe in one hand only). Each notes where white swings stop
// being dodged or parried, which is lower: the white table takes the level gap in after expertise.
func (c *computer) expertise(parry bool) []capDef {
	type hand struct {
		hand  proto.StatCapHand
		sheet func(core.PlayerSheet) core.HandSheet
		seed  core.HandSheet
	}
	mainHand := func(s core.PlayerSheet) core.HandSheet { return s.MainHand }
	offHand := func(s core.PlayerSheet) core.HandSheet { return s.OffHand }
	mh, oh := c.seed.MainHand, c.seed.OffHand
	if !mh.Swings {
		return []capDef{c.expertiseDef("Expertise (dodge)", proto.StatCapHand_StatCapHandBoth, core.HandSheet{}, true,
			func(s core.PlayerSheet) bool { return s.ExpertiseDodgeChance == 0 }, nil)}
	}
	hands := []hand{{proto.StatCapHand_StatCapHandBoth, mainHand, mh}}
	if oh.Swings && oh.BonusExpertiseRating != mh.BonusExpertiseRating {
		hands = []hand{{proto.StatCapHand_StatCapHandMain, mainHand, mh}, {proto.StatCapHand_StatCapHandOff, offHand, oh}}
	}

	var defs []capDef
	for _, h := range hands {
		defs = append(defs, c.expertiseDef("Expertise (dodge)", h.hand, h.seed, true,
			func(s core.PlayerSheet) bool { return h.sheet(s).Swings && h.sheet(s).YellowDodgeChance == 0 },
			func(s core.PlayerSheet) bool { return h.sheet(s).Swings && h.sheet(s).WhiteDodgeChance == 0 }))
	}
	// a target that never parries (a non-boss that isn't humanoid) has no parry cap
	if _, pct := c.bareExpertise(false); !parry || pct == 0 {
		return defs
	}
	for _, h := range hands {
		defs = append(defs, c.expertiseDef("Expertise (parry)", h.hand, h.seed, false,
			func(s core.PlayerSheet) bool { return h.sheet(s).Swings && h.sheet(s).YellowParryChance == 0 },
			func(s core.PlayerSheet) bool { return h.sheet(s).Swings && h.sheet(s).WhiteParryChance == 0 }))
	}
	return defs
}

func (c *computer) expertiseDef(label string, hand proto.StatCapHand, seed core.HandSheet, dodge bool, yellowOK, whiteOK func(core.PlayerSheet) bool) capDef {
	rating, pct := c.bareExpertise(dodge)
	def := capDef{
		label:     label,
		stat:      stats.Expertise,
		hand:      hand,
		kind:      proto.StatCapKind_StatCapFloor,
		sheetCap:  c.bisect(stats.Expertise, yellowOK),
		bare:      rating,
		barePct:   pct,
		restLabel: "Talents on your attacks",
	}
	if seed.BonusExpertiseRating != 0 {
		def.handBonus = seed.BonusExpertiseRating
		def.handBonusLabel = fmt.Sprintf("%s %s", raceName(racialTraits(c.player)), weaponName(seed.WeaponType))
		if hand != proto.StatCapHand_StatCapHandBoth {
			def.handBonusLabel += ": " + handName(hand) + " only"
		}
	}
	if whiteOK != nil {
		avoided := "dodged"
		if !dodge {
			avoided = "parried"
		}
		def.note = func(frame float64) string {
			white, err := BisectCapBonus(c.sheetAt, stats.Expertise, whiteOK)
			if err != nil {
				return ""
			}
			white -= frame
			return fmt.Sprintf("White swings stop being %s at %.0f (%.1f expertise).",
				avoided, white, white/core.ExpertisePerQuarterPercentReduction)
		}
	}
	return def
}

// bareExpertise is the yellow dodge or parry cap with nothing of the player's.
func (c *computer) bareExpertise(dodge bool) (rating, pct float64) {
	return bareRating(func(r float64) int32 {
		in := c.seed.BareTable
		in.ExpertisePct = float32(r / core.ExpertisePerQuarterPercentReduction / 4)
		table := core.YellowMeleeTableBP(in, core.YellowOptions{})
		if dodge {
			return table.Dodge
		}
		return table.Parry
	})
}

func (c *computer) armorPen() capDef {
	perPercent := core.RatingPerPercent(c.player.GetClass(), core.CRArmorPenetration)
	return capDef{
		label:     "Armor penetration",
		stat:      stats.ArmorPenetration,
		kind:      proto.StatCapKind_StatCapCeiling,
		sheetCap:  c.bisect(stats.ArmorPenetration, func(s core.PlayerSheet) bool { return s.ArmorPenPercent >= 100 }),
		bare:      math.Ceil(100*perPercent - 1e-9),
		barePct:   100,
		bareLabel: "100% armor penetration",
		restLabel: "Stances and talents",
	}
}

func (c *computer) spellHaste() capDef {
	return capDef{
		label: "Haste",
		stat:  stats.SpellHaste,
		kind:  proto.StatCapKind_StatCapCeiling,
		sheetCap: c.bisect(stats.SpellHaste, func(s core.PlayerSheet) bool {
			return GlobalCooldown(s.CastSpeed) <= HasteCeilingFloor
		}),
		note: func(float64) string {
			return fmt.Sprintf("Your GCD is %.2f s now (cast speed ×%.3f). Past this it stays at 1 s.",
				GlobalCooldown(c.seed.CastSpeed).Seconds(), c.seed.CastSpeed)
		},
	}
}

func (c *computer) rangedHaste() capDef {
	return capDef{
		label: "Haste",
		stat:  stats.MeleeHaste,
		kind:  proto.StatCapKind_StatCapCeiling,
		sheetCap: c.bisect(stats.MeleeHaste, func(s core.PlayerSheet) bool {
			return SteadyShotCastTime(s.RangedSwingSpeed) <= HasteCeilingFloor
		}),
		note: func(float64) string {
			return fmt.Sprintf("Steady Shot casts in %.2f s now (ranged speed ×%.3f). The ceiling is where it casts in 1 s.",
				SteadyShotCastTime(c.seed.RangedSwingSpeed).Seconds(), c.seed.RangedSwingSpeed)
		},
	}
}

func (c *computer) defense() capDef {
	return capDef{
		label:    "Defense (crit immunity)",
		stat:     stats.Defense,
		kind:     proto.StatCapKind_StatCapFloor,
		sheetCap: func() (float64, error) { return RequiredDefense(c.sheetAt) },
		note: func(float64) string {
			return "Counts your resilience and the talents that cut the boss's crit chance."
		},
	}
}

// compute bisects def on the sheet and lays it out in the gear-side frame.
func (c *computer) compute(def capDef) *proto.StatCap {
	st := def.stat
	base := c.seed.BaseStats[st]
	talents := c.seed.TalentsStats[st] - c.seed.GearStats[st]
	buffs := c.seed.BuffsStats[st] - c.seed.TalentsStats[st]
	frame := base + talents + buffs
	result := &proto.StatCap{
		Label:  def.label,
		Stat:   proto.Stat(st),
		Hand:   def.hand,
		Kind:   def.kind,
		Rating: c.seed.FinalStats[st] - frame,
	}
	sheetCap, err := def.sheetCap()
	if err != nil {
		result.Needed = math.Inf(1)
		result.Note = "Out of reach: " + err.Error() + "."
		return result
	}
	result.Needed = sheetCap - frame

	perPercent := c.ratingPerPercent(st)
	sum := 0.0
	add := func(label string, rating float64, pct float64) {
		// a snapshot difference can leave float noise where nothing changed
		if math.Abs(rating) < 1e-6 {
			return
		}
		result.Sources = append(result.Sources, &proto.StatCapSource{Label: label, Rating: rating, Percent: pct})
		sum += rating
	}
	share := func(label string, rating float64) { add(label, -rating, -rating/perPercent) }
	if def.bare > 0 {
		label := def.bareLabel
		if label == "" {
			label = fmt.Sprintf("Level %d target", c.seed.BareTable.DefenderLevel)
		}
		add(label, def.bare, def.barePct)
	}
	share(raceName(racialTraits(c.player)), base)
	share(def.handBonusLabel, def.handBonus)
	share("Talents", talents)
	share(c.buffsLabel(st), buffs)
	share(def.debuffLabel, def.debuffs)
	// less than a rating point off is the table's rounding, not a source
	if def.bare > 0 && result.Needed > 0 {
		if rest := result.Needed - sum; math.Abs(rest) >= 1 {
			add(def.restLabel, rest, rest/perPercent)
		}
	}
	if result.Needed <= 0 {
		result.Needed = 0
		result.Note = "Your race, talents and buffs already reach it."
	} else if def.note != nil {
		result.Note = def.note(frame)
	}
	return result
}

func (c *computer) ratingPerPercent(st stats.Stat) float64 {
	switch st {
	case stats.MeleeHit:
		return core.MeleeHitRatingPerHitChance
	case stats.SpellHit:
		return core.SpellHitRatingPerHitChance
	case stats.Expertise:
		return core.ExpertisePerQuarterPercentReduction * 4
	case stats.ArmorPenetration:
		return core.RatingPerPercent(c.player.GetClass(), core.CRArmorPenetration)
	case stats.Defense:
		// as the boss's crit chance: a point of defense skill takes 0.04% off it
		return core.DefenseRatingPerDefense / core.PercentPerSkillPoint
	default:
		return core.HasteRatingPerHastePercent
	}
}

func (c *computer) buffsLabel(st stats.Stat) string {
	if (st == stats.MeleeHit || st == stats.SpellHit) && heroicPresence(c.raid, c.raidIndex) {
		return "Heroic Presence"
	}
	return "Buffs"
}

// bareRating is the smallest whole rating, up to MaxCapBonus, at which chanceAt reaches 0, and the
// chance at no rating in percent.
func bareRating(chanceAt func(rating float64) int32) (rating, pct float64) {
	at := sort.Search(MaxCapBonus+1, func(r int) bool { return chanceAt(float64(r)) <= 0 })
	if at > MaxCapBonus {
		at = 0
	}
	return float64(at), float64(chanceAt(0)) / 100
}

// heroicPresence is whether a Draenei's aura reaches the player's party (character.AddPartyBuffs),
// or the party buffs set it.
func heroicPresence(raid *proto.Raid, raidIndex int) bool {
	party := raid.GetParties()[raidIndex/5]
	if party.GetBuffs().GetHeroicPresence() {
		return true
	}
	for _, p := range party.GetPlayers() {
		if racialTraits(p) == proto.Race_RaceDraenei {
			return true
		}
	}
	return false
}

func spellHitDebuffLabel(debuffs *proto.Debuffs) string {
	misery := debuffs.GetMisery()
	improvedFaerieFire := debuffs.GetFaerieFire() == proto.TristateEffect_TristateEffectImproved
	switch {
	case misery && improvedFaerieFire:
		// one exclusive effect, so they don't stack
		return "Misery or Improved Faerie Fire"
	case misery:
		return "Misery"
	case improvedFaerieFire:
		return "Improved Faerie Fire"
	}
	return "Target debuffs"
}

// racialTraits is the race whose racials the player has, which mod-racial-trait-swap can set apart
// from its race.
func racialTraits(player *proto.Player) proto.Race {
	if traits := player.GetRacialTraits(); traits != proto.Race_RaceUnknown {
		return traits
	}
	return player.GetRace()
}

// raceName is "Night Elf" for RaceNightElf.
func raceName(race proto.Race) string {
	return spaced(strings.TrimPrefix(race.String(), "Race"))
}

func weaponName(weapon proto.WeaponType) string {
	if weapon == proto.WeaponType_WeaponTypeFist {
		return "fist weapon"
	}
	return strings.ToLower(spaced(strings.TrimPrefix(weapon.String(), "WeaponType")))
}

func handName(hand proto.StatCapHand) string {
	if hand == proto.StatCapHand_StatCapHandOff {
		return "off hand"
	}
	return "main hand"
}

func spaced(camel string) string {
	var b strings.Builder
	for i, r := range camel {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
