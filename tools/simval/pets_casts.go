package main

import (
	"fmt"
	"math"
	"slices"
	"time"

	simpkg "github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// TestPetfixFireElementalCasts and TestPetfixMirrorImageCasts log what the two scripted guardians cast and
// when, read off the packets a bot sees. The replay runs the sim's guardian the same way and holds each
// record to it.

const petsCastsFixture = "pets_casts.jsonl"

const (
	// a packet's arrival against the map update that sent it, either way
	petsCastsJitterMs = 250
	// the longest live map update: a cast or swing that's due goes on the next one
	petsCastsUpdateMs = 110
	// the live updates jitter around 102.7 ms and the sim's don't: a 3 s cast is 3081 ms in the sim and 3048 on
	// average live, so a chain of casts runs about 1% apart
	petsCastsDrift = 0.015
	// a cast this close to an image's death rides on the update period, so either side may lack it
	petsCastsEdgeMs = 500
	// lives of the sim's elemental the recorded ones are held to
	petsCastsSimLives = 600
)

const (
	fireElementalBlast = 57984
	fireElementalNova  = 12470
	mirrorImageSummon  = 55342
	mirrorImageBolt    = 59638
	mirrorImageBlast   = 59637
)

// petsCastsRecord is a pets_casts.jsonl line, in ms since the guardian came: one Greater Fire Elemental's 2
// min (a PETFIX_CASTS line) or one summon of Mirror Images (a PETFIX_IMAGES line).
type petsCastsRecord struct {
	NovaStart  []int64 `json:"novaStart"`
	NovaGo     []int64 `json:"novaGo"`
	NovaFailed []int64 `json:"novaFailed"`
	BlastGo    []int64 `json:"blastGo"`
	SwingAt    []int64 `json:"swingAt"`
	// each swing's wait since the swing, Fire Blast or landed Fire Nova before it
	SwingWait []int64 `json:"swingWait"`

	// the mage was fighting as the images came; else the pull came Pulled ms after them
	Fighting bool             `json:"fighting"`
	Pulled   int64            `json:"pulled"`
	Images   []petsCastsImage `json:"images"`
}

type petsCastsImage struct {
	Opener      string  `json:"opener"`
	FrostboltGo []int64 `json:"frostboltGo"`
	FireBlastGo []int64 `json:"fireBlastGo"`
}

const petsCastsBlastOpener = "Fire Blast"

func (r petsCastsRecord) isElemental() bool { return len(r.Images) == 0 }

// petsCastsGuardian is one of a player's guardians in a sim of its own, with nothing else going on.
type petsCastsGuardian struct {
	sim   *core.Simulation
	agent core.PetAgent
	pet   *core.Pet
}

func newPetsCastsGuardian(player *proto.Player, name string) (g *petsCastsGuardian, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("building the sim's %s: %v", name, r)
		}
	}()
	simpkg.RegisterAll()
	encounter := core.MakeSingleTargetEncounter(0)
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(player, &proto.PartyBuffs{}, &proto.RaidBuffs{}, &proto.Debuffs{}),
		Encounter:  encounter,
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 101},
	})
	owner := sim.Raid.Parties[0].Players[0].GetCharacter()
	for _, agent := range owner.PetAgents {
		if pet := agent.GetPet(); pet.Name == name {
			return &petsCastsGuardian{sim: sim, agent: agent, pet: pet}, nil
		}
	}
	return nil, fmt.Errorf("the sim's %s has no %s", player.Class, name)
}

// watch appends the ms since *came of every cast of the guardian's spell that goes off.
func (g *petsCastsGuardian) watch(spell *core.Spell, came *time.Duration, at *[]int64) {
	effects := spell.ApplyEffects
	spell.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		*at = append(*at, (sim.CurrentTime - *came).Milliseconds())
		effects(sim, target, spell)
	}
}

func (g *petsCastsGuardian) spell(id int32) (*core.Spell, error) {
	if spell := g.pet.GetSpell(core.ActionID{SpellID: id}); spell != nil {
		return spell, nil
	}
	return nil, fmt.Errorf("the sim's %s has no spell %d", g.pet.Name, id)
}

// petsCastsLife is what the sim's elemental did over one life, in ms since it came.
type petsCastsLife struct {
	novaGo, blastGo, swingAt []int64
}

// petsCastsElemental is the sim's Greater Fire Elemental over many lives, each fighting from its first update
// as the recorded ones did, with its Fire Nova's cast time and its swing's.
type petsCastsElemental struct {
	lives                   []petsCastsLife
	novaMs, swingMs, lifeMs int64
}

func simElementalLives(n int) (*petsCastsElemental, error) {
	g, err := newPetsCastsGuardian(&proto.Player{
		// a tauren, as recorded: a draenei's Heroic Presence would give the elemental hit
		Race:      proto.Race_RaceTauren,
		Class:     proto.Class_ClassShaman,
		Equipment: &proto.EquipmentSpec{},
		Consumes:  &proto.Consumes{},
		Spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{
			Options: &proto.ElementalShaman_Options{Totems: &proto.ShamanTotems{UseFireElemental: true}},
		}},
		Rotation: core.APLRotationFromJsonString(`{"type": "TypeAPL", "priorityList": []}`),
	}, "Greater Fire Elemental")
	if err != nil {
		return nil, err
	}
	blast, errBlast := g.spell(fireElementalBlast)
	nova, errNova := g.spell(fireElementalNova)
	if err := firstErr(errBlast, errNova); err != nil {
		return nil, err
	}
	var came time.Duration
	var life petsCastsLife
	g.watch(blast, &came, &life.blastGo)
	g.watch(nova, &came, &life.novaGo)
	g.watch(g.pet.AutoAttacks.MHAuto(), &came, &life.swingAt)

	const lasts = 2 * time.Minute
	out := &petsCastsElemental{swingMs: int64(g.pet.AutoAttacks.MH().SwingSpeed * 1000), lifeMs: lasts.Milliseconds()}
	for i := 0; i < n; i++ {
		g.sim.Reseed(int64(1000 + i))
		g.sim.Reset()
		g.sim.PrePull()
		life, came = petsCastsLife{}, g.sim.CurrentTime
		g.pet.EnableWithTimeout(g.sim, g.agent, lasts)
		if i == 0 {
			out.novaMs = nova.CastTime().Milliseconds()
		}
		for g.pet.IsEnabled() && g.sim.CurrentTime < came+lasts+time.Second && !g.sim.Step() {
		}
		if g.pet.IsEnabled() {
			return nil, fmt.Errorf("the sim's elemental is still out %v after it came", g.sim.CurrentTime-came)
		}
		out.lives = append(out.lives, life)
		g.sim.Cleanup()
	}
	return out, nil
}

func petsCastsGaps(at []int64) []int64 {
	gaps := make([]int64, 0, len(at))
	for i := 1; i < len(at); i++ {
		gaps = append(gaps, at[i]-at[i-1])
	}
	return gaps
}

// checkPetsCastsMean holds the recorded lives' mean count to the sim's, within maxZ standard errors of a mean
// over that many lives.
func checkPetsCastsMean(name string, recorded, sim []float64) Check {
	simMean, got := mean(sim), mean(recorded)
	stderr := stdDev(sim) / math.Sqrt(float64(len(recorded)))
	detail := fmt.Sprintf("sim %.2f a life, server %.2f over %d lives (z %+.2f)", simMean, got, len(recorded), (got-simMean)/stderr)
	if math.Abs(got-simMean) <= maxZ*stderr {
		return pass(name, detail)
	}
	return fail(name, detail)
}

// petsCastsSlack is how far a recorded time may sit from the sim's, either way: the packet jitter, a map
// update and the clocks' drift over that long.
func petsCastsSlack(ms int64) int64 {
	return petsCastsJitterMs + petsCastsUpdateMs + int64(float64(ms)*petsCastsDrift)
}

// checkPetsCastsSpan holds every recorded value to the span the sim's values cover, with petsCastsSlack at
// both ends.
func checkPetsCastsSpan(name string, recorded, sim []int64) Check {
	if len(recorded) == 0 || len(sim) == 0 {
		return fail(name, fmt.Sprintf("%d recorded, %d in the sim", len(recorded), len(sim)))
	}
	lo, hi := slices.Min(sim), slices.Max(sim)
	detail := fmt.Sprintf("sim %d to %d ms, server %d to %d ms over %d", lo, hi, slices.Min(recorded), slices.Max(recorded), len(recorded))
	for _, v := range recorded {
		if v < lo-petsCastsSlack(lo) || v > hi+petsCastsSlack(hi) {
			return fail(name, fmt.Sprintf("%s: %d ms is outside", detail, v))
		}
	}
	return pass(name, detail)
}

// checkPetsCastsElemental holds the recorded lives to the sim's elemental: how many Fire Novas land, Fire
// Blasts go off and swings land a life, when the first Blast comes and how far apart Blasts and landed Novas
// are, each landed Nova its cast time after its start, each broken one on a Blast, and each swing one attack
// time after the swing or cast before it.
func checkPetsCastsElemental(lives []petsCastsRecord, sim *petsCastsElemental) []Check {
	if len(lives) == 0 {
		return []Check{fail("lives", "no recorded life")}
	}
	counts := func(of func(petsCastsLife) []int64) (out []float64) {
		for _, l := range sim.lives {
			out = append(out, float64(len(of(l))))
		}
		return out
	}
	recCounts := func(of func(petsCastsRecord) []int64) (out []float64) {
		for _, l := range lives {
			out = append(out, float64(len(of(l))))
		}
		return out
	}
	checks := []Check{
		checkPetsCastsMean("Fire Novas landed", recCounts(func(r petsCastsRecord) []int64 { return r.NovaGo }),
			counts(func(l petsCastsLife) []int64 { return l.novaGo })),
		checkPetsCastsMean("Fire Blasts", recCounts(func(r petsCastsRecord) []int64 { return r.BlastGo }),
			counts(func(l petsCastsLife) []int64 { return l.blastGo })),
		checkPetsCastsMean("swings", recCounts(func(r petsCastsRecord) []int64 { return r.SwingAt }),
			counts(func(l petsCastsLife) []int64 { return l.swingAt })),
	}

	var simFirst, simBlastGaps, simNovaGaps, recFirst, recBlastGaps, recNovaGaps []int64
	for _, l := range sim.lives {
		if len(l.blastGo) > 0 {
			simFirst = append(simFirst, l.blastGo[0])
		}
		simBlastGaps = append(simBlastGaps, petsCastsGaps(l.blastGo)...)
		simNovaGaps = append(simNovaGaps, petsCastsGaps(l.novaGo)...)
	}
	for _, l := range lives {
		if len(l.BlastGo) > 0 {
			recFirst = append(recFirst, l.BlastGo[0])
		}
		recBlastGaps = append(recBlastGaps, petsCastsGaps(l.BlastGo)...)
		recNovaGaps = append(recNovaGaps, petsCastsGaps(l.NovaGo)...)
	}
	checks = append(checks,
		checkPetsCastsSpan("first Fire Blast", recFirst, simFirst),
		checkPetsCastsSpan("Fire Blast gaps", recBlastGaps, simBlastGaps),
		checkPetsCastsSpan("landed Fire Nova gaps", recNovaGaps, simNovaGaps),
	)

	near := func(at []int64, ms, early, late int64) bool {
		return slices.ContainsFunc(at, func(a int64) bool { return a >= ms-early && a <= ms+late })
	}
	landed, broken, swings := pass("Fire Nova cast time", fmt.Sprintf("every landed Nova %d ms after its start", sim.novaMs)),
		pass("broken Fire Novas", "each on a Fire Blast"),
		pass("swing waits", fmt.Sprintf("each %d ms after the swing or cast before it", sim.swingMs))
	for n, l := range lives {
		for _, ms := range l.NovaGo {
			// it lands on the first update its cast time is over by
			if !near(l.NovaStart, ms-sim.novaMs, petsCastsSlack(sim.novaMs), petsCastsJitterMs) {
				landed = fail(landed.Name, fmt.Sprintf("life %d: the Nova landing at %d ms has no start %d ms before it", n, ms, sim.novaMs))
			}
		}
		for _, ms := range l.NovaFailed {
			// the one it's casting as it goes fails too
			if ms < sim.lifeMs-petsCastsJitterMs-petsCastsUpdateMs && !near(l.BlastGo, ms, petsCastsJitterMs, petsCastsJitterMs) {
				broken = fail(broken.Name, fmt.Sprintf("life %d: the Nova broken at %d ms has no Fire Blast with it", n, ms))
			}
		}
		for i := 1; i < len(l.SwingWait); i++ {
			if wait := l.SwingWait[i]; wait < sim.swingMs-petsCastsJitterMs || wait > sim.swingMs+petsCastsSlack(sim.swingMs) {
				swings = fail(swings.Name, fmt.Sprintf("life %d: a swing %d ms after the last one or cast, the sim's takes %d", n, wait, sim.swingMs))
			}
		}
	}
	return append(checks, landed, broken, swings)
}

// petsCastsImageRun is what the sim's images cast over one summon, in ms since they came, and how long they
// lasted.
type petsCastsImageRun struct {
	boltGo, blastGo []int64
	lifeMs          int64
}

func (r petsCastsImageRun) blastOpener() bool {
	return len(r.blastGo) > 0 && (len(r.boltGo) == 0 || r.blastGo[0] < r.boltGo[0])
}

// simImageRuns summons the sim's Mirror Images the way a record's were: lead before the pull, or that long
// into the fight when it's negative. It returns one run for each opener the sim's images took, keyed by
// whether it was a Fire Blast.
func simImageRuns(lead time.Duration) (map[bool]petsCastsImageRun, error) {
	cast := fmt.Sprintf(`"castSpell": {"spellId": {"spellId": %d}}`, mirrorImageSummon)
	apl := fmt.Sprintf(`{"type": "TypeAPL", "priorityList": [{"action": {"condition": {"cmp": {"op": "OpGe",
		"lhs": {"currentTime": {}}, "rhs": {"const": {"val": "%dms"}}}}, %s}}]}`, (-lead).Milliseconds(), cast)
	if lead > 0 {
		apl = fmt.Sprintf(`{"type": "TypeAPL", "prepullActions": [{"action": {%s}, "doAtValue": {"const": {"val": "-%dms"}}}],
			"priorityList": []}`, cast, lead.Milliseconds())
	}
	g, err := newPetsCastsGuardian(&proto.Player{
		Race:      proto.Race_RaceHuman,
		Class:     proto.Class_ClassMage,
		Equipment: &proto.EquipmentSpec{},
		Consumes:  &proto.Consumes{},
		Spec:      &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{}}},
		Rotation:  core.APLRotationFromJsonString(apl),
	}, "Mirror Image")
	if err != nil {
		return nil, err
	}
	bolt, errBolt := g.spell(mirrorImageBolt)
	blast, errBlast := g.spell(mirrorImageBlast)
	if err := firstErr(errBolt, errBlast); err != nil {
		return nil, err
	}
	var came time.Duration
	var run petsCastsImageRun
	g.watch(bolt, &came, &run.boltGo)
	g.watch(blast, &came, &run.blastGo)

	runs := map[bool]petsCastsImageRun{}
	// one opener for a summon into a fight, two ahead of it
	want := 2
	if lead <= 0 {
		want = 1
	}
	for seed := int64(1000); len(runs) < want && seed < 1100; seed++ {
		g.sim.Reseed(seed)
		g.sim.Reset()
		g.sim.PrePull()
		run = petsCastsImageRun{}
		for !g.pet.IsEnabled() && g.sim.CurrentTime < -lead+time.Second && !g.sim.Step() {
		}
		if !g.pet.IsEnabled() {
			return nil, fmt.Errorf("no images out in the sim at %v, summoned for %v", g.sim.CurrentTime, -lead)
		}
		// their first cast is over a second off, so the clock is set before any is logged
		came = g.sim.CurrentTime
		for g.pet.IsEnabled() && g.sim.CurrentTime < came+time.Minute && !g.sim.Step() {
		}
		run.lifeMs = (g.sim.CurrentTime - came).Milliseconds()
		runs[run.blastOpener()] = run
		g.sim.Cleanup()
	}
	if len(runs) < want {
		return nil, fmt.Errorf("the sim's images took %d of %d openers over 100 summons %v ahead of the pull", len(runs), want, lead)
	}
	return runs, nil
}

// checkPetsCastsTimes holds one image's recorded casts of a spell to the sim's, cast for cast. Both count
// from origin (the pull, or the summon into a fight). A cast within petsCastsEdgeMs of the image's death can
// be missing on either side.
func checkPetsCastsTimes(name string, recorded, sim []int64, recOrigin, simOrigin, lifeMs int64) Check {
	detail := fmt.Sprintf("sim %d casts, server %d", len(sim), len(recorded))
	both := min(len(recorded), len(sim))
	for i := 0; i < both; i++ {
		got, want := recorded[i]-recOrigin, sim[i]-simOrigin
		if slack := petsCastsSlack(want); got < want-slack || got > want+slack {
			return fail(name, fmt.Sprintf("%s: cast %d came %d ms in, the sim's %d", detail, i+1, got, want))
		}
	}
	for _, extra := range [][]int64{recorded[both:], sim[both:]} {
		for _, at := range extra {
			if at < lifeMs-petsCastsEdgeMs {
				return fail(name, fmt.Sprintf("%s: the cast %d ms in, %d ms before the image dies, is on one side only", detail, at, lifeMs-at))
			}
			detail += fmt.Sprintf(", one of them %d ms in, as the image dies", at)
		}
	}
	return pass(name, detail)
}

// checkPetsCastsSummon holds each image of a recorded summon to the sim's images out the same way with the
// same opener.
func checkPetsCastsSummon(rec petsCastsRecord, runs map[bool]petsCastsImageRun, lead time.Duration) []Check {
	if len(rec.Images) != 3 {
		return []Check{fail("images", fmt.Sprintf("%d images cast, a summon brings 3", len(rec.Images)))}
	}
	var checks []Check
	for n, img := range rec.Images {
		blastOpener := img.Opener == petsCastsBlastOpener
		run, ok := runs[blastOpener]
		if !ok {
			checks = append(checks, fail(fmt.Sprintf("image %d opener", n+1), fmt.Sprintf("opened on a %s, which the sim's never do here", img.Opener)))
			continue
		}
		recOrigin, simOrigin := int64(0), int64(0)
		if !rec.Fighting {
			recOrigin, simOrigin = rec.Pulled, lead.Milliseconds()
		}
		checks = append(checks,
			checkPetsCastsTimes(fmt.Sprintf("image %d Frostbolts", n+1), img.FrostboltGo, run.boltGo, recOrigin, simOrigin, run.lifeMs),
			checkPetsCastsTimes(fmt.Sprintf("image %d Fire Blasts", n+1), img.FireBlastGo, run.blastGo, recOrigin, simOrigin, run.lifeMs))
	}
	return checks
}

// petsCastsLead is how far ahead of the pull a recorded summon came, to the tenth of a second the test
// slept, negative for one into a fight.
func petsCastsLead(rec petsCastsRecord) time.Duration {
	return time.Duration(math.Round(float64(rec.Pulled)/100)) * 100 * time.Millisecond
}

// checkPetsCastsOpeners holds the Fire Blast openers among the images that came ahead of the pull to the even
// pick CasterAI makes for each.
func checkPetsCastsOpeners(records []petsCastsRecord) Check {
	images, blasts := 0.0, 0.0
	for _, rec := range records {
		if rec.isElemental() || rec.Fighting {
			continue
		}
		for _, img := range rec.Images {
			images++
			if img.Opener == petsCastsBlastOpener {
				blasts++
			}
		}
	}
	if images == 0 {
		return fail("Fire Blast openers", "no image came ahead of the pull")
	}
	return checkRate("Fire Blast openers", blasts, images, core.MaxRollBP/2)
}
