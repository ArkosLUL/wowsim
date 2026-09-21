package optimizer

import (
	"sync"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

// The pool's sheets match a fresh build, one memo entry per loadout, racial traits included, and
// goroutines can share it.
func TestPoolSheetMemo(t *testing.T) {
	p := rtPool(t, nil)
	geared := rtLoadout()
	human := geared
	human.RacialTraits = proto.Race_RaceHuman
	var empty Loadout
	empty.RacialTraits = geared.RacialTraits
	loadouts := []Loadout{geared, human, empty}

	for round := 0; round < 2; round++ {
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			for _, l := range loadouts {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := p.sheet(l); err != nil {
						t.Error(err)
					}
				}()
			}
		}
		wg.Wait()
	}
	if n := len(p.sheets.sheets); n != len(loadouts) {
		t.Errorf("%d memo entries, want one per loadout (%d)", n, len(loadouts))
	}
	for i, l := range loadouts {
		want, err := playerSheet(p.base, p.targetIndex, l, stats.Stats{})
		if err != nil {
			t.Fatal(err)
		}
		got, err := p.sheet(l)
		if err != nil || got != want {
			t.Errorf("loadout %d: memo %+v (%v), fresh %+v", i, got, err, want)
		}
	}
}
