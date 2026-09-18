package optimizer

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	goproto "google.golang.org/protobuf/proto"
)

func TestLoadoutRoundTrip(t *testing.T) {
	es := &proto.EquipmentSpec{Items: []*proto.ItemSpec{
		{Id: 1, Enchant: 2, Gems: []int32{3, 0, 4, 0}, Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36}},
		{},
		{Id: 5, Gems: []int32{0}},
	}}
	l, err := LoadoutFromProto(es, proto.Race_RaceTroll)
	if err != nil {
		t.Fatal(err)
	}
	if l.RacialTraits != proto.Race_RaceTroll {
		t.Errorf("racial traits = %v, want troll", l.RacialTraits)
	}

	got := l.Equipment()
	if len(got.Items) != NumSlots {
		t.Fatalf("got %d items, want %d", len(got.Items), NumSlots)
	}
	want := []*proto.ItemSpec{
		{Id: 1, Enchant: 2, Gems: []int32{3, 0, 4}, Reforge: &proto.ItemReforge{FromStatType: 32, ToStatType: 36}},
		{},
		{Id: 5},
	}
	for slot, spec := range got.Items {
		wantSpec := &proto.ItemSpec{}
		if slot < len(want) {
			wantSpec = want[slot]
		}
		if !goproto.Equal(spec, wantSpec) {
			t.Errorf("slot %d = %v, want %v", slot, spec, wantSpec)
		}
	}

	again, err := LoadoutFromProto(got, l.RacialTraits)
	if err != nil {
		t.Fatal(err)
	}
	if again != l {
		t.Errorf("round trip changed the loadout: %+v, want %+v", again, l)
	}

	cs := l.Items[0].CoreSpec()
	if cs.ID != 1 || cs.Enchant != 2 || len(cs.Gems) != 3 || cs.Reforge.GetToStatType() != 36 {
		t.Errorf("CoreSpec = %+v", cs)
	}
	if l.Items[1].CoreSpec().Reforge != nil {
		t.Error("an unreforged slot got a reforge")
	}
}

func TestLoadoutFromProtoRejects(t *testing.T) {
	if _, err := LoadoutFromProto(&proto.EquipmentSpec{Items: []*proto.ItemSpec{{Id: 1, Gems: []int32{1, 2, 3, 4, 5}}}}, 0); err == nil {
		t.Error("5 gems on one item didn't fail")
	}
	if _, err := LoadoutFromProto(&proto.EquipmentSpec{Items: make([]*proto.ItemSpec, NumSlots+1)}, 0); err == nil {
		t.Error("an extra slot didn't fail")
	}
	if l, err := LoadoutFromProto(nil, 0); err != nil || l != (Loadout{}) {
		t.Errorf("nil equipment = %+v, %v; want an empty loadout", l, err)
	}
}

func TestMetricsRoundTrip(t *testing.T) {
	m := &proto.OptimizerMetrics{Dps: 1, Hps: 2, Tps: 3, Dtps: 4, Tmi: 5, PDeath: 6}
	got := MetricsFromProto(m)
	if got != (Metrics{1, 2, 3, 4, 5, 6}) {
		t.Errorf("MetricsFromProto = %v", got)
	}
	if !goproto.Equal(got.ToProto(), m) {
		t.Errorf("ToProto = %v, want %v", got.ToProto(), m)
	}
	if MetricsFromProto(nil) != (Metrics{}) {
		t.Error("nil metrics aren't zero")
	}
}
