package class

import "example.com/fixture/sim/core"

const FrostboltSpellID = 42842

type config struct {
	ID     int32
	AuraID int32
	Stacks int32
}

var configs = []config{{ID: 50362, AuraID: 71484, Stacks: 3}}

func rankSpellID(rank int32) int32 {
	if rank == 1 {
		return 14179
	}
	return []int32{0, 31221, 31222}[rank]
}

func spells(rank int32, heroic bool, spell core.ActionID, ids map[core.Stat]int32) {
	_ = core.ActionID{SpellID: 75}
	_ = core.ActionID{SpellID: FrostboltSpellID, Tag: 2}
	_ = core.ActionID{SpellID: core.TernaryInt32(heroic, 63512, 62478)}
	_ = core.ActionID{ItemID: 40211}
	_ = core.ActionID{SpellID: 58420 + rank}
	_ = core.ActionID{SpellID: rankSpellID(rank)}
	_ = core.NewAura("proc", 5, 71600)

	auraIDs := []int32{71561, 71556}
	_ = core.ActionID{SpellID: auraIDs[0]}
	_ = map[core.Stat]core.ActionID{core.Strength: {SpellID: 60229}}

	if spell.SpellID == 47465 || spell.IsSpellAction(12867) {
		return
	}
	switch spell.ItemID {
	case 40798, 40802:
	}
	var procSpellId int32
	procSpellId = 71846
	_ = procSpellId
}
