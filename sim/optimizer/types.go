// Package optimizer finds a player's BiS: items, gems, enchants, reforges and racial traits.
package optimizer

import (
	"fmt"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
)

// NumSlots counts the equipment slots, head through ranged.
const NumSlots = int(proto.ItemSlot_ItemSlotRanged) + 1

// MaxGems is three sockets plus the extra one from a belt buckle or blacksmithing.
const MaxGems = 4

// ItemChoice is what one slot holds. It's comparable, so loadouts can key maps.
type ItemChoice struct {
	ItemID int32
	// SimEnchant effect id.
	Enchant int32
	Gems    [MaxGems]int32
	// Server ItemModType ids; both 0 means no reforge.
	ReforgeFrom int32
	ReforgeTo   int32
}

// Loadout is one candidate answer: a choice per slot, plus racial traits.
type Loadout struct {
	Items        [NumSlots]ItemChoice
	RacialTraits proto.Race
}

func ItemChoiceFromProto(spec *proto.ItemSpec) (ItemChoice, error) {
	var c ItemChoice
	if spec == nil {
		return c, nil
	}
	if len(spec.Gems) > MaxGems {
		return c, fmt.Errorf("item %d has %d gems, at most %d fit", spec.Id, len(spec.Gems), MaxGems)
	}
	c.ItemID = spec.Id
	c.Enchant = spec.Enchant
	copy(c.Gems[:], spec.Gems)
	c.ReforgeFrom = spec.GetReforge().GetFromStatType()
	c.ReforgeTo = spec.GetReforge().GetToStatType()
	return c, nil
}

// ToProto drops trailing empty gems, which don't change anything in the sim.
func (c ItemChoice) ToProto() *proto.ItemSpec {
	spec := &proto.ItemSpec{
		Id:      c.ItemID,
		Enchant: c.Enchant,
		Gems:    c.gems(),
	}
	if c.ReforgeFrom != 0 || c.ReforgeTo != 0 {
		spec.Reforge = &proto.ItemReforge{FromStatType: c.ReforgeFrom, ToStatType: c.ReforgeTo}
	}
	return spec
}

// CoreSpec is the ItemSpec core.NewItem takes.
func (c ItemChoice) CoreSpec() core.ItemSpec {
	spec := core.ItemSpec{
		ID:      c.ItemID,
		Enchant: c.Enchant,
		Gems:    c.gems(),
	}
	if c.ReforgeFrom != 0 || c.ReforgeTo != 0 {
		spec.Reforge = &proto.ItemReforge{FromStatType: c.ReforgeFrom, ToStatType: c.ReforgeTo}
	}
	return spec
}

func (c ItemChoice) gems() []int32 {
	n := MaxGems
	for n > 0 && c.Gems[n-1] == 0 {
		n--
	}
	if n == 0 {
		return nil
	}
	return append([]int32(nil), c.Gems[:n]...)
}

// LoadoutFromProto reads an EquipmentSpec by slot index. Missing trailing slots are empty.
func LoadoutFromProto(es *proto.EquipmentSpec, racialTraits proto.Race) (Loadout, error) {
	l := Loadout{RacialTraits: racialTraits}
	items := es.GetItems()
	if len(items) > NumSlots {
		return l, fmt.Errorf("equipment has %d items, there are only %d slots", len(items), NumSlots)
	}
	for slot, spec := range items {
		c, err := ItemChoiceFromProto(spec)
		if err != nil {
			return l, fmt.Errorf("slot %s: %w", proto.ItemSlot(slot), err)
		}
		l.Items[slot] = c
	}
	return l, nil
}

// Equipment always holds NumSlots items, with empty slots as empty specs, like the UI sends.
func (l Loadout) Equipment() *proto.EquipmentSpec {
	es := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, NumSlots)}
	for slot, c := range l.Items {
		es.Items[slot] = c.ToProto()
	}
	return es
}

// Metric indexes Metrics, in proto.OptimizerMetrics' field order.
type Metric int

const (
	MetricDPS Metric = iota
	MetricHPS
	MetricTPS
	MetricDTPS
	MetricTMI
	MetricPDeath
	NumMetrics
)

// Metrics holds one number per unit metric, like proto.OptimizerMetrics.
type Metrics [NumMetrics]float64

func MetricsFromProto(m *proto.OptimizerMetrics) Metrics {
	return Metrics{
		MetricDPS:    m.GetDps(),
		MetricHPS:    m.GetHps(),
		MetricTPS:    m.GetTps(),
		MetricDTPS:   m.GetDtps(),
		MetricTMI:    m.GetTmi(),
		MetricPDeath: m.GetPDeath(),
	}
}

func (m Metrics) ToProto() *proto.OptimizerMetrics {
	return &proto.OptimizerMetrics{
		Dps:    m[MetricDPS],
		Hps:    m[MetricHPS],
		Tps:    m[MetricTPS],
		Dtps:   m[MetricDTPS],
		Tmi:    m[MetricTMI],
		PDeath: m[MetricPDeath],
	}
}

// Estimate is a sim-measured mean with its standard error.
type Estimate struct {
	Mean float64
	SE   float64
}

// ProgressFunc gets progress updates, one call at a time, and never after Optimize returns (RunAsync
// closes its channel then). It can keep the message: Optimize never changes one after reporting it.
type ProgressFunc func(*proto.OptimizerProgress)

// Request is an OptimizeGearRequest that PrepareRequest checked and deep-copied. Every item, gem and
// enchant it names is in core's database, and no Player.database is left in it.
type Request struct {
	// Read-only: clone it before changing anything.
	Base        *proto.RaidSimRequest
	TargetIndex int
	// Defaults already applied.
	Settings *proto.OptimizerSettings
	Pool     *proto.CandidatePool

	// The target's gear and racial traits going in.
	Seed       Loadout
	WarmStarts []Loadout

	// The pool's catalog rows, limit groups and meta conditions, by id.
	Catalog        map[int32]*proto.CatalogItem
	LimitGroups    map[int32]*proto.LimitGroup
	MetaConditions map[int32]*proto.MetaGemCondition
}

// Target is the player being optimized, inside Base.
func (r *Request) Target() *proto.Player {
	return r.Base.Raid.Parties[r.TargetIndex/5].Players[r.TargetIndex%5]
}
