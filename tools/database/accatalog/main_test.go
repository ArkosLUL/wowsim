package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// Each change lands in its list; a pvp flip shows even when the tier moved too.
func TestPrintDiff(t *testing.T) {
	source := &proto.CatalogSource{Kind: proto.CatalogSourceKind_CatalogSourceVendor}
	gear := func(tier int32, pvp bool, sources int) *proto.CatalogItem {
		item := &proto.CatalogItem{Id: 1, Name: "Gear", ProgressionTier: tier, Pvp: pvp}
		for range sources {
			item.Sources = append(item.Sources, source)
		}
		return item
	}
	tests := []struct {
		name          string
		before, after *proto.CatalogItem
		want          map[string]int
	}{
		{"unchanged", gear(13, false, 1), gear(13, false, 1), nil},
		{"added", nil, gear(13, false, 1), map[string]int{"added": 1}},
		{"removed", gear(13, false, 1), nil, map[string]int{"removed": 1}},
		{"tier", gear(13, false, 1), gear(15, false, 1), map[string]int{"tier changed": 1}},
		{"pvp", gear(13, false, 1), gear(13, true, 1), map[string]int{"pvp flag changed": 1}},
		{"tier and pvp", gear(13, false, 1), gear(15, true, 1), map[string]int{"tier changed": 1, "pvp flag changed": 1}},
		{"pvp and sources", gear(13, false, 1), gear(13, true, 2), map[string]int{"pvp flag changed": 1}},
		{"sources", gear(13, false, 1), gear(13, false, 2), map[string]int{"source count changed": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous, catalog := &proto.ServerCatalog{}, &proto.ServerCatalog{}
			if tt.before != nil {
				previous.Items = append(previous.Items, tt.before)
			}
			if tt.after != nil {
				catalog.Items = append(catalog.Items, tt.after)
			}
			var out bytes.Buffer
			printDiff(&out, previous, catalog)
			for _, list := range []string{"added", "removed", "tier changed", "pvp flag changed", "source count changed"} {
				if want := fmt.Sprintf("%s: %d\n", list, tt.want[list]); !strings.Contains(out.String(), want) {
					t.Errorf("want %q in:\n%s", want, out.String())
				}
			}
		})
	}
}

func TestCatalogDate(t *testing.T) {
	today := time.Now().Format(time.DateOnly)
	item := func(tier int32) []*proto.CatalogItem {
		return []*proto.CatalogItem{{Id: 1, Name: "Gear", ProgressionTier: tier}}
	}
	previous := &proto.ServerCatalog{Date: "2026-01-01", Items: item(14)}

	if got := catalogDate(&proto.ServerCatalog{Items: item(14)}, previous); got != "2026-01-01" {
		t.Errorf("unchanged catalog got date %s, want the old one", got)
	}
	if got := catalogDate(&proto.ServerCatalog{Items: item(15)}, previous); got != today {
		t.Errorf("changed catalog got date %s, want today", got)
	}
	if got := catalogDate(&proto.ServerCatalog{Items: item(14)}, nil); got != today {
		t.Errorf("first catalog got date %s, want today", got)
	}

	*date = "2026-02-02"
	defer func() { *date = "" }()
	if got := catalogDate(&proto.ServerCatalog{Items: item(14)}, previous); got != "2026-02-02" {
		t.Errorf("-date got ignored: %s", got)
	}
}
