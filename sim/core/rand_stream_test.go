package core

import (
	"strconv"
	"testing"

	"github.com/wowsims/wotlk/sim/core/proto"
)

// every golden pins its RNG draws to this exact formula; seedFromLabelHash only caches the
// re-hash, it must never change the result
func TestSeedFromLabelHashMatchesConcatenatedHash(t *testing.T) {
	labels := []string{"Damage Roll", "Physical Crit Roll", "jow", ""}
	seeds := []int64{0, 1, -1, 12345, -987654321, 9007199254740993, -9007199254740993}

	for _, label := range labels {
		labelHash := fnvHashString(fnvOffset32, label)
		for _, rseed := range seeds {
			got := seedFromLabelHash(labelHash, rseed)
			want := int64(hash(label + strconv.FormatInt(rseed, 16)))
			if got != want {
				t.Errorf("seedFromLabelHash(%q, %d) = %d, want %d", label, rseed, got, want)
			}
		}
	}
}

func TestUnitRandomFloatMatchesRandomFloatWhenUnscoped(t *testing.T) {
	const label = "Test Label"

	shared := &Simulation{isTest: true, rseed: 42, testRands: map[string]*testRandStream{}, Options: &proto.SimOptions{}}
	want := shared.RandomFloat(label)

	nilUnit := &Simulation{isTest: true, rseed: 42, testRands: map[string]*testRandStream{}, Options: &proto.SimOptions{}}
	if got := nilUnit.UnitRandomFloat(nil, label); got != want {
		t.Errorf("UnitRandomFloat(nil, ...) = %v, want %v (RandomFloat)", got, want)
	}

	optionOff := &Simulation{isTest: true, rseed: 42, testRands: map[string]*testRandStream{}, Options: &proto.SimOptions{}}
	if got := optionOff.UnitRandomFloat(&Unit{UnitIndex: 5}, label); got != want {
		t.Errorf("UnitRandomFloat(unit, ...) with PerUnitRandomSeeds off = %v, want %v (RandomFloat)", got, want)
	}
}

func TestUnitRandomFloatSeparatesStreamsPerUnit(t *testing.T) {
	const label = "Test Label"
	sim := &Simulation{isTest: true, rseed: 42, testRands: map[string]*testRandStream{}, Options: &proto.SimOptions{PerUnitRandomSeeds: true}}
	u1 := &Unit{UnitIndex: 0}
	u2 := &Unit{UnitIndex: 1}

	v1 := sim.UnitRandomFloat(u1, label)
	v2 := sim.UnitRandomFloat(u2, label)
	if v1 == v2 {
		t.Errorf("expected units 0 and 1 to draw from separate streams, both got %v", v1)
	}

	again := sim.UnitRandomFloat(u1, label)
	if again == v1 {
		t.Errorf("expected the second draw on the same stream to advance, got %v both times", v1)
	}
}
