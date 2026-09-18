package main

import (
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/tools/database"
	"github.com/wowsims/wotlk/tools/database/azerothcore"
)

func TestApplyServerData(t *testing.T) {
	const (
		obtainable = iota + 1
		unobtainable
		randomEnchant
		missing
		notAGem
		gem
		gemMissing
	)
	db := database.NewWowDatabase()
	for _, id := range []int32{obtainable, unobtainable, randomEnchant, missing} {
		db.Items[id] = &proto.UIItem{Id: id, Ilvl: 232}
	}
	for _, id := range []int32{notAGem, gem, gemMissing} {
		db.Gems[id] = &proto.UIGem{Id: id}
	}
	server := &serverData{
		dbc: &azerothcore.DBC{GemProperties: map[int32]*azerothcore.GemPropertiesEntry{1: {ID: 1, Color: 2}}},
		items: map[int32]*azerothcore.ItemRow{
			obtainable:    {Entry: obtainable, ItemLevel: 226},
			unobtainable:  {Entry: unobtainable, ItemLevel: 226},
			randomEnchant: {Entry: randomEnchant, ItemLevel: 226, RandomSuffix: 5},
			notAGem:       {Entry: notAGem},
			gem:           {Entry: gem, GemProperties: 1},
		},
		obtainable: map[int32][]string{obtainable: {"vendor"}, randomEnchant: {"vendor"}},
	}

	applyServerData(db, server)

	for id, want := range map[int32]int32{obtainable: 226, unobtainable: 232, randomEnchant: 232, missing: 232} {
		if got := db.Items[id].Ilvl; got != want {
			t.Errorf("item %d: ilvl %d, want %d", id, got, want)
		}
	}
	if _, ok := db.Gems[notAGem]; ok {
		t.Error("kept a gem the server has as a non-gem item")
	}
	for _, id := range []int32{gem, gemMissing} {
		if _, ok := db.Gems[id]; !ok {
			t.Errorf("dropped gem %d", id)
		}
	}
}
