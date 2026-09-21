package optimizer

import (
	"sync"

	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
	goproto "google.golang.org/protobuf/proto"
)

// playerSheet is the target's character sheet in l, with offset added to its bonus stats, plus the
// chance the encounter's boss crits it. Each call builds a whole environment, about what setting up
// one sim costs.
func playerSheet(base *proto.RaidSimRequest, targetIndex int, l Loadout, offset stats.Stats) (core.PlayerSheet, error) {
	rsr := goproto.Clone(base).(*proto.RaidSimRequest)
	player := rsr.Raid.Parties[targetIndex/5].Players[targetIndex%5]
	player.Equipment = l.Equipment()
	player.RacialTraits = l.RacialTraits
	if offset != (stats.Stats{}) {
		if player.BonusStats == nil {
			player.BonusStats = &proto.UnitStats{}
		}
		player.BonusStats.Stats = stats.FromFloatArray(player.BonusStats.Stats).Add(offset).ToFloatArray()
	}
	return core.ComputePlayerSheet(rsr.Raid, rsr.Encounter, targetIndex)
}

// sheetMemo keeps each loadout's sheet, since a run asks for the same ones over and over: verify,
// the neighborhood and every result recheck floors, and each result reads its sheet again. The zero
// value is ready to use.
type sheetMemo struct {
	mu     sync.Mutex
	sheets map[Loadout]sheetEntry
}

type sheetEntry struct {
	sheet core.PlayerSheet
	err   error
}

// sheet is the target's character sheet in l, memoized per loadout. Nothing writes the pool's base
// after CompilePool (the healing pin runs before it), so a memoized sheet never goes stale.
func (p *Pool) sheet(l Loadout) (core.PlayerSheet, error) {
	m := &p.sheets
	m.mu.Lock()
	entry, ok := m.sheets[l]
	m.mu.Unlock()
	if ok {
		return entry.sheet, entry.err
	}
	// built outside the lock: two goroutines may both build one, but they get the same answer
	entry.sheet, entry.err = playerSheet(p.base, p.targetIndex, l, stats.Stats{})
	m.mu.Lock()
	if m.sheets == nil {
		m.sheets = map[Loadout]sheetEntry{}
	}
	m.sheets[l] = entry
	m.mu.Unlock()
	return entry.sheet, entry.err
}

// finalStats is the target's final stats in l, with buffs, as core.ComputeStats reports them to the
// UI.
func (p *Pool) finalStats(l Loadout) (stats.Stats, error) {
	sheet, err := p.sheet(l)
	return sheet.FinalStats, err
}
