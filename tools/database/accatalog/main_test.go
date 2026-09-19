package main

import (
	"testing"
	"time"

	"github.com/wowsims/wotlk/sim/core/proto"
)

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
