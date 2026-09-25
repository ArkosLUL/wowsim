package azerothcore

import "testing"

func TestMarkPlayers(t *testing.T) {
	characters := map[uint32]*CharacterRows{
		1: {Name: "Deathsong"},
		2: {Name: "Nightwarrior"},
		3: {Name: "Felesta"},
	}

	warnings := markPlayers(characters, []string{"deathsong", "Nightwarrior", "Nobody"})
	for guid, wantBot := range map[uint32]bool{1: false, 2: false, 3: true} {
		if characters[guid].Bot != wantBot {
			t.Errorf("%s: bot %v, want %v", characters[guid].Name, characters[guid].Bot, wantBot)
		}
	}
	if len(warnings) != 1 || findWarning(warnings, "Nobody") == "" {
		t.Errorf("warnings = %v", warnings)
	}
}
