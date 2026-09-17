package azerothcore

import "testing"

func TestOverrideSpellKeepsClientText(t *testing.T) {
	client := &SpellEntry{ID: 65019, Name: "Mjolnir Runestone", Description: "Increases armor penetration rating by $s1 for $d."}
	row := &SpellEntry{ID: 65019, EffectBasePoints: [3]int32{664}, AuraDescription: "Custom aura text"}

	got := overrideSpell(client, row)
	if got.EffectBasePoints[0] != 664 {
		t.Errorf("numbers not taken from spell_dbc: %+v", got)
	}
	if got.Name != client.Name || got.Description != client.Description {
		t.Errorf("empty spell_dbc text replaced the client text: %+v", got)
	}
	if got.AuraDescription != "Custom aura text" {
		t.Errorf("spell_dbc text not applied: %q", got.AuraDescription)
	}

	serverOnly := &SpellEntry{ID: 900001}
	if overrideSpell(nil, serverOnly) != serverOnly {
		t.Error("server-only spell not added as is")
	}
}

func TestItemSpellCooldownMs(t *testing.T) {
	for _, tc := range []struct {
		cooldown, category, want int32
	}{
		{-1, -1, -1},
		{120000, -1, 120000},
		{-1, 60000, 60000},
		{30000, 60000, 60000},
		{0, -1, 0},
	} {
		s := ItemSpell{Cooldown: tc.cooldown, CategoryCooldown: tc.category}
		if got := s.CooldownMs(); got != tc.want {
			t.Errorf("CooldownMs(%d, %d) = %d, want %d", tc.cooldown, tc.category, got, tc.want)
		}
	}
}
