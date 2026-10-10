package optimizer

import (
	"fmt"
	"math"
	"slices"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	"github.com/wowsims/wotlk/sim/rogue"
	"github.com/wowsims/wotlk/sim/warrior"
)

// weaponTalent is a talent the sim keys on both hands' weapon types at once
// (Character.GetProcMaskForTypes), so two covered hands can be worth more or less than each covered
// hand next to an uncovered one.
type weaponTalent struct {
	name  string
	types []proto.WeaponType
}

func (t *weaponTalent) covers(item *core.Item) bool {
	return item.ID != 0 && slices.Contains(t.types, item.WeaponType)
}

// weaponTalentsOf lists the target's weapon talents it has points in.
func weaponTalentsOf(target *proto.Player) (out []weaponTalent, err error) {
	// FillTalentsProto panics on a string that doesn't fit the class's trees
	defer func() {
		if e := recover(); e != nil {
			out, err = nil, fmt.Errorf("target's talents %q: %v", target.GetTalentsString(), e)
		}
	}()
	add := func(rank int32, name string, types ...proto.WeaponType) {
		if rank > 0 {
			out = append(out, weaponTalent{name, types})
		}
	}
	switch target.GetClass() {
	case proto.Class_ClassRogue:
		talents := &proto.RogueTalents{}
		core.FillTalentsProto(talents.ProtoReflect(), target.GetTalentsString(), rogue.TalentTreeSizes)
		add(talents.CloseQuartersCombat, "Close Quarters Combat", proto.WeaponType_WeaponTypeDagger, proto.WeaponType_WeaponTypeFist)
		add(talents.HackAndSlash, "Hack and Slash", proto.WeaponType_WeaponTypeSword, proto.WeaponType_WeaponTypeAxe)
		add(talents.MaceSpecialization, "Mace Specialization", proto.WeaponType_WeaponTypeMace)
	case proto.Class_ClassWarrior:
		talents := &proto.WarriorTalents{}
		core.FillTalentsProto(talents.ProtoReflect(), target.GetTalentsString(), warrior.TalentTreeSizes)
		add(talents.SwordSpecialization, "Sword Specialization", proto.WeaponType_WeaponTypeSword)
		add(talents.PoleaxeSpecialization, "Poleaxe Specialization", proto.WeaponType_WeaponTypeAxe, proto.WeaponType_WeaponTypePolearm)
		add(talents.MaceSpecialization, "Mace Specialization", proto.WeaponType_WeaponTypeMace)
	}
	return out, nil
}

// weaponTerm is a weapon talent priced from both hands together: what weapons it covers in both
// hands add beyond what each covered hand adds next to an uncovered one. Each hand's own share stays
// in that weapon's residual.
type weaponTerm struct {
	talent weaponTalent
	both   Estimate
}

// weaponPair is what the weapon talents' terms add with mh and oh in the hands.
func (s *surrogate) weaponPair(mh, oh *core.Item) float64 {
	v := 0.0
	for i := range s.weaponTerms {
		t := &s.weaponTerms[i]
		if t.talent.covers(mh) && t.talent.covers(oh) {
			v += t.both.Mean
		}
	}
	return v
}

func (s *surrogate) weaponPairOf(mhID, ohID int32) float64 {
	return s.weaponPair(s.handItem(proto.ItemSlot_ItemSlotMainHand, mhID), s.handItem(proto.ItemSlot_ItemSlotOffHand, ohID))
}

// weaponDelta is the weapon talents' terms with l's hands, against the seed's.
func (s *surrogate) weaponDelta(l *Loadout) float64 {
	if len(s.weaponTerms) == 0 {
		return 0
	}
	return s.weaponPairOf(l.Items[proto.ItemSlot_ItemSlotMainHand].ItemID, l.Items[proto.ItemSlot_ItemSlotOffHand].ItemID) - s.seedWeapon
}

// weaponBounds bounds what the weapon talents' terms move when item goes into slot's empty hand,
// whatever the other hand holds.
func (s *surrogate) weaponBounds(slot proto.ItemSlot, item *core.Item) (lb, ub float64) {
	if !isHand(slot) || slot == proto.ItemSlot_ItemSlotMainHand && s.pool.fillsHands(item) {
		return 0, 0
	}
	for i := range s.weaponTerms {
		if t := &s.weaponTerms[i]; t.talent.covers(item) {
			lb, ub = lb+min(0, t.both.Mean), ub+max(0, t.both.Mean)
		}
	}
	return lb, ub
}

// weaponShare is how far the weapon talents' terms moved from k's base to its variant. The terms
// price that themselves, so k's residual leaves it out.
func (s *surrogate) weaponShare(fx *effects, k effectKey) float64 {
	f, variant := fx.family[k], fx.variant[k]
	if len(s.weaponTerms) == 0 || f == nil {
		return 0
	}
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	return s.weaponPairOf(variant.Items[mh].ItemID, variant.Items[oh].ItemID) - s.weaponPairOf(f.base.Items[mh].ItemID, f.base.Items[oh].ItemID)
}

// weaponAfter is st's weapon talent term with changes made.
func (se *searcher) weaponAfter(st *state, changes []change) float64 {
	if len(se.s.weaponTerms) == 0 {
		return st.weapon
	}
	mh, oh := st.l.Items[proto.ItemSlot_ItemSlotMainHand].ItemID, st.l.Items[proto.ItemSlot_ItemSlotOffHand].ItemID
	moved := false
	for _, ch := range changes {
		switch ch.slot {
		case proto.ItemSlot_ItemSlotMainHand:
			mh, moved = ch.choice.ItemID, true
		case proto.ItemSlot_ItemSlotOffHand:
			oh, moved = ch.choice.ItemID, true
		}
	}
	if !moved {
		return st.weapon
	}
	return se.s.weaponPairOf(mh, oh) - se.s.seedWeapon
}

// talentProbe is the four sims that price one weapon talent: the seed with a covered weapon in
// each hand, each one swapped or not for its twin under a type nothing covers. A twin only differs in
// its type, so whatever its hand alone gains from that cancels, and only what the hands do together
// is left.
type talentProbe struct {
	talent weaponTalent
	// by main hand covered, then off hand covered
	points [2][2]Point
}

// twinTypes are the weapon types a twin can take, in order of preference.
var twinTypes = []proto.WeaponType{proto.WeaponType_WeaponTypeMace, proto.WeaponType_WeaponTypeSword,
	proto.WeaponType_WeaponTypeAxe, proto.WeaponType_WeaponTypeFist, proto.WeaponType_WeaponTypeDagger}

// talentProbes lists a probe per weapon talent of the target's that both hands can hold a weapon for.
func (r *run) talentProbes(s *surrogate, v stats.Stats, gp *gemPlan) []*talentProbe {
	p, seed := s.pool, s.seed
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	// with a hand locked, the talent only moves with the other one, and that hand's residuals carry it
	if p.Locked[mh] || p.Locked[oh] {
		return nil
	}
	talents, err := weaponTalentsOf(r.r.Target())
	if err != nil {
		r.warn("couldn't read the weapon talents: %v", err)
		return nil
	}
	if len(talents) == 0 {
		return nil
	}
	// a twin's type is one no talent covers, and no racial weapon specialization either, which also
	// reads both hands
	taken := map[proto.WeaponType]bool{}
	for _, t := range talents {
		for _, wt := range t.types {
			taken[wt] = true
		}
	}
	if spec, ok := core.RacialWeaponSpecialization(seed.RacialTraits); ok {
		for _, wt := range spec.WeaponTypes {
			taken[wt] = true
		}
	}
	i := slices.IndexFunc(twinTypes, func(wt proto.WeaponType) bool { return !taken[wt] })
	if i < 0 {
		r.trace("weapon talents: every weapon type is covered, so there's no twin to price them with")
		return nil
	}
	twinType := twinTypes[i]

	// the seed's own choice when it fits, else the fitting weapon with the most damage per second,
	// one without an effect first, configured as its residual sim has it
	pick := func(slot proto.ItemSlot, fits func(*core.Item) bool) (ItemChoice, *core.Item, bool) {
		if c := seed.Items[slot]; c.ItemID != 0 {
			if item, ok := p.item(slot, c.ItemID); ok && fits(item) {
				return c, item, true
			}
		}
		var best *Candidate
		better := func(cand *Candidate) bool {
			if effect, bestEffect := ItemHasEffect(cand.Item.ID), ItemHasEffect(best.Item.ID); effect != bestEffect {
				return !effect
			}
			return weaponDPS(&cand.Item) > weaponDPS(&best.Item)
		}
		for _, cand := range p.Slots[slot] {
			if fits(&cand.Item) && (best == nil || better(cand)) {
				best = cand
			}
		}
		if best == nil {
			return ItemChoice{}, nil, false
		}
		return s.quickChoice(slot, best, v, gp, -1), &best.Item, true
	}
	weapon := func(item *core.Item) bool { return isWeapon(*item, false) }
	mainHand := func(item *core.Item) bool { return weapon(item) && !p.fillsHands(item) }

	var out []*talentProbe
	for _, t := range talents {
		mhCovered, mhItem, ok1 := pick(mh, allOf(mainHand, t.covers))
		ohCovered, ohItem, ok2 := pick(oh, allOf(weapon, t.covers))
		if !ok1 || !ok2 {
			r.trace("weapon talent %s: no weapon it covers for each hand", t.name)
			continue
		}
		mhTwin, ohTwin := mhCovered, ohCovered
		var ok3, ok4 bool
		mhTwin.ItemID, ok3 = typeTwin(mhItem, twinType)
		ohTwin.ItemID, ok4 = typeTwin(ohItem, twinType)
		if !ok3 || !ok4 {
			continue
		}
		probe := &talentProbe{talent: t}
		for a, mhChoice := range [2]ItemChoice{mhTwin, mhCovered} {
			for b, ohChoice := range [2]ItemChoice{ohTwin, ohCovered} {
				l := seed
				l.Items[mh], l.Items[oh] = mhChoice, ohChoice
				probe.points[a][b] = Point{Loadout: l}
			}
		}
		out = append(out, probe)
	}
	return out
}

// Twins' ids: no real item reaches twinStride, so a twin's id can't be one.
const (
	twinBase   = 1_000_000_000
	twinStride = 10_000_000
)

// typeTwin is the id of item's copy under weapon type wt, which it adds to core's database the first
// time. Only the talent probes sim it; no pool offers it.
func typeTwin(item *core.Item, wt proto.WeaponType) (int32, bool) {
	if item.ID <= 0 || item.ID >= twinStride {
		return 0, false
	}
	id := twinBase + int32(wt)*twinStride + item.ID
	if _, ok := core.LookupItem(id); ok {
		return id, true
	}
	twin := &proto.SimItem{Id: id, Name: item.Name, Type: item.Type, ArmorType: item.ArmorType, WeaponType: wt,
		HandType: item.HandType, RangedWeaponType: item.RangedWeaponType, Stats: item.Stats.ToFloatArray(),
		GemSockets: item.GemSockets, SocketBonus: item.SocketBonus.ToFloatArray(), WeaponDamageMin: item.WeaponDamageMin,
		WeaponDamageMax: item.WeaponDamageMax, WeaponSpeed: item.SwingSpeed, SetName: item.SetName}
	for _, st := range item.ServerStats {
		twin.ServerStats = append(twin.ServerStats, &proto.ItemStat{StatType: st.Type, Value: st.Value})
	}
	core.AddToDatabase(&proto.SimDatabase{Items: []*proto.SimItem{twin}})
	return id, true
}

func allOf(a, b func(*core.Item) bool) func(*core.Item) bool {
	return func(item *core.Item) bool { return a(item) && b(item) }
}

// talentProbePoints is the probes' points no residual family sims, which can run alongside the families.
// The shared ones wait for their family's screening sims, and priceWeaponTalents extends them.
func talentProbePoints(probes []*talentProbe, families []*effectFamily, seed Loadout) []Point {
	simmed := map[Point]bool{{Loadout: seed}: true}
	for _, f := range families {
		if f.matched {
			continue
		}
		simmed[Point{Loadout: f.base}] = true
		for _, v := range f.variants {
			simmed[Point{Loadout: v}] = true
		}
	}
	var out []Point
	for _, pr := range probes {
		for _, row := range pr.points {
			for _, pt := range row {
				if !simmed[pt] {
					simmed[pt] = true
					out = append(out, pt)
				}
			}
		}
	}
	return out
}

// priceWeaponTalents sims the probes at full iterations and keeps each talent's term where it clears
// 2 standard errors: the paired change from the off hand's twin to the covered weapon next to the
// covered main hand, less the same change next to the main hand's twin.
func (r *run) priceWeaponTalents(s *surrogate, probes []*talentProbe) error {
	s.weaponTerms = nil
	if len(probes) > 0 {
		var points []Point
		for _, pr := range probes {
			for _, row := range pr.points {
				points = append(points, row[:]...)
			}
		}
		evals, err := r.evaluate(points, r.budget.Iterations)
		if err != nil {
			return fmt.Errorf("weapon talent sims: %w", err)
		}
		for i, pr := range probes {
			uu, uc, cu, cc := evals[4*i], evals[4*i+1], evals[4*i+2], evals[4*i+3]
			if uu == nil || uc == nil || cu == nil || cc == nil {
				r.warn("left %s out of the weapon pricing: a sim of it failed", pr.talent.name)
				continue
			}
			withCovered, withUncovered := r.obj.Delta(cu, cc), r.obj.Delta(uu, uc)
			p := pr.points
			term := weaponTerm{pr.talent, Estimate{Mean: withCovered.Mean - withUncovered.Mean, SE: math.Hypot(withCovered.SE, withUncovered.SE)}}
			kept := abs(term.both.Mean) > 2*term.both.SE
			r.trace("weapon talent %s: both hands covered %+.1f ± %.1f beyond each alone (main hand %d, off hand %d, twins %d and %d), kept %v",
				pr.talent.name, term.both.Mean, term.both.SE, p[1][1].Loadout.Items[proto.ItemSlot_ItemSlotMainHand].ItemID,
				p[1][1].Loadout.Items[proto.ItemSlot_ItemSlotOffHand].ItemID, p[0][0].Loadout.Items[proto.ItemSlot_ItemSlotMainHand].ItemID,
				p[0][0].Loadout.Items[proto.ItemSlot_ItemSlotOffHand].ItemID, kept)
			if kept {
				s.weaponTerms = append(s.weaponTerms, term)
			}
		}
	}
	mh, oh := proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand
	s.seedWeapon = s.weaponPairOf(s.seed.Items[mh].ItemID, s.seed.Items[oh].ItemID)
	return nil
}
