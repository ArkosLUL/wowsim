package azerothcore

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

func TestMarshalCatalogJSON(t *testing.T) {
	catalog := &proto.ServerCatalog{
		Date: "2026-09-19",
		Items: []*proto.CatalogItem{
			{Id: 50363, Name: "Deathbringer's Will", ProgressionTier: 16, MaxCount: 1, LimitCategory: 47,
				Sources: []*proto.CatalogSource{{Kind: proto.CatalogSourceKind_CatalogSourceRaid25Heroic, ProgressionTier: 16, NpcId: 37813}}},
			{Id: 40111, Name: "Bold Cardinal Ruby", IsGem: true, ProgressionTier: 13},
		},
		LimitGroups: []*proto.LimitGroup{{Id: 47, Name: "Deathbringer's Will", MaxEquipped: 1}, {Id: 2, Name: "Jeweler's Gems", MaxEquipped: 3}},
	}
	data, err := MarshalCatalogJSON(catalog)
	if err != nil {
		t.Fatal(err)
	}
	again, err := MarshalCatalogJSON(catalog)
	if err != nil || !bytes.Equal(data, again) {
		t.Fatalf("second marshal differs: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	want := []string{
		`{`,
		`"date":"2026-09-19",`,
		`"items":[`,
		`{"id":40111,"name":"Bold Cardinal Ruby","isGem":true,"progressionTier":13},`,
		`{"id":50363,"name":"Deathbringer's Will","progressionTier":16,"sources":[{"kind":6,"progressionTier":16,"npcId":37813}],"maxCount":1,"limitCategory":47}`,
		`],`,
		`"limitGroups":[`,
		`{"id":2,"name":"Jeweler's Gems","maxEquipped":3},`,
		`{"id":47,"name":"Deathbringer's Will","maxEquipped":1}`,
		`]`,
		`}`,
		``,
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines:\n%s", len(lines), data)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %s\nwant %s", i, lines[i], want[i])
		}
	}

	parsed, err := ParseCatalogJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Date != catalog.Date || len(parsed.Items) != 2 || !googleProto.Equal(parsed.Items[1], catalog.Items[0]) {
		t.Errorf("round trip = %v", parsed)
	}
	if catalog.Items[0].Id != 50363 {
		t.Error("marshalling reordered the caller's items")
	}
}
