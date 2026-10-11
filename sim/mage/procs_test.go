package mage

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	googleProto "google.golang.org/protobuf/proto"
)

var procLogLine = regexp.MustCompile(`^\[(-?[\d.]+)\] \[([^\]]*)\] (.*)$`)

type procLogEntry struct {
	at   time.Duration
	unit string
	text string
}

func procMage(talents, gear, apl string, spec *proto.Player_Mage) *proto.Player {
	return &proto.Player{
		Race:               proto.Race_RaceTroll,
		Class:              proto.Class_ClassMage,
		Equipment:          core.GetGearSet("../../ui/mage/gear_sets", gear).GearSet,
		Consumes:           &proto.Consumes{},
		Spec:               spec,
		Rotation:           core.APLRotationFromJsonString(apl),
		TalentsString:      talents,
		DistanceFromTarget: 30,
	}
}

// castApl takes condition and spell id pairs, "" for no condition, and casts the first spell whose condition holds.
func castApl(steps ...string) string {
	var actions []string
	for i := 0; i < len(steps); i += 2 {
		action := `"castSpell": {"spellId": {"spellId": ` + steps[i+1] + `}}`
		if steps[i] != "" {
			action = `"condition": ` + steps[i] + `, ` + action
		}
		actions = append(actions, `{"action": {`+action+`}}`)
	}
	return `{"type": "TypeAPL", "priorityList": [` + strings.Join(actions, ", ") + `]}`
}

func auraUp(id int) string {
	return `{"auraIsActive": {"auraId": {"spellId": ` + strconv.Itoa(id) + `}}}`
}

func runProcLog(t *testing.T, player *proto.Player, seconds float64, seed int64) []procLogEntry {
	t.Helper()
	return runProcLogOn(t, player, nil, seconds, seed)
}

// runProcLogOn is runProcLog on a server with these settings, nil for the live ones.
func runProcLogOn(t *testing.T, player *proto.Player, settings *proto.ServerSettings, seconds float64, seed int64) []procLogEntry {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Duration = seconds
	encounter.ServerSettings = settings
	return runProcLogIn(t, player, encounter, seed)
}

// runProcLogIn is the debug log of one iteration of the player in the encounter.
func runProcLogIn(t *testing.T, player *proto.Player, encounter *proto.Encounter, seed int64) []procLogEntry {
	t.Helper()
	res := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, Debug: true, RandomSeed: seed},
	})
	if res.ErrorResult != "" {
		t.Fatal(res.ErrorResult)
	}
	var entries []procLogEntry
	for _, line := range strings.Split(res.Logs, "\n") {
		m := procLogLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		secs, _ := strconv.ParseFloat(m[1], 64)
		entries = append(entries, procLogEntry{time.Duration(secs*1000+0.5) * time.Millisecond, strings.TrimSpace(m[2]), m[3]})
	}
	return entries
}

// logTimes are the times of the log lines that start with any of the texts.
func logTimes(entries []procLogEntry, texts ...string) []time.Duration {
	var out []time.Duration
	for _, e := range entries {
		if slices.ContainsFunc(texts, func(text string) bool { return strings.HasPrefix(e.text, text) }) {
			out = append(out, e.at)
		}
	}
	return out
}

func gainedOrRefreshed(id string) []string {
	return []string{"Aura gained: {SpellID: " + id + "}", "Aura refreshed: {SpellID: " + id + "}"}
}

// T10's 2 pieces (Pushing the Limit) come from a Hot Streak or Missile Barrage used up, never from one timing
// out (Aura::HandleAuraSpecificMods); a Missile Barrage gives it again when the Arcane Missiles channel ends
// (spell_mage_arcane_missiles).
func TestPushingTheLimitOnlyWhenUsed(t *testing.T) {
	t10 := gainedOrRefreshed("70752")
	const fireball, pyroblast, arcaneBlast, arcaneMissiles = "42833", "42891", "42897", "42846"

	for seed := int64(1); seed <= 3; seed++ {
		entries := runProcLog(t, procMage(FireTalents, "p5_bis_fire", castApl("", fireball), PlayerOptionsFire), 120, seed)
		if streaks := logTimes(entries, "Aura gained: {SpellID: 44448}"); len(streaks) == 0 {
			t.Fatalf("seed %d: no Hot Streak in 120 s of Fireballs", seed)
		}
		if got := logTimes(entries, t10...); len(got) != 0 {
			t.Errorf("seed %d: Hot Streak never used, but Pushing the Limit at %v", seed, got)
		}

		entries = runProcLog(t, procMage(FireTalents, "p5_bis_fire", castApl(auraUp(44448), pyroblast, "", fireball),
			PlayerOptionsFire), 120, seed)
		pyros := logTimes(entries, "Completed cast {SpellID: "+pyroblast+"}")
		got := logTimes(entries, t10...)
		if len(got) == 0 || len(got) != len(pyros) {
			t.Errorf("seed %d: Pushing the Limit at %v, Pyroblasts on Hot Streak at %v", seed, got, pyros)
		}
		for _, at := range got {
			if !slices.Contains(pyros, at) {
				t.Errorf("seed %d: Pushing the Limit at %v, with no Pyroblast", seed, at)
			}
		}

		entries = runProcLog(t, procMage(ArcaneTalents, "p5_bis_arcane", castApl("", arcaneBlast), PlayerOptionsArcane), 60, seed)
		if barrages := logTimes(entries, "Aura gained: {SpellID: 44401}"); len(barrages) == 0 {
			t.Fatalf("seed %d: no Missile Barrage in 60 s of Arcane Blasts", seed)
		}
		if got := logTimes(entries, t10...); len(got) != 0 {
			t.Errorf("seed %d: Missile Barrage never used, but Pushing the Limit at %v", seed, got)
		}

		entries = runProcLog(t, procMage(ArcaneTalents, "p5_bis_arcane", castApl(auraUp(44401), arcaneMissiles, "", arcaneBlast),
			PlayerOptionsArcane), 60, seed)
		casts := logTimes(entries, "Completed cast {SpellID: "+arcaneMissiles+"}")
		ends := logTimes(entries, "Aura faded: {SpellID: "+arcaneMissiles+"}")
		atCast, atEnd := 0, 0
		for _, at := range logTimes(entries, t10...) {
			switch {
			case slices.Contains(casts, at):
				atCast++
			case slices.Contains(ends, at):
				atEnd++
			default:
				t.Errorf("seed %d: Pushing the Limit at %v, neither an Arcane Missiles cast nor its end", seed, at)
			}
		}
		// the fight can end mid-channel
		if atCast == 0 || atEnd < atCast-1 || atEnd > atCast {
			t.Errorf("seed %d: Pushing the Limit %d times at a Missile Barrage cast, %d at its channel's end", seed, atCast, atEnd)
		}
	}
}

// spell_proc 44545 rolls Fingers of Frost at Frostbolt's cast, 15% at 2 points, not when it lands.
func TestFingersOfFrostRollsAtTheCast(t *testing.T) {
	casts, procs := 0, 0
	for seed := int64(1); seed <= 3; seed++ {
		entries := runProcLog(t, procMage(FrostTalents, "p3_frost_alliance", castApl("", "42842"), PlayerOptionsFrost), 180, seed)
		cast := logTimes(entries, "Completed cast {SpellID: 42842}")
		casts += len(cast)
		for _, at := range logTimes(entries, gainedOrRefreshed("44545")...) {
			procs++
			if !slices.Contains(cast, at) {
				t.Errorf("seed %d: Fingers of Frost at %v, not at a Frostbolt's cast", seed, at)
			}
		}
	}
	if rate := float64(procs) / float64(casts); rate < 0.08 || rate > 0.24 {
		t.Errorf("%d procs in %d Frostbolts (%.3f), want about 0.15", procs, casts, rate)
	}
}

func newProcSim(t *testing.T, player *proto.Player, targets ...*proto.Target) (*core.Simulation, *Mage) {
	t.Helper()
	encounter := core.MakeSingleTargetEncounter(0)
	if len(targets) > 0 {
		encounter.Targets = targets
	}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, nil, nil, nil),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].(*Mage)
}

// Shatter's crit under Fingers of Frost reaches only the spells both Shatter's and 44544's class masks
// cover (Blizzard's isn't in Shatter's), not the mage's crit at large. A charge goes per cast of a spell in
// spell_proc 74396's mask, not per Icy Veins, and spell_mage_fingers_of_frost takes none for a channel's cast
// but one for each landed hit of a triggered spell in it, such as Blizzard's ticks.
func TestFingersOfFrostShatterAndCharges(t *testing.T) {
	sim, mage := newProcSim(t, procMage(FrostTalents, "p3_frost_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFrost))
	if mage.Talents.Shatter != 3 || mage.Talents.FingersOfFrost != 2 {
		t.Fatalf("FrostTalents: Shatter %d, Fingers of Frost %d", mage.Talents.Shatter, mage.Talents.FingersOfFrost)
	}
	spells := []*core.Spell{mage.IceLance, mage.Frostbolt, mage.Blizzard}
	before := make([]float64, len(spells))
	for i, spell := range spells {
		before[i] = spell.BonusCritRating
	}
	crit := mage.GetStat(stats.SpellCrit)

	mage.FingersOfFrostAura.Activate(sim)
	mage.FingersOfFrostAura.SetStacks(sim, 2)
	shatter := 50 * core.CritRatingPerCritChance
	for i, want := range []float64{shatter, shatter, 0} {
		if got := spells[i].BonusCritRating - before[i]; got < want-1e-6 || got > want+1e-6 {
			t.Errorf("%v: crit rating %+.2f under Fingers of Frost, want %+.2f", spells[i].ActionID, got, want)
		}
	}
	if got := mage.GetStat(stats.SpellCrit); got != crit {
		t.Errorf("spell crit %v under Fingers of Frost, was %v", got, crit)
	}

	target := sim.GetTargetUnit(0)
	mage.IcyVeins.Cast(sim, target)
	if got := mage.FingersOfFrostAura.GetStacks(); got != 2 {
		t.Errorf("%d charges after Icy Veins, want 2", got)
	}
	mage.IceLance.Cast(sim, target)
	if got := mage.FingersOfFrostAura.GetStacks(); got != 1 {
		t.Errorf("%d charges after Ice Lance, want 1", got)
	}

	sim, mage = newProcSim(t, procMage(FrostTalents, "p3_frost_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFrost))
	target = sim.GetTargetUnit(0)
	mage.FingersOfFrostAura.Activate(sim)
	mage.FingersOfFrostAura.SetStacks(sim, 2)
	mage.Blizzard.Cast(sim, target)
	if got := mage.FingersOfFrostAura.GetStacks(); got != 2 {
		t.Errorf("%d charges after Blizzard's cast, want 2", got)
	}
	tick := mage.GetSpell(core.ActionID{SpellID: 42938})
	for want := int32(2); want > 0; {
		misses := tick.SpellMetrics[target.UnitIndex].Misses
		tick.Cast(sim, target)
		if tick.SpellMetrics[target.UnitIndex].Misses == misses {
			want--
		}
		if got := mage.FingersOfFrostAura.GetStacks(); got != want {
			t.Fatalf("%d charges after a Blizzard tick, want %d", got, want)
		}
	}
}

// Glyph of Frostbolt takes the slow off a Frostbolt at once (Aura::HandleAuraSpecificMods), so Torment the
// Weak has nothing to count on an add.
func TestGlyphOfFrostboltChillsNothing(t *testing.T) {
	for _, glyph := range []bool{false, true} {
		player := procMage(FireTalents, "p3_fire_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFire)
		if glyph {
			player.Glyphs = &proto.Glyphs{Major1: int32(proto.MageMajorGlyph_GlyphOfFrostbolt)}
		}
		sim, mage := newProcSim(t, player, &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid})
		add := sim.GetTargetUnit(0)
		mage.Frostbolt.Cast(sim, add)
		metrics := &mage.Frostbolt.SpellMetrics[add.UnitIndex]
		// a hit's damage counts when it lands
		for metrics.TotalDamage == 0 && metrics.Misses == 0 && !sim.Step() {
		}
		landed := metrics.Hits+metrics.Crits > 0
		if landed == (metrics.Misses > 0) {
			t.Fatalf("glyph %v: Frostbolt hits %d, crits %d, misses %d", glyph, metrics.Hits, metrics.Crits, metrics.Misses)
		}
		if got, want := mage.chills.Get(add).IsActive(), landed && !glyph; got != want {
			t.Errorf("glyph %v, Frostbolt landed %v: add chilled %v, want %v", glyph, landed, got, want)
		}
	}
}

// Torment the Weak against an add: the mage's chills slow it (Unit::SpellPctDamageModsDone's
// HasAuraWithMechanic), but a raid boss is immune to snares.
func TestTormentTheWeakChillsAnAdd(t *testing.T) {
	boss := core.NewDefaultTarget()
	add := &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid}
	sim, mage := newProcSim(t, procMage(FireTalents, "p3_fire_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFire),
		boss, add)
	bossUnit, addUnit := sim.GetTargetUnit(0), sim.GetTargetUnit(1)
	if bossUnit.IsWorldBoss == addUnit.IsWorldBoss {
		t.Fatalf("world boss: boss %v, add %v", bossUnit.IsWorldBoss, addUnit.IsWorldBoss)
	}
	for _, unit := range []*core.Unit{bossUnit, addUnit} {
		if got := mage.tormentTheWeak(unit); got != 1 {
			t.Errorf("%s unslowed: x%v", unit.Label, got)
		}
		mage.chill(sim, unit, 9*time.Second)
	}
	if got := mage.tormentTheWeak(bossUnit); got != 1 {
		t.Errorf("boss after a chill: x%v, want x1", got)
	}
	if got := mage.tormentTheWeak(addUnit); got < 1.12-1e-9 || got > 1.12+1e-9 {
		t.Errorf("add after a chill: x%v, want x1.12", got)
	}
	if got := mage.chills.Get(addUnit).Duration; got != 9*time.Second+time.Duration(mage.Talents.Permafrost)*time.Second {
		t.Errorf("chill lasts %v with %d points of Permafrost", got, mage.Talents.Permafrost)
	}
}

// Snare immunity is the target's creature_immunities mask, not the boss flag, and a snare counts for Torment
// the Weak whoever put it on (Unit::HasAuraWithMechanic looks at every aura on the victim), another mage's
// chill included.
func TestTormentTheWeakReadsImmunitiesAndOtherUnitsSnares(t *testing.T) {
	newMage := func() *proto.Player {
		return procMage(FireTalents, "p3_fire_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFire)
	}
	wantTorment := func(when string, mage *Mage, target *core.Unit, want float64) {
		t.Helper()
		if got := mage.tormentTheWeak(target); got < want-1e-9 || got > want+1e-9 {
			t.Errorf("%s: x%v, want x%v", when, got, want)
		}
	}

	// a boss with no immunity row takes snares, an add whose row has snares doesn't
	boss := googleProto.Clone(core.NewDefaultTarget()).(*proto.Target)
	boss.MechanicImmuneMask = googleProto.Uint64(0)
	add := &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid, MechanicImmuneMask: googleProto.Uint64(1 << core.MechanicSnare)}
	sim, mage := newProcSim(t, newMage(), boss, add)
	bossUnit, addUnit := sim.GetTargetUnit(0), sim.GetTargetUnit(1)
	if !bossUnit.IsWorldBoss || addUnit.IsWorldBoss {
		t.Fatalf("world boss: boss %v, add %v", bossUnit.IsWorldBoss, addUnit.IsWorldBoss)
	}
	mage.chill(sim, bossUnit, 9*time.Second)
	mage.chill(sim, addUnit, 9*time.Second)
	wantTorment("a boss that takes snares, chilled", mage, bossUnit, 1.12)
	wantTorment("a snare-immune add, chilled", mage, addUnit, 1)

	// two mages on an add
	encounter := core.MakeSingleTargetEncounter(0)
	encounter.Targets = []*proto.Target{{Level: 80, MobType: proto.MobType_MobTypeHumanoid}}
	sim = core.NewSim(&proto.RaidSimRequest{
		Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{newMage(), newMage()}, Buffs: &proto.PartyBuffs{}}},
			Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, IsTest: true, RandomSeed: 101},
	})
	sim.Reset()
	first, second := sim.Raid.Parties[0].Players[0].(*Mage), sim.Raid.Parties[0].Players[1].(*Mage)
	target := sim.GetTargetUnit(0)
	wantTorment("an add nobody snared", first, target, 1)
	second.chill(sim, target, 9*time.Second)
	wantTorment("an add another mage chilled", first, target, 1.12)
	second.chills.Get(target).Deactivate(sim)
	wantTorment("the add with the chill gone", first, target, 1)
}

// Brain Freeze (spell_proc -44546) rolls on a landed Frostbolt or Blizzard tick, never on a miss, and needs no
// Improved Blizzard.
func TestBrainFreezeRollsOnLandedHits(t *testing.T) {
	player := procMage(FrostTalents, "p3_frost_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFrost)
	// no gear, so about one hit in six misses
	player.Equipment = &proto.EquipmentSpec{}
	sim, mage := newProcSim(t, player)
	if mage.Talents.BrainFreeze != 3 || mage.Talents.ImprovedBlizzard != 0 {
		t.Fatalf("FrostTalents: Brain Freeze %d, Improved Blizzard %d", mage.Talents.BrainFreeze, mage.Talents.ImprovedBlizzard)
	}
	target := sim.GetTargetUnit(0)
	tick := mage.GetSpell(core.ActionID{SpellID: 42938})
	metrics := &tick.SpellMetrics[target.UnitIndex]
	landed, missed, procs := 0, 0, 0
	for i := 0; i < 2000; i++ {
		misses := metrics.Misses
		tick.Cast(sim, target)
		proc := mage.BrainFreezeAura.IsActive()
		mage.BrainFreezeAura.Deactivate(sim)
		if metrics.Misses > misses {
			missed++
			if proc {
				t.Fatalf("tick %d missed and gave Brain Freeze", i)
			}
			continue
		}
		landed++
		if proc {
			procs++
		}
	}
	if missed < 200 {
		t.Fatalf("%d of 2000 ticks missed, want about 340", missed)
	}
	if rate := float64(procs) / float64(landed); rate < 0.12 || rate > 0.18 {
		t.Errorf("%d procs in %d landed ticks (%.3f), want about 0.15", procs, landed, rate)
	}

	mage.Blizzard.Cast(sim, target)
	if mage.BrainFreezeAura.IsActive() {
		t.Errorf("Blizzard's own cast gave Brain Freeze")
	}

	procs = 0
	for seed := int64(1); seed <= 3; seed++ {
		entries := runProcLog(t, procMage(FrostTalents, "p3_frost_alliance", castApl("", "42842"), PlayerOptionsFrost), 180, seed)
		procs += len(logTimes(entries, gainedOrRefreshed("44549")...))
	}
	if procs == 0 {
		t.Errorf("no Brain Freeze in 540 s of Frostbolts")
	}
}

// A Mirror Image's Frostbolt, Blast Wave and Dragon's Breath snare an add too, so they count for Torment the
// Weak. Only the image's Frostbolt is in Permafrost's class mask, and Dragon's Breath's aura has one proc
// charge, so the next direct hit that damages the add takes its snare off.
func TestTormentTheWeakCountsImagesAndFireSnares(t *testing.T) {
	add := &proto.Target{Level: 80, MobType: proto.MobType_MobTypeHumanoid}
	newSim := func(targets ...*proto.Target) (*core.Simulation, *Mage) {
		sim, mage := newProcSim(t, procMage(FireTalents, "p3_fire_alliance", `{"type": "TypeAPL", "priorityList": []}`, PlayerOptionsFire),
			targets...)
		if mage.BlastWave == nil || mage.DragonsBreath == nil {
			t.Fatalf("FireTalents lacks Blast Wave or Dragon's Breath")
		}
		// read when a snare goes on
		mage.Talents.Permafrost = 2
		return sim, mage
	}

	sim, mage := newSim(core.NewDefaultTarget(), add)
	bossUnit, addUnit := sim.GetTargetUnit(0), sim.GetTargetUnit(1)
	// castUntil casts the spell, off its cooldown, until done says so
	castUntil := func(spell *core.Spell, target *core.Unit, done func() bool) {
		t.Helper()
		for i := 0; !done(); i++ {
			if i == 10 {
				t.Fatalf("10 casts of %s and none landed on the add", spell.ActionID)
			}
			if spell.CD.Timer != nil {
				spell.CD.Reset()
			}
			mage.GCD.Reset()
			spell.Cast(sim, target)
		}
	}
	wantTorment := func(when string, target *core.Unit, want float64) {
		t.Helper()
		if got := mage.tormentTheWeak(target); got < want-1e-9 || got > want+1e-9 {
			t.Errorf("%s: x%v, want x%v", when, got, want)
		}
	}

	chilled := mage.chills.Get(addUnit)
	castUntil(mage.BlastWave, bossUnit, chilled.IsActive)
	if chilled.Duration != 6*time.Second {
		t.Errorf("Blast Wave snares the add for %v, want 6 s", chilled.Duration)
	}
	wantTorment("the boss after Blast Wave", bossUnit, 1)
	wantTorment("the add after Blast Wave", addUnit, 1.12)
	chilled.Deactivate(sim)

	breath := mage.dragonsBreathSnares.Get(addUnit)
	castUntil(mage.DragonsBreath, bossUnit, breath.IsActive)
	if breath.Duration != 5*time.Second || chilled.IsActive() {
		t.Errorf("Dragon's Breath snares the add for %v (chilled %v), want 5 s of its own aura", breath.Duration, chilled.IsActive())
	}
	wantTorment("the boss after Dragon's Breath", bossUnit, 1)
	wantTorment("the add after Dragon's Breath", addUnit, 1.12)
	explosion := mage.GetSpell(core.ActionID{SpellID: livingBombExplosionID})
	if explosion == nil {
		t.Fatalf("FireTalents lacks Living Bomb")
	}
	damage := func() float64 { return explosion.SpellMetrics[addUnit.UnitIndex].TotalDamage }
	castUntil(explosion, addUnit, func() bool { return damage() > 0 })
	if !breath.IsActive() {
		t.Errorf("Living Bomb's explosion took Dragon's Breath's snare off")
	}
	castUntil(mage.FireBlast, addUnit, func() bool { return !breath.IsActive() })
	if hits := mage.FireBlast.SpellMetrics[addUnit.UnitIndex]; hits.Hits+hits.Crits != 1 {
		t.Errorf("the snare came off after %d Fire Blasts landed, want the first", hits.Hits+hits.Crits)
	}
	wantTorment("the add after a Fire Blast", addUnit, 1)

	sim, mage = newSim(add)
	addUnit = sim.GetTargetUnit(0)
	mage.MirrorImage.Cast(sim, addUnit)
	metrics := &mage.mirrorImage.Frostbolt.SpellMetrics[addUnit.UnitIndex]
	// a hit's damage counts when it lands
	for metrics.TotalDamage == 0 && !sim.Step() {
		if metrics.TotalDamage == 0 && mage.chills.Get(addUnit).IsActive() {
			t.Fatalf("add snared before an image's Frostbolt landed")
		}
	}
	if metrics.TotalDamage == 0 {
		t.Fatalf("no image Frostbolt landed")
	}
	if aura := mage.chills.Get(addUnit); !aura.IsActive() || aura.Duration != 6*time.Second {
		t.Errorf("after an image's Frostbolt: add snared %v for %v, want 4 s plus Permafrost's 2", aura.IsActive(), aura.Duration)
	}
	if got := mage.tormentTheWeak(addUnit); got < 1.12-1e-9 || got > 1.12+1e-9 {
		t.Errorf("after an image's Frostbolt: x%v, want x1.12", got)
	}
}
