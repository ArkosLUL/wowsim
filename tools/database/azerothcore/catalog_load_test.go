package azerothcore

import (
	"slices"
	"testing"
)

func TestLearnedSpells(t *testing.T) {
	dbc := &DBC{Spells: map[int32]*SpellEntry{
		// "Learning": the recipe's learn slot names the spell
		483: {ID: 483, Effect: [3]int32{spellEffectLearnSpell}},
		// an old recipe's on-use spell names it itself
		18040: {ID: 18040, Effect: [3]int32{SpellEffectApplyAura, spellEffectLearnSpell}, EffectTriggerSpell: [3]int32{99, 17635}},
		2963:  {ID: 2963, Effect: [3]int32{SpellEffectCreateItem}, EffectTriggerSpell: [3]int32{5}},
	}}
	tests := []struct {
		spell int32
		want  []int32
	}{
		{483, nil},
		{18040, []int32{17635}},
		{2963, nil},
		{1, nil},
	}
	for _, tt := range tests {
		if got := learnedSpells(dbc, tt.spell); !slices.Equal(got, tt.want) {
			t.Errorf("learnedSpells(%d) = %v, want %v", tt.spell, got, tt.want)
		}
	}
}
