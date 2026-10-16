package core

import (
	"fmt"
	"slices"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/serverdata"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The server's threat arithmetic: ThreatManager::CalculateModifiedThreat on each hit's damage or healing,
// plus the bonus Spell::HandleThreatSpells adds once per cast and Spell::EffectThreat's flat amount. A spell
// whose ThreatMultiplier is 0 makes no threat at all, bonus included.

// Attribute bits the bonus reads (SharedDefines.h, SpellInfo.h).
const (
	attr1NoThreat            = 0x00000400
	attr3SuppressTargetProcs = 0x00020000
	attrCuNoInitialThreat    = 0x00000010
	attrCuNegativeEffects    = 0x00007000

	effectThreat = 63 // SPELL_EFFECT_THREAT
)

// spellThreat is what the spell's server data says about its threat, set at registration, and the
// enemies its open casts have hit so far.
type spellThreat struct {
	serverID int32   // the server spell all this is read from, 0 for none
	pct      float64 // spell_threat's pctMod, 0 under SPELL_ATTR1_NO_THREAT

	noBonus     bool    // neither a bonus nor an effect's flat threat
	flat, ap    float64 // the bonus: flatMod and apPctMod, else the spell's level
	assist      bool    // a positive spell's bonus is assist threat
	casterShare bool    // the caster is one of the units the bonus is divided between
	split       float64 // those units for a hit outside a cast, from the server data alone
	effect      float64 // SPELL_EFFECT_THREAT on each enemy hit, unmodified

	healScale float64 // heal threat per point healed before the mods, 0 until the first heal

	// the spell whose open cast takes this one's hits, when the server deals both in one cast
	castOf *Spell

	openCasts int
	hits      []threatHit
}

type threatHit struct {
	target int32 // the enemy's UnitIndex: no pointer, so appending a hit skips the GC write barrier
	landed bool
}

func (spell *Spell) setThreatFromServer(threatSpellID int32, noBonus bool) {
	s := spell.serverSpell
	if threatSpellID != 0 {
		if s = serverdata.SpellByID(threatSpellID); s == nil {
			panic(fmt.Sprintf("ThreatSpellID %d of spell %s isn't in the server data", threatSpellID, spell.ActionID))
		}
	}
	spell.threat = serverThreat(spell.Unit, s, noBonus)
}

// serverThreat is what the server spell s makes of a cast by unit.
func serverThreat(unit *Unit, s *serverdata.Spell, noBonus bool) spellThreat {
	threat := spellThreat{pct: 1, noBonus: true, split: 1}
	if s == nil {
		return threat
	}
	threat.serverID = s.ID
	// ThreatManager::AddThreat and ForwardThreatForAssistingMe drop all of it: damage, healing, bonus
	if s.Attributes[1]&attr1NoThreat != 0 {
		threat.pct = 0
		return threat
	}
	t := serverdata.ThreatBySpellID(s.ID)
	threat.pct = float64(t.Pct())
	if noBonus || unit.Type == EnemyUnit || unit.Env == nil {
		return threat
	}
	threat.effect = serverThreatEffect(s)
	if s.Attributes[3]&attr3SuppressTargetProcs == 0 {
		switch {
		case t != nil && t.Row != 0:
			threat.flat, threat.ap = float64(t.FlatMod), float64(t.APPctMod)
		case s.AttributesCu&attrCuNoInitialThreat == 0:
			threat.flat = float64(s.SpellLevel)
		}
	}
	threat.noBonus = threat.flat == 0 && threat.ap == 0 && threat.effect == 0
	// the NEGATIVE_EFFn bits are _IsPositiveSpell's answer per effect; unlike IsPositive they ignore
	// spell_custom_attr's POSITIVE_EFFn (Envenom)
	threat.assist = s.AttributesCu&attrCuNegativeEffects == 0
	threat.casterShare = t != nil && t.Targets&serverdata.ThreatTargetsCaster != 0
	threat.split = float64(threatSplit(s, t, unit.Env.GetNumTargets()))
	return threat
}

// serverThreatEffect is SPELL_EFFECT_THREAT's amount at a level 80 caster, the mean of its roll: Feint's
// is a cut. Spell::EffectThreat adds it to each enemy hit, unmodified.
func serverThreatEffect(s *serverdata.Spell) float64 {
	amount := 0.0
	for i, e := range s.Effects {
		if e.Effect == effectThreat {
			lo, hi := s.EffectRange(i)
			amount += float64(lo+hi) / 2
		}
	}
	return amount
}

// threatSplit is how many units a cast selects with every enemy in range, by the server data: the enemies
// its area or chain effects reach, else one, plus the caster when an effect lands on it. Spell mods on
// the jump targets aren't in it.
func threatSplit(s *serverdata.Spell, t *serverdata.Threat, numTargets int32) int32 {
	var targets serverdata.ThreatTargets
	if t != nil {
		targets = t.Targets
	}
	enemies := int32(1)
	if targets&serverdata.ThreatTargetsEnemyArea != 0 {
		enemies = numTargets
		if s.MaxTargets > 0 {
			enemies = min(enemies, s.MaxTargets)
		}
	}
	for _, e := range s.Effects {
		if e.ChainTargets > 1 {
			enemies = max(enemies, min(numTargets, e.ChainTargets))
		}
	}
	enemies = max(enemies, 1)
	if targets&serverdata.ThreatTargetsCaster != 0 {
		enemies++
	}
	return enemies
}

// ThreatFromDamage is CalculateModifiedThreat on a hit's damage or healing, 0 unless it landed. The
// cast's bonus isn't in it.
func (spell *Spell) ThreatFromDamage(outcome HitOutcome, damage float64) float64 {
	if !outcome.Matches(OutcomeLanded) {
		return 0
	}
	return spell.modifiedThreat(damage)
}

// ThreatFromHealing is the threat the spell's heals make of gain health, before it's shared over the enemies.
func (spell *Spell) ThreatFromHealing(gain float64) float64 {
	return spell.modifiedThreat(gain * healThreatScale(spell.Unit))
}

// ThreatServerSpellID is the server spell the spell's threat is read from: its own, its ThreatSpellID, or 0.
func (spell *Spell) ThreatServerSpellID() int32 {
	return spell.threat.serverID
}

// modifiedThreat is spell_threat's pctMod, then the spell's threat mods, then the unit's school auras.
func (spell *Spell) modifiedThreat(amount float64) float64 {
	return amount * spell.threat.pct * spell.ThreatMultiplier * spell.Unit.ThreatSchoolMultiplier(spell.SpellSchool)
}

// main hand attack power without the creature type bonus (Unit::GetTotalAttackPowerValue)
func (spell *Spell) threatBonusBase() float64 {
	return spell.threat.bonusBase(spell.Unit)
}

func (t *spellThreat) bonusBase(unit *Unit) float64 {
	return t.flat + t.ap*unit.stats[stats.AttackPower]
}

func (spell *Spell) hasEnemyThreatBonus() bool {
	return !spell.threat.noBonus && !spell.threat.assist && spell.ThreatMultiplier != 0
}

// threatBonus is the bonus a hit that isn't a periodic tick adds to its own threat. A hit inside a cast
// adds nothing yet: the cast hands out its bonus at the end, once it knows how many enemies it reached.
// Kept small enough to inline, so a spell without an enemy bonus pays no call.
func (spell *Spell) threatBonus(result *SpellResult) float64 {
	if spell.threat.noBonus || spell.threat.assist {
		return 0
	}
	return spell.enemyThreatBonus(result)
}

func (spell *Spell) enemyThreatBonus(result *SpellResult) float64 {
	if spell.ThreatMultiplier == 0 || result.Target.Type != EnemyUnit {
		return 0
	}
	if c := spell.threat.castOf; c != nil && c.threat.openCasts > 0 {
		c.threat.hits = append(c.threat.hits, threatHit{result.Target.UnitIndex, result.Landed()})
		return 0
	}
	if spell.threat.openCasts > 0 {
		spell.threat.hits = append(spell.threat.hits, threatHit{result.Target.UnitIndex, result.Landed()})
		return 0
	}
	if !result.Landed() {
		return 0
	}
	bonus := spell.threatBonusBase() / spell.threat.split
	if spell.threat.effect != 0 {
		// HandleThreatSpells runs before the effects
		bonus += spell.threatEffectOn(result.Target.UnitIndex, &spell.threat, bonus)
	}
	return bonus
}

// SetThreatCastOf puts the spell's hits in main's open cast, so main's bonus is split over the enemies
// both reach: the server deals them in one cast (Heart Strike's second target).
func (spell *Spell) SetThreatCastOf(main *Spell) {
	spell.threat.castOf = main
}

// threatEffectOn is t's SPELL_EFFECT_THREAT on the target, pending being threat not dealt yet. A cut leaves
// the unit's threat there at 0 at the least (ThreatReference::AddThreat).
func (spell *Spell) threatEffectOn(target int32, t *spellThreat, pending float64) float64 {
	if t.effect >= 0 {
		return t.effect
	}
	return max(t.effect, -(spell.Unit.threatOn(target) + pending))
}

// threatOn is the threat the unit's spells and gains made on the target so far this iteration.
func (unit *Unit) threatOn(target int32) float64 {
	threat := 0.0
	for _, s := range unit.Spellbook {
		for _, metrics := range s.splitSpellMetrics {
			threat += metrics[target].TotalThreat
		}
	}
	for _, rm := range unit.Metrics.resources {
		threat += rm.threat / float64(len(unit.Env.Encounter.TargetUnits))
	}
	return threat
}

// LandThreatBonus counts a cast landing on an enemy without a hit of its own, like the enemies a
// persistent area aura selects as it's cast, so the enemy gets a share of the cast's bonus.
func (spell *Spell) LandThreatBonus(sim *Simulation, target *Unit) {
	if !spell.hasEnemyThreatBonus() || target.Type != EnemyUnit {
		return
	}
	if spell.threat.openCasts > 0 {
		spell.threat.hits = append(spell.threat.hits, threatHit{target.UnitIndex, true})
	} else if sim.CurrentTime >= 0 {
		spell.SpellMetrics[target.UnitIndex].TotalThreat += spell.threatBonusBase() / spell.threat.split
	}
}

// openThreatCast starts collecting the cast's hits, and adds a positive spell's bonus. It and
// closeThreatCast run on every cast: keep both inlinable.
func (spell *Spell) openThreatCast(sim *Simulation) int {
	if spell.threat.assist && !spell.threat.noBonus {
		spell.addAssistThreat(sim)
	}
	spell.threat.openCasts++
	return len(spell.threat.hits)
}

func (spell *Spell) closeThreatCast(sim *Simulation, start int) {
	spell.threat.openCasts--
	if len(spell.threat.hits) > start {
		spell.shareThreatBonus(sim, start)
	}
}

// shareThreatBonus splits the bonus between the enemies the cast reached and the caster if an effect
// lands on it. An enemy it missed every time gets no share, but still counts.
func (spell *Spell) shareThreatBonus(sim *Simulation, start int) {
	t := &spell.threat
	hits := t.hits[start:]
	t.hits = t.hits[:start]
	spell.shareThreat(sim, t, hits)
}

// shareThreat hands out the bonus of one cast of t's server spell, which reached these enemies, onto the
// spell's metrics, then each landed enemy's SPELL_EFFECT_THREAT.
func (spell *Spell) shareThreat(sim *Simulation, t *spellThreat, hits []threatHit) {
	if sim.CurrentTime < 0 {
		return
	}

	units := 0
	if t.casterShare {
		units++
	}
	for i := range hits {
		if !threatHitSeen(hits, i) {
			units++
		}
	}
	share := t.bonusBase(spell.Unit) / float64(units)
	for i, hit := range hits {
		if threatHitSeen(hits, i) || !threatHitLands(hits[i:], hit.target) {
			continue
		}
		spell.SpellMetrics[hit.target].TotalThreat += share
		if sim.Log != nil {
			// UnitIndex is the unit's place in AllUnits
			spell.Unit.Log(sim, "%s %s threat bonus %0.3f, 1 of %d units", sim.AllUnits[hit.target].LogLabel(), spell.ActionID, share, units)
		}
		if t.effect != 0 {
			effect := spell.threatEffectOn(hit.target, t, 0)
			spell.SpellMetrics[hit.target].TotalThreat += effect
			if sim.Log != nil {
				spell.Unit.Log(sim, "%s %s threat effect %0.3f", sim.AllUnits[hit.target].LogLabel(), spell.ActionID, effect)
			}
		}
	}
}

// threatHitSeen says whether a hit before hits[i] has its enemy.
func threatHitSeen(hits []threatHit, i int) bool {
	for _, h := range hits[:i] {
		if h.target == hits[i].target {
			return true
		}
	}
	return false
}

func threatHitLands(hits []threatHit, target int32) bool {
	for _, h := range hits {
		if h.target == target && h.landed {
			return true
		}
	}
	return false
}

// addAssistThreat hands a positive spell's bonus, modified, to the enemies in even shares
// (ThreatManager::ForwardThreatForAssistingMe). Nothing's fighting the unit before the pull.
func (spell *Spell) addAssistThreat(sim *Simulation) {
	spell.addAssistThreatOf(sim, &spell.threat)
}

// addAssistThreatOf is addAssistThreat for t, the threat of the server spell the spell casts.
func (spell *Spell) addAssistThreatOf(sim *Simulation, t *spellThreat) {
	if t.noBonus || !t.assist || spell.ThreatMultiplier == 0 || sim.CurrentTime < 0 {
		return
	}
	enemies := sim.Encounter.TargetUnits
	threat := t.bonusBase(spell.Unit) * t.pct * spell.ThreatMultiplier * spell.Unit.ThreatSchoolMultiplier(spell.SpellSchool) /
		float64(len(enemies))
	for _, enemy := range enemies {
		spell.SpellMetrics[enemy.UnitIndex].TotalThreat += threat
	}
	if sim.Log != nil {
		spell.Unit.Log(sim, "%s assist threat %0.3f on each of %d enemies", spell.ActionID, threat, len(enemies))
	}
}

// healThreat adds a heal's threat and returns it: half the health it restored, half again for a paladin,
// modified like a hit, shared out over the enemies like assist threat (Spell::DoAllEffectOnTarget,
// AuraEffect::HandlePeriodicHealAurasTick). None before the pull.
func (spell *Spell) healThreat(sim *Simulation, gain float64) float64 {
	if gain <= 0 || spell.ThreatMultiplier == 0 || spell.threat.pct == 0 || sim.CurrentTime < 0 {
		return 0
	}
	if spell.threat.healScale == 0 {
		spell.threat.healScale = healThreatScale(spell.Unit)
	}
	enemies := sim.Encounter.TargetUnits
	threat := spell.modifiedThreat(gain * spell.threat.healScale)
	for _, enemy := range enemies {
		spell.SpellMetrics[enemy.UnitIndex].TotalThreat += threat / float64(len(enemies))
	}
	return threat
}

// healThreatScale is the part of the health a heal restores that the server makes threat of: half, a
// quarter for a paladin.
func healThreatScale(unit *Unit) float64 {
	if unit.Env != nil && unit.Env.Raid != nil {
		if agent := unit.Env.Raid.GetPlayerFromUnit(unit); agent != nil &&
			agent.GetCharacter().Class == proto.Class_ClassPaladin {
			return 0.25
		}
	}
	return 0.5
}

// EnergizeKind is how the server works out an energize's threat: half of it, unmodified, shared over the
// enemies fighting the unit as assist threat (ThreatManager::ForwardThreatForAssistingMe).
type EnergizeKind uint8

const (
	// Unit::EnergizeBySpell, from SPELL_EFFECT_ENERGIZE, ENERGIZE_PCT and POWER_DRAIN's gain: half the
	// amount asked for, what the power cap cuts off included
	EnergizeBySpell EnergizeKind = iota + 1
	// a PERIODIC_ENERGIZE or OBS_MOD_POWER tick: half the power gained
	EnergizeByTick
	// EnergizeBySpell of a SPELLFAMILY_POTION spell, which also energizes a power the unit isn't using
	// (a mana potion in cat form)
	EnergizeByPotion
)

// gainThreat is the threat the server makes of the gains through one ResourceMetrics, an energize's or a
// heal's.
type gainThreat struct {
	perPoint  float64 // energize: the threat per point of the sim's power
	requested bool    // energize: on the amount asked for, not the amount gained
	// energize: Spell::EffectEnergize and the periodic ticks skip a player's power other than its current
	// one, potions excepted
	bar      PowerBarType
	anyPower bool

	heal *serverdata.Spell // the heal's server spell, read at the first heal
	// what heal makes of a cast by the unit; healScale stays 0 until then
	healThreat  spellThreat
	healSchool  SpellSchool
	healScale   float64
	castPerGain bool // each gain is a cast of heal, so it adds the cast's bonus too
}

// EnergizeThreat makes each power gain through the metrics an energize the unit casts on itself. Only for
// energizes without SPELL_ATTR1_NO_THREAT: a gain the server makes through ModifyPower or SetPower (regen,
// rage from damage, a rune spell's runic power, refunds) makes no threat, and one another unit casts makes
// that unit's.
func (metrics *ResourceMetrics) EnergizeThreat(kind EnergizeKind) *ResourceMetrics {
	bar, perPoint := metrics.energizePower()
	metrics.gainThreat = &gainThreat{perPoint: perPoint, requested: kind != EnergizeByTick, bar: bar,
		anyPower: kind == EnergizeByPotion}
	return metrics
}

// energizePower is the power the metrics count and an energize's threat per point of it.
func (metrics *ResourceMetrics) energizePower() (bar PowerBarType, perPoint float64) {
	switch metrics.Type {
	case proto.ResourceType_ResourceTypeMana:
		return ManaBar, 0.5
	case proto.ResourceType_ResourceTypeEnergy:
		return EnergyBar, 0.5
	case proto.ResourceType_ResourceTypeRage:
		// the server counts rage and runic power in tenths
		return RageBar, 5
	case proto.ResourceType_ResourceTypeRunicPower:
		return RunicPower, 5
	default:
		panic(fmt.Sprintf("energize threat on %v metrics of %s", metrics.Type, metrics.ActionID))
	}
}

// AddEnergizeTickThreat books a tick of a periodic energize the unit put on someone else, itself included,
// under the metrics' action: half of gain, the power the tick gave its target, unmodified. The server hands
// it to the aura's caster on the enemies fighting the target (AuraEffect::HandlePeriodicEnergizeAuraTick),
// and in the sim every enemy fights the whole raid. The caller checks the target takes the tick at all.
func (unit *Unit) AddEnergizeTickThreat(sim *Simulation, metrics *ResourceMetrics, gain float64) {
	if sim.CurrentTime < 0 || gain <= 0 {
		return
	}
	_, perPoint := metrics.energizePower()
	metrics.addThreat(sim, unit, gain*perPoint)
}

// addThreat books assist threat the unit made under the metrics' action.
func (metrics *ResourceMetrics) addThreat(sim *Simulation, unit *Unit, threat float64) {
	if threat == 0 {
		return
	}
	metrics.threat += threat
	if sim.Log != nil {
		unit.Log(sim, "%s assist threat %0.3f over %d enemies", metrics.ActionID, threat, len(sim.Encounter.TargetUnits))
	}
}

// CastThreat is the cast bonus of a server spell the sim puts on with no cast of its own: a proc's aura, the
// aura another spell triggers, an area buff a proc casts. Positive spells only, whose bonus is assist threat.
type CastThreat struct {
	unit  *Unit
	spell *serverdata.Spell

	// what spell makes of a cast by unit, read at the first cast
	threat spellThreat
	school SpellSchool
	read   bool
}

// NewCastThreat reads server spell spellID, which the unit casts on itself or its raid.
func (unit *Unit) NewCastThreat(spellID int32) *CastThreat {
	s := serverdata.SpellByID(spellID)
	if s == nil {
		panic(fmt.Sprintf("cast spell %d isn't in the server data", spellID))
	}
	cast := &CastThreat{unit: unit, spell: s}
	unit.castThreats = append(unit.castThreats, cast)
	return cast
}

// Add books one cast's bonus under the metrics' action, modified like a hit of the spell and shared over the
// enemies (Spell::HandleThreatSpells, ThreatManager::ForwardThreatForAssistingMe). None before the pull.
func (cast *CastThreat) Add(sim *Simulation, metrics *ResourceMetrics) {
	if sim.CurrentTime < 0 {
		return
	}
	metrics.addThreat(sim, cast.unit, cast.bonus())
}

// bonus is what one cast adds now, summed over the enemies.
func (cast *CastThreat) bonus() float64 {
	t := &cast.threat
	if !cast.read {
		*t = serverThreat(cast.unit, cast.spell, false)
		if !t.noBonus && !t.assist {
			panic(fmt.Sprintf("cast threat of %d, which isn't a positive spell", cast.spell.ID))
		}
		cast.school = SpellSchoolFromServerMask(cast.spell.SchoolMask)
		cast.read = true
	}
	if t.noBonus {
		return 0
	}
	return t.bonusBase(cast.unit) * t.pct * cast.unit.ThreatSchoolMultiplier(cast.school)
}

// CastThreatOf is what one cast of server spell spellID adds now through the unit's CastThreat, summed over the
// enemies; ok is false when the unit has none.
func (unit *Unit) CastThreatOf(spellID int32) (bonus float64, ok bool) {
	for _, cast := range unit.castThreats {
		// spellID first: TestHardCodedItemIDsCoverTheSim reads `.ID == x` as an item id check
		if spellID == cast.spell.ID {
			return cast.bonus(), true
		}
	}
	return 0, false
}

// AddEnergizeThreat makes an energize's threat the sim books as part of another gain, like Chill of the
// Grave's runic power in the strike's own: amount is what the server's energize asks for.
func (unit *Unit) AddEnergizeThreat(sim *Simulation, metrics *ResourceMetrics, amount float64) {
	if metrics.gainThreat == nil || !metrics.gainThreat.requested {
		panic(fmt.Sprintf("%s isn't an EnergizeBySpell's metrics", metrics.ActionID))
	}
	unit.addGainThreat(sim, metrics, amount, 0)
}

// HealThreat makes each health gain through the metrics a heal of the server spell spellID the unit casts
// on itself: half the health gained (a quarter for a paladin), plus the cast's bonus when each gain is its
// own cast, both modified like a hit of that spell and shared over the enemies as assist threat.
func (unit *Unit) HealThreat(metrics *ResourceMetrics, spellID int32) *ResourceMetrics {
	if metrics.Type != proto.ResourceType_ResourceTypeHealth {
		panic(fmt.Sprintf("heal threat on %v metrics of %s", metrics.Type, metrics.ActionID))
	}
	s := serverdata.SpellByID(spellID)
	if s == nil {
		panic(fmt.Sprintf("heal spell %d of %s isn't in the server data", spellID, metrics.ActionID))
	}
	metrics.gainThreat = &gainThreat{heal: s}
	return metrics
}

// SPELL_EFFECT_HEAL, HEAL_MAX_HEALTH and HEAL_PCT: one cast, one heal
var castHealEffects = []int32{10, 67, 136}

// addGainThreat makes the threat of a gain of amount, gain of it landing, through the metrics. Nothing's
// fighting the unit before the pull.
func (unit *Unit) addGainThreat(sim *Simulation, metrics *ResourceMetrics, amount, gain float64) {
	if sim.CurrentTime < 0 {
		return
	}
	// a gain past a max cut by an expired buff comes out below 0, which the server's capped bars never have
	gain = max(gain, 0)
	g := metrics.gainThreat
	var threat float64
	if g.heal == nil {
		if g.anyPower && !unit.hasPower(g.bar) || !g.anyPower && unit.currentPowerBar != g.bar {
			return
		}
		if g.requested {
			threat = amount * g.perPoint
		} else {
			threat = gain * g.perPoint
		}
	} else {
		threat = g.healThreatOf(unit, gain)
	}
	metrics.addThreat(sim, unit, threat)
}

func (unit *Unit) hasPower(bar PowerBarType) bool {
	switch bar {
	case ManaBar:
		return unit.HasManaBar() && unit.MaxMana() > 0
	case EnergyBar:
		return unit.HasEnergyBar()
	case RageBar:
		return unit.HasRageBar()
	default:
		return unit.HasRunicPowerBar()
	}
}

func (g *gainThreat) healThreatOf(unit *Unit, gain float64) float64 {
	if g.healScale == 0 {
		g.healThreat = serverThreat(unit, g.heal, false)
		g.healSchool = SpellSchoolFromServerMask(g.heal.SchoolMask)
		g.healScale = healThreatScale(unit)
		for _, e := range g.heal.Effects {
			g.castPerGain = g.castPerGain || slices.Contains(castHealEffects, e.Effect)
		}
	}
	t := &g.healThreat
	threat := gain * g.healScale
	if g.castPerGain && !t.noBonus {
		threat += t.bonusBase(unit)
	}
	return threat * t.pct * unit.ThreatSchoolMultiplier(g.healSchool)
}

// HealThreatOf splits the threat a gain of health through a HealThreat of server spell spellID makes into the
// heal's part and the cast's bonus; ok is false when the unit has no such heal.
func (unit *Unit) HealThreatOf(spellID int32, gain float64) (heal, bonus float64, ok bool) {
	for _, rm := range unit.Metrics.resources {
		// spellID first: TestHardCodedItemIDsCoverTheSim reads `.ID == x` as an item id check
		if g := rm.gainThreat; g != nil && g.heal != nil && spellID == g.heal.ID {
			bonus = g.healThreatOf(unit, 0)
			return g.healThreatOf(unit, gain) - bonus, bonus, true
		}
	}
	return 0, 0, false
}

// GainThreat is the assist threat booked under the action's resource metrics so far this iteration: its
// energizes, heals and aura casts, summed over the enemies.
func (unit *Unit) GainThreat(actionID ActionID) float64 {
	threat := 0.0
	for _, rm := range unit.Metrics.resources {
		if rm.ActionID == actionID {
			threat += rm.threat
		}
	}
	return threat
}

// EnergizeThreatOf splits the assist threat the action's energizes made so far this iteration: bySpell is its
// EnergizeBySpell and potion gains', byTick what its tick metrics hold, an aura cast's bonus booked there
// included, and tickGain the power those ticks gave.
func (unit *Unit) EnergizeThreatOf(actionID ActionID) (bySpell, byTick, tickGain float64) {
	for _, rm := range unit.Metrics.resources {
		g := rm.gainThreat
		if rm.ActionID != actionID || g == nil || g.heal != nil {
			continue
		}
		if g.requested {
			bySpell += rm.threat
		} else {
			byTick += rm.threat
			tickGain += rm.ActualGainForCurrentIteration()
		}
	}
	return bySpell, byTick, tickGain
}

// doneGainThreat puts this iteration's gain threat in the unit's metrics, each under its gain's action and
// shared evenly over the enemies.
func (unit *Unit) doneGainThreat(sim *Simulation) {
	enemies := sim.Encounter.TargetUnits
	for _, rm := range unit.Metrics.resources {
		if rm.threat == 0 {
			continue
		}
		action := unit.Metrics.actions[rm.ActionID]
		if action == nil {
			action = &ActionMetrics{}
			unit.Metrics.actions[rm.ActionID] = action
		}
		if len(action.Targets) == 0 {
			action.Targets = make([]TargetedActionMetrics, len(unit.AttackTables))
			for i := range action.Targets {
				action.Targets[i].UnitIndex = unit.AttackTables[i].Defender.UnitIndex
			}
		}
		for _, enemy := range enemies {
			action.Targets[enemy.UnitIndex].Threat += rm.threat / float64(len(enemies))
		}
		unit.Metrics.threat.Total += rm.threat
		rm.threat = 0
	}
}

// ThreatTick is a server spell an aura casts on each tick, which adds its own bonus split over the enemies
// that tick reaches (HandleThreatSpells runs for every cast): Bladestorm's Whirlwind, Volley's arrows, or
// as assist threat for a positive one: Penance's heals. The tick's damage or healing stays the aura
// spell's, so the bonus goes on that spell too.
type ThreatTick struct {
	spell  *Spell
	threat spellThreat
}

// NewThreatTick reads the server spell an aura of the spell casts each tick.
func (spell *Spell) NewThreatTick(serverSpellID int32) *ThreatTick {
	s := serverdata.SpellByID(serverSpellID)
	if s == nil {
		panic(fmt.Sprintf("tick spell %d of %s isn't in the server data", serverSpellID, spell.ActionID))
	}
	return &ThreatTick{spell: spell, threat: serverThreat(spell.Unit, s, false)}
}

// Hit counts an enemy the tick reached, missed or not. A positive tick spell's needs none.
func (tick *ThreatTick) Hit(result *SpellResult) {
	if tick.threat.noBonus || tick.threat.assist || tick.spell.ThreatMultiplier == 0 || result.Target.Type != EnemyUnit {
		return
	}
	tick.threat.hits = append(tick.threat.hits, threatHit{result.Target.UnitIndex, result.Landed()})
}

// Done hands out the tick's bonus to the enemies it hit, or a positive tick spell's as assist threat.
func (tick *ThreatTick) Done(sim *Simulation) {
	if tick.threat.assist {
		tick.spell.addAssistThreatOf(sim, &tick.threat)
		return
	}
	if len(tick.threat.hits) == 0 {
		return
	}
	tick.spell.shareThreat(sim, &tick.threat, tick.threat.hits)
	tick.threat.hits = tick.threat.hits[:0]
}

// ThreatEffect is the spell's SPELL_EFFECT_THREAT on each enemy it hits, before a cut is held to the
// threat there.
func (spell *Spell) ThreatEffect() float64 {
	return spell.threat.effect
}

// threatSchoolMod is a SPELL_AURA_MOD_THREAT aura on some schools only.
type threatSchoolMod struct {
	school     SpellSchool
	multiplier float64
}

// AddThreatSchoolMultiplier adds a SPELL_AURA_MOD_THREAT aura covering some schools. It multiplies a
// spell's threat once if the spell has any of them (Unit::GetTotalAuraMultiplierByMiscMask), so a mixed
// school spell isn't hit twice. An aura on every school goes on PseudoStats.ThreatMultiplier instead.
func (unit *Unit) AddThreatSchoolMultiplier(school SpellSchool, multiplier float64) {
	unit.threatSchoolMods = append(unit.threatSchoolMods, threatSchoolMod{school, multiplier})
}

// RemoveThreatSchoolMultiplier takes off an aura AddThreatSchoolMultiplier added.
func (unit *Unit) RemoveThreatSchoolMultiplier(school SpellSchool, multiplier float64) {
	i := slices.Index(unit.threatSchoolMods, threatSchoolMod{school, multiplier})
	if i < 0 {
		panic(fmt.Sprintf("%s has no school threat aura %v x%v to remove", unit.Label, school, multiplier))
	}
	unit.threatSchoolMods = slices.Delete(unit.threatSchoolMods, i, i+1)
}

// ThreatSchoolMultiplier is CalculateModifiedThreat's school part for a spell of these schools. A spell with no
// school takes none: GetTotalAuraMultiplierByMiscMask finds no MOD_THREAT aura for an empty mask.
func (unit *Unit) ThreatSchoolMultiplier(school SpellSchool) float64 {
	if school == SpellSchoolNone {
		return 1
	}
	m := unit.PseudoStats.ThreatMultiplier
	for _, mod := range unit.threatSchoolMods {
		if mod.school&school != 0 {
			m *= mod.multiplier
		}
	}
	return m
}
